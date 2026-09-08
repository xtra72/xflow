---
id: SPEC-CANVAS-004
version: "0.2.0"
status: draft
created: 2026-09-08
updated: 2026-09-08
author: xtra
priority: P2
lifecycle_level: spec-first
title: "설비 심볼 라이브러리 — 원시형 합성 심볼 카탈로그 + 그룹 요소 + 겉모습 캐스케이드"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, canvas, symbol, library, hvac, facility, frontend]
---

# SPEC-CANVAS-004 — 설비 심볼 라이브러리

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-09-08 | 0.2.0 | **사용자 결정 반영 — 그룹에 완전한 겉모습 캐스케이드를 준다.** 0.1.0 은 그룹에 `style`·`rules` 를 주지 않고 그 질문을 002 로 이연했으나, 사용자가 캐스케이드 채택을 결정했다. 이에 따라 **파생 우선 4단 우선순위 법칙**(부품 규칙 패치 > 그룹 규칙 패치 > 부품 기본 스타일 > 그룹 기본 스타일)을 정의하고, `opacity` 를 유일한 예외(곱셈 합성)로 규정했으며, 규칙 표·트윈 엔진과의 닫힘(closure)을 명시했다. REQ-06 신설, REQ-01/04/05 개정. 미사용 시 001 과 결과가 동일해야 한다는 **무동작 보장**을 요구사항으로 못박았다. | xtra |
| 2026-09-08 | 0.1.0 | 최초 작성. SPEC-CANVAS-001 출시 후 "구성할 컴포넌트가 하나도 없다" 는 사용자 보고에서 출발. 범위: 코드 내장 심볼 카탈로그(9종) + `group` 노드 + 스탬핑(복사) 배치 + 부품 단위 바인딩·규칙 + 중첩 좌표 투영 + 심볼 고르기 UI. 중심 설계 결정으로 **원시형 합성(A)** 를 택하고 **이미지/SVG 에셋(B)** 를 기각했다. 백엔드 변경 없음. | xtra |

## 개요 (Overview)

SPEC-CANVAS-001 은 구현이 끝났고(브랜치 `feature/SPEC-CANVAS-001`, 테스트 전량 green) 대시보드에
`canvas` 패널 타입이 실제로 떠 있다. 그 상태에서 사용자가 패널을 써 보고 낸 보고는 하나다:
**"Canvas 패널을 구성할 컴포넌트가 하나도 없음."**

이 보고를 조사해 확인한 사실은 아래와 같다. 추정이 아니라 코드와 문서에서 확인한 것이다.

- **렌더 결함이 아니다.** 요소 편집기(`CanvasElementsEditor.tsx`)는 정상 동작하며 4종 원시형
  각각의 추가 버튼을 모두 제공한다(`addElement(kind)`, i18n `elements.kindRect/kindEllipse/kindLine/kindText`).
- **001 이 주는 구성 재료는 `rect` · `ellipse` · `line` · `text` 넷뿐이다**(`CanvasElementKind`).
- SPEC-CANVAS-001 의 개요는 이 패널의 대상 화면을 "설비 계통도, 탱크 수위 표시, 밸브 개폐 상태판"
  으로 못박았다. 그런데 밸브 하나, 펌프 하나, 실외기 하나를 **매번 사각형과 선으로 손수 조립해야**
  그 화면에 도달한다. 대상 화면과 제공 재료 사이의 거리가 실용성을 삼킨다.
- **로드맵 어디에도 심볼 라이브러리가 없다.** 002 는 캔버스 내 시각 편집기(드래그 배치·크기 조절·
  스냅·z-order·그룹·배경 에셋)이고, 003 은 애니메이션(반복 효과 + 값 구동)이다. 둘 중 어느 것도
  재사용 가능한 설비 심볼을 다루지 않는다. 즉 이것은 **구현 누락이 아니라 SPEC 계열의 공백**이다.

같은 조사에서 별개의 작은 사용성 결함 하나를 찾아 이미 고쳤다 — 빈 캔버스 안내가 "구성된 요소가
없습니다" 로만 끝나 어디서 요소를 더하는지 알려 주지 않던 문제로, 지금은 **편집 모드**와 **패널 설정**
을 이름으로 지목한다(커밋 `ccf7442b`). 본 SPEC 은 그 수정을 범위에 다시 넣지 않는다. 맥락일 뿐이다.

본 SPEC 은 그 공백을 메운다. **설비 심볼 카탈로그**를 코드에 내장하고, 사용자가 심볼 하나를 골라
캔버스에 놓으면 그것이 **부품을 가진 하나의 덩어리**로 배치되며, 각 부품이 001 의 규칙 표를 그대로
써서 상태에 따라 색·문구를 바꾼다.

### 중심 설계 결정 — 심볼은 무엇으로 만들어지는가

두 후보를 검토했고, **(A) 기존 원시형의 합성**을 채택하고 **(B) 이미지/SVG 에셋**을 기각한다.

**채택 — (A) 원시형 합성.** 심볼은 자기 로컬 정규화(0..1) 좌표계 안에 놓인 `rect`/`ellipse`/`line`/`text`
부품의 이름 붙은 묶음이다. 채택 이유는 **한 문장으로 이것이다: 규칙이 부품을 지목할 수 있어야
"밸브 열림 = 몸통이 초록으로 + 스템이 열린 자리로" 가 001 의 규칙 표만으로 표현되기 때문이다.**
그 밖에 에셋 파이프라인·업로드 수명주기·백엔드 변경이 전부 불필요하고, config 는 계속 불투명 JSON 이며,
심볼을 놓은 뒤에도 001 의 요소 편집기로 그대로 손볼 수 있다.

**기각 — (B) 이미지/SVG 에셋.** 인프라는 이미 있다(`POST/GET /api/v1/dashboard-assets`,
`internal/api/handler/dashboard_asset.go`, `web/src/pages/dashboard/panels/heatmap/svgAsset.ts`).
그림의 표현력도 (A) 보다 높다. 그럼에도 기각하는 결정적 이유는 **규칙이 심볼 전체 속성(색조·불투명도·
표시 여부)만 바꿀 수 있고 내부를 지목할 수 없다**는 것이다. 그런데 본 SPEC 이 겨냥하는 사례가 정확히
내부 지목이다 — 밸브(몸통 + 스템), 펌프(케이싱 + 임펠러 + 상태등), 실외기(본체 + 팬). **그러므로 이
한계는 (B) 를 죽인다. 완곡하게 말하지 않겠다.** 여기에 부수적 비용이 더 붙는다.

- `drawElement.ts` 의 최소 context 인터페이스 `DrawContext2D` 에는 `drawImage` 가 없다. 추가하면
  타입만 늘어나는 것이 아니라, **비동기 이미지 로드·디코드·캐시 수명주기가 001 이 의도적으로
  동기·유휴 정지로 유지한 rAF 루프 안으로 들어온다**(REQ-05 의 유휴 정지 규율과 정면으로 충돌한다).
- 자산은 인증 헤더가 필요해 `<img src>` 직접 참조가 불가능하고, JSON 으로 받아 data-URL 로 바꿔 쓴다
  (`useFloorPlanSources.ts`). 심볼마다 이 왕복이 붙는다.
- `svgAsset.ts` 헤더 주석이 문서화한 제약을 전부 물려받는다: 업로드 SVG 는 **인라인 금지, data-URL
  `<img>` 로만** 그린다(스크립트 격리), SVG 문서의 `preserveAspectRatio` 가 CSS 를 이기고,
  `naturalWidth/Height` 는 sizeless SVG 에서 원본 크기가 아니다.

