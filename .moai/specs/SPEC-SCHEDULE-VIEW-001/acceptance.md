---
id: SPEC-SCHEDULE-VIEW-001
title: "Schedule View — 인수 기준"
version: "0.3.0"
status: in-progress
created: 2026-07-31
updated: 2026-08-01
author: xtra
tier: L
---

# SPEC-SCHEDULE-VIEW-001 — 인수 기준 (acceptance.md)

> **버전 노트 (0.3.0)**: as-implemented 동기화. 0.3.0 확장(B~G, spec.md §8)을 커버하는 신규 AC-19~28 을 가산하고, AM-0.3.0-2(그룹→플랫 테이블)에 따라 **AC-7 을 플랫 테이블 기준으로 갱신**했다(원 시나리오 취지는 보존). AC-1~18(M1~M7)은 불변 보존.
> **버전 노트 (0.2.0)**: OQ-1~7 확정(RD-5~11) 반영. AC-3(fire+control 집계 + actual/declared + targets, RD-6/7), AC-4(agent 필터 선언·실제 매칭), AC-7(선언 agent_id 그룹핑, RD-9), AC-11(RBAC 전체 인증, RD-5) 갱신 + 신규 AC-15~18(발화-only RD-8, fire↔result 조인 RD-8, 비-admin 접근 RD-5, 크로스-플로우 팬아웃 RD-10).
> Given-When-Then. 각 시나리오는 대응 REQ 를 추적한다. 식별자는 영문.

## AC-1 — 스케줄에 대상 에이전트 명시 저장 (REQ-01-01, 01-05)

- **Given** 트리거 노드의 스케줄과 대상 에이전트 `agent_id="xsfm-line2"` 를 편집하는 사용자
- **When** 대시보드 패널 또는 Schedule View 에서 스케줄을 저장하면
- **Then** 해당 스케줄이 `config.schedules[].agent_id = "xsfm-line2"` 로 영속되고, 재로드 시 그 값이 복원된다.

## AC-2 — 발화 메시지에 agent_id 상관 메타 방출 (REQ-01-03/04)

- **Given** `agent_id` 가 설정된 스케줄을 가진 실행 중 트리거 노드
- **When** 그 스케줄이 발화하면
- **Then** 방출 메시지 메타에 `trigger.agent_id` 가 포함된다.
- **And Given** `agent_id` 가 없는 기존 스케줄
- **When** 발화하면
- **Then** `trigger.agent_id` 메타가 추가되지 않고 발화 동작이 이전과 동일하다(무회귀).

## AC-3 — 발화+제어가 단일 집계 상관 레코드를 생성 (REQ-02-04/04b/04c/05, RD-2/6/7)

- **Given** 로그 저장소가 구성되고, 트리거→xsfm-control→에이전트 경로가 연결된 스케줄(`schedule_id=S1`, `rule_name="야간정지"`, 선언 `agent_id="xsfm-line2"`)이 그룹 셀렉터로 4개 대상을 제어(3 성공 1 실패)
- **When** S1 이 발화하여 제어가 실행되면
- **Then** 조회 시 `correlation_id`(=`S1`+trigger_time)로 fire+result 가 병합된 **단일 논리 레코드 1건**이 표시되며, `record_kind` fire/result, `schedule_id=S1`, `rule_name="야간정지"`, `declared_agent_id="xsfm-line2"`, `actor_agent_id`(실제 실행 에이전트, RD-6 권위), `trigger_time`, `target`, `action`, 집계 `result`, 그리고 **대상별 ok/error 가 `targets` 에 임베드(4개 항목, ok=3/4)** 되어 있다(RD-7).
- **And** 대상별로 별도의 레코드 4건이 생성되지 않는다(집계 1건만, RD-7).

## AC-4 — 스케줄/에이전트로 로그 조회 (REQ-03-01~04, REQ-02-02)

- **Given** 여러 스케줄의 실행 로그가 기록된 상태
- **When** `GET /schedules/logs?schedule_id=S1` 을 호출하면
- **Then** S1 관련 레코드만 최신순으로 반환된다.
- **And When** `?agent_id=xsfm-line2` 로 호출하면
- **Then** 선언(`declared_agent_id`) 또는 실제(`actor_agent_id`) 어느 쪽이든 해당 에이전트와 매칭되는 레코드가 반환된다(RD-6).
- **And When** `limit`/`offset` 을 지정하면 페이지네이션이 적용된다.

## AC-5 — 로그 미구성 시 안전 동작 (REQ-02-07, REQ-03-03)

