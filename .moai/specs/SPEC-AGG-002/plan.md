---
id: SPEC-AGG-002
title: "Aggregate Node Group-By Partitioning"
version: "1.3.0"
status: completed
created: "2026-02-23"
updated: "2026-02-23"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-23 | xtra | 초기 구현 계획 작성 |
| 1.1.0 | 2026-02-23 | xtra | 복합 키(composite key) 지원 반영 |
| 1.2.0 | 2026-02-23 | xtra | Sliding Window 마일스톤 추가 (M6, M7) |
| 1.3.0 | 2026-02-23 | xtra | 구현 완료 (status: completed) |

# SPEC-AGG-002: 구현 계획

## 1. 구현 전략

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 기준):
- **기존 코드 수정** (DDD): `aggregate.go`의 기존 Process, executeAggregate 메서드 변경 시 ANALYZE-PRESERVE-IMPROVE 사이클
- **신규 코드 추가** (TDD): 그룹 버퍼 관리, group_by 파싱, flushGroup/flushAllGroups, Sliding Window 로직은 RED-GREEN-REFACTOR 사이클

### 1.2 선행 조건

SPEC-AGG-001의 다중 필드/다중 함수 집계 기능이 완전히 구현되어 있어야 한다. 본 SPEC은 SPEC-AGG-001의 구조체 및 메서드를 기반으로 확장한다.

### 1.3 변경 영향 범위

| 파일 | 변경 유형 | 영향도 |
|------|-----------|--------|
| `internal/node/aggregate.go` | 핵심 수정 | 높음 - 구조체 확장, Configure/Process/Shutdown 변경, Sliding Window |
| `internal/node/aggregate_test.go` | 테스트 확장 | 높음 - 기존 테스트 회귀 확인 + 신규 그룹 테스트 추가 |
| `internal/node/errors.go` | 에러 추가 | 낮음 - 새로운 에러 상수 추가 |
| `examples/flows/mqtt-metrics.yaml` | 예제 업데이트 | 낮음 - group_by 설정 예시 추가 |

## 2. 마일스톤

### 마일스톤 1: group_by Configure + 그룹별 버퍼 구조 (Primary Goal)

기존 동작 보존 확인 및 group_by 파싱/그룹 버퍼 구조 구현

**작업 목록:**

- **T1.1**: 기존 테스트 전체 실행 (`go test -race ./internal/node/...`) 및 베이스라인 확보
  - SPEC-AGG-001 기반 모든 테스트가 PASS 상태인지 확인
  - 커버리지 베이스라인 기록

- **T1.2**: `AggregateNode` 구조체 필드 확장
  - `groupByKeys []string` 필드 추가 (단일 키: 길이 1, 복합 키: 길이 2+, 미설정: nil)
  - `maxGroups int` 필드 추가
  - `groupBuffers map[string][]message.Message` 필드 추가

- **T1.3**: 에러 정의 추가 (`internal/node/errors.go`)
  - `ErrAggregateGroupByInvalid` 에러 변수 추가
  - `ErrAggregateMaxGroupsInvalid` 에러 변수 추가

- **T1.4**: `Configure` 메서드 확장
  - `group_by` 키 파싱: 문자열 → `groupByKeys = []string{value}` (단일 키)
  - `group_by` 키 파싱: `[]any` → `groupByKeys = []string{...}` (복합 키)
  - `group_by` 빈 문자열 또는 빈 배열 → 에러 반환
  - `max_groups` 키 파싱: 양의 정수 저장, 기본값 100, 0 이하 에러 반환
  - `group_by` 설정 시 `groupBuffers` 맵 초기화
  - REQ-AGG-100, REQ-AGG-100a, REQ-AGG-101, REQ-AGG-102, REQ-AGG-140, REQ-AGG-142 충족

