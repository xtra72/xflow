---
id: SPEC-SUBFLOW-001
title: "플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트"
version: "1.3.0"
status: implemented
created: "2026-06-04"
updated: "2026-06-09"
author: "xtra"
priority: high
related_specs:
  - SPEC-FLOW-001
  - SPEC-FLOW-002
  - SPEC-WIRE-001
  - SPEC-ENGINE-001
  - SPEC-WEB-001
  - SPEC-AGENT-NODE-001
  - SPEC-REMOTE-001
tags:
  - subflow
  - flow-node
  - composition
  - flow-port
  - namespace
  - cycle-detection
  - editor
  - engine
  - remote-subflow
  - remote-reference
  - remote-flow-bridge
  - live-bridge
  - ws-bridge
  - distributed-execution
  - bidirectional-port-bridge
  - query-proxy
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-04 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-06-05 | xtra | 구현 완료 동기화 — OPEN QUESTIONS 1~5 확정, 최종 아키텍처 반영(센티넬 경계 와이어 규약·직접 재배선·합성 경계/영역 노드 UI), 구현 상태(M1~M4 완료) 기록 |
| 1.2.0 | 2026-06-09 | xtra | v1.2 확장 — **원격 서브플로우 참조(그룹 R/RU/RC)**. LOCAL 플로우의 flow-node 가 **원격 관리 노드(SPEC-REMOTE-001 managed node)의 플로우**를 참조하고, **배포 시점에 항상 최신**으로 해석한다. 채택 = **배포 시 원격 참조(deploy-time remote reference)**: ExpandSubflows 가 원격 참조를 감지하면 매니저의 **SPEC-REMOTE-001 query 프록시(`flow`/`get`, REQ-J04)** 로 원격 플로우 정의를 fetch 하여 로컬 서브플로우처럼 인라인 확장(네임스페이스·평탄화·순환검출)한다. 확장된 서브그래프는 **배포 인스턴스(매니저/서버)에서 실행**(정의 임베딩이며 분산 실행이 아님 — 원격 노드는 라이브 실행 피어가 아니다). 데이터 모델 = `flow_id` 의 `remote://{instance_id}/{flow_id}` 정규형(bare id = LOCAL, 하위 호환). 실패 의미 = 노드 오프라인/원격 플로우 없음 → **배포 실패(명확한 에러, stale/empty 무음 사용 금지)**. **제약/비목표(반드시 명시)**: (1) fetch 된 원격 정의는 **redacted**(노드-로컬 시크릿 마스킹, REMOTE F06) → 매니저에서 실행 시 시크릿 의존 노드 미동작, (2) 원격 노드의 agent/device 참조는 매니저 환경에서 해석 → 원격-전용 자원 참조 실패, (3) self-contained/비시크릿 로직 플로우에서만 동작. 마일스톤 M8~M10 추가. OPEN QUESTIONS OQ-R1~R5 제기 → **✅ 전부 RESOLVED(2026-06-09, 사용자 권고안대로 확정: URI 형식 / 라이브 프록시 전용·미러 폴백 없음 / 오프라인 하드 실패 / 중첩 원격 참조 v1 거부 / 시크릿·agent·device 한계 배포 시 경고+정적 스캔)**. 기존 REQ-SUBFLOW-A01~F03/N01~N02 보존 — 추가만 |
| 1.3.0 | 2026-06-09 | xtra | v1.3 확장 — **원격 참조 = 라이브 브리지(LIVE BRIDGE) 로 전환(그룹 RB 신설)**. v1.2 의 **배포 시 정의 임베딩(deploy-time embedding)** 모델은 device-bound 원격 플로우에서 매니저에 디바이스가 없어 무출력(RC02/RC03 한계 확인)이므로, 원격 참조 flow-node 를 **라이브 브리지**로 **SUPERSEDE** 한다. 핵심: 원격 참조 flow-node 는 (1) 로컬 플로우 배포 시 **원격 노드가 참조 플로우를 자기 디바이스/에이전트로 실행**하고, (2) 로컬 flow-node 는 **기존 remote WS 세션(SPEC-REMOTE-001 그룹 D/J 의 양방향 node↔manager 채널) 위의 클라이언트 브리지**가 되어, (3) **양방향**으로 메시지를 패싱한다(로컬 입력 핸들→원격 입력 경계 포트, 원격 출력 경계 포트→로컬 출력 핸들; 경계 포트 이름으로 매핑). 전송 = 기존 WS 세션 재사용(신규 서버/포트/인증 없음) + **flow-bridge 메시지 타입**(open/subscribe·input-data·output-data·close) 신설(command/query/stream RPC 와 대칭하되 **READ+WRITE 양방향**). 참조 종류별 분기: LOCAL(bare id) = **임베딩 유지(불변)**, REMOTE(`remote://`) = **라이브 브리지(로컬 미확장)**. `remote://` 데이터 모델·피커(그룹 RU) **보존** — **실행 의미만 임베딩→브리지로 변경**. **device/secret 한계 해소**: 원격 플로우가 **노드에서 실행**되므로 노드-로컬 시크릿/디바이스/에이전트가 **정상 동작**(v1.2 RC01/RC02 한계 = 브리지로 해결). **SUPERSEDED**: §4.8 그룹 R 의 R02~R09(배포 시 fetch+인라인 확장·`ExpandSubflowsWithFetcher`·`RemoteFlowFetcher` 인라인 확장)는 원격 참조에 한해 폐기(로컬 bare-id 임베딩은 불변). §4.10 그룹 RC 의 RC01~RC03(시크릿/device 한계·정의 임베딩) 재작성(해소·반전). 마일스톤 P1~P4 추가(M8~M10 임베딩 경로 대체). **OPEN QUESTIONS OQ-RB1~RB6 ✅ 전부 RESOLVED(2026-06-09, 사용자 권고안대로 확정: 매니저 관리형 자동 배포 / flow-node 별 독립 브리지 / 포트별 경계 버퍼+oldest-drop·coalesce / 양방향 v1 포함 / 매니저 관리형 라이프사이클 / 쓰기 채널 authz=승인∧온라인∧노출+감사)**. 기존 LOCAL 서브플로우 요구(A~F/N·R01·RU01~RU05) 보존 |

> **구현 상태(Implementation Status)** — 2026-06-05, 브랜치 `feature/subflow-node`.
> 본 기능은 백엔드·프론트 전 마일스톤이 구현 완료되었다.
> - **M1 — 플로우 레벨 포트 모델**: 구현됨. `pkg/flow/flow.go`(`Inputs()/Outputs()`, `WithFlowInputPorts/WithFlowOutputPorts`, `SetInputs/SetOutputs`), `pkg/flow/serialize.go`(정의 최상위 `inputs`/`outputs` 라운드트립, `normalizeFlowPortIDs`).
> - **M2 — 엔진 확장/순환 검출**: 구현됨. `internal/api/service/subflow_expand.go`(`ExpandSubflows`), `internal/api/service/subflow_cycle.go`(`DetectFlowReferenceCycle`), `pkg/flow/boundary.go`(센티넬·`StripBoundaryWires`·`RebuildFlow`), `internal/api/service/flow_adapter.go`(DeployFlow 사전 확장 + 저장/배포 순환 훅).
> - **M3 — flow-node 프론트**: 구현됨. `web/src/config/nodeSchemas.ts`(`flow-node` 스키마 + `computePortsForNode`), `web/src/lib/flow/subflowPorts.ts`, `internal/node/flow_node.go`·`internal/node/registry.go`(빌트인 등록).
> - **M4 — 경계 포트 UI**: 구현됨. `web/src/lib/flow/boundary.ts`(합성 경계/영역 노드, 센티넬, 포트 영속), `FlowBoundaryNode.tsx`, `FlowAreaNode.tsx`, `FlowPortPanel.tsx`, `web/src/stores/editorStore.ts`.
>
> 본문 요구사항/인수 기준은 보존하되, 구현으로 충족된 항목은 "구현됨"으로 주석한다. 신규 요구사항은 추가하지 않는다.

