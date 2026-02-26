---
id: SPEC-AGG-001
title: "Aggregate Node Multi-Field Stats Enhancement"
version: "1.1.0"
status: completed
created: "2026-02-22"
updated: "2026-02-27"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-22 | xtra | 초기 구현 계획 작성 |
| 1.1.0 | 2026-02-27 | xtra | 구현 완료 (status: completed), 모든 마일스톤 달성 |

# SPEC-AGG-001: 구현 계획

## 1. 구현 전략

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 기준):
- **기존 코드 수정** (DDD): `aggregate.go`의 기존 메서드 변경 시 ANALYZE-PRESERVE-IMPROVE 사이클
- **신규 코드 추가** (TDD): 새로운 헬퍼 함수, 에러 정의, 테스트 케이스는 RED-GREEN-REFACTOR 사이클

### 1.2 변경 영향 범위

| 파일 | 변경 유형 | 영향도 |
|------|-----------|--------|
| `internal/node/aggregate.go` | 핵심 수정 | 높음 - 구조체, Configure, executeAggregate 변경 |
| `internal/node/aggregate_test.go` | 테스트 확장 | 높음 - 기존 테스트 회귀 확인 + 신규 테스트 추가 |
| `internal/node/errors.go` | 에러 추가 | 낮음 - 새로운 에러 상수 추가 |
| `examples/flows/mqtt-metrics.yaml` | 예제 업데이트 | 낮음 - 다중 필드/함수 설정 예시 추가 |

## 2. 마일스톤

### 마일스톤 1: 기초 작업 (Primary Goal)

기존 동작 보존 확인 및 구조체 확장

**작업 목록:**

- **T1.1**: 기존 테스트 전체 실행 (`go test -race ./internal/node/...`) 및 베이스라인 확보
  - 모든 기존 테스트가 PASS 상태인지 확인
  - 커버리지 베이스라인 기록

- **T1.2**: `AggregateNode` 구조체 필드 확장
  - `aggregateFn AggregateFn` -> `aggregateFns []AggregateFn` 변경
  - `fields []string` 필드 추가
  - `windowSizeRaw any` 필드 추가

- **T1.3**: 에러 정의 추가 (`internal/node/errors.go`)
  - `ErrAggregateFieldInvalid` 에러 변수 추가
  - `ErrAggregateFnInvalid` 에러 변수 추가 (기존 에러와 구분)

- **T1.4**: `aggregateFn` -> `aggregateFns` 내부 참조 일괄 변경
  - `executeAggregate` 내 `n.aggregateFn` -> `n.aggregateFns[0]` 변환 (임시 호환)
  - 기존 테스트 모두 PASS 확인 (회귀 방지)

**완료 기준:** 기존 테스트 전체 PASS + race 검증 통과

### 마일스톤 2: Configure 확장 (Primary Goal)

다중 함수/필드 파싱 로직 구현

**작업 목록:**

- **T2.1**: `Configure` 메서드의 `aggregate_fn` 파싱 확장
  - `string` 타입 -> `[]AggregateFn{fn}` 변환 (하위 호환)
  - `[]any` 타입 -> 각 요소 `string`으로 변환 후 `[]AggregateFn` 생성
  - 유효하지 않은 함수명 검증 및 에러 반환
  - 빈 리스트 검증 및 에러 반환
  - REQ-AGG-001, REQ-AGG-002, REQ-AGG-003 충족

- **T2.2**: `Configure` 메서드의 `fields`/`field` 파싱 구현
  - `fields` 키: `[]any` -> `[]string` 변환 (우선)
  - `field` 키: `string` -> `[]string{field}` 변환
  - 미설정 시 기본값 `[]string{"value"}` 적용
  - `fields` + `field` 동시 설정 시 `fields` 우선
  - 빈 리스트 검증 및 에러 반환
  - REQ-AGG-010, REQ-AGG-011, REQ-AGG-012, REQ-AGG-013, REQ-AGG-014 충족

- **T2.3**: Configure 확장 테스트 작성
  - 다중 함수 리스트 파싱 테스트
  - 단일 함수 문자열 파싱 하위 호환 테스트
  - 다중 필드 리스트 파싱 테스트
  - 단일 필드 문자열 파싱 하위 호환 테스트
  - 기본값 "value" 적용 테스트
  - 유효하지 않은 함수/빈 리스트 에러 테스트

**완료 기준:** Configure 관련 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 3: 집계 로직 확장 (Primary Goal)

다중 필드/함수 집계 실행 및 출력 구조

**작업 목록:**

- **T3.1**: `extractNumericValues` -> `extractNumericValuesForField` 리팩토링
  - 하드코딩된 `"value"` 키 대신 매개변수로 필드명 수신
  - `msg.Payload().Get(fieldName)` 호출로 변경
  - 기존 `extractNumericValues`는 호환성을 위해 `extractNumericValuesForField(buf, n.fields[0])` 래퍼로 유지 또는 제거
  - REQ-AGG-040, REQ-AGG-041 충족

