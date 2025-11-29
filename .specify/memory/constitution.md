# Tvarr Constitution

## Related Documents

- **Architecture**: [.specify/memory/architecture.md](architecture.md) - System design and technical decisions
- **Roadmap**: [.specify/memory/roadmap.md](roadmap.md) - Feature phases and timeline
- **Personas**: [.specify/memory/personas/](personas/) - Role definitions for spec-driven development

## Core Principles

### I. Memory-First Design
Every component must be designed with memory efficiency as a primary concern. Large datasets (100k+ channels, millions of EPG entries) must be processed using streaming and batching patterns. No unbounded in-memory collections. All batch sizes must be configurable. Memory cleanup must be explicit between processing stages.

### II. Modular Pipeline Architecture
Features are implemented as composable pipeline stages. Each stage implements a common interface. Stages are independently testable and replaceable. Data flows through stages via streaming interfaces, not buffered collections. Pipeline orchestration is separate from stage implementation.

### III. Test-First Development (NON-NEGOTIABLE)
TDD mandatory: Tests written and approved before implementation begins. Red-Green-Refactor cycle strictly enforced. Integration tests required for: database operations, external API calls, FFmpeg interactions, pipeline stage composition. Contract tests for all public interfaces.

### IV. Clean Architecture with SOLID Principles
- **Single Responsibility**: Each package/type has one reason to change
- **Open/Closed**: Extend via interfaces, not modification
- **Liskov Substitution**: Implementations are interchangeable
- **Interface Segregation**: Small, focused interfaces
- **Dependency Inversion**: Depend on abstractions, not concretions

Repository pattern for all database access. Service layer for business logic. Handler layer for HTTP/CLI concerns.

### V. Idiomatic Go
Follow Go conventions strictly: error handling via explicit returns, context propagation for cancellation, goroutines with proper lifecycle management, defer for cleanup, table-driven tests. No exceptions to `go fmt` and `go vet`. Use `golangci-lint` with strict configuration.

### VI. Observable and Debuggable
Structured logging with slog (no emojis in log output). All operations must be traceable through request IDs. Health checks and readiness probes required. Circuit breakers for external dependencies.

### VII. Security by Default
All file operations sandboxed to configured directories. Path traversal prevention mandatory. Input validation at system boundaries. No hardcoded credentials. Configuration via environment variables with sensible defaults.

### VIII. No Magic Strings or Literals
**Reusable values must be constants or variables, never inline literals.** This includes:
- Version strings, user agents, application names
- HTTP headers, content types, status messages  
- Configuration keys, environment variable names
- Error messages that may be matched or displayed
- API endpoint paths, query parameter names

Build-time values (version, commit, build date) must be injected via LDFLAGS, not hardcoded. A centralized `internal/version` package must expose these values.

### IX. Resilient HTTP Clients
All external HTTP communication must use a standardized HTTP client package (`pkg/httpclient`) that provides:
- **Configurable circuit breaker** with service-specific profiles
- **Automatic retries** with exponential backoff
- **Transparent decompression** (gzip, deflate, brotli)
- **Structured logging and instrumentation** with request/response tracing
- **Timeout configuration** at connect and request levels
- **Credential obfuscation** in logs and error messages

The client must accept the standard `*http.Client` interface for injection and testing. Circuit breaker state must be observable via health endpoints.

### X. Production-Grade CI/CD
This project uses **GitHub Actions** with a multi-stage, matrix-based build pipeline:

**Build Requirements:**
- Matrix builds for: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`
- ARM64 builds use dedicated ARM runners where available
- All binaries built with CGO_ENABLED=0 for static linking
- LDFLAGS injection for version, commit SHA, and build timestamp

**Pipeline Stages:**
1. **Prepare** - Determine version (tag-based or snapshot with short SHA)
2. **Lint** - golangci-lint with strict configuration
3. **Test** - Full test suite with coverage reporting
4. **Build** - Matrix builds with artifact upload
5. **Release** - GoReleaser for tagged releases

**Artifact Standards:**
- Binary naming: `{name}_{version}_{os}_{arch}`
- Checksums for all release artifacts
- SBOM generation for supply chain security

## Technology Stack

| Layer | Technology | Version |
|-------|------------|---------|
| Language | Go | 1.25.x (latest stable) |
| Web Framework | Huma + Chi | v2.34+ / Latest stable |
| ORM | GORM | v2 |
| Database | SQLite/PostgreSQL/MySQL | Configurable |
| Logging | slog (stdlib) | Go 1.21+ |
| Configuration | Viper | Latest |
| CLI | Cobra | Latest |
| Testing | testify + gomock | Latest |
| FFmpeg | External binary or go-ffstatic | Configurable |
| **Build System** | **Taskfile** | Latest |

## Build System: Taskfile

This project uses [Taskfile](https://taskfile.dev/) instead of Make. Taskfile is designed for Go projects and provides:
- YAML-based task definitions
- Cross-platform compatibility
- Built-in Go task support
- Variable interpolation
- Task dependencies

All build, test, and development commands are defined in `Taskfile.yml` at the project root.

Common tasks:
```bash
task build      # Build the binary
task test       # Run tests
task lint       # Run linter
task run        # Build and run
task migrate    # Run database migrations
task clean      # Clean build artifacts
```

## Personas

This project uses persona-based development with spec-kit. Personas define roles, responsibilities, and decision authority for AI agents working on the codebase.

### Available Personas

| Persona | Role | Primary Phases |
|---------|------|----------------|
| **Product Owner** | Define requirements and business value | Specify, Validate |
| **Architect** | Design system architecture | Plan, Tasks, Validate |
| **Go Engineer** | Implement features | Implement |
| **Senior Go Engineer** | Complex implementations, mentoring | Implement, Validate |
| **Code Reviewer** | Final quality gate | Validate |
| **QA Tester** | Testing and validation | Validate |
| **Performance Engineer** | Profiling and optimization | Plan, Validate |
| **Security Engineer** | Security review (VETO POWER) | Plan, Validate |
| **Documentation Specialist** | Technical documentation | All phases |
| **DevOps Engineer** | CI/CD and deployment | Plan, Implement |
| **Client** | Business requirements, UAT | Specify, Validate (FINAL) |

### Persona Usage

When working on a task, adopt the appropriate persona:
1. Read the persona file in `.specify/memory/personas/{persona}.yaml`
2. Follow the constraints and decision authority defined
3. Defer to other personas as specified
4. Produce the deliverables expected for that role

### Decision Authority Hierarchy

```
Client (Final Approval)
    └─→ Product Owner (Business)
          └─→ Architect (System Design)
                ├─→ Senior Go Engineer (Complex Impl)
                ├─→ Performance Engineer (Optimization)
                ├─→ Security Engineer (VETO on Security)
                ├─→ Go Engineer (Implementation)
                ├─→ DevOps Engineer (CI/CD)
                └─→ Documentation Specialist (Docs)
          └─→ QA Tester (Quality Gates)
                └─→ Code Reviewer (Merge Approval)
```

## Development Workflow

### Spec-Kit Phases

1. **Constitution** - This document (updated when principles change)
2. **Specify** - `/speckit.specify` (Product Owner - define requirements)
3. **Clarify** - `/speckit.clarify` (Architect - resolve ambiguities)
4. **Plan** - `/speckit.plan` (Architect - technical design)
5. **Tasks** - `/speckit.tasks` (Architect - task decomposition)
6. **Implement** - `/speckit.implement` (Engineers - code generation)
7. **Validate** - `/speckit.analyze`, `/speckit.checklist` (QA/Security/Client - validation)

### Quality Gates (ALL REQUIRED)

- [ ] Spec approved (Product Owner)
- [ ] Plan approved (Architect)
- [ ] Tests pass (QA Tester)
- [ ] Performance validated (Performance Engineer)
- [ ] Security cleared (Security Engineer - VETO POWER)
- [ ] Code reviewed (Code Reviewer)
- [ ] UAT approved (Client - FINAL)

### Code Standards

- All tests pass (`task test`)
- `golangci-lint` passes with zero warnings (`task lint`)
- Code coverage minimum 80% for new code
- No TODO comments without linked issues
- All exported functions have godoc comments
- No magic numbers (use named constants)
- Structured logging with appropriate levels

## Key Systems (from Architecture)

The following systems are critical and must preserve their design:

1. **Expression Filter/Rules Engine**
   - Recursive descent parser with AST
   - Field registry with aliases
   - Complex grouped expressions
   - Validation with suggestions

2. **FFmpeg Proxy Methods**
   - Redirect, Proxy, Relay modes
   - Hardware acceleration detection
   - Relay profiles with transcoding
   - Circuit breaker for failures

3. **Logo Registry & Caching**
   - In-memory index with hash-based lookup
   - Deterministic URL normalization
   - Lazy loading with background scan
   - No database storage for cached logos

4. **Staged Pipeline**
   - Data mapping/rules stages
   - Source merging logic
   - Static content rendering
   - Progress tracking

See [architecture.md](architecture.md) for detailed specifications.

## Performance Targets

| Metric | Target | Threshold |
|--------|--------|-----------|
| Channel Ingestion (100k) | < 5 min | < 10 min |
| EPG Ingestion (1M programs) | < 10 min | < 20 min |
| Proxy Generation (50k channels) | < 2 min | < 5 min |
| Memory (ingestion) | < 500 MB | < 1 GB |
| Memory (generation) | < 500 MB | < 1 GB |
| API Response (list) | < 200 ms | < 500 ms |
| Relay Startup | < 3 s | < 5 s |

## Governance

This constitution supersedes all other practices. Amendments require:
1. Written justification
2. Impact analysis
3. Migration plan for existing code
4. Approval from Architect and Product Owner

All code reviews must verify constitutional compliance. Complexity violations must be documented with justification.

Security Engineer has **VETO POWER** on any security-related concerns.

**Version**: 2.1.0 | **Ratified**: 2025-11-29 | **Last Amended**: 2025-11-29
