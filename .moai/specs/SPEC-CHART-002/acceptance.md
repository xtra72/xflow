# SPEC-CHART-002 인수 조건

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)
형식: Given-When-Then + **기계 검증 가능한 단언**(vitest 파일·테스트명, 또는 grep/tsc 명령과 기대 출력)

## 공통 픽스처

`useStoreChartData` 는 테스트 주입 지점(`queryMatrixFn` / `resolveKeysFn` / `nowFn`)을 갖고 있으므로 네트워크 없이 매트릭스를 주입한다. 기존 `charts/__mocks__` 및 `StatPanelStoreSource.test.tsx` 의 모킹 패턴을 그대로 쓴다.

**F1 — 3시리즈 매트릭스** (`interval_ms` 균일, 5버킷)

| 시리즈 표시 이름 | b0 | b1 | b2 | b3 | b4 |
|------------------|----|----|----|----|----|
| `temp.room1` | 20 | 22 | 26 | 24 | 21 |
| `temp.room2` | 18 | `null` | 19 | 19 | 23 |
| `temp.room3` | `null` | `null` | `null` | `null` | `null` |

F1 기준 대표값 기대치:

| 시리즈 | max | avg | min | last | sum | count | delta |
|--------|-----|-----|-----|------|-----|-------|-------|
| `temp.room1` | 26 | 22.6 | 20 | 21 | 113 | 5 | 1 |
| `temp.room2` | 23 | 19.75 | 18 | 23 | 79 | 4 | 5 |
| `temp.room3` | `undefined` | `undefined` | `undefined` | `undefined` | `undefined` | `0` | `undefined` |

**F2 — 게이지 레거시 config 4종**: `{sourceType:'resource', resource:'cpu'}` · `{sourceType:'flow', flowId:'f1', dataField:'x'}` · `{sourceType:'chart-emitter', channelName:'ch1', displayField:'labels.temp'}` · `{sourceType:'store', storeAgentId:'a1', storeAgent:'store-a', storeKey:'k1', storeNamespace:'default'}`

---

## A. 두 축의 분리 (U1)


> **[v0.3.0 감사 주의 — 테스트 이름 드리프트]** 아래 AC 들이 지정한 vitest 테스트 **이름**은 실제 구현된 테스트 이름과 여러 건 어긋난다.
> 동작 커버리지는 모두 존재하지만 **이름으로 대조하면 오탐(미커버로 보임)이 난다.** 대표 사례:
> AC-19 는 `store + store_source 비활성(series 0) → legacy` 로 적혀 있으나 실제 테스트는 레거시 `dataSources` 유무까지 분기해
> `... + 레거시 있음 → legacy` / `... + 레거시 없음 → legacy` 2건으로 더 촘촘하다.
> AC-16 의 3건은 구현에서 2건으로 병합됐고, AC-21~AC-24 가 지정한 레거시 보존 동작은 `CH-01`~`CH-20` 특성화 테스트가 담당한다.
> 감사 시에는 테스트 **이름**이 아니라 각 테스트 파일 상단의 `@spec SPEC-CHART-002 AC-NN` 주석으로 대조할 것.


### AC-01 — `series_reduce` 는 `store_source` 밖의 패널 config 필드다

```gherkin
Given chartChannelTypes.ts 에 SeriesReduceFunc 타입이 정의되어 있을 때
When 타입 정의를 검사하면
Then series_reduce 는 ChartPanelConfigBase 의 옵셔널 필드이고 StoreSourceConfig 에는 존재하지 않는다
```

```bash
# ChartPanelConfigBase 에 존재
grep -n "series_reduce?: SeriesReduceFunc" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts   # 1행
# StoreSourceConfig 블록(interface StoreSourceConfig ~ 닫는 중괄호) 안에는 없음
awk '/^export interface StoreSourceConfig/,/^}/' web/src/pages/dashboard/panels/charts/chartChannelTypes.ts | grep -c "series_reduce"   # 0
# 7종 유니온이 정확히 정의됨
grep -n "export type SeriesReduceFunc" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts   # 1행
```

