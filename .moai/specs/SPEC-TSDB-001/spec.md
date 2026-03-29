---
id: SPEC-TSDB-001
version: "1.1.0"
status: completed
created: "2026-03-19"
updated: "2026-03-27"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-19 | 1.0.0 | 초기 SPEC 작성 - 인메모리 시계열 데이터베이스 |
| 2026-03-27 | 1.1.0 | 전체 구현 완료: TSDBAgent 시스템 에이전트, tsdb-write/tsdb-query 노드, REST API 핸들러(POST /tsdb/query), WebSocket 실시간 구독. Start() Stopped 상태 복구 로직 포함 |

---

# SPEC-TSDB-001: 인메모리 시계열 데이터베이스 (In-Memory Time-Series Database)

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 플랫폼에 내장되는 인메모리 시계열 데이터베이스(TSDB)를 구현한다. IoT 센서 데이터, 메트릭, 상태 이력 등 시간 기반 데이터를 메모리에 효율적으로 저장하고 조회하는 경량 스토리지 엔진이다.

외부 TSDB(InfluxDB, Prometheus 등)에 의존하지 않고 xflow 자체적으로 시계열 데이터를 관리할 수 있어, 에지 환경이나 단독 배포 시나리오에서 외부 인프라 없이 데이터 시각화와 실시간 모니터링이 가능하다.

### 1.2 기술 스택

| 항목 | 기술 |
|------|------|
| 언어 | Go 1.23+ |
| 동시성 | sync.RWMutex (시리즈별), atomic (메트릭 카운터) |
| 직렬화 | encoding/json, encoding/binary |
| 시간 | time 표준 라이브러리 |
| 테스트 | testing + testify |
| 메트릭 | Prometheus client_golang (관찰성 통합) |

### 1.3 설계 원칙

- **제로 외부 의존성**: Go 표준 라이브러리 + 기존 xflow 의존성만 사용
- **시리즈별 격리**: 각 시리즈가 독립적인 잠금과 저장소를 보유하여 교차 간섭 최소화
- **설정 가능한 보존 정책**: 시간/개수/용량 기반 제한을 시리즈별 또는 글로벌로 설정
- **Lazy + Periodic 이중 퇴거**: 읽기/쓰기 시 만료 검사 + 백그라운드 정리 고루틴
- **xflow 통합 우선**: Node, Agent, API, WebSocket 등 기존 xflow 아키텍처에 자연스럽게 통합

### 1.4 범위

**포함**:
- 인메모리 시계열 저장 엔진 (internal/tsdb/)
- 보존 정책 (시간/개수/용량)
- 범위 쿼리 및 집계 함수
- xflow 노드 통합 (tsdb-write, tsdb-query)
- REST API 쿼리 엔드포인트
- WebSocket 실시간 구독

**미포함**:
- 디스크 영속화 (WAL, 스냅샷) - 후속 SPEC으로 분리
- 분산 복제 / 클러스터링
- SQL 쿼리 인터페이스
- 외부 TSDB 호환 프로토콜 (InfluxDB Line Protocol, Prometheus Remote Write)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| Series (시리즈) | 동일한 measurement와 태그 조합을 가진 데이터 포인트의 시간순 집합. 예: `temperature{room=101,floor=2}` |
| DataPoint (데이터 포인트) | 타임스탬프와 하나 이상의 필드 값으로 구성된 단일 측정값 |
| Measurement (측정) | 데이터의 논리적 이름. 관계형 DB의 테이블에 대응 |
| Tag (태그) | 시리즈를 구분하는 인덱싱된 문자열 키-값 쌍. 카디널리티 결정 요소 |
| Field (필드) | 실제 측정값을 담는 키-값 쌍. float64, int64, string, bool 타입 지원 |
| Retention (보존) | 데이터를 유지하는 기간/개수/용량 정책 |
| Eviction (퇴거) | 보존 정책을 초과한 데이터 포인트를 삭제하는 프로세스 |
| Bucket (버킷) | 다운샘플링 시 데이터를 그룹화하는 시간 구간 (예: 1m, 5m, 1h) |
| Down-sampling (다운샘플링) | 고해상도 데이터를 저해상도로 축소하여 요약하는 과정 |
| Series Key (시리즈 키) | measurement와 정렬된 태그의 조합으로 시리즈를 고유 식별하는 문자열 |
| Ingestion (수집) | 데이터 포인트를 TSDB에 기록하는 과정 |

