package sfu

import (
	"strings"
	"sync"
	"time"

	"github.com/maiguangyang/relay_core/pkg/utils"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

// CongestionInterceptorFactory implements interceptor.Factory
type CongestionInterceptorFactory struct {
	Interceptor *CongestionInterceptor
}

func (f *CongestionInterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	return f.Interceptor, nil
}

// CongestionInterceptor drops P-frames when network is congested
type CongestionInterceptor struct {
	interceptor.NoOp

	mu        sync.RWMutex
	congested bool
	rtt       time.Duration
	loss      float64
}

func NewCongestionInterceptor() *CongestionInterceptor {
	return &CongestionInterceptor{}
}

func (c *CongestionInterceptor) SetCongested(congested bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.congested != congested {
		c.congested = congested
		if congested {
			utils.Info("[Congestion] Entered congestion state")
		} else {
			utils.Info("[Congestion] Exited congestion state")
		}
	}
}

func (c *CongestionInterceptor) IsCongested() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.congested
}

func (c *CongestionInterceptor) UpdateStats(rtt time.Duration, loss float64) {
	c.mu.Lock()
	c.rtt = rtt
	c.loss = loss
	// Thresholds: RTT > 300ms or Loss > 5%
	isBad := rtt > 300*time.Millisecond || loss > 0.05
	c.mu.Unlock()

	c.SetCongested(isBad)
}

// BindLocalStream intercepts outgoing RTP packets
func (c *CongestionInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	isAudio := strings.HasPrefix(strings.ToLower(info.MimeType), "audio/")
	codec := ""
	if strings.Contains(strings.ToLower(info.MimeType), "vp8") {
		codec = "vp8"
	} else if strings.Contains(strings.ToLower(info.MimeType), "vp9") {
		codec = "vp9"
	} else if strings.Contains(strings.ToLower(info.MimeType), "h264") {
		codec = "h264"
	}

	return &congestionWriter{
		RTPWriter: writer,
		parent:    c,
		isAudio:   isAudio,
		codec:     codec,
	}
}

type congestionWriter struct {
	interceptor.RTPWriter
	parent  *CongestionInterceptor
	isAudio bool
	codec   string
}

func (w *congestionWriter) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	// Never drop audio
	if w.isAudio {
		return w.RTPWriter.Write(header, payload, attributes)
	}

	// If congested, check if we can drop this packet
	if w.parent.IsCongested() {
		// Only drop if it's NOT a keyframe (i.e., drop P-frames)
		if !IsKeyframe(payload, w.codec) {
			// Drop it!
			// utils.Debug("[Congestion] Dropped P-frame packet")
			// Commented out to avoid log spam, enable for debugging
			return 0, nil
		}
	}

	return w.RTPWriter.Write(header, payload, attributes)
}

// IsKeyframe checks if a packet payload contains a keyframe
func IsKeyframe(payload []byte, codec string) bool {
	if len(payload) == 0 {
		return false
	}

	switch codec {
	case "vp8":
		// https://tools.ietf.org/html/rfc7741#section-4.3
		// S bit is the first bit of the payload
		// 0 1 2 3 4 5 6 7
		// X R N S R PID
		// But in simple VP8 payload (Descriptor?), the payload starts immediately?
		// Pion RTP depacketizer logic:
		// PayloadDescriptor is the first bytes.
		// If X is set, extension present.

		// Simplified check for raw VP8 payload (after descriptor? No, payload IS the RTP payload)
		// Assuming standard packetization.
		// VP8 Payload Descriptor:
		// First byte: X(1) R(1) N(1) S(1) PartID(4)
		// S=1 means start of partition 0 (which contains keyframe info)
		// BUT Keyframe is indicated in the VP8 Header (inside the payload),
		// specifically the P bit (inverse of Keyframe) in the first byte of VP8 payload.

		// Actually, Pion's `codecs/vp8` has `IsKeyframe`.
		// Let's implement a heuristic.

		// First byte of Payload Descriptor:
		// [X|R|N|S|PartID]
		S := (payload[0] & 0x10) != 0
		if !S {
			// Not start of a partition, so definitely not start of a frame, so not a keyframe packet?
			// Well, a keyframe might span multiple packets. We must drop the whole frame if we drop.
			// If we drop parts of a P-frame, it's fine.
			// If we drop parts of a Keyframe, it's BAD.
			// So we must identify if this packet BELONGS to a Keyframe.
			// This is hard without state.
			// BUT, for P-frames, if we drop the FIRST packet (S=1), the rest are useless.
			// If we keep the Keyframe's first packet, we usually want to keep the rest.

			// Heuristic: Just check S bit. If S=1, check if Keyframe.
			// If S=0, we rely on "Stream Consistency"?
			// If we drop a start-packet of P-frame, the decoder ignores the rest.
			// So we effectively drop the frame.
			// If it's a Keyframe, we MUST pass S=1 packet AND subsequent packets.
			// Since we can't easily know if a middle-packet belongs to Keyframe without tracking,
			// we might be Risking dropping Keyframe parts?

			// Optimization: Only drop packets with S=1 that are P-frames?
			// Then the decoder sees missing frame.
			// Revisiting Strategy:
			// We only drop packets that are clearly P-frames.
			// How to know if a middle packet is P-frame? We don't.
			// So we should only drop packets that are START of P-frames?
			// Or we maintain state.
			return true // Safe default: Don't drop middle packets to be safe.
		}

		// S=1. Check VP8 Header.
		// Payload Descriptor might vary in length.
		// Parsing is complex.
		// Let's be conservative. If we can't parse, don't drop.
		return true

	case "h264":
		// H264 NAL Unit Type.
		// First byte: F(1) NRI(2) Type(5)
		nalType := payload[0] & 0x1F
		// Type 5 = IDR (Keyframe)
		// Type 7 = SPS, 8 = PPS (Keep them)
		if nalType == 5 || nalType == 7 || nalType == 8 {
			return true
		}
		// Type 28/29 = FU-A/B (Fragmentation)
		if nalType == 28 {
			// FU-A. Inner byte: S(1) E(1) R(1) Type(5)
			if len(payload) < 2 {
				return false
			}
			originalType := payload[1] & 0x1F
			if originalType == 5 {
				return true
			}
		}
		return false // P-frame or other

	case "vp9":
		// https://tools.ietf.org/html/draft-ietf-payload-vp9-16#section-4.2
		// Payload Descriptor.
		// I bit (Inter-picture predicted). 0 = Keyframe (Intra).
		// First byte: I P L F B E V Z
		// I (bit 7): 0 for Keyframe (or Intra-only), 1 for P-frame.
		if (payload[0] & 0x80) == 0 {
			return true
		}
		return false
	}

	return true // Default: Keep it
}
