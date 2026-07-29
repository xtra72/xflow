package xsfm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// ---------------------------------------------------------------------------
// B7 — 제어 감사 배선 (REQ-XSFM-001-06-03)
// ---------------------------------------------------------------------------
//
// 제어 감사는 원격 관리 감사 저장소(storage.RemoteAuditRepository)로 개별/그룹 제어를
// 기록한다. 저장소는 device_id_repo(패키지-레벨 싱글턴 + setter, nil-safe 접근) 패턴을
// 그대로 미러한다: main.go 가 startup 시 SetAuditRepository 로 단 한 번 주입하며(B8),
// 미설정(nil) 상태에서도 제어 흐름이 죽지 않도록 접근자가 nil 을 반환하면 감사를 no-op 으로
// 건너뛴다. 감사 기록 실패는 best-effort 로 로깅만 하고 제어 명령을 실패시키지 않는다.
//
// RemoteAuditRecord 필드 매핑(원 SPEC-REMOTE-001 은 노드 instance_id 기준):
//   - InstanceID    : 감사 대상 식별자 — 개별은 device_id, 그룹은 셀렉터 값(group_id 등).
//   - Actor         : 수행 주체 — 에이전트 이름/ID(원격 관리 admin username 이 없는 로컬 제어).
//   - Action        : 항상 command(제어 명령).
//   - Domain        : 항상 device(디바이스 제어).
//   - CommandAction : 개별은 제어 명령(set_power/...), 그룹은 셀렉터 종류(group_id/station/line).
//   - Result        : ok | error(그룹은 전 멤버 ok 여야 ok).
//   - Reason        : 비밀-아님 분류 — 개별은 상태(timeout 등), 그룹은 셀렉터+멤버 목록.
//   - Timestamp     : epoch milliseconds(int64, 프로젝트 규약).
//
// 시크릿 금지(REQ-F06 준수): 제어 인자/페이로드는 기록하지 않고 device_id/command/결과만 남긴다.

var (
	auditRepoMu sync.RWMutex
	auditRepo   storage.RemoteAuditRepository
)

// SetAuditRepository 는 패키지-레벨 제어 감사 저장소를 설정한다(device_id_repo 패턴).
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다(B8 배선).
func SetAuditRepository(r storage.RemoteAuditRepository) {
	auditRepoMu.Lock()
	defer auditRepoMu.Unlock()
	auditRepo = r
}

// getAuditRepository 는 현재 설정된 감사 저장소를 반환한다(없으면 nil).
// 호출자는 nil 체크 후 감사를 no-op 으로 건너뛴다(graceful degradation).
func getAuditRepository() storage.RemoteAuditRepository {
	auditRepoMu.RLock()
	defer auditRepoMu.RUnlock()
	return auditRepo
}

// auditActor 는 감사 레코드의 Actor("who")로 사용할 에이전트 식별자를 반환한다.
// 로컬 제어에는 원격 관리 admin username 이 없으므로 에이전트 이름(없으면 ID)을 쓴다.
// 감사 호출 시점(controlDevice/fanOutControl)에는 a.mu 를 보유하지 않으므로 여기서 RLock 을
// 취득해도 재진입 deadlock 이 없다.
func (a *XSFMAgent) auditActor() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.agentConfig.Name != "" {
		return a.agentConfig.Name
	}
	return a.agentConfig.ID
}

// recordControlAudit 는 단일 디바이스 제어 감사 레코드를 기록한다(REQ-XSFM-001-06-03).
//
// controlDevice 가 모든 제어 경로(개별 명령 + 셀렉터 fan-out 의 멤버별 실행)에서 호출하므로,
// device_id 단위의 who/when/what 감사가 두 경로에 일관되게 남는다. 저장소 미설정이면 no-op,
// 기록 실패는 로깅만 하고 제어를 실패시키지 않는다(best-effort). 호출 시 a.mu 미보유.
func (a *XSFMAgent) recordControlAudit(res memberResult) {
	repo := getAuditRepository()
	if repo == nil {
		return // 감사 저장소 미설정 → graceful no-op
	}
	result := storage.AuditResultOK
	reason := ""
	if res.Status != "ok" {
		result = storage.AuditResultError
		reason = res.Status // "timeout" 등(비밀 아님)
	}
	rec := storage.RemoteAuditRecord{
		InstanceID:    res.DeviceID,
		Actor:         a.auditActor(),
		Action:        storage.AuditActionCommand,
		Domain:        "device",
		CommandAction: res.Command,
		Result:        result,
		Reason:        reason,
		Timestamp:     time.Now().UnixMilli(),
	}
	if err := repo.Append(context.Background(), rec); err != nil {
		a.logger.Warn("xsfm: control audit append failed", "device_id", res.DeviceID, "error", err)
	}
}

// recordGroupAudit 는 셀렉터 fan-out 의 그룹 감사 레코드를 기록한다(REQ-XSFM-001-06-03 그룹 절).
//
// 멤버별 device_id 감사는 controlDevice→recordControlAudit 가 이미 남기므로, 여기서는 그룹
// 차원의 요약 레코드 하나(대상 셀렉터 종류·값 + fan-out 멤버 목록 + 집계 결과)를 추가한다.
// 저장소 미설정이면 no-op, 실패는 로깅만 한다(best-effort). 호출 시 a.mu 미보유.
func (a *XSFMAgent) recordGroupAudit(sel selectorRef, results []groupResult) {
	repo := getAuditRepository()
	if repo == nil {
		return
	}
	members := make([]string, 0, len(results))
	ok := 0
	for _, r := range results {
		members = append(members, r.DeviceID)
		if r.Status == "ok" {
			ok++
		}
	}
	result := storage.AuditResultOK
	if ok != len(results) {
		result = storage.AuditResultError
	}
	rec := storage.RemoteAuditRecord{
		InstanceID:    sel.Value, // 그룹/셀렉터 값이 감사 대상
		Actor:         a.auditActor(),
		Action:        storage.AuditActionCommand,
		Domain:        "device",
		CommandAction: sel.Type, // group_id | station | line (fan-out 종류)
		Result:        result,
		Reason:        fmt.Sprintf("%s=%s members=[%s] ok=%d/%d", sel.Type, sel.Value, strings.Join(members, ","), ok, len(results)),
		Timestamp:     time.Now().UnixMilli(),
	}
	if err := repo.Append(context.Background(), rec); err != nil {
		a.logger.Warn("xsfm: group audit append failed", "selector", sel.Value, "error", err)
	}
}
