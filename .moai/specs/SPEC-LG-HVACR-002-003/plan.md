---
id: SPEC-LG-HVACR-002-003
type: plan
version: "1.0.0"
created: "2026-04-06"
updated: "2026-05-29"
---

# SPEC-LG-HVACR-002-003 구현 계획: LG HVACR-02 플로우 노드

> **명명 규약 (v2.0 rename, 2026-05-29 이후)**: 프로토콜 `lg_icp02` (LG ICP-02) / 에이전트 `lg_hvacr02` (LG HVACR-02). 이전 SPEC ID: SPEC-LGCP-003.

## 1. 구현 전략

### 접근 방식

LGAP 노드(`internal/node/lgap.go`)를 참조 구현으로 삼아 LG HVACR-02 노드를 구현한다. 구조적으로 거의 동일하되, LG ICP-02 프로토콜 고유의 차이점(address 기반 제어, get_stats/get_recent 상태 조회, control 시 address 필수)을 반영한다.

### 핵심 설계 결정

1. **별도 파일 분리**: `lg_hvacr02.go`를 독립 파일로 생성하여 LGAP 코드와 분리 유지
2. **LGAP 패턴 미러링**: hvacr02NodeBase, LGHvacr02StatusNode, LGHvacr02ControlNode, LGHvacr02Node 구조를 LGAP과 동일하게 구성
3. **address 필수 검증**: LG ICP-02 제어 명령에는 address가 반드시 필요하므로, 누락 시 `ErrHvacr02MissingAddress` 반환
4. **poll_command 확장**: LGAP의 get_state/get_all_states 대신 get_stats/get_recent를 지원하며, poll_command 설정으로 선택 가능

---

## 2. 마일스톤

### Primary Goal: 에러 정의 및 공통 기반

**대상 파일**:
- `internal/node/errors.go` (수정)
- `internal/node/lg_hvacr02.go` (신규: hvacr02NodeBase, Hvacr02NodeConfig, 헬퍼 함수)

**작업 내용**:
1. `errors.go`에 LG HVACR-02 센티널 에러 5종 추가
   - `ErrHvacr02WrongAgentType`
   - `ErrHvacr02MissingAgentRef`
   - `ErrHvacr02NoResolver`
   - `ErrHvacr02ProcessFailed`
   - `ErrHvacr02MissingAddress`
2. `lg_hvacr02.go` 상수 정의 (hvacr02DefaultTimeout, hvacr02DefaultPollInterval, 커맨드 문자열)
3. `Hvacr02NodeConfig` 구조체 정의
4. `hvacr02NodeBase` 구조체 및 메서드 구현
   - `configure()`: 설정 파싱 (agent_ref 필수 검증, default_address, poll_command, recent_count)
   - `initAgent()`: AgentResolver + `*lg.Hvacr02Agent` 타입 체크
   - `callAgentProcess()`: timeout context 적용 호출
   - `shutdown()`: 공통 종료
5. 헬퍼 함수 구현
   - `applyHvacr02Overrides()`: address, timeout, poll_command, count 오버라이드
   - `buildHvacr02StatusCommand()`: get_stats / get_recent 커맨드 빌더
   - `buildHvacr02ControlCommand()`: 제어 커맨드 빌더 (address 필수 검증)
   - `hasHvacr02ControlKeys()`: 제어 키 감지
   - `hvacr02ControlKeys`: 제어 키 목록

### Secondary Goal: 3종 노드 구현

**대상 파일**:
- `internal/node/lg_hvacr02.go` (계속)

**작업 내용**:

**LGHvacr02StatusNode**:
1. 구조체 정의 (hvacr02NodeBase 임베딩 + pollInterval, sourceCh, stopCh, pollOnce)
2. `NewHvacr02StatusNode()` 팩토리 함수
3. `Configure()`: hvacr02NodeBase.configure() + poll_interval 파싱
4. `Init()`: initAgent() + pollLoop 고루틴 시작
5. `pollLoop()`: ticker 기반 상태 조회 루프
6. `Process()`: on-demand 상태 조회 (오버라이드 적용)
7. `Shutdown()`: stopCh close (sync.Once 보호)
8. `SourceCh()`: sourceCh 반환

**LGHvacr02ControlNode**:
1. 구조체 정의 (hvacr02NodeBase 임베딩)
2. `NewHvacr02ControlNode()` 팩토리 함수
3. `Configure()`: hvacr02NodeBase.configure()
4. `Init()`: initAgent()
5. `Process()`: 제어 커맨드 추출 및 에이전트 호출
6. `Shutdown()`: 종료

