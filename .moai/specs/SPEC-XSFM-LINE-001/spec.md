---
id: SPEC-XSFM-LINE-001
title: "xsfm 라인 1급화 + 코드 기반 주소 체계 + 디바이스 네이밍"
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
tags: "xsfm, line, line-registry, group-code, code-addressing, device-naming, composeName, migration, station, selector, fan-out, frontend, node-control"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-07-30 | 0.1.0 | 초기 SPEC 작성 — xsfm 에 **라인(line)을 1급 엔티티로 승격**하고 **코드 기반 통일 주소 체계**를 도입. (1) 라인 전용 레지스트리 신설(`Line{Code, Name, Order}` + 자체 RWMutex + write-through 영속, station_registry 미러) + `add_line`/`remove_line`/`list_lines` 명령, (2) 커스텀 그룹에 사용자 코드 도입(그룹 id `custom:<name>` → `custom:<code>`, name 은 표시 전용), (3) 제어/셀렉터를 `station:<code>`/`line:<code>`/`custom:<code>` 로 통일, (4) 디바이스 자동 이름 규칙 확장 `{line}:{station}:{place}:{index:03d}`, (5) 1회성 로드 마이그레이션(기존 `station.Line` 문자열 → Line 엔티티 ensure-create, 기존 `custom:<name>` → `custom:<code>`). 확정 설계 RD-1~3 반영. 빈-라인 디바이스 네이밍 기본값 등 4건 열린 질문(OQ) 기재. |
| 2026-07-30 | 0.2.0 | OQ-1~4 사용자 확정 반영 — RD-4~7 승격 및 §6 Open Questions 제거(미해결 OQ 없음). (RD-4) 빈-라인 네이밍은 라인 세그먼트 생략(3-세그먼트, 하위호환), (RD-5) 참조 중 라인 `remove_line` 은 `ErrLineInUse` 반환 거부, (RD-6) 통일 코드 포맷 `^[a-z0-9][a-z0-9_-]*$` 강제 + 레거시 비적합 name slugify 마이그레이션 규칙 명세, (RD-7) `Line.Order` 채택(기본값=생성 순번). §4 사양(composeName 빈-라인 분기·예시, remove_line ErrLineInUse, 코드 포맷+slugify 규칙+워크드 예시, Line 구조체 Order), 센티널 에러 `ErrLineInUse` 추가, Module 1/4/5 EARS 요구사항 확정 거동 참조로 갱신. |
| 2026-07-30 | 0.3.0 | **M1~M6 구현 완료** (백엔드 `ef4f28a7`, 프런트 `e8678033`). 신규 `line_registry.go`(`LineRegistry` 자체 RWMutex + write-through, `Line{Code,Name,Order}`, `add_line`/`remove_line`/`list_lines`, `ErrLineInUse`) + `code.go`(§4.6 slugify: 한글 Revised Romanization + `validCode`) + 로드 마이그레이션(`station.Line`→Line 엔티티, 레거시 `custom:<name>`→`custom:<slug>`) + `Group.Code`(id `custom:<code>`) + composeName 4/3-세그먼트 분기(sticky 보존). 프런트: `useLine` 훅, `XsfmLinesTab`, 그룹 코드 입력, AgentDetailPanel 라인 탭. xsfm 커버리지 89.1% · `-race` 클린 · `go test ./...` exit 0, 프런트 vitest 2275 pass · `tsc` 클린. §7 as-implemented 분기 5건 기록. status draft→completed. |

---

# SPEC-XSFM-LINE-001: xsfm 라인 1급화 + 코드 기반 주소 체계 + 디바이스 네이밍

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이며, `internal/agent/xsfm` 는 지하철 역사(station) 설비를 MQTT 로 제어·모니터링하는 에이전트이다(SPEC-XSFM-001 정의). SPEC-XSFM-GROUP-001 에서 **그룹(group)을 1급 개념으로 도입**(그룹 엔티티·다대다 멤버십·일괄 제어·타입 접두사 id 인코딩)하며 완료(v0.4.0)되었다.

본 SPEC 은 그 위에 **라인(line)을 1급 엔티티로 승격**하고, station/line/custom 을 관통하는 **코드 기반 통일 주소 체계**를 확립한다. 현재 라인은 독립 엔티티가 아니라 **역사의 속성**(`StationRegistryEntry.Line`, 역사당 단일 문자열)으로만 존재하며, 라인 집합은 등록된 역사들의 `Line` 값으로부터 **파생**된다(station→line SSOT). `add_line` 명령도, 빈 라인(역사 0개) 개념도 없다. 본 SPEC 은 이 가정을 **의도적으로 갱신**하여 라인을 코드·이름을 가진 1급 엔티티로 승격한다.

### 1.2 기술 환경

