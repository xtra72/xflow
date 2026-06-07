// remote_stream_test.go 는 M8(그룹 J) 브라우저-측 스트리밍 엔드포인트(SSE)의 게이팅·
// 프레임 전달·teardown 을 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J08/J08b).
//
// 검증:
//   - 게이팅: 미관리 → 503, 노출 범위 밖 → 404, admin 아님 → 403.
//   - happy path: SSE 헤더 + data: 프레임 전달.
//   - teardown: 노드 오프라인(Done) 시 SSE 루프 종료.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
)

// fakeStreamSvc 는 RemoteStreamService 의 테스트 구현이다.
type fakeStreamSvc struct {
	managed bool
	exposed map[string]bool // kind/id

	frames chan json.RawMessage
	done   chan struct{}
	unsub  func()

	subscribed []string // "domain/action/id"
	subErr     error
}

func newFakeStreamSvc() *fakeStreamSvc {
	return &fakeStreamSvc{
		exposed: make(map[string]bool),
		frames:  make(chan json.RawMessage, 8),
		done:    make(chan struct{}),
		unsub:   func() {},
	}
}

func (f *fakeStreamSvc) IsManaged(string) bool { return f.managed }

func (f *fakeStreamSvc) IsResourceExposed(_ context.Context, _ string, kind, id string) (bool, error) {
	return f.exposed[kind+"/"+id], nil
}

func (f *fakeStreamSvc) SubscribeStream(_ string, domain, streamAction string, args json.RawMessage) (remote.StreamHandle, error) {
	id := ""
	var m map[string]any
	if json.Unmarshal(args, &m) == nil {
		if v, ok := m["id"].(string); ok {
			id = v
		}
	}
	f.subscribed = append(f.subscribed, domain+"/"+streamAction+"/"+id)
	if f.subErr != nil {
		return remote.StreamHandle{}, f.subErr
	}
	return remote.StreamHandle{Frames: f.frames, Done: f.done, Unsubscribe: f.unsub}, nil
}

// serveStream 은 stream 핸들러를 직접 호출한다(raw http.HandlerFunc — 인증 우회 모드).
func serveStream(t *testing.T, svc RemoteStreamService, target string, role string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteStreamHandler(svc, nil) // jwtSvc nil → 테스트 인증 우회.
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if role != "" {
		ctx := context.WithValue(req.Context(), testRoleKey{}, role)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.HandleStream(rec, req)
	return rec
}

// TestStream_HappyPath 는 SSE 헤더 + 프레임 전달 + 노드 Done 시 종료를 검증한다(REQ-J08).
func TestStream_HappyPath(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	svc.exposed["device/d1"] = true

	var rec *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec = serveStream(t, svc, "/api/v1/remote/nodes/n1/devices/d1/state/stream", "admin")
	}()

	// 프레임 1개 주입 후 노드 Done 으로 종료.
	svc.frames <- json.RawMessage(`{"on":true}`)
	time.Sleep(50 * time.Millisecond)
	close(svc.done)
	wg.Wait()

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `data: {"on":true}`)
	assert.Equal(t, []string{"device/state/d1"}, svc.subscribed)
}

// TestStream_NotManaged503 은 미관리 노드 구독이 503 인지 검증한다(REQ-J05).
func TestStream_NotManaged503(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = false
	svc.exposed["device/d1"] = true
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/devices/d1/state/stream", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestStream_NotExposed404 은 노출 범위 밖 구독이 404 인지 검증한다(REQ-J05).
func TestStream_NotExposed404(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/agents/a1/stats/stream", "admin")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, svc.subscribed)
}

// TestStream_RequiresAdmin 은 node-role 이 거부(403)되는지 검증한다(REQ-F04).
func TestStream_RequiresAdmin(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	svc.exposed["device/d1"] = true
	rec := serveStream(t, svc, "/api/v1/remote/nodes/n1/devices/d1/state/stream", "node")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestStream_AgentSeriesRoute 는 agent/series 스트림 라우트 매핑을 검증한다(REQ-J08).
func TestStream_AgentSeriesRoute(t *testing.T) {
	svc := newFakeStreamSvc()
	svc.managed = true
	svc.exposed["agent/a1"] = true

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = serveStream(t, svc, "/api/v1/remote/nodes/n1/agents/a1/series/stream", "admin")
	}()
	time.Sleep(30 * time.Millisecond)
	close(svc.done)
	wg.Wait()

	require.Len(t, svc.subscribed, 1)
	assert.True(t, strings.HasPrefix(svc.subscribed[0], "agent/series/"))
}
