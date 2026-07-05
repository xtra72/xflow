// remote_chart_log_stream_test.go 는 M10(그룹 L) 노드 측 스트림 브리지의 차트
// (chart.chart, in-process hub 탭) + 로그(monitor.logs) 매핑을 검증한다
// (@SPEC:SPEC-REMOTE-001 M10, REQ-L06/L07/J08/J08b).
//
// 차트: 노드의 in-process ChartChannelRegistry 를 직접 구독해 backfill/append 를 중계한다
// (별도 WS 경로 미신설 — /ws/chart/{channel} 자가 dial 금지). 로그: 노드의 로그 스트림
// hub 를 구독해 라이브 tail 을 push 한다. 둘 다 캐시 우회(라이브 — REQ-J16).
package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/remote"
)

// newChartLogStreamSource 는 chart/log 소스만 바인딩한 스트림 소스를 만든다.
func newChartLogStreamSource(reg *system.ChartChannelRegistry, logs *logStreamHub) *remoteStreamSource {
	s := newRemoteStreamSource(nil, nil, nil, 10*time.Millisecond)
	s.charts = reg
	s.logs = logs
	return s
}

// TestStreamBridge_ChartBackfillThenAppend 는 chart.chart 구독이 backfill 프레임을
// 즉시 흘리고, 이후 Publish 한 append 프레임을 in-process 로 중계하는지 검증한다
// (REQ-L07 — 노드 in-process 차트 hub 탭, 자가 WS dial 없음).
func TestStreamBridge_ChartBackfillThenAppend(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	defer reg.Close()
	ch, err := reg.Register("temp", "flow1", "node1", 100, 0)
	require.NoError(t, err)
	// backfill 용 기존 엔트리.
	require.NoError(t, ch.Publish(system.ChartEntry{Timestamp: 1, Value: 21.0}))

	src := newChartLogStreamSource(reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := src.Subscribe(ctx, remote.DomainChart, remote.StreamActionChart, json.RawMessage(`{"channelName":"temp"}`))
	require.NoError(t, err)
	defer sub.Close()

	// 1) backfill 프레임(chart.backfill) 수신.
	select {
	case v := <-sub.Updates():
		assert.Contains(t, string(v), "chart.backfill")
		assert.Contains(t, string(v), `"channel":"temp"`)
	case <-time.After(time.Second):
		t.Fatal("chart backfill 프레임 타임아웃")
	}

	// 2) 이후 Publish → append 프레임(chart.append) 중계.
	require.NoError(t, ch.Publish(system.ChartEntry{Timestamp: 2, Value: 22.0}))
	select {
	case v := <-sub.Updates():
		assert.Contains(t, string(v), "chart.append")
	case <-time.After(time.Second):
		t.Fatal("chart append 프레임 타임아웃")
	}
}

// TestStreamBridge_ChartTeardownUnsubscribes 는 Close 시 노드 차트 채널에서
// 구독자가 제거되는지 검증한다(REQ-J08b — teardown, 누수 없음).
func TestStreamBridge_ChartTeardownUnsubscribes(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	defer reg.Close()
	ch, err := reg.Register("cpu", "flow1", "node1", 100, 0)
	require.NoError(t, err)

	src := newChartLogStreamSource(reg, nil)
	sub, err := src.Subscribe(context.Background(), remote.DomainChart, remote.StreamActionChart, json.RawMessage(`{"channelName":"cpu"}`))
	require.NoError(t, err)

	// 구독 직후 backfill drain.
	<-sub.Updates()
	require.Eventually(t, func() bool { return ch.Info().SubscriberCount == 1 },
		time.Second, 5*time.Millisecond, "구독자 1명 등록 대기")

	// Close → teardown(노드 채널 Unsubscribe).
	require.NoError(t, sub.Close())
	require.Eventually(t, func() bool { return ch.Info().SubscriberCount == 0 },
		time.Second, 5*time.Millisecond, "Close 후 구독자 제거(teardown)")
}

// TestStreamBridge_ChartChannelNotFound 는 미존재 채널 구독이 명확한 오류를 반환하는지
// 검증한다(서버가 502 node-error 로 매핑 — REQ-L02). 패닉 금지.
func TestStreamBridge_ChartChannelNotFound(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	defer reg.Close()

	src := newChartLogStreamSource(reg, nil)
	_, err := src.Subscribe(context.Background(), remote.DomainChart, remote.StreamActionChart, json.RawMessage(`{"channelName":"absent"}`))
	require.Error(t, err)
}

// TestStreamBridge_ChartMissingReg 는 차트 레지스트리 미바인딩 시 미지원 오류를
// 반환하는지 검증한다(노드 보호).
func TestStreamBridge_ChartMissingReg(t *testing.T) {
	src := newRemoteStreamSource(nil, nil, nil, 10*time.Millisecond)
	_, err := src.Subscribe(context.Background(), remote.DomainChart, remote.StreamActionChart, json.RawMessage(`{"channelName":"x"}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, remote.ErrQueryActionUnsupported)
}

// TestStreamBridge_LogsLiveTail 는 monitor.logs 구독이 로그 hub 의 라이브 라인을
// stream_data 로 push 하는지 검증한다(REQ-L06 — 라이브 tail, 캐시 우회).
func TestStreamBridge_LogsLiveTail(t *testing.T) {
	hub := newLogStreamHub()
	src := newChartLogStreamSource(nil, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := src.Subscribe(ctx, remote.DomainMonitor, remote.StreamActionLogs, nil)
	require.NoError(t, err)
	defer sub.Close()

	// hub 에 로그 라인 주입(io.Writer 경유 — observe.StreamRouter 가 호출하는 형태).
	line := []byte(`{"level":"INFO","msg":"hello","time":"2026-06-08T00:00:00Z"}`)
	_, _ = hub.Write(line)

	select {
	case v := <-sub.Updates():
		assert.Contains(t, string(v), "hello")
		assert.Contains(t, string(v), `"level"`)
	case <-time.After(time.Second):
		t.Fatal("monitor.logs 라이브 tail 타임아웃")
	}
}

// TestStreamBridge_LogsTeardown 는 Close 시 로그 hub 에서 구독자가 제거되는지
// 검증한다(REQ-J08b — teardown, 누수 없음).
func TestStreamBridge_LogsTeardown(t *testing.T) {
	hub := newLogStreamHub()
	src := newChartLogStreamSource(nil, hub)

	sub, err := src.Subscribe(context.Background(), remote.DomainMonitor, remote.StreamActionLogs, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return hub.SubscriberCount() == 1 },
		time.Second, 5*time.Millisecond, "구독자 등록 대기")

	require.NoError(t, sub.Close())
	require.Eventually(t, func() bool { return hub.SubscriberCount() == 0 },
		time.Second, 5*time.Millisecond, "Close 후 구독자 제거(teardown)")
}

// TestStreamBridge_LogsMissingHub 는 로그 hub 미바인딩 시 미지원 오류를 반환하는지
// 검증한다(노드 보호).
func TestStreamBridge_LogsMissingHub(t *testing.T) {
	src := newRemoteStreamSource(nil, nil, nil, 10*time.Millisecond)
	_, err := src.Subscribe(context.Background(), remote.DomainMonitor, remote.StreamActionLogs, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, remote.ErrQueryActionUnsupported)
}
