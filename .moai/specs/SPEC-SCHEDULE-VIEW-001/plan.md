---
id: SPEC-SCHEDULE-VIEW-001
title: "Schedule View — 구현 계획"
version: "0.3.0"
status: in-progress
created: 2026-07-31
updated: 2026-08-01
author: xtra
tier: L
---

# SPEC-SCHEDULE-VIEW-001 — 구현 계획 (plan.md)

> **버전 노트 (0.3.0)**: as-implemented 동기화. M1~M7(0.2.0) 구현 완료 후 도입된 확장을 **M8~M13**(§3.1) 으로 가산 기록하고 M5/M6 버그수정(FIX-1~3)을 명시한다. 3건의 설계 수정(RD-11 전체 초기화(Clear), RD-3/RD-7 그룹→플랫 테이블, RD-3/RD-9 create 하류 엣지 순회 agent 자동 도출)은 spec.md §9 에 승계 기록. M1~M7 텍스트는 불변 보존.
> **버전 노트 (0.2.0)**: OQ-1~7 확정(RD-5~11) 반영. M1 스키마에 `correlation_id`/`record_kind`/`declared_agent_id`/`actor_agent_id`/`targets` 추가(RD-6/7/8) + 무제한 append-only(RD-11), M2 를 fire-side 주입 관측자(RD-8) + result-side 제어 기록(RD-6/7)의 2-이벤트 경로로 확장, M3 RBAC 전체 인증(RD-5), M5 집계는 프런트 팬아웃(RD-10) + 스케줄별 agent 필드(RD-9). 리스크 표에서 확정 항목(OQ→RD) 갱신.
> 시간 예측 없음. 우선순위·마일스톤 순서로 표기. 각 마일스톤은 선행 완료 후 착수한다.
> 개발 방법론: Hybrid(신규 코드 TDD, 기존 코드 DDD ANALYZE-PRESERVE-IMPROVE).

## 1. 기술 접근 (technical approach)

- **백엔드-우선(backend-first)** 흐름: 로그 저장소 → 상관 반송/기록 → 조회 API → 프런트. 상관은 발화(트리거) 메타 방출은 이미 존재하므로, 반송 경로(명령 인코딩)와 기록 지점(에이전트)이 핵심 신규 작업이다.
- **비침습 가산(additive)**: 트리거 스케줄 `agent_id`, 로그 저장소, 로그 API, 전용 페이지는 모두 기존 경로에 가산된다. 기존 트리거 발화·설비 패널·remote_audit 는 불변(RD-4, REQ-07-01).
- **재사용 최대화**: `remote_audit_*`(저장소·API·배선 패턴), `facilityScheduleUtils`/`triggerPanelUtils`(프런트 순수 헬퍼), `FacilityRuleModal`(모달), `AgentListPage`(페이지 패턴).
- **dual-write 훅 추출은 순수 리팩터**: 패널 동작 무회귀(REQ-06-04) 특성화 테스트로 보증 후 추출.

## 2. 아키텍처 설계 방향

- 데이터 흐름(로그, 2-이벤트 RD-8): `TriggerNode.buildMessage`(schedule_id/rule_name/agent_id/trigger_time 메타) → **(A) fire-side**: 주입 `ScheduleFireObserver.OnScheduleFire` → `ScheduleLogRepository.Append{fire}`(비결합, 발화-only 포함); **(B) result-side**: 와이어 → `XsfmControlNode.Process` → `buildXsfmControlCommand`(메타→명령 상관 블록) → `XSFMAgent.Process`(실행) → `recordScheduleLog`(상관 + actor_agent_id RD-6 + 집계 result + targets 임베드 RD-7) → `ScheduleLogRepository.Append{result, 동일 correlation_id}`. 조회 시 `correlation_id` 로 조인.
- 데이터 흐름(조회): `ScheduleLogRepository.List(filter)` → `ScheduleLogHandler`(`GET /schedules/logs`) → 프런트 로그 훅 → 로그 뷰.
- 데이터 흐름(관리): `useFlows`+`useQueries` → config.schedules 수집 → agent_id 그룹핑 → `FacilityRuleModal` CRUD → `useScheduleDualWrite` → LIVE+PERSIST.

## 3. 마일스톤 (우선순위 순서)

### M1 — 스케줄 실행 로그 저장소 신설 (Priority High, RD-1 / Module 2 저장소 절)

