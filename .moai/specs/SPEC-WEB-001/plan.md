---
id: SPEC-WEB-001
type: plan
version: "1.3.0"
status: completed
created: "2026-03-07"
updated: "2026-03-08"
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
| M7: 로그 뷰어 컴포넌트/소스 필터링 | Module 7 | P1 (중요) | SPEC-OBS-004 완료 필수 | 계획됨 |

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

## 9. 의존성 그래프

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
```

### 실행 순서

1. **BF + M1 + M2 병렬 진행**: BF는 백엔드 버그 수정, M1은 프론트엔드+백엔드, M2는 백엔드 monitor 핸들러만 수정
2. **M3 + M4 병렬 진행**: M2의 API가 완성된 후 에이전트 UI(M3)와 노드 UI(M4) 병렬 구현 가능
3. **M5 진행**: M4의 FlowDetailPanel 패턴을 기반으로 캔버스 런타임 통계 구현
4. **M7 독립 진행**: SPEC-OBS-004 백엔드 구현이 완료된 상태이므로 즉시 실행 가능. M1-M5와 파일 충돌 없음 (LogViewer.tsx, MonitoringPage.tsx만 수정)

---

## 9. 리스크 분석

| 리스크 | 심각도 | 발생 확률 | 완화 방안 |
|--------|--------|----------|----------|
| `detail=summary`가 대량 에이전트에서 성능 저하 유발 | 중 | 낮 | 에이전트 수가 적은 IoT 환경이므로 영향 미미. 필요 시 페이지네이션 활용 |
| `LevelManager`에 `ListLevels` 미구현 또는 상이한 시그니처 | 중 | 낮 | 구현 전 인터페이스 확인 완료. 테스트 코드에서 `ListLevels` 사용 확인됨 |
| 컴포넌트 ID 형식 불일치 (프론트엔드 vs 백엔드) | 높 | 중 | 에이전트 ID 기반 `agent.{id}` 형식으로 통일. 구현 시 백엔드 컴포넌트 등록 패턴 확인 필요 |
| 기존 글로벌 로그 레벨 API 하위 호환성 깨짐 | 높 | 낮 | GET 응답 확장 방식으로 기존 필드 유지. 기존 PUT 동작 변경 없음 |
| M7: 10,000건 로그에서 3중 필터 성능 저하 | 중 | 낮 | useMemo 캐싱 + 단순 순회 방식으로 16ms 이내 보장. Set.has()와 includes()는 O(1)/O(n) 복잡도 |
| M7: 기존 LogEntry 인터페이스 변경으로 인한 타입 호환성 | 낮 | 낮 | component/source를 optional 필드로 추가하여 기존 코드 영향 없음 |
| M7: 행 너비 초과로 인한 레이아웃 깨짐 | 중 | 중 | source 배지(55px) + component(140px) 추가로 총 385px 고정 너비 사용. message 영역이 truncate 처리되므로 문제 없음. 모바일 반응형은 OUT OF SCOPE |

---

## 10. 변경 파일 목록 (전체)

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

---

## 12. 전문가 상담 권장

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 | expert-backend | Monitor 핸들러 API 설계, LevelManager 통합, Go 테스트 |
| 프론트엔드 | expert-frontend | React 컴포넌트 설계, shadcn/ui 드롭다운 통합, 상태 관리 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.3.0*
*상태: planned*
*최종 수정: 2026-03-08*