> **v1.2 확장 상태(2026-06-09)** — **SUPERSEDED by v1.3(원격 참조에 한함)**. v1.2 의 "배포 시 정의 임베딩(ExpandSubflows 원격 분기 + `RemoteFlowFetcher` 인라인 확장)"은 **미구현 상태로 폐기**되었다. 원격 참조 실행 의미는 v1.3 의 **라이브 브리지(그룹 RB)** 로 대체된다(§4.11·§5.14~§5.17). v1.2 의 **데이터 모델(`remote://` URI, R01)·피커(그룹 RU)** 는 그대로 보존되며, **fetch+인라인 확장(R02~R09)·정의 임베딩 제약(RC01~RC03)** 만 변경된다.

> **v1.3 확장 상태(2026-06-09, 브랜치 `feature/subflow-node`)** — **계획(planned), OQ-RB ✅ 전부 RESOLVED**. v1.3 은 원격 참조 flow-node 를 **deploy-time embedding → live bridge** 로 전환한다(그룹 RB, §4.11·§5.14~§5.17). 원격 플로우는 **원격 노드에서 그 노드의 디바이스/에이전트로 실행**되고, 로컬 flow-node 는 **기존 remote WS 세션(SPEC-REMOTE-001 그룹 D/J)** 위의 양방향 브리지가 되어 메시지를 라이브로 패싱한다. 본 확장 요구사항은 미구현이며 `/moai run SPEC-SUBFLOW-001` 으로 마일스톤 P1~P4 증분 구현한다. **OPEN QUESTIONS OQ-RB1~RB6(§5.17) ✅ 전부 RESOLVED(2026-06-09, 사용자 권고안대로 확정) — 구현 시 고정 제약이며 Run 진입 가능.**
> - 형제 변경(별건)으로 flow-node 피커가 **target-aware** 화되어, **원격 노드의 플로우를 편집할 때 그 노드의 플로우 목록**을 보이게 했다(same-node 서브플로우). v1.2/v1.3 은 이와 구분되는 **cross-node** 케이스다 — **LOCAL 플로우가 원격 노드의 플로우를 참조**한다.
> - **v1.2 와의 차이(핵심)**: v1.2 = 원격 정의를 fetch 해 **매니저에서 실행**(디바이스/시크릿 없음 → device-bound 플로우 무출력). v1.3 = 원격 플로우를 **원격 노드에서 실행**하고 출력을 로컬로 **라이브 중계**(device/secret 한계 해소).

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

#### 결정 5 — 참조 종류별 실행 분기: LOCAL=임베딩 / REMOTE=라이브 브리지 (v1.3, 확정)

flow-node 는 **참조 종류(reference kind)** 에 따라 **다르게 동작**한다.

- **LOCAL 참조(bare id, 예 `flow-abc123`)** — **결정 1 의 인스턴스화(배포 시 서브그래프 임베딩) 유지(불변)**. 참조 플로우 정의를 매니저에 인라인 확장하여 같은 인스턴스에서 실행한다. v1.1 동작 그대로이며 v1.3 에서 변경 없다.
- **REMOTE 참조(`remote://{instance_id}/{flow_id}`)** — **라이브 브리지(live bridge)**. 매니저는 원격 정의를 **로컬에 확장하지 않는다**. 대신 (a) 로컬 플로우 배포 시 **원격 노드가 참조 플로우를 자기 디바이스/에이전트로 실행**하고, (b) 로컬 flow-node 는 **기존 remote WS 세션**(SPEC-REMOTE-001 그룹 D/J 의 양방향 node↔manager 채널) 위의 **클라이언트 브리지 엔드포인트**가 되어, (c) 양방향으로 메시지를 패싱한다(로컬 입력 핸들 → 원격 입력 경계 포트, 원격 출력 경계 포트 → 로컬 출력 핸들; 경계 포트 이름으로 매핑).

> **v1.2(임베딩) → v1.3(브리지) SUPERSEDE 근거**: v1.2 는 원격 정의를 fetch 해 **매니저에서 실행**했다. 그러나 매니저에는 원격 노드의 디바이스가 없으므로, **노드의 시리얼 포트 등 device-bound 원격 플로우는 매니저에서 무출력**이다(REQ-SUBFLOW-RC02/RC03 한계 = 확인됨). v1.3 는 원격 플로우를 **노드에서 실행**(디바이스/시크릿 보유)하고 출력만 로컬로 라이브 중계하므로, 이 한계를 **근본 해소**한다. 이것이 본 변경의 목적이다.

> **전송 = 기존 remote WS 세션 재사용**: 새 서버/포트/인증을 만들지 않는다. SPEC-REMOTE-001 의 영속 WS 세션(command/query/stream RPC 운반)에 **flow-bridge 메시지 타입**(open/subscribe·input-data·output-data·close)을 추가한다(그룹 D 명령·그룹 J query/stream 과 대칭하되 **READ+WRITE 양방향** — read-only query/stream 프록시와 구분).

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

**포함(v1.2 — 원격 서브플로우 참조 데이터 모델/피커, v1.3 에서 보존):**
- 백엔드: flow-node `flow_id` 의 **원격 참조 정규형**(`remote://{instance_id}/{flow_id}`, bare id = LOCAL 하위 호환) 파싱/포맷 (REQ-SUBFLOW-R01 — **v1.3 보존**).
- 프론트: flow-node 피커에 **원격 노드 선택기**(기본 LOCAL) + 그 노드의 플로우 목록(SPEC-REMOTE-001 원격 플로우 목록/`useFlowsTarget` 재사용) + **원격 배지** + 원격 참조 핸들 계산(query 프록시 `flow`/`get` 으로 원격 플로우 inputs/outputs READ-ONLY 취득) (그룹 RU — **v1.3 보존**).

> ~~배포 시 ExpandSubflows 의 원격 분기 fetch+인라인 확장(R02~R09)~~ 는 **v1.3 에서 SUPERSEDED** — 아래 "포함(v1.3)" 의 라이브 브리지로 대체된다.

**포함(v1.3 — 원격 참조 = 라이브 브리지):**
- 백엔드(원격 프로토콜): 기존 remote WS 세션 위 **flow-bridge 메시지 타입**(`bridge_open`/`bridge_input`/`bridge_output`/`bridge_close`/`bridge_status`) 신설(SPEC-REMOTE-001 그룹 D/J 와 대칭, **READ+WRITE 양방향**). 브리지 인스턴스별 상관 id(`bridge_id`). 승인∧온라인 게이팅(REQ-J05) 상속 + 쓰기 채널 authz.
- 백엔드(노드 측 브리지): 로컬 플로우 배포 시 노드가 참조 플로우를 실행(**OQ-RB1/RB5 ✅ RESOLVED = 매니저 관리형 자동 배포** — `bridge_open` 으로 deploy+start, 실행 중이면 재사용)하고, 그 플로우의 **입출력 경계 포트를 탭(tap)** 하여 `bridge_input` 을 입력 경계 포트로 주입·출력 경계 포트 메시지를 `bridge_output` 으로 전송.
- 백엔드(매니저 측 브리지 + 엔진): flow-node 가 **원격 모드(remote mode)** 로 동작 — 로컬 확장/임베딩하지 않고 **브리지 엔드포인트**로 동작. 수신 입력 메시지를 WS 로 노드에 전달(`bridge_input`), 수신 출력 메시지(`bridge_output`)를 하류 와이어로 emit. 로컬 플로우 undeploy/노드 오프라인 → 브리지 teardown.
- 프론트: flow-node **원격 브리지 상태/표시기**(연결/실행/오프라인/오류), 원격 배지(그룹 RU 보존) + 브리지 상태 통합.

**제외:**
- 라이브(런타임) 플로우 간 메시지 버스 (명시적 금지 — 인스턴스화 모델만 사용).
- 참조 스냅샷 고정/버전 핀 (항상 최신 정책).
- 서브플로우 인스턴스별 파라미터/환경 변수 오버라이드 (향후 SPEC, OPEN QUESTION).
- 중첩 깊이 제한 정책의 정밀 튜닝(기본 안전 상한만 — Section 5 참조).

