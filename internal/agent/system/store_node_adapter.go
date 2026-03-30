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
