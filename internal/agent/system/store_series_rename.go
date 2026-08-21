package system

import (
	"context"
	"errors"
	"fmt"
)

// store_series_rename.go — 스토어 키(시리즈) rename.
//
// 디바이스와 스토어 시리즈는 직접적 관계가 없고(사용상으로만 연관), 시리즈 키는 순수한
// 스토어 식별자이다. 따라서 사용자가 키 이름을 변경할 수 있도록 지원한다. 예: 디바이스
// name 변경으로 새 시리즈가 생겼을 때, 기존 시리즈를 새 이름으로 옮겨 정리할 수 있다.

// RenameKey 는 (namespace, oldKey) 아래의 모든 시리즈(field/tags 조합 전체)를 newKey 로
// 이동한다. 각 시리즈의 값 + 히스토리 + 레지스트리 메타(DataType/Field/Tags/Source)를
// 보존한다.
//
// 정책:
//   - newKey 는 비어있지 않아야 하고 oldKey 와 달라야 한다.
//   - 대상(newKey 로 인코딩된) 시리즈 키가 하나라도 이미 존재하면 ErrKeyExists 를 반환하고
//     아무것도 변경하지 않는다(덮어쓰기 금지).
//   - oldKey 에 일치하는 시리즈가 0개면 (0, nil) 을 반환한다.
//
// namespace 빈 값은 "default" 로 정규화된다. best-effort: 이동 중 동시 만료/삭제로 인한
// ErrKeyNotFound 는 건너뛴다. 그 외 도메인 에러는 즉시 반환하며 이미 이동된 시리즈는
// 롤백하지 않는다(관리 연산).
func (a *UserStoreAgent) RenameKey(
	ctx context.Context,
	namespace, oldKey, newKey string,
) (moved int, err error) {
	if namespace == "" {
		namespace = defaultStoreNamespace
	}
	if newKey == "" {
		return 0, fmt.Errorf("store rename: newKey must not be empty")
	}
	if newKey == oldKey {
		return 0, fmt.Errorf("store rename: newKey must differ from oldKey")
	}

	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return 0, fmt.Errorf("store rename: agent is not initialized")
	}

	store := inner.ForNamespace(namespace)

	rawKeys, listErr := store.Keys(ctx, "*")
	if listErr != nil {
		return 0, listErr
	}

	// 1) oldKey 에 속한 시리즈를 수집하고 대상(newKey) 시리즈 키를 계산한다.
	type movePair struct{ oldRaw, newRaw string }
	var pairs []movePair
	for _, rawKey := range rawKeys {
		sid := decodeStorageKeyToSeries(rawKey)
		if sid.Measurement != oldKey {
			continue
		}
		newRaw := EncodeSeriesKey(SeriesID{Measurement: newKey, Field: sid.Field, Tags: sid.Tags})
		pairs = append(pairs, movePair{oldRaw: rawKey, newRaw: newRaw})
	}
	if len(pairs) == 0 {
		return 0, nil
	}

	// 2) 충돌 사전검사: 대상 키가 하나라도 이미 존재하면 전체 거부(변경 없음).
	for _, p := range pairs {
		exists, herr := store.Has(ctx, p.newRaw)
		if herr != nil {
			return 0, herr
		}
		if exists {
			return 0, ErrKeyExists
		}
	}

	// 3) 이동: 값+히스토리(Store.Rename) + 레지스트리 메타(MoveStaticKeyMeta).
	for _, p := range pairs {
		if rerr := store.Rename(ctx, p.oldRaw, p.newRaw); rerr != nil {
			if errors.Is(rerr, ErrKeyNotFound) {
				continue // 동시 만료/삭제 → 건너뜀
			}
			return moved, rerr
		}
		inner.MoveStaticKeyMeta(p.oldRaw, p.newRaw)
		moved++
	}

	return moved, nil
}