- **T1.5**: Configure 확장 테스트 작성
  - group_by 단일 문자열 파싱 테스트
  - group_by 문자열 배열(복합 키) 파싱 테스트
  - group_by 빈 배열 에러 테스트
  - group_by 미설정 시 하위 호환 테스트
  - group_by 빈 문자열 에러 테스트
  - max_groups 양의 정수 파싱 테스트
  - max_groups 기본값(100) 테스트
  - max_groups 0 이하 에러 테스트
  - 기존 SPEC-AGG-001 Configure 테스트 회귀 확인

**완료 기준:** Configure 관련 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 2: 그룹별 집계 로직 - count + time 윈도우 (Primary Goal)

그룹별 독립 버퍼 관리 및 count/time 윈도우 플러시 구현

**작업 목록:**

- **T2.1**: `Process` 메서드 그룹 분기 로직 구현
  - `groupByKeys == nil` -> 기존 단일 버퍼 로직 (변경 없음)
  - `groupByKeys != nil` -> `extractGroupKey(msg)` 호출 후 그룹 버퍼 할당
  - `extractGroupKey` 헬퍼 구현:
    - 단일 키: `msg.Payload().Get(groupByKeys[0])` → 문자열 변환
    - 복합 키: 각 필드 값 추출 → `|` 구분자로 결합 (nil 필드 → `"_unknown"`)
  - REQ-AGG-110, REQ-AGG-100b 충족

- **T2.2**: count 윈도우 그룹별 플러시 구현
  - 그룹 버퍼 크기가 `windowSize`에 도달 시 해당 그룹만 플러시
  - `flushGroup(groupKey, buf)` 헬퍼 함수 구현
  - REQ-AGG-111, REQ-AGG-121 충족

- **T2.3**: time 윈도우 그룹별 플러시 구현
  - 타이머 만료 시 `flushAllGroups()` 호출
  - 비어있지 않은 그룹만 플러시
  - 각 그룹별 독립 출력 메시지 생성
  - REQ-AGG-112, REQ-AGG-122 충족

- **T2.4**: 그룹별 집계 실행 (`flushGroup`)
  - 기존 `executeAggregate(buf)` 호출 (SPEC-AGG-001 로직 재사용)
  - 단일 키: `group_key` + `group_value` 추가
  - 복합 키: `group_keys` + `group_value` + `group_values` (필드명→값 맵) 추가
  - 메타데이터에 `_group_keys`, `_group_value` 추가
  - REQ-AGG-120, REQ-AGG-100c, REQ-AGG-130, REQ-AGG-131 충족

- **T2.5**: count/time 윈도우 그룹 테스트 작성
  - count 윈도우: 2개 그룹 독립 플러시 검증 (단일 키)
  - count 윈도우: 그룹 A 플러시 시 그룹 B 미영향 검증
  - count 윈도우: 복합 키 2개 필드 그룹 플러시 검증
  - time 윈도우: 다중 그룹 동시 플러시 검증
  - time 윈도우: 빈 그룹 스킵 검증
  - 단일 키 출력 구조 (group_key, group_value, stats) 검증
  - 복합 키 출력 구조 (group_keys, group_value, group_values, stats) 검증
  - 그룹 메타데이터 (_group_keys, _group_value) 검증

**완료 기준:** 그룹별 count/time 윈도우 테스트 전체 PASS

### 마일스톤 3: 출력 형식 + 그룹화된 stats 매트릭스 (Primary Goal)

그룹별 다중 필드/함수 매트릭스 출력 및 하위 호환성

**작업 목록:**

- **T3.1**: 그룹 모드 출력 형식 구현
  - 각 그룹 출력 메시지에 `stats` 매트릭스 포함 (SPEC-AGG-001 형식)
  - `group_key` + `group_value` + `stats` + `window_count` + `window_type` + `window_size`
  - REQ-AGG-130 충족

- **T3.2**: 비그룹 모드 하위 호환 검증
  - `group_by` 미설정 시 기존 출력 형식 유지
  - 단일 필드/단일 함수 레거시 출력 (`result` + `count`) 유지
  - REQ-AGG-132 충족

