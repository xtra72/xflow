package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xtra/xflow/internal/fillpolicy"
)

// 직전값 채우기가 DB 에서 Go 로 옮겨진 뒤의 동작.
//
// 옮긴 이유가 사용 기간 제한이므로, 제한이 없을 때 **종전 DB 동작과 같은지**가
// 먼저다. 그 다음이 제한이 실제로 걸리는지다.
func TestApplyPreviousFill(t *testing.T) {
	const interval = int64(60_000)

	// 값 목록을 버킷 배열로. nil 은 빈 버킷이다.
	build := func(tags map[string]string, vals ...any) []SeriesBucket {
		out := make([]SeriesBucket, len(vals))
		for i, v := range vals {
			out[i] = SeriesBucket{StartMs: int64(i) * interval, Value: v, Tags: tags}
		}
		return out
	}
	values := func(bs []SeriesBucket) []any {
		out := make([]any, len(bs))
		for i, b := range bs {
			out[i] = b.Value
		}
		return out
	}

	t.Run("제한이 없으면 끝까지 잇는다 — 종전 DB 동작과 같다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, 10.0, nil, nil, nil), nil, interval, fillpolicy.Previous{})
		assert.Equal(t, []any{10.0, 10.0, 10.0, 10.0}, values(got))
	})

	t.Run("첫 실측 이전의 빈 버킷은 그대로 둔다 — 이을 상대가 없다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, nil, nil, 10.0, nil), nil, interval, fillpolicy.Previous{})
		assert.Equal(t, []any{nil, nil, 10.0, 10.0}, values(got))
	})

	t.Run("기간을 넘기면 비운다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, 10.0, nil, nil, nil), nil, interval,
			fillpolicy.Previous{MaxMs: 60_000}) // 1칸
		assert.Equal(t, []any{10.0, 10.0, nil, nil}, values(got))
	})

	t.Run("기간을 넘기면 지정 값으로 채울 수 있다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, 10.0, nil, nil, nil), nil, interval,
			fillpolicy.Previous{MaxMs: 60_000, Overflow: fillpolicy.OverflowValue, Value: -1})
		assert.Equal(t, []any{10.0, 10.0, -1.0, -1.0}, values(got))
	})

	t.Run("실측이 다시 나오면 기간을 처음부터 다시 센다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, 10.0, nil, 20.0, nil, nil), nil, interval,
			fillpolicy.Previous{MaxMs: 60_000})
		assert.Equal(t, []any{10.0, 10.0, 20.0, 20.0, nil}, values(got))
	})

	t.Run("값 타입을 바꾸지 않는다 — 정수는 정수로 이어진다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, int64(7), nil), nil, interval, fillpolicy.Previous{})
		assert.Equal(t, []any{int64(7), int64(7)}, values(got))
	})

	t.Run("그룹이 바뀌면 직전값을 넘기지 않는다", func(t *testing.T) {
		// normalizeSeriesBuckets 가 그룹 → 시각 순으로 정렬해 둔 모습.
		a := build(map[string]string{"host": "a"}, 10.0, nil)
		b := build(map[string]string{"host": "b"}, nil, 20.0)
		got := applyPreviousFill(append(a, b...), []string{"host"}, interval, fillpolicy.Previous{})
		// b 의 첫 버킷은 a 의 10 을 물려받지 않는다 — 다른 대상의 값이다.
		assert.Equal(t, []any{10.0, 10.0, nil, 20.0}, values(got))
	})

	t.Run("그룹마다 기간을 따로 센다", func(t *testing.T) {
		a := build(map[string]string{"host": "a"}, 10.0, nil, nil)
		b := build(map[string]string{"host": "b"}, 20.0, nil, nil)
		got := applyPreviousFill(
			append(a, b...), []string{"host"}, interval, fillpolicy.Previous{MaxMs: 60_000})
		assert.Equal(t, []any{10.0, 10.0, nil, 20.0, 20.0, nil}, values(got))
	})

	t.Run("인터벌을 모르면 제한이 걸리지 않는다", func(t *testing.T) {
		got := applyPreviousFill(
			build(nil, 10.0, nil, nil), nil, 0, fillpolicy.Previous{MaxMs: 60_000})
		assert.Equal(t, []any{10.0, 10.0, 10.0}, values(got))
	})
}
