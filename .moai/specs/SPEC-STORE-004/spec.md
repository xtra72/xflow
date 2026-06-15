---
id: SPEC-STORE-004
title: Store 에이전트 복합 식별(시리즈) 모델 — (key, metric_type, tags) 통일
version: 0.1.0
status: draft
created: 2026-06-15
updated: 2026-06-15
author: xtra
priority: high
lifecycle: spec-anchored
related: [SPEC-STORE-001, SPEC-STORE-002, SPEC-STORE-003, SPEC-TSDB-001, SPEC-CHART-001, SPEC-WEB-005]
---

# SPEC-STORE-004: Store 에이전트 복합 식별(시리즈) 모델

## HISTORY

| Version | Date       | Author | Change                                                                 |
| ------- | ---------- | ------ | ---------------------------------------------------------------------- |
| 0.1.0   | 2026-06-15 | xtra   | 최초 작성 — 저장 아이템 식별을 `(key, metric_type, sorted(tags))` 복합 식별(시리즈)로 통일. data_type 은 식별에서 제외. 키 조회 시 전체 시리즈 반환 + metric/tags 필터로 단일 시리즈 선택. 마이그레이션 경로 및 tsdb 역할 구분 포함. |

## 개요 (Overview)

현재 Store 에이전트(SPEC-STORE-001/002/003)는 저장 아이템을 **단일 string key** 하나로
식별한다. 같은 key 에 metric_type 또는 tags 가 다른 값을 쓰면 **기존 값과 메타를 덮어쓴다**.
즉, 한 key 는 정확히 하나의 (value + StaticKeyMeta) 만 가질 수 있다.

본 SPEC 은 저장 아이템 식별을 **`(key, metric_type, sorted(tags))` 복합 식별 = 시리즈
(series)** 로 **전면 재설계**한다. 같은 key 라도 metric_type 또는 tags 가 다르면 **서로 독립된
별개의 시리즈(아이템)** 로 저장된다. 이는 InfluxDB / Prometheus 의 시리즈
(measurement + tag set) 식별 모델과 유사하다.

핵심 원칙:

1. **시리즈 식별자** = `(key, metric_type, sorted(tags))`. `data_type` 은 **식별에서 제외**한다
   (값 타입일 뿐, 시리즈를 가르는 차원이 아니다).
2. **키 조회** = 해당 key 아래 **모든 시리즈**(모든 metric/tags 조합)를 반환한다.
   `metric_type` / `tags` 필터 파라미터로 **특정 단일 시리즈**를 선택할 수 있다.
3. **전면 통일(BREAKING)** — 부분 하위호환이 아니라 모델 자체를 복합 식별로 재설계한다.
   단, 기존 단일-키 데이터의 마이그레이션/전환 경로를 본 SPEC 에서 정의한다.

**Breaking Change Notice**: 현재의 "key 1개 = 아이템 1개" 모델은 본 SPEC 적용 후
"key 1개 = 시리즈 N개" 모델로 바뀐다. `VolatileStore`(sync.Map string 키), `staticKeys`
(bare key 키잉), `NamespacedStore`(`namespace:key` prefix), `SetWithMeta`/store-write 쓰기
경로, `POST /query` · `GET /keys` 조회 API, `PersistentStore`/`StoreRepository` 영속 백엔드,
그리고 웹 UI(TSDB 데이터 뷰어)가 모두 영향을 받는다.

## 배경 (Background)

### 현재 모델의 정확한 동작 (코드 grounding)

