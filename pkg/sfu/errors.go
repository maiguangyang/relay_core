/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-24
 *
 * Relay Core Errors
 * 定义系统统一的错误码
 */
package sfu

import "errors"

// Standard Errors
var (
	ErrRoomClosed      = errors.New("room is closed")
	ErrPeerNotFound    = errors.New("peer not found")
	ErrPeerClosed      = errors.New("peer is closed")
	ErrICEFailed       = errors.New("ice connection failed")
	ErrForwarderClosed = errors.New("forwarder is closed")
)

// RelayErrorCode 错误码枚举
type RelayErrorCode int

const (
	// ErrCodeOK 成功
	ErrCodeOK RelayErrorCode = 0

	// ErrCodeGenericError 通用错误
	ErrCodeGenericError RelayErrorCode = 1

	// ErrCodeRoomNotFound 房间不存在
	ErrCodeRoomNotFound RelayErrorCode = 100

	// ErrCodeRoomClosed 房间已关闭
	ErrCodeRoomClosed RelayErrorCode = 101

	// ErrCodeRoomCreateFailed 房间创建失败
	ErrCodeRoomCreateFailed RelayErrorCode = 102

	// ErrCodePeerNotFound Peer 未找到
	ErrCodePeerNotFound RelayErrorCode = 200

	// ErrCodePeerClosed Peer 连接已关闭
	ErrCodePeerClosed RelayErrorCode = 201

	// ErrCodePeerConnectionFailed 建立 PeerConnection 失败
	ErrCodePeerConnectionFailed RelayErrorCode = 202

	// ErrCodeInvalidParams 参数无效（如 JSON 解析失败）
	ErrCodeInvalidParams RelayErrorCode = 300

	// ErrCodeSignalingError 信令相关错误
	ErrCodeSignalingError RelayErrorCode = 400

	// ErrCodeInternalError 内部错误
	ErrCodeInternalError RelayErrorCode = 500
)
