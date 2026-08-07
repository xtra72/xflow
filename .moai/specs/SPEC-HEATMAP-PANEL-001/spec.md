---
id: SPEC-HEATMAP-PANEL-001
version: "1.0.0"
status: completed
created: 2026-08-07
updated: 2026-08-07
author: xtra
priority: P2
lifecycle_level: spec-first
title: "Heatmap Panel (MVP) — Canvas 2D IDW 온도 히트맵 대시보드 패널"
phase: plan
module: web/dashboard
tier: M
tags: [dashboard, panel, heatmap, canvas, idw, store, temperature, frontend]
---

# SPEC-HEATMAP-PANEL-001 — 히트맵 패널 (MVP)

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-07 | 0.1.0 | 최초 작성. MVP 범위: 패널 타입 등록 + store 데이터 바인딩 + Canvas 2D IDW 보간 렌더 + 표시값 상하한(clamp) + 색상표. floor-plan 배경/드래그 배치(002), 등고선(003)은 후속 SPEC 로 분리. | xtra |
| 2026-08-07 | 1.0.0 | 구현 완료(커밋 `69294ced`). REQ-01~05 전량 구현, 신규 코드 커버리지 99%, LSP 0(tsc/eslint), 회귀 786 tests 통과. 좌표 결합 로직을 순수 모듈 `heatmapJoin.ts` 로 분리(단위 테스트 격리 목적, plan.md 대비 유일 분기). 구현 노트는 §구현 노트 참고. | xtra |

## 개요 (Overview)

대시보드에 신규 PanelType `heatmap` 을 추가한다. 공간 내에 배치된 다수의 온도 센서 값을 store 에서 태그
필터로 바인딩하고, 각 센서의 **수동 배치 좌표 {x, y}** 를 기준으로 **IDW(Inverse Distance Weighting)**
보간을 수행하여 연속적인 온도장(temperature field)을 **HTML Canvas 2D** 에 픽셀 단위로 렌더한다. 보간된
값은 사용자가 설정한 **상·하한(min/max)** 으로 clamp 된 뒤 **색상표(color table)** 를 통해 색으로 매핑된다.

이 SPEC 은 코드베이스 최초의 실제 2D drawing canvas 를 도입한다(허용된 결정). 신규 백엔드/엔드포인트/전송
계층은 도입하지 않으며, 패널 config 는 기존과 동일하게 불투명 JSON(`json.RawMessage`)으로 저장되어 백엔드
스키마 변경이 없다. 데이터 조회는 기존 store REST 폴링(`POST /api/v1/store/{agent}/query`)을 그대로 재사용한다.

범위 분할(3개 SPEC):
- **SPEC-HEATMAP-PANEL-001 (MVP, 본 SPEC)**: 패널 타입 등록 + store 데이터 바인딩 + Canvas 2D IDW 보간 렌더 + 표시값 상하한 clamp + 색상표.
- SPEC-HEATMAP-PANEL-002 (후속): floor-plan 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터.
- SPEC-HEATMAP-PANEL-003 (후속): 등고선(contour lines, marching squares).

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite 6/Vitest, Tailwind v4). 백엔드 변경 없음.
- 렌더 기술: HTML `<canvas>` 2D context(`CanvasRenderingContext2D`) + `ImageData` 픽셀 채움. 신규 canvas 도입.
- 보간 알고리즘: IDW(Inverse Distance Weighting) — 배치된 센서점 집합에 대해 그리드 픽셀마다 가중 평균 계산.
- 데이터 소스(기존, 신규 없음):
  - store 매트릭스 폴링 훅 `useStoreChartData` (`web/src/pages/dashboard/panels/charts/useStoreChartData.ts`).
  - store REST 클라이언트 `storeSeriesDataSource().queryMatrix` / `queryStoreMatrix` (`web/src/services/api/store.ts:589`).
  - store config 타입 `StoreSourceConfig`(`chartChannelTypes.ts:97`), `StoreSeriesRef`(`:55`), `ChartPanelConfigBase`(`:152`).
  - 조회 방식: `selection_mode: 'tag'` — 공간 내 온도 센서를 태그 필터로 동적 바인딩. 각 시리즈의 최신 값이 센서 1개 판독값.
