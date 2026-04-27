---
id: SPEC-NODE-003
version: "1.0.0"
status: draft
created: "2026-04-10"
updated: "2026-04-10"
author: xtra
priority: high
tags: [node, tcp, metadata, connection, framing]
related_spec: SPEC-NODE-002, SPEC-NODE-001, SPEC-SOCKET-001
---

# SPEC-NODE-003: Implementation Plan - TCP 소스 노드 connection_id 메타데이터 주입

## 1. 구현 개요

본 계획은 SPEC-NODE-003 의 요구사항 (M1~M4) 을 3개의 구현 단계 (Phase 0 ~ Phase 2) 로 분할한다. 본 SPEC 은 소규모 변경 (핵심 변경은 receiveLoop 에 메타데이터 설정 추가) 이므로 전체 구현 범위가 작다.

### 1.1 개발 방법론

- **Methodology**: Hybrid (`.moai/config/sections/quality.yaml` 참조)
  - Phase 0 (기존 동작 분석): DDD - ANALYZE 단계
  - Phase 1 (connection_id 주입): `internal/node/tcp_io.go` 의 기존 코드 수정이므로 DDD (ANALYZE-PRESERVE-IMPROVE). 기존 동작을 characterization test 로 보존한 후 변경.
  - Phase 2 (통합 테스트): framer 노드와의 통합은 TDD (RED-GREEN)
- **Test Coverage Target**: 변경 코드 >= 85%

### 1.2 우선순위

| 우선순위 | 단계 | 설명 |
|---------|------|------|
| Primary Goal | Phase 0, 1 | 기존 동작 분석 + connection_id 주입 구현 |
| Secondary Goal | Phase 2 | framer 노드 통합 테스트 + 문서화 |

> **시간 추정 금지**: 본 계획은 우선순위 기반이다.

---

## 2. 사전 설계 결정 (Pre-Implementation Decisions)

### 2.1 결정 (a): connection_id 값 형식

**선택지 a1 - remoteAddr 그대로 사용**:

- 서버 모드: `"192.168.1.100:51234"` (host:port)
- 장점: 사람이 읽을 수 있음, 디버깅 용이, 추가 변환 불필요
- 단점: NAT 환경에서 포트 재사용 시 동일 값이 나타날 수 있음 (그러나 이전 연결은 이미 종료)

**선택지 a2 - UUID 생성**:

- 서버 모드: 연결마다 UUID v4 생성
- 장점: 절대 충돌 없음
- 단점: 디버깅 시 어떤 클라이언트인지 파악 어려움, 에이전트 레벨에서 연결-UUID 매핑 필요, 현재 `ConnAwareReceiver` 인터페이스는 UUID 를 제공하지 않으므로 인터페이스 변경 필요

**선택지 a3 - remoteAddr + 타임스탬프 조합**:

- 서버 모드: `"192.168.1.100:51234@1712750400"` (host:port@unix_seconds)
- 장점: NAT 재사용 구분 가능
- 단점: 같은 연결 내에서도 매 메시지마다 동일 값을 생성하려면 연결 시작 시각을 추적해야 하는데, 현재 `ConnAwareReceiver` 는 이를 제공하지 않음

**결정**: **a1 선택 (remoteAddr 그대로, 권장)**. 사유: (1) 가장 단순하며 추가 인터페이스 변경이 불필요. (2) `remoteAddr` 는 이미 `tcp.remote_addr` 로 설정 중이므로 동일 값을 `connection_id` 에도 사용하는 것은 자연스럽다. (3) NAT 포트 재사용 문제는 framer 노드의 `stream_idle_timeout` 으로 이전 스트림이 정리된 후 동일 키로 새 스트림이 생성되므로 실질적 문제 없다. (4) `tcp.remote_addr` 과 `connection_id` 가 동일한 값을 가지는 중복이 발생하지만, 두 키의 의미가 다르다 (`tcp.remote_addr` 은 TCP 프로토콜 전용 라우팅 키, `connection_id` 는 범용 스트림 식별자).

### 2.2 결정 (b): TCP 클라이언트 모드의 connection_id 값

**선택지 b1 - 노드 ID 사용**:

- `connection_id` = `n.ID()` (예: `"tcp-in-1"`)
- 장점: 단순, 고유 (한 플로우 내에서 노드 ID 는 고유), 사람이 읽을 수 있음
- 단점: 연결 재설정 시 같은 값이 사용됨 (단일 연결이므로 문제 없음)

