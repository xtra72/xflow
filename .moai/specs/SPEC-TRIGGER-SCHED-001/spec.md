---
id: SPEC-TRIGGER-SCHED-001
title: "설비 제어 예약 패널 (스케줄 규칙 테이블 + 모달 편집)"
version: "0.3.0"
status: completed
created: 2026-07-31
updated: 2026-07-31
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/node + web/src/pages/dashboard"
lifecycle: spec-anchored
tags: "trigger, schedule, facility-control, xsfm, dashboard, panel, table, modal, target-selector, action-command, validity-window, priority, enabled, fire-gating, dual-write, frontend, backend"
tier: L
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ---------- | ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-31 | 0.1.0 | 초기 SPEC 작성 — 설비 제어 예약 전용 대시보드 패널 도입. (1) Trigger 스케줄 모델을 확장해 규칙별 `name`/유효기간(`valid_from`/`valid_to`)/`priority`/`enabled` 메타를 부여하고 백엔드가 발화 시점에 존중(비활성·유효기간 밖은 발화 금지), (2) 규칙 payload = xsfm 제어 명령(TARGET=셀렉터, ACTION=제어 명령)으로 특화, (3) 읽기 전용 요약 **테이블**(SCHEDULE/TARGET/PLAN/ACTION/PRIO/STATE) + **모달** 생성/편집 + 행별 EDIT + STATE 토글 UX. RD-1~3 확정 반영. OQ-1~6 미해결(사용자 결정 대기). 모드 축(Auto/Sleep) 코드 점검 결과: xsfm 제어 모델은 2축(power/fan_speed)만 지원, `mode` 축 부재 → OQ-5 로 플래그. |
| 2026-07-31 | 0.2.0 | OQ-1~6 사용자 확정 → RD-4~9 승격, §6 Open Questions 제거(잔여 없음). RD-4: ACTION 은 xsfm 2축(power/fan_speed)만 — 모드 축(Auto/Sleep) 부재로 v1 범위 밖(라벨에서 Auto/Sleep 제거). RD-5: 신규 `facility-schedule` 패널이 범용 `trigger-config` 패널과 공존(대체 아님). RD-6: TARGET "전체" = `line` 셀렉터(호선 전체, 예 "2호선 전체"), 에이전트 전체 셀렉터 미도입. RD-7: priority 는 v1 표시+테이블 정렬 전용(런타임 충돌 해소 유보). RD-8: 유효기간은 서버 로컬 시간·날짜 단위 양끝 inclusive, 빈 `valid_to`=무기한/빈 `valid_from`=하한 무제한, 발화는 활성 AND [valid_from, valid_to] 내에서만. RD-9: 확장 필드는 dual-write(configureNode live + updateFlow persist)로 trigger 스케줄 config 에 지속(RD-1 일관). |
| 2026-07-31 | 0.3.0 | 구현 완료 + 3-phase close. 전 마일스톤(M1~M6) 구현·커밋 — 백엔드 M1~M2(커밋 `03e8827b`: `TriggerSchedule` 5필드 확장 + `makeHandler` 발화 게이팅), 프런트 M3~M6(커밋 `fb40c4b2`: 신규 `facility-schedule` 패널 — 6컬럼 테이블 + 모달 + TARGET 피커 + ACTION 편집기 + dual-write). `status: draft→completed`, `version: 0.2.0→0.3.0`. spec-anchored Level 2 규율에 따라 §8 구현 노트(as-implemented) IN-1~IN-7 신설. 검증: `go test ./...` exit 0(42 pkgs)·`-race` 클린·백엔드 M1 신규 함수 커버리지 100%, 프런트 vitest 2370(+50)·`tsc`/eslint 클린. 무회귀(범용 `trigger-config` 패널·trigger 노드 기존 동작 보존). |

---

# SPEC-TRIGGER-SCHED-001: 설비 제어 예약 패널

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이다. `internal/node/trigger.go` 의 **Trigger 노드**는 스케줄(interval/cron/once/times/weekly/monthly)에 따라 downstream 으로 메시지를 방출하는 SourceNode 이다. 방출된 payload 는 하위 노드(예: xsfm 제어 노드 또는 mqtt)를 통해 **xsfm(지하철 설비 관리) 에이전트**의 제어 명령으로 흘러간다.

선행 SPEC-TRIGGER-PANEL-001(완료)은 대시보드에서 특정 trigger 노드를 타겟팅하여 **범용** 스케줄 CRUD + 페이로드 카탈로그 + dual-write(즉시 live 반영 + flow 정의 지속화)를 제공하는 `trigger-config` 패널을 도입했다.