- **T3.2**: `executeAggregate` 메서드 확장
  - 다중 모드 판별 로직: `len(aggregateFns) > 1 || len(fields) > 1`
  - 다중 모드: 필드별 x 함수별 매트릭스 계산
    - `stats` 맵 구성: `map[string]map[string]any`
    - `window_count`, `window_type`, `window_size` 출력 포함
  - 단일 모드: 기존 `result` + `count` 출력 유지 + `stats` 추가 포함
  - REQ-AGG-020, REQ-AGG-021, REQ-AGG-022 충족

- **T3.3**: 헬퍼 함수 - 개별 집계 함수 실행기
  - `computeAggregate(fn AggregateFn, values []float64, buf []message.Message) any`
  - `sum`, `avg`, `min`, `max`, `count` 공통 로직 추출
  - `first`, `last`는 개별 필드 값 기준으로 동작하도록 조정

- **T3.4**: 집계 로직 확장 테스트 작성
  - 다중 필드 + 다중 함수 stats 출력 구조 검증
  - 단일 필드 + 단일 함수 레거시 출력 호환 검증
  - 단일 필드 + 다중 함수 출력 검증
  - 다중 필드 + 단일 함수 출력 검증
  - 필드 누락 메시지 skip 동작 검증
  - 메타데이터 (_aggregate_fn, _window_type, _fields) 검증

**완료 기준:** 모든 집계 시나리오 테스트 PASS + stats 출력 구조 검증

### 마일스톤 4: 시간 윈도우 및 동시성 (Secondary Goal)

time 윈도우 다중 필드 지원 및 동시성 검증

**작업 목록:**

- **T4.1**: time 윈도우 타이머 플러시의 다중 필드 지원 검증
  - 타이머 만료 시 `executeAggregate` 호출이 다중 모드로 동작하는지 확인
  - `lastFlushResult`에 다중 필드 결과가 올바르게 저장되는지 검증
  - REQ-AGG-050, REQ-AGG-051 충족

- **T4.2**: 동시성 안전 테스트 강화
  - 다중 필드 모드에서의 `go test -race` 검증
  - 동시 Process 호출 시 stats 결과의 정합성 확인
  - REQ-AGG-060 충족

- **T4.3**: Shutdown 시 잔여 버퍼 플러시 검증
  - 다중 필드 모드에서 Shutdown 시 올바른 stats 출력 확인

**완료 기준:** time 윈도우 + race 테스트 PASS

### 마일스톤 5: 예제 및 문서화 (Final Goal)

예제 플로우 업데이트 및 최종 검증

**작업 목록:**

- **T5.1**: `examples/flows/mqtt-metrics.yaml` 업데이트
  - 기존 단일 함수/필드 설정을 다중 함수/필드 설정으로 확장
  - 주석에 출력 형식 예시 업데이트

- **T5.2**: 전체 테스트 스위트 실행
  - `go test -race -cover ./internal/node/...`
  - 커버리지 85% 이상 확인
  - `go vet ./...` 통과 확인

- **T5.3**: 통합 시나리오 확인
  - mqtt-metrics.yaml 플로우가 다중 필드 설정으로 올바르게 로드되는지 확인
  - Configure -> Init -> Process -> Shutdown 전체 라이프사이클 테스트

**완료 기준:** 전체 테스트 PASS + 커버리지 85%+ + 예제 업데이트

## 3. 기술 접근 방식

### 3.1 Configure 파싱 전략

YAML에서 파싱된 `map[string]any` 설정값의 타입 분기:

```
aggregate_fn 파싱:
  config["aggregate_fn"] 타입 검사:
    case string:  -> []AggregateFn{AggregateFn(s)}
    case []any:   -> 각 요소를 string 단언 -> AggregateFn 변환 -> 유효성 검증
    default:      -> 에러 반환

fields 파싱 (우선순위: fields > field > 기본값):
  if config["fields"] 존재:
    case []any:   -> 각 요소를 string 단언 -> []string
    default:      -> 에러 반환
  elif config["field"] 존재:
    case string:  -> []string{s}
    default:      -> 에러 반환
  else:
    -> []string{"value"}  (기본값)
```

### 3.2 다중 모드 판별

다중 모드와 단일 모드를 판별하는 기준:

```
isMultiMode := len(n.aggregateFns) > 1 || len(n.fields) > 1
```

이 판별은 `executeAggregate` 내부에서 출력 형식을 결정하는 데 사용된다. 단일 모드에서도 `stats` 필드는 포함하되, 기존 `result` + `count` 필드도 함께 유지하여 하위 호환성을 보장한다.

### 3.3 필드 추출 리팩토링

기존 `extractNumericValues`는 하드코딩된 `"value"` 키를 사용한다:

```
기존: msg.Payload().Get("value")
변경: msg.Payload().Get(fieldName)  // 매개변수화
```

이를 통해 동일한 메시지 버퍼에서 여러 필드에 대해 독립적으로 숫자 값을 추출할 수 있다.

### 3.4 집계 함수 실행 분리

