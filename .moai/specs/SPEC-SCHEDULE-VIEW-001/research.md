---
id: SPEC-SCHEDULE-VIEW-001
title: "Schedule View — 조사 근거"
version: "0.3.0"
status: in-progress
created: 2026-07-31
updated: 2026-08-01
author: xtra
tier: L
---

# SPEC-SCHEDULE-VIEW-001 — 조사 근거 (research.md)

> **버전 노트 (0.3.0)**: as-implemented 동기화. §1~§10(0.2.0 정찰)은 불변 보존하고, 0.3.0 확장(M8~M13)이 실제 착지한 코드 지점을 §11 에 요약한다(상세 설계는 design.md §6~§7, 요구사항은 spec.md §8~§9).
> 코드베이스 정찰(reconnaissance) 결과. 모든 참조는 검증된 file:line. 결함 주장이 아니라
> 재사용 지점·통합 지점·현재 단절 지점의 근거 기록이다.

## 1. 스케줄 저장 구조

- `internal/node/trigger.go:68-95` — `TriggerSchedule` 구조체. SPEC-TRIGGER-SCHED-001 규칙 메타 필드 존재: `Name`(:88), `ValidFrom`(:89), `ValidTo`(:90), `Priority`(:91), `Enabled *bool`(:94, tri-state). **`agent_id` 필드 없음** → REQ-01-01 로 가산.
- config 파싱: `trigger.go:246-305`(`schedules` 배열 → 항목별 name/valid_from/valid_to/priority/enabled 파싱). `agent_id` 파싱 추가 지점.
- **결론**: 확장 필드는 config 부재 시 하위 호환 기본값(무회귀) 이미 확립 → `agent_id` 가산이 동일 패턴으로 안전.

## 2. 발화 시 상관 메타 방출

- `internal/node/trigger.go:930-965` — `buildMessage`. `vars` 에 `schedule_id`/`trigger_id`(:933-936). `opts` 메타: `trigger.schedule_id`(:946), `trigger.tick_count`(:948), `trigger.trigger_time`(:949), `trigger.node_name`(:950).
- 조건부 pass-through(:955-962): `entry.name != ""` → `trigger.rule_name`(:957), `entry.priority != 0` → `trigger.priority`(:960). **무의미 값 무방출로 무회귀 보장** → `trigger.agent_id` 도 동일 조건부 방출로 추가(REQ-01-03/04).

## 3. xsfm 제어 경로 — 현재 상관 단절

- `internal/node/xsfm.go:474` — `XsfmControlNode.Process` → `buildXsfmControlCommand(msg, deviceID, n.ID())`(:477).
- `internal/node/xsfm.go:936-1010` — `buildXsfmControlCommand`. 명령을 `msg.Payload()` 로부터 구성(device_id/group_id/device_name/group_name/params/command). **`msg.Metadata()` 를 전혀 읽지 않음** → 트리거 상관 메타가 여기서 유실. 이것이 §4.2 반송 경로의 핵심 확장 지점.
- 셀렉터 추출 헬퍼: `xsfmExtractGroupID`(:942), `xsfmExtractDeviceName`/`xsfmExtractGroupName`. 명령 검증: 제어 키(power/fan_speed) 추론(:986-1005).

## 4. xsfm 감사 경로 — 스케줄 상관 부재

- `internal/agent/xsfm/audit.go:74` — `recordControlAudit(res memberResult)` → `RemoteAuditRecord{ InstanceID: res.DeviceID, Action: command, Domain: device, CommandAction: res.Command, Result, Reason }`(:85-96). 결과는 있으나 **schedule_id/rule_name 없음**.
- `internal/agent/xsfm/audit.go:105` — `recordGroupAudit(sel, results)` → 그룹 요약 1건, Reason 에 `members=[..] ok=N/M`(:129). **fan-out 집계 로그 패턴의 참조 구현**(OQ-1 집계안 근거).
- `audit.go:40-42` — `SetAuditRepository`(패키지 싱글턴, main.go 단일 주입) → `SetScheduleLogRepository` 동형 배선 근거.
- **결론**: 제어 경로가 결과를 이미 기록 중 → 여기에 상관 블록을 더해 신설 로그 저장소로 기록하는 것이 최소 변경.

## 5. append-only 저장소 패턴 (재사용 템플릿)

