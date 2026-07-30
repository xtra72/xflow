---
id: SPEC-TRIGGER-PANEL-001
title: "Trigger 노드 대시보드 패널 (스케줄 설정 + 페이로드 카탈로그 + 노드 맵핑)"
version: "0.2.0"
status: draft
created: 2026-07-31
updated: 2026-07-31
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/node + web/src/pages/dashboard"
lifecycle: spec-anchored
tags: "trigger, dashboard, panel, schedule, payload-catalog, node-mapping, live-reconfigure, dual-write, configureNode, updateFlow, frontend, backend"
tier: L
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                                                                                                                                                                                                       |
| ---------- | ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-31 | 0.1.0 | 초기 SPEC 작성 — Trigger 노드용 대시보드 패널 도입. (1) 대시보드에서 특정 trigger 노드를 타겟팅하여 스케줄 CRUD, (2) 패널 config 레벨 페이로드 카탈로그 정의 + 스케줄에 이름으로 배정 시 인라인 주입, (3) `TriggerNode.Configure` live 타이머 재등록(cancel+re-register), (4) dual-write 지속성(configureNode live + updateFlow persist), (5) 6개 스케줄 타입 편집 UI. RD-1~5 확정 반영, OQ-1~6 미해결. |
| 2026-07-31 | 0.2.0 | OQ-1~6 엔지니어링 기본값 확정 → RD-6~11 승격, §6 Open Questions 전면 해소(잔여 없음). (RD-6) 재등록 no-double-fire 를 **re-arm generation 토큰**으로 확정(stale in-flight fire drop), (RD-7) 빈 스케줄 live Configure 는 전 타이머 취소 후 valid IDLE(오류 아님), (RD-8) 동시 flow 편집은 last-write-wins + 대시보드 통지(낙관적 잠금은 향후 SPEC), (RD-9) 카탈로그는 패널-로컬(`config.payloadCatalog`) 확정(공유 저장소 out of scope), (RD-10) 대시보드는 기존 flow API(useFlows/useFlowNodes read, updateFlow write) 재사용 + running/stopped 배지 + 404 시 persist-only+통지, (RD-11) 카탈로그 배정은 **스냅샷 주입**(카탈로그 후속 편집이 기주입 스케줄을 소급 변경하지 않음; 재선택 시에만 갱신). 관련 EARS(REQ-01-02/05, REQ-04-04, REQ-05-03/05) 및 §4 사양 갱신. |

---

# SPEC-TRIGGER-PANEL-001: Trigger 노드 대시보드 패널

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이다. `internal/node/trigger.go` 의 **Trigger 노드**는 스케줄(interval/cron/once/times/weekly/monthly)에 따라 메시지를 방출하는 SourceNode 이다. 현재 스케줄·페이로드 설정은 **flow 에디터의 속성 위젯**(`web/src/components/property/TriggerScheduleEditor.tsx`)에서만 편집 가능하며, 편집 후에는 flow 재배포를 거쳐야 반영된다.

본 SPEC 은 그 위에 **대시보드에서 직접 trigger 노드를 운영**할 수 있는 전용 패널을 도입한다. 즉 (1) 대시보드 패널이 특정 trigger 노드를 타겟팅하고, (2) 스케줄을 편집하면 **즉시(live)** 노드에 반영되며, (3) 동시에 flow 정의로 **지속화(persist)**되어 재배포 후에도 유지된다. 페이로드는 패널 레벨 카탈로그에 이름으로 정의하고 스케줄에 배정한다.

### 1.2 기술 환경

- **백엔드**: Go 1.23+ / `internal/node/trigger.go` (기존 `TriggerNode` 확장), 런타임 재설정 경로 `internal/api/handler/flow.go` + `internal/engine/engine.go`.
- **프런트엔드**: React + TypeScript (`web/src/`) — 대시보드 패널 시스템(`stores/uiStore.ts`, `pages/dashboard/renderDashboardPanel.tsx`, `components/dashboard/AddPanelDialog.tsx`), 노드 서비스(`services/api/nodeService.ts`), flow 서비스(`services/api/flowService.ts`), 훅(`hooks/useNodeTypeInstances.ts`, `hooks/useFlow.ts`).
- **테스트**: 백엔드 Go testify(`internal/node/trigger_test.go`), 프런트 vitest(`web/src/**/*.test.ts(x)`).