**그림이 필요한 자리는 002 가 이미 맡고 있다.** 002 의 배경 에셋 레이어가 도면·사진 배경을 담당하므로,
(B) 가 주려던 표현력은 로드맵 안에서 이미 배달된다. 역할 분담이 이렇게 갈린다 — **그림은 002 의 배경이,
상태는 004 의 부품이 맡는다.** 또한 004 의 `group` 노드는 훗날 이미지 부품(`kind: 'image'`)이 필요해지면
그것을 담을 자연스러운 그릇이기도 하므로, 이 기각은 문을 닫는 결정이 아니다.

### 두 번째 설계 결정 — 배치는 복사인가 참조인가

심볼을 캔버스에 놓을 때 **정의를 복사(스탬핑)** 할 것인가, **정의를 참조**할 것인가. 이것이 본 SPEC 의
하중을 받는 갈림길이다.

**채택 — 복사(스탬핑).** 카탈로그 정의는 코드가 들고 있는 **틀**이고, 배치는 그 틀을 config 안으로
**독립된 사본**으로 찍어 넣는 일이다. config 에 `definitions[]` 는 없고 정의 참조 id 도 렌더 경로에
쓰이지 않는다. 이유 셋:

1. **놓은 뒤에도 편집된다.** 사본이므로 001 의 요소 편집기가 부품 하나하나를 그대로 손본다.
   "이 실외기만 팬을 크게" 가 특수 사례가 아니라 기본 동작이다.
2. **결손 참조라는 실패 모드가 생기지 않는다.** 참조 방식이라면 관용 파서가 "정의를 못 찾은 인스턴스"
   에 대한 정책을 발명해야 한다(버릴 것인가·자리표시자를 그릴 것인가·통째로 숨길 것인가). 사본에는
   그 질문 자체가 없다.
3. **대시보드는 살아 있는 도면이 아니라 문서다.** 사용자는 놓아 둔 심볼이 앱 업데이트로 조용히
   달라지지 않기를 기대한다.

**대가는 정확히 이것이다: 카탈로그를 나중에 고쳐도 이미 놓인 심볼은 갱신되지 않는다.** 받아들인다.
갱신이 필요한 사용자는 지우고 다시 놓는다. 이 비용을 치르는 대신 위 셋을 얻는다.

**다만 평평하게 찍지는 않는다.** 심볼을 형제 원시형 8개로 흩어 찍으면 스키마 변경이 0 이라는 이점이
있지만, 그 순간 심볼은 **정체성을 잃는다** — 통째로 옮길 수도, 지울 수도 없고, 요소 목록이 8배로
길어져 읽을 수 없게 되며, 002 의 드래그는 부품 하나씩만 집게 된다. 그것은 "손으로 조립하는 문제"를
한 발짝 뒤로 미룬 것에 지나지 않는다. 그래서 **`group` 노드 하나에 부품을 담아 찍는다**(§명세).

### 세 번째 설계 결정 — 겉모습 캐스케이드와 파생 우선 4단 법칙

그룹은 자기 `style` 과 `rules` 를 갖고, 그것이 부품으로 **흘러내린다**. "이 설비가 오프라인이면 심볼
전체를 흐리게" 를 부품 여덟 개에 따로 설정하지 않고 그룹 한 줄로 쓰기 위해서다.

캐스케이드에서 진짜 일은 필드 두 개를 더하는 것이 아니라 **우선순위 법칙을 정하는 것**이다. 예측할 수
없는 캐스케이드는 캐스케이드가 없는 것보다 나쁘다 — 사용자가 무엇을 보게 될지 모르기 때문이다.

**채택 — 파생 우선(derivation-first) 4단 법칙.** 우선순위를 값이 **어디에 있는가**(부품/그룹)가 아니라
**어떻게 파생되었는가**(상태 판정/저술)로 먼저 가른다.

| 순위 | 층 | 성격 |
|------|-----|------|
| 1 (최상) | 부품 규칙 패치 | 동적 — 상태 판정 |
| 2 | **그룹 규칙 패치** | 동적 — 상태 판정 |
| 3 | 부품 기본 스타일 | 정적 — 저술 |
| 4 (최하) | 그룹 기본 스타일 | 정적 — 저술 |

한 문장으로: **상태가 저술을 이기고, 같은 성격 안에서는 부품이 그룹을 이긴다.**

**기각 — CSS 유사 구체성 우선(specificity-first).** 부품이 모든 층에서 그룹을 이기는 순서
(부품 규칙 → 부품 기본 → 그룹 규칙 → 그룹 기본)다. 직관적으로 들리지만 **본 캐스케이드의 동기를
정면으로 무너뜨린다**: 어느 부품이 자기 `opacity` 를 저술해 두었다면, "장비 오프라인 → 심볼 전체를
흐리게" 라는 그룹 규칙이 그 부품에만 조용히 적용되지 않아 **심볼이 얼룩덜룩하게 흐려진다.** 사용자는
그룹에 규칙을 걸었는데 화면 일부가 말을 듣지 않는 것을 보게 되고, 원인은 "그 부품이 예전에 투명도를
저술했다" 는, 지금 화면 어디에도 보이지 않는 사실이다. 그래서 기각한다.

**법칙의 대가와 탈출구.** 2층이 3층을 이기므로 **그룹 규칙 패치는 부품의 저술을 덮는다** — 강력하고
동시에 무디다. 하지만 사용자에게 양쪽 탈출구가 이미 있다. (a) 그룹 규칙에 덮고 싶지 않은 속성을
넣지 않으면 된다(패치는 속성별이다). (b) 특정 부품만 그룹 규칙을 이기게 하려면 그 부품의 규칙 표에
행을 두면 된다 — 1층이 2층을 이긴다. 특히 001 의 `nodata` 행 관용구(표 맨 위에 두면 결측이 최우선)가
그대로 살아 있어, "오프라인일 때만은 부품 규칙이 이기게" 같은 요구도 새 문법 없이 표현된다.

**무동작 보장이 이 법칙의 가장 강한 근거다.** 그룹이 `style` 도 `rules` 도 갖지 않으면 2층과 4층이
비고, 식은 `부품 규칙 패치 → 부품 기본 스타일` 로 줄어든다. 그것은 001 의 `evaluateRules(값, 부품.rules,
부품.style)` 과 **정확히 같다.** 캐스케이드를 쓰지 않는 config 는 001 과 결과가 다를 수 없으며, 이는
특수 분기로 만든 것이 아니라 법칙에서 저절로 나온다(§명세, REQ-06).

### `opacity` 는 유일한 예외다 — 곱셈 합성

`opacity` 만은 위 4단 법칙을 따르지 **않고 곱셈으로 합성한다.** 그룹 0.3 위의 부품 0.9 는
"둘 중 하나가 이긴다" 가 아니라 **0.27** 이다. 사용자가 지금까지 만난 모든 체계(SVG 그룹 불투명도,
CSS 부모 `opacity`)가 그렇게 동작하고, 승자 독식으로 두면 "심볼 전체를 흐리게" 가 다시 한 번
자기 `opacity` 를 가진 부품에게 무너진다.

**이것이 유일한 예외다. 명시하지 않은 예외는 예약된 버그 보고서이므로 여기에 못박는다.**
자세한 식과 "양쪽 다 미지정이면 곱하지 않고 미지정으로 남긴다" 는 단서는 §명세에 있다.

### 범위 분할 (4개 SPEC 로 확장)

- SPEC-CANVAS-001 (완료): 패널 등록 + 도형 모델 + Canvas 2D 렌더러 + 데이터 바인딩 + 조건 규칙 표 + 설정 UI + 상태 전이 트위닝.
- **SPEC-CANVAS-004 (본 SPEC)**: 설비 심볼 라이브러리 — 코드 내장 카탈로그, `group` 노드, 스탬핑 배치, 부품 단위 바인딩·규칙, 중첩 좌표 투영, 심볼 고르기 UI.
- SPEC-CANVAS-002 (후속): 캔버스 내 시각 편집기 — 드래그 배치, 크기 조절, 정렬·스냅, z-order, 그룹 조작, 배경 에셋.
- SPEC-CANVAS-003 (후속): 애니메이션 전량(반복 효과 + 값 구동), 고급 표현식 형태, 요소 복제.

