/// Relay 核心错误码枚举
///
/// 对应 Go 层 `pkg/sfu/errors.go` 中的定义
enum RelayErrorCode {
  /// 成功
  ok(0),

  /// 通用错误
  genericError(1),

  /// 房间不存在
  roomNotFound(100),

  /// 房间已关闭
  roomClosed(101),

  /// 房间创建失败
  roomCreateFailed(102),

  /// Peer 未找到
  peerNotFound(200),

  /// Peer 连接已关闭
  peerClosed(201),

  /// 建立 PeerConnection 失败
  peerConnectionFailed(202),

  /// 参数无效
  invalidParams(300),

  /// 信令相关错误
  signalingError(400),

  /// 内部错误
  internalError(500);

  final int value;
  const RelayErrorCode(this.value);

  /// 从 int 值解析枚举
  static RelayErrorCode fromValue(int value) {
    return RelayErrorCode.values.firstWhere(
      (e) => e.value == value,
      orElse: () => RelayErrorCode.genericError,
    );
  }
}

/// Relay 异常类
class RelayException implements Exception {
  final RelayErrorCode code;
  final String? message;

  RelayException(this.code, [this.message]);

  @override
  String toString() => 'RelayException(code: $code, message: $message)';
}
