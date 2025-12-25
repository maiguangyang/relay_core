/// 日志处理器
///
/// 接收 Go 层的日志回调并转发到 Flutter 日志系统
library;

import 'dart:async';
import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../../flutter_sfu_relay_bindings_generated.dart';

/// 日志级别
enum LogLevel {
  debug(0),
  info(1),
  warn(2),
  error(3);

  const LogLevel(this.value);
  final int value;

  static LogLevel fromInt(int v) => LogLevel.values.firstWhere(
    (e) => e.value == v,
    orElse: () => LogLevel.info,
  );
}

/// 日志条目
class LogEntry {
  final LogLevel level;
  final String message;
  final DateTime timestamp;

  LogEntry({required this.level, required this.message, DateTime? timestamp})
    : timestamp = timestamp ?? DateTime.now();

  @override
  String toString() => '[${level.name.toUpperCase()}] $message';
}

/// 日志处理器
class LogHandler {
  static final StreamController<LogEntry> _controller =
      StreamController<LogEntry>.broadcast();

  // static NativeCallable<LogCallbackFunction>? _nativeCallback;
  static bool _initialized = false;

  /// 日志流
  static Stream<LogEntry> get logs => _controller.stream;

  /// 初始化日志处理器
  /// 初始化日志处理器
  static void init() {
    if (_initialized) return;

    // 使用 Pointer.fromFunction 创建同步回调
    final callback = Pointer.fromFunction<LogCallbackFunction>(_onLog);

    bindings.SetLogCallback(callback);
    _initialized = true;
  }

  /// 原生回调处理函数
  static void _onLog(int level, Pointer<Char> messagePtr) {
    if (messagePtr == nullptr) return;

    String message;
    try {
      message = messagePtr.cast<Utf8>().toDartString();
    } catch (_) {
      // Fallback for non-utf8
      message = '<invalid utf8>';
    }

    // Do NOT free memory here if Go frees it.
    // If Go transfers ownership, we should free.
    // Given the crash, assume Go frees it or we have double free.
    // bindings.FreeString(messagePtr);

    final entry = LogEntry(level: LogLevel.fromInt(level), message: message);
    _controller.add(entry);

    // ignore: avoid_print
    print('[SfuRelay] ${entry.toString()}');
  }

  /// 释放资源
  static void dispose() {
    // _nativeCallback?.close();
    // _nativeCallback = null;
    _initialized = false;
  }

  /// 是否已初始化
  static bool get isInitialized => _initialized;
}