- **언어/모듈**: Go 1.23+ / `internal/agent/xsfm/` (기존 패키지 확장)
- **프런트엔드**: React + TypeScript (`web/src/`) — 역사/디바이스/그룹 탭 및 대시보드 패널
- **관련 기존 anchor (본 SPEC 이 정확히 참조·재사용)**:
  - 역사 레지스트리: `station_registry.go` `StationRegistry`(station→line SSOT, 자체 RWMutex, write-through 영속), `ResolveLine(station)`(station→line 해석, 미등록 시 `ErrStationNotFound`, station_registry.go:129), `StationsByLine(line)`(Order 오름차순, station_registry.go:152), `handleAddStation`/`handleRemoveStation`/`handleListStations`
  - 멤버 파생: `agent.go` `DevicesByStation(station)`(agent.go:464), `DevicesByLine(line)`(agent.go:487, `StationsByLine` 순회로 파생)
  - 그룹 레지스트리: `group_registry.go` `GroupRegistry`(자체 RWMutex + 캐시 + write-through atomic, `StationRegistry` 미러), `Group{ID, Name, Type, Ref, Members}`(group_registry.go:45), 타입 접두사 인코딩 `groupPrefixCustom/Station/Line`(group_registry.go:34-36), `customIDFor(name)="custom:"+name`(group_registry.go:73), `groupTypeFromID`(group_registry.go:55)
  - 디바이스 이름 합성: `mapping.go` `composeName(station, place, index)` = `fmt.Sprintf("%s:%s:%03d", station, place, index)`(mapping.go:240-242, index 3자리 0-채움)
  - sticky-name: `control.go` `nameOverridden`(사용자 수동 override 이름 보존)
  - 셀렉터 fan-out: `control.go` `dispatchControl`/`handleSelectorControl`, `group.go` `fanOutControl`/`buildControlPlan`/`aggregateStatus`, 우선순위 `device_id > station > line > group_id`
  - 명령 라우팅: `agent.go` `Process()` switch, `processRequest.fillFromParams()`(params backfill)
  - 영속화: `persist.go` `deviceRegistryStore`, `group_registry.go` `groupRegistryStore`(tmp+rename atomic write)
  - 센티널 에러: `errors.go` (`ErrGroupAlreadyExists`, `ErrStationNotFound`, `ErrInvalidCommand`, `ErrDeviceNotFound` 등)
  - 노드 레벨 제어: SPEC-XSFM-GROUP-001 Module 7 — xsfm-control / 상태·제어 통합 `xsfm` 노드가 `station:`/`line:`/`custom:` 접두사 group_id 셀렉터를 에이전트로 **접두사 그대로 전달(pass-through)**
- **테스트**: Go 표준 `testing` + `testify`; 프런트 `vitest`

### 1.3 설계 원칙

- **비침습 가산 레이어(핵심)**: 라인 레지스트리는 `GroupRegistry` 가 그러했듯 **별도 레이어로 신설**한다. 기존 station→line 파생·fan-out·그룹(GROUP-001) 동작의 **구조를 바꾸지 않으며**, 무회귀여야 한다.
- **기존 패턴 준수**: 라인 레지스트리는 `StationRegistry`/`GroupRegistry` 의 락 규율(자체 RWMutex, 로스터/pending/타 레지스트리 락과 절대 중첩 금지 — 프로젝트 RWMutex 재귀 deadlock 트랩)과 영속 패턴(캐시 + write-through, atomic write)을 그대로 따른다.
- **파생 vs 명시 분리 유지**: `line:<code>` 멤버십은 여전히 해당 라인 코드를 참조하는 역사들로부터 `DevicesByLine` 로 **파생**한다(라인 레지스트리는 라인 엔티티의 존재·코드·표시명만 관장하며 멤버 device_id 를 저장하지 않는다). 커스텀 그룹 멤버십만 명시 저장(GROUP-001 계승).
- **코드 = SSOT 식별자**: station/line/custom 모두 `<타입>:<code>` 로 지정한다. `station.Line` 은 이제 **라인 코드**를 참조한다(자유 문자열 → 코드).
- **마이그레이션 비파괴**: 로드 1회성 마이그레이션으로 기존 데이터를 자동 승격하며 원본을 삭제하지 않는다(GROUP-001 마이그레이션 방침 계승).

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - 라인 1급 엔티티 `Line{Code, Name, Order}` + 라인 레지스트리(신설, 별도 영속 저장소) + `add_line`/`remove_line`/`list_lines` 명령
  - 커스텀 그룹 코드 필드 도입(id `custom:<name>` → `custom:<code>`, name 표시 전용) + `add_group{code, name, members}`
  - 코드 기반 통일 주소(`station:<code>`/`line:<code>`/`custom:<code>`)
  - 디바이스 자동 이름 규칙 확장 `{line}:{station}:{place}:{index:03d}` (sticky-name 보존)
  - 1회성 로드 마이그레이션(`station.Line` → Line 엔티티, `custom:<name>` → `custom:<code>`)
  - 프런트: 라인 관리 UI + 그룹 코드 입력 + 디바이스 이름 표시 반영
