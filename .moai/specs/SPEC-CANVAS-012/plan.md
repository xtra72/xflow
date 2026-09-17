---
id: SPEC-CANVAS-012
version: "0.2.0"
status: implemented
created: 2026-09-16
updated: 2026-09-16
author: xtra
title: "SPEC-CANVAS-012 구현 계획"
---

# SPEC-CANVAS-012 — 구현 계획

## 구현 전략

### 순서를 이렇게 잡는 이유

여섯 마일스톤의 차례는 **"화면이 거짓말하는 구간을 만들지 않는다"** 하나로 정해졌다.

핵심은 **안내문이 언제 죽는가**다. `connectorReadOnlyHint` 는 고칠 칸이 없다는 사실의
대역이므로, **칸보다 먼저 죽으면** 사용자는 칸도 안내도 없는 화면을 만난다. 그래서 그
문장은 M4 에서 **칸이 서는 그 커밋에** 죽는다 — 그 전에 죽이면 두 커밋 사이에 "없는 것을
결함으로 읽는" 구간이 생기고, 그 구간은 소스 주석이 이름으로 경고해 둔 바로 그것이다.

반대로 **좌표 셋(시작 · 끝 · 꺾임점)은 아무것도 대역하지 않는다.** 그것이 없어도 화면은
아무 약속을 어기지 않으므로 M1 에서 먼저 걷는다 — 가장 싸고 가장 독립적이다.

모델(M2) → 렌더(M3) → 칸(M4) 의 차례는 이 저장소의 통상 차례다. 뒤집으면 칸이 저장할 수
없는 값을 받거나, 저장된 값을 아무도 그리지 못하는 구간이 생긴다.

M5(백분율)를 마지막 직전에 두는 것은 그것이 **M4 가 세우는 칸까지 함께 바꾸기** 때문이다.
M4 보다 먼저 하면 연결선 칸만 옛 눈금으로 태어나 곧바로 다시 고쳐야 한다.

### 핵심 설계 결정

| # | 결정 | 근거 |
|---|------|------|
| D1 | `strokeDash` 는 이름 넷(`solid`·`dash`·`dot`·`dashDot`) | spec §결정 1 — 배열은 캐스케이드 비교 · 트윈 · UI 셋을 함께 망가뜨린다 |
| D2 | `setLineDash` 는 **선택적** 멤버 | spec §결정 2 — 필수면 스텁 공장 9 곳이 컴파일되지 않는다 |
| D3 | 무늬는 두께의 배수 | spec §결정 3 — `paintStroke` 가 이미 구한 두께를 그대로 쓴다(두 번째 측정 없음) |
| D4 | 백분율은 화면의 단위, 저장은 0..1 | spec §결정 4 — 저장을 바꾸면 기존 대시보드가 100 배 불투명해진다 |
| D5 | 파선 무늬 표는 `drawElement` 가 아니라 **잎 모듈**에 둔다 | 이름 넷과 무늬의 대응은 자료이지 그리기가 아니다; 파서(`canvasConfig`)와 렌더가 **같은 표**를 봐야 갈라지지 않는다 |
| D6 | 트윈은 `strokeDash` 를 건너뛴다 | 이산 축이다. 보간할 중간값이 없다 |

D5 를 적어 둔다: 표를 `drawElement` 안에 두면 파서가 유효한 이름을 판정하려고 렌더 모듈을
들이게 되고, 그 방향은 이 저장소가 `connectorTypes` · `anchorTypes` · `pathTypes` 로 세 번
피해 온 고리다. 잎 모듈 하나(`strokeDashTypes.ts` 가정)가 **이름 넷 · 기본값 · 무늬 표**를
함께 든다 — `pathTypes` 가 `MAX_PATH_COMMANDS` 를 든 그 자리 규율이다.

## 마일스톤

### M1 — 연결선 상세에서 좌표 셋을 걷는다 (REQ-01)

**대상**: `CanvasElementsEditor.tsx` 연결선 행 본문.

