import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';
import 'package:livekit_client/livekit_client.dart' as lk;
import 'package:permission_handler/permission_handler.dart';

import '../theme/app_theme.dart';
import '../widgets/control_bar.dart';
import '../widgets/participant_tile.dart';

/// 首页 - 独立完整的会议页面
/// 包含连接表单、LiveKit集成、屏幕共享、语音会议功能
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  // 连接配置
  final _urlController = TextEditingController(
    text: 'ws://192.167.167.129:7880',
  );
  final _tokenController = TextEditingController();

  // 页面状态
  bool _isInMeeting = false;
  bool _isConnecting = false;
  String? _errorMessage;

  // LiveKit
  lk.Room? _room;
  lk.LocalParticipant? _localParticipant;
  lk.EventsListener<lk.RoomEvent>? _roomListener;

  // SFU Relay
  AutoCoordinator? _autoCoord;
  AutoCoordinatorState _relayState = AutoCoordinatorState.idle;
  String? _currentRelay;
  double _localScore = 0;

  // 参与者列表
  final List<lk.Participant> _participants = [];

  // 控制状态
  ControlState _controlState = const ControlState();

  @override
  void initState() {
    super.initState();
    _initSdk();
  }

  void _initSdk() {
    EventHandler.init();
    LogHandler.init();
    PingHandler.init();
  }

  @override
  void dispose() {
    _disconnect();
    _urlController.dispose();
    _tokenController.dispose();
    EventHandler.dispose();
    LogHandler.dispose();
    PingHandler.dispose();
    super.dispose();
  }

  // ============================================================
  // 权限请求
  // ============================================================

  Future<bool> _requestPermissions() async {
    // Web 和桌面平台：LiveKit 会自动处理权限弹窗
    if (kIsWeb || Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
      return true;
    }

    // Android/iOS: 尝试使用 permission_handler，失败则继续
    try {
      final statuses = await [
        Permission.microphone,
        Permission.camera,
      ].request();
      final micGranted =
          statuses[Permission.microphone] == PermissionStatus.granted;
      if (!micGranted && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('需要麦克风权限才能加入会议')));
      }
      // 即使权限被拒绝也继续，让 LiveKit 尝试
      return true;
    } catch (e) {
      debugPrint('Permission request error: $e');
      return true; // 继续连接，让 LiveKit 处理
    }
  }

  // ============================================================
  // 连接逻辑
  // ============================================================

  Future<void> _connect() async {
    if (_tokenController.text.isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('请输入 Token')));
      return;
    }

    setState(() {
      _isConnecting = true;
      _errorMessage = null;
    });

    try {
      // 1. 创建并连接 LiveKit 房间
      _room = lk.Room(
        roomOptions: const lk.RoomOptions(
          adaptiveStream: true,
          dynacast: true,
          defaultAudioOutputOptions: lk.AudioOutputOptions(speakerOn: true),
        ),
      );
      _roomListener = _room!.createListener();

      await _room!.connect(_urlController.text, _tokenController.text);
      _localParticipant = _room!.localParticipant;

      // 2. 设置 LiveKit 事件监听
      _setupRoomListeners();

      // 3. 创建信令和 AutoCoordinator
      final signaling = _LiveKitSignaling(
        room: _room!,
        localPeerId: _localParticipant!.identity,
      );

      _autoCoord = AutoCoordinator(
        roomId: _room!.name ?? 'room',
        localPeerId: _localParticipant!.identity,
        signaling: signaling,
        config: AutoCoordinatorConfig(
          deviceType: _detectDeviceType(),
          connectionType: ConnectionType.wifi,
          powerState: PowerState.pluggedIn,
          autoElection: true,
        ),
      );

      // 4. 监听 Relay 状态
      _autoCoord!.onStateChanged.listen((state) {
        if (mounted) setState(() => _relayState = state);
      });

      _autoCoord!.onRelayChanged.listen((relayId) {
        if (mounted) setState(() => _currentRelay = relayId);
      });

      // 5. 启动
      await _autoCoord!.start();

      setState(() {
        _isConnecting = false;
        _isInMeeting = true;
        _localScore = _autoCoord!.localScore;
        _updateParticipants();
      });

      // 默认静音，用户可点击麦克风按钮开启
    } catch (e) {
      setState(() {
        _isConnecting = false;
        _errorMessage = e.toString();
      });
    }
  }

  void _setupRoomListeners() {
    _roomListener!
      ..on<lk.ParticipantConnectedEvent>((_) => _updateParticipants())
      ..on<lk.ParticipantDisconnectedEvent>((_) => _updateParticipants())
      ..on<lk.TrackPublishedEvent>((_) => _updateParticipants())
      ..on<lk.TrackUnpublishedEvent>((_) => _updateParticipants())
      ..on<lk.TrackSubscribedEvent>((_) => _updateParticipants())
      ..on<lk.TrackMutedEvent>((_) => _updateParticipants())
      ..on<lk.TrackUnmutedEvent>((_) => _updateParticipants());
  }

  void _updateParticipants() {
    if (!mounted || _room == null) return;

    setState(() {
      _participants.clear();
      if (_localParticipant != null) {
        _participants.add(_localParticipant!);
      }
      _participants.addAll(_room!.remoteParticipants.values);
    });
  }

  DeviceType _detectDeviceType() {
    if (kIsWeb) return DeviceType.pc;
    if (Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
      return DeviceType.pc;
    }
    if (Platform.isIOS || Platform.isAndroid) {
      return DeviceType.mobile;
    }
    return DeviceType.unknown;
  }

  Future<void> _disconnect() async {
    await _autoCoord?.stop();
    _autoCoord?.dispose();
    _autoCoord = null;

    _roomListener?.dispose();
    _roomListener = null;

    await _room?.disconnect();
    _room = null;
    _localParticipant = null;

    if (mounted) {
      setState(() {
        _isInMeeting = false;
        _relayState = AutoCoordinatorState.idle;
        _currentRelay = null;
        _participants.clear();
        _controlState = const ControlState();
      });
    }
  }

  // ============================================================
  // 媒体控制
  // ============================================================

  Future<void> _toggleMic() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.micEnabled;

    // 开启麦克风时请求权限
    if (newState && !await _requestPermissions()) {
      return;
    }

    try {
      await _localParticipant!.setMicrophoneEnabled(newState);
      setState(() {
        _controlState = _controlState.copyWith(micEnabled: newState);
      });
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('无法启用麦克风: $e')));
    }
  }

  Future<void> _toggleCamera() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.cameraEnabled;
    try {
      await _localParticipant!.setCameraEnabled(newState);
      setState(() {
        _controlState = _controlState.copyWith(cameraEnabled: newState);
      });
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('无法启用摄像头: $e')));
    }
  }

  Future<void> _toggleScreenShare() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.screenShareEnabled;

    try {
      if (newState) {
        // 开启屏幕共享
        if (!kIsWeb && (Platform.isMacOS || Platform.isWindows)) {
          // 桌面平台：使用窗口选择器
          final source = await showDialog<dynamic>(
            context: context,
            builder: (context) => lk.ScreenSelectDialog(),
          );

          if (source == null) return;

          final track = await lk.LocalVideoTrack.createScreenShareTrack(
            lk.ScreenShareCaptureOptions(
              sourceId: source.id as String,
              maxFrameRate: 15.0,
            ),
          );
          await _localParticipant!.publishVideoTrack(track);
        } else if (!kIsWeb && Platform.isIOS) {
          // iOS: 需要 Broadcast Extension，模拟器不支持
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('iOS 屏幕共享需要在真机上运行，且需配置 Broadcast Extension'),
              duration: Duration(seconds: 3),
            ),
          );
          return;
        } else {
          // Android 和其他平台
          await _localParticipant!.setScreenShareEnabled(true);
        }
      } else {
        await _localParticipant!.setScreenShareEnabled(false);
      }

      setState(() {
        _controlState = _controlState.copyWith(screenShareEnabled: newState);
      });
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('屏幕共享失败: $e')));
    }
  }

  void _leave() {
    _disconnect();
  }

  // ============================================================
  // UI 构建
  // ============================================================

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: AppTheme.darkTheme,
      home: Scaffold(
        body: Container(
          decoration: AppTheme.gradientBackground,
          child: SafeArea(
            bottom: false,
            child: _isInMeeting ? _buildMeetingView() : _buildLoginView(),
          ),
        ),
      ),
    );
  }

  // -------------------- 登录视图 --------------------

  Widget _buildLoginView() {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Spacer(flex: 2),
          _buildHeader(),
          const SizedBox(height: 48),
          _buildConnectionForm(),
          if (_errorMessage != null) ...[
            const SizedBox(height: 16),
            Text(
              _errorMessage!,
              style: const TextStyle(color: AppTheme.accentColor),
              textAlign: TextAlign.center,
            ),
          ],
          const Spacer(flex: 3),
          _buildFooter(),
        ],
      ),
    );
  }

  Widget _buildHeader() {
    return Column(
      children: [
        Container(
          width: 100,
          height: 100,
          decoration: BoxDecoration(
            gradient: const LinearGradient(
              colors: [AppTheme.primaryColor, AppTheme.secondaryColor],
            ),
            borderRadius: BorderRadius.circular(24),
            boxShadow: [
              BoxShadow(
                color: AppTheme.primaryColor.withOpacity(0.4),
                blurRadius: 30,
                offset: const Offset(0, 10),
              ),
            ],
          ),
          child: const Icon(Icons.groups, size: 48, color: Colors.white),
        ),
        const SizedBox(height: 24),
        const Text(
          'SFU Relay',
          style: TextStyle(
            fontSize: 32,
            fontWeight: FontWeight.bold,
            color: AppTheme.textPrimary,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          '局域网视频会议',
          style: TextStyle(fontSize: 16, color: AppTheme.textSecondary),
        ),
      ],
    );
  }

  Widget _buildConnectionForm() {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: AppTheme.glassDecoration,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextField(
            controller: _urlController,
            style: const TextStyle(color: AppTheme.textPrimary),
            decoration: const InputDecoration(
              labelText: 'LiveKit 服务器',
              hintText: 'ws://your-server:7880',
              prefixIcon: Icon(Icons.dns, color: AppTheme.textSecondary),
            ),
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _tokenController,
            style: const TextStyle(color: AppTheme.textPrimary),
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Token',
              hintText: '输入访问令牌',
              prefixIcon: Icon(Icons.vpn_key, color: AppTheme.textSecondary),
            ),
          ),
          const SizedBox(height: 24),
          ElevatedButton(
            onPressed: _isConnecting ? null : _connect,
            style: ElevatedButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
            child: _isConnecting
                ? const SizedBox(
                    width: 24,
                    height: 24,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  )
                : const Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(Icons.login),
                      SizedBox(width: 8),
                      Text('加入会议'),
                    ],
                  ),
          ),
        ],
      ),
    );
  }

  Widget _buildFooter() {
    return Column(
      children: [
        Text(
          '提示: 使用 lk token create 生成 Token',
          style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.flash_on, size: 14, color: AppTheme.secondaryColor),
            const SizedBox(width: 4),
            Text(
              '局域网 ~2.5s 故障切换',
              style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
            ),
          ],
        ),
      ],
    );
  }

  // -------------------- 会议视图 --------------------

  Widget _buildMeetingView() {
    return Column(
      children: [
        _buildStatusBar(),
        Expanded(child: _buildParticipantGrid()),
        ControlBar(
          state: _controlState.copyWith(isRelay: _autoCoord?.isRelay ?? false),
          onMicToggle: _toggleMic,
          onCameraToggle: _toggleCamera,
          onScreenShareToggle: _toggleScreenShare,
          onLeave: _leave,
        ),
      ],
    );
  }

  Widget _buildStatusBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: _getStateColor().withOpacity(0.2),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(color: _getStateColor()),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(_getStateIcon(), size: 16, color: _getStateColor()),
                const SizedBox(width: 6),
                Text(
                  _getStateText(),
                  style: TextStyle(color: _getStateColor(), fontSize: 12),
                ),
              ],
            ),
          ),
          const Spacer(),
          Text(
            '分数: ${_localScore.toInt()}',
            style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
          ),
          const SizedBox(width: 16),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: AppTheme.cardDark,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(
                  Icons.people,
                  size: 16,
                  color: AppTheme.textSecondary,
                ),
                const SizedBox(width: 6),
                Text(
                  '${_participants.length}',
                  style: TextStyle(color: AppTheme.textPrimary, fontSize: 12),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildParticipantGrid() {
    if (_participants.isEmpty) {
      return const Center(
        child: Text(
          '等待其他参与者加入...',
          style: TextStyle(color: AppTheme.textSecondary),
        ),
      );
    }

    return Padding(
      padding: const EdgeInsets.all(8),
      child: GridView.builder(
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: _getGridColumns(),
          crossAxisSpacing: 8,
          mainAxisSpacing: 8,
          childAspectRatio: 4 / 3,
        ),
        itemCount: _participants.length,
        itemBuilder: (context, index) {
          final participant = _participants[index];
          final isLocal = participant.identity == _localParticipant?.identity;
          final isRelay = participant.identity == _currentRelay;

          return ParticipantTile(
            participant: participant,
            isLocal: isLocal,
            isRelay: isRelay,
          );
        },
      ),
    );
  }

  int _getGridColumns() {
    final count = _participants.length;
    if (count <= 1) return 1;
    if (count <= 4) return 2;
    return 3;
  }

  Color _getStateColor() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return Colors.orange;
      case AutoCoordinatorState.asRelay:
        return AppTheme.relayColor;
      case AutoCoordinatorState.connected:
        return AppTheme.onlineColor;
      default:
        return AppTheme.textSecondary;
    }
  }

  IconData _getStateIcon() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return Icons.how_to_vote;
      case AutoCoordinatorState.asRelay:
        return Icons.router;
      case AutoCoordinatorState.connected:
        return Icons.check_circle;
      default:
        return Icons.radio_button_unchecked;
    }
  }

  String _getStateText() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return '选举中';
      case AutoCoordinatorState.asRelay:
        return 'Relay';
      case AutoCoordinatorState.connected:
        return '已连接';
      default:
        return '离线';
    }
  }
}

