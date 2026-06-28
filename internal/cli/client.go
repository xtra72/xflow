package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// defaultTimeout is the default HTTP request timeout.
const defaultTimeout = 30 * time.Second

// Client communicates with the xflowd API server.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	verbose    bool
}

// ClientOption 은 Client 생성 시 동작을 조정하는 함수형 옵션이다.
// NewClient 의 가변 인자로 전달되어 기본 구성 이후에 적용된다.
type ClientOption func(*Client)

// WithInsecure 는 insecure 가 true 일 때 TLS 인증서 검증을 건너뛰도록 설정한다.
// http.DefaultTransport 를 복제하여 InsecureSkipVerify 를 활성화한 tls.Config 를 적용한다.
// insecure 가 false 이면 기본 트랜스포트를 그대로 유지한다.
//
// 이 옵션은 명시적 opt-in 이며 자체 서명 인증서나 사설망 전용으로,
// 서버의 기존 insecure_skip_verify 패턴과 동일한 취지를 가진다.
func WithInsecure(insecure bool) ClientOption {
	return func(c *Client) {
		if !insecure {
			return
		}
		// http.DefaultTransport 를 복제하여 커넥션 풀 등 기본 설정을 보존한다.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 옵트인(자체 서명/사설망 전용); 서버의 insecure_skip_verify 패턴과 동일.
		c.httpClient.Transport = transport
	}
}