```bash
npx tsc --noEmit   # exit 0
```

### AC-02 — 기존 `AggFunc` 와 `aggregation` 이 그대로 남는다

```gherkin
Given SPEC-CHART-002 구현이 완료되었을 때
When 기존 집계 타입을 검사하면
Then AggFunc 유니온과 StoreSourceConfig.aggregation 유니온이 변경 없이 존재한다
```

```bash
grep -n "export type AggFunc = 'count' | 'sum' | 'avg';" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts   # 1행
grep -n "aggregation: 'min' | 'max' | 'average' | 'first' | 'last';" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts   # 1행
grep -n "agg_func?: AggFunc" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts   # 2행 (Bar, Pie)
```

---

## B. 구간 대표값 7종 (U2)

### AC-03 — 7종 대표값이 F1 기대치와 일치한다

```gherkin
Given F1 의 temp.room1 / temp.room2 타임라인이 주어질 때
When reduceSeries 를 7종 함수로 각각 호출하면
Then 공통 픽스처의 기대치 표와 정확히 일치한다
```

vitest: `src/pages/dashboard/panels/charts/seriesReduce.test.ts`
- `reduceSeries > max/avg/min/last/sum/count/delta 가 F1 기대치와 일치한다`
- `reduceSeries > null 버킷은 표본에서 제외된다 (temp.room2 avg = 19.75)`

### AC-04 — 표본 0개 윈도우: `count` 만 0, 나머지는 값 없음

```gherkin
Given F1 의 temp.room3 처럼 모든 버킷이 null 인 타임라인이 주어질 때
When 7종 대표값을 계산하면
Then count 는 0 이고 나머지 6종은 undefined 다
```

vitest: `seriesReduce.test.ts`
- `빈 윈도우 > count 는 0 을 반환한다`
- `빈 윈도우 > sum 은 0 이 아니라 undefined 를 반환한다`
- `빈 윈도우 > max/avg/min/last/delta 는 undefined 를 반환한다`
- `빈 배열 입력도 전(全) null 윈도우와 동일하게 취급한다`

### AC-05 — `delta` 는 `last − first` 이며 표본 1개면 값 없음

```gherkin
Given 표본이 1개뿐인 타임라인이 주어질 때
When delta 를 계산하면
Then undefined 를 반환한다
And 표본이 2개 이상이면 (마지막 표본 − 첫 표본) 을 반환한다
```

vitest: `seriesReduce.test.ts`
- `delta > 표본 2개 이상이면 last - first 를 반환한다`
- `delta > 표본 1개면 undefined 를 반환한다 (변화량 미정의)`
- `delta > 감소 구간에서 음수를 반환한다`

### AC-06 — 비수치 값은 표본에서 제외되고 `count` 는 수치 표본만 센다

```gherkin
Given 문자열과 숫자가 섞인 타임라인 [10, "abc", 20, null, 30] 이 주어질 때
When 대표값을 계산하면
Then count = 3, sum = 60, avg = 20 이고 avg === sum / count 항등이 성립한다
And 전부 문자열인 시리즈는 count = 0, 나머지 6종 = undefined 다
```

vitest: `seriesReduce.test.ts`
- `비수치 값 > 문자열 표본은 제외되고 count 는 수치 표본 수를 센다`
- `비수치 값 > avg === sum / count 항등이 성립한다`
- `비수치 값 > 전부 문자열이면 count 0 + 나머지 undefined`

### AC-07 — 불리언 시리즈는 1/0 으로 접힌다

```gherkin
Given data_type='boolean' 시리즈가 [1, 0, 1, 1] 로 도착할 때
When 대표값을 계산하면
Then max=1, min=0, avg=0.75, sum=3, count=4, last=1, delta=0 이다
```

vitest: `seriesReduce.test.ts`
- `불리언 시리즈 > avg 는 duty ratio 를 반환한다 (0.75)`
- `불리언 시리즈 > delta 는 -1 / 0 / 1 중 하나다`

---

## C. Store 데이터 소스 동등화 (U3)