- 목표: `internal/storage/schedule_log_repository.go`(인터페이스 + `ScheduleLogRecord` — `correlation_id`/`record_kind`/`declared_agent_id`/`actor_agent_id`/`targets` 포함, RD-6/7/8) + `schedule_log_sqlite.go`(`Append`/`List`/`Close`, 마이그레이션, 인덱스 `correlation_id`/`schedule_id`/`actor_agent_id`/`declared_agent_id`/`timestamp desc`).
- 방법론: TDD. `remote_audit_*` 를 참조 구현으로 미러. **무제한 append-only**(pruning 없음, RD-11), 갱신/삭제 API 없음.
- 산출: append-only 저장소, 필터(schedule_id/agent_id/rule_name — agent 는 선언·실제 매칭) + limit/offset + 최신순 List. fire/result 이벤트가 `correlation_id` 로 조인 가능함을 단위 테스트로 검증. 단위 테스트 + `-race`.

### M2 — 상관 반송 경로 + 로그 기록 (Priority High, RD-2 / Module 1·2)

- 선행: M1.
- 목표(백엔드):
  1. `TriggerSchedule.AgentID` 필드 + config 파싱(스케줄별, RD-9) + `buildMessage` 의 `trigger.agent_id`/`trigger.trigger_time` 방출(REQ-01-01~04). DDD(기존 trigger.go 특성화 테스트로 발화 무회귀 보증 후 가산).
  2. **fire-side(RD-8)**: `ScheduleFireObserver` 인터페이스 + 패키지 싱글턴 `SetScheduleFireObserver`(미주입 no-op). 트리거 발화 지점에서 best-effort 호출(제네릭 노드는 저장소 타입 비인지, REQ-02-11). facility-schedule 측 관측자 구현이 `RecordKind=fire` 레코드(`correlation_id`=schedule_id+trigger_time) append — **발화-only 포함**.
  3. **result-side**: `buildXsfmControlCommand` 가 `msg.Metadata()` 상관 메타를 명령 상관 블록(`declared_agent_id` 포함)으로 인코딩(REQ-02-05). 상관 메타 부재 시 무회귀(REQ-02-06).
  4. `XSFMAgent` 제어 경로에 `recordScheduleLog`(가칭) 추가 — 동일 `correlation_id` 의 `RecordKind=result` 이벤트로 기록. `actor_agent_id`=실제 실행 에이전트(RD-6 권위), `declared_agent_id` 병기(RD-6), fan-out 은 집계 1건 + `targets` 임베드(RD-7). best-effort no-op(REQ-02-07). `main.go` `SetScheduleLogRepository` + `SetScheduleFireObserver` 배선.
- 산출: 발화→제어 상관 로그 end-to-end. 테스트: (a) 발화+제어 → fire+result 가 동일 correlation_id 로 조인(actor+declared+targets), (b) 발화-only(제어 미연결) → fire 레코드 1건(result 없음), (c) 스케줄 상관 없는 수동 제어 → 로그 0건(무회귀), (d) fan-out → result 집계 1건 + 대상별 임베드.

### M3 — 로그 조회 API (Priority High, RD-1 / Module 3)

- 선행: M1.
- 목표: `internal/api/handler` 에 스케줄 로그 핸들러(`GET /schedules/logs`, `?schedule_id=&rule_name=&agent_id=&limit=&offset=`), `remote_admin.go` Audit 핸들러 패턴 미러하되 **`requireAdmin` 미적용 — 전체 인증 사용자**(RD-5). `main.go` RegisterRoutes 배선 + 미구성 시 빈 목록(REQ-03-03).
- 산출: API + 핸들러 테스트(필터/페이지네이션/미구성/전체 인증 접근).

### M4 — dual-write 훅 추출 (Priority Medium, RD-4 / Module 6)

- 선행: 없음(프런트 독립). 단, View(M5)가 소비.
- 목표: `FacilitySchedulePanel.persist` → `useScheduleDualWrite` 훅 추출, 패널이 훅 소비하도록 교체(순수 리팩터, REQ-06-04 무회귀). 패널 특성화 테스트(vitest)로 저장 동작 보존 확인.
- 산출: 공유 훅 + 패널 무회귀.

### M5 — 전용 페이지: 에이전트별 스케줄 관리 (Priority Medium, RD-3·4 / Module 4)

