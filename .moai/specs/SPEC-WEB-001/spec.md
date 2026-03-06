---
id: SPEC-WEB-001
version: "1.0.0"
status: completed
created: "2026-03-07"
updated: "2026-03-07"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-07 | 1.0.0 | 초기 SPEC 작성 - 에이전트 통계 버그 수정 + 컴포넌트별 로그 레벨 제어 |

---

# SPEC-WEB-001: 웹 UI 에이전트 통계 표시 버그 수정 및 디버깅 레벨 설정

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 Web Dashboard에서 에이전트 관련 두 가지 이슈를 해결한다:

1. **에이전트 목록 통계 버그 수정**: 에이전트 목록 페이지에서 uptime, messages_in, messages_out 통계가 항상 빈 값(`-`)으로 표시되는 버그
2. **컴포넌트별 디버깅 레벨 설정**: 현재 글로벌 로그 레벨 설정만 지원하는데, 에이전트/노드 단위의 개별 로그 레벨 제어 기능 추가

### 1.2 기술 환경

- **프론트엔드**: TypeScript 5.x, React 19.x, Vite 6.x, Tailwind CSS 4.x
- **백엔드**: Go, xflowd 데몬
- **패키지 경로**: `web/src/` (프론트엔드), `internal/` (백엔드)
- **상태 관리**: Zustand 5.x (클라이언트 상태), @tanstack/react-query 5.x (서버 상태)
- **HTTP 클라이언트**: axios
- **UI 컴포넌트**: shadcn/ui + Radix UI
- **의존 SPEC**:
  - SPEC-API-001: REST API 엔드포인트
  - SPEC-OBS-001: 관찰성 시스템 (LevelManager 인터페이스)
  - SPEC-AGENT-001: Agent 관리

### 1.3 설계 원칙

- **최소 변경 원칙**: 기존 백엔드 LevelManager 인터페이스를 최대한 활용하여 새로운 추상화를 최소화한다
- **프론트엔드 우선 수정**: Issue 1은 프론트엔드만 수정하여 해결한다 (백엔드 변경 불필요)
- **점진적 기능 추가**: 글로벌 로그 레벨 기능을 유지하면서 컴포넌트별 로그 레벨 기능을 추가한다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Module 1: 에이전트 목록 API 호출 시 `detail=summary` 파라미터 추가 (프론트엔드)
- Module 2: 컴포넌트별 로그 레벨 API 엔드포인트 추가 (백엔드)
- Module 3: 컴포넌트별 로그 레벨 UI 추가 (프론트엔드)

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- 로그 레벨 영속화 (서버 재시작 시 초기화됨)
- 로그 레벨 변경 이력 감사(audit) 로그
- 실시간 WebSocket 기반 로그 레벨 동기화

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| LevelManager | Go 백엔드의 `observe.LevelManager` 인터페이스. 컴포넌트별 slog.Level을 관리한다 |
| Component | 로그 레벨을 개별 설정할 수 있는 단위. `agent.{name}`, `node.{name}` 형식 |
| Pattern | 와일드카드(`*`)를 포함하는 컴포넌트 매칭 문자열. 예: `agent.*` |
| detail parameter | 에이전트 목록 API의 쿼리 파라미터. `summary`일 때 stats, uptime, health 포함 |
| Global Log Level | 시스템 전체에 적용되는 기본 로그 레벨 |
| Component Log Level | 특정 에이전트 또는 노드에만 적용되는 개별 로그 레벨 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: 백엔드 `GET /agents` API는 `detail=summary` 쿼리 파라미터를 이미 지원하며, stats, uptime, health 필드를 반환한다
- A-002: `observe.LevelManager` 인터페이스의 `SetLevel`, `GetLevel`, `SetLevelByPattern`, `ListLevels` 메서드가 정상 동작한다
- A-003: `internal/api/handler/monitor.go`의 기존 `PUT /monitor/loglevel` 핸들러가 글로벌 로그 레벨 변경 기능을 올바르게 수행한다
- A-004: 프론트엔드 `AgentInfo` 타입에 이미 `stats`, `uptime`, `health` optional 필드가 정의되어 있다

### 3.2 운영 가정

