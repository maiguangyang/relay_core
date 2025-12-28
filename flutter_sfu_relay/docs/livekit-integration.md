# LiveKit 集成指南

## 概述

Flutter SFU Relay 与 LiveKit 配合使用，实现局域网优化：

```
                     互联网 (只需 1 条上行)
                            ↑
                    ┌───────┴───────┐
                    │ LiveKit 云端   │
                    └───────┬───────┘
                            │
┌───────────────────────────┼───────────────────────────┐
│                     局域网                              │
│                           │                            │
│      ┌───────────[Relay]──┴────────────┐              │
│      │         (AutoCoordinator)        │              │
│      ▼              ▼              ▼                   │
│   设备A           设备B           设备C                │
└───────────────────────────────────────────────────────┘
```

---

## 方式一：使用 AutoCoordinator（推荐 ⭐）

最简单的集成方式，一键自动处理所有事情：

```dart
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';
import 'package:livekit_client/livekit_client.dart';

class LiveKitRelayService {
  late Room _lkRoom;
  late AutoCoordinator _autoCoord;
  
  // ========== 连接 ==========
  
  Future<void> connect(String url, String token) async {
    // 1. 连接 LiveKit
    _lkRoom = Room();
    await _lkRoom.connect(url, token);
    
    // 2. 创建 DataChannel 信令适配器
    final signaling = LiveKitDataChannelSignaling(
      room: _lkRoom,
      localPeerId: _lkRoom.localParticipant!.identity,
    );
    
    // 3. 创建 AutoCoordinator
    _autoCoord = AutoCoordinator(
      roomId: _lkRoom.name!,
      localPeerId: _lkRoom.localParticipant!.identity,
      signaling: signaling,
      config: AutoCoordinatorConfig(
        deviceType: await _detectDeviceType(),
        connectionType: await _detectConnectionType(),
        powerState: await _detectPowerState(),
      ),
    );
    
    // 4. 监听状态
    _setupListeners();
    
    // 5. 一键启动！
    await _autoCoord.start();
  }
  
  void _setupListeners() {
    // 监听 Relay 变更
    _autoCoord.onRelayChanged.listen((relayId) {
      print('🎯 Relay 变更: $relayId');
      print('   是否本机: ${_autoCoord.isRelay}');
    });
    
    // 监听 Peer 加入/离开
    _autoCoord.onPeerJoined.listen((peerId) {
      print('👤 Peer 加入: $peerId');
    });
    
    _autoCoord.onPeerLeft.listen((peerId) {
      print('👤 Peer 离开: $peerId');
    });
    
    // 监听错误
    _autoCoord.onError.listen((error) {
      print('❌ 错误: $error');
    });
  }
  
  // ========== 断开 ==========
  
  Future<void> disconnect() async {
    await _autoCoord.stop();
    _autoCoord.dispose();
    await _lkRoom.disconnect();
  }
  
  // ========== 设备检测 ==========
  
  Future<DeviceType> _detectDeviceType() async {
    if (Platform.isAndroid || Platform.isIOS) {
      return DeviceType.mobile;
    }
    return DeviceType.pc;
  }
  
  Future<ConnectionType> _detectConnectionType() async {
    // 使用 connectivity_plus 包
    // final result = await Connectivity().checkConnectivity();
    return ConnectionType.wifi;
  }
  
  Future<PowerState> _detectPowerState() async {
    // 使用 battery_plus 包
    // final battery = Battery();
    return PowerState.battery;
  }
}
```

---

## LiveKit DataChannel 信令适配器

将 LiveKit DataChannel 适配为 SignalingBridge：

