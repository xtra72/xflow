---
id: SPEC-CANVAS-001
version: "0.1.0"
status: draft
created: 2026-09-08
updated: 2026-09-08
author: xtra
priority: P2
lifecycle_level: spec-first
title: "Canvas Panel (MVP) — 도형 구성 + 조건 규칙 표 + 상태 전이 트위닝 대시보드 패널"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, canvas, shapes, animation, binding, rules, frontend]
---

# SPEC-CANVAS-001 — 캔버스 패널 (MVP)

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-09-08 | 0.1.0 | 최초 작성. MVP 범위: 패널 타입 등록 + 도형 모델 + Canvas 2D 렌더러 + 데이터 바인딩 + 조건 규칙 표 + 설정 UI + 상태 전이 트위닝. 요소 배치는 설정 다이얼로그 수치 입력으로 한다. 캔버스 내 시각 편집기(002), 반복 효과·값 구동 애니메이션(003)은 후속 SPEC 으로 분리. | xtra |

## 개요 (Overview)

대시보드에 신규 PanelType `canvas` 를 추가한다. 사용자가 **임의의 도형을 캔버스 위에 구성**하고, 각 도형을
살아 있는 데이터에 바인딩하며, **데이터 상태에 따라 도형의 겉모습과 문구가 바뀌는** 동적 패널이다. 설비
계통도, 탱크 수위 표시, 밸브 개폐 상태판처럼 "값 한 줄"이 아니라 "그림 한 장"으로 읽어야 하는 화면이 대상이다.

조건 판정은 **규칙 표(rule table)** 로 저술한다. 요소마다 `[바인딩 값][비교 연산자][임계값] → [스타일·문구 패치]`
행을 순서대로 두고, **위에서부터 처음 일치하는 행 하나**가 이긴다. 식(expression) 언어도, 파서도, `eval` 도
도입하지 않는다. 이 어휘는 이미 코드베이스에 있는 임계값 UI(`thresholdFill.ts`, GaugePanel thresholds,
`acControlColors.ts`)와 같은 것이며, 사용자가 새 문법을 배우지 않아도 되도록 의도적으로 그 어휘를 재사용한다.

렌더는 **Canvas 2D** 다(`HeatmapCanvas.tsx` 선례). SVG 가 아니다. 이 선택에는 따라오는 설계 사실이 있으며,
반대 근거가 아니라 본 SPEC 이 감당해야 할 몫으로 아래 §환경·§명세에 명시한다.

### 애니메이션 범위 분할

사용자가 요구한 애니메이션은 세 갈래다. Canvas 2D 는 CSS `transition`/`@keyframes` 를 쓸 수 없어 세 갈래 모두가
**렌더 루프 문제**가 되므로, 본 MVP 는 (a) 하나만 담고 (b)(c) 는 003 으로 이연한다. 사용자의 선택이 축소된 것이
아니라 로드맵에 걸쳐 온전히 이행된다는 뜻이다.

| 갈래 | 내용 | 담는 SPEC |
|------|------|-----------|
| (a) 상태 전이 트위닝 | 규칙 일치가 바뀔 때 색·위치·크기·불투명도가 지정 시간 동안 이징으로 건너간다 | **001 (본 SPEC)** |
| (b) 반복 효과 | 점멸(blink) · 맥동(pulse) · 회전(spin) · 파선 흐름(dash-flow) | 003 |
| (c) 값 구동 연속 애니메이션 | 바인딩 값에 비례하는 속성(탱크 수위 높이, RPM 에 비례한 회전 속도) | 003 |

(a) 를 MVP 에 두는 이유는 그것이 애니메이션 기능이기 이전에 **렌더 루프의 존재 근거**이기 때문이다. `requestAnimationFrame`
루프, 유휴 정지, 가시성 게이팅은 (a) 만으로도 전부 필요하며, 그 골격이 서면 (b)(c) 는 루프에 프레임 계산을
얹는 일이 된다. 반대로 (a) 없이 (b)(c) 를 먼저 넣으면 루프는 있는데 전이가 튀는 패널이 된다.

### 범위 분할 (3개 SPEC)

