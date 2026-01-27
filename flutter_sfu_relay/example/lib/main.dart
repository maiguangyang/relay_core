import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';
import 'package:livekit_client/livekit_client.dart' as lk;
import 'package:window_manager/window_manager.dart';

import 'pages/home_page.dart';

void main() async {
  // 使用 runZonedGuarded 捕获所有未处理的异步错误
  // 这可以防止 LiveKit SDK 内部的类型转换错误导致程序卡住
  runZonedGuarded(
    () async {
      WidgetsFlutterBinding.ensureInitialized();

      // 设置 Flutter 框架错误处理
      FlutterError.onError = (FlutterErrorDetails details) {
        // 检查是否是 LiveKit SDK 的已知类型转换错误
        final errorStr = details.exception.toString();
        if (errorStr.contains('LocalParticipant') &&
            errorStr.contains('RemoteParticipant')) {
          // 忽略这个 LiveKit SDK 的已知 bug，不要让它导致程序崩溃
          debugPrint('[LiveKit SDK Bug] Ignoring type cast error: $errorStr');
          return;
        }
        // 其他错误正常处理
        FlutterError.presentError(details);
      };

      // 初始化 window_manager（桌面平台全屏功能需要）
      if (!kIsWeb &&
          (Platform.isMacOS || Platform.isWindows || Platform.isLinux)) {
        await windowManager.ensureInitialized();
      }

      // 确保在启动时清理旧的 Go 回调 (防止 Hot Restart 导致的 Crash)
      // Go层现在有50ms grace period来让进行中的回调完成
      try {
        SfuRelay.instance.cleanupAll();
      } catch (e) {
        print('Cleanup failed (expected on first run): $e');
      }
      runApp(const MyApp());
    },
    (error, stackTrace) {
      // 全局异常处理器 - 捕获所有未处理的异步错误
      final errorStr = error.toString();

      // 忽略 LiveKit SDK 的已知类型转换 bug
      if (errorStr.contains('LocalParticipant') &&
          errorStr.contains('RemoteParticipant')) {
        debugPrint('[LiveKit SDK Bug] Caught and ignored: $errorStr');
        return;
      }

      // 其他错误打印日志
      debugPrint('[Unhandled Error] $error');
      debugPrint('$stackTrace');
    },
  );
}

class MyApp extends StatefulWidget {
  const MyApp({super.key});

  @override
  State<MyApp> createState() => _MyAppState();
}

class _MyAppState extends State<MyApp> {
  // LiveKit
  lk.Room? _lkRoom;
  lk.LocalParticipant? _localParticipant;
  lk.EventsListener<lk.RoomEvent>? _roomListener;

  // SFU Relay
  AutoCoordinator? _autoCoord;
  String _version = 'Loading...';
  AutoCoordinatorState _relayState = AutoCoordinatorState.idle;
  bool _isRelay = false;
  String? _currentRelay;
  double _localScore = 0;

  // UI State
  final List<String> _logs = [];
  final List<String> _peers = [];
  bool _isConnecting = false;

  // LiveKit Config (请修改为你的 LiveKit 服务器)
  final _urlController = TextEditingController(
    text: 'ws://192.167.167.129:7880',
  );
  final _tokenController = TextEditingController(text: '');

  @override
  void initState() {
    super.initState();
    _initSdk();
  }

  void _initSdk() {
    // 初始化回调系统
    EventHandler.init();
    LogHandler.init();
    PingHandler.init();

    // 监听日志
    LogHandler.logs.listen((entry) {
      _addLog('[Go] ${entry.toString()}');
    });

    setState(() {
      _version = SfuRelay.instance.version;
    });

    _addLog('SDK initialized, version: $_version');
  }

  void _addLog(String message) {
    setState(() {
      _logs.insert(
        0,
        '[${DateTime.now().toString().substring(11, 19)}] $message',
      );
      if (_logs.length > 100) _logs.removeLast();
    });
  }

