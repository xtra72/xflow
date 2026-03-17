---
id: SPEC-WEB-001
type: plan
version: "1.12.0"
status: in_progress
created: "2026-03-07"
updated: "2026-03-17"
author: xtra
---

# SPEC-WEB-001: 구현 계획 - 에이전트 통계 버그 수정 및 디버깅 레벨 설정

## 1. 마일스톤 개요

| 마일스톤 | 모듈 | 우선순위 | 의존성 | 상태 |
|----------|------|----------|--------|------|
| M1: 에이전트 목록 통계 버그 수정 | Module 1 | P0 (즉시) | 없음 | 완료 |
| M2: 백엔드 컴포넌트별 로그 레벨 API | Module 2 | P1 (중요) | 없음 | 완료 |
| M3: 에이전트 로그 레벨 UI | Module 3 | P1 (중요) | M2 완료 필수 | 완료 |
| M4: 플로우 노드 통계 + 로그 레벨 UI | Module 4 | P1 (중요) | M2 완료 필수 | 완료 |
| M5: 캔버스 런타임 통계 | Module 5 | P1 (중요) | M4 완료 필수 | 완료 |
| BF: 백엔드 포트 카운터/브릿지 버그 수정 | 버그 수정 | P0 (즉시) | 없음 | 완료 |
| M7: 로그 뷰어 컴포넌트/소스 필터링 | Module 7 | P1 (중요) | SPEC-OBS-004 완료 필수 | 완료 |
| M8: 동적 포트 시스템 | Module 8 | P0 (즉시) | 없음 | 완료 |
| M9: 에러 포트 타입 지원 | Module 9 | P0 (즉시) | M8 완료 필수 | 완료 |
| M10: Handle ID 접두사 제거 | Module 10 | P0 (리팩토링) | M8/M9 완료 필수 | 완료 |
| M11: 리스트 정렬 기능 | Module 11 | P1 (신규 기능) | 없음 | 완료 |
| M12: 대시보드 패널 재구성 | Module 12 | P1 (리팩토링) | M1, M11 완료 권장 | 완료 |
| M13: Import/Export 기능 | Module 13 | P2 (개선) | 없음 | 완료 |
| M15: 화면 테마 시스템 | Module 15 | P1 (신규 기능) | 없음 (M1-M13 완료 후 M15-6 권장) | 완료 |
| M16: 대시보드 커스터마이징 | Module 16 | P1 (신규 기능) | M12 완료 (대시보드 기반) | 완료 |

---

## 2. M1: 에이전트 목록 통계 버그 수정 (P0)

### 2.1 근본 원인 분석

- **증상**: `AgentListPage.tsx`에서 uptime, messages_in, messages_out 컬럼이 항상 `-`로 표시됨
- **원인 (프론트엔드)**: `agentService.getAgents()`가 `GET /agents` 호출 시 `detail` 파라미터를 전달하지 않음
- **원인 (백엔드)**: `ListOptions` 구조체에 `Detail` 필드가 없고, `parseListOptions()`가 `detail` 쿼리 파라미터를 파싱하지 않으며, `ListAgents()`가 `agentToHandlerInfo(ag, "")` 빈 문자열로 하드코딩
- **결과**: 백엔드가 기본 필드(id, name, type, status, config, connected)만 반환하여 stats, uptime, health 필드가 누락됨

### 2.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/services/api/agentService.ts` | `getAgents()` 호출에 `detail: 'summary'` 파라미터 추가 | 소 (3줄) |
| `internal/api/dto/request.go` | `ListOptions` 구조체에 `Detail` 필드 추가 | 소 (1줄) |
| `internal/api/handler/flow.go` | `parseListOptions()`에 `ctx.Query("detail")` 파싱 추가 | 소 (1줄) |
| `internal/api/service/agent_adapter.go` | `agentToHandlerInfo(ag, "")` → `agentToHandlerInfo(ag, opts.Detail)` | 소 (1줄) |

### 2.3 기술 접근

**프론트엔드**: `agentService.ts`의 `getAgents()` 함수에서 params에 `detail: 'summary'`를 병합한다.

**백엔드**: `ListOptions`에 `Detail` 필드를 추가하고 `parseListOptions`에서 파싱하여, `ListAgents`에서 `opts.Detail` 값을 `agentToHandlerInfo`에 전달한다.

### 2.4 검증 방법

- `curl http://localhost:8080/api/v1/agents?detail=summary`로 stats/uptime 포함 확인
- 에이전트 목록 페이지 UI에서 통계가 실제 값으로 표시되는지 확인

---

## 3. M2: 백엔드 컴포넌트별 로그 레벨 API (P1)

### 3.1 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `internal/api/handler/monitor.go` | 3개 엔드포인트 핸들러 추가 (GET/PUT/DELETE) | 중 (80-120줄) |
| `internal/api/handler/monitor.go` | 라우트 등록 추가 | 소 (3줄) |
| `internal/api/handler/monitor_test.go` | 신규 엔드포인트 테스트 | 중 (100-150줄) |

### 3.2 기술 접근

#### 3.2.1 기존 인프라 활용

`observe.LevelManager` 인터페이스가 이미 다음 메서드를 제공:
- `GetLevel(component string) slog.Level`
- `SetLevel(component string, level slog.Level)`
- `SetLevelByPattern(pattern string, level slog.Level) int`
- `ListLevels() map[string]slog.Level`
- `SetDefaultLevel(level slog.Level)`

이 인터페이스를 monitor 핸들러에서 직접 활용한다.

#### 3.2.2 엔드포인트 설계

**GET /monitor/loglevel** (기존 확장):
- `LevelManager.ListLevels()`로 컴포넌트 맵 조회
- 기본 레벨도 함께 반환
- 응답 형식: `{ default_level: "info", components: { "agent.mqtt-001": "debug", ... } }`

**PUT /monitor/loglevel/{component}**:
- 요청 바디에서 `level` 추출
- 유효성 검증: debug, info, warn, error 중 하나
- `LevelManager.SetLevel(component, parsedLevel)` 호출
- 응답: `{ component: "agent.mqtt-001", level: "debug" }`

**DELETE /monitor/loglevel/{component}**:
- `LevelManager.SetLevel(component, defaultLevel)` 호출로 기본 레벨 복원
- 또는 내부 맵에서 해당 항목 제거 로직 구현
- 응답: `{ component: "agent.mqtt-001" }`

#### 3.2.3 레벨 문자열-slog.Level 매핑

| 문자열 | slog.Level |
|--------|-----------|
| `debug` | `slog.LevelDebug` (-4) |
| `info` | `slog.LevelInfo` (0) |
| `warn` | `slog.LevelWarn` (4) |
| `error` | `slog.LevelError` (8) |

### 3.3 하위 호환성

- 기존 `PUT /monitor/loglevel` (글로벌 레벨 변경) 동작을 유지
- `GET /monitor/loglevel` 응답 형식을 확장하되, 기존 클라이언트가 추가 필드를 무시할 수 있도록 설계

---

## 4. M3: 프론트엔드 로그 레벨 UI (P1)

### 4.1 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/services/api/monitorService.ts` | 3개 API 함수 추가 | 소 (20줄) |
| `web/src/pages/agents/AgentDetailPanel.tsx` | 통계 탭에 로그 레벨 드롭다운 추가 | 중 (40-60줄) |
| `web/src/pages/settings/SettingsPage.tsx` | 시스템 탭에 컴포넌트 오버라이드 목록 추가 | 중 (60-80줄) |

### 4.2 기술 접근

#### 4.2.1 monitorService.ts 확장

```typescript
// 신규 함수
export async function getLogLevels(): Promise<LogLevelInfo> { ... }
export async function setComponentLogLevel(component: string, level: string): Promise<void> { ... }
export async function resetComponentLogLevel(component: string): Promise<void> { ... }
```

#### 4.2.2 AgentDetailPanel 로그 레벨 컨트롤

- 통계 탭(`StatsTab`) 내에 로그 레벨 드롭다운을 추가
- 드롭다운 옵션: `기본값(default)`, `DEBUG`, `INFO`, `WARN`, `ERROR`
- "기본값" 선택 시 `DELETE /monitor/loglevel/agent.{id}` 호출
- 다른 레벨 선택 시 `PUT /monitor/loglevel/agent.{id}` 호출
- 변경 성공/실패 시 토스트 알림

#### 4.2.3 SettingsPage 컴포넌트 오버라이드 관리

- 시스템 탭의 기존 "로그 레벨" 카드 아래에 "컴포넌트별 로그 레벨" 섹션 추가
- `GET /monitor/loglevel` 호출하여 오버라이드 목록 로드
- 테이블 형식: 컴포넌트 이름 | 현재 레벨 | 리셋 버튼
- Viewer 역할: 목록만 조회 가능, 리셋 버튼 비활성화

---

## 5. M4: 플로우 노드 통계 + 로그 레벨 UI (P1)

### 5.1 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/pages/flows/FlowDetailPanel.tsx` | 노드 인스턴스 테이블 + In/Out 분리 + 로그 레벨 + 실시간 갱신 (신규) | 중 (200줄) |
| `web/src/pages/flows/FlowListPage.tsx` | 확장 가능 행 추가 (FlowRow 분리) | 중 (80줄 추가) |
| `web/src/hooks/useFlow.ts` | useFlowNodes에 refetchInterval 파라미터 추가 | 소 (3줄) |

### 5.2 기술 접근

- **FlowDetailPanel**: `useFlowNodes(flowId, refetchInterval)` 훅으로 노드 목록 조회. `useFlowStatus`로 running 감지 → 3초 자동 갱신. In/Out 분리 표시 (ArrowDownToLine/ArrowUpFromLine 아이콘)
- **FlowListPage**: AgentListPage 패턴을 따라 `expandedId` 상태 + FlowRow 컴포넌트로 확장 가능 행 구현
- **useFlow.ts**: `useFlowNodes`에 선택적 `refetchInterval` 파라미터 추가
- **기존 API 재활용**: `GET /flows/{id}/nodes`, `GET /flows/{id}/status`, `GET/PUT/DELETE /monitor/loglevel/node.{name}` - 신규 백엔드 변경 불필요

### 5.3 검증 방법

- 플로우 행 클릭 시 노드 목록 패널이 토글되는지 확인
- 노드 로그 레벨 드롭다운 변경 시 monitor API 호출 확인
- 플로우명 클릭 시 에디터 페이지로 이동 확인
- running 플로우의 In/Out 메시지 수가 3초마다 갱신되는지 확인

---

## 6. M5: 캔버스 런타임 통계 (P1)

### 6.1 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/contexts/RuntimeStatsContext.ts` | RuntimeStatsContext + useNodeRuntimeStats 훅 (신규) | 소 (25줄) |
| `web/src/pages/editor/EditorPage.tsx` | 런타임 통계 폴링 + Provider 래핑 | 중 (40줄 추가) |
| `web/src/components/flow/CustomNode.tsx` | 런타임 상태 표시등 + In/Out 표시 | 소 (25줄 추가) |