### 순서 — 004 가 002·003 보다 먼저다

**004 를 먼저 둔다.** 근거는 사용자의 보고 그 자체다. 002 는 저술을 *쾌적하게* 만들고 003 은 화면을
*생동하게* 만들지만, 지금 보고된 것은 불편도 정적임도 아니라 **놓을 것이 없다**는 것이다. 재료가 없는
채로 드래그 배치를 얹으면 사각형을 더 편하게 끄는 패널이 되고, 애니메이션을 얹으면 사각형이 깜박이는
패널이 된다. 결핍의 순서가 우선순위의 순서다.

**두 번째 근거는 결합이다.** 004 의 `group` 노드는 002 에게 **드래그·히트 테스트의 자연스러운 단위**를
선물한다. 002 가 먼저 오면 원시형 단위로 만든 선택·드래그 계층을 004 가 그룹 단위로 다시 손봐야 한다.
004 가 먼저 오면 002 는 처음부터 옳은 단위 위에 선다. 더불어 002 는 SPEC-CHART-004/005 편집 툴킷 API
안정화라는 독립적인 선행 조건을 갖고 있어(001 §위험 R5) 어차피 지금 착수할 수 없다.

**정직한 단서 하나.** 004 만으로 패널이 쾌적해지지는 않는다 — 심볼을 놓을 자리를 여전히 수치로 입력해야
하고, 심볼 여럿을 계통도로 배열하는 일은 002 의 드래그가 있어야 즐거워진다. **004 는 가능하게 만들고,
002 는 즐겁게 만든다.** 그 둘을 바꿔 말하지 않는다.

### 백엔드 변경 0

카탈로그는 **코드에 내장**되고(프론트 TS 모듈), 배치 결과는 패널 `config` 안에 들어간다. `config` 는
Go 쪽에서 불투명 JSON(`json.RawMessage`)으로 저장되므로 **신규 엔드포인트도, 스키마 변경도, 전송 계층
변경도 없다.** 자산 API(`dashboard-assets`)도 쓰지 않는다 — (B) 를 기각했기 때문이다. 다만 config 크기
축에는 실제 상한이 있으므로 그것은 가정 A3 과 위험 R2 에서 정면으로 다룬다.

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite 6/Vitest, Tailwind v4). 백엔드 변경 없음.
- 확장 대상(SPEC-CANVAS-001 산출물, `web/src/pages/dashboard/panels/canvas/`):
  - `canvasConfig.ts` — `CanvasPanelConfig { background?, tween?, elements }`, `kind` 로 판별하는 `CanvasElement` 유니온(`rect`/`ellipse`/`line`/`text`), 정규화(0..1) 기하(clamp 하지 않는다), 예외를 던지지 않는 관용 파서 `parseCanvasConfig`.
  - `canvasRules.ts` — `evaluateRules(value, rules, baseStyle) → ResolvedStyle`, first-match-wins.
  - `canvasGeometry.ts` — `projectBox`/`projectLine`/`projectPoint`(모두 `StageSize` 기준 절대 px 반환), `ellipseParams`, `resolveTextOrigin`, `labelAnchor`.
  - `canvasTween.ts` — 이징·색/수치 보간·트윈 진행. 기하 수치도 보간할 수 있게 이미 만들어져 있다.
  - `drawElement.ts` — 최소 context 인터페이스 `DrawContext2D` 위에서 그리는 얇은 층. **`drawImage` 는 없다.**
  - `CanvasSurface.tsx` — rAF 루프(유휴 정지 + 가시성 게이팅 + DPR/`ResizeObserver`). 트윈 장부와 스타일 맵을 **평평한 `el.id` 키**로 들고 있다.
  - `CanvasPanel.tsx` — `usePanelSeriesData` 바인딩, `buildCanvasFrame` 이 `Record<string, ResolvedStyle>` 를 낸다.
  - `CanvasElementsEditor.tsx`(911행) · `CanvasRuleTableEditor.tsx`(558행) — 설정 UI.
- 배열 순서가 001 의 유일한 z-order 수단이다(뒤가 위).
- 도메인 근거 자산(MVP 심볼 집합의 출처): `internal/agent/lg`(LG ICP-01/ICP-02/HVACR01/HVACR02/LGAP), `internal/agent/samsung`, `internal/agent/hvac/codes.go`, `internal/agent/modbus`, `web/src/pages/dashboard/panels/{AcControlPanel,OutdoorControlPanel,HvacControlPanel,PropertiesGridPanel}.tsx`, `acControlTypes.ts`(`AcMode` 5종 · `FanSpeed` 6종), i18n `device.type.{indoor,outdoor,sensor,controller}` · `propValve`(밸브 개도) · modbus 대량 등록 예시의 "펌프 상태".
- 저장 상한(신규 확인 사실): 대시보드 snapshot PUT 은 **256KB**(`internal/api/handler/dashboard.go` `maxDashboardPayloadBytes`). 이 예산은 **대시보드 전체가 공유**한다. 자산 API 가 분리된 이유가 바로 이것이며(도면 한 장이 예산을 삼켜 저장이 통째로 실패하는 것을 막는다), 스탬핑 방식의 config 팽창은 이 축에서 평가해야 한다.
- 신규 의존성 **0**. 심볼은 원시형 합성이므로 아이콘 라이브러리도, SVG 파서도, 식 평가기도 필요 없다.
- i18n: `web/src/lib/i18n/{ko.json,en.json}` 양쪽. 캔버스 키는 `dashboard.settings.canvas.*` 아래에 이미 자리가 있다. **키 이름 안에 점을 넣지 않는다**(프로젝트 규약).
- 신규 컴포넌트 위치: `web/src/pages/dashboard/panels/canvas/` (심볼 관련은 `canvas/symbols/` 하위).

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | 심볼 카탈로그를 **코드에 내장**하면 백엔드 변경이 불필요하다 | 확정 | 카탈로그는 TS 모듈이고 배치 결과만 불투명 JSON config 로 간다. 001 과 같은 경로 |
| A2 | 심볼 하나의 부품 수는 **10개 이하**다. 그 이상이 필요한 그림은 심볼이 아니라 002 의 배경 에셋이 맡을 일이다 | 중간 | 설계 전제. MVP 카탈로그 9종은 전부 3~6부품. 상한 초과 시 §위험 R1 |
| A3 | 스탬핑으로 늘어난 config 가 대시보드 snapshot 256KB 예산 안에 든다 | **중간(정량 검증 필요)** | 요소 1건 직렬화 ≈ 150~250B, 심볼 1개(6부품) ≈ 1~1.5KB. 심볼 30개 ≈ 45KB 로 예산 안이지만 **예산은 대시보드 전체가 공유**한다. 인수 기준 AC-E5 가 실측으로 확인한다 |
| A4 | 배치는 **복사(스탬핑)** 이고 참조가 아니다. 카탈로그 갱신은 이미 놓인 심볼에 소급되지 않는다 | 확정 | 사용자 확정 설계 결정(§개요) |
| A5 | 규칙은 **부품 단위**로 걸린다. 심볼 전체를 한 규칙 표로 다스리지 않는다 | 확정 | (B) 기각의 근거이자 (A) 채택의 근거 |
| A6 | 중첩은 **1단계뿐**이다. 그룹 안에 그룹은 오지 않는다 | 확정 | 타입으로 강제한다(§명세 — `parts: CanvasElement[]`, 그룹은 `CanvasNode` 에만 온다) |
| A7 | 그룹 박스를 비균등하게 늘리면 심볼도 함께 왜곡된다(원이 타원이 된다) | 확정 | 001 의 스테이지가 이미 그렇다(패널 리사이즈 시 요소가 함께 늘어난다). 여기서만 종횡비를 지키면 같은 패널 안에 규칙이 둘이 된다 |
| A8 | MVP 카탈로그는 **이 프로젝트가 실제로 다루는 도메인**(HVAC 공조 + Modbus 설비)에서 뽑는다. 일반 산업용 P&ID(ISA-5.1) 기호 집합이 아니다 | 확정 | 코드베이스에 화학 공정 도메인이 없다. §명세 §MVP 심볼 카탈로그가 심볼마다 출처를 댄다 |
| A9 | 사용자 정의 심볼(선택 영역을 심볼로 저장)은 **004 범위 밖**이다 | 확정 | 저장 위치 결정(config 내장 / 자산 API / 신규 엔드포인트)이 필요하고, 그 질문은 백엔드 변경 0 을 흔든다. 후속 SPEC |
| A10 | 겉모습 우선순위는 **파생 축**(동적 상태 판정 / 정적 저술)이 **위치 축**(부품 / 그룹)보다 우선한다 | 확정 | 사용자 확정 설계 결정. CSS 유사 구체성 우선은 "심볼이 얼룩덜룩하게 흐려지는" 실패로 기각(§개요) |
| A11 | `opacity` 는 4단 법칙을 따르지 않고 **곱셈 합성**한다. 이것이 유일한 예외다 | 확정 | SVG 그룹 불투명도 · CSS 부모 `opacity` 와 같은 관용. 승자 독식이면 "전체 흐리게" 가 다시 무너진다 |
| A12 | 캐스케이드는 **쓰지 않으면 아무 일도 하지 않는다**. 그룹이 `style`·`rules` 를 갖지 않은 config 는 001 과 결과가 동일하다 | 확정 | 4단 법칙에서 2·4층이 비면 식이 001 의 `evaluateRules(값, 부품.rules, 부품.style)` 로 축약된다 — 특수 분기가 아니라 법칙의 귀결(REQ-06, AC-E7) |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Optional / Unwanted)을 5개 모듈에 모두 표현한다.

