# Data Model: Codebase Cleanup and Refactoring

**Feature**: 015-codebase-cleanup
**Date**: 2025-12-14

## Summary

This feature does not introduce any new data models or modify existing entity schemas. It focuses on code quality improvements and sensitive data handling in logs.

## Existing Entities (Reference)

The following entities contain sensitive fields that require log redaction:

### StreamSource

```go
type StreamSource struct {
    // ... other fields ...
    Password string `gorm:"size:255" json:"password,omitempty"` // SENSITIVE - redact in logs
}
```

### EpgSource

```go
type EpgSource struct {
    // ... other fields ...
    Password string `gorm:"size:255" json:"password,omitempty"` // SENSITIVE - redact in logs
}
```

## Sensitive Field Registry

For logging purposes, the following field names are considered sensitive and will be redacted:

| Field Name | Case Variants | Entity |
|------------|---------------|--------|
| password | `password`, `Password` | StreamSource, EpgSource |
| secret | `secret`, `Secret` | Generic |
| token | `token`, `Token` | Generic |
| apikey | `apikey`, `ApiKey`, `api_key` | Generic |
| credential | `credential`, `Credential` | Generic |

## No Schema Changes

- No database migrations required
- No new tables or columns
- No relationship changes
- No index modifications

## Configuration Additions

The logger configuration will be extended to include redaction settings. This is a runtime configuration, not a data model change.
