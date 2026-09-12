// dashboard_budget.go — 캔버스 요소 상한에서 대시보드 페이로드 예산을 유도한다.
//
// **관계가 뒤집혔다.** 예전에는 고정 예산(256KB)이 있고 가져오기 상한이 그 아래에
// 머물러야 했다. 이제는 운영자가 **몇 개짜리 도면을 들일지** 정하고, 서버가 그 수에
// 필요한 자리를 확보한다. 사용자의 결정이며(SPEC-CANVAS-007 §결정 14), 그 뒤집힘이
// 이 파일의 존재 이유다.
//
// 이 파일이 순수 함수만 담는 것에 뜻이 있다 — viper 도 config 인스턴스도 보지 않으므로
// 바닥값·천장값·중간값을 시험이 직접 부를 수 있다.
package config

// DefaultMaxCanvasElements 는 한 번의 SVG 가져오기가 만들 수 있는 요소 수의 기본 상한이다.
//
// 1024 는 SPEC-CANVAS-007 0.7.0 이 프론트엔드에 못박아 둔 그 수다. 기본값을 그대로 두는
// 것이 요점이다 — 설정을 쓰지 않는 기존 설치는 이 변경으로 **아무것도 달라지지 않는다**.
const DefaultMaxCanvasElements = 1024

// canvasElementBudgetBytes 는 요소 하나에 확보하는 바이트다.
//
// **이 수가 이 파일에서 유일하게 고른 값이므로 고른 근거를 적는다.** 실측 밀도는
// 문서의 모양마다 다르고, 그 차이가 열 배에 달한다(측정: 문서 → 계획 → 요소 →
// `JSON.stringify` 의 UTF-8 바이트. `web/src/pages/dashboard/panels/canvas/svgimport/
// svgImportNative.test.ts` §예산 이 표를 `toBe` 로 못박는다).
//
//	사용자 실파일(draw.io · 60 요소 · 18,386 B)      ~  306 B/요소
//	원시형 1024개(사각 — 칠과 선을 둘 다 말함)       ~  150 B/요소
//	경로 1024개 × 10 명령                            ~  789 B/요소
//	문구 1024개 × 256자 한글                         ~  898 B/요소
//	섞은 최악(경로 40개가 명령 10240 + 문구 984개)   ~1,542 B/요소
//
// **1024(1 KiB)를 고른다.** 두 끝을 다 기각한 자리다.
//
//   - **306 을 고르면** 실파일 밀도에 딱 맞아 여유가 0 이다 — 같은 도구가 낸 조금 더 촘촘한
//     도면이 곧바로 413 을 받는다. 실측 한 점에 천장을 붙이는 것은 재지 않은 것과 같다.
//   - **1,542 를 고르면** 최악이 덮이지만 기본 1024 개에 약 1.5 MB 를 요구한다. 그 최악은
//     도구가 내는 그림이 아니라 **손으로 지어야 나오는 모양**이고(명령을 가장 적은 경로에
//     몰고 남은 자리를 256자 한글로 채운다), 그 하나를 위해 모든 설치가 여섯 배를 예약한다.
//   - **1024 는 위 다섯 중 넷을 덮는다** — 원시형 · 실파일 · 경로 · 문구. 덮지 못하는 것은
//     의도적으로 섞은 마지막 줄 하나뿐이다. 곧 **한 종류로 채운 문서의 최악(문구 898)까지는
//     덮고**, 두 상한의 틈을 노려 섞은 것만 거절한다. 그 선이 "도구가 낼 수 있는 것" 과
//     "지어야 나오는 것" 의 경계에 가장 가깝다.
//
// 여유는 실파일 밀도의 **3.3배**다. 그리고 이 수는 예산을 늘리는 대가로 확보되는 것이므로
// 공짜가 아니다 — 요소당 바이트를 올리면 같은 요소 수에 더 큰 본문을 받아들이게 되고,
// 그것은 곧 DoS 표면이다. 아래 천장이 그 표면을 닫는다.
const canvasElementBudgetBytes = 1024

