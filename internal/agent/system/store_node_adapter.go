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
type NodeStoreAdapter struct {
	store Store
}

// NewNodeStoreAdapter 는 system.Store를 감싸는 NodeStoreAdapter를 생성한다.
func NewNodeStoreAdapter(store Store) *NodeStoreAdapter {
	return &NodeStoreAdapter{store: store}
}

// Set 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) Set(ctx context.Context, key string, value any) error {
	return a.store.Set(ctx, key, value)
}

// SetWithTTL 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	return a.store.SetWithTTL(ctx, key, value, ttl)
}

// Get 은 node.StoreReader 인터페이스를 구현한다.
// Store.Get() 의 (StoreEntry, error) 를 (any, bool, error) 로 변환한다.
// ErrKeyNotFound 발생 시 (nil, false, nil) 을 반환한다.
func (a *NodeStoreAdapter) Get(ctx context.Context, key string) (any, bool, error) {
	entry, err := a.store.Get(ctx, key)
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
	return a.store.Has(ctx, key)
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 반환한다.
// 반환 값은 []any 형태로 이전 값들만 담는다 (최신순).
func (a *NodeStoreAdapter) GetHistory(ctx context.Context, key string) ([]any, error) {
	entries, err := a.store.GetHistory(ctx, key)
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
// map 슬라이스로 통일한다. 결과는 최신순이며, 현재값을 포함한다.
// ErrKeyNotFound 발생 시 (nil, nil)을 반환하여 노드 계층에서 "값 없음"으로 처리한다.
func (a *NodeStoreAdapter) QueryHistory(ctx context.Context, key string, query HistoryQuery) ([]map[string]any, error) {
	entries, err := a.store.QueryHistory(ctx, key, query)
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
			"timestamp": e.Timestamp,
		}
	}
	return result, nil
}

// GetMetadata 는 주어진 키의 메타데이터를 map[string]any 형태로 반환한다.
// 반환되는 키: "store_count", "store_created_at", "store_updated_at", "store_oldest_at"
func (a *NodeStoreAdapter) GetMetadata(ctx context.Context, key string) (map[string]any, error) {
	entry, err := a.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	oldestAt := entry.CreatedAt

	// 히스토리가 존재하면 가장 오래된 항목의 타임스탬프를 사용한다
	if entry.HistoryCount > 0 {
		history, hErr := a.store.GetHistory(ctx, key)
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
