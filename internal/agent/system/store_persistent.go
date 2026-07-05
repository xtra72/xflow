package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"
)

// PersistentStore 는 리포지토리 기반 영속 저장소이다.
// sync.Map 캐시와 StoreRepository 백엔드를 조합한 Write-Through 캐시 패턴을 사용한다.
//
// @spec SPEC-STORE-004 (M6 영속 정합)
// 시리즈 정합: PersistentStore/StoreRepository 는 시리즈를 "전혀 모른 채" 불투명한
// string 키만 다룬다. 시리즈 인코딩(metric|tags|key)은 상위 system 계층(SetWithMeta)이
// EncodeSeriesKey 로 산출하며, 그 결과 문자열이 namespace prefix 와 결합되어
// (`namespace:metric|tags|key`) 본 저장소의 Set/Get/Delete 키로 그대로 전달된다.
// 따라서 영속 계층은 시리즈 키를 byte-perfect 로 저장/조회/삭제하기만 하면 정합하며,
// 시리즈 도입으로 인한 인터페이스/시그니처 변경은 없다. (회귀 방지:
// store_persistent_series_test.go 의 characterization 테스트가 이 계약을 고정한다.)
type PersistentStore struct {
	cache        sync.Map
	repo         StoreRepository
	maxKeyLength int
}

// NewPersistentStore 는 리포지토리와 최대 키 길이를 받아 PersistentStore를 생성한다.
func NewPersistentStore(repo StoreRepository, maxKeyLength int) *PersistentStore {
	return &PersistentStore{
		repo:         repo,
		maxKeyLength: maxKeyLength,
	}
}

// validateKey 는 키 길이를 검증한다.
func (s *PersistentStore) validateKey(key string) error {
	if len(key) > s.maxKeyLength {
		return ErrKeyTooLong
	}
	return nil
}

// isExpired 는 아이템이 만료되었는지 확인한다.
func (s *PersistentStore) isExpired(item *storeItem) bool {
	if item.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(item.expiresAt)
}

// checkSerializable 는 값이 JSON 직렬화 가능한지 확인한다.
func (s *PersistentStore) checkSerializable(value any) error {
	_, err := json.Marshal(value)
	if err != nil {
		return ErrNotSerializable
	}
	return nil
}

// Get 은 주어진 키에 해당하는 엔트리를 반환한다.
// 캐시를 먼저 확인하고, 캐시 미스 시 리포지토리에서 조회하여 캐시에 적재한다.
// 만료된 키는 lazy expiration으로 삭제 후 ErrKeyNotFound를 반환한다.
func (s *PersistentStore) Get(ctx context.Context, key string) (StoreEntry, error) {
	// 캐시에서 먼저 조회
	if raw, ok := s.cache.Load(key); ok {
		item := raw.(*storeItem)

		// lazy expiration: 만료된 키는 삭제 후 ErrKeyNotFound 반환
		if s.isExpired(item) {
			s.cache.Delete(key)
			return StoreEntry{}, ErrKeyNotFound
		}

		return s.itemToEntry(item), nil
	}

	// 캐시 미스 → 리포지토리에서 조회
	repoEntry, err := s.repo.GetEntry(ctx, key)
	if err != nil {
		return StoreEntry{}, err
	}

	// 만료된 엔트리는 반환하지 않는다
	if !repoEntry.ExpiresAt.IsZero() && time.Now().After(repoEntry.ExpiresAt) {
		return StoreEntry{}, ErrKeyNotFound
	}

	// JSON 디시리얼라이즈: 리포지토리 값은 JSON 문자열로 저장됨
	var decoded any
	if jsonStr, ok := repoEntry.Value.(string); ok {
		if err := json.Unmarshal([]byte(jsonStr), &decoded); err != nil {
			// JSON 파싱 실패 시 원래 값 사용
			decoded = repoEntry.Value
		}
	} else {
		decoded = repoEntry.Value
	}

	// 캐시에 적재
	item := &storeItem{
		value:     decoded,
		createdAt: repoEntry.CreatedAt,
		updatedAt: repoEntry.UpdatedAt,
		expiresAt: repoEntry.ExpiresAt,
		namespace: repoEntry.Namespace,
	}
	s.cache.Store(key, item)

	return s.itemToEntry(item), nil
}

