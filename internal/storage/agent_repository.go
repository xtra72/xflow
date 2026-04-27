package storage

import (
	"context"
	"errors"

	"github.com/xtra/xflow/internal/agent"
)

// ErrAgentNotFound 는 요청한 에이전트가 저장소에 없을 때 반환된다.
var ErrAgentNotFound = errors.New("agent not found")

// AgentRepository 는 에이전트 설정의 영속 저장소 인터페이스이다.
type AgentRepository interface {
	// Save 는 에이전트 설정을 저장소에 저장한다. 동일 ID 가 있으면 덮어쓴다.
	Save(ctx context.Context, config agent.AgentConfig) error
	// Get 은 ID 로 에이전트 설정을 조회한다. 없으면 ErrAgentNotFound.
	Get(ctx context.Context, id string) (agent.AgentConfig, error)
	// List 는 저장소의 모든 에이전트 설정을 반환한다.
	List(ctx context.Context) ([]agent.AgentConfig, error)
	// Delete 는 ID 로 에이전트 설정을 삭제한다. 없으면 ErrAgentNotFound.
	Delete(ctx context.Context, id string) error
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}
