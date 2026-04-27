---
id: SPEC-MODBUS-001
type: plan
version: "1.0.0"
created: "2026-02-26"
updated: "2026-02-26"
author: xtra
---

# SPEC-MODBUS-001 구현 계획: MODBUS/TCP Client Agent

## 1. 작업 분해

### Primary Goal: 기반 구조 + 프로토콜 (M1)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | 센티널 에러 정의 (12개) | `internal/agent/modbus/errors.go` | High |
| 2 | MODBUS 프로토콜 상수 및 Function Code 정의 | `internal/agent/modbus/protocol.go` | High |
| 3 | MBAP 프레임 빌더/파서, FC01~FC04 요청/응답 인코딩/디코딩 | `internal/agent/modbus/protocol.go` | High |
| 4 | ModbusConfig, DeviceConfig, RegisterGroupConfig 파싱 및 검증 | `internal/agent/modbus/config.go` | High |
| 5 | RegisterModbusTypes 팩토리 등록 함수 | `internal/agent/modbus/register.go` | High |
| 6 | protocol_test.go: MBAP 프레임 빌드/파서, FC 인코딩/디코딩 테스트 | `internal/agent/modbus/protocol_test.go` | High |
| 7 | config_test.go: 설정 파싱, 기본값, 유효성 검증 테스트 | `internal/agent/modbus/config_test.go` | High |

### Secondary Goal: 전송 + 읽기 (M2)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 8 | ModbusTCPTransport 구현 (TCP 연결, MBAP 프레임 송수신, 자동 재연결) | `internal/agent/modbus/transport.go` | High |
| 9 | ModbusDevice 디바이스별 상태 및 연결 관리 | `internal/agent/modbus/device.go` | High |
| 10 | ModbusAgent 스캐폴드 (Init, Start, Stop 기본 구현) | `internal/agent/modbus/agent.go` | High |
| 11 | agent_test.go: 생명주기 테스트 (Init/Start/Stop) | `internal/agent/modbus/agent_test.go` | High |

### Tertiary Goal: 레지스터 캐시 (M3)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 12 | RegisterCache CRUD, staleness 감지, CompareAndUpdate | `internal/agent/modbus/cache.go` | High |
| 13 | StatefulAgent.State() 구현 | `internal/agent/modbus/state.go` | High |
| 14 | 폴링 루프에 캐시 통합 (읽기 성공 시 캐시 갱신) | `internal/agent/modbus/agent.go` | High |
| 15 | cache_test.go: 캐시 CRUD, staleness, 동시성 테스트 | `internal/agent/modbus/cache_test.go` | High |
| 16 | state_test.go: State() 반환값 검증 테스트 | `internal/agent/modbus/state_test.go` | High |

### Quaternary Goal: 읽기 모드 + Interval/Event (M4)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 17 | read_mode 분기 (direct vs cached) 구현 | `internal/agent/modbus/agent.go` | High |
| 18 | Interval Mode: 매 폴링 주기마다 전체 데이터 전송 | `internal/agent/modbus/agent.go` | High |
| 19 | Event Mode: RegisterCache 기반 변경 감지 + heartbeat | `internal/agent/modbus/agent.go` | High |
| 20 | Direct Mode: Process() `read_registers` 명령 처리 | `internal/agent/modbus/agent.go` | High |
| 21 | `force: true` 파라미터 지원 (캐시 바이패스) | `internal/agent/modbus/agent.go` | High |

### Quinary Goal: 레지스터 쓰기 (M5)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 22 | FC05/FC06/FC15/FC16 쓰기 구현 | `internal/agent/modbus/write.go` | High |
| 23 | protocol.go에 쓰기 요청/응답 인코딩 추가 | `internal/agent/modbus/protocol.go` | High |
| 24 | Process() 쓰기 명령 디스패치 | `internal/agent/modbus/agent.go` | High |
| 25 | Write-Through 캐시 갱신 (성공 후에만) | `internal/agent/modbus/write.go` | High |
| 26 | write_test.go: 쓰기 명령, 읽기 전용 거부, 범위 초과 테스트 | `internal/agent/modbus/write_test.go` | High |

