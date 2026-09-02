package xsfm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// §1. 설정 파싱·검증 (Module 3, M1) — SPEC-XSFM-AGENT-IO-001
// ---------------------------------------------------------------------------

// AC-1.1: forward/emit 키 부재 시 기본값 (forward=false, mode=event, interval=60s).
func TestEmitConfig_Defaults(t *testing.T) {
	cfg, err := parseXSFMConfig(directOpts())
	require.NoError(t, err)
	assert.False(t, cfg.ForwardReceivedToNode)
	assert.Equal(t, stateEmitModeEvent, cfg.StateEmitMode)
	assert.Equal(t, 60*time.Second, cfg.StateEmitInterval)
}

// AC-1.2: state_emit_mode 유효 enum(event/interval/both) 수락.
func TestEmitConfig_ValidModesAccepted(t *testing.T) {
	for _, mode := range []string{"event", "interval", "both"} {
		opts := directOpts()
		opts["state_emit_mode"] = mode
		cfg, err := parseXSFMConfig(opts)
		require.NoError(t, err, "mode %q 는 수락되어야 한다", mode)
		assert.Equal(t, mode, cfg.StateEmitMode)
	}
}

// AC-1.3: state_emit_mode 위반 값 거부 (ErrInvalidStateEmitMode, 조용한 폴백 없음).
func TestEmitConfig_InvalidModeRejected(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "periodic"
	_, err := parseXSFMConfig(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStateEmitMode), "ErrInvalidStateEmitMode 여야 한다")

	// 비문자열 타입도 거부.
	opts["state_emit_mode"] = 42
	_, err = parseXSFMConfig(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStateEmitMode))
}

// AC-1.4: state_emit_interval 파싱 + 음수 거부.
func TestEmitConfig_IntervalParseAndNegativeReject(t *testing.T) {
	opts := directOpts()
	opts["state_emit_interval"] = "30s"
	cfg, err := parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.StateEmitInterval)

	opts["state_emit_interval"] = "-5s"
	_, err = parseXSFMConfig(opts)
	require.Error(t, err, "음수 interval 은 거부되어야 한다")
}

// AC-1.5: forward_received_to_node 파싱.
func TestEmitConfig_ForwardParse(t *testing.T) {
	opts := directOpts()
	opts["forward_received_to_node"] = true
	cfg, err := parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.True(t, cfg.ForwardReceivedToNode)
}

// AC-3.8: interval 기본값 60s 유지 + 미설정 mode=interval 도 기본 60s.
func TestEmitConfig_IntervalDefaultWithIntervalMode(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	cfg, err := parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, stateEmitModeInterval, cfg.StateEmitMode)
	assert.Equal(t, 60*time.Second, cfg.StateEmitInterval)
}

// ---------------------------------------------------------------------------
// §2. 수신 forward 옵션 (Module 1, M2)
// ---------------------------------------------------------------------------

// AC-2.1: forward on — 매 수신 방출(무변경 포함).
func TestForward_EmitsOnEveryInbound(t *testing.T) {
	opts := directOpts()
	opts["forward_received_to_node"] = true
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "line2"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 1차 유입(변경 있음).
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	require.NotNil(t, firstEventOfType(events, "device_state_received"), "1차 유입에서 device_state_received 방출")

	// 2차 유입(동일 값 = 무변경) — forward 는 여전히 방출.
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	events = drainEvents(t, ap, 80*time.Millisecond)
	rcv := firstEventOfType(events, "device_state_received")
	require.NotNil(t, rcv, "무변경 유입에서도 device_state_received 가 방출되어야 한다(매 수신 탭)")
	// 무변경이므로 device_state_changed 는 없어야 한다.
	assert.Nil(t, firstEventOfType(events, "device_state_changed"), "무변경 시 device_state_changed 는 없어야 한다")
}

// AC-2.2: forward off(기본) — device_state_received 미방출.
func TestForward_OffEmitsNone(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	assert.Nil(t, firstEventOfType(events, "device_state_received"), "forward off 이면 device_state_received 미방출")
	// on-change 는 정상 방출(회귀 없음).
	assert.NotNil(t, firstEventOfType(events, "device_state_changed"))
}

