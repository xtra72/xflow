package node

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// mqtt-to-modbus 플로우 E2E 통합 테스트
//
// 실제 YAML 설정과 동일한 config를 사용하여 파이프라인 전체를 검증한다:
//   MQTT 메시지 → address-resolver (merge) → temp-cmd (select)
//   → payload.ToJSON() (브릿지 직렬화) → ModbusServerAgent.Process()
//   → get_map / get_register_typed 으로 레지스터 값 검증
// ---------------------------------------------------------------------------

// testModbusAgentConfig 는 테스트용 Modbus 서버 에이전트 설정을 반환한다.
func testModbusAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "test-modbus-server",
		Name: "Test MODBUS Server",
		Type: "modbus-gateway",
		Transport: agent.TransportConfig{
			Type: "modbus-gateway",
			Options: map[string]any{
				"listen_address":   "127.0.0.1",
				"listen_port":      0,
				"unit_id":          1,
				"max_connections":  5,
				"idle_timeout":     "30s",
				"msg_channel_size": 64,
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"coils": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"input_registers": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"discrete_inputs": map[string]any{
						"start_address": 0,
						"count":         100,
					},
				},
			},
		},
	}
}

// buildMqttSensorMessage 는 MQTT 센서 메시지를 생성한다.
// mqtt-to-modbus.yaml에 정의된 입력 형식과 동일하다.
func buildMqttSensorMessage(location, point string, sensorData map[string]any) message.Message {
	payload := map[string]any{
		"deviceInfo": map[string]any{
			"devEui": "a1b2c3d4e5f60001",
			"tags": map[string]any{
				"location": location,
				"point":    point,
			},
		},
		"object": sensorData,
	}

	return message.New(
		message.WithPayload(message.NewPayload(payload)),
	)
}

// addressResolverConfig 는 mqtt-to-modbus.yaml의 address-resolver 노드 설정을 반환한다.
// mode: merge, address_table 변수 바인딩, expression에서 $address_table 참조.
func addressResolverConfig() map[string]any {
	return map[string]any{
		"mode": "merge",
		"address_table": map[string]any{
			"창고:서버 옆":    0,
			"실습실:전방 우측":  8,
			"실습실:후방 오른쪽": 16,
			"실습실:전방 좌측":  24,
			"실습실:앞문":     32,
			"회의실:":       40,
		},
		"expression": `{
          _base: $address_table[
            $.payload.deviceInfo.tags.location & ":" & $.payload.deviceInfo.tags.point
          ]
        }`,
	}
}

// tempCmdConfig 는 mqtt-to-modbus.yaml의 temp-cmd 노드 설정을 반환한다.
// mode가 명시되지 않으므로 기본값 select가 적용된다.
func tempCmdConfig() map[string]any {
	return map[string]any{
		"expression": `{
          command: "set_input",
          params: {
            area: "input_registers",
            address: $.payload._base + 0,
            value: $.payload.object.temperature,
            data_type: "float32",
            byte_order: "big_endian"
          }
        }`,
	}
}

// humiCmdConfig 는 mqtt-to-modbus.yaml의 humi-cmd 노드 설정을 반환한다.
func humiCmdConfig() map[string]any {
	return map[string]any{
		"expression": `{
          command: "set_input",
          params: {
            area: "input_registers",
            address: $.payload._base + 2,
            value: $.payload.object.humidity,
            data_type: "float32",
            byte_order: "big_endian"
          }
        }`,
	}
}

// presCmdConfig 는 mqtt-to-modbus.yaml의 pres-cmd 노드 설정을 반환한다.
func presCmdConfig() map[string]any {
	return map[string]any{
		"expression": `{
          command: "set_input",
          params: {
            area: "input_registers",
            address: $.payload._base + 4,
            value: $.payload.object.pressure,
            data_type: "float32",
            byte_order: "big_endian"
          }
        }`,
	}
}

// battCmdConfig 는 mqtt-to-modbus.yaml의 batt-cmd 노드 설정을 반환한다.
// data_type과 byte_order가 없다 (기본값 uint16).
func battCmdConfig() map[string]any {
	return map[string]any{
		"expression": `{
          command: "set_input",
          params: {
            area: "input_registers",
            address: $.payload._base + 6,
            value: $.payload.object.battery
          }
        }`,
	}
}

