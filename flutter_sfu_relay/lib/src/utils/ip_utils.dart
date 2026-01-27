import 'dart:async';
import 'package:flutter_webrtc/flutter_webrtc.dart';

/// Utility class for IP operations
class IpUtils {
  static String? _cachedPublicIp;

  /// Get the public IP address of the current device using WebRTC STUN.
  /// This avoids dependencies on external HTTP services (like ipify).
  static Future<String?> getPublicIp() async {
    if (_cachedPublicIp != null) return _cachedPublicIp;

    RTCPeerConnection? pc;
    try {
      final completer = Completer<String?>();

      // 1. Create RTCPeerConnection with Google's STUN server
      // STUN servers are lightweight and standardized for this exact purpose (NAT discovery)
      final config = {
        'iceServers': [
          {'urls': 'stun:stun.l.google.com:19302'},
        ],
        'sdpSemantics': 'unified-plan',
      };

      pc = await createPeerConnection(config);

      // 2. Create a dummy DataChannel to trigger ICE gathering
      // We don't need to actually send data, just start the negotiation
      await pc.createDataChannel('ip_discovery', RTCDataChannelInit());

      // 3. Listen for ICE candidates
      pc.onIceCandidate = (RTCIceCandidate candidate) {
        if (candidate.candidate == null) return;

        final candStr = candidate.candidate!;

        // Search for "srflx" (Server Reflexive) candidates
        // Format: candidate:842163049 1 udp 1677729535 203.0.113.10 55432 typ srflx raddr ...
        if (candStr.contains('typ srflx')) {
          final parts = candStr.split(' ');
          // Find the IP address: usually following priority, or explicitly parse
          // Standard structure: foundation component protocol priority ip port typ type ...
          // Typically IP is at index 4 (0-based) for srflx

          for (int i = 0; i < parts.length; i++) {
            // Simple heuristic to find the IP: look for the token before "typ" that looks like an IP?
            // Or simpler: Standard RFC 5245 format puts IP at index 4
            if (i >= 4 &&
                _isValidIp(parts[i]) &&
                parts.length > i + 2 &&
                parts[i + 2] == 'srflx') {
              if (!completer.isCompleted) {
                _cachedPublicIp = parts[i];
                completer.complete(_cachedPublicIp);
              }
              return;
            }
          }

          // Fallback parsing if standard format varies slightly (e.g. Chrome vs Firefox format quirks)
          // Look for the token immediately preceding the port number (which is numeric)
          // This is harder to robustly regex without assumptions.
          // Let's stick to index 4 as per RFC 5245 which webrtc implementations follow.
        }
      };

      // 4. Create Offer and SetLocalDescription to start the process
      final offer = await pc.createOffer({});
      await pc.setLocalDescription(offer);

      // 5. Wait with timeout
      // ICE gathering usually takes < 1s
      return await completer.future.timeout(
        const Duration(seconds: 2),
        onTimeout: () => null,
      );
    } catch (e) {
      print('[IpUtils] Failed to get public IP via STUN: $e');
      return null;
    } finally {
      await pc?.close();
      await pc?.dispose();
    }
  }

  static bool _isValidIp(String ip) {
    // Basic IPv4 validation
    final parts = ip.split('.');
    if (parts.length != 4) return false;
    for (final part in parts) {
      final num = int.tryParse(part);
      if (num == null || num < 0 || num > 255) return false;
    }
    return true;
  }
}
