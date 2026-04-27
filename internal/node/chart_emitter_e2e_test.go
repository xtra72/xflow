package node_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	apiws "github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// chart-emitter → WebSocket 구독자로의 end-to-end 데이터 흐름 검증.
// 사용자 시나리오 재현: store-read 의 배열 출력 (descending 최신순) 을
// chart-emitter 에 entries_field 로 공급하면 구독자가 모든 엔트리를
// timestamp 오름차순으로 수신하는지 확인.

// 배치 입력 → chart.backfill 수신 경로 검증.
// (새 구독자가 연결 후 링버퍼 전체를 한 번에 받음)
func TestChartEmitter_E2E_BatchToBackfill(t *testing.T) {
	// 공유 레지스트리 + WS 서버
	reg := system.NewChartChannelRegistry()
	defer reg.Close()
	system.SetDefaultChartChannelRegistry(reg)
	defer system.SetDefaultChartChannelRegistry(nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/chart/{channel}",
		apiws.NewChartChannelHandler(reg, slog.New(slog.NewTextHandler(io.Discard, nil))).HandleUpgrade)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// chart-emitter 노드 구성: entries_field="store_value"
	def := flow.NewNodeDef("em-e2e", "chart-emitter",
		flow.WithInputPorts(flow.Port{ID: "in", Name: "in", Direction: flow.PortInput}),
		flow.WithOutputPorts(),
	)
	n, err := node.NewChartEmitterNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "e2e_batch",
		"buffer_size":   600,
		"retention_sec": 0, // 시간 만료 비활성 (과거 타임스탬프 허용)
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))
	defer func() { _ = n.Shutdown(context.Background()) }()

	// 사용자 데이터 샘플 축소판 (descending 최신순) — store-read 출력 포맷 동일.
	storeValue := []any{}
	for ts := int64(1776339916504); ts >= 1776336319713; ts -= 1000 {
		storeValue = append(storeValue, map[string]any{
			"timestamp": ts,
			"value":     float64((ts % 3) + 21), // 21 / 22 / 23 순환
		})
	}
	require.Greater(t, len(storeValue), 500, "최소 500개 이상의 테스트 데이터 필요")

	// chart-emitter 에 배치 메시지 전달
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"store_value": storeValue,
	})))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 링버퍼 상태 확인 (buffer_size=600 이므로 최신 600개 남음)
	ch, ok := reg.Get("e2e_batch")
	require.True(t, ok, "채널이 등록되어 있어야 함")
	snap := ch.Snapshot()
	require.Len(t, snap, 600, "링버퍼에 최신 600개 엔트리가 있어야 함 (FIFO + 오름차순 정렬)")

	// 링버퍼 내부 순서: Publish 순서 = 오름차순 정렬 후 insert 였으므로
	// snap[0] 이 가장 오래된 timestamp, snap[599] 가 가장 최신 timestamp.
	require.Less(t, snap[0].Timestamp, snap[len(snap)-1].Timestamp,
		"링버퍼가 timestamp 오름차순이어야 함")

	// 이제 구독자가 연결 → backfill 수신
	wsURL, _ := url.Parse(ts.URL)
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws/chart/e2e_batch"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err, "WS upgrade 실패: %v (resp=%v)", err, resp)
	defer conn.Close()

	// 첫 프레임은 chart.backfill 이어야 함
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)

	var frame map[string]any
	require.NoError(t, json.Unmarshal(data, &frame))

	assert.Equal(t, "chart.backfill", frame["type"], "첫 프레임은 backfill")
	assert.Equal(t, "e2e_batch", frame["channel"])

	entries, ok := frame["entries"].([]any)
	require.True(t, ok, "entries 는 배열이어야 함")
	assert.Len(t, entries, 600, "backfill 은 링버퍼 전체 크기와 동일해야 함")

	// 오름차순 정렬 검증
	first := entries[0].(map[string]any)
	last := entries[len(entries)-1].(map[string]any)
	firstTs, _ := first["timestamp"].(float64) // JSON 역직렬화 결과는 float64
	lastTs, _ := last["timestamp"].(float64)
	require.NotZero(t, firstTs)
	require.NotZero(t, lastTs)
	assert.Less(t, firstTs, lastTs,
		"backfill entries 는 timestamp 오름차순 정렬되어야 함 (first=%.0f, last=%.0f)",
		firstTs, lastTs)

	// value 가 숫자로 수신되는지 (라인 차트 렌더링의 필수 조건)
	firstVal, ok := first["value"]
	require.True(t, ok, "첫 엔트리에 value 필드가 있어야 함")
	_, isFloat := firstVal.(float64)
	assert.True(t, isFloat, "value 는 float64 로 수신되어야 함 (라인 차트 렌더링용), got %T", firstVal)
}

// 구독 중인 상태에서 배치가 publish 되면 각 엔트리가 개별 chart.append 로 수신되는지 검증.
// 실시간 append 경로.
func TestChartEmitter_E2E_BatchToAppendDuringSubscription(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	defer reg.Close()
	system.SetDefaultChartChannelRegistry(reg)
	defer system.SetDefaultChartChannelRegistry(nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/chart/{channel}",
		apiws.NewChartChannelHandler(reg, slog.New(slog.NewTextHandler(io.Discard, nil))).HandleUpgrade)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	def := flow.NewNodeDef("em-live", "chart-emitter",
		flow.WithInputPorts(flow.Port{ID: "in", Name: "in", Direction: flow.PortInput}),
		flow.WithOutputPorts(),
	)
	n, err := node.NewChartEmitterNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{
		"channel_name":  "e2e_live",
		"buffer_size":   100,
		"retention_sec": 0,
		"entries_field": "store_value",
	}))
	require.NoError(t, n.Init(context.Background()))
	defer func() { _ = n.Shutdown(context.Background()) }()

	// 먼저 구독 (빈 backfill)
	wsURL, _ := url.Parse(ts.URL)
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws/chart/e2e_live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer conn.Close()

	// 빈 backfill 프레임 소비
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, first, err := conn.ReadMessage()
	require.NoError(t, err)
	var bf map[string]any
	require.NoError(t, json.Unmarshal(first, &bf))
	assert.Equal(t, "chart.backfill", bf["type"])

	// 이후 배치 publish → append 3개 수신 (오름차순)
	storeValue := []any{
		map[string]any{"timestamp": int64(300), "value": 23.0},
		map[string]any{"timestamp": int64(200), "value": 22.0},
		map[string]any{"timestamp": int64(100), "value": 21.0},
	}
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"store_value": storeValue,
	})))
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// 3개 append 프레임 수신 (오름차순)
	var appends []map[string]any
	for i := 0; i < 3; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, rerr := conn.ReadMessage()
		require.NoError(t, rerr, "append %d 수신 실패", i)
		var f map[string]any
		require.NoError(t, json.Unmarshal(data, &f))
		require.Equal(t, "chart.append", f["type"], "i=%d", i)
		entry, ok := f["entry"].(map[string]any)
		require.True(t, ok)
		appends = append(appends, entry)
	}

	// append 순서 = 오름차순 publish 순서
	ts0 := appends[0]["timestamp"].(float64)
	ts1 := appends[1]["timestamp"].(float64)
	ts2 := appends[2]["timestamp"].(float64)
	assert.Equal(t, 100.0, ts0)
	assert.Equal(t, 200.0, ts1)
	assert.Equal(t, 300.0, ts2)
}
