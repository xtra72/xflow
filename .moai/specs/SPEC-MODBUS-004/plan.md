# SPEC-MODBUS-004: Implementation Plan

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-004 |
| 제목 | MODBUS Reader/Writer Processing Node (`modbus`) |
| 상태 | Draft |
| 관련 TAG | R-MBRW-001 ~ R-MBRW-040 |

---

## 구현 전략

### 접근 방식

DDD (ANALYZE-PRESERVE-IMPROVE) 방식으로 기존 노드 시스템의 패턴을 분석하고, 기존 동작을 보존하면서 `modbus` 노드를 추가한다.

### 핵심 설계 결정

1. **단일 노드 설계**: 읽기/쓰기를 `operation` 설정으로 구분하는 단일 `modbus` 노드
2. **Agent 직접 접근**: Bridge 노드와 달리 `AgentAccessor`를 통해 `Process()` 직접 호출
3. **Agent 타입 자동 감지**: 타입 어서션으로 Server/Client Agent 자동 판별
4. **기존 패턴 재사용**: `BaseNode` 임베딩, `NodeFactory` 팩토리 패턴, `NodeOption` 옵션 패턴

---

## 마일스톤

### M1: 노드 구조체 및 팩토리 등록 (Primary Goal)

**목적**: `modbus` 노드의 기본 골격을 생성하고 Registry에 등록

**관련 TAG**: R-MBRW-001, R-MBRW-002, R-MBRW-003, R-MBRW-004, R-MBRW-005, R-MBRW-006, R-MBRW-033

**파일 변경**:
- `internal/node/modbus.go` (신규): ModbusNode, ModbusConfig 구조체, NewModbusNode 팩토리, Configure() 메서드
- `internal/node/registry.go` (수정): registerBuiltins()에 modbus 등록 (12번째 빌트인 노드)

**구현 상세**:
- `ModbusConfig` 구조체 정의 (agent_ref, operation, register_area, address, count, data_type, byte_order, device_id, timeout)
- `ModbusNode` 구조체 정의 (`BaseNode` 임베딩)
- `NewModbusNode` 팩토리 함수
- `Configure()` 메서드: config map 파싱, 유효성 검증 (operation, register_area, 읽기 전용 영역 쓰기 차단, count/address 범위 검증)
- Registry에 `{"modbus", NewModbusNode, "processing", "MODBUS 레지스터 읽기/쓰기"}` 등록

**의존성**: 없음 (첫 번째 마일스톤)

---

### M2: Agent 해석 및 타입 감지 (Primary Goal)

**목적**: AgentResolver를 통한 Agent resolve 및 Server/Client 자동 판별

**관련 TAG**: R-MBRW-007, R-MBRW-008, R-MBRW-009, R-MBRW-012

**파일 변경**:
- `internal/node/modbus.go` (수정): Init() 메서드 구현

**구현 상세**:
- `Init()` 메서드에서 `AgentResolver`를 config에서 추출 (Bridge 노드와 동일한 패턴)
- `ResolveAgent(ctx, AgentRef)` 호출로 `AgentTransport` 획득
- `AgentAccessor` 인터페이스로 타입 어서션하여 원본 `agent.Agent` 획득
- Server/Client 타입 감지:
  - `*modbusserver.MODBUSServerAgent` -> agentType = "server"
  - `*modbus.MODBUSAgent` -> agentType = "client"
  - 그 외 -> `ErrUnsupportedAgentType` 에러 반환
- timeout 설정 파싱 (`time.ParseDuration`, 기본값 5s)

**의존성**: M1 완료 필요

---

### M3: 읽기 연산 구현 (Primary Goal)

**목적**: Server Agent와 Client Agent 모두에서 레지스터 읽기 기능 구현

**관련 TAG**: R-MBRW-013, R-MBRW-014, R-MBRW-015, R-MBRW-016, R-MBRW-017, R-MBRW-018, R-MBRW-019, R-MBRW-020

**파일 변경**:
- `internal/node/modbus.go` (수정): Process() 메서드 - 읽기 로직

