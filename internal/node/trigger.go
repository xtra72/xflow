package node

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Sentinel 에러 정의
// ---------------------------------------------------------------------------

var (
	// ErrTriggerNoSchedules 는 스케줄 설정이 없을 때 반환된다.
	ErrTriggerNoSchedules = fmt.Errorf("trigger: %w: schedules not configured", ErrInvalidConfig)

	// ErrTriggerInvalidScheduleType 는 지원하지 않는 스케줄 타입일 때 반환된다.
	ErrTriggerInvalidScheduleType = fmt.Errorf("trigger: %w: unsupported schedule type", ErrInvalidConfig)

	// ErrTriggerInvalidScheduleValue 는 스케줄 값이 유효하지 않을 때 반환된다.
	ErrTriggerInvalidScheduleValue = fmt.Errorf("trigger: %w: invalid schedule value", ErrInvalidConfig)

	// ErrTriggerTimerNotAvailable 는 Timer Agent를 찾을 수 없을 때 반환된다.
	ErrTriggerTimerNotAvailable = fmt.Errorf("trigger: %w: timer agent not available", ErrNodeNotInitialized)

	// ErrTriggerPayloadTemplateFailed 는 템플릿 페이로드 생성에 실패했을 때 반환된다.
	ErrTriggerPayloadTemplateFailed = fmt.Errorf("trigger: payload template evaluation failed")
)

// ---------------------------------------------------------------------------
// 스케줄 타입 정의
// ---------------------------------------------------------------------------

// TriggerScheduleType 은 스케줄 종류를 나타내는 타입이다.
type TriggerScheduleType string

const (
	// TriggerScheduleInterval 은 주기적 인터벌 스케줄이다.
	TriggerScheduleInterval TriggerScheduleType = "interval"

	// TriggerScheduleCron 은 cron 표현식 기반 스케줄이다.
	TriggerScheduleCron TriggerScheduleType = "cron"

	// TriggerScheduleOnce 는 1회성 타임아웃 스케줄이다.
	TriggerScheduleOnce TriggerScheduleType = "once"

	// TriggerScheduleTimes 는 매일 반복 시각 기반 스케줄이다.
	TriggerScheduleTimes TriggerScheduleType = "times"
)

// TriggerSchedule 은 개별 스케줄 설정이다.
type TriggerSchedule struct {
	Type  TriggerScheduleType
	Value any // string 또는 []any (times 타입)
}

// triggerTimerEntry 는 등록된 타이머의 추적 정보이다.
type triggerTimerEntry struct {
	timerID      system.TimerID
	scheduleType TriggerScheduleType
}

// ---------------------------------------------------------------------------
// TriggerNode 구조체
// ---------------------------------------------------------------------------

// TriggerNode 는 스케줄 기반 데이터 자동 생성 SourceNode이다.
// Timer Agent를 사용하여 설정된 스케줄에 따라 메시지를 생성하고 sourceCh로 전달한다.
type TriggerNode struct {
	*BaseNode
	sourceCh  chan message.Message
	timer     system.Timer
	resolver  AgentResolver
	schedules []TriggerSchedule

	// 등록된 타이머 추적
	timerEntries []*triggerTimerEntry
	timerMu      sync.Mutex

	// 페이로드 설정
	payload         any            // 정적 페이로드 (nil이면 기본값 사용)
	payloadTemplate map[string]any // 템플릿 페이로드 (nil이면 사용 안 함)

	// 일시정지 상태
	paused  bool
	pauseMu sync.RWMutex

	logger *slog.Logger
}

// 컴파일 타임 인터페이스 체크
var (
	_ Node       = (*TriggerNode)(nil)
	_ SourceNode = (*TriggerNode)(nil)
)

// ---------------------------------------------------------------------------
// NodeOption
// ---------------------------------------------------------------------------

// WithTimer 는 TriggerNode에 Timer 인터페이스를 직접 주입하는 옵션을 반환한다.
// 시스템 Timer Agent는 AgentResolver를 통해 접근할 수 없으므로
// (agent manager가 아닌 system agent manager에 소속) 엔진 생성 시 이 옵션으로 주입한다.
func WithTimer(timer system.Timer) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_timer_agent"] = timer
	}
}