### Senary Goal: 브릿지 통합 (M6)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 27 | MessageReceiver 인터페이스 구현 (ReceiveMessage/SourceCh) | `internal/agent/modbus/agent.go` | High |
| 28 | BridgeInOut 방향 지원 | `internal/agent/modbus/agent.go` | High |
| 29 | Process() 명령 라우팅 (read/write/cache 명령) | `internal/agent/modbus/agent.go` | High |
| 30 | 메시지 포맷 (Metadata: device_id, address, timestamp, mode) | `internal/agent/modbus/agent.go` | High |

### Septenary Goal: 오류 처리 + 마무리 (M7)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 31 | Health 관리 (Degraded/Unhealthy 전환 로직) | `internal/agent/modbus/agent.go` | High |
| 32 | 구조화 로깅 (slog) 전체 적용 | `internal/agent/modbus/agent.go` | High |
| 33 | 재연결 로직 (max_reconnect_attempts 준수) | `internal/agent/modbus/transport.go` | High |
| 34 | Stale 경고 이벤트 (register_group_stale 메시지) | `internal/agent/modbus/cache.go` | High |
| 35 | 쓰기 이벤트 알림 (enable_write_events 시 성공/실패 이벤트) | `internal/agent/modbus/write.go` | Medium |

### Final Goal: 통합 테스트 + 예제 (M8)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 36 | Mock MODBUS 서버 기반 End-to-End 테스트 | `internal/agent/modbus/agent_test.go` | High |
| 37 | 예제 에이전트 YAML | `examples/agents/modbus-plc.yaml` | Medium |
| 38 | 예제 플로우 YAML | `examples/flows/modbus-monitoring.yaml` | Medium |
| 39 | main.go에 RegisterModbusTypes 호출 추가 | `cmd/xflowd/main.go` | High |

---

## 2. 기술 접근 방식

### 2.1 아키텍처 개요

```
                    +----------------------------------------------+
                    |              ModbusAgent                      |
                    |  (Agent + MessageReceiver + StatefulAgent)    |
                    +------+--------+--------+---------------------+
                           |        |        |
                +----------+  +-----+-----+  +-----------+
                |             |           |               |
        ModbusTCPTransport    |     RegisterCache    ModbusConfig
        (TCP conn, MBAP       |     (per-device      (Transport
         send/receive,        |      read cache,       .Options
         auto-reconnect)      |      staleness)        parsing)
                              |
                       ModbusDevice
                       (per-device state,
                        connection, register groups)
                              |
                        protocol.go
                        (MBAP frame builder/parser,
                         FC request/response codec)
                              |
                    msgCh ---------> Bridge Node (BridgeInOut)
```

### 2.2 설정 파싱 전략 (config.go)

Samsung NASA Agent와 동일한 패턴을 따른다:

- `AgentConfig.Transport.Options` (`map[string]any`)에서 MODBUS 전용 설정을 파싱
- `parseModbusConfig(opts map[string]any) (*ModbusConfig, error)` 함수 구현
- 필수 필드 검증: `devices` 배열 (비어있지 않아야 함)
- 디바이스별 필수 필드: `device_id`, `host`, `registers`
- 기본값 적용: `poll_interval="5s"`, `mode="interval"`, `read_mode="cached"`, `port=502`, `unit_id=1`
- `time.Duration` 문자열 파싱: `time.ParseDuration()` 사용
- `stale_threshold` 미설정 시 `poll_interval * 3` 자동 계산

### 2.3 프로토콜 구현 (protocol.go)

MODBUS/TCP MBAP 프레임을 직접 구현한다 (Samsung NASA 패턴과 일관):

**MBAP Header 구조 (7 bytes):**

