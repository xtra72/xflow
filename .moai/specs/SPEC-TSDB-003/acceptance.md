# SPEC-TSDB-003 인수 조건 (v0.1.0)

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)

형식: **기계 검증 가능한 단언** — 실행 가능한 셸 명령과 기대 출력, 또는 테스트 파일·테스트명.

## 실행 규약

[SPEC-TSDB-002](../SPEC-TSDB-002/acceptance.md) 의 규약을 그대로 승계한다.

- 모든 셸 단언은 **저장소 루트**에서 실행한다.
- **`grep -c … || true` 를 쓰지 않는다.** 파일이 없을 때 아무것도 출력하지 않아 "0"을 잘못 기록하게 된다. 항상 다음 형태를 쓴다.

  ```bash
  grep -o <패턴> <파일> 2>/dev/null | wc -l     # 미존재 파일 → 0
  ```

- **수치는 "일치 횟수"(occurrence)이며 "일치 줄 수"가 아니다.**
- 각 단언에 `현재:` 값을 병기한다. 이는 **v0.1.0 작성 시점(HEAD `e2809f4b`)에 실제로 실행해 관측한 값**이며, 구현 후 `기대:` 값으로 바뀌어야 한다.
- 리터럴에 `|` `(` `{` `"` 등이 포함되면 `grep -oF`(고정 문자열)를 쓴다.

## 미확정 의존 (읽기 전 확인)

**AC-13 ~ AC-17(v3 열거)은 spec.md §7 OQ2 가 확정된 뒤에야 최종 형태가 정해진다.** 아래 본문은 권장안(SQL `DISTINCT` 3단)을 전제로 적혀 있으며, M1 실측이 `GROUP BY *` 단일 쿼리를 채택하면 해당 AC 의 쿼리 문자열 단언이 바뀐다. **그 외 AC 는 OQ 확정과 무관하다.**

## 공통 픽스처

**F1 — 열거 응답 2행** (v2 경로, `field_exact: true`)

```json
{
  "series": [
    { "tags": { "host": "a", "region": "kr" }, "fields": ["idle", "usage"] },
    { "tags": { "host": "b", "region": "kr" }, "fields": ["usage"] }
  ],
  "field_exact": true, "count": 2, "truncated": false,
  "window": { "start_ms": 1700000000000, "end_ms": 1702592000000 }
}
```

전개된 후보 행 **3개**: `(cpu, idle, {host:a,region:kr})` · `(cpu, usage, {host:a,region:kr})` · `(cpu, usage, {host:b,region:kr})`.

**F2 — 열거 응답 근사 field** (v3 경로, `field_exact: false`)

```json
{
  "series": [
    { "tags": { "host": "a" }, "fields": ["usage", "idle"] },
    { "tags": { "host": "b" }, "fields": ["usage", "idle"] }
  ],
  "field_exact": false, "count": 2, "truncated": false,
  "window": { "start_ms": 1700000000000, "end_ms": 1702592000000 }
}
```

두 태그 집합이 **같은** field 목록을 갖는 것이 근사의 표식이다(§2.5 3단계).

**F3 — 열거 명세 1종** (쿼리 생성 테스트의 공통 입력)

```
measurement = "cpu"
bucket      = "metrics"
startMs     = 1700000000000
endMs       = 1702592000000
tagFilter   = { "region": "kr" }
rowLimit    = 20000
```

**F4 — 절단 응답**: `count = 1000`, `truncated = true`.

---

## A. 열거 라우트 D5 (U2)

### AC-01 — `GET /influxdb/{agent_name}/series` 가 `store.read` 로 등록된다

```bash
grep -oF 'g.GETPerm("/influxdb/{agent_name}/series", "store.read"' \
  internal/api/handler/influxdb_management.go 2>/dev/null | wc -l   # 기대 1 · 현재 0
```

- Go: `TestInfluxSeriesEnum_RouteRegistered`

### AC-02 — 응답이 5개 최상위 키를 담는다

```bash
for k in '"series"' '"field_exact"' '"count"' '"truncated"' '"window"'; do
  grep -ohF "$k" internal/api/dto/influxdb.go 2>/dev/null | wc -l
done   # 기대 각 1 이상 · 현재 전부 0
```

- Go: `TestInfluxSeriesEnum_ResponseShape` — F1 을 직렬화해 5키 존재를 단언