- `internal/storage/remote_audit_repository.go:48-72` — `RemoteAuditRecord`(Timestamp epoch ms 규약 명시 :47) + `RemoteAuditRepository` 인터페이스(`Append` 추가 전용, `List` 최신순 + instanceID 필터 + limit/offset, `Close`).
- `internal/storage/remote_audit_sqlite.go` — `sqliteDSN`(:30), `NewRemoteAuditSQLiteRepository`(:41), `migrateRemoteAuditSchema`(:66), `Append`(:88), `List`(:101, ts desc + 필터 + 페이지네이션), `Close`(:144).
- `NewRemoteAuditRepository(ctx, storageType, sqlitePath)`(:77) 팩토리. **결론**: 스키마 필드만 교체하여 그대로 미러(RD-1).

## 6. 조회 API 패턴 (재사용 템플릿)

- `internal/api/handler/remote_admin.go:163` — `g.GET("/remote/audit", h.Audit)`. `Audit` 핸들러(:331) admin-gated(`requireAdmin` :178, role=admin :180) + `?instance_id=` 필터 + `limit`/`offset`(기본 limit=100), audit 미구성 시 빈 목록(:339). `h.audit.List(...)`(:346).
- `internal/api/server.go:150` — `RegisterRoutes(register func(g *RouteGroup))`. `cmd/xflowd/main.go:871-884` — 핸들러별 `RegisterRoutes(g)` 배선. **결론**: `GET /schedules/logs` 동형 신설(RD-1, Module 3).

## 7. xsfm 노드 config

- `internal/node/xsfm.go:52-58` — `XSFMNodeConfig{ AgentRef(:53), Timeout, EmitMetadata }`. `AgentRef` = 실행 대상 에이전트 → OQ-2(로그 agent_id 권위: 선언 스케줄 agent_id vs 실행 AgentRef)의 근거.

## 8. 프런트 재사용 자산

- `web/src/router.tsx` — createBrowserRouter + React.lazy, AuthGuard→AppLayout children. `/admin/system` 추가 AuthGuard(admin) 선례(:주석). 라우트 추가 지점.
- `web/src/components/layout/Sidebar.tsx:65-147` — `NAV_ENTRIES`(labelKey i18n, path, icon, roles 게이팅). remote 그룹은 `roles: ['admin']` 선례(:107~). nav 추가 지점.
- `web/src/pages/agents/AgentListPage.tsx:67` — 페이지 패턴.
- `web/src/hooks/useFlow.ts:11/35` — `useFlows` / `useFlowNodes`(config.schedules). `useNodeTypeInstances('trigger')` 는 RUNNING flow + no config → 집계에 부적합.
- `web/src/hooks/useAgent.ts:11` — `useAgents`, xsfm 필터 `a.type==='xsfm'`.
- `web/src/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils.ts` — 셀렉터/액션 build-parse, formatter, sort, rule↔schedule(순수·이식형).
- `web/src/pages/dashboard/panels/triggerPanelUtils.ts` — `buildFullTriggerConfig` / `patchNodeConfigInDefinition` / `findNodeConfigInDefinition` / `detectConflict`(dual-write 헬퍼).
- `web/src/pages/dashboard/panels/facilitySchedule/FacilitySchedulePanel.tsx:73`(패널-로컬 config.agentId), `:133-179`(dual-write persist 인라인 — Module 6 추출 대상). `FacilityRuleModal.tsx`(이식형, agentId 필요).

## 9. 근거 → OQ → 확정(RD) 매핑

> v0.2.0: OQ-1~7 전부 사용자 확정 → RD-5~11 승급(spec.md §5). 아래 근거가 각 확정의 코드 기반이다.

| 근거 | OQ → 확정 | 확정 요지 |
| --- | --- | --- |
| `recordGroupAudit` 집계 패턴(audit.go:105) | OQ-1 → **RD-7** | 발화당 집계 1건 + 대상별 `targets` 임베드(N건 아님) |
| `XSFMNodeConfig.AgentRef`(xsfm.go:53) vs 스케줄 agent_id | OQ-2 → **RD-6** | 실제 실행(`actor_agent_id`) 권위 + 선언(`declared_agent_id`) 병기 |
| `TriggerSchedule` per-schedule 필드(trigger.go:68) | OQ-3 → **RD-9** | agent 필드는 스케줄별 `config.schedules[].agent_id` |
| `useFlows`+`useQueries` 팬아웃(useFlow.ts:11) | OQ-4 → **RD-10** | 프런트 팬아웃, v1 백엔드 집계 없음(스케일 시 후속) |
| `requireAdmin`(remote_admin.go:178) | OQ-5 → **RD-5** | 전체 인증 사용자(admin 미적용, AgentListPage 동급) |
| `buildXsfmControlCommand` fire→control 결합점(xsfm.go:936) + `SetAuditRepository` 싱글턴(audit.go:40) | OQ-6 → **RD-8** | 발화-only 도 기록. fire 는 발화 지점 **주입 관측자**(`ScheduleFireObserver`, 저장소 싱글턴 패턴 미러)로 비결합 기록, result 는 제어 경로가 동일 `correlation_id`(schedule_id+trigger_time)로 append → 조회 시 조인. 하류 컬렉터 노드는 fire-only 를 못 잡아 기각 |
| `remote_audit` 무제한 append(remote_audit_repository.go:47) | OQ-7 → **RD-11** | v1 무제한 append-only, pruning 후속 |