- **범위 밖(Out-of-Scope)**:
  - MQTT 토픽/페이로드 규약 변경(불변)
  - SPEC-XSFM-GROUP-001 그룹 다대다 멤버십 모델 자체의 변경(가산만)
  - place(위치) 를 1급 엔티티로 승격(별도 SPEC 대상, 본 SPEC 은 device_index 를 place 하위 인덱스로 유지)
  - fan-out/controlDevice 경로 재구현(대상 집합 도출만 담당)

## 2. Assumptions (가정)

- **A-1**: `StationRegistry.ResolveLine` 는 station→line 해석의 SSOT 로 유지된다. 라인 레지스트리는 라인 엔티티의 존재/코드/표시명을 관장할 뿐, station→line 매핑의 소유권을 가져오지 않는다.
- **A-2**: 라인 코드는 역사 코드와 동일한 코드 문자열 도메인을 공유한다(예: `st01` 형태). 허용 포맷은 RD-6 에서 `^[a-z0-9][a-z0-9_-]*$` 로 확정되었다.
- **A-3**: 노드 레이어는 group_id 문자열을 접두사 판별 없이 그대로 전달(pass-through)하므로, 커스텀 그룹 id 가 name→code 로 바뀌어도 노드 코드 변경이 필요 없다(REQ-03-04 에서 검증).
- **A-4**: 디바이스의 자동 생성 이름만 새 포맷을 적용받으며, `nameOverridden` 로 표시된 사용자 지정 이름은 보존된다.
- **A-5**: 라인은 역사 0개 상태(빈 라인)로 존재할 수 있으며, 빈 라인의 `line:<code>` 조회는 빈 멤버 집합을 반환한다(에러 아님).

## 3. Requirements (EARS 요구사항)

### Module 1 — 라인 1급 엔티티 & 라인 레지스트리 (REQ-01)

- **REQ-01-01** (Ubiquitous): 시스템은 항상 라인을 `Line{Code, Name, Order}` 엔티티로 표현해야 한다. `Code` 는 고유 식별자, `Name` 은 표시명, `Order` 는 UI 정렬 키이다.
- **REQ-01-02** (Ubiquitous): 시스템은 항상 라인 엔티티를 **별도 라인 레지스트리**(자체 RWMutex + 인메모리 캐시 + write-through atomic 영속, `StationRegistry` 미러)에 보관해야 한다.
- **REQ-01-03** (Event): WHEN `add_line{code, name}` 명령을 수신 THEN 시스템은 해당 코드로 라인 엔티티를 upsert 하고 영속화해야 한다.
- **REQ-01-04** (Unwanted): 시스템은 이미 존재하는 라인 코드를 **중복 생성(신규 등록)** 하여 기존 표시명을 소리 없이 덮어써서는 안 된다 — 재지정은 upsert(명시적 갱신) 의미로만 허용한다.
- **REQ-01-05** (Event): WHEN `remove_line{code}` 명령을 수신 THEN 시스템은 해당 라인 엔티티를 제거하고 영속화해야 한다.
- **REQ-01-05a** (Unwanted): 시스템은 해당 라인 코드를 참조하는 역사(또는 디바이스)가 하나라도 존재하는 라인을 제거해서는 안 되며, 이 경우 `ErrLineInUse` 를 반환해야 한다(RD-5, dangling 참조 방지). 참조 역사를 먼저 비우거나 재배치한 뒤에만 제거 가능하다.
- **REQ-01-06** (Event): WHEN `list_lines` 명령을 수신 THEN 시스템은 등록된 라인 목록을 `Order` 오름차순으로 반환해야 한다.
- **REQ-01-07** (State): IF 라인에 소속된 역사가 0개이면(빈 라인) THEN 시스템은 그 라인을 유효한 엔티티로 유지하고, `line:<code>` 멤버 조회 시 빈 집합을 반환해야 한다.
- **REQ-01-08** (Ubiquitous): 시스템은 항상 `line:<code>` 그룹 멤버십을 **해당 라인 코드를 참조하는 역사들로부터 파생**(`DevicesByLine`)해야 하며, 라인 레지스트리에 멤버 device_id 를 저장하지 않아야 한다.

### Module 2 — 커스텀 그룹 코드 (REQ-02)

- **REQ-02-01** (Ubiquitous): 시스템은 항상 커스텀 그룹을 사용자 지정 `Code` 로 식별해야 하며, 그룹 id 를 `custom:<code>` 로 인코딩해야 한다(기존 `custom:<name>` 대체).
- **REQ-02-02** (Ubiquitous): 시스템은 항상 커스텀 그룹의 `Name` 을 표시 전용 필드로 취급해야 한다(식별에 사용하지 않음).
- **REQ-02-03** (Event): WHEN `add_group{code, name, members}` 명령을 수신 THEN 시스템은 코드 포맷·유일성을 검증한 뒤 그룹을 생성해야 한다.
- **REQ-02-04** (Unwanted): 시스템은 이미 존재하는 커스텀 그룹 코드로 신규 그룹을 생성해서는 안 되며, 이 경우 `ErrGroupAlreadyExists` 를 반환해야 한다.
- **REQ-02-05** (Unwanted): 시스템은 코드 포맷 검증(`^[a-z0-9][a-z0-9_-]*$`, RD-6)을 통과하지 못한 코드로 그룹을 생성해서는 안 된다.

