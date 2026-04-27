---
id: SPEC-LOG-001
version: "2.0.0"
status: draft
created: "2026-02-15"
updated: "2026-04-02"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-15 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-15 | 1.0.0 | 구현 완료 (61 tests, 88.0% coverage) |
| 2026-04-02 | 2.0.0 | ConsoleLoggerAgent 출력 형식 개선 -- 메시지 부분 선택 및 바이너리 포맷 추가 |

---

# SPEC-LOG-001: Logger System Agent - 로깅 시스템 에이전트

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 5개 System Agent(Event, Logger, File, Timer, Store) 중 하나인 Logger Agent를 정의한다. Logger Agent는 기존 `internal/observe` 패키지를 **래핑(wrapping)** 하여 System Agent 인터페이스를 제공하는 통합 로깅 서비스이다. `internal/observe`의 Observer, LoggerFactory, LevelManager, StreamRouter를 활용하며, 생명주기 관리와 Bridge Node 연동을 추가한다.

**핵심 설계 원칙: `internal/observe` 패키지를 대체하지 않고 래핑한다.**

본 SPEC은 다음을 다룬다:
- `Logger` 인터페이스 정의 (WriteLog, SetLevel, SetLevelByPattern, GetLevel, Subscribe, Unsubscribe, Components)
- `LoggerAgent` 구조체 (Observer 래핑, 생명주기 관리, 구독 관리)
- 옵션 시스템 (WithDefaultLevel, WithFormat, WithDefaultWriter, WithObserver)
- LoggerAgent 생명주기 (Lifecycle, HealthChecker 인터페이스 구현)
- Pause 시 Debug/Info 억제, Warn/Error 허용 (안전 우선 정책)
- Bridge Node 연동 (메시지 기반 로깅 제어)
- 에러 타입 정의
- **[v2.0.0] ConsoleLoggerAgent 메시지 콘텐츠 모드** (content_mode 설정 옵션)
- **[v2.0.0] ConsoleLoggerAgent 바이너리 출력 포맷** (format: "binary" 옵션)

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/system/logger.go` (LoggerAgent 구현), `internal/agent/system/console_logger.go` (ConsoleLoggerAgent 구현)
- **의존성**: 표준 라이브러리 (`sync`, `io`, `log/slog`, `context`, `errors`, `fmt`, `encoding/hex`) + `pkg/lifecycle/` + `internal/observe/`
- **선택적 의존성**: `pkg/message/` (Bridge Node 연동 시)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **래퍼 패턴**: `internal/observe`의 Observer를 래핑하여 System Agent 인터페이스를 제공. observe 패키지의 기능을 그대로 위임
- **동시성 안전**: 구독 관리(subscribe/unsubscribe)는 `sync.RWMutex`로 보호
- **안전 우선 Pause**: Pause 상태에서 Debug/Info는 억제하되 Warn/Error는 항상 허용 (안전 관련 로그 보장)
- **생명주기 통합**: `pkg/lifecycle/Lifecycle`, `HealthChecker` 인터페이스 구현
- **System Agent 패턴**: Transport/Protocol 설정 불필요, 시스템 시작 시 자동 활성화
- **최소 중복**: observe 패키지의 기존 기능을 재구현하지 않고 위임을 통해 활용
- **[v2.0.0] 하위 호환**: 기존 설정에 content_mode가 없으면 기본값 "full"로 동작하여 기존 동작 변경 없음

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `Logger` 인터페이스 정의 (WriteLog, SetLevel, SetLevelByPattern, GetLevel, Subscribe, Unsubscribe, Components) -- v1.0.0 완료
- `LoggerAgent` 구조체 (Observer 래핑, 구독 맵 관리, 생명주기) -- v1.0.0 완료
- 옵션 패턴 (WithDefaultLevel, WithFormat, WithDefaultWriter, WithObserver) -- v1.0.0 완료
- Pause 시 레벨별 억제 로직 (Debug/Info 억제, Warn/Error 허용) -- v1.0.0 완료
- Subscribe/Unsubscribe를 통한 동적 로그 스트림 라우팅 -- v1.0.0 완료
- Bridge Node 통한 메시지 기반 로깅 제어 패턴 정의 -- v1.0.0 완료
- 에러 타입 정의 (ErrLoggerClosed, ErrLoggerPaused, ErrInvalidLevel, ErrInvalidComponent, ErrSubscriptionNotFound) -- v1.0.0 완료
- **[v2.0.0] ConsoleLoggerAgent의 content_mode 설정 옵션 (full / payload)**
- **[v2.0.0] ConsoleLoggerAgent의 format에 "binary" (hex dump) 옵션 추가**
- **[v2.0.0] Web UI 설정 필드 추가 (agentTypeMeta.ts, agentSchemas.ts)**

**OUT OF SCOPE (별도 SPEC)**:
- `internal/observe` 패키지 자체 (이미 구현 완료, 86 tests, 95.7% coverage)
- Observer의 Metrics, Tracer 기능 (Logger Agent는 로깅/스트림만 래핑)
- Flow Engine 통합 (internal/engine/ - 별도 SPEC)
- Bridge Node 구현 자체 (internal/node/bridge.go - SPEC-FLOW-001 범위)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: internal/config/)
- REST API 핸들러 (별도 SPEC: internal/api/)
- 다른 System Agent 구현 (Event, File, Timer, Store)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Logger Agent가 Lifecycle, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001 | 동료 | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용 |
| SPEC-STORE-001 | 동료 | 동일한 System Agent 패턴(BaseLifecycle 임베딩, Options 패턴) 공유 |
| SPEC-TIMER-001 | 동료 | 동일한 System Agent 패턴 공유, Bridge 핸들러 패턴 참조 |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Logger Agent 참조 |
| SPEC-OBS-001 | 래핑 대상 | Logger Agent가 internal/observe의 Observer를 래핑 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `internal/observe/Observer`는 이미 완성된 관찰성 시스템이며, Logger Agent는 이를 래핑하여 System Agent 인터페이스를 추가한다
- A2: `observe.LoggerFactory`는 `sync.Map` 기반으로 동시성 안전하며, `NewLogger(component)`는 멱등적이다
- A3: `observe.LevelManager`는 `slog.LevelVar`(atomic) 기반으로 동시성 안전하며, `SetLevel`/`SetLevelByPattern` 호출이 즉시 반영된다
- A4: `observe.StreamRouter`는 `sync.RWMutex` 기반으로 동시성 안전하며, `AddRoute`/`RemoveRoute`로 동적 Writer 라우팅을 지원한다
- A5: Logger Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 생명주기 로직을 재사용한다
- A6: Bridge Node 연동 시 요청/응답 패턴은 `pkg/message/Message`의 메타데이터에 연산 유형을 포함하여 전달한다
- A7: 구독(Subscribe) 시 `StreamRouter.AddRoute`로 Writer를 등록하며, 구독 ID는 UUID 또는 고유 문자열로 생성한다
- A8: 하나의 LoggerAgent 인스턴스가 전역으로 공유되며, 컴포넌트명으로 개별 로거를 식별한다

### 2.2 도메인 가정

- A9: Logger Agent는 시스템 시작 시 자동 활성화되며, 외부 연결 설정이 불필요하다
- A10: 기본 로그 레벨은 `slog.LevelInfo`이며, 런타임에 컴포넌트별로 변경 가능하다
- A11: 기본 로그 포맷은 "json"이며, "text" 포맷도 지원한다 (Observer 생성 시 결정)
- A12: Pause 상태에서 Debug/Info 로그를 억제하는 것은 노이즈 감소 목적이며, Warn/Error는 시스템 안전을 위해 항상 기록한다
- A13: Logger Agent의 Graceful Shutdown 시, 모든 구독을 해제하고 Observer 참조를 정리한다
- A14: Logger Agent의 HealthCheck는 Observer 유효성과 Lifecycle 상태를 기반으로 healthy/degraded/unhealthy를 판별한다

### 2.3 v2.0.0 추가 가정

- A15: ConsoleLoggerAgent의 `Process(data []byte)` 파라미터 `data`는 플로우 엔진에서 전달하는 직렬화된 바이트이다. JSON 구조를 가진 경우 `payload` 필드를 추출할 수 있다.
- A16: content_mode의 기본값은 "full"이며, 이는 v1.0.0의 기존 동작과 동일하다 (하위 호환성 보장)
- A17: "binary" 포맷은 slog 핸들러를 거치지 않고, 직접 hex dump 형태로 writer에 출력한다
- A18: hex dump 포맷은 `hexdump -C`와 유사한 형식 (오프셋 | hex 바이트 | ASCII 문자)을 따른다
- A19: content_mode="payload"일 때 JSON 파싱 실패 시 원본 데이터를 그대로 출력한다 (graceful fallback)

---

## 3. Requirements (요구사항)

### Module 1: Logger Interface - 로거 인터페이스 (P0) [v1.0.0 완료]

#### REQ-LOG-001-01-01 (Ubiquitous) Logger 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Logger` 인터페이스를 제공해야 한다:

