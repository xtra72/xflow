# SPEC-SUBFLOW-001 구현 계획 (plan.md)

> 플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트
> 본 문서는 PLAN 단계 산출물이다. 코드는 포함하지 않으며 기술 접근/마일스톤/위험을 정의한다.

> **구현 상태(2026-06-05, 브랜치 `feature/subflow-node`)**: M1(포트 모델)·M2(flow-node 등록/핸들)·M3(순환 검출)·M4(배포 인스턴스화) **구현 완료**. M5·M6(프론트 포트 UI/flow-node·피커)는 합성 노드 방식으로 구현 완료. 본문 마일스톤 표기는 spec 의 M1~M4(포트 모델/엔진 확장·순환검출/flow-node 프론트/경계 포트 UI) 그룹화와 매핑된다. 아래 § 1.3·§ 1.5 의 두 옵션은 **옵션 1(어댑터 사전 확장) + 직접 재배선** 으로 확정되었다.

## 1. 기술 접근

### 1.1 백엔드 — 플로우 레벨 포트 모델 (`pkg/flow`)

- **확정 구현**: `Flow` 인터페이스에 `Inputs() []Port`·`Outputs() []Port` 접근자와 `SetInputs/SetOutputs` 변이를 추가했다. `defaultFlow` 에 `inputs/outputs []Port` 필드를 두었다.
- **확정 설계 — 별도 `FlowPort` 타입 미신설, 기존 노드 포트 타입 `Port` 재사용**. 플로우 레벨 엔티티로서 정의 최상위에만 두어 노드 포트와 구분하며, 방향은 소속 목록에서 정규화한다.
- 포트는 노드가 아니므로 `Nodes()` 에 절대 포함하지 않는다(REQ-SUBFLOW-B04 불변식).
- 노드용 `WithInputPorts`(node.go)와 충돌하지 않도록 플로우 레벨 FlowOption 은 `WithFlowInputPorts`/`WithFlowOutputPorts` 로 명명했다.

### 1.2 어댑터 양방향 변환 (`internal/api/service/flow_adapter.go`)

- 플로우 정의 JSON 최상위 `inputs`/`outputs` 배열을 `FlowPort` 로/에서 변환한다. 노드 변환(`normalizeReactFlowDefinition`)과 별개의 최상위 경로로 처리하여 노드의 `inputs`/`outputs` 와 혼동을 방지한다.
- `flowFromDefinition` / `flowToReactFlowConfig` 양쪽에서 플로우 포트를 보존한다(저장/로드/export-import 라운드트립).
- flow-node 의 `flow_id` 는 `NodeDef.Config["flow_id"]` 로 통과한다(NodeDef.UnmarshalJSON 의 unknown-field→Config 흡수 동작과 일관).

### 1.3 엔진 확장 — 두 옵션 비교 → **확정: 옵션 1 (어댑터 사전 확장)**

| 항목 | 옵션 1: 어댑터 사전 확장 (권장) | 옵션 2: 엔진 리포 주입 |
|------|-------------------------------|------------------------|
| 위치 | `FlowServiceAdapter.DeployFlow`(~304) 에서 `repo.Get` 후 평탄화하여 엔진에 전달 | `Engine.DeployFlow`(~79) 노드 생성 루프(~116) 직전 확장 |
| 엔진 변경 | 없음 (회귀 위험 최소) | `WithFlowRepository` 추가 + DeployFlow 침습 |
| 의존성 | 어댑터가 이미 `repo` 보유 → 추가 없음 | 엔진→storage 패키지 결합도 상승 |
| 선례 | 어댑터 deploy 경로 기존재 | `WithAgentManager`(options.go) 동일 패턴 존재 |
| CLI 직접 배포 | 미지원(어댑터 우회 시) → 절충 필요 | 일관 지원 |
| 권장 | ✅ 기본 채택 | CLI 서브플로우 필요 시 또는 공용 확장 함수 절충 |

- **확정**: 옵션 1 채택. 엔진을 "합성에 무지(subflow-agnostic)"하게 유지한다. 확장 로직은 `internal/api/service/subflow_expand.go` `ExpandSubflows` 공용 함수로 분리했다(추후 CLI 가 필요하면 동일 함수 재사용 가능).
- 엔진(`engine.go`, `options.go`)은 **변경 없음**. `WithFlowRepository` 미추가. 어댑터 `DeployFlow` 순서: cycle-detect → `ExpandSubflows` → `StripBoundaryWires` → `engine.DeployFlow`.

