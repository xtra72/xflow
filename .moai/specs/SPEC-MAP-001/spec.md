---
id: SPEC-MAP-001
version: "1.0.0"
status: completed
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
priority: P1
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-11 | 1.0.0 | 초기 SPEC 작성 |
| 2026-03-11 | 1.1.0 | 구현 완료 - MappingNode 코어 + 테스트 + 프론트엔드 스키마 |

---

# SPEC-MAP-001: Mapping Node - 키 기반 값 매핑 노드

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 데이터 처리 노드 시스템에 **Mapping Node**를 추가한다. Mapping Node는 메시지 필드 값을 키로 사용하여 설정된 매핑 테이블에서 대응하는 값을 조회하고, 조회 결과를 메시지에 기록하는 처리 노드이다.

IoT 데이터 파이프라인에서 센서 코드를 사람이 읽을 수 있는 이름으로 변환하거나, 상태 코드를 상태 메시지로 변환하는 등의 룩업(lookup) 패턴을 별도 스크립트 작성 없이 설정만으로 처리할 수 있다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/node/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `Lifecycle`, `BaseLifecycle`, `State` 임베딩
  - `pkg/flow/` (SPEC-FLOW-001): `NodeDef`, `Port`, `PortDirection` 데이터 구조
  - `pkg/message/` (SPEC-MSG-001): `Message`, `Payload` 인터페이스 (메시지 입출력)
  - `internal/node/base.go` (SPEC-NODE-001): `BaseNode`, `Node` 인터페이스, `NodeOption`
  - `internal/node/registry.go` (SPEC-NODE-001): `Registry`, `NodeFactory` 빌트인 등록
  - `internal/node/expression.go` (SPEC-NODE-001): `messageToMap()` 메시지-맵 변환 유틸리티
- **프론트엔드**:
  - `web/src/config/nodeSchemas.ts`: 노드 타입별 설정 스키마
  - `web/src/pages/nodes/nodeTypeMeta.ts`: 노드 타입별 상세 메타데이터
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: 노드당 1 goroutine (Engine이 관리), 내부 설정은 `sync.RWMutex` 보호

### 1.3 설계 원칙

- **기존 패턴 준수**: SPEC-NODE-001에서 정의한 Node 인터페이스, BaseNode 임베딩, 팩토리 레지스트리 패턴을 동일하게 적용
- **설정 기반 동작**: Go 코드 없이 `Configure()` 메서드를 통한 JSON/YAML 설정만으로 매핑 룩업 동작을 정의
- **명시적 에러 처리**: 키 미발견 시 기본값이 없으면 센티널 에러 반환, 에러 포트로 라우팅
- **Hot Configuration**: 런타임 중 매핑 테이블 변경 가능 (재시작 없이)
- **인터페이스 컴파일 체크**: `var _ Node = (*MappingNode)(nil)` 패턴으로 인터페이스 구현 검증

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- MappingNode 구조체 및 팩토리 함수 (`mapping.go`)
- MappingNode 테스트 (`mapping_test.go`)
- 매핑 전용 센티널 에러 추가 (`errors.go`)
- 빌트인 레지스트리 등록 (`registry.go`)
- 프론트엔드 설정 스키마 추가 (`nodeSchemas.ts`)
- 프론트엔드 노드 메타데이터 추가 (`nodeTypeMeta.ts`)

**OUT OF SCOPE (별도 SPEC)**:
- 정규식 기반 키 매칭 (향후 확장)
- 외부 데이터소스 기반 동적 매핑 테이블 (DB, API 등)
- 다중 필드 복합 키 매핑

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 의존 | Node 인터페이스, BaseNode, Registry, 포트 시스템, 기존 10개 빌트인 노드 |
| SPEC-LIFE-001 | 의존 | Lifecycle, BaseLifecycle, State 임베딩 |
| SPEC-FLOW-001 | 의존 | NodeDef, Port, PortDirection 데이터 구조 |
| SPEC-MSG-001 | 의존 | Message, Payload 인터페이스 (GetPath 메서드) |
| SPEC-ENGINE-001 | 소비자 | Engine이 MappingNode의 Process를 호출하여 메시지 처리 |

---

## 2. Assumptions (가정)

### 2.1 기술 가정

- `pkg/message` 패키지의 `Payload.GetPath(jsonpath)` 메서드가 JSONPath 기반 중첩 데이터 접근을 지원한다
- `internal/node/expression.go`의 `messageToMap()` 함수가 메시지를 `map[string]any`로 변환할 수 있다
- 매핑 키는 문자열로 변환 가능한 값이어야 한다 (`fmt.Sprintf("%v", value)`)
- 매핑 테이블의 값은 `any` 타입으로, 문자열, 숫자, 불리언, 객체 등 모든 JSON 호환 타입을 지원한다
- 매핑 테이블 크기는 메모리에 적재할 수 있는 수준이다 (수천~수만 항목)

### 2.2 프로세스 가정

