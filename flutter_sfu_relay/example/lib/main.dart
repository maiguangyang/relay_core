import 'package:flutter/material.dart';
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatefulWidget {
  const MyApp({super.key});

  @override
  State<MyApp> createState() => _MyAppState();
}

class _MyAppState extends State<MyApp> {
  String _version = 'Loading...';
  String _roomState = 'Disconnected';
  bool _isRelay = false;
  List<String> _peers = [];
  List<String> _logs = [];

  RelayRoom? _room;

  @override
  void initState() {
    super.initState();
    _initSdk();
  }

  void _initSdk() {
    // 初始化回调系统
    EventHandler.init();
    LogHandler.init();
    PingHandler.init();

    // 监听日志
    LogHandler.logs.listen((entry) {
      setState(() {
        _logs.insert(0, entry.toString());
        if (_logs.length > 50) _logs.removeLast();
      });
    });

    setState(() {
      _version = SfuRelay.instance.version;
    });
  }

  Future<void> _joinRoom() async {
    final signaling = WebSocketSignaling(
      url: 'ws://localhost:8080/ws',
      localPeerId: 'flutter-peer-${DateTime.now().millisecondsSinceEpoch}',
    );

    _room = RelayRoom(
      roomId: 'test-room',
      localPeerId: signaling.localPeerId,
      signaling: signaling,
    );

    // 监听事件
    _room!.onStateChanged.listen((state) {
      setState(() {
        _roomState = state.name;
      });
    });

    _room!.onPeerJoined.listen((peer) {
      setState(() {
        _peers.add(peer.peerId);
      });
    });

    _room!.onPeerLeft.listen((peerId) {
      setState(() {
        _peers.remove(peerId);
      });
    });

    _room!.onRelayChanged.listen((relayId) {
      setState(() {
        _isRelay = _room!.isRelay;
      });
    });

    try {
      await _room!.join(
        deviceType: DeviceType.pc,
        connectionType: ConnectionType.wifi,
      );
    } catch (e) {
      setState(() {
        _logs.insert(0, 'Error joining room: $e');
      });
    }
  }

  Future<void> _leaveRoom() async {
    await _room?.leave();
    _room?.dispose();
    _room = null;
    setState(() {
      _roomState = 'Disconnected';
      _peers.clear();
      _isRelay = false;
    });
  }

  @override
  void dispose() {
    _room?.dispose();
    EventHandler.dispose();
    LogHandler.dispose();
    PingHandler.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      theme: ThemeData.dark().copyWith(
        colorScheme: ColorScheme.dark(
          primary: Colors.tealAccent,
          secondary: Colors.cyanAccent,
        ),
      ),
      home: Scaffold(
        appBar: AppBar(
          title: const Text('Flutter SFU Relay Demo'),
          backgroundColor: Colors.teal,
        ),
        body: Padding(
          padding: const EdgeInsets.all(16.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // SDK 信息
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(16.0),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'SDK Version: $_version',
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 8),
                      Row(
                        children: [
                          Text('Room State: '),
                          Chip(
                            label: Text(_roomState),
                            backgroundColor: _roomState == 'connected'
                                ? Colors.green
                                : _roomState == 'sharing'
                                ? Colors.blue
                                : Colors.grey,
                          ),
                        ],
                      ),
                      if (_isRelay) ...[
                        const SizedBox(height: 8),
                        Chip(
                          label: const Text('I am Relay'),
                          backgroundColor: Colors.orange,
                        ),
                      ],
                    ],
                  ),
                ),
              ),

              const SizedBox(height: 16),

              // 操作按钮
              Row(
                children: [
                  ElevatedButton(
                    onPressed: _roomState == 'disconnected' ? _joinRoom : null,
                    child: const Text('Join Room'),
                  ),
                  const SizedBox(width: 16),
                  ElevatedButton(
                    onPressed: _roomState != 'disconnected' ? _leaveRoom : null,
                    child: const Text('Leave Room'),
                  ),
                ],
              ),

              const SizedBox(height: 16),

              // Peer 列表
              Text(
                'Peers (${_peers.length}):',
                style: Theme.of(context).textTheme.titleMedium,
              ),
              Wrap(
                spacing: 8,
                children: _peers.map((p) => Chip(label: Text(p))).toList(),
              ),

              const SizedBox(height: 16),

              // 日志
              Expanded(
                child: Card(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Padding(
                        padding: const EdgeInsets.all(8.0),
                        child: Text(
                          'Logs:',
                          style: Theme.of(context).textTheme.titleMedium,
                        ),
                      ),
                      Expanded(
                        child: ListView.builder(
                          itemCount: _logs.length,
                          itemBuilder: (context, index) => Padding(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 8,
                              vertical: 2,
                            ),
                            child: Text(
                              _logs[index],
                              style: const TextStyle(
                                fontSize: 12,
                                fontFamily: 'monospace',
                              ),
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
