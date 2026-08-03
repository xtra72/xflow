package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M4 — F1 런타임 디바이스 등록 add/remove (REQ-MODBUS-008-04, AC-07/AC-08)
// ---------------------------------------------------------------------------

// makeAddDeviceCmd 는 add_device 명령 JSON 을 만든다(TCP 디바이스).
func makeAddDeviceCmd(id, host string, groups []any) []byte {
	b, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"id":              id,
			"host":            host,
			"port":            502,
			"unit_id":         5,
			"register_groups": groups,
		},
	})
	return b
}

// oneGroup 는 단일 레지스터 그룹 슬라이스를 만든다.
func oneGroup(name string) []any {
	return []any{
		map[string]any{"name": name, "function_code": 3, "start_address": 0, "quantity": 4},
	}
}

// TestAddDevice_RuntimeRegistration 는 재시작 없이 디바이스가 추가되어 devices/캐시/통계에
// 편입됨을 검증한다(AC-07). connect on add 는 대상이 없어 실패하지만(오프라인 편입) 등록 자체는 성공한다.
func TestAddDevice_RuntimeRegistration(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	require.Len(t, a.devices, 1)

	resp, err := a.Process(makeAddDeviceCmd("dyn-1", "127.0.0.1", oneGroup("g1")))
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp, &out))
	assert.Equal(t, "device_added", out["status"])
	assert.Equal(t, "dyn-1", out["device_id"])

	// devices/캐시/통계 맵에 신규 디바이스가 편입되어야 한다(런타임 통계 확장, AC-10).
	require.Len(t, a.devices, 2)
	_, hasDev := a.findDevice("dyn-1")
	assert.NoError(t, hasDev)
	_, hasCache := a.caches["dyn-1"]
	assert.True(t, hasCache, "신규 디바이스 캐시가 생성되어야 한다")
	_, hasDevStat := a.devStats["dyn-1"]
	assert.True(t, hasDevStat, "신규 디바이스 통계가 생성되어야 한다")
	_, hasGroupStat := a.groupStats[groupStatKey("dyn-1", "g1")]
	assert.True(t, hasGroupStat, "신규 그룹 통계가 생성되어야 한다")

	// 기존 디바이스(plc-1)의 통계/캐시는 보존되어야 한다(copy-on-write).
	_, keepCache := a.caches["plc-1"]
	assert.True(t, keepCache, "기존 디바이스 캐시는 보존되어야 한다")
}

// TestAddDevice_DuplicateRejectedAtomically 는 중복 ID 추가가 원자적으로 거부되고 상태가
// 변하지 않음을 검증한다(AC-07, 부분 적용 없음).
func TestAddDevice_DuplicateRejectedAtomically(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	// plc-1 은 이미 존재한다.
	_, err := a.Process(makeAddDeviceCmd("plc-1", "127.0.0.1", oneGroup("g")))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateDevice)

	// 상태 불변: 디바이스 수 유지.
	assert.Len(t, a.devices, 1)
}

// TestAddDevice_InvalidConfigRejected 는 유효하지 않은 디바이스(TCP host 누락) 추가가
// 거부되고 부분 적용이 없음을 검증한다(AC-07).
func TestAddDevice_InvalidConfigRejected(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	// host 누락(에이전트 기본 tcp) → parseDeviceConfig 검증 실패.
	bad, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"id":              "nohost",
			"unit_id":         3,
			"register_groups": oneGroup("g"),
		},
	})
	_, err := a.Process(bad)
	require.Error(t, err)

	// 부분 적용 없음: 디바이스/캐시/통계 미생성.
	assert.Len(t, a.devices, 1)
	_, hasCache := a.caches["nohost"]
	assert.False(t, hasCache)
}

// TestAddDevice_MissingID 는 id 없는 add 가 거부됨을 검증한다.
func TestAddDevice_MissingID(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	bad, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"host":            "127.0.0.1",
			"register_groups": oneGroup("g"),
		},
	})
	_, err := a.Process(bad)
	assert.ErrorIs(t, err, ErrMissingDeviceID)
}

// TestAddDevice_MissingParams 는 params 없는 add 가 거부됨을 검증한다.
func TestAddDevice_MissingParams(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	bad, _ := json.Marshal(map[string]any{"command": "add_device"})
	_, err := a.Process(bad)
	require.Error(t, err)
	assert.Len(t, a.devices, 1)
}

