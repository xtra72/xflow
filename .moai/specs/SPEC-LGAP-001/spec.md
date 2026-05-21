# SPEC-LGAP-001: LG LGAP HVAC Agent

**Version**: 1.11.0
**Status**: Done
**Created**: 2026-03-17
**Updated**: 2026-05-21
**Completed**: 2026-03-17

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-05-21 | 1.11.0 | **BREAKING — protocol-prefixed metadata 키 제거**. `lgap_node_id` → `node_id`, `lgap_source` → `node_source`. 모든 노드 통일 prefix-less 표준 (HVAC + mqtt + modbus). 다운스트림: `$.metadata.lgap_*` 참조를 통일 키로 마이그레이션. |
| 2026-05-21 | 1.10.0 | **BREAKING — `lgap_source="request"` 제거**. message_type="device_state.response" 와 중복. Process 응답에서 lgap_source 라인 삭제. lgap_source="poll" 는 유지. 다운스트림: `lgap_source == "request"` → `message_type == "device_state.response"`. |
| 2026-05-21 | 1.9.0 | **BREAKING — payload.type 제거**. v0.8.0 message_type 계층형 분류로 인해 payload.type="device_state" 가 prefix 의 중복이 됨. emitDeviceStateLocked 의 payload map literal 에서 "type" 키 제거 + `sendEventLocked` 가 eventType="" 일 때 type 필드 주입 skip (transport/device_registered 등 다른 이벤트는 type 유지). 다운스트림: `$.payload.type` 검사 → `$.metadata.message_type` prefix 검사. |
| 2026-05-21 | 1.8.0 | **BREAKING — metadata.message_type 계층형 분류 + payload.trigger 제거**. v0.7.x 까지의 직교 분류 (`payload.trigger` + `metadata.message_type="event\|response"`) 가 종속 관계라는 사용자 지적으로 단일 진실원천 통합. payload.trigger → `metadata.message_type="device_state.<trigger>"` 변환 (`applyDeviceStateMessageType` 헬퍼). 값 체계: `device_state.change` / `.report` / `.keepalive` / `.init` / `.poll` + `device_state.response`. LGAP 노드의 poll/Process 모든 emit 사이트 적용. 다운스트림 필터 변경 필요. |
| 2026-05-21 | 1.7.14 | **HVAC status payload 의 nested metadata 를 message metadata 로 promote**. `promotePayloadMetadata` 헬퍼 신설. LGAP push/poll emit 사이트 적용. |
| 2026-05-21 | 1.7.8 | **5 HVAC 통합 v0.7.x**. (1) v0.7.0: 출력 schema `type:"device_state"` 단일화. LGAP `device_state_changed`/`device_state_report` 폐기 → `emitDeviceStateLocked(zone, dev, trigger)`. (2) v0.7.1: `get_all_states` → `get_all` (deprecation alias 유지). (3) v0.7.2: `recentSnapshots` cumulative buffer + `processGetRecent` 신규 — emitDeviceStateLocked 안에서 device_state payload 를 push. lastSeq cursor 기반 응답. (4) v0.7.3: `processGetStats` 추가. (5) v0.7.5: StateForJSON 의 Mode/FanSpeed 출력을 string → hvac 통일 ID (int). `lgapStateOutput` 신규 구조체. Power=false 시 0 강제. (6) v0.7.6: Manager.Restart lock holding 단축. (7) v0.7.7~v0.7.8: 노드 pollSingle byte-equal dedup + last_seen_ms 제외. |
| 2026-05-20 | 1.6.8 | **LGAP 정기 보고 (`trigger=report`) 실제 구현**. v1.2.x 까지 `notifyTicker` / report 로직 자체가 없었음. `notifyLoop` + `emitPeriodicReport` 신규 (LGCP 의 sendDeviceNotifications 패턴 차용). `sendEventLocked("device_state_report", ...)` 송신. |
| 2026-05-20 | 1.6.7 | **온도 게이트 확장**. `event_temp_threshold` 적용 — `nonTempFieldsChangedLGAP` + `maxTempDeltaLGAP` helper (RoomTemp + PipeInTemp + PipeOutTemp 의 max\|Δ\| 기반). |
| 2026-05-19 | 1.6.0 | **옵션 명칭 통일 (notify→report)**. trigger 값 `keepalive` → `report`. JSON schema 슬림화. |
| 2026-05-14 | 1.2.0 | **노드 Init-tolerance 패턴 적용** (REQ-LGAP-001-06 노드 동작 보강). `lgap`/`lgap-status`/`lgap-control` 노드가 Init 시점에 `agent_ref` 에이전트를 resolve 하지 못하면(disabled 또는 미등록) hard-fail 하지 않고 경고 로그 + Running 전이(deferred connection) 후, 에이전트 활성화 시 SPEC-ENGINE-001 `ReinitNodesForAgent` 로 자동 재연결한다. resolver 미설정(구성 오류) 및 에이전트 타입 불일치는 회복 불가능하므로 hard-fail 유지. 본 SPEC 의 EARS 요구사항 자체는 변경 없으며 노드 Init 동작만 LGCP-003 v1.1.0 / SERIAL-001 v2.2.0 / NASA-001 v1.9.0 과 동일 패턴으로 정렬. 관련: SPEC-AGENT-005 v1.1.0, SPEC-ENGINE-001 v1.3.0 Module 8. |