| 영역 | 파일 | 현재 동작 |
| --- | --- | --- |
| 인메모리 저장 | `internal/agent/system/store_volatile.go` | `VolatileStore.data` 는 `sync.Map` 을 **string 키** 로 사용. `storeItem{value, createdAt, updatedAt, expiresAt, namespace, history}`. 아이템 식별 = (namespace prefix 포함) string key 하나. |
| 키 메타 | `internal/agent/system/store_data_type.go` | `StaticKeyMeta{DataType, MetricType, Tags, Source}`. **키당 메타 1개**(metric_type 1개 + tags 1세트). 같은 키에 다른 metric/tags 로 쓰면 덮어쓴다. |
| 에이전트 | `internal/agent/system/store.go` | `StoreAgent.config.staticKeys map[string]StaticKeyMeta` 는 **bare key** 로 키잉. `SetKeyMeta(key, metricType, tags)`, `SetKeyDataType(key, dt)`, `checkKeyAllowed`(auto 등록 + 타입 검증), `StaticKeyMetaFor`, `StaticKeysSnapshot`. |
| 네임스페이스 | `internal/agent/system/store_namespace.go` | `NamespacedStore` 가 `namespace + ":" + key` 로 VolatileStore 키 prefix. `staticKeys` 검증은 prefix 없는 사용자 key 로 수행. |
| 노드 어댑터 | `internal/agent/system/store_node_adapter.go` | `NodeStoreAdapter.SetWithMeta(ctx, key, value, StoreWriteMeta{DataType, MetricType, Tags, TTL})` → `SetKeyDataType` + `writeValue`(Set/SetWithTTL) + `SetKeyMeta`. metric/tags 는 **키에 부여**되며 같은 key 재쓰기 시 **덮어쓴다**. |
| 쓰기 노드 | `internal/node/store_write.go` | store-write 노드가 `key_template`/`value_key`/`metric_type`/`tags`/`data_type`/`ttl` 을 config 로 받아 위 어댑터로 쓴다. metric/tags 는 리터럴 또는 `$.` 경로(메시지 필드)로 해석된다. |
| 조회/리스트 | `internal/agent/system/store_query.go` | `UserStoreAgent.QueryHistory(ctx, namespace, key, q)`, `ListStoreKeys(ns, pattern)`, `StaticKeyTags`, `IsStaticKey`, `StaticTagPairs`. |
| HTTP API | `internal/api/handler/store_query.go` | `POST /api/v1/store/{name}/query`(key+namespace 로 단일 시리즈 조회), `GET /keys`(객체 배열 {key, registration, data_type, metric_type, tags} + `?tag=`/`?data_type=`/`?metric_type=`/`?registration=` AND 필터), `PUT /keys/{key}/meta`, `DELETE /keys/{key}`, `DELETE /keys`. |
| 영속 백엔드 | `internal/agent/system/store_persistent.go` | `PersistentStore` 는 `StoreRepository`(SetEntry/GetEntry/ListKeys/…) 를 **string key** 로 호출. 현재 StoreAgent 는 VolatileStore 만 사용하나 인터페이스/구현은 존재. |
| 웹 UI | `web/src` | "TSDB 데이터 뷰어" 가 시리즈 키/metric/tags 컬럼을 표시. |
| 별도 TSDB | `internal/agent/system/tsdb_agent.go`, `internal/tsdb` | 독립 시계열 시리즈 저장소. `tsdb.BuildSeriesKey(measurement, tags)` 가 이미 `"measurement,k1=v1,k2=v2"`(태그 정렬) 형식으로 시리즈 키를 생성한다. |

### 현재 모델의 한계

- **메타 덮어쓰기 충돌**: 같은 key 에 `metric_type=temperature` 와 `metric_type=humidity`
  를 번갈아 쓰면 마지막 쓰기가 이전 메타를 덮어쓴다. 의미상 다른 측정을 동일 key 로 보낼 수
  없다. 다중 센서/측정을 하나의 논리 key 로 묶기 어렵다.
- **tags 의 비식별성**: tags 는 메타데이터일 뿐 식별 차원이 아니다. `room=1` 과 `room=2` 를
  같은 key 로 쓰면 한쪽이 다른 쪽을 덮어쓴다.
- **TSDB 모델과의 불일치**: 이미 존재하는 tsdb 에이전트는 (measurement + tag set) 시리즈
  모델인데, store 는 key 1개 모델이라 데이터 모델이 갈린다.

### 목표

- store 아이템을 **시리즈 단위**(`key + metric_type + sorted(tags)`)로 식별·저장하여,
  하나의 논리 key 아래 여러 측정(metric)·여러 라벨(tags) 조합을 **독립 보관**한다.
- 조회는 key 단위로 **전체 시리즈를 한 번에** 가져오되, metric/tags 필터로 단일 시리즈를
  좁힐 수 있게 한다.
- 기존 단일-키 데이터를 무손실로 시리즈 모델로 흡수하는 **마이그레이션 경로**를 제공한다.

## 용어 정의 (Definitions)

- **시리즈 식별자 (Series ID)**: `(key, metric_type, sorted(tags))` 의 정규화된 결합.
  같은 key 라도 이 셋 중 하나라도 다르면 다른 시리즈이다.
