// remote_log_hub.go 는 M10(그룹 L) monitor.logs 스트림 소스를 위한 노드-로컬 로그
// 팬아웃 hub 를 정의한다(@SPEC:SPEC-REMOTE-001 M10, REQ-L06/J08/J08b).
//
// 설계(in-process 탭, 자가 WS dial 없음 — A18 일관):
//
//	logStreamHub 는 io.Writer 이다. observe.StreamRouter 의 기본 writer 체인에 합류
//	(cmd/xflowd main.go 가 MonitoringBroadcaster 의 추가 로그 writer 로 주입)하여,
//	노드가 자신의 로그 파이프라인을 in-process 로 탭한다. Write 는 JSON 로그 라인을
//	파싱해 로컬 log.entry 와 동형(level/message/timestamp/source/...)의 페이로드로
//	정규화한 뒤, 등록된 모든 구독자 채널로 비차단 팬아웃한다(백프레셔 — 최신값 우선).
//
//	각 monitor.logs 스트림 구독은 hub.subscribe() 로 채널을 받고, Close 시
//	hub.unsubscribe() 로 제거한다(teardown — REQ-J08b, 누수 없음). 캐시 우회(라이브).
//
// redaction(REQ-J06): 로그 라인 자체는 시크릿을 담지 않도록 로깅 측에서 관리되며,
// 추가로 client 가 전송 전 QueryRedactor 를 적용한다(이중 방어).
package main

import (
	"encoding/json"
	"io"
	"sync"
)

// logStreamSubBuffer 는 구독자 채널 버퍼 크기이다(백프레셔 유계 — 최신값 우선 drop).
const logStreamSubBuffer = 64

// logStreamHub 는 노드의 로그 라인을 다중 monitor.logs 구독자로 팬아웃하는 io.Writer
// 이다(REQ-L06). 스레드 안전하며, 구독자 없으면 파싱/팬아웃을 생략한다(저비용).
type logStreamHub struct {
	mu   sync.Mutex
	subs map[*logStreamSub]struct{}
}

// logStreamSub 는 단일 monitor.logs 구독자 채널이다.
type logStreamSub struct {
	ch chan json.RawMessage
}

// newLogStreamHub 는 빈 로그 hub 를 생성한다.
func newLogStreamHub() *logStreamHub {
	return &logStreamHub{subs: make(map[*logStreamSub]struct{})}
}

// logHubWriter 는 *logStreamHub 를 io.Writer 로 nil-안전하게 변환한다. 타입 nil 을
// io.Writer 인터페이스로 직접 넘기면 non-nil 인터페이스가 되어버리므로, nil hub 는
// 진짜 nil io.Writer 를 반환한다(WithExtraLogWriter 가 무시하도록 — server/disabled 모드).
func logHubWriter(h *logStreamHub) io.Writer {
	if h == nil {
		return nil
	}
	return h
}

// Write 는 io.Writer 를 구현한다(observe.StreamRouter 기본 writer 체인에서 호출).
//
// slog 파이프라인 안정성을 위해 항상 len(p), nil 을 반환한다(wsLogWriter 와 동일 정신).
// 구독자가 없으면 즉시 반환한다(저비용). JSON 로그 라인을 파싱해 로컬 log.entry 와 동형
// 페이로드로 정규화한 뒤 팬아웃한다. 비-JSON/파싱 실패 라인은 무시한다.
func (h *logStreamHub) Write(p []byte) (int, error) {
	n := len(p)

	h.mu.Lock()
	hasSubs := len(h.subs) > 0
	h.mu.Unlock()
	if !hasSubs {
		return n, nil
	}

	payload := normalizeLogLine(p)
	if payload == nil {
		return n, nil // 비-JSON/무시 대상.
	}

	h.mu.Lock()
	subs := make([]*logStreamSub, 0, len(h.subs))
	for s := range h.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	for _, s := range subs {
		fanoutLog(s.ch, payload)
	}
	return n, nil
}

// normalizeLogLine 은 JSON slog 라인을 로컬 log.entry 와 동형 페이로드로 변환한다
// (LogPanel 이 소비하는 형상 — level/message/timestamp/source/componentKind/componentName).
// 비-JSON 또는 빈 라인은 nil 을 반환한다(무시).
func normalizeLogLine(p []byte) json.RawMessage {
	var logLine map[string]any
	if err := json.Unmarshal(p, &logLine); err != nil {
		return nil
	}
	level, _ := logLine["level"].(string)
	msg, _ := logLine["msg"].(string)
	ts, _ := logLine["time"].(string)
	compType, _ := logLine["type"].(string)
	compName, _ := logLine["name"].(string)

	out, err := json.Marshal(map[string]string{
		"level":         level,
		"message":       msg,
		"timestamp":     ts,
		"source":        logSourceFromType(compType),
		"componentKind": compType,
		"componentName": compName,
	})
	if err != nil {
		return nil
	}
	return out
}

// logSourceFromType 은 type 값을 로그 소스 분류로 매핑한다(wsLogWriter 와 동형 정책).
func logSourceFromType(compType string) string {
	switch compType {
	case "":
		return "system"
	case "node":
		return "node"
	case "flow":
		return "flow"
	case "api":
		return "api"
	case "engine":
		return "engine"
	default:
		return "agent"
	}
}

// subscribe 는 새 구독자를 등록하고 채널을 반환한다(REQ-L06).
func (h *logStreamHub) subscribe() *logStreamSub {
	s := &logStreamSub{ch: make(chan json.RawMessage, logStreamSubBuffer)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// unsubscribe 는 구독자를 제거한다(teardown — REQ-J08b, 멱등).
func (h *logStreamHub) unsubscribe(s *logStreamSub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// SubscriberCount 는 현재 구독자 수를 반환한다(테스트/관측용).
func (h *logStreamHub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// fanoutLog 는 구독자 채널로 비차단 송신한다(백프레셔 — REQ-J08b). 버퍼가 가득 차면
// 가장 오래된 1개를 비우고 최신값을 넣는다(최신값 우선 coalesce). 느린 소비자가 로그
// 파이프라인(slog)을 막지 않게 한다.
func fanoutLog(ch chan json.RawMessage, v json.RawMessage) {
	select {
	case ch <- v:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}
