import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Screen Share Helper for macOS and Windows
///
/// Provides functionality to:
/// - Exclude application window from screen capture
/// - Show/hide overlay UI during screen sharing (macOS)
///
/// Example usage:
/// ```dart
/// // When screen sharing starts
/// await ScreenShareHelper.setExcludeFromCapture(true);
/// await ScreenShareHelper.showOverlay(); // macOS only - shows floating toolbar
///
/// // When screen sharing ends
/// await ScreenShareHelper.setExcludeFromCapture(false);
/// await ScreenShareHelper.hideOverlay();
/// ```
class ScreenShareHelper {
  static const _channel = MethodChannel('com.flutter_sfu_relay.screen_share');

  /// Callback for when stop sharing is requested from native overlay
  static VoidCallback? onStopSharingRequested;

  /// Initialize the helper and set up method call handler
  /// Call this once at app startup
  static void initialize() {
    _channel.setMethodCallHandler((call) async {
      if (call.method == 'onStopSharingRequested') {
        debugPrint('[ScreenShareHelper] Stop sharing requested from native UI');
        onStopSharingRequested?.call();
      }
    });
  }

  /// Check if screen share helper is supported on this platform
  static bool get isSupported {
    if (kIsWeb) return false;
    return Platform.isMacOS || Platform.isWindows;
  }

  /// Set whether the application window should be excluded from screen capture
  ///
  /// When [exclude] is true:
  /// - macOS: Window will not appear in screen recordings or screenshots
  /// - Windows: Uses SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE)
  ///
  /// When [exclude] is false:
  /// - Window returns to normal and can be captured
  static Future<bool> setExcludeFromCapture(bool exclude) async {
    if (!isSupported) return false;
    try {
      final result = await _channel.invokeMethod<bool>(
        'setExcludeFromCapture',
        {'exclude': exclude},
      );
      return result ?? false;
    } catch (e) {
      debugPrint('[ScreenShareHelper] setExcludeFromCapture failed: $e');
      return false;
    }
  }

  /// Show screen sharing overlay UI
  ///
  /// On macOS: Shows a floating toolbar with "正在共享屏幕" status
  /// and a "结束共享" button, plus green corner borders
  ///
  /// On Windows: No-op (returns true for compatibility)
  static Future<bool> showOverlay() async {
    if (!isSupported) return false;
    try {
      final result = await _channel.invokeMethod<bool>('showOverlay');
      return result ?? false;
    } catch (e) {
      debugPrint('[ScreenShareHelper] showOverlay failed: $e');
      return false;
    }
  }

  /// Hide screen sharing overlay UI
  ///
  /// On macOS: Hides floating toolbar and green corner borders
  /// On Windows: No-op (returns true for compatibility)
  static Future<bool> hideOverlay() async {
    if (!isSupported) return false;
    try {
      final result = await _channel.invokeMethod<bool>('hideOverlay');
      return result ?? false;
    } catch (e) {
      debugPrint('[ScreenShareHelper] hideOverlay failed: $e');
      return false;
    }
  }
}
