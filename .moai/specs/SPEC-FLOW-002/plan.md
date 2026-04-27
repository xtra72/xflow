---
id: SPEC-FLOW-002
type: plan
version: "1.0.0"
created: "2026-04-08"
updated: "2026-04-08"
---

# SPEC-FLOW-002 구현 계획

## 1. 개요

플로우 가져오기(Import) 시 누락 에이전트에 대해 동일 타입의 기존 에이전트를 대체 선택할 수 있는 기능을 `ImportDialog.tsx`와 `importParser.ts`에 추가한다. 백엔드 변경은 불필요하며, 프론트엔드만 수정한다.

## 2. 마일스톤

### Primary Goal: 리매핑 유틸리티 및 타입 정의 (Module 1 + Module 3)

타입 정의와 에이전트 이름 리매핑 유틸리티 함수를 먼저 구현한다. 이 함수들은 독립적으로 단위 테스트가 가능하다.

**구현 파일**:
- `web/src/lib/utils/importParser.ts` - 타입 추가 및 `remapAgentNames()` 함수

**구현 내용**:
1. `AgentResolution`, `AgentResolutionState`, `ExistingAgentOption` 타입 정의
2. `remapAgentNames(definition, remapTable)` 함수 구현
   - `definition` 객체를 deep copy
   - `nodes[]` 순회하여 `agent_ref.agent_name` 치환
   - `agent_ref.agent_id`는 빈 문자열로 초기화
3. `filterAgentsByType(agents, type)` 헬퍼 함수

**완료 기준**:
- `remapAgentNames` 함수가 정확히 에이전트 이름을 치환한다
- 원본 definition 객체가 변경되지 않는다 (불변성 보장)
- 다중 노드가 같은 에이전트를 참조할 때 모두 치환된다
- 단위 테스트 통과

### Secondary Goal: 대체 선택 UI (Module 2)

ImportDialog에 드롭다운 기반 대체 에이전트 선택 UI를 추가한다.

**구현 파일**:
- `web/src/components/common/ImportDialog.tsx` - UI 로직 변경

**구현 내용**:
1. 상태 관리 변경
   - `selectedAgentIndices: Set<number>` 대신 `agentResolutions: Map<number, AgentResolutionState>` 사용
   - `sameTypeAgents: Map<number, ExistingAgentOption[]>` - 각 누락 에이전트별 동일 타입 목록
2. 누락 에이전트 감지 후 동일 타입 필터링
   - 기존 `getAgents()` 호출 결과를 활용 (추가 API 호출 불필요)
   - 각 누락 에이전트의 `type`으로 기존 에이전트 필터링
3. UI 렌더링
   - 타입 정보가 있고 동일 타입 에이전트가 존재: 드롭다운 표시
   - 타입 정보가 있고 동일 타입 없음: "새로 생성" 체크박스 (기존 동작)
   - 타입 정보 없음: "타입 정보 없음 - 수동 생성 필요" (기존 동작)

**완료 기준**:
- 동일 타입 에이전트가 있을 때 드롭다운이 표시된다
- 드롭다운에서 선택 시 `agentResolutions` 상태가 업데이트된다
- 동일 타입 에이전트가 없을 때 기존 UI가 유지된다

### Final Goal: 가져오기 실행 통합 (Module 4)

가져오기 실행 시 해결 방법에 따른 분기 처리와 리매핑을 통합한다.

**구현 파일**:
- `web/src/components/common/ImportDialog.tsx` - `handleImport` 함수 수정

**구현 내용**:
1. `handleImport` 수정
   - `agentResolutions` 순회
   - `create`: 기존 `createAgent` 호출
   - `substitute`: `remapTable` 구성
   - `skip`: 무시
2. `remapTable`이 비어있지 않으면 `remapAgentNames` 호출
3. 리매핑된 definition으로 `createFlow` 호출

**완료 기준**:
- "새로 생성" 선택 시 기존과 동일하게 에이전트 생성 후 플로우 생성
- "기존 에이전트 대체" 선택 시 에이전트 생성 없이 리매핑된 플로우 생성
- "건너뛰기" 선택 시 해당 에이전트 무시하고 플로우 생성
- 혼합 선택 (일부 생성, 일부 대체) 정상 동작

