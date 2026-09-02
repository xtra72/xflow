package samsung

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// DetectDeviceType 테스트
// ---------------------------------------------------------------------------

// TestDetectDeviceType 는 주소의 첫 번째 바이트 또는 상수 주소로
// 디바이스 타입을 올바르게 판별하는지 검증한다.
func TestDetectDeviceType(t *testing.T) {
	tests := []struct {
		name string
		addr NasaAddress
		want string
	}{
		{name: "outdoor unit 0x10", addr: NasaAddress{0x10, 0x00, 0x00}, want: "HVACR.ODU"},
		{name: "outdoor unit index 5", addr: NasaAddress{0x10, 0x05, 0x00}, want: "HVACR.ODU"},
		{name: "indoor unit 0x20", addr: NasaAddress{0x20, 0x00, 0x01}, want: "HVACR.IDU"},
		{name: "indoor unit index 3-7", addr: NasaAddress{0x20, 0x03, 0x07}, want: "HVACR.IDU"},
		{name: "controller", addr: AddrController, want: "controller"},
		{name: "broadcast all is unknown", addr: NasaAddress{0xB0, 0xFF, 0xFF}, want: "unknown"},
		{name: "arbitrary address is unknown", addr: NasaAddress{0x50, 0x00, 0x00}, want: "unknown"},
		{name: "zero address is unknown", addr: NasaAddress{0x00, 0x00, 0x00}, want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectDeviceType(tt.addr)
			if got != tt.want {
				t.Errorf("DetectDeviceType(%v) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 온도 인코딩/디코딩 테스트
// ---------------------------------------------------------------------------

// TestEncodeTemperature 는 섭씨 온도를 NASA uint16 인코딩으로 변환하는지 검증한다.
func TestEncodeTemperature(t *testing.T) {
	tests := []struct {
		name string
		temp float32
		want uint16
	}{
		{name: "18.0C", temp: 18.0, want: 0x00B4},
		{name: "24.0C", temp: 24.0, want: 0x00F0},
		{name: "30.0C", temp: 30.0, want: 0x012C},
		{name: "0.0C", temp: 0.0, want: 0x0000},
		{name: "-1.0C", temp: -1.0, want: 0xFFF6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeTemperature(tt.temp)
			if got != tt.want {
				t.Errorf("EncodeTemperature(%v) = 0x%04X, want 0x%04X", tt.temp, got, tt.want)
			}
		})
	}
}

// TestDecodeTemperature 는 NASA uint16 인코딩을 섭씨 온도로 변환하는지 검증한다.
func TestDecodeTemperature(t *testing.T) {
	tests := []struct {
		name string
		raw  uint16
		want float32
	}{
		{name: "180 = 18.0C", raw: 0x00B4, want: 18.0},
		{name: "240 = 24.0C", raw: 0x00F0, want: 24.0},
		{name: "300 = 30.0C", raw: 0x012C, want: 30.0},
		{name: "0 = 0.0C", raw: 0x0000, want: 0.0},
		{name: "0xFFF6 = -1.0C", raw: 0xFFF6, want: -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeTemperature(tt.raw)
			if got != tt.want {
				t.Errorf("DecodeTemperature(0x%04X) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestTemperatureRoundTrip 은 인코딩과 디코딩의 왕복 변환이 일치하는지 검증한다.
func TestTemperatureRoundTrip(t *testing.T) {
	temps := []float32{18.0, 24.0, 30.0, 0.0, -1.0, 16.5, 25.5}
	for _, temp := range temps {
		encoded := EncodeTemperature(temp)
		decoded := DecodeTemperature(encoded)
		if decoded != temp {
			t.Errorf("round-trip failed: %v -> 0x%04X -> %v", temp, encoded, decoded)
		}
	}
}

// ---------------------------------------------------------------------------
// 모드 매핑 테스트
// ---------------------------------------------------------------------------

// TestModeToString 은 모든 모드 바이트가 올바른 문자열로 변환되는지 검증한다.
func TestModeToString(t *testing.T) {
	tests := []struct {
		name string
		mode byte
		want string
	}{
		{name: "auto", mode: 0x00, want: "auto"},
		{name: "cool", mode: 0x01, want: "cool"},
		{name: "dry", mode: 0x02, want: "dry"},
		{name: "fan", mode: 0x03, want: "fan"},
		{name: "heat", mode: 0x04, want: "heat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ModeToString[tt.mode]
			if !ok {
				t.Fatalf("ModeToString[0x%02X] not found", tt.mode)
			}
			if got != tt.want {
				t.Errorf("ModeToString[0x%02X] = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// TestStringToMode 는 모든 모드 문자열이 올바른 바이트로 변환되는지 검증한다.
func TestStringToMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want byte
	}{
		{name: "auto", mode: "auto", want: 0x00},
		{name: "cool", mode: "cool", want: 0x01},
		{name: "dry", mode: "dry", want: 0x02},
		{name: "fan", mode: "fan", want: 0x03},
		{name: "heat", mode: "heat", want: 0x04},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := StringToMode[tt.mode]
			if !ok {
				t.Fatalf("StringToMode[%q] not found", tt.mode)
			}
			if got != tt.want {
				t.Errorf("StringToMode[%q] = 0x%02X, want 0x%02X", tt.mode, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 팬 스피드 매핑 테스트
// ---------------------------------------------------------------------------

// TestFanSpeedToString 은 모든 팬 속도 바이트가 올바른 문자열로 변환되는지 검증한다.
func TestFanSpeedToString(t *testing.T) {
	tests := []struct {
		name  string
		speed byte
		want  string
	}{
		{name: "auto", speed: 0x00, want: "auto"},
		{name: "low", speed: 0x01, want: "low"},
		{name: "medium", speed: 0x02, want: "medium"},
		{name: "high", speed: 0x03, want: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FanSpeedToString[tt.speed]
			if !ok {
				t.Fatalf("FanSpeedToString[0x%02X] not found", tt.speed)
			}
			if got != tt.want {
				t.Errorf("FanSpeedToString[0x%02X] = %q, want %q", tt.speed, got, tt.want)
			}
		})
	}
}

// TestStringToFanSpeed 는 모든 팬 속도 문자열이 올바른 바이트로 변환되는지 검증한다.
func TestStringToFanSpeed(t *testing.T) {
	tests := []struct {
		name  string
		speed string
		want  byte
	}{
		{name: "auto", speed: "auto", want: 0x00},
		{name: "low", speed: "low", want: 0x01},
		{name: "medium", speed: "medium", want: 0x02},
		{name: "high", speed: "high", want: 0x03},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := StringToFanSpeed[tt.speed]
			if !ok {
				t.Fatalf("StringToFanSpeed[%q] not found", tt.speed)
			}
			if got != tt.want {
				t.Errorf("StringToFanSpeed[%q] = 0x%02X, want 0x%02X", tt.speed, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateFromMessageSets 테스트
// ---------------------------------------------------------------------------

// TestUpdateFromMessageSets_Power 는 전원 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_Power(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
		want  bool
	}{
		{name: "power on", value: []byte{0x01}, want: true},
		{name: "power off", value: []byte{0x00}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &NasaDeviceState{
				RawMessageSets: make(map[uint16][]byte),
			}
			s.UpdateFromMessageSets([]NasaMessageSet{
				{Index: MsgPower, Value: tt.value},
			})
			if s.Power != tt.want {
				t.Errorf("Power = %v, want %v", s.Power, tt.want)
			}
		})
	}
}

// TestUpdateFromMessageSets_Mode 는 모드 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_Mode(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
		want  string
	}{
		{name: "cool mode", value: []byte{0x01}, want: "cool"},
		{name: "heat mode", value: []byte{0x04}, want: "heat"},
		{name: "auto mode", value: []byte{0x00}, want: "auto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &NasaDeviceState{
				RawMessageSets: make(map[uint16][]byte),
			}
			s.UpdateFromMessageSets([]NasaMessageSet{
				{Index: MsgMode, Value: tt.value},
			})
			if s.Mode != tt.want {
				t.Errorf("Mode = %q, want %q", s.Mode, tt.want)
			}
		})
	}
}

// TestUpdateFromMessageSets_FanSpeed 는 팬 스피드 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_FanSpeed(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgFanSpeed, Value: []byte{0x02}},
	})
	if s.FanSpeed != "medium" {
		t.Errorf("FanSpeed = %q, want %q", s.FanSpeed, "medium")
	}
}

// TestUpdateFromMessageSets_TargetTemp 는 목표 온도 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_TargetTemp(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	// 24.0C = 0x00F0 = [0x00, 0xF0]
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgTargetTemp, Value: []byte{0x00, 0xF0}},
	})
	if s.TargetTemp != 24.0 {
		t.Errorf("TargetTemp = %v, want %v", s.TargetTemp, 24.0)
	}
}

// TestUpdateFromMessageSets_CurrentTemp 는 현재 온도 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_CurrentTemp(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	// 18.0C = 0x00B4 = [0x00, 0xB4]
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgCurrentTemp, Value: []byte{0x00, 0xB4}},
	})
	if s.CurrentTemp != 18.0 {
		t.Errorf("CurrentTemp = %v, want %v", s.CurrentTemp, 18.0)
	}
}

// TestUpdateFromMessageSets_CurrentHumidity 는 현재 습도 메시지 세트(NASA V1.1 지표
// 세트 #10, 0x4038)가 1바이트 uint8 percent 로 처리되고 관측 기반 emit 되는지 검증한다.
func TestUpdateFromMessageSets_CurrentHumidity(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	// 62% = 0x3E = [0x3E]
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgCurrentHumidity, Value: []byte{0x3E}},
	})
	if s.CurrentHumidity != 62 {
		t.Errorf("CurrentHumidity = %v, want %v", s.CurrentHumidity, 62)
	}

	// 관측 기반 emit: 관측된 current_humidity 는 StateForJSON 에 나타나야 한다.
	result := s.StateForJSON(false)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if v, ok := m["current_humidity"]; !ok {
		t.Error("관측된 current_humidity 는 emit 되어야 함")
	} else if v != float64(62) { // JSON 숫자는 float64 로 역직렬화
		t.Errorf("current_humidity = %v, want 62", v)
	}
}

