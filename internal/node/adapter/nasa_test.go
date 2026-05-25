package adapter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// --- Validate 테스트 ---

// TestNASAAdapter_Validate 는 Validate가 항상 성공하는지 확인한다.
func TestNASAAdapter_Validate(t *testing.T) {
	a := NewNASAAdapter()
	err := a.Validate(node.BridgeConfig{})
	assert.NoError(t, err)
}

// --- TransformToFlow 테스트 ---

// TestNASAAdapter_TransformToFlow_JSON 은 유효한 JSON 데이터 변환을 확인한다.
func TestNASAAdapter_TransformToFlow_JSON(t *testing.T) {
	a := NewNASAAdapter()

	data := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"address":     "200000",
				"device_id":   "living-room",
				"device_type": "HVACR.IDU",
				"online":      true,
			},
		},
	}
	raw, err := json.Marshal(data)
	require.NoError(t, err)

	msg, err := a.TransformToFlow(raw, node.AgentMeta{})
	require.NoError(t, err)

	// 페이로드에 status와 devices가 존재하는지 확인
	status, ok := msg.Payload().Get("status")
	assert.True(t, ok)
	assert.Equal(t, "ok", status)

	devices, ok := msg.Payload().Get("devices")
	assert.True(t, ok)
	assert.NotNil(t, devices)

	// 메타데이터 확인
	source, ok := msg.Metadata().Get("nasa.source")
	assert.True(t, ok)
	assert.Equal(t, "event", source)
}

// TestNASAAdapter_TransformToFlow_RawData 는 비-JSON 데이터를 raw 필드로 저장하는지 확인한다.
func TestNASAAdapter_TransformToFlow_RawData(t *testing.T) {
	a := NewNASAAdapter()

	rawData := []byte("this is not json")
	msg, err := a.TransformToFlow(rawData, node.AgentMeta{})
	require.NoError(t, err)

	// raw 필드에 원본 데이터가 저장되는지 확인
	raw, ok := msg.Payload().Get("raw")
	assert.True(t, ok)
	assert.Equal(t, "this is not json", raw)

	// 메타데이터에 raw 포맷 표시 확인
	format, ok := msg.Metadata().Get("nasa.format")
	assert.True(t, ok)
	assert.Equal(t, "raw", format)

	source, ok := msg.Metadata().Get("nasa.source")
	assert.True(t, ok)
	assert.Equal(t, "event", source)
}

// --- TransformToAgent 테스트 ---

// TestNASAAdapter_TransformToAgent 는 플로우 메시지를 에이전트 명령으로 올바르게 변환하는지 확인한다.
func TestNASAAdapter_TransformToAgent(t *testing.T) {
	a := NewNASAAdapter()

	msg := message.New()
	msg.Payload().Set("command", "set_state")
	msg.Payload().Set("device_id", "living-room")
	msg.Payload().Set("power", "on")

	data, meta, err := a.TransformToAgent(msg)
	require.NoError(t, err)

	// AgentType 확인
	assert.Equal(t, "samsung-nasa", meta.AgentType)

	// JSON 직렬화 확인
	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "set_state", parsed["command"])
	assert.Equal(t, "living-room", parsed["device_id"])
	assert.Equal(t, "on", parsed["power"])
}

// --- PollCommand 테스트 ---

// TestNASAAdapter_PollCommand 는 get_all_states 명령이 올바르게 생성되는지 확인한다.
func TestNASAAdapter_PollCommand(t *testing.T) {
	a := NewNASAAdapter()

	cmd := a.PollCommand()
	require.NotNil(t, cmd)

	var parsed map[string]any
	err := json.Unmarshal(cmd, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "get_all_states", parsed["command"])
}

// --- AssemblePollMessage 테스트 ---

// TestNASAAdapter_AssemblePollMessage 는 폴링 응답을 올바르게 메시지로 변환하는지 확인한다.
func TestNASAAdapter_AssemblePollMessage(t *testing.T) {
	a := NewNASAAdapter()

	response := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"address":     "200000",
				"device_id":   "living-room",
				"device_type": "HVACR.IDU",
				"online":      true,
				"state": map[string]any{
					"power":       "on",
					"temperature": 24.5,
				},
			},
		},
	}
	raw, err := json.Marshal(response)
	require.NoError(t, err)

	msg, err := a.AssemblePollMessage(raw)
	require.NoError(t, err)

	// 페이로드 확인
	status, ok := msg.Payload().Get("status")
	assert.True(t, ok)
	assert.Equal(t, "ok", status)

	devices, ok := msg.Payload().Get("devices")
	assert.True(t, ok)
	assert.NotNil(t, devices)

	// 메타데이터: poll 소스 확인
	source, ok := msg.Metadata().Get("nasa.source")
	assert.True(t, ok)
	assert.Equal(t, "poll", source)
}