본 SPEC 은 그 위에, **설비 제어 예약**에 특화된 **별도** 대시보드 패널을 도입한다. 운영자는 첨부 이미지와 같이 예약 제어 **규칙(rule)**을 표(테이블) 형태로 조회하고, 모달 팝업으로 생성/편집하며, 각 규칙에 (a) 이름 + 유효기간, (b) 대상 설비(TARGET: 전체/그룹/개별), (c) 실행 계획(PLAN: 스케줄), (d) 제어 명령(ACTION), (e) 우선순위(PRIO), (f) 활성/비활성(STATE)을 지정한다.

### 1.2 기술 환경

- **백엔드**: Go 1.23+ / `internal/node/trigger.go` (`TriggerSchedule` 구조체 확장 + `makeHandler` 발화 게이팅), 런타임 재설정 경로 `internal/api/handler/flow.go` + `internal/engine/engine.go` (SPEC-TRIGGER-PANEL-001 M1 에서 확립된 live 재무장 + `rearmGen` 세대 토큰 재사용).
- **xsfm 제어 모델**: `internal/agent/xsfm/` — 셀렉터 우선순위 `device_id > station > line > group_id` + 이름 셀렉터 `device_name`/`group_name`(SPEC-XSFM-NAMESEL-001), 제어 명령 `set_power`/`set_fan_speed`/`set_multiple`, 파라미터 축 `power`(bool)/`fan_speed`(int 1~3). **모드 축(Auto/Sleep)은 부재**(control.go:269 "2-축: power on/off, fan_speed 1/2/3").
- **프런트엔드**: React + TypeScript (`web/src/`) — 대시보드 패널 시스템(`stores/uiStore.ts`, `pages/dashboard/renderDashboardPanel.tsx`, `components/dashboard/AddPanelDialog.tsx`), 노드/flow 서비스(`services/api/nodeService.ts`, `services/api/flowService.ts`), 훅(`hooks/useNodeTypeInstances.ts`, `hooks/useFlow.ts`), 선행 유틸(`triggerPanelUtils.ts`).

### 1.3 참조 정찰 (baseline, 선행 작업)

- `internal/node/trigger.go`
  - `TriggerSchedule` 구조체(67-83): 현재 필드는 `Type`/`Value`/`Days`/`Times`/`Day`/`Payload`/`PayloadTmpl` 만 존재 — **name/유효기간/priority/enabled 부재**.
  - `parseScheduleConfig`(≈196-241): config → `[]TriggerSchedule` 파싱.
  - `registerSchedules`/`registerSingleSchedule`(390-419), `newEntry`/`addEntry`(421-441): 스케줄→타이머 엔트리 등록.
  - `makeHandler`(712-759): 발화 클로저. **게이트 순서**는 (1) `entry.gen != rearmGen`(stale drop) → (2) `paused` → (3) `lastDayGate`(monthly last) → `buildMessage` → `sourceCh` 전송. 본 SPEC 의 **enabled + 유효기간 게이트**가 삽입될 지점.
  - `buildMessage`(761-): per-schedule payload → 노드 레벨 payload → 기본 payload 순 해결.
  - `Configure` live 재무장 + `rearmGen` 세대 토큰(SPEC-TRIGGER-PANEL-001 M1), `payloadMu`.
- SPEC-TRIGGER-PANEL-001(완료): `trigger-config` 패널 + `TriggerConfigPanel.tsx`(범용 스케줄 + payload 카탈로그), `useNodeTypeInstances('trigger')` 노드 피커(running/stopped 배지), dual-write(configureNode live + getFlow→patch→updateFlow persist, 404→persist-only, last-write-wins 통지), `triggerPanelUtils.ts`.
- xsfm 제어(`internal/agent/xsfm/`): 셀렉터 라우팅(agent.go:874 "device_id > station > line > group_id"), 이름 셀렉터 승격(agent.go:166-172), 제어 명령 3종(control.go:272-351), `set_multiple` 은 power ON 을 fan 보다 먼저 방출(control.go:338).

## 2. Assumptions (가정)

