# SPEC-CHART-002 구현 계획

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

## 0. 전략 요약

프로젝트 개발 방식은 `hybrid`(`.moai/config/sections/quality.yaml`)다. 따라서 마일스톤마다 두 규율을 나눠 적용한다.

| 대상 | 방식 | 커버리지 목표 |
|------|------|---------------|
| 신규 파일(`seriesReduce.ts`, `SeriesTileGrid.tsx`, `gaugeLegacyBinding.ts`) | **TDD** (RED → GREEN → REFACTOR) | 85% |
| 기존 파일 수정(`StatPanel` · `BarChartPanel` · `PieChartPanel` · `GaugePanel` · `PanelSettingsDialog` · `ChartPanelSections`) | **DDD** (ANALYZE → PRESERVE → IMPROVE). 특성화 테스트를 **먼저** 작성해 현재 동작을 잠근 뒤 수정 | 85% |

핵심 위험은 하나다 — **게이지의 레거시 `config.dataSources[]` 경로**. 이 경로는 4종 소스(`resource` / `flow` / `chart-emitter` / `store`) × 우선순위 규칙 × "값 없음" 표기를 갖고 있고, 대응하는 특성화 테스트가 부분적으로만 존재한다. 따라서 **M5 를 독립 마일스톤으로 분리하고 단독 커밋한다.**

각 마일스톤은 독립적으로 `npm run build && npm test` 를 통과해야 한다.

---

## 1. 마일스톤 분해

### M1 — 타입 + 구간 대표값 순수 로직 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | `SeriesReduceFunc` 타입 + `ChartPanelConfigBase.series_reduce` 필드 추가. `store_source.aggregation` 과의 축 차이를 주석으로 명시 | `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts` |
| 1.2 | `reduceSeries(entries, fn): number \| undefined` 순수 함수 — 7종 전량 | `web/src/pages/dashboard/panels/charts/seriesReduce.ts`(신규) |
| 1.3 | `reduceAllSeries(seriesEntries, seriesNames, seriesStyles, fn)` — 시리즈 순서 보존 → `Array<{ name, value, color }>`. `color` 는 **설정된 색 그대로이거나 `undefined`** 이며 팔레트 폴백(`pickSeriesColor`)을 적용하지 않는다 — §2.6 이 폴백을 패널별로 다르게 정의하므로(bar/pie → 팔레트, stat/gauge → 기본 라벨색) 여기서 결합하면 stat/gauge 가 되돌려야 한다 | 동일 |
| 1.4 | `REDUCE_PANEL_TYPES` 집합 + 대표값 라벨 i18n 키 정의 | `chartChannelTypes.ts`, `web/src/lib/i18n/{ko,en}.json` |
| 1.5 | 단위 테스트 — 7종 × (정상 / 표본0 / 전null / 비수치혼합 / 단일표본 / 불리언 / 음수) | `charts/seriesReduce.test.ts`(신규) |

**설계 메모**: `reduceSeries` 는 `ChartEntry[]` 를 받고 내부에서 표본 정규화(null·비유한 제외)를 수행한다. 정규화를 호출자에게 맡기면 4개 패널에 같은 필터가 복제되고 규칙이 갈린다.

`delta` 는 `last − first` 이며 표본 1개면 `undefined` 다. `count` 만 표본 0개에서 `0` 을 반환한다(spec.md §2.2 / §4.3).

**검증**: `npx vitest run src/pages/dashboard/panels/charts/seriesReduce.test.ts` + `npx tsc --noEmit`.

**M1 을 먼저 두는 이유**: 이후 모든 마일스톤이 `series_reduce` 필드의 존재와 `reduceSeries` 의 계약을 전제한다. 순수 함수라 UI 없이 전수 테스트가 가능하므로, 정의의 애매함(빈 윈도우, 비수치, 불리언)을 여기서 전부 소진한다.

---

### M2 — 기존 4종 패널 특성화 테스트 (Priority High, DDD PRESERVE)

**패널 코드를 한 줄도 고치기 전에** 현재 동작을 테스트로 잠근다. 이 마일스톤의 산출물은 테스트뿐이며, 프로덕션 코드 변경은 없다.

