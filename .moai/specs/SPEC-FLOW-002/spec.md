---
id: SPEC-FLOW-002
version: "1.0.0"
status: planned
created: "2026-04-08"
updated: "2026-04-08"
author: xtra
priority: medium
related:
  - SPEC-FLOW-001
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-08 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-FLOW-002: 플로우 가져오기 시 동일 타입 에이전트 대체 선택 기능

## 1. Environment (환경)

### 1.1 시스템 개요

플로우 가져오기(Import) 시, 참조된 에이전트 이름이 서버에 존재하지 않더라도 동일한 타입(type)의 에이전트가 존재하면 사용자가 기존 에이전트를 선택하여 대체할 수 있는 기능을 제공한다.

현재 시스템은 에이전트를 정확한 이름(name)으로만 매칭하기 때문에, 동일한 종류의 에이전트가 다른 이름으로 존재하더라도 "누락"으로 표시되며, 새로 생성하거나 수동으로 에이전트 파일을 드래그 앤 드롭해야 한다. 이 SPEC은 타입 기반 대체 매칭을 추가하여 이 한계를 해소한다.

### 1.2 기술 환경

- **프론트엔드**: React 19, TypeScript 5.x, Vite
- **대상 파일**: `web/src/components/common/ImportDialog.tsx`, `web/src/lib/utils/importParser.ts`
- **API 의존성**: `GET /api/v1/agents` (에이전트 목록 조회, 타입 정보 포함)
- **백엔드**: 변경 없음 (기존 API로 충분)

### 1.3 설계 원칙

- **최소 변경 원칙**: 기존 ImportDialog의 구조와 UX 패턴을 최대한 유지하며 기능을 확장한다
- **사용자 선택 우선**: 자동 대체 없이, 사용자가 명시적으로 대체 에이전트를 선택하도록 한다
- **기존 동작 보존**: 새로 생성(auto-create) 옵션은 그대로 유지하며, 대체 선택은 추가 옵션이다
- **타입 안전성**: TypeScript 타입 시스템을 활용하여 에이전트 매핑 데이터의 정합성을 보장한다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- 누락 에이전트에 대해 동일 타입 기존 에이전트 목록을 조회하고 표시
- 사용자가 기존 에이전트를 선택하여 대체할 수 있는 UI (드롭다운/셀렉트)
- 선택된 대체 에이전트에 따라 플로우 노드 정의의 에이전트 이름을 리매핑
- 리매핑된 플로우 데이터로 생성 API 호출

**OUT OF SCOPE (별도 SPEC)**:
- 백엔드 API 변경 (불필요)
- CLI 플로우 가져오기 에이전트 해석 (별도 SPEC)
- 에이전트 설정(config) 호환성 검증 (타입이 같으면 호환으로 간주)
- 에이전트 자동 매칭 (사용자 확인 없이 자동 대체)

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `GET /api/v1/agents` API가 각 에이전트의 `type` 필드를 반환한다 (현재 구현 확인 완료)
- A2: 플로우 내보내기 데이터의 `required_agents` 배열에 각 에이전트의 `type` 필드가 포함되어 있다 (현재 구현 확인 완료)
- A3: 동일 타입의 에이전트는 노드 설정(config)과 호환된다고 가정한다 (예: `lgcp` 타입 에이전트끼리는 교환 가능)
- A4: 에이전트 이름 리매핑은 플로우 정의(definition) 내 `nodes[].agent_ref.agent_name` 필드만 변경하면 된다
- A5: 하나의 누락 에이전트에 대해 여러 노드가 참조할 수 있으며, 리매핑 시 모든 참조를 일괄 변경한다

### 2.2 도메인 가정

- A6: 사용자는 동일 타입 에이전트의 차이점(예: 접속 대상 장비)을 이해하고 있다
- A7: 대체 에이전트 선택 시 에이전트의 설정(config)은 변경하지 않는다 (에이전트 자체 설정 사용)
- A8: 누락 에이전트의 타입 정보가 없는 경우(`type` 필드 미포함), 기존 동작(수동 생성 안내)을 그대로 유지한다

---

## 3. Requirements (요구사항)

### Module 1: 동일 타입 에이전트 조회

#### REQ-FLOW-002-01-01 (Event-Driven) 동일 타입 에이전트 필터링

