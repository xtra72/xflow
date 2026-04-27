package serial

import "time"

// 프레이밍 타입 상수 정의.
const (
	FramingRaw          = "raw"           // 원시 바이트 스트림 (프레이밍 없음)
	FramingNewline      = "newline"       // 구분자 기반 프레이밍 (기본: '\n')
	FramingLengthPrefix = "length_prefix" // 4바이트 빅엔디안 길이 접두사
	FramingFixedSize    = "fixed_size"    // 고정 크기 프레이밍
	FramingStream       = "stream"        // 유휴 타임아웃 기반 스트림 프레이밍
	FramingFrame        = "frame"         // 프로토콜 수준 프레임 감지 (STX/길이/ETX/체크섬)
)

// 시리얼 설정 기본값.
const (
	DefaultBaudRate       = 9600
	DefaultDataBits       = 8
	DefaultStopBits       = 1
	DefaultParity         = "none"
	DefaultReadTimeout    = 100 * time.Millisecond
	DefaultIdleTimeout    = 1 * time.Millisecond // 스트림 모드 유휴 타임아웃 (framing=stream 시)
	DefaultBufferSize     = 4096
	DefaultMaxMessageSize = 1048576 // 1MB
)

// validBaudRates 는 지원되는 보드레이트 목록이다.
var validBaudRates = map[int]bool{
	300: true, 1200: true, 2400: true, 4800: true,
	9600: true, 19200: true, 38400: true, 57600: true,
	115200: true, 230400: true, 460800: true, 921600: true,
}

// validDataBits 는 지원되는 데이터 비트 목록이다.
var validDataBits = map[int]bool{5: true, 6: true, 7: true, 8: true}

// validStopBits 는 지원되는 스톱 비트 목록이다.
var validStopBits = map[int]bool{1: true, 2: true}

// validParities 는 지원되는 패리티 목록이다.
var validParities = map[string]bool{
	"none": true, "even": true, "odd": true, "mark": true, "space": true,
}

// validFramingTypes 는 지원되는 프레이밍 타입 목록이다.
var validFramingTypes = map[string]bool{
	FramingRaw:          true,
	FramingNewline:      true,
	FramingLengthPrefix: true,
	FramingFixedSize:    true,
	FramingStream:       true,
	FramingFrame:        true,
}

// validChecksumTypes 는 지원되는 체크섬 타입 목록이다.
var validChecksumTypes = map[string]bool{
	"none": true, "sum8": true, "xor": true,
}

// validLengthEndians 는 지원되는 길이 필드 엔디안 목록이다.
var validLengthEndians = map[string]bool{
	"big": true, "little": true,
}
