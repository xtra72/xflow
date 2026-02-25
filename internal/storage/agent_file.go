package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xtra/xflow/internal/agent"
)

// AgentFileRepository 는 파일 시스템 기반의 AgentRepository 구현체이다.
// 각 에이전트 설정을 {dir}/agents/{id}.yaml 파일로 저장한다.
type AgentFileRepository struct {
	dir string
}

// 컴파일 타임 인터페이스 충족 검증
var _ AgentRepository = (*AgentFileRepository)(nil)

// NewAgentFileRepository 는 지정된 디렉토리 하위에 agents/ 서브디렉토리를 생성하고
// 파일 기반 에이전트 저장소를 반환한다.
// 디렉토리가 존재하지 않으면 자동으로 생성한다.
func NewAgentFileRepository(dir string) (*AgentFileRepository, error) {
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return nil, fmt.Errorf("create agents directory: %w", err)
	}
	return &AgentFileRepository{dir: agentsDir}, nil
}

// Save 는 에이전트 설정을 YAML 파일로 저장한다.
// 임시 파일에 먼저 쓴 후 os.Rename 으로 원자적 교체를 수행한다.
// 동일 ID 가 있으면 덮어쓴다.
func (r *AgentFileRepository) Save(_ context.Context, config agent.AgentConfig) error {
	id := config.ID

	data, err := agent.AgentConfigToYAML(config)
	if err != nil {
		return fmt.Errorf("serialize agent %s: %w", id, err)
	}

	tmpPath := filepath.Join(r.dir, id+".yaml.tmp")
	finalPath := filepath.Join(r.dir, id+".yaml")

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file for agent %s: %w", id, err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		// 임시 파일 정리 시도 (best-effort)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file for agent %s: %w", id, err)
	}

	return nil
}

// Get 은 ID 로 에이전트 설정을 조회한다.
// 파일이 존재하지 않으면 ErrAgentNotFound 를 반환한다.
func (r *AgentFileRepository) Get(_ context.Context, id string) (agent.AgentConfig, error) {
	path := filepath.Join(r.dir, id+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return agent.AgentConfig{}, ErrAgentNotFound
		}
		return agent.AgentConfig{}, fmt.Errorf("read agent %s: %w", id, err)
	}

	config, err := agent.AgentConfigFromYAML(data)
	if err != nil {
		return agent.AgentConfig{}, fmt.Errorf("deserialize agent %s: %w", id, err)
	}

	return config, nil
}

// List 는 저장소의 모든 에이전트 설정을 반환한다.
func (r *AgentFileRepository) List(_ context.Context) ([]agent.AgentConfig, error) {
	pattern := filepath.Join(r.dir, "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob agent files: %w", err)
	}

	configs := make([]agent.AgentConfig, 0, len(matches))
	for _, path := range matches {
		// .yaml.tmp 파일은 제외
		if strings.HasSuffix(path, ".yaml.tmp") {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read agent file %s: %w", path, err)
		}

		config, err := agent.AgentConfigFromYAML(data)
		if err != nil {
			return nil, fmt.Errorf("deserialize agent file %s: %w", path, err)
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// Delete 는 ID 로 에이전트 설정 파일을 삭제한다.
// 파일이 존재하지 않으면 ErrAgentNotFound 를 반환한다.
func (r *AgentFileRepository) Delete(_ context.Context, id string) error {
	path := filepath.Join(r.dir, id+".yaml")

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ErrAgentNotFound
		}
		return fmt.Errorf("stat agent %s: %w", id, err)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete agent %s: %w", id, err)
	}

	return nil
}

// Close 는 저장소 리소스를 정리한다.
// 파일 기반 저장소는 정리할 리소스가 없으므로 항상 nil 을 반환한다.
func (r *AgentFileRepository) Close() error {
	return nil
}
