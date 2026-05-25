// classify.go — composite/UUID/orphan/ambiguous 분류 로직.
//
// 각 metadata key 를 다음 카테고리 중 하나로 분류한다:
//   - already UUID: key 자체가 UUID v4 형식 → 변환 skip (idempotent).
//   - convert: composite key 가 ID repository 에 매핑되어 있고 동일 UUID 가
//     아직 메타데이터의 다른 key 로 사용되지 않음 → 변환 대상.
//   - ambiguous: composite key 의 UUID 가 이미 메타데이터의 다른 key 와
//     충돌하거나, 동일 UUID 가 ID repository 내 여러 composite 와 매핑됨.
//   - orphan: composite key 인데 ID repository 에 매핑이 없음.
package deviceids

import (
	"regexp"
	"sort"
)

// uuidPattern 은 UUID v4 형식을 매칭한다 (8-4-4-4-12 hex, version nibble 4).
//
// google/uuid 의 Validate 와 호환되는 정규식 — 도구는 google/uuid 에 의존하지
// 않고도 빠르게 형식만 검증할 수 있다.
var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

// isUUID 는 s 가 UUID v4 형식이면 true 를 반환한다.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// classifyResult 는 분류 결과를 보관한다.
//
// 각 슬라이스는 결정론을 위해 key-sorted 순으로 정렬된다 (테스트 결정성 보장).
type classifyResult struct {
	convert     []ConvertEntry
	alreadyUUID []string
	orphan      []string
	ambiguous   []AmbiguousEntry
}

// classify 는 메타데이터와 ID 매핑을 분석하여 entry 별 카테고리를 결정한다.
//
// 알고리즘:
//  1. 메타데이터 key 들을 두 그룹으로 분리 — 이미 UUID 명명 vs composite 명명.
//  2. ID 매핑 (composite → UUID) 의 역방향 매핑 (UUID → []composite) 생성.
//  3. 각 composite key 에 대해:
//     a. ID 매핑에 없으면 orphan.
//     b. 대응 UUID 가 역방향 매핑에서 여러 composite 와 충돌하면 ambiguous.
//     c. 대응 UUID 가 이미 메타데이터의 다른 key 로 존재하면 ambiguous
//     (변환 시 collision).
//     d. 그 외는 convert 대상.
func classify(meta rawMetadata, ids idMapping) classifyResult {
	// 1. UUID 이미 사용 중인 메타데이터 key 집합.
	metaUUIDs := make(map[string]struct{}, len(meta))
	for k := range meta {
		if isUUID(k) {
			metaUUIDs[k] = struct{}{}
		}
	}

	// 2. 역방향 매핑: UUID → composite slice (정렬).
	reverse := make(map[string][]string)
	for composite, uid := range ids {
		reverse[uid] = append(reverse[uid], composite)
	}
	for uid := range reverse {
		sort.Strings(reverse[uid])
	}

	// 3. 카테고리 분류 — 결정론을 위해 key-sorted 순회.
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var res classifyResult
	for _, key := range keys {
		if _, isUUIDKey := metaUUIDs[key]; isUUIDKey {
			res.alreadyUUID = append(res.alreadyUUID, key)
			continue
		}
		uid, mapped := ids[key]
		if !mapped {
			res.orphan = append(res.orphan, key)
			continue
		}
		// 매핑된 UUID 가 메타데이터에 이미 다른 key 로 존재하면 collision.
		if _, exists := metaUUIDs[uid]; exists {
			res.ambiguous = append(res.ambiguous, AmbiguousEntry{
				Composite: key,
				UUID:      uid,
				Conflicts: []string{uid + " (이미 metadata 에 존재)"},
			})
			continue
		}
		// 동일 UUID 가 ID repository 내 여러 composite 와 매핑됨 → 다대일.
		if conflicts := reverse[uid]; len(conflicts) > 1 {
			// 본 composite 자신은 conflicts 에서 제외하여 의미 있는 충돌만 노출.
			others := make([]string, 0, len(conflicts)-1)
			for _, c := range conflicts {
				if c != key {
					others = append(others, c)
				}
			}
			res.ambiguous = append(res.ambiguous, AmbiguousEntry{
				Composite: key,
				UUID:      uid,
				Conflicts: others,
			})
			continue
		}
		res.convert = append(res.convert, ConvertEntry{Composite: key, UUID: uid})
	}

	return res
}
