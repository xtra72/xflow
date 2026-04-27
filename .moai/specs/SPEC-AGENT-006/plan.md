---
id: SPEC-AGENT-006
version: "1.0.0"
status: draft
created: "2026-04-10"
author: xtra
priority: high
related_spec: SPEC-AGENT-006
---

# SPEC-AGENT-006: Implementation Plan - Transport Agent Configuration 분리 및 Hot-Reload

## 1. 구현 개요

본 계획은 SPEC-AGENT-006 의 요구사항 (M1~M7) 을 7개의 구현 단계 (Phase) 로 분할한다. 각 Phase 는 의존 관계에 따라 순차적으로 실행되며, Hybrid 개발 모드 (신규 코드는 TDD, 기존 코드 수정은 DDD) 를 적용한다.

### 1.1 개발 방법론

- **Methodology**: Hybrid (TDD for new code + DDD for existing code modifications)
  - 신규 도메인 모델 (connection/operation 분류 인터페이스, 헬퍼 메서드): TDD
  - 기존 Configure 메서드 수정: DDD (characterization test 우선 작성으로 기존 동작 보존)
  - 기존 `agent_adapter.go` 의 `needsRestart` 수정: DDD
- **Test Coverage Target**: 신규 코드 ≥ 85%, 수정 코드 ≥ 85%
- **참조 패턴**: SPEC-AGENT-005 의 `NodeDef.Enabled *bool` + `IsEnabled()` 패턴을 분류 API 에도 적용한다 (메서드 기반 위임).

### 1.2 우선순위

| 우선순위 | 단계 | 설명 |
|---------|------|------|
| Primary Goal | Phase 1, 2 | 분류 도메인 모델 + Configure 재파싱 (결함 A 직접 수정) |
| Secondary Goal | Phase 3, 4 | 서비스 어댑터 분류 위임 + 5개 에이전트 확대 적용 (결함 B 수정) |
| Tertiary Goal | Phase 5, 6 | 전체 회귀 테스트 + 동시성 검증 |
| Final Goal | Phase 7 | 문서화 + 최종 검증 |

> **시간 추정 금지**: 본 계획은 우선순위 기반이며, 각 Phase 의 절대 소요 시간은 명시하지 않는다. Phase 간 의존성은 명시한다.

---

## 2. 사전 설계 결정 (Pre-Implementation Decisions)

Plan 승인 단계에서 확정해야 할 설계 결정을 아래에 정리한다. 각 항목은 후속 Phase 의 코드 구조를 결정하므로 구현 시작 전 합의가 필요하다.

### 2.1 결정 (a): Connection/Operation 을 별도 struct 로 분리할지, 같은 struct 안의 메서드로 분류할지

**선택지 A1 - 별도 struct 분리**:

```
type TCPClientConnectionConfig struct { Host, Port, ... }
type TCPClientOperationConfig  struct { MaxMessageSize, ReconnectInterval, ... }
type TCPClientConfig struct {
    Connection TCPClientConnectionConfig
    Operation  TCPClientOperationConfig
}
```

- 장점: 타입 시스템으로 분류 강제, 파싱 함수도 분리 가능.
- 단점: 기존 `TCPClientConfig` 사용처 전반 수정. 파싱 함수 호환성 깨짐. 내부 사용 지점 (tcp_client.go:133, 139, 257, 269 등) 모두 수정.

**선택지 A2 - 동일 struct + 메서드 기반 분류** (권장):

```
type TCPClientConfig struct { Host, Port, ... }  // 기존 그대로

func TCPClientConnectionKeys() []string {
    return []string{"host", "port", "framing", "delimiter", "fixed_size", "buffer_size"}
}
```

- 장점: 기존 구조체와 사용처 변경 최소화. 결함 A/B 수정에만 집중. 코드 diff 축소.
- 단점: 타입 레벨 강제력 없음. 분류 위반 가능성은 테스트로 보완.

**결정**: **A2 선택 (권장)**. 본 SPEC 은 결함 수정에 초점을 두므로 구조 대변경을 피한다. 분류의 correctness 는 테스트와 문서로 보장한다. Phase 1 에서 각 에이전트별 `ConnectionKeys()` 함수 또는 메서드를 정의한다.

### 2.2 결정 (b): Configure 의 시그니처 변경 여부

**선택지 B1 - 시그니처 유지, 내부 동작만 수정**:

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error
```

- 장점: 인터페이스 호환. agent_adapter.go 의 호출 지점 변경 최소화.
- 단점: connection 변경 여부를 반환할 방법이 없으므로 별도 API 필요.

**선택지 B2 - ConfigDiff 반환**:

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) (ConfigDiff, error)
// ConfigDiff 에 ConnectionChanged bool, OperationChanged bool 포함
```

- 장점: 호출자가 즉시 분기 가능.
- 단점: `Agent` 인터페이스의 Configure 시그니처 변경 → 모든 에이전트 수정 필요.