### 1.4 네임스페이스 규칙

- 접두사: `subflow_<flowNodeID>_<originalNodeID>`. 와이어 ID 와 엔드포인트도 동일 재작성. 중첩 시 접두사 누적.
- flow-node ID 를 접두사에 포함 → 같은 플로우를 N번 참조해도 인스턴스 격리(상태 비공유, REQ-SUBFLOW-D03).
- `resolveFlowComponentPrefix`(engine.go ~1618) 와 결합한 컴포넌트 이름으로 모니터링 추적성 확보.

### 1.5 boundary 브리지 합성 → **확정: 직접 재배선(direct rewire)**

- **확정**: 브리지 노드 미합성. 엔드포인트 해석(`inMap`/`outMap` 카르테시안)으로 flow-node 핸들 ↔ 참조 플로우 경계 포트를 와이어로 직접 합성한다(`subflow_expand.go`). 노드 수 최소·회귀 위험 최소.
- 일반 노드/두 flow-node 직접 연결/입력→출력 passthrough 를 단일 해석 규칙으로 일관 처리한다.
- 브리지 노드 방식(pass-through 노드 합성)은 채택하지 않았다.

### 1.6 순환 검출 (`internal/api/service/subflow_cycle.go`)

- 리포지토리의 플로우 정의들에서 flow-node `flow_id` 로 "플로우→플로우" 방향 그래프 구성.
- DFS(흰/회/검 색칠)로 사이클 검출. self-loop = 자기참조.
- 저장 경로(`CreateFlow` flow_adapter.go:82 / `UpdateFlow` :245)와 배포 경로(`DeployFlow` :359) 양쪽에서 호출(REQ-SUBFLOW-E04).
- 에러에 순환 경로(플로우 id 시퀀스) 포함. 깊이 안전 상한 `maxFlowReferenceDepth=8`.

### 1.7 프론트

- **확정 구현 — 합성 노드 방식**(오버레이/모달 아님):
- **editorStore**(`editorStore.ts`): `flowInputs`/`flowOutputs` 상태 + `addFlowPort`/`renameFlowPort`/`removeFlowPort` 액션. 실제 편집만 dirty. 저장 시 `realNodesOnly` 로 합성 노드를 `nodes` 에서 제외(포트는 정의 inputs/outputs 로만 영속). 합성 노드 선택 가드.
- **경계/영역 노드**(`web/src/lib/flow/boundary.ts`, `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx`): 합성 React Flow 노드 `__flow_input__`(좌)/`__flow_output__`(우) + 영역 표시 노드 `__flow_area__`. 위치는 실노드 바운딩 박스에서 파생(노드 이동 시 따라옴). `draggable:false`/`deletable:false`.
- **포트 관리 패널**(`FlowPortPanel.tsx`, 신규): 추가/이름/삭제(툴바 "플로우 포트").
- **flow-node 핸들**: `nodeSchemas.ts` `computePortsForNode` `case 'flow-node'` — `config.flow_id` 의 참조 플로우 inputs/outputs 로 핸들 동적 계산(`subflowPorts.ts` 비정규화 캐시, 항상 최신).
- **플로우 피커**: `nodeSchemas.ts` 의 `flow_picker` 필드 + `flowService.getFlows` 재사용, 후보에서 현재 편집 플로우(자기) 제외.
- **타입**: `types/flow.ts` `FlowInfo.inputs/outputs`, `boundary.ts` `FlowPortDef`.

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

> 큰 기능이므로 백엔드 모델 → 엔진 확장 → 순환검출 → 어댑터 → 프론트 포트UI → 프론트 flow-node 순으로 분할한다. 각 마일스톤은 독립 검증 가능 단위로 둔다.

### 마일스톤 1 — 우선순위 High: 플로우 포트 모델 (백엔드) — ✅ 구현됨
- `pkg/flow` 플로우 레벨 포트(`FlowPort`, Flow 인터페이스 변이, defaultFlow 필드, FlowOption).
- 저장/로드/export-import 라운드트립 보존(어댑터 최상위 inputs/outputs 변환).
- 검증: flow_test.go, flow_adapter_test.go (TDD 신규).
- 의존: 없음. 이후 모든 마일스톤의 토대.

### 마일스톤 2 — 우선순위 High: flow-node 등록 + 핸들 계산 — ✅ 구현됨
- `flow-node` 빌트인 등록(registry.go), 확장 마커 노드 정의(flow_node.go).
- `flow_id` config 통과. 핸들 = 참조 플로우 포트(백엔드 표현).
- 검증: registry_test.go.
- 의존: 마일스톤 1(플로우 포트로 핸들 계산).