**제외(v1.3 — 라이브 브리지 비목표/제약):**
- **임베딩(매니저 실행) — 원격 참조에 한해 SUPERSEDED**: ~~원격 정의 fetch + 매니저 인라인 확장/실행~~ 은 v1.3 에서 폐기된다(device/secret 무동작 한계). **LOCAL bare-id 임베딩은 불변**(결정 5).
- **신규 전송/서버/포트/인증**: 브리지는 **기존 remote WS 세션만 재사용**한다. 별도 브리지 서버·포트·인증 채널을 신설하지 않는다(REQ-SUBFLOW-RB02).
- **원격 노드로의 양방향 미러/역참조**: 원격 노드의 플로우가 매니저의 LOCAL 플로우를 참조하는 역방향(역브리지)은 본 SPEC 제외.
- **중첩 브리지(브리지된 원격 플로우가 또 다른 원격 참조 flow-node 를 포함)**: 원격 플로우가 그 자체로 또 원격 참조 flow-node 를 포함하는 체인은 v1 비목표(OQ — §5.17). 노드 측 self-contained 실행이므로 분산 순환 위험은 LOCAL 임베딩보다 낮으나 v1 은 단순화를 위해 제한 권고.
- **다중 flow-node 간 브리지 공유**: 같은 원격 플로우를 참조하는 복수 flow-node 의 브리지 공유는 v1 비목표(**OQ-RB2 ✅ RESOLVED = flow-node 별 독립 브리지** 확정).
- **브리지 출력 영속/리플레이**: 오프라인 동안의 출력 버퍼링/리플레이는 비목표(오프라인=무출력+상태 표시, REQ-SUBFLOW-RB09).

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

### 3.1 가정 (v1.2 — 원격 참조 데이터 모델/게이팅, v1.3 보존)

- **A8**(v1.2, 보존): 본 확장은 **SPEC-REMOTE-001 의 매니저(서버 모드) 인프라 위에서만** 동작한다. 매니저는 `*remote.Server` 를 보유하며, 원격 노드와 **영속 양방향 WS 세션**(command/query/stream RPC 운반 — 그룹 D/J)을 유지한다. server 모드가 아니거나 브리지 엔드포인트가 구성되지 않으면 원격 참조는 배포 시 명확한 에러로 거부된다(REQ-SUBFLOW-RB09/R07 대체).
- **A9**(v1.2, 보존): 원격 자원 접근은 **승인(approved)∧온라인(online)∧노출(exposure) 범위** 게이팅(REQ-J05)에 종속된다. 피커의 핸들 계산(원격 inputs/outputs 취득)은 **query 프록시 `flow`/`get`(READ-ONLY, redacted)** 을 그대로 사용한다(RU05 보존). 게이팅 실패/오프라인은 기존 실패 의미(미관리/오프라인→503·타임아웃→504·노드오류→502)로 표면화된다.

> **~~A10·A11·A12(v1.2 임베딩 가정)~~ 는 v1.3 에서 SUPERSEDED** — 원격 참조는 더 이상 매니저에 정의를 fetch·확장·실행하지 않는다. 아래 v1.3 가정(A13~A17)으로 대체된다.

### 3.1b 가정 (v1.3 — 라이브 브리지)

- **A13**(v1.3): 원격 참조 flow-node 의 데이터 평면은 **기존 remote WS 세션 위의 신규 flow-bridge 메시지**로만 흐른다. 이 채널은 **READ+WRITE 양방향**(입력 주입 + 출력 수신)이며, READ-ONLY 인 query/stream 프록시(그룹 J)와 **구분**된다. 신규 서버/포트/인증을 만들지 않는다.
- **A14**(v1.3): 원격 노드는 **라이브 실행 피어**이다(v1.2 와 정반대). 참조 플로우는 **원격 노드에서 그 노드의 디바이스/에이전트/시크릿으로 실행**된다. 따라서 노드-로컬 시크릿/디바이스/에이전트 의존 플로우가 **정상 동작**한다(v1.2 RC01/RC02 한계 해소).
- **A15**(v1.3): 로컬 flow-node 는 **원격 모드에서 로컬 확장/임베딩을 하지 않는다**. 엔진에는 flow-node 가 **브리지 엔드포인트 노드**로 남아, 입력 핸들 수신 메시지를 WS 로 노드에 전달하고 노드로부터 받은 출력 메시지를 출력 핸들 하류로 emit 한다. LOCAL bare-id 참조의 인라인 임베딩(결정 1·A6)은 **불변**으로 공존한다.
- **A16**(v1.3): 원격 참조 flow-node 의 입출력 핸들 ↔ 원격 플로우의 입출력 **경계 포트** 매핑은 **포트 이름**(in1/out1 ...)으로 한다. 핸들 포트 집합은 피커가 query 프록시 `flow`/`get` 의 `config.inputs/outputs` 로 이미 계산한다(RU05 — 그대로 사용).
- **A17**(v1.3): 원격 플로우의 **run 라이프사이클**(배포·시작·정지)은 로컬 플로우 배포/undeploy 와 브리지 open/close 에 연동된다. 소유 모델은 **OQ-RB1/RB5 ✅ RESOLVED = 매니저 관리형 자동 배포**(매니저가 `bridge_open` 으로 deploy+start, 이미 실행 중이면 재사용; 배포 시 오프라인이면 무연결 배포 + online 시 자동 재open). 노드 오프라인/로컬 undeploy 시 브리지는 teardown 되고 원격 run 은 매니저 관리 모델에 따라 정리된다.

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

### 4.8 그룹 R — 원격 서브플로우 참조 (Remote Subflow Reference) — v1.2 도입, v1.3 부분 SUPERSEDE

> **상태(v1.3)** — 본 그룹의 **데이터 모델(R01, `remote://` URI)** 은 **보존**된다. 그러나 **배포 시 fetch+인라인 확장(R02~R09)** 은 v1.3 의 **라이브 브리지(그룹 RB, §4.11)** 로 **SUPERSEDED** 되었다. 즉 원격 참조는 더 이상 매니저에 정의를 fetch·확장·실행하지 않는다. R02~R09 는 **이력 목적으로만 유지**하며, 구현 대상은 그룹 RB 이다. (LOCAL bare-id 임베딩 = 그룹 A~F·N·D01~D06 은 **전부 보존**.)

**REQ-SUBFLOW-R01**: 원격 참조 데이터 모델 (`remote://` 정규형, 하위 호환) — **✅ v1.3 보존**
시스템은 **항상** flow-node 의 `flow_id` 가 **원격 참조 정규형 `remote://{instance_id}/{flow_id}`**(OQ-R1 ✅ RESOLVED = URI 확정)을 운반할 수 있어야 하며, 이 형식이 원격 노드 `instance_id` 와 그 노드의 플로우 id 를 식별하도록 해야 한다. **WHERE** `flow_id` 가 정규형이 아닌 bare id 이면, 시스템은 이를 **LOCAL 참조**로 해석해야 한다(기존 동작 하위 호환, REQ-SUBFLOW-C01 보존). 형식 판별은 결정적이고 모호하지 않아야 한다(`remote://` 스킴 유무로 판별). 이 형식은 v1.3 라이브 브리지에서 `bridge_open` 의 (instance_id, remote flow_id) 식별에 그대로 사용된다(REQ-SUBFLOW-RB01).

