---
id: SPEC-SCHEDULE-VIEW-001
title: "Schedule View — 기술 설계"
version: "0.3.0"
status: in-progress
created: 2026-07-31
updated: 2026-08-01
author: xtra
tier: L
---

# SPEC-SCHEDULE-VIEW-001 — 기술 설계 (design.md)

> **버전 노트 (0.3.0)**: as-implemented 동기화. §1~§5(0.2.0 설계)는 불변 보존하고, §6(0.3.0 확장 설계 — 3-백엔드 스토리지 / 로그 API 변경 / 로그 탭 UX / 저장방식 설정 / TARGET 테이블 picker / 관리 탭 플랫 테이블 + agent 자동 도출)과 §7(0.3.0 설계 수정 Amendments — RD-11 Clear, RD-3/RD-7 플랫 테이블, RD-3/RD-9 create 하류 엣지 순회)을 가산했다.
> **버전 노트 (0.2.0)**: OQ-1~7 확정(RD-5~11) 반영. 로그 축을 **2-이벤트 모델**(fire 이벤트 + result 이벤트, `correlation_id` 조인)로 확장하여 발화-only 기록(RD-8)을 포함하고, fire 기록을 **주입 관측자**로 제네릭 트리거 노드에서 비결합했다. agent 필드는 선언(`declared_agent_id`) + 실제(`actor_agent_id`) 병기(RD-6), fan-out 은 집계 1건 + 대상별 임베드(RD-7), RBAC 는 전체 인증 사용자(RD-5), 집계는 프런트 팬아웃(RD-10), 보존은 무제한 append-only(RD-11).

## 1. 아키텍처 개요

두 개의 데이터 축을 신설한다.

1. **로그 축(write→read)**: 트리거 발화 → xsfm 제어 → 상관 로그 기록 → 조회 API → 로그 뷰.
2. **관리 축(aggregate→CRUD)**: 크로스-플로우 스케줄 집계 → 에이전트별 그룹핑 → 모달 CRUD → dual-write.

```
[TriggerNode.buildMessage / fire path]
   emits metadata: trigger.schedule_id / rule_name / priority / (신규) agent_id / trigger_time
   ├─▶ (A) fire-side: SetScheduleFireObserver 로 주입된 ScheduleFireObserver.OnScheduleFire(corr)  (best-effort, 미주입 시 no-op)
   │        └─▶ facility-schedule 관측자 → ScheduleLogRepository.Append{RecordKind:fire, correlation_id}
   │            (제네릭 트리거 노드는 저장소 타입을 모름 — RD-8 비결합)
   │
   └─▶ (B) wire; metadata preserved — A-2
        ▼
[XsfmControlNode.Process → buildXsfmControlCommand]
   reads msg.Metadata() → encodes cmd["_correlation"]={schedule_id,rule_name,declared_agent_id,trigger_time}
        │
        ▼
[XSFMAgent.Process → recordControlAudit/recordGroupAudit + (신규) recordScheduleLog]
   correlation block + actor_agent_id(RD-6) + 집계 result + targets 임베드(RD-7)
        └─▶ ScheduleLogRepository.Append{RecordKind:result, 동일 correlation_id}   (best-effort)
        │
        ▼
[storage.ScheduleLogRepository (신설, append-only sqlite)]
   fire ∪ result 이벤트; correlation_id 로 조인 → 하나의 논리 레코드 (fire-only = fire 만)
        │
        ▼
[ScheduleLogHandler: GET /schedules/logs?schedule_id&rule_name&agent_id&limit&offset]  (전체 인증 사용자 — RD-5)
        │
        ▼
[web: useScheduleLogs → 로그 탭 (correlation_id 로 fire+result 병합 표시)]

[web: useFlows + useQueries → config.schedules 수집 → agent_id 그룹핑]
        │
        ▼
[web: SchedulesPage 관리 탭 → FacilityRuleModal → useScheduleDualWrite → configureNode + updateFlow]
```

## 2. 백엔드 설계

### 2.1 저장소 (신설 — RD-1)

- 파일(제안): `internal/storage/schedule_log_repository.go`(인터페이스 + `ScheduleLogRecord` + `ScheduleLogFilter`), `internal/storage/schedule_log_sqlite.go`(구현).
- 참조 미러: `internal/storage/remote_audit_repository.go`(:48 record, :62 interface) + `remote_audit_sqlite.go`(`Append` :88 / `List` :101 / 마이그레이션 :66).
- `ScheduleLogRecord`(§4.1 spec.md 표 — `CorrelationID`/`RecordKind`/`DeclaredAgentID`/`ActorAgentID`/`Targets` 포함) + `ScheduleLogFilter{ ScheduleID, RuleName, AgentID string }`(AgentID 는 선언·실제 어느 쪽이든 매칭).
- 인터페이스:

```
Append(ctx, rec ScheduleLogRecord) error   // fire 이벤트·result 이벤트 각각 append (갱신 API 없음)
List(ctx, f ScheduleLogFilter, limit, offset int) ([]ScheduleLogRecord, error)  // 최신순
Close() error
```

- 인덱스: `idx_schedule_log_corr (correlation_id)`, `idx_schedule_log_schedule (schedule_id)`, `idx_schedule_log_actor (actor_agent_id)`, `idx_schedule_log_declared (declared_agent_id)`, `idx_schedule_log_ts (timestamp DESC)`.
- 보존: 무제한 append-only, pruning 없음(RD-11). 정리 정책은 후속.
- 배선: `main.go` startup → `NewScheduleLogRepository(...)` → `xsfm.SetScheduleLogRepository(repo)`(result 측) + `SetScheduleFireObserver(obs)`(fire 측 주입 관측자) — 둘 다 패키지 싱글턴 `SetAuditRepository` 패턴, 미주입 시 no-op + 핸들러 주입.

### 2.2 상관 반송 경로 — 2-이벤트 모델 (RD-2 / RD-6 / RD-7 / RD-8)

**조인 키(RD-8)**: `correlation_id = schedule_id + ":" + trigger_time`(epoch ms). 스케줄은 반복 발화하므로 `schedule_id` 단독은 발화별 고유성이 없어 `trigger_time` 을 결합한다(`tick_count` 보조). 트리거가 `trigger.trigger_time`(trigger.go:949)/`trigger.tick_count`(:948)를 이미 방출 → fire·result 양측이 동일 키를 산출한다. append-only + 갱신 API 부재(REQ-02-03)이므로 fire 는 in-place 갱신하지 않고 result 를 동일 키로 별도 append, 조회 시 조인한다(REQ-02-10).

**(A) fire-side 기록 — 제네릭 트리거 노드 비결합(RD-8)**:

- **관측자 인터페이스**: 중립 위치(예 `internal/node` 또는 신설 `internal/schedule`)에 `ScheduleFireObserver interface { OnScheduleFire(corr ScheduleFireContext) }` 정의. `ScheduleFireContext{ CorrelationID, ScheduleID, RuleName, DeclaredAgentID string; TriggerTime int64 }`.
- **주입**: 패키지 싱글턴 `SetScheduleFireObserver(obs)`(`SetAuditRepository` 미러, 기본 nil → no-op). `main.go` startup 에서 facility-schedule 측 구현을 주입.
- **트리거 호출 지점**: `TriggerSchedule.AgentID`(신규) → `buildMessage`(trigger.go:945~)에서 `entry.agentID != ""` 일 때만 `message.WithMetadata("trigger.agent_id", ...)` 추가(Name/Priority 조건부 방출 패턴 계승, 무회귀). 발화 지점에서 **관측자가 설정돼 있으면** `obs.OnScheduleFire(corr)` best-effort 호출 — 트리거 노드는 `ScheduleLogRepository` 타입을 알지 못한다(REQ-02-11).
- **fire 이벤트 append**: 주입된 facility-schedule 관측자 구현이 `RecordKind=fire` 레코드(result 없음, `DeclaredAgentID` 채움, `ActorAgentID` 빈 값)를 `ScheduleLogRepository.Append` 로 기록. best-effort(REQ-02-07). **이 경로는 하류 제어 연결 여부와 무관하게 항상 실행 → 발화-only 포착**.

**(B) result-side 기록 — 제어 결과 반송**:

- **명령 인코딩**: `buildXsfmControlCommand`(xsfm.go:936) 확장 — `msg.Metadata()` 에서 `trigger.schedule_id`/`trigger.rule_name`/`trigger.agent_id`/`trigger.trigger_time`/`trigger.priority` 읽어 `cmd["_correlation"]` 블록으로 실음. 전부 부재 시 블록 생략(무회귀, REQ-02-06). 기존 device_id/group_id/params/command 인코딩은 불변.
- **에이전트 기록**: 제어 경로(`controlDevice`→`recordControlAudit`, 셀렉터 fan-out→`recordGroupAudit`) 인접에 `recordScheduleLog(correlation, actor, result)` 추가. 상관 블록이 있을 때만 호출. 동일 `correlation_id` 의 `RecordKind=result` append. `ActorAgentID` = 실제 실행 에이전트(auditActor/`XSFMNodeConfig.AgentRef`, RD-6 권위), `DeclaredAgentID` = 상관 블록의 선언 agent(RD-6 병기). best-effort(REQ-02-07).
- **입도(RD-7)**: fan-out 은 **집계 result 1건** + 대상별 ok/error 를 `Targets`(JSON)에 임베드(`recordGroupAudit` 요약 패턴 계승, Reason 에 `members=[..] ok=N/M`). 대상별 N 레코드를 남기지 않는다.

