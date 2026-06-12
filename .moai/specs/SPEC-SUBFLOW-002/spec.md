---
id: SPEC-SUBFLOW-002
title: "서브플로우 의미론 재설계 — flow-node = 공유 인스턴스 연결(로컬 라이브 브리지)"
version: "1.0.0"
status: planned
created: "2026-06-12"
updated: "2026-06-12"
author: "xtra"
priority: high
related_specs:
  - SPEC-SUBFLOW-001
  - SPEC-FLOW-001
  - SPEC-FLOW-002
  - SPEC-WIRE-001
  - SPEC-ENGINE-001
  - SPEC-WEB-001
  - SPEC-REMOTE-001
tags:
  - subflow
  - flow-node
  - shared-instance
  - live-bridge
  - local-bridge
  - in-process-bridge
  - bridge-generalization
  - self-healing
  - fan-in
  - fan-out
  - lifecycle
  - cycle-detection
  - migration
  - breaking-change
  - editor
  - engine
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-12 | xtra | 초기 SPEC 작성. **flow-node 의미론 재설계** — SPEC-SUBFLOW-001 의 LOCAL 인라인 확장(인스턴스화)을 기본에서 폐지하고, flow-node 를 **"플로우 간 연결"** 기능으로 재정의한다. 신규 `mode` 키(`shared` 기본 / `instance` 옵션) 도입. `shared` 모드는 SPEC-SUBFLOW-001 v1.3 의 **원격 라이브 브리지 인프라를 in-process 로 일반화**하여 참조 플로우의 **단일 실행 인스턴스(플로우 리스트의 그 플로우)** 에 연결한다. 인라인 확장 금지·미실행 시 오프라인 대기·자가치유·다중참조 fan-in/fan-out·자동시작 없음. `mode` 미지정 → `shared` 해석(**breaking**). 기존 인라인 확장은 `instance` 명시로 보존. |

> **상태(Implementation Status)** — 2026-06-12, **계획(planned)**. 본 SPEC 은 설계 문서이며 코드 구현을 포함하지 않는다. 구현은 `/moai run SPEC-SUBFLOW-002` 으로 마일스톤 M1~M6 증분 진행한다.

> **SPEC-SUBFLOW-001 과의 관계(반드시 숙지)** — 본 SPEC 은 SPEC-SUBFLOW-001 을 **대체하지 않고 그 위에 의미론을 재정의**한다.
> - SPEC-SUBFLOW-001 의 **그룹 A~B(플로우 레벨 포트 모델·경계 포트 UI)·E(순환 검출)·N02(라우팅 보존)** 은 **전부 보존**된다.
> - SPEC-SUBFLOW-001 의 **그룹 D(LOCAL 배포 인스턴스화 = 인라인 확장)** 는 본 SPEC 에서 **`instance` 모드로 강등**된다 — 더 이상 LOCAL flow-node 의 기본 동작이 아니다.
> - SPEC-SUBFLOW-001 의 **그룹 RB(원격 라이브 브리지)·RU(원격 피커)·R01(`remote://` 데이터 모델)** 인프라는 **재사용·일반화**된다. 본 SPEC 의 `shared` 모드는 그 원격 브리지를 **로컬(in-process)** 로 확장한 것이다.

# SPEC-SUBFLOW-002: 서브플로우 의미론 재설계 — flow-node = 공유 인스턴스 연결(로컬 라이브 브리지)

## 1. 개요

### 1.1 목적

flow-node 를 **"플로우 간 연결(flow-to-flow link)"** 기능으로 재정의한다. flow-node 가 참조하는 플로우는 **플로우 리스트에 있는 그 플로우와 동일한(공유) 실행 인스턴스**여야 한다. 즉 부모 플로우는 참조 플로우의 **복사본(별도 인스턴스)** 을 만들지 않고, **이미 실행 중인 단일 인스턴스에 라이브로 연결**한다.

이를 위해 flow-node config 에 신규 `mode` 키를 도입한다.

1. **`shared`(기본값)** — flow-node 가 **로컬 in-process 라이브 브리지**로 참조 플로우의 **단일 실행 인스턴스**에 연결한다. 인라인 확장하지 않는다.
2. **`instance`(옵션)** — SPEC-SUBFLOW-001 의 인라인 확장(네임스페이스 복제본을 flow-node 마다 생성)을 그대로 수행한다(기존 동작 보존).

원격 참조(`remote://`)는 이미 라이브 브리지이며, **로컬 `shared` 모드는 이를 in-process 로 일반화한 것**이다.

### 1.2 문제 정의 (현재 구현의 한계)

SPEC-SUBFLOW-001 의 LOCAL flow-node 는 배포 시 `internal/api/service/subflow_expand.go` 의 `ExpandSubflows` 가 **로컬 참조(bare flow_id)를 인라인 확장**한다 — 참조 플로우의 노드/와이어를 부모로 복제하고 `subflow_<flowNodeID>_<originalID>` 로 네임스페이싱한다. 결과적으로 **참조 플로우의 별도 복사본(인스턴스)이 부모 안에서 돈다**.

이 모델의 한계:

- **별개 인스턴스**: 플로우 리스트의 플로우와 부모 안의 서브플로우가 서로 다른 실행 인스턴스다.
- **편집 반영 지연**: 참조 플로우를 편집해도 **부모를 재배포하기 전**에는 반영되지 않는다.
- **중복 실행**: 같은 플로우를 여러 곳에서 참조하면 **각각 별도 복사본**이 독립 실행된다(상태/리소스 비공유, 디바이스 다중 점유 가능).

