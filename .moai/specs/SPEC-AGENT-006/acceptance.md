---
id: SPEC-AGENT-006
version: "1.0.0"
status: draft
created: "2026-04-10"
author: xtra
priority: high
related_spec: SPEC-AGENT-006
---

# SPEC-AGENT-006: Acceptance Criteria - Transport Agent Configuration 분리 및 Hot-Reload

본 문서는 SPEC-AGENT-006 의 인수 기준을 Given-When-Then 형식으로 정의한다. 각 시나리오는 자동화된 Go 테스트로 검증 가능해야 한다.

---

## 1. M1: Connection/Operation 분류 도메인 모델

### AC1.1: ConnectionConfigChecker 인터페이스 정의

**Given** `internal/agent` 패키지
**When** `ConnectionConfigChecker` 인터페이스를 확인하면
**Then**:
- `IsConnectionChange(oldOpts, newOpts map[string]any) bool` 메서드 시그니처를 가져야 한다
- 타입 단언으로 검사 가능해야 한다

### AC1.2: OptionsDiffer 헬퍼 - 빈 맵

**Given** 빈 oldOpts 와 빈 newOpts, 임의의 keys 리스트
**When** `OptionsDiffer(oldOpts, newOpts, keys)` 를 호출하면
**Then** `false` 를 반환해야 한다

### AC1.3: OptionsDiffer 헬퍼 - 같은 값

**Given** oldOpts `{"host": "a", "port": 1}`, newOpts `{"host": "a", "port": 1}`, keys `["host", "port"]`
**When** 호출하면
**Then** `false` 를 반환해야 한다

### AC1.4: OptionsDiffer 헬퍼 - 다른 값

**Given** oldOpts `{"host": "a"}`, newOpts `{"host": "b"}`, keys `["host"]`
**When** 호출하면
**Then** `true` 를 반환해야 한다

### AC1.5: OptionsDiffer 헬퍼 - 한쪽에만 존재

**Given** oldOpts `{}`, newOpts `{"host": "a"}`, keys `["host"]`
**When** 호출하면
**Then** `true` 를 반환해야 한다

### AC1.6: TCPClientConnectionKeys 정확성

**Given** `TCPClientConnectionKeys()` 함수
**When** 호출하면
**Then** 결과 슬라이스는 정확히 `host, port, framing, delimiter, fixed_size, buffer_size` 를 포함해야 한다 (R4.1 의 표와 일치)

### AC1.7: TCPServerConnectionKeys 정확성

**When** `TCPServerConnectionKeys()` 를 호출하면
**Then** 결과는 정확히 `host, port, framing, delimiter, fixed_size, buffer_size` 이어야 한다

### AC1.8: UDPClientConnectionKeys 정확성

**When** `UDPClientConnectionKeys()` 를 호출하면
**Then** 결과는 정확히 `host, port` 이어야 한다

### AC1.9: UDPServerConnectionKeys 정확성

**When** `UDPServerConnectionKeys()` 를 호출하면
**Then** 결과는 정확히 `host, port` 이어야 한다

### AC1.10: SerialConnectionKeys 정확성

**When** `SerialConnectionKeys()` 를 호출하면
**Then** 결과는 `port, baud_rate, data_bits, stop_bits, parity, framing` 과 framing 관련 구조적 파라미터를 포함해야 한다

---

## 2. M2: Configure 재파싱 동작

### AC2.1: TCP Client - Configure 후 a.config.Host 갱신

**Given** TCP Client 에이전트가 `host=localhost, port=10001` 로 생성된 상태
**When** `Configure` 를 `host=localhost, port=10002` 로 호출하면
**Then**:
- `Configure` 는 nil error 를 반환해야 한다
- 에이전트의 내부 `a.config.Port` 는 `10002` 이어야 한다
- 에이전트의 내부 `a.config.Host` 는 `localhost` 이어야 한다

### AC2.2: TCP Client - Configure 실패 시 상태 보존 (R2.3, R7.6, R7.7)

**Given** TCP Client 에이전트가 `host=localhost, port=10001` 로 running 상태
**When** `Configure` 를 `port=-1` (invalid) 로 호출하면
**Then**:
- `Configure` 는 non-nil error 를 반환해야 한다
- 에이전트의 `a.config.Port` 는 여전히 `10001` 이어야 한다
- 에이전트의 `a.agentConfig` 는 변경되지 않아야 한다

