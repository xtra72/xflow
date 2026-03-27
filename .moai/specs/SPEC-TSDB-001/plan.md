---
id: SPEC-TSDB-001
type: plan
version: "1.0.0"
created: "2026-03-19"
updated: "2026-03-19"
---

# SPEC-TSDB-001: 구현 계획

## 1. 마일스톤

### Primary Goal: 코어 TSDB 엔진 + 보존 정책

**대상 모듈**: Module 1 (Core TSDB Engine), Module 2 (Retention Policies)

**구현 순서**:

1. **internal/tsdb/errors.go**: 센티널 에러 정의
   - ErrSeriesNotFound, ErrInvalidMeasurement, ErrInvalidField, ErrQueryTimeout, ErrMaxPointsExceeded 등

2. **internal/tsdb/datapoint.go**: DataPoint 구조체
   - DataPoint 구조체 (Timestamp, Fields)
   - EstimateSize() 메모리 추정 함수
   - 지원 필드 타입 상수 정의

3. **internal/tsdb/config.go**: TSDB 설정 구조체
   - TSDBConfig, RetentionConfig, EvictionConfig, QueryConfig
   - Viper 통합 키 등록
   - 기본값 정의

4. **internal/tsdb/retention.go**: 보존 정책
   - RetentionPolicy 구조체 (MaxAge, MaxPoints, MaxBytes)
   - ApplyTimeRetention(), ApplyCountRetention() 함수
   - IsExpired() 판별 함수

5. **internal/tsdb/series.go**: 시리즈 저장소
   - Series 구조체 (Key, Points, mu, Retention, memorySize)
   - Write(dp DataPoint) error
   - QueryRange(start, end time.Time) []DataPoint
   - Latest(n int) []DataPoint
   - Evict() - lazy eviction
   - SeriesKey 생성 함수: BuildSeriesKey(measurement, tags)
   - 이진 탐색 기반 삽입/조회

6. **internal/tsdb/memory.go**: 메모리 추정
   - EstimatePointSize(dp DataPoint) int64
   - 글로벌 atomic.Int64 카운터 관리

7. **internal/tsdb/eviction.go**: 백그라운드 퇴거
   - Evictor 구조체 (고루틴 관리)
   - Start()/Stop() 생명주기
   - 주기적 시리즈 순회 및 만료 데이터 정리
   - 용량 기반 퇴거 로직

8. **internal/tsdb/tsdb.go**: TSDB 인터페이스 및 구현체
   - TSDB 인터페이스 정의
   - defaultTSDB 구현체 (map[string]*Series + sync.RWMutex)
   - New(config TSDBConfig) TSDB 팩토리
   - Write/WriteBatch 구현
   - Close() - 고루틴 정리, 리소스 해제

**테스트**: 각 파일별 테이블 기반 단위 테스트, `go test -race` 동시성 검증

### Secondary Goal: 쿼리 엔진

**대상 모듈**: Module 3 (Query Engine)

**구현 순서**:

1. **internal/tsdb/query.go**: 쿼리 엔진
   - Query 구조체 (SeriesKey, Start, End, Aggregation, BucketInterval, Limit)
   - QueryResult 구조체
   - AggregateFunc 타입 (min, max, avg, sum, count, first, last)
   - Execute(q *Query) (*QueryResult, error)
   - 다운샘플링 로직 (버킷 그룹화 + 집계)

2. **SeriesFilter**: 시리즈 검색
   - measurement 기반 필터링
   - 태그 키-값 기반 필터링
   - 와일드카드 매칭

**테스트**: 다양한 집계 함수의 수학적 정확성 검증, 다운샘플링 버킷 경계값 테스트

### Tertiary Goal: xflow 노드 통합

**대상 모듈**: Module 4 (xflow Node Integration)

**구현 순서**:

1. **internal/node/adapter/tsdb.go**: TSDB 노드 어댑터
   - TSDBWriteAdapter: Node 인터페이스 구현
     - Config: measurement, tag_mappings, field_mappings
     - Process(): Message Payload에서 데이터 추출 -> TSDB Write
     - Pass-through: 원본 메시지 그대로 출력
   - TSDBQueryAdapter: Node 인터페이스 구현
     - Config: measurement, tags, time_range, aggregation, bucket
     - Process(): 쿼리 실행 -> 결과를 출력 Message Payload에 설정

2. **internal/node/adapter/register.go**: 노드 타입 등록
   - "tsdb-write", "tsdb-query" 노드 타입을 레지스트리에 등록

