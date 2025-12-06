package handlers

import "github.com/danielgtaylor/huma/v2"

// CORSConfig holds CORS configuration options.
type CORSConfig struct {
	AllowOrigin  string
	AllowMethods string
	AllowHeaders string
	ExposeHeaders string
}

// DefaultCORSConfig returns the default CORS configuration for streaming endpoints.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigin:   "*",
		AllowMethods:  "GET, OPTIONS",
		AllowHeaders:  "Content-Type, Accept, Range",
		ExposeHeaders: "Content-Length, Content-Range",
	}
}

// SetCORSHeaders sets CORS headers on a Huma context for streaming responses.
func SetCORSHeaders(ctx huma.Context, config CORSConfig) {
	ctx.SetHeader("Access-Control-Allow-Origin", config.AllowOrigin)
	ctx.SetHeader("Access-Control-Allow-Methods", config.AllowMethods)
	ctx.SetHeader("Access-Control-Allow-Headers", config.AllowHeaders)
	if config.ExposeHeaders != "" {
		ctx.SetHeader("Access-Control-Expose-Headers", config.ExposeHeaders)
	}
}

// SetDefaultCORSHeaders sets the default CORS headers for streaming endpoints.
func SetDefaultCORSHeaders(ctx huma.Context) {
	SetCORSHeaders(ctx, DefaultCORSConfig())
}
