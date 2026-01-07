/// AutoCoordinator - 真正的一键自动代理
///
/// 完整整合 Coordinator + Signaling + Callbacks，实现真正的自动切换
library;

import 'dart:async';
import 'dart:ffi';

import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:flutter/foundation.dart';
import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import 'dart:convert';
import '../callbacks/callbacks.dart';
import '../enums.dart';
import '../signaling/signaling.dart';
import 'coordinator.dart';

/// Bot Token 请求回调类型
/// 当设备当选为 Relay 时调用，返回 Bot Token 用于影子连接
typedef BotTokenCallback = Future<String?> Function(String roomId);

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

  /// 最大选举失败次数，超过后降级到直连 SFU 模式
  final int maxElectionFailures;

  /// 降级后自动恢复延迟（毫秒），0 表示不自动恢复
  final int recoveryDelayMs;

  /// LiveKit URL (Relay 模式专用，Go 层直连 SFU)
  final String? livekitUrl;

  /// 动态获取 Bot Token 的回调
  /// 只有当设备当选为 Relay 时才会调用
  /// 返回 null 表示不启动影子连接
  final BotTokenCallback? onRequestBotToken;

  const AutoCoordinatorConfig({
    this.deviceType = DeviceType.unknown,
    this.connectionType = ConnectionType.unknown,
    this.powerState = PowerState.unknown,
    this.electionTimeoutMs = 1000, // 1秒选举超时
    this.autoElection = true,
    this.maxElectionFailures = 3, // 连续3次失败后降级
    this.recoveryDelayMs = 30000, // 30秒后自动恢复
    this.livekitUrl,
    this.onRequestBotToken,
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
  // Keepalive? _keepalive; // 心跳管理器

  // 选举失败降级机制
  int _electionFailureCount = 0;
  bool _relayModeDisabled = false;
  Timer? _recoveryTimer;

  final Set<String> _peers = {};

  // 订阅
  StreamSubscription<SignalingMessage>? _signalingSubscription;
  StreamSubscription<SfuEvent>? _eventSubscription;
  StreamSubscription<PingRequest>? _pingSubscription;
  StreamSubscription<String>? _peerDisconnectedSubscription;
  StreamSubscription<String>? _peerConnectedSubscription;

  // P2P 订阅者连接（当本机不是 Relay 且在局域网时使用）
  RTCPeerConnection? _p2pConnection;
  MediaStream? _p2pRemoteStream;
  bool _p2pConnected = false;

  // 屏幕共享状态
  String? _screenSharerPeerId; // 当前屏幕共享者的 ID
  bool _isLocalScreenSharing = false; // 本机是否正在屏幕共享

  // 事件流
  final _stateController = StreamController<AutoCoordinatorState>.broadcast();
  final _relayChangedController = StreamController<String>.broadcast();
  final _peerJoinedController = StreamController<String>.broadcast();
  final _peerLeftController = StreamController<String>.broadcast();
  final _errorController = StreamController<String>.broadcast();
  final _remoteStreamController = StreamController<MediaStream?>.broadcast();
  final _screenShareChangedController = StreamController<String?>.broadcast();

  // 是否已销毁（用于防止向已关闭的 controller 添加事件）
  bool _disposed = false;

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

  /// 本设备是否在局域网上
  /// 蜂窝网络不在局域网，不应参与 Relay 系统
  bool get isOnLan =>
      config.connectionType == ConnectionType.ethernet ||
      config.connectionType == ConnectionType.wifi;

  /// 本设备是否可以成为 Relay
  /// 只有 WiFi 或 Ethernet 连接的设备才能成为 Relay
  bool get canBeRelay =>
      config.connectionType == ConnectionType.ethernet ||
      config.connectionType == ConnectionType.wifi;

  /// 房间内所有 Peer
  Set<String> get peers => Set.unmodifiable(_peers);

  /// P2P 远程视频流（订阅者从 Relay 接收的视频）
  /// 只有局域网订阅者才会有此流，蜂窝网络设备为 null
  MediaStream? get p2pRemoteStream => _p2pRemoteStream;

  /// P2P 连接是否已建立
  bool get hasP2PConnection => _p2pConnected && _p2pRemoteStream != null;

  /// 当前屏幕共享者的 ID（如果没有人共享则为 null）
  String? get screenSharerPeerId => _screenSharerPeerId;

  /// 是否有远程屏幕共享且 P2P 已连接
  /// 局域网订阅者使用此属性判断是否应该显示屏幕共享
  bool get hasRemoteScreenShare =>
      _screenSharerPeerId != null &&
      _screenSharerPeerId != localPeerId &&
      (hasP2PConnection || !isOnLan);

  /// 本机是否正在屏幕共享
  bool get isLocalScreenSharing => _isLocalScreenSharing;

  // ========== 事件流 ==========

  Stream<AutoCoordinatorState> get onStateChanged => _stateController.stream;
  Stream<String> get onRelayChanged => _relayChangedController.stream;
  Stream<String> get onPeerJoined => _peerJoinedController.stream;
  Stream<String> get onPeerLeft => _peerLeftController.stream;

  /// P2P 远程流变化事件
  /// 当 P2P 连接建立或断开时触发
  Stream<MediaStream?> get onRemoteStream => _remoteStreamController.stream;
  Stream<String> get onError => _errorController.stream;

  /// 屏幕共享者变化事件
  /// 当有人开始/停止屏幕共享时触发，发出共享者 ID（null 表示没人共享）
  Stream<String?> get onScreenShareChanged =>
      _screenShareChangedController.stream;

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

      // 让 UI 有机会更新
      await Future.delayed(Duration.zero);

      // 2. 启用 Coordinator (FFI 调用)
      _coordinator.enable();

      // 让 UI 有机会更新
      await Future.delayed(Duration.zero);

      // 3. 更新本机设备信息
      _updateLocalDeviceInfo();

      // 4. 连接信令
      await signaling.connect();

      // 蜂窝网络设备：等待 DataChannel 稳定后再发送信令消息
      // 这解决了蜂窝网络上 publishData 间歇性超时的问题
      if (config.connectionType == ConnectionType.cellular) {
        await Future.delayed(const Duration(seconds: 2));
      }

      await signaling.joinRoom(roomId, localPeerId);

      // 5. 设置所有监听
      _setupListeners();

      // 让 UI 有机会更新
      await Future.delayed(Duration.zero);

      // 6. 开始选举或直接连接
      // maxElectionFailures <= 0 表示禁用 Relay 模式，直接走 SFU
      if (config.maxElectionFailures <= 0) {
        _relayModeDisabled = true;
        _updateState(AutoCoordinatorState.connected);
      } else {
        _updateState(AutoCoordinatorState.electing);

        if (config.autoElection) {
          _startElection(isInitial: true); // 初始选举使用更长超时
        }
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
    // _keepalive?.stop();
    // _keepalive?.destroy();
    // _keepalive = null;

    // 如果是 Relay，断开 Go 层 LiveKit 连接
    if (isRelay) {
      _disconnectLiveKitBridge();
    }

    _coordinator.disable();

    // 取消恢复定时器
    _recoveryTimer?.cancel();
    _recoveryTimer = null;

    try {
      await signaling.leaveRoom(roomId);
    } catch (_) {}

    // 断开 P2P 订阅者连接
    await _closeP2PConnection();

    _peers.clear();
    _currentRelay = null;
    _electionFailureCount = 0;
    _relayModeDisabled = false;
    _updateState(AutoCoordinatorState.idle);
  }

  /// 释放资源
  void dispose() {
    _disposed = true; // 先标记为已销毁，防止后续操作添加事件
    stop();
    signaling.dispose();
    _stateController.close();
    _relayChangedController.close();
    _peerJoinedController.close();
    _peerLeftController.close();
    _errorController.close();
    _remoteStreamController.close();
    _screenShareChangedController.close();
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

  /// 通知屏幕共享已开始
  ///
  /// 当本地用户开始屏幕共享时调用此方法，会通过信令广播给其他用户
  void notifyScreenShareStarted() {
    _isLocalScreenSharing = true;
    _screenSharerPeerId = localPeerId;
    if (!_disposed) {
      _screenShareChangedController.add(localPeerId);
    }
    // 广播给其他用户
    signaling.sendScreenShare(roomId, true);
  }

  /// 通知屏幕共享已停止
  ///
  /// 当本地用户停止屏幕共享时调用此方法，会通过信令广播给其他用户
  void notifyScreenShareStopped() {
    _isLocalScreenSharing = false;
    if (_screenSharerPeerId == localPeerId) {
      _screenSharerPeerId = null;
      if (!_disposed) {
        _screenShareChangedController.add(null);
      }
    }
    // 广播给其他用户
    signaling.sendScreenShare(roomId, false);
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
        _localScore -= 100; // 蜂窝网络：有效禁止成为 Relay
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

      // 添加到心跳检测 (Go 层自动处理)
      // _keepalive?.addPeer(message.peerId);

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
        // _keepalive?.handlePong(message.peerId); // Go 层自动处理
        break;

      case SignalingMessageType.relayClaim:
        _handleRelayClaim(message.peerId, message.data);
        break;

      case SignalingMessageType.relayChanged:
        _handleRelayChanged(message.data);
        break;

      case SignalingMessageType.offer:
        // Relay 收到订阅者的 Offer（Go 层 RelayRoom 处理）
        _handleOfferFromSubscriber(message.peerId, message.data);
        break;

      case SignalingMessageType.answer:
        // 订阅者收到 Relay 的 Answer
        if (message.data != null && message.data!['sdp'] != null) {
          _handleP2PAnswer(message.peerId, message.data!['sdp'] as String);
        }
        break;

      case SignalingMessageType.candidate:
        // 收到 ICE 候选
        if (message.data != null) {
          if (isRelay) {
            // Relay 收到订阅者的 ICE 候选
            _handleCandidateFromSubscriber(message.peerId, message.data);
          } else if (message.data!['candidate'] != null) {
            // 订阅者收到 Relay 的 ICE 候选
            _handleP2PCandidate(
              message.peerId,
              message.data!['candidate'] as String,
            );
          }
        }
        break;

      case SignalingMessageType.screenShare:
        // 收到屏幕共享状态变更
        _handleScreenShareMessage(message.peerId, message.data);
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

      // 如果本地正在屏幕共享，告诉新 Peer
      // 这确保后加入的 Peer 能知道当前谁在共享屏幕
      debugPrint(
        '[AutoCoordinator] New peer joined: $peerId, isLocalScreenSharing: $_isLocalScreenSharing',
      );
      if (_isLocalScreenSharing) {
        debugPrint(
          '[AutoCoordinator] Sending screenShare to new peer: $peerId',
        );
        signaling.sendScreenShare(roomId, true);
      }
    }
  }

  void _handlePeerLeft(String peerId) {
    _peers.remove(peerId);
    _coordinator.removePeer(peerId);
    _coordinator.removePeer(peerId);
    // _keepalive?.removePeer(peerId); // Go 层自动处理
    if (!_disposed) {
      _peerLeftController.add(peerId);
    }

    // 如果离开的是屏幕共享者，清除屏幕共享状态
    if (_screenSharerPeerId == peerId) {
      _screenSharerPeerId = null;
      if (!_disposed) {
        _screenShareChangedController.add(null);
      }
    }

    // 如果是 Relay 离开，触发重新选举
    if (peerId == _currentRelay) {
      _currentRelay = null;
      triggerElection();
    }
  }

  /// 处理屏幕共享消息
  void _handleScreenShareMessage(String peerId, Map<String, dynamic>? data) {
    final isSharing = data?['isSharing'] as bool? ?? false;

    if (isSharing) {
      // 有人开始屏幕共享
      _screenSharerPeerId = peerId;
    } else {
      // 有人停止屏幕共享
      if (_screenSharerPeerId == peerId) {
        _screenSharerPeerId = null;
      }
    }

    if (!_disposed) {
      _screenShareChangedController.add(_screenSharerPeerId);
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

    // 如果我们是当前 Relay，直接通知 claimer（不允许抢占）
    if (_currentRelay == localPeerId) {
      signaling.sendRelayChanged(
        roomId,
        localPeerId,
        _currentEpoch,
        _localScore,
      );
      return;
    }

    // 已有其他 Relay，忽略 claim（claimer 会收到 Relay 的通知）
    if (_currentRelay != null) {
      return;
    }

    // 没有 Relay，进行选举冲突解决
    _resolveElection(peerId, epoch, score);
  }

  void _resolveElection(String claimerId, int epoch, double claimerScore) {
    // 「先到先得」逻辑：如果我们正在选举中（已广播 claim，等待超时），
    // 则不向后来的节点让位，无论其分数高低。
    // 这样可以避免新加入的节点抢占正在选举的节点。
    if (_state == AutoCoordinatorState.electing) {
      // 我们正在选举，不让位。告知对方我们的存在。
      signaling.sendRelayClaim(roomId, _currentEpoch, _localScore);
      return;
    }

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
    } else {
      // 我们已经是 Relay，广播 relayChanged 告知 claimer
      signaling.sendRelayChanged(
        roomId,
        localPeerId,
        _currentEpoch,
        _localScore,
      );
    }
  }

  void _acceptRelay(String relayId, int epoch, double score) {
    // 如果之前是 Relay，需要断开影子连接
    final wasRelay = isRelay;
    if (wasRelay) {
      _disconnectLiveKitBridge();
    }

    // 蜂窝设备不记录 Relay 信息（它们不在同一局域网，无法使用 Relay）
    // 但仍然需要更新 epoch 以保持同步
    _currentEpoch = epoch;

    if (isOnLan) {
      // 只有局域网设备才记录 Relay 信息
      _currentRelay = relayId;
      _currentRelayScore = score;
      _coordinator.setRelay(relayId, epoch);
      _relayChangedController.add(relayId);
    } else {
      // 蜂窝设备：不设置 currentRelay，保持 null
      _currentRelay = null;
      _currentRelayScore = 0;
    }

    _electionTimer?.cancel();

    // 接受其他 Peer 为 Relay 时，更新状态为 connected
    // 无论当前是 electing 还是 asRelay，都需要更新
    if (_state != AutoCoordinatorState.idle) {
      _updateState(AutoCoordinatorState.connected);
    }

    // 局域网订阅者：创建到 Relay 的 P2P 连接
    if (isOnLan && !isRelay) {
      _createP2PConnectionToRelay(relayId);
    }
  }

  void _becomeRelay() {
    // 安全检查：只有在局域网上的设备才能成为 Relay
    if (!isOnLan) {
      // 蜂窝网络设备不能成为 Relay，直接进入已连接状态
      _updateState(AutoCoordinatorState.connected);
      return;
    }

    _currentRelay = localPeerId;
    _currentRelayScore = _localScore;
    _coordinator.setRelay(localPeerId, _currentEpoch);

    _electionTimer?.cancel();
    _updateState(AutoCoordinatorState.asRelay);

    // 启动 Go 层 LiveKit 桥接（如果配置了 URL 和 Token）
    _connectLiveKitBridge();

    // 广播我们成为 Relay
    signaling.sendRelayChanged(roomId, localPeerId, _currentEpoch, _localScore);

    _relayChangedController.add(localPeerId);
  }

  /// 连接 Go 层 LiveKit 桥接器（影子连接）
  /// 只有当设备当选为 Relay 时才会调用
  void _connectLiveKitBridge() {
    // 检查是否配置了 URL 和回调
    if (config.livekitUrl == null || config.onRequestBotToken == null) {
      // 没有配置影子连接，跳过
      return;
    }

    // 直接异步执行（Go 层 LiveKitBridgeConnect 已在 goroutine 中运行，不会阻塞）
    _connectLiveKitBridgeAsync();
  }

  /// 异步连接 LiveKit 桥接器
  Future<void> _connectLiveKitBridgeAsync() async {
    try {
      // 动态请求 Bot Token
      final botToken = await config.onRequestBotToken!(roomId);
      if (botToken == null || botToken.isEmpty) {
        // 回调返回空，不启动影子连接
        return;
      }

      final roomPtr = toCString(roomId);
      final urlPtr = toCString(config.livekitUrl!);
      final tokenPtr = toCString(botToken);

      try {
        print(
          '[AutoCoordinator] connecting native bridge to $urlPtr with token length ${botToken.length}',
        );

        // 1. 创建 RelayRoom (P2P 服务端)
        final iceServersPtr = toCString('[]');
        try {
          bindings.RelayRoomCreate(roomPtr, iceServersPtr);
          print('[AutoCoordinator] RelayRoom created');
        } finally {
          calloc.free(iceServersPtr);
        }

        // 2. 创建桥接器 (影子连接)
        bindings.LiveKitBridgeCreate(roomPtr);
        print('[AutoCoordinator] LiveKitBridgeCreate done');

        // 3. 连接到 LiveKit SFU (Go 层异步执行)
        bindings.LiveKitBridgeConnect(roomPtr, urlPtr, tokenPtr);
        print('[AutoCoordinator] LiveKitBridgeConnect started');
      } finally {
        calloc.free(roomPtr);
        calloc.free(urlPtr);
        calloc.free(tokenPtr);
      }
    } catch (e, stack) {
      // 影子连接失败不应阻塞主流程，只记录错误
      // ignore: avoid_print
      print('[AutoCoordinator] 影子连接启动失败: $e\n$stack');
    }
  }

  /// 断开 Go 层 LiveKit 桥接器
  void _disconnectLiveKitBridge() {
    final roomPtr = toCString(roomId);
    try {
      // 1. 断开并销毁 Bridge
      bindings.LiveKitBridgeDisconnect(roomPtr);
      bindings.LiveKitBridgeDestroy(roomPtr);

      // 2. 销毁 RelayRoom
      bindings.RelayRoomDestroy(roomPtr);
      print('[AutoCoordinator] LiveKitBridge & RelayRoom destroyed');
    } finally {
      calloc.free(roomPtr);
    }
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
        // 关键修复：Go 层 Coordinator 有时会通过 EventTypePeerOffline (22)
        // 发送 action="ping_request" 的数据，这不是真正的离线，而是需要发 Ping
        // 必须检查 data 字段，否则会导致误判离线，引发 Split Brain
        if (event.data != null && event.data!.contains('ping_request')) {
          signaling.sendPing(roomId, event.peerId);
          return;
        }

        // 心跳超时检测到 Peer 离线
        notifyPeerDisconnected(event.peerId);
        break;

      case SfuEventType.ping:
        // Go 层需要发送 Ping，通过信令发送
        signaling.sendPing(roomId, event.peerId);
        break;

      case SfuEventType.iceCandidate:
        // Relay 生成了面向订阅者的 ICE 候选，通过信令发送给订阅者
        if (event.data != null && event.peerId.isNotEmpty) {
          // event.data 是 JSON String，直接发送
          print(
            '[Relay] Forwarding ICE candidate to subscriber: ${event.peerId}',
          );
          signaling.sendCandidate(roomId, event.peerId, event.data!);
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

  void _startElection({bool isInitial = false}) {
    // 如果 Relay 模式已降级，不参与选举
    if (_relayModeDisabled) {
      _updateState(AutoCoordinatorState.connected);
      return;
    }

    // 不在局域网的设备不参与选举（蜂窝网络等）
    // 这些设备不广播 claim，等待其他有 LAN 的设备成为 Relay
    if (!isOnLan) {
      _updateState(AutoCoordinatorState.connected);
      return;
    }

    _broadcastClaim();

    // 设置选举超时
    // 初始选举使用更长时间（2x），给现有 Relay 时间来通知我们
    final timeoutMs = isInitial
        ? config.electionTimeoutMs * 2
        : config.electionTimeoutMs;

    _electionTimer = Timer(Duration(milliseconds: timeoutMs), () {
      // 超时后，如果没有确定 Relay
      if (_currentRelay == null) {
        _electionFailureCount++;

        // 检查是否超过最大失败次数
        if (_electionFailureCount >= config.maxElectionFailures) {
          _disableRelayMode();
          return;
        }

        // 继续尝试成为 Relay
        _becomeRelay();
      } else {
        // 选举成功（有 Relay），重置失败计数
        _electionFailureCount = 0;
      }
    });
  }

  /// 降级：禁用 Relay 模式，直接走 SFU
  void _disableRelayMode() {
    _relayModeDisabled = true;
    _electionTimer?.cancel();
    _updateState(AutoCoordinatorState.connected);

    // 通知应用层已降级
    _errorController.add(
      'Relay mode disabled after $_electionFailureCount consecutive failures, will retry in ${config.recoveryDelayMs}ms',
    );

    // 自动恢复定时器
    if (config.recoveryDelayMs > 0) {
      _recoveryTimer?.cancel();
      _recoveryTimer = Timer(
        Duration(milliseconds: config.recoveryDelayMs),
        () {
          _enableRelayMode();
        },
      );
    }
  }

  /// 恢复 Relay 模式，重新尝试选举
  void _enableRelayMode() {
    if (!_relayModeDisabled) return;

    _relayModeDisabled = false;
    _electionFailureCount = 0;

    // 如果已经有 Relay 存在，不需要重新选举
    if (_currentRelay != null) {
      _errorController.add(
        'Relay mode re-enabled, existing Relay: $_currentRelay',
      );
      return;
    }

    _errorController.add('Relay mode re-enabled, starting new election');

    // 重新开始选举
    if (config.autoElection && isOnLan) {
      _startElection(isInitial: true);
    }
  }

  void _broadcastClaim() {
    // 蜂窝网络设备不需要广播 claim，它们无法成为 Relay
    if (!isOnLan) return;
    signaling.sendRelayClaim(roomId, _currentEpoch, _localScore);
  }

  // ========== P2P 订阅者连接 ==========

  /// 创建到 Relay 的 P2P 连接（订阅者使用）
  Future<void> _createP2PConnectionToRelay(String relayId) async {
    // 只有局域网设备才能使用 P2P
    if (!isOnLan) {
      print('[P2P] Not on LAN, skipping P2P connection');
      return;
    }

    // Relay 不需要创建 P2P 连接
    if (isRelay) return;

    // 如果已有连接，先关闭
    await _closeP2PConnection();

    try {
      final configuration = <String, dynamic>{
        'iceServers': [
          {'urls': 'stun:stun.l.google.com:19302'},
        ],
        'sdpSemantics': 'unified-plan',
      };

      _p2pConnection = await createPeerConnection(configuration);

      // 添加收发器以接收视频和音频
      await _p2pConnection!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
        init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
      );
      await _p2pConnection!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
        init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
      );

      // 监听远程流
      _p2pConnection!.onTrack = (RTCTrackEvent event) {
        if (event.streams.isNotEmpty) {
          _p2pRemoteStream = event.streams.first;
          _remoteStreamController.add(_p2pRemoteStream);
          print('[P2P] Received remote stream from Relay');
        }
      };

      // 监听连接状态
      _p2pConnection!.onConnectionState = (RTCPeerConnectionState state) {
        print('[P2P] Connection state: $state');
        if (state == RTCPeerConnectionState.RTCPeerConnectionStateConnected) {
          print('[P2P] Remote stream connected! Ready to render.');
          _p2pConnected = true;
        } else if (state ==
                RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
            state ==
                RTCPeerConnectionState.RTCPeerConnectionStateDisconnected ||
            state == RTCPeerConnectionState.RTCPeerConnectionStateClosed) {
          print('[P2P] Remote stream disconnected or failed: $state');
          _p2pConnected = false;
          _p2pRemoteStream = null;
          _remoteStreamController.add(null);
        }
      };

      // 监听 ICE 候选
      _p2pConnection!.onIceCandidate = (RTCIceCandidate candidate) {
        // 使用标准 JSON 格式发送候选
        signaling.sendCandidate(roomId, relayId, jsonEncode(candidate.toMap()));
      };

      // 创建 Offer
      final offer = await _p2pConnection!.createOffer();
      await _p2pConnection!.setLocalDescription(offer);

      // 发送 Offer 给 Relay
      signaling.sendOffer(roomId, relayId, offer.sdp!);
      print('[P2P] Sent offer to Relay: $relayId');
    } catch (e) {
      print('[P2P] Failed to create P2P connection: $e');
      _errorController.add('P2P connection failed: $e');
    }
  }

  /// 关闭 P2P 连接
  Future<void> _closeP2PConnection() async {
    if (_p2pConnection != null) {
      await _p2pConnection!.close();
      _p2pConnection = null;
    }
    _p2pRemoteStream = null;
    _p2pConnected = false;
    // 检查是否已销毁，避免向已关闭的 controller 添加事件
    if (!_disposed) {
      _remoteStreamController.add(null);
    }
  }

  /// 处理 Answer（订阅者收到 Relay 的 Answer）
  Future<void> _handleP2PAnswer(String peerId, String sdp) async {
    if (_p2pConnection == null) return;

    try {
      await _p2pConnection!.setRemoteDescription(
        RTCSessionDescription(sdp, 'answer'),
      );
      print('[P2P] Set remote description from Relay: $peerId');
    } catch (e) {
      print('[P2P] Failed to set remote description: $e');
    }
  }

  /// 处理 ICE 候选
  Future<void> _handleP2PCandidate(
    String peerId,
    String candidateJsonStr,
  ) async {
    if (_p2pConnection == null) return;

    try {
      final map = jsonDecode(candidateJsonStr) as Map<String, dynamic>;
      final candidate = RTCIceCandidate(
        map['candidate'],
        map['sdpMid'],
        map['sdpMLineIndex'],
      );
      print('[P2P] Adding ICE candidate from Relay: ${candidate.candidate}');
      await _p2pConnection!.addCandidate(candidate);
    } catch (e) {
      print('[P2P] Failed to add ICE candidate: $e');
    }
  }

  /// Relay 处理订阅者的 Offer
  void _handleOfferFromSubscriber(
    String subscriberId,
    Map<String, dynamic>? data,
  ) {
    // 只有 Relay 才处理 Offer
    if (!isRelay) return;

    final sdp = data?['sdp'] as String?;
    if (sdp == null) return;

    try {
      // 调用 Go 层 RelayRoom 添加订阅者，返回 Answer
      final roomPtr = toCString(roomId);
      final peerPtr = toCString(subscriberId);
      final offerPtr = toCString(sdp);

      try {
        final answerPtr = bindings.RelayRoomAddSubscriber(
          roomPtr,
          peerPtr,
          offerPtr,
        );
        if (answerPtr != Pointer.fromAddress(0)) {
          final answerSdp = fromCString(answerPtr);
          // 发送 Answer 给订阅者
          signaling.sendAnswer(roomId, subscriberId, answerSdp);
          print('[Relay] Sent answer to subscriber: $subscriberId');
        } else {
          print(
            '[Relay] Failed to create answer for subscriber: $subscriberId',
          );
        }
      } finally {
        calloc.free(roomPtr);
        calloc.free(peerPtr);
        calloc.free(offerPtr);
      }
    } catch (e) {
      print('[Relay] Error handling offer from subscriber: $e');
    }
  }

  /// Relay 处理订阅者的 ICE 候选
  void _handleCandidateFromSubscriber(
    String subscriberId,
    Map<String, dynamic>? data,
  ) {
    // 只有 Relay 才处理
    if (!isRelay) return;

    // data['candidate'] 是 JSON 字符串
    final candidateJsonStr = data?['candidate'] as String?;
    if (candidateJsonStr == null) return;

    try {
      final roomPtr = toCString(roomId);
      final peerPtr = toCString(subscriberId);
      final candidatePtr = toCString(candidateJsonStr);
      print('[Relay] Adding ICE candidate from Subscriber: $subscriberId');

      try {
        bindings.RelayRoomAddICECandidate(roomPtr, peerPtr, candidatePtr);
      } finally {
        calloc.free(roomPtr);
        calloc.free(peerPtr);
        calloc.free(candidatePtr);
      }
    } catch (e) {
      print('[Relay] Error handling ICE candidate from subscriber: $e');
    }
  }
}
