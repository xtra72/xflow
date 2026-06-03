# SPEC-SUBFLOW-001 구현 계획 (plan.md)

> 플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트
> 본 문서는 PLAN 단계 산출물이다. 코드는 포함하지 않으며 기술 접근/마일스톤/위험을 정의한다.

## 1. 기술 접근

### 1.1 백엔드 — 플로우 레벨 포트 모델 (`pkg/flow`)

- `Flow` 인터페이스에 플로우 레벨 입출력 포트 접근/변이 메서드를 추가한다: `Inputs() []FlowPort`, `Outputs() []FlowPort`, 그리고 추가/이름변경/삭제 변이 메서드(예: `AddInputPort`, `RenamePort`, `RemovePort`). `defaultFlow` 에 `inputs []FlowPort` / `outputs []FlowPort` 필드를 추가한다.
- `FlowPort` 는 노드 `flow.Port`(node.go)와 형태는 유사하나 **플로우 레벨 엔티티**로 별도 정의한다(id 불변, name 변경 가능, direction).
- 포트는 노드가 아니므로 `Nodes()` 에 절대 포함하지 않는다(REQ-SUBFLOW-B04 불변식).
- `NewFlow` 에 `WithInputPorts`/`WithOutputPorts`(플로우 레벨) FlowOption 추가 — 노드용 `WithInputPorts`(node.go)와 패키지 내 이름 충돌하지 않도록 네이밍 주의(예: `WithFlowInputPorts`).

### 1.2 어댑터 양방향 변환 (`internal/api/service/flow_adapter.go`)

- 플로우 정의 JSON 최상위 `inputs`/`outputs` 배열을 `FlowPort` 로/에서 변환한다. 노드 변환(`normalizeReactFlowDefinition`)과 별개의 최상위 경로로 처리하여 노드의 `inputs`/`outputs` 와 혼동을 방지한다.
- `flowFromDefinition` / `flowToReactFlowConfig` 양쪽에서 플로우 포트를 보존한다(저장/로드/export-import 라운드트립).
- flow-node 의 `flow_id` 는 `NodeDef.Config["flow_id"]` 로 통과한다(NodeDef.UnmarshalJSON 의 unknown-field→Config 흡수 동작과 일관).

### 1.3 엔진 확장 — 두 옵션 비교 (핵심 설계 결정)

| 항목 | 옵션 1: 어댑터 사전 확장 (권장) | 옵션 2: 엔진 리포 주입 |
|------|-------------------------------|------------------------|
| 위치 | `FlowServiceAdapter.DeployFlow`(~304) 에서 `repo.Get` 후 평탄화하여 엔진에 전달 | `Engine.DeployFlow`(~79) 노드 생성 루프(~116) 직전 확장 |
| 엔진 변경 | 없음 (회귀 위험 최소) | `WithFlowRepository` 추가 + DeployFlow 침습 |
| 의존성 | 어댑터가 이미 `repo` 보유 → 추가 없음 | 엔진→storage 패키지 결합도 상승 |
| 선례 | 어댑터 deploy 경로 기존재 | `WithAgentManager`(options.go) 동일 패턴 존재 |
| CLI 직접 배포 | 미지원(어댑터 우회 시) → 절충 필요 | 일관 지원 |
| 권장 | ✅ 기본 채택 | CLI 서브플로우 필요 시 또는 공용 확장 함수 절충 |

- **권장안**: 옵션 1. 엔진을 "합성에 무지(subflow-agnostic)"하게 유지한다. 확장 로직을 `internal/api/service/subflow_expand.go` 공용 함수로 분리하여, 추후 CLI가 필요해지면 동일 함수를 재사용한다(엔진 침습 없이 절충).
- 엔진의 노드 생성 루프(engine.go ~116-196)와 `CreateRuntimeWires`(~199, wire.go)는 평탄화된 노드/와이어만 받으므로 변경 불필요.

### 1.4 네임스페이스 규칙

- 접두사: `subflow_<flowNodeID>_<originalNodeID>`. 와이어 ID 와 엔드포인트도 동일 재작성. 중첩 시 접두사 누적.
- flow-node ID 를 접두사에 포함 → 같은 플로우를 N번 참조해도 인스턴스 격리(상태 비공유, REQ-SUBFLOW-D03).
- `resolveFlowComponentPrefix`(engine.go ~1618) 와 결합한 컴포넌트 이름으로 모니터링 추적성 확보.

