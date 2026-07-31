---
id: SPEC-SCHEDULE-VIEW-001
title: "Schedule View — 에이전트별 스케줄 관리 + 스케줄 실행 로그"
version: "0.3.0"
status: in-progress
created: 2026-07-31
updated: 2026-08-01
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/node + internal/agent/xsfm + internal/storage + internal/api + web/src"
lifecycle: spec-anchored
tier: L
tags: "schedule, view, per-agent, schedule-log, execution-log, correlation, trigger, xsfm, facility-schedule, append-only, sqlite, dual-write, dual-write-hook, react-router, sidebar-nav, aggregation, frontend, backend, storage, api"
depends_on:
  - SPEC-TRIGGER-SCHED-001
  - SPEC-FACILITY-DASHBOARD-001
  - SPEC-XSFM-GROUP-001
  - SPEC-XSFM-001
  - SPEC-REMOTE-001
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                                                                                                                                                                                                                                                                                                     |
| ---------- | ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-31 | 0.1.0 | 초기 SPEC 작성 — 설비 제어 예약 대시보드 패널(SPEC-TRIGGER-SCHED-001)을 **전용 풀페이지 VIEW** 로 확장. (1) **스케줄에 대상 에이전트 명시 저장**(RD-3, `config.schedules[].agent_id`), (2) **스케줄 실행 로그 전용 저장소 신설**(RD-1, append-only SQLite, `remote_audit_*` 패턴 미러) + **발화↔제어결과 상관** 기록(RD-2), (3) **로그 조회 API 신설**, (4) **전용 페이지 `/schedules` + 사이드바 nav**(RD-4) — 크로스-플로우 집계 에이전트별 그룹핑 + CRUD(dual-write hook 추출 재사용) + 로그 뷰, (5) 기존 설비 예약 **대시보드 패널은 불변 병존**. 열린 질문 §6 (OQ-1 ~ OQ-7) 사용자 확정 대기. |
| 2026-07-31 | 0.2.0 | 열린 질문 OQ-1~7 **전부 사용자 확정 → RD-5~11 로 승격**, §6 Open Questions 소거(잔여 없음). (RD-5) `/schedules` 페이지 + 로그 조회 API 를 **전체 인증 사용자** 접근으로 확정(admin 전용 아님, AgentListPage 동급). (RD-6) 로그의 agent 권위 = **실제 실행 에이전트(auditActor/AgentRef)** + **선언 `agent_id`(RD-3) 병기** — 선언↔실제 불일치 가시화. (RD-7) 발화당 **집계 1건** + 대상별 ok/error **임베드**(N건 아님). (RD-8) **발화-only 도 기록** — 트리거 발화 시점에 fire 로그 이벤트를 (주입된 관측자 경유로) 기록하고, 제어 경로가 동일 상관 id(`schedule_id + trigger_time`)로 result 이벤트를 append 하여 조인. 제네릭 트리거 노드는 스케줄-로그 저장소에 비결합(주입 관측자). (RD-9) agent 필드는 **스케줄별**(`config.schedules[].agent_id`). (RD-10) 크로스-플로우 집계는 **프런트 `useQueries` 팬아웃**, v1 백엔드 집계 엔드포인트 없음(후속). (RD-11) v1 **무제한 append-only**, 보존/pruning 정책 후속. |
| 2026-08-01 | 0.3.0 | **as-implemented sync** — M1~M7 구현 완료 후 확장/버그수정을 반영(M8~M13, §8). (A) M5/M6 버그수정 3건(§8.0): 규칙 모달의 xsfm 에이전트 경유 TARGET 열거, 스케줄 0개 트리거 노드 관리 뷰 숨김, 삭제-영속 정상 확인(크로스-서피스 에디터 staleness 로 판명). (B, M8) 로그 저장소 **3-백엔드 팩토리**(sqlite 기본/memory/JSONL) + `storage.schedule_log.type` config + 인터페이스 `Clear`/`Count` 추가 → **RD-11 수정**(개별 삭제는 여전히 부재, 수동 전체 초기화(Clear) 예외 허용). (C, M9) `GET /schedules/logs` 응답을 `{items,total}` 로 변경 + `DELETE /schedules/logs`(전체 인증) + `GET/PUT /system/schedule-log-config`(admin). (D, M10) 로그 탭 페이지네이션(25/50/100)·CSV 내보내기·전체 초기화. (E, M11) admin 전용 저장방식 Settings 카드. (F, M12) TARGET 선택을 **테이블**(이름/라인/역사/위치)로. (G, M13) 관리 탭을 **에이전트별 그룹 → 단일 플랫 테이블(에이전트 이름 컬럼)** 로 개편 → **RD-3/RD-7 수정** + 규칙 생성 모달 내부화 + 트리거→제어 하류 엣지 순회 **agent 자동 도출**(`deriveDownstreamAgents`) → **RD-3/RD-9 수정**. 3건 amendment 는 §9 에 기록(원 RD 텍스트 보존 + 승계 노트). |

---

# SPEC-SCHEDULE-VIEW-001: Schedule View — 에이전트별 스케줄 관리 + 스케줄 실행 로그

> **버전 노트 (0.3.0)** — as-implemented 동기화. M1~M7(0.2.0 설계) 구현이 완료·테스트·green 상태로 종료된 뒤 도입된 확장(M8~M13)과 M5/M6 버그수정을 문서화한다. §1~§7(M1~M7)은 **불변 보존**하고, §8(0.3.0 확장 as-implemented)과 §9(0.3.0 설계 수정 Amendments)를 가산했다. 3건의 설계 수정(**RD-11** 전체 초기화(Clear) 예외, **RD-3/RD-7** 에이전트별 그룹→플랫 테이블, **RD-3/RD-9** create 시 하류 엣지 순회 agent 자동 도출)은 §9 에 승계 노트로 기록하며, 원 RD 텍스트(§5)는 덮어쓰지 않는다. 본 SPEC 은 아직 sync-close 되지 않았다(status: in-progress).

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이며, 트리거(trigger) 노드가 스케줄에 따라 메시지를 발화하고, 그 메시지가 `xsfm-control` / 상태·제어 통합 `xsfm` 노드를 거쳐 지하철 설비(facility) 에이전트를 제어한다(SPEC-XSFM-001, SPEC-XSFM-GROUP-001 AM-1).

SPEC-TRIGGER-SCHED-001 은 이 스케줄을 **대시보드 패널**(설비 제어 예약 패널, 규칙 테이블 + 모달 편집)로 관리하는 기능을 도입했다. 본 SPEC 은 그 작업을 **전용 풀페이지 VIEW** 로 승격하여 다음 두 가지를 추가한다.

1. **에이전트별 스케줄 관리** — 전체 플로우에 흩어진 트리거 스케줄을 크로스-플로우로 집계하여 **대상 에이전트별로 그룹핑** 하고, 한 화면에서 CRUD 한다.
2. **스케줄 실행 로그 관리** — 스케줄 **발화(fire)** 와 그 발화로 인한 **제어 결과(control result)** 를 상관(correlate)하여 전용 저장소에 기록하고, 스케줄/규칙/에이전트로 필터·조회한다.

### 1.2 기술 환경

