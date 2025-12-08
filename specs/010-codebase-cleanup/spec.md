# Feature Specification: Codebase Cleanup & Migration Compaction

**Feature Branch**: `010-codebase-cleanup`
**Created**: 2025-12-07
**Status**: Draft
**Input**: User description: "Thorough codebase review to remove dead code, eliminate unnecessary method wrappers (methods that just call other methods without benefit like myMethod() calling myMethodWithOptions()), compact database migrations since no production database exists, fix golangci-lint and ESLint warnings, use modernized code patterns, and optimize for zero-allocation where possible with proper memory cleanup"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Clean Codebase Without Dead Code (Priority: P1)

As a developer working on tvarr, I want the codebase to contain only actively used code, so I can understand the system more easily and avoid maintaining unused functionality.

**Why this priority**: Dead code increases cognitive load, can confuse developers about what's actually used, and adds unnecessary maintenance burden. Removing it is the highest impact cleanup task.

**Independent Test**: Can be fully tested by building the application and verifying all tests pass after dead code removal, with no functionality changes observed in the running application.

**Acceptance Scenarios**:

1. **Given** the codebase contains unused functions, **When** I run code analysis tools, **Then** no unreferenced public functions exist outside of API contracts
2. **Given** the codebase contains unused imports, **When** I run go fmt and linters, **Then** no unused imports remain
3. **Given** the codebase contains commented-out code blocks, **When** I review the codebase, **Then** no significant commented-out code remains (excluding documentation)

---

### User Story 2 - Simplified Method Signatures Without Unnecessary Wrappers (Priority: P1)

As a developer, I want methods with a single clear purpose without unnecessary indirection, so I can understand the code flow without following chains of trivial wrapper methods.

**Why this priority**: Methods that simply call other methods (like `myMethod()` just calling `myMethodWithOptions(defaults)`) add indirection without value. They obscure the actual logic and make debugging harder.

**Independent Test**: Can be fully tested by reviewing all public methods that are simple pass-throughs and either inlining them or removing them, then verifying all tests pass.

**Acceptance Scenarios**:

1. **Given** a method exists that only calls another method with default parameters, **When** I review the method, **Then** it either provides meaningful abstraction or is removed/inlined
2. **Given** a method wrapper exists, **When** I analyze its call sites, **Then** callers are updated to use the underlying method directly where appropriate
3. **Given** wrapper methods have been removed, **When** I build and test, **Then** all functionality remains intact

---

### User Story 3 - Compacted Database Migrations (Priority: P2)

As a developer deploying tvarr for the first time, I want database migrations to be concise and fast, so initial setup doesn't require running 34+ incremental migrations when a single schema creation would suffice.

**Why this priority**: With no production databases to migrate, the incremental migration history provides no value. A clean slate migration improves deployment speed and reduces complexity.

**Independent Test**: Can be fully tested by running the compacted migrations against a fresh SQLite database and verifying the resulting schema matches the current schema exactly.

**Acceptance Scenarios**:

1. **Given** the current 34 migrations exist, **When** migrations are compacted, **Then** a single or minimal set of migrations creates the complete current schema
2. **Given** compacted migrations run on a fresh database, **When** I compare the schema to pre-compaction, **Then** both schemas are functionally identical
3. **Given** migrations are compacted, **When** I run all tests, **Then** all database-dependent tests pass

---

### User Story 4 - Zero Lint Warnings (Priority: P1)

As a developer, I want the codebase to have zero lint warnings in both Go and TypeScript, so I can trust the linter output and catch real issues quickly.

**Why this priority**: Linter warnings indicate potential bugs, anti-patterns, or code quality issues. A clean lint output means developers can trust new warnings are meaningful.

**Independent Test**: Run `golangci-lint run ./...` and `pnpm run lint` with zero warnings.

**Acceptance Scenarios**:

1. **Given** golangci-lint runs on the Go codebase, **When** I check the output, **Then** there are zero warnings
2. **Given** ESLint runs on the frontend, **When** I check the output, **Then** there are zero warnings
3. **Given** proper error handling is required, **When** I review errcheck warnings, **Then** all unchecked returns are either handled or explicitly ignored with `_ =`

---

### User Story 5 - Memory-Efficient Code (Priority: P1)

As a system administrator, I want tvarr to use memory efficiently with zero-allocation patterns where possible, so large channel lists (100k+) can be processed without excessive memory usage.