- **Given** 로그 저장소가 미구성인 시스템
- **When** 스케줄 제어가 실행되면
- **Then** 제어는 정상 성공하고 로그 기록은 조용히 no-op 된다(제어 무실패).
- **And When** `GET /schedules/logs` 를 호출하면
- **Then** 오류 대신 빈 목록을 반환한다.

## AC-6 — 상관 메타 없는 제어는 스케줄 로그 미기록 (REQ-02-06)

- **Given** 수동 제어 등 트리거 상관 메타가 없는 발화 메시지
- **When** xsfm 제어가 실행되면
- **Then** 스케줄 로그 레코드가 생성되지 않는다(기존 제어/감사 동작 무회귀).

## AC-7 — 스케줄별 선언 agent 필드 기반 그룹핑 (REQ-04-03/04, RD-3/9)

- **Given** 여러 플로우에 흩어진 트리거 스케줄(각 스케줄이 `config.schedules[].agent_id` 를 개별 보유, RD-9)
- **When** Schedule View 관리 탭이 로드되면
- **Then** 전체 플로우에서 스케줄이 수집되어 스케줄별 선언 `agent_id` 기준으로 그룹핑 표시되며, 그룹핑은 엣지 순회가 아닌 명시 필드에 근거한다.

> **[0.3.0 수정 — spec.md §9 AM-0.3.0-2 / AC-25]** 표시 형태는 **에이전트별 그룹 섹션 → 단일 플랫 테이블(에이전트(이름) 컬럼)** 로 변경되었다. 스케줄별 선언 `agent_id` 기준 표기·명시 필드 근거라는 취지는 불변이며, 플랫 테이블 시나리오는 AC-25 로 검증한다.

## AC-8 — 미지정 에이전트 버킷 (REQ-04-06)

- **Given** `agent_id` 가 없는 스케줄이 존재
- **When** 관리 탭이 로드되면
- **Then** 해당 스케줄이 "미지정" 버킷에 누락 없이 표시된다.

## AC-9 — View CRUD dual-write (REQ-04-05, REQ-06-01~03)

- **Given** Schedule View 관리 탭
- **When** 사용자가 스케줄을 생성/수정/삭제/토글하면
- **Then** LIVE(configureNode) + PERSIST(patch-then-PUT) dual-write 가 수행되고, 노드 미실행(404) 시 persist-only 폴백으로 저장만 적용됨을 알린다.
- **And** 이 dual-write 는 대시보드 패널과 동일한 공유 훅(`useScheduleDualWrite`)을 사용한다.

## AC-10 — 실행 로그 뷰 (REQ-05-01~05)

- **Given** Schedule View 로그 탭
- **When** 탭이 표시되면
- **Then** 로그 API 를 호출하여 발화(schedule_id/rule_name/agent/trigger_time)와 제어 결과(대상별 ok/error)를 상관된 행으로 표시한다.
- **And When** `schedule_id`/`rule_name`/`agent_id` 필터를 지정하면 결과가 좁혀진다.
- **And When** 로그가 비어 있으면 빈 상태를 표시한다(오류 아님).

## AC-11 — 전용 라우트 + 네비게이션 (REQ-04-01/02, RD-4)

- **Given** 인증된 사용자
- **When** 사이드바에서 스케줄 항목(`nav.schedules`)을 클릭하면
- **Then** `/schedules` 풀페이지가 `AppLayout` 내부에서 로드된다.
- **And** admin 전용 게이팅 없이 전체 인증 사용자에게 nav 와 페이지가 노출된다(RD-5).

## AC-12 — 대시보드 패널 불변 병존 (REQ-07-01, REQ-06-04, RD-4)

- **Given** 기존 설비 제어 예약 대시보드 패널
- **When** 본 SPEC 의 변경이 적용된 후
- **Then** 패널의 UI/저장 동작이 이전과 동일하게 유지되고(특성화 테스트 통과), View 와 병존한다.

## AC-13 — 트리거 노드 발화 무회귀 (REQ-07-01/03)

- **Given** `agent_id` 를 사용하지 않는 기존 트리거 스케줄들
- **When** 본 SPEC 적용 후 발화하면
- **Then** 발화 타이밍/페이로드/메타(신규 `trigger.agent_id` 제외)가 이전과 동일하고, 모든 타임스탬프는 epoch ms 이다.

## AC-14 — 품질 게이트 (REQ-07-02/04)

- **Given** 본 SPEC 구현
- **When** 검증을 실행하면
- **Then** 신규 백엔드 코드 커버리지 85%+, `go test -race` 클린, `tsc` 클린, `vitest` 클린이며, 로그 기록 지연/실패가 제어 경로를 지연/실패시키지 않는다.

## AC-15 — 발화-only 는 fire-only 레코드로 기록 (REQ-02-09, RD-8)