### AC2.3: TCP Server - Configure 후 a.config 갱신

**Given** TCP Server 에이전트가 `port=20001` 로 생성된 상태
**When** `Configure` 를 `port=20002` 로 호출하면
**Then** 에이전트의 내부 `a.config.Port` 는 `20002` 이어야 한다

### AC2.4: UDP Client - Configure 후 a.config 갱신

**Given** UDP Client 에이전트가 `host=a, port=30001` 로 생성된 상태
**When** `Configure` 를 `host=b, port=30002` 로 호출하면
**Then** 에이전트의 내부 `a.config.Host = b`, `a.config.Port = 30002` 이어야 한다

### AC2.5: UDP Server - Configure 후 a.config 갱신

**Given** UDP Server 에이전트가 `port=40001` 로 생성된 상태
**When** `Configure` 를 `port=40002` 로 호출하면
**Then** 에이전트의 내부 `a.config.Port = 40002` 이어야 한다

### AC2.6: Serial - Configure 후 a.config 갱신

**Given** Serial 에이전트가 `baud_rate=9600` 으로 생성된 상태
**When** `Configure` 를 `baud_rate=19200` 으로 호출하면
**Then** 에이전트의 내부 `a.config.BaudRate = 19200` 이어야 한다

### AC2.7: Configure 와 readLoop 동시 실행 - race 없음 (R2.8, NFR6)

**Given** 5개 Transport 에이전트 각각을 running 상태로 시작
**When** `go test -race` 환경에서 Configure 를 1000회 반복 호출하는 동시에 readLoop/reconnectLoop 가 `a.config` 를 읽으면
**Then** data race 가 감지되지 않아야 한다

### AC2.8: IsConnectionChange - host 변경 탐지 (TCP Client)

**Given** TCP Client 에이전트, oldOpts `{"host": "a", "port": 1}`, newOpts `{"host": "b", "port": 1}`
**When** `agent.IsConnectionChange(oldOpts, newOpts)` 를 호출하면
**Then** `true` 를 반환해야 한다

### AC2.9: IsConnectionChange - reconnect_interval 변경은 false (TCP Client)

**Given** TCP Client, oldOpts `{"host": "a", "reconnect_interval": "1s"}`, newOpts `{"host": "a", "reconnect_interval": "2s"}`
**When** 호출하면
**Then** `false` 를 반환해야 한다

### AC2.10: IsConnectionChange - 동일 옵션은 false

**Given** 임의의 에이전트, oldOpts 와 newOpts 가 deep-equal
**When** 호출하면
**Then** `false` 를 반환해야 한다

---

## 3. M3: ConfigureAgent 서비스 어댑터 동작

### AC3.1: TCP Client host 변경 - Restart 트리거 (결함 B 회귀)

**Given**:
- 테스트 TCP 서버 S1 (port 10001), S2 (port 10002) 모두 listening
- 에이전트 A 가 `host=localhost, port=10001` 로 생성되고 Start 되어 S1 에 연결됨

**When** `ConfigureAgent` 를 `port=10002` 로 호출하면
**Then**:
- `ConfigureAgent` 는 nil error 를 반환해야 한다
- 매니저의 `Restart` 가 호출되어야 한다
- 에이전트 A 는 S2 에 연결되어야 한다 (S2 의 connection counter 증가)
- S1 의 connection 은 종료되어야 한다
- 로그에 "connection config changed, restart triggered" 가 기록되어야 한다

### AC3.2: TCP Client reconnect_interval 변경 - Restart 없음

**Given** 에이전트 A 가 `reconnect_interval=5s` 로 running 상태
**When** `ConfigureAgent` 를 `reconnect_interval=1s` 로 호출하면
**Then**:
- `ConfigureAgent` 는 nil error 를 반환해야 한다
- 매니저의 `Restart` 가 호출되지 않아야 한다 (Restart 카운터 변화 없음)
- 에이전트 A 의 내부 `a.config.ReconnectInterval` 은 `1s` 이어야 한다
- 로그에 "operation config changed, hot-reloaded" 가 기록되어야 한다

### AC3.3: TCP Client max_retries 변경 - Restart 없음

