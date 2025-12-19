# Feature Specification: tvarr-ffmpegd Distributed Transcoding

**Feature Branch**: `020-ffmpegd`
**Created**: 2025-12-19
**Status**: Draft
**Input**: User description: "Implement tvarr-ffmpegd as per TODO/ffmpegd.md. Standard tvarr container ships with ffmpeg/ffmpegd, spawning both processes. Separate tvarr-only (smaller) and ffmpegd-only images available. tvarr-ffmpegd is a separate binary with protobuf interfaces available for third-party clients, using pkg/* and cmd/tvarr-ffmpegd."

## Overview

Implement a distributed FFmpeg transcoding daemon (tvarr-ffmpegd) that separates transcoding from the main tvarr coordinator. This enables horizontal scaling of transcoding workloads across multiple machines with different hardware capabilities (GPU encoders, CPU cores).

**Key Design Principles**:
- **Separate binary**: `tvarr-ffmpegd` is built as `cmd/tvarr-ffmpegd`, distinct from `cmd/tvarr`
- **Public interfaces**: Protobuf definitions and client libraries in `pkg/` for third-party integration
- **Shared code**: Common FFmpeg/hwaccel logic reused from existing `internal/ffmpeg/` where appropriate

**Container Strategy**:
- **tvarr** (default, also tagged `ffmpeg-full`): Ships both tvarr and ffmpegd binaries, spawning both processes via entrypoint script (default experience)
- **tvarr-coordinator**: Minimal image with only tvarr binary, no FFmpeg (for users with remote transcoders)
- **tvarr-ffmpegd**: Same image as default but entrypoint only starts ffmpegd process (for dedicated transcoding nodes)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Default All-in-One Deployment (Priority: P1)

A user deploys tvarr using Docker with the standard image. The container automatically starts both the tvarr coordinator and a local ffmpegd instance, providing a complete transcoding solution without any additional configuration.

**Why this priority**: This is the default experience for most users. It must work out-of-the-box with zero configuration beyond what exists today.

**Independent Test**: Can be fully tested by deploying a single container and verifying that live transcoding works (channel relay with encoding profile applied).

**Acceptance Scenarios**:

1. **Given** a user runs the default tvarr container, **When** the container starts, **Then** both tvarr and ffmpegd processes are running and the local ffmpegd is registered with the coordinator.
2. **Given** tvarr-full container is running, **When** a user creates a relay session with an encoding profile, **Then** the local ffmpegd handles transcoding and the stream is served correctly.
3. **Given** tvarr-full container is running, **When** the user views the Transcoders dashboard, **Then** the local ffmpegd appears as a connected daemon with its capabilities displayed.

---

### User Story 2 - Distributed Transcoding with Remote Workers (Priority: P2)

A user runs tvarr-coordinator on a low-power NAS and deploys tvarr-ffmpegd containers on separate machines with GPU hardware. The coordinator automatically discovers and uses the remote transcoders based on their reported capabilities.

**Why this priority**: This is the key differentiating feature that enables hardware acceleration offloading and horizontal scaling.

**Independent Test**: Deploy tvarr-coordinator on machine A, tvarr-ffmpegd on machine B with GPU, verify transcoding jobs route to machine B.

**Acceptance Scenarios**:

1. **Given** tvarr-coordinator is running, **When** a tvarr-ffmpegd instance connects with coordinator URL configured, **Then** it appears in the daemon registry with reported capabilities.
2. **Given** multiple ffmpegd instances are registered with different capabilities, **When** a transcoding job requires a specific encoder (e.g., NVENC), **Then** the coordinator routes the job to an ffmpegd instance with that capability.
3. **Given** an ffmpegd instance becomes unavailable, **When** a new transcoding job arrives, **Then** the coordinator routes to an available daemon or applies the configured fallback policy.

---

### User Story 3 - GPU Session-Aware Load Balancing (Priority: P2)

The coordinator tracks GPU encoder session limits across the cluster and routes jobs to daemons with available capacity, preventing failures due to session exhaustion on consumer GPUs.

**Why this priority**: Critical for reliability - without session tracking, transcoding can fail with cryptic FFmpeg errors when GPU session limits are exceeded.

**Independent Test**: Run 5 concurrent transcoding jobs against an ffmpegd with 3-session GPU limit, verify 3 succeed on GPU and remaining jobs are handled according to fallback policy.

**Acceptance Scenarios**:

1. **Given** an ffmpegd reports max_encode_sessions=5 and active_encode_sessions=4, **When** a new GPU encoding job is requested, **Then** the coordinator routes to this daemon (1 session available).
2. **Given** all ffmpegd instances have exhausted GPU sessions, **When** a new encoding job is requested, **Then** the coordinator applies the configured policy (queue, fallback to software, or reject).
3. **Given** a transcoding job completes, **When** the daemon reports updated session counts via heartbeat, **Then** the coordinator's session tracking reflects the freed session.

