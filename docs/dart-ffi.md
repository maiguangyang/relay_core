# Dart FFI 集成

## 概述

本指南介绍如何在 Flutter 项目中通过 FFI 调用 Relay Core 库。

## 1. 添加依赖

```yaml
# pubspec.yaml
dependencies:
  ffi: ^2.1.0
```

## 2. 加载动态库

```dart
// lib/relay_core/relay_bindings.dart

import 'dart:ffi';
import 'dart:io';

final DynamicLibrary _lib = _loadLibrary();

DynamicLibrary _loadLibrary() {
  if (Platform.isAndroid) {
    return DynamicLibrary.open('librelay.so');
  } else if (Platform.isIOS) {
    return DynamicLibrary.process();
  } else if (Platform.isMacOS) {
    return DynamicLibrary.open('librelay.dylib');
  } else if (Platform.isWindows) {
    return DynamicLibrary.open('librelay.dll');
  } else if (Platform.isLinux) {
    return DynamicLibrary.open('librelay.so');
  }
  throw UnsupportedError('Unsupported platform');
}
```

## 3. 定义类型

```dart
// 回调类型
typedef EventCallbackNative = Void Function(
  Int32 eventType,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> data
);
typedef EventCallback = void Function(
  int eventType,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> data
);

typedef LogCallbackNative = Void Function(Int32 level, Pointer<Utf8> message);
typedef LogCallback = void Function(int level, Pointer<Utf8> message);
```

## 4. 绑定函数

```dart
// RelayRoom 函数
typedef RelayRoomCreateNative = Int32 Function(
  Pointer<Utf8> roomId,
  Pointer<Utf8> iceServersJson
);
typedef RelayRoomCreateDart = int Function(
  Pointer<Utf8> roomId,
  Pointer<Utf8> iceServersJson
);

final relayRoomCreate = _lib
    .lookup<NativeFunction<RelayRoomCreateNative>>('RelayRoomCreate')
    .asFunction<RelayRoomCreateDart>();

typedef RelayRoomDestroyNative = Int32 Function(Pointer<Utf8> roomId);
typedef RelayRoomDestroyDart = int Function(Pointer<Utf8> roomId);

final relayRoomDestroy = _lib
    .lookup<NativeFunction<RelayRoomDestroyNative>>('RelayRoomDestroy')
    .asFunction<RelayRoomDestroyDart>();

// ... 其他函数类似
```

## 5. 完整绑定类

```dart
// lib/relay_core/relay_bindings.dart

import 'dart:convert';
import 'dart:ffi';
import 'dart:io';
import 'package:ffi/ffi.dart';

class RelayCore {
  static final RelayCore instance = RelayCore._();
  late final DynamicLibrary _lib;
  
  RelayCore._() {
    _lib = _loadLibrary();
    _initBindings();
  }
  
  // === RelayRoom ===
  late final int Function(Pointer<Utf8>, Pointer<Utf8>) _relayRoomCreate;
  late final int Function(Pointer<Utf8>) _relayRoomDestroy;
  late final Pointer<Utf8> Function(Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>) _relayRoomAddSubscriber;
  late final int Function(Pointer<Utf8>, Pointer<Utf8>) _relayRoomRemoveSubscriber;
  late final int Function(Pointer<Utf8>, int, Pointer<Void>, int) _relayRoomInjectSFU;
  late final int Function(Pointer<Utf8>, Pointer<Utf8>) _relayRoomStartLocalShare;
  late final int Function(Pointer<Utf8>) _relayRoomStopLocalShare;
  
  // === Election ===
  late final int Function(int, Pointer<Utf8>) _electionEnable;
  late final int Function(int, Pointer<Utf8>, Pointer<Utf8>, int, int, int) _electionUpdateDeviceInfo;
  late final Pointer<Utf8> Function(int, Pointer<Utf8>) _electionTrigger;
  
  // === Callbacks ===
  late final void Function(Pointer<NativeFunction<EventCallbackNative>>) _setEventCallback;
  late final void Function(Pointer<Utf8>) _freeString;
  
  void _initBindings() {
    _relayRoomCreate = _lib
        .lookup<NativeFunction<Int32 Function(Pointer<Utf8>, Pointer<Utf8>)>>('RelayRoomCreate')
        .asFunction();
        
    _relayRoomDestroy = _lib
        .lookup<NativeFunction<Int32 Function(Pointer<Utf8>)>>('RelayRoomDestroy')
        .asFunction();
        
    // ... 其他绑定
    
    _freeString = _lib
        .lookup<NativeFunction<Void Function(Pointer<Utf8>)>>('FreeString')
        .asFunction();
  }
  
  // === 公开 API ===
  
  bool createRelayRoom(String roomId, List<Map<String, dynamic>> iceServers) {
    final roomIdPtr = roomId.toNativeUtf8();
    final icePtr = jsonEncode(iceServers).toNativeUtf8();
    
    try {
      return _relayRoomCreate(roomIdPtr, icePtr) == 0;
    } finally {
      malloc.free(roomIdPtr);
      malloc.free(icePtr);
    }
  }
  
  void destroyRelayRoom(String roomId) {
    final ptr = roomId.toNativeUtf8();
    try {
      _relayRoomDestroy(ptr);
    } finally {
      malloc.free(ptr);
    }
  }
  
  String? addSubscriber(String roomId, String peerId, String offerSdp) {
    final roomIdPtr = roomId.toNativeUtf8();
    final peerIdPtr = peerId.toNativeUtf8();
    final offerPtr = offerSdp.toNativeUtf8();
    
    try {
      final resultPtr = _relayRoomAddSubscriber(roomIdPtr, peerIdPtr, offerPtr);
      if (resultPtr == nullptr) return null;
      
      final result = resultPtr.toDartString();
      _freeString(resultPtr);
      return result;
    } finally {
      malloc.free(roomIdPtr);
      malloc.free(peerIdPtr);
      malloc.free(offerPtr);
    }
  }
  
  void injectSfuPacket(String roomId, bool isVideo, Uint8List data) {
    final roomIdPtr = roomId.toNativeUtf8();
    final dataPtr = malloc<Uint8>(data.length);
    dataPtr.asTypedList(data.length).setAll(0, data);
    
    try {
      _relayRoomInjectSFU(roomIdPtr, isVideo ? 1 : 0, dataPtr.cast(), data.length);
    } finally {
      malloc.free(roomIdPtr);
      malloc.free(dataPtr);
    }
  }
  
  // ... 更多方法
}
```

