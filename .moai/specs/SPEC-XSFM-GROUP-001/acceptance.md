---
id: SPEC-XSFM-GROUP-001
title: "xsfm 그룹 1급 개념 도입 — 인수 기준"
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
tags: "xsfm, group, acceptance, given-when-then, fan-out, frontend"
---

# SPEC-XSFM-GROUP-001 인수 기준: xsfm 그룹 1급 개념 도입

> 각 시나리오는 Given-When-Then 형식이며 기계 검증 가능(Go `testing`/`testify`, 프런트 `vitest`)해야 한다.

## 1. Module 1 — 그룹 레지스트리 · 엔티티

### Scenario 1.1: 그룹 엔티티 생성·조회
- Given: 빈 그룹 레지스트리
- When: `UpsertGroup{ID:"custom:a", Name:"A", Type:"custom", Members:["d1"]}`
- Then: `GetGroup("custom:a")` 가 동일 엔티티를 반환하고 `ListGroups` 에 1건 포함.

### Scenario 1.2: 별도 레이어 — 기존 레지스트리 불변
- Given: station 레지스트리·디바이스 로스터가 구성됨
- When: 그룹 레지스트리 CRUD 를 수행
- Then: `station_registry` 저장소·`device_registry.json` 포맷/내용은 그룹 CRUD 로 변경되지 않는다.

### Scenario 1.3: 커스텀 그룹 멤버 영속 write-through
- Given: 저장소 경로가 설정된 그룹 레지스트리
- When: `add_group` 후 프로세스 재기동 및 로드
- Then: 커스텀 그룹과 멤버가 복원된다(atomic write, REQ-01-06/NF-01).

### Scenario 1.4: 락 중첩 없음 (`-race`)
- Given: 그룹 조회와 fan-out 을 동시 실행
- When: `go test -race`
- Then: race/deadlock 미검출(REQ-01-07/NF-04).

## 2. Module 2 — 다대다 멤버십

### Scenario 2.1: 디바이스 다중 소속
- Given: 디바이스 `d1`(Station="S", 커스텀 그룹 `custom:x` 멤버)
- When: `GroupsForDevice("d1")`
- Then: 최소 `station:S`(파생) + `custom:x` 를 모두 포함.

### Scenario 2.2: 커스텀 멤버 도출
- Given: `custom:x` 의 members=["d1","d2"]
- When: `GroupMembers("custom:x")`
- Then: ["d1","d2"] 반환(정렬).

### Scenario 2.3: 기본 그룹 파생 도출
- Given: `d1`,`d2` 의 Station="S", 역사 그룹 `station:S`
- When: `GroupMembers("station:S")`
- Then: `DevicesByStation("S")` 와 동일 결과.

### Scenario 2.4: 기존 group_id 마이그레이션 + primary 유지 (RD-1)
- Given: 레거시 로스터에 `Device.GroupID="legacy1"` 인 `d1`
- When: Init 로드
- Then: (1) `custom:legacy1` 그룹이 생성되고 `d1` 이 멤버로 편입(REQ-02-04), (2) `d1.GroupID` 는 "legacy1" 로 **유지**(primary — 폐기되지 않음).

### Scenario 2.5: 위치 변경 즉시 반영 (파생)
- Given: `d1` Station="S"
- When: `set_device{station:"T"}`
- Then: `GroupMembers("station:S")` 에서 `d1` 제외, `GroupMembers("station:T")` 에 포함(REQ-04-05).

### Scenario 2.6: 유령 멤버 조회 필터 (RD-3)
- Given: `custom:x` members=["d1","d2"], `d2` 를 `remove_device`
- When: `GroupMembers("custom:x")` (또는 group_id fan-out)
- Then: 삭제된 `d2` 는 결과에 나타나지 않는다(로스터 대조 필터). `remove_device` 는 그룹 멤버 목록을 직접 건드리지 않는다(비침습, REQ-02-06).

### Scenario 2.7: primary 그룹 단일 태그 방출 무회귀 (RD-1)
- Given: `d1.GroupID="legacy1"` 이 커스텀 그룹 + station 그룹에 다중 소속
- When: status/telemetry/이벤트 방출(status.go/monitor.go/provider.go)
- Then: 단일 `group_id` 태그가 primary("legacy1")로 방출되며, 기존 방출 포맷/경로가 회귀 없이 동일하다(REQ-02-07). InfluxDB 태그는 단일 값 유지.

