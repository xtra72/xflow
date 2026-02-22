---
id: SPEC-AGG-001
title: "Aggregate Node Multi-Field Stats Enhancement"
version: "1.0.0"
status: planned
created: "2026-02-22"
updated: "2026-02-22"
author: "xtra"
priority: high
related_specs:
  - SPEC-INFLUX-001
tags:
  - aggregate
  - statistics
  - mqtt
  - iot
  - time-window
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-22 | xtra | 초기 SPEC 작성 |

# SPEC-AGG-001: Aggregate 노드 다중 필드 통계 기능 확장

## 1. 개요

### 1.1 목적

기존 xflow Aggregate 노드를 확장하여, MQTT 센서 메시지의 **여러 필드**에 대해 **여러 집계 함수**를 동시에 실행하고, 정해진 시간(또는 개수) 구간에서의 최소, 최대, 평균 등의 통계 정보를 구조화된 형태로 생성하는 기능을 구현한다.

### 1.2 배경

현재 Aggregate 노드는 단일 집계 함수(`aggregate_fn: "avg"`)와 단일 필드(`field: "temperature"` 또는 하드코딩된 `"value"` 키)만 지원한다. IoT 센서 데이터 파이프라인에서는 temperature, humidity 등 여러 필드에 대해 avg, min, max, count 등 다양한 통계를 한 번에 산출해야 하는 요구가 빈번하다. 현재 구조에서는 필드별/함수별로 별도 Aggregate 노드를 배치해야 하므로 플로우가 복잡해지고 자원이 낭비된다.

### 1.3 범위

- **포함**: Aggregate 노드의 Configure, Process, executeAggregate, extractNumericValues 메서드 확장
- **포함**: 다중 `aggregate_fn` 리스트 지원 및 단일 문자열 하위 호환
- **포함**: 다중 `fields` 리스트 지원 및 단일 `field` 하위 호환
- **포함**: 구조화된 `stats` 출력 형식 (필드별 x 함수별 매트릭스)
- **포함**: 기존 단일 필드/단일 함수 출력(`result` + `count`) 하위 호환
- **포함**: 예제 플로우 YAML 업데이트
- **제외**: 새로운 노드 타입 생성 (기존 aggregate 타입 확장)
- **제외**: 슬라이딩 윈도우 (tumbling window만 지원)
- **제외**: 집계 결과의 외부 저장소 연동 (SPEC-INFLUX-001 범위)

## 2. 환경 (Environment)

### 2.1 시스템 환경

