---
id: SPEC-AGENT-001
version: "1.1.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-13 | 1.1.0 | Module 16 (Agent Info & Stats) 추가, Agent/Manager 인터페이스에 Info()/Stats()/Summary() 메서드 추가, 기존 Module 16 -> 17 번호 변경 |

---

# SPEC-AGENT-001: Agent System - 설정 기반 프로토콜 파싱 서비스 프레임워크

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에서 Agent는 플로우와 독립적으로 실행되는 서비스 단위이다. 사용자가 프로토콜 구조를 설정하면 이에 맞게 데이터를 파싱하며, 통신 인터페이스(Serial, TCP 등)를 선택할 수 있다. 여러 플로우에서 공유하여 사용할 수 있다.

본 SPEC은 Agent 시스템의 **프레임워크**를 정의한다:

- **Agent 인터페이스 및 BaseAgent**: Agent의 공통 동작 계약과 기본 구현
- **Agent Manager**: Agent 생명주기 관리 (생성, 시작, 중지, 재시작, 삭제)
- **Agent Registry**: 실행 중인 Agent 목록 관리 및 조회
- **Agent Health Check**: 주기적 헬스 체크, 장애 감지, 자동 재시작
- **Shared Agent**: 다중 플로우 참조 카운팅 기반 공유 관리
- **Transport Interface**: 통신 인터페이스 추상화 (Serial, TCP, UDP)
- **Protocol Definition Engine**: 설정 기반 바이트스트림 파서/직렬화기
- **Agent Configuration**: AgentConfig 구조체 및 설정 관리
- **System Agent Interface**: Transport/Protocol 없이 직접 참조 가능한 내장 서비스 인터페이스
- **Agent Types Registry**: Agent 타입별 팩토리 등록
- **Error Types**: 본 패키지 전용 sentinel 에러 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/`
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스, 생명주기 상태 관리
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스 (Agent <-> Bridge 메시지 교환)
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
  - `pkg/flow/` (SPEC-FLOW-001): Flow 구조 참조 (다중 플로우 공유)
- **소비 패키지**:
  - `internal/node/` (SPEC-NODE-001): Bridge 노드가 Agent를 참조
  - `internal/engine/` (SPEC-ENGINE-001): 플로우 엔진이 Agent를 참조
  - `internal/observe/` (SPEC-OBS-001): Agent 관찰성 통합
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **외부 의존성**:
  - `go.bug.st/serial` (Serial Transport)
  - `sync` 패키지 (RWMutex, Map, atomic)

### 1.3 설계 원칙

- **인터페이스 우선**: Agent, Transport, ProtocolDefinition은 인터페이스로 정의하여 확장 가능
- **플로우 독립 실행**: Agent는 플로우 생명주기와 무관하게 독립적으로 실행
- **다중 플로우 공유**: 하나의 Agent 인스턴스를 여러 플로우에서 참조 카운팅으로 공유
- **설정 기반 파싱**: 프로토콜 구조를 YAML/JSON으로 정의하여 바이트 데이터를 자동 파싱/직렬화
- **동시성 안전**: Manager, Registry는 sync.RWMutex로 보호
- **Graceful Shutdown**: 모든 Agent는 정상 종료를 보장
- **관찰성 내장**: 연결 상태, 프로토콜 파싱 결과, 헬스 체크 등 추적 가능

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Agent 인터페이스 및 BaseAgent 기본 구현
- Agent Manager (생명주기 관리)
- Agent Registry (목록 관리 및 조회)
- Agent Health Check (헬스 체크, 자동 재시작)
- Shared Agent (다중 플로우 참조 카운팅)
- Transport Interface (인터페이스 정의 + Serial/TCP/UDP 구현)
- Protocol Definition Engine (정의 구조체, 파서, 필드 타입, 체크섬, 로더)
- Agent Configuration (AgentConfig 구조체)
- System Agent Interface (인터페이스 정의만)
- Agent Types Registry (팩토리 등록)
- Error Types (sentinel 에러)

**OUT OF SCOPE (별도 SPEC)**:
- Bridge 노드 구현 (SPEC-NODE-001: bridge.go)
- 표준 Agent 구현 - MQTT, HTTP, WebSocket, gRPC (별도 SPEC: SPEC-AGENT-MQTT-001 등)
- System Agent 구현 - Event, Logger, File, Timer, Store (별도 SPEC: SPEC-SYSAGENT-001)
- Samsung NASA Agent 구현 (별도 SPEC)
- 플로우 엔진의 Agent 참조 방식 (SPEC-ENGINE-001)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | State 인터페이스, 생명주기 상태 전이 |
| SPEC-MSG-001 | 의존 | Message 인터페이스 (Agent <-> Bridge 메시지 교환) |
| SPEC-ERR-001 | 의존 | ErrorMessage, StatusEvent 타입 (에러 출력, 상태 이벤트) |
| SPEC-FLOW-001 | 의존 | Flow 구조 참조 (다중 플로우 공유 시 플로우 ID) |
| SPEC-NODE-001 | 소비자 | Bridge 노드가 Agent를 참조하여 메시지 교환 |
| SPEC-ENGINE-001 | 소비자 | 플로우 엔진이 Agent Manager를 통해 Agent 참조 |
| SPEC-OBS-001 | 소비자 | Agent 관찰성 (연결 상태, 파싱 결과, 헬스 체크 메트릭) |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `pkg/lifecycle/` 패키지가 State 인터페이스, 상태 전이 함수, 7개 공통 상태 상수를 제공한다 (SPEC-LIFE-001)
- A2: `pkg/message/` 패키지가 Message 인터페이스를 제공하며, Agent는 이 인터페이스를 통해 Bridge 노드와 메시지를 교환한다 (SPEC-MSG-001)
- A3: `pkg/xferr/` 패키지가 ErrorMessage, StatusEvent 타입을 제공한다 (SPEC-ERR-001)
- A4: Go의 `sync.RWMutex`, `sync/atomic` 패키지가 Agent Manager와 Registry의 동시성 보호에 충분하다
- A5: Transport 인터페이스의 Read/Write는 blocking 호출이며, 별도 goroutine에서 실행된다
- A6: Protocol Definition의 YAML/JSON 파일은 Agent 시작 시 1회 로드되며, 런타임 변경은 Configure()를 통해 수행한다

### 2.2 도메인 가정

- A7: Agent는 Transport + Protocol Definition의 조합으로 동작하며, System Agent는 이 두 요소가 불필요하다
- A8: 하나의 Agent 인스턴스는 하나의 Transport에 바인딩되며, Transport 타입 변경 시 Agent 재생성이 필요하다
- A9: 다중 플로우 공유 시 참조 카운트가 0이 되면 Agent 중지 여부는 구성 가능하다
- A10: Health Check는 주기적으로 Transport의 Available() 상태와 마지막 성공 시각을 기반으로 판단한다
- A11: 프로토콜 정의의 필드 타입은 uint8, uint16_be/le, int32, float32, string, bytes, bitmask로 초기 버전에 충분하다
- A12: 체크섬 알고리즘은 CRC-16, CRC-32, XOR, Modbus CRC로 초기 버전에 충분하다
- A13: 조건부 파싱(헤더 값에 따라 페이로드 구조 분기)은 Protocol Definition Engine의 핵심 기능이다

---

## 3. Requirements (요구사항)

### Module 1: Agent Interface & BaseAgent (P0)

#### REQ-AGENT-001-01-01 (Ubiquitous) Agent 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Agent` 인터페이스를 제공해야 한다:

- `Init(config AgentConfig) error` - Agent 초기화 (Transport 연결, Protocol 로딩)
- `Start(ctx context.Context) error` - Agent 실행 시작
- `Stop(ctx context.Context) error` - Agent 정상 종료
- `Pause(ctx context.Context) error` - Agent 일시정지 (연결 유지, 수신 버퍼링)
- `Resume(ctx context.Context) error` - Agent 재개 (버퍼링된 데이터 처리)
- `Health() HealthStatus` - 현재 헬스 상태 반환
- `Process(data []byte) ([]byte, error)` - 수신 데이터 처리 (파싱/직렬화)
- `Configure(config AgentConfig) error` - 런타임 설정 변경
- `ID() string` - Agent 고유 식별자 반환
- `Name() string` - Agent 표시 이름 반환
- `Type() string` - Agent 타입 반환
- `Info() AgentInfo` - Agent 종합 상태 스냅샷 반환
- `Stats() AgentStats` - Agent 처리 통계 반환

#### REQ-AGENT-001-01-02 (Ubiquitous) BaseAgent 기본 구현

시스템은 **항상** `BaseAgent` 구조체를 `Agent` 인터페이스의 기본 구현으로 제공해야 한다:

- `pkg/lifecycle/` 연동을 통한 상태 관리 (Created -> Initializing -> Running <-> Paused -> Stopping -> Stopped)
- Transport 인터페이스 래핑 (통신 추상화)
- Protocol Definition 연동 (설정 기반 파싱/직렬화)
- `pkg/xferr/` 연동을 통한 에러 출력 포트 지원
- `sync.RWMutex`로 내부 상태 동시성 보호
- `AgentStats` 내부 카운터 관리 (`atomic` 연산으로 동시성 안전 보장)
- `Info()` 구현 시 현재 상태, Transport 정보, Config 등 집계하여 `AgentInfo` 스냅샷 반환

#### REQ-AGENT-001-01-03 (Event-Driven) Agent 상태 전이 이벤트 발행

**WHEN** Agent의 상태가 변경되면, **THEN** `pkg/xferr/StatusEvent`를 생성하여 에러 라우터에 전달해야 한다.

#### REQ-AGENT-001-01-04 (State-Driven) 일시정지 상태에서의 데이터 버퍼링

**IF** Agent가 Paused 상태이면, **THEN** Transport에서 수신되는 데이터를 내부 버퍼에 축적해야 한다. 버퍼 크기는 AgentConfig에서 설정 가능하다.

#### REQ-AGENT-001-01-05 (Event-Driven) 재개 시 버퍼 데이터 우선 처리

**WHEN** Agent가 Paused에서 Running으로 전환되면, **THEN** 내부 버퍼에 축적된 데이터를 우선 처리하여 데이터 연속성을 보장해야 한다.

#### REQ-AGENT-001-01-06 (Unwanted) 잘못된 상태 전이 거부

시스템은 유효하지 않은 상태 전이(예: Stopped -> Paused)를 수행**하지 않아야 한다**. `ErrInvalidStateTransition` 에러를 반환해야 한다.

---

### Module 2: Agent Manager (P0)

#### REQ-AGENT-001-02-01 (Ubiquitous) Manager 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Manager` 인터페이스를 제공해야 한다:

- `Create(config AgentConfig) (Agent, error)` - Agent 인스턴스 생성
- `Start(ctx context.Context, agentID string) error` - Agent 실행 시작
- `Stop(ctx context.Context, agentID string) error` - Agent 정상 종료
- `Restart(ctx context.Context, agentID string) error` - Agent 재시작 (Stop + Start)
- `Delete(agentID string) error` - Agent 인스턴스 삭제 (Running 상태이면 먼저 Stop)
- `Get(agentID string) (Agent, error)` - Agent 조회
- `List() []Agent` - 모든 Agent 목록 반환
- `Shutdown(ctx context.Context) error` - 모든 Agent Graceful Shutdown
- `Summary() ManagerSummary` - 전체 Agent 관리 데이터 요약

#### REQ-AGENT-001-02-02 (Ubiquitous) Manager 동시성 안전

시스템은 **항상** `Manager`의 모든 메서드가 동시성 안전(goroutine-safe)해야 한다. `sync.RWMutex`를 사용하여 내부 Agent 맵을 보호해야 한다.

#### REQ-AGENT-001-02-03 (Event-Driven) Graceful Shutdown

**WHEN** `Manager.Shutdown(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 모든 Running/Paused 상태 Agent에 Stop 전파
2. context deadline까지 모든 Agent의 정상 종료 대기
3. deadline 초과 시 강제 종료 및 에러 로깅

#### REQ-AGENT-001-02-04 (Unwanted) 중복 Agent ID 생성 거부

시스템은 이미 존재하는 Agent ID로 `Create`를 호출한 경우 Agent를 생성**하지 않아야 한다**. `ErrAgentAlreadyExists` 에러를 반환해야 한다.

#### REQ-AGENT-001-02-05 (Unwanted) 존재하지 않는 Agent 조작 거부

시스템은 존재하지 않는 Agent ID로 `Start`, `Stop`, `Restart`, `Delete`를 호출한 경우 해당 작업을 수행**하지 않아야 한다**. `ErrAgentNotFound` 에러를 반환해야 한다.

---

### Module 3: Agent Registry (P0)

#### REQ-AGENT-001-03-01 (Ubiquitous) Registry 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Registry` 인터페이스를 제공해야 한다:

- `Register(agent Agent) error` - Agent 등록
- `Unregister(agentID string) error` - Agent 등록 해제
- `Get(agentID string) (Agent, bool)` - ID 기반 Agent 조회
- `GetByName(name string) (Agent, bool)` - 이름 기반 Agent 조회
- `GetByType(agentType string) []Agent` - 타입 기반 Agent 필터링
- `List() []Agent` - 전체 Agent 목록
- `Count() int` - 등록된 Agent 수

#### REQ-AGENT-001-03-02 (Ubiquitous) Registry 동시성 안전

시스템은 **항상** `Registry`의 모든 메서드가 동시성 안전해야 한다. `sync.RWMutex`를 사용하여 내부 레지스트리를 보호해야 한다.

#### REQ-AGENT-001-03-03 (Unwanted) 중복 Agent 등록 거부

시스템은 이미 등록된 Agent ID로 `Register`를 호출한 경우 등록을 수행**하지 않아야 한다**. `ErrAgentAlreadyExists` 에러를 반환해야 한다.

---

### Module 4: Agent Health Check (P1)

#### REQ-AGENT-001-04-01 (Ubiquitous) HealthStatus 구조체 정의

시스템은 **항상** `HealthStatus` 구조체를 제공해야 한다:

- `Status HealthState` - 헬스 상태 (Healthy, Degraded, Unhealthy)
- `LastCheck time.Time` - 마지막 헬스 체크 시각
- `LastSuccess time.Time` - 마지막 성공 시각
- `ConsecutiveFailures int` - 연속 실패 횟수
- `Message string` - 상태 설명 메시지

#### REQ-AGENT-001-04-02 (Ubiquitous) HealthState 열거 정의

시스템은 **항상** 다음 `HealthState` 상수를 제공해야 한다:

- `HealthHealthy` - 정상 (Transport 연결 활성, 최근 데이터 수신)
- `HealthDegraded` - 저하 (연결은 유지되나 응답 지연 또는 간헐적 실패)
- `HealthUnhealthy` - 비정상 (Transport 연결 끊김 또는 연속 실패 임계값 초과)

#### REQ-AGENT-001-04-03 (Ubiquitous) HealthChecker 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `HealthChecker` 인터페이스를 제공해야 한다:

- `Start(ctx context.Context) error` - 주기적 헬스 체크 시작
- `Stop() error` - 헬스 체크 중지
- `Check(agentID string) HealthStatus` - 특정 Agent 즉시 헬스 체크
- `SetInterval(d time.Duration)` - 헬스 체크 주기 변경
- `OnUnhealthy(callback func(agentID string, status HealthStatus))` - Unhealthy 콜백 등록

#### REQ-AGENT-001-04-04 (Event-Driven) Unhealthy 감지 시 자동 재시작

**WHEN** Agent의 HealthState가 Unhealthy가 되면, **THEN** 지수 백오프(exponential backoff) 기반 재시작 전략을 실행해야 한다:

1. 초기 대기 시간: 1초
2. 최대 대기 시간: 5분
3. 백오프 배수: 2배
4. 최대 재시작 횟수: AgentConfig에서 설정 가능 (기본: 10)

#### REQ-AGENT-001-04-05 (Unwanted) 최대 재시작 횟수 초과 금지

시스템은 최대 재시작 횟수를 초과하여 Agent를 재시작**하지 않아야 한다**. `ErrMaxRestartsExceeded` 에러를 발행하고 Agent를 Stopped 상태로 전환해야 한다.

---

### Module 5: Shared Agent - 다중 플로우 공유 (P1)

#### REQ-AGENT-001-05-01 (Ubiquitous) SharedRef 구조체 정의

시스템은 **항상** `SharedRef` 구조체를 제공해야 한다:

- `AgentID() string` - 참조 대상 Agent ID
- `RefCount() int32` - 현재 참조 카운트
- `Acquire(flowID string) error` - 참조 획득 (카운트 증가)
- `Release(flowID string) error` - 참조 해제 (카운트 감소)
- `Flows() []string` - 현재 참조 중인 플로우 ID 목록

#### REQ-AGENT-001-05-02 (Ubiquitous) 참조 카운트 원자적 관리

시스템은 **항상** 참조 카운트를 `atomic.Int32`로 관리하여 동시성 안전을 보장해야 한다.

#### REQ-AGENT-001-05-03 (Event-Driven) 참조 카운트 0 도달 시 동작

**WHEN** Agent의 참조 카운트가 0이 되면, **THEN** AgentConfig의 `StopOnZeroRef` 설정에 따라 다음을 수행해야 한다:

- `true` (기본): Agent Stop 요청
- `false`: Agent 유지 (유휴 상태)

#### REQ-AGENT-001-05-04 (State-Driven) 플로우 재배포 시 연결 유지

**IF** 플로우가 재배포되어 Agent 참조가 Release 후 즉시 Acquire되면, **THEN** Agent의 Transport 연결은 유지되어야 한다.

#### REQ-AGENT-001-05-05 (Unwanted) 중복 플로우 참조 거부

시스템은 동일한 flowID로 `Acquire`를 중복 호출한 경우 참조 카운트를 증가시키**지 않아야 한다**. `ErrFlowAlreadyReferenced` 에러를 반환해야 한다.

---

### Module 6: Transport Interface (P0)

#### REQ-AGENT-001-06-01 (Ubiquitous) Transport 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Transport` 인터페이스를 제공해야 한다:

- `Open(config TransportConfig) error` - 연결 열기
- `Close() error` - 연결 닫기
- `Read(buf []byte) (int, error)` - 데이터 읽기
- `Write(data []byte) (int, error)` - 데이터 쓰기
- `Available() bool` - 연결 상태 확인

#### REQ-AGENT-001-06-02 (Ubiquitous) TransportConfig 구조체 정의

시스템은 **항상** `TransportConfig` 구조체를 제공해야 한다:

- `Type string` - Transport 타입명 ("serial", "tcp", "udp")
- `Options map[string]any` - Transport별 설정 옵션

