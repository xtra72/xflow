---
id: SPEC-AGENT-005
version: "1.0.0"
status: draft
created: "2026-04-09"
author: xtra
priority: high
related_spec: SPEC-AGENT-005
---

# SPEC-AGENT-005: Implementation Plan - Agent Enable/Disable

## 1. 구현 개요

본 계획은 SPEC-AGENT-005 의 요구사항(M1~M7)을 6개의 구현 단계(Phase)로 분할한다. 각 Phase 는 의존 관계에 따라 순차적으로 실행되며, 가능한 경우 TDD(RED-GREEN-REFACTOR) 사이클을 적용한다.

### 1.1 개발 방법론

- **Methodology**: Hybrid (TDD for new code + DDD for existing code)
  - 신규 추가 코드 (Enable/Disable 로직): TDD
  - 기존 파일 수정 (main.go, agent_validation.go): DDD (characterization tests 우선)
- **Test Coverage Target**: 신규 코드 ≥ 85%, 수정 코드 ≥ 85%
- **참조 패턴**: `pkg/flow/node.go` 의 `NodeDef.Enabled *bool` + `IsEnabled()` 패턴

### 1.2 우선순위

| 우선순위 | 단계 | 설명 |
|---------|------|------|
| Primary Goal | Phase 1, 2 | Core 데이터 모델 + 데몬 자동 시작 제어 (가장 중요한 사용자 가치) |
| Secondary Goal | Phase 3, 4 | API 노출 + Engine 검증 (운영 가능성 확보) |
| Tertiary Goal | Phase 5 | Web UI (사용성 향상) |
| Final Goal | Phase 6 | 통합 테스트 + 문서화 |

> **시간 추정 금지**: 본 계획은 우선순위 기반이며, 각 Phase 의 절대 소요 시간은 명시하지 않는다. Phase 간 의존성은 명시한다.

---

## 2. Phase 1: Core - AgentConfig + Serialization

### 2.1 목표

`AgentConfig` 에 `Enabled *bool` 필드를 추가하고 직렬화를 지원한다.

### 2.2 변경 파일

#### `internal/agent/config.go`

**변경 사항:**

1. `AgentConfig` 구조체에 `Enabled *bool` 필드 추가 (Logger 필드 위쪽에 배치)
2. `IsEnabled() bool` 메서드 신규 추가 (NodeDef 패턴과 동일)
   - nil 일 때 `true` 반환
   - non-nil 일 때 `*c.Enabled` 반환
3. `Validate()` 는 변경 없음 (Enabled 는 항상 유효)
4. `DefaultAgentConfig()` 는 변경 없음 (nil 이면 enabled 로 해석되므로)

**TDD 단계:**

1. **RED**: `config_test.go` 에 `TestAgentConfig_IsEnabled_NilDefaultsToTrue`, `TestAgentConfig_IsEnabled_ExplicitFalse`, `TestAgentConfig_IsEnabled_ExplicitTrue` 작성 → 컴파일 실패
2. **GREEN**: `Enabled` 필드 + `IsEnabled()` 메서드 추가 → 테스트 통과
3. **REFACTOR**: NodeDef 와 코드 스타일 일치 확인

#### `internal/agent/serialize.go`

**변경 사항:**

1. `agentConfigJSON` 구조체에 `Enabled *bool` 필드 추가 with `json:"enabled,omitempty" yaml:"enabled,omitempty"`
2. `toJSON()` 에서 `config.Enabled` 를 `j.Enabled` 로 그대로 전달
3. `fromJSON()` 에서 `j.Enabled` 를 `AgentConfig.Enabled` 로 그대로 전달

**TDD 단계:**

1. **RED**: `serialize_test.go` 에 다음 테이블 테스트 추가:
   - `enabled=nil` → JSON 에 `enabled` 키 없음
   - `enabled=true (포인터)` → JSON `"enabled": true`
   - `enabled=false (포인터)` → JSON `"enabled": false`
   - 역방향: JSON `{}` → `Enabled == nil`
   - 역방향: JSON `{"enabled": false}` → `*Enabled == false`
2. **GREEN**: serialize.go 수정
3. **REFACTOR**: omitempty 가 제대로 동작하는지 raw JSON 검증

### 2.3 의존성

- 외부 의존: 없음
- 후속 Phase 의존: Phase 2, 3 가 본 Phase 의 결과물에 의존

### 2.4 완료 조건

