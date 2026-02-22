---
id: SPEC-AGG-001
title: "Aggregate Node Multi-Field Stats Enhancement"
version: "1.0.0"
status: planned
created: "2026-02-22"
updated: "2026-02-22"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-22 | xtra | 초기 수용 기준 작성 |

# SPEC-AGG-001: 수용 기준 (Acceptance Criteria)

## 1. 다중 집계 함수 지원 (REQ-AGG-001, REQ-AGG-002, REQ-AGG-003)

### AC-001: 다중 집계 함수 리스트 설정

```gherkin
Feature: 다중 집계 함수 리스트 지원

  Scenario: aggregate_fn 리스트 설정 시 모든 함수 실행
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                       |
      | window_size  | 3                           |
      | aggregate_fn | ["avg", "min", "max"]       |
      | fields       | ["temperature"]             |
    When 다음 메시지 3건이 순서대로 Process된다:
      | temperature |
      | 20.0        |
      | 25.0        |
      | 30.0        |
    Then 출력 메시지의 stats.temperature는 다음을 포함한다:
      | avg  | 25.0 |
      | min  | 20.0 |
      | max  | 30.0 |

  Scenario: aggregate_fn 리스트에 count 함수 포함
    Given AggregateNode가 aggregate_fn: ["avg", "count"]로 구성된다
    And window_type: "count", window_size: 5, fields: ["humidity"]로 구성된다
    When humidity 값 [60.0, 70.0, 65.0, 80.0, 55.0]을 가진 메시지 5건이 Process된다
    Then 출력 메시지의 stats.humidity.avg는 66.0이다
    And 출력 메시지의 stats.humidity.count는 5이다
```

### AC-002: 단일 집계 함수 하위 호환

```gherkin
Feature: 단일 집계 함수 하위 호환

  Scenario: aggregate_fn 문자열 설정 시 기존 동작 유지
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 2             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
    When temperature 값 [10.0, 30.0]을 가진 메시지 2건이 Process된다
    Then 출력 메시지의 payload.result는 20.0이다
    And 출력 메시지의 payload.count는 2이다
    And 출력 메시지의 metadata._aggregate_fn은 "avg"이다
```

### AC-003: 유효하지 않은 집계 함수 거부

```gherkin
Feature: 유효하지 않은 집계 함수 거부

  Scenario: 지원하지 않는 함수명으로 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 aggregate_fn: ["avg", "median"]으로 호출된다
    Then ErrAggregateFnInvalid 에러가 반환된다
    And 에러 메시지에 "median"이 포함된다

  Scenario: 빈 함수 리스트로 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 aggregate_fn: []로 호출된다
    Then ErrAggregateFnInvalid 에러가 반환된다
    And 에러 메시지에 "must not be empty"가 포함된다
```

## 2. 다중 필드 지원 (REQ-AGG-010 ~ REQ-AGG-014)

### AC-010: 다중 필드 리스트 설정

```gherkin
Feature: 다중 필드 리스트 지원

  Scenario: fields 리스트 설정 시 각 필드 독립 집계
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 3                                  |
      | aggregate_fn | ["avg", "min", "max"]              |
      | fields       | ["temperature", "humidity"]        |
    When 다음 메시지 3건이 순서대로 Process된다:
      | temperature | humidity |
      | 20.0        | 60.0     |
      | 25.0        | 70.0     |
      | 30.0        | 50.0     |
    Then 출력 메시지의 stats.temperature는 다음을 포함한다:
      | avg | 25.0 |
      | min | 20.0 |
      | max | 30.0 |
    And 출력 메시지의 stats.humidity는 다음을 포함한다:
      | avg | 60.0 |
      | min | 50.0 |
      | max | 70.0 |
    And 출력 메시지의 window_count는 3이다
```

### AC-011: 단일 필드 하위 호환

```gherkin
Feature: 단일 field 키 하위 호환

  Scenario: field 문자열 설정 시 단일 요소 리스트로 처리
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 2             |
      | aggregate_fn | "sum"         |
      | field        | "temperature" |
    When temperature 값 [10.0, 20.0]을 가진 메시지 2건이 Process된다
    Then 출력 메시지의 payload.result는 30.0이다
    And 출력 메시지의 payload.count는 2이다
```

### AC-012: 필드 미설정 시 기본값