반면 원격 참조(`remote://`)는 SPEC-SUBFLOW-001 v1.3 에서 **라이브 브리지**(그룹 RB, `internal/api/service/flow_bridge.go`·`remote_bridge_node.go`·`managerBridgeController` 자가치유 supervisor 등)로 **실행 중인 원격 플로우에 연결**한다 — 복사하지 않는다. 즉 **원격은 이미 올바른 의미론**을 가지며, 로컬만 인라인 확장이라는 다른 의미론을 가진다.

### 1.3 핵심 모델 (확정 — 사용자 승인)

> **중요**: 본 SPEC 은 아래 결정을 따른다. 로컬 `shared` 는 원격 라이브 브리지 인프라(SPEC-SUBFLOW-001 그룹 RB)를 in-process 로 일반화·재사용한다.

#### 결정 1 — flow-node = 연결 기능, 참조 플로우 = 공유 단일 인스턴스

flow-node 는 참조 플로우의 **복사본을 만들지 않는다**(`shared` 모드 기본). 부모 플로우는 참조 플로우의 **플로우 리스트에 있는 그 단일 실행 인스턴스**에 라이브 브리지로 연결한다. 부모 와이어 ↔ flow-node 입출력 핸들 ↔ 참조 플로우의 경계 포트(`__flow_input__`/`__flow_output__`) 사이로 메시지를 양방향 패싱한다.

#### 결정 2 — `mode` 옵션: `shared`(기본) / `instance`

flow-node config 에 신규 `mode` 키를 둔다.
- **`shared`** — 로컬 in-process 라이브 브리지(미확장, 단일 공유 인스턴스).
- **`instance`** — SPEC-SUBFLOW-001 의 인라인 확장(flow-node 마다 네임스페이스 복제본).
- **미지정** → `shared` 로 해석(**breaking**, §1.6 마이그레이션).

#### 결정 3 — 원격과 로컬 shared 의 공통 추상화

원격 참조(`remote://`)도 `shared` 모델의 한 형태다(이미 라이브 브리지). 로컬 `shared` 는 동일한 브리지 추상화(`FlowBridgeOpener`/`FlowBridge`/`managerBridgeController` 자가치유 supervisor)를 **전송만 in-process 로 바꿔** 재사용한다. 즉 transport 차이(WS 세션 vs in-process 채널)를 제외하면 라이프사이클·자가치유·포트 매핑·상태 표시가 동일하다.

#### 결정 4 — 생명주기: 자동 시작 없음 + 오프라인 대기 + 자가치유

`shared` 모드에서 참조 플로우는 **이미 실행 중이어야 한다(flow-node 가 자동 시작하지 않는다)**. 부모 시작 시 참조 플로우가 미실행이면 flow-node 는 **브리지 오프라인 상태**(원격 브리지 오프라인과 동일한 상태 표시)로 대기하고, 참조 플로우가 시작되면 **자가치유로 연결**된다. 참조 플로우가 독립적으로 정지되면 flow-node 는 **연결 끊김(오프라인)** 으로 표시되고, 재시작 시 자가치유된다. **참조 카운팅/자동 정지는 없다**(자동 시작이 없으므로).

#### 결정 5 — 다중 참조 = fan-in/fan-out (단일 인스턴스 공유)

같은 `flow_id` 를 가리키는 모든 `shared` flow-node 는 **동일한 실행 인스턴스**로 라우팅된다. 여러 부모(또는 한 부모 안 여러 flow-node)의 입력은 참조 플로우의 입력 경계 포트로 **fan-in** 되고, 참조 플로우의 출력 경계 포트는 모든 연결된 flow-node 로 **fan-out** 된다.

#### 결정 6 — 마이그레이션 = `mode` 미지정 → shared (breaking)

기존 로컬 flow-node(=`mode` 미지정)는 재배포 시 **`shared` 로 동작**한다(동작 변경, breaking). 기존 인라인 확장 동작을 원하면 **명시적으로 `instance` 를 설정**해야 한다.

### 1.4 배경 (코드 기준)

- **인라인 확장 진입점**: `internal/api/service/subflow_expand.go` 의 `ExpandSubflows`/`expandSubflowsRec` 가 LOCAL(bare id) flow-node 를 네임스페이스 복제·직접 재배선(`instantiateSubflow`/`cloneNodeWithPrefix`/`cloneWireWithPrefix`)으로 인라인 확장한다. REMOTE(`remote://`) flow-node 는 **확장하지 않고 LIVE NODE 로 보존**한다(미확장 분기 이미 존재).
- **원격 라이브 브리지 인프라(재사용 대상)**:
  - `internal/api/service/remote_bridge_node.go` — `managerBridgeController`(자가치유 supervisor: `start`/`supervise`/`reopen`/`runGeneration`/`pumpOutputs`/`pumpStatus`), `managerBridgeTable`, `rewireRemoteBridges`. **오프라인 시작 → 노드 도착 시 자동 open → 드롭 시 bounded 백오프 재open** 이 단일 코드 경로로 구현되어 있다(SPEC-SUBFLOW-002 의 로컬 shared 생명주기와 **의미론이 동일**).
  - `internal/api/service/server_bridge_opener.go` — `FlowBridgeOpener`/`FlowBridge` 를 `remote.Server`/`remote.ServerBridge` 위로 어댑트. 본 SPEC 은 동일 인터페이스의 **in-process opener** 를 추가한다.
  - `internal/api/service/flow_bridge.go`, `internal/remote/bridge_runner.go`, `internal/node/remote_bridge_node.go` — 노드 측 브리지 실행·경계 포트 탭(tap).
  - `internal/remote/protocol.go` — `bridge_open`/`bridge_input`/`bridge_output`/`bridge_status`/`bridge_close` 메시지 타입(원격 전송용; 로컬 shared 는 동일 의미를 in-process 채널로 구현).
