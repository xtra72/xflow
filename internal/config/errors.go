package config

import (
	"errors"
	"strings"
)

// 센티널 에러 - errors.Is()와 호환됨
var (
	ErrInvalidPort          = errors.New("config: port must be between 1 and 65535")
	ErrFileNotFound         = errors.New("config: required file not found")
	ErrRequiredField        = errors.New("config: required field is missing")
	ErrInvalidStorageType   = errors.New("config: unsupported storage type")
	ErrInvalidLogLevel      = errors.New("config: invalid log level")
	ErrInvalidPositiveValue = errors.New("config: value must be positive")
	ErrInvalidDuration      = errors.New("config: invalid duration string")
	ErrImmutableKey         = errors.New("config: key is immutable at runtime")
	ErrConfigNotLoaded      = errors.New("config: configuration not loaded")
	ErrInvalidLogFormat     = errors.New("config: invalid log format (must be 'json' or 'text')")
	ErrInvalidLogOutput     = errors.New("config: invalid log output target")
	// ErrInvalidRemoteMode 는 remote_management.mode 가 server|client|disabled 가
	// 아닐 때 반환된다 (@SPEC:SPEC-REMOTE-001 M1, REQ-A01).
	ErrInvalidRemoteMode = errors.New("config: invalid remote_management.mode (must be 'server', 'client', or 'disabled')")
	// ErrInsecureTransport 는 remote_management.require_secure=true 인데 비-dev 환경
	// 에서 평문 전송(ws://)이나 TLS 미설정이 감지될 때 반환된다
	// (@SPEC:SPEC-REMOTE-001 M6, REQ-F01).
	ErrInsecureTransport = errors.New("config: insecure remote transport rejected (require_secure=true; use wss:// and enable TLS in non-dev)")
)

// ValidationErrors - 여러 유효성 검증 에러를 집계하는 타입
type ValidationErrors struct {
	errs []error
}

// Error - error 인터페이스 구현, 모든 에러 메시지를 줄바꿈으로 연결
func (ve *ValidationErrors) Error() string {
	msgs := make([]string, len(ve.errs))
	for i, err := range ve.errs {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n")
}

// Errors - 집계된 에러 슬라이스 반환
func (ve *ValidationErrors) Errors() []error {
	return ve.errs
}

// HasErrors - 에러가 존재하는지 확인
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.errs) > 0
}

// Add - 에러를 집계 목록에 추가
func (ve *ValidationErrors) Add(err error) {
	ve.errs = append(ve.errs, err)
}

// Unwrap - errors.Is() 호환을 위한 다중 에러 언래핑
func (ve *ValidationErrors) Unwrap() []error {
	return ve.errs
}
