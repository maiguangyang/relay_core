# API 参考

## Core 模块

### SfuRelay

SDK 入口单例。

```dart
// 获取版本
String version = SfuRelay.instance.version;

// 设置日志级别 (0=Debug, 1=Info, 2=Warn, 3=Error)
SfuRelay.instance.setLogLevel(1);

// 清理所有资源
SfuRelay.instance.cleanupAll();

// 获取支持的编解码器
List<String> videoCodecs = SfuRelay.instance.getSupportedVideoCodecs();
List<String> audioCodecs = SfuRelay.instance.getSupportedAudioCodecs();
```

### AutoCoordinator（推荐 ⭐）

真正的一键自动代理，内置信令集成、选举、故障切换。

```dart
final autoCoord = AutoCoordinator(
  roomId: 'room-1',
  localPeerId: 'peer-1',
  signaling: WebSocketSignaling(url: 'ws://...', localPeerId: 'peer-1'),
  config: AutoCoordinatorConfig(
    deviceType: DeviceType.pc,           // 分数: PC=40, Pad=30, Mobile=20
    connectionType: ConnectionType.wifi, // 分数: Ethernet=40, WiFi=30
    powerState: PowerState.pluggedIn,    // 分数: PluggedIn=20, Battery=10
    electionTimeoutMs: 1000,             // 选举超时 (重选 1s, 初始 2s)
    autoElection: true,                  // 自动选举
    maxElectionFailures: 3,              // 连续 N 次选举失败后降级为直连 SFU (0=禁用Relay)
    recoveryDelayMs: 30000,              // 降级后自动恢复延迟 (默认 30秒)
    // 影子连接配置 (当设备成为 Relay 时连接 LiveKit 云端)
    livekitUrl: 'wss://your-livekit.com', // LiveKit 服务器 URL
    onRequestBotToken: (roomId) async {   // 动态获取 Bot Token
      return await api.getBotToken(roomId);
    },
    // 带宽优化回调：当 P2P 可用时取消云端订阅，避免双重拉流
    onCloudSubscriptionChanged: (peerId, shouldSubscribe) {
      // peerId: 屏幕共享者 ID
      // shouldSubscribe: true=恢复云端订阅, false=取消云端订阅
      final pub = findScreenSharePublication(peerId);
      shouldSubscribe ? pub?.subscribe() : pub?.unsubscribe();
    },
  ),
);

// 生命周期
Future<void> start();       // 一键启动！
Future<void> stop();
void dispose();

// RTP 注入
bool injectSfuPacket(bool isVideo, List<int> data);
bool injectLocalPacket(bool isVideo, List<int> data);

// 本地分享
bool startLocalShare();
bool stopLocalShare();
void notifyScreenShareStarted(); // 通知 Coordinator 本地屏幕共享开始
void notifyScreenShareStopped(); // 通知 Coordinator 本地屏幕共享结束

// 手动触发
void triggerElection();

// 状态
Map<String, dynamic> getStatus();

// 属性
AutoCoordinatorState get state;
bool get isRelay;
String? get currentRelay;
int get currentEpoch;
double get localScore;
Set<String> get peers;
String? get screenSharerPeerId; // 当前屏幕共享者 PeerId (null表示无人共享)

// 事件流
Stream<AutoCoordinatorState> get onStateChanged;
Stream<String> get onRelayChanged;
Stream<String> get onPeerJoined;
Stream<String> get onPeerLeft;
Stream<String?> get onScreenShareChanged; // 屏幕共享状态变更 (null=结束)
Stream<String> get onError;
```

### Coordinator

手动代理管理器（需自行处理信令和事件）。

```dart
final coordinator = Coordinator(
  roomId: 'room-1',
  localPeerId: 'peer-1',
);

// 生命周期
bool enable();
bool disable();

// Peer 管理
bool addPeer(String peerId, {DeviceType, ConnectionType, PowerState});
bool removePeer(String peerId);
bool handlePong(String peerId);

// Relay 协调
bool setRelay(String relayId, int epoch);
bool receiveClaim(String peerId, int epoch, double score);

// RTP 注入
bool injectSfuPacket(bool isVideo, Uint8List data);
bool injectLocalPacket(bool isVideo, Uint8List data);

// 本地分享
bool startLocalShare(String sharerId);
bool stopLocalShare();

// 状态
Map<String, dynamic> getStatus();
bool get isRelay;
```

---

## Room 模块

### RelayRoomP2P

底层 P2P 连接管理。

```dart
final room = RelayRoomP2P('room-1');

// 房间生命周期
bool create({List<Map<String, dynamic>> iceServers});
bool destroy();
bool becomeRelay(String peerId);
bool? get isRelay;

// 订阅者管理
String? addSubscriber(String peerId, String offerSdp);
bool removeSubscriber(String peerId);
List<String> getSubscribers();
int get subscriberCount;

// ICE
bool addIceCandidate(String peerId, Map<String, dynamic> candidate);

// SDP 重协商
Map<String, String>? triggerRenegotiation();
String? createOffer(String peerId);
bool handleAnswer(String peerId, String answerSdp);

// RTP 注入
bool injectSfu({required bool isVideo, required Uint8List data});
bool injectLocal({required bool isVideo, required Uint8List data});

// 本地分享
bool startLocalShare(String sharerId);
bool stopLocalShare();

// 状态
Map<String, dynamic>? getStatus();
```

### RelayRoom

高级房间封装（含信令）。

