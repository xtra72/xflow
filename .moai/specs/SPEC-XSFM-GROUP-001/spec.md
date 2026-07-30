---
id: SPEC-XSFM-GROUP-001
title: "xsfm 그룹 1급 개념 도입 (그룹 엔티티 · 다대다 멤버십 · 일괄 제어)"
version: "0.2.0"
status: draft
created: 2026-07-30
updated: 2026-07-30
author: xtra
priority: P2
phase: "v0.4.0 target"
module: "internal/agent/xsfm"
lifecycle: spec-anchored
tier: M
tags: "xsfm, group, group-registry, membership, many-to-many, bulk-control, fan-out, station, line, facility, frontend"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-07-30 | 0.1.0 | 초기 SPEC 작성 — xsfm 에 **그룹(group)을 1급 개념으로 도입**. (1) 그룹 엔티티 `{id, name, type: line\|station\|custom, members}` + 별도 그룹 레지스트리 신설(비침습 레이어 추가, 기존 station/line 레지스트리 불변), (2) 디바이스 다대다 멤버십(단일 `Device.GroupID` → 다중 소속 확장), (3) 커스텀 그룹 CRUD, (4) 라인/역사 추가 시 대응 기본 그룹 자동 생성·동기화, (5) 그룹 셀렉터 일괄 제어(기존 fan-out 재사용), (6) 프런트엔드 그룹 탭 추가 + 설비 역사 패널 → 설비 그룹 패널 개편 |
| 2026-07-30 | 0.2.0 | **4개 열린 질문(OQ) 사용자 확정 반영 → SPEC 확정**. RD-1(=OQ-1) `Device.GroupID` primary group 유지(단일 태그 방출 무회귀), RD-2(=OQ-2) 그룹 id 타입 접두사 인코딩(`station:`/`line:`/`custom:`), RD-3(=OQ-3) 조회 시 로스터 대조 필터(유령 멤버 자동 무시, remove_device 비침습), RD-4(=OQ-4) `line`/`station` 셀렉터 ↔ `group_id` 그룹 셀렉터 병존(동일 경로 수렴). "열린 질문" 섹션 → "확정된 설계 결정" 전환. REQ-02-07(primary 유지) 추가, REQ-05-06(셀렉터 병존) 추가, §4.5 primary 선정/갱신 규칙 추가 |

---

# SPEC-XSFM-GROUP-001: xsfm 그룹 1급 개념 도입

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이며, `internal/agent/xsfm` 는 지하철 역사 설비(facility) 를 MQTT 로 제어·모니터링하는 에이전트이다(SPEC-XSFM-001 에서 정의). 본 SPEC 은 그 위에 **그룹(group) 을 1급(first-class) 개념으로 도입**하여, 사용자가 여러 디바이스를 묶어 관리·일괄 제어하고 UI 로 그룹을 다룰 수 있게 한다.

현재 그룹은 **디바이스 속성**(`Device.GroupID`, 디바이스당 단일 자유 문자열 태그) 으로만 존재하며, 명시적 그룹 엔티티·CRUD 가 없다(SPEC-XSFM-001 A-4 가정). 본 SPEC 은 이 가정을 **의도적으로 갱신**하여, 그룹을 이름·타입·멤버를 가진 엔티티로 승격하고, 디바이스가 여러 그룹에 동시 소속(다대다)할 수 있게 한다.

### 1.2 기술 환경

