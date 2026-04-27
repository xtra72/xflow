# SPEC-LGCP-001: Implementation Plan (v2.0.0 - Clean Transport Abstraction)

**SPEC Reference**: SPEC-LGCP-001
**Version**: 2.0.0
**Status**: Implemented
**Created**: 2026-04-06

## 1. Technical Approach

### 1.1 Core Strategy

Hybrid 방법론 적용: 기존 코드(DDD: ANALYZE-PRESERVE-IMPROVE), 신규 기능(TDD: RED-GREEN-REFACTOR).

핵심 원칙:
1. **기존 `LGAPTransport` 인터페이스를 변경하지 않는다.** 새로운 TCP 전송은 이 인터페이스의 추가 구현체이다.
2. **LGCP 에이전트 내부 구조를 변경하지 않는다.** `captureLoop()`, `transportReader`, `LGCPFrameParser`, `Process()` 는 그대로 유지.
3. **기존 `lgapSerialTransport` 를 수정하지 않는다.** 검증된 코드는 건드리지 않는다.
4. **`NewLGCPAgent()` 에 Transport Factory만 추가한다.** 설정에 따라 올바른 transport 구현체를 생성.

### 1.2 Key Technical Decisions

**Transport 구현 패턴**: `lgapSerialTransport` 가 `io.ReadWriteCloser` + `sync.Mutex` + `atomic.Bool` 패턴을 사용하므로, TCP 전송도 동일한 패턴을 따른다. 이를 통해 `captureLoop()` 과 `reconnectLoop()` 이 transport 종류에 무관하게 동작한다.

**TCP Server 연결 관리**: LGCP 는 point-to-point 프로토콜이므로 TCP Server 모드에서 단일 활성 연결만 유지한다. 새 연결이 들어오면 기존 연결을 닫고 새 연결로 교체한다.

**재연결 로직 재사용**: 기존 `reconnectLoop()` 이 `transport.Available()` 확인 → `transport.Close()` → `transport.Open()` 순서로 재연결하므로, TCP transport 의 `Open()/Close()/Available()` 만 올바르게 구현하면 재연결이 자동으로 동작한다.

**Config 확장**: `LGCPConfig` 에 `TransportType`, `TCPHost`, `TCPPort`, `TCPReadTimeout`, `TCPWriteTimeout`, `TCPConnectTimeout` 필드를 추가하고, `parseLGCPConfig()` 에서 `transport_type` 에 따라 필수 필드를 조건부로 검증한다.

### 1.3 Architecture Decisions

| 결정 | 선택 | 근거 |
|------|------|------|
| Transport 추가 방식 | LGAPTransport 인터페이스 구현체 추가 | 기존 인터페이스 재사용, 에이전트 내부 변경 불필요 |
| TCP 연결 관리 | net.Conn + sync.Mutex + atomic.Bool | lgapSerialTransport 와 동일한 패턴, 검증됨 |
| TCP Server 다중 연결 | 최신 연결 교체 (단일 활성) | LGCP point-to-point 프로토콜 특성 |
| 재연결 로직 | 기존 reconnectLoop 재사용 | transport.Open()/Available() 만 구현하면 자동 동작 |
| 파일 구조 | transport_tcp.go 신규 생성 | 기존 transport.go 는 수정하지 않음 |
| Config 확장 | 기존 LGCPConfig 에 필드 추가 | 별도 struct 불필요, 단순성 유지 |

---

## 2. Milestones

### M1: Existing Behavior Preservation (Primary Goal)

**목표**: 기존 시리얼 모드 동작의 완전한 보존을 보장하는 characterization 테스트 작성

**산출물**:
- `lgcp_agent_test.go`: 기존 serial 모드 동작에 대한 characterization 테스트 보강
  - Process() 명령 처리 (get_stats, get_recent, set_power 등) 테스트
  - captureLoop 동작 확인 테스트
  - Lifecycle (Start/Stop/Pause/Resume) 동작 테스트
  - ReceiveMessage() 를 통한 msgCh 이벤트 전달 테스트
  - reconnectLoop 동작 확인 테스트
- `lgcp_config_test.go`: 기존 설정 파싱에 대한 characterization 테스트 보강

**의존성**: 없음 (기존 코드 기반)

**방법론**: DDD (ANALYZE-PRESERVE)

### M2: Config Extension (Primary Goal)

**목표**: `transport_type`, `tcp_host`, `tcp_port` 등 TCP 설정 필드 추가 및 조건부 serial_port 검증

