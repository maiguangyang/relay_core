/// AutoCoordinator - 真正的一键自动代理
///
/// 完整整合 Coordinator + Signaling + Callbacks，实现真正的自动切换
library;

import 'dart:async';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../callbacks/callbacks.dart';
import '../enums.dart';
import '../signaling/signaling.dart';
import 'coordinator.dart';

import 'package:ffi/ffi.dart';

/// 自动协调器配置
class AutoCoordinatorConfig {
  /// 设备类型
  final DeviceType deviceType;

  /// 连接类型
  final ConnectionType connectionType;

  /// 电源状态
  final PowerState powerState;

  /// 选举超时（毫秒）
  final int electionTimeoutMs;

  /// 是否自动触发选举
  final bool autoElection;

  const AutoCoordinatorConfig({
    this.deviceType = DeviceType.unknown,
    this.connectionType = ConnectionType.unknown,
    this.powerState = PowerState.unknown,
    this.electionTimeoutMs = 3000,
    this.autoElection = true,
  });
}

/// 自动协调器状态
enum AutoCoordinatorState {
  /// 未启动
  idle,

  /// 正在连接
  connecting,

  /// 选举中
  electing,

  /// 已连接（作为观察者）
  connected,

  /// 作为 Relay
  asRelay,

  /// 错误
  error,
}

/// AutoCoordinator - 真正的一键自动代理
///
/// 完整整合所有组件，只需调用 `start()` 即可自动：
/// - 连接信令
/// - 初始化回调
/// - 参与选举
/// - 处理 Ping/Pong
/// - 自动故障切换
///
/// ```dart
/// final autoCoord = AutoCoordinator(
///   roomId: 'room-1',
///   localPeerId: 'my-peer',
///   signaling: WebSocketSignaling(url: 'ws://...', localPeerId: 'my-peer'),
///   config: AutoCoordinatorConfig(
///     deviceType: DeviceType.pc,
///     connectionType: ConnectionType.wifi,
///     powerState: PowerState.pluggedIn,
///   ),
/// );
///
/// await autoCoord.start();
///
/// // 监听状态
/// autoCoord.onStateChanged.listen((state) {
///   print('状态: $state');
/// });
///
/// // 监听 Relay 变更
/// autoCoord.onRelayChanged.listen((relayId) {
///   print('新 Relay: $relayId');
/// });
///
/// // 注入 RTP（如果是 Relay）
/// if (autoCoord.isRelay) {
///   autoCoord.injectSfuPacket(true, rtpData);
/// }
///
/// // 停止
/// await autoCoord.stop();
/// ```
class AutoCoordinator {
  final String roomId;
  final String localPeerId;
  final SignalingBridge signaling;
  final AutoCoordinatorConfig config;

  late final Coordinator _coordinator;

  AutoCoordinatorState _state = AutoCoordinatorState.idle;
  String? _currentRelay;
  int _currentEpoch = 0;
  double _localScore = 0;
  Timer? _electionTimer;

  final Set<String> _peers = {};

  // 订阅
  StreamSubscription<SignalingMessage>? _signalingSubscription;
  StreamSubscription<SfuEvent>? _eventSubscription;
  StreamSubscription<PingRequest>? _pingSubscription;

  // 事件流
  final _stateController = StreamController<AutoCoordinatorState>.broadcast();
  final _relayChangedController = StreamController<String>.broadcast();
  final _peerJoinedController = StreamController<String>.broadcast();
  final _peerLeftController = StreamController<String>.broadcast();
  final _errorController = StreamController<String>.broadcast();

  AutoCoordinator({
    required this.roomId,
    required this.localPeerId,
    required this.signaling,
    this.config = const AutoCoordinatorConfig(),
  }) {
    _coordinator = Coordinator(roomId: roomId, localPeerId: localPeerId);
    _calculateLocalScore();
  }

  // ========== 公开属性 ==========

  /// 当前状态
  AutoCoordinatorState get state => _state;

  /// 是否是 Relay
  bool get isRelay => _coordinator.isRelay;

  /// 当前 Relay ID
  String? get currentRelay => _currentRelay;

  /// 当前 Epoch
  int get currentEpoch => _currentEpoch;

  /// 本机分数
  double get localScore => _localScore;

  /// 房间内所有 Peer
  Set<String> get peers => Set.unmodifiable(_peers);

  // ========== 事件流 ==========

  Stream<AutoCoordinatorState> get onStateChanged => _stateController.stream;
  Stream<String> get onRelayChanged => _relayChangedController.stream;
  Stream<String> get onPeerJoined => _peerJoinedController.stream;
  Stream<String> get onPeerLeft => _peerLeftController.stream;
  Stream<String> get onError => _errorController.stream;

  // ========== 生命周期 ==========

