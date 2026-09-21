// audit_retention.go 는 원격 관리 로그의 보존 기간을 집행한다
// (@SPEC:SPEC-REMOTE-LOG-001).
//
// 감사 표는 append-only 이고 그것이 옳다 — 기록을 골라 지우는 문을 열면 감사의
// 뜻이 사라진다. 다만 **나이**로 지우는 문 하나는 필요하다. 연결·끊어짐까지 남기기
// 시작하면 불안정한 노드 하나가 표를 빠르게 불리고, 그 표는 노드 목록과 같은 DB 에
// 산다. 상한이 없는 표는 언젠가 반드시 문제가 된다.
package remote

import (
	"context"
	"log/slog"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// auditPurgeInterval 은 정리를 시도하는 주기이다. 보존 기간이 날 단위이므로
// 시간 단위로 충분하다 — 더 자주 돌아도 지울 것이 없다.
const auditPurgeInterval = 1 * time.Hour

// StartAuditRetention 은 보존 기간을 넘긴 감사 기록을 주기적으로 지우는 고루틴을
// 시작한다. ctx 가 취소되면 멈춘다.
//
// retention 이 0 이하이면 아무것도 시작하지 않는다 — "보존 기간 미설정" 은 "무제한
// 보존" 이며, 그것을 "전부 삭제" 로 읽으면 설정 실수 한 번이 기록을 통째로 날린다.
//
// 시작 직후 한 번 돌린다. 서버가 자주 재시작되는 환경에서 주기만 기다리면 정리가
// 영영 돌지 않을 수 있다.
func StartAuditRetention(ctx context.Context, repo storage.RemoteAuditRepository, retention time.Duration, logger *slog.Logger) {
	if repo == nil || retention <= 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	go func() {
		ticker := time.NewTicker(auditPurgeInterval)
		defer ticker.Stop()
		purgeAuditOnce(ctx, repo, retention, logger)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purgeAuditOnce(ctx, repo, retention, logger)
			}
		}
	}()
}

// purgeAuditOnce 는 보존 기간을 넘긴 기록을 한 번 지운다.
func purgeAuditOnce(ctx context.Context, repo storage.RemoteAuditRepository, retention time.Duration, logger *slog.Logger) {
	cutoff := time.Now().Add(-retention).UnixMilli()
	removed, err := repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		logger.Warn("원격 관리 로그 정리 실패", "error", err)
		return
	}
	if removed > 0 {
		logger.Info("원격 관리 로그 정리",
			"removed", removed, "retention", retention.String())
	}
}
