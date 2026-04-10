// Package framing 은 바이트 스트림에서 프로토콜 프레임을 분리하는
// 범용 프레이밍 엔진을 제공한다.
//
// 본 패키지는 원래 시리얼 에이전트 (`internal/agent/serial`) 내부에 있던
// 프레이밍 로직을 공개 패키지로 승격한 결과물이다. 시리얼 에이전트와
// 플로우 그래프의 framer 노드, 그 외 임의의 바이트 스트림 소스
// (TCP, UDP, 파일 등) 가 동일한 엔진을 공유한다.
//
// 여섯 가지 프레이밍 모드를 지원한다:
//
//   - ModeRaw:          원시 바이트 스트림 (프레이밍 없음)
//   - ModeNewline:      구분자 기반 프레이밍 (기본: '\n')
//   - ModeLengthPrefix: 4바이트 빅엔디안 길이 접두사
//   - ModeFixedSize:    고정 크기 프레이밍
//   - ModeStream:       유휴 타임아웃 기반 스트림 프레이밍
//   - ModeFrame:        프로토콜 수준 프레임 감지 (STX/길이/ETX/체크섬)
//
// 모든 framer 구현체는 Framer 인터페이스를 만족하며,
// New 팩토리 함수를 통해 생성된다.
package framing

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"
)

// 프레이밍 모드 상수. 문자열 값은 시리얼 에이전트 설정 스키마와의
// 하위 호환을 위해 기존 값을 그대로 유지한다.
const (
	ModeRaw          = "raw"           // 원시 바이트 스트림 (프레이밍 없음)
	ModeNewline      = "newline"       // 구분자 기반 프레이밍 (기본: '\n')
	ModeLengthPrefix = "length_prefix" // 4바이트 빅엔디안 길이 접두사
	ModeFixedSize    = "fixed_size"    // 고정 크기 프레이밍
	ModeStream       = "stream"        // 유휴 타임아웃 기반 스트림 프레이밍
	ModeFrame        = "frame"         // 프로토콜 수준 프레임 감지 (STX/길이/ETX/체크섬)
)

// 기본값 상수.
const (
	// DefaultBufferSize 는 버퍼 기반 framer (raw, newline, stream) 의
	// 기본 읽기 버퍼 크기이다.
	DefaultBufferSize = 4096
)

// 센티넬 에러. 시리얼 에이전트가 기존에 `internal/agent/serial/errors.go` 에
// 정의했던 에러와 동일한 의미를 가진다. 시리얼 패키지는 이 에러들을
// 그대로 래핑하여 외부 관측 동작을 유지한다.
var (
	// ErrInvalidFraming 는 지원되지 않는 프레이밍 타입일 때 반환된다.
	ErrInvalidFraming = errors.New("framing: unsupported framing type")

	// ErrMaxMessageSize 는 메시지가 최대 크기를 초과했을 때 반환된다.
	ErrMaxMessageSize = errors.New("framing: message exceeds max size")

	// ErrChecksumMismatch 는 수신된 프레임의 체크섬이 불일치할 때 반환된다.
	ErrChecksumMismatch = errors.New("framing: frame checksum mismatch")

	// ErrETXMismatch 는 수신된 프레임의 ETX 가 기대값과 불일치할 때 반환된다.
	ErrETXMismatch = errors.New("framing: frame ETX mismatch")

	// ErrFrameTooLarge 는 프레임이 최대 크기를 초과했을 때 반환된다.
	ErrFrameTooLarge = errors.New("framing: frame exceeds max_message_size")

	// ErrLengthInvalid 는 길이 필드가 불가능한 값 (음수, 오버플로 등) 을
	// 가질 때 반환된다. 바이트 부족 (부분 프레임) 과는 구분된다.
	ErrLengthInvalid = errors.New("framing: length field invalid")
)

