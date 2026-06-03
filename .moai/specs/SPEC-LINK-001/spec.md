---
id: SPEC-LINK-001
title: "가상(네임드) 링크 — 와이어 가상화 표시로 캔버스 연결 간소화"
version: "1.0.0"
status: planned
created: "2026-06-04"
updated: "2026-06-04"
author: "xtra"
priority: medium
related_specs:
  - SPEC-WIRE-001
  - SPEC-WEB-001
tags:
  - link
  - virtual-wire
  - wire
  - edge
  - canvas
  - editor
  - ui
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-04 | xtra | 초기 SPEC 작성 |

# SPEC-LINK-001: 가상(네임드) 링크 — 와이어 가상화 표시로 캔버스 연결 간소화

## 1. 개요

### 1.1 목적

노드가 많거나 한 노드가 여러 노드와 연결되면 와이어(커넥션)가 많아져 캔버스가 복잡(스파게티)해진다. 본 SPEC은 이를 **시각적으로** 간소화한다. 핵심은 **와이어 가상화 표시**이다: 기존 와이어(edge)를 그대로 두되, `virtual=true` 인 와이어는 캔버스에서 긴 연결선을 그리지 않고 양 끝을 짧은 스텁(배지)으로 분해 표시한다.

### 1.2 핵심 모델 (확정 — 반드시 준수)

> **중요**: 별도의 link 노드를 만들거나 엔진이 이름으로 연결을 합성하는 방식이 **아니다.** 다음의 "와이어 가상화 표시" 모델만 사용한다.

- **기존 와이어(edge) 데이터 구조와 엔진 라우팅을 그대로 사용한다.** 메시지 전달 동작은 기존 와이어와 100% 동일하다(큐/카운터/브로드캐스트/순서 모두 불변). 엔진에는 라우팅 로직 변경이 없다.
- 와이어에 두 가지 옵션을 추가한다:
  - `virtual` (bool, 기본 false): 가상화(화면에서 긴 선을 숨김) 여부. **신규 추가 대상.**
  - `name` (string): 링크 이름(그룹 식별). **이미 `Wire.Name` 이 존재하므로 재사용한다** (`pkg/flow/connection.go` 확인 완료).
- `virtual=true` 인 와이어는 **하나의 실제 와이어**이지만 화면에서는 긴 선 대신 **양 끝을 짧은 스텁+배지로 분해 표시**한다:
  - 출발(소스) 포트 옆: "출력 링크 [name]" 배지
  - 도착(타겟) 포트 옆: "입력 링크 [name]" 배지
  - 중간의 긴 연결선은 그리지 않는다.
- **이름 기반 그룹(플로우 내 범위)**: 같은 `name` 을 가진 가상 와이어들은 같은 링크로 취급(표시상). 이름 공간은 플로우 단위(플로우마다 독립).
- **표시 보조는 이름 라벨만**(자동 색상/배지 색 구분 없음).
- **링크 클릭 시 연결성 표시**: 가상 링크 배지를 클릭하면 같은 이름(또는 그 와이어의 실제 상대 엔드포인트들)을 하이라이트하여 어디에 연결되어 있는지 보여준다.
- 다대다 관계는 기존 와이어의 fan-out/fan-in 으로 자연 표현된다: 출력 포트 1개가 여러 입력 포트로 가면 그만큼 실제 와이어가 있고, 모두 같은 이름으로 가상화하면 한 그룹으로 표시된다. 입력↔입력, 출력↔출력은 와이어가 본래 output→input 이므로 연결성이 없다(불변식).

### 1.3 배경

- xflow 는 IoT FBP(Flow Based Programming) 플랫폼이며, 웹 에디터(React Flow)에서 노드와 와이어로 플로우를 구성한다.
- 와이어는 백엔드에서 `flow.Wire` 구조체로 표현되고, 프론트의 React Flow `edge` 와 `internal/api/service/flow_adapter.go` 의 `convertReactFlowEdgesToWires` 로 상호 변환된다.
- 이미 커스텀 엣지(`web/src/components/flow/CustomEdge.tsx`)와 `edgeTypes = { custom: CustomEdge }` (`web/src/pages/editor/EditorPage.tsx`) 가 도입되어 있어, 가상 엣지 렌더링은 이 커스텀 엣지 경로를 확장하여 구현할 수 있다.

### 1.4 범위

**포함:**
- 백엔드: `flow.Wire` 에 `virtual` bool 필드 추가(+ 기존 `Name` 보존). flow_adapter edge↔wire 변환·저장·export/import 시 `virtual`/`name` 보존.
- 프론트: EdgePropertyPanel 에 "가상 링크(virtual)" 토글 + "링크 이름(name)" 입력. 커스텀 엣지로 가상 와이어를 "선 숨김 + 엔드포인트 배지"로 렌더. 같은 이름 그룹/상대 엔드포인트 하이라이트.

