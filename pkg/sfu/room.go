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

// Room represents a conference room containing multiple peers
type Room struct {
	mu     sync.RWMutex
	id     string
	sfu    *SFU
	peers  map[string]*Peer
	closed bool

	// Published tracks that need to be forwarded to all subscribers
	// Key: trackID, Value: forwarder
	forwarders map[string]*Forwarder
}

// newRoom creates a new room
func newRoom(id string, sfu *SFU) *Room {
	return &Room{
		id:         id,
		sfu:        sfu,
		peers:      make(map[string]*Peer),
		forwarders: make(map[string]*Forwarder),
	}
}

// ID returns the room ID
func (r *Room) ID() string {
	return r.id
}

// AddPeer creates and adds a new peer to the room
// Returns the peer and answer SDP
func (r *Room) AddPeer(peerID string, offerSDP string) (*Peer, string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, "", ErrRoomClosed
	}

	// Check if peer already exists
	if existing, exists := r.peers[peerID]; exists {
		r.mu.Unlock()
		return existing, "", nil
	}
	r.mu.Unlock()

	// Create new peer
	peer, err := newPeer(peerID, r)
	if err != nil {
		return nil, "", err
	}

	// Process offer and get answer
	answer, err := peer.HandleOffer(offerSDP)
	if err != nil {
		peer.Close()
		return nil, "", err
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		peer.Close()
		return nil, "", ErrRoomClosed
	}
	r.peers[peerID] = peer

	// Subscribe this peer to all existing forwarders
	for _, fwd := range r.forwarders {
		if fwd.publisherID != peerID {
			fwd.AddSubscriber(peer)
		}
	}
	r.mu.Unlock()

	// Emit peer joined event
	r.sfu.emitPeerJoined(r.id, peerID)

	return peer, answer, nil
}

// GetPeer returns a peer by ID
func (r *Room) GetPeer(peerID string) *Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[peerID]
}

// RemovePeer removes a peer from the room
func (r *Room) RemovePeer(peerID string) error {
	r.mu.Lock()
	peer, exists := r.peers[peerID]
	if !exists {
		r.mu.Unlock()
		return nil
	}
	delete(r.peers, peerID)

	// Remove forwarders published by this peer
	for trackID, fwd := range r.forwarders {
		if fwd.publisherID == peerID {
			fwd.Close()
			delete(r.forwarders, trackID)
		} else {
			// Remove this peer as subscriber from other forwarders
			fwd.RemoveSubscriber(peerID)
		}
	}
	r.mu.Unlock()

	if peer != nil {
		peer.Close()
	}

	// Emit peer left event
	r.sfu.emitPeerLeft(r.id, peerID)

	return nil
}

// ListPeers returns all peer IDs
func (r *Room) ListPeers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.peers))
	for id := range r.peers {
		ids = append(ids, id)
	}
	return ids
}

// PeerCount returns the number of peers in the room
func (r *Room) PeerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// Close closes the room and all peers
func (r *Room) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	peers := make([]*Peer, 0, len(r.peers))
	for _, peer := range r.peers {
		peers = append(peers, peer)
	}
	r.peers = make(map[string]*Peer)

	forwarders := make([]*Forwarder, 0, len(r.forwarders))
	for _, fwd := range r.forwarders {
		forwarders = append(forwarders, fwd)
	}
	r.forwarders = make(map[string]*Forwarder)
	r.mu.Unlock()

	// Close all forwarders
	for _, fwd := range forwarders {
		fwd.Close()
	}

	// Close all peers
	for _, peer := range peers {
		peer.Close()
	}

	return nil
}

// onTrackPublished is called when a peer publishes a new track
func (r *Room) onTrackPublished(peer *Peer, track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	trackID := track.ID()

	// Create forwarder for this track
	fwd := NewForwarder(peer.id, track)
	r.forwarders[trackID] = fwd

	// Add all other peers as subscribers
	for id, p := range r.peers {
		if id != peer.id {
			fwd.AddSubscriber(p)
		}
	}

	// Start forwarding
	go fwd.Start()

	// Emit track added event
	r.sfu.emitTrackAdded(r.id, peer.id, trackID, track.Kind().String())
}

// onICECandidate is called when a peer generates an ICE candidate
func (r *Room) onICECandidate(peer *Peer, candidate *webrtc.ICECandidate) {
	r.sfu.emitICECandidate(r.id, peer.id, candidate)
}

// onError is called when an error occurs
func (r *Room) onError(peer *Peer, err error) {
	r.sfu.emitError(r.id, peer.id, err)
}

// AddICECandidate adds an ICE candidate to a peer
func (r *Room) AddICECandidate(peerID string, candidate webrtc.ICECandidateInit) error {
	peer := r.GetPeer(peerID)
	if peer == nil {
		return ErrPeerNotFound
	}
	return peer.AddICECandidate(candidate)
}
