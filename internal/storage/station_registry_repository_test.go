package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStationRegistryFileRepository_SaveAndGet(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("NewStationRegistryFileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	entry := StationRegistryEntry{Station: "ST-101", Line: "line-2", DisplayName: "강남", Order: 5}
	if err := repo.Save(ctx, "ST-101", entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.Get(ctx, "ST-101")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, entry) {
		t.Fatalf("Get mismatch: got %+v want %+v", got, entry)
	}
}

func TestStationRegistryFileRepository_GetMissingReturnsZero(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("NewStationRegistryFileRepository: %v", err)
	}
	defer repo.Close()

	got, err := repo.Get(context.Background(), "absent")
	if err != nil {
		t.Fatalf("Get returned error for missing key: %v", err)
	}
	if !reflect.DeepEqual(got, StationRegistryEntry{}) {
		t.Fatalf("expected zero entry for missing key, got %+v", got)
	}
}

// TestStationRegistryFileRepository_AtomicReload 는 단일 JSON 파일 + atomic write 로
// 저장된 항목이 새 인스턴스(재시작 시뮬레이션)에서 복원됨을 검증한다.
func TestStationRegistryFileRepository_AtomicReload(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	repo1, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("NewStationRegistryFileRepository: %v", err)
	}
	e1 := StationRegistryEntry{Station: "ST-101", Line: "line-2", DisplayName: "강남", Order: 5}
	e2 := StationRegistryEntry{Station: "ST-102", Line: "line-2", DisplayName: "역삼", Order: 6}
	if err := repo1.Save(ctx, "ST-101", e1); err != nil {
		t.Fatalf("Save e1: %v", err)
	}
	if err := repo1.Save(ctx, "ST-102", e2); err != nil {
		t.Fatalf("Save e2: %v", err)
	}
	_ = repo1.Close()

	// 파일이 실제로 생성되었는지 확인 (station_registry.json).
	if _, err := os.Stat(filepath.Join(dir, "station_registry.json")); err != nil {
		t.Fatalf("station_registry.json not created: %v", err)
	}

	// 새 인스턴스로 재로드.
	repo2, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()

	all, err := repo2.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries after reload, got %d", len(all))
	}
	if !reflect.DeepEqual(all["ST-101"], e1) || !reflect.DeepEqual(all["ST-102"], e2) {
		t.Fatalf("reloaded entries mismatch: %+v", all)
	}
}

func TestStationRegistryFileRepository_Delete(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	repo, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("NewStationRegistryFileRepository: %v", err)
	}
	defer repo.Close()

	if err := repo.Save(ctx, "ST-101", StationRegistryEntry{Station: "ST-101", Line: "line-2"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, "ST-101"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, "ST-101")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if !reflect.DeepEqual(got, StationRegistryEntry{}) {
		t.Fatalf("expected zero entry after delete, got %+v", got)
	}
}

// TestStationRegistryFileRepository_EmptyDirNoFile 는 파일이 없을 때 생성자가
// os.IsNotExist 를 관용적으로 처리해 빈 캐시로 시작함을 검증한다.
func TestStationRegistryFileRepository_EmptyDirNoFile(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewStationRegistryFileRepository(dir)
	if err != nil {
		t.Fatalf("NewStationRegistryFileRepository on empty dir: %v", err)
	}
	defer repo.Close()

	all, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(all))
	}
}
