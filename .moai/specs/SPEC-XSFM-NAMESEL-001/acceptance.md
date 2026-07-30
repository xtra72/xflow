# Acceptance Criteria — SPEC-XSFM-NAMESEL-001

> Given-When-Then 인수 시나리오. 요구 추적: `spec.md` §4.7. Definition of Done 은 §6 참조.
> 버전: v0.2.0 — 구 OQ-1~3 사용자 확정(RD-4~6) 반영. 아래 시나리오는 확정값 기준으로 구체화됨: 우선순위 체인 `device_id > device_name > station > line > group_id > group_name`(RD-4, 개별 먼저), group_name 전 타입 매칭(RD-5), trim 후 정확 일치 + 대소문자 구분(RD-6). v0.1.0 = 제안 기본안 기준 초안.

## AC 1.x — 이름 리졸버 + 모호성/부재 (Module 1, Module 4)

### AC 1.1 — device_name 유일 매치 → device_id 해소
- **Given** 로스터에 `Name="환기팬-01"` 인 디바이스가 정확히 1개(device_id="01") 존재
- **When** `DeviceByName("환기팬-01")` 를 호출
- **Then** device_id `"01"` 을 반환하고 에러가 없다

### AC 1.2 — device_name 다중 매치 → ErrAmbiguousName (무방출)
- **Given** 로스터에 `Name="환기팬"` 인 디바이스가 2개 이상 존재
- **When** `device_name="환기팬"` 로 제어 명령(set_power)을 실행
- **Then** `ErrAmbiguousName` 을 반환하고, **어떠한 제어 명령도 방출되지 않는다**(cmdSink 호출 0회)

### AC 1.3 — device_name 0 매치 → ErrDeviceNotFound
- **Given** 로스터에 `Name="없는이름"` 인 디바이스가 없음
- **When** `DeviceByName("없는이름")` 를 호출
- **Then** `ErrDeviceNotFound` 를 반환한다

### AC 1.4 — group_name 유일 매치(커스텀 그룹) → 그룹 해소
- **Given** 그룹 레지스트리에 `Name="2층 환기"` 인 커스텀 그룹이 정확히 1개(id="custom:floor2") 존재
- **When** `GroupByName("2층 환기")` 를 호출
- **Then** `"custom:floor2"` 를 반환하고 에러가 없다

### AC 1.4b — group_name 유일 매치(파생 station 그룹, RD-5 전 타입) → 그룹 해소
- **Given** 파생 station 그룹의 표시명이 `Name="강남역"`(그룹 id/code 예: "station:gangnam")이며, 그 표시명과 충돌하는 다른 그룹(커스텀/파생)이 없음
- **When** `GroupByName("강남역")` 를 호출
- **Then** 파생 station 그룹 id/code(`"station:gangnam"`)를 반환하고 에러가 없다 — 커스텀뿐 아니라 파생 station/line 그룹도 표시명으로 해소된다(RD-5)

### AC 1.5 — group_name 다중 매치(커스텀 + 파생 충돌, RD-5+RD-2) → ErrAmbiguousName (무방출)
- **Given** `Name="강남역"` 인 그룹이 2개 이상 존재 — 예: 커스텀 그룹 `Name="강남역"` + 파생 station 그룹 표시명 `"강남역"`(전 타입 매칭이므로 둘 다 후보)
- **When** `group_name="강남역"` 로 그룹 제어를 실행
- **Then** `ErrAmbiguousName` 을 반환하고, 어떤 멤버로도 명령이 방출되지 않는다(타입 간 충돌도 안전 거부, 타입 우선순위 없음)

### AC 1.6 — group_name 0 매치 → ErrGroupNotFound
- **Given** `Name="없는그룹"` 인 그룹이 없음
- **When** `GroupByName("없는그룹")` 를 호출
- **Then** `ErrGroupNotFound` 를 반환한다

### AC 1.7 — 공백 trim 정확 일치
- **Given** `Name="환기팬-01"` 인 디바이스가 1개 존재
- **When** `DeviceByName("  환기팬-01  ")`(앞뒤 공백) 를 호출
- **Then** trim 후 정확 일치하여 device_id `"01"` 을 반환한다

