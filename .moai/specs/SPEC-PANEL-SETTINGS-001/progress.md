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

---

## §E.2 Run-phase Evidence — Tertiary Goal 마일스톤 (T2 + T3 + T9)

### 범위 (이 마일스톤에서 수행)

- **T2**: 3분할 셸 2경계 드래그 리사이즈 — 좌우(좌측 컬럼↔옵션, M1 스플리터 재사용) + 상하(미리보기↔데이터소스, 신규). 인접 영역 라이브 리사이즈, 최소 크기 클램프·비율 정규화(R4). REQ-02/AC-02.
- **T3**: 두 경계 비율을 패널별 localStorage 키 `panel-settings-ratio:<panelId>` 로 영속/복원. 손상/부재 → 기본 비율 폴백(throw 없음). REQ-03/AC-03.
- **T9**: draft/committed 분리(`useDraftPanelConfig`) + 디바운스 미리보기(`useDebouncedValue`, 실 store 데이터 패널 heatmap/line/gauge/modbus) + 선택 상한 가드(SELECTED_KEYS_LIMIT=48, 초과 안내 + 반영 억제). REQ-13/14, AC-13/14/15.

이번 마일스톤 밖(후속): T8(heatmap 색상 프리셋) + T10 회귀 확정.

### 파일 (files_created / files_modified)

**신규**
- `web/src/pages/dashboard/usePanelSettingsRatio.ts` — 2경계 비율 훅 + 순수 clamp/load/save(패널별 키, 방어적 파싱).
- `web/src/pages/dashboard/useDraftPanelConfig.ts` — draft/committed 분리 훅(patch/cancel/isDirty, panelId 전환 시에만 재초기화).
- `web/src/pages/dashboard/useDebouncedValue.ts` — 디바운스 훅(미리보기 재렌더/재조회 억제).
- 테스트: `usePanelSettingsRatio.test.ts`(8), `useDraftPanelConfig.test.tsx`(4), `useDebouncedValue.test.tsx`(2), `PanelSettingsDialog.resize.test.tsx`(6).

**확장**
- `web/src/pages/dashboard/PanelSettingsDialog.tsx` — 인라인 draft → `useDraftPanelConfig` 로 이관, `useDebouncedValue` 기반 `previewPanel`(디바운스)로 실데이터 미리보기 분리, 전역키 `panelSettings.leftWidth` → 패널별 `usePanelSettingsRatio`, 상하 경계 드래그 핸들러 추가, 셸에 previewRatio/스플리터 콜백 전달.
- `PanelSettingsShell`(동 파일) — 상하 스플리터(`panel-settings-preview-splitter`) + previewRatio flex-basis 좌측 컬럼 구조 추가(접힘/데이터소스 부재 시 기존 자연 배치 유지).
- `web/src/pages/dashboard/PanelSettingsDataSource.tsx` — 선택 상한 가드(초과 안내 배너 + 추가 억제) 배선.
- `web/src/pages/dashboard/panels/charts/storeSelectedKeys.ts` — `SELECTED_KEYS_LIMIT` + `isSelectionAtLimit` 추가(additive).
- i18n `ko.json`/`en.json` — `dashboard.settings.previewSplitterAria` + `dataSourceStoreSelectOverLimit`(ko/en 대칭).

### tests_created + 상태

- 훅 단위: `usePanelSettingsRatio`(8, clamp/영속/복원/panelId 재로드), `useDraftPanelConfig`(4, patch/cancel/isDirty/전환 재초기화), `useDebouncedValue`(2, 지연/중간값 스킵).
- 통합: `PanelSettingsDialog.resize.test.tsx`(6, 2스플리터 렌더·비율 복원·손상 폴백·드래그 영속·draft 누수 없음·적용 승격).
- 가드: `storeSelectedKeys.test.ts` +2(isSelectionAtLimit), `PanelSettingsDataSource.test.tsx` +2(상한 초과 안내·미만 정상). — **전량 통과**.

