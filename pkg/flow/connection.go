package flow

import (
	"time"

	"github.com/google/uuid"
)

// WireMode 는 와이어의 데이터 전달 모드를 나타내는 문자열 타입이다.
type WireMode string

const (
	// WireBypass 는 버퍼 없이 데이터를 즉시 전달하는 기본 모드이다.
	WireBypass WireMode = "bypass"

	// WireBuffer 는 버퍼를 사용하여 데이터를 전달하는 모드이다.
	WireBuffer WireMode = "buffer"
)

// Wire 는 두 노드의 포트를 연결하는 와이어를 정의하는 구조체이다.
type Wire struct {
	ID           string        `json:"id"`
	SourceNodeID string        `json:"source_node_id"`
	SourcePort   string        `json:"source_port"`
	TargetNodeID string        `json:"target_node_id"`
	TargetPort   string        `json:"target_port"`
	Mode         WireMode      `json:"mode"`
	BufferSize   int           `json:"buffer_size"`
	TTL          time.Duration `json:"ttl"`
}

// WireOption 은 NewWire 팩토리 함수에 전달되는 옵션 함수 타입이다.
type WireOption func(*Wire)

// NewWire 는 소스와 타겟 노드/포트를 연결하는 새로운 Wire를 생성한다.
// 기본 모드는 WireBypass(버퍼 없음)이며, BufferSize와 TTL은 0이다.
// 추가 설정은 WireOption 함수를 통해 적용할 수 있다.
func NewWire(sourceNodeID, sourcePort, targetNodeID, targetPort string, opts ...WireOption) Wire {
	wire := Wire{
		ID:           uuid.New().String(),
		SourceNodeID: sourceNodeID,
		SourcePort:   sourcePort,
		TargetNodeID: targetNodeID,
		TargetPort:   targetPort,
		Mode:         WireBypass,
		BufferSize:   0,
		TTL:          0,
	}

	for _, opt := range opts {
		opt(&wire)
	}

	return wire
}

// WithWireMode 는 와이어의 데이터 전달 모드를 설정하는 WireOption이다.
func WithWireMode(mode WireMode) WireOption {
	return func(w *Wire) {
		w.Mode = mode
	}
}

// WithBufferSize 는 와이어의 버퍼 크기를 설정하는 WireOption이다.
func WithBufferSize(size int) WireOption {
	return func(w *Wire) {
		w.BufferSize = size
	}
}

// WithTTL 은 와이어의 메시지 생존 시간(TTL)을 설정하는 WireOption이다.
func WithTTL(ttl time.Duration) WireOption {
	return func(w *Wire) {
		w.TTL = ttl
	}
}
