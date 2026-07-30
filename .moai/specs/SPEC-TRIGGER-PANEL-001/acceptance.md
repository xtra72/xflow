# Acceptance Criteria — SPEC-TRIGGER-PANEL-001

> Given-When-Then. 기계 검증 가능: 백엔드 Go testify(`internal/node/trigger_test.go`), 프런트 vitest(`web/src/**/*.test.ts(x)`). REQ 추적은 각 시나리오 말미.
>
> **버전 노트**: v0.2.0 (2026-07-31) — OQ-1~6 확정(RD-6~11) 반영. A-2(generation 토큰 stale drop), A-5(빈 스케줄 IDLE), D-3(스냅샷 주입 비소급), E-3(404 persist-only + 배지), E-4(last-write-wins 통지) 를 확정안으로 구체화하고, B-5(running/stopped 배지) 신규 추가.

## §A. Trigger 노드 live 재등록 (Go testify) — M1

### A-1 — running 노드 live 재등록 (schedule 교체)
- **Given** Init 로 스케줄 A(interval 1s)가 등록된 Running `TriggerNode`(fake timer 주입)
- **When** `Configure({trigger_schedules:[B(interval 5s)]})` 를 호출하면
- **Then** A 의 타이머는 `Cancel` 되고 B 가 새로 등록되며, `timerEntries` 는 정확히 B 하나만 포함한다. (REQ-01-01)

### A-2 — no-double-fire / no-orphan (generation 토큰)
- **Given** 스케줄 A 등록 Running 노드(fake timer, `rearmGen`=G0)
- **When** live 재등록(A→B)을 수행하고, cancel→re-register 창에서 A 의 stale in-flight 발화(캡처 세대 G0)가 도착하면
- **Then** 재무장으로 세대가 G1 로 증가하여 stale G0 발화는 세대 불일치로 drop 되고(중복 발화 없음), 취소된 A 타이머는 재무장 이후 발화하지 않으며(orphan 없음), B(세대 G1)는 각 주기마다 정확히 1회 발화한다. (REQ-01-02, RD-6 확정)

### A-3 — 비-Running 상태는 타이머 재등록 안 함
- **Given** Created/Paused/Stopped 상태 노드
- **When** `Configure(newSchedules)` 를 호출하면
- **Then** `n.schedules`/`n.payload` 는 갱신되지만 `timer.Cancel`/register 는 호출되지 않는다(추적 spy 로 확인). (REQ-01-03)

### A-4 — Init 경로 불변 (회귀)
- **Given** `Configure` 미호출 배포 경로
- **When** `Init()` 를 수행하면
- **Then** 최초 스케줄 등록 동작·엔트리 수가 기존과 동일하다. (REQ-01-04)

### A-5 — 빈 스케줄 live Configure (IDLE 확정)
- **Given** 스케줄 A 등록 Running 노드
- **When** `Configure({trigger_schedules:[]})` 를 호출하면
- **Then** 모든 타이머가 취소되고 `timerEntries` 는 비며 오류를 반환하지 않는다 — 노드는 발화 없는 유효 IDLE 상태로 Running 을 유지한다. (REQ-01-05, RD-7 확정)

### A-6 — 재등록 실패 롤백
- **Given** Running 노드
- **When** `Configure` 에 잘못된 스케줄(예: interval value non-string)을 전달해 `registerSchedules` 가 실패하면
- **Then** 부분 등록 타이머가 `cancelAllTimers` 로 롤백되고 오류가 반환되며 lifecycle 상태는 Error 로 전이하지 않는다. (REQ-01-06)

### A-7 — payload 폴백 보존
- **Given** per-schedule payload + 노드 레벨 payload 가 섞인 스케줄 셋
- **When** live 재등록 후 각 스케줄이 발화하면
- **Then** `buildMessage` 폴백 순서(per-schedule → 노드 레벨 → 기본)가 재등록 전과 동일하다. (REQ-01-07)

## §B. 패널 + 노드 타겟팅 (vitest) — M2

### B-1 — trigger 노드 타겟 피커
- **Given** running flow 2개에 trigger 노드가 존재(`useNodeTypeInstances('trigger')` mock)
- **When** AddPanelDialog 에서 trigger-config 패널을 추가하면
- **Then** flowName/nodeName 쌍 목록이 피커에 렌더된다. (REQ-02-02)

### B-2 — 타겟 선택 시 config 저장
- **When** 사용자가 특정 노드를 선택하면
- **Then** 생성된 패널 config 에 `{flowId, nodeId}` 가 저장된다. (REQ-02-03)

### B-3 — 대상 노드 config 로드 렌더
- **Given** `config.flowId/nodeId` 를 가진 패널(`useFlowNodes` mock: 스케줄 2개)
- **When** 패널이 렌더되면
- **Then** 노드의 현재 스케줄/페이로드가 표시된다(패널 config 에 스케줄 복제 없음). (REQ-02-04)

### B-4 — not-running 표시
- **Given** running 인스턴스 목록에 대상 nodeId 없음
- **When** 패널이 렌더되면
- **Then** not-running 상태가 표시되고 persist-only 모드 표식이 나타난다. (REQ-02-05)

