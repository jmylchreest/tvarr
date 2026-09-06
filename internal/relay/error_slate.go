// Package relay provides stream relay functionality including transcoding,
// connection pooling, and failure handling.
package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// StreamErrorKind classifies why a stream cannot be delivered. It maps onto the
// PlaceholderType values so the same taxonomy drives both the embedded static
// placeholders and the rendered slates.
type StreamErrorKind string

const (
	// StreamErrorUnavailable covers an origin that cannot be reached or read.
	StreamErrorUnavailable StreamErrorKind = "unavailable"
	// StreamErrorLimitReached covers provider-side concurrency refusals.
	StreamErrorLimitReached StreamErrorKind = "limit_reached"
	// StreamErrorEnded covers an origin that closed the stream cleanly.
	StreamErrorEnded StreamErrorKind = "ended"
	// StreamErrorStarting covers the gap before the first origin bytes arrive.
	StreamErrorStarting StreamErrorKind = "starting"
)

// PlaceholderType maps the error kind onto the buffer injector's placeholder
// taxonomy, so a slate can fall back to embedded static content when no encoder
// exists for the negotiated variant.
func (k StreamErrorKind) PlaceholderType() PlaceholderType {
	switch k {
	case StreamErrorLimitReached:
		return PlaceholderLimitReached
	case StreamErrorEnded:
		return PlaceholderEnded
	case StreamErrorStarting:
		return PlaceholderStarting
	default:
		return PlaceholderUnavailable
	}
}

// StreamError describes a stream failure in terms a viewer can act on. It
// carries both the machine-readable kind and the two lines rendered onto the
// slate, so the same value drives logging, the session state and the picture.
type StreamError struct {
	// Kind classifies the failure.
	Kind StreamErrorKind
	// Headline is the large first line, e.g. "Channel Unavailable".
	Headline string
	// Detail is the smaller second line naming the specific cause.
	Detail string
	// HTTPStatus is the upstream status when the failure came from a response.
	HTTPStatus int
	// Err is the underlying error, if any.
	Err error
}

// Error implements the error interface.
func (e *StreamError) Error() string {
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("%s: %s (upstream HTTP %d)", e.Headline, e.Detail, e.HTTPStatus)
	}
	return fmt.Sprintf("%s: %s", e.Headline, e.Detail)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *StreamError) Unwrap() error { return e.Err }

// CacheKey identifies the rendered slate for this error. Errors that render
// identically share one encode, so a flapping origin does not re-encode a slate
// on every retry.
func (e *StreamError) CacheKey() string {
	return string(e.Kind) + "\x00" + e.Headline + "\x00" + e.Detail
}

// NewUpstreamStatusError builds a StreamError from an upstream HTTP status.
//
// Xtream-family panels do not use the documented Player API JSON to signal a
// refusal on the streaming endpoint; they return a bare non-standard status from
// the nginx layer in front of the panel. 555 and 999 are both in live use and
// are not defined by HTTP or by the Xtream API, so they are classified here
// rather than left to a generic 5xx branch. The account is typically fine -- the
// same credentials keep working against player_api.php -- so the slate says
// "too many connections" rather than implying the subscription is dead.
func NewUpstreamStatusError(status int) *StreamError {
	e := &StreamError{HTTPStatus: status}

	switch {
	case status == 429, status == 509, status == 555, status == 999:
		e.Kind = StreamErrorLimitReached
		e.Headline = "Too Many Connections"
		e.Detail = fmt.Sprintf("Provider refused the stream (HTTP %d). Another device may be watching.", status)

	case status == 401 || status == 403:
		e.Kind = StreamErrorUnavailable
		e.Headline = "Not Authorised"
		e.Detail = fmt.Sprintf("Provider rejected the credentials (HTTP %d).", status)

	case status == 404 || status == 410:
		e.Kind = StreamErrorEnded
		e.Headline = "Channel Not Found"
		e.Detail = fmt.Sprintf("Provider no longer offers this stream (HTTP %d).", status)

	case status >= 500:
		e.Kind = StreamErrorUnavailable
		e.Headline = "Provider Error"
		e.Detail = fmt.Sprintf("Upstream server returned HTTP %d.", status)

	default:
		e.Kind = StreamErrorUnavailable
		e.Headline = "Channel Unavailable"
		e.Detail = fmt.Sprintf("Upstream returned HTTP %d.", status)
	}

	return e
}