**선택지 B3 - 별도 API 추가**:

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error
// 신규:
func (a *TCPClientAgent) WasLastConfigureConnectionChange() bool
// 또는 ConnectionConfigChecker 인터페이스로 "비교" 를 서비스 어댑터가 수행
```

**결정**: **B3 의 변형 - `ConnectionConfigChecker` 인터페이스** 를 채택한다. 각 Transport 에이전트가 다음 인터페이스를 구현한다:

```
type ConnectionConfigChecker interface {
    // IsConnectionChange 는 old 와 new Options 맵을 비교하여
    // connection 설정에 차이가 있는지 반환한다.
    IsConnectionChange(oldOpts, newOpts map[string]any) bool
}
```

이 방식은 다음 이점이 있다:

- 기존 `Configure` 시그니처 유지 (결정 B1 의 호환성).
- 서비스 어댑터가 `Configure` 호출 전에 old options 스냅샷을 보유하고 있으므로 비교 가능.
- 에이전트가 자신의 분류 책임을 완전히 가짐 (R1.3, R3.3).
- 비-Transport 에이전트는 인터페이스를 구현하지 않으므로 기본 동작 (Restart 항상 트리거 또는 항상 트리거 안 함) 을 기존 로직에 맞춰 선택 가능.

### 2.3 결정 (c): Agent 인터페이스에 새 메서드 추가 여부

**결정**: `Agent` 공통 인터페이스에 새 메서드를 강제 추가하지 않는다. 대신 별도의 **옵셔널 인터페이스** `ConnectionConfigChecker` 를 `internal/agent` 패키지에 추가하고, 서비스 어댑터는 타입 단언으로 확인한다:

```
if checker, ok := ag.(agent.ConnectionConfigChecker); ok {
    if checker.IsConnectionChange(oldOpts, newOpts) {
        // Restart
    }
}
```

이 방식은 Modbus, LG, NASA, File, System, MQTT 같이 본 SPEC 범위 외의 에이전트에 영향을 주지 않는다. 본 SPEC 의 5개 Transport 에이전트만 이 인터페이스를 구현한다.

**기본 동작 (fallback)**: 에이전트가 `ConnectionConfigChecker` 를 구현하지 않는 경우, 서비스 어댑터는 **보수적으로 Restart 를 트리거** 한다 (기존 동작 보존을 위해). 또는 에이전트가 직접 "나는 Restart 불필요" 를 알릴 수 있는 추가 옵셔널 인터페이스를 미래에 도입할 수 있으나, 본 SPEC 범위 외이다.

### 2.4 결정 (d): 5개 에이전트의 변경을 단일 PR 로 묶을지, agent 별로 분할할지

**결정**: **단일 PR** 로 처리한다. 사유:

- 결함 A 와 B 는 5개 에이전트에 동일하게 존재하며, 분리 시 중간 상태에서 일부 에이전트만 수정되어 일관성이 깨진다.
- 테스트 스위트는 5개 모두를 동일한 패턴으로 검증한다.
- 회귀 방지 테스트 (Phase 5) 는 모든 에이전트에 걸쳐 있어야 의미가 있다.

단, Phase 별로는 파일 단위 또는 에이전트 단위로 작업을 분할한다 (TodoList 로 진행 추적).

### 2.5 결정 (e): Hybrid 개발 모드 적용

- **신규 코드 (TDD)**:
  - Phase 1 의 `ConnectionConfigChecker` 인터페이스 및 `ConnectionKeys()` 함수군
  - Phase 1 의 각 에이전트의 `IsConnectionChange` 메서드
  - Phase 4 의 회귀 테스트 스위트
- **기존 코드 수정 (DDD)**:
  - Phase 2 의 각 에이전트 `Configure` 메서드 수정 - characterization test 로 기존 동작 (매니저 Restart 경로가 깨지지 않음) 을 먼저 확인
  - Phase 3 의 `agent_adapter.go` 의 `needsRestart` 및 `ConfigureAgent` 수정 - characterization test 로 기존 ConfigureAgent 성공 경로를 보존

---

## 3. Phase 1: 분류 도메인 모델 - 인터페이스 및 Connection Keys 정의

### 3.1 목표

Connection 설정과 Operation 설정을 분류하는 도메인 모델을 정의한다. `ConnectionConfigChecker` 옵셔널 인터페이스와 각 Transport 에이전트의 `ConnectionKeys()` 함수를 도입한다.

### 3.2 변경 파일

#### `internal/agent/agent.go` 또는 `internal/agent/config.go`

**변경 사항:**

1. 옵셔널 인터페이스 정의:

   ```
   // ConnectionConfigChecker 는 Transport 에이전트가 자신의 connection 설정
   // 변경 여부를 판단할 수 있도록 하는 옵셔널 인터페이스이다. 이 인터페이스를
   // 구현한 에이전트는 Configure 호출 전에 old options 와 new options 를
   // 비교하여 매니저의 Restart 가 필요한지 알려준다.
   type ConnectionConfigChecker interface {
       IsConnectionChange(oldOpts, newOpts map[string]any) bool
   }
   ```

2. 헬퍼 함수 (optional):

   ```
   // OptionsDiffer 는 주어진 키 집합에 대해 두 options 맵이 다른지 판단한다.
   // 에이전트 구현이 공통으로 사용할 수 있다.
   func OptionsDiffer(oldOpts, newOpts map[string]any, keys []string) bool
   ```

**TDD 단계:**

1. **RED**: `agent_test.go` 또는 `config_test.go` 에 `TestOptionsDiffer_*` 테이블 테스트 작성
   - 빈 맵 vs 빈 맵 → false
   - 같은 값 → false
   - 다른 값 → true
   - 한쪽에만 키 존재 → true
2. **GREEN**: `OptionsDiffer` 구현
3. **REFACTOR**: 코드 스타일 점검, godoc 보완

#### `internal/agent/socket/config.go`

**변경 사항:**

1. 각 Transport config 에 대해 connection keys 를 반환하는 패키지 레벨 함수 추가:

   ```
   // TCPClientConnectionKeys 는 TCP Client 에이전트의 connection 설정 키 목록을 반환한다.
   func TCPClientConnectionKeys() []string {
       return []string{"host", "port", "framing", "delimiter", "fixed_size", "buffer_size"}
   }
   func TCPServerConnectionKeys() []string { ... }
   func UDPClientConnectionKeys() []string { ... }
   func UDPServerConnectionKeys() []string { ... }
   ```

#### `internal/agent/serial/config.go`

**변경 사항:**

1. Serial 에 대해 동일한 함수 추가:

   ```
   func SerialConnectionKeys() []string {
       return []string{"port", "baud_rate", "data_bits", "stop_bits", "parity", "framing",
                       "stx", "etx", "length_offset", "length_size", "length_includes_header"}
   }
   ```

   (실제 framing 관련 키는 `config.go` 의 `ParseSerialConfig` 를 확인하여 정확히 맞춘다)

**TDD 단계**:

1. **RED**: `config_test.go` 에 `TestTCPClientConnectionKeys_ContainsExpected` 등 테스트 작성 (R4.1 의 표에 명시된 키들이 정확히 포함되는지)
2. **GREEN**: 함수 구현
3. **REFACTOR**: 키 상수화 여부 검토

### 3.3 의존성

- 외부 의존: 없음
- 후속 Phase 의존: Phase 2, 3 이 본 Phase 의 결과물에 의존

### 3.4 완료 조건

- [ ] `ConnectionConfigChecker` 인터페이스 정의
- [ ] `OptionsDiffer` 헬퍼 구현 및 단위 테스트
- [ ] 5개 에이전트별 `*ConnectionKeys()` 함수 구현
- [ ] 각 `*ConnectionKeys()` 함수가 R4.1, R5.1, R5.3, R5.5, R5.7 과 일치함을 검증하는 테스트
- [ ] 기존 테스트 회귀 없음

---

## 4. Phase 2: 각 Transport 에이전트의 Configure 재파싱 수정

### 4.1 목표

5개 Transport 에이전트의 `Configure` 메서드가 입력된 새 옵션을 재파싱하여 internal config 구조체를 갱신하도록 수정한다 (결함 A 직접 수정). 동시에 각 에이전트가 `ConnectionConfigChecker` 인터페이스를 구현한다.

### 4.2 변경 파일

#### `internal/agent/socket/tcp_client.go`

**현재 동작** (lines 404-415):

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error {
    if err := config.Validate(); err != nil { ... }
    a.mu.Lock()
    a.agentConfig = config
    a.mu.Unlock()
    return nil
}
```

