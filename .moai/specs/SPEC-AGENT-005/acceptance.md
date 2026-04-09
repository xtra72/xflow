---
id: SPEC-AGENT-005
version: "1.0.0"
status: draft
created: "2026-04-09"
author: xtra
priority: high
related_spec: SPEC-AGENT-005
---

# SPEC-AGENT-005: Acceptance Criteria - Agent Enable/Disable

본 문서는 SPEC-AGENT-005 의 인수 기준을 Given-When-Then 형식으로 정의한다. 각 시나리오는 자동화된 테스트로 검증 가능해야 한다.

---

## 1. M1: AgentConfig 확장 (Enabled 필드)

### AC1.1: 기본값 - nil 은 enabled 로 해석

**Given** 새로운 `AgentConfig` 가 `Enabled` 필드를 설정하지 않은 채 생성되었을 때
**When** `cfg.IsEnabled()` 를 호출하면
**Then** 결과는 `true` 여야 한다

### AC1.2: 명시적 true

**Given** `Enabled` 가 `&true` 로 설정된 `AgentConfig`
**When** `cfg.IsEnabled()` 를 호출하면
**Then** 결과는 `true` 여야 한다

### AC1.3: 명시적 false

**Given** `Enabled` 가 `&false` 로 설정된 `AgentConfig`
**When** `cfg.IsEnabled()` 를 호출하면
**Then** 결과는 `false` 여야 한다

### AC1.4: JSON 직렬화 - nil 일 때 키 없음

**Given** `Enabled = nil` 인 `AgentConfig`
**When** `json.Marshal` 로 직렬화하면
**Then** 결과 JSON 에 `enabled` 키가 존재하지 않아야 한다 (omitempty)

### AC1.5: JSON 직렬화 - false 일 때 명시 출력

**Given** `Enabled = &false` 인 `AgentConfig`
**When** `json.Marshal` 로 직렬화하면
**Then** 결과 JSON 에 `"enabled": false` 가 포함되어야 한다

### AC1.6: JSON 역직렬화 - 키 없음 → nil

**Given** `enabled` 키가 없는 JSON 문자열
**When** `json.Unmarshal` 로 `AgentConfig` 로 역직렬화하면
**Then** `cfg.Enabled` 는 `nil` 이어야 하고 `cfg.IsEnabled()` 는 `true` 를 반환해야 한다

### AC1.7: JSON 역직렬화 - false 명시

**Given** `{"enabled": false, ...}` JSON
**When** 역직렬화하면
**Then** `cfg.Enabled != nil && *cfg.Enabled == false` 이어야 한다

### AC1.8: YAML 직렬화 동일성

**Given** `Enabled = &false` 인 `AgentConfig`
**When** `yaml.Marshal` 로 직렬화하면
**Then** 결과 YAML 에 `enabled: false` 가 포함되어야 한다

---

## 2. M2: 데몬 자동 시작 제어

### AC2.1: enabled 에이전트는 자동 시작

**Given** 저장소에 `enabled = true` 인 에이전트 A 가 등록되어 있을 때
**When** 데몬이 시작되어 자동 시작 루프를 실행하면
**Then** A 는 running 상태로 시작되어야 한다

### AC2.2: disabled 에이전트는 자동 시작 안 됨

**Given** 저장소에 `enabled = false` 인 에이전트 B 가 등록되어 있을 때
**When** 데몬이 시작되어 자동 시작 루프를 실행하면
**Then** B 는 매니저에 등록(Create)되지만 stopped 상태로 남아야 한다

### AC2.3: nil enabled 는 enabled 처럼 동작

**Given** 저장소에 `enabled` 필드가 없는 (nil) 에이전트 C 가 등록되어 있을 때
**When** 데몬이 시작되어 자동 시작 루프를 실행하면
**Then** C 는 running 상태로 시작되어야 한다 (하위 호환)

### AC2.4: disabled 에이전트도 List API 에 노출

**Given** 데몬이 시작되어 disabled 에이전트 B 를 생성했을 때
**When** `GET /agents` 를 호출하면
**Then** 응답에 B 가 포함되어야 하고, 응답의 `enabled` 필드는 `false` 여야 한다

### AC2.5: 자동 시작 건너뜀 로그

