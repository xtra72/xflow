# SPEC-MSG-001: Message System - 인수 테스트 기준

> TAG: SPEC-MSG-001
> Status: Planned
> Created: 2026-02-12

---

## AC-001: Message 생성 및 인터페이스 준수

**Given** `pkg/message` 패키지가 임포트된 상태에서

**When** `message.New()` 함수를 호출하면

**Then** Message 인터페이스를 만족하는 객체가 반환되어야 한다

- `ID()`는 UUID v4 형식의 고유 문자열을 반환한다
- `Timestamp()`는 생성 시점의 `time.Time`을 반환한다
- `Payload()`는 빈 Payload 인터페이스를 반환한다
- `Metadata()`는 빈 Metadata 인터페이스를 반환한다
- `History()`는 `nil`을 반환한다 (기본 비활성화)
- `HistoryEnabled()`는 `false`를 반환한다

**검증 방법:**
- `ID()` 반환값이 정규식 `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`에 매칭되는지 확인
- 두 번 호출 시 서로 다른 ID가 생성되는지 확인
- `Timestamp()`가 호출 전후 시간 범위 내에 있는지 확인

---

## AC-002: Options 패턴을 통한 Message 생성

**Given** 다양한 Option이 준비된 상태에서

**When** `message.New(WithHistory(true), WithMaxHistory(50), WithMetadata("_source", "agent-1"))` 호출 시

**Then** 이력 추적이 활성화되고, 최대 이력 50건, 메타데이터에 `"_source"="agent-1"`이 설정된 Message가 반환되어야 한다

- `HistoryEnabled()`는 `true`를 반환한다
- `History()`는 빈 `[]ChangeRecord` 슬라이스를 반환한다 (`nil`이 아님)
- `Metadata().Get("_source")`는 `("agent-1", true)`를 반환한다
- 이력 최대 건수는 50으로 설정된다

**검증 방법:**
- 각 Option이 독립적으로 동작하는지 개별 테스트
- 복수 Option 조합 시 상호 간섭 없이 적용되는지 확인

---

## AC-003: Payload CRUD 연산

**Given** 빈 Payload가 생성된 상태에서

**When** `Add("temperature", 25.5)`, `Set("humidity", 60)`, `Get("temperature")`, `Delete("humidity")`, `Keys()` 순서로 호출하면

**Then**

| 연산 | 기대 결과 |
|------|----------|
| `Add("temperature", 25.5)` | 성공 (에러 없음) |
| `Set("humidity", 60)` | 성공 |
| `Get("temperature")` | `(25.5, true)` 반환 |
| `Delete("humidity")` | `humidity` 키 삭제 |
| `Keys()` | `["temperature"]` 반환 |

**추가 검증:**
- `Get("nonexistent")`는 `(nil, false)`를 반환한다
- `Delete("nonexistent")`는 에러 없이 무시된다
- `Keys()` 반환값은 안정적인 정렬 순서를 보장한다
- `Len()`은 현재 키 개수를 정확히 반환한다

---

## AC-004: Payload Add 중복 키 에러

**Given** Payload에 `"key1"` 키가 이미 존재하는 상태에서

**When** `Add("key1", "new_value")`를 호출하면

**Then** `ErrKeyExists` 에러를 반환하고, 기존 값은 변경되지 않아야 한다

**검증 방법:**
- `errors.Is(err, ErrKeyExists)`가 `true`인지 확인
- `Get("key1")`이 원래 값을 반환하는지 확인
- `Set("key1", "new_value")`는 정상 동작하여 값이 덮어쓰기되는지 확인 (Add와의 차이)

---

## AC-005: Payload GetPath JSONPath 접근

**Given** 중첩 데이터가 Payload에 설정된 상태에서

```json
{
  "sensors": [
    {"name": "temp", "value": 25.5},
    {"name": "humid", "value": 60}
  ]
}
```

**When** `GetPath("$.sensors[0].value")`를 호출하면

**Then** `25.5`를 반환해야 한다

---

**When** `GetPath("$.sensors[*].name")`를 호출하면

**Then** `["temp", "humid"]`를 반환해야 한다

---

**When** `GetPath("$.invalid.path")`를 호출하면

**Then** 에러를 반환해야 한다

**추가 검증:**
- 빈 Payload에서 `GetPath` 호출 시 에러를 반환한다
- 와일드카드 경로 `$.*`로 최상위 키 접근이 가능하다
- 배열 인덱스 범위 초과 시 에러를 반환한다

---

## AC-006: Metadata CRUD 연산

