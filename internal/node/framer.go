// Package node 의 framer.go 는 바이트 스트림에서 프로토콜 프레임을 분리하는
// framer 노드의 Phase 2 (다중 스트림) 구현이다.
//
// # Phase 2: 다중 스트림 버퍼 모델
//
// Phase 1 은 단일 공용 버퍼 (`n.buf []byte` + `n.index int`) 만을 사용하였다.
// Phase 2 는 메타데이터 기반 stream key (기본: `connection_id`) 로 분리된
// 독립 버퍼 맵 (`streams map[string]*streamBuffer`) 을 도입한다. 각
// `streamBuffer` 는 자신의 `framing.Framer` 인스턴스, rolling 버퍼, 마지막
// 활동 시각, 프레임 인덱스 카운터를 보유한다. 이는 SPEC-NODE-002 M4 (R4.1 ~
// R4.8) 요구사항에 대응한다.
//
// # 자원 제한
//
//   - MaxStreams: 양수일 때 현재 스트림 수가 cap 에 도달하면 새 키에 대한
//     메시지는 error 포트로 `frame.buffer.max_streams_exceeded` 코드를
//     담은 메시지로 거부된다. 기존 스트림은 영향을 받지 않는다 (reject
//     policy). LRU 축출은 본 SPEC 의 범위 외이다.
//
//   - StreamIdleTimeout: 양수일 때 각 Process 호출의 시작에서 현재 시각 기준
//     lastActivity 가 timeout 이상 경과한 스트림을 삭제한다 (lazy 축출). idle
//     축출은 silent 하며 error 메시지를 발행하지 않는다. 백그라운드 고루틴은
//     사용하지 않는다.
//
// # Shutdown 설계 결정 (Option B + sink)
//
// Phase 5 이전의 엔진 (`internal/engine/engine.go`) 에는 노드가 Shutdown 중에
// 결과 메시지를 downstream 으로 비동기 발행할 수 있는 경로가 없다. Shutdown
// 은 wires 가 이미 close 된 이후에 호출되고, runNode 고루틴이 완료되었기
// 때문이다. 본 SPEC 의 R4.6 "error 포트로 incomplete_on_stop 경고 발행" 을
// 엄밀히 만족시키려면 엔진에 async-emit 훅 (예: `chan<- message.Message`,
// 또는 Shutdown 전용 drain 인터페이스) 이 필요하다.
//
// Phase 2 는 다음 두 단계로 대응한다:
//
//  1. 운영 기본값: `slog.Warn` 레벨로 미완성 스트림을 구조적으로 로깅한다
//     (`stream_key`, `bytes_remaining`, `framer_type` 필드). 즉 R7.4 의
//     WARN 로그 요구는 완전히 충족한다.
//
//  2. 테스트 및 Phase 5 연계: `incompleteSink func(incompleteStreamReport)`
//     를 노드에 주입할 수 있다. sink 가 non-nil 이면 Shutdown 은 각
//     미완성 스트림에 대해 sink 를 호출한다. 테스트는 이 sink 로 리포트를
//     수집한다. Phase 5 에서 엔진이 async-emit 을 지원하면, 그 발행 함수를
//     `incompleteSink` 로 주입하거나 래핑하여 R4.6 원문의 "error 포트
//     발행" 을 달성할 수 있다.
//
// TODO(Phase 5): 엔진이 노드에 `ErrorEmitter` 인터페이스를 제공하면
// incompleteSink 를 해당 emitter 로 대체하여 `error` 포트 발행으로 승격.
//
// # 에러 포트 라우팅 컨벤션 (Phase 1 연속)
//
// 현재 엔진은 Process 반환값의 단일 out 포트로만 라우팅한다. 본 노드는 성공
// 프레임과 파싱 에러를 동시에 반환할 수 있어야 하므로, Phase 1 부터 다음
// 임시 규약을 사용한다:
//
//   - 정상 프레임: 반환 슬라이스에 포함, 메타데이터에 frame.port 키 없음
//   - 에러 메시지: 반환 슬라이스에 포함, 메타데이터에 frame.port="error"
//     및 frame.error.code 등 에러 정보
//   - Process 자체는 대부분의 경우 nil error 를 반환
//
// Phase 5 (엔진 error 포트 통합) 에서 결과 슬라이스를 메타데이터 기반으로
// out/error 포트로 분기하도록 확장될 예정이다.
package node

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/framing"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// framer 노드에서 사용하는 에러 코드 상수.
const (
	errCodeInvalidPayload     = "frame.input.invalid_payload"
	errCodeETXMismatch        = "frame.parse.etx_mismatch"
	errCodeChecksumMismatch   = "frame.parse.checksum_mismatch"
	errCodeMaxSizeExceeded    = "frame.parse.max_size_exceeded"
	errCodeLengthInvalid      = "frame.parse.length_invalid"
	errCodeMaxStreamsExceeded = "frame.buffer.max_streams_exceeded"
	errCodeIncompleteOnStop   = "frame.buffer.incomplete_on_stop"
	errCodeGenericParseError  = "frame.parse.error"
)

