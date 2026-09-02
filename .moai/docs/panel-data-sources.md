# 대시보드 패널 데이터소스 — Store · TSDB 능력 비교

관련 SPEC: [SPEC-TSDB-002](../specs/SPEC-TSDB-002/spec.md) · [SPEC-STORE-004](../specs/SPEC-STORE-004/spec.md) · [SPEC-CHART-002](../specs/SPEC-CHART-002/spec.md) · [SPEC-WEB-005](../specs/SPEC-WEB-005/spec.md)

이 문서는 대시보드 패널이 고를 수 있는 데이터소스의 능력 차이를 **구현 기준으로** 정리한다. 서술의 근거는 SPEC 본문의 산문이 아니라 트리의 실제 코드이며, 각 항목에 파일 경로를 붙였다.

패널이 고를 수 있는 소스는 셋이다 — `channel` · `store` · `tsdb`. 이 중 조회 파라미터를 갖는 것은 `store` 와 `tsdb` 둘뿐이므로 아래 비교 표는 그 둘을 대상으로 한다. `channel` 은 플로우 채널을 직접 구독하므로 집계 · 시간창 · fill 같은 축이 존재하지 않는다.

**memTSDB 는 이 목록에 없다.** 그 이유와 남은 상태는 §3 에 적었다.

---

## 1. 두 소스의 정체

| 이름 | 실체 | 위치 | 패널 라우트 |
|------|------|------|-------------|
| **Store** | 인메모리 TSDB. `NewVolatileStore(maxKeyLength, maxHistorySize, historyTTL)` — 히스토리와 TTL 을 가진 휘발성 저장소. 생성 가능한 에이전트 타입이다 | `internal/agent/system/store.go` | `POST /api/v1/store/{agent_name}/query` |
| **TSDB** | 에이전트를 통해 접근하는 **외부** 시계열 DB. **InfluxDB 가 첫 지원 백엔드**다 | `internal/agent/system/influxdb_*.go` | `POST /api/v1/influxdb/{agent_name}/series/query` |

두 소스 모두 최종적으로 `SeriesMatrix` 를 산출하며, 패널 렌더 코드는 오직 그것만 소비한다. 소스를 갈아타도 패널 렌더 코드는 바뀌지 않는다.

백엔드 판별은 **참조된 에이전트의 실제 타입**에서 파생한다. 패널 config 에 기록된 `tsdb_source.backend` 값은 라우팅의 정본이 아니라 (a) 에이전트 목록 로드 전의 낙관적 표시값이자 (b) 불일치 감지용 대조군이다 — `resolveTsdbBackend()`(`web/src/services/api/tsdbSource.ts`).

---

## 2. 능력 비교 표

### 2.1 활성 판정과 바인딩

정본은 `resolvePanelSourceBinding()`(`web/src/pages/dashboard/panels/charts/panelDataSource.ts`) 한 곳이다. 패널 각 지점이 `config.data_source === 'store'` 같은 동등 비교를 하지 않는다.

| 축 | Store | TSDB (influxdb) |
|----|-------|-----------------|
| 활성 조건 | `series.length > 0` **또는** (`selection_mode === 'tag'` 이고 `tag_filters` ≥ 1) | `agent_name` 이 비어 있지 않고 **그리고** `series.length > 0` |
| 에이전트 참조 | 선택 (`agentRequired: false`) | **필수** (`agentRequired: true`) — 어느 외부 DB 에 연결할지 알 수 없다 |
| 판정 함수 | `isStoreSourceActive()` | `isTsdbSourceActive()` |
| config 블록 | `store_source` | `tsdb_source` (**백엔드별로 쪼개지 않는 단일 블록** + `backend` 판별자) |
| 시리즈 식별 축 | `key` + `field` + `tags` | `key`(= measurement) + `field` + `tags` |
| `field` 필수 여부 | 선택 | **필수** — "첫 번째 숫자 필드" 류의 폴백을 두지 않는다 |

TSDB 에 `field` 폴백을 두지 않는 이유는 그 폴백이 오류가 아니라 **조용한 오답**을 만들기 때문이다. 잘못된 필드의 값이 정상적인 그래프로 그려진다.

### 2.2 집계 어휘

