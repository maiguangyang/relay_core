/// RelayRoom P2P 管理
///
/// Go RelayRoom 的完整 Dart 封装，管理 Relay 节点与订阅者之间的 P2P WebRTC 连接
library;

import 'dart:convert';
import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// RelayRoom P2P 管理器
///
/// 底层 P2P 连接管理，适用于需要完全控制的高级用户。
/// 如果只需自动代理模式，请使用 [Coordinator]。
///
/// ```dart
/// final relayRoom = RelayRoomP2P('room-1');
///
/// // 创建房间
/// relayRoom.create(iceServers: [
///   {'urls': ['stun:stun.l.google.com:19302']}
/// ]);
///
/// // 成为 Relay 节点
/// relayRoom.becomeRelay('my-peer-id');
///
/// // 添加订阅者（收到 Offer 后）
/// final answer = relayRoom.addSubscriber('subscriber-1', offerSdp);
///
/// // 注入 RTP 数据
/// relayRoom.injectSfu(isVideo: true, data: rtpPacket);
/// ```
class RelayRoomP2P {
  final String roomId;

  RelayRoomP2P(this.roomId);

  // ========== 房间生命周期 ==========

  /// 创建 Relay 房间
  ///
  /// [iceServers] ICE 服务器配置列表
  /// 返回 true 表示成功
  bool create({List<Map<String, dynamic>> iceServers = const []}) {
    final roomPtr = toCString(roomId);
    final iceJson = jsonEncode(iceServers);
    final icePtr = toCString(iceJson);

    try {
      return bindings.RelayRoomCreate(roomPtr, icePtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(icePtr);
    }
  }

  /// 销毁房间
  bool destroy() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.RelayRoomDestroy(roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 成为 Relay 节点
  ///
  /// 当选举系统选中本机为 Relay 时调用
  bool becomeRelay(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.RelayRoomBecomeRelay(roomPtr, peerPtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 检查是否是 Relay 节点
  ///
  /// 返回: true=是, false=否, null=错误
  bool? get isRelay {
    final roomPtr = toCString(roomId);
    try {
      final result = bindings.RelayRoomIsRelay(roomPtr);
      if (result == -1) return null;
      return result == 1;
    } finally {
      calloc.free(roomPtr);
    }
  }

  // ========== 订阅者管理 ==========

  /// 添加订阅者
  ///
  /// 收到远端 Offer 后，创建 P2P 连接并返回 Answer SDP
  /// [peerId] 订阅者 ID
  /// [offerSdp] 远端 Offer SDP
  /// 返回 Answer SDP，失败返回 null
  String? addSubscriber(String peerId, String offerSdp) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final offerPtr = toCString(offerSdp);

    try {
      final resultPtr = bindings.RelayRoomAddSubscriber(
        roomPtr,
        peerPtr,
        offerPtr,
      );
      if (resultPtr == nullptr) return null;
      return fromCString(resultPtr);
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
      calloc.free(offerPtr);
    }
  }

  /// 移除订阅者
  bool removeSubscriber(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      return bindings.RelayRoomRemoveSubscriber(roomPtr, peerPtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 获取订阅者列表
  List<String> getSubscribers() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.RelayRoomGetSubscribers(roomPtr);
      if (resultPtr == nullptr) return [];
      final json = fromCString(resultPtr);
      if (json.isEmpty) return [];
      return List<String>.from(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 获取订阅者数量
  int get subscriberCount {
    final roomPtr = toCString(roomId);
    try {
      return bindings.RelayRoomGetSubscriberCount(roomPtr);
    } finally {
      calloc.free(roomPtr);
    }
  }

  // ========== ICE 处理 ==========

  /// 添加 ICE 候选
  ///
  /// [peerId] 订阅者 ID
  /// [candidate] ICE 候选信息 (包含 candidate, sdpMid, sdpMLineIndex)
  bool addIceCandidate(String peerId, Map<String, dynamic> candidate) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final candidatePtr = toCString(jsonEncode(candidate));

    try {
      return bindings.RelayRoomAddICECandidate(
            roomPtr,
            peerPtr,
            candidatePtr,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
      calloc.free(candidatePtr);
    }
  }

  // ========== SDP 重协商 ==========

  /// 触发全员重协商
  ///
  /// 为所有已连接的订阅者生成新的 Offer
  /// 返回 Map<peerId, offerSdp>，失败返回 null
  Map<String, String>? triggerRenegotiation() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.RelayRoomTriggerRenegotiation(roomPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return {};
      return Map<String, String>.from(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 为指定订阅者创建 Offer
  String? createOffer(String peerId) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    try {
      final resultPtr = bindings.RelayRoomCreateOffer(roomPtr, peerPtr);
      if (resultPtr == nullptr) return null;
      return fromCString(resultPtr);
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
    }
  }

  /// 处理订阅者的 Answer（重协商响应）
  bool handleAnswer(String peerId, String answerSdp) {
    final roomPtr = toCString(roomId);
    final peerPtr = toCString(peerId);
    final answerPtr = toCString(answerSdp);
    try {
      return bindings.RelayRoomHandleAnswer(roomPtr, peerPtr, answerPtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(peerPtr);
      calloc.free(answerPtr);
    }
  }

  // ========== RTP 注入 ==========

  /// 注入 SFU RTP 包
  ///
  /// 从 LiveKit 或其他源接收到 RTP 包后，注入到 RelayRoom 转发给所有订阅者
  bool injectSfu({required bool isVideo, required Uint8List data}) {
    final roomPtr = toCString(roomId);
    final dataPtr = calloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);

    try {
      return bindings.RelayRoomInjectSFU(
            roomPtr,
            isVideo ? 1 : 0,
            dataPtr.cast(),
            data.length,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(dataPtr);
    }
  }

  /// 注入本地分享 RTP 包
  bool injectLocal({required bool isVideo, required Uint8List data}) {
    final roomPtr = toCString(roomId);
    final dataPtr = calloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);

    try {
      return bindings.RelayRoomInjectLocal(
            roomPtr,
            isVideo ? 1 : 0,
            dataPtr.cast(),
            data.length,
          ) ==
          0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(dataPtr);
    }
  }

  // ========== 本地分享 ==========

  /// 开始本地分享
  ///
  /// 切换到本地分享模式，后续使用 [injectLocal] 注入本地流
  bool startLocalShare(String sharerId) {
    final roomPtr = toCString(roomId);
    final sharerPtr = toCString(sharerId);
    try {
      return bindings.RelayRoomStartLocalShare(roomPtr, sharerPtr) == 0;
    } finally {
      calloc.free(roomPtr);
      calloc.free(sharerPtr);
    }
  }

  /// 停止本地分享
  ///
  /// 切回 SFU 模式
  bool stopLocalShare() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.RelayRoomStopLocalShare(roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 处理本地 Loopback P2P 连接 Offer
  ///
  /// 返回 Answer SDP
  String? handleLocalPublisherOffer(String offerSdp) {
    final roomPtr = toCString(roomId);
    final offerPtr = toCString(offerSdp);
    try {
      final resultPtr = bindings.RelayRoomHandleLocalPublisherOffer(
        roomPtr,
        offerPtr,
      );
      if (resultPtr == nullptr) return null;
      return fromCString(resultPtr);
    } finally {
      calloc.free(roomPtr);
      calloc.free(offerPtr);
    }
  }

  // ========== 状态查询 ==========

  /// 获取房间状态
  ///
  /// 返回包含 subscriber_count, is_relay, source_switcher 等信息的 Map
  Map<String, dynamic>? getStatus() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.RelayRoomGetStatus(roomPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return {};
      return Map<String, dynamic>.from(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }
}