- **SPEC-CANVAS-001 (MVP, 본 SPEC)**: 패널 타입 등록 + 도형 모델 + Canvas 2D 렌더러 + 데이터 바인딩 + 조건 규칙 표 + 설정 UI + 상태 전이 트위닝. 요소 배치는 설정 다이얼로그의 수치·폼 입력으로 저술한다.
- SPEC-CANVAS-002 (후속): 캔버스 내 시각 편집기 — 드래그 배치, 크기 조절, 정렬·스냅, z-order, 그룹, 에셋 배경.
- SPEC-CANVAS-003 (후속): 애니메이션 전량(반복 효과 + 값 구동), 고급 표현식 형태, 요소 복제.

### 백엔드 변경 0

패널 `config` 는 Go 쪽에서 불투명 JSON(`json.RawMessage`)으로 저장된다. 따라서 본 SPEC 은 **백엔드 변경이 없다** —
신규 엔드포인트도, 스키마 변경도, 전송 계층 변경도 없다. 데이터 조회는 기존 통합 시리즈 훅
`usePanelSeriesData` 를 그대로 쓴다(store / TSDB / sysmetrics 3종 소스가 이미 그 훅 뒤에 있다).

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite 6/Vitest, Tailwind v4). 백엔드 변경 없음.
- 렌더 기술: HTML `<canvas>` 2D context(`CanvasRenderingContext2D`) — 도형 경로 그리기(`rect`/`ellipse`/`moveTo`+`lineTo`) + `fillText`. `HeatmapCanvas.tsx` 의 devicePixelRatio 백킹 버퍼 스케일 · `ResizeObserver` 재계산 규율을 그대로 따른다.
- 신규 의존성 **0**. `web/package.json` 에 식 평가 라이브러리는 없고, 본 SPEC 도 추가하지 않는다(규칙 표가 파서를 대신한다). React 19 / zustand / recharts / @xyflow/react 는 이미 있으나 캔버스 패널은 차트 라이브러리를 쓰지 않는다 — 외부 차트 의존 없이 직접 그리는 선례는 `web/src/pages/dashboard/panels/AgentStatusDiagram.tsx` 다(코드베이스의 자족 규약).
- 데이터 소스(기존, 신규 없음): `web/src/pages/dashboard/panels/charts/usePanelSeriesData.ts` — store / TSDB / sysmetrics 를 한 훅 뒤로 통합한 패널 시리즈 훅. 캔버스 요소는 이 훅이 돌려준 시리즈를 참조할 뿐 별도 조회 경로를 만들지 않는다.
- 좌표계: `web/src/pages/dashboard/panels/heatmap/stage.ts` 의 **정규화(0..1) 좌표 ↔ 컨테이너 px** 변환 규약을 따른다. 패널 크기가 바뀌어도 요소가 제자리에 있어야 하기 때문이다.
- 애니메이션 구동: `requestAnimationFrame` 루프. CSS `transition`/`@keyframes` 는 canvas 픽셀에 적용되지 않으므로 쓸 수 없다.
- 가시성 게이팅: `web/src/pages/dashboard/panels/charts/visiblePolling.ts` 의 `VisibilitySource`(document `visibilitychange` 구독, 테스트 대체 가능) 를 재사용한다.
- 임계값 어휘 선례: `web/src/pages/dashboard/panels/charts/thresholdFill.ts`, GaugePanel thresholds, `web/src/pages/dashboard/panels/acControlColors.ts`.
- config 파서 선례: `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` 의 `parseHeatmapConfig` — 결측/손상/구버전 입력을 예외 없이 기본값으로 보정하는 관용 파서 패턴.
- 패널 등록 6지점: `web/src/stores/uiStore.ts:164`(`PanelType` 유니온) · `:328`(`PANEL_DEFAULT_SIZES: Record<PanelType, PanelGridSize>`) · `:453`(`createDefaultPanel` switch), `web/src/pages/dashboard/AddPanelDialog.tsx:156`(카탈로그 행), `web/src/pages/dashboard/renderDashboardPanel.tsx`(렌더 switch), `web/src/pages/dashboard/PanelSettingsDialog.tsx`(설정 섹션 + 라이브 미리보기; heatmap 블록이 ~1318 / ~1804 행에 있다).
- i18n: `web/src/lib/i18n/{ko.json,en.json}` 양쪽. 키는 `dashboard.addPanel.labels/descriptions.*` 와 `dashboard.settings.*` 아래.
- 신규 컴포넌트 위치: `web/src/pages/dashboard/panels/canvas/`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | 패널 config 는 불투명 JSON 으로 저장되어 백엔드 스키마 변경이 불필요하다 | 높음 | Go 측 `json.RawMessage` config 영속화. 기존 모든 패널이 같은 경로 |
| A2 | 요소 수는 **수백 개 이하**(실용적으로 100 내외)다. 그 이상은 대시보드 패널이 아니라 도면 편집기의 영역이며 본 SPEC 의 성능 설계 대상이 아니다 | 중간 | 설계 전제. 상한 초과 시 §위험 R1 의 완화(요소 상한 + 프레임 예산)로 방어 |
| A3 | 요소 목록을 포함한 패널 config 크기가 기존 대시보드 저장 경로의 한도 안에 든다 | 중간 | 기존 패널 config 도 시리즈 목록·색상표 등 배열을 담고 같은 경로로 저장된다. A2 의 요소 수 전제와 함께 성립 |
| A4 | 001 에서 사용자는 요소의 기하(위치·크기)를 **수치로** 저술한다. 캔버스 위 드래그 배치는 002 다 | 확정 | 사용자 확정 범위 분할 |
| A5 | 요소는 `usePanelSeriesData` 가 돌려주는 시리즈를 **동일성 키**로 참조하며, 신규 데이터 경로·엔드포인트는 만들지 않는다 | 확정 | 사용자 확정 설계 결정 |
| A6 | 조건 저술은 규칙 표뿐이다. 식 언어·파서·`eval` 은 도입하지 않는다 | 확정 | 사용자 확정 설계 결정 #1 |
| A7 | 001 은 **렌더 전용**이다. 요소 클릭·요소별 툴팁 등 per-element 상호작용은 없다 — Canvas 에서 그것은 좌표 역산 히트 테스트이며 시각 편집기와 함께 002 에서 다룬다 | 확정 | Canvas 2D 선택의 귀결(아래 §명세) |
| A8 | 규칙 미일치·시리즈 결측은 오류가 아니라 **기본 스타일로의 정상 폴백**이다 | 확정 | 견고성 요구(REQ-05) |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Optional / Unwanted)을 5개 모듈에 모두 표현한다.