- **A-1**: 본 패널은 SPEC-TRIGGER-PANEL-001 의 범용 `trigger-config` 패널을 **대체하지 않고 공존**하는 신규 패널 타입(`facility-schedule`)이다. (RD-5)
- **A-2**: 확장 메타(name/유효기간/priority/enabled)는 trigger 스케줄 config 에 저장되며, dual-write 경로(configureNode live + updateFlow persist)로 지속화된다. (RD-1 / RD-9)
- **A-3**: 규칙 payload 는 xsfm 제어 명령 형태(TARGET=셀렉터, ACTION=제어 명령)이다. 패널은 이 payload 를 TARGET 피커 + ACTION 편집기로 조립한다. (RD-2)
- **A-4**: 발화 게이팅(enabled + 유효기간)은 trigger 노드 발화 경로(`makeHandler`)에 위치한다. priority/name 은 발화에 영향을 주지 않는 메타(통과 전달)이다. (RD-1 / RD-8)
- **A-5**: 우선순위(priority)는 v1 에서 **표시 + 정렬 전용**이며, 동일 시각·중첩 대상 규칙 간 충돌 해소는 하지 않는다(하위 노드/에이전트가 도착 순서대로 각각 실행). (RD-7)
- **A-6**: trigger → xsfm 제어 배선(하위 노드가 xsfm 제어 노드/에이전트)은 flow 작성자의 책임이며, 본 패널은 payload 형태만 보장한다. (배선 가정 명시)
- **A-7**: 유효기간은 `valid_from`/`valid_to`(`YYYY-MM-DD`), 서버 로컬·날짜 단위 양끝 inclusive, 빈 `valid_from`=하한 무제한·빈 `valid_to`=무기한. (RD-8)

## 3. Requirements (요구사항, EARS)

> 표기: EARS 5패턴. 식별자·명령·셀렉터 키는 영문. REQ-ID 는 `REQ-SCHED-{모듈}-{번호}`.

### Module 01 — 스케줄 모델 확장 + 발화 게이팅 (백엔드) — REQ-SCHED-01-xx

- **REQ-SCHED-01-01 (Ubiquitous)**: 시스템은 각 trigger 스케줄이 `name`(string), `valid_from`(string, `YYYY-MM-DD` 또는 빈 값), `valid_to`(string, `YYYY-MM-DD` 또는 빈 값), `priority`(int), `enabled`(bool) 메타 필드를 **항상** 보유할 수 있도록 `TriggerSchedule` 모델을 확장해야 한다.
- **REQ-SCHED-01-02 (Ubiquitous)**: 시스템은 확장 필드가 config 에 없을 때 하위 호환 기본값(`name`=""; `valid_from`/`valid_to`=""(무기한); `priority`=0; `enabled`=true)을 **항상** 적용해야 한다. (기존 스케줄 무회귀)
- **REQ-SCHED-01-03 (State-Driven)**: **IF** 규칙의 `enabled` 가 false **THEN** 시스템은 해당 스케줄의 타이머 발화 시 메시지를 방출하지 **않아야 한다**. (RD-8)
- **REQ-SCHED-01-04 (State-Driven)**: **IF** 발화 시각이 규칙의 유효기간(**서버 로컬 시간** 기준 날짜 단위 비교, `valid_from` ≤ now ≤ `valid_to` **양끝 inclusive**, 빈 `valid_from`=하한 무제한·빈 `valid_to`=무기한) 밖 **THEN** 시스템은 메시지를 방출하지 **않아야 한다**. (RD-8)
- **REQ-SCHED-01-05 (Event-Driven)**: **WHEN** 활성(`enabled`=true)이고 유효기간(서버 로컬·양끝 inclusive) 내인 규칙의 타이머가 발화 **THEN** 시스템은 기존 게이트(gen/paused/lastDayGate) 통과 후 규칙 payload(제어 명령)를 downstream 으로 방출해야 한다. 발화 조건은 `enabled` AND `now ∈ [valid_from, valid_to]` 의 논리곱이다. (RD-8)
- **REQ-SCHED-01-06 (Ubiquitous)**: 시스템은 `priority` 와 `name` 을 발화 여부에 영향을 주지 않는 메타로 취급하고, 방출 메시지/감사 로그에 통과 전달(pass-through)해야 한다.
- **REQ-SCHED-01-07 (State-Driven)**: **IF** 대시보드에서 규칙 메타가 live 로 변경(`Configure`) **THEN** 시스템은 선행 SPEC 의 `rearmGen` 세대 토큰 규약으로 재무장하여 stale in-flight 발화의 중복/누락 없이 신규 메타를 즉시 반영해야 한다.

### Module 02 — 규칙 요약 테이블 뷰 (프런트엔드) — REQ-SCHED-02-xx

