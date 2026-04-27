---
id: SPEC-MAP-002
type: acceptance
version: "1.0.0"
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
---

# SPEC-MAP-002: 수락 기준 - Key-Value Map Editor

## 1. 테스트 시나리오

### 시나리오 1: 타입 유니온 확장 [R-MAP2-001]

```gherkin
Given web/src/types/node.ts의 ConfigField 인터페이스가 존재한다
When ConfigField.type 유니온 타입을 확인하면
Then 'key_value_map' 리터럴이 유니온에 포함되어 있다
And 기존 타입('string', 'number', 'boolean', 'select', 'object', 'agent_select', 'register_map', 'transform_pipeline')이 모두 유지된다
And npx tsc --noEmit 컴파일이 성공한다
```

### 시나리오 2: 매핑 데이터 렌더링 (정상 경로) [R-MAP2-003, R-MAP2-007]

```gherkin
Given KeyValueMapEditor에 다음 value가 전달되었다:
  | value | {"status_code": "정상", "error_code": "에러", "warning": "경고"} |
When 컴포넌트가 렌더링되면
Then 테이블에 3개의 데이터 행이 표시된다
And 첫 번째 행의 키 입력에 "status_code", 값 입력에 "정상"이 표시된다
And 두 번째 행의 키 입력에 "error_code", 값 입력에 "에러"가 표시된다
And 세 번째 행의 키 입력에 "warning", 값 입력에 "경고"가 표시된다
And 각 행에 삭제 버튼(Trash2 아이콘)이 표시된다
```

### 시나리오 3: 빈 매핑 데이터 렌더링 [R-MAP2-003]

```gherkin
Given KeyValueMapEditor에 빈 객체 {} 가 value로 전달되었다
When 컴포넌트가 렌더링되면
Then 테이블 본문에 "매핑 항목이 없습니다" 메시지가 표시된다
And 데이터 행은 0개이다
And "+ 항목 추가" 버튼이 표시된다
```

### 시나리오 4: null/undefined 입력 처리 [R-MAP2-007]

```gherkin
Given KeyValueMapEditor에 null이 value로 전달되었다
When 컴포넌트가 렌더링되면
Then 테이블 본문에 "매핑 항목이 없습니다" 메시지가 표시된다
And 에러가 발생하지 않는다

Given KeyValueMapEditor에 undefined가 value로 전달되었다
When 컴포넌트가 렌더링되면
Then 테이블 본문에 "매핑 항목이 없습니다" 메시지가 표시된다
And 에러가 발생하지 않는다
```

### 시나리오 5: 행 추가 [R-MAP2-004]

```gherkin
Given KeyValueMapEditor에 {"a": "1"} 가 value로 전달되었다
And onChange 콜백이 등록되어 있다
When 사용자가 "+ 항목 추가" 버튼을 클릭하면
Then onChange가 호출된다
And 전달된 값에 기존 키 "a"와 빈 문자열 키 ""가 포함된다
And 테이블에 2개의 행이 표시된다
```

### 시나리오 6: 행 삭제 [R-MAP2-005]

```gherkin
Given KeyValueMapEditor에 {"a": "1", "b": "2", "c": "3"} 가 value로 전달되었다
And onChange 콜백이 등록되어 있다
When 사용자가 "b" 키 행의 삭제 버튼을 클릭하면
Then onChange가 호출된다
And 전달된 값은 {"a": "1", "c": "3"} 이다 ("b" 항목이 제거됨)
```

### 시나리오 7: 키 편집 [R-MAP2-006]

```gherkin
Given KeyValueMapEditor에 {"old_key": "value1"} 가 value로 전달되었다
And onChange 콜백이 등록되어 있다
When 사용자가 "old_key" 입력 필드를 "new_key"로 변경하면
Then onChange가 호출된다
And 전달된 값은 {"new_key": "value1"} 이다
```

### 시나리오 8: 값 편집 [R-MAP2-006]

```gherkin
Given KeyValueMapEditor에 {"key1": "old_value"} 가 value로 전달되었다
And onChange 콜백이 등록되어 있다
When 사용자가 "old_value" 입력 필드를 "new_value"로 변경하면
Then onChange가 호출된다
And 전달된 값은 {"key1": "new_value"} 이다
```

### 시나리오 9: 중복 키 처리 [R-MAP2-007]

```gherkin
Given KeyValueMapEditor에 2개의 행이 있다
And 첫 번째 행의 키가 "dup"이고 값이 "first"이다
And 두 번째 행의 키도 "dup"이고 값이 "second"이다
When 내부 행 배열이 Record<string, string>으로 변환되면
Then 결과 객체의 "dup" 키의 값은 "second"이다 (마지막 값 우선)
```

### 시나리오 10: 읽기 전용 모드 [R-MAP2-008]

```gherkin
Given KeyValueMapEditor에 readOnly={true}가 설정되었다
And value로 {"a": "1", "b": "2"} 가 전달되었다
When 컴포넌트가 렌더링되면
Then 모든 키/값 입력 필드가 disabled 상태이다
And "+ 항목 추가" 버튼이 표시되지 않는다
And 삭제 버튼 컬럼이 표시되지 않는다
And 입력 필드에 읽기 전용 스타일(opacity-60 cursor-not-allowed)이 적용된다
```

