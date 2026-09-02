package node

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	// TriggerScheduleWeekly 는 특정 요일 × 시각 반복 스케줄이다. (v1.2.0)
	TriggerScheduleWeekly TriggerScheduleType = "weekly"

	// TriggerScheduleMonthly 는 특정 일자/first/last × 시각 반복 스케줄이다. (v1.2.0)
	TriggerScheduleMonthly TriggerScheduleType = "monthly"
)

// TriggerSchedule 은 개별 스케줄 설정이다.
type TriggerSchedule struct {
	Type  TriggerScheduleType
	Value any // interval/cron/once: string, times: []any

	// weekly (v1.2.0): 요일 × 시각 다중 조합
	Days []string // 요일 토큰 (sun~sat 또는 0~6). weekly 전용
	// weekly/monthly (v1.2.0): 시각 배열 ("HH:MM")
	Times []string // weekly/monthly 전용 (times 타입은 Value 사용, 하위 호환)
	// monthly (v1.2.0): 일자 (정수 1~31 | "first" | "last")
	Day any // monthly 전용

	// per-schedule payload (v1.2.0): 스케줄 항목별 페이로드 오버라이드 (선택)
	// nil이면 노드 레벨 payload/payloadTemplate 로 폴백한다.
	Payload     any            // 이 스케줄의 정적 페이로드 (선택)
	PayloadTmpl map[string]any // 이 스케줄의 템플릿 페이로드 (선택)

	// 규칙 메타 (SPEC-TRIGGER-SCHED-001 RD-1 / RD-8): 설비 제어 예약 규칙의
	// 표시·유효기간·우선순위·활성 상태. 확장 필드가 config 에 없으면 하위 호환
	// 기본값(Name=""; ValidFrom/ValidTo=""(무기한); Priority=0; Enabled=nil→true)이
	// 적용되어 기존 스케줄이 무회귀로 계속 발화한다. (REQ-SCHED-01-01/02)
	Name      string // 규칙 표시 이름 (config `name`). 발화 비영향 pass-through 메타.
	ValidFrom string // 유효 시작 (config `valid_from`, `YYYY-MM-DD`). 빈 값=하한 무제한.
	ValidTo   string // 유효 종료 (config `valid_to`, `YYYY-MM-DD`). 빈 값=무기한.
	Priority  int    // 표시/정렬 우선순위 (config `priority`). 발화 비영향 pass-through 메타.
	// AgentID 는 스케줄이 선언한 대상(설비 제어) 에이전트이다(config `agent_id`,
	// SPEC-SCHEDULE-VIEW-001 M2). 스케줄 로그의 DeclaredAgentID(선언된 대상)로 실린다.
	// 빈 값=대상 미선언(하위 호환) — 발화 비영향 pass-through 메타이며, 값이 있을 때만
	// 방출 메시지에 `trigger.agent_id` 메타로 통과 전달된다(무회귀, AC-13).
	AgentID string
	// Enabled 는 활성 여부(config `enabled`)이다. 포인터로 tri-state 를 표현하여
	// config 부재(nil)를 명시적 false 와 구분한다 — nil=부재=활성(기본 true).
	Enabled *bool
}