> **RD-8 비결합 근거**: `SetAuditRepository`(audit.go:40, 패키지 싱글턴 + 미주입 no-op)가 이미 확립된 주입 패턴이므로, 동형 `SetScheduleFireObserver` 를 발화 지점에 두면 제네릭 트리거 노드가 `ScheduleLogRepository` 를 인지하지 않고도 fire 를 포착한다. `trigger.go:955-962` 의 조건부 메타 방출과 같은 지점에서 관측자를 best-effort 호출한다.

## 10. 참고 상위 SPEC

- SPEC-TRIGGER-SCHED-001(v0.3.0, completed) — 스케줄 규칙/패널/발화 게이팅/상관 메타.
- SPEC-FACILITY-DASHBOARD-001(v0.1.0, completed) — 설비 패널/집계/셀렉터.
- SPEC-XSFM-GROUP-001(v0.4.0, completed) — AM-1 노드 제어 명령 셋/그룹 셀렉터.
- SPEC-XSFM-001 — 제어 감사(REQ-XSFM-001-06-03).
- SPEC-REMOTE-001(M6) — append-only 감사 저장소 + admin-gated 조회 API.

## 11. 0.3.0 확장 코드 지점 (as-implemented)

> 0.3.0 확장(M8~M13)이 착지한 신설/변경 코드 지점 요약. 상세 설계는 design.md §6~§7, 요구사항은 spec.md §8~§9 참조.

- **스토리지 3-백엔드 (M8/B)**: 신설 `internal/storage/schedule_log_memory.go`(memory, 비영속), `internal/storage/schedule_log_jsonl.go`(JSONL append-only 파일). 기존 `schedule_log_sqlite.go`(sqlite 기본). 인터페이스에 `Clear(ctx)` + `Count(ctx, filter)` 추가. config: `internal/config` `StorageConfig.ScheduleLogType`(키 `storage.schedule_log.type`, 기본 `sqlite`).
- **로그 API 변경 (M9/C)**: `GET /schedules/logs` → `{ items, total }`, 신설 `DELETE /schedules/logs`(전체 인증), 신설 핸들러 `internal/api/handler/schedule_log_config.go`(`GET/PUT /system/schedule-log-config`, admin 전용, needs_restart). `internal/config/overrides.go` allowlist 에 `storage.schedule_log.type` 확장.
- **로그 탭 UX (M10/D)**: 페이지네이션(25/50/100)·CSV 내보내기(UTF-8 BOM)·전체 초기화(confirm→DELETE) — 로그 탭 프런트.
- **저장방식 Settings 카드 (M11/E)**: admin 전용 카드 → 설정 엔드포인트 소비.
- **TARGET 테이블 picker (M12/F)**: 규칙 모달 device-target picker 를 테이블(이름/라인/역사/위치 — device ⋈ station line + station display_name + place display_name)로. 공유 모달(Schedule View + 대시보드 Trigger 패널). `TargetSpec` 불변.
- **관리 탭 개편 (M13/G)**: 에이전트별 그룹 → 단일 플랫 테이블(에이전트 이름 컬럼), per-node `<tbody>` 로 Rules-of-Hooks 보존. 규칙 생성 모달 내부화 + dual-write persist 를 `persistScheduleDualWrite` 로 추출. 트리거 선택 시 `deriveDownstreamAgents`(하류 엣지 순회) agent 자동 도출.
- **M5/M6 버그수정 (FIX-1~3)**: `FacilityRuleModal` 에 xsfm 에이전트 선택 가산(TARGET 열거, 대시보드 패널 불변); 스케줄 0개 트리거 노드 관리 뷰 숨김; 삭제-영속 정상(크로스-서피스 에디터 staleness 로 판명).

> **참고**: SPEC-TRIGGER-SCHED-001 은 0.3.0 으로 completed 동기화됨(별도 SPEC).
