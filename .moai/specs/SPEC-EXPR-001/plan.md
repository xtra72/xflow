---
id: SPEC-EXPR-001
title: "Transform Expression Engine Enhancement - Implementation Plan"
version: "1.0.0"
status: completed
created: "2026-02-28"
methodology: hybrid
---

# SPEC-EXPR-001: 구현 계획

## 개발 방법론

**Hybrid** (TDD for new + DDD for legacy)
- 신규 파일 (expr_lexer, expr_parser, expr_eval, expr_funcs): TDD (RED-GREEN-REFACTOR)
- 기존 파일 수정 (transform.go, errors.go): DDD (ANALYZE-PRESERVE-IMPROVE)

## Phase 1: 기반 구축 (에러 타입 + 렉서)

### Step 1.1: 에러 타입 추가 (DDD)
- **파일**: `internal/node/errors.go`
- **작업**: 신규 에러 변수 추가
  - `ErrTypeMismatch`
  - `ErrDivisionByZero`
  - `ErrUndefinedVariable`
  - `ErrUndefinedFunction`
  - `ErrArgumentCount`
- **방법론**: DDD — 기존 에러 패턴 분석 후 동일 패턴으로 추가

### Step 1.2: 렉서 구현 (TDD)
- **파일**: `internal/node/expr_lexer.go`, `internal/node/expr_lexer_test.go`
- **작업**: Expression 토큰화기 구현
- **RED-GREEN-REFACTOR 순서**:
  1. JSONPath 토큰 (`$.payload.field`) — 기존 condition.go 렉서 패턴 참조
  2. 변수 참조 토큰 (`$var_name`) — `$` 뒤에 `.`이 없는 경우
  3. 숫자 리터럴 (정수, 소수, 음수)
  4. 문자열 리터럴 (작은따옴표, 큰따옴표)
  5. 불리언/Null 리터럴 (`true`, `false`, `null`)
  6. 산술 연산자 (`+`, `-`, `*`, `/`, `%`)
  7. 문자열 연결 연산자 (`&`)
  8. 구조 토큰 (`{`, `}`, `[`, `]`, `(`, `)`, `:`, `,`)
  9. 식별자 토큰 (키 이름, 함수 이름)
  10. 공백/줄바꿈 무시
  11. 에러 보고 (미인식 문자)
- **예상 테스트**: 30+ 테스트 케이스

## Phase 2: 파서 구현

### Step 2.1: 파서 코어 (TDD)
- **파일**: `internal/node/expr_parser.go`, `internal/node/expr_parser_test.go`
- **작업**: 재귀 하강 파서 및 AST 노드 정의
- **RED-GREEN-REFACTOR 순서**:
  1. AST 노드 인터페이스 및 타입 정의
  2. 리터럴 파싱 (LiteralNode)
  3. JSONPath 파싱 (PathNode)
  4. 변수 참조 파싱 (VarNode)
  5. 괄호 그룹 파싱
  6. 단항 마이너스 (unary minus)
  7. 곱셈/나눗셈 파싱 (BinaryNode, `*`, `/`, `%`)
  8. 덧셈/뺄셈 파싱 (BinaryNode, `+`, `-`)
  9. 문자열 연결 파싱 (ConcatNode, `&`)
  10. 함수 호출 파싱 (CallNode)
  11. 테이블 룩업 파싱 (IndexNode, `$var[expr]`)
  12. 오브젝트 리터럴 파싱 (ObjectNode, `{ key: expr }`)
  13. 중첩 오브젝트 파싱
  14. 에러 보고 (문법 오류)
- **예상 테스트**: 40+ 테스트 케이스

## Phase 3: 평가기 + 내장 함수

### Step 3.1: 평가기 코어 (TDD)
- **파일**: `internal/node/expr_eval.go`, `internal/node/expr_eval_test.go`
- **작업**: AST 평가기 및 EvalContext 구현
- **RED-GREEN-REFACTOR 순서**:
  1. EvalContext 구조체 정의 (message, variables, functions)
  2. LiteralNode 평가
  3. PathNode 평가 (messageToMap + GetPath)
  4. VarNode 평가 (variables 조회)
  5. BinaryNode 평가 (산술 연산, 타입 변환)
  6. ConcatNode 평가 (문자열 연결)
  7. IndexNode 평가 (테이블 룩업)
  8. CallNode 평가 (함수 호출)
  9. ObjectNode 평가 (map 생성)
  10. 에러 처리 (0 나누기, 타입 불일치, 미정의 변수/함수)