- `WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error` - 지정 컴포넌트로 로그 기록
- `SetLevel(ctx context.Context, component string, level slog.Level) error` - 컴포넌트별 로그 레벨 설정
- `SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)` - 패턴 매칭 레벨 일괄 설정
- `GetLevel(ctx context.Context, component string) (slog.Level, error)` - 컴포넌트 로그 레벨 조회
- `Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)` - 컴포넌트 로그 스트림 구독 (구독 ID 반환)
- `Unsubscribe(ctx context.Context, subscriptionID string) error` - 구독 해제
- `Components(ctx context.Context) ([]string, error)` - 등록된 컴포넌트 목록 조회

#### REQ-LOG-001-01-02 (Event-Driven) WriteLog 호출 시 Observer 위임

**WHEN** `Logger.WriteLog(ctx, component, level, msg, args...)` 호출 시, **THEN** Observer의 LoggerFactory에서 해당 컴포넌트의 ComponentLogger를 획득하고, 지정된 레벨의 로그 메서드(Debug/Info/Warn/Error)를 호출해야 한다.

#### REQ-LOG-001-01-03 (Event-Driven) SetLevel 호출 시 LevelManager 위임

**WHEN** `Logger.SetLevel(ctx, component, level)` 호출 시, **THEN** Observer의 LevelManager.SetLevel(component, level)을 호출하여 해당 컴포넌트의 로그 레벨을 즉시 변경해야 한다.