// createTransformNode 는 TransformNode를 생성하고 설정을 적용한 뒤 Init한다.
func createTransformNode(t *testing.T, name string, config map[string]any) *TransformNode {
	t.Helper()
	def := flow.NewNodeDef(name, "transform")
	n, err := NewTransformNode(def)
	require.NoError(t, err, "NewTransformNode(%s) 실패", name)

	tn := n.(*TransformNode)
	err = tn.Configure(config)
	require.NoError(t, err, "Configure(%s) 실패", name)

	err = tn.Init(context.Background())
	require.NoError(t, err, "Init(%s) 실패", name)

	return tn
}

// processTransform 은 TransformNode를 통해 메시지를 처리하고 첫 번째 결과를 반환한다.
func processTransform(t *testing.T, tn *TransformNode, msg message.Message) message.Message {
	t.Helper()
	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err, "Process(%s) 실패", tn.Name())
	require.Len(t, results, 1, "Process(%s) 결과가 1개여야 한다", tn.Name())
	return results[0]
}

// simulateBridgeSend 는 브릿지의 Send 동작을 시뮬레이션한다.
// engine/resolver.go의 agentTransportAdapter.Send()와 동일:
//
//	data, err := msg.Payload().ToJSON()
//	_, err = agent.Process(data)
func simulateBridgeSend(t *testing.T, msg message.Message, ag agent.Agent) []byte {
	t.Helper()
	data, err := msg.Payload().ToJSON()
	require.NoError(t, err, "Payload().ToJSON() 실패")

	t.Logf("브릿지 전송 데이터: %s", string(data))

	resp, err := ag.Process(data)
	require.NoError(t, err, "Agent.Process() 실패")
	return resp
}

