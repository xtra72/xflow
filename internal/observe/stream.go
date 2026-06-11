package observe

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

// componentAttrKey 는 로그 속성에서 컴포넌트를 식별하는 키 이름이다.
const componentAttrKey = "component"

// StreamRouter 는 컴포넌트별 로그 스트림 라우팅을 관리하는 인터페이스이다.
type StreamRouter interface {
	// AddRoute 는 컴포넌트에 Writer 를 추가한다.
	AddRoute(component string, writer io.Writer)

	// RemoveRoute 는 컴포넌트에서 특정 Writer 를 제거한다.
	RemoveRoute(component string, writer io.Writer)

	// Routes 는 컴포넌트에 등록된 Writer 목록의 복사본을 반환한다.
	Routes(component string) []io.Writer

	// SetDefaultWriter 는 기본 출력 Writer 를 교체한다.
	SetDefaultWriter(writer io.Writer)

	// Handler 는 내부 routingHandler 를 slog.Handler 로 반환한다.
	Handler() slog.Handler
}

// streamRouter 는 StreamRouter 의 구현체이다.
type streamRouter struct {
	mu            sync.RWMutex
	routes        map[string][]io.Writer
	defaultWriter io.Writer
	format        string
	levelManager  LevelManager
}

// NewStreamRouter 는 새 StreamRouter 를 생성한다.
func NewStreamRouter(opts ...StreamOption) StreamRouter {
	cfg := &streamConfig{
		defaultWriter: os.Stdout,
		format:        "json",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &streamRouter{
		routes:        make(map[string][]io.Writer),
		defaultWriter: cfg.defaultWriter,
		format:        cfg.format,
		levelManager:  cfg.levelManager,
	}
}

// AddRoute 는 컴포넌트에 Writer 를 추가한다.
func (sr *streamRouter) AddRoute(component string, writer io.Writer) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.routes[component] = append(sr.routes[component], writer)
}

// RemoveRoute 는 컴포넌트에서 특정 Writer 를 제거한다.
// Writer 비교는 포인터 동일성으로 수행한다.
func (sr *streamRouter) RemoveRoute(component string, writer io.Writer) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	writers := sr.routes[component]
	for i, w := range writers {
		if w == writer {
			sr.routes[component] = append(writers[:i], writers[i+1:]...)
			break
		}
	}

	// 빈 슬라이스면 항목 자체를 삭제한다
	if len(sr.routes[component]) == 0 {
		delete(sr.routes, component)
	}
}

// Routes 는 컴포넌트에 등록된 Writer 목록의 복사본을 반환한다.
func (sr *streamRouter) Routes(component string) []io.Writer {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	writers := sr.routes[component]
	if len(writers) == 0 {
		return nil
	}

	// 복사본을 반환한다
	result := make([]io.Writer, len(writers))
	copy(result, writers)
	return result
}

// SetDefaultWriter 는 기본 출력 Writer 를 교체한다.
func (sr *streamRouter) SetDefaultWriter(writer io.Writer) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.defaultWriter = writer
}

// Handler 는 라우팅 핸들러를 slog.Handler 로 반환한다.
func (sr *streamRouter) Handler() slog.Handler {
	return &routingHandler{
		router: sr,
		format: sr.format,
	}
}

// routingHandler 는 slog.Handler 를 구현하여 컴포넌트별 로그 라우팅을 수행한다.
type routingHandler struct {
	router *streamRouter
	attrs  []slog.Attr
	groups []string
	format string
}

// Enabled 는 해당 레벨의 로그가 활성화되었는지 확인한다.
// LevelManager 가 설정된 경우 컴포넌트별 레벨을 확인한다.
func (h *routingHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.router.levelManager == nil {
		return true
	}

	// attrs 에서 component 를 추출한다
	component := h.extractComponent()
	if component == "" {
		// 컴포넌트 정보가 없으면 기본 레벨로 확인한다
		return level >= h.router.levelManager.DefaultLevel()
	}

	return level >= h.router.levelManager.GetLevel(component)
}

// Handle 은 로그 레코드를 적절한 Writer 로 라우팅한다.
func (h *routingHandler) Handle(ctx context.Context, record slog.Record) error {
	// record 속성에서 component 를 추출한다
	component := h.extractComponentFromRecord(record)

	// 대상 Writer 를 결정한다
	writers := h.router.Routes(component)
	if len(writers) == 0 {
		// 라우트가 없으면 기본 Writer 사용
		h.router.mu.RLock()
		dw := h.router.defaultWriter
		h.router.mu.RUnlock()
		writers = []io.Writer{dw}
	}

	// 모든 대상 Writer 에 팬아웃한다
	for _, w := range writers {
		h.writeToTarget(ctx, record, w)
	}

	return nil
}

// writeToTarget 은 레코드를 지정된 Writer 에 포맷팅하여 기록한다.
// Writer 에러는 무시하고 다음 Writer 로 계속 진행한다.
func (h *routingHandler) writeToTarget(ctx context.Context, record slog.Record, w io.Writer) {
	var handler slog.Handler
	if h.format == "text" {
		handler = slog.NewTextHandler(w, logHandlerOpts)
	} else {
		handler = slog.NewJSONHandler(w, logHandlerOpts)
	}

	// 내부 라우팅 키(component)를 제외하고 attrs 를 적용한다
	if len(h.attrs) > 0 {
		filtered := make([]slog.Attr, 0, len(h.attrs))
		for _, attr := range h.attrs {
			if attr.Key == componentAttrKey {
				continue
			}
			filtered = append(filtered, attr)
		}
		if len(filtered) > 0 {
			handler = handler.WithAttrs(filtered)
		}
	}

	// 기존 groups 를 적용한다
	for _, g := range h.groups {
		handler = handler.WithGroup(g)
	}

	// 에러 무시 (에러 허용 정책)
	_ = handler.Handle(ctx, record)
}

// WithAttrs 는 속성이 추가된 새 핸들러를 반환한다. (불변 패턴)
func (h *routingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)

	newGroups := make([]string, len(h.groups))
	copy(newGroups, h.groups)

	return &routingHandler{
		router: h.router,
		attrs:  newAttrs,
		groups: newGroups,
		format: h.format,
	}
}

// WithGroup 은 그룹이 추가된 새 핸들러를 반환한다. (불변 패턴)
func (h *routingHandler) WithGroup(name string) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs))
	copy(newAttrs, h.attrs)

	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)

	return &routingHandler{
		router: h.router,
		attrs:  newAttrs,
		groups: newGroups,
		format: h.format,
	}
}

// extractComponent 는 핸들러에 미리 설정된 attrs 에서 component 값을 추출한다.
func (h *routingHandler) extractComponent() string {
	for _, attr := range h.attrs {
		if attr.Key == componentAttrKey {
			return attr.Value.String()
		}
	}
	return ""
}

// extractComponentFromRecord 는 레코드 속성에서 component 값을 추출한다.
// 레코드의 attrs 를 먼저 확인하고, 없으면 핸들러의 attrs 에서 찾는다.
func (h *routingHandler) extractComponentFromRecord(record slog.Record) string {
	var component string

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == componentAttrKey {
			component = attr.Value.String()
			return false // 찾았으므로 중단
		}
		return true
	})

	if component != "" {
		return component
	}

	// 핸들러에 미리 설정된 attrs 확인
	return h.extractComponent()
}
