package system

import (
	"context"
	"path"
	"sync"
	"time"
)

// storeItem 은 sync.Map에 저장되는 내부 구조체이다.
type storeItem struct {
	value     any
	createdAt time.Time
	updatedAt time.Time
	expiresAt time.Time // zero value면 만료 없음
	namespace string
}

// VolatileStore 는 sync.Map 기반의 인메모리 키-값 저장소이다.
type VolatileStore struct {
	data         sync.Map
	maxKeyLength int
}

// NewVolatileStore 는 주어진 최대 키 길이로 VolatileStore를 생성한다.
func NewVolatileStore(maxKeyLength int) *VolatileStore {
	return &VolatileStore{
		maxKeyLength: maxKeyLength,
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
		Value:     item.value,
		CreatedAt: item.createdAt,
		UpdatedAt: item.updatedAt,
		ExpiresAt: item.expiresAt,
		Namespace: item.namespace,
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
			s.data.Store(key, &storeItem{
				value:     value,
				createdAt: existing.createdAt,
				updatedAt: now,
				expiresAt: existing.expiresAt,
				namespace: existing.namespace,
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
	if raw, ok := s.data.Load(key); ok {
		existing := raw.(*storeItem)
		if !s.isExpired(existing) {
			createdAt = existing.createdAt
			namespace = existing.namespace
		}
	}

	s.data.Store(key, &storeItem{
		value:     value,
		createdAt: createdAt,
		updatedAt: now,
		expiresAt: expiresAt,
		namespace: namespace,
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

// Clear 는 모든 키를 삭제한다.
func (s *VolatileStore) Clear(_ context.Context) error {
	s.data.Range(func(k, _ any) bool {
		s.data.Delete(k)
		return true
	})
	return nil
}