- **경계 포트(tap 지점)**: `pkg/flow/boundary.go`(센티넬 `__flow_input__`/`__flow_output__`·`StripBoundaryWires`·`RebuildFlow`), `internal/api/service/bridge_tap_nodes.go`(브리지 배포 시 경계 와이어 재배선).
- **flow-node config 키**: `internal/node/flow_node.go` `flowNodeConfigFlowID = "flow_id"`. 본 SPEC 은 동일 위치에 `mode` 키를 추가한다.
- **순환 검출**: `internal/api/service/subflow_cycle.go` `DetectFlowReferenceCycle`(저장·배포 경로). `shared` 참조도 동일 그래프에 포함하여 순환을 검출한다.
- **통계**: 기존 subflow-stats(임베디드 병합)는 `instance` 모드 인라인 확장 노드 통계를 부모로 병합한다. `shared` 모드는 참조 플로우 자체의 통계를 직접 사용한다.

### 1.5 범위

**포함(백엔드):**
- flow-node config 의 신규 `mode` 키(`shared`|`instance`) 파싱/기본값(미지정→`shared`).
- 배포 분기: `shared`(로컬) = **미확장 + in-process 라이브 브리지**, `instance` = **기존 인라인 확장**, `remote://` = **기존 원격 라이브 브리지**.
- 로컬 `shared` 브리지: 기존 `FlowBridgeOpener`/`FlowBridge`/`managerBridgeController` 추상화를 in-process 로 일반화(원격 WS opener 와 대칭하는 in-process opener).
- 생명주기/자가치유: 참조 플로우 실행 상태 변화 감지(엔진 라이프사이클 훅 또는 폴링) → 브리지 open/재연결/teardown. 자동 시작 없음. 오프라인 대기.
- 다중참조 fan-in/fan-out: 같은 `flow_id` 의 `shared` flow-node 가 동일 실행 인스턴스로 라우팅.
- 순환 검출: `shared` 참조 그래프 포함(저장·배포).
- 통계 정합: `shared` = 참조 플로우 자체 통계 직접 사용, `instance` = 기존 임베디드 병합 유지.

**포함(프론트):**
- flow-node 설정 패널에 `mode` 토글(공유/인스턴스).
- `shared` 연결 상태 인디케이터(원격 브리지 상태 표시기 RU06 과 일관: 미연결/연결됨·실행중/오프라인/오류).
- `shared` 모드 안내: "참조 플로우 편집은 그 플로우 재시작 시 반영"을 사용자가 이해하도록 표시.

**제외(비목표):**
- flow-node 가 참조 플로우를 **자동 시작** 하는 기능(결정 4 — 자동 시작 없음).
- 참조 카운팅 기반 자동 정지/가비지 컬렉션.
- `shared` 인스턴스별 파라미터/환경 변수 오버라이드(공유 인스턴스이므로 구조적으로 불가 — 필요 시 `instance` 모드).
- 오프라인 동안 입력 버퍼링/리플레이(오프라인 = 무출력 + 상태 표시; SPEC-SUBFLOW-001 RB09 와 일관).
- 원격→로컬 역브리지/미러(SPEC-SUBFLOW-001 비목표 유지).
- 기존 `instance`(인라인 확장)·`remote://`(원격 브리지) 동작 자체의 변경(보존만).

### 1.6 마이그레이션 & breaking 영향 (확정)

- **`mode` 미지정 = `shared` 해석**: 기존에 저장된 로컬 flow-node(`mode` 키 없음)는 재배포 시 `shared` 로 동작한다 → **인라인 복사본 대신 단일 공유 인스턴스 연결**로 동작이 바뀐다(**breaking**).
- **영향 1 — 인스턴스 격리 의존 플로우**: 같은 플로우를 여러 flow-node 로 참조하며 **독립 상태/독립 실행을 기대**하던 구성은 동작이 달라진다(fan-in/fan-out 으로 단일 인스턴스 공유). 이런 구성은 **명시적으로 `mode: instance`** 로 전환해야 기존 동작이 유지된다.
- **영향 2 — 참조 플로우 미실행 시**: 기존(인라인 확장)은 부모 배포만으로 서브그래프가 함께 실행됐다. `shared` 는 참조 플로우가 **별도로 실행되어 있어야** 데이터가 흐른다(미실행 시 오프라인 대기·무출력). 운영 절차 변경이 필요하다.
- **영향 3 — 통계 화면**: `instance` 는 임베디드 병합(기존 subflow-stats) 유지, `shared` 는 참조 플로우 자체 통계로 표시된다 → 화면 의미가 모드별로 달라진다(§4.7 그룹 S).
- **하위 호환 보존 경로**: `mode: instance` 명시 = SPEC-SUBFLOW-001 LOCAL 동작 그대로(regression-0). `remote://` 참조 = 기존 원격 브리지 그대로.

## 2. 환경

