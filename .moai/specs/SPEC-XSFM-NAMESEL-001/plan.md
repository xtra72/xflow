# Implementation Plan — SPEC-XSFM-NAMESEL-001

> xsfm 이름 기반 제어 셀렉터(`device_name`/`group_name`). Tier M. 상세 요구는 `spec.md`, 인수 기준은 `acceptance.md` 참조.
> 버전: v0.3.0(status: completed) — **핵심 구현 완료(M1~M3, `2acb980d`)**. M1(에이전트 리졸버 `name_resolver.go` + `dispatchControl` 우선순위 체인 + `handleIndividualControl` 추출) + M2(노드 `buildXsfmControlCommand`/`hasXsfmControlCommand` pass-through) + M3(테스트/무회귀). 검증: `go test ./...` exit 0(42 pkgs), 신규 함수 커버리지 100%, `-race` 클린. **M4(프런트엔드 이름 제어 UI)는 선택·저우선으로 이연(DEFERRED)** — 에이전트+노드 레벨에서 기능 완전 사용 가능, "completed" 는 M1~M3 핵심에 한함(spec.md §7.5). as-implemented 분기 4건은 spec.md §7.
> 버전: v0.2.0 — 구 OQ-1~3 사용자 확정(RD-4 우선순위 체인 `device_id > device_name > station > line > group_id > group_name`, RD-5 전 타입 group_name 매칭, RD-6 trim+대소문자 구분) 반영. v0.1.0 = 초안(RD-1~3).

## 1. 기술 접근 (Technical Approach)

이름 셀렉터는 기존 id/code 셀렉터 위에 **가산되는 얇은 해소 레이어**이다. 핵심은 "이름 → 정확히 하나의 대상(device_id 또는 group id/code)으로 해소한 뒤 기존 개별/fan-out 경로를 재사용"이며, 제어 의미론은 재구현하지 않는다.

- **에이전트 레이어**:
  - `processRequest` 에 `DeviceName`/`GroupName`(json `device_name`/`group_name`) 필드 추가 + `fillFromParams` 승격(group_id 승격 패턴 계승).
  - 리졸버 `DeviceByName`/`GroupByName` 신설(신규 `name_resolver.go` 권장 — `agent.go`/`control.go` diff 최소화, `group_membership.go` 확장도 대안). 매칭 규칙은 trim 후 정확 일치 + 대소문자 구분(RD-6); `GroupByName` 은 전 타입 그룹(custom + 파생 station/line) 표시명 대상(RD-5). 스냅샷-안전: 로스터/그룹 레지스트리 락 취득 → 스냅샷 → 해제 → 락 미보유로 반환. 두 락 절대 중첩 금지(HVAC RWMutex 재진입 트랩 회피).
  - `handleSelectorControl`(group.go) 우선순위 게이트를 RD-4 체인(`device_id > device_name > station > line > group_id > group_name`)으로 확장: "개별 먼저"에 따라 `device_name`(개별) 은 station 앞, `group_name`(그룹) 은 group_id 뒤. `control.go` 는 불변 지향.
  - `errors.go` 에 `ErrAmbiguousName` 추가.
- **노드 레이어**(`internal/node/xsfm.go`):
  - `xsfmExtractDeviceName`/`xsfmExtractGroupName`(payload 우선, metadata 폴백) 미러 추가.
  - `buildXsfmControlCommand`: 이름 셀렉터 존재 시 top-level 방출.
  - `hasXsfmControlCommand`: 이름 셀렉터 존재 시 제어 라우팅(OR 추가).
- **무회귀 우선**: 기존 셀렉터/우선순위/집계 응답 경로는 손대지 않고 분기만 삽입한다. 기존 테스트가 회귀 게이트.

## 2. 마일스톤 (우선순위 기반, 시간 예측 없음)

### Primary Goal — M1: 에이전트 리졸버 + 디스패치 (Priority High)
- `processRequest.DeviceName`/`GroupName` 필드 + `fillFromParams` 승격.
- `DeviceByName`/`GroupByName` 리졸버(스냅샷-안전, 0/1/≥2 매치 처리, trim+대소문자 구분 RD-6, GroupByName 전 타입 매칭 RD-5).
- `errors.go` `ErrAmbiguousName`.
- `handleSelectorControl` RD-4 우선순위 체인 확장(`device_id > device_name > station > line > group_id > group_name`; device_name → 개별, group_name → fan-out).
- 산출: 이름 셀렉터로 개별/그룹 제어가 동작하고, 모호성/부재가 정확한 센티널로 거부.

