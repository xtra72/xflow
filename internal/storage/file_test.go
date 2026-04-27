package storage

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/xtra/xflow/pkg/flow"
)

// newTestFlowFromYAML 은 YAML 데이터로부터 테스트용 Flow를 생성한다.
func newTestFlowFromYAML(t *testing.T, yamlData string) flow.Flow {
	t.Helper()
	f, err := flow.FlowFromYAML([]byte(yamlData))
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}
	return f
}

// testFlowYAML 은 테스트용 YAML 플로우 정의를 반환한다.
func testFlowYAML(name string) string {
	return `name: "` + name + `"
nodes:
  - name: "source"
    type: "generator"
    inputs:
      - name: "in"
    outputs:
      - name: "out"
edges:
  - from: "source:out"
    to: "source:in"
`
}

func TestFileRepository_Save_Get(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 테스트 플로우 생성
	f := newTestFlowFromYAML(t, testFlowYAML("test-flow"))

	// Save
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save 실패: %v", err)
	}

	// Get
	got, err := repo.Get(ctx, f.ID())
	if err != nil {
		t.Fatalf("Get 실패: %v", err)
	}

	// 필드 검증
	if got.Name() != f.Name() {
		t.Errorf("Name: got %q, want %q", got.Name(), f.Name())
	}
	if got.ID() != f.ID() {
		t.Errorf("ID: got %q, want %q", got.ID(), f.ID())
	}
	if len(got.Nodes()) != len(f.Nodes()) {
		t.Errorf("Nodes count: got %d, want %d", len(got.Nodes()), len(f.Nodes()))
	}
	if len(got.Wires()) != len(f.Wires()) {
		t.Errorf("Wires count: got %d, want %d", len(got.Wires()), len(f.Wires()))
	}
}

func TestFileRepository_Save_Overwrite(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 플로우 생성 및 저장
	f := newTestFlowFromYAML(t, testFlowYAML("original-flow"))
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("첫 번째 Save 실패: %v", err)
	}

	// 동일 ID 로 설명 변경 후 다시 저장
	f.SetDescription("updated description")
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("두 번째 Save 실패: %v", err)
	}

	// 덮어쓰기 확인
	got, err := repo.Get(ctx, f.ID())
	if err != nil {
		t.Fatalf("Get 실패: %v", err)
	}
	if got.Description() != "updated description" {
		t.Errorf("Description: got %q, want %q", got.Description(), "updated description")
	}
}

func TestFileRepository_List(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 여러 플로우 저장
	f1 := newTestFlowFromYAML(t, testFlowYAML("flow-1"))
	f2 := newTestFlowFromYAML(t, testFlowYAML("flow-2"))
	f3 := newTestFlowFromYAML(t, testFlowYAML("flow-3"))

	for _, f := range []flow.Flow{f1, f2, f3} {
		if err := repo.Save(ctx, f); err != nil {
			t.Fatalf("Save 실패: %v", err)
		}
	}

	// List
	flows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}

	if len(flows) != 3 {
		t.Errorf("List count: got %d, want 3", len(flows))
	}

	// 모든 플로우의 이름이 포함되어 있는지 확인
	names := make(map[string]bool)
	for _, f := range flows {
		names[f.Name()] = true
	}
	for _, expected := range []string{"flow-1", "flow-2", "flow-3"} {
		if !names[expected] {
			t.Errorf("List 결과에 %q 가 포함되지 않음", expected)
		}
	}
}

func TestFileRepository_List_Empty(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	flows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 실패: %v", err)
	}

	if len(flows) != 0 {
		t.Errorf("빈 저장소 List count: got %d, want 0", len(flows))
	}
}

func TestFileRepository_Delete(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 플로우 저장
	f := newTestFlowFromYAML(t, testFlowYAML("delete-me"))
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save 실패: %v", err)
	}

	// Delete
	if err := repo.Delete(ctx, f.ID()); err != nil {
		t.Fatalf("Delete 실패: %v", err)
	}

	// Get 으로 삭제 확인
	_, err = repo.Get(ctx, f.ID())
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("삭제 후 Get: got err=%v, want ErrFlowNotFound", err)
	}
}

func TestFileRepository_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	_, err = repo.Get(ctx, "non-existent-id")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("존재하지 않는 플로우 Get: got err=%v, want ErrFlowNotFound", err)
	}
}

func TestFileRepository_Delete_NotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	err = repo.Delete(ctx, "non-existent-id")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("존재하지 않는 플로우 Delete: got err=%v, want ErrFlowNotFound", err)
	}
}

func TestFileRepository_Close(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileRepository(dir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}

	if err := repo.Close(); err != nil {
		t.Errorf("Close: got err=%v, want nil", err)
	}
}

func TestNewFileRepository_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := dir + "/nested/subdir"

	repo, err := NewFileRepository(subdir)
	if err != nil {
		t.Fatalf("NewFileRepository 실패: %v", err)
	}
	defer repo.Close()

	// 디렉토리가 생성되었는지 확인
	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("디렉토리 확인 실패: %v", err)
	}
	if !info.IsDir() {
		t.Error("생성된 경로가 디렉토리가 아님")
	}
}