**결선 근거(왜 fire=관측자, result=제어 경로)**: fire-only(pure trigger / xsfm-control 미연결)는 하류 노드가 없어 하류 컬렉터 노드로는 포착 불가 → fire 포착은 발화 지점(주입 관측자)이어야 한다. result 는 제어 경로만이 발화 상관(통과 보유) + 실제 실행 에이전트 + 결과를 동시 보유 → 제어 경로가 기록. 하류 컬렉터 노드 대안은 fire-only 를 놓치므로 기각.

### 2.3 조회 API (Module 3)

- 파일(제안): `internal/api/handler/schedule_log.go`. `remote_admin.go` Audit(:331) 미러.
- 라우트: `g.GET("/schedules/logs", h.Logs)` — `?schedule_id=&rule_name=&agent_id=&limit=&offset=`(기본 limit=100), 미구성 시 빈 목록.
- RBAC(RD-5): **전체 인증 사용자** — `requireAdmin` 을 적용하지 않는다(AgentListPage 동급). remote/audit 의 admin 게이트와 의도적으로 상이.
- DTO: `internal/api/dto` 에 `ScheduleLogResponse`(fire+result 조인 표현 — `correlation_id`, `record_kind`, `declared_agent_id`/`actor_agent_id`, `targets` 포함).

## 3. 프런트엔드 설계

### 3.1 라우팅/네비 (Module 4)

- `router.tsx`: `const SchedulesPage = lazy(() => import('@/pages/schedules/SchedulesPage'))` + AppLayout children `{ path: '/schedules', element: <SuspenseWrapper><SchedulesPage/></SuspenseWrapper> }`. **admin 전용 추가 AuthGuard 없음** — 전체 인증 사용자(RD-5).
- `Sidebar.tsx`: `NAV_ENTRIES` 에 `{ labelKey: 'nav.schedules', path: '/schedules', icon: CalendarClock }` — `roles: ['admin']` 게이팅 없음(RD-5). i18n 리소스에 `nav.schedules` 추가.

### 3.2 페이지 (Module 4/5)

- `web/src/pages/schedules/SchedulesPage.tsx`(AgentListPage 패턴). 탭 2개.
- **관리 탭**: `useFlows` + `useQueries`(flow별 getFlow) → `config.schedules` 수집 → `agent_id` 그룹핑(미지정 버킷) → 에이전트 섹션별 규칙 테이블(`facilityScheduleUtils` sort/parse) → `FacilityRuleModal`(agentId 반영) CRUD → `useScheduleDualWrite`.
- **로그 탭**: `useScheduleLogs({ scheduleId, ruleName, agentId, limit, offset })`(react-query) → 필터 바 + 테이블(발화+결과 상관) + 페이지네이션 + 빈 상태.

### 3.3 dual-write 훅 (Module 6)

- `web/src/pages/dashboard/panels/facilitySchedule/useScheduleDualWrite.ts`(또는 공유 hooks 디렉터리).
- `FacilitySchedulePanel.persist`(:133-179) 로직 이관: LIVE `configureNode` → 404 persist-only → PERSIST `getFlow`+`detectConflict`+`patchNodeConfigInDefinition`+`updateFlow` → baseline 갱신 + 알림.
- 시그니처: `useScheduleDualWrite({ flowId, nodeId, baselineRef }) => { persist(next), saving }`. 패널이 동일 훅 소비하도록 교체(순수 리팩터, REQ-06-04).

## 4. 데이터 계약 (요약)

- fire 관측자 컨텍스트: `ScheduleFireContext = { correlation_id, schedule_id, rule_name, declared_agent_id, trigger_time }`.
- 명령 상관 블록: `cmd._correlation = { schedule_id, rule_name, declared_agent_id, trigger_time }`(전 필드 선택; 부재 시 블록 생략).
- 조인 키: `correlation_id = schedule_id + ":" + trigger_time`.
- 로그 레코드: spec.md §4.1 표(`correlation_id`/`record_kind`/`declared_agent_id`/`actor_agent_id`/`targets` 포함).
- API 응답: `{ logs: ScheduleLogResponse[], total?: number }`(remote/audit 응답 형태 준용; fire+result 는 `correlation_id` 로 병합 표현).