| # | 대상 | 잠글 동작 | 파일 |
|---|------|-----------|------|
| 2.1 | `StatPanel` | 평탄화 마지막 값 + 직전 entry 대비 delta(↑/↓/→) + `threshold_color_rules` + `display_field` dot-path + 빈 상태 `—` + closed/error 오버레이 | `charts/StatPanel.test.tsx`(확장) |
| 2.2 | `StatPanel` (Store) | `data_source:'store'` + 다중 시리즈 + `series_reduce` **부재** → 여전히 평탄화 마지막 값 1개만 표시 | `charts/StatPanelStoreSource.test.tsx`(확장) |
| 2.3 | `BarChartPanel` | `category` 모드 `labels.name` 그룹 최신값 / `time_bin` × `agg_func`(sum·count·avg) / `max_points` 트리밍 / 막대 채움색 `#3b82f6` | `charts/BarChartPanel.test.tsx`(확장) |
| 2.4 | `PieChartPanel` | `aggregateByLabel` 그룹화 + `agg_func`(sum·count) + `max_points` 최근 트리밍 + `PIE_COLORS` 순환 + `show_legend`/`show_percentage` | `charts/PieChartPanel.test.tsx`(확장) |
| 2.5 | `GaugePanel` 레거시 4종 | `resource` / `flow` / `chart-emitter` / `store` 각 바인딩의 값 해석 | `GaugePanel.test.tsx`(확장) |
| 2.6 | `GaugePanel` 우선순위 | `chart-emitter > store(latest) > static config.value`. 복수 `dataSources` 에서 첫 유효 항목만 사용 | 동일 |
| 2.7 | `GaugePanel` 빈/비수치 | `entries` 0개 → `--`, 숫자 변환 불가 → `--`, 바인딩 없음 → `config.value` 그대로 | 동일 |
| 2.8 | `GaugePanel` store 폴링 | `useStoreLatestValue` 의 `mode:'latest'` 요청 형상 + 5초 인터벌 + 실패 시 이전 값 유지 | `GaugePanel.storeLegacy.test.tsx`(신규) |
| 2.9 | 설정 화면 | `gauge` 는 현재 `panel-settings-data-source` 를 렌더하지 **않는다**(변경 전 기준선). `line-chart`/`table`/`heatmap` 은 대표값 선택기를 갖지 않는다 | `PanelSettingsDialog.shell.test.tsx`(확장) |

2.5 ~ 2.8 이 이 SPEC 전체에서 가장 중요한 안전망이다. `GaugePanel.tsx` 는 776줄이고 값 해석 경로가 `pickChartEmitterSource` · `pickStoreSource` · `useStoreLatestValue` · `chartLiveValue` · `liveValue` · `hasBinding` · `parsedBase` 7개 지점에 분산되어 있다. M5 에서 이 흐름에 신규 분기를 끼워 넣으므로, 그 전에 현재 진리표가 테스트로 고정되어야 한다.

**검증**: 전체 프론트 테스트 통과 + 신규 특성화 테스트가 **수정 전 코드에서 전부 GREEN**(RED 가 하나라도 있으면 그것은 특성화가 아니라 요구사항이므로 별도 이슈로 분리).

---

### M3 — stat / bar / pie 다중 출력 (Priority High, DDD IMPROVE + TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | 공용 타일 그리드 컴포넌트 — 열 수 자동 결정, 최소 타일 폭, `multi_output_limit` 초과 시 `+K` 표기. **`ChartPanelConfigBase.multi_output_limit?: number` 필드와 `DEFAULT_MULTI_OUTPUT_LIMIT = 12` 상수를 이 작업이 신설한다**(M1 미포함 항목) | `charts/SeriesTileGrid.tsx`(신규, TDD), `chartChannelTypes.ts` |
| 3.2 | `StatPanel`: `series_reduce` 존재 + store 모드 → `seriesEntries` 소비 → 타일 배열. 부재 시 기존 경로 그대로 | `charts/StatPanel.tsx` |
| 3.3 | `BarChartPanel`: 시리즈당 막대 1개. 카테고리 = `seriesNames`, 채움색 = `seriesStyles.color ?? pickSeriesColor(i)`. `undefined` 시리즈는 막대 생략 | `charts/BarChartPanel.tsx` |
| 3.4 | `PieChartPanel`: 시리즈당 조각 1개. 음수/`undefined` 시리즈 조각 생략 + 사유 안내 | `charts/PieChartPanel.tsx` |
| 3.5 | 대표값 선택기 UI — `StoreSourceSection` 안, Store 모드 + `REDUCE_PANEL_TYPES` 일 때만 노출 | `ChartPanelSections.tsx` |
| 3.6 | `pie-chart` + 음수 가능 대표값(`delta`/`min`/`sum`) 조합 경고 | 동일 |
| 3.7 | 신규 동작 테스트 3종 | `StatPanel.multiOutput.test.tsx` · `BarChartPanel.multiOutput.test.tsx` · `PieChartPanel.multiOutput.test.tsx`(신규) |