- **정규화 (Normalization)**: tags 를 tag-key 사전순으로 정렬하고, metric_type 빈 값을
  `"unknown"` 으로 보정한 뒤 결정적 인코딩 문자열로 만드는 과정.
- **시리즈 키 인코딩 (Series Key Encoding)**: 시리즈 식별자를 `VolatileStore`/영속 백엔드의
  단일 string 키로 직렬화한 결과. (후보는 명세 §시리즈 키 인코딩 참조)
- **data_type**: 시리즈가 보관하는 값의 타입(int/float/string/boolean/bytes/json). **식별
  차원이 아니다.** 한 시리즈 내 값들의 타입 계약일 뿐이다.

## 환경 (Environment)

- 언어/런타임: Go 1.23+ (프로젝트 표준).
- 동시성: `VolatileStore` 는 `sync.Map`, `StoreAgent` 는 `sync.RWMutex` 로 `staticKeys`(향후
  시리즈 레지스트리) 를 보호한다. 본 SPEC 의 모든 신규 자료구조는 동일 수준의 동시성 안전을
  유지해야 한다.
- 타임스탬프: 프로젝트 컨벤션에 따라 payload/직렬화 타임스탬프는 epoch milliseconds
  (int64, UnixMilli) 를 사용한다. (`store_node_adapter.go` QueryHistory 출력 동일)
- 백엔드: 현 시점 StoreAgent 는 `VolatileStore`(in-memory) 만 사용한다. `PersistentStore`/
  `StoreRepository` 는 인터페이스/구현이 존재하나 활성 경로가 아니다 (가정 A1 참조).

## 가정 (Assumptions)

- **A1**: StoreAgent 런타임 경로는 현재 VolatileStore 만 사용한다 (`store.go` Init 에서
  `NewVolatileStore` 만 생성). `PersistentStore` 는 존재하지만 활성 백엔드가 아니다. → 본
  SPEC 의 1차 구현 범위는 VolatileStore + staticKeys 이며, PersistentStore 는 인터페이스
  정합성만 맞추고 시리즈 키 인코딩을 동일 규칙으로 적용한다 (실데이터 마이그레이션 대상 아님).
  *(검증 필요: 운영 배포에서 영속 백엔드를 활성화한 사례가 있는지 확인)*
- **A2**: `namespace` 는 시리즈 식별자의 **상위 스코프**로 유지된다. 즉 시리즈 식별은
  `namespace` 내에서 `(key, metric_type, sorted(tags))` 로 유일하다. namespace 자체를 시리즈
  식별자에 합치는 것은 본 SPEC 범위 밖이다 (기존 prefix 모델 보존).
- **A3**: `metric_type` 은 `^[a-zA-Z0-9_-]+$` 정규식과 기본값 `"unknown"` 규칙
  (`validateMetricType`) 을 그대로 따른다. tags 의 key/value 도 기존 `^[a-zA-Z0-9_-]+$`
  규칙을 따른다. 따라서 시리즈 키 인코딩에 사용할 구분자(`,` `=` 등)는 이 문자집합과 충돌하지
  않는다 *(인코딩 안전성의 근거)*.
- **A4**: store-write 노드는 한 메시지에서 `metric_type`/`tags` 를 **모든 키에 공유** 적용한다
  (`store_write.go` Process). 시리즈 모델에서도 이 공유 메타가 각 (키,값) 쌍과 결합되어 시리즈를
  결정한다. 즉 한 store-write 실행은 `len(targets)` 개의 시리즈에 쓴다.
- **A5**: history(값 변경 이력)는 **시리즈 단위**로 보관된다. 즉 같은 key 라도 metric/tags 가
  다른 시리즈는 서로 독립된 history 를 가진다. (현재는 string key 단위 history)
- **A6**: 기존 운영 데이터는 휘발성(VolatileStore, 재시작 시 소멸)이 대부분이므로, 인메모리
  데이터의 "마이그레이션"은 재시작 후 신규 쓰기로 자연 흡수된다. 영속 데이터(있다면)에 대한
  변환만 명시적 마이그레이션 절차가 필요하다 (가정 A1 연계).

## 요구사항 (Requirements — EARS)

### Ubiquitous (항상 적용)

- **U1**: 시스템은 **항상** 저장 아이템을 `(key, metric_type, sorted(tags))` 복합 식별자(시리즈)
  로 식별해야 한다.
