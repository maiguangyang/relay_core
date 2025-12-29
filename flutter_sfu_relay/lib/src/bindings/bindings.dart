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
    print('Current Dir: ${Directory.current.path}');
    if (!File('librelay.dll').existsSync()) {
      print('librelay.dll NOT found!');
      try {
        Directory.current.listSync().forEach(
          (e) => print('  ${e.path.split(Platform.pathSeparator).last}'),
        );
      } catch (e) {
        print('List failed: $e');
      }
    } else {
      print('librelay.dll found.');
    }
    return DynamicLibrary.open('$_libName.dll');
  }
  throw UnsupportedError('Unknown platform: ${Platform.operatingSystem}');
}();

/// FFI 绑定实例
final FlutterSfuRelayBindings bindings = FlutterSfuRelayBindings(dylib);
