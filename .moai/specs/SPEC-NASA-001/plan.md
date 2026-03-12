---
id: SPEC-NASA-001
type: plan
version: "1.3.0"
created: "2026-02-24"
updated: "2026-03-12"
author: xtra
---

# SPEC-NASA-001 구현 계획: Samsung NASA Agent 구현

## 1. 작업 분해

### Primary Goal: 에러, 타입, 설정, 프로토콜 기반 구조

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | 센티널 에러 정의 (12개) | `internal/agent/samsung/errors.go` | High |
| 2 | NASAConfig 설정 파싱 및 검증 | `internal/agent/samsung/config.go` | High |
| 3 | NASADevice, NASADeviceState 타입 정의 | `internal/agent/samsung/device.go` | High |
| 4 | NASAMessage, NASACommand 타입 정의 | `internal/agent/samsung/message.go` | High |
| 5 | NASAProtocol 인터페이스 및 기본 구현 | `internal/agent/samsung/protocol.go` | High |
| 6 | nasa.yaml 프로토콜 정의 파일 작성 | `internal/agent/samsung/nasa.yaml` | High |

### Secondary Goal: 트랜스포트 및 에이전트 코어

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 7 | NASATransport 인터페이스 + Serial/TCP 구현 | `internal/agent/samsung/transport.go` | High |
| 8 | NASAAgent 메인 구현 (Agent + MessageReceiver) | `internal/agent/samsung/agent.go` | High |
| 9 | RegisterSamsungNASATypes 타입 등록 | `internal/agent/samsung/register.go` | High |
| 10 | main.go에서 RegisterSamsungNASATypes 호출 추가 | `cmd/xflowd/main.go` | High |

### Final Goal: 테스트 작성

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 11 | 설정 파싱 테스트 | `internal/agent/samsung/samsung_test.go` | High |
| 12 | 프로토콜 인코딩/디코딩 테스트 | `internal/agent/samsung/samsung_test.go` | High |
| 13 | 트랜스포트 목(mock) 기반 테스트 | `internal/agent/samsung/samsung_test.go` | High |
| 14 | 에이전트 Process/ReceiveMessage 테스트 | `internal/agent/samsung/samsung_test.go` | High |
| 15 | 디바이스 관리 (온/오프라인, 상태 변경) 테스트 | `internal/agent/samsung/samsung_test.go` | High |
| 16 | 센티널 에러 errors.Is() 호환 테스트 | `internal/agent/samsung/samsung_test.go` | High |

### Optional Goal: 예제 및 문서

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 17 | 예제 에이전트 설정 YAML | `examples/agents/samsung-nasa.yaml` | Medium |
| 18 | 예제 플로우 설정 YAML (상태 모니터링) | `examples/flows/nasa-monitoring.yaml` | Medium |
| 19 | go.mod 의존성 추가 (go.bug.st/serial) | `go.mod`, `go.sum` | High |

---

## 2. 기술 접근 방식

### 2.1 아키텍처 개요

```
                    ┌──────────────────────────────┐
                    │        NASAAgent             │
                    │  (Agent + MessageReceiver)   │
                    ├───────────┬──────────────────┤
                    │           │                  │
                    │  NASATransport               │
                    │  (Serial / TCP)              │
                    │           │                  │
                    │  NASAProtocol                │
                    │  (Encode / Decode)           │
                    │           │                  │
                    │  DeviceManager               │
                    │  (map[byte]*NASADevice)      │
                    │           │                  │
                    │  msgCh ──────> Bridge Node   │
                    └───────────┴──────────────────┘
```

### 2.2 설정 파싱 전략 (Module 1, 2)

기존 MQTT, InfluxDB Agent와 동일한 패턴을 따른다:
- `AgentConfig.Transport.Options` (`map[string]any`)에서 NASA 전용 설정을 파싱
- `parseNASAConfig(opts map[string]any) (*NASAConfig, error)` 함수 구현
- 필수 필드 검증: `transport_type`, `device_addresses`
- 조건부 필수 검증: Serial 시 `serial_port`, TCP 시 `tcp_address`
- 기본값 적용: `baud_rate=9600`, `poll_interval=30s`, `offline_threshold=3`

### 2.3 트랜스포트 추상화 (Module 2)

`NASATransport` 인터페이스를 정의하고 Serial/TCP 두 가지 구현체를 제공한다:

**Serial 구현 (`NASASerialTransport`)**:
- `go.bug.st/serial` 패키지 사용
- RS-485 반이중(half-duplex) 통신 패턴: Send 후 일정 시간 내 Receive 대기
- 프레임 수신 시 바이트 단위 읽기 + SOF 탐지 + 길이 기반 프레임 완성

**TCP 구현 (`NASATCPTransport`)**:
- Go 표준 `net` 패키지 사용
- TCP-to-Serial 게이트웨이 연결
- 자동 재연결 (지수 백오프: 1s, 2s, 4s, ... 최대 reconnect_interval)
- 연결 풀링은 단일 연결로 충분 (RS-485 마스터-슬레이브 모델)

**팩토리 함수**:
```go
func NewNASATransport(config *NASAConfig) (NASATransport, error)
```

### 2.4 프로토콜 처리 (Module 3)

NASA 프로토콜의 패킷 구조를 `nasa.yaml`로 정의하고, 인코딩/디코딩 로직을 구현한다:

- **패킷 구조**: SOF(1) + Length(2) + SrcAddr(1) + DstAddr(1) + MsgType(1) + CmdCode(2) + Payload(N) + Checksum(1)
- **체크섬**: XOR 기반 또는 CRC-8 (프로토콜 스펙에 따라 결정)
- **프레임 수신**: 바이트 스트림에서 SOF 탐지 -> Length 읽기 -> 나머지 프레임 읽기 -> 체크섬 검증

Protocol Definition Engine(`internal/agent/protocol/`)과의 연동:
- 기본적으로 자체 인코더/디코더를 구현하되, Protocol Definition Engine이 지원하는 범위 내에서 `nasa.yaml`을 활용
- 가변 길이 페이로드, 조건부 필드 등 엔진에서 처리하기 어려운 부분은 `protocol.go`에서 직접 처리

### 2.5 디바이스 관리 (Module 4)

- `devices map[byte]*NASADevice`로 주소 기반 디바이스 관리
- `sync.RWMutex`로 동시성 보호 (폴링 고루틴과 Process 호출 동시 접근)
- 상태 변경 감지: 이전 상태와 비교하여 변경된 경우에만 메시지 발행
- 오프라인 판정: `OfflineThreshold` (기본 3) 연속 응답 실패 시

### 2.6 Bridge 연동 (Module 6)

기존 InfluxDB Agent와 동일한 패턴:
- `Process(data []byte)` 메서드로 제어 명령 수신 (Bridge -> Agent)
- `ReceiveMessage(ctx)` 메서드로 상태 데이터 전송 (Agent -> Bridge)
- JSON 기반 메시지 포맷으로 Bridge 노드와 통신
- `msgCh` 채널(버퍼 크기 설정 가능)로 비동기 메시지 전달

---

## 3. 의존성 분석

### 3.1 내부 의존성

| 패키지 | 용도 | 변경 필요 |
|--------|------|-----------|
| `internal/agent` | Agent, BaseAgent, AgentConfig, TypeRegistry | 변경 없음 (기존 인터페이스 사용) |
| `internal/agent/protocol` | Protocol Definition Engine (nasa.yaml 로드) | 변경 없음 |
| `pkg/lifecycle` | BaseLifecycle (상태 전이) | 변경 없음 |
| `cmd/xflowd/main.go` | RegisterSamsungNASATypes 호출 추가 | 1줄 추가 |

### 3.2 외부 의존성

| 패키지 | 버전 | 용도 | 설치 |
|--------|------|------|------|
| `go.bug.st/serial` | v1.6+ | RS-485 시리얼 포트 통신 | `go get go.bug.st/serial` |
| `github.com/stretchr/testify` | v1.9+ | 테스트 어설션 | 이미 사용 중 |

### 3.3 외부 시스템 의존성

| 시스템 | 조건 | 비고 |
|--------|------|------|
| RS-485 USB 어댑터 | Serial 모드 사용 시 | `/dev/ttyUSB0` 등 |
| TCP-to-Serial 게이트웨이 | TCP 모드 사용 시 | 포트 4196 등 |
| Samsung NASA 시스템 에어컨 | 실제 운영 시 | 실내기/실외기/컨트롤러 |

---

## 4. 리스크 분석

### 리스크 1: NASA 프로토콜 상세 사양 미확인

- **설명**: NASA 프로토콜의 정확한 바이트 구조, 명령 코드, 체크섬 알고리즘이 공식 문서화되어 있지 않을 수 있다
- **영향**: 프로토콜 구현의 정확성에 영향
- **대응**: 프로토콜 인터페이스를 먼저 설계하고, 구현체를 교체 가능하도록 설계. 실제 장비에서 패킷 캡처로 검증