```gherkin
Feature: 필드 미설정 시 기본값 "value"

  Scenario: field와 fields 모두 미설정 시 "value" 키 사용
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count |
      | window_size  | 2     |
      | aggregate_fn | "sum" |
    When value 값 [10.0, 20.0]을 가진 메시지 2건이 Process된다
    Then 출력 메시지의 payload.result는 30.0이다
    And 이는 기존 동작과 동일하다
```

### AC-013: fields와 field 동시 설정 시 우선순위

```gherkin
Feature: fields 우선순위

  Scenario: fields와 field 동시 설정 시 fields 사용
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                       |
      | window_size  | 2                           |
      | aggregate_fn | "avg"                       |
      | field        | "temperature"               |
      | fields       | ["humidity", "pressure"]    |
    When Configure가 호출된다
    Then 내부 fields는 ["humidity", "pressure"]이다
    And "temperature"는 무시된다
```

### AC-014: 빈 필드 리스트 거부

```gherkin
Feature: 빈 필드 리스트 거부

  Scenario: fields가 빈 리스트일 때 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 fields: []로 호출된다
    Then ErrAggregateFieldInvalid 에러가 반환된다
    And 에러 메시지에 "must not be empty"가 포함된다
```

## 3. 구조화된 출력 형식 (REQ-AGG-020 ~ REQ-AGG-022)

### AC-020: 다중 모드 stats 출력 구조

```gherkin
Feature: 다중 필드/함수 stats 출력 구조

  Scenario: 2필드 x 4함수 매트릭스 출력 검증
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 4                                  |
      | aggregate_fn | ["avg", "min", "max", "count"]     |
      | fields       | ["temperature", "humidity"]        |
    When 다음 메시지 4건이 순서대로 Process된다:
      | temperature | humidity |
      | 20.0        | 40.0     |
      | 25.0        | 50.0     |
      | 30.0        | 60.0     |
      | 25.0        | 50.0     |
    Then 출력 메시지 페이로드는 다음 구조를 가진다:
      """json
      {
        "stats": {
          "temperature": {
            "avg": 25.0,
            "min": 20.0,
            "max": 30.0,
            "count": 4
          },
          "humidity": {
            "avg": 50.0,
            "min": 40.0,
            "max": 60.0,
            "count": 4
          }
        },
        "window_count": 4,
        "window_type": "count",
        "window_size": 4
      }
      """
```

### AC-021: 단일 모드 레거시 출력 호환

```gherkin
Feature: 단일 모드 레거시 출력 유지

  Scenario: 단일 필드 + 단일 함수일 때 result + count + stats 모두 포함
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 3             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
    When temperature 값 [10.0, 20.0, 30.0]을 가진 메시지 3건이 Process된다
    Then 출력 메시지 페이로드의 result는 20.0이다
    And 출력 메시지 페이로드의 count는 3이다
    And 출력 메시지 페이로드의 stats.temperature.avg는 20.0이다
    And 출력 메시지 페이로드의 window_count는 3이다
```

### AC-022: 메타데이터 확장

```gherkin
Feature: 출력 메타데이터 확장

  Scenario: 다중 함수/필드 메타데이터 포함
    Given AggregateNode가 aggregate_fn: ["avg", "min"], fields: ["temperature", "humidity"]로 구성된다
    When 윈도우 플러시가 발생한다
    Then 출력 메시지의 metadata._aggregate_fn은 "avg,min"이다
    And 출력 메시지의 metadata._window_type은 설정된 윈도우 타입이다
    And 출력 메시지의 metadata._fields는 "temperature,humidity"이다

  Scenario: 단일 함수/필드 메타데이터 하위 호환
    Given AggregateNode가 aggregate_fn: "avg", field: "temperature"로 구성된다
    When 윈도우 플러시가 발생한다
    Then 출력 메시지의 metadata._aggregate_fn은 "avg"이다
    And 출력 메시지의 metadata._fields는 "temperature"이다
```

## 4. 필드 값 추출 (REQ-AGG-040, REQ-AGG-041)

### AC-040: 필드별 독립 추출

```gherkin
Feature: 필드별 독립 값 추출

  Scenario: 각 필드에 대해 Payload.Get(fieldName) 호출
    Given AggregateNode가 fields: ["temperature", "humidity"]로 구성된다
    And aggregate_fn: ["avg"]로 구성된다
    And window_type: "count", window_size: 2로 구성된다
    When 다음 메시지 2건이 Process된다:
      | temperature | humidity |
      | 20.0        | 60.0     |
      | 30.0        | 80.0     |
    Then stats.temperature.avg는 25.0이다
    And stats.humidity.avg는 70.0이다
```

