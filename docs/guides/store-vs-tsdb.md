# Store vs TSDB 에이전트 역할 구분

> `@spec SPEC-STORE-004` (O4) — Store 가 복합 식별(시리즈) 모델을 갖게 되면서 TSDB 에이전트와
> 역할이 중첩될 수 있다. 본 문서는 두 에이전트의 경계를 명확히 하여 운영자가 어느 에이전트를
> 쓸지 판단하도록 가이드한다.

## 한눈에 보기

| 측면 | Store (SPEC-STORE-004) | TSDB (SPEC-TSDB-001) |
| --- | --- | --- |
| 1차 목적 | 현재값/최근값 중심 KV + 제한적 시계열(history) | 본격 시계열(대량 포인트, retention/eviction) |
| 식별 | `(key, metric_type, sorted(tags))` (namespace 스코프) | `(measurement, tags)` 시리즈 |
| 보관 | history 개수/TTL 제한 (`maxHistorySize`, `historyTTL`) | `MaxPointsPerSeries`(기본 100000), `MaxAge`(기본 24h), 메모리 상한, eviction loop |
| 접근 | 키 단위 Get/Set + QueryHistory | 시리즈 단위 Write + QueryRange/Latest, subscriber |
| 권장 사용 | 디바이스 상태/설정/최근 측정의 빠른 KV 조회 | 고빈도·장기 보존이 필요한 메트릭 |
| 백엔드 | VolatileStore(인메모리, 활성). PersistentStore 는 인터페이스만 존재 (A1) | 인메모리 시계열 DB + eviction |

## 언제 무엇을 쓰는가

### Store 를 선택하는 경우

- 디바이스의 **현재 상태/설정값**을 빠르게 읽고 써야 할 때 (예: 마지막 온도, on/off 상태).
- 값의 **최근 변화 몇 건**(history)만 필요하고 장기 보존이 불필요할 때.
- 키 단위로 "이 키의 모든 측정(시리즈)을 한 번에" 조회하고 싶을 때
  (SPEC-STORE-004: 키 조회 시 전체 시리즈 fan-out, metric/tags 필터로 좁히기).

### TSDB 를 선택하는 경우

- **고빈도 샘플링**(초당 수~수백 포인트)을 장기간 누적해야 할 때.
- retention/eviction(`MaxAge`, `MaxPointsPerSeries`, 메모리 상한)으로 **자동 정리**가 필요할 때.
- 시간 범위 질의(`QueryRange`), 구독(subscriber) 등 **시계열 전용 접근 패턴**이 필요할 때.

## 식별 모델 관계 (공유 규약)

두 에이전트는 **독립 유지**하되, 시리즈 식별의 **하위 문법만 공유**한다 (O1):

- TSDB: `tsdb.BuildSeriesKey(measurement, tags)` → `"measurement,k1=v1,k2=v2"` (태그 key 사전순 정렬).
- Store: `system.EncodeSeriesKey(SeriesID{Key, MetricType, Tags})` → `"metric_type|tagsEncoded|key"`.
  - 이때 `tagsEncoded` 부분(`k1=v1,k2=v2`, 정렬)은 TSDB 태그 인코딩과 **동일한 하위 문법**이다.
  - 바깥 프레임(`metric|tags|key`)은 Store 고유다. Store 는 TSDB 에 없는 **임의 Key 차원**을
    포함하므로 (디바이스 UUID 등 구분자 포함 가능), measurement 위치에 임의 key 를 넣으면
    구분자 충돌(N5)이 발생한다. 따라서 제약된 문자집합 필드(metric, tags)를 앞쪽에 배치하여
    `|` 구분자 위치를 결정적으로 만들고, 임의 key 는 두 번째 `|` 이후로 안전하게 흡수한다.

## 결정 사항: 통합 여부

- **결정**: 본 SPEC(STORE-004)에서는 **두 에이전트를 독립 유지**한다. Store 는 KV + 제한적
  시계열, TSDB 는 본격 시계열로 사용처를 분리한다. 공유하는 것은 **시리즈 인코딩 하위 규약**뿐이다.
- **분리된 결정**: Store 시리즈를 TSDB 위임으로 **완전 통합**할지는 **별도 결정 사항**으로 남긴다.
  본 SPEC 의 범위는 Store 의 식별 모델을 시리즈로 통일하는 데 한정한다.
- **고카디널리티 주의**: Store 에서 한 key 가 다수 tags 조합으로 분할되면 시리즈 수가 폭증할 수
  있다. 고빈도·고카디널리티 메트릭은 Store 대신 TSDB 사용을 권장한다.

## 참고

- 식별/인코딩 구현: `internal/agent/system/store_series.go` (`SeriesID`, `EncodeSeriesKey`).
- TSDB 시리즈 키: `internal/tsdb/series.go` (`BuildSeriesKey`).
- 영속 정합/마이그레이션: `internal/agent/system/store_persistent.go`,
  `internal/migrate/storeseries/`, `docs/migration/v0.4.0-store-series.md`.
- 전체 명세: `.moai/specs/SPEC-STORE-004/spec.md` (§store vs tsdb 역할 구분).
