---
id: SPEC-PANEL-SETTINGS-001
version: "0.5.0"
status: draft
created: 2026-08-08
updated: 2026-08-11
author: xtra
priority: P2
lifecycle_level: spec-anchored
title: "Panel Settings 재설계 — 구현 계획 (plan)"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, settings, plan, refactor, live-preview, store-table, frontend]
---

# SPEC-PANEL-SETTINGS-001 — 구현 계획 (plan.md)

> 관련: spec.md(요구사항/명세), design.md(설계), research.md(재사용 분석), acceptance.md(인수 기준).
> 방법론: hybrid(신규 코드 TDD / 기존 코드 DDD 특성화). 시간 예측 없음 — 우선순위·마일스톤 기반.

## 기술 접근 (Technical Approach)

- 프론트엔드 전용 변경(백엔드 무변경). 패널 config 는 불투명 JSON, `StoreSourceConfig` 는 additive 확장만.
- 대규모 골격(`PanelSettingsDialog.tsx` 4,370L)은 **셸만 교체하고 기존 옵션/데이터소스 편집 슬롯을 이관**한다
  (내부 편집 로직 재작성 금지 — R7 완화).
- 에이전트 상세의 Store 리스트 로직을 **공용 `StoreEntryTable`** 로 추출하되, 추출은 **행위 보존 리팩터**로
  진행하고 특성화 테스트로 에이전트 상세 회귀 0 을 방어한다(DDD).
- 신규 순수/독립 코드(gradient 프리셋 상수, 비율 영속 훅, 미리보기 draft 상태 모델)는 TDD 로 작성.

## 작업 분해 (Tasks)

| ID | 작업 | 대상(주요 파일) | REQ | 방법론 |
|----|------|-----------------|-----|--------|
| T1 | 3분할 셸 재작성 — 다이얼로그 본문을 미리보기/옵션/데이터소스 3영역 컨테이너로 교체(기존 슬롯 이관) | `PanelSettingsDialog.tsx` | REQ-01 | DDD(셸)+TDD(슬롯 배선) |
| T2 | 좌우·상하 2경계 드래그 리사이즈 스플리터(좌측 컬럼↔옵션 + 미리보기↔데이터소스) | 신규 스플리터 컴포넌트, `PanelSettingsDialog.tsx` | REQ-02 | TDD |
| T3 | 비율 영속/복원 — 패널별 localStorage 키, 손상/부재 폴백 | 신규 비율 훅 | REQ-02, REQ-03 | TDD |
| T4 | 데이터소스 Store/TSDB 토글 + TSDB 비활성 안내 placeholder | 데이터소스 영역, i18n | REQ-04, REQ-05 | TDD |
| T5 | 공용 `StoreEntryTable` 추출 — `AgentDetailPanel` 렌더/정렬/필터를 단일 소스로 | 신규 `StoreEntryTable`, `AgentDetailPanel.tsx`, `storeColumns.tsx`, `storeEntrySort.ts` | REQ-06, REQ-08 | DDD(특성화 우선) |
| T6 | 체크박스 + actions→Alias 컬럼 치환 + 선택→`StoreSourceConfig` 반영 | `StoreEntryTable`(컨텍스트 주입), `chartChannelTypes.ts` | REQ-07, REQ-09, REQ-04 | TDD |
| T7 | 필터/정렬/표시숨김 배선 + localStorage 영속/복원 | `StoreEntryTable`, `storeColumns.tsx` | REQ-08 | DDD+TDD |
| T8 | gradient 프리셋 상수(4~5종) + 선택 UI(heatmap 조건부) + 커스텀 ColorStop | 신규 프리셋 상수, heatmap 옵션 섹션 | REQ-10, REQ-11, REQ-12 | TDD |
| T9 | 라이브 미리보기 파이프라인 — draft/committed 상태, debounce, 실 store 데이터, 저장/취소 롤백, 선택 상한 가드 | 미리보기 영역, draft 상태 훅 | REQ-13, REQ-14 | TDD |
| T10 | 회귀 방어 특성화 테스트 + TRUST5 게이트(신규 85%, LSP 0) | 신규/기존 테스트, `AgentDetailPanel` 특성화 | 전 REQ | DDD |