### AC-08 — 게이지 설정에 공용 Store 데이터 소스 surface 가 렌더된다

```gherkin
Given 패널 타입이 gauge 인 패널 설정 화면을 열었을 때
When 설정 화면이 렌더되면
Then data-testid="panel-settings-data-source" 영역이 존재하고
  And 그 안에 데이터 소스 토글(chart-data-source-store)과 공용 시리즈 선택 테이블이 렌더된다
```

vitest: `src/pages/dashboard/PanelSettingsDialog.shell.test.tsx`
- `gauge 패널에 공용 데이터 소스 섹션이 렌더된다 (SPEC-CHART-002 U3)`

```bash
# gauge 는 CHART_PANEL_TYPES 에 추가되지 않는다 (UB1-9)
grep -n "const CHART_PANEL_TYPES" web/src/pages/dashboard/PanelSettingsDialog.tsx
awk "/const CHART_PANEL_TYPES/,/;/" web/src/pages/dashboard/PanelSettingsDialog.tsx | grep -c "'gauge'"   # 0
# 노출은 dataSourceBelowPreview 조건 확장으로만
grep -n "dataSourceBelowPreview" web/src/pages/dashboard/PanelSettingsDialog.tsx | head -1
awk "/const dataSourceBelowPreview/,/;/" web/src/pages/dashboard/PanelSettingsDialog.tsx | grep -c "gauge"   # 1
```

### AC-09 — 대표값 선택기는 4종 패널 × Store 모드에서만 노출된다

```gherkin
Given 데이터 소스가 Store 모드일 때
When 패널 타입별로 설정 화면을 렌더하면
Then stat / gauge / bar-chart / pie-chart 는 대표값 선택기를 노출하고
  And line-chart / table / heatmap 은 노출하지 않으며
  And 같은 4종 패널이라도 채널 모드에서는 노출하지 않는다
```

vitest: `src/pages/dashboard/ChartPanelSections.test.tsx`
- `대표값 선택기 > stat/gauge/bar-chart/pie-chart + Store 모드에서 노출된다`
- `대표값 선택기 > line-chart/table/heatmap 에서는 노출되지 않는다`
- `대표값 선택기 > 채널 모드에서는 노출되지 않는다`
- `대표값 선택기 > 선택지는 max/avg/min/last/sum/count/delta 7개이며 first 는 없다`

---

## D. 다중 시리즈 → 다중 출력 (U4)

### AC-10 — stat: 시리즈 3개 → 타일 3개

```gherkin
Given F1 매트릭스와 config { data_source:'store', series_reduce:'max' } 가 주어질 때
When StatPanel 이 렌더되면
Then 타일 3개가 시리즈 순서대로 렌더되고
  And temp.room1 타일은 26, temp.room2 타일은 23, temp.room3 타일은 — 를 표시한다
```

vitest: `src/pages/dashboard/panels/charts/StatPanel.multiOutput.test.tsx`
- `series_reduce 지정 시 시리즈당 타일 1개를 렌더한다`
- `대표값이 undefined 인 시리즈도 슬롯을 유지하고 — 를 표시한다`
- `타일 순서는 store_source.series 배열 순서를 따른다`

### AC-11 — gauge: 시리즈 3개 → 게이지 3개

```gherkin
Given F1 매트릭스와 config { data_source:'store', series_reduce:'last', gaugeType:'simple', min:0, max:100 } 가 주어질 때
When GaugePanel 이 렌더되면
Then 게이지 3개가 렌더되고 각각 패널 공통 min/max/unit/thresholds/gaugeType 을 공유하며
  And 각 게이지 캡션에 시리즈 표시 이름이 표시되고
  And 값이 없는 시리즈는 -- 로 표시된다
```

vitest: `src/pages/dashboard/panels/GaugePanel.storeSource.test.tsx`
- `store_source + series_reduce 지정 시 시리즈당 게이지 1개를 렌더한다`
- `각 게이지는 패널 공통 min/max/gaugeType 을 공유한다`
- `값 없는 시리즈 게이지는 -- 를 표시한다`

