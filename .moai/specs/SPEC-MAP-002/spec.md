---
id: SPEC-MAP-002
version: "1.0.0"
status: completed
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
priority: high
category: frontend
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-11 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-MAP-002: Key-Value Map Editor - Mapping Node 매핑 테이블 편집기

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 웹 UI의 Mapping Node (SPEC-MAP-001) 프로퍼티 패널에서 **매핑 테이블(`mappings`)을 시각적으로 편집**할 수 있는 Key-Value Map Editor 컴포넌트를 추가한다.

현재 `nodeSchemas.ts`에서 `mappings` 필드의 타입이 `'json'`으로 선언되어 있으나, `ConfigField` 타입 유니온에 `'json'`이 포함되어 있지 않고, `FormField.tsx`에도 해당 타입에 대한 핸들러가 없어 매핑 테이블을 웹 UI에서 편집할 수 없는 상태이다.

기존 `RegisterMapEditor.tsx` 패턴(테이블 기반 에디터, 행 추가/삭제, Props 인터페이스)을 따라 `KeyValueMapEditor.tsx`를 구현하고, 타입 시스템과 폼 렌더러를 통합한다.

### 1.2 기술 환경

- **프레임워크**: React 19 + TypeScript 5.9+
- **빌드 도구**: Vite
- **스타일링**: Tailwind CSS (cn 유틸리티)
- **아이콘**: lucide-react (Plus, Trash2)
- **경로**:
  - `web/src/types/node.ts`: ConfigField 타입 정의
  - `web/src/components/property/FormField.tsx`: 타입별 폼 렌더러
  - `web/src/components/property/RegisterMapEditor.tsx`: 참조 패턴
  - `web/src/config/nodeSchemas.ts`: 노드 설정 스키마

### 1.3 설계 원칙

- **기존 패턴 준수**: `RegisterMapEditor.tsx`의 컴포넌트 구조(Props, useMemo, useCallback, cn, lucide-react)를 동일하게 적용
- **타입 안전성**: `ConfigField.type` 유니온에 새 타입을 추가하여 컴파일 타임 검증
- **의미적 타입명**: `'json'` 대신 `'key_value_map'`을 사용하여 용도를 명확히 표현
- **다크 모드 지원**: 기존 UI 테마와 일관된 다크 모드 스타일 적용
- **읽기 전용 지원**: `readOnly` prop으로 편집 불가 모드 지원

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `ConfigField` 타입 유니온에 `'key_value_map'` 추가 (`node.ts`)
- `KeyValueMapEditor.tsx` 신규 컴포넌트 구현
- `FormField.tsx`에 `'key_value_map'` 타입 핸들러 추가
- `nodeSchemas.ts`에서 `mappings` 필드 타입을 `'json'`에서 `'key_value_map'`으로 변경

**OUT OF SCOPE (별도 SPEC)**:
- 매핑 값의 복합 타입(객체, 배열) 지원 편집기
- JSON 스키마 기반 동적 폼 생성기
- 매핑 테이블 Import/Export (CSV, JSON 파일)
- 매핑 키 자동완성 기능

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MAP-001 | 의존 | Mapping Node 코어 구현, `nodeSchemas.ts`에 mapping 스키마 정의 |
| SPEC-WEB-001 | 의존 | 웹 UI 프레임워크, 프로퍼티 패널 렌더링 시스템 |

---

## 2. Assumptions (가정)

### 2.1 기술 가정

- `RegisterMapEditor.tsx`와 동일한 Props 인터페이스 (`{ value: unknown; onChange: (value: unknown) => void; readOnly?: boolean }`)를 사용한다
- 매핑 데이터의 외부 형태는 `Record<string, string>`이며, 키와 값 모두 문자열이다
- `cn()` 유틸리티가 `@/lib/utils/cn` 경로에서 import 가능하다
- `lucide-react` 패키지에서 `Plus`, `Trash2` 아이콘을 사용할 수 있다
- `FormField.tsx`의 기존 조건부 렌더링 패턴 (`{field.type === 'xxx' && (...)}`)을 따른다

### 2.2 프로세스 가정

