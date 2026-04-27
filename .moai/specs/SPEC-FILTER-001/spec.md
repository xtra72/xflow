---
id: SPEC-FILTER-001
title: "Filter Node Condition Expression Parser"
version: "1.1.0"
status: completed
created: "2026-02-23"
updated: "2026-02-23"
author: "xtra"
priority: high
related_specs:
  - SPEC-AGG-001
tags:
  - filter
  - expression
  - parser
  - condition
  - node
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-23 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-02-23 | xtra | 구현 완료 - Implementation Notes 추가 |

# SPEC-FILTER-001: Filter Node Condition Expression Parser

## 1. 개요

### 1.1 목적

FilterNode에 문자열 기반 조건식 파서를 추가하여, YAML 플로우 정의에서 직접 조건식을 작성할 수 있도록 한다. 현재 FilterNode는 Go 함수 타입(`FilterCondition func(msg message.Message) bool`)만 지원하므로, YAML에서 `condition: "$.payload.temperature >= 30"` 같은 문자열 조건식을 사용할 수 없다.

### 1.2 배경

- xflow는 IoT FBP(Flow Based Programming) 플랫폼으로, YAML로 플로우를 정의한다.
- TransformNode는 이미 문자열 expression을 파싱하여 Go 함수로 컴파일하는 패턴이 구현되어 있다 (`expression.go`).
- FilterNode도 동일한 패턴으로, YAML 설정에서 문자열 조건식을 지원해야 한다.
- 현재 `filter.go`의 `Configure` 메서드는 `FilterCondition` Go 함수만 인식하고, 문자열 조건식은 무시한다.

### 1.3 범위

**포함:**
- 조건식 렉서(Tokenizer) 구현
- 재귀 하강 파서(Recursive Descent Parser) 및 AST 생성
- AST 평가기(Evaluator) 구현
- FilterNode.Configure 확장 (문자열 조건식 지원)
- 에러 정의 추가

**제외:**
- 사용자 정의 함수 등록 (향후 확장 가능)
- 정규식 매칭 연산자 (향후 확장 가능)
- 집계/윈도우 관련 로직 (SPEC-AGG-001 영역)

## 2. 환경

| 항목 | 상세 |
|------|------|
| 런타임 | Go 1.23+ |
| 대상 모듈 | `internal/node/` |
| 의존성 | `pkg/message` (Path, Payload), `internal/node` (BaseNode, FilterCondition) |
| 테스트 프레임워크 | `testing` + `testify/assert`, `testify/require` |
| 동시성 | `sync.RWMutex` (기존 FilterNode 패턴 유지) |
| 기존 참조 | `expression.go` (TransformNode 파서 패턴), `path.go` (JSONPath 평가) |

## 3. 가정

- **A1**: 조건식은 YAML `condition` 키를 통해 `map[string]any`의 `string` 타입으로 전달된다.
- **A2**: JSONPath 경로(`$.payload.xxx`)는 기존 `evaluatePath` 함수를 재사용한다.
- **A3**: 메시지-맵 변환은 기존 `messageToMap` 함수를 재사용한다.
- **A4**: Go 함수 타입(`FilterCondition`)이 설정된 경우 문자열 조건식보다 우선한다 (TransformNode Configure 패턴과 동일).
- **A5**: 조건식 파싱은 Configure 시점에 1회 수행하며, 파싱 결과(AST)를 캐싱하여 Process 시 재사용한다.
- **A6**: 연산자 우선순위는 일반적인 프로그래밍 언어 규칙을 따른다 (`!` > 비교 > `&&` > `||`).

## 4. 요구사항

### 4.1 조건식 파싱 (Lexer/Tokenizer)

**REQ-FILTER-001**: 렉서 - 기본 토큰 인식
시스템은 **항상** 조건식 문자열을 토큰 시퀀스로 분해해야 한다. 지원 토큰 타입:
- JSONPath 토큰: `$.payload.temperature`, `$.object.humidity`
- 비교 연산자: `>=`, `<=`, `==`, `!=`, `>`, `<`
- 논리 연산자: `&&`, `||`
- 괄호: `(`, `)`
- 부정 연산자: `!`
- 숫자 리터럴: `42`, `-40.5`, `3.14`
- 문자열 리터럴: `'active'`, `"active"` (작은따옴표, 큰따옴표 모두 지원)
- 불리언 리터럴: `true`, `false`
- Null 리터럴: `null`
- 함수 호출: `exists($.path)`