// Phase 1 전용 에러 포트 마커 키.
const (
	framerPortMetaKey  = "frame.port"
	framerPortErrorVal = "error"
)

// 기본 옵션 값.
const (
	defaultStreamKeyMetadata = "connection_id"
)

// errInvalidFramerPayload 는 페이로드에서 바이트를 추출할 수 없을 때 내부적으로
// 사용하는 sentinel 에러이다.
var errInvalidFramerPayload = errors.New("framer: invalid payload (no raw or data key)")

// errMaxStreamsExceeded 는 max_streams cap 초과 시 내부적으로 사용하는
// sentinel 에러이다. 테스트 및 로깅에서 구분에 사용된다.
var errMaxStreamsExceeded = errors.New("framer: max_streams exceeded")

// framerNodeOptions 는 framer 노드 고유의 (framing.Options 와 구분되는)
// 런타임 옵션이다. stream key 해상도, 자원 제한, idle 축출 등이 해당된다.
type framerNodeOptions struct {
	// StreamKeyMetadata 는 메시지 메타데이터에서 스트림 키를 찾을 때 사용할
	// 키 이름이다. 값이 빈 문자열이면 defaultStreamKeyMetadata 를 사용한다.
	StreamKeyMetadata string

	// StreamIdleTimeout 은 스트림이 lazy 축출되기 전 허용되는 최대 유휴
	// 기간이다. 0 이면 축출을 비활성화한다.
	StreamIdleTimeout time.Duration

	// MaxStreams 는 동시 활성 스트림 수의 상한이다. 0 이면 무제한이다.
	// 양수일 때 cap 에 도달하면 새 키에 대한 메시지는 reject 된다.
	MaxStreams int
}

// streamBuffer 는 하나의 스트림 키에 대응하는 독립 버퍼 상태이다. 각
// 스트림은 자신의 `framing.Framer` 인스턴스를 소유한다. 이는 framer 구현이
// 미래에 내부 상태 (예: CRC 누적기) 를 가지더라도 스트림 간 간섭을 완전히
// 차단하기 위한 설계이다.
type streamBuffer struct {
	framer       framing.Framer // 해당 스트림 전용 framer 인스턴스
	buf          []byte         // rolling 누적 버퍼
	index        int            // 해당 스트림 내 frame.index 카운터
	lastActivity time.Time      // 마지막 Process 활동 시각 (idle 축출 기준)
}

// incompleteStreamReport 는 Shutdown 시 미완성 버퍼가 남아 있는 스트림에
// 대한 리포트이다. 테스트는 incompleteSink 를 통해 이 리포트를 수집하며,
// Phase 5 에서 엔진 error 포트로 승격될 수 있다.
type incompleteStreamReport struct {
	StreamKey      string
	BytesRemaining int
	FramerType     string
	ErrorCode      string
}

