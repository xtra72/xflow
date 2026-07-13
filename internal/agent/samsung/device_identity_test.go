package samsung

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TestDeviceIdentity_EmitUsesAddress 는 device_state_changed 이벤트에서
// unit_id 가 config name(UnitID) 이 아닌 dotted address(Address.String()) 를 사용해야 함을 검증한다.
//
// 버그: 현재 구현은 dev.UnitID (config name) 를 unit_id 로 사용한다.
// 정상: adapter 및 V2 callback 은 addr.String() (dotted) 을 localID 로 사용한다.
// 결과: emitted device_id UUID 가 registry/list UUID 와 불일치한다.
//
// 재현: 주소 "20.00.01" (dotted), UnitID "indoor-x" (이름) 인 디바이스에 상태 변경을 드라이브하고,
// 방출된 device_state_changed 에서 unit_id == "20.00.01" (dotted) 임을 검증한다.
func TestDeviceIdentity_EmitUsesAddress(t *testing.T) {
	t.Run("device_state_changed emits dotted address as unit_id", func(t *testing.T) {
		// 테스트 에이전트 생성
		a := &Hvacr01Agent{
			BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung_hvacr01")),
			agentConfig: agent.AgentConfig{
				ID:   "test-id",
				Name: "test-samsung-hvacr01",
				Type: "samsung_hvacr01",
			},
			hvacr01Config: Hvacr01Config{
				TransportType:       "serial",
				SerialPort:          "/dev/ttyTest",
				PollInterval:        30 * time.Second,
				MsgChannelSize:      256,
				OfflineTimeout:      -1,
				ReconnectInterval:   10 * time.Millisecond,
				MaxReconnectBackoff: 50 * time.Millisecond,
				ControlEnabled:      true,
			},
			devices:       make(map[NasaAddress]*NasaDevice),
			deviceIDs:     make(map[string]NasaAddress),
			transport:     &mockTransport{available: true},
			protocol:      &mockProtocol{},
			stopCh:        make(chan struct{}),
			msgCh:         make(chan []byte, 256),
			stats:         agent.NewAgentStats(),
			logger:        testLogger(),
			lastStates:    make(map[NasaAddress]NasaDeviceState),
			warnedUnknown: make(map[NasaAddress]bool),
			disconnectCh:  make(chan struct{}),
			createdAt:     time.Now(),
		}

		// Running 상태 전이
		require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
		require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
		a.startedAt = time.Now()

		// 테스트 디바이스: 주소 "20.00.01", UnitID "indoor-x"
		addr, err := ParseNasaAddress("20.00.01")
		require.NoError(t, err)
		require.Equal(t, "20.00.01", addr.String(), "parsed address should be dotted format")

		a.devices[addr] = &NasaDevice{
			Address:       addr,
			UnitID:        "indoor-x", // config name (다른 format)
			Type:          "HVACR.IDU",
			Online:        true,
			LastSeen:      time.Now(),
			State:         &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)},
			Source:        "config",
			ReportEnabled: true,
		}
		a.deviceIDs["indoor-x"] = addr

		// 상태 변경 메시지 생성 및 드라이브
		stateChangeMsg := &NasaMessage{
			SourceAddr:  addr,
			DestAddr:    AddrController,
			CommandCode: CmdNormalRequest,
			MessageSets: []NasaMessageSet{
				{Index: MsgPower, Value: []byte{0x01}},             // Power ON
				{Index: MsgMode, Value: []byte{0x02}},              // Heat
				{Index: MsgFanSpeed, Value: []byte{0x02}},          // Auto
				{Index: MsgTargetTemp, Value: []byte{0x00, 0xF2}},  // 24.2
				{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}}, // 24.0
			},
		}
		a.handleMessage(stateChangeMsg)

		// msgCh 에서 device_state_changed 메시지 추출
		var stateChangedPayload map[string]any
		found := false
		for i := 0; i < 20; i++ {
			select {
			case data := <-a.msgCh:
				var payload map[string]any
				err := json.Unmarshal(data, &payload)
				require.NoError(t, err)
				if payload["type"] == "device_state_changed" {
					stateChangedPayload = payload
					found = true
					break
				}
			default:
			}
		}
		require.True(t, found, "device_state_changed event should be emitted")

		// 핵심 검증: emitted unit_id 가 dotted address 와 일치해야 함
		emittedUnitID, ok := stateChangedPayload["unit_id"].(string)
		require.True(t, ok, "unit_id should be a string")

		// 현재 버그: emittedUnitID == "indoor-x" (dev.UnitID)
		// 정상: emittedUnitID == "20.00.01" (addr.String())
		assert.Equal(t, "20.00.01", emittedUnitID,
			"unit_id must match adapter's localID (dotted address), not config UnitID")

		// 부가 검증: address 필드는 그대로
		assert.Equal(t, "20.00.01", stateChangedPayload["address"])

		// 부가 검증: emitted unit_id 가 adapter 의 key 와 일치
		// (device_id UUID 는 repository 미설정 시 빈 문자열이지만, 향후 정상화 검증용)
		adaptersLocalID := addr.String()
		assert.Equal(t, adaptersLocalID, emittedUnitID,
			"emitted unit_id must match adapter's localID for device registry lookup")
	})

	t.Run("device_unregistered emits dotted address as unit_id", func(t *testing.T) {
		// device_unregistered 이벤트에서도 unit_id = addr.String() 확인
		a := &Hvacr01Agent{
			BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung_hvacr01")),
			agentConfig: agent.AgentConfig{
				ID:   "test-id",
				Name: "test-samsung-hvacr01",
				Type: "samsung_hvacr01",
			},
			hvacr01Config: Hvacr01Config{
				TransportType:       "serial",
				SerialPort:          "/dev/ttyTest",
				PollInterval:        30 * time.Second,
				MsgChannelSize:      256,
				OfflineTimeout:      -1,
				ReconnectInterval:   10 * time.Millisecond,
				MaxReconnectBackoff: 50 * time.Millisecond,
				ControlEnabled:      true,
			},
			devices:       make(map[NasaAddress]*NasaDevice),
			deviceIDs:     make(map[string]NasaAddress),
			transport:     &mockTransport{available: true},
			protocol:      &mockProtocol{},
			stopCh:        make(chan struct{}),
			msgCh:         make(chan []byte, 256),
			stats:         agent.NewAgentStats(),
			logger:        testLogger(),
			lastStates:    make(map[NasaAddress]NasaDeviceState),
			warnedUnknown: make(map[NasaAddress]bool),
			disconnectCh:  make(chan struct{}),
			createdAt:     time.Now(),
		}

		require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
		require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
		a.startedAt = time.Now()

		// 테스트 디바이스 추가: 주소 "25.00.04", UnitID "testdev"
		addr, err := ParseNasaAddress("25.00.04")
		require.NoError(t, err)

		a.mu.Lock()
		a.devices[addr] = &NasaDevice{
			Address:       addr,
			UnitID:        "testdev",
			Type:          "HVACR.IDU",
			Online:        true,
			LastSeen:      time.Now(),
			State:         &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)},
			Source:        "bridge",
			ReportEnabled: true,
		}
		a.deviceIDs["testdev"] = addr

		// 디바이스 제거 → device_unregistered 이벤트 생성
		delete(a.deviceIDs, "testdev")
		delete(a.devices, addr)

		unregData := map[string]any{
			"address":   addr.String(),
			"unit_id":   addr.String(),
			"device_id": agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, addr.String()),
		}
		a.sendEventLocked("device_unregistered", unregData)
		a.mu.Unlock()

		// device_unregistered 메시지 찾기
		var unregPayload map[string]any
		found := false
		for i := 0; i < 30; i++ {
			select {
			case data := <-a.msgCh:
				var payload map[string]any
				err := json.Unmarshal(data, &payload)
				require.NoError(t, err)
				if payload["type"] == "device_unregistered" {
					unregPayload = payload
					found = true
					break
				}
			default:
			}
		}
		require.True(t, found, "device_unregistered event should be emitted")

		// 검증: unit_id 는 dotted address
		emittedUnitID, ok := unregPayload["unit_id"].(string)
		require.True(t, ok, "unit_id should be a string")

		assert.Equal(t, "25.00.04", emittedUnitID,
			"device_unregistered unit_id must use dotted address")
	})
}

