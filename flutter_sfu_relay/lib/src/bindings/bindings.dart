/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-25 09:29:31
 */
/// FFI 绑定初始化
library;

import 'dart:ffi';
import 'dart:io';

import '../../flutter_sfu_relay_bindings_generated.dart';

export '../../flutter_sfu_relay_bindings_generated.dart'
    show FlutterSfuRelayBindings;

const String _libName = 'librelay';

/// 动态库
final DynamicLibrary dylib = () {
  if (Platform.isMacOS) {
    return DynamicLibrary.open('$_libName.dylib');
  }
  if (Platform.isIOS) {
    // iOS 使用静态库链接，符号已在主进程中，使用 process() 查找
    return DynamicLibrary.process();
  }
  if (Platform.isAndroid || Platform.isLinux) {
    return DynamicLibrary.open('$_libName.so');
  }
  if (Platform.isWindows) {
    try {
      final exeDir = File(Platform.resolvedExecutable).parent;
      final libFile = File(
        '${exeDir.path}${Platform.pathSeparator}librelay.dll',
      );

      if (libFile.existsSync()) {
        print('[ScreenShare] Loading librelay.dll from: ${libFile.path}');
        return DynamicLibrary.open(libFile.path);
      } else {
        print('[ScreenShare] librelay.dll NOT found in ${exeDir.path}');
        print('[ScreenShare] Listing directory contents:');
        try {
          exeDir.listSync().forEach((e) {
            print('  ${e.path.split(Platform.pathSeparator).last}');
          });
        } catch (_) {}
      }
    } catch (e) {
      print('[ScreenShare] Error determining library path: $e');
    }
    // Fallback
    return DynamicLibrary.open('$_libName.dll');
  }
  throw UnsupportedError('Unknown platform: ${Platform.operatingSystem}');
}();

/// FFI 绑定实例
final FlutterSfuRelayBindings bindings = FlutterSfuRelayBindings(dylib);