- **U2**: 시스템은 **항상** 시리즈 식별 시 tags 를 tag-key 사전순으로 정렬한 정규화된 형태로
  사용해야 하며, tag 순서가 달라도 동일 시리즈로 식별해야 한다.
- **U3**: 시스템은 **항상** `data_type` 을 시리즈 식별에서 **제외**해야 한다. 같은
  `(key, metric_type, sorted(tags))` 이면 data_type 이 달라도 동일 시리즈로 취급해야 한다.
- **U4**: 시스템은 **항상** 시리즈 식별자를 결정적(deterministic) 인코딩으로 단일 저장 키
  문자열로 직렬화해야 하며, 동일 식별자는 항상 동일 인코딩을 생성해야 한다.
- **U5**: 시스템은 **항상** history(값 변경 이력)를 시리즈 단위로 독립 보관해야 한다.
- **U6**: 시스템은 **항상** `namespace` 를 시리즈 식별의 상위 스코프로 유지해야 한다 (시리즈
  유일성은 namespace 내부에서 보장).

### Event-Driven (이벤트 기반)

- **E1**: **WHEN** store-write 노드(또는 SetWithMeta) 가 `(key, metric_type, tags)` 로 값을 쓸
  때 **THEN** 시스템은 해당 시리즈 식별자에 매핑되는 시리즈에 값을 기록해야 한다.
- **E2**: **WHEN** 같은 key 에 **다른 metric_type** 으로 값을 쓸 때 **THEN** 시스템은 기존
  시리즈를 덮어쓰지 않고 **새로운 독립 시리즈**를 생성/갱신해야 한다.
- **E3**: **WHEN** 같은 key 에 **다른 tags(정규화 후)** 로 값을 쓸 때 **THEN** 시스템은 기존
  시리즈를 덮어쓰지 않고 **새로운 독립 시리즈**를 생성/갱신해야 한다.
- **E4**: **WHEN** 키로 조회(`POST /query` 또는 동등 API)할 때 **THEN** 시스템은 해당 key 아래
  **모든 시리즈**(모든 metric/tags 조합)를 반환해야 한다.
- **E5**: **WHEN** 조회 요청에 `metric_type` 또는 `tags` 필터가 포함될 때 **THEN** 시스템은
  필터에 정확히 일치하는 **단일(또는 부분집합) 시리즈**만 반환해야 한다.
- **E6**: **WHEN** 같은 시리즈 식별자에 값이 반복 기록될 때 **THEN** 시스템은 해당 시리즈의
  history 에 이전 값을 누적하고 현재값을 갱신해야 한다 (시리즈 내 시계열 유지).
- **E7**: **WHEN** `GET /keys` 가 호출될 때 **THEN** 시스템은 각 시리즈를 하나의 행으로 노출하고
  `(key, metric_type, tags, data_type, registration)` 을 포함해야 한다.
- **E8**: **WHEN** 단일 시리즈 reset(`DELETE`) 이 요청될 때 **THEN** 시스템은 식별된 단일
  시리즈만 대상(정적: history clear, 동적: entry delete)으로 처리하고 다른 시리즈에 영향을 주지
  않아야 한다.

### State-Driven (상태 기반)

- **S1**: **IF** registration_type 이 `manual` **THEN** 시스템은 사전 정의되지 않은 시리즈
  식별자에 대한 쓰기를 거부(`ErrKeyNotAllowed` 계열)해야 한다.
- **S2**: **IF** registration_type 이 `auto` **THEN** 시스템은 미등록 시리즈 식별자를 첫 쓰기
  시점에 자동 등록(SourceAuto)해야 한다.
- **S3**: **IF** 조회 시 `metric_type` 필터만 주어지고 `tags` 필터가 생략되면 **THEN** 시스템은
  해당 key + metric_type 에 속하는 **모든 tags 조합 시리즈**를 반환해야 한다.
- **S4**: **IF** 조회 시 어떤 시리즈 필터도 일치하지 않으면 **THEN** 시스템은 빈 결과(HTTP 200,
  엔트리 0개)를 반환해야 한다 (에러 아님 — 기존 ErrKeyNotFound→200 정책 보존).

### Unwanted (금지)

- **N1**: 시스템은 다른 metric_type 을 가진 쓰기로 인해 기존 시리즈의 값/history/메타를
  **덮어쓰지 않아야 한다**.
