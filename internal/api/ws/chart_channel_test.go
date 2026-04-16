package ws

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xtra/xflow/internal/agent/system"
)

// --- 테스트 헬퍼 ---

// chartTestServer 는 단일 ChartChannelHandler 를 httptest.Server 로 제공한다.
// Go 1.22 ServeMux 의 GET /ws/chart/{channel} 경로 패턴을 사용한다.
type chartTestServer struct {
	server  *httptest.Server
	handler *ChartChannelHandler
	reg     *system.ChartChannelRegistry
}

// newChartTestServer 는 전용 레지스트리와 핸들러를 구성하여 httptest.Server 를 시작한다.
// 호출자는 반드시 defer ts.Close() 로 정리해야 한다.
func newChartTestServer(t *testing.T) *chartTestServer {
	t.Helper()

	reg := system.NewChartChannelRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewChartChannelHandler(reg, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/chart/{channel}", handler.HandleUpgrade)

	server := httptest.NewServer(mux)

	t.Cleanup(func() {
		server.Close()
		reg.Close()
	})

	return &chartTestServer{
		server:  server,
		handler: handler,
		reg:     reg,
	}
}

// wsURL 은 http:// URL 을 ws:// URL 로 변환한다.
func (ts *chartTestServer) wsURL(path string) string {
	u, _ := url.Parse(ts.server.URL)
	u.Scheme = "ws"
	u.Path = path
	return u.String()
}

// httpURL 은 http://.../ws/chart/{channel} 형태의 평문 HTTP URL 을 반환한다 (업그레이드 시도 전 검증 경로).
func (ts *chartTestServer) httpURL(path string) string {
	return ts.server.URL + path
}

// readJSONFrame 은 WS 로부터 한 프레임(텍스트 메시지)을 읽어 generic map 으로 디코딩한다.
func readJSONFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame failed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode frame failed: %v (raw=%s)", err, string(data))
	}
	return out
}

// expectConnClose 는 WS 연결이 주어진 시간 내에 끊어지는지 확인한다.
func expectConnClose(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected connection close, got message")
	}
}

// --- Test 1: 채널 이름 검증 (REQ-M2-07) ---

func TestChartChannelHandler_InvalidChannelName_Returns400(t *testing.T) {
	ts := newChartTestServer(t)

	tests := []struct {
		name     string
		urlPath  string
		wantBody string // 응답 JSON 의 error 필드에 포함될 문자열
	}{
		{
			name:     "leading digit",
			urlPath:  "/ws/chart/1abc",
			wantBody: "invalid_channel_name",
		},
		{
			name:     "dot character",
			urlPath:  "/ws/chart/abc.def",
			wantBody: "invalid_channel_name",
		},
		{
			name:     "too long (65 chars)",
			urlPath:  "/ws/chart/" + "a" + strings.Repeat("b", 64),
			wantBody: "invalid_channel_name",
		},
		{
			name:     "encoded slash",
			urlPath:  "/ws/chart/abc%2Fdef",
			wantBody: "invalid_channel_name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(ts.httpURL(tc.urlPath))
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("Content-Type = %q, want JSON", resp.Header.Get("Content-Type"))
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if errStr, _ := body["error"].(string); errStr != tc.wantBody {
				t.Fatalf("error = %q, want %q", errStr, tc.wantBody)
			}
			if _, hasReason := body["reason"]; !hasReason {
				t.Fatalf("missing reason field: %+v", body)
			}
		})
	}
}

// --- Test 2: 채널 미존재 (REQ-M2-05) ---

func TestChartChannelHandler_ChannelNotFound_SendsErrorThenCloses(t *testing.T) {
	ts := newChartTestServer(t)

	conn, resp, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/nosuchchannel"), nil)
	if err != nil {
		t.Fatalf("dial failed: %v (resp=%v)", err, resp)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d, want 101", resp.StatusCode)
	}

	frame := readJSONFrame(t, conn, 2*time.Second)
	if frame["type"] != "chart.error" {
		t.Fatalf("frame type = %v, want chart.error (frame=%+v)", frame["type"], frame)
	}
	if frame["channel"] != "nosuchchannel" {
		t.Fatalf("frame channel = %v, want nosuchchannel", frame["channel"])
	}
	if frame["reason"] != "channel_not_found" {
		t.Fatalf("frame reason = %v, want channel_not_found", frame["reason"])
	}

	expectConnClose(t, conn, 2*time.Second)
}

