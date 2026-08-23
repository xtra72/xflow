# SPEC-TSDB-002 인수 조건 (v0.2.0)

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)

형식: Given-When-Then + **기계 검증 가능한 단언**(vitest/Go 테스트 파일·테스트명, 또는 실행 가능한 셸 명령과 기대 출력)

## 실행 규약

- 모든 셸 단언은 **저장소 루트**에서 실행한다(별도 표기 시 예외).
- **v0.1.0 규약 정정 — `grep -c … || true` 를 쓰지 않는다.** 그 형태는 **파일이 존재하지 않을 때 아무것도 출력하지 않는다**(grep 이 stderr 로 오류를 내고 exit 2, `|| true` 가 이를 삼킨다). v0.1.0 은 이 함정으로 미존재 파일 4건에 대해 "현재 0"을 잘못 기록했다. v0.2.0 은 전부 다음 형태를 쓴다.

  ```bash
  grep -o <패턴> <파일> 2>/dev/null | wc -l     # 항상 숫자를 출력한다. 미존재 파일 → 0
  ```

- **수치는 "일치 횟수"(occurrence)이며 "일치 줄 수"(`grep -c`)가 아니다.** 두 값이 다른 지점이 있다(예: `tsdbMode` 는 10회 / 9줄). 본 문서의 기준은 항상 횟수다.
- 각 단언에 `현재:` 값을 병기한다. 이는 **v0.2.0 작성 시점에 실제로 실행해 관측한 값**이며, 구현 후 `기대:` 값으로 바뀌어야 한다. 아래 §M 에 이번 회차에 실행한 단언 목록을 적었다.
- 주석 라인의 오탐을 피하기 위해 디스패치 단언은 `\.data_source ===` 처럼 **접근 형태**로 좁힌다. 산문 주석에는 점(`.`)이 없다.
- 리터럴에 `|` `(` `{` 등이 포함되면 `grep -oF`(고정 문자열)를 쓴다.

## 공통 픽스처

`useStoreChartData` 계열 훅은 테스트 주입 지점(`queryMatrixFn` / `resolveKeysFn` / `nowFn`)을 가지므로 네트워크 없이 매트릭스를 주입한다. 기존 `charts/__mocks__` 와 `StatPanelStoreSource.test.tsx` 의 모킹 패턴을 그대로 쓴다.

**F1 — 3컬럼 매트릭스** (`intervalMs = 60_000`, 버킷 5개, `bucketStartMs` 는 60000 의 배수)

| 컬럼 | b0 | b1 | b2 | b3 | b4 |
|------|----|----|----|----|----|
| `room.temp` | 20 | 22 | 26 | 24 | 21 |
| `room.humid` | 40 | `null` | 44 | 44 | 48 |
| `room.co2` | `null` | `null` | `null` | `null` | `null` |

**F2 — 소스별 config 3종** (v0.1.0 의 4종에서 `CFG_INFLUX` 가 사라지고 `CFG_TSDB` 가 재정의됨)

```ts
const CFG_NONE    = {};                                  // data_source 부재
const CFG_CHANNEL = { data_source: 'channel' };
const CFG_STORE   = { data_source: 'store', store_source: { agent_name: 'st', series: [{ key: 'k1' }] } };
const CFG_TSDB    = {
  data_source: 'tsdb',
  tsdb_source: {
    backend: 'influxdb',
    agent_id: 'a-1', agent_name: 'ix',
    series: [{ key: 'cpu', field: 'usage', tags: { host: 'a' } }],
    time_window_ms: 3_600_000, interval_ms: 60_000, aggregation: 'average',
  },
};
```

**F3 — InfluxDB 구조화 질의 요청 1종** (v2/v3 생성 테스트의 공통 입력. **시리즈 1개**다 — spec.md §2.6)

```json
{
  "bucket": "metrics",
  "measurement": "cpu", "field": "usage", "tags": { "host": "a" },
  "start_ms": 1700000000000, "end_ms": 1700003600000,
  "interval_ms": 60000, "aggregation": "average", "fill": ""
}
```

**F4 — 응답 엔트리 형상** (평탄 `chartQueryResponse` 재사용 확인용)

```json
{ "entries": [
    { "timestamp": 1700000000000, "value": 21.5, "labels": { "__field__": "usage", "host": "a" } },
    { "timestamp": 1700000060000, "value": 22.0, "labels": { "__field__": "usage", "host": "a" } }
  ], "count": 2, "truncated": false }
```

**F5 — 백엔드 불일치 3종** (spec.md §2.18)

| 케이스 | `tsdb_source.backend` | 해석된 에이전트 타입 | 기대 |
|--------|----------------------|---------------------|------|
| M-OK | `'influxdb'` | `influxdb` | 정상 질의 |
| M-UNSUPPORTED | `'influxdb'` | `store` | 백엔드 불일치 오류, 질의 없음 |
| M-MISSING | `'influxdb'` | (에이전트 해석 불가) | `agent_name` 폴백 → 실패 시 오류 오버레이 |

---

## A. 어휘 분리와 계약 타입 (U1 · U2)

### AC-01 — 종류 유니온이 3종이며 memTSDB 식별자가 개명된다

```gherkin
Given 계약 타입이 정의되어 있을 때
When 두 유니온과 개명 완결성을 검사하면
Then ChartDataSourceKind 는 3종이고 SeriesDataSourceKind 는 'memtsdb' 를 포함하며
And  memTSDB 를 가리키던 'tsdb' 리터럴이 남아 있지 않다
```

```bash
grep -oF "export type ChartDataSourceKind = 'channel' | 'store' | 'tsdb';" \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null | wc -l   # 기대 1 · 현재 0
grep -oF "export type SeriesDataSourceKind = 'store' | 'tsdb' | 'memtsdb';" \
  web/src/services/api/seriesDataSource.ts 2>/dev/null | wc -l                     # 기대 1 · 현재 0
# 개명 완결성: memTSDB 어댑터가 더 이상 'tsdb' 를 kind 로 쓰지 않는다
grep -oF "kind: 'tsdb'" web/src/services/api/tsdb.ts 2>/dev/null | wc -l           # 기대 0 · 현재 1
# 'memtsdb' 가 세 지점에 등장한다
grep -oF "'memtsdb'" web/src/services/api/seriesDataSource.ts \
  web/src/services/api/tsdb.ts web/src/pages/agents/AgentDetailPanel.tsx 2>/dev/null | wc -l  # 기대 3 이상 · 현재 0
# 에이전트 상세의 kind 배정이 개명된 값을 쓴다
grep -oF "agentType === 'store' ? 'store' : 'tsdb'" \
  web/src/pages/agents/AgentDetailPanel.tsx 2>/dev/null | wc -l                    # 기대 0 · 현재 1
```

- vitest: `AgentDetailPanel` 의 Series 탭 렌더가 **개명 전후 동일**함(UB1-24). 기존 테스트 무변경 통과로 갈음한다.

### AC-02 — TSDB 라벨이 외부 시계열 DB 를 가리키고 placeholder 문구가 은퇴한다

```gherkin
Given 데이터 소스 토글이 3종을 노출할 때
When ko/en 로케일을 검사하면
Then TSDB 라벨이 존재하고, placeholder 전용 문구는 사라졌으며, 백엔드 불일치 문구가 추가되었다
```