- **REQ-SCHED-02-01 (Ubiquitous)**: 패널은 규칙 목록을 **읽기 전용 요약 테이블**로 렌더링하며 컬럼은 **SCHEDULE / TARGET / PLAN / ACTION / PRIO / STATE** 여야 한다.
- **REQ-SCHED-02-02 (Ubiquitous)**: SCHEDULE 컬럼은 규칙 이름 + 유효기간 범위(`valid_from ~ valid_to`, `valid_to` 빈 값이면 "무기한")를 표기해야 한다.
- **REQ-SCHED-02-03 (Ubiquitous)**: TARGET 컬럼은 셀렉터 종류 배지(전체/그룹/개별) + 대상 설명(예: "2호선 전체", "대합실 그룹", "강남 대합실 #2")을 표기해야 한다.
- **REQ-SCHED-02-04 (Ubiquitous)**: PLAN 컬럼은 실행 계획을 사람이 읽을 수 있는 요약(예: "주간 · 월화수목금 05:30", "단발 · 2026-07-22 02:00")으로 표기해야 한다.
- **REQ-SCHED-02-05 (Ubiquitous)**: ACTION 컬럼은 제어 명령을 요약(예: "전원 ON", "전원 OFF", "풍량 1")으로 표기해야 한다.
- **REQ-SCHED-02-06 (Ubiquitous)**: PRIO 컬럼은 규칙의 `priority` 정수를 표기하고, 테이블은 기본적으로 priority 오름차순(동률은 안정 정렬)으로 정렬해야 한다. priority 는 v1 에서 표시 + 테이블 정렬 전용 메타이며 런타임 발화/충돌 해소에 영향을 주지 않는다. (RD-7)
- **REQ-SCHED-02-07 (Ubiquitous)**: STATE 컬럼은 활성/비활성 배지를 표기하며, 배지는 토글 컨트롤로 동작해야 한다(REQ-SCHED-03-06 연계).
- **REQ-SCHED-02-08 (Event-Driven)**: **WHEN** 대상 trigger 노드가 없거나 규칙이 0건 **THEN** 패널은 빈 상태 안내 + "+"(신규 규칙) 진입점을 표시해야 한다.

### Module 03 — 모달 생성/편집 (프런트엔드) — REQ-SCHED-03-xx

- **REQ-SCHED-03-01 (Event-Driven)**: **WHEN** 사용자가 "+" 를 클릭 **THEN** 패널은 빈 규칙 생성 모달을 열어야 한다.
- **REQ-SCHED-03-02 (Event-Driven)**: **WHEN** 사용자가 특정 행의 EDIT 버튼을 클릭 **THEN** 패널은 해당 규칙 값이 채워진 편집 모달을 열어야 한다.
- **REQ-SCHED-03-03 (Ubiquitous)**: 모달은 규칙의 이름, 유효기간(`valid_from`/`valid_to`), PLAN(스케줄), TARGET(피커), ACTION(편집기), priority, enabled 를 편집하는 폼을 제공해야 한다.
- **REQ-SCHED-03-04 (Event-Driven)**: **WHEN** 사용자가 모달에서 저장 **THEN** 패널은 규칙을 규칙 집합에 반영하고 dual-write(REQ-SCHED-06)를 수행한 뒤 모달을 닫고 테이블을 갱신해야 한다.
- **REQ-SCHED-03-05 (Unwanted)**: 시스템은 필수 값(이름, PLAN, TARGET, ACTION)이 누락된 규칙을 저장하지 **않아야 한다**(모달 내 검증 오류 표시).
- **REQ-SCHED-03-06 (Event-Driven)**: **WHEN** 사용자가 STATE 배지를 토글 **THEN** 패널은 해당 규칙의 `enabled` 를 반전하고 dual-write 로 지속화해야 한다.
- **REQ-SCHED-03-07 (Event-Driven)**: **WHEN** 사용자가 모달을 취소 **THEN** 패널은 변경을 버리고 규칙 집합/테이블을 원상 유지해야 한다.

### Module 04 — TARGET 피커 (셀렉터 조립) — REQ-SCHED-04-xx

- **REQ-SCHED-04-01 (Ubiquitous)**: TARGET 피커는 셀렉터 종류 **전체 / 그룹 / 개별** 을 선택하는 UI 를 제공해야 한다. (RD-6)
- **REQ-SCHED-04-02 (State-Driven)**: **IF** 사용자가 "개별" 을 선택 **THEN** 피커는 `device_id`(또는 `device_name`) 셀렉터 payload 를 조립해야 한다.
- **REQ-SCHED-04-03 (State-Driven)**: **IF** 사용자가 "그룹" 을 선택 **THEN** 피커는 `group_id`(또는 `group_name`) 셀렉터 payload 를 조립해야 한다.
- **REQ-SCHED-04-04 (State-Driven)**: **IF** 사용자가 "전체" 를 선택 **THEN** 피커는 해당 **호선 전체**를 의미하는 `line` 셀렉터 payload 를 조립해야 한다(예: "2호선 전체" → `line: "2"`). 에이전트 전역 "all" 셀렉터는 도입하지 않으며(xsfm 에 부재), "전체" 는 항상 특정 호선 단위이다. (RD-6)
- **REQ-SCHED-04-05 (Ubiquitous)**: 피커는 조립한 셀렉터를 xsfm 셀렉터 우선순위(`device_id > station > line > group_id`, 이름 셀렉터 승격 포함)와 일치하는 키로 payload 에 기록해야 한다.