---

## 3. Assumptions (가정 사항)

| ID | 가정 | 근거 | 위험도 |
|----|------|------|--------|
| A1 | 데이터 포인트의 타임스탬프는 나노초 정밀도(time.Time)를 사용한다 | Go 표준 time.Time이 나노초 해상도 제공 | 낮음 |
| A2 | 시리즈 카디널리티(고유 시리즈 수)는 단일 인스턴스에서 10,000 이하이다 | IoT 게이트웨이 환경 기준 센서 수 예상치 | 중간 |
| A3 | 단일 시리즈 내 데이터 포인트는 시간순으로 수집된다 (out-of-order 비율 < 5%) | IoT 센서의 일반적인 데이터 전송 패턴 | 낮음 |
| A4 | 인메모리 데이터는 프로세스 재시작 시 유실된다 (영속화는 후속 SPEC) | MVP 범위 제한, 에지 환경의 일시적 데이터 활용 | 중간 |
| A5 | 필드 값의 주요 타입은 float64이며, int64/string/bool도 지원한다 | 센서 데이터의 대부분은 숫자 측정값 | 낮음 |
| A6 | xflow의 기존 Message Payload 구조(map[string]any)에서 데이터를 추출하여 저장한다 | 기존 노드 시스템과의 호환성 | 낮음 |

---

## 4. Requirements (요구사항)

### Module 1: Core TSDB Engine (코어 TSDB 엔진)

**[REQ-TSDB-001]** 시스템은 **항상** DataPoint를 `Series Key` 기준으로 분류하여 저장해야 한다.
- Series Key 형식: `{measurement},{tag1}={val1},{tag2}={val2}` (태그는 키 기준 알파벳 정렬)
- 동일 Series Key의 DataPoint는 타임스탬프 기준 시간순으로 정렬되어 저장

**[REQ-TSDB-002]** 시스템은 **항상** DataPoint를 다음 구조로 저장해야 한다:
- Timestamp: `time.Time` (나노초 정밀도)
- Fields: `map[string]any` (float64, int64, string, bool 지원)

**[REQ-TSDB-003]** **WHEN** 새로운 DataPoint가 기록될 때 **THEN** 해당 시리즈의 인덱스를 업데이트하고, 시리즈가 존재하지 않으면 자동으로 생성해야 한다.

**[REQ-TSDB-004]** 시스템은 **항상** 시리즈별 `sync.RWMutex`를 사용하여 동시 읽기는 허용하고 쓰기는 배타적으로 처리해야 한다.

**[REQ-TSDB-005]** 시스템은 **항상** 시리즈별 DataPoint를 정렬된 슬라이스(`[]DataPoint`)에 저장하여 이진 탐색 기반 범위 쿼리를 지원해야 한다.

**[REQ-TSDB-006]** **WHEN** out-of-order DataPoint가 수집될 때 **THEN** 올바른 시간순 위치에 삽입해야 한다 (`sort.Search` 기반 삽입 위치 탐색).

### Module 2: Retention Policies (보존 정책)

**[REQ-TSDB-010]** 시스템은 **항상** 다음 3가지 보존 정책을 지원해야 한다:
- **시간 기반**: 지정된 기간(`time.Duration`)보다 오래된 DataPoint 퇴거
- **개수 기반**: 시리즈당 최대 DataPoint 수 초과 시 가장 오래된 데이터부터 퇴거
- **용량 기반**: 전체 TSDB의 추정 메모리 사용량이 임계값 초과 시 가장 오래된 시리즈의 가장 오래된 데이터부터 퇴거

**[REQ-TSDB-011]** **IF** 시리즈별 보존 정책이 설정되어 있으면 **THEN** 해당 시리즈에 시리즈별 정책을 적용하고, 그렇지 않으면 글로벌 기본 정책을 적용해야 한다.

**[REQ-TSDB-012]** **WHILE** TSDB가 실행 중일 때 **AND WHEN** 보존 정책 검사 주기(기본: 30초)에 도달할 때 **THEN** 백그라운드 고루틴이 만료된 DataPoint를 퇴거해야 한다.

