---
id: SPEC-SUBFLOW-001
title: "플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트"
version: "1.1.0"
status: implemented
created: "2026-06-04"
updated: "2026-06-05"
author: "xtra"
priority: high
related_specs:
  - SPEC-FLOW-001
  - SPEC-FLOW-002
  - SPEC-WIRE-001
  - SPEC-ENGINE-001
  - SPEC-WEB-001
  - SPEC-AGENT-NODE-001
tags:
  - subflow
  - flow-node
  - composition
  - flow-port
  - namespace
  - cycle-detection
  - editor
  - engine
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-04 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-06-05 | xtra | 구현 완료 동기화 — OPEN QUESTIONS 1~5 확정, 최종 아키텍처 반영(센티넬 경계 와이어 규약·직접 재배선·합성 경계/영역 노드 UI), 구현 상태(M1~M4 완료) 기록 |

> **구현 상태(Implementation Status)** — 2026-06-05, 브랜치 `feature/subflow-node`.
> 본 기능은 백엔드·프론트 전 마일스톤이 구현 완료되었다.
> - **M1 — 플로우 레벨 포트 모델**: 구현됨. `pkg/flow/flow.go`(`Inputs()/Outputs()`, `WithFlowInputPorts/WithFlowOutputPorts`, `SetInputs/SetOutputs`), `pkg/flow/serialize.go`(정의 최상위 `inputs`/`outputs` 라운드트립, `normalizeFlowPortIDs`).
> - **M2 — 엔진 확장/순환 검출**: 구현됨. `internal/api/service/subflow_expand.go`(`ExpandSubflows`), `internal/api/service/subflow_cycle.go`(`DetectFlowReferenceCycle`), `pkg/flow/boundary.go`(센티넬·`StripBoundaryWires`·`RebuildFlow`), `internal/api/service/flow_adapter.go`(DeployFlow 사전 확장 + 저장/배포 순환 훅).
> - **M3 — flow-node 프론트**: 구현됨. `web/src/config/nodeSchemas.ts`(`flow-node` 스키마 + `computePortsForNode`), `web/src/lib/flow/subflowPorts.ts`, `internal/node/flow_node.go`·`internal/node/registry.go`(빌트인 등록).
> - **M4 — 경계 포트 UI**: 구현됨. `web/src/lib/flow/boundary.ts`(합성 경계/영역 노드, 센티넬, 포트 영속), `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx`, `FlowPortPanel.tsx`, `web/src/stores/editorStore.ts`.
>
> 본문 요구사항/인수 기준은 보존하되, 구현으로 충족된 항목은 "구현됨"으로 주석한다. 신규 요구사항은 추가하지 않는다.

# SPEC-SUBFLOW-001: 플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트

## 1. 개요

### 1.1 목적

한 플로우를 다른 플로우 안에서 **플로우 노드(flow-node)** 로 참조하여 재사용·합성하는 기능을 제공한다. 이를 위해 두 가지 신규 개념을 도입한다.

1. **플로우 레벨 입출력 포트**: 플로우 자체에 입력/출력 포트를 부여한다(노드가 아닌 **플로우 레벨 엔티티**). 플로우 편집 화면에서 포트를 추가·이름 지정·삭제한다.
2. **플로우 노드(flow-node)**: 다른 플로우를 참조하는 특수 노드. 참조 플로우의 입출력 포트가 그 플로우 노드의 핸들(포트)로 표시된다. 플로우 노드에 연결하면 메시지가 참조 플로우의 입력 포트로 들어가고 참조 플로우의 출력 포트에서 나온다.

자기 플로우 참조(자기참조)와 간접 순환 참조(A→B→A)는 전체 금지한다.

### 1.2 핵심 모델 (확정 — 반드시 준수)

> **중요**: 본 SPEC은 아래 4개 확정 설계 결정을 따른다. 라이브 플로우 간 메시지 버스를 신설하지 않는다.

#### 결정 1 — 실행 모델 = 인스턴스화(배포 시 서브그래프 확장)

플로우 간 런타임 메시지 버스가 없으므로 라이브 참조는 불가하다. 부모 플로우 배포 시, 각 flow-node를 참조 플로우의 노드+와이어를 **네임스페이스 복제**한 격리 인스턴스로 확장하고, flow-node의 핸들 ↔ 참조 플로우의 플로우 포트를 브리지한다. (Node-RED 서브플로우 모델.) 각 flow-node = 독립 인스턴스(상태 비공유).

#### 결정 2 — 참조 신선도 = 항상 최신

flow-node는 매 배포 시점의 참조 플로우 **현재 정의**를 반영한다(템플릿/라이브러리 방식). 스냅샷 고정이 아니다. 참조 플로우를 수정하고 부모를 재배포하면 최신 정의가 확장된다.

#### 결정 3 — 순환 참조 = 전체 금지

직접 자기참조 + 간접 순환(A→B→A, A→B→C→A 등) 모두 **저장 시점과 배포 시점**에 검출하여 거부한다. 인스턴스화 무한 확장을 원천 차단한다.

#### 결정 4 — 플로우 포트 UI = 캔버스 경계 포트 + 관리 패널

입력 포트는 캔버스 **좌측 경계**, 출력 포트는 캔버스 **우측 경계**에 표시한다. 내부 노드와 와이어로 연결한다. 포트 추가/이름/삭제를 위한 관리 패널(툴바 "플로우 포트" 패널)을 제공한다. 포트는 노드가 아닌 플로우 레벨 엔티티이다.

