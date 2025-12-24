# Relay Core - 嵌入式微型 SFU

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Pion WebRTC](https://img.shields.io/badge/Pion-WebRTC%20v4-blue?style=flat)](https://github.com/pion/webrtc)
[![Platform](https://img.shields.io/badge/Platform-Android%20|%20iOS%20|%20macOS%20|%20Windows%20|%20Linux-brightgreen?style=flat)]()
[![License](https://img.shields.io/badge/License-MIT-yellow?style=flat)]()

基于 **Pion WebRTC** 的嵌入式微型 SFU（Selective Forwarding Unit）核心，专为 **Dart FFI** 集成设计，实现 RTP 数据包的**纯透传转发**（零解码），确保低功耗极致性能。

## ✨ 特性

- 🚀 **纯透传转发** - 零解码 RTP 包转发，最大化性能
- 🌐 **全平台支持** - Android、iOS、macOS、Windows、Linux
- 🔌 **Dart FFI 兼容** - C-shared 库导出，完美集成 Flutter
- 🗳️ **动态代理选举** - 基于网络质量评分的代理节点选举
- 📡 **事件回调系统** - 实时事件通知桥接 Go 与 Dart 层
- 💾 **节省带宽** - 局域网代理模式可节省 95% 公网带宽

## 📁 项目结构

```
relay_core/
├── main.go              # CGO 导出入口
├── proxy_mode.go        # 代理模式 FFI 导出
├── instance.go          # 实例管理（sync.Map 线程安全）
├── go.mod
├── go.sum
├── build_all.sh         # 全平台构建脚本
├── README.md
└── pkg/
    ├── sfu/
    │   ├── sfu.go           # SFU 核心引擎
    │   ├── room.go          # 房间管理
    │   ├── peer.go          # Peer 管理
    │   ├── forwarder.go     # RTP 转发器
    │   ├── source_switcher.go # 源切换器（代理模式核心）
    │   └── errors.go        # 错误定义
    ├── signaling/
    │   └── types.go         # 信令类型定义
    ├── election/
    │   └── election.go      # 增强代理选举（设备类型权重）
    └── utils/
        └── logger.go        # 日志工具
```

## 🏗️ 架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Flutter App (Dart)                        │
├─────────────────────────────────────────────────────────────┤
│                      Dart FFI Bridge                         │
├─────────────────────────────────────────────────────────────┤
│                  Go SFU Core (C-Shared Lib)                  │
│  ┌─────────────┬─────────────┬─────────────┬──────────────┐ │
│  │  C-Export   │ SFU Engine  │ RTP Forward │   Election   │ │
│  │    API      │   (Room)    │   (Peer)    │   (Proxy)    │ │
│  └─────────────┴─────────────┴─────────────┴──────────────┘ │
├─────────────────────────────────────────────────────────────┤
│                    Pion WebRTC v4                            │
└─────────────────────────────────────────────────────────────┘
```

## 🔧 环境要求

### 必需

- **Go 1.21+**
- **Xcode** (macOS/iOS 构建)
- **Android NDK** (Android 构建)

### 可选（用于全平台交叉编译）

```bash
# macOS 上安装交叉编译工具

# Windows 交叉编译
brew install mingw-w64

# Linux 交叉编译 (二选一)
brew install zig
# 或
brew install FiloSottile/musl-cross/musl-cross
```

## 🚀 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/maiguangyang/relay_core.git
cd relay_core
```

### 2. 安装依赖

```bash
go mod tidy
```

### 3. 构建全平台

```bash
chmod +x build_all.sh
./build_all.sh
```

### 4. 构建产物

```
build/
├── android/jniLibs/arm64-v8a/
│   ├── librelay.so          # Android 动态库
│   └── librelay.h
├── ios/librelay.xcframework/ # iOS 框架
├── linux/
│   ├── librelay.so          # Linux 动态库
│   └── librelay.h
├── macos/
│   ├── librelay.dylib       # macOS 动态库
│   └── librelay.h
└── windows/
    ├── librelay.dll         # Windows 动态库
    └── librelay.h
```

## 📖 API 参考

### 实例管理

| 函数 | 说明 | 返回值 |
|------|------|--------|
| `RelayCreate()` | 创建 SFU 实例 | `int64` 实例 ID |
| `RelayCreateWithConfig(iceServersJSON, debug)` | 使用配置创建实例 | `int64` 实例 ID |
| `RelayDestroy(relayID)` | 销毁实例 | `int` 0=成功, -1=失败 |
| `GetVersion()` | 获取版本号 | `char*` 版本字符串 |

### 房间管理

| 函数 | 说明 | 返回值 |
|------|------|--------|
| `RoomCreate(relayID, roomID)` | 创建房间 | `int` 0=成功 |
| `RoomDestroy(relayID, roomID)` | 销毁房间 | `int` 0=成功 |
| `RoomList(relayID)` | 列出所有房间 | `char*` JSON 数组 |
| `RoomInfo(relayID, roomID)` | 获取房间信息 | `char*` JSON 对象 |

### Peer 管理

| 函数 | 说明 | 返回值 |
|------|------|--------|
| `PeerAdd(relayID, roomID, peerID, offerSDP)` | 添加 Peer | `char*` Answer SDP |
| `PeerRemove(relayID, roomID, peerID)` | 移除 Peer | `int` 0=成功 |
| `PeerList(relayID, roomID)` | 列出房间内 Peer | `char*` JSON 数组 |
| `HandleICECandidate(relayID, roomID, peerID, candidateJSON)` | 添加 ICE 候选 | `int` 0=成功 |

### 代理选举（增强版）

| 函数 | 说明 | 返回值 |
|------|------|--------|
| `ElectionEnable(relayID, roomID)` | 启用选举 | `int` 0=成功 |
| `ElectionDisable(relayID, roomID)` | 禁用选举 | `int` 0=成功 |
| `ElectionUpdateDeviceInfo(relayID, roomID, peerID, deviceType, connType, powerState)` | 更新设备信息 | `int` 0=成功 |
| `ElectionUpdateNetworkMetrics(relayID, roomID, peerID, bandwidth, latency, packetLoss)` | 更新网络指标 | `int` 0=成功 |
| `ElectionTrigger(relayID, roomID)` | 手动触发选举 | `char*` 选举结果 JSON |
| `ElectionGetProxy(relayID, roomID)` | 获取当前代理 | `char*` Peer ID |
| `ElectionGetCandidates(relayID, roomID)` | 获取候选者列表 | `char*` JSON 数组 |

**设备类型 (deviceType)**:
- `0` = Unknown
- `1` = PC/Mac (Tier 1, 100分)
- `2` = Pad (Tier 3, 60分)
- `3` = Mobile (Tier 4, 40分)
- `4` = TV (90分)

**连接类型 (connectionType)**:
- `0` = Unknown
- `1` = Ethernet (×1.0)
- `2` = WiFi (×0.8)
- `3` = Cellular (×0.3)

**电源状态 (powerState)**:
- `0` = Unknown
- `1` = PluggedIn (×1.0)
- `2` = Battery (×0.7)
- `3` = LowBattery (×0.3)

### 代理模式 (Proxy Mode) 🆕

| 函数 | 说明 | 返回值 |
|------|------|--------|
| `ProxyModeInit(relayID, roomID)` | 初始化代理模式 | `int` 0=成功 |
| `ProxyModeCleanup(relayID, roomID)` | 清理代理模式 | `int` 0=成功 |
| `ProxyModeGetStatus(relayID, roomID)` | 获取综合状态 | `char*` JSON |
| `SourceSwitcherCreate(roomID)` | 创建源切换器 | `int` 0=成功 |
| `SourceSwitcherDestroy(roomID)` | 销毁源切换器 | `int` 0=成功 |
| `SourceSwitcherInjectSFU(roomID, isVideo, data, dataLen)` | 注入 SFU RTP 包 | `int` 0=成功 |
| `SourceSwitcherInjectLocal(roomID, isVideo, data, dataLen)` | 注入本地 RTP 包 | `int` 0=成功 |
| `SourceSwitcherStartLocalShare(roomID, sharerID)` | 开始本地分享 | `int` 0=成功 |
| `SourceSwitcherStopLocalShare(roomID)` | 停止本地分享 | `int` 0=成功 |
| `SourceSwitcherGetStatus(roomID)` | 获取切换器状态 | `char*` JSON |
| `SourceSwitcherIsLocalSharing(roomID)` | 是否正在本地分享 | `int` 1=是, 0=否 |

### 回调注册

| 函数 | 说明 |
|------|------|
| `SetEventCallback(callback)` | 设置事件回调 |
| `SetLogCallback(callback)` | 设置日志回调 |
| `SetLogLevel(level)` | 设置日志级别 (0=DEBUG, 1=INFO, 2=WARN, 3=ERROR) |

### 内存管理

| 函数 | 说明 |
|------|------|
| `FreeString(s)` | 释放由 Go 分配的字符串 |

## 📨 事件类型

```c
enum EventType {
    EVENT_PEER_JOINED   = 1,  // Peer 加入房间
    EVENT_PEER_LEFT     = 2,  // Peer 离开房间
    EVENT_TRACK_ADDED   = 3,  // 新增 Track
    EVENT_ERROR         = 4,  // 错误发生
    EVENT_ICE_CANDIDATE = 5,  // ICE 候选生成
    EVENT_PROXY_CHANGE  = 6,  // 代理切换
    EVENT_ANSWER        = 7,  // SDP Answer
    EVENT_OFFER         = 8,  // SDP Offer
};
```

## 🔌 Dart FFI 集成示例

### 加载动态库

```dart
import 'dart:ffi';
import 'package:ffi/ffi.dart';

// 平台特定加载
DynamicLibrary _loadLibrary() {
  if (Platform.isAndroid) {
    return DynamicLibrary.open('librelay.so');
  } else if (Platform.isIOS) {
    return DynamicLibrary.process(); // iOS 使用静态链接
  } else if (Platform.isMacOS) {
    return DynamicLibrary.open('librelay.dylib');
  } else if (Platform.isWindows) {
    return DynamicLibrary.open('librelay.dll');
  } else if (Platform.isLinux) {
    return DynamicLibrary.open('librelay.so');
  }
  throw UnsupportedError('Unsupported platform');
}

final _dylib = _loadLibrary();
```

### 绑定函数

```dart
// 类型定义
typedef RelayCreateNative = Int64 Function();
typedef RelayCreateDart = int Function();

typedef PeerAddNative = Pointer<Utf8> Function(
  Int64 relayId,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> offerSdp,
);
typedef PeerAddDart = Pointer<Utf8> Function(
  int relayId,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> offerSdp,
);

// 绑定
final relayCreate = _dylib.lookupFunction<RelayCreateNative, RelayCreateDart>('RelayCreate');
final peerAdd = _dylib.lookupFunction<PeerAddNative, PeerAddDart>('PeerAdd');
```

### 使用示例

```dart
// 创建 SFU 实例
final relayId = relayCreate();
print('Relay created: $relayId');

// 添加 Peer
final roomId = 'room-1'.toNativeUtf8();
final peerId = 'peer-1'.toNativeUtf8();
final offer = sdpOffer.toNativeUtf8();

final answerPtr = peerAdd(relayId, roomId, peerId, offer);
final answer = answerPtr.toDartString();

// 释放内存
calloc.free(roomId);
calloc.free(peerId);
calloc.free(offer);
freeString(answerPtr); // 使用 Go 的 FreeString
```

### 回调处理

```dart
// 定义回调类型
typedef EventCallbackNative = Void Function(
  Int32 eventType,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> data,
);
typedef EventCallbackDart = void Function(
  int eventType,
  Pointer<Utf8> roomId,
  Pointer<Utf8> peerId,
  Pointer<Utf8> data,
);

// 创建回调
void _onEvent(int eventType, Pointer<Utf8> roomId, Pointer<Utf8> peerId, Pointer<Utf8> data) {
  print('Event: type=$eventType, room=${roomId.toDartString()}, peer=${peerId.toDartString()}');
}

// 注册回调
final callback = Pointer.fromFunction<EventCallbackNative>(_onEvent);
setEventCallback(callback);
```

## 🏃 运行流程

```
1. Dart 调用 RelayCreate() 创建 SFU 实例
2. Dart 注册 SetEventCallback() 接收事件
3. Peer A 发送 Offer → Dart 调用 PeerAdd() → 返回 Answer
4. Dart 通过信令服务器交换 SDP
5. ICE Candidate 通过 HandleICECandidate() 添加
6. 连接建立后，Go 自动进行 RTP 纯透传转发
7. 新 Track 添加时触发 EVENT_TRACK_ADDED 回调
8. 代理选举启用后自动根据网络质量选择最优节点
```

## ⚙️ 构建配置

### Android NDK 路径

脚本会自动检测 NDK 路径，也可手动设置：

```bash
# 在 build_all.sh 中修改
ANDROID_NDK_HOME=/path/to/your/ndk
```

### 最小 API 级别

默认 Android API Level 24，iOS 最低版本 12.0。可在 `build_all.sh` 中调整：

```bash
# Android
API=24

# iOS
-miphoneos-version-min=12.0
```

## 🐛 常见问题

### Q: Android 构建报错 `invalid reference to net.zoneCache`

**A:** 这是 Go 1.23+ 的 linkname 限制问题。构建脚本已添加 `-checklinkname=0` 修复。

### Q: iOS 构建报错 `stdlib.h not found`

**A:** 需要正确设置 SDK sysroot。脚本已自动处理。

### Q: 如何减小库体积？

**A:** 构建脚本默认使用 `-ldflags="-s -w"` 去除调试信息。进一步压缩可使用 UPX：

```bash
upx --best build/android/jniLibs/arm64-v8a/librelay.so
```

## 📄 许可证

MIT License

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📮 联系

- **Author:** Marlon.M
- **Email:** maiguangyang@163.com
