---
id: SPEC-SWITCH-001
title: "Switch Node 완성 — 문자열 조건 기반 다중 포트 라우팅"
version: "1.0.0"
status: planned
created: "2026-06-03"
updated: "2026-06-03"
author: "xtra"
priority: high
related_specs:
  - SPEC-FILTER-001
  - SPEC-EXPR-001
tags:
  - switch
  - routing
  - condition
  - expression
  - node
  - dynamic-ports
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-03 | xtra | 초기 SPEC 작성 (PLAN 단계) |

# SPEC-SWITCH-001: Switch Node 완성 — 문자열 조건 기반 다중 포트 라우팅

## 1. 개요

### 1.1 목적

이미 존재하지만 에디터에서 사용할 수 없는 `switch` 노드를 **완성**한다. 본 SPEC의 목표는 새 노드 타입을 만드는 것이 아니라, 기존 `SwitchNode`가 플로우 JSON / 에디터에서 설정 가능하도록 다음 3가지를 보완하는 것이다.

1. **문자열 조건식 라우트**: `Configure`가 Go 함수 클로저가 아닌 문자열 표현식 기반 라우트 규칙을 받도록 확장한다.
2. **동적 출력 포트**: `routes`와 `default_port`로부터 출력 포트를 런타임에 파생하여, 프론트엔드 `computePortsForNode` 출력과 정렬한다.
3. **구조화된 라우트 편집 UI**: 원시 JSON `routes` 입력 필드를 (조건식, 포트명) 행 단위의 구조화된 에디터로 교체한다.

### 1.2 배경 (검증된 사실)

- `internal/node/switch.go`: `SwitchNode`는 `routes []SwitchRoute{Condition func(msg) bool, TargetPort string}` + `defaultPort`를 가진다. `Process`는 라우트를 순차 평가(first-match)하여 `_target_port` 메타데이터를 설정하고 메시지를 Clone한다. 매칭 없음 + default → default 포트, 매칭 없음 + default 없음 → 드롭(빈 슬라이스).
  - **핵심 결함**: `Configure`는 `config["routes"]`를 `[]SwitchRoute`(Go 함수 클로저)로만 인식한다. 엔진은 `nd.Config`(JSON/YAML에서 온 원시 `map[string]any`)를 그대로 전달하므로, 라우트가 `[]any`(각 원소는 `map[string]any{name, condition}`)로 도착해 **타입 단언이 실패하고 무시된다**. 결과적으로 플로우 JSON / 에디터에서 switch를 설정할 수 없다.
- `internal/node/condition.go`: 문자열 조건식 엔진 `compileCondition(expr string) (FilterCondition, error)`가 이미 존재하며 `filter.go`에서 사용 중이다. JSONPath(`$.payload.x`, `$.metadata.k`), 연산자(`== != > < >= <=`), 논리(`&& || !`), `exists(path)`, 리터럴(숫자/문자열/bool/null)을 지원한다. switch 라우트 조건에 **이 엔진을 재사용**한다.
- `internal/node/registry.go`: `switch`는 이미 등록되어 있다 → `{"switch", NewSwitchNode, "routing", "조건에 따라 메시지를 라우팅"}`. (재등록/중복 불필요)
- `internal/node/filter.go`: 문자열 조건식 Configure 패턴(`condition` string → `compileCondition`)과 reject 포트(`_target_port` = "reject") 패턴의 참조 구현.
- `internal/node/select_field.go`: drop 포트 + `configBool` (string/bool 모두 허용, Web UI 호환) 패턴의 참조 구현.
- `internal/node/base.go`: `BaseNode.Ports()`는 `inputs/outputs/errorPort`를 합쳐 반환한다. `SwitchNode`는 출력 포트를 `routes` 기반으로 동적 제공하려면 이 동작을 오버라이드해야 한다.
- `web/src/config/nodeSchemas.ts`: `switch` NODE_SCHEMAS 항목이 존재하나 `routes`가 일반 `object`(원시 JSON), `defaultPorts`는 정적 `[in, out]`이다. 헬퍼 `computePortsForNode(nodeType, config)`(~2383행)는 이미 switch 동적 포트를 파생한다 — `route.name`마다 출력 포트 1개 + `default` 포트(routes 비면 `[in, out]`로 폴백).
- `internal/node/switch_test.go`: 라우트 매칭/기본 포트/드롭/동시성을 커버하나, **문자열 표현식 라우트는 커버하지 않는다**.
- 엔진은 `_target_port` 메타데이터를 와이어의 `SourcePort`와 매칭하여 포트별로 라우팅한다(filter.go의 reject 패턴과 동일).

