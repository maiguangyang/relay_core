/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package signaling

// MessageType represents the type of signaling message
type MessageType string

const (
	// MessageTypeOffer is an SDP offer
	MessageTypeOffer MessageType = "offer"
	// MessageTypeAnswer is an SDP answer
	MessageTypeAnswer MessageType = "answer"
	// MessageTypeCandidate is an ICE candidate
	MessageTypeCandidate MessageType = "candidate"
	// MessageTypePeerJoined indicates a peer joined the room
	MessageTypePeerJoined MessageType = "peer_joined"
	// MessageTypePeerLeft indicates a peer left the room
	MessageTypePeerLeft MessageType = "peer_left"
	// MessageTypeTrackAdded indicates a new track was added
	MessageTypeTrackAdded MessageType = "track_added"
	// MessageTypeError indicates an error occurred
	MessageTypeError MessageType = "error"
	// MessageTypeElection indicates proxy election
	MessageTypeElection MessageType = "election"
	// MessageTypeProxyChange indicates proxy has changed
	MessageTypeProxyChange MessageType = "proxy_change"
)

// Message represents a signaling message
type Message struct {
	Type    MessageType `json:"type"`
	RoomID  string      `json:"room_id,omitempty"`
	PeerID  string      `json:"peer_id,omitempty"`
	Payload string      `json:"payload,omitempty"` // SDP or ICE candidate JSON
}

// OfferMessage represents an SDP offer
type OfferMessage struct {
	SDP string `json:"sdp"`
}

// AnswerMessage represents an SDP answer
type AnswerMessage struct {
	SDP string `json:"sdp"`
}

// CandidateMessage represents an ICE candidate
type CandidateMessage struct {
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid,omitempty"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex,omitempty"`
	UsernameFragment *string `json:"usernameFragment,omitempty"`
}

// TrackInfo represents track information
type TrackInfo struct {
	TrackID  string `json:"track_id"`
	StreamID string `json:"stream_id"`
	Kind     string `json:"kind"` // "audio" or "video"
}

// ErrorMessage represents an error
type ErrorMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PeerInfo represents peer information
type PeerInfo struct {
	PeerID string      `json:"peer_id"`
	Tracks []TrackInfo `json:"tracks,omitempty"`
}

// RoomInfo represents room information
type RoomInfo struct {
	RoomID    string     `json:"room_id"`
	Peers     []PeerInfo `json:"peers"`
	ProxyID   string     `json:"proxy_id,omitempty"` // Current proxy peer
	PeerCount int        `json:"peer_count"`
}
