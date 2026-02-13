---
id: SPEC-AGENT-001
type: plan
version: "1.1.0"
spec_ref: SPEC-AGENT-001
---

# SPEC-AGENT-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
  - `pkg/flow/` (SPEC-FLOW-001): Flow 구조 참조
- **외부 의존성**: `go.bug.st/serial` (Serial Transport)
- **동시성**: `sync.RWMutex`, `sync/atomic`

### 1.3 패키지 위치

- **경로**: `internal/agent/` (루트), `internal/agent/transport/`, `internal/agent/protocol/`
- **Tier**: Tier 2 - 내부 실행 계층
- **소비자**: `internal/node/`, `internal/engine/`, `internal/observe/`

---

## 2. 마일스톤

### Primary Goal: 핵심 인터페이스 + Error Types + Agent Info & Stats + Agent 프레임워크 (P0)

**범위**: Module 1, 2, 3, 6, 10, 11, 16, 17

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 14개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트

2. `agent.go` + `agent_test.go` 작성
   - `Agent` 인터페이스 정의 (13개 메서드: 기존 11개 + Info(), Stats())
   - `BaseAgent` 구현체 (상태 관리, Transport 래핑, Protocol 연동)
   - `BaseAgent` 내부 `AgentStats` atomic 카운터 관리
   - `Info()` 구현: 현재 상태, Transport 정보, Config, Stats 집계하여 `AgentInfo` 스냅샷 반환
   - `Stats()` 구현: 현재 `AgentStats` 스냅샷 반환
   - `pkg/lifecycle/` State 연동 테스트
   - 상태 전이 유효성 검증 테스트
   - Paused 상태 버퍼링 테스트
   - Resume 시 버퍼 데이터 우선 처리 테스트

3. `info.go` + `info_test.go` 작성
   - `AgentInfo` 구조체 정의 (ID, Name, Type, State, Health, Transport, Protocol, Config, Stats, SharedInfo, StartedAt, Uptime, CreatedAt)
   - `TransportInfo` 구조체 정의 (Type, Connected, RemoteAddr, LocalAddr, ConnectedAt)
   - `ProtocolInfo` 구조체 정의 (Name, FieldCount, HasChecksum, SourceFile)
   - `AgentStats` 구조체 정의 (MessagesReceived, MessagesSent, MessagesErrored, BytesRead, BytesWritten, LastActivityAt, AvgProcessingLatency, RestartCount)
   - `SharedInfo` 구조체 정의 (RefCount, Flows)
   - `ManagerSummary` 구조체 정의 (TotalAgents, RunningAgents, PausedAgents, StoppedAgents, ErrorAgents, HealthyAgents, UnhealthyAgents, TotalMessagesProcessed, TotalErrors, Agents)
   - `ResetStats()` 메서드 구현
   - atomic 카운터 업데이트 헬퍼 함수
   - 구조체 초기화 및 필드 정확성 테스트
   - Stats atomic 카운터 동시성 안전 테스트 (`go test -race`)
   - ResetStats() 초기화 테스트
   - 재시작 시 통계 보존 테스트

4. `manager.go` + `manager_test.go` 작성
   - `Manager` 인터페이스 정의 (기존 8개 메서드 + Summary())
   - `DefaultManager` 구현체 (`sync.RWMutex` 보호)
   - `Summary()` 구현: 모든 Agent의 Info()를 수집하여 상태별 카운트 집계, `ManagerSummary` 반환
   - Create, Start, Stop, Restart, Delete 테스트
   - 중복 ID 생성 거부 테스트
   - 존재하지 않는 Agent 조작 거부 테스트
   - Graceful Shutdown 테스트
   - Summary() 집계 정확성 테스트
   - 동시성 안전 테스트

5. `registry.go` + `registry_test.go` 작성
   - `Registry` 인터페이스 정의
   - `DefaultRegistry` 구현체 (`sync.RWMutex` 보호)
   - Register, Unregister, Get, GetByName, GetByType 테스트
   - 중복 등록 거부 테스트
   - 동시성 안전 테스트

6. `transport/transport.go` + `transport/transport_test.go` 작성
   - `Transport` 인터페이스 정의
   - `TransportConfig` 구조체 정의
   - `TransportFactory` 인터페이스 + `DefaultTransportFactory` 구현
   - 팩토리 패턴 테스트
   - 미등록 타입 거부 테스트

7. `protocol/definition.go` + `protocol/definition_test.go` 작성
   - `ProtocolDefinition` 구조체 정의
   - `FieldDefinition` 구조체 정의
   - `ConditionalPayload` 구조체 정의
   - `ByteOrder` 열거 정의