// TestAddDevice_PollingScheduleJoin 는 폴링 중 추가된 디바이스의 poll_interval 그룹에 대해
// 전용 스케줄러가 시작됨을 검증한다(AC-07, 다음 폴부터 폴링).
func TestAddDevice_PollingScheduleJoin(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, cfg, mt)

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	// poll_interval 지정 그룹을 가진 디바이스 추가.
	groups := []any{
		map[string]any{"name": "pg", "function_code": 3, "start_address": 0, "quantity": 4, "poll_interval": "50ms"},
	}
	_, err := a.Process(makeAddDeviceCmd("dyn-poll", "127.0.0.1", groups))
	require.NoError(t, err)

	// 전용 그룹 스케줄러가 등록되어야 한다(재시작 없이 폴링 편입).
	a.mu.RLock()
	_, hasLoop := a.groupStops[groupStatKey("dyn-poll", "pg")]
	a.mu.RUnlock()
	assert.True(t, hasLoop, "추가된 poll_interval 그룹의 전용 스케줄러가 시작되어야 한다")
}

// TestRemoveDevice_Runtime 는 디바이스 삭제가 devices/캐시/통계에서 제거하고 트랜스포트를
// close 함을 검증한다(AC-08).
func TestRemoveDevice_Runtime(t *testing.T) {
	cfg := twoDeviceAgentConfig()
	mt1 := &mockModbusTransport{connected: true}
	mt2 := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt1, mt2)
	require.Len(t, a.devices, 2)

	cmd, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": "plc-1"})
	resp, err := a.Process(cmd)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp, &out))
	assert.Equal(t, "device_removed", out["status"])

	require.Len(t, a.devices, 1)
	assert.Equal(t, "plc-2", a.devices[0].config.ID)
	_, hasCache := a.caches["plc-1"]
	assert.False(t, hasCache, "제거된 디바이스 캐시는 삭제되어야 한다")
	_, hasStat := a.devStats["plc-1"]
	assert.False(t, hasStat, "제거된 디바이스 통계는 삭제되어야 한다")

	// 독립 트랜스포트(plc-1)는 close 되어야 한다.
	assert.GreaterOrEqual(t, mt1.closeCnt, 1, "제거된 독립 디바이스의 트랜스포트는 close 되어야 한다")
	assert.Equal(t, 0, mt2.closeCnt, "남은 디바이스의 트랜스포트는 close 되지 않아야 한다")
}

// TestRemoveDevice_NotFoundRejected 는 미존재 디바이스 삭제가 오류로 거부되고 상태가
// 변하지 않음을 검증한다(AC-08).
func TestRemoveDevice_NotFoundRejected(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	cmd, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": "ghost"})
	_, err := a.Process(cmd)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	assert.Len(t, a.devices, 1, "미존재 삭제 후 상태 불변")
	assert.Equal(t, 0, mt.closeCnt, "미존재 삭제는 아무 트랜스포트도 close 하지 않아야 한다")
}

// TestRemoveDevice_MissingID 는 device_id 없는 remove 가 거부됨을 검증한다.
func TestRemoveDevice_MissingID(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	cmd, _ := json.Marshal(map[string]any{"command": "remove_device"})
	_, err := a.Process(cmd)
	assert.ErrorIs(t, err, ErrMissingDeviceID)
}

// TestRemoveDevice_IDInParams 는 device_id 를 params 로도 받을 수 있음을 검증한다(명령 표면 유연성).
func TestRemoveDevice_IDInParams(t *testing.T) {
	cfg := minimalAgentConfig()
	mt := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mt)

	cmd, _ := json.Marshal(map[string]any{
		"command": "remove_device",
		"params":  map[string]any{"device_id": "plc-1"},
	})
	_, err := a.Process(cmd)
	require.NoError(t, err)
	assert.Len(t, a.devices, 0)
}

// TestCloseRemovedTransport_RawSharedLastRef 는 원시(비래핑) 트랜스포트를 포인터로 공유하는
// 디바이스(기본 RTU 단일 버스 선례)의 마지막-참조 close 규칙을 검증한다(AC-08).
func TestCloseRemovedTransport_RawSharedLastRef(t *testing.T) {
	cfg := minimalAgentConfig()
	mtInit := &mockModbusTransport{connected: true}
	a, _ := newTestModbusAgent(t, cfg, mtInit)

	// 하나의 mock 트랜스포트를 두 디바이스가 포인터로 공유한다.
	shared := &mockModbusTransport{connected: true}
	dev1 := newModbusDeviceWithTransport(DeviceConfig{ID: "s1"}, shared, a.logger)
	dev2 := newModbusDeviceWithTransport(DeviceConfig{ID: "s2"}, shared, a.logger)

	// dev1 제거 → dev2 가 아직 같은 인스턴스를 참조하므로 close 계획이 없어야 한다(nil).
	if closeFn := a.planTransportCloseLocked(dev1, []*ModbusDevice{dev2}); closeFn != nil {
		closeFn()
	}
	assert.Equal(t, 0, shared.closeCnt, "남은 디바이스가 참조 중이면 원시 공유 트랜스포트는 close 되지 않아야 한다")

	// dev2 제거(마지막 참조) → close 계획이 반환되고 실행 시 실제 close.
	if closeFn := a.planTransportCloseLocked(dev2, nil); closeFn != nil {
		closeFn()
	}
	assert.Equal(t, 1, shared.closeCnt, "마지막 참조 제거 시 원시 공유 트랜스포트가 close 되어야 한다")
}