#### REQ-AGENT-001-06-03 (Ubiquitous) TransportFactory 팩토리 패턴

시스템은 **항상** `TransportFactory` 인터페이스를 제공해야 한다:

- `Create(transportType string) (Transport, error)` - 타입명에 따른 Transport 인스턴스 생성
- `Register(transportType string, creator func() Transport)` - 새 Transport 타입 등록

#### REQ-AGENT-001-06-04 (Unwanted) 미등록 Transport 타입 거부

시스템은 등록되지 않은 Transport 타입으로 `Create`를 호출한 경우 Transport를 생성**하지 않아야 한다**. `ErrTransportNotAvailable` 에러를 반환해야 한다.

---

### Module 7: Serial Transport (P1)

#### REQ-AGENT-001-07-01 (Ubiquitous) SerialTransport 구현

시스템은 **항상** `go.bug.st/serial` 패키지를 활용하여 `SerialTransport` 구현체를 제공해야 한다:

- RS-485/RS-232 통신 지원
- 보레이트(BaudRate), 패리티(Parity), 스톱비트(StopBits), 데이터비트(DataBits) 설정

#### REQ-AGENT-001-07-02 (Event-Driven) 시리얼 연결 끊김 시 자동 재연결

**WHEN** 시리얼 포트 연결이 끊기면, **THEN** 지수 백오프 기반으로 자동 재연결을 시도해야 한다.

#### REQ-AGENT-001-07-03 (Optional) SerialConfig 확장 옵션

**가능하면** 다음 확장 옵션을 제공해야 한다:
- 읽기 타임아웃 설정
- 쓰기 타임아웃 설정
- DTR/RTS 핀 제어

---

### Module 8: TCP Transport (P1)

#### REQ-AGENT-001-08-01 (Ubiquitous) TCPTransport 구현

시스템은 **항상** Go 표준 `net` 패키지를 활용하여 `TCPTransport` 구현체를 제공해야 한다:

- TCP 클라이언트 및 서버 모드 지원
- 연결 풀링 지원
- 타임아웃(연결, 읽기, 쓰기) 관리

#### REQ-AGENT-001-08-02 (Event-Driven) TCP 연결 끊김 시 자동 재연결

**WHEN** TCP 연결이 끊기면, **THEN** 지수 백오프 기반으로 자동 재연결을 시도해야 한다.

#### REQ-AGENT-001-08-03 (Optional) TLS 지원

**가능하면** `crypto/tls` 패키지를 활용한 TLS 암호화 연결을 지원해야 한다.

---

### Module 9: UDP Transport (P2)

#### REQ-AGENT-001-09-01 (Ubiquitous) UDPTransport 구현

시스템은 **항상** Go 표준 `net` 패키지를 활용하여 `UDPTransport` 구현체를 제공해야 한다:

- UDP 유니캐스트 통신 지원
- 수신 버퍼 크기 설정

#### REQ-AGENT-001-09-02 (Optional) 멀티캐스트 지원

**가능하면** UDP 멀티캐스트 그룹 가입 및 수신을 지원해야 한다.

---

### Module 10: Protocol Definition Engine (P0)

#### REQ-AGENT-001-10-01 (Ubiquitous) ProtocolDefinition 구조체 정의

시스템은 **항상** `ProtocolDefinition` 구조체를 제공해야 한다:

- `Name string` - 프로토콜 이름
- `Version string` - 프로토콜 버전
- `ByteOrder ByteOrder` - 기본 바이트 순서 (BigEndian, LittleEndian)
- `Header []FieldDefinition` - 헤더 필드 정의
- `Payload []FieldDefinition` - 페이로드 필드 정의
- `Trailer []FieldDefinition` - 트레일러 필드 정의
- `Checksum *ChecksumDefinition` - 체크섬 설정 (nil이면 미사용)
- `ConditionalPayloads []ConditionalPayload` - 조건부 페이로드 정의

#### REQ-AGENT-001-10-02 (Ubiquitous) FieldDefinition 구조체 정의

시스템은 **항상** `FieldDefinition` 구조체를 제공해야 한다:

- `Name string` - 필드 이름
- `Type FieldType` - 필드 타입
- `Size int` - 필드 크기 (바이트, 가변 길이 시 0)
- `ByteOrder *ByteOrder` - 필드별 바이트 순서 오버라이드 (nil이면 프로토콜 기본값)
- `Description string` - 필드 설명

#### REQ-AGENT-001-10-03 (Ubiquitous) FieldType 열거 정의

시스템은 **항상** 다음 `FieldType` 상수를 제공해야 한다:

- `FieldUint8` - unsigned 8-bit integer
- `FieldUint16BE` - unsigned 16-bit integer (Big Endian)
- `FieldUint16LE` - unsigned 16-bit integer (Little Endian)
- `FieldInt32` - signed 32-bit integer
- `FieldFloat32` - 32-bit floating point
- `FieldString` - 문자열 (길이 필드 또는 고정 크기)
- `FieldBytes` - 원시 바이트 배열
- `FieldBitmask` - 비트마스크 필드

#### REQ-AGENT-001-10-04 (Ubiquitous) Parser 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Parser` 인터페이스를 제공해야 한다:

- `Parse(data []byte) (*ParsedMessage, error)` - 바이트 데이터를 구조화된 메시지로 파싱
- `Serialize(msg *ParsedMessage) ([]byte, error)` - 구조화된 메시지를 바이트 데이터로 직렬화

#### REQ-AGENT-001-10-05 (Ubiquitous) ParsedMessage 구조체 정의

시스템은 **항상** `ParsedMessage` 구조체를 제공해야 한다:

- `Header map[string]any` - 파싱된 헤더 필드
- `Payload map[string]any` - 파싱된 페이로드 필드
- `Trailer map[string]any` - 파싱된 트레일러 필드
- `RawData []byte` - 원본 바이트 데이터
- `ChecksumValid bool` - 체크섬 검증 결과

#### REQ-AGENT-001-10-06 (Event-Driven) 조건부 파싱

**WHEN** 헤더 필드의 값이 `ConditionalPayload`의 조건과 매칭되면, **THEN** 해당 조건에 정의된 페이로드 구조로 파싱해야 한다.

#### REQ-AGENT-001-10-07 (Unwanted) 체크섬 불일치 거부

