---
id: SPEC-FILTER-001
title: "Filter Node Condition Expression Parser"
version: "1.1.0"
status: completed
created: "2026-02-23"
updated: "2026-02-23"
author: "xtra"
priority: high
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-23 | xtra | 초기 인수 기준 작성 |
| 1.1.0 | 2026-02-23 | xtra | 구현 완료 - Implementation Notes 추가 |

# SPEC-FILTER-001: 인수 기준 (Acceptance Criteria)

## 1. 렉서 테스트 시나리오

### AC-FILTER-001: 기본 토큰 인식 (REQ-FILTER-001)

**Scenario: JSONPath 토큰 인식**
```gherkin
Given 조건식 문자열 "$.payload.temperature >= 30"
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.payload.temperature"), COMPARE_OP(">="), NUMBER("30"), EOF] 를 반환한다
```

**Scenario: 논리 연산자 토큰 인식**
```gherkin
Given 조건식 문자열 "$.a > 1 && $.b < 2"
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.a"), COMPARE_OP(">"), NUMBER("1"), LOGICAL_OP("&&"), PATH("$.b"), COMPARE_OP("<"), NUMBER("2"), EOF] 를 반환한다
```

**Scenario: 문자열 리터럴 토큰 인식 (작은따옴표)**
```gherkin
Given 조건식 문자열 "$.status == 'active'"
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.status"), COMPARE_OP("=="), STRING("active"), EOF] 를 반환한다
```

**Scenario: 문자열 리터럴 토큰 인식 (큰따옴표)**
```gherkin
Given 조건식 문자열 '$.status == "active"'
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.status"), COMPARE_OP("=="), STRING("active"), EOF] 를 반환한다
```

**Scenario: 불리언/null 리터럴 토큰 인식**
```gherkin
Given 조건식 문자열 "$.enabled == true"
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.enabled"), COMPARE_OP("=="), BOOL("true"), EOF] 를 반환한다
```

**Scenario: 음수 숫자 리터럴 인식**
```gherkin
Given 조건식 문자열 "$.temperature >= -40.5"
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.temperature"), COMPARE_OP(">="), NUMBER("-40.5"), EOF] 를 반환한다
```

**Scenario: exists 함수 토큰 인식**
```gherkin
Given 조건식 문자열 "exists($.payload.error)"
When tokenize 함수를 호출하면
Then 토큰 배열 [FUNC("exists"), LPAREN, PATH("$.payload.error"), RPAREN, EOF] 를 반환한다
```

**Scenario: 부정 연산자 토큰 인식**
```gherkin
Given 조건식 문자열 "!exists($.error)"
When tokenize 함수를 호출하면
Then 토큰 배열 [NOT("!"), FUNC("exists"), LPAREN, PATH("$.error"), RPAREN, EOF] 를 반환한다
```

### AC-FILTER-002: 공백 처리 (REQ-FILTER-002)

**Scenario: 다양한 공백 무시**
```gherkin
Given 조건식 문자열 "  $.a   >=   30  "
When tokenize 함수를 호출하면
Then 토큰 배열 [PATH("$.a"), COMPARE_OP(">="), NUMBER("30"), EOF] 를 반환한다
```

### AC-FILTER-003: 렉서 에러 보고 (REQ-FILTER-003)

**Scenario: 인식 불가 문자 에러**
```gherkin
Given 조건식 문자열 "$.value @ 30"
When tokenize 함수를 호출하면
Then ErrInvalidExpression 에러를 반환한다
And 에러 메시지에 위치 정보가 포함되어 있다
```

## 2. 파서 테스트 시나리오

### AC-FILTER-010: 비교 표현식 파싱 (REQ-FILTER-010)

**Scenario: 단순 비교식 AST 생성**
```gherkin
Given 토큰 배열 [PATH("$.value"), COMPARE_OP(">="), NUMBER("30"), EOF]
When parse 함수를 호출하면
Then ComparisonNode{Left: PathExpr{Path: "$.value"}, Op: GTE, Right: LiteralExpr{Value: 30.0, Type: Numeric}} AST를 반환한다
```