- **T3.3**: group_by + 다중 필드 + 다중 함수 통합 테스트
  - 단일 키: `group_by: "location"`, `fields: ["temperature", "humidity"]`, `aggregate_fn: ["avg", "min", "max"]`
  - 복합 키: `group_by: ["location", "device_id"]`, 동일 필드/함수 조합
  - 그룹별 stats 매트릭스 검증 (2그룹 x 2필드 x 3함수)
  - 복합 키 `|` 결합 키 생성 및 `group_values` 맵 검증
  - REQ-AGG-100b, REQ-AGG-100c, REQ-AGG-180 충족

- **T3.4**: 출력 형식 테스트
  - 그룹 모드: JSON 구조 검증 (group_key, group_value, stats)
  - 비그룹 모드: SPEC-AGG-001 형식 유지 검증
  - 메타데이터 확장 검증

**완료 기준:** 출력 형식 테스트 전체 PASS + SPEC-AGG-001 회귀 없음

### 마일스톤 4: 하위 호환 + 엣지 케이스 (Secondary Goal)

max_groups, _unknown 그룹, 타입 변환, Shutdown 처리

**작업 목록:**

- **T4.1**: max_groups 제한 구현
  - 활성 그룹 수가 `maxGroups` 도달 시 신규 그룹 메시지 드롭
  - 드롭 시 로그 출력 (Warning 레벨)
  - 기존 그룹 메시지는 정상 처리
  - REQ-AGG-141 충족

- **T4.2**: 그룹 키 누락 -> `_unknown` 그룹 처리
  - `msg.Payload().Get(groupBy)` 반환값이 nil -> `"_unknown"` 그룹
  - REQ-AGG-150 충족

- **T4.3**: 그룹 키 타입 변환
  - `float64`, `int`, `bool` 등 비문자열 타입 -> `fmt.Sprintf("%v", value)` 변환
  - REQ-AGG-151 충족

- **T4.4**: 플러시 후 그룹 버퍼 초기화
  - 플러시 완료 후 `groupBuffers[key] = groupBuffers[key][:0]` (슬라이스 재사용)
  - 그룹 맵에서 키 제거하지 않음
  - REQ-AGG-161 충족

- **T4.5**: Shutdown 시 잔여 그룹 버퍼 플러시
  - `Shutdown()` 호출 시 비어있지 않은 모든 그룹 버퍼 플러시
  - 결과를 `lastFlushResult`에 저장
  - REQ-AGG-181 충족

- **T4.6**: 동시성 안전 테스트 강화
  - 그룹 모드에서 다수 고루틴 동시 Process 호출
  - `go test -race` 플래그 검증
  - REQ-AGG-170 충족

- **T4.7**: 엣지 케이스 테스트 작성
  - max_groups 초과 메시지 드롭 검증
  - 단일 키 _unknown 그룹 할당 검증
  - 복합 키 부분 누락 검증 (location 있음 + device_id nil → "factory-A|_unknown")
  - 복합 키 전체 누락 검증 (→ "_unknown|_unknown")
  - 그룹 키 타입 변환 검증 (float64 -> "25.5", int -> "42", bool -> "true")
  - 플러시 후 버퍼 초기화 및 재사용 검증
  - Shutdown 잔여 그룹 플러시 검증
  - 그룹 모드 동시성 race 검증

**완료 기준:** 엣지 케이스 + race 테스트 전체 PASS

### 마일스톤 5: mqtt-metrics.yaml 업데이트 + 통합 테스트 (Final Goal)

예제 플로우 업데이트 및 최종 검증

**작업 목록:**

- **T5.1**: `examples/flows/mqtt-metrics.yaml` 업데이트
  - fan-out 패턴: 3개 시간 윈도우(10분/1시간/3시간) 동시 실행
  - 각 윈도우에 `group_by: "location"` 설정 (단일 키)
  - 복합 키 예시는 주석으로 안내 (`group_by: ["location", "device_id"]`)
  - 주석에 그룹화된 출력 형식 예시 업데이트
  - 기존 설정과의 하위 호환성 확인

