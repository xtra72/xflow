---
id: SPEC-EXPR-001
title: "Transform Expression Engine Enhancement"
version: "1.2.0"
status: completed
created: "2026-02-28"
updated: "2026-05-21"
author: "xtra"
priority: high
related_specs:
  - SPEC-FILTER-001
  - SPEC-SCRIPT-001
  - SPEC-NODE-001
tags:
  - expression
  - transform
  - parser
  - arithmetic
  - function
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-28 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-02-28 | xtra | 구현 완료 — status: completed |
| 1.2.0 | 2026-05-21 | xtra | **transform 노드의 필드 평가 결과가 nil 인 경우 결과 객체에서 생략 (v0.7.11)**. 이전: `evalObject` 가 nil 값을 그대로 `result[key] = nil` 로 set → JSON 직렬화 시 `{"field": null}` 출력. 부재 필드를 참조하는 변환식이 downstream payload 를 null 로 오염. 변경: nil 값은 result 에 추가하지 않음 — 부재 필드는 부재 그대로 (omit). `TestTransformNode_Configure_StripNulls_false_nil값유지` → `TestTransformNode_MissingPath_OmittedByDefault` 로 재작성. `internal/node/expr_eval.go` 의 `evalObject` 함수 수정. |

# SPEC-EXPR-001: Transform Expression Engine Enhancement

## 1. 개요

### 1.1 목적

TransformNode의 expression 파서를 확장하여, 산술 연산, 문자열 연결, 변수 참조/테이블 룩업, 내장 함수를 지원한다. 현재 expression 파서는 단순 `{ key: $.path }` 형식의 필드 매핑만 지원하므로, `$._base + 2`나 `$address_table[key]` 같은 고급 표현식을 사용할 수 없다.

### 1.2 배경

- xflow는 IoT FBP 플랫폼으로, YAML로 플로우를 정의한다.
- `mqtt-to-modbus.yaml` 예제에서 이미 산술 연산(`$._base + 0`), 문자열 연결(`&`), 테이블 룩업(`$address_table[...]`), 내장 함수(`now()`)를 사용하고 있으나, 실제 expression 파서(`expression.go`)는 이를 지원하지 않는다.
- 현재 파서는 정규식 기반 문자열 분할로 구현되어 있어, 연산자 파싱이 불가능하다.
- FilterNode의 조건식 파서(SPEC-FILTER-001)는 이미 렉서 + 재귀 하강 파서 + AST 패턴을 사용하고 있어, 동일한 아키텍처를 expression 파서에 적용할 수 있다.

### 1.3 범위

**포함:**
- Expression 렉서(Tokenizer) — 산술/문자열 연산자, 변수 참조, 함수 호출 토큰 지원
- 재귀 하강 파서(Recursive Descent Parser) — 값 표현식 AST 생성
- AST 평가기(Evaluator) — 메시지 컨텍스트에서 표현식 평가
- 중첩 오브젝트 리터럴 파싱 — `{ key: expr, nested: { a: expr } }` 지원
- 변수 참조 및 config 바인딩 — `$var_name`, `$table[key]` 구문
- 내장 함수 라이브러리 — `now()`, `round()`, `floor()`, `ceil()`, `len()`, `upper()`, `lower()`, `trim()`
- TransformNode.Configure 확장 — 신규 파서 통합
- 기존 동작 하위 호환성 보장

**제외:**
- FilterNode 조건식 확장 (SPEC-FILTER-001 영역)
- ScriptNode Lua 확장 (SPEC-SCRIPT-001 영역)
- 사용자 정의 함수 등록 (향후 확장 가능)
- 타입 시스템 도입 (향후 확장 가능)

## 2. 환경

| 항목 | 상세 |
|------|------|
| 런타임 | Go 1.23+ |
| 대상 모듈 | `internal/node/` |
| 의존성 | `pkg/message` (Path, Payload), `internal/node` (BaseNode, TransformFunc) |
| 테스트 프레임워크 | `testing` + `testify/assert`, `testify/require` |
| 동시성 | `sync.RWMutex` (기존 TransformNode 패턴 유지) |
| 기존 참조 | `condition.go` (렉서/파서 패턴), `expression.go` (기존 파서), `path.go` (JSONPath) |

## 3. 가정

