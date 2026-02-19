package api

import (
	"errors"
	"fmt"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/xferr"
)

// APIError 는 HTTP 상태 코드 매핑을 포함하는 API 에러를 나타낸다.
type APIError struct {
	HTTPCode int    // HTTP 상태 코드 (400, 401, 403, 404, 409, 422, 429, 500, 503)
	Code     string // 기계 판독용 에러 코드 (예: "BAD_REQUEST")
	Message  string // 사람이 읽을 수 있는 에러 메시지
	Details  any    // 선택적 추가 세부 정보
}

// Error 는 error 인터페이스를 구현한다. "api: CODE: message" 형식을 반환한다.
func (e *APIError) Error() string {
	return fmt.Sprintf("api: %s: %s", e.Code, e.Message)
}

// WithMessage 는 커스텀 메시지를 가진 새 APIError를 반환한다 (HTTPCode와 Code는 보존).
func (e *APIError) WithMessage(msg string) *APIError {
	return &APIError{
		HTTPCode: e.HTTPCode,
		Code:     e.Code,
		Message:  msg,
		Details:  e.Details,
	}
}

// WithDetails 는 추가 세부 정보를 가진 새 APIError를 반환한다.
func (e *APIError) WithDetails(details any) *APIError {
	return &APIError{
		HTTPCode: e.HTTPCode,
		Code:     e.Code,
		Message:  e.Message,
		Details:  details,
	}
}

// 사전 정의된 센티널 에러 인스턴스
var (
	ErrBadRequest         = &APIError{HTTPCode: 400, Code: "BAD_REQUEST", Message: "bad request"}
	ErrUnauthorized       = &APIError{HTTPCode: 401, Code: "UNAUTHORIZED", Message: "unauthorized"}
	ErrForbidden          = &APIError{HTTPCode: 403, Code: "FORBIDDEN", Message: "forbidden"}
	ErrNotFound           = &APIError{HTTPCode: 404, Code: "NOT_FOUND", Message: "not found"}
	ErrRequestTimeout     = &APIError{HTTPCode: 408, Code: "REQUEST_TIMEOUT", Message: "request timeout"}
	ErrConflict           = &APIError{HTTPCode: 409, Code: "CONFLICT", Message: "resource conflict"}
	ErrValidationFailed   = &APIError{HTTPCode: 422, Code: "VALIDATION_FAILED", Message: "validation failed"}
	ErrRateLimitExceeded  = &APIError{HTTPCode: 429, Code: "RATE_LIMIT_EXCEEDED", Message: "rate limit exceeded"}
	ErrInternalServer     = &APIError{HTTPCode: 500, Code: "INTERNAL_ERROR", Message: "internal server error"}
	ErrServiceUnavailable = &APIError{HTTPCode: 503, Code: "SERVICE_UNAVAILABLE", Message: "service unavailable"}
)

// MapDomainError 는 pkg/xferr 도메인 에러를 APIError로 매핑한다.
// errors.Is()를 사용하여 알려진 센티널 에러를 매칭하고 적절한 HTTP 에러를 반환한다.
// 알 수 없는 에러는 ErrInternalServer로 매핑된다.
func MapDomainError(err error) *APIError {
	if err == nil {
		return nil
	}

	switch {
	// 유효성 검증 관련 에러 → 400 Bad Request
	case errors.Is(err, xferr.ErrNilMessage),
		errors.Is(err, xferr.ErrNilError),
		errors.Is(err, xferr.ErrInvalidSeverity),
		errors.Is(err, xferr.ErrInvalidCategory),
		errors.Is(err, xferr.ErrInvalidDropReason),
		errors.Is(err, xferr.ErrInvalidComponentType):
		return ErrBadRequest

	// 수신자 없음 → 503 Service Unavailable
	case errors.Is(err, xferr.ErrNoReceiver):
		return ErrServiceUnavailable

	// 상태 충돌 → 409 Conflict
	case errors.Is(err, xferr.ErrSameStateTransition),
		errors.Is(err, engine.ErrFlowAlreadyDeployed),
		errors.Is(err, engine.ErrFlowNotLoaded),
		errors.Is(err, engine.ErrFlowNotRunning),
		errors.Is(err, engine.ErrFlowNotPaused),
		errors.Is(err, engine.ErrFlowNotStopped),
		errors.Is(err, agent.ErrAgentAlreadyExists),
		errors.Is(err, agent.ErrAgentNotRunning),
		errors.Is(err, agent.ErrAgentAlreadyStopped),
		errors.Is(err, agent.ErrConfigImmutable),
		errors.Is(err, agent.ErrInvalidStateTransition):
		return ErrConflict

	// 리소스 없음 → 404 Not Found
	case errors.Is(err, engine.ErrFlowNotFound),
		errors.Is(err, agent.ErrAgentNotFound):
		return ErrNotFound

	// 유효성 검증 실패 → 422 Unprocessable Entity
	case errors.Is(err, engine.ErrFlowValidationFailed),
		errors.Is(err, engine.ErrCycleDetected),
		errors.Is(err, agent.ErrInvalidConfig),
		errors.Is(err, agent.ErrTransportNotAvailable):
		return ErrValidationFailed.WithMessage(err.Error())

	// 노드 시작 실패 → 422 Unprocessable Entity
	case errors.Is(err, engine.ErrNodeStartFailed):
		return ErrValidationFailed.WithMessage(err.Error())

	// 셧다운 타임아웃 → 408 Request Timeout
	case errors.Is(err, engine.ErrShutdownTimeout):
		return ErrRequestTimeout.WithMessage(err.Error())

	// 임계값 초과 → 429 Rate Limit Exceeded
	case errors.Is(err, xferr.ErrAlertThresholdExceeded):
		return ErrRateLimitExceeded

	// 알 수 없는 에러 → 500 Internal Server Error
	default:
		return ErrInternalServer
	}
}
