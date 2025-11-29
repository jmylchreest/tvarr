# Reference Codebase: m3u-proxy

**Status**: READ-ONLY REFERENCE

The original Rust implementation of this project exists at `../m3u-proxy/` relative to the tvarr root directory.

## Location

```
/home/johnm/dtkr4-cnjjf/github.com/jmylchreest/m3u-proxy/
```

## Purpose

This codebase serves as a reference for:
- API design and endpoint structure
- Frontend implementation (Next.js)
- Feature parity validation
- Business logic understanding

## Key Components

### Backend (Rust)
- `crates/` - Rust workspace crates
- `config.toml` - Configuration format reference
- `README-*.md` - Detailed documentation on:
  - EPG Generator Process
  - M3U Generator Process  
  - Pipeline Generation Process
  - Relay functionality

### Frontend (Next.js)
- `frontend/src/app/` - Next.js app router pages
- `frontend/src/components/` - React components (shadcn/ui based)
- `frontend/src/hooks/` - Custom React hooks
- `frontend/src/lib/` - Utility libraries
- `frontend/src/providers/` - React context providers

## Usage Guidelines

1. **DO NOT** modify any files in the m3u-proxy directory
2. **DO** reference for API structure and feature understanding
3. **DO** reference frontend for UI/UX patterns to recreate
4. **DO** use documentation files for understanding complex features

## API Compatibility Goal

The tvarr REST API should maintain compatibility with m3u-proxy where possible to allow:
- Frontend reuse with minimal modifications
- Easier migration path for existing users
- Consistent user experience