현재 `executeAggregate` 내부의 switch 문에서 직접 계산하는 로직을 별도 함수로 분리:

```
computeAggregate(fn, values) -> any:
  switch fn:
    case sum:   -> 합계
    case avg:   -> 평균
    case min:   -> 최솟값
    case max:   -> 최댓값
    case count: -> len(values)  // 또는 len(buf) 기준
```

`first`와 `last`는 숫자 값이 아닌 원본 메시지 페이로드를 참조하므로, 다중 필드 모드에서는 각 필드의 첫 번째/마지막 값을 개별적으로 추출한다.

## 4. 아키텍처 설계 방향

### 4.1 구조체 설계

```
AggregateNode 구조체 변경:
  - aggregateFns []AggregateFn     (기존 aggregateFn -> 복수형)
  - fields       []string          (신규: 집계 대상 필드 리스트)
  - windowSizeRaw any              (신규: 원본 window_size 보존)
  - (나머지 기존 필드 유지)
```

### 4.2 메서드 시그니처 변경

| 메서드 | 변경 사항 |
|--------|-----------|
| `Configure` | `aggregate_fn` 리스트/문자열 파싱, `fields`/`field` 파싱 추가 |
| `executeAggregate` | 다중 모드 분기 추가, stats 맵 구성 |
| `extractNumericValues` | -> `extractNumericValuesForField(buf, fieldName)` 시그니처 변경 |
| (신규) `computeAggregate` | 개별 집계 함수 실행 헬퍼 |
| (신규) `isMultiMode` | 다중 모드 판별 헬퍼 |

### 4.3 출력 메시지 구조

다중 모드에서 `message.New()` 생성 후:

```
payload.Set("stats", statsMap)           // map[string]map[string]any
payload.Set("window_count", len(buf))    // int
payload.Set("window_type", windowType)   // string
payload.Set("window_size", windowSizeRaw) // any (int 또는 string)
```

단일 모드에서 추가:

```
payload.Set("result", singleResult)      // float64 또는 int
payload.Set("count", len(buf))           // int (기존 호환)
```

## 5. 리스크 및 대응 방안

| 리스크 | 가능성 | 영향도 | 대응 방안 |
|--------|--------|--------|-----------|
| 기존 테스트 회귀 | 중간 | 높음 | `aggregateFn` -> `aggregateFns` 변경 시 기존 참조 일괄 수정 + 회귀 테스트 우선 실행 |
| YAML 파싱 타입 불일치 | 중간 | 중간 | `[]any` vs `[]string` 타입 단언 실패 시 명확한 에러 메시지 반환 |
| time 윈도우 타이머 경쟁 조건 | 낮음 | 높음 | 기존 `sync.Mutex` 패턴 유지, `-race` 플래그로 지속 검증 |
| `first`/`last` 다중 필드 동작 모호성 | 중간 | 낮음 | 각 필드별 첫 번째/마지막 숫자 값을 독립 추출하는 것으로 명확히 정의 |
| 메모리 증가 (다중 필드 stats 맵) | 낮음 | 낮음 | 필드 수와 함수 수의 곱 만큼만 증가하므로 IoT 시나리오에서 문제 없음 |

## 6. 다음 단계

구현 완료 후:
- `/moai:2-run SPEC-AGG-001` 명령으로 DDD 구현 시작
- 관련 SPEC-INFLUX-001과의 통합 시 다중 필드 stats 출력을 InfluxDB write point로 변환하는 연동 검토
- `/moai:3-sync SPEC-AGG-001` 명령으로 문서 동기화

## 7. 구현 완료 요약

### 7.1 마일스톤 달성 현황

| 마일스톤 | 상태 | 비고 |
|----------|------|------|
| M1: 기초 작업 | 완료 | 구조체 확장, 에러 정의, 기존 테스트 회귀 통과 |
| M2: Configure 확장 | 완료 | 다중 함수/필드 파싱, 유효성 검증 구현 |
| M3: 집계 로직 확장 | 완료 | 다중 모드 stats 출력, 레거시 호환 유지 |
| M4: 시간 윈도우 및 동시성 | 완료 | time 윈도우 다중 필드 지원, race 검증 통과 |
| M5: 예제 및 문서화 | 완료 | 예제 업데이트, 커버리지 85%+ 달성 |

### 7.2 추가 구현 사항 (SPEC 범위 확장)

계획된 마일스톤 외에 다음 기능이 추가로 구현되었다:

- **AggregateNode.Info()** 메서드: 런타임 노드 상태 정보 조회 (윈도우 설정, 버퍼 크기, 부분 통계)
- **Engine infoProvider 인터페이스**: 노드별 추가 정보를 `NodeInstanceInfo.Extra` 필드로 노출
- **API/CLI Extra 필드**: REST API 및 CLI에서 노드 상세 정보 출력 지원

### 7.3 품질 지표

- `internal/node` 패키지 커버리지: 92.0%
- `internal/engine` 패키지 커버리지: 82.2%
- `go test -race`: 데이터 레이스 0건
- 수정 파일 수: 7개