// NewClient creates a new API client.
// If timeout is 0, the default timeout of 30 seconds is used.
// 가변 인자 opts 로 ClientOption 을 전달하여 동작을 조정할 수 있다.
func NewClient(baseURL, token string, timeout time.Duration, verbose bool, opts ...ClientOption) *Client {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	c := &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		verbose: verbose,
	}

	// 각 옵션을 기본 구성 이후에 순서대로 적용한다.
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Get performs a GET request and decodes the response data into result.
func (c *Client) Get(path string, result any) error {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// PaginationMeta 는 목록 응답의 페이지네이션 메타데이터이다.
// 서버 응답 엔벨로프의 meta.pagination 필드와 매핑된다.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// GetWithMeta 는 GET 요청을 수행하고 응답 데이터와 함께 페이지네이션 메타를 반환한다.
// 목록 엔드포인트(예: /api/v1/flows, /api/v1/agents)의 전체 개수(total)가 필요할 때 사용한다.
// 메타가 없으면 meta 는 nil 이다.
func (c *Client) GetWithMeta(path string, result any) (*PaginationMeta, error) {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return c.handleResponseWithMeta(resp, result)
}

// Post performs a POST request with a JSON body and decodes the response data into result.
func (c *Client) Post(path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("요청 본문 직렬화 실패: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	resp, err := c.doRequest(http.MethodPost, path, bodyReader)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// Put performs a PUT request with a JSON body and decodes the response data into result.
func (c *Client) Put(path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("요청 본문 직렬화 실패: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	resp, err := c.doRequest(http.MethodPut, path, bodyReader)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// Delete performs a DELETE request and decodes the response data into result.
func (c *Client) Delete(path string, result any) error {
	resp, err := c.doRequest(http.MethodDelete, path, nil)
	if err != nil {
		return c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// GetRaw performs a GET request and returns the raw response bytes.
func (c *Client) GetRaw(path string) ([]byte, error) {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// PostRaw performs a POST request with a JSON body and returns raw response bytes.
func (c *Client) PostRaw(path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("요청 본문 직렬화 실패: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	resp, err := c.doRequest(http.MethodPost, path, bodyReader)
	if err != nil {
		return nil, c.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// Ping checks server connectivity by calling GET /health.
// Returns nil on success, ErrServerUnreachable on failure.
func (c *Client) Ping() error {
	resp, err := c.doRequest(http.MethodGet, "/health", nil)
	if err != nil {
		return ErrServerUnreachable(c.baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ErrServerUnreachable(c.baseURL)
	}
	return nil
}

// doRequest performs the actual HTTP request with headers.
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}

	// Set standard headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Inject auth token if present
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// Verbose logging to stderr
	if c.verbose {
		fmt.Fprintf(os.Stderr, "[HTTP] %s %s\n", method, url)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if c.verbose {
		fmt.Fprintf(os.Stderr, "[HTTP] %d %s\n", resp.StatusCode, resp.Status)
	}

	return resp, nil
}

// apiResponse is the internal structure for parsing API response envelopes.
type apiResponse struct {
	Success bool             `json:"success"`
	Data    json.RawMessage  `json:"data,omitempty"`
	Error   *apiErrorDetail  `json:"error,omitempty"`
	Meta    *apiResponseMeta `json:"meta,omitempty"`
}

// apiResponseMeta 는 응답 엔벨로프의 meta 필드를 파싱하기 위한 내부 구조이다.
type apiResponseMeta struct {
	Pagination *PaginationMeta `json:"pagination,omitempty"`
}

// handleResponseWithMeta 는 응답 엔벨로프를 파싱하여 데이터를 result 에 디코딩하고,
// 페이지네이션 메타를 함께 반환한다. handleResponse 와 동일한 에러 매핑을 따른다.
func (c *Client) handleResponseWithMeta(resp *http.Response, result any) (*PaginationMeta, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, MapAPIError(resp.StatusCode, bodyBytes)
	}

	if len(bodyBytes) == 0 {
		return nil, nil
	}

	var apiResp apiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	if !apiResp.Success {
		return nil, MapAPIError(resp.StatusCode, bodyBytes)
	}

	if result != nil && apiResp.Data != nil {
		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return nil, fmt.Errorf("데이터 디코딩 실패: %w", err)
		}
	}

	if apiResp.Meta != nil {
		return apiResp.Meta.Pagination, nil
	}
	return nil, nil
}

// handleResponse parses the API response envelope and maps errors.
func (c *Client) handleResponse(resp *http.Response, result any) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("응답 읽기 실패: %w", err)
	}

	// For error status codes, delegate to MapAPIError
	if resp.StatusCode >= 400 {
		return MapAPIError(resp.StatusCode, bodyBytes)
	}

	// 204 No Content 등 빈 응답은 파싱 없이 성공 반환
	if len(bodyBytes) == 0 {
		return nil
	}

	// Parse the envelope
	var apiResp apiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("응답 파싱 실패: %w", err)
	}

	// Check success field
	if !apiResp.Success {
		return MapAPIError(resp.StatusCode, bodyBytes)
	}

	// Decode data into result if provided
	if result != nil && apiResp.Data != nil {
		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return fmt.Errorf("데이터 디코딩 실패: %w", err)
		}
	}

	return nil
}

// wrapConnectionError wraps a raw network error into a CLIError.
// TLS/인증서 검증 실패가 감지되면 --insecure 사용을 안내하는 전용 에러를 반환하고,
// 그 외에는 기존 서버 연결 실패 메시지를 유지한다.
func (c *Client) wrapConnectionError(err error) error {
	// TLS/인증서 관련 에러인지 에러 문자열로 판별한다.
	if isTLSError(err) {
		return &CLIError{
			Message:  fmt.Sprintf("TLS 인증서를 검증할 수 없습니다: %s", c.baseURL),
			Hint:     "자체 서명 인증서라면 --insecure (-k) 옵션으로 검증을 건너뛸 수 있습니다",
			Cause:    err,
			ExitCode: 1,
		}
	}

	return &CLIError{
		Message:  fmt.Sprintf("서버에 연결할 수 없습니다: %s", c.baseURL),
		Hint:     "xflow config server <url> 명령어로 서버 주소를 확인하세요",
		Cause:    err,
		ExitCode: 1,
	}
}

// isTLSError 는 에러 문자열을 검사하여 TLS/인증서 검증 관련 실패인지 판별한다.
func isTLSError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "x509:") ||
		strings.Contains(msg, "certificate signed by unknown authority") ||
		strings.Contains(msg, "tls:")
}