- **언어**: Go 1.25+
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`
- **대상 파일**: `internal/node/aggregate.go`, `internal/node/aggregate_test.go`
- **관련 파일**: `examples/flows/mqtt-metrics.yaml`, `pkg/message/message.go`

### 2.2 기존 아키텍처

- `AggregateNode` 구조체가 `BaseNode`를 임베딩
- `sync.Mutex`로 동시성 보호
- `[]message.Message` 버퍼에 메시지를 축적
- count 윈도우: `len(buffer) >= windowSize`일 때 플러시
- time 윈도우: `time.AfterFunc`로 주기적 플러시
- `extractNumericValues`가 하드코딩된 `"value"` 키에서 숫자 추출

### 2.3 의존성

- `pkg/message`: Payload의 `Get(key)`, `Set(key, value)` 인터페이스
- `pkg/flow`: `NodeDef` 및 플로우 직렬화
- `pkg/lifecycle`: 노드 라이프사이클 상태 전이

## 3. 가정 (Assumptions)

- **A1**: MQTT 센서 메시지의 페이로드는 JSON 파싱이 완료된 `map[string]any` 형태로 Aggregate 노드에 도달한다 (upstream의 transform 노드가 필드 추출을 수행).
- **A2**: 집계 대상 필드의 값은 숫자 타입(`float64`, `int` 등)이다. 비숫자 값은 기존 동작과 동일하게 무시(skip)한다.
- **A3**: `fields` 리스트의 각 필드는 페이로드 최상위 키에 해당한다 (중첩 경로 미지원).
- **A4**: 단일 `aggregate_fn`(문자열) 설정과 리스트 설정이 혼재할 수 없다 - 문자열이면 단일 함수, 리스트이면 다중 함수.
- **A5**: `first`와 `last` 집계 함수는 다중 필드 모드에서도 개별 필드에 대해 독립적으로 동작한다.
- **A6**: 기존 테스트 코드는 하드코딩된 필드(`"value"`)를 사용하므로, `field`/`fields` 미설정 시 기본값 `"value"`를 유지하여 하위 호환성을 보장한다.

## 4. 요구사항 (Requirements)

### 4.1 다중 집계 함수 지원

**REQ-AGG-001**: 다중 집계 함수 리스트 지원
- **WHEN** `aggregate_fn` 설정 값이 문자열 리스트(`[]string`)로 제공되면, **THEN** 시스템은 지정된 모든 집계 함수를 버퍼의 메시지에 대해 동시에 실행해야 한다.
- 지원 함수: `sum`, `avg`, `min`, `max`, `count`, `first`, `last`

**REQ-AGG-002**: 단일 집계 함수 하위 호환
- **WHEN** `aggregate_fn` 설정 값이 단일 문자열(예: `"avg"`)로 제공되면, **THEN** 시스템은 기존과 동일하게 단일 집계 함수만 실행해야 한다.

**REQ-AGG-003**: 유효하지 않은 집계 함수 거부
- **IF** `aggregate_fn` 리스트에 지원되지 않는 함수명이 포함되면, **THEN** 시스템은 `Configure` 단계에서 에러를 반환해야 한다.

### 4.2 다중 필드 지원

**REQ-AGG-010**: 다중 필드 리스트 지원
- **WHEN** `fields` 설정 값이 문자열 리스트(`[]string`)로 제공되면, **THEN** 시스템은 각 필드에 대해 독립적으로 숫자 값을 추출하고 집계 함수를 적용해야 한다.

**REQ-AGG-011**: 단일 필드 하위 호환 (`field` 키)
- **WHEN** `field` 설정 값이 단일 문자열로 제공되면, **THEN** 시스템은 해당 값을 단일 요소 리스트로 변환하여 처리해야 한다.

**REQ-AGG-012**: 필드 미설정 시 기본값
- **IF** `field`와 `fields` 모두 설정되지 않은 경우, **THEN** 시스템은 기본값 `"value"`를 사용하여 기존 동작과 호환성을 유지해야 한다.

**REQ-AGG-013**: `fields`와 `field` 동시 설정 시 우선순위
- **WHEN** `fields`와 `field`가 동시에 설정되면, **THEN** 시스템은 `fields` 설정을 우선 적용해야 한다.

**REQ-AGG-014**: 빈 필드 리스트 거부
- **IF** `fields`가 빈 리스트(`[]`)로 제공되면, **THEN** 시스템은 `Configure` 단계에서 에러를 반환해야 한다.

### 4.3 구조화된 출력 형식

**REQ-AGG-020**: 다중 필드/다중 함수 출력 구조
- **WHEN** 다중 필드와 다중 집계 함수가 설정된 상태에서 윈도우 플러시가 발생하면, **THEN** 출력 메시지 페이로드는 다음 구조를 포함해야 한다:
  - `stats`: 필드명을 키, 함수명-결과값 맵을 값으로 하는 중첩 맵 (`map[string]map[string]any`)
  - `window_count`: 윈도우 내 메시지 수 (`int`)
  - `window_type`: 윈도우 타입 (`string`: `"count"` 또는 `"time"`)
  - `window_size`: 윈도우 크기 설정값 (count: `int`, time: `string` 예: `"30s"`)

**REQ-AGG-021**: 단일 필드/단일 함수 레거시 출력 유지
- **WHEN** 단일 필드 + 단일 집계 함수 설정으로 실행될 때, **THEN** 출력 메시지 페이로드는 기존 형식(`result` + `count`)을 유지해야 한다.
- 시스템은 기존 `result` + `count` 형식과 함께 `stats` 형식도 동시에 제공해야 한다.

**REQ-AGG-022**: 메타데이터 확장
- 시스템은 **항상** 출력 메시지 메타데이터에 다음을 포함해야 한다:
  - `_aggregate_fn`: 사용된 집계 함수 (단일이면 문자열, 다중이면 쉼표 구분 문자열)
  - `_window_type`: 윈도우 타입
  - `_fields`: 집계 대상 필드 (쉼표 구분 문자열)

### 4.4 Configure 메서드 확장

**REQ-AGG-030**: `aggregate_fn` 타입 감지 및 파싱
- **WHEN** `aggregate_fn` 설정이 `[]any` 타입(YAML 리스트)으로 전달되면, **THEN** 시스템은 각 요소를 `string`으로 변환하여 `[]AggregateFn` 슬라이스에 저장해야 한다.
- **WHEN** `aggregate_fn` 설정이 `string` 타입으로 전달되면, **THEN** 시스템은 단일 요소 슬라이스 `[]AggregateFn{fn}`으로 변환해야 한다.

**REQ-AGG-031**: `fields` 타입 감지 및 파싱
- **WHEN** `fields` 설정이 `[]any` 타입으로 전달되면, **THEN** 시스템은 각 요소를 `string`으로 변환하여 `[]string` 슬라이스에 저장해야 한다.
- **WHEN** `field` 설정이 `string` 타입으로 전달되면, **THEN** 시스템은 단일 요소 슬라이스 `[]string{field}`으로 변환해야 한다.

### 4.5 필드 값 추출 방식 개선

**REQ-AGG-040**: 필드별 독립 추출
- 시스템은 **항상** 각 필드에 대해 `msg.Payload().Get(fieldName)`을 호출하여 값을 독립적으로 추출해야 한다.
- 기존 `extractNumericValues`는 하드코딩된 `"value"` 키 대신, 지정된 필드명으로 값을 추출하도록 변경해야 한다.

**REQ-AGG-041**: 필드 누락 메시지 처리
- **IF** 특정 메시지에 지정된 필드가 존재하지 않으면, **THEN** 해당 메시지는 그 필드의 집계에서 제외(skip)되어야 한다.
- 다른 필드의 집계에는 영향을 주지 않아야 한다.

### 4.6 시간 기반 윈도우 신뢰성

**REQ-AGG-050**: 타이머 플러시 결과의 다중 필드 지원
- **WHEN** time 윈도우의 타이머가 만료되어 버퍼를 플러시할 때, **THEN** 다중 필드/다중 함수 결과가 `lastFlushResult`에 올바르게 저장되어야 한다.

**REQ-AGG-051**: 빈 버퍼 안전 처리
- **IF** 타이머 만료 시 버퍼가 비어있으면, **THEN** 시스템은 플러시를 수행하지 않아야 한다 (기존 동작 유지).

### 4.7 동시성 안전

**REQ-AGG-060**: 스레드 안전성 유지
- 시스템은 **항상** 다중 필드/다중 함수 확장 후에도 `sync.Mutex` 기반 동시성 보호를 유지해야 한다.
- 시스템은 `go test -race`에서 데이터 레이스가 발생**하지 않아야 한다**.

## 5. 명세 (Specifications)

### 5.1 구조체 변경

`AggregateNode` 구조체 필드 변경:

| 기존 필드 | 변경 후 | 설명 |
|-----------|---------|------|
| `aggregateFn AggregateFn` | `aggregateFns []AggregateFn` | 복수 함수 지원으로 슬라이스 변경 |
| (신규) | `fields []string` | 집계 대상 필드명 리스트 |
| (신규) | `windowSizeRaw any` | Configure 시 원본 window_size 값 보존 (출력용) |

### 5.2 Configure 로직

```
Configure 호출 시:
1. window_type, window_size 파싱 (기존 동작 유지)
2. aggregate_fn 파싱:
   - string -> []AggregateFn{fn}
   - []any -> 각 요소를 AggregateFn으로 변환, 유효성 검증