### AC 1.8 — 대소문자 구분 매칭(RD-6): 대소문자만 다른 이름은 매치 안 됨
- **Given** `Name="Fan-A"` 인 디바이스가 정확히 1개(device_id="01") 존재하고, `Name="fan-a"` 인 디바이스는 없음
- **When** `DeviceByName("fan-a")`(소문자) 를 호출
- **Then** case folding 을 하지 않으므로 매치되지 않고 `ErrDeviceNotFound` 를 반환한다. `DeviceByName("Fan-A")`(정확 케이스) 호출 시에만 `"01"` 을 반환한다

## AC 2.x — 이름 셀렉터 제어 (Module 2)

### AC 2.1 — device_name → 개별 제어
- **Given** `Name="환기팬-01"`(device_id="01") 이 유일하게 존재
- **When** `{ "device_name": "환기팬-01", "command": "set_power", "params": {"power": true} }` 제어 실행
- **Then** device_id "01" 에 대해 개별 제어(controlDevice)가 1회 실행되고 성공 응답을 반환한다

### AC 2.2 — group_name → 그룹 fan-out
- **Given** `Name="2층 환기"`(id="custom:floor2", 멤버 device_id ["01","02"]) 그룹이 유일하게 존재
- **When** `{ "group_name": "2층 환기", "command": "set_fan_speed", "params": {"fan_speed": 2} }` 제어 실행
- **Then** 그룹 멤버 ["01","02"] 각각에 fan-out 되고, 집계 응답(`{selector, results, status}`)을 반환한다

### AC 2.3 — group_name 이 빈 그룹 해소 → ErrEmptyGroup
- **Given** `Name="빈그룹"`(멤버 0) 그룹이 유일하게 존재
- **When** `group_name="빈그룹"` 로 제어 실행
- **Then** 어떠한 명령도 방출되지 않고 `ErrEmptyGroup` 을 반환한다

## AC 3.x — 노드 pass-through / 라우팅 (Module 3)

### AC 3.1 — buildXsfmControlCommand device_name top-level 방출
- **Given** flow 메시지 payload `{ "device_name": "환기팬-01", "power": true }`
- **When** `buildXsfmControlCommand` 를 호출
- **Then** 생성된 명령 JSON 은 top-level `device_name="환기팬-01"` 을 포함하고 `command="set_power"` 로 추론된다

### AC 3.2 — buildXsfmControlCommand group_name top-level 방출
- **Given** flow 메시지 payload `{ "group_name": "2층 환기", "fan_speed": 2 }`
- **When** `buildXsfmControlCommand` 를 호출
- **Then** 명령 JSON 은 top-level `group_name="2층 환기"` + `command="set_fan_speed"` 를 포함한다

### AC 3.3 — hasXsfmControlCommand 이름 셀렉터 → 제어 라우팅
- **Given** 통합 `xsfm` 노드가 `{ "device_name": "환기팬-01", "power": true }`(command/params 없음) 메시지를 받음
- **When** `hasXsfmControlCommand` 를 평가
- **Then** `true`(제어 명령)를 반환하고 상태 주입(FeedState)이 아닌 제어 경로로 라우팅된다. `group_name` 만 실린 경우에도 동일하다

### AC 3.4 — fillFromParams 이름 승격
- **Given** HTTP exec 계약으로 `{ "command":"set_power", "params": { "device_name":"환기팬-01", "power": true } }` 수신(top-level device_name 없음)
- **When** `fillFromParams` 실행
- **Then** `req.DeviceName == "환기팬-01"` 으로 승격되어 이름 셀렉터로 정상 해소된다

### AC 3.5 — 셀렉터 드롭 금지(위임)
- **Given** flow 메시지 `{ "device_id":"01", "device_name":"환기팬-02", "power": true }`
- **When** `buildXsfmControlCommand` 를 호출
- **Then** 명령 JSON 은 `device_id` 와 `device_name` 을 **둘 다** top-level 로 포함한다(노드가 조용히 드롭하지 않음)

## AC 4.x — 우선순위 (Module 2, RD-4 확정 체인 `device_id > device_name > station > line > group_id > group_name`)