// ClassifyStreamError maps an arbitrary pipeline error onto a StreamError. An
// error that is already a *StreamError passes through unchanged, so a precise
// classification made at the failure site is never flattened by a later caller.
func ClassifyStreamError(err error) *StreamError {
	if err == nil {
		return nil
	}

	var se *StreamError
	if errors.As(err, &se) {
		return se
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &StreamError{
			Kind:     StreamErrorEnded,
			Headline: "Stream Stopped",
			Detail:   "The session was closed.",
			Err:      err,
		}
	}

	// DNS failure: the provider hostname no longer resolves. Distinguished from a
	// refused connection because it is almost always a dead or changed provider
	// domain rather than a transient outage, and the viewer's fix differs.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &StreamError{
			Kind:     StreamErrorUnavailable,
			Headline: "Provider Not Found",
			Detail:   fmt.Sprintf("Cannot resolve %s.", dnsErr.Name),
			Err:      err,
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &StreamError{
			Kind:     StreamErrorUnavailable,
			Headline: "Provider Timed Out",
			Detail:   "The upstream server did not respond in time.",
			Err:      err,
		}
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return &StreamError{
			Kind:     StreamErrorUnavailable,
			Headline: "Connection Refused",
			Detail:   "The upstream server refused the connection.",
			Err:      err,
		}
	case strings.Contains(msg, "connection reset"):
		return &StreamError{
			Kind:     StreamErrorUnavailable,
			Headline: "Connection Dropped",
			Detail:   "The upstream server closed the connection.",
			Err:      err,
		}
	case strings.Contains(msg, "no daemon") || strings.Contains(msg, "no available daemon"):
		return &StreamError{
			Kind:     StreamErrorUnavailable,
			Headline: "Transcoder Unavailable",
			Detail:   "No transcoding capacity is currently available.",
			Err:      err,
		}
	}

	return &StreamError{
		Kind:     StreamErrorUnavailable,
		Headline: "Channel Unavailable",
		Detail:   "The stream could not be started.",
		Err:      err,
	}
}

// ErrorSlateConfig configures slate rendering and encoding.
type ErrorSlateConfig struct {
	// Width and Height of the rendered slate. These must match the resolution
	// already advertised by the variant's init data when injecting mid-stream,
	// or decoders will reject the samples.
	Width  int
	Height int
	// FrameRate of the encoded slate. A still image needs very few frames per
	// second; this is kept low deliberately because every frame is piped to
	// FFmpeg uncompressed.
	FrameRate int
	// Duration in seconds of one encoded loop.
	Duration float64
	// VideoBitrate in kbps.
	VideoBitrate int
	// FFmpegPath is the ffmpeg binary to shell out to.
	FFmpegPath string
	// Timeout bounds a single encode.
	Timeout time.Duration
	// Scale is the integer upscale applied to the bitmap font. Glyphs are
	// nearest-neighbour scaled so they stay crisp rather than blurred.
	Scale int
}

// DefaultErrorSlateConfig returns defaults sized for a 720p slate.
func DefaultErrorSlateConfig() ErrorSlateConfig {
	return ErrorSlateConfig{
		Width:        1280,
		Height:       720,
		FrameRate:    10,
		Duration:     2.0,
		VideoBitrate: 800,
		FFmpegPath:   "ffmpeg",
		Timeout:      30 * time.Second,
		Scale:        3,
	}
}

// ErrGeneratorNoEncoder is returned when the negotiated variant has no software
// encoder in this FFmpeg build, so no slate can be rendered for it.
var ErrGeneratorNoEncoder = errors.New("no software encoder for variant")

// ErrorSlateGenerator renders StreamErrors to ES samples for a codec variant.
//
// Text is rasterised in Go and piped to FFmpeg as raw frames rather than drawn
// with the drawtext filter: the ffmpeg build shipped in the tvarr image is
// compiled without libfreetype, so drawtext does not exist in it, and the image
// carries no fonts at all. Rasterising here also keeps the slate identical
// across every deployment regardless of what fontconfig would have found.
type ErrorSlateGenerator struct {
	config   ErrorSlateConfig
	injector *BufferInjector
	logger   *slog.Logger

	mu    sync.RWMutex
	cache map[string]*CachedPlaceholder
}