| 오프셋 | 크기 | 필드 | 설명 |
|--------|------|------|------|
| 0 | 2 bytes | Transaction ID | 요청-응답 매칭 (Big-Endian, 순차 증가) |
| 2 | 2 bytes | Protocol ID | 항상 0x0000 |
| 4 | 2 bytes | Length | Unit ID + PDU 바이트 수 |
| 6 | 1 byte | Unit ID | 디바이스 식별자 |

**PDU 인코딩 (Function Code별):**

- FC01/FC02 요청: FC(1) + StartAddr(2) + Quantity(2)
- FC03/FC04 요청: FC(1) + StartAddr(2) + Quantity(2)
- FC05 요청: FC(1) + Address(2) + Value(2) (0xFF00 또는 0x0000)
- FC06 요청: FC(1) + Address(2) + Value(2)
- FC15 요청: FC(1) + StartAddr(2) + Quantity(2) + ByteCount(1) + Data(N)
- FC16 요청: FC(1) + StartAddr(2) + Quantity(2) + ByteCount(1) + Data(N)

**인코딩 라이브러리**: `encoding/binary` (BigEndian) -- MODBUS 표준 바이트 순서

**핵심 함수:**

- `BuildMBAPFrame(transactionID uint16, unitID byte, pdu []byte) []byte`
- `ParseMBAPFrame(data []byte) (transactionID uint16, unitID byte, pdu []byte, err error)`
- `BuildReadRequest(fc byte, startAddr, quantity uint16) []byte`
- `ParseReadResponse(fc byte, pdu []byte) (values interface{}, err error)`
- `BuildWriteSingleCoilRequest(addr uint16, value bool) []byte`
- `BuildWriteSingleRegisterRequest(addr, value uint16) []byte`
- `BuildWriteMultipleCoilsRequest(startAddr, quantity uint16, values []bool) []byte`
- `BuildWriteMultipleRegistersRequest(startAddr uint16, values []uint16) []byte`
- `ParseWriteResponse(fc byte, pdu []byte) error`
- `IsExceptionResponse(pdu []byte) (byte, byte, bool)` -- (FC, ExceptionCode, isException)

### 2.4 TCP 전송 계층 (transport.go)

Go 표준 `net` 패키지로 TCP 연결을 관리한다:

- `net.DialTimeout()` 으로 연결, `connect_timeout` 적용
- `conn.SetDeadline()` / `SetReadDeadline()` 으로 읽기 타임아웃 적용
- Transaction ID 기반 요청-응답 매칭 (MBAP Header의 Transaction ID 검증)
- 연결 풀링 불필요: MODBUS 디바이스는 단일 TCP 연결로 순차 통신
- 자동 재연결: 연결 실패/끊김 시 `reconnect_interval` 간격으로 재시도
- 최대 재연결 횟수 (`max_reconnect_attempts`) 초과 시 중단

**인터페이스 설계 (테스트 가능성):**

```go
type ModbusTransport interface {
    Connect(ctx context.Context) error
    Send(ctx context.Context, frame []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
    IsConnected() bool
}
```

Mock 주입으로 TCP 연결 없이 단위 테스트 가능.

### 2.5 디바이스 관리 (device.go)

디바이스별 독립적인 상태와 연결을 관리한다:

- `ModbusDevice` 구조체: 디바이스 설정, TCP 연결, 연결 상태, 에러 카운터
- 한 디바이스의 통신 오류가 다른 디바이스에 전파되지 않음
- 디바이스별 Health 추적: 연속 에러 횟수 기반 Degraded/Unhealthy 판정
- 동일 host:port의 서로 다른 Unit ID는 하나의 TCP 연결을 공유 가능 (리스크 완화)

### 2.6 캐시 관리 (cache.go)

`RegisterCache` 구조체로 디바이스별 레지스터 값을 관리한다:

- 4개 맵: `Coils`, `DiscreteInputs`, `HoldingRegisters`, `InputRegisters`
- `LastUpdateTime`: 레지스터 그룹별 마지막 갱신 시각 (키: `"FC{code}_{startAddr}"`)
- `sync.RWMutex`로 동시성 보호 (폴링 goroutine과 Process() 동시 접근)
- `CompareAndUpdate()`: Event Mode의 변경 감지 -- 이전/현재 값 비교 후 변경분 반환
- Staleness 감지: `LastUpdateTime`이 `stale_threshold`를 초과하면 `stale` 표시

**Write-Through 전략:**
- 쓰기 요청 -> 디바이스 응답 대기 -> 성공 시에만 캐시 갱신
- 실패 시 캐시는 원래 값 유지 (롤백 로직 불필요)

### 2.7 읽기 모드 전략 (agent.go)

두 가지 읽기 모드를 지원한다:

**Cached Mode (기본값):**
- 에이전트 Start 시 폴링 goroutine 시작
- `poll_interval` 주기로 모든 디바이스/레지스터 그룹 순회 읽기
- 읽은 값을 `RegisterCache`에 저장
- Process()의 `read_registers` 요청에 캐시 값 반환
- `force: true` 파라미터로 캐시 바이패스 가능

**Direct Mode:**
- 폴링 goroutine 없음
- Process()의 `read_registers` 요청마다 디바이스에 직접 쿼리
- 캐시 미사용, Interval/Event Mode 미적용

### 2.8 Interval/Event 모드 전략 (agent.go)

Cached Mode에서의 데이터 전달 방식을 결정한다:

**Interval Mode (기본값):**
- 매 폴링 주기마다 수집된 전체 레지스터 데이터를 JSON으로 변환
- `msgCh` 채널을 통해 Bridge에 즉시 전달
- 메시지 type: `"register_data"`

**Event Mode:**
- 폴링 주기마다 `RegisterCache.CompareAndUpdate()` 호출
- 변경된 레지스터만 JSON으로 변환하여 전달
- 메시지 type: `"register_changed"`
- heartbeat 타이머 (`heartbeat_interval`, 기본값 60s): 변경 여부 무관하게 전체 상태 전송
- heartbeat 메시지 type: `"register_heartbeat"`

### 2.9 Bridge 연동 (agent.go)

기존 Samsung NASA Agent와 동일한 패턴을 따른다:

- `MessageReceiver` 인터페이스 구현: `ReceiveMessage(ctx)` -> `msgCh` select 대기
- `BridgeInOut` 방향: 읽기 데이터 수신(In) + 쓰기/읽기 명령 전송(Out) 동시 지원
- `Process(data []byte)`: JSON 명령 파싱 -> 명령 유형별 디스패치

**Process() 명령 라우팅:**

| 명령 | Function Code | 처리 |
|------|---------------|------|
| `write_coil` | FC05 | write.go |
| `write_register` | FC06 | write.go |
| `write_coils` | FC15 | write.go |
| `write_registers` | FC16 | write.go |
| `read_registers` | FC01~FC04 | agent.go (direct/cached) |
| `get_cache` | - | cache.go |
| `get_all_caches` | - | cache.go |
| `refresh_cache` | FC01~FC04 | agent.go (강제 폴링) |

### 2.10 동시성 모델

Samsung NASA Agent 패턴을 따른다:

- **폴링 goroutine**: `Start()` 시 시작, `stopCh` 채널로 종료 신호
- **heartbeat goroutine** (Event Mode): 별도 ticker로 주기적 전체 상태 전송
- **RegisterCache**: `sync.RWMutex`로 보호
  - 읽기 (폴링, State(), get_cache): `RLock()/RUnlock()`
  - 쓰기 (폴링 갱신, Write-Through 갱신): `Lock()/Unlock()`
- **디바이스 맵**: Init 이후 변경 없음 (불변) -- 별도 잠금 불필요
- **msgCh**: 버퍼링된 채널 (기본 256) -- 폴링 goroutine이 블로킹되지 않도록