```bash
grep -oF '"dataSourceTsdb"' web/src/lib/i18n/ko.json 2>/dev/null | wc -l                    # 기대 1 · 현재 1
grep -oF '"dataSourceTsdbTitle"' web/src/lib/i18n/ko.json 2>/dev/null | wc -l               # 기대 0 · 현재 1 (은퇴)
grep -oF '"dataSourceTsdbBody"' web/src/lib/i18n/ko.json 2>/dev/null | wc -l                # 기대 0 · 현재 1 (은퇴)
grep -oF '"dataSourceTsdbBackendMismatch"' web/src/lib/i18n/ko.json 2>/dev/null | wc -l      # 기대 1 · 현재 0
grep -oF '"dataSourceTsdbBackendMismatch"' web/src/lib/i18n/en.json 2>/dev/null | wc -l      # 기대 1 · 현재 0
```

### AC-03 — TSDB 소스 config 가 **단일 블록 + 백엔드 판별자**다 (OQ1)

```gherkin
Given spec.md §2.2 의 형상을 구현했을 때
When 타입 정의를 검사하면
Then TsdbSourceConfig 하나만 존재하고 backend 판별자를 가지며
And  백엔드별로 쪼갠 블록(InfluxSourceConfig / influx_source)이 존재하지 않는다
```

```bash
grep -oE '^export interface (TsdbSourceConfig|TsdbSeriesRef)' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null | wc -l   # 기대 2 · 현재 0
grep -oF 'export type TsdbBackend' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null | wc -l   # 기대 1 · 현재 0
grep -oF 'backend: TsdbBackend' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null | wc -l   # 기대 1 · 현재 0
# 백엔드별 블록 분리 금지 (UB1-2)
grep -roF 'InfluxSourceConfig' web/src 2>/dev/null | wc -l                          # 기대 0 · 현재 0
grep -roF 'influx_source' web/src 2>/dev/null | wc -l                               # 기대 0 · 현재 0
```

> 이 AC 는 **v0.1.0 AC-03 의 반전**이다. v0.1.0 은 두 블록의 **분리 존재**를 요구했다.

### AC-04 — `TsdbSourceConfig` 에 에이전트 참조가 **필수로 존재**한다 (OQ2)

```gherkin
Given 외부 TSDB 는 어느 DB에 연결할지 알아야 하므로
When TsdbSourceConfig 블록을 검사하면
Then agent_id(정본)와 agent_name(폴백)이 모두 존재한다
```

```bash
awk '/^export interface TsdbSourceConfig/,/^}/' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null \
  | grep -oF 'agent_name' | wc -l                # 기대 1 · 현재 0
awk '/^export interface TsdbSourceConfig/,/^}/' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null \
  | grep -oF 'agent_id' | wc -l                  # 기대 1 · 현재 0
```

- vitest: `panelDataSource.test.ts` — `'tsdb + agent_name 빈 문자열 → tsdb/inactive'`

> 이 AC 는 **v0.1.0 AC-04 의 반전**이다. v0.1.0 은 두 필드의 **부재**를 요구했다.

---

## B. 디스패치 일반화 (U3)

### AC-05 — 소스 판정 순수 모듈이 존재하고 3종 × 활성조건을 전수 커버한다

```gherkin
Given F2 의 config 3종과 비활성 변형이 주어졌을 때
When resolvePanelSourceBinding 을 호출하면
Then spec.md §2.3 의 표와 일치하는 kind/active 를 반환한다
```

- vitest: `web/src/pages/dashboard/panels/charts/panelDataSource.test.ts`
  - `'data_source 부재 → channel/active'`
  - `'store + series 0 → store/inactive'`
  - `'store + series N → store/active'`
  - `'store + selection_mode:tag + tag_filters N → store/active'`
  - `'tsdb + tsdb_source 부재 → tsdb/inactive'`
  - `'tsdb + agent_name 빈 문자열 → tsdb/inactive'`
  - `'tsdb + agent_name + series 0 → tsdb/inactive'`
  - `'tsdb + agent_name + series N → tsdb/active'`
  - `'인식 불가 문자열 → channel/active + unknownKind'`

```bash
test -f web/src/pages/dashboard/panels/charts/panelDataSource.ts && echo present || echo absent   # 기대 present · 현재 absent
```

### AC-06 — 5종 차트 패널에서 하드코딩 동등 비교가 사라진다

```bash
grep -ohE "\.data_source === '(store|tsdb)'" \
  web/src/pages/dashboard/panels/charts/{LineChartPanel,StatPanel,BarChartPanel,PieChartPanel,TablePanel}.tsx \
  2>/dev/null | wc -l                                    # 기대 0 · 현재 5
grep -lE 'resolvePanelSourceBinding|usePanelSeriesData' \
  web/src/pages/dashboard/panels/charts/{LineChartPanel,StatPanel,BarChartPanel,PieChartPanel,TablePanel}.tsx \
  2>/dev/null | wc -l                                    # 기대 5 · 현재 0
```

### AC-07 — 히트맵과 게이지(이형 2종)도 일반화된다

```bash
grep -ohE "\.data_source === '(store|tsdb)'" \
  web/src/pages/dashboard/panels/heatmap/HeatmapPanel.tsx 2>/dev/null | wc -l          # 기대 0 · 현재 1
grep -ohE "flags\.dataSource === '(store|tsdb)'" \
  web/src/pages/dashboard/panels/charts/gaugeLegacyBinding.ts 2>/dev/null | wc -l      # 기대 0 · 현재 2
# 게이지의 series_reduce 논리곱은 gaugeLegacyBinding 에 남아야 한다
grep -oF 'hasSeriesReduce' \
  web/src/pages/dashboard/panels/charts/gaugeLegacyBinding.ts 2>/dev/null | wc -l      # 기대 1 이상 · 현재 3
grep -oF 'hasSeriesReduce' \
  web/src/pages/dashboard/panels/charts/panelDataSource.ts 2>/dev/null | wc -l         # 기대 0 · 현재 0(파일 없음)
```

### AC-08 — 미리보기 2곳과 범례 1곳도 일반화된다

```bash
grep -ohE "\.data_source === '(store|tsdb)'" \
  web/src/pages/dashboard/PanelSettingsDialog.tsx 2>/dev/null | wc -l                  # 기대 0 · 현재 2
grep -ohE "dataSource === '(store|tsdb)'" \
  web/src/pages/dashboard/panels/charts/previewSeries.ts 2>/dev/null | wc -l           # 기대 0 · 현재 1
```

---

## C. 하위 호환 특성화 (U4)

> plan.md §3 의 CT-01 ~ CT-21 이 이 절의 구현체다. 모두 **M3 수정 전 코드에서 GREEN** 이어야 하며, M3 이후에도 GREEN 을 유지한다.

### AC-09 — 저장된 config 3형태의 렌더 결과가 불변이다

- vitest (CT-01 ~ CT-05): `charts/{LineChartPanel,StatPanelStoreSource,BarChartPanel,PieChartPanel,TablePanel}.test.tsx`
  - `'config 부재 → 채널 경로'` / `'data_source:channel → 채널 경로'` / `'data_source:store + store_source 부재 → 채널 경로'` / `'data_source:store + series 0 → 채널 경로'` / `'data_source:store + series N → store 경로'`

### AC-10 — 히트맵은 자기 config 키를 계속 사용한다

- vitest (CT-06 ~ CT-08): `heatmap/HeatmapPanel.test.tsx` — `'store_source 만 있고 히트맵 키가 비면 비활성'`

### AC-11 — 게이지 판정 진리표 4행이 불변이다

- vitest (CT-09 ~ CT-12): `charts/gaugeLegacyBinding.test.ts` — 기존 진리표 테스트 4건 무변경 통과

### AC-12 — 미리보기 범례의 채널 폴백이 보존된다

