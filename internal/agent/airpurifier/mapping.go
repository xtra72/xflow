package airpurifier

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// 토픽 템플릿 placeholder 이름 상수 (다중 필드 주소 지정, M14).
//
// 토픽 템플릿은 "/" 로 분리된 세그먼트로 구성되며, 각 세그먼트는 리터럴이거나
// "{name}" 형태의 placeholder 이다. placeholder 는 세그먼트 전체를 차지한다
// (부분 세그먼트 "ap-{device_id}" 는 리터럴로 취급). 아래 이름들은 로스터 Device 의
// 필드로 양방향 매핑되는 표준 placeholder 이름이다.
const (
	placeholderDeviceID    = "device_id"    // → Device.DeviceID (하위호환 단일 필드)
	placeholderStationCode = "station_code" // → Device.Station
	placeholderPlaceCode   = "place_code"   // → Device.Place
	placeholderDeviceIndex = "device_index" // → Device.Index
	placeholderAttribute   = "attribute"    // 상태 축 이름(power/fan_speed/online), 주소가 아님
)

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

// ---------------------------------------------------------------------------
// 다중 필드 토픽 주소 지정 (M14)
// ---------------------------------------------------------------------------
//
// 토픽 seam 은 하나의 parse/render 쌍으로 통일된다: {device_id} 단일 필드 모델과
// {station_code}/{place_code}/{device_index}/{attribute} 다중 필드 모델을 모두 지원하며,
// {attribute} placeholder 의 존재 여부로 디코드 모드를 디스패치한다. placeholder 는
// 세그먼트 단위("{name}" 전체 세그먼트)이며 리터럴 세그먼트는 정확히 일치해야 한다.

// placeholderOf 는 세그먼트가 "{name}" 형태면 name 과 true 를, 아니면 "", false 를 반환한다.
func placeholderOf(segment string) (string, bool) {
	if len(segment) >= 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
		return segment[1 : len(segment)-1], true
	}
	return "", false
}

// placeholderNames 는 템플릿의 placeholder 이름을 템플릿 순서대로 반환한다.
func placeholderNames(template string) []string {
	var names []string
	for _, seg := range strings.Split(template, "/") {
		if n, ok := placeholderOf(seg); ok {
			names = append(names, n)
		}
	}
	return names
}

// templateHasAttribute 는 템플릿에 {attribute} placeholder 가 있는지 반환한다
// (attribute-per-topic 디코드/인코드 모드 디스패치의 기준).
func templateHasAttribute(template string) bool {
	for _, n := range placeholderNames(template) {
		if n == placeholderAttribute {
			return true
		}
	}
	return false
}

// templateIsComposite 는 템플릿이 합성 주소 모델(다중 필드)인지 반환한다. 비-attribute
// placeholder 가 없거나 {device_id} 단독이면 false(blob/{device_id} 모델), 그 외(station_code/
// place_code/device_index 등)면 true. seedKeyAndAddress 의 키 판별 규칙과 동형이다.
func templateIsComposite(template string) bool {
	var nonAttr []string
	for _, n := range placeholderNames(template) {
		if n != placeholderAttribute {
			nonAttr = append(nonAttr, n)
		}
	}
	if len(nonAttr) == 0 {
		return false
	}
	if len(nonAttr) == 1 && nonAttr[0] == placeholderDeviceID {
		return false
	}
	return true
}

// buildSubscriptionTopic 은 템플릿의 모든 {...} placeholder 세그먼트를 MQTT 단일 레벨
// 와일드카드 "+" 로 치환한다. 템플릿당 단 하나의 와일드카드 구독을 산출하여 디바이스별
// 렌더 구독을 대체한다 (예: state/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}
// → state/ui-line/+/+/bse9000/+/+).
func buildSubscriptionTopic(template string) string {
	segs := strings.Split(template, "/")
	for i, seg := range segs {
		if _, ok := placeholderOf(seg); ok {
			segs[i] = "+"
		}
	}
	return strings.Join(segs, "/")
}

// parseTopic 은 실제 토픽을 템플릿과 세그먼트 단위로 대조하여 placeholder→값 맵을 추출한다.
// 리터럴 세그먼트가 정확히 일치하지 않거나 세그먼트 수가 다르면 (nil, false)를 반환한다
// (우리 소유가 아닌 토픽 → 무시). placeholder 세그먼트는 실제 값을 캡처한다.
func parseTopic(template, actual string) (map[string]string, bool) {
	ts := strings.Split(template, "/")
	as := strings.Split(actual, "/")
	if len(ts) != len(as) {
		return nil, false
	}
	out := make(map[string]string, len(ts))
	for i, seg := range ts {
		if n, ok := placeholderOf(seg); ok {
			out[n] = as[i]
			continue
		}
		if seg != as[i] {
			return nil, false // 리터럴 불일치 → 우리 소유 아님
		}
	}
	return out, true
}

