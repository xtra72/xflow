---
id: SPEC-PANEL-SETTINGS-001
version: "0.1.0"
status: draft
created: 2026-08-08
updated: 2026-08-08
author: xtra
priority: P2
lifecycle_level: spec-anchored
title: "Panel Settings 재설계 — 코드베이스 분석 (research)"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, settings, research, reuse, coupling-analysis, tsdb, frontend]
---

# SPEC-PANEL-SETTINGS-001 — 코드베이스 분석 (research.md)

> 재사용 자산의 **경로/규모/개조 방식**을 실측(직접 파일 관측)으로 기록한다. 아래 경로·라인 수치는
> 본 SPEC 작성 시점에 `wc -l` / `grep` 로 직접 확인한 값이다.
> 주의: 기획 단계에서 일부 경로가 `src/pages/dashboard/` 로 가정되었으나, **Store 리스트 자산은 실제로는
> `src/pages/agents/` 에 위치**함을 실측으로 정정하였다(§2).

## 1. 설정 다이얼로그 골격

| 파일 | 규모 | 역할 | 개조 방식 |
|------|------|------|-----------|
| `web/src/pages/dashboard/PanelSettingsDialog.tsx` | 4,370L | 패널 설정 다이얼로그 골격(옵션·데이터소스·프리뷰 누적) | 본문 레이아웃을 3분할 셸로 교체, 기존 편집 섹션은 슬롯 이관(로직 보존) |
| `web/src/pages/dashboard/PanelSettingsPage.tsx` | 30L | 다이얼로그 진입점(얇은 래퍼) | 변경 최소 |
| `web/src/pages/dashboard/ChartPanelSections.tsx` | 2,896L | 차트 5종 옵션 섹션 + 공용 `StoreSourceSection`/`ChartChannelSection` | 옵션 슬롯으로 재배치(재작성 아님) |

- `PanelSettingsDialog.tsx` 은 `ChartPanelSections` 에서 `ChartChannelSection`, `StoreSourceSection`,
  `StatChartSection`, `LineChartSection`, `BarChartSection`, `PieChartSection`, `TableChartSection` 을 import
  (관측: import 블록 81~91L). 데이터소스 섹션(`StoreSourceSection`)은 좌측 프리뷰 아래로 이동된 이력 주석 존재
  (570L, "SPEC-WEB-005").
- 패널 타입 분기는 `web/src/pages/dashboard/renderDashboardPanel.tsx` 258~269L 에서
  `stat`/`line-chart`/`bar-chart`/`pie-chart`/`table`/`heatmap` 을 각각 case 로 처리(관측). → 본 SPEC 대상 패널
  집합과 일치.

## 2. Store 리스트 강결합 분석 (추출 대상)

**핵심 발견(정정)**: Store 엔트리 리스트 로직은 대시보드가 아니라 **에이전트 상세 화면**(`src/pages/agents/`)에
있다. 패널 설정과 공유하려면 이 위치에서 공용 컴포넌트로 추출해야 한다.

| 파일 | 규모 | 내용(관측) |
|------|------|-----------|
| `web/src/pages/agents/AgentDetailPanel.tsx` | 4,616L | `StoreEntryRow`(2,414L~), `renderCell(columnId)`(2,605L~), `case 'actions'`(2,686/2,755L), thead/tbody 단일 목록 공유(2,994L~ 주석 "헤더와 본문이 동일한 목록을 공유하는 단일 출처"), `<StoreEntryRow>` 사용(3,619L) |
| `web/src/pages/agents/storeColumns.tsx` | 667L | `STORE_COLUMNS` 레지스트리 + `ColumnSettingsMenu`/`ColumnFilterButton`/`ColumnHeader`(표시숨김·필터·헤더) |
| `web/src/pages/agents/storeEntrySort.ts` | 386L | 정렬 비교자 |

- 강결합 성격: 렌더(`StoreEntryRow`/`renderCell`) + 정렬/필터/표시숨김 상태가 `AgentDetailPanel` 내부에 인라인.
  단, thead/tbody 가 이미 "단일 출처"로 정리되어 있고 `storeColumns`/`storeEntrySort` 가 분리 파일이라 **추출
  가능성이 높다**(design.md §2 공유 계약).
- 추출 리스크: `renderCell` 의 `case 'actions'` 는 에이전트 상세 고유(액션 버튼). 패널 설정은 이를 Alias 컬럼으로
  치환해야 하므로 **컨텍스트별 컬럼 세트/셀 오버라이드 주입**이 필요(단일 소스 유지 + 분기).