**[REQ-TSDB-013]** **WHEN** DataPoint가 기록 또는 조회될 때 **THEN** 해당 시리즈의 보존 정책을 lazy하게 검사하여 만료 데이터를 즉시 정리해야 한다.

**[REQ-TSDB-014]** 가능하면 보존 정책 설정을 런타임에 변경(Hot Reload)할 수 있어야 한다.

**[REQ-TSDB-015]** 시스템은 **항상** 각 DataPoint의 추정 메모리 크기를 추적해야 한다:
- 기본 오버헤드: 타임스탬프(24바이트) + 맵 오버헤드(8바이트)
- 필드별: 키 문자열 크기 + 값 크기 (float64=8, int64=8, bool=1, string=len)

### Module 3: Query Engine (쿼리 엔진)

**[REQ-TSDB-020]** **WHEN** 시간 범위 쿼리가 요청될 때 **THEN** 시작/종료 타임스탬프 사이의 DataPoint를 이진 탐색으로 효율적으로 반환해야 한다.

**[REQ-TSDB-021]** **WHEN** 최신 N개 포인트 쿼리가 요청될 때 **THEN** 시리즈의 마지막 N개 DataPoint를 반환해야 한다.

**[REQ-TSDB-022]** **WHEN** 집계 쿼리가 요청될 때 **THEN** 지정된 시간 범위와 필드에 대해 다음 집계 함수를 지원해야 한다:
- `min`: 최솟값
- `max`: 최댓값
- `avg`: 평균값
- `sum`: 합계
- `count`: 개수
- `first`: 범위 내 첫 번째 값
- `last`: 범위 내 마지막 값

**[REQ-TSDB-023]** **WHEN** 다운샘플링 쿼리가 요청될 때 **THEN** 지정된 버킷 간격(`time.Duration`)으로 데이터를 그룹화하고 각 버킷에 집계 함수를 적용한 결과를 반환해야 한다.

**[REQ-TSDB-024]** **WHEN** 시리즈 목록 쿼리가 요청될 때 **THEN** measurement 이름 또는 태그 키-값 기반 필터링으로 일치하는 시리즈 키 목록을 반환해야 한다.

**[REQ-TSDB-025]** 시스템은 쿼리 결과의 최대 반환 포인트 수를 **항상** 제한해야 한다(하지 않으면 않아야 한다: 메모리 과다 사용 방지). 기본 제한: 10,000 포인트.

### Module 4: xflow Node Integration (xflow 노드 통합)

**[REQ-TSDB-030]** 시스템은 **항상** `tsdb-write` 노드 타입을 제공해야 한다:
- 입력: xflow Message (Payload에서 measurement, tags, fields 추출)
- 설정: measurement 이름 (고정 또는 Payload 키 참조), 태그 매핑, 필드 매핑
- 동작: Message를 DataPoint로 변환하여 TSDB에 기록
- 출력: 원본 Message를 그대로 통과 (pass-through)

**[REQ-TSDB-031]** 시스템은 **항상** `tsdb-query` 노드 타입을 제공해야 한다:
- 입력: xflow Message (쿼리 파라미터를 Payload 또는 노드 설정에서 추출)
- 설정: measurement, 태그 필터, 시간 범위, 집계 함수, 버킷 간격
- 동작: 쿼리 실행 후 결과를 Message Payload에 설정
- 출력: 쿼리 결과가 포함된 Message

**[REQ-TSDB-032]** **WHEN** `tsdb-write` 노드가 플로우에 배포될 때 **THEN** 공유 TSDB 인스턴스에 접근하여 데이터를 기록해야 한다 (TSDB는 xflowd 프로세스당 싱글톤).

**[REQ-TSDB-033]** **WHEN** `tsdb-write` 또는 `tsdb-query` 노드에서 에러가 발생할 때 **THEN** 에러 포트로 에러 메시지를 전달해야 한다.

### Module 5: API & WebSocket (API 및 웹소켓)