### 6.2 기술 접근

- **RuntimeStatsContext**: editorStore와 분리된 React Context. 런타임 데이터가 isDirty/undo/redo에 영향을 주지 않음
- **EditorPage**: `useFlowStatus` (5초)로 running 감지 → `useFlowNodes` (3초)로 노드 데이터 폴링 → 포트 direction 필터링으로 in/out 메시지 계산 → `runtimeStatsMap` 구성 → Provider로 전달
- **CustomNode**: `useNodeRuntimeStats(id)` 훅으로 런타임 통계 구독. 상태 표시등(초록/빨강/회색) + In/Out 아이콘 + 메시지 수 표시

### 6.3 검증 방법

- 에디터에서 running 플로우의 CustomNode에 상태 표시등과 In/Out이 표시되는지 확인
- 런타임 통계가 3초마다 갱신되는지 확인
- 노드 위치 이동 등 편집 작업 시 isDirty가 런타임 통계로 인해 변경되지 않는지 확인

---

## 7. BF: 백엔드 포트 카운터/브릿지 버그 수정 (P0)

### 7.1 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `internal/engine/engine.go` | 포트 카운터 초기화 - 모든 포트에 대해 atomic.Int64 초기화 | 소 (20줄) |
| `internal/node/bridge.go` | msgCh 초기화, Process() 반환값, BridgeOut Process 수정 | 소 (12줄) |
| `internal/node/bridge_test.go` | 브릿지 테스트 케이스 업데이트 | 소 (6줄) |

### 7.2 기술 접근

- **engine.go**: 포트 인스턴스 생성 시 input/output/error 모든 방향의 포트에 대해 `atomic.Int64` 카운터를 명시적으로 초기화. 기존 코드는 일부 포트 타입만 초기화하여 나머지 포트의 메시지 카운트가 0으로 고정되는 문제
- **bridge.go**: `msgCh` 채널 미초기화로 인한 nil 패닉 수정. `Process()` 메서드에서 정상 처리 시 nil 대신 에러를 반환하던 문제 수정

---

## 8. M7: 로그 뷰어 컴포넌트/소스 필터링 (P1)

### 8.1 배경

SPEC-OBS-004에서 백엔드 `wsLogWriter.Write()`가 WebSocket `log.entry` 페이로드에 `component`와 `source` 필드를 추가하도록 구현 완료. 현재 프론트엔드 `MonitoringPage.tsx`의 `handleLog`는 `level`, `message`, `timestamp`만 추출하고, `LogViewer.tsx`의 `LogEntry` 인터페이스에도 해당 필드가 없음.

### 8.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/pages/monitoring/LogViewer.tsx` | LogEntry 인터페이스 확장, SourceType/SOURCE_STYLES 추가, source 필터 버튼, 컴포넌트 검색 입력, 행 표시 확장 | 대 (120-150줄 추가/수정) |
| `web/src/pages/monitoring/MonitoringPage.tsx` | handleLog 콜백에서 component/source 추출 | 소 (5줄 수정) |

### 8.3 기술 접근

#### 8.3.1 LogEntry 인터페이스 확장

기존 `LogEntry`에 optional 필드 2개 추가:

- `component?: string` — 백엔드 component 이름 (예: `agent.modbus-001`, `node.transform-1`, `unknown`)
- `source?: string` — classifySource() 결과 (예: `agent`, `node`, `flow`, `api`, `engine`, `system`)

optional로 설계하여 기존 LogEntry 호환성을 유지한다.

#### 8.3.2 MonitoringPage handleLog 수정

기존 타입 캐스팅:
```typescript
const d = data as { level?: string; message?: string; timestamp?: string };
```

변경:
```typescript
const d = data as { level?: string; message?: string; timestamp?: string; component?: string; source?: string };
```

LogEntry 생성 시 추가:
```typescript
component: d.component ?? '',
source: d.source ?? 'system',
```

#### 8.3.3 Source 배지 스타일 맵

`LEVEL_STYLES`와 동일한 패턴으로 `SOURCE_STYLES` 정의:

| Source | 배지 색상 | dark 모드 |
|--------|----------|-----------|
| `agent` | `bg-purple-100 text-purple-700` | `bg-purple-900 text-purple-300` |
| `node` | `bg-teal-100 text-teal-700` | `bg-teal-900 text-teal-300` |
| `flow` | `bg-green-100 text-green-700` | `bg-green-900 text-green-300` |
| `api` | `bg-orange-100 text-orange-700` | `bg-orange-900 text-orange-300` |
| `engine` | `bg-indigo-100 text-indigo-700` | `bg-indigo-900 text-indigo-300` |
| `system` | `bg-gray-200 text-gray-600` | `bg-gray-700 text-gray-400` |

#### 8.3.4 필터 상태 관리

3가지 독립 필터를 AND 결합:

1. **레벨 필터** (기존): `filter: LogLevel | null` — 단일 선택
2. **소스 필터** (신규): `sourceFilter: Set<SourceType>` — 다중 선택 토글 (빈 Set = 전체)
3. **컴포넌트 검색** (신규): `componentSearch: string` — 텍스트 입력 (300ms 디바운스)

필터 적용 순서:
```
entries → level 필터 → source 필터 → component 검색 → filtered
```

#### 8.3.5 UI 레이아웃

**툴바 영역 (2행 구성)**:

행 1 (기존 확장):
- 좌측: `전체` | `DEBUG` | `INFO` | `WARN` | `ERROR` (기존 레벨 필터)
- 좌측 이어서: `│` 구분선 + `agent` | `node` | `flow` | `api` | `engine` | `system` (source 필터, 다중 선택 가능)
- 우측: 필터 결과 건수 + 자동 스크롤 토글 (기존)

행 2 (신규):
- 좌측: `Search` 아이콘 + 컴포넌트 검색 입력 (placeholder: "컴포넌트 필터...")
- 우측: 활성 필터 요약 또는 필터 초기화 버튼

**로그 행 레이아웃 (기존 확장)**:

기존: `[timestamp 140px] [LEVEL 50px] [message 나머지]`

변경: `[timestamp 140px] [LEVEL 50px] [source 55px] [component 140px] [message 나머지]`

- source 배지: `text-[10px]` 크기, SOURCE_STYLES 색상, 고정 너비 `w-[55px]` 중앙 정렬
- component 텍스트: `text-gray-500` 색상, `w-[140px]` 고정 너비, `truncate` 오버플로 처리

#### 8.3.6 성능 고려사항

- 필터링은 `useMemo`로 캐싱하여 불필요한 재계산 방지
- 컴포넌트 검색은 `useDebounce` 커스텀 훅으로 300ms 디바운스 적용
- MAX_ENTRIES(10,000건)에서 3중 필터 AND 결합이 16ms 이내에 완료되도록 단순 순회 방식 사용
- ROW_HEIGHT(28px) 유지 — source/component 배지는 기존 행 높이 내에 수용 가능

### 8.4 검증 방법

- 모니터링 페이지 로그 탭에서 source 배지와 component 이름이 표시되는지 확인
- source 필터 버튼 다중 선택/해제 시 로그 필터링이 올바르게 동작하는지 확인
- 컴포넌트 검색에 `modbus` 입력 시 해당 이름 포함 로그만 표시되는지 확인
- 레벨 필터 + source 필터 + 컴포넌트 검색 동시 적용 시 AND 결합 동작 확인
- 10,000건 로그 상태에서 필터 전환 시 UI 렌더링 지연이 없는지 확인

---

## 9. M8: 동적 포트 시스템 (P0)

### 9.1 근본 원인 분석

- **증상**: 플로우 노드의 입출력 포트가 설정한 갯수/방향과 다르게 표시됨
- **원인 1 (브릿지 포트)**: `BRIDGE_DEFAULT_PORTS`가 direction과 무관하게 항상 `[input, output]`을 반환. BridgeIn(agent->flow)은 output만, BridgeOut(flow->agent)은 input만 표시해야 함
- **원인 2 (스위치 포트)**: 스위치 노드가 `[in, out]` 고정 포트를 사용하지만, 라우트 수에 따라 다수의 출력 포트가 필요함
- **원인 3 (포트 미갱신)**: `getDefaultPorts(nodeType)`가 노드 생성 시점에만 호출되고, 설정 변경(direction, routes) 시 포트가 재계산되지 않음

### 9.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/config/nodeSchemas.ts` | `computePortsForNode(nodeType, config)` 함수 추가, 브릿지/스위치 동적 포트 로직 | 중 (40-60줄) |
| `web/src/pages/editor/EditorPage.tsx` | 노드 생성 시 `computePortsForNode` 사용, 설정 변경 시 포트 재계산 + 삭제 엣지 정리 | 중 (30-50줄 추가/수정) |
| `pkg/flow/node.go` | `NewNodeDef()` 또는 별도 함수에서 브릿지 direction 기반 포트 생성 | 소 (20-30줄) |
| `web/src/components/flow/CustomNode.tsx` | (선택) error 포트 렌더링 필요 시 수정 | 소 (5-10줄) |

### 9.3 기술 접근

#### 9.3.1 프론트엔드: `computePortsForNode` 함수

`web/src/config/nodeSchemas.ts`에 노드 타입과 config를 받아 포트를 동적 계산하는 함수를 추가한다:

- **bridge 타입**: `config.direction` 값에 따라 포트 결정
  - `in` → output 포트만 (`[{name:'out', direction:'output'}]`)
  - `out` → input 포트만 (`[{name:'in', direction:'input'}]`)
  - `inout` / `request_reply` → 양방향 (`[input, output]`)
  - 미설정 → 기본값 양방향
- **switch 타입**: `config.routes` 배열에 따라 포트 결정
  - routes 존재 → input 1개 + 라우트별 output + default output
  - routes 미존재 → 기본값 `[in, out]`
- **기타 타입**: 기존 `getDefaultPorts(nodeType)` 로직 그대로 위임

기존 `getDefaultPorts(nodeType)` 함수는 하위 호환성을 위해 유지한다. config가 없는 호출에 대한 폴백으로 동작한다.

#### 9.3.2 프론트엔드: 에디터 포트 재계산

`web/src/pages/editor/EditorPage.tsx`에서 두 지점을 수정한다:

1. **노드 생성 시**: 기존 `getDefaultPorts(nodeType.type)` 호출을 `computePortsForNode(nodeType.type, initialConfig)`로 변경
2. **설정 변경 시**: 노드 config 변경 콜백에서 `computePortsForNode(nodeType, updatedConfig)`를 호출하여 `data.ports`를 업데이트. 삭제된 포트에 연결된 엣지는 `setEdges` 필터로 자동 제거

#### 9.3.3 백엔드: `NewNodeDef` 브릿지 방향 인식

`pkg/flow/node.go`에서 `NewNodeDef()` 또는 별도의 포트 생성 함수를 추가하여 브릿지 direction에 따라 적절한 `Inputs`/`Outputs` 포트를 생성한다:

- `BridgeIn`: `Inputs = []`, `Outputs = [Port{Name:"out"}]`
- `BridgeOut`: `Inputs = [Port{Name:"in"}]`, `Outputs = []`
- `BridgeInOut` / `BridgeRequestReply`: `Inputs = [Port{Name:"in"}]`, `Outputs = [Port{Name:"out"}]`

이 변경으로 저장된 플로우의 포트 정의가 정확해지며, `flowToReactFlowConfig()`에서 프론트엔드로 변환 시에도 올바른 포트가 전달된다.

### 9.4 검증 방법

- 브릿지 노드 생성 시 direction에 따라 올바른 포트만 표시되는지 확인
- 스위치 노드에 라우트 추가/삭제 시 출력 포트가 동적으로 변경되는지 확인
- 에디터에서 브릿지 direction 변경 시 포트가 즉시 재계산되는지 확인
- 삭제된 포트에 연결된 엣지가 자동 제거되는지 확인
- 기존 노드 타입(bridge/switch 외)의 포트가 변경 없이 유지되는지 확인
- 백엔드에서 브릿지 노드 저장/로드 시 올바른 포트 정의가 유지되는지 확인

---

## 10. M9: 에러 포트 타입 지원 (P0)

### 10.1 근본 원인 분석

- **증상**: 백엔드에서 `direction: "error"`로 전달하는 에러 포트가 프론트엔드 캔버스에 표시되지 않음
- **원인 1 (타입 제한)**: `PortDef.direction`과 `NodeTypeDefinition.ports[].direction`이 `'input' | 'output'`만 허용하여 `'error'`가 타입 레벨에서 차단됨
- **원인 2 (필터링 누락)**: `CustomNode.tsx`에서 `direction === 'input'`과 `direction === 'output'`만 필터링하여 에러 포트가 렌더링에서 완전히 누락됨
- **원인 3 (색상 미지원)**: `NodeHandle.tsx`에서 파란색(input)과 초록색(output)만 지원하여 에러 포트를 위한 빨간색 핸들이 없음

### 10.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/config/nodeSchemas.ts` | `PortDef.direction` 타입에 `'error'` 추가, `computePortsForNode`에서 에러 포트 포함 | 소 (5-10줄) |
| `web/src/types/node.ts` | `NodeTypeDefinition.ports[].direction` 타입에 `'error'` 추가 | 소 (1줄) |
| `web/src/components/flow/CustomNode.tsx` | 에러 포트 필터링 + 하단 배치 렌더링 로직 추가 | 소 (15-20줄) |
| `web/src/components/flow/NodeHandle.tsx` | 에러 포트 핸들 빨간색(`bg-red-500`) 색상 추가 | 소 (3-5줄) |

### 10.3 기술 접근

#### 10.3.1 타입 확장

`web/src/config/nodeSchemas.ts`의 `PortDef` 인터페이스와 `web/src/types/node.ts`의 `NodeTypeDefinition` 타입에서 `direction` 필드를 `'input' | 'output' | 'error'`로 확장한다. 이 변경은 타입 레벨에서 에러 포트를 허용하며, 기존 `'input'`과 `'output'` 사용에는 영향 없다.

#### 10.3.2 CustomNode 에러 포트 렌더링

`CustomNode.tsx`에서 기존 input/output 필터링에 에러 포트 그룹을 추가한다:

- `const errorPorts = ports.filter(p => p.direction === 'error')`
- 에러 포트는 노드 하단(Bottom position)에 React Flow의 `Position.Bottom` 핸들로 렌더링
- 에러 포트가 빈 배열이면 하단 핸들 영역을 렌더링하지 않음 (하위 호환)

#### 10.3.3 NodeHandle 색상 추가

`NodeHandle.tsx`의 색상 로직에 에러 포트를 위한 빨간색 추가:
- input(target): 파란색 `bg-blue-500`
- output(source): 초록색 `bg-green-500`
- error(source): 빨간색 `bg-red-500`

에러 포트 핸들의 React Flow `type`은 `source`로 설정 (에러 포트는 데이터 출력 방향).

#### 10.3.4 computePortsForNode 확장

`computePortsForNode()`에서 백엔드가 에러 포트를 전달하는 노드 타입의 경우 에러 포트를 반환 배열에 포함한다. 백엔드 `flow_adapter.go`에서 이미 에러 포트를 `direction: "error"`로 직렬화하므로, 프론트엔드는 백엔드 응답의 포트 데이터를 그대로 수용하면 된다.

### 10.4 검증 방법

- 에러 포트가 있는 노드(예: `WithErrorPort` 옵션이 적용된 노드)에서 빨간색 핸들이 노드 하단에 표시되는지 확인
- 에러 포트가 없는 기존 노드가 변경 없이 정상 렌더링되는지 확인
- 에러 포트에 엣지를 연결하고 플로우를 저장/로드 시 포트와 엣지가 유지되는지 확인
- TypeScript strict 모드에서 타입 에러가 발생하지 않는지 확인
- NodeHandle 색상이 input=파란, output=초록, error=빨강으로 올바르게 구분되는지 확인

---

## 11. M10: Handle ID 접두사 제거 (P0 - 리팩토링)

### 11.1 배경

Module 8/9에서 도입한 포트 시스템의 Handle ID에 방향 접두사(`in-`, `out-`, `err-`)를 사용하고 있었으나, 포트 direction이 별도 필드로 구분되므로 접두사가 불필요. 접두사가 프론트엔드/백엔드 간 불일치를 유발하여 와이어 연결 실패의 원인이 됨.

### 11.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/components/flow/CustomNode.tsx` | Handle id 속성을 port.name으로 변경 | 소 (3-5줄) |
| `internal/api/service/flow_adapter.go` | Edge 생성/역변환에서 접두사 추가/제거 로직 삭제 | 소 (10-15줄) |
| `web/src/components/property/PropertyPanel.tsx` | Port 타입에 'error' direction 추가 | 소 (3-5줄) |

### 11.3 기술 접근

- 설계 원칙: 포트 이름(name) = Handle ID = Wire Port. 중간 변환 없이 직접 매핑
- CustomNode: `id={port.name}` (이전: `id={\`in-${port.name}\`}`)
- flow_adapter.go: `sourceHandle: w.SourcePort` 직접 설정 (이전: `"out-" + w.SourcePort`)
- normalizeReactFlowDefinition: `source_port: sourceHandle` 직접 매핑 (이전: TrimPrefix)
- strings 패키지 import 제거

### 11.4 검증 방법

- Go 서버 재시작 후 API 응답의 sourceHandle/targetHandle에 접두사 없는지 확인
- React Flow 에디터에서 와이어 연결 정상 동작 확인
- TypeScript 타입 체크 통과

---

## 12. M11: 리스트 정렬 기능 (P1 - 신규 기능)

### 12.1 배경

FlowListPage와 AgentListPage에 정렬 기능이 없어 사용자가 원하는 항목을 찾기 어려움. 백엔드 `ListOptions.Sort` 필드와 프론트엔드 `ListOptions.sort` 타입이 이미 정의되어 있으나 실제 구현되지 않은 상태.

### 12.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `internal/api/service/sort_util.go` (신규) | parseSortParam 유틸리티 함수 | 소 (30-40줄) |
| `internal/api/service/sort_util_test.go` (신규) | parseSortParam 테스트 | 중 (60-80줄) |
| `internal/api/service/flow_adapter.go` | ListFlows에 sort 처리 추가 | 소 (10-15줄) |
| `internal/api/service/agent_adapter.go` | ListAgents에 sort 처리 추가 | 소 (10-15줄) |
| `web/src/components/common/SortableHeader.tsx` (신규) | 정렬 가능 헤더 공용 컴포넌트 | 중 (40-60줄) |
| `web/src/pages/flows/FlowListPage.tsx` | 4개 컬럼에 SortableHeader 적용 | 중 (30-40줄 추가/수정) |
| `web/src/pages/agents/AgentListPage.tsx` | 3개 컬럼에 SortableHeader 적용 | 중 (30-40줄 추가/수정) |
| `web/src/pages/flows/FlowDetailPanel.tsx` | 노드 인스턴스 3개 컬럼에 SortableHeader 적용 | 중 (20-30줄 추가/수정) |

### 12.3 기술 접근

- 정렬 파라미터 형식: `field:direction` (예: `name:asc`, `created_at:desc`)
- 기본 정렬: `name:asc`
- 백엔드: sort.Slice로 필터링 후, 페이지네이션 전에 정렬 적용
- 프론트엔드: SortableHeader 컴포넌트로 정렬 상태 관리 및 UI 표시
- FlowListPage/AgentListPage: 클라이언트 사이드 정렬 (useMemo)
- FlowDetailPanel: 클라이언트 사이드 정렬 (useMemo)
- 정렬 가능 필드:
  - FlowListPage: name, status, created_at, updated_at
  - AgentListPage: name, type, status
  - FlowDetailPanel: name, type, state

### 12.4 검증 방법

- parseSortParam 단위 테스트 7개 통과
- Go 전체 테스트 통과
- TypeScript 타입 체크 통과
- 각 리스트 페이지에서 헤더 클릭 시 정렬 동작 확인

---

## 13. M12: 대시보드 패널 재구성 (P1 - 리팩토링)

### 13.1 배경

대시보드의 2x2 위젯 그리드(SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget, ResourceWidget)를 3패널 구조(FlowPanel, AgentPanel, ResourcePanel)로 재구성한다. 플로우 관련 정보(상태 요약 + 리스트)를 하나의 패널로 통합하고, 에이전트도 동일한 구조로 통합하며, 시스템 리소스는 별도 패널로 유지한다.

### 13.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/pages/dashboard/panels/FlowPanel.tsx` (신규) | 플로우 상태 요약 + 리스트 테이블 + 정렬 + 액션 + 더 보기 링크 | 대 (200-250줄) |
| `web/src/pages/dashboard/panels/AgentPanel.tsx` (신규) | 에이전트 상태 요약 + 리스트 테이블 + 정렬 + 액션 + 더 보기 링크 | 대 (200-250줄) |
| `web/src/pages/dashboard/DashboardPage.tsx` | 2x2 그리드 → 3패널 반응형 레이아웃 | 중 (50-80줄 수정) |
| `web/src/pages/dashboard/widgets/SystemStatusWidget.tsx` | 삭제 | 삭제 |
| `web/src/pages/dashboard/widgets/RecentFlowsWidget.tsx` | 삭제 | 삭제 |
| `web/src/pages/dashboard/widgets/AgentStatusWidget.tsx` | 삭제 | 삭제 |
| `web/src/pages/dashboard/widgets/ResourceWidget.tsx` | 유지 (소폭 개선 가능) | 소 (0-20줄) |

### 13.3 기술 접근

#### 13.3.1 FlowPanel 구현