- **Given** 로그 저장소가 구성되고, 하류 제어에 연결되지 않은(또는 순수) 트리거 스케줄(`schedule_id=S2`, `rule_name="점검알림"`, 선언 `agent_id="xsfm-line3"`)과 주입된 fire 관측자
- **When** S2 가 발화하면(하류 제어 미실행)
- **Then** 스케줄 로그에 `record_kind=fire`, `correlation_id`(=`S2`+trigger_time), `schedule_id=S2`, `rule_name`, `declared_agent_id`, `trigger_time` 를 가진 **fire-only 레코드 1건**이 기록되며, `result`/`actor_agent_id`/`targets` 는 비어 있다.
- **And** 이 기록은 제네릭 트리거 노드가 아니라 주입된 `ScheduleFireObserver` 경로로 수행된다(RD-8 비결합).

## AC-16 — fire 이벤트와 result 이벤트의 correlation 조인 (REQ-02-10, RD-8)

- **Given** 제어까지 도달한 발화(S1)의 fire 이벤트와 result 이벤트가 append 된 상태
- **When** 로그를 조회하면
- **Then** 두 이벤트가 동일 `correlation_id` 로 조인되어 **하나의 논리 행**(발화 + 제어결과)으로 표현되고, fire-only(S2)는 result 없이 fire 행으로만 표현된다.
- **And Given** 스케줄 상관 메타가 전혀 없는 수동 제어
- **When** 제어가 실행되면
- **Then** fire/result 어느 레코드도 생성되지 않는다(REQ-02-06 무회귀).

## AC-17 — 비-admin 인증 사용자 접근 (REQ-03-05, REQ-04-01, RD-5)

- **Given** admin 이 아닌 일반 인증 사용자(AgentListPage 접근 가능 등급)
- **When** `/schedules` 페이지를 열고 `GET /schedules/logs` 를 호출하면
- **Then** 페이지와 로그 API 가 정상 접근되며 403/권한 거부가 발생하지 않는다(remote/audit 의 admin 게이트와 상이).

## AC-18 — 크로스-플로우 팬아웃 집계 (REQ-04-03, RD-10)

- **Given** 스케줄이 여러 플로우에 흩어져 있는 상태
- **When** 관리 탭이 로드되면
- **Then** View 가 `useFlows`+`useQueries` 팬아웃으로 각 플로우의 `config.schedules` 를 수집·집계하며(백엔드 집계 엔드포인트 미사용, RD-10), 로딩/부분 실패 상태를 명시적으로 처리한다(REQ-07-05).

---

## 0.3.0 확장 인수 기준 (AC-19~28, as-implemented)

> B~G(spec.md §8, Module 8~13) 커버. 모두 **이미 구현·테스트 완료(green)** 상태의 as-implemented 검증이다.

## AC-19 — 3-백엔드 스토리지 + config 선택 (REQ-08-01/02, B)

- **Given** `storage.schedule_log.type` config 키(기본 `sqlite`)
- **When** 값을 `memory` 또는 `file`(JSONL) 로 설정하고 startup 하면
- **Then** 해당 백엔드(sqlite 영속 / memory 비영속 / JSONL append-only 파일)로 로그 저장소가 구성되며, 미설정 시 `sqlite` 가 적용된다.

## AC-20 — 전체 초기화(Clear) + total (REQ-08-03/04, REQ-09-02, RD-11 amendment)

- **Given** 여러 로그가 기록된 저장소
- **When** `DELETE /schedules/logs`(전체 인증 사용자) 를 호출하면
- **Then** `Clear` 로 **모든 로그가 삭제**되고, `Count`/`total` 이 0 으로 반영된다.
- **And** 개별 레코드 삭제 API 는 제공되지 않는다(RD-11 amendment — 전체 초기화 예외만 허용).

## AC-21 — 로그 응답 형태 { items, total } (REQ-09-01, C)

- **Given** 로그가 기록된 상태
- **When** `GET /schedules/logs` 를 호출하면
- **Then** 응답이 bare array 가 아니라 `{ items: [...], total: N }` 형태로 반환되어 페이지네이션 total 을 제공한다.

## AC-22 — 저장방식 설정 엔드포인트 (REQ-09-03, REQ-11-01, C/E)

- **Given** admin 사용자
- **When** `GET /system/schedule-log-config` 로 현재 저장방식을 읽고 `PUT` 으로 `sqlite`/`file`/`memory` 중 하나로 설정하면
- **Then** 값이 영속되고 **needs_restart 신호**가 반환되며, 재시작 후 해당 백엔드가 적용된다.
- **And Given** admin 이 아닌 사용자
- **When** 동일 엔드포인트를 호출하면
- **Then** admin 게이팅으로 거부된다(로그 조회 API 의 전체-인증 정책과 상이 — 설정은 admin 전용).

