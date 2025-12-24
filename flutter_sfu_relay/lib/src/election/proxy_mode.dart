/// ProxyMode - 代理模式便捷函数
///
/// 组合初始化 SourceSwitcher + Election 的便捷 API
library;

import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// 代理模式状态
class ProxyModeStatus {
  final bool sourceSwitcherActive;
  final bool electionActive;
  final String currentSource;
  final String? currentProxy;
  final bool isLocalSharing;

  ProxyModeStatus({
    required this.sourceSwitcherActive,
    required this.electionActive,
    required this.currentSource,
    this.currentProxy,
    required this.isLocalSharing,
  });

  factory ProxyModeStatus.fromJson(Map<String, dynamic> json) {
    return ProxyModeStatus(
      sourceSwitcherActive: json['source_switcher_active'] ?? false,
      electionActive: json['election_active'] ?? false,
      currentSource: json['current_source'] ?? 'SFU',
      currentProxy: json['current_proxy'],
      isLocalSharing: json['is_local_sharing'] ?? false,
    );
  }

  @override
  String toString() =>
      'ProxyMode(source: $currentSource, proxy: $currentProxy)';
}

/// ProxyMode - 代理模式管理器
///
/// 便捷的代理模式管理，一键初始化 SourceSwitcher + Election
///
/// ```dart
/// final proxyMode = ProxyMode(roomId: 'room-1');
///
/// // 一键初始化
/// proxyMode.init();
///
/// // 获取综合状态
/// final status = proxyMode.getStatus();
/// print('当前源: ${status?.currentSource}');
///
/// // 清理
/// proxyMode.cleanup();
/// ```
class ProxyMode {
  final String roomId;
  final int relayId;

  ProxyMode({required this.roomId, this.relayId = 0});

  /// 初始化代理模式
  ///
  /// 同时创建 SourceSwitcher 并启用 Election
  bool init() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.ProxyModeInit(relayId, roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 清理代理模式
  ///
  /// 同时销毁 SourceSwitcher 并禁用 Election
  bool cleanup() {
    final roomPtr = toCString(roomId);
    try {
      return bindings.ProxyModeCleanup(relayId, roomPtr) == 0;
    } finally {
      calloc.free(roomPtr);
    }
  }

  /// 获取代理模式综合状态
  ProxyModeStatus? getStatus() {
    final roomPtr = toCString(roomId);
    try {
      final resultPtr = bindings.ProxyModeGetStatus(relayId, roomPtr);
      if (resultPtr == nullptr) return null;
      final json = fromCString(resultPtr);
      if (json.isEmpty) return null;
      return ProxyModeStatus.fromJson(jsonDecode(json));
    } finally {
      calloc.free(roomPtr);
    }
  }
}
