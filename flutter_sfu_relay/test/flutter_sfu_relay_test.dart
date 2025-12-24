import 'package:flutter_test/flutter_test.dart';

import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';

void main() {
  group('SfuRelay', () {
    test('instance should be singleton', () {
      final instance1 = SfuRelay.instance;
      final instance2 = SfuRelay.instance;
      expect(identical(instance1, instance2), true);
    });
  });

  group('Enums', () {
    test('DeviceType should have correct values', () {
      expect(DeviceType.unknown.value, 0);
      expect(DeviceType.pc.value, 1);
      expect(DeviceType.pad.value, 2);
      expect(DeviceType.mobile.value, 3);
      expect(DeviceType.tv.value, 4);
    });

    test('ConnectionType should have correct values', () {
      expect(ConnectionType.unknown.value, 0);
      expect(ConnectionType.ethernet.value, 1);
      expect(ConnectionType.wifi.value, 2);
      expect(ConnectionType.cellular.value, 3);
    });

    test('PowerState should have correct values', () {
      expect(PowerState.unknown.value, 0);
      expect(PowerState.pluggedIn.value, 1);
      expect(PowerState.battery.value, 2);
      expect(PowerState.lowBattery.value, 3);
    });

    test('PeerStatus.fromInt should work correctly', () {
      expect(PeerStatus.fromInt(0), PeerStatus.unknown);
      expect(PeerStatus.fromInt(1), PeerStatus.online);
      expect(PeerStatus.fromInt(2), PeerStatus.slow);
      expect(PeerStatus.fromInt(3), PeerStatus.offline);
      expect(PeerStatus.fromInt(99), PeerStatus.unknown);
    });

    test('SfuEventType.fromInt should work correctly', () {
      expect(SfuEventType.fromInt(0), SfuEventType.unknown);
      expect(SfuEventType.fromInt(1), SfuEventType.peerJoined);
      expect(SfuEventType.fromInt(3), SfuEventType.electionResult);
      expect(SfuEventType.fromInt(99), SfuEventType.unknown);
    });
  });

  group('SignalingMessage', () {
    test('fromJson should parse correctly', () {
      final json = {
        'type': 'offer',
        'roomId': 'room-1',
        'peerId': 'peer-1',
        'targetPeerId': 'peer-2',
        'data': {'sdp': 'v=0...'},
      };

      final message = SignalingMessage.fromJson(json);
      expect(message.type, SignalingMessageType.offer);
      expect(message.roomId, 'room-1');
      expect(message.peerId, 'peer-1');
      expect(message.targetPeerId, 'peer-2');
      expect(message.data?['sdp'], 'v=0...');
    });

    test('toJson should serialize correctly', () {
      final message = SignalingMessage(
        type: SignalingMessageType.answer,
        roomId: 'room-2',
        peerId: 'peer-3',
        data: {'sdp': 'answer-sdp'},
      );

      final json = message.toJson();
      expect(json['type'], 'answer');
      expect(json['roomId'], 'room-2');
      expect(json['peerId'], 'peer-3');
      expect(json['data']?['sdp'], 'answer-sdp');
    });
  });

  group('SfuEvent', () {
    test('should create with current timestamp', () {
      final before = DateTime.now();
      final event = SfuEvent(
        type: SfuEventType.peerJoined,
        roomId: 'room-1',
        peerId: 'peer-1',
      );
      final after = DateTime.now();

      expect(
        event.timestamp.isAfter(before.subtract(const Duration(seconds: 1))),
        true,
      );
      expect(
        event.timestamp.isBefore(after.add(const Duration(seconds: 1))),
        true,
      );
    });

    test('toString should format correctly', () {
      final event = SfuEvent(
        type: SfuEventType.relayChanged,
        roomId: 'room-1',
        peerId: 'peer-1',
      );

      expect(event.toString(), contains('relayChanged'));
      expect(event.toString(), contains('room-1'));
    });
  });

  group('LogEntry', () {
    test('should format toString correctly', () {
      final entry = LogEntry(level: LogLevel.warn, message: 'Test warning');

      expect(entry.toString(), '[WARN] Test warning');
    });
  });

  group('RtpStats', () {
    test('should calculate correctly', () {
      final stats = RtpStats(
        packetsForwarded: 100,
        packetsReceived: 50,
        bytesForwarded: 12800,
      );

      expect(stats.packetsForwarded, 100);
      expect(stats.packetsReceived, 50);
      expect(stats.bytesForwarded, 12800);
    });
  });

  group('ProxyState', () {
    test('should have correct states', () {
      expect(ProxyState.idle.name, 'idle');
      expect(ProxyState.electing.name, 'electing');
      expect(ProxyState.asRelay.name, 'asRelay');
      expect(ProxyState.connectedToRelay.name, 'connectedToRelay');
      expect(ProxyState.failover.name, 'failover');
    });
  });

  group('RelayInfo', () {
    test('should create with correct values', () {
      final relay = RelayInfo(peerId: 'relay-peer', epoch: 5, score: 75.0);

      expect(relay.peerId, 'relay-peer');
      expect(relay.epoch, 5);
      expect(relay.score, 75.0);
    });
  });

  group('PeerInfo', () {
    test('should create with defaults', () {
      final peer = PeerInfo(peerId: 'test-peer');

      expect(peer.peerId, 'test-peer');
      expect(peer.deviceType, DeviceType.unknown);
      expect(peer.connectionType, ConnectionType.unknown);
      expect(peer.isRelay, false);
    });
  });

  group('RoomState', () {
    test('should have correct states', () {
      expect(RoomState.disconnected.name, 'disconnected');
      expect(RoomState.connecting.name, 'connecting');
      expect(RoomState.connected.name, 'connected');
      expect(RoomState.sharing.name, 'sharing');
    });
  });
}