**구현 상세**:
- `Process()` 메서드에서 `operation` 분기
- `readFromServer()` 내부 메서드:
  - register_area -> Process() command 매핑 (get_coils, get_discrete_inputs, get_holding_registers, get_input_registers, get_register_typed)
  - data_type != "uint16" && (holding_registers || input_registers) -> `get_register_typed` 사용
  - Process() 메시지 구성 및 호출
  - 응답에서 결과 추출
- `readFromClient()` 내부 메서드:
  - register_area -> function_code 매핑 (FC01, FC02, FC03, FC04)
  - read_registers 명령 메시지 구성
  - mode: 입력 메시지 payload `read_mode` 키 확인 -> 없으면 기본값 "force"
  - device_id: 입력 메시지 payload `device_id` 키 확인 -> 없으면 config 값 사용
- 출력 메시지 구성: 원본 payload 보존 + result/register_area/address/count/data_type/agent_type 추가

**의존성**: M2 완료 필요

---

### M4: 쓰기 연산 구현 (Primary Goal)

**목적**: Server Agent와 Client Agent 모두에서 레지스터 쓰기 기능 구현

**관련 TAG**: R-MBRW-021, R-MBRW-022, R-MBRW-023, R-MBRW-024, R-MBRW-025, R-MBRW-026, R-MBRW-027, R-MBRW-028

**파일 변경**:
- `internal/node/modbus.go` (수정): Process() 메서드 - 쓰기 로직

**구현 상세**:
- `writeToServer()` 내부 메서드:
  - register_area + count -> command 매핑 (set_coil/set_coils, set_register/set_registers)
  - 입력 payload에서 value/values 추출
  - data_type, byte_order 파라미터 포함
- `writeToClient()` 내부 메서드:
  - register_area + count -> command 매핑 (write_coil/write_coils, write_register/write_registers)
  - device_id 처리 (메시지 오버라이드 -> config 기본값)
- 값 누락 에러 처리: value도 values도 없으면 에러 포트로 전달
- 출력 메시지 구성: 원본 payload 보존 + success/register_area/address/count/agent_type 추가

**의존성**: M2 완료 필요 (M3과 병렬 가능)

---

### M5: 에러 처리 및 엣지 케이스 (Secondary Goal)

**목적**: 모든 에러 시나리오에 대한 견고한 처리

**관련 TAG**: R-MBRW-029, R-MBRW-030, R-MBRW-031, R-MBRW-032, R-MBRW-034, R-MBRW-035

**파일 변경**:
- `internal/node/modbus.go` (수정): 에러 처리 로직 강화

**구현 상세**:
- `Process()` 메서드 전체를 `defer recover()` 래핑 (R-MBRW-032)
- Agent Process() 호출 에러 -> 에러 포트로 원본 메시지 + 에러 정보 전달
- Context deadline exceeded 감지 -> timeout 정보 포함 에러 메시지
- Agent 상태 체크 (Paused/Stopped) -> 상태 관련 에러 메시지
- 센티널 에러 정의:
  - `ErrUnsupportedAgentType`
  - `ErrInvalidOperation`
  - `ErrInvalidRegisterArea`
  - `ErrReadOnlyArea`
  - `ErrMissingWriteValue`
  - `ErrInvalidAddress`
  - `ErrInvalidCount`
  - `ErrAgentNotAvailable`

**의존성**: M3, M4 완료 필요

---

### M6: 프론트엔드 스키마 및 메타데이터 (Secondary Goal)

**목적**: 웹 대시보드에서 modbus 노드를 설정하고 정보를 표시할 수 있도록 프론트엔드 통합

**관련 TAG**: R-MBRW-036, R-MBRW-037, R-MBRW-038, R-MBRW-039, R-MBRW-040

**파일 변경**:
- `web/src/config/nodeSchemas.ts` (수정): modbus NodeTypeSchema 추가
- `web/src/pages/nodes/nodeTypeMeta.ts` (수정): modbus NodeTypeDetailMeta 추가

**구현 상세**:
- `nodeSchemas.ts`:
  - `NODE_TYPE_SCHEMAS` 객체에 `modbus` 키 추가
  - configSchema: agent_ref(agent_select), operation(select), register_area(select), address(number), count(number), data_type(select), byte_order(select), device_id(number)
  - defaultPorts: input, output, error