- **T5.2**: 전체 테스트 스위트 실행
  - `go test -race -cover ./internal/node/...`
  - 커버리지 85% 이상 확인
  - `go vet ./...` 통과 확인
  - 기존 SPEC-AGG-001 테스트 전체 회귀 없음

- **T5.3**: 통합 시나리오 확인
  - mqtt-metrics.yaml 플로우가 group_by 설정으로 올바르게 로드되는지 확인
  - Configure -> Init -> Process -> Shutdown 전체 라이프사이클 테스트 (그룹 모드)
  - group_by + 다중 필드 + 다중 함수 + count/time/sliding 윈도우 전체 조합 검증

**완료 기준:** 전체 테스트 PASS + 커버리지 85%+ + 예제 업데이트

### 마일스톤 6: Sliding Window 기본 구현 (Primary Goal)

슬라이딩 윈도우 비그룹/그룹 모드 구현

**작업 목록:**

- **T6.1**: `timestampedMessage` 내부 구조체 추가
  - `msg message.Message` + `receivedAt time.Time` 필드
  - `tsBuffer []timestampedMessage` 필드 (비그룹 모드)
  - `groupTsBuffers map[string][]timestampedMessage` 필드 (그룹 모드)
  - `slideDuration time.Duration` 필드

- **T6.2**: Configure에 `sliding` 윈도우 타입 파싱 추가
  - `window_type: "sliding"` 인식
  - `slide_interval` duration 문자열 파싱 (`time.ParseDuration`)
  - 미설정 시 기본값: `windowDuration / 10`
  - `slideDuration > windowDuration` → 에러 반환 (`ErrAggregateSlideIntervalInvalid`)
  - 에러 정의 추가 (`errors.go`)
  - REQ-AGG-190, REQ-AGG-191 충족

- **T6.3**: Process에 sliding 모드 분기 추가
  - 비그룹 모드: `tsBuffer`에 `{msg, time.Now()}` 추가
  - 그룹 모드: `extractGroupKey` → `groupTsBuffers[key]`에 추가
  - REQ-AGG-192 충족

- **T6.4**: `slidingFlush` 구현 (비그룹 모드)
  - `cutoff := time.Now().Add(-windowDuration)` 계산
  - `tsBuffer`에서 `receivedAt < cutoff` 메시지 evict
  - 남은 메시지로 `executeAggregate` 실행
  - 출력에 `window_start`, `window_end` (RFC3339) 추가
  - `slide_interval` 후 타이머 재시작 (반복)
  - REQ-AGG-193, REQ-AGG-195 충족

- **T6.5**: `slidingFlushAllGroups` 구현 (그룹 모드)
  - 모든 그룹 순회, 각 그룹 타임스탬프 버퍼 eviction
  - 유효 메시지 0건 그룹 스킵
  - 유효 메시지로 `flushGroup(groupKey, msgs)` 실행
  - 출력에 `window_start`, `window_end` 추가
  - REQ-AGG-194 충족

- **T6.6**: Sliding Window 테스트 작성
  - Configure: `window_type: "sliding"` + `slide_interval` 파싱 테스트
  - Configure: `slide_interval` 미설정 시 기본값 테스트
  - Configure: `slide_interval > window_size` 에러 테스트
  - Process: 메시지 타임스탬프 기록 검증
  - 비그룹 모드: eviction 후 윈도우 범위 내 메시지만 집계 검증
  - 그룹 모드: 그룹별 독립 슬라이딩 윈도우 검증
  - 출력: `window_start`, `window_end` 포함 검증
  - 빈 윈도우(메시지 전부 evict) 시 출력 없음 검증

**완료 기준:** Sliding Window 비그룹/그룹 모드 테스트 전체 PASS

### 마일스톤 7: Sliding Window 통합 + Shutdown + 최종 검증 (Secondary Goal)

Shutdown 처리, 동시성, mqtt-metrics.yaml 슬라이딩 예시

**작업 목록:**

