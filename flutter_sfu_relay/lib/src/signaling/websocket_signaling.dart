/// WebSocket 信令实现
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'signaling_bridge.dart';

/// WebSocket 信令实现
///
/// 通过 WebSocket 连接到信令服务器
class WebSocketSignaling implements SignalingBridge {
  final String url;
  final String localPeerId;
  final Duration reconnectDelay;
  final int maxReconnectAttempts;

  WebSocket? _socket;
  bool _isConnected = false;
  bool _shouldReconnect = true;
  int _reconnectAttempts = 0;
  String? _currentRoomId;

  final StreamController<SignalingMessage> _messageController =
      StreamController<SignalingMessage>.broadcast();

  WebSocketSignaling({
    required this.url,
    required this.localPeerId,
    this.reconnectDelay = const Duration(seconds: 2),
    this.maxReconnectAttempts = 5,
  });

  @override
  bool get isConnected => _isConnected;

  @override
  Stream<SignalingMessage> get messages => _messageController.stream;

  // WebSocket 信令不直接检测 Peer 断开，依赖心跳或其他机制
  final _peerDisconnectedController = StreamController<String>.broadcast();
  @override
  Stream<String> get peerDisconnected => _peerDisconnectedController.stream;

  // WebSocket 信令不直接检测 Peer 连接
  final _peerConnectedController = StreamController<String>.broadcast();
  @override
  Stream<String> get peerConnected => _peerConnectedController.stream;

  @override
  Future<void> connect() async {
    if (_isConnected) return;

    try {
      _socket = await WebSocket.connect(url);
      _isConnected = true;
      _reconnectAttempts = 0;

      _socket!.listen(
        _onMessage,
        onError: _onError,
        onDone: _onDone,
        cancelOnError: false,
      );
    } catch (e) {
      _isConnected = false;
      _scheduleReconnect();
      rethrow;
    }
  }

  @override
  Future<void> disconnect() async {
    _shouldReconnect = false;
    _isConnected = false;
    await _socket?.close();
    _socket = null;
  }

  @override
  Future<void> joinRoom(String roomId, String peerId) async {
    _currentRoomId = roomId;
    await _send(
      SignalingMessage(
        type: SignalingMessageType.join,
        roomId: roomId,
        peerId: peerId,
      ),
    );
  }

  @override
  Future<void> leaveRoom(String roomId) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.leave,
        roomId: roomId,
        peerId: localPeerId,
      ),
    );
    _currentRoomId = null;
  }

  @override
  Future<void> sendOffer(String roomId, String targetPeerId, String sdp) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.offer,
        roomId: roomId,
        peerId: localPeerId,
        targetPeerId: targetPeerId,
        data: {'sdp': sdp},
      ),
    );
  }

  @override
  Future<void> sendAnswer(
    String roomId,
    String targetPeerId,
    String sdp,
  ) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.answer,
        roomId: roomId,
        peerId: localPeerId,
        targetPeerId: targetPeerId,
        data: {'sdp': sdp},
      ),
    );
  }

  @override
  Future<void> sendCandidate(
    String roomId,
    String targetPeerId,
    String candidate,
  ) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.candidate,
        roomId: roomId,
        peerId: localPeerId,
        targetPeerId: targetPeerId,
        data: {'candidate': candidate},
      ),
    );
  }

  @override
  Future<void> sendPing(String roomId, String targetPeerId) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.ping,
        roomId: roomId,
        peerId: localPeerId,
        targetPeerId: targetPeerId,
      ),
    );
  }

  @override
  Future<void> sendPong(String roomId, String targetPeerId) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.pong,
        roomId: roomId,
        peerId: localPeerId,
        targetPeerId: targetPeerId,
      ),
    );
  }

  @override
  Future<void> sendRelayClaim(String roomId, int epoch, double score) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.relayClaim,
        roomId: roomId,
        peerId: localPeerId,
        data: {'epoch': epoch, 'score': score},
      ),
    );
  }

  @override
  Future<void> sendRelayChanged(
    String roomId,
    String relayId,
    int epoch,
    double score,
  ) async {
    await _send(
      SignalingMessage(
        type: SignalingMessageType.relayChanged,
        roomId: roomId,
        peerId: localPeerId,
        data: {'relayId': relayId, 'epoch': epoch, 'score': score},
      ),
    );
  }

  @override
  void dispose() {
    _shouldReconnect = false;
    _socket?.close();
    _messageController.close();
  }

  Future<void> _send(SignalingMessage message) async {
    if (!_isConnected || _socket == null) {
      throw StateError('WebSocket not connected');
    }
    _socket!.add(jsonEncode(message.toJson()));
  }

  void _onMessage(dynamic data) {
    try {
      final json = jsonDecode(data as String) as Map<String, dynamic>;
      final message = SignalingMessage.fromJson(json);
      _messageController.add(message);
    } catch (e) {
      // 解析失败，忽略无效消息
    }
  }

  void _onError(Object error) {
    _isConnected = false;
    _messageController.addError(error);
    _scheduleReconnect();
  }

  void _onDone() {
    _isConnected = false;
    _socket = null;
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    if (!_shouldReconnect) return;
    if (_reconnectAttempts >= maxReconnectAttempts) return;

    _reconnectAttempts++;
    Future.delayed(reconnectDelay * _reconnectAttempts, () async {
      if (!_shouldReconnect) return;
      try {
        await connect();
        // 重连成功后重新加入房间
        if (_currentRoomId != null) {
          await joinRoom(_currentRoomId!, localPeerId);
        }
      } catch (_) {
        // 重连失败，继续尝试
      }
    });
  }
}