**산출물**:
- `lgcp_config.go` 수정:
  - `TransportType string` 필드 추가 (기본값: `"serial"`)
  - `TCPHost string`, `TCPPort int` 필드 추가
  - `TCPReadTimeout`, `TCPWriteTimeout`, `TCPConnectTimeout` 필드 추가
  - `parseLGCPConfig()` 에서 `transport_type` 파싱 및 필드 조건부 검증
  - `serial_port` 는 `transport_type="serial"` 일 때만 필수
  - `tcp_port` 는 `transport_type="tcp-client"` 또는 `"tcp-server"` 일 때 필수
- `lgcp_config_test.go` 수정:
  - `transport_type` 파싱 테스트 (serial, tcp-client, tcp-server)
  - 기본값 적용 테스트 (미설정 시 serial)
  - `serial_port` 조건부 필수 테스트
  - `tcp_host`/`tcp_port` 필수 필드 검증 테스트
  - 알 수 없는 `transport_type` 에러 테스트

**의존성**: M1 (characterization 테스트로 기존 동작 보장)

**방법론**: TDD (신규 필드) + DDD (기존 parseLGCPConfig 수정)

### M3: TCP Client Transport (Primary Goal)

**목표**: `lgapTCPClientTransport` 구현 - 원격 TCP 서버에 접속하는 LGAPTransport 구현체

**산출물**:
- `transport_tcp.go` 신규:
  - `lgapTCPClientTransport` 구조체
  - `Open()`: `net.DialTimeout()` 으로 TCP 연결 수립
  - `Close()`: TCP 연결 종료
  - `Send(data)` / `Write(data)`: TCP 전송, write deadline 적용
  - `Receive(buf)`: TCP 수신, read deadline 적용
  - `Available()`: atomic.Bool 기반 연결 상태 추적
  - 에러 발생 시 자동 `Available()` = false 전환
- `transport_tcp_test.go` 신규:
  - Open/Close 테스트 (net.Pipe 또는 로컬 TCP 서버 활용)
  - Send/Receive 데이터 전송 테스트
  - Available() 상태 추적 테스트
  - 연결 끊김 후 Available() = false 테스트
  - 타임아웃 테스트 (read/write deadline)
  - 동시 접근 안전성 테스트 (go test -race)

**의존성**: M2 (Config에 TCP 필드 존재)

**방법론**: TDD (RED-GREEN-REFACTOR, 완전 신규 코드)

### M4: TCP Server Transport (Primary Goal)

**목표**: `lgapTCPServerTransport` 구현 - TCP 접속을 수락하는 LGAPTransport 구현체

**산출물**:
- `transport_tcp.go` 추가:
  - `lgapTCPServerTransport` 구조체
  - `Open()`: `net.Listen()` + Accept 고루틴 시작
  - `Close()`: 리스너 + 활성 연결 종료
  - `Send(data)` / `Write(data)`: 활성 연결로 전송
  - `Receive(buf)`: 활성 연결에서 수신
  - `Available()`: 활성 연결 존재 여부
  - 새 연결 수락 시 기존 연결 교체 (최신 우선)
- `transport_tcp_test.go` 추가:
  - Listen/Accept 테스트
  - 연결 교체 테스트 (새 연결이 기존 연결을 교체)
  - Send/Receive 데이터 전송 테스트
  - Close 시 리스너 + 연결 모두 종료 테스트
  - 연결 없이 Send 시 에러 테스트
  - 동시 접근 안전성 테스트 (go test -race)

**의존성**: M3 (TCP Client와 공통 패턴 공유)

**방법론**: TDD (RED-GREEN-REFACTOR, 완전 신규 코드)

### M5: Transport Factory and Agent Integration (Secondary Goal)

**목표**: NewLGCPAgent() 에 Transport Factory 추가, reconnectLoop 미세 조정

**산출물**:
- `lgcp_agent.go` 수정:
  - `NewLGCPAgent()` 에 `transport_type` 에 따른 transport 생성 로직 추가
  - `"serial"`: 기존 `lgapSerialTransport` 생성 (기존 코드 유지)
  - `"tcp-client"`: `lgapTCPClientTransport` 생성
  - `"tcp-server"`: `lgapTCPServerTransport` 생성
  - 알 수 없는 값: 에러 반환
  - `reconnectLoop()` 에 TCP transport 호환 확인 (필요시 미세 조정)
- `lgcp_agent_test.go` 수정:
  - Transport Factory 테스트 (각 transport_type 에 대한 올바른 구현체 생성 검증)
  - TCP Client transport 로 에이전트 생성 테스트
  - TCP Server transport 로 에이전트 생성 테스트
  - 기존 Serial transport 생성 동작 보존 테스트

**의존성**: M3, M4 (TCP transport 구현체 필요)

**방법론**: Hybrid (TDD: Factory 로직, DDD: 기존 NewLGCPAgent 수정)

### M6: Integration Testing and Quality Verification (Secondary Goal)

