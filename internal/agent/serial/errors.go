package serial

import (
	"errors"

	"github.com/xtra/xflow/pkg/framing"
)

// 시리얼 에이전트 패키지의 센티넬 에러 정의.
//
// 프레이밍 관련 에러 (ErrInvalidFraming, ErrMaxMessageSize, ErrChecksumMismatch,
// ErrETXMismatch, ErrFrameTooLarge) 는 공개 패키지 `pkg/framing` 으로 이동되었다.
// 시리얼 패키지는 하위 호환을 위해 동일 이름으로 해당 에러를 재노출한다.
// errors.Is 비교는 식별자 동일성을 기준으로 하므로, 재할당만으로 양쪽
// 패키지의 `errors.Is` 검사가 모두 성공한다.
var (
	// ErrPortRequired 는 포트 경로가 지정되지 않았을 때 반환된다.
	ErrPortRequired = errors.New("serial: port path is required")

	// ErrInvalidBaudRate 는 지원되지 않는 보드레이트일 때 반환된다.
	ErrInvalidBaudRate = errors.New("serial: invalid baud_rate")

	// ErrInvalidDataBits 는 지원되지 않는 데이터 비트일 때 반환된다.
	ErrInvalidDataBits = errors.New("serial: invalid data_bits")

	// ErrInvalidStopBits 는 지원되지 않는 스톱 비트일 때 반환된다.
	ErrInvalidStopBits = errors.New("serial: invalid stop_bits")

	// ErrInvalidParity 는 지원되지 않는 패리티일 때 반환된다.
	ErrInvalidParity = errors.New("serial: invalid parity")

	// ErrInvalidFraming 는 지원되지 않는 프레이밍 타입일 때 반환된다.
	// pkg/framing.ErrInvalidFraming 과 동일한 값이다.
	ErrInvalidFraming = framing.ErrInvalidFraming

	// ErrFixedSizeRequired 는 fixed_size 프레이밍에서 크기가 지정되지 않았을 때 반환된다.
	ErrFixedSizeRequired = errors.New("serial: fixed_size required when framing is fixed_size")

	// ErrPortNotFound 는 시리얼 포트를 찾을 수 없을 때 반환된다.
	ErrPortNotFound = errors.New("serial: port not found")

	// ErrPermissionDenied 는 시리얼 포트 접근 권한이 없을 때 반환된다.
	ErrPermissionDenied = errors.New("serial: permission denied")

	// ErrDeviceDisconnected 는 장치가 연결 해제되었을 때 반환된다.
	ErrDeviceDisconnected = errors.New("serial: device disconnected")

	// ErrPortClosed 는 포트가 이미 닫혀있을 때 반환된다.
	ErrPortClosed = errors.New("serial: port is closed")

	// ErrNotRunning 는 에이전트가 실행 중이지 않을 때 반환된다.
	ErrNotRunning = errors.New("serial: agent is not running")

	// ErrMaxMessageSize 는 메시지가 최대 크기를 초과했을 때 반환된다.
	// pkg/framing.ErrMaxMessageSize 과 동일한 값이다.
	ErrMaxMessageSize = framing.ErrMaxMessageSize

	// ErrInvalidSTX 는 STX 가 빈 문자열이거나 유효하지 않은 hex 일 때 반환된다.
	ErrInvalidSTX = errors.New("serial: invalid stx (must be non-empty hex string)")

	// ErrInvalidLengthSize 는 길이 필드 크기가 1 또는 2 가 아닐 때 반환된다.
	ErrInvalidLengthSize = errors.New("serial: invalid length_size (must be 1 or 2)")

	// ErrInvalidEndian 는 길이 필드 엔디안이 big 또는 little 이 아닐 때 반환된다.
	ErrInvalidEndian = errors.New("serial: invalid length_endian (must be 'big' or 'little')")

	// ErrInvalidChecksum 는 체크섬 타입이 유효하지 않을 때 반환된다.
	ErrInvalidChecksum = errors.New("serial: invalid checksum type (must be 'none', 'sum8', or 'xor')")

	// ErrChecksumMismatch 는 수신된 프레임의 체크섬이 불일치할 때 반환된다.
	// pkg/framing.ErrChecksumMismatch 과 동일한 값이다.
	ErrChecksumMismatch = framing.ErrChecksumMismatch

	// ErrETXMismatch 는 수신된 프레임의 ETX 가 기대값과 불일치할 때 반환된다.
	// pkg/framing.ErrETXMismatch 과 동일한 값이다.
	ErrETXMismatch = framing.ErrETXMismatch

	// ErrFrameTooLarge 는 프레임이 최대 크기를 초과했을 때 반환된다.
	// pkg/framing.ErrFrameTooLarge 과 동일한 값이다.
	ErrFrameTooLarge = framing.ErrFrameTooLarge
)
