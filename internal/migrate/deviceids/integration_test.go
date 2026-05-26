// integration_test.go — round-trip, backup integrity, idempotency 시나리오.
//
// acceptance MIG-AC1 (round-trip metadata-by-metadata 동일성), MIG-AC4
// (idempotency), MIG-AC5 (백업 byte-perfect 복원 가능성) 의 자동 검증.
package deviceids

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestIntegration_MIG_AC1_RoundTripMetadata 는 변환 전/후 메타데이터의
// entry-by-entry 일치를 검증한다 (acceptance MIG-AC1).
//
// 가상 역방향 도구 대신 본 테스트는: composite → UUID 변환 후, UUID key 에서
// 각 value 를 꺼내 원본 composite key 의 value 와 reflect.DeepEqual 비교한다.
// value 가 json.RawMessage 로 byte-perfect 보존되므로 메타데이터의 모든
// 필드 (tags / location / group / labels / pinned 등) 가 보존되어야 한다.
func TestIntegration_MIG_AC1_RoundTripMetadata(t *testing.T) {
	t.Parallel()

	// 다양한 메타데이터 형식을 포함하는 fixture.
	pinned := true
	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {
			"name":     "indoor-1",
			"tags":     []interface{}{"hvac", "indoor"},
			"location": "Building A, Floor 2",
			"group":    "floor-2",
			"labels":   map[string]interface{}{"zone": "north", "vlan": "100"},
			"pinned":   pinned,
		},
		"lgcnp:82": {
			"name":     "indoor-2",
			"tags":     []interface{}{},
			"location": "",
			"group":    "",
			"labels":   map[string]interface{}{},
		},
		"samsung:0.0.16": {
			"name":     "outdoor",
			"location": "Roof",
		},
	}
	ids := map[string]string{
		"lgcnp:81":       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82":       "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"samsung:0.0.16": "c7d6e88a-7963-4d5e-bf40-9f5eac3d4e5f",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())
	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Verified {
		t.Fatalf("verified = false")
	}

	// 변환 후 메타데이터 읽기.
	after := readJSONFile(t, filepath.Join(metadataDir, metadataFileName))

	// 각 composite → UUID 매핑마다 원본 value 와 변환 후 value 가 일치하는지.
	for composite, uid := range ids {
		original := metadata[composite]
		converted, ok := after[uid]
		if !ok {
			t.Errorf("UUID key %s 가 변환 후 메타데이터에 없음", uid)
			continue
		}
		if !reflect.DeepEqual(original, converted) {
			t.Errorf("round-trip 불일치 (composite=%s, uuid=%s):\n  orig=%+v\n  conv=%+v",
				composite, uid, original, converted)
		}
	}

	// entry 수 일치.
	if len(after) != len(metadata) {
		t.Errorf("entry count 변화: before=%d after=%d", len(metadata), len(after))
	}
}

// TestIntegration_MIG_AC5_BackupRestorable 는 백업 파일이 byte-perfect 로
// 원본을 보존하여 운영자가 cp 만으로 복원 가능한지 검증한다.
func TestIntegration_MIG_AC5_BackupRestorable(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1", "location": "Floor 2"},
		"lgcnp:82": {"name": "indoor-2", "location": "Floor 3"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)
	mdPath := filepath.Join(metadataDir, metadataFileName)

	originalBytes, _ := os.ReadFile(mdPath)
	originalHash := sha256.Sum256(originalBytes)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())
	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// 백업 파일이 원본과 byte-perfect 동일.
	backupFile := filepath.Join(result.BackupPath, metadataFileName)
	backupBytes, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(originalBytes, backupBytes) {
		t.Errorf("backup byte mismatch")
	}
	backupHash := sha256.Sum256(backupBytes)
	if backupHash != originalHash {
		t.Errorf("backup sha256 mismatch: original=%x backup=%x", originalHash, backupHash)
	}

	// 시뮬레이션 복원: 백업 → 원본 위치로 복사.
	if err := os.WriteFile(mdPath, backupBytes, 0o644); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restoredBytes, _ := os.ReadFile(mdPath)
	restoredHash := sha256.Sum256(restoredBytes)
	if restoredHash != originalHash {
		t.Errorf("restored sha256 mismatch after manual copy")
	}
}

