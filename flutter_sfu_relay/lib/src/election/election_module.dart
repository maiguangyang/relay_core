/// 选举和故障切换模块导出
library;

export 'election.dart' hide Election;
export 'failover.dart';
export 'proxy_manager.dart';
export 'proxy_mode.dart';

// 重新导出 Election 类
export 'election.dart' show Election, CandidateInfo, ElectionResult;
