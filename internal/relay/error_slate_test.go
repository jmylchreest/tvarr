package relay

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// newTestESVariant builds a standalone h264/aac variant for injection tests.
func newTestESVariant(t *testing.T) *ESVariant {
	t.Helper()
	return NewESVariantWithMaxBytes(NewCodecVariant("h264", "aac"), 8<<20, true)
}

func TestNewUpstreamStatusError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantKind StreamErrorKind
		wantWord string
	}{
		// The two codes seen in production from the Xtream panel. Neither is a
		// real HTTP status, and both mean the panel refused the stream while the
		// same account still authenticates against player_api.php.
		{"xtream 555", 555, StreamErrorLimitReached, "Too Many Connections"},
		{"xtream 999", 999, StreamErrorLimitReached, "Too Many Connections"},
		{"too many requests", 429, StreamErrorLimitReached, "Too Many Connections"},
		{"bandwidth limit", 509, StreamErrorLimitReached, "Too Many Connections"},
		{"unauthorized", 401, StreamErrorUnavailable, "Not Authorised"},
		{"forbidden", 403, StreamErrorUnavailable, "Not Authorised"},
		{"not found", 404, StreamErrorEnded, "Channel Not Found"},
		{"gone", 410, StreamErrorEnded, "Channel Not Found"},
		{"server error", 500, StreamErrorUnavailable, "Provider Error"},
		{"bad gateway", 502, StreamErrorUnavailable, "Provider Error"},
		{"teapot", 418, StreamErrorUnavailable, "Channel Unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUpstreamStatusError(tt.status)
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Headline != tt.wantWord {
				t.Errorf("Headline = %q, want %q", got.Headline, tt.wantWord)
			}
			if got.HTTPStatus != tt.status {
				t.Errorf("HTTPStatus = %d, want %d", got.HTTPStatus, tt.status)
			}
			// The status must reach the viewer: it is the one piece of detail that
			// tells them (or us, from a photo of the TV) what the provider said.
			if !strings.Contains(got.Detail, fmt.Sprintf("%d", tt.status)) {
				t.Errorf("Detail %q does not mention status %d", got.Detail, tt.status)
			}
		})
	}
}