// Set 은 주어진 키에 값을 저장한다.
// 캐시와 리포지토리 양쪽에 저장(Write-Through)하며, 리포지토리 실패 시 캐시를 롤백한다.
func (s *PersistentStore) Set(ctx context.Context, key string, value any) error {
	if value == nil {
		return ErrNilValue
	}
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.checkSerializable(value); err != nil {
		return err
	}

	now := time.Now()

	// 롤백을 위해 기존 값 백업
	var prevItem *storeItem
	var hadPrev bool
	if raw, ok := s.cache.Load(key); ok {
		existing := raw.(*storeItem)
		if !s.isExpired(existing) {
			cp := *existing
			prevItem = &cp
			hadPrev = true
		}
	}

	// 캐시에 새 값 저장
	newItem := &storeItem{
		value:     value,
		createdAt: now,
		updatedAt: now,
	}
	if hadPrev {
		newItem.createdAt = prevItem.createdAt
		newItem.expiresAt = prevItem.expiresAt
		newItem.namespace = prevItem.namespace
	}
	s.cache.Store(key, newItem)

	// 리포지토리에 저장 (JSON 직렬화)
	jsonBytes, _ := json.Marshal(value)
	repoEntry := &StoreEntry{
		Value:     string(jsonBytes),
		CreatedAt: newItem.createdAt,
		UpdatedAt: newItem.updatedAt,
		ExpiresAt: newItem.expiresAt,
		Namespace: newItem.namespace,
	}

	if err := s.repo.SetEntry(ctx, key, repoEntry); err != nil {
		// 리포지토리 실패 → 캐시 롤백
		if hadPrev {
			s.cache.Store(key, prevItem)
		} else {
			s.cache.Delete(key)
		}
		return fmt.Errorf("persistent store: repo set 실패: %w", err)
	}

	return nil
}

// SetWithTTL 은 주어진 키에 TTL과 함께 값을 저장한다.
// TTL이 0이면 만료 없음을 의미한다. 음수 TTL은 ErrInvalidTTL을 반환한다.
func (s *PersistentStore) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	if value == nil {
		return ErrNilValue
	}
	if ttl < 0 {
		return ErrInvalidTTL
	}
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.checkSerializable(value); err != nil {
		return err
	}

	now := time.Now()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	// 롤백을 위해 기존 값 백업
	var prevItem *storeItem
	var hadPrev bool
	if raw, ok := s.cache.Load(key); ok {
		existing := raw.(*storeItem)
		if !s.isExpired(existing) {
			cp := *existing
			prevItem = &cp
			hadPrev = true
		}
	}

	// 캐시에 새 값 저장
	createdAt := now
	namespace := ""
	if hadPrev {
		createdAt = prevItem.createdAt
		namespace = prevItem.namespace
	}

	newItem := &storeItem{
		value:     value,
		createdAt: createdAt,
		updatedAt: now,
		expiresAt: expiresAt,
		namespace: namespace,
	}
	s.cache.Store(key, newItem)

	// 리포지토리에 저장 (JSON 직렬화)
	jsonBytes, _ := json.Marshal(value)
	repoEntry := &StoreEntry{
		Value:     string(jsonBytes),
		CreatedAt: newItem.createdAt,
		UpdatedAt: newItem.updatedAt,
		ExpiresAt: newItem.expiresAt,
		Namespace: newItem.namespace,
	}

	if err := s.repo.SetEntry(ctx, key, repoEntry); err != nil {
		// 리포지토리 실패 → 캐시 롤백
		if hadPrev {
			s.cache.Store(key, prevItem)
		} else {
			s.cache.Delete(key)
		}
		return fmt.Errorf("persistent store: repo set with ttl 실패: %w", err)
	}

	return nil
}

// Delete 는 주어진 키를 캐시와 리포지토리에서 삭제한다.
// 키가 존재하지 않아도 에러를 반환하지 않는다.
func (s *PersistentStore) Delete(ctx context.Context, key string) error {
	s.cache.Delete(key)
	return s.repo.DeleteEntry(ctx, key)
}

// Rename 은 oldKey 의 엔트리를 newKey 로 이동한다.
// PersistentStore 는 히스토리 체인을 보관하지 않으므로 값 + TTL/타임스탬프만 이동한다.
//   - oldKey 없음 → ErrKeyNotFound.
//   - newKey 이미 존재 → ErrKeyExists.
func (s *PersistentStore) Rename(ctx context.Context, oldKey, newKey string) error {
	if err := s.validateKey(newKey); err != nil {
		return err
	}
	if exists, _ := s.Has(ctx, newKey); exists {
		return ErrKeyExists
	}
	repoEntry, err := s.repo.GetEntry(ctx, oldKey)
	if err != nil || repoEntry == nil {
		return ErrKeyNotFound
	}
	if !repoEntry.ExpiresAt.IsZero() && time.Now().After(repoEntry.ExpiresAt) {
		s.cache.Delete(oldKey)
		_ = s.repo.DeleteEntry(ctx, oldKey)
		return ErrKeyNotFound
	}
	if err := s.repo.SetEntry(ctx, newKey, repoEntry); err != nil {
		return err
	}
	s.cache.Delete(oldKey)
	return s.repo.DeleteEntry(ctx, oldKey)
}