### REQ-01 — 심볼 카탈로그와 그룹 노드 스키마 (Ubiquitous)

시스템은 **항상** 심볼 정의를 **코드 내장 카탈로그**(`canvas/symbols/symbolCatalog.ts`)로 들고 있어야 하며,
심볼 정의를 패널 `config` 에 저장하지 **않아야** 한다.
시스템은 **항상** 각 심볼 정의를 `{ id, labelKey, aspect?, parts[] }` 로 두고, 각 부품을
`{ role, kind, geometry, style, text?, bindable? }` 로 두어야 한다. 부품 기하는 **심볼 로컬 정규화(0..1)**
좌표다.
시스템은 **항상** 카탈로그 `id` 와 부품 `role` 을 로케일 비의존 영문 키로 두고, 표시 이름은 i18n 키를
통해서만 얻어야 한다(카탈로그가 한국어 문자열을 들고 있으면 안 된다).
시스템은 **항상** 신규 노드 종류 `group` 을 정의하고, 최상위 배열 원소 타입을
`CanvasNode = CanvasElement | GroupElement` 로 넓혀야 한다. `GroupElement` 는 **자기 `style` 과 `rules` 를
가져야 하며**(겉모습 캐스케이드의 2·4층 공급원, REQ-06), 그 둘은 모두 선택 필드여야 한다. `CanvasElement`(001 의 원시형 4종 유니온)의
정의는 **바뀌지 않아야 하며**, `GroupElement.parts` 는 `CanvasElement[]` 여야 한다 — 이로써 그룹 중첩
불가(A6)가 런타임 검사가 아니라 **타입으로** 강제된다.
시스템은 **항상** `parseCanvasConfig` 를 확장해 `group` 노드를 읽어야 하며, 확장 후에도 어떤 입력에도
예외를 던지지 않아야 한다(001 관용 파서 계약 유지).
시스템은 **항상** 신규 i18n 키를 `ko.json` 과 `en.json` **양쪽**에 추가해야 하고, 키 이름 안에 점을
넣지 않아야 한다.

### REQ-02 — 심볼 배치(스탬핑) (Event-Driven)

**WHEN** 사용자가 심볼 고르기 UI 에서 심볼 하나를 선택하면 **THEN** 시스템은 그 정의의 부품 전체를
**독립된 사본**으로 펼쳐 `group` 노드 하나를 만들고 `elements[]` 끝에 덧붙여야 한다(배열 끝 = 맨 위).
**WHEN** 그룹이 만들어지면 **THEN** 시스템은 그룹 기하를 카탈로그의 권장 종횡비(`aspect`)를 반영한
기본 배치 박스로 채우고, 부품 id 를 그룹 안에서 유일하게 발급해야 한다.
**WHEN** 스탬핑이 끝나면 **THEN** 만들어진 부품은 **일반 요소와 구별 없이** 편집 가능해야 한다 —
기하·스타일·문구·바인딩·규칙 모두 001 의 요소 편집기와 같은 수단으로 고칠 수 있어야 한다.
**WHEN** 심볼을 놓은 뒤 카탈로그 정의가 바뀌더라도 **THEN** 시스템은 이미 놓인 그룹을 갱신하지
**않아야 한다**(A4 — 사본이 곧 진실이다).
시스템은 **항상** 그룹에 출처 기록 `symbol { catalog_id, version }` 을 남겨야 하되, 렌더 경로가 그 값을
읽지 않아야 한다 — 표시·감사용이며 결손 시에도 그림은 완전하다.

### REQ-03 — 중첩 좌표 투영과 렌더 (State-Driven)

**IF** 노드가 `group` 이면 **THEN** 시스템은 그룹 기하를 스테이지 px 박스로 투영한 뒤, 각 부품의 로컬
정규화 기하를 **그 박스 안으로** 다시 투영해 그려야 한다.
**IF** 그룹 박스가 스테이지 전체(원점 0, 크기 = 스테이지)라면 **THEN** 중첩 투영 결과는 001 의
`projectBox`/`projectLine`/`projectPoint` 결과와 **같아야 한다** — 001 의 투영은 중첩 투영의 특수 사례이며,
이 항등식은 인수 기준으로 검증한다.
**IF** 그룹이 있으면 **THEN** 그리기 순서는 (1) 최상위 배열 순서, (2) 그룹 안 부품 배열 순서의 2단
순서여야 한다. 부품이 그룹 밖 요소보다 위로 올라오는 일은 없어야 한다.
**IF** 그룹 박스의 폭·높이가 비균등하게 바뀌면 **THEN** 부품도 같은 비율로 함께 늘어나야 한다(A7).
**IF** 프레임 상태(목표 스타일·문구·트윈 장부)를 키로 들고 있어야 하면 **THEN** 시스템은 부품에 대해
**복합 키**(그룹 id + 부품 id)를 쓰고, 최상위 원시형에 대해서는 001 과 동일한 평평한 `el.id` 키를
유지해야 한다 — 001 의 기존 동작이 보존된다.

### REQ-04 — 부품 단위 바인딩·규칙과 그룹 상속 (Optional)

