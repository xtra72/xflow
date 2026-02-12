# SPEC-MSG-001: 구현 계획

> TAG: SPEC-MSG-001
> 상태: Planned
> 개발 방법론: Hybrid (TDD for new code)

---

## 1. 구현 개요

- **SPEC-MSG-001**: 인터페이스 기반 구조적 메시지 데이터 처리 및 변경 이력 추적
- **패키지**: `pkg/message/`
- **개발 방법론**: Hybrid (TDD for new code)
- **의존성**: 없음 (Tier 1 - 독립 패키지)

---

## 2. 구현 파일 및 순서

| 단계 | 파일 | 내용 | 예상 라인 수 |
|------|------|------|-------------|
| 1 | `pkg/message/metadata.go` | Metadata interface + mapMetadata + `NewMetadata()` + 시스템 키 상수 | ~100 |
| 2 | `pkg/message/payload.go` | Payload interface + mapPayload + `NewPayload()` + `ErrKeyExists` + GetPath 파서 | ~200 |
| 3 | `pkg/message/history.go` | ChangeRecord struct + historyPayload + historyMetadata 데코레이터 | ~150 |
| 4 | `pkg/message/clone.go` | `deepCopy()` 유틸리티 (JSON round-trip) | ~60 |
| 5 | `pkg/message/message.go` | Message interface + defaultMessage + `New()` + Option + `FromJSON()` | ~200 |
| 6 | `pkg/message/message_test.go` | 전체 테스트 (table-driven) | ~400 |

**총 예상: ~1,110 lines**

---

## 3. 기술 스택

- **Go 1.23+**
- **표준 라이브러리**: `encoding/json`, `time`, `sync`, `fmt`, `errors`
- **외부 의존성**: `github.com/google/uuid` (Message ID 생성)

---

## 4. 설계 결정사항

### Interface vs Struct

- Message, Payload, Metadata 모두 Go interface로 정의
- 기본 구현은 unexported struct (`defaultMessage`, `mapPayload`, `mapMetadata`)
- 팩토리 생성자가 interface를 반환
- 플러그인/커스텀 구현 확장성 확보

### Decorator 패턴 이력 추적

- `historyPayload`: Payload를 래핑하여 `Add`/`Set`/`Delete` 시 ChangeRecord 기록
- `historyMetadata`: Metadata를 래핑하여 `Set`/`Remove` 시 ChangeRecord 기록
- 비활성화 시 래퍼 없음 (zero overhead)
- ChangeRecord에 Target 필드 (`"payload"`/`"metadata"`) 추가

### Deep Copy 전략

- 초기 구현: `encoding/json` round-trip
- 프로파일링 후 필요 시 reflection 기반 최적화
- `map`, `slice` 재귀적 복사

### JSONPath

- MVP: 기본 경로 표현식 (`$.key`, `$.key.nested`, `$.array[0]`, `$.array[*].field`)
- 자체 경량 파서 구현 (외부 의존성 최소화)
- 고급 JSONPath는 후속 SPEC으로 분리

---

## 5. 마일스톤

| 마일스톤 | 내용 | 완료 기준 |
|---------|------|----------|
| **M1** | Metadata + Payload 인터페이스 및 기본 구현 | 인터페이스 정의, CRUD 연산, 단위 테스트 통과 |
| **M2** | History 데코레이터 + ChangeRecord | Payload/Metadata 변경 이력 기록, FIFO 삭제, 테스트 통과 |
| **M3** | Message 인터페이스 + Options 패턴 | `New()` 생성자, `Clone()`, `WithHistory`, `WithPayload` 테스트 통과 |
| **M4** | JSON 직렬화 + deepCopy | `MarshalJSON`, `FromJSON`, Clone 독립성 테스트 통과 |
| **M5** | 통합 테스트 + 벤치마크 | 커버리지 85%+, 경쟁 조건 없음, 성능 벤치마크 기록 |

---

## 6. 리스크 및 대응

| 리스크 | 심각도 | 대응 |
|--------|--------|------|
| 인터페이스 메서드 추가 = breaking change | High | 최소 메서드셋 설계, 확장 시 옵셔널 인터페이스 추가 |
| JSON 역직렬화 시 커스텀 구현 복원 불가 | Medium | `FromJSON`은 기본 구현 반환, 커스텀은 구현자 책임 |
| `any` 타입 deep copy edge case | Medium | JSON round-trip, 특수 타입(`chan`, `func`) 제외 |
| GetPath 파서 복잡도 | Medium | 4가지 기본 패턴만 지원, 고급은 후속 SPEC |
| historyMetadata 추가로 인한 복잡도 증가 | Low | 동일 데코레이터 패턴으로 일관성 유지 |

---

## 7. 테스트 전략

- **TDD** (RED-GREEN-REFACTOR) 적용 (새 코드)
- **Table-driven tests** 사용
- `go test -race` 필수 (경쟁 조건 검증)
- **벤치마크 테스트**: `New()`, `Clone()`, `Set()`, `GetPath()`
- **커버리지 목표**: 85%+

---

## 8. 의존성 관계

**이 패키지에 의존하는 패키지:**

- `internal/engine`
- `internal/node`
- `internal/agent`
- `internal/script`
- `internal/plugin`

**이 패키지의 외부 의존성:**

- `github.com/google/uuid` (1개만)
