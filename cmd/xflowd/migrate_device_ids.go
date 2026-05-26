// @SPEC:SPEC-DEVICE-IDENTITY-001 Phase C § C1
// migrate_device_ids.go — `xflowd migrate device-ids` 서브명령.
//
// 영속 메타데이터 파일 (device_metadata.json) 의 map key 를 composite (agent:unit_id)
// 에서 UUID 로 변환한다. DeviceIDRepository (device_ids.json) 가 매핑의 권위
// (source of truth) 이며, 매핑되지 않은 composite 는 정책에 따라 skip 또는 abort.
//
// 실제 변환 로직은 internal/migrate/deviceids 패키지에 캡슐화되어 있다 (테스트 격리).
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/migrate/deviceids"
)

// migrateDeviceIDsFlags 는 device-ids 서브명령의 플래그 값을 보관한다.
type migrateDeviceIDsFlags struct {
	metadataDir string
	backupDir   string
	idRepo      string
	dryRun      bool
	strict      bool
	assumeYes   bool
}

// newMigrateDeviceIDsCmd 는 `xflowd migrate device-ids` 명령을 생성한다.
//
// 동작 흐름:
//  1. 검증: metadata-dir 와 id-repo 의 존재 확인.
//  2. 계획: composite key 를 UUID 로 매핑하고 결과 (변환/skip/ambiguous/orphan) 보고.
//  3. dry-run 이거나 사용자 확인이 거부되면 종료.
//  4. 백업: backup-dir 에 원본 device_metadata.json 의 sha256 manifest 와 함께 복사.
//  5. 실행: in-place 로 map key 를 UUID 로 변환하고 atomic rename.
//  6. 검증: 변환 후 entry 수와 메타데이터 sha256 합산이 백업과 일치하는지 확인.
func newMigrateDeviceIDsCmd() *cobra.Command {
	flags := &migrateDeviceIDsFlags{}

	cmd := &cobra.Command{
		Use:   "device-ids",
		Short: "device_metadata.json 의 composite key 를 UUID 로 변환",
		Long: `device_metadata.json 의 map key 를 composite (agent:unit_id) 에서 UUID 로 변환합니다.

권위 매핑 (composite → UUID) 은 --id-repo 디렉토리의 device_ids.json 에서 읽어옵니다.

안전 장치:
  - 자동 백업: 변경 전 metadata 파일과 manifest (sha256) 를 backup-dir 에 복사
  - dry-run: --dry-run 플래그로 실제 변경 없이 계획만 출력
  - strict: --strict 플래그로 ambiguous mapping 발견 시 abort (기본은 skip+warn)
  - idempotency: 이미 UUID 명명된 키는 건드리지 않음, 재실행 시 0 변환 보고
  - atomic rename: 임시 파일에 작성 후 원자적 교체

예시:
  # 기본 (dry-run 권장):
  xflowd migrate device-ids --metadata-dir /var/lib/xflow/device_metadata \
      --id-repo /var/lib/xflow/device_ids --dry-run

  # 실제 실행 (운영자 확인):
  xflowd migrate device-ids --metadata-dir /var/lib/xflow/device_metadata \
      --id-repo /var/lib/xflow/device_ids

  # CI/배치 (대화형 확인 건너뛰기, ambiguous 발견 시 abort):
  xflowd migrate device-ids --metadata-dir /var/lib/xflow/device_metadata \
      --id-repo /var/lib/xflow/device_ids --yes --strict`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// SPEC-DEVICE-IDENTITY-001 Phase D § D-T19: xflowd v1.0 (greenfield)
			// 부터 composite key 자체가 시스템에서 사라졌으므로 마이그레이션 대상
			// 부재. 명령은 deprecated noop 으로 유지 (brownfield 사용자가 v0.x
			// 환경에서 사전 마이그레이션 후 v1.0 으로 업그레이드하는 경로 보존).
			fmt.Fprintln(cmd.OutOrStdout(),
				"DEPRECATED: xflowd migrate device-ids — v1.0 환경에는 마이그레이션 대상 없음.")
			fmt.Fprintln(cmd.OutOrStdout(),
				"  composite key 형식은 이미 제거되었으므로 본 명령은 noop 으로 종료합니다.")
			fmt.Fprintln(cmd.OutOrStdout(),
				"  brownfield 사용자: v0.x (Phase C3 완료 시점) 버전에서 마이그레이션 수행 후 v1.0 으로 업그레이드하십시오.")
			fmt.Fprintln(cmd.OutOrStdout(),
				"  핵심 로직 (internal/migrate/deviceids) 은 보존되어 있어 향후 필요 시 재활성화 가능합니다.")
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.metadataDir, "metadata-dir", "",
		"composite-key 메타데이터 디렉토리 (Phase D 부터 무시됨)")
	cmd.Flags().StringVar(&flags.idRepo, "id-repo", "",
		"DeviceID 저장소 디렉토리 (Phase D 부터 무시됨)")
	cmd.Flags().StringVar(&flags.backupDir, "backup-dir", "",
		"백업 저장 위치 (Phase D 부터 무시됨)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false,
		"실제 변경 없이 계획만 출력 (Phase D 부터 무시됨)")
	cmd.Flags().BoolVar(&flags.strict, "strict", false,
		"ambiguous/orphan mapping 발견 시 abort (Phase D 부터 무시됨)")
	cmd.Flags().BoolVar(&flags.assumeYes, "yes", false,
		"대화형 확인 건너뛰기 (Phase D 부터 무시됨)")

	// Phase D § D-T19: 필수 플래그 강제 제거 — 사용자가 빈 명령으로 실행 시에도
	// deprecated 안내 메시지가 출력되도록 한다.

	return cmd
}

