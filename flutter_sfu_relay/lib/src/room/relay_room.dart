/// RelayRoom - 高级房间管理器
///
/// 整合 Coordinator、Signaling、Callbacks 提供一站式 API
library;

import 'dart:async';

import '../core/coordinator.dart';
import '../callbacks/callbacks.dart';
import '../signaling/signaling.dart';
import '../enums.dart';

/// 房间状态
enum RoomState {
  /// 未连接
  disconnected,

  /// 正在连接
  connecting,

  /// 已连接
  connected,

  /// 正在分享
  sharing,

  /// 错误
  error,
}

/// Peer 信息
class PeerInfo {
  final String peerId;
  final DeviceType deviceType;
  final ConnectionType connectionType;
  final bool isRelay;
  final DateTime joinedAt;

  PeerInfo({
    required this.peerId,
    this.deviceType = DeviceType.unknown,
    this.connectionType = ConnectionType.unknown,
    this.isRelay = false,
    DateTime? joinedAt,
  }) : joinedAt = joinedAt ?? DateTime.now();
}

/// RelayRoom - 局域网代理房间
///
/// 提供完整的房间管理功能：
/// - 自动 Relay 选举和故障切换
/// - 信令消息处理
/// - 事件通知
///
/// ```dart
/// final room = RelayRoom(
///   roomId: 'room-1',
///   localPeerId: 'peer-1',
///   signaling: WebSocketSignaling(url: 'ws://...', localPeerId: 'peer-1'),
/// );
///
/// await room.join();
/// room.onPeerJoined.listen((peer) => print('Peer joined: ${peer.peerId}'));
/// ```
class RelayRoom {
  final String roomId;
  final String localPeerId;
  final SignalingBridge signaling;

  late final Coordinator _coordinator;
  RoomState _state = RoomState.disconnected;

  final Map<String, PeerInfo> _peers = {};
  StreamSubscription<SignalingMessage>? _signalingSubscription;
  StreamSubscription<SfuEvent>? _eventSubscription;
  StreamSubscription<PingRequest>? _pingSubscription;

  // 事件流控制器
  final _stateController = StreamController<RoomState>.broadcast();
  final _peerJoinedController = StreamController<PeerInfo>.broadcast();
  final _peerLeftController = StreamController<String>.broadcast();
  final _relayChangedController = StreamController<String>.broadcast();
  final _errorController = StreamController<String>.broadcast();

  RelayRoom({
    required this.roomId,
    required this.localPeerId,
    required this.signaling,
  }) {
    _coordinator = Coordinator(roomId: roomId, localPeerId: localPeerId);
  }

  /// 当前状态
  RoomState get state => _state;

  /// 是否是 Relay
  bool get isRelay => _coordinator.isRelay;

  /// 房间内所有 Peer
  List<PeerInfo> get peers => _peers.values.toList();

  // 事件流
  Stream<RoomState> get onStateChanged => _stateController.stream;
  Stream<PeerInfo> get onPeerJoined => _peerJoinedController.stream;
  Stream<String> get onPeerLeft => _peerLeftController.stream;
  Stream<String> get onRelayChanged => _relayChangedController.stream;
  Stream<String> get onError => _errorController.stream;

  /// 加入房间
  Future<void> join({
    DeviceType deviceType = DeviceType.unknown,
    ConnectionType connectionType = ConnectionType.unknown,
    PowerState powerState = PowerState.unknown,
  }) async {
    if (_state != RoomState.disconnected) {
      throw StateError('Room is already connected or connecting');
    }

    _updateState(RoomState.connecting);

    try {
      // 初始化回调系统
      EventHandler.init();
      LogHandler.init();
      PingHandler.init();

      // 启用 Coordinator
      _coordinator.enable();

      // 连接信令
      await signaling.connect();
      await signaling.joinRoom(roomId, localPeerId);

      // 监听信令消息
      _signalingSubscription = signaling.messages.listen(
        _handleSignalingMessage,
      );

      // 监听 Go 事件
      _eventSubscription = EventHandler.events.listen(_handleSfuEvent);

      // 监听 Ping 请求
      _pingSubscription = PingHandler.pingRequests.listen(_handlePingRequest);

      _updateState(RoomState.connected);
    } catch (e) {
      _updateState(RoomState.error);
      _errorController.add(e.toString());
      rethrow;
    }
  }