**Given** disabled 에이전트가 자동 시작 루프에 도달했을 때
**When** 자동 시작이 건너뛰어지면
**Then** 로그에 `"skipped auto-start"` 와 에이전트 ID 가 INFO 레벨로 기록되어야 한다

---

## 3. M3: Enable/Disable API 엔드포인트

### AC3.1: Disable - 영속화 검증

**Given** 에이전트 A 가 `enabled = true` 상태로 존재할 때
**When** `POST /agents/A/disable` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- 응답 body 에 `"enabled": false` 포함
- 저장소에서 다시 로드해도 `enabled = false` 유지

### AC3.2: Enable - 영속화 검증

**Given** 에이전트 B 가 `enabled = false` 상태로 존재할 때
**When** `POST /agents/B/enable` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- 응답 body 에 `"enabled": true` 포함
- 저장소 재로드 후에도 `enabled = true` 유지

### AC3.3: Disable - running 상태 영향 없음 (R3.7)

**Given** 에이전트 C 가 `enabled = true` 이면서 running 상태일 때
**When** `POST /agents/C/disable` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- C 는 여전히 running 상태여야 한다 (Stop 호출 안 됨)
- C 의 `enabled = false` 만 변경되어야 한다

### AC3.4: Enable - 자동 Start 안 함 (R3.8)

**Given** 에이전트 D 가 `enabled = false` 이면서 stopped 상태일 때
**When** `POST /agents/D/enable` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- D 는 여전히 stopped 상태여야 한다
- D 의 `enabled = true` 만 변경되어야 한다

### AC3.5: 존재하지 않는 에이전트

**Given** 에이전트 ID `UNKNOWN` 이 존재하지 않을 때
**When** `POST /agents/UNKNOWN/enable` 또는 `disable` 을 호출하면
**Then** 응답 코드는 404 Not Found 여야 한다

### AC3.6: Idempotent - 이미 enabled 인데 enable

**Given** 에이전트 E 가 `enabled = true` 상태일 때
**When** `POST /agents/E/enable` 를 다시 호출하면
**Then**:
- 응답 코드는 200 OK
- 상태 변경 없음 (정상 응답)

### AC3.7: GET /agents/{id} 응답에 enabled 포함

**Given** 에이전트 F 가 존재할 때
**When** `GET /agents/F` 를 호출하면
**Then** 응답 JSON 에 `"enabled": true` 또는 `"enabled": false` 필드가 명시적으로 포함되어야 한다

### AC3.8: 동시성 안전

**Given** 에이전트 G 가 존재할 때
**When** 두 클라이언트가 동시에 `POST /agents/G/enable` 과 `POST /agents/G/disable` 을 호출하면
**Then** 최종 상태는 두 호출 중 하나의 결과로 일관되게 결정되어야 한다 (race condition 없음)

### AC3.9: 영속화 실패 시 롤백 (NFR7)

**Given** 저장소가 일시적으로 쓰기 실패하는 상황
**When** `POST /agents/H/disable` 를 호출하면
**Then**:
- 응답 코드는 5xx 에러
- in-memory 의 `enabled` 값도 원래 상태로 롤백되어야 한다

---

## 4. M4: Disabled 에이전트 수동 Start 허용

### AC4.1: Disabled 에이전트의 수동 Start 성공

**Given** 에이전트 A 가 `enabled = false` 이고 stopped 상태일 때
**When** `POST /agents/A/start` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- A 는 running 상태가 되어야 한다
- A 의 `enabled` 는 여전히 `false` 여야 한다 (R4.2)

### AC4.2: 수동 Start 후 데몬 재시작 시 자동 시작 안 됨

**Given** AC4.1 의 시나리오 후 A 가 running 상태일 때
**When** 데몬이 정지되고 다시 시작되면
**Then** A 는 stopped 상태여야 한다 (자동 시작 안 됨)

### AC4.3: 수동 Start 로그

**Given** disabled 에이전트의 수동 Start 가 호출될 때
**When** Start 가 성공하면
**Then** INFO 로그에 `"manual start of disabled agent"` 와 에이전트 ID 가 기록되어야 한다

### AC4.4: Disabled 에이전트의 수동 Stop