// AC-2.3: forward shape — 파싱/정규화 단일 디바이스, changed_fields 부재.
func TestForward_Shape(t *testing.T) {
	opts := directOpts()
	opts["forward_received_to_node"] = true
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "line2"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":3}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	rcv := firstEventOfType(events, "device_state_received")
	require.NotNil(t, rcv)

	assert.Equal(t, "device_state_received", rcv["type"])
	assert.Equal(t, "ap-101", rcv["device_id"])
	assert.Equal(t, "line2", rcv["group_id"])
	assert.Equal(t, true, rcv["online"])
	assert.Equal(t, true, rcv["power"])
	assert.Equal(t, float64(3), rcv["fan_speed"])
	// timestamp 는 epoch ms.
	ts, ok := rcv["timestamp"].(float64)
	require.True(t, ok)
	assert.Greater(t, ts, float64(1_000_000_000_000))
	// changed_fields 는 싣지 않는다(패스스루 탭).
	_, hasChanged := rcv["changed_fields"]
	assert.False(t, hasChanged, "device_state_received 는 changed_fields 를 싣지 않아야 한다")
	// 배열이 아닌 단일 디바이스 메시지.
	_, hasDevices := rcv["devices"]
	assert.False(t, hasDevices, "forward 는 단일 디바이스 메시지여야 한다(devices 배열 아님)")
}

// AC-2.4: forward 는 emit_mode 와 독립 — interval 억제여도 forward 는 방출.
func TestForward_IndependentOfEmitMode(t *testing.T) {
	opts := directOpts()
	opts["forward_received_to_node"] = true
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "1h" // tick 은 사실상 발생하지 않게 크게.
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	// interval 모드 → on-change 억제.
	assert.Nil(t, firstEventOfType(events, "device_state_changed"), "interval 모드에서 on-change 억제")
	// forward 는 여전히 방출.
	assert.NotNil(t, firstEventOfType(events, "device_state_received"), "forward 는 방출 모드와 독립적으로 방출")
}

