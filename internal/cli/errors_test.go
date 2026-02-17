package cli

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLIError_ErrorInterface - CLIError 가 error 인터페이스를 구현하는지 검증
func TestCLIError_ErrorInterface(t *testing.T) {
	var err error = &CLIError{Message: "테스트 에러"}
	assert.NotNil(t, err, "CLIError 는 error 인터페이스를 구현해야 합니다")
}

// TestCLIError_Error - CLIError.Error() 가 Message 를 반환하는지 검증
func TestCLIError_Error(t *testing.T) {
	cliErr := &CLIError{
		Message:  "테스트 에러 메시지",
		Hint:     "힌트 메시지",
		Cause:    errors.New("원인 에러"),
		ExitCode: 1,
	}

	assert.Equal(t, "테스트 에러 메시지", cliErr.Error(),
		"Error() 는 Message 필드를 반환해야 합니다")
}

// TestCLIError_Unwrap - CLIError.Unwrap() 이 Cause 를 반환하는지 검증
func TestCLIError_Unwrap(t *testing.T) {
	cause := errors.New("원인 에러")
	cliErr := &CLIError{
		Message: "테스트",
		Cause:   cause,
	}

	assert.Equal(t, cause, cliErr.Unwrap(),
		"Unwrap() 은 Cause 를 반환해야 합니다")
}

// TestCLIError_Unwrap_Nil - Cause 가 nil 인 경우 Unwrap() 이 nil 을 반환하는지 검증
func TestCLIError_Unwrap_Nil(t *testing.T) {
	cliErr := &CLIError{Message: "테스트"}
	assert.Nil(t, cliErr.Unwrap(),
		"Cause 가 nil 이면 Unwrap() 도 nil 이어야 합니다")
}