**REQ-FILTER-002**: 렉서 - 공백 처리
시스템은 **항상** 토큰 사이의 공백(스페이스, 탭)을 무시해야 한다.

**REQ-FILTER-003**: 렉서 - 에러 보고
**IF** 인식할 수 없는 문자가 입력되면, **THEN** 시스템은 에러 위치(오프셋)를 포함한 `ErrInvalidExpression` 에러를 반환해야 한다.

### 4.2 조건식 파싱 (Parser/AST)

**REQ-FILTER-010**: 파서 - 비교 표현식
**WHEN** `$.path 연산자 리터럴` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `ComparisonNode{Left, Op, Right}` AST 노드를 생성해야 한다.

**REQ-FILTER-011**: 파서 - 논리 AND 표현식
**WHEN** `표현식1 && 표현식2` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `LogicalNode{Left: expr1, Op: AND, Right: expr2}` AST 노드를 생성해야 한다.

**REQ-FILTER-012**: 파서 - 논리 OR 표현식
**WHEN** `표현식1 || 표현식2` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `LogicalNode{Left: expr1, Op: OR, Right: expr2}` AST 노드를 생성해야 한다.

**REQ-FILTER-013**: 파서 - 괄호 그룹
**WHEN** `(표현식)` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 괄호 내부 표현식을 우선 파싱하여 AST 노드를 생성해야 한다.

**REQ-FILTER-014**: 파서 - 부정 표현식
**WHEN** `!표현식` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `NotNode{Expr}` AST 노드를 생성해야 한다.

**REQ-FILTER-015**: 파서 - exists 함수
**WHEN** `exists($.path)` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `ExistsNode{Path}` AST 노드를 생성해야 한다.

**REQ-FILTER-016**: 파서 - 연산자 우선순위
시스템은 **항상** 다음 우선순위에 따라 표현식을 파싱해야 한다 (높은 순):
1. `!` (부정)
2. `>=`, `<=`, `==`, `!=`, `>`, `<` (비교)
3. `&&` (논리 AND)
4. `||` (논리 OR)

**REQ-FILTER-017**: 파서 - 에러 보고
**IF** 문법적으로 유효하지 않은 토큰 시퀀스가 입력되면, **THEN** 시스템은 예상 토큰과 실제 토큰 정보를 포함한 `ErrInvalidExpression` 에러를 반환해야 한다.

### 4.3 조건식 평가 (Evaluator)

**REQ-FILTER-020**: 평가기 - 비교 연산
**WHEN** `ComparisonNode`를 평가할 때, **THEN** 시스템은 왼쪽 피연산자의 JSONPath 값을 추출하고 오른쪽 리터럴과 비교 연산을 수행하여 `bool` 결과를 반환해야 한다.

**REQ-FILTER-021**: 평가기 - 숫자 비교
**WHEN** 비교 대상이 숫자 타입일 때, **THEN** 시스템은 `float64`로 변환하여 산술 비교를 수행해야 한다. 정수 타입(`int`, `int64` 등)도 `float64`로 자동 변환한다.

**REQ-FILTER-022**: 평가기 - 문자열 비교
**WHEN** 비교 대상이 문자열 타입일 때, **THEN** 시스템은 `==`와 `!=` 연산만 허용하고, `>`, `<`, `>=`, `<=` 연산에 대해서는 `false`를 반환해야 한다.

**REQ-FILTER-023**: 평가기 - null 비교
**WHEN** 비교 대상 리터럴이 `null`일 때, **THEN** 시스템은 `==`는 필드가 nil인 경우 true, `!=`는 필드가 nil이 아닌 경우 true를 반환해야 한다.

**REQ-FILTER-024**: 평가기 - 논리 AND/OR
**WHEN** `LogicalNode`를 평가할 때, **THEN** 시스템은 단락 평가(short-circuit evaluation)를 적용해야 한다:
- `&&`: 왼쪽이 false이면 오른쪽을 평가하지 않고 false 반환
- `||`: 왼쪽이 true이면 오른쪽을 평가하지 않고 true 반환

**REQ-FILTER-025**: 평가기 - 부정
**WHEN** `NotNode`를 평가할 때, **THEN** 시스템은 내부 표현식의 평가 결과를 반전하여 반환해야 한다.

**REQ-FILTER-026**: 평가기 - exists 함수
**WHEN** `ExistsNode`를 평가할 때, **THEN** 시스템은 `evaluatePath`를 호출하여:
- 경로가 존재하면 (에러 없음) `true` 반환
- `ErrPathNotFound` 에러 시 `false` 반환
- 기타 에러 시 `false` 반환