### AC-041: 필드 누락 메시지 처리

```gherkin
Feature: 필드 누락 메시지 skip 처리

  Scenario: 일부 메시지에 특정 필드가 없으면 해당 필드 집계에서 제외
    Given AggregateNode가 fields: ["temperature", "humidity"]로 구성된다
    And aggregate_fn: ["avg", "count"]로 구성된다
    And window_type: "count", window_size: 3으로 구성된다
    When 다음 메시지 3건이 Process된다:
      | temperature | humidity |
      | 20.0        | 60.0     |
      | 30.0        | (없음)   |
      | 25.0        | 80.0     |
    Then stats.temperature.avg는 25.0이다 (3건 모두 포함)
    And stats.temperature.count는 3이다
    And stats.humidity.avg는 70.0이다 (2건만 포함: 60+80/2)
    And stats.humidity.count는 2이다

  Scenario: 모든 메시지에 특정 필드가 없으면 해당 필드의 집계 값은 0 또는 기본값
    Given AggregateNode가 fields: ["temperature", "nonexistent"]로 구성된다
    And aggregate_fn: ["avg", "count"]로 구성된다
    And window_type: "count", window_size: 2로 구성된다
    When temperature 값 [20.0, 30.0]을 가진 메시지 2건이 Process된다
    Then stats.temperature.avg는 25.0이다
    And stats.nonexistent.count는 0이다
    And stats.nonexistent.avg는 0이다
```

## 5. 시간 기반 윈도우 (REQ-AGG-050, REQ-AGG-051)

### AC-050: 타이머 플러시의 다중 필드 지원

```gherkin
Feature: time 윈도우 다중 필드 stats 출력

  Scenario: 타이머 만료 시 다중 필드 stats 생성
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | time                               |
      | window_size  | "50ms"                             |
      | aggregate_fn | ["avg", "min", "max"]              |
      | fields       | ["temperature", "humidity"]        |
    And Init이 호출되어 타이머가 시작된다
    When 다음 메시지가 50ms 이내에 Process된다:
      | temperature | humidity |
      | 20.0        | 60.0     |
      | 30.0        | 80.0     |
    And 100ms를 대기한다 (타이머 만료)
    Then lastFlushResult에 다중 필드 stats가 저장되어 있다
    And stats.temperature.avg는 25.0이다
    And stats.humidity.avg는 70.0이다
    And window_type은 "time"이다
```

### AC-051: 빈 버퍼 안전 처리

```gherkin
Feature: 빈 버퍼 타이머 만료 시 안전 처리

  Scenario: 타이머 만료 시 버퍼가 비어있으면 플러시 안 함
    Given AggregateNode가 time 윈도우 "50ms"로 구성된다
    And Init이 호출되어 타이머가 시작된다
    When 메시지를 보내지 않고 100ms를 대기한다
    Then lastFlushResult는 nil이다
    And 에러가 발생하지 않는다
```

## 6. 동시성 안전 (REQ-AGG-060)

### AC-060: Race Condition 미발생

```gherkin
Feature: 다중 필드 모드 동시성 안전

  Scenario: 동시 Process 호출 시 데이터 레이스 없음
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 100                                |
      | aggregate_fn | ["avg", "min", "max"]              |
      | fields       | ["temperature", "humidity"]        |
    When 10개의 고루틴이 동시에 각각 10건의 메시지를 Process한다
    Then go test -race 플래그에서 데이터 레이스가 보고되지 않는다
    And 패닉(panic)이 발생하지 않는다
```

## 7. Configure 파싱 (REQ-AGG-030, REQ-AGG-031)

### AC-030: aggregate_fn 파싱

```gherkin
Feature: aggregate_fn 타입별 파싱

  Scenario: string 타입 -> 단일 요소 슬라이스 변환
    Given AggregateNode가 생성된다
    When Configure가 aggregate_fn: "avg"로 호출된다
    Then 내부 aggregateFns는 [AggregateAvg]이다

  Scenario: []any 타입 -> AggregateFn 슬라이스 변환
    Given AggregateNode가 생성된다
    When Configure가 aggregate_fn: ["sum", "avg", "min", "max", "count"]로 호출된다
    Then 내부 aggregateFns는 [AggregateSum, AggregateAvg, AggregateMin, AggregateMax, AggregateCount]이다
    And 에러는 반환되지 않는다

  Scenario: 중복 함수 허용
    Given AggregateNode가 생성된다
    When Configure가 aggregate_fn: ["avg", "avg"]로 호출된다
    Then 에러는 반환되지 않는다
    And 내부 aggregateFns 길이는 2이다
```