**상단: 플로우 상태 요약**
- `useFlows()` 훅 데이터에서 `status` 필드별 카운트 계산 (`useMemo`)
- 상태 카운트를 가로 배치: FlowStatusBadge + 건수 (예: `Running 3 | Stopped 2 | Error 1`)
- 기존 SystemStatusWidget에서 표시하던 정보를 그대로 포함

**하단: 플로우 리스트 테이블**
- 정렬 상태: `useState<{field: string, direction: 'asc'|'desc'}>({field: 'name', direction: 'asc'})`
- `useMemo`로 정렬된 플로우 목록 계산 → `.slice(0, 10)` 최대 10행 표시
- 컬럼: 이름(Link) | 상태(FlowStatusBadge) | 노드 수 | 동작 시간 | 액션(Play/Pause 버튼)
- SortableHeader 컴포넌트로 이름 컬럼 정렬 지원
- 이름 클릭: `<Link to={`/editor/${flow.id}`}>` 에디터 페이지 이동
- 동작 시간: `formatDistanceToNow(new Date(flow.updated_at))` 또는 유사 유틸
- 10건 초과 시 "더 보기 →" 링크 (`<Link to="/flows">`)

**플로우 액션 버튼**
- `running` → Pause 아이콘 + `flowService.stopFlow(id)` 호출
- `stopped`/`stored`/`loaded` → Play 아이콘 + `flowService.startFlow(id)` 호출
- `error` → RotateCcw 아이콘 + restart 로직
- `useMutation` (react-query)으로 상태 관리, 성공 시 쿼리 무효화, 실패 시 토스트

#### 13.3.2 AgentPanel 구현

**상단: 에이전트 상태 요약**
- 에이전트 데이터에서 상태별 카운트 계산: total, active(running), inactive(stopped + error)
- AgentStatusBadge + 건수 가로 배치
- 기존 AgentStatusWidget의 total/active/inactive 카운트를 그대로 포함

**하단: 에이전트 리스트 테이블**
- 동일한 정렬 상태 관리 패턴
- 컬럼: 이름 | 타입 | 상태(AgentStatusBadge) | 업타임 | 메시지 IN/OUT | 액션
- `detail=summary`로 호출하여 stats/uptime 포함 (M1 수정 활용)
- 업타임: `agent.uptime ?? '-'` 표시
- 메시지: `${agent.stats?.messages_in ?? '-'} / ${agent.stats?.messages_out ?? '-'}` 표시
- 10건 초과 시 "더 보기 →" 링크 (`<Link to="/agents">`)

#### 13.3.3 DashboardPage 레이아웃 변경

기존:
```
[SystemStatusWidget] [AgentStatusWidget]
[RecentFlowsWidget]  [ResourceWidget]
```

변경:
```
데스크톱 (md 이상):
[FlowPanel      ] [AgentPanel     ]
[ResourceWidget (전체 너비)        ]

모바일 (md 미만):
[FlowPanel                        ]
[AgentPanel                       ]
[ResourceWidget                   ]
```

Tailwind CSS 클래스:
- 상단 영역: `grid grid-cols-1 md:grid-cols-2 gap-4`
- ResourceWidget: `col-span-full` 또는 별도 행

#### 13.3.4 기존 위젯 삭제

- SystemStatusWidget.tsx: FlowPanel 상단 상태 요약으로 완전 대체
- RecentFlowsWidget.tsx: FlowPanel 하단 리스트 테이블로 완전 대체
- AgentStatusWidget.tsx: AgentPanel 상단 상태 요약으로 완전 대체
- DashboardPage에서 해당 import 및 사용처 제거

### 13.4 검증 방법

- 대시보드 페이지 로드 시 3패널 구조(FlowPanel, AgentPanel, ResourceWidget)가 표시되는지 확인
- FlowPanel 상단에 상태별 건수가 올바르게 표시되는지 확인 (기존 SystemStatusWidget 데이터와 일치)
- FlowPanel 리스트에서 이름 클릭 시 에디터 페이지로 이동하는지 확인
- FlowPanel 리스트에서 Start/Stop 버튼 클릭 시 API 호출 및 피드백 확인
- FlowPanel 이름 컬럼 헤더 클릭 시 정렬이 토글되는지 확인
- 11개 이상 플로우 존재 시 "더 보기" 링크가 표시되고 `/flows`로 이동하는지 확인
- AgentPanel이 FlowPanel과 동일한 패턴으로 동작하는지 확인
- ResourceWidget이 기존과 동일하게 CPU/메모리 게이지를 표시하는지 확인
- 데스크톱에서 FlowPanel과 AgentPanel이 나란히 배치되는지 확인
- 모바일에서 3패널이 세로 스택으로 배치되는지 확인
- 기존 SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget이 제공하던 모든 정보가 누락 없는지 확인

---

## 14. M13: Import/Export 기능 (P2 - 신규 기능)

### 14.1 배경

플로우와 에이전트를 JSON/YAML 파일로 내보내기(Export) 및 가져오기(Import) 기능을 추가한다. CLI(`xflowd flow import`/`xflowd agent import`)와 동일한 포맷을 사용하여 웹과 CLI 간 상호 호환성을 보장한다. 기존 에이전트 Export API는 구현되어 있으므로 플로우 Export API만 신규 추가하고, Import은 기존 Create API를 활용한다.

### 14.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `internal/api/handler/flow.go` | `Export()`, `ExportAll()` 핸들러 추가 + 라우트 등록 | 중 (60-80줄) |
| `web/src/lib/utils/download.ts` (신규) | `downloadJSON(data, filename)` Blob 다운로드 유틸 | 소 (15-20줄) |
| `web/src/lib/utils/importParser.ts` (신규) | `parseImportFile`, `validateFlowImport`, `validateAgentImport` | 중 (80-100줄) |
| `web/src/components/common/ImportDialog.tsx` (신규) | Import 공용 모달 (파일 선택, 드래그 앤 드롭, 미리보기, 이름 편집, 유효성 에러, 로딩) | 대 (250-300줄) |
| `web/src/services/api/flowService.ts` | `exportFlow(id)`, `exportAllFlows()` 함수 추가 | 소 (15-20줄) |
| `web/src/services/api/agentService.ts` | `exportAgent(id)`, `exportAllAgents()` 함수 추가 | 소 (15-20줄) |
| `web/src/pages/flows/FlowActionMenu.tsx` | "내보내기" 메뉴 항목 추가 | 소 (10-15줄) |
| `web/src/pages/flows/FlowListPage.tsx` | "가져오기"/"전체 내보내기" 툴바 버튼 + ImportDialog 연동 | 중 (30-40줄 추가) |
| `web/src/pages/agents/AgentListPage.tsx` | "가져오기"/"전체 내보내기" 툴바 버튼 + ImportDialog 연동 | 중 (30-40줄 추가) |
| `web/package.json` | `js-yaml` + `@types/js-yaml` 의존성 추가 | 소 (2줄) |

### 14.3 기술 접근

#### 14.3.1 백엔드: 플로우 Export API

`internal/api/handler/flow.go`에 두 개의 Export 핸들러를 추가한다:

- **`Export()` (GET /flows/:id/export)**: 개별 플로우 조회 후 런타임 필드(id, status, stats, created_at, updated_at)를 제거하고 `{ name, description?, definition }` 구조로 반환
- **`ExportAll()` (GET /flows/export)**: 전체 플로우를 조회하여 각각 Export 형식으로 변환한 배열을 반환

기존 에이전트 Export API(`GET /agents/{id}/export`, `GET /agents/export`)는 이미 구현되어 있으므로 수정 불필요.

#### 14.3.2 프론트엔드: downloadJSON 유틸리티

`web/src/lib/utils/download.ts`에 JSON 파일 다운로드 유틸 함수를 추가한다:

- `Blob(JSON.stringify(data, null, 2), { type: 'application/json' })` 생성
- `URL.createObjectURL(blob)` → `<a>` 동적 생성 → `click()` → `URL.revokeObjectURL()`
- UTF-8 BOM 미포함

#### 14.3.3 프론트엔드: importParser 유틸리티

`web/src/lib/utils/importParser.ts`에 파일 파싱 및 유효성 검사 함수를 추가한다:

- `parseImportFile(file)`: `FileReader.readAsText()` → 확장자 기반 JSON.parse / yaml.load 자동 감지
- `validateFlowImport(data)`: `name`과 `definition` 필수 필드 검증. 배열 입력 시 각 항목 개별 검증
- `validateAgentImport(data)`: `name`과 `type` 필수 필드 검증

#### 14.3.4 프론트엔드: ImportDialog 공용 모달

`web/src/components/common/ImportDialog.tsx`에 Import 공용 모달을 추가한다:

- **파일 선택**: `<input type="file" accept=".json,.yaml,.yml" />` 파일 선택기
- **드래그 앤 드롭**: `onDragOver`, `onDragLeave`, `onDrop` 이벤트 핸들러로 드래그 앤 드롭 영역
- **미리보기**: 파싱 성공 시 항목별 이름/타입/설명 + 편집 가능한 이름 입력 필드
- **배열 파일**: 리스트 형태로 각 항목 미리보기 + 개별 이름 편집
- **유효성 에러**: 빨간색 에러 메시지 + 확인 버튼 비활성화
- **로딩 상태**: Import 진행 중 스피너 + 버튼 비활성화
- **API 에러**: 에러 메시지 표시 + 대화상자 유지 (재시도 가능)
- **성공**: 토스트 알림 + 대화상자 닫기 + `onImportSuccess` 콜백

#### 14.3.5 서비스 함수 확장

- `flowService.ts`: `exportFlow(id)` (GET /flows/{id}/export), `exportAllFlows()` (GET /flows/export)
- `agentService.ts`: `exportAgent(id)` (GET /agents/{id}/export), `exportAllAgents()` (GET /agents/export)

#### 14.3.6 목록 페이지 툴바 및 메뉴 확장

- `FlowActionMenu.tsx`: "내보내기" 메뉴 항목 추가 (Download 아이콘)
- `FlowListPage.tsx`: "가져오기" 버튼(Upload 아이콘) + "전체 내보내기" 버튼(Download 아이콘) + ImportDialog 연동
- `AgentListPage.tsx`: 동일한 패턴으로 "가져오기"/"전체 내보내기" 버튼 + ImportDialog 연동

#### 14.3.7 이름 충돌 처리

- 자동 덮어쓰기 없음
- ImportDialog 미리보기에서 사용자가 이름을 편집한 후 확인
- API가 중복 이름 에러(409 Conflict)를 반환하면 ImportDialog에서 에러 표시 + 사용자가 이름 수정 후 재시도

### 14.4 검증 방법