### test_results (검증 증거)

- `cd web && npx tsc --noEmit` → **exit 0, 0 type errors**.
- `cd web && npx eslint .` → **44 problems (1 error, 43 warnings)** = baseline. 신규 발견 0.
- `cd web && npx vitest run` → **234 files / 2924 tests 전량 통과**(실패 0).
- 신규 모듈 커버리지(usePanelSettingsRatio/useDraftPanelConfig/useDebouncedValue/storeSelectedKeys): **stmts 96.55% / branch 92.98% / funcs 100% / lines 96.55%** — 목표 ≥85% 충족.

### behavior_preserved (AC-16 회귀 0 증거)

- M1/M2 테스트(shell 3, storeCharacterization 7, StoreEntryTable 7, PanelSettingsDataSource 12, StoreSourceSection/StoreTagMode/ChartPanelSections) 통과 유지.
- 셸 리사이즈/접기/줌: 기존 좌우 스플리터·collapse·zoom 동작 보존(상하 스플리터는 additive). 접힘/비-차트 패널은 기존 자연 배치 유지.
- draft/committed: 편집 후 닫기(취소)가 committed 미변경(누수 없음) — 테스트로 검증. 백엔드/config JSON shape 무변경.

### implementation_divergence

- **draft/committed 는 M1부터 이미 존재** — T9 는 이를 `useDraftPanelConfig` 훅으로 형식화(행위 보존)하고 디바운스·선택 가드를 추가. 취소=닫기(onClose)로 draft 폐기(기존 동작) → AC-14 충족.
- **디바운스 범위 = 실 store 데이터 패널만**(heatmap/line/gauge/modbus). 악센트 미리보기(device/list 등)는 store 데이터 미사용이며 즉시 반영이 상호작용에 유리하여 디바운스 미적용(문서화된 결정).
- **좌우 경계 비율 = 옵션 컬럼 폭(optionsWidth)** — design 의 `leftColWidth` 는 좌측 컬럼 폭이나, M1 레이아웃이 옵션 폭(우측 고정)을 제어하므로 그 축을 영속. 전역키 `panelSettings.leftWidth` 는 패널별 `panel-settings-ratio:<panelId>` 로 대체(마이그레이션 아님 — 신규 키).
- **상한 가드 위치**: 선택 추가 시점(체크박스 onToggle)에서 억제 + 안내. selected_keys 는 M2 additive 필드이며 미리보기 소비는 후속(T10/consumer) — 가드는 선택 집합 크기 자체를 보호.
- **jsdom 드래그 한계**: `getBoundingClientRect`=0 이라 상하 드래그 기하는 통합 테스트에서 직접 검증 불가 → 비율 훅 단위 테스트 + 좌우 드래그(클램프 결과) + 스플리터 렌더로 커버.

---

## §E.2 Run-phase Evidence — Final Goal 마일스톤 (T8 + T10)

### 범위

- **T8**: heatmap 전용 gradient 색상 프리셋(Viridis/Turbo/Warm/Cool + 기존 기본 = 5종). 옵션 영역(heatmap 전용) 프리셋 선택 + 기존 커스텀 ColorStop 편집. 선택 시 draft `color_table` 반영(M3 draft/debounce 파이프라인 재사용). REQ-10/11, AC-11. heatmap 외 미노출(REQ-12/AC-12).
- **T10**: 회귀 확정 + AC-01~16 커버리지 매트릭스 + TRUST5/LSP 게이트 확인.

### 파일

**신규**: `web/src/pages/dashboard/panels/heatmap/heatmapColorPresets.ts`(프리셋 상수 + clonePresetStops), 테스트 `heatmapColorPresets.test.ts`(4) + `PanelSettingsDialog.colorPresets.test.tsx`(3).
**확장**: `PanelSettingsDialog.tsx`(HeatmapSettingsSection color_table 섹션에 프리셋 선택 UI 추가 + import), i18n `ko.json`/`en.json`(`dashboard.settings.heatmapColorPreset`, ko/en 대칭).