**선택지 b2 - 고정 문자열 "default"**:

- `connection_id` = `"default"`
- 장점: 더 단순
- 단점: 여러 tcp-in 클라이언트 노드가 같은 framer 를 공유하는 경우 충돌 (그러나 framer 노드는 단일 입력 포트이므로 이 케이스는 발생하지 않음)

**선택지 b3 - 빈 문자열 (미설정)**:

- 클라이언트 모드에서는 `connection_id` 를 설정하지 않음
- 장점: 가장 단순
- 단점: 메타데이터 규약의 일관성 저하. framer 노드는 키 없음 → 공용 버퍼로 정상 동작하지만, 메타데이터에서 소스를 구분할 방법이 없음

**결정**: **b1 선택 (노드 ID 사용, 권장)**. 사유: (1) 일관성: 서버/클라이언트 모두 `connection_id` 메타데이터가 존재하므로 downstream 에서 메타데이터 기반 처리를 할 때 예측 가능. (2) 디버깅: 로그에서 어떤 노드에서 온 메시지인지 식별 가능. (3) framer 노드와의 동작: 클라이언트 모드는 단일 연결이므로 모든 메시지가 같은 `connection_id` → 단일 스트림 버퍼로 동작 (기존과 동일한 결과).

### 2.3 결정 (c): connection_id 설정 위치

**선택지 c1 - receiveLoop 내부 (노드 계층)**:

- `TCPInNode.receiveLoop` 에서 메시지 생성 직후 설정
- 장점: 에이전트 계층 변경 불필요, 노드가 자신의 메타데이터를 책임짐
- 단점: 없음 (현재 `tcp.remote_addr` 도 같은 위치에서 설정)

**선택지 c2 - 에이전트 계층**:

- `ConnAwareReceiver` 인터페이스를 확장하여 `connection_id` 를 별도 반환
- 장점: 에이전트가 연결 식별자를 완전 제어
- 단점: 인터페이스 변경은 모든 구현체에 영향, 본 SPEC 의 범위를 초과

**결정**: **c1 선택 (receiveLoop 내부, 권장)**. 사유: 최소 변경 원칙. `tcp.remote_addr` 설정과 동일한 위치에 1줄 추가.

---

## 3. Phase 0: 기존 동작 분석 (DDD - ANALYZE)

### 3.1 목표

`TCPInNode.receiveLoop` 의 현재 동작을 정확히 파악하고, 기존 테스트를 characterization test 기준으로 확인한다.

### 3.2 분석 대상

1. `internal/node/tcp_io.go`:
   - receiveLoop 의 메타데이터 설정 순서와 조건
   - ConnAwareReceiver 경로와 MessageReceiver 경로의 분기
   - 메시지 생성 패턴 (`message.New()`, `Payload.Set`, `Metadata.Set`)

2. `internal/node/tcp_io_test.go` (존재 시):
   - 기존 테스트의 커버리지 확인
   - 메타데이터 관련 assertion 확인
   - characterization test 로 사용 가능한 테스트 식별

3. `TCPOutNode.Process`:
   - `tcp.remote_addr` 메타데이터를 읽어 응답 라우팅에 사용
   - `connection_id` 추가가 이 경로에 영향을 주지 않는지 확인

### 3.3 완료 조건

- [ ] receiveLoop 의 서버/클라이언트 분기 동작 파악 완료
- [ ] 기존 테스트 현황 파악 및 커버리지 확인
- [ ] `TCPOutNode` 가 `connection_id` 를 사용하지 않는지 확인

---

## 4. Phase 1: connection_id 주입 구현 (DDD - PRESERVE + IMPROVE)

### 4.1 목표

`TCPInNode.receiveLoop` 에 `connection_id` 메타데이터를 주입한다. 기존 동작은 보존하면서 새 메타데이터만 추가한다.

### 4.2 변경 파일

#### 수정

- `internal/node/tcp_io.go`: receiveLoop 에 `connection_id` 설정 추가

#### 신규

- `internal/node/tcp_io_test.go` (기존 파일 확장 또는 신규): connection_id 관련 테스트

### 4.3 DDD 단계

1. **PRESERVE (기존 동작 보존)**:
   - 기존 메타데이터 (`tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type`) 가 정확히 유지되는 characterization test 작성 (이미 존재하면 확인)
   - 서버 모드와 클라이언트 모드 각각에 대해 메타데이터 스냅샷 테스트

