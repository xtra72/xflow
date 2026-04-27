package xferr

import (
	"context"
	"errors"
	"net"
)

// ErrorSeverity 는 에러의 심각도를 나타내는 문자열 타입이다.
type ErrorSeverity string

const (
	// SeverityCritical 은 치명적인 에러를 나타낸다.
	SeverityCritical ErrorSeverity = "critical"
	// SeverityError 는 일반 에러를 나타낸다.
	SeverityError ErrorSeverity = "error"
	// SeverityWarning 은 경고를 나타낸다.
	SeverityWarning ErrorSeverity = "warning"
)

// ErrorCategory 는 에러의 분류 카테고리를 나타내는 문자열 타입이다.
type ErrorCategory string

const (
	// CategoryProcessing 은 처리 중 발생한 에러를 나타낸다.
	CategoryProcessing ErrorCategory = "processing"
	// CategoryValidation 은 유효성 검증 에러를 나타낸다.
	CategoryValidation ErrorCategory = "validation"
	// CategoryTimeout 은 시간 초과 에러를 나타낸다.
	CategoryTimeout ErrorCategory = "timeout"
	// CategoryConnection 은 연결 관련 에러를 나타낸다.
	CategoryConnection ErrorCategory = "connection"
	// CategoryConfiguration 은 설정 관련 에러를 나타낸다.
	CategoryConfiguration ErrorCategory = "configuration"
	// CategorySystem 은 시스템 에러를 나타낸다.
	CategorySystem ErrorCategory = "system"
)

// DropReason 는 메시지가 폐기된 사유를 나타내는 문자열 타입이다.
type DropReason string

const (
	// ReasonTTLExpired 는 TTL 만료로 인한 폐기를 나타낸다.
	ReasonTTLExpired DropReason = "ttl_expired"
	// ReasonBackpressureDrop 은 백프레셔로 인한 폐기를 나타낸다.
	ReasonBackpressureDrop DropReason = "backpressure_drop"
	// ReasonFilterRejected 는 필터에 의한 거부를 나타낸다.
	ReasonFilterRejected DropReason = "filter_rejected"
	// ReasonMaxRetriesExceeded 는 최대 재시도 횟수 초과를 나타낸다.
	ReasonMaxRetriesExceeded DropReason = "max_retries_exceeded"
	// ReasonNodeStopped 는 노드 중지로 인한 폐기를 나타낸다.
	ReasonNodeStopped DropReason = "node_stopped"
	// ReasonChannelFull 은 채널 가득 참으로 인한 폐기를 나타낸다.
	ReasonChannelFull DropReason = "channel_full"
)

// ComponentType 는 컴포넌트 유형을 나타내는 문자열 타입이다.
type ComponentType string

const (
	// ComponentFlow 는 플로우 컴포넌트를 나타낸다.
	ComponentFlow ComponentType = "flow"
	// ComponentNode 는 노드 컴포넌트를 나타낸다.
	ComponentNode ComponentType = "node"
	// ComponentAgent 는 에이전트 컴포넌트를 나타낸다.
	ComponentAgent ComponentType = "agent"
	// ComponentScriptEngine 은 스크립트 엔진 컴포넌트를 나타낸다.
	ComponentScriptEngine ComponentType = "script_engine"
	// ComponentPlugin 은 플러그인 컴포넌트를 나타낸다.
	ComponentPlugin ComponentType = "plugin"
)

// IsValidSeverity 는 주어진 심각도가 유효한지 확인한다.
func IsValidSeverity(s ErrorSeverity) bool {
	switch s {
	case SeverityCritical, SeverityError, SeverityWarning:
		return true
	default:
		return false
	}
}

// IsValidCategory 는 주어진 카테고리가 유효한지 확인한다.
func IsValidCategory(c ErrorCategory) bool {
	switch c {
	case CategoryProcessing, CategoryValidation, CategoryTimeout,
		CategoryConnection, CategoryConfiguration, CategorySystem:
		return true
	default:
		return false
	}
}

// IsValidDropReason 는 주어진 폐기 사유가 유효한지 확인한다.
func IsValidDropReason(r DropReason) bool {
	switch r {
	case ReasonTTLExpired, ReasonBackpressureDrop, ReasonFilterRejected,
		ReasonMaxRetriesExceeded, ReasonNodeStopped, ReasonChannelFull:
		return true
	default:
		return false
	}
}

// IsValidComponentType 는 주어진 컴포넌트 타입이 유효한지 확인한다.
func IsValidComponentType(ct ComponentType) bool {
	switch ct {
	case ComponentFlow, ComponentNode, ComponentAgent,
		ComponentScriptEngine, ComponentPlugin:
		return true
	default:
		return false
	}
}

// ClassifySeverity 는 에러를 분석하여 적절한 심각도를 반환한다.
// context.DeadlineExceeded → SeverityError, 패닉 복구 → SeverityCritical, 기본값 → SeverityError
func ClassifySeverity(err error) ErrorSeverity {
	if errors.Is(err, context.DeadlineExceeded) {
		return SeverityError
	}
	return SeverityError
}

// ClassifyCategory 는 에러를 분석하여 적절한 카테고리를 반환한다.
// context.DeadlineExceeded/Canceled → CategoryTimeout, net.Error → CategoryConnection, 기본값 → CategoryProcessing
func ClassifyCategory(err error) ErrorCategory {
	// context 관련 타임아웃 확인 (net.Error보다 먼저 검사해야 한다)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return CategoryTimeout
	}

	// net.Error 인터페이스 확인 (connection 관련)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return CategoryConnection
	}

	return CategoryProcessing
}