### 1.3 관련 기존 anchor (본 SPEC 이 정확히 참조·재사용, 조사 기준선)

- **Trigger 노드 스케줄 모델** (`internal/node/trigger.go`):
  - 스케줄 타입 상수 `TriggerScheduleType` — `trigger.go:46-64` (interval/cron/once/times/weekly/monthly).
  - per-schedule 구조체 `TriggerSchedule{Type,Value,Days,Times,Day, Payload, PayloadTmpl}` — `trigger.go:67-82`.
  - 노드 레벨 페이로드 필드 `payload`/`payloadTemplate` — `trigger.go:117-118`.
  - 파서 `parseScheduleConfig` — `trigger.go:196-241`.
  - 페이로드 해석 순서(per-schedule → 노드 레벨 → 기본) `buildMessage` — `trigger.go:728-756`.
  - **타이머 등록은 `Init()` 만 수행** — `trigger.go:279-334` (내부 `registerSchedules` `trigger.go:359-387`). register 실패 시 `cancelAllTimers()` 롤백.
  - **타이머 취소 헬퍼 이미 존재** — `cancelAllTimers()` `trigger.go:924-937` (`timerMu` 보호, 각 `entry.timerID` `Cancel` 후 `timerEntries=nil`). Stop 경로에서 호출.
  - **`Configure()` 는 스케줄/페이로드만 재파싱** — `trigger.go:951-995`. **타이머 취소·재등록을 하지 않음** → live Configure 가 오늘날 타이머를 재무장하지 않는 근본 원인.
- **런타임 노드 config 갱신 경로 (존재)**: `POST /flows/{id}/nodes/{nodeID}/configure` → `FlowService.ReconfigureFlowNode` → `engine.ReconfigureNode`(`engine.go:703-748`) → live `node.Configure`(persist 없음, 미실행 노드는 404). 프런트 클라이언트 `nodeService.ts:31-37 configureNode(flowId,nodeId,config)`, dual-write 선례 `components/flow/CustomNode.tsx:107-131`.
- **지속 경로 (별개 축)**: `PUT /flows/{id}` `UpdateFlow`(`flow_adapter.go:312-361`) — flow 정의 전체 재빌드·저장, 재배포 시 적용. 프런트 `flowService.updateFlow`(`flowService.ts:41`).
- **대시보드 패널** (`stores/uiStore.ts`): `PanelType` union(`175-199`, **flow/trigger 타입 없음**), `PanelConfig{id,type,title,config}`(`202-208`, config 자유형), `createDefaultPanel`/`panelDefaultSize`/`addPanelWithConfig`. 렌더 디스패치 `renderDashboardPanel.tsx:72-242`. 추가 다이얼로그 `AddPanelDialog.tsx`(**flow 노드를 타겟하는 패널 없음**). 패널 컴포넌트 계약 `{panelId,title,config,onConfigChange,onTitleChange}`.
- **노드 타겟팅 열거**: `useNodeTypeInstances(nodeType)`(`useNodeTypeInstances.ts:23-70`) — running 플로우별 `{flowId,flowName,nodeId,nodeName}` 반환. `useFlowNodes(flowId)`(`useFlow.ts:35`) — 노드 현재 config 조회.
- **명명 페이로드 저장소 없음** — 순수 신규(net-new). 본 SPEC 은 별도 백엔드 저장소를 만들지 않고 **패널 config 레벨**에 카탈로그를 둔다.

## 2. Assumptions (가정)

