/// 事件处理器
///
/// 使用 NativeCallable 接收 Go 层的事件回调
library;

import 'dart:async';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../../flutter_sfu_relay_bindings_generated.dart';
import '../enums.dart';

/// SFU 事件
class SfuEvent {
  final SfuEventType type;
  final String roomId;
  final String peerId;
  final String? data;
  final DateTime timestamp;

  SfuEvent({
    required this.type,
    required this.roomId,
    required this.peerId,
    this.data,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  @override
  String toString() =>
      'SfuEvent(type: $type, roomId: $roomId, peerId: $peerId)';
}

/// 事件处理器
///
/// 使用 NativeCallable.listener 注册 Go 层回调
class EventHandler {
  static final StreamController<SfuEvent> _controller =
      StreamController<SfuEvent>.broadcast();

  // static NativeCallable<EventCallbackFunction>? _nativeCallback;
  static bool _initialized = false;

  /// 事件流
  static Stream<SfuEvent> get events => _controller.stream;

  /// 初始化事件处理器
  ///
  /// 必须在使用其他功能前调用
  static void init() {
    if (_initialized) return;

    // 使用 Pointer.fromFunction 创建同步回调
    // 这样可以在 Go 函数返回前处理数据，避免 UAF 问题
    final callback = Pointer.fromFunction<EventCallbackFunction>(_onEvent);

    // 注册到 Go 层
    bindings.SetEventCallback(callback);
    _initialized = true;
  }

  /// 原生回调处理函数
  static void _onEvent(
    int eventType,
    Pointer<Char> roomIdPtr,
    Pointer<Char> peerIdPtr,
    Pointer<Char> dataPtr,
  ) {
    // 转换 C 字符串为 Dart 字符串
    final roomId = roomIdPtr == nullptr
        ? ''
        : roomIdPtr.cast<Utf8>().toDartString();
    final peerId = peerIdPtr == nullptr
        ? ''
        : peerIdPtr.cast<Utf8>().toDartString();
    final data = dataPtr == nullptr
        ? null
        : dataPtr.cast<Utf8>().toDartString();

    final event = SfuEvent(
      type: SfuEventType.fromInt(eventType),
      roomId: roomId,
      peerId: peerId,
      data: data,
    );

    _controller.add(event);
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
