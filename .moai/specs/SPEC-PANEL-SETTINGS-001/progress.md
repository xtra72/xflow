# SPEC-PANEL-SETTINGS-001 — 진행 기록 (progress.md)

> 방법론: hybrid (기존 코드 DDD 특성화 / 신규 순수 코드 TDD). Tier L. 프론트엔드 전용.

## §E.2 Run-phase Evidence — Primary Goal 마일스톤 (T10 + T5 + T1)

### 범위 (이 마일스톤에서 수행)

- **T10 (PRESERVE, 선행)**: 에이전트 상세 Store 리스트의 현행 관측 동작을 특성화 테스트로 캡처(회귀 안전망, R1 완화).
- **T5 (행위 보존 추출)**: `AgentDetailPanel.tsx` 의 Store 리스트 렌더/행/셀/정렬·필터·컬럼 로직을 공용 단일 소스 `StoreEntryTable` 로 추출. 에이전트 상세 회귀 0(AC-16).
- **T1 (셸 골격 교체)**: `PanelSettingsDialog.tsx` 다이얼로그 본문을 명시적 3분할 셸(`PanelSettingsShell`)로 교체하고 기존 옵션/미리보기/데이터소스 편집 슬롯을 각 영역으로 이관(로직 보존, R6/R7 완화).

이번 마일스톤 밖(후속): T6/T7(체크박스·Alias·필터/정렬 영속), T4(데이터소스 토글·TSDB placeholder), T2/T3(리사이즈·비율 영속), T9(라이브 미리보기 draft/롤백), T8(heatmap 색상 프리셋).

### 파일 (files_created / files_modified)

**신규(files_created)**
- `web/src/pages/agents/StoreEntryTable.tsx` — 공용 Store 테이블(단일 소스). `StoreEntryRow` + `renderCell` + thead/tbody 단일 목록. 컨텍스트 주입: `rowActions`(에이전트 상세), `selection`/`renderCellExtra`(후속 T6/T7 오버라이드).
- `web/src/pages/agents/storeEntryHelpers.ts` — `formatTimeAgo`/`extractEntryTags`/`extractEntryMetricType` 순수 헬퍼(컴포넌트 파일과 분리 — react-refresh 규약 준수).
- `web/src/pages/agents/AgentDetailPanel.storeCharacterization.test.tsx` — T10 특성화(회귀 안전망).
- `web/src/pages/agents/StoreEntryTable.test.tsx` — `StoreEntryTable` 단위 테스트(히스토리 확장/selection/renderCellExtra/readOnly/액션/긴 값).
- `web/src/pages/dashboard/PanelSettingsDialog.shell.test.tsx` — T1 AC-01 3분할 셸 구성 테스트.

**확장(files_modified)**
- `web/src/pages/agents/AgentDetailPanel.tsx` — Store 리스트 렌더를 `<StoreEntryTable/>` 소비로 치환(행위 보존). 이동한 `StoreEntryRow`/헬퍼 제거, import 재배선, 미사용 아이콘(ArrowUpCircle/Tag) 정리.
- `web/src/pages/dashboard/PanelSettingsDialog.tsx` — 본문(428–922)을 `PanelSettingsShell` 로 추출, 옵션/미리보기/데이터소스 3영역 슬롯 이관. 영역 마커 testid(`panel-settings-options`/`panel-settings-preview`/기존 `panel-settings-data-source`) 부여. `TranslationFn` import 추가.

### characterization_tests_created

- `web/src/pages/agents/AgentDetailPanel.storeCharacterization.test.tsx` (7 tests) — actions 컬럼 매트릭스(정적/동적), binding 배지, storeEntrySort 정렬(asc/desc), 컬럼 표시/숨김 + localStorage 영속/복원. 추출 전/후 모두 통과(회귀 0).

### test_results (검증 증거)

- `cd web && npx tsc --noEmit` → **exit 0, 0 type errors** (baseline 0 유지).
- `cd web && npx eslint .` → **44 problems (1 error, 43 warnings)** = baseline 동일. 신규 발견 0. (1 error = 기존 `vite.config.js:10 __dirname no-undef`).
- `cd web && npx vitest run` → **227 files / 2880 tests 전량 통과**. (실패 0)
- 신규 코드 커버리지(`StoreEntryTable.tsx` + `storeEntryHelpers.ts`): **stmts 96.94% / branch 89.65% / funcs 91.66% / lines 96.94%** — 목표 ≥85% 충족.

### behavior_preserved (AC-16 회귀 0 증거)

- 추출 전 T10 특성화 7 tests가 **현행 코드에서 통과** → 추출 후 동일 7 tests 통과(무수정).
- 기존 `AgentDetailPanel.storeTab.test.tsx`(10), `storeColumns.test.ts`(16), `storeEntrySort.test.ts`(37) 통과 유지.
- 기존 `PanelSettingsDialog.*.test.tsx` + `ChartPanelSections.test.tsx` + `StoreSourceSection.test.tsx` + `StoreTagMode.test.tsx` 통과 유지(차트/heatmap 설정 편집 회귀 0).
- `PanelSettingsShell` 은 기존 본문 JSX를 verbatim 이관(상태·핸들러 동명 props 주입)하여 리사이즈/접기/줌 동작 보존.

