---
id: SPEC-TSDB-003
title: TSDB 시리즈 열거 — measurement + 태그 집합으로 실재 시리즈를 고른다
version: 0.4.0
status: draft
created: 2026-08-23
updated: 2026-08-23
author: xtra
priority: high
domain: dashboard
related_specs:
  - SPEC-TSDB-002
  - SPEC-WEB-005
  - SPEC-WEB-006
  - SPEC-PANEL-SETTINGS-001
  - SPEC-CHART-002
  - SPEC-INFLUX-001
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-23 | xtra | 최초 작성. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 가 만든 TSDB 선택 UI 가 **태그 조합을 접어** 보여 주는 문제를 다룬다. 사용자 요구는 "시리즈는 Measurement 와 태그로 분류됨" 한 줄이며, 이를 **실재하는 시리즈를 열거해 고르게 한다**로 해석했다. 백엔드 비대칭(v3 는 `SHOW SERIES` 를 지원하지 않음 — 공식 문서 실측) 이 이 SPEC 의 하중 지지점이다. |
| 0.2.0 | 2026-08-23 | xtra | **초안 승인 + 열린 질문 3건 확정.** (1) SPEC 승인 — M1(실측 스파이크)부터 착수한다. (2) **OQ6 확정 — 로컬 편집 상태.** 탐색 창을 `tsdb_source` 에 영속하지 않는다; §2.10 형상 무변경과 AC-28·AC-29·AC-30 이 그대로 유지된다. (3) **OQ8 확정 — 대체 선언한다.** 본 SPEC 이 SPEC-TSDB-002 §2.15 [O1] 의 3단 드릴다운 항목을 대체한다(§2.15 [S3]). 나머지 OQ1·OQ3·OQ4·OQ5·OQ7·OQ9·OQ10 은 **잠정안 그대로 진행**하며 M1 종료 시 실측 근거와 함께 최종 확정한다. OQ2 는 여전히 미확정이며 M1 이 닫는다. 요구사항 변경 없음 — 확정만 기록한다. |
| 0.3.0 | 2026-08-23 | xtra | **M2 구현 회차 — 사실 정정 1건 + OQ5 확정.** M1(실측 스파이크)은 실측 대상 InfluxDB 인스턴스가 없어 **보류**했고, M1→M2 가 연성 의존이므로 M2(v2 Flux 열거)를 선행했다. M3(v3)은 M1 과 함께 보류 상태다. §HISTORY-0.3.0 참조. |
| 0.4.0 | 2026-08-23 | xtra | **M1 실측 부분 수행 — v2 축 확정, v3 축 미해결.** 사용자가 실 InfluxDB 2.x 인스턴스에서 `BuildFluxSeriesEnumQuery` 생성 질의를 직접 실행해 주석 CSV 원문을 관측했다. **M2 가 가정으로 세운 3건이 전부 사실로 확인**되었고, 문서로는 알 수 없던 사실 1건(점이 든 태그 키)이 추가로 드러났다. 관측 데이터를 회귀 테스트로 잠갔다. OQ2(v3 대체 경로)는 **여전히 미확정** — 관측된 인스턴스가 v2 이므로 M3 은 계속 보류다. §HISTORY-0.4.0 참조. |

---

### HISTORY-0.4.0 — M1 실측(v2 축) 결과

`from(bucket:) |> range() |> filter(_measurement) |> first() |> group() |> limit()` 를 실 InfluxDB 2.x
인스턴스(measurement `temperature`, LoRaWAN 센서 12대)에 실행하고 주석 CSV 를 관측했다.

**(1) M2 의 가정 3건이 전부 사실로 확인되었다 — 코드 변경 없음.**

| # | M2 의 가정 | 관측 결과 |
|---|-----------|----------|
| 1 | `result` · `table` 구조 컬럼이 실재하므로 제외해야 한다 | **확인.** 헤더가 `,result,table,_start,_stop,_time,_value,_field,_measurement,...` 로 온다. 제외하지 않았다면 12개 시리즈 전부에 `result:_result` · `table:0` 이 태그로 붙었다 |
| 2 | `group()` 이 서로 다른 태그 집합을 한 테이블로 합치며 없는 컬럼은 **빈 문자열**로 채워진다 | **확인.** 전 행이 `table=0` 이고, `point` · `spot` 태그가 없는 2개 디바이스의 해당 컬럼이 빈 문자열로 왔다. `null` 도 아니고 별도 result 섹션도 아니다 |
| 3 | `_field` 가 컬럼으로 존재한다 | **확인.** `_field=value` |

가정 2 의 의미론도 함께 닫혔다. InfluxDB 는 빈 태그 값을 저장하지 않으므로 `point=""` 는
"그 시리즈에 `point` 태그가 없다"는 뜻이며, M2 의 **"빈 값 = 태그 아님"** 규칙이 옳다.
`AM103-089152` 와 `EM320-TH-389818` 은 태그 5개, 나머지 10대는 7개로 접힌다.

**(2) 새 사실 — 태그 키에 점(`.`)이 들어간다.**

관측된 태그 키는 `device.dev_eui` · `device.id` · `device.name` · `device.type` · `location` ·
`point` · `spot` 이다. 앞 4개가 점을 포함한다. 이는 문서로는 예측할 수 없던 실측 소득이다.

두 지점에서 문제가 없음을 확인했다 — 질의 생성이 `r.tagkey` 점 표기가 아니라
`r["tagkey"]` **대괄호 표기**를 쓰므로 점이 필드 접근으로 오해되지 않고, `serializeTagSet` 은
`\` · `,` · `=` 만 이스케이프하므로 점이 구분자와 충돌하지 않는다. 우연히 옳았던 것이 아니라
두 선택 모두 기존 `BuildFluxSeriesQuery` 규율을 따른 결과다.

**(3) 관측 데이터를 회귀 테스트로 잠갔다.**

`TestEnumerateSeries_실측CSV_회귀`(`influxdb_seriesenum_test.go`)가 관측된 헤더 + 12행을
그대로 픽스처로 쓴다. 주석 3줄(`#datatype` · `#group` · `#default`)만 컬럼 타입에서 재구성했고
데이터 행은 손대지 않았다. 단언: 시리즈 12개 · 구조/내부 컬럼이 태그로 새지 않음 ·
점이 든 키 보존 · 빈 값 태그 제외(7개 vs 5개).

**(4) 남은 갭 — OQ2 는 닫히지 않았다.**

관측 대상이 **v2 인스턴스**였다. v3 의 `SHOW SERIES` 미지원 확인(P1)과 `GROUP BY *` 의
행 형상(P5 — OQ2 의 정본)은 **관측되지 않았다.** 따라서 M3(v3 열거)은 계속 보류이고,
M4(HTTP 라우트)는 M2·M3 경성 의존이므로 함께 대기한다. 프로브 지시서는
`m1-probe.md` 에 있다.

---

### HISTORY-0.3.0 — M2 구현 회차

**(1) §2.4 의 "밑줄로 시작하지 않는 모든 컬럼 → 태그" 는 문언 그대로 구현하면 오답이다.**

정본이 지목한 선례 `filterInternalTagKeys`(`influxdb_schema.go:202`)는 밑줄 접두만 거른다. 그러나 그 함수가 받는 입력은 `schema.measurementTagKeys()` 가 **이미 걸러 낸 태그 키 목록**인 반면, 열거 경로는 `queryFlux` 가 돌려주는 `FluxRecord.Values()` — 즉 주석 CSV 의 **원시 컬럼 전체** — 를 읽는다. Flux 의 구조 컬럼 `result` 와 `table` 은 밑줄로 시작하지 않으므로 문언대로 구현하면 **모든 시리즈에 `result:_result` · `table:0` 이라는 존재하지 않는 태그가 붙는다.**

이는 §4.2 가 "데카르트 곱은 비용 문제 이전에 오답"이라고 규정한 것과 같은 부류의 조용한 오답이다. 구현은 두 컬럼을 명시적으로 제외했고 전용 테스트로 잠갔다. **요구사항은 바뀌지 않는다** — 정본이 의도한 것은 "태그가 아닌 컬럼을 배제한다"이며, 어긋난 것은 그 의도를 밑줄 규칙 하나로만 서술한 산문이다. §2.4 의 해당 항목을 정정했다.

부수적으로, 빈 문자열 태그 값의 처분이 §2.4 에 없었다. `group()` 이 서로 다른 태그 집합의 테이블을 합칠 때 없는 컬럼이 빈 값으로 채워질 수 있고 InfluxDB 는 빈 태그 값을 저장하지 않으므로, 구현은 이를 **태그 아님**으로 처리했다. §2.4 에 명시했다.

**(2) OQ5 확정 — `schema.*` 3종의 `start` 기본값이 모두 `-30d` 로 일치한다.**

§6 가정 5 가 "`measurementTagKeys` 만 확인했고 나머지 둘은 같은 계열이라 동일하다고 가정한다"로 남겨 둔 부분을 공식 문서로 닫았다. 세 문서 모두 `Default is -30d` 를 명시한다.

| 함수 | 문서 |
|------|------|
| `schema.measurementTagKeys` | https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/measurementtagkeys/ |
| `schema.measurementTagValues` | https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/measurementtagvalues/ |
| `schema.measurementFieldKeys` | https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/measurementfieldkeys/ |

따라서 탐색 창 기본값 30일(OQ5)은 **현상 유지**라는 §4.6 의 논거가 3종 전부에 대해 성립한다.

**(3) M1 보류 사유와 그 결과.**

