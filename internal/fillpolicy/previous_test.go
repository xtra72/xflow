package fillpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrevious_제로값은_종전동작(t *testing.T) {
	var p Previous
	assert.True(t, p.Unlimited())
	assert.Zero(t, p.MaxRun(60_000))

	// 몇 번째든 직전값을 그대로 쓴다.
	for _, run := range []int64{1, 10, 10_000} {
		v, ok := p.FillAt(7, run, 60_000)
		assert.True(t, ok)
		assert.Equal(t, 7.0, v)
	}
}

func TestPrevious_MaxRun(t *testing.T) {
	t.Run("나누어떨어지면 그 몫", func(t *testing.T) {
		assert.Equal(t, int64(5), Previous{MaxMs: 300_000}.MaxRun(60_000))
	})

	t.Run("나머지가 있으면 올린다 — 상한이지 배수가 아니다", func(t *testing.T) {
		// 5분 제한 + 2분 인터벌 → 2칸(4분)이 아니라 3칸(6분).
		assert.Equal(t, int64(3), Previous{MaxMs: 300_000}.MaxRun(120_000))
	})

	t.Run("기간이 인터벌보다 짧아도 한 칸은 쓴다", func(t *testing.T) {
		// 0 이면 채우기가 통째로 꺼진 것처럼 보인다 — 그건 다른 설정이다.
		assert.Equal(t, int64(1), Previous{MaxMs: 1_000}.MaxRun(60_000))
	})

	t.Run("인터벌을 모르면 제한할 수 없다", func(t *testing.T) {
		assert.Zero(t, Previous{MaxMs: 300_000}.MaxRun(0))
	})
}

func TestPrevious_FillAt(t *testing.T) {
	const interval = 60_000

	t.Run("기간 안이면 직전값", func(t *testing.T) {
		p := Previous{MaxMs: 180_000} // 3칸
		for run := int64(1); run <= 3; run++ {
			v, ok := p.FillAt(7, run, interval)
			assert.True(t, ok, "run=%d", run)
			assert.Equal(t, 7.0, v)
		}
	})

	t.Run("기간을 넘기면 기본은 비움", func(t *testing.T) {
		p := Previous{MaxMs: 180_000}
		_, ok := p.FillAt(7, 4, interval)
		assert.False(t, ok)
	})

	t.Run("기간을 넘기면 지정 값으로 채울 수 있다", func(t *testing.T) {
		p := Previous{MaxMs: 180_000, Overflow: OverflowValue, Value: -1}
		v, ok := p.FillAt(7, 4, interval)
		assert.True(t, ok)
		assert.Equal(t, -1.0, v)
	})

	t.Run("지정 값이 0 이어도 채운다 — 비움과 다르다", func(t *testing.T) {
		p := Previous{MaxMs: 180_000, Overflow: OverflowValue, Value: 0}
		v, ok := p.FillAt(7, 9, interval)
		assert.True(t, ok)
		assert.Equal(t, 0.0, v)
	})

	t.Run("인터벌을 모르면 제한이 걸리지 않는다", func(t *testing.T) {
		p := Previous{MaxMs: 1_000, Overflow: OverflowValue, Value: -1}
		v, ok := p.FillAt(7, 999, 0)
		assert.True(t, ok)
		assert.Equal(t, 7.0, v)
	})
}