**Where** 부품에 바인딩과 규칙 행이 설정되면 시스템은 001 의 `evaluateRules` 를 **그대로** 써서
first-match-wins 로 평가하고, 그 결과를 해당 부품에만 적용해야 한다(새 규칙 엔진을 만들지 않는다).
**Where** 그룹에 바인딩이 설정되면 시스템은 **자기 바인딩이 없는 부품에 한해** 그룹 바인딩을
물려주어야 한다. 바인딩 상속은 **한 단계**이며 부품이 자기 바인딩을 가지면 그것이 이긴다(겉모습
캐스케이드와는 별개의 축이다 — 이쪽은 값의 출처를 정하고, 저쪽은 겉모습의 우선순위를 정한다).
**Where** 그룹과 부품이 모두 규칙 표를 가지면 시스템은 **그룹을 먼저, 부품을 나중에** 평가해야 한다 —
그룹의 규칙 패치가 부품 평가의 입력(2층)이 되기 때문이다. 그룹 규칙 평가는 **프레임당 그룹 1회**여야
하며 부품 수만큼 반복되지 않아야 한다.
**Where** 그룹에 바인딩이 없으면 시스템은 `group.rules` 가 비어 있지 않더라도 그룹 규칙을 평가하지
않아야 한다(2층이 빈다) — 바인딩 없는 요소의 규칙을 평가하지 않는 001 `buildCanvasFrame` 의 규율과 같다.
**Where** 트윈이 필요하면 시스템은 `부품.tween → 그룹.tween → 패널.tween` 순의 3단 폴백으로 트윈 사양을
정해야 한다.
**Where** 부품이 카탈로그에서 `bindable: true` 로 표시되면 설정 UI 는 그 부품의 규칙 표를 먼저 펼쳐
보여야 한다 — 상태를 나타내라고 만든 부품이 무엇인지 사용자가 찾아 헤매지 않게 한다.
**Where** 사용자가 그룹을 통째로 지우면 부품 전체가 함께 사라져야 한다(그룹이 삭제·이동의 단위다).

### REQ-05 — 견고성 (Unwanted)

시스템은 다음을 **하지 않아야 한다**:
- 부품이 0개인 그룹, 알 수 없는 `role`, 알 수 없는 `catalog_id` 를 만났을 때 렌더 예외를 던지지 **않아야 한다**
  (부품 0개 그룹은 아무것도 그리지 않는 빈 그룹으로 남고, 알 수 없는 `role`/`catalog_id` 는 표시 메타일 뿐이라 그림에 영향이 없다).
- `parts` 안에 다시 `group` 이 들어온 입력을 만났을 때 **재귀 렌더로 들어가지 않아야 한다**(그 항목은 버린다).
- 그룹 안 부품 id 가 중복될 때 어느 쪽을 가리키는지 모호한 상태로 두지 **않아야 한다**(001 의 요소 id 중복 규칙과 같이 **먼저 온 것이 이긴다**).
- 001 이 정의한 `CanvasElement` 유니온·`evaluateRules` 시그니처·`projectBox`/`projectLine`/`projectPoint` 시그니처를 바꾸지 **않아야 한다**(행위 보존).
- 이미지·SVG 에셋을 그리기 위해 `DrawContext2D` 에 `drawImage` 를 추가하거나, rAF 루프 안에 비동기 로드를 들이지 **않아야 한다**(§개요의 (B) 기각).
- 심볼 미리보기를 위해 별도의 렌더 경로를 만들지 **않아야 한다** — 미리보기도 `drawElements` 를 그대로 쓴다(그리지 못하는 카탈로그 항목이 조용히 존재하는 것을 막는다).
- **구체성 우선(부품이 모든 층에서 그룹을 이기는 순서)으로 겉모습을 해석하지 않아야 한다** — 그 순서는 그룹 규칙이 일부 부품에만 적용되어 심볼이 얼룩덜룩하게 흐려지는 실패를 낳는다(§개요, A10).
- `opacity` 를 승자 독식으로 해석하지 **않아야 하며**, 반대로 그룹·부품 **양쪽 모두 미지정일 때 곱셈 결과 `1` 을 만들어 채우지도 않아야 한다**(001 은 그 자리를 미지정으로 남기고 `sameStyle` 이 `===` 로 비교하므로, 채우면 무동작 보장이 깨진다).
- 캐스케이드를 해석할 때 **조상을 키로 조회하지 않아야 한다** — 그룹의 기여는 2단 순회 중 인자로 넘긴다(§명세 §키 안전, 위험 R3).
- 그룹이 `style`·`rules` 를 갖지 않은 config 의 렌더 결과를 001 과 다르게 만들지 **않아야 한다**(무동작 보장, A12).
- 캐스케이드를 위해 `evaluateRules` 의 시그니처를 넓히거나 그 동작을 바꾸지 **않아야 한다**.
- 카탈로그 정의를 패널 config 에 복제 저장하거나, 신규 백엔드 엔드포인트를 추가하거나, 백엔드 스키마를 변경하지 **않아야 한다**.
- 신규 npm 의존성을 추가하지 **않아야 한다**.

### REQ-06 — 겉모습 캐스케이드와 우선순위 법칙 (State-Driven)

**IF** 그룹이 `style` 또는 `rules` 를 가지면 **THEN** 시스템은 각 부품의 최종 스타일을 **속성별 4단
폴백**으로 해석해야 한다: 부품 규칙 패치 → 그룹 규칙 패치 → 부품 기본 스타일 → 그룹 기본 스타일 →
(모두 없으면) 미지정. 어느 층도 그 속성을 정의하지 않으면 **미지정으로 남아야 하며**, 렌더측 기본값을
만들어 채우지 않아야 한다(001 파서의 "미지정을 만들어 채우지 않는다" 규율과 같은 이유).

**IF** 해석 대상 속성이 `opacity` 이면 **THEN** 시스템은 4단 법칙 대신 **곱셈 합성**을 적용해야 한다:
그룹측 해석값(그룹 규칙 패치 ?? 그룹 기본) × 부품측 해석값(부품 규칙 패치 ?? 부품 기본), 결과는 [0,1]
로 clamp 한다. **IF** 그룹측과 부품측이 **모두 미지정**이면 **THEN** 곱하지 않고 미지정으로 남겨야 한다.

**IF** 그룹이 `style` 과 `rules` 를 **둘 다 갖지 않으면** **THEN** 부품의 최종 스타일은
`evaluateRules(부품값, 부품.rules, 부품.style)` 의 결과와 **정확히 같아야 한다**(무동작 보장, A12).
이 동일성은 조건 분기로 특별 처리하는 것이 아니라 4단 법칙의 2·4층이 비면서 자연히 성립해야 한다.

**IF** 부품이 바인딩을 갖지 않으면 **THEN** 시스템은 그 부품의 규칙 표를 평가하지 않되(001 규율),
**그룹의 규칙 패치는 여전히 적용해야 한다** — 바인딩 없는 라벨도 그룹의 오프라인 규칙에 따라 함께
흐려져야 한다. 이것이 캐스케이드가 존재하는 이유다.

**IF** 그룹의 규칙 일치 결과가 바뀌어 N개 부품의 최종 스타일이 달라지면 **THEN** 시스템은 **실제로
값이 달라진 부품에 대해서만** 트윈을 시작해야 하며(변하지 않은 부품은 기존 트윈을 이어간다),
트윈 사양은 `부품.tween → 그룹.tween → 패널.tween` 순으로 정해야 한다. 진행 중인 트윈이 하나도 없으면
프레임을 예약하지 않는 001 의 유휴 정지 보장은 **그대로 유지되어야 한다**(REQ-05).

## 명세 (Specifications)

### config 스키마 확장

001 의 타입은 **건드리지 않고**, 최상위 배열의 원소 타입만 넓힌다.

```
// 001 그대로 — 이름·형상 불변.
CanvasElement = RectElement | EllipseElement | LineElement | TextElement

// 004 가 더하는 것.
GroupElement {
  id: string                 // 최상위 배열 안에서 유일.
  kind: 'group'
  geometry: BoxGeometry      // 스테이지 정규화(0..1). 심볼이 놓인 자리.
  parts: CanvasElement[]     // 부품. 기하는 **그룹 박스 로컬** 정규화(0..1).
                             // 타입이 CanvasElement 라 그룹은 여기 올 수 없다(A6 을 타입으로 강제).
  binding?: ElementBinding   // 부품이 물려받는 기본 바인딩(REQ-04). 없으면 상속 없음.
  style?: ElementStyle       // 캐스케이드 4층(최하) — 정적 저술. 001 과 같은 ElementStyle 이다.
  rules?: RuleRow[]          // 캐스케이드 2층 — 동적 상태 판정. binding 이 있어야 평가된다.
  tween?: TweenSpec          // 부품이 물려받는 기본 트윈. 패널 기본보다 우선.
  symbol?: SymbolStamp       // 출처 기록. 렌더는 읽지 않는다.
}

SymbolStamp { catalog_id: string; version: string }

// 최상위 배열의 원소.
CanvasNode = CanvasElement | GroupElement

CanvasPanelConfig {
  ...ChartPanelConfigBase
  background?: string
  tween?: TweenSpec
  elements: CanvasNode[]     // 001 은 CanvasElement[] 였다. 넓히기만 한다.
}
```