- `connectorFromLabel` · `connectorToLabel` · `connectorPointsLabel` 세 `FieldGroup` 제거
- `connectorEndText` 도우미와 `pointCount` 지역 변수가 무덮개가 되면 함께 제거
- i18n 키 제거: `connectorFromLabel` · `connectorToLabel` · `connectorPointsLabel` ·
  `connectorPointsSummary` · `connectorEndAttached` · `connectorEndFree` (ko · en 양쪽)

**남는 것**: 끊김 배지 · `connectorBrokenHint` · 순서 · 삭제 · 접기. 좌표가 아니라 상태를
말하는 것들이다.

**주의**: `canvas011*` i18n 가드가 키 목록을 **손으로 적은 리터럴**로 들고 있다. 키를
지우면서 그 목록을 함께 줄이지 않으면 "코드가 부르지 않는 키" 가드가 빨개진다 — 그것이
그 가드가 있는 이유이므로, 빨개지는 것이 정상이고 목록을 줄이는 것이 답이다.

### M2 — `strokeDash` 축을 세운다 (REQ-04 모델 · REQ-06 파서)

**대상**: 새 잎 모듈 + `canvasConfig.ts`.

- `StrokeDash` 타입 · `DEFAULT_STROKE_DASH` · `STROKE_DASH_PATTERN` 표(두께 배수)
- `ElementStyle.strokeDash?: StrokeDash`
- 파서: 모르는 값이면 **키를 버리고** 나머지 스타일을 살린다(REQ-06)
- 규칙 패치에 `canvasRules.mergePatch` 한 줄을 더한다. **[0.2.0 정정]** 0.1.0 은 여기에
  "`StylePatch` 가 `Partial<ElementStyle>` 이므로 **자동으로** 얻는다" 고 적었고 그것은
  **거짓이었다** — 그 함수는 키를 손으로 열거한다. 008 이 예고한 "규칙 패치가 함께
  넓어진다" 는 손으로 넓혀야 하는 일이며, 빠뜨리면 저장은 통과한 점선이 규칙이 한 번 맞는
  순간 조용히 실선으로 돌아간다. 열거의 전수성을 재는 가드를 함께 세운다
- 트윈: 이산 축이므로 보간 대상에서 제외(D6)

### M3 — 파선을 그린다 (REQ-04 렌더 · §결정 2 · §결정 3)

**대상**: `drawElement.ts`.

- `DrawContext2D` 에 선택적 `setLineDash?(segments: readonly number[]): void`
- `paintStroke` 가 두께를 구한 **직후** 무늬를 건다:
  `ctx.setLineDash?.(patternFor(style.strokeDash, width))`
- `solid`·미지정은 빈 배열 — `setLineDash([])` 가 실선으로 되돌리는 표준 동작이다
- `save`/`restore` 안에 있으므로 요소 사이에 무늬가 새지 않는다(canvas 상태의 일부다)

**이 한 자리가 도형과 연결선 양쪽을 덮는다**(spec §결정 6). 갈라 두려면 칠하는 함수를
둘로 나눠야 하고, 그것이 008 이래 피해 온 형상이다.

### M4 — 연결선 겉모습 칸 넷 · 안내문 사망 (REQ-02 · REQ-03)

**대상**: `CanvasElementsEditor.tsx` 연결선 행 본문.

- 색(`ColorPicker alpha clearable`) · 굵기(`number`) · 선 스타일(`select`) ·
  투명도(`number`, M5 에서 %로) — 요소 스타일 줄과 **같은 컨트롤 · 같은 차례**
- `connectorReadOnlyHint` 렌더 제거 + ko · en 키 제거
- 새 i18n: `connectorStrokeDashAria` + 이름 넷의 표시 문구 · `connectorStyleLabel`

**이 커밋에서 안내문이 죽는다** — 칸이 서는 그 커밋이다(§순서).

### M5 — 투명도를 백분율로 말한다 (REQ-05)