- **T7.1**: Shutdown 시 sliding 버퍼 처리
  - 비그룹 모드: `tsBuffer`의 남은 메시지로 마지막 집계 실행
  - 그룹 모드: 모든 그룹의 `groupTsBuffers` 남은 메시지 집계
  - 타이머 취소
  - REQ-AGG-181 확장 충족

- **T7.2**: Sliding Window 동시성 테스트
  - 다수 고루틴 동시 Process + 타이머 만료 경쟁 검증
  - `go test -race` 플래그 검증
  - REQ-AGG-170 확장 충족

- **T7.3**: Tumbling + Sliding 호환성 테스트
  - `window_type: "count"` → 기존 동작 그대로 (회귀 없음)
  - `window_type: "time"` → 기존 tumbling 동작 그대로 (회귀 없음)
  - `window_type: "sliding"` → 신규 sliding 동작 검증

- **T7.4**: mqtt-metrics.yaml에 sliding 예시 추가
  - 기존 tumbling 10min/1hr/3hr 설정과 함께 sliding 변형 예시 주석 포함
  - `slide_interval` 설정 안내

- **T7.5**: 전체 회귀 테스트
  - `go test -race -cover ./internal/node/...`
  - 커버리지 85% 이상 확인
  - SPEC-AGG-001 기존 테스트 전체 회귀 없음

**완료 기준:** 전체 테스트 PASS + 커버리지 85%+ + Sliding Window 통합 완료

## 3. 기술 접근 방식

### 3.1 그룹 키 추출 전략

메시지에서 그룹 키를 추출하는 로직:

```
extractGroupKey(msg) -> string:
  if len(groupByKeys) == 1:
    // 단일 키 모드
    rawValue := msg.Payload().Get(groupByKeys[0])
    return toStringKey(rawValue)
  else:
    // 복합 키 모드
    parts := make([]string, len(groupByKeys))
    for i, key := range groupByKeys:
      rawValue := msg.Payload().Get(key)
      parts[i] = toStringKey(rawValue)
    return strings.Join(parts, "|")

toStringKey(value) -> string:
  switch v := value.(type):
    case nil:     -> "_unknown"
    case string:  -> v
    case float64: -> fmt.Sprintf("%v", v)
    case int:     -> fmt.Sprintf("%v", v)
    case bool:    -> fmt.Sprintf("%v", v)
    default:      -> fmt.Sprintf("%v", v)
```

### 3.2 Process 분기 전략

`Process` 메서드에서 단일/그룹 모드를 분기하는 기준:

```
isGroupMode := len(n.groupByKeys) > 0
```

그룹 모드에서는 `n.buffer` 대신 `n.groupBuffers[groupKey]`를 사용한다. 단일 모드에서는 기존 `n.buffer` 로직을 변경 없이 유지하여 SPEC-AGG-001 하위 호환성을 보장한다.

### 3.3 flushGroup 재사용 전략

그룹 플러시 시 기존 `executeAggregate(buf)` 로직을 그대로 재사용한다:

```
flushGroup(groupKey, buf):
  results := n.executeAggregate(buf)  // SPEC-AGG-001 로직 재사용
  for _, result := range results:
    if len(n.groupByKeys) == 1:
      // 단일 키 모드
      result.Payload().Set("group_key", n.groupByKeys[0])
      result.Payload().Set("group_value", groupKey)
    else:
      // 복합 키 모드
      result.Payload().Set("group_keys", n.groupByKeys)
      result.Payload().Set("group_value", groupKey)  // "val1|val2" 결합 문자열
      result.Payload().Set("group_values", parseGroupValues(n.groupByKeys, groupKey))
    result.Metadata().Set("_group_keys", strings.Join(n.groupByKeys, ","))
    result.Metadata().Set("_group_value", groupKey)
  return results

parseGroupValues(keys, compositeKey) -> map[string]string:
  parts := strings.Split(compositeKey, "|")
  result := map[string]string{}
  for i, key := range keys:
    result[key] = parts[i]
  return result
```

### 3.4 time 윈도우 그룹 플러시 전략