#### REQ-LOG-001-01-04 (Event-Driven) SetLevelByPattern 호출 시 패턴 매칭 위임

**WHEN** `Logger.SetLevelByPattern(ctx, pattern, level)` 호출 시, **THEN** Observer의 LevelManager.SetLevelByPattern(pattern, level)을 호출하여 패턴에 매칭되는 모든 컴포넌트의 레벨을 변경하고, 변경된 컴포넌트 수를 반환해야 한다.

#### REQ-LOG-001-01-05 (Event-Driven) Subscribe 호출 시 StreamRouter 라우트 추가

**WHEN** `Logger.Subscribe(ctx, component, writer)` 호출 시, **THEN** Observer의 StreamRouter.AddRoute(component, writer)를 호출하여 해당 컴포넌트의 로그 출력을 추가 Writer로 라우팅하고, 고유한 구독 ID를 반환해야 한다.

#### REQ-LOG-001-01-06 (Event-Driven) Unsubscribe 호출 시 StreamRouter 라우트 제거

**WHEN** `Logger.Unsubscribe(ctx, subscriptionID)` 호출 시, **THEN** 해당 구독 ID에 연결된 Writer를 StreamRouter.RemoveRoute로 제거해야 한다. 존재하지 않는 구독 ID에 대해서는 `ErrSubscriptionNotFound`를 반환해야 한다.

#### REQ-LOG-001-01-07 (Event-Driven) Components 호출 시 LoggerFactory 위임

**WHEN** `Logger.Components(ctx)` 호출 시, **THEN** Observer의 LoggerFactory.Components()를 호출하여 등록된 모든 컴포넌트 이름의 정렬된 목록을 반환해야 한다.

---

### Module 2: LoggerAgent Lifecycle - 로거 에이전트 생명주기 (P0) [v1.0.0 완료]

#### REQ-LOG-001-02-01 (Ubiquitous) LoggerAgent 구조체

시스템은 **항상** `LoggerAgent` 구조체를 제공해야 한다. `LoggerAgent`는 다음 인터페이스를 구현한다:

- `pkg/lifecycle/Lifecycle` - 생명주기 관리 (Init, Start, Pause, Resume, Stop, State)
- `pkg/lifecycle/HealthChecker` - 헬스 체크 (HealthCheck)

#### REQ-LOG-001-02-02 (Ubiquitous) BaseLifecycle 임베딩

시스템은 **항상** `LoggerAgent`가 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 전이 로직을 재사용해야 한다.

#### REQ-LOG-001-02-03 (Event-Driven) Init 시 Observer 초기화

**WHEN** `LoggerAgent.Init(ctx)` 호출 시, **THEN** 옵션에 따라 Observer를 생성(또는 외부 주입된 Observer 사용)하고, 구독 맵을 초기화하고, Running 상태로 전이해야 한다.

#### REQ-LOG-001-02-04 (Event-Driven) Stop 시 Graceful Shutdown

**WHEN** `LoggerAgent.Stop(ctx)` 호출 시, **THEN** 모든 활성 구독을 해제(StreamRouter.RemoveRoute)하고, 구독 맵을 비우고, Observer 참조를 정리한 뒤, Stopped 상태로 전이해야 한다.

#### REQ-LOG-001-02-05 (Event-Driven) Pause 시 Debug/Info 억제

**WHEN** `LoggerAgent.Pause(ctx)` 호출 시, **THEN** Debug 및 Info 레벨 로그의 WriteLog 호출을 억제(nil 반환)해야 한다. Warn 및 Error 레벨 로그는 정상적으로 기록해야 한다.

#### REQ-LOG-001-02-06 (Event-Driven) Resume 시 전체 레벨 복원

**WHEN** `LoggerAgent.Resume(ctx)` 호출 시, **THEN** Pause 상태에서 억제되었던 Debug/Info 레벨 로그를 다시 허용하여 전체 레벨로 복원해야 한다.

#### REQ-LOG-001-02-07 (Ubiquitous) 자동 활성화

시스템은 **항상** 시스템 시작 시 LoggerAgent를 자동으로 생성하고 Init/Start하여 활성화해야 한다. 별도의 사용자 등록이 불필요하다.

#### REQ-LOG-001-02-08 (Ubiquitous) HealthCheck 구현

시스템은 **항상** `LoggerAgent.HealthCheck(ctx)` 호출 시 다음을 확인하여 `HealthStatus`를 반환해야 한다:

- Observer가 nil이 아닌지 확인
- Lifecycle 상태: Running = "healthy", Paused = "degraded", 기타 = "unhealthy"
- Details에 활성 구독 수, 등록된 컴포넌트 수 포함

---

### Module 3: Options System - 옵션 시스템 (P0) [v1.0.0 완료]

#### REQ-LOG-001-03-01 (Ubiquitous) LoggerOption 함수 타입

시스템은 **항상** `LoggerOption func(*loggerConfig)` 함수 옵션 타입을 제공하고, `NewLoggerAgent(opts ...LoggerOption)` 생성자를 지원해야 한다.

#### REQ-LOG-001-03-02 (Ubiquitous) WithLogDefaultLevel 옵션

시스템은 **항상** `WithLogDefaultLevel(level slog.Level) LoggerOption`을 제공하여, Observer 생성 시 기본 로그 레벨을 설정할 수 있어야 한다. 기본값은 `slog.LevelInfo`이다.

#### REQ-LOG-001-03-03 (Ubiquitous) WithLogFormat 옵션

시스템은 **항상** `WithLogFormat(format string) LoggerOption`을 제공하여, 로그 출력 포맷을 "json" 또는 "text"로 설정할 수 있어야 한다. 기본값은 "json"이다.

#### REQ-LOG-001-03-04 (Ubiquitous) WithLogWriter 옵션

시스템은 **항상** `WithLogWriter(writer io.Writer) LoggerOption`을 제공하여, 기본 로그 출력 대상을 설정할 수 있어야 한다. 기본값은 `os.Stdout`이다.

#### REQ-LOG-001-03-05 (Ubiquitous) WithLogObserver 옵션