### M6 — 데이터 소스 시리즈 선택 UI 개선 (추가 작업)

> 현행 구현 기준점(v0.2.0): 시리즈 선택은 `PanelSettingsDataSource.tsx`(체크박스로 `store_source.series[]`
> 선택, 컬럼 `['key','metric']`+합성 `alias` `:214-219`, heatmap 센서 좌표 `:544-600`), 시리즈 세부 편집은
> `ChartPanelSections.tsx:913` `SelectedSeriesList`. 태그 필터는 `tag_filters`(키당 단일값·크로스키 AND).

| ID | 작업 | 대상(주요 파일) | REQ | 방법론 |
|----|------|-----------------|-----|--------|
| T11 | 통일된 표시 필터 매처 "같은 차원 다중값 OR + 차원 간 AND" — 필터 상태 `Record<dimId, Set<value>>`(구축된 `storeColumnValueFilter.ts`). **태그는 태그 키(종류)별 차원화**: 현행 tags=단일 컬럼(`PanelSettingsDataSource.tsx:114` `PANEL_FILTER_COLUMNS`) 폐기, 각 태그 키를 자기 필터 차원으로. 태그 필터를 표시 필터에 편입하되 모드 자동 전환 제거. `tag_filters` 는 `Record<string,string>` 유지(다중값-per-키=표시 상태, 바인딩 파생 best-effort last-value) | `PanelSettingsDataSource.tsx:114`(태그 컬럼→키별 차원), 필터 파이프라인 `:235-`+암묵 전환 `:437-447` 제거, `storeColumns.tsx`(다중값 UI), `storeColumnValueFilter.ts` | REQ-15 | TDD(매처 순수함수·진리표) + DDD |
| T17 | 명시적 동적 바인딩 토글 + 표시/바인딩 분리 — 토글 UI(신규 OFF=keys/명시, ON=tag/poll 동적), ON 시 태그 기준을 태그-컬럼 필터에서 파생하여 `tag_filters` 구성, 하위호환 로드(저장 `selection_mode:'tag'`→토글 ON+`tag_filters` 보존), additive only | `PanelSettingsDataSource.tsx:437-447`(현행 암묵 전환→명시 토글), `useStoreChartData.ts:299/372`(동적 키 해석 경로), `chartChannelTypes.ts`(`StoreSourceConfig`) | REQ-15, REQ-22 | TDD |
| T12 | 사용자 표기 "별칭"→"이름" — i18n `colAlias` 등 라벨만 치환(필드명 불변) | `web/src/lib/i18n/{ko.json,en.json}`, 라벨 사용처 | REQ-16 | TDD(스냅샷) |
| T13 | 컬럼 순서 재배치 `[key, name(alias), metric, tag]` | `PanelSettingsDataSource.tsx:214-219` | REQ-17 | DDD |
| T14 | 선택 행 **인라인 펼침 세부 정보**로 재구성 — 이름(편집 텍스트→`StoreSeriesRef.alias` draft·범례)·색상·선 스타일 편집을 선택 행 펼침으로 이관. 별도 그룹 `StoreSourceEditor`(`ChartPanelSections.tsx:517`)/`SelectedSeriesList`(`:574`) **제거**. 미선택 행 펼침 없음, 기본 접힘. **레이아웃**: 이름 옆 색상 입력(`:929-936`) 제거+색상 컨트롤 이동(색상 편집 유지), 좌표 이름 뒤 2열, 폰트 최소 sm | `PanelSettingsDataSource.tsx`/`StoreEntryTable.tsx`(행 펼침 렌더), `ChartPanelSections.tsx:517/574/670/929-936`, `chartChannelTypes.ts`(StoreSeriesRef) | REQ-18, REQ-20 | DDD(행위 보존 이관)+TDD |
| T15 | heatmap 센서 좌표(x/y) 편집을 **선택 행 펼침** 안 이름 뒤 2열로 통합(heatmap 조건부) | `PanelSettingsDataSource.tsx:556-565`(`SensorPositionInputs`), `ChartPanelSections.tsx:955-956`, `heatmapConfig.ts`(`SensorPosition`, `sensor_positions`) | REQ-21 | DDD(이관)+TDD |
| T16 | **키 설명 서브라인 제거**(REQ-19 폐지) — 이름 위 키·종류·태그 나열 라인(`ChartPanelSections.tsx:903-914` `SeriesDetailEditor`) 제거. 키는 키 컬럼+이름 컬럼 mono 에코로만 노출. `keyRowExpansion` 미도입 | `ChartPanelSections.tsx:903-914` | REQ-19(폐지) | DDD |
| T18 | 세부 정보 ↔ 바인딩 모드 **결합 제거** — 세부 정보 keys-모드 게이트(`ChartPanelSections.tsx:572`) 제거하여 행 펼침 편집을 `selection_mode`/토글과 독립화 | `ChartPanelSections.tsx:572`, `PanelSettingsDataSource.tsx` | REQ-22(정제) | DDD |

