package lg

import "errors"

var (
	// ErrChecksumMismatch 는 체크섬 검증 실패 시 반환된다.
	ErrChecksumMismatch = errors.New("lgap: 체크섬 불일치")

	// ErrDeviceNotFound 는 장치를 찾을 수 없을 때 반환된다.
	ErrDeviceNotFound = errors.New("lgap: 장치를 찾을 수 없음")

	// ErrDeviceIDNotFound 는 device_id 를 찾을 수 없을 때 반환된다.
	ErrDeviceIDNotFound = errors.New("lgap: device_id 를 찾을 수 없음")

	// ErrDeviceAlreadyRegistered 는 장치가 이미 등록되어 있을 때 반환된다.
	ErrDeviceAlreadyRegistered = errors.New("lgap: 장치가 이미 등록됨")

	// ErrDuplicateDeviceID 는 중복된 device_id 가 발견되었을 때 반환된다.
	ErrDuplicateDeviceID = errors.New("lgap: 중복된 device_id")

	// ErrConfigDeviceProtected 는 설정 파일에서 정의된 장치를 제거하려 할 때 반환된다.
	ErrConfigDeviceProtected = errors.New("lgap: 설정 기반 장치는 제거할 수 없음")

	// ErrDeviceOffline 은 장치가 오프라인 상태일 때 반환된다.
	ErrDeviceOffline = errors.New("lgap: 장치가 오프라인 상태")

	// ErrInvalidCommand 는 잘못된 명령이 전달되었을 때 반환된다.
	ErrInvalidCommand = errors.New("lgap: 잘못된 명령")

	// ErrInvalidMode 는 잘못된 운전 모드가 전달되었을 때 반환된다.
	ErrInvalidMode = errors.New("lgap: 잘못된 운전 모드")

	// ErrInvalidFanSpeed 는 잘못된 팬 속도가 전달되었을 때 반환된다.
	ErrInvalidFanSpeed = errors.New("lgap: 잘못된 팬 속도")

	// ErrTemperatureOutOfRange 는 온도가 범위(16-30도)를 벗어났을 때 반환된다.
	ErrTemperatureOutOfRange = errors.New("lgap: 온도 범위 초과 (16-30)")

	// ErrInvalidZone 은 잘못된 존 주소가 전달되었을 때 반환된다.
	ErrInvalidZone = errors.New("lgap: 잘못된 존 주소")

	// ErrNoResponse 는 장치로부터 응답이 없을 때 반환된다.
	ErrNoResponse = errors.New("lgap: 장치 응답 없음")

	// ErrResponseError 는 장치가 에러를 반환했을 때 반환된다.
	ErrResponseError = errors.New("lgap: 장치가 에러를 반환함")

	// ErrTransportNotConnected 는 트랜스포트가 연결되지 않은 상태에서 전송/수신을 시도할 때 반환된다.
	ErrTransportNotConnected = errors.New("lgap: 트랜스포트 미연결")
)
