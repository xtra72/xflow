package modbusserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// intra-server 공유 레지스터 맵 + 주소 변환 테스트 (B1/B2)
// ---------------------------------------------------------------------------

// multiDeviceAgentConfig 는 devices 배열로 구성된 modbus-server 설정을 만든다.
func multiDeviceAgentConfig(id string, devices []any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   id,
		Name: id,
		Type: "modbus-server",
		Transport: agent.TransportConfig{
			Type: "modbus-server",
			Options: map[string]any{
				"listen_address": "127.0.0.1",
				"listen_port":    0,
				"devices":        devices,
			},
		},
	}
}

// seg 는 로컬/공유 세그먼트 맵을 만드는 헬퍼이다. shared<0 이면 로컬.
func hseg(address, count, shared int) map[string]any {
	m := map[string]any{"address": address, "count": count}
	if shared >= 0 {
		m["shared_address"] = shared
	}
	return m
}

// sharedTopology 는 표준 테스트 토폴로지를 반환한다:
//   - unit_id 0 컨테이너: holding[0..1000)
//   - unit_id 1: holding[0..50)→shared@100 (공유) + coils[0..16) (로컬)
//   - unit_id 2: holding[0..50)→shared@100 (dev1 과 동일 공유 영역)
func sharedTopology() []any {
	return []any{
		map[string]any{
			"unit_id": 0, "name": "shared",
			"register_map": map[string]any{
				"holding_registers": []any{hseg(0, 1000, -1)},
			},
		},
		map[string]any{
			"unit_id": 1, "name": "dev1",
			"register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, 100)},
				"coils":             []any{map[string]any{"address": 0, "count": 16}},
			},
		},
		map[string]any{
			"unit_id": 2, "name": "dev2",
			"register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, 100)},
			},
		},
	}
}

func newSharedServer(t *testing.T) *ModbusServerAgent {
	t.Helper()
	a, err := NewModbusServerAgent(multiDeviceAgentConfig("shared-srv", sharedTopology()), nil)
	require.NoError(t, err)
	return a.(*ModbusServerAgent)
}

// (a) 파싱: 컨테이너 + 공유 세그먼트 디바이스가 정상 구성된다.
func TestSharedMap_ParseAndConstruct(t *testing.T) {
	srv := newSharedServer(t)

	// 컨테이너는 서빙 집합에서 제외된다.
	assert.Nil(t, srv.deviceManager.GetDevice(0), "unit 0 는 서빙 집합에 없어야 함")
	require.NotNil(t, srv.deviceManager.SharedContainer(), "컨테이너는 SharedContainer 로 접근")
	assert.Equal(t, 2, srv.deviceManager.DeviceCount(), "서빙 디바이스는 2개(unit1,2)")
}

// (b) 서빙: 공유 세그먼트 읽기/쓰기가 컨테이너로 변환되고 다른 디바이스에 전파된다.
func TestSharedMap_TranslationAndPropagation(t *testing.T) {
	srv := newSharedServer(t)
	container := srv.deviceManager.SharedContainer().RegisterMap
	s1 := srv.deviceManager.GetDevice(1).ReqHandler.store
	s2 := srv.deviceManager.GetDevice(2).ReqHandler.store

	// dev1 의 디바이스 주소 5 쓰기 → 컨테이너 주소 105 로 변환.
	_, err := s1.WriteHoldingRegisters(5, []uint16{0x1234})
	require.NoError(t, err)

	got, err := container.ReadHoldingRegisters(105, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0x1234}, got, "dev1 쓰기가 컨테이너 105 에 반영")

	// dev2 는 같은 공유 영역이므로 디바이스 주소 5 에서 동일 값을 읽는다.
	got2, err := s2.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0x1234}, got2, "dev2 가 dev1 쓰기를 즉시 관측(라이브 공유)")

	// 컨테이너에 직접 쓰면 dev1/dev2 모두 관측한다.
	_, err = container.WriteHoldingRegisters(110, []uint16{0xBEEF})
	require.NoError(t, err)
	g1, _ := s1.ReadHoldingRegisters(10, 1) // 100+10 = 110
	g2, _ := s2.ReadHoldingRegisters(10, 1)
	assert.Equal(t, []uint16{0xBEEF}, g1)
	assert.Equal(t, []uint16{0xBEEF}, g2)

	// 로컬 세그먼트(dev1 coils)는 컨테이너와 무관하게 자체 맵을 쓴다.
	_, err = s1.WriteCoils(0, []bool{true})
	require.NoError(t, err)
	c1, err := s1.ReadCoils(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, c1)

	// 세그먼트 경계 횡단 요청은 illegal address 로 거부된다(조용히 분할 안 함).
	_, err = s1.ReadHoldingRegisters(49, 2) // [49,51) 은 [0,50) 를 벗어남
	assert.ErrorIs(t, err, ErrAddressNotMapped)
}

