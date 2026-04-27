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
| 1.0.0 | 2026-02-23 | xtra | 초기 구현 계획 작성 |
| 1.1.0 | 2026-02-23 | xtra | 구현 완료 - Implementation Notes 추가 |

# SPEC-FILTER-001: 구현 계획

## 1. 구현 전략

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 기준):
- **신규 코드** (TDD): `condition.go`, `condition_test.go`는 RED-GREEN-REFACTOR 사이클
- **기존 코드 수정** (DDD): `filter.go`의 Configure 확장은 ANALYZE-PRESERVE-IMPROVE 사이클

### 1.2 변경 영향 범위

| 파일 | 변경 유형 | 영향도 |
|------|-----------|--------|
| `internal/node/condition.go` | 신규 생성 | 높음 - 렉서, 파서, 평가기, 컴파일러 전체 구현 |
| `internal/node/condition_test.go` | 신규 생성 | 높음 - 단위 테스트 전체 작성 |
| `internal/node/filter.go` | 핵심 수정 | 중간 - Configure 메서드 확장 |
| `internal/node/filter_test.go` | 테스트 확장 | 중간 - 기존 테스트 회귀 확인 + 통합 테스트 추가 |
| `internal/node/errors.go` | 에러 추가 | 낮음 - 필요 시 새로운 에러 상수 추가 |

## 2. 마일스톤

### 마일스톤 1: 기초 작업 및 렉서 구현 (Primary Goal)

기존 동작 보존 확인 및 토큰화 구현

**작업 목록:**

- **T1.1**: 기존 테스트 전체 실행 (`go test -race ./internal/node/...`) 및 베이스라인 확보
  - 모든 기존 테스트가 PASS 상태인지 확인
  - 커버리지 베이스라인 기록

- **T1.2**: `condition.go` 파일 생성 - 토큰 타입 정의
  - `TokenType` 상수 정의 (TOKEN_PATH, TOKEN_NUMBER, TOKEN_STRING 등)
  - `Token` 구조체 정의: `{Type TokenType, Value string, Pos int}`

- **T1.3**: 렉서(Tokenizer) 구현
  - `tokenize(expr string) ([]Token, error)` 함수
  - JSONPath 토큰 인식: `$` 로 시작하는 경로
  - 비교 연산자 인식: 2문자(`>=`, `<=`, `==`, `!=`) 우선, 1문자(`>`, `<`) 후순위
  - 논리 연산자 인식: `&&`, `||`
  - 괄호, 부정 연산자 인식
  - 숫자 리터럴: 음수(`-`), 소수점(`.`) 지원
  - 문자열 리터럴: 작은따옴표, 큰따옴표 지원
  - 키워드: `true`, `false`, `null`, `exists`
  - 공백 건너뛰기
  - REQ-FILTER-001, REQ-FILTER-002, REQ-FILTER-003 충족

- **T1.4**: 렉서 단위 테스트 작성 (`condition_test.go`)
  - 각 토큰 타입별 인식 테스트 (table-driven)
  - 복합 조건식 토큰화 테스트
  - 공백 처리 테스트
  - 에러 케이스 테스트 (인식 불가 문자)

**완료 기준:** 렉서 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 2: 파서 및 AST 구현 (Primary Goal)

재귀 하강 파서 구현 및 AST 노드 생성

**작업 목록:**

- **T2.1**: AST 노드 타입 정의
  - `ConditionNode` 인터페이스: `Evaluate(data map[string]any) (bool, error)` 메서드
  - `ComparisonNode` 구조체: `{Left PathExpr, Op CompareOp, Right LiteralExpr}`
  - `LogicalNode` 구조체: `{Left ConditionNode, Op LogicalOp, Right ConditionNode}`
  - `NotNode` 구조체: `{Expr ConditionNode}`
  - `ExistsNode` 구조체: `{Path string}`
  - `PathExpr` 구조체: `{Path string}`
  - `LiteralExpr` 구조체: `{Value any, Type LiteralType}`
  - REQ-FILTER-010 ~ REQ-FILTER-015 충족

- **T2.2**: 재귀 하강 파서 구현
  - `parse(tokens []Token) (ConditionNode, error)` 함수
  - `parser` 구조체: `{tokens []Token, pos int}`
  - `parseOrExpr()` -> `parseAndExpr()` -> `parseUnaryExpr()` -> `parsePrimaryExpr()` 재귀 구조
  - `parsePrimaryExpr()`에서 comparison, exists, 괄호 분기
  - 연산자 우선순위 자연 반영 (재귀 깊이 = 낮은 우선순위)
  - REQ-FILTER-016, REQ-FILTER-017 충족