**제외:**
- 엔진 라우팅 로직 변경 (없음. 가상 와이어도 일반 와이어로 라우팅).
- 별도 link 노드 생성, 엔진의 이름 기반 연결 합성 (명시적 금지).
- 자동 색상/배지 색 구분 (표시 보조는 이름 라벨만).

## 2. 환경

| 항목 | 상세 |
|------|------|
| 백엔드 런타임 | Go 1.23+ |
| 백엔드 대상 모듈 | `pkg/flow` (Wire), `internal/api/service` (flow_adapter) |
| 프론트 런타임 | TypeScript 5.9+, React 19, `@xyflow/react` (React Flow) |
| 프론트 대상 모듈 | `web/src/stores/editorStore.ts`, `web/src/components/property/EdgePropertyPanel.tsx`, `web/src/components/flow/CustomEdge.tsx`, `web/src/components/flow/CustomNode.tsx`, `web/src/pages/editor/EditorPage.tsx` |
| 테스트 | Go `testing`+`testify`, 프론트 Vitest + Testing Library |
| 개발 방법론 | Hybrid (신규 = TDD, 기존 변경 = 동작 보존 DDD) |

## 3. 가정

- **A1**: `Wire.Name` (string) 이 이미 존재하므로 링크 이름은 이 필드를 재사용한다 (`pkg/flow/connection.go` 확인).
- **A2**: `virtual` 은 표시(렌더링) 전용 플래그이며, 엔진의 와이어 라우팅/큐/카운터/브로드캐스트/순서에 어떠한 영향도 주지 않는다.
- **A3**: 이름 공간은 플로우 단위로 독립이다. 같은 플로우 내에서만 같은 `name` 이 같은 링크로 취급된다.
- **A4**: `flow_adapter.go` 의 edge→wire 변환은 이미 `name` 을 통과시킨다(`converted["name"]`). `virtual` 만 통과 로직을 추가하면 된다.
- **A5**: 프론트 엣지 메타는 React Flow `edge` 의 추가 속성(`name`, `mode`, `buffer_size` 등)으로 보존된다. `virtual` 도 동일 방식으로 추가한다.
- **A6**: 가상화 토글(on/off)은 "표시 전환"이며 라우팅을 바꾸지 않는다. 선택만으로는 dirty 가 되지 않아야 하며(이미 `onEdgesChange` 가 `select` 변경을 dirty 에서 제외), `virtual`/`name` 의 실제 변경(편집)만 dirty 로 처리한다.
- **A7**: 같은 소스 포트 + 같은 이름의 가상 와이어가 여러 개일 때의 배지 표기 규칙은 OPEN QUESTION 으로 남기고, 본 SPEC 구현 시 결정한다(Section 5.5 참조).

## 4. 요구사항 (EARS)

### 4.1 백엔드 — Wire 모델 및 변환 (최소 변경)

**REQ-LINK-001**: Wire.virtual 필드 추가
시스템은 **항상** `flow.Wire` 구조체에 `virtual` bool 필드(JSON 태그 `virtual`, 기본 false)를 포함해야 한다. 기존 `Name` 필드는 변경 없이 보존한다.

**REQ-LINK-002**: edge→wire 변환 시 virtual/name 보존
**WHEN** `convertReactFlowEdgesToWires` 가 React Flow edge 를 wire 로 변환할 때, **THEN** 시스템은 edge 의 `virtual`(bool) 과 `name`(string) 값을 변환 결과 wire 에 그대로 보존해야 한다.

**REQ-LINK-003**: 저장/로드 시 virtual/name 보존
시스템은 **항상** 플로우 정의의 저장 및 재로드 후에도 각 wire 의 `virtual`/`name` 값을 변경 없이 유지해야 한다.

**REQ-LINK-004**: export/import 시 virtual/name 보존
**WHEN** 플로우를 export 하고 다시 import 할 때, **THEN** 시스템은 각 wire 의 `virtual`/`name` 값을 유지해야 한다. (단, export redaction 로직이 적용되는 다른 필드와 충돌하지 않아야 한다.)

**REQ-LINK-005**: 엔진 라우팅 불변 (동작 보존)
시스템은 **항상** `virtual` 플래그와 무관하게 와이어 라우팅을 기존과 동일하게 수행해야 한다. 메시지 전달, 큐 동작, 카운터, 브로드캐스트, 메시지 순서는 `virtual=true` 와이어와 `virtual=false` 와이어가 완전히 동일해야 한다. 엔진은 `virtual` 플래그를 **무시**한다.

