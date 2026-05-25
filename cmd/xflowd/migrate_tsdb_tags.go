// @SPEC:SPEC-DEVICE-IDENTITY-001 Phase C § C2
// migrate_tsdb_tags.go — `xflowd migrate tsdb-tags` 서브명령.
//
// InfluxDB (v2 또는 v3) 의 schema 를 스캔하여 composite tag value 후보를
// 식별하고, composite → UUID 매핑을 device_ids.json 에서 조회하여
// Flux / SQL backfill 스크립트를 생성한다.
//
// 본 도구는 Influx 서버에 어떤 write 도 수행하지 않는다 (read-only). 출력은
// OutputDir 의 스크립트 파일이며, 운영자가 staging → production 순서로
// 직접 실행한다.
//
// 실제 schema discovery + 분류 + 스크립트 생성 로직은 internal/migrate/tsdbtags
// 패키지에 캡슐화되어 있다 (테스트 격리).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/migrate/tsdbtags"
)

// migrateTSDBTagsFlags 는 tsdb-tags 서브명령의 플래그 값을 보관한다.
type migrateTSDBTagsFlags struct {
	influxURL    string
	influxToken  string
	bucket       string
	org          string
	idRepo       string
	outputDir    string
	target       string
	measurements []string
	dryRun       bool
}

// newSchemaClientFn 은 SchemaClient 생성 함수의 변수 (테스트에서 mock 주입용).
//
// 본 변수는 production 에서 적절한 v2/v3 어댑터를 생성하는 기본 구현으로
// 초기화된다. 테스트는 본 변수를 fixture-driven mock 생성 함수로 swap 한다.
// 본 변수는 cmd 전역이므로 테스트 간 swap 후 복원 책임은 테스트에 있다.
var newSchemaClientFn = func(_ context.Context, _ tsdbtags.Options) (tsdbtags.SchemaClient, error) {
	return nil, errors.New("tsdb-tags 실제 client 구현이 아직 연결되지 않았습니다 (Phase C2 commit 4 에서 추가)")
}

