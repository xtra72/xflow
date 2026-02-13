---
id: SPEC-MSG-001
version: "1.0.0"
status: completed
created: "2026-02-12"
updated: "2026-02-14"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-12 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-14 | 1.0.0 | 구현 완료 (57 tests, 93.6% coverage, race clean, TRUST 5 PASS) |

---

# SPEC-MSG-001: Message System - 인터페이스 기반 구조적 메시지 데이터 처리 및 변경 이력 추적

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 엔진의 핵심 데이터 단위인 Message 시스템을 정의한다. Message는 Flow 내 노드 간 데이터 전달의 기본 단위로, Payload(데이터), Metadata(부가 정보), Change History(변경 이력)를 포함한다. 모든 외부 접근은 인터페이스를 통해 이루어지며, 구현체는 unexported로 캡슐화한다.

### 1.2 기술 환경

- **언어**: Go 1.22+
- **패키지 경로**: `pkg/message/`
- **의존성**: 표준 라이브러리만 사용 (uuid 생성을 위한 `crypto/rand`, `encoding/json`, `time` 등)
- **테스트 프레임워크**: Go 표준 `testing` 패키지
- **외부 의존성**: 없음 (Tier 1 - 독립 패키지)

### 1.3 설계 원칙

- **인터페이스 우선**: 모든 공개 API는 인터페이스로 정의
- **캡슐화**: 구현체(struct)는 unexported, 생성자만 exported
- **불변성 보장**: Clone 메서드를 통한 deep copy 제공
- **선택적 기능**: Options Pattern을 통한 유연한 설정
- **제로 오버헤드**: History 비활성화 시 래퍼 없이 직접 동작

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: UUID v4는 `crypto/rand` 기반으로 충분한 고유성을 보장한다
- A2: Message는 단일 goroutine에서 사용되며, 동시성 안전(thread-safety)은 이 SPEC의 범위 밖이다
- A3: Payload 값은 `encoding/json`으로 직렬화 가능한 타입만 허용한다
- A4: Metadata 값은 `string` 타입만 허용한다 (key-value 모두 string)
- A5: 중첩 구조의 deep copy는 `encoding/json` Marshal/Unmarshal을 통해 수행한다

### 2.2 도메인 가정

- A6: 하나의 Message는 하나의 Payload와 하나의 Metadata를 가진다
- A7: History는 Payload와 Metadata 모든 변경을 하나의 목록에 통합 추적한다
- A8: Clone된 Message는 새로운 UUID를 가지며, 원본과 완전히 독립적이다
- A9: History 활성화 여부는 Message 생성 시 결정되며, 이후 변경할 수 없다
- A10: WithMaxHistory 기본값은 100이며, FIFO 방식으로 오래된 기록을 제거한다

---

## 3. Requirements (요구사항)

### Module 1: Core - 인터페이스 정의 및 생성자

#### REQ-MSG-001-01-01 (Ubiquitous) Message 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Message` 인터페이스를 제공해야 한다:

- `ID() string` - 메시지 고유 식별자 반환
- `Timestamp() time.Time` - 메시지 생성 시각 반환
- `Payload() Payload` - Payload 인터페이스 반환
- `Metadata() Metadata` - Metadata 인터페이스 반환
- `History() []ChangeRecord` - 변경 이력 목록 반환
- `HistoryEnabled() bool` - History 활성화 여부 반환
- `Clone() Message` - 메시지 deep copy 반환

#### REQ-MSG-001-01-02 (Ubiquitous) Message 기본 구현체

시스템은 **항상** `defaultMessage`(unexported struct)를 `Message` 인터페이스의 기본 구현체로 사용해야 한다.

#### REQ-MSG-001-01-03 (Ubiquitous) Message 생성자

시스템은 **항상** `New(opts ...Option) Message` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `Message` 인터페이스이다
- ID는 UUID v4로 자동 생성한다
- Timestamp는 `time.Now()`로 자동 설정한다
- 옵션이 없으면 기본 빈 Payload와 빈 Metadata로 생성한다

