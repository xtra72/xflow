# SPEC-WIRE-001: Wire 구조체 Name/Type 필드 추가

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-WIRE-001 |
| 제목 | Wire 구조체 Name/Type 필드 추가 |
| 생성일 | 2026-04-08 |
| 상태 | Completed |
| 완료일 | 2026-04-08 |
| 우선순위 | High |
| 관련 파일 | `pkg/flow/connection.go`, `pkg/flow/serialize.go`, `internal/engine/wire.go` |

---

## Environment (환경)

- **플랫폼**: xflow - Go 기반 플로우 엔진 + React 프론트엔드
- **언어**: Go 1.23+, TypeScript 5.x (React Flow)
- **핵심 구조체**: `flow.Wire` (pkg/flow/connection.go), `engine.RuntimeWire` (internal/engine/wire.go)
- **직렬화**: YAML/JSON 기반 플로우 파일, `pkg/flow/serialize.go`의 정규화 파이프라인
- **프론트엔드**: @xyflow/react Edge 객체, `web/src/stores/editorStore.ts`

---

## Assumptions (가정)

1. Wire ID는 이미 `uuid.New().String()`으로 생성되고 있으며, 이 SPEC에서는 이를 공식 규격으로 명시화한다.
2. Wire Name은 자동 생성되며 사용자가 직접 입력하지 않는다.
3. Wire Type의 초기 값은 "simple" 하나이며, 추가 타입은 향후 별도 SPEC에서 정의한다.
4. 기존 YAML 플로우 파일에 `name`과 `type` 필드가 없는 경우, 정규화 단계에서 기본값을 자동 설정한다.
5. Wire Name 형식에서 노드 이름은 `NodeDef.Name` 필드를 사용한다.
6. Name 필드는 디버깅, 로깅, UI 표시 목적으로 사용되며, 라우팅 로직에는 사용하지 않는다.

---

## Requirements (요구사항)

### REQ-1: Wire ID UUID 규격화 (Ubiquitous)

시스템은 **항상** Wire ID를 UUID v4 형식으로 생성해야 한다.

- Wire ID는 `github.com/google/uuid` 패키지의 `uuid.New().String()`으로 생성한다.
- YAML/JSON에서 ID가 비어있을 경우, `normalizeWireDefaults`에서 UUID를 자동 할당한다.
- 기존 `<flowName>.wire-<index>` 형식의 ID 생성 로직을 UUID로 교체한다.

### REQ-2: Wire Name 필드 추가 (Event-Driven)

**WHEN** Wire가 생성될 때 **THEN** Name 필드가 `<src_node_name>.<src_port_name>_to_<dest_node_name>.<dest_port_name>` 형식으로 자동 생성되어야 한다.

- `Wire` 구조체에 `Name string` 필드를 추가한다 (JSON 태그: `"name"`)
- Name은 소스/타겟 노드의 `NodeDef.Name`과 포트 이름을 조합하여 생성한다.
- 예시: `mqtt-subscriber.output_to_lgcp-encoder.input`
- YAML/JSON에서 name이 명시적으로 제공된 경우, 자동 생성하지 않고 명시된 값을 사용한다.

### REQ-3: Wire Type 필드 추가 (State-Driven)

**IF** Wire에 Type 필드가 설정되지 않았다면 **THEN** 기본값 `"simple"`이 적용되어야 한다.

- `WireType` 문자열 타입과 `WireSimple WireType = "simple"` 상수를 정의한다.
- `Wire` 구조체에 `Type WireType` 필드를 추가한다 (JSON 태그: `"type"`)
- `normalizeWireDefaults`에서 Type이 비어있으면 `WireSimple`을 기본값으로 설정한다.

### REQ-4: RuntimeWire 동기화 (Ubiquitous)

시스템은 **항상** `RuntimeWire` 구조체에 `Name`과 `Type` 필드를 포함하고, `CreateRuntimeWires`에서 `flow.Wire`의 값을 복사해야 한다.

