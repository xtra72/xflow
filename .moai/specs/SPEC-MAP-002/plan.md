---
id: SPEC-MAP-002
type: plan
version: "1.0.0"
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
---

# SPEC-MAP-002: 구현 계획 - Key-Value Map Editor

## 1. 개요

Mapping Node(SPEC-MAP-001)의 매핑 테이블을 웹 UI에서 시각적으로 편집할 수 있는 Key-Value Map Editor 컴포넌트를 구현한다. 기존 `RegisterMapEditor.tsx` 패턴을 따라 테이블 기반 에디터를 구현하고, ConfigField 타입 시스템과 FormField 렌더러에 통합한다.

## 2. 의존성

| 의존 대상 | SPEC ID | 상태 | 설명 |
|-----------|---------|------|------|
| Mapping Node 코어 | SPEC-MAP-001 | completed | `nodeSchemas.ts`에 mapping 스키마 정의, `mappings` 필드 존재 |
| 웹 UI 프레임워크 | SPEC-WEB-001 | completed | 프로퍼티 패널, FormField 렌더링 시스템 |
| RegisterMapEditor | - | completed | 참조 패턴: 테이블 에디터 컴포넌트 구조 |

모든 의존성이 `completed` 상태이므로 즉시 구현 가능하다.

## 3. 마일스톤

### 마일스톤 1: ConfigField 타입 확장 (Primary Goal) [R-MAP2-001]

**태스크**:
- `web/src/types/node.ts`의 `ConfigField.type` 유니온에 `'key_value_map'` 리터럴 추가
- 기존 타입 유니온 끝에 파이프(`|`) 연산자로 추가

**변경 파일**: `web/src/types/node.ts` (수정)

**검증**: `npx tsc --noEmit` 컴파일 성공, 기존 코드에 영향 없음 확인

---

### 마일스톤 2: KeyValueMapEditor 컴포넌트 구현 (Primary Goal) [R-MAP2-002 ~ R-MAP2-010]

**태스크**:

1. `web/src/components/property/KeyValueMapEditor.tsx` 파일 생성
2. 내부 타입 정의:
   - `KeyValueRow { key: string; mapKey: string; mapValue: string }` 행 타입
   - `KeyValueMapEditorProps { value: unknown; onChange: (value: unknown) => void; readOnly?: boolean }` Props 타입
3. 변환 유틸리티 구현:
   - `nextKey(): string` - React 리렌더링용 고유 키 생성 (카운터 + 타임스탬프)
   - `toRows(value: unknown): KeyValueRow[]` - `Record<string, string>` -> 행 배열 변환
   - `toRecord(rows: KeyValueRow[]): Record<string, string>` - 행 배열 -> `Record<string, string>` 변환
4. 스타일 상수 정의:
   - `cellInput`: `RegisterMapEditor.tsx`와 동일한 셀 입력 스타일
   - `readOnlyInput`: 읽기 전용 스타일
5. `KeyValueMapEditor` 컴포넌트 구현:
   - `useMemo`: value -> rows 변환 캐싱
   - `useCallback`: `emit`, `handleAdd`, `handleRemove`, `handleChange` 메모이제이션
   - 테이블 렌더링: "키" | "값" | (삭제) 컬럼
   - 빈 상태 메시지: "매핑 항목이 없습니다"
   - 행 추가 버튼: "+ 항목 추가" (readOnly 시 숨김)
   - 삭제 버튼: `Trash2` 아이콘 (readOnly 시 컬럼 숨김)
   - 다크 모드: `dark:*` Tailwind 클래스 적용
   - readOnly: 입력 disabled, 버튼 숨김, 스타일 변경

**변경 파일**: `web/src/components/property/KeyValueMapEditor.tsx` (신규)

**검증**: TypeScript 컴파일 성공, 프로퍼티 패널에서 매핑 에디터 렌더링 확인

---

### 마일스톤 3: FormField 통합 (Primary Goal) [R-MAP2-011]

**태스크**:
- `web/src/components/property/FormField.tsx` 상단에 `KeyValueMapEditor` import 추가
- 기존 `register_map` 핸들러 뒤에 `'key_value_map'` 조건부 렌더링 블록 추가:
  ```tsx
  {field.type === 'key_value_map' && (
    <KeyValueMapEditor
      value={value}
      onChange={onChange}
      readOnly={readOnly}
    />
  )}
  ```