시스템은 **항상** `WithLogObserver(observer *observe.Observer) LoggerOption`을 제공하여, 외부에서 생성된 Observer를 주입할 수 있어야 한다. 주입된 Observer가 있으면 Init 시 새로 생성하지 않고 주입된 것을 사용한다.

---

### Module 4: Bridge Node Integration - 브릿지 노드 연동 (P1) [v1.0.0 완료]

#### REQ-LOG-001-04-01 (Optional) Bridge Node 메시지 기반 로깅 제어

**가능하면** Logger Agent는 Bridge Node를 통해 메시지 기반으로 접근할 수 있어야 한다. 메시지의 메타데이터에 연산 유형을 포함한다:

- `logger.operation`: `write`, `set_level`, `set_level_pattern`, `get_level`, `subscribe`, `unsubscribe`, `components`
- `logger.component`: 대상 컴포넌트 이름
- `logger.level`: 로그 레벨 문자열 ("debug", "info", "warn", "error")
- `logger.message`: 로그 메시지 텍스트 (write 연산 시)
- `logger.pattern`: 패턴 문자열 (set_level_pattern 연산 시)

응답 메시지:
- `logger.status`: "ok" 또는 "error"
- `logger.error`: 에러 메시지 (status가 "error"일 때)

#### REQ-LOG-001-04-02 (Event-Driven) Write 메시지 처리

**WHEN** Bridge Node를 통해 `write` 연산 메시지를 수신하면, **THEN** 메시지에서 component, level, message를 추출하여 `WriteLog`를 호출하고, 결과를 응답 메시지로 반환해야 한다.

#### REQ-LOG-001-04-03 (Event-Driven) SetLevel 메시지 처리

**WHEN** Bridge Node를 통해 `set_level` 연산 메시지를 수신하면, **THEN** 메시지에서 component와 level을 추출하여 `SetLevel`을 호출하고, 결과를 응답 메시지로 반환해야 한다.

#### REQ-LOG-001-04-04 (Event-Driven) GetLevel 요청-응답

**WHEN** Bridge Node를 통해 `get_level` 연산 메시지를 수신하면, **THEN** 해당 컴포넌트의 로그 레벨을 조회하여 응답 메시지의 Payload에 레벨 문자열을 포함하여 반환해야 한다.

#### REQ-LOG-001-04-05 (Event-Driven) Components 요청-응답

**WHEN** Bridge Node를 통해 `components` 연산 메시지를 수신하면, **THEN** 등록된 컴포넌트 목록을 조회하여 응답 메시지의 Payload에 문자열 배열로 포함하여 반환해야 한다.

---

### Module 5: Error Types - 에러 타입 (P0) [v1.0.0 완료]

#### REQ-LOG-001-05-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrLoggerClosed` | 중지된 Logger Agent에 대한 연산 시도 시 |
| `ErrLoggerPaused` | 일시정지된 Logger Agent에 대한 Debug/Info WriteLog 시도 시 |
| `ErrInvalidLevel` | 유효하지 않은 로그 레벨 지정 시 |
| `ErrInvalidComponent` | 빈 문자열 등 유효하지 않은 컴포넌트 이름 지정 시 |
| `ErrSubscriptionNotFound` | 존재하지 않는 구독 ID로 Unsubscribe 시도 시 |

#### REQ-LOG-001-05-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

---

### Module 6: Message Content Mode - 메시지 콘텐츠 모드 (P0) [v2.0.0 신규]

#### REQ-LOG-001-06-01 (Ubiquitous) content_mode 설정 필드

시스템은 **항상** `ConsoleLoggerConfig`에 `ContentMode string` 필드를 제공해야 한다. 유효한 값은 다음과 같다:

| 값 | 동작 | 설명 |
|----|------|------|
| `"full"` (기본값) | 수신한 `data` 바이트 전체를 출력 | v1.0.0 기존 동작과 동일 |
| `"payload"` | 메시지에서 payload 부분만 추출하여 출력 | JSON의 `payload` 필드 추출 |

#### REQ-LOG-001-06-02 (Event-Driven) content_mode 파싱

**WHEN** `parseConsoleLoggerConfig(cfg)`가 호출될 때, **THEN** `opts["content_mode"]` 값을 읽어 `ConsoleLoggerConfig.ContentMode`에 설정해야 한다. 미지정 또는 빈 문자열일 경우 기본값 `"full"`을 사용해야 한다.

