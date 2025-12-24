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

    _nativeCallback = NativeCallable<PingCallbackFunction>.listener(_onPing);

    bindings.SetPingCallback(_nativeCallback!.nativeFunction);
    _initialized = true;
  }

  /// 原生回调处理函数
  static void _onPing(Pointer<Char> peerIdPtr) {
    final peerId = peerIdPtr == nullptr
        ? ''
        : peerIdPtr.cast<Utf8>().toDartString();

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