**Given** disabled 에이전트가 (수동 시작으로) running 상태일 때
**When** `POST /agents/A/stop` 를 호출하면
**Then**:
- 응답 코드는 200 OK
- A 는 stopped 상태가 되어야 한다
- A 의 `enabled` 는 여전히 `false` 여야 한다

---

## 5. M5: 플로우 배포 시 Disabled 에이전트 검증

### AC5.1: 모든 enabled 에이전트 - 배포 성공

**Given** 모든 참조 에이전트가 enabled 상태인 플로우 F1
**When** `DeployFlow(F1)` 을 호출하면
**Then** 배포가 성공해야 한다

### AC5.2: Disabled 에이전트 참조 - 배포 거부

**Given** 노드 N1 이 disabled 에이전트 A 를 참조하는 플로우 F2
**When** `DeployFlow(F2)` 를 호출하면
**Then**:
- 에러가 반환되어야 한다
- 에러는 `ErrAgentDisabled` 를 wrap 해야 한다 (`errors.Is(err, ErrAgentDisabled) == true`)
- 에러 메시지에 에이전트 A 의 ID 가 포함되어야 한다
- 에러 메시지에 노드 N1 의 ID 또는 이름이 포함되어야 한다
- 에러 메시지에 enable 안내가 포함되어야 한다 (예: `POST /agents/A/enable`)

### AC5.3: Disabled 에이전트 enable 후 재배포

**Given** AC5.2 의 거부된 플로우 F2
**When** 에이전트 A 를 enable 하고 다시 `DeployFlow(F2)` 를 호출하면
**Then** 배포가 성공해야 한다

### AC5.4: 다중 disabled 에이전트 - 누적 보고 (R5.6, optional)

**Given** 노드 N1 이 disabled 에이전트 A 를, N2 가 disabled 에이전트 B 를 참조하는 플로우 F3
**When** `DeployFlow(F3)` 를 호출하면
**Then**:
- 에러가 반환되어야 한다
- 에러 메시지에 A 와 B 모두 언급되어야 한다 (조기 반환 대신 모든 검증 누적)

### AC5.5: AgentRef 없는 노드는 영향 없음

**Given** AgentRef 가 nil 인 노드만 포함하는 플로우 F4
**When** `DeployFlow(F4)` 를 호출하면
**Then** disabled 검증을 건너뛰고 배포가 성공해야 한다

### AC5.6: 존재하지 않는 에이전트 vs disabled

**Given** 노드가 존재하지 않는 에이전트 ID `GHOST` 를 참조하는 플로우 F5
**When** `DeployFlow(F5)` 를 호출하면
**Then** `ErrAgentNotFound` (또는 기존 에러) 가 반환되어야 하며, `ErrAgentDisabled` 가 아니어야 한다 (검증 우선순위)

---

## 6. M6: Web UI Enable/Disable 컨트롤

### AC6.1: AgentInfo 인터페이스에 enabled 필드

**Given** Web UI 의 `AgentInfo` 타입 정의
**When** TypeScript 컴파일러가 검사하면
**Then** `enabled?: boolean` 필드가 존재해야 한다

### AC6.2: Disabled 배지 렌더링

**Given** `enabled = false` 인 AgentInfo 가 props 로 전달된 `AgentStatusBadge` 컴포넌트
**When** 컴포넌트가 렌더링되면
**Then** "비활성화" 텍스트가 포함된 회색(또는 시각적으로 구분되는) 배지가 표시되어야 한다

### AC6.3: Enabled 배지는 disabled 배지 없음

**Given** `enabled = true` 또는 `enabled = undefined` 인 AgentInfo
**When** `AgentStatusBadge` 가 렌더링되면
**Then** "비활성화" 배지가 표시되지 않아야 한다

### AC6.4: Enable 버튼 클릭

**Given** disabled 에이전트가 표시된 `AgentActionButtons` 컴포넌트
**When** 사용자가 "활성화" 버튼을 클릭하면
**Then**:
- `useEnableAgent` mutation 이 해당 에이전트 ID 로 호출되어야 한다
- 성공 시 `['agents']` 쿼리가 무효화되어야 한다

### AC6.5: Disable 버튼 클릭