- 기존 빌트인 노드(filter, transform 등)와 동일한 테스트 패턴(테이블 드리븐, 한국어 테스트명)을 따른다
- 프론트엔드 스키마 변경은 기존 패턴(`nodeSchemas.ts`, `nodeTypeMeta.ts`)에 항목 추가 방식으로 수행한다
- 레지스트리 등록은 `registerBuiltins()` 함수의 `builtins` 슬라이스에 항목을 추가한다

---

## 3. Requirements (요구사항)

### 모듈 1: MappingNode 코어 (Go 백엔드)

#### R-MAP-001: MappingNode 구조체 및 팩토리 (Ubiquitous)

시스템은 **항상** 다음 구조를 가진 `MappingNode`를 제공해야 한다:

- `MappingNode` 구조체는 `*BaseNode`를 임베딩한다
- `field` (string): 소스 필드 JSONPath (예: `$.payload.status`)
- `mappings` (map[string]any): 키-값 매핑 테이블
- `defaultValue` (any): 키 미발견 시 기본값 (nil이면 기본값 없음)
- `hasDefault` (bool): 기본값 설정 여부 플래그
- `target` (string): 출력 필드명 (빈 문자열이면 소스 필드 덮어쓰기)
- `mu` (sync.RWMutex): 설정 보호 뮤텍스

`NewMappingNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수를 제공한다.

#### R-MAP-002: Configure 메서드 (Event-Driven)

**WHEN** `Configure(config map[string]any)`가 호출되면 **THEN** MappingNode는 다음을 수행해야 한다:

1. `BaseNode.Configure(config)` 호출하여 기본 설정 처리
2. `config["field"]` (string, 필수): 소스 필드 JSONPath 추출, 누락 시 `ErrInvalidConfig` 반환
3. `config["mappings"]` (map[string]any, 필수): 매핑 테이블 추출, 누락 또는 빈 맵이면 `ErrInvalidConfig` 반환
4. `config["default"]` (any, 선택): 기본값 설정, 존재하면 `hasDefault = true`
5. `config["target"]` (string, 선택): 출력 필드명 설정, 미지정 시 빈 문자열 (소스 필드 덮어쓰기)
6. 모든 설정은 `sync.RWMutex` 보호 하에 원자적으로 적용

#### R-MAP-003: Process 메서드 - 키 매칭 (Event-Driven)

**WHEN** `Process(ctx, msg)`가 호출되면 **THEN** MappingNode는 다음을 수행해야 한다:

1. `messageToMap(msg)`로 메시지를 맵으로 변환
2. `message.NewPayload(맵).GetPath(field)`로 소스 필드 값을 조회
3. 소스 필드 값을 `fmt.Sprintf("%v", value)`로 문자열 키로 변환
4. 매핑 테이블에서 키를 조회
5. **키가 존재하면**: 매핑된 값을 결과로 사용
6. **키가 없고 기본값이 있으면**: 기본값을 결과로 사용
7. **키가 없고 기본값이 없으면**: `ErrMappingKeyNotFound` 에러 반환
8. 결과 값을 `target` 필드에 기록 (target이 빈 문자열이면 소스 field 경로에 기록)
9. 변경된 메시지를 `[]message.Message{msg}` 슬라이스로 반환

#### R-MAP-004: Process 메서드 - 소스 필드 미발견 (Unwanted)

MappingNode는 소스 필드가 메시지에 존재하지 않을 때 `ErrMappingFieldNotFound` 에러를 **반환해야 한다**. 소스 필드 미발견 시 메시지를 출력 포트로 전달하지 않아야 한다.

#### R-MAP-005: Init / Shutdown (Ubiquitous)

시스템은 **항상** 다음 생명주기 메서드를 제공해야 한다:

- `Init(ctx)`: `BaseNode.TransitionTo(StateInitializing)` -> `BaseNode.TransitionTo(StateRunning)` 전이
- `Shutdown(ctx)`: `BaseNode.TransitionTo(StateStopping)` 전이

#### R-MAP-006: 센티널 에러 (Ubiquitous)

시스템은 **항상** 다음 센티널 에러를 `errors.go`에 정의해야 한다:

- `ErrMappingKeyNotFound = errors.New("node: mapping key not found")` - 매핑 테이블에 키가 없고 기본값이 설정되지 않았을 때
- `ErrMappingFieldNotFound = errors.New("node: mapping source field not found")` - 소스 필드가 메시지에 존재하지 않을 때

#### R-MAP-007: 레지스트리 등록 (Ubiquitous)

시스템은 **항상** `registerBuiltins()` 함수의 `builtins` 슬라이스에 다음 항목을 포함해야 한다:

```
{"mapping", NewMappingNode, "processing", "키 기반 값 매핑"}
```

#### R-MAP-008: 포트 정의 (Ubiquitous)

시스템은 **항상** MappingNode에 다음 포트를 정의해야 한다:

- 입력: `in` (input) - 매핑할 메시지 입력
- 출력: `out` (output) - 매핑 결과 메시지 출력
- 에러: `error` (error) - 키 미발견/필드 미발견 에러 시 출력

포트는 `flow.NewNodeDef(name, type)`에 의해 자동 생성되는 `in`+`out` 포트를 기본으로 사용한다.

### 모듈 2: 프론트엔드 스키마 및 메타데이터

#### R-MAP-009: nodeSchemas.ts 설정 스키마 (Ubiquitous)

시스템은 **항상** `NODE_SCHEMAS` 레지스트리에 `mapping` 항목을 포함해야 한다:

- `field` (string, 필수): 소스 필드 JSONPath
- `mappings` (json, 필수): 키-값 매핑 테이블 (JSON 객체)
- `default` (string, 선택): 기본값
- `target` (string, 선택): 출력 필드명
- 기본 포트: `in` (input), `out` (output)

#### R-MAP-010: nodeTypeMeta.ts 메타데이터 (Ubiquitous)

시스템은 **항상** `NODE_TYPE_META` 레지스트리에 `mapping` 항목을 포함해야 한다:

- `description`: 한국어 설명 (매핑 노드의 동작 설명)
- `ports`: in (input), out (output), error (error) 3개 포트 메타
- `configFields`: field, mappings, default, target 4개 설정 필드 메타
- `configExample`: 실제 사용 예제 JSON

---

## 4. Specifications (명세)

### 4.1 설정 구조

| 키 | 타입 | 필수 | 설명 | 기본값 |
|-----|------|------|------|--------|
| `field` | string | Yes | 소스 필드 JSONPath (예: `$.payload.status`) | - |
| `mappings` | map[string]any | Yes | 키-값 매핑 테이블 | - |
| `default` | any | No | 키 미발견 시 기본값 (nil이면 에러 반환) | nil |
| `target` | string | No | 출력 필드명 (빈 문자열이면 소스 필드 덮어쓰기) | "" |

### 4.2 처리 흐름

```
메시지 수신
  │
  ├─ field로 소스 값 추출 ──── 실패 → ErrMappingFieldNotFound → 에러 포트
  │
  ├─ 소스 값을 문자열 키로 변환
  │
  ├─ mappings[키] 조회
  │   ├─ 발견 → 매핑 값 사용
  │   └─ 미발견
  │       ├─ hasDefault=true → 기본값 사용
  │       └─ hasDefault=false → ErrMappingKeyNotFound → 에러 포트
  │
  ├─ target 필드에 결과 기록
  │   ├─ target 지정 → msg.Payload().Set(target, 결과)
  │   └─ target 미지정 → 소스 필드 경로에 기록
  │
  └─ 메시지 출력
