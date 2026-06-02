---
id: SPEC-SWITCH-001
title: "Switch Node 완성 — 문자열 조건 기반 다중 포트 라우팅"
version: "1.0.0"
status: planned
created: "2026-06-03"
updated: "2026-06-03"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-03 | xtra | 초기 구현 계획 작성 (PLAN 단계) |

# SPEC-SWITCH-001: 구현 계획

## 1. 구현 전략

### 1.1 개발 방법론

Hybrid 모드 적용 (`quality.yaml: development_mode: hybrid`):

- **신규 설정 경로 (TDD)**: `Configure`의 문자열 라우트 파싱, `match_mode`, `Ports()` 동적 파생은 새로 추가되는 동작이므로 RED-GREEN-REFACTOR로 작성한다.
- **기존 Process 동작 (DDD)**: `Process`의 first-match / `_target_port` / Clone / 드롭 동작은 **보존 대상**이다. 변경(특히 `all` 모드 추가) 전에 기존 동작을 캡처하는 characterization 테스트를 유지하고, 회귀가 없음을 확인하며 점진적으로 확장한다.

### 1.2 변경 영향 범위

| 파일 | 변경 유형 | 영향도 | 비고 |
|------|-----------|--------|------|
| `internal/node/switch.go` | 수정 (Configure 확장, Ports 추가, Process all 모드) | 높음 | 핵심 |
| `internal/node/switch_test.go` | 추가 | 중간 | 문자열 라우트·all·Ports·에러 케이스 |
| `web/src/config/nodeSchemas.ts` | 수정 (switch configSchema → 구조화 필드) | 중간 | UI 스키마 |
| `web/src/components/.../CustomNode.tsx` | 수정 (구조화 에디터 + 동적 핸들) | 중간 | UI 렌더링 |
| `internal/node/registry.go` | 변경 없음 | — | switch 이미 등록됨 |
| `internal/node/condition.go` | 변경 없음 | — | `compileCondition` 재사용만 |

## 2. 기술 접근

### 2.1 백엔드 — `switch.go: Configure`

기존 `Configure`는 `config["routes"].([]SwitchRoute)` 타입 단언만 수행한다. 다음 분기 우선순위로 확장한다 (filter.go의 condition 처리 패턴과 동일한 철학).

1. **`[]SwitchRoute` (Go 함수)**: 기존 동작 보존(REQ-SWITCH-004).
2. **`[]any` (JSON/YAML)**: 각 원소를 `map[string]any`로 단언 → `name`(string), `condition`(string) 추출 → `compileCondition(condition)` 호출 → 실패 시 라우트 식별 정보를 포함한 에러 반환(REQ-SWITCH-002). 빈 `name`/`condition` 또는 비문자열은 에러(REQ-SWITCH-003).
3. `match_mode`는 `select_field.go`의 `configBool` 유사 패턴(문자열 허용, Web UI 호환)으로 읽어 `first`/`all`로 정규화, 미인식은 `first`(REQ-SWITCH-010).
4. `default_port`는 기존대로 string으로 읽는다(REQ-SWITCH-020/021).

컴파일된 라우트는 `mu.Lock()` 하에 `n.routes`에 저장한다(A5, A6). 파싱한 원본 `routes` 메타(name 순서)도 `Ports()` 파생을 위해 보관한다.

### 2.2 백엔드 — `switch.go: Process` (all 모드)

기존 first-match 루프를 보존하되, `match_mode == all`일 때 매칭되는 모든 라우트에 대해 Clone + `_target_port` 설정 후 결과 슬라이스에 누적한다(REQ-SWITCH-012). `first`는 첫 매칭에서 즉시 반환(REQ-SWITCH-011). 매칭 0건일 때만 default/드롭 분기(REQ-SWITCH-020/021/022). `_target_port` = 라우트 포트명 규약 유지.

### 2.3 백엔드 — `switch.go: Ports()` 오버라이드

`BaseNode.Ports()`는 정적 `inputs/outputs`를 반환하므로, `SwitchNode`에 `Ports()`를 추가해 `mu.RLock()` 하에 동적으로 구성한다.

- 입력: `in`.
- 출력: `routes` 비면 `out`; 아니면 각 라우트 포트명 + (`defaultPort != ""`면) `defaultPort`.
- 프론트엔드 `computePortsForNode`와 동일한 규칙(REQ-SWITCH-030/031/032). `NodePort` 구조는 base.go의 정의를 따른다.

### 2.4 프론트엔드 — `nodeSchemas.ts`

`switch` 항목의 `routes` 필드(`type: 'object'`, 원시 JSON)를 구조화 라우트 에디터를 트리거하는 필드 타입으로 교체하거나, 전용 에디터 컴포넌트와 연결되는 필드 메타로 변경한다. `match_mode`(select: first/all), `default_port`(string) 필드를 명시한다. `computePortsForNode`는 이미 `routes[].name` + `default`를 파생하므로 로직 변경 없이 재사용하되, `default` 포트 이름을 실제 `default_port` 값과 정렬할지 결정한다(아래 위험요소 R4 참조).