// triggerTimerEntry 는 등록된 타이머의 추적 정보이다.
type triggerTimerEntry struct {
	timerID      system.TimerID
	scheduleType TriggerScheduleType

	// per-schedule payload (v1.2.0): 해당 스케줄이 지정한 페이로드 소스.
	// 둘 다 nil이면 노드 레벨 → 기본 페이로드 순으로 폴백한다.
	payload     any
	payloadTmpl map[string]any

	// lastDayGate (v1.2.0): monthly "last" 스케줄이면 true.
	// 핸들러 진입 시 현재 날짜가 해당 월의 마지막 날일 때만 emit한다.
	lastDayGate bool

	// 규칙 메타 (SPEC-TRIGGER-SCHED-001 RD-8): 발화 게이팅에 필요한 값을 등록 시점에
	// 캡처한다. 엔트리 로컬 불변이므로(payload 와 동일) 발화 핸들러에서 lock 없이 읽는다.
	//   enabled  : 활성 여부(config 부재 시 true 로 해소). false 면 발화 시 emit 하지 않음.
	//   validFrom/validTo : 유효기간 경계(빈 값=해당 방향 무제한). 밖이면 emit 하지 않음.
	//   name/priority : 발화 비영향 pass-through 메타(메시지 메타데이터로 전달).
	enabled   bool
	validFrom string
	validTo   string
	name      string
	priority  int
	// agentID (SPEC-SCHEDULE-VIEW-001 M2): 스케줄이 선언한 대상 에이전트. 발화 시
	// 스케줄 로그 fire 이벤트의 DeclaredAgentID + `trigger.agent_id` 메타 전달에 쓴다.
	// name/priority 와 동일한 발화 비영향 pass-through 메타(엔트리 로컬 불변, lock 없이 읽음).
	agentID string

	// gen (SPEC-TRIGGER-PANEL-001 RD-6): 이 엔트리가 등록된 시점의 re-arm 세대.
	// 발화 시 노드의 현재 rearmGen 과 불일치하면 live 재무장으로 무효화된 stale
	// in-flight 발화이므로 폐기(drop)된다 — cancel→re-register 창의 중복 발화 방지.
	gen uint64
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

	// rearmGen (SPEC-TRIGGER-PANEL-001 RD-6): live 재무장(re-arm) 세대 카운터.
	// Configure 가 running 노드에서 타이머를 재등록할 때마다 증가한다. 각 등록
	// 타이머 엔트리는 등록 시점의 세대를 캡처하며(entry.gen), 발화 시 자신의 세대가
	// 현재 세대와 다르면 폐기된다 — cancel→re-register 창의 stale in-flight 발화 제거.
	// atomic 으로 접근하여 timer agent 고루틴의 발화와 Configure 고루틴의 증가가
	// 경합해도 race-free 하다.
	rearmGen atomic.Uint64

	// started (SPEC-TRIGGER-PANEL-001 RD-2): Init 완료(타이머 최초 등록) 여부.
	// Init 성공 말미에 true, Shutdown 시 false. Configure 는 started && Running 일
	// 때만 live 재무장한다. 최초 Configure(Init 이전, Created 상태)에서는 재등록하지
	// 않아 Configure+Init 이중 등록을 방지한다.
	started atomic.Bool

	// 페이로드 설정
	payload         any            // 정적 페이로드 (nil이면 기본값 사용)
	payloadTemplate map[string]any // 템플릿 페이로드 (nil이면 사용 안 함)

	// payloadMu (SPEC-TRIGGER-PANEL-001): 노드 레벨 payload/payloadTemplate 필드를
	// 보호한다. live Configure(재무장)가 이 필드를 리셋하는 동안 발화 핸들러의
	// buildMessage 가 폴백으로 읽을 수 있어(cancel→re-register 창) 경합이 발생한다.
	// per-schedule payload(entry.payload/payloadTmpl)는 엔트리 로컬 불변이라 무관.
	payloadMu sync.RWMutex

	// 일시정지 상태
	paused  bool
	pauseMu sync.RWMutex

	// nowFunc 는 현재 시각 조회 훅이다. 기본값 time.Now.
	// monthly "last" 월말 게이트에서 사용하며, 테스트가 고정 clock을 주입한다. (v1.2.0)
	nowFunc func() time.Time

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
		nowFunc:  time.Now,
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
		sched := TriggerSchedule{
			Type:  TriggerScheduleType(schedType),
			Value: s["value"],
		}

		// weekly/monthly 필드 (v1.2.0)
		if d, ok := s["days"]; ok {
			sched.Days = anySliceToStrings(d)
		}
		if t, ok := s["times"]; ok {
			sched.Times = anySliceToStrings(t)
		}
		if day, ok := s["day"]; ok {
			sched.Day = day
		}

		// per-schedule payload (v1.2.0)
		if p, ok := s["payload"]; ok {
			sched.Payload = p
		}
		if pt, ok := s["payload_template"]; ok {
			if tmpl, ok := pt.(map[string]any); ok {
				sched.PayloadTmpl = tmpl
			}
		}

		// 규칙 메타 (SPEC-TRIGGER-SCHED-001 RD-1 / RD-8): 확장 필드 파싱.
		// 키가 없거나 타입이 다르면 하위 호환 기본값을 유지한다(REQ-SCHED-01-02).
		if name, ok := s["name"].(string); ok {
			sched.Name = name
		}
		if vf, ok := s["valid_from"].(string); ok {
			sched.ValidFrom = vf
		}
		if vt, ok := s["valid_to"].(string); ok {
			sched.ValidTo = vt
		}
		if p, ok := s["priority"]; ok {
			sched.Priority = anyToInt(p)
		}
		// agent_id: 스케줄이 선언한 대상 에이전트(SPEC-SCHEDULE-VIEW-001 M2). 키가 없거나
		// 타입이 다르면 빈 값(대상 미선언)을 유지한다 — 무회귀(AC-13).
		if a, ok := s["agent_id"].(string); ok {
			sched.AgentID = a
		}
		// enabled: 부재(키 없음)면 nil 을 유지해 발화 게이트에서 true 로 해소된다.
		// bool 값이 명시된 경우에만 포인터를 세팅한다(명시 false 를 부재와 구분).
		if e, ok := s["enabled"].(bool); ok {
			sched.Enabled = &e
		}

		n.schedules = append(n.schedules, sched)
	}
}

// anySliceToStrings 는 []any 를 []string 으로 변환한다.
// 문자열은 그대로, 숫자(요일 0~6 등)는 정수 문자열로 변환한다.
func anySliceToStrings(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		out = append(out, anyToToken(e))
	}
	return out
}

// anyToInt 은 config 에서 온 임의의 수치 값을 int 로 변환한다.
// JSON 디코딩 경로는 숫자를 float64 로, Go 리터럴 config(테스트)는 int 로 전달하므로
// 두 표현을 모두 수용한다. 문자열 정수도 관용적으로 허용한다. 변환 불가 시 0(기본값).
func anyToInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

