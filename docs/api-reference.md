# API 参考

## 概览

Relay Core 提供 **85 个** C 导出函数，分为以下几类：

| 分类 | 数量 | 主要功能 |
|------|------|---------|
| [RelayRoom](#relayroom---p2p-连接管理) | 17 | P2P 连接管理 |
| [SourceSwitcher](#sourceswitcher---源切换) | 8 | 双源切换 |
| [Election](#election---代理选举) | 8 | 动态选举 |
| [Keepalive](#keepalive---心跳保活) | 12 | 心跳检测 |
| [Stats](#stats---流量统计) | 13 | 流量监控 |
| [Codec](#codec---编解码器) | 5 | 编码协商 |
| [JitterBuffer](#jitterbuffer---抖动缓冲) | 7 | 可选抖动缓冲 |
| [回调 & 工具](#回调--工具) | 7 | 事件/日志回调 |

---

## RelayRoom - P2P 连接管理

### 房间生命周期

```c
// 创建 Relay 房间
// iceServersJSON: ICE 服务器配置，如 [{"urls": ["stun:stun.l.google.com:19302"]}]
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

// 开始本地分享
int RelayRoomStartLocalShare(char* roomID, char* sharerID);

// 停止本地分享
int RelayRoomStopLocalShare(char* roomID);

// 获取房间状态 (JSON)
char* RelayRoomGetStatus(char* roomID);
```

---

## SourceSwitcher - 源切换

```c
// 创建源切换器
int SourceSwitcherCreate(char* roomID);

// 销毁源切换器
int SourceSwitcherDestroy(char* roomID);

// 注入 SFU RTP 包
int SourceSwitcherInjectSFU(char* roomID, int isVideo, void* data, int dataLen);

// 注入本地分享 RTP 包
int SourceSwitcherInjectLocal(char* roomID, int isVideo, void* data, int dataLen);

// 开始本地分享（切到 Local 源）
int SourceSwitcherStartLocalShare(char* roomID, char* sharerID);

// 停止本地分享（切回 SFU 源）
int SourceSwitcherStopLocalShare(char* roomID);

// 获取状态 (JSON)
char* SourceSwitcherGetStatus(char* roomID);

// 是否正在本地分享
int SourceSwitcherIsLocalSharing(char* roomID);  // 1=是, 0=否
```

---

## Election - 代理选举

```c
// 启用选举
int ElectionEnable(int64_t relayID, char* roomID);

// 禁用选举
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

// 更新候选（旧版，兼容）
int ElectionUpdateCandidate(int64_t relayID, char* roomID, char* peerID,
                            int64_t bandwidth, int64_t latency, double packetLoss);

// 手动触发选举（返回结果 JSON）
char* ElectionTrigger(int64_t relayID, char* roomID);

// 获取当前代理 ID
char* ElectionGetProxy(int64_t relayID, char* roomID);

// 获取候选者列表 (JSON)
char* ElectionGetCandidates(int64_t relayID, char* roomID);
```

---

## Keepalive - 心跳保活

```c
// 创建心跳管理器
int KeepaliveCreate(char* roomID, int intervalMs, int timeoutMs);

// 销毁心跳管理器
int KeepaliveDestroy(char* roomID);

// 启动/停止心跳检测
int KeepaliveStart(char* roomID);
int KeepaliveStop(char* roomID);

// 添加/移除监控的 Peer
int KeepaliveAddPeer(char* roomID, char* peerID);
int KeepaliveRemovePeer(char* roomID, char* peerID);

// 处理 Pong 响应
int KeepaliveHandlePong(char* roomID, char* peerID);

// 获取 Peer 状态 (0=Unknown, 1=Online, 2=Slow, 3=Offline)
int KeepaliveGetPeerStatus(char* roomID, char* peerID);

// 获取 RTT (毫秒)
int64_t KeepaliveGetPeerRTT(char* roomID, char* peerID);

// 获取 Peer 详情 (JSON)
char* KeepaliveGetPeerInfo(char* roomID, char* peerID);

// 获取所有 Peer 信息 (JSON 数组)
char* KeepaliveGetAllPeerInfo(char* roomID);

// 设置 Ping 回调
void SetPingCallback(PingCallback callback);
```

---

## Stats - 流量统计

```c
// 创建/销毁统计
int StatsCreate(char* roomID);
int StatsDestroy(char* roomID);

// 添加接收/发送字节数
int StatsAddBytesIn(char* roomID, char* peerID, uint64_t bytes);
int StatsAddBytesOut(char* roomID, char* peerID, uint64_t bytes);

// 添加丢包
int StatsAddPacketLost(char* roomID);

// 计算码率（每秒调用一次）
int StatsCalculateBitrate(char* roomID);

// 获取统计
char* StatsGetSnapshot(char* roomID);  // 完整快照
char* StatsGetTraffic(char* roomID);   // 总体流量
double StatsGetBitrateIn(char* roomID);  // 入站码率 bps
double StatsGetBitrateOut(char* roomID); // 出站码率 bps
double StatsGetLossRate(char* roomID);   // 丢包率

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
// 获取支持的编解码器
char* CodecGetSupportedVideo(void);  // VP8, VP9, H264, AV1
char* CodecGetSupportedAudio(void);  // Opus, G722

// 解析 MimeType
char* CodecParseType(char* mimeType);

// 类型判断
int CodecIsVideo(char* codecType);  // 1=是, 0=否
int CodecIsAudio(char* codecType);
```

---

## JitterBuffer - 抖动缓冲

```c
// 创建抖动缓冲（默认禁用）
int JitterBufferCreate(char* key, int enabled, int targetDelayMs);

// 销毁
int JitterBufferDestroy(char* key);

// 启用/禁用
int JitterBufferEnable(char* key, int enabled);

// 设置目标延迟
int JitterBufferSetDelay(char* key, int delayMs);

// 清空缓冲区
int JitterBufferFlush(char* key);

// 获取统计
char* JitterBufferGetStats(char* key);

// 是否启用
int JitterBufferIsEnabled(char* key);
```

---

## 回调 & 工具

### 事件回调

```c
// 设置事件回调
typedef void (*EventCallback)(int eventType, const char* roomId, 
                              const char* peerId, const char* data);
void SetEventCallback(EventCallback callback);
```

事件类型：
- `1` = Peer 加入
- `2` = Peer 离开
- `3` = Track 添加
- `4` = 错误
- `5` = ICE 候选
- `6` = 代理变更
- `10` = 订阅者加入
- `11` = 订阅者离开
- `12` = 需要重协商
- `20` = Peer 上线
- `21` = Peer 响应缓慢
- `22` = Peer 离线
- `23` = 需要发送 Ping

### 日志回调

```c
typedef void (*LogCallback)(int level, const char* message);
void SetLogCallback(LogCallback callback);
void SetLogLevel(int level);  // 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR
```

### 工具函数

```c
// 释放 Go 分配的字符串
void FreeString(char* s);

// 获取版本号
char* GetVersion(void);
```

### 便捷组合函数

```c
// 初始化代理模式（创建 SourceSwitcher + 启用选举）
int ProxyModeInit(int64_t relayID, char* roomID);

// 清理代理模式
int ProxyModeCleanup(int64_t relayID, char* roomID);

// 获取代理模式综合状态
char* ProxyModeGetStatus(int64_t relayID, char* roomID);
```
