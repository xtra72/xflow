package modbusserver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-012 M3 — 백킹 관측 메트릭 (REQ-06-01/02)
//
// 아래 테스트는 backedStore 의 upstream 카운터(reqCount/errCount/latencyNs)와
// 메트릭 스냅샷(metrics), 그리고 agent.go 의 get_device_status.backing /
// list_devices.backed·mode 노출을 검증한다. DeviceStats(서빙측)와는 별개다.
// ---------------------------------------------------------------------------

// directBackedStore 는 direct 모드 backedStore + fake 트랜스포트를 만든다(테스트 헬퍼).
func directBackedStore(resp func(pdu []byte) ([]byte, error)) (*backedStore, *fakeTransport) {
	fake := &fakeTransport{resp: resp}
	bs := newBackedStore(backedTestInner(), fake, &BackingConfig{
		Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second,
	})
	return bs, fake
}

// TestBackingMetrics_ReqCountIncrementsOnUpstreamCall — 성공 upstream 조회마다 reqCount 증가.
func TestBackingMetrics_ReqCountIncrementsOnUpstreamCall(t *testing.T) {
	bs, _ := directBackedStore(func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{42}), nil
	})

	m0 := bs.metrics()
	require.Equal(t, int64(0), m0.RequestCount, "초기 request_count 는 0 이어야 한다")
	require.Equal(t, int64(0), m0.ErrorCount)

	// direct 읽기 3회 → upstream 3회 호출.
	for i := 0; i < 3; i++ {
		_, err := bs.ReadHoldingRegisters(0, 1)
		require.NoError(t, err)
	}

	m := bs.metrics()
	assert.Equal(t, int64(3), m.RequestCount, "성공 조회마다 request_count 가 증가해야 한다")
	assert.Equal(t, int64(0), m.ErrorCount, "성공만 했으므로 error_count 는 0 이어야 한다")
	assert.Greater(t, m.AvgLatencyMs, float64(0), "요청이 있으면 avg_latency_ms 는 양수여야 한다")
	assert.Greater(t, m.LastOKMillis, int64(0), "성공 후 last_ok(ms)가 기록되어야 한다")
	assert.True(t, m.Connected, "성공 요청 후 direct 는 connected=true 여야 한다")
	assert.Equal(t, BackingModeDirect, m.Mode)
}

// TestBackingMetrics_ErrorIncrementsErrCount — upstream 트랜스포트 실패 시 errCount 증가 + connected=false.
func TestBackingMetrics_ErrorIncrementsErrCount(t *testing.T) {
	fail := true
	var mu sync.Mutex
	bs, _ := directBackedStore(func(pdu []byte) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			return nil, errors.New("upstream down")
		}
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{42}), nil
	})

	// 실패 2회.
	for i := 0; i < 2; i++ {
		_, err := bs.ReadHoldingRegisters(0, 1)
		require.Error(t, err)
	}
	m := bs.metrics()
	assert.Equal(t, int64(2), m.RequestCount, "실패도 요청 총수에 포함되어야 한다")
	assert.Equal(t, int64(2), m.ErrorCount, "트랜스포트 실패마다 error_count 가 증가해야 한다")
	assert.False(t, m.Connected, "직전 요청 실패 시 direct 는 connected=false 여야 한다")

	// 복구 후 성공 1회 → connected=true 로 회복.
	mu.Lock()
	fail = false
	mu.Unlock()
	_, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	m = bs.metrics()
	assert.Equal(t, int64(3), m.RequestCount)
	assert.Equal(t, int64(2), m.ErrorCount, "성공은 error_count 를 늘리지 않아야 한다")
	assert.True(t, m.Connected, "성공 요청 후 connected=true 로 회복되어야 한다")
}