## 5. 대안 및 근거

- **로그를 remote_audit 에 통합** vs **전용 저장소**: 전용(RD-1). 감사(변경 mutation)와 스케줄 실행 로그는 관심사/스키마/RBAC 가 상이 → 오버로딩 회피.
- **agent_id 를 엣지 순회로 유도** vs **명시 필드**: 명시 필드(RD-3/RD-9). 엣지 순회는 취약(런타임 의존, 부정확) → 그룹핑 1급화.
- **로그 agent = 선언만** vs **실제만** vs **둘 다 병기**: 둘 다 병기(RD-6). 선언(`declared_agent_id`)은 그룹핑 공급원, 실제(`actor_agent_id`)는 권위 값 → 불일치 가시화를 위해 병기.
- **fire 를 트리거 노드가 직접 저장** vs **주입 관측자** vs **하류 컬렉터 노드**: 주입 관측자(RD-8). 직접 저장은 제네릭 노드를 스케줄-로그에 결합, 하류 컬렉터 노드는 fire-only(미연결/순수 트리거)를 놓침 → 발화 지점의 주입 관측자가 유일하게 비결합 + fire-only 포착을 동시 만족.
- **fire→result in-place 갱신** vs **동일 correlation_id 별도 append + 조회 조인**: 별도 append(RD-8). append-only + 갱신 API 부재(REQ-02-03) 규약 준수, 조회 시 조인.
- **result 를 대상별 N 건** vs **집계 1건 + 임베드**: 집계 1건(RD-7). `recordGroupAudit` 패턴 계승, 관측성 충분 + 저장 절약.
- **프런트 팬아웃** vs **백엔드 집계**: 팬아웃(RD-10, A-5), v1 백엔드 엔드포인트 없음. 규모 초과 시 후속 승급.
- **RBAC admin 전용** vs **전체 인증**: 전체 인증(RD-5). 스케줄 관리는 운영 성격 → AgentListPage 동급, remote/audit admin 게이트와 의도적 상이.
- **보존 무제한** vs **pruning**: v1 무제한 append-only(RD-11), 보존/정리 정책 후속.

---

## 6. 0.3.0 확장 설계 (as-implemented)

> M1~M7 구현 완료 후 실제 구축된 확장 설계. spec.md §8(Module 8~13) 에 대응.

### 6.1 스토리지 3-백엔드 팩토리 (Module 8 — B)

- **팩토리**: `storage.schedule_log.type` config(`internal/config` `StorageConfig.ScheduleLogType`, 기본 `sqlite`)에 따라 3 구현 중 하나를 startup 에 생성.
  - `sqlite`(기본, 영속) — 기존 `schedule_log_sqlite.go`.
  - `memory`(비영속) — 신설 `internal/storage/schedule_log_memory.go`(in-memory 슬라이스 + mutex).
  - `file`(JSONL append-only 파일) — 신설 `internal/storage/schedule_log_jsonl.go`(라인당 1 레코드 JSON, append).
- **인터페이스 확장**: `Append`/`List` 에 더해 **`Clear(ctx)`**(전 로그 삭제 — 수동 전체 초기화) + **`Count(ctx, filter)`**(페이지네이션 total). 3-백엔드 모두 동형 계약. **개별 레코드 삭제 API 는 미도입**(RD-11 amendment — §7 AM-0.3.0-1).
- **배선**: `main.go` startup 에서 config 값을 읽어 팩토리로 저장소 인스턴스를 선택 후 `SetScheduleLogRepository` 주입.

### 6.2 로그 조회 API 변경 (Module 9 — C)

- **응답 형태**: `GET /schedules/logs` → `{ items: ScheduleLogResponse[], total: number }`(기존 bare array 에서 변경; `Count` 로 total 산출).
- **`DELETE /schedules/logs`**: **전체 인증 사용자**(admin 아님 — RD-5), `Clear` 호출.
- **저장방식 설정**: 신설 핸들러 `internal/api/handler/schedule_log_config.go` — `GET/PUT /system/schedule-log-config`(**admin 전용**), `storage.schedule_log.type` 읽기/설정 + **needs_restart** 신호 반환. `internal/config/overrides.go` allowlist 에 해당 키 확장(런타임 override 허용). RBAC 대비: 로그 조회/삭제 = 전체 인증(운영), 저장방식 설정 = admin 전용(구성 변경) — 의도적 분리.