### Module 05 — ACTION 편집기 (제어 명령 조립) — REQ-SCHED-05-xx

- **REQ-SCHED-05-01 (Ubiquitous)**: ACTION 편집기는 제어 명령 `set_power` / `set_fan_speed` / `set_multiple` 중 하나를 조립하는 UI 를 제공해야 한다. 축은 전원(power)·풍량(fan_speed) **2축으로 한정**하며 모드 축(Auto/Sleep)은 제공하지 않는다. (RD-4)
- **REQ-SCHED-05-02 (State-Driven)**: **IF** 사용자가 전원 축만 지정 **THEN** 편집기는 `set_power` + `params.power`(bool) payload 를 조립해야 한다.
- **REQ-SCHED-05-03 (State-Driven)**: **IF** 사용자가 풍량 축만 지정 **THEN** 편집기는 `set_fan_speed` + `params.fan_speed`(int 1~3) payload 를 조립해야 한다.
- **REQ-SCHED-05-04 (State-Driven)**: **IF** 사용자가 전원 + 풍량을 함께 지정 **THEN** 편집기는 `set_multiple` + `params.{power, fan_speed}` payload 를 조립해야 한다.
- **REQ-SCHED-05-05 (Unwanted)**: 편집기는 `fan_speed` 범위(1~3) 밖 값을 조립하지 **않아야 한다**.
- **REQ-SCHED-05-06 (Optional)**: **Where** 향후 별도 SPEC(xsfm 제어 모델 확장)으로 모드 축(Auto/Sleep)이 도입되면, 편집기는 모드 선택을 제공할 수 있다. v1 범위에서는 전원/풍량 2축으로 한정하며 모드 축은 범위 밖이다. (RD-4)

### Module 06 — Dual-Write 지속성 (선행 재사용) — REQ-SCHED-06-xx

- **REQ-SCHED-06-01 (Event-Driven)**: **WHEN** 규칙이 생성/편집/토글 **THEN** 패널은 대상 노드에 configureNode(live) + getFlow→patch→updateFlow(persist) 의 dual-write 를 수행해야 한다.
- **REQ-SCHED-06-02 (State-Driven)**: **IF** live configureNode 가 404(노드 미기동) **THEN** 패널은 persist-only 로 폴백하고 사용자에게 통지해야 한다(선행 규약 계승).
- **REQ-SCHED-06-03 (Ubiquitous)**: 패널은 동시 flow 편집에 대해 last-write-wins + 대시보드 통지 규약(선행 SPEC RD-8)을 따라야 한다.
- **REQ-SCHED-06-04 (Ubiquitous)**: 확장 메타(name/유효기간/priority/enabled) 및 TARGET/ACTION payload 는 trigger 스케줄 config 에 저장되어(별도 저장소 없이) dual-write(configureNode live + updateFlow persist) 경로로 지속화되고 재배포 후에도 유지되어야 한다. (RD-9)

### Module 07 — 비기능 요구 (NFR) — REQ-SCHED-07-xx

- **REQ-SCHED-07-01 (Ubiquitous)**: 시스템은 본 SPEC 의 신규 `facility-schedule` 패널을 범용 `trigger-config` 패널(SPEC-TRIGGER-PANEL-001)과 **공존**시켜야 하며(대체 아님), 두 패널 타입이 동시에 대시보드에 등록·사용 가능해야 한다. 본 SPEC 도입으로 범용 패널 및 trigger 노드의 기존 동작을 회귀시키지 **않아야 한다**. (RD-5)
- **REQ-SCHED-07-02 (Ubiquitous)**: 백엔드 확장은 기존 6종 스케줄 타입 및 per-schedule payload 해결 순서를 보존해야 한다.
- **REQ-SCHED-07-03 (Ubiquitous)**: 발화 게이팅은 lock-holding 함수 내 재귀 RLock 을 유발하지 않아야 하며(`-race` 클린), 코드 커버리지 85% 이상을 유지해야 한다.

## 4. Specifications (사양)

### 4.1 확장된 TriggerSchedule 필드

`internal/node/trigger.go` `TriggerSchedule` 에 다음 필드를 추가한다(제안):

| 필드          | 타입     | config 키       | 기본값        | 의미 |
| ----------- | ------ | -------------- | ---------- | -- |
| `Name`      | string | `name`         | ""         | 규칙 표시 이름 (SCHEDULE 컬럼) |
| `ValidFrom` | string | `valid_from`   | ""         | 유효 시작(`YYYY-MM-DD`), 빈 값=하한 무제한 |
| `ValidTo`   | string | `valid_to`     | ""         | 유효 종료(`YYYY-MM-DD`), 빈 값=무기한 |
| `Priority`  | int    | `priority`     | 0          | 표시/정렬 우선순위 (PRIO 컬럼) |
| `Enabled`   | *bool  | `enabled`      | true       | 활성 여부 (STATE 토글). 포인터/부재=true 로 하위 호환 |