시스템은 체크섬 검증이 실패한 경우 파싱 결과를 유효한 것으로 처리**하지 않아야 한다**. `ErrChecksumMismatch` 에러를 반환하고, `ParsedMessage.ChecksumValid`를 `false`로 설정해야 한다.

#### REQ-AGENT-001-10-08 (Unwanted) 불충분한 데이터 파싱 거부

시스템은 프로토콜 정의에 비해 데이터 길이가 부족한 경우 파싱을 수행**하지 않아야 한다**. `ErrProtocolParseError` 에러를 반환해야 한다.

---

### Module 11: Protocol Checksum (P0)

#### REQ-AGENT-001-11-01 (Ubiquitous) ChecksumDefinition 구조체 정의

시스템은 **항상** `ChecksumDefinition` 구조체를 제공해야 한다:

- `Algorithm ChecksumAlgorithm` - 체크섬 알고리즘
- `StartOffset int` - 체크섬 계산 시작 오프셋
- `EndOffset int` - 체크섬 계산 종료 오프셋 (음수이면 끝에서부터)
- `ResultSize int` - 체크섬 결과 크기 (바이트)

#### REQ-AGENT-001-11-02 (Ubiquitous) ChecksumAlgorithm 열거 정의

시스템은 **항상** 다음 `ChecksumAlgorithm` 상수를 제공해야 한다:

- `ChecksumCRC16` - CRC-16 알고리즘
- `ChecksumCRC32` - CRC-32 알고리즘
- `ChecksumXOR` - XOR 체크섬
- `ChecksumModbusCRC` - Modbus CRC 알고리즘

#### REQ-AGENT-001-11-03 (Ubiquitous) Checksum 계산 및 검증 함수

시스템은 **항상** 다음 함수를 제공해야 한다:

- `Calculate(algorithm ChecksumAlgorithm, data []byte) ([]byte, error)` - 체크섬 계산
- `Verify(algorithm ChecksumAlgorithm, data []byte, expected []byte) (bool, error)` - 체크섬 검증

---

### Module 12: Protocol Loader (P1)

#### REQ-AGENT-001-12-01 (Ubiquitous) Loader 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Loader` 인터페이스를 제공해야 한다:

- `LoadFromFile(path string) (*ProtocolDefinition, error)` - YAML/JSON 파일에서 프로토콜 정의 로드
- `LoadFromBytes(data []byte, format string) (*ProtocolDefinition, error)` - 바이트 데이터에서 프로토콜 정의 로드
- `Validate(def *ProtocolDefinition) error` - 프로토콜 정의 유효성 검증

#### REQ-AGENT-001-12-02 (Event-Driven) YAML/JSON 자동 감지

**WHEN** `LoadFromFile`이 호출되면, **THEN** 파일 확장자(.yaml, .yml, .json)에 따라 적절한 디코더를 자동 선택해야 한다.

#### REQ-AGENT-001-12-03 (Unwanted) 유효하지 않은 프로토콜 정의 거부

시스템은 다음 경우 프로토콜 정의를 로드**하지 않아야 한다**:
- 필수 필드(Name) 누락
- 중복 필드 이름
- 유효하지 않은 FieldType
- 유효하지 않은 ChecksumAlgorithm
- 조건부 파싱 참조 필드가 헤더에 존재하지 않는 경우

`ErrInvalidConfig` 에러를 반환해야 한다.

---

### Module 13: Agent Configuration (P1)

#### REQ-AGENT-001-13-01 (Ubiquitous) AgentConfig 구조체 정의

시스템은 **항상** `AgentConfig` 구조체를 제공해야 한다:

- `ID string` - Agent 고유 식별자
- `Name string` - Agent 표시 이름
- `Type string` - Agent 타입 ("custom", "mqtt", "http", "system" 등)
- `Transport TransportConfig` - Transport 설정
- `Protocol *ProtocolDefinition` - Protocol 정의 (인라인) 또는 nil
- `ProtocolFile string` - Protocol 정의 파일 경로 (Protocol이 nil일 때 사용)
- `HealthCheckInterval time.Duration` - 헬스 체크 주기 (기본: 30초)
- `MaxRestarts int` - 최대 재시작 횟수 (기본: 10)
- `StopOnZeroRef bool` - 참조 카운트 0 시 자동 중지 (기본: true)
- `BufferSize int` - 일시정지 시 수신 버퍼 크기 (기본: 1024)
- `Metadata map[string]string` - Agent 메타데이터

#### REQ-AGENT-001-13-02 (Ubiquitous) AgentConfig 유효성 검증

시스템은 **항상** `AgentConfig.Validate() error` 메서드를 제공해야 한다:

- ID 비어있지 않은지 확인
- Name 비어있지 않은지 확인
- HealthCheckInterval이 0보다 큰지 확인
- MaxRestarts가 0 이상인지 확인
- BufferSize가 0보다 큰지 확인

#### REQ-AGENT-001-13-03 (Unwanted) 불변 설정 런타임 변경 거부

시스템은 Agent가 Running 상태에서 Transport 타입 변경을 수행**하지 않아야 한다**. `ErrConfigImmutable` 에러를 반환해야 한다.

---

### Module 14: System Agent Interface (P1)

#### REQ-AGENT-001-14-01 (Ubiquitous) SystemAgent 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `SystemAgent` 인터페이스를 제공해야 한다:

- Agent 인터페이스 임베딩 (모든 Agent 메서드 상속)
- `IsSystem() bool` - 항상 true 반환
- `RequiresTransport() bool` - 항상 false 반환

#### REQ-AGENT-001-14-02 (Ubiquitous) SystemAgentType 열거 정의

시스템은 **항상** 다음 System Agent 타입 상수를 제공해야 한다:

- `SystemAgentEvent` - 시스템 이벤트 발행/구독
- `SystemAgentLogger` - 로그 관리
- `SystemAgentFile` - 파일 시스템 접근
- `SystemAgentTimer` - 타이머/스케줄러
- `SystemAgentStore` - 키-값 저장소

#### REQ-AGENT-001-14-03 (State-Driven) System Agent 자동 활성화

**IF** 시스템이 시작되면, **THEN** 모든 System Agent가 자동으로 생성 및 활성화되어야 한다. 별도의 사용자 등록이 불필요하다.

---

### Module 15: Agent Types Registry (P1)