- **N2**: 시스템은 다른 tags(정규화 후)를 가진 쓰기로 인해 기존 시리즈를 **덮어쓰지 않아야
  한다**.
- **N3**: 시스템은 data_type 차이만으로 **새 시리즈를 생성하지 않아야 한다** (data_type 은 식별
  차원이 아님).
- **N4**: 시스템은 거부된 쓰기(manual 미등록 등)에 대해 시리즈 엔트리/history 에 흔적을 남기지
  **않아야 한다** (기존 gatekeeper 보존 동작 유지).
- **N5**: 시스템은 시리즈 키 인코딩에서 metric_type/tag 값 충돌로 인한 **시리즈 오병합(서로 다른
  식별자가 같은 인코딩으로 충돌)** 을 허용하지 않아야 한다.

### Optional (선택)

- **O1**: **가능하면** 시리즈 키 인코딩을 기존 `tsdb.BuildSeriesKey(measurement, tags)`
  (`"measurement,k1=v1,k2=v2"`) 규약과 정렬하여 store/tsdb 간 시리즈 모델 일관성을 제공한다.
- **O2**: **가능하면** `GET /keys` 응답에 key 단위로 시리즈를 그룹핑한 뷰(또는 `?group_by=key`)
  를 제공하여 UI 가 한 key 의 시리즈들을 묶어 표시하도록 지원한다.
- **O3**: **가능하면** 영속 백엔드 활성 시 기존 단일-키 엔트리를 시리즈 인코딩으로 변환하는
  일회성 마이그레이션 커맨드/유틸을 제공한다.
- **O4**: **가능하면** store 와 tsdb 의 역할 경계를 문서화하여 운영자가 어느 에이전트를 쓸지
  판단하도록 가이드한다 (명세 §store vs tsdb 참조).

## 명세 (Specifications)

### 시리즈 식별자 정의 및 정규화

시리즈 식별자는 다음으로 구성된다 (namespace 스코프 내):

```
SeriesID = (key, metric_type, sorted(tags))
```

정규화 규칙:

1. `metric_type` 가 빈 문자열이면 `"unknown"` 으로 보정 (`validateMetricType` 재사용).
2. `tags` 를 tag-key 사전순(ascending)으로 정렬. 빈 tags 는 빈 집합으로 취급.
3. `data_type` 은 식별에 사용하지 않는다 (시리즈의 값 타입 계약으로만 저장).

### 시리즈 키 인코딩 (후보)

`VolatileStore`(sync.Map) 및 영속 백엔드는 단일 string 키를 요구하므로 SeriesID 를 결정적
문자열로 직렬화해야 한다. 후보:

- **후보 1 (권장, O1 정렬)**: tsdb 스타일.
  `key|metric_type|k1=v1,k2=v2` 또는 `BuildSeriesKey(key+"#"+metric_type, sorted(tags))`.
  구분자는 metric_type/tag 문자집합(`^[a-zA-Z0-9_-]+$`, A3)과 겹치지 않는 `|` `#` `,` `=` 를
  사용하여 충돌을 피한다 (N5).
- **후보 2**: 안정적 직렬화 해시. `sha256(canonical(SeriesID))` 의 hex. 충돌 위험 무시 가능,
  단 디버깅 가독성 저하.
- **후보 3**: 구조적 키. sync.Map 키를 string 대신 비교 가능한 struct(`seriesKey{key,
  metric, tagsEncoded}`) 로 변경. 인코딩 가독성/안전성 최고, 단 `VolatileStore` 와
  `StoreRepository`(string key) 전반 시그니처 변경 폭이 큼.

> 구현 단계에서 후보 1 을 1순위로 평가하되, namespace prefix(`NamespacedStore`)와의 결합
> 순서(`namespace:` 가 바깥, 시리즈 인코딩이 안쪽)를 확정한다. 최종 결정은 plan.md 의 마일스톤
> M1 에서 PoC 후 확정한다.

### 저장 키잉 변경 (VolatileStore / staticKeys / NamespacedStore)

- `VolatileStore.data` 의 string 키가 (namespace prefix + 시리즈 인코딩) 이 된다. `storeItem`
  자체 구조는 유지 가능하나, 메타(metric/tags)는 시리즈 키에 흡수되므로 `storeItem` 에 시리즈
  메타 역참조 필드 추가를 검토한다.
