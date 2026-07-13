package samsung

import (
	"encoding/binary"
)

// ---------------------------------------------------------------------------
// 실외기(ODU) 상태 디코딩
//
// 실외기는 자기 상태를 버스에 Notification(C0 14)으로 브로드캐스트하므로, SA 가
// 10 xx 00(실외기)인 프레임의 Message Set 을 파싱하면 별도 요청 없이 수동 수집할 수
// 있다. 실내기 상태(0x40xx/0x42xx)와 달리 실외기 상태는 0x80xx(ENUM 1byte),
// 0x82xx(VAR 2byte), 0x84xx(LVAR 4byte) 및 0x24FC(전압, LVAR) 대역에 위치한다.
//
// 아래 인덱스 정의는 references/protocols/samsung_nasa_protocol.md "실외기 상태 정보"
// 를 근거로 하며, 그 출처는 Samsung SNET Pro 디컴파일 기반 오픈소스
// (esphome_samsung_hvac_bus, pysamsungnasa) 리버스 엔지니어링이다. 모델별로 존재
// 여부·의미·스케일이 다를 수 있어 실측 검증이 필요하다 — 온도는 /10 로 확인되나,
// 전류·전압·전력은 raw 값으로만 전달되어 나눗셈 계수·단위가 미확인이므로 raw 정수로
// 노출한다.
// ---------------------------------------------------------------------------

// outdoorFieldKind 는 실외기 메시지 셋 값의 디코드 방식을 나타낸다.
type outdoorFieldKind uint8

const (
	oduTempSigned outdoorFieldKind = iota // VAR 2B, 부호 있음, /10 → °C (float32)
	oduEnum                               // ENUM 1B → int
	oduVarRaw                             // VAR 2B → int (raw, 부호 없음)
	oduLvarRaw                            // LVAR 4B → int64 (raw, 부호 없음)
)

// outdoorField 는 실외기 인덱스의 이름·디코드 방식·이산 여부를 정의한다.
//
// Discrete=true 인 필드(운전상태·압축기 On/Off·에러코드 등 이산 ENUM)는 값이 바뀌면
// 즉시 device_state 를 emit 한다. false 인 연속 센서(온도·주파수·전류·전력)는 정기
// 보고(report_interval) 주기에만 실려 매 프레임 변동으로 인한 emit 폭주를 방지한다.
type outdoorField struct {
	Name     string
	Kind     outdoorFieldKind
	Discrete bool
}

// outdoorFieldRegistry 는 문서화된 실외기 상태 인덱스 → 필드 정의 맵이다.
var outdoorFieldRegistry = map[uint16]outdoorField{
	// ── 온도 센서 (VAR 2B, 부호 있음, /10 = °C) ──
	// outdoor_temperature / compressor_discharge_temperature 는 LG HVACR 필드명과 통일(의미 동일).
	0x8204: {"outdoor_temperature", oduTempSigned, false},              // 실외 외기(대기) 온도 (LG 통일)
	0x8280: {"compressor_discharge_temperature", oduTempSigned, false}, // 압축기 토출(Discharge) 온도 (LG 통일)
	0x8261: {"out_sensor_pipein3", oduTempSigned, false},               // 열교환기 파이프 입구 온도
	0x8262: {"out_sensor_pipein4", oduTempSigned, false},
	0x8263: {"out_sensor_pipein5", oduTempSigned, false},
	0x8264: {"out_sensor_pipeout1", oduTempSigned, false}, // 파이프 출구 온도
	0x8265: {"out_sensor_pipeout2", oduTempSigned, false},
	0x8266: {"out_sensor_pipeout3", oduTempSigned, false},
	0x8267: {"out_sensor_pipeout4", oduTempSigned, false},
	0x8268: {"out_sensor_pipeout5", oduTempSigned, false},

	// ── 압축기 / 운전 상태 (ENUM 1B, 이산) ──
	0x8001: {"out_operation_odu_mode", oduEnum, true}, // 실외기 운전 상태 (0=정지, 2=정상, 5=제상 등)
	0x8003: {"out_operation_heatcool", oduEnum, true}, // 냉방/난방 (1=냉방, 2=난방)
	0x8010: {"out_load_comp1", oduEnum, true},         // 압축기1 On/Off
	0x8011: {"out_load_comp2", oduEnum, true},         // 압축기2 On/Off
	0x8012: {"out_load_comp3", oduEnum, true},         // 압축기3 On/Off
	0x801A: {"out_load_4way", oduEnum, true},          // 4-way(사방) 밸브 On/Off
	0x8061: {"out_deice_step", oduEnum, true},         // 제상(defrost) 단계

	// ── 압축기 주파수 (VAR 2B, raw Hz, 연속) ──
	0x8274: {"out_control_order_cfreq_comp2", oduVarRaw, false},  // 압축기2 지령 주파수
	0x8275: {"out_control_target_cfreq_comp2", oduVarRaw, false}, // 압축기2 목표 주파수

	// ── 전기 / 전력 ──
	0x8217: {"out_sensor_ct1", oduVarRaw, false},            // 실외기 전류 (CT1), VAR 2B raw
	0x82DB: {"out_phase_current", oduVarRaw, false},         // 상(Phase) 전류, VAR 2B raw
	0x8235: {"error_code", oduVarRaw, true},                 // 실외기 에러 코드 (이산), VAR 2B (LG/실내기 통일)
	0x24FC: {"out_sensor_voltage", oduLvarRaw, false},       // 공급 전압, LVAR 4B raw
	0x8413: {"wattmeter_1min_sum", oduLvarRaw, false},       // 순시 소비전력, LVAR 4B raw
	0x8414: {"wattmeter_all_unit_accum", oduLvarRaw, false}, // 누적 전력량, LVAR 4B raw
}