### 1.3 범위

**포함:**
- `SwitchNode.Configure` 확장: `routes`를 `{name, condition}` 구조의 배열로 파싱하고 각 `condition`을 `compileCondition`으로 컴파일.
- `match_mode` 설정(`first` | `all`) 지원.
- `default_port` 설정 처리(기존 동작 보존).
- `SwitchNode.Ports()` 오버라이드: `routes` + `default_port`로부터 출력 포트 동적 파생.
- 프론트엔드 구조화 라우트 에디터(조건식 / 포트명 행 추가·삭제) + 동적 출력 핸들.
- 백엔드↔프론트엔드 포트명 정렬 보증.
- 신규 동작에 대한 테스트 추가.

**제외:**
- 새 노드 타입 추가(기존 `switch` 완성만).
- 조건식 엔진(`condition.go`) 자체 변경 — 재사용만 한다.
- 정규식·사용자 정의 함수 등 조건식 문법 확장(SPEC-FILTER-001 / SPEC-EXPR-001 영역).
- 와이어 자동 재배선(포트 이름 변경 시 무효화된 와이어 처리는 위험요소로 문서화하되 자동 복구는 범위 외).

## 2. 환경

| 항목 | 상세 |
|------|------|
| 런타임 (백엔드) | Go 1.23+ |
| 대상 모듈 (백엔드) | `internal/node/` |
| 의존성 (백엔드) | `pkg/message`, `pkg/flow`, `pkg/lifecycle`, `internal/node` (BaseNode, FilterCondition, compileCondition) |
| 런타임 (프론트엔드) | TypeScript 5.9+, React, React Flow |
| 대상 모듈 (프론트엔드) | `web/src/config/nodeSchemas.ts`, `web/src/components/.../CustomNode.tsx` |
| 테스트 프레임워크 | `testing` + `testify/assert`, `testify/require` (백엔드) |
| 동시성 | `sync.RWMutex` (기존 SwitchNode 패턴 유지) |
| 기존 참조 | `filter.go` (문자열 조건 Configure + reject 포트), `condition.go` (compileCondition), `select_field.go` (configBool, drop 포트), `computePortsForNode` (동적 포트) |

## 3. 가정

- **A1**: `routes`는 플로우 JSON/YAML을 통해 `[]any`로 전달되며, 각 원소는 `map[string]any{"name": string, "condition": string}`이다 (엔진이 `nd.Config`를 그대로 전달).
- **A2**: 라우트 조건식 문자열은 `compileCondition`으로 컴파일되며, 문법은 SPEC-FILTER-001에 정의된 것과 동일하다.
- **A3**: 라우트의 `name` 값이 곧 출력 포트 이름이며, 와이어의 `SourcePort`와 `_target_port` 메타데이터와 동일해야 한다 (3-way 정렬: `route.name` = 포트명 = `SourcePort` = `_target_port`).
- **A4**: `match_mode`는 문자열로 전달되며 `first`(기본) 또는 `all`이다.
- **A5**: 조건식 컴파일은 Configure 시점에 1회 수행하고, 컴파일된 클로저를 Process에서 재사용한다.
- **A6**: 기존 동작(Go 함수 `[]SwitchRoute` 직접 주입, 테스트에서 사용)은 보존한다 — 문자열 경로는 추가 경로이다.
- **A7**: `default_port`가 비어 있거나 미설정이면 미매칭 메시지는 드롭된다(기존 동작 보존).

## 4. 요구사항

### 4.1 문자열 조건 라우트 설정 (Configure)

