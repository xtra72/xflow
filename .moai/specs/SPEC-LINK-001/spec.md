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
| 1.1.0 | 2026-06-04 | xtra | 구현 반영 — 인라인 배지 → 컴팩트 인디케이터 + 측면 팝오버로 변경. 뷰 전용 표시 컨트롤 추가(`showVirtualWires` 가상 와이어 점선 표시, `focusConnectionsOnSelect`+`focusDepth` 방향성 연결 포커스). 백엔드 모델/엔진 라우팅은 불변. |

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
- `virtual=true` 인 와이어는 **하나의 실제 와이어**이지만 화면에서는 긴 선 대신 **포트 옆 컴팩트 인디케이터(링크 아이콘 + 개수)로 분해 표시**한다 (구현 반영 — 인라인 이름 배지는 노드 폭을 넓혀 폐기):
  - 가상 링크가 있는 포트 옆에 **컴팩트 LinkIndicator**(링크 아이콘 + 해당 포트의 가상 링크 개수)만 표시한다. 노드 폭에 영향을 주지 않도록 최소 폭이다.
  - 인디케이터를 **클릭하면 노드 바깥(측면)에 팝오버 목록**이 뜬다: 출력 포트는 노드의 **오른쪽**, 입력 포트는 노드의 **왼쪽**에 anchor 된다. 목록은 그 포트의 가상 링크를 **이름 그룹별 1행**으로 보이며, 각 행에 상대 끝점(상대 `노드:포트`)을 표시한다.
  - 팝오버 항목을 **선택하면 그 이름 그룹이 하이라이트**된다(같은 이름의 숨겨진 와이어를 점선으로 일시 표시 + 상대 끝점 강조).
  - 중간의 긴 연결선은 (기본 숨김 모드에서) 그리지 않는다.
- **이름 기반 그룹(플로우 내 범위)**: 같은 `name` 을 가진 가상 와이어들은 같은 링크로 취급(표시상). 이름 공간은 플로우 단위(플로우마다 독립).
- **표시 보조는 이름 라벨만**(자동 색상/배지 색 구분 없음).
- **링크 클릭 시 연결성 표시**: 인디케이터 클릭 → 팝오버에서 항목 선택 시 같은 이름 그룹(또는 그 와이어의 실제 상대 엔드포인트들)을 하이라이트하여 어디에 연결되어 있는지 보여준다.
- 다대다 관계는 기존 와이어의 fan-out/fan-in 으로 자연 표현된다: 출력 포트 1개가 여러 입력 포트로 가면 그만큼 실제 와이어가 있고, 모두 같은 이름으로 가상화하면 한 그룹으로 표시된다. 입력↔입력, 출력↔출력은 와이어가 본래 output→input 이므로 연결성이 없다(불변식).

### 1.3 배경

- xflow 는 IoT FBP(Flow Based Programming) 플랫폼이며, 웹 에디터(React Flow)에서 노드와 와이어로 플로우를 구성한다.
- 와이어는 백엔드에서 `flow.Wire` 구조체로 표현되고, 프론트의 React Flow `edge` 와 `internal/api/service/flow_adapter.go` 의 `convertReactFlowEdgesToWires` 로 상호 변환된다.
- 이미 커스텀 엣지(`web/src/components/flow/CustomEdge.tsx`)와 `edgeTypes = { custom: CustomEdge }` (`web/src/pages/editor/EditorPage.tsx`) 가 도입되어 있어, 가상 엣지 렌더링은 이 커스텀 엣지 경로를 확장하여 구현할 수 있다.

### 1.4 범위

**포함:**
- 백엔드: `flow.Wire` 에 `virtual` bool 필드 추가(+ 기존 `Name` 보존). flow_adapter edge↔wire 변환·저장·export/import 시 `virtual`/`name` 보존.
- 프론트: EdgePropertyPanel 에 "가상 링크(virtual)" 토글 + "링크 이름(name)" 입력. 커스텀 엣지로 가상 와이어 선 숨김 + 포트 옆 **컴팩트 LinkIndicator** + **측면 팝오버 목록**(LinkListPopover) 렌더. 같은 이름 그룹/상대 엔드포인트 하이라이트.
- 프론트(뷰 전용 표시 컨트롤, 구현 반영): 전역 툴바 토글로 (1) **가상 와이어 표시**(`showVirtualWires` — 숨김/점선 전환), (2) **방향성 연결 포커스**(`focusConnectionsOnSelect` + `focusDepth` 1..5/전체). 모두 표시 전용이며 라우팅/데이터/dirty/undo/저장에 영향 없음.

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

