/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Example: Basic Relay Usage
 *
 * 这个示例展示了 Relay Core 的基本使用方法。
 * 注意：这是一个独立的演示程序，不作为 C-shared 库编译。
 *
 * 构建命令: go build -o relay_example example/basic/main.go
 */
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/maiguangyang/relay_core/pkg/sfu"
)

func main() {
	fmt.Println("=== Relay Core Basic Example ===")
	fmt.Println()

	// 1. 创建 SourceSwitcher
	fmt.Println("1. Creating SourceSwitcher...")
	switcher, err := sfu.NewSourceSwitcher("example-room")
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	defer switcher.Close()
	fmt.Println("   ✓ SourceSwitcher created")

	// 2. 设置源切换回调
	switcher.SetOnSourceChanged(func(roomID string, source sfu.SourceType, sharerID string) {
		fmt.Printf("   → Source changed to: %s\n", source)
	})

	// 3. 创建选举系统
	fmt.Println("\n2. Creating Election System...")
	electionConfig := election.DefaultElectorConfig()
	elector := election.NewElector("example-room", electionConfig)
	defer elector.Close()
	fmt.Println("   ✓ Elector created")

	// 4. 添加候选者
	fmt.Println("\n3. Adding Candidates...")
	candidates := []election.Candidate{
		{
			PeerID:         "pc-office",
			DeviceType:     election.DeviceTypePC,
			ConnectionType: election.ConnectionTypeEthernet,
			PowerState:     election.PowerStatePluggedIn,
			Bandwidth:      100000000, // 100 Mbps
			Latency:        10,        // 10ms
			PacketLoss:     0.0,
		},
		{
			PeerID:         "macbook-meeting",
			DeviceType:     election.DeviceTypePC,
			ConnectionType: election.ConnectionTypeWiFi,
			PowerState:     election.PowerStateBattery,
			Bandwidth:      50000000, // 50 Mbps
			Latency:        20,       // 20ms
			PacketLoss:     0.001,
		},
		{
			PeerID:         "ipad-user",
			DeviceType:     election.DeviceTypePad,
			ConnectionType: election.ConnectionTypeWiFi,
			PowerState:     election.PowerStatePluggedIn,
			Bandwidth:      30000000, // 30 Mbps
			Latency:        30,       // 30ms
			PacketLoss:     0.002,
		},
		{
			PeerID:         "mobile-user",
			DeviceType:     election.DeviceTypeMobile,
			ConnectionType: election.ConnectionTypeWiFi,
			PowerState:     election.PowerStateBattery,
			Bandwidth:      20000000, // 20 Mbps
			Latency:        50,       // 50ms
			PacketLoss:     0.01,
		},
	}

	for _, c := range candidates {
		elector.UpdateCandidate(c)
		fmt.Printf("   + Added: %s (Device=%s, Conn=%s)\n",
			c.PeerID,
			deviceTypeName(c.DeviceType),
			connectionTypeName(c.ConnectionType))
	}

	// 5. 执行选举
	fmt.Println("\n4. Running Election...")
	result := elector.Elect()
	if result != nil {
		fmt.Printf("   ✓ Winner: %s (Score: %.2f)\n", result.ProxyID, result.Score)
		fmt.Printf("   Reason: %s\n", result.Reason)
	}

	// 6. 显示所有候选者分数
	fmt.Println("\n5. Candidate Scores:")
	allCandidates := elector.GetCandidates()
	for i, c := range allCandidates {
		fmt.Printf("   %d. %s: %.2f points\n", i+1, c.PeerID, c.Score)
	}

	// 7. 模拟 RTP 包注入
	fmt.Println("\n6. Simulating RTP Injection...")
	rtpPacket := createMockRTPPacket(1)
	err = switcher.InjectSFUPacket(true, rtpPacket)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Println("   ✓ Injected SFU video packet")
	}

	// 8. 模拟本地分享
	fmt.Println("\n7. Simulating Local Share...")
	fmt.Println("   Starting local share from 'mobile-user'...")
	switcher.StartLocalShare("mobile-user")

	time.Sleep(100 * time.Millisecond)

	status := switcher.GetStatus()
	statusJSON, _ := json.MarshalIndent(status, "   ", "  ")
	fmt.Printf("   Status:\n%s\n", string(statusJSON))

	fmt.Println("\n   Stopping local share...")
	switcher.StopLocalShare()

	// 9. 显示心跳状态
	fmt.Println("\n8. Keepalive Demo...")
	km := sfu.NewKeepaliveManager(sfu.DefaultKeepaliveConfig())
	defer km.Stop()

	km.AddPeer("pc-office")
	km.AddPeer("macbook-meeting")
	km.HandlePong("pc-office")

	fmt.Printf("   pc-office status: %s\n", km.GetPeerStatus("pc-office"))
	fmt.Printf("   pc-office RTT: %v\n", km.GetPeerRTT("pc-office"))

	// 10. BufferPool 演示
	fmt.Println("\n9. BufferPool Demo...")
	for i := 0; i < 100; i++ {
		buf := sfu.GetRTPBuffer()
		buf[0] = byte(i)
		sfu.PutRTPBuffer(buf)
	}

	poolStats := sfu.GetGlobalBufferPoolStats()
	fmt.Printf("   Standard Reuses: %d\n", poolStats.StandardReuses)
	fmt.Printf("   Reuse Ratio: %.1f%%\n", poolStats.ReuseRatio*100)

	fmt.Println("\n=== Example Complete ===")
}

func createMockRTPPacket(seq uint16) []byte {
	packet := make([]byte, 100)
	packet[0] = 0x80 // Version 2
	packet[1] = 96   // Payload type
	packet[2] = byte(seq >> 8)
	packet[3] = byte(seq)
	return packet
}

func deviceTypeName(dt election.DeviceType) string {
	names := map[election.DeviceType]string{
		election.DeviceTypeUnknown: "Unknown",
		election.DeviceTypePC:      "PC",
		election.DeviceTypePad:     "Pad",
		election.DeviceTypeMobile:  "Mobile",
		election.DeviceTypeTV:      "TV",
	}
	return names[dt]
}

func connectionTypeName(ct election.ConnectionType) string {
	names := map[election.ConnectionType]string{
		election.ConnectionTypeUnknown:  "Unknown",
		election.ConnectionTypeEthernet: "Ethernet",
		election.ConnectionTypeWiFi:     "WiFi",
		election.ConnectionTypeCellular: "Cellular",
	}
	return names[ct]
}