// NewErrorSlateGenerator creates a slate generator.
func NewErrorSlateGenerator(config ErrorSlateConfig, logger *slog.Logger) *ErrorSlateGenerator {
	if logger == nil {
		logger = slog.Default()
	}
	if config.Width <= 0 || config.Height <= 0 {
		d := DefaultErrorSlateConfig()
		config.Width, config.Height = d.Width, d.Height
	}
	if config.FrameRate <= 0 {
		config.FrameRate = DefaultErrorSlateConfig().FrameRate
	}
	if config.Duration <= 0 {
		config.Duration = DefaultErrorSlateConfig().Duration
	}
	if config.FFmpegPath == "" {
		config.FFmpegPath = "ffmpeg"
	}
	if config.Timeout <= 0 {
		config.Timeout = DefaultErrorSlateConfig().Timeout
	}
	if config.Scale <= 0 {
		config.Scale = DefaultErrorSlateConfig().Scale
	}

	return &ErrorSlateGenerator{
		config:   config,
		injector: GetBufferInjector(),
		logger:   logger,
		cache:    make(map[string]*CachedPlaceholder),
	}
}

// cacheKey combines variant, size and message: the same text must be encoded
// once per codec variant AND per resolution, because a slate spliced onto a live
// track has to match the parameter sets that track already advertises.
func (g *ErrorSlateGenerator) cacheKey(variant CodecVariant, size SlateSize, se *StreamError) string {
	return fmt.Sprintf("%s\x00%dx%d\x00%s", variant, size.Width, size.Height, se.CacheKey())
}

// SlateSize is the raster size of a slate. Zero values mean "use the
// generator's configured default".
type SlateSize struct {
	Width  int
	Height int
}

// normalise fills in generator defaults for any zero dimension and forces even
// dimensions, which yuv420p requires.
func (s SlateSize) normalise(cfg ErrorSlateConfig) SlateSize {
	if s.Width <= 0 || s.Height <= 0 {
		s = SlateSize{Width: cfg.Width, Height: cfg.Height}
	}
	s.Width &^= 1
	s.Height &^= 1
	return s
}

// Slate returns ES samples rendering the error for the variant at the requested
// size, encoding on first use and serving from cache afterwards.
//
// Size matters as much as codec. Splicing a 720p slate onto a track that has
// already published 1080p SPS/PPS gives the decoder slices its parameter sets do
// not describe, so callers pass the live stream's resolution when there is one.
func (g *ErrorSlateGenerator) Slate(ctx context.Context, variant CodecVariant, se *StreamError, size SlateSize) (*CachedPlaceholder, error) {
	if se == nil {
		return nil, errors.New("nil stream error")
	}

	size = size.normalise(g.config)
	key := g.cacheKey(variant, size, se)

	g.mu.RLock()
	cached, ok := g.cache[key]
	g.mu.RUnlock()
	if ok {
		return cached, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check under the write lock: two sessions can fail concurrently.
	if cached, ok := g.cache[key]; ok {
		return cached, nil
	}

	start := time.Now()

	data, err := g.encode(ctx, variant, se, size)
	if err != nil {
		return nil, err
	}

	// Reuse the buffer injector's fMP4 demuxer so slates and the embedded
	// placeholders produce identically shaped ES samples.
	parsed, err := g.injector.demuxFMP4(data, variant)
	if err != nil {
		return nil, fmt.Errorf("demux slate: %w", err)
	}

	if err := toAnnexBInPlace(parsed, variant); err != nil {
		return nil, fmt.Errorf("converting slate to the live sample format: %w", err)
	}

	g.cache[key] = parsed

	g.logger.Info("rendered error slate",
		slog.String("variant", string(variant)),
		slog.String("size", fmt.Sprintf("%dx%d", size.Width, size.Height)),
		slog.String("kind", string(se.Kind)),
		slog.String("headline", se.Headline),
		slog.Int("video_samples", len(parsed.VideoSamples)),
		slog.Int("audio_samples", len(parsed.AudioSamples)),
		slog.Duration("encode_time", time.Since(start)),
	)

	return parsed, nil
}

// slateUnsupportedVideoCodecs lists codecs we will not render a slate in even
// when FFmpeg can encode them.
//
// AV1 is excluded because mediacommon's fMP4 parsing does not handle it -- the
// same limitation buffer_injector.go documents for the embedded AV1 placeholder.
// FFmpeg encodes it happily and the demux then fails with "not enough bits", so
// the check has to be here rather than left to the encoder probe.
var slateUnsupportedVideoCodecs = map[string]bool{
	"av1": true,
}

// encoderProbe caches which encoders the FFmpeg binary actually provides.
type encoderProbe struct {
	once     sync.Once
	encoders map[string]bool
	err      error
}

var ffmpegEncoders sync.Map // ffmpeg path -> *encoderProbe

// availableEncoders asks the FFmpeg binary what it can encode, once per binary.
//
// The codec registry knows libaom-av1 exists; it cannot know whether the build
// in this image was compiled with it. The tvarr image ships a custom FFmpeg, so
// capability has to be read from the binary rather than assumed.
func availableEncoders(ffmpegPath string) (map[string]bool, error) {
	v, _ := ffmpegEncoders.LoadOrStore(ffmpegPath, &encoderProbe{})
	probe := v.(*encoderProbe)

	probe.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders").Output()
		if err != nil {
			probe.err = fmt.Errorf("probing ffmpeg encoders: %w", err)
			return
		}

		found := make(map[string]bool)
		for _, line := range strings.Split(string(out), "\n") {
			// Lines look like " V....D libx264   libx264 H.264 / AVC ...".
			fields := strings.Fields(line)
			if len(fields) < 2 || !strings.HasPrefix(line, " ") {
				continue
			}
			found[fields[1]] = true
		}
		probe.encoders = found
	})

	return probe.encoders, probe.err
}

