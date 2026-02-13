---
id: SPEC-FLOW-001
type: plan
version: "1.0.0"
created: "2026-02-12"
updated: "2026-02-12"
---

# SPEC-FLOW-001 구현 계획

## 1. 개요

`pkg/flow/` 패키지에 Flow System의 공개 API를 구현한다. SPEC-MSG-001의 설계 패턴(인터페이스 우선, unexported 구현체, Options Pattern)을 동일하게 적용하며, Flow 정의/직렬화/검증에 필요한 모든 데이터 구조와 유틸리티를 제공한다.

## 2. 마일스톤

### Primary Goal: 핵심 데이터 구조 (Module 1 + Module 3)

Module 1 (Core Definitions)과 Module 3 (State Model)을 우선 구현한다. 이 두 모듈이 전체 시스템의 기반이 된다.

**구현 파일**:
- `pkg/flow/errors.go` - 패키지 에러 정의
- `pkg/flow/state.go` - FlowState 상수, 전이 맵, 유틸리티
- `pkg/flow/node.go` - NodeDef, Port, PortDirection, AgentRef, BridgeDirection, NewNodeDef()
- `pkg/flow/connection.go` - Wire, WireMode, NewWire(), WireOption
- `pkg/flow/flow.go` - Flow 인터페이스, defaultFlow, NewFlow(), FlowOption, FlowConfig
- `pkg/flow/state_test.go`
- `pkg/flow/node_test.go`
- `pkg/flow/connection_test.go`
- `pkg/flow/flow_test.go`

**구현 순서**: errors.go -> state.go -> node.go -> connection.go -> flow.go (의존성 순서)

**완료 기준**:
- 모든 타입과 인터페이스가 정의됨
- Flow 생성, 노드/와이어 추가/제거 동작
- 상태 전이 검증 동작
- 테스트 커버리지 85% 이상

### Secondary Goal: 식별 및 직렬화 (Module 2 + Module 4)

Module 2 (Identification & Addressing)와 Module 4 (Serialization)를 구현한다.

**구현 파일**:
- `pkg/flow/path.go` - NodePath, ParseNodePath(), String()
- `pkg/flow/serialize.go` - JSON/YAML 직렬화, 파일 로드/저장
- `pkg/flow/path_test.go`
- `pkg/flow/serialize_test.go`

**구현 순서**: path.go -> serialize.go

**완료 기준**:
- Dot 표기법 파싱/생성 동작 (대괄호 이스케이프 포함)
- JSON/YAML 양방향 직렬화 동작
- 파일 로드/저장 동작
- 라운드트립 테스트 통과 (직렬화 -> 역직렬화 -> 동일성 검증)

### Final Goal: 유효성 검증 (Module 5)

Module 5 (Validation)를 구현하여 전체 Flow 그래프의 무결성을 검증한다.

**구현 파일**:
- `pkg/flow/validate.go` - Validate(), ValidationError, ValidationSeverity
- `pkg/flow/validate_test.go`

**완료 기준**:
- 11개 검증 규칙 모두 구현
- Error/Warning 분류 동작
- 통합 검증 결과 반환

## 3. 기술 접근

### 3.1 아키텍처 설계 방향

SPEC-MSG-001과 동일한 설계 철학을 적용한다:

1. **인터페이스 우선**: `Flow`는 인터페이스로 정의하고, `defaultFlow`는 unexported struct로 구현
2. **Exported struct**: `NodeDef`, `Port`, `Wire`, `AgentRef`, `FlowConfig`는 직렬화 호환을 위해 exported struct
3. **Options Pattern**: `NewFlow()`, `NewNodeDef()`, `NewWire()` 모두 가변 옵션 인자 지원
4. **방어적 복사**: `Nodes()`, `Wires()`, `Metadata()` 등은 내부 컬렉션의 복사본을 반환

### 3.2 Flow 인터페이스 설계 근거

Flow를 인터페이스로 설계하는 이유:
- 런타임 엔진(`internal/engine/`)에서 확장 구현 가능 (예: 런타임 메트릭 수집이 포함된 runtimeFlow)
- 테스트 시 mock 교체 용이
- `pkg/flow/`의 공개 API 안정성 보장

### 3.3 NodeDef를 exported struct로 설계하는 근거

NodeDef는 인터페이스가 아닌 exported struct로 설계하는 이유:
- 직렬화/역직렬화에서 필드 직접 접근이 필요
- 노드 정의는 불변 데이터에 가까움 (생성 후 변경 드묾)
- 외부 도구에서 Flow 파일을 읽을 때 간단한 구조체 접근이 편리

