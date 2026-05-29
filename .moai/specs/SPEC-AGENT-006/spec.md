---
id: SPEC-AGENT-006
version: "1.1.0"
status: draft
created: "2026-04-10"
updated: "2026-05-14"
author: xtra
priority: high
tags: [agent, transport, configuration, hot-reload]
related_spec: SPEC-AGENT-005, SPEC-SERIAL-001, SPEC-SOCKET-001, SPEC-ENGINE-001
---

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-10 | 1.0.0 | 초기 SPEC 작성 (draft) |
| 2026-05-14 | 1.1.0 | xagent04 실배포 검증 hotfix 반영 — 본 SPEC 의 M3 가 의존하는 매니저 `Restart` 경로의 신뢰성 결함을 사전 수정. (1) **Manager.Stop / Manager.Restart 멱등성** — 이미 `Stopped` 상태인 에이전트에 Stop/Restart 호출 시 no-op 처리 (커밋 `a783c16`). (2) **Serial 재시작 생명주기** — `stopCh` 재설정 + bounded `Stop()`(5초) + `Error` 상태 회복 경로 (커밋 `d0651aa`, 상세는 SPEC-SERIAL-001 v2.2.0). 가정 A5(`TypeRegistry` 재생성 경로 정상 동작)·NFR3(매니저 Restart 동작 보존)을 보강하는 신규 요구사항 M8 추가. 본 SPEC 의 EARS 요구사항(M1~M7) 자체는 변경 없음. |

# SPEC-AGENT-006: Transport Agent Configuration - Connection/Operation 분리 및 Hot-Reload

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-AGENT-006 |
| 제목 | Transport Agent 설정의 Connection/Operation 분리 및 Hot-Reload 지원 |
| 버전 | 1.0.0 |
| 상태 | draft |
| 작성일 | 2026-04-10 |
| 작성자 | xtra |
| 우선순위 | high |
| 관련 SPEC | SPEC-AGENT-005 (Enable/Disable), SPEC-SERIAL-001 (Serial), SPEC-SOCKET-001 (Socket) |
| 도메인 | Agent Framework / Transport Configuration |

---

## 1. Overview (개요)

### 1.1 배경

현재 xflow 의 Transport 에이전트 (TCP Client/Server, UDP Client/Server, Serial) 는 운영 중 설정 변경 시 다음과 같은 결함을 보인다:

- **결함 A (Configure 불완전)**: Web UI 에서 에이전트 설정을 변경한 후 사용자가 수동으로 Stop → Start 를 수행해도 새 설정이 적용되지 않는다. 연결은 여전히 이전 주소로 이뤄진다. 반면 데몬을 재시작하거나 매니저의 Restart 경로로 접근할 때는 정상 동작한다.
- **결함 B (Restart 트리거 누락)**: 호스트나 포트 같은 연결 설정이 변경되어도 서비스 어댑터가 매니저의 Restart 를 호출하지 않는다. 결과적으로 기존 인스턴스가 이전 설정으로 계속 동작한다.
- **설정 분류 부재**: 어떤 설정이 "재연결이 필요한 connection 설정" 이고 어떤 설정이 "핫 리로드 가능한 operation 설정" 인지에 대한 명확한 구분이 없다.

### 1.2 근본 원인 분석

**결함 A - Configure 메서드가 파싱된 config 를 갱신하지 않음**

