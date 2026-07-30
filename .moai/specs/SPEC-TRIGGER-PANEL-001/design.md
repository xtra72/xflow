# Design — SPEC-TRIGGER-PANEL-001

> Tier L 기술 설계. 아키텍처, 컴포넌트 경계, 데이터 모델, 재등록 시퀀스, dual-write 시퀀스.
>
> **버전 노트**: v0.3.0 (2026-07-31) — M1~M6 구현 완료 + sync-phase close. 본 설계는 구현으로 실현됨(§2.2 재무장 게이트는 started + StateRunning 으로, §3.3 스냅샷은 JSON deep-clone 으로, §3.4 dual-write 는 patch-then-PUT 로 구현). 구현 트리거 config 키는 `schedules`. as-implemented 정련 6건은 spec.md §8 IN-1~IN-6 참조.

## 1. 아키텍처 개요

```
[Dashboard]
  AddPanelDialog ──useNodeTypeInstances('trigger')──▶ {flowId,flowName,nodeId,nodeName}
        │ select
        ▼
  PanelConfig(type='trigger-config', config={flowId,nodeId,payloadCatalog})
        │
  TriggerConfigPanel(신규 컴포넌트)
        │  read: useFlowNodes(flowId) → 대상 노드 config(schedules/payload) [SSOT]
        │  edit: 스케줄 CRUD + 카탈로그 이름참조
        │  serialize: 이름참조 → payloadCatalog[name] inline 주입 → fullTriggerConfig
        │  save (dual-write):
        │     ├─ LIVE   : nodeService.configureNode(flowId,nodeId,fullTriggerConfig)  ──▶ POST /flows/{id}/nodes/{nodeId}/configure ──▶ engine.ReconfigureNode ──▶ TriggerNode.Configure() [live re-arm]
        │     └─ PERSIST: flowService.getFlow → patch node.config → flowService.updateFlow ──▶ PUT /flows/{id} [redeploy 생존]
        ▼
  renderDashboardPanel dispatch('trigger-config')
```

- **두 쓰기 경로는 독립**: LIVE 는 실행 중 인스턴스 즉시 반영(persist 없음), PERSIST 는 정의 저장(재배포 시 반영). 둘 다 성공해야 "완전 저장". 404 LIVE 는 persist-only 로 강등.

## 2. 백엔드 설계 — `TriggerNode.Configure` live 재등록

### 2.1 현재 상태(조사)
- `Configure()`(`trigger.go:951-995`): `BaseNode.Configure` → `n.schedules=nil; parseScheduleConfig()` → `n.payload=nil; n.payloadTemplate=nil` 재설정 → `_timer_agent`/`_agent_resolver`/`source_ch_size` 반영. **타이머 재무장 없음.**
- 재사용 자산: `cancelAllTimers()`(`trigger.go:924-937`, `timerMu` 보호), `registerSchedules()`(`trigger.go:359-387`), Init 롤백 패턴(`trigger.go:322-333`).

### 2.2 변경 설계 (RD-6 generation 토큰, RD-7 빈 스케줄 IDLE)
```go
// Configure() 말미(기존 로직 뒤)에 추가:
if n.isRunning() && n.timer != nil {         // 상태 게이트 (REQ-01-03)
    n.cancelAllTimers()                      // 기존 헬퍼 (timerMu)
    n.rearmGen++                             // 재무장 세대 증가 (RD-6, timerMu 경계 내)
    if err := n.registerSchedules(); err != nil { // 등록 타이머가 현재 rearmGen 캡처
        n.cancelAllTimers()                  // 롤백 (REQ-01-06) — Error 전이 없음
        return err
    }
    // 빈 스케줄이면 registerSchedules 는 no-op → 타이머 없는 유효 IDLE (RD-7, 오류 아님)
}
// 발화 핸들러 게이트: if firedGen != n.rearmGen { return }  // stale in-flight drop (RD-6)
```
- `isRunning()`: `BaseNode` lifecycle state == Running 조회(기존 상태 API 활용).
- Init 경로는 이 블록을 타지 않음(Init 은 Configure 를 거치지 않거나, Configure 시점에 아직 Running 아님) → REQ-01-04 보장.
- 빈 스케줄(`trigger_schedules:[]`): cancel 후 registerSchedules 가 등록할 타이머가 없어 유효 IDLE Running 으로 남는다 — 오류 아님(RD-7).

### 2.3 동시성 설계 (RD-6 확정 — generation 토큰)
- **확정안**: `rearmGen uint64` 세대 토큰. 재무장 시 `timerMu` 경계 내에서 `rearmGen` 을 증가시키고, 각 등록 타이머는 등록 시점의 세대를 캡처한다. 발화 핸들러는 자신의 세대가 현재 `rearmGen` 과 불일치하면 방출을 no-op 으로 drop 한다.
- 효과: cancel→re-register 창에서 이미 디스패치된 stale in-flight 핸들러(이전 세대)는 세대 불일치로 폐기되고, 신규 타이머는 각 주기마다 정확히 1회만 발화 → double-fire 없음, orphan 없음.
- `cancelAllTimers()`(trigger.go:924-937, timerMu 보호)와 generation 토큰을 결합한다. 별도 rearm mutex 나 전역 pause 는 도입하지 않는다.
- 테스트(A-2)는 "취소된 stale 세대 발화 drop + 재등록 후 정확 1회"를 검증.

## 3. 프런트 설계

