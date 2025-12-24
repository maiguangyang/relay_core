/// NetworkProbe - 网络探测
///
/// 实时网络质量探测，获取延迟、带宽、丢包率等指标
library;

import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 网络指标
class NetworkMetrics {
  final String peerId;
  final int rtt;
  final int bandwidth;
  final double packetLoss;
  final int jitter;

  NetworkMetrics({
    required this.peerId,
    required this.rtt,
    required this.bandwidth,
    required this.packetLoss,
    required this.jitter,
  });

  factory NetworkMetrics.fromJson(Map<String, dynamic> json) {
    return NetworkMetrics(
      peerId: json['peer_id'] ?? '',
      rtt: json['rtt'] ?? 0,
      bandwidth: json['bandwidth'] ?? 0,
      packetLoss: (json['packet_loss'] as num?)?.toDouble() ?? 0.0,
      jitter: json['jitter'] ?? 0,
    );
  }

  @override
  String toString() =>
      'NetworkMetrics($peerId, rtt: ${rtt}ms, loss: ${(packetLoss * 100).toStringAsFixed(1)}%)';
}

/// NetworkProbe - 网络探测器
///
/// 用于实时监测网络质量，提供延迟、带宽、丢包率等指标
///
/// ```dart
/// final probe = NetworkProbe('room-1');
///
/// // 创建探测器
/// probe.create();
///
/// // 获取指定 Peer 的指标
/// final metrics = probe.getMetrics('peer-1');
/// print('RTT: ${metrics?.rtt}ms');
///
/// // 获取所有 Peer 的指标
/// final allMetrics = probe.getAllMetrics();
/// ```
class NetworkProbe {
  final String roomId;

  NetworkProbe(this.roomId);

  /// 创建探测器
  bool create() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.NetworkProbeCreate(roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 销毁探测器
  bool destroy() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.NetworkProbeDestroy(roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 获取指定 Peer 的网络指标
  NetworkMetrics? getMetrics(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      final resultPtr = bindings.NetworkProbeGetMetrics(roomPtr, peerPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return null;
      return NetworkMetrics.fromJson(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 获取所有 Peer 的网络指标
  List<NetworkMetrics> getAllMetrics() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.NetworkProbeGetAllMetrics(roomPtr);
      if (resultPtr == nullptr) return [];
      final json = fromCString(resultPtr);
      if (json.isEmpty) return [];
      final list = jsonDecode(json) as List;
      return list.map((e) => NetworkMetrics.fromJson(e)).toList();
    } finally {
      calloc.free(roomPtr);
    }
  }
}
