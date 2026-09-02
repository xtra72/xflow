---
id: SPEC-PANEL-SETTINGS-001
version: "0.5.0"
status: in-progress
created: 2026-08-08
updated: 2026-08-11
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
| 2026-08-08 | 0.1.0 | 최초 작성. Store 바인딩 패널(line/bar/pie/stat/table + heatmap) 설정 UX 를 3분할 셸(좌측 상단 미리보기·좌측 하단 데이터소스·우측 옵션)로 재편. 좌우·상하 2경계 드래그 리사이즈 + 패널별 localStorage 비율 영속. 에이전트 상세(`AgentDetailPanel`)의 Store 리스트 렌더/정렬/필터를 **공용 `StoreEntryTable`** 로 추출(단일 소스), actions 컬럼→Alias Name 치환 + 행 선행 체크박스. heatmap 전용 gradient 색상 프리셋(4~5종 + 커스텀). 실 store 데이터 기반 라이브 미리보기(디바운스·미확정·롤백). TSDB 데이터소스 토글은 placeholder+후속 안내만(실동작 후속 SPEC). EARS 5모듈/14 REQ. | xtra |
| 2026-08-11 | 0.2.0 | **M6 — 데이터 소스 시리즈 선택 UI 개선** 요구그룹 추가(7 REQ, REQ-15~21). 컬럼 필터 시맨틱을 "같은 컬럼 다중값 OR + 컬럼 간 AND" 로 확장(기존 태그 필터의 키당 단일값 크로스키 AND 를 일반화). 사용자 표기 "별칭(alias)"→"이름(name)"(데이터 필드 `alias` 유지). 선택 테이블 컬럼 순서 key·name·metric·tag 재배치. 각 시리즈 "시리즈별 세부 정보" 편집 영역 신설 — 기존 별도 섹션 `SelectedSeriesList`(별칭/색상/선 스타일)와 heatmap 센서 좌표(x/y) 편집을 이 영역으로 통합. 키 컬럼 행별 상세 확장(기본 펼침) 표시. 현행 구현 기준점은 `PanelSettingsDataSource.tsx`/`ChartPanelSections.tsx`. EARS 6모듈/21 REQ. | xtra |
| 2026-08-11 | 0.2.1 | M6 사용자 리뷰 반영 2건. (1) REQ-19 반전: 키 컬럼 상세를 **기본 펼침 → 기본 접음(collapsed)** 으로 변경, 필요 시 사용자가 펼쳐서(expand) 전체 상세 확인(여전히 행별 접기/펼치기, 그룹 트리 아님). (2) 이름 편집 명시: "이름(name, 저장 필드 `alias`)"은 "시리즈별 세부 정보" 영역에서 **시리즈별로 편집 가능한 텍스트 필드**이며 편집 결과가 `StoreSeriesRef.alias` 로 draft 반영·차트 범례/표시에 반영(REQ-18 명시, REQ-16 상호참조). | xtra |
| 2026-08-11 | 0.3.0 | **"표시 필터 통일 + 바인딩 분리"** 시맨틱 채택(사용자 결정). REQ-15 정제: 모든 컬럼 필터(key/name/metric/tag)는 선택 테이블에 대한 **표시(display) 필터**(같은 컬럼 OR·컬럼 간 AND)이며, 태그 컬럼 필터가 **단독으로 패널을 동적 바인딩 모드(`selection_mode:'tag'`)로 전환하지 않는다**. 선택은 명시적 체크박스(keys 모드)로 유지. **REQ-22 신설**: 명시적 "동적 바인딩(dynamic binding)" 토글 — OFF(신규 패널 기본)=keys/명시 선택, ON=poll 시 태그 기준으로 키 동적 해석(`selection_mode:'tag'` + `tag_filters`), ON 시 태그 기준은 태그-컬럼 필터 값에서 파생. 하위호환: 기존 `selection_mode:'tag'` 저장 패널은 토글 ON + `tag_filters` 보존으로 로드(동작 불변). additive only, `selection_mode`/`tag_filters` 제거 금지. R8 은 명시적 분리로 해소. M6 = REQ-15~22(8 REQ). | xtra |
| 2026-08-11 | 0.4.0 | **M6 "시리즈별 세부 정보" 배치 모델 정정**(앱 검증 후 사용자 피드백). 기존 "별도 그룹 섹션 + 키 상세만 펼침 + keys 모드 게이팅" 모델이 오류였음. 정정: (REQ-18) 시리즈별 세부 정보(이름 편집/색상/선스타일/(heatmap)위치)는 별도 그룹이 아니라 **선택된(체크된) 각 시리즈 행의 인라인 펼침/접힘 상세**로 제공(미선택 행은 상세 없음, 기본 접힘). (REQ-19) 별도 "키 상세만" 펼침(`keyRowExpansion`) 폐기, 키 전체 상세를 REQ-18 시리즈 세부 정보와 **동일 행 펼침에 통합**. (REQ-20) 별도 `SelectedSeriesList` 섹션 **제거** — 단일 편집 지점=행 펼침. (REQ-22) 세부 정보 편집은 동적 바인딩 토글/`selection_mode` 와 **완전 독립**(keys-모드 게이팅 `ChartPanelSections.tsx:572` 제거). design.md §8 에 tag 모드 시리즈 스타일 영속(key 기준, best-effort) 설계 고려 기록. additive only. M6 = REQ-15~22(8 REQ). | xtra |
| 2026-08-11 | 0.5.0 | **M6 앱 재검증 후 정제 3건.** (1) REQ-15 태그 필터 시맨틱 정제: "태그" 컬럼을 단일 컬럼으로 보지 않고 **태그 키(종류)별** 로 — 같은 태그 키 내 다중 값 OR, 서로 다른 태그 키 간 AND(각 태그 키가 자기 AND 차원). 예: `device_id ∈ {A,B}` AND `type ∈ {report}`. (2) **REQ-19 폐지(void/superseded)**: 행 펼침 안의 키·종류·태그 설명 라인(이름 위)이 테이블 컬럼(키/메트릭/태그)+alias 컬럼 키 에코와 중복 → 제거. 키는 키 컬럼+alias 컬럼 mono 에코로만 노출, 펼침에서 미중복. (3) REQ-18 정제: 펼침 상세 내용은 시리즈 편집 필드(이름 편집+색상+선스타일+(heatmap)좌표)만 — 키/종류/태그 설명 서브라인 없음. 레이아웃: 이름 옆 색상 입력 제거(색상 편집은 선 스타일/세부 영역 다른 위치로 이동, `color` config 불변), (heatmap)좌표는 이름 뒤 2열 레이아웃(이름|좌표), 세부 영역 폰트 최소 sm(본문) 수준. additive only, `tag_filters` 는 `Record<string,string>` 유지(다중값-per-키는 표시 필터 상태 관심사, 바인딩 파생은 기존 best-effort last-value). M6 = REQ-15~22(REQ-19 폐지, 재번호 없음). | xtra |