  /// 启动自动协调器
  Future<void> start() async {
    if (_state != AutoCoordinatorState.idle) {
      throw StateError('AutoCoordinator already started');
    }

    _updateState(AutoCoordinatorState.connecting);

    try {
      // 1. 初始化回调系统
      EventHandler.init();
      LogHandler.init();
      PingHandler.init();

      // 2. 启用 Coordinator
      _coordinator.enable();

      // 3. 更新本机设备信息
      _updateLocalDeviceInfo();

      // 4. 连接信令
      await signaling.connect();
      await signaling.joinRoom(roomId, localPeerId);

      // 5. 设置所有监听
      _setupListeners();

      // 6. 开始选举
      _updateState(AutoCoordinatorState.electing);

      if (config.autoElection) {
        _startElection();
      }
    } catch (e) {
      _updateState(AutoCoordinatorState.error);
      _errorController.add(e.toString());
      rethrow;
    }
  }

  /// 停止自动协调器
  Future<void> stop() async {
    _electionTimer?.cancel();

    await _signalingSubscription?.cancel();
    await _eventSubscription?.cancel();
    await _pingSubscription?.cancel();

    _coordinator.disable();

    try {
      await signaling.leaveRoom(roomId);
    } catch (_) {}

    _peers.clear();
    _currentRelay = null;
    _updateState(AutoCoordinatorState.idle);
  }

  /// 释放资源
  void dispose() {
    stop();
    signaling.dispose();
    _stateController.close();
    _relayChangedController.close();
    _peerJoinedController.close();
    _peerLeftController.close();
    _errorController.close();
  }

  // ========== 公开方法 ==========

  /// 手动触发选举
  void triggerElection() {
    _currentEpoch++;
    _updateState(AutoCoordinatorState.electing);
    _broadcastClaim();
  }

  /// 注入 SFU RTP 包
  bool injectSfuPacket(bool isVideo, List<int> data) {
    return _coordinator.injectSfuPacket(isVideo, data as dynamic);
  }

  /// 注入本地 RTP 包
  bool injectLocalPacket(bool isVideo, List<int> data) {
    return _coordinator.injectLocalPacket(isVideo, data as dynamic);
  }

  /// 开始本地分享
  bool startLocalShare() {
    return _coordinator.startLocalShare(localPeerId);
  }

  /// 停止本地分享
  bool stopLocalShare() {
    return _coordinator.stopLocalShare();
  }

  /// 获取综合状态
  Map<String, dynamic> getStatus() {
    final coordStatus = _coordinator.getStatus();
    return {
      'state': _state.name,
      'isRelay': isRelay,
      'currentRelay': _currentRelay,
      'currentEpoch': _currentEpoch,
      'localScore': _localScore,
      'peerCount': _peers.length,
      ...coordStatus,
    };
  }

  // ========== 内部方法 ==========

  void _updateState(AutoCoordinatorState newState) {
    if (_state == newState) return;
    _state = newState;
    _stateController.add(newState);
  }

  void _calculateLocalScore() {
    _localScore = 50.0;

    switch (config.deviceType) {
      case DeviceType.pc:
        _localScore += 30;
        break;
      case DeviceType.pad:
        _localScore += 20;
        break;
      case DeviceType.tv:
        _localScore += 15;
        break;
      case DeviceType.mobile:
        _localScore += 10;
        break;
      default:
        break;
    }

    switch (config.connectionType) {
      case ConnectionType.ethernet:
        _localScore += 30;
        break;
      case ConnectionType.wifi:
        _localScore += 20;
        break;
      case ConnectionType.cellular:
        _localScore += 5;
        break;
      default:
        break;
    }

    switch (config.powerState) {
      case PowerState.pluggedIn:
        _localScore += 20;
        break;
      case PowerState.battery:
        _localScore += 10;
        break;
      case PowerState.lowBattery:
        _localScore -= 20;
        break;
      default:
        break;
    }
  }

  void _updateLocalDeviceInfo() {
    final roomPtr = toCString(roomId);
    bindings.CoordinatorUpdateLocalDevice(
      roomPtr,
      config.deviceType.value,
      config.connectionType.value,
      config.powerState.value,
    );
    calloc.free(roomPtr);
  }

  void _setupListeners() {
    // 监听信令消息
    _signalingSubscription = signaling.messages.listen(_handleSignalingMessage);

    // 监听 Go 层事件
    _eventSubscription = EventHandler.events
        .where((e) => e.roomId == roomId)
        .listen(_handleSfuEvent);

    // 监听 Ping 请求
    _pingSubscription = PingHandler.pingRequests.listen(_handlePingRequest);
  }

  void _handleSignalingMessage(SignalingMessage message) {
    if (message.peerId == localPeerId) return;

    switch (message.type) {
      case SignalingMessageType.join:
        _handlePeerJoined(message.peerId, message.data);
        break;

      case SignalingMessageType.leave:
        _handlePeerLeft(message.peerId);
        break;

      case SignalingMessageType.pong:
        _coordinator.handlePong(message.peerId);
        break;

      case SignalingMessageType.relayClaim:
        _handleRelayClaim(message.peerId, message.data);
        break;

      case SignalingMessageType.relayChanged:
        _handleRelayChanged(message.data);
        break;

      default:
        break;
    }
  }

