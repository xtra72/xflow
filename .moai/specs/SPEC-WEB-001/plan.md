---
id: SPEC-WEB-001
type: plan
version: "1.0.0"
status: completed
created: "2026-03-07"
updated: "2026-03-07"
author: xtra
---

# SPEC-WEB-001: 구현 계획 - 에이전트 통계 버그 수정 및 디버깅 레벨 설정

## 1. 마일스톤 개요

| 마일스톤 | 모듈 | 우선순위 | 의존성 |
|----------|------|----------|--------|
| M1: 에이전트 목록 통계 버그 수정 | Module 1 | P0 (즉시) | 없음 |
| M2: 백엔드 컴포넌트별 로그 레벨 API | Module 2 | P1 (중요) | 없음 |
| M3: 프론트엔드 로그 레벨 UI | Module 3 | P1 (중요) | M2 완료 필수 |

---

## 2. M1: 에이전트 목록 통계 버그 수정 (P0)

### 2.1 근본 원인 분석

- **증상**: `AgentListPage.tsx`에서 uptime, messages_in, messages_out 컬럼이 항상 `-`로 표시됨
- **원인**: `agentService.getAgents()`가 `GET /agents` 호출 시 `detail` 파라미터를 전달하지 않음
- **결과**: 백엔드가 기본 필드(id, name, type, status, config, connected)만 반환하여 stats, uptime, health 필드가 누락됨

### 2.2 수정 대상 파일

| 파일 | 변경 내용 | 변경 크기 |
|------|----------|----------|
| `web/src/services/api/agentService.ts` | `getAgents()` 호출에 `detail: 'summary'` 파라미터 추가 | 소 (3줄) |
| `web/src/pages/agents/AgentListPage.tsx` | 변경 불필요 - 이미 `agent.stats`, `agent.uptime` 필드를 올바르게 참조 중 | 없음 |

### 2.3 기술 접근

`agentService.ts`의 `getAgents()` 함수에서 `getList` 호출 시 params에 `detail: 'summary'`를 병합한다.

```
변경 전: getList<AgentInfo>('/agents', { params })
변경 후: getList<AgentInfo>('/agents', { params: { ...params, detail: 'summary' } })
```

### 2.4 검증 방법

- 에이전트 목록 페이지 로드 시 Network 탭에서 `GET /api/v1/agents?detail=summary` 호출 확인
- 응답 JSON에 `stats`, `uptime` 필드가 포함되는지 확인
- UI에서 업타임 및 메시지 통계가 `-` 대신 실제 값으로 표시되는지 확인

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

## 5. 의존성 그래프

```
M1 (에이전트 통계 버그) ──── 독립 (즉시 실행 가능)

M2 (백엔드 API) ──── 독립 (M1과 병렬 실행 가능)
  │
  └── M3 (프론트엔드 UI) ──── M2 완료 후 실행
```

### 실행 순서

1. **M1 + M2 병렬 진행**: M1은 프론트엔드만, M2는 백엔드만 수정하므로 파일 충돌 없이 병렬 가능
2. **M3 순차 진행**: M2의 API가 완성된 후 프론트엔드 UI 구현

---

## 6. 리스크 분석

| 리스크 | 심각도 | 발생 확률 | 완화 방안 |
|--------|--------|----------|----------|
| `detail=summary`가 대량 에이전트에서 성능 저하 유발 | 중 | 낮 | 에이전트 수가 적은 IoT 환경이므로 영향 미미. 필요 시 페이지네이션 활용 |
| `LevelManager`에 `ListLevels` 미구현 또는 상이한 시그니처 | 중 | 낮 | 구현 전 인터페이스 확인 완료. 테스트 코드에서 `ListLevels` 사용 확인됨 |
| 컴포넌트 ID 형식 불일치 (프론트엔드 vs 백엔드) | 높 | 중 | 에이전트 ID 기반 `agent.{id}` 형식으로 통일. 구현 시 백엔드 컴포넌트 등록 패턴 확인 필요 |
| 기존 글로벌 로그 레벨 API 하위 호환성 깨짐 | 높 | 낮 | GET 응답 확장 방식으로 기존 필드 유지. 기존 PUT 동작 변경 없음 |

---

## 7. 변경 파일 목록 (전체)

| 파일 | 모듈 | 변경 유형 |
|------|------|----------|
| `web/src/services/api/agentService.ts` | M1 | 수정 |
| `internal/api/handler/monitor.go` | M2 | 수정 |
| `internal/api/handler/monitor_test.go` | M2 | 수정/신규 |
| `web/src/services/api/monitorService.ts` | M3 | 수정 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | M3 | 수정 |
| `web/src/pages/settings/SettingsPage.tsx` | M3 | 수정 |

---

## 8. 전문가 상담 권장

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 | expert-backend | Monitor 핸들러 API 설계, LevelManager 통합, Go 테스트 |
| 프론트엔드 | expert-frontend | React 컴포넌트 설계, shadcn/ui 드롭다운 통합, 상태 관리 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-03-07*