### B-5 — running/stopped 배지 (RD-10)
- **Given** `useNodeTypeInstances('trigger')` mock — 대상 nodeId 가 (a) running 목록에 포함 / (b) 미포함인 두 케이스
- **When** 패널이 렌더되면
- **Then** (a) 케이스는 running 배지를, (b) 케이스는 stopped 배지를 표시하여 persist-only 동작을 사전에 드러낸다. (REQ-05-03, RD-10)

## §C. 스케줄 편집 UI (vitest) — M3

### C-1 — 6개 타입 CRUD
- **When** 사용자가 interval/cron/once/times/weekly/monthly 각각을 추가·수정·삭제하면
- **Then** 편집 상태가 각 타입 스키마대로 갱신되고, 직렬화 결과가 `parseScheduleConfig` 수용 형태(`trigger_schedules`)가 된다. (REQ-03-01, REQ-03-04)

### C-2 — per-schedule 페이로드 셀렉터
- **Given** 카탈로그에 name→payload 2개
- **When** 한 스케줄에 페이로드 지정 UI 를 열면
- **Then** 카탈로그 이름 목록이 셀렉터로 제시된다. (REQ-03-03)

## §D. 페이로드 카탈로그 + inline 주입 (vitest) — M4

### D-1 — 카탈로그 CRUD
- **When** 사용자가 카탈로그 항목(name, payload object)을 추가/수정/삭제하면
- **Then** 패널 config `payloadCatalog` 가 갱신된다. (REQ-04-01, REQ-04-05)

### D-2 — 스케줄 배정 시 inline 주입
- **Given** `payloadCatalog = {warn:{level:3}}`, 스케줄 S 가 payload 이름 `warn` 참조
- **When** 노드 config 로 직렬화하면
- **Then** S 의 `payload` 필드에 `{level:3}` 객체가 inline 포함되고, 직렬화 결과 어디에도 이름 `warn`/카탈로그 구조가 노드로 전달되지 않는다. (REQ-04-02, REQ-04-03)

### D-3 — stale 주입 스냅샷 (RD-11 확정, 비소급)
- **Given** `payloadCatalog={warn:{level:3}}`, 스케줄 S 에 `warn` 배정 시점에 `{level:3}` 스냅샷이 S 의 `payload` 로 인라인 복사 완료
- **When** 카탈로그의 `warn` 을 `{level:5}` 로 수정하면(재선택 없음)
- **Then** S 의 기존 주입 payload 는 `{level:3}` 그대로 변하지 않는다(소급 변경 없음). 사용자가 S 에서 `warn` 을 **재선택**할 때에만 `{level:5}` 로 갱신된다. (REQ-04-04, RD-11 확정)

## §E. dual-write 지속성 (vitest) — M5

### E-1 — live + persist 동시 수행
- **Given** running 대상 노드, `configureNode`·`getFlow`·`updateFlow` mock
- **When** 사용자가 편집 후 저장하면
- **Then** `configureNode(flowId,nodeId,fullConfig)` 가 호출되고, `getFlow`→노드 config 패치→`updateFlow(flowId, patchedDef)` 가 호출된다(다른 노드/와이어 보존). (REQ-05-01, REQ-05-02)

### E-2 — FULL config 전송
- **When** 저장 시
- **Then** `configureNode`·`updateFlow` 에 전달된 trigger config 는 스케줄+페이로드 전체를 포함한다(부분 델타 아님). (REQ-05-04)

### E-3 — 404 persist-only (+ stopped 배지)
- **Given** `configureNode` 가 404(노드 미실행) 반환
- **When** 저장하면
- **Then** 오류로 처리하지 않고 persist(`updateFlow`)만 수행하며, 사용자에게 persist-only 통지가 표시되고 패널은 stopped 배지를 노출한다. (REQ-05-03, RD-10 확정)

### E-4 — 동시 편집 last-write-wins 통지 (RD-8 확정)
- **Given** persist 중 flow 가 외부에서 변경(last-write-wins — 낙관적 잠금 없음)
- **When** `updateFlow` 가 완료되면
- **Then** 사용자에게 덮어쓰기 발생 가능성 통지가 대시보드에 표시된다(무통지 덮어쓰기 없음). version/etag 잠금 경로는 호출되지 않는다. (REQ-05-05, RD-8 확정)

## §F. no-regression / 품질 게이트 — M6

### F-1 — 기존 패널·에디터 불변
- **Then** 기존 대시보드 패널 렌더 및 flow 에디터 `TriggerScheduleEditor` 편집 경로의 기존 테스트가 모두 green 이다. (REQ-06-01)

### F-2 — 커버리지 / 게이트
- **Then** 신규 백엔드/프런트 코드 커버리지 ≥85%, gofmt/prettier 클린, Go `-race` 클린, vitest·`tsc` 클린. (REQ-06-02)

### F-3 — 입력 검증
- **Then** 잘못된 cron/interval/JSON payload 는 저장 전 거부되고 사용자에게 오류가 표시된다. (REQ-06-03)

## Definition of Done

- [ ] §A~§E 모든 시나리오 자동 테스트 통과(Go testify + vitest)
- [ ] §F no-regression + 품질 게이트 통과
- [ ] RD-1~11 구현 확인(OQ-1~6 확정안 반영 완료 — 미해결 없음), 노드 페이로드 모델 불변 확인
- [ ] RD-6 generation 토큰 재무장, RD-7 빈-스케줄 IDLE, RD-11 스냅샷 비소급 검증
- [ ] Conventional Commit + SPEC 참조