**REQ-LINK-006**: 와이어 라우팅 코드 미변경
시스템은 **항상** 엔진의 와이어 라우팅/메시지 전달 코드를 변경하지 않아야 한다 (변경 범위는 Wire 구조체 필드 추가 및 어댑터 통과로 한정).

### 4.2 프론트 — 편집(속성 패널)

**REQ-LINK-010**: 가상 링크 토글
**WHEN** 사용자가 EdgePropertyPanel 에서 "가상 링크(virtual)" 토글을 켜거나 끄면, **THEN** 시스템은 선택된 edge 의 `virtual` 값을 갱신하고 캔버스 표현을 즉시 전환해야 한다(라우팅 불변).

**REQ-LINK-011**: 링크 이름 입력
**WHEN** 사용자가 EdgePropertyPanel 에서 "링크 이름(name)" 입력을 편집하면, **THEN** 시스템은 선택된 edge 의 `name` 값을 갱신해야 한다. (현재 패널은 `name` 을 읽기 전용으로 표시하므로, 편집 가능 입력으로 확장한다.)

**REQ-LINK-012**: 편집만 dirty 처리
시스템은 **항상** `virtual`/`name` 의 실제 사용자 편집만 dirty(미저장) 상태로 표시해야 한다. 단순 edge 선택(`select`)만으로는 dirty 가 되지 않아야 한다.

### 4.3 프론트 — 캔버스 렌더링(가상화 표시)

**REQ-LINK-020**: 가상 와이어 선 숨김
**IF** edge 의 `virtual` 이 true 이면, **THEN** 시스템은 캔버스에서 해당 와이어의 긴 연결선(베지어 경로)을 그리지 않아야 한다.

**REQ-LINK-021**: 출력 링크 배지
**IF** edge 의 `virtual` 이 true 이면, **THEN** 시스템은 소스(출력) 포트 옆에 "출력 링크 [name]" 배지를 표시해야 한다.

**REQ-LINK-022**: 입력 링크 배지
**IF** edge 의 `virtual` 이 true 이면, **THEN** 시스템은 타겟(입력) 포트 옆에 "입력 링크 [name]" 배지를 표시해야 한다.

**REQ-LINK-023**: 이름 기반 그룹 표시
시스템은 **항상** 같은 플로우 내에서 같은 `name` 을 가진 가상 와이어들을 같은 링크로 취급하여 표시해야 한다. 표시 보조는 이름 라벨만 사용하며, 자동 색상/배지 색 구분은 하지 않는다.

**REQ-LINK-024**: 링크 클릭 시 연결성 하이라이트
**WHEN** 사용자가 가상 링크 배지를 클릭하면, **THEN** 시스템은 같은 이름 그룹(또는 그 와이어의 실제 상대 엔드포인트들)을 하이라이트하여 어디에 연결되어 있는지 표시해야 한다.

**REQ-LINK-025**: 비가상 와이어 렌더 불변 (하위 호환)
시스템은 **항상** `virtual=false` 인 와이어를 기존과 동일하게(베지어 곡선 연결선) 렌더링해야 한다.

### 4.4 비기능 요구사항

**REQ-LINK-030**: 메시지 전달 동작 불변
시스템은 **항상** 가상화 적용 전후로 메시지 전달/큐/카운터 동작을 동일하게 유지해야 한다(회귀 0).

**REQ-LINK-031**: 기존 플로우 호환
시스템은 **항상** `virtual` 필드가 없는 기존 플로우 정의를 정상 로드하고(기본 false 로 해석) 기존과 동일하게 동작시켜야 한다.

## 5. 명세

### 5.1 설정 키 계약

| 키 | 타입 | 기본값 | 위치 | 의미 |
|----|------|--------|------|------|
| `wire.virtual` | bool | `false` | `flow.Wire.Virtual` (`json:"virtual"`) | 화면에서 긴 선을 숨기고 엔드포인트 배지로 분해 표시 여부. 라우팅 무관. |
| `wire.name` | string | `""` | `flow.Wire.Name` (`json:"name"`, **기존 재사용**) | 링크 이름(그룹 식별). 플로우 내에서 같은 이름은 같은 링크로 표시 취급. |

프론트 edge 메타 대응:

| edge 키 | 타입 | 의미 |
|---------|------|------|
| `virtual` | boolean | `wire.virtual` 와 1:1 매핑 |
| `name` | string | `wire.name` 와 1:1 매핑(기존) |

### 5.2 Wire 구조체 변경(백엔드)