// TestUpdateFromMessageSets_SwingVertical 는 수직 스윙 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_SwingVertical(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgSwingVertical, Value: []byte{0x01}},
	})
	if !s.SwingVertical {
		t.Errorf("SwingVertical = %v, want %v", s.SwingVertical, true)
	}
}

// TestUpdateFromMessageSets_FilterAlarm 는 필터 알람 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_FilterAlarm(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgFilterCleanAlarm, Value: []byte{0x01}},
	})
	if !s.FilterAlarm {
		t.Errorf("FilterAlarm = %v, want %v", s.FilterAlarm, true)
	}
}

// TestUpdateFromMessageSets_ErrorCode 는 에러 코드 메시지 세트가 올바르게 처리되는지 검증한다.
func TestUpdateFromMessageSets_ErrorCode(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	// error code 0x0102
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgErrorCode, Value: []byte{0x01, 0x02}},
	})
	if s.ErrorCode != 0x0102 {
		t.Errorf("ErrorCode = 0x%04X, want 0x%04X", s.ErrorCode, 0x0102)
	}
}

// TestUpdateFromMessageSets_MultipleSets 는 여러 메시지 세트를 한번에 처리하는지 검증한다.
func TestUpdateFromMessageSets_MultipleSets(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgPower, Value: []byte{0x01}},
		{Index: MsgMode, Value: []byte{0x01}},
		{Index: MsgTargetTemp, Value: []byte{0x00, 0xF0}},
		{Index: MsgCurrentTemp, Value: []byte{0x00, 0xB4}},
		{Index: MsgFanSpeed, Value: []byte{0x03}},
	})

	if !s.Power {
		t.Errorf("Power = %v, want %v", s.Power, true)
	}
	if s.Mode != "cool" {
		t.Errorf("Mode = %q, want %q", s.Mode, "cool")
	}
	if s.TargetTemp != 24.0 {
		t.Errorf("TargetTemp = %v, want %v", s.TargetTemp, 24.0)
	}
	if s.CurrentTemp != 18.0 {
		t.Errorf("CurrentTemp = %v, want %v", s.CurrentTemp, 18.0)
	}
	if s.FanSpeed != "high" {
		t.Errorf("FanSpeed = %q, want %q", s.FanSpeed, "high")
	}
}

