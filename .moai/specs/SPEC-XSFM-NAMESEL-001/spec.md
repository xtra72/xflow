---
id: SPEC-XSFM-NAMESEL-001
title: "xsfm 이름 기반 제어 셀렉터 (device_name / group_name)"
version: "0.3.0"
status: completed
created: 2026-07-30
updated: 2026-07-30
author: xtra
priority: P2
phase: "v0.5.0 target"
module: "internal/agent/xsfm"
lifecycle: spec-anchored
tier: M
tags: "xsfm, control, selector, device-name, group-name, name-resolver, ambiguity, fan-out, node-control, facility"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-07-30 | 0.1.0 | 초기 SPEC 작성 — 제어 명령에서 **이름 기반 셀렉터**(`device_name`/`group_name`)를 신설. (1) 신규 processRequest 필드 `DeviceName`/`GroupName`(json `device_name`/`group_name`) — CRUD `name` 필드와 별개, (2) 에이전트 이름 리졸버 `DeviceByName`/`GroupByName`(스냅샷-안전 락 규율), (3) 모호성 = 거부(≥2 매치 → `ErrAmbiguousName`, 무방출), 0 매치 → not-found(`ErrDeviceNotFound`/`ErrGroupNotFound`), 정확히 1개 → 진행, (4) 셀렉터 우선순위 확장(제안 `device_id > device_name > station > line > group_id > group_name`), (5) 노드 pass-through(buildXsfmControlCommand top-level 방출 + hasXsfmControlCommand 라우팅 + fillFromParams 승격), (6) 기존 셀렉터 무회귀. RD-1~3 사용자 확정 반영. 미해결 열린 질문 3건(OQ-1 우선순위 순서, OQ-2 그룹 이름 공간, OQ-3 case/trim). |
| 2026-07-30 | 0.2.0 | 열린 질문 3건(OQ-1~3) 사용자 확정 → RD-4~6 으로 승격하고 본문 baked-in, §6 Open Questions 제거(잔여 열린 질문 없음). **RD-4(우선순위)**: 셀렉터 우선순위 체인 = `device_id > device_name > station > line > group_id > group_name` 확정 — "개별 먼저"(device_id/device_name 개별 셀렉터가 집계/그룹 셀렉터에 선행), device_name 은 device_id 바로 뒤. **RD-5(group_name 이름 공간)**: `GroupByName` 은 **전 타입**(custom + 파생 station/line 그룹) 표시명 매칭 — 타입 간 충돌은 RD-2 의 `ErrAmbiguousName`(안전 거부)으로 처리. **RD-6(매칭 규칙)**: 공백 trim 후 정확 일치, **대소문자 구분**(case-sensitive) — 입력·비교 대상 Name 양쪽 trim, case folding 없음. §3 EARS(Module 1 리졸버 매칭·전 타입, Module 2 우선순위 체인)·§4 명세 정정. |
| 2026-07-30 | 0.3.0 | **구현 완료(M1~M3) — sync-phase 3-phase close.** 백엔드 M1(에이전트 리졸버 + 디스패치) + M2(노드 pass-through) + M3(테스트 + 무회귀 검증) 구현·커밋(`2acb980d`). 신규 `name_resolver.go`(`DeviceByName`/`GroupByName`, trim+대소문자 구분, ≥2→`ErrAmbiguousName`, 0→NotFound, 스냅샷-안전 락), `dispatchControl` 우선순위 체인 `device_id > device_name > station > line > group_id > group_name`(`handleIndividualControl` 추출), `GroupByName` 전 타입 매칭(신규 `allGroups()` 추출), 노드 `buildXsfmControlCommand` device_name/group_name top-level pass-through + `hasXsfmControlCommand` 라우팅 + `xsfmExtractDeviceName`/`GroupName`, 신규 `ErrAmbiguousName` 센티널. 검증: `go test ./...` exit 0(42 pkgs), 신규 함수 커버리지 **100%**, `-race` 클린, 무회귀. `status: draft → completed`(핵심 M1~M3 기준), `version: 0.2.0 → 0.3.0`. as-implemented 분기 4건은 §7 신설. **M4(프런트엔드 이름 제어 UI)는 선택·저우선(§plan)으로 이연** — 에이전트+노드 레벨에서 기능 완전 사용 가능, "completed" 는 핵심 기능(M1~M3)에 한함(§7.5 명시). |

---

# SPEC-XSFM-NAMESEL-001: xsfm 이름 기반 제어 셀렉터

## 1. Environment (환경)