- `ConfigField.type` 유니온 확장은 기존 타입 시스템에 영향을 주지 않는다 (유니온 추가는 하위 호환)
- `nodeSchemas.ts`에서 `'json'` 타입은 다른 노드에서 사용되지 않으므로, `'key_value_map'`으로 변경해도 부작용이 없다
- TypeScript 컴파일 검증(`npx tsc --noEmit`)으로 타입 호환성을 확인할 수 있다

---

## 3. Requirements (요구사항)

### 모듈 1: 타입 시스템 확장

#### R-MAP2-001: ConfigField 타입 유니온 확장 (Ubiquitous)

시스템은 **항상** `ConfigField.type` 유니온에 `'key_value_map'` 리터럴 타입을 포함해야 한다.

변경 전:
```
type: 'string' | 'number' | 'boolean' | 'select' | 'object' | 'agent_select' | 'register_map' | 'transform_pipeline'
```

변경 후:
```
type: 'string' | 'number' | 'boolean' | 'select' | 'object' | 'agent_select' | 'register_map' | 'transform_pipeline' | 'key_value_map'
```

### 모듈 2: KeyValueMapEditor 컴포넌트

#### R-MAP2-002: 컴포넌트 Props 인터페이스 (Ubiquitous)

시스템은 **항상** 다음 Props 인터페이스를 가진 `KeyValueMapEditor` 컴포넌트를 제공해야 한다:

- `value` (unknown): 외부에서 전달되는 매핑 데이터 (`Record<string, string>` 또는 기타)
- `onChange` ((value: unknown) => void): 매핑 데이터 변경 콜백
- `readOnly?` (boolean): 읽기 전용 모드 (기본값 false)

#### R-MAP2-003: 테이블 렌더링 (Ubiquitous)

시스템은 **항상** 다음 구조의 테이블을 렌더링해야 한다:

- 헤더 행: "키" | "값" 2개 컬럼 (readOnly가 아닐 때 "삭제" 컬럼 추가)
- 데이터 행: 각 매핑 항목의 키/값을 `<input type="text">` 위젯으로 표시
- 빈 상태: 매핑 항목이 없을 때 "매핑 항목이 없습니다" 메시지 표시

#### R-MAP2-004: 행 추가 기능 (Event-Driven)

**WHEN** 사용자가 "+ 항목 추가" 버튼을 클릭하면 **THEN** 시스템은 빈 키/값 쌍을 가진 새 행을 테이블 하단에 추가하고, `onChange`를 통해 업데이트된 매핑 데이터를 전달해야 한다.

#### R-MAP2-005: 행 삭제 기능 (Event-Driven)

**WHEN** 사용자가 행의 삭제 버튼(Trash2 아이콘)을 클릭하면 **THEN** 시스템은 해당 행을 테이블에서 제거하고, `onChange`를 통해 업데이트된 매핑 데이터를 전달해야 한다.

#### R-MAP2-006: 키/값 편집 기능 (Event-Driven)

**WHEN** 사용자가 키 또는 값 입력 필드를 수정하면 **THEN** 시스템은 변경된 데이터를 `Record<string, string>` 형태로 변환하여 `onChange`를 통해 전달해야 한다.

#### R-MAP2-007: 데이터 변환 (Ubiquitous)

시스템은 **항상** 다음 변환 규칙을 적용해야 한다:

- **외부 -> 내부**: `Record<string, string>` (또는 unknown)을 내부 행 배열(`{ key: string; mapKey: string; mapValue: string }[]`)로 변환
- **내부 -> 외부**: 행 배열을 `Record<string, string>`로 변환하여 `onChange`에 전달
- 중복 키 처리: 마지막 값이 우선 (뒤에 정의된 행이 앞의 동일 키를 덮어쓴다)

#### R-MAP2-008: 읽기 전용 모드 (State-Driven)

**IF** `readOnly`가 `true`이면 **THEN** 시스템은 다음을 수행해야 한다:

- 모든 입력 필드를 `disabled` 상태로 설정
- "+ 항목 추가" 버튼을 숨김
- 삭제 버튼 컬럼을 숨김
- 읽기 전용 스타일(`opacity-60 cursor-not-allowed`) 적용