// renderTopic 은 템플릿의 각 {name} placeholder 세그먼트를 fields[name] 으로 치환한다
// (REQ-AIRPUR-001-01-05, 다중 필드 일반화). {device_id} 단일 필드 템플릿에는
// fields={"device_id": id} 를 넘겨 하위호환을 유지한다. 토픽 스킴은 하드코딩하지 않고
// 설정 템플릿으로만 산출한다.
func renderTopic(template string, fields map[string]string) string {
	segs := strings.Split(template, "/")
	for i, seg := range segs {
		if n, ok := placeholderOf(seg); ok {
			segs[i] = fields[n]
		}
	}
	return strings.Join(segs, "/")
}

// synthesizeAddress 는 {attribute} 를 제외한 모든 placeholder 값을 템플릿 순서대로 ":" 로
// 이어 로스터 키(합성 device_id)를 만든다. {device_id} 단일 필드 템플릿에서는 device_id
// 자체가 된다 (정확한 하위호환).
func synthesizeAddress(template string, fields map[string]string) string {
	var parts []string
	for _, n := range placeholderNames(template) {
		if n == placeholderAttribute {
			continue
		}
		parts = append(parts, fields[n])
	}
	return strings.Join(parts, ":")
}

// compositeKey 는 위치 계층 주소를 정규화한 보조 인덱스 키를 만든다: "{station}:{place}:{index}".
// index 는 int 로 정규화(선행 0 무시)되므로 토픽의 ".../3/..." 와 ".../003/..." 가 같은 키로
// 귀결된다 — 보조 인덱스(compositeKey → device_id/UUID) 조회의 안정성을 보장한다.
func compositeKey(station, place string, index int) string {
	return station + ":" + place + ":" + strconv.Itoa(index)
}

// compositeKeyFromFields 는 파싱된 토픽 placeholder 로부터 정규화 보조 인덱스 키를 만든다.
// device_index 는 int 로 정규화한다(compositeKey 와 동일 규칙).
func compositeKeyFromFields(fields map[string]string) string {
	return compositeKey(fields[placeholderStationCode], fields[placeholderPlaceCode], toInt(fields[placeholderDeviceIndex]))
}

// composeName 은 표시용 Name 을 위치 계층으로 합성한다: "{station}:{place}:{index-3자리-0채움}".
// compositeKey 와 달리 index 를 3자리로 0-채움한다(예: 3 → "003") — 사람이 읽는 표시 규약이며,
// 보조 인덱스 키(정규화 int)와는 목적이 다르다(표시 vs 매칭).
func composeName(station, place string, index int) string {
	return fmt.Sprintf("%s:%s:%03d", station, place, index)
}

// nonAttrFields 는 {attribute} 를 제외한 placeholder 값 맵의 복사본을 반환한다
// (Device.Address 저장 / 명령 렌더 재구성용).
func nonAttrFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if k == placeholderAttribute {
			continue
		}
		out[k] = v
	}
	return out
}

// scalarWire 는 attribute-per-topic 의 원시 스칼라 페이로드 바이트를 wire 값으로 해석한다.
// JSON 스칼라(true / 2 / "on")면 그 값으로, 아니면 원시 문자열("on")로 본다.
func scalarWire(payload []byte) any {
	var v any
	if err := json.Unmarshal(payload, &v); err == nil {
		return v
	}
	return string(payload)
}

// scalarBytes 는 wire 값을 attribute-per-topic 의 원시 스칼라 페이로드 바이트로 인코딩한다.
// 문자열은 그대로(따옴표 없이), bool/정수는 그 문자열 표현으로 발행한다.
func scalarBytes(v any) []byte {
	switch t := v.(type) {
	case string:
		return []byte(t)
	case bool:
		if t {
			return []byte("true")
		}
		return []byte("false")
	case nil:
		return []byte("")
	default:
		return []byte(fmt.Sprintf("%v", t))
	}
}

// decodeAttributeScalar 는 attribute 토큰이 지칭하는 상태 축의 스칼라 페이로드를 디코딩하여
// 해당 축만 *Set=true 인 decodedState 를 반환한다 (attribute-per-topic 모드). attribute 가
// 어떤 축과도 일치하지 않으면 ok=false (알 수 없는 attribute → 무시).
func (m PayloadMapping) decodeAttributeScalar(attribute string, payload []byte) (decodedState, bool) {
	wire := scalarWire(payload)
	var st decodedState
	switch attribute {
	case m.Power.Name:
		st.Power = m.Power.decodeBool(wire)
		st.PowerSet = true
		return st, true
	case m.FanSpeed.Name:
		st.FanSpeed = m.FanSpeed.decodeInt(wire)
		st.FanSpeedSet = true
		return st, true
	default:
		if m.Online != nil && attribute == m.Online.Name {
			st.Online = m.Online.decodeBool(wire)
			st.OnlineSet = true
			return st, true
		}
		return decodedState{}, false
	}
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
