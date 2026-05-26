package deviceids

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeJSONFile 는 테스트용 helper — JSON 직렬화 후 파일에 기록.
func writeJSONFile(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// readJSONFile 는 테스트용 helper — 파일 JSON 을 map 으로 디코드.
func readJSONFile(t *testing.T, path string) map[string]map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	m := map[string]map[string]interface{}{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// setupFixture 는 테스트용 metadata + id-repo 파일을 t.TempDir() 에 생성한다.
//
// 반환값: (metadataDir, idRepoDir).
func setupFixture(
	t *testing.T,
	metadata map[string]map[string]interface{},
	ids map[string]string,
) (string, string) {
	t.Helper()
	root := t.TempDir()
	metadataDir := filepath.Join(root, "metadata")
	idRepoDir := filepath.Join(root, "ids")
	writeJSONFile(t, filepath.Join(metadataDir, metadataFileName), metadata)
	writeJSONFile(t, filepath.Join(idRepoDir, idRepoFileName), ids)
	return metadataDir, idRepoDir
}

func TestPlanner_PlanDryRun_NoChange(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81":       {"name": "indoor-1", "location": "Floor 2"},
		"lgcnp:82":       {"name": "indoor-2", "location": "Floor 3"},
		"samsung:0.0.16": {"name": "outdoor-1", "location": "Roof"},
	}
	ids := map[string]string{
		"lgcnp:81":       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82":       "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"samsung:0.0.16": "c7d6e88a-7963-4d5e-bf40-9f5eac3d4e5f",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	originalBytes, err := os.ReadFile(filepath.Join(metadataDir, metadataFileName))
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	var stdout, stderr bytes.Buffer
	planner, err := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
		DryRun:      true,
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	plan, err := planner.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ConvertCount() != 3 {
		t.Errorf("convert count = %d, want 3", plan.ConvertCount())
	}

	// dry-run 시 Apply 호출 안함. 파일이 변경되지 않았는지 검증.
	afterBytes, err := os.ReadFile(filepath.Join(metadataDir, metadataFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(originalBytes, afterBytes) {
		t.Errorf("dry-run 후 metadata 파일이 변경됨 (byte mismatch)")
	}
}

func TestPlanner_Apply_HappyPath(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1", "location": "Floor 2", "group": "f2"},
		"lgcnp:82": {"name": "indoor-2", "location": "Floor 3"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, err := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	plan, err := planner.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Verified {
		t.Errorf("Verified = false, want true")
	}
	if result.Converted != 2 {
		t.Errorf("Converted = %d, want 2", result.Converted)
	}
	if result.BackupPath == "" {
		t.Errorf("BackupPath empty")
	}

	// 변환 후 메타데이터 검증 — UUID key 만 존재해야 함.
	after := readJSONFile(t, filepath.Join(metadataDir, metadataFileName))
	if len(after) != 2 {
		t.Errorf("after entry count = %d, want 2", len(after))
	}
	for _, uid := range ids {
		if _, ok := after[uid]; !ok {
			t.Errorf("UUID key %q 가 변환 후 메타데이터에 없음", uid)
		}
	}
	for composite := range ids {
		if _, ok := after[composite]; ok {
			t.Errorf("composite key %q 가 변환 후에도 남아 있음", composite)
		}
	}

	// 백업 디렉토리에 원본이 보존되어야 함.
	backupFile := filepath.Join(result.BackupPath, metadataFileName)
	beforeFromBackup := readJSONFile(t, backupFile)
	if len(beforeFromBackup) != 2 {
		t.Errorf("backup entry count = %d, want 2", len(beforeFromBackup))
	}
	for composite := range ids {
		if _, ok := beforeFromBackup[composite]; !ok {
			t.Errorf("백업에 원본 composite key %q 가 없음", composite)
		}
	}

	// manifest 도 존재해야 함.
	manifestPath := filepath.Join(result.BackupPath, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("manifest 부재: %v", err)
	}
}

func TestPlanner_Apply_PreservesMetadataValues(t *testing.T) {
	t.Parallel()

	// 복합 메타데이터로 byte-perfect value 보존 검증.
	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {
			"name":     "indoor-1",
			"tags":     []interface{}{"hvac", "indoor"},
			"location": "Building A, Floor 2",
			"group":    "floor-2",
			"labels":   map[string]interface{}{"zone": "north", "vlan": "100"},
			"pinned":   true,
		},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, err := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	plan, _ := planner.Plan(context.Background())
	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Verified {
		t.Fatalf("verified = false")
	}

	after := readJSONFile(t, filepath.Join(metadataDir, metadataFileName))
	originalValue := metadata["lgcnp:81"]
	convertedValue := after["a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"]

	if !reflect.DeepEqual(originalValue, convertedValue) {
		t.Errorf("value mismatch after conversion:\n  orig=%+v\n  conv=%+v", originalValue, convertedValue)
	}
}

func TestPlanner_Apply_Idempotency_SecondRunNoConvert(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	// 1차 마이그레이션.
	planner1, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan1, _ := planner1.Plan(context.Background())
	if plan1.ConvertCount() != 1 {
		t.Fatalf("1차 convert count = %d, want 1", plan1.ConvertCount())
	}
	if _, err := planner1.Apply(context.Background(), plan1); err != nil {
		t.Fatalf("1차 Apply: %v", err)
	}

	// 2차 마이그레이션 — 변환 대상 0 이어야 함.
	planner2, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan2, err := planner2.Plan(context.Background())
	if err != nil {
		t.Fatalf("2차 Plan: %v", err)
	}
	if plan2.ConvertCount() != 0 {
		t.Errorf("2차 convert count = %d, want 0 (idempotent)", plan2.ConvertCount())
	}
	if plan2.AlreadyUUIDCount() != 1 {
		t.Errorf("2차 alreadyUUID count = %d, want 1", plan2.AlreadyUUIDCount())
	}
}

func TestPlanner_Apply_OrphanSkipNonStrict(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81":  {"name": "indoor-1"},
		"orphan:xx": {"name": "no-mapping"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())
	if plan.OrphanCount() != 1 {
		t.Errorf("orphan count = %d, want 1", plan.OrphanCount())
	}
	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply (non-strict): %v", err)
	}
	if !result.Verified {
		t.Errorf("verified = false")
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}

	// orphan 은 원본 key 로 보존되어야 함.
	after := readJSONFile(t, filepath.Join(metadataDir, metadataFileName))
	if _, ok := after["orphan:xx"]; !ok {
		t.Errorf("orphan key 가 보존되지 않음 (non-strict 정책 위반)")
	}
}

func TestPlanner_EmptyMetadata_NoOp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadataDir := filepath.Join(root, "metadata")
	idRepoDir := filepath.Join(root, "ids")
	// 파일을 생성하지 않음 — 신규 환경 시뮬레이션.

	planner, err := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	plan, err := planner.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan (empty env): %v", err)
	}
	if plan.ConvertCount() != 0 {
		t.Errorf("convert count = %d, want 0 (empty env)", plan.ConvertCount())
	}
}

func TestPlanner_BackupDir_AlreadyExists_Fails(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)
	existingBackup := filepath.Join(t.TempDir(), "preexisting-backup")
	if err := os.MkdirAll(existingBackup, 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
		BackupDir:   existingBackup,
	})
	plan, _ := planner.Plan(context.Background())
	_, err := planner.Apply(context.Background(), plan)
	if err == nil {
		t.Errorf("이미 존재하는 backup-dir 에 대해 에러가 발생해야 함")
	}
}
