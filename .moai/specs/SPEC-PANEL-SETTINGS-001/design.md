---
id: SPEC-PANEL-SETTINGS-001
version: "0.1.0"
status: draft
created: 2026-08-08
updated: 2026-08-08
author: xtra
priority: P2
lifecycle_level: spec-anchored
title: "Panel Settings 재설계 — 기술 설계 (design)"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, settings, design, architecture, store-table, live-preview, frontend]
---

# SPEC-PANEL-SETTINGS-001 — 기술 설계 (design.md)

> Tier L 설계 문서. spec.md 명세를 아키텍처 수준으로 구체화한다. 구현 세부는 run 단계에서 확정.

## 1. 3분할 레이아웃 구조

```
┌───────────────── PanelSettingsDialog (3분할 셸) ─────────────────┐
│  ┌──────────┬──────────────────────────┬────────────────────┐   │
│  │ 미리보기 │        옵션 (options)       │   데이터소스        │   │
│  │ (1/4)    │        (2/4·3/4)            │   (4/4)             │   │
│  │          │                            │                    │   │
│  │ 실 패널  │  패널 타입별 섹션 슬롯:     │  Store / TSDB 토글  │   │
│  │ draft    │  - 차트5종 (ChartPanel-    │  ┌──────────────┐  │   │
│  │ render   │    Sections)               │  │ StoreEntry   │  │   │
│  │ + 실     │  - heatmap 섹션            │  │ Table (공용)  │  │   │
│  │ store    │  - heatmap: 색상 프리셋    │  │ [chk] Alias  │  │   │
│  │ data     │    (조건부)                │  └──────────────┘  │   │
│  └────↕─────┴──────────↕─────────────────┴─────────↕──────────┘   │
│     좌우·상하 드래그 스플리터 (비율 → 패널별 localStorage)          │
└──────────────────────────────────────────────────────────────────┘
```

- **컨테이너 교체 원칙**: `PanelSettingsDialog.tsx`(4,370L)의 다이얼로그 **본문 레이아웃만** 3영역 셸로 교체.
  기존 옵션 섹션 컴포넌트(`ChartPanelSections` 의 `StatChartSection`/`LineChartSection`/`BarChartSection`/
  `PieChartSection`/`TableChartSection`, 공용 `StoreSourceSection`, heatmap 섹션)는 **슬롯으로 이관**하고 내부
  편집 로직은 재작성하지 않는다(R6/R7 완화).
- **영역 슬롯 계약**:
  - `PreviewSlot(panelType, draftConfig, realStoreData)` — 실 패널 컴포넌트를 draft 로 렌더.
  - `OptionsSlot(panelType, draftConfig, onChange)` — panelType 분기로 기존 섹션 렌더 + (heatmap 시) 프리셋 섹션.
  - `DataSourceSlot(dataSourceMode, storeSelection, onSelect)` — Store/TSDB 토글 + 공용 `StoreEntryTable`.

### 1.1 4방향 리사이즈 + 비율 영속

- 스플리터는 좌우(미리보기↔옵션, 옵션↔데이터소스) + 상하(필요 시 옵션 내부) 경계에 배치.
- 비율 상태: `{ preview, options, dataSource }`(합=1로 정규화, 각 영역 최소 크기 클램프).
- 영속 키: `panel-settings-ratio:<panelId>`(패널별). 손상/부재 시 기본 비율 폴백(REQ-03).
- 신규 훅 `usePanelSettingsRatio(panelId)` — {ratio, setRatio, resetToDefault} + localStorage 직렬화/파싱 방어.

## 2. `StoreEntryTable` 추출 경계 (에이전트 상세 ↔ 패널 설정 공유 계약)

### 2.1 추출 원본 (강결합 분해 대상)

`web/src/pages/agents/AgentDetailPanel.tsx`(4,616L) 내부:
- `StoreEntryRow`(2,414L~): 단일 행 렌더.
- `renderCell(columnId)`(2,605L~): 컬럼별 셀 렌더. `case 'actions'`(2,686/2,755L)가 액션 버튼 렌더.
- thead/tbody 단일 목록 공유(2,994L~ 주석: "헤더와 본문이 동일한 목록을 공유하는 단일 출처").
- 정렬/필터/컬럼 표시숨김 상태 + `storeColumns.tsx`(667L: `STORE_COLUMNS`/`ColumnSettingsMenu`/
  `ColumnFilterButton`/`ColumnHeader`) + `storeEntrySort.ts`(386L) 소비.

### 2.2 공유 컴포넌트 계약 (신규 `StoreEntryTable`)

```
StoreEntryTable(props):
  entries:        StoreEntry[]            // 공통
  columns:        StoreColumnId[]         // 컨텍스트별 컬럼 세트 (주입)
  renderCellExtra?: (col, entry) => Node  // 컨텍스트별 셀 오버라이드 (Alias 등)
  sortState / onSortChange                // storeEntrySort 재사용
  filterState / onFilterChange            // storeColumns 필터 재사용
  columnVisibility / onVisibilityChange   // 표시숨김 (localStorage 영속)
  selection?: { selected, onToggle }      // 패널 설정 전용 (체크박스). 미주입 시 미표시
  persistKey?: string                     // 필터/정렬/표시숨김 영속 스코프
```