---

## 3. 의존성 분석

### 3.1 내부 의존성

| 패키지 | 용도 | 변경 필요 |
|--------|------|-----------|
| `internal/agent/agent.go` | Agent, MessageReceiver, StatefulAgent 인터페이스 | 변경 없음 |
| `internal/agent/type_registry.go` | TypeRegistry, AgentFactory | 변경 없음 |
| `pkg/lifecycle/` | BaseLifecycle 상태 머신 | 변경 없음 |
| `internal/node/bridge.go` | BridgeNode, BridgeDirection (InOut) | 변경 없음 |
| `cmd/xflowd/main.go` | RegisterModbusTypes 호출 추가 | 1줄 추가 |

### 3.2 외부 의존성

| 패키지 | 용도 |
|--------|------|
| Go 표준 `net` | TCP 소켓 연결 |
| Go 표준 `encoding/binary` | Big-Endian 바이트 인코딩 |
| Go 표준 `log/slog` | 구조화 로깅 |
| Go 표준 `sync` | RWMutex 동시성 보호 |
| Go 표준 `encoding/json` | JSON 메시지 직렬화/역직렬화 |
| `github.com/stretchr/testify` | 테스트 어설션 (이미 사용 중) |

**CGO 의존성 없음** -- tech.md pure Go 정책 준수.

### 3.3 외부 시스템 의존성

| 시스템 | 조건 | 비고 |
|--------|------|------|
| MODBUS/TCP 디바이스 (PLC 등) | 실제 운영 시 | TCP 포트 502 |
| TCP 네트워크 | 항상 | 방화벽 설정 사전 완료 가정 |

---

## 4. 리스크 분석

### 리스크 1: MBAP 프로토콜 구현 버그

- **영향**: High
- **가능성**: Medium
- **설명**: MBAP Header 길이 계산, Transaction ID 매칭, Big-Endian 바이트 순서 등에서 구현 오류 발생 가능
- **대응**: protocol_test.go에서 프로토콜 레벨 단위 테스트를 최우선 작성. MODBUS 표준 문서의 예제 바이트 시퀀스로 검증. Table-driven 테스트로 모든 Function Code 커버

### 리스크 2: 다중 디바이스 TCP 리소스 관리

- **영향**: Medium
- **가능성**: Medium
- **설명**: 다수의 MODBUS 디바이스 연결 시 TCP 소켓 리소스 부족 또는 연결 관리 복잡도 증가
- **대응**: 동일 host:port의 서로 다른 Unit ID는 TCP 연결 공유. 연결 풀링은 단일 연결이므로 리소스 부담 최소. 연결 상태 모니터링으로 좀비 연결 방지

### 리스크 3: 폴링 드리프트

- **영향**: Low
- **가능성**: Low
- **설명**: `time.Ticker`를 사용한 주기적 폴링에서 디바이스 수가 많으면 한 주기의 처리 시간이 `poll_interval`을 초과할 수 있음
- **대응**: 산업용 환경에서 허용 가능한 수준. 향후 디바이스별 독립 폴링 goroutine으로 확장 가능 (REQ-MODBUS-001-04-04 Optional)

### 리스크 4: Event Mode 변경 감지 오탐

- **영향**: Medium
- **가능성**: Medium
- **설명**: 센서 노이즈에 의한 미세한 값 변동이 불필요한 이벤트를 발생시킬 수 있음
- **대응**: 현재 범위에서는 정확한 값 비교 (`==`)로 구현. 향후 deadband (임계값 필터링) 옵션을 Optional로 추가 가능

### 리스크 5: 벤더별 비표준 MODBUS 구현

- **영향**: Medium
- **가능성**: High
- **설명**: 일부 PLC 벤더가 MODBUS 표준을 완벽히 준수하지 않을 수 있음 (비표준 Exception Code, 비표준 응답 길이 등)
- **대응**: 표준 준수를 우선 구현. 벤더별 워크어라운드는 Optional 범위로 분리. 프로토콜 인터페이스 설계로 커스텀 파서 주입 가능