// MinDashboardPayloadBytes 는 유도된 예산의 **바닥**이다(오늘 값 256KB).
//
// 요소 수를 작게 설정해도 예산이 이 아래로 내려가지 않는다. 이유는 하나다 — **이미 동작하던
// 것이 설정 때문에 깨지면 안 된다.** 캔버스를 쓰지 않는 대시보드도 이 예산 안에서 저장되어
// 왔고, 요소 수를 줄인 운영자가 원한 것은 "가져오기를 죄는 것" 이지 "기존 대시보드를 저장
// 못 하게 하는 것" 이 아니다.
const MinDashboardPayloadBytes int64 = 256 * 1024

// MaxDashboardPayloadBytes 는 유도된 예산의 **천장**이다(8 MiB).
//
// **페이로드 상한은 DoS 방어물이고, 그 성질은 이 변경을 넘어 살아남아야 한다.** 설정에서
// 오는 수는 오타 하나로 10 억이 될 수 있으며, 천장이 없으면 그 오타가 "서버가 무제한 본문을
// 받는다" 가 된다. `readLimitedBody` 는 `maxBytes+1` 까지를 메모리에 올리므로 이 천장이 곧
// 요청 하나가 먹을 수 있는 메모리의 상한이다.
//
// 8 MiB 인 이유: 기본 예산(1 MiB)의 여덟 배이고, 요소 8192 개 — 사용자 실파일 밀도로
// 화면 백 장이 넘는 도면 — 까지를 덮는다. 그보다 큰 대시보드는 한 장으로 다룰 것이 아니다.
const MaxDashboardPayloadBytes int64 = 8 * 1024 * 1024

// MaxCanvasElementsCeiling 은 요소 수 자체의 천장이다.
//
// 예산 천장이 바로 이 수에서 닿는다(8192 × 1024 B = 8 MiB). 둘을 따로 고르지 않고 한쪽에서
// 유도한 것이 요점이다 — 따로 고르면 "예산은 천장인데 요소는 더 받는다"(저장이 조용히
// 실패한다) 또는 그 반대(확보한 자리를 쓸 수 없다)가 생긴다.
const MaxCanvasElementsCeiling = MaxDashboardPayloadBytes / canvasElementBudgetBytes

// ClampMaxCanvasElements 는 설정에서 온 요소 수를 쓸 수 있는 범위로 죈다.
//
// 0 이하(미설정·오타·음수)는 기본값으로 되돌리고, 천장을 넘는 값은 천장으로 자른다.
// **거절하지 않고 죄는 이유**: 이 값은 시작 설정이므로 거절하면 서버가 뜨지 않고, 오타 하나로
// 데몬이 죽는 것은 오타가 부를 대가로 지나치다.
func ClampMaxCanvasElements(n int) int {
	if n <= 0 {
		return DefaultMaxCanvasElements
	}
	if int64(n) > MaxCanvasElementsCeiling {
		return int(MaxCanvasElementsCeiling)
	}
	return n
}

// DeriveDashboardPayloadBytes 는 요소 수에서 대시보드 PUT 페이로드 예산을 유도한다.
//
// `죈 요소 수 × canvasElementBudgetBytes` 를 [MinDashboardPayloadBytes,
// MaxDashboardPayloadBytes] 로 자른다. 입력을 먼저 죄므로 어떤 정수가 들어와도
// 반환값은 그 구간 안이다(오버플로 포함 — 죈 뒤 최대가 8192 이므로 곱이 넘칠 수 없다).
func DeriveDashboardPayloadBytes(maxCanvasElements int) int64 {
	budget := int64(ClampMaxCanvasElements(maxCanvasElements)) * canvasElementBudgetBytes
	if budget < MinDashboardPayloadBytes {
		return MinDashboardPayloadBytes
	}
	if budget > MaxDashboardPayloadBytes {
		return MaxDashboardPayloadBytes
	}
	return budget
}
