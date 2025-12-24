/// SFU Relay 主入口
library;

import 'dart:convert';

import '../bindings/bindings.dart';
import '../bindings/utils.dart';

/// SFU Relay 主入口类
class SfuRelay {
  SfuRelay._();
  static final SfuRelay instance = SfuRelay._();

  /// 获取版本号
  String get version => fromCString(bindings.GetVersion());

  /// 设置日志级别 (0=Debug, 1=Info, 2=Warn, 3=Error)
  void setLogLevel(int level) => bindings.SetLogLevel(level);

  /// 清理所有资源
  void cleanupAll() => bindings.CleanupAll();

  /// 获取支持的视频编解码器
  List<String> getSupportedVideoCodecs() {
    final json = fromCString(bindings.CodecGetSupportedVideo());
    if (json.isEmpty) return [];
    return List<String>.from(jsonDecode(json));
  }

  /// 获取支持的音频编解码器
  List<String> getSupportedAudioCodecs() {
    final json = fromCString(bindings.CodecGetSupportedAudio());
    if (json.isEmpty) return [];
    return List<String>.from(jsonDecode(json));
  }
}
