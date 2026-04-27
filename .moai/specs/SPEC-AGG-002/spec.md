---
id: SPEC-AGG-002
title: "Aggregate Node Group-By Partitioning"
version: "1.3.0"
status: completed
created: "2026-02-23"
updated: "2026-02-23"
author: "xtra"
priority: high
related_specs:
  - SPEC-AGG-001
tags:
  - aggregate
  - group-by
  - partitioning
  - iot
  - statistics
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-23 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-02-23 | xtra | 복합 키(composite key) 지원 추가 (REQ-AGG-100a~100c) |
| 1.2.0 | 2026-02-23 | xtra | Sliding Window 지원 추가 (REQ-AGG-190~195), 섹션 번호 수정 |
| 1.3.0 | 2026-02-23 | xtra | 구현 완료 (status: completed), Implementation Notes 추가 |

# SPEC-AGG-002: Aggregate 노드 Group-By 파티셔닝 기능

## 1. 개요

### 1.1 목적

xflow Aggregate 노드에 `group_by` 설정을 추가하여, 메시지 페이로드의 특정 필드 값을 기준으로 메시지를 그룹별로 분류하고 **그룹별 독립 버퍼**에서 개별 집계를 수행하는 기능을 구현한다. 이를 통해 단일 Aggregate 노드가 여러 센서/장치별 통계를 동시에 산출할 수 있다.

### 1.2 배경

SPEC-AGG-001에서 다중 필드/다중 함수 집계가 구현되었으나, 모든 메시지가 **단일 버퍼**에 축적되어 집계된다. IoT 파이프라인에서는 `location`, `device_id`, `sensor_type` 등 특정 필드 값을 기준으로 메시지를 분류하여 그룹별 독립 통계를 산출해야 하는 요구가 빈번하다.

현재 구조에서 그룹별 집계를 수행하려면 upstream에서 메시지를 그룹별로 분기하는 별도 노드를 배치하거나, 동일한 Aggregate 노드를 그룹 수만큼 복제해야 하므로 플로우가 불필요하게 복잡해진다.

### 1.3 범위

- **포함**: `group_by` 설정 키 파싱 (Configure 확장)
- **포함**: 그룹별 독립 버퍼 관리 (`map[string][]message.Message`)
- **포함**: 그룹별 독립 집계 실행 (count/time/sliding 윈도우 모두 지원)
- **포함**: 그룹화된 출력 형식 (`groups` 맵 구조)
- **포함**: `max_groups` 설정을 통한 메모리 보호
- **포함**: `group_by` 미설정 시 기존 단일 버퍼 동작 유지 (SPEC-AGG-001 하위 호환)
- **포함**: 그룹 키 누락 메시지의 `_unknown` 그룹 처리
- **포함**: 예제 플로우 YAML 업데이트 (`mqtt-metrics.yaml`)
- **포함**: 복합 키 그룹핑 (`group_by`에 문자열 배열 지원, `|` 구분자로 키 결합)
- **포함**: Sliding Window (`window_type: "sliding"`, `slide_interval` 설정)
- **제외**: 그룹별 서로 다른 윈도우 크기 설정
- **제외**: 그룹 결과의 외부 저장소 연동 (SPEC-INFLUX-001 범위)

## 2. 환경 (Environment)

### 2.1 시스템 환경

