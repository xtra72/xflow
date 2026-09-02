package xsfm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// ---------------------------------------------------------------------------
// 스케줄 로그 result 이벤트 배선 (SPEC-SCHEDULE-VIEW-001 M2)
// ---------------------------------------------------------------------------
//
// 예약(스케줄) 제어의 실행 결과는 xsfm 제어 경로만 알 수 있다 — 발화 상관(schedule_id·
// trigger_time·선언 대상 에이전트)과 실제 실행 에이전트, 집계 결과가 모두 이 지점에 모인다.
// 그래서 result 이벤트는 여기서 append 하며, fire 이벤트(trigger 노드가 기록)와 동일한
// CorrelationID(schedule_id + ":" + trigger_time_ms)로 조인된다(AC-16).
//
// 저장소는 audit.go 의 device_id_repo 싱글턴 + setter 패턴을 그대로 미러한다: main.go 가 startup
// 시 SetScheduleLogRepository 로 단 한 번 주입하며, 미설정(nil)에서도 제어가 죽지 않도록 접근자가
// nil 을 반환하면 기록을 no-op 으로 건너뛴다(AC-5/AC-14). 기록 실패는 best-effort 로 로깅만 한다.
//
// 시크릿 금지: 제어 인자/페이로드는 기록하지 않고 셀렉터/명령/결과 분류만 남긴다.

var (
	scheduleLogRepoMu sync.RWMutex
	scheduleLogRepo   storage.ScheduleLogRepository
)

// SetScheduleLogRepository 는 패키지-레벨 스케줄 로그 저장소를 설정한다(audit 저장소 패턴).
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다.
func SetScheduleLogRepository(r storage.ScheduleLogRepository) {
	scheduleLogRepoMu.Lock()
	defer scheduleLogRepoMu.Unlock()
	scheduleLogRepo = r
}

// getScheduleLogRepository 는 현재 설정된 스케줄 로그 저장소를 반환한다(없으면 nil).
// 호출자는 nil 체크 후 기록을 no-op 으로 건너뛴다(graceful degradation).
func getScheduleLogRepository() storage.ScheduleLogRepository {
	scheduleLogRepoMu.RLock()
	defer scheduleLogRepoMu.RUnlock()
	return scheduleLogRepo
}

// scheduleCorrelation 은 processRequest 의 _correlation 블록이다(예약 발화 상관).
// trigger_time 은 메타의 문자열 형태(RFC3339)로 전달되며 기록 시 epoch ms 로 파싱한다.
type scheduleCorrelation struct {
	ScheduleID      string `json:"schedule_id"`
	RuleName        string `json:"rule_name"`
	DeclaredAgentID string `json:"declared_agent_id"`
	TriggerTime     string `json:"trigger_time"`
}

// scheduleTargetEntry 는 스케줄 로그 result 레코드의 Targets(대상별 결과) 임베드 원소이다.
// fan-out 은 멤버마다 1원소, 개별 제어는 1원소이다(RD-7: fan-out 도 result 는 1레코드).
type scheduleTargetEntry struct {
	Target string `json:"target"`
	Result string `json:"result"` // ok | error
	Reason string `json:"reason,omitempty"`
}

// recordScheduleLog 는 예약 제어 1회 실행에 대해 result 레코드 1건을 append 한다(RD-7).
//
// 호출 규약: 제어 명령 처리의 최상위(dispatchControl)에서 실행 완료 후 집계 결과를 알 때, 그리고
// _correlation 블록이 있을 때만(req.Correlation != nil) 호출한다 — 수동 제어(상관 없음)는 어떤
// 스케줄 로그도 남기지 않는다(AC-6). fan-out 이라도 대상별 ok/error 는 Targets 에 임베드하고
// 레코드는 1건만 남긴다. best-effort: 저장소 미설정 no-op, append 실패는 로깅만(제어 무영향, AC-14).
func (a *XSFMAgent) recordScheduleLog(req processRequest, resp []byte, controlErr error) {
	repo := getScheduleLogRepository()
	if repo == nil {
		return // 저장소 미설정 → graceful no-op
	}
	corr := req.Correlation
	if corr == nil {
		return // 상관 없음 → 미기록(AC-6). 호출부에서 이미 걸러지나 이중 안전.
	}

	triggerMs := parseScheduleTriggerMs(corr.TriggerTime)
	// fire 이벤트와 동일 공식으로 correlation_id 를 재구성한다(schedule_id + ":" + epoch ms).
	correlationID := corr.ScheduleID + ":" + strconv.FormatInt(triggerMs, 10)

	target, action, result, targets, reason := a.buildScheduleResult(req, resp, controlErr)

	rec := storage.ScheduleLogRecord{
		CorrelationID:   correlationID,
		RecordKind:      storage.ScheduleLogRecordKindResult,
		ScheduleID:      corr.ScheduleID,
		RuleName:        corr.RuleName,
		DeclaredAgentID: corr.DeclaredAgentID, // 스케줄이 선언한 대상(RD-6)
		ActorAgentID:    a.auditActor(),       // 실제 실행 에이전트(RD-6 authority)
		TriggerTime:     triggerMs,
		Target:          target,
		Action:          action,
		Result:          result,
		Targets:         targets,
		Reason:          reason,
		Timestamp:       time.Now().UnixMilli(),
	}
	if err := repo.Append(context.Background(), rec); err != nil {
		a.logger.Warn("xsfm: schedule log append failed",
			"schedule_id", corr.ScheduleID, "correlation_id", correlationID, "error", err)
	}
}

