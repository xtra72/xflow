---
id: SPEC-WEB-003
version: "1.1.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
---

# SPEC-WEB-003 구현 계획: Agent Observer 통합

## 1. 구현 전략

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 준수):
- **새 파일** (`internal/agent/logger.go`): TDD (RED-GREEN-REFACTOR)
- **기존 파일 수정** (manager.go, config.go, 각 에이전트 파일): DDD (ANALYZE-PRESERVE-IMPROVE)

### 1.2 실행 순서

Module 1 → Module 2 → Module 3 (순차적 의존성)

---

## 2. Module별 구현 계획

### 2.1 Module 1: Manager Observer 통합

**마일스톤**: Manager가 Observer를 수신하고 에이전트 생성 시 로거를 주입하는 인프라 구축

#### 변경 파일 목록

| 파일 | 변경 유형 | 예상 크기 |
|------|----------|----------|
| `internal/agent/config.go` | 필드 추가 | ~5줄 |
| `internal/agent/manager.go` | 옵션 패턴 추가 + Create() 수정 | ~30줄 |
| `internal/agent/manager_test.go` | 테스트 추가 | ~50줄 |
| `cmd/xflowd/main.go` | WithObserver 호출 추가 | ~1줄 |

#### 구현 단계

1. `AgentConfig`에 `Logger *slog.Logger` 필드 추가 (json:"-" 태그)
2. `ManagerOption` 타입 및 `WithObserver()` 옵션 함수 정의
3. `NewManager()` 시그니처를 `NewManager(opts ...ManagerOption)`으로 변경
4. `DefaultManager`에 `observer *observe.Observer` 필드 추가
5. `Create()` 메서드에 로거 주입 로직 추가
6. `main.go`에서 `NewManager(agent.WithObserver(obs))` 호출
7. 테스트 작성 및 검증

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| NewManager() 시그니처 변경으로 기존 호출 코드 영향 | 중 | 가변 인자이므로 기존 `NewManager()` 호출은 영향 없음 |
| Observer nil 시 패닉 | 높 | Create()에서 nil 체크 후 기존 동작 유지 |

### 2.2 Module 2: Agent 로거 마이그레이션

**마일스톤**: 모든 에이전트 팩토리가 주입된 로거를 사용

#### 변경 파일 목록

| 파일 | 변경 유형 | 예상 크기 |
|------|----------|----------|
| `internal/agent/logger.go` (신규) | 헬퍼 함수 | ~15줄 |
| `internal/agent/logger_test.go` (신규) | 테스트 | ~30줄 |
| `internal/agent/modbus/agent.go` | slog.Default() 교체 | ~2줄 |
| `internal/agent/modbusserver/agent.go` | slog.Default() 교체 | ~2줄 |
| `internal/agent/samsung/agent.go` | slog.Default() 교체 | ~2줄 |
| `internal/agent/system/mqtt_subscriber.go` | slog.Default() 교체 | ~2줄 |
| `internal/agent/system/console_logger.go` | slog.Default() 교체 | ~2줄 |
| `internal/agent/system/influxdb_agent.go` | slog.Default() 교체 | ~2줄 |

#### 구현 단계

1. `internal/agent/logger.go` 생성 - `ResolveLogger(config AgentConfig) *slog.Logger` 헬퍼
2. 헬퍼 함수 테스트 작성 (TDD)
3. 각 에이전트 팩토리에서 `slog.Default()` → `agent.ResolveLogger(config)` 교체
4. 기존 테스트 실행하여 하위 호환성 확인

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| 에이전트 패키지 간 순환 의존성 | 중 | ResolveLogger는 agent 패키지에 위치하므로 하위 패키지에서 import 가능 |
| 기존 테스트에서 Logger가 nil인 AgentConfig 사용 | 낮 | ResolveLogger가 nil 시 slog.Default() 반환하여 호환 |

### 2.3 Module 3: Agent 로그 레벨 관리

**마일스톤**: 에이전트별 로그 레벨 등록 및 런타임 변경 가능

#### 변경 파일 목록

| 파일 | 변경 유형 | 예상 크기 |
|------|----------|----------|
| `internal/agent/config.go` | LogLevel 필드 추가 | ~3줄 |
| `internal/agent/manager.go` | Create()에 레벨 설정 로직 | ~10줄 |
| `internal/agent/manager_test.go` | 테스트 추가 | ~30줄 |

#### 구현 단계

1. `AgentConfig`에 `LogLevel string` 필드 추가 (이미 있으면 확인)
2. `Create()`에서 LogLevel 파싱 및 `observer.Levels.SetLevel()` 호출
3. 테스트 작성

---

## 3. 전체 파일 변경 요약

| 파일 | M1 | M2 | M3 | 총 변경 |
|------|----|----|----|----|
| `internal/agent/config.go` | O | - | O | 수정 |
| `internal/agent/manager.go` | O | - | O | 수정 |
| `internal/agent/manager_test.go` | O | - | O | 수정 |
| `internal/agent/logger.go` | - | O | - | 신규 |
| `internal/agent/logger_test.go` | - | O | - | 신규 |
| `internal/agent/modbus/agent.go` | - | O | - | 수정 |
| `internal/agent/modbusserver/agent.go` | - | O | - | 수정 |
| `internal/agent/samsung/agent.go` | - | O | - | 수정 |
| `internal/agent/system/mqtt_subscriber.go` | - | O | - | 수정 |
| `internal/agent/system/console_logger.go` | - | O | - | 수정 |
| `internal/agent/system/influxdb_agent.go` | - | O | - | 수정 |
| `cmd/xflowd/main.go` | O | - | - | 수정 |

**총 파일 수**: 12개 (신규 2개, 수정 10개)

---

## 4. 테스트 전략

### 4.1 단위 테스트

- `ResolveLogger()` 함수: nil → slog.Default(), non-nil → 주입된 로거 반환
- `NewManager(WithObserver(obs))`: Observer 저장 확인
- `Create()` with Observer: config.Logger에 Observer 기반 로거 설정 확인
- `Create()` without Observer: 기존 동작 유지 확인 (config.Logger = nil)
- LogLevel 파싱: "debug", "info", "warn", "error", "" 각각 올바른 slog.Level 반환

### 4.2 통합 테스트

- Observer + Manager + 에이전트 생성 → StreamRouter를 통한 로그 라우팅 검증
- SetDefaultWriter(MultiWriter) 상태에서 에이전트 로그가 wsLogWriter에 도달하는지 확인

### 4.3 하위 호환성 테스트

- 기존 에이전트 테스트가 수정 없이 통과하는지 확인 (`go test -race ./internal/agent/...`)

---

## 5. 리스크 요약

| 리스크 | 심각도 | 발생 확률 | 완화 방안 |
|--------|--------|----------|----------|
| NewManager 시그니처 변경 영향 | 낮 | 낮 | 가변 인자 패턴으로 기존 코드 호환 |
| 순환 의존성 (agent → observe → agent) | 중 | 낮 | agent 패키지에서 observe를 단방향 import. 역방향 없음 |
| 기존 테스트 깨짐 | 중 | 낮 | ResolveLogger nil 폴백으로 하위 호환성 보장 |
| 에이전트 로그 폭증으로 WebSocket 과부하 | 중 | 중 | SPEC-WEB-002의 rate limiter (초당 100건) 활용 |

---

*SPEC ID: SPEC-WEB-003*
*버전: 1.1.0*
*상태: completed*
*최종 수정: 2026-03-08*