### AC-03 — `measurement` 누락은 400

- Go: `TestInfluxSeriesEnum_MissingMeasurement_400`

### AC-04 — 오류 매핑 4종

| 조건 | 기대 |
|------|------|
| 에이전트 없음 | 404 |
| 에이전트가 InfluxDB 아님 | 400 |
| 이스케이프 불가 식별자 | 400 |
| `end_ms <= start_ms` | 400 |

- Go: `TestInfluxSeriesEnum_ErrorMapping` (4 서브테스트)

### AC-05 — `tags` 질의 파라미터가 `k=v,k=v` 로 파싱된다

- Go: `TestInfluxSeriesEnum_ParseTagFilter` — `"region=kr,host=a"` → `map[region:kr host:a]`; `"region"`(등호 없음) → 400

### AC-06 — 응답 `series` 가 태그 직렬화 오름차순으로 정렬된다 (UB1-16)

- Go: `TestInfluxSeriesEnum_DeterministicOrder` — 입력 순서를 뒤집어도 같은 출력

---

## B. 순수 생성 함수 (U3)

### AC-07 — 쿼리 생성이 순수 함수이며 네트워크 없이 테스트된다

```bash
ls internal/agent/system/influxdb_seriesenum.go 2>/dev/null | wc -l   # 기대 1 · 현재 0
grep -ocE 'func Build[A-Za-z]*SeriesEnumQuery' \
  internal/agent/system/influxdb_seriesenum.go 2>/dev/null            # 기대 1 이상 · 현재 0
```

### AC-08 — 이스케이프 헬퍼를 재사용한다 (신규 이스케이프 함수 0개)

```bash
# 신규 파일이 자체 이스케이프 함수를 정의하지 않는다
grep -ocE 'func escape[A-Za-z]+\(' internal/agent/system/influxdb_seriesenum.go 2>/dev/null   # 기대 0
# 기존 헬퍼를 호출한다
grep -oE 'escapeFluxStringLiteral|escapeInfluxQLIdent|validateSeriesIdentifier' \
  internal/agent/system/influxdb_seriesenum.go 2>/dev/null | wc -l                            # 기대 1 이상 · 현재 0
```

- Go: `TestSeriesEnumQuery_RejectsUnescapableIdentifier` — 제어문자 포함 measurement → `ErrUnescapableIdentifier`

---

## C. v2 Flux 열거 (U4)

### AC-09 — 생성 쿼리가 그룹 키 파이프라인을 따른다

F3 입력에 대해 생성 문자열이 다음을 **모두** 포함한다.

```bash
# 테스트 안에서 단언한다(아래 Go 테스트). 참고용 리터럴:
#   from(bucket: "metrics")
#   |> range(start: time(v: 1700000000000000000), stop: time(v: 1702592000000000000))
#   |> filter(fn: (r) => r._measurement == "cpu")
#   |> filter(fn: (r) => r["region"] == "kr")
#   |> first()
#   |> group()
#   |> limit(n: 20000)
```

- Go: `TestBuildFluxSeriesEnumQuery_Shape`

### AC-10 — `distinct()` 를 쓰지 않는다 (UB1-13)

```bash
grep -oF 'distinct(' internal/agent/system/influxdb_seriesenum.go 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

### AC-11 — 밑줄 접두 컬럼이 태그에서 제외되고 `_field` 가 field 로 승격된다

입력 행 `{_start:…, _stop:…, _measurement:"cpu", _field:"usage", _value:1, host:"a"}` →
출력 `{tags:{host:"a"}, fields:["usage"]}`.

- Go: `TestFoldEnumRows_ExcludesInternalColumns`

### AC-12 — (field, 태그 집합) 행이 태그 집합 기준으로 접히고 `field_exact` 가 참이다

행 3개 → 태그 집합 2개(F1). `fields` 는 사전순 정렬.

- Go: `TestFoldEnumRows_GroupsByTagSet`
- Go: `TestInfluxSeriesEnum_V2_FieldExactTrue`

---

## D. v3 열거 (U5) — **OQ2 확정 의존**

### AC-13 — `SHOW SERIES` 문자열이 트리에 존재하지 않는다 (UB1-1)

```bash
grep -rhoF 'SHOW SERIES' internal/ --include='*.go' 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

이 단언은 **현재도 0 이므로 회귀 방지용**이다. v3 경로를 작성하다 `SHOW SERIES` 로 돌아가는 것을 막는다.

