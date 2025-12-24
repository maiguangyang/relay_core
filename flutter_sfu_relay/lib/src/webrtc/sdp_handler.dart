/// SDP/ICE 处理器
///
/// 处理 WebRTC 信令 (SDP Offer/Answer, ICE Candidate)
library;

import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../signaling/signaling.dart';
import 'webrtc_manager.dart';

/// SDP/ICE 处理器
///
/// 将 WebRTC 连接与信令层桥接
class SdpHandler {
  final WebRTCManager webrtcManager;
  final SignalingBridge signaling;
  final String roomId;
  final String localPeerId;

  StreamSubscription<SignalingMessage>? _signalingSubscription;
  final Map<String, StreamSubscription<RTCIceCandidate>> _iceSubscriptions = {};

  SdpHandler({
    required this.webrtcManager,
    required this.signaling,
    required this.roomId,
    required this.localPeerId,
  });

  /// 启动处理器
  void start() {
    _signalingSubscription = signaling.messages.listen(_handleSignalingMessage);
  }

  /// 与指定 Peer 建立连接
  Future<void> connectToPeer(String peerId) async {
    // 创建连接（作为发起者）
    final connection = await webrtcManager.createConnection(
      peerId,
      isInitiator: true,
    );

    // 监听 ICE 候选
    _subscribeToIceCandidates(peerId, connection);

    // 创建并发送 Offer
    final offer = await connection.createOffer();
    await signaling.sendOffer(roomId, peerId, offer.sdp!);
  }

  /// 断开与指定 Peer 的连接
  Future<void> disconnectFromPeer(String peerId) async {
    await _iceSubscriptions[peerId]?.cancel();
    _iceSubscriptions.remove(peerId);
    await webrtcManager.removeConnection(peerId);
  }

  /// 停止处理器
  void stop() {
    _signalingSubscription?.cancel();
    for (final sub in _iceSubscriptions.values) {
      sub.cancel();
    }
    _iceSubscriptions.clear();
  }

  /// 释放资源
  void dispose() {
    stop();
  }

  void _subscribeToIceCandidates(String peerId, WebRTCConnection connection) {
    _iceSubscriptions[peerId]?.cancel();
    _iceSubscriptions[peerId] = connection.onIceCandidate.listen((candidate) {
      signaling.sendCandidate(roomId, peerId, candidate.toMap().toString());
    });
  }

  Future<void> _handleSignalingMessage(SignalingMessage message) async {
    // 忽略自己的消息
    if (message.peerId == localPeerId) return;
    // 忽略不是发给自己的消息
    if (message.targetPeerId != null && message.targetPeerId != localPeerId)
      return;

    switch (message.type) {
      case SignalingMessageType.offer:
        await _handleOffer(message);
        break;
      case SignalingMessageType.answer:
        await _handleAnswer(message);
        break;
      case SignalingMessageType.candidate:
        await _handleCandidate(message);
        break;
      default:
        break;
    }
  }

  Future<void> _handleOffer(SignalingMessage message) async {
    final sdp = message.data?['sdp'] as String?;
    if (sdp == null) return;

    // 创建连接（作为接收者）
    final connection = await webrtcManager.createConnection(
      message.peerId,
      isInitiator: false,
    );

    // 监听 ICE 候选
    _subscribeToIceCandidates(message.peerId, connection);

    // 设置远程描述并创建 Answer
    await connection.setRemoteDescription(RTCSessionDescription(sdp, 'offer'));
    final answer = await connection.createAnswer();
    await signaling.sendAnswer(roomId, message.peerId, answer.sdp!);
  }

  Future<void> _handleAnswer(SignalingMessage message) async {
    final sdp = message.data?['sdp'] as String?;
    if (sdp == null) return;

    final connection = webrtcManager.getConnection(message.peerId);
    if (connection == null) return;

    await connection.setRemoteDescription(RTCSessionDescription(sdp, 'answer'));
  }

  Future<void> _handleCandidate(SignalingMessage message) async {
    final candidateStr = message.data?['candidate'] as String?;
    if (candidateStr == null) return;

    final connection = webrtcManager.getConnection(message.peerId);
    if (connection == null) return;

    // 解析 ICE 候选 (简化处理，实际需要更严格的解析)
    try {
      // 假设格式为 {candidate: ..., sdpMid: ..., sdpMLineIndex: ...}
      final candidate = RTCIceCandidate(
        candidateStr,
        'audio', // sdpMid
        0, // sdpMLineIndex
      );
      await connection.addIceCandidate(candidate);
    } catch (e) {
      // 解析失败，忽略
    }
  }
}
