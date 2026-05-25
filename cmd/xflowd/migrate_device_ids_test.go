// migrate_device_ids_test.go — `xflowd migrate device-ids` CLI 통합 테스트.
//
// cobra Execute 를 통해 전체 흐름을 검증한다 (플래그 파싱 + Planner 호출 +
// 출력 + 종료 코드). 모든 IO 는 t.TempDir() / 메모리 버퍼로 격리한다.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 테스트용 metadata + id-repo fixture 헬퍼.
func setupMigrateFixture(
	t *testing.T,
	metadata map[string]map[string]interface{},
	ids map[string]string,
) (metadataDir, idRepoDir string) {
	t.Helper()
	root := t.TempDir()
	metadataDir = filepath.Join(root, "metadata")
	idRepoDir = filepath.Join(root, "ids")
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatalf("mkdir metadata: %v", err)
	}
	if err := os.MkdirAll(idRepoDir, 0o755); err != nil {
		t.Fatalf("mkdir ids: %v", err)
	}
	mdBytes, _ := json.MarshalIndent(metadata, "", "  ")
	idBytes, _ := json.MarshalIndent(ids, "", "  ")
	if err := os.WriteFile(filepath.Join(metadataDir, "device_metadata.json"), mdBytes, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(idRepoDir, "device_ids.json"), idBytes, 0o644); err != nil {
		t.Fatalf("write ids: %v", err)
	}
	return metadataDir, idRepoDir
}

// runCLI 는 cobra Execute 를 호출하고 stdout/stderr/error 를 반환한다.
func runCLI(t *testing.T, args []string, stdin string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newMigrateCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestMigrateCLI_DryRun_NoFileChange(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
		"lgcnp:82": {"name": "indoor-2"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)
	mdPath := filepath.Join(metadataDir, "device_metadata.json")
	originalBytes, _ := os.ReadFile(mdPath)

	stdout, _, err := runCLI(t, []string{
		"device-ids",
		"--metadata-dir", metadataDir,
		"--id-repo", idRepoDir,
		"--dry-run",
	}, "")
	if err != nil {
		t.Fatalf("CLI execute: %v", err)
	}
	if !strings.Contains(stdout, "convert=2") {
		t.Errorf("stdout 에 'convert=2' 없음: %s", stdout)
	}
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("stdout 에 [dry-run] 표시 없음: %s", stdout)
	}

	afterBytes, _ := os.ReadFile(mdPath)
	if !bytes.Equal(originalBytes, afterBytes) {
		t.Errorf("dry-run 후 파일이 변경됨")
	}
}

func TestMigrateCLI_StrictAbortsOnAmbiguous(t *testing.T) {
	t.Parallel()

	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	metadata := map[string]map[string]interface{}{
		"lgcnp:81":  {"name": "indoor-1"},
		"lgcnp:81b": {"name": "indoor-1-dup"},
	}
	ids := map[string]string{
		"lgcnp:81":  uid,
		"lgcnp:81b": uid, // 다대일 → ambiguous
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)

	_, _, err := runCLI(t, []string{
		"device-ids",
		"--metadata-dir", metadataDir,
		"--id-repo", idRepoDir,
		"--strict",
		"--yes",
	}, "")
	if err == nil {
		t.Errorf("strict 모드에서 ambiguous 발견 시 에러를 반환해야 함")
	}
	if err != nil && !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("에러 메시지에 'ambiguous' 포함되어야 함: %v", err)
	}

	// 파일이 변경되지 않아야 함 (abort 후 일관성).
	mdPath := filepath.Join(metadataDir, "device_metadata.json")
	after, _ := os.ReadFile(mdPath)
	if !strings.Contains(string(after), "lgcnp:81") {
		t.Errorf("strict abort 후 원본 composite key 가 제거됨")
	}
}

