/// JitterBuffer - 抖动缓冲控制
///
/// 可选的抖动缓冲层，用于平滑网络抖动
library;

import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 抖动缓冲统计
class JitterBufferStats {
  final int jitterMs;
  final int bufferSize;
  final int packetsReceived;
  final int packetsDropped;

  JitterBufferStats({
    required this.jitterMs,
    required this.bufferSize,
    required this.packetsReceived,
    required this.packetsDropped,
  });

  factory JitterBufferStats.fromJson(Map<String, dynamic> json) {
    return JitterBufferStats(
      jitterMs: json['jitter_ms'] ?? 0,
      bufferSize: json['buffer_size'] ?? 0,
      packetsReceived: json['packets_received'] ?? 0,
      packetsDropped: json['packets_dropped'] ?? 0,
    );
  }

  @override
  String toString() => 'JitterBuffer(jitter: ${jitterMs}ms, size: $bufferSize)';
}

/// JitterBuffer - 抖动缓冲管理器
///
/// 提供可选的抖动缓冲功能，用于处理网络抖动导致的包乱序问题
///
/// ```dart
/// final jitter = JitterBuffer('room-1-video');
///
/// // 创建并启用
/// jitter.create(enabled: true, targetDelayMs: 50);
///
/// // 动态调整延迟
/// jitter.setDelay(100);
///
/// // 获取统计
/// final stats = jitter.getStats();
/// print('抖动: ${stats?.jitterMs}ms');
/// ```
class JitterBuffer {
  final String key;

  JitterBuffer(this.key);

  /// 创建抖动缓冲
  ///
  /// [enabled] 初始是否启用
  /// [targetDelayMs] 目标延迟（毫秒）
  bool create({bool enabled = true, int targetDelayMs = 50}) {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferCreate(
            keyPtr,
            enabled ? 1 : 0,
            targetDelayMs,
          ) ==
          0;
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 销毁抖动缓冲
  bool destroy() {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferDestroy(keyPtr) == 0;
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 启用/禁用抖动缓冲
  bool setEnabled(bool enabled) {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferEnable(keyPtr, enabled ? 1 : 0) == 0;
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 设置目标延迟
  bool setDelay(int delayMs) {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferSetDelay(keyPtr, delayMs) == 0;
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 清空缓冲区
  bool flush() {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferFlush(keyPtr) == 0;
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 获取统计信息
  JitterBufferStats? getStats() {
    final keyPtr = toCString(key);
    try {
      final resultPtr = bindings.JitterBufferGetStats(keyPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return null;
      return JitterBufferStats.fromJson(jsonDecode(json));
    } finally {
      calloc.free(keyPtr);
    }
  }

  /// 检查是否启用
  bool get isEnabled {
    final keyPtr = toCString(key);
    try {
      return bindings.JitterBufferIsEnabled(keyPtr) == 1;
    } finally {
      calloc.free(keyPtr);
    }
  }
}