- **컨텍스트 분기(주입으로 해결)**:
  - 에이전트 상세: `columns` 에 `actions` 포함, `selection` 미주입 → 기존 동작 그대로(회귀 0).
  - 패널 설정: `columns` 에서 `actions`→`alias` 치환, `selection` 주입(행 체크박스) → 선택 결과를 상위로.
- **행위 보존 리팩터(DDD)**: 추출 전 `AgentDetailPanel` 의 관측 가능한 동작(정렬/필터/표시숨김/렌더)을 특성화
  테스트로 캡처(PRESERVE) → 추출(IMPROVE) → 동일 테스트 통과로 회귀 0 검증.
- `storeColumns`/`storeEntrySort` 는 이동하지 않고 공용 테이블이 참조(중복 방지).

## 3. `StoreSourceConfig` additive 스키마 (`chartChannelTypes.ts:97`)

- 기존 `StoreSourceConfig`(패널의 `store_source?: StoreSourceConfig`, :164)에 **선택 계열(체크박스 결과)**
  표현을 additive 필드로 추가. 기존 필드/의미 불변, 미설정 시 기존 동작과 동일(하위호환 파싱).
- 소비처(예: `useStoreChartData.ts:270`, 각 차트 패널)는 신규 필드 미설정 시 기존 경로를 그대로 사용해야 하며,
  신규 필드가 있을 때만 선택 계열을 반영(R2 완화).
- **색상 프리셋은 `StoreSourceConfig` 와 독립** — heatmap config 의 색상표(ColorStop[])에만 반영.

## 4. 라이브 미리보기 상태 모델 (draft vs committed)

```
committedConfig  ──(edit)──▶  draftConfig ──(debounce)──▶ PreviewSlot(실 패널 render + 실 store data)
     ▲                            │
     └───────(cancel: 폐기·복원)──┘
     ▲                            │
     └───────(save: 승격)─────────┘
```

- `draftConfig`: 편집 중 config. 옵션/색상/데이터소스 변경 → debounce(예: 성능 가드) → draft 갱신 → 미리보기 재렌더.
- `committedConfig`: 저장된 config. 취소 시 draft←committed 복원, 저장 시 committed←draft 승격.
- 미리보기는 **실 store 데이터**(기존 프리뷰 방식: 실제 패널 컴포넌트를 draft 로 렌더)를 사용.
- **선택 상한 가드**: 선택 계열이 합리적 상한(예: 수십 개)을 넘으면 안내 + 초과 반영 억제(R3).
- 신규 훅 `useDraftPanelConfig(committed)` — {draft, patch, save, cancel, isDirty}.

## 5. TSDB placeholder 확장 지점

- 데이터소스 토글에 Store/TSDB 노출. TSDB 선택 시 **비활성 안내 placeholder**(후속 SPEC 안내 문구, i18n)만 렌더.
- 본 SPEC 은 TSDB 를 실제로 바인딩·질의하지 않는다. 후속 SPEC 진입점(참고, 본 SPEC 비활성):
  `web/src/services/api/seriesDataSource.ts`, `web/src/services/api/tsdb.ts`, `web/src/hooks/useTsdb.ts`.
- 확장 계약: `DataSourceSlot` 의 `dataSourceMode ∈ {store, tsdb}` 로 분기하되 tsdb 브랜치는 placeholder 렌더로
  고정(후속 SPEC 에서 실 바인딩으로 대체). TSDB 선택이 기존 Store 설정을 파괴하지 않도록 상태 분리(REQ-05).

## 6. 색상 프리셋 설계 (heatmap 전용)

- 신규 프리셋 상수(예: `heatmapColorPresets.ts`): `{ id, name, stops: ColorStop[] }[]` — Viridis / Turbo /
  Warm / Cool + 기존 기본(`idw.ts:25 DEFAULT_COLOR_TABLE`).
- heatmap 옵션 섹션에서만 프리셋 선택 + 커스텀 ColorStop 편집 노출(REQ-12: 차트 5종 미노출).
- 프리셋 선택 → heatmap draft 색상표(ColorStop[])로 반영 → 미리보기 갱신(REQ-11).

## 7. 컴포넌트/파일 계획 요약

| 종류 | 파일 | 비고 |
|------|------|------|
| 신규 | `StoreEntryTable.tsx`(경로 run 확정) | 공용 리스트(단일 소스) |
| 신규 | 3분할 셸 컨테이너 + 스플리터 컴포넌트 | `PanelSettingsDialog` 본문 |
| 신규 | `usePanelSettingsRatio` / `useDraftPanelConfig` 훅 | 비율 영속 / draft 상태 |
| 신규 | `heatmapColorPresets` 상수 | gradient 프리셋 |
| 확장 | `PanelSettingsDialog.tsx` | 셸 골격 교체, 슬롯 이관 |
| 확장 | `AgentDetailPanel.tsx` | 공용 테이블 사용으로 치환(행위 보존) |
| 확장 | `chartChannelTypes.ts` | `StoreSourceConfig` additive |
| 확장 | `ChartPanelSections.tsx` | 옵션 슬롯 재배치(로직 보존) |
| 재사용 | `storeColumns.tsx` / `storeEntrySort.ts` / `colorSwatchPalette.tsx` / `idw.ts` | 이동 없이 참조 |
| 확장 | i18n `ko.json`/`en.json` | 데이터소스/프리셋/placeholder 키 |

> 구체 경로/네이밍은 run 단계에서 프로젝트 규약에 맞춰 확정. 본 문서는 경계·계약을 고정한다.