### Module 3 — 코드 기반 통일 주소 (REQ-03)

- **REQ-03-01** (Ubiquitous): 시스템은 항상 그룹 제어 셀렉터를 `station:<code>` / `line:<code>` / `custom:<code>` 통일 접두사 스킴으로 해석해야 한다.
- **REQ-03-02** (State): IF group_id 가 `line:<code>` THEN 시스템은 해당 라인 코드에 대한 `DevicesByLine` 파생 대상 집합으로 fan-out 해야 한다(기존 fan-out 경로 재사용).
- **REQ-03-03** (State): IF group_id 가 `custom:<code>` THEN 시스템은 해당 코드의 커스텀 그룹 명시 멤버로 fan-out 해야 한다.
- **REQ-03-04** (Ubiquitous): 시스템은 항상 노드 레이어(SPEC-XSFM-GROUP-001 Module 7)가 전달한 접두사+코드 group_id 를 코드 변경 없이 해석해야 한다(노드는 문자열 pass-through, 에이전트가 접두사 판별).

### Module 4 — 디바이스 네이밍 (REQ-04)

- **REQ-04-01** (Ubiquitous): 시스템은 항상 디바이스 **자동 생성 이름**을 `{line}:{station}:{place}:{index:03d}` 포맷으로 합성해야 한다. 여기서 `line` = `ResolveLine(station)`, `index` 는 3자리 0-채움이다.
- **REQ-04-02** (State): IF 디바이스가 `nameOverridden`(사용자 지정) 상태이면 THEN 시스템은 새 자동 이름 포맷을 적용하지 않고 지정 이름을 보존해야 한다.
- **REQ-04-03** (State): IF 역사에 라인 코드가 없거나 `ResolveLine` 이 미해석(빈 라인 코드/`ErrStationNotFound`)이면 THEN 시스템은 **라인 세그먼트를 생략**하여 자동 이름을 `{station}:{place}:{index:03d}`(현행 3-세그먼트, 하위호환·무회귀)로 합성해야 한다(RD-4).
- **REQ-04-03a** (Event): WHEN 이후 역사에 라인 코드가 지정되어 자동 이름 재계산이 트리거되면 THEN 시스템은 자동 이름을 4-세그먼트로 재계산해야 한다. 단, `nameOverridden`(sticky) 이름은 보존한다(REQ-04-02).
- **REQ-04-04** (Ubiquitous): 시스템은 항상 이름 합성에서 `index` 를 3자리 0-채움(예: 3 → `003`)으로 유지해야 한다(기존 규약 계승).
- **REQ-04-05** (Unwanted): 시스템은 디바이스 이름 규칙 변경을 매칭용 보조 인덱스 키(정규화 int)에 적용해서는 안 된다(표시 vs 매칭 목적 분리 유지, mapping.go 규약 계승).

### Module 5 — 마이그레이션 (REQ-05)

- **REQ-05-01** (Event): WHEN 에이전트가 기동하며 영속 데이터를 로드 THEN 시스템은 기존 `station.Line` 문자열 값 각각에 대해 대응 Line 엔티티를 라인 레지스트리에 ensure-create(없으면 생성, code = 기존 라인 문자열) 해야 한다.
- **REQ-05-02** (Event): WHEN 에이전트가 영속 그룹 데이터를 로드 THEN 시스템은 기존 `custom:<name>` 그룹을 `custom:<code>` 로 마이그레이션해야 한다. 코드 포맷(`^[a-z0-9][a-z0-9_-]*$`)을 만족하는 name 은 `code = name` 으로 그대로 승격하고, 포맷 비적합 name(공백·한글·콜론 등 포함)은 **slugify** 하여 code 를 생성하되 name 표시값은 원문을 보존해야 한다(RD-6, §4.6 규칙).
- **REQ-05-02a** (Ubiquitous): 시스템은 항상 slugify 결과가 이미 존재하는 코드와 충돌하면 접미 번호를 부여해 유일성을 보장해야 한다(RD-6).
- **REQ-05-03** (Ubiquitous): 시스템은 항상 마이그레이션을 **1회성·비파괴**로 수행해야 한다(원본 데이터를 삭제하지 않고 승격만 수행, 재기동 시 멱등).
- **REQ-05-04** (Unwanted): 시스템은 마이그레이션 실패 시 기존 station/line 파생 및 그룹 동작을 회귀시켜서는 안 된다(마이그레이션은 가산 레이어 복원에 국한).

### Module 6 — 프런트엔드 (REQ-06)