- **A1**: Expression은 YAML `expression` 키를 통해 `map[string]any`의 `string` 타입으로 전달된다.
- **A2**: JSONPath 경로(`$.payload.xxx`)는 기존 `GetPath` 함수를 재사용한다.
- **A3**: 메시지-맵 변환은 기존 `messageToMap` 함수를 재사용한다.
- **A4**: Config 변수(`$address_table` 등)는 TransformNode의 `config` 맵에서 바인딩된다.
- **A5**: Expression 파싱은 Configure 시점에 1회 수행하며, 파싱 결과(AST)를 캐싱하여 Process 시 재사용한다.
- **A6**: 산술 연산자 우선순위는 표준 수학 규칙을 따른다 (`*`, `/` > `+`, `-`).
- **A7**: 기존 `{ key: $.path }` 문법은 100% 하위 호환성을 유지한다.
- **A8**: 내장 함수는 부작용(side-effect)이 없는 순수 함수만 포함한다(`now()` 제외).

## 4. 요구사항

### 4.1 렉서 확장 (Lexer/Tokenizer)

**REQ-EXPR-001**: 렉서 — 기존 토큰 유지
시스템은 **항상** 기존 토큰 타입을 인식해야 한다:
- JSONPath 토큰: `$.payload.temperature`, `$.object.humidity`
- 문자열 리터럴: `"active"`, `'active'`
- 숫자 리터럴: `42`, `-40.5`, `3.14`
- 불리언 리터럴: `true`, `false`
- Null 리터럴: `null`

**REQ-EXPR-002**: 렉서 — 산술 연산자 토큰
시스템은 **항상** 다음 산술 연산자를 토큰으로 인식해야 한다:
- `+` (더하기)
- `-` (빼기, 음수 부호와 구분)
- `*` (곱하기)
- `/` (나누기)
- `%` (나머지)

**REQ-EXPR-003**: 렉서 — 문자열 연결 연산자 토큰
시스템은 **항상** `&` 를 문자열 연결 연산자 토큰으로 인식해야 한다.

**REQ-EXPR-004**: 렉서 — 구조 토큰
시스템은 **항상** 다음 구조 토큰을 인식해야 한다:
- `{` (오브젝트 시작)
- `}` (오브젝트 끝)
- `[` (배열/인덱스 시작)
- `]` (배열/인덱스 끝)
- `(` (함수 인자 시작)
- `)` (함수 인자 끝)
- `:` (키-값 구분)
- `,` (항목 구분)

**REQ-EXPR-005**: 렉서 — 변수 참조 토큰
시스템은 **항상** `$`로 시작하되 `.`이 뒤따르지 않는 식별자를 변수 참조 토큰으로 인식해야 한다:
- `$address_table` — config 변수 참조
- `$_base` — 메시지 내 필드 참조 (`$._base`의 단축)

**REQ-EXPR-006**: 렉서 — 함수 이름 토큰
시스템은 **항상** 알파벳으로 시작하고 `(`가 뒤따르는 식별자를 함수 호출 토큰으로 인식해야 한다:
- `now()`, `round(x)`, `len(arr)` 등

**REQ-EXPR-007**: 렉서 — 식별자 토큰
시스템은 **항상** `$`나 `$.` 접두사 없이 알파벳/밑줄로 시작하는 문자열을 식별자(키 이름) 토큰으로 인식해야 한다:
- `command`, `params`, `area` 등 (오브젝트 리터럴의 키)

**REQ-EXPR-008**: 렉서 — 공백 및 줄바꿈 처리
시스템은 **항상** 토큰 사이의 공백(스페이스, 탭, 줄바꿈)을 무시해야 한다.

**REQ-EXPR-009**: 렉서 — 에러 보고
**IF** 인식할 수 없는 문자가 입력되면, **THEN** 시스템은 에러 위치(행:열)를 포함한 `ErrInvalidExpression` 에러를 반환해야 한다.

### 4.2 파서 (Parser/AST)

**REQ-EXPR-010**: 파서 — 오브젝트 리터럴
**WHEN** `{ key1: expr1, key2: expr2 }` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `ObjectNode{Fields: [{Key, Value}...]}` AST 노드를 생성해야 한다.

**REQ-EXPR-011**: 파서 — 중첩 오브젝트
**WHEN** 오브젝트 값 위치에 다른 오브젝트 리터럴이 나타나면, **THEN** 시스템은 중첩된 `ObjectNode`를 생성해야 한다.