#### REQ-AGENT-001-15-01 (Ubiquitous) TypeRegistry 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `TypeRegistry` 인터페이스를 제공해야 한다:

- `RegisterType(agentType string, factory AgentFactory) error` - Agent 타입 등록
- `CreateAgent(agentType string, config AgentConfig) (Agent, error)` - 타입에 따른 Agent 생성
- `ListTypes() []string` - 등록된 타입 목록
- `HasType(agentType string) bool` - 타입 존재 여부

#### REQ-AGENT-001-15-02 (Ubiquitous) AgentFactory 타입 정의

시스템은 **항상** `AgentFactory` 함수 타입을 제공해야 한다:

- `type AgentFactory func(config AgentConfig) (Agent, error)` - Agent 생성 팩토리

#### REQ-AGENT-001-15-03 (Ubiquitous) 기본 Agent 타입 등록

시스템은 **항상** 다음 기본 Agent 타입을 자동 등록해야 한다:

- `"custom"` - 사용자 정의 프로토콜 Agent (BaseAgent + Transport + Protocol)
- System Agent 타입 5종 (`"event"`, `"logger"`, `"file"`, `"timer"`, `"store"`)

표준 Agent 타입 (mqtt, http, websocket, grpc)은 별도 SPEC에서 등록한다.

#### REQ-AGENT-001-15-04 (Unwanted) 중복 타입 등록 거부

시스템은 이미 등록된 Agent 타입명으로 `RegisterType`을 호출한 경우 등록을 수행**하지 않아야 한다**. 에러를 반환해야 한다.

---

### Module 16: Agent Info & Stats (P0)

#### REQ-AGENT-001-16-01 (Ubiquitous) AgentInfo 구조체 정의

시스템은 **항상** `AgentInfo` 구조체를 제공해야 한다:

- `ID string` - Agent 고유 식별자
- `Name string` - Agent 표시 이름
- `Type string` - Agent 타입 (mqtt, http, custom 등)
- `State lifecycle.State` - 현재 생명주기 상태 (Created, Running, Paused, Stopped 등)
- `Health HealthStatus` - 현재 헬스 상태
- `Transport TransportInfo` - Transport 연결 정보
- `Protocol ProtocolInfo` - 프로토콜 정의 정보
- `Config AgentConfig` - 현재 설정 스냅샷
- `Stats AgentStats` - 처리 통계
- `SharedInfo *SharedInfo` - 공유 참조 정보 (nil if not shared)
- `StartedAt time.Time` - 마지막 시작 시각 (zero if never started)
- `Uptime time.Duration` - 현재 가동 시간 (Running 상태에서의 누적 시간)
- `CreatedAt time.Time` - Agent 생성 시각

#### REQ-AGENT-001-16-02 (Ubiquitous) TransportInfo 구조체 정의

시스템은 **항상** `TransportInfo` 구조체를 제공해야 한다:

- `Type string` - Transport 타입 (serial, tcp, udp)
- `Connected bool` - 현재 연결 상태
- `RemoteAddr string` - 원격 주소 (해당되는 경우)
- `LocalAddr string` - 로컬 주소 (해당되는 경우)
- `ConnectedAt time.Time` - 연결 수립 시각

#### REQ-AGENT-001-16-03 (Ubiquitous) ProtocolInfo 구조체 정의

시스템은 **항상** `ProtocolInfo` 구조체를 제공해야 한다:

- `Name string` - 프로토콜 정의 이름
- `FieldCount int` - 정의된 필드 수
- `HasChecksum bool` - 체크섬 사용 여부
- `SourceFile string` - 프로토콜 정의 파일 경로 (인라인이면 빈 문자열)

#### REQ-AGENT-001-16-04 (Ubiquitous) AgentStats 구조체 정의

시스템은 **항상** `AgentStats` 구조체를 제공해야 한다:

- `MessagesReceived int64` - 수신 메시지 총 수
- `MessagesSent int64` - 송신 메시지 총 수
- `MessagesErrored int64` - 처리 중 에러 발생 메시지 수
- `BytesRead int64` - Transport에서 읽은 총 바이트
- `BytesWritten int64` - Transport에 쓴 총 바이트
- `LastActivityAt time.Time` - 마지막 데이터 수신/송신 시각
- `AvgProcessingLatency time.Duration` - 평균 메시지 처리 지연시간
- `RestartCount int` - 재시작 횟수

#### REQ-AGENT-001-16-05 (Ubiquitous) SharedInfo 구조체 정의

시스템은 **항상** `SharedInfo` 구조체를 제공해야 한다:

- `RefCount int32` - 현재 참조 카운트
- `Flows []string` - 참조 중인 플로우 ID 목록

#### REQ-AGENT-001-16-06 (Ubiquitous) ManagerSummary 구조체 정의

시스템은 **항상** `ManagerSummary` 구조체를 제공해야 한다:

- `TotalAgents int` - 전체 Agent 수
- `RunningAgents int` - Running 상태 Agent 수
- `PausedAgents int` - Paused 상태 Agent 수
- `StoppedAgents int` - Stopped 상태 Agent 수
- `ErrorAgents int` - Error 상태 Agent 수
- `HealthyAgents int` - Healthy 상태 Agent 수
- `UnhealthyAgents int` - Unhealthy 상태 Agent 수
- `TotalMessagesProcessed int64` - 전체 처리 메시지 합계
- `TotalErrors int64` - 전체 에러 합계
- `Agents []AgentInfo` - 모든 Agent의 AgentInfo 목록

#### REQ-AGENT-001-16-07 (Ubiquitous) Stats 원자적 카운터 관리

시스템은 **항상** `AgentStats`의 모든 수치 필드를 `atomic` 연산으로 업데이트하여 동시성 안전을 보장해야 한다.

#### REQ-AGENT-001-16-08 (Event-Driven) 메시지 처리 시 통계 자동 업데이트

**WHEN** Agent가 메시지를 수신/송신/에러 처리할 때, **THEN** 해당 `AgentStats` 카운터를 자동으로 증가시켜야 한다.

#### REQ-AGENT-001-16-09 (Event-Driven) Agent 재시작 시 통계 보존

**WHEN** Agent가 재시작되면, **THEN** `AgentStats`의 누적 통계(MessagesReceived, BytesRead 등)는 초기화하지 않고 유지해야 한다. `RestartCount`만 증가시켜야 한다.