### 리스크 2: RS-485 하드웨어 의존성

- **설명**: RS-485 시리얼 통신은 실제 하드웨어가 있어야 완전한 통합 테스트가 가능하다
- **영향**: CI/CD 환경에서 통합 테스트 제한
- **대응**: NASATransport 인터페이스를 목(mock)으로 대체하여 단위 테스트 가능하도록 설계. 통합 테스트는 별도 하드웨어 환경에서 수행

### 리스크 3: 프레임 수신 동기화

- **설명**: RS-485 바이트 스트림에서 프레임 경계를 식별하는 것은 노이즈나 데이터 손실 시 문제가 될 수 있다
- **영향**: 프로토콜 파싱 안정성
- **대응**: SOF 바이트 탐지 + 타임아웃 기반 프레임 리셋 메커니즘 구현. 불완전한 프레임 수신 시 버퍼 초기화 후 재동기화

### 리스크 4: 동시성 문제

- **설명**: 폴링 고루틴과 Process 메서드가 동시에 트랜스포트에 접근할 수 있다
- **영향**: 데이터 레이스, 패킷 충돌
- **대응**: 트랜스포트 레벨에서 `sync.Mutex`로 Send/Receive 직렬화. RS-485 반이중 통신의 특성상 요청-응답 쌍을 원자적으로 처리

---

## 5. 마일스톤 요약

| 마일스톤 | 산출물 | 우선도 |
|----------|--------|--------|
| M1: 기반 구조 | errors.go, config.go, device.go, message.go | High |
| M2: 프로토콜 | protocol.go, nasa.yaml | High |
| M3: 트랜스포트 | transport.go (Serial + TCP) | High |
| M4: 에이전트 코어 | agent.go, register.go | High |
| M5: 테스트 | samsung_test.go (85%+ 커버리지) | High |
| M6: 예제 | 에이전트/플로우 설정 YAML | Medium |

---

## 6. v1.2.0 — Transport 재연결 구현 계획

### 6.1 작업 분해

#### Primary Goal: 재연결 기반 구조

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R1 | NASAConfig에 `ReconnectInterval`, `MaxReconnectBackoff` 필드 추가 및 파싱 | `config.go` | REQ-NASA-001-01-09 | High |
| R2 | Transport `Available()` 강화 — Send()/Receive() I/O 에러 시 `open` 플래그 갱신 | `transport.go` | REQ-NASA-001-02-05 | High |
| R3 | TCP 트랜스포트에서 `ReconnectInterval`/`MaxReconnectAttempts` 제거 | `transport.go` | REQ-NASA-001-02-03 | High |
| R4 | NASAAgent에 `disconnectCh`, `reconnecting` 필드 추가 | `agent.go` | REQ-NASA-001-01-12 | High |

#### Secondary Goal: 재연결 루프 및 감지 로직

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R5 | `reconnectLoop()` 고루틴 구현 (지수 백오프, 로그 억제) | `agent.go` | REQ-NASA-001-01-09 | High |
| R6 | `receiveLoop` 연결 끊김 감지 — I/O 에러 시 disconnectCh 전송, reconnectLoop 시작 | `agent.go` | REQ-NASA-001-01-10 | High |
| R7 | `pollLoop` disconnectCh 처리 — 연결 끊김 시 폴링 중지 | `agent.go` | REQ-NASA-001-01-11 | High |
| R8 | `Start()` 수정 — transport.Open() 실패 시 reconnectLoop 시작, nil 반환 | `agent.go` | REQ-NASA-001-01-05 | High |

#### Final Goal: 이벤트 메시지 및 State() 확장

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R9 | `State()` 출력에 `transport_connected`, `reconnecting`, `reconnect_attempts` 추가 | `agent.go` | REQ-NASA-001-01-08 | High |
| R10 | 재연결 이벤트 메시지 전송 (`transport_disconnected`/`reconnecting`/`reconnected`) | `agent.go` | REQ-NASA-001-07-03 | High |