// TestNASAAdapter_AssemblePollMessage_InvalidJSON 은 잘못된 JSON 응답에 대한 에러 처리를 확인한다.
func TestNASAAdapter_AssemblePollMessage_InvalidJSON(t *testing.T) {
	a := NewNASAAdapter()

	_, err := a.AssemblePollMessage([]byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nasa adapter: unmarshal poll response")
}

// --- AssemblePollMessages 테스트 ---

// TestNASAAdapter_AssemblePollMessages_MultiDevice 는 다중 디바이스 응답을 개별 메시지로 분리하는지 확인한다.
func TestNASAAdapter_AssemblePollMessages_MultiDevice(t *testing.T) {
	a := NewNASAAdapter()

	response := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"address":     "200000",
				"device_id":   "living-room",
				"device_type": "HVACR.IDU",
				"online":      true,
				"state": map[string]any{
					"Power":       true,
					"Mode":        "cool",
					"TargetTemp":  24.0,
					"CurrentTemp": 25.5,
					"FanSpeed":    "auto",
				},
			},
			map[string]any{
				"address":     "200100",
				"device_id":   "bedroom",
				"device_type": "HVACR.IDU",
				"online":      true,
				"state": map[string]any{
					"Power":       false,
					"Mode":        "heat",
					"TargetTemp":  22.0,
					"CurrentTemp": 20.0,
					"FanSpeed":    "low",
				},
			},
		},
	}
	raw, err := json.Marshal(response)
	require.NoError(t, err)

	msgs, err := a.AssemblePollMessages(raw)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)

	// 각 메시지가 state-formatter 표현식과 호환되는 구조인지 확인:
	// $.payload.device_id, $.payload.state.Power 등
	for _, msg := range msgs {
		deviceID, ok := msg.Payload().Get("device_id")
		assert.True(t, ok, "device_id 필드가 있어야 한다")
		assert.NotNil(t, deviceID)

		stateVal, ok := msg.Payload().Get("state")
		assert.True(t, ok, "state 필드가 있어야 한다")
		stateMap, ok := stateVal.(map[string]any)
		assert.True(t, ok, "state는 map[string]any여야 한다")
		assert.NotNil(t, stateMap["Power"], "state.Power 필드가 있어야 한다")

		source, ok := msg.Metadata().Get("nasa.source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	}

	// 첫 번째 디바이스 확인
	id0, _ := msgs[0].Payload().Get("device_id")
	id1, _ := msgs[1].Payload().Get("device_id")
	ids := []string{id0.(string), id1.(string)}
	assert.ElementsMatch(t, []string{"living-room", "bedroom"}, ids)
}

// TestNASAAdapter_AssemblePollMessages_SingleDevice 는 단일 디바이스 응답도 올바르게 처리되는지 확인한다.
func TestNASAAdapter_AssemblePollMessages_SingleDevice(t *testing.T) {
	a := NewNASAAdapter()

	response := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"device_id": "living-room",
				"state": map[string]any{
					"Power": true,
				},
			},
		},
	}
	raw, err := json.Marshal(response)
	require.NoError(t, err)

	msgs, err := a.AssemblePollMessages(raw)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	id, ok := msgs[0].Payload().Get("device_id")
	assert.True(t, ok)
	assert.Equal(t, "living-room", id)
}

// TestNASAAdapter_AssemblePollMessages_EmptyDevices 는 devices 배열이 빈 경우 폴백을 확인한다.
func TestNASAAdapter_AssemblePollMessages_EmptyDevices(t *testing.T) {
	a := NewNASAAdapter()

	response := map[string]any{
		"status":  "ok",
		"devices": []any{},
	}
	raw, err := json.Marshal(response)
	require.NoError(t, err)

	msgs, err := a.AssemblePollMessages(raw)
	require.NoError(t, err)
	// 빈 devices → AssemblePollMessage 폴백 → 전체 응답이 단일 메시지로
	assert.Len(t, msgs, 1)
}

// --- DefaultConfig 테스트 ---

// TestNASAAdapter_DefaultConfig 는 기본 설정이 빈 BridgeConfig를 반환하는지 확인한다.
func TestNASAAdapter_DefaultConfig(t *testing.T) {
	a := NewNASAAdapter()
	config := a.DefaultConfig()
	assert.Equal(t, node.BridgeConfig{}, config)
}

// --- HandleControl 테스트 ---

// TestNASAAdapter_HandleControl 은 제어 메시지 처리가 에러 없이 완료되는지 확인한다.
func TestNASAAdapter_HandleControl(t *testing.T) {
	a := NewNASAAdapter()
	msg := message.New()
	err := a.HandleControl(msg)
	assert.NoError(t, err)
}