### 3.1 신규 컴포넌트 `TriggerConfigPanel`
- 계약: `{panelId, title, config, onConfigChange, onTitleChange}`(기존 패널 계약).
- 상태 소스: `useFlowNodes(config.flowId)` → 대상 노드 `config`(schedules/payload). 편집은 로컬 draft, 저장 시 dual-write.
- 하위 위젯: 스케줄 리스트(6 타입 CRUD, `TriggerScheduleEditor` 재사용/미러), 카탈로그 에디터, per-schedule 페이로드 셀렉터.

### 3.2 config 스키마
```ts
interface TriggerPanelConfig {
  flowId: string;
  nodeId: string;
  payloadCatalog: Record<string, Record<string, unknown>>; // name → payload object
}
```
- 스케줄은 config 에 없음(노드 SSOT). 카탈로그만 패널-로컬.

### 3.3 직렬화 유틸 (catalog → inline, RD-11 스냅샷)
```
serializeToNodeConfig(draftSchedules, payloadCatalog):
  trigger_schedules = draftSchedules.map(s => ({
     ...s,
     payload: s.payloadRef ? payloadCatalog[s.payloadRef] : s.payload  // inline 주입
     // payloadRef(이름)는 결과에서 제외 — 노드는 이름을 보지 않음
  }))
  return { trigger_schedules, payload: nodeLevelPayload? }
```
- **RD-11 스냅샷**: 스케줄에 카탈로그 이름을 배정하는 시점에 `payloadCatalog[name]` 객체를 스케줄 draft 의 `payload` 로 **인라인 복사**한다(배정 = 스냅샷 확정). 이후 카탈로그를 편집해도 배정된 스케줄의 스냅샷은 소급 변경되지 않으며, 사용자가 그 스케줄에서 이름을 **재선택**할 때에만 최신 값이 다시 복사된다. `payloadRef` 는 UI 편의용 재선택 힌트일 뿐, 노드 config 직렬화 결과에는 포함되지 않는다.

### 3.4 dual-write 저장 핸들러 (RD-8 last-write-wins, RD-10 flow API 재사용/배지)
```
onSave(fullConfig):
  live  = configureNode(flowId, nodeId, fullConfig).catch(404 → markPersistOnly)  // RD-10
  flow  = await getFlow(flowId)
  patched = replaceNodeConfig(flow.definition, nodeId, fullConfig) // 다른 노드/와이어 보존
  await updateFlow(flowId, patched)      // RD-8 last-write-wins (낙관적 잠금 없음)
  notify(live.persistOnly ? "persist-only(미실행)" : "저장 완료")
  notifyIfConflictRisk()                 // RD-8 동시 편집 덮어쓰기 가능성 통지
```
- **RD-10**: read `useFlows`/`useFlowNodes`, write `updateFlow` — 기존 flow API 재사용, 신규 권한 계층 없음. 패널은 running/stopped 배지를 표시하고, 404(미실행) 시 persist-only 로 강등. 구현은 `updateFlow` 가 대시보드 컨텍스트에서 호출 가능함을 검증한다.
- **RD-8**: persist 는 last-write-wins + 대시보드 통지. version/etag 낙관적 잠금은 본 SPEC 범위 밖(향후 확장).

## 4. 시퀀스 다이어그램 (저장)

```
User        Panel          nodeService     flowService     Engine/API
 │ save ──▶ onSave
 │           ├─ configureNode ─────────────────────────▶ POST /configure ─▶ Configure()[re-arm]  (404 → persist-only)
 │           ├─ getFlow ───────────────▶ GET /flows/{id}
 │           │◀── flow def ─────────────┘
 │           ├─ patch node.config (local)
 │           └─ updateFlow ─────────────▶ PUT /flows/{id} ─▶ 정의 저장(재배포 시 반영)
 │◀─ notify
```

## 5. 결정 근거 (design rationale)

- **패널이 노드 config 를 복제하지 않는 이유**: 스케줄 SSOT 이중화는 drift 를 유발. `useFlowNodes` 재조회로 항상 최신 노드 config 를 편집 기준선으로 삼음.
- **카탈로그를 노드에 넣지 않는 이유**: 노드 페이로드 모델 불변(RD-4) — 백엔드 무변경, 노드는 항상 resolved inline payload.
- **cancelAllTimers 재사용**: 신규 취소 로직 작성 시 `timerMu` 규율 재구현 리스크 → 기존 검증된 헬퍼 사용.

## 6. 확정된 설계 결정 — RD 매핑 (OQ-1~6 해소)
- RD-6 → §2.2/§2.3 generation 토큰 재무장(stale 세대 발화 drop) — 확정
- RD-7 → §2.2 빈 스케줄 → 전 타이머 취소 후 유효 IDLE(오류 아님) — 확정
- RD-8 → §3.4 updateFlow 충돌 = last-write-wins + 통지(낙관적 잠금 미도입) — 확정
- RD-9 → §3.2 카탈로그 패널-로컬(`config.payloadCatalog`), 공유 저장소 out of scope — 확정
- RD-10 → §3.4 getFlow/updateFlow 기존 API 재사용 + running/stopped 배지 + 404 persist-only — 확정
- RD-11 → §3.3 스냅샷 주입(배정 시점 복사, 비소급, 재선택 시 갱신) — 확정

잔여 열린 질문 없음(OQ-1~6 전량 RD-6~11 로 확정).
