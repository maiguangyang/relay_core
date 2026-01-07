/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Relay Room - 局域网代理转发专用房间
 * 实现 Relay 模式下的 P2P 连接管理
 * Relay 节点作为"广播站"，将 SourceSwitcher 的 Track 转发给所有订阅者
 */
package sfu

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maiguangyang/relay_core/pkg/utils"
	"github.com/pion/webrtc/v4"
)

// SubscriberState 订阅者连接状态
type SubscriberState int

const (
	SubscriberStateConnecting SubscriberState = iota
	SubscriberStateConnected
	SubscriberStateDisconnected
	SubscriberStateFailed
)

func (s SubscriberState) String() string {
	switch s {
	case SubscriberStateConnecting:
		return "connecting"
	case SubscriberStateConnected:
		return "connected"
	case SubscriberStateDisconnected:
		return "disconnected"
	case SubscriberStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Subscriber 订阅者 - 从 Relay 接收流的 Peer
type Subscriber struct {
	mu sync.RWMutex

	id    string
	pc    *webrtc.PeerConnection
	state SubscriberState

	// 发送器
	videoSender *webrtc.RTPSender
	audioSender *webrtc.RTPSender

	// 统计
	bytesSent    uint64
	packetsSent  uint64
	lastActivity time.Time

	closed bool
}

// RelayRoom 代理房间 - 管理 Relay 到订阅者的连接
type RelayRoom struct {
	mu sync.RWMutex

	id     string
	api    *webrtc.API
	config webrtc.Configuration

	// 源切换器
	switcher *SourceSwitcher

	// 订阅者列表
	subscribers map[string]*Subscriber

	// 状态
	isRelay     bool   // 本机是否是 Relay
	relayPeerID string // Relay 节点的 ID

	// 回调
	onSubscriberJoined func(roomID, peerID string)
	onSubscriberLeft   func(roomID, peerID string)
	onICECandidate     func(roomID, peerID string, candidate *webrtc.ICECandidate)
	onNeedRenegotiate  func(roomID, peerID string, offer string)
	onError            func(roomID, peerID string, err error)
	onKeyframeRequest  func(roomID string) // 请求关键帧回调

	// PLI 节流
	lastPLIRequest time.Time

	closed bool
}

// RelayRoomOption 配置选项
type RelayRoomOption func(*RelayRoom)

// WithWebRTCAPI 设置自定义 WebRTC API (用于测试或自定义配置)
func WithWebRTCAPI(api *webrtc.API) RelayRoomOption {
	return func(r *RelayRoom) {
		r.api = api
	}
}

// WithSourceSwitcher 使用外部的 SourceSwitcher（用于与 ProxyModeCoordinator 共享）
// 这确保 LiveKitBridge 和 RelayRoom 使用同一个 SourceSwitcher 实例
func WithSourceSwitcher(ss *SourceSwitcher) RelayRoomOption {
	return func(r *RelayRoom) {
		r.switcher = ss
	}
}

// NewRelayRoom 创建代理房间
func NewRelayRoom(id string, iceServers []webrtc.ICEServer, opts ...RelayRoomOption) (*RelayRoom, error) {
	room := &RelayRoom{
		id:          id,
		subscribers: make(map[string]*Subscriber),
		config: webrtc.Configuration{
			ICEServers: iceServers,
		},
	}

	// 先应用选项（包括 WithSourceSwitcher）
	for _, opt := range opts {
		opt(room)
	}

	// 只有在没有通过选项提供 SourceSwitcher 时才创建新的
	if room.switcher == nil {
		switcher, err := NewSourceSwitcher(id)
		if err != nil {
			return nil, err
		}
		room.switcher = switcher
	}

	// 在 switcher 上注册回调（无论是内部创建的还是外部传入的）
	room.switcher.SetOnTrackChanged(func(videoTrack, audioTrack *webrtc.TrackLocalStaticRTP) {
		room.UpdateTracks(videoTrack, audioTrack)
	})

	// 如果没有设置 API，使用默认的
	if room.api == nil {
		m := &webrtc.MediaEngine{}
		if err := m.RegisterDefaultCodecs(); err != nil {
			return nil, err
		}
		room.api = webrtc.NewAPI(webrtc.WithMediaEngine(m))
	}

	return room, nil
}

// SetCallbacks 设置回调
func (r *RelayRoom) SetCallbacks(
	onJoined func(roomID, peerID string),
	onLeft func(roomID, peerID string),
	onICE func(roomID, peerID string, candidate *webrtc.ICECandidate),
	onRenegotiate func(roomID, peerID string, offer string),
	onError func(roomID, peerID string, err error),
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onSubscriberJoined = onJoined
	r.onSubscriberLeft = onLeft
	r.onICECandidate = onICE
	r.onNeedRenegotiate = onRenegotiate
	r.onError = onError
}

// SetKeyframeRequestCallback 设置关键帧请求回调
// 当新订阅者加入时触发，用于向 SFU 请求关键帧
func (r *RelayRoom) SetKeyframeRequestCallback(fn func(roomID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onKeyframeRequest = fn
}

// GetSourceSwitcher 返回源切换器
func (r *RelayRoom) GetSourceSwitcher() *SourceSwitcher {
	return r.switcher
}

// BecomeRelay 成为 Relay 节点
func (r *RelayRoom) BecomeRelay(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isRelay = true
	r.relayPeerID = peerID
}

// IsRelay 是否是 Relay 节点
func (r *RelayRoom) IsRelay() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isRelay
}

// AddSubscriber 添加订阅者 - 使用远端 Offer 创建连接
// 返回 Answer SDP
func (r *RelayRoom) AddSubscriber(peerID string, offerSDP string) (string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", ErrRoomClosed
	}

	// 检查是否已存在
	if existing, exists := r.subscribers[peerID]; exists {
		r.mu.Unlock()
		// 如果已存在且连接正常，返回错误或处理重连
		if existing.state == SubscriberStateConnected {
			return "", nil
		}
		// 否则先移除旧连接
		r.RemoveSubscriber(peerID)
		r.mu.Lock()
	}
	r.mu.Unlock()

	// 创建 PeerConnection
	pc, err := r.api.NewPeerConnection(r.config)
	if err != nil {
		return "", err
	}

	sub := &Subscriber{
		id:           peerID,
		pc:           pc,
		state:        SubscriberStateConnecting,
		lastActivity: time.Now(),
	}

	// 添加 SourceSwitcher 的 Track
	videoTrack := r.switcher.GetVideoTrack()
	if videoTrack != nil {
		sender, err := pc.AddTrack(videoTrack)
		if err != nil {
			pc.Close()
			return "", err
		}
		sub.videoSender = sender

		// 读取 RTCP 反馈（必须消费，否则会阻塞）
		go r.readRTCP(peerID, sender)
	}

	audioTrack := r.switcher.GetAudioTrack()
	if audioTrack != nil {
		sender, err := pc.AddTrack(audioTrack)
		if err != nil {
			pc.Close()
			return "", err
		}
		sub.audioSender = sender
		go r.readRTCP(peerID, sender)
	}

	// 设置 ICE 处理 (必须在 SetLocalDescription 之前)
	r.setupICEHandlers(sub)

	// 处理 Offer
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return "", err
	}

	// 创建 Answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return "", err
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return "", err
	}

	// 初始连接完成后，设置协商处理器
	r.setupNegotiationHandlers(sub)

	// 注册订阅者
	r.mu.Lock()
	r.subscribers[peerID] = sub
	r.mu.Unlock()

	// 触发回调
	r.emitSubscriberJoined(peerID)

	// 确保新订阅者使用当前活跃的视频轨道
	// 如果 SetVideoCodec 已经被调用过，当前的视频轨道可能与 AddTrack 时使用的不同
	// 通过 ReplaceTrack 确保新订阅者获得正确的视频轨道
	currentVideoTrack := r.switcher.GetVideoTrack()
	currentAudioTrack := r.switcher.GetAudioTrack()
	if sub.videoSender != nil && currentVideoTrack != nil {
		if err := sub.videoSender.ReplaceTrack(currentVideoTrack); err != nil {
			utils.Error("[RelayRoom] ReplaceTrack for new subscriber %s failed: %v", peerID, err)
		} else {
			utils.Info("[RelayRoom] ReplaceTrack for new subscriber %s success", peerID)
		}
	}
	if sub.audioSender != nil && currentAudioTrack != nil {
		if err := sub.audioSender.ReplaceTrack(currentAudioTrack); err != nil {
			utils.Error("[RelayRoom] ReplaceAudioTrack for new subscriber %s failed: %v", peerID, err)
		}
	}

	// 请求关键帧，确保新订阅者能立即看到画面
	// 否则新订阅者需要等待下一个自然关键帧（可能需要几分钟）
	r.emitKeyframeRequest()

	return answer.SDP, nil
}