func TestClassifyStreamError(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if got := ClassifyStreamError(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("existing StreamError passes through unchanged", func(t *testing.T) {
		orig := NewUpstreamStatusError(555)
		got := ClassifyStreamError(fmt.Errorf("wrapped: %w", orig))
		if got != orig {
			t.Fatalf("classification replaced the precise error: got %+v", got)
		}
		if got.Kind != StreamErrorLimitReached {
			t.Errorf("Kind = %q, want %q", got.Kind, StreamErrorLimitReached)
		}
	})

	t.Run("DNS failure names the host", func(t *testing.T) {
		err := &net.DNSError{Err: "no such host", Name: "cf.bek252.xyz", IsNotFound: true}
		got := ClassifyStreamError(err)
		if got.Kind != StreamErrorUnavailable {
			t.Errorf("Kind = %q, want %q", got.Kind, StreamErrorUnavailable)
		}
		if got.Headline != "Provider Not Found" {
			t.Errorf("Headline = %q", got.Headline)
		}
		if !strings.Contains(got.Detail, "cf.bek252.xyz") {
			t.Errorf("Detail %q does not name the host", got.Detail)
		}
	})

	t.Run("context cancellation is not a failure", func(t *testing.T) {
		got := ClassifyStreamError(context.Canceled)
		if got.Kind != StreamErrorEnded {
			t.Errorf("Kind = %q, want %q", got.Kind, StreamErrorEnded)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		got := ClassifyStreamError(errors.New("dial tcp 1.2.3.4:80: connect: connection refused"))
		if got.Headline != "Connection Refused" {
			t.Errorf("Headline = %q", got.Headline)
		}
	})

	t.Run("unknown error still yields a showable slate", func(t *testing.T) {
		got := ClassifyStreamError(errors.New("something bizarre"))
		if got.Headline == "" || got.Detail == "" {
			t.Errorf("unknown error produced an unrenderable slate: %+v", got)
		}
	})

	t.Run("unwrap exposes the cause", func(t *testing.T) {
		cause := errors.New("root cause")
		got := ClassifyStreamError(fmt.Errorf("outer: %w", cause))
		if !errors.Is(got, cause) {
			t.Errorf("errors.Is could not reach the cause")
		}
	})
}

func TestStreamErrorKindPlaceholderType(t *testing.T) {
	tests := map[StreamErrorKind]PlaceholderType{
		StreamErrorLimitReached: PlaceholderLimitReached,
		StreamErrorEnded:        PlaceholderEnded,
		StreamErrorStarting:     PlaceholderStarting,
		StreamErrorUnavailable:  PlaceholderUnavailable,
		StreamErrorKind("nope"): PlaceholderUnavailable,
	}
	for kind, want := range tests {
		if got := kind.PlaceholderType(); got != want {
			t.Errorf("%q.PlaceholderType() = %q, want %q", kind, got, want)
		}
	}
}

func TestStreamErrorCacheKeyDistinguishesMessages(t *testing.T) {
	a := NewUpstreamStatusError(555)
	b := NewUpstreamStatusError(999)
	if a.CacheKey() == b.CacheKey() {
		t.Error("555 and 999 share a cache key, so one would render the other's text")
	}
	if a.CacheKey() != NewUpstreamStatusError(555).CacheKey() {
		t.Error("identical errors produced different cache keys, defeating the cache")
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  int // expected line count
	}{
		{"empty", "", 20, 0},
		{"short stays on one line", "hello world", 20, 1},
		{"long wraps", "the quick brown fox jumps over the lazy dog", 15, 3},
		{"word longer than width is not lost", "supercalifragilistic", 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.in, tt.width)
			if len(got) != tt.want {
				t.Errorf("wrapText(%q, %d) = %d lines %q, want %d", tt.in, tt.width, len(got), got, tt.want)
			}
			// No content may be dropped by wrapping.
			if strings.Join(strings.Fields(strings.Join(got, " ")), " ") !=
				strings.Join(strings.Fields(tt.in), " ") {
				t.Errorf("wrapText lost or reordered content: %q -> %q", tt.in, got)
			}
		})
	}
}

func TestRGBAFrameBytesHandlesStridePadding(t *testing.T) {
	// A sub-image has a stride wider than its row. Feeding img.Pix directly to
	// the rawvideo demuxer would skew every row of the slate.
	base := image.NewRGBA(image.Rect(0, 0, 16, 4))
	sub := base.SubImage(image.Rect(0, 0, 8, 4)).(*image.RGBA)

	got := rgbaFrameBytes(sub)
	want := 8 * 4 * 4
	if len(got) != want {
		t.Errorf("len = %d, want %d (stride padding not stripped)", len(got), want)
	}
}

func TestRenderProducesReadableSlate(t *testing.T) {
	g := NewErrorSlateGenerator(DefaultErrorSlateConfig(), nil)
	se := NewUpstreamStatusError(555)

	img := g.render(se, SlateSize{})

	bounds := img.Bounds()
	if bounds.Dx() != 1280 || bounds.Dy() != 720 {
		t.Fatalf("slate is %dx%d, want 1280x720", bounds.Dx(), bounds.Dy())
	}

	// Text must actually be drawn: count pixels that differ from the background.
	var lit int
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			if img.RGBAAt(x, y) != slateBackground {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Fatal("slate is entirely background - no text or rule was drawn")
	}
	// Guard against a runaway fill that would render a solid block.
	if lit > bounds.Dx()*bounds.Dy()/2 {
		t.Errorf("slate is %d/%d pixels lit, which is not text", lit, bounds.Dx()*bounds.Dy())
	}
}