#### Test Goal: 테스트 작성

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| R11 | config 파싱 테스트 — ReconnectInterval, MaxReconnectBackoff | `config_test.go` | High |
| R12 | transport Available() 상태 갱신 테스트 — I/O 에러 vs 타임아웃 구분 | `transport_test.go` | High |
| R13 | reconnectLoop 단위 테스트 — 지수 백오프, 성공/실패/종료 | `agent_test.go` | High |
| R14 | receiveLoop 연결 끊김 감지 테스트 | `agent_test.go` | High |
| R15 | pollLoop disconnectCh 중지 테스트 | `agent_test.go` | High |
| R16 | Start() 연결 실패 시 재연결 루프 진입 테스트 | `agent_test.go` | High |
| R17 | State() 확장 필드 테스트 | `agent_test.go` | High |
| R18 | 재연결 이벤트 메시지 테스트 | `agent_test.go` | High |

### 6.2 변경 파일 목록

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/agent/samsung/config.go` | 수정 | ReconnectInterval, MaxReconnectBackoff 필드 추가 및 파싱 |
| `internal/agent/samsung/transport.go` | 수정 | Available() 강화, TCP에서 재연결 필드 제거, I/O 에러 시 open 플래그 갱신 |
| `internal/agent/samsung/agent.go` | 수정 | reconnectLoop, disconnectCh, reconnecting 필드, Start() 수정, State() 확장, 이벤트 메시지 |
| `internal/agent/samsung/config_test.go` | 수정 | ReconnectInterval/MaxReconnectBackoff 파싱 테스트 |
| `internal/agent/samsung/transport_test.go` | 수정 | Available() 상태 갱신 테스트 |
| `internal/agent/samsung/agent_test.go` | 수정 | reconnectLoop, receiveLoop, pollLoop, Start(), State(), 이벤트 테스트 |

### 6.3 구현 순서

권장 구현 순서 (의존성 기반):

1. **R1** config 파싱 → 다른 모든 작업의 기반
2. **R3** TCP 트랜스포트 재연결 필드 제거 → 충돌 방지
3. **R2** transport Available() 강화 → reconnectLoop의 전제 조건
4. **R4** NASAAgent 필드 추가 → reconnectLoop의 전제 조건
5. **R5** reconnectLoop 구현 → 핵심 로직
6. **R6** receiveLoop 연결 끊김 감지 → reconnectLoop 트리거
7. **R7** pollLoop disconnectCh 처리 → 연결 끊김 시 안전한 중지
8. **R8** Start() 수정 → 초기 연결 실패 시 graceful 처리
9. **R9** State() 출력 확장 → 모니터링 가시성
10. **R10** 이벤트 메시지 → 플로우 알림
11. **R11~R18** 테스트 작성

### 6.4 리스크 분석

#### 리스크 R1: 고루틴 생명주기 관리 복잡성

- **설명**: reconnectLoop, pollLoop, receiveLoop 세 고루틴 간의 시작/중지 조율이 복잡하다. 특히 reconnectLoop가 성공 후 pollLoop/receiveLoop를 다시 시작하고, receiveLoop가 에러 시 reconnectLoop를 트리거하는 순환 구조가 발생한다.
- **영향**: 고루틴 누수, 데드락, 중복 시작 가능성
- **대응**: `reconnecting` atomic.Bool로 중복 reconnectLoop 방지. `disconnectCh`는 매번 새로 생성하여 재사용 문제 방지. `stopCh`를 최상위 종료 시그널로 사용하여 모든 고루틴이 Stop() 시 확실히 종료되도록 보장.

#### 리스크 R2: 지수 백오프 타이밍 테스트

- **설명**: 실제 시간 대기가 포함된 지수 백오프 로직은 테스트가 느려질 수 있다
- **영향**: 테스트 실행 시간 증가
- **대응**: 테스트에서 짧은 ReconnectInterval(예: 10ms)과 MaxReconnectBackoff(예: 50ms) 사용. 타이머를 주입 가능하도록 설계 고려.

#### 리스크 R3: 타임아웃 vs 연결 끊김 구분

- **설명**: I/O 에러가 타임아웃인지 연결 끊김인지 정확히 구분해야 한다. 일부 환경에서는 연결 끊김이 타임아웃으로 보고될 수 있다.
- **영향**: 불필요한 재연결 시도 또는 연결 끊김 미감지
- **대응**: `net.Error` 인터페이스의 `Timeout()` 메서드로 1차 판별. 타임아웃이 아닌 에러에서 `transport.Available()`을 추가 확인하여 2중 검증.

---

---

## 7. v1.3.0 — NASA Nodes 구현 계획

### 7.1 작업 분해

#### Primary Goal: 기반 구조 (에러, 설정, 공통 유틸리티)

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R1 | NASA 노드 센티널 에러 4종 정의 | `internal/node/errors.go` | REQ-NASA-001-09-15 | High |
| R2 | NASANodeConfig 구조체 및 parseNASANodeConfig 구현 | `internal/node/nasa.go` | REQ-NASA-001-09-01 | High |
| R3 | callAgentProcess 공통 함수 (NASA 노드 공유) | `internal/node/nasa.go` | REQ-NASA-001-09-18 | High |

#### Secondary Goal: 노드 구현

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R4 | NASAStatusNode 구조체, 팩토리, Configure, Init | `internal/node/nasa.go` | REQ-NASA-001-09-02~04 | High |
| R5 | NASAStatusNode Process (get_state / get_all_states) | `internal/node/nasa.go` | REQ-NASA-001-09-05 | High |
| R6 | NASAStatusNode SourceNode 폴링 (SourceCh, 폴링 고루틴) | `internal/node/nasa.go` | REQ-NASA-001-09-06 | High |
| R7 | NASAControlNode 구조체, 팩토리, Configure, Init | `internal/node/nasa.go` | REQ-NASA-001-09-07~09 | High |
| R8 | NASAControlNode Process — 직접 명령 형식 | `internal/node/nasa.go` | REQ-NASA-001-09-10 | High |
| R9 | NASAControlNode Process — 간소화 형식 (set_multiple 자동 변환) | `internal/node/nasa.go` | REQ-NASA-001-09-11 | High |
| R10 | NASANode 복합 노드 구조체, 팩토리, Configure, Init | `internal/node/nasa.go` | REQ-NASA-001-09-12~13 | High |
| R11 | NASANode Process — 자동 감지 분기 (status vs control) | `internal/node/nasa.go` | REQ-NASA-001-09-14 | High |

#### Final Goal: 등록, 프론트엔드, 공통 기능

| 순서 | 작업 | 파일 | 요구사항 | 우선도 |
|------|------|------|----------|--------|
| R12 | Shutdown 구현 (3종 공통) | `internal/node/nasa.go` | REQ-NASA-001-09-19 | High |
| R13 | 런타임 메시지 오버라이드 (applyNASAOverrides) | `internal/node/nasa.go` | REQ-NASA-001-09-20 | High |
| R14 | 컴파일 타임 인터페이스 검증 (Node, SourceNode) | `internal/node/nasa.go` | REQ-NASA-001-09-21 | High |
| R15 | 노드 레지스트리에 3종 빌트인 등록 (12 -> 15) | `internal/node/registry.go` | REQ-NASA-001-09-16 | High |
| R16 | 프론트엔드 nodeSchemas.ts에 3종 스키마 추가 | `web/src/config/nodeSchemas.ts` | REQ-NASA-001-09-22 | Medium |
| R17 | 프론트엔드 nodeTypeMeta.ts에 3종 메타데이터 추가 | `web/src/config/nodeTypeMeta.ts` | REQ-NASA-001-09-23 | Medium |

#### Test Goal: 테스트 작성

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| R18 | NASAStatusNode 단위 테스트 (Configure, Init, Process, SourceNode) | `internal/node/nasa_test.go` | High |
| R19 | NASAControlNode 단위 테스트 (직접 명령, 간소화 형식, 오버라이드) | `internal/node/nasa_test.go` | High |
| R20 | NASANode 복합 단위 테스트 (자동 감지, 상태/제어 분기) | `internal/node/nasa_test.go` | High |
| R21 | 레지스트리 등록 테스트 (15개 빌트인 확인) | `internal/node/registry_test.go` | High |

### 7.2 변경 파일 목록

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/node/nasa.go` | **신규** | NASANodeConfig, NASAStatusNode, NASAControlNode, NASANode, callAgentProcess, 팩토리 함수 |
| `internal/node/nasa_test.go` | **신규** | NASA 노드 3종 단위 테스트 |
| `internal/node/errors.go` | 수정 | NASA 노드 센티널 에러 4종 추가 |
| `internal/node/registry.go` | 수정 | registerBuiltins()에 3종 추가 (nasa-status, nasa-control, nasa), 주석 12->15 업데이트 |
| `internal/node/registry_test.go` | 수정 | 빌트인 수 검증 12->15 업데이트 |
| `web/src/config/nodeSchemas.ts` | 수정 | NASA 노드 3종 스키마 추가 |
| `web/src/config/nodeTypeMeta.ts` | 수정 | NASA 노드 3종 메타데이터 추가 |

