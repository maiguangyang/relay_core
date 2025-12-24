/// 代理模式协调器
library;

import 'dart:convert';
import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../enums.dart';

/// 代理模式协调器
///
/// 一键启用，全自动管理 Relay 选举和故障切换
class Coordinator {
  final String roomId;
  final String localPeerId;

  Coordinator({required this.roomId, required this.localPeerId});

  /// 启用协调器
  bool enable() {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(localPeerId);
    final result = bindings.CoordinatorEnable(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 禁用协调器
  bool disable() {
    final roomPtr = toCString(roomId);
    final result = bindings.CoordinatorDisable(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 添加 Peer
  bool addPeer(
    String peerId, {
    DeviceType deviceType = DeviceType.unknown,
    ConnectionType connectionType = ConnectionType.unknown,
    PowerState powerState = PowerState.unknown,
  }) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.CoordinatorAddPeer(
      roomPtr,
      peerPtr,
      deviceType.value,
      connectionType.value,
      powerState.value,
    );
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 移除 Peer
  bool removePeer(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.CoordinatorRemovePeer(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 处理 Pong 响应
  bool handlePong(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.CoordinatorHandlePong(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 设置当前 Relay
  bool setRelay(String relayId, int epoch) {
    final roomPtr = toCString(roomId);
    final relayPtr = toCString(relayId);
    final result = bindings.CoordinatorSetRelay(roomPtr, relayPtr, epoch);
    calloc.free(roomPtr);
    calloc.free(relayPtr);
    return result == 0;
  }

  /// 接收 Relay 声明
  bool receiveClaim(String peerId, int epoch, double score) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.CoordinatorReceiveClaim(
      roomPtr,
      peerPtr,
      epoch,
      score,
    );
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 开始本地分享
  bool startLocalShare(String sharerId) {
    final roomPtr = toCString(roomId);
    final sharerPtr = toCString(sharerId);
    final result = bindings.CoordinatorStartLocalShare(roomPtr, sharerPtr);
    calloc.free(roomPtr);
    calloc.free(sharerPtr);
    return result == 0;
  }

  /// 停止本地分享
  bool stopLocalShare() {
    final roomPtr = toCString(roomId);
    final result = bindings.CoordinatorStopLocalShare(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 注入 SFU RTP 包
  bool injectSfuPacket(bool isVideo, Uint8List data) {
    final roomPtr = toCString(roomId);
    final dataPtr = calloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);
    final result = bindings.CoordinatorInjectSFU(
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
    final result = bindings.CoordinatorInjectLocal(
      roomPtr,
      isVideo ? 1 : 0,
      dataPtr.cast(),
      data.length,
    );
    calloc.free(roomPtr);
    calloc.free(dataPtr);
    return result == 0;
  }

  /// 获取状态 (JSON)
  Map<String, dynamic> getStatus() {
    final roomPtr = toCString(roomId);
    final json = fromCString(bindings.CoordinatorGetStatus(roomPtr));
    calloc.free(roomPtr);
    if (json.isEmpty) return {};
    return jsonDecode(json) as Map<String, dynamic>;
  }

  /// 是否是 Relay
  bool get isRelay {
    final roomPtr = toCString(roomId);
    final result = bindings.CoordinatorIsRelay(roomPtr);
    calloc.free(roomPtr);
    return result == 1;
  }
}