- [ ] `AgentConfig.Enabled *bool` 필드 추가
- [ ] `IsEnabled()` 메서드 추가 (NodeDef 패턴 일치)
- [ ] JSON/YAML 직렬화 정상 동작 (omitempty 포함)
- [ ] 신규 테스트 모두 통과 (config_test.go, serialize_test.go)
- [ ] 기존 테스트 회귀 없음

---

## 3. Phase 2: Daemon Auto-Start Control

### 3.1 목표

데몬 부팅 시 disabled 에이전트의 자동 시작을 건너뛴다.

### 3.2 변경 파일

#### `cmd/xflowd/main.go` (lines 358~378 부근)

**현재 동작:**

```
저장된 모든 에이전트를 순회 → agentMgr.Create(...) → agentMgr.Start(id)
```

**변경 후 동작:**

```
저장된 모든 에이전트를 순회
  → agentMgr.Create(...)  (항상 등록)
  → if !cfg.IsEnabled():
       slog.Info("skipped auto-start (disabled)", "agent_id", cfg.ID)
       continue
  → agentMgr.Start(id)
```

**DDD 단계** (기존 코드 수정):

1. **ANALYZE**: 현재 자동 시작 루프의 흐름 파악 (lines 358-378)
2. **PRESERVE**: characterization test 작성 - "모든 에이전트가 enabled 일 때 모두 시작됨"
3. **IMPROVE**: enabled 체크 추가, 새 테스트 - "disabled 에이전트는 건너뜀"
4. characterization test 가 여전히 통과하는지 확인

### 3.3 테스트 전략

**Integration Test** (가능한 경우 `cmd/xflowd/main_test.go` 또는 별도 `internal/agent/manager_autostart_test.go`):

1. 임시 저장소에 3개 에이전트 등록 (enabled=true, enabled=false, enabled=nil)
2. 자동 시작 헬퍼 함수 실행
3. Manager.List() 결과 검증:
   - enabled=true → running
   - enabled=false → stopped (created only)
   - enabled=nil → running (default)

### 3.4 의존성

- 선행: Phase 1 (AgentConfig.IsEnabled() 필요)
- 후속: Phase 6 통합 테스트

### 3.5 완료 조건

- [ ] 데몬 부팅 시 disabled 에이전트 자동 시작 안 됨
- [ ] disabled 에이전트도 매니저에 등록 (List API 에 노출)
- [ ] INFO 로그 출력 검증
- [ ] characterization test + 신규 disabled test 통과

---

## 4. Phase 3: API Layer (Handlers, Service, DTO)

### 4.1 목표

Enable/Disable HTTP 엔드포인트와 응답 DTO 에 `enabled` 필드를 추가한다.

### 4.2 변경 파일

#### `internal/api/dto/response.go`

**변경 사항:**

1. `AgentInfo` 응답 DTO 에 `Enabled bool` 필드 추가 (`json:"enabled"`)
2. AgentConfig → AgentInfo 변환 함수에서 `cfg.IsEnabled()` 호출하여 채움
   - **주의**: bool (포인터 아님) 으로 노출. 항상 명시적 true/false 를 응답한다.

#### `internal/api/service/agent_adapter.go`

**변경 사항:**

1. `EnableAgent(ctx context.Context, id string) (AgentInfo, error)` 메서드 신규 추가
   - 에이전트 조회 → `Enabled = &true` 설정 → 저장소 영속화 → AgentInfo 반환
   - 영속화 실패 시 in-memory 롤백
2. `DisableAgent(ctx context.Context, id string) (AgentInfo, error)` 메서드 신규 추가
   - 에이전트 조회 → `Enabled = &false` 설정 → 저장소 영속화 → AgentInfo 반환
   - **주의**: Stop 호출 금지 (R3.7)
3. 기존 `GetAgent`, `ListAgents` 응답에 `Enabled` 필드가 채워지는지 확인

#### `internal/api/handler/agent.go`

**변경 사항:**

1. `handleEnable(w, r)` HTTP 핸들러 신규 추가
   - URL 파라미터에서 `id` 추출
   - `service.EnableAgent(ctx, id)` 호출
   - 404 / 200 응답 처리
2. `handleDisable(w, r)` HTTP 핸들러 신규 추가 (대칭 구조)
3. 라우터 등록:
   - `POST /agents/{id}/enable` → handleEnable
   - `POST /agents/{id}/disable` → handleDisable
   - 기존 start/stop 라우트 등록 패턴과 동일