3. fields / field 파싱:
   - fields ([]any) -> []string (우선)
   - field (string) -> []string{field}
   - 미설정 -> []string{"value"} (기본값)
4. 빈 리스트 검증 (aggregate_fn, fields)
```

### 5.3 executeAggregate 로직 변경

```
executeAggregate(buf) 호출 시:
1. 다중 모드 판별: len(aggregateFns) > 1 || len(fields) > 1
2. 다중 모드:
   a. stats 맵 초기화: map[string]map[string]any
   b. 각 field에 대해:
      - extractNumericValuesForField(buf, field) -> []float64
      - 각 aggregateFn에 대해 결과 계산
      - stats[field][fn] = 결과값
   c. 출력 메시지에 stats, window_count, window_type, window_size 설정
3. 단일 모드 (하위 호환):
   - 기존 로직 유지 (result + count 출력)
   - 추가로 stats 형식도 포함
```

### 5.4 출력 메시지 형식

**다중 모드 출력 예시** (fields: ["temperature", "humidity"], aggregate_fn: ["avg", "min", "max", "count"]):

```json
{
  "stats": {
    "temperature": {
      "avg": 25.3,
      "min": 20.1,
      "max": 30.5,
      "count": 100
    },
    "humidity": {
      "avg": 65.2,
      "min": 45.0,
      "max": 85.3,
      "count": 100
    }
  },
  "window_count": 100,
  "window_type": "time",
  "window_size": "30s"
}
```

**단일 모드 출력 예시** (field: "temperature", aggregate_fn: "avg"):

```json
{
  "result": 25.3,
  "count": 100,
  "stats": {
    "temperature": {
      "avg": 25.3
    }
  },
  "window_count": 100,
  "window_type": "count",
  "window_size": 10
}
```

### 5.5 YAML 설정 예시

**확장된 다중 필드/다중 함수 설정**:

```yaml
- name: "stats-generator"
  type: "aggregate"
  config:
    window_type: "time"
    window_size: "30s"
    aggregate_fn:
      - "avg"
      - "min"
      - "max"
      - "count"
    fields:
      - "temperature"
      - "humidity"
