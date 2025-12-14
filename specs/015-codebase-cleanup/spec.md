# Feature Specification: Codebase Cleanup and Refactoring

**Feature Branch**: `015-codebase-cleanup`
**Created**: 2025-12-14
**Status**: Draft
**Input**: User description: "We've made a lot of changes recently. I want to make sure the project is still well structured, DRY, idiomatic Go 1.25, packages are appropriately split with files not being too large/containing many unrelated things/have suitable names. It's important to me that methods aren't needlessly created, that old/unused (or code that is easily replaced) code is removed so there's no dead code. I also want to make sure we have no sensitive information, we have a password stored in the database, but we need to use the password in API calls in plain text over HTTP so it's not a big deal. I want to use go-masq the slog middleware to redact where that password gets used (source API queries). We also make use of a mediacommon fork which is currently just held locally in a go.mod redirect but it should use the upstream URL github.com/jmylchreest/mediacommon/ (feat/eac3-support branch)."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sensitive Data Protection in Logs (Priority: P1)

As a developer or operator reviewing logs, I want passwords and other sensitive data to be automatically redacted so that log files can be safely shared, stored, and analyzed without exposing credentials.

**Why this priority**: Security is paramount. Exposing credentials in logs is a compliance and security risk. Even if passwords are transmitted in plain text over HTTP (necessary for Xtream API compatibility), they should never appear in application logs.

**Independent Test**: Can be fully tested by triggering a source ingestion with Xtream credentials and verifying that log output shows redacted values (e.g., `password=***REDACTED***`) instead of actual passwords.

**Acceptance Scenarios**:

1. **Given** a stream source with Xtream credentials, **When** an ingestion job runs and logs the API request, **Then** the password field is redacted in all log output
2. **Given** an EPG source with Xtream credentials, **When** an ingestion job runs, **Then** sensitive fields (password, username) are redacted in logs
3. **Given** any log entry containing sensitive data patterns, **When** the log is written, **Then** the sensitive data is replaced with a redaction marker
4. **Given** the log level is set to DEBUG, **When** detailed request logging occurs, **Then** passwords are still redacted even in verbose output

---

### User Story 2 - Upstream Dependency Migration (Priority: P2)

As a developer, I want the project to use proper Go module references to forked dependencies so that the codebase can be built without local filesystem dependencies and CI/CD pipelines work correctly.

**Why this priority**: Build reproducibility and CI/CD compatibility are essential for team collaboration and automated deployments. A local `replace` directive breaks builds for anyone without the local fork.

**Independent Test**: Can be fully tested by removing any local `replace` directives from go.mod, running `go mod tidy`, and successfully building the project with `go build ./...`.

**Acceptance Scenarios**:

1. **Given** the go.mod file, **When** inspecting module dependencies, **Then** the mediacommon dependency references `github.com/jmylchreest/mediacommon/v2` at the `feat/eac3-support` branch
2. **Given** a fresh clone of the repository, **When** running `go build ./...`, **Then** the build succeeds without requiring local filesystem dependencies
3. **Given** the go.mod file, **When** inspecting for `replace` directives, **Then** no local filesystem replace directives exist for external dependencies

---

### User Story 3 - Dead Code Elimination (Priority: P3)

As a maintainer, I want all unused code removed from the codebase so that the project is easier to understand, maintain, and has a smaller footprint.

**Why this priority**: Dead code increases cognitive load, can mislead developers, and may contain outdated security patterns. Removing it improves maintainability.

**Independent Test**: Can be fully tested by running static analysis tools (e.g., `staticcheck`, `deadcode`) and verifying zero reports of unused code.

**Acceptance Scenarios**:

1. **Given** the codebase, **When** running dead code analysis, **Then** no unused exported functions, types, or variables are reported
2. **Given** the codebase, **When** running dead code analysis, **Then** no unused unexported functions that aren't test helpers are reported
3. **Given** any function or type that was previously unused, **When** reviewing the codebase, **Then** it has been removed or documented as intentionally exported for external use

---

### User Story 4 - Code Structure and Organization (Priority: P4)

As a developer, I want the codebase to follow idiomatic Go patterns with appropriately sized files, well-named packages, and DRY principles so that the code is easy to navigate and maintain.

**Why this priority**: Good code organization reduces onboarding time and maintenance burden. While important, it doesn't block functionality.

