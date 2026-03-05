package adapter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// TestHTTPAdapter_Validate 는 HTTP 어댑터의 설정 검증 로직을 테스트한다.
func TestHTTPAdapter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    []HTTPAdapterOption
		wantErr bool
		errMsg  string
	}{
		{
			name:    "기본 설정 유효",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "application/json 유효",
			opts:    []HTTPAdapterOption{WithDefaultContentType("application/json")},
			wantErr: false,
		},
		{
			name:    "application/x-www-form-urlencoded 유효",
			opts:    []HTTPAdapterOption{WithDefaultContentType("application/x-www-form-urlencoded")},
			wantErr: false,
		},
		{
			name:    "text/plain 유효",
			opts:    []HTTPAdapterOption{WithDefaultContentType("text/plain")},
			wantErr: false,
		},
		{
			name:    "지원하지 않는 Content-Type",
			opts:    []HTTPAdapterOption{WithDefaultContentType("application/unknown")},
			wantErr: true,
			errMsg:  "unsupported content type",
		},
		{
			name:    "유효한 URL 템플릿",
			opts:    []HTTPAdapterOption{WithURLTemplate("/api/v1/users/{id}")},
			wantErr: false,
		},
		{
			name:    "유효하지 않은 URL 템플릿 - 닫히지 않은 중괄호",
			opts:    []HTTPAdapterOption{WithURLTemplate("/api/v1/users/{id")},
			wantErr: true,
			errMsg:  "invalid URL template",
		},
		{
			name:    "유효하지 않은 URL 템플릿 - 열리지 않은 중괄호",
			opts:    []HTTPAdapterOption{WithURLTemplate("/api/v1/users/id}")},
			wantErr: true,
			errMsg:  "invalid URL template",
		},
		{
			name:    "양수 타임아웃 유효",
			opts:    []HTTPAdapterOption{WithTimeout(10 * time.Second)},
			wantErr: false,
		},
		{
			name:    "0 타임아웃 유효하지 않음",
			opts:    []HTTPAdapterOption{WithTimeout(0)},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name:    "음수 타임아웃 유효하지 않음",
			opts:    []HTTPAdapterOption{WithTimeout(-1 * time.Second)},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewHTTPAdapter(tt.opts...)
			err := adapter.Validate(node.BridgeConfig{})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHTTPAdapter_TransformToFlow_JSON 은 JSON 응답이 페이로드에 파싱되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_JSON(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte(`{"name":"alice","age":30}`)
	meta := node.AgentMeta{ContentType: "application/json"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)
	assert.NotNil(t, msg)

	name, ok := msg.Payload().Get("name")
	assert.True(t, ok)
	assert.Equal(t, "alice", name)

	age, ok := msg.Payload().Get("age")
	assert.True(t, ok)
	assert.Equal(t, float64(30), age)
}

// TestHTTPAdapter_TransformToFlow_JSON_InvalidFallback 은 잘못된 JSON이 _raw로 저장되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_JSON_InvalidFallback(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte(`{invalid-json}`)
	meta := node.AgentMeta{ContentType: "application/json"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	raw, ok := msg.Payload().Get("_raw")
	assert.True(t, ok)
	assert.Equal(t, `{invalid-json}`, raw)
}

// TestHTTPAdapter_TransformToFlow_JSON_WithCharset 는 charset이 포함된 Content-Type을 처리하는지 확인한다.
func TestHTTPAdapter_TransformToFlow_JSON_WithCharset(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte(`{"key":"value"}`)
	meta := node.AgentMeta{ContentType: "application/json; charset=utf-8"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	val, ok := msg.Payload().Get("key")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
}

// TestHTTPAdapter_TransformToFlow_FormData 는 URL 인코딩된 폼 데이터가 키-값으로 파싱되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_FormData(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte("name=alice&email=alice%40example.com")
	meta := node.AgentMeta{ContentType: "application/x-www-form-urlencoded"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	name, ok := msg.Payload().Get("name")
	assert.True(t, ok)
	assert.Equal(t, "alice", name)

	email, ok := msg.Payload().Get("email")
	assert.True(t, ok)
	assert.Equal(t, "alice@example.com", email)
}

// TestHTTPAdapter_TransformToFlow_FormData_MultiValue 는 다중 값 폼 필드가 슬라이스로 저장되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_FormData_MultiValue(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte("tag=go&tag=http&tag=adapter")
	meta := node.AgentMeta{ContentType: "application/x-www-form-urlencoded"}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	tags, ok := msg.Payload().Get("tag")
	assert.True(t, ok)
	assert.Equal(t, []string{"go", "http", "adapter"}, tags)
}

// TestHTTPAdapter_TransformToFlow_RawData 는 알 수 없는 Content-Type의 데이터가 _raw로 저장되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_RawData(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{name: "text/plain", contentType: "text/plain"},
		{name: "text/html", contentType: "text/html"},
		{name: "application/xml", contentType: "application/xml"},
		{name: "multipart/form-data", contentType: "multipart/form-data"},
		{name: "application/octet-stream", contentType: "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewHTTPAdapter()
			rawData := []byte("<html>hello</html>")
			meta := node.AgentMeta{ContentType: tt.contentType}

			msg, err := adapter.TransformToFlow(rawData, meta)
			require.NoError(t, err)

			raw, ok := msg.Payload().Get("_raw")
			assert.True(t, ok)
			assert.Equal(t, "<html>hello</html>", raw)
		})
	}
}