### AC-14 — v3 열거가 지원되는 SHOW 문만 쓴다

```bash
# 생성 문자열에 등장하는 SHOW 계열이 문서상 지원 3종뿐이다
grep -ohE 'SHOW (MEASUREMENTS|TAG KEYS|TAG VALUES|FIELD KEYS)' \
  internal/agent/system/influxdb_seriesenum.go 2>/dev/null | sort -u
# 기대: SHOW FIELD KEYS / SHOW TAG KEYS 만 (또는 OQ2 확정에 따라 0줄)
```

### AC-15 — v3 경로가 `field_exact: false` 를 반환한다

- Go: `TestInfluxSeriesEnum_V3_FieldExactFalse` — 모의 클라이언트로 F2 산출

### AC-16 — v3 3단 조회의 호출 순서와 인자

모의 클라이언트가 다음을 관측한다: `SHOW TAG KEYS FROM "cpu"` → SQL `DISTINCT` (관측된 태그 키만 SELECT 목록에 등장) → `SHOW FIELD KEYS FROM "cpu"`.

- Go: `TestInfluxSeriesEnum_V3_ThreeStepOrder`

### AC-17 — 태그 키 0개 measurement 는 단일 항목을 반환한다

`series = [{ tags: {}, fields: [...] }]`, `count = 1`.

- Go: `TestInfluxSeriesEnum_V3_NoTagKeys`
- Go: `TestInfluxSeriesEnum_V2_NoTagKeys` (v2 대응)

---

## E. 능력 표 확장 (U6)

### AC-18 — `TsdbBackendCapabilities` 에 두 필드가 추가된다

```bash
grep -oF 'seriesEnumeration' web/src/pages/dashboard/panels/charts/panelDataSource.ts 2>/dev/null | wc -l  # 기대 3 이상 · 현재 0
grep -oF 'seriesFieldExact'  web/src/pages/dashboard/panels/charts/panelDataSource.ts 2>/dev/null | wc -l  # 기대 3 이상 · 현재 0
```

기대 3 이상 = 인터페이스 선언 1 + `'2'` 항목 1 + `'3'` 항목 1.

### AC-19 — v2 는 `seriesFieldExact: true`, v3 는 `false`

- vitest: `panelDataSource.test.ts` — `'TSDB_BACKEND_CAPABILITIES 는 v2 에서 seriesFieldExact 가 참이고 v3 에서 거짓이다'`

`Record<InfluxBackendVersion, TsdbBackendCapabilities>` 이므로 필드 누락은 **컴파일 오류**다 — `npx tsc --noEmit`(AC-54)이 이를 겸한다.

---

## F. 상한과 절단 (U7)

### AC-20 — 태그 집합 상한 1,000 이 서버에 강제된다

- Go: `TestInfluxSeriesEnum_TagSetCap` — 1,200개 태그 집합 입력 → `count == 1000`, `truncated == true`

### AC-21 — 원시 행 상한이 생성 쿼리에 들어간다

```bash
grep -oF 'limit(n: 20000)' internal/agent/system/influxdb_seriesenum.go 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
```

(v3 경로는 `LIMIT 20000`. OQ2 확정에 따라 리터럴이 달라진다.)

### AC-22 — 절단 시 상위 N 이 결정적이다 (OQ9)

- Go: `TestInfluxSeriesEnum_TruncationDeterministic` — 같은 입력을 셔플해 2회 실행 → 같은 1,000개

### AC-23 — UI 가 절단 배너를 표시하고 필터 중에도 유지한다

- vitest: `TsdbSourceSection.test.tsx`
  - `'절단 응답에서 절단 배너를 표시한다'` (F4)
  - `'클라이언트 필터가 걸려도 절단 배너가 유지된다'` (UB2-5)

```bash
grep -oF 'chart-tsdb-series-truncated' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
```

---

## G. 탐색 시간창 (U8)

### AC-24 — 창 미지정 시 기본 30일이 적용되고 응답에 드러난다

- Go: `TestInfluxSeriesEnum_DefaultWindow30d` — `start_ms` 미지정 → 응답 `window.start_ms ≈ now - 30d`(허용 오차 5초)

### AC-25 — 지정한 창이 생성 쿼리의 범위에 반영된다