**Given** 에이전트 A 가 `max_retries=10` 로 running
**When** `ConfigureAgent` 를 `max_retries=3` 로 호출하면
**Then**:
- Restart 호출되지 않음
- `a.config.MaxRetries = 3` 갱신

### AC3.4: TCP Server port 변경 - Restart 트리거

**Given** 에이전트 B 가 `port=20001` 로 listening
**When** `ConfigureAgent` 를 `port=20002` 로 호출하면
**Then**:
- Restart 호출됨
- 에이전트 B 는 port 20002 에서 listening 해야 한다
- 20001 의 bind 는 해제되어야 한다

### AC3.5: UDP Client host 변경 - Restart 트리거

**Given** UDP Client C 가 `host=a, port=30001` 로 running
**When** `ConfigureAgent` 를 `host=b` 로 호출하면
**Then** Restart 호출됨

### AC3.6: UDP Server port 변경 - Restart 트리거

**Given** UDP Server D 가 `port=40001` 로 running
**When** `ConfigureAgent` 를 `port=40002` 로 호출하면
**Then** Restart 호출됨

### AC3.7: Serial baud_rate 변경 - Restart 트리거

**Given** Serial 에이전트 E 가 `baud_rate=9600` 으로 running
**When** `ConfigureAgent` 를 `baud_rate=19200` 으로 호출하면
**Then** Restart 호출됨

### AC3.8: Serial gap_timeout 변경 - Restart 없음 (R5.9)

**Given** Serial 에이전트 E 가 `gap_timeout=100ms` 로 running
**When** `ConfigureAgent` 를 `gap_timeout=50ms` 로 호출하면
**Then**:
- Restart 호출되지 않음
- `a.config.GapTimeout = 50ms` 갱신
- 다음 read 주기부터 새 값 반영

### AC3.9: ConfigureAgent 영속화 실패 - 롤백 (R3.7, NFR12)

**Given** 에이전트 A 가 running, 저장소가 일시적으로 Put 실패
**When** `ConfigureAgent` 를 호출하면
**Then**:
- `ConfigureAgent` 는 error 를 반환해야 한다
- 에이전트 A 의 in-memory `a.config` 는 호출 이전 상태로 롤백되어야 한다
- 에이전트 A 의 `a.agentConfig` 도 이전 상태여야 한다

### AC3.10: ConfigureAgent Restart 실패 - 롤백

**Given** 에이전트 A, Restart 가 실패하도록 매니저 mock 설정
**When** `ConfigureAgent` 를 connection 변경으로 호출하면
**Then**:
- error 반환
- in-memory 롤백

### AC3.11: Non-transport 에이전트 - 기존 동작 보존 (R6.5, 위험 10.4)

**Given** Modbus 또는 MQTT 에이전트 (ConnectionConfigChecker 미구현) 가 running
**When** `ConfigureAgent` 를 호출하면
**Then**:
- 기존 동작과 동일하게 처리되어야 한다
- characterization test 가 새 코드에서도 동일 결과를 얻어야 한다

### AC3.12: 하드코딩된 transportKeys 제거 검증

**Given** `internal/api/service/agent_adapter.go` 의 소스 코드
**When** grep 검색하면
**Then** `tcp_host`, `tcp_port` 문자열 리터럴이 파일에 존재하지 않아야 한다 (결함 B 제거 증거)

---

## 4. M4: TCP Client 분류 구체 검증

### AC4.1: Connection keys - host

**Given** TCP Client, oldOpts `{"host":"a"}`, newOpts `{"host":"b"}`
**When** `IsConnectionChange` 호출
**Then** true

### AC4.2: Connection keys - port

**Given** TCP Client, oldOpts `{"port":1}`, newOpts `{"port":2}`
**When** 호출
**Then** true

### AC4.3: Connection keys - framing

**Given** TCP Client, oldOpts `{"framing":"delimited"}`, newOpts `{"framing":"fixed"}`
**When** 호출
**Then** true

### AC4.4: Connection keys - buffer_size

**Given** TCP Client, oldOpts `{"buffer_size":1024}`, newOpts `{"buffer_size":2048}`
**When** 호출
**Then** true

### AC4.5: Operation keys - max_message_size

**Given** TCP Client, oldOpts `{"max_message_size":1024}`, newOpts `{"max_message_size":2048}`
**When** 호출
**Then** false