- **T2.3**: 파서 단위 테스트 작성
  - 단순 비교식 파싱 테스트
  - AND/OR 논리식 파싱 테스트
  - 괄호 그룹 파싱 테스트
  - 부정 표현식 파싱 테스트
  - exists 함수 파싱 테스트
  - 연산자 우선순위 테스트
  - 파싱 에러 케이스 테스트

**완료 기준:** 파서 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 3: 평가기 구현 (Primary Goal)

AST 노드별 평가 로직 구현

**작업 목록:**

- **T3.1**: `ComparisonNode.Evaluate` 구현
  - `evaluatePath`로 왼쪽 피연산자 값 추출
  - 타입별 비교 로직:
    - 숫자: `float64` 변환 후 산술 비교 (모든 연산자)
    - 문자열: `==`, `!=`만 허용, 나머지는 `false`
    - 불리언: `==`, `!=`만 허용, 나머지는 `false`
    - null: nil 여부 검사
  - 경로 미존재 시 `false` 반환 (null 비교 제외)
  - REQ-FILTER-020 ~ REQ-FILTER-023, REQ-FILTER-027, REQ-FILTER-028 충족

- **T3.2**: `LogicalNode.Evaluate` 구현
  - 단락 평가(short-circuit) 적용
  - `&&`: 왼쪽 false → 즉시 false
  - `||`: 왼쪽 true → 즉시 true
  - REQ-FILTER-024 충족

- **T3.3**: `NotNode.Evaluate` 구현
  - 내부 표현식 결과 반전
  - REQ-FILTER-025 충족

- **T3.4**: `ExistsNode.Evaluate` 구현
  - `evaluatePath` 호출 결과로 존재 여부 판별
  - `ErrPathNotFound` → false, 에러 없음 → true
  - REQ-FILTER-026 충족

- **T3.5**: 평가기 통합 테스트 작성
  - 각 노드 타입별 평가 테스트 (table-driven)
  - 복합 조건식 평가 테스트
  - 타입 변환 경계값 테스트 (int → float64, 등)
  - 경로 미존재 케이스 테스트
  - 단락 평가 동작 검증 테스트
  - null 비교 테스트

**완료 기준:** 평가기 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 4: FilterNode 통합 (Primary Goal)

FilterNode.Configure 확장 및 통합 테스트

**작업 목록:**

- **T4.1**: `compileCondition` 함수 구현
  - `compileCondition(expr string) (FilterCondition, error)` 시그니처
  - tokenize → parse → 클로저 래핑 순서
  - 클로저 내부: `messageToMap(msg)` → `node.Evaluate(data)` 체인
  - REQ-FILTER-031 충족

- **T4.2**: `FilterNode.Configure` 메서드 확장
  - 기존 DDD 패턴: ANALYZE(기존 Configure 분석) → PRESERVE(기존 테스트 확인) → IMPROVE(확장)
  - 분기 순서:
    1. `FilterCondition` Go 함수 타입 → 직접 설정 (기존 동작 보존)
    2. `string` 타입 → `compileCondition` 호출
    3. 빈 문자열 → `ErrInvalidExpression` 반환
    4. 기타 타입 / 키 없음 → 기존 동작 유지
  - REQ-FILTER-030, REQ-FILTER-032, REQ-FILTER-033, REQ-FILTER-034 충족

- **T4.3**: 하위 호환성 검증 테스트
  - 기존 `FilterCondition` 함수 설정이 변경 없이 동작하는지 확인
  - condition nil 시 pass-through 동작 확인
  - condition false 시 `ErrFilterRejected` 반환 확인
  - REQ-FILTER-040, REQ-FILTER-041 충족

- **T4.4**: 문자열 조건식 통합 테스트
  - 다양한 조건식 문자열로 Configure → Process 라이프사이클 테스트
  - 파싱 에러 시 Configure 에러 반환 테스트
  - 빈 문자열 에러 테스트

**완료 기준:** FilterNode 통합 테스트 전체 PASS + 기존 테스트 회귀 없음

### 마일스톤 5: 동시성 검증 및 최종 품질 (Secondary Goal)