- **언어**: Go 1.25+
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`
- **대상 파일**: `internal/node/aggregate.go`, `internal/node/aggregate_test.go`
- **관련 파일**: `examples/flows/mqtt-metrics.yaml`, `pkg/message/message.go`

### 2.2 기존 아키텍처 (SPEC-AGG-001 구현 완료 기준)

- `AggregateNode` 구조체가 `BaseNode`를 임베딩
- `sync.Mutex`로 동시성 보호
- `aggregateFns []AggregateFn` - 다중 집계 함수 슬라이스
- `fields []string` - 다중 집계 대상 필드 리스트
- `buffer []message.Message` - **단일 버퍼** (이 SPEC에서 그룹별 버퍼로 확장)
- count 윈도우: `len(buffer) >= windowSize`일 때 플러시
- time 윈도우: `time.AfterFunc`로 주기적 플러시
- `extractNumericValuesForField(buf, fieldName)` - 필드별 숫자 값 추출
- `computeAggregate(fn, values, buf)` - 개별 집계 함수 실행
- `executeAggregate(buf)` - 다중 모드/단일 모드 판별 후 집계 실행
- `windowSizeRaw any` - Configure 시 원본 window_size 값 보존

### 2.3 의존성

- `pkg/message`: Payload의 `Get(key)`, `Set(key, value)` 인터페이스
- `pkg/flow`: `NodeDef` 및 플로우 직렬화
- `pkg/lifecycle`: 노드 라이프사이클 상태 전이
- SPEC-AGG-001: 다중 필드/다중 함수 집계 기반 (선행 구현 필수)

## 3. 가정 (Assumptions)

- **A1**: SPEC-AGG-001의 다중 필드/다중 함수 집계가 완전히 구현된 상태에서 본 SPEC을 구현한다.
- **A2**: `group_by`는 단일 문자열 키 또는 문자열 배열(복합 키)을 지원한다. 복합 키 사용 시 각 필드 값을 `|` 구분자로 결합하여 그룹 키를 생성한다.
- **A3**: `group_by` 대상 필드의 값은 문자열 또는 문자열로 변환 가능한 타입(`string`, `float64`, `int`)이다.
- **A4**: 그룹 키 값은 메시지 페이로드의 최상위 키에 해당한다 (중첩 경로 미지원).
- **A8**: 복합 키 구분자는 `|`(파이프)로 고정한다. 필드 값에 `|`가 포함된 경우는 고려하지 않는다.
- **A5**: 모든 그룹은 동일한 윈도우 설정(type, size), 집계 함수, 대상 필드를 공유한다.
- **A9**: Sliding Window의 `slide_interval`은 `window_size`보다 작거나 같아야 한다. 기본값은 `window_size / 10`이다.
- **A10**: Sliding Window에서 메시지 수신 시각은 `Process()` 호출 시점의 `time.Now()`를 사용한다 (메시지 내부 타임스탬프 미사용).
- **A6**: `group_by` 미설정 시 기존 SPEC-AGG-001 동작과 100% 동일해야 한다 (단일 버퍼 모드).
- **A7**: `max_groups` 기본값은 100이다. 그룹 수가 `max_groups`를 초과하면 신규 그룹의 메시지는 버린다(drop).

## 4. 요구사항 (Requirements)

### 4.1 group_by 설정 지원

**REQ-AGG-100**: group_by 단일 키 파싱
- **WHEN** `group_by` 설정 값이 문자열로 제공되면, **THEN** 시스템은 해당 필드명을 그룹 분류 키로 저장해야 한다.
- 설정 예: `group_by: "location"`

**REQ-AGG-100a**: group_by 복합 키 파싱
- **WHEN** `group_by` 설정 값이 문자열 배열로 제공되면, **THEN** 시스템은 배열의 모든 필드명을 복합 그룹 분류 키로 저장해야 한다.
- 설정 예: `group_by: ["location", "device_id"]`
- 배열이 비어있으면 `Configure` 단계에서 에러를 반환해야 한다.

**REQ-AGG-100b**: 복합 키 그룹 값 생성
- **WHEN** 복합 키가 설정된 상태에서 메시지가 Process되면, **THEN** 시스템은 각 필드 값을 `|` 구분자로 결합하여 단일 그룹 키 문자열을 생성해야 한다.
- 예: `location="factory-A"`, `device_id="sensor-001"` → 그룹 키 = `"factory-A|sensor-001"`

**REQ-AGG-100c**: 복합 키 출력 확장
- **WHEN** 복합 키가 설정된 상태에서 그룹 플러시가 발생하면, **THEN** 출력 메시지에 `group_keys` (필드명 배열)와 `group_values` (필드명→값 맵)를 추가해야 한다.
- 예: `group_keys: ["location", "device_id"]`, `group_values: {"location": "factory-A", "device_id": "sensor-001"}`

**REQ-AGG-101**: group_by 미설정 시 하위 호환
- **IF** `group_by`가 설정되지 않은 경우, **THEN** 시스템은 기존 단일 버퍼 방식으로 동작하여 SPEC-AGG-001과 동일한 결과를 생성해야 한다.

**REQ-AGG-102**: group_by 빈 문자열 거부
- **IF** `group_by`가 빈 문자열(`""`)로 제공되면, **THEN** 시스템은 `Configure` 단계에서 에러를 반환해야 한다.

### 4.2 그룹별 독립 버퍼

**REQ-AGG-110**: 그룹별 버퍼 생성
- **WHEN** `group_by`가 설정된 상태에서 메시지가 Process되면, **THEN** 시스템은 `extractGroupKey(msg)` 로직으로 그룹 키를 추출하여 해당 그룹의 버퍼에 메시지를 추가해야 한다.
- 단일 키: `msg.Payload().Get(groupByKeys[0])` 값 사용
- 복합 키: 각 `msg.Payload().Get(key)` 값을 `|`로 결합하여 사용

**REQ-AGG-111**: 그룹별 독립 count 윈도우
- **WHEN** count 윈도우 모드에서 특정 그룹의 버퍼 크기가 `windowSize`에 도달하면, **THEN** 시스템은 해당 그룹의 버퍼만 플러시하고 집계를 실행해야 한다.
- 다른 그룹의 버퍼는 영향받지 않아야 한다.

**REQ-AGG-112**: 그룹별 독립 time 윈도우
- **WHEN** time 윈도우 모드에서 타이머가 만료되면, **THEN** 시스템은 비어있지 않은 모든 그룹의 버퍼를 동시에 플러시하고 각 그룹별 집계를 실행해야 한다.
- 비어있는 그룹의 버퍼는 플러시하지 않아야 한다.

### 4.3 그룹별 집계 실행

**REQ-AGG-120**: 그룹별 독립 집계
- **WHEN** 그룹 버퍼가 플러시되면, **THEN** 시스템은 해당 그룹의 메시지만으로 집계 함수를 실행해야 한다.
- 기존 `executeAggregate` 로직(다중 필드/함수 매트릭스)이 그룹 단위로 적용되어야 한다.

**REQ-AGG-121**: count 윈도우 그룹별 플러시 시 단일 출력
- **WHEN** count 윈도우에서 특정 그룹이 플러시되면, **THEN** 해당 그룹의 집계 결과 1건만 출력 메시지로 생성해야 한다.

**REQ-AGG-122**: time 윈도우 그룹별 플러시 시 다중 출력
- **WHEN** time 윈도우에서 타이머 만료로 여러 그룹이 동시에 플러시되면, **THEN** 각 그룹별로 독립된 출력 메시지를 생성하고, 그룹 결과 목록을 `lastFlushResult`에 저장해야 한다.

### 4.4 출력 형식

**REQ-AGG-130**: 그룹화된 출력 메시지 구조
- **WHEN** `group_by`가 설정된 상태에서 그룹 버퍼가 플러시되면, **THEN** 각 출력 메시지 페이로드는 다음 구조를 포함해야 한다:
  - 단일 키: `group_key` (string) + `group_value` (string)
  - 복합 키: `group_keys` ([]string) + `group_value` (string, `|` 결합) + `group_values` (map[string]string, 필드명→값)
  - `stats`: 기존 SPEC-AGG-001의 필드별 x 함수별 매트릭스 (`map[string]map[string]any`)
  - `window_count`: 해당 그룹 윈도우 내 메시지 수 (`int`)
  - `window_type`: 윈도우 타입 (`string`)
  - `window_size`: 윈도우 크기 설정값 (`any`)

**REQ-AGG-131**: 그룹별 메타데이터 확장
- 시스템은 **항상** 그룹화된 출력 메시지의 메타데이터에 다음을 포함해야 한다:
  - `_group_key`: 그룹 분류 필드명
  - `_group_value`: 해당 그룹의 키 값
  - 기존 메타데이터(`_aggregate_fn`, `_window_type`, `_fields`) 유지

**REQ-AGG-132**: 비그룹 모드 출력 호환
- **WHEN** `group_by`가 설정되지 않은 상태에서 플러시가 발생하면, **THEN** 출력 형식은 SPEC-AGG-001과 동일해야 한다 (기존 `stats` + `result`/`count` + `window_count` 구조).

### 4.5 max_groups 설정

**REQ-AGG-140**: max_groups 설정
- **WHEN** `max_groups` 설정 값이 양의 정수로 제공되면, **THEN** 시스템은 활성 그룹 수가 이 값을 초과하지 않도록 관리해야 한다.
- 기본값: 100

**REQ-AGG-141**: max_groups 초과 시 메시지 처리
- **IF** 활성 그룹 수가 `max_groups`에 도달한 상태에서 새로운 그룹 키를 가진 메시지가 도착하면, **THEN** 시스템은 해당 메시지를 드롭(drop)하고 로그를 남겨야 한다.
- 기존 그룹에 속하는 메시지는 정상 처리해야 한다.

**REQ-AGG-142**: max_groups 유효성 검증
- **IF** `max_groups`가 0 이하의 값으로 설정되면, **THEN** 시스템은 `Configure` 단계에서 에러를 반환해야 한다.

### 4.6 그룹 키 누락 처리

**REQ-AGG-150**: 그룹 키 필드 누락 시 `_unknown` 처리
- **IF** 단일 키 모드에서 메시지 페이로드에 `group_by` 필드가 존재하지 않으면, **THEN** 시스템은 해당 메시지를 `"_unknown"` 그룹에 배정해야 한다.
- **IF** 복합 키 모드에서 일부 필드가 누락되면, **THEN** 해당 필드 값만 `"_unknown"`으로 대체하고 나머지 필드와 `|`로 결합하여 그룹 키를 생성해야 한다.
- 예: location="factory-A", device_id=nil → 그룹 키 = `"factory-A|_unknown"`

**REQ-AGG-151**: 그룹 키 값 타입 변환
- **WHEN** `group_by` 필드의 값이 문자열이 아닌 타입(`float64`, `int`, `bool`)이면, **THEN** 시스템은 `fmt.Sprintf("%v", value)` 형식으로 문자열 변환하여 그룹 키로 사용해야 한다.

### 4.7 Sliding Window 지원

**REQ-AGG-190**: sliding 윈도우 타입 설정
- **WHEN** `window_type`이 `"sliding"`으로 설정되면, **THEN** 시스템은 슬라이딩 윈도우 모드로 동작해야 한다.
- `window_size`는 전체 윈도우 범위를 지정한다 (예: `"1h"` = 최근 1시간).
- `slide_interval`은 집계 주기를 지정한다 (예: `"10m"` = 10분마다 집계).

**REQ-AGG-191**: slide_interval 설정
- **WHEN** `slide_interval`이 문자열 duration으로 제공되면, **THEN** 시스템은 해당 주기로 슬라이딩 윈도우 집계를 실행해야 한다.
- **IF** `slide_interval`이 미설정이면, **THEN** 기본값은 `window_size / 10`으로 설정한다.
- **IF** `slide_interval`이 `window_size`보다 크면, **THEN** `Configure` 단계에서 에러를 반환해야 한다.

**REQ-AGG-192**: sliding 윈도우 메시지 보관
- **WHEN** sliding 모드에서 메시지가 Process되면, **THEN** 시스템은 메시지와 함께 수신 시각(`time.Now()`)을 기록하여 버퍼에 저장해야 한다.
- 비그룹 모드: 단일 타임스탬프 버퍼
- 그룹 모드: 그룹별 독립 타임스탬프 버퍼

**REQ-AGG-193**: sliding 윈도우 집계 실행
- **WHEN** `slide_interval` 타이머가 만료되면, **THEN** 시스템은 현재 시각에서 `window_size` 이전까지의 메시지만 선택하여 집계를 실행해야 한다.
- 윈도우 범위 밖의 오래된 메시지는 버퍼에서 제거(evict)해야 한다.

**REQ-AGG-194**: sliding 윈도우 그룹 모드 통합
- **WHEN** `group_by`와 `window_type: "sliding"`이 동시에 설정되면, **THEN** 시스템은 각 그룹별로 독립된 슬라이딩 윈도우를 유지해야 한다.
- 각 그룹의 타임스탬프 버퍼에서 윈도우 범위 내 메시지만 집계한다.

**REQ-AGG-195**: sliding 윈도우 출력 형식
- 슬라이딩 윈도우의 출력 메시지는 기존 그룹/비그룹 출력 구조에 추가로 다음을 포함해야 한다:
  - `window_start`: 윈도우 시작 시각 (RFC3339 문자열)
  - `window_end`: 윈도우 종료 시각 (RFC3339 문자열)
  - `window_type`: `"sliding"`

### 4.8 빈 그룹 처리

**REQ-AGG-160**: time/sliding 윈도우 빈 그룹 스킵
- **WHEN** time 또는 sliding 윈도우 타이머 만료 시 특정 그룹의 버퍼가 비어있으면(또는 윈도우 범위 내 메시지가 없으면), **THEN** 시스템은 해당 그룹에 대해 플러시를 수행하지 않아야 한다.

**REQ-AGG-161**: 플러시 후 그룹 버퍼 초기화
- **WHEN** 그룹 버퍼가 플러시된 후, **THEN** 시스템은 해당 그룹의 버퍼를 비워야 한다.
- 그룹 맵에서 키를 제거하지 않고 빈 슬라이스로 초기화한다 (재사용).

### 4.9 동시성 안전

**REQ-AGG-170**: 그룹별 버퍼 스레드 안전성
- 시스템은 **항상** 그룹별 버퍼 맵 접근 시 `sync.Mutex` 기반 동시성 보호를 유지해야 한다.
- 시스템은 `go test -race`에서 데이터 레이스가 발생**하지 않아야 한다**.

### 4.10 기존 기능 통합

**REQ-AGG-180**: SPEC-AGG-001 기능과의 통합
- 시스템은 **항상** `group_by` + 다중 필드(`fields`) + 다중 함수(`aggregate_fn`) 조합을 지원해야 한다.
- 그룹별 집계 시 기존 다중 필드/함수 매트릭스(`stats`)가 그대로 적용되어야 한다.

**REQ-AGG-181**: Shutdown 시 잔여 그룹 버퍼 플러시
- **WHEN** 노드가 Shutdown될 때, **THEN** 시스템은 비어있지 않은 모든 그룹의 버퍼를 플러시하고 집계 결과를 `lastFlushResult`에 저장해야 한다.

## 5. 명세 (Specifications)

### 5.1 구조체 변경

`AggregateNode` 구조체 필드 변경:

| 기존 필드 | 변경 후 | 설명 |
|-----------|---------|------|
| `buffer []message.Message` | `buffer []message.Message` (유지) | group_by 미설정 시 단일 버퍼 모드 |
| (신규) | `groupBuffers map[string][]message.Message` | 그룹별 독립 버퍼 맵 |
| (신규) | `groupByKeys []string` | 그룹 분류 기준 필드명 목록 (단일 키: 길이 1, 복합 키: 길이 2+) |
| (신규) | `maxGroups int` | 최대 허용 그룹 수 (기본값: 100) |
| (신규) | `slideDuration time.Duration` | 슬라이딩 윈도우 집계 주기 (slide_interval 파싱 결과) |
| (신규) | `timestampedMessage` | 내부 구조체: `{ msg message.Message, receivedAt time.Time }` |
| (신규) | `tsBuffer []timestampedMessage` | 슬라이딩 윈도우 비그룹 모드 타임스탬프 버퍼 |
| (신규) | `groupTsBuffers map[string][]timestampedMessage` | 슬라이딩 윈도우 그룹별 타임스탬프 버퍼 |

### 5.2 Configure 로직 확장

```
Configure 호출 시 (기존 SPEC-AGG-001 로직 이후 추가):
1. group_by 파싱:
   - string -> groupByKeys = []string{value} (단일 키)
   - []any (문자열 배열) -> groupByKeys = []string{values...} (복합 키)
   - 빈 문자열("") -> 에러 반환
   - 빈 배열 -> 에러 반환
   - 미설정 -> groupByKeys = nil (단일 버퍼 모드)