```

### 4.3 설정 예제

```json
{
  "field": "$.payload.status_code",
  "mappings": {
    "0": "정상",
    "1": "경고",
    "2": "위험",
    "3": "긴급"
  },
  "default": "알 수 없음",
  "target": "status_text"
}
```

위 설정은 메시지의 `$.payload.status_code` 값을 조회하여 `status_text` 필드에 한국어 상태명을 기록한다. 매핑 테이블에 없는 코드는 "알 수 없음"으로 처리된다.

### 4.4 파일 구조

| 파일 | 액션 | 설명 |
|------|------|------|
| `internal/node/mapping.go` | 신규 | MappingNode 구조체, 팩토리, Init/Process/Shutdown/Configure |
| `internal/node/mapping_test.go` | 신규 | 테이블 드리븐 테스트 (인터페이스 체크, 팩토리, 설정, 프로세스 정상/에러/엣지 케이스) |
| `internal/node/errors.go` | 수정 | `ErrMappingKeyNotFound`, `ErrMappingFieldNotFound` 센티널 에러 추가 |
| `internal/node/registry.go` | 수정 | `registerBuiltins()`의 `builtins` 슬라이스에 mapping 항목 추가 |
| `web/src/config/nodeSchemas.ts` | 수정 | `NODE_SCHEMAS`에 mapping 스키마 추가 |
| `web/src/pages/nodes/nodeTypeMeta.ts` | 수정 | `NODE_TYPE_META`에 mapping 메타데이터 추가 |

### 4.5 추적성 태그

- `[SPEC-MAP-001]` - 본 SPEC의 모든 구현 파일에 커밋 메시지 접두사로 사용
- `[R-MAP-001]` ~ `[R-MAP-010]` - 각 요구사항의 구현 추적용

---

## 5. Implementation Notes (구현 노트)

### 5.1 구현 결과

| 항목 | 결과 |
|------|------|
| 신규 파일 | `mapping.go`, `mapping_test.go` |
| 수정 파일 | `errors.go`, `registry.go`, `registry_test.go`, `nodeSchemas.ts`, `nodeTypeMeta.ts` |
| 테스트 수 | 16개 (+ lastPathKey 4개 서브테스트) |
| 커버리지 | 87.8% (패키지 전체) |
| Race Detector | 통과 |
| go vet | 경고 없음 |

### 5.2 추가 구현 사항

- `lastPathKey()` 헬퍼 함수: JSONPath에서 마지막 키를 추출하는 유틸리티 (target 미지정 시 소스 필드 덮어쓰기에 사용)

### 5.3 요구사항 추적성

모든 요구사항 R-MAP-001 ~ R-MAP-010이 구현 및 테스트 완료되었다.