**[REQ-TSDB-040]** 시스템은 **항상** 다음 REST API 엔드포인트를 제공해야 한다:
- `POST /api/v1/tsdb/write`: DataPoint 배치 기록
- `POST /api/v1/tsdb/query`: 쿼리 실행 (범위 조회, 집계, 다운샘플링)
- `GET /api/v1/tsdb/series`: 시리즈 목록 조회
- `GET /api/v1/tsdb/series/{key}/latest`: 특정 시리즈의 최신 DataPoint 조회
- `GET /api/v1/tsdb/stats`: TSDB 통계 (시리즈 수, 전체 포인트 수, 메모리 사용량)
- `DELETE /api/v1/tsdb/series/{key}`: 시리즈 삭제

**[REQ-TSDB-041]** **WHEN** WebSocket 구독 요청이 수신될 때 **THEN** 지정된 시리즈에 새로운 DataPoint가 기록될 때마다 실시간으로 해당 구독자에게 전달해야 한다.
- 엔드포인트: `WS /api/v1/tsdb/subscribe`
- 구독 메시지: `{"action": "subscribe", "series": ["temperature,room=101"]}`
- 해제 메시지: `{"action": "unsubscribe", "series": ["temperature,room=101"]}`

**[REQ-TSDB-042]** 시스템은 API 응답에서 시간 필드를 **항상** RFC3339Nano 형식 또는 Unix 나노초 정수로 반환해야 한다 (클라이언트 선택 가능).

**[REQ-TSDB-043]** 시스템은 API 쿼리 요청에 **항상** 타임아웃(기본: 10초)을 적용하여 장시간 쿼리가 시스템을 차단하지 않아야 한다.

---

## 5. Acceptance Criteria (인수 기준)

### Module 1: Core TSDB Engine

| ID | 기준 | 검증 방법 |
|----|------|----------|
| AC-001 | 10개의 서로 다른 시리즈에 각 1,000개의 DataPoint를 기록한 후, 각 시리즈에서 시간순으로 정렬된 상태로 조회 가능 | 단위 테스트 |
| AC-002 | 10개의 고루틴이 동시에 같은 시리즈에 기록할 때 데이터 레이스 없이 모든 포인트 저장됨 | `go test -race` |
| AC-003 | Out-of-order 타임스탬프를 가진 DataPoint가 올바른 위치에 삽입됨 | 단위 테스트 |
| AC-004 | 존재하지 않는 시리즈에 첫 DataPoint를 기록하면 시리즈가 자동 생성됨 | 단위 테스트 |

### Module 2: Retention Policies

| ID | 기준 | 검증 방법 |
|----|------|----------|
| AC-010 | 시간 기반 보존: 1시간 정책 설정 후 1시간 이전 데이터가 퇴거됨 | 단위 테스트 (시간 모킹) |
| AC-011 | 개수 기반 보존: 최대 100개 설정 후 101번째 기록 시 가장 오래된 1개 퇴거됨 | 단위 테스트 |
| AC-012 | 용량 기반 보존: 메모리 제한 1MB 설정 후 초과 시 데이터 퇴거됨 | 단위 테스트 |
| AC-013 | 시리즈별 보존 정책이 글로벌 정책보다 우선 적용됨 | 단위 테스트 |
| AC-014 | 백그라운드 퇴거 고루틴이 설정 주기대로 실행됨 | 통합 테스트 |

### Module 3: Query Engine

| ID | 기준 | 검증 방법 |
|----|------|----------|
| AC-020 | 10,000개 포인트 중 특정 시간 범위(1시간)의 포인트만 정확히 반환됨 | 단위 테스트 |
| AC-021 | 최신 50개 포인트 요청 시 정확히 50개 반환됨 | 단위 테스트 |
| AC-022 | avg/min/max/sum/count 집계가 수학적으로 정확한 결과 반환 | 단위 테스트 |
| AC-023 | 5분 버킷 다운샘플링 시 각 버킷에 올바른 집계 값 반환 | 단위 테스트 |
| AC-024 | 쿼리 결과가 최대 반환 제한(10,000)을 초과하지 않음 | 단위 테스트 |

### Module 4: xflow Node Integration