2. max_groups 파싱:
   - 양의 정수 -> maxGroups 필드에 저장
   - 미설정 -> maxGroups = 100 (기본값)
   - 0 이하 -> 에러 반환
3. group_by 설정 시 groupBuffers 맵 초기화:
   - groupBuffers = make(map[string][]message.Message)
4. window_type == "sliding" 추가 파싱:
   - slide_interval 파싱:
     - duration 문자열 -> time.ParseDuration(value) -> slideDuration
     - 미설정 -> slideDuration = windowDuration / 10 (기본값)
     - slideDuration > windowDuration -> 에러 반환
     - slideDuration <= 0 -> 에러 반환
   - 슬라이딩 윈도우 버퍼 초기화:
     - group_by 미설정: tsBuffer = []timestampedMessage{}
     - group_by 설정: groupTsBuffers = make(map[string][]timestampedMessage)
   - slide_interval 주기 타이머 시작 (time.AfterFunc, 반복)
```

### 5.3 Process 로직 변경

```
Process(msg) 호출 시:
1. group_by 미설정 (groupByKeys == nil):
   - 기존 단일 버퍼 로직 그대로 (SPEC-AGG-001)
2. group_by 설정 (groupByKeys != nil):
   a. groupKey 추출 (extractGroupKey):
      - 단일 키 (len(groupByKeys) == 1):
        - msg.Payload().Get(groupByKeys[0]) 호출
        - nil -> "_unknown"
        - string -> 그대로 사용
        - 기타 타입 -> fmt.Sprintf("%v", value)
      - 복합 키 (len(groupByKeys) >= 2):
        - 각 필드에 대해 msg.Payload().Get(key) 호출
        - nil -> "_unknown" (해당 필드만)
        - 각 값을 문자열 변환 후 "|"로 결합
        - 예: ["factory-A", "sensor-001"] -> "factory-A|sensor-001"
   b. 그룹 존재 여부 확인:
      - 기존 그룹 -> 해당 그룹 버퍼에 추가
      - 신규 그룹:
        - len(groupBuffers) >= maxGroups -> 메시지 드롭 + 로그
        - 그 외 -> 새 그룹 버퍼 생성 + 메시지 추가
   c. count 윈도우: 해당 그룹 버퍼 크기 확인 -> windowSize 도달 시 그룹 플러시
   d. time 윈도우: 버퍼 추가만 (타이머 만료 시 전체 그룹 플러시)
   e. sliding 윈도우: 타임스탬프 버퍼에 {msg, time.Now()} 추가만 (타이머 만료 시 집계)