// FramerNode 는 메타데이터 기반 스트림 키 별로 독립 버퍼를 유지하면서
// pkg/framing.Framer 의 Drain API 를 호출하여 완성된 프레임을 출력하는 처리
// 노드이다.
//
// Phase 2 범위: 다중 스트림 버퍼, max_streams reject 정책, lazy idle eviction,
// Shutdown flush. Phase 3 에서 Registry 등록 및 상세 옵션 파싱이 추가된다.
type FramerNode struct {
	*BaseNode

	framingMode string          // framing 옵션 값 (raw, newline, ...)
	options     framing.Options // framing 엔진 옵션
	nodeOptions framerNodeOptions

	mu       sync.Mutex
	streams  map[string]*streamBuffer
	stopOnce sync.Once // Shutdown 은 멱등해야 한다

	// incompleteSink 는 Shutdown 시 미완성 스트림 리포트를 수집하는 옵셔널
	// 훅이다. 운영 환경에서는 nil 이며 slog.Warn 만 기록된다. 테스트는
	// withIncompleteSink 로 주입한다. Phase 5 에서 엔진 error 포트 방출
	// 훅으로 승격될 수 있다.
	incompleteSink func(incompleteStreamReport)

	// nowFn 은 lastActivity 검사에 사용하는 시각 함수이다. 테스트 격리 목적
	// 에서 override 가능하도록 함수 포인터로 분리한다. 기본값은 time.Now.
	nowFn func() time.Time
}

// NewFramerNode 는 주어진 NodeDef 로부터 새로운 FramerNode 를 생성한다. 본
// Phase 2 구현은 Phase 1 과 동일하게 간단한 옵션 파싱만 수행한다. framing
// 키의 값을 읽어 framing.Options 를 구성하되, framing 엔진 인스턴스 자체는
// 스트림별로 구성되므로 여기서는 만들지 않는다. nodeOptions (stream_key_metadata,
// stream_idle_timeout, max_streams) 는 기본값으로 초기화하며, 상세 옵션 검증은
// Phase 3 의 Registry 등록 단계에서 추가된다.
func NewFramerNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &FramerNode{
		BaseNode: base,
		streams:  make(map[string]*streamBuffer),
		nowFn:    time.Now,
	}

	// framing 옵션 추출
	cfg := def.Config
	mode, _ := cfg["framing"].(string)
	if mode == "" {
		// Phase 2 는 기본값으로 raw 를 허용 (Phase 3 에서 엄격화)
		mode = framing.ModeRaw
	}
	// 단순 옵션만 구성 (Phase 3 에서 전체 옵션 파서 추가)
	fopts := framing.Options{}
	if bs, ok := cfg["buffer_size"].(int); ok {
		fopts.BufferSize = bs
	}
	if fs, ok := cfg["fixed_size"].(int); ok {
		fopts.FixedSize = fs
	}
	if mms, ok := cfg["max_message_size"].(int); ok {
		fopts.MaxMessageSize = mms
	}
	// 팩토리 검증: mode + options 가 유효한지 확인 (인스턴스는 스트림 생성
	// 시점에 만들어지므로, 여기서는 생성만 해보고 폐기한다).
	if _, err := framing.New(mode, fopts); err != nil {
		return nil, fmt.Errorf("framer node: %w", err)
	}
	n.framingMode = mode
	n.options = fopts

	// nodeOptions 기본값
	n.nodeOptions = framerNodeOptions{
		StreamKeyMetadata: defaultStreamKeyMetadata,
		StreamIdleTimeout: 0,
		MaxStreams:        0,
	}

	return n, nil
}

// Init 은 노드를 Running 상태로 전이시킨다.
func (n *FramerNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 Stopping 상태로 전이시키고 모든 미완성 스트림 버퍼를
// flush 한다. 멱등하므로 여러 번 호출되어도 안전하며, 두 번째 이후 호출은
// 아무 동작도 하지 않는다 (sync.Once 로 보호).
func (n *FramerNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() {
		n.flushAllStreams()
	})
	// 상태 전이 실패 (이미 stopped) 는 무시 (idempotent).
	_ = n.BaseNode.TransitionTo(lifecycle.StateStopping)
	return nil
}

