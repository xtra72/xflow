package node

import (
	"context"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
)

// mockStoreWriter 는 테스트용 StoreWriter + StoreReader 구현체이다.
type mockStoreWriter struct {
	mu   sync.Mutex
	data map[string]mockStoreEntry
	// mockHistory 는 QueryHistory 호출 시 반환할 시계열 엔트리이다.
	// 설정되지 않으면 현재값(data[key])만 포함한 단일 엔트리 슬라이스를 반환한다.
	mockHistory map[string][]map[string]any
	// meta 는 SetWithMeta 로 기록된 메타 쓰기 이력이다 (키별 시간순).
	meta map[string][]mockStoreMetaWrite
}

type mockStoreEntry struct {
	value any
	ttl   time.Duration
}

func newMockStore() *mockStoreWriter {
	return &mockStoreWriter{
		data:        make(map[string]mockStoreEntry),
		mockHistory: make(map[string][]map[string]any),
	}
}

// setMockHistory 는 QueryHistory 가 반환할 시계열 엔트리를 설정한다 (테스트 헬퍼).
func (m *mockStoreWriter) setMockHistory(key string, entries []map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockHistory[key] = entries
}

func (m *mockStoreWriter) Set(_ context.Context, key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = mockStoreEntry{value: value}
	return nil
}

func (m *mockStoreWriter) SetWithTTL(_ context.Context, key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = mockStoreEntry{value: value, ttl: ttl}
	return nil
}

func (m *mockStoreWriter) Get(_ context.Context, key string) (any, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	return entry.value, true, nil
}

func (m *mockStoreWriter) Has(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockStoreWriter) GetHistory(_ context.Context, key string) ([]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	// 테스트용: 간단한 히스토리를 반환한다
	return []any{"prev-value-2", "prev-value-1"}, nil
}

// QueryHistory 는 HistoryQueryReader 인터페이스를 구현한다.
// mockHistory 가 설정되어 있으면 해당 값을 그대로 반환하고,
// 없으면 data 에 저장된 현재값 1건을 포함한 단일 엔트리를 반환한다.
// system.HistoryQuery 의 Validate 를 호출하지만, 필터링은 적용하지 않는다 (테스트는 모드별 직접 검증).
func (m *mockStoreWriter) QueryHistory(_ context.Context, key string, q system.HistoryQuery) ([]map[string]any, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if hist, ok := m.mockHistory[key]; ok {
		// 반환 슬라이스를 복사하여 외부 변형을 방지한다.
		out := make([]map[string]any, len(hist))
		for i, e := range hist {
			cp := make(map[string]any, len(e))
			for k, v := range e {
				cp[k] = v
			}
			out[i] = cp
		}
		return out, nil
	}

	entry, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return []map[string]any{
		{"value": entry.value, "timestamp": time.Now().UnixMilli()},
	}, nil
}

// GetMetadata 는 MetadataReader 인터페이스를 구현한다.
// 테스트용 고정 메타데이터를 반환한다.
func (m *mockStoreWriter) GetMetadata(_ context.Context, key string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	now := time.Now()
	return map[string]any{
		"store_count":      3,
		"store_created_at": now.Add(-1 * time.Hour),
		"store_updated_at": now,
		"store_oldest_at":  now.Add(-2 * time.Hour),
	}, nil
}

// SetWithMeta 는 StoreMetaWriter 인터페이스를 구현한다 (storage-write 메타 경로 검증용).
// 마지막 쓰기의 메타를 키별로 보관하여 테스트가 검사할 수 있게 한다.
func (m *mockStoreWriter) SetWithMeta(_ context.Context, key string, value any, opts system.StoreWriteMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.meta == nil {
		m.meta = make(map[string][]mockStoreMetaWrite)
	}
	m.data[key] = mockStoreEntry{value: value, ttl: opts.TTL}
	m.meta[key] = append(m.meta[key], mockStoreMetaWrite{value: value, opts: opts})
	return nil
}

// metaWrites 는 키에 대해 기록된 메타 쓰기 이력을 복사해 반환한다.
func (m *mockStoreWriter) metaWrites(key string) []mockStoreMetaWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockStoreMetaWrite, len(m.meta[key]))
	copy(out, m.meta[key])
	return out
}

// mockStoreMetaWrite 는 SetWithMeta 한 번의 호출 기록이다.
type mockStoreMetaWrite struct {
	value any
	opts  system.StoreWriteMeta
}

// mockStoreAgent 는 storage-write 가 백엔드를 고르는 경로(Type + storeProvider)를
// 만족하는 테스트용 Store 에이전트이다.
type mockStoreAgent struct {
	store *mockStoreWriter
}

func (a *mockStoreAgent) Type() string { return "store" }

func (a *mockStoreAgent) NodeStoreForNamespace(_ string) any { return a.store }