### 4.3 TDD 단계

1. **RED** (DTO):
   - `dto/response_test.go`: AgentInfo 에 enabled 필드가 포함되는지
   - 변환 함수가 `IsEnabled()` 결과를 올바르게 채우는지
2. **GREEN**: DTO 수정
3. **RED** (Service):
   - `agent_adapter_test.go`: `TestEnableAgent_PersistsEnabledTrue`
   - `TestDisableAgent_PersistsEnabledFalse_DoesNotStop` (Mock manager 의 Stop 이 호출되지 않음을 검증)
   - `TestEnableAgent_NotFound_ReturnsError`
4. **GREEN**: Service 메서드 구현
5. **RED** (Handler):
   - `agent_handler_test.go`: HTTP 엔드포인트 통합 테스트
   - 200, 404 케이스
6. **GREEN**: Handler + 라우팅 추가
7. **REFACTOR**: start/stop 핸들러와의 코드 중복 추출

### 4.4 의존성

- 선행: Phase 1 (AgentConfig.Enabled), Phase 2 (자동 시작 동작 확정)
- 후속: Phase 5 (Web UI 가 본 API 호출)

### 4.5 완료 조건

- [ ] `POST /agents/{id}/enable` 정상 동작
- [ ] `POST /agents/{id}/disable` 정상 동작 (running 에이전트의 런타임 영향 없음)
- [ ] `GET /agents`, `GET /agents/{id}` 응답에 `enabled` 필드 포함
- [ ] 영속화 실패 시 롤백
- [ ] 동시성 안전 (manager lock 통과)

---

## 5. Phase 4: Engine Validation (Disabled Reference Check)

### 5.1 목표

`DeployFlow` 가 disabled 에이전트를 참조하는 플로우의 배포를 거부한다.

### 5.2 변경 파일

#### `internal/engine/errors.go`

**변경 사항:**

1. Sentinel error 추가:
   ```
   ErrAgentDisabled = errors.New("agent is disabled")
   ```
2. 기존 error 패턴(예: `ErrAgentNotFound`)과 일관된 명명 적용

#### `internal/engine/agent_validation.go`

**변경 사항:**

1. `validateAgentRefs(flow, agentMgr)` 함수 확장:
   - 기존: 에이전트 ID 존재 여부만 검증
   - 추가: 에이전트가 enabled 인지 검증
2. 에러 메시지 형식:
   - `"agent %q is disabled (referenced by node %q): use POST /agents/%s/enable to activate"`
3. **선택적 누적 검증** (R5.6): `errors.Join` 으로 여러 disabled 에이전트를 한 번에 보고

**DDD 단계:**

1. **ANALYZE**: 현재 `validateAgentRefs` 의 호출 경로와 반환 에러 패턴 파악
2. **PRESERVE**: 기존 검증 (존재하지 않는 에이전트 거부) characterization test 작성
3. **IMPROVE**: disabled 검증 추가 + 신규 테스트
4. 두 검증이 모두 통과하는지 확인

### 5.3 테스트 전략

**Unit Tests** (`agent_validation_test.go`):

1. `TestValidateAgentRefs_AllEnabled_ReturnsNil`
2. `TestValidateAgentRefs_OneDisabled_ReturnsErrAgentDisabled`
3. `TestValidateAgentRefs_MultipleDisabled_ReturnsJoinedError` (R5.6)
4. `TestValidateAgentRefs_NodeWithoutAgentRef_Skipped`
5. `TestValidateAgentRefs_DisabledNodeReferencingDisabledAgent_StillValidated`

**Integration Test**: `engine_test.go` 에서 DeployFlow 가 ErrAgentDisabled 를 반환하는지 검증.

### 5.4 의존성

- 선행: Phase 1 (AgentConfig.IsEnabled())
- 후속: 없음 (독립적)

### 5.5 완료 조건

- [ ] `ErrAgentDisabled` sentinel error 정의
- [ ] `validateAgentRefs` 가 disabled 에이전트 거부
- [ ] DeployFlow 가 명확한 에러 메시지로 거부
- [ ] 다중 disabled 에이전트 누적 보고 (선택)
- [ ] 모든 신규/기존 테스트 통과

---

## 6. Phase 5: Web UI

### 6.1 목표

Web UI 에서 에이전트의 enable/disable 상태를 표시하고 토글할 수 있도록 한다.