**REQ-FILTER-027**: 평가기 - 경로 미존재 처리
**IF** 비교 연산에서 JSONPath 경로가 존재하지 않으면, **THEN** 시스템은 해당 비교를 `false`로 평가해야 한다 (null 비교 제외).

**REQ-FILTER-028**: 평가기 - 불리언 리터럴 비교
**WHEN** 비교 대상이 불리언 타입일 때, **THEN** 시스템은 `==`와 `!=` 연산만 허용하고, 다른 비교 연산에 대해서는 `false`를 반환해야 한다.

### 4.4 FilterNode Configure 확장

**REQ-FILTER-030**: Configure - Go 함수 우선
**IF** `config["condition"]`이 `FilterCondition` Go 함수 타입이면, **THEN** 시스템은 해당 함수를 직접 설정하고 문자열 파싱을 건너뛰어야 한다.

**REQ-FILTER-031**: Configure - 문자열 조건식 파싱
**WHEN** `config["condition"]`이 `string` 타입이면, **THEN** 시스템은 조건식을 파싱하고, 컴파일된 `FilterCondition`을 설정해야 한다.

**REQ-FILTER-032**: Configure - 파싱 에러 전파
**IF** 문자열 조건식 파싱에 실패하면, **THEN** 시스템은 `ErrInvalidExpression` 에러를 반환해야 한다.

**REQ-FILTER-033**: Configure - 빈 문자열 처리
**IF** `config["condition"]`이 빈 문자열(`""`)이면, **THEN** 시스템은 `ErrInvalidExpression` 에러를 반환해야 한다.

**REQ-FILTER-034**: Configure - condition 미설정
**IF** `config["condition"]` 키가 존재하지 않으면, **THEN** 시스템은 에러 없이 반환하고, `condition`은 nil 상태를 유지해야 한다 (Process에서 pass-through 동작).

### 4.5 하위 호환성

**REQ-FILTER-040**: 기존 FilterCondition 호환
시스템은 **항상** 기존 `FilterCondition` Go 함수 타입 설정이 변경 없이 동작해야 한다.

**REQ-FILTER-041**: Process 동작 보존
시스템은 **항상** 기존 Process 메서드의 동작을 보존해야 한다:
- condition == nil: 메시지 통과 (pass-through)
- condition(msg) == true: 메시지 통과 (output 포트 전송)
- condition(msg) == false: `ErrFilterRejected` 반환

### 4.6 동시성 안전

**REQ-FILTER-050**: 동시 Process 안전
**WHILE** 다중 고루틴에서 동시에 Process를 호출하는 동안, 시스템은 **항상** 데이터 레이스 없이 조건식을 평가해야 한다.

**REQ-FILTER-051**: Configure/Process 동시 안전
**WHILE** Configure와 Process가 동시에 호출되는 동안, 시스템은 **항상** `sync.RWMutex`를 사용하여 안전하게 condition을 읽고 쓸 수 있어야 한다.

## 5. 명세

### 5.1 AST 노드 타입

```
ConditionNode (인터페이스)
├── ComparisonNode   - 비교 표현식: {Left: PathExpr, Op: CompareOp, Right: LiteralExpr}
├── LogicalNode      - 논리 표현식: {Left: ConditionNode, Op: LogicalOp, Right: ConditionNode}
├── NotNode          - 부정 표현식: {Expr: ConditionNode}
├── ExistsNode       - 존재 확인: {Path: string}
├── PathExpr         - JSONPath 경로: {Path: string}
└── LiteralExpr      - 리터럴 값: {Value: any, Type: LiteralType}
    ├── NumericLiteral  (float64)
    ├── StringLiteral   (string)
    ├── BoolLiteral     (bool)
    └── NullLiteral     (nil)
```

### 5.2 토큰 타입

```
TokenType:
  TOKEN_PATH       - JSONPath ($.xxx.yyy)
  TOKEN_NUMBER     - 숫자 리터럴
  TOKEN_STRING     - 문자열 리터럴
  TOKEN_BOOL       - 불리언 리터럴
  TOKEN_NULL       - null 리터럴
  TOKEN_COMPARE_OP - 비교 연산자 (>=, <=, ==, !=, >, <)
  TOKEN_LOGICAL_OP - 논리 연산자 (&&, ||)
  TOKEN_NOT        - 부정 연산자 (!)
  TOKEN_LPAREN     - 왼쪽 괄호
  TOKEN_RPAREN     - 오른쪽 괄호
  TOKEN_FUNC       - 함수 이름 (exists)
  TOKEN_EOF        - 입력 끝
```

