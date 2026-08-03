package modbusserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// buildWriteSingleRegPDU 는 FC06(단일 보유 레지스터 쓰기) 요청 PDU 를 만든다.
func buildWriteSingleRegPDU(addr, value uint16) []byte {
	return []byte{
		modbus.FC06WriteSingleRegister,
		byte(addr >> 8), byte(addr),
		byte(value >> 8), byte(value),
	}
}

// (a) notify_on_write=true → 와이어 쓰기(원격 마스터)가 register_change 를 msgCh 로 발행한다.
func TestNotifyOnWrite_Enabled_WirePushesNotification(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 4)
	handler := NewModbusHandler(dm, msgCh, true, nil)

	dev := dm.GetDevice(1)
	require.NotNil(t, dev)

	// 와이어 쓰기 경로(HandleWriteRequest → handler.sendChangeNotification) 실행.
	pdu := buildWriteSingleRegPDU(0, 0x1234)
	handler.handleDeviceRequest(dev, pdu, modbus.FC06WriteSingleRegister, "10.0.0.9:502")

	select {
	case n := <-msgCh:
		assert.Equal(t, "register_change", n["type"])
		assert.Equal(t, "holding_registers", n["area"])
		assert.Equal(t, "10.0.0.9:502", n["source"])
		assert.Equal(t, byte(1), n["unit_id"])
	default:
		t.Fatal("expected register_change notification when notify_on_write=true")
	}
}

// (b) notify_on_write=false(기본) → 와이어 쓰기는 레지스터를 갱신하지만 알림은 발행하지 않는다.
func TestNotifyOnWrite_Disabled_WirePushesNothing(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 4)
	handler := NewModbusHandler(dm, msgCh, false, nil)

	dev := dm.GetDevice(1)
	require.NotNil(t, dev)

	pdu := buildWriteSingleRegPDU(0, 0x1234)
	handler.handleDeviceRequest(dev, pdu, modbus.FC06WriteSingleRegister, "10.0.0.9:502")

	// 레지스터는 정상 갱신되어야 한다(게이트는 알림에만 적용, 쓰기 자체는 그대로).
	vals, err := rm.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), vals[0])

	// 알림은 없어야 한다.
	select {
	case n := <-msgCh:
		t.Fatalf("expected no notification when notify_on_write=false, got %v", n)
	default:
	}
}

// (c) 플로우 입력 경로(set_* Process → sendChangeEvent)는 notify_on_write=false 여도 영향받지 않는다.
//
//	notify_on_write 게이트는 와이어 경로(handler)에만 적용되며 내부 이벤트 경로는 그대로 동작한다.
func TestNotifyOnWrite_Disabled_FlowInputEventUnaffected(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Transport.Options["notify_on_write"] = false // 명시적으로 비활성(기본과 동일)

	a, err := NewModbusServerAgent(cfg, nil)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 소비자 활성화(ReceiveMessage 이전에 sendChangeEvent 가 이벤트를 생성하도록).
	msa.activateReceiver()

	// 플로우 입력 포트 경유의 set_register(보유 레지스터 쓰기) 명령.
	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address": 0,
			"value":   0x1234,
		},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := msa.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.NotNil(t, msg)

	var notification map[string]any
	require.NoError(t, json.Unmarshal(msg, &notification))
	// 플로우 입력 경로는 register_updated / source=internal 이며 notify_on_write 와 무관하게 발행된다.
	assert.Equal(t, "register_updated", notification["type"])
	assert.Equal(t, "internal", notification["source"])
	assert.Equal(t, "set_register", notification["command"])
	assert.Equal(t, "holding_registers", notification["area"])
}