### AC-031: fields 파싱

```gherkin
Feature: fields 타입별 파싱

  Scenario: []any 타입 -> string 슬라이스 변환
    Given AggregateNode가 생성된다
    When Configure가 fields: ["temperature", "humidity", "pressure"]로 호출된다
    Then 내부 fields는 ["temperature", "humidity", "pressure"]이다

  Scenario: field 키만 있을 때 단일 요소 변환
    Given AggregateNode가 생성된다
    When Configure가 field: "temperature"로 호출된다
    Then 내부 fields는 ["temperature"]이다

  Scenario: fields와 field 동시 존재 시 fields 우선
    Given AggregateNode가 생성된다
    When Configure가 fields: ["humidity"], field: "temperature"로 호출된다
    Then 내부 fields는 ["humidity"]이다
```

## 8. 기존 테스트 회귀 검증

### AC-COMPAT: 기존 테스트 전체 PASS

```gherkin
Feature: 하위 호환성 보장

  Scenario: 기존 테스트 스위트 전체 통과
    Given 기존 aggregate_test.go의 모든 테스트가 존재한다
    When go test -race ./internal/node/... 을 실행한다
    Then 다음 기존 테스트가 모두 PASS한다:
      | TestNewAggregateNode_정상생성               |
      | TestAggregateNode_Init_상태전이              |
      | TestAggregateNode_Configure_카운트윈도우      |
      | TestAggregateNode_Configure_타임윈도우        |
      | TestAggregateNode_Configure_nil에러          |
      | TestAggregateNode_Process_카운트윈도우_Sum    |
      | TestAggregateNode_Process_카운트윈도우_Avg    |
      | TestAggregateNode_Process_카운트윈도우_Count  |
      | TestAggregateNode_Process_카운트윈도우_Min    |
      | TestAggregateNode_Process_카운트윈도우_Max    |
      | TestAggregateNode_Process_카운트윈도우_First  |
      | TestAggregateNode_Process_카운트윈도우_Last   |
      | TestAggregateNode_Process_비숫자값_스킵       |
      | TestAggregateNode_Process_정수값_숫자변환     |
      | TestAggregateNode_Process_타임윈도우_자동플러시 |
      | TestAggregateNode_Shutdown_상태전이           |
      | TestAggregateNode_Shutdown_잔여버퍼플러시      |
      | TestAggregateNode_Configure_유효하지않은윈도우사이즈 |
      | TestAggregateNode_Configure_유효하지않은윈도우타입  |
      | TestAggregateNode_동시성안전_Process           |
      | TestAggregateNode_Process_집계함수테이블       |
    And 데이터 레이스가 보고되지 않는다
```

## 9. 품질 게이트 (Quality Gates)

### Definition of Done

- [ ] 모든 기존 테스트 PASS (하위 호환성 확인)
- [ ] 신규 테스트 PASS (다중 필드, 다중 함수, Configure 파싱, 출력 구조)
- [ ] `go test -race ./internal/node/...` 데이터 레이스 0건
- [ ] `go vet ./...` 경고 0건
- [ ] 테스트 커버리지 85% 이상 (`internal/node/aggregate.go` 기준)
- [ ] `examples/flows/mqtt-metrics.yaml` 다중 필드 설정 예시 포함
- [ ] `stats` 출력 구조가 명세(spec.md 5.4)와 일치
- [ ] 단일 모드에서 `result` + `count` 레거시 출력 유지 확인
- [ ] 메타데이터에 `_aggregate_fn`, `_window_type`, `_fields` 포함 확인

### 검증 방법

| 항목 | 도구 | 명령어 |
|------|------|--------|
| 단위 테스트 | Go testing | `go test -v ./internal/node/...` |
| 레이스 검출 | Go race detector | `go test -race ./internal/node/...` |
| 커버리지 | Go cover | `go test -cover -coverprofile=coverage.out ./internal/node/...` |
| 정적 분석 | Go vet | `go vet ./...` |
| YAML 로드 | 통합 테스트 | 플로우 설정 로드 및 Configure 호출 검증 |
