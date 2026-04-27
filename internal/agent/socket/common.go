package socket

import "time"

// 프레이밍 타입 상수 정의.
const (
	FramingRaw          = "raw"           // 원시 바이트 스트림 (프레이밍 없음)
	FramingNewline      = "newline"       // 구분자 기반 프레이밍 (기본: '\n')
	FramingLengthPrefix = "length_prefix" // 4바이트 빅엔디안 길이 접두사
	FramingFixedSize    = "fixed_size"    // 고정 크기 프레이밍
)

// 소켓 설정 기본값.
const (
	DefaultBufferSize        = 4096
	DefaultHost              = "0.0.0.0"
	DefaultClientHost        = "localhost"
	DefaultReconnectInterval = 5 * time.Second
	DefaultConnectTimeout    = 10 * time.Second
	DefaultMaxMessageSize    = 1048576 // 1MB
)