// ============================================================
// LiveKit 信令实现
// ============================================================

class _LiveKitSignaling implements SignalingBridge {
  final lk.Room room;
  final String localPeerId;

  final _messageController = StreamController<SignalingMessage>.broadcast();
  final _peerDisconnectedController = StreamController<String>.broadcast();
  final _peerConnectedController = StreamController<String>.broadcast();

  lk.EventsListener<lk.RoomEvent>? _listener;
  String? _currentRoomId;
  bool _isConnected = false;

  _LiveKitSignaling({required this.room, required this.localPeerId}) {
    _listener = room.createListener();
    _listener?.on<lk.DataReceivedEvent>((event) {
      _onDataReceived(event.data, event.participant);
    });
    _listener?.on<lk.ParticipantConnectedEvent>((event) {
      _peerConnectedController.add(event.participant.identity);
    });
    _listener?.on<lk.ParticipantDisconnectedEvent>((event) {
      _peerDisconnectedController.add(event.participant.identity);
    });
  }

  @override
  Stream<SignalingMessage> get messages => _messageController.stream;

  @override
  Stream<String> get peerDisconnected => _peerDisconnectedController.stream;

  @override
  Stream<String> get peerConnected => _peerConnectedController.stream;

  @override
  Future<void> connect() async {
    _isConnected = room.connectionState == lk.ConnectionState.connected;
  }