  /// 离开房间
  Future<void> leave() async {
    if (_state == RoomState.disconnected) return;

    try {
      await signaling.leaveRoom(roomId);
      _coordinator.disable();

      await _signalingSubscription?.cancel();
      await _eventSubscription?.cancel();
      await _pingSubscription?.cancel();

      _peers.clear();
      _updateState(RoomState.disconnected);
    } catch (e) {
      _errorController.add(e.toString());
    }
  }

  /// 开始本地分享
  Future<void> startSharing() async {
    if (_state != RoomState.connected) {
      throw StateError('Must be connected to start sharing');
    }

    _coordinator.startLocalShare(localPeerId);
    _updateState(RoomState.sharing);
  }

  /// 停止本地分享
  Future<void> stopSharing() async {
    if (_state != RoomState.sharing) return;

    _coordinator.stopLocalShare();
    _updateState(RoomState.connected);
  }

  /// 获取综合状态
  Map<String, dynamic> getStatus() => _coordinator.getStatus();

  /// 释放资源
  void dispose() {
    _signalingSubscription?.cancel();
    _eventSubscription?.cancel();
    _pingSubscription?.cancel();
    signaling.dispose();
    _stateController.close();
    _peerJoinedController.close();
    _peerLeftController.close();
    _relayChangedController.close();
    _errorController.close();
  }

  void _updateState(RoomState newState) {
    _state = newState;
    _stateController.add(newState);
  }

  void _handleSignalingMessage(SignalingMessage message) {
    // 忽略自己发送的消息
    if (message.peerId == localPeerId) return;

    switch (message.type) {
      case SignalingMessageType.join:
        _handlePeerJoined(message);
        break;
      case SignalingMessageType.leave:
        _handlePeerLeft(message);
        break;
      case SignalingMessageType.pong:
        _coordinator.handlePong(message.peerId);
        break;
      case SignalingMessageType.relayClaim:
        final epoch = message.data?['epoch'] as int? ?? 0;
        final score = (message.data?['score'] as num?)?.toDouble() ?? 0.0;
        _coordinator.receiveClaim(message.peerId, epoch, score);
        break;
      case SignalingMessageType.relayChanged:
        final relayId = message.data?['relayId'] as String? ?? '';
        final epoch = message.data?['epoch'] as int? ?? 0;
        _coordinator.setRelay(relayId, epoch);
        _relayChangedController.add(relayId);
        break;
      default:
        // 其他消息类型（offer/answer/candidate）由 WebRTC 层处理
        break;
    }
  }

  void _handlePeerJoined(SignalingMessage message) {
    final deviceType = DeviceType.values.firstWhere(
      (e) => e.value == (message.data?['deviceType'] as int? ?? 0),
      orElse: () => DeviceType.unknown,
    );
    final connectionType = ConnectionType.values.firstWhere(
      (e) => e.value == (message.data?['connectionType'] as int? ?? 0),
      orElse: () => ConnectionType.unknown,
    );
    final powerState = PowerState.values.firstWhere(
      (e) => e.value == (message.data?['powerState'] as int? ?? 0),
      orElse: () => PowerState.unknown,
    );

    final peer = PeerInfo(
      peerId: message.peerId,
      deviceType: deviceType,
      connectionType: connectionType,
    );

    _peers[message.peerId] = peer;
    _coordinator.addPeer(
      message.peerId,
      deviceType: deviceType,
      connectionType: connectionType,
      powerState: powerState,
    );
    _peerJoinedController.add(peer);
  }

  void _handlePeerLeft(SignalingMessage message) {
    _peers.remove(message.peerId);
    _coordinator.removePeer(message.peerId);
    _peerLeftController.add(message.peerId);
  }

  void _handleSfuEvent(SfuEvent event) {
    // 仅处理当前房间的事件
    if (event.roomId != roomId) return;

    switch (event.type) {
      case SfuEventType.relayChanged:
        _relayChangedController.add(event.data ?? event.peerId);
        break;
      case SfuEventType.error:
        _errorController.add(event.data ?? 'Unknown error');
        break;
      default:
        break;
    }
  }

  void _handlePingRequest(PingRequest request) {
    // 通过信令发送 Ping
    signaling.sendPing(roomId, request.peerId);
  }
}
