---
id: SPEC-SUBFLOW-001
title: "플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트"
version: "1.0.0"
status: planned
created: "2026-06-04"
updated: "2026-06-04"
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

입력 포트는 캔버스 **좌측 경계**, 출력 포트는 캔버스 **우측 경계**에 표시한다. 내부 노드와 와이어로 연결한다. 포트 추가/이름/삭제를 위한 관리 패널을 제공한다. 포트는 노드가 아닌 플로우 레벨 엔티티이다.

### 1.3 배경

- xflow 는 IoT FBP(Flow Based Programming) 플랫폼이며, 웹 에디터(React Flow)에서 노드와 와이어로 플로우를 구성한다.
- `pkg/flow/flow.go` 의 `Flow` 인터페이스와 `defaultFlow` 구현체는 id/name/description/state/nodes/wires/config/metadata 를 보유하나 **플로우 레벨 포트가 없다**. (확인 완료 — flow.go에 `Inputs()/Outputs()` 부재.)
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

- **A1**: `pkg/flow` 의 `Flow` 인터페이스/`defaultFlow` 에는 플로우 레벨 포트가 없으므로 신설한다(`Inputs() []FlowPort` / `Outputs() []FlowPort` 또는 동등). 신규 모델이며 기존 `NodeDef.Port` 와 별개의 플로우 레벨 엔티티이다.
- **A2**: 노드 ID/와이어 엔드포인트는 자유 문자열이므로 네임스페이스 접두사 부여로 인스턴스 격리가 가능하다. 동일 부모 내 여러 flow-node가 같은 플로우를 참조해도 접두사가 flow-node ID를 포함하면 충돌하지 않는다.
- **A3**: 어댑터(`FlowServiceAdapter`)는 `repo storage.FlowRepository` 를 보유하므로 배포 직전 참조 플로우를 조회·확장할 수 있다(엔진 비침습 옵션). 대안으로 엔진에 `WithFlowRepository` 를 주입할 수 있으며(`WithAgentManager` 선례), 두 옵션은 plan.md에서 비교한다.
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

`FlowPort` 구조(노드 `flow.Port` 와 형태 유사하되 플로우 레벨 엔티티):

| 필드 | 타입 | 의미 |
|------|------|------|
| `id` | string | 안정적 식별자(이름 변경에도 불변). |
| `name` | string | 표시·연결 이름. flow-node 핸들 이름으로 사용. |
| `direction` | `"input"` \| `"output"` | 방향. |

> 노드의 `inputs`/`outputs`(NodeDef 내부)와 구분하기 위해 플로우 레벨 포트는 **정의 최상위**에만 둔다. 어댑터는 노드 변환(`normalizeReactFlowDefinition`)과 별개로 최상위 `inputs`/`outputs` 를 통과·변환한다.

### 5.2 데이터 계약 — flow-node 노드

| 키 | 타입 | 위치 | 의미 |
|----|------|------|------|
| `type` | string | NodeDef.Type | `"flow-node"` 고정. |
| `flow_id` | string | NodeDef.Config["flow_id"] | 참조 플로우 id. 자기 id 금지. |

- flow-node 의 입출력 핸들(`NodeDef.Inputs/Outputs`)은 **참조 플로우의 `inputs`/`outputs` 로부터 동적 계산**한다(저장 시점 핸들이 아니라 매 로드/배포 시점 참조 정의 기준 — 결정 2).
- 빌트인 등록(`internal/node/registry.go`)에 `{"flow-node", NewFlowNode, "composition", "다른 플로우를 참조하여 합성"}` 패턴으로 추가한다. 단, flow-node 는 직접 인스턴스화 대신 **배포 시 확장으로 소비**되는 특수 노드이다(Section 5.4 참조).

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

### 5.4 boundary 브리지 합성

서브그래프 확장 시 flow-node 핸들과 참조 플로우 플로우 포트를 연결한다.

- 부모 와이어가 `flow-node.in1`(입력 핸들)로 보내는 메시지 → 서브그래프의 입력 포트 `in1` 에 연결된 내부 노드들로 라우팅.
- 서브그래프의 출력 포트 `out1` 에서 나오는 메시지 → 부모의 `flow-node.out1`(출력 핸들) 하류 와이어로 라우팅.
- 구현 방식(택1, plan.md에서 결정):
  1. **와이어 재배선(rewire)**: flow-node 핸들로 들어오는 부모 와이어의 타겟을, 서브그래프 입력 포트에 연결되었던 내부 노드 포트로 직접 재배선. 출력도 대칭. (브리지 노드 없이 와이어만 합성 — 가장 가벼움.)
  2. **boundary 브리지 노드**: 입력/출력 포트마다 pass-through 노드를 합성하여 명시적 경계 노드로 둠(가독성·모니터링 우수, 노드 수 증가). `WithBridgePorts`(node.go) 의 in/out 포트 패턴을 참고할 수 있다.