| 항목 | 상세 |
|------|------|
| 백엔드 런타임 | Go 1.23+ |
| 백엔드 대상 모듈 | `internal/api/service` (subflow_expand 배포 분기·flow_adapter·remote_bridge_node 일반화·server_bridge_opener 대칭 in-process opener·subflow_cycle), `internal/node` (flow_node `mode` config), `internal/engine` (참조 플로우 실행 상태 라이프사이클 훅/조회), `pkg/flow` (경계 포트·boundary 재사용) |
| 프론트 런타임 | TypeScript 5.9+, React 19, `@xyflow/react` (React Flow) |
| 프론트 대상 모듈 | `web/src/config/nodeSchemas.ts` (flow-node `mode` 필드·핸들 계산), flow-node 설정 패널(mode 토글), 브리지 상태 인디케이터(RU06 재사용/일반화), `web/src/lib/i18n/*.json` |
| 테스트 | Go `testing`+`testify`, 프론트 Vitest + Testing Library |
| 개발 방법론 | Hybrid (신규 = TDD, 기존 변경 = 동작 보존 DDD) |

## 3. 가정

- **A1**: SPEC-SUBFLOW-001 의 플로우 레벨 포트 모델(`Inputs()`/`Outputs()`, 경계 센티넬 `__flow_input__`/`__flow_output__`)이 구현되어 있다. 본 SPEC 의 로컬 shared 브리지는 이 경계 포트를 **탭(tap) 지점**으로 그대로 사용한다.
- **A2**: SPEC-SUBFLOW-001 v1.3 의 원격 라이브 브리지 추상화(`FlowBridgeOpener`/`FlowBridge`/`managerBridgeController` 자가치유 supervisor)가 구현되어 있다. 로컬 shared 는 **동일 인터페이스의 in-process opener** 를 추가하여 재사용한다(transport 만 교체).
- **A3**: 엔진(`internal/engine`)은 실행 중인 플로우의 인스턴스를 `flows map[string]*flowRuntime` 으로 보유한다. 로컬 shared 브리지는 이 맵에서 참조 플로우의 **실행 인스턴스를 조회**하여 경계 포트에 in-process 로 연결한다. 미존재(미실행)는 **오프라인** 상태로 취급한다.
- **A4**: 참조 플로우의 실행 상태 변화(deploy/start/stop/undeploy)는 라이프사이클 훅 또는 폴링으로 감지 가능하다(자가치유 트리거).
- **A5**: flow-node config 의 `mode` 키는 `NodeDef.Config["mode"]` 로 통과하며(`flow_id` 와 동일 경로), 미지정 시 기본 `shared` 로 정규화한다.
- **A6**: 순환 검출 그래프는 `shared`/`instance` 참조 모두를 포함한다(`flow_id` 의 LOCAL 참조 그래프). `shared` 는 매니저 인라인 확장이 없어 무한 확장 위험은 없으나, **연결 순환(A↔B)** 자체는 의미 모호·무한 패싱 위험이 있으므로 배포 시 거부한다(결정과 정합).

## 4. 요구사항 (EARS)

### 4.1 그룹 M — flow-node mode (config + 기본값)

**REQ-SUBFLOW2-M01**: mode config 모델
시스템은 **항상** flow-node config 에 참조 실행 모드를 나타내는 `mode` 키(`shared`|`instance`)를 보유할 수 있어야 한다.

**REQ-SUBFLOW2-M02**: mode 기본값 = shared
**IF** flow-node config 에 `mode` 가 지정되지 않으면, **THEN** 시스템은 이를 `shared` 로 해석해야 한다(저장·배포·렌더 전 경로 일관).

**REQ-SUBFLOW2-M03**: mode 결정성
시스템은 **항상** `mode` 값을 결정적으로 판별해야 한다(`shared`/`instance` 외의 값은 명확한 에러로 거부하거나 기본 `shared` 로 정규화하되, 동작은 일관·문서화되어야 한다).

**REQ-SUBFLOW2-M04**: remote 참조와의 직교성
시스템은 **항상** `mode` 를 `flow_id` 의 참조 종류(LOCAL bare id / REMOTE `remote://`)와 **직교**하게 처리해야 한다. `remote://` 참조는 종류상 라이브 브리지이며(원격 shared), 로컬 bare id 의 `mode` 가 `shared`/`instance` 를 구분한다.

### 4.2 그룹 SH — shared 모드 (로컬 라이브 브리지)

**REQ-SUBFLOW2-SH01**: 인라인 확장 금지
**WHEN** `mode=shared` 인 LOCAL flow-node 를 포함한 부모 플로우를 배포할 때, **THEN** 시스템은 그 flow-node 를 **인라인 확장(네임스페이스 복제)하지 않아야 한다**. flow-node 는 라이브 브리지 엔드포인트(LIVE NODE)로 보존된다.

**REQ-SUBFLOW2-SH02**: 단일 공유 인스턴스 연결
시스템은 **항상** `mode=shared` flow-node 를 참조 플로우의 **단일 실행 인스턴스(플로우 리스트의 그 플로우)** 에 **로컬 in-process 라이브 브리지**로 연결해야 한다. 복사본을 생성하지 않는다.

**REQ-SUBFLOW2-SH03**: 양방향 포트 라우팅
시스템은 **항상** `shared` 브리지에서 (a) 부모 와이어 → flow-node 입력 핸들 → 참조 플로우의 입력 경계 포트(`__flow_input__`), (b) 참조 플로우의 출력 경계 포트(`__flow_output__`) → flow-node 출력 핸들 → 부모 와이어로 메시지를 **경계 포트 이름 기준으로 매핑**하여 양방향 라우팅해야 한다.