// buildScheduleResult 는 제어 응답/에러로부터 스케줄 로그 result 레코드의 표시 필드를 구성한다.
//
//   - 제어 자체 실패(대상 없음/검증 실패/빈 그룹 등): 셀렉터 단위 error 1건, Targets 는 셀렉터 값 1원소.
//   - 개별 성공: Target=device_id=<id>, Targets 는 해당 device 1원소(ok).
//   - fan-out 성공/부분성공: Target=<셀렉터종류>=<값>, Targets 는 멤버별 ok/error, 집계는 전 멤버 ok 여야 ok.
func (a *XSFMAgent) buildScheduleResult(req processRequest, resp []byte, controlErr error) (target, action, result, targets, reason string) {
	action = req.Command

	if controlErr != nil {
		kind, value := scheduleSelectorRef(req)
		reason = controlErr.Error() // 비밀-아님 분류(device_id/셀렉터 값 — recordGroupAudit 동일 관례)
		entries := []scheduleTargetEntry{{Target: value, Result: storage.ScheduleLogResultError, Reason: reason}}
		return kind + "=" + value, action, storage.ScheduleLogResultError, marshalScheduleTargets(entries), reason
	}

	// 성공 응답 파싱: fan-out(selector+results) vs 개별(device_id).
	var probe struct {
		Selector *selectorRef  `json:"selector"`
		Results  []groupResult `json:"results"`
		DeviceID string        `json:"device_id"`
	}
	_ = json.Unmarshal(resp, &probe)

	if probe.Results != nil { // fan-out 집계 응답
		sel := selectorRef{}
		if probe.Selector != nil {
			sel = *probe.Selector
		}
		entries := make([]scheduleTargetEntry, 0, len(probe.Results))
		ok := 0
		for _, r := range probe.Results {
			if r.Status == "ok" {
				ok++
				entries = append(entries, scheduleTargetEntry{Target: r.DeviceID, Result: storage.ScheduleLogResultOK})
			} else {
				entries = append(entries, scheduleTargetEntry{Target: r.DeviceID, Result: storage.ScheduleLogResultError, Reason: r.Status})
			}
		}
		agg := storage.ScheduleLogResultOK
		if ok != len(probe.Results) { // 전 멤버 ok 여야 ok, 그 외 error(플랜)
			agg = storage.ScheduleLogResultError
		}
		reason = fmt.Sprintf("%s=%s ok=%d/%d", sel.Type, sel.Value, ok, len(probe.Results))
		return sel.Type + "=" + sel.Value, action, agg, marshalScheduleTargets(entries), reason
	}

	// 개별 제어 성공: 응답의 device_id(이름 셀렉터 해소 결과 포함), 없으면 요청 device_id 폴백.
	deviceID := probe.DeviceID
	if deviceID == "" {
		deviceID = req.DeviceID
	}
	entries := []scheduleTargetEntry{{Target: deviceID, Result: storage.ScheduleLogResultOK}}
	return "device_id=" + deviceID, action, storage.ScheduleLogResultOK, marshalScheduleTargets(entries), ""
}

// scheduleSelectorRef 는 요청에서 대상 셀렉터의 종류·값을 도출한다(제어 실패 시 Target 표기용).
// 우선순위는 dispatchControl 체인과 동일: device_id > device_name > station > line > group_id > group_name.
func scheduleSelectorRef(req processRequest) (kind, value string) {
	switch {
	case req.DeviceID != "":
		return "device_id", req.DeviceID
	case req.DeviceName != "":
		return "device_name", req.DeviceName
	case req.Station != "":
		return "station", req.Station
	case req.Line != "":
		return "line", req.Line
	case req.GroupID != "":
		return "group_id", req.GroupID
	case req.GroupName != "":
		return "group_name", req.GroupName
	default:
		return "", ""
	}
}

// parseScheduleTriggerMs 는 상관 블록의 trigger_time 문자열을 epoch ms 로 파싱한다.
// RFC3339(Nano) 문자열을 우선 파싱하고, 실패 시 epoch ms 정수 문자열도 관용적으로 수용한다.
// 어느 쪽도 아니면 0 을 반환한다(best-effort — 기록을 막지 않는다).
func parseScheduleTriggerMs(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return 0
}

// marshalScheduleTargets 는 대상별 결과 임베드를 JSON 배열 문자열로 직렬화한다.
// 직렬화 실패 시 빈 배열("[]")을 반환한다(기록 자체는 계속된다).
func marshalScheduleTargets(entries []scheduleTargetEntry) string {
	b, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(b)
}