// TestUpdateFromMessageSets_UnknownIndex 는 알 수 없는 인덱스도
// RawMessageSets 에 저장되는지 검증한다.
func TestUpdateFromMessageSets_UnknownIndex(t *testing.T) {
	s := &NasaDeviceState{
		RawMessageSets: make(map[uint16][]byte),
	}
	unknownIndex := uint16(0x4099)
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: unknownIndex, Value: []byte{0xAB}},
	})

	stored, ok := s.RawMessageSets[unknownIndex]
	if !ok {
		t.Fatal("RawMessageSets should contain unknown index 0x4099")
	}
	if len(stored) != 1 || stored[0] != 0xAB {
		t.Errorf("RawMessageSets[0x4099] = %v, want [0xAB]", stored)
	}
}

// ---------------------------------------------------------------------------
// HexKeyByteMap 테스트
// ---------------------------------------------------------------------------

// TestHexKeyByteMap_MarshalJSON 은 uint16 키가 "0x0402" 형식 16진수로 직렬화되는지 검증한다.
func TestHexKeyByteMap_MarshalJSON(t *testing.T) {
	m := HexKeyByteMap{
		0x0402: {0x01, 0x02},
		0x4000: {0xFF},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal result failed: %v", err)
	}

	if _, ok := result["0x0402"]; !ok {
		t.Error("expected key '0x0402' in JSON output")
	}
	if _, ok := result["0x4000"]; !ok {
		t.Error("expected key '0x4000' in JSON output")
	}
	// 10진수 키가 없어야 함
	if _, ok := result["1026"]; ok {
		t.Error("unexpected decimal key '1026' in JSON output")
	}
}