- **트리거 노드**: `internal/node/trigger.go` — `TriggerSchedule` 구조체(:68-95)에 규칙 메타(`Name`/`ValidFrom`/`ValidTo`/`Priority`/`Enabled`)가 이미 존재(SPEC-TRIGGER-SCHED-001 M1). `buildMessage`(:930-965)가 발화 시 `trigger.schedule_id` / `trigger.rule_name` / `trigger.priority` 상관 메타데이터를 방출.
- **xsfm 제어 경로**: `internal/node/xsfm.go` — `XsfmControlNode.Process`(:474) → `buildXsfmControlCommand`(:936). 명령 envelope 를 `msg.Payload()` 로부터 구성하지만 **`msg.Metadata()` 의 트리거 상관 메타를 명령에 실어 보내지 않음**(현재 상관 단절 지점). `XSFMNodeConfig.AgentRef`(:53)가 실행 대상 에이전트.
- **xsfm 감사 경로**: `internal/agent/xsfm/audit.go` — `recordControlAudit(res memberResult)`(:74) / `recordGroupAudit(sel, results)`(:105) 가 `storage.RemoteAuditRecord` 로 제어 결과를 기록하나 **스케줄/규칙 상관 필드가 없음**.
- **append-only 저장소 패턴(재사용 템플릿)**: `internal/storage/remote_audit_repository.go`(인터페이스, `RemoteAuditRecord` :48) + `internal/storage/remote_audit_sqlite.go`(`Append`/`List` :88/:101, 추가 전용, 최신순 + limit/offset). `main.go` startup 시 `xsfm.SetAuditRepository` 싱글턴 주입.
- **로그 API 패턴(재사용 템플릿)**: `internal/api/handler/remote_admin.go` — `GET /remote/audit`(:163, `h.Audit` :331) admin-gated(`requireAdmin` :178, role=admin) + `?instance_id=` 필터 + `limit`/`offset`. `cmd/xflowd/main.go:871` `server.RegisterRoutes` 에서 핸들러 배선.
- **프런트 라우팅**: `web/src/router.tsx` — `createBrowserRouter` + `React.lazy`, `AuthGuard` → `AppLayout` 하위 child route. 페이지 패턴 `web/src/pages/agents/AgentListPage.tsx`. 사이드바 `web/src/components/layout/Sidebar.tsx` `NAV_ENTRIES`(:65-147, i18n `labelKey`, `roles` 게이팅).
- **스케줄 조회 훅**: `web/src/hooks/useFlow.ts` — `useFlows`(:11) / `useFlowNodes`(:35, `config.schedules`). 에이전트: `web/src/hooks/useAgent.ts` `useAgents`(:11), xsfm 필터 `a.type==='xsfm'`.
- **재사용(순수/이식형)**: `web/src/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils.ts`(셀렉터/액션 build-parse, formatter, 정렬, rule↔schedule), `web/src/pages/dashboard/panels/triggerPanelUtils.ts`(`buildFullTriggerConfig` / `patchNodeConfigInDefinition` / `detectConflict` dual-write 헬퍼), `FacilityRuleModal.tsx`(이식형, `agentId` 필요). dual-write 오케스트레이션은 현재 `FacilitySchedulePanel.persist`(:133-179)에 인라인.

### 1.3 현재 상태 (as-is)

- 스케줄은 **트리거 노드의 `config.schedules`** 에만 존재. 에이전트 바인딩은 대시보드 패널의 `config.agentId`(패널-로컬, FacilitySchedulePanel.tsx:73)로만 잡히며 **스케줄 단위로 영속되지 않음**. 페이로드 셀렉터는 `group_id`/`device_id` 를 기록할 뿐 에이전트를 기록하지 않음.
- 스케줄 발화는 **휘발성**(trigger.go `makeHandler`, `tick_count` 미영속). 제어 감사(`remote_audit`)는 존재하나 **스케줄/규칙 상관이 없음**. 트리거가 방출하는 상관 메타(`trigger.schedule_id`/`rule_name`)는 하류 xsfm 제어 경로에서 **버려짐**.
- 스케줄 관리 UI 는 **대시보드 패널** 뿐이며, 전용 페이지·에이전트별 집계·실행 로그 뷰가 없음.

### 1.4 범위 경계

**범위 내(in-scope)**:

- 스케줄에 대상 에이전트를 명시 영속(`config.schedules[].agent_id`)하는 트리거 노드/프런트 확장.
- 스케줄 실행 로그 **전용** 저장소(신설, append-only SQLite) + 발화↔제어결과 상관 기록.
- 스케줄 로그 조회 API 신설.
- 전용 풀페이지 `/schedules`(에이전트별 스케줄 CRUD + 로그 뷰) + 사이드바 nav 1건.
- dual-write 오케스트레이션을 공유 훅으로 추출(패널 + View 재사용).

**범위 외(out-of-scope)**:

- 기존 설비 제어 예약 **대시보드 패널의 UI/동작 변경**(불변 병존 — RD-4). alias/호환만 허용, 기능 개편 없음.
- 트리거 발화 게이팅 로직 변경(유효기간/enabled 판정은 SPEC-TRIGGER-SCHED-001 규약 불변).
- MQTT 규약·에이전트 제어 프로토콜 변경.
- 기존 `remote_audit` 저장소 스키마 변경(오버로딩 금지 — RD-1).

---

## 2. Assumptions (가정)

- **A-1**: 트리거 노드의 `config.schedules` 는 SPEC-TRIGGER-SCHED-001 확장 필드(name/valid_from/valid_to/priority/enabled)를 이미 지원하며, 미지정 확장 필드는 하위 호환 기본값으로 처리된다(무회귀). 본 SPEC 은 여기에 `agent_id` 필드를 **가산(additive)** 한다.
- **A-2**: 스케줄 발화 메시지는 트리거 → (와이어) → `xsfm-control` / 통합 `xsfm` 노드 → 에이전트 경로로 흐르며, `message.Metadata()` 는 이 경로에서 보존·전달 가능하다(`buildMessage` 가 이미 `trigger.schedule_id` 등을 실음).
- **A-3**: `remote_audit_*` 의 append-only + 최신순 List + limit/offset 패턴은 스케줄 실행 로그에도 그대로 이식 가능하다(스키마 필드만 상이).
- **A-4**: 대다수 스케줄은 하나의 대상 에이전트를 가진다(에이전트별 그룹핑이 1:N 로 자연스럽다). 다중 에이전트/셀렉터 팬아웃 스케줄은 부차적 케이스로 처리한다(§6 OQ-3 참조).
- **A-5**: 전체 플로우 수는 UI 크로스-플로우 집계를 `useQueries` 팬아웃으로 감당 가능한 규모이다(수백 이하). 초과 시 백엔드 집계 엔드포인트로 승급한다(§6 OQ-4).
- **A-6**: 로그 저장소는 관리 서버 측(xflowd) 단일 인스턴스에 영속되며, 프로젝트 타임스탬프 규약(epoch milliseconds, int64 `UnixMilli`)을 따른다.

---

## 3. Requirements (요구사항 — EARS)

> 표기: **The [system] shall …**(Ubiquitous), **When [event], the [system] shall …**(Event-Driven), **While [state], the [system] shall …**(State-Driven), **Where [feature], the [system] shall …**(Optional), **If [undesired], then the [system] shall …**(Unwanted). 식별자·필드명은 영문.

### Module 1 — Schedule Agent Field (스케줄 대상 에이전트 명시 저장)

- **REQ-01-01** (Ubiquitous): 트리거 노드 스케줄 스키마는 스케줄 항목별 대상 에이전트 식별자 필드 `config.schedules[].agent_id` (문자열, 선택)를 지원해야 한다.
- **REQ-01-02** (Ubiquitous): `agent_id` 는 발화 여부에 영향을 주지 않는 pass-through 메타여야 한다(`Name`/`Priority` 와 동일 취급). 빈 값/부재는 하위 호환 기본값(미바인딩)으로 처리해야 한다.
- **REQ-01-03** (Event-Driven): When 트리거 노드가 `agent_id` 가 설정된 스케줄로 메시지를 발화할 때, the 트리거 노드 shall 발화 메시지에 상관 메타데이터 `trigger.agent_id` 를 실어야 한다.
- **REQ-01-04** (State-Driven): While 기존 스케줄(`agent_id` 부재)이 존재하는 동안, the 트리거 노드 shall `trigger.agent_id` 메타를 추가하지 않고 무회귀로 계속 발화해야 한다.
- **REQ-01-05** (Ubiquitous): 설비 제어 예약 대시보드 패널과 Schedule View 는 **모두** 스케줄 저장 시 `agent_id` 를 `config.schedules[].agent_id` 로 기록해야 한다(패널의 패널-로컬 `config.agentId` 를 스케줄 단위로 승격).

