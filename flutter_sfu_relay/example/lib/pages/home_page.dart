import 'dart:async';
import 'dart:convert';
import 'package:flutter/services.dart';
import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:device_info_plus/device_info_plus.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_sfu_relay/flutter_sfu_relay.dart';
import 'package:livekit_client/livekit_client.dart' as lk;
import 'package:permission_handler/permission_handler.dart';

import '../theme/app_theme.dart';
import '../widgets/control_bar.dart';
import '../widgets/screen_share_dialog.dart';

/// 首页 - 独立完整的会议页面
/// 包含连接表单、LiveKit集成、屏幕共享、语音会议功能
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  static const platform = MethodChannel(
    'com.example.flutterSfuRelay/broadcast_picker',
  );

  // GlobalKey for ScaffoldMessenger to show SnackBars from anywhere
  final GlobalKey<ScaffoldMessengerState> _scaffoldMessengerKey =
      GlobalKey<ScaffoldMessengerState>();

  // 连接配置
  final _urlController = TextEditingController(
    // text: 'wss://frp.marlon.proton-system.com',
    text: 'wss://oxygen-sl1zv95n.livekit.cloud',
  );
  final _tokenController = TextEditingController();
  // 影子连接专用：Bot Token (identity: "relay-bot", hidden: true, canSubscribe: true)
  final _botTokenController = TextEditingController(
    // 测试用写死的 Bot Token
    text:
        'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3OTgyNjQ4MDQsImlkZW50aXR5IjoicmVsYXktYm90IiwiaXNzIjoiQVBJQnNza2pZczZqU2t5IiwibmFtZSI6InJlbGF5LWJvdCIsIm5iZiI6MTc2NjcyODgwNCwic3ViIjoicmVsYXktYm90IiwidmlkZW8iOnsicm9vbSI6InRlc3Rfcm9vbSIsInJvb21Kb2luIjp0cnVlfX0.UJQj70gBARSlOuRU9EdVacm-03oC91DwKqpM6BDUFB8',
  );

  // 页面状态
  bool _isInMeeting = false;
  bool _isConnecting = false;
  String? _errorMessage;

  // LiveKit
  lk.Room? _room;
  lk.LocalParticipant? _localParticipant;
  lk.EventsListener<lk.RoomEvent>? _roomListener;

  // SFU Relay
  AutoCoordinator? _autoCoord;
  AutoCoordinatorState _relayState = AutoCoordinatorState.idle;
  String? _currentRelay;
  double _localScore = 0;

  // RTP 监控统计
  Map<String, dynamic> _rtpStats = {};
  Timer? _statsTimer;

  // 参与者列表
  final List<lk.Participant> _participants = [];

  // 屏幕共享状态
  lk.Participant? _screenShareParticipant;
  bool _isScreenShareMaximized = true; // 默认最大化显示

  // 控制状态
  ControlState _controlState = const ControlState();

  // 网络变化监听
  StreamSubscription<List<ConnectivityResult>>? _connectivitySubscription;
  ConnectionType? _lastConnectionType;

  @override
  void initState() {
    super.initState();
    _initSdk();
    _startConnectivityListener();

    // 初始化屏幕共享覆盖层回调 (macOS/Windows)
    if (Platform.isMacOS || Platform.isWindows) {
      ScreenCaptureChannel.initialize();
      ScreenCaptureChannel.onStopSharingRequested = () {
        // 当用户点击原生浮动控制栏的"结束共享"按钮 (macOS)
        // 或者通过其他方式请求停止共享 (Windows)
        _toggleScreenShare();
      };
    }
  }

  void _startConnectivityListener() {
    _connectivitySubscription = Connectivity().onConnectivityChanged.listen((
      results,
    ) {
      _handleConnectivityChange(results);
    });
  }

  void _handleConnectivityChange(List<ConnectivityResult> results) {
    // 计算当前网络类型
    ConnectionType? currentType;
    if (results.contains(ConnectivityResult.ethernet)) {
      currentType = ConnectionType.ethernet;
    } else if (results.contains(ConnectivityResult.wifi)) {
      currentType = ConnectionType.wifi;
    } else if (results.contains(ConnectivityResult.mobile)) {
      currentType = ConnectionType.cellular;
    }

    // 如果在会议中且网络类型发生变化
    if (_isInMeeting &&
        _lastConnectionType != null &&
        currentType != null &&
        _lastConnectionType != currentType) {
      debugPrint(
        '[Network] Network changed from $_lastConnectionType to $currentType, disconnecting...',
      );
      _handleNetworkChange(currentType);
    }

    _lastConnectionType = currentType;
  }

  Future<void> _handleNetworkChange(ConnectionType newType) async {
    // 先断开当前连接
    await _disconnect();

    await Future.delayed(const Duration(milliseconds: 1500));

    // 重新连接
    await _connect();
  }

  void _initSdk() {
    EventHandler.init();
    LogHandler.init();
    PingHandler.init();
  }

  @override
  void dispose() {
    _connectivitySubscription?.cancel();
    _disconnect();
    _urlController.dispose();
    _tokenController.dispose();
    _botTokenController.dispose();
    EventHandler.dispose();
    LogHandler.dispose();
    PingHandler.dispose();
    super.dispose();
  }

  // ============================================================
  // 权限请求
  // ============================================================

  Future<bool> _requestPermissions() async {
    // Web 和桌面平台：LiveKit 会自动处理权限弹窗
    if (kIsWeb || Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
      return true;
    }

    // Android/iOS: 尝试使用 permission_handler，失败则继续
    try {
      final statuses = await [
        Permission.microphone,
        Permission.camera,
      ].request();
      final micGranted =
          statuses[Permission.microphone] == PermissionStatus.granted;
      if (!micGranted && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('需要麦克风权限才能加入会议')));
      }
      // 即使权限被拒绝也继续，让 LiveKit 尝试
      return true;
    } catch (e) {
      debugPrint('Permission request error: $e');
      return true; // 继续连接，让 LiveKit 处理
    }
  }

  /// 检测当前网络连接类型
  Future<ConnectionType> _detectConnectionType() async {
    try {
      final results = await Connectivity().checkConnectivity();
      // results 是 List<ConnectivityResult>
      if (results.contains(ConnectivityResult.ethernet)) {
        return ConnectionType.ethernet;
      } else if (results.contains(ConnectivityResult.wifi)) {
        return ConnectionType.wifi;
      } else if (results.contains(ConnectivityResult.mobile)) {
        return ConnectionType.cellular;
      }
      return ConnectionType.unknown;
    } catch (e) {
      debugPrint('Network detection error: $e');
      // 桐面默认返回 WiFi（macOS/Windows/Linux）
      if (Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
        return ConnectionType.wifi;
      }
      return ConnectionType.unknown;
    }
  }

  // ============================================================
  // 连接逻辑
  // ============================================================

  Future<void> _connect() async {
    if (_tokenController.text.isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('请输入 Token')));
      return;
    }

    setState(() {
      _isConnecting = true;
      _errorMessage = null;
    });

    try {
      // 1. 创建并连接 LiveKit 房间
      _room = lk.Room(
        roomOptions: const lk.RoomOptions(
          adaptiveStream: true,
          dynacast: true,
          defaultAudioOutputOptions: lk.AudioOutputOptions(speakerOn: true),
          defaultAudioCaptureOptions: lk.AudioCaptureOptions(
            noiseSuppression: true,
            echoCancellation: true,
            autoGainControl: true,
          ),
          defaultAudioPublishOptions: lk.AudioPublishOptions(dtx: true),
        ),
      );
      _roomListener = _room!.createListener();

      debugPrint('[Connect] Connecting to room...');
      // 添加重试机制
      int retryCount = 0;
      const maxRetries = 3;

      while (true) {
        try {
          await _room!.connect(_urlController.text, _tokenController.text);
          debugPrint('[Connect] Room connected');
          break; // 连接成功，跳出循环
        } catch (e) {
          retryCount++;
          debugPrint('[Connect] Connection attempt $retryCount failed: $e');

          if (retryCount >= maxRetries) {
            debugPrint('[Connect] Max retries reached. Giving up.');
            rethrow; // 超过最大重试次数，抛出异常
          }

          debugPrint('[Connect] Retrying in ${retryCount * 2} seconds...');
          await Future.delayed(Duration(seconds: retryCount * 2));
        }
      }

      _localParticipant = _room!.localParticipant;

      // 2. 设置 LiveKit 事件监听
      _setupRoomListeners();

      // 3. 检测网络类型
      final connectionType = await _detectConnectionType();
      _lastConnectionType = connectionType; // 记录初始网络类型
      debugPrint('[Network] Detected connection type: $connectionType');

      // 4. 创建信令和 AutoCoordinator
      final signaling = _LiveKitSignaling(
        room: _room!,
        localPeerId: _localParticipant!.identity,
      );

      _autoCoord = AutoCoordinator(
        roomId: _room!.name ?? 'room',
        localPeerId: _localParticipant!.identity,
        signaling: signaling,
        config: AutoCoordinatorConfig(
          deviceType: _detectDeviceType(),
          connectionType: connectionType, // 使用检测到的网络类型
          powerState: PowerState.pluggedIn,
          autoElection: true,
          // 影子连接配置
          livekitUrl: _urlController.text,
          // 动态获取 Bot Token 的回调（只有当选为 Relay 时才会调用）
          onRequestBotToken: (roomId) async {
            debugPrint(
              '[ShadowConnection] Requesting Bot Token for room: $roomId',
            );
            // 如果填写了 Bot Token，使用影子连接
            if (_botTokenController.text.isNotEmpty) {
              debugPrint('[ShadowConnection] Returning explicit Bot Token');
              return _botTokenController.text;
            }
            // 没有填写则不启动影子连接
            debugPrint('[ShadowConnection] No Bot Token available');
            return null;
          },
        ),
      );

      // 4. 监听 Relay 状态
      _autoCoord!.onStateChanged.listen((state) {
        if (mounted) setState(() => _relayState = state);
      });

      _autoCoord!.onRelayChanged.listen((relayId) {
        if (mounted) setState(() => _currentRelay = relayId);
      });

      // 5. 监听 Peer 加入/离开 (比 LiveKit 事件更快)
      _autoCoord!.onPeerJoined.listen((peerId) {
        if (mounted) {
          debugPrint('[Signaling] Peer joined: $peerId');
          _updateParticipants();
        }
      });

      _autoCoord!.onPeerLeft.listen((peerId) {
        if (mounted) {
          debugPrint('[Signaling] Peer left: $peerId');
          _updateParticipants();
        }
      });

      // 5. 启动
      await _autoCoord!.start();

      setState(() {
        _isConnecting = false;
        _isInMeeting = true;
        _localScore = _autoCoord!.localScore;
        _updateParticipants();
      });

      // 启动 RTP 统计监控
      _startStatsMonitor();

      // 默认静音，用户可点击麦克风按钮开启
    } catch (e) {
      debugPrint('[Connect] Error: $e');

      // 关键修复：如果连接过程中发生错误，必须清理已建立的连接
      // 否则会产生"幽灵用户" - 服务器端已连接但本地UI显示未连接
      try {
        _autoCoord?.dispose();
        _autoCoord = null;
      } catch (_) {}

      try {
        _roomListener?.dispose();
        _roomListener = null;
      } catch (_) {}

      try {
        await _room?.disconnect();
        _room?.dispose();
        _room = null;
      } catch (_) {}

      _localParticipant = null;

      setState(() {
        _isConnecting = false;
        _errorMessage = e.toString();
      });

      debugPrint('[Connect] Cleanup completed after error');
    }
  }

  void _setupRoomListeners() {
    _roomListener!
      ..on<lk.ParticipantConnectedEvent>((_) => _updateParticipants())
      ..on<lk.ParticipantDisconnectedEvent>((_) => _updateParticipants())
      ..on<lk.TrackPublishedEvent>((_) => _updateParticipants())
      ..on<lk.TrackUnpublishedEvent>((_) => _updateParticipants())
      // 远程轨道订阅 - 确保音频播放
      ..on<lk.TrackSubscribedEvent>((event) {
        _updateParticipants();
        // 如果是远程音频轨道，确保开始播放
        if (event.track is lk.RemoteAudioTrack) {
          final audioTrack = event.track as lk.RemoteAudioTrack;
          audioTrack.start();
          debugPrint(
            '[Audio] Remote audio track subscribed: ${event.participant.identity}',
          );
        }
      })
      ..on<lk.TrackMutedEvent>((_) => _updateParticipants())
      ..on<lk.TrackUnmutedEvent>((_) => _updateParticipants())
      // 说话者变化事件 - 用于更新发言指示器
      ..on<lk.ActiveSpeakersChangedEvent>((_) {
        if (mounted) setState(() {});
      })
      // 音频播放状态
      ..on<lk.AudioPlaybackStatusChanged>((event) {
        debugPrint('[Audio] Playback status changed: ${event.isPlaying}');
        if (!event.isPlaying) {
          // 尝试恢复音频播放
          _room?.startAudio();
        }
      })
      // 本地轨道发布/取消发布事件
      ..on<lk.LocalTrackPublishedEvent>((event) {
        _updateParticipants();
        _updateControlState();
        if (event.publication.source == lk.TrackSource.microphone) {
          debugPrint('[Audio] Local mic track published');
        }
      })
      ..on<lk.LocalTrackUnpublishedEvent>((event) {
        _updateParticipants();
        _updateControlState();
      });
  }

  void _updateControlState() {
    if (_localParticipant == null) return;

    // 检查本地屏幕共享状态
    final hasScreenShare = _localParticipant!.videoTrackPublications.any(
      (pub) =>
          pub.source == lk.TrackSource.screenShareVideo && pub.track != null,
    );

    if (_controlState.screenShareEnabled != hasScreenShare) {
      setState(() {
        _controlState = _controlState.copyWith(
          screenShareEnabled: hasScreenShare,
        );
      });
    }
  }

  void _updateParticipants() {
    if (!mounted || _room == null) return;

    setState(() {
      _participants.clear();
      if (_localParticipant != null) {
        _participants.add(_localParticipant!);
      }

      // 添加远程参与者，但蜂窝网络用户不显示 relay-bot（影子用户）
      for (final participant in _room!.remoteParticipants.values) {
        // 蜂窝网络用户过滤掉 relay-bot
        // if (_lastConnectionType == ConnectionType.cellular) {
        final identity = participant.identity;
        if (identity == 'relay-bot' || identity.startsWith('relay-bot')) {
          continue; // 跳过 relay-bot
        }
        // }
        _participants.add(participant);
      }

      // 检测屏幕共享参与者
      _screenShareParticipant = null;
      for (final p in _participants) {
        final isLocal = p.identity == _localParticipant?.identity;
        for (final pub in p.videoTrackPublications) {
          if (pub.source == lk.TrackSource.screenShareVideo &&
              pub.track != null &&
              !pub.muted) {
            // 本地参与者不需要检查 subscribed，远程参与者需要
            if (isLocal || pub.subscribed) {
              _screenShareParticipant = p;
              break;
            }
          }
        }
        if (_screenShareParticipant != null) break;
      }
    });
  }

  /// 启动 RTP 统计监控
  void _startStatsMonitor() {
    _statsTimer?.cancel();
    _statsTimer = Timer.periodic(const Duration(seconds: 2), (_) {
      if (_autoCoord != null && mounted) {
        final status = _autoCoord!.getStatus();
        if (mounted) {
          setState(() {
            _rtpStats = status;
          });
        }
        // 打印到控制台供调试
        final sfuPackets = status['sfu_packets'] ?? 0;
        final localPackets = status['local_packets'] ?? 0;
        final isRelay = _autoCoord?.isRelay ?? false;
        if (isRelay || sfuPackets > 0 || localPackets > 0) {
          debugPrint(
            '[RTP Stats] isRelay: $isRelay, SFU: $sfuPackets, Local: $localPackets',
          );
        }
      }
    });
  }

  /// 停止 RTP 统计监控
  void _stopStatsMonitor() {
    _statsTimer?.cancel();
    _statsTimer = null;
  }

  DeviceType _detectDeviceType() {
    if (kIsWeb) return DeviceType.pc;
    if (Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
      return DeviceType.pc;
    }
    if (Platform.isIOS || Platform.isAndroid) {
      return DeviceType.mobile;
    }
    return DeviceType.unknown;
  }

  Future<void> _disconnect() async {
    // 停止统计监控
    _stopStatsMonitor();

    // 1. 先停止 AutoCoordinator（包含信令清理）
    if (_autoCoord != null) {
      try {
        // 给 AutoCoordinator 停止一个超时时间
        await _autoCoord!.stop().timeout(const Duration(seconds: 2));
      } catch (e) {
        debugPrint('Error stopping AutoCoordinator: $e');
      }
      // 无论 stop 是否成功，都要 dispose
      try {
        _autoCoord!.dispose();
      } catch (e) {
        debugPrint('Error disposing AutoCoordinator: $e');
      }
      _autoCoord = null;
    }

    // 2. 清理 Room Listener
    if (_roomListener != null) {
      try {
        _roomListener!.dispose();
      } catch (e) {
        debugPrint('Error disposing roomListener: $e');
      }
      _roomListener = null;
    }

    // 3. 断开并销毁 Room
    if (_room != null) {
      try {
        await _room!.disconnect().timeout(const Duration(seconds: 2));
      } catch (e) {
        debugPrint('Error disconnecting room: $e');
      }
      // 完全销毁 Room 对象，确保 Data Channel 等内部状态被清理
      try {
        _room!.dispose();
      } catch (e) {
        debugPrint('Error disposing room: $e');
      }
      _room = null;
    }
    _localParticipant = null;

    // 等待更长时间，让 SDK 完成异步清理（解决网络切换后 LocalParticipant 类型错误）
    await Future.delayed(const Duration(milliseconds: 2000));

    if (mounted) {
      setState(() {
        _isInMeeting = false;
        _relayState = AutoCoordinatorState.idle;
        _currentRelay = null;
        _participants.clear();
        _controlState = const ControlState();
        _errorMessage = null; // 清除错误信息
      });
    }
  }

  // ============================================================
  // 媒体控制
  // ============================================================

  Future<void> _toggleMic() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.micEnabled;

    // 开启麦克风时请求权限
    if (newState && !await _requestPermissions()) {
      return;
    }

    try {
      // iOS 上需要延迟以确保 SDP offer 包含音频信息
      if (!kIsWeb && Platform.isIOS) {
        await Future.delayed(const Duration(milliseconds: 500));
      }

      await _localParticipant!.setMicrophoneEnabled(newState);

      // 确保扬声器打开
      if (newState && !kIsWeb) {
        try {
          await lk.Hardware.instance.setSpeakerphoneOn(true);
        } catch (_) {}
      }

      setState(() {
        _controlState = _controlState.copyWith(micEnabled: newState);
      });
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('无法启用麦克风: $e')));
    }
  }

  Future<void> _toggleCamera() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.cameraEnabled;
    try {
      await _localParticipant!.setCameraEnabled(newState);
      setState(() {
        _controlState = _controlState.copyWith(cameraEnabled: newState);
      });
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('无法启用摄像头: $e')));
    }
  }

  Future<void> _toggleScreenShare() async {
    if (_localParticipant == null) return;

    final newState = !_controlState.screenShareEnabled;

    try {
      if (newState) {
        // 检查是否已有人在分享屏幕
        if (_screenShareParticipant != null &&
            _screenShareParticipant!.identity != _localParticipant?.identity) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(
                '“${_screenShareParticipant!.identity}” 正在分享屏幕，请等待结束后再试',
              ),
              duration: const Duration(seconds: 2),
            ),
          );
          return;
        }

        // 开启屏幕共享
        if (!kIsWeb && (Platform.isMacOS || Platform.isWindows)) {
          // 桌面平台：使用自定义窗口选择器
          final result = await showDialog<ScreenShareResult>(
            context: context,
            barrierColor: Colors.black54,
            builder: (context) => const ScreenShareDialog(),
          );

          if (result == null) return;

          // 使用 sharingType = .none 实现自排除，无需额外处理
          final track = await lk.LocalVideoTrack.createScreenShareTrack(
            lk.ScreenShareCaptureOptions(
              sourceId: result.source.id,
              maxFrameRate: 15.0,
            ),
          );
          await _localParticipant!.publishVideoTrack(track);

          // macOS/Windows/Linux: 显示屏幕共享 UI（最小化窗口 + 排除捕获）
          if ((Platform.isMacOS || Platform.isWindows || Platform.isLinux) &&
              result.isScreen) {
            await ScreenCaptureChannel.showScreenShareUI();
          }
        } else if (!kIsWeb && Platform.isIOS) {
          // iOS: 使用 Broadcast Extension 进行屏幕共享
          // 检查是否是模拟器，模拟器不支持 ReplayKit 屏幕共享
          final deviceInfo = DeviceInfoPlugin();
          final iosInfo = await deviceInfo.iosInfo;

          if (!iosInfo.isPhysicalDevice) {
            if (mounted) {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(
                  content: Text('iOS 模拟器不支持屏幕共享，请使用真机测试'),
                  duration: Duration(seconds: 3),
                ),
              );
            }
            return;
          }

          // 先启动 Socket 创建（不等待完成，让它在后台运行）
          debugPrint(
            '[ScreenShare] Starting setScreenShareEnabled with useiOSBroadcastExtension...',
          );
          final screenShareFuture = _localParticipant!
              .setScreenShareEnabled(
                true,
                screenShareCaptureOptions: const lk.ScreenShareCaptureOptions(
                  useiOSBroadcastExtension: true,
                  maxFrameRate: 15.0,
                ),
              )
              .then((_) {
                debugPrint(
                  '[ScreenShare] setScreenShareEnabled completed successfully',
                );
              })
              .catchError((e) {
                debugPrint('[ScreenShare] setScreenShareEnabled error: $e');
              });

          // 稍等一下让 Socket 创建完成
          debugPrint('[ScreenShare] Waiting 500ms for socket creation...');
          await Future.delayed(const Duration(milliseconds: 500));
          debugPrint('[ScreenShare] Delay complete, launching picker...');

          // 再弹出 Picker，此时 Socket 应该已经准备好
          try {
            await platform.invokeMethod('launchBroadcastPicker');
            debugPrint('[ScreenShare] Picker launched successfully');
          } catch (e) {
            debugPrint('[ScreenShare] Failed to launch broadcast picker: $e');
          }

          // 等待屏幕共享完成
          debugPrint('[ScreenShare] Waiting for screen share to complete...');
          await screenShareFuture;
          debugPrint('[ScreenShare] Screen share flow finished');
        } else {
          // Android 和其他平台
          await _localParticipant!.setScreenShareEnabled(true);
        }
      } else {
        await _localParticipant!.setScreenShareEnabled(false);

        // macOS/Windows: 隐藏屏幕共享 UI 并恢复窗口
        if (Platform.isMacOS || Platform.isWindows) {
          await ScreenCaptureChannel.hideScreenShareUI();
        }
      }

      setState(() {
        _controlState = _controlState.copyWith(screenShareEnabled: newState);
      });
    } catch (e) {
      _scaffoldMessengerKey.currentState?.showSnackBar(
        SnackBar(content: Text('屏幕共享失败: $e')),
      );
    }
  }

  void _leave() {
    _disconnect();
  }

  // ============================================================
  // UI 构建
  // ============================================================

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      scaffoldMessengerKey: _scaffoldMessengerKey,
      debugShowCheckedModeBanner: false,
      theme: AppTheme.darkTheme,
      home: Scaffold(
        body: Container(
          decoration: AppTheme.gradientBackground,
          child: SafeArea(
            bottom: false,
            child: _isInMeeting ? _buildMeetingView() : _buildLoginView(),
          ),
        ),
      ),
    );
  }

  // -------------------- 登录视图 --------------------

  Widget _buildLoginView() {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 48), // Top padding
            _buildHeader(),
            const SizedBox(height: 48),
            _buildConnectionForm(),
            if (_errorMessage != null) ...[
              const SizedBox(height: 16),
              Text(
                _errorMessage!,
                style: const TextStyle(color: AppTheme.accentColor),
                textAlign: TextAlign.center,
              ),
            ],
            const SizedBox(height: 48), // Bottom padding before footer
            _buildFooter(),
            const SizedBox(height: 24), // Extra bottom padding
          ],
        ),
      ),
    );
  }

  Widget _buildHeader() {
    return Column(
      children: [
        Container(
          width: 100,
          height: 100,
          decoration: BoxDecoration(
            gradient: const LinearGradient(
              colors: [AppTheme.primaryColor, AppTheme.secondaryColor],
            ),
            borderRadius: BorderRadius.circular(24),
            boxShadow: [
              BoxShadow(
                color: AppTheme.primaryColor.withOpacity(0.4),
                blurRadius: 30,
                offset: const Offset(0, 10),
              ),
            ],
          ),
          child: const Icon(Icons.groups, size: 48, color: Colors.white),
        ),
        const SizedBox(height: 24),
        const Text(
          'SFU Relay',
          style: TextStyle(
            fontSize: 32,
            fontWeight: FontWeight.bold,
            color: AppTheme.textPrimary,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          '局域网视频会议',
          style: TextStyle(fontSize: 16, color: AppTheme.textSecondary),
        ),
      ],
    );
  }

  Widget _buildConnectionForm() {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: AppTheme.glassDecoration,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextField(
            controller: _urlController,
            style: const TextStyle(color: AppTheme.textPrimary),
            decoration: const InputDecoration(
              labelText: 'LiveKit 服务器',
              hintText: 'ws://your-server:7880',
              prefixIcon: Icon(Icons.dns, color: AppTheme.textSecondary),
            ),
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _tokenController,
            style: const TextStyle(color: AppTheme.textPrimary),
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Token',
              hintText: '输入访问令牌',
              prefixIcon: Icon(Icons.vpn_key, color: AppTheme.textSecondary),
            ),
          ),
          const SizedBox(height: 16),
          // 影子连接模式：独立 Bot Token (可选)
          TextField(
            controller: _botTokenController,
            style: const TextStyle(color: AppTheme.textPrimary),
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Bot Token (可选)',
              hintText: '影子连接：hidden=true, canSubscribe=true',
              prefixIcon: Icon(Icons.smart_toy, color: AppTheme.textSecondary),
            ),
          ),
          const SizedBox(height: 24),
          ElevatedButton(
            onPressed: _isConnecting ? null : _connect,
            style: ElevatedButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
            child: _isConnecting
                ? const SizedBox(
                    width: 24,
                    height: 24,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  )
                : const Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(Icons.login),
                      SizedBox(width: 8),
                      Text('加入会议'),
                    ],
                  ),
          ),
        ],
      ),
    );
  }

  Widget _buildFooter() {
    return Column(
      children: [
        Text(
          '提示: 使用 lk token create 生成 Token',
          style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.flash_on, size: 14, color: AppTheme.secondaryColor),
            const SizedBox(width: 4),
            Text(
              '局域网 ~2.5s 故障切换',
              style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
            ),
          ],
        ),
      ],
    );
  }

  // -------------------- 会议视图 --------------------

  Widget _buildMeetingView() {
    return Column(
      children: [
        _buildStatusBar(),
        Expanded(child: _buildParticipantGrid()),
        ControlBar(
          state: _controlState.copyWith(isRelay: _autoCoord?.isRelay ?? false),
          participantCount: _participants.length,
          onMicToggle: _toggleMic,
          onCameraToggle: _toggleCamera,
          onScreenShareToggle: _toggleScreenShare,
          onParticipants: _showParticipantsDrawer,
          onLeave: _leave,
        ),
      ],
    );
  }

  void _showParticipantsDrawer() {
    showGeneralDialog(
      context: context,
      barrierDismissible: true,
      barrierLabel: 'Participants',
      barrierColor: Colors.black54,
      transitionDuration: const Duration(milliseconds: 300),
      pageBuilder: (context, animation, secondaryAnimation) {
        return Align(
          alignment: Alignment.centerRight,
          child: Material(
            color: Colors.transparent,
            child: Container(
              width: 320,
              height: double.infinity,
              decoration: BoxDecoration(
                color: AppTheme.surfaceDark,
                borderRadius: const BorderRadius.horizontal(
                  left: Radius.circular(20),
                ),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withOpacity(0.3),
                    blurRadius: 20,
                    offset: const Offset(-5, 0),
                  ),
                ],
              ),
              child: Column(
                children: [
                  // 标题栏
                  Container(
                    padding: const EdgeInsets.all(20),
                    child: Row(
                      children: [
                        const Icon(Icons.people, color: AppTheme.primaryColor),
                        const SizedBox(width: 12),
                        Text(
                          '参与者 (${_participants.length})',
                          style: const TextStyle(
                            color: AppTheme.textPrimary,
                            fontSize: 18,
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                        const Spacer(),
                        IconButton(
                          icon: const Icon(
                            Icons.close,
                            color: AppTheme.textSecondary,
                          ),
                          onPressed: () => Navigator.pop(context),
                        ),
                      ],
                    ),
                  ),
                  const Divider(color: AppTheme.cardDark, height: 1),
                  // 参与者列表
                  Expanded(
                    child: ListView.builder(
                      padding: const EdgeInsets.all(16),
                      itemCount: _participants.length,
                      itemBuilder: (context, index) {
                        final participant = _participants[index];
                        final isLocal =
                            participant.identity == _localParticipant?.identity;
                        final isRelay = participant.identity == _currentRelay;
                        final isScreenSharing =
                            participant.identity ==
                            _screenShareParticipant?.identity;
                        final isMuted = participant.audioTrackPublications
                            .every((pub) => pub.muted || !pub.subscribed);

                        return Container(
                          margin: const EdgeInsets.only(bottom: 8),
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: AppTheme.cardDark,
                            borderRadius: BorderRadius.circular(12),
                            border: isRelay
                                ? Border.all(
                                    color: AppTheme.relayColor,
                                    width: 2,
                                  )
                                : null,
                          ),
                          child: Row(
                            children: [
                              CircleAvatar(
                                radius: 18,
                                backgroundColor: isRelay
                                    ? AppTheme.relayColor
                                    : AppTheme.primaryColor,
                                child: Text(
                                  participant.identity.isNotEmpty
                                      ? participant.identity[0].toUpperCase()
                                      : '?',
                                  style: const TextStyle(
                                    color: Colors.white,
                                    fontWeight: FontWeight.bold,
                                    fontSize: 14,
                                  ),
                                ),
                              ),
                              const SizedBox(width: 10),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      isLocal
                                          ? '${participant.identity} (我)'
                                          : participant.identity,
                                      style: const TextStyle(
                                        color: AppTheme.textPrimary,
                                        fontWeight: FontWeight.w500,
                                        fontSize: 13,
                                      ),
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                    if (isRelay || isScreenSharing) ...[
                                      const SizedBox(height: 4),
                                      Row(
                                        children: [
                                          if (isRelay)
                                            _buildTag(
                                              'RELAY',
                                              AppTheme.relayColor,
                                            ),
                                          if (isScreenSharing)
                                            _buildTag(
                                              '共享',
                                              AppTheme.secondaryColor,
                                            ),
                                        ],
                                      ),
                                    ],
                                  ],
                                ),
                              ),
                              Icon(
                                isMuted ? Icons.mic_off : Icons.mic,
                                color: isMuted
                                    ? AppTheme.textSecondary
                                    : AppTheme.onlineColor,
                                size: 18,
                              ),
                            ],
                          ),
                        );
                      },
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
      },
      transitionBuilder: (context, animation, secondaryAnimation, child) {
        return SlideTransition(
          position: Tween<Offset>(
            begin: const Offset(1, 0),
            end: Offset.zero,
          ).animate(CurvedAnimation(parent: animation, curve: Curves.easeOut)),
          child: child,
        );
      },
    );
  }

  Widget _buildTag(String text, Color color) {
    return Container(
      margin: const EdgeInsets.only(right: 6),
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.2),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color, width: 1),
      ),
      child: Text(
        text,
        style: TextStyle(
          color: color,
          fontSize: 10,
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  Widget _buildStatusBar() {
    final isRelay = _autoCoord?.isRelay ?? false;
    final sfuPackets = _rtpStats['sfu_packets'] ?? 0;
    final localPackets = _rtpStats['local_packets'] ?? 0;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: _getStateColor().withOpacity(0.2),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(color: _getStateColor()),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(_getStateIcon(), size: 16, color: _getStateColor()),
                const SizedBox(width: 6),
                Text(
                  _getStateText(),
                  style: TextStyle(color: _getStateColor(), fontSize: 12),
                ),
              ],
            ),
          ),
          // RTP 统计 (仅 Relay 显示)
          if (isRelay) ...[
            const SizedBox(width: 12),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              decoration: BoxDecoration(
                color: Colors.green.withOpacity(0.2),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.green.withOpacity(0.5)),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(Icons.sync_alt, size: 14, color: Colors.green),
                  const SizedBox(width: 4),
                  Text(
                    'SFU:$sfuPackets L:$localPackets',
                    style: const TextStyle(color: Colors.green, fontSize: 10),
                  ),
                ],
              ),
            ),
          ],
          const Spacer(),
          Text(
            '分数: ${_localScore.toInt()}',
            style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
          ),
          const SizedBox(width: 16),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: AppTheme.cardDark,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(
                  Icons.people,
                  size: 16,
                  color: AppTheme.textSecondary,
                ),
                const SizedBox(width: 6),
                Text(
                  '${_participants.length}',
                  style: TextStyle(color: AppTheme.textPrimary, fontSize: 12),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildParticipantGrid() {
    if (_participants.isEmpty) {
      return const Center(
        child: Text(
          '等待其他参与者加入...',
          style: TextStyle(color: AppTheme.textSecondary),
        ),
      );
    }

    // 如果有人在分享屏幕且已最大化，使用特殊布局
    // 但如果是本地参与者正在共享屏幕，则不显示自己的共享画面（继续显示普通会议室视图）
    final isLocalSharing =
        _screenShareParticipant?.identity == _localParticipant?.identity;

    if (_screenShareParticipant != null && _isScreenShareMaximized) {
      return _buildFeaturedLayout();
    }

    // 分离有视频和无视频的参与者
    final List<_ParticipantData> withVideo = [];
    final List<_ParticipantData> withoutVideo = [];

    for (final participant in _participants) {
      final isLocal = participant.identity == _localParticipant?.identity;
      final isRelay = participant.identity == _currentRelay;
      final isScreenSharing =
          participant.identity == _screenShareParticipant?.identity;
      final isSpeaking = participant.isSpeaking;
      final isMuted = participant.audioTrackPublications.every(
        (pub) => pub.muted || !pub.subscribed,
      );

      // 获取视频轨道
      final videoTrack = participant.videoTrackPublications
          .where(
            (pub) =>
                pub.source == lk.TrackSource.camera &&
                pub.track != null &&
                !pub.muted,
          )
          .map((pub) => pub.track as lk.VideoTrack?)
          .firstOrNull;

      final data = _ParticipantData(
        participant: participant,
        videoTrack: videoTrack,
        isLocal: isLocal,
        isRelay: isRelay,
        isScreenSharing: isScreenSharing,
        isSpeaking: isSpeaking,
        isMuted: isMuted,
      );

      if (videoTrack != null) {
        withVideo.add(data);
      } else {
        withoutVideo.add(data);
      }
    }

    return LayoutBuilder(
      builder: (context, constraints) {
        return SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: BoxConstraints(
              minHeight: constraints.maxHeight - 48, // 减去 padding
            ),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                // 有视频的参与者区域（顶部）
                if (withVideo.isNotEmpty)
                  Center(
                    child: Wrap(
                      spacing: 24,
                      runSpacing: 20,
                      alignment: WrapAlignment.center,
                      children: withVideo
                          .map(
                            (data) => _buildParticipantAvatar(
                              participant: data.participant,
                              videoTrack: data.videoTrack,
                              isLocal: data.isLocal,
                              isRelay: data.isRelay,
                              isScreenSharing: data.isScreenSharing,
                              isSpeaking: data.isSpeaking,
                              isMuted: data.isMuted,
                            ),
                          )
                          .toList(),
                    ),
                  ),
                // 间距
                if (withVideo.isNotEmpty && withoutVideo.isNotEmpty)
                  const SizedBox(height: 32),
                // 无视频的参与者区域（底部）
                if (withoutVideo.isNotEmpty)
                  Center(
                    child: Wrap(
                      spacing: 24,
                      runSpacing: 16,
                      alignment: WrapAlignment.center,
                      children: withoutVideo
                          .map(
                            (data) => _buildParticipantAvatar(
                              participant: data.participant,
                              videoTrack: data.videoTrack,
                              isLocal: data.isLocal,
                              isRelay: data.isRelay,
                              isScreenSharing: data.isScreenSharing,
                              isSpeaking: data.isSpeaking,
                              isMuted: data.isMuted,
                            ),
                          )
                          .toList(),
                    ),
                  ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _buildParticipantAvatar({
    required lk.Participant participant,
    lk.VideoTrack? videoTrack,
    required bool isLocal,
    required bool isRelay,
    required bool isScreenSharing,
    required bool isSpeaking,
    required bool isMuted,
  }) {
    // 如果有视频，显示大的视频卡片
    if (videoTrack != null) {
      return _buildVideoCard(
        participant: participant,
        videoTrack: videoTrack,
        isLocal: isLocal,
        isRelay: isRelay,
        isScreenSharing: isScreenSharing,
        isSpeaking: isSpeaking,
        isMuted: isMuted,
      );
    }

    // 没有视频时，显示小的头像
    return _buildAvatarCard(
      participant: participant,
      isLocal: isLocal,
      isRelay: isRelay,
      isScreenSharing: isScreenSharing,
      isSpeaking: isSpeaking,
      isMuted: isMuted,
    );
  }

  /// 大尺寸视频卡片 - 摄像头打开时使用
  Widget _buildVideoCard({
    required lk.Participant participant,
    required lk.VideoTrack videoTrack,
    required bool isLocal,
    required bool isRelay,
    required bool isScreenSharing,
    required bool isSpeaking,
    required bool isMuted,
  }) {
    const cardWidth = 200.0;
    const cardHeight = 260.0;

    return Container(
      width: cardWidth,
      height: cardHeight,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(16),
        border: Border.all(
          color: isSpeaking
              ? AppTheme.onlineColor
              : isRelay
              ? AppTheme.relayColor
              : Colors.white.withOpacity(0.1),
          width: isSpeaking ? 3 : 1,
        ),
        boxShadow: isSpeaking
            ? [
                BoxShadow(
                  color: AppTheme.onlineColor.withOpacity(0.4),
                  blurRadius: 16,
                  spreadRadius: 2,
                ),
              ]
            : [
                BoxShadow(
                  color: Colors.black.withOpacity(0.3),
                  blurRadius: 8,
                  offset: const Offset(0, 4),
                ),
              ],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(15),
        child: Stack(
          fit: StackFit.expand,
          children: [
            // 视频
            lk.VideoTrackRenderer(videoTrack),
            // 底部渐变遮罩
            Positioned(
              bottom: 0,
              left: 0,
              right: 0,
              height: 60,
              child: Container(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [Colors.transparent, Colors.black.withOpacity(0.7)],
                  ),
                ),
              ),
            ),
            // 名称和状态
            Positioned(
              bottom: 8,
              left: 8,
              right: 8,
              child: Row(
                children: [
                  Icon(
                    isMuted ? Icons.mic_off : Icons.mic,
                    size: 16,
                    color: isMuted ? Colors.red : AppTheme.onlineColor,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      _getDisplayName(participant, isLocal),
                      style: const TextStyle(
                        color: Colors.white,
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        shadows: [Shadow(blurRadius: 4, color: Colors.black)],
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ],
              ),
            ),
            // RELAY 标签
            if (isRelay)
              Positioned(
                top: 8,
                right: 8,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: AppTheme.relayColor,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Text(
                    'RELAY',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ),
              ),
            // 屏幕共享标签
            if (isScreenSharing)
              Positioned(
                top: 8,
                left: 8,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: AppTheme.secondaryColor,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.screen_share, size: 12, color: Colors.white),
                      SizedBox(width: 4),
                      Text(
                        '共享中',
                        style: TextStyle(
                          color: Colors.white,
                          fontSize: 10,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  /// 小头像卡片 - 摄像头关闭时使用
  Widget _buildAvatarCard({
    required lk.Participant participant,
    required bool isLocal,
    required bool isRelay,
    required bool isScreenSharing,
    required bool isSpeaking,
    required bool isMuted,
  }) {
    const avatarSize = 80.0;

    return SizedBox(
      width: 130,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // 头像带边框和 RELAY 标签
          Stack(
            clipBehavior: Clip.none,
            children: [
              Container(
                width: avatarSize + 8,
                height: avatarSize + 8,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: isSpeaking
                        ? AppTheme.onlineColor
                        : isScreenSharing
                        ? AppTheme.secondaryColor
                        : Colors.transparent,
                    width: 1,
                  ),
                  boxShadow: isSpeaking
                      ? [
                          BoxShadow(
                            color: AppTheme.onlineColor.withOpacity(0.4),
                            blurRadius: 12,
                            spreadRadius: 2,
                          ),
                        ]
                      : null,
                ),
                child: Padding(
                  padding: const EdgeInsets.all(2),
                  child: ClipOval(
                    child: Container(
                      color: isRelay
                          ? AppTheme.relayColor
                          : AppTheme.primaryColor,
                      child: Center(
                        child: Text(
                          _getParticipantInitials(participant.identity),
                          style: const TextStyle(
                            color: Colors.white,
                            fontSize: 32,
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              // RELAY 标签在右上角
              if (isRelay)
                Positioned(
                  top: -4,
                  right: -14,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 6,
                      vertical: 2,
                    ),
                    decoration: BoxDecoration(
                      color: AppTheme.relayColor,
                      borderRadius: BorderRadius.circular(8),
                      boxShadow: [
                        BoxShadow(
                          color: Colors.black.withOpacity(0.3),
                          blurRadius: 4,
                          offset: const Offset(0, 2),
                        ),
                      ],
                    ),
                    child: const Text(
                      'RELAY',
                      style: TextStyle(
                        color: Colors.white,
                        fontSize: 8,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 8),
          // 名称和状态图标
          Row(
            mainAxisSize: MainAxisSize.min,
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // 麦克风状态
              Icon(
                isMuted ? Icons.mic_off : Icons.mic,
                size: 14,
                color: isMuted ? AppTheme.textSecondary : AppTheme.onlineColor,
              ),
              const SizedBox(width: 4),
              // 名称
              Flexible(
                child: Text(
                  _getDisplayName(participant, isLocal),
                  style: TextStyle(
                    color: AppTheme.textPrimary,
                    fontSize: 12,
                    fontWeight: isSpeaking
                        ? FontWeight.bold
                        : FontWeight.normal,
                  ),
                  overflow: TextOverflow.ellipsis,
                  textAlign: TextAlign.center,
                ),
              ),
            ],
          ),
          // 屏幕共享标签
          if (isScreenSharing)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: _buildSmallTag('共享', AppTheme.secondaryColor),
            ),
        ],
      ),
    );
  }

  /// 获取参与者的显示名称
  /// 对于 relay-bot，显示它属于哪个 Relay 用户
  String _getDisplayName(lk.Participant participant, bool isLocal) {
    final identity = participant.identity;

    // 检测是否是 relay-bot
    if (identity == 'relay-bot' || identity.startsWith('relay-bot')) {
      if (_currentRelay != null) {
        return 'Bot ($_currentRelay)';
      }
      return 'Bot';
    }

    // 本地用户
    if (isLocal) {
      return '$identity (我)';
    }

    return identity;
  }

  Widget _buildSmallTag(String text, Color color) {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 2),
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.2),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color, width: 1),
      ),
      child: Text(
        text,
        style: TextStyle(
          color: color,
          fontSize: 9,
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  String _getParticipantInitials(String name) {
    if (name.isEmpty) return '?';
    final parts = name.split('_');
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return name.substring(0, name.length.clamp(0, 1)).toUpperCase();
  }

  /// 特刊布局：屏幕共享者放大显示
  Widget _buildFeaturedLayout() {
    final screenSharer = _screenShareParticipant!;
    final isSharerLocal = screenSharer.identity == _localParticipant?.identity;

    // 获取屏幕共享视频轨道
    lk.VideoTrack? screenTrack;
    for (final pub in screenSharer.videoTrackPublications) {
      if (pub.source == lk.TrackSource.screenShareVideo &&
          pub.subscribed &&
          pub.track != null &&
          !pub.muted) {
        screenTrack = pub.track as lk.VideoTrack;
        break;
      }
    }

    return Column(
      children: [
        // 屏幕共享主视图
        Expanded(
          flex: 3,
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: AppTheme.secondaryColor, width: 3),
                boxShadow: [
                  BoxShadow(
                    color: AppTheme.secondaryColor.withOpacity(0.3),
                    blurRadius: 20,
                    spreadRadius: 2,
                  ),
                ],
              ),
              child: ClipRRect(
                borderRadius: BorderRadius.circular(13),
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    // 屏幕共享视频 - 使用 FittedBox 确保完整显示
                    if (screenTrack != null)
                      Container(
                        color: Colors.black,
                        child: FittedBox(
                          fit: BoxFit.contain,
                          child: SizedBox(
                            width: 1920,
                            height: 1080,
                            child: lk.VideoTrackRenderer(screenTrack),
                          ),
                        ),
                      )
                    else
                      Container(
                        color: AppTheme.cardDark,
                        child: const Center(
                          child: Icon(
                            Icons.screen_share,
                            size: 48,
                            color: AppTheme.textSecondary,
                          ),
                        ),
                      ),
                    // 分享者标签
                    Positioned(
                      bottom: 12,
                      left: 12,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 6,
                        ),
                        decoration: BoxDecoration(
                          color: AppTheme.secondaryColor,
                          borderRadius: BorderRadius.circular(20),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            const Icon(
                              Icons.screen_share,
                              size: 14,
                              color: Colors.white,
                            ),
                            const SizedBox(width: 6),
                            Text(
                              isSharerLocal
                                  ? '你正在分享屏幕'
                                  : '${screenSharer.identity} 的屏幕',
                              style: const TextStyle(
                                color: Colors.white,
                                fontSize: 12,
                                fontWeight: FontWeight.w500,
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
          ),
        ),
      ],
    );
  }

  Color _getStateColor() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return Colors.orange;
      case AutoCoordinatorState.asRelay:
        return AppTheme.relayColor;
      case AutoCoordinatorState.connected:
        return AppTheme.onlineColor;
      default:
        return AppTheme.textSecondary;
    }
  }

  IconData _getStateIcon() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return Icons.how_to_vote;
      case AutoCoordinatorState.asRelay:
        return Icons.router;
      case AutoCoordinatorState.connected:
        return Icons.check_circle;
      default:
        return Icons.radio_button_unchecked;
    }
  }

  String _getStateText() {
    switch (_relayState) {
      case AutoCoordinatorState.electing:
        return '选举中';
      case AutoCoordinatorState.asRelay:
        return 'Relay';
      case AutoCoordinatorState.connected:
        return '已连接';
      default:
        return '离线';
    }
  }
}

// ============================================================
// LiveKit 信令实现
// ============================================================

class _LiveKitSignaling implements SignalingBridge {
  final lk.Room room;
  final String localPeerId;

  final _messageController = StreamController<SignalingMessage>.broadcast();
  final _peerDisconnectedController = StreamController<String>.broadcast();
  final _peerConnectedController = StreamController<String>.broadcast();

  lk.EventsListener<lk.RoomEvent>? _listener;
  String? _currentRoomId;
  bool _isConnected = false;
  bool _disposed = false; // 防止 dispose 后处理事件

  _LiveKitSignaling({required this.room, required this.localPeerId}) {
    _listener = room.createListener();
    _listener?.on<lk.DataReceivedEvent>((event) {
      // 检查是否已 dispose，防止处理过时事件
      if (_disposed) return;
      // 使用 try-catch 防止 LiveKit SDK 内部类型转换错误
      try {
        _onDataReceived(event.data, event.participant);
      } catch (e) {
        debugPrint('[Signaling] DataReceivedEvent error: $e');
      }
    });
    _listener?.on<lk.ParticipantConnectedEvent>((event) {
      if (_disposed) return;
      try {
        _peerConnectedController.add(event.participant.identity);
      } catch (e) {
        debugPrint('[Signaling] ParticipantConnectedEvent error: $e');
      }
    });
    _listener?.on<lk.ParticipantDisconnectedEvent>((event) {
      if (_disposed) return;
      try {
        _peerDisconnectedController.add(event.participant.identity);
      } catch (e) {
        debugPrint('[Signaling] ParticipantDisconnectedEvent error: $e');
      }
    });
  }

  @override
  Stream<SignalingMessage> get messages => _messageController.stream;

  @override
  Stream<String> get peerDisconnected => _peerDisconnectedController.stream;

  @override
  Stream<String> get peerConnected => _peerConnectedController.stream;

  @override
  Future<void> connect() async {
    _isConnected = room.connectionState == lk.ConnectionState.connected;
  }

  @override
  bool get isConnected =>
      _isConnected || room.connectionState == lk.ConnectionState.connected;

  @override
  Future<void> disconnect() async {
    _listener?.dispose();
  }

  @override
  void dispose() {
    // 先设置标志，防止事件处理
    _disposed = true;
    _listener?.dispose();
    _listener = null;
    _messageController.close();
    _peerDisconnectedController.close();
    _peerConnectedController.close();
  }

  @override
  Future<void> joinRoom(String roomId, String peerId) async {
    _currentRoomId = roomId;
    await _broadcast({'type': 'join'});
  }

  @override
  Future<void> leaveRoom(String roomId) async {
    await _broadcast({'type': 'leave'});
    _currentRoomId = null;
  }

  @override
  Future<void> sendPing(String roomId, String targetPeerId) async {
    await _broadcast({'type': 'ping', 'targetPeerId': targetPeerId});
  }

  @override
  Future<void> sendPong(String roomId, String targetPeerId) async {
    await _broadcast({'type': 'pong', 'targetPeerId': targetPeerId});
  }

  @override
  Future<void> sendRelayClaim(String roomId, int epoch, double score) async {
    await _broadcast({'type': 'relayClaim', 'epoch': epoch, 'score': score});
  }

  @override
  Future<void> sendRelayChanged(
    String roomId,
    String relayId,
    int epoch,
    double score,
  ) async {
    await _broadcast({
      'type': 'relayChanged',
      'relayId': relayId,
      'epoch': epoch,
      'score': score,
    });
  }

  @override
  Future<void> sendOffer(String roomId, String targetPeerId, String sdp) async {
    await _broadcast({
      'type': 'offer',
      'targetPeerId': targetPeerId,
      'sdp': sdp,
    });
  }

  @override
  Future<void> sendAnswer(
    String roomId,
    String targetPeerId,
    String sdp,
  ) async {
    await _broadcast({
      'type': 'answer',
      'targetPeerId': targetPeerId,
      'sdp': sdp,
    });
  }

  @override
  Future<void> sendCandidate(
    String roomId,
    String targetPeerId,
    String candidate,
  ) async {
    await _broadcast({
      'type': 'candidate',
      'targetPeerId': targetPeerId,
      'candidate': candidate,
    });
  }

  Future<void> _broadcast(Map<String, dynamic> data) async {
    if (!_isConnected && room.connectionState != lk.ConnectionState.connected) {
      return;
    }

    data['peerId'] = localPeerId;
    data['roomId'] = _currentRoomId;

    // 在蜂窝网络上，数据通道可能需要更长时间准备
    // 添加重试机制
    for (int attempt = 0; attempt < 3; attempt++) {
      try {
        await room.localParticipant?.publishData(
          utf8.encode(jsonEncode(data)),
          reliable: true,
        );
        return; // 成功，退出
      } catch (e) {
        debugPrint('[Signaling] publishData attempt ${attempt + 1} failed: $e');
        if (attempt < 2) {
          await Future.delayed(Duration(milliseconds: 500 * (attempt + 1)));
        }
      }
    }
    // 即使失败也不抛出异常，让会议继续
    debugPrint(
      '[Signaling] publishData failed after 3 attempts, continuing...',
    );
  }

  void _onDataReceived(List<int> data, lk.Participant? participant) {
    // 忽略来自 LocalParticipant 的数据（不应该发生，但为了安全）
    // 以及 null 参与者
    if (participant == null || participant is lk.LocalParticipant) return;

    try {
      final json = jsonDecode(utf8.decode(data)) as Map<String, dynamic>;
      final type = json['type'] as String?;

      final message = SignalingMessage(
        type: _parseType(type),
        roomId: json['roomId'] as String? ?? _currentRoomId ?? '',
        peerId: participant.identity,
        targetPeerId: json['targetPeerId'] as String?,
        data: json,
      );

      _messageController.add(message);
    } catch (_) {
      // 忽略解析错误
    }
  }

  SignalingMessageType _parseType(String? type) {
    switch (type) {
      case 'join':
        return SignalingMessageType.join;
      case 'leave':
        return SignalingMessageType.leave;
      case 'ping':
        return SignalingMessageType.ping;
      case 'pong':
        return SignalingMessageType.pong;
      case 'relayClaim':
        return SignalingMessageType.relayClaim;
      case 'relayChanged':
        return SignalingMessageType.relayChanged;
      case 'offer':
        return SignalingMessageType.offer;
      case 'answer':
        return SignalingMessageType.answer;
      case 'candidate':
        return SignalingMessageType.candidate;
      default:
        return SignalingMessageType.error;
    }
  }
}

/// 参与者数据包装类，用于分组显示
class _ParticipantData {
  final lk.Participant participant;
  final lk.VideoTrack? videoTrack;
  final bool isLocal;
  final bool isRelay;
  final bool isScreenSharing;
  final bool isSpeaking;
  final bool isMuted;

  const _ParticipantData({
    required this.participant,
    this.videoTrack,
    required this.isLocal,
    required this.isRelay,
    required this.isScreenSharing,
    required this.isSpeaking,
    required this.isMuted,
  });
}