### AC-FILTER-011: 논리 AND 파싱 (REQ-FILTER-011)

**Scenario: AND 논리식 AST 생성**
```gherkin
Given 조건식 "$.a > 1 && $.b < 2"
When tokenize 후 parse 함수를 호출하면
Then LogicalNode{Left: ComparisonNode($.a > 1), Op: AND, Right: ComparisonNode($.b < 2)} AST를 반환한다
```

### AC-FILTER-012: 논리 OR 파싱 (REQ-FILTER-012)

**Scenario: OR 논리식 AST 생성**
```gherkin
Given 조건식 "$.type == 'a' || $.type == 'b'"
When tokenize 후 parse 함수를 호출하면
Then LogicalNode{Left: ComparisonNode, Op: OR, Right: ComparisonNode} AST를 반환한다
```

### AC-FILTER-013: 괄호 그룹 파싱 (REQ-FILTER-013)

**Scenario: 괄호로 우선순위 변경**
```gherkin
Given 조건식 "($.a == 1 || $.b == 2) && $.c == 3"
When tokenize 후 parse 함수를 호출하면
Then LogicalNode{Left: LogicalNode(OR), Op: AND, Right: ComparisonNode} AST를 반환한다
And 괄호 내 OR가 AND보다 먼저 그룹화되었다
```

### AC-FILTER-014: 부정 표현식 파싱 (REQ-FILTER-014)

**Scenario: 부정 표현식 AST 생성**
```gherkin
Given 조건식 "!($.value > 100)"
When tokenize 후 parse 함수를 호출하면
Then NotNode{Expr: ComparisonNode($.value > 100)} AST를 반환한다
```

### AC-FILTER-015: exists 함수 파싱 (REQ-FILTER-015)

**Scenario: exists 함수 AST 생성**
```gherkin
Given 조건식 "exists($.payload.error)"
When tokenize 후 parse 함수를 호출하면
Then ExistsNode{Path: "$.payload.error"} AST를 반환한다
```

### AC-FILTER-016: 연산자 우선순위 (REQ-FILTER-016)

**Scenario: AND가 OR보다 높은 우선순위**
```gherkin
Given 조건식 "$.a == 1 || $.b == 2 && $.c == 3"
When tokenize 후 parse 함수를 호출하면
Then LogicalNode{Left: ComparisonNode($.a == 1), Op: OR, Right: LogicalNode{Left: ComparisonNode($.b == 2), Op: AND, Right: ComparisonNode($.c == 3)}} AST를 반환한다
And && 가 || 보다 먼저 결합되었다
```

### AC-FILTER-017: 파서 에러 보고 (REQ-FILTER-017)

**Scenario: 문법 오류 시 에러 반환**
```gherkin
Given 조건식 "$.value >="
When tokenize 후 parse 함수를 호출하면
Then ErrInvalidExpression 에러를 반환한다
And 에러 메시지에 예상 토큰 정보가 포함되어 있다
```

**Scenario: 닫히지 않은 괄호 에러**
```gherkin
Given 조건식 "($.value > 1"
When tokenize 후 parse 함수를 호출하면
Then ErrInvalidExpression 에러를 반환한다
```

## 3. 평가기 테스트 시나리오

### AC-FILTER-020: 숫자 비교 평가 (REQ-FILTER-020, REQ-FILTER-021)