### 1.1 시스템 개요

`internal/agent/xsfm` 는 지하철 역사 설비(facility)를 MQTT 로 제어·모니터링하는 Go 에이전트이다(SPEC-XSFM-001). 제어 명령의 대상은 현재 단일 `device_id`(개별) 또는 셀렉터(`station`/`line`/`group_id`)로만 지정 가능하다. 본 SPEC 은 사용자가 **사람이 읽는 이름**(`device_name`/`group_name`)으로도 제어 대상을 지정할 수 있도록 **이름 기반 셀렉터**를 신설한다.

사용자 요구: **"제어 명령에서 device_name 이나 group_name 으로도 제어 가능."**

### 1.2 기술 환경

- **언어/모듈**: Go 1.23+ / `internal/agent/xsfm/` (기존 패키지 확장) + `internal/node/xsfm.go` (노드 레이어 확장)
- **프런트엔드(선택)**: React + TypeScript (`web/src/`) — 이름 기반 제어 UI 는 후속(선택) 마일스톤
- **관련 기존 anchor (본 SPEC 이 정확히 참조·재사용)**:
  - 제어 디스패치: `group.go` `dispatchControl`(device_id 유무 라우팅) / `handleSelectorControl`(station→line→group_id 순차 해석) / `fanOutControl`(동시 fan-out + 집계 응답) — 현행 우선순위 `device_id > station > line > group_id`
  - 개별 제어: `control.go` `controlDevice`(락 하 스냅샷 → 해제 → 디스패치), `handleSetPower`/`handleSetFanSpeed`/`handleSetMultiple`
  - 멤버/이름 도출: `agent.go` `ListDevices()`(정렬 값 복사본), `group_membership.go` `GroupMembers(groupID)`(접두사 분기 + 로스터 필터), `GroupsForDevice`, `group_registry.go` `ListGroups()`
  - 명령 라우팅·backfill: `agent.go` `processRequest`(셀렉터 필드), `fillFromParams()`(params → top-level 승격, group_id 포함)
  - 센티널 에러: `errors.go` (`ErrDeviceNotFound`, `ErrGroupNotFound`, `ErrEmptyGroup`, `ErrInvalidCommand`)
  - 노드 레이어: `internal/node/xsfm.go` `buildXsfmControlCommand`(device_id + group_id top-level 방출), `hasXsfmControlCommand`(command|params|group_id 존재 → 제어 라우팅), `xsfmExtractDeviceID`/`xsfmExtractGroupID`
- **테스트**: Go 표준 `testing` + `testify`; 프런트 `vitest`

### 1.3 설계 원칙

- **비침습 가산 셀렉터(핵심)**: 이름 셀렉터는 기존 id/code 셀렉터(device_id/station/line/group_id) 위에 **가산**된다. 기존 셀렉터의 자료구조·우선순위·디스패치 경로는 **무회귀**여야 한다(REQ-NF-01).
- **CRUD `name` 필드 비-오버로드**: `processRequest.Name`(json `name`)은 `add_device`/`add_group`/`set_device` 의 **명명(CRUD)** 에 쓰이며 제어 셀렉터가 아니다. 이름 셀렉터는 **신규 필드** `DeviceName`/`GroupName`(json `device_name`/`group_name`)으로 표현하며 CRUD `name` 과 절대 혼용하지 않는다.
- **이름→id 해소는 얇은 레이어**: 이름 셀렉터는 리졸버(`DeviceByName`/`GroupByName`)로 **정확히 하나의** device_id/group 로 해소한 뒤, 기존 개별 제어/그룹 fan-out 경로를 **그대로 재사용**한다(제어 의미론 재구현 금지).
- **락 규율 준수**: 리졸버는 프로젝트의 RWMutex 재진입 deadlock 트랩을 회피한다 — 로스터/그룹 레지스트리 락을 취득해 **스냅샷을 뜬 뒤 해제**하고, 락을 해제한 상태에서 디스패치를 호출하며, 두 락을 **절대 중첩하지 않는다**.
- **모호성 안전(fail-closed)**: 이름은 유일하지 않을 수 있으므로(그룹 표시명은 유일 아님, 디바이스 이름도 이론상 충돌 가능), 다중 매치는 조용히 임의 선택하지 않고 **거부**한다(RD-2).

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - `processRequest` 신규 셀렉터 필드 `DeviceName`/`GroupName`(json `device_name`/`group_name`) + `fillFromParams` 승격
  - 에이전트 이름 리졸버 `DeviceByName(name)` / `GroupByName(name)` (스냅샷-안전, 모호성/부재 처리)
  - 제어 디스패치 확장: 이름 셀렉터 → id 해소 → 기존 개별/fan-out 경로 재사용, 우선순위 체인 확장
  - 신규 센티널 에러 `ErrAmbiguousName`
  - 노드 레이어 pass-through: `buildXsfmControlCommand` 가 `device_name`/`group_name` 를 top-level 로 방출, `hasXsfmControlCommand` 가 이름 셀렉터 존재 시 제어로 라우팅, `xsfmExtract*` 미러
  - 기존 셀렉터 무회귀(device_id/station/line/group_id + 우선순위)