### 3.4 UUID 생성 전략

SPEC-MSG-001과 동일하게 `crypto/rand` 기반 UUID v4를 사용한다. 별도 UUID 패키지 의존성을 추가하지 않고, 내부 유틸리티 함수로 구현한다.

### 3.5 Dot 표기법 파서 설계

간단한 상태 기반 파서로 구현한다:
- 상태: Normal, InBracket
- `.` 문자를 만나면 FlowRef/NodeRef 분리
- `[` 문자를 만나면 InBracket 상태 전환
- `]` 문자를 만나면 Normal 상태 복귀
- 파서는 정규표현식이 아닌 수동 순회로 구현 (성능, 에러 메시지 품질)

### 3.6 직렬화 전략

- **JSON**: `encoding/json` 표준 라이브러리 사용. Flow 인터페이스를 위해 커스텀 `MarshalJSON`/`UnmarshalJSON` 구현
- **YAML**: `gopkg.in/yaml.v3` 사용. JSON 중간 표현을 거치지 않고 직접 YAML 매핑
- **파일**: 확장자 기반 포맷 자동 감지 (`.json`, `.yaml`, `.yml`)

## 4. 리스크 및 대응

### R1: internal/engine/ 경계 모호성

**리스크**: FlowState 관련 로직이 `pkg/flow/`와 `internal/engine/` 사이에서 경계가 모호할 수 있다.

**대응**: `pkg/flow/`는 상태 열거와 유효 전이 "규칙 정의"만 담당하고, 실제 "전이 실행"(고루틴 제어, 채널 관리)은 `internal/engine/`에 명확히 위임한다. `Flow.SetState()`는 전이 유효성만 검증하고 단순히 상태 값을 변경한다.

### R2: Bridge Node AgentRef 결합도

**리스크**: AgentRef가 `internal/agent/` 패키지와 결합될 수 있다.

**대응**: AgentRef는 Agent의 ID/Name 문자열만 참조하며, `internal/agent/` 패키지를 직접 import하지 않는다. 런타임에서 Agent 인스턴스 해석은 엔진의 책임이다.

### R3: Dot 표기법 이름 충돌

**리스크**: 노드 이름에 마침표가 포함되면 경로 파싱이 모호해진다.

**대응**: 대괄호 이스케이프 표기법 (`[name.with.dots]`)을 도입하여 명확한 구분을 보장한다. `NodePath.String()` 메서드가 자동으로 필요 시 대괄호를 적용한다.

### R4: YAML 외부 의존성

**리스크**: `gopkg.in/yaml.v3` 추가로 인한 의존성 증가.

**대응**: YAML은 IoT 설정 파일의 사실상 표준이므로 필수 지원. `gopkg.in/yaml.v3`는 Go 생태계에서 가장 널리 사용되는 YAML 라이브러리이며, Go 표준 팀의 유지보수를 받는다.

### R5: 동시성 미지원

**리스크**: 단일 goroutine 가정으로 인해 다중 접근 시 데이터 레이스 발생 가능.

**대응**: SPEC-MSG-001과 동일한 전략. `pkg/flow/`는 데이터 정의 패키지로, 동시성 제어는 상위 계층(`internal/engine/`)에서 담당한다. 문서에 명시적으로 "동시성 안전하지 않음"을 표기한다.

## 5. 전문가 참고 사항

### Backend Expert (expert-backend) 참고

- Go 인터페이스 설계 패턴 (`Flow` 인터페이스 + unexported `defaultFlow`)
- Options Pattern 적용 (SPEC-MSG-001의 `Option` 패턴과 일관성)
- 방어적 복사 전략 (slice/map 반환 시 deep copy)
- `encoding/json` 커스텀 마샬링 (`MarshalJSON`/`UnmarshalJSON`)
- 상태 머신 패턴 (transition map 기반 유효성 검증)

### 의존성 구조

```
pkg/flow/ (본 SPEC)
    ├── 표준 라이브러리 (encoding/json, time, crypto/rand 등)
    ├── gopkg.in/yaml.v3
    └── (참조만) pkg/message/ - Message 타입 참조는 없음, 독립 패키지

internal/engine/ (SPEC-ENGINE-001, 별도)
    └── pkg/flow/ 임포트
```

`pkg/flow/`는 `pkg/message/`를 직접 import하지 않는다. 두 패키지는 Tier 1 독립 패키지로, 런타임 엔진(`internal/engine/`)에서 통합된다.
