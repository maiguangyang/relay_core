/// Failover - 故障切换 API
///
/// 自动 Relay 故障检测和切换，包含冲突解决机制
library;

import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 故障切换状态
class FailoverState {
  final String currentRelay;
  final int epoch;
  final double localScore;
  final bool isActive;

  FailoverState({
    required this.currentRelay,
    required this.epoch,
    required this.localScore,
    required this.isActive,
  });

  factory FailoverState.fromJson(Map<String, dynamic> json) {
    return FailoverState(
      currentRelay: json['current_relay'] ?? '',
      epoch: json['epoch'] ?? 0,
      localScore: (json['local_score'] as num?)?.toDouble() ?? 0.0,
      isActive: json['is_active'] ?? false,
    );
  }

  @override
  String toString() =>
      'FailoverState(relay: $currentRelay, epoch: $epoch, active: $isActive)';
}

/// Failover - 故障切换管理器
///
/// 独立的故障切换 API，用于需要自定义切换逻辑的场景。
/// 如果使用 [Coordinator]，故障切换已自动集成。
///
/// ```dart
/// final failover = Failover(roomId: 'room-1', localPeerId: 'peer-1');
///
/// // 启用故障切换
/// failover.enable();
///
/// // 设置当前 Relay
/// failover.setCurrentRelay('relay-peer', epoch: 1);
///
/// // 更新本机分数
/// failover.updateLocalScore(85.0);
///
/// // 接收其他节点的 Relay 声明（冲突解决）
/// failover.receiveClaim('other-peer', epoch: 2, score: 90.0);
/// ```
class Failover {
  final String roomId;
  final String localPeerId;

  Failover({required this.roomId, required this.localPeerId});

  /// 启用故障切换
  bool enable() {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(localPeerId);
    try {
      return bindings.FailoverEnable(roomPtr, peerPtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 禁用故障切换
  bool disable() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.FailoverDisable(roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 设置当前 Relay
  ///
  /// 收到信令通知时调用
  bool setCurrentRelay(String relayId, {required int epoch}) {
    final roomPtr = toCString(roomId);
    final relayPtr = toCString(relayId);
    try {
      return bindings.FailoverSetCurrentRelay(roomPtr, relayPtr, epoch) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(relayPtr);
    }
  }

  /// 更新本机分数
  ///
  /// 用于选举排序，分数高的节点优先成为 Relay
  bool updateLocalScore(double score) {
    final roomPtr = toCString(roomId);
    try {
      return bindings.FailoverUpdateLocalScore(roomPtr, score) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 接收 Relay 声明
  ///
  /// 收到其他节点的 Relay 声明时调用，用于冲突解决
  /// 冲突解决规则：
  /// 1. epoch 更高者优先
  /// 2. 同 epoch，分数高者优先
  /// 3. 分数相同，PeerID 字典序大者优先
  bool receiveClaim(
    String peerId, {
    required int epoch,
    required double score,
  }) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.FailoverReceiveClaim(roomPtr, peerPtr, epoch, score) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 获取故障切换状态
  FailoverState? getState() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.FailoverGetState(roomPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return null;
      return FailoverState.fromJson(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }
}