func TestNewErrorSlateGeneratorAppliesDefaults(t *testing.T) {
	g := NewErrorSlateGenerator(ErrorSlateConfig{}, nil)
	d := DefaultErrorSlateConfig()

	if g.config.Width != d.Width || g.config.Height != d.Height {
		t.Errorf("size = %dx%d, want %dx%d", g.config.Width, g.config.Height, d.Width, d.Height)
	}
	if g.config.FFmpegPath == "" || g.config.Scale <= 0 || g.config.FrameRate <= 0 {
		t.Errorf("zero-value config not defaulted: %+v", g.config)
	}
}

func TestSlateRejectsVariantWithNoEncoder(t *testing.T) {
	g := NewErrorSlateGenerator(DefaultErrorSlateConfig(), nil)

	// The shipped FFmpeg build has neither libaom-av1 nor libsvtav1, so an AV1
	// client must fall back to the embedded static placeholder rather than
	// silently receive nothing.
	_, err := g.Slate(context.Background(), NewCodecVariant("av1", "opus"), NewUpstreamStatusError(555), SlateSize{})
	if !errors.Is(err, ErrGeneratorNoEncoder) {
		t.Errorf("err = %v, want ErrGeneratorNoEncoder", err)
	}
}

func TestSlateRejectsNilError(t *testing.T) {
	g := NewErrorSlateGenerator(DefaultErrorSlateConfig(), nil)
	if _, err := g.Slate(context.Background(), NewCodecVariant("h264", "aac"), nil, SlateSize{}); err == nil {
		t.Error("expected an error for a nil StreamError")
	}
}

func TestInjectErrorSlateKeepsTimestampsMonotonic(t *testing.T) {
	variant := newTestESVariant(t)

	slate := &CachedPlaceholder{
		VideoSamples: []ESSample{
			{PTS: 0, DTS: 0, Data: []byte{0x01}, IsKeyframe: true},
			{PTS: 3600, DTS: 3600, Data: []byte{0x02}},
		},
		AudioSamples: []ESSample{{PTS: 0, DTS: 0, Data: []byte{0x03}}},
		Duration:     time.Second,
	}

	// Start from a non-zero PTS, as happens when a live stream fails partway in.
	const startPTS = int64(1_000_000)

	next := InjectErrorSlate(variant, slate, startPTS, 3*time.Second)

	if next <= startPTS {
		t.Fatalf("next PTS %d did not advance past start %d", next, startPTS)
	}

	samples := variant.VideoTrack().ReadFrom(0, 1000)
	if len(samples) == 0 {
		t.Fatal("no video samples were injected")
	}

	var prev int64 = -1
	for i, s := range samples {
		if s.PTS < startPTS {
			t.Errorf("sample %d has PTS %d, before the start PTS %d - clients treat this as a discontinuity", i, s.PTS, startPTS)
		}
		if s.PTS < prev {
			t.Errorf("sample %d PTS %d went backwards from %d", i, s.PTS, prev)
		}
		prev = s.PTS
	}
}

func TestInjectErrorSlateHandlesNilInputs(t *testing.T) {
	variant := newTestESVariant(t)

	if got := InjectErrorSlate(nil, nil, 42, time.Second); got != 42 {
		t.Errorf("nil variant: got %d, want 42", got)
	}
	if got := InjectErrorSlate(variant, nil, 42, time.Second); got != 42 {
		t.Errorf("nil slate: got %d, want 42", got)
	}
	empty := &CachedPlaceholder{Duration: time.Second}
	if got := InjectErrorSlate(variant, empty, 42, time.Second); got != 42 {
		t.Errorf("empty slate: got %d, want 42", got)
	}
}

