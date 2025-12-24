# 自动代理模式 (Coordinator)

## 概述

`Coordinator` 是一键启用的自动代理模式，集成了所有核心组件：

- **Keepalive** - 心跳检测
- **Election** - 代理选举
- **Failover** - 故障切换
- **SourceSwitcher** - 源切换

只需调用 `CoordinatorEnable` 即可自动管理 Relay 选举和故障恢复。

## 快速开始

```dart
// 1. 一键启用
coordinatorEnable(roomId, myPeerId);

// 2. 更新本机设备信息（影响选举分数）
coordinatorUpdateLocalDevice(roomId, 
  DeviceType.PC,           // 1=PC, 2=Pad, 3=Mobile, 4=TV
  ConnectionType.Ethernet, // 1=Ethernet, 2=WiFi, 3=Cellular
  PowerState.PluggedIn     // 1=插电, 2=电池, 3=低电量
);

// 3. 添加其他 Peer
coordinatorAddPeer(roomId, peerId, deviceType, connType, powerState);

// 4. 收到 Pong 时（通过信令）
coordinatorHandlePong(roomId, peerId);

// 5. 完成后禁用
coordinatorDisable(roomId);
```

## 自动故障切换流程

```
正常运行中...
        │
Keepalive 检测 Relay 连续 2 次无响应
        ↓ (自动)
FailoverManager 触发
        ↓
计算退避时间（分数高等待短）
PC (100分) → 0ms
Mobile (40分) → 600ms
        ↓ (自动)
最高分节点先完成选举
        ↓ (自动)
广播 Relay 声明
        ↓ (自动)
收到声明的节点放弃
        ↓
新 Relay 接管 ✅
```

## 冲突解决

当多个节点同时声明成为 Relay（信令延迟导致）：

| 优先级 | 规则 |
|-------|------|
| 1 | epoch 更高者优先 |
| 2 | 同 epoch，分数高者优先 |
| 3 | 分数相同，PeerID 字典序大者优先 |

## 事件处理

```dart
void onEvent(int type, String roomId, String peerId, String data) {
  switch (type) {
    case EventTypeProxyChange: // 6
      final info = jsonDecode(data);
      if (info['action'] == 'become_relay') {
        // 本机成为 Relay
        print('I am now the relay!');
      } else if (info['proxy_id'] != null) {
        // Relay 变更
        print('New relay: ${info['proxy_id']}');
      }
      break;
      
    case EventTypePeerOffline: // 22
      print('$peerId went offline');
      break;
  }
}
```

## 信令集成

Coordinator 需要通过信令传递以下消息：

### 1. Ping/Pong

```dart
// Go 通过 EventTypePing (23) 请求发送 Ping
void onPingRequest(peerId) {
  signaling.send(peerId, {'type': 'ping', 'from': myPeerId});
}

// 收到 Ping 回复 Pong
void onSignalingMessage(msg) {
  if (msg['type'] == 'ping') {
    signaling.send(msg['from'], {'type': 'pong', 'from': myPeerId});
  } else if (msg['type'] == 'pong') {
    coordinatorHandlePong(roomId, msg['from']);
  }
}
```

### 2. Relay 声明

```dart
// 收到 become_relay 事件时广播
void onBecomeRelay(roomId) {
  final epoch = getCurrentEpoch();
  final score = myScore;
  
  signaling.broadcast({
    'type': 'relay_claim',
    'peer_id': myPeerId,
    'epoch': epoch,
    'score': score,
  });
}

// 收到声明
void onRelayClaim(msg) {
  coordinatorReceiveClaim(
    roomId, 
    msg['peer_id'], 
    msg['epoch'], 
    msg['score']
  );
}
```

## Dart FFI 绑定

```dart
typedef _CoordinatorEnableC = Int32 Function(Pointer<Utf8> roomID, Pointer<Utf8> localPeerID);
typedef _CoordinatorEnableDart = int Function(Pointer<Utf8> roomID, Pointer<Utf8> localPeerID);

final coordinatorEnable = lib
    .lookup<NativeFunction<_CoordinatorEnableC>>('CoordinatorEnable')
    .asFunction<_CoordinatorEnableDart>();
```

## 与独立组件的关系

| 场景 | 推荐使用 |
|------|---------|
| 完整自动代理 | `Coordinator*` 系列 |
| 只需选举 | `Election*` 系列 |
| 只需心跳 | `Keepalive*` 系列 |
| 只需源切换 | `SourceSwitcher*` 系列 |
| 自定义故障切换 | `Failover*` 系列 |

使用 `Coordinator` 时，无需单独创建 Keepalive、Election、SourceSwitcher，它们都由 Coordinator 内部管理。