// ---------------------------------------------------------------------------
// 팩토리 함수
// ---------------------------------------------------------------------------

// NewTriggerNode 는 새로운 TriggerNode를 생성하는 팩토리 함수이다.
func NewTriggerNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)

	// sourceCh 버퍼 크기 결정 (기본 64)
	chSize := 64
	if v, ok := def.Config["source_ch_size"]; ok {
		if size, ok := v.(int); ok && size > 0 {
			chSize = size
		}
	}

	n := &TriggerNode{
		BaseNode: base,
		sourceCh: make(chan message.Message, chSize),
		logger:   slog.Default(),
	}

	// def.Config에서 직접 추출 (팩토리 시점에 사용 가능한 설정)
	if r, ok := def.Config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}

	// NodeOption 경로로 주입된 Timer 는 base.config["_timer_agent"]에 저장되어 있다.
	// BaseNode.config 가 Configure 시점에 def.Config로 덮어씌워지므로, 팩토리 시점에 읽어
	// 노드 인스턴스로 끌어올린다.
	if a, ok := base.config["_timer_agent"]; ok {
		if timer, ok := a.(system.Timer); ok {
			n.timer = timer
		}
	}

	return n, nil
}

// parseScheduleConfig 는 config에서 스케줄 설정을 파싱한다.
func (n *TriggerNode) parseScheduleConfig() {
	rawSchedules, ok := n.config["schedules"]
	if !ok {
		return
	}

	schedList, ok := rawSchedules.([]any)
	if !ok {
		return
	}

	for _, raw := range schedList {
		s, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		schedType, _ := s["type"].(string)
		value := s["value"]
		n.schedules = append(n.schedules, TriggerSchedule{
			Type:  TriggerScheduleType(schedType),
			Value: value,
		})
	}
}

// ---------------------------------------------------------------------------
// Node 인터페이스 메서드
// ---------------------------------------------------------------------------

// Init 은 TriggerNode를 초기화한다.
// Timer Agent를 resolve하고, 스케줄을 등록하고, Running 상태로 전이한다.
func (n *TriggerNode) Init(ctx context.Context) error {
	// Created -> Initializing 전이
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// Configure가 아직 호출되지 않았으면 config에서 직접 파싱
	if len(n.schedules) == 0 && n.config != nil {
		n.parseScheduleConfig()
		if p, ok := n.config["payload"]; ok {
			n.payload = p
		}
		if pt, ok := n.config["payload_template"]; ok {
			if tmpl, ok := pt.(map[string]any); ok {
				n.payloadTemplate = tmpl
			}
		}
		if a, ok := n.config["_timer_agent"]; ok {
			if timer, ok := a.(system.Timer); ok {
				n.timer = timer
			}
		}
		if r, ok := n.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	// 스케줄 유효성 검사
	if len(n.schedules) == 0 {
		_ = n.BaseNode.TransitionTo(lifecycle.StateError)
		return ErrTriggerNoSchedules
	}

	// Timer Agent resolve
	if err := n.resolveTimer(ctx); err != nil {
		_ = n.BaseNode.TransitionTo(lifecycle.StateError)
		return err
	}

	// 스케줄 등록
	if err := n.registerSchedules(); err != nil {
		// 롤백: 이미 등록된 타이머 취소
		n.cancelAllTimers()
		_ = n.BaseNode.TransitionTo(lifecycle.StateError)
		return err
	}

	// Initializing -> Running 전이
	if err := n.BaseNode.TransitionTo(lifecycle.StateRunning); err != nil {
		n.cancelAllTimers()
		return err
	}

	return nil
}

// resolveTimer 는 Timer Agent를 resolve한다.
// 주입 경로 우선순위:
//  1. 이미 n.timer 가 설정되어 있음 (NodeOption WithTimer 를 통해 팩토리에서 주입된 경우)
//  2. Configure 로 전달된 config["_timer_agent"] (테스트 및 런타임 재설정 경로)
func (n *TriggerNode) resolveTimer(_ context.Context) error {
	// 팩토리 시점에 이미 Timer 가 주입되어 있으면 통과
	if n.timer != nil {
		return nil
	}

	// config 경로 (테스트용 직접 주입)
	if a, ok := n.config["_timer_agent"]; ok {
		if timer, ok := a.(system.Timer); ok {
			n.timer = timer
			return nil
		}
	}

	return ErrTriggerTimerNotAvailable
}

// registerSchedules 는 모든 스케줄을 Timer Agent에 등록한다.
func (n *TriggerNode) registerSchedules() error {
	for i, sched := range n.schedules {
		entry, err := n.registerSingleSchedule(i, sched)
		if err != nil {
			return err
		}
		n.timerMu.Lock()
		n.timerEntries = append(n.timerEntries, entry)
		n.timerMu.Unlock()
	}
	return nil
}

// registerSingleSchedule 는 하나의 스케줄을 Timer Agent에 등록한다.
func (n *TriggerNode) registerSingleSchedule(index int, sched TriggerSchedule) (*triggerTimerEntry, error) {
	entry := &triggerTimerEntry{
		scheduleType: sched.Type,
	}

	switch sched.Type {
	case TriggerScheduleInterval:
		return n.registerInterval(index, sched, entry)
	case TriggerScheduleCron:
		return n.registerCron(index, sched, entry)
	case TriggerScheduleOnce:
		return n.registerOnce(index, sched, entry)
	case TriggerScheduleTimes:
		return n.registerTimes(index, sched, entry)
	default:
		return nil, fmt.Errorf("%w: %q", ErrTriggerInvalidScheduleType, sched.Type)
	}
}

// registerInterval 은 interval 타입 스케줄을 등록한다.
func (n *TriggerNode) registerInterval(index int, sched TriggerSchedule, entry *triggerTimerEntry) (*triggerTimerEntry, error) {
	valueStr, ok := sched.Value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: interval value must be a string", ErrTriggerInvalidScheduleValue)
	}
	duration, err := time.ParseDuration(valueStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTriggerInvalidScheduleValue, err)
	}

	timerID := fmt.Sprintf("%s-interval-%d", n.Name(), index)
	id, err := n.timer.SetInterval(timerID, duration, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return nil, fmt.Errorf("trigger: SetInterval failed: %w", err)
	}
	entry.timerID = id
	return entry, nil
}