> **구현 시 확정된 추가 결정(OPEN QUESTIONS 1~5 → DECIDED)** — Section 5.9 참조. 요약:
> 1. **확장 위치 = 어댑터 사전 확장**: `FlowServiceAdapter.DeployFlow` 가 `engine.DeployFlow` 호출 전에 확장한다. 엔진은 비침습(리포지토리 주입 없음).
> 2. **브리지 방식 = 직접 재배선(direct rewire)**: 경계 브리지 노드를 합성하지 않고, 엔드포인트 해석(inMap/outMap 카르테시안)으로 와이어만 직접 합성한다.
> 3. **상한 = 최대 중첩 깊이 8, 최대 확장 노드 5000**(초과 시 에러).
> 4. **dangling(참조 포트 제거) = 경고 + 해당 와이어 정리(드랍)**.
> 5. **export = `flow_id` 참조 유지**(참조 플로우 동봉 안 함).

### 1.3 배경

- xflow 는 IoT FBP(Flow Based Programming) 플랫폼이며, 웹 에디터(React Flow)에서 노드와 와이어로 플로우를 구성한다.
- `pkg/flow/flow.go` 의 `Flow` 인터페이스와 `defaultFlow` 구현체는 id/name/description/state/nodes/wires/config/metadata 를 보유했으나 플로우 레벨 포트가 없었다. **구현됨**: `Flow` 인터페이스에 `Inputs() []Port`(flow.go:67)·`Outputs() []Port`(flow.go:71), `defaultFlow` 에 `inputs/outputs []Port`(flow.go:147) 와 `SetInputs/SetOutputs`(flow.go:564,572), FlowOption `WithFlowInputPorts/WithFlowOutputPorts`(flow.go:286,295) 를 추가했다. 별도 `FlowPort` 타입을 신설하지 않고 기존 노드 포트 타입 `Port` 를 재사용한다.
- 노드 ID는 자유 문자열이며(`NodeDef.ID string`), 와이어는 문자열 ID(`Wire.SourceNodeID/SourcePort/TargetNodeID/TargetPort`)로 연결한다. 따라서 네임스페이싱(접두사 부여)으로 인스턴스 격리가 가능하다.
- 엔진(`internal/engine/engine.go`)의 `Engine` 구조체는 `flows map[string]*flowRuntime` 만 보유하고 리포지토리 참조가 없다. 단, 이미 `agentManager agent.Manager` 를 배포 시 참조 검증용으로 보유하며(`WithAgentManager` EngineOption, options.go), 이는 "엔진에 배포 시점 외부 의존성 주입"의 선례이다.
- `internal/api/service/flow_adapter.go` 의 `FlowServiceAdapter` 는 `repo storage.FlowRepository` 를 보유하고 `DeployFlow(ctx, id)` 에서 `repo.Get(ctx, id)` 로 플로우 정의를 조회한다. 따라서 **어댑터가 배포 직전 서브플로우를 사전 확장**할 수 있다.
- `internal/storage/repository.go` 의 `FlowRepository` 인터페이스는 `Get/List/Save/Delete/Close` 를 제공한다 → 참조 플로우 정의 조회 및 순환 검출용 그래프 탐색이 가능하다.

### 1.4 범위

**포함:**
- 백엔드: `pkg/flow` 에 플로우 레벨 입출력 포트 모델(`Inputs()/Outputs()` 또는 동등) 추가. 어댑터 양방향 변환(정의 최상위 `inputs`/`outputs`). flow-node 빌트인 등록. 배포 시 서브그래프 확장(네임스페이스 복제 + boundary 브리지). 순환/자기참조 검출.
- 프론트: 캔버스 경계 포트 UI(좌 입력/우 출력) + 포트 관리 패널, flow-node 노드(참조 플로우 포트로 핸들 동적 계산), 플로우 피커 모달(자기 플로우 제외), editorStore 플로우 포트 상태.

**제외:**
- 라이브(런타임) 플로우 간 메시지 버스 (명시적 금지 — 인스턴스화 모델만 사용).
- 참조 스냅샷 고정/버전 핀 (항상 최신 정책).
- 서브플로우 인스턴스별 파라미터/환경 변수 오버라이드 (향후 SPEC, OPEN QUESTION).
- 중첩 깊이 제한 정책의 정밀 튜닝(기본 안전 상한만 — Section 5 참조).

## 2. 환경

| 항목 | 상세 |
|------|------|
| 백엔드 런타임 | Go 1.23+ |
| 백엔드 대상 모듈 | `pkg/flow` (Flow 포트, Wire, NodeDef), `internal/engine` (DeployFlow 확장 또는 사전 확장 소비), `internal/node` (flow-node 등록/확장), `internal/api/service` (flow_adapter 변환·사전 확장), `internal/storage` (FlowRepository 그래프 탐색) |
| 프론트 런타임 | TypeScript 5.9+, React 19, `@xyflow/react` (React Flow) |
| 프론트 대상 모듈 | `web/src/types/flow.ts`, `web/src/stores/editorStore.ts`, `web/src/config/nodeSchemas.ts` (`computePortsForNode`), `web/src/services/api/flowService.ts` (`getFlows`), 캔버스 경계 포트 UI(신규), 플로우 피커 모달(신규), 포트 관리 패널(신규) |
| 테스트 | Go `testing`+`testify`, 프론트 Vitest + Testing Library |
| 개발 방법론 | Hybrid (신규 = TDD, 기존 변경 = 동작 보존 DDD) |

## 3. 가정