| ID | 기준 | 검증 방법 |
|----|------|----------|
| AC-030 | tsdb-write 노드가 Message Payload에서 measurement/tags/fields를 추출하여 TSDB에 기록 | 단위 테스트 |
| AC-031 | tsdb-query 노드가 설정된 쿼리를 실행하고 결과를 출력 Message에 포함 | 단위 테스트 |
| AC-032 | tsdb-write 노드가 원본 Message를 그대로 통과시킴 (pass-through) | 단위 테스트 |
| AC-033 | 잘못된 Payload 형식 시 에러 포트로 에러 메시지 전달 | 단위 테스트 |
| AC-034 | 노드 레지스트리에 tsdb-write, tsdb-query 타입이 등록됨 | 통합 테스트 |

### Module 5: API & WebSocket

| ID | 기준 | 검증 방법 |
|----|------|----------|
| AC-040 | POST /tsdb/write로 배치 기록 후 POST /tsdb/query로 조회 가능 | API 테스트 |
| AC-041 | GET /tsdb/stats가 시리즈 수, 포인트 수, 메모리 사용량 반환 | API 테스트 |
| AC-042 | WebSocket 구독 후 새 데이터 기록 시 실시간으로 수신 확인 | 통합 테스트 |
| AC-043 | DELETE /tsdb/series/{key}로 시리즈 삭제 후 조회 시 빈 결과 | API 테스트 |
| AC-044 | 쿼리 타임아웃(10초) 초과 시 408 응답 반환 | API 테스트 |

---

## 6. Technical Approach (기술 접근)

### 6.1 패키지 구조

```
internal/tsdb/
  tsdb.go          # TSDB 인터페이스, 싱글톤 팩토리
  series.go        # Series 구조체, DataPoint 저장/조회
  datapoint.go     # DataPoint 구조체, 필드 타입
  retention.go     # RetentionPolicy, 퇴거 로직
  eviction.go      # 백그라운드 퇴거 고루틴
  query.go         # 쿼리 엔진 (범위, 집계, 다운샘플링)
  memory.go        # 메모리 사용량 추정
  config.go        # TSDB 설정 구조체
  subscriber.go    # WebSocket 구독 관리
  errors.go        # 센티널 에러 정의
  tsdb_test.go     # 코어 테스트
  series_test.go   # 시리즈 테스트
  retention_test.go # 보존 정책 테스트
  query_test.go    # 쿼리 엔진 테스트

internal/node/adapter/
  tsdb.go          # tsdb-write, tsdb-query 노드 어댑터
  tsdb_test.go     # 노드 어댑터 테스트

internal/api/handler/
  tsdb.go          # TSDB REST API 핸들러
  tsdb_test.go     # API 핸들러 테스트
```

### 6.2 핵심 데이터 구조

```go
// DataPoint - 단일 시계열 측정값
type DataPoint struct {
    Timestamp time.Time
    Fields    map[string]any  // float64, int64, string, bool
}

// Series - 동일 시리즈 키의 데이터 포인트 집합
type Series struct {
    Key        string           // "temperature,room=101,floor=2"
    Points     []DataPoint      // 시간순 정렬
    mu         sync.RWMutex     // 시리즈별 잠금
    Retention  *RetentionPolicy // nil이면 글로벌 정책 사용
    memorySize int64            // 추정 메모리 사용량 (바이트)
}

// RetentionPolicy - 보존 정책
type RetentionPolicy struct {
    MaxAge      time.Duration // 0이면 무제한
    MaxPoints   int           // 0이면 무제한
    MaxBytes    int64         // 0이면 무제한
}

// TSDB - 시계열 데이터베이스 인터페이스
type TSDB interface {
    Write(measurement string, tags map[string]string, fields map[string]any, ts time.Time) error
    WriteBatch(points []WriteRequest) error
    Query(q *Query) (*QueryResult, error)
    Latest(seriesKey string, n int) ([]DataPoint, error)
    Series(filter *SeriesFilter) ([]string, error)
    DeleteSeries(key string) error
    Stats() *Stats
    Subscribe(seriesKeys []string, ch chan<- DataPoint) (unsubscribe func())
    Close() error
}
```

### 6.3 Series Key 생성 규칙

```
{measurement},{tag1_key}={tag1_val},{tag2_key}={tag2_val}
```

