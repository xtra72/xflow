// remote_chart_log_stream_test.go 는 M10(그룹 L) 차트/로그 브라우저 SSE 엔드포인트의
// 게이팅·매핑·teardown 을 검증한다(@SPEC:SPEC-REMOTE-001 M10, REQ-L06/L07/J08/J08b).
//
// 차트/로그는 노드-레벨 라이브 스트림(per-resource 노출 범위 없음 — REQ-L03)이므로
// 게이팅은 IsManaged(승인+온라인)만 평가한다. 차트는 channelName, 로그는 자원 식별자
// 없이 노드 단위로 구독한다. 변경 경로 없음(READ-ONLY — REQ-J03).
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChartStream_HappyPath 는 .../charts/{channel}/stream 가 chart/chart 로 매핑되고
// channelName 을 args 로 전달하는지 검증한다(REQ-L07). 노출 범위 게이트 미호출.
func TestChartStream_HappyPath(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true

	var rec *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec = serveStream(t, svc, "/api/v1/remote/nodes/n1/charts/temp/stream", "admin")
	}()

	svc.frames <- json.RawMessage(`{"type":"chart.append"}`)
	time.Sleep(50 * time.Millisecond)
	close(svc.done)
	wg.Wait()

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "chart.append")
	require.Len(t, svc.subscribed, 1)
	// chart 도메인 + channelName 전달 검증.
	assert.Equal(t, "chart/chart/", svc.subscribed[0]) // args.id 없음(channelName 사용).
	assert.Equal(t, "temp", svc.lastChannelName)
}

// TestChartStream_NotManaged503 은 미관리 노드 차트 구독이 503 인지 검증한다(REQ-L11).
func TestChartStream_NotManaged503(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = false
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/charts/temp/stream", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestChartStream_NotAdmin403 은 비-admin 차트 구독이 403 인지 검증한다(REQ-F04).
func TestChartStream_NotAdmin403(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/charts/temp/stream", "node")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestLogStream_HappyPath 는 .../logs/stream 가 monitor/logs 로 매핑되는지 검증한다
// (REQ-L06). 자원 식별자 없이 노드 단위 구독, 노출 범위 게이트 미호출.
func TestLogStream_HappyPath(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true

	var rec *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec = serveStream(t, svc, "/api/v1/remote/nodes/n1/logs/stream", "admin")
	}()

	svc.frames <- json.RawMessage(`{"level":"INFO","message":"hi"}`)
	time.Sleep(50 * time.Millisecond)
	close(svc.done)
	wg.Wait()

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"hi"`)
	require.Len(t, svc.subscribed, 1)
	assert.Equal(t, "monitor/logs/", svc.subscribed[0])
}

// TestLogStream_NotManaged503 은 미관리 노드 로그 구독이 503 인지 검증한다(REQ-L11).
func TestLogStream_NotManaged503(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = false
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/logs/stream", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestLogStream_NotAdmin403 은 비-admin 로그 구독이 403 인지 검증한다(REQ-F04).
func TestLogStream_NotAdmin403(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/logs/stream", "node")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestChartStream_NodeStreamsRegistered 는 chart/logs 라우트가 streamRoutes 표에
// 포함되어 RegisterRawHandlers 로 등록 가능한지 검증한다(별도 WS 경로 미신설 — REQ-L07).
func TestChartStream_NodeStreamsRegistered(t *testing.T) {
	var patterns []string
	h := NewRemoteStreamHandler(newFakeStreamSvc(), nil)
	h.RegisterRawHandlers(func(pattern string, _ http.HandlerFunc) {
		patterns = append(patterns, pattern)
	})
	joined := ""
	for _, p := range patterns {
		joined += p + "\n"
	}
	assert.Contains(t, joined, "/charts/{channel}/stream")
	assert.Contains(t, joined, "/logs/stream")
}
