---
id: SPEC-XSFM-GROUP-001
title: "xsfm 그룹 1급 개념 도입 — 구현 계획"
version: "0.3.0"
status: completed
created: 2026-07-30
updated: 2026-07-30
author: xtra
priority: P2
phase: "v0.4.0 target"
module: "internal/agent/xsfm"
lifecycle: spec-anchored
tier: M
tags: "xsfm, group, plan, milestones, membership, fan-out, frontend"
---

# SPEC-XSFM-GROUP-001 구현 계획: xsfm 그룹 1급 개념 도입

> 시간 예측을 사용하지 않는다. 마일스톤은 **우선도·의존 순서**로만 표현한다.

## 1. 작업 분해 (우선도 기반 마일스톤)

의존 순서: 백엔드 그룹 레지스트리/멤버십(M1) → 마이그레이션(M2) → 커스텀 CRUD(M3) → 자동 동기화(M4) → 그룹 일괄제어(M5) → 프런트 그룹 탭(M6) → 패널 개편(M7). 각 백엔드 마일스톤은 자체 단위 테스트를 동반한다.

### Primary Goal (M1): 그룹 레지스트리 + 멤버십 도출 (신설 레이어)

- 우선도: High
- 신설 `group_registry.go`: `Group{ID,Name,Type,Ref,Members}`(id 는 접두사 인코딩 `station:`/`line:`/`custom:` — RD-2), `GroupRegistry`(자체 RWMutex + write-through 영속, `StationRegistry` 패턴 미러).
- 커스텀 그룹 CRUD 내부 API: `UpsertGroup`/`RemoveGroup`/`GetGroup`/`ListGroups`/`AddMember`/`RemoveMember`.
- `GroupMembers(groupID)` **재구현**: id 접두사 분기(`custom:`→저장 members, `station:`→`DevicesByStation`, `line:`→`DevicesByLine`) + **모든 경로 로스터 대조 필터로 유령 멤버 무시(RD-3)**. 기존 시그니처 유지(호출부 `handleSelectorControl` 무변경).
- 역방향 질의 `GroupsForDevice(deviceID)`.
- 센티널 에러 추가: `ErrGroupNotFound`, `ErrGroupNotCustom`, `ErrGroupAlreadyExists`.
- 완료 조건: REQ-01-01~07, 02-01/03/05 커버, 락 중첩 없음(레지스트리 자체 락).

### Secondary Goal (M2): 기존 group_id 마이그레이션 + primary 유지 (범위 축소)

- 우선도: High
- **RD-1 확정으로 범위 축소**: `Device.GroupID` 는 폐기하지 않고 **primary 그룹으로 유지** — status.go/monitor.go/provider.go 의 단일 `group_id` 태그 방출 경로를 **손대지 않는다**(무회귀). 방출 경로 재작성 작업이 제거되어 M2 범위가 줄어든다.
- Init(로스터 복원) 시 비어있지 않은 `Device.GroupID` → (1) primary 로 유지, (2) `custom:<value>` 그룹 보장 생성 + 멤버 편입(A-4, REQ-02-04).
- primary 선정/갱신 규칙 구현(§4.5): 초기 유지 / 커스텀 첫 편입 시 선택적 설정 / `set_device{group_id}` 명시 갱신 / primary 그룹 삭제 시 리셋.
- 완료 조건: 기존 group_id 데이터가 재시작 후 primary + 커스텀 그룹으로 일관 노출; 단일 태그 방출 무회귀(REQ-02-07).

### M3: 커스텀 그룹 CRUD 명령

- 우선도: High
- `agent.go` `Process()` switch 에 `add_group`/`remove_group`/`set_group`/`list_groups` 배선(기존 예약 `set_group` 미구현 스텁 대체).
- 핸들러 `handleAddGroup`/`handleRemoveGroup`/`handleSetGroup`/`handleListGroups`(부분 갱신은 `handleSetDevice` 의 present() 패턴 재사용).
- `group_registered`/`group_unregistered` 이벤트(`sendEvent` 재사용) + 영속화(`persistGroups`).
- 기본 그룹 보호(REQ-03-05: type≠custom 편집 거부 `ErrGroupNotCustom`).
- 완료 조건: REQ-03-01~06 커버.

### M4: 기본 그룹 자동 동기화 (라인/역사 ↔ 그룹)