## 개요 (Overview)

대시보드의 **Store 바인딩 패널** 설정 경험을 재설계한다. 현재 설정 UI(`PanelSettingsDialog.tsx`, 4,370L)는
차트 5종(stat/line-chart/bar-chart/pie-chart/table)과 heatmap 의 config 편집 섹션을 단일 다이얼로그에
수직 누적하는 골격이다. 본 SPEC 은 이 다이얼로그 골격을 **3분할 셸**로 교체하여 **좌측 컬럼**에 (1) 상단 **라이브 미리보기**(실제
패널을 draft config 로 렌더)와 (2) 그 아래 **데이터소스 선택**(Store 리스트 + TSDB 토글 placeholder)을
세로로 배치하고, **우측**에 (3) 패널 타입별 **옵션 편집**을 배치한다. 즉 미리보기와 데이터소스가 옵션의
좌측에 상하로 놓이고, 옵션이 우측 넓은 영역을 차지한다.

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
| A8 | 좌우·상하 2경계 드래그 리사이즈 비율은 패널별로 localStorage 에 영속되고 재진입 시 복원된다 | 확정 | 오케스트레이터 확정 세부 기본값 |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Unwanted / Optional)을 6개 모듈에 표현한다.
총 22 REQ (M1~M5 = REQ-01~14, M6 = REQ-15~22; **REQ-19 는 v0.5.0 에서 폐지(VOID), 재번호 없음**).

### M1 — 3분할 레이아웃 셸

#### REQ-01 — 3분할 셸 구성 (Ubiquitous)