| `aggregation` | Store 백엔드 | TSDB — Flux (v2) | TSDB — InfluxQL (v3) |
|--------------|--------------|------------------|----------------------|
| `min` | `min` | `min` | `MIN` |
| `max` | `max` | `max` | `MAX` |
| `average` | `avg` | `mean` | `MEAN` |
| `first` | **미지원 → 클라이언트 집계 폴백** | `first` (백엔드 직접) | `FIRST` (백엔드 직접) |
| `last` | **미지원 → 클라이언트 집계 폴백** | `last` (백엔드 직접) | `LAST` (백엔드 직접) |

- Store 쪽 매핑: `toBackendAggregation()`(`web/src/services/api/store.ts`) — `first`/`last` 에서 `null` 을 반환해 클라이언트 집계 경로로 내린다.
- TSDB 쪽 매핑: `fluxAggregationFn()` · `influxQLAggregationFn()`(`internal/agent/system/influxdb_seriesquery.go`).

**`average → mean` 이 매핑 표의 유일한 이름 불일치다.** Flux 에 `average` 함수가 없고 InfluxQL 에 `AVG` 가 없다(`MEAN` 이다). 두 함수 모두 5종을 명시한 `switch` + `default:` 오류로 구현해, 케이스를 빠뜨리면 조용한 오답이 아니라 오류가 나게 했다.

### 2.3 fill (빈 버킷) 처리

| 요청 | Store | TSDB — Flux (v2) | TSDB — InfluxQL (v3) |
|------|-------|------------------|----------------------|
| `""` (생략) | 미지원 (지정해도 무시) | `createEmpty: false` | `FILL(none)` |
| `"null"` | 미지원 | `createEmpty: true` | `FILL(null)` |
| `"zero"` | 미지원 | `createEmpty: true` + 후처리 `fill(value: 0.0)` | `FILL(0)` |
| `"previous"` | 미지원 | `createEmpty: true` + `fill(usePrevious: true)` | `FILL(previous)` |
| `"avg"` | 미지원 | **400 거부** | **400 거부** |

`"avg"` 는 Flux · InfluxQL 어느 쪽에도 대응 연산이 없다. 다른 전략으로 조용히 대체하지 않고 `parseSeriesFill()`(`internal/api/handler/influxdb_series.go`)이 400 으로 거부한다.

Store 가 fill 전략을 지원하지 않는다는 사실은 UI 에서 비활성 + 사유(`capReasonFillStore`)로 드러난다 — 숨기지 않는다.

### 2.4 버킷 정렬의 소유자

| 소스 | 버킷 시작 계산 | 소유자 |
|------|---------------|--------|
| Store | `(tsMs / intervalMs) * intervalMs` — epoch-zero 정렬, 시작 레이블 | 서버 (`internal/api/handler/store_query.go`) |
| TSDB v2 | `aggregateWindow(every: <interval>, ..., timeSrc: "_start")` — `offset` 인자 미지정 | 서버 (`BuildFluxSeriesQuery`) |
| TSDB v3 | `GROUP BY time(<interval>)` — `offset` 인자 미지정(기본 offset 0 = epoch 정렬) | 서버 (`BuildInfluxQLSeriesQuery`) |

**정렬은 전적으로 서버가 소유한다.** 클라이언트는 `floor(ts / interval)` 같은 경계 계산을 하지 않는다 — `collectSeriesBuckets()`(`tsdbSource.ts`)는 서버가 준 타임스탬프를 그대로 키로 쓴다. 클라이언트가 하는 일은 버킷 **합집합 수집과 컬럼 배치**뿐이며, 이는 경계 계산이 아니다.

`timeSrc: "_start"` 는 선택 사항이 아니다. `aggregateWindow` 의 기본값은 `timeSrc: "_stop"` 이고, 그 경우 모든 값이 한 인터벌만큼 미래로 밀린다. 증상이 "값이 틀리다"가 아니라 "값이 약간 늦는다"여서 리뷰에서 잡히지 않는다.

세 소스가 같은 경계를 산출한다는 사실은 `internal/api/handler/bucket_alignment_crosscheck_test.go` 가 divisor 인터벌 7종(1s·10s·30s·60s·300s·900s·3600s)과 non-divisor 인터벌(420s)에서 고정한다.

### 2.5 tag / selection 모드

| 축 | Store | TSDB |
|----|-------|------|
| `selection_mode: 'keys'` (직접 선택) | 지원 | 지원 |
| `selection_mode: 'tag'` (태그 기반 **동적** 바인딩) | 지원 — 폴링 시점마다 매칭 키를 확장 | **미지원** |
| 시리즈 필터로서의 태그 | 지원 | 지원 — 요청 본문의 `tags` |

