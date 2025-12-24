# API 参考

## 概览

Relay Core 提供 **106 个** C 导出函数，分为以下几类：

| 分类 | 数量 | 主要功能 |
|------|------|---------| 
| [Coordinator](#coordinator---一键自动代理) | 14 | 一键启用自动代理和故障切换 |
| [RelayRoom](#relayroom---p2p-连接管理) | 17 | P2P 连接管理 |
| [SourceSwitcher](#sourceswitcher---源切换) | 8 | 双源切换 |
| [Election](#election---代理选举) | 8 | 动态选举 |
| [Failover](#failover---故障切换) | 6 | 自动故障切换 |
| [Keepalive](#keepalive---心跳保活) | 12 | 心跳检测 |
| [Stats](#stats---流量统计) | 13 | 流量监控 |
| [Codec](#codec---编解码器) | 5 | 编码协商 |
| [JitterBuffer](#jitterbuffer---抖动缓冲) | 7 | 可选抖动缓冲 |
| [回调 & 工具](#回调--工具) | 8 | 事件/日志回调 |

---

## Coordinator - 一键自动代理

**推荐使用**：Coordinator 集成了所有组件（Keepalive、Election、Failover、SourceSwitcher），提供一键启用的自动代理模式。

### 核心功能

```c
// 一键启用自动代理模式（推荐）
// 自动管理：心跳检测、选举、故障切换、Relay 接管
int CoordinatorEnable(char* roomID, char* localPeerID);

// 禁用自动代理模式
int CoordinatorDisable(char* roomID);

// 获取状态
char* CoordinatorGetStatus(char* roomID);

// 本机是否是 Relay
int CoordinatorIsRelay(char* roomID);  // 1=是, 0=否, -1=错误
```

### Peer 管理

```c
// 添加 Peer（自动监控心跳 + 加入选举候选）
int CoordinatorAddPeer(char* roomID, char* peerID, 
                       int deviceType, int connectionType, int powerState);

// 移除 Peer
int CoordinatorRemovePeer(char* roomID, char* peerID);

// 处理 Pong（心跳响应）
int CoordinatorHandlePong(char* roomID, char* peerID);

// 更新本机设备信息
int CoordinatorUpdateLocalDevice(char* roomID, 
                                 int deviceType, int connectionType, int powerState);
```

### Relay 协调

```c
// 设置当前 Relay（收到信令通知时）
int CoordinatorSetRelay(char* roomID, char* relayID, uint64_t epoch);

// 接收其他节点的 Relay 声明（冲突解决）
// score: 声明者分数，用于同 epoch 冲突解决
int CoordinatorReceiveClaim(char* roomID, char* peerID, uint64_t epoch, double score);
```

### RTP 注入

```c
// 注入 SFU RTP 包
int CoordinatorInjectSFU(char* roomID, int isVideo, void* data, int dataLen);

// 注入本地分享 RTP 包
int CoordinatorInjectLocal(char* roomID, int isVideo, void* data, int dataLen);

// 开始/停止本地分享
int CoordinatorStartLocalShare(char* roomID, char* sharerID);
int CoordinatorStopLocalShare(char* roomID);
```

---

## RelayRoom - P2P 连接管理

### 房间生命周期

```c
// 创建 Relay 房间
int RelayRoomCreate(char* roomID, char* iceServersJSON);

// 销毁房间
int RelayRoomDestroy(char* roomID);

// 成为 Relay 节点
int RelayRoomBecomeRelay(char* roomID, char* peerID);

// 检查是否是 Relay 节点
int RelayRoomIsRelay(char* roomID);  // 返回: 1=是, 0=否, -1=错误
```

### 订阅者管理

```c
// 添加订阅者（返回 Answer SDP）
char* RelayRoomAddSubscriber(char* roomID, char* peerID, char* offerSDP);

// 移除订阅者
int RelayRoomRemoveSubscriber(char* roomID, char* peerID);

// 获取订阅者列表 (JSON 数组)
char* RelayRoomGetSubscribers(char* roomID);

// 获取订阅者数量
int RelayRoomGetSubscriberCount(char* roomID);
```

### ICE 处理

```c
// 添加 ICE 候选
int RelayRoomAddICECandidate(char* roomID, char* peerID, char* candidateJSON);
```

### SDP 重协商

```c
// 触发全员重协商（返回 {peerID: offerSDP} 的 JSON）
char* RelayRoomTriggerRenegotiation(char* roomID);

// 为单个订阅者创建 Offer
char* RelayRoomCreateOffer(char* roomID, char* peerID);

// 处理 Answer
int RelayRoomHandleAnswer(char* roomID, char* peerID, char* answerSDP);
```

### RTP 注入 & 本地分享

```c
// 注入 SFU RTP 包
int RelayRoomInjectSFU(char* roomID, int isVideo, void* data, int dataLen);

// 注入本地分享 RTP 包
int RelayRoomInjectLocal(char* roomID, int isVideo, void* data, int dataLen);

// 开始/停止本地分享
int RelayRoomStartLocalShare(char* roomID, char* sharerID);
int RelayRoomStopLocalShare(char* roomID);

// 获取房间状态 (JSON)
char* RelayRoomGetStatus(char* roomID);
```

---

## SourceSwitcher - 源切换

```c
// 创建/销毁源切换器
int SourceSwitcherCreate(char* roomID);
int SourceSwitcherDestroy(char* roomID);

// 注入 RTP 包
int SourceSwitcherInjectSFU(char* roomID, int isVideo, void* data, int dataLen);
int SourceSwitcherInjectLocal(char* roomID, int isVideo, void* data, int dataLen);

// 本地分享控制
int SourceSwitcherStartLocalShare(char* roomID, char* sharerID);
int SourceSwitcherStopLocalShare(char* roomID);

// 状态查询
char* SourceSwitcherGetStatus(char* roomID);
int SourceSwitcherIsLocalSharing(char* roomID);  // 1=是, 0=否
```

---

## Election - 代理选举

```c
// 启用/禁用选举
int ElectionEnable(int64_t relayID, char* roomID);
int ElectionDisable(int64_t relayID, char* roomID);

// 更新设备信息
// deviceType: 0=Unknown, 1=PC, 2=Pad, 3=Mobile, 4=TV
// connectionType: 0=Unknown, 1=Ethernet, 2=WiFi, 3=Cellular
// powerState: 0=Unknown, 1=PluggedIn, 2=Battery, 3=LowBattery
int ElectionUpdateDeviceInfo(int64_t relayID, char* roomID, char* peerID,
                             int deviceType, int connectionType, int powerState);

// 更新网络指标
int ElectionUpdateNetworkMetrics(int64_t relayID, char* roomID, char* peerID,
                                 int64_t bandwidth, int64_t latency, double packetLoss);

// 手动触发选举
char* ElectionTrigger(int64_t relayID, char* roomID);

// 获取当前代理/候选者列表
char* ElectionGetProxy(int64_t relayID, char* roomID);
char* ElectionGetCandidates(int64_t relayID, char* roomID);
```

---

## Failover - 故障切换

自动 Relay 故障检测和切换，包含冲突解决机制。

```c
// 启用/禁用故障切换
int FailoverEnable(char* roomID, char* localPeerID);
int FailoverDisable(char* roomID);

// 设置当前 Relay
int FailoverSetCurrentRelay(char* roomID, char* relayID, uint64_t epoch);

// 更新本机分数（用于选举排序）
int FailoverUpdateLocalScore(char* roomID, double score);

// 接收 Relay 声明（冲突解决）
// 收到同 epoch 声明时，分数高者优先
int FailoverReceiveClaim(char* roomID, char* peerID, uint64_t epoch, double score);

// 获取状态
char* FailoverGetState(char* roomID);
```

### 冲突解决策略

当多个节点同时声明成为 Relay 时：

1. **epoch 更高者优先**
2. **同 epoch，分数更高者优先**
3. **分数相同，PeerID 字典序更大者优先**

---

## Keepalive - 心跳保活

```c
// 创建/销毁心跳管理器
int KeepaliveCreate(char* roomID, int intervalMs, int timeoutMs);
int KeepaliveDestroy(char* roomID);

// 启动/停止
int KeepaliveStart(char* roomID);
int KeepaliveStop(char* roomID);

// Peer 管理
int KeepaliveAddPeer(char* roomID, char* peerID);
int KeepaliveRemovePeer(char* roomID, char* peerID);
int KeepaliveHandlePong(char* roomID, char* peerID);

// 状态查询
int KeepaliveGetPeerStatus(char* roomID, char* peerID);  // 0=Unknown, 1=Online, 2=Slow, 3=Offline
int64_t KeepaliveGetPeerRTT(char* roomID, char* peerID);
char* KeepaliveGetPeerInfo(char* roomID, char* peerID);
char* KeepaliveGetAllPeerInfo(char* roomID);

// Ping 回调
void SetPingCallback(PingCallback callback);
```

---

## Stats - 流量统计

```c
// 创建/销毁
int StatsCreate(char* roomID);
int StatsDestroy(char* roomID);

// 添加统计数据
int StatsAddBytesIn(char* roomID, char* peerID, uint64_t bytes);
int StatsAddBytesOut(char* roomID, char* peerID, uint64_t bytes);
int StatsAddPacketLost(char* roomID);
int StatsCalculateBitrate(char* roomID);

// 获取统计
char* StatsGetSnapshot(char* roomID);
char* StatsGetTraffic(char* roomID);
double StatsGetBitrateIn(char* roomID);
double StatsGetBitrateOut(char* roomID);
double StatsGetLossRate(char* roomID);

// 缓冲池统计
char* BufferPoolGetStats(void);
void BufferPoolResetStats(void);

// 网络探测
int NetworkProbeCreate(char* roomID);
int NetworkProbeDestroy(char* roomID);
char* NetworkProbeGetMetrics(char* roomID, char* peerID);
char* NetworkProbeGetAllMetrics(char* roomID);
```

---

## Codec - 编解码器

```c
char* CodecGetSupportedVideo(void);  // VP8, VP9, H264, AV1
char* CodecGetSupportedAudio(void);  // Opus, G722
char* CodecParseType(char* mimeType);
int CodecIsVideo(char* codecType);
int CodecIsAudio(char* codecType);
```

---

## JitterBuffer - 抖动缓冲

```c
int JitterBufferCreate(char* key, int enabled, int targetDelayMs);
int JitterBufferDestroy(char* key);
int JitterBufferEnable(char* key, int enabled);
int JitterBufferSetDelay(char* key, int delayMs);
int JitterBufferFlush(char* key);
char* JitterBufferGetStats(char* key);  // 包含 jitter_ms 字段
int JitterBufferIsEnabled(char* key);
```

---

## 回调 & 工具

### 事件回调

```c
typedef void (*EventCallback)(int eventType, const char* roomId, 
                              const char* peerId, const char* data);
void SetEventCallback(EventCallback callback);
```

**事件类型**：

| 值 | 事件 | 说明 |
|----|------|------|
| 1 | Peer 加入 | |
| 2 | Peer 离开 | |
| 3 | Track 添加 | |
| 4 | 错误 | |
| 5 | ICE 候选 | |
| 6 | 代理变更 | Relay 切换、本机成为 Relay |
| 10 | 订阅者加入 | |
| 11 | 订阅者离开 | |
| 12 | 需要重协商 | |
| 20 | Peer 上线 | 心跳恢复 |
| 21 | Peer 响应缓慢 | RTT 超过阈值 |
| 22 | Peer 离线 | 心跳超时 |
| 23 | 需要发送 Ping | |

### 日志回调

```c
typedef void (*LogCallback)(int level, const char* message);
void SetLogCallback(LogCallback callback);
void SetLogLevel(int level);  // 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR
```

### 工具函数

```c
void FreeString(char* s);       // 释放 Go 分配的字符串
void CleanupAll(void);          // 释放所有资源
char* GetVersion(void);         // 获取版本号
```

### 便捷组合函数

```c
int ProxyModeInit(int64_t relayID, char* roomID);
int ProxyModeCleanup(int64_t relayID, char* roomID);
char* ProxyModeGetStatus(int64_t relayID, char* roomID);
```