- **A1** (구현 반영): `pkg/flow` 의 `Flow`/`defaultFlow` 에 플로우 레벨 포트를 신설했다. **확정 설계 — 별도 `FlowPort` 타입을 만들지 않고 기존 노드 포트 타입 `Port` 를 재사용한다**(`Inputs() []Port` / `Outputs() []Port`). 플로우 레벨 엔티티로서 정의 최상위에만 두며(노드 포트와 위치로 구분), 직렬화 시 방향은 소속 목록(inputs=input, outputs=output)에서 정규화한다(`normalizeFlowPortDirection`, `normalizeFlowPortIDs`).
- **A2**: 노드 ID/와이어 엔드포인트는 자유 문자열이므로 네임스페이스 접두사 부여로 인스턴스 격리가 가능하다. 동일 부모 내 여러 flow-node가 같은 플로우를 참조해도 접두사가 flow-node ID를 포함하면 충돌하지 않는다.
- **A3** (확정): 어댑터(`FlowServiceAdapter`)는 `repo storage.FlowRepository` 를 보유하므로 배포 직전 참조 플로우를 조회·확장한다. **채택된 설계 = 어댑터 사전 확장**(엔진 비침습). 엔진에 `WithFlowRepository` 를 주입하는 대안은 채택하지 않았다 — 엔진 코드는 평탄화된 노드/와이어만 보며 서브플로우에 무지하다(회귀 위험 최소).
- **A4**: 참조 플로우는 별도 저장 엔티티이며 단독으로도 배포 가능하다. 포트만 있고 flow-node가 아닌 기존 플로우 정의는 영향받지 않는다.
- **A5**: 순환 검출은 flow-node가 만드는 "플로우→플로우" 참조 그래프에 대해 수행한다. 이 그래프는 리포지토리의 플로우 정의들(각 flow-node의 `flow_id`)로 구성된다.
- **A6**: 확장은 배포 시점에만 일어나며 저장된 부모 정의 자체는 flow-node를 그대로 보존한다(확장 결과를 영속화하지 않는다 — 결정 2 "항상 최신" 보장).
- **A7**: 참조 플로우 포트와 flow-node 핸들의 동기화는 매 편집/로드 시 `getFlows`/참조 플로우 정의 조회로 갱신한다. 참조 포트가 사라지면 해당 핸들에 걸린 부모 와이어는 dangling 으로 처리한다(경고/정리).

## 4. 요구사항 (EARS)

### 4.1 그룹 A — 플로우 레벨 포트 (백엔드 모델 + 영속)

**REQ-SUBFLOW-A01**: 플로우 입출력 포트 모델
시스템은 **항상** `pkg/flow` 의 Flow 모델에 플로우 레벨 입력 포트 목록과 출력 포트 목록을 보유해야 한다(노드 포트와 구분되는 플로우 레벨 엔티티). 각 포트는 안정적 식별자(id)와 이름(name), 방향(input/output)을 가진다.

**REQ-SUBFLOW-A02**: 포트 추가
**WHEN** 사용자가 플로우 편집 화면에서 입력 또는 출력 포트를 추가하면, **THEN** 시스템은 해당 방향의 포트 목록에 고유 id 와 사용자 지정 이름을 가진 포트를 추가해야 한다.

**REQ-SUBFLOW-A03**: 포트 이름 변경
**WHEN** 사용자가 플로우 포트의 이름을 변경하면, **THEN** 시스템은 포트 id 를 유지한 채 이름만 갱신해야 한다(id 불변 → 내부 와이어/외부 flow-node 핸들 연속성 보존).

**REQ-SUBFLOW-A04**: 포트 삭제
**WHEN** 사용자가 플로우 포트를 삭제하면, **THEN** 시스템은 해당 포트를 목록에서 제거하고, 그 포트를 엔드포인트로 사용하던 내부 와이어를 함께 제거(또는 dangling 처리)해야 한다.

**REQ-SUBFLOW-A05**: 저장/로드 보존
시스템은 **항상** 플로우 정의의 저장 및 재로드 후에도 입출력 포트 목록(id/name/방향)을 변경 없이 유지해야 한다.

**REQ-SUBFLOW-A06**: export/import 보존
**WHEN** 플로우를 export 하고 다시 import 할 때, **THEN** 시스템은 입출력 포트 목록을 유지해야 한다.

**REQ-SUBFLOW-A07**: 정의 최상위 표현
시스템은 **항상** 플로우 정의 JSON 최상위에 `inputs`/`outputs` 배열로 플로우 레벨 포트를 표현하고, 어댑터에서 양방향 변환해야 한다(노드의 `inputs`/`outputs` 와 혼동되지 않게 플로우 정의 최상위에 둔다).

### 4.2 그룹 B — 플로우 포트 UI (캔버스 경계 + 관리 패널)

**REQ-SUBFLOW-B01**: 캔버스 경계 표시
시스템은 **항상** 입력 포트를 캔버스 좌측 경계에, 출력 포트를 캔버스 우측 경계에 표시해야 한다.

**REQ-SUBFLOW-B02**: 내부 노드와 와이어 연결
시스템은 **항상** 입력 경계 포트를 소스로, 출력 경계 포트를 싱크로 하여 내부 노드와 와이어 연결을 허용해야 한다(경계 포트는 boundary 엔드포인트이며 노드가 아니다).

**REQ-SUBFLOW-B03**: 포트 관리 패널
시스템은 **항상** 포트 추가/이름 변경/삭제를 수행할 수 있는 관리 패널을 제공해야 한다.

**REQ-SUBFLOW-B04**: 포트 비-노드 불변식
시스템은 **항상** 플로우 포트를 노드 목록(nodes)에 포함시키지 않아야 한다. 포트는 플로우 레벨 엔티티로만 표현한다.

