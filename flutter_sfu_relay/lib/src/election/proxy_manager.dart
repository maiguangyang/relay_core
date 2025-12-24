/// ProxyManager - 代理管理器
///
/// 自动处理 Relay 选举、故障切换和代理状态
library;

import 'dart:async';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../core/coordinator.dart';
import '../callbacks/callbacks.dart';
import '../enums.dart';

/// 代理状态
enum ProxyState {
  /// 未初始化
  idle,

  /// 选举中
  electing,

  /// 等待 Relay
  waitingRelay,

  /// 作为 Relay
  asRelay,

  /// 连接到 Relay
  connectedToRelay,

  /// 故障切换中
  failover,

  /// 错误
  error,
}

/// Relay 信息
class RelayInfo {
  final String peerId;
  final int epoch;
  final double score;
  final DateTime electedAt;

  RelayInfo({
    required this.peerId,
    required this.epoch,
    this.score = 0.0,
    DateTime? electedAt,
  }) : electedAt = electedAt ?? DateTime.now();

  @override
  String toString() =>
      'RelayInfo(peerId: $peerId, epoch: $epoch, score: $score)';
}

/// ProxyManager - 代理管理器
///
/// 自动管理 Relay 选举和故障切换：
/// - 监控 Peer 状态
/// - 参与/执行选举
/// - 自动故障切换
/// - 无缝源切换
///
/// ```dart
/// final proxyManager = ProxyManager(
///   roomId: 'room-1',
///   localPeerId: 'peer-1',
/// );
///
/// proxyManager.onStateChanged.listen((state) {
///   print('Proxy state: $state');
/// });
///
/// proxyManager.onRelayChanged.listen((relay) {
///   print('New relay: ${relay.peerId}');
/// });
///
/// await proxyManager.start();
/// ```
class ProxyManager {
  final String roomId;
  final String localPeerId;
  final DeviceType deviceType;
  final ConnectionType connectionType;
  final PowerState powerState;

  late final Coordinator _coordinator;
  ProxyState _state = ProxyState.idle;
  RelayInfo? _currentRelay;
  int _currentEpoch = 0;

  StreamSubscription<SfuEvent>? _eventSubscription;

  final _stateController = StreamController<ProxyState>.broadcast();
  final _relayChangedController = StreamController<RelayInfo>.broadcast();
  final _electionController = StreamController<void>.broadcast();

  ProxyManager({
    required this.roomId,
    required this.localPeerId,
    this.deviceType = DeviceType.unknown,
    this.connectionType = ConnectionType.unknown,
    this.powerState = PowerState.unknown,
  }) {
    _coordinator = Coordinator(roomId: roomId, localPeerId: localPeerId);
  }

  /// 当前代理状态
  ProxyState get state => _state;

  /// 当前 Relay 信息
  RelayInfo? get currentRelay => _currentRelay;

  /// 是否是 Relay
  bool get isRelay => _coordinator.isRelay;

  /// 当前 Epoch
  int get currentEpoch => _currentEpoch;

  // 事件流
  Stream<ProxyState> get onStateChanged => _stateController.stream;
  Stream<RelayInfo> get onRelayChanged => _relayChangedController.stream;
  Stream<void> get onElectionTriggered => _electionController.stream;

  /// 启动代理管理器
  Future<void> start() async {
    if (_state != ProxyState.idle) {
      throw StateError('ProxyManager already started');
    }

    // 初始化回调
    EventHandler.init();

    // 启用 Coordinator
    _coordinator.enable();

    // 更新本地设备信息
    _updateLocalDeviceInfo();

    // 监听事件
    _eventSubscription = EventHandler.events
        .where((e) => e.roomId == roomId)
        .listen(_handleEvent);

    _updateState(ProxyState.electing);

    // 计算本地分数并广播声明
    final score = _calculateLocalScore();
    _broadcastClaim(score);
  }

