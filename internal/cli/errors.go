package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CLIError is a user-facing error with optional hint and exit code.
type CLIError struct {
	Message  string
	Hint     string
	Cause    error
	ExitCode int
}

// Error implements the error interface.
func (e *CLIError) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause for errors.Is/As compatibility.
func (e *CLIError) Unwrap() error {
	return e.Cause
}

// ErrServerUnreachable creates an error for server connection failure.
func ErrServerUnreachable(url string) *CLIError {
	return &CLIError{
		Message:  fmt.Sprintf("서버에 연결할 수 없습니다: %s", url),
		Hint:     "xflow config server <url> 명령어로 서버 주소를 확인하세요",
		ExitCode: 1,
	}
}

// ErrAuthenticationFailed creates an error for authentication failure.
func ErrAuthenticationFailed() *CLIError {
	return &CLIError{
		Message:  "인증에 실패했습니다. 토큰을 확인해주세요",
		Hint:     "xflow config token <token> 명령어로 토큰을 설정하세요",
		ExitCode: 1,
	}
}

// ErrPermissionDenied creates an error for permission denial.
func ErrPermissionDenied() *CLIError {
	return &CLIError{
		Message:  "이 작업을 수행할 권한이 없습니다",
		ExitCode: 1,
	}
}

// ErrResourceNotFound creates an error for missing resources.
func ErrResourceNotFound(resourceType, id string) *CLIError {
	return &CLIError{
		Message:  fmt.Sprintf("리소스를 찾을 수 없습니다: %s %s", resourceType, id),
		ExitCode: 1,
	}
}

// ErrInvalidInput creates an error for invalid user input.
func ErrInvalidInput(detail string) *CLIError {
	return &CLIError{
		Message:  fmt.Sprintf("잘못된 입력: %s", detail),
		ExitCode: 1,
	}
}

// ErrFileNotFound creates an error for missing files.
func ErrFileNotFound(path string) *CLIError {
	return &CLIError{
		Message:  fmt.Sprintf("파일을 찾을 수 없습니다: %s", path),
		ExitCode: 1,
	}
}

// ErrConfigNotInitialized creates an error for uninitialized configuration.
func ErrConfigNotInitialized() *CLIError {
	return &CLIError{
		Message:  "설정이 초기화되지 않았습니다",
		Hint:     "xflow config init 명령어로 설정을 초기화하세요",
		ExitCode: 1,
	}
}

// apiErrorResponse is the internal structure for parsing API error bodies.
type apiErrorResponse struct {
	Success bool            `json:"success"`
	Error   *apiErrorDetail `json:"error,omitempty"`
}

// apiErrorDetail holds the error code and message from API responses.
type apiErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MapAPIError maps an HTTP status code and response body to a CLIError.
func MapAPIError(statusCode int, body []byte) *CLIError {
	// Handle 401 and 403 with predefined errors regardless of body content
	switch statusCode {
	case 401:
		return ErrAuthenticationFailed()
	case 403:
		return ErrPermissionDenied()
	}

	// HTTPS 서버에 평문 HTTP 요청을 보낸 경우 서버는 평문 본문으로 응답한다.
	// 이 경우 일반 "서버 에러 (코드: ...)" 폴백보다 우선하여 명확한 안내를 제공한다.
	if strings.Contains(string(body), "HTTP request to an HTTPS server") {
		return &CLIError{
			Message:  "서버가 HTTPS 를 사용하지만 http:// 요청이 전송되었습니다",
			Hint:     "--server https://... 를 사용하거나 설정의 server.url 을 https URL 로 변경하세요",
			ExitCode: 1,
		}
	}

	// Try to parse the error body
	var apiResp apiErrorResponse
	if err := json.Unmarshal(body, &apiResp); err == nil && apiResp.Error != nil {
		msg := apiResp.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("서버 에러 (코드: %d)", statusCode)
		}
		return &CLIError{
			Message:  msg,
			ExitCode: 1,
		}
	}

	// Fallback for unparseable bodies
	return &CLIError{
		Message:  fmt.Sprintf("서버 에러 (코드: %d)", statusCode),
		ExitCode: 1,
	}
}

// FormatError formats a CLIError for display to the user.
// When verbose is true, the underlying cause is included.
func FormatError(err *CLIError, verbose bool) string {
	if err == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(err.Message)

	if err.Hint != "" {
		b.WriteString("\n힌트: ")
		b.WriteString(err.Hint)
	}

	if verbose && err.Cause != nil {
		b.WriteString("\n원인: ")
		b.WriteString(err.Cause.Error())
	}

	return b.String()
}