### test_results

- `npx tsc --noEmit` → **exit 0**.
- `npx eslint .` → **44 problems (1 error, 43 warnings)** = baseline, 신규 0.
- `npx vitest run` → **236 files / 2931 tests 전량 통과**(실패 0).
- 신규 코드 커버리지(`heatmapColorPresets.ts`): **stmts/branch/funcs/lines 100%**.
- 회귀 넷 명시 재실행(특성화 + 기존 chart/heatmap 편집 + heatmap 렌더): **11 files / 157 tests 통과**.

### AC-01 ~ AC-16 커버리지 매트릭스

| AC | 요구 | 구현 | 커버 테스트 | 상태 |
|----|------|------|-------------|------|
| AC-01 | 3분할 셸 구성 | `PanelSettingsShell`(preview/options/dataSource 3영역, 좌상단/좌하단/우측) | `PanelSettingsDialog.shell.test.tsx` | 충족 |
| AC-02 | 리사이즈 + 영속 | 좌우(M1 재사용)+상하(M3) 스플리터, 드래그→비율 갱신, 패널키 영속 | `PanelSettingsDialog.resize.test.tsx`(스플리터 렌더·좌우 드래그 영속) + `usePanelSettingsRatio.test.ts`(clamp/영속) | 충족(단, jsdom rect=0 로 상하 드래그 기하·라이브 인접 리사이즈는 직접 검증 불가 — 훅 단위+좌우 드래그로 대체) |
| AC-03 | 비율 복원 + 손상 폴백 | `usePanelSettingsRatio` load/restore, 손상→기본(throw 없음) | `resize.test.tsx`(복원 500px, 손상→360px no-throw) + `usePanelSettingsRatio.test.ts` | 충족 |
| AC-04 | Store 선택→config | 체크박스→`selected_keys`(additive) 반영 | `PanelSettingsDataSource.test.tsx` | 충족(config 반영). 미리보기의 selected_keys 소비는 후속 consumer 작업 — additive 미소비(문서화) |
| AC-05 | TSDB placeholder | Store/TSDB 토글, TSDB=안내 placeholder, config 불변 | `PanelSettingsDataSource.test.tsx`(placeholder + onConfigChange 미호출) | 충족 |
| AC-06 | 동일 컬럼·렌더 | 공용 `StoreEntryTable` + `STORE_COLUMNS`/`storeEntrySort` | `PanelSettingsDataSource.test.tsx` + `StoreEntryTable.test.tsx` | 충족(가시 컬럼은 key/metric/tags/alias 서브셋 — 동일 정의·비교자·공용 컴포넌트 사용, keyObjects 메타 소스, 문서화) |
| AC-07 | 행 선택 체크박스 | selection→`selected_keys` add/remove | `PanelSettingsDataSource.test.tsx` | 충족 |
| AC-08 | 필터/정렬/표시숨김 영속 | `panelStoreTablePrefs`(패널키) + 재진입 복원 | `PanelSettingsDataSource.test.tsx`(숨김/정렬 영속·복원) + `panelStoreTablePrefs.test.ts` | 충족 |
| AC-09 | actions→Alias | `renderCellExtra` alias 컬럼(패널 설정) / actions 유지(에이전트 상세) | `PanelSettingsDataSource.test.tsx`(Alias 표시·colActions 부재) + `AgentDetailPanel.storeCharacterization.test.tsx`(actions 유지) | 충족 |
| AC-10 | 빈 store graceful | no-agent/empty 안내(throw 없음) | `PanelSettingsDataSource.test.tsx` | 충족 |
| AC-11 | 프리셋 제공·적용 | `HEATMAP_COLOR_PRESETS`(5종) 선택 + 커스텀 편집 → draft color_table | `PanelSettingsDialog.colorPresets.test.tsx` + `heatmapColorPresets.test.ts` | 충족 |
| AC-12 | heatmap 외 미노출 | `HeatmapSettingsSection` heatmap 전용 렌더 | `colorPresets.test.tsx`(bar-chart 미노출) | 충족 |
| AC-13 | 즉시 갱신(실데이터·디바운스) | `useDebouncedValue`→`previewPanel`(heatmap/line/gauge/modbus 실 store 데이터) | `useDebouncedValue.test.tsx`(디바운스) + previewRenderPanel 배선(코드 검증) | 충족(디바운스 훅 단위 검증 + 배선 코드 검증. 렌더된 미리보기의 end-to-end 갱신 스냅샷은 미어서션 — 통합 수준) |
| AC-14 | 미확정·저장·롤백 | `useDraftPanelConfig` + 적용/닫기 | `resize.test.tsx`(편집→닫기 미커밋·편집→적용 승격) + `useDraftPanelConfig.test.tsx`(cancel/isDirty) | 충족 |
| AC-15 | 선택 상한 | `SELECTED_KEYS_LIMIT`(48) + 초과 안내·억제 | `PanelSettingsDataSource.test.tsx`(초과 안내·미호출) + `storeSelectedKeys.test.ts`(isSelectionAtLimit) | 충족 |
| AC-16 | 회귀 0 | 특성화 + 기존 chart/heatmap 편집 무변경 | 전체 스위트 2931/2931 + 회귀 넷 157/157 | 충족 |

