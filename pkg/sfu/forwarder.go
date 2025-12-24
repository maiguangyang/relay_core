/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package sfu

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// Forwarder handles RTP packet forwarding from one track to multiple subscribers
// This is the core component for pure RTP relay without decoding
type Forwarder struct {
	mu          sync.RWMutex
	publisherID string
	remoteTrack *webrtc.TrackRemote

	// Subscribers: peerID -> localTrack
	subscribers map[string]*subscriberInfo

	closed  int32 // atomic
	closeCh chan struct{}

	// Statistics
	packetsForwarded uint64
	bytesForwarded   uint64
}

// subscriberInfo holds subscriber's track info
type subscriberInfo struct {
	peer       *Peer
	localTrack *webrtc.TrackLocalStaticRTP
}

// NewForwarder creates a new forwarder for the given remote track
func NewForwarder(publisherID string, remoteTrack *webrtc.TrackRemote) *Forwarder {
	return &Forwarder{
		publisherID: publisherID,
		remoteTrack: remoteTrack,
		subscribers: make(map[string]*subscriberInfo),
		closeCh:     make(chan struct{}),
	}
}

// AddSubscriber adds a peer as subscriber to this forwarder
func (f *Forwarder) AddSubscriber(peer *Peer) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if atomic.LoadInt32(&f.closed) == 1 {
		return ErrForwarderClosed
	}

	if _, exists := f.subscribers[peer.ID()]; exists {
		return nil // Already subscribed
	}

	// Create local track with same codec as remote track
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		f.remoteTrack.Codec().RTPCodecCapability,
		f.remoteTrack.ID(),
		f.remoteTrack.StreamID(),
	)
	if err != nil {
		return err
	}

	// Add track to peer's connection
	_, err = peer.AddTrack(localTrack)
	if err != nil {
		return err
	}

	f.subscribers[peer.ID()] = &subscriberInfo{
		peer:       peer,
		localTrack: localTrack,
	}

	return nil
}

// RemoveSubscriber removes a peer from subscribers
func (f *Forwarder) RemoveSubscriber(peerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	info, exists := f.subscribers[peerID]
	if !exists {
		return
	}

	// Remove track from peer
	if info.peer != nil && info.localTrack != nil {
		info.peer.RemoveTrack(info.localTrack.ID())
	}

	delete(f.subscribers, peerID)
}

// Start begins the RTP forwarding loop
// This is the core forwarding logic - reads RTP packets and writes to all subscribers
// NO DECODING is performed - pure relay for maximum performance
func (f *Forwarder) Start() {
	defer f.cleanup()

	for {
		select {
		case <-f.closeCh:
			return
		default:
		}

		// Read RTP packet from remote track
		// This does NOT decode the packet - it's raw RTP data
		rtpPacket, _, err := f.remoteTrack.ReadRTP()
		if err != nil {
			if err == io.EOF {
				return
			}
			// Track ended or error
			return
		}

		// Forward to all subscribers
		f.forwardPacket(rtpPacket)
	}
}

// forwardPacket sends RTP packet to all subscribers
func (f *Forwarder) forwardPacket(pkt *rtp.Packet) {
	f.mu.RLock()
	subscribers := make([]*subscriberInfo, 0, len(f.subscribers))
	for _, info := range f.subscribers {
		subscribers = append(subscribers, info)
	}
	f.mu.RUnlock()

	for _, info := range subscribers {
		if info.localTrack != nil && info.peer != nil && !info.peer.IsClosed() {
			// Write RTP packet directly - no transcoding, pure relay
			if err := info.localTrack.WriteRTP(pkt); err != nil {
				// Log error but continue forwarding to other subscribers
				continue
			}
			atomic.AddUint64(&f.packetsForwarded, 1)
			atomic.AddUint64(&f.bytesForwarded, uint64(pkt.MarshalSize()))
		}
	}
}

// Close stops the forwarder
func (f *Forwarder) Close() {
	if !atomic.CompareAndSwapInt32(&f.closed, 0, 1) {
		return // Already closed
	}
	close(f.closeCh)
}

// cleanup removes all subscribers
func (f *Forwarder) cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for peerID, info := range f.subscribers {
		if info.peer != nil && info.localTrack != nil {
			info.peer.RemoveTrack(info.localTrack.ID())
		}
		delete(f.subscribers, peerID)
	}
}

// Stats returns forwarding statistics
func (f *Forwarder) Stats() (packetsForwarded, bytesForwarded uint64) {
	return atomic.LoadUint64(&f.packetsForwarded), atomic.LoadUint64(&f.bytesForwarded)
}

// SubscriberCount returns the number of subscribers
func (f *Forwarder) SubscriberCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.subscribers)
}

// TrackID returns the track ID being forwarded
func (f *Forwarder) TrackID() string {
	return f.remoteTrack.ID()
}

// TrackKind returns the track kind (audio/video)
func (f *Forwarder) TrackKind() webrtc.RTPCodecType {
	return f.remoteTrack.Kind()
}