#### REQ-LOG-001-06-03 (Event-Driven) content_mode="full" 동작

**WHEN** content_mode가 "full"이고 `Process(data)` 또는 `PublishMessage(topic, ..., payload)`가 호출될 때, **THEN** 수신한 데이터 전체를 출력 대상에 기록해야 한다. 이는 v1.0.0의 기존 동작과 완전히 동일하다.

#### REQ-LOG-001-06-04 (Event-Driven) content_mode="payload" 동작 -- JSON 메시지

**WHEN** content_mode가 "payload"이고 수신 데이터가 유효한 JSON이며 `"payload"` 키가 존재할 때, **THEN** `"payload"` 키의 값만 추출하여 출력해야 한다. payload 값이 문자열이면 문자열 그대로, 객체/배열이면 JSON 직렬화하여 출력한다.

#### REQ-LOG-001-06-05 (Unwanted) content_mode="payload" 동작 -- 비JSON 또는 payload 키 미존재

시스템은 content_mode="payload"일 때 JSON 파싱 실패 또는 `"payload"` 키가 없는 경우에도 **에러를 발생시키지 않아야 한다**. 대신 원본 데이터 전체를 그대로 출력해야 한다 (graceful fallback).

#### REQ-LOG-001-06-06 (Event-Driven) Configure 시 content_mode 반영

**WHEN** `Configure(config)` 호출로 에이전트 설정이 변경될 때, **THEN** 새 설정의 content_mode 값이 즉시 반영되어 이후 Process/PublishMessage 호출에 적용되어야 한다.

---

### Module 7: Binary Output Format - 바이너리 출력 포맷 (P0) [v2.0.0 신규]

#### REQ-LOG-001-07-01 (Ubiquitous) format="binary" 옵션

시스템은 **항상** ConsoleLoggerAgent의 `format` 설정에 `"binary"` 옵션을 지원해야 한다. 기존 옵션("text", "json")과 함께 3가지 포맷을 지원한다:

| 포맷 | 출력 방식 | 용도 |
|------|----------|------|
| `"text"` | slog TextHandler 통한 구조화 로그 | 사람이 읽기 쉬운 일반 로그 |
| `"json"` | slog JSONHandler 통한 JSON 로그 | 기계 처리용 구조화 로그 |
| `"binary"` | hex dump (오프셋 + 16진수 바이트 + ASCII) | 바이너리 프로토콜 디버깅 |

#### REQ-LOG-001-07-02 (Event-Driven) binary 포맷 출력 형식

**WHEN** format="binary"이고 데이터가 출력될 때, **THEN** 다음 형식으로 hex dump를 생성하여 writer에 기록해야 한다:

```
[prefix] 00000000  48 65 6c 6c 6f 20 57 6f  72 6c 64 21 0a 00 ff fe  |Hello World!....|
[prefix] 00000010  01 02 03 04                                       |....|
```

각 행의 구성:
- `[prefix]`: 설정된 prefix 문자열 (기본: "[logger]")
- `00000000`: 8자리 16진수 오프셋
- `48 65 6c 6c ...`: 최대 16바이트의 16진수 값 (8바이트씩 두 그룹으로 구분)
- `|Hello World!....|`: ASCII 표현 (출력 불가 문자는 `.`으로 대체, 0x20~0x7e 범위만 표시)

#### REQ-LOG-001-07-03 (Event-Driven) binary 포맷에서의 slog 우회

**WHEN** format="binary"일 때, **THEN** Process/PublishMessage는 slog 핸들러를 거치지 않고 직접 hex dump 문자열을 writer에 기록해야 한다. slog의 TextHandler/JSONHandler는 binary 포맷에 적합하지 않으므로 우회한다.

#### REQ-LOG-001-07-04 (Event-Driven) binary 포맷 + content_mode 조합

**WHEN** format="binary"이고 content_mode가 설정되어 있을 때, **THEN** 먼저 content_mode에 따라 출력 대상 데이터를 결정한 뒤, 해당 데이터에 대해 hex dump를 생성해야 한다. 즉 content_mode 적용이 format 적용보다 선행한다.