```dart
import 'dart:async';
import 'dart:convert';
import 'package:livekit_client/livekit_client.dart';
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';

class LiveKitDataChannelSignaling implements SignalingBridge {
  final Room room;
  final String localPeerId;
  
  final _messageController = StreamController<SignalingMessage>.broadcast();
  bool _isConnected = false;
  String? _currentRoomId;
  
  LiveKitDataChannelSignaling({
    required this.room,
    required this.localPeerId,
  }) {
    // 监听 DataChannel 消息
    room.addListener(_RoomListener(this));
  }
  
  @override
  bool get isConnected => _isConnected;
  
  @override
  Stream<SignalingMessage> get messages => _messageController.stream;
  
  @override
  Future<void> connect() async {
    _isConnected = room.connectionState == ConnectionState.connected;
  }
  
  @override
  Future<void> disconnect() async {
    _isConnected = false;
  }
  
  @override
  Future<void> joinRoom(String roomId, String peerId) async {
    _currentRoomId = roomId;
    // 广播加入消息
    await _broadcast({'type': 'join', 'peerId': peerId});
  }
  
  @override
  Future<void> leaveRoom(String roomId) async {
    await _broadcast({'type': 'leave', 'peerId': localPeerId});
    _currentRoomId = null;
  }
  
  @override
  Future<void> sendPing(String roomId, String targetPeerId) async {
    await _broadcast({'type': 'ping', 'targetPeerId': targetPeerId});
  }
  
  @override
  Future<void> sendPong(String roomId, String targetPeerId) async {
    await _broadcast({'type': 'pong', 'targetPeerId': targetPeerId});
  }
  
  @override
  Future<void> sendRelayClaim(String roomId, int epoch, double score) async {
    await _broadcast({
      'type': 'relayClaim',
      'epoch': epoch,
      'score': score,
    });
  }
  
  @override
  Future<void> sendRelayChanged(String roomId, String relayId, int epoch) async {
    await _broadcast({
      'type': 'relayChanged',
      'relayId': relayId,
      'epoch': epoch,
    });
  }
  
  @override
  Future<void> sendOffer(String roomId, String targetPeerId, String sdp) async {
    await _broadcast({
      'type': 'offer',
      'targetPeerId': targetPeerId,
      'sdp': sdp,
    });
  }
  
  @override
  Future<void> sendAnswer(String roomId, String targetPeerId, String sdp) async {
    await _broadcast({
      'type': 'answer',
      'targetPeerId': targetPeerId,
      'sdp': sdp,
    });
  }
  
  @override
  Future<void> sendCandidate(String roomId, String targetPeerId, String candidate) async {
    await _broadcast({
      'type': 'candidate',
      'targetPeerId': targetPeerId,
      'candidate': candidate,
    });
  }
  
  @override
  void dispose() {
    _messageController.close();
  }
  
  Future<void> _broadcast(Map<String, dynamic> data) async {
    data['peerId'] = localPeerId;
    data['roomId'] = _currentRoomId;
    
    await room.localParticipant?.publishData(
      utf8.encode(jsonEncode(data)),
      reliable: true,
    );
  }
  
  void _onDataReceived(List<int> data, RemoteParticipant? participant) {
    if (participant == null) return;
    
    try {
      final json = jsonDecode(utf8.decode(data)) as Map<String, dynamic>;
      final type = json['type'] as String?;
      
      final message = SignalingMessage(
        type: _parseType(type),
        roomId: json['roomId'] ?? _currentRoomId ?? '',
        peerId: participant.identity,
        targetPeerId: json['targetPeerId'],
        data: json,
      );
      
      _messageController.add(message);
    } catch (e) {
      // 忽略解析错误
    }
  }
  
  SignalingMessageType _parseType(String? type) {
    switch (type) {
      case 'join': return SignalingMessageType.join;
      case 'leave': return SignalingMessageType.leave;
      case 'ping': return SignalingMessageType.ping;
      case 'pong': return SignalingMessageType.pong;
      case 'relayClaim': return SignalingMessageType.relayClaim;
      case 'relayChanged': return SignalingMessageType.relayChanged;
      case 'offer': return SignalingMessageType.offer;
      case 'answer': return SignalingMessageType.answer;
      case 'candidate': return SignalingMessageType.candidate;
      default: return SignalingMessageType.error;
    }
  }
}

class _RoomListener extends RoomListener {
  final LiveKitDataChannelSignaling signaling;
  
  _RoomListener(this.signaling);
  
  @override
  void onDataReceived(List<int> data, RemoteParticipant? participant, String? topic) {
    signaling._onDataReceived(data, participant);
  }
}
```

---

## 完整示例

