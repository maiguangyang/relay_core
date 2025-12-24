/// Election - 独立选举 API
///
/// 动态代理选举系统，基于设备性能、网络质量、电源状态自动选择最优 Relay
library;

import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../enums.dart';

/// 候选者信息
class CandidateInfo {
  final String peerId;
  final double score;
  final DeviceType deviceType;
  final ConnectionType connectionType;
  final PowerState powerState;
  final int bandwidth;
  final int latency;
  final double packetLoss;

  CandidateInfo({
    required this.peerId,
    required this.score,
    this.deviceType = DeviceType.unknown,
    this.connectionType = ConnectionType.unknown,
    this.powerState = PowerState.unknown,
    this.bandwidth = 0,
    this.latency = 0,
    this.packetLoss = 0.0,
  });

  factory CandidateInfo.fromJson(Map<String, dynamic> json) {
    return CandidateInfo(
      peerId: json['peer_id'] ?? '',
      score: (json['score'] as num?)?.toDouble() ?? 0.0,
      deviceType: DeviceType.values.firstWhere(
        (e) => e.value == (json['device_type'] ?? 0),
        orElse: () => DeviceType.unknown,
      ),
      connectionType: ConnectionType.values.firstWhere(
        (e) => e.value == (json['connection_type'] ?? 0),
        orElse: () => ConnectionType.unknown,
      ),
      powerState: PowerState.values.firstWhere(
        (e) => e.value == (json['power_state'] ?? 0),
        orElse: () => PowerState.unknown,
      ),
      bandwidth: json['bandwidth'] ?? 0,
      latency: json['latency'] ?? 0,
      packetLoss: (json['packet_loss'] as num?)?.toDouble() ?? 0.0,
    );
  }

  @override
  String toString() =>
      'Candidate($peerId, score: $score, device: ${deviceType.name})';
}

/// 选举结果
class ElectionResult {
  final String proxyId;
  final double score;
  final String reason;

  ElectionResult({
    required this.proxyId,
    required this.score,
    this.reason = '',
  });

  factory ElectionResult.fromJson(Map<String, dynamic> json) {
    return ElectionResult(
      proxyId: json['proxy_id'] ?? '',
      score: (json['score'] as num?)?.toDouble() ?? 0.0,
      reason: json['reason'] ?? '',
    );
  }
}

/// Election - 动态选举系统
///
/// 独立的选举 API，用于需要自定义选举逻辑的场景。
/// 如果使用 [Coordinator]，选举已自动集成，无需单独调用。
///
/// ```dart
/// final election = Election(roomId: 'room-1');
///
/// // 启用选举
/// election.enable();
///
/// // 更新设备信息
/// election.updateDeviceInfo(
///   peerId: 'peer-1',
///   deviceType: DeviceType.pc,
///   connectionType: ConnectionType.ethernet,
///   powerState: PowerState.pluggedIn,
/// );
///
/// // 手动触发选举
/// final result = election.trigger();
/// print('新代理: ${result?.proxyId}');
/// ```
class Election {
  final String roomId;
  final int relayId;

  Election({required this.roomId, this.relayId = 0});

  /// 启用选举
  bool enable() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.ElectionEnable(relayId, roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 禁用选举
  bool disable() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.ElectionDisable(relayId, roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 更新设备信息
  ///
  /// 影响选举分数计算
  bool updateDeviceInfo({
    required String peerId,
    required DeviceType deviceType,
    required ConnectionType connectionType,
    required PowerState powerState,
  }) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.ElectionUpdateDeviceInfo(
            relayId,
            roomPtr,
            peerPtr,
            deviceType.value,
            connectionType.value,
            powerState.value,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 更新网络指标
  ///
  /// [bandwidth] 带宽 (bps)
  /// [latency] 延迟 (ms)
  /// [packetLoss] 丢包率 (0.0-1.0)
  bool updateNetworkMetrics({
    required String peerId,
    required int bandwidth,
    required int latency,
    required double packetLoss,
  }) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.ElectionUpdateNetworkMetrics(
            relayId,
            roomPtr,
            peerPtr,
            bandwidth,
            latency,
            packetLoss,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 更新候选者 (旧 API 兼容)
  bool updateCandidate({
    required String peerId,
    required int bandwidth,
    required int latency,
    required double packetLoss,
  }) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.ElectionUpdateCandidate(
            relayId,
            roomPtr,
            peerPtr,
            bandwidth,
            latency,
            packetLoss,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 手动触发选举
  ///
  /// 返回选举结果，包含新代理 ID、分数和原因
  ElectionResult? trigger() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.ElectionTrigger(relayId, roomPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return null;
      return ElectionResult.fromJson(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 获取当前代理
  String? getProxy() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.ElectionGetProxy(relayId, roomPtr);
      if (resultPtr == nullptr) return null;
      return fromCString(resultPtr);
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 获取所有候选者列表
  List<CandidateInfo> getCandidates() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.ElectionGetCandidates(relayId, roomPtr);
      if (resultPtr == nullptr) return [];
      final json = fromCString(resultPtr);
      if (json.isEmpty) return [];
      final list = jsonDecode(json) as List;
      return list.map((e) => CandidateInfo.fromJson(e)).toList();
    } finally {
      calloc.free(roomPtr);
    }
  }
}