**WHEN** 플로우 가져오기에서 누락 에이전트가 감지되면, **THEN** 시스템은 각 누락 에이전트의 `type`과 동일한 타입을 가진 기존 에이전트 목록을 필터링하여 제공해야 한다.

#### REQ-FLOW-002-01-02 (State-Driven) 타입 정보 없는 에이전트 처리

**IF** 누락 에이전트에 `type` 정보가 없으면, **THEN** 시스템은 해당 에이전트에 대해 대체 선택 옵션을 표시하지 않고, 기존 동작(수동 생성 안내)을 유지해야 한다.

#### REQ-FLOW-002-01-03 (State-Driven) 동일 타입 에이전트가 없는 경우

**IF** 누락 에이전트의 타입과 동일한 타입의 기존 에이전트가 없으면, **THEN** 시스템은 대체 선택 옵션을 비활성화하고 "새로 생성" 또는 "에이전트 파일 추가" 안내를 유지해야 한다.

### Module 2: 대체 에이전트 선택 UI

#### REQ-FLOW-002-02-01 (Event-Driven) 대체 에이전트 선택 드롭다운 표시

**WHEN** 누락 에이전트에 동일 타입의 기존 에이전트가 1개 이상 존재하면, **THEN** 시스템은 해당 에이전트 항목에 드롭다운 선택 컴포넌트를 표시해야 한다. 드롭다운 옵션은 다음을 포함한다:

- "새로 생성" (기본 선택, 기존 auto-create 동작)
- 동일 타입 기존 에이전트 목록 (이름 표시)
- "건너뛰기" (생성하지 않고 누락 상태 유지)

#### REQ-FLOW-002-02-02 (Event-Driven) 대체 에이전트 선택 시 상태 업데이트

**WHEN** 사용자가 드롭다운에서 기존 에이전트를 선택하면, **THEN** 시스템은 해당 누락 에이전트의 해결 방법을 "대체(substitute)"로 기록하고, 선택된 에이전트 정보를 저장해야 한다.

#### REQ-FLOW-002-02-03 (Ubiquitous) 선택 옵션별 시각적 구분

시스템은 **항상** 누락 에이전트의 해결 상태를 시각적으로 구분하여 표시해야 한다:

- "새로 생성": 기존 체크박스 + 타입 정보 표시 (현재 동작)
- "기존 에이전트 대체": 선택된 에이전트 이름과 매핑 화살표 표시
- "건너뛰기": 비활성화된 스타일로 표시

### Module 3: 에이전트 이름 리매핑

#### REQ-FLOW-002-03-01 (Event-Driven) 플로우 정의 내 에이전트 이름 치환

**WHEN** 사용자가 대체 에이전트를 선택하고 가져오기를 실행하면, **THEN** 시스템은 플로우 정의(definition) 내 모든 노드에서 원래 에이전트 이름을 선택된 대체 에이전트 이름으로 치환해야 한다.

치환 대상 필드:
- `nodes[].agent_ref.agent_name`
- `nodes[].agent_ref.agent_id` (빈 문자열로 초기화, 서버에서 재해석)

#### REQ-FLOW-002-03-02 (Unwanted) 원본 데이터 변경 금지

시스템은 리매핑 과정에서 원본 가져오기 데이터(파싱된 원본)를 직접 수정**하지 않아야 한다**. 리매핑은 가져오기 실행 시 복사본에서 수행한다.

#### REQ-FLOW-002-03-03 (Event-Driven) 다중 노드 일괄 리매핑

**WHEN** 하나의 누락 에이전트를 여러 노드가 참조하고 있을 때, **THEN** 시스템은 해당 에이전트를 참조하는 모든 노드의 에이전트 이름을 일괄적으로 치환해야 한다.

### Module 4: 가져오기 실행 통합

#### REQ-FLOW-002-04-01 (Event-Driven) 해결 방법별 분기 처리

**WHEN** 가져오기 실행 버튼을 클릭하면, **THEN** 시스템은 각 누락 에이전트의 해결 방법에 따라 다음을 수행해야 한다:

- "새로 생성": 기존 동작과 동일하게 `createAgent` API 호출
- "기존 에이전트 대체": `createAgent` 호출 없이, 플로우 정의 내 에이전트 이름만 리매핑
- "건너뛰기": 해당 에이전트에 대해 아무 작업도 수행하지 않음

