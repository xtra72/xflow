package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xtra/xflow/internal/device"
)

func TestDeviceMetadataFileRepository_SaveAndGet(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	meta := device.DeviceMetadata{
		Tags:     []string{"hvac", "indoor"},
		Location: "Building A, Floor 2",
		Group:    "floor-2",
		Labels: map[string]string{
			"zone": "north",
		},
	}

	// Save
	if err := repo.Save(ctx, "nasa:20.00.01", meta); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Get
	got, err := repo.Get(ctx, "nasa:20.00.01")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Location != "Building A, Floor 2" {
		t.Errorf("Location = %q, want %q", got.Location, "Building A, Floor 2")
	}
	if got.Group != "floor-2" {
		t.Errorf("Group = %q, want %q", got.Group, "floor-2")
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(got.Tags))
	}
	if got.Labels["zone"] != "north" {
		t.Errorf("Labels[zone] = %q, want north", got.Labels["zone"])
	}
}

func TestDeviceMetadataFileRepository_GetNonExistent(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()
	meta, err := repo.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty metadata, not an error
	if meta.Location != "" {
		t.Errorf("expected empty Location, got %q", meta.Location)
	}
	if meta.Tags != nil {
		t.Errorf("expected nil Tags, got %v", meta.Tags)
	}
}

func TestDeviceMetadataFileRepository_Delete(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()
	meta := device.DeviceMetadata{Location: "test"}

	if err := repo.Save(ctx, "dev-1", meta); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := repo.Delete(ctx, "dev-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.Get(ctx, "dev-1")
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if got.Location != "" {
		t.Error("expected empty metadata after delete")
	}
}

func TestDeviceMetadataFileRepository_List(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()

	// Initially empty
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	// Add some entries
	repo.Save(ctx, "dev-1", device.DeviceMetadata{Location: "room-1"})
	repo.Save(ctx, "dev-2", device.DeviceMetadata{Location: "room-2"})

	list, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list["dev-1"].Location != "room-1" {
		t.Errorf("dev-1 Location = %q, want room-1", list["dev-1"].Location)
	}
	if list["dev-2"].Location != "room-2" {
		t.Errorf("dev-2 Location = %q, want room-2", list["dev-2"].Location)
	}
}

func TestDeviceMetadataFileRepository_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create and save
	repo1, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()
	repo1.Save(ctx, "dev-1", device.DeviceMetadata{
		Location: "server-room",
		Group:    "infrastructure",
		Tags:     []string{"critical"},
	})
	repo1.Close()

	// Reopen and verify data persisted
	repo2, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository (reopen): %v", err)
	}

	got, err := repo2.Get(ctx, "dev-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Location != "server-room" {
		t.Errorf("Location = %q, want server-room", got.Location)
	}
	if got.Group != "infrastructure" {
		t.Errorf("Group = %q, want infrastructure", got.Group)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "critical" {
		t.Errorf("Tags = %v, want [critical]", got.Tags)
	}
}

func TestDeviceMetadataFileRepository_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()
	repo.Save(ctx, "dev-1", device.DeviceMetadata{Location: "test"})

	// Verify file exists
	filePath := filepath.Join(dir, "device_metadata.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("metadata file should exist after save")
	}

	// Verify no temp file remains
	tmpPath := filePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}
}

func TestDeviceMetadataFileRepository_Update(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("NewDeviceMetadataFileRepository: %v", err)
	}

	ctx := context.Background()

	// Initial save
	repo.Save(ctx, "dev-1", device.DeviceMetadata{
		Location: "room-1",
		Group:    "group-a",
	})

	// Update
	repo.Save(ctx, "dev-1", device.DeviceMetadata{
		Location: "room-2",
		Group:    "group-b",
		Tags:     []string{"updated"},
	})

	got, _ := repo.Get(ctx, "dev-1")
	if got.Location != "room-2" {
		t.Errorf("Location = %q, want room-2", got.Location)
	}
	if got.Group != "group-b" {
		t.Errorf("Group = %q, want group-b", got.Group)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "updated" {
		t.Errorf("Tags = %v, want [updated]", got.Tags)
	}
}

func TestDeviceMetadataFileRepository_EmptyFile(t *testing.T) {
	dir := t.TempDir()

	// Create empty file
	filePath := filepath.Join(dir, "device_metadata.json")
	os.WriteFile(filePath, []byte{}, 0644)

	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("should handle empty file gracefully: %v", err)
	}

	ctx := context.Background()
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list for empty file, got %d items", len(list))
	}
}

func TestDeviceMetadataFileRepository_CreateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	repo, err := NewDeviceMetadataFileRepository(dir)
	if err != nil {
		t.Fatalf("should create nested directory: %v", err)
	}

	ctx := context.Background()
	if err := repo.Save(ctx, "dev-1", device.DeviceMetadata{Location: "test"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
}