- **A-1**: 런타임 노드 config 갱신 경로(`configureNode` → `ReconfigureNode` → `node.Configure`)와 flow 정의 지속 경로(`updateFlow` → `PUT /flows`)는 이미 동작하며 본 SPEC 이 신규 구현하지 않는다(조사 기준선 §1.3).
- **A-2**: 대시보드는 flow 목록/노드/정의에 대한 read+write 권한을 가진다(`useFlows`/`useFlowNodes` read, `flowService.updateFlow` write). — RD-10 확정(기존 flow API 재사용, 신규 권한 계층 없음). 구현은 `updateFlow` 가 대시보드 컨텍스트에서 호출 가능함을 검증해야 한다.
- **A-3**: `TriggerNode.Configure` 가 running 노드에서 호출될 때 `n.timer` 는 이미 Init 에서 resolve 되어 non-nil 이다(런타임 `ReconfigureNode` 경로).
- **A-4**: 페이로드 카탈로그는 대시보드-로컬(패널 레이아웃) 상태이며, 패널·사용자 간 공유되지 않는다(별도 공유 저장소 신설 없음). — RD-4, RD-9 확정(공유 명명 페이로드 저장소는 out of scope, 향후 SPEC).
- **A-5**: 트리거 노드 페이로드 모델(inline payload)은 불변이다. 카탈로그는 편의 계층이며 노드는 항상 resolved inline payload 만 본다. — RD-4.

## 3. Requirements (요구사항, EARS)

> 표기: `shall`(항상) = Ubiquitous, `WHEN…THEN`= Event-driven, `IF…THEN`= State-driven, `가능하면`= Optional, `shall not`= Unwanted.

### Module 1 — Trigger 노드 live 재등록 (`internal/node/trigger.go`) · REQ-01-01 ~ REQ-01-07

- **REQ-01-01** (Event-driven): **WHEN** `Configure()` 가 **Running** 상태의 `TriggerNode` 에 새 스케줄 셋과 함께 호출되면 **THEN** 노드는 기존 등록 타이머를 모두 취소하고 새 스케줄로 타이머를 재등록해야 한다.
- **REQ-01-02** (Ubiquitous): 시스템은 재등록 시 항상 `cancelAllTimers()` → `registerSchedules()` 순서를 `timerMu` 경계 규율 하에 수행하고, **re-arm generation 토큰**으로 orphan 타이머와 double-fire 를 방지해야 한다. 각 등록 타이머는 등록 시점의 generation 을 캡처하며, 발화 시 자신의 generation 이 현재 generation 과 불일치하면 그 발화는 무시(drop)되어야 한다 — cancel→re-register 창에서 이미 디스패치된 stale in-flight 발화를 폐기한다. (RD-6 확정)
- **REQ-01-03** (State-driven): **IF** 노드가 Running 이 아니면(Created/Paused/Stopped) **THEN** `Configure` 는 스케줄/페이로드 필드만 갱신하고 타이머 재등록은 수행하지 않아야 한다(다음 `Init`/`Resume` 경로가 반영).
- **REQ-01-04** (Unwanted): 시스템은 `Init()` 경로의 기존 최초 1회 등록 동작을 변경해서는 안 된다(`Configure` 가 호출되지 않는 배포/부팅 경로 불변).
- **REQ-01-05** (Event-driven): **WHEN** `Configure` 가 전달한 config 의 스케줄이 비어 있으면 **THEN** 시스템은 모든 타이머를 취소하고 노드를 유효한 IDLE 상태(발화 없음)로 두어야 하며, 이를 오류로 취급해서는 안 된다. (RD-7 확정)
- **REQ-01-06** (State-driven): **IF** 재등록 중 `register*` 가 실패하면 **THEN** 부분 등록 타이머를 `cancelAllTimers()` 로 롤백하고 오류를 반환하되, 노드 lifecycle 상태는 유지해야 한다(Error 전이 없음).
- **REQ-01-07** (Ubiquitous): 재등록은 per-schedule `Payload`/`PayloadTmpl` 및 노드 레벨 payload 폴백 순서(`buildMessage`, `trigger.go:728-756`)를 보존해야 한다.

### Module 2 — 대시보드 패널 + 노드 타겟팅 (`web/`) · REQ-02-01 ~ REQ-02-06