**목표**: 전체 TCP 전송 모드 End-to-End 검증

**산출물**:
- `lgcp_agent_test.go` 추가:
  - TCP Client transport 로 captureLoop 동작 테스트:
    - TCP 서버에서 LGCP 프레임 전송 → captureLoop 파싱 → msgCh 출력
  - TCP Server transport 로 captureLoop 동작 테스트:
    - TCP 클라이언트에서 LGCP 프레임 전송 → captureLoop 파싱 → msgCh 출력
  - Process() 명령이 TCP transport 로 전송되는지 검증
  - reconnectLoop 이 TCP 재연결을 처리하는지 검증
  - 기존 Serial 테스트 전체 통과 확인
- `web/src/config/agentSchemas.ts` 수정:
  - `transport_type` select 필드 추가 (serial, tcp-client, tcp-server)
  - `tcp_host`, `tcp_port` 필드 추가
- `web/src/pages/agents/agentTypeMeta.ts` 수정:
  - Transport 유형별 필드 메타데이터 추가
- Race condition 검증: `go test -race ./internal/agent/lg/...`
- go vet: `go vet ./internal/agent/lg/...`

**의존성**: M5 (Transport Factory 통합 완료)

---

## 3. Risk Assessment

| 리스크 | 영향 | 대응 |
|--------|------|------|
| TCP 연결이 half-open 상태에 빠질 수 있음 | captureLoop 가 무한 대기 | Read deadline 설정으로 타임아웃 적용, Available() 갱신 |
| TCP Server Accept 루프에서 고루틴 누수 가능 | 메모리 누수 | Close() 시 listener.Close() 로 Accept 루프 종료 보장 |
| reconnectLoop 이 TCP transport 와 호환되지 않을 수 있음 | 재연결 실패 | M1 characterization 테스트로 reconnectLoop 동작 확인 후, TCP 호환성 검증 |
| TCP Server 에서 연결 교체 시 데이터 손실 | 프레임 유실 | Mutex 보호로 원자적 교체, 기존 연결 Close 전 drain |
| 기존 serial_port 필수 검증 로직 변경으로 하위 호환성 깨짐 | 기존 사용자 설정 오류 | transport_type 미설정 시 기본 "serial" 로 기존 검증 경로 유지 |

---

## 4. File Change Summary

| 파일 | 변경 유형 | 설명 |
|------|-----------|------|
| `internal/agent/lg/lgcp_config.go` | 수정 | TransportType, TCPHost, TCPPort, TCP 타임아웃 필드 추가, serial_port 조건부 필수 |
| `internal/agent/lg/lgcp_agent.go` | 수정 | NewLGCPAgent() 에 Transport Factory 추가, reconnectLoop 미세 조정 |
| `internal/agent/lg/transport_tcp.go` | 신규 | lgapTCPClientTransport, lgapTCPServerTransport 구현 |
| `internal/agent/lg/transport_tcp_test.go` | 신규 | TCP Transport 단위 테스트 |
| `internal/agent/lg/lgcp_config_test.go` | 수정 | TCP 설정 파싱 테스트 추가 |
| `internal/agent/lg/lgcp_agent_test.go` | 수정 | Transport Factory 및 TCP 통합 테스트 추가 |
| `web/src/config/agentSchemas.ts` | 수정 | transport_type, tcp_host, tcp_port 필드 추가 |
| `web/src/pages/agents/agentTypeMeta.ts` | 수정 | Transport 유형별 필드 메타데이터 추가 |

---

## 5. Development Methodology

| 마일스톤 | 방법론 | 근거 |
|---------|--------|------|
| M1: Existing Behavior Preservation | DDD (ANALYZE-PRESERVE) | 기존 코드 동작 보존 |
| M2: Config Extension | Hybrid (TDD + DDD) | 신규 필드 + 기존 parser 수정 |
| M3: TCP Client Transport | TDD (RED-GREEN-REFACTOR) | 완전히 신규 코드 |
| M4: TCP Server Transport | TDD (RED-GREEN-REFACTOR) | 완전히 신규 코드 |
| M5: Transport Factory | Hybrid (TDD + DDD) | 신규 Factory + 기존 NewLGCPAgent 수정 |
| M6: Integration Testing | DDD (PRESERVE-IMPROVE) | 전체 파이프라인 검증 |

---

## 6. Expert Consultation Recommendations

| 도메인 | 에이전트 | 상담 사유 |
|--------|---------|----------|
| Backend | expert-backend | TCP Transport 구현 패턴, reconnectLoop 호환성, TCP Server Accept 루프 설계 |
| Frontend | expert-frontend | agentSchemas.ts 에 transport_type 조건부 필드 표시 UX |