`internal/agent/socket/tcp_client.go:404-415`, `tcp_server.go:331-339`, `udp_client.go:237-245`, `udp_server.go:282-290`, `internal/agent/serial/agent.go:383-393` 의 5개 에이전트는 모두 동일한 결함 패턴을 가진다:

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error {
    if err := config.Validate(); err != nil { ... }
    a.mu.Lock()
    a.agentConfig = config   // 원본 wrapper 만 저장
    a.mu.Unlock()
    return nil
    // 버그: a.config (TCPClientConfig) 는 재파싱되지 않음
}
```

에이전트의 실제 운영 필드 (`a.config.Host`, `a.config.Port`, `a.config.ConnectTimeout`, `a.config.ReconnectInterval`, `a.config.MaxRetries` 등) 는 `NewTCPClientAgent()` 시점에 `ParseTCPClientConfig()` 로 생성된 파싱 구조체에서 파생된다. `Configure()` 가 재파싱하지 않으므로, 이후의 모든 동작 (connect, reconnect, readLoop) 은 stale 값을 사용한다.

tcp_client.go 에서 `a.config` 를 읽는 지점:

- `tcp_client.go:133` (`serverAddr`): `a.config.Host`, `a.config.Port`
- `tcp_client.go:139` (`connect`): `a.config.ConnectTimeout`
- `tcp_client.go:257-259` (`reconnectLoop`): `a.config.MaxRetries`
- `tcp_client.go:269` (`calculateBackoff`): `a.config.ReconnectInterval`

매니저의 `Restart` 경로 (`internal/agent/manager.go:189-256`) 는 인스턴스를 파괴하고 `TypeRegistry` 로 새 인스턴스를 생성하므로 `ParseTCPClientConfig` 가 재실행되어 정상 동작한다. 그러나 단일 인스턴스에서의 Stop → Start 경로는 같은 stale `a.config` 를 사용하므로 깨진다.

**결함 B - needsRestart 가 잘못된 키 이름 사용**

`internal/api/service/agent_adapter.go:285-300`:

```
var transportKeys = []string{
    "transport_type", "port", "serial_port", "baud_rate", "data_bits", "stop_bits", "parity",
    "tcp_host", "tcp_port",  // 버그: socket agent 는 "host", "port" 키 사용
}
```

Socket 에이전트 (`internal/agent/socket/config.go`) 는 options 맵에서 `host`, `port`, `framing`, `delimiter`, `fixed_size`, `buffer_size`, `max_message_size`, `reconnect_interval`, `max_retries`, `connect_timeout`, `max_connections` 키를 파싱한다. 화이트리스트의 `tcp_host`/`tcp_port` 는 존재하지 않는 키이므로, TCP client 의 host 변경은 `needsRestart = true` 를 트리거하지 못한다.

**결함 A 와 B 의 결합**: 사용자가 설정을 변경하면 `ConfigureAgent` 가 `ag.Configure()` 를 호출한다 (결함 A: `a.config` 미갱신). `needsRestart` 는 false 를 반환한다 (결함 B). 매니저 `Restart` 가 호출되지 않는다. 사용자가 수동으로 Stop 후 Start 를 누른다. `connect()` 는 stale `a.config` 를 사용하여 이전 주소로 연결된다.

### 1.3 제안 기능

1. 각 Transport 에이전트의 `Configure` 메서드가 입력된 새 옵션을 **재파싱**하여 internal config 구조체를 갱신한다 (결함 A 수정).
2. 각 에이전트가 자신의 **connection key set** 을 명시적으로 노출하여, 서비스 어댑터가 화이트리스트 하드코딩 대신 에이전트에 위임하도록 한다 (결함 B 수정).
3. Connection 설정 (재연결/재바인드/포트 재개방 필요) 과 Operation 설정 (즉시 핫 리로드 가능) 을 명확히 분류하고 문서화한다.
4. Operation 설정 변경은 동일 인스턴스에서 즉시 반영되며, Connection 설정 변경은 매니저 `Restart` 를 자동으로 트리거한다.

### 1.4 핵심 원칙

1. **Source of Truth 명확화**: `a.config` (파싱된 구조체) 가 에이전트 운영의 단일 source of truth 이며, `Configure` 호출 후 반드시 최신 상태를 반영해야 한다.
2. **분류 책임 위임**: 어떤 키가 connection 이고 어떤 키가 operation 인지 판단하는 책임은 에이전트 자체가 가진다. 서비스 어댑터는 하드코딩된 화이트리스트를 사용하지 않는다.
3. **하위 호환성**: `Transport.Options` 맵의 키 이름 (`host`, `port`, `framing`, ...) 은 변경하지 않는다. 저장소 스키마, API 응답 구조, Web UI 스키마도 변경하지 않는다.
4. **원자성**: Configure 호출 중 실패 시 in-memory 상태와 저장소 상태는 원래대로 롤백되어야 한다.
5. **동작 보존**: 기존에 정상 동작하던 경로 (데몬 재시작, 매니저 Restart) 의 동작은 변경되지 않는다.

---

## 2. Environment (환경)

### 2.1 현재 아키텍처

- **Transport 에이전트**:
  - `internal/agent/socket/tcp_client.go`, `tcp_server.go`, `udp_client.go`, `udp_server.go`
  - `internal/agent/serial/agent.go`
  - 각 에이전트는 `BaseAgent` 를 임베드하고 자신만의 파싱된 config 구조체 (`TCPClientConfig` 등) 를 보유한다.
- **Config 파싱**:
  - `internal/agent/socket/config.go`: `ParseTCPClientConfig`, `ParseTCPServerConfig`, `ParseUDPClientConfig`, `ParseUDPServerConfig`
  - `internal/agent/serial/config.go`: `ParseSerialConfig`
- **매니저**:
  - `internal/agent/manager.go:189-256` (`Restart`): 인스턴스 파괴 후 `TypeRegistry` 로 재생성. 이 경로는 정상 동작.
- **서비스 어댑터**:
  - `internal/api/service/agent_adapter.go:285-300` (`needsRestart`): 하드코딩된 `transportKeys` 화이트리스트로 Restart 필요성 판단.
  - `internal/api/service/agent_adapter.go:302-348` (`ConfigureAgent`): `ag.Configure()` → 조건부 `m.manager.Restart()` → 저장소 영속화.
- **HTTP 핸들러**:
  - `internal/api/handler/agent.go`: `PUT /agents/{id}/config`.

### 2.2 영향 범위

**수정 대상**

- Core 계층 (5개 에이전트):
  - `internal/agent/socket/tcp_client.go`
  - `internal/agent/socket/tcp_server.go`
  - `internal/agent/socket/udp_client.go`
  - `internal/agent/socket/udp_server.go`
  - `internal/agent/serial/agent.go`
- Config 파싱 (분류 노출):
  - `internal/agent/socket/config.go`
  - `internal/agent/serial/config.go`
- 에이전트 인터페이스 또는 헬퍼:
  - `internal/agent/agent.go` 또는 `internal/agent/config.go` (ConnectionConfigChecker 인터페이스 또는 유사 메커니즘)
- 서비스 어댑터:
  - `internal/api/service/agent_adapter.go` (`needsRestart` 재설계)

**수정 없음 (Out of Scope)**

- 에이전트 Options 의 키 이름 (`host`, `port`, `framing`, ...)
- 저장소 스키마
- API 응답 구조
- Web UI 스키마 (`web/src/config/agentSchemas.ts` 등)
- Modbus, Modbus Server, LG, Samsung NASA, File, System, HTTP Receiver, MQTT 에이전트

---

## 3. Assumptions (가정)

1. 사용자는 Web UI 나 API 로 에이전트의 설정을 변경할 때, 변경 결과가 즉시 (또는 깨끗한 Restart 후) 적용되기를 기대한다.
2. Connection 설정 변경은 기존 연결을 끊고 새 연결을 맺어야 하므로 본질적으로 짧은 서비스 중단을 수반한다.
3. Operation 설정 변경은 현재 연결을 유지한 채 반영될 수 있다.
4. 에이전트는 자신의 파싱된 config 구조체를 뮤텍스로 보호하고 있거나 원자적 교체가 가능한 상태이다.
5. `TypeRegistry` 를 통한 재생성 경로는 깨지지 않았으며, 본 SPEC 의 변경 이후에도 동일하게 동작해야 한다.
6. 저장소는 JSON BLOB 이므로 `Transport.Options` 의 키 집합이 변하지 않는 이상 마이그레이션이 필요 없다.
7. 동시 다중 클라이언트의 Configure 호출은 기존 매니저 락으로 충분히 직렬화된다.

---

## 4. Requirements (요구사항 - EARS Format)

### M1: Connection vs Operation 설정 분리 - 도메인 모델

**R1.1 (Ubiquitous)**: 시스템은 항상 각 Transport 에이전트에 대해 connection 설정 키 집합과 operation 설정 키 집합을 명확히 정의하여 문서화해야 한다.

**R1.2 (Ubiquitous)**: 시스템은 항상 connection 설정을 "변경 시 기존 연결/바인드/포트를 파괴하고 재생성이 필요한 설정" 으로, operation 설정을 "변경 시 동일 인스턴스에서 다음 동작부터 반영되는 설정" 으로 정의해야 한다.

**R1.3 (Ubiquitous)**: 시스템은 항상 에이전트 자체가 자신의 connection key set 을 외부에 노출하는 공개 API (메서드 또는 인터페이스) 를 제공해야 한다.

**R1.4 (Unwanted)**: 시스템은 에이전트의 분류 책임을 외부 서비스 어댑터에 하드코딩해서는 안 된다.

**R1.5 (Unwanted)**: 본 SPEC 의 구현은 `Transport.Options` 의 키 이름을 변경해서는 안 된다.

---

### M2: Configure 메서드 동작 명세

**R2.1 (Ubiquitous)**: 시스템은 항상 5개 Transport 에이전트 (TCP client, TCP server, UDP client, UDP server, Serial) 의 `Configure` 메서드가 입력된 새 옵션을 재파싱하여 내부 파싱 config 구조체를 갱신하도록 해야 한다.

**R2.2 (Event-Driven)**: WHEN `Configure` 가 호출되면, THEN 시스템은 새 옵션을 기존 파싱 함수 (`ParseTCPClientConfig` 등) 로 재파싱해야 한다.

**R2.3 (State-Driven)**: IF 재파싱 결과가 유효하지 않으면 (Validate 실패) THEN 시스템은 에이전트의 기존 파싱 config 를 변경하지 않고 에러를 반환해야 한다.

**R2.4 (Event-Driven)**: WHEN `Configure` 가 성공하면, THEN 시스템은 `agentConfig` 필드와 파싱된 config 필드 (`a.config`) 를 모두 원자적으로 갱신해야 한다.

**R2.5 (Ubiquitous)**: 시스템은 항상 에이전트가 자신의 `a.config` 필드를 읽을 때 뮤텍스로 보호하거나, 원자적으로 교체된 스냅샷을 읽도록 보장해야 한다.

**R2.6 (Event-Driven)**: WHEN operation 설정만 변경된 상태에서 `Configure` 가 성공하면, THEN 동일 인스턴스는 다음 동작 (reconnect 시도, read 호출, backoff 계산 등) 부터 새 값을 반영해야 한다.

**R2.7 (Event-Driven)**: WHEN connection 설정이 변경된 경우, THEN `Configure` 메서드 자체는 연결을 직접 끊지 않으며, 호출자 (서비스 어댑터) 가 Restart 를 트리거할 수 있도록 connection 변경 여부를 알릴 수단을 제공해야 한다.

**R2.8 (Unwanted)**: `Configure` 는 readLoop 또는 reconnectLoop 와 race condition 을 유발해서는 안 된다.

---

### M3: ConfigureAgent 서비스 어댑터 동작 명세

**R3.1 (Ubiquitous)**: `ConfigureAgent` 는 항상 에이전트의 `Configure` 를 먼저 호출한 후, connection 변경 여부를 판단하여 필요 시 매니저의 `Restart` 를 호출해야 한다.

**R3.2 (Unwanted)**: 시스템은 `agent_adapter.go` 의 `needsRestart` 가 사용하던 하드코딩된 `transportKeys` 화이트리스트 방식을 유지해서는 안 된다.

**R3.3 (Ubiquitous)**: 시스템은 항상 connection 변경 여부 판단을 에이전트 자체에 위임해야 한다 (예: 에이전트의 공개 API 호출, 또는 에이전트가 구현한 인터페이스 타입 단언).

**R3.4 (Event-Driven)**: WHEN connection 설정이 변경된 것으로 판단되면, THEN `ConfigureAgent` 는 매니저의 `Restart` 를 호출해야 한다.

**R3.5 (Event-Driven)**: WHEN operation 설정만 변경된 것으로 판단되면, THEN `ConfigureAgent` 는 `Restart` 를 호출하지 않고 동일 인스턴스를 유지해야 한다.

**R3.6 (Ubiquitous)**: `ConfigureAgent` 는 항상 Configure / Restart 의 성공 여부와 무관하게 저장소 영속화를 적절한 시점에 수행해야 한다 (성공 시 영속화, 실패 시 in-memory 롤백).

**R3.7 (State-Driven)**: IF 저장소 영속화가 실패하면 THEN 시스템은 in-memory 상태를 Configure 호출 이전 상태로 롤백해야 한다 (NFR 준수).

**R3.8 (Event-Driven)**: WHEN `ConfigureAgent` 가 connection 변경을 감지할 때마다, THEN 시스템은 INFO 로그로 "connection config changed, restart triggered" 를 에이전트 ID 와 함께 기록해야 한다.

**R3.9 (Event-Driven)**: WHEN `ConfigureAgent` 가 operation 변경만 감지할 때마다, THEN 시스템은 INFO 로그로 "operation config changed, hot-reloaded" 를 에이전트 ID 와 함께 기록해야 한다.

---

### M4: TCP Client 분류 (구체)

**R4.1 (Ubiquitous)**: 시스템은 항상 TCP Client 의 다음 옵션 키를 connection 설정으로 분류해야 한다:

| 키 | 사유 |
|----|------|
| `host` | 대상 서버 주소 변경 → 기존 소켓 파괴 필요 |
| `port` | 대상 포트 변경 → 기존 소켓 파괴 필요 |
| `framing` | 프레이밍 알고리즘 변경 → readLoop 재초기화 필요 |
| `delimiter` | 프레이밍의 구조적 파라미터 |
| `fixed_size` | 프레이밍의 구조적 파라미터 |
| `buffer_size` | bufio reader/writer 크기 → 재생성 필요 |

**R4.2 (Ubiquitous)**: 시스템은 항상 TCP Client 의 다음 옵션 키를 operation 설정으로 분류해야 한다:

| 키 | 반영 시점 |
|----|----------|
| `max_message_size` | 다음 read 호출부터 |
| `reconnect_interval` | 다음 백오프 계산부터 |
| `max_retries` | 다음 attempt 비교부터 |
| `connect_timeout` | 다음 reconnect 시도부터 |

**R4.3 (Event-Driven)**: WHEN TCP Client 의 `reconnect_interval` 또는 `max_retries` 가 `Configure` 로 변경되면, THEN 진행 중인 `reconnectLoop` 의 다음 반복부터 새 값을 사용해야 한다.

**R4.4 (Event-Driven)**: WHEN TCP Client 의 `connect_timeout` 이 `Configure` 로 변경되면, THEN 다음 `connect` 호출의 타임아웃으로 새 값이 사용되어야 한다.

---

### M5: TCP Server / UDP Client / UDP Server / Serial 분류

**R5.1 (Ubiquitous)**: 시스템은 항상 TCP Server 의 다음 옵션 키를 connection 설정으로 분류해야 한다: `host`, `port`, `framing`, `delimiter`, `fixed_size`, `buffer_size`.

**R5.2 (Ubiquitous)**: 시스템은 항상 TCP Server 의 다음 옵션 키를 operation 설정으로 분류해야 한다: `max_message_size`, `max_connections`.

**R5.3 (Ubiquitous)**: 시스템은 항상 UDP Client 의 다음 옵션 키를 connection 설정으로 분류해야 한다: `host`, `port`.

**R5.4 (Ubiquitous)**: 시스템은 항상 UDP Client 의 다음 옵션 키를 operation 설정으로 분류해야 한다: `buffer_size`.

**R5.5 (Ubiquitous)**: 시스템은 항상 UDP Server 의 다음 옵션 키를 connection 설정으로 분류해야 한다: `host`, `port`.

**R5.6 (Ubiquitous)**: 시스템은 항상 UDP Server 의 다음 옵션 키를 operation 설정으로 분류해야 한다: `buffer_size`.

**R5.7 (Ubiquitous)**: 시스템은 항상 Serial 의 다음 옵션 키를 connection 설정으로 분류해야 한다: `port`, `baud_rate`, `data_bits`, `stop_bits`, `parity`, `framing`, 그리고 프레이밍 관련 구조적 파라미터 (`stx`, `etx`, `length_offset`, `length_size`, `length_includes_header` 등).

**R5.8 (Ubiquitous)**: 시스템은 항상 Serial 의 다음 옵션 키를 operation 설정으로 분류해야 한다: `read_timeout`, `idle_timeout`, `gap_timeout`, `buffer_size`, `max_message_size`, `delimiter`, `fixed_size`.

**R5.9 (Event-Driven)**: WHEN Serial 의 `gap_timeout` 이 `Configure` 로 변경되면, THEN 에이전트는 다음 read 주기부터 새 값을 반영해야 한다. 필요 시 내부적으로 `SetReadTimeout` 을 재호출할 수 있다.

**R5.10 (Optional)**: WHERE TCP Server 의 `max_connections` 를 핫 리로드할 때 이미 초과한 기존 연결을 강제로 끊지 않는다. 새 값은 이후의 accept 판정부터 적용된다.

---

### M6: 하위 호환성

**R6.1 (Ubiquitous)**: 시스템은 항상 기존 `Transport.Options` 의 키 이름 (`host`, `port`, `framing`, `delimiter`, `fixed_size`, `buffer_size`, `max_message_size`, `reconnect_interval`, `max_retries`, `connect_timeout`, `max_connections`, Serial 관련 키들) 을 변경 없이 그대로 사용해야 한다.

**R6.2 (Ubiquitous)**: 시스템은 항상 본 SPEC 의 적용 이후에도 기존 저장소 데이터가 마이그레이션 없이 로드 가능하도록 유지해야 한다.

**R6.3 (Ubiquitous)**: 시스템은 항상 기존 API 응답 구조 (`AgentInfo`, `AgentConfig` 직렬화 형태) 를 변경 없이 유지해야 한다.

**R6.4 (Ubiquitous)**: 시스템은 항상 Web UI 의 기존 schema (`web/src/config/agentSchemas.ts` 등) 를 변경 없이 유지해야 한다.

**R6.5 (Unwanted)**: 시스템은 기존에 정상 동작하던 매니저 `Restart` 경로의 동작을 변경해서는 안 된다.

**R6.6 (Unwanted)**: 시스템은 기존에 정상 동작하던 데몬 재시작 후 `restoreAgents` 경로의 동작을 변경해서는 안 된다.

---

### M7: 회귀 방지 - 결함 A/B 의 명시적 검증

**R7.1 (Ubiquitous)**: 시스템은 항상 단일 에이전트 인스턴스에서 `Configure(host=A)` → (재)연결 → `Configure(host=B)` → (재)연결 시 각각 다른 주소로 연결되는 것을 검증하는 단위 테스트를 포함해야 한다.

**R7.2 (Ubiquitous)**: 시스템은 항상 TCP Client 의 `reconnect_interval` 변경이 진행 중인 reconnectLoop 에 즉시 반영되는 것을 검증하는 테스트를 포함해야 한다.

**R7.3 (Ubiquitous)**: 시스템은 항상 TCP Client 의 `max_retries` 변경이 다음 시도부터 반영되는 것을 검증하는 테스트를 포함해야 한다.

**R7.4 (Ubiquitous)**: 시스템은 항상 `ConfigureAgent` 가 connection 설정 변경 시 Restart 를 트리거하는 것과 operation 설정 변경 시 Restart 를 트리거하지 않는 것을 각각 검증하는 테스트를 포함해야 한다.

**R7.5 (Ubiquitous)**: 시스템은 항상 5개 에이전트 (TCP client, TCP server, UDP client, UDP server, Serial) 모두에 대해 동일한 회귀 검증 패턴을 적용해야 한다.

**R7.6 (Ubiquitous)**: 시스템은 항상 잘못된 옵션 (예: invalid framing, 음수 port) 으로 `Configure` 호출 시 in-memory 파싱 config 와 저장소가 모두 변경되지 않는 것을 검증하는 테스트를 포함해야 한다.

**R7.7 (State-Driven)**: IF `Configure` 가 Validate 단계에서 실패하면 THEN 에이전트의 `a.config`, `a.agentConfig`, 저장소 모두 호출 이전 상태로 유지되어야 한다.

---

### M8: 매니저 Restart 경로 신뢰성 보강 (v1.1.0 신규)

> 본 모듈은 M3 (`ConfigureAgent` → `manager.Restart`) 가 의존하는 매니저 Restart
> 경로의 신뢰성 결함을 사전 수정한다. 가정 A5 / NFR3 / NFR4 를 보강한다. Serial
> 에이전트의 재시작 생명주기 세부 사항은 SPEC-SERIAL-001 v2.2.0 에서 정의한다.

**R8.1 (State-Driven)**: IF 에이전트가 이미 `Stopped` 상태일 때 `Manager.Stop` 이 호출되면 THEN 시스템은 이를 no-op 으로 처리하고 에러를 반환하지 않아야 한다 (멱등성).

**R8.2 (State-Driven)**: IF 에이전트가 이미 `Stopped` 상태일 때 `Manager.Restart` 가 호출되면 THEN 시스템은 Stop 단계를 no-op 으로 건너뛰고 Start 단계만 수행해야 한다.

**R8.3 (Event-Driven)**: WHEN `Manager.Restart` 가 connection 설정 변경으로 트리거되어 인스턴스를 재생성할 때, THEN 재생성된 인스턴스는 `Stopped`/`Error` 등 어떤 직전 상태에서 출발하더라도 정상 lifecycle 경로로 진입해야 한다.

**R8.4 (Unwanted)**: 시스템은 멱등성 보강으로 인해 기존에 정상 동작하던 Restart 경로 (Running → Stop → Start) 의 동작을 변경해서는 안 된다 (NFR3 보존).

---

## 5. Out of Scope (범위 외)

본 SPEC 에서 다루지 않는 항목은 다음과 같다:

1. **Modbus / Modbus Server 에이전트**: 별도의 Configure 시맨틱스 (레지스터, 폴링 주기 등) 를 가지므로 별도 SPEC 에서 다룬다.
2. **LG (ACP5, LG HVACR-02, LGAP) / Samsung NASA 에이전트**: 프로토콜 특화 설정이 많아 별도 분류 작업이 필요하므로 본 SPEC 범위 외.
3. **File / System / HTTP Receiver / MQTT 에이전트**: Transport 에이전트가 아니거나 다른 수명주기를 가지므로 본 SPEC 범위 외.
4. **옵션 키 이름 변경**: 기존 키를 유지하며 하위 호환성을 보장한다. 키 리네이밍은 별도 SPEC.
5. **새 옵션 추가**: 본 SPEC 은 기존 옵션의 동작을 바로잡는 것이 목적이며, 새 옵션을 추가하지 않는다.
6. **Web UI 컴포넌트 변경**: UI 가 이미 `/agents/{id}/config` API 를 호출하고 있으므로 UI 계층 변경은 불필요하다.
7. **영속 저장소 스키마 변경**: JSON BLOB 방식은 그대로 유지한다.
8. **동시 다중 클라이언트 Configure 호출에 대한 별도 락 전략**: 기존 매니저 락으로 충분하다고 판단.
9. **Connection 설정 변경 시 Graceful shutdown 전략 개선**: 현재 매니저 Restart 경로가 담당하며, 본 SPEC 에서는 동작 보존에 집중한다.
10. **배포된 플로우의 런타임 영향 평가**: 에이전트 재시작이 연결된 플로우에 미치는 영향은 별도 SPEC (예: SPEC-FLOW-00x) 에서 다룬다.

---

## 6. Non-Functional Requirements (비기능 요구사항)

### 6.1 하위 호환성 (Backward Compatibility)

- **NFR1**: 본 SPEC 적용 후에도 기존 저장소 데이터는 마이그레이션 없이 로드 가능해야 한다.
- **NFR2**: `Transport.Options` 의 키 이름은 변경되지 않으며, 기존 API 클라이언트 (CLI, Web UI, 외부 스크립트) 는 수정 없이 동작해야 한다.

### 6.2 동작 보존 (Behavior Preservation)

- **NFR3**: 매니저 `Restart` 경로의 동작은 변경되지 않는다.
- **NFR4**: 데몬 재시작 후 `restoreAgents` 경로의 동작은 변경되지 않는다.
- **NFR5**: 기존에 Configure → Restart 로 정상 동작하던 경로는 동일하게 동작해야 한다.

### 6.3 동시성 안전 (Concurrency Safety)

- **NFR6**: `Configure` 호출 중 readLoop, reconnectLoop, accept loop 가 진행 중이어도 race condition 이 발생하지 않아야 한다. `go test -race ./...` 통과를 요구한다.
- **NFR7**: 에이전트의 `a.config` 필드 접근은 뮤텍스로 보호되거나 원자적 스냅샷 교체를 통해 안전해야 한다.

### 6.4 관측성 (Observability)

- **NFR8**: connection 설정 변경으로 인한 Restart 트리거는 INFO 레벨로 기록되어야 한다 (에이전트 ID 포함).
- **NFR9**: operation 설정 변경으로 인한 핫 리로드는 INFO 레벨로 기록되어야 한다 (에이전트 ID 포함).
- **NFR10**: Configure 실패 (Validate 에러, 파싱 에러, 저장소 실패) 는 ERROR 레벨로 기록되어야 한다.

### 6.5 신뢰성 (Reliability)

- **NFR11**: Configure 호출이 Validate 단계에서 실패하면 in-memory 상태는 절대로 부분 변경되어서는 안 된다.
- **NFR12**: 저장소 영속화 실패 시 in-memory 상태는 롤백되어야 한다.

### 6.6 테스트 커버리지 (Test Coverage)

- **NFR13**: 본 SPEC 으로 신규 추가되는 코드의 라인 커버리지는 85% 이상이어야 한다.
- **NFR14**: 결함 A 와 B 의 회귀 방지 테스트는 CI 에서 항상 실행되어야 한다.

### 6.7 성능 (Performance)

- **NFR15**: Configure 호출의 응답 시간은 매니저 Restart 가 필요 없는 경우 기존 Configure 호출 대비 증가분이 10% 이내여야 한다.
- **NFR16**: connection 변경 판정 로직은 옵션 키 수에 비례하는 O(N) 이어야 한다.

---

## 7. Traceability

| 요구사항 | 구현 모듈 | 테스트 | 비고 |
|----------|-----------|--------|------|
| M1 (R1.1~R1.5) | `internal/agent/agent.go` 또는 `internal/agent/config.go` (인터페이스), 각 에이전트 파일 | 인터페이스 정의 테스트 | 분류 위임 메커니즘 |
| M2 (R2.1~R2.8) | `internal/agent/socket/tcp_client.go`, `tcp_server.go`, `udp_client.go`, `udp_server.go`, `internal/agent/serial/agent.go` | `tcp_client_test.go`, `tcp_server_test.go`, `udp_client_test.go`, `udp_server_test.go`, `serial/agent_test.go` | Configure 재파싱 |
| M3 (R3.1~R3.9) | `internal/api/service/agent_adapter.go` | `agent_adapter_test.go` | needsRestart 재설계 |
| M4 (R4.1~R4.4) | `internal/agent/socket/tcp_client.go`, `internal/agent/socket/config.go` | `tcp_client_test.go`, `config_test.go` | TCP Client 분류 |
| M5 (R5.1~R5.10) | 나머지 4개 에이전트 + `config.go` | 각 에이전트 테스트 | 분류 |
| M6 (R6.1~R6.6) | 전체 변경 범위 | backward 회귀 테스트 | 하위 호환 |
| M7 (R7.1~R7.7) | 5개 에이전트 + service adapter | 회귀 테스트 스위트 | 결함 A/B 방지 |
| M8 (R8.1~R8.4) | `internal/agent/manager.go` (Stop/Restart 멱등성), `internal/agent/serial/agent.go` | `manager_test.go`, `serial/agent_test.go` | v1.1.0 신규, SPEC-SERIAL-001 v2.2.0 연계 |

---

## 8. References (참조)

- **결함 분석 파일**:
  - `internal/agent/socket/tcp_client.go:404-415` (Configure 결함 A)
  - `internal/agent/socket/tcp_server.go:331-339`
  - `internal/agent/socket/udp_client.go:237-245`
  - `internal/agent/socket/udp_server.go:282-290`
  - `internal/agent/serial/agent.go:383-393`
  - `internal/api/service/agent_adapter.go:285-300` (결함 B needsRestart 화이트리스트)
  - `internal/api/service/agent_adapter.go:302-348` (ConfigureAgent)
- **관련 SPEC**:
  - SPEC-AGENT-005 (Enable/Disable, AgentConfig 확장 패턴 참조)
  - SPEC-SERIAL-001 (Serial 에이전트 기본 설계)
  - SPEC-SOCKET-001 (Socket 에이전트 기본 설계)
- **EARS Format**: Easy Approach to Requirements Syntax (Mavin, 2009)

---

## 9. Implementation Notes (구현 메모)

본 섹션은 `/moai sync` 단계에서 채워질 자리표시자이다. Level 1
(spec-first) lifecycle 에 따라 SPEC 본문의 요구사항은 변경하지 않고,
실제 구현이 계획과 달라진 부분과 보완된 부분만 기록한다.

현재 상태: `draft` - 구현 시작 전.