**변경 후 동작**:

```
func (a *TCPClientAgent) Configure(config agent.AgentConfig) error {
    if err := config.Validate(); err != nil { ... }
    // 1) 재파싱 (실패 시 기존 상태 유지)
    newCfg, err := ParseTCPClientConfig(config)
    if err != nil {
        return fmt.Errorf("parse tcp client config: %w", err)
    }
    // 2) 원자적 갱신
    a.mu.Lock()
    a.agentConfig = config
    a.config = newCfg
    a.mu.Unlock()
    return nil
}

// IsConnectionChange 는 old 와 new options 에서 connection 관련 키가 다른지 반환한다.
func (a *TCPClientAgent) IsConnectionChange(oldOpts, newOpts map[string]any) bool {
    return agent.OptionsDiffer(oldOpts, newOpts, TCPClientConnectionKeys())
}
```

**또한 a.config 읽기 지점의 동시성 보호** (DDD ANALYZE 결과에 따라):

- `tcp_client.go:133`, `139`, `257-259`, `269` 는 현재 `a.config` 를 잠금 없이 읽는다. Configure 가 수정 중일 때 race 발생 가능.
- 해결: `a.config` 읽기를 `a.mu` 로 감싸거나, 스냅샷 복사본을 반환하는 헬퍼 (`a.snapshotConfig() TCPClientConfig`) 를 도입한다.
- 또는 `atomic.Pointer[TCPClientConfig]` 사용 (Go 1.19+). 단순성 위해 뮤텍스 접근 권장.

**DDD 단계** (기존 코드 수정):