```
flow.Wire {
  ID, Name, Type,
  SourceNodeID, SourcePort, TargetNodeID, TargetPort,
  Mode, BufferSize, TTL,
  Virtual bool  // 신규: json:"virtual", 기본 false
}
```

- `Name` 은 변경 없음(이미 존재). `Virtual` 만 추가.
- `NewWire` 기본값: `Virtual = false`. (선택적으로 `WithVirtual(bool)` WireOption 추가 가능.)

### 5.3 어댑터 변환(백엔드)

`internal/api/service/flow_adapter.go` `convertReactFlowEdgesToWires`:
- 이미 `name` 통과: `if name, ok := edge["name"].(string); ok { converted["name"] = name }`.
- 추가 통과: `if v, ok := edge["virtual"].(bool); ok { converted["virtual"] = v }`.
- 그 외 `mode`/`buffer_size` 정책은 변경 없음.

### 5.4 프론트 데이터/렌더 흐름

| 단계 | 위치 | 변경 |
|------|------|------|
| 엣지 생성 메타 | `editorStore.ts` `onConnect` | edge 메타에 `virtual: false` 기본 추가 |
| 편집 | `EdgePropertyPanel.tsx` | virtual 토글 + name 편집 입력 추가 |
| 렌더 분기 | `CustomEdge.tsx` (+ EditorPage `edgeTypes`) | `data.virtual` 일 때 선 숨김 + 엔드포인트 배지 렌더 |
| 배지 배치 | `CustomNode.tsx` 핸들 영역 / 또는 오버레이 | 소스/타겟 포트 옆 배지 표시 (구현 방식 OPEN QUESTION) |
| 하이라이트 | 선택 상태(store) | 같은 이름 그룹 강조 |

### 5.5 OPEN QUESTIONS (구현 시 결정)

1. **배지 합치기 규칙**: 같은 소스 포트 + 같은 이름의 가상 와이어 N개를 "출력 링크" 배지 1개로 합칠지, 와이어별로 둘지.
2. **하이라이트 방식**: 링크 클릭 하이라이트가 "숨긴 선을 일시적으로 그림" vs "엔드포인트만 강조" 중 무엇인지.
3. **배지 구현 위치**: 배지를 포트 핸들 자체로 구현할지(핸들 옆 라벨, `NodeHandle`/`CustomNode` 확장) vs 별도 오버레이 레이어.
4. **레이아웃 충돌**: 배지/포트 라벨 겹침 처리(`CustomNode` 카드 포트 row 레이아웃과의 상호작용).
5. **export redaction**: export 시 민감정보 redaction 로직과 `virtual`/`name` 통과의 상호작용 검증.

### 5.6 파일 구조

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `pkg/flow/connection.go` | Wire 에 `Virtual` 추가 (선택 `WithVirtual` 옵션) | 수정 |
| `internal/api/service/flow_adapter.go` | edge→wire 변환에 `virtual` 통과 | 수정 |
| `web/src/stores/editorStore.ts` | `onConnect` edge 메타에 `virtual` 기본값, 편집 액션 | 수정 |
| `web/src/components/property/EdgePropertyPanel.tsx` | virtual 토글 + name 편집 입력 | 수정 |
| `web/src/components/flow/CustomEdge.tsx` | 가상 와이어 선 숨김 + 배지 렌더 분기 | 수정 |
| `web/src/components/flow/CustomNode.tsx` / `NodeHandle.tsx` | 포트 옆 배지 배치(방식 미정) | 수정(가능) |
| `web/src/pages/editor/EditorPage.tsx` | (필요 시) edgeTypes/하이라이트 연계 | 수정(가능) |

## 6. 추적성

| 요구사항 ID | 구현 위치(예정) | 검증 |
|-------------|----------------|------|
| REQ-LINK-001 | `pkg/flow/connection.go` | connection_test.go |
| REQ-LINK-002 | `flow_adapter.go` convertReactFlowEdgesToWires | flow_adapter_test.go |
| REQ-LINK-003 ~ 004 | flow_adapter 저장/export-import 경로 | flow_adapter_test.go |
| REQ-LINK-005 ~ 006 | 엔진(변경 없음) | 기존 엔진 회귀 테스트 통과 |
| REQ-LINK-010 ~ 012 | EdgePropertyPanel.tsx, editorStore.ts | EdgePropertyPanel.test.tsx |
| REQ-LINK-020 ~ 025 | CustomEdge.tsx, CustomNode.tsx | CustomEdge.test.tsx |
| REQ-LINK-030 ~ 031 | 전체(백엔드+프론트) | 회귀 테스트 + 로드 호환 테스트 |