- **REQ-02-01** (Ubiquitous): 시스템은 신규 `PanelType` `'trigger-config'` 를 제공해야 한다(`uiStore.ts` union 확장).
- **REQ-02-02** (Event-driven): **WHEN** 사용자가 `AddPanelDialog` 에서 trigger-config 패널을 추가하면 **THEN** `useNodeTypeInstances('trigger')` 로 running trigger 노드 목록(`flowId/flowName/nodeId/nodeName`)을 타겟 피커로 제시해야 한다.
- **REQ-02-03** (Event-driven): **WHEN** 사용자가 특정 trigger 노드를 선택하면 **THEN** 패널 config 에 `{flowId, nodeId}` 를 저장해야 한다.
- **REQ-02-04** (Ubiquitous): 패널은 `config.flowId/nodeId` 로 대상 노드의 현재 config 를 `useFlowNodes` 로 읽어 스케줄/페이로드를 렌더링해야 한다(스케줄은 노드 config 가 SSOT, 패널 config 에 복제하지 않음).
- **REQ-02-05** (State-driven): **IF** 대상 노드가 running 인스턴스 목록에 없으면 **THEN** 패널은 not-running 상태를 표시하고 persist-only 모드로 동작해야 한다(REQ-05-03 연계).
- **REQ-02-06** (Ubiquitous): 시스템은 `renderDashboardPanel.tsx` 디스패치에 trigger-config 케이스를 추가하고 `panelDefaultSize`/`createDefaultPanel` 을 확장해야 한다.

### Module 3 — 스케줄 편집 UI · REQ-03-01 ~ REQ-03-04

- **REQ-03-01** (Ubiquitous): 패널은 6개 스케줄 타입(interval/cron/once/times/weekly/monthly) 전부에 대해 스케줄 CRUD(추가/수정/삭제)를 제공해야 한다.
- **REQ-03-02** (Optional): 가능하면 `TriggerScheduleEditor.tsx` 를 재사용하거나 미러링하여 편집 위젯을 구성한다.
- **REQ-03-03** (Event-driven): **WHEN** 사용자가 한 스케줄에 페이로드를 지정하려 하면 **THEN** 패널은 카탈로그(REQ-04-01)에서 이름으로 선택하는 per-schedule 페이로드 셀렉터를 제공해야 한다.
- **REQ-03-04** (Ubiquitous): 편집 결과는 트리거 노드 config 형태(`trigger_schedules` + per-schedule payload)로 직렬화되어야 한다(`parseScheduleConfig` 가 수용하는 스키마).

### Module 4 — 페이로드 카탈로그 + 인라인 주입 · REQ-04-01 ~ REQ-04-05

- **REQ-04-01** (Ubiquitous): 명명 페이로드는 패널 config `payloadCatalog: {name: payloadObject}` 에 정의되어야 한다(대시보드 패널 상태).
- **REQ-04-02** (Event-driven): **WHEN** 한 스케줄에 카탈로그 페이로드가 이름으로 배정되면 **THEN** 패널은 해당 payload 객체를 그 스케줄의 `payload` 필드에 **인라인 주입**하여 노드 config 를 구성해야 한다.
- **REQ-04-03** (Unwanted): 시스템은 트리거 노드 페이로드 모델을 변경해서는 안 된다 — 노드는 항상 resolved inline payload 만 관측한다(카탈로그·이름은 노드에 전달되지 않음).
- **REQ-04-04** (State-driven): **IF** 카탈로그 항목이 배정 이후 수정되면 **THEN** 이미 주입된 스케줄 payload 는 변경되지 않아야 한다 — 주입은 **스냅샷**(배정 시점에 payload 객체를 스케줄 config 로 인라인 복사)이며, 스케줄은 재선택 시에만 갱신된 값을 채택한다(소급 변경 없음). (RD-11 확정)
- **REQ-04-05** (Ubiquitous): 패널은 카탈로그 항목 CRUD(이름·payload 객체 정의) 에디터를 제공해야 한다.

### Module 5 — dual-write 지속성 · REQ-05-01 ~ REQ-05-05

