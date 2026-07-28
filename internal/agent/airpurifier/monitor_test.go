package airpurifier

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainEvents 는 msgCh 를 deadline 까지 비우며 디코딩된 이벤트 목록을 반환한다.
func drainEvents(t *testing.T, ap *AirPurifierAgent, deadline time.Duration) []map[string]any {
	t.Helper()
	var out []map[string]any
	timeout := time.After(deadline)
	for {
		select {
		case b := <-ap.msgCh:
			var m map[string]any
			require.NoError(t, json.Unmarshal(b, &m))
			out = append(out, m)
		case <-timeout:
			return out
		}
	}
}

// firstEventOfType 는 이벤트 목록에서 지정 type 의 첫 이벤트를 반환한다 (없으면 nil).
func firstEventOfType(events []map[string]any, eventType string) map[string]any {
	for _, e := range events {
		if e["type"] == eventType {
			return e
		}
	}
	return nil
}

// newDirectAgentWithMock 은 mock 클라이언트가 주입된 direct 모드 에이전트를 만든다.
func newDirectAgentWithMock(t *testing.T, opts map[string]any) (*AirPurifierAgent, *mockMQTTClient) {
	t.Helper()
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)
	mock := newMockMQTTClient()
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}
	return ap, mock
}

// Scenario 5.1: state 유입 → 로스터 갱신 + device_state_changed(changed_fields + epoch ms timestamp).
func TestMonitor_StateIngressEmitsChanged(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "line2"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))

	// 로스터 갱신 확인.
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Power)
	assert.Equal(t, 2, dev.FanSpeed)
	assert.False(t, dev.LastSeen.IsZero())

	events := drainEvents(t, ap, 100*time.Millisecond)
	evt := firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt, "device_state_changed 가 방출되어야 한다")

	assert.Equal(t, "ap-101", evt["device_id"])
	assert.Equal(t, "line2", evt["group_id"])
	assert.Equal(t, true, evt["online"])
	assert.Equal(t, true, evt["power"])
	assert.Equal(t, float64(2), evt["fan_speed"])

	// changed_fields 에 power/fan_speed/online 포함 (최초 관측 + 오프라인→온라인).
	changed := toStringSet(evt["changed_fields"])
	assert.Contains(t, changed, "power")
	assert.Contains(t, changed, "fan_speed")
	assert.Contains(t, changed, "online")

	// timestamp 가 epoch milliseconds(int64) 크기여야 한다.
	ts, ok := evt["timestamp"].(float64)
	require.True(t, ok, "timestamp 는 숫자여야 한다")
	assert.Greater(t, ts, float64(1_000_000_000_000), "timestamp 는 epoch ms 크기여야 한다")
}

// Scenario 5.2: 관측 기반 emit — 미관측 fan_speed 생략, power=false 시 fan_speed 생략.
func TestMonitor_ObservedGatedEmit(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 재시작 직후 power 만 관측 (fan_speed 미관측).
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 100*time.Millisecond)
	evt := firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt)
	_, hasFan := evt["fan_speed"]
	assert.False(t, hasFan, "미관측 fan_speed 는 방출 payload 에 없어야 한다")
	assert.Equal(t, true, evt["power"])

	// power=false 로 전환 → 신뢰 불가한 fan_speed 는 생략 (관측됐더라도).
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":false,"fan_speed":3}`))
	events = drainEvents(t, ap, 100*time.Millisecond)
	evt = firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt)
	assert.Equal(t, false, evt["power"])
	_, hasFan = evt["fan_speed"]
	assert.False(t, hasFan, "power=off 이면 fan_speed 를 생략해야 한다")
}