**REQ-SUBFLOW-B05**: 편집만 dirty 처리
시스템은 **항상** 플로우 포트의 실제 편집(추가/이름/삭제)만 dirty(미저장) 상태로 표시해야 한다. 단순 선택만으로는 dirty 가 되지 않아야 한다.

### 4.3 그룹 C — flow-node 노드 (참조 + 핸들)

**REQ-SUBFLOW-C01**: 다른 플로우 참조
**WHEN** 사용자가 flow-node 를 생성하고 참조 플로우를 지정하면, **THEN** 시스템은 flow-node 의 설정에 참조 플로우 식별자(`flow_id`)를 저장해야 한다.

**REQ-SUBFLOW-C02**: 핸들 = 참조 플로우 포트
시스템은 **항상** flow-node 의 입력 핸들을 참조 플로우의 입력 포트로, 출력 핸들을 참조 플로우의 출력 포트로 표시해야 한다(핸들 이름 = 참조 플로우 포트 이름).

**REQ-SUBFLOW-C03**: 핸들 항상 최신
**WHEN** 참조 플로우의 포트가 변경된 뒤 부모 플로우를 다시 열거나 갱신하면, **THEN** 시스템은 flow-node 핸들을 참조 플로우의 현재 포트로 갱신해야 한다(스냅샷 고정 없음).

**REQ-SUBFLOW-C04**: 플로우 피커 자기 제외
**WHEN** 사용자가 flow-node 의 참조 플로우를 선택하는 피커를 열면, **THEN** 시스템은 현재 편집 중인 플로우(자기 자신)를 후보 목록에서 제외해야 한다.

**REQ-SUBFLOW-C05**: 사라진 포트 dangling 처리
**IF** 참조 플로우에서 어떤 포트가 삭제되어 flow-node 핸들이 사라지면, **THEN** 시스템은 그 핸들에 연결된 부모 플로우 와이어를 dangling 으로 표시(경고)하고 정리 경로를 제공해야 한다.

### 4.4 그룹 D — 배포 인스턴스화 (서브그래프 확장 + 브리지)

**REQ-SUBFLOW-D01**: 서브그래프 네임스페이스 확장
**WHEN** flow-node 를 포함한 부모 플로우를 배포할 때, **THEN** 시스템은 각 flow-node 를 참조 플로우의 노드+와이어를 네임스페이스 접두사로 복제한 격리 인스턴스로 확장해야 한다.

**REQ-SUBFLOW-D02**: boundary 브리지 합성
**WHEN** 서브그래프를 확장할 때, **THEN** 시스템은 flow-node 의 입력 핸들 ↔ 참조 플로우 입력 포트, flow-node 의 출력 핸들 ↔ 참조 플로우 출력 포트를 와이어로 브리지해야 한다(부모 와이어가 flow-node 핸들로 보낸 메시지가 서브그래프 입력 포트로 라우팅되고, 서브그래프 출력 포트의 메시지가 부모로 라우팅되도록).

**REQ-SUBFLOW-D03**: 인스턴스 독립성
시스템은 **항상** 동일 부모 내 여러 flow-node(같은 플로우를 참조하더라도)를 상태 비공유 독립 인스턴스로 확장해야 한다(네임스페이스 접두사에 flow-node ID 포함).

**REQ-SUBFLOW-D04**: 항상 최신 반영
**WHEN** 부모 플로우를 배포할 때, **THEN** 시스템은 매 배포 시점의 참조 플로우 현재 정의를 확장해야 한다(저장된 부모 정의에는 flow-node 가 보존되고, 확장 결과는 영속화하지 않는다).

**REQ-SUBFLOW-D05**: 메시지 라우팅 경로
시스템은 **항상** flow-node 입력 핸들로 들어온 메시지가 서브그래프 내부를 거쳐 flow-node 출력 핸들로 나가도록 라우팅해야 한다.

**REQ-SUBFLOW-D06**: 확장 노드 식별 가독성
시스템은 **항상** 확장된 서브그래프 노드의 ID/컴포넌트 이름이 어떤 flow-node 인스턴스에서 유래했는지 식별 가능하도록 네임스페이스 규칙을 적용해야 한다(모니터링·로깅 추적성).

### 4.5 그룹 E — 순환/자기참조 금지

**REQ-SUBFLOW-E01**: 직접 자기참조 거부
**IF** flow-node 의 `flow_id` 가 자신이 속한 플로우의 id 와 같으면, **THEN** 시스템은 저장 및 배포를 거부하고 명확한 에러를 반환해야 한다.

**REQ-SUBFLOW-E02**: 간접 순환 거부
**IF** flow-node 참조 그래프에 순환(A→B→A, A→B→C→A 등)이 존재하면, **THEN** 시스템은 저장 및 배포를 거부하고 순환 경로를 포함한 에러를 반환해야 한다.

**REQ-SUBFLOW-E03**: 무한 확장 방지
시스템은 **항상** 순환이 검출된 구성의 배포 인스턴스화를 수행하지 않아야 한다(무한 서브그래프 확장 방지).

**REQ-SUBFLOW-E04**: 검출 시점 이원화
시스템은 **항상** 순환/자기참조 검출을 저장 시점과 배포 시점 양쪽에서 수행해야 한다(저장 후 참조 대상이 바뀌어 순환이 생기는 경우를 배포 시점에 재검출).

### 4.6 그룹 F — 라이프사이클/호환

**REQ-SUBFLOW-F01**: 포트 보유 플로우 단독 배포
시스템은 **항상** 플로우 레벨 포트를 가진 플로우를 단독으로도 배포 가능하게 해야 한다. 단독 배포 시 포트는 비활성(엔드포인트 dangling)으로 처리되며 내부 노드 실행에는 영향을 주지 않는다.

