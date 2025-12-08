package relay

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a simple MP4 box
func makeBox(boxType string, content []byte) []byte {
	size := uint32(8 + len(content))
	box := make([]byte, size)
	binary.BigEndian.PutUint32(box[0:4], size)
	copy(box[4:8], boxType)
	copy(box[8:], content)
	return box
}

// Helper to create an extended size box
func makeExtendedBox(boxType string, content []byte) []byte {
	size := uint64(16 + len(content))
	box := make([]byte, size)
	binary.BigEndian.PutUint32(box[0:4], 1) // Extended size marker
	copy(box[4:8], boxType)
	binary.BigEndian.PutUint64(box[8:16], size)
	copy(box[16:], content)
	return box
}

// Helper to create an mfhd box with sequence number
func makeMfhd(seqNum uint32) []byte {
	content := make([]byte, 8) // version(1) + flags(3) + sequence_number(4)
	binary.BigEndian.PutUint32(content[4:8], seqNum)
	return makeBox("mfhd", content)
}

// Helper to create a simple tfdt box
func makeTfdt(decodeTime uint64) []byte {
	content := make([]byte, 12) // version(1=64bit) + flags(3) + decode_time(8)
	content[0] = 1              // version 1 for 64-bit time
	binary.BigEndian.PutUint64(content[4:12], decodeTime)
	return makeBox("tfdt", content)
}

// Helper to create a simple trun box
func makeTrun(sampleCount uint32) []byte {
	content := make([]byte, 8) // version(1) + flags(3) + sample_count(4)
	binary.BigEndian.PutUint32(content[4:8], sampleCount)
	return makeBox("trun", content)
}

// Helper to create a traf box
func makeTraf(tfdt, trun []byte) []byte {
	content := append(tfdt, trun...)
	return makeBox("traf", content)
}

// Helper to create a moof box
func makeMoof(seqNum uint32, decodeTime uint64, sampleCount uint32) []byte {
	mfhd := makeMfhd(seqNum)
	tfdt := makeTfdt(decodeTime)
	trun := makeTrun(sampleCount)
	traf := makeTraf(tfdt, trun)
	content := append(mfhd, traf...)
	return makeBox("moof", content)
}

// Helper to create mdat box
func makeMdat(data []byte) []byte {
	return makeBox("mdat", data)
}

// Helper to create a simple mvhd box
func makeMvhd(timescale uint32) []byte {
	content := make([]byte, 100) // Simplified mvhd
	// version(1) + flags(3) + create_time(4) + mod_time(4) + timescale(4)
	binary.BigEndian.PutUint32(content[12:16], timescale)
	return makeBox("mvhd", content)
}

// Helper to create a simple hdlr box
func makeHdlr(handlerType string) []byte {
	content := make([]byte, 24)
	// version(1) + flags(3) + pre_defined(4) + handler_type(4) + reserved(12)
	copy(content[8:12], handlerType)
	return makeBox("hdlr", content)
}

// Helper to create a mdia box with hdlr
func makeMdia(handlerType string) []byte {
	hdlr := makeHdlr(handlerType)
	return makeBox("mdia", hdlr)
}

// Helper to create a trak box
func makeTrak(handlerType string) []byte {
	mdia := makeMdia(handlerType)
	return makeBox("trak", mdia)
}

// Helper to create a moov box
func makeMoov(timescale uint32, hasVideo, hasAudio bool) []byte {
	mvhd := makeMvhd(timescale)
	content := mvhd

	if hasVideo {
		content = append(content, makeTrak("vide")...)
	}
	if hasAudio {
		content = append(content, makeTrak("soun")...)
	}

	return makeBox("moov", content)
}

// Helper to create an ftyp box
func makeFtyp() []byte {
	content := make([]byte, 8)
	copy(content[0:4], "isom")
	return makeBox("ftyp", content)
}

func TestPeekBoxHeader(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantSize   uint64
		wantType   string
		wantErr    bool
		wantExtend bool
	}{
		{
			name:     "standard box",
			data:     makeBox("test", []byte{1, 2, 3, 4}),
			wantSize: 12,
			wantType: "test",
		},
		{
			name:       "extended size box",
			data:       makeExtendedBox("tst2", []byte{1, 2}),
			wantSize:   18,
			wantType:   "tst2",
			wantExtend: true,
		},
		{
			name:    "too short",
			data:    []byte{0, 0, 0, 8},
			wantErr: true,
		},
		{
			name:    "empty",
			data:    []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, err := peekBoxHeader(tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSize, header.Size)
			assert.Equal(t, tt.wantType, header.Type)
			assert.Equal(t, tt.wantExtend, header.Extended)
		})
	}
}