**그룹은 `style` 과 `rules` 를 갖되 자기 `text`·`geometry` 외의 도형 속성은 그리는 데 쓰지 않는다.**
그룹에는 그릴 도형이 없으므로 그룹의 `style` 은 **오직 부품으로 흘러내리기 위해서만** 존재한다
(캐스케이드 4층). 그룹의 `rules` 도 마찬가지로 자기를 칠하지 않고 **부품에 얹을 패치를 고르는 데만**
쓰인다(2층). 두 체계가 충돌할 때의 규칙은 아래 §겉모습 캐스케이드가 **하나의 법칙으로** 정한다 —
0.1.0 이 이 체계를 만들지 않았던 이유가 "충돌 규칙을 또 정해야 한다" 였고, 그 규칙을 정하는 것이
이 개정의 내용이다.

### 겉모습 캐스케이드 — 파생 우선 4단 법칙

**속성별로** 아래 순서를 위에서부터 훑어 처음 정의된 층의 값을 쓴다. 객체 단위가 아니라 **속성 단위**
폴백이다 — 부품이 `fill` 만 저술했다면 `stroke` 는 계속 그룹에서 내려온다.

```
resolve(prop) = 부품 규칙 패치[prop]      // 1층 — 동적, 구체
             ?? 그룹 규칙 패치[prop]      // 2층 — 동적, 광역
             ?? 부품 기본 스타일[prop]    // 3층 — 정적, 구체
             ?? 그룹 기본 스타일[prop]    // 4층 — 정적, 광역
             ?? 미지정                     // 렌더측 기본. 만들어 채우지 않는다.
```

**구현은 001 의 함수 둘을 그대로 쓴다.** 새 병합 의미론을 발명하지 않는다.

```
그룹패치   = matchRulePatch(그룹값, group.rules)                     // 신규(아래)
캐스케이드베이스 = mergePatch(mergePatch(group.style, part.style), 그룹패치)   // 4층 → 3층 → 2층
최종스타일 = evaluateRules(부품값, part.rules, 캐스케이드베이스)      // 1층을 위에 얹는다 — 001 함수 그대로
그 뒤 opacity 예외를 적용해 덮어쓴다(아래).
```

이 형태가 갖는 성질 셋이 이 설계의 근거다.

1. **`evaluateRules` 의 시그니처도 동작도 바뀌지 않는다.** 캐스케이드 전체가 그 함수의 `baseStyle`
   인자 안에서 표현된다. 즉 "부품에게 캐스케이드가 흘러들 때 `baseStyle` 은 무엇인가" 의 답은
   **4층·3층·2층을 이 순서로 겹친 것**이다.
2. **`mergePatch` 의 규율을 물려받는다** — "키는 있는데 값이 `undefined` 인 것은 덮어쓰지 않음".
   JSON 왕복이나 편집 중간 상태가 만든 `undefined` 가 아래층 값을 지우지 않는다. (`mergePatch` 는
   현재 모듈 사설이므로 **export 만 추가**한다. 시그니처·동작은 그대로다.)
3. **무동작 보장이 저절로 나온다.** 그룹이 `style`·`rules` 를 갖지 않으면
   `그룹패치 = undefined`, `group.style = {}` 이므로 캐스케이드베이스는 `{...part.style}` 이 되고,
   식은 `evaluateRules(부품값, part.rules, part.style)` — **001 그 자체**다(A12, AC-E7).

#### 신규 함수 하나 (`matchRulePatch`)

캐스케이드는 그룹의 **규칙 패치를 그룹의 기본 스타일과 분리된 채로** 필요로 한다(2층과 4층이 부품
기본 스타일 3층의 **반대편**에 있기 때문이다). 그런데 `evaluateRules` 는 둘을 이미 병합한 결과를
돌려주므로 쓸 수 없다. 그래서 형제 함수를 **하나 더한다**(기존 함수를 조용히 넓히지 않는다).

```
matchRulePatch(value, rules): StylePatch | undefined
  = rules?.find((row) => matchesRule(value, row))?.patch
```

`matchesRule` 은 001 이 이미 export 한 함수이므로 새 판정 로직이 생기지 않는다. 두 함수가 시간이
지나며 갈라지지 않도록 **합치 항등식**을 테스트로 못박는다(§좌표 중첩 투영의 항등식과 같은 규율):

```
evaluateRules(v, rules, base) === mergePatch(base, matchRulePatch(v, rules) ?? {})
```

#### `opacity` 예외 — 곱셈 합성

`opacity` 는 위 4단 법칙을 따르지 않는다. **유일한 예외이며, 그래서 여기에 따로 적는다.**

```
og = 그룹 규칙 패치.opacity ?? group.style.opacity        // 그룹측 해석값
op = 부품 규칙 패치.opacity ?? part.style.opacity         // 부품측 해석값

og 와 op 가 **둘 다 미지정**  → 결과도 미지정(곱하지 않는다)
그 밖의 경우                  → clamp01((og ?? 1) * (op ?? 1))
```

- 그룹 0.3 × 부품 0.9 = **0.27**. SVG 그룹 불투명도·CSS 부모 `opacity` 와 같은 관용이다.
- **양쪽 미지정일 때 `1` 을 만들어 채우지 않는 것이 중요하다.** 001 은 그 자리를 미지정으로 두고,
  `CanvasSurface.sameStyle` 은 `STYLE_KEYS` 를 `===` 로 비교한다. 여기서 `1` 을 채우면
  `opacity: 1 !== undefined` 가 되어 **캐스케이드를 쓰지 않는 패널에서도 트윈 판정이 달라진다** —
  무동작 보장이 깨진다. 이 한 줄이 그 보장을 지킨다.
- 각 측 안에서는 여전히 "동적이 정적을 이긴다"(규칙 패치가 기본 스타일보다 먼저다). 곱셈은
  **그룹측과 부품측 사이에서만** 일어난다.

#### 평가 순서와 닫힘 (규칙 표)

1. **그룹 먼저, 부품 나중.** 그룹의 패치가 부품 평가의 입력(2층)이므로 순서가 강제된다.
2. **그룹 평가는 프레임당 그룹 1회.** 부품 수만큼 반복하지 않는다(부품 N개여도 `matchRulePatch` 는 1회).
3. **그룹 값의 출처는 `group.binding`.** 그룹에 바인딩이 없으면 `group.rules` 가 있어도 평가하지 않는다
   (2층이 빈다) — 바인딩 없는 요소의 규칙을 평가하지 않는 001 `buildCanvasFrame` 규율과 동일하다.
4. **바인딩 없는 부품도 그룹 패치를 받는다.** 부품 자신의 규칙 표만 평가되지 않을 뿐, 2층은 부품의
   *베이스*에 얹히므로 라벨처럼 바인딩 없는 부품도 그룹의 오프라인 규칙에 함께 반응한다.
   **이것이 캐스케이드를 도입하는 이유 그 자체다.**
5. first-match-wins 는 그룹 표 안에서도, 부품 표 안에서도 001 그대로다. 표 **사이의** 우선순위만
   위 4단 법칙이 정한다.

#### 닫힘 (트윈 엔진)

