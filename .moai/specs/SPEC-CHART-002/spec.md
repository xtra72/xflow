---
id: SPEC-CHART-002
title: 통계/게이지/바/파이 패널의 Store 데이터소스 통합 + 구간 대표값 + 다중 시리즈 다중 출력
version: 0.3.0
status: draft
created: 2026-08-21
updated: 2026-08-21
author: xtra
priority: high
domain: dashboard
related_specs:
  - SPEC-CHART-001
  - SPEC-WEB-005
  - SPEC-WEB-006
  - SPEC-PANEL-SETTINGS-001
  - SPEC-DASHBOARD-001
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-21 | xtra | 최초 작성 — `stat` · `gauge` · `bar-chart` · `pie-chart` 4종 패널을 라인 차트와 동일한 Store 데이터 소스 선택 surface 로 통일하고, **윈도우 단위 구간 대표값(`series_reduce`)** 축을 신설하며, 시리즈 2개 이상 선택 시 **시리즈당 1개 출력**(타일/게이지/막대/조각 배열)으로 렌더한다. 게이지의 레거시 `config.dataSources[]` 바인딩은 파괴 없이 병행 지원하고 사용자 조작으로만 이관한다. |
| 0.2.0 | 2026-08-21 | xtra | 구현 착수 승인 — §7 열린 질문 OQ1~OQ8 을 **각 항목의 잠정 결정 그대로 확정**한다. 확정 내용: OQ1 상한 12개 + `+K` 잘림 표기 / OQ2 Store 전용(채널 모드 미제공) / OQ3 결합 출력 모드 미제공 / OQ4 게이지 레거시 `dataSources[]` 본 SPEC 에서 미폐기 / OQ5 stat 타일 보조 지표 미표시 / OQ6 pie 음수 대표값은 조각 생략 + 사유 안내 / OQ7 대표값은 패널 단위 단일 설정 / OQ8 `series_reduce` 존재 시 bar `mode` 무시(조합 불가). 이에 따라 acceptance.md §L 의 조건부 인수조건 (AC-10 · AC-12 · AC-13 · AC-14 · AC-25)은 잠정 결정 기준 기대값으로 확정된다. |
| 0.3.0 | 2026-08-21 | xtra | M2 특성화 결과 반영 — §6-5 의 "`resource` / `flow` 바인딩은 레거시 경로로 동작한다" 가정이 **사실이 아님**을 코드 확인으로 정정한다(`GaugePanel.tsx` 에 두 sourceType 의 리더가 존재하지 않음). §5 이관 결정의 근거도 이에 맞춰 대체하되 결정 자체(레거시 미폐기)는 유지한다. 결정에 영향 없음 — 정정된 것은 근거이며, 발견된 선행 결함 3건(resource/flow 미해석 · stat store 모드 delta 의 시리즈 교차 비교 · 게이지 store 폴링 실패와 비수치 성공의 비대칭)은 모두 본 SPEC 범위 밖으로 분리한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 세 문장이다.

```
통계/게이지/바/파이 차트
- Store 라인 차트와 동일하게 데이터 소스 선택
- 표시값은 구간 최고값/평균값/최저값/마지막 값 등에서 선택
- 둘 이상의 데이터 소스 선택시 다중 출력(파이 배열, 바 배열, 게이지 배열 등)
```

이를 세 축으로 분해한다.

1. **데이터 소스 선택 동등화** — `gauge` 가 공용 Store 소스 선택기(`PanelSettingsDataSource`)를 렌더하도록 하고, `stat` · `bar-chart` · `pie-chart` 는 이미 갖고 있는 동일 surface 를 유지한다. 4종 패널이 라인 차트와 같은 방식으로 Store 에이전트 · 시리즈 · 시간창을 고른다.
2. **구간 대표값(window reduce)** — 시간 윈도우 전체를 시리즈당 **숫자 1개**로 접는 신규 축 `series_reduce` 를 도입한다. 후보는 `max` · `avg` · `min` · `last` · `sum` · `count` · `delta` 7종이다.
3. **다중 시리즈 → 다중 출력** — 시리즈를 N개 고르면 출력도 N개다. `stat` 은 타일 배열, `gauge` 는 게이지 배열, `bar-chart` 는 시리즈당 막대 1개, `pie-chart` 는 시리즈당 조각 1개.

### 1.2 배경

#### 1.2.1 gauge 만 공용 데이터 소스 surface 에서 빠져 있다

[`web/src/pages/dashboard/PanelSettingsDialog.tsx:125`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx) 의 차트 패널 집합에 `gauge` 가 없다.

```ts
const CHART_PANEL_TYPES = new Set(['stat', 'line-chart', 'bar-chart', 'pie-chart', 'table']);
```

같은 파일 `:445` 가 데이터 소스 섹션의 노출을 결정한다.

```ts
const dataSourceBelowPreview = isChartPanel || panel.type === 'heatmap';
```

`gauge` 가 두 조건 어디에도 걸리지 않으므로 [`PanelSettingsDataSource`](../../../web/src/pages/dashboard/PanelSettingsDataSource.tsx) — 즉 [`StoreSourceSection`](../../../web/src/pages/dashboard/ChartPanelSections.tsx)(`:283`)과 공용 시리즈 선택 테이블 — 은 게이지에서 **한 번도 렌더되지 않는다.** 반면 `stat` · `bar-chart` · `pie-chart` 는 라인 차트와 완전히 같은 편집기를 이미 공유한다.

#### 1.2.2 gauge 는 별도의 레거시 바인딩을 쓴다

[`web/src/pages/dashboard/panels/GaugePanel.tsx:46`](../../../web/src/pages/dashboard/panels/GaugePanel.tsx) 이 게이지 전용 바인딩 형상을 정의한다.

```ts
interface GaugeDataSource {
  sourceType: 'resource' | 'flow' | 'chart-emitter' | 'store';
  resource?: string; flowId?: string; dataField?: string;
  channelName?: string; displayField?: string;
  storeAgentId?: string; storeAgent?: string; storeKey?: string; storeNamespace?: string;
}
```

이 배열은 `config.dataSources` 에 저장되고 [`GaugeSection`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx)(`:3318`)이 편집한다. store 분기는 `useStoreLatestValue`(`:77`)가 `POST /store/{agent}/query` 를 `mode:'latest'` 로 5초마다 1키만 폴링한다. **시간창 · 버킷 집계 · 다중 시리즈 · 태그 바인딩 개념이 전부 없다.** 즉 게이지의 store 지원은 라인 차트의 store 지원과 **다른 구현, 다른 config, 다른 표현력**이다.