8. `protocol/field.go` + `protocol/field_test.go` 작성
   - `FieldType` 열거 정의 (8개 타입)
   - 필드 타입별 바이트 변환 함수 (encode/decode)
   - 테이블 드리븐 테스트 (모든 필드 타입)

9. `protocol/checksum.go` + `protocol/checksum_test.go` 작성
   - `ChecksumDefinition` 구조체 정의
   - `ChecksumAlgorithm` 열거 정의 (4개 알고리즘)
   - `Calculate()`, `Verify()` 함수
   - 알고리즘별 정확성 테스트 (알려진 테스트 벡터 사용)

10. `protocol/parser.go` + `protocol/parser_test.go` 작성
    - `Parser` 인터페이스 정의
    - `ParsedMessage` 구조체 정의
    - `DefaultParser` 구현체
    - 기본 파싱 테스트 (헤더 + 페이로드 + 트레일러)
    - 조건부 파싱 테스트
    - 체크섬 검증 실패 테스트
    - 불충분한 데이터 거부 테스트

**산출물**: Agent 프레임워크 핵심 동작 가능

---

### Secondary Goal: Health Check + Shared Agent + Transport 구현 + Loader (P1)

**범위**: Module 4, 5, 7, 8, 12, 13, 14, 15

**작업 항목**:

1. `health.go` + `health_test.go` 작성
   - `HealthStatus` 구조체 정의
   - `HealthState` 열거 정의
   - `HealthChecker` 인터페이스 + `DefaultHealthChecker` 구현
   - 주기적 헬스 체크 goroutine 테스트
   - Unhealthy 콜백 호출 테스트
   - 지수 백오프 재시작 전략 테스트
   - 최대 재시작 횟수 초과 테스트

2. `shared.go` + `shared_test.go` 작성
   - `SharedRef` 구조체 (atomic.Int32 기반 참조 카운트)
   - Acquire/Release 테스트
   - 참조 카운트 0 시 StopOnZeroRef 동작 테스트
   - 중복 플로우 참조 거부 테스트
   - 동시성 안전 테스트 (다중 goroutine Acquire/Release)

3. `transport/serial.go` + `transport/serial_test.go` 작성
   - `SerialTransport` 구현체 (go.bug.st/serial)
   - BaudRate, Parity, StopBits, DataBits 설정 테스트
   - 자동 재연결 테스트
   - (실제 시리얼 포트 없이 mock/interface 기반 테스트)

4. `transport/tcp.go` + `transport/tcp_test.go` 작성
   - `TCPTransport` 구현체 (net 패키지)
   - 클라이언트/서버 모드 테스트
   - 연결 풀링 테스트
   - 타임아웃 테스트
   - 자동 재연결 테스트

5. `protocol/loader.go` + `protocol/loader_test.go` 작성
   - `Loader` 인터페이스 + `DefaultLoader` 구현
   - YAML 파일 로드 테스트
   - JSON 파일 로드 테스트
   - 유효성 검증 테스트 (필수 필드 누락, 중복 필드명, 잘못된 타입)
   - 자동 포맷 감지 테스트

6. `config.go` + `config_test.go` 작성
   - `AgentConfig` 구조체 정의
   - `Validate()` 메서드 구현 및 테스트
   - 기본값 적용 테스트
   - 불변 설정 런타임 변경 거부 테스트

7. `system.go` + `system_test.go` 작성
   - `SystemAgent` 인터페이스 정의
   - `SystemAgentType` 열거 정의
   - IsSystem(), RequiresTransport() 반환값 테스트
   - (구체적인 System Agent 구현은 별도 SPEC)

8. `type_registry.go` + `type_registry_test.go` 작성
   - `TypeRegistry` 인터페이스 + `DefaultTypeRegistry` 구현
   - `AgentFactory` 함수 타입 정의
   - 타입 등록/생성 테스트
   - 기본 타입 자동 등록 테스트
   - 중복 타입 등록 거부 테스트
   - 동시성 안전 테스트

**산출물**: Agent 시스템 전체 기능 완성

---

### Tertiary Goal: UDP Transport (P2)

**범위**: Module 9

**작업 항목**:

1. `transport/udp.go` + `transport/udp_test.go` 작성
   - `UDPTransport` 구현체 (net 패키지)
   - 유니캐스트 통신 테스트
   - 수신 버퍼 크기 설정 테스트
   - 멀티캐스트 지원 (Optional)

**산출물**: UDP Transport 완성

---

### Optional Goal: 패키지 문서화

**범위**: doc.go

**작업 항목**:

1. `doc.go` 작성
   - 패키지 개요 문서
   - Agent 아키텍처 설명 (Transport + Protocol Definition 조합)
   - 주요 타입 사용 예시
   - System Agent vs Standard Agent vs Custom Agent 구분 설명

