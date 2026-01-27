# Feature Specification: Local Screen Share Refactor (Loopback)

**Feature Branch**: `001-local-share-loopback`
**Created**: 2026-01-27
**Status**: Draft
**Input**: User description: "Execute cleanup of Bridge dead code, then use standard WebRTC Loopback PeerConnection: Let Dart layer create a PeerConnection, directly connect to Go layer's PeerConnection. This uses WebRTC stack to automatically handle AV transmission, which is standard and efficient."

## User Scenarios & Testing

### User Story 1 - Reliable Local Screen Sharing (Priority: P1)

As a presenter in a local network meeting, I want to share my screen with other participants on the LAN, so that they can see my content with low latency and high quality, without sending data through the internet.

**Why this priority**: Core functionality for the "Relay" (Local-First) product value proposition. The current implementation is broken/dead code.

**Independent Test**:
1.  Connect two devices to the same Wi-Fi (No Internet if possible to prove LAN path).
2.  Device A starts "Screen Share".
3.  Device B (Subscriber) sees Device A's screen.
4.  Latency is perceptible but low (< 200ms).

**Acceptance Scenarios**:

1.  **Given** I am in a Room with other LAN peers, **When** I click "Share Screen", **Then** my screen content is visible to other peers.
2.  **Given** I am sharing functionality, **When** I click "Stop Sharing", **Then** the stream stops for all peers and resources are released.
3.  **Given** poor Wi-Fi, **When** packet loss occurs, **Then** the video may degrade but should not crash or freeze permanently (WebRTC congestion control should handle this).

---

### User Story 2 - System Stability & Efficiency (Priority: P1)

As a user, I want the screen sharing feature to use reasonable system resources and release them fully when done, so that my device doesn't slow down or crash over time.

**Why this priority**: Previous architecture had memory leaks and "dead code" (UDP Bridge) that consumed resources without function.

**Independent Test**:
1.  Run the app with performance monitoring (Mem/CPU).
2.  Start/Stop screen share 10 times.
3.  Verify memory returns to baseline (allow for GC variance).

**Acceptance Scenarios**:

1.  **Given** the app is idle, **When** I start local screen share, **Then** CPU usage should not spike unreasonably (e.g., >50% increase for 1080p).
2.  **Given** I have stopped screen sharing, **When** I wait for 1 minute, **Then** no "zombie" threads or UDP ports remain open.

---

## Requirements

## Clarifications

### Session 2026-01-27
- Q: How should SDP Offer/Answer be exchanged between Dart and Go? → A: **Direct FFI Strings**. Use synchronous FFI calls to pass SDP strings.
- Q: Which ICE transport policy should be used for Loopback? → A: **Host Candidates Only**. Disable STUN/TURN for the loopback PC to ensure local-only path and fast connection.

### Functional Requirements

- **FR-001**: The System MUST remove all legacy "LocalShareBridge" code (UDP-based injection) from both Dart and Go layers to eliminate dead code and maintenance burden.
- **FR-002**: The System MUST implement a "Loopback Connection" mechanism where the Dart layer establishes a standard WebRTC PeerConnection with the Go Relay Core running in the same process.
- **FR-003**: The System MUST support standard SDP Offer/Answer negotiation for this Loopback Connection via **synchronous FFI string passing** (no WebSocket/HTTP).
- **FR-004**: The Go Relay Core MUST accept the Loopback Connection as a valid input source and forward its media tracks to the `RelayRoom` (P2P Mesh/SFU).
- **FR-005**: The System MUST automatically handle video codec negotiation (e.g., VP8/H.264) via the standard WebRTC handshake between Dart and Go.

### Constraints & Non-Functional Requirements

- **NFR-001**: **Standard protocols**: Communication between Dart and Go for media MUST use standard SRTP/RTP via the PeerConnection, not custom UDP packetization.
- **NFR-002**: **Zero FFI Data Plane**: Audio/Video data MUST NOT pass through Dart FFI (no `Uint8List` passing). It must flow directly from Flutter WebRTC native -> Localhost UDP -> Go WebRTC.
- **NFR-003**: **Resource cleanup**: All PeerConnections must be explicitly closed and disposed of when sharing stops.
- **NFR-004**: **ICE Strategy**: The Loopback connections MUST use `iceTransportPolicy: 'all'` (standard) but filter/prefer **Host Candidates** to ensure traffic stays local. To enforce security, Go listener SHOULD bind only to `127.0.0.1`.

### Key Entities

- **LoopbackPC (Dart)**: The `RTCPeerConnection` instance in Flutter that captures the screen and "publishes" it to localhost.
- **LoopbackReceiver (Go)**: The `WebRTCReceiver` in Go that answers the Dart connection and pipes the track to `SourceSwitcher`.

## Success Criteria

### Measurable Outcomes

- **SC-001**: **Functionality**: Local screen share works (Device B sees Device A) without internet connection.
- **SC-002**: **Code Health**: `LocalShareBridge` class and files are completely removed.
- **SC-003**: **Stability**: ZERO crash reports or "Duplicate Identity" errors during start/stop cycles.
- **SC-004**: **Memory**: Heap usage growth after 5 start/stop cycles is < 5MB (effectively zero leak).