**LGHvacr02Node** (통합):
1. 구조체 정의 (hvacr02NodeBase 임베딩 + pollInterval, sourceCh, stopCh, pollOnce)
2. `NewHvacr02Node()` 팩토리 함수
3. `Configure()`: hvacr02NodeBase.configure() + poll_interval 파싱
4. `Init()`: initAgent() + pollLoop 고루틴 시작
5. `pollLoop()`: ticker 기반 상태 조회 루프
6. `Process()`: hasHvacr02ControlKeys 감지 -> 제어 또는 상태 조회
7. `Shutdown()`: stopCh close (sync.Once 보호)
8. `SourceCh()`: sourceCh 반환

### Final Goal: 레지스트리 등록 및 테스트

**대상 파일**:
- `internal/node/registry.go` (수정)
- `internal/node/lg_hvacr02_test.go` (신규)

**작업 내용**:
1. `registry.go`의 `registerBuiltins()`에 3종 노드 추가
2. `lg_hvacr02_test.go` 테스트 구현:
   - hvacr02NodeBase configure 테스트 (필수 필드 검증, 기본값 적용)
   - LGHvacr02StatusNode 팩토리 및 Configure 테스트
   - LGHvacr02ControlNode 팩토리 및 Configure 테스트
   - LGHvacr02Node 팩토리 및 Configure 테스트
   - buildHvacr02StatusCommand 테스트 (get_stats, get_recent)
   - buildHvacr02ControlCommand 테스트 (직접 커맨드, set_multiple, address 누락)
   - hasHvacr02ControlKeys 테스트
   - applyHvacr02Overrides 테스트
   - 레지스트리 등록 확인 테스트

### Optional Goal: 예제 플로우

**대상 파일**:
- `examples/flows/lg_hvacr02_status-poll.yaml` (신규)
- `examples/flows/lg_hvacr02_control.yaml` (신규)

**작업 내용**:
1. `lg_hvacr02_status-poll.yaml`: lg_hvacr02_status 노드를 사용한 주기적 상태 조회 예제
2. `lg_hvacr02_control.yaml`: lg_hvacr02_control 노드를 사용한 실내기 제어 예제

---

## 3. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| LG HVACR-02 에이전트 Process() 인터페이스 변경 | 커맨드 빌더 수정 필요 | SPEC-LG-HVACR-002-001/002 커맨드 형식을 기준으로 구현, 변경 시 빌더만 수정 |
| AgentAccessor 타입 캐스팅 실패 | 노드 Init 실패 | 명확한 에러 메시지와 ErrHvacr02WrongAgentType 센티널 에러 제공 |
| address 누락으로 인한 제어 실패 | 에이전트 에러 전파 | 노드 레벨에서 사전 검증하여 ErrHvacr02MissingAddress 반환 |
| 폴링 고루틴 리소스 누수 | 메모리/CPU 증가 | sync.Once로 stopCh close 보호, Shutdown에서 확실한 종료 |

---

## 4. 수정 대상 파일 요약

| 파일 | 작업 | 설명 |
|------|------|------|
| `internal/node/lg_hvacr02.go` | 신규 | LG HVACR-02 3종 노드 + 공통 기반 + 헬퍼 함수 |
| `internal/node/lg_hvacr02_test.go` | 신규 | LG HVACR-02 노드 테스트 |
| `internal/node/errors.go` | 수정 | LG HVACR-02 센티널 에러 5종 추가 |
| `internal/node/registry.go` | 수정 | 빌트인 노드 3종 등록 |
| `examples/flows/lg_hvacr02_status-poll.yaml` | 신규 (선택) | 상태 조회 예제 플로우 |
| `examples/flows/lg_hvacr02_control.yaml` | 신규 (선택) | 제어 예제 플로우 |

---

## 5. 전문가 상담 권장

### expert-backend 상담 권장

이 SPEC은 Go 백엔드 구현(노드 패턴, 인터페이스 설계, 동시성 처리)을 포함하므로, 구현 단계(`/moai run SPEC-LG-HVACR-002-003`)에서 expert-backend 에이전트의 상담을 권장한다.

상담 포인트:
- hvacr02NodeBase와 lgapNodeBase 간 코드 중복 최소화 방안
- 폴링 고루틴의 graceful shutdown 패턴 검증
- buildHvacr02ControlCommand의 address 검증 로직 리뷰

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-06*
*작성: MoAI SPEC Builder (manager-spec)*