동시성 안전 검증 및 전체 품질 게이트 통과

**작업 목록:**

- **T5.1**: 동시성 안전 테스트
  - 다중 고루틴 동시 Process 호출 테스트
  - Configure/Process 동시 호출 테스트
  - `go test -race` 플래그로 전체 검증
  - REQ-FILTER-050, REQ-FILTER-051 충족

- **T5.2**: 전체 테스트 스위트 실행
  - `go test -race -cover ./internal/node/...`
  - 커버리지 85% 이상 확인
  - `go vet ./...` 통과 확인

- **T5.3**: 엣지 케이스 보완
  - 매우 긴 조건식 처리
  - 깊은 중첩 괄호 처리
  - 유니코드 문자열 리터럴 처리
  - 숫자 경계값 (MaxFloat64, NaN, Inf) 처리

- **T5.4**: 예제 플로우 업데이트 (해당하는 경우)
  - YAML 플로우 예제에 문자열 조건식 사용 예시 추가

**완료 기준:** 전체 테스트 PASS + 커버리지 85%+ + race 검증 통과

## 3. 기술 접근 방식

### 3.1 렉서 설계

문자 단위 순차 스캔 방식:

```
tokenize(expr string) ([]Token, error):
  pos := 0
  tokens := []Token{}

  while pos < len(expr):
    ch := expr[pos]
    switch:
      공백 -> pos++ (건너뛰기)
      '$' -> scanPath() -> TOKEN_PATH
      '>' -> peek('=') ? ">=" : ">"  -> TOKEN_COMPARE_OP
      '<' -> peek('=') ? "<=" : "<"  -> TOKEN_COMPARE_OP
      '=' -> expect('=') -> "=="     -> TOKEN_COMPARE_OP
      '!' -> peek('=') ? "!=" : "!"  -> TOKEN_COMPARE_OP 또는 TOKEN_NOT
      '&' -> expect('&') -> "&&"     -> TOKEN_LOGICAL_OP
      '|' -> expect('|') -> "||"     -> TOKEN_LOGICAL_OP
      '(' -> TOKEN_LPAREN
      ')' -> TOKEN_RPAREN
      '\'' 또는 '"' -> scanString()  -> TOKEN_STRING
      숫자 또는 '-' -> scanNumber()  -> TOKEN_NUMBER
      문자 -> scanKeyword()          -> TOKEN_BOOL / TOKEN_NULL / TOKEN_FUNC
      기타 -> 에러

  tokens = append(tokens, TOKEN_EOF)
  return tokens, nil
```

### 3.2 파서 설계

재귀 하강 파서로 연산자 우선순위를 자연스럽게 구현:

```
parseExpression() -> parseOrExpr()

parseOrExpr():
  left = parseAndExpr()
  while current == "||":
    advance()
    right = parseAndExpr()
    left = LogicalNode{left, OR, right}
  return left

parseAndExpr():
  left = parseUnaryExpr()
  while current == "&&":
    advance()
    right = parseUnaryExpr()
    left = LogicalNode{left, AND, right}
  return left

parseUnaryExpr():
  if current == "!":
    advance()
    expr = parseUnaryExpr()  // 재귀로 !! 지원
    return NotNode{expr}
  return parsePrimaryExpr()

parsePrimaryExpr():
  if current == "(":
    advance()
    expr = parseOrExpr()
    expect(")")
    return expr
  if current == TOKEN_FUNC("exists"):
    advance()
    expect("(")
    path = expect(TOKEN_PATH)
    expect(")")
    return ExistsNode{path}
  if current == TOKEN_PATH:
    path = advance()
    op = expect(TOKEN_COMPARE_OP)
    literal = parseLiteral()
    return ComparisonNode{PathExpr{path}, op, literal}
  error: unexpected token
```

### 3.3 평가기 설계

각 AST 노드가 `Evaluate(data map[string]any) (bool, error)` 인터페이스를 구현:

```
ComparisonNode.Evaluate(data):
  val, err = evaluatePath(data, left.Path)
  if err == ErrPathNotFound:
    if right == null && op == "==": return true
    return false
  // 타입별 비교 수행

LogicalNode.Evaluate(data):
  leftResult = left.Evaluate(data)
  if op == AND && !leftResult: return false  // 단락 평가
  if op == OR && leftResult: return true     // 단락 평가
  return right.Evaluate(data)
```

### 3.4 compileCondition 설계

