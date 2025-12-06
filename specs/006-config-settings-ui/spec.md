# Feature Specification: Configuration Settings & Debug UI Consolidation

**Feature Branch**: `006-config-settings-ui`
**Created**: 2025-12-06
**Status**: Draft
**Input**: User description: "Comprehensive configuration review and debug UI improvements - consolidate runtime vs startup settings, fix debug page errors, enable config persistence, show CPU/memory/circuit breaker metrics with rich visualization, simplify/consolidate backend API"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View and Modify Runtime Settings (Priority: P1)

As an administrator, I want to view all runtime-configurable settings in a consolidated interface and modify them without restarting the service, so I can tune operational behavior on-the-fly.

**Why this priority**: This is the core value proposition - enabling dynamic configuration changes without service disruption. Critical for production operations.

**Independent Test**: Can be fully tested by changing log level from "info" to "debug" and verifying logs immediately show debug messages without service restart.

**Acceptance Scenarios**:

1. **Given** the settings page is open, **When** I view the Runtime Settings section, **Then** I see all runtime-configurable options (log level, request logging, circuit breaker thresholds) with their current values
2. **Given** a runtime setting is displayed, **When** I change its value and click Save, **Then** the change takes effect immediately without requiring service restart
3. **Given** I have made runtime setting changes, **When** I click the Save button, **Then** I receive confirmation that changes were applied with a list of what changed

---

### User Story 2 - View System Health Metrics on Debug Page (Priority: P1)

As an administrator, I want to view CPU usage, memory consumption, and circuit breaker status on the debug page, so I can monitor system health and diagnose performance issues.

**Why this priority**: Essential for operational visibility and troubleshooting. The debug page is currently broken and unusable.

**Independent Test**: Can be fully tested by opening the debug page and verifying CPU load percentage, memory usage bars, and circuit breaker states are displayed without errors.

**Acceptance Scenarios**:

1. **Given** the debug page is open, **When** health data is available, **Then** I see CPU load (1/5/15 min averages), memory usage (total, used, free), and process memory breakdown
2. **Given** circuit breakers are active, **When** viewing the debug page, **Then** I see each breaker's name, state (Closed/Open/Half-Open), success/failure counts, and failure rate
3. **Given** any health component data is unavailable, **When** viewing the debug page, **Then** the page renders gracefully with "Not available" indicators instead of crashing

---

### User Story 3 - Consolidated Configuration API (Priority: P1)

As a developer integrating with tvarr, I want a unified configuration API that reduces the number of endpoints I need to call, so I can build simpler and more maintainable integrations.

**Why this priority**: The current API fragmentation (6+ separate endpoints for config-related data) creates complexity for both the frontend and any external integrations. Consolidation reduces maintenance burden and improves developer experience.

**Independent Test**: Can be fully tested by calling a single unified endpoint and receiving all configuration data (runtime, startup, circuit breakers) in one response.

**Acceptance Scenarios**:

1. **Given** I need configuration data, **When** I call the unified config endpoint, **Then** I receive runtime settings, startup config, and circuit breaker config in a single structured response
2. **Given** I want to update runtime settings, **When** I submit changes to the unified endpoint, **Then** all changes are applied atomically with a single response
3. **Given** the old endpoints exist, **When** I call them during a transition period, **Then** they continue to work but may be marked as deprecated

---

### User Story 4 - Rich Circuit Breaker Visualization (Priority: P2)

As an administrator, I want to see detailed circuit breaker metrics including a visual breakdown of success/error rates by category and time spent in each state, so I can quickly diagnose resilience issues and understand system behavior patterns.

**Why this priority**: Circuit breakers are critical for system resilience. Rich visualization helps operators understand failure patterns and make informed decisions about threshold tuning.

**Independent Test**: Can be fully tested by viewing a circuit breaker card and verifying it shows a segmented bar with success/error breakdown, state duration, and recent state transition history.

**Acceptance Scenarios**:

1. **Given** a circuit breaker has processed requests, **When** I view its card on the debug page, **Then** I see a segmented progress bar showing success rate with error breakdown (server errors, client errors, timeouts, network errors)
2. **Given** a circuit breaker has been active, **When** I view its status, **Then** I see how long it has been in its current state and a summary of time spent in each state (Closed, Open, Half-Open)
3. **Given** state transitions have occurred, **When** I view the circuit breaker details, **Then** I see a timeline or list of recent state changes with timestamps and trigger reasons
4. **Given** the circuit breaker is in Open state, **When** I view its card, **Then** I see when it will attempt to transition to Half-Open (countdown/timestamp)

---

### User Story 5 - View Startup Configuration (Read-Only) (Priority: P2)

As an administrator, I want to view all startup-only configuration values in a dedicated section, clearly distinguished from runtime settings, so I understand what requires a restart to change.

**Why this priority**: Important for configuration transparency but doesn't block day-to-day operations. Users need to know what can't be changed dynamically.

**Independent Test**: Can be fully tested by viewing the Startup Configuration section and verifying all server, database, storage, pipeline, and scheduler settings are displayed as read-only.

**Acceptance Scenarios**:

1. **Given** the settings page is open, **When** I view the Startup Configuration section, **Then** I see grouped settings (Server, Database, Storage, Pipeline, Scheduler, Relay) with current values
2. **Given** a startup setting is displayed, **When** I view it, **Then** it is clearly marked as read-only with an indicator that restart is required to change
3. **Given** I want to change a startup setting, **When** I view the Startup Configuration section, **Then** I see instructions for how to modify the config file or CLI flags

---

### User Story 6 - Persist Configuration Changes to File (Priority: P2)

As an administrator, I want to persist my runtime and startup configuration changes to the config file, so changes survive service restarts.

**Why this priority**: Without persistence, runtime changes are lost on restart. This completes the configuration management workflow.

**Independent Test**: Can be fully tested by changing a setting, clicking "Save to Config", restarting the service, and verifying the setting persists.

**Acceptance Scenarios**:

1. **Given** I have modified runtime settings, **When** I click "Save to Config File", **Then** the changes are written to the configuration file
2. **Given** changes have been saved to config, **When** the service restarts, **Then** the saved settings are loaded and applied
3. **Given** I attempt to save config, **When** the config file is not writable, **Then** I receive a clear error message explaining the issue

---

### User Story 7 - Modify Circuit Breaker Configuration (Priority: P3)

As an administrator, I want to adjust circuit breaker thresholds and timeouts at runtime, so I can tune resilience behavior without restart.

**Why this priority**: Advanced tuning capability for specific operational scenarios. Most users will use defaults.

**Independent Test**: Can be fully tested by changing a circuit breaker's failure threshold from 3 to 5, then observing the breaker behavior reflects the new threshold.

**Acceptance Scenarios**:

1. **Given** circuit breakers are configured, **When** I view circuit breaker settings, **Then** I see global defaults and per-service overrides with current values
2. **Given** I want to change a threshold, **When** I modify and save, **Then** the change applies immediately to that circuit breaker
3. **Given** I want to reset a tripped breaker, **When** I click Reset on a specific breaker, **Then** it returns to Closed state

---

### Edge Cases

- What happens when the health endpoint returns partial data (some components missing)?
  - UI displays available data and shows "Not available" for missing components
- How does the system handle concurrent config changes from multiple administrators?
  - Last write wins; UI refreshes to show current state after save
- What happens when config file save fails due to permissions?
  - Clear error message displayed; runtime changes still applied (just not persisted)
- What if circuit breaker service doesn't exist when trying to configure it?
  - Error displayed; user informed the breaker will be configured when service is created
- What happens during config save if the service is under heavy load?
  - Save operation queued; user sees loading indicator; timeout with retry option after 30 seconds
- What happens to existing API consumers when endpoints are consolidated?
  - Deprecated endpoints continue to work during transition; deprecation warnings returned in responses
- What if circuit breaker has no request history yet?
  - UI shows "No data yet" state with zero counts and neutral styling