기존 타이머 콜백에서 단일 버퍼 플러시를 그룹별 플러시로 확장:

```
기존 (SPEC-AGG-001):
  time.AfterFunc(windowDur, func() {
    mu.Lock(); defer mu.Unlock()
    if len(buffer) > 0:
      results = executeAggregate(buffer)
      buffer = buffer[:0]
      lastFlushResult = results
  })

확장 (SPEC-AGG-002):
  time.AfterFunc(windowDur, func() {
    mu.Lock(); defer mu.Unlock()
    if len(groupByKeys) == 0:
      // 기존 단일 버퍼 로직 유지
    else:
      allResults = []message.Message{}
      for key, buf := range groupBuffers:
        if len(buf) > 0:
          results = flushGroup(key, buf)
          allResults = append(allResults, results...)
          groupBuffers[key] = buf[:0]
      lastFlushResult = allResults
  })
```

### 3.5 Sliding Window 타이머 전략

슬라이딩 윈도우는 `slide_interval` 주기로 반복 타이머를 실행한다:

```
Configure 시:
  if windowType == "sliding":
    slideDuration = parseDuration(slideIntervalStr)  // 또는 windowDuration/10
    time.AfterFunc(slideDuration, func() {
      mu.Lock(); defer mu.Unlock()
      if isGroupMode:
        slidingFlushAllGroups()
      else:
        slidingFlush()
      // 타이머 재시작 (반복)
      resetSlidingTimer()
    })

slidingFlush() (비그룹):
  now := time.Now()
  cutoff := now.Add(-windowDuration)
  // evict old messages
  validIdx := 0
  for _, ts := range tsBuffer:
    if ts.receivedAt.Before(cutoff): continue
    tsBuffer[validIdx] = ts; validIdx++
  tsBuffer = tsBuffer[:validIdx]
  if len(tsBuffer) == 0: return  // 빈 윈도우 스킵
  msgs := extractMessages(tsBuffer)
  results := executeAggregate(msgs)
  // window_start/window_end 추가
  for _, r := range results:
    r.Payload().Set("window_start", cutoff.Format(time.RFC3339))
    r.Payload().Set("window_end", now.Format(time.RFC3339))
  lastFlushResult = results
```

### 3.6 메모리 관리 전략

- 그룹 버퍼는 `map[string][]message.Message`로 관리
- 플러시 후 슬라이스 `[:0]` 초기화로 메모리 재사용 (맵 키 제거 안 함)
- `max_groups`로 무한 그룹 생성 방지
- 그룹 수가 적은 IoT 시나리오(location, device_id 등)에 최적화
- Sliding Window: 오래된 메시지 eviction으로 메모리 누수 방지
- Sliding Window: `tsBuffer`도 슬라이스 재사용 (`[:validIdx]` 패턴)

## 4. 아키텍처 설계 방향

### 4.1 구조체 설계

```
AggregateNode 구조체 확장:
  - groupByKeys   []string                          (신규: 그룹 분류 기준 필드명 목록, nil=비그룹)
  - maxGroups     int                               (신규: 최대 허용 그룹 수)
  - groupBuffers  map[string][]message.Message       (신규: 그룹별 독립 버퍼)
  - slideDuration  time.Duration                    (신규: slide_interval 파싱 결과)
  - tsBuffer       []timestampedMessage              (신규: sliding 비그룹 타임스탬프 버퍼)
  - groupTsBuffers map[string][]timestampedMessage    (신규: sliding 그룹별 타임스탬프 버퍼)
  - (기존 필드 모두 유지: buffer, aggregateFns, fields, windowType, 등)

timestampedMessage 구조체:
  - msg        message.Message
  - receivedAt time.Time
```

### 4.2 메서드 시그니처 변경

