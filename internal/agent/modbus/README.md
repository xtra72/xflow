# internal/agent/modbus - MODBUS/TCP 클라이언트 에이전트

## 개요

`internal/agent/modbus`는 xflow FBP 플랫폼의 MODBUS/TCP 클라이언트 에이전트 구현체이다. `agent.Agent`, `agent.MessageReceiver`, `agent.StatefulAgent` 인터페이스를 구현하며, 다중 디바이스 연결과 주기적 폴링을 통해 MODBUS 서버의 레지스터 데이터를 수집한다.

주요 기능으로 다중 디바이스 관리, 3가지 읽기 모드(direct/cached/force), 이벤트/인터벌 전송 모드, Cache-Level TypeOverlay 패턴, 쓰기 명령(FC05/FC06/FC15/FC16)을 지원한다.

## 주요 타입

### ModbusAgent

MODBUS/TCP 클라이언트 에이전트 구조체이다. `lifecycle.BaseLifecycle`을 임베딩하여 생명주기를 관리한다.

- `NewModbusAgent(agentConfig) (agent.Agent, error)`: 팩토리 함수
- 에이전트 타입: `"modbus-tcp"`

### ModbusConfig

클라이언트 에이전트의 설정 구조체이다.

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `Mode` | `string` | `"interval"` | 전송 모드: `"interval"` (매 폴 전송) / `"event"` (변경 시만 전송) |
| `ReadMode` | `string` | `"cached"` | 읽기 모드: `"direct"` (직접 읽기) / `"cached"` (캐시 사용) |
| `PollInterval` | `Duration` | `5s` | 폴링 주기 |
| `HeartbeatInterval` | `Duration` | `60s` | event 모드 전체 데이터 전송 주기 |
| `StaleThreshold` | `Duration` | `PollInterval*3` | 데이터 신선도 기준 |
| `WriteTimeout` | `Duration` | `5s` | 쓰기 응답 대기 시간 |
| `EnableWriteEvents` | `bool` | `true` | 쓰기 이벤트 발행 여부 |
| `ReconnectInterval` | `Duration` | `10s` | 디바이스 재연결 간격 |
| `MaxRetries` | `int` | `3` | 최대 재시도 횟수 |
| `RequestTimeout` | `Duration` | `3s` | 요청 타임아웃 |
| `MsgChannelSize` | `int` | `256` | 메시지 채널 버퍼 크기 |
| `Devices` | `[]DeviceConfig` | - | 디바이스 목록 (필수) |

### DeviceConfig / RegisterGroupConfig

디바이스 및 레지스터 그룹 설정이다.

```go
type DeviceConfig struct {
    ID             string
    Host           string              // 필수
    Port           int                 // 기본값 502
    UnitID         byte
    RegisterGroups []RegisterGroupConfig
}

type RegisterGroupConfig struct {
    Name         string
    FunctionCode byte                  // 1, 2, 3, 4
    StartAddress uint16
    Quantity     uint16                // 필수, > 0
    DataType     string               // 그룹 기본 데이터 타입 (기본: "uint16")
    TypeMap      []modbus.TypeMapEntry // 주소별 타입 오버라이드 (선택)
}
```

### RegisterCache

디바이스별 레지스터 값의 인메모리 캐시이다. `sync.RWMutex` 기반 동시성 안전을 보장한다.

- 4개 영역: `Coils`, `DiscreteInputs`, `HoldingRegisters`, `InputRegisters`
- `LastUpdateTime`: 레지스터 그룹별 마지막 갱신 시각 (키: `"FC{code}_{startAddr}"`)
- Cache-Level TypeOverlay: 키 형식 `"FC{code}:{address}"` (예: `"FC3:0"`)
- `CompareAndUpdate()`: 이벤트 모드 변경 감지 및 `typed_changed_values` 생성

## Process 명령

`Process(data []byte)` 메서드로 JSON 명령을 처리한다.

| 명령 | 설명 |
|------|------|
| `read_registers` | 디바이스 레지스터 읽기 (모드에 따라 캐시/직접/강제 읽기) |
| `get_status` | 전체 디바이스 상태 반환 |
| `get_cache` | 지정 디바이스 캐시 스냅샷 반환 |
| `get_all_caches` | 모든 디바이스 캐시 스냅샷 반환 |
| `write_coil` | 단일 코일 쓰기 (FC05) |
| `write_register` | 단일 레지스터 쓰기 (FC06, 2-레지스터 타입 시 FC16 자동 전환) |
| `write_coils` | 다중 코일 쓰기 (FC15) |
| `write_registers` | 다중 레지스터 쓰기 (FC16, `data_type` 지정 가능) |

## 읽기 모드

| 모드 | 조건 | 동작 |
|------|------|------|
| `cached` | `force=false` | 캐시된 값 반환 (디바이스 쿼리 없음) |
| `force` | `force=true` | 디바이스에서 직접 읽고 캐시 갱신 후 반환 |
| `direct` | `read_mode="direct"` | 항상 디바이스에서 직접 읽어 반환 |