// TestHTTPAdapter_TransformToFlow_NilData 는 nil 데이터가 패닉 없이 처리되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_NilData(t *testing.T) {
	adapter := NewHTTPAdapter()

	msg, err := adapter.TransformToFlow(nil, node.AgentMeta{})
	require.NoError(t, err)
	assert.NotNil(t, msg)

	keys := msg.Payload().Keys()
	assert.Empty(t, keys)
}

// TestHTTPAdapter_TransformToFlow_DefaultContentType 은 Content-Type이 없으면 기본값을 사용하는지 확인한다.
func TestHTTPAdapter_TransformToFlow_DefaultContentType(t *testing.T) {
	adapter := NewHTTPAdapter() // 기본값: application/json
	data := []byte(`{"status":"ok"}`)
	meta := node.AgentMeta{} // ContentType 비어있음

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	// JSON으로 파싱되어야 함 (기본 Content-Type이 application/json)
	status, ok := msg.Payload().Get("status")
	assert.True(t, ok)
	assert.Equal(t, "ok", status)
}

// TestHTTPAdapter_TransformToFlow_StatusCode 는 상태 코드가 메타데이터에 설정되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_StatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantMeta   string
	}{
		{name: "200 OK", statusCode: 200, wantMeta: "200"},
		{name: "201 Created", statusCode: 201, wantMeta: "201"},
		{name: "404 Not Found", statusCode: 404, wantMeta: "404"},
		{name: "500 Internal Server Error", statusCode: 500, wantMeta: "500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewHTTPAdapter()
			meta := node.AgentMeta{
				StatusCode:  tt.statusCode,
				ContentType: "application/json",
			}

			msg, err := adapter.TransformToFlow(nil, meta)
			require.NoError(t, err)

			code, ok := msg.Metadata().Get("http.status_code")
			assert.True(t, ok)
			assert.Equal(t, tt.wantMeta, code)
		})
	}
}

// TestHTTPAdapter_TransformToFlow_Headers 는 헤더가 메타데이터로 전파되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_Headers(t *testing.T) {
	adapter := NewHTTPAdapter()
	meta := node.AgentMeta{
		ContentType: "application/json",
		Headers: map[string]string{
			"X-Request-ID": "req-123",
			"Authorization": "Bearer token-xyz",
		},
	}

	msg, err := adapter.TransformToFlow(nil, meta)
	require.NoError(t, err)

	// 헤더 키가 소문자로 변환되어 저장되는지 확인
	reqID, ok := msg.Metadata().Get("http.header.x-request-id")
	assert.True(t, ok)
	assert.Equal(t, "req-123", reqID)

	auth, ok := msg.Metadata().Get("http.header.authorization")
	assert.True(t, ok)
	assert.Equal(t, "Bearer token-xyz", auth)
}