- **REQ-05-01** (Event-driven): **WHEN** 사용자가 패널에서 스케줄/페이로드를 편집·저장하면 **THEN** 시스템은 `configureNode(flowId,nodeId,fullConfig)` 로 LIVE 반영하고 flow 정의 갱신(`updateFlow` / `PUT /flows`)으로 지속화해야 한다(dual-write).
- **REQ-05-02** (Ubiquitous): 지속 경로는 flow 를 읽어 대상 trigger 노드의 config 를 패치한 뒤 PUT 하는 **patch-then-PUT** 시퀀스여야 한다(다른 노드/와이어 보존).
- **REQ-05-03** (State-driven): **IF** live `configureNode` 가 404(노드 미실행)를 반환하면 **THEN** persist-only 로 진행하고 사용자에게 통지해야 한다(에러로 취급하지 않음). 패널은 대상 노드에 대해 running/stopped 배지를 표시하여 노드가 실행 중이 아닐 때 persist-only 동작을 사전에 드러내야 한다. (RD-10 확정)
- **REQ-05-04** (Ubiquitous): 패널은 델타가 아닌 **FULL trigger config**(schedules + payload)를 전송해야 한다 — `Configure` 가 스케줄/페이로드를 리셋(`n.schedules=nil`, `n.payload=nil`)하므로 부분 전송은 설정 유실을 유발한다(partial-config hazard, RD-2).
- **REQ-05-05** (Unwanted): 시스템은 동시 편집으로 인한 무통지 덮어쓰기를 해서는 안 된다 — 충돌 처리는 **last-write-wins + 대시보드 사용자 통지**로 확정한다. 본 SPEC 은 version/etag 낙관적 잠금을 도입하지 않는다(향후 확장으로 기록). (RD-8 확정)

### Module 6 — 비기능 요구 (NFR) · REQ-06-01 ~ REQ-06-04

- **REQ-06-01** (Unwanted, no-regression): 시스템은 기존 대시보드 패널 및 flow 에디터의 trigger 편집 동작을 변경해서는 안 된다(본 패널은 순수 가산형).
- **REQ-06-02** (Tested): 백엔드 재등록은 Go testify, 프런트 패널/타겟팅/카탈로그/dual-write 는 vitest 로 커버되어야 한다(≥85% 신규 코드 커버리지 목표).
- **REQ-06-03** (Secured): 스케줄 값·페이로드 입력은 파서 수용 범위로 검증되어야 한다(잘못된 cron/interval/JSON 거부).
- **REQ-06-04** (Trackable): 모든 변경은 Conventional Commit + SPEC-TRIGGER-PANEL-001 참조를 가져야 한다.

## 4. Specifications (사양)

### 4.1 패널 config shape

```
PanelConfig.config (type='trigger-config'):
{
  flowId: string,        // 대상 flow (REQ-02-03)
  nodeId: string,        // 대상 trigger 노드 (REQ-02-03)
  payloadCatalog: {      // 대시보드-로컬 명명 페이로드 (REQ-04-01)
    [name: string]: Record<string, unknown>
  },
  // (선택) 표시 옵션: 정렬/필터 등
}
```

- **스케줄은 패널 config 에 복제하지 않는다.** 스케줄/페이로드는 트리거 노드 config 가 SSOT 이며, 패널은 `useFlowNodes` 로 읽고 dual-write 로 쓴다(REQ-02-04). 패널은 노드의 **live 에디터**이다.

### 4.2 `TriggerNode.Configure` live 재등록 메커니즘 (RD-2)

`Configure()`(`trigger.go:951-995`) 말미에 재무장 단계를 추가한다(기존 파싱 로직 보존):

```
Configure(config):
  BaseNode.Configure(config)
  n.schedules = nil; parseScheduleConfig()      // 기존
  n.payload = nil; n.payloadTemplate = nil; ... // 기존
  resolve _timer_agent / _agent_resolver / source_ch_size // 기존
  --- 신규 재무장 (RD-6 generation 토큰) ---
  IF n.state() == Running AND n.timer != nil:
     n.cancelAllTimers()          // trigger.go:924-937 (timerMu 보호, 기존 헬퍼 재사용)
     n.rearmGen++                 // 재무장 세대 증가 (timerMu 경계 내)
     err = n.registerSchedules()  // trigger.go:359-387 (등록 타이머가 현재 rearmGen 캡처)
     IF err != nil:
        n.cancelAllTimers()       // 롤백 (REQ-01-06)
        return err
  // 발화 핸들러: IF firedGen != n.rearmGen THEN drop (stale in-flight 폐기)
```