### 시나리오 11: 다크 모드 스타일 [R-MAP2-009]

```gherkin
Given 시스템이 다크 모드로 설정되어 있다
When KeyValueMapEditor가 렌더링되면
Then 테이블 테두리에 dark:border-gray-700 클래스가 적용된다
And 헤더 행에 dark:bg-gray-800 클래스가 적용된다
And 입력 필드에 dark:border-gray-600, dark:bg-gray-800, dark:text-gray-100 클래스가 적용된다
And 빈 상태 메시지에 dark:text-gray-500 클래스가 적용된다
And 삭제 버튼 hover에 dark:hover:bg-red-900/20, dark:hover:text-red-400 클래스가 적용된다
And 추가 버튼에 dark:border-gray-600, dark:text-gray-400 클래스가 적용된다
```

### 시나리오 12: FormField 통합 [R-MAP2-011]

```gherkin
Given FormField 컴포넌트에 field.type이 'key_value_map'인 ConfigField가 전달되었다
And value로 {"x": "10"} 가 전달되었다
When FormField가 렌더링되면
Then KeyValueMapEditor 컴포넌트가 렌더링된다
And value, onChange, readOnly props가 올바르게 전달된다
```

### 시나리오 13: nodeSchemas.ts 타입 업데이트 [R-MAP2-012]

```gherkin
Given web/src/config/nodeSchemas.ts의 mapping 스키마를 확인한다
When mappings 필드의 type을 조회하면
Then type이 'key_value_map'이다 ('json'이 아니다)
And npx tsc --noEmit 컴파일이 성공한다
```

### 시나리오 14: 프로퍼티 패널 통합 (E2E) [R-MAP2-011, R-MAP2-012]

```gherkin
Given 웹 UI에서 mapping 타입 노드를 선택했다
When 프로퍼티 패널이 열리면
Then "매핑 테이블" 레이블 아래에 Key-Value 테이블 에디터가 표시된다
And "소스 필드" 입력 필드가 text 입력으로 표시된다
And "기본값" 입력 필드가 text 입력으로 표시된다
And "출력 필드" 입력 필드가 text 입력으로 표시된다
```

---

## 2. 품질 게이트

### 2.1 코드 품질

- [x] `npx tsc --noEmit` TypeScript 컴파일 에러 없음
- [x] `ConfigField.type` 유니온에 `'key_value_map'` 포함 확인
- [x] `KeyValueMapEditor.tsx`가 `RegisterMapEditor.tsx`와 동일한 패턴 적용 확인
- [x] 다크 모드 클래스 (`dark:*`) 전수 적용 확인
- [x] readOnly 모드 동작 확인 (disabled, 버튼 숨김)
- [x] 한국어 주석 및 UI 텍스트 사용 (code_comments=ko)

### 2.2 TRUST 5 품질 검증

| 차원 | 검증 항목 | 기준 |
|------|----------|------|
| **Tested** | TypeScript 컴파일 | `npx tsc --noEmit` 성공 |
| **Tested** | 수동 UI 검증 | 시나리오 2~14 동작 확인 |
| **Readable** | 코드 주석 | 한국어, 파일 상단 설명 주석 |
| **Readable** | 네이밍 컨벤션 | RegisterMapEditor 패턴 일관성 유지 |
| **Unified** | 코드 스타일 | ESLint/Prettier 경고 없음 |
| **Unified** | UI 스타일 | RegisterMapEditor와 동일한 Tailwind 클래스 |
| **Secured** | 입력 검증 | null/undefined 입력 시 안전한 빈 배열 반환 |
| **Secured** | XSS 방지 | React의 기본 이스케이핑으로 충분 |
| **Trackable** | 커밋 메시지 | `feat(SPEC-MAP-002): ...` 형식 |
| **Trackable** | 요구사항 추적 | R-MAP2-001 ~ R-MAP2-012 매핑 |

---

## 3. Definition of Done

### 3.1 필수 완료 조건

- [x] `web/src/types/node.ts`에 `'key_value_map'` 타입 추가
- [x] `web/src/components/property/KeyValueMapEditor.tsx` 구현 완료
- [x] `web/src/components/property/FormField.tsx`에 핸들러 추가
- [x] `web/src/config/nodeSchemas.ts`에서 `mappings` 필드 타입 변경
- [x] `npx tsc --noEmit` TypeScript 컴파일 성공
- [x] 프로퍼티 패널에서 매핑 에디터 렌더링 확인
- [x] 행 추가/삭제/편집 동작 확인
- [x] 읽기 전용 모드 동작 확인
- [x] 다크 모드 스타일 확인

### 3.2 최종 검증

```bash
# TypeScript 타입 체크
cd web && npx tsc --noEmit

# ESLint 검증
cd web && npx eslint src/types/node.ts src/components/property/KeyValueMapEditor.tsx src/components/property/FormField.tsx src/config/nodeSchemas.ts

# 개발 서버 기동 및 수동 확인
cd web && npm run dev
```