- **범위 외(Out-of-Scope)**:
  - MQTT 토픽/페이로드 스키마 변경 (이름 셀렉터는 순수 논리 해소 레이어 — 브로커 통신 규약 불변). 노드 pass-through 는 flow 입력 규약이지 브로커 규약 변경이 아니다.
  - 이름 유일성 강제(디바이스/그룹 이름을 unique 로 만드는 것) — 본 SPEC 은 유일성을 요구하지 않고 **모호성 거부**로 대응한다.
  - 그룹 엔티티/멤버십 모델(SPEC-XSFM-GROUP-001), 라인/역사 레지스트리 재설계 — 그대로 유지
  - 프런트엔드 이름 제어 UI — 후속(선택) 마일스톤, 필수 아님(§plan M4)

---

## 2. Assumptions (가정)

### A-1. CRUD `name` 필드와 셀렉터 이름의 분리

`processRequest.Name`(json `name`)은 add_device/add_group/set_device 의 명명 인자이며, 제어 대상 지정에 쓰이지 않는다. 이름 셀렉터는 별도 필드 `DeviceName`/`GroupName` 로 표현한다. 기존 `name` 사용 경로(control.go handleAddDevice/handleSetDevice)는 무회귀여야 한다.

### A-2. 이름은 유일하지 않을 수 있다

- 그룹 표시명(`Group.Name`)은 유일하지 않다 — 유일한 것은 `custom:<code>` id/code 뿐이다(SPEC-XSFM-GROUP-001). 파생 station/line 그룹의 표시명도 서로/커스텀과 충돌 가능.
- 디바이스 이름(`Device.Name`)은 `composeName` 파생 또는 sticky override 이며 이론상 충돌 가능(위치 계층이 동일하거나 사용자가 동일 이름을 지정).

따라서 이름→대상 해소는 **0/1/≥2 매치** 세 경우를 모두 다뤄야 한다.

### A-3. 이름 해소 후에는 기존 경로 재사용

`device_name` 은 정확히 하나의 device_id 로 해소되어 **개별 제어**(controlDevice 경로)로 처리되고, `group_name` 은 정확히 하나의 그룹으로 해소되어 그 그룹의 id/code 로 **그룹 fan-out**(GroupMembers → fanOutControl)으로 처리된다. 제어 의미론(값 검증, 2-축, 응답 대기, best-effort, 집계 응답)은 재사용한다.

### A-4. 노드는 이름 셀렉터를 top-level 로 나른다

노드 레이어(`buildXsfmControlCommand`)는 `device_name`/`group_name` 을 명령 JSON 의 top-level 셀렉터로 방출하며, 에이전트 `processRequest` 가 이를 top-level json 태그로 읽는다. `params.device_name`/`params.group_name` 은 `fillFromParams` 가 top-level 로 승격시켜 명령 shape 정합을 보장한다(group_id 승격 패턴 계승).

---

## 3. Requirements (요구사항 — EARS)

> ID 체계: `REQ-XSFM-NAMESEL-001-{모듈}-{순번}`. 인수 기준의 상세 Given-When-Then 은 `acceptance.md` 참조.

### Module 1 — 이름 리졸버 (agent name resolvers)

