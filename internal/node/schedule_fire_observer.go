// schedule_fire_observer.go 는 스케줄(예약) 발화 관측자 인터페이스와 패키지 싱글턴을
// 정의한다(SPEC-SCHEDULE-VIEW-001 M2, RD-8).
//
// 제네릭 trigger 노드는 스케줄 로그 저장소(internal/storage)를 직접 알지 않고 이 관측자
// 인터페이스에만 의존한다 — storage 를 임포트하는 구체 어댑터는 별도 패키지
// (internal/schedulelog)에 두어 node→storage 결합을 피한다(import cycle 회피). main.go 가
// SetScheduleFireObserver 로 단 한 번 어댑터를 주입하며(xsfm.SetAuditRepository 패턴 미러),
// 미설정(nil) 상태에서도 발화 경로가 죽지 않도록 접근자가 nil 을 반환하면 통지를 no-op 으로
// 건너뛴다.
package node

import "sync"

// ScheduleFireContext 는 스케줄 1회 발화의 관측 컨텍스트이다.
//
// CorrelationID 는 fire↔result 조인 키(schedule_id + ":" + trigger_time_ms)이며, result
// 이벤트가 동일 공식으로 재구성해 조인한다. TriggerTime 은 발화 시각 epoch ms(프로젝트 규약).
// DeclaredAgentID 는 스케줄이 선언한 대상 에이전트(빈 값=미선언)이다.
type ScheduleFireContext struct {
	CorrelationID   string
	ScheduleID      string
	RuleName        string
	DeclaredAgentID string
	TriggerTime     int64
}

// ScheduleFireObserver 는 스케줄 발화 이벤트를 통지받는 관측자이다(fire 이벤트 기록 표면).
//
// 구현은 best-effort 여야 하며(기록 실패는 로깅만), OnScheduleFire 는 발화 핸들러를 블록/
// 실패시키지 않아야 한다.
type ScheduleFireObserver interface {
	OnScheduleFire(ctx ScheduleFireContext)
}

var (
	scheduleFireObsMu sync.RWMutex
	scheduleFireObs   ScheduleFireObserver
)

// SetScheduleFireObserver 는 패키지-레벨 발화 관측자를 설정한다(nil=미설정=no-op).
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다.
func SetScheduleFireObserver(obs ScheduleFireObserver) {
	scheduleFireObsMu.Lock()
	defer scheduleFireObsMu.Unlock()
	scheduleFireObs = obs
}

// getScheduleFireObserver 는 현재 설정된 발화 관측자를 반환한다(없으면 nil).
// 호출자는 nil 체크 후 통지를 no-op 으로 건너뛴다(graceful degradation).
func getScheduleFireObserver() ScheduleFireObserver {
	scheduleFireObsMu.RLock()
	defer scheduleFireObsMu.RUnlock()
	return scheduleFireObs
}