TSDB 가 지원하지 않는 것은 "태그로 시리즈를 **동적으로 발견**하는 것"이지 "태그로 시리즈를 **한정**하는 것"이 아니다. 후자는 요청 축에 그대로 있다.

### 2.6 부분 실패 처리

| 축 | Store | TSDB |
|----|-------|------|
| 병렬 수집 정책 | `Promise.all` | `Promise.allSettled` |
| 시리즈 1개 실패의 결과 | **전체 실패** — 패널이 비워진다 | **격리** — 실패 컬럼만 전 버킷 `null`, 나머지는 정상 렌더 |
| 실패 신호 | 없음(전체 오류) | 실패 인덱스 목록(`TsdbSeriesFailure[]`)을 상위로 전달 → 부분 실패 배지 |
| 전 시리즈 실패 | 전체 실패 | **전체 실패** — 첫 오류를 그대로 던진다(부분 실패가 아니다) |

**이 차이는 의도적이다.** 본 SPEC 은 Store 를 바꾸지 않으며, TSDB 어댑터에만 `allSettled` 를 요구한다. 두 소스를 맞추는 작업은 SPEC-TSDB-002 §7 NQ3 이 후속 SPEC 으로 열어 두었다.

부분 실패는 성공으로도 오류로도 뭉개지 않는다. 성공으로 뭉개면 사용자가 시리즈가 비어 보이는 이유를 모르고, 오류로 뭉개면 성공한 나머지 시리즈까지 오버레이에 가린다. 다음 폴링이 전부 성공하면 배지는 사라진다.

### 2.7 취소 전파

| 축 | Store | TSDB |
|----|-------|------|
| `AbortSignal` 개별 요청 전달 | 지원 | 지원 |
| 취소 시 처분 | axios 요청 중단 | 중단 + `AbortError` 를 던져 결과를 상태에 반영하지 않는다 |

폴링 중 config 가 바뀌면 이전 요청이 남는다. 두 어댑터 모두 signal 을 **각 요청에** 전달하며, TSDB 는 `signal.aborted` 를 확인해 취소된 조회의 매트릭스를 돌려주지 않는다.

### 2.8 요청 축과 응답 형상

| 축 | Store | TSDB |
|----|-------|------|
| 요청 단위 | **시리즈당 1요청** | **시리즈당 1요청** |
| N 시리즈 패널의 폴링당 요청 수 | N (최대 48) | N (최대 48) |
| 응답 타입 | `chartQueryResponse{entries[], count, truncated}` | **동일** — 신규 응답 타입을 만들지 않는다 |
| 시리즈 구분 | `labels` (`__field__` 예약 키 + 태그) | **동일한 라벨 규약** |
| 피벗 구현 | `buildSeriesMatrix()`(`seriesMatrixPivot.ts`) | **같은 함수** |

두 소스가 같은 피벗 기계를 쓰는 것이 설계의 하중 지지점이다. `groupEntriesBySeries` · 버킷 합집합 피벗 · 0행 컬럼 자리 보존이 전부 소스를 모른 채 동작하므로, TSDB 를 얹는 데 새 기계가 생기지 않았다.

시리즈당 1요청의 비용(폴링당 최대 48회 InfluxDB 질의)은 본 SPEC 이 **의도적으로 수용한** 지점이다. Store 와 같은 축을 유지하는 일관성을 우선했고, 배치 확장 여지는 §7 NQ2 에 기록되어 있다.

### 2.9 가드

| 축 | 상한 | 강제 위치 |
|----|------|-----------|
| 버킷 수 = `ceil((end_ms - start_ms) / interval_ms)` | **100,000** | 서버. Store 와 **같은 상수**(`maxAggregationBuckets`)를 쓴다 |
| 시리즈 수 | **48** | 클라이언트 선택 UI (`STORE_SERIES_LIMIT`). 요청 1건이 시리즈 1개이므로 서버는 셀 대상이 없다 |

두 상한을 Store 와 같은 값으로 둔 이유는 소스를 갈아탔을 때 "왜 이건 되고 저건 안 되지"가 생기지 않게 하기 위함이다.

---

## 2.10 InfluxDB v2 와 v3 의 능력 차이

버전은 소스 종류가 아니라 **백엔드의 하위 축**이다. `ChartDataSourceKind` 를 늘리지 않고 `TSDB_BACKEND_CAPABILITIES`(`panelDataSource.ts`)가 이 축을 담는다.

