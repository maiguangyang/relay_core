<!--
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24 14:40:01
-->
# Relay Core

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Pion WebRTC](https://img.shields.io/badge/Pion-WebRTC%20v4-blue?style=flat)](https://github.com/pion/webrtc)
[![Platform](https://img.shields.io/badge/Platform-Android%20|%20iOS%20|%20macOS%20|%20Windows%20|%20Linux-brightgreen?style=flat)]()
[![FFI Functions](https://img.shields.io/badge/FFI%20Functions-106-orange?style=flat)]()

基于 **Pion WebRTC** 的嵌入式微型 SFU 核心，专为 **Dart FFI** 集成设计，实现 RTP 数据包的**纯透传转发**（零解码），支持局域网代理模式和自动故障切换。

## ✨ 特性

- 🚀 **纯透传转发** - 零解码 RTP 包转发，最大化性能
- 🌐 **全平台支持** - Android、iOS、macOS、Windows、Linux
- 🔌 **Dart FFI 兼容** - C-shared 库导出，完美集成 Flutter
- 🤖 **一键自动代理** - Coordinator 模式一键启用自动选举和故障切换
- 👻 **影子连接** - Go 层直接连接 SFU 获取 RTP 包，解决 Flutter SDK 限制
- 🔄 **无感故障切换** - Relay 故障时自动重选举，用户无感知切换
- 🗳️ **动态代理选举** - 基于设备类型/网络质量的智能选举
- 🔀 **代理模式** - 双源切换，本地分享无缝切换
- 💾 **节省带宽** - 局域网代理模式可节省 95% 公网带宽

## 🚀 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 构建全平台

```bash
chmod +x build_all.sh
./build_all.sh
```

### 3. 构建产物

```
build/
├── android/jniLibs/arm64-v8a/librelay.so
├── ios/librelay.xcframework/
├── linux/librelay.so
├── macos/librelay.dylib
└── windows/librelay.dll
```

### 4. 一键启用自动代理 (推荐)

使用 `AutoCoordinator` 类，自动管理信令、选举和影子连接：

```dart
final autoCoord = AutoCoordinator(
  roomId: 'room-1',
  localPeerId: 'device-A',
  signaling: mySignaling,
  config: AutoCoordinatorConfig(
    autoElection: true,
    // 影子连接配置
    livekitUrl: 'wss://your-livekit.com',
    onRequestBotToken: (roomId) async {
      return await api.getBotToken(roomId);
    },
  ),
);

await autoCoord.start();
```

## 📖 文档

| 文档 | 说明 |
|------|------|
| [架构设计](docs/architecture.md) | 整体架构与模块设计 |
| [API 参考](docs/api-reference.md) | **106 个** FFI 函数完整列表 |
| [**自动代理模式**](docs/coordinator.md) | **一键启用自动选举和故障切换** |
| [**影子连接**](docs/shadow-connection.md) | **LiveKit 桥接与 RTP 转发机制** |
| [Relay P2P 管理](docs/relay-room.md) | RelayRoom 使用教程 |
| [代理模式](docs/proxy-mode.md) | SourceSwitcher 与双源切换 |
| [动态选举](docs/election.md) | 设备评分与代理选举 |
| [心跳保活](docs/keepalive.md) | 断线检测与重连 |
| [流量统计](docs/stats.md) | 码率/丢包率监控 |
| [Dart FFI 集成](docs/dart-ffi.md) | Flutter 集成指南 |
| [构建配置](docs/build.md) | 跨平台构建说明 |

## 📁 项目结构

```
relay_core/
├── main.go                  # 核心 FFI 入口
├── proxy_mode_ffi.go        # 代理模式 + Coordinator FFI
├── relay_room_ffi.go        # Relay 房间 FFI
├── livekit_bridge_ffi.go    # LiveKit 桥接 FFI (Shadow Connection)
├── keepalive_codec_ffi.go   # 心跳/编码 FFI
├── stats_probe_ffi.go       # 统计/探测 FFI
├── instance.go              # 实例管理
├── example/                 # 使用示例
│   └── basic/main.go
└── pkg/sfu/
    ├── coordinator.go       # 一键自动代理协调器
    ├── livekit_bridge.go    # LiveKit Go 客户端 (Shadow Connection)
    ├── failover.go          # 故障切换 + 冲突解决
    ├── relay_room.go        # Relay P2P 连接管理
    ├── source_switcher.go   # 双源切换器
    ├── keepalive.go         # 心跳保活
    ├── codec.go             # 编码协商
    ├── stats.go             # 流量统计
    ├── network_probe.go     # 网络探测
    ├── jitter_buffer.go     # 抖动缓冲（自适应延迟）
    └── buffer_pool.go       # 缓冲池
```

## 🧪 测试

```bash
# 运行所有测试
go test ./pkg/... -v

# 运行基准测试
go test ./pkg/... -bench=.

# 运行场景测试（真实 RTP 模拟）
go test ./pkg/sfu -run "Scenario" -v
```

## 📄 许可证

MIT License

## 📮 联系

- **Author:** Marlon.M
- **Email:** maiguangyang@163.com