시스템은 **항상** Store 바인딩 패널 설정을 3분할 셸로 제공해야 한다: **미리보기 영역**(실 패널을 draft config
로 렌더), **옵션 영역**(패널 타입별 설정 섹션), **데이터소스 영역**(Store 리스트 + TSDB 토글). 기본 배치는
**좌측 컬럼을 상하로 분할하여 상단=미리보기 / 하단=데이터소스**, **우측=옵션** 이다(미리보기와 데이터소스가
옵션의 좌측에 세로로 놓임). 시스템은 **항상** 기존 옵션/데이터소스 편집 슬롯을 이 셸의 해당 영역에 이관하여,
편집 기능 손실 없이 배치만 재구성해야 한다.

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

### M6 — 데이터 소스 시리즈 선택 UI 개선

> 현행 구현 기준점: 시리즈 선택 테이블 `web/src/pages/dashboard/PanelSettingsDataSource.tsx`(체크박스로
> `store_source.series[]` 선택, 컬럼 제한 `['key','metric']` + 합성 `alias` 컬럼 `:214-219`, heatmap 센서 좌표
> 편집 `:544-600`), 시리즈 세부 편집 `web/src/pages/dashboard/ChartPanelSections.tsx:913`(`SelectedSeriesList`),
> 태그 필터 `tag_filters`(키당 단일값·크로스키 AND) `StoreTagSelectionEditor`(`ChartPanelSections.tsx:603`).

#### REQ-15 — 통일된 표시(display) 필터 OR/AND 결합 시맨틱 (State-Driven)

**IF** 사용자가 시리즈 선택 테이블의 컬럼 필터(key/name/metric/tag 전부)에 하나 이상의 조건을 설정하면
**THEN** 시스템은 그것을 **표시(display) 필터**로 취급하여 **같은 컬럼 내 다중 값은 OR** 로, **서로 다른 컬럼
간은 AND** 로 결합하여 선택 테이블에 매칭 행만 표시해야 한다. 즉 어떤 행이 통과하려면 필터가 걸린 **모든 컬럼
각각에서 그 컬럼에 지정된 값들 중 적어도 하나**를 만족해야 한다. **IF** 특정 컬럼에 필터가 없으면 **THEN** 그
컬럼은 AND 결합에서 제외(항상 통과)해야 한다.

**태그(tags) 필터는 태그 키(종류)별로 결합해야 한다** — "태그" 를 하나의 컬럼으로 뭉치지 않고, **각 태그 키가
자기 AND 차원**이 된다. 즉 **같은 태그 키(종류) 내 다중 값은 OR**, **서로 다른 태그 키 간은 AND** 로 결합한다.
예: `device_id ∈ {A, B}` **AND** `type ∈ {report}` → 어떤 행이 통과하려면 `(device_id=A OR device_id=B) AND
(type=report)` 를 만족해야 한다. 이는 일반 규칙(같은 컬럼 OR / 컬럼 간 AND)과 일관되며, 각 태그 키를 자기
컬럼처럼 취급한 것이다.

시스템은 **항상** 태그 필터를 이 통일된 표시 필터의 일부로만 취급해야 하며, 태그 필터가 **단독으로 패널을 동적
바인딩 모드(`selection_mode:'tag'`)로 전환하지 않아야 한다**. 즉 표시 필터와 실제 바인딩은 분리된 관심사이며,
선택(바인딩)은 명시적 체크박스 선택(`series[]`, keys 모드)으로 유지된다. 동적 바인딩 활성화는 REQ-22 의 명시적
토글로만 제어한다. (미설정 시 기존 표시 동작과 동일. **하위호환**: `tag_filters` 는 `Record<string,string>` 유지 —
태그 키당 다중값은 **표시 필터 상태**의 관심사이며, 동적 바인딩 파생 시 태그 키당 값은 기존 best-effort
last-value 로 기록.)

#### REQ-16 — "이름(name)" 표기 (Ubiquitous)

시스템은 **항상** 시리즈 선택/세부 정보 UI 에서 "별칭(alias)" 용어·컬럼 라벨을 **"이름(name)"** 으로 표시해야
한다. 데이터 모델 필드명(`StoreSeriesRef.alias`, `StoreColumn` id `alias`)은 **유지**하고 사용자 표기(i18n
라벨)만 변경해야 한다. 즉 저장 스키마·하위호환에는 영향을 주지 **않는다**. 본 요구는 **표기 라벨 변경**에
한정하며, 이름 **값의 편집 가능성**은 REQ-18(시리즈별 세부 정보)에서 규정한다.

