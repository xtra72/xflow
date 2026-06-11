package engine

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// countingHandler 는 WARN 로그 호출 횟수를 세는 테스트용 slog.Handler 이다.
type countingHandler struct {
	mu    sync.Mutex
	warns int
	msgs  []string
}

func (h *countingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.mu.Lock()
		h.warns++
		h.msgs = append(h.msgs, r.Message)
		h.mu.Unlock()
	}
	return nil
}

func (h *countingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *countingHandler) warnCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.warns
}

// warnCountContaining 은 WARN 메시지 중 substr 을 포함하는 건수를 반환한다.
// 특정 경고(예: 미연결 출력 포트 경고)만 선별 검증할 때 사용한다.
func (h *countingHandler) warnCountContaining(substr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, m := range h.msgs {
		if strings.Contains(m, substr) {
			n++
		}
	}
	return n
}

// testComponentLogger 는 countingHandler 를 감싸 ComponentLogger 인터페이스를
// 만족시키는 최소 테스트 더블이다.
type testComponentLogger struct {
	l *slog.Logger
}

func newTestLogger(h slog.Handler) observe.ComponentLogger {
	return &testComponentLogger{l: slog.New(h)}
}

func (t *testComponentLogger) Debug(msg string, args ...any) { t.l.Debug(msg, args...) }
func (t *testComponentLogger) Info(msg string, args ...any)  { t.l.Info(msg, args...) }
func (t *testComponentLogger) Warn(msg string, args ...any)  { t.l.Warn(msg, args...) }
func (t *testComponentLogger) Error(msg string, args ...any) { t.l.Error(msg, args...) }
func (t *testComponentLogger) With(args ...any) observe.ComponentLogger {
	return &testComponentLogger{l: t.l.With(args...)}
}
func (t *testComponentLogger) WithGroup(name string) observe.ComponentLogger {
	return &testComponentLogger{l: t.l.WithGroup(name)}
}
func (t *testComponentLogger) Component() string    { return "test" }
func (t *testComponentLogger) Logger() *slog.Logger { return t.l }

// TestSendToWires_DeliveredCount 는 sendToWires 가 실제 전달된 와이어 수를
// 반환하는지 검증한다 (연결된 와이어가 있을 때).
func TestSendToWires_DeliveredCount(t *testing.T) {
	e := &Engine{}
	w1 := &RuntimeWire{ID: "w1", Ch: make(chan message.Message, 1), Mode: flow.WireBuffer}
	w2 := &RuntimeWire{ID: "w2", Ch: make(chan message.Message, 1), Mode: flow.WireBuffer}

	delivered := e.sendToWires(context.Background(), message.New(), []*RuntimeWire{w1, w2}, "n1")
	if delivered != 2 {
		t.Errorf("delivered = %d, want 2", delivered)
	}
}

// TestSendToWires_EmptyWires_ZeroDelivered 는 와이어가 없으면 delivered 0 을
// 반환하는지 검증한다.
func TestSendToWires_EmptyWires_ZeroDelivered(t *testing.T) {
	e := &Engine{}
	delivered := e.sendToWires(context.Background(), message.New(), nil, "n1")
	if delivered != 0 {
		t.Errorf("와이어 없을 때 delivered = %d, want 0", delivered)
	}
}

// TestWarnUnconnectedPort_Once 는 동일 (nodeID, portName) 조합에 대해 미연결
// 경고가 반복 호출에도 단 한 번만 로깅되는지 검증한다.
func TestWarnUnconnectedPort_Once(t *testing.T) {
	h := &countingHandler{}
	e := &Engine{logger: newTestLogger(h)}

	// 동일 포트에 대해 여러 번 경고 시도 (pc=nil 경로: 폴백 dedupe 맵 사용)
	for i := 0; i < 10; i++ {
		e.warnUnconnectedPort("n1", "노드1", "out", nil)
	}

	if got := h.warnCount(); got != 1 {
		t.Errorf("동일 포트 미연결 경고 횟수 = %d, want 1 (스팸 방지)", got)
	}
}

// TestWarnUnconnectedPort_PerPort 는 서로 다른 (nodeID, portName) 조합은
// 각각 한 번씩 경고되는지 검증한다.
func TestWarnUnconnectedPort_PerPort(t *testing.T) {
	h := &countingHandler{}
	e := &Engine{logger: newTestLogger(h)}

	e.warnUnconnectedPort("n1", "노드1", "out", nil)
	e.warnUnconnectedPort("n1", "노드1", "error", nil)
	e.warnUnconnectedPort("n2", "노드2", "out", nil)
	// 반복 (추가 경고 없어야 함)
	e.warnUnconnectedPort("n1", "노드1", "out", nil)
	e.warnUnconnectedPort("n2", "노드2", "out", nil)

	if got := h.warnCount(); got != 3 {
		t.Errorf("서로 다른 포트 경고 횟수 = %d, want 3", got)
	}
}