`triggerTimerEntry`(85-)에도 발화 게이팅에 필요한 값(`enabled`, `validFrom`, `validTo`)을 `newEntry`(424) 시점에 캡처해 전달한다. `priority`/`name` 은 발화에 불필요하므로 payload/메시지 메타로만 전달.

### 4.2 유효기간 + 활성 발화 게이팅

`makeHandler`(712-759) 클로저의 게이트 순서에 삽입한다(제안 위치: `gen` 게이트와 `paused` 게이트 사이 또는 `lastDayGate` 직전):

1. `entry.gen != rearmGen` → drop (기존)
2. `paused` → return (기존)
3. **NEW**: `entry.enabled == false` → return (REQ-SCHED-01-03)
4. **NEW**: `now` 가 `[validFrom, validTo]` 밖 → return (REQ-SCHED-01-04)
5. `lastDayGate` (기존, monthly last)
6. `buildMessage` → `sourceCh` (기존)

유효기간 경계·타임존 의미(RD-8 확정): **서버 로컬 시간** 기준 **날짜 단위** 비교, `valid_from` 00:00:00 포함 ~ `valid_to` 23:59:59 포함 = **양끝 inclusive**, 빈 `valid_from`=하한 무제한, 빈 `valid_to`=무기한(상한 무제한). 발화는 `enabled==true` **AND** `now ∈ [valid_from, valid_to]` 일 때만 허용된다.

### 4.3 payload = 제어 명령 형태 (TARGET→셀렉터 + ACTION→명령)

규칙 payload 는 xsfm 제어 exec 계약과 호환되는 JSON 객체이다(예):

```
// 개별 · 전원 ON
{ "command": "set_power", "device_id": "GN-HALL:P1:2", "params": { "power": true } }

// 그룹 · 풍량 1 (전원 ON 동반)
{ "command": "set_multiple", "group_id": "hall-group", "params": { "power": true, "fan_speed": 1 } }

// 전체(호선 전체) · 전원 OFF   ← RD-6: "전체" = line 셀렉터(2호선 전체)
{ "command": "set_power", "line": "2", "params": { "power": false } }
```

- **TARGET → 셀렉터 키(RD-6)**: 개별=`device_id`/`device_name`, 그룹=`group_id`/`group_name`, 전체=`line`(해당 호선 전체, 예 `line: "2"`). 에이전트 전역 "all" 셀렉터는 도입하지 않는다(xsfm 에 부재).
- **ACTION → 명령(RD-4)**: 전원=`set_power`, 풍량=`set_fan_speed`, 전원+풍량=`set_multiple`. 파라미터 축은 `power`(bool)/`fan_speed`(int 1~3) **2축으로 한정**하며 모드 축(Auto/Sleep)은 v1 범위 밖(payload 에 `mode` 키를 넣지 않음).
- payload 는 선행 per-schedule payload 경로(`Payload`/`PayloadTmpl`, buildMessage 766-)에 실려 방출된다.

### 4.4 테이블 컬럼 매핑

| 컬럼       | 소스 |
| -------- | -- |
| SCHEDULE | `name` + `valid_from ~ valid_to`(무기한) |
| TARGET   | 셀렉터 종류 배지(전체/그룹/개별) + 셀렉터 값 설명 |
| PLAN     | 스케줄 타입 + 요일/시각 요약 (interval/cron/once/times/weekly/monthly) |
| ACTION   | `command` + `params` 요약 (전원 ON/OFF, 풍량 N) |
| PRIO     | `priority` |
| STATE    | `enabled` 토글 배지 |

### 4.5 모달 폼

모달은 4.4 의 각 소스를 편집한다: 이름/유효기간(날짜 입력), PLAN(스케줄 타입 + 세부), TARGET(피커, Module 04), ACTION(편집기, Module 05), priority(정수), enabled(토글). 저장 시 규칙 → config 스케줄 항목으로 직렬화 후 dual-write.

### 4.6 priority 처리 (v1)

v1 에서 priority 는 테이블 정렬 + PRIO 표기 전용이다(RD-7). 동일 시각·중첩 대상 규칙은 각각 독립 발화되어 하위 노드/xsfm 에 도착 순서대로 전달된다(런타임 충돌 해소 없음). 충돌 해소(고우선 우선/후발 우선)는 후속 SPEC 으로 유보.

## 5. Resolved Decisions (확정 결정, RD)

