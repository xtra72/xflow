---
id: SPEC-AGG-002
title: "Aggregate Node Group-By Partitioning"
version: "1.1.0"
status: completed
created: "2026-02-23"
updated: "2026-02-23"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-23 | xtra | 초기 수용 기준 작성 |
| 1.1.0 | 2026-02-23 | xtra | 구현 완료 (status: completed) |

# SPEC-AGG-002: 수용 기준 (Acceptance Criteria)

## 1. group_by 설정 지원 (REQ-AGG-100, REQ-AGG-101, REQ-AGG-102)

### AC-100: group_by 설정 파싱

```gherkin
Feature: group_by 설정 파싱

  Scenario: group_by 문자열 설정 시 그룹 분류 키 저장
    Given AggregateNode가 생성된다
    When Configure가 다음 설정으로 호출된다:
      | window_type  | count       |
      | window_size  | 5           |
      | aggregate_fn | "avg"       |
      | group_by     | "location"  |
    Then 내부 groupBy는 "location"이다
    And 내부 groupBuffers 맵이 초기화되어 있다
    And 에러는 반환되지 않는다

  Scenario: group_by와 다중 필드/함수 함께 설정
    Given AggregateNode가 생성된다
    When Configure가 다음 설정으로 호출된다:
      | window_type  | time                         |
      | window_size  | "30s"                        |
      | aggregate_fn | ["avg", "min", "max"]        |
      | fields       | ["temperature", "humidity"]  |
      | group_by     | "location"                   |
    Then 내부 groupBy는 "location"이다
    And 내부 aggregateFns 길이는 3이다
    And 내부 fields 길이는 2이다
    And 에러는 반환되지 않는다
```

### AC-101: group_by 미설정 시 하위 호환

```gherkin
Feature: group_by 미설정 시 하위 호환

  Scenario: group_by 없이 Configure 시 단일 버퍼 모드
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 3             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
    When temperature 값 [20.0, 25.0, 30.0]을 가진 메시지 3건이 Process된다
    Then 출력 메시지의 payload.result는 25.0이다
    And 출력 메시지의 payload.count는 3이다
    And 출력 메시지에 group_key 필드가 존재하지 않는다
    And 출력 메시지에 group_value 필드가 존재하지 않는다
    And 이는 SPEC-AGG-001 동작과 동일하다
```

### AC-102: group_by 빈 문자열 거부

```gherkin
Feature: group_by 빈 문자열 거부

  Scenario: group_by가 빈 문자열일 때 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 group_by: ""로 호출된다
    Then ErrAggregateGroupByInvalid 에러가 반환된다
    And 에러 메시지에 "must not be empty string"이 포함된다
```

## 2. 그룹별 독립 버퍼 (REQ-AGG-110, REQ-AGG-111, REQ-AGG-112)

### AC-110: 그룹별 버퍼 할당

```gherkin
Feature: 그룹별 독립 버퍼 할당

  Scenario: 서로 다른 그룹의 메시지가 별도 버퍼에 저장
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count       |
      | window_size  | 10          |
      | aggregate_fn | "avg"       |
      | field        | "temperature" |
      | group_by     | "location"  |
    When 다음 메시지가 순서대로 Process된다:
      | location | temperature |
      | "seoul"  | 20.0        |
      | "busan"  | 25.0        |
      | "seoul"  | 22.0        |
    Then 내부 groupBuffers["seoul"] 길이는 2이다
    And 내부 groupBuffers["busan"] 길이는 1이다
```

### AC-111: count 윈도우 그룹별 독립 플러시

```gherkin
Feature: count 윈도우 그룹별 독립 플러시

  Scenario: 그룹 A가 windowSize에 도달하면 그룹 A만 플러시
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 2             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    When 다음 메시지가 순서대로 Process된다:
      | location | temperature |
      | "seoul"  | 20.0        |
      | "busan"  | 30.0        |
      | "seoul"  | 24.0        |
    Then "seoul" 그룹의 집계 결과 1건이 출력된다
    And 출력 메시지의 group_key는 "location"이다
    And 출력 메시지의 group_value는 "seoul"이다
    And 출력 메시지의 stats.temperature.avg는 22.0이다
    And "busan" 그룹은 아직 플러시되지 않는다

  Scenario: 각 그룹이 독립적으로 windowSize에 도달하여 플러시
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 2             |
      | aggregate_fn | "sum"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    When 다음 메시지가 순서대로 Process된다:
      | location | temperature |
      | "seoul"  | 10.0        |
      | "busan"  | 20.0        |
      | "seoul"  | 15.0        |
      | "busan"  | 25.0        |
    Then "seoul" 그룹의 출력 stats.temperature.sum은 25.0이다
    And "busan" 그룹의 출력 stats.temperature.sum은 45.0이다
```