- **REQ-XSFM-NAMESEL-001-01-01** (Ubiquitous): 시스템은 `DeviceByName(name)` 리졸버를 제공해야 하며, 이는 로스터에서 `Device.Name` 이 name 과 일치하는 디바이스를 찾아 그 device_id 를 반환해야 한다.
- **REQ-XSFM-NAMESEL-001-01-02** (Ubiquitous): 시스템은 `GroupByName(name)` 리졸버를 제공해야 하며, 이는 **전 타입 그룹**(custom + 파생 station/line 그룹)에서 표시명 `Group.Name` 이 name 과 일치하는 그룹을 찾아 그 그룹 id/code 를 반환해야 한다(RD-5). 타입 간 표시명 충돌은 REQ-01-03(`ErrAmbiguousName`, 안전 거부)로 처리한다.
- **REQ-XSFM-NAMESEL-001-01-03** (Unwanted): **IF** name 이 정확히 2개 이상의 디바이스(DeviceByName) 또는 그룹(GroupByName)과 일치하면 **THEN** 시스템은 `ErrAmbiguousName` 을 반환하고 **어떠한 명령도 방출하지 않아야 한다**(RD-2, fail-closed).
- **REQ-XSFM-NAMESEL-001-01-04** (Unwanted): **IF** name 이 어떤 디바이스/그룹과도 일치하지 않으면(0 매치) **THEN** 시스템은 각각 `ErrDeviceNotFound`/`ErrGroupNotFound` 로 거부해야 한다.
- **REQ-XSFM-NAMESEL-001-01-05** (Ubiquitous): 리졸버는 스냅샷-안전 락 규율을 따라야 한다 — 로스터/그룹 레지스트리 락을 취득해 스냅샷을 뜬 뒤 **해제**하고, 락을 **중첩하지 않으며**, 리졸버 반환 후 락 미보유 상태에서 디스패치가 진행되어야 한다(RWMutex 재진입 deadlock 회피).
- **REQ-XSFM-NAMESEL-001-01-06** (Ubiquitous): 이름 매칭은 **공백 trim 후 정확 일치, 대소문자 구분(case-sensitive)** 규칙을 따라야 한다(RD-6) — 입력과 비교 대상 `Name` 양쪽을 `strings.TrimSpace` 로 trim 한 뒤 정확히 일치할 때만 매치로 간주하며 case folding 은 하지 않는다. 리졸버는 결정적이어야 하며, 동일 입력·동일 상태에 대해 동일 결과(또는 동일 에러)를 내야 한다.

### Module 2 — 셀렉터 디스패치 (fields + priority)

- **REQ-XSFM-NAMESEL-001-02-01** (Ubiquitous): `processRequest` 는 신규 셀렉터 필드 `DeviceName`(json `device_name`)·`GroupName`(json `group_name`)을 가져야 하며, 이는 CRUD `Name`(json `name`) 필드와 **별개**여야 한다.
- **REQ-XSFM-NAMESEL-001-02-02** (Event-driven): **WHEN** 제어 명령이 `device_name` 셀렉터를 담고 상위 우선순위 셀렉터(device_id)가 없으면 **THEN** 시스템은 `DeviceByName` 으로 device_id 를 해소하고 **개별 제어**(controlDevice 경로)로 처리해야 한다.
- **REQ-XSFM-NAMESEL-001-02-03** (Event-driven): **WHEN** 제어 명령이 `group_name` 셀렉터를 담고 상위 우선순위 셀렉터가 없으면 **THEN** 시스템은 `GroupByName` 으로 그룹을 해소하고 그 그룹 id/code 로 **그룹 fan-out**(GroupMembers → fanOutControl)으로 처리해야 한다.
- **REQ-XSFM-NAMESEL-001-02-04** (State-driven): **IF** 여러 셀렉터가 동시 지정되면 **THEN** 시스템은 확정된 우선순위 체인 **`device_id > device_name > station > line > group_id > group_name`**(RD-4) 을 적용하여 첫 번째로 지정된 셀렉터만 해석해야 한다. "개별 먼저" 원칙에 따라 개별 셀렉터(id·이름 모두)가 집계/그룹 셀렉터에 선행하며, `device_name` 은 `device_id` 바로 뒤에 위치한다.
- **REQ-XSFM-NAMESEL-001-02-05** (State-driven): **IF** `group_name` 이 해소한 그룹이 멤버 없는 그룹이면 **THEN** 시스템은 어떠한 명령도 방출하지 않고 `ErrEmptyGroup` 을 반환해야 한다(그룹 fan-out 기존 동작 계승).

### Module 3 — 노드 pass-through (flow 노드 레벨)

