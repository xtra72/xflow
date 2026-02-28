---
id: SPEC-EXPR-001
title: "Transform Expression Engine Enhancement - Acceptance Criteria"
version: "1.0.0"
status: completed
created: "2026-02-28"
---

# SPEC-EXPR-001: 인수 조건

## AC-1: 렉서 — 토큰화 정확성

**Given** 유효한 expression 문자열이 주어졌을 때
**When** 렉서가 토큰화를 수행하면
**Then** 올바른 토큰 타입과 값의 시퀀스를 반환해야 한다

### 검증 케이스

| # | 입력 | 예상 토큰 시퀀스 |
|---|------|----------------|
| 1 | `$.payload.temp` | `[Path("$.payload.temp"), EOF]` |
| 2 | `$address_table` | `[Var("address_table"), EOF]` |
| 3 | `42` | `[Number(42), EOF]` |
| 4 | `-3.14` | `[Minus, Number(3.14), EOF]` 또는 `[Number(-3.14), EOF]` (컨텍스트 의존) |
| 5 | `"hello"` | `[String("hello"), EOF]` |
| 6 | `true` | `[Bool(true), EOF]` |
| 7 | `null` | `[Null, EOF]` |
| 8 | `+ - * / %` | `[Plus, Minus, Star, Slash, Percent, EOF]` |
| 9 | `&` | `[Ampersand, EOF]` |
| 10 | `{ } [ ] ( ) : ,` | `[LBrace, RBrace, LBracket, RBracket, LParen, RParen, Colon, Comma, EOF]` |
| 11 | `now` | `[Ident("now"), EOF]` |
| 12 | 공백/탭/줄바꿈 포함 | 공백 무시됨 |

## AC-2: 렉서 — 에러 처리

**Given** 유효하지 않은 문자가 포함된 expression 문자열이 주어졌을 때
**When** 렉서가 토큰화를 수행하면
**Then** 에러 위치를 포함한 `ErrInvalidExpression`을 반환해야 한다

### 검증 케이스

| # | 입력 | 예상 에러 |
|---|------|----------|
| 1 | `$.payload.temp @` | `ErrInvalidExpression` (위치 정보 포함) |
| 2 | `"unterminated` | `ErrInvalidExpression` (미종료 문자열) |

## AC-3: 파서 — 기본 필드 매핑 (하위 호환)

**Given** `{ key: $.path }` 형식의 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** `ObjectNode` 내에 `PathNode`를 값으로 가진 필드가 생성되어야 한다

### 검증 케이스

| # | 입력 | 예상 AST |
|---|------|---------|
| 1 | `{ temp: $.payload.temperature }` | `ObjectNode{[{Key:"temp", Value:PathNode("$.payload.temperature")}]}` |
| 2 | `{ a: $.x, b: $.y }` | 2개 필드를 가진 `ObjectNode` |

## AC-4: 파서 — 산술 연산

**Given** 산술 연산을 포함하는 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** 올바른 우선순위의 `BinaryNode` 트리가 생성되어야 한다

### 검증 케이스

| # | 입력 | 예상 동작 |
|---|------|----------|
| 1 | `{ addr: $._base + 2 }` | `BinaryNode(PathNode, "+", LiteralNode(2))` |
| 2 | `{ f: $.temp * 1.8 + 32 }` | `+`가 `*`보다 낮은 우선순위 |
| 3 | `{ x: ($.a + $.b) * 2 }` | 괄호가 우선순위 오버라이드 |
| 4 | `{ r: $.a % 3 }` | 나머지 연산 |

## AC-5: 파서 — 문자열 연결

**Given** `&` 연산자를 포함하는 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** `ConcatNode`가 생성되어야 한다

### 검증 케이스

| # | 입력 | 예상 동작 |
|---|------|----------|
| 1 | `{ tag: $.building & ":" & $.floor }` | 3개 피연산자의 연결 |
| 2 | `{ mixed: $.name & " #" & $.id }` | 경로 + 문자열 리터럴 연결 |

## AC-6: 파서 — 변수 참조 및 테이블 룩업

**Given** `$var` 또는 `$var[key]` 형식의 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** `VarNode` 또는 `IndexNode`가 생성되어야 한다