### Module 2 — Schedule-Execution-Log Store + Correlation (실행 로그 전용 저장소 + 상관)

- **REQ-02-01** (Ubiquitous): 시스템은 스케줄 실행 로그 **전용** append-only 저장소를 신설해야 하며, 기존 `remote_audit` 저장소를 오버로딩하지 않아야 한다(RD-1).
- **REQ-02-02** (Ubiquitous): 로그 레코드는 최소 다음 필드를 포함해야 한다 — `id`(자동증가 PK), `correlation_id`(조인 키 = `schedule_id`+`trigger_time`, RD-8), `record_kind`(`fire` | `result`, RD-8), `schedule_id`, `rule_name`, `declared_agent_id`(스케줄 선언 에이전트, RD-9/RD-6 보조), `actor_agent_id`(실제 실행 에이전트 — RD-6 권위, fire 이벤트에는 빈 값), `trigger_time`(epoch ms), `target`(셀렉터: device_id/group_id 종류·값), `action`(제어 명령), `result`(집계 ok/error — fire 이벤트에는 없음), `targets`(대상별 ok/error 임베드, RD-7), `reason`(비밀 아님 사유/오류 분류), `timestamp`(기록 시각 epoch ms). (스키마 확정 §4.1)
- **REQ-02-03** (Ubiquitous): 저장소는 갱신/삭제 API 를 노출하지 않아야 한다(추가 전용, 감사 무결성 — `remote_audit` 패턴).
- **REQ-02-04** (Event-Driven): When xsfm 제어 경로가 트리거 상관 메타(`trigger.schedule_id`)를 가진 발화 메시지를 처리할 때, the xsfm 제어 경로 shall 그 상관 메타(`schedule_id`/`rule_name`/선언 `agent_id`)와 **실제 실행 에이전트(`actor_agent_id`, RD-6 권위)** 및 **제어 결과** 를 담은 `record_kind=result` 로그 이벤트를 발화 fire 이벤트와 동일한 `correlation_id`(RD-8)로 저장소에 append 해야 한다(RD-2).
- **REQ-02-04b** (Ubiquitous): 하나의 발화가 셀렉터 fan-out(그룹/다중 device)으로 여러 대상을 제어할 때, the xsfm 제어 경로 shall 발화당 **집계 result 이벤트 1건** 을 기록하고 대상별 ok/error 를 그 레코드에 **임베드(`targets`)** 해야 하며, 대상별 N 건을 별도로 남기지 않아야 한다(RD-7).
- **REQ-02-04c** (Ubiquitous): 로그의 `actor_agent_id` 는 **실제 실행 에이전트**(auditActor/`XSFMNodeConfig.AgentRef`)를 권위 값으로 기록해야 하며, 스케줄의 선언 `agent_id`(RD-9)를 `declared_agent_id` 로 병기하여 선언↔실제 불일치가 조회 시 가시화되어야 한다(RD-6).
- **REQ-02-05** (Ubiquitous): 상관 반송 경로(carry path)는 `buildXsfmControlCommand` 가 `msg.Metadata()` 의 `trigger.schedule_id`/`trigger.rule_name`/`trigger.agent_id`/`trigger.priority` 를 읽어 에이전트 명령 envelope 에 상관 필드로 실어 보내고, 에이전트 제어 경로(`recordControlAudit`/`recordGroupAudit` 인접)가 그 상관 필드 + 결과로 스케줄 로그를 기록하는 방식이어야 한다(§4.2).
- **REQ-02-06** (State-Driven): While 발화 메시지에 **스케줄 상관 메타(`schedule_id`)가 전혀 없는 동안**(스케줄 외 수동 제어 등), the xsfm 제어 경로 shall result 로그 이벤트를 기록하지 않아야 한다(기존 제어 무회귀 — 스케줄 로그는 스케줄-상관에만 남는다). (참고: 스케줄이 발화했으나 하류 제어가 없는 fire-only 는 REQ-02-09 로 fire 이벤트가 기록된다 — 이는 상관 메타가 있는 경우이므로 본 조항과 상충하지 않는다.)
- **REQ-02-07** (Unwanted): If 로그 저장소가 미구성이거나 기록이 실패하면, then the 시스템 shall 제어를 실패시키지 않고 로깅만 하는 best-effort no-op 으로 처리해야 한다(`remote_audit` best-effort 패턴 계승).
- **REQ-02-08** (Unwanted): The 로그 저장소는 시크릿(토큰/명령 인자 전체/원본 페이로드)을 저장하지 않아야 한다(도메인/셀렉터/명령/결과/사유만 — `remote_audit` REQ-F06 계승).
- **REQ-02-09** (Event-Driven): When 스케줄이 발화할 때(하류 제어 연결 여부와 무관하게), the 시스템 shall 발화 시점에 `record_kind=fire` 로그 이벤트(`correlation_id`=`schedule_id`+`trigger_time`, `schedule_id`/`rule_name`/`declared_agent_id`/`trigger_time` 포함, result 없음)를 기록해야 한다(RD-8 발화-only 기록). 이 fire 이벤트는 **주입된 fire 관측자**(§4.2)가 기록하며 제네릭 트리거 노드가 직접 저장소를 호출하지 않는다.
- **REQ-02-10** (Ubiquitous): fire 이벤트와 result 이벤트는 **동일한 `correlation_id`**(`schedule_id`+`trigger_time`)로 조인되어 조회 시 **하나의 논리 레코드**(발화 + 선택적 제어결과)로 표현되어야 한다. 하류 제어에 도달한 발화는 fire+result 로, fire-only 발화는 fire 만으로 나타나야 한다(RD-8). (append-only + 갱신 API 부재(REQ-02-03) 규약상 fire 와 result 는 별도 append 이벤트이며, 단일 논리 레코드는 조회 시점의 조인이다.)
- **REQ-02-11** (Unwanted): If 제네릭 트리거 노드가 스케줄-로그 저장소(`ScheduleLogRepository`) 타입에 직접 결합되면, then the 변경 shall 거부되어야 한다(RD-8 비결합). 트리거 노드는 제네릭 관측자 인터페이스(`ScheduleFireObserver`, 미주입 시 no-op)만 알아야 하며, 스케줄-로그 저장소 결합은 주입된 관측자(facility-schedule 측)에 둔다.
- **REQ-02-12** (Ubiquitous): v1 로그 저장소는 **무제한 append-only**(pruning 없음)여야 한다(RD-11). 보존 한도/정리(pruning) 정책은 후속 SPEC 으로 이연한다.

### Module 3 — Schedule-Log Query API (로그 조회 API)

- **REQ-03-01** (Ubiquitous): 시스템은 스케줄 실행 로그 조회 REST 엔드포인트(예: `GET /schedules/logs`)를 제공해야 한다.
- **REQ-03-02** (Ubiquitous): 조회 API 는 `schedule_id` / `rule_name` / `agent_id` 필터와 `limit`/`offset` 페이지네이션을 지원하며 최신순(timestamp 내림차순)으로 반환해야 한다.
- **REQ-03-03** (State-Driven): While 로그 저장소가 미구성인 동안, the 조회 API shall 오류 대신 빈 목록을 반환해야 한다(`GET /remote/audit` 패턴).
- **REQ-03-04** (Event-Driven): When 필터 파라미터가 주어질 때, the API shall 해당 필터로 좁힌 레코드만 반환해야 한다.
- **REQ-03-05** (Ubiquitous): 로그 조회 API 는 **전체 인증 사용자** 에게 접근 가능해야 하며(admin 전용 아님 — `AgentListPage` 와 동급, RD-5), `remote/audit` 의 `requireAdmin` 게이팅을 적용하지 않아야 한다(스케줄 관리는 운영 성격이므로 remote/audit 의 admin 게이트와 의도적으로 상이).