// TestHexKeyByteMap_UnmarshalJSON_Hex 는 16진수 키 파싱을 검증한다.
func TestHexKeyByteMap_UnmarshalJSON_Hex(t *testing.T) {
	input := `{"0x0402":"AQI=","0x4000":"/w=="}`
	var m HexKeyByteMap
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if _, ok := m[0x0402]; !ok {
		t.Error("expected key 0x0402")
	}
	if _, ok := m[0x4000]; !ok {
		t.Error("expected key 0x4000")
	}
}

// TestHexKeyByteMap_UnmarshalJSON_Decimal 은 10진수 키도 파싱되는지 검증한다.
func TestHexKeyByteMap_UnmarshalJSON_Decimal(t *testing.T) {
	input := `{"1026":"AQI="}`
	var m HexKeyByteMap
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if _, ok := m[1026]; !ok {
		t.Error("expected key 1026 (0x0402)")
	}
}

// ---------------------------------------------------------------------------
// StateForJSON 테스트
// ---------------------------------------------------------------------------

// TestStateForJSON_IncludeRaw 는 includeRaw=true일 때 RawMessageSets가 포함되는지 검증한다.
func TestStateForJSON_IncludeRaw(t *testing.T) {
	s := &NasaDeviceState{
		Power:          true,
		Mode:           "cool",
		TargetTemp:     24,
		CurrentTemp:    25.5,
		RawMessageSets: HexKeyByteMap{0x4000: {0x01}},
		observedCore:   observedPower | observedMode | observedTargetTemp | observedCurrentTemp,
	}

	result := s.StateForJSON(true)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["raw_message_sets"]; !ok {
		t.Error("raw_message_sets should be included when includeRaw=true")
	}
}