// registerCron 은 cron 타입 스케줄을 등록한다.
func (n *TriggerNode) registerCron(index int, sched TriggerSchedule, entry *triggerTimerEntry) (*triggerTimerEntry, error) {
	cronExpr, ok := sched.Value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: cron value must be a string", ErrTriggerInvalidScheduleValue)
	}

	timerID := fmt.Sprintf("%s-cron-%d", n.Name(), index)
	id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return nil, fmt.Errorf("trigger: SetCron failed: %w", err)
	}
	entry.timerID = id
	return entry, nil
}

// registerOnce 는 once 타입 스케줄을 등록한다.
func (n *TriggerNode) registerOnce(index int, sched TriggerSchedule, entry *triggerTimerEntry) (*triggerTimerEntry, error) {
	valueStr, ok := sched.Value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: once value must be a string", ErrTriggerInvalidScheduleValue)
	}

	targetTime, err := time.Parse(time.RFC3339, valueStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTriggerInvalidScheduleValue, err)
	}

	delay := time.Until(targetTime)
	if delay <= 0 {
		return nil, fmt.Errorf("%w: once target time is in the past", ErrTriggerInvalidScheduleValue)
	}

	timerID := fmt.Sprintf("%s-once-%d", n.Name(), index)
	id, err := n.timer.SetTimeout(timerID, delay, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return nil, fmt.Errorf("trigger: SetTimeout failed: %w", err)
	}
	entry.timerID = id
	return entry, nil
}