**Why this priority**: IPTV systems can have massive channel lists. Unnecessary allocations cause GC pressure and high memory usage. Pre-allocation and cleanup are critical for scalability.

**Independent Test**: Profile memory usage before/after changes; verify no regressions and improvements in hot paths.

**Acceptance Scenarios**:

1. **Given** a pipeline stage processes channels, **When** it allocates slices, **Then** slices are pre-allocated with known or estimated capacity
2. **Given** a pipeline stage completes processing, **When** it no longer needs data, **Then** large slices/maps are set to nil for GC collection
3. **Given** streaming operations use buffers, **When** buffers are allocated frequently, **Then** sync.Pool is considered for reuse

---

### User Story 6 - Consistent Code Patterns (Priority: P3)

As a developer, I want consistent patterns throughout the codebase, so similar operations are implemented similarly and code is predictable.

**Why this priority**: Inconsistent patterns make code harder to read and maintain. While not as critical as dead code removal, consistency improves long-term maintainability.

**Independent Test**: Can be fully tested by reviewing similar components (e.g., all repositories, all handlers) and verifying they follow consistent patterns.

**Acceptance Scenarios**:

1. **Given** multiple repository implementations exist, **When** I compare their structure, **Then** they follow the same organizational patterns
2. **Given** multiple handlers exist, **When** I compare error handling patterns, **Then** they handle errors consistently
3. **Given** configuration is loaded in multiple places, **When** I review loading patterns, **Then** they use a consistent approach

---

### Edge Cases

- What happens when removing code that appears dead but is called via reflection or interface satisfaction?
  - Verify all interface implementations are preserved; use compiler to detect missing interface methods
- What happens when compacting migrations that contain data manipulation (not just schema)?
  - Ensure seed data (default profiles, filters, rules) is preserved in compacted migration
- What if removing a wrapper breaks external API compatibility?
  - All HTTP API endpoints must remain unchanged; only internal method signatures can change
- What happens when removing code that is only used in tests?
  - Test-only helpers in `_test.go` files should be retained; only production dead code is removed

## Requirements *(mandatory)*

### Functional Requirements

**Dead Code Removal**
- **FR-001**: System MUST identify and remove all unreferenced private functions
- **FR-002**: System MUST identify and remove all unreferenced private types
- **FR-003**: System MUST remove all unused imports
- **FR-004**: System MUST remove significant blocks of commented-out code
- **FR-005**: System MUST NOT remove functions that satisfy interfaces even if not explicitly called
- **FR-006**: System MUST NOT remove exported functions that are part of the public API

**Method Wrapper Elimination**
- **FR-007**: System MUST identify methods that only call another method with default parameters
- **FR-008**: System MUST inline or remove wrapper methods where they provide no abstraction value
- **FR-009**: System MUST update all call sites when wrapper methods are removed
- **FR-010**: System MUST preserve backward compatibility for HTTP API endpoints
- **FR-011**: System MUST NOT remove methods that provide meaningful abstraction or error handling

**Migration Compaction**
- **FR-012**: System MUST compact the 34+ migrations into a minimal set (ideally 1-3)
- **FR-013**: Compacted migrations MUST create identical database schema to current migrations
- **FR-014**: Compacted migrations MUST include all seed data (default profiles, filters, rules, mappings)
- **FR-015**: Compacted migrations MUST preserve migration versioning for future changes
- **FR-016**: Migration registry MUST be updated to reference new compacted migrations

**Lint Cleanup**
- **FR-017**: All golangci-lint warnings MUST be resolved (errors handled or explicitly ignored with `_ =`)
- **FR-018**: All ESLint warnings MUST be resolved (unused imports removed, types defined, hooks fixed)
- **FR-019**: All type assertions in Go MUST use two-value form or be explicitly justified
- **FR-020**: All viper.BindPFlag calls MUST handle errors (panic at startup is acceptable)

**Memory Optimization**
- **FR-021**: Slice allocations in pipeline stages MUST pre-allocate with known or estimated capacity
- **FR-022**: Map allocations MUST pre-allocate when count is known
- **FR-023**: Large slices/maps MUST be set to nil after use in pipeline stages for GC collection
- **FR-024**: Frequently allocated objects SHOULD use sync.Pool where beneficial
- **FR-025**: String concatenation in loops MUST use strings.Builder

**Code Consistency**
- **FR-026**: All repositories MUST follow consistent error handling patterns
- **FR-027**: All handlers MUST follow consistent request validation patterns
- **FR-028**: All models MUST follow consistent field naming conventions