1. **ANALYZE**: `a.config` 의 모든 read 지점 (tcp_client.go:133, 139, 257-259, 269) 을 조사하고 현재 어떻게 접근하는지 파악.
2. **PRESERVE**: characterization test 작성:
   - `TestConfigure_MatchesManagerRestartBehavior` - 매니저 Restart 경로가 깨지지 않음을 확인
   - `TestConfigure_Validate_Failure_LeavesStateUntouched` - R7.7
3. **IMPROVE**:
   - 재파싱 로직 추가
   - `IsConnectionChange` 구현
   - `a.config` 읽기 지점의 동시성 보호
4. **VERIFY**: 기존 테스트 + 신규 테스트 모두 통과. `go test -race` 통과.

#### `internal/agent/socket/tcp_server.go`

동일한 패턴으로 수정:

- `Configure` 에서 `ParseTCPServerConfig` 재호출
- `IsConnectionChange` 구현 (TCPServerConnectionKeys 사용)
- `a.config` 접근 지점 (accept loop 등) 의 동시성 보호

#### `internal/agent/socket/udp_client.go`

동일 패턴. UDPClientConnectionKeys 사용.

#### `internal/agent/socket/udp_server.go`

동일 패턴. UDPServerConnectionKeys 사용.

#### `internal/agent/serial/agent.go`

동일 패턴. `ParseSerialConfig` 재호출. SerialConnectionKeys 사용.

추가 고려: Serial 의 `gap_timeout` 핫 리로드 (R5.9). 현재 Serial 이 `SetReadTimeout` 을 어떻게 사용하는지 확인 후, Configure 에서 새 값을 반영하도록 처리한다. 시리얼 포트가 열려있을 때만 재적용하며, 포트가 닫혀있으면 다음 Open 시 자동 반영되므로 추가 처리 불필요.

### 4.3 테스트 전략

각 에이전트별로 다음 TDD 사이클 적용 (신규 테스트는 TDD, 기존 동작 보존 검증은 characterization test):

**tcp_client_test.go 신규 테스트**:

1. `TestConfigure_ReparsesInternalConfig` - 새 host 를 Configure 하면 `a.config.Host` 가 갱신됨
2. `TestConfigure_ParseFailure_PreservesOldConfig` - 잘못된 옵션으로 Configure 호출 시 `a.config` 변경 없음 (R7.7)
3. `TestConfigure_ValidateFailure_PreservesOldConfig` - 동일
4. `TestIsConnectionChange_HostDiffer_ReturnsTrue` - host 변경 탐지
5. `TestIsConnectionChange_ReconnectIntervalDiffer_ReturnsFalse` - operation 키 변경은 false
6. `TestIsConnectionChange_SameOptions_ReturnsFalse`
7. `TestConfigure_Race_NoDataRace` - `go test -race` 로 검증 (Configure 와 동시에 a.config 읽기 루프 실행)

**동일 패턴을 tcp_server_test.go, udp_client_test.go, udp_server_test.go, serial/agent_test.go 에 적용**.

### 4.4 의존성

- 선행: Phase 1 (ConnectionKeys 함수, OptionsDiffer)
- 후속: Phase 3 이 본 Phase 의 결과물에 의존

### 4.5 완료 조건

- [ ] 5개 에이전트 모두 `Configure` 에서 재파싱 수행
- [ ] 5개 에이전트 모두 `ConnectionConfigChecker` 인터페이스 구현
- [ ] 5개 에이전트 모두 Configure 실패 시 상태 롤백
- [ ] `a.config` 읽기 지점의 동시성 보호
- [ ] `go test -race ./internal/agent/...` 통과
- [ ] 기존 `Restart` 경로 동작 보존 (characterization test 통과)

---

## 5. Phase 3: agent_adapter.go 재설계 - 분류 위임

### 5.1 목표

`ConfigureAgent` 가 하드코딩된 `transportKeys` 화이트리스트 대신 에이전트의 `ConnectionConfigChecker` 인터페이스를 사용하여 Restart 필요성을 판단하도록 재설계한다 (결함 B 수정).

### 5.2 변경 파일

#### `internal/api/service/agent_adapter.go`

**현재 결함** (lines 285-300, 302-348):

- `needsRestart` 는 하드코딩된 `transportKeys` 에 `tcp_host`, `tcp_port` 등 존재하지 않는 키를 사용.
- Socket 에이전트는 실제로 `host`, `port` 키를 사용하므로 host 변경이 탐지되지 않음.

**변경 후 동작**:

1. `transportKeys` 하드코딩 삭제.
2. `ConfigureAgent` 가 다음 순서로 동작:
   - (a) 기존 에이전트의 old options 스냅샷 저장
   - (b) `ag.Configure(newCfg)` 호출 (결함 A 가 수정된 후이므로 내부 config 가 갱신됨)
   - (c) 에이전트가 `ConnectionConfigChecker` 를 구현하면 `checker.IsConnectionChange(oldOpts, newOpts)` 호출
   - (d) connection 변경 시 `m.manager.Restart(ctx, id)` 호출, 아니면 스킵
   - (e) 저장소 영속화
   - (f) 실패 시 in-memory 롤백

**구체 의사 코드**:

```
func (s *AgentService) ConfigureAgent(ctx context.Context, id string, newCfg agent.AgentConfig) (AgentInfo, error) {
    // 1) 기존 상태 스냅샷
    oldCfg, err := s.manager.GetConfig(id)
    if err != nil { return AgentInfo{}, err }
    oldOpts := copyOptions(oldCfg.Transport.Options)

    ag, err := s.manager.Get(id)
    if err != nil { return AgentInfo{}, err }

    // 2) Configure 호출 (재파싱 포함)
    if err := ag.Configure(newCfg); err != nil {
        return AgentInfo{}, fmt.Errorf("configure agent: %w", err)
    }

    // 3) Connection 변경 판단
    isConnChange := false
    if checker, ok := ag.(agent.ConnectionConfigChecker); ok {
        isConnChange = checker.IsConnectionChange(oldOpts, newCfg.Transport.Options)
    } else {
        // 인터페이스 미구현 에이전트 - 보수적으로 Restart 트리거
        isConnChange = !optionsEqual(oldOpts, newCfg.Transport.Options)
    }

    // 4) 필요 시 Restart
    if isConnChange {
        slog.Info("connection config changed, restart triggered", "agent_id", id)
        if err := s.manager.Restart(ctx, id); err != nil {
            // 5) 롤백
            _ = ag.Configure(oldCfg)
            return AgentInfo{}, fmt.Errorf("restart agent: %w", err)
        }
    } else {
        slog.Info("operation config changed, hot-reloaded", "agent_id", id)
    }

    // 6) 영속화
    if err := s.store.Put(id, newCfg); err != nil {
        _ = ag.Configure(oldCfg)
        return AgentInfo{}, fmt.Errorf("persist agent config: %w", err)
    }

    return s.GetAgent(ctx, id)
}
```

**주의**: 실제 메서드 시그니처와 매니저/스토어 API 는 구현 시점에 확인. 위 의사 코드는 설계 원칙을 표현한다.

**DDD 단계**:

1. **ANALYZE**: 현재 `agent_adapter.go:285-348` 의 전체 흐름 파악. 특히 성공 경로, 에러 경로, 영속화 순서 확인.
2. **PRESERVE**: characterization test 작성:
   - `TestConfigureAgent_CurrentSuccessPath_Unchanged` - Restart 가 필요한 기존 케이스 (예: transport_type 변경) 가 그대로 동작
   - `TestConfigureAgent_CurrentPersistencePath_Unchanged` - 저장소 영속화 타이밍 유지
   - `TestConfigureAgent_CurrentRollbackPath_Unchanged` - 실패 시 롤백 유지
3. **IMPROVE**:
   - `needsRestart` 삭제
   - 새 흐름 구현
4. **VERIFY**: characterization + 신규 테스트 통과

### 5.3 테스트 전략

**agent_adapter_test.go 신규 테스트**:

1. `TestConfigureAgent_TCPClient_HostChanged_TriggersRestart` (핵심 회귀 테스트)
2. `TestConfigureAgent_TCPClient_ReconnectIntervalChanged_NoRestart`
3. `TestConfigureAgent_TCPClient_MaxRetriesChanged_NoRestart`
4. `TestConfigureAgent_TCPServer_PortChanged_TriggersRestart`
5. `TestConfigureAgent_UDPClient_HostChanged_TriggersRestart`
6. `TestConfigureAgent_Serial_BaudRateChanged_TriggersRestart`
7. `TestConfigureAgent_Serial_GapTimeoutChanged_NoRestart`
8. `TestConfigureAgent_PersistenceFailure_RollsBackAgentState`
9. `TestConfigureAgent_RestartFailure_RollsBackAgentState`
10. `TestConfigureAgent_NonCheckerAgent_ConservativeRestart` (File, Modbus 등 인터페이스 미구현 에이전트)

Mock 또는 fake agent 로 `ConnectionConfigChecker` 구현 여부를 시뮬레이션한다.

### 5.4 의존성

- 선행: Phase 1 (인터페이스 정의), Phase 2 (에이전트 구현)
- 후속: Phase 4, 5 (통합 테스트)

### 5.5 완료 조건

- [ ] `transportKeys` 하드코딩 삭제
- [ ] `ConnectionConfigChecker` 인터페이스 타입 단언으로 분류 판단
- [ ] 5개 에이전트 각각에 대해 connection 변경 시 Restart, operation 변경 시 no-op 검증
- [ ] 영속화/Restart 실패 시 롤백
- [ ] characterization test 통과 (기존 ConfigureAgent 동작 보존)

---

## 6. Phase 4: 5개 에이전트 회귀 방지 테스트 스위트

### 6.1 목표

결함 A 와 B 가 다시 발생하지 않도록 하는 명시적인 회귀 방지 테스트를 각 에이전트에 추가한다.

### 6.2 추가 테스트 (각 에이전트별)

#### TCP Client (`internal/agent/socket/tcp_client_test.go`)

