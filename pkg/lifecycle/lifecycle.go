package lifecycle

import "context"

// Lifecycle 은 컴포넌트의 생명주기를 관리하는 인터페이스이다 (AC-LIFE-001-08).
type Lifecycle interface {
	// Init 은 컴포넌트를 초기화한다.
	// Created 상태에서만 호출 가능하며, 성공 시 Running 상태로 전이한다.
	Init(ctx context.Context) error

	// Start 는 컴포넌트를 시작한다.
	// Initializing 완료 후 Running 상태로 전이한다.
	Start(ctx context.Context) error

	// Pause 는 컴포넌트를 일시정지한다.
	// Running 상태에서만 호출 가능하며, Paused 상태로 전이한다.
	Pause(ctx context.Context) error

	// Resume 은 일시정지된 컴포넌트를 재개한다.
	// Paused 상태에서만 호출 가능하며, Running 상태로 전이한다.
	Resume(ctx context.Context) error

	// Stop 은 컴포넌트를 정지한다.
	// Running 또는 Paused 상태에서 호출 가능하며, Stopped 상태로 전이한다.
	Stop(ctx context.Context) error

	// State 는 컴포넌트의 현재 상태를 반환한다.
	State() State
}