func TestCMAFMuxer_NewMuxer(t *testing.T) {
	config := DefaultCMAFMuxerConfig()
	muxer := NewCMAFMuxer(config)

	assert.NotNil(t, muxer)
	assert.Equal(t, 10, muxer.maxFragments)
	assert.True(t, muxer.expectingInit)
	assert.Empty(t, muxer.fragments)
}

func TestCMAFMuxer_ParseInitSegment(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// Create init segment (ftyp + moov)
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, true)
	initData := append(ftyp, moov...)

	n, err := muxer.Write(initData)
	require.NoError(t, err)
	assert.Equal(t, len(initData), n)

	// Check init segment was parsed
	assert.True(t, muxer.HasInitSegment())

	init := muxer.GetInitSegment()
	require.NotNil(t, init)
	assert.True(t, init.HasVideo)
	assert.True(t, init.HasAudio)
	assert.Equal(t, uint32(90000), init.Timescale)
	assert.Equal(t, initData, init.Data)
}

func TestCMAFMuxer_ParseFragment(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// First write init segment
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	initData := append(ftyp, moov...)
	_, err := muxer.Write(initData)
	require.NoError(t, err)

	// Now write a fragment (moof + mdat)
	moof := makeMoof(1, 0, 30)
	mdat := makeMdat([]byte("video frame data"))
	fragData := append(moof, mdat...)

	_, err = muxer.Write(fragData)
	require.NoError(t, err)

	// Check fragment was parsed
	assert.Equal(t, 1, muxer.FragmentCount())

	frag := muxer.GetLatestFragment()
	require.NotNil(t, frag)
	assert.Equal(t, uint32(1), frag.SequenceNumber)
	assert.Equal(t, fragData, frag.Data)
}

func TestCMAFMuxer_MultipleFragments(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// Write init segment
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	_, err := muxer.Write(append(ftyp, moov...))
	require.NoError(t, err)

	// Write multiple fragments
	for i := uint32(1); i <= 5; i++ {
		moof := makeMoof(i, uint64(i)*3000, 30)
		mdat := makeMdat([]byte("frame data"))
		_, err := muxer.Write(append(moof, mdat...))
		require.NoError(t, err)
	}

	assert.Equal(t, 5, muxer.FragmentCount())

	// Check all fragments
	fragments := muxer.GetFragments()
	assert.Len(t, fragments, 5)

	for i, frag := range fragments {
		assert.Equal(t, uint32(i+1), frag.SequenceNumber)
	}

	// Get specific fragment
	frag := muxer.GetFragment(3)
	require.NotNil(t, frag)
	assert.Equal(t, uint32(3), frag.SequenceNumber)
}

func TestCMAFMuxer_FragmentLimit(t *testing.T) {
	config := CMAFMuxerConfig{MaxFragments: 3}
	muxer := NewCMAFMuxer(config)

	// Write init segment
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	_, err := muxer.Write(append(ftyp, moov...))
	require.NoError(t, err)

	// Write more fragments than the limit
	for i := uint32(1); i <= 10; i++ {
		moof := makeMoof(i, uint64(i)*3000, 30)
		mdat := makeMdat([]byte("frame"))
		_, err := muxer.Write(append(moof, mdat...))
		require.NoError(t, err)
	}

	// Should only have last 3 fragments
	assert.Equal(t, 3, muxer.FragmentCount())

	fragments := muxer.GetFragments()
	assert.Equal(t, uint32(8), fragments[0].SequenceNumber)
	assert.Equal(t, uint32(9), fragments[1].SequenceNumber)
	assert.Equal(t, uint32(10), fragments[2].SequenceNumber)
}

