# tvarr Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-12-05

## Active Technologies
- SQLite/PostgreSQL/MySQL (configurable via GORM) (005-ffmpeg-relay-transcoding)
- Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary) (005-ffmpeg-relay-transcoding)
- Go 1.25.x + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary) (005-ffmpeg-relay-transcoding)
- Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, Viper (config) (006-config-settings-ui)
- SQLite/PostgreSQL/MySQL (configurable via GORM), YAML config files (006-config-settings-ui)
- Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), gohlslib v2, go-astits (008-multi-format-streaming)
- SQLite/PostgreSQL/MySQL (configurable via GORM) - existing relay_profiles table (008-multi-format-streaming)
- Go 1.25.x (backend), TypeScript/Next.js 16.x (frontend) + Huma v2.34+ (API), React 19, shadcn/ui, Tailwind CSS v4 (011-frontend-theme-polish)
- File system for custom themes (`$DATA/themes/`), localStorage for user preferences (011-frontend-theme-polish)
- Dockerfile, Go 1.25.x (for tvarr build), Bash (scripts) + FFmpeg 7.x+, libx264, libx265, libvpx, libaom, libopus, libmp3lame, libva, libvpl, nv-codec-headers, Mesa, UPX (012-docker-ffmpeg-packaging)
- Volume mount at `/data` for SQLite/config persistence (012-docker-ffmpeg-packaging)
- Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), m-mizutani/masq (new) (015-codebase-cleanup)

- Go 1.25.x (latest stable) + Huma v2.34+ (API), Chi (router), GORM v2 (ORM), FFmpeg (external binary) (005-ffmpeg-relay-transcoding)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.25.x (latest stable)

## Code Style

Go 1.25.x (latest stable): Follow standard conventions

## Recent Changes
- 015-codebase-cleanup: Added Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), m-mizutani/masq (new)
- 014-relay-profile-simplify: Added Go 1.25.x (latest stable) + Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), gohlslib v2
- 012-docker-ffmpeg-packaging: Added Dockerfile, Go 1.25.x (for tvarr build), Bash (scripts) + FFmpeg 7.x+, libx264, libx265, libvpx, libaom, libopus, libmp3lame, libva, libvpl, nv-codec-headers, Mesa, UPX


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
