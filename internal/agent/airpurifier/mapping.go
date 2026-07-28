package airpurifier

import (
	"encoding/json"
	"fmt"
	"strings"
)

// deviceIDPlaceholder 는 토픽 템플릿에서 device_id 로 치환되는 placeholder 이다.
const deviceIDPlaceholder = "{device_id}"

// PayloadMapping 은 설정 주도 페이로드 시임이다 (REQ-AIRPUR-001-01-06).
//
// 디바이스 상태 JSON 의 어느 필드가 각 상태 축(power/fan_speed/online)을 나르는지와
// 각 축 값의 wire 표현(bool/문자열/정수)을 선언한다. direct·port 두 모드가 이 매핑을
// 공유하여 상태 디코딩(REQ-05-02)과 제어 인코딩(REQ-03-04)의 결과가 트랜스포트 경계와
// 무관하게 바이트 단위로 동일하다 (REQ-01-11).
type PayloadMapping struct {
	// Power 는 전원 축(bool)의 필드명 + on/off wire 표현이다 (필수).
	Power BoolField
	// FanSpeed 는 풍량 축(1/2/3)의 필드명 + 값 표현이다 (필수).
	FanSpeed IntField
	// Online 은 온라인 축(bool)의 필드명 + 표현이다 (선택). nil 이면 LWT/타임아웃 기반 판정.
	Online *BoolField
}

// BoolField 는 boolean 상태 축(power/online)을 JSON 필드 + on/off wire 값으로 매핑한다.
type BoolField struct {
	// Name 은 이 축을 나르는 JSON 필드 키이다.
	Name string
	// OnValue / OffValue 는 true/false 의 wire 표현이다 (문자열 "on"/"off", 정수 1/0 등).
	// 둘 다 nil 이면 native JSON boolean 을 사용한다.
	OnValue  any
	OffValue any
}

// IntField 는 정수 상태 축(fan_speed)을 JSON 필드 + 값 테이블로 매핑한다.
type IntField struct {
	// Name 은 이 축을 나르는 JSON 필드 키이다.
	Name string
	// Values 는 도메인 정수(fan_speed 1/2/3) → wire 표현 매핑이다 (인코딩 방향).
	// 비어 있으면 native 정수 값을 그대로 사용한다.
	Values map[int]any
}

// IsValid 는 필수 필드(power/fan_speed 필드명)가 채워졌는지 반환한다.
func (m PayloadMapping) IsValid() bool {
	return m.Power.Name != "" && m.FanSpeed.Name != ""
}

// commandPayload 는 제어 명령으로 설정할 축을 나타낸다. nil 포인터는 "미설정"(해당 축을
// 페이로드에 포함하지 않음)을 의미한다. 설정된 축만 인코딩되어 두 모드에서 바이트 동일하다.
type commandPayload struct {
	Power    *bool
	FanSpeed *int
}

// decodedState 는 상태 페이로드 디코딩 결과이다. *Set 플래그는 해당 축이 페이로드에
// 존재했는지(관측 여부)를 나타내며, 관측 기반 emit(REQ-05-02)의 입력이 된다.
type decodedState struct {
	Power       bool
	FanSpeed    int
	Online      bool
	PowerSet    bool
	FanSpeedSet bool
	OnlineSet   bool
}

// renderTopic 은 토픽 템플릿의 {device_id} placeholder 를 deviceID 로 치환한다
// (REQ-AIRPUR-001-01-05). 토픽 스킴은 하드코딩하지 않고 설정 템플릿으로만 산출한다.
func renderTopic(template, deviceID string) string {
	return strings.ReplaceAll(template, deviceIDPlaceholder, deviceID)
}

// encodeBool 은 boolean 축 값을 wire 표현으로 인코딩한다.
// OnValue/OffValue 가 지정돼 있으면 그 값을, 아니면 native bool 을 사용한다.
func (f BoolField) encodeBool(v bool) any {
	if v {
		if f.OnValue != nil {
			return f.OnValue
		}
		return true
	}
	if f.OffValue != nil {
		return f.OffValue
	}
	return false
}

