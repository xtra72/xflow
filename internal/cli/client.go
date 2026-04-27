package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

// NewClient creates a new API client.
// If timeout is 0, the default timeout of 30 seconds is used.
func NewClient(baseURL, token string, timeout time.Duration, verbose bool) *Client {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		verbose: verbose,
	}
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
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *apiErrorDetail `json:"error,omitempty"`
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
func (c *Client) wrapConnectionError(err error) error {
	return &CLIError{
		Message:  fmt.Sprintf("서버에 연결할 수 없습니다: %s", c.baseURL),
		Hint:     "xflow config server <url> 명령어로 서버 주소를 확인하세요",
		Cause:    err,
		ExitCode: 1,
	}
}
