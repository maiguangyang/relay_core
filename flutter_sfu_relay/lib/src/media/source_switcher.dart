/// 源切换器
library;

import 'dart:convert';
import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 源切换器
///
/// 管理 SFU 源和本地分享源之间的切换
class SourceSwitcher {
  final String roomId;

  SourceSwitcher({required this.roomId});

  /// 创建源切换器
  bool create() {
    final roomPtr = toCString(roomId);
    final result = bindings.SourceSwitcherCreate(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 销毁源切换器
  bool destroy() {
    final roomPtr = toCString(roomId);
    final result = bindings.SourceSwitcherDestroy(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 注入 SFU RTP 包
  bool injectSfuPacket(bool isVideo, Uint8List data) {
    final roomPtr = toCString(roomId);
    final dataPtr = calloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);
    final result = bindings.SourceSwitcherInjectSFU(
      roomPtr,
      isVideo ? 1 : 0,
      dataPtr.cast(),
      data.length,
    );
    calloc.free(roomPtr);
    calloc.free(dataPtr);
    return result == 0;
  }

  /// 注入本地 RTP 包
  bool injectLocalPacket(bool isVideo, Uint8List data) {
    final roomPtr = toCString(roomId);
    final dataPtr = calloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);
    final result = bindings.SourceSwitcherInjectLocal(
      roomPtr,
      isVideo ? 1 : 0,
      dataPtr.cast(),
      data.length,
    );
    calloc.free(roomPtr);
    calloc.free(dataPtr);
    return result == 0;
  }

  /// 开始本地分享
  bool startLocalShare(String sharerId) {
    final roomPtr = toCString(roomId);
    final sharerPtr = toCString(sharerId);
    final result = bindings.SourceSwitcherStartLocalShare(roomPtr, sharerPtr);
    calloc.free(roomPtr);
    calloc.free(sharerPtr);
    return result == 0;
  }

  /// 停止本地分享
  bool stopLocalShare() {
    final roomPtr = toCString(roomId);
    final result = bindings.SourceSwitcherStopLocalShare(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 是否正在本地分享
  bool get isLocalSharing {
    final roomPtr = toCString(roomId);
    final result = bindings.SourceSwitcherIsLocalSharing(roomPtr);
    calloc.free(roomPtr);
    return result == 1;
  }

  /// 获取状态 (JSON)
  Map<String, dynamic> getStatus() {
    final roomPtr = toCString(roomId);
    final json = fromCString(bindings.SourceSwitcherGetStatus(roomPtr));
    calloc.free(roomPtr);
    if (json.isEmpty) return {};
    return jsonDecode(json) as Map<String, dynamic>;
  }
}