// anyToToken 은 단일 값을 토큰 문자열로 변환한다.
func anyToToken(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.Itoa(int(x))
	default:
		return fmt.Sprintf("%v", x)
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
		// payloadMu 로 보호하여 노드 레벨 payload 접근 규율을 통일한다(발화 이전 경로).
		n.payloadMu.Lock()
		if p, ok := n.config["payload"]; ok {
			n.payload = p
		}
		if pt, ok := n.config["payload_template"]; ok {
			if tmpl, ok := pt.(map[string]any); ok {
				n.payloadTemplate = tmpl
			}
		}
		n.payloadMu.Unlock()
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

	// 최초 등록 완료 — 이후 Configure 는 live 재무장 대상이 된다 (RD-2).
	n.started.Store(true)

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
		if err := n.registerSingleSchedule(i, sched); err != nil {
			return err
		}
	}
	return nil
}

// registerSingleSchedule 는 하나의 스케줄을 Timer Agent에 등록한다.
// 등록된 타이머는 각 register* 함수가 addEntry 로 timerEntries 에 직접 추가한다.
func (n *TriggerNode) registerSingleSchedule(index int, sched TriggerSchedule) error {
	switch sched.Type {
	case TriggerScheduleInterval:
		return n.registerInterval(index, sched)
	case TriggerScheduleCron:
		return n.registerCron(index, sched)
	case TriggerScheduleOnce:
		return n.registerOnce(index, sched)
	case TriggerScheduleTimes:
		return n.registerTimes(index, sched)
	case TriggerScheduleWeekly:
		return n.registerWeekly(index, sched)
	case TriggerScheduleMonthly:
		return n.registerMonthly(index, sched)
	default:
		return fmt.Errorf("%w: %q", ErrTriggerInvalidScheduleType, sched.Type)
	}
}

// newEntry 는 스케줄로부터 타이머 엔트리를 생성한다.
// 스케줄 항목의 per-schedule 페이로드(Payload/PayloadTmpl)를 엔트리로 전달하며,
// 둘 다 nil이면 발화 시 노드 레벨 → 기본 페이로드로 폴백한다. (v1.2.0)
func (n *TriggerNode) newEntry(sched TriggerSchedule, lastDayGate bool) *triggerTimerEntry {
	return &triggerTimerEntry{
		scheduleType: sched.Type,
		payload:      sched.Payload,
		payloadTmpl:  sched.PayloadTmpl,
		lastDayGate:  lastDayGate,
		// 규칙 메타 캡처 (SPEC-TRIGGER-SCHED-001 RD-8): enabled 는 config 부재(nil)
		// 시 true 로 해소하여 하위 호환(기존 스케줄 무회귀)을 보장한다 (REQ-SCHED-01-02).
		enabled:   sched.Enabled == nil || *sched.Enabled,
		validFrom: sched.ValidFrom,
		validTo:   sched.ValidTo,
		name:      sched.Name,
		priority:  sched.Priority,
		agentID:   sched.AgentID,
		// 등록 시점의 re-arm 세대를 캡처한다. registerSchedules 는 rearmGen 증가
		// 이후 동일 고루틴에서 호출되므로 항상 최신 세대를 읽는다 (RD-6).
		gen: n.rearmGen.Load(),
	}
}

// addEntry 는 등록된 타이머 엔트리를 잠금 하에 추가한다.
func (n *TriggerNode) addEntry(e *triggerTimerEntry) {
	n.timerMu.Lock()
	n.timerEntries = append(n.timerEntries, e)
	n.timerMu.Unlock()
}

// registerInterval 은 interval 타입 스케줄을 등록한다.
func (n *TriggerNode) registerInterval(index int, sched TriggerSchedule) error {
	valueStr, ok := sched.Value.(string)
	if !ok {
		return fmt.Errorf("%w: interval value must be a string", ErrTriggerInvalidScheduleValue)
	}
	duration, err := time.ParseDuration(valueStr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTriggerInvalidScheduleValue, err)
	}

	// 타이머 ID 는 노드 고유 ID(UUID) 기반으로 만든다. 노드 이름은 import/복제 시
	// 동일하게 유지되어, 같은 이름의 trigger 노드를 가진 두 플로우를 동시 실행하면
	// 전역 timer agent 에서 "duplicate timer ID" 충돌이 발생한다. 노드 ID 는 생성·
	// import 재발급 시 항상 고유하므로 플로우 간 충돌을 방지한다.
	timerID := fmt.Sprintf("%s-interval-%d", n.ID(), index)
	entry := n.newEntry(sched, false)
	id, err := n.timer.SetInterval(timerID, duration, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return fmt.Errorf("trigger: SetInterval failed: %w", err)
	}
	entry.timerID = id
	n.addEntry(entry)
	return nil
}

// registerCron 은 cron 타입 스케줄을 등록한다.
func (n *TriggerNode) registerCron(index int, sched TriggerSchedule) error {
	cronExpr, ok := sched.Value.(string)
	if !ok {
		return fmt.Errorf("%w: cron value must be a string", ErrTriggerInvalidScheduleValue)
	}

	timerID := fmt.Sprintf("%s-cron-%d", n.ID(), index)
	entry := n.newEntry(sched, false)
	id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return fmt.Errorf("trigger: SetCron failed: %w", err)
	}
	entry.timerID = id
	n.addEntry(entry)
	return nil
}

