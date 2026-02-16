package engine

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// RuntimeWire 는 Flow 실행 중 두 노드 간 데이터를 전달하는 런타임 와이어이다.
type RuntimeWire struct {
	ID           string
	SourceNodeID string
	SourcePort   string
	TargetNodeID string
	TargetPort   string
	Mode         flow.WireMode
	BufferSize   int
	TTL          time.Duration
	Ch           chan message.Message
	closed       atomic.Bool
}

// CreateRuntimeWires 는 flow.Wire 슬라이스로부터 RuntimeWire 슬라이스를 생성한다.
// WireBypass 모드는 unbuffered 채널, WireBuffer 모드는 buffered 채널(최소 1)을 생성한다.
func CreateRuntimeWires(wires []flow.Wire) ([]*RuntimeWire, error) {
	result := make([]*RuntimeWire, 0, len(wires))

	for _, w := range wires {
		rw := &RuntimeWire{
			ID:           w.ID,
			SourceNodeID: w.SourceNodeID,
			SourcePort:   w.SourcePort,
			TargetNodeID: w.TargetNodeID,
			TargetPort:   w.TargetPort,
			Mode:         w.Mode,
			BufferSize:   w.BufferSize,
			TTL:          w.TTL,
		}

		switch w.Mode {
		case flow.WireBuffer:
			size := w.BufferSize
			if size < 1 {
				size = 1
			}
			rw.Ch = make(chan message.Message, size)
		default:
			// WireBypass: unbuffered 채널
			rw.Ch = make(chan message.Message)
		}

		result = append(result, rw)
	}

	return result, nil
}

// Send 는 컨텍스트를 존중하면서 와이어 채널에 메시지를 전송한다.
// 채널이 닫혀 있으면 ErrChannelClosed를 반환한다.
// 닫힌 채널에 대한 panic을 recover로 안전하게 처리한다.
func (w *RuntimeWire) Send(ctx context.Context, msg message.Message) (err error) {
	if w.closed.Load() {
		return ErrChannelClosed
	}

	defer func() {
		if r := recover(); r != nil {
			err = ErrChannelClosed
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case w.Ch <- msg:
		return nil
	}
}

// Close 는 와이어 채널을 닫는다. 멱등성을 보장한다.
func (w *RuntimeWire) Close() {
	if w.closed.CompareAndSwap(false, true) {
		close(w.Ch)
	}
}

// IsClosed 는 와이어 채널이 닫혔는지 반환한다.
func (w *RuntimeWire) IsClosed() bool {
	return w.closed.Load()
}