#### REQ-17 — 컬럼 순서 재배치 (Ubiquitous)

시스템은 **항상** 시리즈 선택 테이블의 컬럼을 **키(key) · 이름(name/alias) · 메트릭(metric) · 태그(tag)** 순서로
표시해야 한다. (현행 `[key, metric, (tags), alias]` 배치를 위 순서로 재배치.)

#### REQ-18 — 시리즈별 세부 정보 = 선택 행 인라인 펼침 상세 (Ubiquitous)

시스템은 **항상** 시리즈별 세부 정보(이름 편집 / 색상 / 선 스타일 / (heatmap 시)좌표)를, 별도 그룹·섹션이
아니라 **선택된(체크된) 각 시리즈 행의 인라인 펼침/접힘(expand/collapse) 상세**로 제공해야 한다. **IF** 사용자가
선택 행을 펼치면 **THEN** 시스템은 그 시리즈의 이름(편집 가능 텍스트)·색상·선 스타일·(heatmap 시)좌표 편집
필드를 그 행 안에 인라인으로 노출해야 하며, 접으면 숨겨야 한다. 기본값은 **접힘**이다. **미선택(미체크) 행은
펼침 상세를 제공하지 않아야 한다**. "이름(name)"은 편집 가능한 텍스트 필드로, 편집 결과를 해당 시리즈의
`StoreSeriesRef.alias` 로 draft 반영하고 미리보기·차트 범례/표시에 반영해야 한다(REQ-16 표기 라벨과 상호참조).
그 밖의 편집 결과도 해당 시리즈의 `StoreSeriesRef`(및 heatmap 좌표 메타)로 draft 반영해야 한다.

펼침 상세의 내용은 **시리즈 편집 필드(이름 편집 + 색상 + 선 스타일 + (heatmap)좌표)만** 포함해야 하며, 이름
위에 키·종류(type)·태그를 나열하는 **설명 서브라인을 포함하지 않아야 한다**(REQ-19 폐지 — 테이블 컬럼 및 이름
컬럼 키 에코와 중복). 키는 키(key) 컬럼과 이름(alias) 컬럼의 mono 키 에코로만 노출된다.

세부 정보 편집기 **레이아웃 요구**(구체 px 는 구현 재량):

- **이름 옆의 색상 입력은 제거**하되 **시리즈별 색상 편집 기능은 유지**한다. 색상 컨트롤을 선 스타일/세부 영역의
  다른 위치로 이동하며, 색상 기능 자체·config `color` 필드는 불변(additive, 기능 보존).
- **(heatmap)좌표(x/y)는 이름 뒤쪽으로 이동하여 2열 레이아웃**(이름 | 좌표)으로 배치한다 — 하단 별도 배치가
  아니다.
- 세부 정보 영역의 **폰트를 가독성 있는 크기로 확대**한다 — 세부 편집 텍스트·라벨은 **최소 본문(sm) 수준**
  이상이어야 한다.

#### REQ-19 — (폐지 / VOID) 키 전체 상세를 시리즈 세부 정보에 통합 — superseded by v0.5.0

**본 요구는 v0.5.0 에서 폐지(void)되었다.** (앱 재검증 결과, 행 펼침 안에서 이름 위에 키·종류·태그를 나열하는
설명 라인이 테이블 컬럼(키/메트릭/태그) 및 이름(alias) 컬럼의 키 에코와 중복되어 제거하기로 결정.) 키의 전체
상세를 행 펼침에 표시하지 **않는다**. 키는 **키(key) 컬럼 + 이름(alias) 컬럼 mono 키 에코**로만 노출되며, 펼침
상세에는 시리즈 편집 필드만 남는다(REQ-18 참조). 현행 `keyRowExpansion` 또한 도입하지 않는다(폐지). 재번호는
하지 않으며 REQ-20~22 는 그대로 유지된다.

#### REQ-20 — 별도 SelectedSeriesList 섹션 제거 (Unwanted)