// TestStateForJSON_ExcludeRaw 는 includeRaw=false일 때 RawMessageSets가 제외되는지 검증한다.
func TestStateForJSON_ExcludeRaw(t *testing.T) {
	s := &NasaDeviceState{
		Power:          true,
		Mode:           "cool",
		TargetTemp:     24,
		CurrentTemp:    25.5,
		RawMessageSets: HexKeyByteMap{0x4000: {0x01}},
		observedCore:   observedPower | observedMode | observedTargetTemp | observedCurrentTemp,
	}

	result := s.StateForJSON(false)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["raw_message_sets"]; ok {
		t.Error("raw_message_sets should not be included when includeRaw=false")
	}
	// 관측된 필드는 존재해야 함
	if _, ok := m["power"]; !ok {
		t.Error("power field should be present")
	}
	if _, ok := m["mode"]; !ok {
		t.Error("mode field should be present")
	}
}

// TestStateForJSON_OmitsUnobserved 는 power 를 포함한 모든 핵심 필드가 관측 기반으로
// emit 되는지 검증한다("확인된 값만 전송").
// 재현: 아무 필드도 관측 안 된(관측 비트 0) 디바이스 — 재시작 직후, power 프레임(0x4000)
// 미수신 — 은 power 를 zero-value(false)로 방출해서는 안 된다. 방출 시 대시보드가 실제
// on 상태를 default off 로 덮어써 "꺼졌다 켜진" 것처럼 보이는 회귀를 유발한다.
func TestStateForJSON_OmitsUnobserved(t *testing.T) {
	// 아무것도 관측 안 됨(재시작 직후, power 프레임 미수신).
	s := &NasaDeviceState{Power: false, observedCore: 0}

	result := s.StateForJSON(false)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// power 미관측 시 payload 에서 생략되어야 한다(기본값 false 방출 금지).
	if v, ok := m["power"]; ok {
		t.Errorf("관측되지 않은 power 는 생략되어야 함(기본값 방출 금지): got %v (present=%v)", v, ok)
	}
	for _, k := range []string{"mode", "target_temperature", "current_temperature", "current_humidity", "fan_speed", "swing_vertical", "filter_alarm", "error_code"} {
		if _, ok := m[k]; ok {
			t.Errorf("관측되지 않은 필드 %q 는 생략되어야 함: got %v", k, m[k])
		}
	}

	// 관측된 경우: current_humidity 관측 시 payload 에 present 해야 한다.
	sObs := &NasaDeviceState{CurrentHumidity: 55, observedCore: observedHumidity}
	obsData, err := json.Marshal(sObs.StateForJSON(false))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var mObs map[string]any
	if err := json.Unmarshal(obsData, &mObs); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if v, ok := mObs["current_humidity"]; !ok {
		t.Error("관측된 current_humidity 는 present 해야 함")
	} else if v != float64(55) {
		t.Errorf("current_humidity = %v, want 55", v)
	}
}

// TestUpdateFromMessageSets_ObservationDrivesEmit 는 메시지 셋으로 관측된 필드만
// StateForJSON 에 나타나는지(관측 파이프라인 통합) 검증한다.
func TestUpdateFromMessageSets_ObservationDrivesEmit(t *testing.T) {
	s := &NasaDeviceState{RawMessageSets: make(HexKeyByteMap)}
	// power 와 target_temp 만 관측시킨다.
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: MsgPower, Value: []byte{0x01}},
		{Index: MsgTargetTemp, Value: []byte{0x00, 0xF0}}, // 24.0℃
	})

	result := s.StateForJSON(false)
	data, _ := json.Marshal(result)
	var m map[string]any
	_ = json.Unmarshal(data, &m)

	if _, ok := m["power"]; !ok {
		t.Error("관측된 power 는 present 여야 함")
	}
	if _, ok := m["target_temperature"]; !ok {
		t.Error("관측된 target_temperature 는 present 여야 함")
	}
	if _, ok := m["mode"]; ok {
		t.Error("미관측 mode 는 생략되어야 함")
	}
	if _, ok := m["current_temperature"]; ok {
		t.Error("미관측 current_temperature 는 생략되어야 함")
	}
}