- **예상 테스트**: 40+ 테스트 케이스

### Step 3.2: 내장 함수 라이브러리 (TDD)
- **파일**: `internal/node/expr_funcs.go`, `internal/node/expr_funcs_test.go`
- **작업**: 내장 함수 레지스트리 및 구현
- **RED-GREEN-REFACTOR 순서**:
  1. 함수 레지스트리 인터페이스 정의
  2. `now()` — 현재 시간
  3. `round(x)`, `floor(x)`, `ceil(x)` — 수학 함수
  4. `abs(x)`, `min(a,b)`, `max(a,b)` — 수학 함수
  5. `upper(s)`, `lower(s)`, `trim(s)` — 문자열 함수
  6. `len(s)` — 길이 함수 (문자열/배열)
  7. `int(x)`, `float(x)`, `string(x)`, `bool(x)` — 타입 변환
  8. 인자 수 검증
- **예상 테스트**: 30+ 테스트 케이스

## Phase 4: 통합

### Step 4.1: TransformNode 통합 (DDD)
- **파일**: `internal/node/transform.go`, `internal/node/transform_test.go`
- **작업**: Configure 메서드에 신규 파서 통합
- **ANALYZE-PRESERVE-IMPROVE 순서**:
  1. ANALYZE: 기존 Configure 동작 파악 (characterization test)
  2. PRESERVE: 기존 테스트 전부 통과 확인
  3. IMPROVE:
     - 변수 바인딩 수집 로직 추가
     - `compileExpressionV2` 호출 추가
     - 기존 `compileExpression` 폴백 유지
  4. 하위 호환성 테스트: 기존 모든 expression 패턴 동작 확인
- **예상 테스트**: 20+ 테스트 케이스 (기존 + 신규)

### Step 4.2: 통합 테스트
- **작업**: mqtt-to-modbus.yaml 예제의 모든 expression 패턴 테스트
  - `$address_table[key]` 룩업
  - `$._base + 0` 산술 연산
  - `$.tags.building & ":" & $.tags.floor` 문자열 연결
  - `now()` 함수 호출
  - 중첩 오브젝트 리터럴

## Phase 5: 검증

### Step 5.1: 품질 검증
- `go test -race ./internal/node/...` — 데이터 레이스 검증
- `go vet ./...` — 정적 분석
- 커버리지 목표: 85%+
- 기존 전체 테스트 통과 확인

## 예상 산출물

| 파일 | 예상 라인 수 | 유형 |
|------|-------------|------|
| expr_lexer.go | ~250 | 신규 |
| expr_parser.go | ~350 | 신규 |
| expr_eval.go | ~300 | 신규 |
| expr_funcs.go | ~200 | 신규 |
| expr_lexer_test.go | ~400 | 신규 |
| expr_parser_test.go | ~500 | 신규 |
| expr_eval_test.go | ~500 | 신규 |
| expr_funcs_test.go | ~300 | 신규 |
| transform.go | +30 | 수정 |
| transform_test.go | +200 | 수정 |
| errors.go | +10 | 수정 |

**총 신규 코드**: ~2,800 라인 (구현) + ~1,900 라인 (테스트)

## 리스크 및 완화

| 리스크 | 영향 | 완화 |
|--------|------|------|
| 기존 expression 호환성 깨짐 | 높음 | Phase 4에서 characterization test 먼저 작성 |
| 렉서에서 `$var` vs `$.path` 구분 실패 | 중간 | `$` 다음 문자가 `.`인지로 구분 (명확한 규칙) |
| 산술 연산 정밀도 문제 | 낮음 | float64 사용 (기존 패턴과 동일) |
| 중첩 오브젝트 파싱 복잡도 | 중간 | 재귀 하강 파서가 자연스럽게 처리 |
| exclude 모드와 신규 파서 충돌 | 낮음 | exclude는 기존 compileExclude 유지 |