// AC-2.5: forward 는 mode-agnostic — port 모드(FeedState)에서도 방출.
func TestForward_ModeAgnosticPort(t *testing.T) {
	opts := portOpts()
	opts["forward_received_to_node"] = true
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// port 모드: 노드가 FeedState 로 유입.
	ap.FeedState("ap-port-1", []byte(`{"power":true,"fan_speed":1}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	rcv := firstEventOfType(events, "device_state_received")
	require.NotNil(t, rcv, "port 모드에서도 device_state_received 가 방출되어야 한다(mode-agnostic)")
	assert.Equal(t, "ap-port-1", rcv["device_id"])
	// 원시 바이트 에코가 아니라 정규화 상태.
	assert.Equal(t, true, rcv["power"])
	assert.Equal(t, float64(1), rcv["fan_speed"])
}

// ---------------------------------------------------------------------------
// §3. 상태 방출 모드 (Module 2, M3)
// ---------------------------------------------------------------------------

// AC-3.1: event 모드 — on-change 방출, 주기 스냅샷 미방출.
func TestEmitMode_EventOnChangeNoSnapshot(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "event"
	opts["state_emit_interval"] = "10ms" // event 모드이므로 무시되어야 한다.
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 120*time.Millisecond)
	assert.NotNil(t, firstEventOfType(events, "device_state_changed"), "event 모드는 on-change 방출")
	assert.Nil(t, firstEventOfType(events, "device_state_snapshot"), "event 모드는 주기 스냅샷 미방출")
}

// AC-3.2: interval 모드 — 주기 스냅샷 방출 + on-change 억제.
func TestEmitMode_IntervalSnapshotSuppressesOnChange(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "20ms"
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101"},
		map[string]any{"device_id": "ap-102"},
	}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	events := drainEvents(t, ap, 120*time.Millisecond)
	// on-change 억제.
	assert.Nil(t, firstEventOfType(events, "device_state_changed"), "interval 모드에서 on-change 억제")
	// 주기 스냅샷 방출.
	snap := firstEventOfType(events, "device_state_snapshot")
	require.NotNil(t, snap, "interval 모드에서 device_state_snapshot 방출")
	devs, ok := snap["devices"].([]any)
	require.True(t, ok, "snapshot.devices 는 배열이어야 한다")
	assert.GreaterOrEqual(t, len(devs), 2, "전체 등록 디바이스 풀 스냅샷")
}

// AC-3.3: both 모드 — on-change + 주기 스냅샷 병행.
func TestEmitMode_BothConcurrent(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "both"
	opts["state_emit_interval"] = "20ms"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 120*time.Millisecond)
	assert.NotNil(t, firstEventOfType(events, "device_state_changed"), "both 모드는 on-change 방출")
	assert.NotNil(t, firstEventOfType(events, "device_state_snapshot"), "both 모드는 주기 스냅샷 방출")
}

// AC-3.4: 주기 스냅샷 — offline 포함(include-all).
func TestEmitMode_SnapshotIncludesOffline(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "20ms"
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-online"},
		map[string]any{"device_id": "ap-offline"}, // never-seen → online:false.
	}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-online/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 120*time.Millisecond)
	snap := firstEventOfType(events, "device_state_snapshot")
	require.NotNil(t, snap)
	devs := snap["devices"].([]any)

	byID := map[string]bool{}
	for _, d := range devs {
		m := d.(map[string]any)
		byID[m["device_id"].(string)] = m["online"].(bool)
	}
	require.Contains(t, byID, "ap-offline", "offline 디바이스도 스냅샷에 포함되어야 한다")
	assert.False(t, byID["ap-offline"], "offline 디바이스는 online:false 로 포함")
	assert.True(t, byID["ap-online"])
}

// AC-3.5: 주기 스냅샷 shape — 단일 {type,timestamp,devices:[...]} 배열 메시지.
func TestEmitMode_SnapshotShapeSingleArray(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "20ms"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 120*time.Millisecond)

	snapCount := 0
	changedCount := 0
	for _, e := range events {
		switch e["type"] {
		case "device_state_snapshot":
			snapCount++
			assert.Contains(t, e, "timestamp")
			_, ok := e["devices"].([]any)
			assert.True(t, ok, "devices 는 배열이어야 한다")
		case "device_state_changed":
			changedCount++
		}
	}
	assert.GreaterOrEqual(t, snapCount, 1, "단일 배열 device_state_snapshot 메시지")
	// per-device 개별 device_state_changed 로 방출되지 않는다(interval 억제).
	assert.Equal(t, 0, changedCount, "interval 모드에서 per-device device_state_changed 미방출")
}

// AC-3.6: 방출기 미기동 조건 — mode=event 또는 interval<=0.
func TestEmitMode_EmitterNotStarted(t *testing.T) {
	t.Run("event mode", func(t *testing.T) {
		opts := directOpts()
		opts["state_emit_mode"] = "event"
		opts["state_emit_interval"] = "10ms"
		opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
		ap, _ := newDirectAgentWithMock(t, opts)
		require.NoError(t, ap.Start(context.Background()))
		defer func() { _ = ap.Stop(context.Background()) }()
		events := drainEvents(t, ap, 80*time.Millisecond)
		assert.Nil(t, firstEventOfType(events, "device_state_snapshot"))
	})

	t.Run("interval mode but interval<=0", func(t *testing.T) {
		opts := directOpts()
		opts["state_emit_mode"] = "interval"
		opts["state_emit_interval"] = "0s"
		opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
		ap, _ := newDirectAgentWithMock(t, opts)
		require.NoError(t, ap.Start(context.Background()))
		defer func() { _ = ap.Stop(context.Background()) }()
		events := drainEvents(t, ap, 80*time.Millisecond)
		assert.Nil(t, firstEventOfType(events, "device_state_snapshot"), "interval<=0 이면 방출기 미기동")
	})
}

// AC-3.7: 전이 이벤트(device_online/offline)는 방출 모드와 무관하게 항상 방출.
func TestEmitMode_TransitionEventsNotGated(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "1h" // tick 회피, 전이만 관찰.
	opts["offline_timeout"] = "30ms"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "g1"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// online 전이 후 타임아웃 offline 전이까지 단일 창에서 관찰(전이 이벤트가 게이팅 창에 걸쳐
	// 발생할 수 있으므로 하나의 drain 으로 모두 수집한다).
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 200*time.Millisecond)
	assert.NotNil(t, firstEventOfType(events, "device_online"), "device_online 전이는 게이팅되지 않는다")
	assert.NotNil(t, firstEventOfType(events, "device_offline"), "device_offline 전이는 게이팅되지 않는다")
	// 전이만 관찰: interval tick 은 1h 로 크게 잡아 스냅샷이 이 창에 방출되지 않는다.
	assert.Nil(t, firstEventOfType(events, "device_state_changed"), "interval 모드에서 on-change 는 억제")
}

// ---------------------------------------------------------------------------
// §4. 생명주기·동시성 (Module 4, M4)
// ---------------------------------------------------------------------------

// AC-4.1: goroutine 누수 없음 — interval 방출기 기동 후 Stop 이 반환한다.
func TestEmitMode_NoGoroutineLeak(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "both"
	opts["state_emit_interval"] = "5ms"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	time.Sleep(30 * time.Millisecond) // 몇 tick 경과.

	done := make(chan error, 1)
	go func() { done <- ap.Stop(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 이 반환하지 않음 — 방출기 goroutine 누수(monitorWG.Wait 미반환)")
	}
}

// AC-4.2: 락 경합 없음 (-race) — 방출기 tick 중 상태 유입·조회 동시 발생.
func TestEmitMode_RaceConcurrentIngestAndSnapshot(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "both"
	opts["state_emit_interval"] = "1ms"
	opts["forward_received_to_node"] = true
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	stop := make(chan struct{})
	// 유입 goroutine.
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
			}
		}
	}()
	// 조회 goroutine.
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = ap.ListDevices()
			}
		}
	}()
	// drain goroutine(채널 포화 방지).
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-ap.msgCh:
			}
		}
	}()

	time.Sleep(60 * time.Millisecond)
	close(stop)
}

// AC-3.8 (clamp): 유효 tick 이 minStateEmitInterval 미만이면 하한 클램프 — panic 없이 스냅샷 방출.
func TestEmitMode_IntervalClampNoPanic(t *testing.T) {
	opts := directOpts()
	opts["state_emit_mode"] = "interval"
	opts["state_emit_interval"] = "1ns" // < minStateEmitInterval(1ms) → 클램프.
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()), "하한 클램프로 time.NewTicker panic 이 없어야 한다")
	defer func() { _ = ap.Stop(context.Background()) }()

	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true}`))
	events := drainEvents(t, ap, 80*time.Millisecond)
	assert.NotNil(t, firstEventOfType(events, "device_state_snapshot"))
}

// ---------------------------------------------------------------------------
// §5. 무회귀 (Module 4, M4)
// ---------------------------------------------------------------------------

// AC-5.1: 기본 설정(forward off, mode=event) 바이트 동일 — 신규 타입 미방출, on-change 불변.
func TestEmitMode_DefaultConfigNoRegression(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "line2"}}
	ap, mock := newDirectAgentWithMock(t, opts)
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// 변경 유입.
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	// 무변경 유입.
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	events := drainEvents(t, ap, 120*time.Millisecond)

	// 신규 타입은 절대 방출되지 않는다.
	assert.Nil(t, firstEventOfType(events, "device_state_received"), "기본 설정: device_state_received 미방출")
	assert.Nil(t, firstEventOfType(events, "device_state_snapshot"), "기본 설정: device_state_snapshot 미방출")
	// on-change 는 변경 시에만 방출(현행 동작 불변).
	assert.NotNil(t, firstEventOfType(events, "device_state_changed"), "변경 시 device_state_changed 방출")
}