- **REQ-06-01** (Event): WHEN 사용자가 라인 관리 UI 에서 라인을 추가/조회 THEN 프런트엔드는 `add_line`/`list_lines` 를 통해 라인 엔티티를 관리해야 한다.
- **REQ-06-02** (Event): WHEN 사용자가 그룹 탭에서 커스텀 그룹을 생성 THEN 프런트엔드는 코드 입력 필드를 제공하고 `add_group{code, name, members}` 를 전송해야 한다.
- **REQ-06-03** (Ubiquitous): 프런트엔드는 항상 디바이스 이름 표시에 새 이름 포맷을 반영해야 한다 — 라인 있음 시 `{line}:{station}:{place}:{index:03d}`(4-세그먼트), 라인 없음 시 `{station}:{place}:{index:03d}`(3-세그먼트, RD-4).
- **REQ-06-04** (Optional): 가능하면 라인 관리 UI 는 `Order` 기반 정렬 및 빈 라인 표시를 제공한다.
- **REQ-06-05** (Ubiquitous): 프런트엔드는 항상 기존 역사/디바이스/그룹(GROUP-001) 탭 동작을 회귀 없이 유지해야 한다(가산 UI).

### Module 7 — 비기능/무회귀 (REQ-07, NFR)

- **REQ-07-01** (Ubiquitous): 시스템은 항상 라인 레지스트리 락을 자체 RWMutex 로 관리하고, 로스터/pending/station/group 레지스트리 락과 **중첩하지 않아야 한다**(프로젝트 RWMutex 재귀 deadlock 트랩 회피).
- **REQ-07-02** (Ubiquitous): 시스템은 항상 라인 레지스트리 영속을 tmp+rename atomic write 로 수행해야 한다.
- **REQ-07-03** (Ubiquitous): 시스템은 항상 SPEC-XSFM-GROUP-001 의 그룹 동작(다대다 멤버십, 접두사 id, fan-out 병존)을 무회귀로 유지해야 한다.
- **REQ-07-04** (Ubiquitous): 시스템은 항상 xsfm 패키지 테스트 커버리지 85% 이상 및 `-race` 클린을 유지해야 한다(TRUST 5 Tested).
- **REQ-07-05** (Ubiquitous): 시스템은 항상 MQTT 토픽/페이로드 규약을 불변으로 유지해야 한다.

## 4. Specifications (사양)

### 4.1 Line 엔티티 & 라인 레지스트리

```go
// Line 은 라인 1급 엔티티이다. Code 는 고유 식별자, Name 은 표시명, Order 는 UI 정렬 키.
type Line struct {
    Code  string `json:"code"`            // 고유 식별자(코드). group id 의 line:<code> 로 사용.
    Name  string `json:"name"`            // 표시명.
    Order int    `json:"order,omitempty"` // UI 정렬 키(오름차순).
}

// LineRegistry 는 라인 엔티티의 인메모리 캐시 + 영속 표면이다(신설 레이어).
// StationRegistry/GroupRegistry 패턴 미러: 자체 RWMutex + 캐시 + write-through atomic.
type LineRegistry struct {
    mu    sync.RWMutex
    cache map[string]Line       // code → Line
    store *lineRegistryStore    // dir=="" 이면 nil(인메모리 전용)
}
```

- 저장 위치: `{dir}/line_registry.json`(GROUP-001 의 `group_registry.json` 과 동일 디렉터리 규약).
- **멤버 미저장**: 라인 레지스트리는 device_id 를 저장하지 않는다. `line:<code>` 멤버는 항상 `DevicesByLine(code)` 로 파생(REQ-01-08).
- **락 규율**: 모든 공개 메서드는 자체 `mu` 만 취득하며, `StationRegistry`/`GroupRegistry`/로스터 락을 보유한 상태에서 호출되지 않도록 호출부에서 순서를 보장한다(REQ-07-01).

### 4.2 명령 API 표

| 명령          | 파라미터                    | 동작                                        | 에러                                   |
| ----------- | ----------------------- | ----------------------------------------- | ------------------------------------ |
| `add_line`    | `{code, name, order}`     | 라인 upsert + 영속                              | 코드 포맷(`^[a-z0-9][a-z0-9_-]*$`) 위반 시 검증 에러 |
| `remove_line` | `{code}`                  | 라인 제거 + 영속                                 | 참조 역사(또는 디바이스) 존재 시 `ErrLineInUse`(RD-5), 미존재 라인 시 `ErrLineNotFound` |
| `list_lines`  | —                       | 라인 목록(`Order` 오름차순) 반환                       | —                                    |
| `add_group`   | `{code, name, members}`   | 커스텀 그룹 생성(id=`custom:<code>`), 코드 포맷·유일성 검증 | `ErrGroupAlreadyExists` / 포맷 위반         |