### AC 4.1 — device_id 가 device_name 을 이긴다
- **Given** `{ "device_id":"01", "device_name":"환기팬-02", "power": true }` (둘 다 유효 해소 가능)
- **When** 제어 실행
- **Then** device_id "01" 개별 제어만 실행되고 `device_name` 은 무시된다

### AC 4.2 — 명시적 group_id 가 group_name 을 이긴다
- **Given** `{ "group_id":"custom:floor2", "group_name":"3층 환기", "power": true }`
- **When** 제어 실행
- **Then** `custom:floor2` 그룹 fan-out 만 실행되고 `group_name` 은 무시된다

### AC 4.3 — device_name 이 station/line/group_id(집계 셀렉터)보다 우선(개별 먼저, RD-4)
- **Given** `{ "device_name":"환기팬-01", "station":"강남역", "line":"2호선", "group_id":"custom:floor2", "power": true }` (device_id 없음)
- **When** 제어 실행
- **Then** RD-4 "개별 먼저" 체인에 따라 `device_name` → 개별 제어(device_id "01")만 실행되고, station/line/group_id 는 무시된다

### AC 4.4 — device_name 이 station 을 이긴다(체인 인접 확인)
- **Given** `{ "device_name":"환기팬-01", "station":"강남역", "power": true }` (device_id 없음)
- **When** 제어 실행
- **Then** `device_name` → 개별 제어만 실행되고 `station` fan-out 은 실행되지 않는다

### AC 4.5 — group_name 이 최하위 우선순위(모든 다른 셀렉터에 후행, RD-4)
- **Given** `{ "station":"강남역", "group_name":"2층 환기", "power": true }` (device_id/device_name/line/group_id 없음)
- **When** 제어 실행
- **Then** 체인상 `station` 이 `group_name` 보다 앞서므로 `station` fan-out 만 실행되고 `group_name` 은 무시된다. group_name 은 다른 상위 셀렉터가 하나도 없을 때에만 해석된다(체인 최하위)

## AC 5.x — 무회귀 / 비기능 (NFR)

### AC 5.1 — 기존 셀렉터 무회귀
- **Given** 이름 셀렉터 미사용, `device_id`/`station`/`line`/`group_id` 로 제어
- **When** 각 기존 셀렉터로 제어 실행
- **Then** 기존과 동일한 개별/fan-out 동작과 집계 응답을 내며 기존 테스트가 전부 통과한다

### AC 5.2 — CRUD name 무회귀
- **Given** `add_device`/`set_device`/`add_group` 가 `name` 인자를 사용
- **When** 각 CRUD 명령 실행
- **Then** `name` 은 명명 인자로만 동작하고 제어 셀렉터로 오인되지 않는다(기존 CRUD 테스트 통과)

### AC 5.3 — 동시성 (-race)
- **Given** 이름 리졸버와 그룹 fan-out 이 동시 실행
- **When** `go test -race ./internal/agent/xsfm/...`
- **Then** race 경고가 없고 락 중첩 deadlock 이 발생하지 않는다

### AC 5.4 — MQTT 규약 불변
- **Given** 이름 셀렉터로 제어
- **When** 명령이 방출됨
- **Then** MQTT 토픽/페이로드 스키마는 기존과 동일하다(이름은 논리 해소 레이어일 뿐)

## 6. Definition of Done

- [ ] REQ-01~04 + NFR 구현 완료, spec §4.7 추적표의 모든 anchor 반영
- [ ] AC 1.x~5.x 전부 통과(테스트 코드로 검증)
- [ ] `go test -race ./internal/agent/xsfm/... ./internal/node/...` 클린
- [ ] xsfm 패키지 커버리지 ≥85% 목표 유지, 기존 테스트 무회귀
- [ ] 프런트 미변경 시 기존 vitest/`tsc` 자동 무회귀 (M4 착수 시 별도 검증)
- [x] OQ-1~3 사용자 확정값 반영 완료(RD-4~6, v0.2.0) — 우선순위 체인(RD-4)·전 타입 group_name 매칭(RD-5)·trim+대소문자 구분(RD-6) baked-in, 본문/AC 정정 완료
- [ ] `gofmt`/`goimports` 클린, Conventional Commits + SPEC 참조