### Module 4 — Per-Agent Schedule View Page + Nav (전용 페이지 + 네비게이션)

- **REQ-04-01** (Ubiquitous): 시스템은 전용 풀페이지 라우트 `/schedules` 를 `AuthGuard` → `AppLayout` 하위 child route 로 등록해야 하며(react-router v7 `React.lazy`), 페이지는 `AgentListPage` 패턴을 따라야 한다. 페이지는 **전체 인증 사용자** 에게 접근 가능해야 한다(admin 전용 게이팅 없음 — RD-5).
- **REQ-04-02** (Ubiquitous): 사이드바 `NAV_ENTRIES` 에 스케줄 View 진입 항목 1건(i18n `labelKey: 'nav.schedules'`)을 추가해야 한다.
- **REQ-04-03** (Event-Driven): When Schedule View 가 로드될 때, the View shall **프런트 `useQueries` 팬아웃**(flow 순회 → `getFlow`/getFlowNodes)으로 모든 트리거 스케줄을 크로스-플로우로 수집하고 **대상 에이전트별로 그룹핑** 하여 표시해야 한다(RD-10; v1 백엔드 집계 엔드포인트 없음, 집계 방식 §4.5).
- **REQ-04-04** (Ubiquitous): 에이전트별 그룹핑은 스케줄의 **명시 `agent_id` 필드**(REQ-01-01)를 1차 기준으로 사용해야 하며, 엣지 순회에 의존하지 않아야 한다.
- **REQ-04-05** (Event-Driven): When 사용자가 View 에서 스케줄을 생성/수정/삭제/토글할 때, the View shall `FacilityRuleModal` + `facilityScheduleUtils` + 추출된 공유 dual-write 훅(Module 6)을 재사용하여 LIVE + PERSIST dual-write 를 수행해야 한다.
- **REQ-04-06** (State-Driven): While 미지정 에이전트(`agent_id` 부재) 스케줄이 존재하는 동안, the View shall 이를 "미지정" 버킷으로 그룹핑하여 표시해야 한다(누락 없이).
- **REQ-04-07** (Optional): v1 은 프런트 `useQueries` 팬아웃을 기본으로 하며 백엔드 집계 엔드포인트를 두지 않는다(RD-10). Where 후속으로 플로우 규모가 커져 백엔드 집계 엔드포인트가 제공되면, the View shall 팬아웃 대신 이를 사용할 수 있어야 한다(후속 범위).

### Module 5 — Schedule-Log View UI (실행 로그 뷰)

- **REQ-05-01** (Ubiquitous): Schedule View 는 스케줄 실행 로그 뷰 영역(탭 또는 패널)을 포함해야 한다.
- **REQ-05-02** (Event-Driven): When 로그 뷰가 표시될 때, the 뷰 shall 로그 조회 API(Module 3)를 호출하여 발화(schedule_id/rule_name/agent/trigger_time)와 제어 결과(대상별 ok/error)를 상관된 형태로 표시해야 한다.
- **REQ-05-03** (Event-Driven): When 사용자가 `schedule_id` / `rule_name` / `agent_id` 필터를 지정할 때, the 뷰 shall 필터를 API 쿼리 파라미터로 전달하여 결과를 좁혀야 한다.
- **REQ-05-04** (Ubiquitous): 로그 뷰는 페이지네이션(limit/offset)과 최신순 정렬을 지원해야 한다.
- **REQ-05-05** (State-Driven): While 로그가 비어 있는 동안, the 뷰 shall 빈 상태(empty state)를 표시해야 한다(오류로 처리하지 않음).

### Module 6 — Dual-Write Hook Extraction (dual-write 훅 추출)

- **REQ-06-01** (Ubiquitous): `FacilitySchedulePanel.persist`(:133-179)의 dual-write 오케스트레이션(LIVE configureNode → PERSIST patch-then-PUT + `detectConflict` last-write-wins + 알림)을 공유 훅 `useScheduleDualWrite`(가칭)로 추출해야 한다.
- **REQ-06-02** (Ubiquitous): 추출된 훅은 대시보드 패널과 Schedule View **양쪽에서 재사용** 되어야 한다.
- **REQ-06-03** (State-Driven): While 대상 노드가 미실행(404)인 동안, the 훅 shall persist-only 로 폴백하여 저장만 적용하고 재배포 시 반영됨을 알려야 한다(기존 동작 보존).
- **REQ-06-04** (Unwanted): If 훅 추출로 인해 대시보드 패널의 기존 저장 동작이 달라지면, then the 변경 shall 거부되어야 한다(패널 동작 무회귀 — 순수 리팩터).

### Module 7 — Non-Functional Requirements (비기능)

- **REQ-07-01** (Unwanted): If 본 SPEC 의 어떤 변경이 트리거 노드 발화 로직 또는 설비 제어 예약 대시보드 패널의 UI/동작을 회귀시키면, then the 변경 shall 거부되어야 한다(무회귀 — RD-4 병존).
- **REQ-07-02** (Ubiquitous): 신설 로그 저장소·API·프런트 변경은 프로젝트 품질 게이트(TRUST 5, 신규 코드 커버리지 85%+, `-race` 클린, `tsc`/`vitest` 클린)를 통과해야 한다.
- **REQ-07-03** (Ubiquitous): 모든 타임스탬프는 epoch milliseconds(int64 `UnixMilli`)를 사용해야 한다(프로젝트 규약).
- **REQ-07-04** (State-Driven): While 로그 상관 기록이 best-effort 인 동안, the 제어 경로 shall 로그 기록 지연/실패로 제어 지연 또는 실패를 유발하지 않아야 한다.
- **REQ-07-05** (Ubiquitous): 크로스-플로우 집계는 로딩 상태·오류 상태를 명시적으로 처리해야 한다(부분 실패 시 가용 데이터 표시).

---

## 4. Specifications (사양)

### 4.1 스케줄 실행 로그 저장소 스키마 (RD-1, REQ-02-01~03)

`remote_audit_*` 를 미러한 신설 저장소(제안 파일: `internal/storage/schedule_log_repository.go` + `schedule_log_sqlite.go`). 레코드 구조(RD-6/7/8 반영):

| 필드                | 타입      | 의미                                                                                   |
| ----------------- | ------- | ------------------------------------------------------------------------------------- |
| `ID`              | int64   | 자동증가 PK(조회 시 채워짐)                                                                  |
| `CorrelationID`   | string  | **조인 키**(RD-8) = `schedule_id` + `:` + `trigger_time`. fire↔result 이벤트 결합           |
| `RecordKind`      | string  | `fire` \| `result`(RD-8). fire-only 는 fire 만, 제어 도달 발화는 fire+result 조인            |
| `ScheduleID`      | string  | 발화 스케줄 식별자(`trigger.schedule_id`)                                                  |
| `RuleName`        | string  | 규칙 표시 이름(`trigger.rule_name`, 없으면 빈 값)                                             |
| `DeclaredAgentID` | string  | **선언** 대상 에이전트(스케줄 `config.schedules[].agent_id`, RD-9/RD-6 보조)                   |
| `ActorAgentID`    | string  | **실제 실행** 에이전트(auditActor/`AgentRef` — RD-6 권위; fire 이벤트에는 빈 값)                 |
| `TriggerTime`     | int64   | 발화 시각(epoch ms, `trigger.trigger_time` 유래)                                          |
| `Target`          | string  | 셀렉터 종류·값(device_id/group_id: 예 `group_id=station:0150`; result 이벤트)                |
| `Action`          | string  | 제어 명령(set_power/set_fan_speed/set_multiple 등; result 이벤트)                          |
| `Result`          | string  | 집계 결과 `ok` \| `error`(result 이벤트; fire 이벤트에는 빈 값)                                 |
| `Targets`         | string  | **대상별 ok/error 임베드**(RD-7, JSON 배열 `[{target,result,reason}]`; fan-out 도 1 레코드)   |
| `Reason`          | string  | 비밀 아님 사유/오류 분류(예 `members=[..] ok=3/4`)                                            |
| `Timestamp`       | int64   | 기록 시각(epoch ms)                                                                     |