#### REQ-MSG-001-01-04 (Ubiquitous) Payload 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Payload` 인터페이스를 제공해야 한다:

- `Add(key string, value any) error` - 키가 없을 때만 추가
- `Set(key string, value any)` - 키 존재 여부 무관하게 설정 (upsert)
- `Delete(key string)` - 키 삭제
- `Get(key string) (any, bool)` - 키로 값 조회
- `GetPath(jsonpath string) (any, error)` - dot-notation 경로로 값 조회
- `Keys() []string` - 모든 키 목록 반환
- `ToMap() map[string]any` - deep copy된 map 반환
- `ToJSON() ([]byte, error)` - JSON 직렬화
- `Clone() Payload` - deep copy 반환

#### REQ-MSG-001-01-05 (Ubiquitous) Payload 기본 구현체 및 생성자

시스템은 **항상** `mapPayload`(unexported, `map[string]any` 기반)를 기본 구현체로 사용하고, `NewPayload(data ...map[string]any) Payload` 생성자를 제공해야 한다.

- 인자가 없으면 빈 map으로 생성한다
- 인자가 있으면 첫 번째 map의 데이터를 복사하여 초기화한다

#### REQ-MSG-001-01-06 (Ubiquitous) Metadata 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Metadata` 인터페이스를 제공해야 한다:

- `Get(key string) (string, bool)` - 키로 값 조회
- `Set(key string, value string)` - 키-값 설정
- `Has(key string) bool` - 키 존재 여부 확인
- `Remove(key string)` - 키 삭제
- `All() map[string]string` - deep copy된 전체 map 반환
- `Clone() Metadata` - deep copy 반환

#### REQ-MSG-001-01-07 (Ubiquitous) Metadata 기본 구현체 및 생성자

시스템은 **항상** `mapMetadata`(unexported, `map[string]string` 기반)를 기본 구현체로 사용하고, `NewMetadata() Metadata` 생성자를 제공해야 한다.

#### REQ-MSG-001-01-08 (Ubiquitous) Options Pattern

시스템은 **항상** 다음 Option 함수들을 제공해야 한다:

- `type Option func(*config)` - 설정 함수 타입
- `WithHistory(enabled bool) Option` - History 활성화/비활성화
- `WithMaxHistory(n int) Option` - 최대 History 개수 설정 (기본값: 100)
- `WithMetadata(key, value string) Option` - 초기 Metadata 설정
- `WithPayload(p Payload) Option` - 커스텀 Payload 주입

#### REQ-MSG-001-01-09 (Ubiquitous) ChangeRecord 구조체

시스템은 **항상** 다음 필드를 가진 `ChangeRecord` 구조체(exported, 값 타입)를 제공해야 한다:

- `Target string` - 변경 대상 (`"payload"` 또는 `"metadata"`)
- `Operation string` - 연산 종류 (`"add"`, `"set"`, `"delete"`)
- `Key string` - 변경된 키
- `OldValue any` - 변경 전 값
- `NewValue any` - 변경 후 값
- `NodeID string` - 변경을 발생시킨 노드 ID
- `Timestamp time.Time` - 변경 시각

---

### Module 2: Payload Operations - 데이터 조작

#### REQ-MSG-001-02-01 (Event-Driven) Payload Add 연산

**WHEN** `Add(key, value)` 호출 시 해당 키가 이미 존재하면, **THEN** `ErrKeyExists` 에러를 반환해야 한다.

#### REQ-MSG-001-02-02 (Event-Driven) Payload Add 성공

**WHEN** `Add(key, value)` 호출 시 해당 키가 존재하지 않으면, **THEN** 키-값 쌍을 추가하고 `nil`을 반환해야 한다.

#### REQ-MSG-001-02-03 (Ubiquitous) Payload Set 연산

시스템은 **항상** `Set(key, value)` 호출 시 키 존재 여부와 관계없이 값을 설정(upsert)해야 한다.