**Scenario: 숫자 비교 - 크거나 같음**
```gherkin
Given 메시지 payload {"temperature": 35.0}
And 조건식 "$.payload.temperature >= 30"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: 숫자 비교 - 미만**
```gherkin
Given 메시지 payload {"temperature": 25.0}
And 조건식 "$.payload.temperature >= 30"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
```

**Scenario: 정수 타입 자동 변환**
```gherkin
Given 메시지 payload {"count": 10}  (int 타입)
And 조건식 "$.payload.count > 5"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
And int 가 float64 로 자동 변환되어 비교되었다
```

### AC-FILTER-022: 문자열 비교 평가 (REQ-FILTER-022)

**Scenario: 문자열 동등 비교**
```gherkin
Given 메시지 payload {"status": "active"}
And 조건식 "$.payload.status == 'active'"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: 문자열 부등 비교**
```gherkin
Given 메시지 payload {"status": "inactive"}
And 조건식 "$.payload.status != 'active'"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: 문자열에 대한 크기 비교는 false**
```gherkin
Given 메시지 payload {"name": "abc"}
And 조건식 "$.payload.name > 'aaa'"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
```

### AC-FILTER-023: null 비교 평가 (REQ-FILTER-023)

**Scenario: null 동등 비교 - 필드가 nil**
```gherkin
Given 메시지 payload {"value": nil}
And 조건식 "$.payload.value == null"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: null 부등 비교 - 필드가 존재하고 nil이 아님**
```gherkin
Given 메시지 payload {"value": 42}
And 조건식 "$.payload.value != null"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

### AC-FILTER-024: 논리 AND/OR 평가 (REQ-FILTER-024)

**Scenario: AND 단락 평가 - 왼쪽 false**
```gherkin
Given 메시지 payload {"a": 0, "b": 100}
And 조건식 "$.payload.a > 5 && $.payload.b > 50"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
And 왼쪽이 false이므로 오른쪽은 평가되지 않았다
```

**Scenario: OR 단락 평가 - 왼쪽 true**
```gherkin
Given 메시지 payload {"a": 10, "b": 0}
And 조건식 "$.payload.a > 5 || $.payload.b > 50"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
And 왼쪽이 true이므로 오른쪽은 평가되지 않았다
```

**Scenario: 범위 조건 (AND)**
```gherkin
Given 메시지 payload {"temperature": 25.0}
And 조건식 "$.payload.temperature >= -40 && $.payload.temperature <= 150"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

### AC-FILTER-025: 부정 평가 (REQ-FILTER-025)

**Scenario: 부정 연산**
```gherkin
Given 메시지 payload {"value": 3}
And 조건식 "!($.payload.value > 5)"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

### AC-FILTER-026: exists 함수 평가 (REQ-FILTER-026)

**Scenario: 필드 존재 - true**
```gherkin
Given 메시지 payload {"error": "timeout"}
And 조건식 "exists($.payload.error)"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: 필드 미존재 - false**
```gherkin
Given 메시지 payload {"value": 42}
And 조건식 "exists($.payload.error)"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
```

