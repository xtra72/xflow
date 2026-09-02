package xsfm

import (
	"strconv"
	"time"
)

// 상태 축별 observed bitmask (samsung observedCore 패턴). 관측 기반 emit 에 사용.
const (
	observedPower    uint8 = 1 << 0 // 0x01
	observedFanSpeed uint8 = 1 << 1 // 0x02
	observedOnline   uint8 = 1 << 2 // 0x04
)

// Device 는 설비 디바이스 로스터 항목이다 (REQ-XSFM-001-02-01).
//
// 위치 계층 속성(Station/Place/Index)은 선택/하위호환이며 미지정 시 빈 값/0 이다 (A-9).
// line(호선)은 디바이스에 저장하지 않고 Station 을 역사 레지스트리로 조회해 해석한다 (A-10).
type Device struct {
	DeviceID string    // 디바이스 식별자 (로스터 기본 키)
	Name     string    // 표시 이름
	GroupID  string    // 선택적 그룹 식별자 (빈 값이면 미소속)
	Station  string    // (선택) 소속 역사 식별자 — 역사 레지스트리 참조
	Place    string    // (선택) 역사 내 위치/구역
	Index    int       // (선택) station(+place) 범위 내 순번 (전역 유일 아님)
	Power    bool      // 전원 상태 (관측된 경우)
	FanSpeed int       // 풍량 1/2/3 (power=ON 일 때만 유효)
	Online   bool      // 온라인 상태
	LastSeen time.Time // 마지막 상태 수신 시각
	Source   string    // 등록 출처: "config", "bridge", "auto"

	// composite 는 이 디바이스가 합성 주소 모델(device_id=UUID, Name=합성)에 속하는지 표시한다.
	// true 이면 Name 이 station/place/index 에서 파생되므로 station/place/index 변경 시 Name 을
	// 재계산한다. false(=blob/{device_id} 모델)이면 Name 은 사용자가 관리하므로 재계산하지 않는다.
	composite bool

	// nameOverridden 은 composite 디바이스의 Name 이 사용자 지정 커스텀 이름으로 고정(sticky)
	// 되었는지 표시한다. true 이면 station/place/index 가 바뀌어도 Name 을 파생값으로 재계산하지
	// 않아 커스텀 이름을 보존한다. 사용자가 명시적으로 빈 이름을 주면 다시 false 로 해제되어
	// composite 파생 이름으로 복귀한다. blob 디바이스에서는 의미 없음(Name 은 항상 사용자 관리).
	nameOverridden bool

	// Address 는 다중 필드 토픽 주소의 원시 placeholder→값 맵이다 ({attribute} 제외, M14).
	// 유입 토픽 파싱 또는 설정 시드에서 채워지며, 명령 토픽 재구성(renderTopic)에 사용한다.
	// {device_id} 단일 필드 모델에서는 {"device_id": id} 이거나 nil(이 경우 device 필드로 폴백).
	Address map[string]string

	// observed 는 각 축(power/fan_speed/online)의 관측 여부 bitmask 이다 (관측 기반 emit).
	// 디바이스 상태가 페이로드로 한 번이라도 관측된 축의 bit 만 set 되며, StateForJSON 이
	// set 된 축만 emit 한다 (samsung observedCore 패턴, REQ-XSFM-001-05-02).
	observed uint8
}

// markObserved 는 지정한 축 bit 를 관측됨으로 표시한다.
func (d *Device) markObserved(bit uint8) { d.observed |= bit }

// isObserved 는 지정한 축 bit 가 관측되었는지 반환한다.
func (d *Device) isObserved(bit uint8) bool { return d.observed&bit != 0 }

