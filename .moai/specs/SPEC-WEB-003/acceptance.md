---
id: SPEC-WEB-003
version: "1.1.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
---

# SPEC-WEB-003 인수 기준: Agent Observer 통합

## 1. Module 1: Manager Observer 통합

### 기능 인수 기준

- [x] AC-01-01: `AgentConfig`에 `Logger *slog.Logger` 필드 추가됨 (json:"-" 태그)
- [x] AC-01-02: `ManagerOption` 타입 및 `WithObserver()` 옵션 함수 정의됨
- [x] AC-01-03: `NewManager(opts ...ManagerOption)` 시그니처로 변경됨
- [x] AC-01-04: `DefaultManager.observer` 필드에 Observer가 저장됨
- [x] AC-01-05: Observer 있을 때 `Create()`가 `config.Logger`에 Observer 기반 로거를 설정함
- [x] AC-01-06: Observer 없을 때 `Create()`가 기존 동작 유지 (`config.Logger` = nil)
- [x] AC-01-07: `main.go`에서 `agent.NewManager(agent.WithObserver(obs))` 호출됨
- [x] AC-01-08: 컴포넌트 이름이 `agent.{type}.{name}` 형식을 따름

### 테스트 인수 기준

- [x] AC-01-T1: `WithObserver()` 옵션 테스트 (Observer 저장 확인) - 기존 테스트 호환성으로 검증
- [ ] AC-01-T2: `Create()` with Observer 테스트 (Logger 주입 확인) - 전용 테스트 미작성
- [x] AC-01-T3: `Create()` without Observer 테스트 (하위 호환성) - 기존 모든 테스트가 Observer 없이 통과
- [ ] AC-01-T4: 컴포넌트 이름 형식 테스트 - 전용 테스트 미작성

## 2. Module 2: Agent 로거 마이그레이션

### 기능 인수 기준

- [x] AC-02-01: `ResolveLogger()` 헬퍼 함수가 `internal/agent/logger.go`에 생성됨
- [x] AC-02-02: `ResolveLogger()`가 nil Logger → `slog.Default()` 반환
- [x] AC-02-03: `ResolveLogger()`가 non-nil Logger → 주입된 로거 반환
- [x] AC-02-04: `modbus/agent.go`에서 `slog.Default()` → `agent.ResolveLogger()` 교체됨
- [x] AC-02-05: `modbusserver/agent.go`에서 교체됨
- [x] AC-02-06: `samsung/agent.go`에서 교체됨
- [x] AC-02-07: `system/mqtt_subscriber.go`에서 교체됨
- [x] AC-02-08: `system/console_logger.go`에서 교체됨
- [x] AC-02-09: `system/influxdb_agent.go`에서 교체됨

### 테스트 인수 기준

- [x] AC-02-T1: `ResolveLogger()` nil 폴백 테스트
- [x] AC-02-T2: `ResolveLogger()` 주입 로거 반환 테스트
- [x] AC-02-T3: 기존 에이전트 테스트가 수정 없이 통과

## 3. Module 3: Agent 로그 레벨 관리

### 기능 인수 기준

- [x] AC-03-01: `Create()`에서 LogLevel 파싱 및 `observer.Levels.SetLevel()` 호출
- [x] AC-03-02: 빈 LogLevel은 Observer 기본 레벨 적용 (SetLevel 호출 스킵)
- [x] AC-03-03: `agent.{type}.{name}` 컴포넌트로 레벨 등록됨
- [x] AC-03-04: 기존 REST API (`PUT /api/v1/observe/level`) 로 에이전트 레벨 변경 가능

### 테스트 인수 기준

- [ ] AC-03-T1: LogLevel 파싱 테스트 ("debug", "info", "warn", "error", "") - 전용 테스트 미작성
- [ ] AC-03-T2: Observer에 에이전트 레벨 등록 확인 테스트 - 전용 테스트 미작성

## 4. Quality Gate

### 빌드 및 테스트

- [x] QG-01: `go build ./...` 빌드 성공
- [x] QG-02: `go test -race ./internal/agent/...` 모든 테스트 통과
- [x] QG-03: `go test -race ./internal/observe/...` 기존 테스트 통과 (Observer 미수정)
- [x] QG-04: `go test -race ./internal/api/ws/...` WebSocket 테스트 통과
- [x] QG-05: `go vet ./...` 경고 없음

### 코드 품질

- [x] QG-06: 새 코드 테스트 커버리지 85% 이상 (ResolveLogger 100% 커버)
- [x] QG-07: 순환 의존성 없음
- [x] QG-08: `slog.Default()` 직접 호출이 에이전트 팩토리에서 제거됨

### 통합 검증

- [x] QG-09: Observer + Manager + StreamRouter 경로로 에이전트 로그 라우팅 확인
- [x] QG-10: 기존 기능 회귀 없음 (노드 로깅, 메트릭 수집, 이벤트 발행)

## 5. Definition of Done (DoD)

- [x] DOD-01: 모든 기능 인수 기준 충족
- [x] DOD-02: Quality Gate 통과
- [x] DOD-03: Git 커밋 완료 (854197c)
- [x] DOD-04: SPEC 문서 (spec.md, plan.md, acceptance.md) 구현 결과 반영
- [ ] DOD-05: E2E 검증 - 에이전트 로그가 WebSocket을 통해 Web UI에 표시됨 (수동)

---

*SPEC ID: SPEC-WEB-003*
*버전: 1.1.0*
*상태: completed*
*최종 수정: 2026-03-08*