### AC-12 — bar: 시리즈당 막대 1개, 값 없는 시리즈는 막대 생략

```gherkin
Given F1 매트릭스와 config { data_source:'store', series_reduce:'avg' } 가 주어질 때
When BarChartPanel 이 렌더되면
Then 막대는 2개(temp.room1=22.6, temp.room2=19.75)이고 temp.room3 막대는 렌더되지 않으며
  And 카테고리 라벨은 시리즈 표시 이름이다
  And mode / label_field / agg_func 는 결과에 영향을 주지 않는다
```

vitest: `src/pages/dashboard/panels/charts/BarChartPanel.multiOutput.test.tsx`
- `series_reduce 지정 시 시리즈당 막대 1개를 렌더한다`
- `대표값이 undefined 인 시리즈는 막대를 생략한다`
- `series_reduce 지정 시 mode/label_field/agg_func 를 무시한다`

### AC-13 — pie: 시리즈당 조각 1개, 음수·값없음 조각 생략 + 사유 안내

```gherkin
Given series_reduce:'delta' 로 어떤 시리즈의 대표값이 음수일 때
When PieChartPanel 이 렌더되면
Then 음수 시리즈의 조각은 렌더되지 않고
  And 패널에 생략 사유 안내가 표시되며
  And 값이 undefined 인 시리즈 조각도 렌더되지 않는다
```

vitest: `src/pages/dashboard/panels/charts/PieChartPanel.multiOutput.test.tsx`
- `series_reduce 지정 시 시리즈당 조각 1개를 렌더한다`
- `음수 대표값 시리즈는 조각을 생략하고 사유를 안내한다`
- `undefined 대표값 시리즈는 조각을 생략한다`
- `series_reduce 지정 시 label_field/agg_func/max_points 를 무시한다`

### AC-14 — 다중 출력 상한과 잘림 표기

```gherkin
Given 시리즈 20개가 선택되고 multi_output_limit 기본값이 12 일 때
When StatPanel / GaugePanel 이 렌더되면
Then 타일·게이지는 12개만 렌더되고 하단에 +8 잘림 표기가 나타나며
  And 잘림 표기는 접근 가능한 텍스트로 개수를 전달한다
And 단일 시리즈(N=1)일 때 그리드 1칸은 단일 출력과 동일한 외형을 유지한다
```

vitest: `src/pages/dashboard/panels/charts/SeriesTileGrid.test.tsx`
- `상한 초과 시 앞에서부터 limit 개만 렌더하고 +K 표기를 노출한다`
- `multi_output_limit 로 상한을 재정의할 수 있다`
- `단일 항목이면 그리드가 1열 1행으로 렌더된다`

---

## E. 표시 이름과 색상 (U5, U6)

### AC-15 — 출력 라벨은 `storeSeriesLabel` 규칙을 그대로 따른다

```gherkin
Given alias 가 지정된 시리즈 A, series_name_format 만 지정된 시리즈 B, 둘 다 없는 시리즈 C 가 있을 때
When 4종 패널의 출력 라벨을 확인하면
Then A 는 alias, B 는 형식 해석 결과, C 는 내장 서술 표기(key · metric{k=v}) 로 표시되고
  And 같은 값이 라인 차트 범례 · 데이터 소스 목록의 이름 컬럼과 문자열이 일치한다
And 라벨을 변경해도 색상 배정과 출력 순서는 변하지 않는다
```

vitest: `StatPanel.multiOutput.test.tsx` / `BarChartPanel.multiOutput.test.tsx`
- `출력 라벨은 storeSeriesLabel 규칙(alias > series_name_format > 서술 표기)을 따른다`
- `alias 변경이 색상/순서에 영향을 주지 않는다 (동일성은 인덱스 기준)`

```bash
# 신규 명명 규칙을 만들지 않았음을 확인 — 4종 패널 어디에도 자체 라벨 조립이 없다
grep -rn "storeSeriesLabel\|seriesNames" web/src/pages/dashboard/panels/charts/StatPanel.tsx \
  web/src/pages/dashboard/panels/charts/BarChartPanel.tsx \
  web/src/pages/dashboard/panels/charts/PieChartPanel.tsx \
  web/src/pages/dashboard/panels/GaugePanel.tsx | wc -l   # >= 4
```