// decodeOutdoorValue 는 kind 에 따라 raw 바이트를 디코드한다.
// 값 길이가 부족하거나 "미장착/무효" 센티넬(전부 0xFF)이면 ok=false 를 반환한다.
//
// 센티넬 처리: 미장착 압축기/센서는 NASA 에서 값 폭 전체를 0xFF 로 보고한다
// (예: 단일 압축기 유닛의 comp2 지령/목표 주파수 0xFFFF=65535). 이를 그대로 노출하면
// 65535 Hz 같은 허위 값이 나가므로, raw 정수 필드(ENUM 1B=0xFF, VAR 2B=0xFFFF,
// LVAR 4B=0xFFFFFFFF)는 센티넬을 무효로 간주해 Fields 에 싣지 않는다(관측 안 됨과 동일).
//
// 온도(oduTempSigned)는 부호 있는 값이라 0xFFFF(-0.1°C)가 정상 저온 판독일 수 있어
// 센티넬 마스킹을 적용하지 않는다 — 실제 음수 온도를 누락하지 않기 위함이다.
func decodeOutdoorValue(kind outdoorFieldKind, v []byte) (any, bool) {
	switch kind {
	case oduTempSigned:
		if len(v) < 2 {
			return nil, false
		}
		return DecodeTemperature(binary.BigEndian.Uint16(v[:2])), true
	case oduEnum:
		if len(v) < 1 {
			return nil, false
		}
		if v[0] == 0xFF {
			return nil, false // 미장착/무효 센티넬
		}
		return int(v[0]), true
	case oduVarRaw:
		if len(v) < 2 {
			return nil, false
		}
		raw := binary.BigEndian.Uint16(v[:2])
		if raw == 0xFFFF {
			return nil, false // 미장착/무효 센티넬
		}
		return int(raw), true
	case oduLvarRaw:
		if len(v) < 4 {
			return nil, false
		}
		raw := binary.BigEndian.Uint32(v[:4])
		if raw == 0xFFFFFFFF {
			return nil, false // 미장착/무효 센티넬
		}
		return int64(raw), true
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// OutdoorState 는 실외기(ODU)의 디코드된 상태이다.
//
// 실내기 NasaDeviceState 와 병렬 구조이며, 문서화된 out_* 인덱스만 디코드해 보관한다.
// 관측 기반 emit("확인된 값만 전송"): 한 번이라도 수신된 필드만 Fields 에 담기며,
// StateForJSON 이 그 필드만 출력한다.
// ---------------------------------------------------------------------------
type OutdoorState struct {
	// Fields 는 관측된 실외기 필드의 디코드 값이다(name → value).
	// 온도는 float32(°C), 그 외는 정수 raw 값(int / int64)이다.
	Fields map[string]any

	// discreteFields 는 이산 필드(ENUM·에러코드)의 최신 값 스냅샷이다(변화 감지 전용).
	discreteFields map[string]any

	// RawMessageSets 는 수신된 모든 실외기 메시지 세트의 원본 바이트이다.
	RawMessageSets HexKeyByteMap
}

// NewOutdoorState 는 초기화된 OutdoorState 를 생성한다.
func NewOutdoorState() *OutdoorState {
	return &OutdoorState{
		Fields:         make(map[string]any),
		discreteFields: make(map[string]any),
		RawMessageSets: make(HexKeyByteMap),
	}
}

// UpdateFromMessageSets 는 실외기 메시지 셋을 디코드해 Fields 를 갱신한다.
//
// 이산 필드(Discrete=true) 중 하나라도 값이 새로 관측되거나 바뀌면 discreteChanged=true
// 를 반환한다(즉시 emit gate). 연속 센서는 Fields 만 갱신하고 emit 을 트리거하지 않아
// 정기 보고 주기에만 실린다. 인식되지 않은 인덱스도 RawMessageSets 에는 보존한다.
func (s *OutdoorState) UpdateFromMessageSets(sets []NasaMessageSet) (discreteChanged bool) {
	for _, ms := range sets {
		raw := make([]byte, len(ms.Value))
		copy(raw, ms.Value)
		s.RawMessageSets[ms.Index] = raw

		field, ok := outdoorFieldRegistry[ms.Index]
		if !ok {
			continue
		}
		val, ok := decodeOutdoorValue(field.Kind, ms.Value)
		if !ok {
			continue
		}
		s.Fields[field.Name] = val
		if field.Discrete {
			if prev, existed := s.discreteFields[field.Name]; !existed || prev != val {
				discreteChanged = true
			}
			s.discreteFields[field.Name] = val
		}
	}
	return discreteChanged
}

// StateForJSON 은 관측된 실외기 필드를 담은 map 을 반환한다.
// includeRaw=true 이고 수신된 메시지 세트가 있으면 raw_message_sets 도 포함한다.
func (s *OutdoorState) StateForJSON(includeRaw bool) any {
	out := make(map[string]any, len(s.Fields)+1)
	for k, v := range s.Fields {
		out[k] = v
	}
	if includeRaw && len(s.RawMessageSets) > 0 {
		out["raw_message_sets"] = s.RawMessageSets
	}
	return out
}