#### 1.2.3 시리즈별 데이터는 이미 있으나 4종 중 아무도 쓰지 않는다

[`useStoreChartData`](../../../web/src/pages/dashboard/panels/charts/useStoreChartData.ts) 는 이미 시리즈별 형상을 반환한다.

```ts
export interface UseStoreChartDataResult {
  entries: ChartEntry[];                          // 평탄화된 단일 타임라인
  seriesEntries: Map<string, ChartEntry[]>;       // 시리즈 표시 이름 → 타임라인
  seriesStyles: Map<string, StoreSeriesStyle>;    // 시리즈 표시 이름 → 색/두께/스타일
  seriesNames: string[];                          // 컬럼(시리즈) 순서
  booleanSeries: Set<string>;
  status: ChartConnectionStatus; /* ... */
}
```

`seriesEntries` / `seriesNames` / `seriesStyles` 를 소비하는 것은 **`LineChartPanel` 뿐이다**(`:374`, `:840`). `StatPanel` · `BarChartPanel` · `PieChartPanel` 은 평탄화된 `entries` 만 읽는다. 따라서 필요한 데이터는 이미 전달되고 있고, 소비 규칙만 없다.

#### 1.2.4 현재 4종 패널의 표시값 규칙

| 패널 | 현재 규칙 | 위치 |
|------|-----------|------|
| `stat` | 평탄화 타임라인의 **마지막 entry** 값. 부가로 **직전 entry 대비 delta**(화살표 + 부호) | [`StatPanel.tsx:62`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx) |
| `bar-chart` | `category` 모드: `label_field`(기본 `labels.name`)별 **최신 값**. `time_bin` 모드: `bin_sec` 버킷 × `agg_func`(count/sum/avg) | [`BarChartPanel.tsx:53`](../../../web/src/pages/dashboard/panels/charts/BarChartPanel.tsx) |
| `pie-chart` | 최근 `max_points` 항목을 `label_field` 로 그룹화 후 `agg_func`(기본 `sum`) | [`PieChartPanel.tsx:76`](../../../web/src/pages/dashboard/panels/charts/PieChartPanel.tsx) |
| `gauge` | 우선순위 `chart-emitter > store(latest 1키) > static config.value` | [`GaugePanel.tsx:719`](../../../web/src/pages/dashboard/panels/GaugePanel.tsx) |

Store 소스에서 오는 모든 entry 는 `labels.name = 시리즈 표시 이름` 을 갖는다([`useStoreChartData.ts` `matrixToEntries`](../../../web/src/pages/dashboard/panels/charts/useStoreChartData.ts)). 그래서 bar/pie 는 **이미 시리즈별로 갈라져 보이지만**, 그 의미는 "시리즈별 원시 표본을 재집계"이지 "시리즈당 대표값 1개"가 아니다. 예를 들어 `pie` 의 기본 `agg_func:'sum'` 은 한 시리즈의 윈도우 내 모든 버킷 값을 **합산**한다 — 온도 센서라면 무의미한 값이다.

### 1.3 비범위 (Out of Scope)

- 백엔드 · 엔드포인트 · 전송 계층 변경 — 패널 config 는 기존과 동일한 불투명 JSON(`json.RawMessage`)이며 Store 조회는 기존 `POST /api/v1/store/{agent}/query` 경로만 쓴다
- `line-chart` · `table` · `heatmap` 의 렌더 규칙 변경 — `series_reduce` 는 이 3종에서 **읽지 않는다**
- 채널(`chart-emitter`) 모드에서의 구간 대표값 — §2.10 [S2] 참조. 채널 모드는 시리즈별 타임라인 자료구조를 stat/bar/pie 경로에 제공하지 않는다
- 게이지 레거시 `config.dataSources[]` 의 **제거** — 본 SPEC 은 병행 지원까지만 하고 제거는 후속 SPEC 으로 미룬다(§7 OQ4)
- 시리즈별로 서로 다른 대표값 지정(시리즈 A 는 최대, 시리즈 B 는 평균) — 대표값은 패널 단위 단일 설정이다(§7 OQ7)
- 다중 시리즈를 **하나로 합치는** 집계(예: 전 시리즈 합계를 stat 1개로) — 다중 선택 = 다중 출력이 본 SPEC 의 규칙이다(§7 OQ3)
- 대표값의 시계열 표시(스파크라인) — 타일은 숫자만 표시한다
- 원격 노드(remote) 패널의 동작 변경 — 원격 경로는 읽기 전용 프록시로 무변경

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 두 축의 분리 — 버킷 집계와 윈도우 대표값

시스템은 Store 소스 데이터를 접는 **서로 다른 두 축**을 유지하며, 어느 한 축이 다른 축을 대체하거나 겸용하지 않는다.

| 축 | 필드 | 위치 | 적용 시점 | 결과 형상 |
|----|------|------|-----------|-----------|
| **버킷 집계** | `store_source.aggregation` | [`chartChannelTypes.ts:161`](../../../web/src/pages/dashboard/panels/charts/chartChannelTypes.ts) (기존) | 조회 시점. `SeriesMatrixQuery.aggregation` 으로 서버에 전달 | 시리즈당 `interval_ms` 버킷마다 값 1개 → **타임라인** |
| **윈도우 대표값** | `series_reduce` | `ChartPanelConfigBase` (신규) | 렌더 시점. 클라이언트 순수 계산 | 시리즈당 **숫자 1개** |

두 축은 순차 합성된다: `원시 표본 → (aggregation) → 버킷 타임라인 → (series_reduce) → 시리즈 대표값 1개`.

예: `aggregation:'average'` + `series_reduce:'max'` 는 "1분 평균들의 구간 최댓값"이다. `aggregation:'max'` + `series_reduce:'max'` 는 "구간 최댓값"이며 두 조합의 결과는 다르다.

신규 필드는 **`store_source` 블록 밖**, 즉 `ChartPanelConfigBase` 에 둔다.

```ts
export type SeriesReduceFunc = 'max' | 'avg' | 'min' | 'last' | 'sum' | 'count' | 'delta';

export interface ChartPanelConfigBase {
  /* ...기존 필드... */
  /**
   * 윈도우 단위 구간 대표값. store_source.aggregation(버킷 집계)과 다른 축이다.
   * 미지정이면 패널은 레거시 렌더 경로를 유지한다(§2.9 [S1]).
   * data_source !== 'store' 인 경우 무시된다(§2.10 [S2]).
   */
  series_reduce?: SeriesReduceFunc;
}
```