### AC4.6: Operation keys - reconnect_interval

**Given** TCP Client, oldOpts `{"reconnect_interval":"1s"}`, newOpts `{"reconnect_interval":"5s"}`
**When** 호출
**Then** false

### AC4.7: Operation keys - max_retries

**Given** TCP Client, oldOpts `{"max_retries":3}`, newOpts `{"max_retries":10}`
**When** 호출
**Then** false

### AC4.8: Operation keys - connect_timeout

**Given** TCP Client, oldOpts `{"connect_timeout":"1s"}`, newOpts `{"connect_timeout":"5s"}`
**When** 호출
**Then** false

### AC4.9: reconnect_interval 진행 중 loop 에 반영 (R4.3)

**Given** TCP Client A 가 연결 불가 서버를 대상으로 reconnectLoop 실행 중 (`reconnect_interval=5s`, `max_retries=100`)
**When** `Configure` 로 `reconnect_interval=100ms` 변경
**Then** 다음 백오프 간격부터 새 값 (100ms) 이 사용됨 (timing 측정으로 검증)

### AC4.10: max_retries 다음 attempt 에 반영 (R4.3)

**Given** TCP Client A 가 `max_retries=100` 으로 reconnectLoop 실행 중 (현재 attempt 3)
**When** `Configure` 로 `max_retries=5` 변경
**Then** attempt 6 이상에서 reconnectLoop 종료

### AC4.11: connect_timeout 다음 시도에 반영 (R4.4)

**Given** TCP Client A 가 `connect_timeout=10s` 로 설정
**When** `Configure` 로 `connect_timeout=1s` 변경 후 재연결 시도
**Then** 다음 `connect` 호출의 timeout 은 1s 이어야 한다

---

## 5. M5: 나머지 4개 에이전트 분류 검증

### AC5.1: TCP Server connection keys

**When** `IsConnectionChange` 를 host/port/framing/delimiter/fixed_size/buffer_size 각각에 대해 호출
**Then** 모두 true 반환

### AC5.2: TCP Server operation keys

**When** `IsConnectionChange` 를 max_message_size/max_connections 각각에 대해 호출
**Then** 모두 false 반환

### AC5.3: UDP Client connection keys

**When** host/port 변경 시 `IsConnectionChange` 호출
**Then** true

### AC5.4: UDP Client operation keys

**When** buffer_size 변경 시 호출
**Then** false

### AC5.5: UDP Server connection keys

**When** host/port 변경 시 호출
**Then** true

### AC5.6: UDP Server operation keys

**When** buffer_size 변경 시 호출
**Then** false

### AC5.7: Serial connection keys

**When** port/baud_rate/data_bits/stop_bits/parity/framing/stx/etx/length_offset 각각 변경 시 호출
**Then** true

### AC5.8: Serial operation keys

**When** read_timeout/idle_timeout/gap_timeout/buffer_size/max_message_size/delimiter/fixed_size 각각 변경 시 호출
**Then** false

### AC5.9: Serial gap_timeout 핫 리로드 반영 (R5.9)

**Given** Serial 에이전트 E 가 `gap_timeout=100ms` 로 running, mock serial port
**When** `Configure` 로 `gap_timeout=50ms` 변경
**Then** 다음 read 주기의 inter-byte gap 판정이 50ms 를 사용해야 한다

### AC5.10: TCP Server max_connections 핫 리로드 (R5.10)

**Given** TCP Server 가 `max_connections=10`, 기존 연결 5개
**When** `Configure` 로 `max_connections=20` 변경
**Then**:
- Restart 없음
- 이후 11 번째 연결도 accept 됨
- 기존 5개 연결은 유지됨

---

## 6. M6: 하위 호환성

### AC6.1: 기존 저장소 데이터 로드

**Given** 본 SPEC 적용 이전에 저장된 `Transport.Options` 가 `{host, port, framing, ...}` 키를 사용하는 에이전트 데이터
**When** 데몬을 새 바이너리로 시작하면
**Then**:
- 모든 에이전트가 정상 로드되어야 한다
- 자동 시작이 이전과 동일하게 동작해야 한다
- 저장소 파일은 변경되지 않아야 한다

### AC6.2: API 응답 구조 불변