- 인터페이스: `Append(ctx, rec)` (추가 전용 — fire 이벤트·result 이벤트 각각 append) + `List(ctx, filter, limit, offset)` (최신순) + `Close()`. **갱신/삭제 API 없음**(REQ-02-03) → fire→result 는 in-place 갱신이 아니라 `correlation_id` 동일한 별도 append 이며 조회 시 조인한다(REQ-02-10).
- 필터: `schedule_id` / `rule_name` / `agent_id`(선언·실제 어느 쪽이든 매칭; 빈 값=전체). SQLite 인덱스는 `(correlation_id)`, `(schedule_id)`, `(actor_agent_id)`, `(declared_agent_id)`, `(timestamp desc)` 제안.
- 보존: v1 **무제한 append-only**(pruning 없음 — RD-11). 보존/정리 정책은 후속.
- 배선: `main.go` startup 단일 주입(싱글턴), `xsfm.SetAuditRepository` 패턴 준용(별도 setter `xsfm.SetScheduleLogRepository` — result 측 + `SetScheduleFireObserver` — fire 측 관측자).

### 4.2 발화↔제어결과 상관 메커니즘 + 반송 경로 (RD-2, REQ-02-04~06)

**현재 단절 지점**: `buildXsfmControlCommand`(xsfm.go:936)는 `msg.Payload()` 만으로 명령을 구성하고 `msg.Metadata()` 의 트리거 상관 메타를 버린다.

**2-이벤트 모델(RD-8)**: 발화당 하나의 논리 레코드는 (a) 발화 시점에 기록되는 **fire 이벤트** 와 (b) 제어 도달 시 기록되는 **result 이벤트** 로 구성되며, 동일한 `correlation_id` 로 조인된다. append-only + 갱신 API 부재(REQ-02-03) 규약상 fire 를 in-place 로 갱신하지 않고 result 를 **동일 `correlation_id` 로 별도 append** 하며, 조회 시 조인하여 한 행으로 표현한다(REQ-02-10). fire-only(하류 제어 없음)는 fire 이벤트만 남는다.

**상관 조인 키(RD-8)**: `correlation_id = schedule_id + ":" + trigger_time`(epoch ms). 스케줄은 반복 발화하므로 `schedule_id` 단독으로는 발화별 고유성이 없어 `trigger_time` 을 결합한다(`tick_count` 는 보조 디스크리미네이터). 트리거가 `trigger.trigger_time`(trigger.go:949)/`trigger.tick_count`(:948)를 이미 방출하므로 fire·result 양측이 동일 키를 산출한다.

**(A) fire-side 기록 경로 — 제네릭 트리거 노드 비결합(RD-8)**:

1. **발화(trigger)**: `buildMessage`(trigger.go:930-965)가 이미 `trigger.schedule_id`/`trigger.rule_name`/`trigger.priority`/`trigger.trigger_time` 를 메타로 방출. REQ-01-03 으로 `trigger.agent_id`(선언) 추가.
2. **주입 관측자 호출**: 트리거 발화 지점에서 **주입된 `ScheduleFireObserver`**(패키지 싱글턴, `SetAuditRepository` 패턴 미러, 미주입 시 no-op)를 best-effort 로 호출하여 발화 상관 컨텍스트(`schedule_id`/`rule_name`/`declared_agent_id`/`trigger_time` → `correlation_id`)를 전달한다. 제네릭 트리거 노드는 `ScheduleLogRepository` 타입을 알지 못하며 오직 제네릭 관측자 인터페이스만 안다(REQ-02-11).
3. **fire 이벤트 append**: 주입된 관측자 구현(facility-schedule / schedule-log 측)이 `RecordKind=fire` 레코드를 `ScheduleLogRepository.Append` 로 기록한다(result 필드 없음). best-effort(REQ-02-07).

**(B) result-side 기록 경로 — 제어 결과 반송**:

1. **와이어 통과**: 메시지 메타가 trigger → xsfm-control 경로에서 보존됨(A-2).
2. **명령 인코딩**: `buildXsfmControlCommand`(xsfm.go:936)가 `msg.Metadata()` 에서 `trigger.schedule_id`/`rule_name`/`agent_id`/`trigger_time`/`priority` 를 읽어 명령 envelope 에 상관 블록(예 `cmd["_correlation"] = {schedule_id, rule_name, declared_agent_id, trigger_time}`)으로 실음. 상관 메타가 없으면 블록을 생략(REQ-02-06 무회귀).
3. **에이전트 실행 + 기록**: 에이전트 제어 경로가 명령을 실행하고, 상관 블록이 존재하면 `recordControlAudit`/`recordGroupAudit` 인접에서 신설 `recordScheduleLog`(가칭)를 호출해 동일 `correlation_id` 의 `RecordKind=result` 이벤트를 append 한다. `ActorAgentID` = 실제 실행 에이전트(auditActor/`AgentRef`, RD-6 권위), `DeclaredAgentID` = 상관 블록의 선언 agent(RD-6 병기). fan-out 은 **집계 1건 + 대상별 `Targets` 임베드**(RD-7), 대상별 N 건을 남기지 않는다.

**결선 근거(왜 fire 는 관측자, result 는 제어 경로인가)**: fire-only(pure trigger / xsfm-control 미연결)는 하류 노드가 없어 하류 컬렉터 노드로는 포착 불가하다. 따라서 fire 포착은 반드시 발화 지점(주입 관측자)이어야 한다. 반면 result 는 제어 경로만이 발화 상관(통과 보유) + 실제 실행 에이전트 + 결과를 동시에 보유하므로 제어 경로가 기록한다. 하류 컬렉터 노드 방식(대안)은 fire-only 를 놓치므로 채택하지 않는다.

### 4.3 스케줄 대상 에이전트 필드 (RD-3, REQ-01-01~05)

- 위치: **스케줄 단위** `config.schedules[].agent_id`(트리거 노드 레벨 아님 — 스케줄마다 다른 에이전트 허용). **확정**(RD-9, RD-3 정합).
- 트리거 노드: `TriggerSchedule.AgentID string`(config `agent_id`) 추가, `registerSchedules` pass-through, `buildMessage` 에서 `trigger.agent_id` 방출(REQ-01-03).
- 프런트: 대시보드 패널의 패널-로컬 `config.agentId` 를 각 스케줄 `agent_id` 로 승격 기록(REQ-01-05). `FacilityRuleModal` 에 `agentId` 입력 반영.

### 4.4 페이지 레이아웃 + 라우팅 + 네비게이션 (RD-4, REQ-04-01~02)

- 라우트: `router.tsx` 에 `const SchedulesPage = lazy(() => import('@/pages/schedules/SchedulesPage'))` + `AppLayout` children 에 `{ path: '/schedules', element: <SuspenseWrapper><SchedulesPage/></SuspenseWrapper> }`. **admin 전용 추가 AuthGuard 없음** — 전체 인증 사용자 접근(RD-5, AgentListPage 동급).
- nav: `Sidebar.tsx` `NAV_ENTRIES` 에 `{ labelKey: 'nav.schedules', path: '/schedules', icon: <적절한 lucide 아이콘, 예 CalendarClock> }` + i18n 리소스 `nav.schedules` 키 추가. `roles: ['admin']` 게이팅을 두지 않는다(RD-5).
- 페이지 레이아웃(제안): 상단 탭 2개 — (1) **스케줄 관리**(에이전트별 그룹 섹션 + 각 그룹 규칙 테이블 + CRUD 모달), (2) **실행 로그**(필터 바 + 로그 테이블 + 페이지네이션).
- 대시보드 패널은 불변 유지(RD-4 병존).