**`store_source` 안에 두지 않는 이유**: `store_source` 의 필드는 `useStoreChartData` 의 `pollKey` 재구독 판정 대상이다. 대표값은 조회 파라미터가 아니라 표현 파라미터이므로, 같은 블록에 두면 대표값 변경이 불필요한 재조회를 유발하거나(포함 시) 두 축의 취급이 필드별로 갈린다(제외 시). 블록을 나누면 "조회 축은 `store_source`, 표현 축은 패널 config" 라는 구분이 타입 수준에서 드러난다.

기존 `AggFunc`(`'count' | 'sum' | 'avg'`, [`chartChannelTypes.ts:479`](../../../web/src/pages/dashboard/panels/charts/chartChannelTypes.ts))는 **제거하지도 이관하지도 않는다.** `BarChartPanelConfig.agg_func`(time_bin 모드)와 `PiePanelConfig.agg_func`(라벨 그룹화)는 그대로 남아 레거시 경로에서 계속 쓰인다. `series_reduce` 가 지정된 패널에서는 `agg_func` 와 `mode` 와 `label_field` 가 **무시된다**(§2.9). 두 필드가 동시에 저장되어 있어도 충돌이 아니며, 어느 쪽이 유효한지는 `series_reduce` 의 유무 하나로 결정된다.

### 2.2 [U2] (Ubiquitous) 구간 대표값 7종의 정의

시스템은 한 시리즈의 윈도우 타임라인을 다음 규칙으로 숫자 1개 또는 "값 없음"(`undefined`)으로 접는다.

**입력 정규화**: 시리즈 타임라인(`seriesEntries.get(name)`)에서 `value` 가 `null` 이 아니고 `Number.isFinite` 를 만족하는 항목만 **표본(sample)** 으로 취한다. 표본 순서는 **가정하지 않는다** — `first` / `last` 는 배열 위치가 아니라 `timestamp` 로 판정한다. §6-6 의 동일 표시 이름 시리즈 병합(`useStoreChartData.matrixToEntries` 의 concat)은 timestamp 오름차순을 깨뜨리므로, 배열 위치 기반 구현은 병합된 시리즈에서 `last` / `delta` 를 조용히 틀리게 만든다.

| 대표값 | 정의 | 표본 0개일 때 | 비고 |
|--------|------|---------------|------|
| `max` | 표본 값의 최댓값 | `undefined` | |
| `avg` | 표본 값의 산술 평균 | `undefined` | 가중치 없음. 버킷 간격이 균일하다는 전제(§6-3) |
| `min` | 표본 값의 최솟값 | `undefined` | |
| `last` | `timestamp` 가 가장 큰 표본의 값 | `undefined` | 동률이면 배열상 뒤 항목 |
| (내부) `first` | `timestamp` 가 가장 작은 표본의 값. 사용자 선택지가 아니며 `delta` 의 입력으로만 쓰인다 | `undefined` | 동률이면 배열상 **앞** 항목(`last` 규칙과 대칭) |
| `sum` | 표본 값의 합 | `undefined` | **0 이 아니다**(§4.3) |
| `count` | 표본의 **개수** | `0` | 7종 중 유일하게 빈 윈도우에서 값을 갖는다 |
| `delta` | `last − first`. `first` 는 `timestamp` 가 가장 작은 표본의 값 | `undefined` | 표본이 **1개뿐이면 `undefined`**(변화량 미정의) |

`first` 자체는 사용자가 고를 수 있는 선택지로 **노출하지 않는다.** `delta` 의 내부 입력으로만 쓴다. (`store_source.aggregation` 에는 별개로 `'first'` 가 존재하지만 이는 버킷 집계 축이며 §2.1 에 따라 다른 축이다.)

**전 표본이 null 인 윈도우**는 표본 0개와 동일하게 취급한다(`count` → `0`, 나머지 → `undefined`).

**비수치 값**(문자열 등)은 표본에서 제외된다. 따라서 문자열 시리즈는 `count` 를 제외한 모든 대표값이 `undefined` 이며, `count` 는 `0` 이다. `count` 는 "수신한 표본 수"가 아니라 "**수치 표본 수**"로 정의한다 — 그래야 `avg = sum / count` 항등이 성립한다.

**불리언 시리즈**(`StoreSeriesRef.data_type === 'boolean'`)는 Store 변환 시점에 이미 `1`/`0` 으로 정규화되어 있다([`useStoreChartData.ts` `matrixToEntries`](../../../web/src/pages/dashboard/panels/charts/useStoreChartData.ts) 주석). 따라서 별도 분기 없이 수치로 접는다. 의미는 다음과 같으며 UI 는 이를 안내한다.

| 대표값 | 불리언 시리즈에서의 의미 |
|--------|--------------------------|
| `max` | 윈도우 내 한 번이라도 참이면 `1` |
| `min` | 윈도우 내 항상 참이면 `1` |
| `avg` | 참이었던 시간 비율(duty ratio, 0..1) |
| `sum` | 참 표본 개수 |
| `last` | 최신 상태 |
| `delta` | `-1` (참→거짓) / `0` (변화 없음) / `1` (거짓→참) |
| `count` | 표본 수 |

### 2.3 [U3] (Ubiquitous) Store 데이터 소스 선택 동등화

시스템은 `stat` · `gauge` · `bar-chart` · `pie-chart` 4종에 대해 라인 차트와 **동일한** Store 데이터 소스 선택 surface 를 제공한다. 구체적으로 [`PanelSettingsDataSource`](../../../web/src/pages/dashboard/PanelSettingsDataSource.tsx) — 즉 [`StoreSourceSection`](../../../web/src/pages/dashboard/ChartPanelSections.tsx)(데이터소스 토글 · 에이전트 선택 · 시리즈 이름 형식) + 공용 시리즈 선택 테이블(`PanelStoreSelectTable`) — 을 렌더한다.

`stat` · `bar-chart` · `pie-chart` 는 이미 이 surface 를 갖고 있으므로 **변경 없이 유지**한다. `gauge` 만 신규 노출한다.

노출 방식은 heatmap 선례를 따른다.

```ts
// PanelSettingsDialog.tsx:445 (변경)
const dataSourceBelowPreview =
  isChartPanel || panel.type === 'heatmap' || panel.type === 'gauge';
```

시스템은 `gauge` 를 `CHART_PANEL_TYPES`(`:125`)에 **추가하지 않아야 한다.** 이 집합은 데이터소스 노출 외에도 차트 전용 채널/타입 분기를 구동하며, 게이지는 그 분기의 대상이 아니다. heatmap 이 같은 이유로 집합 밖에 남아 있다(`:444` 주석).