### 마일스톤 3 — 우선순위 High: 순환/자기참조 검출 — ✅ 구현됨
- `cycle_detect.go` DFS 그래프 검출. 저장+배포 경로 훅.
- 직접 자기참조 + 간접 순환 거부 + 명확 에러.
- 검증: cycle_detect_test.go (자기참조, A→B→A, A→B→C→A, 정상 DAG).
- 의존: 마일스톤 2(flow_id 그래프).

### 마일스톤 4 — 우선순위 High: 배포 인스턴스화 (서브그래프 확장) — ✅ 구현됨
- `subflow_expand.go`: 네임스페이스 복제 + **직접 재배선**(브리지 노드 없음). 재귀 확장(중첩) + 안전 상한(깊이 8/노드 5000) + dangling 드랍.
- 어댑터 `DeployFlow` 사전 확장 연결(옵션 1): cycle-detect → ExpandSubflows → StripBoundaryWires → engine.DeployFlow.
- `pkg/flow/boundary.go` 센티넬 규약·`StripBoundaryWires`·`RebuildFlow` 동반.
- 검증: subflow_expand_test.go (단일/중첩/다중 인스턴스 격리/누락 참조/상한).
- 의존: 마일스톤 1·2·3.

### 마일스톤 5 — 우선순위 Medium: 프론트 플로우 포트 UI — ✅ 구현됨
- editorStore 포트 상태/액션, **합성 경계/영역 노드**(`boundary.ts`, `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx`), 포트 관리 패널(`FlowPortPanel.tsx`).
- 내부 노드 와이어 연결(센티넬), dirty 처리, 합성 노드 영속 제외/선택 가드.
- 검증: editorStore.test.ts, boundary.test.ts, FlowPortPanel.test.tsx.
- 의존: 마일스톤 1(백엔드 포트 계약).

### 마일스톤 6 — 우선순위 Medium: 프론트 flow-node + 피커 — ✅ 구현됨
- `computePortsForNode` flow-node 케이스(참조 포트 핸들, `subflowPorts.ts` 비정규화), `flow_picker` 필드(자기 제외), dangling 처리(미해결 핸들 비렌더로 사전 억제).
- 검증: subflowPorts.test.ts, nodeSchemas/CustomNode 검증.
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
| export/import flow-node 참조 정합 | import 시 끊어진 참조 | **확정: id 참조 유지(동봉 안 함)**. 누락 시 배포 엄격 에러(F03) |
| 깊은 중첩 확장 비용/메모리 | 배포 지연·OOM | 안전 상한(N01), 중첩 깊이 한계, 확장 비용 측정 |
| 단독 실행 시 포트 비활성 처리 | 포트 dangling 오해 | 단독 배포 시 포트 비활성 명시(F01), UI 안내 |
| 엔진 침습(옵션 2 선택 시) 회귀 | 기존 배포 경로 영향 | **해소: 옵션 1(어댑터 사전 확장) 채택 — 엔진/options.go 불변** |
| 패키지 내 옵션 이름 충돌 | 노드용/플로우용 `WithInputPorts` 혼동 | 플로우 레벨은 `WithFlowInputPorts` 등 구분 네이밍 |

## 4. 개발 방법론

- **Hybrid** (`.moai/config/sections/quality.yaml` development_mode=hybrid):
  - 신규 코드(FlowPort, subflow_expand, cycle_detect, flow-node, 프론트 신규 컴포넌트) = **TDD** (RED-GREEN-REFACTOR, 신규 커버리지 85%+).
  - 기존 변경(flow.go, flow_adapter.go, registry.go, editorStore.ts, nodeSchemas.ts) = **동작 보존 DDD** (ANALYZE-PRESERVE-IMPROVE, 회귀 0).
- 엔진은 옵션 1 채택 시 변경 없음 → 기존 엔진 회귀 스위트로 보호.

## 5. 검증 전략 요약

- 백엔드 단위: flow round-trip, flow_adapter_test, subflow_cycle_test, subflow_expand_test, registry.
- 프론트 단위: editorStore.test, boundary.test, subflowPorts.test, FlowPortPanel.test.
- 통합/회귀: 단독+합성 배포, 메시지 라우팅 end-to-end, 기존 플로우 불변.
- 품질 게이트: TRUST 5, LSP zero-error(run), Go `go test -race ./...` + golangci-lint, 프론트 Vitest.
