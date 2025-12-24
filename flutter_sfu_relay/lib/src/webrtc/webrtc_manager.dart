/// WebRTC 管理器
///
/// 管理 PeerConnection 和 RTP 数据流
library;

import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_webrtc/flutter_webrtc.dart';

/// PeerConnection 配置
class PeerConnectionConfig {
  final List<String> iceServers;
  final bool enableAudio;
  final bool enableVideo;

  const PeerConnectionConfig({
    this.iceServers = const [],
    this.enableAudio = true,
    this.enableVideo = true,
  });

  Map<String, dynamic> toConfiguration() => {
    'iceServers': iceServers.isEmpty
        ? [
            {'urls': 'stun:stun.l.google.com:19302'},
          ]
        : iceServers.map((s) => {'urls': s}).toList(),
    'sdpSemantics': 'unified-plan',
  };
}

/// RTP 包
class RtpPacket {
  final bool isVideo;
  final Uint8List data;
  final DateTime timestamp;

  RtpPacket({required this.isVideo, required this.data, DateTime? timestamp})
    : timestamp = timestamp ?? DateTime.now();
}

/// WebRTC 连接管理器
///
/// 管理与单个 Peer 的 WebRTC 连接
class WebRTCConnection {
  final String peerId;
  final PeerConnectionConfig config;
  final bool isInitiator;

  RTCPeerConnection? _peerConnection;
  MediaStream? _localStream;

  final _iceCandidateController = StreamController<RTCIceCandidate>.broadcast();
  final _remoteStreamController = StreamController<MediaStream>.broadcast();
  final _connectionStateController =
      StreamController<RTCPeerConnectionState>.broadcast();

  WebRTCConnection({
    required this.peerId,
    this.config = const PeerConnectionConfig(),
    this.isInitiator = false,
  });

  /// ICE 候选流
  Stream<RTCIceCandidate> get onIceCandidate => _iceCandidateController.stream;

  /// 远程流
  Stream<MediaStream> get onRemoteStream => _remoteStreamController.stream;

  /// 连接状态流
  Stream<RTCPeerConnectionState> get onConnectionState =>
      _connectionStateController.stream;

  /// 是否已连接
  bool get isConnected =>
      _peerConnection?.connectionState ==
      RTCPeerConnectionState.RTCPeerConnectionStateConnected;

  /// 初始化连接
  Future<void> initialize() async {
    _peerConnection = await createPeerConnection(config.toConfiguration());

    _peerConnection!.onIceCandidate = (candidate) {
      _iceCandidateController.add(candidate);
    };

    _peerConnection!.onTrack = (event) {
      if (event.streams.isNotEmpty) {
        _remoteStreamController.add(event.streams.first);
      }
    };

    _peerConnection!.onConnectionState = (state) {
      _connectionStateController.add(state);
    };
  }

  /// 设置本地媒体流
  Future<void> setLocalStream(MediaStream stream) async {
    _localStream = stream;
    for (final track in stream.getTracks()) {
      await _peerConnection?.addTrack(track, stream);
    }
  }

  /// 创建 Offer
  Future<RTCSessionDescription> createOffer() async {
    final offer = await _peerConnection!.createOffer();
    await _peerConnection!.setLocalDescription(offer);
    return offer;
  }

  /// 创建 Answer
  Future<RTCSessionDescription> createAnswer() async {
    final answer = await _peerConnection!.createAnswer();
    await _peerConnection!.setLocalDescription(answer);
    return answer;
  }

  /// 设置远程 SDP
  Future<void> setRemoteDescription(RTCSessionDescription description) async {
    await _peerConnection!.setRemoteDescription(description);
  }

  /// 添加 ICE 候选
  Future<void> addIceCandidate(RTCIceCandidate candidate) async {
    await _peerConnection!.addCandidate(candidate);
  }

  /// 关闭连接
  Future<void> close() async {
    await _peerConnection?.close();
    _peerConnection = null;
    _localStream?.dispose();
    _localStream = null;
  }

  /// 释放资源
  void dispose() {
    close();
    _iceCandidateController.close();
    _remoteStreamController.close();
    _connectionStateController.close();
  }
}

/// WebRTC 管理器
///
/// 管理多个 PeerConnection
class WebRTCManager {
  final PeerConnectionConfig config;
  final Map<String, WebRTCConnection> _connections = {};
  MediaStream? _localStream;

  final _remoteStreamController =
      StreamController<(String, MediaStream)>.broadcast();

  WebRTCManager({this.config = const PeerConnectionConfig()});

  /// 远程流 (peerId, stream)
  Stream<(String, MediaStream)> get onRemoteStream =>
      _remoteStreamController.stream;

  /// 所有连接
  Map<String, WebRTCConnection> get connections =>
      Map.unmodifiable(_connections);

  /// 获取本地媒体流
  Future<MediaStream> getLocalMediaStream() async {
    _localStream ??= await navigator.mediaDevices.getUserMedia({
      'audio': config.enableAudio,
      'video': config.enableVideo
          ? {
              'facingMode': 'user',
              'width': {'ideal': 1280},
              'height': {'ideal': 720},
            }
          : false,
    });
    return _localStream!;
  }

  /// 获取屏幕共享流
  Future<MediaStream> getDisplayMedia() async {
    _localStream = await navigator.mediaDevices.getDisplayMedia({
      'video': true,
      'audio': true,
    });
    return _localStream!;
  }

  /// 创建与 Peer 的连接
  Future<WebRTCConnection> createConnection(
    String peerId, {
    bool isInitiator = false,
  }) async {
    if (_connections.containsKey(peerId)) {
      return _connections[peerId]!;
    }

    final connection = WebRTCConnection(
      peerId: peerId,
      config: config,
      isInitiator: isInitiator,
    );

    await connection.initialize();

    // 如果有本地流，添加到连接
    if (_localStream != null) {
      await connection.setLocalStream(_localStream!);
    }

    // 监听远程流
    connection.onRemoteStream.listen((stream) {
      _remoteStreamController.add((peerId, stream));
    });

    _connections[peerId] = connection;
    return connection;
  }

  /// 移除连接
  Future<void> removeConnection(String peerId) async {
    final connection = _connections.remove(peerId);
    await connection?.close();
    connection?.dispose();
  }

  /// 获取连接
  WebRTCConnection? getConnection(String peerId) => _connections[peerId];

  /// 关闭所有连接
  Future<void> closeAll() async {
    for (final connection in _connections.values) {
      await connection.close();
      connection.dispose();
    }
    _connections.clear();
    _localStream?.dispose();
    _localStream = null;
  }

  /// 释放资源
  void dispose() {
    closeAll();
    _remoteStreamController.close();
  }
}