### 4.5 크로스-플로우 집계 (REQ-04-03~04, RD-10)

- 확정 방식(RD-10): `useFlows` → 각 flow 에 대해 `useQueries` 팬아웃으로 `getFlow`/getFlowNodes 조회 → `config.schedules` 수집 → `agent_id` 기준 그룹핑. `useNodeTypeInstances('trigger')` 는 RUNNING 플로우만 보고 config 를 주지 않으므로 사용하지 않음. **v1 백엔드 집계 엔드포인트 없음.**
- 후속(스케일 시): 플로우 수가 매우 커지면 백엔드 집계 엔드포인트(`GET /schedules` 전 트리거 스케줄 통합)로 승급 가능(REQ-04-07 — 후속 범위, future 로 플래그).

### 4.6 dual-write 훅 (REQ-06-01~04)

- 추출 대상: LIVE `configureNode(flowId,nodeId,fullConfig)` → 404 시 persist-only → PERSIST `getFlow`+`detectConflict`+`patchNodeConfigInDefinition`+`updateFlow` → baseline 갱신 + 알림.
- 시그니처(제안): `useScheduleDualWrite({ flowId, nodeId }) => { persist(nextSchedules): Promise<void>, saving }`. `buildFullTriggerConfig`/`patchNodeConfigInDefinition`/`detectConflict`(triggerPanelUtils) 재사용.

---

## 5. Resolved Decisions (확정된 설계 결정)

> 사용자 확정. 본 절은 구현이 반드시 따르는 결정이다. 세부 파라미터 중 열린 항목은 §6 OQ 로 분리.

- **RD-1 — 전용 로그 저장소 신설**: 스케줄 실행 로그는 `remote_audit` 를 오버로딩하지 않고 **신설 append-only SQLite 저장소** + 전용 조회 API 로 관리한다(`internal/storage/remote_audit_*` 패턴 미러). (REQ-02-01, REQ-03-01)
- **RD-2 — 로그 내용 = 발화 + 제어결과 상관**: 로그는 트리거 발화(schedule_id/rule_name/agent/trigger_time)와 그에 상관된 제어 결과(대상별 ok/error)를 함께 기록한다. 상관은 `correlation_id`(`schedule_id`+`trigger_time`, RD-8)로 조인하며, 상관 메타를 통과 보유한 **xsfm 제어 경로가 결과와 함께 result 이벤트를 기록**한다(§4.2). 발화-only 는 RD-8 로 fire 이벤트로 기록된다. (REQ-02-04~06)
- **RD-3 — 스케줄에 agent 명시 저장**: 대상 에이전트를 **스케줄 단위**로 명시 영속(`config.schedules[].agent_id`)한다. 대시보드 패널과 View 가 모두 이를 기록하며, 에이전트별 그룹핑의 1차 기준이자 로그 `declared_agent_id` 의 공급원이 된다. (REQ-01-01~05)
  > **[0.3.0 수정 — §9 AM-0.3.0-2 / AM-0.3.0-3]** 명시 `agent_id`(→ `declared_agent_id`) 저장·로깅·표시는 유지. 단 (a) 관리 뷰의 **에이전트별 그룹핑**은 **단일 플랫 테이블 + 에이전트(이름) 컬럼**으로 대체(RD-7 항의 그룹핑도 함께 수정), (b) create 플로우에서 명시 필드를 **하류 엣지 순회 자동 도출**(`deriveDownstreamAgents`, 단일-노드)로 채우도록 보강(RD-9 항의 "엣지 순회 배제"를 create 범위에서 완화). 원 텍스트는 보존; 상세는 §9.
- **RD-4 — 전용 페이지 + 패널 병존**: 신설 풀페이지 `/schedules`(에이전트별 CRUD + 로그 뷰) + 사이드바 nav 를 추가하고, 기존 설비 제어 예약 **대시보드 패널은 불변으로 병존**한다. (REQ-04-01~02, REQ-07-01)
- **RD-5 — RBAC: 전체 인증 사용자**: `/schedules` View 페이지와 스케줄 로그 조회 API 는 **전체 인증 사용자** 에게 접근 가능하다(admin 전용 아님 — `AgentListPage` 와 동급). 이는 스케줄 관리가 운영 성격이기 때문이며, remote/audit 의 admin 게이트와는 **의도적으로 상이**하다. (REQ-03-05, REQ-04-01, OQ-5 확정)
- **RD-6 — 로그 agent 권위 = 실제 실행 + 선언 병기**: 로그는 **실제 실행 에이전트**(auditActor / `XSFMNodeConfig.AgentRef`)를 `actor_agent_id` 권위 값으로 기록하고, 스케줄의 **선언 `agent_id`**(RD-9)를 `declared_agent_id` 로 병기한다. 두 필드를 모두 레코드에 두어 선언↔실제 불일치가 조회 시 가시화된다. (REQ-02-02, REQ-02-04c, OQ-2 확정)
- **RD-7 — 로그 입도 = 집계 1건 + 대상별 임베드**: 하나의 발화(셀렉터 fan-out 포함)는 발화당 **집계 result 이벤트 1건** 으로 기록하며, 대상별 ok/error 를 그 레코드의 `targets` 에 **임베드**한다(recordGroupAudit 집계 패턴 미러). 대상별 N 건을 별도 레코드로 남기지 않는다. (REQ-02-04b, OQ-1 확정)
  > **[0.3.0 수정 — §9 AM-0.3.0-2]** 로그 **입도(집계 1건 + 임베드)** 자체는 불변. 다만 이 RD 가 함께 규정하던 **관리 뷰의 "에이전트별 그룹" 표시**(AC-7)는 **단일 플랫 테이블**로 대체되었다. 상세는 §9.
- **RD-8 — 발화-only 도 기록 + fire↔result 상관**: 하류 제어가 없는(트리거가 xsfm-control 에 미연결 또는 순수 트리거) 발화도 **fire-only 레코드**(result 없음)로 기록한다. 발화 시점에 **fire 이벤트** 를 기록하고, 제어 경로가 동일 `correlation_id`(`schedule_id`+`trigger_time`)로 **result 이벤트** 를 append 하여 조회 시 조인한다(제어 도달 발화 = fire+result, fire-only = fire). **비결합**: fire 이벤트는 제네릭 트리거 노드에 스케줄-로그 저장소를 하드코딩하지 않고 **주입된 `ScheduleFireObserver`**(패키지 싱글턴, 미주입 시 no-op)가 기록하며, 저장소 결합은 facility-schedule 측 관측자 구현에 둔다. 하류 컬렉터 노드 방식은 fire-only 를 놓치므로 채택하지 않는다(§4.2). (REQ-02-09~11, OQ-6 확정)
- **RD-9 — agent 필드 배치 = 스케줄별**: agent 연관은 **스케줄별**(`config.schedules[].agent_id`)에 둔다(트리거-노드 레벨 단일 필드 아님). RD-3 와 정합. (REQ-01-01, OQ-3 확정)
  > **[0.3.0 수정 — §9 AM-0.3.0-3]** 스케줄별 명시 `agent_id` 저장은 불변. 다만 이 RD 가 함께 규정하던 **"엣지 순회 배제"**(그룹핑을 엣지가 아닌 명시 필드로)는 **create 플로우에 한해 완화** — 트리거 노드 선택 시 하류 엣지를 제어 노드 `agent_ref` 까지 **단일-노드 순회**하여 명시 필드를 자동 도출(`deriveDownstreamAgents`)한다(취약성 우려는 대규모 크로스-플로우 그룹핑에 국한). 명시 `declared_agent_id` 필드는 저장/로깅/표시용으로 유지. 상세는 §9.
