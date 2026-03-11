# Plan: 노드 타입 페이지 - 상세 정보 표시

## Context

현재 노드 타입 페이지(`NodeTypesPage`)는 카드 그리드로 노드 타입의 기본 정보(이름, 카테고리, 설명, 소스)만 표시한다. 사용자가 요청한 기능: 포트 정보, 설정 예제, 현재 생성된 인스턴스 목록 등의 상세 정보를 볼 수 있어야 한다.

**제약**: 백엔드 `NodeTypeMeta`(`internal/node/registry.go:14`)는 `{type, category, description, source}` 4개 필드만 반환. 포트/설정 정보는 프론트엔드 정적 메타데이터로 제공. (10개 빌트인 타입이 고정이므로 합리적)

## Step 1: 정적 노드 타입 메타데이터 파일

**File (신규):** `web/src/pages/nodes/nodeTypeMeta.ts`

10개 빌트인 노드의 상세 메타데이터. Go 소스(`internal/node/*.go`의 `Configure()`)에서 추출.

```typescript
interface PortMeta { name: string; direction: 'input' | 'output' | 'error'; description: string }
interface ConfigFieldMeta { name: string; type: string; required: boolean; description: string; default?: string }
interface NodeTypeDetailMeta {
  description: string;
  ports: PortMeta[];
  configFields: ConfigFieldMeta[];
  configExample: Record<string, unknown>;
}
export const NODE_TYPE_META: Record<string, NodeTypeDetailMeta> = { ... }
```

| 타입 | 포트 | 주요 설정 키 |
|------|------|-------------|
| filter | in, out, error | `condition` |
| transform | in, out | `expression`, `mode`, `strip_nulls` |
| switch | in, out (동적) | `routes`, `default_port` |
| bridge | direction별 | `payload_format`, `topics`, `polling_interval_ms` |
| script | in, out | `script` |
| catch | in, out | `catch_types` |
| aggregate | in, out | `window_type`, `window_size`, `aggregate_fn`, `fields`, `group_by` |
| debug | in, out | `level` |
| status | in, out | `watch_nodes` |
| deadletter | in, out | `strategy` |

## Step 2: NodeTypeCard에 클릭 토글 추가

**File:** `web/src/pages/nodes/NodeTypeCard.tsx`

- Props에 `isExpanded: boolean`, `onToggle: () => void` 추가
- 카드에 `cursor-pointer` + 클릭 핸들러
- 확장 시 하단에 ChevronDown, 축소 시 ChevronRight 아이콘 표시

## Step 3: 노드 타입 상세 패널 컴포넌트

**File (신규):** `web/src/pages/nodes/NodeTypeDetailPanel.tsx`

카드 아래 확장되는 상세 패널. 4개 섹션:

1. **기능 설명** — `NODE_TYPE_META`에서 상세 설명
2. **포트** — 방향별 아이콘과 함께 리스트 (입력=파란색 화살표, 출력=초록색, 에러=빨간색)
3. **설정 필드** — 이름, 타입, 필수 여부, 설명 테이블
4. **설정 예제** — JSON 코드 블록 (`<pre>` + 스타일링)
5. **인스턴스** — 현재 플로우에서 사용 중인 노드 목록 (Step 4의 훅 사용)

## Step 4: 인스턴스 집계 커스텀 훅

**File (신규):** `web/src/hooks/useNodeTypeInstances.ts`

- 기존 `useFlows()` 훅으로 running 플로우 목록 가져오기
- `useQueries`로 각 running 플로우의 `getFlowStatus(flowId)` 병렬 호출
- `node_stats[].node_type === targetType`으로 매칭
- `FlowStatusInfo.node_stats`에 `node_type` 필드 존재 (`web/src/types/flow.ts:30`)

반환값: `{ instances: { flowId, flowName, nodeId, nodeName, processed, errors }[]; isLoading }`

기존 타입/함수 재사용:
- `FlowStatusInfo`, `NodeStatInfo` (`web/src/types/flow.ts`)
- `getFlowStatus` (`web/src/services/api/flowService.ts`)
- `useFlows` (`web/src/hooks/useFlows.ts`)

## Step 5: NodeTypesPage에 확장 상태 관리

**File:** `web/src/pages/nodes/NodeTypesPage.tsx`

- `expandedType` 상태 추가 (`string | null`)
- 카드 그리드 유지 (3열), 확장 시 카드 뒤에 `col-span-full` 상세 패널 삽입
- `filteredNodes` 배열 순회 시 확장된 타입 뒤에 `NodeTypeDetailPanel` 렌더링

레이아웃: CSS Grid의 `col-span-3`(또는 `col-span-full`)으로 패널이 행 전체 차지.

## 파일 목록

| # | 파일 | 변경 | 설명 |
|---|------|------|------|
| 1 | `web/src/pages/nodes/nodeTypeMeta.ts` | **신규** | 10개 빌트인 노드 상세 메타데이터 |
| 2 | `web/src/pages/nodes/NodeTypeDetailPanel.tsx` | **신규** | 상세 패널 컴포넌트 |
| 3 | `web/src/hooks/useNodeTypeInstances.ts` | **신규** | 플로우별 노드 인스턴스 집계 훅 |
| 4 | `web/src/pages/nodes/NodeTypeCard.tsx` | 수정 | 클릭 토글 props 추가 |
| 5 | `web/src/pages/nodes/NodeTypesPage.tsx` | 수정 | 확장 상태 + 상세 패널 연동 |

**백엔드 변경 없음.**

## Verification

1. `npx tsc --noEmit` — TypeScript 빌드 검증
2. 노드 타입 카드 클릭 → 상세 패널 확장/축소 확인
3. 포트 목록, 설정 필드, 설정 예제 JSON 정확성 확인
4. running 플로우가 있을 때 인스턴스 섹션에 노드 목록 표시 확인
5. running 플로우가 없을 때 "사용 중인 인스턴스 없음" 표시 확인
6. 카테고리 필터 및 검색이 기존대로 동작하는지 확인