> **구현 반영 (v1.1.0)**: 초기 SPEC은 포트에 인라인 이름 배지("출력/입력 링크 [name]")를 직접 붙이는 안이었으나, 이는 노드 카드 폭을 넓히는 문제가 있어 **컴팩트 인디케이터(링크 아이콘 + 개수) + 측면 팝오버 목록** 방식으로 구현되었다. 아래 요구사항은 구현된 UI를 반영한다.

**REQ-LINK-020**: 가상 와이어 선 숨김 (기본/숨김 모드)
**IF** edge 의 `virtual` 이 true 이고 `showVirtualWires`(REQ-LINK-040)가 꺼져 있으며 클릭 강조 상태가 아니면, **THEN** 시스템은 캔버스에서 해당 와이어의 긴 연결선(베지어 경로)을 그리지 않아야 한다.

**REQ-LINK-021**: 포트 옆 컴팩트 링크 인디케이터
**IF** 한 포트(소스 또는 타겟)에 `virtual=true` 인 와이어가 하나 이상 있으면, **THEN** 시스템은 그 포트 옆에 **컴팩트 LinkIndicator**(링크 아이콘 + 해당 포트의 가상 링크 개수)를 표시해야 한다. 인디케이터는 노드 폭에 영향을 주지 않도록 최소 폭이어야 하며, 인라인 이름 배지를 표시하지 않는다.

**REQ-LINK-022**: 측면 팝오버 목록
**WHEN** 사용자가 포트의 LinkIndicator 를 클릭하면, **THEN** 시스템은 노드 바깥(측면)에 그 포트의 가상 링크 팝오버 목록(LinkListPopover)을 열어야 한다. **출력 포트는 노드의 오른쪽**, **입력 포트는 노드의 왼쪽**에 anchor 되며, 목록은 **이름 그룹별 1행**으로 각 행에 상대 끝점(상대 `노드:포트`)을 표시해야 한다.

**REQ-LINK-023**: 이름 기반 그룹 표시
시스템은 **항상** 같은 플로우 내에서 같은 `name` 을 가진 가상 와이어들을 같은 링크로 취급하여 표시해야 한다(인디케이터 개수 집계 및 팝오버 그룹핑은 (포트, 이름) 기준). 표시 보조는 이름 라벨만 사용하며, 자동 색상/배지 색 구분은 하지 않는다.

**REQ-LINK-024**: 팝오버 항목 선택 시 연결성 하이라이트
**WHEN** 사용자가 팝오버 목록에서 한 이름 그룹 항목을 선택하면, **THEN** 시스템은 그 이름 그룹(같은 `name` 의 가상 와이어들)을 하이라이트하여 어디에 연결되어 있는지 표시해야 한다. 하이라이트는 숨겨진 가상 와이어를 또렷한 파란 dashed 선으로 일시 표시하고 상대 끝점을 강조하는 방식이며(클릭으로 연결 확인), 빈 이름 그룹은 그룹 식별이 불가하므로 하이라이트하지 않는다.

**REQ-LINK-025**: 비가상 와이어 렌더 불변 (하위 호환)
시스템은 **항상** `virtual=false` 인 와이어를 기존과 동일하게(베지어 곡선 연결선) 렌더링해야 한다.

### 4.4 비기능 요구사항

**REQ-LINK-030**: 메시지 전달 동작 불변
시스템은 **항상** 가상화 적용 전후로 메시지 전달/큐/카운터 동작을 동일하게 유지해야 한다(회귀 0).

**REQ-LINK-031**: 기존 플로우 호환
시스템은 **항상** `virtual` 필드가 없는 기존 플로우 정의를 정상 로드하고(기본 false 로 해석) 기존과 동일하게 동작시켜야 한다.

### 4.5 뷰 전용 표시 컨트롤 (구현 반영 — 프론트엔드 전용)

> 모두 **표시(뷰) 전용** 토글이다. 엔진/데이터/와이어 라우팅과 무관하며, 토글·변경만으로 dirty(미저장)·undo·저장이 발생하지 않아야 한다(순수 표시 상태).

**REQ-LINK-040**: 가상 와이어 표시 토글 (`showVirtualWires`)
시스템은 **항상** 전역 툴바에 "가상 와이어 표시/숨김" 토글을 제공해야 한다.
- **숨김 모드(기본, off)**: 가상 와이어의 선을 캔버스에서 **완전히 그리지 않고**, 연결은 포트 옆 컴팩트 LinkIndicator 로만 표시한다.
- **표시 모드(on)**: 가상 와이어를 **점선(dotted)** 으로 렌더하여 일반 실선 연결과 시각적으로 구분한다.

