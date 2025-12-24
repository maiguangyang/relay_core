# 代理模式 (Proxy Mode)

## 概述

代理模式是 Relay Core 的核心特性，实现了**双输入源无缝切换**：

- **Input A (SFU)**: 来自远端 LiveKit SFU 的流
- **Input B (本地分享)**: 来自局域网内分享者的流

切换时订阅者**无需重新连接**，仅需 SDP 重协商。

## 工作原理

```
┌──────────────┐     ┌──────────────┐
│   SFU 流     │     │  本地分享者   │
│  (Input A)   │     │   (Input B)   │
└──────┬───────┘     └───────┬──────┘
       │                     │
       └──────────┬──────────┘
                  ▼
         ┌───────────────┐
         │ SourceSwitcher │  ← 自动选择活跃源
         └───────┬───────┘
                 │
    ┌────────────┼────────────┐
    ▼            ▼            ▼
订阅者1       订阅者2       订阅者3
```

## 使用教程

### 1. 初始化代理模式

```dart
// 方式一：一键初始化（推荐）
final relayId = 0; // 如果使用 RelayRoom 可以传 0
proxyModeInit(relayId, "room-123".toNativeUtf8());

// 方式二：分步初始化
sourceSwitcherCreate("room-123".toNativeUtf8());
electionEnable(0, "room-123".toNativeUtf8());
```

### 2. 正常观看（注入 SFU 流）

从 LiveKit 获取 RTP 包后注入：

```dart
// LiveKit Track 回调中
void onRtpPacket(Uint8List rtpData, bool isVideo) {
  sourceSwitcherInjectSFU(
    "room-123".toNativeUtf8(),
    isVideo ? 1 : 0,
    rtpData.cast<Void>(),
    rtpData.length
  );
}
```

### 3. 开始本地分享

当用户发起屏幕/摄像头分享：

```dart
// 1. 通知 Relay 开始本地分享
sourceSwitcherStartLocalShare(
  "room-123".toNativeUtf8(),
  "sharer-id".toNativeUtf8()
);

// 2. 此后注入本地分享者的 RTP 包
void onLocalRtpPacket(Uint8List rtpData, bool isVideo) {
  sourceSwitcherInjectLocal(
    "room-123".toNativeUtf8(),
    isVideo ? 1 : 0,
    rtpData.cast<Void>(),
    rtpData.length
  );
}
```

### 4. 停止本地分享

```dart
// 停止分享，自动切回 SFU 流
sourceSwitcherStopLocalShare("room-123".toNativeUtf8());
```

### 5. 获取状态

```dart
final statusPtr = sourceSwitcherGetStatus("room-123".toNativeUtf8());
final status = jsonDecode(statusPtr.toDartString());

print("当前源: ${status['current_source']}");  // "SFU" 或 "Local"
print("本地分享者: ${status['local_sharer_id']}");
print("是否正在分享: ${status['is_local_sharing']}");
```

### 6. 清理

```dart
proxyModeCleanup(0, "room-123".toNativeUtf8());

// 或分步清理
sourceSwitcherDestroy("room-123".toNativeUtf8());
electionDisable(0, "room-123".toNativeUtf8());
```

## 与 RelayRoom 集成

RelayRoom 内置了 SourceSwitcher，可以直接使用便捷方法：

```dart
// 通过 RelayRoom 注入（自动转发给所有订阅者）
relayRoomInjectSFU(roomId, isVideo, data, len);
relayRoomInjectLocal(roomId, isVideo, data, len);
relayRoomStartLocalShare(roomId, sharerId);
relayRoomStopLocalShare(roomId);
```

## 事件处理

### 源切换回调

```dart
// 设置源切换回调（在 SourceSwitcher 创建时配置）
// 通过 EventCallback 接收事件

void handleEvent(int type, String roomId, String peerId, String data) {
  if (type == 6) { // EventTypeProxyChange
    final info = jsonDecode(data);
    print("代理变更: ${info['proxy_id']}");
    print("源类型: ${info['source_type']}");
  }
}
```

## 场景示例

### 场景 1: 主持人屏幕分享

```dart
// 1. 主持人发起分享
localShare.start();

// 2. 通知 Relay 切换源
sourceSwitcherStartLocalShare(roomId, myPeerId);

// 3. 注入分享数据
localShare.onRtpPacket = (data) {
  sourceSwitcherInjectLocal(roomId, 1, data, data.length);
};

// 4. 结束分享
localShare.stop();
sourceSwitcherStopLocalShare(roomId);
// 自动切回观看 SFU 流
```

### 场景 2: 切换分享者

```dart
// 分享者 A 结束
sourceSwitcherStopLocalShare(roomId);

// 分享者 B 开始（无缝切换）
sourceSwitcherStartLocalShare(roomId, "sharer-b");
```

## 注意事项

1. **RTP 格式**：确保注入的是完整 RTP 包（包含 RTP Header）
2. **编码一致**：本地分享的编码应与 SFU 流一致（如都用 VP8）
3. **重协商**：切换时会自动触发 SDP 重协商
4. **带宽**：本地分享不经过公网，节省带宽