구간 대표값 선택기는 `StoreSourceSection` 안에서 **Store 모드일 때만** 렌더되며, 대상 패널 타입은 다음 집합으로 한정한다.

```ts
const REDUCE_PANEL_TYPES = new Set(['stat', 'gauge', 'bar-chart', 'pie-chart']);
```

`line-chart` · `table` · `heatmap` 은 같은 `StoreSourceSection` 을 쓰지만 선택기가 노출되지 않는다.

### 2.4 [U4] (Ubiquitous) 다중 시리즈 → 다중 출력 렌더 규칙

**While** `data_source === 'store'` 이고 `series_reduce` 가 지정되어 있는 동안, 시스템은 선택된 시리즈 **각각**에 대해 §2.2 의 대표값을 계산하고 패널 타입별로 다음과 같이 **시리즈당 1개**의 출력을 렌더한다.

| 패널 | 출력 단위 | 배치 | 시리즈 0개 | 대표값 `undefined` 인 시리즈 |
|------|-----------|------|------------|------------------------------|
| `stat` | 값 타일 1개(라벨 + 숫자 + 단위) | 반응형 그리드 | 기존 빈 상태(`—`) | 슬롯 유지, 값 자리에 `—` |
| `gauge` | 게이지 1개 | 반응형 그리드 | 기존 빈 상태(`--`) | 슬롯 유지, 값 자리에 `--` |
| `bar-chart` | 막대 1개(카테고리 = 시리즈 표시 이름) | 기존 `BarChart` 축 | 빈 차트 | **막대 생략** |
| `pie-chart` | 조각 1개(이름 = 시리즈 표시 이름) | 기존 `PieChart` | 빈 차트 | **조각 생략** |

**시리즈 순서**는 `keys` 모드에서 `store_source.series[]` 배열 순서, `tag` 모드에서 태그 해석 키 순서(= `seriesNames` 순서)를 따른다. 정렬을 재배치하지 않는다.

**슬롯 유지 vs 생략의 근거**: `stat`/`gauge` 는 "이 센서가 지금 값이 없다"가 사용자에게 의미 있는 정보이므로 자리를 남긴다. `bar`/`pie` 에서 값 없는 막대·조각은 0 과 구분되지 않아 오독을 만들므로 생략한다.

**그리드 배치**(`stat` · `gauge`): 열 수는 `min(ceil(sqrt(N)), 패널 폭이 허용하는 최대 열)` 로 결정하고 CSS 그리드로 배치한다. 타일 최소 폭이 확보되지 않으면 열 수를 줄인다.

**출력 개수 상한**: 표시 타일/막대/조각 수의 기본 상한은 **12**이며 `multi_output_limit?: number` 로 재정의할 수 있다. 상한을 넘는 시리즈는 순서상 뒤에서부터 잘리고, 패널 하단에 `+K` 표기로 잘린 개수를 알린다. 시리즈 선택 자체의 상한은 기존 `STORE_SERIES_LIMIT = 48`([`chartChannelTypes.ts:342`](../../../web/src/pages/dashboard/panels/charts/chartChannelTypes.ts))이 계속 강제한다. 상한값 12 는 §7 OQ1 에서 확정되었다. `multi_output_limit` 필드와 `DEFAULT_MULTI_OUTPUT_LIMIT = 12` 상수는 **M3.1(`SeriesTileGrid.tsx`)이 소유**한다 — M1 은 이 필드를 추가하지 않는다.

**단일 시리즈**(N=1)일 때도 같은 경로를 쓴다. 그리드 1칸은 기존 단일 출력과 시각적으로 동일해야 한다 — 다중 출력 도입이 단일 시리즈 패널의 외형을 바꾸면 안 된다.

### 2.5 [U5] (Ubiquitous) 시리즈 표시 이름은 기존 규칙을 재사용한다

시스템은 각 출력의 라벨로 [`storeSeriesLabel`](../../../web/src/pages/dashboard/panels/charts/chartChannelTypes.ts)(`:297`)이 결정한 표시 이름을 사용한다. 신규 명명 규칙을 만들지 **않는다.**

결정 순서(기존 그대로):

1. 시리즈에 직접 붙인 이름(`StoreSeriesRef.alias`) — [`resolveSeriesAlias`](../../../web/src/pages/dashboard/panels/charts/aliasTemplate.ts) 토큰 해석 포함
2. 패널의 시리즈 이름 형식(`store_source.series_name_format`) — 동일 토큰 문법
3. 내장 서술 표기(`seriesRefDisplayName(key, field, tags)`) — `key · metric{k=v}`

실무상 이 값은 `useStoreChartData` 가 반환하는 `seriesNames[j]` 및 각 entry 의 `labels.name` 과 **같은 문자열**이다. 따라서 라인 차트 범례 · 데이터 소스 목록의 이름 컬럼 · 히트맵 마커 · 본 SPEC 의 타일/막대/조각 라벨이 모두 같은 글자로 표시된다.

시스템은 표시 이름을 **동일성 판정에 쓰지 않아야 한다.** 시리즈 매칭은 기존대로 `seriesNames` 인덱스(또는 `storeSeriesId`)로 한다 — 이름을 바꿔도 색/순서/선택이 끊기면 안 된다.

### 2.6 [U6] (Ubiquitous) 시리즈 색상 승계 규칙

시스템은 `store_source.series[].color`(및 tag 모드의 자동 팔레트 `pickSeriesColor(index)`)를 패널별로 다음과 같이 반영한다. 색상은 `useStoreChartData` 의 `seriesStyles` 에서 시리즈 표시 이름 기준으로 읽는다.

| 패널 | 시리즈 색상 적용 대상 | 미지정 시 | 기존 하드코딩 대체 여부 |
|------|----------------------|-----------|--------------------------|
| `bar-chart` | 막대 채움색 | `pickSeriesColor(i)` | 다중 출력 모드에서만 `#3b82f6`([`BarChartPanel.tsx:131`](../../../web/src/pages/dashboard/panels/charts/BarChartPanel.tsx)) 대체 |
| `pie-chart` | 조각 채움색 | `pickSeriesColor(i)` | 다중 출력 모드에서만 `PIE_COLORS`([`PieChartPanel.tsx:33`](../../../web/src/pages/dashboard/panels/charts/PieChartPanel.tsx)) 대체 |
| `stat` | 타일의 **시리즈 라벨** 색 | 기본 라벨색 | 값 숫자 색은 대체하지 **않는다** |
| `gauge` | 게이지 **캡션 라벨** 색 | 기본 라벨색 | 게이지 호(arc) 색은 대체하지 **않는다** |