**REQ-LINK-041**: 클릭/선택 시 가상 와이어 일시 표시 (두 모드 공통)
**WHEN** 가상 와이어가 클릭/그룹 하이라이트되거나 엣지가 선택되면, **THEN** 시스템은 `showVirtualWires` 모드와 무관하게(숨김/표시 모두) 해당 가상 와이어를 **또렷한 파란 dashed 선**으로 드러내야 한다(클릭으로 연결 확인). 표시 모드의 점선(dotted)과 클릭 강조의 dashed 는 시각적으로 구분된다.

**REQ-LINK-042**: 방향성 연결 포커스 토글 (`focusConnectionsOnSelect`)
**WHILE** 연결 포커스 토글이 켜져 있고 **정확히 한 노드**가 선택되어 있는 동안, **THEN** 시스템은 선택 노드의 **방향성 연결 사슬**만 강조하고 그 외 노드/엣지는 흐리게(dim) 처리해야 한다.
- **하류(downstream)**: 선택 노드의 출력에서 시작해 `source→target` 방향으로 따라간다.
- **상류(upstream)**: 선택 노드의 입력에서 시작해 `target→source` 방향으로 따라간다.
- 각 방향을 `focusDepth` hop 까지 독립적으로 탐색하며, 한 경로 안에서 방향을 섞지 않는다.
- 따라간(traversed) 노드 **및 엣지**를 강조하고 나머지는 흐리게 처리한다.
- **형제/무관 노드 제외**: 선택 노드의 순수 상류/하류가 아니라 단지 공통 이웃을 공유할 뿐인 형제(sibling) 노드는 강조에서 **제외**한다.
- 가상 와이어도 논리적 연결이므로 포커스 탐색에 포함하되, 숨김 모드에서 보이지 않는 가상 엣지는 강조/흐림 대상으로 렌더하지 않는다.

**REQ-LINK-043**: 연결 단계 선택 (`focusDepth`)
시스템은 **항상** 연결 포커스가 켜졌을 때 강조 단계(`focusDepth`)를 **1..5** 또는 **"전체"**(전체 연결 체인, Infinity)로 조절하는 컨트롤을 제공해야 한다. `focusDepth=1` 은 선택 노드 + 직접 상/하류 이웃이며, "전체" 는 도달 가능한 전체 방향성 체인을 강조한다(사이클은 방문 집합으로 종료). 유한 값은 [1, 5] 로 클램프한다.

**REQ-LINK-044**: 표시 컨트롤의 부작용 없음
시스템은 **항상** `showVirtualWires` / `focusConnectionsOnSelect` / `focusDepth` 의 토글·변경이 라우팅·데이터·dirty·undo·저장 어디에도 영향을 주지 않도록 해야 한다(순수 표시 상태).

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

| 단계 | 위치 | 변경 (구현 반영) |
|------|------|------|
| 엣지 생성 메타 | `editorStore.ts` `onConnect` | edge 메타에 `virtual: false` 기본 추가 |
| 편집 | `EdgePropertyPanel.tsx` | virtual 토글 + name 편집 입력 추가 |
| 렌더 분기 | `CustomEdge.tsx` (+ EditorPage `edgeTypes`) | `showVirtualWires` 숨김 모드면 선 숨김, 표시 모드면 점선(dotted). 클릭/선택/그룹 하이라이트 시 파란 dashed 로 드러냄. 연결 포커스 강조/흐림 적용. |
| 인디케이터 | `LinkIndicator.tsx` (CustomNode 포트 영역) | 가상 링크 있는 포트 옆 컴팩트 칩(아이콘+개수), 노드 폭 영향 없음 |
| 팝오버 목록 | `LinkListPopover.tsx` (NodeToolbar) | 인디케이터 클릭 시 측면 목록(출력=오른쪽/입력=왼쪽), 이름 그룹별 상대 끝점 표시 |
| 순수 헬퍼 | `lib/flow/virtualLinks.ts` | `computeLinkBadges`/`computeLinkList`(포트+이름 그룹핑, 상대 끝점 계산) |
| 연결 포커스 | `lib/flow/connectionFocus.ts`, `EditorToolbar.tsx` | 방향성 BFS(`getConnectedElements`) + 툴바 토글/depth 스테퍼 |
| 하이라이트 | 선택 상태(store `highlightedLinkName`) | 같은 이름 그룹 강조(숨긴 와이어 파란 dashed 일시 표시) |

### 5.5 OPEN QUESTIONS — 구현 시 결정됨 (RESOLVED)

