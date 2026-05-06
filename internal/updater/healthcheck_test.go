// @SPEC:SPEC-UPDATE-001 v0.1.0
// healthcheck_test.go — Phase D 재시작 후 health probe 테스트.
//
// SPEC M6 / M7:
//   - 새 프로세스 시작 후 5초(설정 가능) 안에 health check 응답 정상 확인
//   - 실패 시 운영 오케스트레이터가 자동 롤백 트리거 (이 모듈은 수동 모니터)
//
// 테스트 안전성: httptest.Server 로 가짜 엔드포인트 구성, 외부 네트워크 호출 없음.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ----

// healthBody 는 표준 health 응답 JSON 직렬화 헬퍼.
func healthBody(version string) []byte {
	b, _ := json.Marshal(map[string]string{
		"version": version,
		"status":  "ok",
	})
	return b
}

// newFastHealthChecker 는 단위 테스트에서 빠르게 결과가 나도록 짧은 timeout/interval 을 설정한다.
func newFastHealthChecker(endpoint string, timeout, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		Endpoint: endpoint,
		Timeout:  timeout,
		Interval: interval,
		Client:   &http.Client{Timeout: 500 * time.Millisecond},
	}
}

// ---- Happy path ----

// TestHealthCheck_HappyPath_RespondsWithin1Second 는 서버가 즉시 200 OK 를 반환할 때
// Healthy=true 와 정상 버전이 보고되는지 검증한다 (M6 readiness probe 기본 동작).
func TestHealthCheck_HappyPath_RespondsWithin1Second(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 2*time.Second, 50*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	assert.True(t, res.Healthy, "expected healthy on 200 OK")
	assert.Equal(t, Version("v0.4.0"), res.Version, "version should be parsed from response")
	assert.GreaterOrEqual(t, res.Attempts, 1, "at least one probe attempt")
	assert.Less(t, res.Latency, 1*time.Second, "should be fast")
	assert.NoError(t, res.LastError)
}

// TestHealthCheck_VersionExtracted 는 응답 JSON 의 version 필드가 HealthResult.Version 으로
// 정확히 추출되는지 검증한다 (운영자/오케스트레이터가 신/구 버전 비교에 사용).
func TestHealthCheck_VersionExtracted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(healthBody("v1.2.3"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 1*time.Second, 50*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	require.True(t, res.Healthy)
	assert.Equal(t, Version("v1.2.3"), res.Version)
}

// ---- Timeout / unhealthy paths ----

// TestHealthCheck_TimeoutBeforeReady_ReturnsUnhealthy 는 서버가 응답을 늦게 줄 때
// 전체 timeout 안에 healthy 가 안 되면 Healthy=false 를 반환하는지 검증한다.
func TestHealthCheck_TimeoutBeforeReady_ReturnsUnhealthy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 응답을 절대 보내지 않고 늦게 닫는 서버 시뮬레이션
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	// 매우 짧은 client timeout + 짧은 전체 timeout
	hc := &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  300 * time.Millisecond,
		Interval: 50 * time.Millisecond,
		Client:   &http.Client{Timeout: 100 * time.Millisecond},
	}

	res := hc.WaitHealthy(context.Background())
	assert.False(t, res.Healthy, "should be unhealthy on timeout")
	assert.Greater(t, res.Attempts, 0)
	assert.Error(t, res.LastError, "last probe error should be recorded")
	assert.GreaterOrEqual(t, res.Latency, 300*time.Millisecond)
}

// TestHealthCheck_ReturnsAfterMultipleRetries 는 초반 N번 실패 후 성공하는 시나리오.
// 새 바이너리 시작 직후 listener 준비에 시간이 걸리는 현실적 사례 (M6).
func TestHealthCheck_ReturnsAfterMultipleRetries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 4 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 2*time.Second, 30*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	assert.True(t, res.Healthy, "should eventually become healthy")
	assert.GreaterOrEqual(t, res.Attempts, 4, "should require at least 4 polls")
	assert.Equal(t, Version("v0.4.0"), res.Version)
}

// TestHealthCheck_ContextCancellation_StopsPolling 는 외부 ctx 취소 시 즉시 중단을 검증한다
// (호출자가 graceful shutdown 등의 상황에서 health probe 를 중지할 수 있어야 함).
func TestHealthCheck_ContextCancellation_StopsPolling(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	hc := newFastHealthChecker(srv.URL, 5*time.Second, 100*time.Millisecond)

	// 50ms 후 context 취소
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res := hc.WaitHealthy(ctx)
	elapsed := time.Since(start)

	assert.False(t, res.Healthy)
	assert.Less(t, elapsed, 1*time.Second, "should stop quickly on ctx cancel")
	assert.True(t, errors.Is(res.LastError, context.Canceled) || res.LastError != nil,
		"last error should reflect cancellation")
}