// checkEncodable reports whether a slate can be produced for the variant.
func (g *ErrorSlateGenerator) checkEncodable(variant CodecVariant, videoEncoder, audioEncoder string) error {
	if slateUnsupportedVideoCodecs[variant.VideoCodec()] {
		return fmt.Errorf("%w: %s cannot be demuxed back to ES samples", ErrGeneratorNoEncoder, variant.VideoCodec())
	}
	if videoEncoder == "" {
		return fmt.Errorf("%w: no encoder for %s", ErrGeneratorNoEncoder, variant.VideoCodec())
	}

	encoders, err := availableEncoders(g.config.FFmpegPath)
	if err != nil {
		// If the probe itself failed, let the encode attempt produce the real
		// error rather than blocking a slate we might have been able to render.
		g.logger.Warn("could not probe ffmpeg encoders",
			slog.String("error", err.Error()))
		return nil
	}

	if !encoders[videoEncoder] {
		return fmt.Errorf("%w: ffmpeg has no %s", ErrGeneratorNoEncoder, videoEncoder)
	}
	if audioEncoder != "" && !encoders[audioEncoder] {
		return fmt.Errorf("%w: ffmpeg has no %s", ErrGeneratorNoEncoder, audioEncoder)
	}
	return nil
}

// encode rasterises the slate and encodes it to a fragmented MP4.
func (g *ErrorSlateGenerator) encode(ctx context.Context, variant CodecVariant, se *StreamError, size SlateSize) ([]byte, error) {
	videoEncoder := codec.GetVideoEncoder(codec.Video(variant.VideoCodec()), codec.HWAccelNone)
	audioEncoder := codec.GetAudioEncoder(codec.Audio(variant.AudioCodec()))
	if err := g.checkEncodable(variant, videoEncoder, audioEncoder); err != nil {
		return nil, err
	}

	img := g.render(se, size)
	frame := rgbaFrameBytes(img)
	frameCount := max(int(g.config.Duration*float64(g.config.FrameRate)), 1)

	ctx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		// Raw RGBA frames on stdin. No demuxer probing, no font handling.
		"-f", "rawvideo",
		"-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", size.Width, size.Height),
		"-framerate", fmt.Sprintf("%d", g.config.FrameRate),
		"-i", "pipe:0",
		// Silent audio so clients that expect an audio track keep their decoder
		// alive across the switch into and back out of the slate.
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=48000:cl=stereo:d=%.3f", g.config.Duration),
		"-c:v", videoEncoder,
	}

	// x264/x265 still-image tunings, bound to the video stream.
	if videoEncoder == "libx264" || videoEncoder == "libx265" {
		args = append(args, "-preset", "ultrafast", "-tune", "stillimage")
	}

	args = append(args,
		"-pix_fmt", "yuv420p",
		"-b:v", fmt.Sprintf("%dk", g.config.VideoBitrate),
		// Every frame a keyframe: the slate is looped by re-injecting its samples,
		// and a consumer joining mid-loop must be able to decode immediately.
		"-g", "1",
		"-c:a", audioEncoder,
		"-b:a", "96k",
		"-shortest",
		"-f", "mp4",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, g.config.FFmpegPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	// Feed the identical frame frameCount times. The encoder collapses them to
	// near-nothing, and this avoids relying on the loop filter being compiled in.
	writeErr := func() error {
		defer stdin.Close()
		for range frameCount {
			if _, err := stdin.Write(frame); err != nil {
				return err
			}
		}
		return nil
	}()

	waitErr := cmd.Wait()

	if waitErr != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w (stderr: %s)", waitErr, strings.TrimSpace(stderr.String()))
	}
	// A broken pipe once ffmpeg has exited cleanly is benign: it means the encoder
	// had all the frames it needed. Only surface a write error that lost frames.
	if writeErr != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("feeding frames to ffmpeg: %w", writeErr)
	}
	if stdout.Len() == 0 {
		return nil, errors.New("ffmpeg produced no output")
	}

	return stdout.Bytes(), nil
}