### AC-16 — bar/pie 는 시리즈 색상을 채움색으로 쓴다

```gherkin
Given 시리즈 A 에 color:'#ff0000' 이 지정되고 시리즈 B 에는 지정되지 않았을 때
When series_reduce 모드로 BarChartPanel / PieChartPanel 이 렌더되면
Then A 의 막대·조각은 #ff0000 이고 B 는 pickSeriesColor(1) 팔레트 색이다
And series_reduce 부재(레거시) 모드에서는 기존 #3b82f6 / PIE_COLORS 가 그대로 쓰인다
```

vitest: `BarChartPanel.multiOutput.test.tsx` / `PieChartPanel.multiOutput.test.tsx`
- `시리즈 color 가 막대/조각 채움색으로 적용된다`
- `color 미지정 시 pickSeriesColor 팔레트가 적용된다`
- `레거시 모드에서는 기존 하드코딩 색상이 유지된다`

### AC-17 — stat/gauge 는 값 색상 축을 침범하지 않는다

```gherkin
Given 시리즈 color 와 threshold_color_rules(stat) / thresholds(gauge) 가 동시에 지정되었을 때
When 패널이 렌더되면
Then stat 의 값 숫자 색은 threshold_color_rules 가 결정하고 시리즈 색은 타일 라벨에만 적용되며
  And gauge 의 호(arc) 색은 thresholds 가 결정하고 시리즈 색은 캡션 라벨에만 적용된다
```

vitest: `StatPanel.multiOutput.test.tsx` / `GaugePanel.storeSource.test.tsx`
- `stat: 값 색은 threshold 가, 라벨 색은 시리즈 color 가 결정한다`
- `gauge: 호 색은 thresholds 가, 캡션 색은 시리즈 color 가 결정한다`

---

## F. 재조회 없는 즉시 반영 (E1)

### AC-18 — 대표값 변경은 Store 재조회를 유발하지 않는다

```gherkin
Given series_reduce:'max' 로 1회 폴링이 완료된 상태에서
When series_reduce 를 'avg' 로 변경하면
Then 화면의 값이 즉시 avg 결과로 바뀌고
  And queryMatrixFn 추가 호출 횟수는 0 이다
And store_source.aggregation 을 변경하면 queryMatrixFn 이 다시 호출된다(대조군)
```

vitest: `StatPanel.multiOutput.test.tsx`
- `series_reduce 변경은 queryMatrixFn 을 다시 호출하지 않는다`
- `store_source.aggregation 변경은 queryMatrixFn 을 다시 호출한다`

```bash
# pollKey 계산식에 series_reduce 가 포함되지 않음
awk "/const pollKey = useMemo/,/}, \[enabled, config, resolvedAgentName\]\);/" \
  web/src/pages/dashboard/panels/charts/useStoreChartData.ts | grep -c "series_reduce"   # 0
```

---

## G. 게이지 레거시 바인딩 (E2, S1)

### AC-19 — 우선순위 판정 진리표 7행 전수

```gherkin
Given plan.md M5 의 판정 진리표 7행 각각의 config 조합이 주어질 때
When resolveGaugeValueSource 를 호출하면
Then 표에 기록된 'store-source' 또는 'legacy' 를 정확히 반환한다
```

vitest: `src/pages/dashboard/panels/charts/gaugeLegacyBinding.test.ts`
- `판정 진리표 > data_source 미지정 + 레거시 있음 → legacy`
- `판정 진리표 > data_source 미지정 + 레거시 없음 → legacy`
- `판정 진리표 > store + store_source 비활성(series 0) → legacy`
- `판정 진리표 > store + store_source 비활성(tag_filters 0) → legacy`
- `판정 진리표 > store + store_source 활성 + series_reduce 부재 → legacy`
- `판정 진리표 > store + store_source 활성 + series_reduce 있음 → store-source`
- `판정 진리표 > store + store_source 활성 + 레거시 없음 → store-source`

