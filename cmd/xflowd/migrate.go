// @SPEC:SPEC-DEVICE-IDENTITY-001 Phase C § C1
// migrate.go — `xflowd migrate` Cobra 서브커맨드 그룹.
//
// 명령 구조:
//
//	xflowd migrate                              # 도움말
//	  device-ids [flags]                        # composite-key 메타데이터를 UUID key 로 변환 (C1)
//	  tsdb-tags  [flags]                        # InfluxDB composite tag → UUID backfill 스크립트 생성 (C2)
//
// 디자인 결정:
//   - C3 (dual-tag 운영) 은 별도 세션 / 별도 서브명령.
//   - 본 파일은 명령 그룹의 진입점만 정의. 실제 마이그레이션 로직은 internal/migrate/ 패키지.
//   - 도구는 모든 실행 단계에서 백업 + 검증 우선 (M7/M8 의 안전망).
package main

import (
	"github.com/spf13/cobra"
)

// newMigrateCmd 는 `xflowd migrate` 명령 그룹을 생성한다.
//
// 본 명령 자체는 도움말만 출력하며, 실제 동작은 서브명령 (device-ids 등) 이 담당한다.
// 운영자 안전을 위해 모든 서브명령은 dry-run 우선, 백업 자동 생성, atomic rename
// 패턴을 강제한다.
func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "디바이스 ID 체계 마이그레이션 도구 모음",
		Long: `xflow 의 영속 데이터를 composite key (agent:unit_id) 에서 UUID key 로 변환합니다.

SPEC-DEVICE-IDENTITY-001 Phase C 의 운영 도구로, 모든 서브명령은:
  - 자동 백업 (백업 디렉토리 미지정 시 메타데이터 디렉토리 아래 .backup-<timestamp>)
  - dry-run 우선 (실제 변경 없이 계획만 출력)
  - sha256 해시 검증 (변환 전후 메타데이터 동일성)
  - atomic rename (부분 변경 방지)
  - idempotency (재실행 시 변환 대상 없으면 no-op)

상세 운영 가이드는 docs/migration/device-identity.md 를 참조하세요.`,
	}

	cmd.AddCommand(newMigrateDeviceIDsCmd())
	cmd.AddCommand(newMigrateTSDBTagsCmd())
	// @spec SPEC-STORE-004 (O3): 영속 store 의 bare key → 기본 시리즈 인코딩 키 변환.
	cmd.AddCommand(newMigrateStoreSeriesCmd())

	return cmd
}