1. `TestRegression_ConfigureHostB_ConnectsToB` (R7.1 핵심):
   - Setup: 로컬 포트 P1 에 테스트 서버 기동, 에이전트에 host=localhost, port=P1 설정
   - Start → 연결 성공 확인
   - Stop → `Configure(host=localhost, port=P2)` (P2 에 다른 테스트 서버) → Start
   - **검증**: 에이전트가 P2 에 연결됨을 확인
   - **결함 A 가 존재하면 이 테스트는 FAIL** (P1 에 계속 연결 시도)

2. `TestRegression_ReconnectIntervalChangeAppliesInLoop` (R7.2):
   - reconnectLoop 가 진행 중인 상태에서 Configure 로 `reconnect_interval` 변경
   - 다음 backoff 계산이 새 값을 사용함을 확인 (timing 기반 검증 또는 mocked clock)

3. `TestRegression_MaxRetriesChangeAppliesNext` (R7.3):
   - `max_retries=5` 로 시작, 3회 시도 후 Configure 로 `max_retries=2` 변경
   - 이후 추가 시도가 새 값을 기준으로 중단됨을 확인

#### TCP Server (`internal/agent/socket/tcp_server_test.go`)

1. `TestRegression_ConfigurePortB_BindsToB`:
   - 포트 P1 에 bind → Configure 로 포트 P2 로 변경 → (서비스 어댑터 경유 시 Restart 발생) → P2 에서 listening 확인
   - 단위 테스트에서는 매니저 없이 에이전트 재시작을 직접 수행 (Configure 후 인스턴스 재생성 경로 시뮬레이션)

#### UDP Client (`internal/agent/socket/udp_client_test.go`)

1. `TestRegression_ConfigureHostB_SendsToB`

#### UDP Server (`internal/agent/socket/udp_server_test.go`)

1. `TestRegression_ConfigurePortB_ReceivesFromB`

#### Serial (`internal/agent/serial/agent_test.go`)

1. `TestRegression_ConfigureBaudRateChangesPortSettings`:
   - Mock serial port 를 사용하여 baud rate 변경이 재열기 시점에 반영됨을 확인

### 6.3 통합 회귀 테스트 (`internal/api/service/agent_adapter_test.go`)

1. `TestRegression_TCPClient_HostChange_EndToEnd`:
   - Full stack: store + manager + adapter + real TCPClient
   - Initial host=A, Start, 연결 확인
   - `ConfigureAgent(host=B)` 호출 → 매니저 Restart 자동 트리거 → B 에 연결 확인
   - **결함 B 가 존재하면 이 테스트는 FAIL** (Restart 트리거되지 않아 A 에 계속 연결)

2. `TestRegression_TCPClient_ReconnectIntervalChange_NoRestart`:
   - Initial 상태에서 ConfigureAgent 로 operation 키만 변경
   - 매니저가 Restart 되지 않음 검증 (예: 매니저의 Restart 호출 카운터)
   - 에이전트의 내부 값이 갱신됨 검증

### 6.4 의존성

- 선행: Phase 1, 2, 3
- 후속: Phase 5

### 6.5 완료 조건

- [ ] 5개 에이전트 각각에 회귀 방지 테스트 추가
- [ ] End-to-end 회귀 테스트 2개 이상 (결함 A 회귀, 결함 B 회귀)
- [ ] `go test -race ./...` 통과
- [ ] 회귀 테스트가 이전 결함 버전의 코드에서 FAIL 하는 것을 확인 (reproduction-first 원칙)

---

## 7. Phase 5: 동시성 및 Race Condition 검증

### 7.1 목표

Configure 호출이 readLoop, reconnectLoop, accept loop 와 동시에 실행되어도 race condition 이 없음을 검증한다.

### 7.2 테스트 전략

**각 에이전트별 race test**:

1. `TestConcurrent_ConfigureDuringReadLoop`:
   - Start → readLoop 활성화 상태에서 Configure 반복 호출
   - `go test -race` 로 data race 감지
2. `TestConcurrent_ConfigureDuringReconnectLoop` (TCP Client):
   - Start 실패 상황 유도 → reconnectLoop 진입 → 동시에 Configure 반복
3. `TestConcurrent_ConfigureDuringAcceptLoop` (TCP Server, UDP Server):
   - Start → accept 대기 → 동시에 Configure 반복

### 7.3 수정 대상

Phase 2 에서 추가한 동시성 보호가 충분한지 검증한다. `go test -race` 실패 시 해당 지점에 추가 락 또는 atomic 교체 적용.

### 7.4 의존성

- 선행: Phase 2, 3, 4
- 후속: Phase 6

### 7.5 완료 조건

- [ ] 5개 에이전트 모두 race test 추가
- [ ] `go test -race ./internal/agent/...` 및 `go test -race ./internal/api/service/...` 통과
- [ ] race 가 발견되면 Phase 2 의 동시성 보호 강화

---

## 8. Phase 6: 통합 시나리오 및 수동 검증

### 8.1 목표

End-to-end 통합 테스트와 Web UI 기반 수동 smoke test 를 수행한다.

### 8.2 통합 테스트 시나리오

#### Test 1: TCP Client 운영 중 host 변경 (Web UI 시나리오)