> **⛔ SUPERSEDED (v1.3) — REQ-SUBFLOW-R02 ~ R09**
> 아래 R02~R09 는 **배포 시 원격 정의 fetch + 매니저 인라인 확장(definition-embedding)** 모델이다. v1.3 에서 **원격 참조에 한해 폐기**되었다(device/secret 무동작 한계 — §1.2 결정 5). 대체 = **그룹 RB(라이브 브리지)**. 다음 매핑으로 대체된다:
> - ~~R02(배포 시 fetch)~~ → **RB05**(로컬 배포 시 원격 노드가 참조 플로우 실행) + **RB03**(bridge_open).
> - ~~R03(항상 최신=매 배포 fetch)~~ → 브리지는 **노드의 라이브 실행**을 직결하므로 "최신"이 자명(노드가 실행 중인 현재 정의).
> - ~~R04(인라인 확장)~~ → **RB07**(매니저는 확장하지 않음 — 브리지 엔드포인트).
> - ~~R05(게이팅 상속)~~ → **RB08**(승인∧온라인∧노출 게이팅 상속, 보존).
> - ~~R06(오프라인=배포 실패)~~ → **RB09**(오프라인=브리지 불가 → 무출력+상태/에러; **OQ-RB1 ✅ RESOLVED = 배포 시 오프라인이면 무연결 배포 허용 + online 시 자동 재open**, 하드 실패 아님).
> - ~~R07(해석기 부재 거부)~~ → **RB02/RB09**(브리지 엔드포인트/서버 모드 부재 시 거부).
> - ~~R08(중첩 원격 참조 거부)~~ → **§5.17 OQ**(브리지된 원격 플로우의 중첩 원격 참조 — v1 비목표, 노드 self-contained 실행).
> - ~~R09(순환/무한 확장 방지)~~ → 브리지는 매니저 확장이 없어 무한 확장 위험이 구조적으로 제거됨. LOCAL 순환 검출(E01~E04)은 보존.
>
> _(R02~R09 원문은 v1.2 이력으로 git 에 보존되며, 본 문서에서는 대체 매핑만 유지한다.)_

### 4.9 그룹 RU — 원격 서브플로우 피커 UI (Remote Subflow Picker) — v1.2 확장 (web)

> **범위(v1.2)** — LOCAL 플로우 편집에서 flow-node 가 **원격 노드의 플로우**를 선택할 수 있게 한다. 기본은 LOCAL 선택(REQ-SUBFLOW-C04 보존)이며, 원격 선택은 SPEC-REMOTE-001 의 원격 플로우 목록 접근(`useFlowsTarget`/원격 자원 목록)을 재사용한다.

**REQ-SUBFLOW-RU01**: 원격 노드 선택기
**WHEN** 사용자가 LOCAL 플로우에서 flow-node 의 참조 대상을 선택하면, **THEN** 시스템은 (a) LOCAL 플로우(기본) 또는 (b) **원격 노드**를 선택하는 노드 선택기를 제공해야 한다. 원격 노드 후보는 승인∧온라인 노드(SPEC-REMOTE-001 노드 목록)로 제한한다.

**REQ-SUBFLOW-RU02**: 원격 노드 플로우 목록
**WHEN** 사용자가 원격 노드를 선택하면, **THEN** 시스템은 그 노드의 플로우 목록을 **SPEC-REMOTE-001 원격 플로우 목록 접근**(`useFlowsTarget`/`?target=remote:{instanceId}` 또는 미러 목록 — REQ-J09~J10)으로 표시하고, 그중 하나를 서브플로우 참조로 선택할 수 있어야 한다. 별도 원격 전용 목록 데이터 경로를 신설하지 않고 재사용해야 한다.

**REQ-SUBFLOW-RU03**: 원격 참조 영속 (정규형 저장)
**WHEN** 사용자가 원격 노드의 플로우를 선택하면, **THEN** 시스템은 flow-node config 의 `flow_id` 에 **원격 참조 정규형**(REQ-SUBFLOW-R01, `remote://{instance_id}/{flow_id}`)을 저장해야 한다. LOCAL 선택은 기존대로 bare id 를 저장해야 한다(하위 호환).

**REQ-SUBFLOW-RU04**: 원격 선택 배지 표시
시스템은 **항상** flow-node 가 원격 플로우를 참조할 때 이를 **원격 배지(remote badge)** 와 출처 노드 식별(노드 이름/instance_id)로 명확히 표시해야 한다(로컬 참조와 시각적으로 구분).

**REQ-SUBFLOW-RU05**: 원격 참조 핸들 계산 (원격 포트, READ-ONLY) — **✅ v1.3 보존**
시스템은 **항상** 원격 참조 flow-node 의 입출력 핸들을 **원격 플로우의 입출력 포트**로 표시해야 한다. 원격 플로우의 `inputs`/`outputs` 는 query 프록시 `flow`/`get`(READ-ONLY, REQ-J04)으로 취득하여 핸들을 동적 계산하며(REQ-SUBFLOW-C02/C03 의 "항상 최신" 일관), 미해결(노드 오프라인/미선택)이면 핸들 없이 둔다(dangling 사전 억제, REQ-SUBFLOW-C05 일관). 이 포트 집합은 v1.3 브리지의 경계 포트 매핑(REQ-SUBFLOW-RB06)에 그대로 사용된다.

**REQ-SUBFLOW-RU06**: 원격 브리지 상태 표시기 (v1.3)
시스템은 **항상** 원격 참조 flow-node 의 **브리지 상태**(미연결/연결됨·실행중/노드 오프라인/오류)를 시각적으로 표시해야 한다(원격 배지 RU04 와 통합). 노드 오프라인이거나 브리지가 teardown 된 동안에는 flow-node 가 데이터를 패싱하지 않음(무출력)을 운영자가 식별할 수 있어야 한다(REQ-SUBFLOW-RB09 와 일관). 상태 소스는 기존 원격 노드 online/offline 신호(SPEC-REMOTE-001 그룹 B/G)와 브리지 라이프사이클(REQ-SUBFLOW-RB03) 이벤트이다.

### 4.10 그룹 RC — 실행 지역성 & 한계 (Execution Locality & Caveats) — v1.2 도입, v1.3 재작성(해소·반전)

> **중요(v1.3)** — v1.2 의 RC01~RC03 은 "원격 정의를 **매니저에서 실행**" 하는 임베딩 모델의 한계(시크릿/device 무동작)였다. v1.3 라이브 브리지는 원격 플로우를 **원격 노드에서 실행**하므로 이 한계를 **근본 해소·반전**한다. 아래는 재작성된 요구이다.

**REQ-SUBFLOW-RC01**: 시크릿 동작 (노드 실행 → 정상) — **v1.3 반전(해소)**
시스템은 **항상** 원격 플로우가 **원격 노드에서 실행**됨을 보장해야 한다. 따라서 원격 플로우의 노드는 **노드-로컬 시크릿을 정상적으로 사용**한다(v1.2 의 redacted 정의·매니저 실행으로 인한 시크릿 무동작 한계는 **해소**됨). 매니저로 와이어를 통해 흐르는 것은 **데이터(브리지 메시지)** 뿐이며, 시크릿 값은 노드에 머문다. (단, 브리지 메시지 페이로드 자체에 시크릿이 포함되지 않도록 페이로드 정책은 §5.14 redaction 규약을 따른다.)

**REQ-SUBFLOW-RC02**: agent/device 동작 (노드 실행 → 정상) — **v1.3 반전(해소)**
시스템은 **항상** 원격 플로우가 참조하는 agent/device 를 **원격 노드 환경에서** 해석·구동하도록 보장해야 한다. 원격 플로우가 노드의 **시리얼 포트·LGAP/LGCP 디바이스·로컬 에이전트** 등 device-bound 자원을 읽더라도, **노드에서 실행되므로 정상 동작**한다(v1.2 의 매니저 환경 해석 실패 한계는 **해소**됨). 이것이 본 변경의 핵심 동기이다(RC02/RC03 한계 확인 → 브리지로 해결).

**REQ-SUBFLOW-RC03**: 실행 지역성 (분산 실행 — 노드 실행 + 라이브 중계) — **v1.3 반전**
시스템은 **항상** 원격 서브플로우를 **분산 실행(distributed execution)** 으로 동작시켜야 한다: 참조 플로우의 **로직/디바이스 I/O 는 원격 노드에서 실행**되고, **출력만 로컬 flow-node 의 출력 핸들로 라이브 중계**되며 **로컬 입력은 원격 입력 경계 포트로 라이브 전달**된다. 매니저는 정의를 임베드·실행하지 않는다(v1.2 "정의 임베딩, 분산 실행 아님" 은 **SUPERSEDED**). 원격 노드는 **라이브 실행 피어**이다(A14).