// CreateOfferForSubscriber 为订阅者创建 Offer（用于重协商）
func (r *RelayRoom) CreateOfferForSubscriber(peerID string) (string, error) {
	r.mu.RLock()
	sub, exists := r.subscribers[peerID]
	r.mu.RUnlock()

	if !exists {
		return "", ErrPeerNotFound
	}

	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.closed {
		return "", ErrPeerClosed
	}

	offer, err := sub.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	if err := sub.pc.SetLocalDescription(offer); err != nil {
		return "", err
	}

	return offer.SDP, nil
}

// HandleSubscriberAnswer 处理订阅者的 Answer（用于重协商）
func (r *RelayRoom) HandleSubscriberAnswer(peerID string, answerSDP string) error {
	r.mu.RLock()
	sub, exists := r.subscribers[peerID]
	r.mu.RUnlock()

	if !exists {
		return ErrPeerNotFound
	}

	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.closed {
		return ErrPeerClosed
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}

	return sub.pc.SetRemoteDescription(answer)
}

// AddICECandidate 添加 ICE 候选
func (r *RelayRoom) AddICECandidate(peerID string, candidate webrtc.ICECandidateInit) error {
	r.mu.RLock()
	sub, exists := r.subscribers[peerID]
	r.mu.RUnlock()

	if !exists {
		return ErrPeerNotFound
	}

	sub.mu.RLock()
	defer sub.mu.RUnlock()

	if sub.closed {
		return ErrPeerClosed
	}

	return sub.pc.AddICECandidate(candidate)
}