// TestMqttToModbus_EndToEnd 는 mqtt-to-modbus 플로우의 전체 데이터 파이프라인을 검증한다.
//
// 테스트 흐름:
//  1. MQTT 센서 메시지 생성 (location="창고", point="서버 옆", temperature=23.5)
//  2. address-resolver 변환 (merge 모드): _base 필드 추가 (주소 테이블 룩업)
//  3. temp-cmd 변환 (select 모드): set_input 커맨드 JSON 생성
//  4. payload.ToJSON() 직렬화 (브릿지 Send 동작 모방)
//  5. ModbusServerAgent.Process() 호출
//  6. get_map 및 get_register_typed 으로 레지스터 값 확인
func TestMqttToModbus_EndToEnd(t *testing.T) {
	ctx := context.Background()

	// ── 1단계: MQTT 센서 메시지 생성 ──
	mqttMsg := buildMqttSensorMessage("창고", "서버 옆", map[string]any{
		"temperature": 23.5,
	})
	t.Logf("1단계: MQTT 입력 메시지 payload = %v", mqttMsg.Payload().ToMap())

	// ── 2단계: address-resolver 변환 (merge 모드) ──
	resolver := createTransformNode(t, "address-resolver", addressResolverConfig())
	resolvedMsg := processTransform(t, resolver, mqttMsg)

	resolvedPayload := resolvedMsg.Payload().ToMap()
	t.Logf("2단계: address-resolver 출력 payload = %v", resolvedPayload)

	// _base 필드가 merge 되었는지 확인
	base, ok := resolvedPayload["_base"]
	require.True(t, ok, "_base 필드가 존재해야 한다")
	t.Logf("2단계: _base = %v (type=%T)", base, base)

	// "창고:서버 옆" → address_table 에서 0
	// address_table 값이 int(0)이지만 expression evaluator에서 어떤 타입으로 나올지 확인
	baseFloat, ok := toFloat64(base)
	require.True(t, ok, "_base가 숫자여야 한다 (got %T: %v)", base, base)
	assert.Equal(t, float64(0), baseFloat, "_base는 0이어야 한다 (창고:서버 옆)")

	// 원본 payload 필드가 보존되었는지 확인 (merge 모드)
	_, hasDeviceInfo := resolvedPayload["deviceInfo"]
	assert.True(t, hasDeviceInfo, "merge 모드이므로 deviceInfo가 보존되어야 한다")
	_, hasObject := resolvedPayload["object"]
	assert.True(t, hasObject, "merge 모드이므로 object가 보존되어야 한다")

	// ── 3단계: temp-cmd 변환 (select 모드) ──
	tempCmd := createTransformNode(t, "temp-cmd", tempCmdConfig())
	cmdMsg := processTransform(t, tempCmd, resolvedMsg)

	cmdPayload := cmdMsg.Payload().ToMap()
	t.Logf("3단계: temp-cmd 출력 payload = %v", cmdPayload)

	// command 필드 확인
	command, ok := cmdPayload["command"]
	require.True(t, ok, "command 필드가 존재해야 한다")
	assert.Equal(t, "set_input", command, "command는 set_input이어야 한다")

	// params 필드 확인
	params, ok := cmdPayload["params"]
	require.True(t, ok, "params 필드가 존재해야 한다")
	paramsMap, ok := params.(map[string]any)
	require.True(t, ok, "params가 map이어야 한다 (got %T)", params)

	t.Logf("3단계: params = %v", paramsMap)

	assert.Equal(t, "input_registers", paramsMap["area"], "area는 input_registers이어야 한다")
	assert.Equal(t, "float32", paramsMap["data_type"], "data_type은 float32이어야 한다")
	assert.Equal(t, "big_endian", paramsMap["byte_order"], "byte_order은 big_endian이어야 한다")

	// address: _base(0) + 0 = 0 (float64 산술 결과)
	addrVal, ok := toFloat64(paramsMap["address"])
	require.True(t, ok, "address가 숫자여야 한다 (got %T: %v)", paramsMap["address"], paramsMap["address"])
	assert.Equal(t, float64(0), addrVal, "address는 0이어야 한다")

	// value: 23.5
	tempVal, ok := toFloat64(paramsMap["value"])
	require.True(t, ok, "value가 숫자여야 한다 (got %T: %v)", paramsMap["value"], paramsMap["value"])
	assert.InDelta(t, 23.5, tempVal, 0.001, "value는 23.5이어야 한다")

	// ── 4단계: 브릿지 직렬화 (payload.ToJSON()) 확인 ──
	jsonData, err := cmdMsg.Payload().ToJSON()
	require.NoError(t, err, "ToJSON() 실패")
	t.Logf("4단계: 직렬화된 JSON = %s", string(jsonData))

	// JSON이 올바른 구조인지 확인
	var jsonCheck map[string]any
	err = json.Unmarshal(jsonData, &jsonCheck)
	require.NoError(t, err, "JSON 역직렬화 실패")
	assert.Equal(t, "set_input", jsonCheck["command"], "JSON command 필드 확인")

	// ── 5단계: Modbus 서버 에이전트 생성 및 Process 호출 ──
	cfg := testModbusAgentConfig()
	modbusAgent, err := modbusserver.NewModbusServerAgent(cfg, nil)
	require.NoError(t, err, "NewModbusServerAgent 실패")

	msa := modbusAgent.(*modbusserver.ModbusServerAgent)

	// 브릿지 Send 시뮬레이션: payload.ToJSON() → agent.Process()
	resp := simulateBridgeSend(t, cmdMsg, msa)
	t.Logf("5단계: Process 응답 = %s", string(resp))

	// Process 응답 확인
	var processResp map[string]any
	err = json.Unmarshal(resp, &processResp)
	require.NoError(t, err, "Process 응답 역직렬화 실패")
	assert.Equal(t, true, processResp["ok"], "Process 응답은 ok=true이어야 한다")

	// ── 6단계: get_map으로 레지스터 값 확인 ──
	getMapCmd, _ := json.Marshal(map[string]any{"command": "get_map"})
	mapResp, err := msa.Process(getMapCmd)
	require.NoError(t, err, "get_map 실패")
	t.Logf("6단계: get_map 응답 = %s", string(mapResp))

	var mapData map[string]any
	err = json.Unmarshal(mapResp, &mapData)
	require.NoError(t, err, "get_map 역직렬화 실패")

	regMap, ok := mapData["register_map"].(map[string]any)
	require.True(t, ok, "register_map이 존재해야 한다")
	inputRegs, ok := regMap["input_registers"].(map[string]any)
	require.True(t, ok, "input_registers가 맵이어야 한다 (got %T)", regMap["input_registers"])

	// address 0~1에 float32(23.5)가 기록되었는지 확인
	// float32는 2개 레지스터를 사용하므로 key "0"과 "1"에 값이 있어야 한다
	t.Logf("6단계: input_registers[\"0\"] = %v, input_registers[\"1\"] = %v",
		inputRegs["0"], inputRegs["1"])

	// 레지스터 0, 1 중 하나라도 0이 아니면 데이터가 기록된 것
	reg0, _ := toFloat64(inputRegs["0"])
	reg1, _ := toFloat64(inputRegs["1"])
	assert.True(t, reg0 != 0 || reg1 != 0,
		"input_registers[0:2]에 데이터가 기록되어야 한다 (got [%v, %v])", reg0, reg1)

	// ── 7단계: get_register_typed 으로 float32 값 정밀 확인 ──
	typedCmd, _ := json.Marshal(map[string]any{
		"command": "get_register_typed",
		"params": map[string]any{
			"area":       "input_registers",
			"address":    0,
			"data_type":  "float32",
			"byte_order": "big_endian",
		},
	})
	typedResp, err := msa.Process(typedCmd)
	require.NoError(t, err, "get_register_typed 실패")
	t.Logf("7단계: get_register_typed 응답 = %s", string(typedResp))

	var typedData map[string]any
	err = json.Unmarshal(typedResp, &typedData)
	require.NoError(t, err, "get_register_typed 역직렬화 실패")
	assert.Equal(t, true, typedData["ok"], "get_register_typed ok=true")

	readValue, ok := toFloat64(typedData["value"])
	require.True(t, ok, "value가 숫자여야 한다")
	assert.InDelta(t, 23.5, readValue, 0.01,
		"읽은 온도 값이 23.5에 근접해야 한다 (got %v)", readValue)

	_ = ctx // prevent unused variable warning
}