**REQ-SUBFLOW2-SH04**: 원격/로컬 공통 추상화 재사용
시스템은 **항상** 로컬 `shared` 브리지를 SPEC-SUBFLOW-001 v1.3 의 원격 브리지 추상화(`FlowBridgeOpener`/`FlowBridge`/`managerBridgeController`)와 **동일한 인터페이스**로 구현해야 한다(전송만 in-process 로 교체). 라이프사이클·자가치유·상태 의미가 원격과 일관되어야 한다.

**REQ-SUBFLOW2-SH05**: 항상 최신 (편집 반영 = 재시작 시)
시스템은 **항상** `shared` flow-node 가 참조 플로우의 **현재 실행 중인 인스턴스**에 연결됨을 보장해야 한다. 참조 플로우 정의를 편집하면 그 변경은 **참조 플로우가 재시작(재배포)될 때** 실행 인스턴스에 반영되며, 부모는 재시작 없이 갱신된 인스턴스에 자동 재연결(자가치유)되어야 한다.

### 4.3 그룹 MR — 다중 참조 (fan-in / fan-out)

**REQ-SUBFLOW2-MR01**: 다중 참조 단일 인스턴스
시스템은 **항상** 같은 `flow_id` 를 가리키는 모든 `shared` flow-node(부모·flow-node 다수)를 **동일한 실행 인스턴스**로 라우팅해야 한다(별도 복사본 생성 금지).

**REQ-SUBFLOW2-MR02**: 입력 fan-in
**WHEN** 여러 `shared` flow-node 가 같은 참조 플로우의 동일 입력 경계 포트로 메시지를 보낼 때, **THEN** 시스템은 모든 입력을 그 단일 인스턴스의 입력 경계 포트로 **fan-in** 해야 한다(일반 와이어 다중 소스와 동일 의미).

**REQ-SUBFLOW2-MR03**: 출력 fan-out
**WHEN** 참조 플로우의 출력 경계 포트가 메시지를 emit 할 때, **THEN** 시스템은 그 메시지를 해당 출력 포트에 연결된 **모든** `shared` flow-node 의 출력 핸들로 **fan-out** 해야 한다.

**REQ-SUBFLOW2-MR04**: 인터리빙 의미 명시
시스템은 **항상** 다중 fan-in 시 메시지 인터리빙이 일반 플로우의 다중 소스 와이어와 **동일한 순서·큐 의미**를 따르도록 해야 한다(브리지가 별도 재정렬·합류 규칙을 도입하지 않는다).

### 4.4 그룹 L — 생명주기 / 자가치유

**REQ-SUBFLOW2-L01**: 자동 시작 없음
시스템은 **항상** `shared` flow-node 가 참조 플로우를 **자동으로 시작하지 않아야 한다**. 참조 플로우는 독립적으로 실행되어 있어야 한다.

**REQ-SUBFLOW2-L02**: 미실행 시 오프라인 대기
**IF** 부모 플로우 시작 시 참조 플로우가 미실행이면, **THEN** 시스템은 flow-node 를 **브리지 오프라인 상태**(원격 브리지 오프라인과 동일 표시)로 대기시키고 데이터를 패싱하지 않아야 한다(무출력). 배포 자체는 실패하지 않아야 한다.

**REQ-SUBFLOW2-L03**: 시작 감지 자가치유
**WHEN** 오프라인 대기 중 참조 플로우가 시작(실행)되면, **THEN** 시스템은 이를 감지하여 브리지를 **자동 연결(self-healing)** 하고 데이터 패싱을 시작해야 한다(원격 브리지의 노드 도착 자가치유와 동일 경로).

**REQ-SUBFLOW2-L04**: 정지 시 연결 끊김
**WHEN** 참조 플로우가 독립적으로 정지(undeploy/stop)되면, **THEN** 시스템은 그 플로우를 쓰는 부모의 flow-node 를 **연결 끊김(오프라인)** 으로 표시하고 데이터 패싱을 중단해야 한다.

**REQ-SUBFLOW2-L05**: 재시작 자가치유
**WHEN** 정지되었던 참조 플로우가 다시 시작되면, **THEN** 시스템은 부모 재시작 없이 브리지를 **자동 재연결(self-healing)** 해야 한다(bounded 백오프 재open 경로 재사용).

**REQ-SUBFLOW2-L06**: 참조 카운팅 없음
시스템은 **항상** 참조 카운팅 기반 자동 정지를 수행하지 않아야 한다(자동 시작이 없으므로). 부모 undeploy 시 그 부모의 브리지 엔드포인트만 teardown 하며, 참조 플로우 인스턴스 자체의 실행에는 영향을 주지 않아야 한다.

**REQ-SUBFLOW2-L07**: 부모 teardown 격리
**WHEN** 부모 플로우가 undeploy 되면, **THEN** 시스템은 그 부모의 `shared` 브리지만 teardown 하고, 같은 참조 플로우를 쓰는 **다른 부모의 브리지·참조 플로우 인스턴스**에는 영향을 주지 않아야 한다.

### 4.5 그룹 IN — instance 모드 (기존 인라인 확장 보존)

**REQ-SUBFLOW2-IN01**: 인라인 확장 보존
**WHEN** `mode=instance` 인 LOCAL flow-node 를 포함한 부모 플로우를 배포할 때, **THEN** 시스템은 SPEC-SUBFLOW-001 그룹 D 의 인라인 확장(네임스페이스 복제 + 직접 재배선)을 **기존과 동일하게** 수행해야 한다(regression-0).