**REQ-SUBFLOW-RC04**: 잔존 한계 표면화 (오프라인/노출/authz) — v1.3
시스템은 **항상** 브리지의 잔존 운영 한계를 운영자에게 표면화해야 한다: (a) 노드 오프라인 시 무출력(RB09 — 상태 표시기 RU06), (b) 참조 플로우가 노출 범위 밖이면 브리지 불가(RB08), (c) 입력 방향(매니저→노드 데이터 주입)은 쓰기 권한(authz)을 요구한다(RB11). 이 한계들은 묻혀서는 안 되며 배포/피커 UX 에 표면화되어야 한다.

### 4.11 그룹 RB — 원격 라이브 브리지 (Remote Live Bridge) — v1.3 확장 (구현 대상)

> **범위(v1.3)** — 본 그룹은 원격 참조 flow-node 의 실행 의미를 **라이브 브리지**로 정의한다(임베딩 R02~R09 를 대체). 전송 = **기존 remote WS 세션**(SPEC-REMOTE-001 그룹 D/J). 신규 = **flow-bridge 메시지 타입**(command/query/stream 과 대칭하되 **READ+WRITE 양방향**). **OPEN QUESTIONS OQ-RB1~RB6(§5.17) ✅ 전부 RESOLVED(2026-06-09, 사용자 권고안대로 확정) — 아래 요구사항의 결정은 고정 제약이다.**

#### RB-a. 참조 종류 분기 & 전송

**REQ-SUBFLOW-RB01**: 참조 종류별 실행 분기 (LOCAL=임베딩 / REMOTE=브리지)
시스템은 **항상** flow-node 의 `flow_id` 참조 종류에 따라 실행 방식을 분기해야 한다. **WHERE** `flow_id` 가 bare id(LOCAL)이면 기존 인스턴스화 임베딩(결정 1·그룹 D)을 **변경 없이** 적용해야 하고, **WHERE** `flow_id` 가 `remote://{instance_id}/{flow_id}`(REMOTE, REQ-SUBFLOW-R01)이면 **로컬 확장/임베딩을 하지 않고** 라이브 브리지(RB02~RB10)로 동작해야 한다. 두 모드는 한 부모 플로우 안에서 공존할 수 있어야 한다.

**REQ-SUBFLOW-RB02**: 기존 remote WS 세션 재사용 (신규 서버/포트/인증 없음)
시스템은 **항상** 브리지의 모든 메시지를 **SPEC-REMOTE-001 의 기존 영속 WS 세션**(양방향 node↔manager 채널, 그룹 B/D/J)으로 운반해야 한다. 별도 브리지 서버·포트·인증·핸드셰이크를 신설하지 않아야 한다. 브리지 메시지는 기존 메시지 라우팅·상관·인증(노드 토큰)·재접속 인프라를 그대로 상속한다.

**REQ-SUBFLOW-RB03**: flow-bridge 메시지 타입 + 브리지 상관 id
시스템은 **항상** WS 세션 위에 다음 flow-bridge 메시지 타입을 제공해야 한다(그룹 D 명령·그룹 J query/stream 과 대칭):
> - `bridge_open`(server→node): "참조 플로우 F 를 실행하고 flow-node X 에 브리지하라". 페이로드 = `{bridge_id, instance_id, remote_flow_id, input_ports[], output_ports[]}`. (open 은 stream subscribe 와 유사하나 **양방향** 채널을 연다.)
> - `bridge_input`(server→node): 로컬 입력 핸들로 들어온 메시지를 원격 입력 경계 포트로 전달. 페이로드 = `{bridge_id, port, payload}`.
> - `bridge_output`(node→server): 원격 출력 경계 포트에서 나온 메시지를 로컬 출력 핸들로 전달. 페이로드 = `{bridge_id, port, payload}`.
> - `bridge_close`(both): 브리지 teardown. 페이로드 = `{bridge_id, reason}`.
> - `bridge_status`(node→server): 브리지 라이프사이클/헬스(open ack, 실행 시작/정지, 오류). 페이로드 = `{bridge_id, state, detail}`.
>
> 각 브리지 인스턴스에는 **상관 id `bridge_id`** 를 부여하여(REQ-D07 패턴 준용) 한 세션 위 다수 브리지를 구분해야 한다.

**REQ-SUBFLOW-RB04**: 양방향 READ+WRITE (read-only 프록시와 구분)
시스템은 **항상** 브리지 채널이 **READ+WRITE 양방향**(입력 주입 + 출력 수신)임을 보장해야 한다. 이는 READ-ONLY 인 query/stream 프록시(그룹 J, REQ-J03)와 **구분**된다. 따라서 브리지는 query/stream 의 read-only 불변식의 예외이며, 별도의 쓰기 authz(REQ-SUBFLOW-RB11)를 가진다.

#### RB-b. 라이프사이클 & 포트 매핑

**REQ-SUBFLOW-RB05**: 원격 플로우 실행 라이프사이클 (로컬 배포 → 노드 실행)
**WHEN** 원격 참조 flow-node 를 포함한 로컬 플로우를 배포하면, **THEN** 시스템은 원격 노드가 참조 플로우를 **실행(running)** 상태로 만들도록 보장하고 브리지를 open 해야 한다. **소유 모델 OQ-RB1/RB5 ✅ RESOLVED = 매니저 관리형 자동 배포(auto-deploy-on-bridge)**: `bridge_open` 이 노드에 "참조 플로우 deploy+start" 를 지시하고, 이미 실행 중이면 재시작 없이 재사용한다. 미채택 대안(상시 실행 전제 = must-be-running)은 §5.17 참조.

**REQ-SUBFLOW-RB06**: 경계 포트 매핑 (이름 기반)
시스템은 **항상** flow-node 의 입력 핸들 ↔ 원격 플로우의 입력 경계 포트, 원격 플로우의 출력 경계 포트 ↔ flow-node 의 출력 핸들을 **포트 이름**(in1/out1 ...)으로 1:1 매핑해야 한다. flow-node 의 포트 집합은 원격 플로우의 inputs/outputs 에서 산출되며(REQ-SUBFLOW-RU05, query 프록시 `flow`/`get` `config.inputs/outputs`), 이름 불일치(원격에서 포트 제거/개명)는 dangling 으로 처리해야 한다(REQ-SUBFLOW-C05 일관).

**REQ-SUBFLOW-RB07**: 매니저 측 엔진 통합 (flow-node = 브리지 엔드포인트, 미확장)
시스템은 **항상** 실행 중인 로컬 플로우 엔진에서 원격 참조 flow-node 가 **브리지 엔드포인트 노드**로 동작하도록 해야 한다: (a) 입력 핸들로 수신한 메시지를 `bridge_input` 으로 WS 를 통해 노드에 전송하고, (b) 노드로부터 수신한 `bridge_output` 메시지를 출력 핸들의 **하류 와이어로 emit** 해야 한다. 이 노드는 **로컬 서브그래프로 확장/임베드되지 않는다**(SPEC 레벨; LOCAL 임베딩과 명확히 구분).

**REQ-SUBFLOW-RB08**: 게이팅 상속 (승인∧온라인∧노출)
시스템은 **항상** 브리지 open/유지를 SPEC-REMOTE-001 세션 게이팅(승인 ∧ 온라인, REQ-F03/F04)과 참조 플로우의 **노출 범위(exposure, REQ-J05/E07)**에 종속시켜야 한다. 미승인/오프라인/노출 범위 밖 원격 플로우는 브리지되지 않으며 명확한 오류/상태로 표면화되어야 한다(무음 금지).

#### RB-c. 실패/저하 & 백프레셔 & 보안

