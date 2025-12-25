# Flutter SFU Relay

局域网代理转发 SDK - 基于 Pion WebRTC 的嵌入式微型 SFU 核心

[![Flutter](https://img.shields.io/badge/Flutter-3.3.0+-blue.svg)](https://flutter.dev/)
[![Dart](https://img.shields.io/badge/Dart-3.0+-blue.svg)](https://dart.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 🎯 定位

> **这是 LiveKit 等云端 RTC 服务的局域网优化层，不是替代品！**

```
┌─────────────── 同一局域网 ────────────────┐
│                                           │
│  设备A ◀───┐                 ┌───▶ 设备B   │
│            │   本地 Relay    │            │
│  设备C ◀───┴───(本插件)───────┴───▶ 设备D   │
│                    │                      │
└────────────────────┼──────────────────────┘
                     │ (只需一条上行)
                     ▼
             ┌──────────────┐
             │ LiveKit 云端  │
             └──────────────┘
```

**效果**: 4 台设备同网，原本 4 条上行 → 优化后 1 条上行

## 功能特性

- 🚀 **RTP 纯透传转发** - 零解码，超低延迟
- 🔄 **动态代理选举** - 自动选择最优 Relay (分数 + PeerId)
- ⚡ **快速故障切换** - ~2.5 秒自动切换到备用 Relay
- 💓 **心跳检测** - 1s 间隔，1.5s 超时，快速检测 Peer 离线
- 📱 **本地分享切换** - 支持屏幕共享优先级
- 📊 **流量统计** - 带宽和丢包监控
- 🎚️ **抖动缓冲** - 可选的网络抖动平滑
- 🔙 **降级机制** - 连续 N 次选举失败后自动降级到直连 SFU

## 📚 详细文档

| 文档 | 说明 |
|------|------|
| [快速开始](docs/quick-start.md) | 3 种使用方式入门 |
| [架构概述](docs/architecture.md) | 模块结构和数据流 |
| [选举系统](docs/election.md) | 评分规则和选举流程 |
| [LiveKit 集成](docs/livekit-integration.md) | 与 LiveKit 配合使用 |
| [平台权限配置](docs/platform-permissions.md) | 各平台权限设置指南 |
| [API 参考](docs/api-reference.md) | 完整 API 文档 |

```yaml
dependencies:
  flutter_sfu_relay:
    path: ../flutter_sfu_relay
```

## 快速开始

### 1. 初始化

```dart
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';

// 初始化回调系统
EventHandler.init();
LogHandler.init();

// 获取版本
print('Version: ${SfuRelay.instance.version}');
```

### 2. 使用 Coordinator（推荐）

一键启用自动代理模式：

```dart
final coordinator = Coordinator(
  roomId: 'room-1',
  localPeerId: 'my-peer-id',
);

// 启用自动代理
coordinator.enable();

// 添加 Peer（自动监控心跳 + 参与选举）
coordinator.addPeer('peer-2', deviceType: DeviceType.pc);

// 处理 Pong 响应
coordinator.handlePong('peer-2');

// 注入 RTP 数据
coordinator.injectSfuPacket(true, rtpData);

// 检查是否是 Relay
if (coordinator.isRelay) {
  print('I am the relay!');
}

// 禁用
coordinator.disable();
```

### 3. 使用 RelayRoom P2P（高级）

完全控制 P2P 连接：

```dart
final room = RelayRoomP2P('room-1');

// 创建房间
room.create(iceServers: [
  {'urls': ['stun:stun.l.google.com:19302']}
]);

// 成为 Relay
room.becomeRelay('my-peer-id');

// 添加订阅者
final answer = room.addSubscriber('subscriber-1', offerSdp);

// 处理 ICE
room.addIceCandidate('subscriber-1', {'candidate': '...'});

// 注入媒体
room.injectSfu(isVideo: true, data: rtpData);

// 重协商
final offers = room.triggerRenegotiation();

// 销毁
room.destroy();
```

### 4. 与 LiveKit 集成

```dart
import 'package:livekit_client/livekit_client.dart';

// 1. 用 LiveKit 加入房间
final lkRoom = Room();
await lkRoom.connect('wss://your-livekit-server', token);

// 2. 创建 ProxyManager（自动计算评分）
// 评分规则: 设备(40) + 网络(40) + 电源(20)
// PC+Ethernet+PluggedIn = 40+40+20 = 100 分（最优）
// Mobile+Cellular+LowBattery = 20+10+0 = 30 分（最低）
final proxyManager = ProxyManager(
  roomId: lkRoom.name!,
  localPeerId: lkRoom.localParticipant!.identity,
  deviceType: DeviceType.mobile,        // PC=40, Pad=30, Mobile=20
  connectionType: ConnectionType.wifi,  // Ethernet=40, WiFi=30, Cellular=10
  powerState: PowerState.battery,       // PluggedIn=20, Battery=10, Low=0
);

await proxyManager.start();

// 3. 监听 Participant 变化
lkRoom.onParticipantConnected = (p) {
  coordinator.addPeer(p.identity, deviceType: DeviceType.mobile);
};

// 4. 监听选举触发，通过 DataChannel 广播
proxyManager.onElectionTriggered.listen((_) {
  final epoch = proxyManager.currentEpoch;
  final status = proxyManager.getStatus();
  
  lkRoom.localParticipant!.publishData(
    utf8.encode(jsonEncode({
      'type': 'relay_claim',
      'epoch': epoch,                    // 由 ProxyManager 管理
      'score': status['local_score'],    // Go 层计算的分数
    })),
    reliable: true,
  );
});

// 5. 接收其他节点的 claim
lkRoom.onDataReceived = (data, participant, topic) {
  final msg = jsonDecode(utf8.decode(data));
  if (msg['type'] == 'relay_claim') {
    proxyManager.handleRelayClaim(
      participant.identity,
      msg['epoch'],
      msg['score'],
    );
  }
};
```

## API 概览

### 核心模块 (`core/`)

| 类 | 功能 |
|----|------|
| `SfuRelay` | SDK 入口，版本、日志级别、编解码器 |
| `Coordinator` | **推荐** - 一键自动代理管理 |

### 房间管理 (`room/`)

| 类 | 功能 |
|----|------|
| `RelayRoomP2P` | 底层 P2P 连接管理 (17 个 Go 函数) |
| `RelayRoom` | 高级房间封装（含信令集成） |

### 选举和故障切换 (`election/`)

| 类 | 功能 |
|----|------|
| `Election` | 独立选举 API (设备/网络评分) |
| `Failover` | 故障切换管理器 |
| `ProxyManager` | 自动代理状态管理 |
| `ProxyMode` | 便捷组合函数 |

### 媒体处理 (`media/`)

| 类 | 功能 |
|----|------|
| `SourceSwitcher` | SFU/本地源切换 |
| `JitterBuffer` | 抖动缓冲控制 (7 个函数) |

### 监控 (`monitoring/`)

| 类 | 功能 |
|----|------|
| `Keepalive` | 心跳检测 (12 个函数) |
| `Stats` | 流量统计 |
| `NetworkProbe` | 网络探测 |

### 回调 (`callbacks/`)

| 类 | 功能 |
|----|------|
| `EventHandler` | Go 层事件 → Dart Stream |
| `LogHandler` | Go 层日志 → Dart Stream |
| `PingHandler` | Ping 请求 → 信令转发 |

### 信令 (`signaling/`)

| 类 | 功能 |
|----|------|
| `SignalingBridge` | 抽象信令接口 |
| `WebSocketSignaling` | WebSocket 实现 |

### WebRTC (`webrtc/`)

| 类 | 功能 |
|----|------|
| `WebRTCManager` | PeerConnection 管理 |
| `SdpHandler` | SDP/ICE 处理 |
| `RtpForwarder` | RTP 包转发 |

## Go API 覆盖

| 模块 | Go 函数数 | Flutter 覆盖 |
|------|----------|-------------|
| Coordinator | 14 | ✅ 100% |
| RelayRoom | 17 | ✅ 100% |
| SourceSwitcher | 8 | ✅ 100% |
| Election | 8 | ✅ 100% |
| Failover | 6 | ✅ 100% |
| Keepalive | 12 | ✅ 92% |
| Stats | 13 | ✅ 85% |
| JitterBuffer | 7 | ✅ 100% |
| NetworkProbe | 4 | ✅ 100% |
| Callbacks | 8 | ✅ 100% |
| **Total** | **106** | **~95%** |

## 事件类型

| 值 | 事件 | 说明 |
|----|------|------|
| 1 | PeerJoined | Peer 加入 |
| 2 | PeerLeft | Peer 离开 |
| 4 | Error | 错误 |
| 5 | IceCandidate | ICE 候选 |
| 6 | ProxyChange | Relay 变更 |
| 10 | SubscriberJoined | 订阅者加入 |
| 11 | SubscriberLeft | 订阅者离开 |
| 12 | NeedRenegotiation | 需要重协商 |
| 20 | PeerOnline | Peer 上线 (心跳检测) |
| 21 | PeerSlow | Peer 响应慢 (心跳检测) |
| 22 | PeerOffline | Peer 离线 (心跳超时) |
| 23 | Ping | 需要发送 Ping |
| 24 | RelayDisabled | Relay 模式已降级 |

## 平台支持

| 平台 | 库文件 | 状态 |
|------|--------|------|
| macOS | `librelay.dylib` | ✅ |
| iOS | `librelay.xcframework` | ✅ |
| Android | `librelay.so` | ✅ |
| Linux | `librelay.so` | ✅ |
| Windows | `librelay.dll` | ✅ |

## 目录结构

```
lib/
├── flutter_sfu_relay.dart     # 主入口
└── src/
    ├── core/                  # 核心入口
    ├── room/                  # 房间管理
    ├── election/              # 选举/故障切换
    ├── media/                 # 媒体处理
    ├── monitoring/            # 监控
    ├── callbacks/             # 回调处理
    ├── signaling/             # 信令
    ├── webrtc/                # WebRTC
    ├── bindings/              # FFI 绑定
    └── enums.dart             # 枚举
```

## License

MIT