### implementation_divergence (계획 대비 실제)

- **StoreEntryTable 경로**: design.md §7 는 `dashboard/` 를 제안했으나, `storeColumns.tsx`/`storeEntrySort.ts`(agents/)를 그대로 참조하고 추출 원본이 agents/ 이므로 **`web/src/pages/agents/StoreEntryTable.tsx`** 에 배치(단일 소스, import 최소 churn). 대시보드 측 소비는 후속 T6/T7 에서 import.
- **헬퍼 분리(신규 파일)**: `formatTimeAgo`/`extractEntryTags`/`extractEntryMetricType` 를 `storeEntryHelpers.ts` 로 분리. 이유: 컴포넌트+함수 동시 export 시 `react-refresh/only-export-components` 신규 경고 발생 → 신규 경고 0 유지 위해 분리.
- **`renderCellExtra` 의미**: design.md 는 "컨텍스트별 셀 오버라이드"로 명시. 기본값-분기(default case)만으로는 유효 `StoreColumnId` 에서 도달 불가하므로 **renderCell 진입 시 우선 호출(undefined 반환 시 기본 셀 유지)** 하는 오버라이드로 구현 — 후속 T6/T7 의 Alias 치환을 `alias` 컬럼 id 없이도 지원.
- **`selection`/`renderCellExtra` 는 forward-compat**: 에이전트 상세는 미주입(동작 불변). 후속 T6/T7 에서 패널 설정이 주입.
- **T1 셸 = 정적 강제 아님**: 마일스톤은 "정적 3분할로 충분"이라 명시하나, 기존 리사이즈/접기/줌은 동작 기능이므로 삭제하지 않고 셸로 verbatim 이관(회귀 0). 리사이즈/비율 **영속 확장**은 T2/T3 에서 셸 seam 위에 추가.
- **신규 디렉터리**: 없음. 모든 신규 파일은 기존 `agents/`·`dashboard/` 내.

### 다음 마일스톤(남은 작업)

- Secondary: T6 + T7(행 체크박스·actions→Alias·필터/정렬/표시숨김 영속 + `StoreSourceConfig` additive) + T4(데이터소스 Store/TSDB 토글·TSDB placeholder).
- Tertiary: T2 + T3(좌우·상하 2경계 드래그 리사이즈·패널별 localStorage 비율 영속) + T9(draft/committed 라이브 미리보기·롤백·선택 상한).
- Final: T8(heatmap gradient 프리셋 4~5종 + 커스텀 ColorStop) + T10 회귀 확정(TRUST5, LSP 0).

---

## §E.2 Run-phase Evidence — Secondary Goal 마일스톤 (T6 + T7 + T4)

### 범위 (이 마일스톤에서 수행)

- **T6**: 패널 설정 컨텍스트에서 공용 `StoreEntryTable` 소비 — 행 선행 체크박스(`selection`) + `actions`→`Alias` 컬럼(`renderCellExtra` + 컨텍스트 컬럼 세트). 선택 집합을 `StoreSourceConfig.selected_keys`(additive)로 반영. 에이전트 상세는 미주입(불변). REQ-04/07/09.
- **T7**: 공용 리스트의 필터/정렬/표시숨김을 패널별 localStorage(`panel-settings.storeTable.<panelId>`)로 영속/복원. 빈 store graceful. REQ-08/AC-08/AC-10.
- **T4**: 데이터소스 Store/TSDB 토글. TSDB = "후속 SPEC 안내" placeholder(실 바인딩 없음, Store 설정 보존). ko/en i18n 대칭 추가. REQ-04/REQ-05/AC-05.

이번 마일스톤 밖(후속): T2/T3(리사이즈·비율 영속), T9(라이브 미리보기 draft/롤백·선택 상한 가드), T8(색상 프리셋).

### 파일 (files_created / files_modified)

**신규**
- `web/src/pages/dashboard/PanelSettingsDataSource.tsx` — 데이터소스 영역 컨테이너(Store/TSDB 토글 + 기존 `StoreSourceSection` 보존 + 공용 `StoreEntryTable` 선택 surface). `PanelStoreSelectTable` 내부 컴포넌트.
- `web/src/pages/dashboard/panels/charts/storeSelectedKeys.ts` — `selected_keys` 방어적 파싱/토글 순수 헬퍼.
- `web/src/pages/dashboard/panels/charts/panelStoreTablePrefs.ts` — 필터/정렬/표시숨김 패널별 localStorage 영속(Set↔배열 직렬화, 방어적 파싱).
- 테스트: `storeSelectedKeys.test.ts`(6), `panelStoreTablePrefs.test.ts`(7), `PanelSettingsDataSource.test.tsx`(10).