// TestIntegration_Manifest_HashStableAcrossKeyChange 는 manifest 의
// value sha256 합산이 key 변경 (composite → UUID) 에 영향받지 않는지 검증한다.
func TestIntegration_Manifest_HashStableAcrossKeyChange(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
		"lgcnp:82": {"name": "indoor-2"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())
	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Verified {
		t.Fatalf("verified = false (key 변경 후 hash 합산이 불일치 — 회귀)")
	}

	// manifest 직접 확인.
	manifestPath := filepath.Join(result.BackupPath, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if m.EntryCount != 2 {
		t.Errorf("manifest entry count = %d, want 2", m.EntryCount)
	}
	if len(m.ValueSHA256ByKey) != 2 {
		t.Errorf("manifest per-key hash count = %d, want 2", len(m.ValueSHA256ByKey))
	}
	if m.SourceSHA256 == "" {
		t.Errorf("manifest source sha256 empty")
	}
}

// TestIntegration_ResumeAfterPartialFailure 는 부분 실패 후 재시작이
// 안전한지 검증한다. (백업이 이미 존재하면 abort 하므로 운영자가 명시적
// backup-dir 을 지정하지 않는 한 2회 실행은 새 백업 디렉토리를 만든다.)
//
// 시나리오: 1차 마이그레이션이 성공한 상태에서 2차 마이그레이션을 실행하면
// 변환 대상이 없어 no-op 이 되어야 한다 (idempotency).
func TestIntegration_ResumeAfterPartialFailure(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "indoor-1"},
		"lgcnp:82": {"name": "indoor-2"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	// 1차.
	planner1, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan1, _ := planner1.Plan(context.Background())
	res1, err := planner1.Apply(context.Background(), plan1)
	if err != nil {
		t.Fatalf("1차 Apply: %v", err)
	}
	if res1.Converted != 2 {
		t.Errorf("1차 converted = %d, want 2", res1.Converted)
	}

	// 2차 — 이미 UUID 명명되었으므로 convert 대상 0.
	planner2, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan2, _ := planner2.Plan(context.Background())
	if plan2.ConvertCount() != 0 {
		t.Errorf("2차 convert count = %d, want 0 (idempotent)", plan2.ConvertCount())
	}
	if plan2.AlreadyUUIDCount() != 2 {
		t.Errorf("2차 alreadyUUID count = %d, want 2", plan2.AlreadyUUIDCount())
	}
}

// TestIntegration_MixedMetadata_OrphanAndConvert 는 일부는 변환되고 일부는
// orphan (skip) 되는 혼합 시나리오를 검증한다.
func TestIntegration_MixedMetadata_OrphanAndConvert(t *testing.T) {
	t.Parallel()

	metadata := map[string]map[string]interface{}{
		"lgcnp:81": {"name": "mapped-1"},
		"lgcnp:82": {"name": "mapped-2"},
		"orphan:1": {"name": "no-mapping-1"},
		"orphan:2": {"name": "no-mapping-2"},
	}
	ids := map[string]string{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())

	if plan.ConvertCount() != 2 {
		t.Errorf("convert count = %d, want 2", plan.ConvertCount())
	}
	if plan.OrphanCount() != 2 {
		t.Errorf("orphan count = %d, want 2", plan.OrphanCount())
	}

	result, err := planner.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Verified {
		t.Errorf("verified = false")
	}

	// 변환 후 — 매핑된 2개 UUID + orphan 2개 composite 그대로 = 4 entries.
	after := readJSONFile(t, filepath.Join(metadataDir, metadataFileName))
	if len(after) != 4 {
		t.Errorf("after entry count = %d, want 4 (2 converted + 2 orphan)", len(after))
	}
	if _, ok := after["orphan:1"]; !ok {
		t.Errorf("orphan:1 보존 안됨")
	}
	if _, ok := after["a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"]; !ok {
		t.Errorf("매핑된 UUID 가 없음")
	}
}

// TestIntegration_Result_Print 는 Result.Print 가 모든 필드를 포함하는지 검증한다.
func TestIntegration_Result_Print(t *testing.T) {
	t.Parallel()
	r := &Result{
		Verified:   true,
		Converted:  5,
		Skipped:    2,
		BackupPath: "/tmp/backup-xyz",
	}
	var buf bytes.Buffer
	if err := r.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	out := buf.String()
	for _, expected := range []string{"converted=5", "skipped=2", "verified=true", "backup=/tmp/backup-xyz"} {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Result.Print 출력에 %q 없음:\n%s", expected, out)
		}
	}
}

