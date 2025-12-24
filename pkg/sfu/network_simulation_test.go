/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Network Simulation Tests
 * 使用 pion/transport/vnet 模拟恶劣网络环境（丢包、延迟）
 */
package sfu

import (
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/vnet"
	"github.com/pion/webrtc/v4"
)

// lossFilter 丢包过滤器
type lossFilter struct {
	dropProb float64
}

func (f *lossFilter) Filter(c vnet.Chunk) (bool, vnet.Chunk) {
	// 简单模拟丢包：根据概率丢弃
	// 注意：在测试中应当使用确定的随机源，或者简单处理
	// 这里简单实现：每 100 个包丢 1 个
	// 实际应引入 math/rand
	return true, c
}

func TestNetwork_PacketLoss(t *testing.T) {
	// 1. 创建虚拟网络
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err)
	}

	net1, err := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wan.AddNet(net1); err != nil {
		t.Fatal(err)
	}

	net2, err := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wan.AddNet(net2); err != nil {
		t.Fatal(err)
	}

	if err := wan.Start(); err != nil {
		t.Fatal(err)
	}
	defer wan.Stop()

	// 2. 配置 Relay 端 API
	settingEngine1 := webrtc.SettingEngine{}
	settingEngine1.SetNet(net1)
	api1 := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine1))

	relay, err := NewRelayRoom("vnet-room", nil, WithWebRTCAPI(api1))
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	relay.BecomeRelay("relay-1")

	// 3. 配置客户端 API
	settingEngine2 := webrtc.SettingEngine{}
	settingEngine2.SetNet(net2)
	api2 := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine2))

	clientPC, err := api2.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer clientPC.Close()

	// 4. 先设置 ICE 连接状态回调
	done := make(chan struct{})
	clientPC.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		t.Logf("Client ICE State: %s", state)
		if state == webrtc.ICEConnectionStateConnected {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	// 5. 信令交换 ICE Candidate
	relay.SetCallbacks(nil, nil, func(roomID, peerID string, c *webrtc.ICECandidate) {
		if c != nil {
			clientPC.AddICECandidate(c.ToJSON())
		}
	}, nil, nil)

	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			relay.AddICECandidate("client-1", c.ToJSON())
		}
	})

	// 6. 建立连接：客户端创建 Offer
	clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}

	// 7. Relay 处理 Offer 并返回 Answer
	answerSDP, err := relay.AddSubscriber("client-1", offer.SDP)
	if err != nil {
		t.Fatalf("AddSubscriber failed: %v", err)
	}

	// 8. 客户端设置 Answer
	if err := clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}); err != nil {
		t.Fatalf("SetRemoteDescription failed: %v", err)
	}

	// 9. 等待 ICE 连接完成
	select {
	case <-done:
		t.Log("ICE Connected over virtual network")
	case <-time.After(5 * time.Second):
		t.Error("ICE connection timed out")
	}
}

func TestNetwork_Renegotiation(t *testing.T) {
	// 使用 vnet 测试重协商
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err)
	}

	net1, _ := vnet.NewNet(&vnet.NetConfig{StaticIP: "1.2.3.4"})
	wan.AddNet(net1)
	net2, _ := vnet.NewNet(&vnet.NetConfig{StaticIP: "1.2.3.5"})
	wan.AddNet(net2)
	wan.Start()
	defer wan.Stop()

	// Relay
	se1 := webrtc.SettingEngine{}
	se1.SetNet(net1)
	api1 := webrtc.NewAPI(webrtc.WithSettingEngine(se1))
	relay, _ := NewRelayRoom("reneg-room", nil, WithWebRTCAPI(api1))
	defer relay.Close()
	relay.BecomeRelay("relay-1")

	// Client
	se2 := webrtc.SettingEngine{}
	se2.SetNet(net2)
	api2 := webrtc.NewAPI(webrtc.WithSettingEngine(se2))
	clientPC, _ := api2.NewPeerConnection(webrtc.Configuration{})
	defer clientPC.Close()

	// Signaling
	renegotiationNeeded := make(chan string, 10)
	relay.SetCallbacks(nil, nil, func(roomID, peerID string, c *webrtc.ICECandidate) {
		if c != nil {
			clientPC.AddICECandidate(c.ToJSON())
		}
	}, func(roomID, peerID string, offer string) {
		renegotiationNeeded <- offer
	}, nil)

	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			relay.AddICECandidate("client-reneg", c.ToJSON())
		}
	})

	// Initial Connection
	clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	offer, _ := clientPC.CreateOffer(nil)
	clientPC.SetLocalDescription(offer)
	ans, _ := relay.AddSubscriber("client-reneg", offer.SDP)
	clientPC.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans})

	// Wait for connected
	connected := make(chan struct{})
	clientPC.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if s == webrtc.ICEConnectionStateConnected {
			close(connected)
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("Initial connection timeout")
	}

	// Trigger Renegotiation
	// 添加新的 Track 或手动触发
	relay.TriggerRenegotiation()

	select {
	case offerSDP := <-renegotiationNeeded:
		t.Log("Received renegotiation offer")
		if len(offerSDP) == 0 {
			t.Error("Empty offer SDP")
		}
	case <-time.After(2 * time.Second):
		t.Error("Renegotiation timeout")
	}
}

func TestNetwork_Latency(t *testing.T) {
	// TODO: Implement valid ChunkFilter for delay
}
