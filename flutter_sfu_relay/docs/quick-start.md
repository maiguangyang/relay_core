# 快速开始

## 安装

```yaml
dependencies:
  flutter_sfu_relay:
    path: ../flutter_sfu_relay
```

## 初始化

```dart
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';

void main() {
  // 初始化回调系统（必须在使用其他 API 之前调用）
  EventHandler.init();
  LogHandler.init();
  PingHandler.init();
  
  // 可选：监听日志
  LogHandler.logs.listen((log) {
    print('[Go] ${log.level}: ${log.message}');
  });
  
  // 可选：设置日志级别 (0=Debug, 1=Info, 2=Warn, 3=Error)
  SfuRelay.instance.setLogLevel(1);
  
  runApp(MyApp());
}
```

---

## 方式一：AutoCoordinator（推荐 ⭐）

**真正的一键自动代理** - 内部完整处理选举、Ping/Pong、故障切换：

```dart
class RelayService {
  late AutoCoordinator _autoCoord;
  
  Future<void> start(String roomId, String peerId, String wsUrl) async {
    // 创建信令
    final signaling = WebSocketSignaling(
      url: wsUrl,
      localPeerId: peerId,
    );
    
    // 创建 AutoCoordinator
    _autoCoord = AutoCoordinator(
      roomId: roomId,
      localPeerId: peerId,
      signaling: signaling,
      config: AutoCoordinatorConfig(
        deviceType: DeviceType.pc,           // 设备: PC=40, Pad=30, Mobile=20
        connectionType: ConnectionType.wifi, // 网络: Ethernet=40, WiFi=30
        powerState: PowerState.pluggedIn,    // 电源: PluggedIn=20, Battery=10
        electionTimeoutMs: 3000,             // 选举超时
        autoElection: true,                  // 自动选举
      ),
    );
    
    // 监听状态
    _autoCoord.onStateChanged.listen((state) {
      switch (state) {
        case AutoCoordinatorState.electing:
          print('🗳️ 选举中...');
          break;
        case AutoCoordinatorState.asRelay:
          print('👑 成为 Relay！');
          break;
        case AutoCoordinatorState.connected:
          print('✅ 已连接到 Relay');
          break;
        default:
          break;
      }
    });
    
    // 监听 Relay 变更
    _autoCoord.onRelayChanged.listen((relayId) {
      print('📡 当前 Relay: $relayId');
    });
    
    // 一键启动 - 自动处理一切！
    await _autoCoord.start();
    
    print('本机分数: ${_autoCoord.localScore}');
  }
  
  void injectRtp(bool isVideo, List<int> data) {
    if (_autoCoord.isRelay) {
      _autoCoord.injectSfuPacket(isVideo, data);
    }
  }
  
  Future<void> stop() async {
    await _autoCoord.stop();
    _autoCoord.dispose();
  }
}
```

### AutoCoordinator 自动处理

| 功能 | 说明 |
|------|------|
| ✅ 信令连接 | 自动连接 WebSocket |
| ✅ 回调初始化 | EventHandler, LogHandler, PingHandler |
| ✅ 分数计算 | 设备(40) + 网络(40) + 电源(20) |
| ✅ 选举广播 | 自动发送 claim |
| ✅ 选举超时 | 无响应时自动成为 Relay |
| ✅ Ping/Pong | 自动心跳转发 |
| ✅ 故障切换 | Relay 离线自动重选 |
| ✅ 冲突解决 | epoch > score > peerId |

---

## 方式二：Coordinator（手动控制）

需要自己处理信令和事件：

```dart
class ManualRelayService {
  late Coordinator _coordinator;
  
  void start(String roomId, String peerId) {
    _coordinator = Coordinator(
      roomId: roomId,
      localPeerId: peerId,
    );
    _coordinator.enable();
    
    // 需要手动处理事件
    EventHandler.events.listen((event) {
      // 需手动处理...
    });
    
    // 需要手动处理 Ping
    PingHandler.pingRequests.listen((req) {
      // 需手动通过信令发送...
    });
  }
}
```

---

## 方式三：RelayRoomP2P（完全控制）

底层 P2P 连接管理：

```dart
final room = RelayRoomP2P('room-1');

room.create(iceServers: [
  {'urls': ['stun:stun.l.google.com:19302']}
]);

room.becomeRelay('my-peer-id');

final answer = room.addSubscriber('subscriber-1', offerSdp);
room.injectSfu(isVideo: true, data: rtpData);

room.destroy();
```

---

## 枚举定义

```dart
// 设备类型 (影响评分)
enum DeviceType {
  unknown(0),   // 0 分
  pc(1),        // 40 分
  pad(2),       // 30 分
  tv(3),        // 25 分
  mobile(4);    // 20 分
}

// 连接类型 (影响评分)
enum ConnectionType {
  unknown(0),   // 0 分
  ethernet(1),  // 40 分
  wifi(2),      // 30 分
  cellular(3);  // 10 分
}

// 电源状态 (影响评分)
enum PowerState {
  unknown(0),     // 0 分
  pluggedIn(1),   // 20 分
  battery(2),     // 10 分
  lowBattery(3);  // 0 分 (减分)
}
```

---

## 下一步

- [架构概述](./architecture.md) - 模块结构
- [选举系统](./election.md) - 评分规则
- [LiveKit 集成](./livekit-integration.md) - 与 LiveKit 配合
- [API 参考](./api-reference.md) - 完整 API