---

### User Story 4 - Third-Party Client Integration (Priority: P3)

A developer builds a custom transcoding client using the public protobuf interfaces to integrate with tvarr-ffmpegd, either as an alternative coordinator or a specialized worker.

**Why this priority**: Enables ecosystem growth and advanced integrations, but not required for core functionality.

**Independent Test**: Build a minimal client using only the `pkg/` protobuf definitions, connect to ffmpegd, and successfully request a transcode operation.

**Acceptance Scenarios**:

1. **Given** the protobuf definitions are published in `pkg/ffmpegd/proto/`, **When** a developer generates client code, **Then** they can successfully connect to and communicate with tvarr-ffmpegd.
2. **Given** a third-party client sends a valid registration request, **When** ffmpegd receives it, **Then** ffmpegd responds with its capabilities and accepts the connection.

---

### User Story 5 - Transcoders Dashboard (Priority: P3)

Users can view all connected transcoding daemons, their capabilities, current workload, and system metrics through a dedicated dashboard page in the tvarr web UI.

**Why this priority**: Important for operations and troubleshooting, but the system must work without users needing to monitor it.

**Independent Test**: Connect 2 ffmpegd instances, navigate to Transcoders page, verify both appear with accurate stats.

**Acceptance Scenarios**:

1. **Given** ffmpegd daemons are connected, **When** user navigates to the Transcoders page, **Then** all daemons are listed with status, capabilities, and active jobs.
2. **Given** a daemon is actively transcoding, **When** user views daemon details, **Then** real-time stats (encoding speed, CPU, GPU utilization) are displayed.
3. **Given** the user wants to add remote workers, **When** user views the registration panel, **Then** the connection URL and auth token are displayed for configuration.

---

### User Story 6 - Standalone ffmpegd Deployment (Priority: P3)

A user deploys only tvarr-ffmpegd on a dedicated transcoding server, connecting it to an existing tvarr-coordinator instance.

**Why this priority**: Enables flexible deployment topologies for advanced users.

**Independent Test**: Deploy ffmpegd-only container with coordinator URL, verify it registers and accepts transcoding jobs.

**Acceptance Scenarios**:

1. **Given** user runs tvarr-ffmpegd container with coordinator URL configured, **When** container starts, **Then** only the ffmpegd process runs and registers with the coordinator.
2. **Given** ffmpegd-only container is running, **When** a transcoding job is routed to it, **Then** it processes the job and returns transcoded samples to the coordinator.

---

### Edge Cases

- What happens when coordinator restarts while ffmpegd instances are running? (ffmpegd reconnects automatically with exponential backoff)
- How does the system handle network partitions between coordinator and ffmpegd? (timeout detection via missed heartbeats, automatic reconnection, job reassignment if configured)
- What happens when ffmpegd crashes mid-transcode? (coordinator detects via heartbeat timeout, marks job failed, reassigns to another daemon if available)
- What happens when all ffmpegd instances are unavailable? (coordinator returns error indicating no transcoding capacity available)
- How does multi-process entrypoint handle one process crashing? (entrypoint monitors both processes, restarts failed process, logs event)
- What happens if ffmpegd binary is run without a coordinator URL? (daemon starts but logs warning about not being connected, useful for local testing)

## Requirements *(mandatory)*

### Functional Requirements

#### Separate Binary Architecture

- **FR-001**: System MUST provide a standalone `tvarr-ffmpegd` binary built from `cmd/tvarr-ffmpegd/`
- **FR-002**: Protobuf service definitions MUST be published in `pkg/ffmpegd/proto/` for third-party client generation
- **FR-003**: Generated protobuf code and client helpers MUST be available in `pkg/ffmpegd/`
- **FR-004**: tvarr-ffmpegd MUST reuse hardware acceleration detection from existing shared code

#### Core Daemon Functionality

- **FR-005**: tvarr-ffmpegd MUST register with the tvarr coordinator, reporting its capabilities (encoders, decoders, hardware acceleration, session limits)
- **FR-006**: tvarr-ffmpegd MUST send periodic heartbeats (default: every 5 seconds) with system stats, GPU metrics, and active job status
- **FR-007**: tvarr-ffmpegd MUST report GPU session limits (max and active encode/decode sessions) per GPU
- **FR-008**: tvarr-ffmpegd MUST accept bidirectional streaming of ES samples for transcoding operations
- **FR-009**: tvarr-ffmpegd MUST manage FFmpeg processes for transcoding (spawn, monitor, terminate)

