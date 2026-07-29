package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/device"
)

// recordingProcessor 는 XSFMProcessor 를 구현하며 수신한 요청 바이트를 기록한다.
type recordingProcessor struct {
	lastRequest []byte
	response    []byte
	err         error
}

func (p *recordingProcessor) Process(data []byte) ([]byte, error) {
	cp := make([]byte, len(data))
	copy(cp, data)
	p.lastRequest = cp
	if p.err != nil {
		return nil, p.err
	}
	return p.response, nil
}

// ---------------------------------------------------------------------------
// Module 8.1 — CommandSpec: set_power(bool) + set_fan_speed(enum 1/2/3)
// ---------------------------------------------------------------------------

func TestXSFMCommandSpecs(t *testing.T) {
	t.Parallel()
	dev := NewControllableXSFMDevice("ap-1", XSFMDeviceInfo{DeviceID: "ap-101"}, nil)
	specs := dev.Commands()
	require.Len(t, specs, 2)

	byName := map[string]device.CommandSpec{}
	for _, s := range specs {
		byName[s.Name] = s
	}

	// set_power (bool)
	sp, ok := byName["set_power"]
	require.True(t, ok, "set_power 커맨드가 있어야 한다")
	require.Len(t, sp.Params, 1)
	assert.Equal(t, "power", sp.Params[0].Name)
	assert.Equal(t, "bool", sp.Params[0].Type)
	assert.True(t, sp.Params[0].Required)

	// set_fan_speed (enum 1/2/3)
	sf, ok := byName["set_fan_speed"]
	require.True(t, ok, "set_fan_speed 커맨드가 있어야 한다")
	require.Len(t, sf.Params, 1)
	assert.Equal(t, "fan_speed", sf.Params[0].Name)
	assert.Equal(t, "enum", sf.Params[0].Type)
	assert.True(t, sf.Params[0].Required)
	assert.Equal(t, []string{"1", "2", "3"}, sf.Params[0].Enum)
}

// 어댑터 인터페이스 준수 + Protocol 키("xsfm").
func TestXSFMAdapter_ProtocolKeyAndInterfaces(t *testing.T) {
	t.Parallel()
	power := true
	fan := 2
	info := XSFMDeviceInfo{
		DeviceID: "ap-101",
		Name:     "대합실-A",
		GroupID:  "concourse-b1",
		Online:   true,
		Power:    &power,
		FanSpeed: &fan,
		Source:   "bridge",
	}
	dev := NewControllableXSFMDevice("ap-agent", info, nil)

	// 어댑터 레지스트리 키.
	assert.Equal(t, "xsfm", dev.Protocol())

	// device.Device + device.ControllableDevice 준수 (컴파일 타임 체크는 파일 상단 var 블록).
	var _ device.Device = dev
	var _ device.ControllableDevice = dev

	assert.Equal(t, "대합실-A", dev.Name())
	assert.True(t, dev.Online())
	assert.Equal(t, device.DeviceTypeActuator, dev.Type())

	st := dev.State()
	assert.Equal(t, true, st.Properties["power"])
	assert.Equal(t, 2, st.Properties["fan_speed"])

	// executor 미설정 시 Execute 는 ErrNotControllable.
	_, err := dev.Execute(context.Background(), "set_power", map[string]any{"power": true})
	assert.ErrorIs(t, err, device.ErrNotControllable)
}

// power=off 이면 fan_speed 는 State() 에서 생략된다 (관측 게이팅 parity).
func TestXSFMAdapter_State_FanOmittedWhenOff(t *testing.T) {
	t.Parallel()
	power := false
	fan := 3
	dev := NewControllableXSFMDevice("ap-agent", XSFMDeviceInfo{
		DeviceID: "ap-101", Power: &power, FanSpeed: &fan,
	}, nil)
	st := dev.State()
	assert.Equal(t, false, st.Properties["power"])
	_, hasFan := st.Properties["fan_speed"]
	assert.False(t, hasFan, "power=off 이면 fan_speed 는 생략되어야 한다")
}

// ---------------------------------------------------------------------------
// Module 8.2 — executor 가 명령을 agent.Process 로 브리지
// ---------------------------------------------------------------------------

func TestXSFMExecutor_BridgesToProcess(t *testing.T) {
	t.Parallel()
	proc := &recordingProcessor{response: []byte(`{"status":"ok","device_id":"ap-101","command":"set_power"}`)}
	exec := NewXSFMExecutor(proc, "ap-101")

	result, err := exec(context.Background(), "set_power", map[string]any{"power": true})
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])

	// 에이전트가 받은 마샬 명령 검증: command / device_id / params.
	var req struct {
		Command  string         `json:"command"`
		DeviceID string         `json:"device_id"`
		Params   map[string]any `json:"params"`
	}
	require.NoError(t, json.Unmarshal(proc.lastRequest, &req))
	assert.Equal(t, "set_power", req.Command)
	assert.Equal(t, "ap-101", req.DeviceID)
	assert.Equal(t, true, req.Params["power"])
}

// 어댑터 접근자(ID/UID/AgentName/LastSeen/Metadata/Source/Capabilities) 커버리지.
func TestXSFMAdapter_Accessors(t *testing.T) {
	t.Parallel()
	seen := time.Unix(1700000000, 0)
	info := XSFMDeviceInfo{
		DeviceID: "ap-101",
		LastSeen: seen,
		Source:   "config",
	}
	dev := NewControllableXSFMDevice("ap-agent", info, nil)

	assert.Equal(t, "ap-agent", dev.AgentName())
	assert.Equal(t, seen, dev.LastSeen())
	assert.Equal(t, "config", dev.Source())
	assert.Equal(t, []string{"set_power", "set_fan_speed"}, dev.Capabilities())
	// ID/UID 는 DeviceIDRepository 미설정 시 graceful degradation(빈 문자열)이지만 호출은 수행.
	assert.Equal(t, dev.ID(), dev.UID())
	// Name 폴백: metadata/info.Name 없으면 DeviceID.
	assert.Equal(t, "ap-101", dev.Name())
	// Metadata 는 기본 zero 값.
	assert.Equal(t, device.DeviceMetadata{}, dev.Metadata())

	// Source 기본값(빈 값) 은 "auto".
	dev2 := NewXSFMDevice("ap-agent", XSFMDeviceInfo{DeviceID: "ap-202"})
	assert.Equal(t, "auto", dev2.Source())
}

// executor 오류는 그대로 전파된다.
func TestXSFMExecutor_ProcessError(t *testing.T) {
	t.Parallel()
	proc := &recordingProcessor{err: assert.AnError}
	exec := NewXSFMExecutor(proc, "ap-101")
	_, err := exec(context.Background(), "set_power", map[string]any{"power": true})
	assert.ErrorIs(t, err, assert.AnError)
}

// executor 가 없는 어댑터는 Execute 시 ErrNotControllable (non-controllable 생성자).
func TestXSFMAdapter_NonControllable(t *testing.T) {
	t.Parallel()
	dev := NewXSFMDevice("ap-agent", XSFMDeviceInfo{DeviceID: "ap-101"})
	assert.Nil(t, dev.Commands())
	_, err := dev.Execute(context.Background(), "set_power", nil)
	assert.ErrorIs(t, err, device.ErrNotControllable)
}