// newMigrateTSDBTagsCmd 는 `xflowd migrate tsdb-tags` 명령을 생성한다.
//
// 동작 흐름:
//  1. 입력 검증: URL/token/bucket 필수, target=v2 면 org 필수.
//  2. Target 결정: auto 면 detect 패키지 가 X-Influxdb-Version 헤더로 추정.
//  3. Schema 스캔: 각 measurement 의 tag value 를 수집.
//  4. 분류: Mapped / Orphan / Ambiguous / UUIDAlready (composite.go).
//  5. 스크립트 생성: v2 → Flux, v3 → SQL + RUN.md (--dry-run 이면 skip).
//  6. 요약 출력.
func newMigrateTSDBTagsCmd() *cobra.Command {
	flags := &migrateTSDBTagsFlags{}

	cmd := &cobra.Command{
		Use:   "tsdb-tags",
		Short: "InfluxDB (v2/v3) 의 composite tag 를 UUID 로 backfill 하는 스크립트 생성",
		Long: `InfluxDB 의 schema 를 스캔하여 composite (agent:unit_id) tag value 를 식별하고,
device_ids.json 의 매핑을 사용해 UUID tag 를 추가하는 Flux / SQL 스크립트를 생성합니다.

본 도구는 Influx 서버에 어떤 write 도 수행하지 않습니다 (read-only). 생성된 스크립트는
운영자가 직접 staging 에서 리허설 후 production 에 적용합니다.

안전 장치:
  - read-only Influx access: 도구는 schema 조회만 수행
  - auto-version 감지: --target auto 면 서버 응답으로 v2/v3 자동 식별
  - dry-run 우선: --dry-run 으로 스크립트 생성 없이 영향 분석만
  - 매핑 검증: ambiguous mapping 발견 시 명시적 경고

예시:
  # 기본 (dry-run 권장):
  xflowd migrate tsdb-tags \
      --influx-url http://localhost:8086 \
      --influx-token <readonly-token> \
      --bucket xflow \
      --org acme \
      --target v2 \
      --dry-run

  # 실제 스크립트 생성:
  xflowd migrate tsdb-tags \
      --influx-url http://localhost:8086 \
      --influx-token <readonly-token> \
      --bucket xflow \
      --target auto \
      --output-dir ./tsdb-migrations

  # 특정 measurement 만 처리:
  xflowd migrate tsdb-tags \
      --influx-url http://localhost:8086 \
      --influx-token <readonly-token> \
      --bucket xflow \
      --org acme \
      --measurements indoor_temp,outdoor_temp`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMigrateTSDBTags(cmd.Context(), flags, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&flags.influxURL, "influx-url", "",
		"InfluxDB 서버 URL (예: http://localhost:8086, 필수)")
	cmd.Flags().StringVar(&flags.influxToken, "influx-token", "",
		"InfluxDB 인증 토큰 (read-only 권한 권장, 필수)")
	cmd.Flags().StringVar(&flags.bucket, "bucket", "",
		"v2 bucket 명 또는 v3 database 명 (필수)")
	cmd.Flags().StringVar(&flags.org, "org", "",
		"v2 organization 명 (target=v2 시 필수)")
	cmd.Flags().StringVar(&flags.idRepo, "id-repo", "",
		"device_ids.json 경로 (기본: ~/.xflow/storage/device_ids/device_ids.json)")
	cmd.Flags().StringVar(&flags.outputDir, "output-dir", "",
		"스크립트 출력 디렉토리 (기본: ./tsdb-migrations-<UTC-timestamp>)")
	cmd.Flags().StringVar(&flags.target, "target", "auto",
		"InfluxDB 버전 (v2 / v3 / auto)")
	cmd.Flags().StringSliceVar(&flags.measurements, "measurements", nil,
		"특정 measurement 만 처리 (콤마 구분, 미지정 시 전체 스캔)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false,
		"스크립트 생성 없이 영향 분석만 수행")

	_ = cmd.MarkFlagRequired("influx-url")
	_ = cmd.MarkFlagRequired("influx-token")
	_ = cmd.MarkFlagRequired("bucket")

	return cmd
}

// runMigrateTSDBTags 는 tsdb-tags 마이그레이션의 실행 흐름을 관장한다.
//
// stdin 은 사용하지 않는다 (tsdb-tags 는 비대화형 — 스크립트 생성만 하고
// 실제 적용은 운영자 책임).
func runMigrateTSDBTags(
	ctx context.Context,
	flags *migrateTSDBTagsFlags,
	stdout, stderr io.Writer,
) error {
	opts := tsdbtags.Options{
		InfluxURL:    flags.influxURL,
		InfluxToken:  flags.influxToken,
		Bucket:       flags.bucket,
		Org:          flags.org,
		IDRepoPath:   flags.idRepo,
		OutputDir:    flags.outputDir,
		Target:       tsdbtags.Target(flags.target),
		Measurements: flags.measurements,
		DryRun:       flags.dryRun,
		Stdout:       stdout,
		Stderr:       stderr,
	}

	// id-repo 기본 경로 해석 (xflowd 데몬과 동일한 표준 위치).
	if opts.IDRepoPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home directory 조회 실패: %w", err)
		}
		opts.IDRepoPath = tsdbtags.ResolveIDRepoPath(opts, home)
	}

	client, err := newSchemaClientFn(ctx, opts)
	if err != nil {
		return fmt.Errorf("InfluxDB 클라이언트 생성 실패: %w", err)
	}
	defer func() { _ = client.Close() }()

	planner, err := tsdbtags.NewPlanner(ctx, opts, client)
	if err != nil {
		return fmt.Errorf("초기화 실패: %w", err)
	}

	fmt.Fprintf(stdout, "Connecting to %s (target=%s)...\n", opts.InfluxURL, client.Version())

	// commit 1 단계에서는 골격만 — 실제 Plan/Apply 로직은 후속 commit 에서 추가.
	_ = planner
	fmt.Fprintln(stdout, "tsdb-tags 실행 로직은 Phase C2 후속 commit 에서 연결됩니다.")
	return nil
}