| 메서드 | 변경 사항 |
|--------|-----------|
| `Configure` | `group_by` (string/[]string), `max_groups` 파싱 추가 |
| `Process` | 그룹 모드 분기 + 그룹 키 추출 + 그룹 버퍼 할당 |
| `Shutdown` | 잔여 그룹 버퍼 전체 플러시 추가 |
| (신규) `extractGroupKey` | 메시지에서 그룹 키 추출 (단일/복합) |
| (신규) `toStringKey` | 단일 필드 값 → 문자열 변환 헬퍼 |
| (신규) `flushGroup` | 단일 그룹 플러시 + 메타데이터 추가 (단일/복합 키 분기) |
| (신규) `flushAllGroups` | time 윈도우 전체 그룹 플러시 |
| (신규) `parseGroupValues` | 복합 키 결합 문자열 → 필드명:값 맵 분해 |
| (신규) `slidingFlush` | 비그룹 sliding 윈도우 집계 (eviction + aggregate) |
| (신규) `slidingFlushAllGroups` | 그룹 모드 sliding 윈도우 전체 집계 |
| (신규) `extractMessages` | timestampedMessage 슬라이스에서 message.Message 슬라이스 추출 |
| (신규) `resetSlidingTimer` | slide_interval 반복 타이머 재시작 |

### 4.3 데이터 흐름

```
[메시지 입력]
     |
     v
  groupByKeys 설정?
   /        \
 nil         != nil
  |            |
  v            v
단일 버퍼   extractGroupKey
(기존 로직) (단일/복합 키)
  |            |
  |            v
  |       groupBuffers[key]에 추가
  |            |
  v            v
windowSize    그룹별 windowSize 도달?
도달?          /          \
  |         Yes            No
  v           |             |
플러시     flushGroup     대기
  |           |
  v           v
출력 메시지  출력 메시지 (+ group_key, group_value)
```

## 5. 리스크 및 대응 방안

| 리스크 | 가능성 | 영향도 | 대응 방안 |
|--------|--------|--------|-----------|
| SPEC-AGG-001 미구현 상태에서 시작 | 높음 | 높음 | 마일스톤 1의 T1.1에서 SPEC-AGG-001 기반 테스트 통과 여부 선확인 |
| 그룹 수 폭발로 메모리 증가 | 중간 | 높음 | `max_groups` 기본값 100으로 제한, 초과 시 메시지 드롭 + 로그 |
| time 윈도우 그룹 플러시 경쟁 조건 | 중간 | 높음 | 기존 `sync.Mutex` 패턴 유지, `flushAllGroups` 내부에서 lock 보유 상태 확인, `-race` 플래그 검증 |
| 그룹 키 타입 불일치 | 낮음 | 중간 | `fmt.Sprintf("%v", value)` 일괄 변환으로 타입 무관하게 문자열 키 생성 |
| 기존 SPEC-AGG-001 테스트 회귀 | 중간 | 높음 | `groupBy == ""` 시 기존 로직 100% 유지, 모든 마일스톤에서 회귀 테스트 실행 |
| Shutdown 시 그룹별 플러시 순서 비결정적 | 낮음 | 낮음 | Go map 순회 순서는 비결정적이나, 각 그룹 결과가 독립적이므로 순서 무관 |
| Sliding Window 메모리 누수 | 중간 | 높음 | eviction 로직으로 윈도우 범위 밖 메시지 자동 제거, `max_groups`와 결합하여 제한 |
| Sliding 타이머 + Process 동시 접근 | 중간 | 높음 | 기존 `sync.Mutex` 패턴 유지, 타이머 콜백 내부에서 lock 획득 후 eviction/집계 수행 |
| slide_interval 매우 작은 값 설정 | 낮음 | 중간 | 최소값 제한 없이 경고 로그 출력, 사용자 책임 (향후 최소값 제한 검토) |

## 6. 다음 단계

구현 완료 후:
- `/moai:2-run SPEC-AGG-002` 명령으로 DDD/Hybrid 구현 시작
- 관련 SPEC-INFLUX-001과의 통합 시 그룹별 stats를 InfluxDB write point의 태그(tag)로 변환하는 연동 검토
- `/moai:3-sync SPEC-AGG-002` 명령으로 문서 동기화
