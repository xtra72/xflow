---
id: SPEC-NASA-001
type: plan
version: "0.1.0"
created: "2026-02-24"
updated: "2026-02-24"
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

*SPEC-NASA-001 Plan v0.1.0*
*작성자: xtra*
*날짜: 2026-02-24*