// StateForJSON 은 관측된 상태 축만 담은 JSON 직렬화용 map 을 반환한다.
//
// 관측 기반 emit("확인된 값만 전송"): 페이로드로 한 번이라도 관측된 축만 포함한다.
// 미관측 축은 zero-value 로 채워 보내지 않고 생략한다. power=off 이면 fan_speed 는
// 유효하지 않으므로 정규화 차원에서 생략한다 (samsung StateForJSON 패턴).
//
// 전체 emit 배선(메시지 포맷/트리거/타임스탬프)은 B5 에서 완성되며, 본 헬퍼는 관측 게이팅
// 스켈레톤이다 (REQ-XSFM-001-05-02).
func (d *Device) StateForJSON() map[string]any {
	out := make(map[string]any, 3)
	if d.isObserved(observedPower) {
		out["power"] = d.Power
	}
	// power 가 관측되고 off 이면 fan_speed 는 생략(정규화). on 이며 관측됐을 때만 emit.
	if d.isObserved(observedFanSpeed) && (!d.isObserved(observedPower) || d.Power) {
		out["fan_speed"] = d.FanSpeed
	}
	if d.isObserved(observedOnline) {
		out["online"] = d.Online
	}
	return out
}

// clone 은 Device 의 값 복사본을 반환한다 (로스터 조회가 내부 포인터를 노출하지 않도록).
// Address 맵은 얕은 공유를 피하기 위해 깊은 복사한다.
func (d *Device) clone() Device {
	c := *d
	if d.Address != nil {
		c.Address = make(map[string]string, len(d.Address))
		for k, v := range d.Address {
			c.Address[k] = v
		}
	}
	return c
}

// hasComposite 는 디바이스가 위치 계층 합성 주소(station/place/index 중 하나라도 비-zero)를
// 가지는지 반환한다. 보조 인덱스 등록 여부의 판별 기준이다 — 합성 주소가 없는 디바이스
// (blob/{device_id} 모델의 위치 미지정 디바이스)는 보조 인덱스에 넣지 않는다(빈 키 충돌 회피).
func hasComposite(d *Device) bool {
	return d.Station != "" || d.Place != "" || d.Index != 0
}

// applyAddressFields 는 파싱된 토픽 placeholder(station_code/place_code/device_index)를 Device
// 의 위치 필드로 역매핑한다. device_index 는 int 로 정규화한다(선행 0 무시).
//
// {line_code} 는 의도적으로 어떤 Device 필드에도 매핑하지 않는다(SPEC-XSFM-LINE-001 RD-8,
// REQ-08-05): 라인은 inbound 토픽이 아니라 역사(station)에서 파생하는 값이므로(station→line SSOT),
// 토픽의 {line_code} 를 식별·저장에 쓰면 SSOT 이중화가 된다. 디바이스 식별은 station_code+
// place_code+device_index 로만 이루어지고(compositeKeyFromFields), 라인은 station 파생으로 단일화한다.
func applyAddressFields(d *Device, fields map[string]string) {
	if v, ok := fields[placeholderStationCode]; ok {
		d.Station = v
	}
	if v, ok := fields[placeholderPlaceCode]; ok {
		d.Place = v
	}
	if v, ok := fields[placeholderDeviceIndex]; ok {
		d.Index = toInt(v)
	}
	// {line_code} 미매핑(REQ-08-05): 위 3필드 외 placeholder(라인 포함)는 Device 필드로 역매핑하지 않는다.
}

// deviceFieldValue 는 표준 placeholder 이름을 Device 필드 값으로 매핑한다 (양방향 매핑의
// 정방향). 알 수 없는 이름은 ok=false. 명령 토픽 렌더에서 Address 에 없는 placeholder 를
// 재구성할 때 폴백으로 쓴다.
func deviceFieldValue(name string, d *Device) (string, bool) {
	switch name {
	case placeholderDeviceID:
		return d.DeviceID, true
	case placeholderStationCode:
		return d.Station, true
	case placeholderPlaceCode:
		return d.Place, true
	case placeholderDeviceIndex:
		return strconv.Itoa(d.Index), true
	default:
		return "", false
	}
}