**Scenario: 부정 exists**
```gherkin
Given 메시지 payload {"value": 42}
And 조건식 "!exists($.payload.error)"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

### AC-FILTER-027: 경로 미존재 처리 (REQ-FILTER-027)

**Scenario: 비교 시 경로 미존재는 false**
```gherkin
Given 메시지 payload {"other": 42}
And 조건식 "$.payload.value > 10"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
```

### AC-FILTER-028: 불리언 비교 평가 (REQ-FILTER-028)

**Scenario: 불리언 동등 비교**
```gherkin
Given 메시지 payload {"enabled": true}
And 조건식 "$.payload.enabled == true"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 true 이다
```

**Scenario: 불리언에 대한 크기 비교는 false**
```gherkin
Given 메시지 payload {"enabled": true}
And 조건식 "$.payload.enabled > false"
When 조건식을 컴파일하고 메시지에 대해 평가하면
Then 결과는 false 이다
```

## 4. FilterNode 통합 테스트 시나리오

### AC-FILTER-030: Configure - Go 함수 우선 (REQ-FILTER-030)

**Scenario: Go 함수 타입 우선 적용**
```gherkin
Given FilterNode 인스턴스
And config에 FilterCondition Go 함수가 설정되어 있음
When Configure를 호출하면
Then 에러 없이 성공한다
And Go 함수가 condition으로 설정되었다
```

### AC-FILTER-031: Configure - 문자열 조건식 (REQ-FILTER-031)

**Scenario: 문자열 조건식 파싱 및 설정**
```gherkin
Given FilterNode 인스턴스
And config = {"condition": "$.payload.temperature >= 30"}
When Configure를 호출하면
Then 에러 없이 성공한다
And 파싱된 FilterCondition이 condition으로 설정되었다
```

### AC-FILTER-032: Configure - 파싱 에러 (REQ-FILTER-032)

**Scenario: 유효하지 않은 조건식**
```gherkin
Given FilterNode 인스턴스
And config = {"condition": "invalid expression %%%"}
When Configure를 호출하면
Then ErrInvalidExpression 에러를 반환한다
```

### AC-FILTER-033: Configure - 빈 문자열 (REQ-FILTER-033)

**Scenario: 빈 조건식 문자열**
```gherkin
Given FilterNode 인스턴스
And config = {"condition": ""}
When Configure를 호출하면
Then ErrInvalidExpression 에러를 반환한다
```

### AC-FILTER-034: Configure - condition 미설정 (REQ-FILTER-034)

**Scenario: condition 키 없음**
```gherkin
Given FilterNode 인스턴스
And config = {"other_key": "value"}
When Configure를 호출하면
Then 에러 없이 성공한다
And condition은 nil 상태이다
```

### AC-FILTER-040: 하위 호환성 (REQ-FILTER-040, REQ-FILTER-041)

**Scenario: 기존 Go 함수 condition 동작 보존**
```gherkin
Given FilterNode에 Go 함수 condition이 설정되어 있음 (항상 true 반환)
And 메시지가 준비되어 있음
When Process를 호출하면
Then 메시지가 output 포트로 전송된다
And ErrFilterRejected 가 반환되지 않는다
```

**Scenario: condition nil 시 pass-through**
```gherkin
Given FilterNode에 condition이 설정되지 않음 (nil)
And 메시지가 준비되어 있음
When Process를 호출하면
Then 메시지가 그대로 통과한다
```

**Scenario: condition false 시 거부**
```gherkin
Given FilterNode에 조건식 "$.payload.value > 100" 이 설정되어 있음
And 메시지 payload {"value": 50}
When Process를 호출하면
Then ErrFilterRejected 에러를 반환한다
```

### AC-FILTER-E2E: 문자열 조건식 End-to-End

**Scenario: Configure에서 Process까지 전체 흐름**
```gherkin
Given FilterNode 인스턴스
And config = {"condition": "$.payload.temperature >= -40 && $.payload.temperature <= 150"}
When Configure를 호출하면
Then 에러 없이 성공한다

Given 위에서 설정된 FilterNode
And 메시지 payload {"temperature": 25.0}
When Process를 호출하면
Then 메시지가 output 포트로 전송된다

Given 위에서 설정된 FilterNode
And 메시지 payload {"temperature": -50.0}
When Process를 호출하면
Then ErrFilterRejected 에러를 반환한다
```

**Scenario: 복합 조건식 End-to-End**
```gherkin
Given FilterNode 인스턴스
And config = {"condition": "!exists($.payload.error) && $.payload.value > 0"}
When Configure를 호출하면
Then 에러 없이 성공한다

Given 위에서 설정된 FilterNode
And 메시지 payload {"value": 42}
When Process를 호출하면
Then 메시지가 output 포트로 전송된다

