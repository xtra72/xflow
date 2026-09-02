package modbusserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// get_device_status 실효(effective) 레지스터 맵 노출 (SPEC-MODBUS-008)
// ---------------------------------------------------------------------------
//
// 이 파일은 get_device_status 가 디바이스의 "실제 서빙 store"(deviceView/backedStore/
// raw RegisterMap) 기준 실효 스냅샷·카운트를 반환하는지 검증한다.
//   - 공유 세그먼트 디바이스: 컨테이너 앨리어스 값 + 비영(non-zero) 카운트가 나타난다(버그 교정).
//   - 로컬 전용 디바이스: 기존 동작과 바이트 동일하다(특성 테스트, 회귀 방지).
//   - 백킹 디바이스: 마스터가 실제로 읽는 inner(폴/조회) 값이 나타난다.

// getDeviceStatusResp 는 get_device_status 응답을 파싱하여 반환하는 헬퍼이다.
func getDeviceStatusResp(t *testing.T, msa *ModbusServerAgent, unitID int) map[string]any {
	t.Helper()
	data, _ := json.Marshal(map[string]any{
		"command": "get_device_status",
		"params":  map[string]any{"unit_id": unitID},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	return result
}

// TestGetDeviceStatus_SharedSegment_ReturnsContainerAliasedValues — 공유 세그먼트 디바이스는
// get_device_status 에서 컨테이너 앨리어스 값과 비영 카운트를 노출한다(버그 교정 검증).
func TestGetDeviceStatus_SharedSegment_ReturnsContainerAliasedValues(t *testing.T) {
	srv := newSharedServer(t)

	// dev1 의 공유 holding 세그먼트(디바이스 주소 5 → 컨테이너 105)에 값을 쓴다.
	s1 := srv.deviceManager.GetDevice(1).ReqHandler.store
	_, err := s1.WriteHoldingRegisters(5, []uint16{0x1234})
	require.NoError(t, err)
	// dev1 의 로컬 coil 세그먼트에도 값을 쓴다.
	_, err = s1.WriteCoils(0, []bool{true})
	require.NoError(t, err)

	result := getDeviceStatusResp(t, srv, 1)

	// 응답 스키마 불변 확인.
	assert.Contains(t, result, "register_counts")
	assert.Contains(t, result, "register_map")

	// register_counts 는 실효 세그먼트를 반영한다: holding=50(공유), coils=16(로컬).
	// (버그 시절엔 로컬 전용 맵만 보아 holding_registers=0 이 되어 프런트가 빈 맵으로 오판했다.)
	rc, ok := result["register_counts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(50), rc["holding_registers"], "공유 holding 세그먼트 50개가 실효 카운트에 포함")
	assert.Equal(t, float64(16), rc["coils"], "로컬 coil 세그먼트 16개가 실효 카운트에 포함")
	assert.Equal(t, float64(0), rc["discrete_inputs"])
	assert.Equal(t, float64(0), rc["input_registers"])

	// register_map.holding_registers 는 컨테이너 앨리어스 값을 디바이스 주소로 노출한다.
	rm, ok := result["register_map"].(map[string]any)
	require.True(t, ok)
	hr, ok := rm["holding_registers"].(map[string]any)
	require.True(t, ok, "공유 holding 세그먼트가 register_map 에 나타나야 한다")
	assert.NotEmpty(t, hr, "isRegisterMapEmpty 가 공유 디바이스를 빈 맵으로 오판하지 않아야 한다")
	assert.Equal(t, float64(0x1234), hr["5"], "디바이스 주소 5 에 컨테이너 105 값이 앨리어싱")

	// 로컬 coil 값도 나타난다.
	coils, ok := rm["coils"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, coils["0"], "로컬 coil 0 값 노출")
}

// TestGetDeviceStatus_SharedSegment_LiveContainerWrite — 컨테이너에 직접 쓴 값이 공유
// 디바이스의 get_device_status 실효 맵에 즉시 반영된다.
func TestGetDeviceStatus_SharedSegment_LiveContainerWrite(t *testing.T) {
	srv := newSharedServer(t)
	container := srv.deviceManager.SharedContainer().RegisterMap

	// 컨테이너 주소 110 에 직접 쓰기 → dev2 디바이스 주소 10 으로 관측되어야 한다.
	_, err := container.WriteHoldingRegisters(110, []uint16{0xBEEF})
	require.NoError(t, err)

	result := getDeviceStatusResp(t, srv, 2)
	rm := result["register_map"].(map[string]any)
	hr := rm["holding_registers"].(map[string]any)
	assert.Equal(t, float64(0xBEEF), hr["10"], "컨테이너 110 값이 dev2 주소 10 에 실효 반영")
}

// TestGetDeviceStatus_LocalOnly_ByteIdentical — 로컬 전용 디바이스의 get_device_status
// register_map/register_counts 는 자체 RegisterMap 스냅샷과 바이트 동일하다(회귀 방지 특성 테스트).
func TestGetDeviceStatus_LocalOnly_ByteIdentical(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 로컬 전용 디바이스(unit 1)에 값을 써서 스냅샷이 자명하지 않게 만든다.
	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)
	_, err = dev.RegisterMap.WriteHoldingRegisters(10, []uint16{42})
	require.NoError(t, err)
	_, err = dev.RegisterMap.WriteCoils(3, []bool{true})
	require.NoError(t, err)

	// 기대치: 자체 RegisterMap 의 스냅샷/카운트(기존 동작).
	wantMap, _ := json.Marshal(dev.RegisterMap.GetSnapshot())
	wantCounts, _ := json.Marshal(dev.RegisterMap.RegisterCounts())

	result := getDeviceStatusResp(t, msa, 1)
	gotMap, _ := json.Marshal(result["register_map"])
	gotCounts, _ := json.Marshal(result["register_counts"])

	assert.JSONEq(t, string(wantMap), string(gotMap), "로컬 전용 register_map 은 자체 맵과 바이트 동일")
	assert.JSONEq(t, string(wantCounts), string(gotCounts), "로컬 전용 register_counts 은 자체 맵과 바이트 동일")
}

// TestGetDeviceStatus_Backed_ReturnsInnerServedValues — 백킹 디바이스의 get_device_status 는
// 마스터가 실제로 읽는 inner(direct 조회 캐시) 값을 노출한다.
func TestGetDeviceStatus_Backed_ReturnsInnerServedValues(t *testing.T) {
	msa, _ := startBackedAgent(t, BackingModeDirect)

	// direct 모드: 서빙 읽기 1회로 upstream(321)을 조회하여 inner 에 캐시한다.
	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)
	_ = dev.ReqHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1))

	result := getDeviceStatusResp(t, msa, 1)

	rc := result["register_counts"].(map[string]any)
	assert.Equal(t, float64(10), rc["holding_registers"], "백킹 inner 맵의 holding 카운트")

	rm := result["register_map"].(map[string]any)
	hr, ok := rm["holding_registers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(321), hr["0"], "direct 조회로 inner 에 캐시된 서빙 값이 노출")
}