// TestRemoveDevice_SharedTransportLastRefClose 는 F3 세션 공유(*sharedTransport) 참조 카운팅의
// 마지막-참조 close 를 런타임 add/remove 로 검증한다(AC-08).
func TestRemoveDevice_SharedTransportLastRefClose(t *testing.T) {
	// 세션 공유 활성 + 동일 엔드포인트 초기 디바이스 1개(실 빌드 경로).
	a := newAgentFromOpts(t, map[string]any{
		"transport":       "tcp",
		"share_session":   true,
		"request_timeout": "150ms",
		"devices":         []any{deviceMapUnit("d1", "10.9.9.9", 1)},
	})
	require.Len(t, a.devices, 1)

	// 런타임으로 동일 엔드포인트 공유 디바이스 추가 → 동일 sharedTransport 에 참조 추가(refCount 2).
	add := makeAddDeviceCmdEndpoint("d2", "10.9.9.9", 502, 2)
	_, err := a.Process(add)
	require.NoError(t, err)

	st, ok := a.devices[0].transport.(*sharedTransport)
	require.True(t, ok, "공유 활성 시 초기 디바이스는 sharedTransport 여야 한다")
	assert.Equal(t, 2, st.refCount(), "런타임 추가로 공유 참조가 2가 되어야 한다")

	// d1 제거 → 마지막 참조가 아니므로 하부는 닫히지 않는다(다른 디바이스 트랜잭션 보존, AC-08).
	rm1, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": "d1"})
	_, err = a.Process(rm1)
	require.NoError(t, err)
	assert.Equal(t, 1, st.refCount(), "한 디바이스 제거 후 참조는 1이어야 한다")
	st.mu.Lock()
	closedAfterFirst := st.closed
	st.mu.Unlock()
	assert.False(t, closedAfterFirst, "마지막 참조가 아니면 공유 연결은 close 되지 않아야 한다")

	// d2 제거(마지막 참조) → 실제 close.
	rm2, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": "d2"})
	_, err = a.Process(rm2)
	require.NoError(t, err)
	assert.Equal(t, 0, st.refCount(), "마지막 디바이스 제거 후 참조는 0이어야 한다")
	st.mu.Lock()
	closedAfterLast := st.closed
	st.mu.Unlock()
	assert.True(t, closedAfterLast, "마지막 참조 제거 시 공유 연결이 close 되어야 한다")
	assert.Len(t, a.devices, 0)
}

// TestRuntimeAddRemove_RaceWithPolling 는 폴링 goroutine 과 런타임 add/remove·상태 조회가
// 경합 없이 동작함을 검증한다(-race, AC-10). `go test -race` 로 실행 시 데이터 경합을 잡는다.
func TestRuntimeAddRemove_RaceWithPolling(t *testing.T) {
	cfg := minimalAgentConfig()
	cfg.Transport.Options["poll_interval"] = "5ms"
	mt := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 10)}
	a, _ := newTestModbusAgent(t, cfg, mt)

	require.NoError(t, a.Start(context.Background()))
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	done := make(chan struct{})
	var wg sync.WaitGroup

	// add/remove 반복 goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-done:
				return
			default:
			}
			id := fmt.Sprintf("dyn-%d", i)
			_, _ = a.Process(makeAddDeviceCmd(id, "127.0.0.1", oneGroup("g")))
			rm, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": id})
			_, _ = a.Process(rm)
			i++
		}
	}()

	// 상태 조회 goroutine(읽기 경로 경합 검증).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = a.State()
			_ = a.Health()
			_ = a.ConnectionStats()
			st, _ := json.Marshal(map[string]any{"command": "get_status"})
			_, _ = a.Process(st)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// deviceMapUnit 는 지정 unit_id 를 갖는 TCP 디바이스 맵을 만든다(공유 테스트용).
func deviceMapUnit(id, host string, unitID int) map[string]any {
	return map[string]any{
		"id":      id,
		"host":    host,
		"port":    502,
		"unit_id": unitID,
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
		},
	}
}

// makeAddDeviceCmdEndpoint 는 특정 (host,port,unit_id) 로 add_device 명령을 만든다.
func makeAddDeviceCmdEndpoint(id, host string, port, unitID int) []byte {
	b, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"id":      id,
			"host":    host,
			"port":    port,
			"unit_id": unitID,
			"register_groups": []any{
				map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
			},
		},
	})
	return b
}