**REQ-EXPR-012**: 파서 — 산술 연산
**WHEN** `expr1 + expr2`, `expr1 - expr2`, `expr1 * expr2`, `expr1 / expr2`, `expr1 % expr2` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `BinaryNode{Left, Op, Right}` AST 노드를 생성해야 한다.

**REQ-EXPR-013**: 파서 — 연산자 우선순위
시스템은 **항상** 다음 우선순위에 따라 표현식을 파싱해야 한다 (높은 순):
1. `()` (함수 호출, 괄호 그룹)
2. `*`, `/`, `%` (곱셈, 나눗셈, 나머지)
3. `+`, `-` (덧셈, 뺄셈)
4. `&` (문자열 연결)

**REQ-EXPR-014**: 파서 — 문자열 연결
**WHEN** `expr1 & expr2` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `ConcatNode{Left, Right}` AST 노드를 생성해야 한다.

**REQ-EXPR-015**: 파서 — JSONPath 경로
**WHEN** `$.path.to.field` 형식의 토큰을 받으면, **THEN** 시스템은 `PathNode{Path}` AST 노드를 생성해야 한다.

**REQ-EXPR-016**: 파서 — 변수 참조
**WHEN** `$variable_name` 형식의 토큰을 받으면, **THEN** 시스템은 `VarNode{Name}` AST 노드를 생성해야 한다.

**REQ-EXPR-017**: 파서 — 테이블 룩업
**WHEN** `$variable[expr]` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `IndexNode{Target: VarNode, Key: expr}` AST 노드를 생성해야 한다.

**REQ-EXPR-018**: 파서 — 함수 호출
**WHEN** `func_name(arg1, arg2, ...)` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 `CallNode{Name, Args: []ExprNode}` AST 노드를 생성해야 한다.

**REQ-EXPR-019**: 파서 — 리터럴 값
시스템은 **항상** 다음 리터럴을 `LiteralNode{Value}` AST 노드로 파싱해야 한다:
- 숫자: `42`, `-3.14`, `0`
- 문자열: `"hello"`, `'world'`
- 불리언: `true`, `false`
- Null: `null`

**REQ-EXPR-020**: 파서 — 괄호 그룹
**WHEN** `(expr)` 형식의 토큰 시퀀스를 받으면, **THEN** 시스템은 내부 표현식을 우선 파싱하여 AST 노드를 생성해야 한다.

**REQ-EXPR-021**: 파서 — 에러 보고
**IF** 문법적으로 유효하지 않은 토큰 시퀀스가 입력되면, **THEN** 시스템은 예상 토큰과 실제 토큰 정보를 포함한 `ErrInvalidExpression` 에러를 반환해야 한다.

### 4.3 평가기 (Evaluator)

**REQ-EXPR-030**: 평가기 — 오브젝트 리터럴 평가
**WHEN** `ObjectNode`를 평가할 때, **THEN** 시스템은 각 필드의 값 표현식을 평가하고, 결과를 `map[string]any`로 반환해야 한다.

**REQ-EXPR-031**: 평가기 — 산술 연산 평가
**WHEN** `BinaryNode`를 평가할 때, **THEN** 시스템은 양쪽 피연산자를 `float64`로 변환하고 산술 연산을 수행해야 한다.
- 정수 타입(`int`, `int64`, `int32` 등)도 `float64`로 자동 변환한다.
- **IF** 피연산자가 숫자로 변환 불가능하면, **THEN** `ErrTypeMismatch` 에러를 반환해야 한다.
- **IF** `/` 또는 `%` 연산에서 우측 피연산자가 0이면, **THEN** `ErrDivisionByZero` 에러를 반환해야 한다.

**REQ-EXPR-032**: 평가기 — 문자열 연결 평가
**WHEN** `ConcatNode`를 평가할 때, **THEN** 시스템은 양쪽 피연산자를 문자열로 변환하고 연결해야 한다.
- 숫자는 `fmt.Sprintf("%v", val)`로 문자열 변환한다.
- **IF** 피연산자가 nil이면, **THEN** 빈 문자열로 처리한다.