// TestMqttToModbus_EndToEnd_MultiSensor 는 여러 센서 필드(온도, 습도, 기압, 배터리)를
// 동시에 전송하는 시나리오를 검증한다.
func TestMqttToModbus_EndToEnd_MultiSensor(t *testing.T) {
	// ── MQTT 센서 메시지 생성: 모든 필드 포함 ──
	mqttMsg := buildMqttSensorMessage("실습실", "전방 우측", map[string]any{
		"temperature": 25.3,
		"humidity":    62.7,
		"pressure":    1013.25,
		"battery":     85,
	})

	// ── address-resolver (merge 모드) ──
	resolver := createTransformNode(t, "address-resolver", addressResolverConfig())
	resolvedMsg := processTransform(t, resolver, mqttMsg)

	resolvedPayload := resolvedMsg.Payload().ToMap()
	t.Logf("address-resolver 출력: %v", resolvedPayload)

	// "실습실:전방 우측" → 8
	base, ok := resolvedPayload["_base"]
	require.True(t, ok, "_base 필드가 존재해야 한다")
	baseFloat, ok := toFloat64(base)
	require.True(t, ok, "_base가 숫자여야 한다")
	assert.Equal(t, float64(8), baseFloat, "_base는 8이어야 한다 (실습실:전방 우측)")

	// ── Modbus 에이전트 생성 ──
	cfg := testModbusAgentConfig()
	modbusAgent, err := modbusserver.NewModbusServerAgent(cfg, nil)
	require.NoError(t, err, "NewModbusServerAgent 실패")
	msa := modbusAgent.(*modbusserver.ModbusServerAgent)

	// ── 각 센서별 커맨드 생성 및 전송 ──
	type sensorTest struct {
		name      string
		config    map[string]any
		wantAddr  float64
		wantValue float64
		dataType  string
	}

	sensors := []sensorTest{
		{
			name:      "temperature",
			config:    tempCmdConfig(),
			wantAddr:  8, // _base(8) + 0
			wantValue: 25.3,
			dataType:  "float32",
		},
		{
			name:      "humidity",
			config:    humiCmdConfig(),
			wantAddr:  10, // _base(8) + 2
			wantValue: 62.7,
			dataType:  "float32",
		},
		{
			name:      "pressure",
			config:    presCmdConfig(),
			wantAddr:  12, // _base(8) + 4
			wantValue: 1013.25,
			dataType:  "float32",
		},
		{
			name:      "battery",
			config:    battCmdConfig(),
			wantAddr:  14, // _base(8) + 6
			wantValue: 85,
			dataType:  "", // uint16 기본값
		},
	}

	for _, s := range sensors {
		t.Run(s.name, func(t *testing.T) {
			cmd := createTransformNode(t, s.name+"-cmd", s.config)
			cmdMsg := processTransform(t, cmd, resolvedMsg)
			cmdPayload := cmdMsg.Payload().ToMap()
			t.Logf("%s-cmd 출력: %v", s.name, cmdPayload)

			// address 확인
			params := cmdPayload["params"].(map[string]any)
			addr, ok := toFloat64(params["address"])
			require.True(t, ok, "address가 숫자여야 한다")
			assert.Equal(t, s.wantAddr, addr, "%s address 불일치", s.name)

			// 브릿지 전송
			resp := simulateBridgeSend(t, cmdMsg, msa)
			t.Logf("%s Process 응답: %s", s.name, string(resp))

			var processResp map[string]any
			err := json.Unmarshal(resp, &processResp)
			require.NoError(t, err)
			assert.Equal(t, true, processResp["ok"])
		})
	}

	// ── get_register_typed 으로 각 센서 값 확인 ──
	for _, s := range sensors {
		if s.dataType == "" {
			continue // battery는 uint16이므로 ReadTyped에서 다르게 처리
		}
		t.Run(s.name+"_read_typed", func(t *testing.T) {
			typedCmd, _ := json.Marshal(map[string]any{
				"command": "get_register_typed",
				"params": map[string]any{
					"area":       "input_registers",
					"address":    s.wantAddr,
					"data_type":  s.dataType,
					"byte_order": "big_endian",
				},
			})
			typedResp, err := msa.Process(typedCmd)
			require.NoError(t, err, "get_register_typed 실패")
			t.Logf("%s typed 응답: %s", s.name, string(typedResp))

			var typedData map[string]any
			err = json.Unmarshal(typedResp, &typedData)
			require.NoError(t, err)
			assert.Equal(t, true, typedData["ok"])

			readVal, ok := toFloat64(typedData["value"])
			require.True(t, ok, "value가 숫자여야 한다")

			// float32 정밀도 고려 (약 6-7자리 유효숫자)
			assert.InDelta(t, s.wantValue, readVal, 0.1,
				"%s: 읽은 값이 원본과 근접해야 한다 (want=%v, got=%v)", s.name, s.wantValue, readVal)
		})
	}
}