1. 테스트 TCP 서버 S1 기동 (port 10001)
2. 에이전트 A 생성 (host=localhost, port=10001), Start → 연결 성공 확인
3. 테스트 TCP 서버 S2 기동 (port 10002)
4. `PUT /agents/A/config` 호출하여 port=10002 로 변경
5. 검증:
   - 서비스 어댑터가 Restart 를 트리거함
   - A 가 S2 에 연결됨 (S2 의 connection counter 증가)
   - S1 의 connection 은 정상 종료됨

#### Test 2: TCP Client operation 설정 핫 리로드

1. 에이전트 A 를 `reconnect_interval=5s`, `max_retries=10` 으로 시작
2. `PUT /agents/A/config` 로 `reconnect_interval=1s`, `max_retries=3` 변경
3. 검증:
   - Restart 가 트리거되지 않음 (매니저의 인스턴스 ID 변경 없음)
   - 에이전트의 내부 값이 갱신됨 (AgentInfo 조회 시)
   - 연결이 끊어져도 새 값으로 reconnectLoop 동작

#### Test 3: Serial baud rate 변경

1. Mock serial 환경에서 에이전트 B 생성 (baud_rate=9600), Start
2. `PUT /agents/B/config` 로 baud_rate=19200 변경
3. 검증: Restart 트리거, 새 baud rate 로 port 재열림

#### Test 4: Serial gap_timeout 핫 리로드

1. 에이전트 B 를 `gap_timeout=100ms` 로 시작
2. Configure 로 `gap_timeout=50ms` 변경
3. 검증: Restart 없음, 다음 read 주기부터 새 값 사용

#### Test 5: 데몬 재시작 경로 보존 (NFR4)

1. 에이전트 A, B, C 를 enabled 로 시작
2. 데몬 정지
3. 데몬 재시작
4. 검증: 3개 에이전트 모두 저장된 설정대로 자동 시작 (restoreAgents 경로 동작 보존)

#### Test 6: 잘못된 설정 시 롤백

1. 에이전트 A (host=localhost, port=10001) running 상태
2. `PUT /agents/A/config` 로 `port=-1` (invalid) 전송
3. 검증:
   - 응답 4xx 에러
   - A 는 여전히 port=10001 로 동작
   - 저장소에도 변경 없음

### 8.3 수동 smoke test (Personal mode operator)

- Web UI 에서 TCP Client 에이전트의 host 변경 시나리오 검증
- 변경 후 즉시 새 주소로 연결되는지 확인
- 로그에서 "connection config changed, restart triggered" 관찰

### 8.4 의존성

- 선행: Phase 1 ~ 5
- 후속: Phase 7

### 8.5 완료 조건

- [ ] 6개 통합 테스트 시나리오 통과
- [ ] 수동 smoke test 통과
- [ ] 기존 기능 회귀 없음 (전체 테스트 스위트 통과)

---

## 9. Phase 7: 문서화 및 최종 검증

### 9.1 문서 업데이트

- [ ] `CHANGELOG.md`: SPEC-AGENT-006 entry 추가 - "Fix: TCP/UDP/Serial 에이전트의 Configure 가 파싱된 config 를 갱신하지 않던 결함 수정. Host/Port 변경 시 자동 Restart 트리거 수정."
- [ ] `docs/` 하위의 에이전트 설정 가이드 (있는 경우): connection vs operation 분류표 추가
- [ ] 코드 내 godoc 주석 (한국어): `ConnectionConfigChecker`, `*ConnectionKeys()` 함수, 각 에이전트의 `Configure` 메서드

### 9.2 최종 검증

- [ ] `go build ./...` 성공
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] `gofmt -l .` 출력 없음
- [ ] `golangci-lint run` 통과
- [ ] `tsc --noEmit` (Web) 통과 - UI 변경이 없으므로 기존 상태 유지
- [ ] 신규 코드 커버리지 ≥ 85%
- [ ] PR 에서 결함 A 와 B 의 재현 시나리오와 수정 확인 내역 포함

### 9.3 완료 조건

- [ ] 모든 품질 게이트 통과
- [ ] CHANGELOG 업데이트
- [ ] SPEC status: `draft` → `completed`

---

## 10. 위험 요소 및 대응

### 10.1 위험: 기존 동작 회귀

- **위험도**: 높음
- **시나리오**: Configure 재파싱 수정이 기존 정상 동작 경로 (매니저 Restart, 데몬 재시작) 에 부작용 유발
- **대응**:
  - Phase 2, 3 의 DDD (characterization test 우선)
  - Phase 6 의 Test 5 로 데몬 재시작 경로 명시 검증
  - 회귀 테스트는 이전 코드와 새 코드 모두에서 실행하여 의도치 않은 변경 탐지

### 10.2 위험: 동시성 issue

- **위험도**: 중간
- **시나리오**: Configure 와 readLoop 가 동시 실행 중 `a.config` race
- **대응**:
  - Phase 2 에서 모든 `a.config` 읽기 지점 식별 및 락 적용
  - Phase 5 의 전용 race test
  - `go test -race` CI 강제

### 10.3 위험: 에이전트별 분류 불일치

