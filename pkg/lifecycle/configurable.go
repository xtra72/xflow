package lifecycle

import "context"

// Configurable 은 런타임에 설정을 변경할 수 있는 컴포넌트 인터페이스이다 (AC-LIFE-001-13).
type Configurable interface {
	// Configure 는 컴포넌트의 설정을 변경한다.
	// Created 또는 Stopped 상태에서만 호출 가능하다.
	Configure(ctx context.Context, cfg map[string]any) error

	// GetConfig 는 컴포넌트의 현재 설정을 반환한다.
	GetConfig() map[string]any
}