### REQ-5: 하위 호환성 (Unwanted)

시스템은 기존 YAML/JSON 플로우 파일에서 `name`과 `type` 필드가 없는 경우에도 정상 동작**하지 않아야 한다**는 오류를 발생시키지 않아야 한다.

- `name` 미지정 시: 정규화 단계에서 자동 생성 (노드 이름 조회 필요)
- `type` 미지정 시: `"simple"` 기본값 적용
- 레거시 `edges` 형식 변환 시에도 동일한 기본값 적용

### REQ-6: 프론트엔드 Wire 정보 표시 (Optional)

**가능하면** 프론트엔드 Edge 객체에 Wire Name과 Type 정보를 포함하여 UI에서 확인할 수 있도록 제공한다.

- `editorStore.ts`의 `onConnect` 훅에서 Name 자동 생성
- Edge 데이터에 name, type 필드 포함
- CustomEdge 컴포넌트에서 Name 표시는 향후 UI 개선 시 구현

---

## Specifications (상세 사양)

### S-1: WireType 열거형

```
type WireType string

const (
    WireSimple WireType = "simple"
)
```

### S-2: Wire 구조체 변경

```
type Wire struct {
    ID           string        `json:"id"`
    Name         string        `json:"name"`
    Type         WireType      `json:"type"`
    SourceNodeID string        `json:"source_node_id"`
    SourcePort   string        `json:"source_port"`
    TargetNodeID string        `json:"target_node_id"`
    TargetPort   string        `json:"target_port"`
    Mode         WireMode      `json:"mode"`
    BufferSize   int           `json:"buffer_size"`
    TTL          time.Duration `json:"ttl"`
}
```

### S-3: Wire Name 생성 규칙

- 형식: `<src_node_name>.<src_port_name>_to_<dest_node_name>.<dest_port_name>`
- 노드 이름이 비어있을 경우 노드 ID를 대신 사용
- 포트 이름이 비어있을 경우 "default" 사용
- 예시: `mqtt-sub.output_to_lgcp-enc.input`

### S-4: normalizeWireDefaults 변경

- ID가 비어있으면 UUID 생성 (`uuid.New().String()`)
- Type이 비어있으면 `WireSimple` 설정
- Name 생성은 노드 이름 조회가 필요하므로 별도 정규화 함수(`normalizeWireNames`) 추가

### S-5: RuntimeWire 구조체 변경

```
type RuntimeWire struct {
    ID, Name     string
    Type         flow.WireType
    SourceNodeID string
    SourcePort   string
    TargetNodeID string
    TargetPort   string
    Mode         flow.WireMode
    BufferSize   int
    TTL          time.Duration
    Ch           chan message.Message
    closed       atomic.Bool
    dropped      atomic.Int64
}
```

### S-6: NewWire 팩토리 변경

- `Type` 기본값을 `WireSimple`로 설정
- `Name`은 팩토리에서 빈 문자열로 초기화 (노드 이름 컨텍스트가 없으므로 정규화 단계에서 생성)

---

## Traceability (추적성)

| 요구사항 | 구현 파일 | 테스트 |
|----------|----------|--------|
| REQ-1 | `pkg/flow/connection.go`, `pkg/flow/serialize.go` | `pkg/flow/connection_test.go` |
| REQ-2 | `pkg/flow/connection.go`, `pkg/flow/serialize.go` | `pkg/flow/serialize_test.go` |
| REQ-3 | `pkg/flow/connection.go`, `pkg/flow/serialize.go` | `pkg/flow/connection_test.go` |
| REQ-4 | `internal/engine/wire.go` | `internal/engine/wire_test.go` |
| REQ-5 | `pkg/flow/serialize.go` | `pkg/flow/serialize_test.go` |
| REQ-6 | `web/src/stores/editorStore.ts` | 프론트엔드 테스트 |
