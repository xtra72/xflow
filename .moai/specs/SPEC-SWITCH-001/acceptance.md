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
| 1.0.0 | 2026-06-03 | xtra | 초기 인수 기준 작성 (PLAN 단계) |
| 1.1.0 | 2026-06-04 | xtra | 구현 반영 — `pass_mode`(copy/original) 인수 기준 추가 (AC-SWITCH-060..063) |

# SPEC-SWITCH-001: 인수 기준 (Acceptance Criteria)

## 1. Configure — 문자열 조건 라우트

### AC-SWITCH-001: 구조화 라우트 파싱 및 라우팅 (REQ-SWITCH-001)

```gherkin
Given switch 노드에 config가 전달된다
  And routes = [
        {name: "hot", condition: "$.payload.temp >= 30"},
        {name: "cold", condition: "$.payload.temp < 10"}
      ]
When Configure를 호출하면
Then 에러 없이 라우트가 설정된다
When payload.temp = 35 인 메시지를 Process하면
Then 출력 1건이 반환되고 _target_port == "hot" 이다
When payload.temp = 5 인 메시지를 Process하면
Then 출력 1건이 반환되고 _target_port == "cold" 이다
```

### AC-SWITCH-002: 잘못된 표현식 → Configure 에러 (REQ-SWITCH-002)

```gherkin
Given routes = [{name: "bad", condition: "$.payload.x >>> 1"}]
When Configure를 호출하면
Then Configure는 에러를 반환한다
  And 에러 메시지는 실패한 라우트("bad" 또는 인덱스)를 식별할 수 있다
```

### AC-SWITCH-003: 라우트 필드 검증 (REQ-SWITCH-003)

```gherkin
Given routes = [{name: "", condition: "$.payload.x == 1"}]
When Configure를 호출하면
Then Configure는 빈 name에 대한 에러를 반환한다

Given routes = [{name: "ok", condition: ""}]
When Configure를 호출하면
Then Configure는 빈 condition에 대한 에러를 반환한다

Given routes = [{name: "ok", condition: 42}]   # 비문자열
When Configure를 호출하면
Then Configure는 타입 에러를 반환한다
```

### AC-SWITCH-004: 기존 Go 함수 라우트 보존 (REQ-SWITCH-004, 하위 호환)

```gherkin
Given routes = []SwitchRoute{ {Condition: func(m) bool {return true}, TargetPort: "a"} }
When Configure를 호출하고 메시지를 Process하면
Then 에러 없이 _target_port == "a" 로 라우팅된다
```

## 2. 매칭 모드 (match_mode)

### AC-SWITCH-010: 기본 모드 first (REQ-SWITCH-010)

```gherkin
Given match_mode가 config에 없다
When Configure 후 라우팅 동작을 확인하면
Then first 모드로 동작한다 (첫 매칭 1건만 출력)
```

### AC-SWITCH-011: first 모드 — 첫 매칭만 (REQ-SWITCH-011)

```gherkin
Given match_mode = "first"
  And routes = [
        {name: "warm", condition: "$.payload.temp >= 20"},
        {name: "hot",  condition: "$.payload.temp >= 30"}
      ]
When payload.temp = 35 인 메시지를 Process하면
Then 출력은 정확히 1건이고 _target_port == "warm" 이다 (첫 매칭)
```

### AC-SWITCH-012: all 모드 — 다중 포트 팬아웃 (REQ-SWITCH-012)

```gherkin
Given match_mode = "all"
  And routes = [
        {name: "warm", condition: "$.payload.temp >= 20"},
        {name: "hot",  condition: "$.payload.temp >= 30"}
      ]
When payload.temp = 35 인 메시지를 Process하면
Then 출력은 2건이다
  And _target_port 집합 == {"warm", "hot"}
  And 각 출력은 원본의 Clone이다
```

## 3. 미매칭 처리 (default_port / 드롭)

### AC-SWITCH-020: default_port로 라우팅 (REQ-SWITCH-020)

```gherkin
Given routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
  And default_port = "other"
When payload.temp = 5 인 (미매칭) 메시지를 Process하면
Then 출력 1건이 반환되고 _target_port == "other" 이다
```

### AC-SWITCH-021: default 없으면 드롭 (REQ-SWITCH-021)

```gherkin
Given routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
  And default_port 미설정
When payload.temp = 5 인 (미매칭) 메시지를 Process하면
Then 출력은 빈 슬라이스이다 (드롭, 아무것도 emit하지 않음)
```

### AC-SWITCH-022: all 모드 + 미매칭 → default/드롭 (REQ-SWITCH-022)

```gherkin
Given match_mode = "all"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
  And default_port = "other"
When payload.temp = 5 인 (전부 미매칭) 메시지를 Process하면
Then 출력 1건이 반환되고 _target_port == "other" 이다

Given match_mode = "all"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
  And default_port 미설정
When payload.temp = 5 인 메시지를 Process하면
Then 출력은 빈 슬라이스이다 (드롭)
```

## 4. 동적 출력 포트 (Ports)

### AC-SWITCH-030: routes 기반 포트 파생 (REQ-SWITCH-030)

```gherkin
Given routes = [{name: "hot", condition: "..."}, {name: "cold", condition: "..."}]
  And default_port = "other"
When Ports()를 호출하면
Then 입력 포트 "in" 이 포함된다
  And 출력 포트 "hot", "cold", "other" 가 포함된다
```

### AC-SWITCH-031: routes 비었을 때 폴백 (REQ-SWITCH-031)

```gherkin
Given routes 미설정 또는 빈 배열
When Ports()를 호출하면
Then 포트는 입력 "in" + 출력 "out" 이다
```

### AC-SWITCH-032: 백엔드↔프론트엔드 포트명 정렬 (REQ-SWITCH-032)

