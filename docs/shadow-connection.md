# 影子连接 (Shadow Connection)

## 概述

**影子连接 (Shadow Connection)** 是 Relay Core 为解决 Flutter SDK 限制而引入的一种架构模式。它允许 Go 层通过独立的 `LiveKitBridge` 直连 SFU 获取 RTP 包，同时保持 Dart 层对业务逻辑的控制。

### 为什么需要它？

1.  **WebRTC 标准限制**：标准 WebRTC 不允许应用层直接访问原始 RTP 包。
2.  **Dart SDK 限制**：`livekit_client` (Flutter) 封装了 WebRTC 实现，没有暴露底层的 RTP 回调接口。
3.  **Relay 需求**：Relay Core 需要获取**原始 RTP 包**并通过 UDP/TCP 转发给局域网其他设备（零解码转发）。

## 架构原理

采用了 **Go/Dart 双连接** 模式：

```mermaid
graph TD
    LiveKit[LiveKit SFU Cloud]
    
    subgraph RelayDevice [Relay 设备]
        direction TB
        subgraph DartLayer [Dart 层 (Flutter)]
            Client[User Client]
            Coord[AutoCoordinator]
        end
        
        subgraph GoLayer [Go 层 (Relay Core)]
            Bridge[LiveKitBridge] -- Shadow Connection --> LiveKit
            Switcher[SourceSwitcher]
            RelayRoom[RelayRoom]
        end
        
        Client -- User Connection --> LiveKit
        Bridge -- RTP/RTCP --> Switcher
        Switcher -- P2P --> RelayRoom
    end
    
    subgraph LAN [局域网用户]
        User1 <--> RelayRoom
        User2 <--> RelayRoom
    end
```

1.  **用户连接 (Dart)**：正常的业务连接，用于与会议室信令交互、展示 UI、发布自己的流。
2.  **影子连接 (Go)**：**只有当选为 Relay 时**才会启动。使用特殊的 `Relay Bot` 身份连接 SFU，专门用于拉取媒体流（Subscribe Only）。

## 关键流程

1.  **选举**：所有设备通过 `AutoCoordinator` 协商选出唯一的 Relay 节点。
2.  **Token 获取**：
    *   当选节点触发 `onRequestBotToken` 回调。
    *   Dart 层请求服务器或生成一个具有 `canSubscribe: true` 权限的 Bot Token。
3.  **启动连接**：
    *   Dart 调用 FFI `LiveKitBridgeConnect`。
    *   Go 层启动 `livekit-server-sdk-go` 客户端。
4.  **RTP 转发**：
    *   Go 层 `OnTrackSubscribed` 接收到新轨道。
    *   启动 `readRTPLoop` 读取 RTP 包。
    *   调用 `SourceSwitcher.InjectSFUPacket()` 注入数据。
    *   `RelayRoom` 将数据转发给局域网所有连接者。

## 配置指南

### Dart 侧配置

在 `AutoCoordinatorConfig` 中配置相关回调：

```dart
final autoCoord = AutoCoordinator(
  // ...
  config: AutoCoordinatorConfig(
    // 启用自动选举
    autoElection: true,
    
    // 配置 LiveKit 服务地址
    livekitUrl: 'wss://your-livekit-server.com',
    
    // 动态获取 Bot Token 的回调
    // 触发时机：当本机被选为 Relay 且需要建立影子连接时
    onRequestBotToken: (roomId) async {
      // 建议：从您的后端 API 获取一个临时的 Bot Token
      // 身份标识建议使用: relay-bot-{roomId}-{deviceId}
      return await myApi.getRelayBotToken(roomId);
    },
  ),
);
```

### Bot Token 权限要求

生成的 Bot Token 必须包含以下权限：

```json
{
  "video": {
    "canPublish": false,
    "canSubscribe": true,  <-- 必须为 true
    "hidden": true         <-- 建议为 true，使其在用户列表中不可见
  }
}
```

## 数据统计

由于采用双连接，流量统计需要分别查看：

*   **Dart 层统计**：用户连接的信令状态、自己发布的流状态。
*   **Go 层统计 (Relay Stats)**：
    *   `SFU Packets`: 从影子连接接收到的包数。
    *   `Local Packets`: 从本地分享接收到的包数。
    
可以通过 `autoCoord.getStatus()` 获取统一后的状态信息。
