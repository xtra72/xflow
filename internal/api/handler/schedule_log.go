// schedule_log.go 는 설비 제어 예약(스케줄) 실행 로그 조회 REST API 를 제공한다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 M3, RD-5, AC-5/AC-17).
//
// remote_admin.go 의 Audit 핸들러(GET /remote/audit)를 미러링하되 admin 게이팅을
// 제거한다 — 로그 조회는 인증된 전체 사용자에게 열려 있다(RD-5, AC-17). 라우트는
// remote_admin 과 동일한 인증 그룹(/api/v1)에 등록되므로 운영 시 Auth 미들웨어가
// 인증을 강제하지만, 핸들러는 requireAdmin 을 호출하지 않는다(관리자 전용 아님).
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// ScheduleLogResponse 는 스케줄 실행 로그 응답 표현이다(spec §4.1). 시크릿은 담지
// 않는다(Reason 은 비밀-아님 분류 텍스트만). TriggerTime/Timestamp 는 epoch ms 이다.
//
// Targets 는 저장소에 JSON 문자열([{target,result,reason}])로 보관되는 대상별
// ok/error 구조이다. json.RawMessage 로 원본을 통과시켜(structured data 전달)
// 프론트엔드가 재파싱 없이 배열로 받도록 한다.
type ScheduleLogResponse struct {
	ID              int64           `json:"id"`
	CorrelationID   string          `json:"correlation_id"`
	RecordKind      string          `json:"record_kind"`
	ScheduleID      string          `json:"schedule_id"`
	RuleName        string          `json:"rule_name"`
	DeclaredAgentID string          `json:"declared_agent_id"`
	ActorAgentID    string          `json:"actor_agent_id"`
	TriggerTime     int64           `json:"trigger_time"` // epoch ms
	Target          string          `json:"target"`
	Action          string          `json:"action"`
	Result          string          `json:"result"`
	Targets         json.RawMessage `json:"targets"` // [{target,result,reason}] 원본 JSON 통과
	Reason          string          `json:"reason"`
	Timestamp       int64           `json:"timestamp"` // epoch ms
}

// ScheduleLogHandler 는 스케줄 로그 조회 엔드포인트를 처리한다. admin 게이팅 없이
// 인증된 전체 사용자에게 서비스한다(RD-5). 읽기 전용이며 갱신/삭제 API 는 없다.
type ScheduleLogHandler struct {
	logs storage.ScheduleLogRepository
}

// NewScheduleLogHandler 는 ScheduleLogHandler 를 생성한다. repo 는 main.go 에서 fire
// (node 측)/result(xsfm 측) 기록에 주입한 것과 동일한 저장소 인스턴스를 읽기 측으로
// 공유 주입받는다. nil 이면 빈 목록을 반환한다(AC-5 — 미구성 시 에러 아님).
func NewScheduleLogHandler(repo storage.ScheduleLogRepository) *ScheduleLogHandler {
	return &ScheduleLogHandler{logs: repo}
}

// RegisterRoutes 는 스케줄 로그 조회 라우트를 그룹에 등록한다. remote_admin 과 동일한
// 인증 그룹에 등록하되 requireAdmin 을 호출하지 않으므로 인증된 전체 사용자가 접근한다(AC-17).
func (h *ScheduleLogHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/schedules/logs", h.Logs)
}

// Logs 는 스케줄 실행 로그를 최신순으로 반환한다. GET /schedules/logs
//
// admin 게이팅 없음 — 인증된 전체 사용자 접근(RD-5, AC-17). 선택적 필터
// ?schedule_id=/?rule_name=/?agent_id=(선언 또는 실행 에이전트 매칭 — RD-6) + ?limit=/
// ?offset= 페이지네이션(기본 limit=100). 저장소 미구성(nil)이면 빈 목록을 반환한다(AC-5).
func (h *ScheduleLogHandler) Logs(ctx api.Context) error {
	if h.logs == nil {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse([]ScheduleLogResponse{}))
	}
	filter := storage.ScheduleLogFilter{
		ScheduleID: ctx.Query("schedule_id"),
		RuleName:   ctx.Query("rule_name"),
		AgentID:    ctx.Query("agent_id"),
	}
	limit := parsePositiveInt(ctx.Query("limit"), 100)
	offset := parsePositiveInt(ctx.Query("offset"), 0)

	records, err := h.logs.List(ctx.Context(), filter, limit, offset)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toScheduleLogDTOs(records)))
}

// emptyTargets 는 대상별 상세가 없을 때(빈 값/유효하지 않은 JSON) 반환하는 빈 JSON
// 배열이다. 응답의 targets 가 항상 유효 JSON 배열이 되도록 보장한다.
var emptyTargets = json.RawMessage("[]")

// toScheduleLogDTOs 는 로그 레코드를 응답 DTO 로 변환한다. Targets 는 저장된 JSON
// 문자열을 그대로 통과시키되(구조화 데이터 전달), 빈 값 또는 유효하지 않은 JSON 은
// 빈 배열([])로 대체해 응답이 항상 유효한 JSON 이 되도록 방어한다.
func toScheduleLogDTOs(records []storage.ScheduleLogRecord) []ScheduleLogResponse {
	out := make([]ScheduleLogResponse, 0, len(records))
	for _, r := range records {
		targets := emptyTargets
		if r.Targets != "" && json.Valid([]byte(r.Targets)) {
			targets = json.RawMessage(r.Targets)
		}
		out = append(out, ScheduleLogResponse{
			ID:              r.ID,
			CorrelationID:   r.CorrelationID,
			RecordKind:      r.RecordKind,
			ScheduleID:      r.ScheduleID,
			RuleName:        r.RuleName,
			DeclaredAgentID: r.DeclaredAgentID,
			ActorAgentID:    r.ActorAgentID,
			TriggerTime:     r.TriggerTime,
			Target:          r.Target,
			Action:          r.Action,
			Result:          r.Result,
			Targets:         targets,
			Reason:          r.Reason,
			Timestamp:       r.Timestamp,
		})
	}
	return out
}