func TestInjectErrorSlateDoesNotOverwriteLiveInitData(t *testing.T) {
	variant := newTestESVariant(t)

	live := []byte{0xde, 0xad, 0xbe, 0xef}
	variant.VideoTrack().SetInitData(live)

	slate := &CachedPlaceholder{
		VideoSamples:  []ESSample{{PTS: 0, Data: []byte{0x01}, IsKeyframe: true}},
		VideoInitData: []byte{0x00, 0x11},
		Duration:      time.Second,
	}
	InjectErrorSlate(variant, slate, 0, time.Second)

	got := variant.VideoTrack().GetInitData()
	if string(got) != string(live) {
		// Replacing SPS/PPS mid-stream reconfigures the decoder under a client
		// that is already decoding, which is exactly the failure this avoids.
		t.Errorf("init data = %v, want the live data %v", got, live)
	}
}

// TestSlateEndToEnd encodes a real slate when ffmpeg is present, covering the
// rasterise -> encode -> demux path that unit tests cannot reach.
func TestSlateEndToEnd(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}

	cfg := DefaultErrorSlateConfig()
	cfg.FFmpegPath = path
	// Keep the encode small so this stays a fast test.
	cfg.Width, cfg.Height = 320, 180
	cfg.Duration = 0.5
	cfg.FrameRate = 5
	cfg.Scale = 1

	g := NewErrorSlateGenerator(cfg, nil)
	variant := NewCodecVariant("h264", "aac")
	se := NewUpstreamStatusError(555)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slate, err := g.Slate(ctx, variant, se, SlateSize{})
	if err != nil {
		t.Fatalf("Slate() failed: %v", err)
	}
	if len(slate.VideoSamples) == 0 {
		t.Error("no video samples were produced")
	}

	// A second call must hit the cache rather than re-encoding.
	start := time.Now()
	again, err := g.Slate(ctx, variant, se, SlateSize{})
	if err != nil {
		t.Fatalf("cached Slate() failed: %v", err)
	}
	if again != slate {
		t.Error("second call re-encoded instead of using the cache")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("cached lookup took %v, cache is not working", elapsed)
	}
}

// TestPublishForClientsReleasesWaiters is the regression test for a slate that
// was rendered, buffered, and then never delivered.
//
// The session only signalled readiness on the successful pipeline path, so a
// client blocked in WaitReady until its own timeout: mpv waited 60 seconds and
// received a 503, while the "Too Many Connections" picture it should have shown
// sat complete in the buffer.
func TestPublishForClientsReleasesWaiters(t *testing.T) {
	s := &RelaySession{readyCh: make(chan struct{})}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	if s.IsReady() {
		t.Fatal("a fresh session must not report ready")
	}

	// A client arriving before the pipeline publishes must block.
	blocked := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		blocked <- s.WaitReady(ctx)
	}()

	select {
	case err := <-blocked:
		t.Fatalf("WaitReady returned %v before the pipeline was published", err)
	case <-time.After(50 * time.Millisecond):
	}

	s.publishForClients(NewCodecVariant("h264", "aac"), 4, 10, 5)

	select {
	case err := <-blocked:
		if err != nil {
			t.Fatalf("WaitReady returned %v after publishing", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client stayed blocked after the pipeline was published")
	}

	if !s.IsReady() {
		t.Error("session does not report ready after publishing")
	}
	// The output path needs both of these, or it fails right after WaitReady.
	if s.processorConfig == nil {
		t.Error("processorConfig not set; on-demand processor creation will fail")
	}
	if s.formatRouter == nil {
		t.Error("formatRouter not set; the client's format cannot be routed")
	}
}

// TestPublishForClientsIsIdempotent covers recovery: the slate publishes, then
// the origin returns and the normal pipeline publishes again on the same
// session. markReady closes a channel, so a second call must not panic.
func TestPublishForClientsIsIdempotent(t *testing.T) {
	s := &RelaySession{readyCh: make(chan struct{})}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	s.publishForClients(NewCodecVariant("h264", "aac"), 4, 10, 5)
	s.publishForClients(NewCodecVariant("h264", "aac"), 4, 10, 5)

	if !s.IsReady() {
		t.Error("session not ready after repeated publishing")
	}
}