- 선행: M4(훅), M2(agent_id 필드).
- 목표: `web/src/pages/schedules/SchedulesPage.tsx`(AgentListPage 패턴), `router.tsx` 라우트(admin 게이팅 없음, RD-5) + `Sidebar.tsx` nav(roles 게이팅 없음, RD-5) + i18n `nav.schedules`. 크로스-플로우 집계(`useFlows`+`useQueries` 팬아웃, RD-10; v1 백엔드 집계 없음) → 스케줄별 `agent_id`(RD-9) 그룹핑(미지정 버킷 포함) → `FacilityRuleModal` CRUD(agentId 반영) + `useScheduleDualWrite`.
- 산출: 관리 탭. 컴포넌트 테스트.

### M6 — 전용 페이지: 실행 로그 뷰 (Priority Medium, Module 5)

- 선행: M3(API), M5(페이지 셸).
- 목표: 로그 뷰 탭 — 로그 조회 훅(`useScheduleLogs`, filter+pagination) + 필터 바(schedule_id/rule_name/agent_id) + 테이블(`correlation_id` 로 fire+result 병합 표시 — 발화 + 선언/실제 agent + 집계 result + 대상별 targets; fire-only 는 result 없이 표시) + 빈 상태.
- 산출: 로그 탭. 컴포넌트 테스트.

### M7 — 통합 검증 + 무회귀 (Priority High, Module 7)

- 선행: M1~M6.
- 목표: end-to-end 인수(acceptance.md) 검증, 트리거 노드·설비 패널 무회귀(REQ-07-01), 품질 게이트(커버리지/`-race`/`tsc`/`vitest`).

## 3.1 0.3.0 확장 마일스톤 (M8~M13, as-implemented)

> M1~M7 구현·테스트 green 종료 후 도입된 확장이다. **이미 구현·테스트 완료(green)** 상태의 as-implemented 기록이다. spec.md §8(Module 8~13) + §9(Amendments)에 대응한다.

### M5/M6 버그수정 (FIX-1~3, as-fixed)

- **FIX-1**: 규칙 추가 모달의 TARGET 열거를 **선택된 xsfm 에이전트 경유**로 수정(원래 미지정 노드는 대상 미표시). `FacilityRuleModal` 에 에이전트 선택 가산(대시보드 패널 불변). (§8.0)
- **FIX-2**: 스케줄 0개 트리거 노드를 관리 뷰에서 숨김.
- **FIX-3**: 삭제-영속은 정상(백엔드 왕복 일관) 확인 — "flow editor 잔존"은 크로스-서피스 에디터-상태 staleness 로 판명(영속 버그 아님, `required:true`/기본 스케줄 주입 미도입).

### M8 — 로그 저장소 3-백엔드 팩토리 + Clear/Count (Priority High, Module 8, RD-11 수정)

- 선행: M1.
- 목표(백엔드): 저장소를 팩토리화하여 **sqlite(기본)/memory/JSONL** 3-백엔드 지원(신설 `internal/storage/schedule_log_memory.go`, `schedule_log_jsonl.go`). config 키 `storage.schedule_log.type`(`internal/config` `StorageConfig.ScheduleLogType`, 기본 `sqlite`) startup 적용. 인터페이스에 `Clear(ctx)`(전 로그 삭제) + `Count(ctx, filter)`(total) 추가. **개별 삭제 API 는 미도입**(RD-11 amendment — spec.md §9 AM-0.3.0-1).
- 방법론: TDD(신규 백엔드/인터페이스 메서드). 3-백엔드 계약 동형 테스트.
- 산출: 3-백엔드 + Clear/Count + config 선택. 단위 테스트 + `-race`.

### M9 — 로그 조회 API 변경 + 삭제/설정 엔드포인트 (Priority High, Module 9)

- 선행: M8, M3.
- 목표: (1) `GET /schedules/logs` 응답을 `{ items, total }` 로 변경(페이지네이션). (2) `DELETE /schedules/logs`(전체 인증, admin 아님 — RD-5) → `Clear`. (3) 신설 `GET/PUT /system/schedule-log-config`(admin 전용, 신설 핸들러 `internal/api/handler/schedule_log_config.go`) → `storage.schedule_log.type` 읽기/설정 + needs_restart 신호. `internal/config/overrides.go` allowlist 에 키 확장.
- 산출: 변경 응답 + 삭제/설정 API + 핸들러 테스트.

### M10 — 로그 탭 UX: 페이지네이션·CSV·초기화 (Priority Medium, Module 10)

- 선행: M6, M9.
- 목표: 로그 탭에 (1) 페이지네이션(page size 25/50/100 + prev/next + total), (2) **CSV 내보내기**(필터 매칭 전체 행, UTF-8 BOM, 컬럼 실행시각/규칙이름/에이전트(선언, 실제 다르면 병기)/대상/동작/결과), (3) **전체 초기화**(confirm 다이얼로그 가드 → DELETE 엔드포인트) 추가.
- 산출: 로그 탭 UX. 컴포넌트 테스트.