**Given** 빈 Metadata가 생성된 상태에서

**When** `Set("_source", "agent-1")`, `Get("_source")`, `Has("_source")`, `Remove("_source")`, `All()` 순서로 호출하면

**Then**

| 연산 | 기대 결과 |
|------|----------|
| `Set("_source", "agent-1")` | 성공 |
| `Get("_source")` | `("agent-1", true)` 반환 |
| `Has("_source")` | `true` 반환 |
| `Remove("_source")` | `_source` 키 삭제 |
| `All()` | 빈 `map[string]string` 반환 |

**추가 검증:**
- `Get("nonexistent")`는 `("", false)`를 반환한다
- `Has("nonexistent")`는 `false`를 반환한다
- `Remove("nonexistent")`는 에러 없이 무시된다
- `All()` 반환 맵을 수정해도 원본 Metadata는 영향받지 않는다 (deep copy)

---

## AC-007: 변경 이력 추적 - Payload 변경

**Given** `WithHistory(true)`로 생성된 Message에서

**When** `Payload().Add("key1", "value1")`, `Payload().Set("key1", "value2")`, `Payload().Delete("key1")` 순서로 호출하면

**Then** `History()`는 3개의 `ChangeRecord`를 반환해야 한다

| 순서 | Target | Operation | Key | OldValue | NewValue |
|------|--------|-----------|-----|----------|----------|
| 1 | `"payload"` | `"add"` | `"key1"` | `nil` | `"value1"` |
| 2 | `"payload"` | `"set"` | `"key1"` | `"value1"` | `"value2"` |
| 3 | `"payload"` | `"delete"` | `"key1"` | `"value2"` | `nil` |

**추가 검증:**
- 각 `ChangeRecord`의 `Timestamp`는 순서대로 증가한다
- `ChangeRecord` 슬라이스는 불변 복사본으로 반환된다 (외부 수정 불가)

---

## AC-008: 변경 이력 추적 - Metadata 변경

**Given** `WithHistory(true)`로 생성된 Message에서

**When** `Metadata().Set("_source", "node-1")`, `Metadata().Remove("_source")` 순서로 호출하면

**Then** `History()`에 2개의 `ChangeRecord`가 추가되어야 한다

| 순서 | Target | Operation | Key | OldValue | NewValue |
|------|--------|-----------|-----|----------|----------|
| 1 | `"metadata"` | `"set"` | `"_source"` | `""` | `"node-1"` |
| 2 | `"metadata"` | `"delete"` | `"_source"` | `"node-1"` | `""` |

**추가 검증:**
- Payload와 Metadata 이력이 동일한 `History()` 슬라이스에 시간순으로 혼합 기록된다
- `Target` 필드로 Payload/Metadata 변경을 구분할 수 있다

---

## AC-009: 이력 FIFO 삭제

**Given** `WithHistory(true)`, `WithMaxHistory(3)`으로 생성된 Message에서

**When** Payload에 4번의 Set 연산을 수행하면

**Then** `History()`는 최신 3개의 `ChangeRecord`만 반환해야 한다 (가장 오래된 1건 삭제)

**검증 방법:**
- 첫 번째 Set의 `ChangeRecord`는 이력에서 제거된다
- 두 번째, 세 번째, 네 번째 Set의 `ChangeRecord`가 순서대로 존재한다
- `len(History())`는 항상 `maxHistory` 이하이다
- 5번째 Set 수행 시 두 번째 Set의 `ChangeRecord`도 제거된다

---

## AC-010: 이력 비활성화 시 zero overhead

**Given** 기본 옵션(이력 비활성화)으로 생성된 Message에서

**When** `Payload().Set("key", "value")`를 호출하면

**Then**
- `History()`는 `nil`을 반환한다
- `HistoryEnabled()`는 `false`를 반환한다

**검증 방법:**
- 이력 비활성화 상태에서 Payload/Metadata 연산이 추가 메모리를 할당하지 않는다
- 벤치마크로 이력 활성화/비활성화 간 성능 차이를 측정한다
- 이력 비활성화 Message의 Payload/Metadata는 래핑 없이 직접 구현체를 사용한다

---

## AC-011: Message Clone 독립성

**Given** Payload와 Metadata에 데이터가 설정된 Message에서

**When** `Clone()`을 호출하여 복사본을 생성하고, 복사본의 Payload/Metadata를 수정하면

**Then**
- 복사본의 `ID()`는 원본과 다른 새 UUID여야 한다
- 복사본의 `Timestamp()`는 원본과 동일해야 한다
- 원본의 Payload/Metadata는 변경되지 않아야 한다
- 이력 추적 설정도 원본과 동일하게 유지되어야 한다