| 능력 | v2 | v3 | 근거 |
|------|----|----|------|
| 질의 언어 | Flux | InfluxQL | v3 는 Flux 미지원. SQL 은 클라이언트가 지원하나 HTTP 핸들러가 `flux\|influxql` 만 화이트리스트하므로 도달 불가 |
| bucket 을 **목록에서** 선택 (`bucketList`) | O — `GET /buckets` | **X** — 관리 API 없음 → 자유 입력 | `TSDB_BACKEND_CAPABILITIES['3'].bucketList = false` |
| 지정한 bucket 이 **질의에 반영됨** (`bucketAffectsQuery`) | O — Flux `from(bucket: "...")` | **X** | 아래 참조 |
| 관리 조작 생성 · 삭제 · truncate (`management`) | O | **X — 501** | 아래 참조 |
| 스키마 디스커버리 (measurements · tag-keys · tag-values · field-keys) | O | **O — v3 에서도 활성** | `SHOW ...` InfluxQL |

### v3 에서 `bucket` 이 질의에 도달하지 않는다

InfluxQL 템플릿 `SELECT <FN>("<f>") FROM "<m>"` 에는 database 를 담을 자리가 없다. v3 에서 database 는 **클라이언트 연결에 바인딩**되어 있으므로, 요청 본문의 `bucket` 필드는 **v2(Flux `from(bucket:)`)에서만 유효**하고 v3 에서는 무시되어 에이전트 기본 database 가 쓰인다.

이 사실을 UI 가 드러내지 않으면 사용자는 값을 바꿔도 결과가 그대로인 이유를 알 수 없다. 그래서 v3 에이전트가 선택되면 bucket 입력이 비활성 + 사유(`capReasonBucketV3`)로 표시된다.

### v3 에서 501 로 남아 있는 관리 조작 5종

`internal/agent/system/influxdb_v3.go` 가 `ErrManagementNotSupported` 를 반환하는 지점은 정확히 다섯이다.

| # | 메서드 | 라우트 |
|---|--------|--------|
| 1 | `ListBuckets` | `GET /influxdb/{agent_name}/buckets` |
| 2 | `CreateBucket` | `POST /influxdb/{agent_name}/buckets` |
| 3 | `DeleteBucket` | `DELETE /influxdb/{agent_name}/buckets/{bucket}` |
| 4 | `TruncateBucket` | `POST /influxdb/{agent_name}/buckets/{bucket}/truncate` |
| 5 | `DeleteMeasurement` | `DELETE /influxdb/{agent_name}/measurements/{name}` |

**`ListMeasurements` 는 이 목록에서 빠졌다 — 의도된 회귀다.** 이전에는 501 이었으나 `SHOW MEASUREMENTS` 로 실제 동작하므로 v3 에서 501 → 200 으로 바뀌었다. v3 에 없는 것은 **관리 API** 이지 스키마 조회가 아니다. 나머지 5종의 501 은 그대로 유지된다 — SPEC-TSDB-002 는 읽기 전용 디스커버리만 다룬다.

UI 에서는 v3 선택 시 관리 조작이 비활성 + 사유(`capReasonManagementV3`)로 표시되며, 그 문구는 "스키마 조회는 사용할 수 있습니다"를 함께 알린다.

### 디스커버리 라우트 4종 (v2 · v3 모두 활성)

| # | 라우트 | 권한 | v2 | v3 |
|---|--------|------|----|----|
| D1 | `GET /influxdb/{agent_name}/measurements` | `store.read` | `schema.measurements()` | `SHOW MEASUREMENTS` |
| D2 | `GET /influxdb/{agent_name}/tag-keys?measurement=<m>` | `store.read` | `schema.measurementTagKeys()` | `SHOW TAG KEYS FROM` |
| D3 | `GET /influxdb/{agent_name}/tag-values?measurement=<m>&tag_key=<k>` | `store.read` | `schema.measurementTagValues()` | `SHOW TAG VALUES ... WITH KEY =` |
| D4 | `GET /influxdb/{agent_name}/field-keys?measurement=<m>` | `store.read` | `schema.measurementFieldKeys()` | `SHOW FIELD KEYS FROM` |

디스커버리 응답은 **캐시하지 않는다**(`staleTime: 0`). 스키마는 쓰기에 따라 계속 변하므로, 캐시된 목록은 방금 만든 measurement 를 감춘다.

---

## 3. memTSDB 는 패널 데이터소스가 아니다

`internal/tsdb/` 와 `/api/v1/tsdb/*` 로 대표되는 **memTSDB** 는 위 비교 표에 등장하지 않는다. 이것은 누락이 아니라 결정이다.