- A-005: 컴포넌트별 로그 레벨은 서버 메모리에만 저장되며, 서버 재시작 시 기본 레벨로 초기화된다
- A-006: 동시에 활성화되는 컴포넌트별 로그 레벨 오버라이드 수는 최대 100개 이내이다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: 에이전트 목록 통계 버그 수정 (P0 - 버그 수정)

#### REQ-WEB-001-01-01 (Event-Driven)
**WHEN** 에이전트 목록 페이지가 로드될 때, **THEN** `GET /agents` API 호출 시 `detail=summary` 쿼리 파라미터를 포함하여 stats, uptime, health 데이터를 함께 요청해야 한다.

#### REQ-WEB-001-01-02 (State-Driven)
**IF** 에이전트 목록 API 응답에 `stats` 필드가 포함된 상태 **THEN** 에이전트 테이블의 "업타임" 컬럼에 `uptime` 값을, "메시지 (IN/OUT)" 컬럼에 `stats.messages_in / stats.messages_out` 값을 표시해야 한다.

#### REQ-WEB-001-01-03 (State-Driven)
**IF** 에이전트의 `stats` 필드가 없거나 null인 상태 **THEN** 해당 통계 컬럼에 `-` 기본값을 표시해야 한다.

### 4.2 Module 2: 백엔드 컴포넌트별 로그 레벨 API (P1 - 신규 기능)

#### REQ-WEB-001-02-01 (Event-Driven)
**WHEN** `PUT /monitor/loglevel/{component}` 요청을 수신하면, **THEN** `LevelManager.SetLevel(component, level)`을 호출하여 해당 컴포넌트의 로그 레벨을 설정하고, 성공 응답을 반환해야 한다.

#### REQ-WEB-001-02-02 (Event-Driven)
**WHEN** `GET /monitor/loglevel` 요청을 수신하면, **THEN** `LevelManager.ListLevels()`를 호출하여 모든 컴포넌트의 로그 레벨 맵(`component -> level`)과 현재 기본 레벨을 반환해야 한다.

#### REQ-WEB-001-02-03 (Event-Driven)
**WHEN** `DELETE /monitor/loglevel/{component}` 요청을 수신하면, **THEN** 해당 컴포넌트의 개별 로그 레벨 오버라이드를 제거하고 기본 레벨로 리셋해야 한다.

#### REQ-WEB-001-02-04 (Unwanted)
시스템은 유효하지 않은 로그 레벨 값(debug, info, warn, error 외)에 대해 설정을 **허용하지 않아야 한다**. 400 Bad Request 응답을 반환해야 한다.

#### REQ-WEB-001-02-05 (Ubiquitous)
시스템은 **항상** 기존 `PUT /monitor/loglevel` 글로벌 로그 레벨 변경 API의 하위 호환성을 유지해야 한다.

### 4.3 Module 3: 프론트엔드 컴포넌트별 로그 레벨 UI (P1 - 신규 기능)

#### REQ-WEB-001-03-01 (Event-Driven)
**WHEN** 에이전트 상세 패널이 열리면, **THEN** 해당 에이전트의 현재 로그 레벨을 표시하는 드롭다운 컨트롤을 통계 탭 또는 별도 영역에 제공해야 한다.

#### REQ-WEB-001-03-02 (Event-Driven)
**WHEN** 에이전트 로그 레벨 드롭다운 값을 변경하면, **THEN** `PUT /monitor/loglevel/agent.{agent-id}` API를 호출하여 해당 에이전트의 로그 레벨을 변경하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-03-03 (Event-Driven)
**WHEN** 에이전트 로그 레벨을 "기본값" 옵션으로 변경하면, **THEN** `DELETE /monitor/loglevel/agent.{agent-id}` API를 호출하여 개별 오버라이드를 제거해야 한다.

#### REQ-WEB-001-03-04 (Event-Driven)
**WHEN** 설정 페이지의 시스템 탭이 로드되면, **THEN** `GET /monitor/loglevel` API를 호출하여 글로벌 기본 레벨과 컴포넌트별 오버라이드 목록을 표시해야 한다.

