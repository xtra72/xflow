package samsung

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeOutdoorValue 는 kind 별 실외기 값 디코딩을 검증한다.
func TestDecodeOutdoorValue(t *testing.T) {
	tests := []struct {
		name string
		kind outdoorFieldKind
		raw  []byte
		want any
		ok   bool
	}{
		{"temp_positive", oduTempSigned, []byte{0x00, 0xD7}, float32(21.5), true}, // 215/10
		{"temp_negative", oduTempSigned, []byte{0xFF, 0xCE}, float32(-5.0), true}, // int16(-50)/10
		{"temp_short", oduTempSigned, []byte{0x00}, nil, false},
		{"enum", oduEnum, []byte{0x05}, 5, true},
		{"enum_empty", oduEnum, []byte{}, nil, false},
		{"var_raw", oduVarRaw, []byte{0x01, 0x2C}, 300, true},
		{"var_short", oduVarRaw, []byte{0x01}, nil, false},
		{"lvar_raw", oduLvarRaw, []byte{0x00, 0x00, 0x08, 0xAE}, int64(2222), true},
		{"lvar_short", oduLvarRaw, []byte{0x00, 0x00, 0x08}, nil, false},
		// 미장착/무효 센티넬 (전부 0xFF) → ok=false.
		{"enum_sentinel", oduEnum, []byte{0xFF}, nil, false},
		{"var_sentinel", oduVarRaw, []byte{0xFF, 0xFF}, nil, false},
		{"lvar_sentinel", oduLvarRaw, []byte{0xFF, 0xFF, 0xFF, 0xFF}, nil, false},
		// 온도는 부호 있음 — 0xFFFF 는 -0.1°C 정상 판독으로 유지(센티넬 마스킹 없음).
		{"temp_ffff_kept", oduTempSigned, []byte{0xFF, 0xFF}, float32(-0.1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decodeOutdoorValue(tt.kind, tt.raw)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestOutdoorState_DiscreteChangeGate 는 이산 필드 변화 시에만 discreteChanged=true 임을 검증한다.
func TestOutdoorState_DiscreteChangeGate(t *testing.T) {
	s := NewOutdoorState()

	// 최초 이산 필드(운전상태 ENUM) 관측 → true.
	changed := s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8001, Value: []byte{0x02}}, // out_operation_odu_mode = 2
	})
	assert.True(t, changed, "최초 이산 필드 관측은 변화로 간주")
	assert.Equal(t, 2, s.Fields["out_operation_odu_mode"])

	// 동일 이산 값 재수신 → false.
	changed = s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8001, Value: []byte{0x02}},
	})
	assert.False(t, changed, "동일 이산 값은 변화 아님")

	// 연속 센서(외기온도)만 변경 → false (emit 폭주 방지).
	changed = s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8204, Value: []byte{0x00, 0xD7}}, // outdoor_temperature = 21.5
	})
	assert.False(t, changed, "연속 센서 변경은 이산 변화 아님")
	assert.Equal(t, float32(21.5), s.Fields["outdoor_temperature"], "연속 센서도 Fields 는 갱신")

	// 이산 값 변경(운전상태 2→5 제상) → true.
	changed = s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8001, Value: []byte{0x05}},
	})
	assert.True(t, changed, "이산 값 변경은 emit 트리거")
	assert.Equal(t, 5, s.Fields["out_operation_odu_mode"])

	// 에러 코드(이산)도 변경 시 트리거.
	changed = s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8235, Value: []byte{0x01, 0x2C}}, // error_code = 300
	})
	assert.True(t, changed, "에러 코드 변경은 emit 트리거")
	assert.Equal(t, 300, s.Fields["error_code"])
}