- Go: `TestBuildFluxSeriesEnumQuery_WindowNs` — F3 의 ms → ns 변환 정확도
- Go: `TestBuildSQLSeriesEnumQuery_WindowRFC3339` (v3, OQ2 의존)

### AC-26 — UI 가 탐색 창 컨트롤과 사용된 창을 표시한다

```bash
grep -oF 'chart-tsdb-enum-window' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
```

- vitest: `TsdbSourceSection.test.tsx` — `'탐색 창을 바꾸면 응답의 window 표시가 갱신된다'`

---

## H. 캐시 금지 · config 불변 (U9 · U10)

### AC-27 — 열거 응답을 캐시하지 않는다

- vitest: `TsdbSourceSection.test.tsx` — `'같은 조건으로 재마운트하면 열거를 다시 호출한다'`

```bash
# react-query 를 이 컴포넌트에 도입하지 않는다
grep -oF 'useQuery' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

### AC-28 — `TsdbSourceConfig` 의 필드 집합이 불변이다 (UB1-7)

```bash
awk '/^export interface TsdbSourceConfig/,/^}/' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts \
  | grep -oE '^  [a-z_]+\??:' | tr -d ' ?:' | sort | tr '\n' ' '
# 기대 = 현재:
#   agent_id agent_name aggregation backend bucket fill interval_ms
#   refresh_interval_ms series series_name_format time_window_ms
```

### AC-29 — `TsdbSeriesRef` 의 필드 집합이 불변이다

```bash
awk '/^export interface TsdbSeriesRef/,/^}/' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts \
  | grep -oE '^  [a-z_]+\??:' | tr -d ' ?:' | sort | tr '\n' ' '
# 기대 = 현재: alias color field key smooth stroke_style stroke_width tags
```

### AC-30 — 질의 경로가 무변경이다

```bash
git diff --stat HEAD -- web/src/services/api/tsdbSource.ts \
  web/src/pages/dashboard/panels/charts/useTsdbChartData.ts \
  internal/api/handler/influxdb_series.go \
  internal/agent/system/influxdb_seriesquery.go
# 기대: 출력 없음(변경 0)
```

---

## I. 열거 촉발 (E1 · E2)

### AC-31 — measurement 선택이 열거를 호출한다

- vitest: `TsdbSourceSection.test.tsx` — `'measurement 를 고르면 열거를 호출한다'` — 주입한 열거 페처가 `(agent, 'cpu', {window, tags})` 로 1회 호출됨

### AC-32 — 열거 결과가 후보 행으로 전개된다

F1 → 행 3개, ID 는 `storeSeriesId('cpu', field, tags)`.

- vitest: `TsdbSourceSection.test.tsx` — `'열거 결과가 (태그 집합 × field) 행으로 전개된다'`

### AC-33 — 태그 사전 필터 변경이 재열거를 촉발한다 (UB1-10)

- vitest: `'태그 값을 고르면 그 필터로 재열거한다'` — 페처 2회 호출, 2번째 인자에 `tags: {region:'kr'}`

### AC-34 — 탐색 창 변경이 재열거를 촉발한다

- vitest: `'탐색 창을 바꾸면 재열거한다'`

### AC-35 — 진행 중 요청이 취소된다

- vitest: `'열거 중 measurement 를 다시 바꾸면 이전 응답을 버린다'` — 늦게 resolve 되는 첫 응답이 목록에 반영되지 않음

---

## J. 상태 표시 (S1 · S2 · S3)

### AC-36 — 5상태가 서로 구분된다 (UB1-19)

| 상태 | testid |
|------|--------|
| 비활성(에이전트) | `chart-tsdb-no-agent` (기존) |
| 비활성(measurement) | `chart-tsdb-no-measurement` (기존) |
| 조회 중 | `chart-tsdb-series-loading` (신규) |
| 빈 결과 | `chart-tsdb-series-empty` (신규) |
| 절단 | `chart-tsdb-series-truncated` (신규) |
| 오류 | `chart-tsdb-series-error` (신규) |

```bash
for t in chart-tsdb-series-loading chart-tsdb-series-empty \
         chart-tsdb-series-truncated chart-tsdb-series-error; do
  grep -oF "$t" web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l