### AC-112: time 윈도우 그룹별 동시 플러시

```gherkin
Feature: time 윈도우 그룹별 동시 플러시

  Scenario: 타이머 만료 시 비어있지 않은 모든 그룹 플러시
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | time          |
      | window_size  | "50ms"        |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    And Init이 호출되어 타이머가 시작된다
    When 다음 메시지가 50ms 이내에 Process된다:
      | location | temperature |
      | "seoul"  | 20.0        |
      | "busan"  | 30.0        |
      | "seoul"  | 24.0        |
    And 100ms를 대기한다 (타이머 만료)
    Then lastFlushResult에 2건의 출력 메시지가 저장되어 있다
    And "seoul" 그룹 결과의 stats.temperature.avg는 22.0이다
    And "busan" 그룹 결과의 stats.temperature.avg는 30.0이다
```

## 3. 출력 형식 (REQ-AGG-130, REQ-AGG-131, REQ-AGG-132)

### AC-130: 그룹화된 출력 메시지 구조

```gherkin
Feature: 그룹화된 출력 메시지 구조

  Scenario: 그룹 모드 출력에 group_key, group_value, stats 포함
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 3                                  |
      | aggregate_fn | ["avg", "min", "max", "count"]     |
      | fields       | ["temperature", "humidity"]        |
      | group_by     | "location"                         |
    When 다음 "seoul" 메시지 3건이 Process된다:
      | location | temperature | humidity |
      | "seoul"  | 20.0        | 60.0     |
      | "seoul"  | 25.0        | 70.0     |
      | "seoul"  | 30.0        | 50.0     |
    Then 출력 메시지 페이로드는 다음 구조를 가진다:
      """json
      {
        "group_key": "location",
        "group_value": "seoul",
        "stats": {
          "temperature": {
            "avg": 25.0,
            "min": 20.0,
            "max": 30.0,
            "count": 3
          },
          "humidity": {
            "avg": 60.0,
            "min": 50.0,
            "max": 70.0,
            "count": 3
          }
        },
        "window_count": 3,
        "window_type": "count",
        "window_size": 3
      }
      """
```

### AC-131: 그룹별 메타데이터 확장

```gherkin
Feature: 그룹별 메타데이터 확장

  Scenario: 그룹 모드 출력에 그룹 메타데이터 포함
    Given AggregateNode가 group_by: "location", aggregate_fn: ["avg"], fields: ["temperature"]로 구성된다
    When "seoul" 그룹의 윈도우 플러시가 발생한다
    Then 출력 메시지의 metadata._group_key는 "location"이다
    And 출력 메시지의 metadata._group_value는 "seoul"이다
    And 출력 메시지의 metadata._aggregate_fn은 "avg"이다
    And 출력 메시지의 metadata._fields는 "temperature"이다
```

### AC-132: 비그룹 모드 출력 호환

```gherkin
Feature: 비그룹 모드 출력 호환

  Scenario: group_by 미설정 시 SPEC-AGG-001 출력 형식 유지
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 3             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
    When temperature 값 [10.0, 20.0, 30.0]을 가진 메시지 3건이 Process된다
    Then 출력 메시지 페이로드의 result는 20.0이다
    And 출력 메시지 페이로드의 count는 3이다
    And 출력 메시지 페이로드의 stats.temperature.avg는 20.0이다
    And 출력 메시지 페이로드에 group_key가 존재하지 않는다
    And 출력 메시지 페이로드에 group_value가 존재하지 않는다
```

## 4. max_groups 설정 (REQ-AGG-140, REQ-AGG-141, REQ-AGG-142)

### AC-140: max_groups 설정 파싱