#### REQ-FLOW-002-04-02 (Event-Driven) 리매핑 후 플로우 생성

**WHEN** 대체 에이전트가 선택된 상태에서 플로우 생성 API를 호출하면, **THEN** 시스템은 리매핑이 적용된 플로우 정의를 전송해야 한다.

---

## 4. Specifications (사양)

### 4.1 변경 대상 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/components/common/ImportDialog.tsx` | 수정 | 대체 에이전트 선택 UI, 리매핑 로직, 가져오기 실행 분기 |
| `web/src/lib/utils/importParser.ts` | 수정 | 에이전트 리매핑 유틸리티 함수 추가 |

### 4.2 새로운 타입 정의

```typescript
/** 누락 에이전트 해결 방법 */
type AgentResolution = 'create' | 'substitute' | 'skip';

/** 누락 에이전트 해결 상태 */
interface AgentResolutionState {
  /** 해결 방법 */
  resolution: AgentResolution;
  /** 대체 선택된 에이전트 이름 (substitute일 때만) */
  substituteAgentName?: string;
}

/** 동일 타입 기존 에이전트 */
interface ExistingAgentOption {
  name: string;
  type: string;
  status: string;
}
```

### 4.3 리매핑 로직

```
remapAgentNames(definition, remapTable) -> remappedDefinition
  - remapTable: Record<string, string> (원래이름 -> 대체이름)
  - definition.nodes 순회
  - node.agent_ref?.agent_name이 remapTable에 있으면 치환
  - node.agent_ref?.agent_id는 빈 문자열로 초기화
  - 원본은 보존, deep copy 후 변경
```

### 4.4 UI 레이아웃 변경

누락 에이전트 섹션의 각 항목에 대해:

```
기존:
  [체크박스] agent-name (타입: lgcp)
  또는
  [비활성] agent-name - 타입 정보 없음

변경 후:
  agent-name (타입: lgcp)
  [드롭다운: 새로 생성 | 기존-agent-1 | 기존-agent-2 | 건너뛰기]
  또는
  [비활성] agent-name - 타입 정보 없음 (변경 없음)
```

### 4.5 데이터 흐름

```
1. 파일 파싱 -> 누락 에이전트 감지
2. GET /api/v1/agents로 기존 에이전트 목록 조회 (기존 호출, 추가 호출 없음)
3. 누락 에이전트별 동일 타입 에이전트 필터링
4. UI 렌더링: 드롭다운으로 해결 방법 선택
5. 가져오기 실행:
   a. "새로 생성" 에이전트 -> createAgent API
   b. "대체" 에이전트 -> remapTable 구성
   c. remapTable로 definition 리매핑
   d. 리매핑된 definition으로 createFlow API
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-FLOW-002-01-01 | 동일 타입 조회 | Event-Driven | 단위 테스트 (필터링 로직) |
| REQ-FLOW-002-01-02 | 동일 타입 조회 | State-Driven | UI 테스트 (타입 없는 에이전트 표시) |
| REQ-FLOW-002-01-03 | 동일 타입 조회 | State-Driven | UI 테스트 (동일 타입 없을 때 비활성화) |
| REQ-FLOW-002-02-01 | 대체 선택 UI | Event-Driven | UI 테스트 (드롭다운 표시) |
| REQ-FLOW-002-02-02 | 대체 선택 UI | Event-Driven | 상태 관리 테스트 |
| REQ-FLOW-002-02-03 | 대체 선택 UI | Ubiquitous | 시각적 검증 |
| REQ-FLOW-002-03-01 | 이름 리매핑 | Event-Driven | 단위 테스트 (리매핑 함수) |
| REQ-FLOW-002-03-02 | 이름 리매핑 | Unwanted | 단위 테스트 (원본 불변 검증) |
| REQ-FLOW-002-03-03 | 이름 리매핑 | Event-Driven | 단위 테스트 (다중 노드 리매핑) |
| REQ-FLOW-002-04-01 | 가져오기 실행 | Event-Driven | 통합 테스트 (분기 처리) |
| REQ-FLOW-002-04-02 | 가져오기 실행 | Event-Driven | 통합 테스트 (리매핑된 데이터 전송) |
