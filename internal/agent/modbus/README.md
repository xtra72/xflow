# internal/agent/modbus - MODBUS 클라이언트 에이전트 (TCP / RTU)

## 개요

`internal/agent/modbus`는 xflow FBP 플랫폼의 MODBUS 클라이언트 에이전트 구현체이다. `agent.Agent`, `agent.MessageReceiver`, `agent.StatefulAgent` 인터페이스를 구현하며, 다중 디바이스 연결과 주기적 폴링을 통해 MODBUS 서버의 레지스터 데이터를 수집한다. type id 는 `modbus-client` 이며, `transport` 설정으로 TCP 또는 RTU(시리얼) 트랜스포트를 선택한다.

주요 기능으로 다중 디바이스 관리, **트랜스포트 선택(TCP/RTU)**, 3가지 읽기 모드(direct/cached/force), 이벤트/인터벌 전송 모드, **레지스터 그룹별 독립 폴링 주기**, **`raw` + 4순열 바이트순서 데이터 타입 변환**, Cache-Level TypeOverlay 패턴, 쓰기 명령(FC05/FC06/FC15/FC16), **플로우 노드를 통한 런타임 재구성(`set_config`)** 을 지원한다.

> **트랜스포트 선택**: `transport: tcp | rtu`(`Transport.Options`). 미지정 시 `tcp` 로 해석되어 기존 동작이 바이트 동일하게 유지된다(하위 호환). RTU 는 CRC-16(poly 0xA001, init 0xFFFF) + `[unitID][PDU][CRC-lo][CRC-hi]` ADU 프레이밍을 사용하는 in-house 반이중 시리얼 마스터이며, 시리얼 버스당 단일 공유 인스턴스 + 단일 turnaround mutex 로 동작한다(multi-drop). 외부 modbus 라이브러리 없이 `go.bug.st/serial`(century 재사용)만 사용한다.

## 주요 타입

### ModbusAgent

MODBUS/TCP 클라이언트 에이전트 구조체이다. `lifecycle.BaseLifecycle`을 임베딩하여 생명주기를 관리한다.

- `NewModbusAgent(agentConfig) (agent.Agent, error)`: 팩토리 함수
- 에이전트 타입: `"modbus-client"`

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
    Host           string              // TCP 필수 (RTU 시 무시)
    Port           int                 // 기본값 502 (TCP)
    UnitID         byte                // 생성 시드 (런타임 판독은 dev.UnitID())
    RegisterGroups []RegisterGroupConfig
}