```gherkin
Feature: max_groups 설정 파싱

  Scenario: max_groups 양의 정수 설정
    Given AggregateNode가 생성된다
    When Configure가 max_groups: 50으로 호출된다
    Then 내부 maxGroups는 50이다

  Scenario: max_groups 미설정 시 기본값 100
    Given AggregateNode가 생성된다
    When Configure가 max_groups 없이 호출된다
    Then 내부 maxGroups는 100이다
```

### AC-141: max_groups 초과 시 메시지 드롭

```gherkin
Feature: max_groups 초과 시 메시지 드롭

  Scenario: 활성 그룹이 max_groups에 도달하면 신규 그룹 메시지 드롭
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 100           |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
      | max_groups   | 2             |
    When 다음 메시지가 순서대로 Process된다:
      | location   | temperature |
      | "seoul"    | 20.0        |
      | "busan"    | 25.0        |
      | "daegu"    | 30.0        |
    Then 내부 groupBuffers 키는 ["seoul", "busan"]이다
    And "daegu" 메시지는 드롭되어 groupBuffers에 포함되지 않는다

  Scenario: max_groups 도달 후에도 기존 그룹 메시지는 정상 처리
    Given AggregateNode가 max_groups: 2, group_by: "location"으로 구성된다
    And "seoul"과 "busan" 그룹이 이미 존재한다
    When "seoul" 그룹의 메시지가 추가로 Process된다
    Then "seoul" 그룹 버퍼에 메시지가 정상 추가된다
```

### AC-142: max_groups 유효성 검증

```gherkin
Feature: max_groups 유효성 검증

  Scenario: max_groups가 0일 때 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 max_groups: 0으로 호출된다
    Then ErrAggregateMaxGroupsInvalid 에러가 반환된다

  Scenario: max_groups가 음수일 때 Configure 실패
    Given AggregateNode가 생성된다
    When Configure가 max_groups: -1로 호출된다
    Then ErrAggregateMaxGroupsInvalid 에러가 반환된다
    And 에러 메시지에 "must be a positive integer"가 포함된다
```

## 5. 그룹 키 누락 처리 (REQ-AGG-150, REQ-AGG-151)

### AC-150: 그룹 키 필드 누락 시 _unknown 그룹

```gherkin
Feature: 그룹 키 누락 시 _unknown 그룹

  Scenario: 메시지에 group_by 필드가 없으면 _unknown 그룹에 배정
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 2             |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    When 다음 메시지가 순서대로 Process된다:
      | location | temperature |
      | "seoul"  | 20.0        |
      | (없음)   | 25.0        |
      | (없음)   | 30.0        |
    Then 내부 groupBuffers["seoul"] 길이는 1이다
    And 내부 groupBuffers["_unknown"] 길이는 2이다

  Scenario: _unknown 그룹도 정상적으로 플러시
    Given AggregateNode가 window_size: 2, group_by: "location"으로 구성된다
    When group_by 필드가 없는 메시지 2건이 Process된다 (temperature: [20.0, 30.0])
    Then "_unknown" 그룹의 출력 메시지가 생성된다
    And 출력의 group_value는 "_unknown"이다
    And 출력의 stats.temperature.avg는 25.0이다
```

### AC-151: 그룹 키 타입 변환

```gherkin
Feature: 그룹 키 타입 변환

  Scenario: float64 타입 그룹 키 문자열 변환
    Given AggregateNode가 group_by: "zone_id"로 구성된다
    When zone_id 값이 float64(1.5)인 메시지가 Process된다
    Then 메시지는 그룹 키 "1.5"에 배정된다

  Scenario: int 타입 그룹 키 문자열 변환
    Given AggregateNode가 group_by: "floor"로 구성된다
    When floor 값이 int(3)인 메시지가 Process된다
    Then 메시지는 그룹 키 "3"에 배정된다

  Scenario: bool 타입 그룹 키 문자열 변환
    Given AggregateNode가 group_by: "active"로 구성된다
    When active 값이 bool(true)인 메시지가 Process된다
    Then 메시지는 그룹 키 "true"에 배정된다
```

## 6. 빈 그룹 처리 (REQ-AGG-160, REQ-AGG-161)

### AC-160: time 윈도우 빈 그룹 스킵