**Given** `GET /agents/{id}` API
**When** 본 SPEC 적용 전후 응답 구조를 비교하면
**Then** 필드 집합이 동일해야 한다 (새 필드 추가/제거 없음)

### AC6.3: Web UI schema 불변

**Given** `web/src/config/agentSchemas.ts` 및 관련 타입 파일
**When** 본 SPEC 의 변경 diff 를 확인하면
**Then** Web UI 의 schema 파일은 변경되지 않아야 한다

### AC6.4: 매니저 Restart 경로 동작 보존 (NFR3)

**Given** 임의의 에이전트 A 가 running
**When** 매니저의 `Restart(ctx, id)` 를 직접 호출하면
**Then** 본 SPEC 이전과 동일하게 인스턴스가 파괴 후 재생성되어 정상 동작해야 한다 (characterization test)

### AC6.5: 데몬 재시작 경로 동작 보존 (NFR4)

**Given** 저장소에 10개 에이전트 등록
**When** 데몬 정지 후 재시작
**Then** `restoreAgents` 가 정상 동작하여 10개 에이전트 모두 복원되고 enabled 인 것은 자동 시작되어야 한다

### AC6.6: transportKeys 하드코딩 완전 제거

**Given** `internal/api/service/agent_adapter.go`
**When** 코드를 검사하면
**Then** `transportKeys` 변수가 존재하지 않거나, 존재하더라도 하드코딩된 tcp_host/tcp_port 를 포함하지 않아야 한다

---

## 7. M7: 결함 A/B 회귀 방지

### AC7.1: 결함 A 회귀 - TCP Client 단일 인스턴스 host 변경 (R7.1 핵심)

**Given**:
- TCP 서버 S1 (port 10001) 과 S2 (port 10002) 모두 기동
- TCP Client 에이전트 A 가 `host=localhost, port=10001` 로 생성, Start, S1 에 연결

**When**:
1. 에이전트 A 를 Stop
2. `Configure(host=localhost, port=10002)` 호출
3. 에이전트 A 를 다시 Start

**Then**:
- 에이전트 A 는 S2 (port 10002) 에 연결되어야 한다
- S1 의 연결 시도는 발생하지 않아야 한다
- **이 테스트는 결함 A 가 있는 이전 코드에서 FAIL 해야 한다** (reproduction-first 원칙)

### AC7.2: 결함 A 회귀 - TCP Server port 변경

**Given** TCP Server 가 port 20001 에 bind, Stop 상태
**When** `Configure(port=20002)` 후 Start
**Then** port 20002 에서 listening (20001 에는 아님)

### AC7.3: 결함 A 회귀 - UDP Client host 변경

**Given** UDP Client 가 `host=localhost, port=30001` 로 구성, Stop 상태
**When** `Configure(port=30002)` 후 Start → 메시지 송신
**Then** port 30002 의 UDP 리스너가 메시지를 수신

### AC7.4: 결함 A 회귀 - Serial baud_rate 변경

**Given** Mock serial port, 에이전트가 baud_rate=9600 으로 구성, Stop 상태
**When** `Configure(baud_rate=19200)` 후 Start
**Then** mock serial port 의 Open 호출 기록에 baud_rate=19200 이 확인되어야 한다

### AC7.5: 결함 B 회귀 - ConfigureAgent host 변경 → 자동 Restart (R7.4)

**Given**:
- TCP Client 에이전트 A 가 S1 에 연결되어 running 상태
- 매니저의 Restart 호출 카운터 = 0

**When** `ConfigureAgent(A, host=S2)` 를 호출하면

**Then**:
- 매니저의 Restart 호출 카운터 = 1
- 에이전트 A 가 S2 에 연결되어 있음
- **이 테스트는 결함 B (transportKeys 화이트리스트 버그) 가 있는 이전 코드에서 FAIL 해야 한다**

### AC7.6: operation 변경 시 Restart 없음 (R7.4)

**Given** TCP Client A running, Restart 카운터 = 0
**When** `ConfigureAgent(A, reconnect_interval=...)` 호출
**Then** Restart 카운터 = 0 (변화 없음)

### AC7.7: 5개 에이전트 모두 회귀 테스트 적용 (R7.5)

**Given** 본 SPEC 의 구현
**When** `go test ./internal/agent/... -run TestRegression` 실행
**Then** TCP Client, TCP Server, UDP Client, UDP Server, Serial 각각에 대한 regression 테스트가 존재하고 모두 통과해야 한다