// TestMqttToModbus_EndToEnd_AddressResolution 은 다양한 태그 조합에 대한
// 주소 해석이 올바르게 동작하는지 검증한다.
func TestMqttToModbus_EndToEnd_AddressResolution(t *testing.T) {
	tests := []struct {
		name     string
		location string
		point    string
		wantBase float64
	}{
		{"창고_서버옆", "창고", "서버 옆", 0},
		{"실습실_전방우측", "실습실", "전방 우측", 8},
		{"실습실_후방오른쪽", "실습실", "후방 오른쪽", 16},
		{"실습실_전방좌측", "실습실", "전방 좌측", 24},
		{"실습실_앞문", "실습실", "앞문", 32},
		{"회의실_빈값", "회의실", "", 40},
	}

	resolver := createTransformNode(t, "address-resolver", addressResolverConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildMqttSensorMessage(tt.location, tt.point, map[string]any{
				"temperature": 20.0,
			})

			resolved := processTransform(t, resolver, msg)
			payload := resolved.Payload().ToMap()

			base, ok := payload["_base"]
			require.True(t, ok, "_base 필드가 존재해야 한다")

			baseFloat, ok := toFloat64(base)
			require.True(t, ok, "_base가 숫자여야 한다 (got %T: %v)", base, base)
			assert.Equal(t, tt.wantBase, baseFloat,
				"태그 %s:%s → register_base %v 기대, 실제 %v", tt.location, tt.point, tt.wantBase, baseFloat)
		})
	}
}