// Process 는 입력 메시지의 바이트를 stream key 별 독립 버퍼에 누적하고,
// 해당 스트림의 framer 에 대해 Drain 을 호출하여 완성된 프레임을 결과로
// 반환한다. 각 Process 호출의 시작에서 idle 축출 스윕이 수행된다.
//
// 반환 정책:
//   - 정상 프레임들은 결과 슬라이스에 순서대로 포함된다.
//   - 페이로드 추출 실패, max_streams 거부, 파싱 에러는 결과 슬라이스에
//     포함된 에러 메시지 (frame.port=error) 로 제공된다.
//   - Process 자체는 대부분 nil 에러를 반환한다.
func (n *FramerNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	// 1) 페이로드에서 바이트 추출. 실패 시 해당 메시지만 error 포트로.
	data, err := extractInputBytes(msg)
	if err != nil {
		return []message.Message{n.makeErrorMessage(msg, errCodeInvalidPayload, err, 0, "")}, nil
	}
	// 빈 바이트는 조기 종료 (버퍼 상태 영향 없음).
	if len(data) == 0 {
		return nil, nil
	}

	// 2) stream key 해상도 (mutex 바깥에서 가능)
	streamKey := n.resolveStreamKey(msg)

	n.mu.Lock()
	defer n.mu.Unlock()

	// 3) idle eviction 스윕
	n.evictIdleStreamsLocked()

	// 4) 스트림 버퍼 조회 또는 생성 (max_streams 검사 포함)
	sb, err := n.getOrCreateStreamLocked(streamKey)
	if err != nil {
		if errors.Is(err, errMaxStreamsExceeded) {
			return []message.Message{
				n.makeErrorMessage(msg, errCodeMaxStreamsExceeded, err, 0, streamKey),
			}, nil
		}
		return []message.Message{
			n.makeErrorMessage(msg, errCodeGenericParseError, err, 0, streamKey),
		}, nil
	}

	// 5) 바이트 누적 및 Drain
	sb.buf = append(sb.buf, data...)
	sb.lastActivity = n.nowFn()

	frames, remainder, drainErr := sb.framer.Drain(sb.buf)

	// remainder 처리: Drain 은 buf 의 backing storage 를 공유할 수 있으므로
	// 새 슬라이스에 복사하여 보관한다.
	if len(remainder) == 0 {
		sb.buf = sb.buf[:0]
	} else {
		cp := make([]byte, len(remainder))
		copy(cp, remainder)
		sb.buf = cp
	}

	// 6) 결과 메시지 구성
	results := make([]message.Message, 0, len(frames)+1)
	for _, frame := range frames {
		out := n.makeFrameMessageLocked(msg, frame, streamKey, sb)
		results = append(results, out)

		// R7.3: DEBUG 로그 - 프레임 조립 성공
		if logger := n.Logger(); logger != nil {
			logger.Debug("framer: frame assembled",
				"framer_type", n.framingMode,
				"stream_key", streamKey,
				"frame_size", len(frame),
				"frame_index", sb.index-1, // index 는 makeFrameMessageLocked 에서 이미 증가
			)
		}
	}
	if drainErr != nil {
		code := classifyFramerError(drainErr)
		results = append(results, n.makeErrorMessage(msg, code, drainErr, len(sb.buf), streamKey))

		// R7.4: WARN 로그 - 파싱 에러
		if logger := n.Logger(); logger != nil {
			logger.Warn("framer: parse error",
				"error_code", code,
				"framer_type", n.framingMode,
				"stream_key", streamKey,
				"buffer_bytes", len(sb.buf),
				"error", drainErr.Error(),
			)
		}
	}

	// R7.5: DEBUG 로그 - Process 호출 요약
	if logger := n.Logger(); logger != nil {
		logger.Debug("framer: process complete",
			"input_bytes", len(data),
			"output_frame_count", len(frames),
			"stream_key", streamKey,
		)
	}

	return results, nil
}

// Configure 는 Phase 2 에서는 BaseNode.Configure 만 위임한다. 세부 옵션
// 재파싱은 Phase 3 이후 팩토리 경로에서 통합된다.
func (n *FramerNode) Configure(config map[string]any) error {
	return n.BaseNode.Configure(config)
}

// --- 내부 helper ---

// resolveStreamKey 는 메시지 메타데이터에서 stream key 를 해상한다. 키가
// 없거나 값이 빈 문자열이면 "" (공용 버퍼) 을 반환한다. SPEC R2.4 의 두
// 번째 절 ("메타데이터 키가 없거나 값이 비어 있으면") 을 구현한다.
func (n *FramerNode) resolveStreamKey(msg message.Message) string {
	keyName := n.nodeOptions.StreamKeyMetadata
	if keyName == "" {
		keyName = defaultStreamKeyMetadata
	}
	val, ok := msg.Metadata().Get(keyName)
	if !ok {
		return ""
	}
	return val // 빈 문자열 그대로 반환 → 공용 버퍼
}

