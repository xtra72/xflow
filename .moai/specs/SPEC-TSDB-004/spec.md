---
id: SPEC-TSDB-004
title: TSDB 시리즈 group by — 태그 키를 지정하면 태그 값으로 시리즈를 나눈다
version: 0.1.0
status: draft
created: 2026-08-23
updated: 2026-08-23
author: xtra
priority: medium
domain: tsdb
related_specs:
  - SPEC-TSDB-002
  - SPEC-TSDB-003
  - SPEC-STORE-004
  - SPEC-WEB-005
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-23 | xtra | 최초 작성 — `TsdbSeriesRef` 에 `group_by` 축을 더해 태그 값 단위로 시리즈를 나눈다. 항목별 모드(정확 일치 ↔ group by)이며 기존 config 는 무변경으로 동작한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

패널에서 **태그 키를 지정하면 그 태그의 값마다 시리즈가 하나씩 생기게** 한다. 다중 태그 키를 지정하면 값 조합마다 하나씩 생긴다.

지금은 사용자가 시리즈를 하나씩 열거해야 한다. `host` 가 30대면 `TsdbSeriesRef` 를 30개 만들어야 하고, 장비가 늘면 패널 설정을 손으로 고쳐야 한다. `group_by: ['host']` 한 줄이면 그 30개가 자동으로 나오고 장비 증감이 그대로 반영된다.

### 1.2 배경

**(1) 다중 시리즈 응답 파이프라인은 이미 존재한다.**

[SPEC-STORE-004](../SPEC-STORE-004/spec.md) 가 세운 라벨 규약이 이미 끝에서 끝까지 깔려 있다.

| 지점 | 현재 상태 |
|------|-----------|
| `chartQueryEntry.Labels` (`store_query.go:227`) | 존재. `{"__field__": field, ...tags}` |
| `chartQueryResponse` (`store_query.go:231`) | 다중 시리즈를 **단일 `entries` 배열로 평탄화**하고 `labels` 로 구분한다(`store_query.go:331`) |
| `buildInfluxSeriesEntries` (`influxdb_series.go:271`) | TSDB 도 **이미 labels 를 채운다** |
| `fetchTsdbSeries` (`tsdbSource.ts`) | **이미 `PivotKeySeries[]` 복수형을 반환**한다 |
| `parseSeriesLabels` (`seriesLabels.ts:42`) | `__field__` 를 metric 으로, 나머지를 태그로 분해 |

`fetchTsdbSeries` 의 주석은 이 용례를 명시적으로 예고한다 — *"태그를 부분만 지정해 여러 시리즈가 매칭되면 각각 독립 컬럼이 되어야 하기 때문"*. 즉 프론트의 그룹화·피벗 경로는 **이미 group by 를 받을 준비가 되어 있다.**

**(2) 따라서 `parseSeriesLabels` 는 바꾸지 않는다.**

[SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §7 **NQ2** 는 다중 시리즈 응답 확장이 "순수 가산이 아니다" 라고 경고하며 `parseSeriesLabels` 변경과 Store 표시 이름 경로 파급을 지목한다. 그 경고의 전제는 **한 응답에 여러 measurement 가 섞이는 경우**이며, `labels` 에 measurement 예약 키를 새로 만들어야 한다는 것이 파급의 원인이다.

본 SPEC 은 그 경우가 아니다. group by 는 **measurement 1개 · field 1개** 안에서 태그 값만 나눈다. `__field__` 는 그룹 사이에서 상수이고 변하는 것은 태그뿐이므로, 기존 라벨 규약이 그대로 표현한다. **예약 키를 새로 만들지 않으며 Store 공유 경로에 파급이 없다.**

**(3) 백엔드는 정반대 방향으로 고정되어 있다.**

현재 질의 경로는 **1요청 = 1시리즈**로 의도적으로 평탄화되어 있다.

| # | 지점 | 현재 | 문제 |
|---|------|------|------|
| 1 | `SeriesQuerySpec` (`influxdb_seriesquery.go:71`) | `Tags map[string]string` — 정확 일치 필터뿐 | "이 키로 묶어라" 축이 없다 |
| 2 | Flux 생성 (`influxdb_seriesquery.go:326`) | `keep(columns: ["_time","_value"])` | **태그 컬럼을 전부 버린다** |
| 3 | InfluxQL 생성 (`influxdb_seriesquery.go:383`) | `GROUP BY time(%s) FILL(%s)` | 태그 키 자리가 없다 |
| 4 | `SeriesBucket` (`influxdb_agent.go:539`) | `{StartMs, Value}` | **태그를 운반하지 못한다** |
| 5 | `buildInfluxSeriesEntries` (`influxdb_series.go:271`) | **요청**의 `req.Tags` 를 복사 | 행별 실제 그룹 태그가 아니라 상수다 |

**(4) 두 백엔드 모두 원재료는 나온다 — 실측 근거가 있다.**

- **v2**: `queryFlux`(`influxdb_v2.go:83`)가 결과 행의 **모든 컬럼**을 통과시킨다. `keep` 만 좁히지 않으면 태그가 살아온다. 이 불변식은 `TestInfluxSeriesEnum_V2_쿼리플럭스_왕복` 이 고정하고 있다.
- **v3**: [SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) §HISTORY-0.5.0 이 로컬 `influxdb:3-core` 컨테이너로 **실측 완료** — InfluxQL `GROUP BY *` 응답에서 태그가 각 행의 평탄한 컬럼으로 드러남을 확인했고, 그 결과 OQ2 가 `GROUP BY *` 채택으로 확정되었다. 본 SPEC 의 `GROUP BY time(d), "t1", "t2"` 는 그 경로의 부분 지정 형태다.

**(5) 표시 메타데이터 매핑이 위치 기반이라 정면으로 충돌한다.**

`matrixToEntries`(`useStoreChartData.ts:195`)는 다음 한 줄로 메타데이터를 정렬한다.

```ts
const aligned = matrix.columns.length === config.series.length;
```

컬럼 수와 요청 시리즈 수가 **정확히 같을 때만** `alias` · `color` · `stroke_style` · `stroke_width` · `smooth` · `data_type` 이 위치 순서대로 매핑된다. group by 는 config 항목 1개가 컬럼 N개를 만들므로 이 등식을 설계상 깬다.

파급이 국소적이지 않다. `aligned` 는 **패널 전체에 대한 단일 불리언**이므로, group by 항목이 하나라도 있으면 같은 패널의 **정확 일치 항목까지** 전부 메타데이터를 잃고 자동 팔레트와 내장 서술 표기로 떨어진다. 이것이 본 SPEC 의 중심 설계 문제이며 §4.1 · OQ1 이 다룬다.

### 1.3 비범위 (Out of Scope)

- **Store 소스의 group by.** 본 SPEC 은 TSDB 소스에 한정한다. Store 는 `StoreSeriesRef` 와 별도 어댑터를 쓰며, [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §1.3 의 "Store 무변경" 원칙을 승계한다.
- **NQ2(요청 배칭).** group by 는 요청 1건이 시리즈 N개를 돌려주므로 결과적으로 요청 수를 줄이지만, 그것은 부수 효과다. `series[]` 배열을 한 요청에 싣는 배칭은 열지 않는다.
- **NQ3(Store `Promise.all` → `allSettled` 정렬).** 무관하며 열지 않는다.
- **여러 measurement 혼재.** 한 요청은 measurement 1개 · field 1개를 유지한다. 이것이 §1.2 (2) 의 무파급 근거이므로 범위를 넓히면 근거가 무너진다.
- **태그 값 기준 정렬·필터 UI.** 그룹이 나온 뒤 사용자가 특정 값만 고르는 기능은 본 SPEC 밖이다. 사전 필터(`tags`)로 좁히는 기존 수단이 있다.

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) group by 는 시리즈 항목의 축이다

시스템은 `TsdbSeriesRef` 에 **옵셔널** `group_by?: string[]` 를 둔다. 값은 태그 키 목록이다.

한 시리즈 항목은 다음 둘 중 하나로 동작한다.

| 모드 | 조건 | 의미 |
|------|------|------|
| 정확 일치 | `group_by` 부재 또는 빈 배열 | 현행. `tags` 로 좁힌 **단일** 시리즈 |
| group by | `group_by` 에 키가 1개 이상 | `tags` 로 좁힌 뒤 그 키들의 **값 조합마다** 시리즈 1개 |

**`tags` 와 `group_by` 는 대체 관계가 아니라 직교한다.** `tags` 는 사전 필터(어느 데이터를 볼지), `group_by` 는 분할 축(어떻게 나눌지)이다. `tags: {region: 'kr'}` + `group_by: ['host']` 는 "kr 리전 안에서 host 별로" 를 뜻한다.

같은 키가 양쪽에 오면 **거부한다**(§2.8 UB1-3) — 값이 하나로 고정된 키로 나누면 그룹이 항상 1개이고, 사용자가 의도한 것과 다를 가능성이 높다.

### 2.2 [U2] (Ubiquitous) 질의 스펙에 그룹 축을 신설한다

시스템은 `SeriesQuerySpec` 에 `GroupBy []string` 을 더한다. 생성 순서는 `Tags` 와 같이 **키 오름차순으로 고정**한다 — 슬라이스 순서를 그대로 쓰면 같은 의미의 요청이 다른 쿼리 문자열을 만들어 캐시·테스트·로그 대조가 어긋난다.

`GroupBy` 의 각 키는 `ValidateIdentifiers` 의 검증을 `Tags` 키와 동일하게 받는다.

### 2.3 [U3] (Ubiquitous) v2 — 그룹 키 컬럼을 보존하고 명시적으로 그룹한다

`GroupBy` 가 비어 있지 않으면 시스템은 Flux 생성에서

1. `keep(columns:)` 목록에 그룹 키를 **추가**하고,
2. `aggregateWindow` **앞에** `group(columns: [<그룹 키>])` 를 넣는다.

`GroupBy` 가 비어 있으면 생성 결과는 **바이트 단위로 현행과 같아야 한다**(§2.9 U9).

`group()` 을 `aggregateWindow` 앞에 두는 이유는 §4.2 다.

### 2.4 [U4] (Ubiquitous) v3 — `GROUP BY` 에 태그 키를 더한다

`GroupBy` 가 비어 있지 않으면 시스템은 InfluxQL 생성에서 `GROUP BY time(d)` 뒤에 그룹 키를 이어 붙인다.

```sql
GROUP BY time(60000ms), "host", "rack" FILL(null)
```

`time(d)` 를 **첫 자리에 유지한다.** 버킷 경계 계약([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.8)이 `offset` 인자 부재에 얹혀 있으므로 그 형태를 흔들지 않는다. 태그 키 추가는 버킷 경계에 영향을 주지 않으며, 이것은 AC 로 고정한다(§AC-08).

`GroupBy` 가 비어 있으면 생성 결과는 현행과 바이트 단위로 같아야 한다.

### 2.5 [U5] (Ubiquitous) 버킷은 자기 그룹의 태그를 운반한다

시스템은 `SeriesBucket` 에 `Tags map[string]string` 을 더한다. 값은 **그 버킷이 속한 그룹의 실제 태그 값**이다.

`GroupBy` 가 비어 있으면 `Tags` 는 `nil` 이다 — 정확 일치 모드에서 태그는 요청이 이미 알고 있으므로 응답에 실을 이유가 없고, `nil` 로 두어야 §2.9 의 무변경이 성립한다.

### 2.6 [U6] (Ubiquitous) 라벨은 요청이 아니라 결과에서 만든다

시스템은 `buildInfluxSeriesEntries` 에서 각 엔트리의 `labels` 를 다음으로 구성한다.

```
labels = {"__field__": req.Field} ∪ req.Tags ∪ bucket.Tags
```

`bucket.Tags` 가 뒤에 오므로 **그룹의 실제 값이 요청 값을 덮는다.** 정확 일치 모드에서는 `bucket.Tags` 가 비어 있어 현행과 결과가 같다.

`__field__` 는 모든 그룹에서 **상수**다. 이것이 §1.2 (2) 의 무파급 근거이므로, 어떤 구현도 `__field__` 를 그룹마다 다르게 만들어서는 안 된다.

### 2.7 [U7] (Ubiquitous) 그룹 수 상한과 절단 신호

시스템은 한 요청이 만드는 **그룹 수에 서버 상한**을 강제한다.

| 축 | 상한 | 강제 위치 |
|----|------|-----------|
| 한 요청의 그룹 수 | **OQ2** | 핸들러 |
| 패널 전체 시리즈 수 | 기존 48 승계 | 클라이언트 |

상한에 걸리면 응답의 `truncated` 를 `true` 로 둔다.

**시스템은 절단을 조용히 수행해서는 안 된다.** UI 는 절단 배너와 함께 **좁히는 방법**을 제시한다 — 태그 사전 필터를 걸거나 그룹 키를 줄이는 것. [SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) §2.7 이 열거 카디널리티에 대해 세운 것과 같은 원칙이며, `too many series` 만으로는 사용자가 무엇을 해야 할지 알 수 없다.

**상한은 서버에 둔다.** 클라이언트 상한은 InfluxDB 가 계산을 마친 뒤에야 작동하므로 백엔드 부하를 막지 못한다.

### 2.8 [UB1] (Unwanted-Behavior) 금지 동작

| # | 금지 |
|---|------|
| UB1-1 | `group_by` 부재·빈 배열인 저장된 config 의 렌더 결과가 달라지는 것 |
| UB1-2 | `group_by` 가 비었을 때 생성 쿼리 문자열이 현행과 달라지는 것 |
| UB1-3 | 같은 키가 `tags` 와 `group_by` 에 동시에 오는 요청을 **수락**하는 것 — 400 으로 거부한다 |
| UB1-4 | 존재하지 않는 태그 키로 group by 했을 때 500 을 내는 것 — 그룹 0개 또는 빈 태그 그룹 1개로 정상 응답한다 |
| UB1-5 | 그룹 수 상한 초과를 `truncated` 없이 조용히 자르는 것 |
| UB1-6 | `__field__` 를 그룹마다 다른 값으로 만드는 것 |
| UB1-7 | group by 항목 하나 때문에 **같은 패널의 정확 일치 항목**이 색·별칭을 잃는 것 (§4.1) |
| UB1-8 | 그룹 순서가 폴링마다 달라져 색이 춤추는 것 — 태그 값 사전순으로 안정 정렬한다 |
| UB1-9 | `GROUP BY` 에 `time(d)` 아닌 것을 첫 자리에 두어 버킷 경계 계약을 흔드는 것 |

### 2.9 [U9] (Ubiquitous) 저장된 config 의 렌더 결과 불변

`group_by` 가 없는 기존 config 는 **질의 문자열 · 응답 · 렌더 결과가 전부 현행과 같아야 한다.** 이는 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.4 가 요구한 렌더 불변의 승계이며, 특성화 테스트로 고정한다.

### 2.10 [E1] (Event-driven) 그룹 키 선택이 재질의를 촉발한다

**When** 사용자가 설정 UI 에서 그룹 키 집합을 바꾸면, 시스템은 해당 시리즈 항목의 질의를 다시 수행하고 결과 시리즈 집합을 교체한다.

### 2.11 [S1] (State-Driven) group by 상태를 구분해 표시한다

**While** 시리즈 항목이 group by 모드이면, 설정 UI 는 그 항목이 런타임에 **여러 시리즈로 펼쳐진다**는 사실과 마지막 질의에서 나온 **그룹 수**를 표시한다. 정확 일치 항목과 시각적으로 구분되어야 한다.

### 2.12 [O1] (Optional) 선택 기능

- 그룹 키 선택 시 [SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) 의 태그 키 디스커버리(D2)를 재사용해 **실재하는 태그 키만** 고르게 한다. 자유 입력도 허용하되 디스커버리 결과를 우선 제시한다.

---

## 3. 트레이서빌리티 표

| 요구 | 구현 지점 | AC |
|------|-----------|-----|
| U1 항목별 모드 | `chartChannelTypes.ts` `TsdbSeriesRef.group_by` | AC-01 · AC-02 |
| U2 질의 스펙 축 | `influxdb_seriesquery.go` `SeriesQuerySpec.GroupBy` | AC-03 |
| U3 v2 생성 | `influxdb_seriesquery.go` `BuildFluxSeriesQuery` | AC-04 · AC-06 |
| U4 v3 생성 | `influxdb_seriesquery.go` `BuildInfluxQLSeriesQuery` | AC-05 · AC-06 · AC-08 |
| U5 버킷 태그 | `influxdb_agent.go` `SeriesBucket.Tags` + `QuerySeriesBuckets` | AC-07 |
| U6 라벨 구성 | `influxdb_series.go` `buildInfluxSeriesEntries` | AC-09 · AC-10 |
| U7 상한·절단 | `influxdb_series.go` 핸들러 | AC-11 · AC-12 |
| U9 렌더 불변 | 특성화 테스트 | AC-06 · AC-13 |
| UB1-3 키 중복 거부 | `influxdb_series.go` 요청 검증 | AC-14 |
| UB1-7 메타데이터 정렬 | `useStoreChartData.ts` `matrixToEntries` | AC-15 · AC-16 |
| UB1-8 안정 정렬 | 핸들러 또는 어댑터 | AC-17 |
| E1 재질의 | `TsdbSourceSection.tsx` | AC-18 |
| S1 상태 표시 | `TsdbSourceSection.tsx` | AC-19 |

---

## 4. 설계 결정

### 4.1 위치 기반 메타데이터 정렬을 라벨 기반으로 바꾼다

**문제.** `matrixToEntries` 의 `aligned = matrix.columns.length === config.series.length` 는 패널 전체에 대한 단일 불리언이다. group by 항목 하나가 컬럼을 늘리면 등식이 깨지고 **패널의 모든 시리즈**가 `alias` · `color` · `stroke_style` · `stroke_width` · `smooth` · `data_type` 을 잃는다. group by 를 쓰는 순간 정확 일치 항목까지 자동 팔레트로 떨어지는 것은 수용할 수 없다(UB1-7).

**방향.** 위치 정렬을 **라벨 기반 귀속**으로 바꾼다. 각 컬럼이 어느 config 항목에서 나왔는지를 컬럼 라벨과 항목의 `key`/`field`/`tags`/`group_by` 로 판정하고, 그 항목의 메타데이터를 상속시킨다. 정확 일치 항목은 지금과 같이 1:1 로 귀속되므로 **결과가 변하지 않는다.**

group by 항목에서 파생된 N개 시리즈의 **색**을 어떻게 배정할지는 확정되지 않았다 — OQ1.

**이 변경은 Store 와 공유하는 함수를 건드린다.** `matrixToEntries` 는 `useStoreChartData.ts` 에 있고 Store 경로가 같이 쓴다. §1.3 의 "Store 무변경" 은 **Store 의 데이터 취득 경로**에 대한 것이고, 표시 계층의 공용 변환기는 두 소스가 이미 한 벌을 공유하도록 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 가 설계한 것이다(`asSeriesConfig` 가 TSDB config 를 `StoreSourceConfig` 로 투영하는 이유). 따라서 여기서의 변경은 **Store 의 렌더 결과를 바꾸지 않는 한** 허용되며, 그 불변은 AC-16 이 고정한다.

### 4.2 v2 는 `group()` 을 `aggregateWindow` **앞**에 둔다

Flux 의 `aggregateWindow` 는 **현재 그룹 키를 유지한 채** 각 테이블을 시간 윈도우로 집계한다. 따라서 그룹을 먼저 확정해야 "그룹마다 시간 버킷" 이 된다. 순서를 뒤집으면 전체를 한 테이블로 접은 뒤 나누게 되어 집계값이 달라진다.

`keep` 은 파이프라인 끝에 남기되 목록에 그룹 키를 더한다 — 중간에서 태그를 버리면 `group()` 이 참조할 컬럼이 사라진다.

### 4.3 그룹 태그를 응답에 싣고 클라이언트가 재파싱하지 않는다

그룹의 태그 값은 **서버가 알고 있다** — 쿼리 결과 행에 들어 있다. 클라이언트가 시리즈 이름 문자열에서 태그를 역파싱하는 방식은 태그 값에 구분자가 들어가면 즉시 깨진다. `SeriesBucket.Tags` 로 구조를 유지한 채 올린다.

이것은 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.8 이 버킷 경계에 대해 세운 원칙("경계는 서버가 소유한다")과 같은 형태다 — 서버가 아는 것을 클라이언트가 다시 계산하지 않는다.

### 4.4 상한을 그룹 수에 두고 페이지네이션을 두지 않는다

[SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) §4.5 의 판단을 승계한다. 차트 패널에서 시리즈 페이지네이션은 의미가 없다 — 사용자는 "다음 20개 라인" 을 원하는 게 아니라 **더 좁은 질문**을 원한다. 상한 + 절단 신호 + 좁히는 방법 안내가 맞는 형태다.

### 4.5 `tags` 를 없애고 `group_by` 로 통합하지 않는다

`tags: {host: 'a'}` 를 `group_by: ['host']` + 값 선택으로 대체하면 개념이 하나가 되지만, 저장된 config 전량 마이그레이션이 필요하고 §2.9 의 렌더 불변을 깬다. 두 축은 의미도 다르다(§2.1) — 필터와 분할은 직교하며, 합치면 "kr 리전 안에서 host 별로" 를 표현할 수 없다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 성능 | group by 요청 1건은 같은 시리즈를 개별 요청 N건으로 받는 것보다 느리지 않아야 한다 |
| 결정성 | 같은 config 는 같은 쿼리 문자열을 만든다(키 정렬 고정) |
| 안정성 | 그룹 순서는 폴링 간 안정적이다(태그 값 사전순) |
| 커버리지 | 신규·수정 파일 85% 이상 |
| 회귀 | `group_by` 없는 경로의 쿼리 문자열·응답·렌더 결과 무변경 |

---

## 6. 가정 및 제약

1. **v2 의 `queryFlux` 가 모든 컬럼을 통과시킨다** — `TestInfluxSeriesEnum_V2_쿼리플럭스_왕복` 이 고정하는 불변식. 깨지면 그룹 태그가 사라진다.
2. **v3 의 `iteratorToMaps` 가 태그를 행 컬럼으로 드러낸다** — [SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) §HISTORY-0.5.0 의 `GROUP BY *` 실측이 근거. `GROUP BY` 부분 지정에서도 같은지는 **M1 이 실측한다**(OQ3).
3. 한 요청은 measurement 1개 · field 1개를 유지한다.
4. `__field__` 는 그룹 사이에서 상수다.
5. v3 는 InfluxQL 이 유일한 경로다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §1.2.9 승계).

---

## 7. 열린 질문 (OQ)

| # | 질문 | 잠정 | 대안 |
|---|------|------|------|
| **OQ1** | group by 항목에서 파생된 N개 시리즈의 **색·별칭**을 어떻게 배정하는가? config 항목은 `color` 를 하나만 갖는다 | **자동 팔레트를 그룹마다 배정하고, 항목의 `color` 는 무시한다.** `stroke_style`·`stroke_width`·`smooth` 는 항목 값을 **전 그룹이 공유**한다 — 이들은 "이 항목의 선 모양" 이라는 의미가 그룹에 그대로 이어진다. 별칭은 `series_name_format` + 그룹 태그 값으로 만든다 | (a) 항목 `color` 를 기준색으로 두고 그룹마다 명도를 변주 (b) 그룹 태그 값 → 색 해시로 폴링 간 색 고정 (c) 사용자가 그룹별 색을 사후 지정 |
| **OQ2** | 한 요청의 **그룹 수 상한**을 얼마로 두는가? | **48** — 패널 전체 시리즈 상한(`STORE_SERIES_LIMIT`)과 같은 값. 한 항목이 패널 전체를 채울 수 있다는 뜻이며, 두 상한이 같으면 사용자가 외울 숫자가 하나다 | (a) 더 낮게(예: 20) 두어 실수로 인한 과다 렌더를 조기 차단 (b) [SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) 의 1,000 처럼 크게 두고 클라이언트 48 로 자름 |
| **OQ3** | v3 의 `GROUP BY` **부분 지정**(전체 `*` 가 아닌)에서도 태그가 행 컬럼으로 드러나는가? | **드러난다** — `GROUP BY *` 실측(SPEC-TSDB-003 §HISTORY-0.5.0)의 자연스러운 부분집합이다 | 드러나지 않으면 v3 는 `GROUP BY *` 로 질의한 뒤 서버에서 지정 키만 남겨 접는다. **M1 이 실측해 확정한다** |