- What happens when state transition history exceeds storage limits?
  - Oldest transitions are discarded (circular buffer); UI shows "showing last N transitions"

## Requirements *(mandatory)*

### Functional Requirements

**Debug Page - Core**
- **FR-001**: System MUST render debug page without errors when any health component data is undefined or null
- **FR-002**: System MUST display CPU load percentages (1min, 5min, 15min averages) when health data is available
- **FR-003**: System MUST display memory usage (total, used, free, available) in human-readable format (MB/GB)
- **FR-004**: System MUST display process memory breakdown (main process, child processes, total tree)
- **FR-005**: System MUST use conditional rendering/optional chaining to prevent runtime errors from undefined data

**Circuit Breaker Visualization - UI**
- **FR-006**: System MUST display a segmented progress bar for each circuit breaker showing request outcome distribution
- **FR-007**: The progress bar MUST visually distinguish: successful requests (green), server errors 5xx (red), client errors 4xx (orange), timeouts (yellow), network errors (gray)
- **FR-008**: System MUST display the current state prominently with color coding: Closed (green), Open (red), Half-Open (amber)
- **FR-009**: System MUST show time elapsed in current state (e.g., "Closed for 2h 15m")
- **FR-010**: System MUST display a state duration summary showing percentage/time spent in each state since startup
- **FR-011**: System MUST show recent state transitions as a timeline or list with timestamps and trigger reasons
- **FR-012**: When in Open state, system MUST display when the next Half-Open transition attempt will occur
- **FR-013**: System MUST show consecutive failure/success counts relative to configured thresholds (e.g., "3/5 failures")

**Circuit Breaker Visualization - Backend**
- **FR-014**: Backend MUST track error counts by category: successful (2xx), client errors (4xx), server errors (5xx), timeouts, network errors
- **FR-015**: Backend MUST record state transitions with: timestamp, old state, new state, trigger reason, consecutive count at transition
- **FR-016**: Backend MUST maintain a bounded history of state transitions (configurable, default: last 50 transitions per breaker)
- **FR-017**: Backend MUST track time spent in each state (cumulative since startup or last reset)
- **FR-018**: Backend MUST expose circuit breaker metrics through the health endpoint or unified config endpoint

**Backend API Consolidation**
- **FR-019**: System MUST provide a unified configuration endpoint that returns all config data in a single response
- **FR-020**: The unified endpoint MUST clearly separate runtime-modifiable settings from startup-only settings in response structure
- **FR-021**: The unified endpoint MUST include circuit breaker configuration and statistics alongside other settings
- **FR-022**: System MUST support atomic updates of multiple runtime settings in a single request
- **FR-023**: System MAY maintain deprecated endpoints for backward compatibility during transition
- **FR-024**: Deprecated endpoints MUST return deprecation warnings in response headers or body

**Settings Page - Runtime Settings**
- **FR-025**: System MUST display all runtime-configurable settings with current values
- **FR-026**: System MUST allow modification of: log level, request logging enabled, feature flags, circuit breaker thresholds
- **FR-027**: System MUST apply runtime setting changes immediately without service restart
- **FR-028**: System MUST provide visual feedback when settings are modified but not yet saved
- **FR-029**: System MUST confirm successful save with list of applied changes

**Settings Page - Startup Configuration**
- **FR-030**: System MUST display all startup-only configuration values grouped by category
- **FR-031**: System MUST clearly distinguish startup settings as read-only
- **FR-032**: System MUST display startup settings in categories: Server, Database, Storage, Pipeline, Scheduler, Relay, Ingestion
- **FR-033**: System MUST show the source of each startup setting (config file, CLI flag, environment variable, default)

**Configuration Persistence**
- **FR-034**: System MUST provide ability to save current configuration to config file
- **FR-035**: System MUST validate write permissions before attempting config save
- **FR-036**: System MUST preserve comments and formatting in existing config file when possible
- **FR-037**: System MUST create config file if it doesn't exist (using default location)
- **FR-038**: System MUST report success/failure of config persistence operation to user