// decodeBool 은 wire 값을 boolean 축 값으로 디코딩한다.
// OnValue/OffValue 가 지정돼 있으면 문자열 동등성으로 판정하고, 아니면 truthy 판정한다.
func (f BoolField) decodeBool(wire any) bool {
	if f.OnValue != nil || f.OffValue != nil {
		if f.OnValue != nil && equalWire(wire, f.OnValue) {
			return true
		}
		if f.OffValue != nil && equalWire(wire, f.OffValue) {
			return false
		}
		// 매핑에 없는 값: OnValue 와 동등하지 않으면 false 로 본다.
		return false
	}
	return truthy(wire)
}

// encodeInt 는 정수 축 값을 wire 표현으로 인코딩한다.
// Values 매핑에 키가 있으면 그 값을, 아니면 native 정수를 사용한다.
func (f IntField) encodeInt(v int) any {
	if len(f.Values) > 0 {
		if wire, ok := f.Values[v]; ok {
			return wire
		}
	}
	return v
}

// decodeInt 는 wire 값을 정수 축 값으로 디코딩한다.
// Values 매핑이 있으면 역방향 조회(wire→도메인)하고, 없으면 정수로 강제 변환한다.
func (f IntField) decodeInt(wire any) int {
	if len(f.Values) > 0 {
		for domain, w := range f.Values {
			if equalWire(wire, w) {
				return domain
			}
		}
	}
	return toInt(wire)
}

// encodeCommandPayload 는 제어 명령을 디바이스 명령 페이로드(JSON)로 인코딩한다.
//
// 설정된 축만 포함되며 Go 의 map JSON 마샬링은 키를 정렬하므로 결과가 결정적이다.
// direct(브로커 발행)·port(제어 출력 포트) 두 모드가 이 함수를 공유하여 바이트 동일한
// 페이로드를 산출한다 (REQ-AIRPUR-001-01-11, Scenario 1B.8).
func encodeCommandPayload(m PayloadMapping, cmd commandPayload) ([]byte, error) {
	out := make(map[string]any, 2)
	if cmd.Power != nil {
		out[m.Power.Name] = m.Power.encodeBool(*cmd.Power)
	}
	if cmd.FanSpeed != nil {
		out[m.FanSpeed.Name] = m.FanSpeed.encodeInt(*cmd.FanSpeed)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("airpurifier: encode command payload: %w", err)
	}
	return b, nil
}

// decodeStatePayload 는 유입 상태 페이로드(JSON)를 상태 축으로 디코딩한다 (REQ-05-02).
//
// 페이로드에 존재한 축만 *Set=true 로 표시하여 관측 기반 emit 을 지원한다. direct(구독
// 콜백)·port(입력 포트) 두 모드가 동일한 이 경로를 거친다 (REQ-AIRPUR-001-01-11).
func decodeStatePayload(m PayloadMapping, raw []byte) (decodedState, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return decodedState{}, fmt.Errorf("airpurifier: decode state payload: %w", err)
	}
	var st decodedState
	if v, ok := obj[m.Power.Name]; ok {
		st.Power = m.Power.decodeBool(v)
		st.PowerSet = true
	}
	if v, ok := obj[m.FanSpeed.Name]; ok {
		st.FanSpeed = m.FanSpeed.decodeInt(v)
		st.FanSpeedSet = true
	}
	if m.Online != nil {
		if v, ok := obj[m.Online.Name]; ok {
			st.Online = m.Online.decodeBool(v)
			st.OnlineSet = true
		}
	}
	return st, nil
}

// equalWire 는 두 wire 값을 타입에 관대하게 비교한다.
// JSON 파싱은 숫자를 float64 로 전달하므로 문자열화 비교로 정수/실수/문자열을 통일한다.
func equalWire(a, b any) bool {
	if a == b {
		return true
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// truthy 는 매핑이 없는 boolean 축의 wire 값을 진리값으로 해석한다.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "on", "true", "1", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