`stat` 의 값 숫자 색은 기존 `threshold_color_rules`([`pickThresholdColor`](../../../web/src/pages/dashboard/panels/charts/chartChannelUtils.ts))가 계속 소유한다. `gauge` 의 호 색은 기존 `thresholds` / `colorMode` / `colorTheme` 가 계속 소유한다. **값을 의미하는 색과 시리즈를 식별하는 색은 다른 축이며 서로를 덮어쓰지 않는다.**

`StoreSeriesRef` 의 라인 전용 스타일(`stroke_style` · `stroke_width` · `smooth`)은 4종 패널에서 **무시된다**(타입 주석이 이미 "라인 차트가 아닌 패널에서는 무시된다"고 규정).

### 2.7 [E1] (Event-driven) 대표값 변경은 재조회 없이 즉시 반영된다

**When** 사용자가 설정 화면에서 `series_reduce` 를 변경하면, **the system shall** 이미 폴링해 둔 윈도우 데이터로 대표값을 다시 계산해 즉시 렌더하고, Store 재조회를 **트리거하지 않는다**.

근거: `series_reduce` 는 `store_source` 밖에 있으므로 `useStoreChartData` 의 `pollKey`([`useStoreChartData.ts`](../../../web/src/pages/dashboard/panels/charts/useStoreChartData.ts))를 바꾸지 않는다. 이는 `seriesStyles` / `booleanSeries` 가 재조회 없이 반응적으로 재계산되는 기존 패턴(`reactiveSeriesStyles`)과 같은 성질이다.

반대로 `store_source.aggregation` · `time_window_ms` · `interval_ms` · 시리즈 선택 변경은 기존대로 재조회를 유발한다.

### 2.8 [E2] (Event-driven) 게이지 레거시 바인딩 이관

**When** 사용자가 게이지 설정 화면에서 "기존 값 바인딩을 Store 데이터 소스로 이전" 액션을 실행하면, **the system shall** `config.dataSources[]` 에서 첫 번째 유효한 `sourceType:'store'` 항목을 찾아 다음을 수행한다.

1. `store_source` 를 생성한다 — `agent_id = storeAgentId`, `agent_name = storeAgent`, `namespace = storeNamespace ?? 'default'`, `selection_mode:'keys'`, `series = [{ key: storeKey }]`, 시간창/인터벌/집계는 기본값(`defaultStoreSource()`).
2. `data_source = 'store'` 로 설정한다.
3. `series_reduce = 'last'` 로 설정한다 — 레거시 `mode:'latest'` 폴링과 의미가 가장 가깝다.
4. **`config.dataSources` 는 그대로 남긴다.**

시스템은 저장된 config 를 **자동으로 조용히 다시 쓰지 않아야 한다.** 이관은 사용자 조작으로만 일어난다.

`config.dataSources` 를 남기는 이유는 되돌리기다. 사용자가 데이터 소스 토글을 `channel` 로 되돌리면 §2.9 [S1] 에 따라 레거시 경로가 그대로 다시 유효해진다. 이관이 파괴적이면 되돌릴 수 없고, 되돌릴 수 없으면 사용자는 이관을 시도하지 않는다.

유효한 `sourceType:'store'` 항목이 없으면(예: `resource` · `flow` · `chart-emitter` 만 있는 경우) 액션은 비활성 상태로 표시되며 안내 문구를 제공한다.

### 2.9 [S1] (State-Driven) `series_reduce` 부재 = 레거시 렌더 경로

**While** `series_reduce` 가 config 에 없는 동안, **the system shall** 각 패널의 기존 렌더 규칙(§1.2.4 표)을 **한 픽셀도 바꾸지 않고** 유지한다.

이것이 하위 호환의 **단일 스위치**다. 신규 필드 하나의 유무가 경로 전체를 결정한다.

| 패널 | `series_reduce` 부재 (레거시) | `series_reduce` 존재 (신규) |
|------|-------------------------------|------------------------------|
| `stat` | 평탄화 `entries` 의 마지막 값 + 직전 entry 대비 delta 보조 표시 | 시리즈별 대표값 타일 배열. **보조 delta 줄 없음** |
| `bar-chart` | `mode`(category/time_bin) + `label_field` + `agg_func` | 시리즈당 막대 1개. `mode` · `label_field` · `agg_func` **무시** |
| `pie-chart` | `label_field` 그룹화 + `agg_func` + `max_points` 트리밍 | 시리즈당 조각 1개. `label_field` · `agg_func` · `max_points` **무시** |
| `gauge` | `config.dataSources[]` 우선순위(`chart-emitter > store latest > static value`) | `store_source` 경로. `dataSources[]` **전체 무시** |

**"기본 대표값"을 정의하지 않는 이유**: 만약 부재를 `series_reduce:'last'` 로 해석하면, 시리즈를 여러 개 고른 기존 `stat` 패널이 하룻밤 사이에 큰 숫자 하나에서 타일 배열로 바뀐다. 저장된 config 를 건드리지 않고도 화면이 달라지는 변경은 하위 호환이 아니다. 부재를 "레거시 모드"로 못박으면 기존 패널은 사용자가 명시적으로 대표값을 고르기 전까지 절대 변하지 않는다.

게이지의 경우 추가 조건이 있다: `series_reduce` 가 있어도 `data_source !== 'store'` 이거나 `store_source` 가 비활성(시리즈 0개 / tag 필터 0개)이면 레거시 `dataSources[]` 경로를 쓴다. 즉 **신규 경로가 실제로 데이터를 낼 수 있을 때만** 레거시를 밀어낸다.

### 2.10 [S2] (State-Driven) 채널 모드에서 `series_reduce` 는 무시된다

**While** `data_source !== 'store'` 인 동안, **the system shall** `series_reduce` 값이 config 에 있더라도 이를 읽지 않고 기존 채널 렌더 경로를 유지하며, 설정 UI 는 대표값 선택기를 노출하지 않는다.

근거: 대표값은 시리즈별 타임라인(`seriesEntries`)을 입력으로 한다. `stat` · `bar-chart` · `pie-chart` 는 채널 모드에서 단일 채널 훅(`useChartChannel`)만 쓰므로 시리즈 축이 존재하지 않는다. 채널 모드로 확장하려면 다채널 훅(`useChartChannels`) 도입이 선행되어야 하며 이는 본 SPEC 의 범위가 아니다(§7 OQ2).

config 에 남아 있는 `series_reduce` 는 **삭제하지 않는다.** 사용자가 Store 모드로 되돌아오면 그대로 다시 유효해져야 한다.