3. window_type == "sliding" (비그룹 모드):
   a. tsBuffer에 {msg, time.Now()} 추가
   b. 타이머 만료 시 (slide_interval 주기):
      - now := time.Now()
      - cutoff := now.Add(-windowDuration)
      - tsBuffer에서 receivedAt < cutoff인 메시지 제거(evict)
      - 남은 메시지로 executeAggregate 실행
      - 출력에 window_start(cutoff), window_end(now) 추가

4. window_type == "sliding" (그룹 모드):
   a. extractGroupKey로 그룹 키 추출
   b. groupTsBuffers[groupKey]에 {msg, time.Now()} 추가
   c. 타이머 만료 시 (slide_interval 주기):
      - 모든 그룹 순회
      - 각 그룹의 타임스탬프 버퍼에서 윈도우 범위 밖 메시지 제거
      - 남은 메시지로 그룹별 독립 집계 실행
      - 빈 그룹(윈도우 내 메시지 0건) 스킵
```

### 5.4 그룹 플러시 로직

```
flushGroup(groupKey, buf) 호출 시:
1. executeAggregate(buf) 호출 (기존 SPEC-AGG-001 로직 재사용)
2. 출력 메시지에 그룹 메타데이터 추가:
   - 단일 키 (len(groupByKeys) == 1):
     - payload.Set("group_key", groupByKeys[0])
     - payload.Set("group_value", groupKey)
   - 복합 키 (len(groupByKeys) >= 2):
     - payload.Set("group_keys", groupByKeys)
     - payload.Set("group_value", groupKey)  // "|" 결합 문자열
     - payload.Set("group_values", map[string]string{...})  // 필드명→값 맵