func TestMigrateCLI_NonStrictSkipsAmbiguous(t *testing.T) {
	t.Parallel()

	uid1 := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	uid2 := "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e"
	uidShared := "c7d6e88a-7963-4d5e-bf40-9f5eac3d4e5f"
	metadata := map[string]map[string]interface{}{
		"lgcnp:81":  {"name": "indoor-1"},
		"lgcnp:82":  {"name": "indoor-2"},
		"lgcnp:99a": {"name": "dup-a"},
		"lgcnp:99b": {"name": "dup-b"},
	}
	ids := map[string]string{
		"lgcnp:81":  uid1,
		"lgcnp:82":  uid2,
		"lgcnp:99a": uidShared,
		"lgcnp:99b": uidShared, // ambiguous
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)

	stdout, _, err := runCLI(t, []string{
		"device-ids",
		"--metadata-dir", metadataDir,
		"--id-repo", idRepoDir,
		"--yes",
	}, "")
	if err != nil {
		t.Fatalf("non-strict 모드 실행 실패: %v", err)
	}
	if !strings.Contains(stdout, "converted=2") {
		t.Errorf("converted=2 출력 없음: %s", stdout)
	}
	if !strings.Contains(stdout, "skipped=2") {
		t.Errorf("skipped=2 출력 없음: %s", stdout)
	}

	// ambiguous 키들은 원본 그대로 보존되어야 함.
	mdPath := filepath.Join(metadataDir, "device_metadata.json")
	data, _ := os.ReadFile(mdPath)
	after := map[string]interface{}{}
	_ = json.Unmarshal(data, &after)
	if _, ok := after["lgcnp:99a"]; !ok {
		t.Errorf("ambiguous key lgcnp:99a 가 보존되지 않음")
	}
	if _, ok := after["lgcnp:99b"]; !ok {
		t.Errorf("ambiguous key lgcnp:99b 가 보존되지 않음")
	}
	if _, ok := after[uid1]; !ok {
		t.Errorf("정상 변환 키 %s 가 없음", uid1)
	}
}

func TestMigrateCLI_RequiredFlags(t *testing.T) {
	t.Parallel()

	_, _, err := runCLI(t, []string{"device-ids"}, "")
	if err == nil {
		t.Errorf("필수 플래그 누락 시 에러를 반환해야 함")
	}
}

func TestMigrateCLI_InteractiveCancel(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)
	mdPath := filepath.Join(metadataDir, "device_metadata.json")
	originalBytes, _ := os.ReadFile(mdPath)

	// 사용자가 "n" 입력 → 취소.
	stdout, _, err := runCLI(t, []string{
		"device-ids",
		"--metadata-dir", metadataDir,
		"--id-repo", idRepoDir,
	}, "n\n")
	if err != nil {
		t.Fatalf("CLI execute: %v", err)
	}
	if !strings.Contains(stdout, "취소") {
		t.Errorf("stdout 에 취소 메시지 없음: %s", stdout)
	}

	afterBytes, _ := os.ReadFile(mdPath)
	if !bytes.Equal(originalBytes, afterBytes) {
		t.Errorf("취소 후 파일이 변경됨")
	}
}

func TestMigrateCLI_AssumeYes_AppliesImmediately(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
	}
	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	ids := map[string]string{
		"lgcnp:81": uid,
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)

	stdout, _, err := runCLI(t, []string{
		"device-ids",
		"--metadata-dir", metadataDir,
		"--id-repo", idRepoDir,
		"--yes",
	}, "")
	if err != nil {
		t.Fatalf("CLI execute: %v", err)
	}
	if !strings.Contains(stdout, "verified=true") {
		t.Errorf("verified=true 출력 없음: %s", stdout)
	}

	// 파일이 UUID key 로 변환되었는지 확인.
	mdPath := filepath.Join(metadataDir, "device_metadata.json")
	data, _ := os.ReadFile(mdPath)
	after := map[string]interface{}{}
	_ = json.Unmarshal(data, &after)
	if _, ok := after[uid]; !ok {
		t.Errorf("UUID key %s 가 변환 후 메타데이터에 없음", uid)
	}
	if _, ok := after["lgcnp:81"]; ok {
		t.Errorf("composite key lgcnp:81 가 변환 후에도 남아 있음")
	}
}

func TestMigrateCLI_IdempotentSecondRun(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
	}
	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	ids := map[string]string{
		"lgcnp:81": uid,
	}
	metadataDir, idRepoDir := setupMigrateFixture(t, metadata, ids)

	// 1차 실행.
	if _, _, err := runCLI(t, []string{
		"device-ids", "--metadata-dir", metadataDir, "--id-repo", idRepoDir, "--yes",
	}, ""); err != nil {
		t.Fatalf("1차 CLI: %v", err)
	}

	// 2차 실행 — 변환 대상 0.
	stdout, _, err := runCLI(t, []string{
		"device-ids", "--metadata-dir", metadataDir, "--id-repo", idRepoDir, "--yes",
	}, "")
	if err != nil {
		t.Fatalf("2차 CLI: %v", err)
	}
	if !strings.Contains(stdout, "convert=0") {
		t.Errorf("2차 실행 convert=0 출력 없음: %s", stdout)
	}
	if !strings.Contains(stdout, "idempotent no-op") {
		t.Errorf("2차 실행 idempotent no-op 메시지 없음: %s", stdout)
	}
}