### 2.11 [O1] (Optional) 선택 기능

- **가능하면** 설정 화면의 라이브 미리보기가 실제 패널을 draft config 로 렌더해 대표값·다중 출력 결과를 그대로 보여준다(라인 차트의 `isStoreLinePreview` 선례, [`PanelSettingsDialog.tsx:858`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx)).
- **가능하면** 대표값 선택기 옆에 현재 조합의 의미를 한 줄로 설명한다(예: "1분 평균의 구간 최댓값").
- **가능하면** `stat` 타일에 대표값 이름을 작은 캡션으로 표시해 "이 숫자가 무엇인지"를 화면만 보고 알 수 있게 한다.
- **가능하면** `multi_output_limit` 초과로 잘린 시리즈 목록을 툴팁으로 노출한다.
- **가능하면** 불리언 시리즈에 `avg` 를 고르면 결과를 백분율(duty %)로 표시하는 옵션을 제공한다.

### 2.12 [UB1] (Unwanted-Behavior) 금지 동작

시스템은 다음을 허용하지 않는다.

| # | 금지 동작 | 대응 |
|---|-----------|------|
| 1 | `series_reduce` 를 `store_source.aggregation` 에 병합하거나 한 필드로 겸용 | 두 필드는 독립 유지(§2.1). 어느 한쪽 값이 다른 쪽으로 자동 복사되지 않는다 |
| 2 | `series_reduce` 도입으로 기존 저장 config 의 렌더 결과가 달라짐 | 부재 = 레거시 경로(§2.9). 저장 config 무변경 |
| 3 | 저장된 `config.dataSources[]`(게이지)를 자동으로 삭제·재작성 | 사용자 조작 이관만 허용(§2.8). 이관 후에도 원본 보존 |
| 4 | 표본 0개 윈도우에서 `sum` 을 `0` 으로 표시 | `undefined` → `—`. `0` 은 "값이 0" 과 구분되지 않는다 |
| 5 | 비수치(문자열) 값을 `0` 으로 강제 변환해 집계에 포함 | 표본에서 제외(§2.2) |
| 6 | `pie-chart` 에 **음수** 대표값을 조각으로 렌더 | 해당 시리즈 조각 생략 + 패널에 사유 안내. 파이는 음수를 표현할 수 없다(§4.4) |
| 7 | 표시 이름을 시리즈 동일성 판정·색상 매칭에 사용 | 인덱스/`storeSeriesId` 기준 유지(§2.5) |
| 8 | `series_reduce` 변경이 Store 재조회를 유발 | `pollKey` 불변(§2.7) |
| 9 | `gauge` 를 `CHART_PANEL_TYPES` 에 추가 | `dataSourceBelowPreview` 조건만 확장(§2.3) |
| 10 | `line-chart` · `table` · `heatmap` 이 `series_reduce` 를 읽음 | 해당 3종은 무시. 설정 UI 에도 미노출 |
| 11 | 다중 출력 시 시리즈 순서를 값 크기로 재정렬 | config 순서 유지(§2.4). 매 폴링마다 타일이 자리를 바꾸면 읽을 수 없다 |
| 12 | 백엔드 스키마 변경 또는 신규 엔드포인트 추가 | config 는 불투명 JSON 유지, 기존 store 쿼리 경로만 사용 |

### 2.13 [UB2] (Unwanted-Behavior) 상태 불일치

1. 시리즈 선택을 모두 해제했는데 `series_reduce` 만 남으면, 패널은 신규 경로의 **빈 상태**를 보여야 한다(레거시 경로로 몰래 되돌아가지 않는다). 게이지는 예외로 §2.9 의 추가 조건에 따라 레거시로 폴백한다 — 게이지만 레거시가 여전히 값을 낼 수 있기 때문이며, 이 비대칭은 의도된 것이다.
2. tag 모드에서 매칭 키가 0개가 되면 출력도 0개이며, 이는 오류가 아니라 정상적인 빈 상태다(기존 `useStoreChartData` 의 `status:'connected'` + 빈 결과 동작 승계).
3. 폴링이 실패해도 마지막 성공 렌더를 파괴하지 않는다. 기존 훅이 `status:'error'` 만 갱신하고 데이터를 보존하는 동작을 그대로 쓴다.
4. 대표값이 `undefined` 인 것과 폴링 실패는 서로 다른 상태이며 화면에서 구분되어야 한다(`—` vs 오류 오버레이).

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 두 축 분리 | `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts`(`SeriesReduceFunc` + `ChartPanelConfigBase.series_reduce`) | AC-01, AC-02 |
| U2 대표값 7종 | `web/src/pages/dashboard/panels/charts/seriesReduce.ts`(신규) | AC-03 ~ AC-07 |
| U3 데이터소스 동등화 | `PanelSettingsDialog.tsx:445`(`dataSourceBelowPreview`), `ChartPanelSections.tsx`(`StoreSourceSection` 내 대표값 선택기) | AC-08, AC-09 |
| U4 다중 출력 | `StatPanel.tsx`, `GaugePanel.tsx`, `BarChartPanel.tsx`, `PieChartPanel.tsx`, `charts/SeriesTileGrid.tsx`(신규) | AC-10 ~ AC-14 |
| U5 표시 이름 재사용 | `chartChannelTypes.ts:297`(`storeSeriesLabel`), `useStoreChartData.ts`(`seriesNames`) | AC-15 |
| U6 색상 승계 | `useStoreChartData.ts`(`seriesStyles`), 4종 패널 렌더 | AC-16, AC-17 |
| E1 즉시 반영 | `useStoreChartData.ts`(`pollKey` 불변) | AC-18 |
| E2 게이지 이관 | `PanelSettingsDialog.tsx`(`GaugeSection` 이관 액션), `charts/gaugeLegacyBinding.ts`(신규 순수 모듈) | AC-19, AC-20 |
| S1 레거시 경로 보존 | 4종 패널 전부 | AC-21 ~ AC-24 (특성화) |
| S2 채널 모드 무시 | `StatPanel.tsx`, `BarChartPanel.tsx`, `PieChartPanel.tsx` | AC-25 |
| O1 선택 기능 | `PanelSettingsDialog.tsx` 미리보기 | (선택) |
| UB1 금지 동작 | 전 파일 | AC-26 ~ AC-30 |
| UB2 상태 정합 | 4종 패널 빈/오류 분기 | AC-31, AC-32 |

---

## 4. 설계 결정

### 4.1 `series_reduce` 를 `store_source` 밖에 둔다