// TestIntegration_NewPlanner_Validation 은 NewPlanner 의 필수 필드 검증을 확인한다.
func TestIntegration_NewPlanner_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "metadata-dir 누락",
			opts:    Options{IDRepoDir: "/tmp/ids"},
			wantErr: true,
		},
		{
			name:    "id-repo 누락",
			opts:    Options{MetadataDir: "/tmp/md"},
			wantErr: true,
		},
		{
			name:    "둘 다 지정",
			opts:    Options{MetadataDir: "/tmp/md", IDRepoDir: "/tmp/ids"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewPlanner(context.Background(), tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestIntegration_Apply_WithoutPlan_Fails 는 Plan() 호출 없이 Apply() 가
// 호출되면 에러를 반환하는지 검증한다.
func TestIntegration_Apply_WithoutPlan_Fails(t *testing.T) {
	t.Parallel()

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: t.TempDir(),
		IDRepoDir:   t.TempDir(),
	})
	_, err := planner.Apply(context.Background(), nil)
	if err == nil {
		t.Errorf("Plan() 없이 Apply() 호출 시 에러를 반환해야 함")
	}
}

// TestIntegration_HelperAccessors 는 Plan 의 단순 accessor 들을 검증한다.
func TestIntegration_HelperAccessors(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		convert: []ConvertEntry{
			{Composite: "lgcnp:81", UUID: "uid-1"},
			{Composite: "lgcnp:82", UUID: "uid-2"},
		},
		ambiguous: []AmbiguousEntry{
			{Composite: "lgcnp:99", UUID: "uid-x", Conflicts: []string{"lgcp:1"}},
		},
	}
	if got := plan.ConvertCount(); got != 2 {
		t.Errorf("ConvertCount = %d, want 2", got)
	}
	if !plan.HasAmbiguous() {
		t.Errorf("HasAmbiguous = false, want true")
	}
	conv := plan.Convert()
	if len(conv) != 2 {
		t.Errorf("Convert() len = %d, want 2", len(conv))
	}
	// 슬라이스 변경이 내부 상태에 영향 없어야 함 (defensive copy).
	conv[0].Composite = "MUTATED"
	if plan.convert[0].Composite != "lgcnp:81" {
		t.Errorf("Convert() 가 내부 슬라이스를 노출함 — defensive copy 실패")
	}

	emptyPlan := &Plan{}
	if emptyPlan.HasAmbiguous() {
		t.Errorf("빈 plan 의 HasAmbiguous 는 false 여야 함")
	}
}

// TestIntegration_PrintToFailingWriter 는 Plan.Print 가 io.Writer 에러를 전파하는지 검증.
func TestIntegration_PrintToFailingWriter(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		convert:     []ConvertEntry{{Composite: "lgcnp:81", UUID: "uid-1"}},
		ambiguous:   []AmbiguousEntry{{Composite: "lgcnp:99", UUID: "uid-x"}},
		orphan:      []string{"orphan:1"},
		alreadyUUID: []string{"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"},
	}
	// 0 byte 만 쓰고 에러 반환하는 writer.
	fw := &failingWriter{}
	if err := plan.Print(fw); err == nil {
		t.Errorf("실패하는 writer 에 대해 에러를 반환해야 함")
	}
}

type failingWriter struct{}

func (fw *failingWriter) Write(_ []byte) (int, error) {
	return 0, &writeFailErr{}
}

type writeFailErr struct{}

func (e *writeFailErr) Error() string { return "intentional write fail" }

// TestIntegration_LoadMetadata_CorruptJSON 은 손상된 JSON 파일 처리.
func TestIntegration_LoadMetadata_CorruptJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	metadataDir := filepath.Join(root, "metadata")
	idRepoDir := filepath.Join(root, "ids")
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, metadataFileName), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	_, err := planner.Plan(context.Background())
	if err == nil {
		t.Errorf("손상된 JSON 에 대해 에러를 반환해야 함")
	}
}

// TestIntegration_PlanOutput_ContainsExpectedSections 는 Plan.Print 출력의
// 형식이 운영자가 읽기 좋은 섹션 구조를 가지는지 검증한다.
func TestIntegration_PlanOutput_ContainsExpectedSections(t *testing.T) {
	t.Parallel()

	uidShared := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	metadata := map[string]map[string]interface{}{
		"lgcnp:81":  {"name": "to-convert"},
		"lgcnp:99a": {"name": "ambiguous-a"},
		"lgcnp:99b": {"name": "ambiguous-b"},
		"orphan:1":  {"name": "no-mapping"},
	}
	ids := map[string]string{
		"lgcnp:81":  "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"lgcnp:99a": uidShared,
		"lgcnp:99b": uidShared, // ambiguous (다대일)
	}
	metadataDir, idRepoDir := setupFixture(t, metadata, ids)

	planner, _ := NewPlanner(context.Background(), Options{
		MetadataDir: metadataDir,
		IDRepoDir:   idRepoDir,
	})
	plan, _ := planner.Plan(context.Background())

	var buf bytes.Buffer
	if err := plan.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	out := buf.String()
	for _, expected := range []string{
		"Plan summary:",
		"convert=1",
		"ambiguous=2",
		"orphan=1",
		"[convert]",
		"[ambiguous]",
		"[orphan]",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Plan output 에 %q 가 없음:\n%s", expected, out)
		}
	}
}
