# Plan: Transform 노드 변환식 테이블 에디터

## Context

Transform 노드의 변환식(expression)이 현재 단일 텍스트 입력 + 모드 셀렉트로 되어 있다.
백엔드는 이미 파이프라인(배열) 형식을 지원하지만 (`[{ select: "..." }, { exclude: "..." }]`),
프론트엔드 UI에서는 활용할 수 없다. 테이블 형식 에디터를 추가하여 다중 변환 단계를 편집 가능하게 한다.

**패턴 재사용**: `RegisterMapEditor` 컴포넌트 패턴을 따른다.

---

## 백엔드 데이터 구조

`internal/node/expression.go` 에 정의된 파이프라인 형식:

```
expression: [
  { "select": "{ device_id: $.payload.device_id }" },
  { "exclude": "raw_adc, firmware" },
  { "merge": "{ source: $.metadata._source }" }
]
```

- 각 단계: 단일 키(mode) 맵 `{ [mode]: expression_string }`
- mode: `select` | `merge` | `exclude`
- 단일 문자열 형식도 하위 호환 지원 (string + mode 필드)

---

## Implementation Steps

### Step 1: ConfigField 타입에 `transform_pipeline` 추가

**파일**: `web/src/types/node.ts`

- `ConfigField.type` 유니온에 `'transform_pipeline'` 추가

### Step 2: TransformPipelineEditor 컴포넌트 생성

**파일**: `web/src/components/property/TransformPipelineEditor.tsx` (신규)

테이블 구조:
| 모드 | 변환식 | 액션 |
|------|--------|------|
| select (드롭다운) | `{ temp: $.payload.temperature }` (텍스트) | [삭제] |
| exclude | `raw_adc, firmware` | [삭제] |
| [+ 단계 추가] |

데이터 변환:
- **입력**: 배열 `[{ select: "..." }, ...]` 또는 단일 문자열(하위 호환)
- **출력**: 배열 `[{ select: "..." }, ...]`
- 단일 행인 경우에도 배열로 출력 (백엔드 호환)

내부 행 타입:
```typescript
interface PipelineRow {
  key: string;
  mode: 'select' | 'merge' | 'exclude';
  expression: string;
}
```

기존 `RegisterMapEditor` 패턴 참조:
- `toRows()` / `toExpressionPipeline()` 양방향 변환
- readOnly 지원
- 추가/삭제 버튼

### Step 3: FormField에 `transform_pipeline` 렌더링 추가

**파일**: `web/src/components/property/FormField.tsx`

- `import { TransformPipelineEditor }` 추가
- `field.type === 'transform_pipeline'` 분기 추가

### Step 4: nodeSchemas.ts 변환 노드 스키마 변경

**파일**: `web/src/config/nodeSchemas.ts`

기존:
```typescript
transform: {
  configSchema: {
    fields: [
      { name: 'expression', type: 'string', ... },
      { name: 'mode', type: 'select', options: ['select', 'merge', 'exclude'], ... },
    ],
  },
}
```

변경:
```typescript
transform: {
  configSchema: {
    fields: [
      { name: 'expression', type: 'transform_pipeline', label: '변환 파이프라인', required: true, description: '...' },
    ],
  },
}
```

- `expression` 필드의 type을 `transform_pipeline`으로 변경
- `mode` 필드 제거 (테이블 행 내 모드 선택으로 통합)

---

## Critical Files

| File | Action | Description |
|------|--------|-------------|
| `web/src/types/node.ts` | Edit | `transform_pipeline` 타입 추가 |
| `web/src/components/property/TransformPipelineEditor.tsx` | Create | 파이프라인 테이블 에디터 |
| `web/src/components/property/FormField.tsx` | Edit | transform_pipeline 렌더링 |
| `web/src/config/nodeSchemas.ts` | Edit | transform 스키마 변경 |

## Reused Code

- `web/src/components/property/RegisterMapEditor.tsx`: 테이블 에디터 패턴, 스타일, readOnly 처리
- `web/src/lib/utils/cn.ts`: className 유틸
- `internal/node/expression.go:129`: `parseExpressionSteps()` - 배열 형식 파싱 (백엔드 호환 확인)

## Verification

1. `npm run build` - 빌드 성공
2. Flow Editor에서 transform 노드 선택 → 프로퍼티 패널에 테이블 표시
3. 행 추가/삭제, 모드 변경, 변환식 편집 동작 확인
4. 기존 단일 expression 문자열 데이터 → 테이블 1행으로 표시 (하위 호환)