#### REQ-WEB-001-03-05 (Event-Driven)
**WHEN** 설정 페이지에서 컴포넌트별 오버라이드의 "리셋" 버튼을 클릭하면, **THEN** `DELETE /monitor/loglevel/{component}` API를 호출하여 해당 오버라이드를 제거하고 목록을 갱신해야 한다.

#### REQ-WEB-001-03-06 (State-Driven)
**IF** 사용자 역할이 Viewer인 경우 **THEN** 로그 레벨 변경 드롭다운과 리셋 버튼을 비활성화(disabled)해야 한다.

---

## 5. Specifications (기술 사양)

### 5.1 Module 1: 프론트엔드 수정 사항

**파일**: `web/src/services/api/agentService.ts`

```typescript
// 변경 전
export async function getAgents(params?: ListOptions): Promise<{ data: AgentInfo[]; total: number }> {
  return getList<AgentInfo>('/agents', { params });
}

// 변경 후
export async function getAgents(params?: ListOptions): Promise<{ data: AgentInfo[]; total: number }> {
  return getList<AgentInfo>('/agents', {
    params: { ...params, detail: 'summary' },
  });
}
```

### 5.2 Module 2: 백엔드 API 엔드포인트

| Method | Endpoint | Request Body | Response | 설명 |
|--------|----------|-------------|----------|------|
| GET | `/monitor/loglevel` | - | `{ default_level: string, components: Record<string, string> }` | 전체 로그 레벨 조회 |
| PUT | `/monitor/loglevel` | `{ level: string }` | `{ level: string }` | 글로벌 기본 레벨 변경 (기존) |
| PUT | `/monitor/loglevel/{component}` | `{ level: string }` | `{ component: string, level: string }` | 컴포넌트별 레벨 설정 |
| DELETE | `/monitor/loglevel/{component}` | - | `{ component: string }` | 컴포넌트 레벨 리셋 |

**유효한 level 값**: `debug`, `info`, `warn`, `error`

**component 형식 예시**:
- `agent.modbus-001` - 특정 에이전트
- `node.transform-1` - 특정 노드
- `engine.scheduler` - 엔진 컴포넌트

### 5.3 Module 3: 프론트엔드 API 서비스 확장

**파일**: `web/src/services/api/monitorService.ts` - 확장

```typescript
// 기존 함수 유지
export async function setLogLevel(level: string): Promise<void>;

// 새로 추가
export async function getLogLevels(): Promise<LogLevelInfo>;
export async function setComponentLogLevel(component: string, level: string): Promise<void>;
export async function resetComponentLogLevel(component: string): Promise<void>;
```

**새 타입 정의**:
```typescript
interface LogLevelInfo {
  default_level: string;
  components: Record<string, string>;
}
```

### 5.4 UI 변경 요약

| 위치 | 변경 내용 |
|------|----------|
| `AgentDetailPanel.tsx` 통계 탭 | 로그 레벨 드롭다운 추가 (debug/info/warn/error/기본값) |
| `SettingsPage.tsx` 시스템 탭 | 컴포넌트별 오버라이드 목록 테이블 추가 (컴포넌트명, 레벨, 리셋 버튼) |
| `monitorService.ts` | `getLogLevels`, `setComponentLogLevel`, `resetComponentLogLevel` 함수 추가 |
| `agentService.ts` | `getAgents` 호출에 `detail=summary` 파라미터 추가 |

### 5.5 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC | 의존 내용 |
|-------------|-----------|----------|
| Module 1 | SPEC-API-001 | `GET /agents?detail=summary` 쿼리 파라미터 지원 |
| Module 2 | SPEC-OBS-001 | `observe.LevelManager` 인터페이스 (SetLevel, GetLevel, ListLevels) |
| Module 3 | SPEC-API-001 | 신규 `/monitor/loglevel/{component}` 엔드포인트 |

### 5.6 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: 에이전트 목록 통계 버그 수정 | 사용자에게 보이는 기존 기능 장애 |
| P1 (중요) | Module 2: 백엔드 로그 레벨 API | Module 3의 전제 조건 |
| P1 (중요) | Module 3: 프론트엔드 로그 레벨 UI | 디버깅 효율성 향상 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-03-07*