done   # 기대 각 1 이상 · 현재 전부 0
```

- vitest: `TsdbSourceSection.test.tsx` — `'빈 결과와 오류를 다른 표시로 구분한다'`

### AC-37 — 열거 실패가 이전 목록을 파괴하지 않는다

- vitest: `'열거가 실패해도 직전 목록과 선택이 유지된다'` (UB2-1)

### AC-38 — 빈 결과 표시가 사용된 탐색 창을 함께 보여 준다

- vitest: `'빈 결과에서 사용된 탐색 창을 함께 표시한다'`

### AC-39 — `field_exact: false` 에서 근사 안내가 표시된다 (S2)

```bash
grep -oF 'seriesFieldApprox' web/src/pages/dashboard/panels/charts/panelDataSource.ts 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
```

- vitest: `'field_exact 가 거짓이면 근사 안내를 표시한다'` (F2)
- vitest: `'field_exact 가 참이면 근사 안내를 표시하지 않는다'` (F1)
- vitest: `'근사 안내는 오류 표시가 아니다'` — `role="alert"` 가 아님

### AC-40 — field 드릴다운의 클라이언트 직접 호출이 사라진다 (S3)

```bash
grep -oF 'fetchFieldKeys' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l  # 기대 0 · 현재 3
grep -oF 'fieldKeys.items.map' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l  # 기대 0 · 현재 1
```

`fetchFieldKeys` 현재 3회 = `TsdbDiscoveryFetchers` 선언 · `DEFAULT_FETCHERS` 매핑 · `useDiscoveryList` 호출.

### AC-41 — D4 라우트와 프런트 클라이언트는 **남는다**

```bash
grep -oF 'g.GETPerm("/influxdb/{agent_name}/field-keys"' \
  internal/api/handler/influxdb_management.go 2>/dev/null | wc -l          # 기대 1 · 현재 1
grep -oF 'export async function fetchInfluxFieldKeys' \
  web/src/services/api/influxdbManagement.ts 2>/dev/null | wc -l           # 기대 1 · 현재 1
```

본 SPEC 은 소비를 멈출 뿐 라우트를 제거하지 않는다(§4.4).

### AC-42 — measurement · 태그 드릴다운은 유지된다

```bash
for t in chart-tsdb-measurement-select chart-tsdb-tag-key-select chart-tsdb-tag-value-select; do
  grep -oF "$t" web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l
done   # 기대 각 1 · 현재 각 1
```

---

## K. 금지 동작 (UB1)

### AC-43 — 데카르트 곱 후보 생성이 없다 (UB1-2)

```bash
# tagValues 를 순회해 조합을 만드는 코드가 없다
grep -oF 'tagValues.items.map' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l   # 기대 1 · 현재 1
```

기대 1 = `<option>` 렌더 1건뿐이며 후보 행 생성에는 쓰이지 않는다.

- vitest: `'태그 값 목록은 사전 필터 선택지일 뿐 후보 행을 만들지 않는다'`

### AC-44 — 시리즈 선택 상한 48 무변경 (UB1-17)

```bash
grep -oF 'STORE_SERIES_LIMIT = 48' \
  web/src/pages/dashboard/panels/charts/chartChannelTypes.ts 2>/dev/null | wc -l   # 기대 1 · 현재 1
```

- vitest(특성화 CT-03): `'48 초과 선택은 절삭되고 배너를 띄운다'`

### AC-45 — 원문 통과 라우트의 언어 화이트리스트 무변경 (UB1-11)

```bash
grep -oF "lang != \"flux\" && lang != \"influxql\"" \
  internal/api/handler/influxdb_query.go 2>/dev/null | wc -l   # 기대 1 · 현재 1
```

### AC-46 — 열거는 원문 통과 핸들러를 경유하지 않는다

```bash
grep -oF 'influxFluxQueryer' internal/api/handler/influxdb_management.go 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

### AC-47 — `internal/migrate/tsdbtags` import 0건 유지 (UB1-12)

```bash
grep -rhoF 'migrate/tsdbtags' internal/api/ internal/agent/ 2>/dev/null | wc -l   # 기대 0 · 현재 0
```

### AC-48 — `InfluxSchemaDiscoverer` 를 넓히지 않는다 (위험 R5)

```bash
awk '/^type InfluxSchemaDiscoverer interface/,/^}/' \
  internal/agent/system/influxdb_schema.go | grep -cE '^\t[A-Z][A-Za-z]+\('   # 기대 3 · 현재 3
```

열거는 **별도 인터페이스**가 소유한다.