**분기 위치 규약**: 각 패널의 `useMemo` 데이터 파생 블록 진입부에서 단 한 번 갈린다.

```
const isReduceMode = isStore && cfg.series_reduce !== undefined;
```

레거시 계산 코드는 **삭제하지 않고** `else` 가지에 그대로 둔다. M2 특성화 테스트가 그 가지를 계속 지킨다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/` 전량 + M2 특성화 테스트 전부 GREEN 유지.

---

### M4 — 게이지 Store 데이터 소스 동등화 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | `dataSourceBelowPreview` 조건에 `panel.type === 'gauge'` 추가. `CHART_PANEL_TYPES` 는 **불변** | `PanelSettingsDialog.tsx:445` |
| 4.2 | 게이지 설정에서 `PanelSettingsDataSource` 렌더 + 대표값 선택기 노출 확인 | 동일 / `ChartPanelSections.tsx` |
| 4.3 | `GaugePanel` 신규 store 경로 — `useStoreChartData(store_source)` + `reduceAllSeries` → 게이지 배열 | `GaugePanel.tsx` |
| 4.4 | 게이지 배열 렌더 — 각 게이지는 패널 공통 `min`/`max`/`unit`/`thresholds`/`gaugeType` 를 공유하고 캡션에 시리즈 이름 표시 | 동일 / `SeriesTileGrid.tsx` 재사용 |
| 4.5 | 시리즈 색상은 캡션 라벨에만 적용(호 색은 thresholds 소유) | 동일 |
| 4.6 | 게이지도 연결 상태 아이콘을 store 모드에서 노출(현재는 chart-emitter 일 때만) | 동일 |
| 4.7 | 신규 동작 테스트 | `GaugePanel.storeSource.test.tsx`(신규) |

**주의**: `GaugePanel` 은 `renderDashboardPanel.tsx:255` 에서 `onConfigChange`/`onTitleChange` 를 받는 유일한 차트 계열 패널이다. 신규 경로가 이 콜백 계약을 바꾸지 않아야 한다.

**M4 와 M5 를 나누는 이유**: M4 는 **추가**(신규 경로 신설)이고 M5 는 **간섭**(레거시 경로와의 우선순위 조정)이다. M4 단독 상태에서는 `data_source` 가 `store` 로 설정된 게이지가 없으므로(신규 필드) 레거시 동작이 전혀 건드려지지 않는다. 두 성격을 한 커밋에 섞으면 회귀 발생 시 원인을 가릴 수 없다.

**검증**: `npx vitest run src/pages/dashboard/panels/GaugePanel*.test.tsx src/pages/dashboard/PanelSettings*.test.tsx`.

---

### M5 — 게이지 레거시 바인딩 병행 지원 + 이관 (Priority High, 최고 회귀 위험 — 단독 커밋)

**이 마일스톤만 단독으로 커밋하고 검증한다. 앞뒤 마일스톤과 섞지 않는다.**

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | 값 소스 우선순위 판정을 순수 함수로 추출 — 입력(config 파생 플래그) → 출력(`'store-source' \| 'legacy'`) | `charts/gaugeLegacyBinding.ts`(신규, TDD) |
| 5.2 | `GaugePanel` 이 5.1 판정 결과로 두 경로 중 하나를 선택. 두 훅은 항상 호출하고 비활성 쪽은 idle 유지(React 훅 규칙) | `GaugePanel.tsx` |
| 5.3 | 레거시 → `store_source` 변환 순수 함수 — `GaugeDataSource(store)` → `StoreSourceConfig` + `series_reduce:'last'` | `gaugeLegacyBinding.ts` |
| 5.4 | 설정 화면 이관 액션 버튼 + 유효한 store 항목이 없을 때 비활성 + 안내 | `PanelSettingsDialog.tsx` `GaugeSection` |
| 5.5 | 이관 후 `config.dataSources` 보존 검증 + `channel` 로 되돌리면 레거시 재활성 검증 | 테스트 |
| 5.6 | 판정 진리표 전수 테스트 | `charts/gaugeLegacyBinding.test.ts`(신규) |
| 5.7 | 이관 액션 통합 테스트 | `PanelSettingsDialog.gaugeMigration.test.tsx`(신규) |

**판정 진리표**(5.1 의 테스트 대상 — 전수):

| `data_source` | `store_source` 활성 | `series_reduce` | 레거시 `dataSources` | 선택 경로 |
|---------------|---------------------|-----------------|----------------------|-----------|
| 미지정 / `'channel'` | — | — | 있음 | `legacy` |
| 미지정 / `'channel'` | — | — | 없음 | `legacy`(→ static `config.value`) |
| `'store'` | 비활성(시리즈 0 / tag 0) | 있음 | 있음 | `legacy` |
| `'store'` | 비활성 | 있음 | 없음 | `legacy`(→ static) |
| `'store'` | 활성 | 없음 | 있음 | `legacy` (spec.md §2.9) |
| `'store'` | 활성 | 있음 | 있음 | `store-source` |
| `'store'` | 활성 | 있음 | 없음 | `store-source` |

`store_source` 활성 판정은 기존 패널들과 동일한 규칙을 쓴다 — `keys` 모드는 `series.length > 0`, `tag` 모드는 `Object.keys(tag_filters).length > 0`.

**롤백 계획**: 본 마일스톤은 config 를 자동 재작성하지 않으므로 데이터 손실 지점이 없다. 문제 발생 시 커밋 되돌리기만으로 원복되며, 이관 액션으로 변경된 사용자 config 도 `dataSources` 가 보존되어 있어 데이터소스 토글을 `channel` 로 되돌리면 즉시 복구된다.

**검증 방법**:

| 항목 | 방법 |
|------|------|
| 레거시 무변경 | M2 의 2.5~2.8 특성화 테스트 전부 GREEN |
| 진리표 전수 | `gaugeLegacyBinding.test.ts` 7행 전부 |
| 이관 비파괴 | 이관 후 `config.dataSources` 가 이관 전과 deep-equal |
| 되돌리기 | 이관 후 `data_source:'channel'` 설정 → 레거시 값이 다시 표시됨 |
| 이관 불가 안내 | `resource`/`flow` 만 있는 config 에서 액션 비활성 |

---

### M6 — 설정 UI · 미리보기 · i18n 마무리 (Priority Medium)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | 대표값 선택기에 현재 조합 설명 한 줄 표시(`aggregation` × `series_reduce`) | `ChartPanelSections.tsx` |
| 6.2 | 불리언 시리즈 선택 시 대표값 의미 안내 | 동일 |
| 6.3 | `stat`/`gauge` 라이브 미리보기가 실제 패널을 draft config 로 렌더(라인 차트 `isStoreLinePreview` 선례) | `PanelSettingsDialog.tsx` |
| 6.4 | 신규 문구 ko/en 양쪽 추가 및 누락 키 검사 | `web/src/lib/i18n/{ko,en}.json` |
| 6.5 | 잘림 표기 `+K` 접근성 텍스트 | `SeriesTileGrid.tsx` |

**검증**: `npm run build`, `npx tsc --noEmit`, eslint 0, 전체 프론트 테스트 통과.

---

## 2. 마일스톤 순서 근거

```
M1 (순수 로직·타입)
  └→ M2 (특성화 = 안전망)          ← 프로덕션 코드 무변경
       ├→ M3 (stat/bar/pie 다중 출력)
       └→ M4 (게이지 신규 경로 추가)  ← 레거시 미간섭
            └→ M5 (게이지 레거시 간섭)  ← 단독 커밋
                 └→ M6 (마무리)