### AC7.8: 잘못된 옵션 - 저장소 변경 없음 (R7.6)

**Given** 에이전트 A 가 `host=a, port=10001` 로 running, 저장소에 동일 상태
**When** `ConfigureAgent(A, port=-1)` 호출
**Then**:
- 응답은 4xx error
- 에이전트 A 의 a.config 는 변경되지 않음
- 저장소에서 로드한 A 의 설정도 이전 상태 유지

### AC7.9: Configure Validate 실패 - 완전 롤백 (R7.7)

**Given** 에이전트 A, 유효한 설정으로 running
**When** `Configure` 에 Validate 를 실패시키는 옵션 전달
**Then**:
- `Configure` 는 error 반환
- `a.config` 변경 없음
- `a.agentConfig` 변경 없음

---

## 8. 통합 시나리오 (End-to-End)

### Integration Scenario 1: Web UI 시나리오 - TCP Client host 변경

**Given**:
- 테스트 TCP 서버 S1 (port 10001), S2 (port 10002) 기동
- 에이전트 A 가 Web UI 로 생성: host=localhost, port=10001
- A 는 S1 에 연결된 running 상태

**When**:
1. Web UI 또는 `PUT /agents/A/config` 로 port=10002 전송

**Then**:
- 응답 200 OK
- 매니저가 자동으로 Restart 실행
- 1~2초 이내에 A 는 S2 에 연결됨
- S1 의 연결 카운터는 감소 (연결 해제 확인)
- 로그에 "connection config changed" 관찰

### Integration Scenario 2: Operation 설정 핫 리로드

**Given** 에이전트 A 가 `reconnect_interval=5s, max_retries=10` 로 running
**When**:
1. `PUT /agents/A/config` 로 `reconnect_interval=1s, max_retries=3` 전송
**Then**:
- 응답 200 OK
- 매니저 Restart 카운터 변화 없음
- `GET /agents/A` 응답의 Transport.Options 는 새 값 반영
- 다음 재연결 시도 시 새 backoff 적용 확인

### Integration Scenario 3: 데몬 재시작 경로 보존

**Given** 3개 에이전트 (TCP Client, UDP Server, Serial) 모두 running
**When**:
1. 데몬 SIGTERM 또는 graceful stop
2. 새 바이너리로 데몬 재시작
**Then**:
- 3개 에이전트 모두 저장된 설정으로 복원
- 자동 시작이 이전과 동일하게 동작
- 본 SPEC 적용 전후 동작 차이 없음

### Integration Scenario 4: 잘못된 설정 거부 및 롤백

**Given** TCP Client A running, 저장소 정상 상태
**When**:
1. `PUT /agents/A/config` 로 `port=-1` 전송
**Then**:
- 응답 400 또는 422
- A 는 여전히 기존 포트로 running
- 저장소 재로드 시 이전 설정 유지

### Integration Scenario 5: Serial gap_timeout 핫 리로드

**Given** Mock serial 환경에서 Serial 에이전트 E 가 `gap_timeout=100ms` 로 running
**When** `PUT /agents/E/config` 로 `gap_timeout=50ms` 전송
**Then**:
- 응답 200 OK
- Restart 없음
- 다음 read 주기부터 50ms 기준 gap 판정 적용

### Integration Scenario 6: 5개 에이전트 종합 회귀

**Given** 5개 에이전트 모두 서로 다른 옵션으로 running
**When**:
1. 각 에이전트에 대해 connection 키 하나씩 변경
2. 각 에이전트에 대해 operation 키 하나씩 변경
**Then**:
- connection 변경 시 모두 Restart
- operation 변경 시 모두 no-op
- 모두 새 설정으로 정상 동작

---

## 9. 에러 시나리오

### Error 1: Configure - parsing 실패

**Given** 에이전트, 파싱 불가능한 옵션 (예: `port: "not-a-number"`)
**When** `Configure` 호출
**Then**:
- error 반환
- 에이전트 상태 변경 없음
- ERROR 로그 기록

### Error 2: ConfigureAgent - Restart 실패

**Given** 에이전트 A, 매니저 Restart 가 실패하도록 mock
**When** connection 변경으로 `ConfigureAgent` 호출
**Then**:
- error 반환
- 에이전트 A 의 in-memory 상태가 이전으로 롤백
- 저장소 영속화 실행되지 않음

