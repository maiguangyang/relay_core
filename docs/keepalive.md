# 心跳保活

## 概述

Keepalive 模块负责检测 P2P 连接的存活状态，及时发现断线设备，用于：

- 断线检测
- RTT 测量
- 触发代理重选举
- 连接质量监控

## 工作原理

```
Relay 节点                    订阅者
    │                           │
    │──── Ping (via DataChannel)──→│
    │                           │
    │←────── Pong ──────────────│
    │                           │
    │    计算 RTT，更新状态       │
```

## 状态说明

| 状态 | 值 | 说明 |
|-----|---|------|
| Online | 1 | 正常响应 |
| Slow | 2 | RTT 超过慢响应阈值 |
| Offline | 3 | 超时无响应 |
| Unknown | 0 | 初始状态 |

## 使用教程

### 1. 创建心跳管理器

```dart
// 创建心跳管理器
// intervalMs: 心跳间隔（毫秒）
// timeoutMs: 超时时间（毫秒）
keepaliveCreate(
  "room-123".toNativeUtf8(),
  5000,   // 每 5 秒发送一次
  15000   // 15 秒无响应视为离线
);
```

### 2. 添加监控的 Peer

```dart
// 添加订阅者到监控列表
keepaliveAddPeer(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8()
);

keepaliveAddPeer(
  "room-123".toNativeUtf8(),
  "subscriber-2".toNativeUtf8()
);
```

### 3. 启动心跳检测

```dart
keepaliveStart("room-123".toNativeUtf8());
```

### 4. 处理 Ping 事件

需要在 Dart 层实现实际的 Ping 发送：

```dart
void handleEvent(int type, String roomId, String peerId, String data) {
  if (type == 23) { // EventTypePing
    // 通过 DataChannel 或信令发送 Ping
    sendPingToPeer(peerId);
  }
}

// 或使用 C 回调
void pingCallback(Pointer<Utf8> peerIdPtr) {
  final peerId = peerIdPtr.toDartString();
  sendPingToPeer(peerId);
}

setPingCallback(Pointer.fromFunction(pingCallback));
```

### 5. 处理 Pong 响应

当收到 Pong 时通知 Go 层：

```dart
void onPongReceived(String peerId) {
  keepaliveHandlePong(
    "room-123".toNativeUtf8(),
    peerId.toNativeUtf8()
  );
}
```

### 6. 查询状态

```dart
// 获取单个 Peer 状态
final status = keepaliveGetPeerStatus(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8()
);

switch (status) {
  case 1: print("在线"); break;
  case 2: print("响应缓慢"); break;
  case 3: print("离线"); break;
}

// 获取 RTT
final rtt = keepaliveGetPeerRTT(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8()
);
print("RTT: $rtt ms");

// 获取详细信息
final infoPtr = keepaliveGetPeerInfo(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8()
);
final info = jsonDecode(infoPtr.toDartString());
print("状态: ${info['status']}");
print("RTT: ${info['rtt_ms']} ms");
print("丢失次数: ${info['missed_pongs']}");
```

### 7. 获取所有 Peer 状态

```dart
final allPtr = keepaliveGetAllPeerInfo("room-123".toNativeUtf8());
final all = jsonDecode(allPtr.toDartString()) as List;

for (final peer in all) {
  print("${peer['peer_id']}: ${peer['status']} (RTT: ${peer['rtt_ms']}ms)");
}
```

### 8. 处理状态变更事件

```dart
void handleEvent(int type, String roomId, String peerId, String data) {
  switch (type) {
    case 20: // EventTypePeerOnline
      print("$peerId 上线");
      break;
    case 21: // EventTypePeerSlow
      final info = jsonDecode(data);
      print("$peerId 响应缓慢: RTT=${info['rtt_ms']}ms");
      break;
    case 22: // EventTypePeerOffline
      print("$peerId 离线");
      // 可能需要触发重选举
      handlePeerOffline(peerId);
      break;
  }
}
```

### 9. 移除 Peer

```dart
keepaliveRemovePeer(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8()
);
```

### 10. 停止和清理

```dart
keepaliveStop("room-123".toNativeUtf8());
keepaliveDestroy("room-123".toNativeUtf8());
```

## Ping/Pong 实现建议

### 方式一：通过 WebRTC DataChannel

```dart
// 创建 DataChannel
final dc = peerConnection.createDataChannel(
  "keepalive",
  RTCDataChannelInit()..ordered = false
);

dc.onMessage = (message) {
  if (message.text == "ping") {
    dc.send(RTCDataChannelMessage("pong"));
  } else if (message.text == "pong") {
    keepaliveHandlePong(roomId, peerId);
  }
};

void sendPingToPeer(String peerId) {
  dataChannels[peerId]?.send(RTCDataChannelMessage("ping"));
}
```

### 方式二：通过信令服务器

```dart
void sendPingToPeer(String peerId) {
  signalingService.send({
    "type": "ping",
    "from": myPeerId,
    "to": peerId,
    "timestamp": DateTime.now().millisecondsSinceEpoch
  });
}

signalingService.onMessage = (msg) {
  if (msg['type'] == 'ping') {
    signalingService.send({
      "type": "pong",
      "from": myPeerId,
      "to": msg['from']
    });
  } else if (msg['type'] == 'pong') {
    keepaliveHandlePong(roomId, msg['from']);
  }
};
```

## 配置建议

| 场景 | 间隔 | 超时 |
|-----|-----|-----|
| 稳定内网 | 5000ms | 15000ms |
| 普通网络 | 3000ms | 10000ms |
| 弱网环境 | 2000ms | 8000ms |

## 与选举集成

当检测到 Relay 离线时，触发重选举：

```dart
void handlePeerOffline(String peerId) {
  final currentProxy = electionGetProxy(relayId, roomId);
  
  if (currentProxy == peerId) {
    // Relay 离线，触发重选举
    electionTrigger(relayId, roomId);
  } else {
    // 普通订阅者离线，移除即可
    relayRoomRemoveSubscriber(roomId, peerId);
  }
}
```