#### REQ-MSG-001-02-04 (Ubiquitous) Payload Delete 연산

시스템은 **항상** `Delete(key)` 호출 시 키가 존재하지 않으면 아무 동작도 하지 않아야 한다 (no-op).

#### REQ-MSG-001-02-05 (Ubiquitous) Payload Get 연산

시스템은 **항상** `Get(key)` 호출 시 `(value, true)` 또는 `(nil, false)` 튜플을 반환해야 한다.

#### REQ-MSG-001-02-06 (Event-Driven) Payload GetPath 연산

**WHEN** `GetPath(jsonpath)` 호출 시, **THEN** dot-notation 및 배열 인덱스 접근을 지원해야 한다:

- `$.key.nested` - 중첩 객체 접근
- `$.array[0]` - 배열 인덱스 접근
- `$.array[*].field` - 배열 전체 요소의 특정 필드 접근

#### REQ-MSG-001-02-07 (Event-Driven) Payload GetPath 에러

**WHEN** `GetPath(jsonpath)` 호출 시 경로가 유효하지 않거나 존재하지 않으면, **THEN** 적절한 에러를 반환해야 한다.

#### REQ-MSG-001-02-08 (Ubiquitous) Payload Keys 연산

시스템은 **항상** `Keys()` 호출 시 현재 저장된 모든 최상위 키 목록을 반환해야 한다.

#### REQ-MSG-001-02-09 (Ubiquitous) Payload ToMap 연산

시스템은 **항상** `ToMap()` 호출 시 내부 데이터의 deep copy를 반환해야 한다. 반환된 map의 수정이 원본에 영향을 주지 않아야 한다.

#### REQ-MSG-001-02-10 (Ubiquitous) Payload ToJSON 연산

시스템은 **항상** `ToJSON()` 호출 시 내부 데이터를 JSON `[]byte`로 직렬화하여 반환해야 한다.

#### REQ-MSG-001-02-11 (Ubiquitous) Payload Clone 연산

시스템은 **항상** `Clone()` 호출 시 모든 키-값 쌍의 deep copy를 가진 새로운 `Payload`를 반환해야 한다.

#### REQ-MSG-001-02-12 (Optional) 커스텀 Payload 호환성

**가능하면** `WithPayload()` Option을 통해 사용자 정의 `Payload` 구현체를 Message에 주입할 수 있도록 제공해야 한다.

---

### Module 3: Metadata Management - 메타데이터 관리

#### REQ-MSG-001-03-01 (Ubiquitous) Metadata Get 연산

시스템은 **항상** `Get(key)` 호출 시 `(value, true)` 또는 `("", false)` 튜플을 반환해야 한다.

#### REQ-MSG-001-03-02 (Ubiquitous) Metadata Set 연산

시스템은 **항상** `Set(key, value)` 호출 시 키-값 쌍을 설정(upsert)해야 한다.

#### REQ-MSG-001-03-03 (Ubiquitous) Metadata Has 연산

시스템은 **항상** `Has(key)` 호출 시 키 존재 여부를 `bool`로 반환해야 한다.

#### REQ-MSG-001-03-04 (Ubiquitous) Metadata Remove 연산

시스템은 **항상** `Remove(key)` 호출 시 키가 존재하지 않으면 아무 동작도 하지 않아야 한다 (no-op).

#### REQ-MSG-001-03-05 (Ubiquitous) Metadata All 연산

시스템은 **항상** `All()` 호출 시 내부 데이터의 deep copy를 반환해야 한다. 반환된 map의 수정이 원본에 영향을 주지 않아야 한다.

#### REQ-MSG-001-03-06 (Ubiquitous) Metadata Clone 연산

시스템은 **항상** `Clone()` 호출 시 모든 키-값 쌍을 복사한 새로운 `Metadata`를 반환해야 한다.

#### REQ-MSG-001-03-07 (Ubiquitous) 시스템 Metadata 키 상수

시스템은 **항상** 다음 시스템 Metadata 키 상수를 제공해야 한다:

- `MetaKeySource = "_source"` - 메시지 출처
- `MetaKeyFlowID = "_flowID"` - Flow 식별자
- `MetaKeyNodeID = "_nodeID"` - 노드 식별자
- `MetaKeyTTL = "_ttl"` - Time-To-Live
- `MetaKeyCorrelationID = "_correlationID"` - 상관 관계 ID

#### REQ-MSG-001-03-08 (Unwanted) Metadata 비문자열 값 금지

시스템은 Metadata 값으로 `string` 이외의 타입을 **허용하지 않아야 한다**. 인터페이스 시그니처 자체가 `string`만 허용하도록 설계한다.

---

### Module 4: Change History - 변경 이력 추적

#### REQ-MSG-001-04-01 (State-Driven) History 비활성 상태 (기본값)

**IF** History가 비활성화 상태(기본값)이면, **THEN** Payload와 Metadata에 래퍼(decorator)를 적용하지 않고 직접 동작시켜야 한다 (제로 오버헤드).

#### REQ-MSG-001-04-02 (State-Driven) History 활성 상태

**IF** `WithHistory(true)`로 History가 활성화되면, **THEN** `historyPayload` decorator가 Payload를 감싸고, `historyMetadata` decorator가 Metadata를 감싸서 모든 변경 연산을 `ChangeRecord`로 기록해야 한다.

#### REQ-MSG-001-04-03 (Event-Driven) Payload 변경 기록

**WHEN** History 활성 상태에서 Payload의 `Add`, `Set`, `Delete` 연산이 호출되면, **THEN** `ChangeRecord{Target: "payload"}`를 생성하여 History에 추가해야 한다.

#### REQ-MSG-001-04-04 (Event-Driven) Metadata 변경 기록

**WHEN** History 활성 상태에서 Metadata의 `Set`, `Remove` 연산이 호출되면, **THEN** `ChangeRecord{Target: "metadata"}`를 생성하여 History에 추가해야 한다.

#### REQ-MSG-001-04-05 (State-Driven) History FIFO 제한

**IF** History 기록 수가 `WithMaxHistory(n)`으로 설정된 최대값(기본값: 100)에 도달하면, **THEN** 가장 오래된 기록을 제거하고 새 기록을 추가해야 한다 (FIFO).

#### REQ-MSG-001-04-06 (Ubiquitous) History 조회

시스템은 **항상** `Message.History()` 호출 시 현재까지의 `[]ChangeRecord` 목록을 반환해야 한다. History가 비활성화 상태이면 빈 슬라이스를 반환한다.

#### REQ-MSG-001-04-07 (Event-Driven) 커스텀 Payload + History 호환

**WHEN** `WithPayload(customPayload)`와 `WithHistory(true)`가 동시에 사용되면, **THEN** `historyPayload` decorator가 커스텀 Payload를 감싸서 변경 이력을 추적해야 한다.

#### REQ-MSG-001-04-08 (Unwanted) History 비활성 시 기록 금지

시스템은 History가 비활성화된 상태에서 `ChangeRecord`를 **생성하지 않아야 한다**.

---

### Module 5: Clone & Serialization - 복제 및 직렬화

#### REQ-MSG-001-05-01 (Ubiquitous) Message Clone

시스템은 **항상** `Message.Clone()` 호출 시 다음 규칙을 따르는 새로운 Message를 반환해야 한다:

- 새로운 UUID v4를 생성한다
- 원본의 Timestamp를 유지한다
- Payload를 deep copy한다
- Metadata를 deep copy한다
- History 설정(활성 여부, 최대 개수)을 복제하되, History 기록은 빈 상태로 시작한다

#### REQ-MSG-001-05-02 (Ubiquitous) Payload Clone 독립성

시스템은 **항상** `Payload.Clone()` 결과물의 수정이 원본 Payload에 영향을 주지 않도록 보장해야 한다.

#### REQ-MSG-001-05-03 (Ubiquitous) Metadata Clone 독립성

