package socket

import (
	"encoding/hex"
	"log/slog"
	"time"
)

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
	// DefaultWriteTimeout 은 framer conn.Write 의 기본 쓰기 데드라인이다.
	// stale 클라이언트가 수신을 멈춰 커널 송신버퍼가 포화될 때 conn.Write 가
	// 무한 블록되어 flow(runNode)를 정지시키는 것을 방지한다. 0 이면 데드라인 미설정.
	DefaultWriteTimeout = 5 * time.Second
)

// logPacket 은 log_messages 옵션이 켜져 있을 때 송/수신 패킷을 hex 로 INFO 로그한다.
//
// enabled 가 false 이거나 logger 가 nil 이면 no-op 이다 (opt-in 진단용).
// dir 은 "RX"(수신) 또는 "TX"(송신), agentType 은 로그 prefix (예: "tcp-server"),
// addr 은 상대 주소이며 빈 문자열이면 로그에서 생략된다.
//
// samsung/lgap 의 log_messages 패턴과 동일한 형식(len + hex)을 사용하여 소켓
// 계열 4개 에이전트(tcp/udp × server/client)의 진단 로그를 통일한다.
func logPacket(logger *slog.Logger, enabled bool, agentType, dir, addr string, data []byte) {
	if !enabled || logger == nil {
		return
	}
	attrs := make([]any, 0, 6)
	if addr != "" {
		attrs = append(attrs, "addr", addr)
	}
	attrs = append(attrs, "len", len(data), "hex", hex.EncodeToString(data))
	logger.Info(agentType+": "+dir, attrs...)
}