**REQ-SUBFLOW2-IN02**: instance 인스턴스 독립성
시스템은 **항상** `mode=instance` flow-node 를 SPEC-SUBFLOW-001 REQ-SUBFLOW-D03 과 동일하게 상태 비공유 독립 인스턴스로 확장해야 한다(네임스페이스 접두사에 flow-node ID 포함).

**REQ-SUBFLOW2-IN03**: 모드 혼합 배포
**WHEN** 한 부모 플로우가 `shared` 와 `instance` flow-node 를 **혼합** 포함할 때, **THEN** 시스템은 각 flow-node 를 그 `mode` 에 따라 분기 처리해야 한다(`shared`=미확장 브리지, `instance`=인라인 확장)하며, 두 처리가 서로 간섭하지 않아야 한다.

### 4.6 그룹 MG — 마이그레이션 (breaking)

**REQ-SUBFLOW2-MG01**: 미지정 → shared (breaking)
시스템은 **항상** `mode` 미지정 LOCAL flow-node 를 재배포 시 `shared` 로 동작시켜야 한다(인라인 확장 아님). 이는 의도된 breaking 변경이다.

**REQ-SUBFLOW2-MG02**: instance 명시로 기존 동작 유지
**WHEN** 사용자가 flow-node 에 `mode: instance` 를 명시하면, **THEN** 시스템은 SPEC-SUBFLOW-001 의 인라인 확장 동작을 그대로 보존해야 한다(하위 호환 경로).

**REQ-SUBFLOW2-MG03**: remote 참조 불변
시스템은 **항상** `remote://` 참조 flow-node 의 동작(원격 라이브 브리지)을 본 변경과 무관하게 그대로 유지해야 한다.

### 4.7 그룹 S — 통계 정합

**REQ-SUBFLOW2-S01**: shared 통계 = 참조 플로우 직접
시스템은 **항상** `mode=shared` flow-node 의 통계를 **참조 플로우 자체의 통계**로 직접 표시해야 한다(임베디드 병합 불필요 — 참조 플로우가 단일 공유 인스턴스이므로).

**REQ-SUBFLOW2-S02**: instance 통계 = 임베디드 병합 유지
시스템은 **항상** `mode=instance` flow-node 의 통계를 기존 subflow-stats 임베디드 병합(확장 노드 통계를 부모로 병합)으로 유지해야 한다.

**REQ-SUBFLOW2-S03**: 통계 모드 구분 표시
시스템은 **항상** 통계 화면에서 `shared`(참조 플로우 직접)와 `instance`(병합)의 의미 차이를 운영자가 혼동하지 않도록 모드를 식별 가능하게 표시해야 한다.

### 4.8 그룹 CY — 순환 참조

**REQ-SUBFLOW2-CY01**: shared 순환 거부
**IF** `shared` 참조를 포함한 flow-node 참조 그래프에 순환(직접 자기참조 또는 A↔B, A→B→C→A 등)이 존재하면, **THEN** 시스템은 저장 및 배포를 거부하고 순환 경로를 포함한 에러를 반환해야 한다(SPEC-SUBFLOW-001 E01~E04 와 정합).

**REQ-SUBFLOW2-CY02**: 혼합 그래프 순환 검출
시스템은 **항상** 순환 검출 그래프에 `shared`·`instance` 참조를 함께 포함하여, 모드가 섞인 순환도 검출해야 한다(`flow_id` 기준 참조 그래프).

### 4.9 그룹 P — 포트

**REQ-SUBFLOW2-P01**: 핸들 = 참조 플로우 경계 포트
시스템은 **항상** flow-node 의 입출력 핸들을 참조 플로우의 경계 포트(`Inputs()`/`Outputs()`, `input_ports`/`output_ports`)에서 파생해야 한다(SPEC-SUBFLOW-001 REQ-SUBFLOW-C02 유지). `mode` 와 무관하게 핸들 파생 규칙은 동일하다.

**REQ-SUBFLOW2-P02**: 사라진 포트 dangling
**IF** 참조 플로우에서 경계 포트가 삭제되어 flow-node 핸들이 사라지면, **THEN** 시스템은 그 핸들에 걸린 부모 와이어를 dangling 으로 표시(경고)하고 정리 경로를 제공해야 한다(SPEC-SUBFLOW-001 REQ-SUBFLOW-C05 유지).

### 4.10 그룹 W — 웹 UI

**REQ-SUBFLOW2-W01**: mode 토글
시스템은 **항상** flow-node 설정에서 `shared`(공유)와 `instance`(인스턴스)를 선택하는 모드 토글을 제공해야 한다(기본 선택 = `shared`).

**REQ-SUBFLOW2-W02**: shared 연결 상태 인디케이터
시스템은 **항상** `shared` flow-node 의 브리지 연결 상태(미연결/연결됨·실행중/오프라인/오류)를 시각적으로 표시해야 한다. 이 인디케이터는 원격 브리지 상태 표시기(SPEC-SUBFLOW-001 REQ-SUBFLOW-RU06)와 **일관된 시각 언어**를 사용해야 한다.

**REQ-SUBFLOW2-W03**: 편집 반영 안내
시스템은 **항상** `shared` 모드일 때 "참조 플로우 편집은 그 플로우 재시작 시 반영됨"을 사용자가 이해하도록 설정 패널/툴팁에 안내해야 한다.

