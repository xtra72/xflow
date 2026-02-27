# internal/agent/modbusserver - MODBUS/TCP 서버 에이전트

## 개요

`internal/agent/modbusserver`는 xflow FBP 플랫폼의 MODBUS/TCP 서버 에이전트 구현체이다. `agent.Agent`, `agent.MessageReceiver`, `agent.StatefulAgent` 인터페이스를 구현하며, TCP 소켓 리스너를 통해 MODBUS 클라이언트의 요청을 수신하고 처리한다.

주요 기능으로 코일, 보유 레지스터, 입력 레지스터, 이산 입력의 4개 영역을 지원하고, TypeMap을 통한 다중 데이터 타입 설정과 Bridge-Level TypeOverlay 패턴을 제공한다.

## 주요 타입

### ModbusServerAgent

MODBUS/TCP 서버 에이전트 구조체이다. `lifecycle.BaseLifecycle`을 임베딩하여 생명주기를 관리한다.

- `NewModbusServerAgent(agentConfig) (agent.Agent, error)`: 팩토리 함수
- 에이전트 타입: `"modbus-tcp-server"`

### ModbusServerConfig

서버 에이전트의 설정 구조체이다.

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `ListenAddress` | `string` | `"0.0.0.0"` | 리슨 주소 |
| `ListenPort` | `int` | `502` | 리슨 포트 (0-65535) |
| `UnitID` | `byte` | `1` | 유닛 ID (0-247) |
| `MaxConnections` | `int` | `10` | 최대 동시 연결 수 |
| `IdleTimeout` | `Duration` | `60s` | 유휴 연결 타임아웃 |
| `MsgChannelSize` | `int` | `256` | 메시지 채널 버퍼 크기 |
| `RegisterMap` | `RegisterMapConfig` | - | 레지스터 맵 설정 (필수) |

### RegisterMap

MODBUS 서버의 공유 레지스터 맵이다. `sync.RWMutex` 기반 동시성 안전한 읽기/쓰기를 제공한다.

- 4개 영역: `coils`, `discrete_inputs`, `holding_registers`, `input_registers`
- Bridge-Level TypeOverlay: 키 형식 `"area:address"` (예: `"holding_registers:0"`)
- `ChangeSet` 기반 값 변경 추적

### RegisterAreaConfig

단일 레지스터 영역의 설정이다.

| 필드 | 타입 | 설명 |
|------|------|------|
| `StartAddress` | `uint16` | 시작 주소 |
| `Count` | `uint16` | 레지스터 수 (필수, > 0) |
| `InitialValues` | `[]any` | 초기값 (선택) |
| `DataType` | `string` | 영역 기본 데이터 타입 (기본: `"uint16"`) |
| `TypeMap` | `[]TypeMapEntry` | 주소별 타입 오버라이드 (선택) |

## Process 명령

`Process(data []byte)` 메서드로 JSON 명령을 처리한다.

| 명령 | 설명 |
|------|------|
| `set_coil` | 단일 코일 값 설정 |
| `set_coils` | 다중 코일 값 설정 |
| `set_register` | 단일 보유 레지스터 설정 (타입 지정 가능) |
| `set_registers` | 다중 보유 레지스터 설정 (타입 지정 가능) |
| `set_input` | 입력 레지스터 또는 이산 입력 설정 (타입 지정 가능) |
| `set_inputs` | 다중 입력 레지스터 또는 이산 입력 설정 (타입 지정 가능) |
| `get_register_typed` | 타입 변환된 레지스터 값 읽기 |
| `get_map` | 레지스터 맵 스냅샷 반환 |
| `get_status` | 서버 상태 반환 |

타입이 지정된 쓰기 명령은 TypeOverlay 또는 명시적 `data_type` 파라미터를 통해 자동으로 다중 레지스터 타입(float32, uint32, int32)을 처리한다.

## 지원 기능 코드