### M11 — 저장방식 Settings 카드 (Priority Medium, Module 11)

- 선행: M9.
- 목표: **admin 전용** Settings 카드("스케줄 로그 저장 방식")로 sqlite/파일(JSONL)/메모리 선택 → config 엔드포인트(M9)로 영속 → 재시작 시 적용.
- 산출: Settings 카드. 컴포넌트 테스트.

### M12 — TARGET 테이블 picker (Priority Medium, Module 12)

- 선행: 없음(모달 확장). Schedule View + Trigger 패널 공유.
- 목표: 규칙 모달의 device-target picker 를 **테이블**(컬럼 이름/라인/역사/위치 — device ⋈ station line + station display_name + place display_name) + 검색 필터 + 행 선택으로 개편. **공유 모달**이므로 Schedule View 와 대시보드 Trigger 패널 양쪽 적용. 저장 `TargetSpec` shape 불변.
- 산출: 테이블 picker(공유). 컴포넌트 테스트.

### M13 — 관리 탭 개편 + agent 자동 도출 (Priority Medium, Module 13, RD-3/RD-7·RD-3/RD-9 수정)

- 선행: M5, M12.
- 목표: (1) 관리 탭을 **에이전트별 그룹 → 단일 플랫 테이블**(에이전트(이름) 컬럼 + 플로우/노드 컬럼)로 개편. Rules-of-Hooks 는 per-node `<tbody>` 컴포넌트로 보존. **AMENDS RD-3/RD-7**. (2) 규칙 생성을 **모달 내부화** — 별도 "대상 노드" 카드 제거, "규칙 추가"가 설정 모달을 직접 열고 대상 노드를 모달 첫 필드로 선택. dual-write persist 를 독립 `persistScheduleDualWrite` 로 추출(저장 시점 노드 타깃팅). (3) 트리거 노드 선택 시 하류 엣지를 제어 노드 `agent_ref` 까지 순회(`deriveDownstreamAgents`)하여 **agent 자동 도출**(단일→자동, 미배선/모호→수동 폴백). 명시 `declared_agent_id` 유지. **AMENDS RD-3/RD-9**.
- 산출: 플랫 테이블 + 모달-내부 create + agent 자동 도출. 컴포넌트 테스트.

## 4. 리스크 및 대응

| 리스크 | 영향 | 대응 |
| --- | --- | --- |
| 메타데이터가 와이어 경로에서 유실(A-2 위반) | result 상관 미기록 | M2 초기 스파이크로 trigger→xsfm-control 메타 보존 실증; 유실 시 payload 반송으로 폴백. fire 이벤트는 관측자 경로라 와이어 유실 무관(RD-8) |
| fire↔result 조인 키 충돌(반복 발화) | 잘못된 병합 | `correlation_id`=schedule_id+trigger_time 로 발화별 고유(RD-8), tick_count 보조; 단위 테스트로 검증 |
| 다중 대상 발화 로그 입도(RD-7 확정) | 스키마 복잡 | 집계 1건 + `targets` JSON 임베드로 확정, recordGroupAudit 패턴 미러 |
| agent_id 권위(RD-6 확정) | 그룹핑/로그 불일치 | 선언(`declared_agent_id`) + 실제(`actor_agent_id`) 병기로 불일치 가시화 |
| 크로스-플로우 팬아웃 성능(RD-10) | View 로딩 지연 | 로딩/부분실패 처리(REQ-07-05); v1 팬아웃, 스케일 시 백엔드 집계 승급(후속, REQ-04-07) |
| 훅 추출 회귀(REQ-06-04) | 패널 저장 파손 | 추출 전 패널 특성화 테스트 고정 |
| fire 관측자 미주입 시 로그 공백(RD-8) | 발화 로그 누락 | main.go 배선 검증; 미주입은 best-effort no-op(제어 무영향, REQ-02-07) |

## 5. 검증 전략

- 백엔드: Go 단위/통합 테스트 + `-race`, 신규 코드 커버리지 85%+.
- 프런트: vitest 컴포넌트/훅 테스트 + `tsc` 클린.
- 무회귀: trigger.go 발화 특성화 테스트, FacilitySchedulePanel 특성화 테스트.
- Git 작업(브랜치/커밋/PR)은 core-git(manager-git) 이 담당한다. 본 계획은 이를 수행하지 않는다.
