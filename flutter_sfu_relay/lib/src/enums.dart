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
  electionResult(3),
  relayChanged(4),
  error(5);

  const SfuEventType(this.value);
  final int value;

  static SfuEventType fromInt(int v) => SfuEventType.values.firstWhere(
    (e) => e.value == v,
    orElse: () => SfuEventType.unknown,
  );
}