**REQ-SUBFLOW-F02**: 기존 플로우 동작 불변
시스템은 **항상** 플로우 레벨 포트가 없고 flow-node 도 없는 기존 플로우 정의를 정상 로드·배포·실행하며 기존과 동일하게 동작시켜야 한다(회귀 0).

**REQ-SUBFLOW-F03**: 누락 참조 처리
**IF** flow-node 의 `flow_id` 가 가리키는 플로우가 저장소에 없으면, **THEN** 시스템은 배포를 거부하거나 명확한 에러로 안내해야 한다(누락 에이전트 참조의 관용 처리와는 달리, 서브그래프 확장은 정의가 반드시 필요하므로 엄격 처리).

### 4.7 비기능 요구사항

**REQ-SUBFLOW-N01**: 확장 비용 한계
시스템은 **항상** 깊은 중첩 시에도 확장 깊이/노드 수에 안전 상한을 두어 메모리·시간 폭증을 방지해야 한다(상한 초과 시 명확한 에러).

**REQ-SUBFLOW-N02**: 라우팅 동작 보존
시스템은 **항상** 확장된 서브그래프 내부 노드/와이어가 일반 플로우와 동일한 라우팅·큐·카운터 의미로 동작하도록 해야 한다(서브그래프 내부도 일반 와이어와 동일).

## 5. 명세

### 5.1 데이터 계약 — 플로우 레벨 포트

플로우 정의 JSON 최상위:

| 키 | 타입 | 기본값 | 의미 |
|----|------|--------|------|
| `inputs` | `FlowPort[]` | `[]` | 플로우 레벨 입력 포트 목록. 캔버스 좌측 경계. |
| `outputs` | `FlowPort[]` | `[]` | 플로우 레벨 출력 포트 목록. 캔버스 우측 경계. |

플로우 레벨 포트 구조 (**확정 — 기존 노드 포트 타입 `flow.Port` 재사용**, 별도 `FlowPort` 타입 미신설):

| 필드 | 타입 | 의미 |
|------|------|------|
| `id` | string | 안정적 식별자(이름 변경에도 불변). 직렬화 시 누락되면 `normalizeFlowPortIDs` 가 기본값 부여. |
| `name` | string | 표시·연결 이름. flow-node 핸들 이름으로 사용. |
| `direction` | `"input"` \| `"output"` | 방향. 소속 목록(inputs/outputs)에서 정규화하여 결정(명시 불필요). |

> 노드의 `inputs`/`outputs`(NodeDef 내부)와 구분하기 위해 플로우 레벨 포트는 **정의 최상위**에만 둔다. 어댑터는 노드 변환(`normalizeReactFlowDefinition`)과 별개로 최상위 `inputs`/`outputs` 를 통과·변환한다(`serialize.go`: 직렬화 시 `nil` → `[]` 보장, 역직렬화 시 id/방향 정규화 — flow.go round-trip 보존).

#### 5.1.1 경계 와이어 센티넬 규약 (구현 — `pkg/flow/boundary.go`)

내부 노드가 플로우 자신의 입출력 포트와 연결될 때는, 포트가 노드가 아니므로(REQ-SUBFLOW-B04) **예약된 센티넬 노드 ID** 를 와이어 엔드포인트로 사용한다.

| 규약 | 값 | 의미 |
|------|----|------|
| `FlowInputBoundaryID` | `"__flow_input__"` | 플로우 입력 포트 → 내부 노드 연결: `SourceNodeID = "__flow_input__"`, `SourcePort = <입력 포트 이름>`. |
| `FlowOutputBoundaryID` | `"__flow_output__"` | 내부 노드 → 플로우 출력 포트 연결: `TargetNodeID = "__flow_output__"`, `TargetPort = <출력 포트 이름>`. |

- `IsBoundaryWire(w)` 가 와이어가 센티넬 엔드포인트를 갖는지 판별한다.
- 핸들 id = 포트 이름.
- **단독 배포 시**: 센티넬은 실제 노드가 아니므로 외부 카운터파트가 없다. `StripBoundaryWires(f)` 가 경계 와이어를 제거한 복사본을 반환하여 엔진 dangling/블로킹을 방지한다(REQ-SUBFLOW-F01).
- **서브플로우 확장 시**: 경계 와이어는 제거하지 않고 부모 와이어로 직접 재배선(Section 5.4)한다.
- `RebuildFlow(base, nodes, wires)` 는 부모 정체성(id/포트/설정)을 보존한 채 노드/와이어만 교체하여 확장 결과를 단일 평탄화 플로우로 조립한다(네임스페이스 ID 훼손 방지).

### 5.2 데이터 계약 — flow-node 노드

| 키 | 타입 | 위치 | 의미 |
|----|------|------|------|
| `type` | string | NodeDef.Type | `"flow-node"` 고정. |
| `flow_id` | string | NodeDef.Config["flow_id"] | 참조 플로우 id. 자기 id 금지. |

- flow-node 의 입출력 핸들은 **참조 플로우의 `inputs`/`outputs` 로부터 동적 계산**한다(저장 시점 핸들이 아니라 매 로드/배포 시점 참조 정의 기준 — 결정 2).
  - **프론트**(`web/src/config/nodeSchemas.ts` `computePortsForNode`, `case 'flow-node'`): `flow_id` 선택 시 참조 플로우 정의를 조회해 비정규화(denormalize)한 표시 전용 캐시로 핸들을 동적 렌더(`web/src/lib/flow/subflowPorts.ts`). 미해결(미선택/포트 0개)이면 핸들 없이 둔다.
  - **백엔드**: 배포 시 실제 참조 플로우 정의에서 재해석한다(항상 최신).
