// migrate_tsdb_tags_test.go — `xflowd migrate tsdb-tags` CLI 통합 테스트.
//
// 실제 Influx 서버에 접근하지 않는다 (안전 가드). 모든 테스트는
// newSchemaClientFn 을 mock 으로 swap 한 뒤 cobra Execute 를 호출한다.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xtra/xflow/internal/migrate/tsdbtags"
)

// withMockSchemaClient 는 newSchemaClientFn 을 mock 으로 swap 한 뒤 복원한다.
func withMockSchemaClient(t *testing.T, schema tsdbtags.MockSchema) func() {
	t.Helper()
	orig := newSchemaClientFn
	newSchemaClientFn = func(_ context.Context, _ tsdbtags.Options) (tsdbtags.SchemaClient, error) {
		return tsdbtags.NewMockClient(schema), nil
	}
	return func() { newSchemaClientFn = orig }
}

// setupTSDBFixture 는 device_ids.json fixture 를 임시 디렉토리에 작성한다.
func setupTSDBFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := `{
  "lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e"
}`
	if err := os.WriteFile(filepath.Join(dir, "device_ids.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return filepath.Join(dir, "device_ids.json")
}

// fixtureSchema 는 두 composite key 를 포함한 v2 schema fixture 이다.
func fixtureSchema() tsdbtags.MockSchema {
	return tsdbtags.MockSchema{
		Version:      tsdbtags.TargetV2,
		Measurements: []string{"indoor_temp"},
		TagsByMeasurement: map[string]map[string][]string{
			"indoor_temp": {
				"device_id": {"lgcnp:81", "lgcnp:82"},
			},
		},
	}
}

// runCLITSDB 는 cobra Execute 를 호출하고 stdout/stderr/error 를 반환한다.
func runCLITSDB(t *testing.T, args []string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newMigrateCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestMigrateTSDBCLI_Help(t *testing.T) {
	t.Parallel()

	stdout, _, err := runCLITSDB(t, []string{"tsdb-tags", "--help"})
	if err != nil {
		t.Fatalf("--help 에러: %v", err)
	}
	for _, want := range []string{
		"--influx-url",
		"--influx-token",
		"--bucket",
		"--target",
		"--dry-run",
		"--measurements",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help 출력에 %q 없음", want)
		}
	}
}

func TestMigrateTSDBCLI_RequiredFlags(t *testing.T) {
	t.Parallel()

	_, _, err := runCLITSDB(t, []string{"tsdb-tags"})
	if err == nil {
		t.Errorf("필수 플래그 누락 시 에러를 반환해야 함")
	}
}

func TestMigrateTSDBCLI_DryRun_V2(t *testing.T) {
	defer withMockSchemaClient(t, fixtureSchema())()

	idRepoPath := setupTSDBFixture(t)
	outDir := filepath.Join(t.TempDir(), "out")

	stdout, _, err := runCLITSDB(t, []string{
		"tsdb-tags",
		"--influx-url", "http://mock:8086",
		"--influx-token", "tok",
		"--bucket", "xflow",
		"--org", "acme",
		"--target", "v2",
		"--id-repo", idRepoPath,
		"--output-dir", outDir,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("CLI execute: %v", err)
	}
	if !strings.Contains(stdout, "mapped=2") {
		t.Errorf("stdout 에 mapped=2 없음:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("stdout 에 dry-run 표시 없음:\n%s", stdout)
	}
	// dry-run 이므로 outDir 가 생성되지 않아야 함.
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("dry-run 후 output-dir 가 생성됨")
	}
}

func TestMigrateTSDBCLI_FullRun_V2(t *testing.T) {
	defer withMockSchemaClient(t, fixtureSchema())()

	idRepoPath := setupTSDBFixture(t)
	outDir := filepath.Join(t.TempDir(), "out")

	stdout, _, err := runCLITSDB(t, []string{
		"tsdb-tags",
		"--influx-url", "http://mock:8086",
		"--influx-token", "tok",
		"--bucket", "xflow",
		"--org", "acme",
		"--target", "v2",
		"--id-repo", idRepoPath,
		"--output-dir", outDir,
	})
	if err != nil {
		t.Fatalf("CLI execute: %v", err)
	}
	if !strings.Contains(stdout, "mapped=2") {
		t.Errorf("stdout 에 mapped=2 없음:\n%s", stdout)
	}
	// 출력 파일 검증.
	flux := filepath.Join(outDir, "migration-v2.flux")
	if _, err := os.Stat(flux); err != nil {
		t.Errorf("migration-v2.flux 가 생성되지 않음: %v", err)
	}
	readme := filepath.Join(outDir, "RUN.md")
	if _, err := os.Stat(readme); err != nil {
		t.Errorf("RUN.md 가 생성되지 않음: %v", err)
	}
}

func TestMigrateTSDBCLI_Idempotency(t *testing.T) {
	defer withMockSchemaClient(t, fixtureSchema())()

	idRepoPath := setupTSDBFixture(t)
	outDir1 := filepath.Join(t.TempDir(), "out1")
	outDir2 := filepath.Join(t.TempDir(), "out2")

	baseArgs := []string{
		"tsdb-tags",
		"--influx-url", "http://mock:8086",
		"--influx-token", "tok",
		"--bucket", "xflow",
		"--org", "acme",
		"--target", "v2",
		"--id-repo", idRepoPath,
	}

	// 1차 실행.
	_, _, err := runCLITSDB(t, append(baseArgs, "--output-dir", outDir1))
	if err != nil {
		t.Fatalf("1차 CLI: %v", err)
	}

	// 2차 실행 (다른 output-dir, 동일 schema + 동일 ids → 동일 mapped 수).
	stdout2, _, err := runCLITSDB(t, append(baseArgs, "--output-dir", outDir2))
	if err != nil {
		t.Fatalf("2차 CLI: %v", err)
	}
	if !strings.Contains(stdout2, "mapped=2") {
		t.Errorf("2차 실행에서도 mapped=2 여야 함:\n%s", stdout2)
	}

	// 두 디렉토리의 migration-v2.flux 의 매핑 내용은 동일해야 한다
	// (timestamp 헤더만 다르고 매핑 블록은 같다).
	body1, _ := os.ReadFile(filepath.Join(outDir1, "migration-v2.flux"))
	body2, _ := os.ReadFile(filepath.Join(outDir2, "migration-v2.flux"))
	// 두 파일 모두 두 매핑을 포함.
	for _, key := range []string{"lgcnp:81", "lgcnp:82",
		"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e"} {
		if !strings.Contains(string(body1), key) {
			t.Errorf("1차 flux 에 %q 없음", key)
		}
		if !strings.Contains(string(body2), key) {
			t.Errorf("2차 flux 에 %q 없음", key)
		}
	}
}

func TestMigrateTSDBCLI_AmbiguousWarning(t *testing.T) {
	defer withMockSchemaClient(t, tsdbtags.MockSchema{
		Version:      tsdbtags.TargetV2,
		Measurements: []string{"hvac_status"},
		TagsByMeasurement: map[string]map[string][]string{
			"hvac_status": {"device_id": {"lgcnp:99a", "lgcnp:99b"}},
		},
	})()

	dir := t.TempDir()
	idRepoPath := filepath.Join(dir, "device_ids.json")
	uidShared := "e9f80aac-9185-4f70-d162-b07fce5f6071"
	ids := `{"lgcnp:99a":"` + uidShared + `","lgcnp:99b":"` + uidShared + `"}`
	_ = os.WriteFile(idRepoPath, []byte(ids), 0o644)

	outDir := filepath.Join(t.TempDir(), "out")
	_, stderr, err := runCLITSDB(t, []string{
		"tsdb-tags",
		"--influx-url", "http://mock:8086",
		"--influx-token", "tok",
		"--bucket", "xflow",
		"--org", "acme",
		"--target", "v2",
		"--id-repo", idRepoPath,
		"--output-dir", outDir,
	})
	if err != nil {
		t.Fatalf("CLI: %v", err)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Errorf("stderr 에 ambiguous 경고 없음:\n%s", stderr)
	}
}

func TestMigrateTSDBCLI_InvalidTarget(t *testing.T) {
	t.Parallel()

	_, _, err := runCLITSDB(t, []string{
		"tsdb-tags",
		"--influx-url", "http://mock:8086",
		"--influx-token", "tok",
		"--bucket", "xflow",
		"--target", "v9",
	})
	if err == nil {
		t.Errorf("잘못된 target 에서 에러를 반환해야 함")
	}
}