3. metadata 추가:
   - metadata.Set("_group_keys", strings.Join(groupByKeys, ","))
   - metadata.Set("_group_value", groupKey)
4. 그룹 버퍼 초기화: groupBuffers[groupKey] = groupBuffers[groupKey][:0]

flushAllGroups() 호출 시 (time 윈도우 타이머 만료):
1. 모든 그룹 순회:
   - 버퍼가 비어있지 않은 그룹만 flushGroup 실행
2. 결과 메시지 목록을 lastFlushResult에 저장

slidingFlush() 호출 시 (slide_interval 타이머 만료, 비그룹 모드):
1. now := time.Now()
2. cutoff := now.Add(-windowDuration)
3. tsBuffer에서 receivedAt >= cutoff인 메시지만 필터링
4. tsBuffer에서 receivedAt < cutoff인 메시지 제거(evict)
5. 필터링된 메시지가 0건이면 스킵
6. 메시지 슬라이스 추출: msgs := extractMessages(filtered)
7. executeAggregate(msgs) 호출
8. 출력 메시지에 window_start, window_end 추가
9. 타이머 재시작 (slide_interval 후 다시 slidingFlush 호출)

slidingFlushAllGroups() 호출 시 (slide_interval 타이머 만료, 그룹 모드):
1. now := time.Now(), cutoff := now.Add(-windowDuration)
2. 모든 그룹 순회:
   - 각 그룹의 타임스탬프 버퍼에서 윈도우 범위 내 메시지 필터링
   - 범위 밖 메시지 제거(evict)
   - 유효 메시지 0건이면 해당 그룹 스킵
   - 유효 메시지로 flushGroup(groupKey, msgs) 실행
   - 출력 메시지에 window_start, window_end 추가