// Scenario 5.3: 오프라인 감지 (타임아웃) → Online=false + device_offline.
func TestMonitor_OfflineTimeout(t *testing.T) {
	opts := directOpts()
	opts["offline_timeout"] = "40ms"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "g1"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 최초 state 로 온라인 전이 + LastSeen 설정.
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	// 온라인 전이 이벤트를 비운다.
	_ = drainEvents(t, ap, 30*time.Millisecond)

	// offline_timeout(40ms) 동안 무수신 → 모니터가 오프라인 전환.
	events := drainEvents(t, ap, 200*time.Millisecond)
	off := firstEventOfType(events, "device_offline")
	require.NotNil(t, off, "device_offline 이 방출되어야 한다")
	assert.Equal(t, "ap-101", off["device_id"])
	assert.Equal(t, false, off["online"])

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.False(t, dev.Online, "타임아웃 후 Online=false 여야 한다")
}

// Scenario 5.4: LWT(direct) → handleLWTOffline 직접 호출 시 오프라인 + device_offline.
func TestMonitor_LWTOffline(t *testing.T) {
	opts := directOpts()
	opts["lwt_enabled"] = true
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 온라인 상태로 만든다.
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	_ = drainEvents(t, ap, 30*time.Millisecond)

	// LWT 통지 (구조적 훅 직접 호출).
	ap.handleLWTOffline("ap-101")

	events := drainEvents(t, ap, 100*time.Millisecond)
	off := firstEventOfType(events, "device_offline")
	require.NotNil(t, off, "LWT 후 device_offline 이 방출되어야 한다")
	assert.Equal(t, "ap-101", off["device_id"])

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.False(t, dev.Online)

	// 이미 오프라인이면 중복 방출하지 않는다.
	ap.handleLWTOffline("ap-101")
	events = drainEvents(t, ap, 50*time.Millisecond)
	assert.Nil(t, firstEventOfType(events, "device_offline"), "이미 오프라인이면 중복 device_offline 금지")
}

// Scenario 5.5: 온라인 복구 — 오프라인 디바이스가 state 보고 → Online=true + device_online.
//
// 오프라인 전이는 타임아웃 모니터(ticker) 대신 결정론적 경로(handleLWTOffline)로 만든다.
// 타임아웃 모니터를 쓰면 복구 직후 재-오프라인 tick 과 경쟁하므로, 오프라인 소스와 무관한
// 복구 로직(REQ-05-05)만 결정론적으로 검증한다. offline_timeout=0 으로 모니터를 비활성화한다.
func TestMonitor_OnlineRecovery(t *testing.T) {
	opts := directOpts()
	opts["offline_timeout"] = "0s" // 모니터 비활성 (재-오프라인 race 제거).
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 온라인으로 만든 뒤 오프라인 전환(LWT 경로, 결정론적).
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	_ = drainEvents(t, ap, 30*time.Millisecond)
	ap.handleLWTOffline("ap-101")
	events := drainEvents(t, ap, 50*time.Millisecond)
	require.NotNil(t, firstEventOfType(events, "device_offline"), "먼저 오프라인이 되어야 한다")

	// 다시 state 보고 → 복구.
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	events = drainEvents(t, ap, 100*time.Millisecond)
	on := firstEventOfType(events, "device_online")
	require.NotNil(t, on, "복구 시 device_online 이 방출되어야 한다")
	assert.Equal(t, "ap-101", on["device_id"])
	assert.Equal(t, true, on["online"])

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Online, "복구 후 Online=true 여야 한다")
}