- `StoreAgent.config.staticKeys` 의 키잉을 bare key → **시리즈 식별자**(또는 그 인코딩)로
  변경한다. 같은 key 의 여러 시리즈가 독립 엔트리로 존재한다. `StaticKeyMeta` 는 metric/tags 를
  이미 보유하므로, 레지스트리 키만 시리즈 단위로 승격하면 된다.
- `checkKeyAllowed`(gatekeeper)·`coerceWriteValue`·`SetKeyMeta`·`SetKeyDataType`·
  `StaticKeyMetaFor`·`StaticKeysSnapshot` 는 모두 (key) 대신 (SeriesID) 를 받도록 시그니처를
  확장한다. metric/tags 는 이미 쓰기 경로(SetWithMeta)에서 전달되므로 시리즈 식별에 활용 가능.
- `NamespacedStore` 는 `namespace:` prefix 를 시리즈 인코딩 **바깥쪽**에 붙인다 (A2/A6).

### 쓰기 경로 변경 (SetWithMeta / store-write)

- `NodeStoreAdapter.SetWithMeta(ctx, key, value, StoreWriteMeta{DataType, MetricType, Tags,
  TTL})` 는 이미 metric/tags 를 전달받는다. 변경점: 이 값들을 **키 메타 덮어쓰기**가 아니라
  **시리즈 식별자 구성**에 사용한다. 동일 (key, metric, tags) 재호출 → 같은 시리즈 갱신,
  다른 metric/tags → 새 시리즈.
- store-write 노드(`store_write.go`)는 변경 없이 동작 가능(이미 metric/tags 해석). 단, 한
  메시지가 여러 시리즈를 만들 수 있음을 acceptance 에서 검증한다 (A4).
- 일반 `Set`/`SetWithTTL`(메타 없는 경로)는 `metric_type="unknown"`, 빈 tags 의 **기본 시리즈**
  로 라우팅된다 (하위 호환: 메타 미지정 쓰기 = 단일 기본 시리즈).

### 조회 API 변경 (key → 다중 시리즈, metric/tags 필터)

- `POST /api/v1/store/{name}/query`:
  - 요청 바디에 선택적 `metric_type`, `tags`(map) 필터 필드를 추가한다.
  - 필터 생략 시: 해당 (namespace, key) 의 **모든 시리즈**를 반환한다. 응답 엔트리는 어느
    시리즈 소속인지 식별 가능하도록 `labels`(metric_type + tags) 를 채운다 (`chartQueryEntry.Labels`
    필드가 이미 존재 — 현재 nil. 이를 활용).
  - 필터 지정 시: 일치하는 단일/부분 시리즈만 반환한다 (E5/S3).
  - 다중 시리즈 반환 시 집계(SPEC-WEB-005 bucketAggregate)는 시리즈별로 적용하거나, 단일 시리즈
    필터를 요구하도록 제약한다 (plan 에서 결정).
- `GET /api/v1/store/{name}/keys`: 응답 행 단위를 key → **시리즈**로 변경한다. 각 행은
  `(key, metric_type, tags, data_type, registration)`. `?group_by=key`(O2) 는 선택.
  기존 `?tag=`/`?metric_type=`/`?data_type=`/`?registration=` 필터는 시리즈 행 필터로 그대로
  재사용 가능.
- `DELETE /keys/{key}` 단일 reset 은 시리즈 식별 파라미터(metric/tags)를 받아 **단일 시리즈**를
  대상으로 한다 (E8). 식별자 누락 시 정책(전체 key 의 모든 시리즈 vs 기본 시리즈)을 plan 에서
  확정한다.

### 영속 백엔드 영향 (PersistentStore / StoreRepository)

- `StoreRepository`(SetEntry/GetEntry/DeleteEntry/ListKeys/…) 는 string key 계약이므로 시리즈
  인코딩된 키를 그대로 사용하면 인터페이스 변경 없이 흡수 가능(후보 1/2). 후보 3(구조적 키)을
  택하면 Repository 시그니처도 변경된다.
- 활성 영속 백엔드가 없으므로(A1) 1차 구현은 인코딩 규칙만 정합화하고 실데이터 변환은 O3 로
  미룬다.

### UI 영향

- "TSDB 데이터 뷰어"(web/src)는 이미 시리즈 키/metric/tags 컬럼을 표시한다. `GET /keys` 가
  시리즈 행을 반환하면 자연 정합된다. `POST /query` 다중 시리즈 응답을 시리즈별 라인으로 분리
  렌더링하도록 차트 컴포넌트를 보강한다 (labels 기반 그룹핑).