### 의존 순서

- T5(공용 추출)는 T6·T7 의 선행이다. T1(셸)은 T2·T3·T4·T8·T9 의 컨테이너를 제공한다.
- T10(특성화)은 T5 착수 **전** 에이전트 상세 현행 동작 스냅샷을 확보하고(PRESERVE), T5 이후 회귀 검증에 재사용한다.
- 순서 권장: T10(특성화 스냅샷 확보) → T5 → {T1 → T2 → T3 → T4} 병행 가능 → T6 → T7 → T8 → T9 → T10(회귀 확정).
- M6: T14(선택 행 인라인 펼침 세부 정보)는 T6(체크박스·선택→config)에 의존하고, T15(위치 통합)·T16(키 상세
  통합)·T18(세부 정보 독립화)는 T14 의 행 펼침 구조에 의존한다. T17(동적 바인딩 토글)은 T11(표시 필터 통일)
  이후 착수. T12(표기)·T13(컬럼 순서)은 독립. M6 권장 순서: T11 → T17 → T13 → T12 → T14 → T16 → T15 → T18.

## 마일스톤 (우선순위 기반 — 시간 예측 없음)

- **Primary Goal (Priority High)**: T10(특성화 스냅샷) + T5(공용 `StoreEntryTable` 추출, 에이전트 상세 회귀 0) +
  T1(3분할 셸). — 재사용 자산의 단일 소스화와 셸 골격 확보가 나머지 작업의 토대.
- **Secondary Goal (Priority High)**: T6 + T7(체크박스·Alias·필터/정렬/영속) + T4(데이터소스 토글·TSDB placeholder).
- **Tertiary Goal (Priority Medium)**: T2 + T3(좌우·상하 2경계 리사이즈·비율 영속) + T9(라이브 미리보기 draft/롤백).
- **Final Goal (Priority Medium)**: T8(heatmap 색상 프리셋·커스텀) + T10 회귀 확정(TRUST5, LSP 0).
- **Optional Goal (Priority Low)**: 선택 상한 안내 문구 다듬기, i18n en/ko 대칭 보강.
- **M6 Goal (Priority Medium)**: T11(표시 필터 통일) + T17(동적 바인딩 토글·표시/바인딩 분리) 선행, 이어
  T13(컬럼 순서) + T12(표기) + T14(선택 행 인라인 펼침 세부 정보) + T16(키 상세 통합) + T15(센서 좌표 통합) +
  T18(세부 정보↔바인딩 모드 독립화). — 시리즈 선택 UI 개선 요구그룹(REQ-15~22).

