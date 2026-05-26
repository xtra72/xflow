// composite.go — composite tag value 식별과 분류 로직.
//
// schema 스캔으로 수집된 tag value 들을 다음 4 카테고리로 분류한다:
//   - Mapped: tag value 가 device_ids.json 의 composite → UUID 매핑에 존재.
//     변환 가능 (스크립트 생성 대상).
//   - Orphan: composite pattern 매칭이지만 device_ids.json 에 매핑 없음.
//     스크립트에 포함되지 않으며 운영자 검토 대상 (skip + warn).
//   - Ambiguous: 동일 UUID 가 device_ids.json 내 여러 composite 와 매핑됨
//     (다대일). 안전을 위해 변환 skip (운영자가 매핑 정리 후 재실행).
//   - UUIDAlready: tag value 자체가 UUID v4 형식. 변환 불필요 (idempotent).
//
// 본 분류는 schema 스캔과 무관하게 순수 함수로 동작하므로 단위 테스트가 쉽다.
package tsdbtags

import (
	"regexp"
	"sort"
)

// uuidPattern 은 UUID v4 형식을 매칭한다 (deviceids 패키지와 동일).
var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

// compositePattern 은 composite tag value 의 형식 매칭 정규식이다.
//
// 형식: <agent_name>:<local_id>
//   - agent_name: 알파벳/숫자/언더스코어/하이픈 (LGCNP / Century / samsung 등).
//   - local_id: 영숫자 + dot/underscore/hyphen (예: "81", "0.0.16", "indoor-1").
//
// 매칭은 휴리스틱이다 — device_ids.json 매핑 존재 여부가 결정적 권위이다.
// 본 정규식은 매핑되지 않은 composite 후보 (orphan) 검출에 사용된다.
var compositePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+:[A-Za-z0-9._-]+$`)

// IsUUID 는 s 가 UUID v4 형식이면 true 를 반환한다.
func IsUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// IsCompositeShape 는 s 가 composite 형식 (agent:local_id) 으로 보이면 true.
//
// "보임" 의 정의는 § compositePattern 의 정규식. 실제 매핑 존재 여부는 별도 검증.
func IsCompositeShape(s string) bool {
	return compositePattern.MatchString(s) && !IsUUID(s)
}

// ClassifyResult 는 tag value 분류 결과를 보관한다.
//
// 각 슬라이스는 결정론을 위해 정렬된다 (테스트 안정성 + 스크립트 출력 결정성).
type ClassifyResult struct {
	// Mapped 는 변환 가능한 (composite, UUID) 쌍이다.
	Mapped []MappedTagValue

	// Orphan 은 composite 형태이지만 매핑이 없는 tag value 들이다.
	Orphan []string

	// Ambiguous 는 다대일 매핑 등으로 변환이 안전하지 않은 entry 들이다.
	Ambiguous []AmbiguousTagValue

	// UUIDAlready 는 이미 UUID 형식인 tag value 들이다 (변환 불필요).
	UUIDAlready []string
}

// MappedTagValue 는 composite → UUID 매핑이 결정된 단일 entry 이다.
type MappedTagValue struct {
	// Composite 은 schema 에서 발견된 composite tag value (예: "lgcnp:81").
	Composite string

	// UUID 는 device_ids.json 매핑에서 조회한 UUID.
	UUID string

	// Measurements 는 본 tag value 가 발견된 measurement 명의 정렬된 집합.
	Measurements []string

	// TagKey 는 본 tag value 가 등장한 tag key (예: "device_id").
	TagKey string
}

// AmbiguousTagValue 는 변환 불가능한 (모호) tag value 이다.
type AmbiguousTagValue struct {
	Composite string
	UUID      string
	Conflicts []string // 다대일 매핑 시 충돌하는 다른 composite 들 또는 사유 설명.
	TagKey    string
}

// ScannedTagValue 는 schema 스캔으로 수집된 tag value 의 출처 정보이다.
//
// composite 분류 알고리즘이 mapped entry 의 Measurements 슬라이스를 채우려면
// 단순 tag value 집합이 아닌 (value, measurement) 매핑이 필요하다.
type ScannedTagValue struct {
	Value        string
	TagKey       string
	Measurements []string
}

// Classify 는 schema 스캔 결과 + ID 매핑을 분류한다.
//
// 알고리즘:
//  1. ID 매핑의 역방향 (UUID → []composite) 생성.
//  2. 각 scanned tag value 에 대해:
//     a. UUID 형식이면 UUIDAlready.
//     b. ID 매핑에 없고 composite shape 도 아니면 무시 (사람이 명명한 일반 tag).
//     c. ID 매핑에 없고 composite shape 이면 Orphan.
//     d. ID 매핑에 있고 역방향이 다대일이면 Ambiguous.
//     e. 그 외는 Mapped.
//  3. 결과를 결정론을 위해 정렬.
func Classify(scanned []ScannedTagValue, ids IDMapping) ClassifyResult {
	reverse := ids.Reverse()

	var res ClassifyResult
	for _, tv := range scanned {
		if IsUUID(tv.Value) {
			res.UUIDAlready = append(res.UUIDAlready, tv.Value)
			continue
		}
		uid, mapped := ids[tv.Value]
		if !mapped {
			if IsCompositeShape(tv.Value) {
				res.Orphan = append(res.Orphan, tv.Value)
			}
			// composite shape 도 아니고 매핑도 없으면 일반 tag — 무시.
			continue
		}
		// 매핑됨 — 다대일 충돌 검사.
		conflicts := reverse[uid]
		if len(conflicts) > 1 {
			others := make([]string, 0, len(conflicts)-1)
			for _, c := range conflicts {
				if c != tv.Value {
					others = append(others, c)
				}
			}
			sort.Strings(others)
			res.Ambiguous = append(res.Ambiguous, AmbiguousTagValue{
				Composite: tv.Value,
				UUID:      uid,
				Conflicts: others,
				TagKey:    tv.TagKey,
			})
			continue
		}
		// 정상 변환 대상.
		ms := make([]string, len(tv.Measurements))
		copy(ms, tv.Measurements)
		sort.Strings(ms)
		res.Mapped = append(res.Mapped, MappedTagValue{
			Composite:    tv.Value,
			UUID:         uid,
			Measurements: ms,
			TagKey:       tv.TagKey,
		})
	}

	// 결정론을 위한 정렬.
	sort.Slice(res.Mapped, func(i, j int) bool { return res.Mapped[i].Composite < res.Mapped[j].Composite })
	sort.Slice(res.Ambiguous, func(i, j int) bool { return res.Ambiguous[i].Composite < res.Ambiguous[j].Composite })
	sort.Strings(res.Orphan)
	sort.Strings(res.UUIDAlready)

	return res
}

// MappedCount 는 변환 가능한 entry 수를 반환한다.
func (r ClassifyResult) MappedCount() int { return len(r.Mapped) }

// OrphanCount 는 매핑 없는 composite shape 의 entry 수를 반환한다.
func (r ClassifyResult) OrphanCount() int { return len(r.Orphan) }

// AmbiguousCount 는 다대일 등 모호한 entry 수를 반환한다.
func (r ClassifyResult) AmbiguousCount() int { return len(r.Ambiguous) }

// UUIDAlreadyCount 는 이미 UUID 인 entry 수를 반환한다.
func (r ClassifyResult) UUIDAlreadyCount() int { return len(r.UUIDAlready) }

// HasMapped 는 변환 가능한 entry 가 1건 이상이면 true.
func (r ClassifyResult) HasMapped() bool { return len(r.Mapped) > 0 }

// HasAmbiguous 는 ambiguous 가 1건 이상이면 true.
func (r ClassifyResult) HasAmbiguous() bool { return len(r.Ambiguous) > 0 }
