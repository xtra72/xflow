// @spec SPEC-STORE-004 (O3, M6)
// migrate_store_series.go — `xflowd migrate store-series` 서브명령.
//
// 영속 store 백엔드의 "시리즈 도입 이전(bare key)" 엔트리를 (key,"unknown",{})
// 기본 시리즈 인코딩 키로 변환하는 일회성 마이그레이션 진입점이다.
//
// 중요 (가정 A1/A6): 현재 운영 store 에이전트는 VolatileStore 만 사용하며 활성
// 영속 백엔드(StoreRepository 구현)가 없다. 인메모리 데이터는 재시작 시 휘발되므로
// 변환이 불필요하다(A6). 따라서 본 명령은 라이브 데이터에 자동 실행하지 않으며,
// 활성 영속 백엔드가 없는 현 상태에서는 안내 메시지를 출력하고 noop 으로 종료한다
// (migrate tsdb-tags 와 동일한 deprecated-noop 정책).
//
// 실제 변환 로직(Plan/Apply/dry-run/백업/검증/멱등)은 internal/migrate/storeseries
// 패키지에 완전히 구현·테스트되어 있으며, 향후 영속 백엔드가 활성화되면 그 Repository
// 구현을 runStoreSeriesMigration 에 주입하여 즉시 사용할 수 있다.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/migrate/storeseries"
)

// migrateStoreSeriesFlags 는 store-series 서브명령의 플래그 값을 보관한다.
type migrateStoreSeriesFlags struct {
	dryRun bool
}

// newMigrateStoreSeriesCmd 는 `xflowd migrate store-series` 명령을 생성한다.
func newMigrateStoreSeriesCmd() *cobra.Command {
	flags := &migrateStoreSeriesFlags{}

	cmd := &cobra.Command{
		Use:   "store-series",
		Short: "영속 store 의 bare key 를 기본 시리즈 인코딩 키로 변환 (SPEC-STORE-004)",
		Long: `영속 store 백엔드의 시리즈 도입 이전(bare key) 엔트리를
(key, "unknown", {}) 기본 시리즈 인코딩 키로 변환합니다.

가정 (SPEC-STORE-004 §A1/A6):
  - 현재 운영 store 에이전트는 VolatileStore(인메모리) 만 사용합니다.
  - 인메모리 데이터는 재시작 시 휘발되므로 변환이 불필요합니다(A6).
    재시작 후 신규 쓰기가 시리즈 모델로 자연 흡수됩니다.
  - 활성 영속 백엔드(StoreRepository 구현)가 없으므로, 본 명령은 현 상태에서
    변환 대상이 없으며 안내 후 noop 으로 종료합니다.

안전 장치 (영속 백엔드 활성 시):
  - 변환 전 백업 (sha256 manifest)
  - dry-run 우선 (--dry-run: 무엇이 어떻게 바뀌는지 출력만, 실제 변경 없음)
  - 멱등성 (이미 시리즈 인코딩된 키는 건너뜀, 재실행 안전)
  - 변환 후 검증 (변환 건수 + value sha256 합산 일치)

핵심 변환 로직은 internal/migrate/storeseries 패키지에 구현·테스트되어 있으며,
향후 영속 백엔드 활성화 시 Repository 구현을 주입하여 사용합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStoreSeriesMigrationNoop(cmd.OutOrStdout(), flags)
		},
	}

	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false,
		"변환 없이 영향 분석만 수행 (영속 백엔드 활성 시)")

	return cmd
}

// runStoreSeriesMigrationNoop 은 활성 영속 백엔드가 없는 현 상태에서의 안내 noop 이다.
//
// 운영자에게 (1) 인메모리는 변환 불필요(A6), (2) 활성 영속 백엔드 부재(A1),
// (3) 향후 활성화 시 사용 가능한 유틸 위치를 안내한다.
func runStoreSeriesMigrationNoop(stdout io.Writer, flags *migrateStoreSeriesFlags) error {
	fmt.Fprintln(stdout,
		"store-series: 활성 영속 store 백엔드가 없습니다 — 변환 대상 없음 (noop).")
	fmt.Fprintln(stdout,
		"  - 인메모리(VolatileStore) 데이터는 재시작 시 휘발되므로 변환이 불필요합니다 (A6).")
	fmt.Fprintln(stdout,
		"  - 현재 store 에이전트 런타임은 VolatileStore 만 사용합니다 (A1).")
	fmt.Fprintln(stdout,
		"  - 영속 백엔드 활성화 시: internal/migrate/storeseries.NewPlanner 로 Plan/Apply 를 수행하세요.")
	if flags.dryRun {
		fmt.Fprintln(stdout, "  - (--dry-run 지정됨: 영속 백엔드 부재로 분석할 데이터가 없습니다.)")
	}
	return nil
}

// runStoreSeriesMigration 은 향후 영속 백엔드 활성 시 호출되는 실제 변환 흐름이다.
//
// 활성 영속 백엔드의 storeseries.Repository 구현을 주입받아 Plan → 출력 →
// (dry-run 이 아니면) Apply 를 수행한다. 현재는 활성 백엔드가 없어 호출 사이트가
// 없으나, 핵심 로직 보존을 위해 정의해 둔다 (storeseries 패키지가 모든 안전망을 제공).
func runStoreSeriesMigration(
	ctx context.Context,
	repo storeseries.Repository,
	flags *migrateStoreSeriesFlags,
	stdout, stderr io.Writer,
) error {
	planner, err := storeseries.NewPlanner(repo, storeseries.Options{
		DryRun: flags.dryRun,
		Backup: stderr, // 백업 manifest 는 stderr 로 (stdout 은 요약 전용).
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return fmt.Errorf("store-series 초기화 실패: %w", err)
	}

	plan, err := planner.Plan(ctx)
	if err != nil {
		return fmt.Errorf("store-series 계획 단계 실패: %w", err)
	}
	if err := plan.Print(stdout); err != nil {
		return fmt.Errorf("store-series 계획 출력 실패: %w", err)
	}

	result, err := planner.Apply(ctx)
	if err != nil {
		return fmt.Errorf("store-series 적용 단계 실패: %w", err)
	}
	if err := result.Print(stdout); err != nil {
		return fmt.Errorf("store-series 결과 출력 실패: %w", err)
	}

	if !result.DryRun && !result.Verified {
		fmt.Fprintln(stderr,
			"WARN: 변환 후 검증 실패 — value sha256 합산 또는 건수 불일치. 백업으로 복원을 검토하세요.")
	}
	return nil
}