- **언어/모듈**: Go 1.23+ / `internal/agent/xsfm/` (기존 패키지 확장)
- **프런트엔드**: React + TypeScript (`web/src/`) — `useStation.ts` 훅, `XsfmDevicesTab`/`XsfmStationsTab` 탭, `FacilityStationPanel`/`FacilityLinePanel`/`facilityShared.tsx` 대시보드 패널
- **관련 기존 anchor (본 SPEC 이 정확히 참조·재사용)**:
  - 셀렉터 fan-out: `control.go` `dispatchControl`/`handleSelectorControl`, `group.go` `fanOutControl`/`buildControlPlan`/`aggregateStatus`, 셀렉터 3차원(station/line/group_id), 우선순위 `device_id > station > line > group_id`
  - 멤버 도출: `agent.go` `GroupMembers(groupID)`(현재 `dev.GroupID == groupID` 순회), `station_registry.go` `DevicesByStation`/`DevicesByLine`
  - 역사 레지스트리: `station_registry.go` `StationRegistry`(station→line SSOT, 자체 RWMutex, write-through 영속), `handleAddStation`/`handleRemoveStation`/`handleListStations`
  - 영속화: `persist.go` `deviceRegistryStore`(단일 JSON + tmp+rename atomic write), `storage.StationRegistryFileRepository`
  - 명령 라우팅: `agent.go` `Process()` switch, `processRequest.fillFromParams()` (params backfill), 이미 예약된 미구현 명령 `set_group`
  - 센티널 에러: `errors.go` (`ErrEmptyGroup`, `ErrInvalidCommand`, `ErrDeviceNotFound`, `ErrStationNotFound` 등)
  - 텔레메트리/이벤트의 group_id 방출: `status.go`, `monitor.go`, `provider.go`
- **테스트**: Go 표준 `testing` + `testify`; 프런트 `vitest`

### 1.3 설계 원칙

- **비침습 레이어 추가(핵심)**: 기존 station→line 레지스트리(`station_registry.go`)·디바이스 로스터(`device.go`/`persist.go`)·fan-out(`control.go`/`group.go`)의 **구조를 바꾸지 않고**, 그룹 레지스트리를 **별도 레이어로 신설**한다. 기존 station/line 기능은 무회귀여야 한다.
- **기존 패턴 준수**: 그룹 레지스트리는 `StationRegistry` 의 락 규율(자체 RWMutex, 로스터/pending 락과 절대 중첩 금지)과 영속 패턴(캐시 + write-through, atomic write)을 그대로 따른다.
- **fan-out 재구현 금지**: 그룹 일괄 제어는 기존 `fanOutControl`/`controlDevice` 경로를 재사용하며, 그룹 레이어는 **대상(member) 집합 도출**만 담당한다(SPEC-XSFM-001 REQ-04-08 원칙 계승).
- **파생 vs 명시 멤버십 분리**: 기본 그룹(line/station)의 멤버십은 기존 `DevicesByStation`/`DevicesByLine` 로 **파생**하여 항상 최신 상태로 자동 동기화하고, 커스텀 그룹의 멤버십만 **명시적으로 저장**한다.

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - 그룹 엔티티 `{id, name, type: line|station|custom, members: []device_id}` + 그룹 레지스트리(신설, 별도 영속 저장소)
  - 디바이스 다대다 멤버십 모델(단일 `Device.GroupID` 확장) + 기존 데이터 마이그레이션 방침
  - 커스텀 그룹 CRUD: `add_group`/`remove_group`/`set_group`/`list_groups` + 멤버 추가/제거
  - 라인/역사 추가·삭제·이름변경 시 대응 기본 그룹(type=line/type=station) 자동 생성·동기화
  - 그룹 셀렉터(group_id) 일괄 제어 — 그룹 타입별 멤버 도출 후 기존 fan-out 재사용
  - 프런트엔드: xsfm 에이전트 상세에 그룹 탭 신설(그룹 CRUD + 멤버 편집 + 일괄 제어 UI), 훅 추가
  - 대시보드: "설비 역사 패널" → "설비 그룹 패널" 개편(역사를 그룹의 한 종류로 표시)
- **범위 외(Out-of-Scope)**:
  - 설비 이외 디바이스 타입 (SPEC-XSFM-001 범위 계승)
  - MQTT 토픽/페이로드 스키마 변경 (그룹은 순수 논리 레이어 — 브로커 통신 규약 불변)
  - 그룹 계층(그룹 안의 그룹, 중첩 그룹) — 본 SPEC 은 평면 그룹만
  - 하드웨어 브로드캐스트 그룹 제어 (에이전트 측 per-device fan-out 만; SPEC-XSFM-001 계승)
  - station→line 레지스트리 자체의 재설계 (그대로 유지)

---

## 2. Assumptions (가정)

### A-1. 그룹은 1급 엔티티 (SPEC-XSFM-001 A-4 갱신)

