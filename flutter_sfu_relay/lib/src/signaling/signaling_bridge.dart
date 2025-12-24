/// 信令消息类型
library;

/// 信令消息类型
enum SignalingMessageType {
  /// SDP Offer
  offer,

  /// SDP Answer
  answer,

  /// ICE Candidate
  candidate,

  /// 加入房间
  join,

  /// 离开房间
  leave,

  /// Relay 声明
  relayClaim,

  /// Relay 变更通知
  relayChanged,

  /// Ping 心跳
  ping,

  /// Pong 响应
  pong,

  /// 错误
  error,
}

/// 信令消息
class SignalingMessage {
  final SignalingMessageType type;
  final String roomId;
  final String peerId;
  final String? targetPeerId;
  final Map<String, dynamic>? data;
  final DateTime timestamp;

  SignalingMessage({
    required this.type,
    required this.roomId,
    required this.peerId,
    this.targetPeerId,
    this.data,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  /// 从 JSON 解析
  factory SignalingMessage.fromJson(Map<String, dynamic> json) {
    return SignalingMessage(
      type: SignalingMessageType.values.firstWhere(
        (e) => e.name == json['type'],
        orElse: () => SignalingMessageType.error,
      ),
      roomId: json['roomId'] ?? '',
      peerId: json['peerId'] ?? '',
      targetPeerId: json['targetPeerId'],
      data: json['data'],
    );
  }

  /// 转换为 JSON
  Map<String, dynamic> toJson() => {
    'type': type.name,
    'roomId': roomId,
    'peerId': peerId,
    if (targetPeerId != null) 'targetPeerId': targetPeerId,
    if (data != null) 'data': data,
  };
}

/// 信令桥接接口
///
/// 抽象信令传输层，支持多种实现方式（WebSocket、HTTP、局域网等）
abstract class SignalingBridge {
  /// 连接到信令服务器
  Future<void> connect();

  /// 断开连接
  Future<void> disconnect();

  /// 是否已连接
  bool get isConnected;

  /// 加入房间
  Future<void> joinRoom(String roomId, String peerId);

  /// 离开房间
  Future<void> leaveRoom(String roomId);

  /// 发送 SDP Offer
  Future<void> sendOffer(String roomId, String targetPeerId, String sdp);

  /// 发送 SDP Answer
  Future<void> sendAnswer(String roomId, String targetPeerId, String sdp);

  /// 发送 ICE Candidate
  Future<void> sendCandidate(
    String roomId,
    String targetPeerId,
    String candidate,
  );

  /// 发送 Ping
  Future<void> sendPing(String roomId, String targetPeerId);

  /// 发送 Pong
  Future<void> sendPong(String roomId, String targetPeerId);

  /// 发送 Relay 声明
  Future<void> sendRelayClaim(String roomId, int epoch, double score);

  /// 发送 Relay 变更通知
  Future<void> sendRelayChanged(String roomId, String relayId, int epoch);

  /// 消息流
  Stream<SignalingMessage> get messages;

  /// 释放资源
  void dispose();
}