// Scenario 5.6: request_state → 캐시 JSON 반환, 브로커 통신(publish) 0회.
func TestMonitor_RequestStateNoBroker(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "group_id": "g1"},
		map[string]any{"device_id": "ap-102"},
	}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 상태 캐시.
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	_ = drainEvents(t, ap, 50*time.Millisecond)

	// 단일 디바이스 request_state.
	resp, err := ap.Process([]byte(`{"command":"request_state","device_id":"ap-101"}`))
	require.NoError(t, err)
	var single map[string]any
	require.NoError(t, json.Unmarshal(resp, &single))
	assert.Equal(t, "ap-101", single["device_id"])
	assert.Equal(t, "g1", single["group_id"])
	assert.Equal(t, true, single["power"])
	assert.Equal(t, float64(2), single["fan_speed"])
	assert.Equal(t, true, single["online"])

	// 전체 request_state.
	respAll, err := ap.Process([]byte(`{"command":"request_state"}`))
	require.NoError(t, err)
	var all map[string]any
	require.NoError(t, json.Unmarshal(respAll, &all))
	assert.Equal(t, "ok", all["status"])
	devs, ok := all["devices"].([]any)
	require.True(t, ok)
	assert.Len(t, devs, 2)

	// 미등록 디바이스 → ErrDeviceNotFound.
	_, err = ap.Process([]byte(`{"command":"request_state","device_id":"missing"}`))
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	// request_state 는 브로커 발행을 하지 않아야 한다.
	assert.Equal(t, 0, mock.publishCount(), "request_state 는 MQTT publish 를 하지 않아야 한다")
}

// remove_device(direct) 는 디바이스 state 토픽을 Unsubscribe 해야 한다.
func TestMonitor_RemoveDeviceUnsubscribes(t *testing.T) {
	opts := directOpts()
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 런타임 등록(add_device) → 구독.
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-201"}`))
	require.NoError(t, err)
	assert.Contains(t, mock.subscribedTopics(), "airpurifier/ap-201/state")

	// remove_device → 해당 state 토픽 Unsubscribe.
	_, err = ap.Process([]byte(`{"command":"remove_device","device_id":"ap-201"}`))
	require.NoError(t, err)
	assert.Contains(t, mock.unsubscribedTopics(), "airpurifier/ap-201/state",
		"remove_device 는 state 토픽을 Unsubscribe 해야 한다")
}

// 모니터 고루틴은 Stop() 에서 깨끗이 종료되어야 한다 (누수 없음).
//
// Stop 은 monitorWG.Wait 로 모니터 종료를 확인하므로, 고루틴이 누수되면(stopCh 미관측)
// Stop 이 영원히 블록한다. 별도 고루틴에서 Stop 을 호출하고 deadline 내 반환을 확인하여
// 결정론적으로 누수 부재를 검증한다 (전역 NumGoroutine 은 타 테스트 누수로 불안정).
func TestMonitor_StopsCleanlyNoLeak(t *testing.T) {
	opts := directOpts()
	opts["offline_timeout"] = "20ms"
	ap, _ := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))

	// 모니터가 몇 tick 돌게 둔다.
	time.Sleep(50 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- ap.Stop(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 가 반환하지 않음 — 모니터 고루틴 누수(monitorWG.Wait 블록)")
	}
}

// offline_timeout==0 이면 오프라인 감지가 비활성이어야 한다 (모니터 미기동, 침묵해도 online 유지).
func TestMonitor_DisabledWhenTimeoutZero(t *testing.T) {
	opts := directOpts()
	opts["offline_timeout"] = "0s"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 온라인으로 만든 뒤 오래 침묵시켜도, 감지 비활성이므로 오프라인 전환이 없어야 한다.
	mock.deliver("airpurifier/ap-101/state", []byte(`{"power":true}`))
	_ = drainEvents(t, ap, 30*time.Millisecond)

	time.Sleep(60 * time.Millisecond) // 어떤 짧은 임계값도 초과할 시간.
	events := drainEvents(t, ap, 30*time.Millisecond)
	assert.Nil(t, firstEventOfType(events, "device_offline"),
		"offline_timeout==0 이면 device_offline 을 방출하지 않아야 한다")

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Online, "감지 비활성 시 online 을 유지해야 한다")
}

// toStringSet 은 JSON 디코딩된 배열([]any)을 문자열 집합으로 변환한다.
func toStringSet(v any) map[string]bool {
	out := map[string]bool{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out[s] = true
		}
	}
	return out
}