- 완화: 추출 전 특성화 테스트(정렬/필터/표시숨김/렌더 스냅샷)로 에이전트 상세 현행 동작 캡처 → 회귀 0 방어(R1).

## 3. 데이터소스 / Store 바인딩 현황

| 파일 | 관측 | 본 SPEC 관계 |
|------|------|--------------|
| `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts` | `interface StoreSourceConfig`(97L), 패널 `store_source?: StoreSourceConfig`(164L) | additive 확장 지점(선택 계열) |
| `web/src/pages/dashboard/panels/charts/useStoreChartData.ts` | `export function useStoreChartData(...)`(270L) | Store 폴링 소비처(바인딩 방식 불변) |
| `StoreSourceSection`(ChartPanelSections 소재, PanelSettingsDialog 913L 사용) | 공용 데이터소스 섹션 컴포넌트 | 데이터소스 영역 슬롯 재사용 |

- `StoreSourceConfig` 는 다수 소비처(`LineChartPanel`/`BarChartPanel`/`PieChartPanel`/`StatPanel`/`TablePanel`/
  `heatmapConfig`/`HeatmapPanel` 등, grep 관측)에서 참조되므로 확장은 반드시 **additive + 미설정 시 기존 동작
  보장**이어야 한다(R2).

## 4. TSDB 통합 추상화 현황 (후속 SPEC 진입점, 본 SPEC 비활성)

실측: TSDB 관련 모듈이 이미 존재하나 대시보드 패널 데이터소스로는 미연결.

| 파일 | 위치 | 비고 |
|------|------|------|
| `web/src/services/api/seriesDataSource.ts` | services/api | 시계열 데이터소스 추상화 |
| `web/src/services/api/tsdb.ts` (+ `tsdb.test.ts`) | services/api | TSDB API |
| `web/src/hooks/useTsdb.ts` | hooks | TSDB 훅 |
| `web/src/pages/agents/tsdbChartNullHandling.ts`, `tsdbCsvExport.ts` | agents | 에이전트 화면의 기존 TSDB 사용례 |

- 본 SPEC 은 이들을 **호출하지 않는다**. 데이터소스 토글에 TSDB 를 노출하되 선택 시 placeholder(후속 안내)만
  렌더한다(REQ-05). 후속 SPEC 에서 `DataSourceSlot` 의 tsdb 브랜치를 위 모듈로 실제 바인딩할 수 있도록 확장
  지점만 남긴다(design.md §5).

## 5. 색상 자산

| 파일 | 관측 | 사용 |
|------|------|------|
| `web/src/pages/dashboard/panels/heatmap/idw.ts` | `export const DEFAULT_COLOR_TABLE: ColorStop[]`(25L), 빈 colorTable 시 폴백(143L) | 기본 gradient 프리셋 원본 |
| `web/src/pages/dashboard/colorSwatchPalette.tsx` | 95L, `export const COLOR_PALETTE`(11L), `ColorSwatchButton` | 단색 스와치(선/계열 색) 재사용 |

- gradient 프리셋(Viridis/Turbo/Warm/Cool)은 신규 상수로 추가하되 `DEFAULT_COLOR_TABLE` 의 ColorStop 형식과
  호환시킨다. 단색 팔레트(`COLOR_PALETTE`)는 프리셋과 별개(선 색 등)로 유지.

## 6. 요약 (설계 반영 포인트)

1. `PanelSettingsDialog`(4,370L)·`AgentDetailPanel`(4,616L)은 대형 골격 — **셸 교체·슬롯 이관 + 행위 보존
   추출**로 접근(재작성 금지, R6/R7).
2. Store 리스트 자산은 `src/pages/agents/` 소재 → 공용 `StoreEntryTable` 로 추출, 컨텍스트 주입으로 분기(R1).
3. `StoreSourceConfig` 는 광범위 소비처 → additive only + 하위호환(R2).
4. TSDB 모듈은 이미 존재하나 본 SPEC 은 미연결(placeholder + 확장 지점, R5).
5. 색상: `DEFAULT_COLOR_TABLE`(ColorStop) 호환 gradient 프리셋 신규 상수 + heatmap 조건부 노출.

> 관측 방법: 각 경로에 대해 `wc -l`(라인 수)과 `grep -n`(심볼 위치)로 직접 확인. 기획 단계 가정 경로 중
> Store 리스트 3파일(`storeColumns`/`AgentDetailPanel`/`storeEntrySort`)은 `dashboard/` 가 아닌 `agents/`
> 소재로 정정됨.