**산출물**: GoDoc 문서 완성

---

## 3. 기술적 접근 방식

### 3.1 인터페이스 설계 패턴

Agent 시스템의 핵심 타입은 인터페이스로 정의하여 확장성을 확보한다:

- `Agent` 인터페이스: 모든 Agent의 공통 계약
- `Transport` 인터페이스: 통신 레이어 추상화
- `Parser` 인터페이스: 프로토콜 파싱 추상화
- `Manager` / `Registry` / `HealthChecker` / `TypeRegistry`: 관리 계층 추상화

BaseAgent는 Agent 인터페이스의 기본 구현으로, 표준 Agent와 커스텀 Agent 모두 BaseAgent를 임베딩하여 공통 로직을 재사용한다.

### 3.2 동시성 모델

- **Manager**: `sync.RWMutex`로 내부 Agent 맵 보호 (Read: Get/List, Write: Create/Delete/Start/Stop)
- **Registry**: `sync.RWMutex`로 내부 레지스트리 보호 (Read: Get/List, Write: Register/Unregister)
- **SharedRef**: `atomic.Int32`로 참조 카운트 원자적 관리, `sync.Mutex`로 플로우 ID 목록 보호
- **BaseAgent**: `sync.RWMutex`로 내부 상태(State, Config) 보호
- **HealthChecker**: 별도 goroutine에서 주기적 헬스 체크 실행, context 기반 종료

### 3.3 Transport 구현 전략

Transport 인터페이스는 Go 표준 `io.Reader`/`io.Writer`와 유사한 `Read(buf []byte) (int, error)` / `Write(data []byte) (int, error)` 시그니처를 사용한다.

각 Transport 구현체는 내부에 연결 객체를 유지하며, `Available()` 메서드로 연결 상태를 노출한다. 자동 재연결은 Transport 내부에서 처리하되, 재연결 이벤트는 관찰성 시스템에 전파한다.

### 3.4 Protocol Definition Engine 설계

프로토콜 정의는 선언적 구조체로 메시지 포맷을 기술하고, Parser가 이 정의를 기반으로 바이트 데이터를 해석한다.

조건부 파싱의 핵심은 `ConditionalPayload` 구조체이다:

```go
type ConditionalPayload struct {
    FieldName  string            // 조건 판단 기준 헤더 필드명
    Conditions map[string][]FieldDefinition // 값 -> 페이로드 구조 매핑
}
```

헤더 파싱 후 지정된 필드의 값을 확인하여 어떤 페이로드 구조로 파싱할지 결정한다.

### 3.5 Health Check 지수 백오프 전략

```
재시작 간격 = min(initialDelay * pow(multiplier, attemptCount), maxDelay)

initialDelay = 1초
multiplier = 2
maxDelay = 5분

시도 1: 1초 대기 후 재시작
시도 2: 2초 대기 후 재시작
시도 3: 4초 대기 후 재시작
...
시도 N: min(2^(N-1)초, 5분) 대기 후 재시작
```

성공적으로 재시작되면 시도 횟수를 0으로 리셋한다.

### 3.6 맵 필드 방어적 복사

`AgentConfig.Metadata`, `ParsedMessage.Header/Payload/Trailer` 등 map 필드는 외부 변경으로부터 보호하기 위해 생성 시 복사본을 저장하고, 조회 시에도 복사본을 반환한다.

---

## 4. 리스크 및 대응

### R1: 의존 패키지 미완성

- **리스크**: pkg/lifecycle/, pkg/message/, pkg/xferr/, pkg/flow/ 패키지가 아직 구현되지 않아 컴파일 불가
- **대응**: 테스트에서 mock 인터페이스 사용, 의존 패키지 완성 후 통합 테스트 실행
- **심각도**: 중간 (인터페이스만 의존하므로 mock으로 대체 가능)

### R2: Serial Transport 테스트

- **리스크**: 실제 시리얼 포트 없이는 Serial Transport 통합 테스트 불가
- **대응**: go.bug.st/serial 패키지의 인터페이스를 래핑하여 mock 가능하도록 설계, 실 환경 테스트는 통합 테스트에서 수행
- **심각도**: 중간 (인터페이스 기반 mock으로 단위 테스트 가능)

### R3: Protocol Definition 복잡도

- **리스크**: 조건부 파싱, 가변 길이 필드 등 복잡한 프로토콜 구조 지원 시 파서 복잡도 증가
- **대응**: 단계적 구현 (고정 길이 -> 가변 길이 -> 조건부 파싱), 각 단계별 테스트 충분히 확보
- **심각도**: 중간 (단계적 구현으로 복잡도 관리)