**Circuit Breaker Configuration**
- **FR-039**: System MUST display circuit breaker global configuration
- **FR-040**: System MUST display per-service circuit breaker overrides
- **FR-041**: System MUST allow runtime modification of circuit breaker thresholds and timeouts
- **FR-042**: System MUST provide ability to reset individual circuit breakers to Closed state

### Key Entities

- **RuntimeSetting**: A configuration value that can be changed without restart (log_level, enable_request_logging, feature flags, circuit breaker thresholds)
- **StartupConfig**: A configuration value that requires restart to change (server settings, database, storage paths, etc.)
- **CircuitBreaker**: A resilience component with state (Closed/Open/Half-Open) and statistics
- **CircuitBreakerStats**: Metrics including request counts by outcome category, state durations, and transition history
- **StateTransition**: A recorded change in circuit breaker state with timestamp, reason, and context
- **HealthMetrics**: System health data including CPU, memory, component status
- **ConfigFile**: YAML configuration file that persists settings across restarts
- **UnifiedConfigResponse**: A single API response structure containing all configuration categories

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Debug page loads without runtime errors 100% of the time regardless of health endpoint response content
- **SC-002**: Users can view all current configuration values (both runtime and startup) on the settings page within 2 seconds of page load
- **SC-003**: Runtime setting changes take effect within 1 second of clicking Save
- **SC-004**: Configuration persistence to file completes within 5 seconds with success/failure feedback
- **SC-005**: CPU and memory metrics update on debug page at user-configurable refresh intervals (1-60 seconds)
- **SC-006**: Circuit breaker status displays accurate real-time state for all active breakers
- **SC-007**: Settings page clearly distinguishes runtime-modifiable from startup-only settings at a glance
- **SC-008**: Frontend requires no more than 2 API calls to load complete settings page (down from 4+ currently)
- **SC-009**: API response payload size for unified config is smaller than sum of individual endpoint responses
- **SC-010**: Circuit breaker visualization shows error breakdown by category within 1 refresh cycle of the error occurring
- **SC-011**: State transition history displays at least the last 10 transitions per circuit breaker
- **SC-012**: Users can identify at a glance which circuit breakers have experienced issues (visual distinction)

## Assumptions

- Backend API changes are in scope - both consolidation and new endpoints
- Backend changes to circuit breaker tracking are in scope (state transitions, error categorization)
- Config file format is YAML as established in the codebase
- Users have access to the filesystem where config files are stored for persistence to work
- Circuit breaker configuration changes don't require recreating existing breaker instances
- Backward compatibility for deprecated endpoints is desired but not required indefinitely
- The `/health` endpoint remains separate as it serves a different purpose (health checks vs configuration)
- State transition history is stored in memory only (reset on service restart) - persistence optional

## Health & Liveness Endpoints

The system requires distinct endpoints for different health-check purposes, aligned with Kubernetes naming conventions.

### Current State

- **`/live`**: Frontend uses this for UI connectivity polling (60s interval) - **currently missing from backend**
- **`/health`**: Full health data for debug page (CPU, memory, circuit breakers) - **exists, used for detailed metrics**
- **No Kubernetes-standard endpoints**: `/livez`, `/readyz` are not implemented

### Endpoint Purpose Separation

| Endpoint | Purpose | Response | Use Case |
| -------- | ------- | -------- | -------- |
| `GET /livez` | Liveness check | `{"status":"ok"}` | UI connectivity polling + K8s liveness probe |
| `GET /readyz` | Readiness check | `{"status":"ok/not_ready"}` | K8s readiness probe (checks DB, scheduler) |
| `GET /health` | Detailed health metrics | Full CPU/memory/circuit breaker data | Debug page, monitoring dashboards |

### Migration

- Frontend will migrate from `/live` to `/livez`
- `/livez` serves both UI polling and Kubernetes liveness needs
- `/readyz` checks startup dependencies (database connected, scheduler running)

### Requirements