// TestOutdoorState_StateForJSON 은 관측된 필드만 노출하고 includeRaw 를 준수함을 검증한다.
func TestOutdoorState_StateForJSON(t *testing.T) {
	s := NewOutdoorState()
	s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8204, Value: []byte{0x00, 0xD7}},             // airout 21.5
		{Index: 0x8001, Value: []byte{0x02}},                   // odu_mode 2
		{Index: 0x8413, Value: []byte{0x00, 0x00, 0x08, 0xAE}}, // wattmeter 2222
	})

	out := s.StateForJSON(false).(map[string]any)
	assert.Equal(t, float32(21.5), out["outdoor_temperature"])
	assert.Equal(t, 2, out["out_operation_odu_mode"])
	assert.Equal(t, int64(2222), out["wattmeter_1min_sum"])
	// 관측되지 않은 필드는 생략.
	_, exists := out["compressor_discharge_temperature"]
	assert.False(t, exists, "미관측 필드는 payload 에서 생략")
	// includeRaw=false 이면 raw_message_sets 없음.
	_, hasRaw := out["raw_message_sets"]
	assert.False(t, hasRaw)

	// includeRaw=true 이면 raw_message_sets 포함.
	outRaw := s.StateForJSON(true).(map[string]any)
	_, hasRaw = outRaw["raw_message_sets"]
	assert.True(t, hasRaw, "includeRaw=true 이면 raw 포함")
}

// TestOutdoorState_UnknownIndexPreservedInRaw 는 미등록 인덱스도 raw 에 보존됨을 검증한다.
func TestOutdoorState_UnknownIndexPreservedInRaw(t *testing.T) {
	s := NewOutdoorState()
	changed := s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x89FF, Value: []byte{0xAB}}, // 미등록 인덱스
	})
	assert.False(t, changed)
	assert.Empty(t, s.Fields, "미등록 인덱스는 Fields 에 디코드되지 않음")
	assert.Contains(t, s.RawMessageSets, uint16(0x89FF), "미등록 인덱스도 raw 에는 보존")
}

// TestOutdoorState_SentinelOmitted 는 미장착 센티넬(0xFFFF) 필드가 Fields 에서 생략되고
// raw 에는 보존됨을 검증한다(단일 압축기 유닛의 comp2 주파수 실측 케이스).
func TestOutdoorState_SentinelOmitted(t *testing.T) {
	s := NewOutdoorState()
	changed := s.UpdateFromMessageSets([]NasaMessageSet{
		{Index: 0x8274, Value: []byte{0xFF, 0xFF}}, // out_control_order_cfreq_comp2 = 미장착
		{Index: 0x8010, Value: []byte{0x01}},       // out_load_comp1 = ON (실제 값)
	})
	assert.True(t, changed, "실제 이산 값(comp1)은 변화로 감지")

	out := s.StateForJSON(false).(map[string]any)
	_, hasComp2 := out["out_control_order_cfreq_comp2"]
	assert.False(t, hasComp2, "센티넬(0xFFFF) 주파수는 payload 에서 생략")
	assert.Equal(t, 1, out["out_load_comp1"], "실제 값은 정상 노출")

	// 센티넬 필드도 raw 에는 보존되어 실측 진단이 가능하다.
	assert.Contains(t, s.RawMessageSets, uint16(0x8274))
}

// TestOutdoorFieldRegistry_ValueSizesConsistent 는 레지스트리의 각 인덱스 kind 별 기대 값
// 크기가 프레임 파서의 니블 규칙(MessageSetValueSize)과 일치함을 검증한다(디코드 정합성).
func TestOutdoorFieldRegistry_ValueSizesConsistent(t *testing.T) {
	kindSize := map[outdoorFieldKind]int{
		oduTempSigned: 2,
		oduEnum:       1,
		oduVarRaw:     2,
		oduLvarRaw:    4,
	}
	for index, field := range outdoorFieldRegistry {
		wantSize := kindSize[field.Kind]
		gotSize := MessageSetValueSize(index)
		require.Equal(t, wantSize, gotSize,
			"index 0x%04X (%s): kind 기대 크기 %d 가 니블 규칙 %d 와 불일치",
			index, field.Name, wantSize, gotSize)
	}
}