- **위험도**: 중간
- **시나리오**: 한 에이전트의 `ConnectionKeys` 에 누락된 키가 있어 실제로는 connection 이지만 operation 으로 오분류
- **대응**:
  - Phase 1 의 테스트에서 R4.1, R5.1, R5.3, R5.5, R5.7 의 표와 정확히 일치하는지 검증
  - 코드 리뷰 체크리스트에 "모든 ParseXxxConfig 의 키를 분류했는가?" 추가
  - Phase 4 회귀 테스트로 각 에이전트의 핵심 키 (host, port, baud_rate) 변경 시 Restart 가 트리거되는지 확인

### 10.4 위험: Non-transport 에이전트의 의도치 않은 Restart

- **위험도**: 낮음
- **시나리오**: `ConnectionConfigChecker` 미구현 에이전트 (Modbus, MQTT 등) 가 보수적 Restart 로 인해 기존 동작이 변함
- **대응**:
  - `ConfigureAgent` 의 fallback 로직을 **기존 로직과 동일하게 동작하도록** 설계 (즉, 과거 `transportKeys` 에서 Modbus 등이 어떻게 처리됐는지 분석 후 동일 결과가 나오도록)
  - 또는 미구현 에이전트에 대해서는 기존 로직을 그대로 유지 (일부 분기)
  - characterization test 로 Modbus 등에 대한 기존 ConfigureAgent 호출이 동일하게 동작함을 보장

### 10.5 위험: Serial 의 gap_timeout 핫 리로드 복잡성

- **위험도**: 낮음
- **시나리오**: gap_timeout 변경 시 이미 열린 포트에 어떻게 반영할지 불명확
- **대응**:
  - 최소 보장: 다음 read 주기부터 새 값 사용 (간단한 구현)
  - 최대 보장 (선택): Configure 내부에서 포트에 `SetReadTimeout` 재호출
  - Phase 2 Serial 구현 시 결정

---

## 11. 구현 순서 요약

```
Phase 1 (도메인 모델)
    ↓
Phase 2 (에이전트 Configure 수정)
    ↓
Phase 3 (서비스 어댑터 재설계)
    ↓
Phase 4 (회귀 테스트 스위트)
    ↓
Phase 5 (동시성 검증)
    ↓
Phase 6 (통합 시나리오)
    ↓
Phase 7 (문서 + 최종 검증)
```

- Phase 1 은 전체의 전제 조건.
- Phase 2 와 Phase 3 는 의존 관계상 순차적.
- Phase 4, 5 는 Phase 3 이후 독립적 진행 가능.
- Phase 6, 7 은 최종 단계.

---

## 12. 기술적 접근

### 12.1 패턴 일관성

본 SPEC 은 SPEC-AGENT-005 와 유사한 접근 (옵셔널 동작을 인터페이스로 위임) 을 사용한다:

| SPEC-AGENT-005 (Enable/Disable) | SPEC-AGENT-006 (Connection/Operation) |
|--------------------------------|----------------------------------------|
| `AgentConfig.IsEnabled()` | `ConnectionConfigChecker.IsConnectionChange()` |
| `*bool` 포인터 + nil 기본값 | 옵셔널 인터페이스 + fallback |
| Manager 가 IsEnabled 로 분기 | ConfigureAgent 가 IsConnectionChange 로 분기 |
| 기존 NodeDef 패턴 재사용 | 기존 Agent 인터페이스에 새 메서드 강제 안 함 |

### 12.2 Go 코드 스타일

- 모든 신규 함수/메서드에 godoc 주석 추가 (한국어)
- error wrapping 사용 (`fmt.Errorf("configure tcp client: %w", err)`)
- `context.Context` 를 첫 인자로 (Service 메서드)
- table-driven test 우선
- `t.Parallel()` 적용 (독립 테스트)
- 뮤텍스 명명: 기존 `a.mu` 패턴 유지
- atomic 사용 시: `atomic.Pointer[T]` (Go 1.19+) 우선, 단순성 위해 sync.Mutex 우선

### 12.3 마이그레이션 전략

**없음 (No Migration)**:

- 저장소 스키마 변경 없음
- API 응답 구조 변경 없음
- Web UI 변경 없음
- 기존 코드는 `ConnectionConfigChecker` 를 구현하지 않아도 컴파일 가능 (옵셔널 인터페이스)

---

## 13. 완료 정의 (Definition of Done)

본 SPEC 의 구현이 완료되었다고 판정하는 기준:

- [ ] 모든 EARS 요구사항 (R1.1 ~ R7.7) 구현
- [ ] 모든 Phase (1~7) 완료
- [ ] 신규 코드 테스트 커버리지 ≥ 85%
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] `gofmt -l .` 출력 없음
- [ ] `golangci-lint run` 통과
- [ ] 6개 통합 시나리오 통과
- [ ] 5개 에이전트 모두 회귀 테스트 추가
- [ ] 기존 매니저 Restart 경로 동작 보존 검증
- [ ] 기존 데몬 재시작 경로 동작 보존 검증
- [ ] CHANGELOG.md 업데이트
- [ ] PR 머지 및 SPEC status: `draft` → `completed`