3. 결과 메시지 목록을 lastFlushResult에 저장
4. 타이머 재시작
```

### 5.5 출력 메시지 형식

**단일 키 그룹 모드 출력 예시** (group_by: "location"):

```json
{
  "group_key": "location",
  "group_value": "seoul",
  "stats": {
    "temperature": {"avg": 25.3, "count": 50},
    "humidity": {"avg": 65.2, "count": 50}
  },
  "window_count": 50,
  "window_type": "count",
  "window_size": 100
}
```

**복합 키 그룹 모드 출력 예시** (group_by: ["location", "device_id"]):

```json
{
  "group_keys": ["location", "device_id"],
  "group_value": "factory-A/line-3|sensor-001",
  "group_values": {
    "location": "factory-A/line-3",
    "device_id": "sensor-001"
  },
  "stats": {
    "temperature": {"avg": 25.3, "min": 18.5, "max": 32.1},
    "humidity": {"avg": 62.4, "min": 40.0, "max": 85.5}
  },
  "window_count": 50,
  "window_type": "time",
  "window_size": "10m"
}
```

**비그룹 모드 출력** (group_by 미설정):
- SPEC-AGG-001과 동일 (기존 `stats` + `result`/`count` 구조)

### 5.6 YAML 설정 예시

**단일 키 그룹별 집계 설정**:

```yaml
- name: "location-stats-10min"
  type: "aggregate"
  config:
    window_type: "time"
    window_size: "10m"
    aggregate_fn:
      - "avg"
      - "min"
      - "max"
    fields:
      - "temperature"
      - "humidity"
    group_by: "location"
    max_groups: 50