- **REQ-XSFM-NAMESEL-001-03-01** (Ubiquitous): `buildXsfmControlCommand` 는 flow 메시지에 실린 `device_name`/`group_name` 을 명령 JSON 의 **top-level 셀렉터**로 방출해야 한다(기존 device_id/group_id top-level 방출 패턴 계승).
- **REQ-XSFM-NAMESEL-001-03-02** (State-driven): **IF** 통합 `xsfm` 노드가 `device_name` 또는 `group_name`(제어 키 포함) 메시지를 받으면 **THEN** `hasXsfmControlCommand` 는 이를 **제어 명령**으로 라우팅해야 한다 — 이름 셀렉터는 device_id 로 키잉되는 상태 스냅샷에 실리지 않으므로(상태 유입은 device_id 기준) 상태 주입(`FeedState`)이 아닌 제어로 판정된다(group_id 라우팅 규칙 계승).
- **REQ-XSFM-NAMESEL-001-03-03** (Ubiquitous): 노드는 `xsfmExtractDeviceID`/`xsfmExtractGroupID` 와 대칭인 이름 추출 경로(payload 우선, metadata 폴백)를 제공해야 한다.
- **REQ-XSFM-NAMESEL-001-03-04** (Ubiquitous): 에이전트 `fillFromParams` 는 `params.device_name`/`params.group_name` 을 top-level `DeviceName`/`GroupName` 로 승격시켜(top-level 우선, 빈 값일 때만 backfill) HTTP exec 계약과 명령 shape 정합을 보장해야 한다(group_id 승격 패턴 계승).
- **REQ-XSFM-NAMESEL-001-03-05** (State-driven): **IF** 노드 메시지에 `device_id`(또는 다른 상위 셀렉터)와 `device_name`/`group_name` 이 함께 지정되면 **THEN** 노드는 존재하는 셀렉터를 조용히 드롭하지 않고 모두 top-level 로 실어 **우선순위 판정을 에이전트에 위임**해야 한다(REQ-02-04 계승).

### Module 4 — 에러 (sentinel)

- **REQ-XSFM-NAMESEL-001-04-01** (Ubiquitous): 시스템은 신규 센티널 에러 `ErrAmbiguousName`(errors.New, `errors.Is` 호환)을 정의해야 하며, 이는 이름 셀렉터가 다중 매치될 때 반환된다.
- **REQ-XSFM-NAMESEL-001-04-02** (Unwanted): `ErrAmbiguousName` 반환 시 시스템은 부분 방출이나 임의 매치 선택을 하지 **않아야 한다**(전무 방출).

### 비기능 요구 (NFR)

- **REQ-XSFM-NAMESEL-001-NF-01** (Unwanted): 이름 셀렉터 도입이 기존 셀렉터(device_id/station/line/group_id)의 동작·우선순위·집계 응답을 **회귀시키지 않아야 한다**(기존 테스트 전부 통과).
- **REQ-XSFM-NAMESEL-001-NF-02** (Ubiquitous): 이름 리졸버와 제어 fan-out 의 동시 실행이 race 없이 동작해야 한다(스냅샷 후 락 해제 패턴, `-race` 클린).
- **REQ-XSFM-NAMESEL-001-NF-03** (Ubiquitous): 이름 해소는 순수 논리 레이어여야 하며 MQTT 토픽/페이로드 스키마를 변경하지 않아야 한다.

---

## 4. Specifications (설계 명세)

### 4.1 processRequest 신규 필드

```go
type processRequest struct {
    // ... 기존 필드 ...
    Name      string `json:"name,omitempty"`        // (기존) CRUD 명명 — 셀렉터 아님
    DeviceName string `json:"device_name,omitempty"` // (신규) 개별 이름 셀렉터
    GroupName  string `json:"group_name,omitempty"`  // (신규) 그룹 이름 셀렉터
    // ...
}
```

`fillFromParams` 에 승격 추가(group_id 패턴 계승):

```go
if req.DeviceName == "" { req.DeviceName = stringField(req.Params, "device_name") }
if req.GroupName  == "" { req.GroupName  = stringField(req.Params, "group_name") }
```

### 4.2 이름 리졸버 (스냅샷-안전)

```go
// DeviceByName 는 Name 이 trim 후 정확 일치(대소문자 구분, RD-6)하는 유일한 device_id 를 반환한다.
// 0 매치 → ErrDeviceNotFound, ≥2 매치 → ErrAmbiguousName.
func (a *XSFMAgent) DeviceByName(name string) (string, error)

// GroupByName 는 전 타입 그룹(custom + 파생 station/line, RD-5)에서 표시명 Name 이
// trim 후 정확 일치(대소문자 구분, RD-6)하는 유일한 그룹 id/code 를 반환한다.
// 0 매치 → ErrGroupNotFound, ≥2 매치 → ErrAmbiguousName.
func (a *XSFMAgent) GroupByName(name string) (string, error)
```

