# 流量统计

## 概述

Stats 模块提供实时流量监控能力：

- 累计字节数/包数统计
- 实时码率计算（bps）
- 丢包率统计
- 每个 Peer 独立统计

## 使用教程

### 1. 创建统计

```dart
statsCreate("room-123".toNativeUtf8());
```

### 2. 记录流量

在转发 RTP 包时记录：

```dart
void forwardRtpPacket(String peerId, Uint8List data) {
  // 记录入站
  statsAddBytesIn(
    "room-123".toNativeUtf8(),
    "".toNativeUtf8(),  // 空字符串表示只更新总体统计
    data.length
  );
  
  // 转发给订阅者...
  
  // 记录出站（每个订阅者）
  for (final subscriber in subscribers) {
    statsAddBytesOut(
      "room-123".toNativeUtf8(),
      subscriber.toNativeUtf8(),
      data.length
    );
  }
}
```

### 3. 记录丢包

```dart
statsAddPacketLost("room-123".toNativeUtf8());
```

### 4. 计算码率

每秒调用一次：

```dart
Timer.periodic(Duration(seconds: 1), (_) {
  statsCalculateBitrate("room-123".toNativeUtf8());
});
```

### 5. 获取统计

```dart
// 获取入站码率 (bps)
final bitrateIn = statsGetBitrateIn("room-123".toNativeUtf8());
print("入站: ${(bitrateIn / 1000000).toStringAsFixed(2)} Mbps");

// 获取出站码率
final bitrateOut = statsGetBitrateOut("room-123".toNativeUtf8());

// 获取丢包率
final lossRate = statsGetLossRate("room-123".toNativeUtf8());
print("丢包率: ${(lossRate * 100).toStringAsFixed(2)}%");

// 获取完整快照
final snapshotPtr = statsGetSnapshot("room-123".toNativeUtf8());
final snapshot = jsonDecode(snapshotPtr.toDartString());
print("""
总入站: ${snapshot['traffic']['total_bytes_in']} bytes
总出站: ${snapshot['traffic']['total_bytes_out']} bytes
入站包数: ${snapshot['traffic']['total_packets_in']}
出站包数: ${snapshot['traffic']['total_packets_out']}
丢包数: ${snapshot['traffic']['packets_lost']}
""");
```

### 6. 获取工流量统计

```dart
final trafficPtr = statsGetTraffic("room-123".toNativeUtf8());
final traffic = jsonDecode(trafficPtr.toDartString());
```

### 7. 清理

```dart
statsDestroy("room-123".toNativeUtf8());
```

---

## 缓冲池统计

监控内存复用效率：

```dart
final poolPtr = bufferPoolGetStats();
final pool = jsonDecode(poolPtr.toDartString());

print("标准分配次数: ${pool['standard_allocs']}");
print("标准复用次数: ${pool['standard_reuses']}");
print("复用率: ${(pool['reuse_ratio'] * 100).toStringAsFixed(1)}%");

// 重置统计
bufferPoolResetStats();
```

---

## 网络探测

获取详细的网络质量指标：

```dart
// 创建探测管理器
networkProbeCreate("room-123".toNativeUtf8());

// 获取单个 Peer 的指标
final metricsPtr = networkProbeGetMetrics(
  "room-123".toNativeUtf8(),
  "peer-1".toNativeUtf8()
);
final metrics = jsonDecode(metricsPtr.toDartString());

print("""
RTT: ${metrics['rtt_ms']} ms
抖动: ${metrics['jitter_ms']} ms
丢包率: ${metrics['packet_loss']}
可用带宽: ${metrics['available_bw']} bps
质量评分: ${metrics['quality_score']} / 100
""");

// 获取所有 Peer 指标
final allPtr = networkProbeGetAllMetrics("room-123".toNativeUtf8());
final all = jsonDecode(allPtr.toDartString()) as Map<String, dynamic>;

// 清理
networkProbeDestroy("room-123".toNativeUtf8());
```

---

## 抖动缓冲

可选功能，用于平滑弱网抖动：

```dart
// 创建并启用抖动缓冲
jitterBufferCreate(
  "video-buffer".toNativeUtf8(),
  1,    // 启用
  50    // 目标延迟 50ms
);

// 动态调整延迟
jitterBufferSetDelay("video-buffer".toNativeUtf8(), 100);

// 查看统计
final statsPtr = jitterBufferGetStats("video-buffer".toNativeUtf8());
final stats = jsonDecode(statsPtr.toDartString());

print("""
启用: ${stats['enabled']}
缓冲包数: ${stats['buffered_packets']}
当前延迟: ${stats['current_delay_ms']} ms
接收包数: ${stats['packets_received']}
丢弃包数: ${stats['packets_dropped']}
乱序包数: ${stats['packets_reorder']}
""");

// 临时禁用
jitterBufferEnable("video-buffer".toNativeUtf8(), 0);

// 清空缓冲区
jitterBufferFlush("video-buffer".toNativeUtf8());

// 清理
jitterBufferDestroy("video-buffer".toNativeUtf8());
```

## UI 展示建议

```dart
Widget buildStatsPanel() {
  return StreamBuilder<StatsSnapshot>(
    stream: statsStream,
    builder: (context, snapshot) {
      final stats = snapshot.data;
      return Column(
        children: [
          Text("↓ ${formatBitrate(stats.bitrateIn)}"),
          Text("↑ ${formatBitrate(stats.bitrateOut)}"),
          Text("丢包率: ${(stats.lossRate * 100).toStringAsFixed(1)}%"),
          LinearProgressIndicator(value: stats.qualityScore / 100),
        ],
      );
    },
  );
}

String formatBitrate(double bps) {
  if (bps > 1000000) return "${(bps / 1000000).toStringAsFixed(1)} Mbps";
  if (bps > 1000) return "${(bps / 1000).toStringAsFixed(0)} Kbps";
  return "$bps bps";
}
```
