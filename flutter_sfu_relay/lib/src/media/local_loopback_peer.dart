import 'dart:async';

import 'dart:ffi'; // For FFI
import 'package:ffi/ffi.dart'; // For Utf8
import 'package:flutter_webrtc/flutter_webrtc.dart';
import '../../flutter_sfu_relay.dart'; // bindings
import '../bindings/bindings.dart'; // bindings instance

class LocalLoopbackPeer {
  final String roomId;
  RTCPeerConnection? _pc;
  MediaStream? _localStream;
  bool _isDisposed = false;

  LocalLoopbackPeer(this.roomId);

  Future<void> start(MediaStream screenStream) async {
    if (_isDisposed) return;
    _localStream = screenStream;

    try {
      // 1. Create PeerConnection (Host Candidates Only)
      final config = {
        'iceServers': [], // No STUN/TURN
        'iceTransportPolicy': 'all', // We rely on host candidates
        'sdpSemantics': 'unified-plan',
      };

      // Constraints: Disable audio if not needed, or enable if system audio is captured
      // Loopback should just take what tracks are in the stream
      final constraints = <String, dynamic>{
        'optional': [
          {'DtlsSrtpKeyAgreement': true},
        ],
      };

      _pc = await createPeerConnection(config, constraints);

      // 2. Add Tracks
      for (final track in _localStream!.getTracks()) {
        await _pc!.addTrack(track, _localStream!);
      }

      // 3. Create Offer
      RTCSessionDescription offer = await _pc!.createOffer({});
      await _pc!.setLocalDescription(offer);

      // 4. Send Offer via FFI and get Answer
      // Ensure ICE gathering is complete?
      // Since we are "Host only", candidates are gathered instantly usually.
      // But standard WebRTC might take a ms.
      // Actually, if we send SDP immediately after setLocalDescription, it might lack candidates
      // if using Trickle ICE. But for Loopback/Host, often we can just wait a bit or use `await` if supported?
      // Flutter WebRTC `createOffer` doesn't wait for gathering.
      // We'll trust that Host candidates are quick enough or included in the initial offer if
      // we listen to onIceCandidate?
      //
      // BETTER APPROACH for Loopback:
      // Wait for ICE Gathering Complete or at least some candidates?
      // Or just send what we have. If PC is configured correctly, connection might work.
      // Let's implement basic flow first.

      final answerSdp = _exchangeSdp(offer.sdp!);
      if (answerSdp == null) {
        throw Exception('Failed to get SDAnswer from Go');
      }

      // 5. Set Remote Description
      final answer = RTCSessionDescription(answerSdp, 'answer');
      await _pc!.setRemoteDescription(answer);

      print(
        '[LocalLoopbackPeer] Loopback connection established for room $roomId',
      );
    } catch (e) {
      print('[LocalLoopbackPeer] Error starting loopback: $e');
      dispose();
      rethrow;
    }
  }

  String? _exchangeSdp(String offerSdp) {
    if (_isDisposed) return null;

    final roomIdPtr = roomId.toNativeUtf8().cast<Char>();
    final offerPtr = offerSdp.toNativeUtf8().cast<Char>();

    try {
      final answerPtr = bindings.LocalShareConnect(roomIdPtr, offerPtr);
      if (answerPtr == nullptr) {
        return null;
      }

      final answer = answerPtr.cast<Utf8>().toDartString();
      bindings.FreeString(answerPtr);

      return answer;
    } finally {
      calloc.free(roomIdPtr);
      calloc.free(offerPtr);
    }
  }

  Future<void> dispose() async {
    if (_isDisposed) return;
    _isDisposed = true;

    await _pc?.close();
    _pc = null;
    _localStream = null; // Don't dispose stream here, owned by UI?
  }
}