  Future<void> _connect() async {
    if (_isConnecting) return;

    setState(() => _isConnecting = true);
    _addLog('Connecting to LiveKit...');

    try {
      // 1. 创建并连接 LiveKit Room
      _lkRoom = lk.Room();

      // 创建事件监听器
      _roomListener = _lkRoom!.createListener();
      _setupRoomListeners();

      await _lkRoom!.connect(_urlController.text, _tokenController.text);

      _localParticipant = _lkRoom!.localParticipant;
      _addLog('Connected to LiveKit room: ${_lkRoom!.name}');
      _addLog('Local participant: ${_localParticipant?.identity}');

      // 2. 创建 LiveKit DataChannel 信令适配器
      final signaling = LiveKitDataChannelSignaling(
        room: _lkRoom!,
        localPeerId: _localParticipant!.identity,
      );

      // 3. 创建 AutoCoordinator
      _autoCoord = AutoCoordinator(
        roomId: _lkRoom!.name ?? 'default-room',
        localPeerId: _localParticipant!.identity,
        signaling: signaling,
        config: AutoCoordinatorConfig(
          deviceType: _detectDeviceType(),
          connectionType: ConnectionType.wifi,
          powerState: PowerState.pluggedIn,
          // electionTimeoutMs 使用默认值 1000ms
          autoElection: true,
        ),
      );

      // 4. 监听 AutoCoordinator 事件
      _autoCoord!.onStateChanged.listen((state) {
        setState(() => _relayState = state);
        _addLog('Relay state: ${state.name}');
      });

      _autoCoord!.onRelayChanged.listen((relayId) {
        setState(() {
          _currentRelay = relayId;
          _isRelay = _autoCoord!.isRelay;
        });
        _addLog('🎯 Relay changed: $relayId (isMe: $_isRelay)');
      });

      _autoCoord!.onPeerJoined.listen((peerId) {
        setState(() => _peers.add(peerId));
        _addLog('👤 Peer joined: $peerId');
      });

      _autoCoord!.onPeerLeft.listen((peerId) {
        setState(() => _peers.remove(peerId));
        _addLog('👤 Peer left: $peerId');
      });

      _autoCoord!.onError.listen((error) {
        _addLog('❌ Error: $error');
      });

      // 5. 启动 AutoCoordinator
      await _autoCoord!.start();

      setState(() {
        _localScore = _autoCoord!.localScore;
      });

      _addLog('✅ AutoCoordinator started, score: $_localScore');
    } catch (e) {
      _addLog('❌ Connection failed: $e');
    } finally {
      setState(() => _isConnecting = false);
    }
  }

  void _setupRoomListeners() {
    _roomListener?.on<lk.ParticipantConnectedEvent>((event) {
      _addLog('LiveKit participant connected: ${event.participant.identity}');
    });

    _roomListener?.on<lk.ParticipantDisconnectedEvent>((event) {
      _addLog(
        'LiveKit participant disconnected: ${event.participant.identity}',
      );
      // 断开检测已自动处理，无需手动调用 notifyPeerDisconnected
    });
  }

  Future<void> _disconnect() async {
    _addLog('Disconnecting...');

    await _autoCoord?.stop();
    _autoCoord?.dispose();
    _autoCoord = null;

    _roomListener?.dispose();
    _roomListener = null;

    await _lkRoom?.disconnect();
    _lkRoom = null;
    _localParticipant = null;

    setState(() {
      _relayState = AutoCoordinatorState.idle;
      _isRelay = false;
      _currentRelay = null;
      _peers.clear();
    });

    _addLog('Disconnected');
  }

  void _triggerElection() {
    _autoCoord?.triggerElection();
    _addLog('🗳️ Election triggered manually');
  }

  DeviceType _detectDeviceType() {
    if (Platform.isAndroid || Platform.isIOS) {
      return DeviceType.mobile;
    }
    return DeviceType.pc;
  }

