// @SPEC:SPEC-UPDATE-001 v0.1.0
// healthcheck.go — Phase D 재시작 후 readiness probe.
//
// SPEC M6 / M7:
//   - 새 바이너리 시작 후 5초(설정 가능) 안에 health check 응답 확인
//   - 실패 시 자동 롤백 트리거 (롤백 자체는 rollback.go 가 수행)
//
// 디자인 결정:
//   - 단순 폴링 (HTTP GET) 으로 시작; 성공 시 응답 본문에서 version 추출 (옵션)
//   - 200 OK 면 healthy, 200 외 / 네트워크 에러는 retry
//   - context 취소 우선 처리 (호출자가 graceful shutdown 등에서 중단 가능)
//   - 응답 본문은 64KiB 제한 (DoS 방어)
//
// 보안:
//   - 외부 입력 (응답 본문) JSON 파싱 실패는 fatal 아님 (binary 가 시작했음 자체가 신호)
//   - http.Client timeout 은 호출자가 설정 (기본 5초)
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HealthChecker 는 새 바이너리의 readiness 를 폴링한다.
//
// Endpoint 는 일반적으로 새 바이너리의 /api/v1/system/version 또는 전용 health endpoint.
// 호출자는 SPEC M11 의 health_check_timeout (기본 5초) 을 Timeout 에 매핑한다.
type HealthChecker struct {
	// Endpoint 는 health probe 대상 URL (HTTP/HTTPS).
	Endpoint string

	// Timeout 은 전체 폴링 윈도우 (이 시간 안에 healthy 가 안 되면 unhealthy 판정).
	// 0이면 30초 기본.
	Timeout time.Duration

	// Interval 은 폴링 간격. 0이면 1초 기본.
	Interval time.Duration

	// Client 는 단일 HTTP 호출에 사용. nil 이면 짧은 timeout 으로 자동 생성.
	Client *http.Client
}

// HealthResult 는 WaitHealthy 의 종합 결과이다.
type HealthResult struct {
	// Healthy 는 폴링 윈도우 안에 200 OK 응답을 적어도 한 번 받았는지 여부.
	Healthy bool

	// Version 은 응답 JSON 에서 추출한 version 필드 (없으면 빈 문자열).
	Version Version

	// Latency 는 WaitHealthy 호출 시점부터 결과 반환까지의 elapsed 시간.
	Latency time.Duration

	// Attempts 는 시도된 폴링 횟수 (성공 시 마지막 성공 시도 포함).
	Attempts int

	// LastError 는 마지막 실패 시도의 에러 (성공 시 nil).
	LastError error
}

// healthResponse 는 응답 JSON 의 부분 모델 (호환을 위해 unknown 필드는 무시).
type healthResponse struct {
	Version string `json:"version"`
	Status  string `json:"status,omitempty"`
}

// WaitHealthy 는 Endpoint 가 응답할 때까지 폴링하고 결과를 반환한다.
//
// 종료 조건:
//  1. 200 OK 응답 → Healthy=true 반환
//  2. ctx 취소 → Healthy=false, LastError=ctx.Err()
//  3. Timeout 경과 → Healthy=false, LastError=마지막 probe 에러
//
// 보안: 응답 본문은 64KiB 제한 (메모리 폭증 방어).
func (h *HealthChecker) WaitHealthy(ctx context.Context) HealthResult {
	// 기본값 적용 (zero 값 안전성)
	if h.Timeout == 0 {
		h.Timeout = 30 * time.Second
	}
	if h.Interval == 0 {
		h.Interval = 1 * time.Second
	}
	if h.Client == nil {
		h.Client = &http.Client{Timeout: 5 * time.Second}
	}

	start := time.Now()
	deadline := start.Add(h.Timeout)
	result := HealthResult{}

	for {
		// ctx 우선 검사 (호출자 취소 즉시 반영)
		select {
		case <-ctx.Done():
			result.Latency = time.Since(start)
			result.LastError = ctx.Err()
			return result
		default:
		}

		result.Attempts++
		version, err := h.probe(ctx)
		if err == nil {
			result.Healthy = true
			result.Version = version
			result.Latency = time.Since(start)
			return result
		}
		result.LastError = err

		// timeout 검사
		if time.Now().After(deadline) {
			result.Latency = time.Since(start)
			return result
		}

		// 다음 polling 까지 대기 (ctx 취소 감지 가능)
		select {
		case <-ctx.Done():
			result.Latency = time.Since(start)
			result.LastError = ctx.Err()
			return result
		case <-time.After(h.Interval):
		}
	}
}

// probe 는 단일 HTTP GET 을 수행하고 (status, body parse) 결과로 (Version, error) 를 반환한다.
//
// 200 OK 만 success 로 간주. 그 외 status / 네트워크 에러 / dial 실패는 모두 error.
// 200 OK 본문이 JSON 이 아니어도 healthy 로 처리 (binary 가 응답한 자체가 의미 있음).
func (h *HealthChecker) probe(ctx context.Context) (Version, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// 응답 본문은 무시 (status 만으로 판정)
		return "", fmt.Errorf("unexpected status: %d %s", resp.StatusCode, resp.Status)
	}

	// 본문 읽기 (DoS 방어: 64KiB 제한)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		// 본문 읽기 실패 — status 는 200 이지만 신뢰할 수 없음
		return "", fmt.Errorf("read body: %w", err)
	}

	// JSON 파싱 시도. 실패해도 healthy (binary 가 응답함이 핵심).
	var hr healthResponse
	if jerr := json.Unmarshal(body, &hr); jerr != nil {
		return "", nil // healthy, 단 version 미정
	}
	return Version(hr.Version), nil
}