**REQ-SUBFLOW-RB09**: 실패/저하 (오프라인=무출력+상태, 재접속)
**IF** 배포 시점 또는 실행 중 원격 노드가 오프라인이거나 브리지가 끊기면, **THEN** 시스템은 해당 flow-node 가 **데이터를 패싱하지 않고**(무출력) **오류/상태를 표면화**해야 한다(상태 표시기 REQ-SUBFLOW-RU06). 시스템은 stale 출력을 무음으로 생성하지 않아야 한다. 노드 재접속 시 시스템은 **기존 remote 세션 재접속 인프라**(SPEC-REMOTE-001 그룹 B 재접속)를 재사용하여 브리지를 재수립해야 한다(재open). 배포 시 노드 오프라인의 처리는 **OQ-RB1 ✅ RESOLVED = 무연결 상태로 배포 허용 + online 시 자동 재open**(상태 표시; 하드 배포 거부 아님).

**REQ-SUBFLOW-RB10**: 백프레셔 & 순서 (경계 버퍼, drop/coalesce)
시스템은 **항상** WS 위 브리지 데이터에 대해 **경계 버퍼링**과 백프레셔 정책을 적용하여 매니저/노드 메모리 폭증을 방지해야 한다. 포트별 **순서(FIFO)** 는 버퍼 내에서 보존하되, 소비자가 느린 경우 정책을 적용해야 한다. **정책 OQ-RB3 ✅ RESOLVED = 포트별 경계 버퍼 + 오버플로 시 oldest-drop 또는 latest-wins coalesce**(SPEC-REMOTE-001 REQ-J08b 백프레셔 정책 재사용, 버퍼 내 포트별 FIFO 보존).

**REQ-SUBFLOW-RB11**: 쓰기 채널 authz (브리지가 노드 플로우에 데이터 주입)
시스템은 **항상** 입력 방향(매니저→노드 `bridge_input`, 노드 플로우 입력 경계 포트에 **데이터 주입**)에 대해 read-only 프록시보다 강한 권한을 강제해야 한다: 대상 노드 **승인∧온라인** + 참조 플로우 **노출 범위 내** + 브리지 open/close 및 입력 주입을 **감사(audit)** 한다(일반 read 미감사인 REQ-J15 와 대비 — 쓰기는 감사). **OQ-RB6 ✅ RESOLVED = 승인∧온라인∧노출 범위 내 + open/close·입력 주입 감사**. **입력 방향(local→remote 쓰기)은 OQ-RB4 ✅ RESOLVED = 양방향 v1 포함**(output-only 폴백 미채택). 미승인/범위 밖 입력 주입은 노드가 거부해야 한다(REQ-D08 패턴 준용).

**REQ-SUBFLOW-RB12**: 브리지 인스턴스 독립성 (flow-node 별 1 브리지)
시스템은 **항상** 각 원격 참조 flow-node 인스턴스를 **독립 브리지(고유 `bridge_id`)** 로 다루어야 한다(LOCAL D03 인스턴스 격리와 일관). 같은 원격 플로우를 참조하는 복수 flow-node 의 **브리지 공유는 v1 비목표**(**OQ-RB2 ✅ RESOLVED = flow-node 별 독립 브리지** 확정). 한 flow-node 의 브리지 상태/오류는 다른 flow-node 에 영향을 주지 않아야 한다.

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

### 5.10 데이터 계약 — 원격 참조 (v1.2)

flow-node 의 `flow_id` 는 두 형식을 가진다(REQ-SUBFLOW-R01):

| 형식 | 예 | 해석 |
|------|----|------|
| bare id | `flow-abc123` | **LOCAL 참조**(기존 동작, 하위 호환). `repo.Get(ctx, flowID)` 로 매니저 로컬 저장소에서 조회. |
| 원격 정규형(권고) | `remote://node-7f3a/flow-abc123` | **원격 참조**. `instance_id=node-7f3a`, 원격 flow id=`flow-abc123`. query 프록시로 fetch. |

- 판별 규칙: `flow_id` 가 `remote://` 스킴이면 원격, 아니면 로컬(결정적·모호성 없음).
- 대안(OQ-R1): 구조화 config `{remote_instance_id, flow_id}`. 분리된 두 필드로 모호성을 더 줄이나 직렬화/피커/파서 변경 폭이 크다. **✅ RESOLVED — `remote://` URI 채택**(flow_id 단일 문자열 필드 재사용, 최소 침습). 구조화 대안은 미채택.
- 파싱/포맷 유틸은 백엔드(`subflow_expand.go` 의 원격 분기)와 프론트(피커 저장/표시) 양쪽에서 공유 규약을 따른다.

### 5.11 ~~배포 시 원격 해석 메커니즘 (v1.2)~~ — ⛔ SUPERSEDED by §5.14~§5.17 (v1.3 라이브 브리지)

> **SUPERSEDED** — v1.2 의 `RemoteFlowFetcher` 기반 "배포 시 fetch + 매니저 인라인 확장(`ExpandSubflows` 원격 분기)" 메커니즘은 v1.3 에서 **폐기**되었다(device/secret 무동작 한계 — §1.2 결정 5). 원격 참조의 실행 메커니즘은 **§5.14 브리지 프로토콜 / §5.15 라이프사이클 / §5.16 포트 매핑** 으로 대체된다. `ExpandSubflows` 는 원격 참조를 만나면 **확장하지 않고** flow-node 를 브리지 엔드포인트로 남긴다(REQ-SUBFLOW-RB07). `RemoteFlowFetcher` 인라인-확장 해석기 및 그 실패 매핑(R06/R07)은 구현하지 않는다. (피커 핸들 계산용 query 프록시 `flow`/`get` READ-ONLY 사용은 RU05 로 별도 보존 — 정의 임베딩이 아니라 포트 표시 전용.)

### 5.12 프론트 — 원격 피커 흐름 (v1.2)

| 단계 | 위치(예정) | 변경 |
|------|------------|------|
| 노드 선택기 | flow-node `flow_picker` 필드 확장(`nodeSchemas.ts`) | LOCAL(기본) / 원격 노드 선택 토글 + 노드 선택기(승인∧온라인 노드, SPEC-REMOTE-001 노드 목록). |
| 원격 플로우 목록 | `useFlowsTarget`/`?target=remote:{instanceId}`(SPEC-REMOTE-001 REQ-J09~J10) 재사용 | 원격 노드의 플로우 목록 표시·선택. |
| 원격 참조 저장 | 피커 onChange | `flow_id = remote://{instance_id}/{flow_id}`(REQ-SUBFLOW-RU03). |
| 원격 배지 | flow-node 렌더(`CustomNode`/배지) | 원격 표식 + 출처 노드 식별(REQ-SUBFLOW-RU04). |
| 핸들 계산 | `computePortsForNode` `case 'flow-node'` 확장 + `subflowPorts.ts` | 원격 참조 시 query 프록시 `flow`/`get` 으로 원격 inputs/outputs 취득 → 핸들 동적 계산(READ-ONLY, 항상 최신, REQ-SUBFLOW-RU05). 미해결 시 핸들 없음. **(v1.3 보존)** |
| **브리지 상태 표시기(v1.3)** | flow-node 렌더(`CustomNode`/배지) | 미연결/연결됨·실행중/노드 오프라인/오류 상태 표시(REQ-SUBFLOW-RU06). 원격 노드 online/offline(그룹 B/G) + 브리지 라이프사이클(RB03) 이벤트. |
| ~~제약 표면화(v1.2)~~ | ~~배포/피커 경고~~ | ⛔ **SUPERSEDED** — v1.2 의 시크릿/agent/device 한계 경고는 device/secret 가 노드 실행으로 해소되어(RC01/RC02 반전) 불필요. v1.3 잔존 한계(오프라인/노출/쓰기 authz)는 RU06 상태 표시기 + RB08/RB11 로 표면화(REQ-SUBFLOW-RC04). |

### 5.13 OPEN QUESTIONS → DECIDED (v1.2 — ✅ 전부 RESOLVED 2026-06-09, 사용자 권고안대로 확정)

> **OQ-R1~R5 ✅ 전부 RESOLVED(2026-06-09)** — 사용자가 권고안대로 확정했다. 구현 시 **고정 제약**이며 재논의하지 않는다. `/moai run SPEC-SUBFLOW-001` 진입 가능.