- `add_station{station, line, display_name, order}` 의 `line` 인자는 이제 **라인 코드**를 참조한다(REQ-05-01 마이그레이션과 정합).
- 신규 센티널 에러(`errors.go` 추가): `ErrLineNotFound`(미존재 라인 조회/제거), `ErrLineInUse`(참조 중인 라인 `remove_line` 거부, RD-5).

### 4.3 composeName 새 포맷

```go
// 기존: composeName(station, place, index) → "{station}:{place}:{index:03d}"  (mapping.go:240-242)
// 신규: 라인 세그먼트를 선두에 부가하되, 라인 코드가 해석되지 않으면 라인 세그먼트를 생략(RD-4).
//   line = ResolveLine(station)   // station→line SSOT
//   composeName(line, station, place, index):
//     - line != ""  → "{line}:{station}:{place}:{index:03d}"   // 4-세그먼트
//     - line == ""  → "{station}:{place}:{index:03d}"          // 3-세그먼트 (하위호환)
```

- 호출부에서 `ResolveLine(station)` 로 라인 코드를 해석해 주입한다(순수 함수 유지, 레지스트리 의존 주입 회피).
- **빈-라인 거동(RD-4, 확정)**: `ResolveLine` 이 `ErrStationNotFound`/빈 코드를 반환하면 라인 세그먼트를 **생략**하여 기존 3-세그먼트 포맷과 하위 호환·무회귀를 유지한다.
  - 예시(라인 있음): station=`st01`, `ResolveLine(st01)="line_2"`, place=`pump`, index=3 → `line_2:st01:pump:003` (4-세그먼트)
  - 예시(라인 없음): station=`st99`, `ResolveLine(st99)=""`, place=`pump`, index=3 → `st99:pump:003` (3-세그먼트)
- **라인 후지정 재계산(RD-4)**: 이후 해당 역사에 라인 코드가 지정되면 자동 이름은 3-세그먼트 → 4-세그먼트로 재계산된다. 단, `nameOverridden`(sticky) 이름은 보존한다.
- **sticky-name**: `nameOverridden` 디바이스는 새 포맷 미적용(REQ-04-02).
- **보조 인덱스 키 불변**: `compositeKey`(정규화 int 매칭 키)는 변경하지 않는다(REQ-04-05).

### 4.4 마이그레이션(로드 1회성)

1. **라인 ensure-create**: 로드 시 `StationRegistry` 의 모든 엔트리 `Line` 값 집합을 순회하여, 라인 레지스트리에 없는 코드는 `Line{Code: line, Name: line, Order: <다음순번>}` 로 생성. 이미 존재하면 무시(멱등).
2. **커스텀 그룹 코드화(RD-6)**: 영속 그룹 로드 시 `custom:<name>` id 를 `custom:<code>` 로 승격한다.
   - name 이 코드 포맷(`^[a-z0-9][a-z0-9_-]*$`)을 만족하면 `code = name` 그대로 승격(id=`custom:<name>` 유지).
   - name 이 포맷 비적합(공백·한글·콜론 등 포함)이면 §4.6 규칙으로 **slugify** 하여 code 를 생성(id=`custom:<slug>`), name 표시값은 원문을 보존.
3. **비파괴·멱등**: 원본 미삭제, 재기동 시 동일 결과(REQ-05-03).

### 4.5 락 규율 & 영속 패턴

- `LineRegistry` 는 `GroupRegistry` 와 동일하게 자체 RWMutex + write-through(atomic tmp+rename).
- 레지스트리 간 락 중첩 금지: `add_station` 등에서 station·line 레지스트리를 함께 다룰 때 한 번에 하나의 레지스트리 락만 보유하도록 호출 순서를 직렬화한다.

### 4.6 코드 포맷 & slugify 규칙 (RD-6, 확정)

- **통일 코드 포맷**: `^[a-z0-9][a-z0-9_-]*$` (소문자 영숫자로 시작, 이후 소문자 영숫자·언더스코어·하이픈 허용). station/line/custom **신규 코드**에 동일하게 강제한다. `add_line`/`add_group`/신규 station 코드는 이 포맷 위반 시 검증 에러로 거부한다(REQ-02-05).
- **slugify 적용 시점**: 로드 마이그레이션에서 포맷 비적합 레거시 `custom:<name>` 을 code 로 변환할 때만 사용한다(신규 생성 경로는 slugify 하지 않고 포맷을 강제 검증한다).
- **slugify 결정 규칙(결정적)**:
  1. Unicode NFC 정규화 후 ASCII 대문자를 소문자화한다.
  2. **한글(Hangul) 음절**은 Revised Romanization(유니코드 자모 분해 기반 고정 lead/vowel/tail 매핑, 음운 동화 미적용)으로 소문자 라틴으로 변환한다. 예: `층`→`cheung`, `창`→`chang`, `고`→`go`.
  3. 그 외 비-ASCII 문자(한자/가나 등)와 공백·콜론 등 비허용 문자는 각각 `-` 로 치환한다.
  4. 연속 하이픈은 하나로 축약하고 양끝 하이픈을 트림한다. 남은 선두 비허용 문자가 없어져 포맷 제약(`[a-z0-9]` 시작)을 만족한다.
  5. 결과가 빈 문자열이면(로마자화 불가한 순수 비-ASCII 등) 원문의 안정적 해시를 폴백으로 사용한다: `g<fnv1a32-hex8>` (예: `g1a2b3c4d`).
  6. **충돌 처리**: 생성된 slug 가 이미 존재하는 코드와 충돌하면 접미 번호를 부여한다 — `<slug>`, `<slug>-2`, `<slug>-3` … (첫 미충돌 값 채택).
