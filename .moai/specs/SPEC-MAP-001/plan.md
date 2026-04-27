---
id: SPEC-MAP-001
type: plan
version: "1.1.0"
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
---

# SPEC-MAP-001: 구현 계획 - Mapping Node

## 1. 개요

Mapping Node는 키 기반 값 매핑을 수행하는 처리 노드이다. 기존 SPEC-NODE-001에서 정의된 10개 빌트인 노드와 동일한 패턴(BaseNode 임베딩, 팩토리 레지스트리, 테이블 드리븐 테스트)을 따라 구현한다.

## 2. 의존성

| 의존 대상 | SPEC ID | 상태 | 설명 |
|-----------|---------|------|------|
| Node Interface & BaseNode | SPEC-NODE-001 | completed | `Node` 인터페이스, `BaseNode` 임베딩 |
| Lifecycle | SPEC-LIFE-001 | completed | `StateInitializing`, `StateRunning`, `StateStopping` 상태 전이 |
| Flow Definitions | SPEC-FLOW-001 | completed | `flow.NodeDef`, `flow.NewNodeDef()` |
| Message System | SPEC-MSG-001 | completed | `message.Message`, `Payload.GetPath()`, `Payload.Set()` |
| Expression Util | SPEC-NODE-001 | completed | `messageToMap()` 유틸리티 함수 |

모든 의존성이 `completed` 상태이므로 즉시 구현 가능하다.

## 3. 마일스톤

### 마일스톤 1: 센티널 에러 정의 (Primary Goal) [R-MAP-006]

**태스크**:
- `internal/node/errors.go`에 `ErrMappingKeyNotFound`, `ErrMappingFieldNotFound` 추가
- 기존 에러 정의 패턴과 동일한 주석 스타일 적용

**변경 파일**: `internal/node/errors.go` (수정)

**검증**: 기존 에러 테스트(`errors_test.go`) 통과 확인

---

### 마일스톤 2: MappingNode 구현 (Primary Goal) [R-MAP-001 ~ R-MAP-005, R-MAP-008]

**태스크**:

1. `internal/node/mapping.go` 파일 생성
2. `MappingNode` 구조체 정의
   - `*BaseNode` 임베딩
   - `field`, `mappings`, `defaultValue`, `hasDefault`, `target` 필드
   - `mu sync.RWMutex` 동시성 보호
3. `NewMappingNode` 팩토리 함수 구현
   - `NewBaseNode(def, opts...)` 호출
   - `MappingNode` 인스턴스 반환
4. `Init(ctx)` 구현
   - `StateInitializing` -> `StateRunning` 전이
5. `Configure(config map[string]any)` 구현
   - `field` (필수, string) 추출 및 검증
   - `mappings` (필수, map[string]any) 추출 및 검증
   - `default` (선택, any) 추출
   - `target` (선택, string) 추출
   - `sync.RWMutex` 락 하에 원자적 적용
6. `Process(ctx, msg)` 구현
   - `messageToMap(msg)`로 맵 변환
   - `message.NewPayload(맵).GetPath(field)`로 소스 값 추출
   - `fmt.Sprintf("%v", value)`로 문자열 키 변환
   - `mappings[키]` 조회 -> 매핑 값 / 기본값 / 에러 반환
   - `msg.Payload().Set(target, result)` 또는 소스 필드에 기록
7. `Shutdown(ctx)` 구현
   - `StateStopping` 전이
8. 인터페이스 컴파일 체크: `var _ Node = (*MappingNode)(nil)`

**변경 파일**: `internal/node/mapping.go` (신규)

**검증**: 컴파일 성공 및 인터페이스 체크 통과

---

### 마일스톤 3: 레지스트리 등록 (Primary Goal) [R-MAP-007]

**태스크**:
- `internal/node/registry.go`의 `registerBuiltins()` 함수에 mapping 항목 추가
- 위치: 기존 `builtins` 슬라이스의 processing 카테고리 노드(aggregate 다음)에 추가

**변경 파일**: `internal/node/registry.go` (수정)

**검증**: `registry_test.go`의 빌트인 타입 수 검증 테스트 업데이트 (10 -> 11)

---

### 마일스톤 4: 단위 테스트 (Primary Goal) [R-MAP-001 ~ R-MAP-008]

**태스크**:

1. `internal/node/mapping_test.go` 파일 생성
2. 테이블 드리븐 테스트 구현:

| 테스트 케이스 | 설명 | 기대 결과 |
|-------------|------|----------|
| 인터페이스 컴파일 체크 | `var _ Node = (*MappingNode)(nil)` | 컴파일 성공 |
| 팩토리 함수 | `NewMappingNode` 호출 | 에러 없이 노드 반환 |
| Init 정상 | `Init(ctx)` 호출 | 에러 없음 |
| Configure 정상 | field + mappings 설정 | 에러 없음 |
| Configure field 누락 | mappings만 설정 | `ErrInvalidConfig` |
| Configure mappings 누락 | field만 설정 | `ErrInvalidConfig` |
| Configure mappings 빈 맵 | 빈 mappings 설정 | `ErrInvalidConfig` |
| Process 키 매칭 성공 | 매핑 테이블에 키 존재 | 매핑 값 출력 |
| Process 키 미발견 + 기본값 | 키 없음, default 설정 | 기본값 출력 |
| Process 키 미발견 + 기본값 없음 | 키 없음, default 미설정 | `ErrMappingKeyNotFound` |
| Process 소스 필드 미발견 | 존재하지 않는 JSONPath | `ErrMappingFieldNotFound` |
| Process target 필드 지정 | target에 결과 기록 | target 필드에 값 존재 |
| Process target 미지정 | 소스 필드 덮어쓰기 | 소스 필드 값 변경 |
| Process 숫자 키 변환 | 숫자 값을 문자열로 변환 | 정상 매핑 |
| Process 불리언 키 변환 | bool 값을 문자열로 변환 | 정상 매핑 |
| Shutdown 정상 | `Shutdown(ctx)` 호출 | 에러 없음 |

3. 테스트명은 한국어로 작성 (기존 패턴 준수)

**변경 파일**: `internal/node/mapping_test.go` (신규)

**검증**: `go test -race ./internal/node/... -run TestMapping` 전체 통과

---

### 마일스톤 5: 프론트엔드 스키마 (Secondary Goal) [R-MAP-009, R-MAP-010]

**태스크**:

1. `web/src/config/nodeSchemas.ts`의 `NODE_SCHEMAS`에 `mapping` 항목 추가:
   - `field`: string 타입, 필수, 소스 필드 JSONPath
   - `mappings`: json 타입, 필수, 키-값 매핑 테이블
   - `default`: string 타입, 선택, 기본값
   - `target`: string 타입, 선택, 출력 필드명
   - 기본 포트: `in` (input), `out` (output)

2. `web/src/pages/nodes/nodeTypeMeta.ts`의 `NODE_TYPE_META`에 `mapping` 항목 추가:
   - description: 한국어 설명
   - ports: in, out, error 3개 포트 메타
   - configFields: field, mappings, default, target 4개 필드 메타
   - configExample: 실제 사용 예제

**변경 파일**: `web/src/config/nodeSchemas.ts` (수정), `web/src/pages/nodes/nodeTypeMeta.ts` (수정)

**검증**: TypeScript 컴파일 성공 (`npx tsc --noEmit`)

---

## 4. 기술 접근

### 4.1 소스 필드 값 추출

`messageToMap(msg)` -> `message.NewPayload(맵).GetPath(field)` 체인을 사용하여 JSONPath 기반 필드 값을 추출한다. 이는 기존 filter, transform 노드에서 사용하는 동일한 패턴이다.

### 4.2 키 변환 전략

`fmt.Sprintf("%v", value)` 를 사용하여 소스 필드 값을 문자열 키로 변환한다:
- 문자열 "error" -> "error"
- 정수 42 -> "42"
- 부동소수점 3.14 -> "3.14"
- 불리언 true -> "true"
- nil -> `ErrMappingFieldNotFound` (GetPath에서 nil 반환 시)

### 4.3 결과 기록 전략

- `target`이 지정된 경우: `msg.Payload().Set(target, result)` - 새 필드에 기록
- `target`이 빈 문자열인 경우: 소스 필드의 최종 키에 `msg.Payload().Set(key, result)` - 기존 값 덮어쓰기

### 4.4 동시성 모델

- `Configure()` 호출 시 `mu.Lock()`으로 배타적 쓰기
- `Process()` 호출 시 `mu.RLock()`으로 공유 읽기
- 노드당 1 goroutine 모델이므로 `Process()` 간 동시 호출은 없으나, `Configure()` (Hot Config)와의 경합을 보호

## 5. 위험 분석

| 위험 | 영향도 | 발생 가능성 | 대응 |
|------|--------|------------|------|
| GetPath가 nil을 반환하는 경우 | 중간 | 높음 | nil 반환 시 ErrMappingFieldNotFound로 명시적 처리 |
| 매핑 테이블 키와 소스 값 타입 불일치 | 중간 | 중간 | fmt.Sprintf로 모든 값을 문자열 변환하여 일관성 확보 |
| 레지스트리 테스트 빌트인 수 하드코딩 | 낮음 | 높음 | registry_test.go의 기대 빌트인 수 10->11 업데이트 |
| 기존 프론트엔드 빌드 실패 | 중간 | 낮음 | TypeScript 컴파일 체크로 사전 검증 |

## 6. 추적성

| 요구사항 ID | 마일스톤 | 파일 |
|------------|---------|------|
| R-MAP-001 | M2 | `internal/node/mapping.go` |
| R-MAP-002 | M2 | `internal/node/mapping.go` |
| R-MAP-003 | M2 | `internal/node/mapping.go` |
| R-MAP-004 | M2 | `internal/node/mapping.go` |
| R-MAP-005 | M2 | `internal/node/mapping.go` |
| R-MAP-006 | M1 | `internal/node/errors.go` |
| R-MAP-007 | M3 | `internal/node/registry.go` |
| R-MAP-008 | M2 | `internal/node/mapping.go` |
| R-MAP-009 | M5 | `web/src/config/nodeSchemas.ts` |
| R-MAP-010 | M5 | `web/src/pages/nodes/nodeTypeMeta.ts` |