- 백엔드: `curl http://localhost:8080/api/v1/flows/{id}/export`로 런타임 필드 제거 확인
- 백엔드: `curl http://localhost:8080/api/v1/flows/export`로 전체 플로우 배열 반환 확인
- 프론트엔드: FlowActionMenu에서 "내보내기" 클릭 시 JSON 파일 다운로드 확인
- 프론트엔드: FlowListPage "전체 내보내기" 클릭 시 flows.json 다운로드 확인
- 프론트엔드: AgentListPage "전체 내보내기" 클릭 시 agents.json 다운로드 확인
- 프론트엔드: "가져오기" 클릭 시 ImportDialog 모달이 열리는지 확인
- ImportDialog: JSON 파일 선택 시 미리보기가 올바르게 표시되는지 확인
- ImportDialog: YAML 파일 선택 시 파싱이 정상 동작하는지 확인
- ImportDialog: 필수 필드 누락 파일 선택 시 유효성 에러가 표시되는지 확인
- ImportDialog: 확인 버튼 클릭 시 Create API 호출 및 성공 토스트 확인
- ImportDialog: API 에러 시 에러 메시지 표시 및 재시도 가능 확인
- CLI 호환: CLI로 내보낸 파일을 웹 ImportDialog에서 가져오기 성공 확인
- CLI 호환: 웹에서 내보낸 파일이 CLI import 형식과 일치하는지 확인

---

## 15. M15: 화면 테마 시스템 (P1)

### 15.1 개요

현재 `dark:` Tailwind variant 클래스 기반 라이트/다크 테마 시스템을 CSS Variable 디자인 토큰 기반으로 전환한다. System/Day/Night/Custom 4가지 테마 모드를 지원하며, 사용자 정의 색상 편집 기능을 제공한다.

### 15.2 서브 모듈별 구현 계획

#### M15-1: CSS Variable 디자인 토큰 시스템

**구현 내용**:
- `index.css`의 `@theme` 블록에 시맨틱 컬러 토큰 정의 (배경, 텍스트, 테두리, 상태, 인터랙션 카테고리)
- `:root` 셀렉터에 Day 프리셋 기본값 정의
- `[data-theme="night"]` 셀렉터에 Night 프리셋 값 정의
- `[data-theme="custom"]` 셀렉터 준비 (M15-3에서 인라인 스타일로 적용)
- `web/src/lib/theme/tokens.ts` 신규 생성: 토큰 카테고리, 이름, 기본값 상수 정의

**수정 파일**:
| 파일 | 변경 유형 |
|------|----------|
| `web/src/index.css` | 수정 |
| `web/src/lib/theme/tokens.ts` | 신규 |

**검증**: 브라우저 DevTools에서 `:root` CSS Variables가 올바르게 정의되고 Tailwind `var()` 참조가 동작하는지 확인

#### M15-2: Day/Night 테마 프리셋

**구현 내용**:
- Day 프리셋: 현재 라이트 모드 Tailwind 색상 → CSS Variable 값 매핑 (gray-50, gray-900, blue-500 등)
- Night 프리셋: 현재 `dark:` 클래스 색상 → CSS Variable 값 매핑 (gray-900, gray-100, blue-400 등)
- `[data-theme="day"]`와 `[data-theme="night"]` CSS 셀렉터에 프리셋 값 정의
- 기존 `dark:` 클래스와의 시각적 동등성 보장

**수정 파일**:
| 파일 | 변경 유형 |
|------|----------|
| `web/src/index.css` | 수정 (M15-1과 함께) |
| `web/src/lib/theme/tokens.ts` | 수정 (프리셋 데이터 추가) |

**검증**: `data-theme="day"` 적용 시 현재 라이트 모드와 동일, `data-theme="night"` 적용 시 현재 다크 모드와 동일한 시각적 결과 확인

#### M15-3: Custom 테마 지원

**구현 내용**:
- `uiStore.ts`에 `customThemeTokens: Record<string, string>` 상태 추가
- `setCustomThemeTokens`, `resetCustomThemeTokens` 액션 추가
- `partialize` 함수에 `customThemeTokens` 추가하여 localStorage 영속화
- `theme` 타입을 `'system' | 'day' | 'night' | 'custom'`으로 변경
- Zustand `persist` 미들웨어에 `migrate` 함수 추가: `'light'` → `'day'`, `'dark'` → `'night'` 자동 변환
- `useTheme` 훅에서 Custom 테마 시 `customThemeTokens`를 `<html>` 인라인 CSS Variable로 적용

**수정 파일**:
| 파일 | 변경 유형 |
|------|----------|
| `web/src/stores/uiStore.ts` | 수정 |
| `web/src/hooks/useTheme.ts` | 수정 |

**검증**: Custom 테마 선택 시 저장된 토큰 값이 인라인 스타일로 적용되고, 새로고침 후에도 유지되는지 확인

#### M15-4: 테마 선택 UI

**구현 내용**:
- Header 컴포넌트의 기존 3-cycle 토글 버튼을 4옵션 드롭다운/팝오버로 교체
- `ThemeSelector.tsx` 신규 컴포넌트: System(Monitor), Day(Sun), Night(Moon), Custom(Palette) 옵션
- 현재 활성 테마에 체크/하이라이트 표시
- 외부 클릭 시 드롭다운 닫기, ESC 키로 닫기
- Custom 옵션에 편집 아이콘/버튼 표시 (테마 에디터 열기)

**수정 파일**:
| 파일 | 변경 유형 |
|------|----------|
| `web/src/components/theme/ThemeSelector.tsx` | 신규 |
| `web/src/components/layout/Header.tsx` | 수정 |

**검증**: Header에서 테마 드롭다운이 4옵션을 표시하고, 선택 시 즉시 테마가 전환되며, 아이콘이 올바르게 변경되는지 확인

#### M15-5: 커스텀 테마 에디터

**구현 내용**:
- `ThemeEditorModal.tsx` 신규: 모달 다이얼로그로 구현
- 카테고리별 시맨틱 토큰 그룹 표시 (배경, 텍스트, 테두리, 상태, 인터랙션)
- `ColorTokenInput.tsx` 신규: 네이티브 `<input type="color">` + hex 텍스트 입력 컴포넌트
- 라이브 프리뷰: 편집 중 `<html>` 인라인 스타일에 실시간 반영
- 저장/취소/초기화 버튼: 저장 시 `setCustomThemeTokens()`, 취소 시 이전 상태 복원, 초기화 시 Day 프리셋 값으로 리셋

**수정 파일**:
| 파일 | 변경 유형 |
|------|----------|
| `web/src/components/theme/ThemeEditorModal.tsx` | 신규 |
| `web/src/components/theme/ColorTokenInput.tsx` | 신규 |

**검증**: 모달에서 색상 변경 시 실시간 화면 반영 확인, 저장 후 새로고침 시 색상 유지 확인, 취소 시 원래 색상 복원 확인

#### M15-6: CSS 마이그레이션

**구현 내용**:
- 약 885개 `dark:` Tailwind variant 클래스를 CSS Variable 기반 시맨틱 토큰으로 점진적 교체
- 우선순위별 마이그레이션: 전역 레이아웃 → 공통 컴포넌트 → 대시보드/패널 → 리스트/테이블 → 에디터/모니터링 → 폼/모달
- 각 파일에서 `bg-white dark:bg-gray-800` → `bg-[var(--color-bg-surface)]` 패턴으로 교체
- 호환 전략: 마이그레이션 과도기에 `dark:` 클래스와 CSS Variable 공존 허용, CSS Variable 우선 적용

**수정 파일**:
| 파일 범위 | 변경 유형 |
|----------|----------|
| 전체 컴포넌트 (~30-40 파일) | 수정 (`dark:` 클래스 → CSS Variable 교체) |

**검증**: Day/Night 테마에서 마이그레이션 전/후 시각적 동등성 확인, TypeScript/Tailwind 빌드 에러 없음 확인

### 15.3 구현 순서

1. **M15-1 + M15-2 병렬 진행**: CSS Variable 토큰 정의와 Day/Night 프리셋은 `index.css`와 `tokens.ts`를 함께 수정하므로 동시 진행
2. **M15-3 진행**: M15-1/M15-2 완료 후 스토어 변경 및 useTheme 훅 수정
3. **M15-4 진행**: M15-3 완료 후 테마 선택 UI 구현 (새 theme 타입 필요)
4. **M15-5 진행**: M15-3 완료 후 커스텀 테마 에디터 구현 (M15-4와 병렬 가능)
5. **M15-6 진행**: M15-1~M15-5 및 기존 Module 1-13 모두 완료 후 점진적 CSS 마이그레이션

### 15.4 검증 방법

- `data-theme` 속성 전환 시 CSS Variable 값이 올바르게 변경되는지 DevTools 확인
- Day 테마 = 현재 라이트 모드, Night 테마 = 현재 다크 모드와 시각적 동등성 확인
- Custom 테마 토큰 저장/로드/리셋 동작 확인
- System 테마에서 OS 다크 모드 전환 시 자동 Day↔Night 전환 확인
- 브라우저 새로고침 후 테마 유지 확인
- FOUC(Flash of Unstyled Content) 없이 테마 전환이 즉시 적용되는지 확인
- TypeScript 컴파일 에러 0건, Vite 프로덕션 빌드 성공 확인

---

## 16. M16: 대시보드 커스터마이징 시스템 (P1)

### 16.1 개요

M12에서 구현한 3패널 대시보드(FlowPanel, AgentPanel, ResourceWidget)를 멀티 대시보드 + 동적 패널 시스템으로 확장한다. 사용자가 여러 대시보드 페이지를 생성/관리하고, 각 페이지에 원하는 패널을 추가/삭제할 수 있도록 한다. 기존 패널(flows, agents, resource) 외에 디바이스와 로그 패널 타입을 추가한다.

### 16.2 서브 모듈별 구현 계획

#### M16-1: 멀티 대시보드 데이터 모델

**수정 대상 파일**:

| 파일 | 변경 유형 | 변경 내용 |
|------|----------|----------|
| `web/src/stores/uiStore.ts` | 수정 | DashboardPageConfig, PanelConfig 타입 추가, dashboardPages 상태, CRUD 액션, persist migrate v2 |

**구현 내용**:

- `DashboardPageConfig` interface: `id`, `name`, `isDefault`, `panels`, `layout`
- `PanelConfig` interface: `id`, `type` (`PanelType`), `title`, `config`
- `PanelType = 'flows' | 'agents' | 'resource' | 'devices' | 'logs'`
- 기존 상태를 `DashboardPageConfig[]`로 통합
- persist version 1->2 마이그레이션: 기존 `dashboardLayout` + panel 설정을 기본 페이지로 변환
- `activeDashboardId`: 현재 표시 중인 페이지 ID
- 액션: `addDashboardPage`, `removeDashboardPage`, `updateDashboardPage`, `setDefaultDashboardPage`, `setActiveDashboard`
- 패널 액션: `addPanel(pageId, type)`, `removePanel(pageId, panelId)`, `updatePanelConfig(pageId, panelId, config)`
- 불변 조건: `isDefault`는 정확히 1개, 최소 1페이지 유지

**검증**: persist version 1->2 마이그레이션이 기존 사용자 데이터를 올바르게 변환하는지 확인. CRUD 액션이 불변 조건을 위반하지 않는지 확인

#### M16-2: 대시보드 관리 UI