#### REQ-AGENT-001-16-10 (Optional) Stats 리셋

**IF** 사용자가 원하면, **THEN** `ResetStats()` 메서드를 통해 AgentStats를 0으로 초기화할 수 있다.

---

### Module 17: Error Types (P0)

#### REQ-AGENT-001-17-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러 변수를 제공해야 한다:

- `ErrAgentNotFound` - Agent ID로 조회 실패
- `ErrAgentAlreadyExists` - 중복 Agent ID 생성 시도
- `ErrAgentNotRunning` - Running 상태가 아닌 Agent에 대한 작업 시도
- `ErrAgentAlreadyStopped` - 이미 Stopped 상태인 Agent에 대한 Stop 시도
- `ErrTransportNotAvailable` - 미등록 Transport 타입 요청
- `ErrTransportClosed` - 닫힌 Transport에 Read/Write 시도
- `ErrProtocolParseError` - 프로토콜 파싱 실패
- `ErrChecksumMismatch` - 체크섬 검증 실패
- `ErrHealthCheckFailed` - 헬스 체크 실패
- `ErrMaxRestartsExceeded` - 최대 재시작 횟수 초과
- `ErrInvalidConfig` - 유효하지 않은 설정
- `ErrConfigImmutable` - 불변 설정 변경 시도
- `ErrInvalidStateTransition` - 유효하지 않은 상태 전이
- `ErrFlowAlreadyReferenced` - 동일 플로우 중복 참조

#### REQ-AGENT-001-17-02 (Ubiquitous) errors.Is() 호환성

시스템은 **항상** 모든 sentinel 에러가 `errors.Is()` 함수로 비교 가능해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/agent/
  agent.go                  # Agent 인터페이스 + BaseAgent 구현
  agent_test.go             # Agent 단위 테스트
  manager.go                # Manager 인터페이스 + DefaultManager 구현
  manager_test.go
  registry.go               # Registry 인터페이스 + DefaultRegistry 구현
  registry_test.go
  health.go                 # HealthChecker 인터페이스 + 구현, HealthStatus, HealthState
  health_test.go
  shared.go                 # SharedRef 구조체 (참조 카운팅)
  shared_test.go
  info.go                   # AgentInfo, TransportInfo, ProtocolInfo, AgentStats, SharedInfo, ManagerSummary 구조체
  info_test.go
  config.go                 # AgentConfig 구조체 + 유효성 검증
  config_test.go
  system.go                 # SystemAgent 인터페이스 + SystemAgentType 열거
  system_test.go
  type_registry.go          # TypeRegistry 인터페이스 + 구현, AgentFactory
  type_registry_test.go
  errors.go                 # sentinel 에러 변수
  errors_test.go
  doc.go                    # 패키지 문서

  transport/
    transport.go            # Transport 인터페이스 + TransportConfig + TransportFactory
    transport_test.go
    serial.go               # SerialTransport 구현
    serial_test.go
    tcp.go                  # TCPTransport 구현
    tcp_test.go
    udp.go                  # UDPTransport 구현
    udp_test.go

  protocol/
    definition.go           # ProtocolDefinition, FieldDefinition, ConditionalPayload 구조체
    definition_test.go
    parser.go               # Parser 인터페이스 + DefaultParser 구현
    parser_test.go
    field.go                # FieldType 열거 + 필드 타입 변환 함수
    field_test.go
    checksum.go             # ChecksumDefinition, ChecksumAlgorithm, Calculate, Verify
    checksum_test.go
    loader.go               # Loader 인터페이스 + DefaultLoader (YAML/JSON)
    loader_test.go
```

### 4.2 주요 타입 시그니처

```go
package agent

import (
    "context"
    "sync/atomic"
    "time"

    "github.com/xtra/xflow/pkg/lifecycle"
    "github.com/xtra/xflow/pkg/message"
    "github.com/xtra/xflow/pkg/xferr"
)

// === Module 1: Agent Interface ===

type Agent interface {
    Init(config AgentConfig) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
    Health() HealthStatus
    Process(data []byte) ([]byte, error)
    Configure(config AgentConfig) error
    ID() string
    Name() string
    Type() string
    Info() AgentInfo
    Stats() AgentStats
}

// === Module 2: Manager ===

type Manager interface {
    Create(config AgentConfig) (Agent, error)
    Start(ctx context.Context, agentID string) error
    Stop(ctx context.Context, agentID string) error
    Restart(ctx context.Context, agentID string) error
    Delete(agentID string) error
    Get(agentID string) (Agent, error)
    List() []Agent
    Shutdown(ctx context.Context) error
    Summary() ManagerSummary
}

// === Module 3: Registry ===

type Registry interface {
    Register(agent Agent) error
    Unregister(agentID string) error
    Get(agentID string) (Agent, bool)
    GetByName(name string) (Agent, bool)
    GetByType(agentType string) []Agent
    List() []Agent
    Count() int
}

// === Module 4: Health Check ===

type HealthState string

const (
    HealthHealthy   HealthState = "healthy"
    HealthDegraded  HealthState = "degraded"
    HealthUnhealthy HealthState = "unhealthy"
)

type HealthStatus struct {
    Status              HealthState
    LastCheck           time.Time
    LastSuccess         time.Time
    ConsecutiveFailures int
    Message             string
}

type HealthChecker interface {
    Start(ctx context.Context) error
    Stop() error
    Check(agentID string) HealthStatus
    SetInterval(d time.Duration)
    OnUnhealthy(callback func(agentID string, status HealthStatus))
}

// === Module 5: Shared Agent ===

type SharedRef struct {
    agentID string
    refCount atomic.Int32
    // ...
}

// === Module 6: Transport Interface ===
// (transport/ 서브패키지)

// === Module 10: Protocol Definition Engine ===
// (protocol/ 서브패키지)

// === Module 13: Agent Configuration ===

type AgentConfig struct {
    ID                  string
    Name                string
    Type                string
    Transport           TransportConfig
    Protocol            *ProtocolDefinition
    ProtocolFile        string
    HealthCheckInterval time.Duration
    MaxRestarts         int
    StopOnZeroRef       bool
    BufferSize          int
    Metadata            map[string]string
}

func (c *AgentConfig) Validate() error

// === Module 14: System Agent ===