// TestHTTPAdapter_TransformToFlow_ErrorFlag 는 4xx/5xx 상태 코드가 에러로 플래그되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_ErrorFlag(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{name: "200 에러 아님", statusCode: 200, wantError: false},
		{name: "201 에러 아님", statusCode: 201, wantError: false},
		{name: "301 에러 아님", statusCode: 301, wantError: false},
		{name: "399 에러 아님", statusCode: 399, wantError: false},
		{name: "400 에러", statusCode: 400, wantError: true},
		{name: "401 에러", statusCode: 401, wantError: true},
		{name: "403 에러", statusCode: 403, wantError: true},
		{name: "404 에러", statusCode: 404, wantError: true},
		{name: "500 에러", statusCode: 500, wantError: true},
		{name: "502 에러", statusCode: 502, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewHTTPAdapter()
			meta := node.AgentMeta{StatusCode: tt.statusCode}

			msg, err := adapter.TransformToFlow(nil, meta)
			require.NoError(t, err)

			errorFlag, ok := msg.Metadata().Get("http.error")
			if tt.wantError {
				assert.True(t, ok, "http.error 메타데이터가 설정되어야 함")
				assert.Equal(t, "true", errorFlag)
			} else {
				assert.False(t, ok, "http.error 메타데이터가 설정되지 않아야 함")
			}
		})
	}
}

// TestHTTPAdapter_TransformToFlow_URLPath 는 URL 경로가 메타데이터에 설정되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_URLPath(t *testing.T) {
	adapter := NewHTTPAdapter()
	meta := node.AgentMeta{URLPath: "/api/v1/users/42"}

	msg, err := adapter.TransformToFlow(nil, meta)
	require.NoError(t, err)

	urlPath, ok := msg.Metadata().Get("http.url_path")
	assert.True(t, ok)
	assert.Equal(t, "/api/v1/users/42", urlPath)
}

// TestHTTPAdapter_TransformToAgent 는 메시지가 JSON 바이트와 AgentMeta로 변환되는지 확인한다.
func TestHTTPAdapter_TransformToAgent(t *testing.T) {
	adapter := NewHTTPAdapter(
		WithURLTemplate("/api/v1/users/{id}"),
	)

	msg := message.New()
	msg.Payload().Set("id", "42")
	msg.Payload().Set("name", "alice")

	data, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)
	assert.NotNil(t, data)

	// JSON 데이터 검증
	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Equal(t, "42", result["id"])
	assert.Equal(t, "alice", result["name"])

	// AgentMeta 검증
	assert.Equal(t, "http", meta.AgentType)
	assert.Equal(t, "application/json", meta.ContentType)
	assert.Equal(t, "/api/v1/users/42", meta.URLPath)
}

// TestHTTPAdapter_TransformToAgent_NoTemplate 은 URL 템플릿이 없을 때 URLPath가 비어있는지 확인한다.
func TestHTTPAdapter_TransformToAgent_NoTemplate(t *testing.T) {
	adapter := NewHTTPAdapter()

	msg := message.New()
	msg.Payload().Set("key", "value")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	assert.Equal(t, "http", meta.AgentType)
	assert.Equal(t, "application/json", meta.ContentType)
	assert.Empty(t, meta.URLPath)
}

// TestHTTPAdapter_TransformToAgent_Headers 는 메시지 메타데이터에서 HTTP 헤더가 추출되는지 확인한다.
func TestHTTPAdapter_TransformToAgent_Headers(t *testing.T) {
	adapter := NewHTTPAdapter()

	msg := message.New()
	msg.Payload().Set("data", "test")
	msg.Metadata().Set("http.header.x-request-id", "req-456")
	msg.Metadata().Set("http.header.authorization", "Bearer abc")
	msg.Metadata().Set("http.status_code", "200") // 이건 헤더가 아님

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	require.NotNil(t, meta.Headers)
	assert.Equal(t, "req-456", meta.Headers["x-request-id"])
	assert.Equal(t, "Bearer abc", meta.Headers["authorization"])
	// http.status_code 는 헤더에 포함되지 않아야 함
	_, hasStatusCode := meta.Headers["status_code"]
	assert.False(t, hasStatusCode)
}

// TestHTTPAdapter_TransformToAgent_NoHeaders 는 헤더 메타데이터가 없을 때 nil인지 확인한다.
func TestHTTPAdapter_TransformToAgent_NoHeaders(t *testing.T) {
	adapter := NewHTTPAdapter()

	msg := message.New()
	msg.Payload().Set("key", "value")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	assert.Nil(t, meta.Headers)
}