시스템은 선택 시리즈 세부 편집을 위한 **별도의 `SelectedSeriesList` 섹션(그룹)을 노출하지 않아야 한다**. 단일
편집 지점은 **선택된 시리즈 행의 인라인 펼침 상세**(REQ-18)이며, 동일 편집 항목(이름/색상/선 스타일)을 별도
섹션으로 중복 표시하지 **않아야 한다**.

#### REQ-21 — 센서 좌표 → 세부 정보 통합 (Optional / heatmap 전용)

**Where** 편집 대상 패널이 heatmap 이면 시스템은 현재 heatmap 전용 별도 영역에서 편집하던 **센서 좌표(x/y)**
편집을 **"시리즈별 세부 정보" 영역으로 통합**하여, 각 선택 시리즈의 세부 정보와 같은 위치에서 편집하도록
제공해야 한다. heatmap 이 아닌 패널에서는 센서 좌표 필드를 노출하지 **않아야 한다**.

#### REQ-22 — 명시적 동적 바인딩(dynamic binding) 토글 (State-Driven)

시스템은 **항상** 데이터소스 영역에 표시 필터와 **분리된** 명시적 **"동적 바인딩(dynamic binding)" 토글**을
제공하여, 패널이 poll 시점에 매칭 키를 동적으로 해석할지 여부를 이 토글로만 제어해야 한다.

- **IF** 토글이 **OFF** 이면 **THEN** 시스템은 `selection_mode:'keys'` 로 동작하여 명시적 체크박스 선택
  (`series[]`)만 바인딩하고, 컬럼 필터는 표시 전용(display-only)으로만 작동해야 한다. **신규 패널의 기본값은
  OFF** 이다.
- **IF** 토글이 **ON** 이면 **THEN** 시스템은 동적 바인딩을 활성화하여 poll 시점에 태그 기준으로 매칭 키를
  해석(`selection_mode:'tag'`, 기존 `tag_filters` 메커니즘)해야 한다. 이때 태그 기준은 **태그-컬럼 필터 값에서
  파생**된다.
- **IF** 기존 저장 패널이 `selection_mode:'tag'` 로 저장되어 있으면 **THEN** 시스템은 로드 시 토글을 **기본
  ON** 으로 설정하고 그 패널의 `tag_filters` 를 **보존**하여, 기존 태그 모드 패널의 동작을 바꾸지 **않아야
  한다**(하위호환).

시스템은 스키마를 **additive only** 로 확장해야 하며, `selection_mode` 또는 `tag_filters` 필드를 제거하지
**않아야 한다**.

또한 시스템은 시리즈별 세부 정보(REQ-18 행 인라인 펼침 편집)를 이 토글 및 `selection_mode`(keys/tag)와
**완전히 독립**적으로 제공해야 한다. 즉 세부 정보 편집 접근성은 바인딩 모드에 의해 숨겨지거나 게이팅되지
**않아야 한다**(현행 "세부 정보가 keys 모드에서만 렌더" 결합 제거). 동적 바인딩은 **어떤 키가 포함되는지**만
제어하고, 세부 정보는 시리즈 스타일/이름 편집으로 독립적으로 동작한다.

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
- 좌우·상하 2경계 리사이즈 스플리터(좌측 컬럼↔옵션 세로 경계 + 미리보기↔데이터소스 가로 경계)와 패널별 localStorage 비율 영속/복원.

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

### M6 — 시리즈 선택 UI 개선 명세

- **통일된 표시 필터 OR/AND(REQ-15)**: 필터 상태를 `Record<columnId, Set<value>>` 로 일반화(이미 구축된
  `storeColumnValueFilter.ts` 매처). 매처는 필터가 걸린 각 컬럼에 대해 `row[col] ∈ values[col]`(같은 컬럼 OR)을
  검사하고, 필터가 걸린 모든 컬럼을 `AND` 로 결합한다. **태그는 태그 키(종류)별 차원화**: "태그" 를 단일 컬럼으로
  보는 현행(`PANEL_FILTER_COLUMNS` 가 tags 를 한 컬럼으로 취급, `PanelSettingsDataSource.tsx:114`)을 폐기하고, 각
  태그 키를 자기 필터 차원으로 확장 — 같은 태그 키 값 OR, 태그 키 간 AND(예: `device_id ∈ {A,B}` AND
  `type ∈ {report}`). **핵심 분리**: 태그 필터는 이 통일된 표시 필터 상태에만 기여하며, **단독으로 `selection_mode`
  를 전환하지 않는다**(현행 암묵 전환 `PanelSettingsDataSource.tsx:437-447` 제거). 미설정 차원은 매칭에서 제외
  (항상 통과). `tag_filters` 는 `Record<string,string>` 유지(태그 키당 다중값은 표시 필터 상태 관심사; 바인딩 파생
  시 태그 키당 best-effort last-value).
