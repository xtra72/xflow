---
id: SPEC-AGENT-004
version: "2.0.0"
status: in-progress
created: "2026-04-07"
updated: "2026-04-07"
---

# SPEC-AGENT-004: Agent Statistics Enhancement - 구현 계획

## 개요

에이전트 통계 시스템을 고도화하여 외부/내부 메시지 분리, 연결 단위별 통계, 노드 참조별 통계를 제공한다.

---

## Milestone 1: 코어 통계 확장 — ✅ 완료

StatsSnapshot 및 AgentStats 구조체를 확장하여 새 카운터를 추가한다.

### 완료 작업

1. **S1: StatsSnapshot 구조체 확장** ✅
   - 외부/내부 메시지 카운터 필드 추가
   - `DroppedMessages`, `LoadTime` 필드 추가
   - `Incr*` / `Add*` 메서드 제공 (합산 일관성 보장 패턴)

2. **기존 카운터 호환성 유지** ✅
   - `Incr*External*()` / `Incr*Internal*()` 메서드가 총 카운터도 동시 증가
   - 개별 `IncrMessages*()` 와 동시 사용 금지 (이중 카운트 방지 규칙)

3. **테스트** ✅
   - 기존 테스트 통과 (`go test -race`)
   - LGCP 에이전트 통계 검증 테스트 추가

### 영향 파일

- `internal/agent/info.go`

---

## Milestone 2: ConnectionStatsProvider 인터페이스 — 미구현

에이전트 타입별 외부 연결 통계를 위한 인터페이스를 정의한다.

### 작업 목록

1. **S2: 인터페이스 및 구조체 정의**
   - `internal/agent/` 패키지에 `ConnectionStats` 구조체 정의
   - `ConnectionStatsProvider` 인터페이스 정의
   - 인터페이스 문서화

2. **에이전트 타입별 구현 (선택적)**
   - 각 에이전트 타입에서 `ConnectionStatsProvider` 인터페이스 구현
   - 에이전트 타입별 연결 식별자 매핑 (토픽, 클라이언트 주소, 장치 주소 등)

3. **테스트 작성**
   - 인터페이스 구현 검증 테스트
   - 연결 통계 스냅샷 정확성 테스트

### 영향 파일

- `internal/agent/agent.go` (인터페이스 정의)
- 에이전트 타입별 파일 (구현)

---

## Milestone 3: NodeRefStats 수집 — ✅ 완료

노드 참조별 내부 통계 수집 메커니즘을 구현한다.

### 완료 작업

1. **S3: NodeRefStats 구조체 및 수집 로직** ✅
   - `NodeRefStats` 구조체 정의
   - `IncrNodeRefSent(nodeID, flowID)` 메서드 구현
   - `NodeRefStatsSnapshot() []NodeRefStats` 메서드 구현

2. **호출 지점 통합** ✅
   - LGCP `processGetRecent`: 신규 프레임 반환 시 `IncrNodeRefSent` 호출
   - LGCP `processDrain`: 소비 프레임 반환 시 `IncrNodeRefSent` 호출
   - 노드가 `node_id` 를 요청에 포함

### 영향 파일

- `internal/agent/info.go`
- `internal/agent/lg/lgcp_agent.go`
- `internal/node/lgcp.go`

---

## Milestone 4: API 응답 구조 확장 — 미구현

통계 API 엔드포인트의 응답 구조를 중첩 JSON으로 개선한다.

### 작업 목록

1. **S4: 핸들러 응답 구조체 확장**
   - `internal/api/handler/agent.go`에 `MessageCounters`, `EnhancedMessagesStats`, `BytesStats`, `BufferStats` 구조체 추가
   - `ConnectionStatsResponse`, `NodeRefStatsResponse` 구조체 추가
   - `AgentStatsInfo` 구조체를 새 중첩 구조로 확장

2. **서비스 어댑터 매핑**
   - `internal/api/service/agent_adapter.go`의 `AgentStats()` 메서드에서 새 필드 매핑
   - `ConnectionStatsProvider` 타입 어설션으로 연결 통계 포함
   - `NodeRefStatsSnapshot()` 호출로 노드 참조 통계 포함

3. **테스트 작성**
   - `internal/api/service/agent_adapter_test.go` 확장
   - JSON 직렬화 포맷 검증 테스트
   - `ConnectionStatsProvider` 미구현 에이전트의 빈 배열 반환 테스트

### 영향 파일

- `internal/api/handler/agent.go`
- `internal/api/service/agent_adapter.go`
- `internal/api/service/agent_adapter_test.go`

---

## Milestone 5: 프론트엔드 업데이트 — 미구현

Agent Detail Panel에 새 통계 정보를 표시한다.

### 작업 목록

1. **TypeScript 타입 정의**
   - 새 API 응답 구조에 대응하는 TypeScript 인터페이스 정의
   - 기존 타입과의 하위 호환성 유지

2. **Agent Detail Panel 업데이트**
   - `web/src/pages/agents/AgentDetailPanel.tsx` 수정
   - 외부/내부 메시지 통계 분리 표시 UI
   - 드롭된 메시지 수 및 로드 시간 표시
   - 연결 단위별 상세 통계 테이블
   - 노드 참조별 통계 테이블