Given 위에서 설정된 FilterNode
And 메시지 payload {"value": 42, "error": "timeout"}
When Process를 호출하면
Then ErrFilterRejected 에러를 반환한다
```

## 5. 동시성 테스트 시나리오

### AC-FILTER-050: 동시 Process 안전 (REQ-FILTER-050)

**Scenario: 다중 고루틴 동시 Process**
```gherkin
Given FilterNode에 조건식 "$.payload.value > 0" 이 설정되어 있음
When 100개의 고루틴에서 동시에 다양한 메시지로 Process를 호출하면
Then 모든 호출이 데이터 레이스 없이 완료된다
And go test -race 플래그로 검증 통과한다
```

### AC-FILTER-051: Configure/Process 동시 안전 (REQ-FILTER-051)

**Scenario: Configure와 Process 동시 호출**
```gherkin
Given FilterNode 인스턴스
When 한 고루틴에서 Configure를 반복 호출하고
And 다른 고루틴에서 Process를 반복 호출하면
Then 데이터 레이스 없이 완료된다
And go test -race 플래그로 검증 통과한다
```

## 6. 복합 시나리오 (Edge Cases)

**Scenario: 깊은 중첩 괄호**
```gherkin
Given 조건식 "(($.a > 1) && ($.b < 2)) || (!($.c == 3))"
When 조건식을 컴파일하면
Then 에러 없이 성공한다
And 올바른 AST가 생성된다
```

**Scenario: 여러 필드 참조**
```gherkin
Given 메시지 payload {"temperature": 25, "humidity": 60, "status": "active"}
And 조건식 "$.payload.temperature > 20 && $.payload.humidity < 80 && $.payload.status == 'active'"
When 조건식을 컴파일하고 평가하면
Then 결과는 true 이다
```

**Scenario: 음수 값 비교**
```gherkin
Given 메시지 payload {"temperature": -30}
And 조건식 "$.payload.temperature >= -40.5"
When 조건식을 컴파일하고 평가하면
Then 결과는 true 이다
```

**Scenario: 존재하지 않는 경로와 null 비교**
```gherkin
Given 메시지 payload {"other": 42}
And 조건식 "$.payload.missing == null"
When 조건식을 컴파일하고 평가하면
Then 결과는 true 이다 (경로 미존재는 null과 동등)
```

## 7. 품질 게이트

### 7.1 Definition of Done

| 항목 | 기준 | 검증 방법 |
|------|------|-----------|
| 기능 완성도 | 모든 REQ-FILTER-XXX 요구사항 충족 | 인수 테스트 전체 PASS |
| 테스트 커버리지 | condition.go 85% 이상 | `go test -cover` |
| 동시성 안전 | 데이터 레이스 없음 | `go test -race` |
| 하위 호환성 | 기존 FilterNode 테스트 회귀 없음 | 기존 테스트 전체 PASS |
| 코드 품질 | go vet 통과 | `go vet ./...` |
| 에러 처리 | 모든 에러 경로 테스트 완료 | 에러 케이스 테스트 PASS |

### 7.2 검증 방법

| 검증 항목 | 도구 | 명령어 |
|-----------|------|--------|
| 단위 테스트 | go test | `go test ./internal/node/... -v` |
| 커버리지 | go test -cover | `go test -cover -coverprofile=coverage.out ./internal/node/...` |
| 레이스 감지 | go test -race | `go test -race ./internal/node/...` |
| 정적 분석 | go vet | `go vet ./...` |
| 회귀 테스트 | go test | `go test ./internal/node/... -run TestFilter` |

### 7.3 TRUST 5 품질 프레임워크 매핑

| TRUST 차원 | 적용 기준 |
|------------|-----------|
| **Tested** | condition_test.go + filter_test.go 커버리지 85%+, -race 통과 |
| **Readable** | 명확한 함수명, 한국어 주석, 일관된 코드 스타일 |
| **Unified** | 기존 expression.go/transform.go 패턴과 일관된 구조 |
| **Secured** | 입력 검증 (빈 문자열, 잘못된 토큰), 에러 전파 |
| **Trackable** | REQ-FILTER-XXX 태그로 요구사항-코드 추적 가능 |
