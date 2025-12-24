/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Relay Core - Embedded Micro SFU for RTP Forwarding
 * This is the main entry point for C-shared library exports.
 * All functions with //export comments are exposed to Dart FFI.
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
	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/signaling"
	"github.com/maiguangyang/relay_core/pkg/utils"
	"github.com/pion/webrtc/v4"
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
// Relay Instance Management
// ==========================================

//export RelayCreate
func RelayCreate() C.int64_t {
	config := sfu.DefaultConfig()
	sfuInstance := sfu.New(config)

	// Set up callbacks
	sfuInstance.SetOnPeerJoined(func(roomID, peerID string) {
		emitEvent(EventTypePeerJoined, roomID, peerID, "")
	})

	sfuInstance.SetOnPeerLeft(func(roomID, peerID string) {
		emitEvent(EventTypePeerLeft, roomID, peerID, "")
	})

	sfuInstance.SetOnTrackAdded(func(roomID, peerID, trackID string, kind string) {
		data, _ := json.Marshal(signaling.TrackInfo{
			TrackID: trackID,
			Kind:    kind,
		})
		emitEvent(EventTypeTrackAdded, roomID, peerID, string(data))
	})

	sfuInstance.SetOnError(func(roomID, peerID string, err error) {
		data, _ := json.Marshal(signaling.ErrorMessage{
			Code:    500,
			Message: err.Error(),
		})
		emitEvent(EventTypeError, roomID, peerID, string(data))
	})

	sfuInstance.SetOnICECandidate(func(roomID, peerID string, candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candidateJSON := candidate.ToJSON()
			data, _ := json.Marshal(signaling.CandidateMessage{
				Candidate:        candidateJSON.Candidate,
				SDPMid:           candidateJSON.SDPMid,
				SDPMLineIndex:    candidateJSON.SDPMLineIndex,
				UsernameFragment: candidateJSON.UsernameFragment,
			})
			emitEvent(EventTypeICECandidate, roomID, peerID, string(data))
		}
	})

	id := registerSFUInstance(sfuInstance)
	utils.Info("Relay created with ID: %d", id)
	return C.int64_t(id)
}

//export RelayCreateWithConfig
func RelayCreateWithConfig(iceServersJSON *C.char, debug C.int) C.int64_t {
	config := sfu.DefaultConfig()
	config.Debug = debug != 0

	if iceServersJSON != nil {
		var servers []webrtc.ICEServer
		if err := json.Unmarshal([]byte(C.GoString(iceServersJSON)), &servers); err == nil {
			config.ICEServers = servers
		}
	}

	sfuInstance := sfu.New(config)
	id := registerSFUInstance(sfuInstance)
	return C.int64_t(id)
}

//export RelayDestroy
func RelayDestroy(relayID C.int64_t) C.int {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.int(-1)
	}

	if err := sfuInstance.Close(); err != nil {
		utils.Error("Failed to close relay %d: %v", relayID, err)
		return C.int(-1)
	}

	unregisterSFUInstance(int64(relayID))
	utils.Info("Relay destroyed: %d", relayID)
	return C.int(0)
}

// ==========================================
// Room Management
// ==========================================

//export RoomCreate
func RoomCreate(relayID C.int64_t, roomID *C.char) C.int {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.int(-1)
	}

	goRoomID := C.GoString(roomID)
	_, err := sfuInstance.CreateRoom(goRoomID)
	if err != nil {
		utils.Error("Failed to create room %s: %v", goRoomID, err)
		return C.int(-1)
	}

	utils.Info("Room created: %s", goRoomID)
	return C.int(0)
}

//export RoomDestroy
func RoomDestroy(relayID C.int64_t, roomID *C.char) C.int {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.int(-1)
	}

	goRoomID := C.GoString(roomID)
	if err := sfuInstance.DestroyRoom(goRoomID); err != nil {
		utils.Error("Failed to destroy room %s: %v", goRoomID, err)
		return C.int(-1)
	}

	utils.Info("Room destroyed: %s", goRoomID)
	return C.int(0)
}

//export RoomList
func RoomList(relayID C.int64_t) *C.char {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.CString("[]")
	}

	rooms := sfuInstance.ListRooms()
	data, _ := json.Marshal(rooms)
	return C.CString(string(data))
}