- 태그는 키 기준 알파벳 순 정렬
- 태그가 없으면 measurement 이름만 사용
- 예: `temperature,floor=2,room=101`

### 6.4 동시성 전략

- **글로벌 시리즈 맵**: `sync.RWMutex`로 보호되는 `map[string]*Series`
  - 시리즈 조회: 읽기 잠금
  - 시리즈 생성/삭제: 쓰기 잠금
- **시리즈별 데이터**: 각 `Series`가 자체 `sync.RWMutex` 보유
  - 읽기 쿼리: 읽기 잠금 (병렬 허용)
  - 쓰기: 쓰기 잠금
- **메모리 카운터**: `atomic.Int64`로 전체 메모리 추정치 관리
- **구독자 알림**: 데이터 기록 후 비차단(non-blocking) 채널 전송

### 6.5 퇴거 전략

1. **Lazy Eviction**: 쓰기/읽기 시 해당 시리즈의 만료 데이터 검사 및 정리
2. **Periodic Eviction**: 백그라운드 고루틴이 주기적(기본 30초)으로 모든 시리즈 순회하며 만료 데이터 정리
3. **Capacity Eviction**: 전체 메모리 추정치가 용량 제한을 초과하면 가장 오래된 데이터부터 삭제

### 6.6 xflow 통합 아키텍처

```
                  ┌──────────────┐
                  │  TSDB Engine │  (internal/tsdb/, 싱글톤)
                  │              │
                  │  ┌────────┐  │
                  │  │Series 1│  │
                  │  │Series 2│  │
                  │  │Series N│  │
                  │  └────────┘  │
                  └──────┬───────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
    ┌────┴────┐    ┌─────┴─────┐   ┌─────┴──────┐
    │tsdb-write│   │tsdb-query │   │  REST API  │
    │  (Node)  │   │  (Node)   │   │  Handler   │
    └─────────┘   └───────────┘   └────────────┘
         │               │               │
    ┌────┴────┐    ┌─────┴─────┐   ┌─────┴──────┐
    │  Flow   │   │   Flow    │   │ WebSocket  │
    │ Engine  │   │  Engine   │   │ Subscribe  │
    └─────────┘   └───────────┘   └────────────┘
```

### 6.7 설정 구조 (Viper 통합)

```yaml
tsdb:
  enabled: true
  retention:
    max_age: "24h"          # 기본 시간 보존
    max_points: 100000      # 시리즈당 최대 포인트
    max_bytes: "256MB"      # 전체 메모리 제한
  eviction:
    interval: "30s"         # 백그라운드 퇴거 주기
  query:
    max_points: 10000       # 쿼리당 최대 반환 포인트
    timeout: "10s"          # 쿼리 타임아웃
  series:
    # 시리즈별 커스텀 보존 정책
    overrides:
      "temperature,*":
        max_age: "7d"
      "status,*":
        max_points: 1000
```

### 6.8 메모리 추정 방식

- DataPoint 기본 오버헤드: `time.Time`(24) + `map` 헤더(8) + 슬라이스 포인터(24) = ~56바이트
- 필드별 추가: 키 문자열(16+len) + 값(float64=8, int64=8, string=16+len, bool=1)
- 시리즈 오버헤드: 키 문자열(16+len) + 뮤텍스(8) + 슬라이스 헤더(24) + 정책 포인터(8) = ~60바이트
- 실제 메모리 = 합산 추정치 * 1.2 (Go 런타임 오버헤드 보정)

---

## 7. Traceability (추적성)

| 요구사항 | 관련 파일 | 테스트 |
|---------|----------|--------|
| REQ-TSDB-001~006 | internal/tsdb/series.go, datapoint.go | series_test.go |
| REQ-TSDB-010~015 | internal/tsdb/retention.go, eviction.go, memory.go | retention_test.go |
| REQ-TSDB-020~025 | internal/tsdb/query.go | query_test.go |
| REQ-TSDB-030~033 | internal/node/adapter/tsdb.go | tsdb_test.go (adapter) |
| REQ-TSDB-040~043 | internal/api/handler/tsdb.go | tsdb_test.go (handler) |

---

*SPEC 버전: 1.0.0*
*최종 수정: 2026-03-19*
*작성: manager-spec*