### Scenario 2.8: 그룹 id 접두사 인코딩 (RD-2)
- Given: 커스텀 그룹 생성 + 역사 "S" 추가
- When: `list_groups`
- Then: 그룹 id 가 `custom:<...>` / `station:S` 형태로 접두사 인코딩되며, 접두사만으로 type 이 판별된다. 커스텀 name 이 station 코드와 동일해도 id 충돌이 없다(REQ-01-05).

## 3. Module 3 — 커스텀 그룹 CRUD

### Scenario 3.1: add_group
- When: `{command:"add_group", params:{name:"2층", members:["d1"]}}`
- Then: `{status:"ok", group_id}` + `group_registered` 이벤트 + 목록 반영.

### Scenario 3.2: set_group 부분 갱신
- Given: `custom:x`(name="A", members=["d1"])
- When: `{command:"set_group", params:{group_id:"custom:x", members:["d1","d2"]}}`
- Then: name 은 "A" 보존, members=["d1","d2"](REQ-03-03).

### Scenario 3.3: remove_group
- When: `{command:"remove_group", params:{group_id:"custom:x"}}`
- Then: `{status:"ok"}` + `group_unregistered` + 목록에서 제거 + 영속 반영.

### Scenario 3.4: list_groups (기본+커스텀)
- Given: station 그룹 1 + 커스텀 그룹 1
- When: `list_groups`
- Then: 두 그룹이 `{id,name,type,member_count,members}` 로 결정적 순서 반환.

### Scenario 3.5: 기본 그룹 편집 거부
- When: type=station 그룹에 `set_group`/`remove_group` 시도
- Then: `ErrGroupNotCustom` 반환, 변경 없음(REQ-03-05).

### Scenario 3.6: 미등록 그룹 거부
- When: 존재하지 않는 group_id 로 `set_group`/`remove_group`
- Then: `ErrGroupNotFound`, 부분 변경 없음(REQ-03-06).

## 4. Module 4 — 기본 그룹 자동 동기화

### Scenario 4.1: 역사 추가 시 station 그룹 생성
- When: `add_station{station:"S", display_name:"S역"}`
- Then: `list_groups` 에 type=station, name="S역" 그룹이 자동 노출(REQ-04-01).

### Scenario 4.2: 역사 삭제 시 station 그룹 제거
- Given: station 그룹 존재
- When: `remove_station{station:"S"}`
- Then: 대응 station 그룹이 목록에서 사라진다(REQ-04-02).

### Scenario 4.3: 역사 표시명 변경 동기화
- When: `add_station{station:"S", display_name:"새이름"}`(upsert)
- Then: station 그룹 name 이 "새이름"으로 갱신(REQ-04-03).

### Scenario 4.4: line 그룹 파생 노출
- Given: station "S1","S2" 가 line "L1"
- When: `list_groups`
- Then: type=line, ref="L1" 그룹이 노출되고 `GroupMembers` 가 `DevicesByLine("L1")` 와 일치(REQ-04-04).

### Scenario 4.5: 마지막 역사 제거 시 line 그룹 소멸
- Given: line "L1" 의 유일 역사 "S1"
- When: `remove_station{station:"S1"}`
- Then: line "L1" 그룹이 더 이상 노출되지 않는다.

## 5. Module 5 — 그룹 셀렉터 일괄 제어

### Scenario 5.1: 커스텀 그룹 fan-out
- Given: `custom:x` members=["d1","d2"] 온라인
- When: `{command:"set_power", params:{group_id:"custom:x", power:true}}`
- Then: 각 멤버로 fan-out, 집계 응답 `{selector:{type:"group_id",value:"custom:x"}, results:[...], status:"ok"}`(REQ-05-01/02).

### Scenario 5.2: 기본(station) 그룹 fan-out
- When: group_id=station 그룹으로 제어
- Then: `DevicesByStation` 멤버로 fan-out(REQ-05-01, A-5).