- **동적 바인딩 토글(REQ-22)**: 표시 필터와 분리된 명시적 토글로 `selection_mode` 를 제어. OFF(신규 기본)=
  `'keys'`(체크박스 `series[]` 명시 바인딩, 컬럼 필터는 표시 전용), ON=`'tag'`(poll 시 동적 해석 —
  `useStoreChartData.ts:299/372` 의 기존 태그 매칭 경로). ON 시 태그 기준은 태그-컬럼 필터 값에서 파생하여
  `tag_filters` 로 기록. 하위호환: 로드 시 저장된 `selection_mode:'tag'` → 토글 ON + `tag_filters` 보존.
  additive only(`selection_mode`/`tag_filters` 유지). **세부 정보 독립성**: REQ-18 행 인라인 펼침 세부 편집은
  이 토글/`selection_mode` 와 무관하게 접근 가능 — 현행 keys-모드 게이트(`ChartPanelSections.tsx:572`) 제거.
- **표기 "이름"(REQ-16) + 값 편집(REQ-18)**: i18n 키(`ko.json`/`en.json`)의 `colAlias`(및 관련 라벨)만
  "이름"/"Name" 으로 변경. 타입 필드 `StoreSeriesRef.alias`(`chartChannelTypes.ts:65`)·`StoreColumn` id `'alias'`
  는 불변. 저장 config 무변경. 이름 **값**은 "시리즈별 세부 정보" 영역의 편집 가능한 텍스트 입력으로 노출하고,
  편집 결과를 `StoreSeriesRef.alias` 로 draft 반영(미리보기·범례 반영).
- **컬럼 순서(REQ-17)**: `PanelSettingsDataSource.tsx:214-219` 의 `columns` 조립을 `[key, alias(name), metric,
  (tags)]` 순으로 재배치. 표시가능/숨김 로직·`STORE_COLUMNS` 레지스트리는 재사용.
- **시리즈별 세부 정보 = 선택 행 인라인 펼침(REQ-18/20/21)**: 별도 컴포넌트/섹션이 아니라 **선택된 행의 인라인
  펼침 상세**로 구현한다. 별도 그룹 `StoreSourceEditor`(`ChartPanelSections.tsx:517`)/`SelectedSeriesList`(`:574`)을
  **제거**하고, 편집 항목(이름 편집 텍스트→`StoreSeriesRef.alias`, 색상, 선 스타일)을 선택 행 펼침으로 이관(REQ-20).
  미선택 행은 펼침 없음. heatmap 좌표(x/y) 편집(`SensorPositionInputs` `PanelSettingsDataSource.tsx:556-565`,
  `SensorPosition {x,y}` `heatmapConfig.ts:35`, `HeatmapPanelConfig.sensor_positions:137`)도 같은 펼침 안 시리즈
  필드로 통합(REQ-21 조건부). 기본 접힘. 행 펼침 상태는 로컬 UI 상태(예: `Set<seriesKey>`).
- **펼침 내용 정제 + 레이아웃(REQ-18, REQ-19 폐지)**: 이름 위의 키·종류·태그 설명 라인(`SeriesDetailEditor`
  `ChartPanelSections.tsx:903-914`)을 **제거**(REQ-19 폐지 — 컬럼/이름 에코와 중복). 펼침 내용은 시리즈 편집
  필드만. 레이아웃: 이름 옆 색상 입력(`ChartPanelSections.tsx:929-936`) 제거하되 색상 편집은 선 스타일/세부
  영역 다른 위치로 이동(색상 기능·`color` 불변; `LineStyleControls:670` 에는 색상 없음 → 색상 컨트롤 별도 배치).
  (heatmap)좌표는 하단(`ChartPanelSections.tsx:955-956` + `PanelSettingsDataSource.tsx:556-565`)에서 **이름 뒤 2열
  (이름|좌표)**로 이동. 세부 영역 폰트는 최소 sm(본문) 이상.