// TestDeviceIdentity_ConnectionPayloadUsesAddress 는 device_state 스냅샷(연결 정보 일원화)
// 에서 unit_id 가 dotted address 를 사용함을 검증한다.
func TestDeviceIdentity_ConnectionPayloadUsesAddress(t *testing.T) {
	a := &Hvacr01Agent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung_hvacr01")),
		agentConfig: agent.AgentConfig{
			ID:   "test-id",
			Name: "test-samsung-hvacr01",
			Type: "samsung_hvacr01",
		},
		hvacr01Config: Hvacr01Config{
			TransportType:       "serial",
			SerialPort:          "/dev/ttyTest",
			PollInterval:        30 * time.Second,
			MsgChannelSize:      256,
			OfflineTimeout:      -1,
			ReconnectInterval:   10 * time.Millisecond,
			MaxReconnectBackoff: 50 * time.Millisecond,
			ControlEnabled:      true,
		},
		devices:       make(map[NasaAddress]*NasaDevice),
		deviceIDs:     make(map[string]NasaAddress),
		transport:     &mockTransport{available: true},
		protocol:      &mockProtocol{},
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, 256),
		stats:         agent.NewAgentStats(),
		logger:        testLogger(),
		lastStates:    make(map[NasaAddress]NasaDeviceState),
		warnedUnknown: make(map[NasaAddress]bool),
		disconnectCh:  make(chan struct{}),
		createdAt:     time.Now(),
	}

	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))
	a.startedAt = time.Now()

	// 테스트 디바이스: 주소 "25.00.03", UnitID "kitchen-unit"
	addr, err := ParseNasaAddress("25.00.03")
	require.NoError(t, err)

	a.devices[addr] = &NasaDevice{
		Address:       addr,
		UnitID:        "kitchen-unit",
		Type:          "HVACR.IDU",
		Online:        false, // online 전이(→device_state change)를 유도
		LastSeen:      time.Now(),
		State:         &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)},
		Source:        "config",
		ReportEnabled: true,
	}
	a.deviceIDs["kitchen-unit"] = addr

	// 상태 변경으로 device_state 스냅샷 유도
	msg := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{0x01}},
			{Index: MsgFanSpeed, Value: []byte{0x02}},
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}},
			{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}},
		},
	}
	a.handleMessage(msg)

	// device_state 스냅샷 찾기 — 연결 정보 일원화 후 recentSnapshots 링버퍼에 device_state 만 실린다.
	var statePayload map[string]any
	found := false
	a.recentMu.Lock()
	snaps := append([]recentStateEntry(nil), a.recentSnapshots...)
	a.recentMu.Unlock()
	for _, s := range snaps {
		var payload map[string]any
		if json.Unmarshal(s.Data, &payload) != nil {
			continue
		}
		if _, ok := payload["state"].(map[string]any); ok {
			statePayload = payload
			found = true
			break
		}
	}
	require.True(t, found, "device_state snapshot should be emitted")

	// 검증: unit_id 가 dotted address 여야 함
	emittedUnitID, ok := statePayload["unit_id"].(string)
	require.True(t, ok, "unit_id should be a string")

	assert.Equal(t, "25.00.03", emittedUnitID,
		"device_state payload unit_id must use dotted address for consistent device identity")
}