// Framer 는 바이트 스트림에서 하나의 프레임을 읽고 쓰는 인터페이스이다.
// 모든 모드 구현체는 동일 입력에 대해 동일 출력을 보장한다.
type Framer interface {
	// Read 는 리더에서 하나의 프레임을 읽어 반환한다.
	// 충분한 바이트가 없으면 io.EOF 또는 io.ErrUnexpectedEOF 를 반환한다.
	//
	// Read 는 io.Reader 기반의 전통적인 경로로, 시리얼 포트 등 스트림 기반
	// 소스에서 직접 사용된다. 부분 프레임 상태를 호출자가 복구할 수 없으므로
	// rolling-buffer 노드에서는 Drain 을 사용한다.
	Read(r io.Reader) ([]byte, error)

	// Write 는 라이터에 하나의 프레임을 쓴다.
	Write(w io.Writer, data []byte) error

	// Drain 은 인메모리 바이트 슬라이스에서 완성된 프레임을 모두 추출한다.
	// Read(io.Reader) 의 byte-slice 기반 카운터파트로, 프레이밍 노드의
	// rolling-buffer 경로에 적합한 API 이다.
	//
	// 매개변수:
	//
	//	buf: 단일 스트림에 대해 누적된 바이트 버퍼
	//
	// 반환값:
	//
	//	frames:    0 개 이상의 완성된 프레임 (조립 순서 유지). 각 프레임은
	//	           새로 할당된 바이트 슬라이스로 호출자가 소유권을 가진다.
	//	remainder: 소비되지 않은 잔여 바이트. 복사가 불필요한 경우 buf 의
	//	           backing storage 를 공유할 수 있으므로, 호출자는 다음
	//	           변경 전에 자신의 rolling 버퍼에 복사하거나 재할당해야 한다.
	//	err:       복구 불가능한 프레이밍 오류 (ETX 불일치, 체크섬 불일치,
	//	           최대 크기 초과, 잘못된 길이 필드) 발생 시에만 non-nil.
	//	           부분 프레임 조건 (바이트 부족) 은 (frames, buf, nil) 을
	//	           반환하며, 호출자는 다음 데이터를 기다려야 한다.
	Drain(buf []byte) (frames [][]byte, remainder []byte, err error)
}

// ScannerConfigurer 는 프레이머 구현체가 bufio.Scanner 기반 버퍼링을
// 지원함을 선택적으로 노출하기 위한 인터페이스이다.
//
// newlineFramer 처럼 Read() 호출마다 새 scanner 를 생성하는 구현체는
// 리더 수명 동안 하나의 scanner 를 재사용해야 버퍼 데이터 손실을 방지할 수 있다.
// 이를 위해 호출자 (예: SerialConnReader) 는 ConfigureScanner 를 호출하여
// 자신에게 종속된 scanner 를 한 번 생성한다.
//
// ScannerConfigurer 를 구현하지 않는 framer 는 Read 호출로 직접 사용한다.
type ScannerConfigurer interface {
	ConfigureScanner(r io.Reader) *bufio.Scanner
}

// Options 는 프레이머 생성 옵션이다.
// 필드 이름과 의미는 시리얼 에이전트의 기존 FramerOptions 와 동일하다.
type Options struct {
	BufferSize     int           // 읽기 버퍼 크기 (rawFramer, newlineFramer, streamFramer 에 사용)
	Delimiter      byte          // 구분자 (newlineFramer 에 사용, 0이면 '\n')
	FixedSize      int           // 고정 크기 (fixedSizeFramer 에 사용)
	MaxMessageSize int           // 최대 메시지 크기 (lengthPrefixFramer, frameFramer 에 사용, 0=무제한)
	IdleTimeout    time.Duration // 유휴 타임아웃 (streamFramer 참조용, 실제 타이밍은 시리얼 포트 ReadTimeout 에 의존)

	// frame 프레이밍 전용 옵션
	STX                  []byte // 프레임 시작 마커
	ETX                  []byte // 프레임 종료 마커
	LengthOffset         int    // STX 부터 길이 필드까지 오프셋
	LengthSize           int    // 길이 필드 크기 (1 또는 2)
	LengthEndian         string // "big" 또는 "little"
	LengthIncludesHeader bool   // 길이에 헤더 포함 여부
	LengthAdjustment     int    // 디코딩된 길이에 더할 보정값
	Checksum             string // "none", "sum8", "xor"
}

// New 는 프레이밍 모드에 따라 적절한 Framer 구현체를 생성한다.
// 지원되지 않는 모드에 대해서는 ErrInvalidFraming 을 래핑하여 반환한다.
func New(mode string, opts Options) (Framer, error) {
	switch mode {
	case ModeRaw:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &rawFramer{bufferSize: size}, nil

	case ModeNewline:
		delim := opts.Delimiter
		if delim == 0 {
			delim = '\n'
		}
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &newlineFramer{delimiter: delim, bufferSize: size}, nil

	case ModeLengthPrefix:
		return &lengthPrefixFramer{maxMessageSize: opts.MaxMessageSize}, nil

	case ModeFixedSize:
		return &fixedSizeFramer{fixedSize: opts.FixedSize}, nil

	case ModeStream:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &streamFramer{bufferSize: size}, nil

	case ModeFrame:
		return &frameFramer{
			stx:                  opts.STX,
			etx:                  opts.ETX,
			lengthOffset:         opts.LengthOffset,
			lengthSize:           opts.LengthSize,
			lengthEndian:         opts.LengthEndian,
			lengthIncludesHeader: opts.LengthIncludesHeader,
			lengthAdjustment:     opts.LengthAdjustment,
			checksum:             opts.Checksum,
			maxMessageSize:       opts.MaxMessageSize,
		}, nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidFraming, mode)
	}
}