  void _handlePeerJoined(String peerId, Map<String, dynamic>? data) {
    _peers.add(peerId);

    final deviceType = DeviceType.values.firstWhere(
      (e) => e.value == (data?['deviceType'] ?? 0),
      orElse: () => DeviceType.unknown,
    );
    final connectionType = ConnectionType.values.firstWhere(
      (e) => e.value == (data?['connectionType'] ?? 0),
      orElse: () => ConnectionType.unknown,
    );
    final powerState = PowerState.values.firstWhere(
      (e) => e.value == (data?['powerState'] ?? 0),
      orElse: () => PowerState.unknown,
    );

    _coordinator.addPeer(
      peerId,
      deviceType: deviceType,
      connectionType: connectionType,
      powerState: powerState,
    );

    _peerJoinedController.add(peerId);

    // 新 Peer 加入，发送我们的 claim
    if (config.autoElection && _currentRelay == null) {
      _broadcastClaim();
    }
  }

  void _handlePeerLeft(String peerId) {
    _peers.remove(peerId);
    _coordinator.removePeer(peerId);
    _peerLeftController.add(peerId);

    // 如果是 Relay 离开，触发重新选举
    if (peerId == _currentRelay) {
      _currentRelay = null;
      triggerElection();
    }
  }

  void _handleRelayClaim(String peerId, Map<String, dynamic>? data) {
    final epoch = data?['epoch'] as int? ?? 0;
    final score = (data?['score'] as num?)?.toDouble() ?? 0.0;

    // 忽略过期的 epoch
    if (epoch < _currentEpoch) return;

    // 更新 epoch
    if (epoch > _currentEpoch) {
      _currentEpoch = epoch;
    }

    // 告知 Go 层
    _coordinator.receiveClaim(peerId, epoch, score);

    // 冲突解决：比较分数
    _resolveElection(peerId, epoch, score);
  }

  void _resolveElection(String claimerId, int epoch, double claimerScore) {
    // 对方分数更高
    if (claimerScore > _localScore) {
      _acceptRelay(claimerId, epoch);
      return;
    }

    // 分数相同，比较 PeerId
    if (claimerScore == _localScore && claimerId.compareTo(localPeerId) > 0) {
      _acceptRelay(claimerId, epoch);
      return;
    }

    // 我们分数更高，保持或成为 Relay
    if (_currentRelay == null) {
      _becomeRelay();
    }
  }

  void _acceptRelay(String relayId, int epoch) {
    _currentRelay = relayId;
    _currentEpoch = epoch;
    _coordinator.setRelay(relayId, epoch);

    _electionTimer?.cancel();

    if (_state == AutoCoordinatorState.electing) {
      _updateState(AutoCoordinatorState.connected);
    }

    _relayChangedController.add(relayId);
  }

  void _becomeRelay() {
    _currentRelay = localPeerId;
    _coordinator.setRelay(localPeerId, _currentEpoch);

    _electionTimer?.cancel();
    _updateState(AutoCoordinatorState.asRelay);

    // 广播我们成为 Relay
    signaling.sendRelayChanged(roomId, localPeerId, _currentEpoch);

    _relayChangedController.add(localPeerId);
  }

  void _handleRelayChanged(Map<String, dynamic>? data) {
    final relayId = data?['relayId'] as String? ?? '';
    final epoch = data?['epoch'] as int? ?? 0;

    if (epoch >= _currentEpoch && relayId.isNotEmpty) {
      _acceptRelay(relayId, epoch);
    }
  }

  void _handleSfuEvent(SfuEvent event) {
    switch (event.type) {
      case SfuEventType.relayChanged:
        // Peer 离线检测：如果收到 relayChanged 且 data 表示离线
        if (event.data?.contains('offline') == true &&
            event.peerId == _currentRelay) {
          _currentRelay = null;
          triggerElection();
        }
        break;

      case SfuEventType.error:
        _errorController.add(event.data ?? 'Unknown error');
        break;

      default:
        break;
    }
  }

  void _handlePingRequest(PingRequest request) {
    // Go 层需要发送 Ping，通过信令发送
    signaling.sendPing(roomId, request.peerId);
  }

  void _startElection() {
    _broadcastClaim();

    // 设置选举超时
    _electionTimer = Timer(
      Duration(milliseconds: config.electionTimeoutMs),
      () {
        // 超时后，如果没有确定 Relay，自己成为 Relay
        if (_currentRelay == null) {
          _becomeRelay();
        }
      },
    );
  }

  void _broadcastClaim() {
    signaling.sendRelayClaim(roomId, _currentEpoch, _localScore);
  }
}