```dart
final room = RelayRoom(
  roomId: 'room-1',
  localPeerId: 'peer-1',
  signaling: WebSocketSignaling(url: '...', localPeerId: '...'),
);

// 生命周期
Future<void> join({DeviceType, ConnectionType, PowerState});
Future<void> leave();
void dispose();

// 本地分享
Future<void> startSharing();
Future<void> stopSharing();

// 状态
RoomState get state;
bool get isRelay;
List<PeerInfo> get peers;
Map<String, dynamic> getStatus();

// 事件流
Stream<RoomState> get onStateChanged;
Stream<PeerInfo> get onPeerJoined;
Stream<String> get onPeerLeft;
Stream<String> get onRelayChanged;
Stream<String> get onError;
```

---

## Election 模块

### Election

独立选举 API。

```dart
final election = Election(roomId: 'room-1');

bool enable();
bool disable();

bool updateDeviceInfo({
  required String peerId,
  required DeviceType deviceType,
  required ConnectionType connectionType,
  required PowerState powerState,
});

bool updateNetworkMetrics({
  required String peerId,
  required int bandwidth,
  required int latency,
  required double packetLoss,
});

ElectionResult? trigger();
String? getProxy();
List<CandidateInfo> getCandidates();
```

### Failover

故障切换管理。

```dart
final failover = Failover(roomId: 'room-1', localPeerId: 'peer-1');

bool enable();
bool disable();

bool setCurrentRelay(String relayId, {required int epoch});
bool updateLocalScore(double score);
bool receiveClaim(String peerId, {required int epoch, required double score});

FailoverState? getState();
```

### ProxyManager

自动代理状态管理。

```dart
final proxyManager = ProxyManager(
  roomId: 'room-1',
  localPeerId: 'peer-1',
  deviceType: DeviceType.mobile,
  connectionType: ConnectionType.wifi,
  powerState: PowerState.battery,
);

Future<void> start();
Future<void> stop();

void triggerElection();
void handleRelayClaim(String peerId, int epoch, double score);
void handleRelayChanged(String relayId, int epoch);
void startFailover();

Map<String, dynamic> getStatus();
void dispose();

// 属性
ProxyState get state;
RelayInfo? get currentRelay;
bool get isRelay;
int get currentEpoch;

// 事件流
Stream<ProxyState> get onStateChanged;
Stream<RelayInfo> get onRelayChanged;
Stream<void> get onElectionTriggered;
```

---

## Media 模块

### SourceSwitcher

源切换器。

```dart
final switcher = SourceSwitcher(roomId: 'room-1');

bool create();
bool destroy();

bool injectSfuPacket(bool isVideo, Uint8List data);
bool injectLocalPacket(bool isVideo, Uint8List data);

bool startLocalShare(String sharerId);
bool stopLocalShare();
bool get isLocalSharing;

Map<String, dynamic> getStatus();
```

### JitterBuffer

抖动缓冲。

```dart
final jitter = JitterBuffer('room-1-video');

bool create({bool enabled = true, int targetDelayMs = 50});
bool destroy();

bool setEnabled(bool enabled);
bool setDelay(int delayMs);
bool flush();

JitterBufferStats? getStats();
bool get isEnabled;
```

---

## Monitoring 模块

### Keepalive

心跳检测。

```dart
final keepalive = Keepalive(roomId: 'room-1');

bool create({int intervalMs = 1000, int timeoutMs = 3000});
bool destroy();
bool start();
bool stop();

bool addPeer(String peerId);
bool removePeer(String peerId);
bool handlePong(String peerId);

PeerStatus getPeerStatus(String peerId);
int getPeerRtt(String peerId);
Map<String, dynamic> getAllPeerInfo();
```

### Stats

流量统计。

```dart
final stats = Stats(roomId: 'room-1');

bool create();
bool destroy();

bool addBytesIn(String peerId, int bytes);
bool addBytesOut(String peerId, int bytes);

Map<String, dynamic> getSnapshot();
```

### NetworkProbe

网络探测。

```dart
final probe = NetworkProbe('room-1');

bool create();
bool destroy();

NetworkMetrics? getMetrics(String peerId);
List<NetworkMetrics> getAllMetrics();
```

---

## Callbacks 模块

### EventHandler

Go 层事件。

```dart
EventHandler.init();

Stream<SfuEvent> get events;
```

### LogHandler

Go 层日志。

```dart
LogHandler.init();

Stream<LogMessage> get logs;
```

### PingHandler

Ping 请求。

```dart
PingHandler.init();

Stream<PingRequest> get pingRequests;
```

---

## Screen Share 模块

### ScreenShareHelper

屏幕共享自排除和原生悬浮控制栏。

```dart
import 'package:flutter_sfu_relay/screen_share_helper.dart';

// 初始化（一次性调用）
ScreenShareHelper.initialize();

// 设置停止共享回调
ScreenShareHelper.onStopSharingRequested = () {
  // 用户点击了悬浮栏的"结束共享"按钮
  stopScreenShare();
};

// 检查平台支持
bool supported = ScreenShareHelper.isSupported; // macOS/Windows/Linux

// 开始屏幕共享时
await ScreenShareHelper.setExcludeFromCapture(true);  // 窗口自排除
await ScreenShareHelper.showOverlay();                 // 显示悬浮控制栏

// 结束屏幕共享时
await ScreenShareHelper.setExcludeFromCapture(false); // 恢复可捕获
await ScreenShareHelper.hideOverlay();                 // 隐藏悬浮控制栏
```

**平台实现：**

| 功能 | macOS | Windows | Linux |
|------|-------|---------|-------|
| 窗口自排除 | `sharingType = .none` | `SetWindowDisplayAffinity` | ❌ 无标准 API |
| 浮动工具栏 | NSWindow | Win32 POPUP | GTK+ POPUP |
| 绿色角落边框 | ✅ | ✅ | ✅ |
| 结束共享按钮 | ✅ | ✅ | ✅ |