### Scenario 5.3: 빈 그룹 no-op
- Given: 멤버 0 그룹
- When: group_id 제어
- Then: 방출 없이 `ErrEmptyGroup`(REQ-05-04).

### Scenario 5.4: 미등록 그룹 거부
- When: 미등록 group_id 제어
- Then: `ErrGroupNotFound`(REQ-05-05).

### Scenario 5.5: 셀렉터 우선순위 불변
- When: `device_id` + `group_id` 동시 지정
- Then: 단일 device_id 경로로 처리, fan-out 없음(REQ-05-03, 기존 우선순위 계승).

### Scenario 5.6: 부분 실패 best-effort
- Given: 멤버 중 일부 timeout
- When: group_id fan-out
- Then: 성공/timeout 혼재 시 최상위 status="partial", 멤버별 결과 표기(기존 의미론 계승).

### Scenario 5.7: 셀렉터 병존 동일 결과 (RD-4)
- Given: 역사 "S" 에 `d1`,`d2` 소속
- When: (a) `{station:"S"}` 셀렉터로 제어, (b) `{group_id:"station:S"}` 셀렉터로 제어
- Then: 두 경로가 모두 허용되고 동일한 `DevicesByStation("S")` 대상으로 수렴하여 fan-out 결과(대상 집합·집계)가 동일하다(REQ-05-06). line("L1") vs `group_id:"line:L1"` 도 동일.

## 6. Module 6 — 프런트엔드

### Scenario 6.1: 그룹 탭 존재
- When: xsfm 에이전트 상세 렌더
- Then: 디바이스/역사 탭과 나란히 그룹 탭이 표시된다(REQ-06-01).

### Scenario 6.2: 그룹 CRUD UI
- When: 그룹 탭에서 생성/수정/삭제
- Then: add_group/set_group/remove_group 호출 + 목록 갱신(REQ-06-02).

### Scenario 6.3: 멤버 편집 (커스텀만)
- When: 커스텀 그룹 멤버 편집 저장
- Then: set_group{members} 호출; 기본 그룹은 멤버 편집 UI 가 비활성/읽기 전용(REQ-06-03).

### Scenario 6.4: 그룹 일괄 제어 UI
- When: 그룹을 대상으로 OFF/풍량 버튼 클릭
- Then: group_id 셀렉터로 제어 호출, `ControlResultView` 로 집계 결과 표시(REQ-06-04).

### Scenario 6.5: 설비 그룹 패널 개편
- When: 대시보드에 설비 그룹 패널 추가
- Then: 역사가 type=station 그룹으로 표시되고 커스텀/라인 그룹도 표시·제어 가능(REQ-06-05). 기존 패널 테스트가 갱신되어 통과.

## 7. 비기능 · 회귀

### Scenario 7.1: 기존 xsfm 테스트 무회귀
- When: `go test ./internal/agent/xsfm/...`
- Then: 기존 테스트 전량 통과(station/line 레지스트리·로스터·기존 fan-out 무회귀, NF-02).

### Scenario 7.2: 다대다 정합
- Given: `d1` 이 station 그룹 + 커스텀 그룹 2개 소속
- When: 각 그룹 조회
- Then: `d1` 이 모든 해당 그룹에 일관되게 나타난다(NF-03).

### Scenario 7.3: 커버리지 게이트
- When: `go test -cover ./internal/agent/xsfm/...`
- Then: 신규/변경 코드 커버리지 ≥ 85%.

## 8. Definition of Done

- REQ-XSFM-GROUP-001-01~06 + NF-01~04 전부 시나리오로 커버·통과.
- 확정 설계 결정 RD-1~4 검증 시나리오 통과: primary 방출 무회귀(2.7), 접두사 id(2.8), 유령 멤버 필터(2.6), 셀렉터 병존 동일 결과(5.7).
- `gofmt`/`goimports`/`golangci-lint` clean, `go test -race` 통과.
- 프런트 `vitest` 통과(그룹 탭·훅·패널).
- 기존 SPEC-XSFM-001·SPEC-FACILITY-DASHBOARD-001 기능 무회귀.

> 열린 질문 OQ-1~4 는 v0.2.0 에서 사용자 확정(RD-1~4)되어 미해결 이슈 없음.