- 재사용 자산: `cancelAllTimers()`·`registerSchedules()` 는 이미 존재. 신규 코드는 상태 게이트 + generation 카운터 + 순서 호출 + 롤백뿐(최소 침습).
- **동시성 규율(RD-6 확정)**: `cancelAllTimers()` 는 `timerMu` 를 잡고 `Cancel`+`timerEntries=nil` 을 수행. 재무장 시 `rearmGen` 세대 카운터를 증가시키고, 각 등록 타이머는 등록 시점의 세대를 캡처한다. 발화 핸들러는 자신이 캡처한 세대가 현재 `rearmGen` 과 불일치하면 방출을 no-op 으로 폐기한다. 이로써 cancel→re-register 창에서 (a) 이미 취소된 타이머의 in-flight 핸들러(stale 세대)는 drop 되고, (b) 신규 타이머는 각 주기마다 정확히 1회만 발화한다 — orphan 없음, double-fire 없음. `cancelAllTimers()` + generation 토큰 조합.

### 4.3 dual-write 시퀀스 (RD-3)

저장 버튼 → (병렬 또는 순차):
1. **LIVE**: `configureNode(flowId, nodeId, fullTriggerConfig)` — 404 면 persist-only 통지(REQ-05-03), 그 외 성공.
2. **PERSIST (patch-then-PUT)**: `flowService.getFlow(flowId)` → 정의에서 `nodeId` 노드의 `config` 를 fullTriggerConfig 로 패치(다른 노드/와이어 보존) → `flowService.updateFlow(flowId, patchedDef)`.
- `fullTriggerConfig` 는 **항상 전체** schedules+payload(REQ-05-04). 카탈로그 배정은 §4.4 로 inline 해소된 뒤 전송.
- 선례 미러: `CustomNode.tsx:107-131` 의 editor-state + live configureNode 듀얼 라이트.

### 4.4 카탈로그 → inline 주입 (RD-4)

- 스케줄 편집 상태에는 페이로드를 **이름 참조**로 보관(UI 편의). 노드 config 직렬화 직전, 각 스케줄의 이름 참조를 `payloadCatalog[name]` 객체로 치환하여 스케줄 `payload` 필드에 inline 주입(REQ-04-02). 노드는 이름/카탈로그를 절대 보지 않음(REQ-04-03).

### 4.5 타겟팅 (RD-1)

- `useNodeTypeInstances('trigger')` 결과를 AddPanelDialog 피커에 바인딩. 선택 시 `{flowId,nodeId}` 를 패널 config 로 저장. 렌더 시 running 목록에 대상이 없으면 not-running(REQ-02-05).

## 5. Resolved Decisions (확정된 설계 결정)

