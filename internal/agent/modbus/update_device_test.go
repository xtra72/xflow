package modbus

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// M2 — F2 백엔드 update_device (REQ-MODBUS-009-02, AC-03/AC-04)
// update_device 는 agent.Process([]byte) 경계에서 검증 가능하므로(노드가 마샬링하는 것과 동일한
// {"command":"update_device",...} JSON), 여기서는 에이전트 핸들러를 직접 구동해 검증한다.
// ---------------------------------------------------------------------------

// updateDeviceJSON 은 update_device 명령 JSON 을 만든다.
func updateDeviceJSON(t *testing.T, deviceID string, params map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"command":   "update_device",
		"device_id": deviceID,
		"params":    params,
	})
	require.NoError(t, err)
	return b
}

// TestUpdateDevice_InPlaceEdit_ConnectionPreserved 는 update_device 가 대상 디바이스의 설정을
// in-place 로 변경하되 트랜스포트/연결을 재생성하지 않음을 검증한다(AC-03, 연결 유지 = 제거+재추가 아님).
func TestUpdateDevice_InPlaceEdit_ConnectionPreserved(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("200ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	dev, err := a.findDevice("plc-1")
	require.NoError(t, err)

	// 변경 전 스냅샷: 디바이스 포인터 정체성, 트랜스포트 정체성, connect 호출 횟수.
	transportBefore := dev.transport
	mt.mu.Lock()
	connectCntBefore := mt.connectCnt
	mt.mu.Unlock()
	require.Equal(t, 200*time.Millisecond, a.testGroupInterval("plc-1", "group-a"))

	// update_device: register_groups(단축 케이던스) + poll_interval 변경.
	cmd := updateDeviceJSON(t, "plc-1", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name": "group-a", "function_code": 3,
				"start_address": 0, "quantity": 10, "poll_interval": "20ms",
			},
		},
		"poll_interval": "500ms",
	})
	resp, err := a.Process(cmd)
	require.NoError(t, err, "유효한 update_device 는 성공해야 한다")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(resp, &decoded))
	assert.Equal(t, "device_updated", decoded["status"])
	assert.Equal(t, "plc-1", decoded["device_id"])
	applied, _ := decoded["applied"].([]any)
	assert.Contains(t, applied, "register_groups")
	assert.Contains(t, applied, "poll_interval")

	// in-place 변경 반영.
	assert.Equal(t, 20*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
		"단축된 그룹 poll_interval 이 반영되어야 한다")
	a.mu.RLock()
	assert.Equal(t, 500*time.Millisecond, a.config.PollInterval)
	a.mu.RUnlock()

	// 연결 유지(HARD): 디바이스/트랜스포트 정체성이 동일해야 하고, 재연결이 발생하지 않아야 한다.
	devAfter, err := a.findDevice("plc-1")
	require.NoError(t, err)
	assert.Same(t, dev, devAfter, "디바이스 인스턴스가 재생성되지 않아야 한다(제거+재추가 아님)")
	assert.Same(t, transportBefore, devAfter.transport, "트랜스포트 인스턴스가 재생성되지 않아야 한다(연결 유지)")
	mt.mu.Lock()
	connectCntAfter := mt.connectCnt
	mt.mu.Unlock()
	assert.Equal(t, connectCntBefore, connectCntAfter, "update_device 는 재연결(Connect)을 유발하지 않아야 한다")

	assert.Equal(t, lifecycle.StateRunning, a.CurrentState(), "재시작 없이 계속 Running 이어야 한다")

	// 변경된 케이던스로 계속 폴링됨을 관측(연결 유지 하 새 그룹 설정으로 동작).
	require.Eventually(t, func() bool {
		mt.mu.Lock()
		defer mt.mu.Unlock()
		return len(mt.sentFrames) > 0
	}, time.Second, 10*time.Millisecond, "변경 후에도 폴링이 계속되어야 한다")
}

