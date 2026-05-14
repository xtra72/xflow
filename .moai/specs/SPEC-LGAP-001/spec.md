# SPEC-LGAP-001: LG LGAP HVAC Agent

**Version**: 1.2.0
**Status**: Done
**Created**: 2026-03-17
**Updated**: 2026-05-14
**Completed**: 2026-03-17

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-17 | 1.0.0 ~ 1.1.0 | 초기 작성 및 노드 타입 추가 |
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