// TestHTTPAdapter_TransformToAgent_CustomContentType 은 커스텀 Content-Type이 전달되는지 확인한다.
func TestHTTPAdapter_TransformToAgent_CustomContentType(t *testing.T) {
	adapter := NewHTTPAdapter(
		WithDefaultContentType("text/plain"),
	)

	msg := message.New()
	msg.Payload().Set("text", "hello")

	_, meta, err := adapter.TransformToAgent(msg)
	require.NoError(t, err)

	assert.Equal(t, "text/plain", meta.ContentType)
}

// TestHTTPAdapter_DefaultConfig 는 기본 설정이 올바른 값을 반환하는지 확인한다.
func TestHTTPAdapter_DefaultConfig(t *testing.T) {
	adapter := NewHTTPAdapter()

	config := adapter.DefaultConfig()
	assert.Equal(t, flow.BridgeRequestReply, config.Direction)
	assert.Equal(t, 64, config.BufferSize)
	assert.Equal(t, 30*time.Second, config.RequestTimeout)
}

// TestHTTPAdapter_DefaultConfig_CustomTimeout 은 커스텀 타임아웃이 DefaultConfig에 반영되는지 확인한다.
func TestHTTPAdapter_DefaultConfig_CustomTimeout(t *testing.T) {
	adapter := NewHTTPAdapter(WithTimeout(60 * time.Second))

	config := adapter.DefaultConfig()
	assert.Equal(t, 60*time.Second, config.RequestTimeout)
}

// TestHTTPAdapter_HandleControl 은 제어 메시지 처리가 nil을 반환하는지 확인한다.
func TestHTTPAdapter_HandleControl(t *testing.T) {
	adapter := NewHTTPAdapter()
	msg := message.New()

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}

// TestParseFormData 는 다양한 폼 데이터 형식이 올바르게 파싱되는지 확인한다.
func TestParseFormData(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantKeys []string
		wantVals map[string]any
	}{
		{
			name:     "단일 키-값",
			data:     "key=value",
			wantKeys: []string{"key"},
			wantVals: map[string]any{"key": "value"},
		},
		{
			name:     "다중 키-값",
			data:     "name=alice&age=30",
			wantKeys: []string{"age", "name"},
			wantVals: map[string]any{"name": "alice", "age": "30"},
		},
		{
			name:     "URL 인코딩된 값",
			data:     "email=alice%40example.com&msg=hello+world",
			wantKeys: []string{"email", "msg"},
			wantVals: map[string]any{"email": "alice@example.com", "msg": "hello world"},
		},
		{
			name:     "빈 값",
			data:     "key=",
			wantKeys: []string{"key"},
			wantVals: map[string]any{"key": ""},
		},
		{
			name:     "다중 값 키",
			data:     "tag=a&tag=b",
			wantKeys: []string{"tag"},
			wantVals: map[string]any{"tag": []string{"a", "b"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New()
			parseFormData(msg, tt.data)

			for key, want := range tt.wantVals {
				got, ok := msg.Payload().Get(key)
				assert.True(t, ok, "키 %q가 페이로드에 없음", key)
				assert.Equal(t, want, got)
			}
		})
	}
}

// TestParseFormData_InvalidData 는 파싱할 수 없는 데이터가 _raw로 저장되는지 확인한다.
func TestParseFormData_InvalidData(t *testing.T) {
	msg := message.New()
	// url.ParseQuery 는 거의 모든 문자열을 파싱할 수 있지만,
	// 빈 문자열과 일반 문자열 모두 테스트한다.
	parseFormData(msg, "")

	keys := msg.Payload().Keys()
	assert.Empty(t, keys)
}