type RegisterGroupConfig struct {
    Name         string
    FunctionCode byte                  // 1, 2, 3, 4
    StartAddress uint16
    Quantity     uint16                // 필수, > 0
    DataType     string               // 그룹 기본 데이터 타입 (기본: "uint16", "raw" 지원)
    PollInterval Duration             // 그룹별 폴링 주기 (선택, 미지정 시 에이전트/디바이스 기본 주기로 폴백)
    TypeMap      []modbus.TypeMapEntry // 주소별 타입 오버라이드 (선택)
}
```

### 트랜스포트 설정 (`Transport.Options`)

| 필드 | 적용 | 기본값 | 설명 |
|------|------|--------|------|
| `transport` | 공통 | `"tcp"` | 트랜스포트 선택: `"tcp"` / `"rtu"`. 미지정 시 `tcp`(하위 호환) |
| `port` | RTU | - | 시리얼 포트 경로 (예: `/dev/ttyUSB0`). **init 전용** |
| `baud` | RTU | - | baudrate. **init 전용** |
| `data_bits` | RTU | - | 데이터 비트. **init 전용** |
| `stop_bits` | RTU | - | 정지 비트. **init 전용** |
| `parity` | RTU | - | 패리티. **init 전용** |

> **init 전용 필드**: 트랜스포트 `tcp↔rtu` 전환 및 RTU 시리얼 하드웨어 파라미터(port/baud/data_bits/stop_bits/parity)는 트랜스포트 오픈에 귀속되므로 런타임 `set_config` 로 변경할 수 없다(요청 시 오류 반환, 부분 적용 없음).

### 데이터 타입 / 바이트순서

| 지정 | 동작 |
|------|------|
| `uint16`/`int16`/`uint32`/`int32`/`float32` | 기존 타입 변환 |
| `raw` | 무변환 — 읽은 워드(uint16 배열)를 그대로 전달 |
| 바이트순서 4순열 | `ABCD` / `BADC` / `CDAB` / `DCBA` (word-swap × byte-swap) |
| `big_endian` (별칭) | `ABCD` 와 동일 (하위 호환 word-swap 별칭) |
| `little_endian` (별칭) | `CDAB` 와 동일 (하위 호환 word-swap 별칭) |

> 알 수 없는 타입/바이트순서 지정은 잘못된 값을 반환하지 않고 파싱/설정 오류로 처리된다. float64/uint64(4워드) 디코딩은 Optional 로 미구현이다.

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
| `set_config` | 런타임 재구성 (에이전트 재시작 없이 런타임 가변 필드 갱신) |

### `set_config` 런타임 재구성

플로우 노드가 `agent_ref` → `Process([]byte)`(기존 제네릭 `callAgentProcess` 경로)로 발행하는 재구성 명령이다. init 경로(`parseModbusConfig`)와 동일한 파싱/검증 규칙을 재사용하며, `a.mu`(RWMutex) 보호 하에 가변 상태를 갱신한다(신규 노드 operation 추가 없음).

| 구분 | 필드 |
|------|------|
| 런타임 가변 | 레지스터 그룹(추가/수정/삭제/enable-disable), 그룹별 `poll_interval`, 디바이스 `unit_id`/timeout/reconnect, 데이터 타입/바이트순서 오버레이 |
| init 전용 (거부) | `transport`(tcp↔rtu 전환), RTU 시리얼 하드웨어 파라미터(port/baud/data_bits/stop_bits/parity) |

> `unit_id` 는 `ModbusDevice` 의 atomic `unitID` SSOT 로 런타임 변이한다(`config.UnitID` = 생성 시드, 런타임 판독 `dev.UnitID()`). `parseUnitIDParam` 은 init `toByte` 보다 엄격하여 범위 초과/비정수를 truncate 하지 않고 거부한다. init 전용 필드 변경 요청 시 오류를 반환하고 에이전트는 직전 설정으로 계속 동작한다(부분 적용 없음).

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
| `protocol.go` | MODBUS 프로토콜 프레임 인코딩/디코딩(ADU-중립 PDU 빌더), 기능 코드 상수 |
| `transport.go` | `ModbusTransport` 인터페이스(`SendAndReceive(ctx, unitID, pdu)→respPDU`) + TCP 트랜스포트(MBAP/txID) |
| `transport_rtu.go` | RTU 시리얼 트랜스포트(반이중, 시리얼 버스당 단일 공유 인스턴스 + turnaround mutex) |
| `rtu_crc.go` | RTU CRC-16 계산 (poly 0xA001, init 0xFFFF) |
| `rtu_adu.go` | RTU ADU 프레이밍/디프레이밍 (`[unitID][PDU][CRC-lo][CRC-hi]`) |
| `set_config.go` | 런타임 재구성 명령 `set_config` 핸들러 (런타임 가변 vs init 전용 필드 분기) |
| `write.go` | 쓰기 명령 처리 (FC05/FC06/FC15/FC16), 파라미터 추출 헬퍼, FC06->FC16 자동 전환 |
| `errors.go` | sentinel 에러 정의 |
| `register.go` | `RegisterModbusTypes()` 에이전트 타입 등록 (type id `modbus-client`) |
| `agent_test.go` | 에이전트 통합 테스트 |
| `cache_test.go` | RegisterCache, TypeOverlay 테스트 |
| `config_test.go` | 설정 파싱 및 검증 테스트 |
| `protocol_test.go` | 프로토콜 인코딩/디코딩 테스트 |
| `write_test.go` | 쓰기 명령 처리 테스트 |

## 테스트

```bash
go test -v -race -cover ./internal/agent/modbus/...
```

- 커버리지: `internal/agent/modbus` 85.6% / `internal/modbus` 99.4%
- Race Detector: 이상 없음
- 참고: `defaultRTUSerialOpener`(실제 시리얼 오픈)는 하드웨어 전용 경로로 설계상 커버리지 제외

## 의존성

| 패키지 | 용도 |
|--------|------|
| `internal/modbus` | 데이터 타입 변환, TypeMapEntry, TypeOverlayEntry |
| `internal/agent` | Agent, MessageReceiver, StatefulAgent 인터페이스 |
| `pkg/lifecycle` | BaseLifecycle 생명주기 관리 |
| `log/slog` | 구조화 로깅 |
| `sync` | RWMutex 기반 동시성 제어 |
| `net` | TCP 연결 관리 |
| `go.bug.st/serial` | RTU 시리얼 포트 오픈/입출력 (century 재사용, 신규 의존성 아님) |
| `encoding/json` | JSON 요청/응답 처리 |
| `encoding/binary` | MODBUS 프레임 바이너리 인코딩 |

## SPEC 문서

- SPEC ID: SPEC-MODBUS-001 (클라이언트 에이전트 기본 구현)
- SPEC ID: SPEC-MODBUS-003 (TypeOverlay 패턴 추가)
- SPEC ID: SPEC-MODBUS-006 (RTU 지원 + 트랜스포트 선택 + 그룹별 폴링 + raw/4순열 바이트순서 + 노드 런타임 `set_config`)
- 상태: 구현 완료

## 디바이스 모델 카탈로그 (SPEC-MODBUS-013)

기종별 레지스터 맵을 JSON 으로 두고 디바이스 편집 화면의 모델 선택기로 불러온다.

- 위치: `~/.xflow/models/*.json` (환경변수 `XFLOW_MODELS_DIR` 로 override)
- 조회: `POST /agents/{id}/query` `{"command":"list_models"}` (읽기 전용)
- 동봉 샘플: `assets/models/gipam-115fi.json` — 설치 시 모델 디렉터리로 복사한다.

```bash
mkdir -p ~/.xflow/models && cp assets/models/*.json ~/.xflow/models/
```

로딩은 파일 단위 fail-open 이다. 깨진 파일 하나가 카탈로그 전체를 막지 않으며,
무효 파일은 경고 로그를 남기고 건너뛴다. `id` 가 중복되면 파일명 사전순으로 먼저 온 파일이 이긴다.

## 레지스터 사용 여부와 블록 병합 (SPEC-MODBUS-013)

- `register_groups[].enabled` (선택, 기본 `true`): `false` 면 폴링·캐시 갱신·메시지 방출을
  모두 생략한다. 그룹 정의는 보존되므로 재활성화에 재입력이 필요 없다.
- `max_block_registers` (에이전트 레벨, 기본 32) / `devices[].max_block_registers` (오버라이드):
  같은 디바이스·같은 폴링 주기·같은 function code·주소가 연속인 그룹들을 이 상한 이내에서
  하나의 물리 읽기로 병합한다. 주소 간극이 있으면 병합하지 않는다 — 미정의 주소를 읽으면
  슬레이브가 `ILLEGAL DATA ADDRESS(02)` 로 응답하기 때문이다.

병합은 트랜스포트 계층에만 적용된다. 캐시 갱신·변경 감지·메시지 방출은 여전히 그룹 단위로
수행되므로 노드·대시보드·플로우가 보는 메시지 형상은 병합 이전과 동일하다.