### 검증 케이스

| # | 입력 | 예상 AST |
|---|------|---------|
| 1 | `{ base: $address_table }` | `VarNode("address_table")` |
| 2 | `{ base: $address_table["A:1F:L1"] }` | `IndexNode(VarNode, LiteralNode("A:1F:L1"))` |
| 3 | `{ base: $address_table[$.tags.building & ":" & $.tags.floor] }` | 동적 키를 가진 `IndexNode` |

## AC-7: 파서 — 함수 호출

**Given** 함수 호출을 포함하는 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** `CallNode`가 생성되어야 한다

### 검증 케이스

| # | 입력 | 예상 AST |
|---|------|---------|
| 1 | `{ ts: now() }` | `CallNode("now", [])` |
| 2 | `{ r: round($.payload.value) }` | `CallNode("round", [PathNode])` |
| 3 | `{ m: max($.a, $.b) }` | `CallNode("max", [PathNode, PathNode])` |
| 4 | `{ s: upper($.name) }` | `CallNode("upper", [PathNode])` |

## AC-8: 파서 — 중첩 오브젝트

**Given** 중첩 오브젝트를 포함하는 expression이 주어졌을 때
**When** 파서가 AST를 생성하면
**Then** 중첩된 `ObjectNode`가 생성되어야 한다

### 검증 케이스

```yaml
expression: |
  {
    command: "set_input",
    params: {
      area: "input_registers",
      address: $._base + 0,
      value: $.payload.object.temperature,
      data_type: "float32"
    }
  }
```

**예상**: 외부 ObjectNode 내에 `params` 키의 값이 내부 ObjectNode

## AC-9: 평가기 — 산술 연산 평가

**Given** 산술 연산을 포함하는 컴파일된 expression과 메시지가 주어졌을 때
**When** 표현식을 평가하면
**Then** 올바른 산술 결과를 반환해야 한다

### 검증 케이스

| # | Expression 값 | 메시지 데이터 | 예상 결과 |
|---|--------------|-------------|----------|
| 1 | `$._base + 2` | `{_base: 8}` | `10` (float64) |
| 2 | `$.temp * 1.8 + 32` | `{temp: 25.0}` | `77.0` |
| 3 | `$.a / $.b` | `{a: 10, b: 3}` | `3.333...` |
| 4 | `$.x % 3` | `{x: 7}` | `1` |
| 5 | `$.a / 0` | `{a: 10}` | `ErrDivisionByZero` |
| 6 | `$.name + 1` | `{name: "abc"}` | `ErrTypeMismatch` |

## AC-10: 평가기 — 문자열 연결 평가

**Given** `&` 연산을 포함하는 컴파일된 expression과 메시지가 주어졌을 때
**When** 표현식을 평가하면
**Then** 연결된 문자열을 반환해야 한다

### 검증 케이스

| # | Expression 값 | 메시지 데이터 | 예상 결과 |
|---|--------------|-------------|----------|
| 1 | `$.building & ":" & $.floor` | `{building:"A", floor:"1F"}` | `"A:1F"` |
| 2 | `$.name & null` | `{name:"test"}` | `"test"` |
| 3 | `$.count & " items"` | `{count: 42}` | `"42 items"` |

## AC-11: 평가기 — 변수 및 테이블 룩업 평가

**Given** 변수 참조를 포함하는 컴파일된 expression, 변수 바인딩, 메시지가 주어졌을 때
**When** 표현식을 평가하면
**Then** 변수에서 올바른 값을 조회해야 한다

### 검증 케이스

| # | Expression | 변수 바인딩 | 메시지 | 예상 결과 |
|---|-----------|-----------|--------|----------|
| 1 | `$address_table["A:1F:L1"]` | `{"A:1F:L1": 0}` | - | `0` |
| 2 | `$address_table[$.tag]` | `{"A:1F:L1": 0}` | `{tag: "A:1F:L1"}` | `0` |
| 3 | `$undefined_var` | `{}` | - | `ErrUndefinedVariable` |
| 4 | `$table["missing_key"]` | `{"a": 1}` | - | `nil` |

## AC-12: 평가기 — 내장 함수 평가