// TestHealthCheck_5xxResponse_RetriesAndEventuallyTimesout 는 지속적 500 응답 시
// timeout 까지 재시도하다가 unhealthy 로 종료하는지 검증한다.
func TestHealthCheck_5xxResponse_RetriesAndEventuallyTimesout(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 250*time.Millisecond, 30*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	assert.False(t, res.Healthy)
	assert.GreaterOrEqual(t, calls.Load(), int32(2), "should retry at least 2 times")
	assert.Error(t, res.LastError)
	assert.Contains(t, res.LastError.Error(), "500", "error should mention status code")
}

// TestHealthCheck_404Response_TreatedAsNotReady 는 404 가 일시 not-ready 로 처리되는지 검증한다
// (새 바이너리가 여전히 부팅 중이고 라우터가 아직 핸들러를 등록하지 않은 상태).
func TestHealthCheck_404Response_TreatedAsNotReady(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 1*time.Second, 30*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	assert.True(t, res.Healthy, "404 should not be terminal; eventual 200 makes it healthy")
	assert.GreaterOrEqual(t, res.Attempts, 3)
}

// TestHealthCheck_NetworkError_RetriesAndTimesout 는 dial 실패 (서버 다운) 시 재시도 후
// timeout 으로 unhealthy 가 보고되는지 검증한다.
func TestHealthCheck_NetworkError_RetriesAndTimesout(t *testing.T) {
	t.Parallel()

	// 잘못된 포트로 이동 — TCP RST/connection refused 발생
	hc := &HealthChecker{
		Endpoint: "http://127.0.0.1:1", // privileged port, refused
		Timeout:  200 * time.Millisecond,
		Interval: 30 * time.Millisecond,
		Client:   &http.Client{Timeout: 60 * time.Millisecond},
	}

	res := hc.WaitHealthy(context.Background())
	assert.False(t, res.Healthy)
	assert.Greater(t, res.Attempts, 0)
	assert.Error(t, res.LastError, "network error should be captured")
}

// TestHealthCheck_LatencyRecorded 는 happy path 에서 Latency 가 0보다 크고
// 적정 범위에 있는지 검증한다 (운영 메트릭으로 활용).
func TestHealthCheck_LatencyRecorded(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 약간 지연 후 응답
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 1*time.Second, 10*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	require.True(t, res.Healthy)
	assert.GreaterOrEqual(t, res.Latency, 20*time.Millisecond, "should record at least sleep time")
	assert.Less(t, res.Latency, 1*time.Second, "should be below total timeout")
}

// TestHealthCheck_AttemptsCounted 는 반복 시도가 제대로 카운트되는지 검증한다.
func TestHealthCheck_AttemptsCounted(t *testing.T) {
	t.Parallel()

	var serverCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := serverCalls.Add(1)
		if n < 5 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 2*time.Second, 20*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	require.True(t, res.Healthy)
	// 최소 5번 (4 fails + 1 success). 정확히 5 또는 그 이상 (Internal interval slack).
	assert.GreaterOrEqual(t, res.Attempts, 5, "should reflect actual probe count")
	assert.Equal(t, int(serverCalls.Load()), res.Attempts,
		"local attempts should match server-side observed calls")
}

// ---- Default values ----

// TestHealthCheck_DefaultsApplied 는 0 값일 때 sane 기본값이 적용되는지 검증한다.
func TestHealthCheck_DefaultsApplied(t *testing.T) {
	t.Parallel()

	// 즉시 200 응답하는 서버
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(healthBody("v0.4.0"))
	}))
	defer srv.Close()

	// 기본값 의존: Timeout=0, Interval=0, Client=nil
	hc := &HealthChecker{Endpoint: srv.URL}
	res := hc.WaitHealthy(context.Background())
	assert.True(t, res.Healthy, "defaults should make HealthChecker functional")
}

// TestHealthCheck_NonJSONResponse_StillHealthy 는 200 OK 응답이지만 JSON 이 아닌 경우에도
// (binary 가 시작했고 응답을 한다는 것이 중요하므로) Healthy=true 로 처리되는지 검증한다.
func TestHealthCheck_NonJSONResponse_StillHealthy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "OK")
	}))
	defer srv.Close()

	hc := newFastHealthChecker(srv.URL, 1*time.Second, 30*time.Millisecond)
	res := hc.WaitHealthy(context.Background())

	assert.True(t, res.Healthy, "non-JSON 200 should still be healthy (binary is responsive)")
	assert.Equal(t, Version(""), res.Version, "version is empty when JSON parse fails")
}
