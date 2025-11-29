package core

import (
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/storage"
)

// Config holds pipeline configuration options.
type Config struct {
	// EnableFiltering enables the filtering stage.
	EnableFiltering bool

	// EnableNumbering enables the channel numbering stage.
	EnableNumbering bool

	// EnableLogoCaching enables logo caching (future).
	EnableLogoCaching bool
}

// DefaultConfig returns a Config with default settings.
func DefaultConfig() Config {
	return Config{
		EnableFiltering:   true,
		EnableNumbering:   true,
		EnableLogoCaching: false,
	}
}

// Builder provides a fluent interface for constructing a Factory.
type Builder struct {
	channelRepo    repository.ChannelRepository
	epgProgramRepo repository.EpgProgramRepository
	sandbox        *storage.Sandbox
	logger         *slog.Logger
	config         Config
}

// NewBuilder creates a new pipeline Builder.
func NewBuilder() *Builder {
	return &Builder{
		config: DefaultConfig(),
	}
}

// WithChannelRepository sets the channel repository.
func (b *Builder) WithChannelRepository(repo repository.ChannelRepository) *Builder {
	b.channelRepo = repo
	return b
}

// WithEpgProgramRepository sets the EPG program repository.
func (b *Builder) WithEpgProgramRepository(repo repository.EpgProgramRepository) *Builder {
	b.epgProgramRepo = repo
	return b
}

// WithSandbox sets the storage sandbox.
func (b *Builder) WithSandbox(sandbox *storage.Sandbox) *Builder {
	b.sandbox = sandbox
	return b
}

// WithLogger sets the logger.
func (b *Builder) WithLogger(logger *slog.Logger) *Builder {
	b.logger = logger
	return b
}

// WithConfig sets the pipeline configuration.
func (b *Builder) WithConfig(config Config) *Builder {
	b.config = config
	return b
}

// EnableFiltering enables or disables the filtering stage.
func (b *Builder) EnableFiltering(enabled bool) *Builder {
	b.config.EnableFiltering = enabled
	return b
}

// EnableNumbering enables or disables the numbering stage.
func (b *Builder) EnableNumbering(enabled bool) *Builder {
	b.config.EnableNumbering = enabled
	return b
}

// EnableLogoCaching enables or disables logo caching.
func (b *Builder) EnableLogoCaching(enabled bool) *Builder {
	b.config.EnableLogoCaching = enabled
	return b
}

// Build creates a Factory with the configured settings.
// This does not register stages - use BuildWithStages for that.
func (b *Builder) Build() (*Factory, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	deps := &Dependencies{
		ChannelRepo:    b.channelRepo,
		EpgProgramRepo: b.epgProgramRepo,
		Sandbox:        b.sandbox,
		Logger:         b.logger,
	}

	return NewFactory(deps), nil
}

// validate checks that all required dependencies are set.
func (b *Builder) validate() error {
	if b.channelRepo == nil {
		return NewConfigurationError("channelRepo", "channel repository is required")
	}
	if b.epgProgramRepo == nil {
		return NewConfigurationError("epgProgramRepo", "EPG program repository is required")
	}
	if b.sandbox == nil {
		return NewConfigurationError("sandbox", "storage sandbox is required")
	}
	return nil
}

// Config returns the current configuration.
func (b *Builder) Config() Config {
	return b.config
}