### 리스크 6: RegisterCache 경쟁 조건

- **영향**: High
- **가능성**: Medium
- **설명**: 폴링 goroutine과 Process() 호출이 동시에 RegisterCache에 접근할 때 데이터 레이스 발생 가능
- **대응**: `sync.RWMutex`로 모든 캐시 접근 보호. `go test -race` 플래그로 경쟁 조건 검출. 테스트에서 동시성 시나리오 포함

---

## 5. 기술 스택

| 구성 요소 | 선택 | 근거 |
|-----------|------|------|
| MODBUS 프로토콜 | 자체 구현 (protocol.go) | Samsung NASA 패턴 일관성. 외부 라이브러리 의존 최소화 |
| TCP 통신 | Go 표준 `net` 패키지 | CGO 불필요, tech.md 정책 준수 |
| 프레임 인코딩 | `encoding/binary` (BigEndian) | MODBUS 표준 바이트 순서 |
| 동시성 | goroutine + channel 패턴 | Samsung NASA Agent 패턴 일관성 |
| 캐시 동시성 보호 | `sync.RWMutex` | 읽기 빈도 >> 쓰기 빈도인 MODBUS 특성에 최적 |
| 로깅 | `log/slog` | 프로젝트 표준 |
| JSON 직렬화 | `encoding/json` | Go 표준, Bridge 메시지 포맷 |
| 테스팅 | `testing` + `testify` | Go 관례 + 프로젝트 기존 의존성 |

**대안 검토: goburrow/modbus 라이브러리**

| 기준 | 자체 구현 | goburrow/modbus |
|------|-----------|-----------------|
| 코드 일관성 | Samsung NASA 패턴과 일치 | 다른 추상화 레벨 |
| 의존성 | 추가 의존성 없음 | 외부 의존성 추가 |
| 테스트 가능성 | 인터페이스 기반 완전 제어 | 라이브러리 내부 모킹 제한 |
| 유지보수 | 직접 관리 필요 | 커뮤니티 유지보수 |
| 구현 복잡도 | 중간 (MBAP + FC 직접 구현) | 낮음 (API 호출) |

**결론**: Samsung NASA Agent와의 아키텍처 일관성 및 테스트 가능성을 우선하여 자체 구현을 선택. 향후 복잡도가 과도하게 증가하면 goburrow/modbus로 마이그레이션 가능.

---

## 6. 개발 방법론

**Hybrid 모드** (quality.yaml `development_mode: "hybrid"` 설정):

- **신규 파일 (TDD)**: 모든 modbus/ 패키지 파일은 신규 생성이므로 TDD 적용
  - RED: 테스트 먼저 작성 (실패 확인)
  - GREEN: 최소 구현으로 테스트 통과
  - REFACTOR: 코드 품질 개선
- **기존 파일 (DDD)**: `cmd/xflowd/main.go`에 1줄 추가는 DDD 적용
  - ANALYZE: 기존 등록 패턴 파악
  - PRESERVE: 기존 동작 보존 확인
  - IMPROVE: RegisterModbusTypes 호출 추가

**커버리지 목표**: 85%+ (`go test -cover ./internal/agent/modbus/...`)

**경쟁 조건 검출**: `go test -race ./internal/agent/modbus/...`

---

## 7. 주요 구조체 설계

### ModbusAgent

```go
type ModbusAgent struct {
    *lifecycle.BaseLifecycle
    config       *ModbusConfig
    devices      map[string]*ModbusDevice     // device_id -> device
    deviceCaches map[string]*RegisterCache    // device_id -> cache
    pollTicker   *time.Ticker                 // cached mode only
    heartbeatTicker *time.Ticker              // event mode only
    msgCh        chan []byte                   // MessageReceiver buffer
    stopCh       chan struct{}
    stats        *agent.AgentStats
    logger       *slog.Logger
}
```

