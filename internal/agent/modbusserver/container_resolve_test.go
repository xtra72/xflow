package modbusserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newNoContainerServer 는 공유 컨테이너(unit_id 0) 없이 서빙 디바이스(unit_id 1)만 갖는
// 서버를 만든다. unit_id 0 대상 exec 명령이 ErrNoSharedContainer 를 반환하는지 검증에 사용한다.
func newNoContainerServer(t *testing.T) *ModbusServerAgent {
	t.Helper()
	devices := []any{
		map[string]any{
			"unit_id": 1, "name": "dev1",
			"register_map": map[string]any{
				"holding_registers": []any{map[string]any{"address": 0, "count": 100}},
			},
		},
	}
	a, err := NewModbusServerAgent(multiDeviceAgentConfig("no-container-srv", devices))
	require.NoError(t, err)
	return a.(*ModbusServerAgent)
}

// TestResolveRegisterMap_SharedContainer 은 unit_id 0 이 명시된 경우 공유 컨테이너의
// RegisterMap 으로, unit_id 1 이 명시된 경우 서빙 디바이스의 RegisterMap 으로 해석되는지,
// 그리고 컨테이너가 없는 서버에서 unit_id 0 이 ErrNoSharedContainer 를 반환하는지 검증한다.
func TestResolveRegisterMap_SharedContainer(t *testing.T) {
	srv := newSharedServer(t)

	t.Run("unit_id 0 resolves shared container map", func(t *testing.T) {
		rm, err := srv.resolveRegisterMap(map[string]any{"unit_id": 0})
		require.NoError(t, err)
		require.NotNil(t, rm)
		assert.Same(t, srv.deviceManager.SharedContainer().RegisterMap, rm,
			"unit_id 0 은 공유 컨테이너의 RegisterMap 포인터를 반환해야 함")
	})

	t.Run("unit_id 1 resolves served device map", func(t *testing.T) {
		rm, err := srv.resolveRegisterMap(map[string]any{"unit_id": 1})
		require.NoError(t, err)
		require.NotNil(t, rm)
		assert.Same(t, srv.deviceManager.GetDevice(1).RegisterMap, rm,
			"unit_id 1 은 서빙 디바이스 1 의 RegisterMap 을 반환해야 함")
		assert.NotSame(t, srv.deviceManager.SharedContainer().RegisterMap, rm,
			"서빙 디바이스 맵은 컨테이너 맵과 달라야 함")
	})

	t.Run("absent unit_id falls back to default device (unchanged)", func(t *testing.T) {
		rm, err := srv.resolveRegisterMap(map[string]any{})
		require.NoError(t, err)
		assert.Same(t, srv.registerMap, rm, "unit_id 미지정 시 기존 기본 디바이스 경로 유지")
	})

	t.Run("no container + unit_id 0 returns ErrNoSharedContainer", func(t *testing.T) {
		bare := newNoContainerServer(t)
		require.Nil(t, bare.deviceManager.SharedContainer(), "이 서버에는 컨테이너가 없어야 함")

		rm, err := bare.resolveRegisterMap(map[string]any{"unit_id": 0})
		assert.Nil(t, rm)
		assert.ErrorIs(t, err, ErrNoSharedContainer)
	})
}

// TestExecCommand_TargetsSharedContainer 은 get/set 레지스터 명령(Process 경로)이
// unit_id 0 을 통해 공유 컨테이너를 대상으로 읽고 쓰는지, 컨테이너가 없으면
// ErrNoSharedContainer 로 실패하는지 end-to-end 로 검증한다.
func TestExecCommand_TargetsSharedContainer(t *testing.T) {
	srv := newSharedServer(t)
	container := srv.deviceManager.SharedContainer().RegisterMap

	// set_register(unit_id=0, addr=200) 는 컨테이너 주소 200 에 직접 쓴다.
	// 200 은 컨테이너 holding[0..1000) 범위 안이며 공유 창(shared@100) 밖이다.
	setReq, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params":  map[string]any{"unit_id": 0, "address": 200, "value": 0x1234},
	})
	_, err := srv.Process(setReq)
	require.NoError(t, err)

	// 컨테이너 맵에 직접 반영되었는지 확인.
	got, err := container.ReadHoldingRegisters(200, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0x1234}, got)

	// get_holding_registers(unit_id=0, addr=200) 도 컨테이너에서 같은 값을 읽는다.
	getReq, _ := json.Marshal(map[string]any{
		"command": "get_holding_registers",
		"params":  map[string]any{"unit_id": 0, "address": 200, "quantity": 1},
	})
	resp, err := srv.Process(getReq)
	require.NoError(t, err)

	var decoded struct {
		OK     bool     `json:"ok"`
		Values []uint16 `json:"values"`
	}
	require.NoError(t, json.Unmarshal(resp, &decoded))
	assert.True(t, decoded.OK)
	assert.Equal(t, []uint16{0x1234}, decoded.Values)

	// 컨테이너 없는 서버에서 unit_id 0 대상 명령은 ErrNoSharedContainer 로 실패한다.
	bare := newNoContainerServer(t)
	_, err = bare.Process(getReq)
	assert.ErrorIs(t, err, ErrNoSharedContainer)
}
