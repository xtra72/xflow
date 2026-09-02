// Package schedulelog 는 스케줄(예약) 발화 관측자의 구체 어댑터를 제공한다
// (SPEC-SCHEDULE-VIEW-001 M2, RD-8).
//
// 제네릭 trigger 노드(internal/node)는 node.ScheduleFireObserver 인터페이스에만 의존하고
// 스케줄 로그 저장소(internal/storage)를 직접 알지 않는다. 그런데 fire 이벤트를 기록하는
// 구체 구현은 storage 를 임포트해야 한다. internal/node 는 이미 internal/agent/xsfm 에
// 임포트되므로(node→xsfm), 어댑터를 xsfm 에 두면 xsfm→node 역방향 임포트가 생겨 import
// cycle(node→xsfm→node)이 발생한다. 따라서 어댑터를 별도 패키지(internal/schedulelog)에 두어
// node 와 storage 를 모두 임포트하되, node·storage 어느 쪽도 schedulelog 를 임포트하지 않게 해
// cycle 을 회피한다. main.go 만 이 패키지를 임포트해 node.SetScheduleFireObserver 로 주입한다.
package schedulelog

import (
	"context"
	"log/slog"
	"time"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
)

// warnLogger 는 best-effort 경고 로깅에 필요한 최소 로거 표면이다. *slog.Logger 와
// observe.ComponentLogger 가 모두 이 시그니처를 만족하므로 observe 패키지에 결합하지 않는다.
type warnLogger interface {
	Warn(msg string, args ...any)
}

// FireObserver 는 node.ScheduleFireObserver 를 구현하는 fire 이벤트 기록 어댑터이다.
//
// 발화마다 storage.ScheduleLogRecord{RecordKind: fire} 하나를 append 한다(fire-only 포착,
// AC-15). best-effort: repo 미설정(nil)이면 no-op, append 실패는 로깅만 하고 발화에 영향을
// 주지 않는다(AC-14).
type FireObserver struct {
	repo   storage.ScheduleLogRepository
	logger warnLogger
}

// 컴파일 타임 인터페이스 체크.
var _ node.ScheduleFireObserver = (*FireObserver)(nil)

// NewFireObserver 는 스케줄 로그 저장소와 로거로 발화 관측 어댑터를 만든다.
// logger 가 nil 이면 slog.Default() 를 사용한다.
func NewFireObserver(repo storage.ScheduleLogRepository, logger warnLogger) *FireObserver {
	if logger == nil {
		logger = slog.Default()
	}
	return &FireObserver{repo: repo, logger: logger}
}

// OnScheduleFire 는 스케줄 발화 이벤트를 fire 레코드로 기록한다(best-effort).
//
// fire 레코드는 대상 실행 결과(ActorAgentID/Result/Targets)를 담지 않는다 — 그 정보는 xsfm
// 제어 경로만 알기에 별도 result 이벤트로 append 되며 동일 CorrelationID 로 조인된다.
func (o *FireObserver) OnScheduleFire(ctx node.ScheduleFireContext) {
	if o == nil || o.repo == nil {
		return // 저장소 미설정 → graceful no-op
	}
	rec := storage.ScheduleLogRecord{
		CorrelationID:   ctx.CorrelationID,
		RecordKind:      storage.ScheduleLogRecordKindFire,
		ScheduleID:      ctx.ScheduleID,
		RuleName:        ctx.RuleName,
		DeclaredAgentID: ctx.DeclaredAgentID,
		TriggerTime:     ctx.TriggerTime,
		Timestamp:       time.Now().UnixMilli(),
	}
	if err := o.repo.Append(context.Background(), rec); err != nil {
		o.logger.Warn("schedulelog: fire append failed",
			"schedule_id", ctx.ScheduleID, "correlation_id", ctx.CorrelationID, "error", err)
	}
}