- 우선도: Medium
- `handleAddStation`/`handleRemoveStation` 성공 직후 station 그룹 upsert/remove 훅(REQ-04-01~03). 역사 표시명 변경 → 그룹 name 동기화.
- line 그룹은 저장하지 않고 `list_groups`/`GroupMembers` 조회 시 station 레지스트리 line 집합에서 파생(REQ-04-04).
- 완료 조건: REQ-04-01~05 커버; station/line 레지스트리 자체는 불변(NF-02).
- **as-implemented (IN-1)**: station/line 그룹은 `handleAddStation`/`handleRemoveStation` 훅 삽입 없이 **조회 시점에 `StationRegistry` 로부터 순수 파생**하도록 구현되어 `station_registry.go` 를 **완전 불변**으로 유지했다(계획한 명시적 upsert/remove 훅보다 강한 비침습성). 역사 CRUD 는 station 레지스트리에만 반영되고 그룹 엔티티는 그때그때 파생되므로 AC 4.1~4.5 를 훅 없이 충족. 상세: spec.md §6 IN-1.

### M5: 그룹 셀렉터 일괄 제어

- 우선도: Medium
- `handleSelectorControl` 의 `group_id` 분기가 재구현된 `GroupMembers`(접두사 분기)를 사용 — `fanOutControl`/`buildControlPlan`/`aggregateStatus` 는 **무변경 재사용**(REQ-05-02).
- 미등록 그룹 → `ErrGroupNotFound`, 빈 그룹 → `ErrEmptyGroup`(REQ-05-04/05).
- 셀렉터 우선순위 `device_id > station > line > group_id` 불변 검증(REQ-05-03).
- **셀렉터 병존(RD-4)**: 기존 `line`/`station` 셀렉터와 `group_id=line:<code>`/`station:<code>` 그룹 셀렉터 둘 다 허용, 동일 `DevicesByLine`/`DevicesByStation` 로 수렴하여 결과 동일 검증(REQ-05-06).
- 완료 조건: REQ-05-01~06 커버; 기존 station/line fan-out 회귀 없음.

### M6: 프런트엔드 그룹 탭 + 훅

- 우선도: Medium
- 훅: `web/src/hooks/` 에 `useGroups`(list_groups)/`useAddGroup`/`useSetGroup`/`useRemoveGroup`(useStation.ts 의 add_station 패턴 미러).
- 신설 `XsfmGroupsTab.tsx`: 그룹 목록(type 배지 · 멤버 수) + 커스텀 그룹 생성/수정/삭제 + 멤버(디바이스) 편집(기본 그룹은 읽기 전용) + 그룹 일괄 제어(`FacilityBulkControl` 재사용, 셀렉터 `{group_id}`).
- 에이전트 상세 탭 바에 그룹 탭 등록.
- 완료 조건: REQ-06-01~04, 06-06 커버.

### Final Goal (M7): 설비 역사 패널 → 설비 그룹 패널 개편

- 우선도: Low
- `FacilityStationPanel` → 그룹 패널로 개편(역사를 type=station 그룹으로 표시, 커스텀/라인 그룹 함께 표시·제어). `facilityShared.tsx`/`facilityAggregation.ts`/`renderDashboardPanel` 정합 반영.
- 완료 조건: REQ-06-05 커버; 기존 패널 테스트 갱신·무회귀.
- **as-implemented (IN-2)**: 기존 `FacilityStationPanel` 을 파괴적으로 개편하지 않고 **신규 `FacilityGroupPanel` + `facility-group` 패널 타입을 가산(additive)**했다. 기존 역사 패널과 그 15개 단위 테스트는 그대로 보존(무회귀 근거 NF-02). 신규 그룹 패널이 역사(type=station)·커스텀·라인 그룹을 함께 표시·제어하므로 REQ-06-05 기능 목표를 충족(AC 6.5 = "설비 그룹 패널 추가"로 해석). 상세: spec.md §6 IN-2. (참고: 에이전트-레벨 그룹 로직은 신규 `group_membership.go` 에 배치되고 `handleSelectorControl` 은 본 코드베이스상 `group.go` 에 존재하여 `control.go` 는 불변 — IN-3.)

### Optional Goal: 회귀·정합 테스트 보강

- 우선도: Low
- 다대다 정합·파생 즉시 반영·동시성 race(`-race`) 테스트, 기존 xsfm 테스트 전체 통과 확인(NF-02~04).

## 2. 기술 접근 방식

### 2.1 아키텍처 (비침습 레이어 추가)

```
[기존 · 불변]                         [신설 · 본 SPEC]
station_registry (station→line SSOT) ──파생──▶ GroupRegistry
device roster (Device.Station/GroupID)          ├─ custom groups (members 저장·영속)
       │                                          ├─ station groups (DevicesByStation 파생)
       └── fan-out (control.go/group.go) ◀────────┴─ line groups (DevicesByLine 파생)
             (fanOutControl 재사용)
```

