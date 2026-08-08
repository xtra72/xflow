---
id: SPEC-PANEL-SETTINGS-001
version: "0.1.0"
status: draft
created: 2026-08-08
updated: 2026-08-08
author: xtra
priority: P2
lifecycle_level: spec-anchored
title: "Panel Settings — 3분할 설정 셸 + 공용 Store 리스트 + heatmap 색상 프리셋 + 라이브 미리보기"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, settings, store, live-preview, color-preset, gradient, tsdb-placeholder, refactor, frontend]
---

# SPEC-PANEL-SETTINGS-001 — Panel Settings 재설계 (3분할 셸 · 공용 Store 리스트 · 색상 프리셋 · 라이브 미리보기)

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-08 | 0.1.0 | 최초 작성. Store 바인딩 패널(line/bar/pie/stat/table + heatmap) 설정 UX 를 3분할 셸(미리보기·옵션·데이터소스)로 재편. 4방향 드래그 리사이즈 + 패널별 localStorage 비율 영속. 에이전트 상세(`AgentDetailPanel`)의 Store 리스트 렌더/정렬/필터를 **공용 `StoreEntryTable`** 로 추출(단일 소스), actions 컬럼→Alias Name 치환 + 행 선행 체크박스. heatmap 전용 gradient 색상 프리셋(4~5종 + 커스텀). 실 store 데이터 기반 라이브 미리보기(디바운스·미확정·롤백). TSDB 데이터소스 토글은 placeholder+후속 안내만(실동작 후속 SPEC). EARS 5모듈/14 REQ. | xtra |

## 개요 (Overview)

대시보드의 **Store 바인딩 패널** 설정 경험을 재설계한다. 현재 설정 UI(`PanelSettingsDialog.tsx`, 4,370L)는
차트 5종(stat/line-chart/bar-chart/pie-chart/table)과 heatmap 의 config 편집 섹션을 단일 다이얼로그에
수직 누적하는 골격이다. 본 SPEC 은 이 다이얼로그 골격을 **3분할 셸**로 교체하여 (1) 좌측에 실제 패널을
draft config 로 렌더하는 **라이브 미리보기**, (2) 중앙에 패널 타입별 **옵션 편집**, (3) 우측에 **데이터소스
선택**(Store 리스트 + TSDB 토글 placeholder)을 배치한다.

핵심 재사용·추출 원칙: 에이전트 상세 화면(`AgentDetailPanel.tsx`, 4,616L)이 이미 보유한 Store 엔트리
**리스트 렌더/정렬/필터/컬럼 표시숨김/localStorage 영속** 로직을, 에이전트 상세와 패널 설정이 **공유하는
단일 소스 컴포넌트 `StoreEntryTable`** 로 추출한다. 패널 설정 측에서는 이 공용 테이블에 (a) `actions` 컬럼을
**Alias Name** 컬럼으로 치환하고, (b) 각 행 선행에 **선택 체크박스**를 추가하여 선택 결과를 패널의
`StoreSourceConfig` 로 반영한다.

heatmap 패널에 한해 이름있는 **gradient 색상 프리셋**(예: Viridis / Turbo / Warm / Cool + 기존 기본)과
커스텀 ColorStop 편집을 제공한다. 색상 프리셋은 heatmap 외 패널 타입에는 노출하지 않는다.

라이브 미리보기는 **실 store 데이터**(기존 프리뷰 방식 — 실제 패널 컴포넌트를 draft config 로 렌더)를
사용하며, 옵션/색상/데이터소스 변경을 디바운스하여 즉시 반영하되 **저장 전에는 미확정(draft) 상태**로
두고 취소 시 committed 상태로 롤백한다.

TSDB(시계열 DB) 데이터소스는 본 SPEC 범위에서 **실동작을 구현하지 않는다**. 데이터소스 영역에
Store / TSDB 토글을 노출하되 TSDB 선택 시 **비활성 안내 placeholder** 만 표시하고, 실제 TSDB 바인딩·질의는
후속 SPEC 으로 명시 안내한다(비목표).

