# Implementation Plan - Local Share Refactor (Loopback)

**Feature**: Local Share Loopback
**Status**: Planning
**Spec**: [spec.md](./spec.md)

## Technical Context

### Architecture Analysis
The current "LocalShareBridge" (UDP Injection) is non-functional dead code. We are replacing it with a standard "Loopback PeerConnection" architecture:
1.  **Dart** acts as `Publisher`: Creates `RTCPeerConnection`, captures screen stream, adds track, creates Offer.
2.  **Go** acts as `Subscriber`: Receives Offer via FFI, creates `WebRTCReceiver`, sets Remote Description, creates Answer, and pipes the track to `SourceSwitcher`.
3.  **Signaling**: Direct FFI string passing (`RelayLocalConnect`).
4.  **Transport**: ICE Host Candidates (Localhost).

### Dependencies
- `flutter_webrtc` (Dart): Standard plugin, no changes needed.
- `pion/webrtc` (Go): Standard library, using `NewPeerConnection`.
- `ffi` (Dart): For signaling.

### Risks & Mitigations
- **Risk**: Connection failure due to firewall/VPN interfering with loopback.
    - *Mitigation*: Force ICE candidate filtering to `127.0.0.1` if possible, or use `pion`'s `SettingEngine` to whitelist loopback.
- **Risk**: Memory leaks if PC is not closed.
    - *Mitigation*: Strict `try/finally` resource management in Dart and Go.

## Constitution Check

| Rule | Compliance Strategy | Status |
|---|---|---|
| **Brain & Muscle** | Dart handles UI/Signaling (Offer/Answer), Go handles Forwarding (RTP piping). | ✅ |
| **FFI Safety** | Using `FreeString` for Go->Dart. | ✅ |
| **Zero Copy** | Media flows via PeerConnection (Internal Sockets), not FFI. | ✅ |
| **Go Rebuild** | Will update `build_all.sh` or remind user to run it. | ✅ |

## Phase 0: Research & Design

### Resolved Clarifications (from Spec)
- **Signaling**: Direct FFI Strings.
- **ICE Policy**: Host Candidates Only.

### Design Decisions
1.  **FFI Interface**:
    - `LocalShareConnect(roomId, offerSdp) -> answerSdp` (Synchronous-ish or Async string return).
    - If `pion` takes time to gather candidates, we may need a callback or a polling mechanic.
    - *Refinement*: Pion can gather candidates trickly or all-at-once. For loopback, it's fast. Swithcing to "Blocking FFI" might freeze UI.
    - *Decision*: Simple Blocking FFI is risky if ICE gathering takes >10ms.
    - *Better*: `LocalShareStart(roomId)` -> Go starts PC, returns SDPOffer/Answer?
    - *Standard*: Dart creates Offer -> Go sets remote -> Go returns Answer.
    - *Blocking Check*: Localhost candidates gathering is < 5ms usually. Blocking FFI is acceptable for V1.

## Phase 1: Implementation Steps

### 1. Go Core Modification
- [ ] Create `pkg/sfu/loopback_peer.go`: Handles the Pion PeerConnection.
- [ ] Update `pkg/sfu/source_switcher.go`: Accept `*webrtc.TrackRemote` instead of UDP packets.
- [ ] Update `local_share_bridge_ffi.go`: Replace legacy methods with `LocalShareConnect`.

### 2. Dart Layer Modification
- [ ] Create `local_loopback_peer.dart`: Manages `RTCPeerConnection` and screen capture.
- [ ] Update `home_page.dart`: Use `LocalLoopbackPeer` instead of `LocalShareBridge`.

### 3. Cleanup
- [ ] Remove `LocalShareBridge` (Dart & Go).

## Phase 2: Verification
- [ ] Compile Go & Rebuild Libs.
- [ ] Run Flutter.
- [ ] Start Screen Share (OFFLINE mode).
- [ ] Verify video on second device.