SPEC-XSFM-001 A-4 는 "그룹은 디바이스 속성(단일 group_id 태그)이며 별도 그룹 엔티티 테이블을 두지 않는다"고 가정했다. 본 SPEC 은 이 가정을 **명시적으로 대체**한다: 그룹은 `id`/`name`/`type`/`members` 를 갖는 1급 엔티티이며, 그룹 레지스트리가 커스텀 그룹 멤버십의 SSOT 이다.

### A-2. 파생 멤버십 (기본 그룹) vs 명시 멤버십 (커스텀 그룹)

`type=station`/`type=line` 기본 그룹의 멤버는 저장하지 않고 기존 `DevicesByStation`/`DevicesByLine`(Device.Station + station 레지스트리) 로 **런타임 파생**한다. 이로써 디바이스의 위치 속성이 바뀌면 기본 그룹 멤버십이 자동으로 최신화된다. 커스텀 그룹의 멤버만 그룹 레지스트리에 명시적으로 저장한다.

### A-3. 다대다 멤버십 (primary group + 확장 레이어)

한 디바이스는 자신의 라인 그룹 + 역사 그룹 + 임의 개수의 커스텀 그룹에 **동시 소속**할 수 있다. 단일 `Device.GroupID` 문자열로는 다중 소속을 표현할 수 없으므로, 다대다 멤버십은 **그룹 레지스트리를 별도 레이어로 추가**하여 표현한다. **`Device.GroupID` 단일 필드는 폐기하지 않고 "대표(primary) 그룹"으로 유지**하며(OQ-1 확정), 이는 디바이스가 소속된 여러 그룹 중 하나를 가리킨다. primary 는 텔레메트리/이벤트/InfluxDB 의 단일 `group_id` 태그 방출 경로(status.go/monitor.go/provider.go)를 **무회귀**로 보존한다. primary 선정/갱신 규칙은 §4.6 에 명시한다.

### A-4. 마이그레이션 (기존 group_id → primary + 커스텀 그룹, OQ-1 확정)

기존 `Device.GroupID`(단일 태그) 데이터는 **로드 시 1회** 처리한다: (1) `Device.GroupID` 값을 그대로 **primary 그룹**으로 유지하고, (2) 동명의 커스텀 그룹을 그룹 레지스트리에 보장 생성하여 디바이스를 그 멤버로 편입한다(§4 REQ-02-04). primary 는 단일 태그 방출 경로를 계속 담당하고, 다대다 소속은 그룹 레지스트리가 담당한다. 별도의 하위호환 alias 코드는 추가하지 않는다(레이어 추가로 해결).

### A-5. 그룹 셀렉터의 보편화 + 접두사 id (OQ-2/OQ-4 확정)

그룹 셀렉터(`group_id`)는 커스텀 그룹뿐 아니라 기본 그룹(station/line)도 addressing 할 수 있어야 한다(사용자 확정: "커스텀/기본 그룹 모두 group 셀렉터로 일괄 제어"). 그룹 `id` 는 **타입 접두사 인코딩**을 사용한다(OQ-2 확정): `station:<code>` / `line:<code>` / `custom:<name|uuid>`. 접두사만으로 그룹 타입을 즉시 판별하므로 자동 그룹과 커스텀 그룹의 id 충돌이 원천 차단된다. `GroupMembers(groupID)` 는 접두사로 타입을 분기하여 멤버를 도출한다. 또한 기존 `line`/`station` 셀렉터와 `group_id=line:<code>`/`station:<code>` 그룹 셀렉터는 **둘 다 허용**되며(OQ-4 확정), 동일한 `DevicesByLine`/`DevicesByStation` 경로로 수렴하여 결과가 동일하다.

### A-6. fan-out·제어 의미론 불변

그룹 일괄 제어의 제어 의미론(2-축, 값 검증, 응답 대기, best-effort, 집계 응답)은 SPEC-XSFM-001 M13/M4 에서 확정된 것을 **그대로** 사용한다. 본 SPEC 은 대상 집합 도출 경로만 그룹 레지스트리로 확장한다.

---

## 3. Requirements (요구사항 — EARS)

> ID 체계: `REQ-XSFM-GROUP-001-{모듈}-{순번}`. 인수 기준의 상세 Given-When-Then 은 `acceptance.md` 참조.

