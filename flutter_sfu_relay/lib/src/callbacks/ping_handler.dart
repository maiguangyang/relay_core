/// Ping 回调处理器
///
/// 接收 Go 层的 Ping 回调，用于心跳检测
library;

import 'dart:async';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../../flutter_sfu_relay_bindings_generated.dart';

/// Ping 请求
class PingRequest {
  final String peerId;
  final DateTime timestamp;

  PingRequest({required this.peerId, DateTime? timestamp})
    : timestamp = timestamp ?? DateTime.now();
}

/// Ping 回调处理器
///
/// 当 Go 层需要发送 Ping 时，会触发此回调。
/// 应用层需要监听此回调并通过信令发送 Ping 消息。
///
/// 使用 NativeCallable.listener 注册 Go 层回调
/// NativeCallable.listener 是线程安全的，可以从任意线程调用
class PingHandler {
  static final StreamController<PingRequest> _controller =
      StreamController<PingRequest>.broadcast();

  static NativeCallable<PingCallbackFunction>? _nativeCallback;
  static bool _initialized = false;

  /// Ping 请求流
  ///
  /// 应用层应监听此流，并将 Ping 请求发送给对应的 Peer
  static Stream<PingRequest> get pingRequests => _controller.stream;

  /// 初始化 Ping 处理器
  static void init() {
    if (_initialized) return;

    // 使用 NativeCallable.listener 创建线程安全的异步回调
    // 这样可以从任意 Go goroutine 安全调用
    _nativeCallback = NativeCallable<PingCallbackFunction>.listener(_onPing);

    bindings.SetPingCallback(_nativeCallback!.nativeFunction);
    _initialized = true;
  }

  /// 原生回调处理函数
  static void _onPing(Pointer<Char> peerIdPtr) {
    // 转换 C 字符串为 Dart 字符串
    final peerId = peerIdPtr == nullptr
        ? ''
        : peerIdPtr.cast<Utf8>().toDartString();

    // 释放 Go 层分配的内存
    // 因为使用 NativeCallable.listener (async)，Go 转移了内存所有权给 Dart
    if (peerIdPtr != nullptr) bindings.FreeString(peerIdPtr);

    final request = PingRequest(peerId: peerId);
    _controller.add(request);
  }

  /// 释放资源
  static void dispose() {
    _nativeCallback?.close();
    _nativeCallback = null;
    _initialized = false;
  }

  /// 是否已初始化
  static bool get isInitialized => _initialized;
}
