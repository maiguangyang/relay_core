import 'dart:async';
import 'package:flutter/material.dart';
import 'package:livekit_client/livekit_client.dart' as lk;
import '../theme/app_theme.dart';

/// 参与者卡片组件
class ParticipantTile extends StatefulWidget {
  final lk.Participant participant;
  final bool isRelay;
  final bool isLocal;
  final bool isScreenSharing;
  final bool isMaximized;
  final VoidCallback? onTap;
  final VoidCallback? onMaximize;

  const ParticipantTile({
    super.key,
    required this.participant,
    this.isRelay = false,
    this.isLocal = false,
    this.isScreenSharing = false,
    this.isMaximized = false,
    this.onTap,
    this.onMaximize,
  });

  @override
  State<ParticipantTile> createState() => _ParticipantTileState();
}

class _ParticipantTileState extends State<ParticipantTile>
    with SingleTickerProviderStateMixin {
  bool _isSpeaking = false;
  double _audioLevel = 0.0;
  lk.EventsListener<lk.ParticipantEvent>? _listener;
  Timer? _speakingTimer;

  @override
  void initState() {
    super.initState();
    _setupListener();
  }

  @override
  void didUpdateWidget(ParticipantTile oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.participant != widget.participant) {
      _listener?.dispose();
      _setupListener();
    }
  }

  void _setupListener() {
    _listener = widget.participant.createListener();

    // 监听音频级别变化
    _listener!.on<lk.AudioPlaybackStatusChanged>((event) {
      _updateSpeakingState();
    });

    // 监听轨道变化
    _listener!.on<lk.TrackMutedEvent>((event) => _updateSpeakingState());
    _listener!.on<lk.TrackUnmutedEvent>((event) => _updateSpeakingState());

    // 定时检查音频状态
    _startAudioLevelPolling();
  }

  void _startAudioLevelPolling() {
    _speakingTimer?.cancel();
    _speakingTimer = Timer.periodic(const Duration(milliseconds: 100), (_) {
      _checkAudioLevel();
    });
  }

  void _checkAudioLevel() {
    if (!mounted) return;

    // 获取音频轨道的音量级别
    final audioTracks = widget.participant.audioTrackPublications;
    double maxLevel = 0.0;

    for (final pub in audioTracks) {
      if (pub.track != null && !pub.muted) {
        // 本地参与者使用 localTrack 的音量
        if (widget.isLocal && pub.track is lk.LocalAudioTrack) {
          // LocalAudioTrack 可以获取当前音量
          maxLevel = 0.3; // 简化处理，开启麦克风就认为可能在说话
        } else if (pub.track is lk.RemoteAudioTrack) {
          // 远程音频轨道
          maxLevel = 0.3;
        }
      }
    }

    // 使用 isSpeaking 状态（如果有的话）
    final isSpeaking = widget.participant.isSpeaking;

    if (mounted) {
      setState(() {
        _isSpeaking = isSpeaking;
        _audioLevel = maxLevel;
      });
    }
  }

  void _updateSpeakingState() {
    if (mounted) {
      setState(() {
        _isSpeaking = widget.participant.isSpeaking;
      });
    }
  }

  @override
  void dispose() {
    _speakingTimer?.cancel();
    _listener?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // 确定边框颜色
    Color borderColor;
    double borderWidth;

    if (_isSpeaking) {
      borderColor = AppTheme.onlineColor; // 绿色表示说话
      borderWidth = 3;
    } else if (widget.isRelay) {
      borderColor = AppTheme.relayColor;
      borderWidth = 2;
    } else {
      borderColor = Colors.transparent;
      borderWidth = 0;
    }

    return GestureDetector(
      onTap: widget.onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        decoration: AppTheme.glassDecoration.copyWith(
          border: Border.all(color: borderColor, width: borderWidth),
          boxShadow: _isSpeaking
              ? [
                  BoxShadow(
                    color: AppTheme.onlineColor.withOpacity(0.5),
                    blurRadius: 12,
                    spreadRadius: 2,
                  ),
                ]
              : null,
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(16),
          child: Stack(
            fit: StackFit.expand,
            children: [
              // 视频或头像
              _buildVideoOrAvatar(),

              // 渐变遮罩
              _buildGradientOverlay(),

              // 信息栏
              _buildInfoBar(),

              // 状态指示器
              _buildStatusIndicators(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildVideoOrAvatar() {
    // 查找视频轨道
    final videoTrack = widget.participant.videoTrackPublications
        .where((pub) => pub.subscribed && pub.track != null && !pub.muted)
        .map((pub) => pub.track as lk.VideoTrack?)
        .firstOrNull;

    if (videoTrack != null) {
      return lk.VideoTrackRenderer(videoTrack);
    }

    // 无视频时显示头像
    return Container(
      color: AppTheme.cardDark,
      child: Center(
        child: CircleAvatar(
          radius: 40,
          backgroundColor: widget.isRelay
              ? AppTheme.relayColor
              : AppTheme.primaryColor,
          child: Text(
            _getInitials(),
            style: const TextStyle(
              fontSize: 24,
              fontWeight: FontWeight.bold,
              color: Colors.white,
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildGradientOverlay() {
    return Positioned(
      bottom: 0,
      left: 0,
      right: 0,
      height: 80,
      child: Container(
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Colors.transparent, Colors.black.withOpacity(0.7)],
          ),
        ),
      ),
    );
  }

  Widget _buildInfoBar() {
    return Positioned(
      bottom: 8,
      left: 8,
      right: 8,
      child: Row(
        children: [
          // 说话指示器
          if (_isSpeaking)
            Container(
              margin: const EdgeInsets.only(right: 6),
              child: Icon(
                Icons.graphic_eq,
                color: AppTheme.onlineColor,
                size: 16,
              ),
            ),

          // 名称
          Expanded(
            child: Text(
              widget.isLocal
                  ? '${widget.participant.identity} (我)'
                  : widget.participant.identity,
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.w500,
                fontSize: 14,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),

          // Relay 标识
          if (widget.isRelay)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
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
        ],
      ),
    );
  }

  Widget _buildStatusIndicators() {
    final isMuted = widget.participant.audioTrackPublications.every(
      (pub) => pub.muted || !pub.subscribed,
    );

    return Positioned(
      top: 8,
      right: 8,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          // 最大化/缩小按钮 (仅屏幕共享时显示)
          if (widget.isScreenSharing && widget.onMaximize != null)
            Padding(
              padding: const EdgeInsets.only(right: 6),
              child: GestureDetector(
                onTap: widget.onMaximize,
                child: Container(
                  padding: const EdgeInsets.all(6),
                  decoration: BoxDecoration(
                    color: AppTheme.secondaryColor.withOpacity(0.9),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Icon(
                    widget.isMaximized
                        ? Icons.fullscreen_exit
                        : Icons.fullscreen,
                    color: Colors.white,
                    size: 16,
                  ),
                ),
              ),
            ),

          // 麦克风状态
          AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: _isSpeaking
                  ? AppTheme.onlineColor.withOpacity(0.8)
                  : Colors.black.withOpacity(0.5),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              isMuted ? Icons.mic_off : Icons.mic,
              color: isMuted ? AppTheme.accentColor : Colors.white,
              size: 16,
            ),
          ),
        ],
      ),
    );
  }

  String _getInitials() {
    final name = widget.participant.identity;
    if (name.isEmpty) return '?';
    final parts = name.split('_');
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return name.substring(0, name.length.clamp(0, 2)).toUpperCase();
  }
}