- **그룹 규칙이 뒤집히면 트윈은 몇 개 시작되는가 — "값이 실제로 달라진 부품의 수"만큼이다.**
  트윈 엔진은 요소별 해석 스타일 사이를 보간하며 프레임 키로 장부를 든다. 그룹은 그릴 도형이 없어
  보간할 스타일 객체 자체가 없으므로 "그룹 트윈 하나"는 성립하지 않는다. 대신 각 부품이
  `sameStyle(live.to, target)` 판정을 거치므로, 그룹 패치가 덮지 못한 부품(예: 1층에서 이미 자기
  `fill` 을 이긴 부품)은 목표가 그대로여서 **트윈을 시작하지 않는다.**
- **유휴 정지는 그대로 유지된다.** N개 부품 트윈은 같은 `nowMs` 에 시작해 같은 창 안에서 끝나므로,
  깨어 있는 시간은 트윈 하나일 때와 같다. 늘어나는 것은 **창의 길이가 아니라 창 안의 프레임당
  보간 횟수**다. `allDone` 은 전 부품의 논리곱이며, 모두 끝나면 001 과 똑같이 프레임 예약이 멈춘다.
- **트윈 사양은 `부품.tween → 그룹.tween → 패널.tween`.** 이것은 객체 단위 폴백이며 **구체성 우선**이다.
  스타일과 반대 순서로 보이지만 모순이 아니다 — **파생 우선은 "동적 상태 판정 vs 정적 저술" 축이
  있을 때만 적용된다.** `TweenSpec` 에는 규칙에서 파생되는 형태가 없어(규칙 패치는 스타일만 담는다)
  그 축이 존재하지 않으므로, 남은 축인 구체성으로 정한다.
- `duration_ms: 0` 은 001 과 같이 즉시 전환이며 루프를 깨우지 않는다.

#### 키 안전 — 조상을 조회하지 않는다

캐스케이드가 생기면 부품의 해석 결과가 **조상에 의존**한다. 여기서 키 조회로 부모를 찾으면, 키가
어긋나는 순간 *엉뚱한 그룹의 패치가 다른 심볼에 새는* 종류의 버그가 생긴다 — 트윈이 튀는 정도가
아니라 **화면이 조용히 거짓말을 한다.**

그래서 해석은 **조상을 키로 조회하지 않는다.** 2단 순회가 그룹을 지나가면서 그 기여
(`group.style` 과 그룹 패치)를 **인자로 넘긴다.** 해석 함수는 순수하고 주변을 보지 않는다:

```
resolvePartStyle({ groupStyle, groupPatch, partStyle, partRules, partValue }) → ResolvedStyle
```

이 형태에서는 "부모를 잘못 찾는" 상태가 **표현 불가능**하다. 프레임 키(`groupId/partId`)는 결과를
담는 자리를 정할 뿐 캐스케이드 입력을 고르는 데 쓰이지 않는다. 위험 R3 의 명명된 완화가 이것이며,
인수 기준 AC-E8 이 검증한다.

### 파서 생존 (`parseCanvasConfig`)

- `parseElement` 의 `kind` 화이트리스트에 `'group'` 을 더한다. **그 외 001 의 판정 로직은 바뀌지 않는다.**
- 그룹 파싱: `id` 없으면 탈락(001 규칙 동일), `geometry` 는 `parseBoxGeometry` 재사용, `parts` 는
  **001 의 `parseElements` 를 그대로 재사용**하되 `group` 항목은 버린다(REQ-05 재귀 금지). 부품 id 중복은
  먼저 온 것이 이긴다. `parts` 가 배열이 아니면 빈 배열이다(빈 그룹은 오류가 아니다).
- `style` 은 **001 의 `parseStyle` 을 그대로 재사용**하고, `rules` 는 **001 의 `parseRules` 를 그대로
  재사용**한다 — 그룹의 스타일·규칙은 부품의 것과 같은 타입이므로 새 파서 갈래를 만들지 않는다.
- `binding`/`tween`/`symbol`/`style`/`rules` 는 결측·손상 시 그냥 미지정이다. **그룹이 `style`·`rules`
  를 갖지 않은 상태가 정상 경로**이며, 그때 캐스케이드는 무동작이다(A12).
- 파서는 `style`·`rules` 를 **만들어 채우지 않는다**. 빈 객체 `{}` 를 넣어 두면 "캐스케이드 미사용" 과
  "빈 캐스케이드" 가 구분되지 않고, `mergePatch({}, ...)` 는 무해하더라도 설정 UI 가 두 상태를 다르게
  보여 사용자를 혼란시킨다.
- **하위 호환**: 001 이 쓴 config 는 `group` 노드가 없으므로 아무 변화 없이 읽힌다.
- **하향 호환(다운그레이드)**: 004 가 쓴 config 를 001 시절 파서가 읽으면 `group` 노드는
  **조용히 버려지고** 나머지 요소는 정상 렌더된다 — 크래시가 아니라 부분 손실이다. 앱과 파서가 함께
  배포되므로 실사용 경로는 아니지만, 관용 파서의 성질로서 명시한다.

### 좌표 중첩 투영

`canvasGeometry.ts` 에 **박스 안 투영** 3종을 더한다. 001 의 3종은 시그니처·동작 모두 그대로 둔다.

```
projectBoxIn(geo: BoxGeometry,  box: PxBox): PxBox
projectLineIn(geo: LineGeometry, box: PxBox): PxLine
projectPointIn(geo: PointGeometry, box: PxBox): PxPoint
```

각각 `원점 오프셋 + 박스 크기 배율`이다. 그리고 다음 **항등식**이 성립해야 한다(인수 기준 AC-02):

```
projectBoxIn(geo, { x:0, y:0, w:stage.width, h:stage.height }) === projectBox(geo, stage)
```

즉 001 의 투영은 "스테이지 전체를 박스로 삼은 중첩 투영"이다. 이 항등식을 테스트로 못박아 두면 두
경로가 나중에 갈라지지 않는다. 0 크기·음수 크기 박스에 대한 방어(NaN 을 흘리지 않는다)는 001 의
`positiveOrZero`/`finite` 규율을 그대로 쓴다.

### 프레임 키 (복합 키)

001 은 목표 스타일·문구·트윈 장부를 `Record<string, _>` 에 **평평한 `el.id`** 로 담는다. 중첩이 생기면
부품 키가 필요하다.

```
frameKey(nodeId)            → nodeId              // 최상위 원시형 — 001 과 동일. 기존 동작 보존.
frameKey(groupId, partId)   → `${groupId}/${partId}`   // 부품
```

부품 id 는 그룹 안에서만 유일하면 되고, 복합 키가 전역 유일성을 만든다. 이 결정으로 001 의 세 지점만
바뀐다: `buildCanvasFrame`(키 생성), `CanvasSurface.advance`(트윈 장부 순회 + 사라진 키 청소),
`drawElements`(부품 순회). **세 지점 모두 최상위 원시형에 대해서는 이전과 같은 키를 낸다.**

### MVP 심볼 카탈로그 (9종)

**이 프로젝트가 실제로 다루는 도메인에서 뽑았다.** 이 코드베이스는 HVAC 공조(LG ICP-01/ICP-02/HVACR01/
HVACR02/LGAP, Samsung NASA)와 Modbus 설비를 모델링한다. 일반 산업용 P&ID 기호 집합은 여기 도메인이 아니다.

