# 架构概述

## 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      Flutter App                                 │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │  Room    │ │ Election │ │  Media   │ │    Monitoring    │   │
│  ├──────────┤ ├──────────┤ ├──────────┤ ├──────────────────┤   │
│  │RelayRoom │ │Election  │ │SourceSw │ │Keepalive│Stats   │   │
│  │RelayRoom │ │Failover  │ │JitterBuf│ │NetworkProbe      │   │
│  │  P2P     │ │ProxyMgr  │ │         │ │                  │   │
│  └────┬─────┘ └────┬─────┘ └────┬────┘ └────────┬─────────┘   │
│       │            │            │               │              │
│  ┌────▼────────────▼────────────▼───────────────▼────────┐     │
│  │                    Core (Coordinator)                  │     │
│  │         一键自动代理 - 整合所有组件                      │     │
│  └────────────────────────────┬──────────────────────────┘     │
│                               │                                 │
│  ┌────────────────────────────▼──────────────────────────┐     │
│  │               FFI Bindings (bindings/)                 │     │
│  │                    Dart ↔ Go 桥接                       │     │
│  └────────────────────────────┬──────────────────────────┘     │
├───────────────────────────────┼─────────────────────────────────┤
│           Native Go Library   │                                 │
│  ┌────────────────────────────▼──────────────────────────┐     │
│  │              Go SFU Core (librelay)                    │     │
│  │  • Pion WebRTC                                         │     │
│  │  • RTP 纯透传                                          │     │
│  │  • 零解码转发                                          │     │
│  └───────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

## 模块职责

### Core 层

| 模块 | 职责 |
|------|------|
| `SfuRelay` | SDK 入口，版本管理，日志级别，编解码器查询 |
| `AutoCoordinator` | **推荐 ⭐** - 真正一键自动代理，内置信令集成、选举、故障切换 |
| `Coordinator` | 手动代理 - 需自行处理信令和事件 |

### Room 层

| 模块 | 职责 |
|------|------|
| `RelayRoom` | 高级房间封装，含信令集成和事件流 |
| `RelayRoomP2P` | 底层 P2P 管理，完全控制订阅者和 SDP 协商 |

### Election 层

| 模块 | 职责 |
|------|------|
| `Election` | 独立选举 API，基于设备/网络评分 |
| `Failover` | 故障检测和自动切换 |
| `ProxyManager` | 自动代理状态管理 |
| `ProxyMode` | 便捷初始化函数 |

### Media 层

| 模块 | 职责 |
|------|------|
| `SourceSwitcher` | SFU 源 ↔ 本地分享源切换 |
| `JitterBuffer` | 可选的抖动缓冲，平滑网络抖动 |

### Monitoring 层

| 模块 | 职责 |
|------|------|
| `Keepalive` | 心跳检测，Peer 在线状态监控 |
| `Stats` | 流量统计，带宽和丢包率 |
| `NetworkProbe` | 网络探测，RTT/带宽/丢包测量 |

## 数据流

### 正常观看模式

```
LiveKit SFU                     局域网 Relay                订阅者
    │                               │                          │
    │   RTP Packet                  │                          │
    │ ──────────────────────────▶   │                          │
    │                               │   P2P Forward            │
    │                               │ ─────────────────────▶   │
    │                               │                          │
```

### 本地分享模式

```
本地屏幕捕获                    局域网 Relay                订阅者
    │                               │                          │
    │   Local RTP                   │                          │
    │ ──────────────────────────▶   │                          │
    │                               │   P2P Forward            │
    │ (SFU 流暂停)                  │ ─────────────────────────▶│
    │                               │                          │
```

## 与 Go Core 的关系

```
Flutter 层                              Go 层
────────────────────────────────────────────────────────────
AutoCoordinator.start()     →    CoordinatorEnable() + 自动信令
Coordinator.enable()        →    CoordinatorEnable()
RelayRoomP2P.addSubscriber() →   RelayRoomAddSubscriber()
Election.trigger()          →    ElectionTrigger()
                            ←    EventCallback (事件回调)
                            ←    LogCallback (日志回调)
                            ←    PingCallback (Ping 请求)
```

## 推荐使用方式

### 最简单 → AutoCoordinator（推荐 ⭐）

```dart
final autoCoord = AutoCoordinator(
  roomId: 'room-1',
  localPeerId: 'peer-1',
  signaling: WebSocketSignaling(url: 'ws://...', localPeerId: 'peer-1'),
  config: AutoCoordinatorConfig(
    deviceType: DeviceType.pc,
    connectionType: ConnectionType.wifi,
    powerState: PowerState.pluggedIn,
  ),
);

await autoCoord.start();  // 一键搞定！自动选举、Ping/Pong、故障切换
```

### 手动控制 → Coordinator

```dart
final coordinator = Coordinator(
  roomId: 'room-1',
  localPeerId: 'peer-1',
);
coordinator.enable();
// 需要自行处理 EventHandler, PingHandler, 信令...
```

### 底层控制 → 组合使用

```dart
final election = Election(roomId: 'room-1');
final failover = Failover(roomId: 'room-1', localPeerId: 'peer-1');
final keepalive = Keepalive(roomId: 'room-1');
```

