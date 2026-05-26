// integration_test.go — Planner Plan/Apply 통합 시나리오 + idempotency.
//
// 실제 Influx 인스턴스 접근 없이 fixture-driven mock 으로 전체 흐름 검증
// (acceptance C-AC6 / C-AC7 / MIG-AC2 의 mock 버전).
package tsdbtags

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPlannerFixture 는 mock client + device-ids.json fixture 로 Planner 를 구성한다.
func setupPlannerFixture(t *testing.T, target Target) (*Planner, string) {
	t.Helper()
	outDir := filepath.Join(t.TempDir(), "out")
	idRepoPath := filepath.Join("testdata", "device-ids.json")

	mc, err := NewMockClientFromFile(filepath.Join("testdata",
		map[Target]string{TargetV2: "v2-schema.json", TargetV3: "v3-schema.json"}[target]))
	if err != nil {
		t.Fatalf("mock client: %v", err)
	}

	opts := Options{
		InfluxURL:   "http://mock:8086",
		InfluxToken: "mock-token",
		Bucket:      "xflow",
		Org:         "acme",
		IDRepoPath:  idRepoPath,
		OutputDir:   outDir,
		Target:      target,
	}
	p, err := NewPlanner(context.Background(), opts, mc)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	return p, outDir
}

func TestIntegration_PlanApply_V2(t *testing.T) {
	t.Parallel()

	p, outDir := setupPlannerFixture(t, TargetV2)

	plan, err := p.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// fixture: lgcnp:81 / lgcnp:82 / century:7 → mapped (3)
	//          samsung:0.0.16 → mapped (1) -- 매핑 존재
	//          lgcnp:99a / lgcnp:99b → ambiguous (2)
	//          a58ba668-... → uuid already (1)
	if plan.MappedCount() != 4 {
		t.Errorf("MappedCount = %d, want 4 (lgcnp:81/82, century:7, samsung:0.0.16): %+v", plan.MappedCount(), plan.result.Mapped)
	}
	if plan.AmbiguousCount() != 2 {
		t.Errorf("AmbiguousCount = %d, want 2", plan.AmbiguousCount())
	}
	if plan.UUIDAlreadyCount() != 1 {
		t.Errorf("UUIDAlreadyCount = %d, want 1", plan.UUIDAlreadyCount())
	}

	// Apply.
	result, err := p.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.MappedCount != 4 {
		t.Errorf("Result.MappedCount = %d, want 4", result.MappedCount)
	}
	if result.OutputDir != outDir {
		t.Errorf("Result.OutputDir 불일치: got=%s want=%s", result.OutputDir, outDir)
	}

	// 파일 생성 확인.
	flux := filepath.Join(outDir, "migration-v2.flux")
	if _, err := os.Stat(flux); err != nil {
		t.Errorf("migration-v2.flux 가 생성되지 않음: %v", err)
	}
	readme := filepath.Join(outDir, "RUN.md")
	if _, err := os.Stat(readme); err != nil {
		t.Errorf("RUN.md 가 생성되지 않음: %v", err)
	}

	// 스크립트 내용 검증.
	data, _ := os.ReadFile(flux)
	body := string(data)
	if !strings.Contains(body, "lgcnp:81") {
		t.Errorf("flux 에 lgcnp:81 매핑 없음:\n%s", body)
	}
	if !strings.Contains(body, "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d") {
		t.Errorf("flux 에 매핑된 UUID 없음")
	}
	// ambiguous 는 스크립트에 포함되지 않아야 함.
	if strings.Contains(body, "lgcnp:99a") {
		t.Errorf("flux 에 ambiguous key lgcnp:99a 가 포함됨 (skip 되어야 함)")
	}
}

func TestIntegration_PlanApply_V3(t *testing.T) {
	t.Parallel()

	p, outDir := setupPlannerFixture(t, TargetV3)

	plan, err := p.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Target() != TargetV3 {
		t.Errorf("plan.Target = %v, want v3", plan.Target())
	}

	result, err := p.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	sql := filepath.Join(outDir, "migration-v3.sql")
	if _, err := os.Stat(sql); err != nil {
		t.Errorf("migration-v3.sql 가 생성되지 않음: %v", err)
	}
	data, _ := os.ReadFile(sql)
	if !strings.Contains(string(data), "SELECT * FROM") {
		t.Errorf("SQL 스크립트에 SELECT 없음:\n%s", string(data))
	}
	if result.OutputDir != outDir {
		t.Errorf("OutputDir 불일치")
	}
}

func TestIntegration_DryRun_NoFileWrite(t *testing.T) {
	t.Parallel()

	p, outDir := setupPlannerFixture(t, TargetV2)
	p.opts.DryRun = true

	plan, err := p.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	result, err := p.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.DryRun {
		t.Errorf("Result.DryRun = false, want true")
	}
	// outDir 가 생성되지 않아야 함.
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("dry-run 후에도 output-dir 가 생성됨: %v", err)
	}
}

