/// 枚举定义
library;

/// 设备类型
enum DeviceType {
  unknown(0),
  pc(1),
  pad(2),
  mobile(3),
  tv(4);

  const DeviceType(this.value);
  final int value;
}

/// 连接类型
enum ConnectionType {
  unknown(0),
  ethernet(1),
  wifi(2),
  cellular(3);

  const ConnectionType(this.value);
  final int value;
}

/// 电源状态
enum PowerState {
  unknown(0),
  pluggedIn(1),
  battery(2),
  lowBattery(3);

  const PowerState(this.value);
  final int value;
}

/// Peer 状态
enum PeerStatus {
  unknown(0),
  online(1),
  slow(2),
  offline(3);

  const PeerStatus(this.value);
  final int value;

  static PeerStatus fromInt(int v) => PeerStatus.values.firstWhere(
    (e) => e.value == v,
    orElse: () => PeerStatus.unknown,
  );
}

/// 事件类型
enum SfuEventType {
  unknown(0),
  peerJoined(1),
  peerLeft(2),
  trackAdded(3),
  error(4),
  iceCandidate(5),
  relayChanged(6), // Mapped to Go's ProxyChange
  answer(7),
  offer(8),

  // Legacy/Dart-only (placeholder to fix analyzer, verify if needed)
  electionResult(99),

  subscriberJoined(10),
  subscriberLeft(11),
  renegotiate(12),
  // 心跳检测事件 (来自 KeepaliveManager)
  peerOnline(20),
  peerSlow(21),
  peerOffline(22),
  ping(23),
  // 降级事件
  relayDisabled(24);

  const SfuEventType(this.value);
  final int value;

  static SfuEventType fromInt(int v) => SfuEventType.values.firstWhere(
    (e) => e.value == v,
    orElse: () => SfuEventType.unknown,
  );
}