### 1.5 boundary 브리지 합성

- 두 방식(plan에서 결정):
  - **재배선(rewire)**: flow-node 핸들 부모 와이어를 서브그래프 내부 포트로 직접 연결(노드 미증가, 경계 가시성 낮음).
  - **브리지 노드**: 입력/출력 포트마다 pass-through 노드 합성(`WithBridgePorts` in/out 패턴 참고). 경계 명시·모니터링 우수, 노드 증가.
- 권장: 브리지 노드를 기본(추적성), 단순 케이스 최적화로 재배선 고려.

### 1.6 순환 검출 (`internal/api/service/cycle_detect.go`)

- 리포지토리의 플로우 정의들에서 flow-node `flow_id` 로 "플로우→플로우" 방향 그래프 구성.
- DFS(흰/회/검 색칠)로 사이클 검출. self-loop = 자기참조.
- 저장 경로(`CreateFlow`/`UpdateFlow`)와 배포 경로(`DeployFlow`) 양쪽에서 호출(REQ-SUBFLOW-E04).
- 에러에 순환 경로(플로우 id/name 시퀀스) 포함.

### 1.7 프론트

- **editorStore**: `flowInputs`/`flowOutputs` 상태 + 추가/이름/삭제 액션. 실제 편집만 dirty(LINK-001 의 select 비-dirty 패턴 준용).
- **경계 포트 UI**(`FlowBoundaryPorts.tsx`, 신규): 캔버스 좌(입력)/우(출력) 경계 렌더 + 내부 노드와 와이어 연결 핸들. React Flow 좌표계 위 오버레이.
- **포트 관리 패널**(`FlowPortPanel.tsx`, 신규): 추가/이름/삭제.
- **flow-node 핸들**: `nodeSchemas.ts` `computePortsForNode` 에 `case 'flow-node'` — `config.flow_id` 로 참조 플로우 inputs/outputs 를 받아 핸들 동적 계산(항상 최신).
- **플로우 피커**(`FlowPickerModal.tsx`, 신규): `flowService.getFlows` 재사용, 후보에서 `currentFlowId` 제외.
- **타입**: `types/flow.ts` 에 `FlowPort`, `FlowInfo.inputs/outputs`.

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

> 큰 기능이므로 백엔드 모델 → 엔진 확장 → 순환검출 → 어댑터 → 프론트 포트UI → 프론트 flow-node 순으로 분할한다. 각 마일스톤은 독립 검증 가능 단위로 둔다.

### 마일스톤 1 — 우선순위 High: 플로우 포트 모델 (백엔드)
- `pkg/flow` 플로우 레벨 포트(`FlowPort`, Flow 인터페이스 변이, defaultFlow 필드, FlowOption).
- 저장/로드/export-import 라운드트립 보존(어댑터 최상위 inputs/outputs 변환).
- 검증: flow_test.go, flow_adapter_test.go (TDD 신규).
- 의존: 없음. 이후 모든 마일스톤의 토대.

### 마일스톤 2 — 우선순위 High: flow-node 등록 + 핸들 계산
- `flow-node` 빌트인 등록(registry.go), 확장 마커 노드 정의(flow_node.go).
- `flow_id` config 통과. 핸들 = 참조 플로우 포트(백엔드 표현).
- 검증: registry_test.go.
- 의존: 마일스톤 1(플로우 포트로 핸들 계산).

### 마일스톤 3 — 우선순위 High: 순환/자기참조 검출
- `cycle_detect.go` DFS 그래프 검출. 저장+배포 경로 훅.
- 직접 자기참조 + 간접 순환 거부 + 명확 에러.
- 검증: cycle_detect_test.go (자기참조, A→B→A, A→B→C→A, 정상 DAG).
- 의존: 마일스톤 2(flow_id 그래프).

