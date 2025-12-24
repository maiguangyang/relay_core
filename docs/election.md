# 动态代理选举

## 概述

动态选举系统根据设备性能、网络质量、电源状态等因素，自动选择最优的 Relay 节点。

## 评分规则

### 设备类型 (deviceType)

| 值 | 类型 | 基础分 |
|---|------|-------|
| 1 | PC/Mac | 100 |
| 4 | 智能电视 | 90 |
| 2 | 平板 | 60 |
| 3 | 手机 | 40 |
| 0 | 未知 | 30 |

### 连接类型 (connectionType)

| 值 | 类型 | 乘数 |
|---|------|-----|
| 1 | Ethernet (有线) | ×1.0 |
| 2 | WiFi | ×0.8 |
| 3 | Cellular (蜂窝) | ×0.3 |
| 0 | 未知 | ×0.5 |

### 电源状态 (powerState)

| 值 | 状态 | 乘数 |
|---|------|-----|
| 1 | 充电中 | ×1.0 |
| 2 | 电池供电 | ×0.7 |
| 3 | 低电量 | ×0.3 |
| 0 | 未知 | ×0.8 |

### 网络质量乘数

根据 RTT、丢包率自动调整：

```
网络乘数 = 1.0 - (latency / 1000 * 0.5) - (packetLoss * 2)
```

### 最终得分

```
最终分 = 基础分 × 连接乘数 × 电源乘数 × 网络乘数
```

## 使用教程

### 1. 启用选举

```dart
final relayId = 0;
electionEnable(relayId, "room-123".toNativeUtf8());
```

### 2. 更新设备信息

每个设备加入时上报自己的信息：

```dart
// PC + 有线 + 插电
electionUpdateDeviceInfo(
  relayId,
  "room-123".toNativeUtf8(),
  "peer-pc".toNativeUtf8(),
  1,  // PC
  1,  // Ethernet
  1   // PluggedIn
);

// 手机 + WiFi + 电池
electionUpdateDeviceInfo(
  relayId,
  "room-123".toNativeUtf8(),
  "peer-mobile".toNativeUtf8(),
  3,  // Mobile
  2,  // WiFi
  2   // Battery
);
```

### 3. 更新网络指标

定期更新（如每 5 秒）：

```dart
electionUpdateNetworkMetrics(
  relayId,
  "room-123".toNativeUtf8(),
  "peer-pc".toNativeUtf8(),
  10000000,  // 10 Mbps
  20,        // RTT 20ms
  0.001      // 0.1% 丢包
);
```

### 4. 获取候选者列表

```dart
final candidatesPtr = electionGetCandidates(relayId, "room-123".toNativeUtf8());
final candidates = jsonDecode(candidatesPtr.toDartString()) as List;

for (final c in candidates) {
  print("${c['peer_id']}: 分数=${c['score']}, 设备=${c['device_type']}");
}
```

### 5. 手动触发选举

```dart
final resultPtr = electionTrigger(relayId, "room-123".toNativeUtf8());
final result = jsonDecode(resultPtr.toDartString());

print("新代理: ${result['proxy_id']}");
print("得分: ${result['score']}");
print("原因: ${result['reason']}");
```

### 6. 获取当前代理

```dart
final proxyPtr = electionGetProxy(relayId, "room-123".toNativeUtf8());
final proxyId = proxyPtr.toDartString();
print("当前代理: $proxyId");
```

### 7. 监听代理变更

```dart
void handleEvent(int type, String roomId, String peerId, String data) {
  if (type == 6) { // EventTypeProxyChange
    final info = jsonDecode(data);
    print("代理变更为: ${info['proxy_id']}");
    print("设备类型: ${info['device_type']}");
    
    if (info['proxy_id'] == myPeerId) {
      // 我被选为 Relay，开始转发
      relayRoomBecomeRelay(roomId, myPeerId);
    }
  }
}
```

## 自动选举

启用选举后，系统会：

1. 每 **5 秒** 自动评估一次（可配置）
2. 如果最高分候选者变化且分数差距 > 10，触发选举
3. 通过 `EventTypeProxyChange` 事件通知

## 场景示例

### 场景 1: 北京办公室

```
设备             设备类型   连接     电源     最终分
PC-A            PC(100)   有线(1.0) 插电(1.0) = 100
手机-B          手机(40)  WiFi(0.8) 电池(0.7) = 22.4
手机-C          手机(40)  WiFi(0.8) 电池(0.7) = 22.4

→ PC-A 当选 Relay
```

### 场景 2: 出差场景（无 PC）

```
设备             设备类型   连接     电源     最终分
iPad-A          平板(60)  WiFi(0.8) 插电(1.0) = 48
手机-B          手机(40)  WiFi(0.8) 电池(0.7) = 22.4
手机-C          手机(40)  蜂窝(0.3) 低电(0.3) = 3.6

→ iPad-A 当选 Relay
```

## 注意事项

1. **定期更新**: 确保定期调用 `UpdateDeviceInfo` 和 `UpdateNetworkMetrics`
2. **设备检测**: Dart 层需实现设备类型/网络类型/电源状态的检测
3. **网络变化**: 网络切换时（WiFi → 蜂窝）及时更新
4. **电量变化**: 监听电量变化并更新