  @override
  bool get isConnected =>
      _isConnected || room.connectionState == lk.ConnectionState.connected;

  @override
  Future<void> disconnect() async {
    _listener?.dispose();
  }

  @override
  void dispose() {
    _listener?.dispose();
    _messageController.close();
    _peerDisconnectedController.close();
    _peerConnectedController.close();
  }

  @override
  Future<void> joinRoom(String roomId, String peerId) async {
    _currentRoomId = roomId;
    await _broadcast({'type': 'join'});
  }

  @override
  Future<void> leaveRoom(String roomId) async {
    await _broadcast({'type': 'leave'});
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
    await _broadcast({'type': 'relayClaim', 'epoch': epoch, 'score': score});
  }

  @override
  Future<void> sendRelayChanged(
    String roomId,
    String relayId,
    int epoch,
    double score,
  ) async {
    await _broadcast({
      'type': 'relayChanged',
      'relayId': relayId,
      'epoch': epoch,
      'score': score,
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

  Future<void> _broadcast(Map<String, dynamic> data) async {
    if (!_isConnected && room.connectionState != lk.ConnectionState.connected) {
      return;
    }

    data['peerId'] = localPeerId;
    data['roomId'] = _currentRoomId;

    await room.localParticipant?.publishData(
      utf8.encode(jsonEncode(data)),
      reliable: true,
    );
  }

  void _onDataReceived(List<int> data, lk.RemoteParticipant? participant) {
    if (participant == null) return;

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
    } catch (_) {
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
      default:
        return SignalingMessageType.error;
    }
  }
}
