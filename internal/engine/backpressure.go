package engine

import (
	"context"

	"github.com/xtra/xflow/pkg/message"
)

// BackpressureStrategy 는 와이어 버퍼가 가득 찼을 때의 처리 전략을 나타내는 타입이다.
type BackpressureStrategy string

const (
	// StrategyBlock 은 버퍼에 여유가 생길 때까지 블로킹하는 전략이다.
	StrategyBlock BackpressureStrategy = "block"

	// StrategyDrop 은 메시지를 드롭하는 전략이다.
	StrategyDrop BackpressureStrategy = "drop"
)

// DropPolicy 는 메시지를 드롭할 때의 정책을 나타내는 타입이다.
type DropPolicy string

const (
	// DropNewest 는 가장 최신 메시지를 드롭한다.
	DropNewest DropPolicy = "drop_newest"

	// DropOldest 는 가장 오래된 메시지를 드롭한다.
	DropOldest DropPolicy = "drop_oldest"
)

// BackpressurePolicy 는 백프레셔 정책 설정을 나타내는 구조체이다.
type BackpressurePolicy struct {
	Strategy            BackpressureStrategy
	BufferHighWaterMark float64
	DropPolicy          DropPolicy
}

// DefaultBackpressurePolicy 는 합리적인 기본값을 가진 BackpressurePolicy를 반환한다.
func DefaultBackpressurePolicy() BackpressurePolicy {
	return BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}
}

// BackpressureResult 는 백프레셔 적용 후 전송 결과를 나타내는 구조체이다.
type BackpressureResult struct {
	// Dropped 는 메시지가 드롭되었는지 여부이다.
	Dropped bool
	// HighWaterMarkReached 는 버퍼 사용량이 HighWaterMark를 초과했는지 여부이다.
	HighWaterMarkReached bool
}

// SendWithBackpressure 는 백프레셔 정책을 적용하여 와이어에 메시지를 전송한다.
//
// StrategyBlock: Go 채널의 자연 백프레셔를 활용하여 버퍼에 여유가 생길 때까지 블로킹한다.
// StrategyDrop + DropNewest: 버퍼가 가득 차면 새 메시지를 드롭한다 (블로킹하지 않음).
// StrategyDrop + DropOldest: 버퍼가 가득 차면 가장 오래된 메시지를 제거하고 새 메시지를 추가한다.
//
// 버퍼 사용량이 BufferHighWaterMark를 초과하면 BackpressureResult.HighWaterMarkReached를 true로 설정한다.
func SendWithBackpressure(ctx context.Context, wire *RuntimeWire, msg message.Message, policy BackpressurePolicy) (BackpressureResult, error) {
	if wire.IsClosed() {
		return BackpressureResult{}, ErrChannelClosed
	}

	result := BackpressureResult{}

	// High Water Mark 검사 (버퍼 채널만 해당)
	if capCh := cap(wire.Ch); capCh > 0 {
		usage := float64(len(wire.Ch)) / float64(capCh)
		if usage >= policy.BufferHighWaterMark {
			result.HighWaterMarkReached = true
		}
	}

	switch policy.Strategy {
	case StrategyDrop:
		return sendWithDrop(ctx, wire, msg, policy, result)
	default:
		// StrategyBlock: Go 채널의 자연 백프레셔 활용
		return sendWithBlock(ctx, wire, msg, result)
	}
}

// sendWithBlock 은 StrategyBlock 전략으로 메시지를 전송한다.
// Go 채널의 자연 백프레셔를 활용하여 버퍼에 여유가 생길 때까지 블로킹한다.
func sendWithBlock(ctx context.Context, wire *RuntimeWire, msg message.Message, result BackpressureResult) (BackpressureResult, error) {
	err := wire.Send(ctx, msg)
	if err != nil {
		return result, err
	}
	return result, nil
}

// sendWithDrop 은 StrategyDrop 전략으로 메시지를 전송한다.
func sendWithDrop(ctx context.Context, wire *RuntimeWire, msg message.Message, policy BackpressurePolicy, result BackpressureResult) (BackpressureResult, error) {
	// 먼저 논블로킹 전송을 시도한다.
	select {
	case wire.Ch <- msg:
		return result, nil
	default:
		// 버퍼가 가득 참 - 드롭 정책에 따라 처리
	}

	switch policy.DropPolicy {
	case DropOldest:
		// 가장 오래된 메시지를 제거한다.
		select {
		case <-wire.Ch:
			// 가장 오래된 메시지 제거 성공
		default:
			// 채널이 비어 있으면 (이론적으로 발생하지 않음)
		}
		// 새 메시지를 추가한다.
		select {
		case wire.Ch <- msg:
		default:
			// 동시성으로 인해 다시 가득 찬 경우
		}
		result.Dropped = true
		result.HighWaterMarkReached = true
		return result, nil

	default:
		// DropNewest: 새 메시지를 드롭한다.
		result.Dropped = true
		result.HighWaterMarkReached = true
		return result, nil
	}
}
