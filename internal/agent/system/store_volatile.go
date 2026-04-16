package system

import (
	"context"
	"path"
	"sync"
	"time"
)

// historyEntry 는 값 변경 히스토리의 내부 항목이다.
type historyEntry struct {
	value     any
	timestamp time.Time
}

// storeItem 은 sync.Map에 저장되는 내부 구조체이다.
type storeItem struct {
	value     any
	createdAt time.Time
	updatedAt time.Time
	expiresAt time.Time // zero value면 만료 없음
	namespace string
	history   []historyEntry // 값 변경 히스토리 (최신순)
}

// VolatileStore 는 sync.Map 기반의 인메모리 키-값 저장소이다.
type VolatileStore struct {
	data           sync.Map
	maxKeyLength   int
	maxHistorySize int           // 히스토리 최대 보관 수 (0이면 비활성)
	historyTTL     time.Duration // 히스토리 항목 최대 보관 시간 (0이면 무제한)
}

// NewVolatileStore 는 주어진 설정으로 VolatileStore를 생성한다.
func NewVolatileStore(maxKeyLength, maxHistorySize int, historyTTL time.Duration) *VolatileStore {
	return &VolatileStore{
		maxKeyLength:   maxKeyLength,
		maxHistorySize: maxHistorySize,
		historyTTL:     historyTTL,
	}
}

// validateKey 는 키 길이를 검증한다.
func (s *VolatileStore) validateKey(key string) error {
	if len(key) > s.maxKeyLength {
		return ErrKeyTooLong
	}
	return nil
}

// isExpired 는 아이템이 만료되었는지 확인한다.
func (s *VolatileStore) isExpired(item *storeItem) bool {
	if item.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(item.expiresAt)
}

// Get 은 주어진 키에 해당하는 엔트리를 반환한다.
// 만료된 키는 lazy expiration으로 삭제한 뒤 ErrKeyNotFound를 반환한다.
func (s *VolatileStore) Get(_ context.Context, key string) (StoreEntry, error) {
	raw, ok := s.data.Load(key)
	if !ok {
		return StoreEntry{}, ErrKeyNotFound
	}

	item := raw.(*storeItem)

	// lazy expiration: 만료된 키는 삭제 후 ErrKeyNotFound 반환
	if s.isExpired(item) {
		s.data.Delete(key)
		return StoreEntry{}, ErrKeyNotFound
	}

	entry := StoreEntry{
		Value:          item.value,
		CreatedAt:      item.createdAt,
		UpdatedAt:      item.updatedAt,
		ExpiresAt:      item.expiresAt,
		Namespace:      item.namespace,
		HistoryCount:   len(item.history),
		MaxHistorySize: s.maxHistorySize,
	}

	// 남은 TTL 계산
	if !item.expiresAt.IsZero() {
		remaining := time.Until(item.expiresAt)
		if remaining > 0 {
			entry.TTL = remaining
		}
	}

	return entry, nil
}

// Set 은 주어진 키에 값을 저장한다.
// 기존 키가 존재하면 CreatedAt과 ExpiresAt을 보존하고 값과 UpdatedAt만 갱신한다.
func (s *VolatileStore) Set(_ context.Context, key string, value any) error {
	if value == nil {
		return ErrNilValue
	}
	if err := s.validateKey(key); err != nil {
		return err
	}

	now := time.Now()

	// 기존 아이템이 존재하면 CreatedAt, ExpiresAt, Namespace를 보존한다
	if raw, ok := s.data.Load(key); ok {
		existing := raw.(*storeItem)
		// 만료되지 않은 경우에만 보존
		if !s.isExpired(existing) {
			newHistory := s.buildHistory(existing)
			s.data.Store(key, &storeItem{
				value:     value,
				createdAt: existing.createdAt,
				updatedAt: now,
				expiresAt: existing.expiresAt,
				namespace: existing.namespace,
				history:   newHistory,
			})
			return nil
		}
	}

	// 새 아이템 생성
	s.data.Store(key, &storeItem{
		value:     value,
		createdAt: now,
		updatedAt: now,
	})
	return nil
}