// TestUpdateDevice_UnitID_InPlace 는 update_device 로 unit_id 를 연결 유지한 채 변경할 수 있음을 검증한다.
func TestUpdateDevice_UnitID_InPlace(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	dev, err := a.findDevice("plc-1")
	require.NoError(t, err)
	require.Equal(t, byte(1), dev.UnitID())
	transportBefore := dev.transport

	resp, err := a.Process(updateDeviceJSON(t, "plc-1", map[string]any{"unit_id": 42}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(resp, &decoded))
	assert.Equal(t, "device_updated", decoded["status"])

	assert.Equal(t, byte(42), dev.UnitID(), "런타임 변경된 unit_id 가 원자 접근자에 반영되어야 한다")
	assert.Same(t, transportBefore, dev.transport, "unit_id 변경은 트랜스포트를 재생성하지 않아야 한다")
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

// TestUpdateDevice_NonexistentID_AtomicReject 는 존재하지 않는 device_id 에 대한 update_device 가
// ErrDeviceNotFound 로 원자적으로 거부되고, 기존 디바이스 상태가 변형되지 않음을 검증한다(AC-04, 부분 적용 없음).
func TestUpdateDevice_NonexistentID_AtomicReject(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("100ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 기존 디바이스(plc-1)의 상태를 스냅샷한다.
	require.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"))

	cmd := updateDeviceJSON(t, "does-not-exist", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name": "group-a", "function_code": 3,
				"start_address": 0, "quantity": 10, "poll_interval": "10ms",
			},
		},
	})
	_, err := a.Process(cmd)
	require.Error(t, err, "미존재 device_id 는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrDeviceNotFound, "ErrDeviceNotFound 로 거부되어야 한다")

	// 부분 적용 없음: 기존 디바이스 설정과 개수가 그대로 유지되어야 한다(직전 상태 계속 동작).
	assert.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
		"거부 시 기존 디바이스 설정이 그대로 유지되어야 한다(부분 적용 금지)")
	a.mu.RLock()
	assert.Len(t, a.devices, 1, "디바이스 개수는 변하지 않아야 한다")
	a.mu.RUnlock()
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

// TestUpdateDevice_InitOnlyField_Rejected 는 update_device 가 transport 전환·RTU 시리얼 하드웨어
// 파라미터 등 init 전용 필드 변경을 rejectInitOnlyFields 규칙으로 거부함을 검증한다(AC-04).
func TestUpdateDevice_InitOnlyField_Rejected(t *testing.T) {
	initOnlyKeys := []string{"transport", "serial_port", "baud_rate", "data_bits", "stop_bits", "parity", "port", "baud"}
	for _, key := range initOnlyKeys {
		t.Run(key, func(t *testing.T) {
			mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
			a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("100ms"), mt)
			require.NoError(t, a.Start(context.Background()))
			defer func() { _ = a.Stop(context.Background()) }()

			dev, err := a.findDevice("plc-1")
			require.NoError(t, err)
			transportBefore := dev.transport

			cmd := updateDeviceJSON(t, "plc-1", map[string]any{key: "some-value"})
			_, err = a.Process(cmd)
			require.Error(t, err, "%s 는 init 전용으로 거부되어야 한다", key)
			assert.ErrorIs(t, err, ErrInitOnlyField, "%s 는 ErrInitOnlyField 로 거부되어야 한다", key)

			// 원자적 거부: 트랜스포트/설정이 변형되지 않고 계속 Running 이어야 한다.
			assert.Same(t, transportBefore, dev.transport, "거부 시 트랜스포트가 재생성되지 않아야 한다")
			assert.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"))
			assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
		})
	}
}

