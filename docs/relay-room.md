# Relay P2P 管理

## 概述

`RelayRoom` 是 Relay Core 的核心模块，负责管理 Relay 节点与局域网订阅者之间的 P2P WebRTC 连接。

## 基本流程

```
1. 创建房间
2. 成为 Relay 节点
3. 添加订阅者（建立 P2P 连接）
4. 注入 RTP 数据（透传给订阅者）
5. 处理本地分享切换
6. 清理资源
```

## 使用教程

### 1. 创建 Relay 房间

```dart
// Dart FFI 示例

// 创建房间，配置 ICE 服务器
final iceServers = jsonEncode([
  {"urls": ["stun:stun.l.google.com:19302"]},
  {
    "urls": ["turn:your-turn-server.com:3478"],
    "username": "user",
    "credential": "pass"
  }
]);

final result = relayRoomCreate(
  "room-123".toNativeUtf8(),
  iceServers.toNativeUtf8()
);

if (result == 0) {
  print("RelayRoom 创建成功");
}
```

### 2. 成为 Relay 节点

当选举系统选中本机为 Relay 时：

```dart
// 标记本机为 Relay 节点
relayRoomBecomeRelay(
  "room-123".toNativeUtf8(),
  "my-peer-id".toNativeUtf8()
);

// 检查是否是 Relay
final isRelay = relayRoomIsRelay("room-123".toNativeUtf8());
print("Is Relay: ${isRelay == 1}");
```

### 3. 添加订阅者

当局域网内其他设备请求接收流时：

```dart
// 收到远端 Offer 后，创建连接并返回 Answer
final answerPtr = relayRoomAddSubscriber(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8(),
  remoteOfferSDP.toNativeUtf8()
);

if (answerPtr != nullptr) {
  final answer = answerPtr.toDartString();
  // 通过信令服务器发送 Answer 给订阅者
  signalingService.sendAnswer("subscriber-1", answer);
  freeString(answerPtr);
}
```

### 4. 添加 ICE 候选

```dart
// 处理收到的 ICE 候选
relayRoomAddICECandidate(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8(),
  jsonEncode({
    "candidate": candidateString,
    "sdpMid": "0",
    "sdpMLineIndex": 0
  }).toNativeUtf8()
);
```

### 5. 注入 RTP 数据

从 LiveKit 或其他源获取 RTP 包后注入：

```dart
// 注入视频 RTP 包
relayRoomInjectSFU(
  "room-123".toNativeUtf8(),
  1,  // isVideo = true
  rtpData.cast<Void>(),
  rtpData.length
);

// 注入音频 RTP 包
relayRoomInjectSFU(
  "room-123".toNativeUtf8(),
  0,  // isVideo = false
  rtpData.cast<Void>(),
  rtpData.length
);
```

### 6. 本地分享

当用户发起屏幕分享时：

```dart
// 开始本地分享
relayRoomStartLocalShare(
  "room-123".toNativeUtf8(),
  "sharer-peer-id".toNativeUtf8()
);

// 此后使用 InjectLocal 注入分享者的 RTP 包
relayRoomInjectLocal(...);

// 停止分享（切回 SFU 流）
relayRoomStopLocalShare("room-123".toNativeUtf8());
```

### 7. 处理重协商

当源切换时，需要更新订阅者的 SDP：

```dart
// 触发重协商，获取所有订阅者的新 Offer
final offersPtr = relayRoomTriggerRenegotiation("room-123".toNativeUtf8());
final offersJson = offersPtr.toDartString();
final offers = jsonDecode(offersJson) as Map<String, String>;

// 发送 Offer 给每个订阅者
for (final entry in offers.entries) {
  signalingService.sendOffer(entry.key, entry.value);
}

// 收到 Answer 后处理
relayRoomHandleAnswer(
  "room-123".toNativeUtf8(),
  "subscriber-1".toNativeUtf8(),
  answerSDP.toNativeUtf8()
);
```

### 8. 获取房间状态

```dart
final statusPtr = relayRoomGetStatus("room-123".toNativeUtf8());
final status = jsonDecode(statusPtr.toDartString());

print("订阅者数量: ${status['subscriber_count']}");
print("是否为 Relay: ${status['is_relay']}");
print("源切换状态: ${status['source_switcher']}");
```

### 9. 清理资源

```dart
// 移除所有订阅者
final subscribers = jsonDecode(
  relayRoomGetSubscribers("room-123".toNativeUtf8()).toDartString()
);

for (final peerId in subscribers) {
  relayRoomRemoveSubscriber(
    "room-123".toNativeUtf8(),
    peerId.toNativeUtf8()
  );
}

// 销毁房间
relayRoomDestroy("room-123".toNativeUtf8());
```

## 事件处理

RelayRoom 会通过 EventCallback 发送以下事件：

| 事件类型 | 值 | 说明 | data 内容 |
|---------|---|------|----------|
| 订阅者加入 | 10 | 新订阅者连接成功 | - |
| 订阅者离开 | 11 | 订阅者断开 | - |
| 需要重协商 | 12 | Track 变化需要更新 SDP | `{type: "offer", sdp: "..."}` |
| ICE 候选 | 5 | 生成了 ICE 候选 | ICE 候选 JSON |
| 错误 | 4 | 发生错误 | `{code: 500, message: "..."}` |

```dart
void handleEvent(int type, String roomId, String peerId, String data) {
  switch (type) {
    case 10: // 订阅者加入
      print("新订阅者: $peerId");
      break;
    case 12: // 需要重协商
      final offer = jsonDecode(data)['sdp'];
      signalingService.sendOffer(peerId, offer);
      break;
    case 5: // ICE 候选
      signalingService.sendICECandidate(peerId, data);
      break;
  }
}
```

## 最佳实践

1. **ICE 服务器配置**：生产环境建议配置 TURN 服务器以确保穿透成功
2. **错误处理**：检查所有 API 返回值
3. **内存管理**：使用 `freeString()` 释放返回的字符串
4. **重协商**：源切换后务必触发重协商
5. **超时处理**：使用 Keepalive 检测断线
