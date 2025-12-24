/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Relay Core - Embedded Micro SFU for RTP Forwarding
 * This is the main entry point for C-shared library exports.
 * All functions with //export comments are exposed to Dart FFI.
 *
 * Note: This is a streamlined version focused on Relay P2P forwarding.
 * Main functionality is in relay_room_ffi.go for P2P connection management.
 */
package main

/*
#include <stdlib.h>
#include <stdint.h>

// Callback function types for events
typedef void (*EventCallback)(int eventType, const char* roomId, const char* peerId, const char* data);
typedef void (*LogCallback)(int level, const char* message);

// Store the callbacks
static EventCallback eventCallback = NULL;
static LogCallback logCallback = NULL;

// Setter functions
static void setEventCallback(EventCallback cb) {
    eventCallback = cb;
}

static void setLogCallback(LogCallback cb) {
    logCallback = cb;
}

// Caller functions (to be called from Go)
static void callEventCallback(int eventType, const char* roomId, const char* peerId, const char* data) {
    if (eventCallback != NULL) {
        eventCallback(eventType, roomId, peerId, data);
    }
}

static void callLogCallback(int level, const char* message) {
    if (logCallback != NULL) {
        logCallback(level, message);
    }
}
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// Event types for callbacks
const (
	EventTypePeerJoined   = 1
	EventTypePeerLeft     = 2
	EventTypeTrackAdded   = 3
	EventTypeError        = 4
	EventTypeICECandidate = 5
	EventTypeProxyChange  = 6
	EventTypeAnswer       = 7
	EventTypeOffer        = 8
)

// ==========================================
// Election Management
// ==========================================

//export ElectionEnable
func ElectionEnable(relayID C.int64_t, roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	config := election.DefaultElectorConfig()
	elector := election.NewElector(goRoomID, config)

	elector.SetOnElection(func(result election.ElectionResult) {
		data, _ := json.Marshal(map[string]interface{}{
			"proxy_id":        result.ProxyID,
			"score":           result.Score,
			"device_type":     result.DeviceType,
			"connection_type": result.ConnectionType,
			"reason":          result.Reason,
			"timestamp":       result.Timestamp.Unix(),
		})
		emitEvent(EventTypeProxyChange, goRoomID, result.ProxyID, string(data))
	})

	registerElector(goRoomID, elector)
	elector.Start()

	utils.Info("Election enabled for room: %s", goRoomID)
	return C.int(0)
}

//export ElectionDisable
func ElectionDisable(relayID C.int64_t, roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	elector := getElector(goRoomID)
	if elector != nil {
		elector.Close()
		unregisterElector(goRoomID)
	}

	utils.Info("Election disabled for room: %s", goRoomID)
	return C.int(0)
}

//export ElectionUpdateCandidate
func ElectionUpdateCandidate(relayID C.int64_t, roomID *C.char, peerID *C.char, bandwidth C.int64_t, latency C.int64_t, packetLoss C.double) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	elector := getElector(goRoomID)
	if elector == nil {
		return C.int(-1)
	}

	elector.UpdateCandidate(election.Candidate{
		PeerID:     goPeerID,
		Bandwidth:  int64(bandwidth),
		Latency:    int64(latency),
		PacketLoss: float64(packetLoss),
	})

	return C.int(0)
}

//export ElectionTrigger
func ElectionTrigger(relayID C.int64_t, roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	elector := getElector(goRoomID)
	if elector == nil {
		return nil
	}

	result := elector.Elect()
	if result == nil {
		return nil
	}

	data, _ := json.Marshal(map[string]interface{}{
		"proxy_id":        result.ProxyID,
		"score":           result.Score,
		"device_type":     result.DeviceType,
		"connection_type": result.ConnectionType,
		"reason":          result.Reason,
		"timestamp":       result.Timestamp.Unix(),
	})

	return C.CString(string(data))
}

//export ElectionGetProxy
func ElectionGetProxy(relayID C.int64_t, roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	elector := getElector(goRoomID)
	if elector == nil {
		return nil
	}

	proxyID := elector.GetCurrentProxy()
	if proxyID == "" {
		return nil
	}

	return C.CString(proxyID)
}

// ==========================================
// Callback Registration
// ==========================================

//export SetEventCallback
func SetEventCallback(callback C.EventCallback) {
	C.setEventCallback(callback)
	utils.Info("Event callback registered")
}

//export SetLogCallback
func SetLogCallback(callback C.LogCallback) {
	C.setLogCallback(callback)

	// Also set the Go logger callback
	utils.SetCallback(func(level utils.LogLevel, message string) {
		cMessage := C.CString(message)
		// Do not free cMessage here; it must be freed by the Dart side to avoid Use-After-Free
		// in async callbacks.
		C.callLogCallback(C.int(level), cMessage)
	})

	utils.Info("Log callback registered")
}

//export SetLogLevel
func SetLogLevel(level C.int) {
	utils.SetLevel(utils.LogLevel(level))
}

// ==========================================
// Utility Functions
// ==========================================

//export FreeString
func FreeString(s *C.char) {
	C.free(unsafe.Pointer(s))
}

//export CleanupAll
func CleanupAll() {
	cleanupAllElectors()
	utils.Info("All resources cleaned up")
}

//export GetVersion
func GetVersion() *C.char {
	return C.CString("1.0.0-relay")
}

// emitEvent sends an event through the callback
func emitEvent(eventType int, roomID, peerID, data string) {
	cRoomID := C.CString(roomID)
	cPeerID := C.CString(peerID)
	cData := C.CString(data)

	defer C.free(unsafe.Pointer(cRoomID))
	defer C.free(unsafe.Pointer(cPeerID))
	defer C.free(unsafe.Pointer(cData))

	C.callEventCallback(C.int(eventType), cRoomID, cPeerID, cData)
}

// main is required but not used for c-shared library
func main() {}