// (c) 검증 에러들.
func TestSharedMap_ValidationErrors(t *testing.T) {
	t.Run("shared_without_container", func(t *testing.T) {
		_, err := parseModbusServerConfig(map[string]any{"devices": []any{
			map[string]any{"unit_id": 1, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, 100)},
			}},
		}})
		assert.ErrorIs(t, err, ErrSharedMapMissing)
	})

	t.Run("shared_range_out_of_bounds", func(t *testing.T) {
		_, err := parseModbusServerConfig(map[string]any{"devices": []any{
			map[string]any{"unit_id": 0, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 100, -1)},
			}},
			map[string]any{"unit_id": 1, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, 90)}, // [90,140) exceeds [0,100)
			}},
		}})
		assert.ErrorIs(t, err, ErrSharedRangeOutOfBounds)
	})

	t.Run("overlapping_segments", func(t *testing.T) {
		_, err := parseModbusServerConfig(map[string]any{"devices": []any{
			map[string]any{"unit_id": 1, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, -1), hseg(40, 20, -1)}, // [0,50)&[40,60)
			}},
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overlap")
	})

	t.Run("shared_under_container", func(t *testing.T) {
		_, err := parseModbusServerConfig(map[string]any{"devices": []any{
			map[string]any{"unit_id": 0, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 50, 10)}, // shared_address under unit 0
			}},
			map[string]any{"unit_id": 1, "register_map": map[string]any{
				"holding_registers": []any{hseg(0, 10, -1)},
			}},
		}})
		assert.ErrorIs(t, err, ErrSharedUnderContainer)
	})
}

// (d) unit_id 0 은 와이어 서빙 집합에서 제외된다.
func TestSharedMap_ContainerNotServed(t *testing.T) {
	srv := newSharedServer(t)
	assert.Nil(t, srv.deviceManager.GetDevice(0))
	for _, dev := range srv.deviceManager.GetAllDevices() {
		assert.NotEqual(t, byte(0), dev.UnitID, "GetAllDevices 는 컨테이너를 제외")
	}
	require.NotNil(t, srv.deviceManager.SharedContainer())
}

// (e) cross-agent: 컨테이너를 가진 main 을 상속한 sub 가 동일 라이브 컨테이너로 변환한다.
func TestSharedMap_SubAdoptsContainer(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	mainAgent, err := mgr.Create(multiDeviceAgentConfig("shared-main", sharedTopology()))
	require.NoError(t, err)
	mainSrv := mainAgent.(*ModbusServerAgent)

	subAgent, err := mgr.Create(serverConfigSubNoDevices("shared-sub", "shared-main"))
	require.NoError(t, err)
	subSrv := subAgent.(*ModbusServerAgent)

	require.NoError(t, subSrv.Start(context.Background()))
	t.Cleanup(func() { _ = subSrv.Stop(context.Background()) })

	// 서브는 main 의 컨테이너를 동일 포인터로 상속한다.
	require.NotNil(t, subSrv.deviceManager.SharedContainer())
	require.Same(t,
		mainSrv.deviceManager.SharedContainer().RegisterMap,
		subSrv.deviceManager.SharedContainer().RegisterMap,
		"sub 는 main 의 컨테이너 맵을 라이브 공유")

	// 서브 dev1 의 공유 세그먼트 쓰기가 main 의 컨테이너 + main dev1 관측으로 전파된다.
	subS1 := subSrv.deviceManager.GetDevice(1).ReqHandler.store
	_, err = subS1.WriteHoldingRegisters(7, []uint16{0xABCD}) // → 컨테이너 107
	require.NoError(t, err)

	mainContainer := mainSrv.deviceManager.SharedContainer().RegisterMap
	gc, err := mainContainer.ReadHoldingRegisters(107, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0xABCD}, gc, "sub 쓰기가 main 컨테이너에 반영")

	mainS1 := mainSrv.deviceManager.GetDevice(1).ReqHandler.store
	gm, err := mainS1.ReadHoldingRegisters(7, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0xABCD}, gm, "main dev1 이 sub 쓰기를 관측(동일 라이브 컨테이너)")
}