- vitest (CT-16 ~ CT-18): `charts/previewSeries.test.ts` — `'store + 시리즈 0 + 채널 M → 채널 범례 M개'`

---

## D. (은퇴) memTSDB 어댑터

**AC-13 ~ AC-16 은 v0.2.0 에서 은퇴했다.** 번호는 재사용하지 않는다.

| 은퇴 AC | v0.1.0 내용 | 은퇴 사유 |
|---------|-------------|-----------|
| AC-13 | `void signal;` 제거, `AbortSignal` 전달 (G2) | memTSDB 는 패널 소스가 아니다(spec.md §1.2.1). 취소 전파 요구는 **TSDB 어댑터 축으로 이전**되어 AC-52 가 승계 |
| AC-14 | `Promise.all` → `allSettled` 부분 실패 격리 (G3) | 상동. **AC-52 가 승계** |
| AC-15 | `seriesFilters.fieldName` → 백엔드 `field` 전달 (G1) | memTSDB 경로의 기존 부채로 남는다(spec.md §1.3). TSDB 축에서는 `field` 가 애초에 필수이므로(§2.2) 같은 결함이 발생할 수 없다 |
| AC-16 | `seriesFilters.tags` 모순 경고 | 상동. memTSDB 의 키 인코딩 특성에서 유래한 요구이며 TSDB 축에 대응물이 없다 |

> 은퇴한 것은 **본 SPEC 의 책임**이지 결함의 존재가 아니다. `web/src/services/api/tsdb.ts` 의 세 결함은 그대로 남아 있으며, 그 경로(`/api/v1/tsdb/*`)가 현재 미등록이라 도달 불가라는 사실이 spec.md §1.2.1 · §7 NQ4 에 기록되어 있다.

---

## E. InfluxDB 구조화 질의 (U6 · U7 · U8 · U9)

### AC-17 — 구조화 엔드포인트가 등록되고 **신규 응답 DTO 를 만들지 않는다**

```gherkin
Given InfluxDB 구조화 질의를 구현했을 때
When 라우트 등록과 응답 타입을 검사하면
Then POST /influxdb/{agent_name}/series/query 가 store.read 로 등록되고
And  응답은 기존 chartQueryResponse 를 재사용하며 신규 응답 타입이 0개다
```

```bash
grep -rhoF '"/influxdb/{agent_name}/series/query"' internal/ 2>/dev/null | wc -l    # 기대 1 이상 · 현재 0
grep -rhoF 'series/query", "store.read"' internal/ 2>/dev/null | wc -l              # 기대 1 · 현재 0
# 신규 응답 타입 금지 (UB1-23) — v0.1.0 이 계획했던 두 타입이 존재해서는 안 된다
grep -rhoE 'influxSeriesQueryResponse|influxSeriesMatrixRow' internal/ 2>/dev/null | wc -l  # 기대 0 · 현재 0
# 기존 평탄 응답 재사용
grep -hoF 'chartQueryResponse' internal/api/handler/influxdb_series.go 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0(파일 없음)
# 기존 원문 통과 경로는 무변경 (UB1-15)
grep -coF 'g.POSTPerm("/influxdb/{agent_name}/query", "store.read", h.Query)' \
  internal/api/handler/influxdb_query.go                                            # 기대 1 · 현재 1
```

### AC-18 — 엔트리가 Store 와 동일한 라벨 규약을 따르고 클라이언트가 컬럼 자리를 보존한다

```gherkin
Given 3개 시리즈를 조회하고 그중 1개는 그 구간에 데이터가 없을 때
When 어댑터가 매트릭스를 조립하면
Then columns 길이는 3이고 순서는 요청과 같으며 데이터 없는 시리즈도 자리를 유지한다
```

- Go: `internal/api/handler/influxdb_series_test.go`
  - `TestInfluxSeriesQuery_EntryLabelsUseFieldReservedKey` — F4 형상. `labels.__field__` = 요청 `field`, 나머지 키는 요청 `tags`
  - `TestInfluxSeriesQuery_EmptyResultReturnsEmptyEntries` — 데이터 없으면 `entries: []`, 오류 아님
- vitest: `web/src/services/api/tsdbSource.test.ts`
  - `'3개 시리즈 중 1개가 0행이어도 컬럼 3개가 유지된다'` (UB1-18)
  - `'컬럼 순서가 요청 시리즈 순서와 같다'`

```bash
# 예약 라벨 규약이 Store 와 공유된다
grep -hoF "METRIC_LABEL_KEY = '__field__'" web/src/services/api/seriesLabels.ts 2>/dev/null | wc -l  # 기대 1 · 현재 1
```

### AC-19 — 요청 검증 7종이 400 을 반환한다

```gherkin
Given 잘못된 요청 7종이 주어졌을 때
When 구조화 질의를 호출하면
Then 400 을 반환하고 원인 축을 메시지에 포함한다
```

- Go: `TestInfluxSeriesQuery_Validation` — 표 구동 7케이스
  `measurement 빈 문자열` · `field 빈 문자열` · `interval_ms <= 0` · `end_ms <= start_ms` · `aggregation 미지의 값` · `fill:'avg'` · `이스케이프 불가 식별자`

> v0.1.0 의 8케이스 중 `series 빈 배열` 이 사라졌다 — 요청이 시리즈 배열이 아니라 시리즈 1개를 담기 때문이다(spec.md §2.6).

### AC-20 — 에이전트 오류 매핑이 기존 규약을 따른다

- Go: `TestInfluxSeriesQuery_AgentNotFound`(404) · `TestInfluxSeriesQuery_NotAnInfluxAgent`(400) · `TestInfluxSeriesQuery_Timeout`(408)

### AC-21 — Flux 생성 계층이 존재한다

```bash
grep -rhoF 'aggregateWindow' --include='*.go' . 2>/dev/null | wc -l    # 기대 1 이상 · 현재 0 (트리 전체)
```

- Go: `internal/agent/system/influxdb_seriesquery_test.go` — `TestBuildFluxSeriesQuery_Golden` (F3 골든 문자열 대조)

### AC-22 — InfluxQL 생성 계층이 존재한다

```bash
grep -rhoF 'GROUP BY time(' internal/agent/system/ 2>/dev/null | wc -l          # 기대 1 이상 · 현재 0
grep -rhoE 'GROUP BY time\([^)]*,' internal/agent/system/ 2>/dev/null | wc -l   # 기대 0 (offset 미지정) · 현재 0
```

- Go: `TestBuildInfluxQLSeriesQuery_Golden`

### AC-23 — 식별자 이스케이프가 강제된다