### 7.3 구현 순서

권장 구현 순서 (의존성 기반):

1. **R1** 센티널 에러 정의 → 다른 모든 작업의 기반
2. **R2** NASANodeConfig 설정 구조체 → 노드 구현의 전제 조건
3. **R3** callAgentProcess 공통 함수 → 노드 Process의 전제 조건
4. **R4** NASAStatusNode 기본 구조 → 핵심 노드
5. **R5** NASAStatusNode Process → 상태 조회 로직
6. **R6** NASAStatusNode SourceNode → 폴링 기능
7. **R7~R9** NASAControlNode 전체 → 제어 노드
8. **R10~R11** NASANode 복합 → 자동 감지 노드
9. **R12~R14** 공통 기능 (Shutdown, 오버라이드, 인터페이스 검증)
10. **R15** 레지스트리 등록 → 빌트인 통합
11. **R16~R17** 프론트엔드 → UI 통합
12. **R18~R21** 테스트 작성

### 7.4 기술 접근 방식

#### 7.4.1 ModbusNode 패턴 준수

NASA 노드 구현은 ModbusNode와 동일한 아키텍처 패턴을 따른다:

**팩토리 함수**: `NewNASAStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` — `base.config["_agent_resolver"]`에서 AgentResolver 추출

**Init 파이프라인**: `AgentResolver.ResolveAgent(ctx, ref)` -> `AgentTransport` -> `AgentAccessor.UnderlyingAgent()` -> `*samsung.NASAAgent` 타입 체크