- **세부 정보 ↔ 바인딩 모드 독립(REQ-22 정제)**: 현행 세부 정보 keys-모드 게이트(`ChartPanelSections.tsx:572`)를
  제거하여, 행 펼침 세부 편집을 `selection_mode`(keys/tag)·동적 바인딩 토글과 무관하게 노출한다.

### 추적성 (Traceability)

- REQ-01 → 3분할 셸 컨테이너(`PanelSettingsDialog` 골격 교체), 옵션/데이터소스 슬롯 이관
- REQ-02/03 → 좌우·상하 2경계 리사이즈 스플리터, 패널별 localStorage 비율 영속/복원
- REQ-04 → 데이터소스 영역 Store 선택 → `StoreSourceConfig` 반영
- REQ-05 → TSDB 토글 placeholder(비활성 안내), Store 설정 보존
- REQ-06/09 → 공용 `StoreEntryTable`(`storeColumns`/`storeEntrySort` 재사용), 컨텍스트별 컬럼 세트(actions↔Alias)
- REQ-07 → 행 선택 체크박스 → 선택 집합 → `StoreSourceConfig`(선택 계열)
- REQ-08 → 공용 리스트 필터/정렬/표시숨김 + localStorage 영속/복원
- REQ-10/11/12 → gradient 프리셋 상수 + 프리셋/커스텀 UI(heatmap 조건부), 차트 5종 미노출
- REQ-13/14 → draft/committed 상태 모델, debounce 미리보기, 실 store 데이터, 저장/취소 롤백
- REQ-15 → 통일된 표시 필터(같은 컬럼/태그 키 OR·컬럼/태그 키 간 AND) `storeColumnValueFilter.ts`; 태그 키별 차원화(현행 tags=단일 컬럼 `PanelSettingsDataSource.tsx:114` 폐기); `storeColumns.tsx` 다중값 UI; 태그 필터 암묵 모드 전환 제거(`:437-447`); `tag_filters` 는 `Record<string,string>` 유지(바인딩 파생 best-effort last-value)
- REQ-22 → 명시적 동적 바인딩 토글 → `selection_mode` 제어(OFF=keys/명시, ON=tag/poll 동적 `useStoreChartData.ts:299/372`), 태그 기준은 태그-컬럼 필터에서 파생, 하위호환 로드(tag 모드→토글 ON+`tag_filters` 보존), additive only; 세부 정보는 바인딩 모드와 독립(keys-모드 게이트 `ChartPanelSections.tsx:572` 제거)
- REQ-16 → i18n `colAlias` 라벨 "이름"/"Name" 치환(표기만), `StoreSeriesRef.alias`/컬럼 id `alias` 불변
- REQ-17 → `PanelSettingsDataSource.tsx:214-219` 컬럼 순서 `[key, name(alias), metric, tag]` 재배치
- REQ-18/20/21 → 선택 행 인라인 펼침 상세(편집 필드만: 이름 편집 텍스트→`StoreSeriesRef.alias` draft·범례 반영, 색상, 선 스타일, (heatmap)좌표); 별도 그룹 `StoreSourceEditor`(`ChartPanelSections.tsx:517`)/`SelectedSeriesList`(`:574`) 제거; 미선택 행 펼침 없음; 레이아웃 — 이름 옆 색상 입력(`:929-936`) 제거+색상 컨트롤 이동, 좌표(`:955-956`,`PanelSettingsDataSource.tsx:556-565`) 이름 뒤 2열, 폰트 최소 sm
- REQ-19 → **폐지(VOID, v0.5.0)**: 이름 위 키·종류·태그 설명 라인(`ChartPanelSections.tsx:903-914`) 제거; 키는 키 컬럼+이름 컬럼 mono 에코로만 노출; `keyRowExpansion` 미도입

## 참고 (References)

- design.md — 3분할 레이아웃 구조, `StoreEntryTable` 추출 경계·공유 계약, `StoreSourceConfig` additive 스키마,
  라이브 미리보기 상태 모델, TSDB placeholder 확장 지점.
- research.md — 재사용 자산 인용(경로/규모/개조 방식), TSDB 통합 추상화 현황, Store 리스트 강결합 분석.
- plan.md — 작업 분해(T1~T10), 위험표(R1~R7), 마일스톤.
- acceptance.md — Given/When/Then 인수 기준 + 엣지 + TRUST5 게이트.