- **FR-043**: System MUST provide a lightweight `/livez` endpoint for liveness checks
- **FR-044**: The `/livez` endpoint MUST respond within 100ms under normal conditions
- **FR-045**: System MUST provide `/readyz` endpoint that verifies database and scheduler health
- **FR-046**: Health/liveness endpoints MUST NOT be consolidated with config endpoints
- **FR-047**: Frontend MUST migrate from `/live` to `/livez` for connectivity polling

## Current API Structure (for reference)

The following endpoints currently exist and may be consolidated:

| Endpoint | Purpose | Consolidation Target |
| -------- | ------- | -------------------- |
| `GET /api/v1/settings` | Runtime settings (log level, request logging) | Unified config endpoint |
| `PUT /api/v1/settings` | Update runtime settings | Unified config endpoint |
| `GET /api/v1/settings/info` | Settings metadata | Unified config endpoint |
| `GET /api/v1/settings/startup` | Startup config (read-only) | Unified config endpoint |
| `GET /api/v1/circuit-breakers/config` | Circuit breaker config | Unified config endpoint |
| `PUT /api/v1/circuit-breakers/config` | Update circuit breaker config | Unified config endpoint |
| `POST /api/v1/circuit-breakers/{name}/reset` | Reset specific breaker | Keep separate (action) |
| `POST /api/v1/circuit-breakers/reset` | Reset all breakers | Keep separate (action) |
| `GET /api/v1/features` | Feature flags | Unified config endpoint |
| `PUT /api/v1/features` | Update feature flags | Unified config endpoint |
| `GET /health` | Health metrics | Keep separate (detailed health) |
| `GET /live` | UI connectivity check | **Migrate to `/livez`** |
| `GET /livez` | Liveness probe | **New** - UI polling + K8s liveness |
| `GET /readyz` | Readiness probe | **New** - K8s readiness (checks DB/scheduler) |

**Proposed Consolidated Structure:**
- `GET /api/v1/config` - All configuration in single response
- `PUT /api/v1/config` - Update any runtime configuration
- `POST /api/v1/config/persist` - Save to config file
- `POST /api/v1/circuit-breakers/{name}/reset` - Keep (action endpoint)
- `POST /api/v1/circuit-breakers/reset` - Keep (action endpoint)
- `GET /health` - Keep (detailed health check, enhanced with circuit breaker stats)
- `GET /livez` - Keep (lightweight liveness for UI polling + K8s)
- `GET /readyz` - Keep (readiness probe checking DB/scheduler)

## Circuit Breaker Visualization Design

### Segmented Progress Bar

Each circuit breaker displays a horizontal bar representing total requests, segmented by outcome:

```
[████████████████░░░░░░░░░] 85% healthy
 ↑ green      ↑ red  ↑ orange
 (success)   (5xx)   (4xx)
```

Color segments (left to right):
- **Green**: Successful requests (2xx responses)
- **Red**: Server errors (5xx responses)
- **Orange**: Client errors (4xx responses)
- **Yellow**: Timeouts
- **Gray**: Network errors

### State Timeline

Visual representation of state transitions over time:

```
Closed ────────┐          ┌────── Closed
               │          │
         Open ─┴──────────┴─ Half-Open
               ↓          ↓
         (threshold)  (recovery)
```

### State Duration Summary

Pie chart or bar showing time distribution:
- Closed: 95.2% (22h 51m)
- Open: 3.1% (45m)
- Half-Open: 1.7% (24m)

### Current State Card

```
┌─────────────────────────────────────┐
│ logos                    [CLOSED]   │
│ ════════════════════════════════    │
│ [██████████████░░░░░░] 87.3%       │
│  ↑ 12,453 ok  ↑ 1,234 err (5xx)    │
│                                     │
│ In state: 2h 15m                    │
│ Failures: 0/5 threshold             │
│ Last transition: 14:32 → Closed     │
│                    (recovery)       │
│                                     │
│ [Reset] [Configure]                 │
└─────────────────────────────────────┘
```