| ID | 질문 | 결정 (✅ RESOLVED) | 근거 |
|----|------|--------------------|------|
| **OQ-R1** | `flow_id` 원격 참조 형식 | ✅ **(a) `remote://{instance_id}/{flow_id}` URI**. bare id = LOCAL(하위 호환). 구조화 config `{remote_instance_id, flow_id}` 는 미채택. | flow_id 단일 문자열 필드 재사용 → 직렬화/피커/파서 최소 침습, bare id 하위 호환 유지. |
| **OQ-R2** | 배포 시 정의 출처 | ✅ **(a) 라이브 query 프록시 `flow`/`get`**. **미러 폴백 없음**(프록시-우선-미러-폴백 (c) 미채택). | "항상 최신"(결정 2) 보장. 미러 폴백은 stale·redaction/노출 정합 위험과 충돌하므로 배제. |
| **OQ-R3** | 배포 시 노드 오프라인 동작 | ✅ **(a) 하드 실패(배포 거부 + 명확한 에러)**. last-mirror 정의 사용 (b) 미채택. | REQ-SUBFLOW-F03/R06 의 누락 엄격 처리와 일관, stale 정의 무음 사용 금지. |
| **OQ-R4** | 중첩 원격 참조 | ✅ **(a) v1 거부(self-contained 경계)**. 허용(재귀 fetch + 분산 순환 검출) (b) 는 향후 SPEC. | 매니저가 원격 그래프를 완전 순회 불가 → 분산 순환 검출 회피. |
| **OQ-R5** | 시크릿/agent/device 한계 표면화 | ✅ **(b) 배포 시 비차단 경고 + best-effort 정적 스캔**(시크릿 보유 노드·미해결 agent/device 참조 나열). 문서-only (a) 미채택. | 제약이 기능 적용 범위를 실질적으로 한정하므로 운영자에게 명시적 표면화 필요. |

#### 향후 SPEC 으로 분리 (본 SPEC 범위 외 — 미채택 대안)

- **구조화 원격 참조 config**(OQ-R1 (b)): `flow_id` URI 대신 분리 필드 `{remote_instance_id, flow_id}`. v1 비채택 — 필요 시 향후 마이그레이션.
- ~~미러 캐시 폴백(OQ-R2)~~·~~중첩 원격 참조 허용(OQ-R4)~~ — v1.2 임베딩 OQ. v1.3 브리지에서는 무의미(노드 라이브 실행). v1.3 OQ 는 §5.17 참조.

### 5.14 브리지 프로토콜 명세 (v1.3, REQ-SUBFLOW-RB02~RB04)

기존 remote WS 세션(SPEC-REMOTE-001) 위에 flow-bridge 메시지 타입을 추가한다. command(그룹 D)·query/stream(그룹 J) RPC 와 **대칭**하되 **READ+WRITE 양방향**이다.

**메시지 타입(권고 명칭 — 구현 시 `internal/remote/protocol.go` 상수로 추가):**

| 타입 | 방향 | 페이로드 | 의미 |
|------|------|----------|------|
| `bridge_open` | server→node | `{bridge_id, instance_id, remote_flow_id, input_ports[], output_ports[]}` | 참조 플로우 실행 + 브리지 채널 개설. (stream subscribe 와 유사하나 양방향) |
| `bridge_open_ack` | node→server | `{bridge_id, ok, error?}` | open 결과(실행 시작 성공/실패). |
| `bridge_input` | server→node | `{bridge_id, port, payload}` | 로컬 입력 핸들 메시지 → 원격 입력 경계 포트 주입(WRITE). |
| `bridge_output` | node→server | `{bridge_id, port, payload}` | 원격 출력 경계 포트 메시지 → 로컬 출력 핸들 emit(READ). |
| `bridge_status` | node→server | `{bridge_id, state, detail}` | 라이프사이클/헬스(running/stopped/error). |
| `bridge_close` | both | `{bridge_id, reason}` | 브리지 teardown(양방향 발신 가능). |

- **상관**: `bridge_id`(REQ-D07 패턴) — 한 세션 위 다수 브리지 구분(REQ-SUBFLOW-RB03/RB12).
- **인증/전송**: 기존 WS 세션 그대로(노드 토큰 JWT, wss). 신규 인증 없음(REQ-SUBFLOW-RB02).
- **redaction**: `bridge_output`/`bridge_input` 페이로드는 **데이터 메시지**이며 시크릿 값을 운반하지 않는 것을 전제한다(시크릿은 노드 실행에 머문다 — RC01). 페이로드에 민감 필드가 포함될 가능성이 있으면 노드 측 전송 전 정책(SPEC-REMOTE-001 `secret_fields.go`, REQ-F06/J06)을 준용한다.
- **read-only 프록시와의 구분**: query/stream(그룹 J)은 READ-ONLY(REQ-J03). 브리지는 입력 주입(WRITE)을 포함하므로 별도 authz(REQ-SUBFLOW-RB11)를 가진다.

### 5.15 브리지 라이프사이클 (v1.3, REQ-SUBFLOW-RB05/RB08/RB09)

**개설(open) — 로컬 플로우 배포 시:**

1. 로컬 플로우 배포 중, 어댑터(또는 엔진 통합 레이어)가 원격 참조 flow-node 를 식별한다(REQ-SUBFLOW-RB01).
2. `ExpandSubflows` 는 원격 참조를 **확장하지 않고** flow-node 를 브리지 엔드포인트로 남긴다(REQ-SUBFLOW-RB07). LOCAL bare-id 만 인라인 확장(불변).
3. 게이팅 평가(승인∧온라인∧노출 — REQ-SUBFLOW-RB08). 실패 시 브리지 미개설 + 상태 표면화.
4. 매니저가 `bridge_open{bridge_id, instance_id, remote_flow_id, ports}` 를 WS 로 노드에 전송.
5. 노드는 참조 플로우를 **running** 으로 만든다(소유 모델 = **OQ-RB1/RB5 ✅ RESOLVED 매니저 관리형 자동 배포** — deploy+start, 실행 중이면 재사용). 입출력 경계 포트를 **탭(tap)** 하고 `bridge_open_ack` 반환.

**데이터 평면(running):**

- 로컬 flow-node 입력 핸들 수신 메시지 → 매니저가 `bridge_input{bridge_id, port, payload}` 전송 → 노드가 해당 입력 경계 포트로 주입(REQ-SUBFLOW-RB07/RB06).
- 원격 플로우 출력 경계 포트 메시지 → 노드가 `bridge_output{bridge_id, port, payload}` 전송 → 매니저가 flow-node 출력 핸들 하류로 emit(REQ-SUBFLOW-RB07/RB06).
- 백프레셔/순서: 포트별 경계 버퍼 + oldest-drop/coalesce(**OQ-RB3 ✅ RESOLVED**, REQ-SUBFLOW-RB10).

**종료/저하(teardown):**

| 트리거 | 처리 |
|--------|------|
| 로컬 플로우 undeploy | 매니저가 `bridge_close` 전송 → 노드가 원격 run 정리(소유 모델 따라). |
| 노드 오프라인 / WS 끊김 | 브리지 즉시 teardown. flow-node 무출력 + 상태=offline(REQ-SUBFLOW-RB09, RU06). 출력 버퍼링/리플레이 없음. |
| 노드 재접속(그룹 B 재접속) | 매니저가 브리지 재open(자동). 상태=재연결. |
| 노드 측 참조 플로우 정지/오류 | 노드가 `bridge_status{error}` → flow-node 상태=오류, 무출력. |

**배포 시 노드 오프라인(OQ-RB1 ✅ RESOLVED)**: 배포 거부 대신 **무연결 상태로 배포 허용 + online 시 자동 재open**(상태 표시). 하드 실패 아님.

**실패 매핑(SPEC-REMOTE-001 세션 의미 준용):** 오프라인 → 브리지 불가(503 계열 상태), open 타임아웃 → 504 계열, 노드 측 open/실행 오류 → 502 계열(`bridge_open_ack.error`/`bridge_status`).

### 5.16 포트 매핑 (v1.3, REQ-SUBFLOW-RB06)