- **RD-1 — Trigger 맵핑**: 패널은 `config.flowId`+`config.nodeId` 로 특정 trigger 노드를 타겟한다. AddPanelDialog 에 `useNodeTypeInstances('trigger')` 기반 피커 추가. 신규 `PanelType 'trigger-config'`. (REQ-02-01~03, REQ-02-06)
- **RD-2 — 스케줄 즉시 반영(live 재등록)**: `TriggerNode.Configure` 를 확장해 **cancel+re-register**. 패널은 FULL config 전송(partial-config hazard 회피). Init 경로 불변. cancel+re-register 는 동시 발화에 안전해야 함(no-double-fire/no-orphan). (REQ-01-01~07, §4.2)
- **RD-3 — dual-write 지속성**: 편집은 `configureNode` 로 LIVE + `updateFlow`(PUT /flows)로 persist(재배포 생존). patch-then-PUT. 404(미실행) 시 persist-only + 통지. `CustomNode` 듀얼 라이트 미러. (REQ-05-01~04, §4.3)
- **RD-4 — 페이로드 카탈로그 + 인라인 주입**: 명명 페이로드는 **패널 config** `payloadCatalog` 에 정의. 스케줄에 이름 배정 시 payload 객체를 스케줄 `payload` 로 inline 주입. 노드 페이로드 모델 불변. 카탈로그는 패널-로컬 편의 계층. (REQ-04-01~05, §4.4)
- **RD-5 — 스케줄 편집 UI**: 6개 타입 전부 CRUD + 카탈로그 에디터 + per-schedule 페이로드 셀렉터. `TriggerScheduleEditor.tsx` 재사용/미러. (REQ-03-01~04)
- **RD-6 — 재등록 no-double-fire(generation 토큰)**: `TriggerNode.Configure` 는 노드의 타이머 mutex(`timerMu`) 하에 기존 타이머를 취소하고 재등록하되, **re-arm generation 카운터**를 사용한다. 각 등록 타이머는 등록 시점의 세대를 캡처하며, 세대가 현재 세대와 불일치하는 발화는 무시된다(cancel→re-register 창의 stale in-flight 발화 drop). `cancelAllTimers()`(trigger.go:924-937) + generation 토큰 조합 → double-fire 없음, orphan 타이머 없음. (REQ-01-02, §4.2 확정)
- **RD-7 — 빈 스케줄 live Configure(IDLE)**: 스케줄이 비어 온 live Configure 는 모든 타이머를 취소하고 노드를 유효한 IDLE 상태(발화 없음)로 둔다. 오류가 아니다. (REQ-01-05 확정)
- **RD-8 — 동시 flow 편집 충돌**: dual-write persist 는 **last-write-wins + 대시보드 사용자 통지**를 사용한다. 본 SPEC 은 version/etag 낙관적 잠금을 도입하지 않는다(향후 확장으로 기록). (REQ-05-05 확정)
- **RD-9 — 카탈로그 범위(패널-로컬)**: 페이로드 카탈로그는 **패널-로컬**(대시보드 패널 config 의 `config.payloadCatalog`)로 확정한다. 공유/전역 명명 페이로드 저장소는 명시적으로 **out of scope**(향후 SPEC). (REQ-04-01, A-4 확정)
- **RD-10 — 대시보드 flow 쓰기**: 대시보드는 기존 flow API 를 재사용한다 — read 는 `useFlows`/`useFlowNodes`, write 는 `updateFlow`(PUT /flows). 신규 권한 계층 없음(동일 API 표면). 패널은 대상 노드에 대해 running/stopped 배지를 표시하고, 노드가 실행 중이 아닐 때(live configure 404) persist-only + 통지로 동작한다. 구현은 `updateFlow` 가 대시보드 컨텍스트에서 호출 가능함을 검증해야 한다. (REQ-02-05, REQ-05-03 확정)
- **RD-11 — 카탈로그 편집 → 주입 스냅샷**: 카탈로그 payload 를 스케줄에 배정하면 **스냅샷**(배정 시점에 payload 객체를 스케줄 config 로 인라인 복사)이 주입된다. 카탈로그를 이후 편집해도 이미 주입된 스케줄은 소급 변경되지 않으며, 스케줄은 **재선택** 시에만 갱신된 값을 채택한다 — 예측 가능, 숨은 소급 변형 없음. (REQ-04-04 확정)

## 6. Open Questions (열린 질문)

없음. OQ-1~6 은 엔지니어링 기본값으로 확정되어 RD-6~11(§5)로 승격되었다. 잔여 열린 질문 없음.

## 7. Traceability

| REQ 범위        | 모듈                          | 구현 대상 (예상)                                                                 | 검증(acceptance.md) |
| ------------- | --------------------------- | ----------------------------------------------------------------------- | ----------------- |
| REQ-01-01~07  | M1 Trigger live 재등록          | `internal/node/trigger.go` `Configure`                                   | §A (Go testify)    |
| REQ-02-01~06  | M2 패널 + 타겟팅                  | `uiStore.ts`, `AddPanelDialog.tsx`, `renderDashboardPanel.tsx`, 신규 패널 컴포넌트 | §B (vitest)        |
| REQ-03-01~04  | M3 스케줄 편집 UI                 | 신규 패널 컴포넌트 + `TriggerScheduleEditor` 재사용                                    | §C (vitest)        |
| REQ-04-01~05  | M4 카탈로그 + inline 주입          | 신규 패널 컴포넌트 + 직렬화 유틸                                                         | §D (vitest)        |
| REQ-05-01~05  | M5 dual-write                | `nodeService.configureNode`, `flowService.updateFlow`, 패널 저장 핸들러          | §E (vitest)        |
| REQ-06-01~04  | M6 NFR                       | 전 범위                                                                     | §F                |