// getOrCreateStreamLocked 는 스트림 키에 대응하는 streamBuffer 를 조회하거나
// 새로 생성한다. 이미 존재하면 그대로 반환한다. 존재하지 않을 때 max_streams
// cap 을 검사하고, 초과 시 errMaxStreamsExceeded 를 반환한다. 호출자는
// n.mu 를 잠근 상태여야 한다.
func (n *FramerNode) getOrCreateStreamLocked(key string) (*streamBuffer, error) {
	if sb, ok := n.streams[key]; ok {
		return sb, nil
	}
	if n.nodeOptions.MaxStreams > 0 && len(n.streams) >= n.nodeOptions.MaxStreams {
		return nil, errMaxStreamsExceeded
	}
	sb, err := n.newStreamBufferLocked()
	if err != nil {
		return nil, err
	}
	n.streams[key] = sb
	return sb, nil
}

// newStreamBufferLocked 는 노드의 framingMode + options 로부터 새로운
// streamBuffer 를 생성한다. 각 스트림은 자신의 framer 인스턴스를 소유한다.
// 호출자는 n.mu 를 잠근 상태여야 한다.
func (n *FramerNode) newStreamBufferLocked() (*streamBuffer, error) {
	f, err := framing.New(n.framingMode, n.options)
	if err != nil {
		return nil, fmt.Errorf("framer: new stream framer: %w", err)
	}
	return &streamBuffer{
		framer:       f,
		buf:          nil,
		index:        0,
		lastActivity: n.nowFn(),
	}, nil
}

// evictIdleStreamsLocked 는 stream_idle_timeout 이 양수일 때 lastActivity 가
// 현재 시각 기준 timeout 이상 경과한 모든 스트림을 맵에서 삭제한다. idle
// 축출은 silent 하며 error 메시지나 sink 호출을 발생시키지 않는다. 호출자는
// n.mu 를 잠근 상태여야 한다.
func (n *FramerNode) evictIdleStreamsLocked() {
	timeout := n.nodeOptions.StreamIdleTimeout
	if timeout <= 0 {
		return
	}
	now := n.nowFn()
	for key, sb := range n.streams {
		if now.Sub(sb.lastActivity) >= timeout {
			delete(n.streams, key)
		}
	}
}

// flushAllStreams 는 Shutdown 시점에 모든 스트림 버퍼를 검사하고 미완성
// 바이트가 남아 있는 스트림에 대해 incompleteSink 또는 slog.Warn 으로
// 리포트를 발행한다. 리포트 발행 후 스트림 맵을 비운다. 호출은 sync.Once
// 로 보호되므로 멱등하다.
func (n *FramerNode) flushAllStreams() {
	n.mu.Lock()
	defer n.mu.Unlock()

	for key, sb := range n.streams {
		if len(sb.buf) == 0 {
			continue
		}
		report := incompleteStreamReport{
			StreamKey:      key,
			BytesRemaining: len(sb.buf),
			FramerType:     n.framingMode,
			ErrorCode:      errCodeIncompleteOnStop,
		}
		if n.incompleteSink != nil {
			n.incompleteSink(report)
		} else if logger := n.Logger(); logger != nil {
			// 운영 기본값: 구조적 WARN 로깅.
			logger.Warn("framer: incomplete buffer on shutdown",
				"error_code", report.ErrorCode,
				"framer_type", report.FramerType,
				"stream_key", report.StreamKey,
				"bytes_remaining", report.BytesRemaining,
			)
		}
	}
	// 맵 초기화: 이후 Process 가 다시 채우지 못하도록 한다 (노드는 이미
	// Shutdown 이므로 Process 는 호출되지 않아야 하지만 안전하게 비운다).
	n.streams = make(map[string]*streamBuffer)
}