// TestBackingMetrics_AvgLatency_ZeroWhenNoRequests — 요청 0 이면 avg_latency_ms 는 0(divide-by-zero 가드).
func TestBackingMetrics_AvgLatency_ZeroWhenNoRequests(t *testing.T) {
	bs, fake := directBackedStore(func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
	})
	m := bs.metrics()
	assert.Equal(t, float64(0), m.AvgLatencyMs, "요청이 없으면 avg_latency_ms 는 0 이어야 한다")
	assert.Equal(t, int64(0), m.LastOKMillis, "성공 이력이 없으면 last_ok 는 0 이어야 한다")

	// 요청 이력이 없으면 direct connected 는 트랜스포트 연결 상태로 대체한다.
	assert.False(t, m.Connected, "미연결 트랜스포트 + 요청 0 이면 connected=false")
	require.NoError(t, fake.Connect(context.Background()))
	assert.True(t, bs.metrics().Connected, "연결된 트랜스포트 + 요청 0 이면 connected=true")
}

// TestBackingMetrics_AvgLatency_Accumulates — 지연 주입 시 누적 레이턴시로 avg 가 산출된다.
func TestBackingMetrics_AvgLatency_Accumulates(t *testing.T) {
	bs, fake := directBackedStore(func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
	})
	fake.delay = 5 * time.Millisecond

	_, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	m := bs.metrics()
	assert.GreaterOrEqual(t, m.AvgLatencyMs, float64(5), "5ms 지연 주입 시 avg_latency_ms 는 5 이상이어야 한다")
}

// TestBackingMetrics_IndirectConnected_FromStaleness — indirect connected 는 !isStale 로 판정.
func TestBackingMetrics_IndirectConnected_FromStaleness(t *testing.T) {
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
	}}
	bs := newBackedStore(backedTestInner(), fake, &BackingConfig{
		Mode: BackingModeIndirect, UnitID: 2, Timeout: 50 * time.Millisecond,
	})

	// 폴 성공 이력이 없으면 stale → connected=false, last_ok=0.
	m := bs.metrics()
	assert.Equal(t, BackingModeIndirect, m.Mode)
	assert.False(t, m.Connected, "성공 폴 이력이 없으면 indirect connected=false")
	assert.Equal(t, int64(0), m.LastOKMillis)

	// 폴 성공 기록 → 신선(!stale) → connected=true, last_ok(ms) 기록.
	now := time.Now()
	bs.recordPollSuccess(now)
	m = bs.metrics()
	assert.True(t, m.Connected, "최근 폴 성공 시 indirect connected=true")
	assert.Equal(t, now.UnixNano()/int64(time.Millisecond), m.LastOKMillis, "indirect last_ok 는 폴 성공 시각(ms)")

	// timeout 초과 후 stale → connected=false.
	bs.recordPollSuccess(now.Add(-time.Second))
	assert.False(t, bs.metrics().Connected, "timeout 초과 시 stale → connected=false")
}

// ---------------------------------------------------------------------------
// agent.go 노출 — get_device_status.backing / list_devices.backed·mode
// ---------------------------------------------------------------------------

// startBackedAgent 는 direct 백킹 디바이스(unit 1) + 순수 slave(unit 2) 에이전트를 기동한다.
func startBackedAgent(t *testing.T, mode string) (*ModbusServerAgent, *fakeTransport) {
	t.Helper()
	cfg := backedAgentConfig(mode, "10ms")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)
	fake := installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{321}), nil
	})
	require.NoError(t, msa.Start(context.Background()))
	t.Cleanup(func() { _ = msa.Stop(context.Background()) })
	return msa, fake
}