```

**하위 호환 단일 설정** (기존 형식 그대로 동작):

```yaml
- name: "metrics-aggregator"
  type: "aggregate"
  config:
    window_type: "count"
    window_size: 10
    aggregate_fn: "avg"
    field: "temperature"
```

### 5.6 에러 정의

| 에러 | 조건 | 메시지 |
|------|------|--------|
| `ErrAggregateFieldInvalid` | fields가 빈 리스트 | `"invalid aggregate field: fields list must not be empty"` |
| `ErrAggregateFnInvalid` | 지원하지 않는 함수명 | `"invalid aggregate function: unknown function %q"` |
| `ErrAggregateFnInvalid` | aggregate_fn이 빈 리스트 | `"invalid aggregate function: function list must not be empty"` |

## 6. 추적성 (Traceability)

| 요구사항 ID | 구현 파일 | 테스트 |
|-------------|-----------|--------|
| REQ-AGG-001 | `internal/node/aggregate.go` | `aggregate_test.go` - 다중 함수 테스트 |
| REQ-AGG-002 | `internal/node/aggregate.go` | `aggregate_test.go` - 기존 단일 함수 테스트 (회귀) |
| REQ-AGG-010 | `internal/node/aggregate.go` | `aggregate_test.go` - 다중 필드 테스트 |
| REQ-AGG-011 | `internal/node/aggregate.go` | `aggregate_test.go` - 단일 field 하위 호환 |
| REQ-AGG-012 | `internal/node/aggregate.go` | `aggregate_test.go` - 기본값 "value" 테스트 |
| REQ-AGG-020 | `internal/node/aggregate.go` | `aggregate_test.go` - stats 출력 구조 검증 |
| REQ-AGG-021 | `internal/node/aggregate.go` | `aggregate_test.go` - 레거시 출력 호환 |
| REQ-AGG-030 | `internal/node/aggregate.go` | `aggregate_test.go` - Configure 파싱 |
| REQ-AGG-040 | `internal/node/aggregate.go` | `aggregate_test.go` - 필드별 추출 |
| REQ-AGG-041 | `internal/node/aggregate.go` | `aggregate_test.go` - 누락 필드 skip |
| REQ-AGG-050 | `internal/node/aggregate.go` | `aggregate_test.go` - time 윈도우 다중 필드 |
| REQ-AGG-060 | `internal/node/aggregate.go` | `aggregate_test.go` - race 테스트 |
