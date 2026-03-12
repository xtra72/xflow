package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/xtra/xflow/internal/device"
)

// DeviceMetadataRepository defines the interface for persisting device metadata.
type DeviceMetadataRepository interface {
	// Save persists metadata for a device.
	Save(ctx context.Context, deviceID string, metadata device.DeviceMetadata) error

	// Get retrieves metadata for a device.
	// Returns empty DeviceMetadata and nil error if not found.
	Get(ctx context.Context, deviceID string) (device.DeviceMetadata, error)

	// Delete removes metadata for a device.
	Delete(ctx context.Context, deviceID string) error

	// List returns all stored device metadata as a map of deviceID -> metadata.
	List(ctx context.Context) (map[string]device.DeviceMetadata, error)

	// Close releases resources.
	Close() error
}

// DeviceMetadataFileRepository is a file-system based implementation
// of DeviceMetadataRepository. All metadata is stored in a single JSON file
// for simplicity and atomic updates.
type DeviceMetadataFileRepository struct {
	filePath string
	mu       sync.RWMutex
	cache    map[string]device.DeviceMetadata // in-memory cache
}

// Compile-time interface check.
var _ DeviceMetadataRepository = (*DeviceMetadataFileRepository)(nil)

// NewDeviceMetadataFileRepository creates a file-based device metadata repository.
// The metadata file is stored at {dir}/device_metadata.json.
// The directory is created if it does not exist.
func NewDeviceMetadataFileRepository(dir string) (*DeviceMetadataFileRepository, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create metadata directory: %w", err)
	}

	r := &DeviceMetadataFileRepository{
		filePath: filepath.Join(dir, "device_metadata.json"),
		cache:    make(map[string]device.DeviceMetadata),
	}

	// Load existing metadata from file.
	if err := r.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load device metadata: %w", err)
	}

	return r, nil
}

// Save persists metadata for a device.
func (r *DeviceMetadataFileRepository) Save(_ context.Context, deviceID string, metadata device.DeviceMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[deviceID] = metadata
	return r.saveToFile()
}

// Get retrieves metadata for a device.
// Returns empty DeviceMetadata and nil error if not found.
func (r *DeviceMetadataFileRepository) Get(_ context.Context, deviceID string) (device.DeviceMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.cache[deviceID]
	if !ok {
		return device.DeviceMetadata{}, nil
	}
	return meta, nil
}

// Delete removes metadata for a device.
func (r *DeviceMetadataFileRepository) Delete(_ context.Context, deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.cache, deviceID)
	return r.saveToFile()
}

// List returns all stored device metadata.
func (r *DeviceMetadataFileRepository) List(_ context.Context) (map[string]device.DeviceMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]device.DeviceMetadata, len(r.cache))
	for k, v := range r.cache {
		result[k] = v
	}
	return result, nil
}

// Close releases resources. No-op for file-based storage.
func (r *DeviceMetadataFileRepository) Close() error {
	return nil
}

// loadFromFile reads metadata from the JSON file into the cache.
func (r *DeviceMetadataFileRepository) loadFromFile() error {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &r.cache)
}

// saveToFile writes the cache to the JSON file atomically.
func (r *DeviceMetadataFileRepository) saveToFile() error {
	data, err := json.MarshalIndent(r.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device metadata: %w", err)
	}

	tmpPath := r.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp metadata file: %w", err)
	}

	if err := os.Rename(tmpPath, r.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp metadata file: %w", err)
	}

	return nil
}