1. **배지 합치기 규칙** → **결정: 합친다.** 같은 (포트, 이름) 조합의 가상 와이어 N개는 인디케이터 개수/팝오버 항목 1개로 합치고, 합쳐진 edge id 를 보존한다(`virtualLinks.ts`).
2. **하이라이트 방식** → **결정: 숨긴 선을 일시적으로 그림 + 엔드포인트 강조 병행.** 그룹 하이라이트 시 같은 이름의 가상 와이어를 또렷한 파란 dashed 로 일시 표시하고 상대 끝점을 강조한다.
3. **배지 구현 위치** → **결정: 인라인 배지 폐기, 컴팩트 인디케이터 + 측면 팝오버.** 포트 옆에는 최소 폭 LinkIndicator 만 두고, 상세는 NodeToolbar 기반 LinkListPopover(노드 바깥)로 분리해 노드 폭/레이아웃 충돌을 회피한다.
4. **레이아웃 충돌** → **해소.** 인디케이터가 최소 폭이고 목록이 노드 바깥 팝오버이므로 포트 row 레이아웃과 겹치지 않는다.
5. **export redaction** → 라운드트립 보존 검증 대상으로 유지(백엔드 `virtual`/`name` 통과는 불변).

### 5.6 파일 구조

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `pkg/flow/connection.go` | Wire 에 `Virtual` 추가 (선택 `WithVirtual` 옵션) | 수정 |
| `internal/api/service/flow_adapter.go` | edge→wire 변환에 `virtual` 통과 | 수정 |
| `web/src/stores/editorStore.ts` | `onConnect` edge 메타에 `virtual` 기본값, 편집 액션 | 수정 |
| `web/src/components/property/EdgePropertyPanel.tsx` | virtual 토글 + name 편집 입력 | 수정 |
| `web/src/components/flow/CustomEdge.tsx` | 가상 와이어 선 숨김/점선 + 클릭 dashed + 연결 포커스 강조·흐림 | 수정 |
| `web/src/components/flow/CustomNode.tsx` | 포트 옆 컴팩트 LinkIndicator 배치 + 연결 포커스 노드 흐림 | 수정 |
| `web/src/components/flow/LinkIndicator.tsx` | 포트 옆 컴팩트 칩(아이콘+개수) | 신규 |
| `web/src/components/flow/LinkListPopover.tsx` | 측면 팝오버 목록(NodeToolbar) | 신규 |
| `web/src/lib/flow/virtualLinks.ts` | 가상 링크 배지/목록 순수 헬퍼 | 신규 |
| `web/src/lib/flow/connectionFocus.ts` | 방향성 연결 BFS 순수 헬퍼 | 신규 |
| `web/src/components/flow/EditorToolbar.tsx` | 가상 와이어 표시/연결 포커스/depth 토글 | 수정 |
| `web/src/pages/editor/EditorPage.tsx` | (필요 시) edgeTypes/하이라이트 연계 | 수정(가능) |

## 6. 추적성

| 요구사항 ID | 구현 위치(예정) | 검증 |
|-------------|----------------|------|
| REQ-LINK-001 | `pkg/flow/connection.go` | connection_test.go |
| REQ-LINK-002 | `flow_adapter.go` convertReactFlowEdgesToWires | flow_adapter_test.go |
| REQ-LINK-003 ~ 004 | flow_adapter 저장/export-import 경로 | flow_adapter_test.go |
| REQ-LINK-005 ~ 006 | 엔진(변경 없음) | 기존 엔진 회귀 테스트 통과 |
| REQ-LINK-010 ~ 012 | EdgePropertyPanel.tsx, editorStore.ts | EdgePropertyPanel.test.tsx |
| REQ-LINK-020 ~ 025 | CustomEdge.tsx, CustomNode.tsx, LinkIndicator.tsx, LinkListPopover.tsx, lib/flow/virtualLinks.ts | CustomEdge.test.tsx, virtualLinks.test.ts |
| REQ-LINK-030 ~ 031 | 전체(백엔드+프론트) | 회귀 테스트 + 로드 호환 테스트 |
| REQ-LINK-040 ~ 041 | CustomEdge.tsx, EditorToolbar.tsx, editorStore.ts(`showVirtualWires`) | CustomEdge.test.tsx |
| REQ-LINK-042 ~ 043 | lib/flow/connectionFocus.ts, EditorToolbar.tsx, CustomEdge.tsx, CustomNode.tsx, editorStore.ts(`focusConnectionsOnSelect`/`focusDepth`) | connectionFocus.test.ts |
| REQ-LINK-044 | editorStore.ts (표시 전용 상태, pushUndo/isDirty 제외) | editorStore.test.ts |