## 3. 기술 접근

### 3.1 상태 관리 설계

기존 `selectedAgentIndices: Set<number>` (체크박스 기반)를 `agentResolutions: Map<number, AgentResolutionState>` (드롭다운 기반)로 교체한다. 이 변경은 하위 호환을 유지하면서 더 풍부한 해결 방법을 지원한다.

```
기존: Set<number> -> 선택 여부만 표현 (create or skip)
변경: Map<number, AgentResolutionState> -> create | substitute | skip
```

### 3.2 동일 타입 에이전트 필터링

기존 `handleImport`에서 이미 `getAgents()`를 호출하여 누락 에이전트를 판별하고 있다. 이 호출 결과를 재사용하여 추가 API 호출 없이 동일 타입 필터링을 수행한다.

```
기존 흐름: getAgents() -> 이름 비교 -> missingAgents 산출
추가 흐름: getAgents() 결과 + missingAgent.type -> 동일 타입 필터링
```

### 3.3 리매핑 전략

deep copy에는 `structuredClone()`을 사용한다 (브라우저 지원 충분). 리매핑은 가져오기 실행 직전에 수행하며, 원본 데이터는 변경하지 않는다.

### 3.4 기존 에이전트 파일 드래그 앤 드롭 호환

기존의 에이전트 파일 드래그 앤 드롭 기능은 그대로 유지한다. 드롭된 에이전트는 `missingAgents` 목록에 추가되며, 동일 타입 필터링에는 영향을 주지 않는다.

## 4. 리스크 및 대응

### R1: 에이전트 설정 비호환

**리스크**: 동일 타입이라도 에이전트 설정(config)이 다를 수 있다 (예: 다른 IP 주소의 장비).

**대응**: 이 SPEC에서는 타입이 같으면 호환으로 간주한다. 노드의 설정은 에이전트 설정과 독립적이며, 에이전트는 통신 채널 역할을 한다. 드롭다운에 에이전트 이름과 상태를 표시하여 사용자가 적절한 에이전트를 선택할 수 있도록 한다.

### R2: 상태 관리 복잡도 증가

**리스크**: `Set<number>` -> `Map<number, AgentResolutionState>` 전환으로 상태 관리 복잡도 증가.

**대응**: `AgentResolutionState` 타입을 명확히 정의하고, 드롭다운 변경 핸들러를 단순하게 유지한다. 기본값은 동일 타입이 있으면 `substitute`(첫 번째 에이전트), 없으면 `create`로 설정한다.

### R3: 에이전트 파일 드래그 앤 드롭과의 상호작용

**리스크**: 드롭된 에이전트 파일이 누락 에이전트 목록을 변경할 때, 드롭다운 상태와 동기화 문제.

**대응**: 드롭된 에이전트로 누락 에이전트가 보완되면 해당 항목의 해결 방법을 `create`로 초기화한다. 이미 동일 이름의 에이전트가 드롭되었으면 대체 선택보다 우선한다.

## 5. 전문가 참고 사항

### Frontend Expert (expert-frontend) 참고

- `ImportDialog.tsx`는 약 520줄로, 기존 구조를 최대한 유지하면서 확장
- `<select>` 또는 커스텀 드롭다운 컴포넌트 선택 필요 (프로젝트 내 기존 컴포넌트 확인)
- `structuredClone()`으로 deep copy (JSON.parse/stringify 대비 안전)
- 에이전트 파일 드래그 앤 드롭 기존 로직과의 상호작용 주의

### 의존성 구조

```
ImportDialog.tsx (본 SPEC 대상)
    ├── importParser.ts (타입 정의 + remapAgentNames 추가)
    ├── agentService.ts (getAgents - 기존 호출, 변경 없음)
    └── flowService.ts (createFlow - 기존 호출, 변경 없음)

백엔드 (변경 없음)
    ├── GET /api/v1/agents (에이전트 목록 + type 필드)
    └── POST /api/v1/flows (플로우 생성)
```