#### R-MAP2-009: 다크 모드 지원 (Ubiquitous)

시스템은 **항상** `RegisterMapEditor.tsx`와 동일한 Tailwind CSS 다크 모드 클래스(`dark:*`)를 적용하여 일관된 다크 모드 UI를 제공해야 한다.

#### R-MAP2-010: 성능 최적화 (Ubiquitous)

시스템은 **항상** 다음 최적화 패턴을 적용해야 한다:

- `useMemo`: `value` prop에서 행 배열 계산
- `useCallback`: `handleAdd`, `handleRemove`, `handleChange`, `emit` 함수 메모이제이션

### 모듈 3: FormField 통합

#### R-MAP2-011: FormField 핸들러 등록 (Ubiquitous)

시스템은 **항상** `FormField.tsx`에서 `field.type === 'key_value_map'` 조건에 대해 `KeyValueMapEditor` 컴포넌트를 렌더링해야 한다:

- `value`, `onChange`, `readOnly` props를 전달
- 기존 `register_map` 핸들러와 동일한 패턴 적용

### 모듈 4: 스키마 업데이트

#### R-MAP2-012: nodeSchemas.ts 타입 변경 (Ubiquitous)

시스템은 **항상** `nodeSchemas.ts`의 `mapping.configSchema.fields`에서 `mappings` 필드의 `type`을 `'json'`에서 `'key_value_map'`으로 변경해야 한다.

---

## 4. Specifications (명세)

### 4.1 컴포넌트 구조

| 항목 | 내용 |
|------|------|
| 파일 경로 | `web/src/components/property/KeyValueMapEditor.tsx` |
| 내보내기 | `export function KeyValueMapEditor(...)` (named export) |
| 내부 타입 | `KeyValueRow { key: string; mapKey: string; mapValue: string }` |
| 변환 함수 | `toRows(value: unknown): KeyValueRow[]` |
| 역변환 함수 | `toRecord(rows: KeyValueRow[]): Record<string, string>` |
| 키 생성 | `nextKey(): string` (React key용 고유 식별자) |

### 4.2 UI 레이아웃

```
┌──────────────────────────────────────────────────┐
│  키              │  값              │  삭제       │
├──────────────────┼──────────────────┼────────────│
│ [status_code   ] │ [정상          ] │  [🗑]      │
│ [error_code    ] │ [에러          ] │  [🗑]      │
│ [warning       ] │ [경고          ] │  [🗑]      │
├──────────────────┴──────────────────┴────────────│
│  [+ 항목 추가]                                    │
└──────────────────────────────────────────────────┘
```

### 4.3 스타일 사양

- 테이블 셀 입력: `RegisterMapEditor.tsx`의 `cellInput` 클래스와 동일
- 읽기 전용: `RegisterMapEditor.tsx`의 `readOnlyInput` 클래스와 동일
- 추가 버튼: `RegisterMapEditor.tsx`의 추가 버튼 스타일과 동일
- 삭제 버튼: `RegisterMapEditor.tsx`의 삭제 버튼 스타일과 동일
- 빈 상태: `RegisterMapEditor.tsx`의 빈 상태 메시지 스타일과 동일

### 4.4 파일 구조

| 파일 | 액션 | 설명 |
|------|------|------|
| `web/src/types/node.ts` | 수정 | ConfigField.type 유니온에 `'key_value_map'` 추가 |
| `web/src/components/property/KeyValueMapEditor.tsx` | 신규 | Key-Value Map 테이블 에디터 컴포넌트 |
| `web/src/components/property/FormField.tsx` | 수정 | `'key_value_map'` 타입 핸들러 추가 |
| `web/src/config/nodeSchemas.ts` | 수정 | `mappings` 필드 타입을 `'json'` -> `'key_value_map'` 변경 |

### 4.5 추적성 태그

- `[SPEC-MAP-002]` - 본 SPEC의 모든 구현 파일에 커밋 메시지 접두사로 사용
- `[R-MAP2-001]` ~ `[R-MAP2-012]` - 각 요구사항의 구현 추적용