### 기존 데이터 마이그레이션

- **인메모리(VolatileStore)**: 재시작 시 휘발되므로 별도 변환 불필요(A6). 재시작 후 신규 쓰기가
  시리즈 모델로 자연 흡수된다. 기존 yaml `staticKeys` 정의는 metric/tags 가 있으면 해당 시리즈로,
  없으면 `(key, "unknown", {})` 기본 시리즈로 로드된다.
- **영속(있다면)**: 기존 단일-키 엔트리를 읽어 `(key, "unknown", {})` 기본 시리즈로 재인코딩하는
  일회성 마이그레이션(O3). 변환 전 백업, 변환 후 검증을 plan 의 마일스톤으로 정의한다.

### store vs tsdb 역할 구분 (중복 해소)

이미 `tsdb` 에이전트(`tsdb_agent.go`, `internal/tsdb`)가 (measurement + tag set) 시리즈
저장소로 존재한다. 본 SPEC 으로 store 가 시리즈 모델을 갖게 되면 역할이 중첩될 수 있으므로 경계를
명확히 한다:

| 측면 | Store (SPEC-STORE-004) | TSDB (SPEC-TSDB-001) |
| --- | --- | --- |
| 1차 목적 | **현재값/최근값 중심 KV + 제한적 시계열(history)** | **본격 시계열(대량 포인트, retention/eviction)** |
| 식별 | `(key, metric_type, sorted(tags))` (namespace 스코프) | `(measurement, tags)` 시리즈 |
| 보관 | history 개수/TTL 제한(maxHistorySize, historyTTL) | MaxPointsPerSeries, MaxAge, 메모리 상한, eviction loop |
| 접근 | 키 단위 Get/Set + QueryHistory | 시리즈 단위 Write + QueryRange/Latest, subscriber |
| 권장 사용 | 디바이스 상태/설정/최근 측정의 빠른 KV 조회 | 고빈도·장기 보존이 필요한 메트릭 |

> 결정 필요(미결): store 의 시리즈화가 tsdb 와 충분히 차별화되는지, 아니면 store 시리즈를
> tsdb 위임으로 통합할지. 본 SPEC 은 **두 에이전트를 독립 유지**하되 식별 모델 규약(시리즈 키
> 인코딩 O1)을 공유하는 방향을 제안한다. 통합 여부는 별도 결정 사항으로 남긴다.

## 영향 파일 (Impact Surface — 요약)

- `internal/agent/system/store_volatile.go` (저장 키잉, history 단위)
- `internal/agent/system/store_data_type.go` (`StaticKeyMeta`, 시리즈 메타)
- `internal/agent/system/store.go` (`staticKeys` 키잉, gatekeeper/coerce/SetKeyMeta/SetKeyDataType/snapshot)
- `internal/agent/system/store_namespace.go` (prefix ↔ 시리즈 인코딩 결합 순서)
- `internal/agent/system/store_node_adapter.go` (SetWithMeta → 시리즈 라우팅)
- `internal/node/store_write.go` (다중 시리즈 쓰기 검증; 코드 변경 최소)
- `internal/agent/system/store_query.go` (QueryHistory 다중 시리즈, ListStoreKeys, IsStaticKey)
- `internal/api/handler/store_query.go` (POST /query metric/tags 필터, GET /keys 시리즈 행, DELETE 시리즈 대상)
- `internal/agent/system/store_persistent.go` (시리즈 인코딩 정합; 1차는 규칙만)
- `internal/tsdb/series.go` (BuildSeriesKey 규약 공유 — 참조/재사용)
- `web/src/**` (TSDB 데이터 뷰어 시리즈 행/차트 라인 분리)

## 트레이서빌리티 (Traceability)

- 본 SPEC ID: `SPEC-STORE-004` — 구현/테스트/문서에 `@spec SPEC-STORE-004` 태그를 부착한다.
- 선행: SPEC-STORE-001(저장소 기반), SPEC-STORE-002, SPEC-STORE-003(정적 키/data_type/metric_type/tags).
- 연관: SPEC-TSDB-001(시리즈 모델 규약), SPEC-CHART-001/SPEC-WEB-005(조회/집계 API).
- 상세 수용 기준은 `acceptance.md`, 구현 계획은 `plan.md` 참조.
