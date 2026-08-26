package system

import (
	"github.com/xtra/xflow/internal/fillpolicy"
)

// applyPreviousFill 은 빈(null) 버킷을 직전값으로 잇는다.
//
// 이 일은 원래 DB 가 했다 — Flux 의 `fill(usePrevious:)`, InfluxQL 의
// `FILL(previous)`. 두 방언 모두 "몇 칸까지만 이어라" 를 표현하지 못해 사용
// 기간 제한을 걸 수 없었다. 그래서 빈 버킷만 null 로 받아 오고(createEmpty)
// 이어 쓰기를 여기로 옮겼다. 판단은 내장 TSDB 와 같은 `fillpolicy` 를 쓴다.
//
// buckets 는 `normalizeSeriesBuckets` 가 **그룹 → 시각** 순으로 정렬해 둔 것을
// 전제한다. 그룹이 바뀌면 직전값과 연속 횟수를 초기화한다 — 서로 다른 시리즈의
// 값을 이어 쓰면 그건 다른 대상의 값을 베끼는 것이다.
//
// 그룹 앞머리의 null(첫 실측 이전)은 이을 상대가 없으므로 그대로 둔다.
func applyPreviousFill(
	buckets []SeriesBucket,
	groupKeys []string,
	intervalMs int64,
	limit fillpolicy.Previous,
) []SeriesBucket {
	var (
		haveGroup bool
		curGroup  string
		prev      any
		run       int64
	)
	for i := range buckets {
		sig := seriesGroupSignature(buckets[i].Tags, groupKeys)
		if !haveGroup || sig != curGroup {
			haveGroup = true
			curGroup = sig
			prev = nil
			run = 0
		}

		if buckets[i].Value != nil {
			prev = buckets[i].Value
			run = 0
			continue
		}
		if prev == nil {
			// 첫 실측 이전 — 이을 상대가 없다.
			continue
		}

		run++
		switch limit.Carry(run, intervalMs) {
		case fillpolicy.CarryPrev:
			buckets[i].Value = prev
		case fillpolicy.FillValue:
			buckets[i].Value = limit.Value
		default:
			// 비운다 — null 그대로 둔다.
		}
	}
	return buckets
}