`store_source.reduce` 로 두는 안도 검토했다. `aggregation` 바로 옆이라 두 축의 대조가 눈에 띈다는 장점이 있다. 그러나 `store_source` 는 `useStoreChartData` 의 `pollKey` 소재지이고, `pollKey` 는 "조회를 다시 해야 하는가"를 판정한다. 표현 파라미터를 조회 블록에 넣으면 두 가지 나쁜 선택지만 남는다 — 포함하면 대표값 변경마다 불필요한 네트워크 왕복이 생기고, 제외하면 같은 블록 안에서 필드마다 취급이 갈려 다음 사람이 실수한다.

`ChartPanelConfigBase.series_reduce` 는 블록 경계가 곧 축 경계가 되게 한다: **`store_source` = 무엇을 어떻게 가져올 것인가, 패널 config = 가져온 것을 어떻게 보일 것인가.**

### 4.2 신규 필드의 **유무**를 하위 호환 스위치로 쓴다

값(예: `'legacy'` 리터럴 추가)이 아니라 유무로 가르는 이유는, 값으로 가르면 "기본값이 무엇인가"라는 질문이 생기고 그 답이 무엇이든 저장된 config 를 읽는 시점의 해석을 바꾸기 때문이다. 유무 스위치는 마이그레이션이 필요 없다 — 기존 config 에는 그 키가 물리적으로 존재하지 않는다.

대가: 코드에 `cfg.series_reduce !== undefined` 분기가 4개 패널에 각각 생긴다. 이는 명시적이고 검색 가능하므로 감수한다.

### 4.3 빈 윈도우의 `sum` 은 `0` 이 아니라 "값 없음"이다

수학적으로 빈 합은 0 이다. 그러나 대시보드에서 `0` 은 강한 주장이다 — "측정했고 결과가 0 이다". 데이터가 없는 것과 값이 0 인 것을 같은 글리프로 표시하면 센서 단선을 정상으로 오독한다. 7종 중 `count` 만 빈 윈도우에서 `0` 을 갖는데, `count` 는 값이 아니라 **개수**이므로 "0개를 셌다"가 정확한 진술이기 때문이다.

### 4.4 파이의 음수 대표값은 조각을 생략한다

`delta` 는 음수가 될 수 있고 `min`/`sum` 도 원본 데이터에 따라 음수가 될 수 있다. 파이 차트에서 음수는 표현 불가능하다. 세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| 절댓값 사용 | −5 와 +5 가 같은 크기 조각이 되어 부호가 소실된다. 조용한 거짓말 |
| 0 으로 clamp | 조각이 사라지지만 사용자는 이유를 모른다 |
| **조각 생략 + 사유 안내** | 조각이 사라지고 이유가 화면에 남는다 |

세 번째를 택한다. 다만 "음수가 나올 수 있는 대표값 + 파이" 조합 자체가 사용자 실수일 가능성이 높으므로, 설정 화면에서 `pie-chart` + `delta` 조합 선택 시 경고를 함께 노출한다.

### 4.5 게이지 레거시 바인딩은 병행 지원한다 (제거·자동이관 아님)

세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| 자동 이관(서버/클라이언트가 config 재작성) | 되돌릴 수 없다. `resource`/`flow` 소스는 대응하는 `store_source` 형상이 아예 없어 이관 대상이 아니며, 부분 이관은 config 를 반쪽 상태로 만든다 |
| 레거시 즉시 제거 | 저장된 모든 게이지 패널이 깨진다. `resource`(CPU/메모리)와 `flow` 바인딩은 Store 로 대체 불가 |
| **병행 지원 + 사용자 조작 이관** | 기존 패널 무변경, 신규 경로는 명시적 opt-in, 되돌리기 가능 |

세 번째를 택한다. **[v0.3.0 근거 정정]** 원래 근거는 "`resource` · `flow` 가 Store 로 대체 불가하므로 `dataSources[]` 가 살아 있어야 한다" 였으나, §6-5 정정대로 그 두 소스는 애초에 동작하지 않는다. 결정 자체는 유지되며 근거는 다음으로 대체된다 — 저장된 게이지 config 의 `store` · `chart-emitter` 항목이 실제로 값을 공급하고 있고(CH-13 · CH-14), 자동 이관은 되돌릴 수 없으며, 즉시 제거는 그 패널들을 깨뜨린다. `resource` / `flow` 항목의 존치 여부는 본 SPEC 이 판단하지 않는다(동작하지 않는 config 의 정리는 별도 이슈).

우선순위는 "신규 경로가 실제로 값을 낼 수 있으면 신규가 이긴다"로 못박는다. `data_source === 'store'` 는 사용자가 토글로 명시한 상태이므로, 그 상태에서 레거시 `chart-emitter` 가 계속 이기면 사용자는 토글이 고장난 것으로 인식한다.

### 4.6 다중 출력 상한을 두되 시리즈 선택 상한과 분리한다

시리즈 선택 상한(`STORE_SERIES_LIMIT = 48`)은 **조회 부하** 보호이고, 다중 출력 상한(12)은 **가독성** 보호다. 48개 게이지를 한 패널에 그리면 각각이 판독 불가능한 크기가 되므로, 조회는 되지만 표시는 잘리는 상태가 정상이다. 두 상한을 하나로 합치면 "라인 차트로는 48개를 볼 수 있는데 게이지로는 12개만"이라는 축 차이를 표현할 수 없다.

### 4.7 `AggFunc` 를 폐기하지 않는다

`AggFunc`(`count`/`sum`/`avg`)와 `SeriesReduceFunc`(7종)는 값 집합이 겹치지만 **적용 대상이 다르다**. `AggFunc` 는 "라벨 그룹 / 시간 bin 안의 표본들"에, `SeriesReduceFunc` 는 "한 시리즈의 윈도우 전체"에 적용된다. 타입을 합치면 `bar-chart` 의 `time_bin` 모드에서 `last`/`delta` 같은 선택지가 노출되어 의미 없는 조합이 생긴다. 두 타입을 분리 유지하고, `series_reduce` 존재 시 `agg_func` 를 무시하는 규칙으로 충돌을 없앤다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 성능 | 대표값 계산은 시리즈당 O(표본 수) 단일 패스. 48시리즈 × 1000버킷 기준 프레임당 1회 `useMemo` 재계산이 16ms 내에 끝난다 |
| 네트워크 | 신규 요청 0. 기존 `POST /api/v1/store/{agent}/query` 폴링만 사용. 대표값 변경은 재조회를 유발하지 않는다 |
| 하위 호환 | `series_reduce` 부재 config 는 렌더 결과가 바이트 동일해야 한다. 특성화 테스트로 고정한다 |
| 백엔드 | Go 코드 변경 0. 패널 config 는 불투명 JSON 유지 |
| 테스트 | 신규 코드 커버리지 85% 이상(quality.yaml). `seriesReduce.ts` 는 7종 × (정상/빈/전null/비수치/단일표본/불리언) 전수 테스트 |
| 접근성 | 다중 출력 그리드의 각 타일은 시리즈 이름을 접근 가능한 텍스트로 가진다. 잘림 표기(`+K`)는 스크린리더에 개수를 전달한다 |
| 국제화 | 신규 문구(대표값 7종 라벨, 이관 액션, 잘림 표기, 경고)는 `web/src/lib/i18n/{ko,en}.json` 두 로케일 모두 추가 |
| LSP | `tsc --noEmit` 0 에러, eslint 0 에러 |