func TestIntegration_NoMapped_NoFileWrite(t *testing.T) {
	t.Parallel()

	// 모든 schema tag value 가 UUID 이거나 일반 tag — mapped 없음.
	emptyMappingPath := filepath.Join(t.TempDir(), "device_ids.json")
	if err := os.WriteFile(emptyMappingPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write empty ids: %v", err)
	}

	mc, _ := NewMockClientFromFile(filepath.Join("testdata", "v2-schema.json"))
	opts := Options{
		InfluxURL:   "http://mock:8086",
		InfluxToken: "tok",
		Bucket:      "xflow",
		Org:         "acme",
		IDRepoPath:  emptyMappingPath,
		OutputDir:   filepath.Join(t.TempDir(), "out"),
		Target:      TargetV2,
	}
	p, _ := NewPlanner(context.Background(), opts, mc)

	plan, _ := p.Plan(context.Background())
	if plan.MappedCount() != 0 {
		t.Errorf("빈 매핑에서 MappedCount = %d, want 0", plan.MappedCount())
	}
	result, err := p.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.OutputDir != "" {
		t.Errorf("매핑 없는 경우 OutputDir 가 비어야 함: %q", result.OutputDir)
	}
}

func TestIntegration_Idempotency_SameInputSameOutput(t *testing.T) {
	t.Parallel()

	// 동일 fixture 로 두 번 Plan 수행 시 결과가 동일해야 한다.
	p1, _ := setupPlannerFixture(t, TargetV2)
	p2, _ := setupPlannerFixture(t, TargetV2)

	plan1, _ := p1.Plan(context.Background())
	plan2, _ := p2.Plan(context.Background())

	if plan1.MappedCount() != plan2.MappedCount() ||
		plan1.AmbiguousCount() != plan2.AmbiguousCount() ||
		plan1.OrphanCount() != plan2.OrphanCount() {
		t.Errorf("동일 입력 두 번에서 다른 결과:\n  1=%+v\n  2=%+v", plan1.result, plan2.result)
	}

	// Plan.Print 출력도 byte-perfect 동일해야 함.
	var buf1, buf2 bytes.Buffer
	_ = plan1.Print(&buf1)
	_ = plan2.Print(&buf2)
	if buf1.String() != buf2.String() {
		t.Errorf("Plan.Print 비결정적:\n  1=%q\n  2=%q", buf1.String(), buf2.String())
	}
}

func TestIntegration_PingFailure(t *testing.T) {
	t.Parallel()

	mc := NewMockClient(MockSchema{
		Version: TargetV2,
		PingErr: "connection refused",
	})
	opts := Options{
		InfluxURL:   "http://mock:8086",
		InfluxToken: "tok",
		Bucket:      "xflow",
		Org:         "acme",
		Target:      TargetV2,
	}
	p, _ := NewPlanner(context.Background(), opts, mc)

	_, err := p.Plan(context.Background())
	if err == nil {
		t.Errorf("Ping 실패 시 Plan 이 에러를 반환해야 함")
	}
	if !strings.Contains(err.Error(), "Influx 연결 실패") {
		t.Errorf("에러 메시지에 '연결 실패' 포함되어야 함: %v", err)
	}
}

func TestIntegration_OutputDirAlreadyExists(t *testing.T) {
	t.Parallel()

	p, outDir := setupPlannerFixture(t, TargetV2)
	// 미리 디렉토리 생성 — Apply 가 abort 해야 한다.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	plan, _ := p.Plan(context.Background())
	_, err := p.Apply(context.Background(), plan)
	if err == nil {
		t.Errorf("output-dir 이미 존재 시 Apply 가 에러를 반환해야 함")
	}
	if !strings.Contains(err.Error(), "이미 존재") {
		t.Errorf("에러 메시지에 '이미 존재' 포함되어야 함: %v", err)
	}
}

func TestIntegration_MeasurementRestrict(t *testing.T) {
	t.Parallel()

	mc, _ := NewMockClientFromFile(filepath.Join("testdata", "v2-schema.json"))
	opts := Options{
		InfluxURL:    "http://mock:8086",
		InfluxToken:  "tok",
		Bucket:       "xflow",
		Org:          "acme",
		IDRepoPath:   filepath.Join("testdata", "device-ids.json"),
		OutputDir:    filepath.Join(t.TempDir(), "out"),
		Target:       TargetV2,
		Measurements: []string{"hvac_status"}, // hvac_status 만 처리.
	}
	p, _ := NewPlanner(context.Background(), opts, mc)
	plan, _ := p.Plan(context.Background())

	// hvac_status 의 lgcnp:99a/99b 는 ambiguous, a58ba668-... 은 uuid already
	// → mapped 는 0 이어야 함.
	if plan.MappedCount() != 0 {
		t.Errorf("hvac_status 만 restrict 시 mapped=%d, want 0: %+v", plan.MappedCount(), plan.result.Mapped)
	}
	if plan.AmbiguousCount() != 2 {
		t.Errorf("hvac_status 만 restrict 시 ambiguous=%d, want 2", plan.AmbiguousCount())
	}
}