// registerTimes 는 times 타입 스케줄을 등록한다.
// 각 "HH:MM" 시각을 cron 표현식 "MM HH * * *"로 변환하여 등록한다.
func (n *TriggerNode) registerTimes(index int, sched TriggerSchedule, entry *triggerTimerEntry) (*triggerTimerEntry, error) {
	timesList, ok := sched.Value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: times value must be an array", ErrTriggerInvalidScheduleValue)
	}

	// times는 여러 개의 cron을 등록하므로, 첫 번째만 entry에 기록하고
	// 나머지는 별도 entry로 추가한다
	for i, raw := range timesList {
		timeStr, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%w: times value must be string array", ErrTriggerInvalidScheduleValue)
		}

		parts := strings.Split(timeStr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: invalid time format %q (expected HH:MM)", ErrTriggerInvalidScheduleValue, timeStr)
		}

		hour, errH := strconv.Atoi(parts[0])
		minute, errM := strconv.Atoi(parts[1])
		if errH != nil || errM != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return nil, fmt.Errorf("%w: invalid time %q", ErrTriggerInvalidScheduleValue, timeStr)
		}

		cronExpr := fmt.Sprintf("%d %d * * *", minute, hour)
		timerID := fmt.Sprintf("%s-times-%d-%d", n.Name(), index, i)

		currentEntry := entry
		if i > 0 {
			currentEntry = &triggerTimerEntry{
				scheduleType: TriggerScheduleTimes,
			}
		}

		id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(currentEntry, "times", timerID))
		if err != nil {
			return nil, fmt.Errorf("trigger: SetCron (times) failed: %w", err)
		}
		currentEntry.timerID = id

		if i > 0 {
			n.timerMu.Lock()
			n.timerEntries = append(n.timerEntries, currentEntry)
			n.timerMu.Unlock()
		}
	}

	return entry, nil
}

// makeHandler 는 타이머 핸들러 함수를 생성한다.
func (n *TriggerNode) makeHandler(entry *triggerTimerEntry, scheduleType string, timerID string) system.TimerHandler {
	return func(trigger system.TimerTrigger) {
		// 일시정지 상태 확인
		n.pauseMu.RLock()
		isPaused := n.paused
		n.pauseMu.RUnlock()
		if isPaused {
			return
		}

		// 트리거에서 전달된 틱 카운트 사용
		tickCount := trigger.TickCount

		// 메시지 생성
		msg := n.buildMessage(trigger, scheduleType, timerID, tickCount)

		// sourceCh로 전송 (non-blocking)
		select {
		case n.sourceCh <- msg:
			// 전송 성공
		default:
			// 버퍼 가득 참 → 드롭 + 경고 로그
			n.logger.Warn("trigger: sourceCh full, dropping message",
				"node", n.Name(),
				"timer_id", timerID,
				"tick_count", tickCount,
			)
		}
	}
}

// buildMessage 는 트리거 이벤트로부터 메시지를 생성한다.
func (n *TriggerNode) buildMessage(trigger system.TimerTrigger, scheduleType string, timerID string, tickCount int64) message.Message {
	triggerTime := trigger.TriggerAt.UTC()
	triggerTimeStr := triggerTime.Format(time.RFC3339Nano)

	// 페이로드 생성
	var payload message.Payload
	var templateErr string
	if n.payloadTemplate != nil {
		payload, templateErr = n.templatePayload(trigger, scheduleType, timerID, tickCount)
	} else if n.payload != nil {
		payload = n.staticPayload()
	} else {
		// 기본 페이로드
		payload = message.NewPayload(map[string]any{
			"trigger_time": triggerTimeStr,
		})
	}

	// 메시지 생성 옵션
	opts := []message.Option{
		message.WithPayload(payload),
		message.WithMetadata("trigger.schedule_type", scheduleType),
		message.WithMetadata("trigger.schedule_id", timerID),
		message.WithMetadata("trigger.tick_count", strconv.FormatInt(tickCount, 10)),
		message.WithMetadata("trigger.trigger_time", triggerTimeStr),
		message.WithMetadata("trigger.node_name", n.Name()),
		message.WithMetadata("message_type", "event"),
	}

	// 템플릿 에러 시 메타데이터 추가
	if templateErr != "" {
		opts = append(opts, message.WithMetadata("trigger.error", templateErr))
	}

	return message.New(opts...)
}

// staticPayload 는 정적 페이로드를 생성한다.
func (n *TriggerNode) staticPayload() message.Payload {
	switch v := n.payload.(type) {
	case map[string]any:
		return message.NewPayload(deepCopyMap(v))
	default:
		// 단일 값을 map으로 래핑
		return message.NewPayload(map[string]any{"value": v})
	}
}