- 빌트인 등록(구현 — `internal/node/registry.go:121`): `{"flow-node", NewFlowNodePlaceholder, "composition", "다른 플로우를 참조하는 서브플로우 노드 (배포 시 확장됨)"}`. flow-node 는 직접 인스턴스화 대신 **배포 시 확장으로 소비**되는 특수 노드이며, `NewFlowNodePlaceholder`(`internal/node/flow_node.go`)는 **안전망 팩토리**로서 확장이 누락된 채 엔진에 도달하면 즉시 에러를 반환한다(정상 경로에서는 호출되지 않음).

### 5.3 네임스페이스 규칙

확장 시 서브그래프 노드/와이어 엔드포인트에 접두사를 부여하여 격리한다.

```
subflow_<flowNodeID>_<originalNodeID>
```

- `<flowNodeID>`: 부모 내 flow-node 노드의 ID(인스턴스 식별 → 같은 플로우를 N번 참조해도 충돌·상태공유 없음, REQ-SUBFLOW-D03).
- `<originalNodeID>`: 참조 플로우 내부 노드의 원래 ID.
- 중첩(서브플로우 안의 서브플로우)은 접두사를 누적한다: `subflow_<fnA>_subflow_<fnB>_<nodeID>`.
- 와이어 엔드포인트(`SourceNodeID`/`TargetNodeID`)도 동일 접두사로 재작성한다. 와이어 ID 도 충돌 방지를 위해 접두사를 부여한다.
- 모니터링/로깅 추적성(REQ-SUBFLOW-D06): 컴포넌트 prefix(`resolveFlowComponentPrefix`, engine.go) 와 결합해 어떤 flow-node 유래인지 표시한다.

### 5.4 boundary 브리지 합성 — **확정: 직접 재배선(direct rewire)**

서브그래프 확장 시 flow-node 핸들과 참조 플로우 플로우 포트를 연결한다. **채택된 방식 = 브리지 노드 없는 직접 재배선**(`internal/api/service/subflow_expand.go`).

- 부모 와이어가 `flow-node.in1`(입력 핸들)로 보내는 메시지 → 서브그래프의 입력 포트 `in1` 에 연결된 내부 노드들로 직접 라우팅.
- 서브그래프의 출력 포트 `out1` 에서 나오는 메시지 → 부모의 `flow-node.out1`(출력 핸들) 하류로 직접 라우팅.

**구현 — 엔드포인트 해석(endpoint resolution):**

- 각 flow-node 인스턴스는 두 경계 매핑을 노출한다(`instantiateSubflow`):
  - `inMap[입력포트] = [(내부 소비자 노드, 포트) ...]` — 입력 메시지의 도착지.
  - `outMap[출력포트] = [(내부 생산자 노드, 포트) ...]` — 출력 메시지의 출발지.
- 부모 와이어의 양 끝을 이 매핑으로 해석한다(소스 끝 = `outMap`, 타겟 끝 = `inMap`). flow-node 가 아닌 일반 노드 끝은 그대로 보존한다.
- 모든 (생산자 × 소비자) **카르테시안 곱**으로 직접 와이어를 합성한다(`rewire_<원본와이어ID>_<seq>`).
- **passthrough**(참조 플로우 입력 포트 → 출력 포트 직결)는 `outMap` 의 마커(`nodeID==""`)로 표현하고 `expandOut` 이 입력 매핑으로 전개하여, 입력→출력 직통도 단일 규칙으로 처리한다.
- 이로써 일반 노드, 두 flow-node 의 직접 연결, passthrough 가 하나의 해석 규칙으로 일관 처리되며 중간 브리지 노드가 생기지 않는다(노드 수 최소, 회귀 위험 최소).
- 브리지 노드 방식(입력/출력 포트마다 pass-through 노드 합성)은 **채택하지 않았다**.

### 5.5 엔진 확장 통합 지점 — **확정: 어댑터 사전 확장 (옵션 1)**

확장(서브그래프 인스턴스화)은 **어댑터 사전 확장**으로 구현했다. 엔진 리포지토리 주입(옵션 2)은 채택하지 않았다.

**채택 — 어댑터 사전 확장 (`internal/api/service/flow_adapter.go` `DeployFlow`):**

배포 시 다음 순서로 수행한다(verified — flow_adapter.go:315~380):

1. `repo.Get(ctx, id)` 로 부모 조회(저장소 미존재 시 엔진 런타임 정의 폴백).
2. **순환/자기참조 검출** `DetectFlowReferenceCycle`(line 359) — 배포 시점 방어선.
3. **서브플로우 확장** `ExpandSubflows`(line 367) — flow-node 를 네임스페이스 인스턴스로 치환한 평탄화 플로우 생성. 순환 검출 이후에 수행하여 무한 확장 차단.
4. **단독 경계 와이어 제거** `flow.StripBoundaryWires`(line 378) — 이 플로우 자신의 top-level 포트 경계 와이어(외부 미연결) 제거.
5. `engine.DeployFlow(ctx, f)`(line 380) 호출.

- 엔진 코드(`engine.go` DeployFlow 노드 생성 루프, `CreateRuntimeWires`)는 **변경 없음**. 엔진은 평탄화된 일반 노드/와이어만 보며 서브플로우에 무지하다.
- 어댑터가 이미 `repo` 를 보유하므로 의존성 추가 없음. 순환 검출은 저장 경로(`CreateFlow` line 82, `UpdateFlow` line 245)와 배포 경로(line 359) 양쪽에서 수행한다(REQ-SUBFLOW-E04).