### REQ-01 — 패널 타입 등록 및 config 스키마 (Ubiquitous)

시스템은 **항상** 신규 PanelType `canvas` 를 대시보드 패널 프레임워크 6지점에 등록해야 한다:
(1) `uiStore.ts` 의 `PanelType` 유니온,
(2) `PANEL_DEFAULT_SIZES`(권장 기본 `{w:6, h:4, minW:3, minH:2}`) — 이 맵은 `Record<PanelType, PanelGridSize>` 라 컴파일러가 완전성을 강제하므로 유니온 멤버 추가가 곧 이 편집을 부른다,
(3) `createDefaultPanel` switch(`buildDefaultHeatmapConfig` 를 미러링한 `buildDefaultCanvasConfig`),
(4) `AddPanelDialog.tsx` 카탈로그 행(`{ type, icon, labelKey, descriptionKey }`),
(5) `renderDashboardPanel.tsx` 렌더 switch(패널은 `panelId`/`title`/`config`/`onConfigChange`/`onTitleChange` 를 받는다),
(6) `PanelSettingsDialog.tsx` 설정 섹션 + 라이브 미리보기.
시스템은 **항상** i18n 키를 `ko.json` 과 `en.json` **양쪽**에 추가해야 한다.
시스템은 **항상** `CanvasPanelConfig` 를 정의하고, 알 수 없거나 구버전인 config 를 예외 없이 기본값으로 보정하는
관용 파서 `parseCanvasConfig` 를 통해서만 config 를 읽어야 한다(`parseHeatmapConfig` 패턴).
시스템은 **항상** 신규 컴포넌트를 `web/src/pages/dashboard/panels/canvas/` 아래에 배치해야 한다.

### REQ-02 — 도형 모델과 Canvas 2D 렌더 (State-Driven)

