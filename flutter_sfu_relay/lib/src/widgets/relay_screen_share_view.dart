import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../core/auto_coordinator.dart';

/// Relay 屏幕共享视图
///
/// 智能选择最佳的渲染方式：
/// 1. 本地分享者：直接使用 LiveKit Track
/// 2. Relay Host (Loopback)：优先使用 P2P 本地回环流（以节省带宽，SFU流通常被取消订阅）
/// 3. 局域网订阅者：优先使用 P2P 流
/// 4. 蜂窝网络/其他：使用 LiveKit SFU 流
class RelayScreenShareView extends StatefulWidget {
  /// The widget to display when not using P2P (e.g. SFU video view)
  final Widget? sfuView;

  /// Automatic coordinator
  final AutoCoordinator? autoCoordinator;

  /// Fallback builder
  final WidgetBuilder? fallbackBuilder;

  /// Loading builder
  final WidgetBuilder? loadingBuilder;

  /// Whether usage of P2P is preferred (e.g. for Loopback/LAN)
  final bool requestP2P;

  /// Object fit for P2P video
  final RTCVideoViewObjectFit objectFit;

  const RelayScreenShareView({
    Key? key,
    this.sfuView,
    this.autoCoordinator,
    this.fallbackBuilder,
    this.loadingBuilder,
    this.requestP2P = true,
    this.objectFit = RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
  }) : super(key: key);

  @override
  State<RelayScreenShareView> createState() => _RelayScreenShareViewState();
}

class _RelayScreenShareViewState extends State<RelayScreenShareView> {
  RTCVideoRenderer? _p2pVideoRenderer;
  bool _p2pRendererInitialized = false;
  StreamSubscription<MediaStream?>? _streamSubscription;
  MediaStream? _currentP2PStream;
  bool _firstFrameRendered = false;

  @override
  void initState() {
    super.initState();
    _initP2PRenderer();
  }

  @override
  void dispose() {
    _streamSubscription?.cancel();
    _p2pVideoRenderer?.dispose();
    super.dispose();
  }

  Future<void> _initP2PRenderer() async {
    if (widget.autoCoordinator == null) return;

    _p2pVideoRenderer = RTCVideoRenderer();
    await _p2pVideoRenderer!.initialize();

    // 设置首帧渲染回调
    _p2pVideoRenderer!.onFirstFrameRendered = () {
      if (mounted) {
        setState(() {
          _firstFrameRendered = true;
        });
      }
    };

    if (mounted) {
      setState(() {
        _p2pRendererInitialized = true;
      });
      _subscribeToP2PStream();
    }
  }

  void _subscribeToP2PStream() {
    if (widget.autoCoordinator == null) return;

    // 初始流
    _handleStreamChanged(widget.autoCoordinator!.p2pRemoteStream);

    // 监听变化
    _streamSubscription = widget.autoCoordinator!.onRemoteStream.listen(
      _handleStreamChanged,
    );
  }

  void _handleStreamChanged(MediaStream? stream) {
    if (stream?.id == _currentP2PStream?.id) return;

    // 如果渲染器已经被dispose，不要处理 (防卫代码)
    if (_p2pVideoRenderer == null) return;

    setState(() {
      _currentP2PStream = stream;
      if (stream != null) {
        _p2pVideoRenderer!.srcObject = stream;
        _firstFrameRendered = false; // 新流重置首帧标志
      } else {
        _p2pVideoRenderer!.srcObject = null;
      }
    });

    if (stream != null) {
      debugPrint(
        '[RelayScreenShareView] Switching to P2P Stream: ${stream.id}',
      );
    } else {
      debugPrint('[RelayScreenShareView] P2P Stream removed');
    }
  }

  @override
  Widget build(BuildContext context) {
    // 1. If we are NOT requesting P2P (or P2P logic says no), show SFU view.
    // 2. But wait, previously we checked isSharerLocal. Logic should be:
    //    If caller provides sfuView, that's what we show when P2P is not active.

    // Check availability
    final useP2P = _shouldUseP2P();

    // Debug info
    // debugPrint('RelayView: useP2P=$useP2P (req=${widget.requestP2P})');

    if (!useP2P) {
      if (widget.sfuView != null) {
        return widget.sfuView!;
      }
      if (widget.fallbackBuilder != null) {
        return widget.fallbackBuilder!(context);
      }
      return Container(color: Colors.black);
    }

    if (useP2P) {
      // P2P 渲染逻辑
      if (_p2pRendererInitialized && _currentP2PStream != null) {
        if (_firstFrameRendered) {
          // 正常渲染
          return RTCVideoView(_p2pVideoRenderer!, objectFit: widget.objectFit);
        } else {
          // 等待首帧：堆叠 Loading
          return Stack(
            fit: StackFit.expand,
            children: [
              RTCVideoView(_p2pVideoRenderer!, objectFit: widget.objectFit),
              if (widget.loadingBuilder != null)
                widget.loadingBuilder!(context)
              else
                const Center(
                  child: CircularProgressIndicator(color: Colors.white),
                ),
            ],
          );
        }
      } else {
        // P2P 尚未就绪
        if (widget.fallbackBuilder != null) {
          return widget.fallbackBuilder!(context);
        }
        // 默认等待 UI
        return const Center(
          child: CircularProgressIndicator(color: Colors.white),
        );
      }
    }

    // Should not reach here if useP2P is true, but just in case
    return Container(color: Colors.black);
  }

  bool _shouldUseP2P() {
    if (!widget.requestP2P) return false;
    if (widget.autoCoordinator == null) return false;
    final ac = widget.autoCoordinator!;

    // Relay Host (Loopback) 或 局域网用户
    final isRelay = ac.isRelay;
    final isOnLan = ac.isOnLan;

    if (!isRelay && !isOnLan) return false;

    // 检查 P2P 连接是否真正可用
    return ac.hasP2PConnection;
  }
}
