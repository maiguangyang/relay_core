# 选举与评分系统

## 评分规则

Go 层根据设备信息自动计算 Relay 候选分数：

```
总分 = 设备分(40) + 网络分(40) + 电源分(20) = 最高 100 分
```

### 设备类型评分

| 类型 | 值 | 分数 | 说明 |
|------|:--:|:----:|------|
| PC | 1 | 40 | 桌面电脑，最稳定 |
| Pad | 2 | 30 | 平板设备 |
| TV | 3 | 25 | 电视设备 |
| Mobile | 4 | 20 | 手机，移动性高但不稳定 |
| Unknown | 0 | 0 | 未知设备 |

### 连接类型评分

| 类型 | 值 | 分数 | 说明 |
|------|:--:|:----:|------|
| Ethernet | 1 | 40 | 有线网络，最稳定 |
| WiFi | 2 | 30 | 无线网络 |
| Cellular | 3 | 10 | 蜂窝网络，不稳定 |
| Unknown | 0 | 0 | 未知 |

### 电源状态评分

| 状态 | 值 | 分数 | 说明 |
|------|:--:|:----:|------|
| PluggedIn | 1 | 20 | 已插电，可长期运行 |
| Battery | 2 | 10 | 电池供电 |
| LowBattery | 3 | 0 | 低电量，可能随时关机 |
| Unknown | 0 | 0 | 未知 |

### 典型组合分数

| 场景 | 设备 | 网络 | 电源 | 总分 |
|------|------|------|------|:----:|
| 办公室台式机 | PC(40) | Ethernet(40) | PluggedIn(20) | **100** |
| 会议室笔记本 | PC(40) | WiFi(30) | PluggedIn(20) | **90** |
| 充电中的平板 | Pad(30) | WiFi(30) | PluggedIn(20) | **80** |
| 便携笔记本 | PC(40) | WiFi(30) | Battery(10) | **80** |
| 手机热点 | Mobile(20) | WiFi(30) | Battery(10) | **60** |
| 低电量手机 | Mobile(20) | Cellular(10) | LowBattery(0) | **30** |

## 选举流程

### 1. 触发条件

- 新 Peer 加入
- 当前 Relay 离线
- 手动调用 `Election.trigger()`
- Keepalive 检测到 Relay 超时

### 2. 选举过程

```
1. 所有节点计算本地分数
2. 通过信令广播 RelayClaim { epoch, peerId, score }
3. 收集所有声明
4. 应用冲突解决规则
5. 分数最高者成为 Relay
6. 广播 RelayChanged 事件
```

### 3. 冲突解决

当多个节点同时声明成为 Relay 时：

```
优先级1: epoch 更高者优先
优先级2: 同 epoch，score 更高者优先  
优先级3: 分数相同，peerID 字典序更大者优先
```

## 使用示例

### 基础用法

```dart
final election = Election(roomId: 'room-1');

// 启用选举
election.enable();

// 更新设备信息（影响评分）
election.updateDeviceInfo(
  peerId: 'peer-1',
  deviceType: DeviceType.pc,
  connectionType: ConnectionType.ethernet,
  powerState: PowerState.pluggedIn,
);

// 更新网络指标（可选，用于动态调整）
election.updateNetworkMetrics(
  peerId: 'peer-1',
  bandwidth: 100000000,  // 100 Mbps
  latency: 5,            // 5ms
  packetLoss: 0.01,      // 1% 丢包
);

// 手动触发选举
final result = election.trigger();
print('新 Relay: ${result?.proxyId}, 分数: ${result?.score}');

// 获取当前 Relay
final proxy = election.getProxy();

// 获取所有候选者
final candidates = election.getCandidates();
for (final c in candidates) {
  print('${c.peerId}: ${c.score}');
}
```

### 集成 ProxyManager

```dart
final proxyManager = ProxyManager(
  roomId: 'room-1',
  localPeerId: 'my-peer',
  deviceType: DeviceType.pc,        // 设置设备类型
  connectionType: ConnectionType.wifi,
  powerState: PowerState.pluggedIn,
);

// 监听 Relay 变更
proxyManager.onRelayChanged.listen((relay) {
  print('Relay 变更: ${relay.peerId}, epoch: ${relay.epoch}');
});

// 获取状态
final status = proxyManager.getStatus();
print('本机分数: ${status['local_score']}');
print('是否 Relay: ${proxyManager.isRelay}');
```

## 故障切换

### Failover 流程

```
1. Keepalive 检测到 Relay 心跳超时
2. 触发 PEER_OFFLINE 事件
3. 自动重新选举
4. 新 Relay 接管
5. 通知所有订阅者重连
```

### 使用 Failover

```dart
final failover = Failover(
  roomId: 'room-1',
  localPeerId: 'peer-1',
);

failover.enable();

// 设置当前 Relay（收到信令通知时）
failover.setCurrentRelay('relay-peer-id', epoch: 1);

// 更新本机分数
failover.updateLocalScore(85.0);

// 接收其他节点的声明
failover.receiveClaim('other-peer', epoch: 2, score: 90.0);

// 获取状态
final state = failover.getState();
print('当前 Relay: ${state?.currentRelay}');
print('Epoch: ${state?.epoch}');
```
