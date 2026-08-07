---
id: SPEC-HEATMAP-PANEL-003
version: "0.1.0"
status: in-progress
created: 2026-08-07
updated: 2026-08-07
author: xtra
priority: P3
lifecycle_level: spec-first
title: "Heatmap Panel — 등고선(contour lines, marching squares) 오버레이"
phase: plan
module: web/dashboard
tier: M
tags: [dashboard, panel, heatmap, contour, marching-squares, isoline, canvas, svg, frontend]
---

# SPEC-HEATMAP-PANEL-003 — 등고선(contour lines, marching squares) 오버레이

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-07 | 0.1.0 | 최초 작성. MVP(SPEC-HEATMAP-PANEL-001)의 IDW 보간 스칼라 격자(`idw.ts` 출력)를 입력으로 marching squares 를 수행하여 등치선(iso-value contour) 경로를 생성·렌더. 등고선은 기존 히트맵 렌더 파이프라인 위 optional toggle 레이어. 재보간 금지(기존 격자 재사용). 레벨 개수/명시 값 목록/선 스타일/등치값 라벨 설정. | xtra |

## 개요 (Overview)

SPEC-HEATMAP-PANEL-001(MVP)이 생성하는 **IDW 보간 스칼라 격자(temperature field grid)** 위에 **등고선
(contour lines / iso-lines)** 을 그린다. 사용자가 지정한 등치값(iso-value) 각각에 대해 **marching squares**
알고리즘을 격자에 적용하여 등치선 경로(iso-value path)를 산출하고, 이를 히트맵 위에 오버레이로 렌더한다.

핵심 제약: 등고선 입력은 **MVP `idw.ts` 가 이미 계산한 동일 격자**를 재사용한다. 등고선을 위해 **다른 방식으로
재보간(re-interpolate)하지 않는다** — MVP 의 IDW 격자 출력(`Float32Array` 스칼라장)을 그대로 marching squares
입력으로 전달한다. 이로써 히트맵 색과 등고선이 동일한 스칼라장을 반영하여 시각적으로 일관된다.

등고선은 **선택(optional) 토글** 레이어로, MVP 설정 섹션에 통합된 설정에 의해 켜질 때만 기존 렌더 파이프라인
위에 얹힌다. 렌더 방식은 SVG `<path>`(canvas 위 레이어) 또는 canvas stroke 중 하나를 plan.md 에서 택1한다.
등치값 개수(count) 또는 명시 값 목록(explicit levels), 선 스타일, 선택적 등치값 라벨을 설정으로 노출한다.

이 SPEC 은 SPEC-002(도면 배경/드래그 배치)와 **독립적**이며 상호 비의존이다. 등고선 레이어는 SPEC-002 레이어
스택이 존재할 경우 그 위(마커 아래)에 얹힐 수 있으나, SPEC-002 없이 MVP 단독 위에서도 동작한다.

범위 분할(3개 SPEC):
- SPEC-HEATMAP-PANEL-001 (MVP): 패널 타입 등록 + store 바인딩 + Canvas 2D IDW 보간 렌더 + 상하한 clamp + 색상표.
- SPEC-HEATMAP-PANEL-002 (후속): floor-plan 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터.
- **SPEC-HEATMAP-PANEL-003 (본 SPEC)**: 등고선(contour lines, marching squares) 오버레이.

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite 6/Vitest, Tailwind v4). 백엔드 변경 없음.
- 알고리즘: marching squares — 스칼라 격자의 셀 4코너를 등치값 기준으로 이진화(case 0..15)하고, 셀 모서리에서
  선형 보간으로 교차점을 구해 등치선 세그먼트를 잇는다. saddle(모호) 케이스(5, 10) 처리 규칙 포함.
- 입력 격자(기존 재사용, **재보간 금지**): MVP `idw.ts` 의 `interpolateIDW(...) → Float32Array`(gridW × gridH
  스칼라장)와 그 격자 메타(gridW/gridH, value bounds). 본 SPEC 은 동일 격자를 marching squares 에 그대로 전달.
- 렌더 기술(plan.md 택1): (a) SVG `<path>` 레이어를 히트맵 canvas 위에 z-index 로 겹치기(벡터, 라벨 배치 용이),
  (b) 히트맵 canvas 에 직접 `ctx.stroke`(단일 canvas, 라벨은 별도 처리).
- 데이터 소스: MVP store 폴링(`useStoreChartData`) 그대로. 본 SPEC 은 store 바인딩을 변경하지 않으며, 폴링으로
  갱신된 격자에 등고선을 재계산한다.
