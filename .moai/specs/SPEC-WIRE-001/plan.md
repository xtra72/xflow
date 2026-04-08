# SPEC-WIRE-001: 구현 계획

## 관련 SPEC

- SPEC ID: SPEC-WIRE-001
- 제목: Wire 구조체 Name/Type 필드 추가

---

## 마일스톤

### Primary Goal: Wire 구조체 확장 (백엔드)

**대상 파일:**

1. `pkg/flow/connection.go`
   - `WireType` 타입 및 `WireSimple` 상수 정의
   - `Wire` 구조체에 `Name`, `Type` 필드 추가
   - `NewWire` 팩토리에 `Type: WireSimple` 기본값 설정

2. `pkg/flow/serialize.go`
   - `normalizeWireDefaults`에서 ID를 UUID로 생성하도록 변경
   - `normalizeWireDefaults`에서 Type 기본값 설정
   - `normalizeWireNames` 함수 신규 추가 (노드 이름 기반 Wire Name 자동 생성)
   - `UnmarshalFlow` 파이프라인에 `normalizeWireNames` 호출 추가

3. `internal/engine/wire.go`
   - `RuntimeWire`에 `Name`, `Type` 필드 추가
   - `CreateRuntimeWires`에서 Name, Type 복사 로직 추가

**기술적 접근:**

- `normalizeWireNames`는 노드 목록(`[]NodeDef`)에서 노드 이름을 조회해야 하므로, 기존 `normalizeWireDefaults(wires, flowName)` 시그니처와 별도로 `normalizeWireNames(wires []Wire, nodes []NodeDef)` 시그니처를 사용
- 노드 이름 조회를 위해 `map[string]string` (nodeID -> nodeName) 맵 구성
- 기존 `<flowName>.wire-<index>` ID 생성 로직을 `uuid.New().String()`으로 교체

### Secondary Goal: 테스트 작성

**대상 파일:**

1. `pkg/flow/connection_test.go`
   - `NewWire` 팩토리의 Type 기본값 검증
   - `WireType` 상수 검증

2. `pkg/flow/serialize_test.go`
   - Wire 정규화 시 Name 자동 생성 검증
   - Wire 정규화 시 Type 기본값 검증
   - 레거시 YAML에서 name/type 미지정 시 정상 동작 검증
   - ID가 비어있을 때 UUID 형식 생성 검증

3. `internal/engine/wire_test.go`
   - `CreateRuntimeWires`에서 Name, Type 복사 검증

### Final Goal: 프론트엔드 동기화

**대상 파일:**

1. `web/src/stores/editorStore.ts`
   - Edge 생성 시 name, type 데이터 포함

2. `web/src/types/` (해당 파일 존재 시)
   - Wire/Edge 타입에 name, type 필드 추가

---

## 기술적 접근

### 백엔드 변경 전략

1. **구조체 변경은 최소 침습적으로 진행**
   - `Wire` 구조체에 필드 2개(Name, Type) 추가
   - JSON 태그의 `omitempty`는 사용하지 않음 (항상 직렬화)

2. **정규화 파이프라인 확장**
   - 기존 파이프라인: `normalizeEdgesToWires` -> `normalizeWireShorthand` -> `normalizeWireDefaults`
   - 확장 파이프라인: 기존 + `normalizeWireNames` (노드 정보 필요하므로 FlowJSON 레벨에서 호출)

3. **하위 호환성 보장**
   - 기존 YAML/JSON 파일은 name, type 필드 없이도 정상 파싱
   - Go의 JSON/YAML 언마샬링은 누락 필드를 zero-value로 처리
   - 정규화 단계에서 zero-value를 기본값으로 채움

### 프론트엔드 변경 전략

- React Flow Edge 객체의 `data` 필드에 name, type 추가
- 기존 CustomEdge 컴포넌트는 변경하지 않음 (표시는 향후)
- `onConnect` 훅에서 name 자동 생성

---

## 리스크 및 대응

| 리스크 | 영향도 | 대응 방안 |
|--------|--------|----------|
| 기존 테스트 깨짐 | 중간 | Wire 필드 추가로 인한 테스트 리터럴 수정 필요 |
| normalizeWireNames에서 노드 이름 조회 실패 | 낮음 | 노드 ID를 fallback으로 사용 |
| 프론트엔드 타입 불일치 | 낮음 | TypeScript 타입 정의 동기화 필요 |
| 직렬화된 YAML 파일 크기 증가 | 낮음 | name, type 필드가 추가되나 영향 미미 |

---

## 의존성

- 외부 의존성 없음 (기존 `github.com/google/uuid` 패키지 사용)
- 다른 SPEC과의 의존성 없음