**대상**: 투명도 칸 **세 자리** — 요소(`canvas-*-opacity-*`) · 그룹
(`canvas-group-opacity-*`) · 규칙 표, 그리고 M4 가 세운 연결선 칸.

- `min=0 max=100 step=1`, 읽을 때 `×100`(반올림), 쓸 때 `÷100`
- 자리표시자 `"투명"` → 백분율임이 드러나는 문구로 (ko · en 양쪽)
- 환산은 **도우미 한 쌍**이 진다(`toPercent` / `fromPercent`) — 네 자리가 제각기 산술을
  적으면 그중 하나가 반올림을 다르게 하는 날이 온다

**저장은 0..1 그대로다**(D4). 이 마일스톤은 저장 형식을 **한 바이트도** 바꾸지 않으며,
그 사실을 시험이 왕복으로 고정한다.

### M6 — 뒤집힌 가드 정리와 회귀 (AC-49)

- `drawElement.connector.test.ts` 의 `absent` 목록에서 `setLineDash` **한 낱말만** 뺀다.
  나머지 셋(`quadraticCurveTo`·`arc`·`arcTo`)은 그대로 — 그 셋의 근거는 살아 있다
- 뺀 자리에 **근거 주석**을 남긴다(지우지 않고 줄인 이유). 지우면 나머지 셋을 지키던
  가드가 함께 사라진다
- `.moai/specs/SPEC-CANVAS-011/acceptance.md` AC-49 에 012 가 목록을 줄였다는 각주
- 캔버스 전량 회귀 + `tsc` + `eslint`

## 시험 전략

### 층을 건너는 시험을 반드시 둔다

편집기와 렌더러가 **각각 100% 덮여도 그 사이의 이음매는 덮이지 않는다** — 이 저장소가
이미 값을 치른 함정이다. 그래서 M4 의 칸과 M3 의 무늬를 **한 시험이 가로지른다**:
칸에서 `dot` 을 고르면 → 저장에 `strokeDash: 'dot'` 이 들어가고 → 기록 스텁의
`setLineDash` 가 `[w, 2w]` 로 불린다. 세 층을 한 단언이 꿴다.

### 스텁이 새 멤버를 갖추는 것을 고정한다

선택적 멤버의 대가는 "구현하지 않은 스텁에서 조용히 지나간다" 이다(spec §결정 2). 파선을
재는 시험의 스텁은 **반드시** `setLineDash` 를 기록해야 하며, 그 사실 자체를 AC 가 고정한다
— 고정하지 않으면 훗날 스텁이 그 멤버를 잃어도 시험이 초록으로 남는다.

### 백분율은 왕복으로 잰다

"칸에 50 을 적으면 저장이 0.5" 만 재면 반쪽이다. **저장 0.5 → 칸 50 → 저장 0.5** 왕복을
재야 환산이 한쪽만 맞는 경우를 잡는다.

### i18n 은 두 로케일을 다 본다

기본 로케일이 `ko` 라 **`en` 만 치환자가 빠져도 초록으로 통과한다** — 이 저장소가 이미
값을 치른 함정이다. 지우는 키 · 더하는 키 모두 두 로케일에서 단언한다.

### 커버리지

M2 · M3 의 순수 함수(파서 · 무늬 표)는 분기 전량. M4 · M5 의 칸은 상호작용 시험.
캔버스 전량 회귀가 기준선(현재 116 파일 / 3,885건)과 **수까지** 맞는지 확인한다 —
줄면 무언가를 조용히 지웠다는 뜻이다.

## 산출물

- `web/src/pages/dashboard/panels/canvas/` — 잎 모듈 1 신설, 수정 3~4
- `web/src/lib/i18n/{ko,en}.json` — 키 6 제거 · 키 5~6 신설
- 시험 — 신설 2~3 파일, 수정 3~5 파일
- `.moai/specs/SPEC-CANVAS-011/acceptance.md` — AC-49 각주