### Module 1 — 그룹 엔티티 · 그룹 레지스트리 (신설 레이어)

- **REQ-XSFM-GROUP-001-01-01** (Ubiquitous): 시스템은 **항상** 그룹을 엔티티 `{id, name, type, members}` 로 표현해야 한다. `type ∈ {line, station, custom}`, `members` 는 device_id 목록이다.
- **REQ-XSFM-GROUP-001-01-02** (Ubiquitous): 시스템은 그룹 레지스트리를 기존 station→line 레지스트리·디바이스 로스터와 **별개의 레이어/저장소**로 유지해야 한다. 기존 두 레지스트리의 자료구조·API·영속 포맷은 변경하지 **않아야 한다**.
- **REQ-XSFM-GROUP-001-01-03** (State-driven): **IF** 그룹 `type=custom` **THEN** 시스템은 `members` 를 그룹 레지스트리에 명시적으로 저장하고 SSOT 로 취급해야 한다.
- **REQ-XSFM-GROUP-001-01-04** (State-driven): **IF** 그룹 `type ∈ {line, station}` **THEN** 시스템은 `members` 를 저장하지 않고 조회 시점에 `DevicesByLine`/`DevicesByStation` 로 파생해야 한다.
- **REQ-XSFM-GROUP-001-01-05** (Ubiquitous): 시스템은 그룹 `id` 를 **타입 접두사 인코딩**으로 유일하게 유지해야 한다(OQ-2 확정) — `station:<code>` / `line:<code>` / `custom:<name|uuid>`. 접두사만으로 그룹 타입을 판별 가능해야 하며, 이를 통해 기본 그룹과 커스텀 그룹 간 id 충돌을 원천 차단한다.
- **REQ-XSFM-GROUP-001-01-06** (Event-driven): **WHEN** 그룹 CRUD 로 레지스트리가 변경되면 **THEN** 시스템은 커스텀 그룹 스냅샷을 저장소에 atomic 하게 write-through 해야 한다(기본 그룹은 파생이므로 멤버를 저장하지 않음).
- **REQ-XSFM-GROUP-001-01-07** (Ubiquitous): 그룹 레지스트리의 락은 로스터 락(`agent.mu`)·pending 락·station 레지스트리 락과 **완전히 분리된 자체 RWMutex** 여야 하며, 절대 중첩하지 않아야 한다(프로젝트 RWMutex 재진입 deadlock 트랩 회피).

### Module 2 — 다대다 멤버십 모델

- **REQ-XSFM-GROUP-001-02-01** (Ubiquitous): 시스템은 한 디바이스가 여러 그룹(라인 그룹 + 역사 그룹 + 다수 커스텀 그룹)에 동시 소속되는 것을 허용해야 한다.
- **REQ-XSFM-GROUP-001-02-02** (Event-driven): **WHEN** 사용자가 디바이스를 커스텀 그룹에 추가/제거하면 **THEN** 시스템은 그룹 레지스트리의 해당 그룹 `members` 를 갱신하고 즉시 조회에 반영해야 한다.
- **REQ-XSFM-GROUP-001-02-03** (Ubiquitous): 시스템은 특정 디바이스가 속한 전체 그룹 목록을 조회하는 역방향 질의를 제공해야 한다(멤버십은 그룹→디바이스 저장, 디바이스→그룹은 도출).
- **REQ-XSFM-GROUP-001-02-04** (Event-driven): **WHEN** 기존 단일 `Device.GroupID` 값을 가진 디바이스를 로드하면 **THEN** 시스템은 (1) 해당 값을 **primary 그룹**으로 유지하고(OQ-1), (2) 동명의 `custom:<value>` 그룹을 1회 보장 생성하여 디바이스를 그 멤버로 편입해야 한다(A-4). 다중 소속은 그룹 레지스트리가, 단일 태그 방출은 primary 가 담당한다.
- **REQ-XSFM-GROUP-001-02-05** (State-driven): **IF** `GroupMembers(groupID)` 가 호출되면 **THEN** 시스템은 그룹 id 의 타입 접두사에 따라 멤버를 도출해야 한다 — `custom:`→저장된 `members`(로스터 대조 필터 적용, REQ-02-06), `station:`→`DevicesByStation`, `line:`→`DevicesByLine`.
- **REQ-XSFM-GROUP-001-02-06** (Unwanted): 시스템은 그룹 멤버 조회/fan-out 시 **현재 로스터에 존재하는 device_id 만 통과**시켜 유령 멤버(삭제된 device_id)를 자동으로 무시해야 한다(OQ-3 확정, 조회 시 로스터 대조 필터). 이 방식은 `remove_device` 경로에 멤버십 연쇄 제거 로직을 요구하지 않아 비침습적이다.
- **REQ-XSFM-GROUP-001-02-07** (Ubiquitous): 시스템은 `Device.GroupID` 를 디바이스의 **대표(primary) 그룹**으로 유지해야 하며(OQ-1), 이를 통해 텔레메트리/이벤트/InfluxDB 단일 `group_id` 태그 방출 경로(status.go/monitor.go/provider.go)를 무회귀로 보존해야 한다. primary 선정/갱신 규칙은 §4.6 을 따른다.