// RemoveSubscriber 移除订阅者
func (r *RelayRoom) RemoveSubscriber(peerID string) error {
	r.mu.Lock()
	sub, exists := r.subscribers[peerID]
	if !exists {
		r.mu.Unlock()
		return nil
	}
	delete(r.subscribers, peerID)
	r.mu.Unlock()

	// 关闭连接
	sub.mu.Lock()
	sub.closed = true
	pc := sub.pc
	sub.mu.Unlock()

	if pc != nil {
		pc.Close()
	}

	// 触发回调
	r.emitSubscriberLeft(peerID)

	return nil
}

// TriggerRenegotiation 触发重协商 - 为所有订阅者生成新 Offer
func (r *RelayRoom) TriggerRenegotiation() map[string]string {
	r.mu.RLock()
	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for _, sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.mu.RUnlock()

	offers := make(map[string]string)

	for _, sub := range subscribers {
		sub.mu.Lock()
		if sub.closed || sub.state != SubscriberStateConnected {
			sub.mu.Unlock()
			continue
		}

		offer, err := sub.pc.CreateOffer(nil)
		if err != nil {
			sub.mu.Unlock()
			continue
		}

		if err := sub.pc.SetLocalDescription(offer); err != nil {
			sub.mu.Unlock()
			continue
		}

		offers[sub.id] = offer.SDP

		// 触发回调通知 Dart 层
		r.emitNeedRenegotiate(sub.id, offer.SDP)
		sub.mu.Unlock()
	}

	return offers
}