// TestDeviceView_EffectiveSnapshotShape — deviceView.EffectiveSnapshot/EffectiveCounts 가
// RegisterMap.GetSnapshot/RegisterCounts 와 동일한 형태(영역 키/생략 규칙)를 따르는지 검증한다.
func TestDeviceView_EffectiveSnapshotShape(t *testing.T) {
	srv := newSharedServer(t)
	view, ok := srv.deviceManager.GetDevice(1).ReqHandler.store.(*deviceView)
	require.True(t, ok, "공유 세그먼트 디바이스의 store 는 *deviceView")

	counts := view.EffectiveCounts()
	// RegisterCounts 처럼 네 영역 키를 항상 포함한다.
	for _, k := range []string{"coils", "discrete_inputs", "holding_registers", "input_registers"} {
		assert.Contains(t, counts, k)
	}
	assert.Equal(t, 50, counts["holding_registers"])
	assert.Equal(t, 16, counts["coils"])

	snap := view.EffectiveSnapshot()
	// GetSnapshot 처럼 세그먼트 없는 영역 키는 생략한다.
	assert.Contains(t, snap, "holding_registers")
	assert.Contains(t, snap, "coils")
	assert.NotContains(t, snap, "discrete_inputs")
	assert.NotContains(t, snap, "input_registers")

	// 스냅샷 맵 크기는 세그먼트 길이 합과 일치한다.
	hr := snap["holding_registers"].(map[uint16]uint16)
	assert.Len(t, hr, 50)
}

