package lifecycle

// BaseOption 은 BaseLifecycle 생성 시 적용할 수 있는 옵션 함수 타입이다.
type BaseOption func(*BaseLifecycle)

// WithName 은 컴포넌트 이름을 설정하는 옵션을 반환한다.
func WithName(name string) BaseOption {
	return func(b *BaseLifecycle) {
		b.name = name
	}
}

// WithOnStateChange 는 상태 변경 콜백을 등록하는 옵션을 반환한다.
func WithOnStateChange(cb StateChangeCallback) BaseOption {
	return func(b *BaseLifecycle) {
		b.callbacks = append(b.callbacks, callbackEntry{
			id: b.nextID,
			cb: cb,
		})
		b.nextID++
	}
}