```gherkin
Feature: time 윈도우 빈 그룹 스킵

  Scenario: 타이머 만료 시 비어있는 그룹은 플러시하지 않음
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | time          |
      | window_size  | "50ms"        |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    And Init이 호출되어 타이머가 시작된다
    When "seoul" 메시지 1건만 50ms 이내에 Process된다
    And 이전 주기에서 "busan" 그룹의 빈 버퍼가 존재한다
    And 100ms를 대기한다 (타이머 만료)
    Then lastFlushResult에 "seoul" 그룹 결과 1건만 존재한다
    And "busan" 그룹 결과는 생성되지 않는다
```

### AC-161: 플러시 후 버퍼 초기화

```gherkin
Feature: 플러시 후 그룹 버퍼 초기화

  Scenario: 그룹 플러시 후 버퍼가 비워지고 재사용 가능
    Given AggregateNode가 window_size: 2, group_by: "location"으로 구성된다
    When "seoul" 그룹 메시지 2건이 Process되어 플러시가 발생한다
    Then "seoul" 그룹의 첫 번째 집계 결과가 출력된다
    When 추가 "seoul" 그룹 메시지 2건이 Process된다
    Then "seoul" 그룹의 두 번째 집계 결과가 출력된다
    And 두 번째 결과는 추가 2건만으로 집계된 값이다
```

## 7. 동시성 안전 (REQ-AGG-170)

### AC-170: 그룹 모드 동시성 안전

```gherkin
Feature: 그룹 모드 동시성 안전

  Scenario: 동시 Process 호출 시 그룹 버퍼 데이터 레이스 없음
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 100                                |
      | aggregate_fn | ["avg", "min", "max"]              |
      | fields       | ["temperature", "humidity"]        |
      | group_by     | "location"                         |
    When 10개의 고루틴이 동시에 다음을 수행한다:
      - 각 고루틴이 "seoul" 또는 "busan" 그룹으로 10건의 메시지를 Process
    Then go test -race 플래그에서 데이터 레이스가 보고되지 않는다
    And 패닉(panic)이 발생하지 않는다

  Scenario: time 윈도우에서 동시 Process + 타이머 플러시 경쟁 없음
    Given AggregateNode가 time 윈도우 "50ms", group_by: "location"으로 구성된다
    When 여러 고루틴이 동시에 메시지를 Process하는 중 타이머가 만료된다
    Then go test -race 플래그에서 데이터 레이스가 보고되지 않는다
```

## 8. 기존 기능 통합 (REQ-AGG-180, REQ-AGG-181)

### AC-180: group_by + 다중 필드/함수 통합

```gherkin
Feature: group_by와 SPEC-AGG-001 기능 통합

  Scenario: group_by + fields + aggregate_fn 전체 조합 검증
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count                              |
      | window_size  | 2                                  |
      | aggregate_fn | ["avg", "min", "max"]              |
      | fields       | ["temperature", "humidity"]        |
      | group_by     | "location"                         |
    When 다음 메시지가 순서대로 Process된다:
      | location | temperature | humidity |
      | "seoul"  | 20.0        | 60.0     |
      | "busan"  | 30.0        | 40.0     |
      | "seoul"  | 24.0        | 80.0     |
      | "busan"  | 26.0        | 50.0     |
    Then "seoul" 그룹 출력의 stats는 다음과 같다:
      | 필드        | avg  | min  | max  |
      | temperature | 22.0 | 20.0 | 24.0 |
      | humidity    | 70.0 | 60.0 | 80.0 |
    And "busan" 그룹 출력의 stats는 다음과 같다:
      | 필드        | avg  | min  | max  |
      | temperature | 28.0 | 26.0 | 30.0 |
      | humidity    | 45.0 | 40.0 | 50.0 |
    And 각 그룹 출력의 window_count는 2이다
```

### AC-181: Shutdown 시 잔여 그룹 버퍼 플러시