### Module 3 — 커스텀 그룹 CRUD

- **REQ-XSFM-GROUP-001-03-01** (Event-driven): **WHEN** `add_group` 명령을 받으면 **THEN** 시스템은 `type=custom` 그룹을 생성하고(name + 초기 members 선택) `group_registered` 이벤트를 방출하고 영속화해야 한다.
- **REQ-XSFM-GROUP-001-03-02** (Event-driven): **WHEN** `remove_group` 명령을 받으면 **THEN** 시스템은 해당 커스텀 그룹을 제거하고 `group_unregistered` 이벤트를 방출하고 영속화해야 한다.
- **REQ-XSFM-GROUP-001-03-03** (Event-driven): **WHEN** `set_group` 명령을 받으면 **THEN** 시스템은 그룹의 name 및/또는 members(부분 갱신)를 수정해야 한다. 제공되지 않은 필드는 보존한다.
- **REQ-XSFM-GROUP-001-03-04** (Event-driven): **WHEN** `list_groups` 명령을 받으면 **THEN** 시스템은 전체 그룹(기본+커스텀)을 `{id, name, type, member_count, members}` 로 결정적 순서(예: type→name)로 반환해야 한다.
- **REQ-XSFM-GROUP-001-03-05** (Unwanted): 시스템은 기본 그룹(type=line/station)을 `remove_group`/멤버 수동 편집으로 변경·삭제하지 **않아야 한다**(기본 그룹은 station/line 레지스트리·디바이스 위치가 SSOT). 시도 시 명확한 에러(예: `ErrGroupNotCustom`)를 반환한다.
- **REQ-XSFM-GROUP-001-03-06** (Unwanted): 시스템은 존재하지 않는 그룹에 대한 `remove_group`/`set_group` 을 명확한 에러(예: `ErrGroupNotFound`)로 거부해야 하며, 어떠한 부분 변경도 남기지 않아야 한다.

### Module 4 — 기본 그룹 자동 동기화 (라인/역사 ↔ 그룹)

- **REQ-XSFM-GROUP-001-04-01** (Event-driven): **WHEN** `add_station`(역사 추가/갱신)이 성공하면 **THEN** 시스템은 대응하는 `type=station` 기본 그룹을 자동 생성/갱신해야 한다(그룹 name 은 역사 표시명, 참조는 station 코드).
- **REQ-XSFM-GROUP-001-04-02** (Event-driven): **WHEN** `remove_station` 이 성공하면 **THEN** 시스템은 대응하는 `type=station` 기본 그룹을 제거해야 한다.
- **REQ-XSFM-GROUP-001-04-03** (Event-driven): **WHEN** 역사 표시명이 변경되면 **THEN** 시스템은 대응 station 그룹의 name 을 동기화해야 한다.
- **REQ-XSFM-GROUP-001-04-04** (State-driven): **IF** station→line 매핑에 어떤 호선(line)이 하나 이상의 역사로 존재하면 **THEN** 시스템은 그 호선에 대응하는 `type=line` 기본 그룹을 노출해야 한다. 호선의 마지막 역사가 제거되면 line 그룹도 사라진다(line 은 역사에서 파생되므로 line 그룹 생애도 파생).
- **REQ-XSFM-GROUP-001-04-05** (Ubiquitous): 기본 그룹의 멤버는 어떤 시점에도 저장된 목록이 아니라 파생 결과여야 하므로(REQ-01-04), 디바이스 위치 속성 변경이 즉시 기본 그룹 멤버십에 반영되어야 한다.