### R4: Health Check goroutine 누수

- **리스크**: HealthChecker goroutine이 적절히 종료되지 않으면 리소스 누수 발생
- **대응**: context.Context 기반 종료, defer 패턴, goroutine 누수 테스트 (`goleak` 활용)
- **심각도**: 중간 (context 기반 종료로 방지 가능)

### R5: Agent Manager 동시성 교착 상태

- **리스크**: Manager와 Registry 간 교차 잠금 시 교착 상태 발생 가능
- **대응**: 잠금 순서 규약 확립 (Manager -> Registry), `go test -race` 필수 실행
- **심각도**: 낮음 (일관된 잠금 순서로 방지)

---

## 5. 의존성 순서

```
구현 순서 (의존성 방향):

1. errors.go                     (의존성 없음)
2. protocol/field.go             (의존성 없음)
3. protocol/checksum.go          (의존성 없음)
4. protocol/definition.go        (field.go, checksum.go 의존)
5. protocol/parser.go            (definition.go 의존)
6. protocol/loader.go            (definition.go 의존)
7. transport/transport.go        (의존성 없음 - 인터페이스만)
8. config.go                     (transport, protocol 타입 의존)
9. info.go                       (config.go, transport, protocol, pkg/lifecycle 타입 의존)
10. agent.go                     (config.go, info.go, pkg/lifecycle 의존)
11. registry.go                  (agent.go 의존)
12. manager.go                   (agent.go, registry.go, info.go 의존)
13. health.go                    (agent.go, manager.go 의존)
14. shared.go                    (agent.go 의존)
15. system.go                    (agent.go 의존 - 인터페이스만)
16. type_registry.go             (agent.go, config.go 의존)
17. transport/serial.go          (transport.go 의존)
18. transport/tcp.go             (transport.go 의존)
19. transport/udp.go             (transport.go 의존)
20. doc.go                       (문서만)
```

---

## 6. 테스트 전략

### 6.1 단위 테스트 구조

- 모든 테스트는 테이블 드리븐 방식으로 작성
- mock Transport/Protocol을 테스트 파일 내에 정의
- `testify/assert` 및 `testify/require` 사용
- 각 모듈의 테스트 파일은 해당 소스 파일과 같은 디렉토리에 위치

### 6.2 테스트 카테고리

| 카테고리 | 대상 | 검증 항목 |
|---------|------|---------|
| 인터페이스 테스트 | Agent, Transport, Parser | 인터페이스 계약 준수, 모든 메서드 동작 검증 |
| 상태 전이 테스트 | BaseAgent | 유효/무효 상태 전이, Pause/Resume 버퍼링, StatusEvent 발행 |
| 동시성 테스트 | Manager, Registry, SharedRef | RWMutex 보호, atomic 연산, data race 없음 |
| 파싱 테스트 | Parser, Field, Checksum | 바이트 변환 정확성, 체크섬 검증, 조건부 파싱 |
| 로더 테스트 | Loader | YAML/JSON 로드, 유효성 검증, 에러 처리 |
| 설정 테스트 | AgentConfig | Validate() 검증, 기본값, 불변 설정 거부 |
| Info & Stats 테스트 | AgentInfo, AgentStats, ManagerSummary | Info() 스냅샷 정확성, Stats atomic 카운터, ResetStats(), 재시작 통계 보존, Summary() 집계 |
| sentinel 에러 테스트 | errors.Is() | 모든 sentinel 에러의 비교 호환성 |
| 헬스 체크 테스트 | HealthChecker | 주기적 체크, 콜백 호출, 지수 백오프, 최대 재시작 |
| 공유 참조 테스트 | SharedRef | 참조 카운팅, 0 도달 동작, 중복 참조 거부 |

### 6.3 벤치마크 테스트

- `BenchmarkParse` - 프로토콜 파싱 성능
- `BenchmarkSerialize` - 프로토콜 직렬화 성능
- `BenchmarkChecksumCRC16` - CRC-16 체크섬 계산 성능
- `BenchmarkChecksumModbus` - Modbus CRC 계산 성능
- `BenchmarkManagerCreate` - Agent 생성 성능
- `BenchmarkRegistryGet` - Agent 조회 성능 (수백 개 Agent 등록 시)

### 6.4 통합 테스트

통합 테스트는 `test/integration/agent_test.go`에서 수행한다:

- Agent 전체 생명주기 테스트 (Create -> Init -> Start -> Process -> Pause -> Resume -> Stop -> Delete)
- Health Check + 자동 재시작 통합 테스트
- Shared Agent + 다중 플로우 참조 통합 테스트
- Transport + Protocol 조합 통합 테스트 (TCP + 커스텀 프로토콜)