// SetWithTTL 은 주어진 키에 TTL과 함께 값을 저장한다.
// TTL이 0이면 만료 없음을 의미한다.
// 기존 키가 존재하면 CreatedAt을 보존하되 ExpiresAt을 갱신한다.
func (s *VolatileStore) SetWithTTL(_ context.Context, key string, value any, ttl time.Duration) error {
	if value == nil {
		return ErrNilValue
	}
	if ttl < 0 {
		return ErrInvalidTTL
	}
	if err := s.validateKey(key); err != nil {
		return err
	}

	now := time.Now()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	// 기존 아이템이 존재하면 CreatedAt을 보존한다
	createdAt := now
	namespace := ""
	var newHistory []historyEntry
	if raw, ok := s.data.Load(key); ok {
		existing := raw.(*storeItem)
		if !s.isExpired(existing) {
			createdAt = existing.createdAt
			namespace = existing.namespace
			newHistory = s.buildHistory(existing)
		}
	}

	s.data.Store(key, &storeItem{
		value:     value,
		createdAt: createdAt,
		updatedAt: now,
		expiresAt: expiresAt,
		namespace: namespace,
		history:   newHistory,
	})
	return nil
}

// Delete 는 주어진 키를 삭제한다.
// 키가 존재하지 않아도 에러를 반환하지 않는다.
func (s *VolatileStore) Delete(_ context.Context, key string) error {
	s.data.Delete(key)
	return nil
}

// Has 는 주어진 키가 존재하고 만료되지 않았는지 확인한다.
func (s *VolatileStore) Has(_ context.Context, key string) (bool, error) {
	raw, ok := s.data.Load(key)
	if !ok {
		return false, nil
	}

	item := raw.(*storeItem)
	if s.isExpired(item) {
		s.data.Delete(key)
		return false, nil
	}

	return true, nil
}

// Keys 는 패턴에 일치하는 키 목록을 반환한다.
// 빈 문자열이나 "*"은 모든 키를 반환한다.
// 만료된 키는 결과에서 제외된다.
func (s *VolatileStore) Keys(_ context.Context, pattern string) ([]string, error) {
	matchAll := pattern == "" || pattern == "*"
	var keys []string

	s.data.Range(func(k, v any) bool {
		key := k.(string)
		item := v.(*storeItem)

		// 만료된 키는 건너뛴다
		if s.isExpired(item) {
			return true
		}

		if matchAll {
			keys = append(keys, key)
			return true
		}

		matched, err := path.Match(pattern, key)
		if err != nil {
			// 잘못된 패턴이면 건너뛴다
			return true
		}
		if matched {
			keys = append(keys, key)
		}
		return true
	})

	return keys, nil
}