TransformNode의 `compileExpression` 패턴을 참조:

```
compileCondition(expr string) (FilterCondition, error):
  tokens, err = tokenize(expr)
  if err: return nil, err

  ast, err = parse(tokens)
  if err: return nil, err

  return func(msg message.Message) bool {
    data = messageToMap(msg)
    result, _ = ast.Evaluate(data)
    return result
  }, nil
```

### 3.5 Configure 확장 설계

TransformNode.Configure 패턴을 따름:

```
FilterNode.Configure(config):
  BaseNode.Configure(config)

  cond, ok = config["condition"]
  if !ok: return nil  // condition 미설정 -> pass-through

  // 1순위: Go 함수 타입
  if fn, ok = cond.(FilterCondition):
    n.condition = fn
    return nil

  // 2순위: 문자열 조건식
  if expr, ok = cond.(string):
    if expr == "": return ErrInvalidExpression
    fn, err = compileCondition(expr)
    if err: return err
    n.condition = fn
    return nil

  return nil
```

## 4. 아키텍처 설계 방향

### 4.1 파일 구조

```
internal/node/
├── condition.go          (신규) 렉서 + 파서 + 평가기 + 컴파일러
├── condition_test.go     (신규) 단위 테스트
├── filter.go             (수정) Configure 확장
├── filter_test.go        (수정) 통합 테스트 확장
└── errors.go             (수정) 필요 시 에러 추가
```

### 4.2 주요 타입 및 함수

| 이름 | 종류 | 역할 |
|------|------|------|
| `TokenType` | 상수 | 토큰 타입 열거 |
| `Token` | 구조체 | 토큰 값 및 위치 |
| `ConditionNode` | 인터페이스 | AST 노드 공통 인터페이스 |
| `ComparisonNode` | 구조체 | 비교 표현식 AST |
| `LogicalNode` | 구조체 | 논리 표현식 AST |
| `NotNode` | 구조체 | 부정 표현식 AST |
| `ExistsNode` | 구조체 | 존재 확인 AST |
| `tokenize` | 함수 | 문자열 → 토큰 배열 |
| `parse` | 함수 | 토큰 배열 → AST |
| `compileCondition` | 함수 | 문자열 → FilterCondition |

### 4.3 의존성 관계

```
condition.go
  ├── uses: evaluatePath (pkg/message/path.go)
  ├── uses: messageToMap (expression.go)
  ├── uses: ErrInvalidExpression (errors.go)
  └── produces: FilterCondition (filter.go)

filter.go (Configure)
  ├── uses: compileCondition (condition.go)
  └── priority: FilterCondition(Go 함수) > string(조건식)
```

## 5. 리스크 및 대응 방안

| 리스크 | 가능성 | 영향도 | 대응 방안 |
|--------|--------|--------|-----------|
| 기존 FilterNode 테스트 회귀 | 낮음 | 높음 | Configure 확장 시 기존 분기 보존, 회귀 테스트 우선 실행 |
| 복잡한 조건식 파싱 에러 | 중간 | 중간 | 에러 메시지에 위치 정보 포함, 다양한 에러 케이스 테스트 |
| evaluatePath 반환값 타입 불일치 | 중간 | 중간 | 타입 스위치로 안전한 변환, 미지원 타입 시 false 반환 |
| 재귀 깊이 초과 (매우 긴 조건식) | 낮음 | 낮음 | Go 기본 스택이 충분, 극단적 케이스는 문서화 |
| messageToMap 성능 오버헤드 | 낮음 | 낮음 | 각 Process 호출당 1회만 변환, IoT 시나리오에서 허용 가능 |
| 동시성 문제 (condition 교체) | 낮음 | 높음 | 기존 sync.RWMutex 패턴 유지, `-race` 플래그로 지속 검증 |

## 6. 다음 단계

구현 완료 후:
- `/moai:2-run SPEC-FILTER-001` 명령으로 Hybrid 모드 구현 시작
  - 신규 `condition.go`: TDD (RED-GREEN-REFACTOR)
  - 기존 `filter.go` 수정: DDD (ANALYZE-PRESERVE-IMPROVE)
- SPEC-AGG-001 통합 시, AggregateNode 출력의 stats 맵을 FilterNode 조건식으로 필터링하는 시나리오 검토
- `/moai:3-sync SPEC-FILTER-001` 명령으로 문서 동기화