**REQ-SUBFLOW2-W04**: 오프라인 무출력 식별
시스템은 **항상** 참조 플로우가 미실행/오프라인일 때 flow-node 가 데이터를 패싱하지 않음(무출력)을 운영자가 식별할 수 있도록 상태를 표시해야 한다(REQ-SUBFLOW2-L02/L04 와 일관).

### 4.11 비기능 요구사항

**REQ-SUBFLOW2-N01**: 회귀 0 (기존 경로 보존)
시스템은 **항상** `mode: instance` 명시 LOCAL flow-node·`remote://` 참조 flow-node·flow-node 미포함 플로우의 동작을 기존과 동일하게 유지해야 한다(회귀 0).

**REQ-SUBFLOW2-N02**: 라우팅 동작 보존
시스템은 **항상** `shared` 브리지를 통과한 메시지가 일반 와이어와 동일한 라우팅·큐·카운터 의미로 동작하도록 해야 한다(브리지가 메시지 의미를 변경하지 않는다).

**REQ-SUBFLOW2-N03**: 백프레셔/유계 버퍼
시스템은 **항상** `shared` 브리지의 포트별 버퍼를 유계로 두고 오버플로 정책(예: oldest-drop/coalesce — SPEC-SUBFLOW-001 RB10 과 일관)을 적용하여 메모리 폭증을 방지해야 한다.

**REQ-SUBFLOW2-N04**: 자가치유 핫스핀 방지
시스템은 **항상** 오프라인 대기·재연결 시 bounded 백오프를 적용하여 핫스핀(busy retry)을 방지해야 한다(원격 브리지 `reopen` 백오프 경로 재사용).

## 5. 명세

### 5.1 mode 데이터 계약

- flow-node config: `flow_id`(기존) + `mode`(신규, `shared`|`instance`, 미지정→`shared`).
- 정규화 위치: `internal/node/flow_node.go`(config 키 상수) + 배포 분기 진입점(`subflow_expand.go` 또는 배포 어댑터). 미지정 시 기본 `shared` 로 정규화한 뒤 분기.
- 직교성: `flow_id` 가 `remote://` 면 종류상 원격 라이브 브리지(원격 shared), bare id 면 `mode` 로 로컬 `shared`/`instance` 결정.

### 5.2 배포 분기 (subflow_expand)

배포 시 각 flow-node 를 다음으로 분기한다.

| 참조 종류 | mode | 처리 | 비고 |
|-----------|------|------|------|
| LOCAL (bare id) | `shared`(기본) | **미확장 + in-process 라이브 브리지**(LIVE NODE 보존) | 본 SPEC 신규 |
| LOCAL (bare id) | `instance` | **인라인 확장**(네임스페이스 복제 + 직접 재배선) | SPEC-SUBFLOW-001 그룹 D 보존 |
| REMOTE (`remote://`) | (직교, 항상 브리지) | **원격 라이브 브리지**(WS 세션) | SPEC-SUBFLOW-001 그룹 RB 보존 |

- `expandSubflowsRec` 의 flow-node 분류 단계에서 LOCAL flow-node 를 `mode` 로 다시 분류: `instance` → 기존 `localFlowNodeIDs`(인라인 확장 대상), `shared` → REMOTE 와 동일하게 LIVE NODE 보존(미확장).
- `shared` LIVE NODE 는 매니저 측 엔진 통합(`remote_bridge_node.go` 의 `rewireRemoteBridges` 일반화)에서 in-process opener 로 브리지 엔드포인트화한다.

### 5.3 로컬 in-process 브리지 추상화 일반화

- 기존 `FlowBridgeOpener`/`FlowBridge` 인터페이스(`server_bridge_opener.go`)를 그대로 사용하되, **in-process opener** 를 추가한다:
  - `OpenBridge(ctx, <local flow_id>, "", inputPorts, outputPorts)` → 엔진 `flows` 맵에서 참조 플로우의 실행 인스턴스를 조회하여 경계 포트(`__flow_input__`/`__flow_output__`)에 in-process 채널로 연결.
  - 참조 플로우 미실행 → opener 가 transient 오류 반환 → `managerBridgeController` 가 오프라인 대기(REQ-SUBFLOW2-L02) 후 자가치유.
- `managerBridgeController` 의 자가치유 supervisor(`start`/`supervise`/`reopen`/`runGeneration`/`pumpOutputs`/`pumpStatus`)는 **그대로 재사용**한다. 차이는 opener 의 transport 뿐이다(WS vs in-process).
- 식별자: in-process 는 `instance_id` 가 없으므로(=로컬 매니저 자기 자신), 브리지 키는 `<parent flow_id>:<flowNodeID>:<ref flow_id>` 로 구성하여 부모별 독립 엔드포인트를 유지한다(REQ-SUBFLOW2-L07).

### 5.4 경계 포트 매핑 (양방향)

- 입력: 부모 와이어 → flow-node 입력 핸들 `inPortName` → 참조 플로우 `__flow_input__` 의 `inPortName` 경계 포트(in-process `SendInput`).
- 출력: 참조 플로우 `__flow_output__` 의 `outPortName` 경계 포트 → 브리지 `Outputs()` → flow-node 출력 핸들 `outPortName` → 부모 하류 와이어(엔진 emit).
- 매핑 키 = 경계 포트 **이름**(SPEC-SUBFLOW-001 RB06 동일 규칙). 핸들은 참조 플로우 `Inputs()`/`Outputs()` 에서 파생(REQ-SUBFLOW2-P01).
- fan-in/fan-out: 같은 참조 플로우 인스턴스의 경계 포트에 **여러 브리지 엔드포인트**가 attach 되며, 입력은 인스턴스 입력 경계로 합류(fan-in), 출력은 attach 된 모든 엔드포인트로 복제(fan-out).

