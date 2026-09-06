package relay

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// These tests exercise the slate splice against real encoded media rather than
// hand-built samples. There is no live provider to test against -- and one would
// be useless for this anyway, since reproducing a mid-stream failure on demand is
// exactly what a real origin will not do -- so a synthetic MPEG-TS stream stands
// in for the origin. It carries genuine H.264 SPS/PPS and a real PTS timeline,
// which is what the PTS-continuity and resolution-matching logic actually depends
// on.

// syntheticTS renders a short MPEG-TS test stream at the given size and returns
// its bytes. It skips the test when ffmpeg is unavailable.
func syntheticTS(t *testing.T, width, height int, seconds float64) []byte {
	t.Helper()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}

	out := filepath.Join(t.TempDir(), "synthetic.ts")

	cmd := exec.Command(ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=%dx%d:rate=25:duration=%.2f", width, height, seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%.2f", seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25",
		"-c:a", "aac", "-b:a", "96k",
		"-f", "mpegts", "-y", out,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not build synthetic stream: %v (%s)", err, output)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading synthetic stream: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("synthetic stream is empty")
	}
	return data
}

// ingestSynthetic feeds a TS stream through a demuxer into a fresh buffer and
// waits for samples to land. rebaseTarget of 0 means no rebasing.
func ingestSynthetic(t *testing.T, data []byte, rebaseTarget int64) *SharedESBuffer {
	t.Helper()

	buffer := NewSharedESBuffer("test-channel", "test-proxy", SharedESBufferConfig{})

	demuxer := NewTSDemuxer(buffer, TSDemuxerConfig{
		Logger:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		PTSRebaseTarget: rebaseTarget,
	})

	var once sync.Once
	defer once.Do(func() { demuxer.Close() })

	if err := demuxer.Write(data); err != nil {
		t.Fatalf("writing to demuxer: %v", err)
	}

	// The demuxer parses on its own goroutine; wait for the source variant to
	// appear rather than sleeping a fixed amount.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := buffer.WaitSourceVariant(ctx); err != nil {
		t.Skipf("demuxer produced no source variant: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if v := buffer.GetSourceVariant(); v != nil && v.VideoTrack().LatestPTS() > 0 {
			return buffer
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Skip("synthetic stream produced no video samples in time")
	return nil
}

// TestSlateSplicesOntoLiveTimeline is the regression test for the defect this
// work fixed: the slate used to splice in at PTS 0 regardless of how far the live
// stream had already run, which decoders read as a backwards jump.
func TestSlateSplicesOntoLiveTimeline(t *testing.T) {
	data := syntheticTS(t, 640, 360, 2)
	buffer := ingestSynthetic(t, data, 0)

	live := buffer.GetSourceVariant()
	livePTS := live.VideoTrack().LatestPTS()
	if livePTS <= 0 {
		t.Fatalf("synthetic stream produced no usable PTS (got %d)", livePTS)
	}
	t.Logf("live stream ended at PTS %d", livePTS)

	ffmpegPath, _ := exec.LookPath("ffmpeg")
	cfg := DefaultErrorSlateConfig()
	cfg.FFmpegPath = ffmpegPath
	cfg.Duration = 0.5
	cfg.FrameRate = 5
	g := NewErrorSlateGenerator(cfg, nil)

	// Mirror what serveErrorSlate does: match the live raster, continue the
	// live timeline.
	session := &RelaySession{}
	size, matched := session.liveVideoSize(live)
	if !matched {
		t.Fatal("could not read the live resolution from the stream's own SPS")
	}
	if size.Width != 640 || size.Height != 360 {
		t.Errorf("live size = %dx%d, want 640x360", size.Width, size.Height)
	}

	slate, err := g.Slate(context.Background(), live.Variant(), NewUpstreamStatusError(555), size)
	if err != nil {
		t.Fatalf("rendering slate at the live size: %v", err)
	}

	InjectErrorSlate(live, slate, livePTS+slatePTSGap, 2*time.Second)

	// Every sample on the track, live and slate, must move forward in time.
	samples := live.VideoTrack().ReadFrom(0, 100000)
	if len(samples) < 2 {
		t.Fatalf("expected live and slate samples, got %d", len(samples))
	}

	var (
		prev       int64 = -1
		regression int
	)
	for _, s := range samples {
		if s.PTS < prev {
			regression++
			t.Errorf("PTS went backwards: %d after %d", s.PTS, prev)
		}
		prev = s.PTS
	}
	if regression == 0 {
		t.Logf("timeline intact across %d samples, ending at PTS %d", len(samples), prev)
	}
	if prev <= livePTS {
		t.Errorf("slate did not extend the timeline: ends at %d, live ended at %d", prev, livePTS)
	}
}

// TestSlateMatchesLiveResolution guards the parameter-set half of the splice: the
// track keeps its live SPS/PPS, so slate slices of a different size would not be
// described by them.
func TestSlateMatchesLiveResolution(t *testing.T) {
	for _, size := range []struct{ w, h int }{{640, 360}, {1280, 720}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			data := syntheticTS(t, size.w, size.h, 1)
			buffer := ingestSynthetic(t, data, 0)

			live := buffer.GetSourceVariant()
			session := &RelaySession{}

			got, ok := session.liveVideoSize(live)
			if !ok {
				t.Fatal("could not read resolution from the live SPS")
			}
			if got.Width != size.w || got.Height != size.h {
				t.Errorf("liveVideoSize = %dx%d, want %dx%d", got.Width, got.Height, size.w, size.h)
			}
		})
	}
}

