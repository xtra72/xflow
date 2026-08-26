// Package fillpolicy 는 빈 버킷 채우기의 **정책**만 담는다.
//
// 채우기 자체는 두 곳에서 일어난다 — 내장 TSDB 는 Go 안에서(`internal/tsdb`),
// InfluxDB 소스는 결과 버킷을 받은 뒤(`internal/agent/system`). 순회 방식은
// 자료 구조가 달라 각자 다르지만, "직전값을 언제까지 이어 쓰고 그 뒤엔 무엇을
// 넣는가" 라는 **판단은 하나여야** 한다. 두 벌로 두면 한쪽만 고쳐져 같은 설정이
// 소스에 따라 다르게 그려진다.
package fillpolicy

// Overflow 는 사용 기간을 넘긴 버킷의 처리 방식이다.
type Overflow string

const (
	// OverflowEmpty 는 기간을 넘기면 값을 비운다(null). 기본값이다 —
	// 비워 두면 그 구간이 결측으로 남아 점선 표기와도 자연스럽게 이어진다.
	OverflowEmpty Overflow = ""
	// OverflowValue 는 기간을 넘기면 지정한 값으로 채운다.
	OverflowValue Overflow = "value"
)

// Previous 는 직전값 채우기(carry-forward)의 사용 기간 제한이다.
//
// 제로값이 곧 종전 동작이다 — MaxMs 가 0 이면 제한 없이 계속 이어 쓴다. 새 필드를
// 넣지 않은 기존 요청·기존 대시보드가 그대로 동작하게 하려는 의도다.
type Previous struct {
	// MaxMs 는 직전값을 이어 쓸 수 있는 최대 기간(ms)이다. 0 이면 무제한.
	//
	// 버킷 개수가 아니라 시간인 이유: 인터벌을 바꿔도 "직전값을 최대 5분까지
	// 쓴다" 는 뜻이 그대로 유지된다. 개수로 두면 인터벌을 절반으로 줄이는
	// 순간 실제 지속 시간도 절반이 되어, 사용자가 건드리지 않은 설정의
	// 의미가 조용히 바뀐다.
	MaxMs int64
	// Overflow 는 기간을 넘긴 버킷의 처리다.
	Overflow Overflow
	// Value 는 Overflow 가 OverflowValue 일 때 채울 값이다.
	Value float64
}

// Unlimited 는 제한이 없는지(종전 동작인지) 알려준다.
func (p Previous) Unlimited() bool { return p.MaxMs <= 0 }

// MaxRun 은 인터벌 기준으로 직전값을 이어 쓸 수 있는 **연속 버킷 수**다.
// 0 이면 무제한이다.
//
// 내림이 아니라 올림이다 — "최대 5분" 에 인터벌 2분이면 2칸(4분)이 아니라
// 3칸(6분)을 쓴다. 사용자가 적은 기간은 상한이지 정확한 배수가 아니고,
// 내림하면 인터벌이 기간을 나누어떨어지지 않을 때 체감 기간이 항상 짧아진다.
func (p Previous) MaxRun(intervalMs int64) int64 {
	if p.Unlimited() || intervalMs <= 0 {
		return 0
	}
	run := p.MaxMs / intervalMs
	if p.MaxMs%intervalMs != 0 {
		run++
	}
	// 기간이 인터벌보다 짧아도 최소 한 칸은 이어 쓴다. 0 으로 두면 "제한을
	// 걸었더니 직전값 채우기가 통째로 꺼졌다" 로 보이는데, 그건 채우기를
	// 끄는 것이지 기간을 제한하는 것이 아니다.
	if run < 1 {
		run = 1
	}
	return run
}

// Action 은 한 빈 버킷에 무엇을 넣을지에 대한 판단이다.
//
// 값의 **타입에 의존하지 않는다** — 내장 TSDB 는 float64 를, InfluxDB 경로는
// `any` 를 다루는데, 판단을 값과 분리해 두어야 두 경로가 같은 규칙을 쓴다.
type Action int

const (
	// CarryPrev — 직전값을 그대로 쓴다(기간 안).
	CarryPrev Action = iota
	// FillValue — 기간을 넘겼고 지정 값으로 채운다.
	FillValue
	// LeaveEmpty — 기간을 넘겼고 비운다(null).
	LeaveEmpty
)

// Carry 는 **연속 run 번째**(1부터) 채움에서 무엇을 할지 판단한다.
func (p Previous) Carry(run int64, intervalMs int64) Action {
	max := p.MaxRun(intervalMs)
	if max == 0 || run <= max {
		return CarryPrev
	}
	if p.Overflow == OverflowValue {
		return FillValue
	}
	return LeaveEmpty
}

// FillAt 은 직전값 prev 를 **연속 run 번째**(1부터)로 채울 때 넣을 값을 정한다.
//
// ok=false 면 그 자리는 비운다(null). 기간 안이면 prev 를, 기간을 넘겼고
// 지정 값 채우기면 그 값을 돌려준다.
func (p Previous) FillAt(prev float64, run int64, intervalMs int64) (float64, bool) {
	switch p.Carry(run, intervalMs) {
	case CarryPrev:
		return prev, true
	case FillValue:
		return p.Value, true
	default:
		return 0, false
	}
}