```

M2 는 M3 · M4 · M5 전부의 전제다. 특성화 없이 M3 을 시작하면 "바꾼 것이 의도한 것인지" 판정할 근거가 없다.

M3 과 M4 는 파일이 겹치지 않으므로(`charts/*Panel.tsx` vs `GaugePanel.tsx`) 순서를 바꾸거나 병렬로 진행해도 무방하다. 단 둘 다 `SeriesTileGrid.tsx` 를 쓰므로 M3.1 이 선행되어야 한다.

---

## 3. 필요한 특성화 테스트 목록 (DDD PRESERVE 산출물)

M2 에서 작성하며, 이후 모든 마일스톤에서 **GREEN 을 유지해야 한다**. 하나라도 RED 가 되면 그것은 하위 호환 위반이다(spec.md §2.9 / UB1-2).

| ID | 대상 | 잠그는 동작 | 근거 요구사항 |
|----|------|-------------|---------------|
| CH-01 | `StatPanel` | 평탄화 마지막 값 표시 | S1 |
| CH-02 | `StatPanel` | 직전 entry 대비 delta + 화살표 3종 | S1 |
| CH-03 | `StatPanel` | `threshold_color_rules` 값 색상 | U6, S1 |
| CH-04 | `StatPanel` | store 다중 시리즈 + reduce 부재 → 단일 값 | S1 |
| CH-05 | `BarChartPanel` | `category` 모드 `labels.name` 그룹 최신값 | S1 |
| CH-06 | `BarChartPanel` | `time_bin` × `agg_func` 3종 | S1, U1 |
| CH-07 | `BarChartPanel` | 막대 채움색 `#3b82f6` | U6 |
| CH-08 | `PieChartPanel` | `aggregateByLabel` + `agg_func` | S1, U1 |
| CH-09 | `PieChartPanel` | `PIE_COLORS` 순환 배정 | U6 |
| CH-10 | `PieChartPanel` | `max_points` 최근 트리밍 | S1 |
| CH-11 | `GaugePanel` | `resource` 바인딩 값 해석 | S1 |
| CH-12 | `GaugePanel` | `flow` 바인딩 값 해석 | S1 |
| CH-13 | `GaugePanel` | `chart-emitter` 바인딩 + `displayField` dot-path | S1 |
| CH-14 | `GaugePanel` | `store` 레거시 `mode:'latest'` 폴링 값 | S1 |
| CH-15 | `GaugePanel` | 우선순위 `chart-emitter > store > static` | S1 |
| CH-16 | `GaugePanel` | 복수 `dataSources` 중 첫 유효 항목만 사용 | S1 |
| CH-17 | `GaugePanel` | 값 없음/비수치 → `--` | S1, UB2-4 |
| CH-18 | `GaugePanel` | 폴링 실패 시 이전 값 유지 | UB2-3 |
| CH-19 | `PanelSettingsDialog` | 변경 전: `gauge` 는 데이터소스 섹션 미노출(기준선) | U3 |
| CH-20 | `PanelSettingsDialog` | `line-chart`/`table`/`heatmap` 에 대표값 선택기 미노출 | UB1-10 |

CH-19 는 M4 에서 **의도적으로 반전**되는 유일한 특성화다. 반전 시 테스트를 삭제하지 않고 기대값을 뒤집으며, 커밋 메시지에 반전 사유를 남긴다.

---

## 4. 위험과 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| 게이지 값 해석 경로 7지점 분산 | 신규 분기 삽입 시 조용한 회귀 | 판정을 순수 함수로 추출(M5.1) + 진리표 전수 테스트 |
| `series_reduce` 를 "기본값 있는 필드"로 오해한 구현 | 기존 패널 외형이 바뀜 | 유무 스위치 규약을 4개 패널 모두 같은 형태(`!== undefined`)로 작성. CH-01~CH-10 이 감지 |
| 다중 출력 시 시리즈 수 폭주(tag 모드) | 렌더 지연 · 판독 불가 | `multi_output_limit` 상한 + 기존 `STORE_SERIES_LIMIT` 이중 방어 |
| 파이 음수 조각 | recharts 렌더 이상 | 조각 생략 규칙 + 설정 경고(M3.4/3.6) |
| 표시 이름 중복 시리즈 | 출력이 예기치 않게 병합 | 기존 `useStoreChartData` 병합 규칙을 그대로 승계하고 테스트로 명시(신규 동작 아님) |
| `PanelSettingsDialog.tsx` 4968줄 | 편집 충돌 · 리뷰 난이도 | 본 SPEC 의 변경 지점을 3곳으로 한정(`dataSourceBelowPreview`, `GaugeSection` 이관 버튼, 미리보기 분기) |

---

## 5. 완료 정의 (Definition of Done)

- spec.md §2 의 U1~U6 · E1~E2 · S1~S2 · UB1~UB2 전량 구현
- acceptance.md 의 AC 전량 통과
- 특성화 테스트 CH-01 ~ CH-20 전부 GREEN (CH-19 는 반전된 기대값으로)
- 신규 코드 커버리지 85% 이상
- `npx tsc --noEmit` 0 에러, eslint 0 에러, `npm run build` 성공
- 프론트 전체 회귀 테스트 통과
- Go 코드 변경 0 (`git diff --stat -- '*.go'` 빈 출력)
- 신규 문구 ko/en 양쪽 존재
- §7 열린 질문(OQ1~OQ8) 중 구현에 영향을 준 항목의 확정 내용이 spec.md HISTORY 에 기록됨