- `DeviceByName`: `ListDevices()`(또는 로스터 RLock 스냅샷) 순회 → `strings.TrimSpace(d.Name) == strings.TrimSpace(input)` 매치 카운트(대소문자 구분, RD-6). 락은 스냅샷 후 해제.
- `GroupByName`: `ListGroups()`(**전 타입** — custom + 파생 station/line, RD-5) 순회 → `strings.TrimSpace(Group.Name) == strings.TrimSpace(input)` 매치 카운트(대소문자 구분, RD-6). 그룹 레지스트리 락은 스냅샷 후 해제. 파생 그룹 표시명과 커스텀 그룹명이 충돌하면 다중 매치가 되어 `ErrAmbiguousName`(RD-2) 로 안전 거부된다.
- 두 리졸버 모두 락 미보유 상태에서 반환하며, 호출부(디스패치)가 반환된 id 로 락 미보유 상태에서 기존 경로를 호출한다.

### 4.3 디스패치 통합 (우선순위 체인 확장)

`dispatchControl`/`handleSelectorControl` 확장(확정 순서 — RD-4, `device_id > device_name > station > line > group_id > group_name`):

```
1. device_id  != ""  → controlDevice 개별 경로 (기존)
2. device_name != "" → DeviceByName → (해소 id) → controlDevice 개별 경로 (신규)
3. station    != ""  → DevicesByStation → fanOut (기존)
4. line       != ""  → DevicesByLine → fanOut (기존)
5. group_id   != ""  → GroupMembers → fanOut (기존)
6. group_name != ""  → GroupByName → (해소 id) → GroupMembers → fanOut (신규)
7. else → ErrInvalidCommand (대상 미지정)
```

- `device_name` 해소 실패(ErrDeviceNotFound/ErrAmbiguousName)는 즉시 반환(방출 없음).
- `group_name` 해소 성공 후에는 해소된 group id 를 기존 group_id 경로(GroupMembers → fanOutControl)에 그대로 넣어 재사용한다 — 빈 그룹은 ErrEmptyGroup(REQ-02-05).
- 구현 배치: 리졸버는 신규 파일(예: `name_resolver.go`) 또는 `group_membership.go` 확장; 디스패치 게이트는 `group.go`(`handleSelectorControl`)에서 확장한다(현행 셀렉터 게이트 위치와 일치, `control.go` 불변 유지 지향).

### 4.4 노드 pass-through

- `buildXsfmControlCommand`: `xsfmExtractDeviceName`/`xsfmExtractGroupName`(payload 우선, metadata 폴백) 추가 → 존재 시 `cmd["device_name"]`/`cmd["group_name"]` top-level 방출.
- `hasXsfmControlCommand`: `xsfmExtractDeviceName(msg) != "" || xsfmExtractGroupName(msg) != ""` 이면 제어로 라우팅(command|params|group_id 기존 조건에 OR 추가).
- 명령 추론은 기존 규칙 그대로(command 명시 우선, 아니면 power→set_power / fan_speed→set_fan_speed / 둘 다→set_multiple).

### 4.5 에러

`errors.go` 에 추가:

```go
// ErrAmbiguousName 은 이름 셀렉터(device_name/group_name)가 2개 이상의 대상과
// 일치할 때 반환된다 (SPEC-XSFM-NAMESEL-001 RD-2). 무방출 fail-closed.
ErrAmbiguousName = errors.New("xsfm: ambiguous name (matches multiple targets)")
```

### 4.6 메시지 형식 (노드 이름 셀렉터)

**개별 이름 제어 (device_name)**
- 간편형: `{ "device_name": "환기팬-01", "power": true }`
- 명시형: `{ "device_name": "환기팬-01", "command": "set_power", "params": { "power": true } }`

**그룹 이름 제어 (group_name, fan-out)**
- `{ "group_name": "2층 환기", "power": false }`
- `{ "group_name": "1호선", "fan_speed": 2 }`

**우선순위 예시**
- `{ "device_id": "01", "device_name": "환기팬-01", "power": true }` → `device_id` 우선(개별 "01" 제어), `device_name` 무시.
- `{ "group_id": "custom:floor2", "group_name": "2층 환기", "power": true }` → `group_id` 우선.

### 4.7 Traceability (추적성)

