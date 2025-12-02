// Package service provides business logic layer for tvarr operations.
package service

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	// Register image format decoders
	_ "image/gif"
	_ "image/jpeg"

	// WebP support from x/image
	_ "golang.org/x/image/webp"
)

// ImageConverter handles image format conversion.
type ImageConverter struct{}

// NewImageConverter creates a new ImageConverter.
func NewImageConverter() *ImageConverter {
	return &ImageConverter{}
}

// ConvertToPNG converts image data to PNG format.
// Returns the PNG data, width, height, and any error.
// If the input is already PNG, it decodes and re-encodes to ensure validity.
func (c *ImageConverter) ConvertToPNG(data []byte) ([]byte, int, int, error) {
	// Decode the image (supports PNG, JPEG, GIF, WebP via registered decoders)
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decoding image (format=%s): %w", format, err)
	}

	// Get dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("encoding to PNG: %w", err)
	}

	return buf.Bytes(), width, height, nil
}

// ConvertToPNGReader converts image data from a reader to PNG format.
// Returns the PNG data, width, height, and any error.
func (c *ImageConverter) ConvertToPNGReader(r io.Reader) ([]byte, int, int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("reading image data: %w", err)
	}
	return c.ConvertToPNG(data)
}

// GetImageDimensions returns the width and height of an image without full conversion.
func (c *ImageConverter) GetImageDimensions(data []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("decoding image config: %w", err)
	}
	return config.Width, config.Height, nil
}

// IsSupportedFormat checks if the content type is a supported image format.
func (c *ImageConverter) IsSupportedFormat(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// IsSVG checks if the content type indicates an SVG image.
// SVGs are not converted to PNG as they are vector graphics.
func (c *ImageConverter) IsSVG(contentType string) bool {
	return contentType == "image/svg+xml"
}