- **RD-10 — 집계 = 프런트 팬아웃**: 크로스-플로우 스케줄 집계는 **프런트 `useQueries` 팬아웃**(flow 순회 → getFlowNodes/getFlow)으로 수행하며, **v1 백엔드 집계 엔드포인트를 두지 않는다**. 스케일이 커지면 백엔드 집계로 승급(후속 플래그). (REQ-04-03, REQ-04-07, OQ-4 확정)
- **RD-11 — 보존 = 무제한 append-only**: v1 로그 저장소는 **무제한 append-only**(pruning 없음)이며, 보존 한도/정리 정책은 **후속 SPEC 으로 이연**한다. (REQ-02-12, OQ-7 확정)
  > **[0.3.0 수정 — §9 AM-0.3.0-1]** 무제한 append-only 및 **개별 레코드 삭제 API 부재**는 유지. 다만 운영상 필요로 **수동 "전체 초기화(Clear)" 예외**를 허용(저장소 인터페이스 `Clear(ctx)` = 전 로그 삭제, `Count(ctx,filter)` = 페이지네이션 total 추가). 개별 삭제/보존-기반 pruning 은 여전히 부재. 상세는 §9.

---

## 6. Open Questions (열린 질문)

> **잔여 없음.** 초기 SPEC 의 OQ-1~7 은 모두 사용자 확정되어 §5 의 RD-5~11 로 승격되었다(v0.2.0). 추적 매핑: OQ-1→RD-7, OQ-2→RD-6, OQ-3→RD-9, OQ-4→RD-10, OQ-5→RD-5, OQ-6→RD-8, OQ-7→RD-11. 후속 범위(scale 시 백엔드 집계 승급 — RD-10, 보존/pruning 정책 — RD-11)는 별도 SPEC 으로 이연한다.

---

## 7. Traceability (추적성)

- 상위 의존: SPEC-TRIGGER-SCHED-001(스케줄 규칙/패널/발화 게이팅·상관 메타), SPEC-FACILITY-DASHBOARD-001(설비 패널·집계), SPEC-XSFM-GROUP-001(노드 제어 명령 셋·셀렉터), SPEC-XSFM-001(제어 감사), SPEC-REMOTE-001(append-only 감사 저장소·admin-gated 조회 API 패턴).
- 모듈 ↔ REQ: Module 1 = REQ-01-01~05 / Module 2 = REQ-02-01~12(신설 04b/04c/09/10/11/12 포함) / Module 3 = REQ-03-01~05 / Module 4 = REQ-04-01~07 / Module 5 = REQ-05-01~05 / Module 6 = REQ-06-01~04 / Module 7 = REQ-07-01~05.
- 모듈 ↔ REQ (0.3.0, §8): Module 8 = REQ-08-01~04 / Module 9 = REQ-09-01~03 / Module 10 = REQ-10-01~03 / Module 11 = REQ-11-01 / Module 12 = REQ-12-01~02 / Module 13 = REQ-13-01~03. 버그수정 FIX-1~3(§8.0).
- RD ↔ OQ: RD-5←OQ-5(RBAC) / RD-6←OQ-2(agent 권위) / RD-7←OQ-1(로그 입도) / RD-8←OQ-6(발화-only) / RD-9←OQ-3(agent 필드 배치) / RD-10←OQ-4(집계) / RD-11←OQ-7(보존).
- Amendments(§9): AM-0.3.0-1 = RD-11(전체 초기화 Clear) / AM-0.3.0-2 = RD-3·RD-7(그룹→플랫 테이블) / AM-0.3.0-3 = RD-3·RD-9(create 하류 엣지 순회 agent 자동 도출).
- 상세 설계: `design.md`. 조사 근거: `research.md`. 인수 기준: `acceptance.md`. 구현 계획: `plan.md`.

---

## 8. 0.3.0 확장 요구사항 (as-implemented, M8~M13)

> **as-implemented 기록.** M1~M7 구현 완료 후 실제 구축된 확장(B~G)과 M5/M6 버그수정(A)이다. REQ 는 기존 넘버링 체계(Module N)를 연장한다. AMENDS 표기 항목의 상세 사유는 §9.

### 8.0 M5/M6 버그수정 (A — as-fixed)

- **FIX-1 — 규칙 추가 모달의 TARGET 열거를 xsfm 에이전트 경유로**: 규칙 추가 모달이 **선택된 xsfm 에이전트**를 통해 TARGET(대상 device/group)을 열거하도록 수정했다(원래 미지정 노드는 대상이 표시되지 않았다). `FacilityRuleModal` 에 **에이전트 선택**을 가산했다(additive). 대시보드 패널의 기존 동작은 불변(REQ-07-01).
- **FIX-2 — 스케줄 0개 트리거 노드 숨김**: 스케줄이 하나도 없는 트리거 노드는 관리 뷰에서 **숨긴다**(원래 자동으로 표시되었다).
- **FIX-3 — 삭제-영속 정상 확인(버그 아님)**: 스케줄 삭제의 백엔드 왕복 영속은 **정상(일관)**으로 검증되었다. "flow editor 에 규칙이 남아 보인다"는 현상은 **크로스-서피스 에디터-상태 staleness**(별도 에디터 화면의 캐시 미갱신)이며 영속 버그가 아니다 — `required:true` 강제나 기본 스케줄 주입은 도입하지 않았다. (문서 기록용 결론)

### Module 8 — Schedule-Log Storage Backends + Retention Amendment (B, RD-11 수정)

- **REQ-08-01** (Ubiquitous): 스케줄 로그 저장소는 **팩토리**를 통해 3개 백엔드를 지원해야 한다 — **sqlite**(기본, 영속) / **memory**(비영속) / **JSONL 파일**(append-only 파일). (신설 파일: `internal/storage/schedule_log_memory.go`, `internal/storage/schedule_log_jsonl.go`)
- **REQ-08-02** (Ubiquitous): 백엔드는 config 키 `storage.schedule_log.type`(기본 `sqlite`)로 선택되며 startup 시 적용되어야 한다. (`internal/config` `StorageConfig.ScheduleLogType`)
- **REQ-08-03** (Ubiquitous): 저장소 인터페이스는 `Clear(ctx)`(전체 로그 삭제 — 수동 전체 초기화)와 `Count(ctx, filter)`(페이지네이션 total 산출)를 제공해야 한다.
- **REQ-08-04** (Unwanted): If **개별 레코드 삭제 API** 가 추가되면, then the 변경 shall 거부되어야 한다. **AMENDS RD-11**(§9 AM-0.3.0-1): 무제한 append-only + 개별 삭제 부재는 유지하되, 수동 **전체 초기화(Clear)** 예외만 허용한다(보존-기반 pruning 은 여전히 후속).

### Module 9 — Log Query API Changes (C)

- **REQ-09-01** (Ubiquitous): `GET /schedules/logs` 응답 형태는 페이지네이션을 지원하는 `{ items: [...], total: N }` 여야 한다(기존 bare array 에서 변경).
- **REQ-09-02** (Event-Driven): When 사용자가 전체 초기화를 요청할 때, the API shall `DELETE /schedules/logs`(전체 인증 사용자 — admin 아님, RD-5)로 모든 로그를 삭제해야 한다.
- **REQ-09-03** (Ubiquitous): 시스템은 `GET/PUT /system/schedule-log-config`(**admin 전용**)를 제공하여 `storage.schedule_log.type` 를 읽기/설정하고 **needs_restart 신호**를 반환해야 한다. (신설 핸들러 `internal/api/handler/schedule_log_config.go`; `internal/config/overrides.go` allowlist 에 해당 키 확장)

### Module 10 — Log Tab UX (D)