시스템은 **항상** `Metadata.Clone()` 결과물의 수정이 원본 Metadata에 영향을 주지 않도록 보장해야 한다.

#### REQ-MSG-001-05-04 (Ubiquitous) JSON 직렬화

시스템은 **항상** `defaultMessage`에 대해 커스텀 `MarshalJSON` 메서드를 제공하여, Message를 JSON으로 직렬화할 수 있어야 한다.

직렬화 구조:

```json
{
  "id": "uuid-string",
  "timestamp": "RFC3339 formatted time",
  "payload": { ... },
  "metadata": { ... },
  "history_enabled": true,
  "history": [ ... ]
}
```

#### REQ-MSG-001-05-05 (Ubiquitous) JSON 역직렬화

시스템은 **항상** `FromJSON(data []byte) (Message, error)` 함수를 제공하여, JSON 데이터로부터 Message를 복원해야 한다.

- 반환되는 Payload는 항상 `mapPayload` 기본 구현체이다
- 반환되는 Metadata는 항상 `mapMetadata` 기본 구현체이다
- 유효하지 않은 JSON은 에러를 반환한다

#### REQ-MSG-001-05-06 (Unwanted) Clone 간 간섭 금지

시스템은 Clone된 Message의 Payload 또는 Metadata 수정이 원본 Message에 영향을 **주지 않아야 한다**. 원본 Message의 수정이 Clone된 Message에 영향을 **주지 않아야 한다**.

---

## 4. Specifications (사양)

### 4.1 패키지 구조

```
pkg/message/
  message.go         # Message 인터페이스, New(), Option, ChangeRecord
  payload.go         # Payload 인터페이스, mapPayload, NewPayload()
  metadata.go        # Metadata 인터페이스, mapMetadata, NewMetadata(), 상수
  history.go         # historyPayload, historyMetadata decorators
  path.go            # GetPath jsonpath 파싱 및 평가 로직
  json.go            # MarshalJSON, FromJSON 직렬화/역직렬화
  errors.go          # ErrKeyExists 등 패키지 에러 정의
  message_test.go    # Message 테스트
  payload_test.go    # Payload 테스트
  metadata_test.go   # Metadata 테스트
  history_test.go    # History 테스트
  path_test.go       # GetPath 테스트
  json_test.go       # 직렬화/역직렬화 테스트
```

### 4.2 에러 정의

| 에러 변수 | 설명 |
|-----------|------|
| `ErrKeyExists` | Payload.Add에서 키가 이미 존재할 때 |
| `ErrInvalidPath` | GetPath에서 경로 구문이 유효하지 않을 때 |
| `ErrPathNotFound` | GetPath에서 경로에 해당하는 값이 없을 때 |

### 4.3 인터페이스 시그니처 요약

```go
// Message - 메시지 인터페이스
type Message interface {
    ID() string
    Timestamp() time.Time
    Payload() Payload
    Metadata() Metadata
    History() []ChangeRecord
    HistoryEnabled() bool
    Clone() Message
}

// Payload - 데이터 인터페이스
type Payload interface {
    Add(key string, value any) error
    Set(key string, value any)
    Delete(key string)
    Get(key string) (any, bool)
    GetPath(jsonpath string) (any, error)
    Keys() []string
    ToMap() map[string]any
    ToJSON() ([]byte, error)
    Clone() Payload
}

// Metadata - 메타데이터 인터페이스
type Metadata interface {
    Get(key string) (string, bool)
    Set(key string, value string)
    Has(key string) bool
    Remove(key string)
    All() map[string]string
    Clone() Metadata
}
```

### 4.4 Decorator 패턴 (History)

```
WithHistory(false) [기본]:
  Message -> Payload (직접)
  Message -> Metadata (직접)

WithHistory(true):
  Message -> historyPayload(Payload) -> 원본 Payload
  Message -> historyMetadata(Metadata) -> 원본 Metadata
```

