/// RTP 转发器
///
/// 将 RTP 包在 Go SFU Core 和 WebRTC 之间转发
library;

import 'dart:async';
import 'dart:typed_data';

import '../core/coordinator.dart';
import '../media/source_switcher.dart';

/// RTP 转发方向
enum RtpDirection {
  /// 从 SFU 接收
  fromSfu,

  /// 发送到 SFU
  toSfu,

  /// 本地分享
  local,
}

/// RTP 包信息
class RtpPacketInfo {
  final bool isVideo;
  final Uint8List data;
  final RtpDirection direction;
  final DateTime timestamp;

  RtpPacketInfo({
    required this.isVideo,
    required this.data,
    required this.direction,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  /// 数据长度
  int get length => data.length;
}

/// RTP 转发器
///
/// 负责在 Go SFU Core 和 Flutter WebRTC 之间转发 RTP 包
///
/// 使用方式：
/// 1. 从 WebRTC DataChannel 或 RTP 接收器获取 RTP 包
/// 2. 调用 injectToSfu() 将包注入 Go 层
/// 3. Go 层处理后通过回调返回给其他 Peer
class RtpForwarder {
  final String roomId;
  final Coordinator? coordinator;
  final SourceSwitcher? sourceSwitcher;

  final _statsController = StreamController<RtpStats>.broadcast();

  int _packetsForwardedToSfu = 0;
  int _packetsFromSfu = 0;
  int _bytesForwarded = 0;

  RtpForwarder({required this.roomId, this.coordinator, this.sourceSwitcher});

  /// 统计流
  Stream<RtpStats> get stats => _statsController.stream;

  /// 注入 RTP 包到 SFU (来自本地 WebRTC)
  ///
  /// 当从本地 WebRTC 发送器接收到 RTP 包时调用
  bool injectToSfu(bool isVideo, Uint8List data) {
    bool success = false;

    if (coordinator != null) {
      success = coordinator!.injectSfuPacket(isVideo, data);
    } else if (sourceSwitcher != null) {
      success = sourceSwitcher!.injectSfuPacket(isVideo, data);
    }

    if (success) {
      _packetsForwardedToSfu++;
      _bytesForwarded += data.length;
    }

    return success;
  }

  /// 注入本地分享 RTP 包
  ///
  /// 当本地分享时，将本地捕获的 RTP 包注入
  bool injectLocal(bool isVideo, Uint8List data) {
    bool success = false;

    if (coordinator != null) {
      success = coordinator!.injectLocalPacket(isVideo, data);
    } else if (sourceSwitcher != null) {
      success = sourceSwitcher!.injectLocalPacket(isVideo, data);
    }

    if (success) {
      _bytesForwarded += data.length;
    }

    return success;
  }

  /// 获取当前统计
  RtpStats getCurrentStats() => RtpStats(
    packetsForwarded: _packetsForwardedToSfu,
    packetsReceived: _packetsFromSfu,
    bytesForwarded: _bytesForwarded,
  );

  /// 重置统计
  void resetStats() {
    _packetsForwardedToSfu = 0;
    _packetsFromSfu = 0;
    _bytesForwarded = 0;
  }

  /// 释放资源
  void dispose() {
    _statsController.close();
  }
}

/// RTP 统计
class RtpStats {
  final int packetsForwarded;
  final int packetsReceived;
  final int bytesForwarded;
  final DateTime timestamp;

  RtpStats({
    required this.packetsForwarded,
    required this.packetsReceived,
    required this.bytesForwarded,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  @override
  String toString() =>
      'RtpStats(fwd: $packetsForwarded, rcv: $packetsReceived, bytes: $bytesForwarded)';
}