**REQ-EXPR-033**: 평가기 — JSONPath 경로 평가
**WHEN** `PathNode`를 평가할 때, **THEN** 시스템은 메시지 맵에서 `GetPath`로 값을 추출해야 한다.
- **IF** 경로가 존재하지 않으면, **THEN** `nil`을 반환해야 한다 (에러 아님).

**REQ-EXPR-034**: 평가기 — 변수 참조 평가
**WHEN** `VarNode`를 평가할 때, **THEN** 시스템은 config 바인딩에서 변수 값을 조회해야 한다.
- **IF** 변수가 바인딩에 존재하지 않으면, **THEN** `ErrUndefinedVariable` 에러를 반환해야 한다.

**REQ-EXPR-035**: 평가기 — 테이블 룩업 평가
**WHEN** `IndexNode`를 평가할 때, **THEN** 시스템은:
1. 대상 표현식을 평가하여 `map[string]any`를 얻는다.
2. 키 표현식을 평가하여 문자열 키를 얻는다.
3. 맵에서 키로 값을 조회한다.
- **IF** 대상이 맵이 아니면, **THEN** `ErrTypeMismatch` 에러를 반환해야 한다.
- **IF** 키가 맵에 존재하지 않으면, **THEN** `nil`을 반환해야 한다.

**REQ-EXPR-036**: 평가기 — 내장 함수 평가
**WHEN** `CallNode`를 평가할 때, **THEN** 시스템은 등록된 내장 함수를 호출해야 한다.
- **IF** 함수가 등록되지 않았으면, **THEN** `ErrUndefinedFunction` 에러를 반환해야 한다.
- **IF** 인자 개수가 함수 시그니처와 불일치하면, **THEN** `ErrArgumentCount` 에러를 반환해야 한다.

**REQ-EXPR-037**: 평가기 — 리터럴 평가
**WHEN** `LiteralNode`를 평가할 때, **THEN** 시스템은 저장된 값을 그대로 반환해야 한다.

### 4.4 내장 함수 (Built-in Functions)

**REQ-EXPR-040**: 시간 함수
시스템은 **항상** 다음 시간 함수를 제공해야 한다:
- `now()`: 현재 시간을 RFC3339Nano 형식 문자열로 반환

**REQ-EXPR-041**: 수학 함수
시스템은 **항상** 다음 수학 함수를 제공해야 한다:
- `round(x)`: 반올림 (정수 반환)
- `floor(x)`: 내림 (정수 반환)
- `ceil(x)`: 올림 (정수 반환)
- `abs(x)`: 절대값 반환
- `min(a, b)`: 두 값 중 작은 값 반환
- `max(a, b)`: 두 값 중 큰 값 반환

**REQ-EXPR-042**: 문자열 함수
시스템은 **항상** 다음 문자열 함수를 제공해야 한다:
- `upper(s)`: 대문자로 변환
- `lower(s)`: 소문자로 변환
- `trim(s)`: 양쪽 공백 제거
- `len(s)`: 문자열 길이 반환 (배열이면 요소 수 반환)

**REQ-EXPR-043**: 타입 변환 함수
시스템은 **항상** 다음 타입 변환 함수를 제공해야 한다:
- `int(x)`: 정수 변환
- `float(x)`: 실수 변환
- `string(x)`: 문자열 변환
- `bool(x)`: 불리언 변환

### 4.5 변수 바인딩 (Variable Binding)

**REQ-EXPR-050**: Config 변수 바인딩
**WHEN** TransformNode의 config에 `expression`과 `address_table` 같은 추가 키가 존재하면, **THEN** 시스템은 `expression`, `mode` 키를 제외한 모든 config 키-값을 변수 바인딩으로 등록해야 한다.

**REQ-EXPR-051**: 예약 키 목록
시스템은 **항상** 다음 키를 예약 키로 취급하고 변수 바인딩에서 제외해야 한다:
- `expression`: 표현식 문자열
- `mode`: 변환 모드
- `transform`: Go 함수 직접 설정

**REQ-EXPR-052**: 메시지 컨텍스트와 변수 우선순위
**IF** 메시지 경로(`$.xxx`)와 config 변수(`$xxx`)가 동일한 이름을 가지면, **THEN** `$.xxx` 문법은 메시지에서, `$xxx` 문법은 config 변수에서 조회해야 한다. 두 네임스페이스는 문법으로 명확히 구분된다.