**수정 대상 파일**:

| 파일 | 변경 유형 | 변경 내용 |
|------|----------|----------|
| `web/src/pages/dashboard/DashboardToolbar.tsx` | 신규 | 대시보드 선택/추가/삭제/기본지정 툴바 |
| `web/src/pages/dashboard/CreateDashboardDialog.tsx` | 신규 | 새 대시보드 이름 입력 다이얼로그 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | DashboardToolbar 통합, 멀티 페이지 렌더링 |

**구현 내용**:

- `DashboardToolbar`: 드롭다운(페이지 목록, 기본 페이지는 별 표시) + 3 버튼(추가/삭제/기본지정)
- `CreateDashboardDialog`: 이름 입력 + 확인/취소
- 삭제: 확인 모달, 기본 페이지 삭제 시 경고 ("기본 페이지는 삭제할 수 없습니다" 또는 다른 페이지를 기본으로 전환 후 삭제)
- 마지막 페이지 삭제 방지
- 대시보드 이름 인라인 편집 (더블클릭 -> input)
- `DashboardPage`에서 `activeDashboardId` 기반 panels/layout 렌더링

**검증**: 대시보드 추가/삭제/기본지정/이름편집 동작 확인. 기본 페이지 삭제 방지 및 마지막 페이지 삭제 방지 확인

#### M16-3: 패널 추가/삭제 시스템

**수정 대상 파일**:

| 파일 | 변경 유형 | 변경 내용 |
|------|----------|----------|
| `web/src/pages/dashboard/AddPanelDialog.tsx` | 신규 | 패널 타입 선택 다이얼로그 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | 동적 패널 렌더링, 패널 추가/삭제 통합 |

**구현 내용**:

- `AddPanelDialog`: 5종 패널을 카드/리스트로 표시 (아이콘 + 이름 + 설명)
  - flows: BarChart3 아이콘, "플로우 현황", "플로우 상태 요약 및 목록"
  - agents: Users 아이콘, "에이전트 현황", "에이전트 상태 및 통계"
  - resource: Activity 아이콘, "프로세스 리소스", "CPU, 메모리, 처리량 차트"
  - devices: Cpu 아이콘, "디바이스", "디바이스 상태 목록"
  - logs: FileText 아이콘, "로그", "실시간 로그 스트림"
- 패널 추가 시 자동 배치: 빈 공간 탐색 -> 없으면 최하단에 w=6, h=4로 추가
- 편집 모드에서 각 패널 우상단 X 버튼
- `DashboardPage`: panels 배열 기반 동적 렌더링 (panelType -> component 매핑)

**검증**: 5종 패널 추가 다이얼로그 동작 확인. 자동 배치 로직 확인. 패널 삭제 후 레이아웃 유지 확인

#### M16-4: 디바이스/로그 패널 타입

**수정 대상 파일**:

| 파일 | 변경 유형 | 변경 내용 |
|------|----------|----------|
| `web/src/pages/dashboard/panels/DevicePanel.tsx` | 신규 | 디바이스 상태 요약 + 리스트 |
| `web/src/pages/dashboard/panels/LogPanel.tsx` | 신규 | 실시간 로그 스트림 패널 |

**구현 내용**:

- `DevicePanel`: `useDevicesRealtime()` 훅으로 데이터. 상단 상태 카드(전체/online/offline) + 테이블(name, type, status, agent, last_seen)
- `LogPanel`: `useWebSocket`으로 LOG_ENTRY 수신. 소스 필터 드롭다운 + 레벨 필터. 최대 100줄 버퍼 (설정 가능). 자동 스크롤
- 두 패널 모두 PanelSettingsDropdown 적용 (제목 편집 + 패널별 설정)
- 기존 FlowPanel, AgentPanel, ResourceWidget은 코드 변경 최소화하여 재사용

**검증**: DevicePanel에서 디바이스 상태가 실시간 갱신되는지 확인. LogPanel에서 소스/레벨 필터가 동작하는지 확인. 자동 스크롤 및 버퍼 제한 동작 확인

### 16.3 구현 순서

1. **M16-1 즉시 진행**: uiStore.ts 데이터 모델 확장 + persist 마이그레이션. M12 대시보드 구조 기반
2. **M16-2 + M16-3 + M16-4 병렬 진행**: M16-1 완료 후 병렬 진행 가능. 파일 충돌 없음 (각각 별도 컴포넌트)
   - M16-2: DashboardToolbar.tsx + CreateDashboardDialog.tsx + DashboardPage.tsx 수정
   - M16-3: AddPanelDialog.tsx + DashboardPage.tsx 수정 (M16-2와 DashboardPage.tsx 공유하나 수정 영역 분리 가능)
   - M16-4: DevicePanel.tsx + LogPanel.tsx 신규 (독립적)

### 16.4 검증 방법

- 기존 사용자의 대시보드 레이아웃이 persist 마이그레이션 후 기본 페이지로 올바르게 변환되는지 확인
- 멀티 대시보드 생성/삭제/전환/기본지정이 정상 동작하는지 확인
- 5종 패널 타입 추가/삭제가 정상 동작하는지 확인
- DevicePanel/LogPanel이 실시간 데이터를 올바르게 표시하는지 확인
- 브라우저 새로고침 후 대시보드 설정(페이지, 패널, 레이아웃)이 유지되는지 확인
- TypeScript 컴파일 에러 0건, Vite 프로덕션 빌드 성공 확인

---

## 17. 의존성 그래프

```
BF (백엔드 버그 수정) ──── 독립 (즉시 실행 가능)

M1 (에이전트 통계 버그) ──── 독립 (즉시 실행 가능)

M2 (백엔드 API) ──── 독립 (M1과 병렬 실행 가능)
  │
  ├── M3 (에이전트 로그 레벨 UI) ──── M2 완료 후 실행
  │
  └── M4 (노드 통계 + 로그 레벨 UI) ──── M2 완료 후 실행 (M3와 병렬 가능)
       │
       └── M5 (캔버스 런타임 통계) ──── M4 완료 후 실행

M7 (로그 뷰어 컴포넌트/소스 필터) ──── 독립 (SPEC-OBS-004 완료 전제, 다른 모듈과 병렬 가능)

M8 (동적 포트 시스템) ──── 독립 (즉시 실행 가능, 다른 모듈과 병렬 가능)
  │
  ├── 프론트엔드: nodeSchemas.ts + EditorPage.tsx (파일 충돌 주의: M5와 EditorPage.tsx 공유)
  │
  └── 백엔드: node.go (BF와 bridge.go 인접 파일이나 직접 충돌 없음)

M9 (에러 포트 타입 지원) ──── M8 완료 후 실행 (nodeSchemas.ts, CustomNode.tsx 공유)
  │
  ├── nodeSchemas.ts: PortDef 타입 확장 (M8의 computePortsForNode 위에 타입 변경)
  ├── node.ts: NodeTypeDefinition 타입 확장
  ├── CustomNode.tsx: 에러 포트 필터링 + 하단 렌더링 (M8의 포트 렌더링 기반)
  └── NodeHandle.tsx: 에러 포트 색상 추가

M10 (Handle ID 접두사 제거) ──── M8/M9 완료 후 실행 (리팩토링)
  │
  ├── CustomNode.tsx: Handle id를 port.name으로 변경 (M9의 에러 포트 렌더링 기반)
  ├── flow_adapter.go: 접두사 추가/제거 로직 삭제
  └── PropertyPanel.tsx: Port 타입에 'error' direction 추가

M11 (리스트 정렬 기능) ──── 독립 (즉시 실행 가능, 다른 모듈과 병렬 가능)
  │
  ├── 백엔드: sort_util.go (신규), flow_adapter.go, agent_adapter.go
  └── 프론트엔드: SortableHeader.tsx (신규), FlowListPage.tsx, AgentListPage.tsx, FlowDetailPanel.tsx

M12 (대시보드 패널 재구성) ──── M1, M11 완료 권장
  │
  ├── FlowPanel.tsx (신규): useFlows + SortableHeader + FlowStatusBadge + 액션 버튼
  ├── AgentPanel.tsx (신규): useAgents + SortableHeader + AgentStatusBadge + 액션 버튼
  ├── DashboardPage.tsx: 2x2 그리드 → 3패널 반응형 레이아웃 변경
  ├── SystemStatusWidget.tsx: 삭제 (FlowPanel으로 대체)
  ├── RecentFlowsWidget.tsx: 삭제 (FlowPanel으로 대체)
  └── AgentStatusWidget.tsx: 삭제 (AgentPanel으로 대체)

M13 (Import/Export 기능) ──── 독립 (즉시 실행 가능, 다른 모듈과 병렬 가능)
  │
  ├── 백엔드: flow.go Export/ExportAll 핸들러 추가 (기존 에이전트 Export 재사용)
  ├── 프론트엔드: download.ts (신규), importParser.ts (신규), ImportDialog.tsx (신규)
  ├── 프론트엔드: flowService.ts + agentService.ts Export 함수 추가
  ├── 프론트엔드: FlowActionMenu.tsx 내보내기 항목 추가
  ├── 프론트엔드: FlowListPage.tsx + AgentListPage.tsx 툴바 버튼 추가
  └── 의존성: package.json에 js-yaml 추가

M15 (화면 테마 시스템) ──── 독립 (프론트엔드 전용, 백엔드 수정 없음)
  │
  ├── M15-1 + M15-2 (CSS Variable 토큰 + Day/Night 프리셋) ──── 독립 (즉시 실행 가능)
  │     │
  │     ├── index.css: @theme 블록 시맨틱 토큰 정의 + :root/[data-theme] 프리셋
  │     └── tokens.ts (신규): 토큰 카테고리, 이름, Day/Night 프리셋 상수
  │
  ├── M15-3 (Custom 테마 지원) ──── M15-1/M15-2 완료 후 실행
  │     │
  │     ├── uiStore.ts: theme 타입 변경 + customThemeTokens 상태/액션 추가
  │     └── useTheme.ts: resolvedTheme 타입 변경 + data-theme 속성 + 커스텀 토큰 인라인 적용
  │
  ├── M15-4 (테마 선택 UI) ──── M15-3 완료 후 실행
  │     │
  │     ├── ThemeSelector.tsx (신규): 4옵션 드롭다운/팝오버
  │     └── Header.tsx: 3-cycle 토글 → ThemeSelector 교체
  │
  ├── M15-5 (커스텀 테마 에디터) ──── M15-3 완료 후 실행 (M15-4와 병렬 가능)
  │     │
  │     ├── ThemeEditorModal.tsx (신규): 모달 + 카테고리별 컬러 피커
  │     └── ColorTokenInput.tsx (신규): 개별 토큰 컬러 피커 + hex 입력
  │
  └── M15-6 (CSS 마이그레이션) ──── M15-1~M15-5 + M1-M13 모두 완료 후 실행
        │
        └── 전체 컴포넌트 (~30-40 파일): dark: 클래스 → CSS Variable 교체

M16 (대시보드 커스터마이징) ──── M12 완료 후 실행 (대시보드 기반)
  ├── M16-1 (데이터 모델) ──── 독립 (즉시 실행 가능)
  ├── M16-2 (관리 UI) ──── M16-1 완료 후 실행
  ├── M16-3 (패널 추가/삭제) ──── M16-1 완료 후 실행 (M16-2와 병렬 가능)
  └── M16-4 (디바이스·로그 패널) ──── M16-1 완료 후 실행 (M16-2/M16-3과 병렬 가능)
```

