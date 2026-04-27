# SPEC-SOCKET-001: 구현 계획

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SOCKET-001 |
| 개발 방법론 | Hybrid (DDD for existing patterns + TDD for new code) |
| 대상 에이전트 | expert-backend |

---

## 마일스톤

### Primary Goal: 핵심 인프라 및 TCP 에이전트

**목표**: 공통 인프라(설정, 프레이밍, 연결 관리)와 TCP 서버/클라이언트 에이전트를 구현한다.

**작업 항목**:

1. **공통 인프라 구현**
   - `internal/agent/socket/` 디렉토리 생성
   - `errors.go`: 센티널 에러 정의 (ErrMaxConnections, ErrConnectionBlocked, ErrReconnectFailed, ErrFramingError, ErrConnectionTimeout 등)
   - `config.go`: SocketConfig 구조체 및 Validate() 메서드 (공통 + TCP/UDP 전용 설정)
   - `framing.go`: Framer 인터페이스 및 4종 구현체 (RawFramer, NewlineFramer, LengthPrefixFramer, FixedSizeFramer)
   - `connection.go`: ConnectionInfo 구조체, ConnectionManager 인터페이스 및 기본 구현체

2. **TCP Server 에이전트 구현**
   - `tcp_server.go`: TCPServerAgent 구현
     - BaseAgent 임베딩
     - net.Listener 기반 수신 대기
     - 고루틴 기반 다중 클라이언트 동시 처리
     - ConnectionManager 통합 (접속 관리, 차단)
     - Framer 통합 (메시지 분리)
     - Pause/Resume 시 버퍼링 처리
     - StatefulAgent, TransportChecker, BufferInfoProvider, MessageReceiver 인터페이스 구현

3. **TCP Client 에이전트 구현**
   - `tcp_client.go`: TCPClientAgent 구현
     - BaseAgent 임베딩
     - net.Dial 기반 연결
     - 자동 재연결 (지수 백오프, max_retries, reconnect_interval)
     - connect_timeout 적용
     - Framer 통합
     - StatefulAgent, TransportChecker, BufferInfoProvider, MessageReceiver 인터페이스 구현

4. **에이전트 등록**
   - `register.go`: agent.Register()를 통한 팩토리 함수 등록

5. **단위 테스트**
   - config_test.go: 설정 파싱 및 유효성 검증 테스트
   - framing_test.go: 4종 프레이밍 방식 테스트 (경계값, 대용량, 불완전 데이터)
   - connection_test.go: 연결 관리, 차단/해제, 동시성 안전성 테스트
   - tcp_server_test.go: 서버 시작/중지, 다중 접속, 최대 접속 제한, 접속 차단, Pause/Resume
   - tcp_client_test.go: 연결/해제, 재연결, 타임아웃, 최대 재시도, 데이터 송수신

**완료 기준**: TCP 에이전트 2종이 독립 실행 가능하고, 85%+ 테스트 커버리지 달성

---

### Secondary Goal: UDP 에이전트

**목표**: UDP 서버/클라이언트 에이전트를 구현한다.

**작업 항목**:

1. **UDP Server 에이전트 구현**
   - `udp_server.go`: UDPServerAgent 구현
     - BaseAgent 임베딩
     - net.ListenUDP 기반 데이터그램 수신
     - 발신자 주소별 통계 관리
     - IP 기반 차단 기능
     - StatefulAgent, MessageReceiver, BufferInfoProvider 인터페이스 구현

2. **UDP Client 에이전트 구현**
   - `udp_client.go`: UDPClientAgent 구현
     - BaseAgent 임베딩
     - net.DialUDP 기반 데이터그램 송신
     - 응답 수신 지원
     - StatefulAgent, MessageReceiver, BufferInfoProvider 인터페이스 구현

3. **단위 테스트**
   - udp_server_test.go: 데이터그램 수신, IP 차단, 통계 관리
   - udp_client_test.go: 데이터그램 송신, 응답 수신

**완료 기준**: UDP 에이전트 2종이 독립 실행 가능하고, 85%+ 테스트 커버리지 달성

---

### Final Goal: 소켓 브릿지 노드 및 통합

**목표**: 소켓 브릿지 노드를 구현하고 플로우 엔진과 통합한다.

**작업 항목**:

1. **소켓 브릿지 어댑터 구현**
   - `internal/node/socket_bridge.go`: SocketBridgeAdapter 구현
     - BridgeAdapter 인터페이스 준수
     - 소켓 에이전트 타입별 메시지 변환
     - TCP 서버의 경우 특정 연결로 응답 라우팅 지원

2. **노드 레지스트리 등록**
   - `socket-input` (BridgeIn 모드): 에이전트 수신 데이터를 플로우로 전달
   - `socket-output` (BridgeOut 모드): 플로우 데이터를 에이전트로 송신
   - `socket-inout` (BridgeInOut 모드): 양방향 데이터 교환

3. **통합 테스트**
   - socket_bridge_test.go: 브릿지 어댑터 동작 검증
   - 에이전트-노드-플로우 통합 시나리오 테스트