### 4.6 TransformNode Configure 확장

**REQ-EXPR-060**: Configure — Go 함수 우선
**IF** `config["transform"]`이 `TransformFunc` Go 함수 타입이면, **THEN** 시스템은 해당 함수를 직접 설정하고 expression 파싱을 건너뛰어야 한다.

**REQ-EXPR-061**: Configure — 신규 파서 적용
**WHEN** `config["expression"]`이 `string` 타입이면, **THEN** 시스템은 신규 expression 파서로 파싱하고, 컴파일된 `TransformFunc`를 설정해야 한다.

**REQ-EXPR-062**: Configure — 파이프라인 지원 유지
**WHEN** `config["expression"]`이 `[]any` 타입(파이프라인)이면, **THEN** 시스템은 각 단계를 신규 파서로 파싱하고 체이닝해야 한다.

**REQ-EXPR-063**: Configure — 변수 바인딩 수집
**WHEN** Configure가 호출되면, **THEN** 시스템은 config 맵에서 예약 키를 제외한 나머지를 변수 바인딩으로 수집하고, 파서에 전달해야 한다.

### 4.7 하위 호환성

**REQ-EXPR-070**: 기존 select 모드 호환
시스템은 **항상** 기존 `{ key: $.path }` 형식의 단순 필드 매핑이 변경 없이 동작해야 한다.

**REQ-EXPR-071**: 기존 merge 모드 호환
시스템은 **항상** 기존 `mode: "merge"` 설정이 변경 없이 동작해야 한다.

**REQ-EXPR-072**: 기존 exclude 모드 호환
시스템은 **항상** 기존 `mode: "exclude"` 설정이 변경 없이 동작해야 한다.

**REQ-EXPR-073**: 기존 파이프라인 호환
시스템은 **항상** 기존 `expression: []` 배열 형식의 파이프라인이 변경 없이 동작해야 한다.

**REQ-EXPR-074**: 기존 TransformFunc 호환
시스템은 **항상** 기존 `TransformFunc` Go 함수 타입 설정이 변경 없이 동작해야 한다.

### 4.8 동시성 안전

**REQ-EXPR-080**: 동시 Process 안전
**WHILE** 다중 고루틴에서 동시에 Process를 호출하는 동안, 시스템은 **항상** 데이터 레이스 없이 표현식을 평가해야 한다.

**REQ-EXPR-081**: Configure/Process 동시 안전
**WHILE** Configure와 Process가 동시에 호출되는 동안, 시스템은 **항상** `sync.RWMutex`를 사용하여 안전하게 transformFn을 읽고 쓸 수 있어야 한다.

### 4.9 에러 처리

**REQ-EXPR-090**: 에러 타입 정의
시스템은 **항상** 다음 에러 타입을 정의하고 사용해야 한다:
- `ErrInvalidExpression`: 파싱 에러 (기존 에러 재사용)
- `ErrTypeMismatch`: 타입 불일치 (예: 문자열에 산술 연산)
- `ErrDivisionByZero`: 0으로 나누기
- `ErrUndefinedVariable`: 미정의 변수 참조
- `ErrUndefinedFunction`: 미정의 함수 호출
- `ErrArgumentCount`: 함수 인자 수 불일치

**REQ-EXPR-091**: 에러 위치 정보
**IF** 파싱 에러가 발생하면, **THEN** 시스템은 에러 위치(행:열 또는 오프셋)를 에러 메시지에 포함해야 한다.

## 5. 명세

### 5.1 AST 노드 타입

```
ExprNode (인터페이스: evaluate(ctx *EvalContext) (any, error))
├── ObjectNode       - 오브젝트 리터럴: {Fields: [{Key string, Value ExprNode}...]}
├── BinaryNode       - 산술 연산: {Left ExprNode, Op string, Right ExprNode}
├── ConcatNode       - 문자열 연결: {Left ExprNode, Right ExprNode}
├── PathNode         - JSONPath 경로: {Path string}
├── VarNode          - 변수 참조: {Name string}
├── IndexNode        - 인덱스/룩업: {Target ExprNode, Key ExprNode}
├── CallNode         - 함수 호출: {Name string, Args []ExprNode}
└── LiteralNode      - 리터럴 값: {Value any}
    ├── float64      (숫자)
    ├── string       (문자열)
    ├── bool         (불리언)
    └── nil          (null)
```