- `nodeTypeMeta.ts`:
  - `NODE_TYPE_META` 객체에 `modbus` 키 추가
  - description, ports, configFields, configExample 정의
  - 설정 예제: 읽기/쓰기 시나리오 모두 포함

**의존성**: M1 완료 필요 (백엔드와 병렬 가능)

---

### M7: 통합 테스트 (Final Goal)

**목적**: 모든 모듈의 통합 동작 검증

**관련 TAG**: R-MBRW-001 ~ R-MBRW-040 (전체)

**파일 변경**:
- `internal/node/modbus_test.go` (신규): 단위 테스트 및 통합 테스트

**구현 상세**:
- 테이블 기반 테스트 (Go table-driven tests)
- 테스트 카테고리:
  1. 설정 유효성 검증 테스트 (Configure)
  2. Agent 해석 및 타입 감지 테스트 (Init)
  3. Server Agent 읽기 테스트 (Process - read)
  4. Client Agent 읽기 테스트 (Process - read)
  5. Server Agent 쓰기 테스트 (Process - write)
  6. Client Agent 쓰기 테스트 (Process - write)
  7. 에러 처리 테스트 (에러 포트, timeout, 패닉 복구)
  8. 메시지 payload 보존 테스트
  9. device_id/read_mode 메시지 오버라이드 테스트
- Mock 구현:
  - `mockAgentResolver`: AgentResolver 인터페이스 mock
  - `mockAgentTransport`: AgentTransport + AgentAccessor 인터페이스 mock
  - `mockModbusServerAgent`: MODBUSServerAgent Process() mock
  - `mockModbusClientAgent`: MODBUSAgent Process() mock
- 커버리지 목표: 85% 이상

**의존성**: M1 ~ M5 완료 필요

---

## 마일스톤 의존성 그래프

```
M1 (노드 구조체/팩토리)
  |
  +---> M2 (Agent 해석/타입 감지)
  |       |
  |       +---> M3 (읽기 연산) ---+
  |       |                       |
  |       +---> M4 (쓰기 연산) ---+---> M5 (에러 처리) ---> M7 (통합 테스트)
  |
  +---> M6 (프론트엔드) ----------+
```

## 파일 변경 요약

| 파일 | 유형 | 마일스톤 |
|------|------|---------|
| `internal/node/modbus.go` | 신규 | M1, M2, M3, M4, M5 |
| `internal/node/registry.go` | 수정 | M1 |
| `internal/node/modbus_test.go` | 신규 | M7 |
| `web/src/config/nodeSchemas.ts` | 수정 | M6 |
| `web/src/pages/nodes/nodeTypeMeta.ts` | 수정 | M6 |

## 기술적 접근

### Agent 통신 패턴

`modbus` 노드는 Bridge 노드와 달리 Agent와의 실시간 스트리밍 연결이 아닌, 요청-응답 패턴으로 동작한다:

1. 입력 메시지가 도착하면 Process() 트리거
2. Agent의 Process() 메서드에 명령 메시지 전달
3. 응답 메시지에서 결과 추출
4. 출력 포트 또는 에러 포트로 전달

### 기존 코드와의 관계

- `BaseNode` 임베딩으로 노드 공통 기능 재사용
- `AgentResolver`/`AgentTransport`/`AgentAccessor` 인터페이스는 Bridge 노드에서 이미 정의됨 (import만 필요)
- `internal/modbus/` 패키지의 타입 변환 유틸리티 활용 가능
- `flow.AgentRef` 구조체로 Agent 참조 정보 구성

### 리스크 및 대응

| 리스크 | 영향 | 대응 방안 |
|--------|------|----------|
| Agent Process() 호출 중 패닉 | 노드 크래시 | defer recover() 래핑 |
| Agent 비정상 상태 (Paused/Stopped) | 메시지 유실 | 상태 체크 후 에러 포트로 전달 |
| Client Agent 디바이스 미연결 | 읽기/쓰기 실패 | 에러 포트로 디바이스 미발견 에러 전달 |
| 타입 변환 실패 (잘못된 data_type) | 데이터 손상 | 설정 시점 유효성 검증 + 런타임 에러 처리 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-11*
*작성: xtra*