## 위험 (Risks)

| ID | 위험 | 영향 | 완화 |
|----|------|------|------|
| R1 | Store 리스트가 `AgentDetailPanel`(4,616L)에 강결합 — 추출 비용/회귀 위험 | 높음 | 공용 `StoreEntryTable` 단일 소스 추출 + 특성화 테스트(T10) 선행, 컨텍스트 주입으로 분기 |
| R2 | `StoreSourceConfig` 확장이 기존 차트/heatmap 소비처(useStoreChartData 등)에 파급 | 중 | additive only + 미설정 시 기존 동작 보장, 하위호환 파싱 |
| R3 | 라이브 미리보기가 잦은 편집/폴링에서 성능 저하 | 중 | 편집 debounce + 격자/데이터 memoize + 선택 계열 합리적 상한(수십 개) |
| R4 | 좌우·상하 2경계 리사이즈에서 비율 정합(최소 크기, 경계 반올림) 깨짐 | 중 | 비율 정규화·최소 크기 클램프, 손상 값 폴백(REQ-03) |
| R5 | TSDB placeholder 를 실동작으로 오인 | 낮음 | 명시적 "후속 SPEC" 안내 문구 + 선택 시 Store 설정 보존(REQ-05) |
| R6 | 기존 차트/heatmap 설정 편집 회귀(옵션 섹션 슬롯 이관 중) | 높음 | 셸만 교체·슬롯 이관(로직 보존), 기존 섹션 테스트 통과 유지 |
| R7 | `PanelSettingsDialog`(4,370L) 골격 재작성 범위 과다 | 중 | 셸 컨테이너만 교체하고 편집 슬롯은 이관(재작성 아님), 단계적 PR 분할 가능 |
| R8 | 필터 OR/AND 일반화가 기존 `tag_filters`(키당 단일값·크로스키 AND) 소비처와 미리보기 바인딩 집합을 어긋나게 함 | 해소(v0.3.0) | **명시적 표시/바인딩 분리(REQ-15+REQ-22)로 해소**: 표시 필터는 `tag_filters` 소비처와 무관(모드 자동 전환 제거), 동적 바인딩은 명시 토글로만 활성화, 기존 tag 모드는 토글 ON+`tag_filters` 보존 로드. 잔여: 매처 순수함수 TDD + 하위호환 로드 테스트 |
| R9 | 세부 정보를 선택 행 인라인 펼침으로 재구성(`SelectedSeriesList`/`StoreSourceEditor` 제거 + 키 설명 서브라인 제거 + 색상 컨트롤 이동 + 좌표 2열 이동 + keys-모드 게이트 제거) 중 기존 편집/heatmap 좌표 편집/색상 편집 회귀 | 높음 | 행위 보존 이관(DDD), 이관 전 스냅샷 특성화, 색상 기능·`color` config 불변 확인, heatmap 조건부 노출 유지, 단일 편집 지점(행 펼침) 검증, 미선택 행 펼침 부재 + 바인딩 모드 독립 확인 |

## 기술 스택 (참고)

- React 19 / TypeScript 5.9 / Vite·Vitest / Tailwind (프로젝트 기존 스택). 신규 npm 의존성 목표 0.
- 상세 라이브러리 버전 확정은 구현(run) 단계에서 확인. 본 SPEC 은 프론트엔드 전용, 백엔드 무변경.

## 완료 조건 요약

- REQ-01~22 전량 구현(REQ-19 폐지). 신규 코드 커버리지 85%+, tsc `--noEmit`/eslint 0.
- 에이전트 상세 Store 리스트 회귀 0(특성화 테스트 통과). 기존 차트/heatmap 설정 편집 회귀 0.
- 색상 프리셋 heatmap 외 미노출. TSDB 선택 시 placeholder + Store 설정 보존. 신규 npm 의존성 0, 백엔드 무변경.