### 5.3 파서 문법 (EBNF)

```
expression   = or_expr
or_expr      = and_expr { "||" and_expr }
and_expr     = unary_expr { "&&" unary_expr }
unary_expr   = "!" unary_expr | primary_expr
primary_expr = comparison | exists_call | "(" expression ")"
comparison   = path compare_op literal
exists_call  = "exists" "(" path ")"
path         = "$" "." identifier { "." identifier | "[" index "]" }
literal      = number | string | bool | null
compare_op   = ">=" | "<=" | "==" | "!=" | ">" | "<"
```

### 5.4 컴파일 함수 시그니처

```
compileCondition(expr string) (FilterCondition, error)
  입력: 조건식 문자열
  출력: FilterCondition 함수 또는 에러

  내부 단계:
    1. tokenize(expr) -> []Token, error
    2. parse(tokens) -> ConditionNode, error
    3. ConditionNode를 클로저로 래핑 -> FilterCondition
```

### 5.5 파일 구조

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `internal/node/condition.go` | 렉서, 파서, 평가기, 컴파일러 | 신규 |
| `internal/node/condition_test.go` | 조건식 파서 단위 테스트 | 신규 |
| `internal/node/filter.go` | Configure 확장 | 수정 |
| `internal/node/filter_test.go` | FilterNode 통합 테스트 확장 | 수정 |
| `internal/node/errors.go` | 에러 정의 추가 (필요 시) | 수정 |

### 5.6 조건식 예시

| 조건식 | 설명 |
|--------|------|
| `$.payload.temperature >= 30` | 온도 30도 이상 |
| `$.payload.temperature >= -40 && $.payload.temperature <= 150` | 온도 범위 검증 |
| `$.payload.status == 'active'` | 상태가 'active' |
| `exists($.payload.error)` | error 필드 존재 여부 |
| `!exists($.payload.error) && $.payload.value > 0` | error 없고 값이 양수 |
| `($.payload.type == 'sensor' \|\| $.payload.type == 'actuator') && $.payload.value != null` | 복합 조건 |
| `$.payload.enabled == true` | 불리언 비교 |
| `$.payload.count != null` | null이 아닌지 확인 |

## 6. 추적성

| 요구사항 ID | 구현 파일 | 테스트 |
|-------------|-----------|--------|
| REQ-FILTER-001 ~ 003 | condition.go (tokenize) | condition_test.go |
| REQ-FILTER-010 ~ 017 | condition.go (parse) | condition_test.go |
| REQ-FILTER-020 ~ 028 | condition.go (evaluate) | condition_test.go |
| REQ-FILTER-030 ~ 034 | filter.go (Configure) | filter_test.go |
| REQ-FILTER-040 ~ 041 | filter.go (Process) | filter_test.go |
| REQ-FILTER-050 ~ 051 | filter.go (mutex) | filter_test.go (-race) |

## 7. 구현 노트 (Implementation Notes)

### 7.1 구현 요약

- **구현 커밋**: `efa8ba4` (feat(node): 필터 노드 조건식 파서 구현)
- **구현 일자**: 2026-02-23
- **개발 방법론**: Hybrid (TDD for condition.go, DDD for filter.go)

### 7.2 구현된 파일

| 파일 | 변경 유형 | 라인 수 | 설명 |
|------|-----------|---------|------|
| `internal/node/condition.go` | 신규 | 630 | 렉서, 재귀 하강 파서, AST 평가기, compileCondition |
| `internal/node/condition_test.go` | 신규 | 935 | 83+ 부테스트 (토큰화, 파싱, 평가, 통합) |
| `internal/node/filter.go` | 수정 | +37/-6 | Configure 확장 (Go 함수 1순위, 문자열 2순위) |
| `internal/node/filter_test.go` | 수정 | +158 | 8개 통합 테스트 추가 |

### 7.3 품질 검증 결과

| 항목 | 결과 |
|------|------|
| 테스트 | 전체 PASS (332개 테스트, 128개 부테스트) |
| 커버리지 | 92.5% (목표: 85%) |
| Race Detector | 클린 |
| go vet | 클린 |
| 회귀 | 0건 |

### 7.4 SPEC 대비 차이점

- `errors.go` 수정은 불필요했음 (기존 `ErrInvalidExpression` 재사용)
- 그 외 계획된 모든 요구사항(REQ-FILTER-001 ~ REQ-FILTER-051)이 구현됨
