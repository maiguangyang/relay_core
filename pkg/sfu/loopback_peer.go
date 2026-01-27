package sfu

import (
	"fmt"
	"sync"

	"github.com/maiguangyang/relay_core/pkg/utils"
	"github.com/pion/webrtc/v4"
)

// LoopbackPeer represents a local PeerConnection for screen sharing loopback
type LoopbackPeer struct {
	pc       *webrtc.PeerConnection
	switcher *SourceSwitcher
	roomId   string
}

// NewLoopbackPeer creates a new LoopbackPeer instance
func NewLoopbackPeer(roomId string, switcher *SourceSwitcher) (*LoopbackPeer, error) {
	// Configure ICE to only use Host candidates (Loopback/LAN)
	// This ensures we don't leak traffic to STUN/TURN servers
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})

	// Create API with SettingEngine
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	// Configuration
	config := webrtc.Configuration{
		// Force Host Only policy as per spec NFR-004
		ICEServers:         []webrtc.ICEServer{},
		ICETransportPolicy: webrtc.ICETransportPolicyAll, // We filter via candidate preference or just relying on local discovery
	}

	// Create PeerConnection
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	peer := &LoopbackPeer{
		pc:       pc,
		switcher: switcher,
		roomId:   roomId,
	}

	// Set up handlers
	pc.OnTrack(peer.onTrack)
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		utils.Info("[LoopbackPeer] ICE Connection State for room %s has changed: %s", roomId, state.String())
	})

	return peer, nil
}

// HandleOffer accepts an Offer SDP and returns an Answer SDP
func (p *LoopbackPeer) HandleOffer(offerSdp string) (string, error) {
	// Set Remote Description
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSdp,
	}

	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	// Create Answer
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	// Set Local Description
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	// Wait for ICE Gathering to complete (since we are local, it should be fast)
	// For loopback, we can likely block or wait for a very short timeout.
	// Since we are using Host candidates, gathering is practically instantaneous.
	// But strictly speaking, we should wait.
	gatherComplete := webrtc.GatheringCompletePromise(p.pc)
	<-gatherComplete

	// Return the answer with candidates embedded
	finalAnswer := p.pc.LocalDescription()
	return finalAnswer.SDP, nil
}

// onTrack handles incoming media tracks from Dart
func (p *LoopbackPeer) onTrack(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	utils.Info("[LoopbackPeer] Remote track received: %s (Kind: %s) for room %s", remoteTrack.ID(), remoteTrack.Kind(), p.roomId)

	// Determine if it's video
	isVideo := remoteTrack.Kind() == webrtc.RTPCodecTypeVideo

	// Pipe to SourceSwitcher
	// Note: We need to modify SourceSwitcher to accept TrackRemote
	if p.switcher != nil {
		utils.Info("[LoopbackPeer] Injecting track into SourceSwitcher for room %s", p.roomId)

		// 自动切换到本地分享模式
		// 使用 "local-share" 作为 sharerID，或者更理想的是从信令中获取
		p.switcher.StartLocalShare("local-share", "", false)

		go p.switcher.InjectRemoteTrack(isVideo, remoteTrack)
	}
}

// Close closes the PeerConnection
func (p *LoopbackPeer) Close() error {
	utils.Info("[LoopbackPeer] Closing for room %s", p.roomId)
	if p.switcher != nil {
		p.switcher.StopLocalShare()
	}
	return p.pc.Close()
}

// ========================================
// Global Registry
// ========================================

var (
	loopbackPeers   = make(map[string]*LoopbackPeer)
	loopbackPeersMu = sync.Mutex{} // Using sync.Mutex directly or we can use sync.RWMutex
)

// GetLoopbackPeer returns the LoopbackPeer for a room
func GetLoopbackPeer(roomId string) *LoopbackPeer {
	loopbackPeersMu.Lock()
	defer loopbackPeersMu.Unlock()
	return loopbackPeers[roomId]
}

// CreateLoopbackPeer creates and registers a LoopbackPeer
func CreateLoopbackPeer(roomId string, switcher *SourceSwitcher) (*LoopbackPeer, error) {
	loopbackPeersMu.Lock()
	defer loopbackPeersMu.Unlock()

	if old, ok := loopbackPeers[roomId]; ok {
		utils.Warn("[LoopbackPeer] Cleaning up existing peer for room %s", roomId)
		old.Close()
		delete(loopbackPeers, roomId)
	}

	peer, err := NewLoopbackPeer(roomId, switcher)
	if err != nil {
		return nil, err
	}

	loopbackPeers[roomId] = peer
	return peer, nil
}

// DestroyLoopbackPeer destroys the LoopbackPeer for a room
func DestroyLoopbackPeer(roomId string) {
	loopbackPeersMu.Lock()
	defer loopbackPeersMu.Unlock()

	if peer, ok := loopbackPeers[roomId]; ok {
		peer.Close()
		delete(loopbackPeers, roomId)
	}
}