// TestErrServerUnreachable - 서버 연결 실패 에러 생성 검증
func TestErrServerUnreachable(t *testing.T) {
	err := ErrServerUnreachable("http://localhost:8080")

	assert.Equal(t, "서버에 연결할 수 없습니다: http://localhost:8080", err.Message,
		"서버 주소가 메시지에 포함되어야 합니다")
	assert.Equal(t, "xflow config server <url> 명령어로 서버 주소를 확인하세요", err.Hint,
		"서버 설정 힌트가 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrAuthenticationFailed - 인증 실패 에러 생성 검증
func TestErrAuthenticationFailed(t *testing.T) {
	err := ErrAuthenticationFailed()

	assert.Equal(t, "인증에 실패했습니다. 토큰을 확인해주세요", err.Message,
		"인증 실패 메시지가 정확해야 합니다")
	assert.Equal(t, "xflow config token <token> 명령어로 토큰을 설정하세요", err.Hint,
		"토큰 설정 힌트가 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrPermissionDenied - 권한 거부 에러 생성 검증
func TestErrPermissionDenied(t *testing.T) {
	err := ErrPermissionDenied()

	assert.Equal(t, "이 작업을 수행할 권한이 없습니다", err.Message,
		"권한 거부 메시지가 정확해야 합니다")
	assert.Empty(t, err.Hint,
		"권한 거부 에러에는 힌트가 없어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrResourceNotFound - 리소스 미발견 에러 생성 검증
func TestErrResourceNotFound(t *testing.T) {
	err := ErrResourceNotFound("workflow", "wf-123")

	assert.Equal(t, "리소스를 찾을 수 없습니다: workflow wf-123", err.Message,
		"리소스 타입과 ID 가 메시지에 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrInvalidInput - 잘못된 입력 에러 생성 검증
func TestErrInvalidInput(t *testing.T) {
	err := ErrInvalidInput("이름은 필수 항목입니다")

	assert.Equal(t, "잘못된 입력: 이름은 필수 항목입니다", err.Message,
		"입력 상세 내용이 메시지에 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrFileNotFound - 파일 미발견 에러 생성 검증
func TestErrFileNotFound(t *testing.T) {
	err := ErrFileNotFound("/path/to/file.yaml")

	assert.Equal(t, "파일을 찾을 수 없습니다: /path/to/file.yaml", err.Message,
		"파일 경로가 메시지에 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestErrConfigNotInitialized - 설정 미초기화 에러 생성 검증
func TestErrConfigNotInitialized(t *testing.T) {
	err := ErrConfigNotInitialized()

	assert.Equal(t, "설정이 초기화되지 않았습니다", err.Message,
		"설정 미초기화 메시지가 정확해야 합니다")
	assert.Equal(t, "xflow config init 명령어로 설정을 초기화하세요", err.Hint,
		"설정 초기화 힌트가 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode,
		"종료 코드는 1 이어야 합니다")
}

// TestMapAPIError - HTTP 상태 코드와 응답 본문을 CLIError 로 매핑하는지 검증
func TestMapAPIError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           []byte
		expectedMsg    string
		expectedHint   string
		checkContains  bool
	}{
		{
			name:       "401 인증 실패",
			statusCode: 401,
			body:       makeErrorBody("UNAUTHORIZED", "인증 토큰이 만료되었습니다"),
			expectedMsg: "인증에 실패했습니다. 토큰을 확인해주세요",
			expectedHint: "xflow config token <token> 명령어로 토큰을 설정하세요",
		},
		{
			name:       "403 권한 거부",
			statusCode: 403,
			body:       makeErrorBody("FORBIDDEN", "관리자 권한이 필요합니다"),
			expectedMsg: "이 작업을 수행할 권한이 없습니다",
		},
		{
			name:          "404 리소스 미발견",
			statusCode:    404,
			body:          makeErrorBody("NOT_FOUND", "워크플로우를 찾을 수 없습니다"),
			expectedMsg:   "워크플로우를 찾을 수 없습니다",
			checkContains: true,
		},
		{
			name:          "400 잘못된 요청",
			statusCode:    400,
			body:          makeErrorBody("INVALID_INPUT", "이름은 필수입니다"),
			expectedMsg:   "이름은 필수입니다",
			checkContains: true,
		},
		{
			name:          "500 서버 내부 에러",
			statusCode:    500,
			body:          makeErrorBody("INTERNAL_ERROR", "내부 서버 에러"),
			expectedMsg:   "내부 서버 에러",
			checkContains: true,
		},
		{
			name:          "잘못된 JSON 본문",
			statusCode:    500,
			body:          []byte("not json"),
			expectedMsg:   "서버 에러",
			checkContains: true,
		},
		{
			name:          "빈 본문",
			statusCode:    502,
			body:          []byte{},
			expectedMsg:   "서버 에러",
			checkContains: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliErr := MapAPIError(tt.statusCode, tt.body)
			require.NotNil(t, cliErr, "MapAPIError 는 nil 을 반환하면 안됩니다")

			if tt.checkContains {
				assert.Contains(t, cliErr.Message, tt.expectedMsg,
					"에러 메시지에 예상 내용이 포함되어야 합니다")
			} else {
				assert.Equal(t, tt.expectedMsg, cliErr.Message,
					"에러 메시지가 정확해야 합니다")
			}

			if tt.expectedHint != "" {
				assert.Equal(t, tt.expectedHint, cliErr.Hint,
					"힌트 메시지가 정확해야 합니다")
			}
		})
	}
}

// TestFormatError_Basic - 기본 에러 포맷팅 검증
func TestFormatError_Basic(t *testing.T) {
	err := &CLIError{
		Message:  "테스트 에러",
		ExitCode: 1,
	}

	result := FormatError(err, false)
	assert.Contains(t, result, "테스트 에러",
		"포맷된 결과에 에러 메시지가 포함되어야 합니다")
}

// TestFormatError_WithHint - 힌트 포함 에러 포맷팅 검증
func TestFormatError_WithHint(t *testing.T) {
	err := &CLIError{
		Message:  "설정이 초기화되지 않았습니다",
		Hint:     "xflow config init 명령어로 설정을 초기화하세요",
		ExitCode: 1,
	}

	result := FormatError(err, false)
	assert.Contains(t, result, "설정이 초기화되지 않았습니다",
		"포맷된 결과에 에러 메시지가 포함되어야 합니다")
	assert.Contains(t, result, "xflow config init",
		"포맷된 결과에 힌트가 포함되어야 합니다")
}

// TestFormatError_Verbose - verbose 모드에서 원인 에러 표시 검증
func TestFormatError_Verbose(t *testing.T) {
	cause := errors.New("연결 거부됨")
	err := &CLIError{
		Message:  "서버 에러",
		Cause:    cause,
		ExitCode: 1,
	}

	// verbose=false 에서는 원인이 표시되지 않아야 함
	resultNonVerbose := FormatError(err, false)
	assert.NotContains(t, resultNonVerbose, "연결 거부됨",
		"verbose=false 에서는 원인 에러가 표시되지 않아야 합니다")

	// verbose=true 에서는 원인이 표시되어야 함
	resultVerbose := FormatError(err, true)
	assert.Contains(t, resultVerbose, "연결 거부됨",
		"verbose=true 에서는 원인 에러가 표시되어야 합니다")
}

// TestFormatError_NilError - nil CLIError 포맷팅 검증
func TestFormatError_NilError(t *testing.T) {
	result := FormatError(nil, false)
	assert.Empty(t, result, "nil 에러의 포맷 결과는 빈 문자열이어야 합니다")
}

// TestSentinelErrors_Distinct - 모든 에러 생성자가 고유한 메시지를 생성하는지 검증
func TestSentinelErrors_Distinct(t *testing.T) {
	errs := []*CLIError{
		ErrServerUnreachable("http://localhost"),
		ErrAuthenticationFailed(),
		ErrPermissionDenied(),
		ErrResourceNotFound("type", "id"),
		ErrInvalidInput("detail"),
		ErrFileNotFound("/path"),
		ErrConfigNotInitialized(),
	}

	for i := 0; i < len(errs); i++ {
		for j := i + 1; j < len(errs); j++ {
			assert.NotEqual(t, errs[i].Message, errs[j].Message,
				"에러 %d 와 %d 의 메시지가 동일합니다", i, j)
		}
	}
}

// makeErrorBody - API 에러 응답 JSON 본문을 생성하는 헬퍼
func makeErrorBody(code, message string) []byte {
	resp := map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}