**REQ-SWITCH-001**: 구조화 라우트 파싱
**WHEN** `config["routes"]`가 `[]any`(각 원소가 `{name, condition}` 맵)로 전달되면, **THEN** 시스템은 각 원소에서 `name`(문자열)과 `condition`(문자열)을 추출하고, `condition`을 `compileCondition`으로 컴파일하여 내부 `SwitchRoute{Condition, TargetPort: name}`를 구성해야 한다.

**REQ-SWITCH-002**: 잘못된 표현식 에러 보고
**IF** 임의의 라우트 `condition`이 유효하지 않은 표현식이면, **THEN** 시스템은 어느 라우트(이름/인덱스)에서 실패했는지 식별 가능한 메시지를 포함한 명확한 에러를 `Configure`에서 반환해야 한다.

**REQ-SWITCH-003**: 라우트 필드 검증
**IF** 라우트 원소에 `name`이 비어 있거나, `condition`이 비어 있거나, 타입이 문자열이 아니면, **THEN** 시스템은 `Configure`에서 명확한 에러를 반환해야 한다.

**REQ-SWITCH-004**: 기존 Go 함수 라우트 보존
**WHERE** `config["routes"]`가 `[]SwitchRoute`(Go 함수 클로저) 타입으로 전달되면, 시스템은 기존 동작대로 해당 라우트를 그대로 설정해야 한다 (하위 호환).

### 4.2 매칭 모드 (match_mode)

**REQ-SWITCH-010**: 매칭 모드 설정
시스템은 **항상** `config["match_mode"]` 값을 `first` 또는 `all`로 해석하며, 미설정·미인식 값일 경우 `first`를 기본값으로 사용해야 한다.

**REQ-SWITCH-011**: first 모드 라우팅
**WHILE** `match_mode`가 `first`인 동안, **WHEN** 메시지를 처리하면, 시스템은 라우트를 순차 평가하여 **첫 번째로 매칭되는** 라우트의 포트로만 메시지(Clone) 1건을 라우팅해야 한다.

**REQ-SWITCH-012**: all 모드 팬아웃
**WHILE** `match_mode`가 `all`인 동안, **WHEN** 메시지를 처리하면, 시스템은 **매칭되는 모든** 라우트의 포트로 각각 메시지 Clone을 팬아웃해야 한다 (멀티 라벨, 출력 N건).

### 4.3 미매칭 처리 (default_port)

**REQ-SWITCH-020**: 기본 포트 라우팅
**WHERE** `config["default_port"]`가 비어 있지 않은 문자열로 설정되어 있고, **IF** 어떤 라우트도 매칭되지 않으면, **THEN** 시스템은 메시지(Clone)를 해당 `default_port`로 라우팅해야 한다.

**REQ-SWITCH-021**: 미설정 시 드롭
**IF** `default_port`가 비어 있거나 미설정이고 어떤 라우트도 매칭되지 않으면, **THEN** 시스템은 아무것도 emit하지 않아야 한다 (드롭, 빈 슬라이스 — 기존 동작 보존).

**REQ-SWITCH-022**: all 모드와 default 상호작용
**WHILE** `match_mode`가 `all`인 동안, **IF** 매칭되는 라우트가 하나도 없으면, 시스템은 REQ-SWITCH-020 / REQ-SWITCH-021의 default/드롭 규칙을 적용해야 한다. 매칭이 하나라도 있으면 default는 사용하지 않는다.

### 4.4 동적 출력 포트 (Ports)

**REQ-SWITCH-030**: routes 기반 포트 파생
시스템은 **항상** `SwitchNode.Ports()`가 입력 포트 `in`과 더불어, 설정된 각 `route.name`을 출력 포트로, 그리고 `default_port`가 설정된 경우 그 포트를 출력 포트로 포함하여 반환하도록 해야 한다.

**REQ-SWITCH-031**: routes 비었을 때 폴백
**IF** `routes`가 비어 있으면, **THEN** `Ports()`는 입력 `in` + 출력 `out`(폴백)을 반환해야 한다 (프론트엔드 `computePortsForNode` 폴백 동작과 정렬).

**REQ-SWITCH-032**: 백엔드↔프론트엔드 포트명 정렬
시스템은 **항상** 백엔드 `Ports()`가 파생하는 출력 포트 이름 집합이 프론트엔드 `computePortsForNode('switch', config)`가 파생하는 포트 이름 집합과 동일하도록 보장해야 한다 (`route.name` = 포트명 = 와이어 `SourcePort` = `_target_port`).