### 6.2 변경 파일

#### `web/src/types/agent.ts`

**변경 사항:**

```ts
export interface AgentInfo {
  // ... 기존 필드
  enabled?: boolean;  // 신규 (백워드 호환을 위해 옵셔널)
}
```

#### `web/src/hooks/useAgent.ts`

**변경 사항:**

1. `useEnableAgent()` mutation hook 신규 추가
   - `POST /agents/{id}/enable` 호출
   - 성공 시 `['agents']` 쿼리 무효화
2. `useDisableAgent()` mutation hook 신규 추가 (대칭 구조)

#### `web/src/pages/agents/AgentStatusBadge.tsx`

**변경 사항:**

1. `enabled === false` 일 때 회색 "비활성화" 배지 추가 표시
2. running 상태와 별개로 enabled 상태 시각화

#### `web/src/pages/agents/AgentActionButtons.tsx`

**변경 사항:**

1. 기존 Start/Stop 버튼 옆에 Enable/Disable 토글 버튼 추가
2. enabled 상태에 따라 버튼 라벨/아이콘 전환
3. 클릭 시 적절한 mutation hook 호출

#### `web/src/pages/agents/AgentListPage.tsx`

**변경 사항:**

1. enabled/disabled 필터 컨트롤 추가 (R6.5, optional)
2. AgentInfo 표시 컬럼에 enabled 상태 노출

### 6.3 테스트 전략

- Vitest + React Testing Library
- `AgentActionButtons.test.tsx`: enable/disable 클릭 시 mutation 호출 검증
- `AgentStatusBadge.test.tsx`: enabled=false 일 때 disabled 배지 렌더링
- `useAgent.test.ts`: hooks 동작 검증

### 6.4 의존성

- 선행: Phase 3 (API 엔드포인트)

### 6.5 완료 조건

- [ ] AgentInfo 인터페이스에 enabled 추가
- [ ] Enable/Disable mutation hooks 동작
- [ ] 시각적 disabled 배지 표시
- [ ] 토글 버튼 클릭으로 상태 변경
- [ ] React Query 캐시 무효화로 즉시 갱신
- [ ] 컴포넌트 테스트 통과

---

## 7. Phase 6: Integration Tests & Documentation

### 7.1 목표

End-to-end 시나리오를 검증하고 관련 문서를 업데이트한다.

### 7.2 통합 테스트 시나리오

#### Test 1: Disable → Daemon Restart → 자동 시작 안 됨

1. 에이전트 A 생성 (enabled=true 기본값)
2. 데몬 시작 → A running 확인
3. `POST /agents/A/disable` 호출
4. A 는 여전히 running 상태 확인 (R3.7)
5. 데몬 재시작
6. A 는 stopped (auto-start 건너뜀) 확인

#### Test 2: Disabled 에이전트의 수동 Start

1. 에이전트 B 생성 후 disable
2. 데몬 재시작 (B 는 stopped)
3. `POST /agents/B/start` 호출
4. B running 확인 (R4.1)
5. B.enabled 는 여전히 false 확인 (R4.2)
6. 데몬 재시작
7. B stopped 확인

#### Test 3: Disabled 에이전트 참조 플로우 거부

1. 에이전트 C 생성 후 disable
2. C 를 참조하는 노드를 포함한 플로우 작성
3. `DeployFlow` 호출
4. `ErrAgentDisabled` 에러 반환 확인
5. 에러 메시지에 C 의 ID, 노드 ID, enable 안내 포함 확인
6. C 를 enable
7. `DeployFlow` 재호출 → 성공 확인

#### Test 4: 하위 호환성 - 기존 저장소 데이터

1. enabled 필드 없는 JSON 으로 에이전트 D 직접 저장
2. 데몬 시작
3. D 는 정상 자동 시작 확인 (nil → enabled 기본값)
4. `GET /agents/D` 응답이 `"enabled": true` 포함 확인

### 7.3 문서 업데이트

- [ ] `docs/api/agents.md` (있는 경우): Enable/Disable 엔드포인트 추가
- [ ] CHANGELOG.md: SPEC-AGENT-005 entry 추가
- [ ] README 의 에이전트 관리 섹션 (있는 경우) 업데이트

### 7.4 완료 조건

- [ ] 4개 통합 테스트 시나리오 모두 통과
- [ ] 신규 코드 커버리지 ≥ 85%
- [ ] golangci-lint 통과
- [ ] go vet 통과
- [ ] go test -race ./... 통과