**IF** config 에 요소가 1개 이상 있으면 **THEN** 시스템은 요소 목록을 배열 순서(뒤로 갈수록 위)대로 `<canvas>`
2D context 에 그려야 한다. 지원 도형 원시형은 `rect` · `ellipse` · `line` · `text` 4종이다.
**IF** 패널 표시 크기 또는 `devicePixelRatio` 가 바뀌면 **THEN** 시스템은 백킹 버퍼를 재계산하고 정규화 좌표를
새 크기로 다시 투영해 요소가 화면상 같은 상대 위치에 남도록 해야 한다(`stage.ts` 좌표 규약, `HeatmapCanvas`
`ResizeObserver`+DPR 규율).
**IF** 요소가 `text` 이거나 도형에 라벨이 붙으면 **THEN** 시스템은 `measureText` 로 문자열 폭을 재어 지정된
정렬(left/center/right)에 맞춰 배치해야 한다 — 폰트와 정렬은 브라우저가 아니라 패널이 책임진다.

### REQ-03 — 데이터 바인딩 (Event-Driven)

**WHEN** 캔버스 패널이 마운트되거나 데이터 소스 config 가 바뀌면 **THEN** 시스템은 `usePanelSeriesData` 를
호출해 시리즈를 획득해야 하며, 신규 훅·신규 REST 경로·신규 백엔드 엔드포인트를 만들지 **않아야 한다**.
**WHEN** 각 폴링이 완료되면 **THEN** 시스템은 바인딩을 가진 요소마다 참조 시리즈의 **최신 값**을 뽑아 규칙
평가(REQ-04) 입력으로 넘기고 재렌더를 예약해야 한다.
**WHEN** 언마운트·설정 변경·비활성화가 발생하면 **THEN** 시스템은 훅의 기존 정리 동작(인터벌 해제,
AbortController 중단)에 맡기고 별도 수명주기를 만들지 않아야 한다.
바인딩 참조는 시리즈 **동일성 키**로 하며, 시리즈 이름 생성은 로케일에 의존하지 **않아야 한다**(훅 경로의
기존 제약 — `usePanelSeriesData` 계열 훅은 I18n/QueryClient Provider 없이도 테스트에서 동작해야 한다).

### REQ-04 — 조건 규칙 표 (Optional)

**Where** 사용자가 요소에 규칙 행을 설정하면 시스템은 각 행을 `[바인딩 값][비교 연산자][임계값] → [스타일·문구 패치]`
로 해석하고, **표의 위에서부터 순서대로 평가하여 처음 일치한 행 하나만 적용**해야 한다(first-match-wins,
나머지 행은 평가를 멈춘다).
**Where** 일치한 행이 스타일 패치를 담으면 시스템은 그것을 요소의 기본 스타일 **위에 덮어써야** 한다(패치에
없는 속성은 기본 스타일을 그대로 유지한다).
**Where** 일치한 행이 문구 패치를 담으면 시스템은 그 문구를 요소의 기본 문구 템플릿 대신 사용해야 하며,
문구 템플릿의 치환은 정해진 토큰(`{value}` · `{name}` · `{unit}`)의 **단순 치환**이어야 한다 — 식 평가가 아니다.
**Where** 어떤 행도 일치하지 않으면 시스템은 요소의 **기본 스타일과 기본 문구**를 그대로 사용해야 한다(오류가 아니다).

### REQ-05 — 견고성과 상태 전이 트위닝 (Unwanted)

시스템은 다음을 **하지 않아야 한다**:
- 요소가 0개일 때 렌더 예외를 던지지 **않아야 한다**(빈 상태 안내로 graceful 처리).
- 바인딩이 가리키는 시리즈가 없거나 값이 비었을 때 요소를 사라지게 하거나 크래시하지 **않아야 한다**(규칙 미일치와 같이 기본 스타일로 그리고, 문구 토큰은 지정된 결측 표기로 치환한다).
- 폴링이 실패했을 때 **마지막으로 그린 프레임을 파괴하지 않아야 한다** — 화면은 마지막 성공 프레임을 유지한 채 오류 배지만 덧붙이고 다음 주기에 재시도한다.
- 애니메이션할 것이 없는데도 `requestAnimationFrame` 프레임을 계속 예약하지 **않아야 한다**(모든 트윈이 끝나면 루프는 정지 = 유휴).
- 패널·탭이 보이지 않을 때 프레임을 예약하지 **않아야 한다**(`visiblePolling.ts` 의 `VisibilitySource` 재사용).
- 백엔드 스키마를 변경하거나 신규 엔드포인트를 추가하지 **않아야 한다**.