### Module 5 — 그룹 셀렉터 일괄 제어

- **REQ-XSFM-GROUP-001-05-01** (Event-driven): **WHEN** `group_id` 셀렉터로 제어 명령(set_power/set_fan_speed/set_multiple)을 받으면 **THEN** 시스템은 그룹 레지스트리에서 그룹을 조회하여 타입별로 멤버를 도출하고(REQ-02-05), 기존 `fanOutControl` 경로로 멤버별 제어를 fan-out 해야 한다.
- **REQ-XSFM-GROUP-001-05-02** (Ubiquitous): 그룹 일괄 제어는 기존 제어 의미론(값 검증 1회 선수행, 2-축, 멤버별 응답 대기, best-effort, 집계 응답 `{selector, results, status}`)을 그대로 재사용해야 한다(재구현 금지, A-6).
- **REQ-XSFM-GROUP-001-05-03** (State-driven): **IF** 셀렉터가 여럿 지정되면 **THEN** 시스템은 기존 우선순위 `device_id > station > line > group_id` 를 유지해야 한다. 다대다 멤버십은 대상 도출에만 영향을 주고 셀렉터 우선순위/디스패치는 불변이다.
- **REQ-XSFM-GROUP-001-05-04** (Unwanted): **IF** group_id 셀렉터가 멤버 없는 그룹을 가리키면 **THEN** 시스템은 어떠한 명령도 방출하지 않고 `ErrEmptyGroup` 을 반환해야 한다(기존 동작 계승).
- **REQ-XSFM-GROUP-001-05-05** (Unwanted): **IF** group_id 셀렉터가 미등록 그룹을 가리키면 **THEN** 시스템은 `ErrGroupNotFound` 로 거부해야 한다.
- **REQ-XSFM-GROUP-001-05-06** (State-driven): **IF** 동일 대상을 기존 `line`/`station` 셀렉터로 지정하거나 `group_id=line:<code>`/`station:<code>` 그룹 셀렉터로 지정하면 **THEN** 시스템은 두 경로를 모두 허용하고(OQ-4 확정) 동일한 `DevicesByLine`/`DevicesByStation` 로 수렴시켜 **동일한 결과**를 내야 한다. 기존 `line`/`station` 셀렉터 동작은 무회귀여야 한다.

### Module 6 — 프런트엔드 (그룹 탭 + 패널 개편)

- **REQ-XSFM-GROUP-001-06-01** (Ubiquitous): xsfm 에이전트 상세 화면은 **그룹 탭**을 제공해야 한다(기존 디바이스 탭/역사 탭과 나란히).
- **REQ-XSFM-GROUP-001-06-02** (Event-driven): **WHEN** 사용자가 그룹 탭에서 그룹을 생성/수정/삭제하면 **THEN** UI 는 대응 명령(add_group/set_group/remove_group)을 호출하고 목록을 갱신해야 한다.
- **REQ-XSFM-GROUP-001-06-03** (Event-driven): **WHEN** 사용자가 그룹의 멤버(디바이스)를 편집하면 **THEN** UI 는 `set_group` 으로 멤버 목록을 갱신해야 한다(커스텀 그룹만; 기본 그룹은 읽기 전용 표시).
- **REQ-XSFM-GROUP-001-06-04** (Event-driven): **WHEN** 사용자가 그룹을 대상으로 일괄 제어를 실행하면 **THEN** UI 는 `group_id` 셀렉터로 제어 명령을 호출하고 집계 결과를 표시해야 한다(기존 `FacilityBulkControl`/`ControlResultView` 재사용).
- **REQ-XSFM-GROUP-001-06-05** (Ubiquitous): 대시보드의 "설비 역사 패널"은 "**설비 그룹 패널**"로 개편되어, 역사를 그룹의 한 종류(type=station)로 표시하고 커스텀/라인 그룹도 함께 표시·제어할 수 있어야 한다.
- **REQ-XSFM-GROUP-001-06-06** (Optional): 가능하면 그룹 목록에 멤버 수·온라인 요약(기존 `StatTiles`/`facilityAggregation` 재사용)을 제공한다.