```gherkin
Feature: Shutdown 시 잔여 그룹 버퍼 플러시

  Scenario: Shutdown 시 비어있지 않은 모든 그룹 버퍼 플러시
    Given AggregateNode가 다음 설정으로 구성된다:
      | window_type  | count         |
      | window_size  | 100           |
      | aggregate_fn | "avg"         |
      | field        | "temperature" |
      | group_by     | "location"    |
    And 다음 메시지가 Process되어 있다 (windowSize 미도달):
      | location | temperature |
      | "seoul"  | 20.0        |
      | "seoul"  | 24.0        |
      | "busan"  | 30.0        |
    When Shutdown이 호출된다
    Then lastFlushResult에 2건의 출력 메시지가 저장되어 있다
    And "seoul" 그룹의 stats.temperature.avg는 22.0이다
    And "busan" 그룹의 stats.temperature.avg는 30.0이다
```

## 9. 기존 테스트 회귀 검증

### AC-COMPAT: SPEC-AGG-001 기존 테스트 전체 PASS

```gherkin
Feature: SPEC-AGG-001 하위 호환성 보장

  Scenario: 기존 테스트 스위트 전체 통과
    Given 기존 aggregate_test.go의 모든 SPEC-AGG-001 테스트가 존재한다
    When go test -race ./internal/node/... 을 실행한다
    Then 다음 기존 테스트가 모두 PASS한다:
      | TestNewAggregateNode_정상생성                       |
      | TestAggregateNode_Init_상태전이                      |
      | TestAggregateNode_Configure_카운트윈도우              |
      | TestAggregateNode_Configure_타임윈도우                |
      | TestAggregateNode_Configure_nil에러                  |
      | TestAggregateNode_Configure_다중함수_리스트파싱        |
      | TestAggregateNode_Configure_다중필드_리스트파싱        |
      | TestAggregateNode_Process_카운트윈도우_Sum            |
      | TestAggregateNode_Process_카운트윈도우_Avg            |
      | TestAggregateNode_Process_카운트윈도우_Count          |
      | TestAggregateNode_Process_카운트윈도우_Min            |
      | TestAggregateNode_Process_카운트윈도우_Max            |
      | TestAggregateNode_Process_카운트윈도우_First          |
      | TestAggregateNode_Process_카운트윈도우_Last           |
      | TestAggregateNode_Process_비숫자값_스킵               |
      | TestAggregateNode_Process_정수값_숫자변환             |
      | TestAggregateNode_Process_다중필드_다중함수_Stats출력   |
      | TestAggregateNode_Process_타임윈도우_자동플러시         |
      | TestAggregateNode_Shutdown_상태전이                   |
      | TestAggregateNode_Shutdown_잔여버퍼플러시              |
      | TestAggregateNode_동시성안전_Process                  |
      | TestAggregateNode_Process_집계함수테이블               |
    And 데이터 레이스가 보고되지 않는다
```

## 10. 품질 게이트 (Quality Gates)

### Definition of Done

- [ ] 모든 기존 SPEC-AGG-001 테스트 PASS (하위 호환성 확인)
- [ ] 신규 테스트 PASS (group_by Configure, 그룹별 버퍼, 그룹별 플러시, 출력 구조)
- [ ] `go test -race ./internal/node/...` 데이터 레이스 0건
- [ ] `go vet ./...` 경고 0건
- [ ] 테스트 커버리지 85% 이상 (`internal/node/aggregate.go` 기준)
- [ ] `examples/flows/mqtt-metrics.yaml` group_by 설정 예시 포함
- [ ] 그룹 모드 출력 구조가 명세(spec.md 5.5)와 일치
- [ ] 비그룹 모드에서 SPEC-AGG-001 출력 형식 100% 유지 확인
- [ ] 메타데이터에 `_group_key`, `_group_value` 포함 확인 (그룹 모드)
- [ ] max_groups 초과 시 메시지 드롭 및 로그 출력 확인
- [ ] _unknown 그룹 할당 동작 확인
- [ ] Shutdown 시 잔여 그룹 버퍼 플러시 확인

### 검증 방법

| 항목 | 도구 | 명령어 |
|------|------|--------|
| 단위 테스트 | Go testing | `go test -v ./internal/node/...` |
| 레이스 검출 | Go race detector | `go test -race ./internal/node/...` |
| 커버리지 | Go cover | `go test -cover -coverprofile=coverage.out ./internal/node/...` |
| 정적 분석 | Go vet | `go vet ./...` |
| YAML 로드 | 통합 테스트 | 플로우 설정 로드 및 Configure 호출 검증 |