- **RD-1 — 스케줄 모델 확장 + 백엔드 발화 존중**: `TriggerSchedule` 에 `name`/`valid_from`/`valid_to`/`priority`/`enabled` 를 신규 필드로 추가한다. 백엔드는 발화 시점(`makeHandler`)에 이를 존중한다 — 비활성 규칙은 발화하지 않고, 유효기간 밖(valid_from 이전/valid_to 이후)에는 발화하지 않는다. priority/name 은 발화 비영향 메타로 통과 전달한다.
- **RD-2 — payload = xsfm 제어 명령(설비 제어 예약 전용)**: 각 규칙의 payload 는 xsfm 제어 명령이다. TARGET 은 제어 셀렉터(전체/그룹/개별 → `group_id`/`station`/`line`/`device_id` + 이름 셀렉터 `group_name`/`device_name`)로, ACTION 은 제어 명령(`set_power`/`set_fan_speed`/`set_multiple` + `power`/`fan_speed` 축)으로 매핑된다. 패널은 TARGET 피커 + ACTION 편집기로 이 payload 를 조립한다. 본 패널은 범용 트리거가 아니라 설비 제어 스케줄링에 특화된다.
- **RD-3 — 테이블 + 모달 UX**: 패널은 읽기 전용 요약 테이블(SCHEDULE/TARGET/PLAN/ACTION/PRIO/STATE, 이미지 일치)을 렌더링한다. 생성/편집은 인라인이 아니라 모달 팝업에서 수행한다. 각 행에는 모달을 여는 EDIT 버튼이 있고, STATE 배지는 활성/비활성을 토글한다. "+" 는 모달을 통해 신규 규칙을 추가한다.
- **RD-4 — ACTION 은 xsfm 2축 한정(모드 축 없음, v1)**: ACTION 은 기존 xsfm 2축 제어로 한정한다 — 전원 ON/OFF(`set_power`) + 풍량 1~3(`set_fan_speed`) + 둘 다(`set_multiple`). xsfm 제어 모델에 모드 축(Auto/Sleep)이 없으므로 v1 에서 모드 축을 도입하지 않는다(별도 xsfm 제어 모델 SPEC 으로 유보). 결과: ACTION 라벨은 모드를 생략한다 — 예 "전원 ON", "전원 OFF", "풍량 1", "전원 ON · 풍량 2"(Auto/Sleep 아님). 이미지에 있던 Auto/Sleep 표기는 2축 현실로 대체한다.
- **RD-5 — 신규 패널 공존(대체 아님)**: 본 SPEC 은 설비 예약 제어에 특화된 **신규 별도 대시보드 패널 타입**(`facility-schedule`)을 도입하며, 범용 `trigger-config` 패널(SPEC-TRIGGER-PANEL-001, 무변경)과 **공존**한다. `facility-schedule` 은 예약 설비 제어 전용으로 특화된다.
- **RD-6 — "전체" = 호선(line)**: TARGET 은 3단계이다 — 전체(=호선 전체, `line` 셀렉터로 매핑, 예 "2호선 전체"), 그룹(=`group_id`/`group_name`), 개별(=`device_id`/`device_name`). 에이전트 전역 "all" 셀렉터는 도입하지 않는다(xsfm 에 부재). "전체" → `line` 셀렉터.
- **RD-7 — priority 는 표시/정렬 전용**: PRIO 는 v1 에서 표시 + 테이블 정렬 전용이다. 중첩 대상/시각 규칙 간 런타임 충돌 해소는 하지 않는다(유보). priority 는 규칙에 실리는 메타이다.
- **RD-8 — 유효기간 게이팅**: `valid_from`/`valid_to` 는 **서버 로컬 시간** 기준으로 발화를 게이팅하며 **날짜 단위 양끝 inclusive** 이다. 빈 `valid_to`=무기한(상한 무제한), 빈 `valid_from`=하한 무제한(과거 무제한). 규칙은 `now` 가 `[valid_from, valid_to]` 내이고 **AND** `enabled` 일 때만 발화한다.
- **RD-9 — 지속성**: 확장 필드(name/valid_from/valid_to/priority/enabled)는 trigger 스케줄 config 에 dual-write 경로(configureNode live + updateFlow persist)로 지속화된다 — RD-1 과 일관.

## 6. Open Questions (미해결 질문)

없음 — OQ-1~6 은 RD-4~9 로 확정되었다. (OQ-1→RD-5, OQ-2→RD-8, OQ-3→RD-7, OQ-4→RD-6, OQ-5→RD-4, OQ-6→RD-9)

## 7. Traceability (추적성)