## AC-23 — 로그 탭 페이지네이션 (REQ-10-01, D)

- **Given** 로그 탭
- **When** page size 를 25(기본)/50/100 중 선택하고 prev/next 로 이동하면
- **Then** 선택 크기로 페이징되고 total 표시가 갱신된다.

## AC-24 — CSV 내보내기 (REQ-10-02, D)

- **Given** 필터가 적용된 로그 탭
- **When** "내보내기"(CSV)를 실행하면
- **Then** 필터 매칭 **전체 행**이 **UTF-8 BOM** CSV 로 내보내지며, 컬럼은 **실행시각 / 규칙이름 / 에이전트(선언, 실제 다르면 병기) / 대상 / 동작 / 결과** 이다.

## AC-25 — 관리 탭 플랫 테이블 + 에이전트 컬럼 (REQ-13-01, RD-3/RD-7 amendment)

- **Given** 여러 에이전트/노드에 걸친 스케줄
- **When** 관리 탭이 로드되면
- **Then** 에이전트별 그룹 섹션이 아니라 **단일 플랫 테이블**로 렌더링되며 **에이전트(이름)** 컬럼과 **플로우/노드** 컬럼을 포함한다.
- **And** React Rules-of-Hooks 위반 없이(per-node `<tbody>` 컴포넌트) 정상 렌더링된다.

## AC-26 — 규칙 생성 모달 내부화 + 노드 선택 (REQ-13-02, G)

- **Given** 관리 탭
- **When** "규칙 추가"를 클릭하면
- **Then** 별도 "대상 노드" 생성 카드 없이 **설정 모달이 직접 열리고**, **대상 노드가 모달 첫 필드**로 선택되며, 이어서 에이전트/대상/계획(plan)/동작(action)을 함께 입력한 뒤 저장 시점 노드로 dual-write(`persistScheduleDualWrite`)가 수행된다.

## AC-27 — 트리거 선택 시 agent 자동 도출 (REQ-13-03, RD-3/RD-9 amendment)

- **Given** create 모달에서 트리거 노드를 선택
- **When** 그 트리거의 하류 엣지가 **단일 제어 노드**로 배선되어 있으면
- **Then** `deriveDownstreamAgents` 가 제어 노드 `agent_ref` 를 순회 도출하여 에이전트가 **자동 채움**된다.
- **And When** 트리거가 **미배선이거나 모호한 fan-out**(다중 하류 제어)이면
- **Then** 모달 내 **수동 선택 폴백**으로 사용자가 에이전트를 지정한다.
- **And** 어느 경우든 명시 `declared_agent_id` 필드가 저장/로깅/표시용으로 기록된다.

## AC-28 — TARGET 테이블 picker (공유) (REQ-12-01/02, F)

- **Given** 규칙 모달의 device-target picker
- **When** picker 가 표시되면
- **Then** **테이블**(컬럼 이름/라인/역사/위치 — device 를 station line + station display_name + place display_name 과 조인) + 검색 필터 + 행 선택으로 렌더링된다.
- **And** 동일 picker 가 Schedule View 와 대시보드 Trigger 패널 양쪽(공유 모달)에서 동작하며, 저장되는 `TargetSpec` shape 은 이전과 동일하다.

## FIX 검증 — M5/M6 버그수정 (§8.0, as-fixed)

- **FIX-1**: 규칙 추가 모달에서 선택된 xsfm 에이전트의 TARGET 이 열거된다(미지정 노드도 에이전트 선택 후 대상 표시). 대시보드 패널 저장 동작 무회귀.
- **FIX-2**: 스케줄 0개 트리거 노드가 관리 뷰에 표시되지 않는다.
- **FIX-3**: 스케줄 삭제가 백엔드 왕복으로 일관 영속됨(에디터 잔존은 크로스-서피스 staleness, 영속 버그 아님).

## Definition of Done

- [ ] AC-1 ~ AC-18 전부 통과 (M1~M7)
- [ ] AC-19 ~ AC-28 + FIX-1~3 전부 통과 (0.3.0 확장, as-implemented green)
- [ ] RD-5~11(전 OQ 확정) 반영 확인
- [ ] 3건 amendment(RD-11 Clear / RD-3·RD-7 플랫 테이블 / RD-3·RD-9 agent 자동 도출) spec.md §9 기록 확인 (SPEC v0.3.0)
- [ ] 트리거 노드 + 설비 예약 대시보드 패널 무회귀 확인
- [ ] 신설 저장소/API/페이지 문서화(sync 단계)
