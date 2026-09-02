# Research — SPEC-TRIGGER-PANEL-001

> 코드베이스 조사 아티팩트. 모든 file:line 은 조사 시점 확인 기준선(2026-07-31, develop 브랜치). 구현 착수 시 재확인 권장.
>
> **버전 노트**: v0.3.0 (2026-07-31) — M1~M6 구현 완료 + sync-phase close. 조사 기준선은 구현으로 검증됨: §1 의 `cancelAllTimers`/`registerSchedules` 재사용 가정이 M1 재무장에서 성립하고, §2~§5 의 기존 경로(configureNode/updateFlow/패널 시스템/TriggerScheduleEditor) 재사용이 M2~M5 에서 확인됨. 구현 트리거 config 키는 `schedules`(SPEC 산문 `trigger_schedules` 정정, spec.md §8 IN-3).

## 1. Trigger 노드 (`internal/node/trigger.go`) — 확인됨

| 항목                         | 위치                    | 확인 내용                                                                                              |
| -------------------------- | --------------------- | ------------------------------------------------------------------------------------------------- |
| 스케줄 타입 상수                   | `trigger.go:46-64`    | interval/cron/once/times/weekly/monthly (`TriggerScheduleType`)                                    |
| per-schedule 구조체            | `trigger.go:67-82`    | `TriggerSchedule{Type,Value,Days,Times,Day, Payload, PayloadTmpl}` — per-schedule payload 존재       |
| 노드 상태 필드                    | `trigger.go:109-118`  | `schedules`, `timerEntries`, `timerMu sync.Mutex`, `payload`, `payloadTemplate`                    |
| `Init()`                   | `trigger.go:279-334`  | **유일 등록 경로**. `registerSchedules()` 호출, 실패 시 `cancelAllTimers()` 롤백, Running 전이                       |
| `registerSchedules`        | `trigger.go:358-387`  | 각 스케줄 → `registerSingleSchedule` → `register*` → `addEntry`(timerMu 개별 락)                           |
| `cancelAllTimers()`        | `trigger.go:924-937`  | **재사용 대상**. `timerMu` 잡고 각 `entry.timerID` `Cancel` 후 `timerEntries=nil`. Stop 경로에서 호출              |
| `Configure()`              | `trigger.go:951-995`  | 스케줄/페이로드 재파싱만. **cancel/register 없음** → live 재무장 미동작 근본원인                                            |
| payload 폴백                  | `buildMessage` (조사서)  | per-schedule → 노드 레벨 → 기본 순서                                                                        |

**핵심 발견**: 재무장에 필요한 두 헬퍼(`cancelAllTimers`, `registerSchedules`)가 이미 존재하고 `timerMu` 규율이 확립되어 있어, `Configure` 변경은 상태 게이트 + 순서 호출 + 롤백의 최소 침습으로 가능.

## 2. 런타임 노드 config 갱신 경로 — 확인됨(기준선, 신규 아님)

- `POST /flows/{id}/nodes/{nodeID}/configure` (handler `flow.go`) → `FlowService.ReconfigureFlowNode` → `engine.ReconfigureNode`(`engine.go:703-748`) → live `node.Configure`. persist 없음. 노드 미실행 시 404.
- 프런트: `nodeService.ts:31-37 configureNode(flowId,nodeId,config)` → `post('/flows/{id}/nodes/{nodeId}/configure', {config})`. (조사에서 파일 직접 확인)
- 듀얼 라이트 선례: `CustomNode.tsx:107-131`.

## 3. flow 정의 지속 경로 — 확인됨(별개 축)

- `PUT /flows/{id}` `UpdateFlow`(`flow_adapter.go:312-361`) — 정의 전체 재빌드·저장, 재배포 시 적용.
- 프런트: `flowService.updateFlow`(`flowService.ts:41`), `getFlow`/`useFlowNodes`(`useFlow.ts:35`, GET /flows/{id}/nodes → node_id/name/type/config).

## 4. 대시보드 패널 시스템 — 확인됨

| 항목                  | 위치                             | 내용                                                     |
| ------------------- | ------------------------------ | ------------------------------------------------------ |
| `PanelType` union   | `uiStore.ts:177-199`           | flow/trigger 타입 **없음** (facility-* 까지). 확장 지점            |
| `PanelConfig`       | `uiStore.ts:202-208`           | `{id,type,title,config}` — config 자유형(Record)          |
| 렌더 디스패치             | `renderDashboardPanel.tsx:72-242` | type 별 switch — 신규 케이스 추가                               |
| 추가 다이얼로그            | `AddPanelDialog.tsx`           | agent/chart 타겟만, **flow 노드 타겟 패널 없음** — 피커 추가 지점        |
| 노드 인스턴스 열거          | `useNodeTypeInstances.ts:23-70` | running 플로우별 `{flowId,flowName,nodeId,nodeName,...}` 반환 |

- 패널 컴포넌트 계약(예 `AcControlPanel.tsx`): `{panelId,title,config,onConfigChange,onTitleChange}`, config.deviceId 로 동작 → 본 SPEC 은 config.flowId/nodeId 로 동일 패턴.

## 5. 스케줄 편집 위젯 — 확인됨

- flow 에디터 속성 위젯: `TriggerScheduleEditor.tsx` + `nodeSchemas.ts:2586-2625`(`trigger_schedules` + `payload` object). 대시보드 패널에서 재사용/미러 대상.

## 6. 갭 분석

| 갭                              | 상태          | 대응                                    |
| ------------------------------ | ----------- | ------------------------------------- |
| live Configure 타이머 재무장         | 없음(미동작)     | M1 — Configure 확장                     |
| flow 노드를 타겟하는 대시보드 패널          | 없음         | M2 — 신규 PanelType + 피커                |
| 명명 페이로드 저장소                     | 없음(net-new) | M4 — 패널 config 레벨 카탈로그(백엔드 무신설)        |
| dual-write 저장(패널→노드+정의)         | 부분(경로만 존재)  | M5 — 저장 핸들러 조립(configureNode+updateFlow) |

## 7. 확정 결정 (OQ-1~6 → RD-6~11)

OQ-1~6 은 엔지니어링 기본값으로 확정됨(spec.md §5 RD-6~11). 아래는 확정 결과 및 구현 착수 시 검증 포인트.

- RD-6 (구 OQ-1): 재무장 no-double-fire 는 **re-arm generation 토큰**으로 확정 — `n.timer.Cancel` 동기 보장에만 의존하지 않고 세대 불일치 발화를 drop. 착수 시 timer agent 발화 경로에 세대 캡처/비교 훅 위치 확인.
- RD-7 (구 OQ-2): 빈 스케줄 live Configure = 전 타이머 취소 후 유효 IDLE(오류 아님).
- RD-8 (구 OQ-3): `updateFlow`/`PUT /flows` 충돌은 last-write-wins + 통지로 확정 — 낙관적 잠금 미도입(향후 SPEC). 백엔드 잠금 지원 조사 불요.
- RD-9 (구 OQ-4): 카탈로그는 패널-로컬(`config.payloadCatalog`) 확정 — 공유 명명 페이로드 저장소는 out of scope.
- RD-10 (구 OQ-5): 대시보드 flow read+write 는 기존 API(useFlows/useFlowNodes, updateFlow) 재사용 확정 + running/stopped 배지 + 404 persist-only. 착수 시 `updateFlow` 가 대시보드 컨텍스트에서 호출 가능함을 검증.
- RD-11 (구 OQ-6): 카탈로그 배정 = **스냅샷 주입**(배정 시점 복사, 비소급, 재선택 시 갱신) 확정.
