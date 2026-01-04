# Requirements Checklist: Codebase Cleanup & Migration Compaction

## Quality Checklist

### Specification Completeness
- [x] Feature branch name is defined (`010-codebase-cleanup`)
- [x] Creation date is specified (2025-12-07)
- [x] User input/request is captured
- [x] User scenarios have priorities assigned (P1-P3)
- [x] Each user story has acceptance scenarios with Given/When/Then format
- [x] Edge cases are identified and addressed
- [x] Functional requirements are defined with unique IDs (FR-001 to FR-019)
- [x] Success criteria are measurable (SC-001 to SC-010)
- [x] Assumptions are documented

### Technical Accuracy
- [x] Requirements align with the user's request for dead code removal
- [x] Requirements align with the user's request for wrapper method elimination
- [x] Requirements align with the user's request for migration compaction
- [x] No production database assumption is documented
- [x] API backward compatibility is required
- [x] Test compatibility is required

### Scope Clarity
- [x] Backend/Go only scope is defined (frontend out of scope)
- [x] Generated code is explicitly out of scope
- [x] Test helper code preservation is addressed
- [x] Interface implementations are protected from removal
- [x] Public API preservation is required

### Feasibility
- [x] Migration compaction strategy is defined
- [x] Seed data to preserve is enumerated
- [x] Success criteria are achievable and measurable
- [x] Review areas are prioritized

## Requirements Coverage

### Dead Code Removal (FR-001 to FR-006)
- [ ] Unreferenced private functions identified
- [ ] Unreferenced private types identified
- [ ] Unused imports removed
- [ ] Commented-out code reviewed
- [ ] Interface implementations preserved
- [ ] Public API functions preserved

### Method Wrapper Elimination (FR-007 to FR-011)
- [ ] Wrapper methods identified
- [ ] Call sites updated
- [ ] HTTP API backward compatibility verified
- [ ] Meaningful abstractions preserved

### Migration Compaction (FR-012 to FR-016)
- [ ] Migrations compacted to 3 or fewer
- [ ] Schema identity verified
- [ ] Seed data preserved
- [ ] Migration versioning maintained
- [ ] Registry updated

### Code Consistency (FR-017 to FR-019)
- [ ] Repository patterns consistent
- [ ] Handler patterns consistent
- [ ] Model naming consistent

## Success Criteria Verification

- [ ] SC-001: Go build passes
- [ ] SC-002: All tests pass
- [ ] SC-003: golangci-lint clean
- [ ] SC-004: Migration count <= 3
- [ ] SC-005: No unused imports (go vet)
- [ ] SC-006: Codebase size reduced >= 5%
- [ ] SC-007: API endpoints unchanged
- [ ] SC-008: DB init < 1 second
- [ ] SC-009: No trivial wrapper methods
- [ ] SC-010: E2E tests pass
