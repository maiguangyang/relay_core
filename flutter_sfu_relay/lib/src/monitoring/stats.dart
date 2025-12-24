/// 流量统计
library;

import 'dart:convert';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 流量统计
class Stats {
  final String roomId;

  Stats({required this.roomId});

  /// 创建统计
  bool create() {
    final roomPtr = toCString(roomId);
    final result = bindings.StatsCreate(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 销毁统计
  bool destroy() {
    final roomPtr = toCString(roomId);
    final result = bindings.StatsDestroy(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 记录入站字节 (需要指定 peerId)
  bool addBytesIn(String peerId, int bytes) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.StatsAddBytesIn(roomPtr, peerPtr, bytes);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 记录出站字节 (需要指定 peerId)
  bool addBytesOut(String peerId, int bytes) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.StatsAddBytesOut(roomPtr, peerPtr, bytes);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 获取快照 (JSON)
  Map<String, dynamic> getSnapshot() {
    final roomPtr = toCString(roomId);
    final json = fromCString(bindings.StatsGetSnapshot(roomPtr));
    calloc.free(roomPtr);
    if (json.isEmpty) return {};
    return jsonDecode(json) as Map<String, dynamic>;
  }
}