또한 시스템은 규칙 일치 결과가 바뀌면 이전 스타일에서 새 스타일로 **지정된 지속 시간·이징으로 트위닝**해야 하며,
트윈 대상은 수치·색 속성(채움/선 색, 불투명도, 선 두께, 기하 수치)에 한정해야 한다. 문구·표시 여부처럼 보간이
성립하지 않는 속성은 트윈 없이 즉시 전환해야 한다. 지속 시간이 0이면 즉시 전환하며 루프를 깨우지 않아야 한다.

## 명세 (Specifications)

### config 스키마

`ChartPanelConfigBase`(데이터 소스 필드) 를 확장하는 `CanvasPanelConfig` 를 정의한다.

```
CanvasPanelConfig {
  // 데이터 소스 — 기존 패널과 동일(store / tsdb / sysmetrics). usePanelSeriesData 가 소비한다.
  ...ChartPanelConfigBase

  background?: string          // 캔버스 배경색. 미지정 시 패널 표면색.
  tween?: TweenSpec            // 패널 기본 트윈. 요소가 덮어쓸 수 있다.
  elements: CanvasElement[]    // 배열 순서 = 그리기 순서(뒤가 위). 001 의 유일한 z-order 수단.
}

CanvasElement {
  id: string                   // 안정 식별자(규칙/설정 UI 가 참조).
  kind: 'rect' | 'ellipse' | 'line' | 'text'
  geometry: Geometry           // 정규화(0..1) 스테이지 좌표. stage.ts 규약.
  style: ElementStyle          // 기본 스타일.
  text?: string                // 기본 문구 템플릿. kind:'text' 는 필수, 도형은 라벨로 선택.
  decimals?: number            // {value} 치환 시 소수 자리. 기본 1.
  unit?: string                // {unit} 치환 값.
  binding?: ElementBinding     // 없으면 정적 도형(규칙 평가 대상 아님).
  rules?: RuleRow[]            // 위에서부터 첫 일치 승리.
  tween?: TweenSpec            // 패널 기본을 덮어쓴다.
}

Geometry =
  | { x, y, w, h }             // rect | ellipse — 좌상단 + 크기, 모두 0..1
  | { x1, y1, x2, y2 }         // line
  | { x, y }                   // text — align 기준점

ElementStyle {
  fill?: string; stroke?: string; strokeWidth?: number   // strokeWidth 는 px
  opacity?: number             // 0..1
  fontSize?: number            // px
  fontWeight?: 'normal' | 'bold'
  textColor?: string
  align?: 'left' | 'center' | 'right'
  visible?: boolean            // 기본 true
}

ElementBinding {
  series: string               // 시리즈 동일성 키(로케일 비의존).
  agg: 'last'                  // 001 은 최신값만. 다른 집계는 후속.
}

RuleRow {
  op: 'gt'|'gte'|'lt'|'lte'|'eq'|'ne'|'between'|'nodata'
  value: number | [number, number]   // between 만 2원소. nodata 는 무시.
  patch: Partial<ElementStyle> & { text?: string }
}

TweenSpec { duration_ms: number; easing: 'linear'|'ease-in'|'ease-out'|'ease-in-out' }
```

### 비교 연산자 집합

`gt` · `gte` · `lt` · `lte` · `eq` · `ne` · `between`(경계 포함) · `nodata`(바인딩 시리즈 결측 또는 값 없음).
`nodata` 를 연산자 자리에 둔 이유는, 결측을 별도 개념으로 두면 사용자가 "값이 없을 때의 모습"을 규칙 표 밖에서
따로 배워야 하기 때문이다. 같은 표 안의 한 행이면 순서(위에 두면 결측 우선)까지 사용자가 통제한다.
`eq`/`ne` 는 부동소수 비교이므로 구현은 허용 오차를 둔다.

### 패치 가능한 스타일 속성

`fill` · `stroke` · `strokeWidth` · `opacity` · `textColor` · `fontWeight` · `visible` · `text`.
기하(위치·크기)는 001 의 패치 대상이 **아니다** — 값에 따라 도형이 움직이거나 자라는 것은 값 구동 애니메이션((c))
이며 003 이다. 다만 트윈 엔진은 기하 수치도 보간할 수 있게 만들어 003 이 엔진을 다시 쓰지 않게 한다.

### 문구 템플릿

토큰 3종의 단순 치환이다: `{value}`(바인딩 최신값, `decimals` 자리로 반올림) · `{name}`(시리즈 표시명) ·
`{unit}`(요소의 `unit`). 알 수 없는 토큰은 치환하지 않고 그대로 둔다(사용자가 오타를 화면에서 본다).
바인딩이 없거나 값이 결측이면 `{value}` 는 지정된 결측 표기(기본 `-`)로 치환한다.