// TestUpdateDevice_MissingDeviceID_Rejected 는 device_id 없는 update_device 가 ErrMissingDeviceID 로
// 거부됨을 검증한다(디바이스 단위 수정 표면).
func TestUpdateDevice_MissingDeviceID_Rejected(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	_, err := a.Process(updateDeviceJSON(t, "", map[string]any{"unit_id": 5}))
	require.Error(t, err, "device_id 없는 update_device 는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrMissingDeviceID)
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

// TestUpdateDevice_ParamsFromParamsDeviceID 는 device_id 가 최상위 필드가 아닌 params.device_id 로
// 전달되어도 대상 디바이스를 찾아 수정함을 검증한다(remove_device 명령 표면과 정합).
func TestUpdateDevice_ParamsFromParamsDeviceID(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 최상위 device_id 는 비우고 params.device_id 로 전달.
	b, err := json.Marshal(map[string]any{
		"command": "update_device",
		"params":  map[string]any{"device_id": "plc-1", "unit_id": 9},
	})
	require.NoError(t, err)

	_, err = a.Process(b)
	require.NoError(t, err)

	dev, err := a.findDevice("plc-1")
	require.NoError(t, err)
	assert.Equal(t, byte(9), dev.UnitID())
}

// TestUpdateDevice_InvalidGroup_NoPartialApply 는 유효하지 않은 register_groups 가 거부되고
// 직전 설정이 부분 적용 없이 유지됨을 검증한다(AC-04, 적용 전 전량 검증).
func TestUpdateDevice_InvalidGroup_NoPartialApply(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("100ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	cmd := updateDeviceJSON(t, "plc-1", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name": "bad", "function_code": 9, // 무효 function_code
				"start_address": 0, "quantity": 10,
			},
		},
	})
	_, err := a.Process(cmd)
	require.Error(t, err, "유효하지 않은 register_groups 는 거부되어야 한다")

	// 부분 적용 없음: 직전 설정 유지.
	assert.Equal(t, 100*time.Millisecond, a.testGroupInterval("plc-1", "group-a"),
		"검증 실패 시 직전 설정이 그대로 유지되어야 한다(부분 적용 금지)")
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

// TestUpdateDevice_GroupStatsReconciled 는 register_groups 변경 시 그룹 통계 맵이 copy-on-write 로
// 정합화되어(사라진 키 제거 + 신규 키 추가), 새 그룹의 통계가 추적됨을 검증한다(SPEC-008 패턴 정합).
func TestUpdateDevice_GroupStatsReconciled(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, minimalAgentConfig(), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 초기 그룹 "holding_0-9" 의 통계 키가 존재해야 한다.
	a.mu.RLock()
	_, hadOld := a.groupStats[groupStatKey("plc-1", "holding_0-9")]
	a.mu.RUnlock()
	require.True(t, hadOld, "초기 그룹 통계 키가 존재해야 한다")

	// register_groups 를 신규 그룹 이름으로 교체.
	cmd := updateDeviceJSON(t, "plc-1", map[string]any{
		"register_groups": []any{
			map[string]any{
				"name": "renamed_group", "function_code": 3,
				"start_address": 0, "quantity": 10,
			},
		},
	})
	_, err := a.Process(cmd)
	require.NoError(t, err)

	a.mu.RLock()
	_, hasNew := a.groupStats[groupStatKey("plc-1", "renamed_group")]
	_, hasOld := a.groupStats[groupStatKey("plc-1", "holding_0-9")]
	a.mu.RUnlock()
	assert.True(t, hasNew, "신규 그룹 통계 키가 추가되어야 한다")
	assert.False(t, hasOld, "사라진 그룹 통계 키는 제거되어야 한다")
}

// TestUpdateDevice_ConcurrentPolling_RaceSafe 는 폴링 goroutine 이 동작 중인 상태에서 update_device 를
// 반복 발행해도 데이터 경합이 없음을 검증한다(`go test -race`, 폴링 vs 그룹/통계 copy-on-write 경합).
func TestUpdateDevice_ConcurrentPolling_RaceSafe(t *testing.T) {
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	// 빠른 케이던스로 폴링하여 핫 패스와 update_device 갱신이 자주 겹치게 한다.
	a, _ := newTestModbusAgent(t, oneGroupWithIntervalConfig("5ms"), mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		mt.mu.Lock()
		defer mt.mu.Unlock()
		return len(mt.sentFrames) > 0
	}, time.Second, 5*time.Millisecond, "폴링이 시작되어야 한다")

	// 폴링과 동시에 update_device 로 그룹/unit_id 를 반복 변경(경합 창 최대화).
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, uid := range []int{2, 3, 4, 5, 6, 7} {
			cmd := updateDeviceJSON(t, "plc-1", map[string]any{
				"unit_id": uid,
				"register_groups": []any{
					map[string]any{
						"name": "group-a", "function_code": 3,
						"start_address": uint16(i * 10), "quantity": 10, "poll_interval": "5ms",
					},
				},
			})
			if _, err := a.Process(cmd); err != nil {
				t.Errorf("동시 update_device(#%d) 는 성공해야 한다: %v", i, err)
			}
			time.Sleep(3 * time.Millisecond)
		}
	}()
	wg.Wait()

	assert.Equal(t, lifecycle.StateRunning, a.CurrentState(), "재시작 없이 계속 Running 이어야 한다")
}