// Phase D § D-T19: runMigrateDeviceIDs / migrateDeviceIDsFlags 핵심 로직 함수는
// brownfield 사용자의 잠재적 필요를 위해 코드베이스에 보존된다 (v1.0 의
// migrate device-ids 명령은 noop 이므로 호출 사이트 없음). 아래 reference 는
// "unused" linter warning 회피용이며, 향후 명령 재활성화 시 RunE 에서 직접
// 호출하면 된다.
var _ = runMigrateDeviceIDs
var _ migrateDeviceIDsFlags

// runMigrateDeviceIDs 는 device-ids 마이그레이션의 실행 흐름을 관장한다.
//
// 입출력을 io.Reader/io.Writer 로 주입받아 테스트에서 stdin/stdout 격리가 가능하다.
func runMigrateDeviceIDs(
	ctx context.Context,
	flags *migrateDeviceIDsFlags,
	stdout, stderr io.Writer,
	stdin io.Reader,
) error {
	opts := deviceids.Options{
		MetadataDir: flags.metadataDir,
		IDRepoDir:   flags.idRepo,
		BackupDir:   flags.backupDir,
		DryRun:      flags.dryRun,
		Strict:      flags.strict,
		Stdout:      stdout,
		Stderr:      stderr,
	}

	planner, err := deviceids.NewPlanner(ctx, opts)
	if err != nil {
		return fmt.Errorf("초기화 실패: %w", err)
	}

	plan, err := planner.Plan(ctx)
	if err != nil {
		return fmt.Errorf("계획 단계 실패: %w", err)
	}

	// 항상 계획을 stdout 으로 출력 (dry-run / 실제 실행 공통).
	if err := plan.Print(stdout); err != nil {
		return fmt.Errorf("계획 출력 실패: %w", err)
	}

	if flags.strict && plan.HasAmbiguous() {
		return fmt.Errorf("strict 모드에서 ambiguous mapping %d 건 발견 — abort", plan.AmbiguousCount())
	}

	if flags.dryRun {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "[dry-run] 실제 변경은 수행되지 않았습니다.")
		return nil
	}

	if plan.ConvertCount() == 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "변환할 composite key 가 없습니다 (idempotent no-op).")
		return nil
	}

	// 비대화형 모드가 아니면 사용자 확인.
	if !flags.assumeYes {
		ok, err := confirmMigrate(stdout, stdin)
		if err != nil {
			return fmt.Errorf("사용자 확인 실패: %w", err)
		}
		if !ok {
			fmt.Fprintln(stdout, "사용자 취소.")
			return nil
		}
	}

	result, err := planner.Apply(ctx, plan)
	if err != nil {
		return fmt.Errorf("마이그레이션 실행 실패: %w", err)
	}

	if err := result.Print(stdout); err != nil {
		return fmt.Errorf("결과 출력 실패: %w", err)
	}

	if !result.Verified {
		return errors.New("검증 단계 실패: 변환 후 sha256 합산이 백업과 일치하지 않습니다 (백업에서 수동 복원 필요)")
	}
	return nil
}

// confirmMigrate 는 사용자에게 마이그레이션 실행을 확인한다.
//
// 입력이 "y" 또는 "yes" 이면 true, 그 외 (빈 입력 / "n" / EOF) 면 false 를 반환한다.
func confirmMigrate(stdout io.Writer, stdin io.Reader) (bool, error) {
	fmt.Fprint(stdout, "마이그레이션을 실행하시겠습니까? [y/N]: ")
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