3. **Zustand Store 업데이트**
   - 통계 데이터 페칭 및 상태 관리 업데이트

### 영향 파일

- `web/src/pages/agents/AgentDetailPanel.tsx`
- `web/src/types/` (타입 정의)

---

## Milestone 6: 통합 테스트 및 검증 — 미구현

전체 통계 파이프라인의 end-to-end 검증을 수행한다.

### 작업 목록

1. **통합 테스트**
   - 에이전트 생성 -> 메시지 송수신 -> 통계 API 조회 시나리오 테스트
   - 외부/내부 메시지 카운터 합산 일관성 검증
   - 동시성 부하 테스트

2. **성능 검증**
   - 통계 수집 오버헤드 벤치마크 (1ms 이내 목표)
   - 대량 연결 시 `ConnectionStats` 조회 성능 측정

3. **하위 호환성 검증**
   - 기존 API 소비자가 새 응답에서도 정상 동작하는지 확인

---

## Milestone 7: LGCP 멀티 노드 + Bridge Guard — ✅ 완료 (신규)

LGCP 에이전트의 멀티 노드 독립 소비 및 bridge 소비자 관리를 구현한다.

### 완료 작업

1. **S6: last_seq 서버사이드 필터링** ✅
   - `lgcpFrameRecord`에 `Seq` 필드 추가
   - `lgcpProcessRequest`에 `LastSeq` 필드 추가
   - `processGetRecent`에서 `seq > lastSeq` 필터링
   - 노드에서 `last_seq` 전송

2. **S7: bridgeActive Guard** ✅
   - `bridgeActive atomic.Bool` 가드
   - `handleCapturedFrame`, `sendStatusEvent`에 적용

3. **기본 poll_command 변경** ✅
   - `drain` → `get_recent` (멀티 노드 호환)

### 영향 파일

- `internal/agent/lg/lgcp_agent.go`
- `internal/agent/lg/lgcp_agent_test.go`
- `internal/node/lgcp.go`
- `internal/node/lgcp_test.go`

---

## 기술적 접근

### 아키텍처 설계 방향

- **계층 분리**: 통계 수집(agent 패키지) -> 스냅샷 변환(service 패키지) -> JSON 직렬화(handler 패키지)
- **인터페이스 기반 확장**: `ConnectionStatsProvider`로 에이전트 타입별 선택적 구현
- **lock-free 카운터**: `sync/atomic`으로 hot path 성능 보장
- **읽기 병행성**: `sync.RWMutex`로 NodeRefStats 읽기 동시 접근 허용
- **서버사이드 필터링**: `last_seq` 기반으로 에이전트가 신규 프레임만 반환 (정확한 카운트 + 멀티노드)
- **합산 보장 패턴**: `Incr*External*`/`Incr*Internal*` 메서드가 총 카운터를 내부적으로 함께 증가

### 위험 요소 및 대응

| 위험 | 영향 | 대응 | 상태 |
|------|------|------|------|
| atomic 카운터 합산 불일치 | 외부+내부 != 전체 | Incr 메서드에서 전체 카운터도 동시 증가 | ✅ 해결 |
| 이중 카운트 | 송신/수신 수 2배 | Incr*External/Internal 과 IncrMessages* 동시 호출 금지 | ✅ 해결 |
| get_recent 반복 카운트 폭증 | 초당 수백배 카운트 | last_seq 서버사이드 필터링으로 신규만 반환/카운트 | ✅ 해결 |
| bridge 없는 상태에서 msgCh 누적 | 메모리 누수 | bridgeActive guard로 소비자 없으면 적재 안함 | ✅ 해결 |
| ConnectionStats 메모리 증가 | 대량 연결 시 메모리 사용 | 최대 연결 수 제한 또는 오래된 연결 정리 | 미구현 |
| API 하위 호환성 깨짐 | 기존 클라이언트 오류 | 기존 flat 필드 유지, 새 중첩 구조 추가 | 미구현 |

---

## 개발 방법론

Hybrid 모드 적용:

- **새 코드** (ConnectionStatsProvider, NodeRefStats, 새 API 구조체): TDD (RED-GREEN-REFACTOR)
- **기존 코드 수정** (StatsSnapshot 확장, AgentStats 확장, adapter 수정): DDD (ANALYZE-PRESERVE-IMPROVE)

커버리지 목표: 85% 이상

---

## 진행 상황 요약

| Milestone | 상태 | 비고 |
|-----------|------|------|
| M1: 코어 통계 확장 | ✅ 완료 | 외부/내부 분리, 운영 카운터 |
| M2: ConnectionStatsProvider | 미구현 | 에이전트 타입별 연결 통계 |
| M3: NodeRefStats | ✅ 완료 | LGCP 에이전트 노드별 통계 |
| M4: API 응답 구조 | 미구현 | 중첩 JSON 응답 |
| M5: 프론트엔드 | 미구현 | Agent Detail Panel |
| M6: 통합 테스트 | 미구현 | E2E 검증 |
| M7: LGCP 멀티노드 + Bridge | ✅ 완료 | last_seq 필터링, bridgeActive |
