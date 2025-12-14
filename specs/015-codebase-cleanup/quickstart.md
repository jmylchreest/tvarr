# Quickstart: Codebase Cleanup and Refactoring

**Feature**: 015-codebase-cleanup
**Date**: 2025-12-14

## Prerequisites

- Go 1.25.x installed
- staticcheck installed: `go install honnef.co/go/tools/cmd/staticcheck@latest`
- Access to `github.com/jmylchreest/mediacommon` repository (for dependency migration)

## Verification Commands

### 1. Check Current staticcheck Status

```bash
# Run staticcheck to see current issues
staticcheck ./...
```

Expected: Multiple U1000 (unused code) and SA1019 (deprecated) warnings.

### 2. Verify Build Works

```bash
# Clean build from scratch
go build ./...
```

### 3. Verify Tests Pass

```bash
# Run all tests
go test ./...
```

### 4. Check for Large Files

```bash
# List Go files over 1000 lines (excluding tests)
find . -name "*.go" -not -name "*_test.go" -exec wc -l {} + | sort -rn | head -20
```

### 5. Check for Local Replace Directives

```bash
# Should show the current local replace directive
grep -E "replace.*=>" go.mod
```

## Post-Implementation Verification

### 1. Sensitive Data Redaction

```bash
# Start the server with debug logging
TVARR_LOGGING_LEVEL=debug task run:dev

# In another terminal, create a test source with password
curl -X POST http://localhost:8080/api/v1/sources/stream \
  -H "Content-Type: application/json" \
  -d '{"name":"test","type":"xtream","url":"http://example.com","username":"user","password":"secret123"}'

# Check logs - password should appear as [REDACTED]
# grep for "secret123" in logs - should find nothing
```

### 2. Build Without Local Dependencies

```bash
# Remove go.sum and vendor if present
rm -f go.sum
rm -rf vendor

# Download dependencies fresh
go mod download

# Build should succeed
go build ./...
```

### 3. Zero staticcheck Issues

```bash
# Should return no output
staticcheck ./...
```

### 4. File Size Check

```bash
# List files over threshold (should be fewer than before)
find . -name "*.go" -not -name "*_test.go" -exec wc -l {} + | awk '$1 > 1000' | sort -rn
```

## Rollback Procedure

If issues are encountered:

1. **Dependency issues**: Restore local replace directive in go.mod
2. **Logger issues**: Remove masq integration, revert to standard handler
3. **Dead code removal broke something**: Use git to restore specific functions

```bash
# Restore a specific file
git checkout HEAD~1 -- path/to/file.go
```

## Success Criteria Checklist

- [ ] SC-001: Zero instances of plaintext passwords in logs
- [ ] SC-002: `go build ./...` succeeds from fresh clone
- [ ] SC-003: `staticcheck ./...` reports zero issues
- [ ] SC-004: No U1000 (unused code) warnings
- [ ] SC-005: No files over 1000 lines (excluding accepted exceptions)
- [ ] SC-006: All 19 password-referencing files reviewed
- [ ] SC-007: No local filesystem replace directives in go.mod