2. **IMPROVE (connection_id 추가)**:
   - receiveLoop 의 서버 모드 분기에 추가:
     ```go
     // 연결 정보를 메타데이터에 저장 (응답 라우팅에 사용)
     if remoteAddr != "" {
         msg.Metadata().Set("tcp.remote_addr", remoteAddr)
         msg.Metadata().Set("connection_id", remoteAddr)  // *** NEW ***
     }
     ```
   - receiveLoop 의 클라이언트 모드 분기에 추가:
     ```go
     // ConnAwareReceiver 가 아닌 경우 (클라이언트 모드)
     if remoteAddr == "" {
         msg.Metadata().Set("connection_id", n.ID())  // *** NEW ***
     }
     ```
   - 변경 후 기존 characterization test 가 여전히 통과하는지 확인
   - 새로운 connection_id 관련 테스트 추가

### 4.4 테스트 (TDD for new assertions)

1. **RED**:
   - `TestTCPInNode_ServerMode_SetsConnectionID`: ConnAwareReceiver mock → `connection_id` == `remoteAddr`
   - `TestTCPInNode_ServerMode_ConnectionID_MatchesRemoteAddr`: `connection_id` 와 `tcp.remote_addr` 가 동일 값
   - `TestTCPInNode_ClientMode_SetsConnectionID`: MessageReceiver mock → `connection_id` == `n.ID()`
   - `TestTCPInNode_ServerMode_PreservesExistingMetadata`: `tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type` 가 여전히 존재
   - `TestTCPInNode_MultiClient_DifferentConnectionIDs`: 두 클라이언트 → 서로 다른 `connection_id`

2. **GREEN**: Phase 4.3 의 코드 변경으로 모든 테스트 통과

3. **REFACTOR**: 메타데이터 설정 코드가 길어지면 헬퍼 함수 추출 (불필요할 경우 생략)

### 4.5 의존성

- 선행: Phase 0
- 후속: Phase 2

### 4.6 완료 조건

- [ ] 서버 모드에서 `connection_id` = `remoteAddr` 설정
- [ ] 클라이언트 모드에서 `connection_id` = `n.ID()` 설정
- [ ] 기존 메타데이터 (`tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type`) 불변
- [ ] `go test -race ./internal/node/...` 통과
- [ ] 변경 코드 커버리지 >= 85%

---

## 5. Phase 2: framer 노드 통합 테스트 및 문서화

### 5.1 목표

`TCPInNode` (서버 모드) → `FramerNode` 파이프라인에서 `connection_id` 기반 다중 스트림 분리가 정상 동작하는지 통합 테스트를 작성한다. CHANGELOG 업데이트 및 SPEC status 를 완료로 전환한다.

### 5.2 변경 파일

#### 신규

- `internal/node/tcp_framer_integration_test.go` (또는 기존 framer_test.go 확장): 통합 테스트

#### 수정

- `CHANGELOG.md`: SPEC-NODE-003 entry 추가

### 5.3 TDD 단계

1. **RED**:
   - `TestIntegration_TCPServer_Framer_MultiClientFraming`:
     - Given: ConnAwareReceiver mock 이 두 클라이언트 (remoteAddr=A, B) 의 바이트를 교대로 반환
     - When: TCPInNode → FramerNode (framing=newline) 파이프라인 실행
     - Then: 각 클라이언트의 프레임이 독립적으로 조립됨, `frame.stream_key` 가 각각 remoteAddr=A, B

   - `TestIntegration_TCPClient_Framer_SingleStream`:
     - Given: MessageReceiver mock (클라이언트 모드)
     - When: TCPInNode → FramerNode (framing=newline) 파이프라인 실행
     - Then: 단일 스트림으로 프레이밍, `frame.stream_key` == `n.ID()`

2. **GREEN**: Phase 1 의 변경으로 통과

3. **REFACTOR**: 테스트 헬퍼 정리

### 5.4 문서 업데이트

- [ ] `CHANGELOG.md`: SPEC-NODE-003 entry 추가
  - "TCP 소스 노드에 connection_id 메타데이터 주입 - framer 노드와 결합하여 TCP 서버의 다중 클라이언트 연결별 독립 프레이밍 지원"