### 실행 순서

1. **BF + M1 + M2 + M8 + M11 병렬 진행**: BF는 백엔드 버그 수정, M1은 프론트엔드+백엔드, M2는 백엔드 monitor 핸들러, M8은 동적 포트 시스템 (nodeSchemas.ts + EditorPage.tsx + node.go), M11은 리스트 정렬 기능 (sort_util.go + SortableHeader.tsx + 리스트 페이지)
2. **M3 + M4 병렬 진행**: M2의 API가 완성된 후 에이전트 UI(M3)와 노드 UI(M4) 병렬 구현 가능
3. **M9 → M10 → M5 순차 진행**: M8 완료 후 M9를 먼저 진행(nodeSchemas.ts 타입 확장, CustomNode/NodeHandle 에러 포트 렌더링). M9 완료 후 M10을 진행(Handle ID 접두사 제거, CustomNode.tsx/flow_adapter.go 리팩토링). M10 완료 후 M5를 진행(EditorPage.tsx 런타임 통계)
4. **M7 독립 진행**: SPEC-OBS-004 백엔드 구현이 완료된 상태이므로 즉시 실행 가능. M1-M5, M8-M10과 파일 충돌 없음 (LogViewer.tsx, MonitoringPage.tsx만 수정)
5. **M12 진행**: M1(에이전트 통계 버그 수정)과 M11(SortableHeader 컴포넌트) 완료 후 진행. DashboardPage.tsx와 panels/ 디렉토리만 수정하므로 다른 모듈과 파일 충돌 없음
6. **M13 독립 진행**: 백엔드 플로우 Export API(flow.go)와 프론트엔드 유틸/모달/서비스 수정. FlowListPage.tsx와 AgentListPage.tsx를 M11과 공유하므로 M11 완료 후 진행을 권장하나, 수정 영역(툴바 버튼 vs 정렬 헤더)이 분리되어 있어 병렬도 가능
7. **M15 진행** (M15-1+M15-2 → M15-3 → M15-4+M15-5 → M15-6):
   - M15-1/M15-2: CSS Variable 토큰 정의 + Day/Night 프리셋 (index.css, tokens.ts). 다른 모듈과 파일 충돌 없으므로 M1-M13과 병렬 진행 가능
   - M15-3: Zustand 스토어 + useTheme 훅 변경. uiStore.ts의 theme 타입 변경이므로 기존 모듈 구현 완료 후 진행 권장
   - M15-4/M15-5: 테마 선택 UI + 에디터 모달. 병렬 진행 가능
   - M15-6: CSS 마이그레이션. M1-M13 및 M15-1~M15-5 모두 완료 후 진행 필수 (기존 컴포넌트의 dark: 클래스 대상)
8. **M16 진행** (M16-1 -> M16-2 + M16-3 + M16-4 병렬):
   - M16-1: uiStore.ts 데이터 모델 확장 + persist 마이그레이션. M12 대시보드 구조 기반
   - M16-2/M16-3/M16-4: M16-1 완료 후 병렬 진행 가능. 파일 충돌 없음 (각각 별도 컴포넌트)

**주의**: M8, M9, M10, M5가 `CustomNode.tsx`를 공유하므로, M8 -> M9 -> M10 -> M5 순서로 진행하는 것을 권장한다. M11은 독립적이므로 아무 시점에서나 병렬 실행 가능하나, FlowListPage.tsx/FlowDetailPanel.tsx를 M4와 공유하므로 M4 완료 후 진행을 권장한다. M12는 M11의 SortableHeader를 재사용하고 M1의 에이전트 detail=summary 기능을 활용하므로 두 모듈 완료 후 진행을 권장한다. M13은 독립적이나 FlowListPage.tsx/AgentListPage.tsx를 M11과 공유하므로 M11 완료 후 진행을 권장한다. M15는 프론트엔드 전용이며 uiStore.ts/Header.tsx/index.css를 공유하므로, 기존 모듈 구현 완료 후 M15-3 이후를 진행하는 것을 권장한다. M15-6(CSS 마이그레이션)은 반드시 모든 기존 모듈 완료 후 진행해야 한다.

---

## 18. 리스크 분석

| 리스크 | 심각도 | 발생 확률 | 완화 방안 |
|--------|--------|----------|----------|
| `detail=summary`가 대량 에이전트에서 성능 저하 유발 | 중 | 낮 | 에이전트 수가 적은 IoT 환경이므로 영향 미미. 필요 시 페이지네이션 활용 |
| `LevelManager`에 `ListLevels` 미구현 또는 상이한 시그니처 | 중 | 낮 | 구현 전 인터페이스 확인 완료. 테스트 코드에서 `ListLevels` 사용 확인됨 |
| 컴포넌트 ID 형식 불일치 (프론트엔드 vs 백엔드) | 높 | 중 | 에이전트 ID 기반 `agent.{id}` 형식으로 통일. 구현 시 백엔드 컴포넌트 등록 패턴 확인 필요 |
| 기존 글로벌 로그 레벨 API 하위 호환성 깨짐 | 높 | 낮 | GET 응답 확장 방식으로 기존 필드 유지. 기존 PUT 동작 변경 없음 |
| M7: 10,000건 로그에서 3중 필터 성능 저하 | 중 | 낮 | useMemo 캐싱 + 단순 순회 방식으로 16ms 이내 보장. Set.has()와 includes()는 O(1)/O(n) 복잡도 |
| M7: 기존 LogEntry 인터페이스 변경으로 인한 타입 호환성 | 낮 | 낮 | component/source를 optional 필드로 추가하여 기존 코드 영향 없음 |
| M7: 행 너비 초과로 인한 레이아웃 깨짐 | 중 | 중 | source 배지(55px) + component(140px) 추가로 총 385px 고정 너비 사용. message 영역이 truncate 처리되므로 문제 없음. 모바일 반응형은 OUT OF SCOPE |
| M8: 기존 엣지가 포트 변경으로 끊어짐 | 높 | 중 | 포트 재계산 시 삭제된 포트에 연결된 엣지를 자동 제거하고, 사용자에게 시각적 피드백 제공. 엣지 정리 로직을 EditorPage에서 일괄 처리 |
| M8: EditorPage.tsx M5와 파일 충돌 | 중 | 높 | M8을 M5보다 먼저 완료하여 충돌 방지. 두 모듈이 다루는 영역(포트 계산 vs 런타임 통계)이 논리적으로 분리되어 있어 병합은 가능 |
| M8: 스위치 라우트 config 형식 불일치 | 중 | 중 | 구현 전 실제 스위치 노드 config 형식을 백엔드 코드에서 확인하여 `routes` 배열 구조 검증 필요 |
| M8: 백엔드 NewNodeDef 변경으로 기존 플로우 호환성 | 높 | 낮 | 기존 저장된 플로우의 포트 정의는 변경하지 않음. 새로 생성/수정하는 노드만 영향. flowToReactFlowConfig 변환 시 백엔드 포트 데이터 우선 사용 |
| M9: 에러 포트 하단 배치가 기존 레이아웃에 영향 | 중 | 낮 | 에러 포트가 없는 노드는 하단 핸들 영역을 렌더링하지 않으므로 기존 레이아웃에 영향 없음. 에러 포트가 있는 노드만 하단에 추가 핸들 표시 |
| M9: nodeSchemas.ts 타입 변경이 M8 computePortsForNode에 영향 | 중 | 낮 | 타입 확장(union에 'error' 추가)은 기존 'input'/'output' 사용에 영향 없음. M8 완료 후 M9를 진행하여 충돌 방지 |
| M9: React Flow Position.Bottom 핸들과 기존 엣지 라우팅 호환 | 낮 | 낮 | React Flow는 Top/Right/Bottom/Left 4방향 핸들을 기본 지원. Bottom 핸들의 엣지 라우팅은 자동 처리됨 |
| M10: 접두사 제거 후 기존 저장된 플로우의 와이어 연결 깨짐 | 높 | 중 | 백엔드 flow_adapter.go에서 접두사 로직을 동시에 제거하여 프론트엔드/백엔드 일관성 유지. 기존 플로우는 서버 재시작 시 새 형식으로 반환됨 |
| M10: PropertyPanel의 error direction 추가 누락 | 중 | 낮 | M9에서 타입을 확장했으나 PropertyPanel에서 별도 처리가 필요. M10에서 함께 수정하여 누락 방지 |
| M11: FlowListPage/FlowDetailPanel이 M4와 파일 충돌 | 중 | 높 | M4 완료 후 M11을 진행하여 충돌 방지. 두 모듈이 다루는 영역(노드 인스턴스 vs 정렬 헤더)이 논리적으로 분리되어 있어 병합은 가능 |
| M11: 클라이언트 사이드 정렬의 대규모 데이터 성능 | 낮 | 낮 | IoT 환경에서 플로우/에이전트 수가 적어 클라이언트 정렬로 충분. 향후 서버 사이드 정렬로 전환 가능 |
| M12: 기존 위젯 삭제 시 참조 누락 | 중 | 중 | SystemStatusWidget, RecentFlowsWidget, AgentStatusWidget 삭제 전 import 참조를 모두 제거. TypeScript 컴파일 에러로 누락 즉시 감지 |
| M12: FlowPanel/AgentPanel에서 기존 위젯 정보 누락 | 높 | 낮 | 기존 3개 위젯이 표시하던 모든 데이터를 FlowPanel/AgentPanel에 매핑 완료 확인. acceptance.md의 데이터 무결성 시나리오로 검증 |
| M12: Start/Stop 액션 버튼의 상태 불일치 | 중 | 중 | useMutation 성공 시 쿼리 무효화로 즉시 갱신. 실패 시 토스트로 피드백. 낙관적 업데이트(optimistic update) 대신 서버 상태 기반 갱신으로 일관성 유지 |
| M12: 대시보드 초기 로드 시 다수 API 호출로 성능 저하 | 낮 | 낮 | useFlows, useAgents, monitor/metrics 3개 쿼리가 기존 4개 위젯에서도 동일하게 호출하던 패턴. 오히려 위젯 수 감소로 리렌더링 횟수 감소 기대 |
| M12: 반응형 레이아웃 전환 시 패널 높이 불균형 | 낮 | 중 | 데스크톱 2열 배치 시 FlowPanel과 AgentPanel의 데이터 수 차이로 높이 불균형 가능. 최대 10행 제한과 "더 보기" 링크로 높이 차이 최소화 |
| M13: YAML 파싱 라이브러리(js-yaml)의 보안 취약점 | 중 | 낮 | js-yaml은 `safeLoad`(v3) / `load`(v4)로 안전한 파싱 수행. 신뢰할 수 없는 YAML의 `!!js/function` 등 위험 태그는 기본 차단됨. 최신 안정 버전 사용 |
| M13: 대용량 파일 Import 시 브라우저 메모리 부족 | 중 | 낮 | IoT 환경에서 플로우/에이전트 수가 적어 파일 크기가 작음. FileReader API 기반 텍스트 파싱으로 스트리밍 불필요. 향후 필요 시 파일 크기 제한(예: 10MB) 추가 가능 |
| M13: Export 포맷과 CLI `xflowd flow import` 포맷 불일치 | 높 | 낮 | CLI 코드의 Import 구조체를 참조하여 동일한 필드 구조(`name`, `definition`, `description?`) 사용. 구현 전 CLI Import 테스트로 호환성 검증 |
| M13: ImportDialog에서 이름 충돌 시 사용자 혼란 | 중 | 중 | 409 Conflict 에러를 한국어 메시지로 변환하여 표시("동일한 이름이 이미 존재합니다"). 이름 편집 필드를 하이라이트하여 수정 유도 |
| M13: FlowListPage/AgentListPage 툴바 영역이 M11 정렬 헤더와 겹침 | 낮 | 낮 | M13 툴바 버튼은 테이블 상단 영역, M11 정렬 헤더는 테이블 헤더 행에 위치하여 물리적 영역이 분리됨. 병렬 진행 시에도 충돌 없음 |
| M15: Tailwind CSS v4 @theme 블록에서 CSS Variable 참조 호환성 | 중 | 중 | Tailwind v4의 @theme 블록은 CSS Variable 정의를 지원하나, `var()` 참조 방식이 v3과 다를 수 있음. 구현 전 Tailwind v4 공식 문서에서 @theme 블록 내 변수 참조 패턴 확인 필요 |
| M15: uiStore theme 타입 변경으로 기존 코드 호환성 깨짐 | 높 | 중 | `'light'` → `'day'`, `'dark'` → `'night'`로 타입 변경 시 기존 코드에서 `'light'`/`'dark'` 리터럴 비교가 실패. Zustand migrate 함수로 저장된 값 자동 변환 + TypeScript 컴파일 에러로 코드 누락 즉시 감지 |
| M15: 885개 dark: 클래스 마이그레이션 시 시각적 회귀 | 높 | 중 | 점진적 마이그레이션으로 영향 범위를 최소화. 각 단계에서 Day/Night 프리셋과 기존 light/dark 모드의 시각적 동등성을 수동 비교 검증 |
| M15: Custom 테마 색상 조합으로 가독성 저하 | 중 | 중 | 사용자 책임 영역이나, 초기화(Reset) 버튼으로 Day 프리셋 기본값 복원 가능. 향후 대비 충분 |
| M15: FOUC(Flash of Unstyled Content) 발생 | 중 | 낮 | localStorage에서 테마 설정을 동기적으로 읽어 `<html>` data-theme 속성을 설정하는 초기화 스크립트를 `<head>`에 인라인으로 배치. ThemeProvider보다 먼저 실행되도록 보장 |
| M15: data-theme 속성과 dark 클래스 공존 시 CSS 우선순위 충돌 | 중 | 중 | `[data-theme]` 셀렉터의 특이성(specificity)이 `.dark` 셀렉터보다 높도록 설계. 마이그레이션 완료 후 dark 클래스 의존성 완전 제거 |
| M16: 기존 사용자 대시보드 레이아웃 마이그레이션 실패 | 중 | 중 | persist version 1->2 마이그레이션에서 기존 dashboardLayout을 기본 DashboardPageConfig으로 변환. 마이그레이션 실패 시 DEFAULT 레이아웃으로 폴백 |
| M16: 동일 타입 패널 복수 사용 시 WebSocket/React Query 데이터 공유 충돌 | 낮 | 중 | 패널별 독립적 데이터 구독. React Query key에 패널 ID 불포함 (같은 데이터 공유 허용). WebSocket은 글로벌 구독으로 모든 패널이 동일 이벤트 수신 |

