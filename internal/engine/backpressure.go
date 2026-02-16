package engine

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
