package airpurifier

import "errors"

// Sentinel errors for the air purifier agent (REQ-AIRPUR-001-09-01).
//
// 모두 errors.New 로 생성되어 errors.Is 비교와 호환된다. 설정/제어/로스터/역사
// 레지스트리/트랜스포트 각 실패 지점이 이 중 하나를 반환한다.
var (
	// ErrInvalidTransportMode 는 transport_mode 가 direct/port 이외일 때 반환된다.
	ErrInvalidTransportMode = errors.New("airpurifier: invalid transport_mode (must be 'direct' or 'port')")
	// ErrBrokerRequired 는 direct 모드에서 broker 가 비어 있을 때 반환된다.
	ErrBrokerRequired = errors.New("airpurifier: broker is required in direct mode")
	// ErrInvalidTopicTemplate 는 토픽 템플릿에 {placeholder} 가 하나도 없을 때 반환된다
	// ({device_id} 를 강제하지 않는다 — 다중 필드 템플릿 지원, M14).
	ErrInvalidTopicTemplate = errors.New("airpurifier: topic template must contain at least one {placeholder}")
	// ErrInvalidPayloadMapping 은 payload_mapping 이 없거나 power/fan_speed 필드가 누락됐을 때 반환된다.
	ErrInvalidPayloadMapping = errors.New("airpurifier: payload_mapping must define power_field and fan_speed_field")
	// ErrDeviceNotFound 는 device_id 로 조회한 디바이스가 로스터에 없을 때 반환된다.
	ErrDeviceNotFound = errors.New("airpurifier: device not found")
	// ErrDeviceAlreadyRegistered 는 이미 존재하는 device_id 를 다시 등록할 때 반환된다.
	ErrDeviceAlreadyRegistered = errors.New("airpurifier: device already registered")
	// ErrConfigDeviceProtected 는 Source="config" 디바이스 삭제 시도 시 반환된다.
	ErrConfigDeviceProtected = errors.New("airpurifier: config-based device cannot be removed")
	// ErrInvalidFanSpeed 는 풍량 값이 허용 범위(1/2/3)를 벗어날 때 반환된다.
	ErrInvalidFanSpeed = errors.New("airpurifier: invalid fan speed (must be 1, 2, or 3)")
	// ErrPowerOff 는 전원이 꺼진 디바이스에 전원 외 제어를 시도할 때 반환된다.
	ErrPowerOff = errors.New("airpurifier: device is powered off, only power control is allowed")
	// ErrEmptyGroup 은 fan-out 대상 그룹/셀렉터에 멤버가 없을 때 반환된다.
	ErrEmptyGroup = errors.New("airpurifier: group has no members")
	// ErrControlTimeout 은 제어 명령 후 상태 에코를 control_response_timeout 내에 받지 못했을 때 반환된다.
	ErrControlTimeout = errors.New("airpurifier: control response timeout")
	// ErrStationNotFound 는 역사 레지스트리에서 station 을 찾지 못했을 때 반환된다.
	ErrStationNotFound = errors.New("airpurifier: station not found")
	// ErrLineNotFound 는 역사 레지스트리에서 line 에 속한 station 이 없을 때 반환된다.
	ErrLineNotFound = errors.New("airpurifier: line not found")
	// ErrPlaceNotFound 는 역사 내에서 place 를 찾지 못했을 때 반환된다 (remove_place/get_place).
	ErrPlaceNotFound = errors.New("airpurifier: place not found")
	// ErrNotConnected 는 direct 모드에서 브로커에 연결되지 않은 상태로 발행을 시도할 때 반환된다.
	ErrNotConnected = errors.New("airpurifier: not connected to broker")
	// ErrInvalidCommand 은 알 수 없는/미구현 Process 명령이 수신됐을 때 반환된다.
	ErrInvalidCommand = errors.New("airpurifier: invalid command")
	// ErrInvalidLivenessSource 는 liveness_source 가 receive/payload 이외일 때 반환된다.
	ErrInvalidLivenessSource = errors.New("airpurifier: invalid liveness_source (must be 'receive' or 'payload')")
)