// --- Test 3: Backfill on connect (REQ-M2-02) ---

func TestChartChannelHandler_Backfill_DeliversPastEntries(t *testing.T) {
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("bf1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 사전 3개 publish (timestamp 오름차순, 보관 기간 내로 맞추기 위해 현재 시각 기반).
	now := time.Now().UnixMilli()
	entries := []system.ChartEntry{
		{Timestamp: now - 3_000, Value: 10.0},
		{Timestamp: now - 2_000, Value: 20.0},
		{Timestamp: now - 1_000, Value: 30.0},
	}
	for _, e := range entries {
		if err := ch.Publish(e); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/bf1"), nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	frame := readJSONFrame(t, conn, 2*time.Second)
	if frame["type"] != "chart.backfill" {
		t.Fatalf("frame type = %v, want chart.backfill", frame["type"])
	}
	if frame["channel"] != "bf1" {
		t.Fatalf("frame channel = %v, want bf1", frame["channel"])
	}
	bfCount, _ := frame["backfilled_count"].(float64)
	if int(bfCount) != 3 {
		t.Fatalf("backfilled_count = %v, want 3", bfCount)
	}
	arr, ok := frame["entries"].([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("entries = %v, want 3 items", frame["entries"])
	}
	// 오름차순 검증
	var prev float64 = -1
	for i, ent := range arr {
		m, _ := ent.(map[string]any)
		ts, _ := m["timestamp"].(float64)
		if ts <= prev {
			t.Fatalf("entries[%d].timestamp=%v not ascending (prev=%v)", i, ts, prev)
		}
		prev = ts
	}
}

// --- Test 4: Fan-out (REQ-M2-04) ---

func TestChartChannelHandler_FanOut_AllSubscribersReceive(t *testing.T) {
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("fan1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 3명 구독
	const numSubs = 3
	conns := make([]*websocket.Conn, numSubs)
	for i := 0; i < numSubs; i++ {
		c, _, dErr := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/fan1"), nil)
		if dErr != nil {
			t.Fatalf("dial[%d] failed: %v", i, dErr)
		}
		conns[i] = c
		// 각 connection 의 초기 backfill (빈 entries) 소모
		frame := readJSONFrame(t, c, 2*time.Second)
		if frame["type"] != "chart.backfill" {
			t.Fatalf("sub[%d] first frame = %v, want chart.backfill", i, frame["type"])
		}
	}

	// 구독자 수가 반영될 때까지 대기
	waitForSubscriberCount(t, ch, numSubs, time.Second)

	// Publish 1 개 (현재 시각 기반 타임스탬프).
	if err := ch.Publish(system.ChartEntry{Timestamp: time.Now().UnixMilli(), Value: 42.0}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// 3명 모두 수신 확인
	for i, c := range conns {
		frame := readJSONFrame(t, c, 2*time.Second)
		if frame["type"] != "chart.append" {
			t.Fatalf("sub[%d] frame type = %v, want chart.append", i, frame["type"])
		}
	}

	// sub0 종료 → SubscriberCount 는 최종 2 가 되어야 함
	conns[0].Close()
	waitForSubscriberCount(t, ch, numSubs-1, 2*time.Second)

	// 다시 publish → 나머지 2명 수신
	if err := ch.Publish(system.ChartEntry{Timestamp: time.Now().UnixMilli(), Value: 43.0}); err != nil {
		t.Fatalf("publish2 failed: %v", err)
	}
	for i := 1; i < numSubs; i++ {
		frame := readJSONFrame(t, conns[i], 2*time.Second)
		if frame["type"] != "chart.append" {
			t.Fatalf("sub[%d] 2nd frame type = %v, want chart.append", i, frame["type"])
		}
	}

	if got := ch.Info().SubscriberCount; got != numSubs-1 {
		t.Fatalf("SubscriberCount = %d, want %d", got, numSubs-1)
	}

	for i := 1; i < numSubs; i++ {
		conns[i].Close()
	}
}

// waitForSubscriberCount 는 ch.Info().SubscriberCount 가 want 가 될 때까지 폴링 대기한다.
func waitForSubscriberCount(t *testing.T, ch *system.ChartChannel, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := ch.Info().SubscriberCount; got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("SubscriberCount did not reach %d within %v (current=%d)",
		want, timeout, ch.Info().SubscriberCount)
}

// --- Test 5: chart.closed on Unregister (REQ-M1-08) ---

func TestChartChannelHandler_Closed_OnUnregister(t *testing.T) {
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("close1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/close1"), nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// 초기 backfill 소모
	_ = readJSONFrame(t, conn, 2*time.Second)

	waitForSubscriberCount(t, ch, 1, time.Second)

	// Unregister → chart.closed 전송 후 close
	if err := ts.reg.Unregister("close1"); err != nil {
		t.Fatalf("unregister failed: %v", err)
	}

	frame := readJSONFrame(t, conn, 2*time.Second)
	if frame["type"] != "chart.closed" {
		t.Fatalf("frame type = %v, want chart.closed", frame["type"])
	}
	if frame["reason"] != "flow_undeployed" {
		t.Fatalf("frame reason = %v, want flow_undeployed", frame["reason"])
	}
	if frame["channel"] != "close1" {
		t.Fatalf("frame channel = %v, want close1", frame["channel"])
	}

	expectConnClose(t, conn, 2*time.Second)
}

// --- Test 6: 클라이언트 종료 시 구독자 자동 제거 (REQ-M2-06) ---

func TestChartChannelHandler_SubscriberRemovedOnClientDisconnect(t *testing.T) {
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("dc1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/dc1"), nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	// 초기 backfill 소모
	_ = readJSONFrame(t, conn, 2*time.Second)

	waitForSubscriberCount(t, ch, 1, time.Second)

	// 클라이언트 종료
	conn.Close()

	// 서버가 read loop 에서 감지하여 Unsubscribe 까지 완료되도록 대기
	waitForSubscriberCount(t, ch, 0, 2*time.Second)
}

// --- Test 7: Race test — 10 subs × 100 publishes ---

func TestChartChannelHandler_Concurrent_10Subs_100Publishes(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("race1", "flow-1", "node-1", 1000, 3600)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	const numSubs = 10
	const numPublishes = 100

	conns := make([]*websocket.Conn, numSubs)
	for i := 0; i < numSubs; i++ {
		c, _, dErr := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/race1"), nil)
		if dErr != nil {
			t.Fatalf("dial[%d] failed: %v", i, dErr)
		}
		conns[i] = c
		// 초기 backfill 소모
		_ = readJSONFrame(t, c, 2*time.Second)
	}

	waitForSubscriberCount(t, ch, numSubs, 2*time.Second)

	// 각 구독자에서 append 프레임 수집 고루틴
	received := make([]atomic.Int64, numSubs)
	var wg sync.WaitGroup
	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		go func(idx int, c *websocket.Conn) {
			defer wg.Done()
			for {
				_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
				_, data, rErr := c.ReadMessage()
				if rErr != nil {
					return
				}
				var frame map[string]any
				if json.Unmarshal(data, &frame) == nil && frame["type"] == "chart.append" {
					received[idx].Add(1)
					if received[idx].Load() >= numPublishes {
						return
					}
				}
			}
		}(i, conns[i])
	}

	// 동시 publish (현재 시각 기반, 서로 유니크하도록 seq 를 오프셋으로 더함).
	baseTS := time.Now().UnixMilli()
	var pubWG sync.WaitGroup
	pubWG.Add(numPublishes)
	for i := 0; i < numPublishes; i++ {
		go func(seq int) {
			defer pubWG.Done()
			_ = ch.Publish(system.ChartEntry{
				Timestamp: baseTS + int64(seq),
				Value:     float64(seq),
			})
		}(i)
	}
	pubWG.Wait()

	// 모든 구독자가 수신 완료될 때까지 대기 (reader goroutine exit 시점)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reader goroutines did not exit within 5s")
	}

	for i := 0; i < numSubs; i++ {
		_ = conns[i].Close()
		if got := received[i].Load(); got < numPublishes {
			t.Errorf("sub[%d] received=%d, want >=%d", i, got, numPublishes)
		}
	}
}

// --- Test 8: nil 레지스트리 → 500 (initialization 실패 방어) ---

func TestChartChannelHandler_NilRegistry_Returns500(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewChartChannelHandler(nil, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/chart/{channel}", handler.HandleUpgrade)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/ws/chart/abc")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "registry_unavailable" {
		t.Fatalf("error = %v, want registry_unavailable", body["error"])
	}
}

// --- Test 9: subscriberID / formatInt64 경계 ---

func TestSubscriberID_FormatEdgeCases(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "chartws-0"},
		{1, "chartws-1"},
		{12345, "chartws-12345"},
		{9223372036854775807, "chartws-9223372036854775807"}, // int64 max
	}
	for _, tc := range tests {
		if got := subscriberID(tc.in); got != tc.want {
			t.Errorf("subscriberID(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- Test 10: Close 멱등성 + 닫힌 이후 Send 실패 ---

func TestChartWSSubscriber_CloseIdempotent_SendAfterCloseFails(t *testing.T) {
	// 진짜 WS conn 없이 단위 테스트: conn nil 으로 생성할 수는 없으므로
	// writePump 를 시작하지 않고 필드만 사용한다.
	sub := &chartWSSubscriber{
		id:     "chartws-test",
		conn:   nil, // start() 호출 안 하므로 writePump 실행 없음
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		sendCh: make(chan []byte, 4),
		done:   make(chan struct{}),
	}

	// Send 는 정상 동작해야 한다 (버퍼 용량 내).
	if err := sub.Send([]byte(`{"t":"a"}`)); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	// Close 를 2번 호출해도 panic 없이 nil 반환.
	_ = sub.Close()
	_ = sub.Close()

	// 닫힌 이후 Send 는 에러 반환.
	if err := sub.Send([]byte(`{"t":"b"}`)); err == nil {
		t.Fatalf("send after close: expected error")
	}
}

// --- Test 11: Send 버퍼 full → slow_consumer 에러 + Close 자동 호출 ---

func TestChartWSSubscriber_SlowConsumer_ClosesOnFullBuffer(t *testing.T) {
	sub := &chartWSSubscriber{
		id:     "chartws-slow",
		conn:   nil,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		sendCh: make(chan []byte, 2), // 의도적 작은 버퍼
		done:   make(chan struct{}),
	}

	// 버퍼 용량만큼 enqueue (drain 없음 — writePump 시작 안 함).
	if err := sub.Send([]byte("a")); err != nil {
		t.Fatalf("send 1 failed: %v", err)
	}
	if err := sub.Send([]byte("b")); err != nil {
		t.Fatalf("send 2 failed: %v", err)
	}

	// 세 번째는 버퍼 full → slow_consumer 에러 + Close 호출.
	err := sub.Send([]byte("c"))
	if err == nil {
		t.Fatalf("send 3 expected slow_consumer error")
	}
	if !strings.Contains(err.Error(), "slow_consumer") &&
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("send 3 error = %v, want slow_consumer or closed", err)
	}

	// done 이 닫혀 있어야 한다 (Close 가 호출됨).
	select {
	case <-sub.done:
	default:
		t.Fatalf("expected done channel to be closed after slow_consumer")
	}
}

// --- Test 12a: writePump close 전 drain 경로 ---

func TestChartWSSubscriber_WritePump_DrainsBufferOnClose(t *testing.T) {
	// httptest 서버로 진짜 WS 연결을 만들어 writePump 경로를 실행한다.
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("drain1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/drain1"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// backfill (빈) 소모
	_ = readJSONFrame(t, conn, 2*time.Second)
	waitForSubscriberCount(t, ch, 1, time.Second)

	// 여러 entry 를 빠르게 publish — writePump 가 drain 한다.
	baseTS := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		if err := ch.Publish(system.ChartEntry{
			Timestamp: baseTS + int64(i),
			Value:     float64(i),
		}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// 5개 프레임 수신 확인.
	for i := 0; i < 5; i++ {
		frame := readJSONFrame(t, conn, 2*time.Second)
		if frame["type"] != "chart.append" {
			t.Fatalf("frame[%d] type=%v, want chart.append", i, frame["type"])
		}
	}

	// registry.Unregister 로 채널을 닫아 writePump 의 done 경로를 실행.
	if err := ts.reg.Unregister("drain1"); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	// closed 프레임 수신 + 종료
	frame := readJSONFrame(t, conn, 2*time.Second)
	if frame["type"] != "chart.closed" {
		t.Fatalf("final frame type=%v, want chart.closed", frame["type"])
	}
	expectConnClose(t, conn, 2*time.Second)
}

// --- Test 12b: 기본 logger (nil) 분기 커버 ---

func TestChartChannelHandler_NilLogger_UsesDefault(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	defer reg.Close()

	h := NewChartChannelHandler(reg, nil) // logger=nil 경로
	if h == nil || h.logger == nil {
		t.Fatalf("expected non-nil handler+logger, got %+v", h)
	}
}

func TestChartWSSubscriber_NilLogger_UsesDefault(t *testing.T) {
	s := newChartWSSubscriber(nil, nil) // conn+logger=nil 경로
	if s == nil || s.logger == nil {
		t.Fatalf("expected non-nil subscriber+logger, got %+v", s)
	}
	if s.id == "" {
		t.Fatalf("expected non-empty id")
	}
}

// --- Test 12c: ping 주기 경로 커버 ---

func TestChartChannelHandler_PingTicker_SendsPing(t *testing.T) {
	// ping 주기를 100ms 로 단축하여 writePump 의 ticker 브랜치를 실행시킨다.
	prevPing := chartWSPingPeriod
	chartWSPingPeriod = 100 * time.Millisecond
	defer func() { chartWSPingPeriod = prevPing }()

	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("ping1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 클라이언트 측 pong 수신 카운터
	var pingReceived atomic.Int64
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(ts.wsURL("/ws/chart/ping1"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetPingHandler(func(appData string) error {
		pingReceived.Add(1)
		return conn.WriteControl(websocket.PongMessage,
			[]byte(appData), time.Now().Add(time.Second))
	})

	// 초기 backfill 소모 (ReadMessage 가 ping 도 동시에 처리함)
	_ = readJSONFrame(t, conn, 2*time.Second)
	waitForSubscriberCount(t, ch, 1, time.Second)

	// ReadMessage 한 번을 별도 고루틴에서 길게 block 시킨다. gorilla 는 read 중에
	// control frame (ping) 을 처리하면서 SetPingHandler 콜백을 호출한다.
	// 이 고루틴은 connection close 시에만 종료된다.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, _ = conn.ReadMessage() // close 또는 timeout 시 종료
	}()

	// ping 이 수신될 때까지 폴링.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && pingReceived.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if pingReceived.Load() == 0 {
		t.Fatalf("no ping received in 1s (ping period=%v)", chartWSPingPeriod)
	}

	// 정리: 연결을 닫아 reader goroutine 종료.
	_ = conn.Close()
	<-done
}

// --- Test 12d: writeText 실패 시 writePump 종료 ---
//
// 연결을 클라이언트 쪽에서 종료한 상태에서 server 가 Send 하면
// writePump 의 writeText 가 에러를 반환하고 고루틴이 정상 종료되어야 한다.
func TestChartChannelHandler_WriteTextError_ExitsWritePump(t *testing.T) {
	ts := newChartTestServer(t)

	ch, err := ts.reg.Register("wterr1", "flow-1", "node-1", 100, 3600)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL("/ws/chart/wterr1"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// 초기 backfill 소모
	_ = readJSONFrame(t, conn, 2*time.Second)
	waitForSubscriberCount(t, ch, 1, time.Second)

	// 클라이언트 측 close → 이후 서버 Publish 는 writeText 에러 경로에 도달.
	_ = conn.Close()

	// 서버 측 read loop 가 disconnect 를 감지할 시간.
	time.Sleep(50 * time.Millisecond)

	// 채널이 여전히 존재하는 동안 publish — publish 자체는 성공 (fan-out 실패 허용).
	// 이 시점에 구독자는 이미 제거되었거나 곧 제거되므로 에러는 무시.
	_ = ch.Publish(system.ChartEntry{
		Timestamp: time.Now().UnixMilli(),
		Value:     99.0,
	})

	// 구독자 수가 0 이 될 때까지 대기 (read loop 에서 Unsubscribe + Close 수행).
	waitForSubscriberCount(t, ch, 0, 2*time.Second)
}

// --- Test 12: isExpectedWSCloseErr 분기 ---

func TestIsExpectedWSCloseErr_Branches(t *testing.T) {
	if !isExpectedWSCloseErr(nil) {
		t.Fatalf("nil should be expected close")
	}
	if isExpectedWSCloseErr(errors.New("other io error")) {
		t.Fatalf("generic error should not be expected close")
	}
	// websocket.ErrCloseSent 는 패키지에서 export 되어 있다.
	if !isExpectedWSCloseErr(websocket.ErrCloseSent) {
		t.Fatalf("ErrCloseSent should be expected close")
	}
}