### AC-20 — 이관 액션은 비파괴적이며 되돌릴 수 있다

```gherkin
Given F2 의 store 레거시 바인딩을 가진 게이지 패널에서
When "Store 데이터 소스로 이전" 액션을 실행하면
Then config.data_source = 'store' 이고
  And config.store_source = { agent_id:'a1', agent_name:'store-a', namespace:'default', selection_mode:'keys', series:[{key:'k1'}], ... } 이며
  And config.series_reduce = 'last' 이고
  And config.dataSources 는 이관 전과 deep-equal 로 동일하다
And 이후 데이터 소스 토글을 'channel' 로 되돌리면 레거시 값이 다시 표시된다
And resource/flow 바인딩만 있는 config 에서는 액션이 비활성이고 안내 문구가 표시된다
```

vitest: `src/pages/dashboard/PanelSettingsDialog.gaugeMigration.test.tsx`
- `이관 액션이 store_source + series_reduce:'last' + data_source:'store' 를 기록한다`
- `이관 후에도 config.dataSources 가 보존된다(비파괴)`
- `채널 모드로 되돌리면 레거시 경로가 다시 유효해진다`
- `유효한 store 바인딩이 없으면 액션이 비활성이고 안내를 표시한다`

```bash
# 자동 재작성 경로가 없음 — 이관은 사용자 조작 핸들러에서만 일어난다
grep -rn "dataSources" web/src/pages/dashboard/panels/GaugePanel.tsx | grep -c "delete\|splice\|= \[\]"   # 0
```

---

## H. 하위 호환 — 레거시 렌더 경로 보존 (S1, 특성화)

### AC-21 — stat 레거시 경로 무변경

```gherkin
Given series_reduce 가 없는 기존 stat config 가 주어질 때
When 패널이 렌더되면
Then 평탄화 타임라인의 마지막 값과 직전 entry 대비 delta(화살표 포함)가 변경 전과 동일하게 표시된다
And store 다중 시리즈 상황에서도 타일 배열이 아니라 단일 값으로 표시된다
```

vitest (특성화 CH-01~CH-04): `charts/StatPanel.test.tsx`, `charts/StatPanelStoreSource.test.tsx` — 기존 테스트 전량 GREEN + 신규
- `series_reduce 부재 + store 다중 시리즈 → 단일 값 1개만 렌더한다(타일 배열 아님)`

### AC-22 — bar 레거시 경로 무변경

vitest (특성화 CH-05~CH-07): `charts/BarChartPanel.test.tsx` — 기존 테스트 전량 GREEN + 신규
- `series_reduce 부재 시 category 모드 labels.name 그룹 최신값을 유지한다`
- `series_reduce 부재 시 time_bin × agg_func 결과를 유지한다`
- `series_reduce 부재 시 막대 채움색 #3b82f6 을 유지한다`

### AC-23 — pie 레거시 경로 무변경

vitest (특성화 CH-08~CH-10): `charts/PieChartPanel.test.tsx` — 기존 테스트 전량 GREEN + 신규
- `series_reduce 부재 시 aggregateByLabel + agg_func 결과를 유지한다`
- `series_reduce 부재 시 PIE_COLORS 순환 배정을 유지한다`
- `series_reduce 부재 시 max_points 최근 트리밍을 유지한다`

### AC-24 — gauge 레거시 경로 무변경

```gherkin
Given F2 의 4종 레거시 바인딩 각각이 주어질 때
When GaugePanel 이 렌더되면
Then 값 해석과 우선순위(chart-emitter > store latest > static)가 변경 전과 동일하고
  And 값 없음/비수치는 -- 로 표시되며
  And store 폴링 실패 시 이전 값이 유지된다
```

vitest (특성화 CH-11~CH-18): `GaugePanel.test.tsx`(기존 전량 GREEN) + `GaugePanel.storeLegacy.test.tsx`(신규)
- `resource 바인딩 값 해석`
- `flow 바인딩 값 해석`
- `chart-emitter 바인딩 + displayField dot-path`
- `store 레거시 mode:'latest' 폴링 값`
- `우선순위 chart-emitter > store > static`
- `복수 dataSources 중 첫 유효 항목만 사용한다`
- `값 없음/비수치는 -- 로 표시한다`
- `폴링 실패 시 이전 값을 유지한다`