// registerOnce 는 once 타입 스케줄을 등록한다.
func (n *TriggerNode) registerOnce(index int, sched TriggerSchedule) error {
	valueStr, ok := sched.Value.(string)
	if !ok {
		return fmt.Errorf("%w: once value must be a string", ErrTriggerInvalidScheduleValue)
	}

	targetTime, err := time.Parse(time.RFC3339, valueStr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTriggerInvalidScheduleValue, err)
	}

	delay := time.Until(targetTime)
	if delay <= 0 {
		return fmt.Errorf("%w: once target time is in the past", ErrTriggerInvalidScheduleValue)
	}

	timerID := fmt.Sprintf("%s-once-%d", n.ID(), index)
	entry := n.newEntry(sched, false)
	id, err := n.timer.SetTimeout(timerID, delay, n.makeHandler(entry, string(sched.Type), timerID))
	if err != nil {
		return fmt.Errorf("trigger: SetTimeout failed: %w", err)
	}
	entry.timerID = id
	n.addEntry(entry)
	return nil
}

// registerTimes 는 times 타입 스케줄을 등록한다.
// 각 "HH:MM" 시각을 cron 표현식 "MM HH * * *"로 변환하여 등록한다.
func (n *TriggerNode) registerTimes(index int, sched TriggerSchedule) error {
	timesList, ok := sched.Value.([]any)
	if !ok {
		return fmt.Errorf("%w: times value must be an array", ErrTriggerInvalidScheduleValue)
	}

	for i, raw := range timesList {
		timeStr, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%w: times value must be string array", ErrTriggerInvalidScheduleValue)
		}

		minute, hour, err := parseHHMM(timeStr)
		if err != nil {
			return err
		}

		cronExpr := fmt.Sprintf("%d %d * * *", minute, hour)
		timerID := fmt.Sprintf("%s-times-%d-%d", n.ID(), index, i)
		entry := n.newEntry(sched, false)
		id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(entry, string(TriggerScheduleTimes), timerID))
		if err != nil {
			return fmt.Errorf("trigger: SetCron (times) failed: %w", err)
		}
		entry.timerID = id
		n.addEntry(entry)
	}

	return nil
}

// registerWeekly 는 weekly 타입 스케줄을 등록한다. (v1.2.0)
// Days 의 각 요일 × Times 의 각 시각 조합마다 cron "MM HH * * DOW"를 등록한다.
func (n *TriggerNode) registerWeekly(index int, sched TriggerSchedule) error {
	if len(sched.Days) == 0 || len(sched.Times) == 0 {
		return fmt.Errorf("%w: weekly requires non-empty days and times", ErrTriggerInvalidScheduleValue)
	}

	for di, day := range sched.Days {
		dow, err := parseWeekday(day)
		if err != nil {
			return err
		}
		for ti, timeStr := range sched.Times {
			minute, hour, err := parseHHMM(timeStr)
			if err != nil {
				return err
			}

			cronExpr := fmt.Sprintf("%d %d * * %s", minute, hour, dow)
			timerID := fmt.Sprintf("%s-weekly-%d-%d-%d", n.ID(), index, di, ti)
			entry := n.newEntry(sched, false)
			id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(entry, string(TriggerScheduleWeekly), timerID))
			if err != nil {
				return fmt.Errorf("trigger: SetCron (weekly) failed: %w", err)
			}
			entry.timerID = id
			n.addEntry(entry)
		}
	}

	return nil
}

// registerMonthly 는 monthly 타입 스케줄을 등록한다. (v1.2.0)
// day 가 정수(1~31)/"first" 이면 cron "MM HH N * *"를, "last" 이면 매일 cron
// "MM HH * * *"를 등록하고 핸들러에서 월말 게이트로 발화를 제한한다.
func (n *TriggerNode) registerMonthly(index int, sched TriggerSchedule) error {
	if len(sched.Times) == 0 {
		return fmt.Errorf("%w: monthly requires non-empty times", ErrTriggerInvalidScheduleValue)
	}

	dayNum, isLast, err := parseMonthlyDay(sched.Day)
	if err != nil {
		return err
	}

	for ti, timeStr := range sched.Times {
		minute, hour, err := parseHHMM(timeStr)
		if err != nil {
			return err
		}

		var cronExpr string
		if isLast {
			// 표준 cron 은 "마지막 날"을 표현하지 못하므로 매일 등록 후 핸들러 게이트.
			cronExpr = fmt.Sprintf("%d %d * * *", minute, hour)
		} else {
			cronExpr = fmt.Sprintf("%d %d %d * *", minute, hour, dayNum)
		}

		timerID := fmt.Sprintf("%s-monthly-%d-%d", n.ID(), index, ti)
		entry := n.newEntry(sched, isLast)
		id, err := n.timer.SetCron(timerID, cronExpr, n.makeHandler(entry, string(TriggerScheduleMonthly), timerID))
		if err != nil {
			return fmt.Errorf("trigger: SetCron (monthly) failed: %w", err)
		}
		entry.timerID = id
		n.addEntry(entry)
	}

	return nil
}

// weekdayTokens 는 소문자 3자 요일 약어를 cron Dow 필드 값으로 매핑한다.
var weekdayTokens = map[string]string{
	"sun": "0",
	"mon": "1",
	"tue": "2",
	"wed": "3",
	"thu": "4",
	"fri": "5",
	"sat": "6",
}