**Given** 내장 함수 호출을 포함하는 컴파일된 expression이 주어졌을 때
**When** 표현식을 평가하면
**Then** 함수가 올바른 결과를 반환해야 한다

### 검증 케이스

| # | Expression | 입력 | 예상 결과 |
|---|-----------|------|----------|
| 1 | `now()` | - | RFC3339Nano 형식 현재 시간 문자열 |
| 2 | `round(3.7)` | - | `4` |
| 3 | `floor(3.7)` | - | `3` |
| 4 | `ceil(3.2)` | - | `4` |
| 5 | `abs(-5)` | - | `5` |
| 6 | `min(3, 7)` | - | `3` |
| 7 | `max(3, 7)` | - | `7` |
| 8 | `upper("hello")` | - | `"HELLO"` |
| 9 | `lower("HELLO")` | - | `"hello"` |
| 10 | `trim("  hi  ")` | - | `"hi"` |
| 11 | `len("hello")` | - | `5` |
| 12 | `int(3.7)` | - | `3` |
| 13 | `float(42)` | - | `42.0` |
| 14 | `string(42)` | - | `"42"` |
| 15 | `unknown_func()` | - | `ErrUndefinedFunction` |
| 16 | `round(1, 2)` | - | `ErrArgumentCount` |

## AC-13: TransformNode 통합 — 하위 호환성

**Given** 기존 형식의 expression이 설정된 TransformNode가 있을 때
**When** Configure 후 Process를 호출하면
**Then** 기존과 동일한 결과를 반환해야 한다

### 검증 케이스

| # | 모드 | Expression | 예상 동작 |
|---|------|-----------|----------|
| 1 | select | `{ temp: $.payload.temperature }` | 기존 동작 유지 |
| 2 | merge | `{ extra: $.payload.value }` | 원본 유지 + 필드 추가 |
| 3 | exclude | `firmware, raw_adc` | 지정 필드 제거 |
| 4 | pipeline | `[{exclude: "x"}, {merge: "{ y: $.z }"}]` | 순차 파이프라인 |
| 5 | transform | `TransformFunc` Go 함수 | Go 함수 우선 적용 |

## AC-14: TransformNode 통합 — 변수 바인딩

**Given** config에 `expression`과 추가 키가 있는 TransformNode가 있을 때
**When** Configure가 호출되면
**Then** 예약 키를 제외한 나머지가 변수 바인딩으로 수집되어야 한다

### 검증 케이스

```yaml
config:
  address_table:
    "A:1F:L1": 0
    "A:1F:L2": 8
  expression: |
    { base: $address_table[$.payload.tag] }
```

**예상**: `address_table` 맵이 변수로 바인딩되고, expression에서 `$address_table`로 접근 가능

## AC-15: TransformNode 통합 — mqtt-to-modbus 시나리오

**Given** mqtt-to-modbus.yaml의 address-resolver 설정이 주어졌을 때
**When** MQTT 메시지를 처리하면
**Then** 태그 조합으로 address_table을 룩업하여 _base 필드를 추가해야 한다

### 검증 메시지

```json
{
  "payload": {
    "deviceInfo": {
      "devEui": "a1b2c3d4e5f60001",
      "tags": { "location": "창고", "point": "서버 옆" }
    },
    "object": { "temperature": 23.5 }
  }
}
```

**예상 결과**:
- `payload` 필드 유지 (원본 그대로)
- `_base` = `0` (`"창고:서버 옆"` → address_table 룩업 → `0`)

## AC-16: 동시성 안전

**Given** 컴파일된 expression이 설정된 TransformNode가 있을 때
**When** 다중 고루틴에서 동시에 Process를 호출하면
**Then** `go test -race`에서 데이터 레이스가 발생하지 않아야 한다

## AC-17: 전체 테스트 통과

**Given** 모든 구현이 완료되었을 때
**When** `go test -race ./internal/node/...`를 실행하면
**Then** 모든 테스트가 통과하고, 커버리지 85% 이상이어야 한다

## AC-18: 기존 테스트 회귀 없음

**Given** 모든 구현이 완료되었을 때
**When** `go test -race ./...`를 실행하면
**Then** 프로젝트 전체 테스트에서 회귀가 없어야 한다