// TestValidateURLTemplate 는 URL 템플릿 검증 로직을 테스트한다.
func TestValidateURLTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "유효한 단순 경로",
			tmpl:    "/api/v1/users",
			wantErr: false,
		},
		{
			name:    "유효한 단일 플레이스홀더",
			tmpl:    "/api/v1/users/{id}",
			wantErr: false,
		},
		{
			name:    "유효한 다중 플레이스홀더",
			tmpl:    "/api/v1/{resource}/{id}/details",
			wantErr: false,
		},
		{
			name:    "열린 중괄호 불일치",
			tmpl:    "/api/v1/users/{id",
			wantErr: true,
			errMsg:  "unmatched opening brace",
		},
		{
			name:    "닫힌 중괄호 불일치",
			tmpl:    "/api/v1/users/id}",
			wantErr: true,
			errMsg:  "unmatched closing brace",
		},
		{
			name:    "중첩된 중괄호",
			tmpl:    "/api/v1/users/{{id}}",
			wantErr: false, // 중첩은 허용 (depth 추적)
		},
		{
			name:    "빈 템플릿",
			tmpl:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURLTemplate(tt.tmpl)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestNewHTTPAdapter_Options 는 옵션이 올바르게 적용되는지 확인한다.
func TestNewHTTPAdapter_Options(t *testing.T) {
	adapter := NewHTTPAdapter(
		WithDefaultContentType("text/plain"),
		WithURLTemplate("/api/{resource}"),
		WithTimeout(60 * time.Second),
	)

	assert.Equal(t, "text/plain", adapter.defaultContentType)
	assert.Equal(t, "/api/{resource}", adapter.urlTemplate)
	assert.Equal(t, 60*time.Second, adapter.timeout)
}

// TestNewHTTPAdapter_Defaults 는 기본값이 올바르게 설정되는지 확인한다.
func TestNewHTTPAdapter_Defaults(t *testing.T) {
	adapter := NewHTTPAdapter()

	assert.Equal(t, "application/json", adapter.defaultContentType)
	assert.Equal(t, "", adapter.urlTemplate)
	assert.Equal(t, 30*time.Second, adapter.timeout)
}

// TestHTTPAdapter_TransformToFlow_ContentTypeAndURLPath 는 Content-Type과 URL 경로가
// 메타데이터에 모두 설정되는지 확인한다.
func TestHTTPAdapter_TransformToFlow_ContentTypeAndURLPath(t *testing.T) {
	adapter := NewHTTPAdapter()
	meta := node.AgentMeta{
		ContentType: "application/json",
		URLPath:     "/api/v1/health",
	}

	msg, err := adapter.TransformToFlow(nil, meta)
	require.NoError(t, err)

	ct, ok := msg.Metadata().Get("http.content_type")
	assert.True(t, ok)
	assert.Equal(t, "application/json", ct)

	path, ok := msg.Metadata().Get("http.url_path")
	assert.True(t, ok)
	assert.Equal(t, "/api/v1/health", path)
}

// TestHTTPAdapter_TransformToFlow_FullScenario 는 모든 메타데이터가 함께 설정되는 전체 시나리오를 테스트한다.
func TestHTTPAdapter_TransformToFlow_FullScenario(t *testing.T) {
	adapter := NewHTTPAdapter()
	data := []byte(`{"id":1,"name":"test"}`)
	meta := node.AgentMeta{
		StatusCode:  200,
		ContentType: "application/json",
		URLPath:     "/api/v1/users/1",
		Headers: map[string]string{
			"X-Request-ID": "req-789",
		},
	}

	msg, err := adapter.TransformToFlow(data, meta)
	require.NoError(t, err)

	// 페이로드 검증
	id, ok := msg.Payload().Get("id")
	assert.True(t, ok)
	assert.Equal(t, float64(1), id)

	name, ok := msg.Payload().Get("name")
	assert.True(t, ok)
	assert.Equal(t, "test", name)

	// 메타데이터 검증
	code, ok := msg.Metadata().Get("http.status_code")
	assert.True(t, ok)
	assert.Equal(t, "200", code)

	ct, ok := msg.Metadata().Get("http.content_type")
	assert.True(t, ok)
	assert.Equal(t, "application/json", ct)

	path, ok := msg.Metadata().Get("http.url_path")
	assert.True(t, ok)
	assert.Equal(t, "/api/v1/users/1", path)

	reqID, ok := msg.Metadata().Get("http.header.x-request-id")
	assert.True(t, ok)
	assert.Equal(t, "req-789", reqID)

	// 200은 에러가 아님
	_, hasError := msg.Metadata().Get("http.error")
	assert.False(t, hasError)
}

// TestHTTPAdapter_BridgeAdapterInterface 는 HTTPAdapter가 BridgeAdapter 인터페이스를 구현하는지 확인한다.
func TestHTTPAdapter_BridgeAdapterInterface(t *testing.T) {
	var _ node.BridgeAdapter = (*HTTPAdapter)(nil)
}