- REQ-SCHED-01-xx → `internal/node/trigger.go` (`TriggerSchedule`, `newEntry`, `triggerTimerEntry`, `makeHandler`, `parseScheduleConfig`)
- REQ-SCHED-02-xx / 03-xx → `web/src/pages/dashboard` 신규 패널 컴포넌트 + 테이블/모달
- REQ-SCHED-04-xx / 05-xx → TARGET 피커 / ACTION 편집기 (신규 프런트 유틸)
- REQ-SCHED-06-xx → 선행 dual-write 경로 재사용(`nodeService`/`flowService`, `triggerPanelUtils.ts`)
- REQ-SCHED-07-xx → 회귀 테스트 (기존 trigger 노드 + `trigger-config` 패널)

## 8. 구현 노트 (as-implemented, spec-anchored Level 2)

> v0.3.0 구현 완료 시점 기록. 계획(§4/§5) 대비 실제 구현의 정련·구체화 7건. 모두 스코프 확장이 아니라 정확성·하위호환을 강화하는 방향이다. 구현 커밋: 백엔드 `03e8827b`(M1~M2), 프런트 `fb40c4b2`(M3~M6).

- **IN-1 — `Enabled` = `*bool` 트라이스테이트**: `TriggerSchedule.Enabled` 를 `bool` 이 아닌 `*bool` 포인터로 구현했다. config 에 `enabled` 키가 **부재**하면 `nil` → `true`(기본 활성)로 해석해 기존 스케줄을 무회귀로 유지하고, 명시적 `false` 는 `nil`(부재)과 구별 가능하다. (REQ-SCHED-01-01/02, §4.1 확정 구현)
- **IN-2 — 날짜 포맷 = `YYYY-MM-DD` + RFC3339 수용**: 유효기간 파싱은 `YYYY-MM-DD` 뿐 아니라 RFC3339 도 수용하되, 비교는 RD-8 대로 **서버 로컬·날짜 단위**로 절삭해 수행한다. 잘못된(파싱 실패) 날짜는 방어적으로 **해당 경계 무제한**으로 처리해 오발화를 방지한다. (REQ-SCHED-01-04, RD-8; §4.2 의 "미발화 처리"에서 "경계 무제한"으로 정련)
- **IN-3 — `rule_name`/`priority` 메타 조건부 pass-through**: `buildMessage` 는 `name`/`priority` 가 지정된 규칙에 한해 방출 메시지에 `rule_name`/`priority` 메타를 실어 통과 전달한다. 미지정 규칙(기존 스케줄)의 방출 메시지는 **byte-identical** 로 보존된다. (REQ-SCHED-01-06, §4.3 무회귀 강화)
- **IN-4 — TARGET 열거는 패널 config `agentId` 기반**: TARGET 피커는 패널 config 의 `agentId`(추가 시점 + 패널 내 셀렉터)로 대상을 열거한다. `agentId` 가 **설정된 경우** id 셀렉터(호선/그룹/개별)를 `useStations`/`useGroups`/`useXsfmDevices` 로 열거하고, **미설정인 경우** free-form 텍스트 입력 + "이름으로 지정" 토글(→ `group_name`/`device_name`)로 폴백한다. (REQ-SCHED-04-02~05, §4.5 피커 구현 구체화)
- **IN-5 — ACTION 인코딩 = `{power:bool|null, fanSpeed:number|null}`**: ACTION 편집기 내부 모델은 `power`/`fanSpeed` 2축을 각각 `null`(무변경) 허용으로 인코딩한다. `buildActionCommand` 는 무-축(둘 다 미지정)·범위 밖(`fan_speed` 1~3 위반) 시 저장을 차단하고, payload 에 `mode` 키를 **어떤 경우에도 방출하지 않는다**. (REQ-SCHED-05-01~06, RD-4; §4.3/§4.5 확정 구현)
- **IN-6 — PLAN = `TriggerScheduleEditor` 단일 원소 배열 재사용**: PLAN(스케줄) 편집은 선행 `TriggerScheduleEditor` 를 단일 원소 배열에 대해 재사용하며 **타이밍 키만** 편집한다. payload(제어 명령)는 ACTION 편집기가 소유해 관심사를 분리한다. (§4.5 모달 폼 구현 구체화, 단순성 사다리 재사용)
- **IN-7 — dual-write + STATE 토글 단일 `persist()` 경로**: 규칙 생성/편집과 STATE 토글은 선행 `triggerPanelUtils`(`buildFullTriggerConfig`/`patchNodeConfigInDefinition`/`detectConflict`)를 재사용하는 단일 `persist()` 경로를 통해 dual-write 된다 — 범용 패널과 동일한 LIVE(`configureNode`)→PERSIST(`updateFlow`)→404 persist-only→last-write-wins 규약을 그대로 계승한다. (REQ-SCHED-06-01~04, RD-9; §4.5 지속화 구현)