```bash
# 레거시 계산 코드가 삭제되지 않고 남아 있다
grep -c "pickChartEmitterSource\|pickStoreSource\|useStoreLatestValue" web/src/pages/dashboard/panels/GaugePanel.tsx   # >= 3
```

---

## I. 채널 모드 무시 (S2)

### AC-25 — 채널 모드에서 `series_reduce` 는 읽히지 않고 삭제되지도 않는다

```gherkin
Given config 에 series_reduce:'max' 가 있으나 data_source 가 'channel' 일 때
When stat / bar-chart / pie-chart 가 렌더되면
Then 기존 채널 렌더 경로 결과가 그대로 나오고
  And 설정 화면에 대표값 선택기가 노출되지 않으며
  And 저장된 series_reduce 값은 config 에 그대로 남는다
And 다시 Store 모드로 전환하면 series_reduce 가 즉시 유효해진다
```

vitest: `StatPanel.multiOutput.test.tsx`, `ChartPanelSections.test.tsx`
- `채널 모드에서는 series_reduce 가 있어도 레거시 채널 경로를 사용한다`
- `채널 모드 전환이 series_reduce 를 config 에서 제거하지 않는다`

---

## J. 금지 동작 (UB1, UB2)

### AC-26 — 빈 윈도우 `sum` 이 화면에 `0` 으로 표시되지 않는다

```gherkin
Given 전 버킷이 null 인 시리즈에 series_reduce:'sum' 이 지정될 때
When stat 타일이 렌더되면
Then 타일에 0 이 아니라 — 가 표시된다
```

vitest: `StatPanel.multiOutput.test.tsx`
- `빈 윈도우 sum 은 0 이 아니라 — 로 표시된다`

### AC-27 — 시리즈 순서가 값 크기로 재정렬되지 않는다

```gherkin
Given 시리즈 순서가 [A(값 10), B(값 90), C(값 50)] 일 때
When 다중 출력이 렌더되고 다음 폴링에서 값이 [A=90, B=10, C=50] 으로 바뀌면
Then 출력 순서는 두 번 모두 A, B, C 로 동일하다
```

vitest: `StatPanel.multiOutput.test.tsx`
- `폴링으로 값이 바뀌어도 출력 순서는 config 순서를 유지한다`

### AC-28 — line-chart / table / heatmap 은 `series_reduce` 를 읽지 않는다

```bash
grep -c "series_reduce" web/src/pages/dashboard/panels/charts/LineChartPanel.tsx   # 0
grep -c "series_reduce" web/src/pages/dashboard/panels/charts/TablePanel.tsx       # 0
grep -rc "series_reduce" web/src/pages/dashboard/panels/heatmap/ | grep -v ":0" | wc -l   # 0
```

### AC-29 — 백엔드 변경 0

```gherkin
Given SPEC-CHART-002 의 전체 변경이 완료되었을 때
When 변경 범위를 확인하면
Then Go 파일 변경이 하나도 없다
```

```bash
git diff --stat main...HEAD -- '*.go'          # 빈 출력
git diff --stat main...HEAD -- 'internal/'     # 빈 출력
git diff --stat main...HEAD -- 'cmd/'          # 빈 출력
```

### AC-30 — 신규 네트워크 경로 0

```bash
# 4종 패널 어디에도 신규 fetch/post 호출이 추가되지 않았다 (게이지 레거시 useStoreLatestValue 의 기존 1건은 제외)
grep -c "post<\|fetch(" web/src/pages/dashboard/panels/charts/StatPanel.tsx \
  web/src/pages/dashboard/panels/charts/BarChartPanel.tsx \
  web/src/pages/dashboard/panels/charts/PieChartPanel.tsx   # 각각 0
grep -c "post<" web/src/pages/dashboard/panels/GaugePanel.tsx   # 1 (기존 useStoreLatestValue)
```

