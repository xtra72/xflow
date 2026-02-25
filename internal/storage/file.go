package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xtra/xflow/pkg/flow"
)

// FileRepository 는 파일 시스템 기반의 FlowRepository 구현체이다.
// 각 플로우를 {flowID}.yaml 파일로 저장한다.
type FileRepository struct {
	dir string
}

// 컴파일 타임 인터페이스 충족 검증
var _ FlowRepository = (*FileRepository)(nil)

// NewFileRepository 는 지정된 디렉토리에 파일 기반 저장소를 생성한다.
// 디렉토리가 존재하지 않으면 자동으로 생성한다.
func NewFileRepository(dir string) (*FileRepository, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &FileRepository{dir: dir}, nil
}

// Save 는 플로우를 YAML 파일로 저장한다.
// 임시 파일에 먼저 쓴 후 os.Rename 으로 원자적 교체를 수행한다.
// 동일 ID 가 있으면 덮어쓴다.
func (r *FileRepository) Save(_ context.Context, f flow.Flow) error {
	id := f.ID()

	data, err := flow.FlowToYAML(f)
	if err != nil {
		return fmt.Errorf("serialize flow %s: %w", id, err)
	}

	tmpPath := filepath.Join(r.dir, id+".yaml.tmp")
	finalPath := filepath.Join(r.dir, id+".yaml")

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file for flow %s: %w", id, err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		// 임시 파일 정리 시도 (best-effort)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file for flow %s: %w", id, err)
	}

	return nil
}

// Get 은 ID 로 플로우를 조회한다.
// 파일이 존재하지 않으면 ErrFlowNotFound 를 반환한다.
func (r *FileRepository) Get(_ context.Context, id string) (flow.Flow, error) {
	path := filepath.Join(r.dir, id+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFlowNotFound
		}
		return nil, fmt.Errorf("read flow %s: %w", id, err)
	}

	f, err := flow.FlowFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize flow %s: %w", id, err)
	}

	return f, nil
}

// List 는 저장소의 모든 플로우를 반환한다.
func (r *FileRepository) List(_ context.Context) ([]flow.Flow, error) {
	pattern := filepath.Join(r.dir, "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob flow files: %w", err)
	}

	flows := make([]flow.Flow, 0, len(matches))
	for _, path := range matches {
		// .yaml.tmp 파일은 제외
		if strings.HasSuffix(path, ".yaml.tmp") {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read flow file %s: %w", path, err)
		}

		f, err := flow.FlowFromYAML(data)
		if err != nil {
			return nil, fmt.Errorf("deserialize flow file %s: %w", path, err)
		}

		flows = append(flows, f)
	}

	return flows, nil
}

// Delete 는 ID 로 플로우 파일을 삭제한다.
// 파일이 존재하지 않으면 ErrFlowNotFound 를 반환한다.
func (r *FileRepository) Delete(_ context.Context, id string) error {
	path := filepath.Join(r.dir, id+".yaml")

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ErrFlowNotFound
		}
		return fmt.Errorf("stat flow %s: %w", id, err)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete flow %s: %w", id, err)
	}

	return nil
}

// Close 는 저장소 리소스를 정리한다.
// 파일 기반 저장소는 정리할 리소스가 없으므로 항상 nil 을 반환한다.
func (r *FileRepository) Close() error {
	return nil
}