```dart
class MyApp extends StatefulWidget {
  @override
  _MyAppState createState() => _MyAppState();
}

class _MyAppState extends State<MyApp> {
  final _service = LiveKitRelayService();
  
  @override
  void initState() {
    super.initState();
    _connect();
  }
  
  Future<void> _connect() async {
    await _service.connect(
      'wss://your-livekit-server.com',
      'your-token',
    );
    
    setState(() {});
  }
  
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text('状态: ${_service._autoCoord.state.name}'),
            Text('是否 Relay: ${_service._autoCoord.isRelay}'),
            Text('分数: ${_service._autoCoord.localScore}'),
            Text('Relay: ${_service._autoCoord.currentRelay ?? "无"}'),
          ],
        ),
      ),
    );
  }
  
  @override
  void dispose() {
    _service.disconnect();
    super.dispose();
  }
}
```

---

## 评分规则

| 设备 | 分 | 网络 | 分 | 电源 | 分 |
|------|:--:|------|:--:|------|:--:|
| PC | 40 | Ethernet | 40 | PluggedIn | 20 |
| Pad | 30 | WiFi | 30 | Battery | 10 |
| TV | 25 | Cellular | 10 | LowBattery | 0 |
| Mobile | 20 | Unknown | 0 | Unknown | 0 |

**最优**: PC + Ethernet + PluggedIn = **100 分**

---

## 网络变化处理

使用 `connectivity_plus` 监听网络变化，当网络切换时自动断开重连：

```dart
import 'package:connectivity_plus/connectivity_plus.dart';

class LiveKitRelayService {
  StreamSubscription<List<ConnectivityResult>>? _connectivitySubscription;
  ConnectionType? _lastConnectionType;
  
  void startNetworkListener() {
    _connectivitySubscription = Connectivity().onConnectivityChanged.listen((results) {
      final currentType = _getConnectionType(results);
      
      // 如果在会议中且网络类型变化
      if (_isInMeeting && _lastConnectionType != null && currentType != _lastConnectionType) {
        print('[Network] 网络切换: $_lastConnectionType -> $currentType');
        _handleNetworkChange(currentType);
      }
      
      _lastConnectionType = currentType;
    });
  }
  
  void _handleNetworkChange(ConnectionType newType) async {
    // 断开当前连接，避免 SDK 状态混乱
    await disconnect();
    
    // 显示提示，让用户重新加入
    showMessage('网络已切换，请重新加入房间');
  }
  
  ConnectionType _getConnectionType(List<ConnectivityResult> results) {
    if (results.contains(ConnectivityResult.ethernet)) return ConnectionType.ethernet;
    if (results.contains(ConnectivityResult.wifi)) return ConnectionType.wifi;
    if (results.contains(ConnectivityResult.mobile)) return ConnectionType.cellular;
    return ConnectionType.unknown;
  }
  
  void dispose() {
    _connectivitySubscription?.cancel();
    // ...
  }
}
```

> ⚠️ **注意**: 网络切换时直接重连可能导致 LiveKit SDK 内部状态异常。
> 建议先完全断开，等待用户手动重新加入。

---

## 正确的清理流程

退出房间时确保完全清理：

```dart
Future<void> disconnect() async {
  // 1. 停止 AutoCoordinator（会断开影子连接）
  try {
    await _autoCoord.stop().timeout(Duration(seconds: 2));
    _autoCoord.dispose();
  } catch (e) {
    print('Error stopping AutoCoordinator: $e');
  }
  
  // 2. 断开 LiveKit Room
  try {
    await _lkRoom.disconnect().timeout(Duration(seconds: 2));
    _lkRoom.dispose();
  } catch (e) {
    print('Error disconnecting room: $e');
  }
  
  // 3. 等待 SDK 异步清理
  await Future.delayed(Duration(seconds: 2));
}
```

---

## 影子连接（Relay Bot）

当设备成为 Relay 时，可以创建一个 "影子连接" 到 LiveKit 云端：

```dart
_autoCoord = AutoCoordinator(
  // ...
  config: AutoCoordinatorConfig(
    // ...
    livekitUrl: 'wss://your-livekit.com',
    onRequestBotToken: (roomId) async {
      // 向服务器请求 Bot Token
      // Bot 的 identity 建议使用: "relay-bot-${relayPeerId}"
      return await api.getBotToken(roomId, identity: 'relay-bot-$peerId');
    },
  ),
);
```

影子连接用于将局域网流量同步到云端，其他非局域网设备可以通过 Bot 接收流。