### 5.2 토큰 타입

```
TokenType:
  exprTokenPath       - JSONPath ($.xxx.yyy)
  exprTokenVar        - 변수 참조 ($var_name)
  exprTokenNumber     - 숫자 리터럴
  exprTokenString     - 문자열 리터럴
  exprTokenBool       - 불리언 리터럴 (true, false)
  exprTokenNull       - null 리터럴
  exprTokenIdent      - 식별자 (키 이름, 함수 이름)
  exprTokenPlus       - +
  exprTokenMinus      - -
  exprTokenStar       - *
  exprTokenSlash      - /
  exprTokenPercent    - %
  exprTokenAmpersand  - & (문자열 연결)
  exprTokenLBrace     - {
  exprTokenRBrace     - }
  exprTokenLBracket   - [
  exprTokenRBracket   - ]
  exprTokenLParen     - (
  exprTokenRParen     - )
  exprTokenColon      - :
  exprTokenComma      - ,
  exprTokenEOF        - 입력 끝
```

### 5.3 파서 문법 (EBNF)

```
expression     = object_literal
object_literal = "{" field_list "}"
field_list     = field { "," field }
field          = identifier ":" value_expr

value_expr     = concat_expr
concat_expr    = additive_expr { "&" additive_expr }
additive_expr  = multiplicative_expr { ("+" | "-") multiplicative_expr }
multiplicative_expr = unary_expr { ("*" | "/" | "%") unary_expr }
unary_expr     = ["-"] primary_expr

primary_expr   = path_expr
               | var_expr
               | func_call
               | literal
               | object_literal
               | "(" value_expr ")"

path_expr      = "$." identifier { "." identifier | "[" index "]" }
var_expr       = "$" identifier [ "[" value_expr "]" ]
func_call      = identifier "(" [ value_expr { "," value_expr } ] ")"
literal        = number | string | bool | null
identifier     = letter { letter | digit | "_" }
```

### 5.4 평가 컨텍스트

```
EvalContext:
  message     map[string]any    // messageToMap(msg) 결과
  variables   map[string]any    // config에서 수집한 변수 바인딩
  functions   map[string]BuiltinFunc  // 내장 함수 레지스트리

BuiltinFunc:
  func(args ...any) (any, error)
```

### 5.5 컴파일 함수 시그니처

```
compileExpressionV2(expr string, mode TransformMode, vars map[string]any) (TransformFunc, error)
  입력: 표현식 문자열, 변환 모드, 변수 바인딩
  출력: TransformFunc 함수 또는 에러

  내부 단계:
    1. exprTokenize(expr) -> []exprToken, error
    2. exprParse(tokens) -> ExprNode, error (ObjectNode 또는 단일 값)
    3. ExprNode + vars를 클로저로 래핑 -> TransformFunc
       - select 모드: ObjectNode 평가 결과가 새 페이로드
       - merge 모드: 원본 페이로드에 ObjectNode 평가 결과를 덮어쓰기
```

### 5.6 파일 구조

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `internal/node/expr_lexer.go` | Expression 렉서 (토큰화) | 신규 |
| `internal/node/expr_parser.go` | Expression 파서 (AST 생성) | 신규 |
| `internal/node/expr_eval.go` | Expression 평가기 (AST 실행) | 신규 |
| `internal/node/expr_funcs.go` | 내장 함수 레지스트리 | 신규 |
| `internal/node/expr_lexer_test.go` | 렉서 단위 테스트 | 신규 |
| `internal/node/expr_parser_test.go` | 파서 단위 테스트 | 신규 |
| `internal/node/expr_eval_test.go` | 평가기 단위 테스트 | 신규 |
| `internal/node/expr_funcs_test.go` | 내장 함수 테스트 | 신규 |
| `internal/node/transform.go` | Configure 확장 (신규 파서 통합) | 수정 |
| `internal/node/transform_test.go` | TransformNode 통합 테스트 확장 | 수정 |
| `internal/node/errors.go` | 에러 타입 추가 | 수정 |
| `internal/node/expression.go` | 기존 코드 유지 (하위 호환) | 유지 |

### 5.7 Expression 예시