### 트윈 명세

규칙 평가 결과(= 적용될 최종 스타일)가 직전 프레임의 것과 달라지면 트윈을 시작한다.
- 보간 대상: 색(`fill`/`stroke`/`textColor` — RGB 선형 보간) · `opacity` · `strokeWidth` · 기하 수치.
- 즉시 전환: `text` · `visible` · `fontWeight` · `align`.
- `duration_ms: 0` 또는 트윈 미설정이면 즉시 전환하고 루프를 깨우지 않는다.
- 트윈 진행 중 규칙 결과가 다시 바뀌면 **현재 보간 중인 값에서** 새 목표로 다시 트윈한다(값이 튀지 않는다).

### 렌더 루프

`requestAnimationFrame` 루프를 본 SPEC 이 도입한다. 규율은 셋이다.
1. **유휴 정지**: 진행 중 트윈이 없으면 다음 프레임을 예약하지 않는다. 데이터가 바뀌거나 트윈이 시작될 때만 깨운다.
2. **가시성 게이팅**: `visiblePolling.ts` 의 `VisibilitySource` 로 문서가 보이지 않으면 루프를 세우고, 다시 보이면 한 프레임을 그려 최신 상태를 즉시 반영한다.
3. **한 프레임 = 전체 다시 그리기**: 001 은 부분 무효화(dirty rect)를 하지 않는다. A2 의 요소 수 전제에서 전면 재그리기가 더 단순하고 빠르다.

### Canvas 2D 선택의 귀결 (설계 사실)

- CSS `transition`/`@keyframes` 는 쓸 수 없다. 모든 애니메이션은 위 렌더 루프 위에서 돈다.
- 요소 선택·툴팁은 DOM 이벤트가 아니라 **좌표 역산 히트 테스트**다. 001 은 렌더 전용으로 두어(A7) 히트 테스트를 시각 편집기와 함께 002 로 이연한다.
- 텍스트 레이아웃은 `measureText` 기반이며 줄바꿈·말줄임이 자동으로 오지 않는다. 폰트·정렬은 패널이 책임진다(REQ-02).

### 컴포넌트 구조 (`web/src/pages/dashboard/panels/canvas/`)

- `canvasConfig.ts` — `CanvasPanelConfig` 타입 + 관용 파서 `parseCanvasConfig` + `buildDefaultCanvasConfig`.
- `canvasRules.ts` — 순수 규칙 평가기. `evaluateRules(value, rules, baseStyle) → ResolvedStyle`. DOM 무의존.
- `canvasGeometry.ts` — 정규화 좌표 ↔ px 투영, 도형별 경로 수치, 텍스트 정렬 기준점 계산. DOM 무의존.
- `canvasTween.ts` — 이징 함수 + 색/수치 보간 + 트윈 상태 진행. DOM 무의존.
- `canvasText.ts` — 문구 토큰 치환. DOM 무의존.
- `drawElement.ts` — 위 순수 결과를 `CanvasRenderingContext2D` 에 그리는 얇은 층.
- `CanvasSurface.tsx` — `<canvas>` + rAF 루프 + DPR/`ResizeObserver` + 가시성 게이팅.
- `CanvasPanel.tsx` — 패널 진입점. `usePanelSeriesData` 바인딩, 빈/오류 상태 분기.

### 추적성 (Traceability)

- REQ-01 → `uiStore.ts`(유니온/`PANEL_DEFAULT_SIZES`/`createDefaultPanel`) · `AddPanelDialog.tsx` · `renderDashboardPanel.tsx` · `PanelSettingsDialog.tsx` · `canvasConfig.ts` · `i18n/{ko,en}.json`
- REQ-02 → `canvasGeometry.ts` · `drawElement.ts` · `CanvasSurface.tsx`
- REQ-03 → `CanvasPanel.tsx`(`usePanelSeriesData` 바인딩) · `canvasText.ts`
- REQ-04 → `canvasRules.ts` · 설정 UI 규칙 표 편집기
- REQ-05 → `CanvasPanel.tsx`(빈/결측/오류 분기) · `CanvasSurface.tsx`(유휴 정지·가시성 게이팅) · `canvasTween.ts`