| 기능 코드 | 설명 | 읽기/쓰기 |
|-----------|------|-----------|
| FC01 | Read Coils | 읽기 |
| FC02 | Read Discrete Inputs | 읽기 |
| FC03 | Read Holding Registers | 읽기 |
| FC04 | Read Input Registers | 읽기 |
| FC05 | Write Single Coil | 쓰기 |
| FC06 | Write Single Register | 쓰기 |
| FC15 | Write Multiple Coils | 쓰기 |
| FC16 | Write Multiple Registers | 쓰기 |

## TypeOverlay (Bridge-Level)

서버 에이전트의 TypeOverlay는 Bridge-Level 해석 계층이다. 내부 저장소는 항상 `uint16` 레지스터이며, TypeOverlay는 읽기/쓰기 시점에 타입 해석을 수행한다.

- `WriteTyped()`: 타입 값을 uint16 레지스터로 변환하여 저장
- `ReadTyped()`: uint16 레지스터를 타입 값으로 변환하여 반환
- 우선순위: 명시적 `data_type` 파라미터 > TypeOverlay > `"uint16"` 기본값

## 에러 타입

| 에러 변수 | 설명 |
|-----------|------|
| `ErrServerAlreadyRunning` | 서버가 이미 실행 중 |
| `ErrListenFailed` | TCP 리슨 실패 |
| `ErrMaxConnectionsReached` | 최대 연결 수 도달 |
| `ErrInvalidRegisterMap` | 유효하지 않은 레지스터 맵 설정 |
| `ErrAddressNotMapped` | 매핑되지 않은 주소 요청 |
| `ErrReadOnlyArea` | 읽기 전용 영역 쓰기 시도 |
| `ErrInvalidCommand` | 유효하지 않은 명령 |
| `ErrTypeMapOverlap` | TypeMap 주소 겹침 |
| `ErrTypeMapOutOfRange` | TypeMap 주소 범위 초과 |
| `ErrUnsupportedDataType` | 지원하지 않는 데이터 타입 |

## 파일 구성

| 파일 | 설명 |
|------|------|
| `agent.go` | ModbusServerAgent 구조체, 생명주기, Process() 명령 디스패치, 이벤트 전송 |
| `config.go` | ModbusServerConfig 파싱, RegisterMapConfig, TypeMap 파싱 및 검증 |
| `register_map.go` | RegisterMap 구현, TypeOverlay 구축, ReadTyped/WriteTyped, ChangeSet |
| `handler.go` | MODBUS TCP 요청 핸들러 (FC01-06, FC15-16 처리) |
| `listener.go` | TCP 리스너 관리, 연결 수 제한, 유휴 타임아웃 |
| `request.go` | MODBUS 요청 프레임 파싱 및 응답 인코딩 |
| `errors.go` | 10개 sentinel 에러 정의 |
| `register.go` | `RegisterModbusServerTypes()` 에이전트 타입 등록 |
| `agent_test.go` | 에이전트 통합 테스트 |
| `config_test.go` | 설정 파싱 및 검증 테스트 |
| `register_map_test.go` | RegisterMap 읽기/쓰기, TypeOverlay 테스트 |
| `handler_test.go` | MODBUS 핸들러 테스트 |
| `listener_test.go` | TCP 리스너 테스트 |
| `request_test.go` | 요청/응답 프레임 파싱 테스트 |

## 테스트

```bash
go test -v -race -cover ./internal/agent/modbusserver/...
```

- 커버리지: 89.3%
- Race Detector: 이상 없음

## 의존성

| 패키지 | 용도 |
|--------|------|
| `internal/modbus` | 데이터 타입 변환, TypeMapEntry, TypeOverlayEntry |
| `internal/agent` | Agent, MessageReceiver, StatefulAgent 인터페이스 |
| `pkg/lifecycle` | BaseLifecycle 생명주기 관리 |
| `log/slog` | 구조화 로깅 |
| `sync` | RWMutex 기반 동시성 제어 |
| `net` | TCP 소켓 리스너 |
| `encoding/json` | JSON 요청/응답 처리 |

## SPEC 문서

- SPEC ID: SPEC-MODBUS-002 (서버 에이전트 기본 구현)
- SPEC ID: SPEC-MODBUS-003 (TypeOverlay 패턴 추가)
- 상태: 구현 완료