### AC-31 — 시리즈 0개는 오류가 아니라 빈 상태다

```gherkin
Given tag 모드의 매칭 키가 0개일 때
When 4종 패널이 렌더되면
Then 크래시 없이 빈 상태가 표시되고 status 는 error 가 아니다
And gauge 는 예외적으로 레거시 dataSources 가 값을 낼 수 있으면 그 값을 표시한다(의도된 비대칭)
```

vitest: `StatPanel.multiOutput.test.tsx`, `GaugePanel.storeSource.test.tsx`
- `tag 매칭 0개는 빈 상태로 렌더되고 오류가 아니다`
- `gauge: store_source 비활성 시 레거시 값으로 폴백한다`

### AC-32 — "값 없음"과 "폴링 실패"가 화면에서 구분된다

```gherkin
Given 대표값이 undefined 인 상태와 폴링이 실패한 상태가 각각 주어질 때
When 패널이 렌더되면
Then 전자는 — (또는 --) 만 표시하고 오버레이가 없으며
  And 후자는 오류 오버레이를 표시하고 마지막 성공 값을 파괴하지 않는다
```

vitest: `StatPanel.multiOutput.test.tsx`
- `대표값 undefined 는 오류 오버레이를 띄우지 않는다`
- `폴링 실패는 오류 오버레이를 띄우고 직전 렌더를 보존한다`

---

## K. 품질 게이트 (TRUST 5 / hybrid)

| 항목 | 기준 | 검증 명령 |
|------|------|-----------|
| **Tested** | 신규 파일(`seriesReduce.ts` · `SeriesTileGrid.tsx` · `gaugeLegacyBinding.ts`) 커버리지 85% 이상. 특성화 CH-01~CH-20 전량 GREEN | `npx vitest run --coverage src/pages/dashboard/panels/` |
| **Readable** | 한국어 코드 주석(프로젝트 규약 `code_comments: ko`). 신규 분기는 `series_reduce !== undefined` 형태로 4개 패널 동일 | `grep -rho "series_reduce !== undefined" web/src/pages/dashboard/panels/charts/{Stat,BarChart,PieChart}Panel.tsx web/src/pages/dashboard/panels/GaugePanel.tsx | wc -l` → 4 |
| **Unified** | 신규 파일은 `panels/charts/` 하위 배치, 기존 포매터/린터 규칙 준수 | `npx eslint web/src --max-warnings=0` |
| **Secured** | 신규 백엔드/엔드포인트 0. 외부 입력(store 값)은 표본 정규화 단계에서 `Number.isFinite` 방어 | AC-29, AC-30 |
| **Trackable** | 커밋 메시지 한국어 Conventional Commits, `SPEC-CHART-002` 참조. M5 는 단독 커밋 | `git log --oneline --grep 'SPEC-CHART-002'` |
| **LSP** | 타입 오류 0, 린트 오류 0 | `npx tsc --noEmit && npx eslint web/src --max-warnings=0` |
| **Build** | 프로덕션 빌드 성공 | `npm run build` |

---

## L. 인수 조건과 열린 질문의 관계

아래 AC 는 spec.md §7 의 열린 질문 확정 결과에 따라 **기대값이 달라진다.** 구현 착수 전 확정이 필요하다.

| AC | 의존하는 열린 질문 | 확정 전 상태 |
|----|--------------------|--------------|
| AC-14 | OQ1(다중 출력 상한 기본값·초과 처리) | 12 / 잘라내기+`+K` 로 잠정 작성 |
| AC-13 | OQ6(파이 음수 처리) | 조각 생략+안내 로 잠정 작성 |
| AC-25 | OQ2(채널 모드 확장) | 확장 없음(무시) 으로 잠정 작성 |
| AC-10 | OQ5(stat 타일 보조 지표) | 보조 지표 없음 으로 잠정 작성 |
| AC-12 | OQ8(bar time_bin × 다중 출력 조합) | 조합 불가(`mode` 무시) 로 잠정 작성 |