- `historyPayload`는 `Payload` 인터페이스를 구현하며, 내부에 원본 `Payload`를 보유한다
- `historyMetadata`는 `Metadata` 인터페이스를 구현하며, 내부에 원본 `Metadata`를 보유한다
- 변경 연산(Add/Set/Delete/Remove) 호출 시 `ChangeRecord`를 기록한 후 원본에 위임한다
- 읽기 연산(Get/Keys/ToMap/ToJSON/Clone/All/Has)은 원본에 직접 위임한다

### 4.5 GetPath 지원 구문

| 구문 | 예시 | 설명 |
|------|------|------|
| Dot-notation | `$.name` | 최상위 키 접근 |
| 중첩 객체 | `$.user.address.city` | 깊은 중첩 접근 |
| 배열 인덱스 | `$.items[0]` | 특정 인덱스 접근 |
| 배열 와일드카드 | `$.items[*].name` | 모든 요소의 특정 필드 |

### 4.6 JSON 직렬화 스키마

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-02-12T10:30:00Z",
  "payload": {
    "key1": "value1",
    "key2": 42,
    "nested": { "deep": true }
  },
  "metadata": {
    "_source": "node-A",
    "_flowID": "flow-123",
    "custom": "value"
  },
  "history_enabled": true,
  "history": [
    {
      "target": "payload",
      "operation": "add",
      "key": "key1",
      "old_value": null,
      "new_value": "value1",
      "node_id": "node-A",
      "timestamp": "2026-02-12T10:30:01Z"
    }
  ]
}
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-MSG-001-01-01 ~ 01-09 | Core | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-MSG-001-02-01 ~ 02-02 | Payload | Event-Driven | 단위 테스트 (Add 성공/실패) |
| REQ-MSG-001-02-03 ~ 02-05 | Payload | Ubiquitous | 단위 테스트 (Set/Delete/Get) |
| REQ-MSG-001-02-06 ~ 02-07 | Payload | Event-Driven | 단위 테스트 (GetPath 성공/에러) |
| REQ-MSG-001-02-08 ~ 02-11 | Payload | Ubiquitous | 단위 테스트 (Keys/ToMap/ToJSON/Clone) |
| REQ-MSG-001-02-12 | Payload | Optional | 통합 테스트 (커스텀 Payload 주입) |
| REQ-MSG-001-03-01 ~ 03-06 | Metadata | Ubiquitous | 단위 테스트 (CRUD 연산) |
| REQ-MSG-001-03-07 | Metadata | Ubiquitous | 컴파일 타임 검증 (상수 선언) |
| REQ-MSG-001-03-08 | Metadata | Unwanted | 컴파일 타임 검증 (타입 시그니처) |
| REQ-MSG-001-04-01 ~ 04-02 | History | State-Driven | 단위 테스트 (활성/비활성 상태) |
| REQ-MSG-001-04-03 ~ 04-04 | History | Event-Driven | 단위 테스트 (변경 기록 생성) |
| REQ-MSG-001-04-05 | History | State-Driven | 단위 테스트 (FIFO 제한) |
| REQ-MSG-001-04-06 | History | Ubiquitous | 단위 테스트 (History 조회) |
| REQ-MSG-001-04-07 | History | Event-Driven | 통합 테스트 (커스텀 Payload + History) |
| REQ-MSG-001-04-08 | History | Unwanted | 단위 테스트 (비활성 시 기록 없음) |
| REQ-MSG-001-05-01 | Clone | Ubiquitous | 단위 테스트 (Message Clone) |
| REQ-MSG-001-05-02 ~ 05-03 | Clone | Ubiquitous | 단위 테스트 (Clone 독립성) |
| REQ-MSG-001-05-04 | Serialization | Ubiquitous | 단위 테스트 (JSON Marshal) |
| REQ-MSG-001-05-05 | Serialization | Ubiquitous | 단위 테스트 (JSON Unmarshal) |
| REQ-MSG-001-05-06 | Clone | Unwanted | 단위 테스트 (양방향 독립성) |
