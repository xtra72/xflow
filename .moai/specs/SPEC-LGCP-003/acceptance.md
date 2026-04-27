---
id: SPEC-LGCP-003
type: acceptance
version: "1.0.0"
created: "2026-04-06"
updated: "2026-04-06"
---

# SPEC-LGCP-003 수락 기준: LGCP 플로우 노드

## 1. 공통 기반 (lgcpNodeBase) 검증

### AC-001: LGCPNodeConfig 필수 필드 검증

**Given** lgcpNodeBase가 초기화될 때
**When** agent_ref가 빈 문자열이거나 누락된 config로 configure()를 호출하면
**Then** ErrLGCPMissingAgentRef 에러를 반환해야 한다

### AC-002: LGCPNodeConfig 기본값 적용

**Given** lgcpNodeBase가 초기화될 때
**When** 선택 필드(poll_interval, timeout, poll_command, recent_count)를 생략한 config로 configure()를 호출하면
**Then** 기본값이 적용되어야 한다:
- poll_interval: "30s"
- timeout: "5s"
- poll_command: "get_stats"
- recent_count: 10

### AC-003: LGCPNodeConfig 전체 필드 파싱

**Given** lgcpNodeBase가 초기화될 때
**When** 모든 필드가 포함된 config로 configure()를 호출하면
**Then** 각 필드가 올바르게 파싱되어야 한다:
- agent_ref: 설정값 그대로
- default_address: hex 문자열 그대로
- poll_interval: duration 파싱 가능
- timeout: duration 파싱 가능
- poll_command: "get_stats" 또는 "get_recent"
- recent_count: 정수값

### AC-004: 에이전트 타입 체크

**Given** AgentResolver가 설정되어 있고 에이전트가 resolve 가능할 때
**When** resolve된 에이전트가 `*lg.LGCPAgent` 타입이 아니면
**Then** ErrLGCPAgentNotLGCP 에러를 반환해야 한다

### AC-005: AgentResolver 미설정 검증

**Given** lgcpNodeBase에 AgentResolver가 nil일 때
**When** initAgent()를 호출하면
**Then** ErrLGCPNoResolver 에러를 반환해야 한다

### AC-006: callAgentProcess timeout 적용

**Given** 에이전트가 초기화되어 있고 timeout이 "2s"로 설정되어 있을 때
**When** callAgentProcess()를 호출하면
**Then** 2초 후에도 응답이 없으면 context deadline exceeded 에러를 반환해야 한다

---

## 2. LGCPStatusNode 검증

### AC-007: 팩토리 함수 및 인터페이스 준수

**Given** 유효한 NodeDef가 준비되어 있을 때
**When** NewLGCPStatusNode()를 호출하면
**Then** Node 및 SourceNode 인터페이스를 모두 구현하는 인스턴스를 반환해야 한다

### AC-008: Configure 성공

**Given** LGCPStatusNode 인스턴스가 생성되었을 때
**When** agent_ref와 poll_interval이 포함된 config로 Configure()를 호출하면
**Then** 에러 없이 설정이 적용되어야 한다

### AC-009: get_stats 폴링 커맨드 생성

**Given** poll_command가 "get_stats"로 설정되어 있을 때
**When** buildLGCPStatusCommand()가 호출되면
**Then** `{"command": "get_stats"}` JSON을 생성해야 한다

### AC-010: get_recent 폴링 커맨드 생성

**Given** poll_command가 "get_recent"이고 recent_count가 20으로 설정되어 있을 때
**When** buildLGCPStatusCommand()가 호출되면
**Then** `{"command": "get_recent", "count": 20}` JSON을 생성해야 한다

### AC-011: Process on-demand 상태 조회

**Given** LGCPStatusNode가 초기화되어 있고 에이전트가 연결되어 있을 때
**When** Process()에 입력 메시지가 전달되면
**Then** 에이전트에 상태 조회 커맨드를 전송하고, 결과를 출력 메시지에 설정하여 반환해야 한다

### AC-012: Process 오버라이드 적용