| Expression | 설명 | 평가 결과 |
|-----------|------|-----------|
| `{ temp: $.payload.temperature }` | 기본 필드 매핑 (하위 호환) | `{"temp": 23.5}` |
| `{ address: $._base + 2 }` | 산술 연산 | `{"address": 10}` |
| `{ tag: $.tags.building & ":" & $.tags.floor }` | 문자열 연결 | `{"tag": "A:1F"}` |
| `{ base: $address_table["A:1F:L1"] }` | 변수 테이블 직접 조회 | `{"base": 0}` |
| `{ base: $address_table[$.tags.building & ":" & $.tags.floor & ":" & $.tags.line] }` | 동적 테이블 룩업 | `{"base": 0}` |
| `{ ts: now() }` | 내장 함수 호출 | `{"ts": "2026-02-28T..."}` |
| `{ temp_f: $.payload.temperature * 1.8 + 32 }` | 복합 산술 | `{"temp_f": 74.3}` |
| `{ cmd: "set", params: { area: "input_registers", addr: $._base + 0 } }` | 중첩 오브젝트 | `{"cmd":"set","params":{"area":"input_registers","addr":8}}` |
| `{ name: upper($.payload.name) }` | 문자열 함수 | `{"name": "ALICE"}` |
| `{ rounded: round($.payload.value) }` | 수학 함수 | `{"rounded": 24}` |

## 6. 추적성

| 요구사항 ID | 구현 파일 | 테스트 |
|-------------|-----------|--------|
| REQ-EXPR-001 ~ 009 | expr_lexer.go | expr_lexer_test.go |
| REQ-EXPR-010 ~ 021 | expr_parser.go | expr_parser_test.go |
| REQ-EXPR-030 ~ 037 | expr_eval.go | expr_eval_test.go |
| REQ-EXPR-040 ~ 043 | expr_funcs.go | expr_funcs_test.go |
| REQ-EXPR-050 ~ 052 | expr_eval.go (EvalContext) | expr_eval_test.go |
| REQ-EXPR-060 ~ 063 | transform.go (Configure) | transform_test.go |
| REQ-EXPR-070 ~ 074 | transform.go, expression.go | transform_test.go |
| REQ-EXPR-080 ~ 081 | transform.go (mutex) | transform_test.go (-race) |
| REQ-EXPR-090 ~ 091 | errors.go | 전체 테스트 |

## 7. 구현 노트

### 7.1 구현 결과 요약

| 항목 | 결과 |
|------|------|
| 구현 완료일 | 2026-02-28 |
| 개발 방법론 | Hybrid (TDD: 신규 파일, DDD: 기존 파일 수정) |
| 테스트 커버리지 | 91.8% (목표 85% 초과) |
| 데이터 레이스 | go test -race 통과 (무결) |
| 정적 분석 | go vet 통과 |
| 기존 테스트 회귀 | 없음 (전체 프로젝트 테스트 통과) |

### 7.2 산출물

| 파일 | 라인 수 | 유형 | 설명 |
|------|---------|------|------|
| expr_lexer.go | 192 | 신규 | 21개 토큰 타입, rune 기반 스캐너 |
| expr_parser.go | ~350 | 신규 | 8개 AST 노드, 재귀 하강 파서 |
| expr_eval.go | ~300 | 신규 | EvalContext, AST 평가기, compileExpressionV2 |
| expr_funcs.go | ~290 | 신규 | 15개 내장 함수 (now, round, floor, ceil, abs, min, max, upper, lower, trim, len, int, float, string, bool) |
| expr_lexer_test.go | ~400 | 신규 | 25개 테스트 함수, 72+ 서브테스트 |
| expr_parser_test.go | ~500 | 신규 | 44개 테스트 케이스 |
| expr_eval_test.go | ~300 | 신규 | 21+ 테스트 케이스 |
| expr_funcs_test.go | ~300 | 신규 | 16개 테스트 함수 |
| errors.go | +15 | 수정 | 5개 에러 타입 추가 |
| transform.go | +18 | 수정 | 변수 바인딩 + compileExpressionV2 통합 |
| expression.go | +4 | 수정 | compileExpressionPipeline 시그니처 변경 |
| expression_test.go | +284 | 수정 | 파이프라인 호출 갱신 + 4개 통합 테스트 |

### 7.3 계획 대비 편차

계획(plan.md) 대비 구현 편차 없음. 모든 Phase(1~5) 계획대로 완료.