### 비기능 요구 (NFR)

- **REQ-XSFM-GROUP-001-NF-01** (Ubiquitous): 커스텀 그룹 레지스트리는 재시작을 넘어 영속되어야 하며(atomic write), 기본 그룹은 재시작 후 station/line 레지스트리로부터 재파생되어야 한다.
- **REQ-XSFM-GROUP-001-NF-02** (Unwanted): 그룹 레이어 도입이 기존 station/line 레지스트리·디바이스 로스터·기존 fan-out(station/line 셀렉터)의 동작을 **회귀시키지 않아야 한다**(기존 테스트 전부 통과).
- **REQ-XSFM-GROUP-001-NF-03** (Ubiquitous): 다대다 멤버십·기본 그룹 파생·커스텀 그룹 저장이 서로 정합해야 한다 — 동일 디바이스가 여러 그룹 조회에서 일관되게 나타나고, 위치 변경이 파생 그룹에 즉시 반영된다.
- **REQ-XSFM-GROUP-001-NF-04** (Ubiquitous): 동시성 안전 — 그룹 레지스트리 읽기/쓰기와 fan-out 동시 실행이 race 없이 동작해야 한다(자체 락 + 스냅샷 후 락 해제 패턴).

---

## 4. Specifications (설계 명세)

### 4.1 그룹 엔티티

```
Group {
  ID       string        // 그룹 식별자 — 타입 접두사 인코딩 (OQ-2): "station:<code>" | "line:<code>" | "custom:<name|uuid>"
  Name     string        // 표시 이름
  Type     string        // "line" | "station" | "custom" (ID 접두사에서 파생 판별 가능)
  Ref      string        // type=station→station 코드, type=line→line id, type=custom→"" (파생 그룹의 원천 참조)
  Members  []string       // device_id 목록 — type=custom 에서만 저장/권위, line/station 은 조회 시 파생
}
```

### 4.2 멤버 도출 (`GroupMembers` 접두사 분기 + 로스터 대조 필터)

id 의 타입 접두사로 분기하며, **모든 반환 경로는 현재 로스터에 존재하는 device_id 만 통과**시킨다(OQ-3 유령 멤버 필터).

- `custom:`: 저장된 `Members` → 로스터 대조 필터 후 반환.
- `station:`: `DevicesByStation(Ref)` 재사용(로스터에서 도출되므로 이미 실재 device_id).
- `line:`: `DevicesByLine(Ref)` 재사용(미등록 station 참조는 excluded 처리 계승).

### 4.3 자동 동기화 훅 지점

- `handleAddStation` 성공 직후 → `station:<code>` 그룹 upsert(name=표시명).
- `handleRemoveStation` 성공 직후 → `station:<code>` 그룹 remove.
- line 그룹(`line:<code>`)은 저장하지 않고 `list_groups`/조회 시 station 레지스트리의 line 집합에서 파생 노출.

### 4.4 마이그레이션 (기존 group_id → primary + 커스텀 그룹, OQ-1)

로드(Init) 시 로스터를 순회하여 비어있지 않은 `Device.GroupID` 값마다: (1) 그 값을 **primary 그룹으로 유지**(필드 폐기하지 않음), (2) `custom:<value>` 그룹을 보장 생성하고 device_id 를 멤버로 편입. 이후 다중 소속은 그룹 레지스트리가, 단일 태그 방출은 primary 가 담당한다.

### 4.5 primary 그룹 선정/갱신 규칙 (OQ-1)

`Device.GroupID` = 대표(primary) 그룹으로 유지하며 텔레메트리/이벤트/InfluxDB 단일 `group_id` 태그 방출(status.go/monitor.go/provider.go)에 계속 사용된다. 선정/갱신 규칙:

- **초기/마이그레이션**: 기존 `Device.GroupID` 값을 그대로 primary 로 유지(위 §4.4).
- **커스텀 그룹 편입 시**: 디바이스가 primary 를 아직 갖지 않았고(빈 값) 커스텀 그룹에 처음 편입되면, 그 그룹을 primary 로 설정(선택적; 이미 primary 가 있으면 유지).
- **명시적 갱신**: `set_device{group_id}` 로 사용자가 primary 를 직접 지정/변경(기존 경로 무변경).
- **primary 그룹 삭제 시**: primary 가 가리키던 커스텀 그룹이 삭제되면 primary 는 빈 값으로 리셋(단일 태그는 미소속으로 방출).
- primary 는 다중 소속의 부분집합(그 중 하나)일 뿐이며, fan-out 대상 도출은 primary 가 아니라 그룹 레지스트리/파생 경로가 담당한다.

### 4.6 Traceability (추적성)

| 요구 | 설계/코드 anchor | 인수 시나리오 |
| --- | --- | --- |
| 01-01~07 | 신설 `group_registry.go` (Group, GroupRegistry, 접두사 id) | AC 1.x |
| 02-01~07 | GroupRegistry.members, `GroupMembers`(접두사 분기+로스터 필터), Init 마이그레이션, primary 유지 | AC 2.x |
| 03-01~06 | `Process()` switch (add_group/remove_group/set_group/list_groups) | AC 3.x |
| 04-01~05 | `handleAddStation`/`handleRemoveStation` 동기화 훅, line 파생 | AC 4.x |
| 05-01~06 | `handleSelectorControl`→접두사 분기, `fanOutControl` 재사용, line/station↔group_id 병존 | AC 5.x |
| 06-01~06 | `web/` 그룹 탭·훅·패널 개편 | AC 6.x |
| NF-01~04 | 영속 저장소, 락 규율, 회귀 테스트, primary 방출 무회귀 | AC 7.x |

---

## 5. 확정된 설계 결정 (Resolved Decisions)

> 아래 4개 결정은 사용자 확정으로 본문 전반에 baked-in 되었다(Assumptions A-3~A-5, REQ-01-05/02-04/02-06/02-07/05-06, §4.1~4.5). 미해결 열린 질문은 없다.

- **RD-1 (= OQ-1) primary group 유지**: `Device.GroupID` 단일 필드를 폐기하지 않고 "대표(primary) 그룹"으로 유지한다. 텔레메트리/이벤트/InfluxDB 단일 `group_id` 태그 방출 경로(status.go/monitor.go/provider.go)를 무회귀로 보존하며, 다대다 멤버십은 그룹 레지스트리를 별도 레이어로 추가해 표현한다. primary 는 다중 소속 중 하나를 가리키고 선정/갱신 규칙은 §4.5. 근거: 단일 태그 방출 경로 무회귀 + 레이어 추가 비침습.
- **RD-2 (= OQ-2) 접두사 방식 id**: 그룹 id 를 타입 접두사로 인코딩한다 — `station:<code>` / `line:<code>` / `custom:<name|uuid>`. 자동 그룹·커스텀 그룹을 id 만으로 구분하며, 충돌을 원천 차단하고 `GroupMembers` 분기를 접두사 기반으로 단순화한다. 근거: group_id 셀렉터가 접두사로 타입 즉시 판별.
- **RD-3 (= OQ-3) 조회 시 로스터 대조 필터**: 그룹 멤버 조회/fan-out 시 현재 로스터에 존재하는 device_id 만 통과시켜 유령 멤버(삭제된 device_id)를 자동 무시한다. `remove_device` 경로에 멤버십 연쇄 제거 로직이 불필요하여 비침습적이다. 근거: 삭제 경로를 그룹 레지스트리와 결합하지 않음.
- **RD-4 (= OQ-4) 셀렉터 병존(둘 다 허용)**: 기존 `line`/`station` 셀렉터와 `group_id=line:<code>`/`station:<code>` 그룹 셀렉터를 둘 다 허용하며, 동일한 `DevicesByLine`/`DevicesByStation` 경로로 수렴시켜 결과가 동일하다. 근거: 기존 line/station 셀렉터 무회귀 + 그룹 UI 는 group_id 로 통일.