**Given** enabled 에이전트가 표시된 `AgentActionButtons` 컴포넌트
**When** 사용자가 "비활성화" 버튼을 클릭하면
**Then**:
- `useDisableAgent` mutation 이 해당 에이전트 ID 로 호출되어야 한다
- 성공 시 `['agents']` 쿼리가 무효화되어야 한다

### AC6.6: 즉시 UI 반영

**Given** 사용자가 disable 버튼을 클릭한 직후
**When** mutation 이 성공 응답을 받으면
**Then** 에이전트 행에 disabled 배지가 1초 이내에 표시되어야 한다 (refetch 완료)

### AC6.7: 필터 (선택, R6.5)

**Given** 에이전트 목록 페이지에 enabled/disabled 필터 컨트롤이 있을 때
**When** 사용자가 "비활성화만" 필터를 선택하면
**Then** disabled 에이전트만 표시되어야 한다

---

## 7. M7: 하위 호환성

### AC7.1: 기존 저장소 데이터 - 자동 시작

**Given** 본 SPEC 적용 이전에 생성된 (enabled 필드 없는) 에이전트 LEGACY 가 저장소에 존재할 때
**When** 데몬을 시작하면
**Then** LEGACY 는 정상적으로 자동 시작되어야 한다 (running 상태)

### AC7.2: 기존 데이터의 API 응답

**Given** AC7.1 의 LEGACY 에이전트
**When** `GET /agents/LEGACY` 를 호출하면
**Then** 응답 JSON 에 `"enabled": true` 가 명시적으로 포함되어야 한다

### AC7.3: 마이그레이션 불필요

**Given** 본 SPEC 적용 전후
**When** 저장소 파일을 비교하면
**Then**:
- 기존 에이전트 데이터는 변경되지 않아야 한다
- 새로 disable 한 에이전트만 `enabled` 키가 추가되어야 한다

### AC7.4: 구버전 클라이언트 호환

**Given** `enabled` 필드를 인지하지 못하는 구버전 클라이언트
**When** `GET /agents` 응답을 받으면
**Then** 클라이언트는 추가 필드를 무시하고 정상 동작해야 한다 (additive change)

---

## 8. 통합 시나리오 (End-to-End)

### Integration Scenario 1: 운영 절차 - 에이전트 점검

**Given**:
- 에이전트 X 가 enabled, running 상태
- 운영자가 X 의 일시 점검을 원함

**When**:
1. 운영자가 `POST /agents/X/disable` 호출
2. 운영자가 데몬을 정지
3. 운영자가 데몬을 다시 시작

**Then**:
- 1번 후: X 는 여전히 running, enabled=false
- 3번 후: X 는 stopped (자동 시작 건너뜀)
- X 점검 완료 후 `POST /agents/X/enable` 으로 복구 가능

### Integration Scenario 2: 신규 에이전트 단계적 배포

**Given**:
- 신규 에이전트 NEW 를 등록하되 즉시 시작하지 않으려 함

**When**:
1. 에이전트 NEW 를 생성 (Create API)
2. 즉시 `POST /agents/NEW/disable` 호출
3. 데몬 재시작
4. 검증 완료 후 `POST /agents/NEW/enable` 호출
5. `POST /agents/NEW/start` 호출 (수동)
- 또는 다음 데몬 재시작을 기다림

**Then**:
- 1번 직후: NEW 는 created (auto-start 정책에 따라 running 또는 stopped)
- 2번 직후: NEW.enabled = false 영속화
- 3번 후: NEW 는 stopped
- 5번 후: NEW running

### Integration Scenario 3: 플로우 배포 거부와 복구

**Given**:
- 플로우 P1 이 에이전트 A1, A2 를 참조
- A1 은 enabled, A2 는 disabled

**When**:
1. `DeployFlow(P1)` 호출
2. 운영자가 응답 에러를 보고 A2 enable
3. `DeployFlow(P1)` 재호출

**Then**:
- 1번: ErrAgentDisabled 에러, 메시지에 A2 와 enable 안내 포함
- 3번: 배포 성공

### Integration Scenario 4: 백워드 호환 검증

**Given**:
- 본 SPEC 이전에 5개의 에이전트가 운영 중인 시스템
- 5개 모두 자동 시작되어 운영되고 있음

**When**:
1. 본 SPEC 의 새 바이너리로 데몬을 재시작