func TestCMAFMuxer_IncrementalWrite(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// Create complete fragment
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	moof := makeMoof(1, 0, 30)
	mdat := makeMdat([]byte("video data"))
	fullData := append(append(append(ftyp, moov...), moof...), mdat...)

	// Write in small chunks to simulate streaming
	chunkSize := 10
	for i := 0; i < len(fullData); i += chunkSize {
		end := i + chunkSize
		if end > len(fullData) {
			end = len(fullData)
		}
		_, err := muxer.Write(fullData[i:end])
		require.NoError(t, err)
	}

	// Check everything was parsed
	assert.True(t, muxer.HasInitSegment())
	assert.Equal(t, 1, muxer.FragmentCount())
}

func TestCMAFMuxer_Reset(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// Write data
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	_, err := muxer.Write(append(ftyp, moov...))
	require.NoError(t, err)

	moof := makeMoof(1, 0, 30)
	mdat := makeMdat([]byte("data"))
	_, err = muxer.Write(append(moof, mdat...))
	require.NoError(t, err)

	assert.True(t, muxer.HasInitSegment())
	assert.Equal(t, 1, muxer.FragmentCount())

	// Reset
	muxer.Reset()

	assert.False(t, muxer.HasInitSegment())
	assert.Equal(t, 0, muxer.FragmentCount())
	assert.Nil(t, muxer.GetInitSegment())
	assert.Nil(t, muxer.GetLatestFragment())
}

func TestCMAFMuxer_ExtractSequenceNumber(t *testing.T) {
	moof := makeMoof(42, 12345, 30)

	seqNum, err := extractSequenceNumber(moof)
	require.NoError(t, err)
	assert.Equal(t, uint32(42), seqNum)
}

func TestCMAFMuxer_ExtractTiming(t *testing.T) {
	moof := makeMoof(1, 90000, 30)

	decodeTime, duration, err := extractTiming(moof)
	require.NoError(t, err)
	assert.Equal(t, uint64(90000), decodeTime)
	assert.Equal(t, uint64(30), duration) // Sample count as simplified duration
}

func TestCMAFMuxer_VideoAudioDetection(t *testing.T) {
	tests := []struct {
		name     string
		hasVideo bool
		hasAudio bool
	}{
		{"video only", true, false},
		{"audio only", false, true},
		{"both", true, true},
		{"neither", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

			ftyp := makeFtyp()
			moov := makeMoov(90000, tt.hasVideo, tt.hasAudio)
			_, err := muxer.Write(append(ftyp, moov...))
			require.NoError(t, err)

			init := muxer.GetInitSegment()
			require.NotNil(t, init)
			assert.Equal(t, tt.hasVideo, init.HasVideo)
			assert.Equal(t, tt.hasAudio, init.HasAudio)
		})
	}
}

func TestCMAFMuxer_GetFragmentNotFound(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	// Write init
	ftyp := makeFtyp()
	moov := makeMoov(90000, true, false)
	_, _ = muxer.Write(append(ftyp, moov...))

	// Write one fragment
	moof := makeMoof(5, 0, 30)
	mdat := makeMdat([]byte("data"))
	_, _ = muxer.Write(append(moof, mdat...))

	// Try to get non-existent fragment
	frag := muxer.GetFragment(999)
	assert.Nil(t, frag)
}

func TestCMAFMuxer_EmptyInput(t *testing.T) {
	muxer := NewCMAFMuxer(DefaultCMAFMuxerConfig())

	n, err := muxer.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, muxer.HasInitSegment())
}

func TestDefaultCMAFMuxerConfig(t *testing.T) {
	config := DefaultCMAFMuxerConfig()
	assert.Equal(t, 10, config.MaxFragments)
}

func TestIsVideoTrack(t *testing.T) {
	videoTrak := makeTrak("vide")
	audioTrak := makeTrak("soun")
	emptyTrak := makeBox("trak", []byte{})

	assert.True(t, isVideoTrack(videoTrak))
	assert.False(t, isVideoTrack(audioTrak))
	assert.False(t, isVideoTrack(emptyTrak))
}

func TestIsAudioTrack(t *testing.T) {
	videoTrak := makeTrak("vide")
	audioTrak := makeTrak("soun")
	emptyTrak := makeBox("trak", []byte{})

	assert.False(t, isAudioTrack(videoTrak))
	assert.True(t, isAudioTrack(audioTrak))
	assert.False(t, isAudioTrack(emptyTrak))
}