#### REQ-LOG-001-07-05 (Unwanted) binary 포맷에서 빈 데이터

시스템은 format="binary"일 때 빈 데이터(`[]byte{}` 또는 `nil`)에 대해 **패닉을 발생시키지 않아야 한다**. 빈 데이터인 경우 아무것도 출력하지 않거나 빈 hex dump 헤더만 출력한다.

#### REQ-LOG-001-07-06 (Event-Driven) binary 포맷의 PublishMessage (topic=filepath)

**WHEN** format="binary"이고 `PublishMessage(topic=filepath, ...)`가 호출될 때, **THEN** hex dump 결과를 해당 파일에 기록해야 한다. 파일 기록 시에도 동일한 hex dump 포맷을 사용한다.

#### REQ-LOG-001-07-07 (Event-Driven) binary 포맷의 createLogger 초기화

**WHEN** format="binary"로 ConsoleLoggerAgent가 초기화될 때, **THEN** slog.Logger를 TextHandler 기반으로 생성하되 (info 메시지 로깅용), hex dump 출력은 별도 경로로 처리해야 한다. 에이전트의 메타 로깅(시작/종료 등)은 text 포맷 slog를 통해 정상 출력된다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/agent/system/
  logger_errors.go       # 센티넬 에러 정의
  logger.go              # LoggerAgent 구조체, Logger 인터페이스
  logger_options.go      # LoggerOption 함수 타입 및 옵션 함수
  logger_bridge.go       # Bridge Node 메시지 핸들러

  console_logger.go      # ConsoleLoggerAgent 구조체 [v2.0.0 수정 대상]
  console_logger_test.go # ConsoleLoggerAgent 테스트 [v2.0.0 수정 대상]

  logger_test.go         # LoggerAgent 통합 테스트
  logger_bridge_test.go  # Bridge Node 연동 테스트
  logger_options_test.go # Options 단위 테스트

web/src/pages/agents/
  agentTypeMeta.ts       # [v2.0.0 수정 대상] logger 설정 필드 추가

web/src/config/
  agentSchemas.ts        # [v2.0.0 수정 대상] CONSOLE_LOGGER_FIELDS 스키마 추가
```

### 4.2 v1.0.0 타입 시그니처 (변경 없음)

```go
// Logger 는 로깅 시스템 에이전트 인터페이스이다.
type Logger interface {
    WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error
    SetLevel(ctx context.Context, component string, level slog.Level) error
    SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)
    GetLevel(ctx context.Context, component string) (slog.Level, error)
    Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)
    Unsubscribe(ctx context.Context, subscriptionID string) error
    Components(ctx context.Context) ([]string, error)
}

// LoggerAgent 는 Logger System Agent이다.
type LoggerAgent struct {
    *lifecycle.BaseLifecycle
    observer *observe.Observer
    config   loggerConfig
    subs     map[string]*subscription
    mu       sync.RWMutex
    paused   bool
    closed   bool
}