| 요구 | 설계/코드 anchor | 인수 시나리오 |
| --- | --- | --- |
| 01-01~06 | 신규 `DeviceByName`/`GroupByName`(스냅샷-안전, 모호성/부재/trim) | AC 1.x |
| 02-01~05 | `processRequest`{DeviceName,GroupName}, `handleSelectorControl` 우선순위 확장, 개별/fan-out 재사용 | AC 2.x, AC 4.x |
| 03-01~05 | `buildXsfmControlCommand`, `hasXsfmControlCommand`, `xsfmExtract*Name`, `fillFromParams` 승격 | AC 3.x |
| 04-01~02 | `ErrAmbiguousName` 센티널, 무방출 | AC 1.x |
| NF-01~03 | 회귀 테스트, `-race` 클린, MQTT 규약 불변 | AC 5.x |

---

## 5. 확정된 설계 결정 (Resolved Decisions)

> 아래 6개 결정은 사용자 확정으로 본문에 baked-in 되었다. (RD-1~3: v0.1.0, RD-4~6: v0.2.0 — 구 OQ-1~3 승격)

- **RD-1 (신규 셀렉터 필드)**: `device_name`/`group_name` 을 신규 제어 셀렉터로 도입한다. 신규 `processRequest` 필드 `DeviceName`/`GroupName`(json `device_name`/`group_name`)을 사용하며, CRUD `name` 필드를 **재사용/오버로드하지 않는다**. `device_name` 은 device_id 로 해소되어 개별 제어, `group_name` 은 그룹으로 해소되어 그룹 fan-out 으로 처리된다.
- **RD-2 (모호성 = 거부)**: 이름이 2개 이상 대상과 일치하면 `ErrAmbiguousName` 으로 거부하고 **아무것도 방출하지 않는다**. 0 매치 → not-found(`ErrDeviceNotFound`/`ErrGroupNotFound`). 정확히 1개 → 진행. 임의 매치 선택은 금지(fail-closed).
- **RD-3 (범위: 에이전트 + 노드)**: 에이전트(`handleSelectorControl` 디스패치 + `DeviceByName`/`GroupByName` 리졸버, 스냅샷-안전 락 규율)와 노드(`buildXsfmControlCommand` top-level pass-through + `hasXsfmControlCommand` 라우팅) 양쪽에 구현한다.
- **RD-4 (셀렉터 우선순위)** *(구 OQ-1 확정)*: 셀렉터 우선순위 체인은 **`device_id > device_name > station > line > group_id > group_name`** 이다. "개별 먼저" 원칙 — 개별 셀렉터(id·이름 모두)가 집계/그룹 셀렉터에 선행한다. `device_name` 은 개별 디바이스로 해소되므로 `device_id` 바로 뒤에 위치하며, `station`/`line`/`group_id` 등 모든 집계 셀렉터보다 앞선다. 여러 셀렉터 동시 지정 시 체인상 첫 번째로 지정된 셀렉터만 해석한다.
- **RD-5 (group_name 이름 공간)** *(구 OQ-2 확정)*: `GroupByName` 은 **전 타입 그룹**(custom + 파생 station/line 그룹)의 표시명 `Name` 을 매칭한다. 즉 `group_name="강남역"` 이 파생 station 그룹으로도 해소될 수 있다. 서로 다른 타입 간(예: 커스텀 그룹명 vs 파생 그룹 표시명) 충돌은 RD-2 의 `ErrAmbiguousName`(다중 매치 안전 거부)으로 처리한다 — 별도 타입 우선순위를 두지 않는다.
- **RD-6 (매칭 규칙)** *(구 OQ-3 확정)*: 이름 매칭은 **공백 trim 후 정확 일치, 대소문자 구분(case-sensitive)** 이다. 입력과 비교 대상 `Name` 양쪽을 `strings.TrimSpace` 로 trim 한 뒤 정확히 일치할 때만 매치이며, case folding(대소문자 무시)은 하지 않는다. 따라서 대소문자만 다른 이름은 매치되지 않는다.

---

## 6. 열린 질문 (Open Questions)

> 잔여 열린 질문 없음. 구 OQ-1~3 은 사용자 확정으로 RD-4~6(§5) 에 승격되어 본문에 baked-in 되었다(v0.2.0).

---

## 7. Implementation Notes (as-implemented, spec-anchored Level 2)

> 아래는 구현(`2acb980d`, M1~M3) 이 사양(§3·§4) 과 **동작 보존적으로** 갈린 지점을 기록한 것이다(spec-anchored L2 규율). 어떤 분기도 요구·인수 기준의 의미를 바꾸지 않으며, 모두 §4 사양과 정합한다. 커버리지·테스트 수치는 run-phase(오케스트레이터) 보고 값이다.

### 7.1 우선순위 체인 분산 배치 + `handleIndividualControl` 추출 (분기 1)