### 5.5 생명주기/자가치유 트리거

- 참조 플로우 실행 상태 변화 감지: 엔진 라이프사이클 훅(deploy/start/stop/undeploy 이벤트) 우선, 미가용 시 폴링 폴백.
- 상태 머신(원격 브리지와 동일):
  - `offline`(참조 플로우 미실행) → open 재시도(백오프).
  - `connected`(인스턴스 존재 + 경계 attach) → 데이터 패싱.
  - `connected → offline`(참조 플로우 stop/undeploy) → teardown + 재open 백오프.
  - 부모 undeploy → 엔드포인트만 teardown(참조 인스턴스 불변, REQ-SUBFLOW2-L06/L07).

### 5.6 통계 정합

- `shared`: 통계 화면은 flow-node 클릭 시 **참조 플로우 자체의 통계 뷰**로 연결(또는 인라인 표시). 임베디드 병합 비활성.
- `instance`: 기존 subflow-stats 임베디드 병합 경로 유지.
- 모드 배지로 화면 의미 구분(REQ-SUBFLOW2-S03).

### 5.7 파일 구조 (예상 — 구현 시 확정)

- `internal/node/flow_node.go` — `mode` config 키 상수/정규화.
- `internal/api/service/subflow_expand.go` — LOCAL flow-node 의 `mode` 분기(`shared`=미확장, `instance`=확장).
- `internal/api/service/remote_bridge_node.go`(+ 신규 `local_bridge_opener.go` 등) — `managerBridgeController`/`rewireRemoteBridges` 일반화, in-process opener.
- `internal/api/service/server_bridge_opener.go` — `FlowBridgeOpener`/`FlowBridge` 인터페이스 재사용(대칭 in-process 구현 추가).
- `internal/api/service/subflow_cycle.go` — `shared` 참조 그래프 포함(기존 그래프 재사용).
- `internal/engine/*` — 참조 플로우 실행 인스턴스 조회/라이프사이클 훅.
- `web/src/config/nodeSchemas.ts` — flow-node `mode` 필드·핸들 계산.
- flow-node 설정 패널/상태 인디케이터(프론트) — mode 토글·shared 연결 상태(RU06 재사용).

### 5.8 OPEN QUESTIONS → DECIDED (사용자 승인)

1. **mode 기본값** — ✅ `shared`(미지정 시), breaking 수용.
2. **로컬 shared 전송** — ✅ in-process(기존 원격 브리지 추상화 일반화, WS 미사용).
3. **자동 시작** — ✅ 안 함(참조 플로우는 독립 실행, 오프라인 대기+자가치유).
4. **참조 카운팅/자동 정지** — ✅ 없음.
5. **다중 참조** — ✅ 단일 인스턴스 fan-in/fan-out.
6. **순환** — ✅ shared 도 순환 거부(저장·배포; 기존 가드 정합).
7. **통계** — ✅ shared=참조 플로우 직접, instance=기존 병합.

#### 향후 SPEC 으로 분리 (본 SPEC 범위 외)

- `shared` 인스턴스별 파라미터/환경 오버라이드(공유 인스턴스이므로 구조적 비목표).
- 오프라인 동안 입력 버퍼링/리플레이.
- 원격→로컬 역브리지/미러.
- `shared` 자동 시작 옵션(필요 시 별도 SPEC).

## 6. 추적성

| 요구사항 | 대상 모듈(예상) | 검증 |
|----------|----------------|------|
| REQ-SUBFLOW2-M01 ~ M04 | `internal/node/flow_node.go`, `subflow_expand.go` | flow_node_test, subflow_expand_test |
| REQ-SUBFLOW2-SH01 ~ SH05 | `subflow_expand.go`, `remote_bridge_node.go`(일반화), in-process opener | subflow_expand_test, bridge_test |
| REQ-SUBFLOW2-MR01 ~ MR04 | in-process opener, 경계 포트 attach | fan-in/fan-out 통합 테스트 |
| REQ-SUBFLOW2-L01 ~ L07 | `managerBridgeController`, 엔진 라이프사이클 훅 | 오프라인/자가치유/teardown 테스트 |
| REQ-SUBFLOW2-IN01 ~ IN03 | `subflow_expand.go`(인라인 확장 보존) | 기존 subflow_expand_test 회귀 |
| REQ-SUBFLOW2-MG01 ~ MG03 | mode 정규화·배포 분기 | 마이그레이션 테스트(미지정→shared) |
| REQ-SUBFLOW2-S01 ~ S03 | 통계 경로, 프론트 통계 뷰 | 통계 정합 테스트 |
| REQ-SUBFLOW2-CY01 ~ CY02 | `subflow_cycle.go` | 순환 거부 테스트(shared/혼합) |
| REQ-SUBFLOW2-P01 ~ P02 | `nodeSchemas.ts`, 핸들 파생 | 핸들/dangling 테스트 |
| REQ-SUBFLOW2-W01 ~ W04 | flow-node 설정 패널, 상태 인디케이터 | Vitest UI 테스트 |
| REQ-SUBFLOW2-N01 ~ N04 | 전 경로 | 회귀·백프레셔·백오프 테스트 |