// buildHistory 는 기존 아이템의 현재 값을 히스토리에 추가하고 트리밍한 결과를 반환한다.
// maxHistorySize가 0이면 빈 슬라이스를 반환한다 (히스토리 비활성).
func (s *VolatileStore) buildHistory(existing *storeItem) []historyEntry {
	if s.maxHistorySize <= 0 {
		return nil
	}

	// 현재 값을 히스토리 맨 앞에 추가 (최신순)
	entry := historyEntry{value: existing.value, timestamp: existing.updatedAt}
	history := make([]historyEntry, 0, len(existing.history)+1)
	history = append(history, entry)
	history = append(history, existing.history...)

	// 개수 기반 트리밍
	if len(history) > s.maxHistorySize {
		history = history[:s.maxHistorySize]
	}

	// 시간 기반 트리밍
	if s.historyTTL > 0 {
		cutoff := time.Now().Add(-s.historyTTL)
		for i, h := range history {
			if h.timestamp.Before(cutoff) {
				history = history[:i]
				break
			}
		}
	}

	return history
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 최신순으로 반환한다.
// 키가 존재하지 않거나 만료된 경우 ErrKeyNotFound를 반환한다.
// 키가 존재하지만 히스토리가 없으면 빈 슬라이스를 반환한다.
func (s *VolatileStore) GetHistory(_ context.Context, key string) ([]HistoryEntry, error) {
	raw, ok := s.data.Load(key)
	if !ok {
		return nil, ErrKeyNotFound
	}

	item := raw.(*storeItem)

	// 만료된 키는 lazy expiration으로 삭제
	if s.isExpired(item) {
		s.data.Delete(key)
		return nil, ErrKeyNotFound
	}

	if len(item.history) == 0 {
		return []HistoryEntry{}, nil
	}

	result := make([]HistoryEntry, len(item.history))
	for i, h := range item.history {
		result[i] = HistoryEntry{Value: h.value, Timestamp: h.timestamp}
	}
	return result, nil
}

// QueryHistory 는 HistoryQuery 조건에 부합하는 시계열 엔트리를 최신순으로 반환한다.
// GetHistory 와 달리 결과 맨 앞에 현재값(item.value, item.updatedAt)을 포함한다.
// 키가 존재하지 않거나 만료된 경우 ErrKeyNotFound를 반환한다.
func (s *VolatileStore) QueryHistory(_ context.Context, key string, q HistoryQuery) ([]HistoryEntry, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	raw, ok := s.data.Load(key)
	if !ok {
		return nil, ErrKeyNotFound
	}

	item := raw.(*storeItem)

	// lazy expiration: 만료된 키는 삭제 후 ErrKeyNotFound 반환
	if s.isExpired(item) {
		s.data.Delete(key)
		return nil, ErrKeyNotFound
	}

	// 현재값을 최신 항목으로 포함하여 통합 시계열을 구성한다.
	combined := make([]HistoryEntry, 0, len(item.history)+1)
	combined = append(combined, HistoryEntry{Value: item.value, Timestamp: item.updatedAt})
	for _, h := range item.history {
		combined = append(combined, HistoryEntry{Value: h.value, Timestamp: h.timestamp})
	}

	return filterHistoryByQuery(combined, q, time.Now()), nil
}

// filterHistoryByQuery 는 최신순 엔트리 슬라이스에 HistoryQuery 필터를 적용한 결과를 반환한다.
// entries 는 반드시 최신순(내림차순) 이어야 하며, 현재값이 entries[0] 이라고 가정한다.
// now 는 duration 모드의 기준 시각으로 사용된다 (테스트 주입 용이성).
func filterHistoryByQuery(entries []HistoryEntry, q HistoryQuery, now time.Time) []HistoryEntry {
	switch q.Mode {
	case QueryModeLatest:
		if len(entries) == 0 {
			return []HistoryEntry{}
		}
		return []HistoryEntry{entries[0]}

	case QueryModeLastN:
		if q.Count >= len(entries) {
			out := make([]HistoryEntry, len(entries))
			copy(out, entries)
			return out
		}
		out := make([]HistoryEntry, q.Count)
		copy(out, entries[:q.Count])
		return out

	case QueryModeDuration:
		cutoff := now.Add(-q.Duration)
		out := make([]HistoryEntry, 0, len(entries))
		for _, e := range entries {
			if e.Timestamp.Before(cutoff) {
				break
			}
			out = append(out, e)
		}
		return out

	case QueryModeTimeRange:
		out := make([]HistoryEntry, 0, len(entries))
		for _, e := range entries {
			// 최신순이므로 To 이후 항목은 건너뛰고, From 이전 항목은 종료.
			if e.Timestamp.After(q.To) {
				continue
			}
			if e.Timestamp.Before(q.From) {
				break
			}
			out = append(out, e)
		}
		return out

	case QueryModeSinceN:
		out := make([]HistoryEntry, 0, q.Count)
		for _, e := range entries {
			if e.Timestamp.Before(q.Since) {
				break
			}
			out = append(out, e)
			if len(out) >= q.Count {
				break
			}
		}
		return out
	}

	return []HistoryEntry{}
}

// setItemNamespace 는 저장된 항목의 namespace 필드를 설정한다 (namespaceWriter 구현).
func (s *VolatileStore) setItemNamespace(key string, namespace string) {
	if raw, ok := s.data.Load(key); ok {
		item := raw.(*storeItem)
		item.namespace = namespace
	}
}

// Clear 는 모든 키를 삭제한다.
func (s *VolatileStore) Clear(_ context.Context) error {
	s.data.Range(func(k, _ any) bool {
		s.data.Delete(k)
		return true
	})
	return nil
}
