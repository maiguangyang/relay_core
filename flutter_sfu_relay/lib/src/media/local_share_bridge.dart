/// LocalShareBridge - 本地分享 UDP 桥接器
///
/// 使用 UDP 发送 RTP 包到 Go 层，避免 FFI 开销
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// LocalShareBridge 状态
enum LocalShareBridgeState {
  /// 未初始化
  idle,

  /// 正在启动
  starting,

  /// 运行中
  running,

  /// 正在停止
  stopping,

  /// 已停止
  stopped,

  /// 失败
  failed,
}

/// LocalShareBridge - 零 FFI 本地分享桥接器
///
/// 通过 UDP 发送 RTP 包到 Go 层的 LocalShareBridge，
/// 避免每个包都经过 FFI 边界的拷贝开销。
///
/// ```dart
/// final bridge = LocalShareBridge(roomId: 'room-1');
/// final port = await bridge.start();
/// print('Bridge listening on port $port');
///
/// // 发送 RTP 包
/// bridge.sendRtpPacket(rtpData);
///
/// // 完成后停止
/// await bridge.stop();
/// ```
class LocalShareBridge {
  final String roomId;

  RawDatagramSocket? _socket;
  InternetAddress? _targetAddress;
  int? _targetPort;
  LocalShareBridgeState _state = LocalShareBridgeState.idle;

  // 统计
  int _packetsSent = 0;
  int _bytesSent = 0;

  LocalShareBridge({required this.roomId});

  /// 当前状态
  LocalShareBridgeState get state => _state;

  /// 是否正在运行
  bool get isRunning => _state == LocalShareBridgeState.running;

  /// 发送的包数量
  int get packetsSent => _packetsSent;

  /// 发送的字节数
  int get bytesSent => _bytesSent;

  /// 启动桥接器
  ///
  /// [preferredPort] 首选端口，0 表示让 Go 层自动分配
  /// 返回 Go 层实际监听的端口
  Future<int> start({int preferredPort = 0}) async {
    if (_state == LocalShareBridgeState.running) {
      return _targetPort!;
    }

    _state = LocalShareBridgeState.starting;

    try {
      // 1. 创建 Go 层桥接器
      final roomPtr = toCString(roomId);
      int result = bindings.LocalShareBridgeCreate(roomPtr);
      calloc.free(roomPtr);

      if (result != 0) {
        _state = LocalShareBridgeState.failed;
        throw Exception('Failed to create LocalShareBridge');
      }

      // 2. 启动 Go 层 UDP 监听
      final roomPtr2 = toCString(roomId);
      final port = bindings.LocalShareBridgeStart(roomPtr2, preferredPort);
      calloc.free(roomPtr2);

      if (port < 0) {
        _state = LocalShareBridgeState.failed;
        throw Exception('Failed to start LocalShareBridge');
      }

      _targetPort = port;
      _targetAddress = InternetAddress.loopbackIPv4;

      // 3. 创建本地 UDP 发送 socket
      _socket = await RawDatagramSocket.bind(InternetAddress.anyIPv4, 0);

      _state = LocalShareBridgeState.running;
      _packetsSent = 0;
      _bytesSent = 0;

      return port;
    } catch (e) {
      _state = LocalShareBridgeState.failed;
      rethrow;
    }
  }

  /// 发送 RTP 包
  ///
  /// 零拷贝方式发送到 Go 层
  bool sendRtpPacket(Uint8List data) {
    if (_socket == null || _targetAddress == null || _targetPort == null) {
      return false;
    }

    final sent = _socket!.send(data, _targetAddress!, _targetPort!);
    if (sent > 0) {
      _packetsSent++;
      _bytesSent += data.length;
      return true;
    }
    return false;
  }

  /// 停止桥接器
  Future<void> stop() async {
    if (_state != LocalShareBridgeState.running) {
      return;
    }

    _state = LocalShareBridgeState.stopping;

    // 关闭本地 socket
    _socket?.close();
    _socket = null;

    // 停止 Go 层桥接器
    final roomPtr = toCString(roomId);
    bindings.LocalShareBridgeStop(roomPtr);
    calloc.free(roomPtr);

    _state = LocalShareBridgeState.stopped;
  }

  /// 销毁桥接器
  Future<void> destroy() async {
    await stop();

    final roomPtr = toCString(roomId);
    bindings.LocalShareBridgeDestroy(roomPtr);
    calloc.free(roomPtr);

    _state = LocalShareBridgeState.idle;
  }

  /// 获取 Go 层状态
  Map<String, dynamic> getStatus() {
    final roomPtr = toCString(roomId);
    final jsonStr = fromCString(bindings.LocalShareBridgeGetStatus(roomPtr));
    calloc.free(roomPtr);

    if (jsonStr.isEmpty) return {};
    return jsonDecode(jsonStr) as Map<String, dynamic>;
  }
}