**변경 파일**: `web/src/components/property/FormField.tsx` (수정)

**검증**: TypeScript 컴파일 성공, `key_value_map` 타입 필드에서 에디터 렌더링 확인

---

### 마일스톤 4: nodeSchemas.ts 타입 업데이트 (Primary Goal) [R-MAP2-012]

**태스크**:
- `web/src/config/nodeSchemas.ts`의 `mapping.configSchema.fields` 배열에서 `mappings` 필드의 `type` 값을 `'json'`에서 `'key_value_map'`으로 변경

**변경 파일**: `web/src/config/nodeSchemas.ts` (수정)

**검증**: TypeScript 컴파일 성공, mapping 노드 선택 시 프로퍼티 패널에서 Key-Value 에디터 표시

---

## 4. 기술 접근

### 4.1 데이터 변환 전략

외부 데이터 형태(`Record<string, string>`)와 내부 행 배열(`KeyValueRow[]`)의 양방향 변환을 제공한다.

**외부 -> 내부 (`toRows`)**:
- `Record<string, string>` 객체의 엔트리를 순회하며 `{ key: nextKey(), mapKey: k, mapValue: v }` 행 생성
- 유효하지 않은 입력(null, 비객체)에 대해 빈 배열 반환

**내부 -> 외부 (`toRecord`)**:
- 행 배열을 순회하며 `Record<string, string>` 객체 구성
- 빈 키(`''`)를 가진 행도 포함 (사용자가 입력 중일 수 있음)
- 동일 키가 여러 행에 있으면 마지막 행의 값이 우선

### 4.2 React 키 관리

`RegisterMapEditor.tsx`의 `nextKey()` 패턴을 동일하게 사용:
- 모듈 스코프 카운터 + `Date.now()` 조합으로 고유 키 생성
- `useMemo`로 `value` 변경 시에만 행 배열 재계산
- 각 행의 `key` 속성은 React 리렌더링 최적화에만 사용

### 4.3 스타일 일관성

`RegisterMapEditor.tsx`에서 사용하는 Tailwind CSS 클래스를 동일하게 적용:
- `cellInput`: 테이블 셀 내 input/select 공통 스타일
- `readOnlyInput`: 읽기 전용 상태 추가 스타일
- 추가/삭제 버튼: 동일한 크기, 색상, hover 효과

### 4.4 타입 안전성

- `ConfigField.type` 유니온 확장은 TypeScript의 유니온 타입 특성상 하위 호환 (기존 코드 수정 불필요)
- `FormField.tsx`에서 `field.type === 'key_value_map'` 타입 가드를 통해 안전한 분기
- `nodeSchemas.ts`의 `type: 'json'` -> `type: 'key_value_map'` 변경은 타입 체크에 의해 `ConfigField` 호환성 검증

## 5. 위험 분석

| 위험 | 영향도 | 발생 가능성 | 대응 |
|------|--------|------------|------|
| `'json'` 타입이 다른 노드에서도 사용 | 높음 | 낮음 | nodeSchemas.ts 전체 검색으로 `'json'` 사용처 사전 확인 |
| 중복 키 입력 시 데이터 손실 | 중간 | 중간 | 마지막 값 우선 규칙 적용, 향후 중복 키 경고 UI 추가 가능 |
| 기존 mapping 노드 설정 데이터 호환성 | 중간 | 낮음 | 런타임 데이터는 JSON 객체이므로 타입 변경에 영향 없음 |
| TypeScript 컴파일 실패 | 높음 | 낮음 | `npx tsc --noEmit`으로 각 마일스톤별 검증 |

## 6. 추적성

| 요구사항 ID | 마일스톤 | 파일 |
|------------|---------|------|
| R-MAP2-001 | M1 | `web/src/types/node.ts` |
| R-MAP2-002 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-003 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-004 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-005 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-006 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-007 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-008 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-009 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-010 | M2 | `web/src/components/property/KeyValueMapEditor.tsx` |
| R-MAP2-011 | M3 | `web/src/components/property/FormField.tsx` |
| R-MAP2-012 | M4 | `web/src/config/nodeSchemas.ts` |