**Process 패턴**: 런타임 오버라이드 적용 -> JSON 명령 구성 -> `callAgentProcess(ctx, cmdBytes)` -> 응답 파싱 -> 출력 메시지 구성

**Import**: `"github.com/xtra/xflow/internal/agent/samsung"` — 타입 체크에 `*samsung.NASAAgent` 사용

#### 7.4.2 NASAControlNode 간소화 형식

간소화 형식은 사용자 편의를 위한 설계이다. 제어 키를 자동 감지하여 `set_multiple` 명령으로 변환한다:

**입력 (간소화)**:
```json
{"device_id": "living-room", "power": true, "mode": "cool", "target_temp": 24.0}
```

**변환 결과 (직접 명령)**:
```json
{"command": "set_multiple", "device_id": "living-room", "params": {"power": true, "mode": "cool", "target_temp": 24.0}}
```

**제어 키 목록**: `power`, `mode`, `temperature`, `target_temp`, `fan_speed`

#### 7.4.3 NASANode 자동 감지

NASANode의 Process는 페이로드 키 검사로 상태/제어를 자동 구분한다:

1. `command` 키 존재 → 제어 (직접 명령 형식)
2. 제어 키 존재 (`power`, `mode`, `temperature`, `target_temp`, `fan_speed`) → 제어 (간소화 형식)
3. 그 외 → 상태 조회

### 7.5 리스크 분석

#### 리스크 R1: samsung 패키지 import 순환 의존성

- **설명**: `internal/node/nasa.go`에서 `internal/agent/samsung` 패키지를 import하면 순환 의존성 발생 가능
- **영향**: 컴파일 실패
- **대응**: ModbusNode가 이미 `internal/agent/modbus`를 import하는 패턴이 확립되어 있으므로 동일하게 적용. `node` -> `agent/samsung` 단방향 의존성만 존재하므로 순환 없음

#### 리스크 R2: SourceNode 폴링 고루틴 리소스 관리

- **설명**: NASAStatusNode와 NASANode의 폴링 고루틴이 Shutdown 시 정확히 종료되지 않으면 고루틴 누수 발생
- **영향**: 메모리 누수, 불필요한 Agent Process 호출
- **대응**: `stopCh` 채널 기반 종료 + `context.WithCancel` 이중 보호. `go test -race`로 검증

#### 리스크 R3: 간소화 형식의 키 충돌

- **설명**: NASANode에서 `power`, `mode` 등의 키가 다른 용도로 사용될 수 있다
- **영향**: 의도하지 않은 제어 명령 발생
- **대응**: 제어 키 목록을 명확히 정의하고, 사용자에게 NASANode 대신 NASAStatusNode/NASAControlNode 분리 사용을 권장

---

*SPEC-NASA-001 Plan v1.3.0*
*작성자: xtra*
*날짜: 2026-03-12*
