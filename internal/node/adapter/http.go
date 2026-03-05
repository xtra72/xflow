package adapter

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// supportedContentTypes 는 HTTP 어댑터가 처리할 수 있는 Content-Type 목록이다.
var supportedContentTypes = map[string]bool{
	"application/json":                  true,
	"application/x-www-form-urlencoded": true,
	"multipart/form-data":               true,
	"text/plain":                        true,
	"text/html":                         true,
	"application/xml":                   true,
	"application/octet-stream":          true,
}

// HTTPAdapter 는 HTTP 프로토콜 전용 브릿지 어댑터이다.
// Content-Type 기반 페이로드 파싱, 상태 코드 매핑, 헤더 전파, URL 경로 템플릿을 지원한다.
type HTTPAdapter struct {
	defaultContentType string        // 기본 Content-Type (기본값: application/json)
	urlTemplate        string        // URL 경로 템플릿 (선택)
	timeout            time.Duration // 타임아웃
}

// HTTPAdapterOption 은 HTTPAdapter 생성 옵션이다.
type HTTPAdapterOption func(*HTTPAdapter)

// WithDefaultContentType 은 기본 Content-Type을 설정한다.
func WithDefaultContentType(ct string) HTTPAdapterOption {
	return func(a *HTTPAdapter) {
		a.defaultContentType = ct
	}
}

// WithURLTemplate 은 URL 경로 템플릿을 설정한다.
// {field_name} 형식의 플레이스홀더를 메시지 페이로드 값으로 치환한다.
func WithURLTemplate(tmpl string) HTTPAdapterOption {
	return func(a *HTTPAdapter) {
		a.urlTemplate = tmpl
	}
}

// WithTimeout 은 요청 타임아웃을 설정한다.
func WithTimeout(d time.Duration) HTTPAdapterOption {
	return func(a *HTTPAdapter) {
		a.timeout = d
	}
}

// NewHTTPAdapter 는 새로운 HTTPAdapter 인스턴스를 반환한다.
func NewHTTPAdapter(opts ...HTTPAdapterOption) *HTTPAdapter {
	a := &HTTPAdapter{
		defaultContentType: "application/json",
		timeout:            30 * time.Second,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Validate 는 주어진 BridgeConfig가 HTTP 어댑터에서 유효한지 검증한다.
// Content-Type, URL 템플릿 형식, 타임아웃 값을 검증한다.
func (a *HTTPAdapter) Validate(_ node.BridgeConfig) error {
	// Content-Type 검증 (설정된 경우)
	if a.defaultContentType != "" {
		if !supportedContentTypes[a.defaultContentType] {
			return fmt.Errorf("http adapter: unsupported content type %q", a.defaultContentType)
		}
	}

	// URL 템플릿 검증
	if a.urlTemplate != "" {
		if err := validateURLTemplate(a.urlTemplate); err != nil {
			return fmt.Errorf("http adapter: invalid URL template: %w", err)
		}
	}

	// 타임아웃 검증
	if a.timeout <= 0 {
		return fmt.Errorf("http adapter: timeout must be positive, got %v", a.timeout)
	}

	return nil
}

// DefaultConfig 는 HTTP 어댑터의 기본 BridgeConfig를 반환한다.
func (a *HTTPAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{
		Direction:      flow.BridgeRequestReply,
		BufferSize:     64,
		RequestTimeout: a.timeout,
	}
}

// TransformToFlow 는 HTTP 응답 데이터를 Content-Type에 따라 플로우 Message로 변환한다.
// application/json: JSON 파싱 후 페이로드에 설정
// application/x-www-form-urlencoded: 폼 데이터를 키-값으로 파싱
// 기타: 원본 데이터를 _raw 키에 저장
// 4xx/5xx 상태 코드는 http.error 메타데이터로 플래그된다.
func (a *HTTPAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	contentType := meta.ContentType
	if contentType == "" {
		contentType = a.defaultContentType
	}

	// Content-Type 에 따른 페이로드 파싱
	if data != nil {
		switch {
		case strings.HasPrefix(contentType, "application/json"):
			if err := trySetJSONPayload(msg, data); err != nil {
				msg.Payload().Set("_raw", string(data))
			}
		case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
			parseFormData(msg, string(data))
		default:
			msg.Payload().Set("_raw", string(data))
		}
	}

	// HTTP 메타데이터 설정
	if meta.StatusCode != 0 {
		msg.Metadata().Set("http.status_code", strconv.Itoa(meta.StatusCode))
	}
	if meta.ContentType != "" {
		msg.Metadata().Set("http.content_type", meta.ContentType)
	}
	if meta.URLPath != "" {
		msg.Metadata().Set("http.url_path", meta.URLPath)
	}
	for key, value := range meta.Headers {
		msg.Metadata().Set(fmt.Sprintf("http.header.%s", strings.ToLower(key)), value)
	}

	// 4xx/5xx 응답을 에러로 플래그
	if meta.StatusCode >= 400 {
		msg.Metadata().Set("http.error", "true")
	}

	return msg, nil
}

// TransformToAgent 는 플로우 Message를 HTTP 요청 바이트 데이터로 변환한다.
// 페이로드를 JSON으로 직렬화하고, URL 템플릿 보간 및 헤더 추출을 수행한다.
func (a *HTTPAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("http adapter: payload to JSON: %w", err)
	}

	meta := node.AgentMeta{
		AgentType:   "http",
		ContentType: a.defaultContentType,
	}

	// URL 템플릿 보간
	if a.urlTemplate != "" {
		meta.URLPath = interpolateTemplate(a.urlTemplate, msg.Payload())
	}

	// 메시지 메타데이터에서 HTTP 헤더 추출
	allMd := msg.Metadata().All()
	headers := make(map[string]string)
	for key, value := range allMd {
		if strings.HasPrefix(key, "http.header.") {
			headerKey := strings.TrimPrefix(key, "http.header.")
			headers[headerKey] = value
		}
	}
	if len(headers) > 0 {
		meta.Headers = headers
	}

	return data, meta, nil
}

// HandleControl 은 제어 메시지를 처리한다.
// 현재 HTTP 어댑터에서는 제어 메시지 처리가 없으므로 nil을 반환한다.
func (a *HTTPAdapter) HandleControl(_ message.Message) error {
	return nil
}

// parseFormData 는 URL 인코딩된 폼 데이터를 파싱하여 메시지 페이로드에 설정한다.
// 파싱에 실패하면 원본 데이터를 _raw 키에 저장한다.
func parseFormData(msg message.Message, data string) {
	values, err := url.ParseQuery(data)
	if err != nil {
		msg.Payload().Set("_raw", data)
		return
	}
	for key, vals := range values {
		if len(vals) == 1 {
			msg.Payload().Set(key, vals[0])
		} else {
			msg.Payload().Set(key, vals)
		}
	}
}

// validateURLTemplate 은 URL 템플릿 문자열의 중괄호 균형을 검증한다.
// 열린 중괄호와 닫힌 중괄호가 짝이 맞지 않으면 에러를 반환한다.
func validateURLTemplate(tmpl string) error {
	depth := 0
	for _, ch := range tmpl {
		if ch == '{' {
			depth++
		}
		if ch == '}' {
			depth--
		}
		if depth < 0 {
			return fmt.Errorf("unmatched closing brace")
		}
	}
	if depth != 0 {
		return fmt.Errorf("unmatched opening brace")
	}
	return nil
}