**확장**
- `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts` — `StoreSourceConfig.selected_keys?: string[]` additive 필드(미설정 ⇒ 기존 동작, 하위호환 파싱).
- `web/src/pages/agents/storeColumns.tsx` — `StoreColumnId` 유니온에 `'alias'` 추가(STORE_COLUMNS 레지스트리에는 미추가 → 에이전트 상세 무영향).
- `web/src/pages/agents/StoreEntryTable.tsx` — `entry.value` 미정의(메타데이터 파생 행) 방어 가드(`JSON.stringify(undefined) ?? ''`). 값이 항상 정의된 에이전트 상세 경로는 동작 불변.
- `web/src/pages/dashboard/PanelSettingsDialog.tsx` — 데이터소스 슬롯을 `StoreSourceSection` → `PanelSettingsDataSource` 로 교체, import 재배선(미사용 `StoreSourceSection` 제거).
- i18n `ko.json`/`en.json` — `agents.detail.store.{colAlias,selectRowAriaLabel,selectColumnAriaLabel}` + `dashboard.settings.{dataSourceStoreTab,dataSourceTsdbTab,dataSourceTsdbTitle,dataSourceTsdbBody,dataSourceStoreSelectTitle,dataSourceStoreSelectEmpty,dataSourceStoreSelectNoAgent}` (ko/en 대칭).

### tests_created + 상태

- `storeSelectedKeys.test.ts`(6) · `panelStoreTablePrefs.test.ts`(7) · `PanelSettingsDataSource.test.tsx`(10, T4 토글/placeholder·AC-05, AC-06/09 Alias, AC-07 체크박스→selected_keys, AC-10 빈 store, T7 정렬/표시숨김 영속·복원, 키 확장) — **전량 통과**.

### test_results (검증 증거)

- `cd web && npx tsc --noEmit` → **exit 0, 0 type errors**.
- `cd web && npx eslint .` → **44 problems (1 error, 43 warnings)** = baseline. 신규 발견 0.
- `cd web && npx vitest run` → **230 files / 2901 tests 전량 통과**(실패 0).
- 신규 코드 커버리지(`PanelSettingsDataSource.tsx` + `storeSelectedKeys.ts` + `panelStoreTablePrefs.ts`): **stmts 95.98% / branch 86.27% / funcs 94.44% / lines 95.98%** — 목표 ≥85% 충족.

### behavior_preserved (AC-16 회귀 0 증거)

- M1 특성화/셸 테스트(`AgentDetailPanel.storeCharacterization` 7, `StoreEntryTable` 6→7, `PanelSettingsDialog.shell` 3) 통과 유지.
- 기존 `StoreSourceSection.test.tsx`/`StoreTagMode.test.tsx`/`ChartPanelSections.test.tsx`/`PanelSettingsDialog.*.test.tsx` 통과 유지 — `StoreSourceSection` 은 무변경 + Store 모드 기본 렌더로 회귀 0.
- `selected_keys` 미설정 시 소비처(useStoreChartData 등) 경로 불변(additive) — 백엔드/ config JSON shape 무변경.

### implementation_divergence

- **StoreSourceSection 공존(비대체)**: 기존 `StoreSourceSection`(에이전트/시리즈/시간창 편집)을 제거하지 않고 Store 모드에서 그대로 렌더 + 공용 `StoreEntryTable` 선택 surface 를 추가(additive, 회귀 0). 결과적으로 Store 모드에 두 개의 키 테이블(기존 `chart-store-key-table` + 공용 표)이 공존한다 — 후속 마일스톤에서 통합 대상. 테스트는 `panel-store-select` 영역으로 스코프.
- **엔트리 데이터 소스**: 공용 표는 `useStoreKeysWithTags` 의 `keyObjects`(메타데이터: key/metric_type/tags)에서 파생. 라이브 값(value/namespace/updated) 컬럼은 노출하지 않아 "잘못된 빈 데이터"를 피한다. AC-06 은 동일 `STORE_COLUMNS` 정의·`storeEntrySort`·공용 컴포넌트 사용으로 충족(가시 컬럼은 key/metric/tags/alias 서브셋).
- **`renderCellExtra` = 오버라이드 우선**(M1 설계) 로 `alias` 컬럼 셀을 렌더. `'alias'` 는 `StoreColumnId` 에 additive 추가하되 `STORE_COLUMNS` 에는 미추가(에이전트 상세 무영향).
- **Alias 컬럼은 표시 전용(현재)**: 별칭 편집 배선(series[].alias 연동)은 이번 범위 밖 — 컬럼은 키를 기본 별칭으로 표시. 추가 편집은 후속.
- **Store/TSDB 토글은 로컬 UI 상태**(config 미기록) — TSDB 는 placeholder 전용이므로 config 를 건드리지 않아 Store 설정 보존(AC-05).
- **`StoreEntryTable` value 가드**: 메타데이터 파생 행(value 부재) 지원을 위한 방어 가드 추가(공용 컴포넌트 견고성 개선, 에이전트 상세 동작 불변).