// UpdateTracks 更新 Track（源切换后调用）
// 这会触发重协商
func (r *RelayRoom) UpdateTracks(videoTrack, audioTrack *webrtc.TrackLocalStaticRTP) {
	r.mu.RLock()
	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for _, sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.mu.RUnlock()

	utils.Info("[RelayRoom] UpdateTracks called, subscriber count: %d, videoTrack=%v, audioTrack=%v",
		len(subscribers), videoTrack != nil, audioTrack != nil)

	for _, sub := range subscribers {
		sub.mu.Lock()
		if sub.closed {
			utils.Info("[RelayRoom] Subscriber %s is closed, skipping", sub.id)
			sub.mu.Unlock()
			continue
		}

		needRenegotiate := false

		// 更新视频 Track
		if videoTrack != nil {
			if sub.videoSender != nil {
				// 已有 sender，直接替换 track
				if err := sub.videoSender.ReplaceTrack(videoTrack); err != nil {
					utils.Error("[RelayRoom] ReplaceTrack failed for %s: %v", sub.id, err)
				} else {
					utils.Info("[RelayRoom] ReplaceTrack success for %s", sub.id)
				}
			} else {
				// 没有 sender，需要动态添加 track 并重协商
				utils.Info("[RelayRoom] Adding new video track for %s (sender was nil)", sub.id)
				sender, err := sub.pc.AddTrack(videoTrack)
				if err != nil {
					utils.Error("[RelayRoom] AddTrack failed for %s: %v", sub.id, err)
				} else {
					sub.videoSender = sender
					needRenegotiate = true
					// 启动 RTCP 读取
					go r.readRTCP(sub.id, sender)
					utils.Info("[RelayRoom] AddTrack success for %s, will renegotiate", sub.id)
				}
			}
		}

		// 更新音频 Track
		if audioTrack != nil {
			if sub.audioSender != nil {
				sub.audioSender.ReplaceTrack(audioTrack)
			} else {
				// 动态添加音频 track
				sender, err := sub.pc.AddTrack(audioTrack)
				if err != nil {
					utils.Error("[RelayRoom] AddTrack (audio) failed for %s: %v", sub.id, err)
				} else {
					sub.audioSender = sender
					needRenegotiate = true
					go r.readRTCP(sub.id, sender)
				}
			}
		}

		peerID := sub.id
		pc := sub.pc
		sub.mu.Unlock()

		// 如果需要重协商，创建新的 Offer 并通知 Dart 层
		if needRenegotiate {
			utils.Info("[RelayRoom] Triggering renegotiation for %s", peerID)
			offer, err := pc.CreateOffer(nil)
			if err != nil {
				utils.Error("[RelayRoom] CreateOffer failed for %s: %v", peerID, err)
				continue
			}

			if err := pc.SetLocalDescription(offer); err != nil {
				utils.Error("[RelayRoom] SetLocalDescription failed for %s: %v", peerID, err)
				continue
			}

			// 通知 Dart 层发送新的 Offer 给订阅者
			if r.onNeedRenegotiate != nil {
				r.onNeedRenegotiate(r.id, peerID, offer.SDP)
				utils.Info("[RelayRoom] Sent renegotiation offer to %s", peerID)
			}
		}
	}
}

// GetSubscribers 获取所有订阅者 ID
func (r *RelayRoom) GetSubscribers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.subscribers))
	for id := range r.subscribers {
		ids = append(ids, id)
	}
	return ids
}

// GetSubscriberCount 获取订阅者数量
func (r *RelayRoom) GetSubscriberCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscribers)
}

