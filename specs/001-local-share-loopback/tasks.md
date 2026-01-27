<!--
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-27 08:08:48
-->
# Tasks: Local Share Refactor (Loopback)

**Feature**: Local Share Loopback
**Status**: Ready
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

## Success Criteria Checklist
- [ ] SC-001: Local screen share works (Device B sees Device A) without internet.
- [ ] SC-002: `LocalShareBridge` class and files are completely removed.
- [ ] SC-003: ZERO crash reports or "Duplicate Identity" errors.
- [ ] SC-004: Heap usage growth < 5MB after start/stop cycles.

## Phase 1: Setup & Infrastructure
**Goal**: Prepare environment and define interfaces.

- [x] T001 Verify Go build environment and native lib paths.
- [x] T002 Update `local_share_bridge_ffi.go` to define `LocalShareConnect` FFI signature (stub).

## Phase 2: Foundational (Go Core)
**Goal**: Implement the Go-side WebRTC receiver and loopback logic.

- [x] T003 Create `pkg/sfu/loopback_peer.go` to handle `webrtc.PeerConnection` and SDP negotiation.
- [x] T004 Modify `pkg/sfu/source_switcher.go` to accept `*webrtc.TrackRemote` instead of UDP packets.
- [x] T005 Implement `LocalShareConnect` in `local_share_bridge_ffi.go` to wire up FFI to `loopback_peer`.
- [x] T006 Rebuild native libraries (`./build_all.sh`) to expose new symbols.

## Phase 3: [US1] Reliable Local Screen Sharing (P1)
**Goal**: Implement Dart-side logic to capture screen and connect to Go loopback.

- [x] T007 [US1] Update `lib/src/bindings/bindings.dart` with `LocalShareConnect` signature.
- [x] T008 [US1] Create `lib/src/media/local_loopback_peer.dart` implementing `RTCPeerConnection` and screen capture.
- [x] T009 [US1] Modify `home_page.dart` to use `LocalLoopbackPeer` for screen sharing.
- [ ] T010 [US1] Verify signaling flow (Dart Offer -> Go Answer) and ICE state (Host candidates).

## Phase 4: [US2] System Stability & Efficiency (P1)
**Goal**: Clean up dead code and verify resource stability.

- [ ] T011 [US2] Delete `lib/src/media/local_share_bridge.dart` (Legacy Dead Code).
- [x] T012 [US2] Verify build and ensure no regressions in `home_page.dart`./stop cycle (Manual Test).
- [x] T013 [US2] Verify compilation of full project after deletion.

- [ ] T014 [US2] Verify memory usage during start/stop cycle (Manual Test).

## Implementation Strategy
- **Step 1**: Implement Go Core (T003-T006). This provides the "Server" for the loopback.
- **Step 2**: Implement Dart Client (T007-T009). This provides the content.
- **Step 3**: Cleanup (T011-T012). Remove the old bridge once the new one works (or before, if safer).
- **Strategy**: Will delete legacy files *after* new implementation to ensure reference for porting logic if needed, but T011/T012 can be done early if preferred. Given it's "dead code", early deletion is also fine. I'll stick to the phase order.