  /// 停止代理管理器
  Future<void> stop() async {
    await _eventSubscription?.cancel();
    _coordinator.disable();
    _currentRelay = null;
    _updateState(ProxyState.idle);
  }

  /// 手动触发选举
  void triggerElection() {
    _currentEpoch++;
    _updateState(ProxyState.electing);
    _electionController.add(null);

    final score = _calculateLocalScore();
    _broadcastClaim(score);
  }

  /// 处理接收到的 Relay 声明
  void handleRelayClaim(String peerId, int epoch, double score) {
    if (epoch < _currentEpoch) return; // 忽略过期声明

    if (epoch > _currentEpoch) {
      _currentEpoch = epoch;
      _updateState(ProxyState.electing);
    }

    _coordinator.receiveClaim(peerId, epoch, score);
  }

  /// 处理 Relay 变更通知
  void handleRelayChanged(String relayId, int epoch) {
    if (epoch < _currentEpoch) return;

    _currentEpoch = epoch;
    _coordinator.setRelay(relayId, epoch);

    _currentRelay = RelayInfo(peerId: relayId, epoch: epoch);
    _relayChangedController.add(_currentRelay!);

    if (relayId == localPeerId) {
      _updateState(ProxyState.asRelay);
    } else {
      _updateState(ProxyState.connectedToRelay);
    }
  }

  /// 开始故障切换
  void startFailover() {
    _updateState(ProxyState.failover);
    triggerElection();
  }

  /// 获取状态摘要
  Map<String, dynamic> getStatus() {
    final coordStatus = _coordinator.getStatus();
    return {
      'state': _state.name,
      'isRelay': isRelay,
      'currentEpoch': _currentEpoch,
      'currentRelay': _currentRelay?.peerId,
      ...coordStatus,
    };
  }

  /// 释放资源
  void dispose() {
    stop();
    _stateController.close();
    _relayChangedController.close();
    _electionController.close();
  }

  void _updateState(ProxyState newState) {
    if (_state == newState) return;
    _state = newState;
    _stateController.add(newState);
  }

  void _updateLocalDeviceInfo() {
    final roomPtr = toCString(roomId);
    bindings.CoordinatorUpdateLocalDevice(
      roomPtr,
      deviceType.value,
      connectionType.value,
      powerState.value,
    );
    calloc.free(roomPtr);
  }

  double _calculateLocalScore() {
    // 分数计算规则：
    // - PC + Ethernet + PluggedIn = 最高分
    // - Mobile + Cellular + LowBattery = 最低分
    double score = 50.0;

    // 设备类型加分
    switch (deviceType) {
      case DeviceType.pc:
        score += 30;
        break;
      case DeviceType.pad:
        score += 20;
        break;
      case DeviceType.tv:
        score += 15;
        break;
      case DeviceType.mobile:
        score += 10;
        break;
      default:
        break;
    }

    // 连接类型加分
    switch (connectionType) {
      case ConnectionType.ethernet:
        score += 30;
        break;
      case ConnectionType.wifi:
        score += 20;
        break;
      case ConnectionType.cellular:
        score += 5;
        break;
      default:
        break;
    }

    // 电源状态加分
    switch (powerState) {
      case PowerState.pluggedIn:
        score += 20;
        break;
      case PowerState.battery:
        score += 10;
        break;
      case PowerState.lowBattery:
        score -= 20;
        break;
      default:
        break;
    }

    return score;
  }

  void _broadcastClaim(double score) {
    // 这里需要通过信令广播，由上层 RelayRoom 处理
    // ProxyManager 只负责计算和状态管理
  }

  void _handleEvent(SfuEvent event) {
    switch (event.type) {
      case SfuEventType.relayChanged:
        final data = event.data;
        if (data != null) {
          // 解析 relay 变更数据
          handleRelayChanged(event.peerId, _currentEpoch);
        }
        break;
      case SfuEventType.error:
        _updateState(ProxyState.error);
        break;
      default:
        break;
    }
  }
}