- **워크드 예시**: 레거시 `custom:"2층 창고"` → 소문자화 `2층 창고` → 로마자화 `2cheung 창고`(`창`→`chang`, `고`→`go` → `2cheung changgo`) → 공백→하이픈 → **`2cheung-changgo`**. 결과 id = `custom:2cheung-changgo`, name 표시값 = 원문 `"2층 창고"` 보존.

## 5. Resolved Decisions (확정된 설계 결정)

> 아래는 사용자 확정 사항이며 재검토하지 않는다.

- **RD-1 — 라인 1급 엔티티**: 라인을 전용 레지스트리를 가진 1급 엔티티로 승격한다(`StationRegistry` 미러: 자체 RWMutex + write-through 영속). `Line{Code, Name, Order}`. 명령 `add_line{code,name}`/`remove_line{code}`/`list_lines`. 역사는 라인을 **코드**로 참조(`StationRegistryEntry.Line` = 라인 코드). 빈 라인(역사 0개) 존재 가능. `line:<code>` 멤버십은 해당 라인 코드를 참조하는 역사로부터 파생(`DevicesByLine` 취지 불변).
- **RD-2 — 커스텀 그룹 사용자 코드 + 표시명**: 커스텀 그룹에 `Code` 필드를 추가한다. 그룹 id = `custom:<code>`(기존 `custom:<name>` 대체). `name` 은 표시 전용. `add_group{code, name, members}` — 사용자가 코드 제공, 포맷·유일성 검증(중복 시 `ErrGroupAlreadyExists`).
- **RD-3 — 코드 기반 통일 주소**: 제어/셀렉터는 `station:<code>` / `line:<code>` / `custom:<code>` 로 통일한다(station/line/custom 일관 접두사+코드 스킴).
- **RD-4 — 빈-라인 디바이스 네이밍(구 OQ-1 확정)**: `composeName` 은 라인 코드가 해석되면(`ResolveLine(station) != ""`) `{line}:{station}:{place}:{index:03d}`(4-세그먼트), 라인 코드가 없으면 **라인 세그먼트를 생략**하여 `{station}:{place}:{index:03d}`(현행 3-세그먼트, 하위호환·무회귀)로 합성한다. 이후 역사에 라인이 지정되면 자동 이름이 4-세그먼트로 재계산되나 `nameOverridden`(sticky) 이름은 보존한다. (§4.3)
- **RD-5 — 참조 중 라인의 remove_line 거부(구 OQ-2 확정)**: `remove_line` 대상 라인을 참조하는 역사(또는 디바이스)가 하나라도 있으면 **거부하고 `ErrLineInUse` 를 반환**한다(dangling 방지). 참조 역사를 먼저 비우거나 재배치한 뒤에만 삭제할 수 있다. (REQ-01-05a, §4.2)
- **RD-6 — 코드 포맷 + slugify 마이그레이션(구 OQ-3 확정)**: 통일 코드 포맷 `^[a-z0-9][a-z0-9_-]*$` 를 station/line/custom **신규 코드**에 강제한다. 로드 마이그레이션 시 포맷 적합 레거시 `custom:<name>` 은 code=name 으로 승격하고, 포맷 비적합 name(공백·한글·콜론 등)은 **slugify**(§4.6 규칙: NFC·소문자화, 한글 Revised Romanization, 비허용문자→`-`, 중복 하이픈 축약, 양끝 트림, 빈 결과 시 해시 폴백, 충돌 시 접미 번호)하여 code 를 생성(id `custom:<slug>`)하되 name 표시값은 원문을 보존한다. 워크드 예시: `"2층 창고"` → `2cheung-changgo`. (§4.6)
- **RD-7 — Line.Order 채택(구 OQ-4 확정)**: `Line{Code, Name, Order}` 에 `Order` 를 포함한다(기본값 = 생성 순번). `StationRegistryEntry.Order` 및 UI 정렬과 일관성을 유지한다. (§4.1)

> §6 Open Questions 는 OQ-1~4 가 모두 RD-4~7 로 확정되어 제거되었다(미해결 열린 질문 없음).

## 6. Traceability