**Then**:
- 5개 에이전트 모두 정상 자동 시작
- `GET /agents` 응답에 모두 `"enabled": true` 표시
- 저장소 파일은 (enable/disable 호출 전까지) 변경 없음
- 운영 동작 변경 없음

---

## 9. 에러 시나리오

### Error 1: 잘못된 HTTP 메서드

**Given** Enable/Disable 엔드포인트
**When** `GET /agents/X/enable` 또는 `PUT /agents/X/enable` 등 잘못된 메서드 호출
**Then** 405 Method Not Allowed 응답

### Error 2: 빈 에이전트 ID

**When** `POST /agents//enable` (빈 ID) 호출
**Then** 404 또는 400 응답 (라우터 정책에 따름)

### Error 3: 영속화 실패 (저장소 다운)

**Given** 저장소가 응답하지 않는 상황
**When** Enable/Disable API 호출
**Then**:
- 5xx 응답
- in-memory 상태는 원래대로 롤백
- 로그에 에러 기록

### Error 4: DeployFlow - 다중 disabled

**Given** 플로우가 3개의 disabled 에이전트를 참조
**When** DeployFlow 호출
**Then**:
- ErrAgentDisabled 에러
- 가능한 경우 (R5.6): 3개 모두 메시지에 포함
- 최소 보장: 1개 이상은 명확히 식별 가능

---

## 10. 성능 / 비기능 검증

### Perf 1: 자동 시작 루프 성능

**Given** 100 개의 에이전트 (50 enabled, 50 disabled)
**When** 데몬 자동 시작을 측정하면
**Then** 자동 시작 루프 자체의 추가 오버헤드는 enabled 체크 추가 전 대비 1ms 이내여야 한다

### Perf 2: DeployFlow 검증 성능

**Given** 1000 개의 노드를 가진 플로우
**When** DeployFlow 검증을 실행하면
**Then** disabled 검증 추가로 인한 지연은 10ms 이내여야 한다 (O(N))

### Reliability 1: 동시 요청 처리

**Given** 같은 에이전트에 대해 1초 동안 100 회의 enable/disable 호출
**When** 모든 요청이 처리된 후
**Then**:
- 모든 응답이 200 또는 명확한 에러
- 최종 상태가 마지막 요청과 일치
- 데드락 없음

### Observability 1: 로그 검증

**Given** 자동 시작 루프 실행 + 다양한 enable/disable 호출
**When** 로그를 분석하면
**Then**:
- disabled 자동 시작 건너뜀: INFO 로그
- enable/disable API 호출: INFO 로그 (agent_id, 새 상태 포함)
- 영속화 실패: ERROR 로그

---

## 11. 검증 도구 및 방법

| 검증 영역 | 도구 | 방법 |
|----------|------|------|
| Go 단위 테스트 | `go test` | `_test.go` 파일 (table-driven) |
| Go 통합 테스트 | `go test` | 실제 storage + manager + handler |
| Race detection | `go test -race` | 동시성 검증 |
| 커버리지 | `go test -cover` | 신규 코드 ≥ 85% |
| Lint | `golangci-lint run` | 코드 품질 |
| Vet | `go vet ./...` | 정적 분석 |
| Web 단위 테스트 | Vitest + RTL | 컴포넌트, hooks |
| API 검증 | curl, httpie | 수동 smoke test |
| E2E (선택) | Playwright | 브라우저 기반 시나리오 |

---

## 12. Definition of Done (인수 완료 기준)

본 SPEC 의 인수가 완료되었다고 판정하는 기준:

- [ ] 모든 AC1.x ~ AC7.x 시나리오 통과
- [ ] Integration Scenario 1~4 통과
- [ ] Error Scenario 1~4 통과
- [ ] Perf 1, 2 통과
- [ ] Reliability 1 통과
- [ ] Observability 1 통과
- [ ] 신규 코드 커버리지 ≥ 85%
- [ ] `go test -race ./...` 통과
- [ ] `golangci-lint run` 통과
- [ ] Web UI 컴포넌트 테스트 통과
- [ ] 수동 smoke test (Personal mode operator) 통과
- [ ] 백워드 호환성 검증 (기존 저장소 데이터 동작 변경 없음)
- [ ] CHANGELOG.md 업데이트