```bash
grep -ocE '^type InfluxSeriesEnumerator interface' \
  internal/agent/system/influxdb_seriesenum.go 2>/dev/null   # 기대 1 · 현재 0
```

### AC-49 — `SeriesSelectTable` 무변경 (UB1-18 · §1.3)

```bash
git diff --stat HEAD -- web/src/pages/agents/SeriesSelectTable.tsx
# 기대: 출력 없음(변경 0)
```

---

## L. 상태 정합 (UB2)

### AC-50 — 열거에 없는 저장된 시리즈가 유지된다 (UB2-2)

주어진 config 의 `series` 에 `(cpu, usage, {host:'z'})` 가 있고 열거 결과(F1)에 없을 때, 선택은 유지되고 "목록에 없음" 표시가 붙는다.

- vitest: `'열거에 없는 저장된 시리즈를 선택 상태로 유지하고 표시를 덧붙인다'`

```bash
grep -oF 'chart-tsdb-series-missing' web/src/pages/dashboard/TsdbSourceSection.tsx 2>/dev/null | wc -l  # 기대 1 이상 · 현재 0
```

### AC-51 — 에이전트 변경 시 `series: []` 초기화 유지 (UB2-4, 특성화 CT-02)

- vitest: `'에이전트를 바꾸면 시리즈 선택이 비워진다'`

### AC-52 — 탐색 창을 좁혀 목록이 줄어도 선택은 유지된다 (UB2-6)

- vitest: `'탐색 창을 좁혀 시리즈가 목록에서 사라져도 선택은 유지된다'`

### AC-53 — 열거 실패가 config 를 바꾸지 않는다 (UB2-1)

- vitest: `'열거 실패 시 onConfigChange 가 호출되지 않는다'` — `expect(onConfigChange).not.toHaveBeenCalled()`

---

## M. 정적 검사 · 커버리지

### AC-54 — 정적 검사 무결

```bash
go build ./...                              # exit 0
go vet ./...                                # exit 0
cd web && npx tsc --noEmit                  # exit 0
cd web && npx eslint src --max-warnings 0   # exit 0
```

네 명령 전부 HEAD `e2809f4b` 기준 baseline exit 0 이다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §HISTORY-0.5.0 (2) 가 eslint 를 0 으로 만들었다).

### AC-55 — 신규 파일 커버리지 85% 이상, **그리고 측정 대상에 포함됨**

```bash
# (1) 먼저 include 를 확인한다 — 측정되지 않은 채 참으로 보이는 상태를 막는다
grep -n 'include' web/vite.config.ts
# (2) 그 다음 커버리지를 측정한다
cd web && npx vitest run --coverage
go test ./internal/agent/system/... ./internal/api/handler/... -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out
```

대상 신규 파일:

| 파일 | 기준 |
|------|------|
| `internal/agent/system/influxdb_seriesenum.go` | 85% 이상 |
| `internal/api/handler` 의 열거 핸들러 | 85% 이상 |
| `web/src/services/api/influxdbManagement.ts`(확장분) | 85% 이상 |
| `web/src/pages/dashboard/TsdbSourceSection.tsx`(수정분) | 회귀 없음 — 현재 96.12% |

---

## N. 특성화 테스트 (M6.1)

열거를 도입하기 **전에** 잠글 동작. 전부 도입 후에도 통과해야 한다(CT-07 제외).

| ID | 테스트명 | 도입 후 |
|----|----------|---------|
| CT-01 | `'에이전트 미선택 시 조회하지 않고 안내를 표시한다'` | 통과 유지 |
| CT-02 | `'에이전트를 바꾸면 시리즈 선택이 비워진다'` | 통과 유지 |
| CT-03 | `'48 초과 선택은 절삭되고 배너를 띄운다'` | 통과 유지 |
| CT-04 | `'목록에 없는 에이전트/버킷을 선택 상태로 유지한다'` | 통과 유지 |
| CT-05 | `'디스커버리 응답을 캐시하지 않는다'` | 통과 유지 |
| CT-06 | `'measurement 미선택 시 선택 표를 노출하지 않는다'` | 통과 유지 |
| CT-07 | `'후보 행 = field 키 × 태그 1벌 (SPEC-TSDB-002 §2.15)'` | **제자리 반전** — 삭제하지 않고 본문을 AC-32 로 바꾸고 반전 사유·대체 SPEC ID 를 주석으로 남긴다 |
