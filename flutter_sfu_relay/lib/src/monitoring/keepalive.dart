/// 心跳管理器
library;

import 'dart:convert';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';
import '../enums.dart';

/// 心跳管理器
class Keepalive {
  final String roomId;

  Keepalive({required this.roomId});

  /// 创建心跳管理器
  bool create({int intervalMs = 1000, int timeoutMs = 3000}) {
    final roomPtr = toCString(roomId);
    final result = bindings.KeepaliveCreate(roomPtr, intervalMs, timeoutMs);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 销毁心跳管理器
  bool destroy() {
    final roomPtr = toCString(roomId);
    final result = bindings.KeepaliveDestroy(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 启动心跳检测
  bool start() {
    final roomPtr = toCString(roomId);
    final result = bindings.KeepaliveStart(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 停止心跳检测
  bool stop() {
    final roomPtr = toCString(roomId);
    final result = bindings.KeepaliveStop(roomPtr);
    calloc.free(roomPtr);
    return result == 0;
  }

  /// 添加 Peer
  bool addPeer(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.KeepaliveAddPeer(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 移除 Peer
  bool removePeer(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.KeepaliveRemovePeer(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 处理 Pong
  bool handlePong(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.KeepaliveHandlePong(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result == 0;
  }

  /// 获取 Peer 状态
  PeerStatus getPeerStatus(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.KeepaliveGetPeerStatus(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return PeerStatus.fromInt(result);
  }

  /// 获取 Peer RTT (毫秒)
  int getPeerRtt(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final result = bindings.KeepaliveGetPeerRTT(roomPtr, peerPtr);
    calloc.free(roomPtr);
    calloc.free(peerPtr);
    return result;
  }

  /// 获取所有 Peer 信息 (JSON)
  Map<String, dynamic> getAllPeerInfo() {
    final roomPtr = toCString(roomId);
    final json = fromCString(bindings.KeepaliveGetAllPeerInfo(roomPtr));
    calloc.free(roomPtr);
    if (json.isEmpty) return {};
    return jsonDecode(json) as Map<String, dynamic>;
  }
}
