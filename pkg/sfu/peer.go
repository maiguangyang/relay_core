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

// Peer represents a participant in a room
type Peer struct {
	mu   sync.RWMutex
	id   string
	room *Room
	pc   *webrtc.PeerConnection

	// Local tracks added to this peer (for receiving RTP from forwarders)
	localTracks map[string]*webrtc.TrackLocalStaticRTP

	closed bool
}

// newPeer creates a new peer with a PeerConnection
func newPeer(id string, room *Room) (*Peer, error) {
	sfu := room.sfu

	// Create PeerConnection
	pc, err := sfu.getAPI().NewPeerConnection(sfu.getWebRTCConfig())
	if err != nil {
		return nil, err
	}

	peer := &Peer{
		id:          id,
		room:        room,
		pc:          pc,
		localTracks: make(map[string]*webrtc.TrackLocalStaticRTP),
	}

	// Set up event handlers
	peer.setupEventHandlers()

	return peer, nil
}

// ID returns the peer ID
func (p *Peer) ID() string {
	return p.id
}

// setupEventHandlers sets up WebRTC event handlers
func (p *Peer) setupEventHandlers() {
	// Handle incoming tracks (published by this peer)
	p.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		p.room.onTrackPublished(p, track, receiver)
	})

	// Handle ICE candidates
	p.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			p.room.onICECandidate(p, candidate)
		}
	})

	// Handle connection state changes
	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed:
			p.room.onError(p, ErrConnectionFailed)
		case webrtc.PeerConnectionStateClosed:
			// Connection closed
		case webrtc.PeerConnectionStateDisconnected:
			// Temporarily disconnected, may reconnect
		}
	})

	// Handle ICE connection state changes
	p.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed {
			p.room.onError(p, ErrICEFailed)
		}
	})
}

// HandleOffer processes an SDP offer and returns an answer
func (p *Peer) HandleOffer(offerSDP string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return "", ErrPeerClosed
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return "", err
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}

	return answer.SDP, nil
}

// CreateOffer creates an SDP offer (for renegotiation)
func (p *Peer) CreateOffer() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return "", ErrPeerClosed
	}

	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return "", err
	}

	return offer.SDP, nil
}

// HandleAnswer processes an SDP answer
func (p *Peer) HandleAnswer(answerSDP string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPeerClosed
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}

	return p.pc.SetRemoteDescription(answer)
}

// AddICECandidate adds an ICE candidate
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return ErrPeerClosed
	}

	return p.pc.AddICECandidate(candidate)
}

// AddTrack adds a local track to this peer for forwarding
func (p *Peer) AddTrack(track *webrtc.TrackLocalStaticRTP) (*webrtc.RTPSender, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrPeerClosed
	}

	sender, err := p.pc.AddTrack(track)
	if err != nil {
		return nil, err
	}

	p.localTracks[track.ID()] = track

	return sender, nil
}

// RemoveTrack removes a local track from this peer
func (p *Peer) RemoveTrack(trackID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPeerClosed
	}

	delete(p.localTracks, trackID)

	// Find and remove the sender
	for _, sender := range p.pc.GetSenders() {
		if sender.Track() != nil && sender.Track().ID() == trackID {
			return p.pc.RemoveTrack(sender)
		}
	}

	return nil
}

// GetPeerConnection returns the underlying PeerConnection
// Use with caution - mainly for advanced operations
func (p *Peer) GetPeerConnection() *webrtc.PeerConnection {
	return p.pc
}

// Close closes the peer connection
func (p *Peer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	pc := p.pc
	p.localTracks = nil
	p.mu.Unlock()

	if pc != nil {
		return pc.Close()
	}
	return nil
}

// IsClosed returns whether the peer is closed
func (p *Peer) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// ConnectionState returns the current connection state
func (p *Peer) ConnectionState() webrtc.PeerConnectionState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pc == nil {
		return webrtc.PeerConnectionStateClosed
	}
	return p.pc.ConnectionState()
}