- **REQ-10-01** (Ubiquitous): 로그 탭은 페이지네이션(page size **25(기본)/50/100** + prev/next + total 표시)을 제공해야 한다.
- **REQ-10-02** (Event-Driven): When 사용자가 **CSV 내보내기**(내보내기)를 실행할 때, the 뷰 shall 필터 매칭 **전체 행**을 **UTF-8 BOM** 으로, 컬럼 **실행시각 / 규칙이름 / 에이전트(선언, 실제 다르면 병기) / 대상 / 동작 / 결과** 로 내보내야 한다.
- **REQ-10-03** (Event-Driven): When 사용자가 **전체 초기화**(초기화)를 실행할 때, the 뷰 shall **confirm 다이얼로그**로 가드된 파괴적 동작으로 `DELETE /schedules/logs` 를 호출해야 한다.

### Module 11 — Storage-Type Settings UI (E)

- **REQ-11-01** (Ubiquitous): 시스템은 **admin 전용 Settings 카드**("스케줄 로그 저장 방식")를 제공하여 sqlite / 파일(JSONL) / 메모리 중 선택하고, config 엔드포인트(REQ-09-03)로 영속하며, **재시작 시 적용**되어야 한다.

### Module 12 — TARGET Table Picker (F)

- **REQ-12-01** (Ubiquitous): 규칙 모달의 device-target picker 는 **테이블**(컬럼 **이름 / 라인 / 역사 / 위치** — device 를 station line + station display_name + place display_name 과 조인) + **검색 필터** + **행 선택** 형태여야 한다.
- **REQ-12-02** (Ubiquitous): 이 테이블 picker 는 **Schedule View 와 대시보드 Trigger 패널 양쪽**(공유 모달)에 적용되어야 하며, 저장되는 `TargetSpec` shape 은 불변이어야 한다.

### Module 13 — Management Tab Redesign + Agent Model (G, RD-3/RD-7 · RD-3/RD-9 수정)

- **REQ-13-01** (Ubiquitous): 관리 탭은 에이전트별 그룹 섹션이 아니라 **단일 플랫 테이블**(**에이전트(이름)** 컬럼 + **플로우/노드** 컬럼)로 렌더링해야 한다. React Rules-of-Hooks 는 **per-node `<tbody>` 컴포넌트**로 보존한다. **AMENDS RD-3/RD-7**(§9 AM-0.3.0-2): 원래의 "에이전트별 그룹"(AC-7) → 플랫 테이블 + 에이전트 컬럼.
- **REQ-13-02** (Event-Driven): When 사용자가 "규칙 추가"를 클릭할 때, the View shall 별도 "대상 노드" 생성 카드 없이 **설정 모달을 직접 열고**, **대상 노드를 모달 내부 첫 필드**로 선택한 뒤 에이전트 + 대상 + 계획(plan) + 동작(action)을 함께 입력받아야 한다. dual-write persist 는 저장 시점 노드를 타깃팅할 수 있도록 독립 함수 `persistScheduleDualWrite` 로 추출한다.
- **REQ-13-03** (Event-Driven): When (생성 시) 트리거 노드가 선택될 때, the View shall 플로우의 **하류 엣지를 제어 노드 `agent_ref` 까지 순회**(`deriveDownstreamAgents`)하여 에이전트를 **자동 도출**해야 한다 — 단일 → 자동 채움, 미배선/모호한 fan-out → 모달 내 수동 선택 폴백. **AMENDS RD-3/RD-9**(§9 AM-0.3.0-3): 원래의 "엣지 순회 배제"를 create 플로우의 **단일-노드 하류 순회**로 완화한다. 명시 `declared_agent_id` 필드는 저장/로깅/표시용으로 유지한다.

---

## 9. 0.3.0 설계 수정 (Amendments)

> **승계 노트.** 아래 3건은 §5 의 원 RD(RD-3/RD-7/RD-9/RD-11)를 **덮어쓰지 않고** 부분 수정한 것이다. 각 항목은 (a) 원 결정, (b) 변경 내용, (c) 사유를 기록하여 이력이 추적 가능하게 한다. 원 RD 텍스트는 §5 에 그대로 남고, 각 RD 말미에 본 §9 로의 승계 마커를 달았다.

### AM-0.3.0-1 — RD-11 수정: 전체 초기화(Clear) 예외 허용

- **원 결정(RD-11)**: v1 로그 저장소는 **무제한 append-only**(pruning 없음)이며, **삭제 API 를 노출하지 않는다**(감사 무결성).
- **변경**: **개별 레코드 삭제 API 는 여전히 부재**하나, 운영상 필요로 **수동 "전체 초기화(Clear)" 예외**를 허용한다. 저장소 인터페이스에 `Clear(ctx)`(전 로그 삭제)와 `Count(ctx, filter)`(페이지네이션 total)를 추가했다. (REQ-08-03/04, REQ-09-02, REQ-10-03)
- **사유**: 운영 중 로그 전체 리셋 수요 + 페이지네이션 total 산출 필요. 개별-레코드 불변성(감사성)은 유지하면서 "전량 초기화"라는 명시적·일괄 파괴 동작만 예외로 허용해 무결성 우려를 최소화한다. 보존-기반 자동 pruning 은 여전히 후속 SPEC.

### AM-0.3.0-2 — RD-3/RD-7 수정: 에이전트별 그룹 → 단일 플랫 테이블

- **원 결정(RD-3/RD-7)**: 관리 뷰는 스케줄을 **대상 에이전트별로 그룹핑**하여 에이전트 섹션별 규칙 테이블로 표시한다(AC-7 grouping).
- **변경**: 관리 탭을 **단일 플랫 테이블**로 개편하고, 그룹 헤더 대신 **에이전트(이름) 컬럼**(+ 플로우/노드 컬럼)을 둔다. React Rules-of-Hooks 는 **per-node `<tbody>` 컴포넌트**로 보존한다. (REQ-13-01; AC-7 은 acceptance.md 에서 플랫 테이블로 갱신)
- **사유**: 다수 에이전트/노드에 걸친 스케줄을 한눈에 훑고 정렬·검색하기에 플랫 테이블이 우수하며, 그룹 섹션 반복 렌더링에서 발생하던 Hooks 순서 취약성을 per-node `<tbody>` 로 안정화. 스케줄별 `agent_id`(→ `declared_agent_id`) 저장·로깅·표시 자체는 불변(로그 입도 RD-7 집계 규약도 불변).

### AM-0.3.0-3 — RD-3/RD-9 수정: create 시 하류 엣지 순회 agent 자동 도출

- **원 결정(RD-3/RD-9)**: agent 는 **스케줄별 명시 필드**(`config.schedules[].agent_id`)이며, 그룹핑/도출은 **엣지 순회에 의존하지 않는다**(엣지 순회는 취약 — 런타임 의존·부정확).
- **변경**: **create(규칙 생성) 플로우에 한해** 트리거 노드 선택 시 플로우 하류 엣지를 제어 노드의 `agent_ref` 까지 **단일-노드 순회**하여 명시 필드를 **자동 도출**(`deriveDownstreamAgents`)한다. 단일 도출 → 자동 채움, 미배선/모호한 fan-out → 모달 내 수동 선택 폴백. 명시 `declared_agent_id` 필드는 **저장/로깅/표시용으로 유지**한다. (REQ-13-02/03)
- **사유**: 원래의 취약성 우려는 **대규모 크로스-플로우 그룹핑**(전체 스케줄을 엣지로 재구성)에 국한된 것이었고, **단일 create 시점의 단일-노드 하류 순회**는 범위가 좁아 안전하다. 사용자가 매번 에이전트를 수동 지정하는 부담을 줄이되, 도출 실패(미배선/모호)에는 명시적 수동 폴백을 두어 정확성을 보장한다. 저장·로깅·표시의 권위는 계속 명시 `declared_agent_id`(RD-6 병기 규약 불변).