---

## 6. 가정 및 제약

1. `useStoreChartData` 의 `seriesEntries` / `seriesNames` / `seriesStyles` 반환 형상은 안정적이며 본 SPEC 은 이를 **소비만** 한다. 훅 자체는 수정하지 않는다.
2. `data_type === 'boolean'` 시리즈의 값은 Store 변환 시점에 이미 `1`/`0` 으로 정규화되어 도착한다([`useStoreChartData.ts` `matrixToEntries`](../../../web/src/pages/dashboard/panels/charts/useStoreChartData.ts) 주석 근거).
3. `avg` 는 표본 개수 기준 산술 평균이며 버킷 간격이 균일하다는 전제 위에 있다. `interval_ms` 가 균일하므로 시간 가중 평균과 결과가 일치한다. 결측 버킷(null)이 많으면 편향이 생길 수 있으나 본 SPEC 은 이를 보정하지 않는다.
4. 패널 config 는 불투명 JSON 이므로 신규 필드 추가에 백엔드 스키마 변경이 필요 없다([SPEC-DASHBOARD-001](../SPEC-DASHBOARD-001/spec.md) 승계).
5. ~~게이지의 `resource` · `flow` 바인딩은 Store 로 대체 불가하며 본 SPEC 이후에도 레거시 경로로만 동작한다.~~ **[v0.3.0 정정 — 이 가정은 사실이 아니다]** M2 특성화 작업 중 코드 직접 확인 결과, `GaugePanel.tsx` 에는 `pickChartEmitterSource` 와 `pickStoreSource` 두 개의 리더만 존재하고 `resource` / `flow` 를 읽는 코드가 **아예 없다**. 두 sourceType 은 `GaugeDataSource` 타입 유니온(L48-50)에 선언만 되어 있고, 해당 바인딩이 설정된 게이지는 `hasBinding === false` 가 되어 정적 `config.value`(기본 0)를 조용히 렌더한다. 따라서 정확한 가정은 다음과 같다 — **`resource` / `flow` 바인딩은 현재 동작하지 않으며, 본 SPEC 은 그 상태를 바꾸지 않는다**(CH-11 · CH-12 가 이 현재 동작을 잠갔다). 이는 본 SPEC 범위 밖의 선행 결함이므로 별도 이슈로 다룬다.
6. 시리즈 표시 이름은 중복될 수 있다(`storeSeriesLabel` 결과가 같은 두 시리즈). 이 경우 `useStoreChartData` 의 기존 병합 규칙(같은 이름의 타임라인을 합침)이 적용되어 출력도 하나로 합쳐진다. 이는 기존 동작이며 본 SPEC 이 바꾸지 않는다.
7. 대상 프론트엔드는 React 19 + TypeScript 5.9 + Vitest 이며 렌더 라이브러리는 기존 recharts(`bar`/`pie`)와 자체 SVG(`gauge`)를 그대로 쓴다.

---

## 7. 설계 질문 확정 내역 (v0.2.0 에서 확정됨)

아래 항목은 v0.1.0 시점에 **잠정 결정**으로 기록되었고, 구현 착수 승인 시 **잠정 결정 그대로 확정**되었다.
따라서 "잠정 결정" 열의 값이 곧 확정된 구현 규칙이며, "대안" 열은 후속 SPEC 에서 재검토할 여지를 남긴 기록이다.

| # | 질문 | 잠정 결정 | 대안 |
|---|------|-----------|------|
| OQ1 | 다중 출력 표시 상한(§2.4)의 기본값과 초과 처리 | 기본 12개, 초과분은 잘라내고 `+K` 표기 | (a) 상한 없음 + 그리드 스크롤 (b) 다른 기본값 (c) 패널 크기에 따라 자동 결정 |
| OQ2 | 채널(chart-emitter) 모드에도 구간 대표값을 제공할 것인가(§2.10) | 제공하지 않음(Store 전용) | 다채널 훅(`useChartChannels`) 도입 후 후속 SPEC 에서 확장 |
| OQ3 | "여러 시리즈를 하나의 출력으로 합치는" 모드가 필요한가(예: 전 시리즈 합계를 stat 1개로) | 제공하지 않음(다중 선택 = 다중 출력) | `output_mode: 'per-series' \| 'combined'` 축 추가 |
| OQ4 | 게이지 레거시 `config.dataSources[]` 의 폐기 시점 | 본 SPEC 에서는 폐기하지 않음 | 후속 SPEC 에서 `store`/`chart-emitter` 항목만 폐기하고 `resource`/`flow` 는 존치 |
| OQ5 | `stat` 다중 출력 타일에 보조 지표(예: 직전 폴링 대비 변화)를 함께 표시할 것인가 | 표시하지 않음(대표값 1개만) | 타일에 보조 delta 줄 복원 |
| OQ6 | `pie-chart` 의 음수 대표값 처리(§4.4) | 조각 생략 + 사유 안내 | (a) 절댓값 (b) 0 clamp (c) `pie` + 음수 가능 대표값 조합 자체를 설정에서 금지 |
| OQ7 | 대표값을 시리즈별로 다르게 지정할 수 있어야 하는가 | 패널 단위 단일 설정 | `StoreSeriesRef.reduce?` 추가로 시리즈별 override |
| OQ8 | `bar-chart` 의 기존 `time_bin` 모드와 신규 다중 출력의 관계 — `series_reduce` + `time_bin` 을 동시에 의미 있게 조합할 여지가 있는가(시리즈 × 시간bin 그룹 막대) | 조합 불가(`series_reduce` 존재 시 `mode` 무시) | 그룹 막대(grouped bar) 렌더 도입 |