**Independent Test**: Can be fully tested by reviewing package structure, checking file sizes against reasonable thresholds (e.g., <1000 lines per file), and verifying no duplicate logic exists.

**Acceptance Scenarios**:

1. **Given** any Go source file, **When** measuring line count, **Then** no file exceeds 1000 lines (excluding generated code)
2. **Given** any package, **When** reviewing its contents, **Then** all files in the package are thematically related
3. **Given** the codebase, **When** searching for duplicate logic patterns, **Then** no significant code duplication exists (DRY principle followed)
4. **Given** any package name, **When** reviewing Go naming conventions, **Then** the package name is lowercase, concise, and descriptive

---

### Edge Cases

- What happens when a new sensitive field is added to a model? (Should be documented which fields require redaction)
- How does redaction handle nested structs or JSON-encoded fields containing passwords?
- What if the mediacommon fork branch is merged to main? (Should be easy to update the dependency reference)
- What if static analysis reports false positives for "unused" code that's used via reflection or build tags?

## Requirements *(mandatory)*

### Functional Requirements

**Sensitive Data Redaction**:
- **FR-001**: System MUST use go-masq slog middleware to redact sensitive data in structured logs
- **FR-002**: System MUST redact password fields from stream source and EPG source log entries
- **FR-003**: System MUST redact credentials when logging Xtream API requests/responses
- **FR-004**: System MUST configure redaction patterns for fields named: `password`, `Password`, `secret`, `token`, `apikey`
- **FR-005**: Redacted values MUST be replaced with a consistent marker (e.g., `[REDACTED]`)

**Dependency Management**:
- **FR-006**: go.mod MUST reference `github.com/jmylchreest/mediacommon/v2` using the `feat/eac3-support` branch
- **FR-007**: go.mod MUST NOT contain local filesystem `replace` directives for external dependencies
- **FR-008**: All dependencies MUST be fetchable via standard `go mod download`

**Code Quality**:
- **FR-009**: Codebase MUST pass `staticcheck` analysis with zero errors
- **FR-010**: Codebase MUST have no unused exported functions, types, or constants (verified by dead code analysis)
- **FR-011**: No Go source file MUST exceed 1000 lines (excluding generated files and test files)
- **FR-012**: All packages MUST follow Go naming conventions (lowercase, no underscores, concise names)
- **FR-013**: Duplicate code blocks (>20 lines of identical or near-identical logic) MUST be refactored into shared functions

**Go Idioms**:
- **FR-014**: Code MUST follow Go 1.25 idioms and best practices
- **FR-015**: Error handling MUST follow Go conventions (explicit error returns, no panic for recoverable errors)
- **FR-016**: Interfaces MUST be defined where they are used, not where they are implemented (unless shared)

### Key Entities

- **StreamSource**: Contains `Password` field that must be redacted in logs
- **EpgSource**: Contains `Password` field that must be redacted in logs
- **XtreamClient**: Makes HTTP requests with credentials that must not appear in logs
- **Logger (slog)**: Central logging system that must be configured with go-masq redaction middleware

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero instances of plaintext passwords appear in any log output at any log level
- **SC-002**: Project builds successfully with `go build ./...` from a fresh clone without local dependencies
- **SC-003**: `staticcheck ./...` reports zero issues
- **SC-004**: Dead code analysis tools report zero unused exported symbols
- **SC-005**: No Go source file exceeds 1000 lines (excluding generated and test files)
- **SC-006**: All 19 files containing password references have appropriate redaction handling
- **SC-007**: go.mod contains zero local filesystem `replace` directives for external dependencies

## Assumptions

- The `feat/eac3-support` branch exists on `github.com/jmylchreest/mediacommon` and is accessible
- go-masq is compatible with the project's current slog configuration
- "Idiomatic Go 1.25" follows the patterns documented in Effective Go and the Go Code Review Comments
- Generated files (e.g., in `internal/assets/static/`) are excluded from line count and code quality checks
- Test files (`*_test.go`) are excluded from line count limits but should still follow quality guidelines
- The threshold of 1000 lines per file is a reasonable guideline; files slightly over may be acceptable if well-organized
- Duplicate code threshold of 20 lines is a guideline; smaller duplications may be acceptable for clarity