```gherkin
Given 동일한 config (routes + default_port)
When 백엔드 Ports()와 프론트엔드 computePortsForNode('switch', config)를 각각 평가하면
Then 출력 포트 이름 집합이 동일하다
  And route.name = 포트명 = 와이어 SourcePort = _target_port 규약이 성립한다
```

### AC-SWITCH-033: Configure 후 Ports 갱신 (REQ-SWITCH-033)

```gherkin
Given switch 노드가 routes = [{name: "a", ...}] 로 설정되어 있다
When routes = [{name: "a", ...}, {name: "b", ...}] 로 재Configure하면
Then 이후 Ports()는 출력 포트 "a", "b"를 반영한다
```

## 5. 구조화 라우트 편집 UI

### AC-SWITCH-040: 구조화 에디터 렌더링 (REQ-SWITCH-040)

```gherkin
Given switch 노드의 설정 패널을 연다
When 라우트 섹션을 보면
Then 원시 JSON 입력 대신 (조건식, 포트명) 행 목록이 한국어 라벨로 표시된다
```

### AC-SWITCH-041: 행 추가/삭제 (REQ-SWITCH-041)

```gherkin
Given 라우트 에디터에 1개 행이 있다
When "행 추가"를 클릭하고 조건식/포트명을 입력하면
Then config.routes에 {name, condition} 원소가 1개 추가된다
When 한 행의 "삭제"를 클릭하면
Then 해당 routes 원소가 제거된다
```

### AC-SWITCH-042: 동적 출력 핸들 (REQ-SWITCH-042)

```gherkin
Given 라우트 2개와 default_port가 설정된 노드
When 노드 카드를 렌더링하면
Then 출력 핸들이 라우트별 + default 로 자동 생성된다 (computePortsForNode 결과와 일치)
```

### AC-SWITCH-043: match_mode / default_port 입력 (REQ-SWITCH-043)

```gherkin
Given switch 설정 패널
When 설정 필드를 보면
Then match_mode(first/all 선택)와 default_port(문자열) 입력이 한국어 라벨로 제공된다
```

## 6. 하위 호환

### AC-SWITCH-050: 기존 플로우 보존 (REQ-SWITCH-050)

```gherkin
Given 기존 switch 노드가 routes 없이 저장되어 있다
When 플로우를 로드하고 메시지를 Process하면
Then 노드는 [in, out] 포트로 동작하며 회귀가 없다 (기존 동작 유지)
```

## 6-b. 전달 모드 (pass_mode)

### AC-SWITCH-060: 기본 모드 copy + 미인식 값 폴백 (REQ-SWITCH-060)

```gherkin
Given pass_mode가 config에 없다
When Configure 후 라우팅 동작을 확인하면
Then copy 모드로 동작한다 (라우팅 시 Clone, 새 ID)

Given pass_mode = "weird"   # 미인식 값
When Configure를 호출하면
Then copy 모드로 폴백한다 (에러 없음)
```

### AC-SWITCH-061: copy 모드 — 방출 메시지 ID 변경 (REQ-SWITCH-061)

```gherkin
Given pass_mode = "copy"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
When payload.temp = 35 인 메시지(원본 ID = X)를 Process하면
Then 출력 1건이 반환되고 _target_port == "hot" 이다
  And 방출 메시지의 ID는 X 와 다르다 (Clone — 새 ID)
```

### AC-SWITCH-062: original 모드 — 단일 매칭 시 원본 ID 보존 (REQ-SWITCH-062)

```gherkin
Given pass_mode = "original"
  And match_mode = "first"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
When payload.temp = 35 인 메시지(원본 ID = X)를 Process하면
Then 출력 1건이 반환되고 _target_port == "hot" 이다
  And 방출 메시지의 ID == X 이다 (원본 그대로, Clone 없음)

Given pass_mode = "original"
  And match_mode = "all"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
When payload.temp = 35 인 (정확히 1건 매칭) 메시지(원본 ID = X)를 Process하면
Then 출력 1건이 반환되고 _target_port == "hot" 이며 ID == X 이다 (원본 보존)
```

### AC-SWITCH-063: original 모드 — default_port 경로 원본 보존 (REQ-SWITCH-062)

```gherkin
Given pass_mode = "original"
  And routes = [{name: "hot", condition: "$.payload.temp >= 30"}]
  And default_port = "other"
When payload.temp = 5 인 (미매칭) 메시지(원본 ID = X)를 Process하면
Then 출력 1건이 반환되고 _target_port == "other" 이며 ID == X 이다 (원본 보존)
```

### AC-SWITCH-064: original 모드 — all 다중 매칭은 Clone 강제 (REQ-SWITCH-063)

```gherkin
Given pass_mode = "original"
  And match_mode = "all"
  And routes = [
        {name: "warm", condition: "$.payload.temp >= 20"},
        {name: "hot",  condition: "$.payload.temp >= 30"}
      ]
When payload.temp = 35 인 (2건 매칭) 메시지(원본 ID = X)를 Process하면
Then 출력은 2건이고 _target_port 집합 == {"warm", "hot"} 이다
  And 각 출력은 원본의 Clone 이며 ID 가 X 와 다르다
      (단일 객체가 서로 다른 _target_port 를 동시에 가질 수 없으므로 Clone 강제)
```

## 7. 품질 게이트

- `go test ./internal/node/...` 전부 통과.
- switch 패키지 라인 커버리지 ≥ 85%.
- `go vet` / `golangci-lint` 무경고.
- 동시성: `go test -race ./internal/node/...` 통과 (Configure/Process/Ports 동시 호출).
- 기존 `switch_test.go`의 모든 테스트(라우트 매칭, 기본 포트, 드롭, 동시성)가 그대로 통과.