**완료 기준**: 소켓 브릿지 노드 3종이 플로우 에디터에서 사용 가능하고, 에이전트와 정상 연동

---

### Optional Goal: 관찰성 및 고급 기능

**목표**: 메트릭, 로깅, API 통합을 완성한다.

**작업 항목**:

1. **Prometheus 메트릭 추가**
   - 활성 연결 수 (gauge)
   - 수신/송신 바이트 수 (counter)
   - 에러 수 (counter)
   - 재연결 시도 수 (counter, 클라이언트)

2. **API 핸들러 확장**
   - GET /api/v1/agents/{id}/connections: 활성 연결 목록 조회
   - POST /api/v1/agents/{id}/connections/block: 특정 주소 차단
   - DELETE /api/v1/agents/{id}/connections/block: 차단 해제

3. **웹 대시보드 연동**
   - 소켓 에이전트 설정 UI 컴포넌트
   - 연결 상태 모니터링 패널

**완료 기준**: 관찰성 메트릭이 Prometheus에 노출되고, API를 통한 연결 관리가 가능

---

## 기술 접근 방법

### 패턴 참조

- **MODBUS Agent 패턴**: 파일 구조, 에이전트 등록, 설정 파싱, 테스트 구조
- **Samsung NASA Agent 패턴**: Transport 추상화, 프로토콜 인코딩/디코딩
- **BridgeNode 패턴**: 에이전트-플로우 연결, BridgeAdapter 인터페이스

### 동시성 설계

- 각 TCP 클라이언트 연결은 독립 고루틴에서 처리
- ConnectionManager는 sync.RWMutex로 동시성 보호
- 수신 채널(recvCh)은 버퍼링된 Go 채널 사용
- 재연결 로직은 별도 고루틴에서 실행, context.Context로 취소 제어

### 에러 처리

- 네트워크 에러는 래핑하여 상위로 전파
- 프레이밍 에러는 에러 포트로 전달 (Catch 노드에서 수신 가능)
- 연결 끊김은 로그 기록 + 재연결 트리거 (클라이언트) 또는 정리 (서버)

### 테스트 전략

- net.Pipe()를 활용한 네트워크 목킹
- 실제 로컬 TCP/UDP 서버를 테스트 내에서 구동
- 테이블 기반 테스트로 다양한 설정 조합 검증
- 레이스 디텍터(`-race`) 필수 적용

---

## 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| TCP 스트림에서 메시지 경계 누락 | 데이터 파싱 오류 | Framer 인터페이스로 명확한 분리, 불완전 데이터 버퍼링 |
| 다중 접속 시 고루틴 누수 | 메모리 누수 | context.Context 기반 취소, sync.WaitGroup으로 고루틴 추적 |
| 재연결 폭풍 (thundering herd) | 서버 과부하 | 지수 백오프 + 지터(jitter) 적용 |
| 대용량 메시지로 인한 메모리 과다 사용 | OOM | max_message_size 설정으로 제한, 버퍼 풀링 |

---

## 생성/수정 파일 목록

### 신규 생성 파일

| 파일 경로 | 설명 |
|-----------|------|
| `internal/agent/socket/common.go` | 공통 타입, 상수 |
| `internal/agent/socket/errors.go` | 센티널 에러 정의 |
| `internal/agent/socket/config.go` | 설정 구조체 및 검증 |
| `internal/agent/socket/framing.go` | Framer 인터페이스 및 구현체 |
| `internal/agent/socket/connection.go` | ConnectionManager 인터페이스 및 구현 |
| `internal/agent/socket/tcp_server.go` | TCPServerAgent |
| `internal/agent/socket/tcp_client.go` | TCPClientAgent |
| `internal/agent/socket/udp_server.go` | UDPServerAgent |
| `internal/agent/socket/udp_client.go` | UDPClientAgent |
| `internal/agent/socket/register.go` | 에이전트 타입 등록 |
| `internal/agent/socket/common_test.go` | 공통 테스트 |
| `internal/agent/socket/config_test.go` | 설정 테스트 |
| `internal/agent/socket/framing_test.go` | 프레이밍 테스트 |
| `internal/agent/socket/connection_test.go` | 연결 관리 테스트 |
| `internal/agent/socket/tcp_server_test.go` | TCP 서버 테스트 |
| `internal/agent/socket/tcp_client_test.go` | TCP 클라이언트 테스트 |
| `internal/agent/socket/udp_server_test.go` | UDP 서버 테스트 |
| `internal/agent/socket/udp_client_test.go` | UDP 클라이언트 테스트 |
| `internal/node/socket_bridge.go` | 소켓 브릿지 어댑터 |
| `internal/node/socket_bridge_test.go` | 소켓 브릿지 테스트 |

### 수정 파일 (기존)

| 파일 경로 | 수정 내용 |
|-----------|----------|
| `internal/agent/register.go` 또는 에이전트 초기화 코드 | 소켓 에이전트 import 추가 |
| `internal/node/registry.go` 또는 노드 초기화 코드 | 소켓 브릿지 노드 타입 등록 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-01*
*작성: MoAI SPEC Builder (manager-spec)*