M1.1 은 "실측 대상 인스턴스가 없으면 사용자 결정을 요청하고 M1 을 중단한다"고 규정한다. 환경 확인 결과 실행 중인 influxd 프로세스 0건, 8086/8181 리스닝 0건, `docker-compose*.yml` 부재, Docker 데몬 미기동, `~/.xflow/xflow.yaml` 의 influxdb 에이전트 0건이었다. 사용자 결정에 따라 **M2 선행**을 택했다.

따라서 **§2.4 의 동작 원리는 여전히 문서 근거이지 실측 근거가 아니다.** 특히 `group()` 이 서로 다른 태그 집합의 테이블을 합칠 때의 실제 컬럼 형상(빈 문자열 채움 / null / 별도 result 섹션)은 관측되지 않았다. M2 의 테스트는 `httptest` 인프로세스 서버 왕복까지만 덮는다. 이 갭은 M4 통합 또는 실서버 확보 시 닫는다.

**(4) M2 산출물.**

`internal/agent/system/influxdb_seriesenum.go` + `_test.go` 2개 파일만 추가했고 기존 파일 수정은 0건이다. 신규 파일 커버리지 106/106 statements = **100.0%**. `InfluxSchemaDiscoverer` 는 넓히지 않고 `InfluxSeriesEnumerator` 를 별도 인터페이스로 두었다(plan §4 R5). v3 열거 코드는 0줄이며 v3 경로는 `ErrSeriesEnumerationNotSupported` 를 반환한다.

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 두 줄이다.

```
데이터 소스 - TSDB
- 시리즈는 Measurement와 태그로 분류됨
```

이것은 **선택 UI 개선 요구**다. 데이터 모델은 이미 이 축을 갖고 있고, 질의도 이미 태그를 전달한다. 없는 것은 **열거(enumeration)** 뿐이다 — 사용자가 "존재하는 시리즈 중에서" 고를 수 없다.

본 SPEC 은 다음 하나를 만든다.

> **(measurement, 시간창, 선택적 태그 사전 필터)** 를 주면 **그 구간에 실제로 기록이 있는 태그 집합의 목록**을 돌려주는 디스커버리 능력(D5)과, 그것을 소비하는 선택 UI.

### 1.2 배경

#### 1.2.1 데이터 모델은 이미 축을 갖고 있다 — 빠진 것은 열거다

| 축 | 상태 | 근거 |
|----|------|------|
| 시리즈 식별자에 태그가 있는가 | **있다** | `TsdbSeriesRef = { key, field, tags? }` — `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts:217` |
| 시리즈 ID 가 태그를 포함하는가 | **한다** | `storeSeriesId(key, field, tags)` — `chartChannelTypes.ts:473` |
| 질의가 태그를 백엔드로 보내는가 | **보낸다** | 요청 바디 조립 — `web/src/services/api/tsdbSource.ts:207-217`(태그는 `:211`) |
| 백엔드가 태그를 술어로 번역하는가 | **한다** | `BuildFluxSeriesQuery` · `BuildInfluxQLSeriesQuery` — `internal/agent/system/influxdb_seriesquery.go:335` · `:393` |
| **실재 태그 조합을 열거하는 능력** | **없다** | 아래 §1.2.3 |

**따라서 이 SPEC 은 표현(representation)이 아니라 열거(enumeration)의 공백을 메운다.** `TsdbSourceConfig` · `TsdbSeriesRef` 의 형상은 **한 글자도 바뀌지 않는다.** 이것이 하위 호환의 구조적 근거다(§2.10 [U9]).

#### 1.2.2 현재 선택 UI 는 태그 조합을 접는다

`web/src/pages/dashboard/TsdbSourceSection.tsx:268-278` 가 후보 행을 만든다.

```ts
const rows: SeriesRow[] = useMemo(() => {
  if (measurement === '') return [];
  return fieldKeys.items.map((field) => ({
    id: storeSeriesId(measurement, field, tagFilters),
    key: measurement,
    field,
    dataType: '',
    registration: '',
    tags: tagFilters,          // ← 모든 행이 같은 태그 집합을 공유한다
  }));
}, [measurement, fieldKeys.items, tagFilters]);
```

`tagFilters`(`:222`)는 드릴다운 3단계가 채우는 **한 벌의** 태그 맵이다. 따라서 **행 = 현재 measurement 의 field 키 × 태그 1벌**이며, `host=a` 와 `host=b` 가 두 행으로 나타나는 일이 없다.

같은 파일 `:261-267` 의 주석이 이 결정의 근거를 이미 적어 두었다.

> 태그를 드릴다운의 **3단계**로 두었으므로 행의 태그는 전부 같다. 태그별로 행을 쪼개려면 `tag-values` 를 조합 폭발로 순회해야 하는데, 그 비용은 InfluxDB 쪽에서 온전히 발생한다(§5). 사용자가 태그를 좁힌 뒤 필드를 고르는 순서가 실제 조작 순서와도 맞는다.

**이 주석이 지목한 대안(태그 키 × 값의 조합 순회)은 본 SPEC 도 채택하지 않는다.** 그 방식은 비용이 문제이기 이전에 **틀린다** — 데카르트 곱은 실제로 기록된 적 없는 조합까지 만들어 내며, 그렇게 고른 시리즈는 조회 시 전 버킷 `null` 이 된다(§4.2). 본 SPEC 이 채택하는 것은 **백엔드에게 실재 조합을 물어보는** 경로다.

#### 1.2.3 디스커버리 표면은 현재 D1~D4 뿐이다

[SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.10 이 만든 4종이 전부이며, 전부 `store.read` 로 등록되어 있다(`internal/api/handler/influxdb_management.go:85` · `:90-92`).

| ID | 라우트 | v2 (Flux) | v3 (InfluxQL) | 구현 |
|----|--------|-----------|---------------|------|
| D1 | `GET /influxdb/{agent_name}/measurements` | `schema.measurements()` | `SHOW MEASUREMENTS` | `influxdb_v2.go` · `influxdb_v3.go:195` |
| D2 | `GET /influxdb/{agent_name}/tag-keys?measurement=` | `schema.measurementTagKeys()` | `SHOW TAG KEYS FROM` | `internal/agent/system/influxdb_schema.go:131` · `:288` |
| D3 | `GET /influxdb/{agent_name}/tag-values?measurement=&tag_key=` | `schema.measurementTagValues()` | `SHOW TAG VALUES ... WITH KEY =` | 동일 파일 |
| D4 | `GET /influxdb/{agent_name}/field-keys?measurement=` | `schema.measurementFieldKeys()` | `SHOW FIELD KEYS FROM` | 동일 파일 |

**시리즈(태그 집합) 열거 라우트는 없다.** 트리 전체에서 확인했다.

#### 1.2.4 v3 는 `SHOW SERIES` 를 지원하지 않는다 — 공식 문서 실측

이것이 본 SPEC 에서 가장 먼저 확정해야 했던 사실이다. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §HISTORY-0.4.0 이 기록했듯, v3 의 능력에 대한 추정은 이미 한 번 틀린 적이 있다(D1 의 v3 501 을 "지원 불가"로 읽었으나 `SHOW MEASUREMENTS` 로 실제 동작했고, 커밋 `1204abe5` 가 이를 해제했다). 따라서 추정하지 않고 문서를 확인했다.

| 문서 | InfluxQL 메타쿼리 지원 |
|------|------------------------|
| [InfluxDB 3 Core — InfluxQL feature support](https://docs.influxdata.com/influxdb3/core/reference/influxql/feature-support/) | 지원: `SHOW DATABASES` · `SHOW RETENTION POLICIES` · `SHOW MEASUREMENTS` · `SHOW TAG KEYS` · `SHOW TAG VALUES` · `SHOW FIELD KEYS`. **미지원: `SHOW SERIES` · `SHOW SERIES CARDINALITY` · `SHOW TAG KEY CARDINALITY` · `SHOW TAG VALUES CARDINALITY` · `SHOW FIELD KEYS CARDINALITY`** |
| [InfluxDB Cloud Dedicated — InfluxQL feature support](https://docs.influxdata.com/influxdb3/cloud-dedicated/reference/influxql/feature-support/) | 동일. **미지원: `SHOW SERIES` · `SHOW SERIES CARDINALITY`** |

두 문서 모두 미지원 사유를 같은 취지로 적는다 — v3 스토리지 엔진에서 시리즈 카디널리티가 더 이상 성능 제약이 아니므로 카디널리티 계열 메타쿼리는 앞으로도 지원되지 않을 가능성이 높다.

**따라서 v3 에는 대체 경로가 필요하며, 그 대체 경로의 선택이 이 SPEC 의 주요 설계 결정이다**(§2.5 [U4] · §4.3).

#### 1.2.5 v2 에도 `schema.series()` 는 없다 — 그러나 그룹 키가 있다

[Flux `schema` 패키지 문서](https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/)가 나열하는 함수는 8개다 — `fieldKeys` · `fieldsAsCols` · `measurementFieldKeys` · `measurements` · `measurementTagKeys` · `measurementTagValues` · `tagKeys` · `tagValues`. **시리즈(measurement + 태그 집합)를 열거하는 함수는 없다.** [InfluxDB v2 스키마 탐색 가이드](https://docs.influxdata.com/influxdb/v2/query-data/flux/explore-schema/)도 시리즈 열거 방법을 문서화하지 않는다.

그러나 Flux 에는 **그룹 키**가 있다. `from |> range |> filter` 이후 스트림의 기본 그룹 키는 `[_start, _stop, _measurement, _field, <모든 태그>]` 이므로 **테이블 1개 = 시리즈 1개**다. 각 테이블을 1행으로 줄인 뒤 그룹을 풀면, 결과 행 집합이 곧 (field, 태그 집합) 쌍의 목록이 된다.

이 경로가 실현 가능한 이유는 두 가지가 이미 트리에 있기 때문이다.

1. **행이 전 컬럼을 통과시킨다.** `internal/agent/system/influxdb_v2.go:83-103` 의 `queryFlux` 는 `result.Record().Values()`(`:93`)를 그대로 맵에 담는다. 태그 컬럼이 버려지지 않는다.
2. **축약 함수가 푸시다운 가능하다.** [InfluxDB v2 쿼리 최적화 문서](https://docs.influxdata.com/influxdb/v2/query-data/optimize-queries/)의 푸시다운 표는 `first()` 를 푸시다운 가능 함수로, `group() |> first()` 를 푸시다운 가능 조합으로 나열한다. `distinct()` 는 그 표에 없다.

#### 1.2.6 v3 의 SQL 경로는 **HTTP 화이트리스트에 막혀 있지 않다**

[SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §1.2.9 는 "v3 SQL 은 HTTP 로 도달할 수 없다"고 적었다. 그 진술은 **원문 통과 라우트에 한정된 사실**이다.

| 계층 | SQL 도달 가능성 | 근거 |
|------|-----------------|------|
| 원문 통과 핸들러 `POST /influxdb/{agent}/query` | **불가** | `internal/api/handler/influxdb_query.go:74-80` 이 `flux`/`influxql` 만 허용 |
| v3 클라이언트 | **가능** | `internal/agent/system/influxdb_v3.go:76-88` 의 `Query` 가 `case "sql": return c.querySQL(...)`; `querySQL` 은 `:90-97` |

**본 SPEC 의 D5 는 원문 통과 핸들러를 지나지 않는다.** 새 핸들러가 에이전트 메서드를 직접 부르고, 에이전트가 클라이언트를 부른다 — D2~D4 가 이미 그렇게 한다(`influxdb_schema.go:41-45` 의 `InfluxSchemaDiscoverer`). 따라서 **화이트리스트를 건드리지 않고도 v3 에서 SQL 을 쓸 수 있다.** 이는 §1.3 의 "원문 통과 경로 무변경"과 충돌하지 않는다.

#### 1.2.7 카디널리티는 UI 쪽에서 먼저 터진다

| 사실 | 값 | 근거 |
|------|-----|------|
| 시리즈 **선택** 상한 | 48 | `chartChannelTypes.ts:552` `STORE_SERIES_LIMIT = 48`; 강제는 `TsdbSourceSection.tsx:286-288` |
| TSDB 는 **시리즈당 1요청**을 폴링마다 보낸다 | 최대 48요청/폴링 | [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.19 · §5 |
| 선택 표가 행을 **가상화하지 않는다** | 전 행 렌더 | `web/src/pages/agents/SeriesSelectTable.tsx:356` `filteredRows.map(...)` — 상한도 슬라이스도 없다 |
| 선택 표에 **검색 + 컬럼 facet 필터가 이미 있다** | 있다 | `SeriesSelectTable.tsx:14`(`Search` 아이콘 import) · `:40`(`FacetColumn = 'field' \| 'dataType' \| 'tags' \| 'registration'`) · `:134`(`filteredRows`) |
| 버킷 수 서버 상한 | 100,000 | `internal/api/handler/store_query.go:24` `maxAggregationBuckets`; TSDB 재사용은 `influxdb_series.go:223-241` |

**세 번째와 네 번째 줄이 함께 이 SPEC 의 카디널리티 대응을 결정한다.** 필터 UI 는 이미 있으므로 새로 만들 것이 없고, 대신 **행 수 자체에 상한이 필요하다** — 수천 행을 반환하면 필터가 있어도 DOM 이 먼저 무너진다(§2.7 [U6]).

#### 1.2.8 현재 디스커버리에는 **보이지 않는 30일 창**이 이미 있다

[Flux `schema.measurementTagKeys()` 문서](https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/measurementtagkeys/)는 선택 인자 `start` 의 기본값을 **`-30d`**, `stop` 의 기본값을 `now()` 로 명시한다.

`internal/agent/system/influxdb_schema.go:131-145` 등 본 트리의 v2 디스커버리 쿼리는 `start` 를 **지정하지 않는다.** 즉 **v2 의 D2~D4 는 이미 최근 30일 창으로 동작하고 있으며, 그 사실이 UI 어디에도 드러나지 않는다.**

이는 §2.8 [U7] 의 근거다 — 시간창 의존은 본 SPEC 이 새로 도입하는 성질이 아니라 **이미 존재하는데 감춰져 있던 성질**이며, 본 SPEC 은 그것을 명시적 · 사용자 가시적으로 만든다.

#### 1.2.9 능력 게이팅 기계가 이미 있다

`web/src/pages/dashboard/panels/charts/panelDataSource.ts` 가 두 축을 갖는다.

| 자산 | 위치 | 성질 |
|------|------|------|
| `SourceCapabilities` + `PANEL_SOURCE_CAPABILITIES` | `:156` · `:183` | `Record<ChartDataSourceKind, ...>` — 컴파일러가 3종 전수성 강제 |
| `InfluxBackendVersion = '2' \| '3'` | `:247` | 백엔드 하위 축 |
| `TsdbBackendCapabilities` + `TSDB_BACKEND_CAPABILITIES` | `:252` · `:278-284` | `Record<InfluxBackendVersion, ...>` — 버전이 늘면 컴파일이 먼저 깨진다 |
| `resolveInfluxVersion(config)` | `:293` | 판독 불가 시 `'2'`(넓은 쪽) |
| `CAPABILITY_REASON_KEYS` | `:308-317` | 미지원 사유 i18n 키를 능력과 **같은 모듈**에 둔다 |

**본 SPEC 의 백엔드 비대칭은 이 기계에 항목을 더해서 표현하며, 새 기계를 만들지 않는다**(§2.6 [U5]).

### 1.3 비범위 (Out of Scope)

- **`TsdbSourceConfig` · `TsdbSeriesRef` 의 형상 변경** — 열거는 선택 UI 가 제시하는 후보를 바꿀 뿐 저장 형상을 바꾸지 않는다(§2.10 [U9])
- **`POST /influxdb/{agent}/series/query` 구조화 질의 경로의 변경** — 요청/응답 계약 무변경
- **`POST /influxdb/{agent}/query` 원문 통과 경로의 변경** — 언어 화이트리스트(`influxdb_query.go:74-80`) 해제 포함. D5 는 그 핸들러를 지나지 않는다(§1.2.6)
- **시리즈당 1요청 팬아웃의 배치화** — [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §7 NQ2 가 소유. 본 SPEC 은 선택 상한 48 을 그대로 승계한다
- **Store 데이터소스의 선택 UI 변경** — Store 는 `selection_mode: 'tag'` 라는 다른 축을 갖는다. 본 SPEC 은 TSDB 소스에만 적용된다
- **태그 기반 **동적** 바인딩(`selection_mode: 'tag'`)의 TSDB 도입** — [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) OQ6 이 "제공하지 않음"으로 확정했다. 본 SPEC 의 태그 사전 필터는 **선택 시점의 좁히기**이지 런타임 동적 해석이 아니다
- **v3 의 bucket/measurement 관리 조작(생성 · 삭제 · truncate) 501 해제** — 무변경
- **디스커버리 응답 캐시 도입** — 금지 유지(§2.9 [U8])
- **선택 표의 가상화(virtualization)** — `SeriesSelectTable` 은 여러 화면이 공유하므로 렌더 전략 변경은 별도 SPEC 이다. 본 SPEC 은 **행 수 상한**으로 대응한다(§4.5)
- **Prometheus 등 InfluxDB 외 백엔드** — 판별자는 확장 가능하게 두되 구현하지 않는다

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 시리즈와 그 두 축의 어휘

시스템은 **시리즈**를 다음으로 정의하고, 이 정의를 열거 · 선택 · 질의 세 지점에서 동일하게 쓴다.

> **시리즈 = (measurement, 태그 집합)** — InfluxDB 자신의 정의.
> **패널이 고르는 단위 = (measurement, field, 태그 집합)** — 여기에 값 축(field)이 하나 더 붙는다.

두 정의가 다르다는 사실이 이 SPEC 의 백엔드 비대칭을 만든다(§2.6). 시스템은 둘을 구분해 다음 어휘를 쓴다.

| 어휘 | 뜻 | 대응 |
|------|-----|------|
| **태그 집합(tag set)** | 한 시리즈의 태그 키/값 전체 | InfluxDB 의 시리즈 |
| **시리즈 행(series row)** | 사용자가 체크박스로 고르는 한 줄 | `TsdbSeriesRef{key, field, tags}` · `SeriesRow`(`SeriesSelectTable.tsx:24`) |
| **열거(enumeration)** | 주어진 (measurement, 창, 사전 필터)에서 **실재하는** 태그 집합을 얻는 행위 | 본 SPEC 의 D5 |

시스템은 **열거되지 않은 조합을 후보로 제시해서는 안 된다** — 단, v3 에서 field 축이 근사임을 §2.6 이 명시적으로 허용하는 경우는 예외이며, 그 예외는 사용자에게 표시된다(§2.13 [S2]).

### 2.2 [U2] (Ubiquitous) 시리즈 열거 라우트 (D5)

시스템은 디스커버리 5번째 항목을 제공한다.

```
GET /api/v1/influxdb/{agent_name}/series      권한: store.read
```

**질의 파라미터**

| 이름 | 필수 | 의미 |
|------|------|------|
| `measurement` | **필수** | 열거 대상. 빈 값이면 400 |
| `bucket` | 선택 | v2 = bucket, v3 = database(무시됨 — §2.6). 빈 값이면 에이전트 기본값 |
| `start_ms` | 선택 | 탐색 창 시작(포함). 미지정 시 §2.8 의 기본값 |
| `end_ms` | 선택 | 탐색 창 끝(미포함). 미지정 시 `now()` |
| `tags` | 선택 | 사전 필터. `k1=v1,k2=v2` 형식. 지정된 키/값에 일치하는 시리즈만 열거한다 |
| `limit` | 선택 | 반환할 태그 집합 수 상한. 미지정 · 상한 초과 시 서버 상한(§2.7)으로 절삭 |

**응답** — 기존 디스커버리 4종과 같은 `dto.NewSuccessResponse` 봉투를 쓴다(`influxdb_management.go:418-444` 선례).

```json
{
  "series": [
    { "tags": { "host": "a", "region": "kr" }, "fields": ["usage", "idle"] },
    { "tags": { "host": "b", "region": "kr" }, "fields": ["usage"] }
  ],
  "field_exact": true,
  "count": 2,
  "truncated": false,
  "window": { "start_ms": 1700000000000, "end_ms": 1700086400000 }
}
```

| 필드 | 의미 |
|------|------|
| `series[].tags` | 실재하는 태그 집합 1벌. 키 없는 시리즈는 `{}` |
| `series[].fields` | 그 태그 집합에서 관측된 field 키 목록 |
| `field_exact` | `fields` 가 **정확한 관측치**인지(`true`) **measurement 전체 field 목록의 근사**인지(`false`). §2.6 |
| `count` | `series` 의 길이 |
| `truncated` | 상한에 걸려 잘렸는가(§2.7) |
| `window` | 서버가 **실제로 사용한** 창. 요청이 생략했을 때 무엇이 적용됐는지 드러낸다(§2.8) |

**시스템은 `series` 를 결정적으로 정렬해 반환한다** — 태그를 키 사전순으로 직렬화한 문자열의 오름차순. 절단이 발생할 때 폴링마다 다른 부분집합이 잘리면 사용자가 고른 시리즈가 목록에서 사라졌다 나타났다 한다.

**라우트 배치.** D5 는 D1~D4 와 같은 `InfluxDBManagementHandler`(`influxdb_management.go:78-92`)에 등록한다. 그 핸들러가 이미 `resolveDiscoverer`(`:97`) · `mapInfluxDiscoveryError`(`:345`) · `defaultInfluxManagementTimeout`(`:43`, 60초)을 갖고 있어 새 배선이 필요 없다. `POST /series/query`(`influxdb_series.go:65`)와 경로 접두사가 겹치나 메서드가 다르므로 충돌하지 않는다(OQ1).

**오류 매핑** — 기존 디스커버리와 동일하다.

| 조건 | 응답 |
|------|------|
| `measurement` 빈 값 | 400 |
| `tags` 파싱 불가 | 400 |
| 이스케이프 불가 식별자 | 400 (`system.ErrUnescapableIdentifier` — `influxdb_management.go:349`) |
| `end_ms <= start_ms` | 400 |
| 에이전트 없음 | 404 |
| 에이전트가 InfluxDB 가 아님 | 400 |
| 조회 타임아웃 | 408 |

### 2.3 [U3] (Ubiquitous) 열거는 순수 쿼리 생성 함수로 분리한다

시스템은 v2/v3 열거 쿼리의 문자열 조립을 **네트워크 없이 전수 테스트 가능한 순수 함수**로 분리한다. 이는 `BuildFluxSeriesQuery`(`influxdb_seriesquery.go:335`) · `buildFluxTagKeysQuery`(`influxdb_schema.go:131`)가 이미 따르는 규율이며, 본 SPEC 은 새 규율을 만들지 않는다.

시스템은 식별자 검증 · 이스케이프에 **기존 헬퍼를 재사용한다.**

| 헬퍼 | 위치 | 용도 |
|------|------|------|
| `validateSeriesIdentifier` | `influxdb_seriesquery.go:188` | 제어문자 · 비 UTF-8 거부 |
| `escapeFluxStringLiteral` | `influxdb_seriesquery.go:245` | Flux 문자열 리터럴 (`${` 보간 차단) |
| `escapeInfluxQLIdent` | `influxdb_seriesquery.go:275` | InfluxQL 식별자 |
| `ErrUnescapableIdentifier` | `influxdb_seriesquery.go:67` | 400 매핑의 sentinel |

시스템은 이스케이프 없이 measurement · 태그 키 · 태그 값을 쿼리 문자열에 삽입해서는 안 된다.

### 2.4 [U4] (Ubiquitous) v2 열거 — 그룹 키에서 시리즈를 도출한다

시스템은 v2(Flux)에서 다음 형상의 쿼리로 열거한다.

```flux
from(bucket: "<bucket>")
  |> range(start: time(v: <startNs>), stop: time(v: <endNs>))
  |> filter(fn: (r) => r._measurement == "<m>")
  |> filter(fn: (r) => r["<tagk>"] == "<tagv>")     // 사전 필터마다 1행
  |> first()
  |> group()
  |> limit(n: <rowCap>)
```

동작 원리는 §1.2.5 다 — `filter` 이후 그룹 키가 `[_start, _stop, _measurement, _field, <태그>]` 이므로 테이블 1개가 시리즈 1개이고, `first()` 가 각 테이블을 1행으로 줄이며, `group()` 이 그것을 한 테이블로 모은다.

서버는 반환된 행에서 다음을 읽는다.

- `_field` → field 이름
- **밑줄로 시작하지 않는 컬럼 중 Flux 구조 컬럼 `result` · `table` 을 제외한 모든 컬럼** → 태그 키/값

밑줄 접두 필터는 신규 규칙이 아니다 — `filterInternalTagKeys`(`influxdb_schema.go:184-195`)가 D2 에서 같은 판정을 이미 한다. 다만 **밑줄 규칙만으로는 부족하다**(§HISTORY-0.3.0 (1) 참조): `filterInternalTagKeys` 가 받는 입력은 `schema.measurementTagKeys()` 가 이미 걸러 낸 태그 키 목록이라 구조 컬럼이 섞이지 않지만, 열거 경로는 **레코드의 원시 컬럼**을 읽으므로 주석 CSV 의 `result` · `table` 이 그대로 들어온다. 두 이름은 밑줄로 시작하지 않으므로 문언 그대로 구현하면 모든 시리즈에 존재하지 않는 태그가 붙는다 — §4.2 가 막으려는 "조용한 오답"과 같은 부류다.

빈 문자열 태그 값은 **태그가 아닌 것**으로 처리한다. InfluxDB 는 빈 태그 값을 저장하지 않으며, `group()` 이 서로 다른 태그 집합의 테이블을 한 테이블로 합칠 때 없는 컬럼이 빈 값으로 채워질 수 있기 때문이다.

그런 다음 서버는 **(field, 태그 집합) 행들을 태그 집합 기준으로 접어** `series[].fields` 를 만든다. 이 경로에서 `field_exact` 는 **`true`** 다 — 각 태그 집합의 field 목록이 관측치이기 때문이다.

**`distinct()` 를 쓰지 않는다.** [푸시다운 표](https://docs.influxdata.com/influxdb/v2/query-data/optimize-queries/)에 `first()` 와 `group() |> first()` 는 있으나 `distinct()` 는 없다. 축약이 스토리지 계층으로 내려가지 않으면 열거 비용이 창 안의 전체 포인트 수에 비례한다.

### 2.5 [U5] (Ubiquitous) v3 열거 — `SHOW SERIES` 가 없으므로 대체 경로를 쓴다

**시스템은 v3 에서 `SHOW SERIES` 를 발행해서는 안 된다.** 공식 문서 두 곳이 미지원을 명시한다(§1.2.4). 발행하면 런타임 오류가 되고, 그 오류는 "디스커버리 실패"로만 보여 사용자가 원인을 알 수 없다.

시스템은 v3 에서 다음 3단 경로로 열거한다.

| 단계 | 쿼리 | 지원 근거 |
|------|------|-----------|
| 1 | `SHOW TAG KEYS FROM "<m>"` (InfluxQL) | 공식 문서 지원 목록 |
| 2 | `SELECT DISTINCT "<tk1>", "<tk2>", ... FROM "<m>" WHERE time >= '<startRFC3339Nano>' AND time < '<endRFC3339Nano>' [AND "<tk>" = '<tv>'] LIMIT <rowCap>` (SQL) | 클라이언트 SQL 경로 존재(`influxdb_v3.go:76-97`); 화이트리스트 무관(§1.2.6) |
| 3 | `SHOW FIELD KEYS FROM "<m>"` (InfluxQL) | 공식 문서 지원 목록 |

3단계의 결과는 **measurement 전체의 field 목록**이며, 모든 태그 집합에 동일하게 부여된다. 따라서 이 경로에서 `field_exact` 는 **`false`** 다.

태그 키가 0개면 2단계를 건너뛰고 `series: [{ tags: {}, fields: [...] }]` 한 항목을 반환한다 — 태그 없는 measurement 도 시리즈 1개다.

**대안과 그 처분은 §4.3 에 있다.** 특히 InfluxQL `GROUP BY *`(공식 문서상 [지원됨](https://docs.influxdata.com/influxdb3/core/reference/influxql/group-by/) — "Groups data by all tags")을 쓰는 단일 쿼리 경로는 매력적이나, 그 결과에서 태그가 행 컬럼으로 드러나는지는 **문서로 확인되지 않는다**(OQ2). M1 이 실측한다.

### 2.6 [U6] (Ubiquitous) 백엔드 비대칭을 능력 표에 명시한다

시스템은 v2/v3 의 열거 능력 차이를 **숨기지 않고 능력 표에 항목으로 둔다.** 이는 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.13 이 세운 기계(`TSDB_BACKEND_CAPABILITIES` — `panelDataSource.ts:278-284`)에 필드를 더하는 방식이며, 새 축을 만들지 않는다.

```ts
export interface TsdbBackendCapabilities {
  bucketList: boolean;            // 기존
  bucketAffectsQuery: boolean;    // 기존
  management: boolean;            // 기존
  /** 시리즈(태그 집합) 열거를 지원하는가. */
  seriesEnumeration: boolean;                                  // 신규
  /** 열거 결과의 field 축이 정확한 관측치인가(false = measurement 전체 field 의 근사). */
  seriesFieldExact: boolean;                                   // 신규
}
```

| 능력 | v2 | v3 | 사유 |
|------|----|----|------|
| `seriesEnumeration` | `true` | `true` | 경로는 다르나 둘 다 가능(§2.4 · §2.5) |
| `seriesFieldExact` | **`true`** | **`false`** | v2 는 그룹 키에 `_field` 가 포함됨. v3 는 `SHOW FIELD KEYS` 가 measurement 단위라 태그 집합별로 쪼갤 수 없음 |

`seriesFieldExact` 가 거짓인 백엔드에서 시스템은 §2.13 [S2] 의 안내를 표시한다.

**시스템은 응답의 `field_exact` 값을 능력 표의 값과 독립적으로 신뢰한다.** 능력 표는 렌더 시점의 낙관적 표시값이고, 정본은 서버 응답이다 — [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.18 이 `backend` 필드에 대해 세운 것과 같은 규율이다.

### 2.7 [U7] (Ubiquitous) 카디널리티 상한과 절단 신호

시스템은 열거 결과에 **서버 상한**을 강제한다.

| 축 | 상한 | 강제 위치 | 근거 |
|----|------|-----------|------|
| 원시 행 수(쿼리 `limit`/`LIMIT`) | 20,000 | 생성된 쿼리 | 접기 전 행이므로 태그 집합 상한보다 커야 한다 |
| 반환 태그 집합 수 | **1,000** | 핸들러 | §1.2.7 — 선택 표가 전 행을 렌더한다(`SeriesSelectTable.tsx:356`) |
| 시리즈 **선택** 수 | 48 | 클라이언트 | 기존 `STORE_SERIES_LIMIT` 승계. 무변경 |

두 상한 중 어느 하나라도 걸리면 응답의 `truncated` 는 `true` 다.

**시스템은 절단을 조용히 수행해서는 안 된다.** UI 는 절단 배너를 표시하고 **좁히는 방법**을 함께 제시한다 — 태그 사전 필터를 걸거나 탐색 창을 줄이는 것. `too many series` 만으로는 사용자가 무엇을 해야 할지 알 수 없다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.9 가 버킷 상한에 대해 세운 것과 같은 원칙).

**시스템은 페이지네이션을 도입하지 않는다.** 근거는 §4.5 다.

**상한은 서버에 둔다.** 클라이언트 상한은 이미 InfluxDB 가 계산을 마친 뒤에야 작동하므로 백엔드 부하를 막지 못한다. 선택 상한 48 이 클라이언트에 있는 것과 다른 판단이며, 그 차이의 근거는 "48 은 요청 수를 제한하고 1,000 은 계산량을 제한한다"는 점이다.

### 2.8 [U8] (Ubiquitous) 탐색 시간창은 명시적이고 사용자에게 보인다

시스템은 열거에 사용할 시간창을 **명시적으로 지정하고, 실제로 사용한 창을 응답에 담아 UI 에 표시한다.**

| 축 | 규칙 |
|----|------|
| 기본값 | `start = now - 30d`, `end = now` |
| 기본값의 근거 | v2 의 `schema.*` 함수가 이미 `start: -30d` 를 기본으로 쓰며(§1.2.8), 현재 D2~D4 는 그 기본값에 의존하고 있다. 같은 값을 쓰면 **본 SPEC 이 창을 좁히거나 넓히지 않는다** |
| 사용자 제어 | 탐색 창은 선택 UI 의 컨트롤로 노출한다. 최소한 "패널 조회 창과 동일" 선택지를 포함한다 |
| 표시 | 열거 결과 근처에 사용된 창을 표시한다 |

**시스템은 "그 구간에 기록이 없어 목록에 없는 시리즈"의 존재를 사용자에게 알린다.** 이것이 시간창 의존의 유일한 실제 위험이다 — 사용자는 "시리즈가 삭제됐다"와 "탐색 창 밖이다"를 구분할 수 없고, 전자로 오해하면 잘못된 조치를 한다.

**패널 조회 창과 탐색 창은 별개 축이다.** 패널 조회 창(`time_window_ms`)의 기본값은 1시간(`DEFAULT_STORE_SOURCE_WINDOW` — `chartChannelTypes.ts:187`)이며, 그것을 탐색에 그대로 쓰면 대부분의 시리즈가 목록에서 사라진다. 반대로 탐색 창을 무한대로 두면 열거 비용이 무한대가 된다. 둘을 분리하고 기본값을 다르게 두는 것이 이 요구의 요지다.

### 2.9 [U9] (Ubiquitous) 디스커버리 응답 캐시 금지 승계

시스템은 열거 응답을 **캐시하지 않는다.** [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) UB1-12 · AC-33 의 규칙을 그대로 승계한다 — 스키마는 쓰기에 따라 변하며, 오래된 목록에서 고른 시리즈는 조회 시 빈 결과가 된다.

구현 축도 승계한다 — `TsdbSourceSection.tsx` 의 `useDiscoveryList`(`:113-`)가 react-query 가 아니라 `useEffect` + `useState` 를 쓰고 늦게 도착한 응답을 `alive` 플래그로 버리는 방식이며, 열거도 같은 훅을 쓴다.

### 2.10 [U10] (Ubiquitous) 저장된 config 의 렌더 결과 불변

시스템은 본 SPEC 이전에 저장된 모든 `tsdb_source` config 에 대해 **바이트 동일한 렌더 결과**를 유지한다.

구조적 근거는 §1.2.1 이다 — 본 SPEC 은 `TsdbSourceConfig` · `TsdbSeriesRef` 의 필드를 **추가하지도 제거하지도 개명하지도 않는다.** 바뀌는 것은 오직 "설정 화면이 어떤 후보 행을 제시하는가"이며, 그것은 저장된 값의 해석 경로에 없다.

| 축 | 처분 |
|----|------|
| `tsdb_source` 스키마 | 무변경 |
| `TsdbSeriesRef` | 무변경 |
| `queryTsdbMatrix` · `fetchTsdbSeries`(`tsdbSource.ts`) | 무변경 |
| `usePanelSeriesData` · `useTsdbChartData` | 무변경 |
| `POST /series/query` 계약 | 무변경 |
| 선택 UI 의 후보 행 생성 | **변경** — 본 SPEC 의 유일한 동작 변경 지점 |

**이미 저장된 시리즈가 열거 결과에 없더라도 시스템은 그것을 config 에서 제거하지 않는다.** 탐색 창 밖이거나 절단되었을 수 있다. 선택 표는 그런 시리즈를 **선택된 상태로 유지**하고 "현재 목록에 없음" 표시를 덧붙인다 — `TsdbSourceSection.tsx:400-408`(목록에 없는 에이전트) · `:455-458`(목록에 없는 버킷)이 이미 같은 패턴을 쓴다.

### 2.11 [E1] (Event-driven) measurement 선택이 열거를 촉발한다

**When** 사용자가 measurement 를 선택하면, **the system shall** 그 measurement · 현재 탐색 창 · 현재 태그 사전 필터로 D5 를 호출하고, 결과를 선택 표의 후보 행으로 전개한다.

전개 규칙은 다음과 같다.

```
후보 행 = ⋃  { (measurement, f, s.tags) | f ∈ s.fields }
        s ∈ series
```

즉 응답의 태그 집합마다 그 집합의 field 수만큼 행이 생긴다. 행 ID 는 기존 `storeSeriesId(measurement, field, tags)`(`chartChannelTypes.ts:473`)를 그대로 쓴다 — 저장된 선택과 대조하려면 ID 규약이 같아야 한다.

**When** measurement 가 비워지면, **the system shall** 후보 행을 비우고 열거 요청을 보내지 않는다.

### 2.12 [E2] (Event-driven) 태그 사전 필터 · 탐색 창 변경이 재열거를 촉발한다

**When** 사용자가 태그 사전 필터를 바꾸면, **the system shall** 바뀐 필터로 재열거한다.

**When** 사용자가 탐색 창을 바꾸면, **the system shall** 바뀐 창으로 재열거한다.

**When** 재열거가 진행 중에 다시 촉발되면, **the system shall** 이전 요청을 취소하고 늦게 도착한 응답을 버린다(`useDiscoveryList` 의 `alive` 규약 승계).

**시스템은 태그 사전 필터를 재열거 없이 클라이언트에서만 적용해서는 안 된다** — 절단이 발생한 상태에서 클라이언트 필터링은 "절단으로 잘려나간 시리즈"를 되살리지 못하므로, 사용자가 좁혔는데도 원하는 시리즈가 나타나지 않는다.

### 2.13 [S1] (State-Driven) 열거 상태를 구분해 표시한다

**While** 열거가 특정 상태인 동안, **the system shall** 다음 다섯을 **서로 구분 가능하게** 표시한다.

| 상태 | 의미 | 표시 |
|------|------|------|
| 비활성 | 에이전트 또는 measurement 미선택 | 안내 문구 + 설정 유도(기존 `chart-tsdb-no-agent` · `chart-tsdb-no-measurement` 승계) |
| 조회 중 | 열거 진행 중 | 진행 표시. 이전 목록을 파괴하지 않는다 |
| 빈 결과 | 그 창 · 그 필터에 시리즈가 없다 | 빈 목록 + **탐색 창을 함께 표시**(§2.8) |
| 절단 | 상한에 걸려 일부만 반환 | 목록 + 절단 배너 + 좁히는 방법 안내(§2.7) |
| 오류 | 열거 실패 | 오류 표시. **이전 목록을 파괴하지 않는다** |

"빈 결과"와 "오류"를 같은 글리프로 표시하면 사용자가 백엔드 장애를 데이터 없음으로 오독한다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.14 와 같은 원칙).

### 2.14 [S2] (State-Driven) v3 의 근사 field 축을 드러낸다

**While** `field_exact` 가 거짓인 동안, **the system shall** 선택 표 근처에 다음 취지의 안내를 표시한다 — *"이 백엔드에서는 field 목록이 measurement 단위이므로, 일부 (field × 태그) 조합은 실제로 존재하지 않을 수 있습니다."*

안내 문구는 `CAPABILITY_REASON_KEYS`(`panelDataSource.ts:308-317`)에 키를 더해 능력과 같은 모듈에 둔다 — 능력 표만 바뀌고 문구가 남아 **틀린 이유**를 보여주는 상태를 막는다(그 모듈의 기존 주석이 이 규율을 이미 적어 두었다).

시스템은 이 사실을 **오류로 표시해서는 안 된다.** 근사는 정상 동작이며, [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.13 이 세운 원칙("숨기지 않고 비활성 + 사유")의 연장이다.

### 2.15 [S3] (State-Driven) 3단 드릴다운의 재배치

**While** `data_source` 가 `'tsdb'` 인 설정 화면이 열려 있는 동안, **the system shall** 선택 흐름을 다음으로 구성한다.

```
에이전트(필수) → bucket → measurement → [선택: 태그 사전 필터] → 열거된 시리즈 행에서 체크
```

기존 3단 드릴다운(`TsdbSourceSection.tsx:489-563`)의 각 단계 처분은 다음과 같다.

| 기존 단계 | 처분 | 사유 |
|-----------|------|------|
| measurement 선택 (`:493-511`) | **유지** — 열거의 요청 축이다 | measurement 는 열거 파라미터이지 열거 결과가 아니다 |
| 태그 키/값 선택 (`:515-561`) | **유지하되 역할 변경** — 후보 행 생성기에서 **열거 사전 필터**로 | 좁히기는 절단 대응 수단이며(§2.7), 백엔드에 전달되어야 실효가 있다(§2.12) |
| field 선택 (D4, `fieldKeys`) | **행 생성기에서 제외** — v2 는 열거가 field 를 준다. v3 는 §2.5 3단계로 서버가 대신 호출한다 | 클라이언트가 D4 를 따로 부를 이유가 사라진다 |

**대체(replace)가 아니라 재배치(re-scope)를 택한 이유는 §4.4 다.**

시스템은 태그 사전 필터를 **필수로 만들어서는 안 된다.** 필수화는 카디널리티 문제를 사용자에게 떠넘기는 것이며, 카디널리티가 낮은 measurement 에서는 불필요한 단계가 된다. 절단이 실제로 발생했을 때만 좁히기를 **권유**한다.

### 2.16 [O1] (Optional) 선택 기능

- **가능하면** 열거 결과의 태그 키를 `SeriesSelectTable` 의 tags facet(`SeriesSelectTable.tsx:40` · `:65-72`)으로 노출해, 서버 사전 필터 없이도 클라이언트에서 즉시 좁힐 수 있게 한다(절단되지 않은 목록에 한해 유효 — §2.12).
- **가능하면** 열거 결과 개수와 절단 여부를 선택 표 헤더에 표시한다.
- **가능하면** 탐색 창 컨트롤에 "패널 조회 창과 동일" · "최근 30일" · "최근 90일" 프리셋을 둔다.
- **가능하면** v3 의 3단 조회(§2.5)를 병렬화한다 — 1단계와 3단계는 서로 독립이다.
- **가능하면** 열거 응답에 `debug_query` 를 선택적으로 포함해 생성 쿼리를 확인할 수 있게 한다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) OQ11 과 같은 취급 — 기본 미노출).

### 2.17 [UB1] (Unwanted-Behavior) 금지 동작

| # | 금지 동작 | 대응 |
|---|-----------|------|
| 1 | v3 에서 `SHOW SERIES` · `SHOW SERIES CARDINALITY` 발행 | 공식 문서 미지원(§1.2.4). 대체 경로(§2.5) |
| 2 | 태그 키 × 태그 값의 **데카르트 곱**으로 후보 행 생성 | 실재하지 않는 조합을 만든다(§4.2). 백엔드 열거만 사용 |
| 3 | 열거 결과 상한 없이 반환 | 태그 집합 1,000 · 원시 행 20,000(§2.7) |
| 4 | 절단을 조용히 수행 | `truncated` 신호 + UI 배너 + 좁히는 방법(§2.7) |
| 5 | 열거 응답 캐시 | 캐시 금지 승계(§2.9) |
| 6 | 탐색 창을 코드에 숨김 | 응답 `window` + UI 표시(§2.8) |
| 7 | `TsdbSourceConfig` · `TsdbSeriesRef` 형상 변경 | 무변경(§2.10) |
| 8 | 열거에 없는 저장된 시리즈를 config 에서 자동 제거 | 유지 + "목록에 없음" 표시(§2.10) |
| 9 | 이스케이프 없이 measurement · 태그를 쿼리에 삽입 | 기존 헬퍼 재사용, 불가 시 400(§2.3) |
| 10 | 태그 사전 필터를 클라이언트에서만 적용 | 재열거(§2.12) |
| 11 | `POST /influxdb/{agent}/query` 의 언어 화이트리스트 해제 | D5 는 그 핸들러를 지나지 않는다(§1.2.6 · §1.3) |
| 12 | `internal/migrate/tsdbtags` 를 API/에이전트 계층에서 import | [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) AC-31 승계. 복제는 허용, import 는 금지 |
| 13 | v2 에서 `distinct()` 로 열거 | 푸시다운 표에 없다(§2.4) |
| 14 | v3 의 근사 field 축을 오류로 표시 | 정상 동작 + 안내(§2.14) |
| 15 | 태그 사전 필터를 필수화 | 권유만(§2.15) |
| 16 | 절단 시 폴링마다 다른 부분집합 반환 | 결정적 정렬(§2.2) |
| 17 | 시리즈 선택 상한 48 의 변경 | 무변경. 요청 팬아웃 축은 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) NQ2 소관 |
| 18 | `SeriesSelectTable` 의 렌더 전략 변경(가상화 도입) | 공유 컴포넌트다. 행 수 상한으로 대응(§4.5 · §1.3) |
| 19 | 빈 결과와 오류를 같은 표시로 축약 | 5상태 구분(§2.13) |

### 2.18 [UB2] (Unwanted-Behavior) 상태 불일치

1. 열거가 실패해도 **이미 저장된 선택은 유지된다.** 선택 표를 못 그린다고 config 를 비우면 사용자가 설정 화면을 열었다 닫는 것만으로 패널이 망가진다.
2. 저장된 시리즈가 열거 결과에 없으면 **선택된 상태로 유지하고 표시만 덧붙인다**(§2.10). 원인은 (a) 탐색 창 밖, (b) 절단, (c) 실제 삭제 셋 중 하나이며 UI 는 셋을 구분할 수 없다 — 구분할 수 없을 때 파괴적 조치를 하지 않는다.
3. `field_exact` 가 거짓인 백엔드에서 사용자가 존재하지 않는 (field, 태그) 조합을 고를 수 있다. 그 시리즈는 조회 시 전 버킷 `null` 이 되며, 이는 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.14 의 **"빈 결과"** 이지 오류가 아니다.
4. 에이전트를 바꾸면 이전 스키마의 시리즈는 해석되지 않는다. 현행 동작(`TsdbSourceSection.tsx:376` — 에이전트 변경 시 `series: []`)을 **그대로 유지**한다.
5. 절단된 목록에서 클라이언트 facet 필터로 좁히면, 사용자는 "전체에서 좁혔다"고 오해한다. 절단 배너는 필터가 걸린 동안에도 표시를 유지한다.
6. 탐색 창을 좁혀 열거 결과가 줄었을 때, 이미 선택된 시리즈가 목록에서 사라지더라도 선택은 유지된다(2번과 동일 규칙).

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 어휘 | (문서) | — |
| U2 D5 라우트 | `internal/api/handler/influxdb_management.go`(확장), `internal/api/dto/influxdb.go`(확장) | AC-01 ~ AC-06 |
| U3 순수 생성 함수 | `internal/agent/system/influxdb_seriesenum.go`(신규) | AC-07, AC-08 |
| U4 v2 열거 | 동일 + `internal/agent/system/influxdb_v2.go` | AC-09 ~ AC-12 |
| U5 v3 열거 | 동일 + `internal/agent/system/influxdb_v3.go` | AC-13 ~ AC-17 |
| U6 능력 표 확장 | `web/src/pages/dashboard/panels/charts/panelDataSource.ts` | AC-18, AC-19 |
| U7 상한 · 절단 | `internal/api/handler/influxdb_management.go`, `TsdbSourceSection.tsx` | AC-20 ~ AC-23 |
| U8 탐색 창 | `internal/agent/system/influxdb_seriesenum.go`, `TsdbSourceSection.tsx` | AC-24 ~ AC-26 |
| U9 캐시 금지 | `TsdbSourceSection.tsx`, `web/src/services/api/influxdbManagement.ts` | AC-27 |
| U10 config 불변 | `chartChannelTypes.ts`(무변경 단언), `tsdbSource.ts`(무변경 단언) | AC-28 ~ AC-30 |
| E1 measurement → 열거 | `TsdbSourceSection.tsx` | AC-31, AC-32 |
| E2 재열거 | `TsdbSourceSection.tsx` | AC-33 ~ AC-35 |
| S1 5상태 | `TsdbSourceSection.tsx` | AC-36 ~ AC-38 |
| S2 근사 field 안내 | `TsdbSourceSection.tsx`, `panelDataSource.ts`, `lib/i18n/{ko,en}.json` | AC-39 |
| S3 드릴다운 재배치 | `TsdbSourceSection.tsx` | AC-40 ~ AC-42 |
| O1 선택 기능 | — | (선택) |
| UB1 금지 동작 | 전 파일 | AC-43 ~ AC-49 |
| UB2 상태 정합 | `TsdbSourceSection.tsx` | AC-50 ~ AC-53 |
| 비기능 · 정적 검사 | 전 파일 | AC-54, AC-55 |

---

## 4. 설계 결정

### 4.1 열거는 신규 디스커버리 항목(D5)이며, 구조화 질의를 확장하지 않는다

세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| `POST /series/query` 에 "열거 모드"를 추가 | 한 라우트가 두 가지 일을 한다. 요청 형상에 상호배타 필드가 생기고, 응답도 두 형상이 된다. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.16 #23("세 번째 응답 형상 금지")의 취지와 어긋난다 |
| 클라이언트가 D2 + D3 를 조합해 열거 | **틀린다**(§4.2). 그리고 조합 순회 비용이 클라이언트 → 백엔드 왕복 수로 나타난다 |
| **신규 디스커버리 라우트 D5** | D1~D4 와 같은 성질(읽기 전용 · 캐시 금지 · `store.read` · 같은 핸들러 · 같은 오류 매핑)이며 기존 기계에 그대로 얹힌다 |

세 번째를 택한다. D5 라는 이름은 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.10 의 D1~D4 연번을 잇는다.

### 4.2 태그 키 × 값의 데카르트 곱은 비용 문제 이전에 **오답**이다

`TsdbSourceSection.tsx:261-267` 의 주석은 조합 순회를 **비용** 때문에 미뤘다고 적는다. 그 판단은 그때 옳았으나, 근거가 하나 빠져 있다.

`host ∈ {a, b}` · `region ∈ {kr, us}` 인 measurement 에서 데카르트 곱은 4개 조합을 만든다. 그러나 실제로 기록된 것이 `(a,kr)` 과 `(b,us)` 뿐이라면 나머지 둘은 **존재하지 않는 시리즈**다. 사용자가 그것을 고르면 조회는 성공하고 결과가 비며, 화면에는 전 버킷 `null` 컬럼이 남는다 — [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.14 의 "빈 결과" 상태이므로 **오류로도 보이지 않는다.**

즉 데카르트 곱은 사용자가 원인을 알 수 없는 조용한 오답을 만든다. 이는 같은 SPEC 의 UB1-4(`field` 폴백 금지)가 막으려 한 것과 정확히 같은 종류의 결함이다.

**따라서 열거는 백엔드가 수행해야 한다.** 실재 여부를 아는 것은 백엔드뿐이다.

### 4.3 v3 대체 경로 — SQL `DISTINCT` 를 기본으로, `GROUP BY *` 를 후보로

`SHOW SERIES` 부재(§1.2.4)에 대한 네 후보를 검토했다.

| 후보 | 처분 | 사유 |
|------|------|------|
| InfluxQL `SHOW SERIES` | **불가** | 공식 문서 두 곳이 미지원 명시 |
| **SQL `SELECT DISTINCT <태그 컬럼들>`** | **채택(기본)** | 구성 요소가 전부 문서상 지원 또는 트리에 존재 — `SHOW TAG KEYS`(문서 지원) + 클라이언트 SQL 경로(`influxdb_v3.go:90-97`). 태그가 v3 에서 테이블 컬럼이라는 사실은 [SQL 스키마 탐색 문서](https://docs.influxdata.com/influxdb3/core/query-data/sql/explore-schema/)의 `SHOW COLUMNS IN <table>` 이 뒷받침한다 |
| InfluxQL `SELECT <f> FROM "<m>" ... GROUP BY *` | **후보(OQ2)** | `GROUP BY *` 는 [문서상 지원된다](https://docs.influxdata.com/influxdb3/core/reference/influxql/group-by/)("Groups data by all tags"). 쿼리 1회로 끝나는 이점이 크다. 그러나 **그 결과에서 태그가 `iteratorToMaps`(`influxdb_v3.go:118-129`) 를 통해 행 컬럼으로 드러나는지 문서로 확인되지 않았다.** M1 이 실측한다 |
| SQL `information_schema` | **기각** | [문서가 직접 질의 예시를 제공하지 않으며](https://docs.influxdata.com/influxdb3/core/query-data/sql/explore-schema/), 컬럼 **정의**를 줄 뿐 어떤 태그 **값 조합**이 실재하는지는 주지 않는다 |

기본을 SQL `DISTINCT` 로 두는 이유는 **구성 요소가 전부 검증되었기 때문**이지 성능 때문이 아니다. `GROUP BY *` 가 실측에서 태그를 드러내면 M1 이 그것을 기본으로 승격할 수 있고, 그 결정은 순수 함수 1개와 클라이언트 메서드 1개에 국한된다.

**v2 와 v3 의 경로가 다른 것을 감추지 않는다.** [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 가 이미 세 가지 백엔드 차이(bucket 도달 여부 · 관리 조작 · fill 지원)를 능력 표로 드러냈고, 본 SPEC 의 `seriesFieldExact` 는 그 네 번째다(§2.6).

### 4.4 3단 드릴다운을 대체하지 않고 재배치한다

세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| 드릴다운을 전부 없애고 열거 결과만 제시 | measurement 는 열거의 **요청 파라미터**이므로 없앨 수 없다(없애면 버킷 전체를 열거해야 한다). 태그 사전 필터도 없애면 절단 대응 수단이 사라진다(§2.7) |
| 드릴다운과 열거를 **두 모드**로 병치 | 같은 목적의 UI 가 두 벌이 되고, 두 모드의 선택 결과가 같은 `series[]` 에 섞인다. 사용자는 어느 모드에서 고른 것인지 기억해야 한다 |
| **드릴다운을 열거의 입력으로 재배치** | measurement 는 요청 축, 태그는 사전 필터, field 는 열거 결과. 각 단계가 **하나의 역할만** 갖는다 |

세 번째를 택한다. 부수 효과로 D4(field-keys)에 대한 **클라이언트 직접 호출이 사라진다** — v2 는 열거가 field 를 주고, v3 는 서버가 대신 부른다. D4 라우트 자체는 남으며(다른 소비자가 생길 수 있고, 본 SPEC 은 라우트를 제거하지 않는다) 프런트의 `fetchInfluxFieldKeys`(`web/src/services/api/influxdbManagement.ts:184`)도 남긴다.

### 4.5 페이지네이션 대신 상한 + 절단 신호 + 좁히기를 택한다

네 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| 페이지네이션 | 선택 표가 **controlled 다중 선택** 컴포넌트다(`SeriesSelectTable.tsx:44-56` — `selectedIds` · `onSelectMany` · `onClearMany`). 페이지를 넘나들며 선택하면 "이 페이지 전체 선택"의 의미가 모호해지고, 48 상한과의 상호작용이 페이지 경계마다 달라진다 |
| 태그 사전 필터 **필수화** | 카디널리티가 낮은 measurement 에서 불필요한 강제 단계가 된다. 문제를 사용자에게 떠넘긴다 |
| 무제한 반환 + 클라이언트 필터 | 선택 표가 전 행을 렌더한다(`SeriesSelectTable.tsx:356`). 필터 이전에 DOM 이 무너진다 |
| **서버 상한 + 절단 신호 + 좁히기 권유** | 백엔드 계산량과 DOM 행 수를 동시에 막고, 이미 있는 검색·facet 필터(`SeriesSelectTable.tsx:14` · `:40`)를 그대로 쓴다. 절단이 실제로 걸릴 때만 사용자에게 조치를 요구한다 |

네 번째를 택한다. 1,000 이라는 값의 근거는 "그보다 크면 선택 표의 렌더가 문제가 된다"는 것이며 정밀한 측정치가 아니다(OQ4).

### 4.6 탐색 창의 기본값을 30일로 두는 것은 **현상 유지**다

`-30d` 는 임의로 고른 값이 아니라 v2 `schema.*` 함수의 문서상 기본값이며(§1.2.8), 현재 D2~D4 가 이미 그 값으로 동작하고 있다. 다른 값을 고르면 본 SPEC 이 사용자가 보던 목록의 범위를 조용히 바꾸게 된다.

세 대안의 처분:

| 대안 | 처분 |
|------|------|
| 패널 조회 창(`time_window_ms`, 기본 1시간)과 동일 | 기본값으로는 너무 좁다 — 대부분의 시리즈가 목록에서 사라진다. **선택지로는 제공한다**(§2.8) |
| 창 없음(전체 기간) | v2 는 `range()` 없이 열거할 수 없고, v3 는 전체 스캔이 된다 |
| **`-30d` (현상 유지) + 사용자 제어** | 동작 변화 0에서 출발하고, 필요하면 사용자가 넓힌다 |

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 성능 (v2) | 열거 1회 = Flux 쿼리 1회. `first()` · `group() \|> first()` 는 푸시다운 가능(§2.4). 쿼리 `limit(n: 20000)` 로 상한 |
| 성능 (v3) | 열거 1회 = 쿼리 최대 3회(`SHOW TAG KEYS` + `SELECT DISTINCT` + `SHOW FIELD KEYS`). 1·3 단계는 병렬 가능(§2.16 O1) |
| 네트워크 | 열거는 **선택 UI 조작 시에만** 발생하며 폴링하지 않는다. 패널 렌더 경로의 요청 수는 무변경(시리즈당 1요청 · 최대 48) |
| 자원 | 반환 태그 집합 1,000 · 원시 행 20,000 · 조회 타임아웃 60초(`defaultInfluxManagementTimeout` — `influxdb_management.go:43`) |
| 하위 호환 | `tsdb_source` 저장 형상 · `POST /series/query` 계약 · D1~D4 계약 전부 무변경. 저장된 config 의 렌더 결과 바이트 동일(§2.10) |
| 보안 | measurement · 태그 키 · 태그 값 전부 이스케이프(§2.3). 이스케이프 불가 시 400. 생성 쿼리는 기본적으로 응답에 포함하지 않는다 |
| 테스트 | 신규 코드 커버리지 85% 이상(`.moai/config/sections/quality.yaml`). 쿼리 생성 순수 함수는 (v2/v3) × (사전 필터 0/1/N) × (태그 키 0/1/N) 조합 테스트 |
| 접근성 | 절단 배너는 `role="alert"`. 근사 field 안내는 `aria-disabled` 가 아니라 보조 설명이므로 `aria-describedby` 로 표에 연결 |
| 국제화 | 신규 문구(탐색 창 라벨 · 프리셋 · 절단 배너 · 근사 field 안내 · 목록에 없음 표시 · 열거 오류)는 `web/src/lib/i18n/{ko,en}.json` 두 로케일 |
| LSP | `npx tsc --noEmit` 0 에러, `npx eslint src --max-warnings 0`, `go vet ./...` 0 에러, `go build ./...` 0 에러 |

---

## 6. 가정 및 제약

1. `TsdbSeriesRef` · `TsdbSourceConfig` · `SeriesMatrixQuery` · `SeriesMatrix` 계약은 안정적이며 본 SPEC 이 수정하지 않는다(§2.10).
2. `SeriesSelectTable`(`web/src/pages/agents/SeriesSelectTable.tsx`)의 props 계약은 안정적이며 본 SPEC 이 수정하지 않는다. 행 개수만 달라진다.
3. `queryFlux`(`influxdb_v2.go:83-103`)가 결과 행의 **모든 컬럼**을 통과시킨다 — `result.Record().Values()`(`:93`). §2.4 가 이에 의존한다.
4. v3 클라이언트가 SQL 을 실행할 수 있다 — `Query(ctx, q, "sql")` → `querySQL`(`influxdb_v3.go:76-97`). §2.5 가 이에 의존한다.
5. InfluxDB v2 의 `schema.*` 함수 기본 `start` 는 `-30d` 다([문서 확인](https://docs.influxdata.com/flux/v0/stdlib/influxdata/influxdb/schema/measurementtagkeys/)). §2.8 · §4.6 이 이에 의존한다. `measurementFieldKeys` · `measurementTagValues` 도 같은 기본값을 갖는 것으로 **가정**하며, 이는 §7 OQ5 가 열어 둔다.
6. InfluxDB 3 은 `SHOW SERIES` 를 지원하지 않는다(문서 2곳 확인 — §1.2.4). 이 상태가 향후 바뀌면 §2.5 의 경로를 재검토할 수 있으나, 본 SPEC 의 능력 표 구조는 그 변화를 항목 하나로 흡수한다.
7. InfluxDB 3 의 InfluxQL `GROUP BY *` 는 문서상 지원되나, 그 결과 형상은 본 SPEC 이 실측하지 않았다(OQ2).
8. 에이전트의 v2/v3 판별은 기존 `resolveInfluxVersion`(`panelDataSource.ts:293`) · `InfluxDBConfig.Version` 으로 가능하며 런타임에 바뀌지 않는다.
9. `internal/migrate/tsdbtags/` 는 계속 import 하지 않는다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §4.5 · AC-31). 필요 시 문자열을 **복제**한다.
10. 대상 프론트엔드는 React 19 + TypeScript 5.9 + Vitest, 백엔드는 Go 이며 기존 `api.RouteGroup` · `dto.NewSuccessResponse` 패턴을 따른다.
11. 현재 브랜치는 `feature/SPEC-TSDB-002` 이고 HEAD 는 `e2809f4b` 이며, [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 의 M1~M7 이 트리에 존재한다.

---

## 7. 열린 질문 (OQ)

**아래는 전부 `/moai run` 이전에 사용자 결정이 필요한 항목이다.** "권장" 열은 본 SPEC 이 근거와 함께 제시하는 기본값이며, 확정 전에는 구현 규칙이 아니다.

| # | 질문 | 권장 | 근거 / 대안 |
|---|------|------|-------------|
| **OQ1** | D5 의 라우트 이름과 배치. `GET /influxdb/{agent}/series` 는 기존 `POST /influxdb/{agent}/series/query` 와 접두사를 공유한다 | `GET /influxdb/{agent_name}/series`, `InfluxDBManagementHandler` 에 등록 | 메서드가 달라 라우팅 충돌은 없고, D1~D4 의 `resolveDiscoverer`(`influxdb_management.go:97`) · 오류 매핑(`:345`)을 그대로 쓴다. **대안**: `GET /series-list` 로 접두사를 분리하거나, `InfluxDBSeriesHandler` 에 등록해 `/series` 접두사를 한 핸들러가 소유 |
| **OQ2** | v3 열거의 기본 경로. SQL `SELECT DISTINCT` (3쿼리) vs InfluxQL `GROUP BY *` (1쿼리) | **M1 스파이크로 실측 후 결정.** 실측 전 기본값은 SQL `DISTINCT` | `GROUP BY *` 는 [문서상 지원](https://docs.influxdata.com/influxdb3/core/reference/influxql/group-by/)되나 태그가 `iteratorToMaps`(`influxdb_v3.go:118-129`)를 통해 행 컬럼으로 드러나는지 미확인. SQL 경로는 구성 요소가 전부 검증됨(§4.3). **이 질문이 확정되기 전에는 M3 을 시작할 수 없다** |
| **OQ3** | v3 의 `field_exact: false` 를 수용할 것인가, 아니면 field 별 `DISTINCT` 를 N회 실행해 정확도를 사는가 | **수용한다**(`false`) | field 가 F개면 쿼리가 F+2회가 된다. 근사의 대가는 "존재하지 않는 조합을 고를 수 있음"이고 그 결과는 빈 결과이며 오류가 아니다(§2.18-3). **대안**: field 수가 임계(예: 5) 이하일 때만 정확 경로를 쓰는 하이브리드 |
| **OQ4** | 반환 태그 집합 상한 1,000 의 값 | **1,000** | 근거는 "선택 표가 전 행을 렌더한다"(`SeriesSelectTable.tsx:356`)는 정성적 사실이며 측정치가 아니다. **대안**: M1 에서 렌더 시간을 실측해 값을 정하거나, 설정 가능하게 둔다 |
| **OQ5** | 탐색 창 기본값 30일. 그리고 `measurementFieldKeys` · `measurementTagValues` 의 `start` 기본값이 `measurementTagKeys` 와 같은가 | **30일**(현상 유지 — §4.6). 나머지 두 함수의 기본값은 M2 에서 문서 재확인 | `measurementTagKeys` 만 문서로 확인했다(`-30d`). 같은 계열이므로 동일할 가능성이 높으나 **확인하지 않았다** |
| **OQ6** (확정 v0.2.0) | 탐색 창을 `tsdb_source` 에 **영속할 것인가**, 편집 커서(로컬 상태)로 둘 것인가 | **로컬 상태** | 탐색 창은 선택 행위의 도구이지 패널의 조회 설정이 아니다. 영속하면 `tsdb_source` 형상이 바뀌어 §2.10(형상 무변경)이 깨진다. **대안**: 영속하면 설정 화면을 다시 열 때 창이 유지되어 편의는 높다 |
| **OQ7** | 사전 필터로 좁힌 태그 키를 열거 결과의 행에도 포함할 것인가(즉 `tags` 에 사전 필터 키가 들어가는가) | **포함한다** | 그래야 `storeSeriesId(measurement, field, tags)` 가 질의 시 실제로 보내는 태그와 일치한다(`tsdbSource.ts:211`). **대안**: 사전 필터를 질의에는 빼고 선택만 좁히면, 저장된 시리즈가 사전 필터 없이 조회되어 여러 시리즈가 매칭된다 |
| **OQ8** (확정 v0.2.0) | 이 SPEC 이 [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.15 [O1] 의 "3단 드릴다운" 항목을 **대체(supersede)** 한다고 선언할 것인가 | **대체 선언한다** | 그 항목은 "가능하면"(Optional)이었고 본 SPEC 이 그 역할을 재배치한다(§2.15 [S3]). 선언하지 않으면 두 SPEC 이 같은 UI 에 대해 다른 것을 요구하는 상태가 남는다 |
| **OQ9** | 절단이 발생했을 때 서버가 **어느 부분집합**을 돌려줄 것인가 | 태그 직렬화 문자열 **오름차순 상위 N** | 결정적이면 폴링 간 흔들림이 없다(§2.2 · UB1-16). **대안**: 최근 기록순(비용 높음), 무작위(금지) |
| **OQ10** | v2 열거 쿼리에서 `first()` 대신 `last()` 를 쓸 것인가 | **`first()`** | 푸시다운 표에 둘 다 있으므로 성능 차이는 없다고 본다. `first()` 를 택하는 이유는 "구간에 존재했음"을 판정하는 데 어느 쪽이든 충분하고 하나를 고정해야 테스트가 결정적이기 때문이다 |