### 6.3 로그 탭 UX (Module 10 — D)

- **페이지네이션**: page size 25(기본)/50/100 셀렉터 + prev/next + total 표시(`{items,total}` 소비).
- **CSV 내보내기**: 필터 매칭 **전체 행**(현재 페이지 아님)을 조회하여 **UTF-8 BOM** 접두 CSV 생성. 컬럼: 실행시각 / 규칙이름 / 에이전트(선언, 실제 다르면 병기) / 대상 / 동작 / 결과.
- **전체 초기화**: 파괴적 버튼 → confirm 다이얼로그 → `DELETE /schedules/logs`.

### 6.4 저장방식 Settings 카드 (Module 11 — E)

- **admin 전용** Settings 카드("스케줄 로그 저장 방식")로 sqlite / 파일(JSONL) / 메모리 선택 → `PUT /system/schedule-log-config` 영속 → 재시작 시 적용(needs_restart 안내).

### 6.5 TARGET 테이블 picker (Module 12 — F)

- 규칙 모달의 device-target picker 를 **테이블**로 개편. 컬럼 **이름 / 라인 / 역사 / 위치** — device 를 station line + station display_name + place display_name 과 조인해 표시. **검색 필터** + **행 선택**.
- **공유 모달**이므로 Schedule View 와 대시보드 Trigger 패널 양쪽에 동시 적용. 저장되는 `TargetSpec` shape 은 불변(표시 계층만 확장).

### 6.6 관리 탭 플랫 테이블 + agent 자동 도출 (Module 13 — G)

- **플랫 테이블**: 에이전트별 그룹 섹션 대신 **단일 플랫 테이블**(에이전트(이름) 컬럼 + 플로우/노드 컬럼). React Rules-of-Hooks 는 **per-node `<tbody>` 컴포넌트**로 분리해 그룹 반복 렌더링에서의 Hooks 순서 취약성을 제거. (AMENDS RD-3/RD-7 — §7 AM-0.3.0-2)
- **규칙 생성 모달 내부화**: 별도 "대상 노드" 생성 카드 제거. "규칙 추가" → 설정 모달 직접 오픈, **대상 노드를 모달 첫 필드**로 선택 → 에이전트 + 대상 + plan + action 일괄 입력. dual-write persist 를 독립 함수 **`persistScheduleDualWrite`** 로 추출(모달 저장 시점에 선택된 노드를 타깃팅).
- **agent 자동 도출**: create 시 트리거 노드 선택 → **`deriveDownstreamAgents`** 가 플로우 하류 엣지를 제어 노드 `agent_ref` 까지 **단일-노드 순회** → 단일이면 자동 채움, 미배선/모호(다중 하류)면 모달 내 수동 폴백. 명시 `declared_agent_id` 는 저장/로깅/표시용으로 유지. (AMENDS RD-3/RD-9 — §7 AM-0.3.0-3)

## 7. 0.3.0 설계 수정 근거 (Amendments)

> 원 RD 텍스트(spec.md §5)는 보존, 아래는 대안 대비 승계 근거. 정식 amendment 기록은 spec.md §9.

- **AM-0.3.0-1 (RD-11)** — **개별 삭제 없음(유지)** vs **전체 초기화(Clear) 예외 허용**: Clear 예외. 개별-레코드 불변(감사성)은 유지하되, 운영상 "전량 리셋"이라는 명시적·일괄 파괴 동작만 예외로 허용. `Count` 는 페이지네이션 total 을 위해 부수 추가. 보존-기반 자동 pruning 은 여전히 후속.
- **AM-0.3.0-2 (RD-3/RD-7)** — **에이전트별 그룹 섹션** vs **단일 플랫 테이블 + 에이전트 컬럼**: 플랫 테이블. 다수 에이전트/노드 스케줄의 일람·정렬·검색성이 우수하고, per-node `<tbody>` 로 Hooks 순서를 안정화. 로그 입도(RD-7 집계 1건 + 임베드) 규약은 불변.
- **AM-0.3.0-3 (RD-3/RD-9)** — **엣지 순회 전면 배제** vs **create 시 단일-노드 하류 순회 자동 도출**: create 한정 단일-노드 순회. 원 취약성 우려는 **대규모 크로스-플로우 그룹핑**(전체 스케줄 엣지 재구성)에 국한 → 단일 create 시점의 좁은 순회는 안전. 도출 실패에는 수동 폴백. 저장·로깅·표시 권위는 계속 명시 `declared_agent_id`(RD-6 병기 불변).