**Given** LGCPStatusNode가 poll_command="get_stats"로 설정되어 있을 때
**When** payload에 poll_command="get_recent"과 count=5가 포함된 메시지로 Process()를 호출하면
**Then** get_recent 커맨드를 count=5로 에이전트에 전송해야 한다

### AC-013: SourceCh 메시지 생성

**Given** LGCPStatusNode가 Init되어 폴링이 시작되었을 때
**When** 폴링 주기가 도래하면
**Then** SourceCh()에서 반환된 채널에 상태 조회 결과 메시지가 전달되어야 한다

### AC-014: 메타데이터 설정

**Given** LGCPStatusNode가 폴링 또는 Process로 메시지를 생성할 때
**When** 출력 메시지가 생성되면
**Then** 메타데이터에 `lgcp_source`("poll" 또는 "request")와 `lgcp_node_id`가 설정되어야 한다

### AC-015: Shutdown 시 폴링 중지

**Given** LGCPStatusNode가 폴링 중일 때
**When** Shutdown()을 호출하면
**Then** 폴링 고루틴이 정상 종료되어야 하고, 이중 호출 시에도 패닉이 발생하지 않아야 한다

---

## 3. LGCPControlNode 검증

### AC-016: 팩토리 함수 및 인터페이스 준수

**Given** 유효한 NodeDef가 준비되어 있을 때
**When** NewLGCPControlNode()를 호출하면
**Then** Node 인터페이스를 구현하는 인스턴스를 반환해야 한다 (SourceNode 아님)

### AC-017: 직접 커맨드 제어

**Given** LGCPControlNode가 초기화되어 있고 default_address가 설정되어 있을 때
**When** payload에 `{"command": "set_power", "address": "00000001", "params": {"power": "ON"}}`이 포함된 메시지로 Process()를 호출하면
**Then** 해당 JSON 커맨드를 에이전트에 전달하고 결과를 반환해야 한다

### AC-018: set_multiple 자동 구성

**Given** LGCPControlNode가 초기화되어 있을 때
**When** payload에 `{"address": "00000001", "power": "ON", "temperature": 24}`가 포함된 메시지로 Process()를 호출하면
**Then** `{"command": "set_multiple", "address": "00000001", "params": {"power": "ON", "temperature": 24}}` 커맨드를 구성하여 전달해야 한다

### AC-019: default_address 폴백

**Given** LGCPControlNode의 default_address가 "00000002"로 설정되어 있을 때
**When** payload에 address가 없고 `{"power": "OFF"}`만 있는 메시지로 Process()를 호출하면
**Then** default_address "00000002"를 사용하여 제어 커맨드를 전송해야 한다

### AC-020: address 누락 에러

**Given** LGCPControlNode의 default_address가 설정되어 있지 않을 때
**When** payload에 address가 없고 제어 키만 있는 메시지로 Process()를 호출하면
**Then** ErrLGCPMissingAddress를 포함한 에러를 반환해야 한다

### AC-021: 제어 결과 메타데이터

**Given** LGCPControlNode가 제어 명령을 성공적으로 처리했을 때
**When** 출력 메시지가 생성되면
**Then** 메타데이터에 `lgcp_command`("control")와 `lgcp_node_id`가 설정되어야 한다

---

## 4. LGCPNode (통합) 검증

### AC-022: 팩토리 함수 및 인터페이스 준수

**Given** 유효한 NodeDef가 준비되어 있을 때
**When** NewLGCPNode()를 호출하면
**Then** Node 및 SourceNode 인터페이스를 모두 구현하는 인스턴스를 반환해야 한다

### AC-023: 제어 키 자동 감지 - 제어 모드

**Given** LGCPNode가 초기화되어 있을 때
**When** payload에 `{"address": "00000001", "temperature": 25}`가 포함된 메시지로 Process()를 호출하면
**Then** 제어 명령으로 처리하여 set_multiple 커맨드를 에이전트에 전달해야 한다

### AC-024: 제어 키 자동 감지 - 상태 조회 모드

**Given** LGCPNode가 초기화되어 있을 때
**When** payload에 제어 키(power, mode, temperature, fan_speed)가 없는 메시지로 Process()를 호출하면
**Then** 상태 조회 명령으로 처리해야 한다