**부분 커버(정직 고지)**: AC-02(상하 드래그 기하·라이브 인접 리사이즈는 jsdom 한계로 직접 미어서션), AC-04(미리보기의 selected_keys 소비 미구현 — config 반영만), AC-13(디바운스 훅+배선 검증, 렌더 미리보기 end-to-end 갱신 스냅샷 미어서션). 나머지 AC 는 명시 테스트로 완전 커버.

### TRUST5 / LSP 게이트

- **Tested**: 신규 코드 커버리지 heatmapColorPresets 100%(전 마일스톤 신규 모듈 ≥85%). 특성화 테스트 통과.
- **Readable/Unified**: 기존 파일 스타일 일치, eslint 신규 0.
- **Secured**: localStorage 파싱 방어(ratio/prefs/selected), TSDB placeholder 로 미검증 렌더 차단.
- **Trackable**: @spec SPEC-PANEL-SETTINGS-001 주석, REQ↔파일 추적.
- **LSP**: tsc `--noEmit` 0 error/type, eslint 신규 error 0(run 게이트 max_*=0 충족). 신규 npm 의존성 0. 백엔드/불투명 config JSON shape 무변경(color_table/selected_keys 는 additive).

### implementation_divergence (T8)

- **커스텀 ColorStop 편집은 기존 존재** — `HeatmapSettingsSection` 이 이미 color_table 편집기를 보유. T8 은 그 위에 named 프리셋 선택만 additive 추가.
- **REQ-12 는 구조적으로 충족** — 프리셋 UI 가 `HeatmapSettingsSection` 내부에 있고 이 섹션은 heatmap 에서만 렌더되므로 차트 5종에 자연히 미노출.
- **프리셋 명칭은 로케일 불변**(Viridis/Turbo/Warm/Cool/Default — 범용 colormap 고유명). 라벨("gradient 프리셋")만 i18n.
- **default 프리셋은 `DEFAULT_COLOR_TABLE` 재사용**(idw.ts, 중복 방지). 적용 시 `clonePresetStops` 로 복제하여 상수 변형 방지.

---

## SPEC 구현 완료 (SPEC implementation complete)