### Key Entities

- **Migration**: A database schema change unit with version, description, Up, and Down functions
- **Repository**: Data access layer implementing CRUD operations for a model
- **Handler**: HTTP handler processing API requests and returning responses
- **Model**: GORM model representing a database table

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Go build produces no errors after cleanup
- **SC-002**: All existing tests pass without modification (unless tests were for removed dead code)
- **SC-003**: golangci-lint produces ZERO warnings (down from ~100)
- **SC-004**: ESLint produces ZERO warnings (down from 422)
- **SC-005**: Migration count reduced from 34+ to 3 or fewer
- **SC-006**: No unused imports detected by go vet
- **SC-007**: Codebase size reduced by at least 5% in lines of code (excluding generated code)
- **SC-008**: All API endpoints remain functional with identical request/response contracts
- **SC-009**: Fresh database initialization completes in under 1 second (vs current ~2s for 34 migrations)
- **SC-010**: No wrapper methods that simply pass through to another method with default values
- **SC-011**: All pipeline stage slice allocations use pre-allocation with capacity hints
- **SC-012**: No `any` types in TypeScript codebase (all replaced with proper interfaces)
- **SC-013**: E2E tests pass with compacted migrations

## Assumptions

- No production database exists that needs to be migrated from old schema versions
- All current tests represent desired behavior and must continue to pass
- External API contracts (HTTP endpoints, request/response shapes) are stable and must not change
- The frontend is out of scope for this cleanup (backend/Go only)
- Generated code (if any) is out of scope and should not be modified
- Test files (`_test.go`) may have helper code that shouldn't be treated as dead code

## Areas to Review

### High-Priority Areas

1. **Database Migrations** (`internal/database/migrations/registry.go`)
   - 34 migrations spanning 1766 lines
   - Many incremental changes that can be collapsed
   - Data seeding interspersed with schema changes

2. **Repository Layer** (`internal/repository/`)
   - Check for unused repository methods
   - Look for wrapper methods that just call other methods
   - Verify consistent patterns

3. **Handler Layer** (`internal/http/handlers/`)
   - Check for unused handlers
   - Look for duplicate code across handlers
   - Verify consistent error handling

4. **Expression Engine** (`internal/expression/`)
   - Complex module with multiple files
   - Check for unused operators or evaluators
   - Look for dead AST node types

### Medium-Priority Areas

5. **Config Package** (`internal/config/`)
   - Check for unused config fields
   - Look for unused helper functions

6. **FFmpeg Integration** (`internal/ffmpeg/`)
   - Check for unused codec mappings
   - Look for dead hardware acceleration code paths

7. **Models** (`internal/models/`)
   - Check for unused model fields
   - Look for unused enum values

### Lower-Priority Areas

8. **Scheduler** (`internal/scheduler/`)
   - Check for unused job types
   - Look for unused hooks

9. **Assets** (`internal/assets/`)
   - Check for unused embedded resources

## Migration Compaction Strategy

The 34 migrations should be compacted into approximately 3 migrations:

1. **Migration 001 - Schema Creation**
   - All table definitions (stream_sources, channels, epg_sources, epg_programs, etc.)
   - All indexes and constraints
   - No data

2. **Migration 002 - System Data**
   - Default filters (Include All Valid Stream URLs, Exclude Adult Content)
   - Default data mapping rules (Timeshift Detection)
   - Default relay profiles (Automatic, Passthrough, h264/AAC, h265/AAC, VP9/Opus, AV1/Opus)
   - Default relay profile mappings (all client detection rules)

3. **Migration 003 - Future Placeholder** (optional)
   - Reserve version 003 for first post-compaction migration
   - Allows easy identification of when compaction occurred

### Data to Preserve in Compaction

From current migrations, the following seed data must be included:

- **Filters** (from migration013):
  - "Include All Valid Stream URLs"
  - "Exclude Adult Content"

- **Data Mapping Rules** (from migration013):
  - "Default Timeshift Detection (Regex)"

- **Relay Profiles** (current state after all migrations):
  - "Automatic" (default)
  - "Passthrough"
  - "h264/AAC"
  - "h265/AAC"
  - "VP9/Opus"
  - "AV1/Opus"

- **Relay Profile Mappings** (from migration034):
  - All 21+ client detection rules (Safari, Chrome, Firefox, VLC, etc.)