### 2.5 프론트엔드 — `CustomNode.tsx`

조건식/포트명 행의 추가·삭제 UI와, `computePortsForNode(config)` 기반 출력 핸들 동적 렌더링을 연결한다(REQ-SWITCH-040..042). 한국어 라벨 사용.

## 3. 작업 분해 (우선순위 기반)

### 1차 목표 (Priority High) — 백엔드 설정 + 포트
1. `switch.go: Configure` — `[]any` 구조화 라우트 파싱 + `compileCondition` 컴파일 + 에러 처리.
2. `switch.go: Configure` — `match_mode` 해석.
3. `switch.go: Ports()` — 동적 포트 파생.
4. `switch_test.go` — 문자열 라우트, 잘못된 표현식 에러, 빈 필드 에러, Ports 파생, 하위 호환 테스트.

### 2차 목표 (Priority High) — 라우팅 동작
5. `switch.go: Process` — `all` 모드 팬아웃 + default/드롭 상호작용.
6. `switch_test.go` — first vs all, default vs 드롭, all+미매칭 테스트.

### 3차 목표 (Priority Medium) — 프론트엔드
7. `nodeSchemas.ts` — switch 구조화 필드 + match_mode/default_port.
8. `CustomNode.tsx` — 구조화 라우트 에디터(행 추가/삭제) + 동적 출력 핸들.

### 최종 목표 (Priority Medium) — 통합 검증
9. 백엔드 `Ports()` ↔ 프론트엔드 `computePortsForNode` 포트명 정렬 통합 확인.
10. 기존 플로우(빈 routes) 하위 호환 회귀 확인.

## 4. 위험요소 및 대응

| ID | 위험 | 영향 | 대응 |
|----|------|------|------|
| R1 | 포트명 변경 시 기존 와이어 무효화 | 사용자가 라우트 `name`을 바꾸면 해당 출력에 연결된 와이어의 `SourcePort`가 더 이상 매칭되지 않아 메시지가 유실 | 본 SPEC 범위에서는 자동 재배선하지 않음. UI에서 포트명 변경 시 경고를 표시하는 것을 권장(별도 후속). 문서화. |
| R2 | first/all 의미 혼동 | 사용자가 `all`을 기대하나 `first`로 동작(또는 반대) | 기본값 `first` 명시, UI에 모드 설명 라벨. 테스트로 두 모드 분기 고정. |
| R3 | RWMutex 하 routes 스레드 안전성 | Configure(쓰기)와 Process/Ports(읽기) 동시 접근 | 기존 `mu sync.RWMutex` 패턴 유지: Configure는 Lock, Process/Ports는 RLock. 락 보유 중 `n.Name()` 등 재귀 RLock 유발 호출 금지(프로젝트 트랩 회피). |
| R4 | `default` 포트 이름 불일치 | 프론트엔드는 `default` 고정 포트명을 파생하나 백엔드는 `default_port` 설정값을 사용 | 정렬 규칙을 명확히 결정: (a) 프론트 `computePortsForNode`가 `default` 대신 실제 `default_port` 값을 사용하도록 맞추거나, (b) 백엔드가 `default_port` 미설정 시 관례 포트명을 사용. AC-SWITCH-032에서 검증. (열린 질문 Q1) |
| R5 | 엔진 config 전달 형식 가정 오류 | `routes`가 `[]any`가 아닌 다른 형식으로 도착 | A1 가정을 엔진 `engine.go:190 Configure(nd.Config)` 경로로 검증하고, 예상 외 타입은 무시가 아닌 명확한 에러로 처리. |

## 5. 개발 모드 노트

- 신규 config 경로(`Configure` 문자열 라우트, `match_mode`, `Ports()`): **TDD**.
- 기존 `Process` first-match/`_target_port`/Clone/드롭: **동작 보존(DDD)** — characterization 테스트로 회귀 방지 후 `all` 모드를 점진 추가.
- `condition.go`는 변경하지 않고 재사용한다.

## 6. 완료 정의 (Definition of Done)

- 모든 REQ-SWITCH-* 요구사항이 acceptance.md의 AC로 검증된다.
- `go test ./internal/node/...` 통과, switch 패키지 커버리지 ≥ 85%.
- `go vet`, `golangci-lint` 무경고.
- 프론트엔드: 구조화 에디터에서 라우트 추가 시 출력 핸들이 동적 생성되고, 백엔드 `Ports()`와 포트명이 정렬된다.
- 기존 switch 플로우(빈 routes)가 회귀 없이 동작한다.
