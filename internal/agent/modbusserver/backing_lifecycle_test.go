package modbusserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// M5 — 런타임/수명 배선 (REQ-MODBUS-010-02/04/05)
// ---------------------------------------------------------------------------

// backedAgentConfig 는 백킹 디바이스(unit_id 1) + 순수 slave 디바이스(unit_id 2)를 가진
// modbus-gateway 설정을 만든다. remove_device 는 마지막 디바이스 제거를 막으므로 백킹
// 디바이스 제거 테스트를 위해 두 번째(순수) 디바이스를 함께 둔다.
func backedAgentConfig(mode string, pollInterval string) agent.AgentConfig {
	backing := map[string]any{
		"transport": "tcp",
		"host":      "127.0.0.1",
		"port":      15020,
		"unit_id":   5,
		"mode":      mode,
		"timeout":   "200ms",
	}
	if mode == BackingModeIndirect {
		backing["poll_interval"] = pollInterval
	}
	return agent.AgentConfig{
		ID:   "test-backed",
		Name: "Backed Gateway",
		Type: "modbus-gateway",
		Transport: agent.TransportConfig{
			Type: "modbus-gateway",
			Options: map[string]any{
				"listen_address":   "127.0.0.1",
				"listen_port":      0,
				"max_connections":  5,
				"idle_timeout":     "30s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"unit_id": 1,
						"name":    "backed",
						"register_map": map[string]any{
							"holding_registers": map[string]any{"start_address": 0, "count": 10},
						},
						"backing": backing,
					},
					map[string]any{
						"unit_id": 2,
						"name":    "plain",
						"register_map": map[string]any{
							"holding_registers": map[string]any{"start_address": 0, "count": 10},
						},
					},
				},
			},
		},
	}
}

// installFakeTransport 는 에이전트의 트랜스포트 생성 seam 에 fake 를 주입하고 그 핸들을
// 반환한다. 모든 백킹 디바이스가 동일 fake 를 공유하도록 단일 인스턴스를 반환한다.
func installFakeTransport(msa *ModbusServerAgent, resp func(pdu []byte) ([]byte, error)) *fakeTransport {
	fake := &fakeTransport{resp: resp}
	msa.newTransport = func(cfg *BackingConfig, logger *slog.Logger) (modbus.ModbusTransport, error) {
		return fake, nil
	}
	return fake
}

// AC-03 — Start 시 upstream Connect 수행 + 백킹 등록 + 서빙 준비.
func TestAgent_Start_ConnectsBackingAndServes(t *testing.T) {
	cfg := backedAgentConfig(BackingModeDirect, "")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	fake := installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{321}), nil
	})

	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	// upstream 에 Connect 가 수행되었다.
	assert.Equal(t, 1, fake.connectCalls, "Start 는 백킹 트랜스포트를 Connect 해야 한다")

	// 백킹 상태가 a.mu 보호 하에 등록되었다.
	msa.mu.RLock()
	_, registered := msa.backings[1]
	msa.mu.RUnlock()
	assert.True(t, registered, "백킹 디바이스가 backings 에 등록되어야 한다")

	// 디바이스가 서빙 준비 상태다: direct 읽기가 upstream 을 조회하여 값을 반환한다.
	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)
	resp := dev.ReqHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1))
	require.NotNil(t, resp)
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, resp[0])
	assert.Equal(t, []byte{0x01, 0x41}, resp[2:4], "direct 읽기가 upstream 조회값(321)을 서빙해야 한다")

	// 순수 slave 디바이스(unit_id 2)는 백킹되지 않았다.
	msa.mu.RLock()
	_, plainBacked := msa.backings[2]
	msa.mu.RUnlock()
	assert.False(t, plainBacked, "순수 slave 디바이스는 백킹되지 않아야 한다")
}

// AC-07(agent) — indirect 폴러가 Start 후 주기적으로 RegisterMap 을 갱신한다.
func TestAgent_Start_IndirectPollerUpdatesRegisterMap(t *testing.T) {
	cfg := backedAgentConfig(BackingModeIndirect, "10ms")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{7, 7, 7, 7, 7, 7, 7, 7, 7, 7}), nil
	})

	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)

	// 폴러가 poll_interval 경과 후 RegisterMap 을 갱신할 때까지 대기한다.
	require.Eventually(t, func() bool {
		v, err := dev.RegisterMap.ReadHoldingRegisters(0, 1)
		return err == nil && len(v) == 1 && v[0] == 7
	}, time.Second, 5*time.Millisecond, "indirect 폴러가 RegisterMap 을 갱신해야 한다")

	// 갱신 후 indirect 읽기는 저장값을 서빙한다(upstream 직접 호출 없이).
	resp := dev.ReqHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1))
	require.NotNil(t, resp)
	assert.Equal(t, []byte{0x00, 0x07}, resp[2:4])
}