// parseWeekday 는 요일 토큰을 cron Dow 필드 값("0"~"6")으로 변환한다.
// 소문자 3자 약어(sun~sat, 대소문자 무시) 및 정수 0~6을 허용한다.
func parseWeekday(token string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(token))
	if dow, ok := weekdayTokens[t]; ok {
		return dow, nil
	}
	if num, err := strconv.Atoi(t); err == nil && num >= 0 && num <= 6 {
		return strconv.Itoa(num), nil
	}
	return "", fmt.Errorf("%w: invalid weekday token %q", ErrTriggerInvalidScheduleValue, token)
}

// parseMonthlyDay 는 monthly 의 day 값을 파싱한다.
// 정수 1~31, "first"(=1), "last"(월말 게이트)를 허용한다.
func parseMonthlyDay(day any) (dayNum int, isLast bool, err error) {
	switch v := day.(type) {
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		switch s {
		case "first":
			return 1, false, nil
		case "last":
			return 0, true, nil
		default:
			num, e := strconv.Atoi(s)
			if e != nil {
				return 0, false, fmt.Errorf("%w: invalid monthly day %q", ErrTriggerInvalidScheduleValue, v)
			}
			dayNum = num
		}
	case int:
		dayNum = v
	case int64:
		dayNum = int(v)
	case float64:
		dayNum = int(v)
	default:
		return 0, false, fmt.Errorf("%w: monthly day must be an int, \"first\", or \"last\"", ErrTriggerInvalidScheduleValue)
	}

	if dayNum < 1 || dayNum > 31 {
		return 0, false, fmt.Errorf("%w: monthly day out of range: %d", ErrTriggerInvalidScheduleValue, dayNum)
	}
	return dayNum, false, nil
}

// parseHHMM 은 "HH:MM" 문자열을 분·시로 파싱하고 유효 범위를 검증한다.
func parseHHMM(timeStr string) (minute, hour int, err error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("%w: invalid time format %q (expected HH:MM)", ErrTriggerInvalidScheduleValue, timeStr)
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("%w: invalid time %q", ErrTriggerInvalidScheduleValue, timeStr)
	}
	return m, h, nil
}

// lastDayOfMonth 는 주어진 시각이 속한 달의 마지막 날(일)을 반환한다.
func lastDayOfMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

// withinValidity 는 now 가 규칙 유효기간 [from, to] 내인지 판정한다.
// (SPEC-TRIGGER-SCHED-001 RD-8)
//
// 의미론:
//   - 기준 시각대(timezone): now 의 위치(서버 로컬). from/to 도 같은 위치로 해석한다.
//   - 비교 단위: 날짜(day) 단위. from 은 해당 일 00:00:00 부터, to 는 해당 일
//     23:59:59.999... 까지 포함 — 양끝 inclusive.
//   - 무제한: from 이 빈 값/파싱 불가면 하한 무제한(과거 무제한), to 가 빈 값/파싱
//     불가면 상한 무제한(무기한). 잘못된 날짜 문자열은 오류가 아니라 해당 방향의
//     무제한으로 관용 처리한다(REQ-SCHED-01-04, 파싱 견고성).
func withinValidity(now time.Time, from, to string) bool {
	if fromDay, ok := parseValidityDate(from, now.Location()); ok {
		// 하한: now 가 from 당일 00:00 이전이면 범위 밖.
		if now.Before(fromDay) {
			return false
		}
	}
	if toDay, ok := parseValidityDate(to, now.Location()); ok {
		// 상한: to 당일 전체 포함 → 다음 날 00:00 이상이면 범위 밖(양끝 inclusive).
		if !now.Before(toDay.AddDate(0, 0, 1)) {
			return false
		}
	}
	return true
}

// parseValidityDate 는 유효기간 경계 문자열을 loc(서버 로컬) 기준 해당 날짜의
// 00:00:00 시각으로 파싱한다. 수용 형식은 `YYYY-MM-DD`(주 형식)와 RFC3339
// datetime(해당 날짜로 정규화)이다. 빈 값/파싱 불가는 (zero, false) 를 반환하여
// 호출부에서 해당 방향 무제한으로 처리한다.
func parseValidityDate(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	// 주 형식: YYYY-MM-DD (loc 기준 자정).
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, true
	}
	// 보조 형식: RFC3339 datetime → loc 기준 날짜로 정규화(날짜 단위 비교).
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		lt := t.In(loc)
		return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc), true
	}
	return time.Time{}, false
}

// now 는 현재 시각을 반환한다. nowFunc 가 주입되어 있으면 이를 사용한다.
func (n *TriggerNode) now() time.Time {
	if n.nowFunc != nil {
		return n.nowFunc()
	}
	return time.Now()
}

// setNowFunc 는 현재 시각 조회 훅을 주입한다.
// monthly "last" 월말 게이트의 경계 동작을 검증하기 위한 테스트 훅이다. (v1.2.0)
func (n *TriggerNode) setNowFunc(f func() time.Time) {
	n.nowFunc = f
}