- 권장: 가독성·추적성을 위해 옵션 2(브리지 노드)를 기본, 단순 케이스 최적화로 옵션 1 고려. plan.md에서 비교·결정.

### 5.5 엔진 확장 통합 지점 (두 옵션)

확장(서브그래프 인스턴스화)을 수행할 위치는 두 가지이며 plan.md에서 비교·권장한다.

**옵션 1 — 어댑터 사전 확장 (권장, 엔진 비침습)**
- `FlowServiceAdapter.DeployFlow`(flow_adapter.go ~304)에서 `repo.Get(ctx, id)` 로 부모를 조회한 뒤, flow-node 를 재귀적으로 확장한 **평탄화된(flattened) 단일 플로우**를 만들어 엔진 `DeployFlow` 에 전달.
- 엔진 코드(`engine.go` DeployFlow 노드 생성 루프 ~116-196, `CreateRuntimeWires` ~199)는 **변경 없음**. 엔진은 평범한 노드/와이어만 본다.
- 어댑터가 이미 `repo` 를 보유하므로 의존성 추가 없음. 순환 검출도 어댑터에서 수행.

**옵션 2 — 엔진 리포지토리 주입**
- 엔진에 `WithFlowRepository(repo)` EngineOption 을 추가(`WithAgentManager` 선례, options.go). `Engine.DeployFlow` 가 노드 생성 루프 직전에 flow-node 를 감지·확장.
- 장점: CLI/직접 엔진 사용 경로에서도 확장 일관성. 단점: 엔진이 storage 패키지에 의존(레이어 결합도 상승), DeployFlow 침습.

**권장**: 옵션 1(어댑터 사전 확장). 엔진을 합성에 무지하게 유지하여 회귀 위험을 최소화하고, 확장/순환검출을 서비스 레이어에 응집한다. 단, CLI 직접 배포 경로가 서브플로우를 지원해야 한다면 옵션 2 또는 공용 확장 함수(`internal/...`)를 어댑터·CLI 양쪽에서 호출하는 절충안을 둔다.

### 5.6 순환 검출 위치

- 입력: 리포지토리의 플로우 정의들. 각 플로우의 flow-node `flow_id` 로 "플로우→플로우" 방향 그래프 구성.
- 알고리즘: DFS 기반 사이클 검출(방문중/방문완료 색칠). 자기참조는 self-loop 로 함께 검출.
- 위치: 서비스 레이어 공용 함수(어댑터 `CreateFlow`/`UpdateFlow` 저장 경로 + `DeployFlow` 배포 경로에서 호출). 옵션 1 채택 시 어댑터가 보유한 `repo` 로 탐색.
- 에러: 순환 경로(플로우 id/name 시퀀스)를 포함한 명확한 에러 메시지.

### 5.7 프론트 데이터/렌더 흐름

| 단계 | 위치 | 변경 |
|------|------|------|
| 플로우 포트 상태 | `editorStore.ts` | `flowInputs`/`flowOutputs` 상태 + 추가/이름/삭제 액션, dirty 처리 |
| 경계 포트 UI | 신규 컴포넌트(캔버스 오버레이) | 좌(입력)/우(출력) 경계 포트 렌더 + 내부 노드 와이어 연결 핸들 |
| 포트 관리 패널 | 신규 패널 | 포트 추가/이름/삭제 |
| flow-node 핸들 | `nodeSchemas.ts` `computePortsForNode` | `case 'flow-node'`: 참조 플로우(`flow_id`) 의 inputs/outputs 로 핸들 동적 계산 |
| 플로우 피커 | 신규 모달 + `flowService.getFlows` | 후보에서 현재 `currentFlowId` 제외(REQ-SUBFLOW-C04) |
| 타입 | `types/flow.ts` | `FlowPort`, `FlowInfo.inputs/outputs`, flow-node 관련 타입 |