// TestDeviceView_EffectiveSnapshot_AllAreas — 네 영역 모두 세그먼트를 가진 deviceView 의
// EffectiveSnapshot 이 모든 영역 키를 포함하는지 검증한다(영역별 분기 커버).
func TestDeviceView_EffectiveSnapshot_AllAreas(t *testing.T) {
	devices := []any{
		map[string]any{
			"unit_id": 0, "name": "shared",
			"register_map": map[string]any{
				"holding_registers": []any{hseg(0, 1000, -1)},
			},
		},
		map[string]any{
			"unit_id": 1, "name": "allareas",
			"register_map": map[string]any{
				"holding_registers": []any{hseg(0, 10, 100)}, // 공유 → deviceView 트리거
				"coils":             []any{map[string]any{"address": 0, "count": 8}},
				"discrete_inputs":   []any{map[string]any{"address": 0, "count": 8}},
				"input_registers":   []any{map[string]any{"address": 0, "count": 8}},
			},
		},
	}
	a, err := NewModbusServerAgent(multiDeviceAgentConfig("allareas-srv", devices))
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	view, ok := msa.deviceManager.GetDevice(1).ReqHandler.store.(*deviceView)
	require.True(t, ok)

	snap := view.EffectiveSnapshot()
	for _, k := range []string{"coils", "discrete_inputs", "holding_registers", "input_registers"} {
		assert.Contains(t, snap, k, "%s 영역이 실효 스냅샷에 포함되어야 한다", k)
	}
	counts := view.EffectiveCounts()
	assert.Equal(t, 8, counts["coils"])
	assert.Equal(t, 8, counts["discrete_inputs"])
	assert.Equal(t, 10, counts["holding_registers"])
	assert.Equal(t, 8, counts["input_registers"])
}

// unknownStore 는 registerStore 를 만족하지만 deviceView/backedStore/RegisterMap 어디에도
// 속하지 않는 테스트용 store 로, effectiveRegisterView 의 방어적 default 분기를 커버한다.
type unknownStore struct{}

func (unknownStore) ReadCoils(uint16, uint16) ([]bool, error)              { return nil, nil }
func (unknownStore) ReadDiscreteInputs(uint16, uint16) ([]bool, error)     { return nil, nil }
func (unknownStore) ReadHoldingRegisters(uint16, uint16) ([]uint16, error) { return nil, nil }
func (unknownStore) ReadInputRegisters(uint16, uint16) ([]uint16, error)   { return nil, nil }
func (unknownStore) WriteCoils(uint16, []bool) (*ChangeSet, error)         { return nil, nil }
func (unknownStore) WriteHoldingRegisters(uint16, []uint16) (*ChangeSet, error) {
	return nil, nil
}

// TestEffectiveRegisterView_UnknownStore_EmptyDefault — 알 수 없는 store 는 빈 스냅샷/카운트를
// 반환한다(방어적 default 분기 커버).
func TestEffectiveRegisterView_UnknownStore_EmptyDefault(t *testing.T) {
	rh := &RequestHandler{store: unknownStore{}}
	snap, counts := rh.effectiveRegisterView()
	assert.Empty(t, snap)
	assert.Empty(t, counts)
}