- **사양(§4.3)**: `dispatchControl`/`handleSelectorControl` 에 확정 순서(RD-4) `device_id > device_name > station > line > group_id > group_name` 를 하나의 순차 게이트로 확장.
- **구현**: 우선순위 체인이 `dispatchControl`(최상위 `device_id` 분기) + `handleSelectorControl`(스위치 진입 전 `device_name` if-guard, `group_name` 을 최종 case 로) 로 **분산 배치**되었다. 개별 제어 경로는 `handleIndividualControl` 로 **추출(extract-method)** 하여 `device_id`·`device_name` 두 진입점이 동일 개별 제어 로직을 공유한다.
- **사유**: 개별(device_id/device_name)과 집계(station/line/group_id/group_name) 셀렉터의 자연스러운 경계를 코드 구조에 반영. extract-method 는 동작 보존적이며 두 개별 진입점의 중복을 제거한다(enforce simplicity). 확정 순서(RD-4)는 코드/주석/테스트에 그대로 고정 — 동작 순서 불변.

### 7.2 `GroupByName` 전 타입 매칭 = `allGroups()` 추출 (분기 2)

- **사양(§4.2, RD-5)**: `GroupByName` 은 전 타입 그룹(custom + 파생 station/line) 표시명을 매칭.
- **구현**: 전 타입 순회를 위해 `handleListGroups` 에서 **`allGroups()`(custom + 파생 station/line 그룹 집합)를 추출**하고, 이를 `GroupByName` 과 `handleListGroups` 가 **공유**한다(DRY, 목록·해소 경로 단일 SSOT).
- **사유**: 전 타입 집합을 도출하는 로직이 두 곳에 중복 구현되는 것을 방지. 목록(`list_groups`) 과 이름 해소(`GroupByName`) 가 동일한 그룹 세계관을 보므로 결과 일관성 보장.

### 7.3 `group_name` fan-out 집계 셀렉터 라벨 (분기 3)

- **사양(§4.3)**: `group_name` 해소 후 해소된 group id 를 기존 group_id 경로(GroupMembers → fanOutControl)에 넣어 재사용.
- **구현**: `group_name` fan-out 집계 응답의 셀렉터 참조가 `selectorRef{Type:"group_name"}` 로 표기된다(해소 전 원 셀렉터 종류를 집계 응답에 보존).
- **사유**: 집계 응답 소비자가 "무엇으로 대상이 지정되었는지"(group_id 가 아닌 group_name) 를 식별할 수 있게 함. fan-out 실행 자체는 해소된 group id 로 기존 경로를 무변경 재사용.

### 7.4 구현 배치 (분기 4 — 사양 권장 대안 채택)

- **사양(§4.3)**: 리졸버는 신규 파일(예: `name_resolver.go`) **또는** `group_membership.go` 확장; 디스패치 게이트는 `group.go` 에서 확장.
- **구현**: 리졸버를 **신규 파일 `name_resolver.go`** 에 배치(사양의 권장 대안). 디스패치 확장은 `group.go`(`dispatchControl`/`handleSelectorControl`) 에서 수행. `control.go` 는 불변 유지.
- **사유**: `agent.go`/`control.go` diff 최소화(GROUP-001 의 `group_membership.go` 신규 파일 패턴 계승), 이름 해소 로직의 응집도 확보.

### 7.5 M4(프런트엔드 이름 제어 UI) 이연 — "completed" 범위 명시

- **범위**: 본 SPEC 의 `status: completed` 전이는 **핵심 기능(M1~M3: 에이전트 리졸버·디스패치 + 노드 pass-through + 테스트/무회귀)** 에 대한 것이다.
- **이연**: **M4(프런트엔드 이름 기반 제어 입력 UI)** 는 plan §2 에서 **선택·저우선(Priority Low)** 으로 분류되어 **이연(DEFERRED)** 되었다. 이름 셀렉터 기능은 에이전트+노드 레벨에서 이미 **완전히 사용 가능**하다 — flow 노드가 `device_name`/`group_name` 메시지를 제어로 라우팅하고, HTTP exec 계약(`params.device_name`/`params.group_name` 승격) 으로도 호출 가능하므로 전용 UI 없이도 기능이 온전하다.
- **overclaim 방지**: 따라서 "completed" 는 M4 UI 를 포함하지 않는다. M4 는 선택적 후속(optional follow-up)으로 남으며, 착수 시 별도 검증(vitest/`tsc`)이 필요하다. plan §2 Optional Goal / acceptance §6 DoD 의 "M4 착수 시 별도 검증" 조항과 정합한다.