- 확장 대상(기존 MVP 자산):
  - `web/src/pages/dashboard/panels/heatmap/idw.ts` — `interpolateIDW` 격자 출력을 등고선 입력으로 소비(변경 최소).
  - `web/src/pages/dashboard/panels/heatmap/HeatmapPanel.tsx` — 등고선 레이어 마운트(토글 시).
  - `web/src/pages/dashboard/panels/heatmap/HeatmapCanvas.tsx` — (b) 방식 선택 시 stroke 지점.
  - `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` — `contour` 설정 필드(추가만).
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx` — heatmap 섹션에 등고선 토글/레벨/스타일/라벨 UI.
- 색상 재사용: `web/src/pages/dashboard/colorSwatchPalette.tsx`(선 색), MVP `color_table` 정지점 패턴.
- i18n: `web/src/lib/i18n/{ko.json,en.json}`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | MVP(SPEC-001)가 선행 완료되어 `idw.ts:interpolateIDW` 가 `Float32Array` 스칼라 격자(gridW×gridH)를 반환한다 | 확정(선행 의존성) | SPEC-001 plan.md T4 `interpolateIDW(...): Float32Array` |
| A2 | 등고선은 MVP 가 이미 계산한 동일 격자를 재사용해야 하며 별도 재보간을 하지 않는다 | 확정 | 오케스트레이터 확정 제약(재보간 금지) |
| A3 | 패널 config 는 불투명 JSON 이므로 `contour` 필드 추가에 백엔드 스키마 변경이 불필요하다 | 높음 | SPEC-001 A4 |
| A4 | 등고선은 선택 토글이며, 미설정/off 시 기존 히트맵 렌더에 영향을 주지 않는다 | 확정 | 오케스트레이터 확정(optional toggle) |
| A5 | store 폴링 주기마다 격자가 갱신될 수 있으므로 등고선 재계산은 격자 대비 memoize/debounce 되어야 한다 | 확정 | 위험(plan.md R2) |
| A6 | marching squares 의 saddle(5,10) 및 경계 셀 케이스는 결정적 규칙으로 처리되어야 한다 | 확정 | 위험(plan.md R1) |
| A7 | 도면 배경/드래그 배치(002)는 본 SPEC 범위 밖이며 상호 비의존이다 | 확정 | 범위 분할 |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Unwanted / Optional)을 5개 모듈에 모두 표현한다.

### REQ-01 — 등고선 생성 (State-Driven)

**IF** 등고선 표시가 활성이고 MVP IDW 보간 격자(스칼라장)가 존재하면 **THEN** 시스템은 설정된 등치값(iso-value)
각각에 대해 해당 격자에 **marching squares** 를 적용하여 등치선 경로를 생성해야 한다. **IF** 셀의 4코너가
등치값 기준으로 교차하면 **THEN** 시스템은 셀 모서리에서 **선형 보간**으로 교차점을 구해 세그먼트를 잇고,
**IF** 셀이 saddle(케이스 5/10) 이면 **THEN** 시스템은 결정적 규칙(예: 셀 중앙값 기준 분기)으로 모호성을
해소해야 한다. 등고선 생성은 반드시 MVP 격자를 입력으로 하며 **재보간하지 않아야 한다**.

### REQ-02 — 등고선 렌더·갱신 (Event-Driven)

**WHEN** store 폴링/설정 변경으로 IDW 격자가 갱신되면 **THEN** 시스템은 (등고선이 활성인 경우) 등치선 경로를
재계산하여 히트맵 위 오버레이로 렌더해야 한다. **WHEN** 사용자가 등치 레벨/선 스타일/라벨 설정을 변경하면
**THEN** 시스템은 등고선을 재계산·재렌더해야 한다. **WHEN** 패널이 리사이즈되면 **THEN** 시스템은 등치선
경로를 표시 크기에 맞춰 스케일하여 히트맵/센서와 정렬되게 재렌더해야 한다.

### REQ-03 — 설정 통합 + optional 레이어 (Ubiquitous)

시스템은 **항상** 등고선 설정을 MVP 히트맵 설정 섹션(PanelSettingsDialog)에 통합하여 제공해야 하며, 등고선은
**기존 히트맵 렌더 파이프라인 위에 얹히는 선택(optional) 레이어**로 동작해야 한다. 시스템은 **항상** 등고선
관련 신규 config 를 MVP `HeatmapPanelConfig` 에 **추가만(additive)** 하여, 기존 필드/의미와 렌더 경로를
변경하지 않아야 한다. 레이어 순서는 (배경/도면 → 히트맵 → **등고선** → 센서 마커) 규약을 따라야 한다.

### REQ-04 — 견고성 / 금지 동작 (Unwanted)

시스템은 다음을 **하지 않아야 한다**:
- 등고선을 위해 격자를 **다른 방식으로 재보간하지 않아야 한다**(MVP `idw.ts` 격자 재사용 강제).
- 등고선 토글이 off 이거나 격자가 비어 있을 때 렌더 예외를 던지지 **않아야 한다**(등고선 미표시로 graceful).
- 매 store 폴링마다 격자가 불변인데도 등고선을 무조건 재계산하지 **않아야 한다**(격자 기준 memoize/debounce).
- saddle/경계 셀에서 비결정적/끊긴 선을 생성하지 **않아야 한다**(결정적 규칙 + 격자 경계 셀 처리).
- 등치값이 격자 값 범위(min/max) 밖일 때 잘못된(빈/전체) 선을 그려 오해를 유발하지 **않아야 한다**(범위 밖 레벨 무시/안내).
- 백엔드 스키마를 변경하거나 신규 엔드포인트를 추가하지 **않아야 한다**(config 는 불투명 JSON 유지).

### REQ-05 — 레벨/스타일/라벨 옵션 (Optional)

**Where** 사용자가 등치 레벨을 **개수(count)** 로 설정하면 시스템은 격자 값 범위를 균등 분할하여 그 개수의
등치값을 산출해야 한다. **Where** 사용자가 **명시 값 목록(explicit levels)** 을 설정하면 시스템은 그 값들을
등치값으로 사용해야 한다(count 보다 우선). **Where** 사용자가 선 스타일(색/두께/실선·점선)을 설정하면 시스템은
등치선 렌더에 반영해야 한다. **Where** 사용자가 등치값 라벨 표시를 켜면 시스템은 각 등치선에 값 라벨을 배치해야
한다. 미설정이면 시스템은 합리적 기본값(예: count=5 균등 레벨, 기본 선 색/두께, 라벨 off)을 사용해야 한다.

## 명세 (Specifications)

### config 스키마 확장 (추가 필드 — additive only)

MVP `HeatmapPanelConfig` 에 다음 필드를 **추가**한다(기존 필드/의미 불변):

- `contour?: { enabled?: boolean; level_count?: number; levels?: number[]; line?: { color?: string; width?: number; dash?: number[] }; labels?: boolean }`
  - `enabled`: 등고선 표시 토글(기본 false).
  - `level_count`: 균등 분할 레벨 개수(기본 5). `levels` 미설정 시 사용.
  - `levels`: 명시 등치값 배열(설정 시 `level_count` 보다 우선).
  - `line`: 선 스타일(색/두께/dash). 미설정 시 기본값.
  - `labels`: 등치값 라벨 표시(기본 false).

### 컴포넌트 구조 (`web/src/pages/dashboard/panels/heatmap/`)

- `marchingSquares.ts`(신규, **TDD 핵심**): 순수 함수 —
  - `computeContours(grid: Float32Array, gridW, gridH, level: number): Segment[]` — 단일 등치값 세그먼트 집합.
    셀 case 0..15 분기, 모서리 선형 보간, saddle(5/10) 결정적 처리, 격자 경계 셀 처리.
  - `resolveLevels(bounds: {min,max}, count?: number, explicit?: number[]): number[]` — 레벨 산출(explicit 우선,
    범위 밖 필터). 범위 밖/빈 격자 방어.
  - `segmentsToPath(segments: Segment[]): string` — SVG path d 문자열(또는 canvas stroke 좌표열)로 변환.
  - DOM 없이 단위 테스트(known-grid → known-segments 골든 케이스, saddle 케이스 포함).
- `ContourLayer.tsx`(신규): 격자 + contour config → `computeContours` 호출 → SVG `<path>`(또는 canvas stroke)
  오버레이 렌더. 격자 대비 memoize(`useMemo`), 표시 크기 스케일, 라벨 배치.
- `HeatmapPanel.tsx`(MVP 확장): 등고선 활성 시 `ContourLayer` 를 히트맵 위(마커 아래) 레이어로 마운트. 격자를
  MVP 렌더 경로에서 파생하여 `ContourLayer` 로 전달(재보간 없이 동일 격자 참조).
- `heatmapConfig.ts`(MVP 확장): `parseHeatmapConfig` 에 `contour` 하위호환 파싱(미설정 기본값 enabled=false 등).

### 성능 (Performance)

- 등고선 재계산은 **격자 참조/해시 기준 memoize** 한다: store 폴링으로 격자가 실제 변경되지 않으면 재계산하지
  않는다(REQ-04). 잦은 폴링에서는 격자 갱신 이벤트를 debounce 하여 렌더 부하를 억제한다.
- marching squares 는 O(gridW × gridH × levels). grid_resolution(MVP idw 파라미터)이 계산량을 지배하므로 저격자
  계산 후 표시 스케일 업(MVP 와 동일 전략)을 유지한다.

### 추적성 (Traceability)

- REQ-01 → `marchingSquares.ts:computeContours`(saddle/보간), MVP `idw.ts` 격자 재사용(재보간 없음)
- REQ-02 → `ContourLayer.tsx`(격자 갱신/설정 변경/리사이즈 재렌더), `HeatmapPanel.tsx`
- REQ-03 → `heatmapConfig.ts:contour`(additive), `PanelSettingsDialog.tsx` 등고선 섹션, 레이어 순서 규약
- REQ-04 → `marchingSquares.ts`(결정적 saddle/경계, 범위밖 필터), `ContourLayer` memoize/debounce, `HeatmapPanel` 토글/빈격자 분기
- REQ-05 → `marchingSquares.ts:resolveLevels`, `ContourLayer`(선 스타일/라벨), `PanelSettingsDialog`(count/levels/style/labels)