// AC-11 — remove_device 시 폴러 종료 + 트랜스포트 Close + goroutine 누수 없음.
func TestAgent_RemoveDevice_TearsDownBacking(t *testing.T) {
	cfg := backedAgentConfig(BackingModeIndirect, "10ms")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	fake := installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}), nil
	})

	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	// 백킹의 폴러 핸들을 확보한다(종료 관측용).
	msa.mu.RLock()
	backing := msa.backings[1]
	msa.mu.RUnlock()
	require.NotNil(t, backing)
	require.NotNil(t, backing.poller)
	poller := backing.poller

	// remove_device(unit_id 1).
	data, _ := json.Marshal(map[string]any{
		"command": "remove_device",
		"params":  map[string]any{"unit_id": 1},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	// 트랜스포트가 Close 되었다.
	assert.Equal(t, 1, fake.closeCalls, "remove_device 는 백킹 트랜스포트를 Close 해야 한다")

	// 폴러 goroutine 이 종료되었다(done 채널 close 관측).
	select {
	case <-poller.done:
		// 정상 종료.
	case <-time.After(time.Second):
		t.Fatal("remove_device 후 폴러 goroutine 이 종료되지 않았다(누수)")
	}

	// 백킹 등록이 해제되었다.
	msa.mu.RLock()
	_, stillRegistered := msa.backings[1]
	msa.mu.RUnlock()
	assert.False(t, stillRegistered, "제거된 디바이스의 백킹 등록이 해제되어야 한다")

	// 순수 slave 디바이스(unit_id 2)는 그대로 서빙된다(등록/서빙 보존).
	assert.NotNil(t, msa.deviceManager.GetDevice(2))
}

// AC-11 — Stop 시 모든 폴러 종료 + 트랜스포트 Close + goroutine 누수 없음.
func TestAgent_Stop_TearsDownAllBackings_NoLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()

	cfg := backedAgentConfig(BackingModeIndirect, "10ms")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	fake := installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{2, 2, 2, 2, 2, 2, 2, 2, 2, 2}), nil
	})

	require.NoError(t, msa.Start(context.Background()))

	msa.mu.RLock()
	poller := msa.backings[1].poller
	msa.mu.RUnlock()
	require.NotNil(t, poller)

	require.NoError(t, msa.Stop(context.Background()))

	// 트랜스포트 Close + 폴러 종료.
	assert.GreaterOrEqual(t, fake.closeCalls, 1, "Stop 은 백킹 트랜스포트를 Close 해야 한다")
	select {
	case <-poller.done:
	case <-time.After(time.Second):
		t.Fatal("Stop 후 폴러 goroutine 이 종료되지 않았다(누수)")
	}

	// backings 맵이 비워졌다.
	msa.mu.RLock()
	n := len(msa.backings)
	msa.mu.RUnlock()
	assert.Equal(t, 0, n, "Stop 후 backings 맵이 비워져야 한다")

	// 전체 goroutine 수가 baseline 으로 회귀한다(누수 없음).
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+2
	}, 2*time.Second, 10*time.Millisecond, "Stop 후 goroutine 이 baseline 으로 회귀해야 한다(누수 없음)")
}

// AC-10(agent) — indirect 쓰기는 폴 캐시를 우회하여 upstream 에 즉시 전달된다.
func TestAgent_IndirectWrite_ForwardsImmediately(t *testing.T) {
	cfg := backedAgentConfig(BackingModeIndirect, "1h") // 폴이 실질적으로 발생하지 않게 긴 주기
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	var mu sync.Mutex
	var lastWritePDU []byte
	fake := installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		if pdu[0] == modbus.FC06WriteSingleRegister {
			mu.Lock()
			lastWritePDU = append([]byte(nil), pdu...)
			mu.Unlock()
			return writeEchoResp(pdu), nil
		}
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{0}), nil
	})

	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)

	// 폴 이력이 없어도 쓰기는 즉시 upstream 으로 전달되어야 한다(timeout/stale 무관).
	resp, cs := dev.ReqHandler.HandleWriteRequest(writeSingleRegisterPDU(3, 4242), "")
	require.NotNil(t, resp)
	require.NotNil(t, cs, "indirect 쓰기 성공은 ChangeSet 을 반환해야 한다")

	mu.Lock()
	got := lastWritePDU
	mu.Unlock()
	require.NotNil(t, got, "indirect 쓰기가 upstream 으로 전달되어야 한다")
	assert.Equal(t, modbus.FC06WriteSingleRegister, got[0])
	assert.Equal(t, byte(5), fake.lastUnitID, "쓰기는 backing unit_id(5)로 전달되어야 한다")

	// upstream 성공 시 RegisterMap 미러.
	v, err := dev.RegisterMap.ReadHoldingRegisters(3, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{4242}, v)
}

// AC-13 — 폴러 갱신과 마스터 읽기/쓰기 동시 접근 race 클린(에이전트 레벨).
func TestAgent_ConcurrentPollAndServe_RaceClean(t *testing.T) {
	cfg := backedAgentConfig(BackingModeIndirect, "1ms")
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	installFakeTransport(msa, func(pdu []byte) ([]byte, error) {
		switch pdu[0] {
		case modbus.FC06WriteSingleRegister:
			return writeEchoResp(pdu), nil
		default:
			return regReadResp(modbus.FC03ReadHoldingRegisters,
				[]uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), nil
		}
	})

	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	dev := msa.deviceManager.GetDevice(1)
	require.NotNil(t, dev)

	// 폴러가 백그라운드로 도는 동안 마스터 읽기/쓰기를 동시에 수행한다.
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = dev.ReqHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 4))
		}()
		go func() {
			defer wg.Done()
			_, _ = dev.ReqHandler.HandleWriteRequest(writeSingleRegisterPDU(0, 99), "")
		}()
	}
	wg.Wait()
}