SPEC-PANEL-SETTINGS-001 의 전 작업(T1~T10) 4개 마일스톤 완료:
- **M1(Primary)**: T10 특성화 + T5 공용 `StoreEntryTable` 추출 + T1 3분할 셸.
- **M2(Secondary)**: T6 체크박스·Alias + `selected_keys` additive + T7 필터/정렬/영속 + T4 Store/TSDB 토글.
- **M3(Tertiary)**: T2/T3 2경계 리사이즈·비율 영속 + T9 draft/committed·디바운스·선택 상한.
- **M4(Final)**: T8 heatmap 색상 프리셋 + T10 회귀 확정.

최종 상태: tsc 0, eslint 신규 0(baseline 1 error + 43 warnings 유지), vitest 236 files/2931 tests 전량 통과, 신규 코드 커버리지 ≥85%(당 마일스톤 100%), 신규 npm 의존성 0, 백엔드/불투명 config JSON shape 무변경. REQ-01~14 구현 + AC-01~16 커버(부분 3건 정직 고지). 에이전트 상세·기존 차트/heatmap 설정 편집 회귀 0(AC-16).

---

## Post-verification fix — 데이터소스 토글 단일화 (dual-heading UX bug)

브라우저 검증에서 데이터소스 영역에 "데이터 소스" 섹션이 중복 노출(내 outer Store/TSDB tablist + `StoreSourceSection` 자체 채널/Store 토글 → "Store" 2회)되는 UX 버그 발견(M2 divergence #1 표면화). 사용자 승인 수정 = 단일 토글.

- **StoreSourceSection**(`ChartPanelSections.tsx`): 기존 [채널 | Store] 토글을 **[채널 | Store | TSDB]** 3옵션으로 확장(단일 "데이터 소스" 헤딩). 채널/Store 는 기존과 동일하게 `config.data_source` 로 영속(byte-identical). **TSDB 는 UI 전용 모드**(config 미기록)로 후속 SPEC 안내 placeholder 만 렌더 → 기존 Store 설정 보존(REQ-05/AC-05). `onModeChange` 콜백으로 모드의 단일 소스 오브 트루스를 상위에 보고.
- **PanelSettingsDataSource.tsx**: 신규 outer Store/TSDB tablist + placeholder + `mode` state **제거**. `StoreSourceSection` 을 `onModeChange` 와 함께 렌더하고, **Store 모드에서만** `PanelStoreSelectTable`(체크박스+Alias) 노출. 채널/TSDB 모드에서는 선택 테이블 미노출.
- **i18n**: `dashboard.chart.dataSourceTsdb`(토글 라벨) 추가(ko/en). 미사용 `dashboard.settings.dataSourceStoreTab`/`dataSourceTsdbTab` 은 참조 0 확인 후 제거(placeholder 텍스트 `dataSourceTsdbTitle`/`Body` 는 StoreSourceSection 이 재사용, 유지).
- **테스트**: `PanelSettingsDataSource.test.tsx` T4 describe 를 단일 토글로 재작성(중복 래퍼 부재 검증 + TSDB→placeholder+테이블 미노출+config 보존 + 채널 모드 테이블 미노출). `emptyPanel` 을 Store 모드(에이전트 미선택)로 조정.
- **검증**: tsc 0, eslint 44(1 error, 43 warnings = baseline, 신규 0), vitest **236 files / 2932 tests 전량 통과**. 채널/Store 바인딩 + StoreSourceSection/StoreTagMode/ChartPanelSections + 에이전트 상세 회귀 0(AC-16). 신규 npm 의존성 0, config JSON shape 무변경(TSDB 는 config 미기록).

**결과: 단일 "데이터 소스" 헤딩 + [채널 | Store | TSDB] 토글.** "Store" 중복 헤딩/토글 제거 완료.

---

## §E.2 Run-phase Evidence — M6 마일스톤 (데이터 소스 시리즈 선택 UI 개선, REQ-15~22)

### 범위 (이 마일스톤에서 수행)

M1~M5(3분할 셸·공용 Store 리스트·색상 프리셋·리사이즈/영속·라이브 미리보기) 위에 데이터 소스 시리즈 선택 UX 를 개선(SPEC v0.5.0, REQ-15~22).

- **REQ-15**: 통일된 표시 필터 — 같은 컬럼 다중값 OR, 다른 컬럼 AND. 태그는 태그 키(종류)별로 — 같은 키 다중값 OR, 다른 키 간 AND.
- **REQ-16/17**: 사용자 표기 "별칭"→"이름"(필드 `alias` 유지) + 컬럼 순서 key·이름·metric·tag.
- **REQ-18/20/21**: 시리즈별 세부 정보 = 선택된(체크된) 행의 인라인 펼침/접힘(이름 편집·색상·선스타일·(heatmap)좌표 2열). 별도 `SelectedSeriesList` 그룹 제거(단일 편집 지점 = 행 펼침). heatmap 센서 좌표 세부 정보 통합.
- **REQ-19(폐지/superseded)**: 펼침 안 키·종류·태그 설명 라인 제거(컬럼/alias 에코 중복) — 재번호 없음.
- **REQ-22**: 명시적 "동적 바인딩" 토글(표시 필터와 분리, 기존 tag 모드 default ON 하위호환).
- 필터 팝오버 잘림 수정(viewport-clamped fixed positioning) + 세부 정보 폰트 확대.

### 파일 (files_created / files_modified)

**신규(files_created)**
- `web/src/pages/dashboard/panels/charts/storeColumnValueFilter.ts` — 통일된 표시 필터 순수 매처(같은 컬럼 OR·컬럼 간 AND·태그 키별 OR/AND). design.md 파일 계획에 포함.
- `web/src/pages/dashboard/panels/charts/storeColumnValueFilter.test.ts` — 매처 단위 테스트.

**확장(files_modified)**
- `web/src/lib/i18n/{en,ko}.json` — "이름" 표기·세부 정보·동적 바인딩 토글 라벨(ko/en 대칭).
- `web/src/pages/agents/{StoreEntryTable,storeColumns}.tsx` — 컬럼 순서 재배치 + 표시 필터/인라인 세부 정보 배선.
- `web/src/pages/dashboard/{ChartPanelSections,PanelSettingsDataSource}.tsx` — 행 펼침 세부 정보(이름 편집·색상·선스타일·(heatmap)좌표) 통합, `SelectedSeriesList` 그룹 제거, 동적 바인딩 토글 분리, 팝오버 fixed positioning·폰트 확대.
- 테스트 확장: `PanelSettingsDataSource.test.tsx`, `PanelSettingsDialog.heatmap.test.tsx`, `StoreSourceSection.test.tsx`, `StoreTagMode.test.tsx`.

### test_results (검증 증거)

- `Test Files 236 passed (236) / Tests 2956 passed (2956)` — 전량 통과(실패 0).
- `tsc -b` → **exit 0** (0 type errors).
- `build` → **exit 0**.

### implementation_divergence (계획 대비 실제)

- 구현은 SPEC v0.5.0 요구그룹(REQ-15~22, REQ-19 폐지)을 추적. 비계획 스코프 0.
- 신규 `storeColumnValueFilter.ts` 는 design.md 파일 계획에 이미 포함된 순수 매처 — 신규 추상화 계층 아님.
- `tag_filters` 는 `Record<string,string>` 유지 — 다중값-per-키는 표시 필터 상태 관심사이며, 동적 바인딩 파생은 기존 best-effort last-value(SPEC v0.5.0 §HISTORY 명시). `PANEL_FILTER_COLUMNS` 는 그룹 필터 UI 용으로 여전히 tag 를 열거하고, 태그 키별 시맨틱은 매처 배선에 위치.

### sync 상태

- CHANGELOG.md M6 항목 추가, spec.md frontmatter status **draft → in-progress**(updated 2026-08-11), 본 §E.2 M6 증거 기록, 동기화 리포트 `.moai/reports/sync-report-panel-settings-m6-001.md` 작성 완료.