// makeHandler 는 타이머 핸들러 함수를 생성한다.
func (n *TriggerNode) makeHandler(entry *triggerTimerEntry, scheduleType string, timerID string) system.TimerHandler {
	return func(trigger system.TimerTrigger) {
		// re-arm 세대 게이트 (SPEC-TRIGGER-PANEL-001 RD-6): 이 엔트리가 등록된
		// 세대가 현재 rearmGen 과 다르면 live 재무장으로 무효화된 stale in-flight
		// 발화이므로 폐기한다. cancel→re-register 창에서 이미 디스패치된 구 타이머
		// 핸들러의 중복 발화를 방지한다 (double-fire·orphan 없음).
		if entry.gen != n.rearmGen.Load() {
			return
		}

		// 일시정지 상태 확인
		n.pauseMu.RLock()
		isPaused := n.paused
		n.pauseMu.RUnlock()
		if isPaused {
			return
		}

		// 활성/유효기간 발화 게이트 (SPEC-TRIGGER-SCHED-001 RD-8):
		// 발화 조건은 enabled AND now∈[valid_from, valid_to] 의 논리곱이다.
		// 비활성이거나 유효기간 밖이면 이번 틱에는 emit 하지 않는다. 타이머는
		// 취소하지 않고 등록된 채 유지되어(반복 타이머) 다음 틱에 재검사된다 —
		// 재활성/기간 복귀 시 별도 재무장 없이 자연히 발화가 재개된다.
		// entry.enabled/validFrom/validTo 는 등록 시점 캡처된 엔트리 로컬 불변이라
		// lock 없이 읽는다. now() 는 서버 로컬 시각(테스트는 nowFunc 로 고정 주입).
		// (REQ-SCHED-01-03/04/05)
		if !entry.enabled {
			return
		}
		if !withinValidity(n.now(), entry.validFrom, entry.validTo) {
			return
		}

		// monthly "last" 월말 emit 게이트 (v1.2.0):
		// 매일 발화하는 타이머이므로, 현재 날짜가 해당 월의 마지막 날일 때만 emit한다.
		if entry.lastDayGate {
			nowT := n.now()
			if nowT.Day() != lastDayOfMonth(nowT) {
				return
			}
		}

		// 트리거에서 전달된 틱 카운트 사용
		tickCount := trigger.TickCount

		// 메시지 생성
		msg := n.buildMessage(entry, trigger, scheduleType, timerID, tickCount)

		// 스케줄 발화 관측(SPEC-SCHEDULE-VIEW-001 M2, RD-8): 관측자가 설정돼 있으면 발화
		// 이벤트를 best-effort 로 통지한다. 제네릭 trigger 노드는 storage 를 몰라도 되도록
		// 관측자 인터페이스에만 의존한다(구체 어댑터는 internal/schedulelog). 발화마다 호출되어
		// fire-only(다운스트림 제어 없음)도 포착한다(AC-15). correlation_id=schedule_id:trigger_ms
		// 는 result 이벤트와 동일 공식으로 조인된다(AC-16). trigger_ms 는 발화 시각 epoch ms 이며,
		// result 측(buildXsfmControlCommand→에이전트)은 `trigger.trigger_time`(RFC3339) 메타를
		// 동일 epoch ms 로 파싱해 같은 키를 재구성한다. 관측자 nil/에러는 발화에 영향을 주지 않는다.
		if obs := getScheduleFireObserver(); obs != nil {
			triggerMs := trigger.TriggerAt.UnixMilli()
			obs.OnScheduleFire(ScheduleFireContext{
				CorrelationID:   timerID + ":" + strconv.FormatInt(triggerMs, 10),
				ScheduleID:      timerID,
				RuleName:        entry.name,
				DeclaredAgentID: entry.agentID,
				TriggerTime:     triggerMs,
			})
		}

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
func (n *TriggerNode) buildMessage(entry *triggerTimerEntry, trigger system.TimerTrigger, scheduleType string, timerID string, tickCount int64) message.Message {
	triggerTime := trigger.TriggerAt.UTC()
	triggerTimeStr := triggerTime.Format(time.RFC3339Nano)

	// 페이로드 소스 선택 (해결 순서, per-schedule payload v1.2.0):
	//  (1) 스케줄 항목 payload_template → payload
	//  (2) 노드 레벨 payload_template → payload
	//  (3) 기본 페이로드 {"trigger_time": now}
	// 선택된 소스는 단일 통합 템플릿 엔진(evalPayload)으로 평가된다. (v1.3.0)
	var src any
	switch {
	case entry != nil && entry.payloadTmpl != nil:
		src = entry.payloadTmpl
	case entry != nil && entry.payload != nil:
		src = entry.payload
	default:
		// 노드 레벨 폴백은 live Configure 와 경합하므로 payloadMu 로 보호한다.
		n.payloadMu.RLock()
		switch {
		case n.payloadTemplate != nil:
			src = n.payloadTemplate
		case n.payload != nil:
			src = n.payload
		}
		n.payloadMu.RUnlock()
	}

	var payload message.Payload
	var templateErr string
	if src == nil {
		// 기본 페이로드 (스케줄·노드 레벨 페이로드 모두 없음)
		payload = message.NewPayload(map[string]any{
			"trigger_time": triggerTimeStr,
		})
	} else {
		// 통합 엔진 알려진 변수 4종 (키는 "$." 접두사 제외)
		vars := map[string]any{
			"trigger_time": triggerTimeStr,
			"tick_count":   tickCount,
			"schedule_id":  trigger.ScheduleID,
			"trigger_id":   n.Name(),
		}
		payload, templateErr = n.evalPayload(src, vars)
	}

	// 메시지 생성 옵션
	// SPEC-MESSAGE-TYPE-001 § T1: metadata.message_type → 1급 WithType.
	opts := []message.Option{
		message.WithPayload(payload),
		message.WithType("event"),
		message.WithMetadata("trigger.schedule_type", scheduleType),
		message.WithMetadata("trigger.schedule_id", timerID),
		message.WithMetadata("trigger.tick_count", strconv.FormatInt(tickCount, 10)),
		message.WithMetadata("trigger.trigger_time", triggerTimeStr),
		message.WithMetadata("trigger.node_name", n.Name()),
	}

	// 규칙 메타 pass-through (SPEC-TRIGGER-SCHED-001 REQ-SCHED-01-06):
	// name/priority 는 발화 여부에 영향을 주지 않는 메타로, 방출 메시지에 통과 전달한다.
	// 기존 스케줄(name="" priority=0) 무회귀를 위해 의미 있는 값일 때만 메타를 추가한다.
	if entry != nil {
		if entry.name != "" {
			opts = append(opts, message.WithMetadata("trigger.rule_name", entry.name))
		}
		if entry.priority != 0 {
			opts = append(opts, message.WithMetadata("trigger.priority", strconv.Itoa(entry.priority)))
		}
		// agent_id(SPEC-SCHEDULE-VIEW-001 M2): 선언된 대상 에이전트. 하위 호환(agent_id 미선언
		// 스케줄 무회귀, AC-13)을 위해 값이 있을 때만 메타를 추가한다. xsfm 제어 노드가 이 메타를
		// 읽어 _correlation.declared_agent_id 로 실어 result 이벤트에 조인한다.
		if entry.agentID != "" {
			opts = append(opts, message.WithMetadata("trigger.agent_id", entry.agentID))
		}
	}

	// 템플릿 에러 시 메타데이터 추가
	if templateErr != "" {
		opts = append(opts, message.WithMetadata("trigger.error", templateErr))
	}

	return message.New(opts...)
}

// evalPayload 는 페이로드 소스를 단일 통합 템플릿 엔진으로 평가한다. (v1.3.0)
//
// config 하위호환 라우팅:
//   - src 가 map[string]any 이면 그대로 평가한다.
//   - src 가 비-map 스칼라/배열(구 static 스칼라)이면 {"value": <src>} 로 래핑 후 평가한다.
//
// 각 최상위 문자열 값은 evalString 규칙(통째 변수 → 네이티브 타입, 그 외 interpolation)으로
// 평가되고, 비문자열 값은 리터럴 패스스루(깊은 복사)된다. 중첩 맵은 재귀 평가하지 않는다
// (알려진 한계). 미지 변수를 만나면 해당 키 값을 nil 로 만들고 에러 메시지를 반환한다
// (여러 키가 실패하면 마지막 에러 우선).
func (n *TriggerNode) evalPayload(src any, vars map[string]any) (message.Payload, string) {
	srcMap, ok := src.(map[string]any)
	if !ok {
		// 비-map 스칼라/배열 → {"value": <src>} 래핑 (config 하위호환 라우팅)
		srcMap = map[string]any{"value": src}
	}

	result := make(map[string]any, len(srcMap))
	var errMsg string
	for key, val := range srcMap {
		strVal, isStr := val.(string)
		if !isStr {
			// 비문자열 값: 리터럴 패스스루 (메시지 간 격리를 위해 깊은 복사)
			result[key] = deepCopyAny(val)
			continue
		}
		evaluated, e := evalString(strVal, vars)
		result[key] = evaluated
		if e != "" {
			errMsg = e
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

// evalString 은 단일 문자열 값을 통합 템플릿 엔진 규칙으로 평가한다. (v1.3.0)
//
//  1. 통째 변수 치환(whole-value): 문자열 전체가 알려진 변수 토큰 "$.<name>" 과 정확히
//     일치하면 변수의 네이티브 타입 값을 반환한다(예: "$.tick_count" → int64).
//  2. 그 외에는 문자 단위 interpolation:
//     - "$$"                 → 리터럴 "$"
//     - "$.<name>" (known)   → 변수의 문자열 형태(fmt.Sprint)
//     - "$.<name>" (unknown) → 에러 기록 + 값 nil (오타 탐지)
//     - 그 외 문자(단독 "$", 식별자 없는 "$." 포함) → 리터럴
//
// 알려진 변수는 vars 맵의 키("trigger_time" 등, "$." 접두사 제외)로 판별한다.
func evalString(s string, vars map[string]any) (any, string) {
	// (1) 통째 변수 치환 (whole-value fast path)
	if name, ok := wholeVarToken(s); ok {
		if v, known := vars[name]; known {
			return v, ""
		}
		// 통째 토큰이지만 미지의 변수이면 아래 interpolation 에서 에러 처리된다.
	}

	// (2) 문자 단위 interpolation
	var b strings.Builder
	var errMsg string
	i := 0
	for i < len(s) {
		c := s[i]
		// "$$" → 리터럴 "$"
		if c == '$' && i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		// "$.<name>"
		if c == '$' && i+1 < len(s) && s[i+1] == '.' {
			j := i + 2
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			name := s[i+2 : j]
			if name == "" {
				// 단독 "$." (뒤에 식별자 없음) → 리터럴 "$" (다음 반복에서 "." 리터럴 처리)
				b.WriteByte('$')
				i++
				continue
			}
			if v, known := vars[name]; known {
				b.WriteString(fmt.Sprint(v))
			} else {
				errMsg = fmt.Sprintf("unknown template variable: $.%s", name)
			}
			i = j
			continue
		}
		// 그 외 모든 문자(단독 "$" 포함) → 리터럴
		b.WriteByte(c)
		i++
	}

	if errMsg != "" {
		// 미지 변수가 하나라도 있으면 해당 키 값은 nil 이 된다.
		return nil, errMsg
	}
	return b.String(), ""
}

// wholeVarToken 은 문자열 전체가 "$.<name>" 형태인지 검사하고 <name> 을 반환한다.
// <name> 은 [A-Za-z0-9_]+ 이어야 한다.
func wholeVarToken(s string) (string, bool) {
	if !strings.HasPrefix(s, "$.") {
		return "", false
	}
	name := s[2:]
	if name == "" {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		if !isIdentChar(name[i]) {
			return "", false
		}
	}
	return name, true
}

// isIdentChar 는 변수명에 허용되는 문자([A-Za-z0-9_])인지 판별한다.
func isIdentChar(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// Shutdown 은 TriggerNode를 종료한다.
func (n *TriggerNode) Shutdown(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateStopping); err != nil {
		return err
	}

	// 종료 후 Configure 는 더 이상 live 재무장 대상이 아니다 (RD-2).
	n.started.Store(false)

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

	// 페이로드 설정 (live Configure 는 발화 핸들러와 경합하므로 payloadMu 보호)
	n.payloadMu.Lock()
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
	n.payloadMu.Unlock()

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

	// --- live 재무장 (SPEC-TRIGGER-PANEL-001 RD-2 / RD-6) ---
	// 이미 Init 을 마친(started) 실행 중(Running) 노드에서 Configure 가 호출되면
	// (런타임 ReconfigureNode 경로) 기존 타이머를 모두 취소하고 새 스케줄로 재등록하여
	// 스케줄 변경을 즉시 반영한다(flow 재배포 불필요). 최초 Configure(Init 이전,
	// Created 상태)에서는 재등록하지 않으며 Init 이 등록을 담당하므로 이중 등록이
	// 발생하지 않는다. Paused/Stopped 상태도 재등록하지 않는다(다음 Resume/Init 경로가
	// 반영). n.timer 는 Init 에서 resolve 되므로 running 노드에서는 항상 non-nil 이다.
	// (REQ-01-01/03/04)
	if n.started.Load() && n.CurrentState() == lifecycle.StateRunning && n.timer != nil {
		return n.rearmTimers()
	}

	return nil
}

// rearmTimers 는 live 재무장을 수행한다: 기존 타이머 전체 취소 → re-arm 세대 증가
// → 새 스케줄 재등록. 실패 시 부분 등록 타이머를 롤백하고 오류를 반환하되 lifecycle
// 상태는 유지한다(Error 전이 없음, REQ-01-06). 스케줄이 비어 있으면 전 타이머만
// 취소되고 발화 없는 유효 IDLE 상태로 Running 을 유지한다(REQ-01-05, RD-7).
//
// @MX:WARN: cancel→re-register 창에서 취소된 타이머의 in-flight 핸들러가 아직
//
//	timer agent 고루틴에서 실행 중일 수 있다. rearmGen 을 증가시켜 각 stale
//	핸들러(구 세대를 캡처한 엔트리)가 발화 시 세대 불일치로 스스로 폐기되게 한다
//	(makeHandler 의 세대 게이트) — double-fire·orphan 없음.
//
// @MX:REASON: 타이머 발화는 timer agent 고루틴에서 비동기 실행되므로 취소와 발화가
//
//	경합한다. atomic generation 토큰이 lock-free 로 stale 발화를 무해화한다.
func (n *TriggerNode) rearmTimers() error {
	n.cancelAllTimers() // 기존 타이머 전체 취소 (timerMu 보호, timerEntries=nil)
	n.rearmGen.Add(1)   // 재무장 세대 증가 — 이후 등록 엔트리가 새 세대를 캡처
	if err := n.registerSchedules(); err != nil {
		n.cancelAllTimers() // 롤백: 부분 등록 타이머 취소 (REQ-01-06)
		return err
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

// deepCopyAny 는 임의의 값을 재귀적으로 깊은 복사한다.
// map/슬라이스는 재귀 복사하고, 그 외 기본 타입은 값 복사한다.
func deepCopyAny(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		cp := make([]any, len(val))
		for i, e := range val {
			cp[i] = deepCopyAny(e)
		}
		return cp
	default:
		return v
	}
}

// deepCopyMap 은 map[string]any를 재귀적으로 깊은 복사한다.
func deepCopyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = deepCopyAny(v)
	}
	return dst
}