## 6. 设置回调

```dart
// 全局回调处理
void _eventCallbackHandler(
  int eventType,
  Pointer<Utf8> roomIdPtr,
  Pointer<Utf8> peerIdPtr,
  Pointer<Utf8> dataPtr
) {
  final roomId = roomIdPtr.toDartString();
  final peerId = peerIdPtr.toDartString();
  final data = dataPtr.toDartString();
  
  // 在主隔离区处理事件
  ReceivePort().sendPort.send({
    'type': eventType,
    'roomId': roomId,
    'peerId': peerId,
    'data': data,
  });
}

void initRelayCore() {
  final callbackPtr = Pointer.fromFunction<EventCallbackNative>(
    _eventCallbackHandler
  );
  RelayCore.instance.setEventCallback(callbackPtr);
}
```

## 7. 平台配置

### Android

```gradle
// android/app/build.gradle
android {
    defaultConfig {
        ndk {
            abiFilters 'arm64-v8a'
        }
    }
}
```

将 `librelay.so` 复制到 `android/app/src/main/jniLibs/arm64-v8a/`

### iOS

1. 将 `librelay.xcframework` 拖入 Xcode 项目
2. 确保 Framework 的 Embed 设置为 "Embed & Sign"

### macOS

将 `librelay.dylib` 复制到 `macos/Runner/` 并添加到 Xcode 项目

### Windows

将 `librelay.dll` 放到应用程序目录或系统 PATH 中

### Linux

将 `librelay.so` 放到 `/usr/local/lib/` 或应用目录

## 8. 完整示例

```dart
import 'package:flutter/material.dart';

class RelayRoomManager {
  final RelayCore _core = RelayCore.instance;
  final String roomId;
  
  RelayRoomManager(this.roomId);
  
  Future<void> initialize() async {
    // 创建房间
    _core.createRelayRoom(roomId, [
      {'urls': ['stun:stun.l.google.com:19302']}
    ]);
    
    // 启用选举
    _core.enableElection(0, roomId);
    
    // 监听事件
    _core.eventStream.listen(_handleEvent);
  }
  
  void _handleEvent(RelayEvent event) {
    switch (event.type) {
      case 10: // 订阅者加入
        print('新订阅者: ${event.peerId}');
        break;
      case 6: // 代理变更
        _handleProxyChange(event);
        break;
      case 22: // Peer 离线
        _handlePeerOffline(event.peerId);
        break;
    }
  }
  
  void _handleProxyChange(RelayEvent event) {
    final info = jsonDecode(event.data);
    if (info['proxy_id'] == myPeerId) {
      _core.becomeRelay(roomId, myPeerId);
    }
  }
  
  void dispose() {
    _core.destroyRelayRoom(roomId);
  }
}
```
