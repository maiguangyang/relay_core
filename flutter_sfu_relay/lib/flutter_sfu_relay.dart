/// Flutter SFU Relay SDK
///
/// 基于 Pion WebRTC 的嵌入式微型 SFU 核心，完整封装 Go 层 106 个 FFI 函数
library;

// ========== 核心模块 ==========
export 'src/core/core.dart';

// ========== 房间管理 ==========
export 'src/room/room.dart';

// ========== 选举和故障切换 ==========
export 'src/election/election_module.dart';

// ========== 媒体处理 ==========
export 'src/media/media.dart';

// ========== 监控 ==========
export 'src/monitoring/monitoring.dart';

// ========== 枚举和类型 ==========
export 'src/enums.dart';

// ========== 回调处理 ==========
export 'src/callbacks/callbacks.dart';

// ========== 信令 ==========
export 'src/signaling/signaling.dart';

// ========== WebRTC ==========
export 'src/webrtc/webrtc.dart';

// ========== 底层绑定 ==========
export 'src/bindings/bindings.dart';
