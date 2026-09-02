---
id: SPEC-PANEL-SETTINGS-001
version: "0.5.0"
status: draft
created: 2026-08-08
updated: 2026-08-11
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
│  ┌──────────────────┬────────────────────────────────────────┐  │
│  │ 미리보기 (좌상단)  │                                        │  │
│  │ 실 패널 draft      │          옵션 (options, 우측)           │  │
│  │ render + 실 store  │                                        │  │
│  │ data               │   패널 타입별 섹션 슬롯:                 │  │
│  ├───────↕──────────┤   - 차트5종 (ChartPanelSections)        │  │
│  │ 데이터소스 (좌하단)│   - heatmap 섹션                        │  │
│  │ Store / TSDB 토글  │   - heatmap: 색상 프리셋 (조건부)        │  │
│  │ ┌──────────────┐  │                                        │  │
│  │ │ StoreEntry    │  │                                        │  │
│  │ │ Table (공용)   │  │                                        │  │
│  │ │ [chk] Alias   │  │                                        │  │
│  │ └──────────────┘  │                                        │  │
│  └──────────────────┴────────────────────────────────────────┘  │
│    ↕ 상하(미리보기↔데이터소스)   ↔ 좌우(좌측 컬럼↔옵션)             │
│    드래그 스플리터 (비율 → 패널별 localStorage)                    │
└──────────────────────────────────────────────────────────────────┘
```

레이아웃: **좌측 컬럼**을 상하로 분할하여 상단=미리보기 / 하단=데이터소스, **우측**=옵션.
경계는 2개 — 좌측 컬럼과 옵션 사이의 **좌우(세로) 경계**, 좌측 컬럼 내부 미리보기와
데이터소스 사이의 **상하(가로) 경계**.

- **컨테이너 교체 원칙**: `PanelSettingsDialog.tsx`(4,370L)의 다이얼로그 **본문 레이아웃만** 3영역 셸로 교체.
  기존 옵션 섹션 컴포넌트(`ChartPanelSections` 의 `StatChartSection`/`LineChartSection`/`BarChartSection`/
  `PieChartSection`/`TableChartSection`, 공용 `StoreSourceSection`, heatmap 섹션)는 **슬롯으로 이관**하고 내부
  편집 로직은 재작성하지 않는다(R6/R7 완화).
- **영역 슬롯 계약**:
  - `PreviewSlot(panelType, draftConfig, realStoreData)` — 실 패널 컴포넌트를 draft 로 렌더.
  - `OptionsSlot(panelType, draftConfig, onChange)` — panelType 분기로 기존 섹션 렌더 + (heatmap 시) 프리셋 섹션.
  - `DataSourceSlot(dataSourceMode, storeSelection, onSelect)` — Store/TSDB 토글 + 공용 `StoreEntryTable`.

### 1.1 좌우·상하 2경계 리사이즈 + 비율 영속

- 스플리터 2개: **좌우 경계**(좌측 컬럼 ↔ 옵션) + **상하 경계**(좌측 컬럼 내부 미리보기 ↔ 데이터소스).
- 비율 상태: `{ leftColWidth, previewHeight }`(좌측 컬럼 폭 + 좌측 컬럼 내 미리보기 높이 비율; 각 영역 최소 크기 클램프, 손상 값 폴백).
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

## 8. 데이터 소스 시리즈 선택 UI 개선 (M6)

> 현행 구현 기준점(v0.2.0). 시리즈 선택 테이블은 원 설계의 `PanelSettingsDialog` 슬롯이 아니라 실제로는
> `web/src/pages/dashboard/PanelSettingsDataSource.tsx` 로 구현되어 있다. 아래 설계는 그 파일과
> `ChartPanelSections.tsx` 를 기준점으로 한다.

### 8.1 통일된 표시 필터 + 바인딩 분리 (REQ-15 + REQ-22)

> v0.3.0 채택 시맨틱: **"표시 필터 통일 + 바인딩 분리"**. 현행 구현은 태그 팝오버가 `tag_filters` 를 쓰면서
> 동시에 패널을 `selection_mode:'tag'`(동적 바인딩)로 **암묵 전환**(`PanelSettingsDataSource.tsx:437-447`)하여
> 두 관심사(표시 필터 vs 바인딩)를 뒤섞는다. 이를 **분리**한다.

#### 8.1.1 통일된 표시 필터 (REQ-15)

- **현행**: 태그 필터 `tag_filters: Record<string,string>`(키당 단일값), 크로스키 AND, `StoreTagSelectionEditor`
  (`ChartPanelSections.tsx:603`) + AND 매처 `storeSourceFilter.matchesTagFilters`(`PanelSettingsDataSource.tsx:236-`).
  컬럼 필터는 `filterColumn` 단위(태그 컬럼은 `filterColumn: undefined` 로 OR/AND 충돌 회피 — `:207-210`).
- **개선 설계(v0.5.0)**: 모든 필터를 **표시(display) 필터**로 통일하되, **차원(dimension)** 단위로 결합한다.
  일반 컬럼(key/name/metric)은 각자 1개 차원이고, **태그는 태그 키(종류)마다 별도 차원**이다(현행 tags=단일
  컬럼 `PANEL_FILTER_COLUMNS`(`PanelSettingsDataSource.tsx:114`) 폐기). 필터 상태를
  `Record<dimId, Set<value>>`(dimId = 컬럼 id 또는 `tag:<키>`)로 일반화(구축된 `storeColumnValueFilter.ts`).
  매처 계약:

  ```
  matches(row, filters):
    for each (dim, values) in filters where values.size > 0:
      if value_of(row, dim) ∉ values:  return false   // 같은 차원: OR(집합 포함)
    return true                                        // 차원 간: AND(모든 필터 차원 통과)
  ```

  - 같은 차원 다중값 = OR, 차원 간 = AND, 미설정 차원(빈 집합) = 스킵(항상 통과).
  - **태그 키별 차원 예**: `device_id ∈ {A,B}` AND `type ∈ {report}` → `tag:device_id`, `tag:type` 는 각자
    AND 차원, 값은 차원 내 OR.
  - **핵심**: 태그 필터는 이 통일된 표시 필터 상태에만 기여하며, **단독으로 `selection_mode` 를 전환하지
    않는다** — 암묵 전환(`:437-447`) 제거. 표시 필터는 어느 바인딩 모드에서도 테이블 표시 행만 좁힌다.
  - **하위호환**: `tag_filters` 는 `Record<string,string>` 유지 — 태그 키당 **다중값은 표시 필터 상태**의
    관심사(런타임 UI 상태)이며, 동적 바인딩 파생 시 태그 키당 값은 기존 best-effort **last-value** 로 기록
    (스키마 무변경, additive only).
- **UI**: `storeColumns.tsx` 컬럼 필터 버튼(`ColumnFilterButton`)이 다중 값 선택을 허용하도록 확장(체크형 다중선택).

#### 8.1.2 명시적 동적 바인딩 토글 (REQ-22)

- 데이터소스 영역에 표시 필터와 **분리된** 명시적 "동적 바인딩(dynamic binding)" 토글을 둔다. 이 토글만이
  `selection_mode` 를 제어한다.

  | 토글 | `selection_mode` | 바인딩 동작 | 컬럼 필터 역할 |
  |------|------------------|-------------|----------------|
  | **OFF** (신규 패널 기본) | `'keys'` | 체크박스 선택 `series[]` 명시 바인딩 | 표시 전용(display-only) |
  | **ON** | `'tag'` | poll 시 태그 기준으로 매칭 키 동적 해석(`useStoreChartData.ts:299/372`) | 태그-컬럼 필터 값 → 태그 기준 파생(`tag_filters`) |

- **ON 시 파생 규칙**: 동적 바인딩이 켜지면 태그-컬럼 필터 값에서 태그 기준을 파생하여 `tag_filters` 를
  구성한다(기존 메커니즘 재사용). 표시 필터의 태그 값이 바인딩 기준의 원천이 되지만, 이는 토글 ON 일 때만
  `selection_mode` 에 영향을 준다.
- **하위호환(중요)**: 로드 시 저장된 `selection_mode:'tag'` 패널은 토글을 **기본 ON** 으로 설정하고 `tag_filters`
  를 보존한다 → 기존 태그 모드 패널의 동작 불변. **additive only**: `selection_mode`/`tag_filters` 필드 제거 금지;
  토글 상태는 `selection_mode` 로부터 유도 가능(별도 신규 필드 불요, 필요 시 additive 필드로만 저장).
- **R8 해소**: 표시(필터)와 바인딩(모드)을 명시 분리함으로써, 표시 필터 OR/AND 일반화가 `tag_filters` 소비처
  (`useStoreChartData.ts`)의 동적 바인딩과 더 이상 충돌하지 않는다.

### 8.2 표기 "이름(name)" (REQ-16) + 이름 값 편집 (REQ-18)

- **표기(REQ-16)**: i18n 라벨만 변경: `colAlias`(및 세부 편집 라벨) → "이름"/"Name". `StoreSeriesRef.alias`
  (`chartChannelTypes.ts:65`), `StoreColumn` id `'alias'`(`PanelSettingsDataSource.tsx:217`), 별칭 템플릿
  (`aliasTemplate.ts`)의 필드/식별자는 불변. 순수 표기 계층 변경 — 저장 스키마·하위호환 무영향.
- **값 편집(REQ-18)**: 이름은 read-only 라벨이 아니라 "시리즈별 세부 정보" 영역의 **편집 가능한 텍스트 입력**
  이다. 사용자가 이름을 편집하면 해당 시리즈의 `StoreSeriesRef.alias` 로 draft 반영하고, 미리보기·차트
  범례/표시에 반영한다. (필드명 `alias` 는 불변, 값만 사용자 편집.)

### 8.3 컬럼 순서 재배치 (REQ-17)

- `PanelSettingsDataSource.tsx:214-219` `columns` 조립을 `[key, alias(name), metric, (tags)]` 순으로 재배치.
  현행 `[key, metric, (tags), alias]` → `alias(name)` 을 `key` 바로 뒤로 이동. 표시가능/숨김·`STORE_COLUMNS` 재사용.

### 8.4 시리즈별 세부 정보 = 선택 행 인라인 펼침 상세 (REQ-18/20/21; REQ-19 폐지)

> v0.4.0: 세부 정보는 **별도 그룹/섹션이 아니라 선택된(체크된) 행의 인라인 펼침 상세**.
> v0.5.0 정제: 펼침 내용은 **시리즈 편집 필드만** — 이름 위 키·종류·태그 설명 라인 제거(REQ-19 폐지). 레이아웃
> 조정(색상 컨트롤 이동, 좌표 이름 뒤 2열, 폰트 확대).

```
┌─ 데이터소스 영역 ──────────────────────────────────┐
│  [Store | TSDB]  [ 동적 바인딩 토글 (REQ-22) ]       │
│  ┌ 시리즈 선택 테이블 ───────────────────────────┐  │
│  │ [chk] ▶ key │ name │ metric │ tag              │  │  ← 선택+접힘 (기본)
│  │ [chk] ▼ key │ name │ metric │ tag              │  │  ← 선택+펼침
│  │      └ 인라인 상세(편집 필드만, 폰트 ≥ sm):      │  │
│  │         [ 이름(name) 편집 →alias ] [ 좌표 x | y ]│  │  ← 이름|좌표 2열 (좌표는 heatmap)
│  │         [ 선 스타일 … | 색상 ]                   │  │  ← 색상은 이름 옆이 아님(이동)
│  │   (※ 키·종류·태그 설명 라인 없음 — REQ-19 폐지)  │  │
│  │ [ ]   key │ name │ metric │ tag                 │  │  ← 미선택: 펼침 없음
│  └────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────┘
```

- **단일 편집 지점 = 선택 행 인라인 펼침**. 제거 대상(별도 그룹): `StoreSourceEditor`(`ChartPanelSections.tsx:517`),
  `SelectedSeriesList`(`:574`). 이 그룹의 편집 항목(이름/색상/선 스타일)을 선택 행 펼침으로 이관(REQ-20).
  - 이름은 **편집 가능 텍스트 입력** → `StoreSeriesRef.alias` draft → 범례 반영(REQ-18). 색상/선 스타일 → 동일
    `StoreSeriesRef`. 미선택 행은 펼침 상세를 렌더하지 않는다.
  - **키 설명 서브라인 제거(REQ-19 폐지)**: 이름 위 키·종류·태그 나열 라인(`SeriesDetailEditor`
    `ChartPanelSections.tsx:903-914`) 제거 — 테이블 컬럼(키/메트릭/태그) 및 이름(alias) 컬럼 mono 키 에코와
    중복이기 때문. 키는 키 컬럼 + 이름 컬럼 에코로만 노출. `keyRowExpansion` 미도입.
  - **레이아웃(REQ-18)**: (a) 이름 필드 옆의 색상 입력(`ChartPanelSections.tsx:929-936`) 제거 — 단, 시리즈 색상
    편집 기능은 유지하여 색상 컨트롤을 선 스타일/세부 영역의 다른 위치로 이동(`LineStyleControls:670` 에는 색상
    없음 → 색상 컨트롤 별도 배치). `color` config 값·기능 불변(additive). (b) (heatmap)좌표(x/y)는 하단
    (`ChartPanelSections.tsx:955-956` + `SensorPositionInputs` `PanelSettingsDataSource.tsx:556-565`)에서 **이름 뒤
    2열(이름 | 좌표)**로 이동. (c) 세부 영역 폰트 최소 sm(본문) 이상.
  - **heatmap 좌표(REQ-21)**: `SensorPosition {x,y}`(`heatmapConfig.ts:35`), `HeatmapPanelConfig.sensor_positions`
    (`:137`) 편집을 같은 펼침 안 이름 뒤 2열 필드로 통합(heatmap 조건부, 차트 5종 미노출).
- **펼침 상태 모델**: 로컬 UI 상태(예: `Set<seriesKey>` of expanded selected rows). 선택/정렬/필터 상태와 독립,
  기본 접힘. 미선택 행은 펼침 대상에서 제외.
- **행위 보존 이관(DDD)**: 이관 전 기존 시리즈 편집/색상/좌표 편집 관측 동작을 특성화로 캡처 → 이관 →
  동일 테스트 통과로 회귀 0(R9 완화). 색상 기능 보존을 특성화로 확인.

### 8.5 세부 정보 ↔ 바인딩 모드 독립 + tag 모드 영속 고려 (REQ-22 정제)

- **독립성**: 현행 세부 정보 keys-모드 게이트(`ChartPanelSections.tsx:572` — 세부 정보가 keys 모드에서만 렌더)를
  **제거**한다. 행 인라인 펼침 세부 편집은 `selection_mode`(keys/tag)·동적 바인딩 토글과 무관하게 접근 가능하다.
  동적 바인딩은 **어떤 키가 포함되는지**만 제어하고, 세부 정보는 시리즈 스타일/이름 편집으로 독립 동작한다.
- **설계 고려사항 — tag 모드 시리즈 스타일/이름 영속 (강제 메커니즘 아님, 기록만)**: 동적 바인딩 ON(tag 모드)
  에서는 `series[]` 가 폴링 시점에 동적 생성되므로 시리즈 스타일/이름 편집의 **영속 대상이 모호**하다. 권장 방향:
  세부 정보는 **시리즈 key 기준으로 영속**(key→스타일/이름 매핑)하여 재해석 시 재적용. **주 경로는 동적 바인딩
  OFF(keys 모드)** — 여기서 행 펼침 편집이 완전히 동작해야 한다(명시 `series[]` 에 직접 영속). tag 모드의 세부
  편집은 **best-effort**(현재 해석된 key 기준 반영)로 기술하며, 구체 영속 스키마·매핑은 run 단계에서 확정한다
  (본 SPEC 은 additive only 유지, 강제 메커니즘 미지정).

### 8.6 M6 파일 계획 요약

| 종류 | 파일 | 비고 |
|------|------|------|
| 확장 | `PanelSettingsDataSource.tsx` | 통일 표시 필터(태그 키별 차원 — `PANEL_FILTER_COLUMNS:114`), 컬럼 순서, 선택 행 인라인 펼침 세부 정보, 좌표(`SensorPositionInputs:556-565`) 이름 뒤 2열 이관, 암묵 모드 전환(`:437-447`) 제거 + 명시 동적 바인딩 토글 |
| 확장 | `StoreEntryTable.tsx` | 선택 행 인라인 펼침 상세 렌더(편집 필드만) |
| 제거 | `ChartPanelSections.tsx:517`(`StoreSourceEditor`)/`:574`(`SelectedSeriesList`)/`:572`(keys-모드 게이트)/`:903-914`(키 설명 서브라인, REQ-19 폐지)/`:929-936`(이름 옆 색상 입력) | 별도 그룹·게이트·설명 라인·이름 옆 색상 제거; 색상 컨트롤은 이동(기능 유지) |
| 확장 | `storeColumns.tsx` | 컬럼 필터 다중값 선택 UI |
| 재사용/확장 | `storeColumnValueFilter.ts` | 통일 표시 필터 매처(같은 차원 OR·차원 간 AND; 태그 키별 차원) |
| 확장 | `chartChannelTypes.ts` | `StoreSourceConfig` — 세부 편집 draft 경로 + (필요 시) 토글 상태 additive 필드; `selection_mode`/`tag_filters` 유지 |
| 참조 | `useStoreChartData.ts:299/372` | 동적 바인딩(ON) 시 태그 기준 키 해석 경로(무변경 재사용) |
| 확장 | i18n `ko.json`/`en.json` | `colAlias`→"이름"/"Name", 동적 바인딩 토글 라벨 등 |
| 재사용 | `heatmapConfig.ts`(`SensorPosition`/`sensor_positions`), `aliasTemplate.ts` | 이동 없이 참조 |