// TestMqttToModbus_EndToEnd_UnknownTag 는 address_table에 없는 태그 조합일 때
// _base가 nil이 되고 이후 산술 연산에서 에러가 발생하는지 확인한다.
func TestMqttToModbus_EndToEnd_UnknownTag(t *testing.T) {
	resolver := createTransformNode(t, "address-resolver", addressResolverConfig())

	// 존재하지 않는 태그 조합
	msg := buildMqttSensorMessage("존재하지않는곳", "알수없는포인트", map[string]any{
		"temperature": 20.0,
	})

	resolved := processTransform(t, resolver, msg)
	payload := resolved.Payload().ToMap()
	t.Logf("알 수 없는 태그 결과: %v", payload)

	// _base는 nil (테이블에 없으므로)
	base := payload["_base"]
	assert.Nil(t, base, "존재하지 않는 태그의 _base는 nil이어야 한다")

	// _base가 nil인 상태에서 temp-cmd 실행 시 산술 에러 확인
	tempCmd := createTransformNode(t, "temp-cmd", tempCmdConfig())
	_, err := tempCmd.Process(context.Background(), resolved)
	// nil + 0 은 산술 연산 에러를 발생시켜야 한다
	t.Logf("nil _base로 temp-cmd 실행 결과: err=%v", err)
	assert.Error(t, err, "nil _base에 대한 산술 연산은 에러를 발생시켜야 한다")
}

// TestMqttToModbus_EndToEnd_Float32Precision 은 float32 인코딩/디코딩의
// 정밀도를 검증한다. IEEE 754 float32는 약 7자리 유효숫자를 보장한다.
func TestMqttToModbus_EndToEnd_Float32Precision(t *testing.T) {
	values := []float64{
		0.0,
		1.0,
		-1.0,
		23.5,
		100.125,
		1013.25,
		-40.0,   // 극저온 센서
		0.001,   // 미세 값
		65535.0, // uint16 최대 범위
	}

	cfg := testModbusAgentConfig()
	modbusAgent, err := modbusserver.NewModbusServerAgent(cfg, nil)
	require.NoError(t, err)
	msa := modbusAgent.(*modbusserver.ModbusServerAgent)

	resolver := createTransformNode(t, "address-resolver", addressResolverConfig())
	tempCmd := createTransformNode(t, "temp-cmd", tempCmdConfig())

	for _, val := range values {
		t.Run(fmt.Sprintf("value_%v", val), func(t *testing.T) {
			msg := buildMqttSensorMessage("창고", "서버 옆", map[string]any{
				"temperature": val,
			})

			resolved := processTransform(t, resolver, msg)
			cmdMsg := processTransform(t, tempCmd, resolved)

			resp := simulateBridgeSend(t, cmdMsg, msa)
			var processResp map[string]any
			require.NoError(t, json.Unmarshal(resp, &processResp))
			assert.Equal(t, true, processResp["ok"])

			// ReadTyped로 확인
			typedCmd, _ := json.Marshal(map[string]any{
				"command": "get_register_typed",
				"params": map[string]any{
					"area":       "input_registers",
					"address":    0,
					"data_type":  "float32",
					"byte_order": "big_endian",
				},
			})
			typedResp, err := msa.Process(typedCmd)
			require.NoError(t, err)

			var typedData map[string]any
			require.NoError(t, json.Unmarshal(typedResp, &typedData))

			readVal, ok := toFloat64(typedData["value"])
			require.True(t, ok)

			// float32 정밀도: float64 → float32 → float64 변환 시 오차 허용
			expected := float64(float32(val))
			if expected == 0 {
				assert.Equal(t, float64(0), readVal,
					"float32 인코딩/디코딩: 0 값 확인")
			} else {
				// 상대 오차로 비교 (float32 유효숫자 약 7자리)
				relErr := math.Abs((expected - readVal) / expected)
				assert.Less(t, relErr, 1e-6,
					"float32 인코딩/디코딩 정밀도 확인: input=%v, expected=%v, got=%v, relErr=%v",
					val, expected, readVal, relErr)
			}
		})
	}
}