// Has 는 주어진 키가 존재하고 만료되지 않았는지 확인한다.
// 캐시를 먼저 확인하고, 캐시 미스 시 리포지토리에서 확인한다.
func (s *PersistentStore) Has(ctx context.Context, key string) (bool, error) {
	// 캐시에서 먼저 확인
	if raw, ok := s.cache.Load(key); ok {
		item := raw.(*storeItem)
		if s.isExpired(item) {
			s.cache.Delete(key)
			return false, nil
		}
		return true, nil
	}

	// 캐시 미스 → 리포지토리에서 확인
	repoEntry, err := s.repo.GetEntry(ctx, key)
	if err != nil {
		return false, nil
	}

	// 만료 확인
	if !repoEntry.ExpiresAt.IsZero() && time.Now().After(repoEntry.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

// Keys 는 패턴에 일치하는 키 목록을 반환한다.
// 캐시와 리포지토리 키를 합치고 중복을 제거하며, 만료된 키는 제외한다.
func (s *PersistentStore) Keys(ctx context.Context, pattern string) ([]string, error) {
	matchAll := pattern == "" || pattern == "*"
	keySet := make(map[string]struct{})

	// 캐시에서 키 수집
	s.cache.Range(func(k, v any) bool {
		key := k.(string)
		item := v.(*storeItem)

		// 만료된 키는 건너뛴다
		if s.isExpired(item) {
			return true
		}

		if matchAll {
			keySet[key] = struct{}{}
			return true
		}

		matched, err := path.Match(pattern, key)
		if err != nil {
			return true
		}
		if matched {
			keySet[key] = struct{}{}
		}
		return true
	})

	// 리포지토리에서 키 수집 (중복 제거)
	repoKeys, err := s.repo.ListKeys(ctx, pattern)
	if err != nil {
		return nil, err
	}
	for _, k := range repoKeys {
		keySet[k] = struct{}{}
	}

	// 결과 생성
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	return keys, nil
}

// Clear 는 캐시와 리포지토리의 모든 키를 삭제한다.
func (s *PersistentStore) Clear(ctx context.Context) error {
	// 캐시 클리어
	s.cache.Range(func(k, _ any) bool {
		s.cache.Delete(k)
		return true
	})

	// 리포지토리 클리어 (빈 문자열 → 전체 삭제)
	return s.repo.ClearNamespace(ctx, "")
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 반환한다.
// PersistentStore는 현재 히스토리를 지원하지 않으므로 키 존재 여부만 확인한다.
func (s *PersistentStore) GetHistory(ctx context.Context, key string) ([]HistoryEntry, error) {
	_, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return []HistoryEntry{}, nil
}

// @spec SPEC-STORE-003
// ClearHistory 는 PersistentStore 가 히스토리 체인을 보관하지 않으므로 키 존재 여부만 확인한다.
// 키가 존재하지 않거나 만료된 경우 ErrKeyNotFound 를 반환하며, 그 외에는 no-op 으로 nil 을 반환한다.
// 엔트리 자체는 절대 삭제하지 않는다 (Store 인터페이스 계약 준수).
func (s *PersistentStore) ClearHistory(ctx context.Context, key string) error {
	if _, err := s.Get(ctx, key); err != nil {
		return err
	}
	return nil
}

// QueryHistory 는 HistoryQuery 조건에 따라 시계열 엔트리를 반환한다.
// PersistentStore 는 히스토리 체인을 보관하지 않으므로 latest 모드에서는 현재값 1건을 반환하고,
// 그 외 모드에서는 현재값 1건을 포함한 단일 시계열을 대상으로 필터링한다.
func (s *PersistentStore) QueryHistory(ctx context.Context, key string, q HistoryQuery) ([]HistoryEntry, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	entry, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	combined := []HistoryEntry{{Value: entry.Value, Timestamp: entry.UpdatedAt}}
	return filterHistoryByQuery(combined, q, time.Now()), nil
}

// LoadFromRepo 는 리포지토리의 모든 만료되지 않은 엔트리를 캐시에 로드한다.
func (s *PersistentStore) LoadFromRepo(ctx context.Context) error {
	// 리포지토리의 모든 키를 가져온다
	keys, err := s.repo.ListKeys(ctx, "*")
	if err != nil {
		return fmt.Errorf("persistent store: load keys 실패: %w", err)
	}

	now := time.Now()

	for _, key := range keys {
		repoEntry, err := s.repo.GetEntry(ctx, key)
		if err != nil {
			continue
		}

		// 만료된 엔트리는 건너뛴다
		if !repoEntry.ExpiresAt.IsZero() && now.After(repoEntry.ExpiresAt) {
			continue
		}

		// JSON 디시리얼라이즈
		var decoded any
		if jsonStr, ok := repoEntry.Value.(string); ok {
			if jsonErr := json.Unmarshal([]byte(jsonStr), &decoded); jsonErr != nil {
				decoded = repoEntry.Value
			}
		} else {
			decoded = repoEntry.Value
		}

		// 캐시에 적재
		item := &storeItem{
			value:     decoded,
			createdAt: repoEntry.CreatedAt,
			updatedAt: repoEntry.UpdatedAt,
			expiresAt: repoEntry.ExpiresAt,
			namespace: repoEntry.Namespace,
		}
		s.cache.Store(key, item)
	}

	return nil
}

// itemToEntry 는 내부 storeItem을 외부 StoreEntry로 변환한다.
func (s *PersistentStore) itemToEntry(item *storeItem) StoreEntry {
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

	return entry
}