---

## 1. Overview

LG LGAP (LG Air-conditioner Protocol) 에이전트를 구현한다. RS-485 기반 PI-485 통신으로 LG 시스템 에어컨을 제어하며, Samsung NASA 에이전트와 동일한 아키텍처 패턴을 따른다.

### 1.1 Protocol Summary

- **Physical**: RS-485, 4800 bps, 8N1 (8 data bits, no parity, 1 stop bit)
- **Model**: Master/slave polling (동기 요청-응답, NASA 의 비동기 수신과 차이)
- **Request**: 8 bytes fixed (TX0-TX7)
- **Response**: 16 bytes fixed (RX0-RX15)
- **Checksum**: (sum of bytes % 256) XOR 0x55
- **Addressing**: Zone byte (high nibble = group, low nibble = unit)
- **Temperature**: 설정 온도 = celsius - 15, 측정 온도 = (192 - raw) / 3.0
- **Mode Combo**: TX5/RX6 bits[7:5]=mode, bits[4:2]=fan, bit1=swing

### 1.2 Key Design Decisions

- **동기 폴링 모델**: NASA(비동기 수신)과 달리 pollMu Mutex 로 시리얼 직렬 접근
- **NASA 어댑터 재사용**: `adapter.NASADeviceInfo` / `adapter.NewControllableNASADevice` 필드 호환으로 재사용
- **InterCommandDelay**: 연속 시리얼 명령 간 50ms 대기
- **Agent type**: `"lgap"` (벤더-프로토콜 패턴: samsung-nasa 와 동일)

## 2. Requirements

### REQ-LGAP-001-01: Agent Type Registration ✅
- Agent type `"lgap"` 을 에이전트 매니저에 등록한다.
- `RegisterLGLGAPTypes()` 로 구현, `cmd/xflowd/main.go` 에서 호출.

### REQ-LGAP-001-02: Serial Transport ✅
- RS-485 시리얼 통신을 지원한다 (4800 bps, 8N1).
- 지수 백오프 재연결 로직 구현 (ReconnectInterval → MaxReconnectBackoff).

### REQ-LGAP-001-03: Protocol Encoding/Decoding ✅
- 8-byte 요청 패킷, 16-byte 응답 패킷을 인코딩/디코딩한다.
- Checksum: `(sum % 256) XOR 0x55`, `CalcLGAPChecksum()` / `VerifyChecksum()`.