그룹 레지스트리는 station 레지스트리를 **읽기 전용으로 참조**(파생 멤버 도출)하고, 커스텀 멤버만 자체 저장소에 영속한다. fan-out 은 대상 집합만 그룹 레이어에서 받는다.

### 2.2 멤버십 모델 (다대다 + primary)

- 저장 방향: 그룹 → 디바이스(`Group.Members`, 커스텀만). 디바이스 → 그룹은 `GroupsForDevice` 로 도출.
- 기본 그룹은 저장 0 바이트(항상 파생) → 위치 변경 즉시 반영, 동기화 코드 불필요.
- 조회/fan-out 시 로스터 대조 필터로 유령 멤버 무시(RD-3) → `remove_device` 에 연쇄 제거 로직 불필요.
- `Device.GroupID` = **primary 그룹으로 유지(RD-1 확정)** → 단일 `group_id` 태그 방출(status.go/monitor.go/provider.go) 무회귀. 다중 소속은 그룹 레지스트리, 단일 태그는 primary. 선정/갱신 규칙 spec §4.5.

### 2.3 영속 스키마

- 커스텀 그룹만 `{dir}/group_registry.json`(device_registry.json 과 동일 dir, atomic tmp+rename). 포맷 예:
  ```json
  { "custom:floor2": { "id": "custom:floor2", "name": "2층", "type": "custom", "members": ["<uuid>", ...] } }
  ```
- 기본 그룹은 저장하지 않음(재시작 시 station 레지스트리로 재파생).

### 2.4 락 규율

- `GroupRegistry` 자체 RWMutex. 파생 멤버 도출 시 station 레지스트리 락 → 해제 → 로스터 락 순서(중첩 금지, `DevicesByLine` 기존 패턴 계승). fan-out 은 대상 스냅샷 후 락 해제하고 `controlDevice` 반복.

### 2.5 명령 API

| 명령 | params | 응답 |
| --- | --- | --- |
| `add_group` | `{name, members?}` | `{status, group_id}` |
| `remove_group` | `{group_id}` | `{status, group_id}` |
| `set_group` | `{group_id, name?, members?}` | `{status, group_id}` |
| `list_groups` | `{}` | `{status, groups:[{id,name,type,member_count,members}]}` |
| `set_power`/`set_fan_speed`/`set_multiple` | `{group_id, ...}` | 기존 fan-out 집계 응답 |

### 2.6 프런트엔드

- `useGroups` 훅 + `XsfmGroupsTab`(디바이스 탭 UI 패턴 재사용) + 그룹 패널 개편. 일괄 제어·집계 결과 표시는 `FacilityBulkControl`/`ControlResultView` 재사용.

## 3. 리스크 및 대응

> OQ-1~4 는 모두 확정(RD-1~4)되어 미결정 리스크에서 제외되었다. 아래는 확정 후 잔여 구현 리스크.

| 리스크 | 영향 | 대응 |
| --- | --- | --- |
| primary 그룹 선정/갱신 규칙 오구현 | 단일 태그 방출 값이 기대와 불일치 | §4.5 규칙 명세대로 구현 + primary 방출 무회귀 테스트(RD-1) |
| 접두사 파싱 엣지(콜론 포함 커스텀 name) | id 분기 오판 | `custom:` 접두사만 분리하고 나머지는 원문 보존; 파싱 규약 테스트(RD-2) |
| 로스터 대조 필터 누락 경로 | 유령 멤버 노출 | 모든 `GroupMembers` 반환 경로에 필터 적용 + 삭제 후 조회 테스트(RD-3) |
| 셀렉터 병존 결과 불일치 | line vs group_id 결과 상이 | 동일 `DevicesByLine`/`DevicesByStation` 로 수렴 + 결과 동일성 테스트(RD-4) |
| 기존 station/line·로스터·fan-out 회귀 | 핵심 기능 손상 | 비침습 레이어 추가 원칙 + 기존 테스트 전량 통과 게이트(NF-02) |
| 락 중첩 deadlock | 런타임 정지 | 레지스트리 자체 락 + 스냅샷 후 해제 패턴 엄수(REQ-01-07) |

## 4. TRUST 5 정합

- Tested: 각 마일스톤 단위 테스트 + `-race` + 기존 회귀 스위트.
- Readable: 기존 xsfm 파일 주석/명명 규약(한국어 code_comments) 준수.
- Unified: `gofmt`/`goimports`; 프런트 기존 컴포넌트 패턴 재사용.
- Secured: 입력 검증(그룹 id/name/members 유효성), 기본 그룹 편집 차단.
- Trackable: Conventional Commit + SPEC-XSFM-GROUP-001 참조.