### Secondary Goal — M2: 노드 pass-through (Priority High)
- `xsfmExtractDeviceName`/`xsfmExtractGroupName` + `buildXsfmControlCommand` top-level 방출 + `hasXsfmControlCommand` 라우팅.
- 산출: flow 노드가 `device_name`/`group_name` 메시지를 제어로 라우팅하고 에이전트로 top-level 전달.

### Final Goal — M3: 테스트 + 무회귀 검증 (Priority High)
- 리졸버 단위 테스트(0/1/≥2, trim), 디스패치 우선순위 테스트, 노드 pass-through/라우팅 테스트, 무회귀(기존 셀렉터) 테스트.
- `-race` 클린, xsfm 패키지 커버리지 유지(≥85% 목표), 기존 vitest/`tsc` 무회귀(프런트 미변경 시 자동 통과).

### Optional Goal — M4: 프런트엔드 이름 제어 UI (Priority Low)
- (선택) 이름 기반 제어 입력 UI. 본 SPEC 필수 아님 — 백엔드+노드 완료로 핵심 요구 충족.

## 3. 아키텍처 설계 방향

- **리졸버 = 순수 해소**: 리졸버는 이름→id 만 담당하고 제어를 실행하지 않는다. 디스패치가 해소 결과를 기존 경로에 위임한다(단일 책임).
- **우선순위는 결정론적 게이트**: `handleSelectorControl` 의 switch/순차 게이트에 이름 분기를 삽입하되, 확정 순서(RD-4: `device_id > device_name > station > line > group_id > group_name`)를 코드/주석/테스트에 명시한다.
- **노드는 위임자**: 노드는 셀렉터를 드롭하지 않고 모두 top-level 로 실어 우선순위 판정을 에이전트에 위임한다(기존 device_id/group_id 위임 패턴 계승).

## 4. 위험 및 대응

| 위험 | 영향 | 대응 |
| --- | --- | --- |
| RWMutex 재진입 deadlock (락 중첩) | fan-out 동시 실행 시 교착 | 리졸버는 스냅샷 후 락 해제 → 락 미보유로 디스패치 호출. `@MX:WARN` 주석 + `-race` 테스트로 게이트. |
| 우선순위 순서(RD-4 확정) | 다중 셀렉터 동작 정의 | 확정값 `device_id > device_name > station > line > group_id > group_name` 을 상수/주석/테스트로 고정. |
| group_name 전 타입 매칭(RD-5 확정) | 파생 그룹 표시명 충돌 시 매치 증가 | 전 타입 매칭 채택. 타입 간 충돌은 RD-2 의 `ErrAmbiguousName` 로 안전 거부. |
| 이름 충돌로 잦은 모호성 거부 | 사용성 저하 | fail-closed 정책(RD-2)은 데이터 무결성 우선. 명확한 에러 메시지로 사용자에게 id 셀렉터 안내. RD-6(대소문자 구분)으로 의도치 않은 광범위 매치도 방지. |
| CRUD `name` 오버로드 실수 | add_device/set_device 회귀 | 신규 필드 분리(A-1) + 기존 name 경로 테스트 무회귀 게이트. |

## 5. TRUST 5 적용

- **Tested**: 리졸버 0/1/≥2 매치, 디스패치 우선순위, 노드 라우팅/pass-through, 무회귀 테스트. 커버리지 ≥85% 목표.
- **Readable**: 이름 셀렉터 분기·리졸버에 한글 주석(code_comments=ko), 우선순위 순서 명시.
- **Unified**: `gofmt`/`goimports`, 기존 셀렉터/노드 패턴과 동형 구조(xsfmExtract* 미러, group_id 승격 패턴 계승).
- **Secured**: fail-closed 모호성 거부(임의 대상 제어 방지) — 잘못된 대상으로의 명령 방출 차단.
- **Trackable**: Conventional Commits + `SPEC-XSFM-NAMESEL-001` 참조, REQ ↔ 코드 anchor ↔ AC 추적표(spec §4.7).