- 상위 SPEC: SPEC-XSFM-001(base agent), SPEC-XSFM-GROUP-001(그룹 1급화, completed v0.4.0)
- 코드 anchor: `station_registry.go`(ResolveLine/StationsByLine), `agent.go`(DevicesByLine/DevicesByStation), `group_registry.go`(Group/GroupRegistry/접두사 id), `mapping.go`(composeName), `control.go`(nameOverridden), `group.go`(fanOutControl)
- 하위 산출물: plan.md(마일스톤·기술 접근·리스크), acceptance.md(Given-When-Then 인수 시나리오)

## 7. Implementation Notes (구현 완료 — as-implemented, Level 2)

> 본 절은 구현(백엔드 `ef4f28a7` M1~M5, 프런트 `e8678033` M6)이 SPEC 사양과 **의도적으로 달라진** 지점을 기록한다(spec-anchored Level 2 as-implemented). 각 분기는 무회귀·하위호환을 위한 결정이며, 관련 요구사항/AC 는 그대로 충족된다.

- **신규 파일**: `line_registry.go`(`LineRegistry` — 자체 RWMutex + write-through 영속, `Line{Code, Name, Order}`, `add_line`/`remove_line`/`list_lines`, `ErrLineInUse`), `code.go`(§4.6 slugify — 한글 Revised Romanization + `validCode` 포맷 검증). Init 로드 마이그레이션에서 `station.Line`→Line 엔티티 ensure-create, 레거시 `custom:<name>`→`custom:<slug>` 승격. `Group.Code` 추가 + id `custom:<code>`. `composeName` = `{line}:{station}:{place}:{index}`(라인 미해석 시 라인 세그먼트 생략 3-세그먼트, sticky 보존).

### 7.1 분기 1 — `add_group` code 파라미터를 **선택(optional)** 으로 구현

- **사양**: RD-2 는 `add_group{code, name, members}` 로 사용자 코드를 필수 입력으로 명세.
- **구현**: `code` 를 **선택 파라미터**로 구현(keyPresent 검사). code 가 있으면 `custom:<code>`(포맷 검증), 없으면 레거시 `custom:<name>` 경로로 폴백.
- **사유**: 기존 SPEC-XSFM-GROUP-001 의 `add_group{name}` 테스트를 무회귀로 유지하기 위함. 신규 코드 기반 생성과 레거시 name 기반 생성이 병존한다. (REQ-02-01/02-03 충족, GROUP-001 무회귀)

### 7.2 분기 2 — 코드 포맷 검증을 station 코드에는 **미강제**

- **사양**: RD-6 통일 코드 포맷 `^[a-z0-9][a-z0-9_-]*$` 를 station/line/custom **신규 코드**에 강제(§4.6).
- **구현**: 포맷 검증(`validCode`)을 `add_line` 과 `add_group`(code 경로)에만 적용하고, **station 코드에는 적용하지 않음**.
- **사유**: 기존 station 테스트가 대문자/짧은 코드(`ST-101`, `S1`)를 사용하므로 강제 시 회귀 발생. station 코드 거부를 요구하는 AC 도 없음. (신규 코드 진입점만 강제, 무회귀 우선)

### 7.3 분기 3 — 마이그레이션 시 라인 코드는 **slugify 미적용**

- **사양**: §4.4 마이그레이션은 라인 ensure-create + 커스텀 그룹 slugify.
- **구현**: `station.Line` 값은 `Line{Code: line, Name: line}` 로 **원문 그대로**(레거시 라인 코드 보존) 승격. slugify 는 **커스텀 그룹 name 에만** 적용(§4.4-2, §7.1).
- **사유**: 기존 라인 코드를 slugify 하면 `station.Line` 참조와의 정합이 깨져 dangling 발생 가능. 라인은 이미 코드 성격의 문자열이므로 원문 보존이 안전. (REQ-05-01 충족)

### 7.4 분기 4 — `golang.org/x/text` 직접 의존성으로 승격

- **구현**: slugify 의 Unicode NFC 정규화를 위해 `golang.org/x/text/unicode/norm` 사용 → `go mod tidy` 로 `x/text` 가 **간접→직접 의존성**으로 승격(`go.mod` require 블록).
- **사유**: §4.6 slugify 규칙(NFC 정규화)의 결정론적 구현 요구. 외부 신규 패키지 도입이 아닌 기존 표준 확장 모듈의 직접화. (§4.6 충족)

### 7.5 분기 5 — 디바이스 이름 표시는 **프런트 변경 없음**

- **사양**: REQ-06-03 는 프런트가 4/3-세그먼트 이름 포맷을 표시하도록 요구.
- **구현**: `XsfmDevicesTab` 및 멤버 테이블이 이미 `device.name` 을 **그대로 렌더**하므로 프런트 코드 변경 불필요. 4-세그먼트/3-세그먼트 포맷은 **백엔드 `composeName`** 이 생산하고 프런트는 이를 표시만 함.
- **사유**: 표시 계층이 이미 백엔드 산출 이름을 신뢰·렌더하는 구조 → 새 포맷이 프런트 수정 없이 자동 반영. (REQ-06-03 충족, 프런트 diff 최소)