---

## 8. 위험 요소 및 대응

### 8.1 위험: 기존 운영 에이전트 동작 변경

- **위험도**: 높음
- **시나리오**: 본 SPEC 적용 후 기존 에이전트가 의도치 않게 disabled 처리됨
- **대응**:
  - `enabled *bool` 포인터 + nil → true 패턴 엄수
  - Phase 7 의 백워드 호환 통합 테스트로 검증
  - 마이그레이션 불필요 (R7.3)

### 8.2 위험: 동시성 이슈

- **위험도**: 중간
- **시나리오**: 같은 에이전트에 동시 enable/disable 호출
- **대응**:
  - Manager-level lock 활용
  - Service 레이어에서 read-modify-write 를 atomic 하게 처리
  - 영속화 실패 시 in-memory 롤백 (NFR7)

### 8.3 위험: 플로우 검증 우회

- **위험도**: 중간
- **시나리오**: DeployFlow 후 에이전트를 disable 하면 런타임 시점에 nil 참조
- **대응**:
  - Disable API 는 현재 배포된 플로우가 참조하는지 확인 후 경고 (선택적, 향후 SPEC)
  - 또는 disable 시 영향받는 플로우 목록을 응답에 포함 (선택)
  - **본 SPEC 의 범위**: DeployFlow 시점 검증만 보장. 런타임 nil 체크는 별도 SPEC.

### 8.4 위험: Web UI 동기화 지연

- **위험도**: 낮음
- **시나리오**: 다른 클라이언트가 enable 한 직후 본 클라이언트가 disabled 로 표시
- **대응**: React Query 의 invalidate + refetch 패턴 + 적절한 staleTime 설정

---

## 9. 구현 순서 요약

```
Phase 1 (Core)  →  Phase 2 (Daemon)  →  Phase 3 (API)  →  Phase 5 (Web UI)
    │                                       │
    └──────→  Phase 4 (Engine Validation) ──┘
                                              │
                                              └──→  Phase 6 (Integration Tests)
```

- Phase 1 은 모든 후속 Phase 의 전제 조건이다.
- Phase 4 는 Phase 1 만 의존하므로 Phase 2/3 와 병렬 진행 가능하다.
- Phase 5 는 Phase 3 의 API 가 사용 가능해진 후 시작한다.
- Phase 6 은 모든 Phase 완료 후 진행한다.

---

## 10. 기술적 접근

### 10.1 패턴 일관성

본 SPEC 의 핵심은 **기존 NodeDef 패턴의 재사용** 이다:

| NodeDef (기존) | AgentConfig (신규) |
|----------------|---------------------|
| `Enabled *bool` 필드 | 동일 |
| `IsEnabled() bool` 메서드 | 동일 |
| nil → true 기본값 | 동일 |
| `omitempty` JSON 태그 | 동일 |
| 엔진에서 disabled 검증 | 동일 (validateAgentRefs 확장) |

### 10.2 Go 코드 스타일

- 모든 신규 함수에 godoc 주석 추가 (한국어)
- error wrapping 사용 (`fmt.Errorf("...: %w", err)`)
- context.Context 를 첫 인자로 (Service 메서드)
- table-driven test 우선
- `t.Parallel()` 적용 (독립 테스트)

### 10.3 마이그레이션 전략

**없음 (No Migration)**: JSON BLOB 저장소이므로 신규 필드 추가는 자동으로 처리된다. 기존 데이터는 unmarshal 시 `Enabled = nil` 이 되며 `IsEnabled()` 가 `true` 를 반환한다.

---

## 11. 완료 정의 (Definition of Done)

본 SPEC 의 구현이 완료되었다고 판정하는 기준:

- [ ] 모든 EARS 요구사항 (R1.1 ~ R7.4) 구현
- [ ] 모든 Phase (1~6) 완료
- [ ] 신규 코드 테스트 커버리지 ≥ 85%
- [ ] `go test -race ./...` 통과
- [ ] `golangci-lint run` 통과
- [ ] `go vet ./...` 통과
- [ ] 4개 통합 테스트 시나리오 통과
- [ ] 백워드 호환성 검증 (기존 에이전트 동작 변경 없음)
- [ ] Web UI 컴포넌트 테스트 통과
- [ ] CHANGELOG 업데이트
- [ ] PR 머지 및 SPEC status: `draft` → `completed`