// GetSubscriberState 获取订阅者状态
func (r *RelayRoom) GetSubscriberState(peerID string) (SubscriberState, bool) {
	r.mu.RLock()
	sub, exists := r.subscribers[peerID]
	r.mu.RUnlock()

	if !exists {
		return SubscriberStateDisconnected, false
	}

	sub.mu.RLock()
	defer sub.mu.RUnlock()
	return sub.state, true
}

// Close 关闭房间
func (r *RelayRoom) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	// 复制订阅者列表
	subscribers := make([]*Subscriber, 0, len(r.subscribers))
	for _, sub := range r.subscribers {
		subscribers = append(subscribers, sub)
	}
	r.subscribers = make(map[string]*Subscriber)
	r.mu.Unlock()

	// 关闭所有订阅者
	for _, sub := range subscribers {
		sub.mu.Lock()
		sub.closed = true
		if sub.pc != nil {
			sub.pc.Close()
		}
		sub.mu.Unlock()
	}

	// 关闭源切换器
	if r.switcher != nil {
		r.switcher.Close()
	}

	return nil
}

// setupICEHandlers 设置 ICE 相关处理器
func (r *RelayRoom) setupICEHandlers(sub *Subscriber) {
	// ICE 候选生成
	sub.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			r.emitICECandidate(sub.id, candidate)
		}
	})

	// 连接状态变化
	sub.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		sub.mu.Lock()
		switch state {
		case webrtc.PeerConnectionStateConnected:
			sub.state = SubscriberStateConnected
			sub.lastActivity = time.Now()
		case webrtc.PeerConnectionStateDisconnected:
			sub.state = SubscriberStateDisconnected
		case webrtc.PeerConnectionStateFailed:
			sub.state = SubscriberStateFailed
		case webrtc.PeerConnectionStateClosed:
			sub.state = SubscriberStateDisconnected
		}
		sub.mu.Unlock()
	})

	// ICE 连接状态变化
	sub.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed {
			r.emitError(sub.id, ErrICEFailed)
		}
	})
}

// setupNegotiationHandlers 设置协商处理器
func (r *RelayRoom) setupNegotiationHandlers(sub *Subscriber) {
	// 协商需求（Track 变化时触发）
	sub.pc.OnNegotiationNeeded(func() {
		// 创建新 Offer 并通知 Dart 层
		sub.mu.Lock()
		if sub.closed {
			sub.mu.Unlock()
			return
		}

		// 检查状态，避免 Glare
		if sub.pc.SignalingState() != webrtc.SignalingStateStable {
			sub.mu.Unlock()
			return
		}

		offer, err := sub.pc.CreateOffer(nil)
		if err != nil {
			sub.mu.Unlock()
			return
		}

		if err := sub.pc.SetLocalDescription(offer); err != nil {
			sub.mu.Unlock()
			return
		}
		sub.mu.Unlock()

		r.emitNeedRenegotiate(sub.id, offer.SDP)
	})
}

// Deprecated: setupSubscriberHandlers is split into setupICEHandlers and setupNegotiationHandlers
func (r *RelayRoom) setupSubscriberHandlers(sub *Subscriber) {
	r.setupICEHandlers(sub)
	r.setupNegotiationHandlers(sub)
}