### REQ-LGAP-001-04: Device Management ✅
- Zone 기반 디바이스 관리 (add_device / remove_device / list_devices).
- `device.DeviceProvider` 인터페이스 구현 (`LGAPDeviceProvider`).

### REQ-LGAP-001-05: Control Commands ✅
- set_power, set_mode, set_temperature, set_fan_speed, set_multiple 명령.
- get_state, get_all_states 상태 조회.

### REQ-LGAP-001-06: Node Types ✅
- lgap-status (상태 폴링), lgap-control (제어), lgap (통합) 3종 노드.
- `internal/node/registry.go` 에 빌트인 등록 (20→23개).

### REQ-LGAP-001-07: Web UI Schema ✅
- agentSchemas.ts: `LG_LGAP_FIELDS` 10개 필드 + AGENT_TYPES 등록.
- nodeSchemas.ts: lgap-status, lgap-control, lgap 3종 스키마.
- nodeTypeMeta.ts: LGAP 노드 메타데이터 3종.

## 3. Architecture

Samsung NASA 에이전트와 동일한 패키지 구조:

```
internal/agent/lg/           (11 소스 + 6 테스트)
  register.go                - RegisterLGLGAPTypes()
  config.go                  - LGAPConfig, parseLGAPConfig()
  checksum.go                - CalcLGAPChecksum(), VerifyChecksum()
  message.go                 - LGAPRequest, LGAPResponse, 온도/모드 인코딩
  protocol.go                - LGAPProtocol interface + impl
  device.go                  - LGAPDevice, LGAPDeviceState
  transport.go               - LGAPTransport (serial, go.bug.st/serial)
  agent.go                   - LGAPAgent (pollMu 동기 폴링)
  provider.go                - LGAPDeviceProvider (NASA 어댑터 재사용)
  executor.go                - CommandExecutor closure
  errors.go                  - 15개 에러 변수
  *_test.go (6파일)          - 49개 유닛 테스트

internal/node/lgap.go        - 3종 노드 타입 (status, control, hybrid)
internal/node/errors.go      - LGAP 에러 4종 추가
internal/node/registry.go    - 빌트인 등록 (23개)
cmd/xflowd/main.go           - RegisterLGLGAPTypes() 호출

web/src/config/agentSchemas.ts    - LG_LGAP_FIELDS, AGENT_TYPES
web/src/config/nodeSchemas.ts     - lgap-status, lgap-control, lgap
web/src/pages/nodes/nodeTypeMeta.ts - LGAP 노드 메타
```

## 4. Milestones

- [x] M1: Protocol core (checksum, message, protocol, errors)
- [x] M2: Agent (config, device, agent, transport, provider, executor, register)
- [x] M3: Node types (lgap-status, lgap-control, lgap)
- [x] M4: Device adapter + registry 통합 (NASA 어댑터 재사용)
- [x] M5: Web UI schemas (agentSchemas, nodeSchemas, nodeTypeMeta)
- [x] M6: Tests (49개) + build verification (go build, tsc)
- [x] M7: Protocol 표시 수정 (NASADeviceAdapter Protocol/ExtraProperties 오버라이드, LGAP 디바이스가 "LGAP indoor"로 정상 표시)

## 5. Test Summary

| 파일 | 테스트 수 | 주요 검증 항목 |
|------|-----------|---------------|
| checksum_test.go | 3 | 체크섬 계산, 검증, 라운드트립 |
| message_test.go | 12 | 온도 인코딩/디코딩, 모드 콤보, 존 헬퍼, 맵 일관성 |
| protocol_test.go | 9 | 패킷 빌드, 디코딩, 에러 케이스 |
| config_test.go | 13 | 파싱, 기본값, 에러, YAML 호환 |
| device_test.go | 8 | 상태 업데이트, 플래그, 모드, 팬, 온도 |
| register_test.go | 2 | 등록, 중복 등록 |
| **합계** | **49** | `go test -v -race` PASS |