#### Coordinator Integration

- **FR-010**: tvarr coordinator MUST maintain a registry of connected ffmpegd daemons with their capabilities
- **FR-011**: Coordinator MUST route transcoding jobs to daemons based on capability matching (required encoder, hardware acceleration)
- **FR-012**: Coordinator MUST track GPU session availability across the cluster to prevent session exhaustion
- **FR-013**: Coordinator MUST support configurable policies when GPU sessions are exhausted: queue, fallback to software encoding, or reject
- **FR-014**: Coordinator MUST detect daemon unavailability via missed heartbeats (default: 30-second threshold for unhealthy status)
- **FR-015**: Coordinator MUST expose a gRPC endpoint for daemon connections (configurable port)

#### Container Architecture

- **FR-016**: System MUST provide three container image variants: tvarr (default), tvarr-coordinator, tvarr-ffmpegd
- **FR-017**: Default tvarr image MUST contain both tvarr and tvarr-ffmpegd binaries plus FFmpeg (also tagged as `ffmpeg-full`)
- **FR-018**: Default tvarr image MUST use an entrypoint script that spawns and supervises both processes
- **FR-019**: Entrypoint script MUST restart failed processes and forward termination signals to both processes
- **FR-020**: tvarr-coordinator image MUST NOT include FFmpeg or encoder libraries (minimal size)
- **FR-021**: tvarr-ffmpegd image MUST be the same as default tvarr but with entrypoint that only starts ffmpegd process

#### Communication Protocol

- **FR-022**: Daemon-coordinator communication MUST use gRPC with bidirectional streaming
- **FR-023**: ES sample transport MUST support batching for efficient network transfer
- **FR-024**: System MUST support authentication between coordinator and daemons via shared token

#### Dashboard

- **FR-025**: System MUST provide a Transcoders page in the web UI showing all connected daemons
- **FR-026**: Dashboard MUST display daemon capabilities, active jobs, system metrics, and GPU utilization
- **FR-027**: Dashboard MUST provide registration info (coordinator URL, auth token) for adding remote workers

### Key Entities

- **Daemon**: A tvarr-ffmpegd instance with unique ID, human-readable name, version, capabilities, and connection state
- **Capability**: Hardware acceleration types available, supported encoders/decoders, GPU session limits, performance metrics
- **TranscodeJob**: An active transcoding session with source/target codec info, routed to a specific daemon
- **GPUSessionState**: Per-GPU tracking of encoder sessions (max limit, currently active, utilization metrics)
- **DaemonRegistry**: Coordinator's view of all connected daemons for routing decisions

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Default tvarr container works identically to current monolithic container for existing users (backward compatible)
- **SC-002**: Users can add remote transcoding capacity by deploying ffmpegd containers with single environment variable configuration
- **SC-003**: Coordinator correctly routes 100% of jobs to daemons with the required encoder capabilities
- **SC-004**: System prevents GPU session exhaustion failures by tracking session limits (zero FFmpeg errors due to session limits when tracking is active)
- **SC-005**: tvarr-coordinator image is at least 50% smaller than default tvarr image (no FFmpeg libraries)
- **SC-006**: ffmpegd reconnects to coordinator within 30 seconds after network interruption
- **SC-007**: Dashboard displays daemon stats that update within 5 seconds of changes
- **SC-008**: Multi-process entrypoint handles termination signals correctly, shutting down both processes within 10 seconds
- **SC-009**: Third-party developers can generate working client code from published protobuf definitions

## Assumptions

1. gRPC is the protocol for daemon-coordinator communication (per TODO/ffmpegd.md analysis)
2. GPU session limits are detectable via nvidia-smi/NVML, known model database, or user environment variable override
3. Initial authentication uses shared token (mTLS can be added in future iteration)
4. Default GPU exhaustion policy is "fallback to software encoding" with warning logged
5. Heartbeat interval of 5 seconds and unhealthy threshold of 30 seconds are appropriate defaults
6. The entrypoint script will use a simple shell-based process supervisor pattern
7. Existing `internal/ffmpeg/hwaccel.go` detection logic is moved/exported to `pkg/` for reuse

## Out of Scope

1. Multi-coordinator support (single coordinator per cluster initially)
2. Automatic DNS-SD/mDNS discovery of coordinator (static configuration via environment variable)
3. TLS encryption between coordinator and daemons (can be added in future iteration)
4. Web-based wizard for deploying remote ffmpegd instances
5. Performance benchmarking on daemon startup (report capabilities only, not encode speed testing)
6. GPU virtualization or partitioning (MIG, vGPU) - daemons see whole GPUs