//export RoomInfo
func RoomInfo(relayID C.int64_t, roomID *C.char) *C.char {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return nil
	}

	goRoomID := C.GoString(roomID)
	room := sfuInstance.GetRoom(goRoomID)
	if room == nil {
		return nil
	}

	info := signaling.RoomInfo{
		RoomID:    goRoomID,
		PeerCount: room.PeerCount(),
		Peers:     make([]signaling.PeerInfo, 0),
	}

	for _, peerID := range room.ListPeers() {
		info.Peers = append(info.Peers, signaling.PeerInfo{
			PeerID: peerID,
		})
	}

	// Get proxy if election is enabled
	elector := getElector(goRoomID)
	if elector != nil {
		info.ProxyID = elector.GetCurrentProxy()
	}

	data, _ := json.Marshal(info)
	return C.CString(string(data))
}

// ==========================================
// Peer Management
// ==========================================

//export PeerAdd
func PeerAdd(relayID C.int64_t, roomID *C.char, peerID *C.char, offerSDP *C.char) *C.char {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return nil
	}

	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)
	goOfferSDP := C.GoString(offerSDP)

	room := sfuInstance.GetRoom(goRoomID)
	if room == nil {
		// Auto-create room if not exists
		var err error
		room, err = sfuInstance.CreateRoom(goRoomID)
		if err != nil {
			utils.Error("Failed to create room %s: %v", goRoomID, err)
			return nil
		}
	}

	_, answerSDP, err := room.AddPeer(goPeerID, goOfferSDP)
	if err != nil {
		utils.Error("Failed to add peer %s to room %s: %v", goPeerID, goRoomID, err)
		return nil
	}

	utils.Info("Peer added: %s to room %s", goPeerID, goRoomID)
	return C.CString(answerSDP)
}

//export PeerRemove
func PeerRemove(relayID C.int64_t, roomID *C.char, peerID *C.char) C.int {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.int(-1)
	}

	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	room := sfuInstance.GetRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	if err := room.RemovePeer(goPeerID); err != nil {
		utils.Error("Failed to remove peer %s from room %s: %v", goPeerID, goRoomID, err)
		return C.int(-1)
	}

	// Also remove from election
	elector := getElector(goRoomID)
	if elector != nil {
		elector.RemoveCandidate(goPeerID)
	}

	utils.Info("Peer removed: %s from room %s", goPeerID, goRoomID)
	return C.int(0)
}

//export PeerList
func PeerList(relayID C.int64_t, roomID *C.char) *C.char {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.CString("[]")
	}

	goRoomID := C.GoString(roomID)
	room := sfuInstance.GetRoom(goRoomID)
	if room == nil {
		return C.CString("[]")
	}

	peers := room.ListPeers()
	data, _ := json.Marshal(peers)
	return C.CString(string(data))
}

// ==========================================
// Signaling Handlers
// ==========================================

//export HandleICECandidate
func HandleICECandidate(relayID C.int64_t, roomID *C.char, peerID *C.char, candidateJSON *C.char) C.int {
	sfuInstance := getSFUInstance(int64(relayID))
	if sfuInstance == nil {
		return C.int(-1)
	}

	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)
	goCandidateJSON := C.GoString(candidateJSON)

	room := sfuInstance.GetRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	var candidateMsg signaling.CandidateMessage
	if err := json.Unmarshal([]byte(goCandidateJSON), &candidateMsg); err != nil {
		utils.Error("Failed to parse ICE candidate: %v", err)
		return C.int(-1)
	}

	candidateInit := webrtc.ICECandidateInit{
		Candidate:        candidateMsg.Candidate,
		SDPMid:           candidateMsg.SDPMid,
		SDPMLineIndex:    candidateMsg.SDPMLineIndex,
		UsernameFragment: candidateMsg.UsernameFragment,
	}

	if err := room.AddICECandidate(goPeerID, candidateInit); err != nil {
		utils.Error("Failed to add ICE candidate for peer %s: %v", goPeerID, err)
		return C.int(-1)
	}

	return C.int(0)
}

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
			"proxy_id":  result.ProxyID,
			"score":     result.Score,
			"reason":    result.Reason,
			"timestamp": result.Timestamp.Unix(),
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
		"proxy_id":  result.ProxyID,
		"score":     result.Score,
		"reason":    result.Reason,
		"timestamp": result.Timestamp.Unix(),
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
		defer C.free(unsafe.Pointer(cMessage))
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

//export GetVersion
func GetVersion() *C.char {
	return C.CString("1.0.0")
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