### AC-025: 통합 노드 메타데이터

**Given** LGCPNode가 Process()를 처리했을 때
**When** 출력 메시지가 생성되면
**Then** 메타데이터에 `lgcp_command`("control" 또는 "status")와 `lgcp_node_id`가 설정되어야 한다

### AC-026: 통합 노드 폴링

**Given** LGCPNode가 Init되어 폴링이 시작되었을 때
**When** 폴링 주기가 도래하면
**Then** 상태 조회 커맨드로 폴링하여 SourceCh에 메시지를 전달해야 한다

---

## 5. 에러 정의 검증

### AC-027: 센티널 에러 정의 확인

**Given** `internal/node/errors.go` 파일이 존재할 때
**When** LGCP 에러 변수를 확인하면
**Then** 다음 5종이 정의되어 있어야 한다:
- ErrLGCPAgentNotLGCP: ErrInvalidConfig를 wrapping
- ErrLGCPMissingAgentRef: ErrInvalidConfig를 wrapping
- ErrLGCPNoResolver: ErrNodeNotInitialized를 wrapping
- ErrLGCPProcessFailed: 독립 에러
- ErrLGCPMissingAddress: ErrInvalidConfig를 wrapping

---

## 6. 레지스트리 등록 검증

### AC-028: 빌트인 노드 등록 확인

**Given** Registry가 기본 옵션으로 생성되었을 때
**When** 등록된 노드 타입을 조회하면
**Then** 다음 3종이 등록되어 있어야 한다:
- "lgcp-status": category "io"
- "lgcp-control": category "io"
- "lgcp": category "io"

### AC-029: 팩토리 함수 동작 확인

**Given** Registry에 lgcp-status가 등록되어 있을 때
**When** 해당 팩토리로 노드를 생성하면
**Then** 에러 없이 Node 인스턴스가 반환되어야 한다

---

## 7. 헬퍼 함수 검증

### AC-030: hasLGCPControlKeys 감지

**Given** 메시지 payload에 "temperature": 24가 있을 때
**When** hasLGCPControlKeys()를 호출하면
**Then** true를 반환해야 한다

### AC-031: hasLGCPControlKeys 미감지

**Given** 메시지 payload에 제어 키가 없고 "status": "ok"만 있을 때
**When** hasLGCPControlKeys()를 호출하면
**Then** false를 반환해야 한다

### AC-032: applyLGCPOverrides 오버라이드 적용

**Given** 기본 LGCPNodeConfig가 있을 때
**When** payload에 address="00000003", timeout="10s"가 포함된 메시지로 applyLGCPOverrides()를 호출하면
**Then** 반환된 config의 DefaultAddress가 "00000003"이고 Timeout이 "10s"여야 한다

---

## 8. 품질 게이트

### QG-001: 테스트 커버리지

- lgcp.go 파일의 테스트 커버리지가 85% 이상이어야 한다

### QG-002: 컴파일 검증

- `go build ./...` 가 에러 없이 통과해야 한다

### QG-003: 테스트 통과

- `go test -race ./internal/node/...` 가 모든 테스트를 통과해야 한다

### QG-004: vet 검사

- `go vet ./internal/node/...` 가 경고 없이 통과해야 한다

### QG-005: 기존 테스트 비회귀

- 기존 LGAP 노드 테스트를 포함한 모든 기존 테스트가 통과해야 한다

---

## 9. Definition of Done

- [ ] `internal/node/errors.go`에 LGCP 센티널 에러 5종 추가
- [ ] `internal/node/lgcp.go`에 lgcpNodeBase, LGCPStatusNode, LGCPControlNode, LGCPNode 구현
- [ ] `internal/node/registry.go`에 3종 노드 빌트인 등록
- [ ] `internal/node/lgcp_test.go`에 테스트 구현 (커버리지 85%+)
- [ ] `go test -race ./internal/node/...` 통과
- [ ] `go vet ./internal/node/...` 통과
- [ ] 기존 테스트 비회귀 확인

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-06*
*작성: MoAI SPEC Builder (manager-spec)*