- [ ] SPEC-NODE-002 spec.md 의 구현 메모 (9.2 항목 5) 에 "SPEC-NODE-003 완료" 참조 추가 (선택적)

### 5.5 최종 검증

- [ ] `go build ./...` 성공
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] 기존 framer 노드 테스트 전체 통과 (회귀 없음)
- [ ] SPEC status: `draft` -> `completed`

### 5.6 완료 조건

- [ ] 통합 테스트 (서버 다중 클라이언트 + 클라이언트 단일 스트림) 통과
- [ ] CHANGELOG 업데이트
- [ ] 품질 게이트 통과

---

## 6. 위험 요소 및 대응

### 6.1 위험: NAT 환경에서 포트 재사용

- **위험도**: 낮음
- **시나리오**: NAT 뒤의 클라이언트가 연결을 끊고 같은 소스 포트로 재연결하면 `connection_id` 가 이전 연결과 동일
- **대응**: framer 노드의 `stream_idle_timeout` 이 이전 연결의 스트림 버퍼를 정리한 후 동일 키로 새 버퍼가 생성됨. 타임아웃 전에 재연결이 일어나면 이전 버퍼에 바이트가 추가되지만, 이는 TCP 연결 자체가 새로운 것이므로 새 바이트로 시작하여 프레이밍이 재시작된다. 실질적 문제는 극히 드물다.

### 6.2 위험: 기존 TCPOutNode 와의 간섭

- **위험도**: 없음
- **시나리오**: `TCPOutNode` 이 `connection_id` 를 읽어 잘못된 라우팅을 수행
- **대응**: `TCPOutNode` 는 `tcp.remote_addr` 만 읽으며 (`msg.Metadata().Get("tcp.remote_addr")`), `connection_id` 는 무시한다. 코드에서 확인 완료 (tcp_io.go:351).

### 6.3 위험: 테스트 mock 구현의 복잡성

- **위험도**: 낮음
- **시나리오**: `ConnAwareReceiver` mock 과 `AgentResolver` mock 의 조합이 복잡
- **대응**: 기존 `tcp_io_test.go` 의 mock 패턴을 재사용. 필요 시 테스트 헬퍼를 분리하여 복잡도 관리.

---

## 7. 구현 순서 요약

```
Phase 0 (기존 동작 분석 - DDD ANALYZE)
    |
Phase 1 (connection_id 주입 - DDD PRESERVE + IMPROVE)
    |
Phase 2 (통합 테스트 + 문서화)
```

- Phase 0 은 분석 단계이며 코드 변경 없음
- Phase 1 은 핵심 변경: receiveLoop 에 `connection_id` 설정 추가 (2줄)
- Phase 2 는 검증 및 마무리

---

## 8. 기술적 접근

### 8.1 변경 코드 의사 코드

```go
// internal/node/tcp_io.go - receiveLoop 내부

// 기존 코드 (유지)
msg := message.New()
msg.Payload().Set("raw", data)
msg.Payload().Set("data", hex.EncodeToString(data))
msg.Metadata().Set("tcp.node_id", n.ID())

// 연결 정보를 메타데이터에 저장
if remoteAddr != "" {
    msg.Metadata().Set("tcp.remote_addr", remoteAddr)
    msg.Metadata().Set("connection_id", remoteAddr)    // *** NEW: 서버 모드 ***
} else {
    msg.Metadata().Set("connection_id", n.ID())        // *** NEW: 클라이언트 모드 ***
}

if n.agent != nil {
    msg.Metadata().Set("tcp.agent_type", n.agent.Type())
}
```

### 8.2 Go 코드 스타일

- 기존 tcp_io.go 의 코드 스타일 유지 (한국어 주석)
- 변경 최소화: 기존 if 블록 내부에 1줄 추가 + else 블록 추가
- 테스트는 table-driven, `t.Parallel()` 적용

---

## 9. 완료 정의 (Definition of Done)

- [ ] M1 ~ M4 의 모든 EARS 요구사항 구현
- [ ] 서버 모드: `connection_id` = `remoteAddr`
- [ ] 클라이언트 모드: `connection_id` = `n.ID()`
- [ ] 기존 메타데이터 (`tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type`) 불변
- [ ] framer 노드 통합 테스트 통과 (다중 클라이언트 연결별 프레이밍)
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] 기존 테스트 회귀 없음
- [ ] CHANGELOG.md 업데이트
- [ ] SPEC status: `draft` -> `completed`