// 에러 변수
var (
    ErrLoggerClosed        = errors.New("logger: logger agent is closed")
    ErrLoggerPaused        = errors.New("logger: logger agent is paused (debug/info suppressed)")
    ErrInvalidLevel        = errors.New("logger: invalid log level")
    ErrInvalidComponent    = errors.New("logger: invalid component name")
    ErrSubscriptionNotFound = errors.New("logger: subscription not found")
)
```

### 4.3 v2.0.0 변경 사항 -- ConsoleLoggerConfig 확장

```go
// ConsoleLoggerConfig 는 ConsoleLoggerAgent의 설정을 담는 구조체이다.
type ConsoleLoggerConfig struct {
    Prefix      string     // 로그 출력 접두어 (기본: "[logger]")
    Level       slog.Level // 최소 로그 레벨 (기본: INFO)
    Output      string     // 출력 대상: "stdout"(기본), "stderr", 또는 파일 경로
    Format      string     // 출력 형식: "text"(기본), "json", "binary" [v2.0.0 확장]
    ContentMode string     // 출력 콘텐츠 모드: "full"(기본), "payload" [v2.0.0 신규]
    MaxSize     int64      // 파일 롤링 최대 크기(바이트)
    MaxAge      int        // 백업 파일 최대 보관 일수
    MaxBackups  int        // 백업 파일 최대 개수
    Compress    bool       // 백업 파일 gzip 압축 여부
}
```

### 4.4 v2.0.0 핵심 함수 설계

#### extractContent -- 콘텐츠 모드 적용

```go
// extractContent 는 content_mode에 따라 출력할 데이터를 결정한다.
// content_mode="full": 원본 데이터 그대로 반환
// content_mode="payload": JSON의 "payload" 필드 추출, 실패 시 원본 반환
func (a *ConsoleLoggerAgent) extractContent(data []byte) []byte
```

#### formatHexDump -- hex dump 생성

```go
// formatHexDump 는 바이트 슬라이스를 hexdump -C 형식의 문자열로 변환한다.
// 각 행: [prefix] OFFSET  HH HH HH HH HH HH HH HH  HH HH HH HH HH HH HH HH  |ASCII...........|
func (a *ConsoleLoggerAgent) formatHexDump(data []byte) string
```

#### Process 수정 (의사 코드)

```
func Process(data []byte):
    stats.IncrMessagesReceived()
    content = extractContent(data)

    if format == "binary":
        dump = formatHexDump(content)
        writer.Write(dump)
    else:
        logger.Info("message received", "prefix", prefix, "payload", string(content))

    stats.IncrMessagesSent()
    return nil, nil
```

#### PublishMessage 수정 (의사 코드)

```
func PublishMessage(topic, _, _, payload):
    stats.IncrMessagesReceived()
    content = extractContent(payload)

    if topic == "":
        if format == "binary":
            dump = formatHexDump(content)
            writer.Write(dump)
        else:
            logger.Info("message received", "prefix", prefix, "payload", string(content))
    else:
        fw = getOrCreateFileWriter(topic)
        if format == "binary":
            dump = formatHexDump(content)
            fw.writer.Write(dump)
        else:
            fw.writer.Write(content + "\n")

    stats.IncrMessagesSent()
    return nil
```

### 4.5 v2.0.0 Web UI 변경

#### agentTypeMeta.ts -- logger configFields 추가

기존 configFields에 다음을 추가:

```typescript
{ name: 'content_mode', type: 'select', required: false, description: '출력 콘텐츠 모드 (full=전체 메시지, payload=페이로드만)', default: 'full' },
```

기존 format 필드의 description 변경:

```typescript
{ name: 'format', type: 'select', required: false, description: '출력 형식 (text/json/binary)', default: 'text' },
```

#### agentSchemas.ts -- CONSOLE_LOGGER_FIELDS 추가

기존 format 필드의 options에 'binary' 추가:

```typescript
{ name: 'format', type: 'select', label: '출력 형식', options: ['text', 'json', 'binary'], default: 'text' },
```

content_mode 필드 추가:

```typescript
{ name: 'content_mode', type: 'select', label: '콘텐츠 모드', options: ['full', 'payload'], default: 'full', description: 'full=전체 메시지, payload=페이로드만 추출' },
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 | 버전 |
|------------|------|------|---------|------|
| REQ-LOG-001-01-01 ~ 01-07 | Logger Interface | logger.go | P0 | v1.0.0 (완료) |
| REQ-LOG-001-02-01 ~ 02-08 | LoggerAgent Lifecycle | logger.go | P0 | v1.0.0 (완료) |
| REQ-LOG-001-03-01 ~ 03-05 | Options System | logger_options.go | P0 | v1.0.0 (완료) |
| REQ-LOG-001-04-01 ~ 04-05 | Bridge Node Integration | logger_bridge.go | P1 | v1.0.0 (완료) |
| REQ-LOG-001-05-01 ~ 05-02 | Error Types | logger_errors.go | P0 | v1.0.0 (완료) |
| REQ-LOG-001-06-01 ~ 06-06 | Message Content Mode | console_logger.go | P0 | v2.0.0 (신규) |
| REQ-LOG-001-07-01 ~ 07-07 | Binary Output Format | console_logger.go | P0 | v2.0.0 (신규) |
| -- | Web UI | agentTypeMeta.ts, agentSchemas.ts | P1 | v2.0.0 (신규) |