### 비목표 (Non-Goals)

- TSDB 데이터소스의 실제 바인딩·질의·렌더 (본 SPEC 은 placeholder + 후속 안내만).
- 백엔드 스키마 변경 / 신규 엔드포인트 (패널 config 는 불투명 JSON 유지, `StoreSourceConfig` 는 additive 확장만).
- 에이전트 상세 화면의 기능 변경 (`StoreEntryTable` 추출은 행위 보존 리팩터이며 에이전트 상세 동작은 불변).
- 신규 패널 타입 추가 / 패널 렌더 파이프라인 변경.
- Store 리스트 라벨 충돌 회피·가상 스크롤 등 대규모 성능 최적화 (선택 상한 가드로 대체).

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite/Vitest, Tailwind). 백엔드 변경 없음.
- 대상 패널: Store 바인딩 패널 전체 — `stat`, `line-chart`, `bar-chart`, `pie-chart`, `table`(차트 5종) +
  `heatmap`. (`renderDashboardPanel.tsx:258-269` 에서 각 타입 분기.)
- 확장 대상(기존 자산):
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx`(4,370L) — 다이얼로그 골격을 3분할 셸로 교체(슬롯 이관).
  - `web/src/pages/dashboard/PanelSettingsPage.tsx`(30L) — 다이얼로그 진입점(변경 최소).
  - `web/src/pages/dashboard/ChartPanelSections.tsx`(2,896L) — 차트 5종 옵션 섹션(`StatChartSection`,
    `LineChartSection`, `BarChartSection`, `PieChartSection`, `TableChartSection`) + 공용 `StoreSourceSection`,
    `ChartChannelSection`. 옵션 슬롯으로 재배치.
  - `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts:97` — `StoreSourceConfig` 인터페이스(additive 확장).
  - `web/src/pages/dashboard/colorSwatchPalette.tsx`(95L) — `COLOR_PALETTE`(단색 스와치, 선/계열 색 재사용).
  - `web/src/pages/dashboard/panels/heatmap/idw.ts:25` — `DEFAULT_COLOR_TABLE`(ColorStop[] 기본 gradient).
- 추출 원본(공용화 대상, 기존 자산):
  - `web/src/pages/agents/AgentDetailPanel.tsx`(4,616L) — `StoreEntryRow`(2,414L~), `renderCell`(2,605L~,
    `case 'actions'` 2,686/2,755L), thead/tbody 단일 목록 공유(2,994L~), 정렬/필터/컬럼 상태.
  - `web/src/pages/agents/storeColumns.tsx`(667L) — `STORE_COLUMNS` 레지스트리 + `ColumnSettingsMenu` /
    `ColumnFilterButton` / `ColumnHeader`.
  - `web/src/pages/agents/storeEntrySort.ts`(386L) — 정렬 비교자.
- 데이터 소스(기존): Store 폴링(`web/src/pages/dashboard/panels/charts/useStoreChartData.ts:270`
  `useStoreChartData(...)`). 본 SPEC 은 Store 바인딩 방식을 변경하지 않는다.
- 후속(TSDB) 진입점(참고, 본 SPEC 비활성): `web/src/services/api/seriesDataSource.ts`,
  `web/src/services/api/tsdb.ts`, `web/src/hooks/useTsdb.ts`.
- i18n: `web/src/lib/i18n/{ko.json,en.json}`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | 패널 config 는 불투명 JSON 이므로 `StoreSourceConfig` 필드 additive 확장에 백엔드 스키마 변경이 불필요하다 | 높음 | HEATMAP-PANEL 계열 A3/A4 선례(불투명 JSON config) |
| A2 | 에이전트 상세의 Store 리스트 로직은 패널 설정과 렌더 요구가 사실상 동일하여 공용 컴포넌트로 추출 가능하다 | 높음 | `AgentDetailPanel` thead/tbody 단일 목록 공유(2,994L~), `storeColumns`/`storeEntrySort` 이미 분리 |
| A3 | 공용 `StoreEntryTable` 추출은 행위 보존 리팩터이며 에이전트 상세 화면 동작을 바꾸지 않는다 | 확정(제약) | 오케스트레이터 확정(단일 소스, 특성화 테스트로 회귀 0 방어) |
| A4 | 라이브 미리보기는 실제 패널 컴포넌트를 draft config 로 렌더하는 기존 프리뷰 방식을 재사용할 수 있다 | 높음 | 현 `PanelSettingsDialog` 프리뷰 존재 |
| A5 | 잦은 편집/폴링에서 미리보기 재렌더는 디바운스되어야 하며, 선택 계열은 합리적 상한이 필요하다 | 확정 | 위험(R3) |
| A6 | 색상 프리셋은 heatmap 전용이며 차트 5종에는 노출되지 않아야 한다 | 확정(제약) | 오케스트레이터 확정 |
| A7 | TSDB 실동작은 본 SPEC 밖(placeholder+후속 안내만)이며, 데이터소스 토글 UI 는 미리 노출한다 | 확정(제약) | 오케스트레이터 확정(비목표) |
| A8 | 4방향 드래그 리사이즈 비율은 패널별로 localStorage 에 영속되고 재진입 시 복원된다 | 확정 | 오케스트레이터 확정 세부 기본값 |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Unwanted / Optional)을 5개 모듈에 표현한다.
총 14 REQ.

### M1 — 3분할 레이아웃 셸

#### REQ-01 — 3분할 셸 구성 (Ubiquitous)

시스템은 **항상** Store 바인딩 패널 설정을 3분할 셸로 제공해야 한다: **미리보기 영역**(실 패널을 draft config
로 렌더), **옵션 영역**(패널 타입별 설정 섹션), **데이터소스 영역**(Store 리스트 + TSDB 토글). 기본 배치는
1/4 미리보기 · 2/4·3/4 옵션 · 4/4 데이터소스 이다. 시스템은 **항상** 기존 옵션/데이터소스 편집 슬롯을 이
셸의 해당 영역에 이관하여, 편집 기능 손실 없이 배치만 재구성해야 한다.

#### REQ-02 — 리사이즈 (Event-Driven)

**WHEN** 사용자가 영역 경계(좌우·상하)를 드래그하면 **THEN** 시스템은 인접 영역의 크기를 실시간으로 조정하고
경계 비율을 갱신해야 한다. **WHEN** 드래그가 종료되면 **THEN** 시스템은 갱신된 비율을 **패널별 localStorage**
키에 영속해야 한다.

#### REQ-03 — 비율 복원 (State-Driven)

**IF** 해당 패널의 저장된 비율이 localStorage 에 존재하면 **THEN** 시스템은 설정 진입 시 그 비율로 3분할
셸을 복원해야 한다. **IF** 저장된 비율이 없거나 손상되었으면 **THEN** 시스템은 기본 비율로 폴백해야 한다.

### M2 — 데이터소스 선택

#### REQ-04 — Store 데이터소스 선택 (Event-Driven)

**WHEN** 사용자가 데이터소스 영역에서 Store 를 선택하면 **THEN** 시스템은 Store 리스트를 노출하고, 선택 결과를
패널의 `StoreSourceConfig` 로 반영하여 미리보기를 갱신해야 한다.

#### REQ-05 — TSDB 미활성 placeholder (Unwanted)

시스템은 데이터소스 토글에 TSDB 를 노출하되, 본 SPEC 범위에서 TSDB 를 실제로 바인딩·질의하지 **않아야 한다**.
**IF** 사용자가 TSDB 를 선택하면 **THEN** 시스템은 실동작 대신 **비활성 안내 placeholder**(후속 SPEC 안내)를
표시해야 하며, 이때 기존 Store 설정을 파괴하거나 잘못된 빈 데이터를 렌더하지 **않아야 한다**.

### M3 — 공용 Store 리스트

#### REQ-06 — 동일 컬럼·렌더 (Ubiquitous)

시스템은 **항상** 패널 설정의 Store 리스트를 에이전트 상세와 **동일한 컬럼 정의·렌더 로직**(공용
`StoreEntryTable` 단일 소스)으로 표시해야 한다. `STORE_COLUMNS` 레지스트리와 정렬 비교자(`storeEntrySort`)를
재사용해야 한다.

#### REQ-07 — 행 선택 체크박스 (Event-Driven)

**WHEN** 사용자가 행 선행 체크박스를 토글하면 **THEN** 시스템은 해당 store 엔트리를 선택 집합에 추가/제거하고,
선택 집합을 패널의 `StoreSourceConfig`(선택 계열)로 반영해야 한다.

#### REQ-08 — 필터/정렬/표시숨김 영속 (State-Driven)

**IF** 사용자가 컬럼 필터·정렬·표시숨김을 설정하면 **THEN** 시스템은 공용 리스트에 즉시 반영하고 그 상태를
localStorage 에 영속하여, **IF** 재진입하면 **THEN** 저장된 필터/정렬/표시숨김 상태를 복원해야 한다.

#### REQ-09 — actions→Alias 컬럼 치환 (Ubiquitous)

시스템은 **항상** 패널 설정 컨텍스트의 공용 리스트에서 에이전트 상세의 `actions` 컬럼을 **Alias Name** 컬럼으로
치환하여 표시해야 한다(에이전트 상세 컨텍스트에서는 `actions` 유지 — 컨텍스트별 컬럼 세트 주입).

### M4 — 색상 프리셋 (heatmap 전용)

#### REQ-10 — gradient 프리셋 제공 (Optional)

**Where** 편집 대상 패널이 heatmap 이면 시스템은 이름있는 gradient 색상 프리셋(예: Viridis / Turbo / Warm /
Cool + 기존 기본, 총 4~5종)과 커스텀 ColorStop 편집을 옵션 영역에 제공해야 한다.

#### REQ-11 — 프리셋 적용 (Event-Driven)

**WHEN** 사용자가 프리셋을 선택하면 **THEN** 시스템은 그 프리셋의 ColorStop 배열을 heatmap config 의 색상표로
draft 반영하고 미리보기를 갱신해야 한다. **WHEN** 사용자가 커스텀 ColorStop 을 편집하면 **THEN** 시스템은
편집 결과를 draft 색상표로 반영해야 한다.

#### REQ-12 — heatmap 외 미노출 (Unwanted)

시스템은 heatmap 이 아닌 패널 타입(차트 5종)에서는 색상 프리셋 UI 를 노출하지 **않아야 한다**.

### M5 — 라이브 미리보기

#### REQ-13 — 즉시 갱신 (Event-Driven)

**WHEN** 사용자가 옵션·색상·데이터소스를 변경하면 **THEN** 시스템은 변경을 디바운스하여 미리보기 영역의 실
패널 렌더를 draft config 로 즉시 갱신해야 한다. 미리보기는 **실 store 데이터**를 사용해야 한다.

#### REQ-14 — 미확정 상태·롤백 (Ubiquitous)

시스템은 **항상** 저장 전 편집을 미확정(draft) 상태로 유지하고 committed 상태와 분리해야 한다. 사용자가
저장하면 draft 를 committed 로 승격하고, 취소하면 draft 를 폐기하고 committed 상태로 롤백해야 한다.

## 명세 (Specifications)

### `StoreSourceConfig` 스키마 확장 (additive only)

`chartChannelTypes.ts:97` 의 `StoreSourceConfig` 에 **선택 계열(체크박스 선택 결과)** 표현을 additive 로
확장한다(기존 필드/의미 불변). 확장 필드는 미설정 시 기존 동작과 동일해야 하며, 하위호환 파싱을 보장한다.
색상 프리셋은 heatmap config(별도, `DEFAULT_COLOR_TABLE` 패턴의 ColorStop 배열)에 반영되며 `StoreSourceConfig`
와 독립이다.

### 3분할 셸 (`PanelSettingsDialog` 골격 교체)

- `PanelSettingsDialog.tsx` 의 다이얼로그 본문을 3분할 셸 컨테이너로 교체한다. 기존 옵션 섹션
  (`ChartPanelSections` 의 5종 + `StoreSourceSection` + heatmap 섹션)은 **슬롯으로 이관**하되 내부 편집 로직은
  보존한다(셸만 교체, 슬롯 이관 — R7 완화).
- 4방향 리사이즈 스플리터(좌우·상하)와 패널별 localStorage 비율 영속/복원.

### 공용 `StoreEntryTable` 추출

- `AgentDetailPanel` 의 `StoreEntryRow` / `renderCell` / thead·tbody 단일 목록 공유 / 정렬·필터·컬럼 상태를
  **공용 `StoreEntryTable`** 컴포넌트로 추출한다(신규 파일, 단일 소스).
- 컨텍스트별 주입: 에이전트 상세는 `actions` 컬럼 + 기존 동작, 패널 설정은 **행 선택 체크박스 + Alias Name
  컬럼**. 컬럼 세트/렌더 훅을 props 로 주입하여 분기한다.
- `storeColumns`(레지스트리/필터/헤더)·`storeEntrySort`(정렬)는 그대로 재사용.

### 색상 프리셋 (heatmap 전용)

- 이름있는 gradient 프리셋 상수(신규): Viridis / Turbo / Warm / Cool + 기존 기본(`DEFAULT_COLOR_TABLE`).
  각 프리셋은 ColorStop 배열이며 heatmap 색상표 형식과 호환.
- heatmap 옵션 영역에서만 프리셋 선택 UI + 커스텀 ColorStop 편집을 노출(REQ-12 조건부).

### 라이브 미리보기 상태 모델

- `draft` config(편집 중) vs `committed` config(저장됨) 분리. 옵션/색상/데이터소스 변경은 debounce 후 draft
  갱신 → 미리보기 실 패널 재렌더. 저장 = draft→committed 승격, 취소 = draft 폐기·committed 복원.
- 선택 계열 상한: 라이브 성능 가드로 합리적 상한(예: 수십 개)을 명시하고 초과 시 안내.

### 추적성 (Traceability)

- REQ-01 → 3분할 셸 컨테이너(`PanelSettingsDialog` 골격 교체), 옵션/데이터소스 슬롯 이관
- REQ-02/03 → 4방향 리사이즈 스플리터, 패널별 localStorage 비율 영속/복원
- REQ-04 → 데이터소스 영역 Store 선택 → `StoreSourceConfig` 반영
- REQ-05 → TSDB 토글 placeholder(비활성 안내), Store 설정 보존
- REQ-06/09 → 공용 `StoreEntryTable`(`storeColumns`/`storeEntrySort` 재사용), 컨텍스트별 컬럼 세트(actions↔Alias)
- REQ-07 → 행 선택 체크박스 → 선택 집합 → `StoreSourceConfig`(선택 계열)
- REQ-08 → 공용 리스트 필터/정렬/표시숨김 + localStorage 영속/복원
- REQ-10/11/12 → gradient 프리셋 상수 + 프리셋/커스텀 UI(heatmap 조건부), 차트 5종 미노출
- REQ-13/14 → draft/committed 상태 모델, debounce 미리보기, 실 store 데이터, 저장/취소 롤백

## 참고 (References)

- design.md — 3분할 레이아웃 구조, `StoreEntryTable` 추출 경계·공유 계약, `StoreSourceConfig` additive 스키마,
  라이브 미리보기 상태 모델, TSDB placeholder 확장 지점.
- research.md — 재사용 자산 인용(경로/규모/개조 방식), TSDB 통합 추상화 현황, Store 리스트 강결합 분석.
- plan.md — 작업 분해(T1~T10), 위험표(R1~R7), 마일스톤.
- acceptance.md — Given/When/Then 인수 기준 + 엣지 + TRUST5 게이트.