// extractInputBytes 는 입력 메시지에서 raw 바이트를 추출한다. 우선순위:
// (1) payload["raw"] 가 []byte 또는 string 이면 그대로 사용,
// (2) payload["data"] 가 hex 문자열이면 디코딩,
// 둘 다 없거나 디코딩 실패 시 errInvalidFramerPayload 를 반환한다.
func extractInputBytes(msg message.Message) ([]byte, error) {
	if raw, ok := msg.Payload().Get("raw"); ok {
		switch v := raw.(type) {
		case []byte:
			return v, nil
		case string:
			return []byte(v), nil
		case nil:
			// nil raw 는 빈 바이트로 취급 (Phase 1 단순 정책)
			return []byte{}, nil
		}
	}
	if d, ok := msg.Payload().Get("data"); ok {
		if s, ok := d.(string); ok {
			decoded, err := hex.DecodeString(s)
			if err != nil {
				return nil, fmt.Errorf("framer: invalid hex in data key: %w", err)
			}
			return decoded, nil
		}
	}
	return nil, errInvalidFramerPayload
}

// makeFrameMessageLocked 는 src 메시지의 메타데이터를 보존한 새 메시지를
// 생성하고 페이로드에 프레임 바이트를 설정한다. 추가로 frame.index,
// frame.framer_type, frame.stream_key 메타를 설정한다. sb.index 는 함수 내부
// 에서 증가한다. 호출자는 n.mu 를 잠근 상태여야 한다.
func (n *FramerNode) makeFrameMessageLocked(src message.Message, frame []byte, streamKey string, sb *streamBuffer) message.Message {
	var opts []message.Option
	for k, v := range src.Metadata().All() {
		opts = append(opts, message.WithMetadata(k, v))
	}
	opts = append(opts,
		message.WithMetadata("frame.index", strconv.Itoa(sb.index)),
		message.WithMetadata("frame.framer_type", n.framingMode),
		message.WithMetadata("frame.stream_key", streamKey),
	)
	sb.index++

	out := message.New(opts...)
	// 프레임 바이트를 새 슬라이스에 복사하여 페이로드에 설정
	copied := make([]byte, len(frame))
	copy(copied, frame)
	out.Payload().Set("raw", copied)
	out.Payload().Set("data", hex.EncodeToString(copied))
	return out
}

// makeErrorMessage 는 에러 포트로 라우팅할 메시지를 생성한다. 메타데이터에
// frame.port=error, frame.error.code, frame.framer_type, frame.stream_key,
// frame.buffer.bytes_at_error 를 설정한다.
func (n *FramerNode) makeErrorMessage(src message.Message, code string, cause error, bufBytesAtError int, streamKey string) message.Message {
	var opts []message.Option
	for k, v := range src.Metadata().All() {
		opts = append(opts, message.WithMetadata(k, v))
	}
	opts = append(opts,
		message.WithMetadata(framerPortMetaKey, framerPortErrorVal),
		message.WithMetadata("frame.error.code", code),
		message.WithMetadata("frame.framer_type", n.framingMode),
		message.WithMetadata("frame.stream_key", streamKey),
		message.WithMetadata("frame.buffer.bytes_at_error", strconv.Itoa(bufBytesAtError)),
	)
	if cause != nil {
		opts = append(opts, message.WithMetadata("frame.error.cause", cause.Error()))
	}
	out := message.New(opts...)
	return out
}

// classifyFramerError 는 pkg/framing 의 sentinel 에러를 에러 코드로 매핑한다.
// 현재 pkg/framing 은 ErrETXMismatch, ErrChecksumMismatch, ErrMaxMessageSize,
// ErrFrameTooLarge, ErrInvalidFraming 을 제공하며, 매칭되지 않으면 일반
// parse 에러 코드로 분류한다.
func classifyFramerError(err error) string {
	switch {
	case errors.Is(err, framing.ErrETXMismatch):
		return errCodeETXMismatch
	case errors.Is(err, framing.ErrChecksumMismatch):
		return errCodeChecksumMismatch
	case errors.Is(err, framing.ErrMaxMessageSize), errors.Is(err, framing.ErrFrameTooLarge):
		return errCodeMaxSizeExceeded
	case errors.Is(err, framing.ErrLengthInvalid):
		return errCodeLengthInvalid
	default:
		return errCodeGenericParseError
	}
}