- Go: `TestBuildFluxSeriesQuery_Escaping` · `TestBuildInfluxQLSeriesQuery_Escaping` — 표 구동 (`"` · `\` · `\n` · `;` · 유니코드)

### AC-24 — `fill` 매핑 5종

- Go: `TestSeriesQueryFillMapping` — (v2/v3) × 5종 = 10 케이스. `'avg'` 는 양쪽 모두 오류

### AC-25 — 집계 어휘 매핑 5종 전수 (§4.6)

```bash
grep -rhoF '"mean"' internal/agent/system/ 2>/dev/null | wc -l      # 기대 1 이상 · 현재 0
grep -rhoF '"MEAN"' internal/agent/system/ 2>/dev/null | wc -l      # 기대 1 이상 · 현재 0
grep -rhoF '"average"' internal/agent/system/ 2>/dev/null | wc -l   # 기대 0 · 현재 0
grep -rhoF '"AVG"' internal/agent/system/ 2>/dev/null | wc -l       # 기대 0 · 현재 0
```

- Go: `TestSeriesQueryAggregationMapping` — 5종 × (v2/v3) = 10 케이스 + `TestSeriesQueryAggregationMapping_UnknownRejected`

### AC-26 — 버킷 레이블이 **시작** 시각이다 (§2.8 · UB1-7)

```bash
grep -rhoF 'timeSrc: "_start"' internal/agent/system/ 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0
grep -rhoF 'timeSrc: "_stop"'  internal/agent/system/ 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

- Go: `TestBuildFluxSeriesQuery_TimeSrcIsStart` · `TestInfluxSeriesEntry_TimestampIsBucketStart`
- Go: `TestBucketAlignment_GeneratedQueriesRequestEpochAlignment` — 생성된 Flux 에 `timeSrc: "_start"` 가 있고 `offset:` 이 없음을, 생성된 InfluxQL 의 `GROUP BY time(...)` 에 offset 인자가 없음을 인터벌 8종에서 단언한다

### AC-27 — Store ↔ InfluxDB 버킷 경계 교차 검증 (**백엔드별 축 분리 — M7.3 개정**)

```gherkin
Given 동일한 타임스탬프 집합과 인터벌이 주어졌을 때
When Store / InfluxDB v2 / InfluxDB v3 의 버킷 시작을 비교하면
Then divisor 인터벌(1s·10s·30s·60s·300s·900s·3600s)에서 3소스가 일치한다
And  non-divisor 인터벌(예: 420s)에서도 3소스가 **일치한다**
```

**3소스 단언의 구성이 M7.3 에서 개정되었다.** 개정 전에는 v2 와 v3 의 버킷 시작을 `influxBucketStartsMs` 헬퍼 한 벌로 접어 `system.SeriesBucketStartMs` 를 **공유 호출**했다 — 즉 "3소스 비교"가 실제로는 "Store vs 공통 정규화" **2소스 비교**였다. 개정 후에는 각 백엔드가 **자기 방언으로 생성한 쿼리 문자열**에서 윈도우 파라미터(폭 · offset · 레이블 위치)를 읽어 낸다.

| 축 | 도출 경로 |
|----|-----------|
| Store | `store_query.go` 의 epoch-zero 정렬 (정본, 무변경) |
| InfluxDB v2 | `BuildFluxSeriesQuery` 가 생성한 Flux `aggregateWindow` 인자 |
| InfluxDB v3 | `BuildInfluxQLSeriesQuery` 가 생성한 InfluxQL `GROUP BY time()` 인자 |

- Go: `internal/api/handler/bucket_alignment_crosscheck_test.go`
  - `TestBucketAlignment_DivisorIntervals_AllPanelSourcesAgree`
  - `TestBucketAlignment_NonDivisorInterval_AllPanelSourcesAgree` — **420초에서도 일치**를 단언한다
  - `TestBucketAlignment_PerBackendWindowParameters` — **신규(M7.3)**. 각 백엔드의 윈도우 폭 · offset · 레이블 위치를 개별 고정해, 교차 검증 실패 시 "폭이 틀렸는가 · 원점이 밀렸는가 · 레이블이 끝인가"를 갈라 준다
  - `TestBucketAlignment_SharedRuntimeNormalizationMatchesStore` — **신규(M7.3)**. 런타임 수렴 지점(`normalizeSeriesBuckets` 가 v2 · v3 양쪽에서 부르는 `system.SeriesBucketStartMs`)이 Store 와 갈라지지 않음을 별도로 고정한다. **이 단언은 3소스 단언을 대체하지 않는다**

> **런타임 수렴 사실은 남는다.** `normalizeSeriesBuckets`(`influxdb_agent.go`)가 두 방언 모두 `SeriesBucketStartMs` 를 통과시키므로, 분리 가능한 지점은 정규화가 아니라 **질의 생성**이다. 축 분리의 실효는 변이 검증 3건으로 입증했다 — v2 전용 결함(`timeSrc: "_start"`→`"_stop"`, `every: interval`→`interval*2`)은 v2 축만, v3 전용 결함(`GROUP BY time(%s)`→`time(%s,7000ms)`)은 v3 축만 실패시킨다. 특히 `every: interval*2` 변이는 **개정 전 테스트 파일 전체가 통과시켰다**(spec.md §HISTORY-0.5.0 (4)).

```bash
# Store 의 epoch-zero 정렬이 정본이며 무변경이다
grep -hoF 'bucketStartMs := (tsMs / intervalMs) * intervalMs' \
  internal/api/handler/store_query.go 2>/dev/null | wc -l    # 기대 1 · 현재 1
```

> **v0.1.0 대비 반전.** v0.1.0 AC-27 은 `TestBucketAlignment_NonDivisorInterval_MemTSDBDiverges` 로 "420초에서 memTSDB 가 Store 와 **어긋난다**"를 단언했다. memTSDB 가 패널 소스에서 빠지면서 비교 대상이 사라졌고, 남은 두 소스는 모두 epoch-zero 정렬이므로 **모든 인터벌에서 일치**한다. `internal/tsdb/query.go` 의 `time.Truncate` 는 범위 밖이며 무변경이다(현재 2건, 변경 감지용 참고값).

### AC-28 — 버킷 수 가드(서버) · 시리즈 수 가드(클라이언트)

```bash
# Store 와 같은 상수를 쓰는 파일이 2개 이상이 된다(store_query.go + influxdb_series.go)
grep -rl 'maxAggregationBuckets' internal/api/handler/ 2>/dev/null | grep -v _test | wc -l   # 기대 2 이상 · 현재 1
```

- Go: `TestInfluxSeriesQuery_BucketLimit` — 100,000 초과 시 400 + 메시지에 축·실제값·상한 포함
- vitest: `PanelSettingsDataSource.test.tsx` 또는 `TsdbSourceSection.test.tsx` — `'시리즈 48개 초과 선택이 차단된다'`

> v0.1.0 의 서버측 시리즈 수 가드(`TestInfluxSeriesQuery_SeriesLimit`)는 은퇴했다 — 요청 1건이 시리즈 1개이므로 서버가 셀 대상이 없다(spec.md §2.9).

---

## F. 스키마 디스커버리 (U10)

### AC-29 — 3종 신규 라우트가 등록된다

```bash
grep -rhoF '"/influxdb/{agent_name}/tag-keys"'   internal/api/handler/ 2>/dev/null | wc -l   # 기대 1 · 현재 0
grep -rhoF '"/influxdb/{agent_name}/tag-values"' internal/api/handler/ 2>/dev/null | wc -l   # 기대 1 · 현재 0
grep -rhoF '"/influxdb/{agent_name}/field-keys"' internal/api/handler/ 2>/dev/null | wc -l   # 기대 1 · 현재 0
```

### AC-30 — 필드 키 디스커버리는 신규 구현이다 (D4)

```bash
grep -rhoF 'measurementFieldKeys' internal/agent/system/ 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0
grep -rhoF 'SHOW FIELD KEYS'      internal/agent/system/ 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0
grep -rhoF 'ListFieldKeys'        internal/agent/system/ 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0
```

- Go: `TestInfluxSchema_ListFieldKeys_V2` · `TestInfluxSchema_ListFieldKeys_V3`

### AC-31 — API 계층이 `internal/migrate/tsdbtags` 를 import 하지 않는다 (§4.5)

```bash
grep -rhoF 'migrate/tsdbtags' internal/api/ internal/agent/ 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

### AC-32 — v3 의 measurements 만 501 이 해제되고 나머지 5종은 유지된다

```bash
grep -choE 'return (nil, |BucketInfo\{\}, )?ErrManagementNotSupported' \
  internal/agent/system/influxdb_v3.go                             # 기대 5 · 현재 6
awk '/func \(c \*influxV3Client\) ListMeasurements/,/^}/' internal/agent/system/influxdb_v3.go \
  | grep -coF 'ErrManagementNotSupported'                          # 기대 0 · 현재 1
```

- Go: `internal/agent/system/influxdb_management_test.go` — measurements 기대값만 갱신, 나머지 5건 무변경
- Go: `internal/api/handler/influxdb_management_test.go` — 동일

### AC-33 — 디스커버리 응답을 캐시하지 않는다 (UB1-12)

- Go: `TestInfluxSchema_NoCache` — mock 클라이언트가 2회 호출됨을 단언

---

## G. 토글 영속과 AC-05 반전 (E1 · E2)

### AC-34 — TSDB 선택이 config 에 기록된다

```bash
# 로컬 tsdbMode 상태가 제거되었다
grep -oF 'tsdbMode' web/src/pages/dashboard/ChartPanelSections.tsx 2>/dev/null | wc -l   # 기대 0 · 현재 10
# placeholder 가 제거되었다
grep -rhoF 'chart-data-source-tsdb-placeholder' web/src --include='*.tsx' 2>/dev/null | wc -l  # 기대 0 · 현재 2
```

- vitest: `PanelSettingsDataSource.test.tsx`
  - `'TSDB 선택 시 data_source:tsdb 가 config 에 기록된다'`
  - `'처음 TSDB 선택 시 defaultTsdbSource() 가 함께 기록된다'`

> v0.1.0 의 `'InfluxDB 선택 시 data_source:influxdb 가 config 에 기록된다'` 는 은퇴했다 — InfluxDB 는 별도 종류가 아니라 TSDB 의 백엔드다.

### AC-35 — AC-05 테스트가 **삭제되지 않고 제자리 반전**된다 (UB1-13)

```bash
# 해당 it() 블록이 여전히 존재한다(삭제 금지)
grep -coF "it('TSDB 선택 시" web/src/pages/dashboard/PanelSettingsDataSource.test.tsx   # 기대 1 · 현재 1

# 그 블록 안의 not.toHaveBeenCalled 단언이 사라졌다
awk "/it\('TSDB 선택 시/,/^  \}\);/" web/src/pages/dashboard/PanelSettingsDataSource.test.tsx \
  | grep -coF 'not.toHaveBeenCalled'                                                     # 기대 0 · 현재 1

# 무관한 두 테스트는 보존된다 — 파일 전체 카운트가 3에서 2로만 줄어야 한다
grep -coF 'not.toHaveBeenCalled' web/src/pages/dashboard/PanelSettingsDataSource.test.tsx  # 기대 2 · 현재 3

# 반전 사유 주석 sentinel
grep -coF 'SPEC-TSDB-002 §2.11 반전' web/src/pages/dashboard/PanelSettingsDataSource.test.tsx  # 기대 1 · 현재 0
```

> `expect(onConfigChange).not.toHaveBeenCalled()` 는 이 파일에 3건(`:138` · `:281` · `:991`) 존재한다(재확인함). **반전 대상은 `:138` 하나뿐**이므로 블록 한정 `awk` 단언과 전체 카운트 단언을 **둘 다** 둔다. 전체 카운트만 보면 무관한 테스트가 함께 지워져도 통과한다.

### AC-36 — 소스 전환이 비파괴다 (E2)

- vitest: `ChartPanelSections.test.tsx` 또는 `StoreSourceSection.test.tsx`
  - `'store → tsdb 전환 시 store_source 가 보존된다'`
  - `'tsdb → store 왕복 후 두 블록이 모두 보존된다'`

---

## H. 능력 게이팅과 상태 (S1 · S2)

### AC-37 — 소스별 능력 표가 컴파일 시 전수성을 갖는다

```bash
grep -oF 'Record<ChartDataSourceKind' \
  web/src/pages/dashboard/panels/charts/panelDataSource.ts 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0(파일 없음)
```

- vitest: `panelDataSource.test.ts` — `'능력 표가 3종 전부를 갖는다'`, `'store 는 fill:null/zero/previous 를 지원하지 않는다'`, `'tsdb 는 fill:avg 를 지원하지 않는다'`, `'tsdb 는 에이전트 선택이 필수다'`

### AC-38 — 미지원 설정은 숨기지 않고 비활성 + 사유를 제공한다

- vitest: `StoreSourceSection.test.tsx` / `TsdbSourceSection.test.tsx` — `'미지원 설정은 숨기지 않고 aria-disabled 로 표시한다'`

### AC-39 — 네 가지 상태가 구분 가능하다

- vitest: `charts/usePanelSeriesData.test.ts` + 대표 패널 1종
  - `'시리즈 0개 → 빈 선택 안내'` / `'에이전트 미선택 → 빈 선택 안내'`
  - `'시리즈 N개 + 매트릭스 0행 → 빈 차트(오류 아님)'`
  - `'일부 컬럼 실패 → 성공 시리즈 렌더 + 실패 개수 배지'`
  - `'조회 전체 실패 → 오류 오버레이 + 마지막 성공 렌더 보존'`

### AC-40 — 부분 실패가 복구되면 배지가 사라진다

- vitest: `charts/usePanelSeriesData.test.ts` — `'부분 실패 복구 시 배지가 제거된다'`

---

## I. 금지 동작 (UB1)

### AC-41 — 디스패치 지점에 두 번째 동등 비교가 추가되지 않는다 (UB1-1)

```bash
grep -rhoE "data_source === 'tsdb'|dataSource === 'tsdb'" \
  web/src/pages/dashboard --include='*.tsx' --include='*.ts' 2>/dev/null \
  | wc -l                                                          # 기대 0 · 현재 0
```

> 이 단언은 `.test.` 파일과 `panelDataSource.ts` 를 포함한 전수 스캔이다. 판정의 정본인 `panelDataSource.ts` 는 **`switch` 로 종류를 다루되 `=== 'tsdb'` 형태의 동등 비교를 쓰지 않는다**는 것이 요구사항이므로 예외를 두지 않는다. 테스트 파일에서 문자열 리터럴이 필요하면 config 객체 안(`data_source: 'tsdb'`)에 두며, 이는 위 패턴에 걸리지 않는다.

### AC-42 — 저장된 `data_source:'store'` config 의 렌더 결과가 불변이다 (UB1-14)

AC-09 ~ AC-12 (특성화 CT-01 ~ CT-21) 전량 GREEN 으로 갈음한다.

### AC-43 — 기존 InfluxDB 계약이 변경되지 않는다 (UB1-15 · UB1-16)

```bash
grep -coF 'QueryLanguage string' internal/api/handler/influxdb_query.go                       # 기대 1 · 현재 1
grep -coF 'lang != "flux" && lang != "influxql"' internal/api/handler/influxdb_query.go       # 기대 1 · 현재 1
grep -choE 'g\.(GET|POST|DELETE)Perm\("/influxdb/\{agent_name\}/buckets' \
  internal/api/handler/influxdb_management.go                                                 # 기대 4 · 현재 4
```

### AC-44 — 클라이언트가 버킷 **경계를 계산**하지 않는다 (UB1-10)

```bash
grep -rhoE '/ *intervalMs|Math\.floor\(.*interval' \
  web/src/services/api/tsdbSource.ts 2>/dev/null | wc -l          # 기대 0 · 현재 0(파일 없음)
```

> 버킷 **합집합 수집**(`allBuckets`)은 경계 계산이 아니므로 금지 대상이 아니다(spec.md §2.8 · UB1-10). AC-53 이 그 재사용을 별도로 검증한다.

### AC-45 — 사장 코드 `hooks/useTsdb.ts` 를 기반으로 삼지 않는다 (UB1-20)

```bash
grep -rhoE "from .@/hooks/useTsdb.|from ./useTsdb." web/src \
  --include='*.ts' --include='*.tsx' 2>/dev/null | wc -l          # 기대 0 · 현재 0
```

### AC-46 — 정적 검사 무결

```bash
cd web && npx tsc --noEmit                # 기대 exit 0 · 현재 exit 0 (v0.2.0 회차에 실행함)
cd web && npx eslint src --max-warnings 0 # 기대 exit 0
go build ./...                            # 기대 exit 0 · 현재 exit 0 (v0.2.0 회차에 실행함)
go vet ./...                              # 기대 exit 0 · 현재 exit 0 (v0.2.0 회차에 실행함)
go test ./...                             # 기대 exit 0
cd web && npm test                        # 기대 exit 0
```

---

## J. 상태 정합 (UB2)

### AC-47 — 소스 블록 부재와 인식 불가 값이 다르게 처리된다

- vitest: `panelDataSource.test.ts` + 대표 패널 1종
  - `'tsdb + tsdb_source 부재 → tsdb/inactive (channel 폴백 아님)'`
  - `'인식 불가 문자열 → channel/active + unknownKind 플래그'`

### AC-48 — 에이전트 소실이 무한 재시도를 유발하지 않는다

- vitest: `charts/useTsdbChartData.test.ts` — `'404 수신 시 마지막 성공 렌더를 보존하고 폴링 주기를 초과 호출하지 않는다'`

---

## K. 백엔드 파생과 재사용 (U11 · U12 · UB1-21~25)

### AC-50 — 백엔드는 **에이전트 타입**에서 파생되며 config `backend` 로 라우팅하지 않는다 (§2.18 · UB1-22)

```gherkin
Given F5 의 3케이스가 주어졌을 때
When 어댑터가 백엔드를 결정하면
Then 에이전트의 실제 타입이 정본이고, config.backend 는 대조군으로만 쓰인다
```

```bash
test -f web/src/services/api/tsdbSource.ts && echo present || echo absent            # 기대 present · 현재 absent
# 라우팅이 에이전트 타입으로 분기한다
grep -hoE "case 'influxdb'|=== 'influxdb'" \
  web/src/services/api/tsdbSource.ts 2>/dev/null | wc -l                             # 기대 1 이상 · 현재 0
# 라우팅이 config 의 backend 를 읽지 않는다
grep -hoE '(tsdb_source|source)\.backend' \
  web/src/services/api/tsdbSource.ts 2>/dev/null | wc -l                             # 기대 0 · 현재 0
```

- vitest: `web/src/services/api/tsdbSource.test.ts`
  - `'M-OK: 에이전트 타입 influxdb → InfluxDB 어댑터'`
  - `'M-UNSUPPORTED: 에이전트 타입 store → 백엔드 불일치 오류, 질의 없음'` (UB2-7)
  - `'M-MISSING: agent_id 해석 실패 → agent_name 폴백'` (UB2-3)
  - `'config.backend 와 에이전트 타입이 어긋나면 에이전트 타입이 이긴다'`

> 선례 대조 — 쓰기 경로의 `newStorageBackend`(`internal/node/storage_write.go:190`)가 같은 축을 쓴다. 그 함수와 두 백엔드 구현의 존재를 참고값으로 고정한다.
> ```bash
> grep -hoF 'func newStorageBackend' internal/node/storage_write.go | wc -l                              # 현재 1
> grep -hoF 'func (b *storeBackend) backendName() string { return "store" }' \
>   internal/node/storage_backend_store.go | wc -l                                                        # 현재 1
> grep -hoF 'func (b *influxdbBackend) backendName() string { return "influxdb" }' \
>   internal/node/storage_backend_influxdb.go | wc -l                                                     # 현재 1
> ```

### AC-51 — memTSDB 가 패널 경로에 배선되지 않는다 (UB1-21)

```gherkin
Given memTSDB 는 플로우 노드·WS 구독자용 내부 설비일 때
When 대시보드 패널 경로를 검사하면
Then memTSDB API·어댑터·라우트를 어디에서도 참조하지 않는다
```

```bash
grep -rhoF "services/api/tsdb'" web/src/pages/dashboard 2>/dev/null | wc -l    # 기대 0 · 현재 0
grep -rhoF 'tsdbSeriesDataSource' web/src/pages/dashboard 2>/dev/null | wc -l  # 기대 0 · 현재 0
grep -rhoF "'/tsdb/" web/src/pages/dashboard 2>/dev/null | wc -l               # 기대 0 · 현재 0
```

참고값(범위 밖, 변경 감지용) — 이 둘이 바뀌면 spec.md §1.2.1 · §7 NQ4 를 재검토한다.

```bash
grep -rl 'NewTSDBHandler' --include='*.go' . 2>/dev/null | grep -v _test \
  | grep -v 'internal/api/handler/tsdb.go' | wc -l    # 현재 0 (프로덕션 호출자 없음)
grep -hoF 'tsdbHandler' cmd/xflowd/main.go 2>/dev/null | wc -l   # 현재 0 (라우트 미등록)
```

### AC-52 — 취소 전파와 부분 실패 격리 (U12 · UB1-5 · UB1-6)

```gherkin
Given F1 의 3컬럼 중 room.humid 요청만 실패할 때
When queryTsdbMatrix 를 호출하면
Then room.temp / room.co2 컬럼은 정상이고 room.humid 는 전 버킷 null 이며 실패 인덱스가 보고된다
And  폴링 중 config 가 바뀌면 이전 요청이 취소되고 그 결과는 상태에 반영되지 않는다
```

```bash
grep -hoF 'Promise.allSettled' web/src/services/api/tsdbSource.ts 2>/dev/null | wc -l   # 기대 1 이상 · 현재 0(파일 없음)
```

- vitest: `web/src/services/api/tsdbSource.test.ts`
  - `'1개 시리즈 실패 시 나머지 컬럼은 보존되고 실패 인덱스를 보고한다'`
  - `'AbortSignal 로 취소된 요청의 응답은 폐기된다'`

> AC-13 · AC-14(은퇴)가 memTSDB 축에서 요구하던 것을 TSDB 축이 승계한 항목이다.
> **Store 는 여전히 `Promise.all` 이며 본 SPEC 이 바꾸지 않는다**(spec.md §1.3 · §7 NQ3). 참고값:
> ```bash
> grep -hoF 'Promise.all(' web/src/services/api/store.ts | wc -l   # 기대 1(무변경) · 현재 1
> ```

### AC-53 — 피벗 기계가 1벌만 존재한다 (UB1-25)

```gherkin
Given 버킷 합집합 피벗과 컬럼 자리 보존 로직이 store.ts 에 있을 때
When TSDB 어댑터를 추가하면
Then 그 로직을 공용 모듈로 추출해 재사용하고 TSDB 전용 사본을 만들지 않는다
```

```bash
test -f web/src/services/api/seriesMatrixPivot.ts && echo present || echo absent   # 기대 present · 현재 absent
grep -hoF 'allBuckets' web/src/services/api/seriesMatrixPivot.ts 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
grep -hoF 'allBuckets' web/src/services/api/store.ts 2>/dev/null | wc -l              # 기대 0(공용 모듈로 이관) · 현재 3
grep -hoF 'allBuckets' web/src/services/api/tsdbSource.ts 2>/dev/null | wc -l         # 기대 0(사본 금지) · 현재 0
grep -hoF 'allBuckets' web/src/services/api/tsdb.ts 2>/dev/null | wc -l               # 기대 3(범위 밖, 무변경) · 현재 3
# 그룹화 함수가 재사용 가능하도록 export 된다
grep -hoE '^export function groupEntriesBySeries' web/src/services/api/store.ts 2>/dev/null | wc -l  # 기대 1 · 현재 0
```

- vitest: 기존 `web/src/services/api/store.test.ts` 전량 GREEN — 공용화가 **무동작 리팩터**임을 잠근다(plan.md M6.2)

### AC-54 — 백엔드 불일치가 화면에 드러난다 (UB2-7)

```gherkin
Given tsdb_source 가 store 에이전트를 가리킬 때
When 패널을 렌더하면
Then 백엔드 불일치 오류가 표시되고 Store 로 조용히 질의하지 않는다
```

- vitest: 대표 패널 1종 — `'미지원 백엔드 에이전트 참조 시 불일치 오류를 표시하고 질의하지 않는다'`
- i18n: `dataSourceTsdbBackendMismatch` (AC-02)

---

## L. i18n

### AC-49 — ko/en 키가 대칭이다

```bash
cd web && node -e "
const ko=require('./src/lib/i18n/ko.json'), en=require('./src/lib/i18n/en.json');
const flat=(o,p='')=>Object.entries(o).flatMap(([k,v])=>typeof v==='object'&&v?flat(v,p+k+'.'):[p+k]);
const a=new Set(flat(ko)), b=new Set(flat(en));
console.log('ko-only:',[...a].filter(k=>!b.has(k)).length,'en-only:',[...b].filter(k=>!a.has(k)).length);
"
# 기대 "ko-only: 0 en-only: 0" · 현재 "ko-only: 0 en-only: 0" (v0.2.0 회차에 실행함)
```

---

## M. 이번 회차(v0.2.0)에 실제 실행한 단언

아래는 SPEC 작성 시점에 **실행하고 출력을 관측한** 항목이다. 실행하지 않은 것은 vitest/Go 테스트 이름(아직 존재하지 않는 테스트)뿐이다.

| 그룹 | 실행한 단언 | 결과 |
|------|-------------|------|
| A | AC-01 4건 · AC-02 5건 · AC-03 5건 · AC-04 2건 | 전부 관측. 위 `현재:` 값에 반영 |
| B | AC-05 1건 · AC-06 2건 · AC-07 4건 · AC-08 2건 | 전부 관측 |
| E | AC-17 5건 · AC-18 1건 · AC-21 1건 · AC-22 2건 · AC-25 4건 · AC-26 2건 · AC-27 1건 · AC-28 1건 | 전부 관측 |
| F | AC-29 3건 · AC-30 3건 · AC-31 1건 · AC-32 2건 | 전부 관측 |
| G | AC-34 2건 · AC-35 4건 | 전부 관측 |
| H | AC-37 1건 | 관측 |
| I | AC-41 1건 · AC-43 3건 · AC-44 1건 · AC-45 1건 · **AC-46 3건 실행**(`go build ./...` exit 0 · `go vet ./...` exit 0 · `npx tsc --noEmit` exit 0) | 전부 관측 |
| K | AC-50 3건 + 선례 3건 · AC-51 3건 + 참고 2건 · AC-52 2건 · AC-53 6건 | 전부 관측 |
| L | AC-49 1건 실행 → `ko-only: 0 en-only: 0` | 관측 |

**v0.1.0 에서 잘못 기록되었던 항목의 정정**

| v0.1.0 단언 | v0.1.0 기록 | 실제 |
|-------------|-------------|------|
| `grep -c 'hasSeriesReduce' …/panelDataSource.ts \|\| echo 0` | 현재 0 | 파일 미존재로 **아무것도 출력되지 않음**. v0.2.0 은 `grep -o … 2>/dev/null \| wc -l` 로 교체해 0 을 실제 출력 |
| `grep -c 'bucket_start_ms' internal/api/dto/influxdb.go \|\| echo 0` | 현재 0 | 동일 함정. v0.2.0 에서 해당 단언 자체가 은퇴(행렬 응답 폐기) |
| `grep -cE '/ *intervalMs…' web/src/services/api/influxdb.ts \|\| echo 0` | 현재 0 | 동일 함정. AC-44 가 파일명을 `tsdbSource.ts` 로 바꾸고 robust 형태로 교체 |
| `grep -c 'Record<ChartDataSourceKind' …panelDataSource.ts \|\| echo 0` | 현재 0 | 동일 함정. AC-37 robust 형태로 교체 |
| AC-28 `maxAggregationBuckets … \| wc -l` | 현재 4 | **횟수 4 / 파일 1.** v0.2.0 은 파일 수 기준(현재 1, 기대 2 이상)으로 바꿔 의미를 명확히 함 |
| AC-27 `grep -c 'Truncate(bucketInterval)'` | 기대 2 · 현재 2 | 값은 맞으나 **AC 자체가 은퇴**(memTSDB 비교 대상 제거) |

---

## M2. M7 회차에 실제 실행한 단언 (구현 완료 후)

아래는 **M7 마무리 회차에 실행하고 출력을 직접 관측한** 항목이다. 값은 전부 이 회차의 실측이며, 이전 회차에서 이월한 값이 아니다. 로그는 `.moai/state/verify/tsdb002-m7/` 에 남겼다.

### M2.1 정적 검사 · 전체 회귀 (AC-46 · M7.1)

| 단언 | 명령 | 관측 결과 |
|------|------|-----------|
| Go 정적 검사 | `go vet ./...` | **exit 0** (`wp4-go-vet.log`) |
| Go 전체 테스트 | `go test ./...` | **exit 0**, `grep -c '^FAIL'` → **0건** (`wp4-go-test.log`) |
| 타입 검사 | `cd web && npx tsc --noEmit` | **exit 0** (`wp4-tsc.log`) |
| 린트 (스코프) | `cd web && npx eslint src --max-warnings 0` | **exit 0** (`wp4-eslint-src.log`) |
| 린트 (비스코프) | `cd web && npx eslint .` | **exit 0** (`wp4-eslint-all.log`) |
| 프론트 전체 테스트 | `cd web && npx vitest run` | **exit 0** — **296 파일 / 4067 테스트 전량 통과** (`wp4-vitest.log`) |

### M2.2 grep 단언

| AC | 명령 | 기대 | 관측 |
|----|------|------|------|
| AC-43 #1 | `grep -coF 'QueryLanguage string' internal/api/handler/influxdb_query.go` | 1 | **1** |
| AC-43 #2 | `grep -coF 'lang != "flux" && lang != "influxql"' internal/api/handler/influxdb_query.go` | 1 | **1** |
| AC-43 #3 | `grep -choE 'g\.(GET\|POST\|DELETE)Perm\("/influxdb/\{agent_name\}/buckets' internal/api/handler/influxdb_management.go` | 4 | **4** |
| AC-45 | `grep -rhoE "from .@/hooks/useTsdb.\|from ./useTsdb." web/src --include='*.ts' --include='*.tsx' \| wc -l` | 0 | **0** |
| AC-49 | ko/en 평탄화 키 대칭 스크립트 | `0 / 0` | **`ko-only: 0 en-only: 0`** (`wp4-i18n-symmetry.log`) |

### M2.3 테스트 존재 · 갈음

| AC | 판정 | 근거 |
|----|------|------|
| AC-42 | **갈음** | 특성화 CT-01 ~ CT-21 전량 GREEN (M2.1 의 vitest 4067건 전량 통과에 포함) |
| AC-47 | **충족** | `panelDataSource.test.ts` 에 두 테스트가 존재하고 통과한다 — `'tsdb + tsdb_source 부재 → tsdb/inactive (channel 폴백 아님)'` · `'인식 불가 문자열 → channel/active + unknownKind 플래그'` |

### M2.4 커버리지 (M7.2)

`cd web && npx vitest run --coverage` → **exit 0** (`wp4-coverage.log`). 전체 `All files` 90.15% Stmts.

| 신규 파일 | % Stmts | 85% 기준 |
|-----------|---------|----------|
| `panelDataSource.ts` | 96.29 | 충족 |
| `panelSeriesStatus.ts` | 100 | 충족 |
| `usePanelSeriesData.ts` | 100 | 충족 |
| `useTsdbChartData.ts` | 97.57 | 충족 |
| `TsdbSourceSection.tsx` | 96.12 | 충족 |
| `seriesMatrixPivot.ts` | 100 | 충족 |
| `tsdbSource.ts` | 100 | 충족 |

수정 파일 2종은 DoD 의 85% 대상이 아니며 기록만 남긴다 — `ChartPanelSections.tsx` 80.3%, `seriesDataSource.ts` 50%.

> 이 회차에 `web/vite.config.ts` 의 coverage `include` 를 확장했다. 확장 전에는 위 표의 `TsdbSourceSection.tsx` · `seriesMatrixPivot.ts` · `tsdbSource.ts` 와 수정 파일 `seriesDataSource.ts` 가 **측정 대상 밖**이었다(spec.md §HISTORY-0.5.0 (3)).

#### 백엔드 신규 파일 (M7.2 보강 회차)

위 표는 **프론트 신규 7종만** 측정했다. 같은 85% 게이트가 걸리는 Go 신규 파일은 이 회차에 별도 측정했다 — `go test -coverprofile ./internal/agent/system/... ./internal/api/handler/... ./internal/api/dto/...` 기준 파일 평균이다.

| 신규 파일 | 평균 | 85% 기준 |
|-----------|------|----------|
| `internal/agent/system/influxdb_seriesenum.go` | 100.0% | 충족 |
| `internal/agent/system/influxdb_seriesenum_v3.go` | 100.0% | 충족 |
| `internal/agent/system/influxdb_seriesquery.go` | 99.5% | 충족 |
| `internal/api/handler/influxdb_series.go` | 99.5% | 충족 |
| `internal/api/handler/influxdb_seriesenum.go` | 97.6% | 충족 |
| `internal/agent/system/influxdb_schema.go` | **71.5% → 86.7%** | 보강 후 충족 |

`influxdb_schema.go` 는 보강 전 **미달**이었다. 미달분의 정체는 `influxV2Client` · `influxV3Client` 의 디스커버리 어댑터 6종으로, 쿼리 생성(`buildFlux*`)과 결과 파싱(`collectFluxSchemaValues`)은 각각 100% 였으나 **그 둘을 잇는 어댑터 자체가 한 번도 실행되지 않았다**.

보강 내용 — `influxdb_schema_test.go` 에 4개 테스트 추가:

- `TestInfluxSchema_V2_어댑터_왕복_D2D3D4` — httptest 인프로세스 서버로 v2 디스커버리 3종을 끝에서 끝까지 왕복. 기본 버킷 해석 · 내부 컬럼(`_`) 필터 · 쿼리에 실린 bucket 을 함께 고정한다
- `TestInfluxSchema_V2_어댑터_인자검증_거부` — 인자 검증 실패가 네트워크 요청을 발생시키지 않음을 고정(`calls == 0`)
- `TestInfluxSchema_V2_어댑터_질의오류_전파` — 세 경로가 서로 구분되는 맥락 메시지로 감싸는지 고정
- `TestInfluxSchema_BuildFluxFieldKeysQuery_인자거부` — D4 생성기의 검증 분기(빈 measurement · 제어문자 bucket)

> **v3 어댑터 2종(`ListTagValues` · `ListFieldKeys`)은 0% 로 남는다 — 수용된 한계다.** influxdb3-go 는 Arrow Flight(gRPC)로 질의하므로 `net/http/httptest` 서버로 대체할 수 없다(`influxdb_seriesenum_v3_test.go` 머리말이 같은 제약을 기록한다). 이들을 덮으려면 gRPC 수준의 테스트 하네스가 필요하며 본 SPEC 범위 밖이다. 파일 평균 86.7% 는 이 잔여를 포함한 값이다.

### M2.5 i18n 은퇴 키 (M7.4)

은퇴 대상이던 placeholder 키 `dashboard.chart.dataSourceTsdbTitle` · `dataSourceTsdbBody` 는 **이미 제거되어 있다**(M6 커밋 `bf461774`). ko/en 양쪽에서 0건이다.

데이터소스 표면(`dashboard.chart.*` · `dashboard.settings.*` · `dashboard.heatmap.*` · `panel.settings.*` · `tsdb.*` · `series.*`)을 대상으로 고아 키를 전수 조사한 결과, **본 SPEC 이 은퇴시킨 개념에 귀속되는 고아 키는 0건**이다. 검출된 고아 후보 21건은 전부 선행 SPEC(SPEC-PANEL-SETTINGS-001 `69f06709` · `9c250ddf`, SPEC-HEATMAP-PANEL-001 `69294ced`, SPEC-STORE-004 `78744734`)에 귀속되므로 본 SPEC 범위 밖이며 삭제하지 않는다.

---

## N. 완료 게이트 요약

| 게이트 | 조건 | 상태 |
|--------|------|------|
| M1 종료 | AC-01 · AC-03 · AC-04 · AC-05 · AC-37 | 이전 회차 종료 |
| M2 종료 | AC-09 ~ AC-12 의 CT 테스트가 **수정 전 코드**에서 전량 GREEN, 프로덕션 diff 0 | 이전 회차 종료 |
| M3 종료 | AC-06 · AC-07 · AC-08 · AC-41 + M2 CT 전량 GREEN 유지 | 이전 회차 종료 |
| M4 종료 | AC-17 ~ AC-28 | 이전 회차 종료 |
| M5 종료 | AC-29 ~ AC-33 · AC-43 | 이전 회차 종료 |
| M6 종료 | AC-02 · AC-18 · AC-34 · AC-35 · AC-36 · AC-38 ~ AC-40 · AC-44 · AC-45 · AC-48 · AC-50 ~ AC-54 | 이전 회차 종료 (커밋 `caa5d62e` · `427477a0` · `1bc1008b` · `bf461774`) |
| M7 종료 | AC-42 · AC-46 · AC-47 · AC-49 + 신규 파일 커버리지 85% | **충족 — 이 회차 실측** (§M2). AC-42 갈음 · AC-46 6종 전부 exit 0 · AC-47 두 테스트 존재 · AC-49 `0 / 0` · 신규 파일 7종 전부 85% 이상 |

> "이전 회차 종료"는 해당 회차에 기록된 사실이며 이 회차에 재측정한 값이 아니다. 이 회차에 직접 관측한 것은 §M2 에 열거한 항목뿐이다. 다만 §M2.1 의 `go test ./...` · `npx vitest run` 전량 통과는 M2 특성화(CT-01 ~ CT-21)를 포함한 전 회차 테스트가 현재 트리에서 여전히 GREEN 임을 의미한다.