### 5.6 순환 검출 위치

- 입력: 리포지토리의 플로우 정의들. 각 플로우의 flow-node `flow_id` 로 "플로우→플로우" 방향 그래프 구성.
- 알고리즘: DFS 기반 사이클 검출(방문중/방문완료 색칠). 자기참조는 self-loop 로 함께 검출.
- 위치: 서비스 레이어 공용 함수(어댑터 `CreateFlow`/`UpdateFlow` 저장 경로 + `DeployFlow` 배포 경로에서 호출). 옵션 1 채택 시 어댑터가 보유한 `repo` 로 탐색.
- 에러: 순환 경로(플로우 id/name 시퀀스)를 포함한 명확한 에러 메시지.

### 5.7 프론트 데이터/렌더 흐름

**구현 — 합성 노드 방식**: 플로우 포트를 별도 오버레이가 아닌 **합성 React Flow 노드**(`__flow_input__`/`__flow_output__`)로 렌더하고, 실노드 바운딩 박스 위에 영역 표시 노드(`__flow_area__`)를 둔다(`web/src/lib/flow/boundary.ts`).

| 단계 | 위치(구현) | 변경 |
|------|------|------|
| 플로우 포트 상태 | `editorStore.ts` | `flowInputs`/`flowOutputs` 상태 + `addFlowPort`/`renameFlowPort`/`removeFlowPort` 액션, dirty 처리. 저장 시 `realNodesOnly` 로 합성 노드를 `nodes` 에서 제외하고 포트는 정의 `inputs`/`outputs` 로만 영속. |
| 경계/영역 노드 | `boundary.ts`, `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx` | 좌(입력 `__flow_input__`)/우(출력 `__flow_output__`) 합성 경계 노드 + 영역 노드 `__flow_area__`. 위치는 실노드 바운딩 박스에서 파생(노드 이동 시 따라옴). 합성 노드는 `draggable:false`/`deletable:false`(패널로만 관리)이며 선택은 editorStore 가드로 무력화. |
| 포트 관리 패널 | `FlowPortPanel.tsx`(제어판/툴바 "플로우 포트") | 포트 추가/이름/삭제. |
| flow-node 핸들 | `nodeSchemas.ts` `computePortsForNode` (`case 'flow-node'`) | 참조 플로우(`flow_id`) 의 inputs/outputs 로 핸들 동적 계산(`subflowPorts.ts` 비정규화 캐시). |
| 플로우 피커 | `nodeSchemas.ts` `flow_picker` 필드 + `flowService.getFlows` | 후보에서 현재 편집 중 플로우(자기) 제외(REQ-SUBFLOW-C04, 자기참조 방지). |
| 타입 | `types/flow.ts`, `boundary.ts` `FlowPortDef` | `FlowInfo.inputs/outputs`, flow-node 관련 타입. |

### 5.8 파일 구조 (구현 — 실제 파일)

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `pkg/flow/flow.go` | Flow 인터페이스/defaultFlow 에 플로우 포트(`Inputs()/Outputs()`, `SetInputs/SetOutputs`, `WithFlowInputPorts/WithFlowOutputPorts`) 추가. `Port` 타입 재사용. | 수정 ✅ |
| `pkg/flow/serialize.go` | 정의 최상위 `inputs`/`outputs` 라운드트립, `normalizeFlowPortIDs`/방향 정규화, `nil`→`[]` 보장 | 수정 ✅ |
| `pkg/flow/boundary.go` | 센티넬(`FlowInputBoundaryID`/`FlowOutputBoundaryID`), `IsBoundaryWire`, `StripBoundaryWires`, `RebuildFlow` | 신규 ✅ |
| `internal/node/registry.go` | `flow-node` 빌트인 등록(line 121) | 수정 ✅ |
| `internal/node/flow_node.go` | flow-node 안전망 팩토리 `NewFlowNodePlaceholder`(확장 누락 시 에러) | 신규 ✅ |
| `internal/api/service/subflow_expand.go` | 서브그래프 확장(네임스페이스 복제 + 직접 재배선/엔드포인트 해석 + dangling 드랍 + 깊이/노드 상한) | 신규 ✅ |
| `internal/api/service/subflow_cycle.go` | 순환/자기참조 DFS 검출 `DetectFlowReferenceCycle`(깊이 상한 8) | 신규 ✅ |
| `internal/api/service/flow_adapter.go` | 최상위 inputs/outputs 변환, DeployFlow 사전 확장(cycle→expand→strip→engine), 저장 시 순환 검출 | 수정 ✅ |
| `internal/engine/engine.go` / `options.go` | (옵션 2 미채택) **변경 없음** — 엔진 비침습 | 불변 ✅ |
| `web/src/types/flow.ts` | `FlowInfo.inputs/outputs` 타입 | 수정 ✅ |
| `web/src/stores/editorStore.ts` | 플로우 포트 상태/액션, 합성 노드 영속 제외, 선택 가드 | 수정 ✅ |
| `web/src/config/nodeSchemas.ts` | `flow-node` 스키마(`flow_picker`) + `computePortsForNode` flow-node 케이스 | 수정 ✅ |
| `web/src/lib/flow/boundary.ts` | 합성 경계/영역 노드, 센티넬, `realNodesOnly`/`isSyntheticNode`, 포트 영속/위치 파생 | 신규 ✅ |
| `web/src/lib/flow/subflowPorts.ts` | flow-node 참조 플로우 포트 비정규화(denormalize) 유틸 | 신규 ✅ |
| `web/src/lib/flow/connectionFocus.ts` | (본 SPEC 무관 — 같은 작업 분기에 포함) | 신규 |
| `web/src/components/flow/FlowBoundaryNode.tsx` | 합성 경계 노드 렌더 | 신규 ✅ |
| `web/src/components/flow/FlowAreaNode.tsx` | 영역 표시 노드 렌더 | 신규 ✅ |
| `web/src/components/flow/FlowPortPanel.tsx` | 포트 관리 패널(툴바 "플로우 포트") | 신규 ✅ |
| `web/src/services/api/flowService.ts` | 피커용 `getFlows` 재사용(자기 제외는 호출부) | 재사용 ✅ |