| id | 이름(ko) | 부품(role) | 채택 근거(코드베이스 출처) |
|----|----------|-----------|---------------------------|
| `hvac-indoor-unit` | 실내기 | `body`(rect) · `grille`(line×2) · `label`(text) | 디바이스 타입 열거 `indoor`, i18n `device.type.indoor`, AcControlPanel |
| `hvac-outdoor-unit` | 실외기 | `body`(rect) · `fan`(ellipse) · `blade`(line×2) · `label`(text) | 디바이스 타입 `outdoor`, `OutdoorControlPanel.tsx`, i18n "실외기 상태" |
| `hvac-compressor` | 압축기 | `shell`(ellipse) · `base`(rect) · `label`(text) | OutdoorControlPanel 설명문 "실외기 운전 상태와 **압축기** · 팬 지표" |
| `hvac-fan` | 팬 / 송풍기 | `ring`(ellipse) · `blade`(line×3) · `hub`(ellipse) | 같은 출처의 "팬 지표". 풍량 `FanSpeed` 6단계가 1급 도메인 개념(`acControlTypes.ts`) |
| `valve-two-way` | 밸브(2방) | `bodyA`·`bodyB`(line 나비형) · `stem`(line) · `handle`(rect) · `label`(text) | PropertiesGrid i18n `propValve: "밸브 개도"`, Modbus 설비 제어 |
| `pump` | 펌프 | `casing`(ellipse) · `base`(rect) · `port`(line) · `lamp`(ellipse) | Modbus 대량 등록 예시 문자열에 "**펌프 상태**" 가 실제로 들어 있다(i18n `bulkPlaceholder`) |
| `sensor-temp` | 온도 센서 | `bulb`(ellipse) · `stem`(rect) · `value`(text) | 디바이스 타입 `sensor`, 온도가 이 도메인의 지배적 계측치(현재 온도/설정 온도) |
| `pipe-segment` | 배관 · 덕트 구간 | `line`(line) · `arrowA`·`arrowB`(line) | 위 심볼들을 **계통도로 잇는 연결 조직**. `FacilityLinePanel.tsx` 가 라인 개념의 선례 |
| `status-lamp` | 상태 램프 | `lamp`(ellipse) · `ring`(ellipse) · `label`(text) | online/offline 이 모든 패널이 공유하는 상태축(i18n `status.online/offline`). 어떤 심볼 옆에도 붙는 최범용 바인딩 대상 |

`bindable: true` 로 표시하는 부품(설정 UI 가 규칙 표를 먼저 펼치는 부품): `hvac-outdoor-unit.fan`,
`hvac-compressor.shell`, `hvac-fan.ring`, `valve-two-way.stem`, `pump.lamp`, `sensor-temp.value`,
`pipe-segment.line`, `status-lamp.lamp`, `hvac-indoor-unit.body`.

카탈로그 id 에 **점을 쓰지 않는다** — 표시 이름 i18n 키를 id 에서 파생시킬 때 점이 섞이면 키 이름 안에
점이 들어가고, 그것은 프로젝트가 이미 가드로 막은 형상이다.

### 심볼 고르기 UI

- `CanvasSymbolPicker.tsx` — 카탈로그 격자. 각 칸은 **작은 `<canvas>` 미리보기 + i18n 이름**이다.
- 미리보기는 **`drawElements` 를 그대로 호출**해 그린다(REQ-05). 별도 썸네일 이미지도, 별도 렌더 경로도
  만들지 않는다. 그리지 못하는 카탈로그 항목이 있으면 미리보기에서 즉시 드러난다.
- 요소 편집기(`CanvasElementsEditor.tsx`)에는 기존 4종 추가 버튼 옆에 **"심볼 추가"** 진입점을 둔다.
  그룹 행은 접힘/펼침으로 부품 목록을 드러내며, 부품 행은 기존 요소 행과 **같은 편집 컨트롤**을 쓴다.
- 설정 다이얼로그(`PanelSettingsDialog.tsx`)에는 마운트 지점만 더한다(001 §위험 R4 와 같은 규율).

### 컴포넌트 구조 (신규 · 수정)

신규 (`web/src/pages/dashboard/panels/canvas/`):
- `symbols/symbolTypes.ts` — `SymbolDef` · `SymbolPart` · `SymbolStamp` 타입. DOM 무의존.
- `symbols/symbolCatalog.ts` — MVP 9종 정의. 순수 데이터. DOM 무의존.
- `symbols/stampSymbol.ts` — 정의 → `GroupElement` 스탬핑(사본 생성 + 부품 id 발급 + 기본 배치 박스). 순수 함수.
- `symbols/CanvasSymbolPicker.tsx` — 카탈로그 격자 + `drawElements` 기반 미리보기.
- `canvasNode.ts` — `CanvasNode` 판별 도우미(`isGroup`), `frameKey`, 부품 순회. 순수 모듈.
- `canvasCascade.ts` — **겉모습 캐스케이드 해석기**. `resolvePartStyle({ groupStyle, groupPatch, partStyle,
  partRules, partValue })` 와 `opacity` 곱셈 합성. 조상을 조회하지 않는 순수 함수. DOM 무의존.

수정(행위 보존):
- `canvasConfig.ts` — `GroupElement`(`style`·`rules` 포함)/`CanvasNode` 추가, `parseCanvasConfig` 확장
  (`parseStyle`·`parseRules` 재사용). **기존 타입 불변.**
- `canvasRules.ts` — `matchRulePatch` **추가** + `mergePatch` **export 추가**.
  `evaluateRules`·`matchesRule` 은 시그니처·동작 **모두 불변**. 합치 항등식 테스트로 갈라짐 방지.
- `canvasGeometry.ts` — `projectBoxIn`/`projectLineIn`/`projectPointIn` 추가. **기존 3종 불변.**
- `drawElement.ts` — 그룹 분기 + 부품 2단 순회. `DrawContext2D` **불변**(`drawImage` 없음).
- `CanvasPanel.tsx` — `buildCanvasFrame` 이 복합 키를 내고, 그룹 바인딩 상속 + **그룹 1회 규칙 평가 후
  부품 캐스케이드 해석**(2단 순회로 그룹 기여를 인자 전달)을 적용.
- `CanvasSurface.tsx` — 트윈 장부가 복합 키를 쓰고, 3단 트윈 폴백을 적용.
- `CanvasElementsEditor.tsx` — 심볼 추가 진입점 + 그룹/부품 중첩 행 + **그룹 기본 스타일·그룹 규칙 표
  편집**(부품 행과 같은 컨트롤 재사용). 어느 속성이 어느 층에서 왔는지 보이도록 캐스케이드로 내려온
  값은 "그룹에서 상속됨" 으로 표시한다.
- `i18n/{ko,en}.json` — 심볼 이름·부품 이름·고르기 UI 문구.

### 추적성 (Traceability)

- REQ-01 → `symbols/symbolTypes.ts` · `symbols/symbolCatalog.ts` · `canvasConfig.ts`(`GroupElement`/`CanvasNode`/파서) · `i18n/{ko,en}.json`
- REQ-02 → `symbols/stampSymbol.ts` · `symbols/CanvasSymbolPicker.tsx` · `CanvasElementsEditor.tsx`
- REQ-03 → `canvasGeometry.ts`(`project*In`) · `canvasNode.ts`(`frameKey`) · `drawElement.ts` · `CanvasSurface.tsx`
- REQ-04 → `CanvasPanel.tsx`(상속 · `evaluateRules` 재사용) · `CanvasSurface.tsx`(3단 트윈 폴백) · `CanvasRuleTableEditor.tsx`(부품 규칙 표)
- REQ-05 → `canvasConfig.ts`(재귀 차단 · 중복 id) · `drawElement.ts`(빈 그룹) · `symbols/CanvasSymbolPicker.tsx`(미리보기가 렌더 경로 재사용) · `canvasCascade.ts`(구체성 우선 금지 · opacity 미지정 보존 · 조상 조회 금지)
- REQ-06 → `canvasCascade.ts`(4단 법칙 · `opacity` 곱셈) · `canvasRules.ts`(`matchRulePatch` · `mergePatch` export) · `CanvasPanel.tsx`(그룹 1회 평가 + 2단 순회 인자 전달) · `CanvasSurface.tsx`(변경된 부품만 트윈 · 3단 트윈 폴백 · 유휴 정지 유지) · `CanvasElementsEditor.tsx`(그룹 스타일·규칙 편집)