// Slate colours. Deliberately dark: these appear full-screen on a television,
// often at night, and a bright slate after a failed channel change is hostile.
var (
	slateBackground = color.RGBA{R: 0x0d, G: 0x0f, B: 0x14, A: 0xff}
	slateAccent     = color.RGBA{R: 0xe5, G: 0x48, B: 0x4b, A: 0xff}
	slateHeadline   = color.RGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff}
	slateDetail     = color.RGBA{R: 0x9a, G: 0xa4, B: 0xb2, A: 0xff}
)

// slateBlock is one positioned run of text in the slate layout.
type slateBlock struct {
	face   font.Face
	text   string
	scale  int
	colour color.RGBA
	rect   image.Rectangle
}

// faceLineHeight returns the full line height of a face, in pixels at 1x.
func faceLineHeight(f font.Face) int {
	m := f.Metrics()
	return (m.Ascent + m.Descent).Round()
}

// layoutSlate positions the accent rule and the text blocks as one vertically
// centred stack.
//
// Every offset comes from the measured line height of the face that will draw
// it. Fixed multiples of the scale were used here at first and they overlapped:
// drawScaledText takes y as the TOP of a block, so a 48px headline drawn at the
// vertical centre ran to centre+48 while the detail line began at centre+30, and
// the two were rendered through each other on screen.
//
// Returned separately from the drawing so the geometry can be asserted directly.
func layoutSlate(se *StreamError, size SlateSize, headScale, detailScale int) (image.Rectangle, []slateBlock) {
	headFace := inconsolata.Bold8x16
	detailFace := basicfont.Face7x13

	headH := faceLineHeight(headFace) * headScale
	detailH := faceLineHeight(detailFace) * detailScale

	// Breathing room proportional to the type, so it holds at any raster size.
	gap := headH / 2
	ruleH := max(headScale, 2)
	ruleW := size.Width / 3

	// Wrap the detail so a long provider message does not run off the raster.
	margin := size.Width / 10
	advance := max(detailFace.Advance*detailScale, 1)
	lines := wrapText(se.Detail, max((size.Width-2*margin)/advance, 20))

	total := ruleH + gap + headH
	if len(lines) > 0 {
		total += gap + len(lines)*detailH
	}

	y := max((size.Height-total)/2, 0)

	rule := image.Rect((size.Width-ruleW)/2, y, (size.Width+ruleW)/2, y+ruleH)
	y += ruleH + gap

	blocks := make([]slateBlock, 0, 1+len(lines))

	headW := textWidth(headFace, se.Headline) * headScale
	blocks = append(blocks, slateBlock{
		face: headFace, text: se.Headline, scale: headScale, colour: slateHeadline,
		rect: image.Rect((size.Width-headW)/2, y, (size.Width+headW)/2, y+headH),
	})
	y += headH + gap

	for _, line := range lines {
		w := textWidth(detailFace, line) * detailScale
		blocks = append(blocks, slateBlock{
			face: detailFace, text: line, scale: detailScale, colour: slateDetail,
			rect: image.Rect((size.Width-w)/2, y, (size.Width+w)/2, y+detailH),
		})
		y += detailH
	}

	return rule, blocks
}

// slateScales returns the type scales for a raster, so a 4K slate is not
// captioned in text sized for 720p and an SD one does not overflow.
func (g *ErrorSlateGenerator) slateScales(size SlateSize) (head, detail int) {
	head = max(g.config.Scale*size.Height/720, 1)
	return head, max(head-1, 1)
}

