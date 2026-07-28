package airpurifier

import "time"

// 상태 축별 observed bitmask (samsung observedCore 패턴). 관측 기반 emit 에 사용.
const (
	observedPower    uint8 = 1 << 0 // 0x01
	observedFanSpeed uint8 = 1 << 1 // 0x02
	observedOnline   uint8 = 1 << 2 // 0x04
)

// Device 는 공기청정기 디바이스 로스터 항목이다 (REQ-AIRPUR-001-02-01).
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

	// observed 는 각 축(power/fan_speed/online)의 관측 여부 bitmask 이다 (관측 기반 emit).
	// 디바이스 상태가 페이로드로 한 번이라도 관측된 축의 bit 만 set 되며, StateForJSON 이
	// set 된 축만 emit 한다 (samsung observedCore 패턴, REQ-AIRPUR-001-05-02).
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
// 스켈레톤이다 (REQ-AIRPUR-001-05-02).
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
func (d *Device) clone() Device { return *d }
