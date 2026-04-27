package storage

import (
	"context"
	"errors"

	"github.com/xtra/xflow/pkg/flow"
)

// ErrFlowNotFound 는 요청한 플로우가 저장소에 없을 때 반환된다.
var ErrFlowNotFound = errors.New("flow not found")

// FlowRepository 는 플로우의 영속 저장소 인터페이스이다.
type FlowRepository interface {
	// Save 는 플로우를 저장소에 저장한다. 동일 ID 가 있으면 덮어쓴다.
	Save(ctx context.Context, f flow.Flow) error
	// Get 은 ID 로 플로우를 조회한다. 없으면 ErrFlowNotFound.
	Get(ctx context.Context, id string) (flow.Flow, error)
	// List 는 저장소의 모든 플로우를 반환한다.
	List(ctx context.Context) ([]flow.Flow, error)
	// Delete 는 ID 로 플로우를 삭제한다. 없으면 ErrFlowNotFound.
	Delete(ctx context.Context, id string) error
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}