> 초기 SPEC 이 예정했던 `FlowPort` 전용 타입, 별도 `FlowBoundaryPorts.tsx` 오버레이, `FlowPickerModal.tsx` 모달, `cycle_detect.go` 명칭은 구현에서 다음으로 귀결되었다: 포트 타입은 `Port` 재사용, 경계 UI 는 합성 노드(`FlowBoundaryNode`/`FlowAreaNode`), 피커는 `nodeSchemas.ts` 의 `flow_picker` 필드, 순환 검출 파일은 `subflow_cycle.go`.

### 5.9 OPEN QUESTIONS → DECIDED (구현 확정)

1. **확장 위치(옵션 1 vs 2)** — **DECIDED: 어댑터 사전 확장(옵션 1)**. `FlowServiceAdapter.DeployFlow` 가 `engine.DeployFlow` 호출 전에 `ExpandSubflows` 로 확장한다. 엔진은 비침습(리포지토리 주입 없음, `engine.go`/`options.go` 불변). Section 5.5 참조.
2. **브리지 방식** — **DECIDED: 직접 재배선(direct rewire)**. 경계 브리지 노드를 합성하지 않고, 엔드포인트 해석(inMap/outMap 카르테시안)으로 와이어만 직접 합성한다(`subflow_expand.go`). Section 5.4 참조.
3. **중첩 깊이/노드 수 상한** — **DECIDED: 최대 중첩 깊이 8, 최대 확장 노드 5000**. 초과 시 명확한 에러. 구현: `maxSubflowExpandDepth=8`(또한 `DetectFlowReferenceCycle` 의 `maxFlowReferenceDepth=8`), `maxSubflowExpandNodes=5000`(`subflow_expand.go`/`subflow_cycle.go`).
4. **dangling 정책 강도** — **DECIDED: 경고 + 해당 와이어 정리(드랍)**. 참조 포트 제거로 매칭 실패한 부모 와이어는 `slog.Warn` 으로 경고 로깅하고 확장 결과에서 드랍한다(`DanglingWire`, `logDangling`). 프론트는 미해결 핸들을 렌더하지 않아 dangling 발생을 사전 억제한다.
5. **export/import 시 flow-node 참조 정합** — **DECIDED: `flow_id` 참조 유지(참조 플로우 동봉 안 함)**. export 본은 `flow_id` 참조만 보존하며 참조 플로우를 self-contained 로 동봉하지 않는다. 누락 참조는 배포 시 엄격 에러로 처리(REQ-SUBFLOW-F03).

#### 향후 SPEC 으로 분리 (본 SPEC 범위 외 — 미결정 유지)

- **인스턴스 파라미터 오버라이드**: 서브플로우 인스턴스별 config 오버라이드(환경/이름)는 본 SPEC 제외 — 향후 SPEC 분리.
- **단독 배포 시 포트 외부 노출 승격**: 현재는 dangling(엔드포인트 비활성) 처리(REQ-SUBFLOW-F01). 단독 배포 시 포트를 외부 노출 엔드포인트(HTTP/WS)로 승격하는 것은 향후 과제.

## 6. 추적성

| 요구사항 ID | 구현 위치(예정) | 검증 |
|-------------|----------------|------|
| REQ-SUBFLOW-A01 ~ A07 | `pkg/flow/flow.go`, `pkg/flow/serialize.go`, `flow_adapter.go` | flow round-trip, flow_adapter_test ✅ |
| REQ-SUBFLOW-B01 ~ B05 | `web/src/lib/flow/boundary.ts`, `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx`, `FlowPortPanel.tsx`, `editorStore.ts` | boundary.test.ts, FlowPortPanel.test.tsx, editorStore.test.ts ✅ |
| REQ-SUBFLOW-C01 ~ C05 | `nodeSchemas.ts`(`computePortsForNode`/`flow_picker`), `subflowPorts.ts`, `registry.go`, `flow_node.go` | subflowPorts.test.ts ✅ |
| REQ-SUBFLOW-D01 ~ D06 | `subflow_expand.go`, `flow_adapter.go` DeployFlow, `boundary.go`(`RebuildFlow`) | subflow_expand_test ✅ |
| REQ-SUBFLOW-E01 ~ E04 | `subflow_cycle.go`, `flow_adapter.go`(save line 82/245 + deploy line 359) | subflow_cycle_test ✅ |
| REQ-SUBFLOW-F01 ~ F03 | `boundary.go`(`StripBoundaryWires`), 어댑터(엄격 누락 처리), 엔진(불변) | 회귀 + 로드 호환 ✅ |
| REQ-SUBFLOW-N01 ~ N02 | `subflow_expand.go`(깊이 8/노드 5000 상한), `subflow_cycle.go`(깊이 8), 엔진(불변) | subflow_expand_test, 엔진 회귀 ✅ |