### 5.8 파일 구조 (예정)

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `pkg/flow/flow.go` | Flow 인터페이스/defaultFlow 에 플로우 포트(`Inputs()/Outputs()` + 변이) 추가 | 수정 |
| `pkg/flow/port.go` (또는 node.go) | `FlowPort` 타입(플로우 레벨) | 신규/수정 |
| `internal/node/registry.go` | `flow-node` 빌트인 등록 | 수정 |
| `internal/node/flow_node.go` | flow-node 정의(확장 마커 노드) | 신규 |
| `internal/api/service/subflow_expand.go` | 서브그래프 확장(네임스페이스+브리지) | 신규 |
| `internal/api/service/cycle_detect.go` | 순환/자기참조 DFS 검출 | 신규 |
| `internal/api/service/flow_adapter.go` | 최상위 inputs/outputs 변환, DeployFlow 사전 확장, 저장 시 순환 검출 | 수정 |
| `internal/storage/repository.go` | (필요 시) 그래프 탐색 헬퍼 — 기존 List/Get 재사용 우선 | (가능) |
| `internal/engine/engine.go` / `options.go` | 옵션 2 채택 시 `WithFlowRepository` + DeployFlow 확장 | (옵션 2 시) 수정 |
| `web/src/types/flow.ts` | FlowPort, inputs/outputs 타입 | 수정 |
| `web/src/stores/editorStore.ts` | 플로우 포트 상태/액션 | 수정 |
| `web/src/config/nodeSchemas.ts` | computePortsForNode flow-node 케이스 | 수정 |
| `web/src/services/api/flowService.ts` | 피커용 getFlows 재사용(자기 제외는 호출부) | (재사용) |
| `web/src/components/flow/FlowBoundaryPorts.tsx` | 캔버스 경계 포트 UI | 신규 |
| `web/src/components/flow/FlowPortPanel.tsx` | 포트 관리 패널 | 신규 |
| `web/src/components/flow/FlowPickerModal.tsx` | 참조 플로우 피커(자기 제외) | 신규 |

### 5.9 OPEN QUESTIONS (구현 시 결정)

1. **확장 위치(옵션 1 vs 2)**: 어댑터 사전 확장(권장) vs 엔진 리포 주입. CLI 직접 배포 경로의 서브플로우 지원 필요 여부에 따라 결정.
2. **브리지 방식**: 와이어 재배선(rewire) vs boundary 브리지 노드. 모니터링 가독성 vs 노드 수.
3. **중첩 깊이/노드 수 상한**: 안전 상한 기본값(예: 깊이 N, 총 노드 M)과 초과 시 동작.
4. **dangling 정책 강도**: 참조 포트 삭제 시 부모 와이어를 자동 정리할지, 경고만 두고 사용자 정리에 맡길지.
5. **인스턴스 파라미터 오버라이드**: 서브플로우 인스턴스별 config 오버라이드(환경/이름)는 본 SPEC 제외 — 향후 SPEC 분리.
6. **단독 배포 시 포트 시맨틱**: dangling 으로 둘지, 단독 배포는 포트를 외부 노출 엔드포인트(예: HTTP/WS)로 승격할지(향후).
7. **export/import 시 flow-node 참조 정합**: export 본에 참조 플로우를 동봉할지(self-contained) vs id 참조만(끊어진 참조 가능). 정합 정책.

## 6. 추적성

| 요구사항 ID | 구현 위치(예정) | 검증 |
|-------------|----------------|------|
| REQ-SUBFLOW-A01 ~ A07 | `pkg/flow/flow.go`, `flow_adapter.go` | flow_test.go, flow_adapter_test.go |
| REQ-SUBFLOW-B01 ~ B05 | FlowBoundaryPorts.tsx, FlowPortPanel.tsx, editorStore.ts | *.test.tsx |
| REQ-SUBFLOW-C01 ~ C05 | nodeSchemas.ts, FlowPickerModal.tsx, registry.go, flow_node.go | nodeSchemas.test.ts, *.test.tsx |
| REQ-SUBFLOW-D01 ~ D06 | subflow_expand.go, flow_adapter.go DeployFlow | subflow_expand_test.go |
| REQ-SUBFLOW-E01 ~ E04 | cycle_detect.go, flow_adapter.go (save+deploy) | cycle_detect_test.go |
| REQ-SUBFLOW-F01 ~ F03 | 전체(어댑터+엔진) | 회귀 + 로드 호환 테스트 |
| REQ-SUBFLOW-N01 ~ N02 | subflow_expand.go (상한), 엔진(불변) | subflow_expand_test.go, 엔진 회귀 |