// templatePayload 는 템플릿 기반 동적 페이로드를 생성한다.
// 에러 발생 시 에러 메시지 문자열도 반환한다.
func (n *TriggerNode) templatePayload(trigger system.TimerTrigger, _ string, _ string, tickCount int64) (message.Payload, string) {
	triggerTimeStr := trigger.TriggerAt.UTC().Format(time.RFC3339Nano)

	// 변수 맵
	vars := map[string]any{
		"$.trigger_time": triggerTimeStr,
		"$.tick_count":   tickCount,
		"$.schedule_id":  trigger.ScheduleID,
		"$.trigger_id":   n.Name(),
	}

	result := make(map[string]any)
	var errMsg string

	for key, tmplVal := range n.payloadTemplate {
		strVal, ok := tmplVal.(string)
		if !ok {
			result[key] = tmplVal
			continue
		}

		if resolved, exists := vars[strVal]; exists {
			result[key] = resolved
		} else {
			// 알 수 없는 변수 → 에러
			errMsg = fmt.Sprintf("unknown template variable: %s", strVal)
			result[key] = nil
		}
	}

	if errMsg != "" {
		n.logger.Warn("trigger: payload template error",
			"node", n.Name(),
			"error", errMsg,
		)
	}

	return message.NewPayload(result), errMsg
}

// Shutdown 은 TriggerNode를 종료한다.
func (n *TriggerNode) Shutdown(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateStopping); err != nil {
		return err
	}

	n.cancelAllTimers()
	close(n.sourceCh)

	return n.BaseNode.TransitionTo(lifecycle.StateStopped)
}

// cancelAllTimers 는 등록된 모든 타이머를 취소한다.
func (n *TriggerNode) cancelAllTimers() {
	n.timerMu.Lock()
	defer n.timerMu.Unlock()

	if n.timer == nil {
		return
	}

	for _, entry := range n.timerEntries {
		_ = n.timer.Cancel(entry.timerID)
	}
	n.timerEntries = nil
}

// Process 는 TriggerNode에서 사용되지 않는다 (SourceNode이므로).
// 빈 슬라이스를 반환한다.
func (n *TriggerNode) Process(_ context.Context, _ message.Message) ([]message.Message, error) {
	return []message.Message{}, nil
}

// SourceCh 는 메시지 출력 채널을 반환한다.
func (n *TriggerNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Configure 는 TriggerNode의 설정을 적용한다.
// 스케줄, 페이로드, Timer Agent 직접 주입 등의 설정을 파싱한다.
func (n *TriggerNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// 스케줄 설정 파싱
	n.schedules = nil
	n.parseScheduleConfig()

	// 페이로드 설정
	n.payload = nil
	n.payloadTemplate = nil
	if p, ok := config["payload"]; ok {
		n.payload = p
	}
	if pt, ok := config["payload_template"]; ok {
		if tmpl, ok := pt.(map[string]any); ok {
			n.payloadTemplate = tmpl
		}
	}

	// Timer Agent 직접 주입 (테스트용)
	if a, ok := config["_timer_agent"]; ok {
		if timer, ok := a.(system.Timer); ok {
			n.timer = timer
		}
	}

	// AgentResolver 갱신
	if r, ok := config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}

	// sourceCh 버퍼 크기 재설정
	if v, ok := config["source_ch_size"]; ok {
		if size, ok := v.(int); ok && size > 0 {
			n.sourceCh = make(chan message.Message, size)
		}
	}

	return nil
}

// Pause 는 메시지 생성을 일시정지한다.
func (n *TriggerNode) Pause(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StatePaused); err != nil {
		return err
	}
	n.pauseMu.Lock()
	n.paused = true
	n.pauseMu.Unlock()
	return nil
}

// Resume 은 일시정지된 메시지 생성을 재개한다.
func (n *TriggerNode) Resume(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateRunning); err != nil {
		return err
	}
	n.pauseMu.Lock()
	n.paused = false
	n.pauseMu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// 헬퍼 함수
// ---------------------------------------------------------------------------

// deepCopyMap 은 map[string]any를 깊은 복사한다.
func deepCopyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case map[string]any:
			dst[k] = deepCopyMap(val)
		case []any:
			cp := make([]any, len(val))
			copy(cp, val)
			dst[k] = cp
		default:
			dst[k] = v
		}
	}
	return dst
}
