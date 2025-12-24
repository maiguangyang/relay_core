/// FFI 工具函数
library;

import 'dart:ffi';

import 'package:ffi/ffi.dart';

import 'bindings.dart';

/// Convert Dart string to C string (caller must free)
Pointer<Char> toCString(String s) => s.toNativeUtf8().cast<Char>();

/// Convert C string to Dart string and free
String fromCString(Pointer<Char> ptr) {
  if (ptr == nullptr) return '';
  final result = ptr.cast<Utf8>().toDartString();
  bindings.FreeString(ptr);
  return result;
}