3. **YAML 스키마**: examples/flows/ 에 TSDB 활용 예제 플로우 추가

**테스트**: 노드 어댑터 단위 테스트, Message Payload 매핑 검증

### Optional Goal: API & WebSocket

**대상 모듈**: Module 5 (API & WebSocket)

**구현 순서**:

1. **internal/tsdb/subscriber.go**: 구독 관리
   - Subscriber 구조체 (시리즈별 구독자 채널 관리)
   - Subscribe(keys, ch) unsubscribe func
   - Notify(key, dp) - 비차단 채널 전송

2. **internal/api/handler/tsdb.go**: REST API 핸들러
   - HandleWrite: POST /tsdb/write
   - HandleQuery: POST /tsdb/query
   - HandleSeries: GET /tsdb/series
   - HandleLatest: GET /tsdb/series/{key}/latest
   - HandleStats: GET /tsdb/stats
   - HandleDeleteSeries: DELETE /tsdb/series/{key}
   - HandleSubscribe: WS /tsdb/subscribe (WebSocket 업그레이드)

3. **internal/api/dto/**: 요청/응답 DTO
   - TSDBWriteRequest, TSDBQueryRequest
   - TSDBQueryResponse, TSDBStatsResponse

4. **internal/api/router.go**: TSDB 라우트 등록

**테스트**: API 핸들러 테스트, WebSocket 구독 통합 테스트

---

## 2. 기술 접근

### 2.1 싱글톤 TSDB 인스턴스

xflowd 프로세스당 하나의 TSDB 인스턴스를 유지한다. cmd/xflowd/main.go에서 초기화하고, 엔진과 API 핸들러에 주입한다.

```
main.go -> tsdb.New(config) -> engine.SetTSDB(db) + handler.SetTSDB(db)
```

### 2.2 노드 어댑터 패턴

기존 `internal/node/adapter/` 패턴을 따른다. `mqtt.go`, `register.go` 와 동일한 구조로 TSDB 노드를 구현하여 일관성을 유지한다.

### 2.3 API 핸들러 패턴

기존 `internal/api/handler/flow.go`, `agent.go` 패턴을 따른다. 서비스 어댑터 레이어를 통해 TSDB 인스턴스에 접근한다.

### 2.4 WebSocket 패턴

기존 `internal/api/ws/` 패턴을 참고한다. EventPublisher, LogWriter 등의 기존 WebSocket 인프라를 활용하거나 확장한다.

---

## 3. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 높은 시리즈 카디널리티(>10,000)로 메모리 부족 | 높음 | 시리즈 수 제한 설정 + 경고 로그. 카디널리티 메트릭 노출 |
| 대량 쿼리(수백만 포인트)로 인한 응답 지연 | 중간 | 쿼리당 최대 반환 포인트 제한(10,000) + 타임아웃(10초) |
| 메모리 추정 오차로 실제 OOM 발생 | 높음 | 추정치에 1.2배 보정 계수 적용. runtime.MemStats 기반 실제 메모리 교차 검증 |
| out-of-order 데이터가 빈번한 경우 삽입 성능 저하 | 낮음 | sort.Search 기반 O(log n) 탐색으로 삽입 위치 결정 |
| 보존 정책 hot reload 시 데이터 정합성 | 중간 | 정책 변경 시 즉시 퇴거하지 않고 다음 퇴거 주기에 적용 |

---

## 4. 의존성

### 내부 의존성 (기존 xflow 패키지)

| 패키지 | 용도 |
|--------|------|
| pkg/message | DataPoint <-> Message 변환 |
| pkg/lifecycle | TSDB 생명주기 관리 (시작/중지) |
| internal/node | 노드 인터페이스, 레지스트리 |
| internal/node/adapter | 어댑터 패턴 |
| internal/api/handler | API 핸들러 패턴 |
| internal/api/ws | WebSocket 인프라 |
| internal/config | Viper 설정 통합 |
| internal/observe | 관찰성 (로깅, 메트릭) |

### 외부 의존성

추가 외부 의존성 없음. Go 표준 라이브러리 + 기존 xflow 의존성만 사용한다.

---

## 5. Expert Consultation 권장

| 도메인 | 에이전트 | 이유 |
|--------|---------|------|
| Backend | expert-backend | 동시성 전략 검증, 메모리 관리 패턴, 이진 탐색 삽입 최적화 |
| Frontend | expert-frontend | 대시보드 WebSocket 구독 UI, 시계열 차트 렌더링 (후속 SPEC) |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-19*
