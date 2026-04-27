package system

import (
	"context"
	"errors"
	"time"
)

// NodeStoreAdapter 는 system.Store 인터페이스를 node.StoreWriter / node.StoreReader
// 인터페이스로 변환하는 어댑터이다.
//
// Store.Get() 는 (StoreEntry, error) 를 반환하지만
// node.StoreReader.Get() 는 (any, bool, error) 를 기대한다.
// ErrKeyNotFound 를 (nil, false, nil) 로 변환하여 인터페이스 차이를 해소한다.
//
// 내부적으로 resolver 함수를 통해 매 호출마다 현재 유효한 Store 를 조회한다.
// 이는 UserStoreAgent 재시작(Stop → Start) 으로 내부 inner 가 새 인스턴스로
// 교체되더라도, 플로우 노드가 들고 있는 기존 Adapter 가 자동으로 새 Store 에
// 라우팅되도록 보장하기 위함이다. 생성 시점의 Store 스냅샷을 잡으면 재시작
// 이후 쓰기/읽기가 모두 정지된 옛 inner 로 향해 실패하는 회귀가 발생한다.
type NodeStoreAdapter struct {
	resolver func() Store
}

// NewNodeStoreAdapter 는 고정된 system.Store 를 감싸는 NodeStoreAdapter 를 생성한다.
// 하위 호환을 위해 기존 시그니처를 유지하며, 내부적으로는 전달된 store 를
// 그대로 반환하는 resolver 로 래핑한다. 생명주기 전환이 없는 테스트 및
// 단순 사용처에 적합하다.
func NewNodeStoreAdapter(store Store) *NodeStoreAdapter {
	return &NodeStoreAdapter{resolver: func() Store { return store }}
}

// NewLazyNodeStoreAdapter 는 매 호출마다 resolver 를 호출하여 현재 Store 를
// 동적으로 얻는 Adapter 를 반환한다. 에이전트 재시작 등으로 내부 Store 가
// 교체되어도 resolver 가 최신 인스턴스를 돌려주면 동일한 Adapter 로 계속
// 올바른 Store 에 접근할 수 있다.
//
// resolver 는 nil 을 반환해서는 안 된다. 호출자는 resolver 가 어떤 동시성
// 보장(락 등) 아래에서 안전하게 현재 Store 를 돌려주도록 구현해야 한다.
func NewLazyNodeStoreAdapter(resolver func() Store) *NodeStoreAdapter {
	return &NodeStoreAdapter{resolver: resolver}
}

// Set 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) Set(ctx context.Context, key string, value any) error {
	return a.resolver().Set(ctx, key, value)
}

// SetWithTTL 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	return a.resolver().SetWithTTL(ctx, key, value, ttl)
}

// Get 은 node.StoreReader 인터페이스를 구현한다.
// Store.Get() 의 (StoreEntry, error) 를 (any, bool, error) 로 변환한다.
// ErrKeyNotFound 발생 시 (nil, false, nil) 을 반환한다.
func (a *NodeStoreAdapter) Get(ctx context.Context, key string) (any, bool, error) {
	entry, err := a.resolver().Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return entry.Value, true, nil
}

// Has 는 node.StoreReader 인터페이스를 구현한다.
func (a *NodeStoreAdapter) Has(ctx context.Context, key string) (bool, error) {
	return a.resolver().Has(ctx, key)
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 반환한다.
// 반환 값은 []any 형태로 이전 값들만 담는다 (최신순).
func (a *NodeStoreAdapter) GetHistory(ctx context.Context, key string) ([]any, error) {
	entries, err := a.resolver().GetHistory(ctx, key)
	if err != nil {
		return nil, err
	}
	result := make([]any, len(entries))
	for i, e := range entries {
		result[i] = e.Value
	}
	return result, nil
}

// QueryHistory 는 HistoryQuery 조건에 따른 시계열 엔트리를 반환한다.
// 반환 형식은 payload 에 그대로 실릴 수 있도록 "value"/"timestamp" 키를 갖는
// map 슬라이스로 통일한다. timestamp 는 프로젝트 전체 컨벤션에 따라 epoch
// 밀리초(int64, UnixMilli)로 직렬화된다. 결과는 최신순이며, 현재값을 포함한다.
// ErrKeyNotFound 발생 시 (nil, nil)을 반환하여 노드 계층에서 "값 없음"으로 처리한다.
func (a *NodeStoreAdapter) QueryHistory(ctx context.Context, key string, query HistoryQuery) ([]map[string]any, error) {
	entries, err := a.resolver().QueryHistory(ctx, key, query)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		result[i] = map[string]any{
			"value":     e.Value,
			"timestamp": e.Timestamp.UnixMilli(),
		}
	}
	return result, nil
}

// GetMetadata 는 주어진 키의 메타데이터를 map[string]any 형태로 반환한다.
// 반환되는 키: "store_count", "store_created_at", "store_updated_at", "store_oldest_at"
func (a *NodeStoreAdapter) GetMetadata(ctx context.Context, key string) (map[string]any, error) {
	store := a.resolver()
	entry, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	oldestAt := entry.CreatedAt

	// 히스토리가 존재하면 가장 오래된 항목의 타임스탬프를 사용한다
	if entry.HistoryCount > 0 {
		history, hErr := store.GetHistory(ctx, key)
		if hErr == nil && len(history) > 0 {
			// 히스토리는 최신순이므로 마지막 항목이 가장 오래된 것
			oldestAt = history[len(history)-1].Timestamp
		}
	}

	return map[string]any{
		"store_count":      entry.HistoryCount,
		"store_created_at": entry.CreatedAt,
		"store_updated_at": entry.UpdatedAt,
		"store_oldest_at":  oldestAt,
	}, nil
}