  @override
  void dispose() {
    // 确保在 widget 销毁时清理 native 回调
    // 这对于防止 Hot Restart 时的 Race Condition 至关重要
    SfuRelay.instance.cleanupAll();

    _disconnect();
    _urlController.dispose();
    _tokenController.dispose();
    EventHandler.dispose();
    LogHandler.dispose();
    PingHandler.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Flutter SFU Relay + LiveKit Demo',
      theme: ThemeData.dark().copyWith(
        colorScheme: const ColorScheme.dark(
          primary: Colors.tealAccent,
          secondary: Colors.cyanAccent,
        ),
      ),
      home: Scaffold(
        appBar: AppBar(
          title: const Text('SFU Relay'),
          backgroundColor: Colors.teal.shade800,
          actions: [
            if (_isRelay)
              Container(
                margin: const EdgeInsets.all(8),
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 4,
                ),
                decoration: BoxDecoration(
                  color: Colors.orange,
                  borderRadius: BorderRadius.circular(16),
                ),
                child: const Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.router, size: 16),
                    SizedBox(width: 4),
                    Text(
                      'RELAY',
                      style: TextStyle(fontWeight: FontWeight.bold),
                    ),
                  ],
                ),
              ),
            // 美观版会议 UI
            Builder(
              builder: (navContext) => IconButton(
                icon: const Icon(Icons.meeting_room),
                tooltip: '美观版会议 UI',
                onPressed: () {
                  Navigator.of(navContext).push(
                    MaterialPageRoute(builder: (context) => const HomePage()),
                  );
                },
              ),
            ),
          ],
        ),
        body: Padding(
          padding: const EdgeInsets.all(16.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // SDK 信息卡片
              _buildInfoCard(),
              const SizedBox(height: 16),

              // 连接配置
              _buildConnectionConfig(),
              const SizedBox(height: 16),

              // 操作按钮
              _buildActionButtons(),
              const SizedBox(height: 16),

              // Peer 列表
              _buildPeerList(),
              const SizedBox(height: 16),

              // 日志
              Expanded(child: _buildLogCard()),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildInfoCard() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'SDK: $_version',
                  style: const TextStyle(fontWeight: FontWeight.bold),
                ),
                _buildStateChip(),
              ],
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                Text('Score: ${_localScore.toStringAsFixed(0)}'),
                const SizedBox(width: 16),
                Text('Relay: ${_currentRelay ?? "None"}'),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildStateChip() {
    Color color;
    IconData icon;
    switch (_relayState) {
      case AutoCoordinatorState.idle:
        color = Colors.grey;
        icon = Icons.radio_button_unchecked;
        break;
      case AutoCoordinatorState.connecting:
        color = Colors.blue;
        icon = Icons.sync;
        break;
      case AutoCoordinatorState.electing:
        color = Colors.orange;
        icon = Icons.how_to_vote;
        break;
      case AutoCoordinatorState.connected:
        color = Colors.green;
        icon = Icons.check_circle;
        break;
      case AutoCoordinatorState.asRelay:
        color = Colors.purple;
        icon = Icons.router;
        break;
      case AutoCoordinatorState.error:
        color = Colors.red;
        icon = Icons.error;
        break;
    }
    return Chip(
      avatar: Icon(icon, size: 16, color: Colors.white),
      label: Text(_relayState.name),
      backgroundColor: color,
    );
  }

  Widget _buildConnectionConfig() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12.0),
        child: Column(
          children: [
            TextField(
              controller: _urlController,
              decoration: const InputDecoration(
                labelText: 'LiveKit URL',
                hintText: 'wss://your-server.livekit.cloud',
                isDense: true,
              ),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _tokenController,
              decoration: const InputDecoration(
                labelText: 'Token',
                hintText: 'your-access-token',
                isDense: true,
              ),
              obscureText: true,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildActionButtons() {
    final isConnected = _relayState != AutoCoordinatorState.idle;

    return Row(
      children: [
        Expanded(
          child: ElevatedButton.icon(
            onPressed: isConnected ? null : (_isConnecting ? null : _connect),
            icon: _isConnecting
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.login),
            label: Text(_isConnecting ? 'Connecting...' : 'Connect'),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.teal),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: ElevatedButton.icon(
            onPressed: isConnected ? _disconnect : null,
            icon: const Icon(Icons.logout),
            label: const Text('Disconnect'),
            style: ElevatedButton.styleFrom(
              backgroundColor: Colors.red.shade700,
            ),
          ),
        ),
        const SizedBox(width: 8),
        IconButton(
          onPressed: isConnected ? _triggerElection : null,
          icon: const Icon(Icons.how_to_vote),
          tooltip: 'Trigger Election',
        ),
      ],
    );
  }

  Widget _buildPeerList() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Peers (${_peers.length}):',
          style: const TextStyle(fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 4),
        Wrap(
          spacing: 8,
          children: _peers
              .map(
                (p) => Chip(
                  avatar: Icon(
                    p == _currentRelay ? Icons.router : Icons.person,
                    size: 16,
                  ),
                  label: Text(p),
                  backgroundColor: p == _currentRelay
                      ? Colors.orange.shade800
                      : null,
                ),
              )
              .toList(),
        ),
      ],
    );
  }

  Widget _buildLogCard() {
    return Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.all(8.0),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'Logs:',
                  style: TextStyle(fontWeight: FontWeight.bold),
                ),
                IconButton(
                  icon: const Icon(Icons.delete_outline, size: 18),
                  onPressed: () => setState(() => _logs.clear()),
                  tooltip: 'Clear logs',
                ),
              ],
            ),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: _logs.length,
              itemBuilder: (context, index) => Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 1),
                child: Text(
                  _logs[index],
                  style: TextStyle(
                    fontSize: 11,
                    fontFamily: 'monospace',
                    color: _logs[index].contains('❌')
                        ? Colors.red
                        : _logs[index].contains('✅')
                        ? Colors.green
                        : _logs[index].contains('🎯')
                        ? Colors.orange
                        : null,
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ============================================================
// LiveKit DataChannel Signaling Adapter
// ============================================================

/// 将 LiveKit DataChannel 适配为 SignalingBridge
class LiveKitDataChannelSignaling implements SignalingBridge {
  final lk.Room room;
  final String localPeerId;

  final _messageController = StreamController<SignalingMessage>.broadcast();
  final _peerDisconnectedController = StreamController<String>.broadcast();
  bool _isConnected = false;
  String? _currentRoomId;
  lk.EventsListener<lk.RoomEvent>? _listener;
  bool _disposed = false; // 防止 dispose 后处理事件

  LiveKitDataChannelSignaling({required this.room, required this.localPeerId}) {
    _listener = room.createListener();
    _listener?.on<lk.DataReceivedEvent>((event) {
      if (_disposed) return;
      try {
        _onDataReceived(event.data, event.participant);
      } catch (e) {
        print('[Signaling] DataReceivedEvent error: $e');
      }
    });
    // 监听 Peer 连接事件（比 signaling 消息更快）
    _listener?.on<lk.ParticipantConnectedEvent>((event) {
      if (_disposed) return;
      try {
        _peerConnectedController.add(event.participant.identity);
      } catch (e) {
        print('[Signaling] ParticipantConnectedEvent error: $e');
      }
    });
    // 监听 Peer 断开事件，自动通知 AutoCoordinator
    _listener?.on<lk.ParticipantDisconnectedEvent>((event) {
      if (_disposed) return;
      try {
        _peerDisconnectedController.add(event.participant.identity);
      } catch (e) {
        print('[Signaling] ParticipantDisconnectedEvent error: $e');
      }
    });
  }

  @override
  bool get isConnected => _isConnected;

  @override
  Stream<SignalingMessage> get messages => _messageController.stream;

  @override
  Stream<String> get peerDisconnected => _peerDisconnectedController.stream;

  final _peerConnectedController = StreamController<String>.broadcast();
  @override
  Stream<String> get peerConnected => _peerConnectedController.stream;

  @override
  Future<void> connect() async {
    _isConnected = room.connectionState == lk.ConnectionState.connected;
  }

  @override
  Future<void> disconnect() async {
    _isConnected = false;
  }

  @override
  Future<void> joinRoom(String roomId, String peerId) async {
    _currentRoomId = roomId;
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
  Future<void> sendRelayClaim(
    String roomId,
    int epoch,
    double score,
    String? publicIp,
  ) async {
    await _broadcast({
      'type': 'relayClaim',
      'epoch': epoch,
      'score': score,
      if (publicIp != null) 'publicIp': publicIp,
    });
  }

  @override
  Future<void> sendRelayChanged(
    String roomId,
    String relayId,
    int epoch,
    double score,
    String? publicIp,
  ) async {
    await _broadcast({
      'type': 'relayChanged',
      'relayId': relayId,
      'epoch': epoch,
      'score': score,
      if (publicIp != null) 'publicIp': publicIp,
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
  Future<void> sendAnswer(
    String roomId,
    String targetPeerId,
    String sdp,
  ) async {
    await _broadcast({
      'type': 'answer',
      'targetPeerId': targetPeerId,
      'sdp': sdp,
    });
  }

  @override
  Future<void> sendCandidate(
    String roomId,
    String targetPeerId,
    String candidate,
  ) async {
    await _broadcast({
      'type': 'candidate',
      'targetPeerId': targetPeerId,
      'candidate': candidate,
    });
  }

  @override
  void dispose() {
    // 先设置标志，防止事件处理
    _disposed = true;
    _listener?.dispose();
    _listener = null;
    _messageController.close();
    _peerDisconnectedController.close();
    _peerConnectedController.close();
  }

  @override
  Future<void> sendScreenShare(String roomId, bool isSharing) async {
    await _broadcast({'type': 'screenShare', 'isSharing': isSharing});
  }

  Future<void> _broadcast(Map<String, dynamic> data) async {
    data['peerId'] = localPeerId;
    data['roomId'] = _currentRoomId;

    await room.localParticipant?.publishData(
      utf8.encode(jsonEncode(data)),
      reliable: true,
    );
  }

  void _onDataReceived(List<int> data, lk.Participant? participant) {
    // 忽略来自 LocalParticipant 的数据以及 null 参与者
    if (participant == null || participant is lk.LocalParticipant) return;

    try {
      final json = jsonDecode(utf8.decode(data)) as Map<String, dynamic>;
      final type = json['type'] as String?;

      final message = SignalingMessage(
        type: _parseType(type),
        roomId: json['roomId'] as String? ?? _currentRoomId ?? '',
        peerId: participant.identity,
        targetPeerId: json['targetPeerId'] as String?,
        data: json,
      );

      _messageController.add(message);
    } catch (e) {
      // 忽略解析错误
    }
  }

  SignalingMessageType _parseType(String? type) {
    switch (type) {
      case 'join':
        return SignalingMessageType.join;
      case 'leave':
        return SignalingMessageType.leave;
      case 'ping':
        return SignalingMessageType.ping;
      case 'pong':
        return SignalingMessageType.pong;
      case 'relayClaim':
        return SignalingMessageType.relayClaim;
      case 'relayChanged':
        return SignalingMessageType.relayChanged;
      case 'offer':
        return SignalingMessageType.offer;
      case 'answer':
        return SignalingMessageType.answer;
      case 'candidate':
        return SignalingMessageType.candidate;
      case 'screenShare':
        return SignalingMessageType.screenShare;
      default:
        return SignalingMessageType.error;
    }
  }
}