type SystemAgent interface {
    Agent
    IsSystem() bool
    RequiresTransport() bool
}

type SystemAgentType string

const (
    SystemAgentEvent  SystemAgentType = "event"
    SystemAgentLogger SystemAgentType = "logger"
    SystemAgentFile   SystemAgentType = "file"
    SystemAgentTimer  SystemAgentType = "timer"
    SystemAgentStore  SystemAgentType = "store"
)

// === Module 15: Agent Types Registry ===

type AgentFactory func(config AgentConfig) (Agent, error)

type TypeRegistry interface {
    RegisterType(agentType string, factory AgentFactory) error
    CreateAgent(agentType string, config AgentConfig) (Agent, error)
    ListTypes() []string
    HasType(agentType string) bool
}

// === Module 16: Agent Info & Stats ===

type AgentInfo struct {
    ID        string
    Name      string
    Type      string
    State     lifecycle.State
    Health    HealthStatus
    Transport TransportInfo
    Protocol  ProtocolInfo
    Config    AgentConfig
    Stats     AgentStats
    SharedInfo *SharedInfo
    StartedAt time.Time
    Uptime    time.Duration
    CreatedAt time.Time
}

type TransportInfo struct {
    Type        string
    Connected   bool
    RemoteAddr  string
    LocalAddr   string
    ConnectedAt time.Time
}

type ProtocolInfo struct {
    Name        string
    FieldCount  int
    HasChecksum bool
    SourceFile  string
}

type AgentStats struct {
    MessagesReceived    int64
    MessagesSent        int64
    MessagesErrored     int64
    BytesRead           int64
    BytesWritten        int64
    LastActivityAt      time.Time
    AvgProcessingLatency time.Duration
    RestartCount        int
}

type SharedInfo struct {
    RefCount int32
    Flows    []string
}

type ManagerSummary struct {
    TotalAgents            int
    RunningAgents          int
    PausedAgents           int
    StoppedAgents          int
    ErrorAgents            int
    HealthyAgents          int
    UnhealthyAgents        int
    TotalMessagesProcessed int64
    TotalErrors            int64
    Agents                 []AgentInfo
}
```

### 4.3 의존성 다이어그램

```
internal/agent/
    ├── 의존 ──► pkg/lifecycle/ (SPEC-LIFE-001)
    ├── 의존 ──► pkg/message/  (SPEC-MSG-001)
    ├── 의존 ──► pkg/xferr/    (SPEC-ERR-001)
    ├── 의존 ──► pkg/flow/     (SPEC-FLOW-001)
    │
    ├── 소비자 ◄── internal/node/ (SPEC-NODE-001)
    │                ├── Bridge Node: Agent 참조하여 메시지 교환
    │                └── Status Node: Agent 상태 이벤트 수신
    │
    ├── 소비자 ◄── internal/engine/ (SPEC-ENGINE-001)
    │                └── Engine: Manager를 통해 Agent 생명주기 관리
    │
    └── 소비자 ◄── internal/observe/ (SPEC-OBS-001)
                     ├── Agent 연결 상태 메트릭
                     ├── 프로토콜 파싱 결과 로깅
                     └── 헬스 체크 메트릭
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 우선순위 | product.md 근거 |
|------------|------|---------|-----------------|
| REQ-AGENT-001-01-01 ~ 06 | Agent Interface & BaseAgent | P0 | "Agent 인터페이스 정의 (Init, Start, Stop, Pause, Resume, Health, Process, Configure)" |
| REQ-AGENT-001-02-01 ~ 05 | Agent Manager | P0 | "Agent 생명주기 관리 (생성, 시작, 중지, 재시작, 삭제)" |
| REQ-AGENT-001-03-01 ~ 03 | Agent Registry | P0 | "실행 중인 Agent 목록 관리, 이름 및 타입 기반 조회" |
| REQ-AGENT-001-04-01 ~ 05 | Agent Health Check | P1 | "주기적 헬스 체크, 연결 상태 모니터링, 장애 감지 및 자동 재시작" |
| REQ-AGENT-001-05-01 ~ 05 | Shared Agent | P1 | "다중 플로우 참조 카운팅, 플로우 삭제 시에도 다른 플로우가 사용 중이면 Agent 유지" |
| REQ-AGENT-001-06-01 ~ 04 | Transport Interface | P0 | "Transport Interface (Open, Close, Read, Write), 팩토리 패턴" |
| REQ-AGENT-001-07-01 ~ 03 | Serial Transport | P1 | "Serial(RS-485/RS-232) 통신 구현, 보레이트/패리티/스톱비트 설정" |
| REQ-AGENT-001-08-01 ~ 03 | TCP Transport | P1 | "TCP 클라이언트/서버 구현, 연결 풀링, 타임아웃 관리" |
| REQ-AGENT-001-09-01 ~ 02 | UDP Transport | P2 | "UDP 통신 구현, 멀티캐스트 지원" |
| REQ-AGENT-001-10-01 ~ 08 | Protocol Definition Engine | P0 | "프로토콜 정의 구조체, 설정 기반 바이트스트림 파서/직렬화기" |
| REQ-AGENT-001-11-01 ~ 03 | Protocol Checksum | P0 | "체크섬/CRC 검증 알고리즘 (CRC-16, CRC-32, XOR, Modbus CRC)" |
| REQ-AGENT-001-12-01 ~ 03 | Protocol Loader | P1 | "YAML/JSON 프로토콜 정의 파일 로더 및 검증" |
| REQ-AGENT-001-13-01 ~ 03 | Agent Configuration | P1 | "AgentConfig 구조체 (Transport 설정, Protocol 설정, Health check 주기)" |
| REQ-AGENT-001-14-01 ~ 03 | System Agent Interface | P1 | "System Agent는 Transport/Protocol 불필요, 자동 활성화, 전역 접근" |
| REQ-AGENT-001-15-01 ~ 04 | Agent Types Registry | P1 | "Agent 타입별 팩토리 등록" |
| REQ-AGENT-001-16-01 ~ 10 | Agent Info & Stats | P0 | "에이전트 상태 정보, 관리 데이터 조회 (AgentInfo, AgentStats, TransportInfo, ProtocolInfo, SharedInfo, ManagerSummary)" |
| REQ-AGENT-001-17-01 ~ 02 | Error Types | P0 | "패키지 전용 sentinel 에러 정의" |
