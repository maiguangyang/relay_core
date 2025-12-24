/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

// Config holds SFU configuration
type Config struct {
	// ICE servers for WebRTC connections
	ICEServers []webrtc.ICEServer
	// Enable debug logging
	Debug bool
}

// DefaultConfig returns default SFU configuration
func DefaultConfig() Config {
	return Config{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
		Debug: false,
	}
}

// SFU is the main Selective Forwarding Unit engine
// It manages rooms and handles RTP packet forwarding without decoding
type SFU struct {
	mu     sync.RWMutex
	config Config
	rooms  map[string]*Room
	api    *webrtc.API

	// Callbacks
	onPeerJoined   func(roomID, peerID string)
	onPeerLeft     func(roomID, peerID string)
	onTrackAdded   func(roomID, peerID, trackID string, kind string)
	onError        func(roomID, peerID string, err error)
	onICECandidate func(roomID, peerID string, candidate *webrtc.ICECandidate)
}

// New creates a new SFU instance
func New(config Config) *SFU {
	// Create media engine with default codecs
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	// Create API with media engine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	return &SFU{
		config: config,
		rooms:  make(map[string]*Room),
		api:    api,
	}
}

// SetOnPeerJoined sets callback for peer joined event
func (s *SFU) SetOnPeerJoined(fn func(roomID, peerID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPeerJoined = fn
}

// SetOnPeerLeft sets callback for peer left event
func (s *SFU) SetOnPeerLeft(fn func(roomID, peerID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPeerLeft = fn
}

// SetOnTrackAdded sets callback for track added event
func (s *SFU) SetOnTrackAdded(fn func(roomID, peerID, trackID string, kind string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTrackAdded = fn
}

// SetOnError sets callback for error event
func (s *SFU) SetOnError(fn func(roomID, peerID string, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = fn
}

// SetOnICECandidate sets callback for ICE candidate event
func (s *SFU) SetOnICECandidate(fn func(roomID, peerID string, candidate *webrtc.ICECandidate)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onICECandidate = fn
}

// CreateRoom creates a new room with the given ID
func (s *SFU) CreateRoom(roomID string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[roomID]; exists {
		return s.rooms[roomID], nil
	}

	room := newRoom(roomID, s)
	s.rooms[roomID] = room
	return room, nil
}

// GetRoom returns a room by ID
func (s *SFU) GetRoom(roomID string) *Room {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rooms[roomID]
}

// DestroyRoom removes a room and disconnects all peers
func (s *SFU) DestroyRoom(roomID string) error {
	s.mu.Lock()
	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.Unlock()
		return nil
	}
	delete(s.rooms, roomID)
	s.mu.Unlock()

	return room.Close()
}

// ListRooms returns all room IDs
func (s *SFU) ListRooms() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.rooms))
	for id := range s.rooms {
		ids = append(ids, id)
	}
	return ids
}

// Close shuts down the SFU and all rooms
func (s *SFU) Close() error {
	s.mu.Lock()
	rooms := make([]*Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room)
	}
	s.rooms = make(map[string]*Room)
	s.mu.Unlock()

	for _, room := range rooms {
		room.Close()
	}
	return nil
}

// emitPeerJoined triggers peer joined callback
func (s *SFU) emitPeerJoined(roomID, peerID string) {
	s.mu.RLock()
	fn := s.onPeerJoined
	s.mu.RUnlock()
	if fn != nil {
		fn(roomID, peerID)
	}
}

// emitPeerLeft triggers peer left callback
func (s *SFU) emitPeerLeft(roomID, peerID string) {
	s.mu.RLock()
	fn := s.onPeerLeft
	s.mu.RUnlock()
	if fn != nil {
		fn(roomID, peerID)
	}
}

// emitTrackAdded triggers track added callback
func (s *SFU) emitTrackAdded(roomID, peerID, trackID string, kind string) {
	s.mu.RLock()
	fn := s.onTrackAdded
	s.mu.RUnlock()
	if fn != nil {
		fn(roomID, peerID, trackID, kind)
	}
}

// emitError triggers error callback
func (s *SFU) emitError(roomID, peerID string, err error) {
	s.mu.RLock()
	fn := s.onError
	s.mu.RUnlock()
	if fn != nil {
		fn(roomID, peerID, err)
	}
}

// emitICECandidate triggers ICE candidate callback
func (s *SFU) emitICECandidate(roomID, peerID string, candidate *webrtc.ICECandidate) {
	s.mu.RLock()
	fn := s.onICECandidate
	s.mu.RUnlock()
	if fn != nil {
		fn(roomID, peerID, candidate)
	}
}

// getWebRTCConfig returns WebRTC configuration
func (s *SFU) getWebRTCConfig() webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers: s.config.ICEServers,
	}
}

// getAPI returns the WebRTC API
func (s *SFU) getAPI() *webrtc.API {
	return s.api
}