### ModbusConfig

```go
type ModbusConfig struct {
    Mode              string           // "interval" | "event"
    ReadMode          string           // "direct" | "cached"
    PollInterval      time.Duration    // default: 5s
    HeartbeatInterval time.Duration    // default: 60s
    StaleThreshold    time.Duration    // default: PollInterval * 3
    WriteTimeout      time.Duration    // default: 5s
    EnableWriteEvents bool             // default: false
    MsgChannelSize    int              // default: 256
    Devices           []DeviceConfig
}
```

### DeviceConfig

```go
type DeviceConfig struct {
    DeviceID             string
    Host                 string
    Port                 int               // default: 502
    UnitID               byte              // default: 1
    PollInterval         time.Duration     // optional, overrides global
    ConnectTimeout       time.Duration     // default: 5s
    ReadTimeout          time.Duration     // default: 3s
    ReconnectInterval    time.Duration     // default: 10s
    MaxReconnectAttempts int               // default: 10
    Registers            []RegisterGroupConfig
}
```

### RegisterGroupConfig

```go
type RegisterGroupConfig struct {
    Name         string   // optional, for logging
    FunctionCode byte     // 1, 2, 3, or 4
    StartAddress uint16
    Quantity     uint16
}
```

### RegisterCache

```go
type RegisterCache struct {
    Coils            map[uint16]bool
    DiscreteInputs   map[uint16]bool
    HoldingRegisters map[uint16]uint16
    InputRegisters   map[uint16]uint16
    LastUpdateTime   map[string]time.Time  // key: "FC{code}_{startAddr}"
    mu               sync.RWMutex
}
```

---

## 8. 마일스톤 요약

| 마일스톤 | 산출물 | 우선도 | 요구사항 매핑 |
|----------|--------|--------|--------------|
| M1: 기반 구조 + 프로토콜 | errors.go, protocol.go, config.go, register.go, protocol_test.go, config_test.go | High | REQ-001-01-01, 001-02-*, 001-11-*, 001-12-04 |
| M2: 전송 + 읽기 | transport.go, device.go, agent.go (scaffold), agent_test.go | High | REQ-001-01-02~05, 001-02-02~04 |
| M3: 레지스터 캐시 | cache.go, state.go, cache_test.go, state_test.go | High | REQ-001-10-*, 001-08-05 |
| M4: 읽기 모드 + 모드 | agent.go (모드 분기), agent_test.go | High | REQ-001-03-*, 001-05-*, 001-06-* |
| M5: 레지스터 쓰기 | write.go, write_test.go | High | REQ-001-09-* |
| M6: 브릿지 통합 | agent.go (MessageReceiver, Process) | High | REQ-001-07-* |
| M7: 오류 처리 | agent.go, transport.go, cache.go (Health, 재연결, Stale) | High | REQ-001-12-*, 001-01-03~05 |
| M8: 통합 + 예제 | agent_test.go (E2E), examples/, main.go | High/Medium | 전체 통합 검증 |

---

## 9. 검증 명령

```bash
# 단위 테스트 (경쟁 조건 검출 포함)
go test -race -cover ./internal/agent/modbus/...

# 빌드 검증
go vet ./...
go build ./cmd/xflowd ./cmd/xflow

# 커버리지 리포트
go test -coverprofile=coverage_modbus.out ./internal/agent/modbus/...
go tool cover -html=coverage_modbus.out

# 수동 검증 (Cached Mode)
xflow agent import -f examples/agents/modbus-plc.yaml
xflow flow import -f examples/flows/modbus-monitoring.yaml
xflow flow deploy modbus-monitoring
xflow flow start modbus-monitoring
```

---

*SPEC-MODBUS-001 Plan v1.0.0*
*작성자: xtra*
*날짜: 2026-02-26*
