import 'package:flutter/material.dart';
import 'package:livekit_client/livekit_client.dart' as lk;
import '../theme/app_theme.dart';

/// 参与者卡片组件
class ParticipantTile extends StatelessWidget {
  final lk.Participant participant;
  final bool isRelay;
  final bool isLocal;
  final VoidCallback? onTap;

  const ParticipantTile({
    super.key,
    required this.participant,
    this.isRelay = false,
    this.isLocal = false,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        decoration: AppTheme.glassDecoration.copyWith(
          border: isRelay
              ? Border.all(color: AppTheme.relayColor, width: 2)
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
    final videoTrack = participant.videoTrackPublications
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
          backgroundColor: isRelay
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
          // 名称
          Expanded(
            child: Text(
              isLocal ? '${participant.identity} (我)' : participant.identity,
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.w500,
                fontSize: 14,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),

          // Relay 标识
          if (isRelay)
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
    final isMuted = participant.audioTrackPublications.every(
      (pub) => pub.muted || !pub.subscribed,
    );

    return Positioned(
      top: 8,
      right: 8,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          // 麦克风状态
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: Colors.black.withOpacity(0.5),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              isMuted ? Icons.mic_off : Icons.mic,
              color: isMuted ? AppTheme.accentColor : AppTheme.onlineColor,
              size: 16,
            ),
          ),
        ],
      ),
    );
  }

  String _getInitials() {
    final name = participant.identity;
    if (name.isEmpty) return '?';
    final parts = name.split('_');
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return name.substring(0, name.length.clamp(0, 2)).toUpperCase();
  }
}