### 3.1 아닌 이유

**(a) 사용자가 고르는 대상이 아니다.** memTSDB 는 플로우 노드(`tsdb-write` · `tsdb-query`)와 WS 구독자를 위한 **프로세스 내 임시 시계열 버퍼**다. "어느 DB 에 저장했는가"를 사용자가 선택하는 축에 속하지 않는다.

**(b) HTTP 표면이 실행 중인 서버에 존재하지 않는다.** `NewTSDBHandler` 의 프로덕션 호출자가 0건이며, `cmd/xflowd/main.go` 의 `server.RegisterRoutes(...)` 블록에 `tsdbHandler.RegisterRoutes(g)` 가 **없다**. 따라서 `POST /api/v1/tsdb/query` 를 비롯한 6종 라우트는 런타임에 존재하지 않는다.

세 이름이 서로 무관한 대상을 가리키므로, 코드에서도 식별자를 분리했다.

| 축 | Store | TSDB (외부) | memTSDB |
|----|-------|-------------|---------|
| `ChartDataSourceKind` | `'store'` | `'tsdb'` | **없음** (패널 소스 아님) |
| `SeriesDataSourceKind` | `'store'` | `'tsdb'` | `'memtsdb'` |
| 패널 config 블록 | `store_source` | `tsdb_source` | **없음** |

`SeriesDataSourceKind` 의 memTSDB 값을 `'memtsdb'` 로 개명한 이유는 같은 문자열이 두 화면에서 반대 뜻을 갖는 상태를 없애기 위함이다. 이는 **식별자 개명이며 동작 변경이 아니다** — 에이전트 상세 화면의 Series 탭 동작은 무변경이다.

### 3.2 남아 있는 잔여 상태 (본 SPEC 범위 밖)

`/api/v1/tsdb/*` 6종 라우트가 미등록이라는 사실의 귀결로, 다음 네 지점이 **404 또는 도달 불가** 상태다.

| 지점 | 상태 |
|------|------|
| `internal/cli/tsdb.go` 의 `tsdb` 하위명령 | 라우트를 호출하나 404 를 받는다 |
| 에이전트 상세 화면의 Series 탭 | 동일 |
| `web/src/services/api/tsdb.ts` | 위 두 지점이 쓰는 클라이언트 |
| `web/src/hooks/useTsdb.ts` | 소비자 0건 — 사장 코드 |

**이는 SPEC-TSDB-002 가 만든 문제가 아니고 고칠 문제도 아니다.** SPEC-TSDB-002 §7 NQ4 가 이를 본 SPEC 범위 밖의 별도 이슈로 분리하고 사실만 기록했다. 처분 선택지는 둘이다 — (a) `main.go` 에 `tsdbHandler` 를 배선하거나, (b) 도달 불가 코드로 판정하고 위 네 지점을 함께 제거한다.

같은 이유로, memTSDB 어댑터(`queryTsdbMatrixPivoted`)의 기존 결함 3건도 그대로 남아 있다. 그중 `seriesFilters.fieldName` 미전달로 "첫 번째 유한 숫자 필드" 폴백이 걸리는 결함은 **조용한 오답**이므로 별도 SPEC 으로 승격할 가치가 있으나, 그 경로 자체가 현재 404 이므로 도달 불가다.

---

## 4. 참조

| 관심사 | 파일 |
|--------|------|
| 소스 종류 판정 · 능력 표 (정본) | `web/src/pages/dashboard/panels/charts/panelDataSource.ts` |
| TSDB 선택 UI · 능력 게이팅 | `web/src/pages/dashboard/TsdbSourceSection.tsx` |
| TSDB 어댑터 · 백엔드 파생 | `web/src/services/api/tsdbSource.ts` |
| Store 어댑터 | `web/src/services/api/store.ts` |
| 공용 피벗 기계 | `web/src/services/api/seriesMatrixPivot.ts` |
| 구조화 질의 핸들러 · 검증 · 가드 | `internal/api/handler/influxdb_series.go` |
| Flux / InfluxQL 생성 · 집계 · fill 매핑 | `internal/agent/system/influxdb_seriesquery.go` |
| 스키마 디스커버리 | `internal/agent/system/influxdb_schema.go` |
| v3 관리 조작 501 · `ListMeasurements` 예외 | `internal/agent/system/influxdb_v3.go` |
| 버킷 정렬 교차 검증 | `internal/api/handler/bucket_alignment_crosscheck_test.go` |