**검증 방법:**
- 복사본에서 `Payload().Set("new_key", "new_value")` 호출 후 원본에서 `Get("new_key")`이 `(nil, false)` 반환
- 복사본에서 `Metadata().Set("_tag", "test")` 호출 후 원본에서 `Has("_tag")`이 `false` 반환
- 중첩 데이터(map, slice)도 deep copy되는지 확인
- 이력 활성화된 Message의 Clone에서 기존 이력은 복사되지 않는다 (새 이력 시작)

---

## AC-012: JSON 직렬화/역직렬화

**Given** Payload와 Metadata에 데이터가 설정되고 이력이 기록된 Message에서

**When** JSON으로 직렬화(`Marshal`)한 후 `FromJSON()`으로 역직렬화하면

**Then**
- `ID`, `Timestamp`, Payload 데이터, Metadata 데이터가 복원되어야 한다
- History 레코드가 복원되어야 한다 (이력 활성화된 경우)
- 역직렬화된 Message는 기본 구현(`defaultMessage`)이어야 한다

**검증 방법:**
- 원본과 역직렬화된 Message의 `ID()`, `Timestamp()` 비교
- 원본과 역직렬화된 Message의 `Payload().Keys()` 비교
- 원본과 역직렬화된 Message의 `Metadata().All()` 비교
- 이력 활성화 상태에서 `History()` 레코드 수 비교
- 빈 Message의 직렬화/역직렬화가 정상 동작하는지 확인
- 잘못된 JSON 입력 시 적절한 에러가 반환되는지 확인

---

## AC-013: 커스텀 Payload 호환성

**Given** Payload 인터페이스를 구현한 커스텀 타입이 존재하는 상태에서

**When** `New(WithPayload(customPayload), WithHistory(true))`로 Message를 생성하면

**Then** 커스텀 Payload가 `historyPayload`로 래핑되어 이력 추적이 정상 동작해야 한다

**검증 방법:**
- 커스텀 Payload의 `Add`, `Set`, `Delete` 연산이 이력에 기록되는지 확인
- 커스텀 Payload의 원래 동작(데이터 저장/조회)이 래핑 후에도 유지되는지 확인
- 이력 비활성화 시 커스텀 Payload가 래핑 없이 직접 사용되는지 확인
- 커스텀 Payload가 `nil`인 경우 기본 Payload로 폴백하는지 확인

---

## AC-014: Payload ToMap Deep Copy

**Given** 중첩 데이터가 포함된 Payload에서

**When** `ToMap()`을 호출하고 반환된 맵을 수정하면

**Then** 원본 Payload의 데이터는 변경되지 않아야 한다

**검증 방법:**
- `ToMap()` 반환 맵에 새 키를 추가한 후 원본 `Keys()` 확인
- `ToMap()` 반환 맵의 기존 값을 수정한 후 원본 `Get()` 확인
- 중첩 맵/슬라이스를 수정한 후 원본 데이터 무결성 확인
- 빈 Payload에서 `ToMap()`이 빈 맵(`map[string]any{}`)을 반환하는지 확인

---

## 품질 게이트

| 항목 | 기준 | 검증 명령 |
|------|------|----------|
| 테스트 커버리지 | 85% 이상 | `go test -coverprofile=cover.out ./pkg/message/...` |
| 경쟁 조건 | go test -race 통과 | `go test -race ./pkg/message/...` |
| 벤치마크 | `New()`, `Clone()`, `Set()`, `GetPath()` 벤치마크 기록 | `go test -bench=. -benchmem ./pkg/message/...` |
| 린트 | go vet + golangci-lint 통과 | `go vet ./pkg/message/... && golangci-lint run ./pkg/message/...` |
| 인터페이스 호환 | 커스텀 구현 호환성 테스트 통과 | AC-013 테스트 케이스 |

---

## Definition of Done

- [ ] AC-001 ~ AC-014 모든 인수 테스트 통과
- [ ] 테스트 커버리지 85% 이상 달성
- [ ] `go test -race` 경쟁 조건 없음
- [ ] `go vet` 및 `golangci-lint` 경고 없음
- [ ] 벤치마크 결과 기록 완료
- [ ] 커스텀 Payload 구현 호환성 테스트 통과
- [ ] JSON 직렬화/역직렬화 왕복(round-trip) 테스트 통과
- [ ] 이력 FIFO 삭제 경계값 테스트 통과
- [ ] Clone deep copy 독립성 테스트 통과