### Error 3: ConfigureAgent - 영속화 실패

**Given** 에이전트 A, 저장소 Put 실패하도록 mock
**When** operation 변경으로 `ConfigureAgent` 호출
**Then**:
- error 반환
- in-memory 롤백 (`Configure` 로 이전 설정 재적용)

### Error 4: 존재하지 않는 에이전트

**Given** 에이전트 ID `UNKNOWN` 미존재
**When** `ConfigureAgent(UNKNOWN, ...)` 호출
**Then** 404 Not Found 또는 에이전트 not found error

---

## 10. 성능 / 비기능 검증

### Perf 1: Configure 응답 시간 (NFR15)

**Given** 100 개의 키를 가진 TCP Client 에이전트
**When** operation 키 하나만 변경하는 `ConfigureAgent` 호출
**Then** 응답 시간은 기존 Configure 대비 10% 이내 증가

### Perf 2: IsConnectionChange 선형 복잡도 (NFR16)

**Given** 50 개의 옵션 키를 가진 에이전트
**When** `IsConnectionChange` 호출
**Then** 실행 시간은 O(N) 이어야 한다 (N = connection keys 수)

### Reliability 1: 동시 Configure 호출

**Given** 같은 에이전트에 대해 2개 goroutine 이 동시에 다른 옵션으로 Configure 호출
**When** 1000회 반복
**Then**:
- 최종 상태는 두 호출 중 하나의 결과로 일관되게 결정
- 데드락 없음
- data race 없음

### Observability 1: 로그 검증

**Given** 5개 에이전트에 대한 다양한 Configure 호출
**When** 로그 분석
**Then**:
- Connection 변경: "connection config changed, restart triggered" INFO + agent_id
- Operation 변경: "operation config changed, hot-reloaded" INFO + agent_id
- Validate 실패: ERROR 로그 + 실패 사유
- 영속화 실패: ERROR 로그 + 롤백 정보

### Race 1: go test -race 통과

**Given** 본 SPEC 의 전체 변경 범위
**When** `go test -race ./...` 실행
**Then** 어떠한 data race 도 감지되지 않아야 한다

---

## 11. 검증 도구 및 방법

| 검증 영역 | 도구 | 방법 |
|----------|------|------|
| Go 단위 테스트 | `go test` | `_test.go` 파일 (table-driven) |
| Go 통합 테스트 | `go test` | 실제 store + manager + adapter + real agents |
| Race detection | `go test -race` | 동시성 검증 (필수) |
| 커버리지 | `go test -cover` | 신규 코드 ≥ 85% |
| Lint | `golangci-lint run` | 코드 품질 |
| Vet | `go vet ./...` | 정적 분석 |
| Format | `gofmt -l .` | 스타일 일관성 |
| Web 검증 | `tsc --noEmit` | Web 코드 변경 없음 확인 |
| 수동 smoke test | curl / Web UI | Personal mode operator |

---

## 12. Definition of Done (인수 완료 기준)

본 SPEC 의 인수가 완료되었다고 판정하는 기준:

- [ ] 모든 AC1.x ~ AC7.x 시나리오 통과
- [ ] Integration Scenario 1~6 통과
- [ ] Error Scenario 1~4 통과
- [ ] Perf 1, 2 통과
- [ ] Reliability 1 통과
- [ ] Observability 1 통과
- [ ] Race 1 (`go test -race`) 통과
- [ ] 신규 코드 커버리지 ≥ 85%
- [ ] `go vet ./...` 통과
- [ ] `golangci-lint run` 통과
- [ ] `gofmt -l .` 출력 없음
- [ ] 5개 에이전트 모두 회귀 테스트 추가
- [ ] 결함 A 와 B 각각의 reproduction-first 테스트가 구현 전 FAIL, 구현 후 PASS 임을 확인
- [ ] 기존 매니저 Restart 경로 동작 보존 (characterization test)
- [ ] 기존 데몬 재시작 경로 동작 보존
- [ ] Web UI 코드 변경 없음 확인
- [ ] 저장소 스키마 변경 없음 확인
- [ ] 수동 smoke test (TCP Client host 변경) 통과
- [ ] CHANGELOG.md 업데이트
