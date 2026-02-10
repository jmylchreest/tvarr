package service

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a color
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func createTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func createTestGIF(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.White, color.Black})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return buf.Bytes()
}

func TestImageConverter_ConvertToPNG_FromPNG(t *testing.T) {
	converter := NewImageConverter()
	input := createTestPNG(t, 100, 50)

	data, w, h, err := converter.ConvertToPNG(input)
	require.NoError(t, err)
	assert.Equal(t, 100, w)
	assert.Equal(t, 50, h)
	assert.NotEmpty(t, data)

	// Verify output is valid PNG
	_, err = png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)
}

func TestImageConverter_ConvertToPNG_FromJPEG(t *testing.T) {
	converter := NewImageConverter()
	input := createTestJPEG(t, 200, 100)

	data, w, h, err := converter.ConvertToPNG(input)
	require.NoError(t, err)
	assert.Equal(t, 200, w)
	assert.Equal(t, 100, h)
	assert.NotEmpty(t, data)

	// Verify output is valid PNG
	_, err = png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)
}

func TestImageConverter_ConvertToPNG_FromGIF(t *testing.T) {
	converter := NewImageConverter()
	input := createTestGIF(t, 64, 32)

	data, w, h, err := converter.ConvertToPNG(input)
	require.NoError(t, err)
	assert.Equal(t, 64, w)
	assert.Equal(t, 32, h)
	assert.NotEmpty(t, data)

	// Verify output is valid PNG
	_, err = png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)
}

func TestImageConverter_ConvertToPNG_InvalidData(t *testing.T) {
	converter := NewImageConverter()

	_, _, _, err := converter.ConvertToPNG([]byte("not an image"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoding image")
}

func TestImageConverter_ConvertToPNG_EmptyData(t *testing.T) {
	converter := NewImageConverter()

	_, _, _, err := converter.ConvertToPNG([]byte{})
	assert.Error(t, err)
}

func TestImageConverter_ConvertToPNGReader(t *testing.T) {
	converter := NewImageConverter()
	input := createTestPNG(t, 80, 60)

	data, w, h, err := converter.ConvertToPNGReader(bytes.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, 80, w)
	assert.Equal(t, 60, h)
	assert.NotEmpty(t, data)
}

func TestImageConverter_ConvertToPNGReader_FromJPEG(t *testing.T) {
	converter := NewImageConverter()
	input := createTestJPEG(t, 120, 90)

	data, w, h, err := converter.ConvertToPNGReader(bytes.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, 120, w)
	assert.Equal(t, 90, h)
	assert.NotEmpty(t, data)
}

func TestImageConverter_GetImageDimensions_PNG(t *testing.T) {
	converter := NewImageConverter()
	input := createTestPNG(t, 150, 75)

	w, h, err := converter.GetImageDimensions(input)
	require.NoError(t, err)
	assert.Equal(t, 150, w)
	assert.Equal(t, 75, h)
}

func TestImageConverter_GetImageDimensions_JPEG(t *testing.T) {
	converter := NewImageConverter()
	input := createTestJPEG(t, 300, 200)

	w, h, err := converter.GetImageDimensions(input)
	require.NoError(t, err)
	assert.Equal(t, 300, w)
	assert.Equal(t, 200, h)
}

func TestImageConverter_GetImageDimensions_GIF(t *testing.T) {
	converter := NewImageConverter()
	input := createTestGIF(t, 48, 48)

	w, h, err := converter.GetImageDimensions(input)
	require.NoError(t, err)
	assert.Equal(t, 48, w)
	assert.Equal(t, 48, h)
}

func TestImageConverter_GetImageDimensions_InvalidData(t *testing.T) {
	converter := NewImageConverter()

	_, _, err := converter.GetImageDimensions([]byte("not an image"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoding image config")
}

func TestImageConverter_IsSupportedFormat(t *testing.T) {
	converter := NewImageConverter()

	tests := []struct {
		contentType string
		expected    bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"image/jpg", true},
		{"image/gif", true},
		{"image/webp", true},
		{"image/svg+xml", false},
		{"application/pdf", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := converter.IsSupportedFormat(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestImageConverter_IsSVG(t *testing.T) {
	converter := NewImageConverter()

	assert.True(t, converter.IsSVG("image/svg+xml"))
	assert.False(t, converter.IsSVG("image/png"))
	assert.False(t, converter.IsSVG("image/jpeg"))
	assert.False(t, converter.IsSVG(""))
}

func TestImageConverter_ConvertToPNG_PreservesDimensions(t *testing.T) {
	converter := NewImageConverter()

	sizes := []struct {
		width, height int
	}{
		{1, 1},
		{10, 10},
		{640, 480},
		{1920, 1080},
	}

	for _, size := range sizes {
		input := createTestPNG(t, size.width, size.height)

		_, w, h, err := converter.ConvertToPNG(input)
		require.NoError(t, err)
		assert.Equal(t, size.width, w)
		assert.Equal(t, size.height, h)
	}
}