## 전송 모드

| 모드 | 동작 |
|------|------|
| `interval` | 매 폴링 주기마다 전체 레지스터 데이터를 `register_data` 이벤트로 전송 |
| `event` | 변경 감지 시 `register_changed` 이벤트 전송, heartbeat 주기마다 전체 전송 |

## TypeOverlay (Cache-Level)

클라이언트 에이전트의 TypeOverlay는 Cache-Level 해석 계층이다. 캐시 스냅샷과 이벤트에 `typed_values` 또는 `typed_changed_values` 필드로 타입 변환된 값을 포함한다.

- `buildCacheTypeOverlay()`: 디바이스의 RegisterGroup 설정에서 TypeOverlay 맵 구축
- `filterTypedValuesByRange()`: 그룹 주소 범위에 속하는 typed_values만 필터링
- FC06 -> FC16 자동 전환: 2-레지스터 타입(float32, uint32, int32) 쓰기 시 자동 처리
- Write-Through 캐시 갱신: 쓰기 성공 시 캐시를 즉시 갱신

## 에러 타입

| 에러 변수 | 설명 |
|-----------|------|
| `ErrReadOnlyRegister` | 읽기 전용 레지스터 쓰기 시도 |
| `ErrAddressOutOfRange` | 레지스터 주소 범위 초과 |
| `ErrQuantityExceeded` | 요청 수량 최대 허용치 초과 |
| `ErrWriteTimeout` | 쓰기 응답 대기 시간 초과 |
| `ErrModbusException` | MODBUS 디바이스 예외 응답 |
| `ErrDeviceOffline` | 디바이스 오프라인 상태 |
| `ErrDeviceNotFound` | 디바이스를 찾을 수 없음 |
| `ErrInvalidFunctionCode` | 유효하지 않은 기능 코드 |
| `ErrInvalidCommand` | 유효하지 않은 명령 |
| `ErrConnectionFailed` | TCP 연결 실패 |
| `ErrFrameTooShort` | 응답 프레임 최소 길이 미달 |
| `ErrCacheNotFound` | 디바이스 캐시를 찾을 수 없음 |
| `ErrUnsupportedDataType` | 지원하지 않는 데이터 타입 |
| `ErrTypeMapOverlap` | TypeMap 주소 겹침 |
| `ErrTypeMapOutOfRange` | TypeMap 주소 범위 초과 |

## 파일 구성

| 파일 | 설명 |
|------|------|
| `agent.go` | ModbusAgent 구조체, 생명주기, 폴링, Process() 명령 디스패치, TypeOverlay 초기화 |
| `cache.go` | RegisterCache, TypeOverlay 지원, CompareAndUpdate, buildTypedValues |
| `config.go` | ModbusConfig/DeviceConfig/RegisterGroupConfig 파싱, TypeMap 파싱 및 검증 |
| `device.go` | ModbusDevice 관리, 연결/재연결, 레지스터 읽기, 트랜잭션 ID 관리 |
| `protocol.go` | MODBUS 프로토콜 프레임 인코딩/디코딩, 기능 코드 상수 |
| `transport.go` | TCP 트랜스포트, 프레임 송수신 |
| `write.go` | 쓰기 명령 처리 (FC05/FC06/FC15/FC16), 파라미터 추출 헬퍼, FC06->FC16 자동 전환 |
| `errors.go` | 15개 sentinel 에러 정의 |
| `register.go` | `RegisterModbusTypes()` 에이전트 타입 등록 |
| `agent_test.go` | 에이전트 통합 테스트 |
| `cache_test.go` | RegisterCache, TypeOverlay 테스트 |
| `config_test.go` | 설정 파싱 및 검증 테스트 |
| `protocol_test.go` | 프로토콜 인코딩/디코딩 테스트 |
| `write_test.go` | 쓰기 명령 처리 테스트 |

## 테스트

```bash
go test -v -race -cover ./internal/agent/modbus/...
```

- 커버리지: 86.2%
- Race Detector: 이상 없음

## 의존성

| 패키지 | 용도 |
|--------|------|
| `internal/modbus` | 데이터 타입 변환, TypeMapEntry, TypeOverlayEntry |
| `internal/agent` | Agent, MessageReceiver, StatefulAgent 인터페이스 |
| `pkg/lifecycle` | BaseLifecycle 생명주기 관리 |
| `log/slog` | 구조화 로깅 |
| `sync` | RWMutex 기반 동시성 제어 |
| `net` | TCP 연결 관리 |
| `encoding/json` | JSON 요청/응답 처리 |
| `encoding/binary` | MODBUS 프레임 바이너리 인코딩 |

## SPEC 문서

- SPEC ID: SPEC-MODBUS-001 (클라이언트 에이전트 기본 구현)
- SPEC ID: SPEC-MODBUS-003 (TypeOverlay 패턴 추가)
- 상태: 구현 완료