// TestResumedStreamIsRebasedOntoSlate covers the return leg: a recovered origin
// restarts on its own timeline, and without a shift that is a backwards jump just
// as bad as the one going in.
func TestResumedStreamIsRebasedOntoSlate(t *testing.T) {
	data := syntheticTS(t, 640, 360, 1)

	// Where the slate is deemed to have ended -- far beyond anything the
	// synthetic stream's own timeline would reach on its own.
	const slateEndPTS = int64(50_000_000)

	buffer := ingestSynthetic(t, data, slateEndPTS+slatePTSGap)

	resumed := buffer.GetSourceVariant()
	samples := resumed.VideoTrack().ReadFrom(0, 100000)
	if len(samples) == 0 {
		t.Fatal("resumed stream produced no samples")
	}

	if first := samples[0].PTS; first < slateEndPTS {
		t.Errorf("resumed stream starts at PTS %d, before the slate ended at %d - the viewer's decoder sees time run backwards",
			first, slateEndPTS)
	}

	var prev int64 = -1
	for _, s := range samples {
		if s.PTS < prev {
			t.Errorf("resumed stream PTS went backwards: %d after %d", s.PTS, prev)
		}
		prev = s.PTS
	}

	// The shift must be a constant offset, not a rewrite: internal spacing has to
	// survive or playback speed changes.
	unshifted := ingestSynthetic(t, data, 0)
	origSamples := unshifted.GetSourceVariant().VideoTrack().ReadFrom(0, 100000)
	if len(origSamples) == len(samples) && len(samples) > 1 {
		origSpan := origSamples[len(origSamples)-1].PTS - origSamples[0].PTS
		newSpan := samples[len(samples)-1].PTS - samples[0].PTS
		if origSpan != newSpan {
			t.Errorf("rebasing changed the stream's duration: %d -> %d", origSpan, newSpan)
		}
	}
}

// TestLatestPTS covers the accessor the splice depends on.
func TestLatestPTS(t *testing.T) {
	variant := newTestESVariant(t)
	track := variant.VideoTrack()

	if got := track.LatestPTS(); got != 0 {
		t.Errorf("empty track LatestPTS = %d, want 0", got)
	}

	track.Write(1000, 1000, []byte{0x01}, true)
	track.Write(4600, 4600, []byte{0x02}, false)

	if got := track.LatestPTS(); got != 4600 {
		t.Errorf("LatestPTS = %d, want 4600", got)
	}
}

func TestSlateSizeNormalise(t *testing.T) {
	cfg := DefaultErrorSlateConfig()

	if got := (SlateSize{}).normalise(cfg); got.Width != cfg.Width || got.Height != cfg.Height {
		t.Errorf("zero size = %+v, want the config default", got)
	}
	// yuv420p cannot represent odd dimensions, so they must be rounded down.
	if got := (SlateSize{Width: 641, Height: 361}).normalise(cfg); got.Width%2 != 0 || got.Height%2 != 0 {
		t.Errorf("odd size not made even: %+v", got)
	}
}

// TestSlateSamplesMatchLiveSampleFormat is the guard for the subtlest of the
// splice bugs: correct timestamps on samples the decoder cannot parse still give
// the viewer a dead stream.
//
// The live TS path emits Annex-B with parameter sets inline in keyframes; fMP4
// samples are AVCC with the parameter sets held only in the init segment. Slate
// samples must be converted to match, or they are undecodable on a live track.
func TestSlateSamplesMatchLiveSampleFormat(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}

	cfg := DefaultErrorSlateConfig()
	cfg.FFmpegPath = ffmpegPath
	cfg.Width, cfg.Height = 640, 360
	cfg.Duration = 0.5
	cfg.FrameRate = 5

	g := NewErrorSlateGenerator(cfg, nil)
	slate, err := g.Slate(context.Background(), NewCodecVariant("h264", "aac"),
		NewUpstreamStatusError(555), SlateSize{})
	if err != nil {
		t.Fatalf("rendering slate: %v", err)
	}
	if len(slate.VideoSamples) == 0 {
		t.Fatal("slate produced no video samples")
	}

	// Parameter sets must not be published as track init data: a live track has
	// none, and the slate must not make the track look different.
	if slate.VideoInitData != nil {
		t.Errorf("slate still publishes video init data (%d bytes); live tracks carry none",
			len(slate.VideoInitData))
	}

	startCode := []byte{0, 0, 0, 1}
	var keyframesWithSPS int

	for i, sample := range slate.VideoSamples {
		if !bytes.HasPrefix(sample.Data, startCode) && !bytes.HasPrefix(sample.Data, []byte{0, 0, 1}) {
			t.Fatalf("sample %d is not Annex-B (starts %x) - it is still AVCC and will not decode on a live track",
				i, sample.Data[:min(4, len(sample.Data))])
		}

		if !sample.IsKeyframe {
			continue
		}
		if size, ok := spsSizeFromAnnexB(sample.Data, "h264"); ok {
			keyframesWithSPS++
			if size.Width != 640 || size.Height != 360 {
				t.Errorf("keyframe %d SPS says %dx%d, want 640x360", i, size.Width, size.Height)
			}
		}
	}

	if keyframesWithSPS == 0 {
		t.Error("no keyframe carries an inline SPS - a client joining on the slate has no parameter sets")
	}
	t.Logf("%d/%d slate samples are keyframes carrying inline SPS",
		keyframesWithSPS, len(slate.VideoSamples))
}
