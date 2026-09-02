# SPEC-MODBUS-009 — 수용 기준 (Acceptance Criteria)

> 형식: Given / When / Then. 각 REQ에 최소 1개 AC 대응. 대상: `modbus-client`(패키지 `internal/agent/modbus/`) + 프론트(`web/src/`).
> 전제: 기존 `modbus-client` type id·동작 보존(하위 호환 HARD).

---

## REQ-MODBUS-009-01 — 백엔드 `list_devices`

### AC-01 — 디바이스 목록 조회 (다중 디바이스)
- **Given** modbus-client 에이전트에 2개 이상의 디바이스가 구성되어 있고,
- **When** `Process()`에 `{"command":"list_devices"}` 명령이 발행되면,
- **Then** 응답은 각 디바이스의 id·host·port·unit_id·transport·register_groups·share_session 등 메타데이터를 담은 배열이어야 하고, 응답 형태는 프론트가 이미 소비하는 형태(AgentDetailPanel.tsx:3617의 `data[]`) 및 modbus-gateway `list_devices` 스키마와 정렬되어야 한다.

### AC-02 — 0-device 및 read-only 보장
- **Given** modbus-client 에이전트에 디바이스가 하나도 없고 폴링 goroutine이 동작 중인 상태에서,
- **When** `list_devices`가 발행되면,
- **Then** 시스템은 빈 목록을 오류 없이 반환하고, `a.mu` RLock 스냅샷으로 devices/트랜스포트/통계 맵을 **변형하지 않아야** 한다(`go test -race` 클린).

---

## REQ-MODBUS-009-02 — 백엔드 `update_device`

### AC-03 — 연결 유지 in-place 수정
- **Given** 연결된 modbus-client 디바이스(예: `dev-1`)가 특정 register_groups·poll_interval로 폴링 중이고,
- **When** `{"command":"update_device","params":{"device_id":"dev-1","register_groups":[...],"poll_interval":"500ms"}}` 이 발행되면,
- **Then** 대상 디바이스의 설정이 in-place로 변경되고, 트랜스포트/연결은 **재생성되지 않으며**(제거+재추가 아님), 변경 후 새 그룹/케이던스로 폴링이 계속되어야 한다(`set_config` 재구성 패턴 재사용).

### AC-04 — 미존재 ID·init 전용 필드 원자적 거부
- **Given** modbus-client 에이전트가 동작 중이고,
- **When** 존재하지 않는 `device_id`에 대한 `update_device`가 발행되거나, transport 전환·RTU 하드웨어 파라미터 등 init 전용 필드 변경이 요청되면,
- **Then** 시스템은 각각 `ErrDeviceNotFound` / init-전용-거부(`rejectInitOnlyFields` 규칙)로 **원자적으로 거부**하고, 부분 적용 없이 직전 상태로 계속 동작해야 한다.

---

## REQ-MODBUS-009-03 — 프론트 설정 탭 숨김

### AC-05 — modbus-client devices 편집기 미렌더 (타입 스코프)
- **Given** 실행 중인 modbus-client 에이전트의 설정 탭(`ConfigTab`)이 렌더되고,
- **When** 설정 폼이 표시되면,
- **Then** modbus-client의 `devices`(`modbus_devices` 에디터) 필드는 렌더되지 않아야 하며, 동일 화면의 다른 필드(transport/serial/케이던스 등) 및 **다른 agentType(예: modbus-gateway)의 설정 폼은 영향을 받지 않아야** 한다.

---

## REQ-MODBUS-009-04 — 프론트 장치 탭 modbus-client 섹션

### AC-06 — 전용 섹션 렌더 및 목록 표시
- **Given** modbus-client 에이전트의 장치 탭(`DevicesTab`)이 렌더되고,
- **When** agentType이 modbus-client이면,
- **Then** 전용 관리 섹션(`ModbusClientDevicesSection`)이 렌더되어 `list_devices` 결과로 현재 디바이스 목록을 표시해야 하며, modbus-gateway·xsfm·NASA/LGAP/LG-ICP의 기존 장치 탭 분기는 변경되지 않아야 한다.

### AC-07 — 추가/제거/수정 CRUD 및 편집 필드 재사용·i18n
- **Given** modbus-client 장치 탭 섹션이 렌더된 상태에서,
- **When** 사용자가 디바이스를 추가/제거/수정하면,
- **Then** 시스템은 `useExecAgent` 경로로 각각 `add_device`/`remove_device`/`update_device` 명령을 발행하고 `list_devices`로 목록을 갱신해야 하며, 추가/수정 폼은 `ModbusDevicesEditor`의 modbus 필드(id/host/port/unit_id/register_groups/per-device transport·serial/share_session)를 재사용하고, 표시 문자열은 하드코딩 없이 `agents.detail.devices.*` i18n 키를 사용해야 한다.

---

## REQ-MODBUS-009-05 — 하위 호환 · 품질 (cross-cutting)

### AC-08 — 하위 호환 (기존 동작 보존)
- **Given** per-device 필드/신규 명령을 사용하지 않는 기존 modbus-client 설정과 기존 명령(read/write/set_config/add_device/remove_device)이 있고,
- **When** 본 SPEC 구현 후 동일 설정으로 에이전트를 기동·운영하면,
- **Then** type id `modbus-client`가 보존되고, 기존 폴링·transport·session sharing·기존 명령 동작이 오늘과 동일하게 유지되어야 한다(특성화 테스트 통과).

### AC-09 — 품질 게이트 및 의존성
- **Given** 백엔드/프론트 변경이 완료된 상태에서,
- **When** 품질 게이트를 실행하면,
- **Then** `go test -race ./internal/agent/modbus/...`가 클린이고 커버리지 ≥85%이며, 프론트 `tsc`(타입 체크)와 `vitest`(단위 테스트)가 통과하고, `go.mod`에 신규 외부 modbus 라이브러리가 추가되지 않으며 `ModbusTransport.SendAndReceive` 시그니처가 변경되지 않고 i18n 키 누락이 없어야 한다.

---

## 품질 게이트 (Quality Gates) 요약

| 게이트 | 기준 |
|--------|------|
| 테스트 | 백엔드 `go test -race` 클린, 커버리지 ≥85% (hybrid: 신규 TDD / 기존 편집 특성화) |
| 프론트 | `tsc` 무오류, `vitest` 통과 |
| 하위 호환 | 기존 명령·폴링·transport·session sharing 특성화 보존 |
| 의존성 | `go.mod` 신규 modbus 모듈 0, SendAndReceive 시그니처 불변 |
| i18n | `agents.detail.devices.*` 키 정합, 누락 0, 하드코딩 0 |
| 명령 표면 | list_devices/update_device가 기존 Process 스위치 스타일(1 동사=1 case) 준수 |

## 완료 정의 (Definition of Done)

- AC-01 ~ AC-09 전량 충족.
- 품질 게이트 전부 통과.
- 설정 탭에서 modbus-client devices 숨김 + 장치 탭 전용 CRUD 섹션 동작 + 백엔드 list_devices/update_device 제공.
- SPEC-MODBUS-008 런타임 add/remove와의 정합 유지(copy-on-write, a.mu 보호).