---

## 19. 변경 파일 목록 (전체)

| 파일 | 모듈 | 변경 유형 |
|------|------|----------|
| `web/src/services/api/agentService.ts` | M1 | 수정 |
| `internal/api/dto/request.go` | M1 | 수정 |
| `internal/api/handler/flow.go` | M1 | 수정 |
| `internal/api/service/agent_adapter.go` | M1 | 수정 |
| `internal/api/handler/monitor.go` | M2 | 수정 |
| `internal/api/handler/monitor_test.go` | M2 | 수정/신규 |
| `web/src/services/api/monitorService.ts` | M3 | 수정 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | M3 | 수정 |
| `web/src/pages/settings/SettingsPage.tsx` | M3 | 수정 |
| `web/src/pages/flows/FlowDetailPanel.tsx` | M4 | 신규 |
| `web/src/pages/flows/FlowListPage.tsx` | M4 | 수정 |
| `web/src/hooks/useFlow.ts` | M4 | 수정 |
| `web/src/contexts/RuntimeStatsContext.ts` | M5 | 신규 |
| `web/src/pages/editor/EditorPage.tsx` | M5 | 수정 |
| `web/src/components/flow/CustomNode.tsx` | M5 | 수정 |
| `internal/engine/engine.go` | BF | 수정 |
| `internal/node/bridge.go` | BF | 수정 |
| `internal/node/bridge_test.go` | BF | 수정 |
| `web/src/pages/monitoring/LogViewer.tsx` | M7 | 수정 |
| `web/src/pages/monitoring/MonitoringPage.tsx` | M7 | 수정 |
| `web/src/config/nodeSchemas.ts` | M8 | 수정 |
| `web/src/pages/editor/EditorPage.tsx` | M8 (+ M5) | 수정 |
| `pkg/flow/node.go` | M8 | 수정 |
| `web/src/config/nodeSchemas.ts` | M9 (+ M8) | 수정 |
| `web/src/types/node.ts` | M9 | 수정 |
| `web/src/components/flow/CustomNode.tsx` | M9 (+ M5) | 수정 |
| `web/src/components/flow/NodeHandle.tsx` | M9 | 수정 |
| `web/src/components/flow/CustomNode.tsx` | M10 (+ M5, M9) | 수정 |
| `internal/api/service/flow_adapter.go` | M10 (+ M11) | 수정 |
| `web/src/components/property/PropertyPanel.tsx` | M10 | 수정 |
| `internal/api/service/sort_util.go` | M11 | 신규 |
| `internal/api/service/sort_util_test.go` | M11 | 신규 |
| `internal/api/service/flow_adapter.go` | M11 (+ M10) | 수정 |
| `internal/api/service/agent_adapter.go` | M11 (+ M1) | 수정 |
| `web/src/components/common/SortableHeader.tsx` | M11 | 신규 |
| `web/src/pages/flows/FlowListPage.tsx` | M11 (+ M4) | 수정 |
| `web/src/pages/agents/AgentListPage.tsx` | M11 | 수정 |
| `web/src/pages/flows/FlowDetailPanel.tsx` | M11 (+ M4) | 수정 |
| `web/src/pages/dashboard/panels/FlowPanel.tsx` | M12 | 신규 |
| `web/src/pages/dashboard/panels/AgentPanel.tsx` | M12 | 신규 |
| `web/src/pages/dashboard/DashboardPage.tsx` | M12 | 수정 |
| `web/src/pages/dashboard/widgets/SystemStatusWidget.tsx` | M12 | 삭제 |
| `web/src/pages/dashboard/widgets/RecentFlowsWidget.tsx` | M12 | 삭제 |
| `web/src/pages/dashboard/widgets/AgentStatusWidget.tsx` | M12 | 삭제 |
| `web/src/pages/dashboard/widgets/ResourceWidget.tsx` | M12 | 유지 (소폭 수정 가능) |
| `internal/api/handler/flow.go` | M13 (+ M1) | 수정 |
| `web/src/lib/utils/download.ts` | M13 | 신규 |
| `web/src/lib/utils/importParser.ts` | M13 | 신규 |
| `web/src/components/common/ImportDialog.tsx` | M13 | 신규 |
| `web/src/services/api/flowService.ts` | M13 (+ M4) | 수정 |
| `web/src/services/api/agentService.ts` | M13 (+ M1) | 수정 |
| `web/src/pages/flows/FlowActionMenu.tsx` | M13 | 수정 |
| `web/src/pages/flows/FlowListPage.tsx` | M13 (+ M4, M11) | 수정 |
| `web/src/pages/agents/AgentListPage.tsx` | M13 (+ M11) | 수정 |
| `web/package.json` | M13 | 수정 |
| `web/src/index.css` | M15-1, M15-2 | 수정 |
| `web/src/lib/theme/tokens.ts` | M15-1, M15-2 | 신규 |
| `web/src/stores/uiStore.ts` | M15-3 | 수정 |
| `web/src/hooks/useTheme.ts` | M15-3 | 수정 |
| `web/src/lib/theme/ThemeProvider.tsx` | M15-3 | 수정 |
| `web/src/components/theme/ThemeSelector.tsx` | M15-4 | 신규 |
| `web/src/components/layout/Header.tsx` | M15-4 | 수정 |
| `web/src/components/theme/ThemeEditorModal.tsx` | M15-5 | 신규 |
| `web/src/components/theme/ColorTokenInput.tsx` | M15-5 | 신규 |
| 전체 컴포넌트 (~30-40 파일) | M15-6 | 수정 (`dark:` 클래스 마이그레이션) |
| `web/src/stores/uiStore.ts` | M16-1 (+ M15-3) | 수정 |
| `web/src/pages/dashboard/DashboardToolbar.tsx` | M16-2 | 신규 |
| `web/src/pages/dashboard/CreateDashboardDialog.tsx` | M16-2 | 신규 |
| `web/src/pages/dashboard/DashboardPage.tsx` | M16-2, M16-3 (+ M12) | 수정 |
| `web/src/pages/dashboard/AddPanelDialog.tsx` | M16-3 | 신규 |
| `web/src/pages/dashboard/panels/DevicePanel.tsx` | M16-4 | 신규 |
| `web/src/pages/dashboard/panels/LogPanel.tsx` | M16-4 | 신규 |

---

## 20. 전문가 상담 권장

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 | expert-backend | Monitor 핸들러 API 설계, LevelManager 통합, Go 테스트 |
| 프론트엔드 | expert-frontend | React 컴포넌트 설계, shadcn/ui 드롭다운 통합, 상태 관리 |
| 프론트엔드 (M15) | expert-frontend | CSS Variable 디자인 토큰 체계, Tailwind CSS v4 @theme 블록 통합, 테마 에디터 UI, dark: 클래스 마이그레이션 전략 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.12.0*
*상태: in_progress*
*최종 수정: 2026-03-17*
