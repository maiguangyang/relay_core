import 'package:flutter/material.dart';
import '../theme/app_theme.dart';

/// 控制栏按钮状态
class ControlState {
  final bool micEnabled;
  final bool cameraEnabled;
  final bool screenShareEnabled;
  final bool isRelay;

  const ControlState({
    this.micEnabled = false,
    this.cameraEnabled = false,
    this.screenShareEnabled = false,
    this.isRelay = false,
  });

  ControlState copyWith({
    bool? micEnabled,
    bool? cameraEnabled,
    bool? screenShareEnabled,
    bool? isRelay,
  }) {
    return ControlState(
      micEnabled: micEnabled ?? this.micEnabled,
      cameraEnabled: cameraEnabled ?? this.cameraEnabled,
      screenShareEnabled: screenShareEnabled ?? this.screenShareEnabled,
      isRelay: isRelay ?? this.isRelay,
    );
  }
}

/// 底部控制栏
class ControlBar extends StatelessWidget {
  final ControlState state;
  final VoidCallback? onMicToggle;
  final VoidCallback? onCameraToggle;
  final VoidCallback? onScreenShareToggle;
  final VoidCallback? onLeave;

  const ControlBar({
    super.key,
    required this.state,
    this.onMicToggle,
    this.onCameraToggle,
    this.onScreenShareToggle,
    this.onLeave,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      decoration: BoxDecoration(
        color: AppTheme.surfaceDark.withOpacity(0.9),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(24)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withOpacity(0.3),
            blurRadius: 20,
            offset: const Offset(0, -5),
          ),
        ],
      ),
      child: SafeArea(
        top: false,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceEvenly,
          children: [
            // 麦克风
            _buildControlButton(
              icon: state.micEnabled ? Icons.mic : Icons.mic_off,
              label: '麦克风',
              isEnabled: state.micEnabled,
              activeColor: AppTheme.onlineColor,
              onTap: onMicToggle,
            ),

            // 摄像头
            _buildControlButton(
              icon: state.cameraEnabled ? Icons.videocam : Icons.videocam_off,
              label: '摄像头',
              isEnabled: state.cameraEnabled,
              activeColor: AppTheme.onlineColor,
              onTap: onCameraToggle,
            ),

            // 屏幕共享
            _buildControlButton(
              icon: Icons.screen_share,
              label: '共享',
              isEnabled: state.screenShareEnabled,
              activeColor: AppTheme.secondaryColor,
              onTap: onScreenShareToggle,
            ),

            // 挂断
            _buildControlButton(
              icon: Icons.call_end,
              label: '离开',
              isEnabled: true,
              activeColor: AppTheme.accentColor,
              backgroundColor: AppTheme.accentColor,
              onTap: onLeave,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildControlButton({
    required IconData icon,
    required String label,
    required bool isEnabled,
    required Color activeColor,
    Color? backgroundColor,
    VoidCallback? onTap,
  }) {
    return GestureDetector(
      onTap: onTap,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(
              color:
                  backgroundColor ??
                  (isEnabled
                      ? activeColor.withOpacity(0.2)
                      : AppTheme.cardDark),
              borderRadius: BorderRadius.circular(16),
              border: isEnabled && backgroundColor == null
                  ? Border.all(color: activeColor, width: 2)
                  : null,
            ),
            child: Icon(
              icon,
              color: backgroundColor != null
                  ? Colors.white
                  : (isEnabled ? activeColor : AppTheme.textSecondary),
              size: 28,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            label,
            style: TextStyle(
              color: isEnabled ? AppTheme.textPrimary : AppTheme.textSecondary,
              fontSize: 12,
            ),
          ),
        ],
      ),
    );
  }
}