// render rasterises the slate for an error at the given size.
func (g *ErrorSlateGenerator) render(se *StreamError, size SlateSize) *image.RGBA {
	size = size.normalise(g.config)

	img := image.NewRGBA(image.Rect(0, 0, size.Width, size.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{slateBackground}, image.Point{}, draw.Src)

	headScale, detailScale := g.slateScales(size)
	rule, blocks := layoutSlate(se, size, headScale, detailScale)

	drawRect(img, rule.Min.X, rule.Min.Y, rule.Dx(), rule.Dy(), slateAccent)
	for _, b := range blocks {
		drawScaledText(img, b.face, b.text, b.rect.Min.X, b.rect.Min.Y, b.scale, b.colour)
	}

	return img
}

// drawRect fills an axis-aligned rectangle.
func drawRect(dst *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(dst.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(dst, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// textWidth returns the unscaled pixel width of s in the given face.
func textWidth(face font.Face, s string) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(s).Round()
}

// drawScaledText renders text with a bitmap face and nearest-neighbour upscales
// it by scale. Bitmap fonts avoid shipping a TTF and, scaled by an integer
// factor, stay crisp instead of the blur a resampled glyph would give.
func drawScaledText(dst *image.RGBA, face font.Face, s string, x, y, scale int, c color.RGBA) {
	if s == "" {
		return
	}

	w := textWidth(face, s)
	m := face.Metrics()
	h := (m.Ascent + m.Descent).Round()
	if w <= 0 || h <= 0 {
		return
	}

	// Render once at 1x into a scratch mask.
	scratch := image.NewRGBA(image.Rect(0, 0, w, h))
	d := &font.Drawer{
		Dst:  scratch,
		Src:  &image.Uniform{c},
		Face: face,
		Dot:  fixed.P(0, m.Ascent.Round()),
	}
	d.DrawString(s)

	// Blit each source pixel as a scale x scale block.
	for sy := range h {
		for sx := range w {
			if scratch.RGBAAt(sx, sy).A == 0 {
				continue
			}
			drawRect(dst, x+sx*scale, y+sy*scale, scale, scale, c)
		}
	}
}

// wrapText breaks s onto lines of at most width characters, splitting on spaces.
func wrapText(s string, width int) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}

	var (
		lines []string
		cur   strings.Builder
	)
	for _, f := range fields {
		if cur.Len() > 0 && cur.Len()+1+len(f) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(f)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// rgbaFrameBytes returns the tightly packed RGBA bytes for one frame. image.RGBA
// may carry a stride wider than the row, which the rawvideo demuxer would read
// as skewed pixels, so rows are copied out individually when that happens.
func rgbaFrameBytes(img *image.RGBA) []byte {
	b := img.Bounds()
	rowLen := b.Dx() * 4
	if img.Stride == rowLen {
		return img.Pix
	}

	out := make([]byte, 0, rowLen*b.Dy())
	for y := range b.Dy() {
		start := y * img.Stride
		out = append(out, img.Pix[start:start+rowLen]...)
	}
	return out
}

// toAnnexBInPlace rewrites slate samples into the format the live pipeline uses.
//
// The two sources disagree, and splicing them onto one track without this is
// undecodable no matter how well the timestamps line up:
//
//   - The TS demuxer emits Annex-B (h264.AnnexB(au).Marshal()) with SPS/PPS
//     carried inline in each keyframe access unit, and sets the track's init data
//     to nil, because that is how MPEG-TS delivers parameter sets.
//   - fMP4 samples are AVCC -- length-prefixed NAL units -- with the parameter
//     sets held once in the init segment and never in the samples.
//
// So each slate sample is converted to Annex-B, and the parameter sets recovered
// from the fMP4 init data are prepended to every keyframe. A decoder joining on a
// slate keyframe then has everything it needs from the sample alone, exactly as
// it would from the live stream.
func toAnnexBInPlace(p *CachedPlaceholder, variant CodecVariant) error {
	var params [][]byte

	switch variant.VideoCodec() {
	case "h264", "avc":
		sps, pps := parseH264InitData(p.VideoInitData)
		if len(sps) > 0 {
			params = append(params, sps)
		}
		if len(pps) > 0 {
			params = append(params, pps)
		}
	case "h265", "hevc":
		vps, sps, pps := parseH265InitData(p.VideoInitData)
		for _, nal := range [][]byte{vps, sps, pps} {
			if len(nal) > 0 {
				params = append(params, nal)
			}
		}
	default:
		// Other codecs are not spliced onto an Annex-B track.
		return nil
	}

	for i, sample := range p.VideoSamples {
		var avcc h264.AVCC
		if err := avcc.Unmarshal(sample.Data); err != nil {
			return fmt.Errorf("sample %d: %w", i, err)
		}

		au := [][]byte(avcc)
		if len(params) > 0 {
			au = append(append([][]byte{}, params...), au...)
		}

		annexB, err := h264.AnnexB(au).Marshal()
		if err != nil {
			return fmt.Errorf("sample %d: %w", i, err)
		}
		p.VideoSamples[i].Data = annexB

		// Every slate frame is encoded with -g 1, so every one is an IDR and
		// now carries its own parameter sets. Marking them says so.
		//
		// This is not cosmetic: the fMP4 sample flags come back with no keyframe
		// set at all, and the output processors pull via ReadFromKeyframe. Left
		// unmarked, consumers find no keyframe to start from and the slate never
		// reaches the viewer -- the dead stream, one layer further down.
		p.VideoSamples[i].IsKeyframe = true
	}

	// The parameter sets now travel in the samples, so the init data must not be
	// published as well: a live track never has any, and adopting it here would
	// make the slate's track shape differ from the stream it replaces.
	p.VideoInitData = nil

	return nil
}

// spsSizeFromAnnexB scans an Annex-B access unit for a parameter set and returns
// the coded resolution it describes.
//
// Live MPEG-TS keeps SPS inline in keyframes rather than in track init data, so
// this is the only place the live resolution can actually be read from.
func spsSizeFromAnnexB(data []byte, videoCodec string) (SlateSize, bool) {
	var au h264.AnnexB
	if err := au.Unmarshal(data); err != nil {
		return SlateSize{}, false
	}

	for _, nal := range au {
		if len(nal) == 0 {
			continue
		}

		switch videoCodec {
		case "h264", "avc":
			if nal[0]&0x1f != 7 { // SPS
				continue
			}
			var sps h264.SPS
			if err := sps.Unmarshal(nal); err != nil {
				continue
			}
			return SlateSize{Width: sps.Width(), Height: sps.Height()}, true

		case "h265", "hevc":
			if len(nal) < 2 || (nal[0]>>1)&0x3f != 33 { // SPS
				continue
			}
			var sps h265.SPS
			if err := sps.Unmarshal(nal); err != nil {
				continue
			}
			return SlateSize{Width: sps.Width(), Height: sps.Height()}, true
		}
	}

	return SlateSize{}, false
}

// InjectErrorSlate loops slate samples into the variant starting at startPTS and
// returns the PTS immediately after the last sample written.
//
// Timestamps continue from the caller's startPTS rather than restarting at zero.
// A decoder that sees PTS jump backwards when the stream switches to the slate
// treats it as a discontinuity and, on most clients, drops the stream -- which
// is the dead stream this whole path exists to avoid.
func InjectErrorSlate(variant *ESVariant, slate *CachedPlaceholder, startPTS int64, target time.Duration) int64 {
	if variant == nil || slate == nil || len(slate.VideoSamples) == 0 {
		return startPTS
	}

	// Video parameter sets travel inline in the samples (see toAnnexBInPlace), so
	// nothing is published to the video track here: a live track carries no video
	// init data either, and the slate must not make the track look different.
	//
	// Audio is the opposite: AAC config lives in the track's init data in both
	// paths, so adopt it only when the track has none of its own.
	if variant.AudioTrack().GetInitData() == nil && len(slate.AudioInitData) > 0 {
		variant.AudioTrack().SetInitData(slate.AudioInitData)
	}

	loopPTS := int64(slate.Duration.Seconds() * 90000)
	if loopPTS <= 0 {
		loopPTS = int64(90000)
	}
	loops := max(int(target/slate.Duration), 1)

	videoTrack := variant.VideoTrack()
	audioTrack := variant.AudioTrack()

	for loop := range loops {
		offset := startPTS + int64(loop)*loopPTS
		for _, s := range slate.VideoSamples {
			videoTrack.Write(s.PTS+offset, s.DTS+offset, s.Data, s.IsKeyframe)
		}
		for _, s := range slate.AudioSamples {
			audioTrack.Write(s.PTS+offset, s.DTS+offset, s.Data, s.IsKeyframe)
		}
	}

	return startPTS + int64(loops)*loopPTS
}