**REQ-SWITCH-033**: 런타임 반영
**WHEN** `Configure`로 `routes` 또는 `default_port`가 갱신되면, **THEN** 이후 `Ports()` 호출은 갱신된 라우트를 반영해야 한다.

### 4.5 구조화 라우트 편집 UI

**REQ-SWITCH-040**: 구조화 에디터 렌더링
시스템은 **항상** switch 노드의 설정 패널에서 원시 JSON `routes` 필드 대신, (조건식 expression, 포트명 name) 쌍의 행 목록을 한국어 라벨로 렌더링해야 한다.

**REQ-SWITCH-041**: 행 추가/삭제
**WHEN** 사용자가 행 추가/삭제를 수행하면, **THEN** 시스템은 `routes` 배열에 `{name, condition}` 원소를 추가/제거하고 설정을 갱신해야 한다.

**REQ-SWITCH-042**: 동적 출력 핸들
**WHEN** `routes`가 변경되면, **THEN** 노드 카드는 `computePortsForNode` 결과에 따라 출력 핸들을 라우트별 + default로 자동 재생성해야 한다.

**REQ-SWITCH-043**: match_mode / default_port 입력
시스템은 **항상** `match_mode`(first/all 선택)와 `default_port`(문자열, 선택) 설정 입력을 한국어 라벨로 제공해야 한다.

### 4.6 하위 호환

**REQ-SWITCH-050**: 기존 플로우 보존
**WHERE** 기존 플로우의 switch 노드가 `routes`를 갖지 않거나 빈 배열을 가지면, 시스템은 기존 동작([in, out] 패스스루/기본 포트)을 유지해야 한다.

## 5. 명세 (설정 키 계약)

### 5.1 설정 키 (config map)

| 키 | 타입 | 필수 | 기본값 | 설명 |
|----|------|------|--------|------|
| `routes` | `[]any` (각 `{name: string, condition: string}`) 또는 `[]SwitchRoute` | 아니오 | `[]` | 라우팅 규칙 배열. 비면 `[in, out]` 폴백. |
| `match_mode` | `string` (`first` \| `all`) | 아니오 | `first` | 매칭 처리 모드. |
| `default_port` | `string` | 아니오 | `""` | 미매칭 시 포트. 비면 드롭. |

### 5.2 라우트 원소 스키마

```
{
  "name": "hot",                       // 출력 포트 이름 (= SourcePort = _target_port)
  "condition": "$.payload.temp >= 30"  // compileCondition 표현식
}
```

### 5.3 포트 파생 규칙

- 입력: 항상 `in`.
- 출력: `routes`가 비면 `out`; 아니면 각 `route.name` + (`default_port`가 설정된 경우) `default_port`.
- 정렬: 백엔드 `Ports()` 출력 = 프론트엔드 `computePortsForNode` 출력.

### 5.4 라우팅 메타데이터

- 매칭된 메시지는 Clone되어 `_target_port` 메타데이터에 대상 포트 이름이 설정된다.
- 엔진은 `_target_port`를 와이어의 `SourcePort`와 매칭하여 포트별로 전달한다.

## 6. 추적성 (Traceability)

| 요구사항 | 구현 대상 | 검증 (acceptance) |
|----------|-----------|-------------------|
| REQ-SWITCH-001..004 | `switch.go: Configure` | AC-SWITCH-001..004 |
| REQ-SWITCH-010..012 | `switch.go: Configure/Process` | AC-SWITCH-010..012 |
| REQ-SWITCH-020..022 | `switch.go: Process` | AC-SWITCH-020..022 |
| REQ-SWITCH-030..033 | `switch.go: Ports` | AC-SWITCH-030..033 |
| REQ-SWITCH-040..043 | `nodeSchemas.ts`, `CustomNode.tsx` | AC-SWITCH-040..043 |
| REQ-SWITCH-050 | `switch.go`, `computePortsForNode` | AC-SWITCH-050 |

세부 계획은 `plan.md`, 인수 기준은 `acceptance.md`를 참조한다.