### 마일스톤 4 — 우선순위 High: 배포 인스턴스화 (서브그래프 확장)
- `subflow_expand.go`: 네임스페이스 복제 + boundary 브리지. 재귀 확장(중첩) + 안전 상한.
- 어댑터 `DeployFlow` 사전 확장 연결(옵션 1).
- 검증: subflow_expand_test.go (단일/중첩/다중 인스턴스 격리/누락 참조/상한).
- 의존: 마일스톤 1·2·3.

### 마일스톤 5 — 우선순위 Medium: 프론트 플로우 포트 UI
- editorStore 포트 상태/액션, 경계 포트 UI, 포트 관리 패널.
- 내부 노드 와이어 연결, dirty 처리.
- 검증: editorStore.test.ts, FlowBoundaryPorts.test.tsx.
- 의존: 마일스톤 1(백엔드 포트 계약).

### 마일스톤 6 — 우선순위 Medium: 프론트 flow-node + 피커
- `computePortsForNode` flow-node 케이스(참조 포트 핸들), 플로우 피커 모달(자기 제외), dangling 처리/경고.
- 검증: nodeSchemas.test.ts, FlowPickerModal.test.tsx.
- 의존: 마일스톤 2·5.

### 마일스톤 7 — 우선순위 Low(최종 목표): 통합·회귀·호환
- 단독 배포 시 포트 비활성, 기존 플로우 동작 불변 회귀, end-to-end(B 배포 → A 서브그래프 확장 → 메시지 라우팅).
- 검증: 통합 테스트 + 기존 엔진/어댑터 회귀 스위트.
- 의존: 전 마일스톤.

## 3. 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| 무한 확장(순환) | 메모리/시간 폭증, 데몬 다운 | 저장+배포 이중 순환검출(E04), 확장 깊이/노드 수 안전 상한(N01) |
| 참조 포트 변경에 따른 dangling | 부모 와이어 끊김, 조용한 메시지 유실 | 핸들 항상-최신 갱신(C03) + dangling 경고/정리(C05). 정책 강도는 OPEN Q4 |
| 네임스페이스 노드 ID 가독성 | 모니터링/로그에서 출처 불명 | `subflow_<flowNodeID>_` 접두사 + 컴포넌트 prefix 결합 표시(D06) |
| export/import flow-node 참조 정합 | import 시 끊어진 참조 | self-contained 동봉 vs id 참조 정책 결정(OPEN Q7), 누락 시 엄격 에러(F03) |
| 깊은 중첩 확장 비용/메모리 | 배포 지연·OOM | 안전 상한(N01), 중첩 깊이 한계, 확장 비용 측정 |
| 단독 실행 시 포트 비활성 처리 | 포트 dangling 오해 | 단독 배포 시 포트 비활성 명시(F01), UI 안내 |
| 엔진 침습(옵션 2 선택 시) 회귀 | 기존 배포 경로 영향 | 옵션 1(어댑터 사전 확장) 권장으로 엔진 불변 유지 |
| 패키지 내 옵션 이름 충돌 | 노드용/플로우용 `WithInputPorts` 혼동 | 플로우 레벨은 `WithFlowInputPorts` 등 구분 네이밍 |

## 4. 개발 방법론

- **Hybrid** (`.moai/config/sections/quality.yaml` development_mode=hybrid):
  - 신규 코드(FlowPort, subflow_expand, cycle_detect, flow-node, 프론트 신규 컴포넌트) = **TDD** (RED-GREEN-REFACTOR, 신규 커버리지 85%+).
  - 기존 변경(flow.go, flow_adapter.go, registry.go, editorStore.ts, nodeSchemas.ts) = **동작 보존 DDD** (ANALYZE-PRESERVE-IMPROVE, 회귀 0).
- 엔진은 옵션 1 채택 시 변경 없음 → 기존 엔진 회귀 스위트로 보호.

## 5. 검증 전략 요약

- 백엔드 단위: flow_test, flow_adapter_test, cycle_detect_test, subflow_expand_test, registry_test.
- 프론트 단위: editorStore.test, FlowBoundaryPorts.test, nodeSchemas.test, FlowPickerModal.test.
- 통합/회귀: 단독+합성 배포, 메시지 라우팅 end-to-end, 기존 플로우 불변.
- 품질 게이트: TRUST 5, LSP zero-error(run), Go `go test -race ./...` + golangci-lint, 프론트 Vitest.