```

**복합 키 그룹별 집계 설정**:

```yaml
- name: "device-location-stats-1h"
  type: "aggregate"
  config:
    window_type: "time"
    window_size: "1h"
    aggregate_fn:
      - "avg"
      - "min"
      - "max"
    fields:
      - "temperature"
      - "humidity"
    group_by:
      - "location"
      - "device_id"
    max_groups: 200
```

**Sliding Window 그룹별 집계 설정**:

```yaml
- name: "location-stats-sliding-1h"
  type: "aggregate"
  config:
    window_type: "sliding"
    window_size: "1h"
    slide_interval: "10m"
    aggregate_fn:
      - "avg"
      - "min"
      - "max"
    fields:
      - "temperature"
      - "humidity"
    group_by: "location"
    max_groups: 50
```

**Sliding Window 비그룹 집계 설정**:

```yaml
- name: "global-stats-sliding-30m"
  type: "aggregate"
  config:
    window_type: "sliding"
    window_size: "30m"
    slide_interval: "5m"
    aggregate_fn: "avg"
    field: "temperature"
```

**하위 호환 설정** (group_by 미설정 - 기존과 동일):

```yaml
- name: "simple-aggregator"
  type: "aggregate"
  config:
    window_type: "count"
    window_size: 10
    aggregate_fn: "avg"
    field: "temperature"