- 타임스탬프: epoch milliseconds(int64 UnixMilli). (프로젝트 규약)
- 대시보드 패널 프레임워크(등록 4지점):
  - `web/src/stores/uiStore.ts` — `PanelType` 유니온(~L184-188), `panelDefaultSize` switch(~L327), `createDefaultPanel` 완전 switch(~L359).
  - `web/src/pages/dashboard/renderDashboardPanel.tsx` — 패널 렌더 switch(~L81).
  - `web/src/pages/dashboard/AddPanelDialog.tsx` — `PANEL_OPTIONS_BY_CATEGORY` chart 카테고리(~L121).
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx` — 신규 설정 섹션(CollapsibleSection 패턴, `CHART_PANEL_TYPES` 참고 L87).
- 신규 컴포넌트 위치: `web/src/pages/dashboard/panels/heatmap/`.
- 색상 재사용: `web/src/pages/dashboard/colorSwatchPalette.tsx`(`COLOR_PALETTE`), `chartChannelTypes.ts` 의 `pickSeriesColor`/threshold 패턴.
- 공간 배치 선례: `web/src/pages/dashboard/panels/FacilityLinePanel.tsx`(absolute 오버레이), `web/src/pages/dashboard/panels/modbus/RegisterMapGrid.tsx`(색상 셀 그리드).
- 미러링 대상 store 바인딩 패널: `web/src/pages/dashboard/panels/charts/LineChartPanel.tsx`(isStore 분기 ~L338-361).
- i18n: `web/src/lib/i18n/{ko.json,en.json}`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | store `selection_mode:'tag'` 로 공간 온도 센서를 태그 필터로 동적 바인딩할 수 있고, 각 시리즈의 최신(last) 값이 센서 1개 판독값이다 | 높음 | `StoreSourceConfig.selection_mode`(`chartChannelTypes.ts`), `useStoreChartData` tag 모드 재해석 폴링 구현 |
| A2 | `useStoreChartData` 는 `entries`/`seriesEntries`/`status` 를 반환하며, 시리즈 표시 이름(alias/key)으로 센서를 식별할 수 있다 | 높음 | `useStoreChartData.ts` 헤더 주석의 반환 형상 규정 |
| A3 | 센서 (x,y) 좌표는 이 MVP 에서 패널 config 에 **수동 저장**된다(시리즈 key/tags → {x,y} 매핑). 시각적 드래그 배치는 002 로 이연 | 확정 | 오케스트레이터 확정 설계 결정 #1 |
| A4 | 패널 config 는 불투명 JSON 으로 저장되어 백엔드 스키마 변경이 불필요하다 | 높음 | 기존 모든 차트 패널이 `json.RawMessage` config 로 영속화(스키마 무변경) |
| A5 | 좌표 미지정 센서, 센서 0개, 폴링 실패는 렌더 예외 없이 graceful 처리되어야 한다 | 확정 | 오케스트레이터 확정 엣지 케이스 요구 |
| A6 | IDW power(거리 감쇠 지수)와 grid resolution(픽셀 격자 밀도)은 config 파라미터로 노출된다 | 확정 | 오케스트레이터 확정 설계 결정 #2 |
| A7 | 등고선/floor-plan 배경은 본 SPEC 범위 밖이다 | 확정 | 범위 분할(002/003) |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Unwanted / Optional)을 5개 모듈에 모두 표현한다.

### REQ-01 — 패널 프레임워크 통합 (Ubiquitous)

시스템은 **항상** 신규 PanelType `heatmap` 을 대시보드 패널 프레임워크 4지점에 등록해야 한다:
(1) `uiStore.ts` `PanelType` 유니온 + `panelDefaultSize`(기본 크기 권장 `{w:5, h:4, minW:3, minH:3}`) + `createDefaultPanel` 완전 switch 의 기본 config,
(2) `renderDashboardPanel.tsx` switch 에 `case 'heatmap'` 렌더 분기,
(3) `AddPanelDialog.tsx` `PANEL_OPTIONS_BY_CATEGORY` 의 **chart 카테고리**에 옵션 추가(아이콘/labelKey/descriptionKey),
(4) `PanelSettingsDialog.tsx` 에 heatmap 전용 설정 섹션(CollapsibleSection).
시스템은 **항상** 신규 컴포넌트를 `web/src/pages/dashboard/panels/heatmap/` 아래에 배치해야 한다.

### REQ-02 — Store 데이터 바인딩 (Event-Driven)

**WHEN** 히트맵 패널이 마운트되거나 store config 가 변경되면 **THEN** 시스템은 `useStoreChartData` 훅을
`selection_mode:'tag'` + `tag_filters` 로 호출하여 공간 온도 센서를 동적 바인딩하고, `refresh_interval_ms`
주기로 매트릭스를 폴링해야 한다. **WHEN** 각 폴링이 완료되면 **THEN** 시스템은 시리즈별 **최신 값**을 센서 1개
판독값으로 추출하여 온도장 입력 집합을 갱신해야 한다. **WHEN** 언마운트/설정 변경/비활성화가 발생하면
**THEN** 시스템은 인터벌을 정리하고 진행 중 요청을 AbortController 로 중단해야 한다(훅 기존 동작 재사용).
시스템은 신규 전송 계층 없이 기존 `POST /api/v1/store/{agent}/query` 경로만 사용해야 한다.

### REQ-03 — Canvas 2D IDW 보간 렌더 (State-Driven)

**IF** 좌표가 지정된 센서가 1개 이상 존재하면 **THEN** 시스템은 `<canvas>` 2D context 에 grid resolution
격자의 각 픽셀에 대해 배치된 센서점들의 IDW(power 지수 기반) 가중 평균을 계산하여 보간 온도장을 `ImageData`
로 렌더해야 한다. **IF** 특정 픽셀이 센서점과 정확히 일치하면 **THEN** 시스템은 해당 센서 원본 값을 사용해야
한다(0-거리 분모 예외 방지). 보간된 값은 REQ-05 의 색상 매핑 입력으로 전달되어야 한다. 렌더는 패널 리사이즈
시 canvas 픽셀 버퍼를 표시 크기에 맞춰 재계산해야 한다.

### REQ-04 — 견고성 / 금지 동작 (Unwanted)

시스템은 다음을 **하지 않아야 한다**:
- 센서가 0개이거나 좌표 지정 센서가 0개일 때 렌더 예외를 던지지 **않아야 한다**(빈 상태 안내 표시로 graceful 처리).
- 좌표가 미지정된(config 에 {x,y} 없는) 센서를 보간 입력에 포함하지 **않아야 한다**(무시하고 안내).
- store 폴링 실패 시 마지막 성공 렌더를 파괴하거나 크래시하지 **않아야 한다**(오류 상태 표시 + 다음 폴링 재시도).
- 백엔드 스키마를 변경하거나 신규 엔드포인트를 추가하지 **않아야 한다**(config 는 불투명 JSON 유지).

### REQ-05 — 표시 상하한 + 색상표 (Optional)

**Where** 사용자가 값 상·하한(min/max)을 설정하면 시스템은 색상 매핑 전에 보간값을 해당 범위로 **clamp**
해야 한다(min 이하는 min 색, max 이상은 max 색). **Where** 사용자가 색상표(color gradient/table)를 설정하면
시스템은 clamp 된 정규화 값(0..1)을 색상표로 매핑하여 픽셀 색을 결정해야 한다. **Where** 사용자가 IDW power
와 grid resolution 을 설정하면 시스템은 해당 파라미터를 보간에 반영해야 한다. 상하한/색상표/IDW 파라미터가
미설정이면 시스템은 합리적 기본값(min/max 자동 = 센서값 범위, 기본 색상표, power=2, 중간 해상도)을 사용해야 한다.

## 명세 (Specifications)

### config 스키마 확장 (heatmap 전용 필드)

`ChartPanelConfigBase`(`data_source`/`store_source` 재사용)를 확장하는 `HeatmapPanelConfig` 를 정의한다:

- `store_source: StoreSourceConfig` — `selection_mode:'tag'`, `tag_filters`(예: `{type:'temperature', space:'...'}`), `aggregation:'last'`.
- `sensor_positions: Record<string, { x: number; y: number }>` — 시리즈 key(또는 alias/tag 조합)를 정규화 좌표(0..1 권장)에 매핑.
- `value_bounds?: { min: number; max: number }` — 색상 매핑 clamp 범위(미지정 시 자동).
- `color_table?: Array<{ stop: number; color: string }>` — 정규화 값(0..1) → 색 정지점(threshold-color 패턴, `COLOR_PALETTE` 재사용). 미지정 시 기본 gradient.
- `idw?: { power?: number; grid_resolution?: number }` — power 기본 2, grid_resolution 기본 중간값(예: 32~64).

### 컴포넌트 구조 (`web/src/pages/dashboard/panels/heatmap/`)

- `HeatmapPanel.tsx` — 패널 진입점. `LineChartPanel` isStore 분기를 미러링하여 `useStoreChartData` 로 시리즈 획득, 센서값+좌표 결합, `<HeatmapCanvas>` 렌더.
- `HeatmapCanvas.tsx` — `<canvas>` + `useEffect` 로 `ImageData` IDW 채움. 리사이즈 대응(ResizeObserver 또는 부모 크기).
- `idw.ts` — 순수 함수 `interpolateIDW(points, gridW, gridH, power)` + `mapValueToColor(value, bounds, colorTable)`. 테스트 대상(TDD).
- `heatmapConfig.ts` — `HeatmapPanelConfig` 타입 + `parseHeatmapConfig` 파서(하위호환 기본값).

### 추적성 (Traceability)

- REQ-01 → uiStore/renderDashboardPanel/AddPanelDialog/PanelSettingsDialog + `heatmap/HeatmapPanel.tsx`
- REQ-02 → `useStoreChartData` 재사용, `HeatmapPanel.tsx` store 분기
- REQ-03 → `HeatmapCanvas.tsx`, `idw.ts:interpolateIDW`
- REQ-04 → `HeatmapPanel.tsx` 빈/오류 상태 분기, `parseHeatmapConfig` 좌표 필터
- REQ-05 → `idw.ts:mapValueToColor`, `HeatmapPanelSettingsSection`, `colorSwatchPalette`

## 구현 노트 (Implementation Notes)

구현 완료 (커밋 `69294ced`, 2026-08-07). Level 1 spec-first SPEC 로서 계획된 파일이 모두 생성되었고, 아래 1건의 분기 외에는 plan.md 설계와 일치한다.

### 생성 파일 (신규 디렉토리 `web/src/pages/dashboard/panels/heatmap/`)

MVP 는 순수 로직 3종 + 렌더/패널 2종 + 설정 UI + i18n + 등록 4지점으로 구성된다. 신규 컴포넌트는 모두 신규 디렉토리에 배치했다(REQ-01).

- `idw.ts` — IDW 보간(`interpolateIDW`, 0-거리 안전 분기) + 값→색 매핑(`mapValueToColor`, clamp + 색상표 정지점 보간) 순수 함수 (REQ-03/05, TDD).
- `heatmapConfig.ts` — `HeatmapPanelConfig` 타입 + `parseHeatmapConfig` 파서(결측/손상 입력을 예외 없이 기본값 보정, 하위호환 — A5/REQ-05).
- `heatmapJoin.ts` — 센서 최신값(`aggregation:'last'`) + 배치 좌표 결합 순수 로직. 좌표 미배치 센서는 보간 입력 제외 + 설정 UI 노출용 별도 목록(AC-E2).
- `HeatmapCanvas.tsx` — `<canvas>` 2D 렌더 컴포넌트. 저해상 IDW 격자 → `ImageData` → devicePixelRatio 업스케일 blit, `ResizeObserver` 리사이즈 재계산(R1 선명도 / R2 성능 / AC-E4).
- `HeatmapPanel.tsx` — 패널 진입점. `LineChartPanel` isStore 분기 미러링으로 `useStoreChartData` 태그 바인딩(REQ-02) + 센서값·좌표 결합(REQ-04 견고성 분기).
- 대응 테스트 5종: `idw.test.ts`, `heatmapConfig.test.ts`, `heatmapJoin.test.ts`, `HeatmapCanvas.test.tsx`, `HeatmapPanel.test.tsx`.

패널 프레임워크 등록 4지점(REQ-01): `stores/uiStore.ts`(`PanelType` 유니온 + `panelDefaultSize` + `createDefaultPanel`), `pages/dashboard/renderDashboardPanel.tsx`(렌더 switch), `pages/dashboard/AddPanelDialog.tsx`(chart 카테고리 옵션), `pages/dashboard/PanelSettingsDialog.tsx`(heatmap 전용 설정 섹션: 태그필터 / 센서 x·y / value_bounds / color_table / IDW power·resolution). i18n `lib/i18n/{ko,en}.json`.

### 분기 (Divergence, as-implemented)

- **IN-1 — 좌표 결합 로직을 `heatmapJoin.ts` 순수 모듈로 분리**: plan.md 는 좌표-센서값 결합을 `HeatmapPanel.tsx` 내부에 두는 것으로 설계했으나, 실제 구현은 이를 별도 순수 모듈 `heatmapJoin.ts` 로 추출했다. **단위 테스트 격리 목적**(DOM 의존 없는 결합 로직을 독립 커버)이며, 새로운 추상화 계층을 도입한 것은 아니다. 그 외 계획 파일·설계는 모두 일치.

### 순-신규 기술 (net-new)

- **HTML Canvas 2D API** (`CanvasRenderingContext2D` + `ImageData`) — 코드베이스 **최초의 실제 2D drawing canvas**. 기존 차트 패널은 SVG/DOM 렌더였으므로 픽셀 단위 `ImageData` 채움은 본 SPEC 이 처음 도입한다.
- 신규 의존성 0, 백엔드/엔드포인트/전송 계층 변경 0(패널 config 는 기존과 동일 불투명 JSON, store REST 폴링 `POST /api/v1/store/{agent}/query` 재사용).

### 품질 결과

- REQ-01~05 전량 구현. 신규 코드 커버리지 **99%**, LSP **0**(tsc `--noEmit` + eslint 클린), 회귀 프론트 **786 tests** 통과.

### 후속 SPEC (범위 밖, 불변)

- SPEC-HEATMAP-PANEL-002: floor-plan 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터.
- SPEC-HEATMAP-PANEL-003: 등고선(contour lines, marching squares).
