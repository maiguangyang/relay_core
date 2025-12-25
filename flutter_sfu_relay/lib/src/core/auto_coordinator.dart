/// AutoCoordinator - 真正的一键自动代理
///
/// 完整整合 Coordinator + Signaling + Callbacks，实现真正的自动切换
library;

import 'dart:async';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../callbacks/callbacks.dart';
import '../enums.dart';
import '../monitoring/keepalive.dart';
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
    this.electionTimeoutMs = 1000, // 1秒选举超时
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
  double _currentRelayScore = 0; // 当前 Relay 的分数
  Timer? _electionTimer;
  Keepalive? _keepalive; // 心跳管理器

  final Set<String> _peers = {};

  // 订阅
  StreamSubscription<SignalingMessage>? _signalingSubscription;
  StreamSubscription<SfuEvent>? _eventSubscription;
  StreamSubscription<PingRequest>? _pingSubscription;
  StreamSubscription<String>? _peerDisconnectedSubscription;
  StreamSubscription<String>? _peerConnectedSubscription;

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

      // 3. 初始化心跳检测 (1s 间隔, 3s 超时)
      _keepalive = Keepalive(roomId: roomId);
      _keepalive!.create(intervalMs: 1000, timeoutMs: 1500); // 1秒心跳, 1.5秒超时
      _keepalive!.start();

      // 4. 更新本机设备信息
      _updateLocalDeviceInfo();

      // 5. 连接信令
      await signaling.connect();
      await signaling.joinRoom(roomId, localPeerId);

      // 6. 设置所有监听
      _setupListeners();

      // 7. 开始选举
      _updateState(AutoCoordinatorState.electing);

      if (config.autoElection) {
        _startElection(isInitial: true); // 初始选举使用更长超时
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
    await _peerDisconnectedSubscription?.cancel();
    await _peerConnectedSubscription?.cancel();

    // 停止心跳检测
    _keepalive?.stop();
    _keepalive?.destroy();
    _keepalive = null;

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

  /// 通知 Peer 断开连接（用于处理外部检测到的断开，如 LiveKit 事件）
  ///
  /// 当应用层检测到 Peer 断开（如 LiveKit ParticipantDisconnected）时调用此方法。
  /// 如果断开的是当前 Relay，会自动触发重新选举。
  void notifyPeerDisconnected(String peerId) {
    if (!_peers.contains(peerId)) return;

    _peers.remove(peerId);
    _coordinator.removePeer(peerId);
    _peerLeftController.add(peerId);

    // 如果是 Relay 断开，触发重新选举
    if (peerId == _currentRelay) {
      _currentRelay = null;
      _currentRelayScore = 0;
      triggerElection();
    }
  }

  /// 手动触发选举
  void triggerElection() {
    _currentEpoch++;
    _currentRelay = null;
    _currentRelayScore = 0;
    _updateState(AutoCoordinatorState.electing);
    _startElection(); // 启动选举（包含定时器）
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
    // 监听 Go 层事件（心跳事件可能没有 roomId，所以要单独处理）
    _eventSubscription = EventHandler.events.listen((event) {
      // 心跳事件（peerOffline/ping）需要特殊处理
      if (event.type == SfuEventType.peerOffline ||
          event.type == SfuEventType.ping) {
        _handleSfuEvent(event);
        return;
      }
      // 其他事件需要匹配 roomId
      if (event.roomId == roomId) {
        _handleSfuEvent(event);
      }
    });

    // 监听 Ping 请求
    _pingSubscription = PingHandler.pingRequests.listen(_handlePingRequest);

    // 监听 Peer 断开事件（自动检测 Relay 故障，无需客户端额外操作）
    _peerDisconnectedSubscription = signaling.peerDisconnected.listen((peerId) {
      notifyPeerDisconnected(peerId);
    });

    // 监听 Peer 连接事件（比 signaling join 消息更快）
    // 如果我们是 Relay，立即发送 relayChanged，避免新 Peer 错误地自选举
    _peerConnectedSubscription = signaling.peerConnected.listen((peerId) {
      if (peerId != localPeerId && _currentRelay == localPeerId) {
        signaling.sendRelayChanged(
          roomId,
          localPeerId,
          _currentEpoch,
          _localScore,
        );
      }
    });
  }

  void _handleSignalingMessage(SignalingMessage message) {
    if (message.peerId == localPeerId) return;

    // 自动检测未知 Peer（修复 join 消息可能丢失的竞争条件）
    // 当收到任何消息时，如果发送者不在 peers 列表中，添加它
    if (!_peers.contains(message.peerId) &&
        message.type != SignalingMessageType.leave) {
      _peers.add(message.peerId);
      _peerJoinedController.add(message.peerId);

      // 添加到心跳检测
      _keepalive?.addPeer(message.peerId);

      // 如果我们是 Relay，立即告诉新 Peer
      if (_currentRelay == localPeerId) {
        signaling.sendRelayChanged(
          roomId,
          localPeerId,
          _currentEpoch,
          _localScore,
        );
      }
    }

    switch (message.type) {
      case SignalingMessageType.join:
        _handlePeerJoined(message.peerId, message.data);
        break;

      case SignalingMessageType.leave:
        _handlePeerLeft(message.peerId);
        break;

      case SignalingMessageType.ping:
        // 收到 ping，回复 pong
        signaling.sendPong(roomId, message.peerId);
        break;

      case SignalingMessageType.pong:
        _coordinator.handlePong(message.peerId);
        _keepalive?.handlePong(message.peerId); // 通知心跳管理器
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
    // 如果 peer 已经存在（通过 auto-detect 添加），只更新设备信息
    final isNewPeer = !_peers.contains(peerId);

    if (isNewPeer) {
      _peers.add(peerId);
    }

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

    // 只有新 Peer 才触发事件和发送消息
    if (isNewPeer) {
      _peerJoinedController.add(peerId);

      // 新 Peer 加入时的处理
      if (config.autoElection) {
        if (_currentRelay == localPeerId) {
          // 我们是 Relay - 直接告诉新 Peer
          signaling.sendRelayChanged(
            roomId,
            localPeerId,
            _currentEpoch,
            _localScore,
          );
        } else if (_currentRelay == null) {
          // 还没有 Relay - 广播我们的 claim
          _broadcastClaim();
        }
      }
    }
  }

  void _handlePeerLeft(String peerId) {
    _peers.remove(peerId);
    _coordinator.removePeer(peerId);
    _keepalive?.removePeer(peerId); // 从心跳检测移除
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

    // 已经有稳定的 Relay，不参与选举
    if (_currentRelay != null) {
      // 如果我们是 Relay，告知 claimer 当前 Relay 信息
      if (_currentRelay == localPeerId) {
        signaling.sendRelayChanged(
          roomId,
          localPeerId,
          _currentEpoch,
          _localScore,
        );
      }
      // 否则忽略 claim（让 claimer 从 Relay 那里获取信息）
      return;
    }

    // 没有 Relay，进行选举冲突解决
    _resolveElection(peerId, epoch, score);
  }

  void _resolveElection(String claimerId, int epoch, double claimerScore) {
    // 如果已经有 Relay，需要比较 claimer 和当前 Relay 的分数
    if (_currentRelay != null && _currentRelay != localPeerId) {
      // 当前 Relay 不是我，比较 claimer 和当前 Relay
      if (claimerScore > _currentRelayScore) {
        // claimer 分数比当前 Relay 高，接受 claimer
        _acceptRelay(claimerId, epoch, claimerScore);
      } else if (claimerScore == _currentRelayScore &&
          claimerId.compareTo(_currentRelay!) > 0) {
        // 分数相同，比较 PeerId
        _acceptRelay(claimerId, epoch, claimerScore);
      }
      // 否则保持当前 Relay
      return;
    }

    // 当前 Relay 是我或没有 Relay，比较 claimer 和我的分数
    if (claimerScore > _localScore) {
      _acceptRelay(claimerId, epoch, claimerScore);
      return;
    }

    // 分数相同，比较 PeerId
    if (claimerScore == _localScore && claimerId.compareTo(localPeerId) > 0) {
      _acceptRelay(claimerId, epoch, claimerScore);
      return;
    }

    // 我们分数更高，保持或成为 Relay
    if (_currentRelay == null) {
      _becomeRelay();
    }
  }

  void _acceptRelay(String relayId, int epoch, double score) {
    _currentRelay = relayId;
    _currentEpoch = epoch;
    _currentRelayScore = score;
    _coordinator.setRelay(relayId, epoch);

    _electionTimer?.cancel();

    // 接受其他 Peer 为 Relay 时，更新状态为 connected
    // 无论当前是 electing 还是 asRelay，都需要更新
    if (_state != AutoCoordinatorState.idle) {
      _updateState(AutoCoordinatorState.connected);
    }

    _relayChangedController.add(relayId);
  }

  void _becomeRelay() {
    _currentRelay = localPeerId;
    _currentRelayScore = _localScore;
    _coordinator.setRelay(localPeerId, _currentEpoch);

    _electionTimer?.cancel();
    _updateState(AutoCoordinatorState.asRelay);

    // 广播我们成为 Relay
    signaling.sendRelayChanged(roomId, localPeerId, _currentEpoch, _localScore);

    _relayChangedController.add(localPeerId);
  }

  void _handleRelayChanged(Map<String, dynamic>? data) {
    final relayId = data?['relayId'] as String? ?? '';
    final epoch = data?['epoch'] as int? ?? 0;
    final score = (data?['score'] as num?)?.toDouble() ?? 0.0;

    // 忽略无效消息
    if (relayId.isEmpty || relayId == localPeerId) return;

    // 只忽略明显过期的 epoch（小于当前 epoch），相同 epoch 仍需处理
    if (epoch < _currentEpoch) return;

    // 更新 epoch 如果更高
    if (epoch > _currentEpoch) {
      _currentEpoch = epoch;
    }

    // 如果我们当前是 Relay，需要比较分数决定是否接受
    if (_currentRelay == localPeerId) {
      // 对方分数更高，接受
      if (score > _localScore) {
        _acceptRelay(relayId, epoch, score);
        return;
      }
      // 分数相同，比较 PeerId（字典序大的优先）
      if (score == _localScore && relayId.compareTo(localPeerId) > 0) {
        _acceptRelay(relayId, epoch, score);
        return;
      }
      // 我们分数更高或相同且 PeerId 更大，重新广播我们的 Relay 状态
      signaling.sendRelayChanged(
        roomId,
        localPeerId,
        _currentEpoch,
        _localScore,
      );
      return;
    }

    // 我们不是 Relay
    // 如果还没有 Relay 或者新 Relay 比当前 Relay 更优，接受
    if (_currentRelay == null) {
      _acceptRelay(relayId, epoch, score);
      return;
    }

    // 比较新 Relay 和当前 Relay
    if (score > _currentRelayScore ||
        (score == _currentRelayScore &&
            relayId.compareTo(_currentRelay!) > 0)) {
      _acceptRelay(relayId, epoch, score);
    }
    // 否则保持当前 Relay
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

      case SfuEventType.peerOffline:
        // 心跳超时检测到 Peer 离线
        notifyPeerDisconnected(event.peerId);
        break;

      case SfuEventType.ping:
        // Go 层需要发送 Ping，通过信令发送
        signaling.sendPing(roomId, event.peerId);
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

  void _startElection({bool isInitial = false}) {
    _broadcastClaim();

    // 设置选举超时
    // 初始选举使用更长时间（2x），给现有 Relay 时间来通知我们
    final timeoutMs = isInitial
        ? config.electionTimeoutMs * 2
        : config.electionTimeoutMs;

    _electionTimer = Timer(Duration(milliseconds: timeoutMs), () {
      // 超时后，如果没有确定 Relay，自己成为 Relay
      if (_currentRelay == null) {
        _becomeRelay();
      }
    });
  }

  void _broadcastClaim() {
    signaling.sendRelayClaim(roomId, _currentEpoch, _localScore);
  }
}