// TestProcess_GetDeviceStatus_BackedIncludesBackingObject — 백킹 디바이스는 backing 서브객체를 포함한다.
func TestProcess_GetDeviceStatus_BackedIncludesBackingObject(t *testing.T) {
	msa, _ := startBackedAgent(t, BackingModeDirect)

	// 서빙 요청 1회로 upstream 카운터를 증가시킨다.
	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)
	_ = dev.ReqHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1))

	data, _ := json.Marshal(map[string]any{
		"command": "get_device_status",
		"params":  map[string]any{"unit_id": 1},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))

	backing, ok := result["backing"].(map[string]any)
	require.True(t, ok, "백킹 디바이스는 backing 서브객체를 가져야 한다")
	assert.Equal(t, BackingModeDirect, backing["mode"])
	assert.Equal(t, true, backing["connected"])
	assert.Equal(t, float64(1), backing["request_count"], "서빙 1회 후 request_count=1")
	assert.Equal(t, float64(0), backing["error_count"])
	assert.Contains(t, backing, "avg_latency_ms")
	assert.Contains(t, backing, "last_ok")

	// 기존 필드 불변(하위 호환): stats/register_map 등 유지.
	assert.Contains(t, result, "stats")
	assert.Contains(t, result, "register_map")
	assert.Contains(t, result, "register_counts")
}

// TestProcess_GetDeviceStatus_UnbackedIsNull — 순수 slave 디바이스는 backing=null 을 반환한다.
func TestProcess_GetDeviceStatus_UnbackedIsNull(t *testing.T) {
	msa, _ := startBackedAgent(t, BackingModeDirect)

	data, _ := json.Marshal(map[string]any{
		"command": "get_device_status",
		"params":  map[string]any{"unit_id": 2},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))

	backing, exists := result["backing"]
	require.True(t, exists, "backing 키는 존재해야 한다")
	assert.Nil(t, backing, "비백킹 디바이스는 backing=null 이어야 한다")
}

// TestProcess_ListDevices_BackedAndMode — list_devices 는 각 디바이스에 backed/mode 를 포함한다.
func TestProcess_ListDevices_BackedAndMode(t *testing.T) {
	msa, _ := startBackedAgent(t, BackingModeIndirect)

	data, _ := json.Marshal(map[string]any{"command": "list_devices"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	devices := result["devices"].([]any)
	require.Len(t, devices, 2)

	byUnit := map[float64]map[string]any{}
	for _, d := range devices {
		m := d.(map[string]any)
		byUnit[m["unit_id"].(float64)] = m
	}

	// unit 1: 백킹(indirect).
	d1 := byUnit[1]
	assert.Equal(t, true, d1["backed"], "백킹 디바이스는 backed=true")
	assert.Equal(t, BackingModeIndirect, d1["mode"])

	// unit 2: 순수 slave.
	d2 := byUnit[2]
	assert.Equal(t, false, d2["backed"], "순수 slave 는 backed=false")
	assert.Equal(t, "", d2["mode"], "비백킹은 mode 빈 문자열")

	// 기존 필드 불변.
	assert.Contains(t, d1, "stats")
	assert.Contains(t, d1, "register_counts")
	assert.Equal(t, "active", d1["status"])
}

// TestBackingMetrics_RaceSafe — 서빙(direct sendUpstream) + 메트릭 조회 동시 접근이 race-free 임을 검증한다.
// `go test -race` 하에서 폴러/서빙/조회 카운터 경합을 잡아낸다.
func TestBackingMetrics_RaceSafe(t *testing.T) {
	bs, fake := directBackedStore(func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{7}), nil
	})
	require.NoError(t, fake.Connect(context.Background()))

	var wg sync.WaitGroup
	// 서빙 goroutine 다수: 각자 upstream 조회(sendUpstream → atomic 카운터 증가).
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = bs.ReadHoldingRegisters(0, 1)
			}
		}()
	}
	// 조회 goroutine: 메트릭 스냅샷을 반복 읽는다.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = bs.metrics()
			}
		}()
	}
	wg.Wait()

	m := bs.metrics()
	assert.Equal(t, int64(8*50), m.RequestCount, "모든 서빙 요청이 원자적으로 집계되어야 한다")
	assert.Equal(t, int64(0), m.ErrorCount)
}