```

### 5.7 에러 정의

| 에러 | 조건 | 메시지 |
|------|------|--------|
| `ErrAggregateGroupByInvalid` | group_by가 빈 문자열 | `"invalid aggregate group_by: must not be empty string"` |
| `ErrAggregateMaxGroupsInvalid` | max_groups가 0 이하 | `"invalid aggregate max_groups: must be a positive integer"` |
| `ErrAggregateSlideIntervalInvalid` | slide_interval이 window_size 초과 | `"invalid aggregate slide_interval: must be <= window_size"` |
| `ErrAggregateSlideIntervalParse` | slide_interval 파싱 실패 | `"invalid aggregate slide_interval: cannot parse duration"` |

## 6. 추적성 (Traceability)

| 요구사항 ID | 구현 파일 | 테스트 |
|-------------|-----------|--------|
| REQ-AGG-100 | `internal/node/aggregate.go` | `aggregate_test.go` - group_by 단일 키 Configure 파싱 |
| REQ-AGG-100a | `internal/node/aggregate.go` | `aggregate_test.go` - group_by 복합 키 Configure 파싱 |
| REQ-AGG-100b | `internal/node/aggregate.go` | `aggregate_test.go` - 복합 키 그룹 값 `|` 결합 생성 |
| REQ-AGG-100c | `internal/node/aggregate.go` | `aggregate_test.go` - 복합 키 출력 (group_keys, group_values) |
| REQ-AGG-101 | `internal/node/aggregate.go` | `aggregate_test.go` - group_by 미설정 하위 호환 |
| REQ-AGG-102 | `internal/node/aggregate.go` | `aggregate_test.go` - group_by 빈 문자열 에러 |
| REQ-AGG-110 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹별 버퍼 할당 |
| REQ-AGG-111 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹별 count 윈도우 플러시 |
| REQ-AGG-112 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹별 time 윈도우 플러시 |
| REQ-AGG-120 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹별 집계 실행 |
| REQ-AGG-121 | `internal/node/aggregate.go` | `aggregate_test.go` - count 윈도우 그룹 단일 출력 |
| REQ-AGG-122 | `internal/node/aggregate.go` | `aggregate_test.go` - time 윈도우 다중 그룹 출력 |
| REQ-AGG-130 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹 출력 구조 검증 |
| REQ-AGG-131 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹 메타데이터 검증 |
| REQ-AGG-132 | `internal/node/aggregate.go` | `aggregate_test.go` - 비그룹 출력 호환 검증 |
| REQ-AGG-140 | `internal/node/aggregate.go` | `aggregate_test.go` - max_groups 설정 파싱 |
| REQ-AGG-141 | `internal/node/aggregate.go` | `aggregate_test.go` - max_groups 초과 드롭 |
| REQ-AGG-142 | `internal/node/aggregate.go` | `aggregate_test.go` - max_groups 유효성 검증 |
| REQ-AGG-150 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹 키 누락 _unknown |
| REQ-AGG-151 | `internal/node/aggregate.go` | `aggregate_test.go` - 그룹 키 타입 변환 |
| REQ-AGG-160 | `internal/node/aggregate.go` | `aggregate_test.go` - time 윈도우 빈 그룹 스킵 |
| REQ-AGG-161 | `internal/node/aggregate.go` | `aggregate_test.go` - 플러시 후 버퍼 초기화 |
| REQ-AGG-170 | `internal/node/aggregate.go` | `aggregate_test.go` - race 테스트 (그룹 모드) |
| REQ-AGG-180 | `internal/node/aggregate.go` | `aggregate_test.go` - group_by + 다중 필드/함수 통합 |
| REQ-AGG-181 | `internal/node/aggregate.go` | `aggregate_test.go` - Shutdown 그룹 버퍼 플러시 |
| REQ-AGG-190 | `internal/node/aggregate.go` | `aggregate_test.go` - sliding 윈도우 타입 Configure |
| REQ-AGG-191 | `internal/node/aggregate.go` | `aggregate_test.go` - slide_interval 파싱 및 기본값 |
| REQ-AGG-192 | `internal/node/aggregate.go` | `aggregate_test.go` - sliding 메시지 타임스탬프 저장 |
| REQ-AGG-193 | `internal/node/aggregate.go` | `aggregate_test.go` - sliding 윈도우 eviction + 집계 |
| REQ-AGG-194 | `internal/node/aggregate.go` | `aggregate_test.go` - sliding + group_by 통합 |
| REQ-AGG-195 | `internal/node/aggregate.go` | `aggregate_test.go` - sliding 출력 window_start/end |

## 7. Implementation Notes

### 7.1 구현 요약

- **구현 커밋**: `8df095d`
- **개발 방법론**: Hybrid (TDD for new code, DDD for existing code modifications)
- **전체 요구사항**: 30개 (REQ-AGG-100 ~ REQ-AGG-195)
- **마일스톤**: 7개 (M1-M7) 전체 완료

### 7.2 구현 파일

| 파일 | 변경 유형 | 설명 |
|------|-----------|------|
| `internal/node/aggregate.go` | 핵심 수정 | Group-By 파티셔닝 + Sliding Window 구현 |
| `internal/node/aggregate_test.go` | 테스트 확장 | 그룹별 집계 + Sliding Window 테스트 추가 |
| `internal/node/errors.go` | 에러 추가 | ErrAggregateGroupByInvalid, ErrAggregateMaxGroupsInvalid, ErrAggregateSlideIntervalInvalid, ErrAggregateSlideIntervalParse |
| `examples/flows/mqtt-metrics.yaml` | 예제 업데이트 | group_by, sliding-aggregator 노드 추가 |
| `examples/flows/etl-pipeline.yaml` | 예제 업데이트 | Aggregate 노드 활용 예시 |
| `examples/flows/iot-sensor.json` | 예제 업데이트 | IoT 센서 파이프라인 Aggregate 활용 |
| `examples/docs/mqtt-metrics-flow.md` | 신규 문서 | Mermaid 다이어그램 + 노드 설명 |

### 7.3 품질 결과

- **테스트**: 전체 통과
- **커버리지**: 92.9%
- **Race Detector**: 이상 없음
- **Go Vet**: 이상 없음

### 7.4 SPEC 다이버전스

- 별도 scope expansion 없음
- 모든 30개 요구사항 구현 완료
- plan.md 대비 추가 파일: etl-pipeline.yaml, iot-sensor.json, examples/docs/mqtt-metrics-flow.md (예제 확장)