- flow-node 의 포트 집합 = 원격 플로우의 `inputs`/`outputs`(경계 포트). 피커가 query 프록시 `flow`/`get` `config.inputs/outputs` 로 이미 산출(REQ-SUBFLOW-RU05).
- 매핑 = **포트 이름 1:1**(in1↔in1, out1↔out1 ...). 별도 인덱스/순서 의존 없음.
- 방향: flow-node **입력 핸들** → 원격 **입력 경계 포트**(`__flow_input__` 센티넬, §5.1.1)로 주입. 원격 **출력 경계 포트**(`__flow_output__`) → flow-node **출력 핸들** 로 emit.
- 이름 불일치(원격에서 포트 제거/개명): 해당 핸들 dangling 처리(REQ-SUBFLOW-C05 일관) + 상태 표면화.
- 노드 측 경계 포트 탭: 노드의 실행 엔진이 참조 플로우의 `__flow_input__`/`__flow_output__` 센티넬 경계 와이어를 브리지 주입/추출 지점으로 사용(로컬 단독 배포 시 `StripBoundaryWires` 와 달리, 브리지 모드에서는 경계를 브리지 엔드포인트로 활성화).

### 5.17 OPEN QUESTIONS → DECIDED (v1.3 — ✅ 전부 RESOLVED 2026-06-09, 사용자 권고안대로 확정)

> **OQ-RB1~RB6 ✅ 전부 RESOLVED(2026-06-09)** — 사용자가 권고안(REC)대로 확정했다. 구현 시 **고정 제약**이며 재논의하지 않는다. `/moai run SPEC-SUBFLOW-001` 진입 가능. (v1.2 OQ-R 과 동일하게 확정 완료.)

| ID | 질문 | 결정 (✅ RESOLVED) | 근거 / 미채택 대안 |
|----|------|--------------------|--------------------|
| **OQ-RB1** | 원격 플로우 자동 배포 vs 상시 실행 전제 | ✅ **매니저 관리형 자동 배포(auto-deploy-on-bridge)** — `bridge_open` 이 노드에 참조 플로우 deploy+start 지시, 이미 실행 중이면 **재사용**(재시작 없음). 배포 시 노드 오프라인이면 **무연결 배포 허용 + online 시 자동 재open**. | 운영자 수동 실행 불요, 로컬 배포와 일관된 라이프사이클. 미채택: must-be-already-running(자율 실행 플로우만 브리지) — 단순하나 수동 실행 강제. |
| **OQ-RB2** | 같은 원격 플로우 참조 다중 flow-node 의 브리지 공유 | ✅ **flow-node 별 독립 브리지(고유 `bridge_id`)** — LOCAL D03 인스턴스 격리와 일관. 한 flow-node 의 오류/오프라인이 타 flow-node 에 무영향(REQ-SUBFLOW-RB12). | 인스턴스 격리·단순 라우팅. 미채택: 공유 브리지(fan-out/fan-in) — 자원 절약하나 상태 공유·라우팅 복잡(향후). |
| **OQ-RB3** | 백프레셔 정책 | ✅ **포트별 경계 버퍼 + 오버플로 시 oldest-drop / latest-wins coalesce**(SPEC-REMOTE-001 J08b 재사용). 버퍼 내 **포트별 FIFO 순서 보존**(REQ-SUBFLOW-RB10). | 메모리 폭증 방지 + 최신성 우선. 미채택: block(노드 측 블로킹 위험), 무제한 버퍼(OOM 위험). |
| **OQ-RB4** | 입력 방향(local→remote 쓰기)을 v1 에 포함 | ✅ **양방향 v1 포함** — 입력(local→remote `bridge_input`) + 출력(remote→local `bridge_output`) 모두 v1 범위(REQ-SUBFLOW-RB04). 파이핑이 본 기능의 핵심. | output-only 축소 폴백 **미채택**(양방향 확정). 쓰기 authz 는 RB11/OQ-RB6 로 통제. |
| **OQ-RB5** | 원격 run 라이프사이클 소유 | ✅ **매니저 관리형**(`bridge_open` 으로 시작, `bridge_close`/undeploy 로 정지 — OQ-RB1 과 일관, REQ-SUBFLOW-RB05). 이미 실행 중이면 재사용. | 자동 배포와 일관된 소유 모델. 미채택: node-autostart(노드 자율 소유, 매니저 구독만). |
| **OQ-RB6** | 쓰기 채널 보안/authz | ✅ **승인(approved)∧온라인(online)∧노출 범위 내(in-exposure-scope)** + **브리지 open/close·입력 주입 감사(audit)**. read-only 프록시(J15 미감사)와 달리 **쓰기는 감사**. 노드가 **미승인/범위 밖 주입 거부**(REQ-SUBFLOW-RB11, REQ-D08 패턴). | 쓰기 채널은 노드 플로우에 데이터를 주입하므로 read gating 보다 강한 통제 필요. 추가 고려(rate-limit·포트별 쓰기 허용)는 향후 확장. |

#### 향후 SPEC 으로 분리 (v1.3 범위 외)

- **중첩 브리지**(브리지된 원격 플로우가 또 다른 원격 참조 flow-node 포함): v1 비목표(§1.4). 노드 self-contained 실행이므로 분산 순환 위험은 낮으나 v1 제한.
- **역방향 브리지**(원격 노드 플로우가 매니저 LOCAL 플로우 참조): 본 SPEC 제외.
- **오프라인 출력 버퍼링/리플레이**: 본 SPEC 제외(오프라인=무출력).

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
| REQ-SUBFLOW-R01 (v1.2 보존) | `remote://` 파서(백·프론트 공유) — 데이터 모델만 보존 | 파서 단위 ⏳ |
| ~~REQ-SUBFLOW-R02 ~ R09~~ (v1.2) | ⛔ **SUPERSEDED by RB** — 임베딩 fetch/인라인 확장 폐기. 구현 대상 아님. | — |
| **REQ-SUBFLOW-RB01 ~ RB04 (v1.3)** | `internal/remote/protocol.go`(bridge_* 메시지 타입·`bridge_id` 상관), `internal/remote/server.go`·`client.go`(브리지 라우팅), 참조 종류 분기(어댑터/엔진 통합) | 예정 — bridge_protocol_test(메시지 라운드트립·상관·분기) ⏳ |
| **REQ-SUBFLOW-RB05 ~ RB08 (v1.3)** | 노드 측 브리지(참조 플로우 실행 + 경계 포트 tap + bridge_input 주입/bridge_output 전송), 매니저 측 엔진 통합(flow-node 브리지 엔드포인트), 게이팅 상속(REQ-J05) | 예정 — bridge_lifecycle_test(open/실행/포트 매핑/게이팅), fake WS 주입 ⏳ |
| **REQ-SUBFLOW-RB09 ~ RB12 (v1.3)** | 오프라인/재접속 teardown·재open(그룹 B 재사용), 백프레셔 버퍼(OQ-RB3), 쓰기 authz·감사(OQ-RB6), flow-node 별 독립 브리지 | 예정 — bridge_failure_test(오프라인 무출력/재접속/백프레셔/authz 거부) ⏳ |
| REQ-SUBFLOW-RU01 ~ RU05 (v1.2 보존) | `nodeSchemas.ts`(`flow_picker` 원격 노드 선택기), `useFlowsTarget` 재사용, `subflowPorts.ts`(원격 핸들 계산), 원격 배지 | 예정 — 피커 원격 선택/배지/핸들 Vitest ⏳ |
| **REQ-SUBFLOW-RU06 (v1.3)** | flow-node 브리지 상태 표시기(`CustomNode`/배지) — online/offline + 브리지 라이프사이클 | 예정 — 브리지 상태 표시 Vitest ⏳ |
| REQ-SUBFLOW-RC01 ~ RC04 (v1.3 재작성) | 브리지 = 노드 실행 → 시크릿/device 정상(RC01/RC02 해소), 분산 실행(RC03), 잔존 한계 표면화(RC04 = RU06/RB08/RB11) | 예정 — 노드 실행 동작·잔존 한계 표면화 ⏳ |