// readRTCP 读取 RTCP 反馈
func (r *RelayRoom) readRTCP(peerID string, sender *webrtc.RTPSender) {
	rtcpBuf := make([]byte, 1500)
	for {
		n, _, err := sender.Read(rtcpBuf)
		if err != nil {
			return
		}

		// 解析 RTCP 包，检测 PLI (Picture Loss Indication) 请求
		// PLI 表示接收端需要关键帧
		// RTCP 包格式: https://tools.ietf.org/html/rfc4585
		// PLI 的 Payload Type 是 206 (PSFB), FMT=1
		if n >= 4 {
			// RTCP header: version(2) + padding(1) + FMT(5) + PT(8) + length(16)
			pt := rtcpBuf[1]
			// PT=206 是 PSFB (Payload-Specific Feedback)
			// FMT=1 是 PLI (Picture Loss Indication)
			fmt := (rtcpBuf[0] >> 0) & 0x1F // 低5位是 FMT
			if pt == 206 && fmt == 1 {
				// 节流 PLI 请求，避免频繁请求关键帧
				// 每隔 1 秒最多转发一次
				r.mu.Lock()
				now := time.Now()
				if now.Sub(r.lastPLIRequest) > 1*time.Second {
					r.lastPLIRequest = now
					r.mu.Unlock()
					utils.Info("[RelayRoom] PLI received from subscriber %s, requesting keyframe from SFU", peerID)
					r.emitKeyframeRequest()
				} else {
					r.mu.Unlock()
					// 跳过此 PLI，太频繁
				}
			}
		}
	}
}

// 回调触发函数
func (r *RelayRoom) emitSubscriberJoined(peerID string) {
	r.mu.RLock()
	fn := r.onSubscriberJoined
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id, peerID)
	}
}

func (r *RelayRoom) emitSubscriberLeft(peerID string) {
	r.mu.RLock()
	fn := r.onSubscriberLeft
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id, peerID)
	}
}

func (r *RelayRoom) emitICECandidate(peerID string, candidate *webrtc.ICECandidate) {
	r.mu.RLock()
	fn := r.onICECandidate
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id, peerID, candidate)
	}
}

func (r *RelayRoom) emitNeedRenegotiate(peerID string, offer string) {
	r.mu.RLock()
	fn := r.onNeedRenegotiate
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id, peerID, offer)
	}
}

func (r *RelayRoom) emitError(peerID string, err error) {
	r.mu.RLock()
	fn := r.onError
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id, peerID, err)
	}
}

func (r *RelayRoom) emitKeyframeRequest() {
	r.mu.RLock()
	fn := r.onKeyframeRequest
	r.mu.RUnlock()
	if fn != nil {
		fn(r.id)
	}
}

// ========================================
// 状态结构体
// ========================================

// SubscriberInfo 订阅者信息
type SubscriberInfo struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	BytesSent    uint64 `json:"bytes_sent"`
	PacketsSent  uint64 `json:"packets_sent"`
	LastActivity int64  `json:"last_activity"`
}

// RelayRoomStatus 房间状态
type RelayRoomStatus struct {
	RoomID          string           `json:"room_id"`
	IsRelay         bool             `json:"is_relay"`
	RelayPeerID     string           `json:"relay_peer_id,omitempty"`
	SubscriberCount int              `json:"subscriber_count"`
	Subscribers     []SubscriberInfo `json:"subscribers"`
	SourceSwitcher  interface{}      `json:"source_switcher,omitempty"`
}

// GetStatus 获取房间状态
func (r *RelayRoom) GetStatus() RelayRoomStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := RelayRoomStatus{
		RoomID:          r.id,
		IsRelay:         r.isRelay,
		RelayPeerID:     r.relayPeerID,
		SubscriberCount: len(r.subscribers),
		Subscribers:     make([]SubscriberInfo, 0, len(r.subscribers)),
	}

	for _, sub := range r.subscribers {
		sub.mu.RLock()
		status.Subscribers = append(status.Subscribers, SubscriberInfo{
			ID:           sub.id,
			State:        sub.state.String(),
			BytesSent:    atomic.LoadUint64(&sub.bytesSent),
			PacketsSent:  atomic.LoadUint64(&sub.packetsSent),
			LastActivity: sub.lastActivity.Unix(),
		})
		sub.mu.RUnlock()
	}

	if r.switcher != nil {
		status.SourceSwitcher = r.switcher.GetStatus()
	}

	return status
}

// ToJSON 序列化为 JSON
func (s RelayRoomStatus) ToJSON() string {
	data, _ := json.Marshal(s)
	return string(data)
}
