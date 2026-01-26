# Relay Core Constitution

> **Master Directive**: This document is the supreme architectural guideline for the Relay Core project. All AI-generated code, refactoring, and architectural decisions **MUST** adhere to these principles.

## I. Core Philosophy (The "Why")

### 1. The "Brain & Muscle" Split
*   **Dart (Flutter) is the Brain**: Handles UI, Signaling, Business Logic, and User Interactions.
*   **Go (Pion) is the Muscle**: Handles RTP forwarding, Networking (UDP/TCP), Congestion Control, and heavy computation.
*   **Rule**: Never do heavy media processing in Dart. Never do complex UI state management in Go.

### 2. Local-First & P2P Priority
*   The system is designed to save bandwidth and reduce latency.
*   **Priority 1**: Direct P2P (LAN).
*   **Priority 2**: Relay via Go Core (LAN).
*   **Priority 3**: Cloud SFU (Internet/Fallback).

### 3. Stability & Failover
*   The Relay system is dynamic. Devices may join/leave/crash at any time.
*   **Code Principle**: Fail-fast and self-heal. If a Relay node dies, the `AutoCoordinator` must verify and elect a new one within seconds.
*   **Resource Safety**: Strict cleanup of Native/Go resources (memory, sockets, ports) is non-negotiable. Leak zero bytes.

---

## II. Architecture & Implementation Rules

### 1. FFI Boundary (The Danger Zone)
*   **String Encoding**: Always use `toNativeUtf8()` in Dart and `C.GoString` in Go.
*   **Memory Management**:
    *   **Dart -> Go**: Dart allocates, Dart frees. Use `try/finally` blocks in Dart to ensuring `calloc.free` is *always* called for C-Strings passed to Go.
    *   **Go -> Dart**: Go returns `C.CString`, Dart converts to String, then Dart calls a dedicated `FreeString` FFI function to let Go free the memory.
*   **Zero Copy Goal**: For high-frequency data (RTP packets), avoid copying where possible. Use pointers or OS-socket bridges (like the UDP Loopback architecture) instead of passing `Uint8List` through FFI if frequency > 60Hz.

### 2. Go Core Development
*   **Concurrency**: Use `sync.RWMutex` for all map access (`relayRooms`, `localShareBridges`).
*   **Atomic Stats**: Use `sync/atomic` for all high-frequency counters (packets sent/received).
*   **Context Management**: Every potentially blocking operation (network I/O) must have a timeout or be interruptible via a `closed` channel.

### 3. Flutter/Dart Development
*   **State Management**: Use `AutoCoordinator` as the singleton source of truth for Relay state.
*   **UI Updates**: Listen to `AutoCoordinator` streams/callbacks. Do not poll.
*   **Asset Management**: Call `dispose()` / `stop()` explicitly on all FFI bridges (`LiveKitBridge`, `RelayRoom`) when pages unmount.

---

## III. Development Workflow

### 1. The "Rebuild" Cycle
When modifying **ANY** Go code (`pkg/` or `*.go`):
1.  **Modify Go Code**.
2.  **Run Tests**: `go test ./pkg/...`
3.  **Rebuild Native Libs**:
    ```bash
    cd relay_core
    ./build_all.sh  # Or make macos/android/ios specifically
    ```
4.  **Run Flutter**: `flutter run`
*   *Attempting to run Flutter without rebuilding Go libs will use the old binary and cause confusing bugs.*

---

## IV. Naming Conventions

*   **Go structs**: `UpperCamelCase` (Exported), `lowerCamelCase` (Private).
*   **FFI Functions**: `PascalCase` (e.g., `RelayRoomCreate`).
*   **Dart Classes**: `PascalCase` (e.g., `RelayRoom`).
*   **Log Tags**: `[RelayCore]`, `[AutoCoord]`, `[FFI]`.

---

## V. Governance

1.  **Strict Typing**: No `dynamic` in core logic.
2.  **Documentation**: All FFI exported functions must have comments explaining params and return values (0 = Success, -1 = Fail).
3.  **Dependencies**: Keep `go.mod` and `pubspec.yaml` minimal. Do not add heavy dependencies without justification.

**Version**: 1.0.0 | **Ratified**: 2026-01-27
