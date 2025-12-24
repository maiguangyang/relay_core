/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package sfu

import "errors"

var (
	// ErrRoomClosed indicates the room has been closed
	ErrRoomClosed = errors.New("room is closed")

	// ErrPeerNotFound indicates the peer was not found
	ErrPeerNotFound = errors.New("peer not found")

	// ErrPeerClosed indicates the peer has been closed
	ErrPeerClosed = errors.New("peer is closed")

	// ErrConnectionFailed indicates the WebRTC connection failed
	ErrConnectionFailed = errors.New("connection failed")

	// ErrICEFailed indicates ICE connection failed
	ErrICEFailed = errors.New("ICE connection failed")

	// ErrForwarderClosed indicates the forwarder has been closed
	ErrForwarderClosed = errors.New("forwarder is closed")

	// ErrInvalidSDP indicates invalid SDP
	ErrInvalidSDP = errors.New("invalid SDP")

	// ErrTrackNotFound indicates the track was not found
	ErrTrackNotFound = errors.New("track not found")
)
