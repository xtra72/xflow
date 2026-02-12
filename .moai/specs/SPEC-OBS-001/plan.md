# SPEC-OBS-001: 구현 계획

> TAG: SPEC-OBS-001
> 상태: Planned
> 개발 방법론: Hybrid (TDD for new code)

---

## 1. 구현 개요

- **SPEC-OBS-001**: 컴포넌트별 구조화된 관찰성 시스템 (로깅, 메트릭, 트레이싱)
- **패키지**: `internal/observe/`
- **개발 방법론**: Hybrid (TDD for new code)
- **의존성**: `log/slog` (표준), `github.com/prometheus/client_golang` v1.18+
- **Tier**: Tier 2 - 횡단 관심사 (모든 `internal/` 패키지에서 임포트)

---

## 2. 구현 파일 및 순서

| 단계 | 파일 | 내용 | 예상 라인 수 |
|------|------|------|-------------|
| 1 | `internal/observe/options.go` | FactoryOption, MetricsOption, TracerOption, StreamOption 타입 및 설정 함수 | ~120 |
| 2 | `internal/observe/level.go` | LevelManager interface + levelManager + slog.LevelVar 관리 + 와일드카드 패턴 매칭 | ~180 |
| 3 | `internal/observe/stream.go` | StreamRouter interface + streamRouter + routingHandler (slog.Handler 구현) | ~200 |
| 4 | `internal/observe/logger.go` | ComponentLogger interface + componentLogger + LoggerFactory + NewLoggerFactory() | ~180 |
| 5 | `internal/observe/metrics.go` | MetricsCollector interface + metricsCollector + 사전 정의 메트릭 + Prometheus 통합 | ~220 |
| 6 | `internal/observe/trace.go` | Tracer interface + Span interface + tracer + noopSpan + 샘플링 로직 | ~200 |
| 7 | `internal/observe/observe.go` | Observer 통합 구조체 + New() 팩토리 | ~80 |
| 8 | `internal/observe/*_test.go` | 모듈별 단위 테스트 + 통합 테스트 + 벤치마크 | ~600 |

**총 예상: ~1,780 lines**

---

## 3. 기술 스택

- **Go 1.23+**
- **표준 라이브러리**: `log/slog`, `sync`, `time`, `io`, `strings`, `math/rand`
- **외부 의존성**: `github.com/prometheus/client_golang` v1.18+ (Prometheus 메트릭)

---

## 4. 설계 결정사항

### Interface-First 설계

- ComponentLogger, LoggerFactory, LevelManager, MetricsCollector, Tracer, Span, StreamRouter 모두 Go interface로 정의
- 기본 구현은 unexported struct (`componentLogger`, `loggerFactory`, `levelManager`, `metricsCollector`, `tracer`, `streamRouter`)
- 팩토리 함수가 interface를 반환
- 테스트 시 mock 구현 용이, 향후 확장(예: OpenTelemetry 통합) 가능

### slog.Handler 합성 패턴

- `StreamRouter`가 `slog.Handler` 인터페이스를 구현하는 `routingHandler`를 내부적으로 생성
- `routingHandler`는 로그 레코드의 `component` 속성을 읽어 적절한 Writer로 라우팅
- `LevelManager`와 연동하여 컴포넌트별 로그 레벨 필터링을 Handler 단에서 수행
- slog의 표준 패턴을 따르므로 기존 slog 생태계와 호환

### Registry 패턴

- `LoggerFactory`가 내부적으로 `sync.Map`으로 컴포넌트-로거 매핑을 관리
- `LevelManager`가 내부적으로 `sync.Map`으로 컴포넌트-LevelVar 매핑을 관리
- 중복 생성 방지 및 전역 조회 기능 제공
- `sync.Map` 사용으로 읽기 위주의 동시성 패턴에 최적화

### Noop 패턴 (제로 오버헤드)

- Tracer 비활성화 시 `noopSpan` 반환 (모든 메서드가 즉시 반환)
- `noopSpan`은 전역 싱글턴으로 할당 비용 없음
- Metrics 비활성화 옵션 시 noop Counter/Histogram/Gauge 반환 (향후 확장)

### 와일드카드 패턴 매칭

- `path.Match` 스타일이 아닌 커스텀 glob 매칭 구현
- `*`는 dot을 포함한 모든 문자와 매칭 (prefix 매칭)
- `agent.*` → `agent.` 접두사를 가진 모든 컴포넌트와 매칭
- `*` 단독 사용 시 모든 컴포넌트와 매칭

### Observer 통합 구조체

- `Observer`는 편의를 위한 통합 진입점
- 각 하위 시스템(LoggerFactory, LevelManager 등)은 독립적으로도 사용 가능
- DI(Dependency Injection) 패턴 지원: 각 인터페이스를 개별적으로 주입 가능

---

## 5. 마일스톤

| 마일스톤 | 내용 | 완료 기준 |
|---------|------|----------|
| **M1** | Options + LevelManager + StreamRouter | Options 타입 정의, LevelManager 인터페이스 구현 (slog.LevelVar, 와일드카드), StreamRouter 인터페이스 구현 (slog.Handler 합성), 단위 테스트 통과 |
| **M2** | LoggerFactory + ComponentLogger | LoggerFactory 구현, ComponentLogger 구현 (slog 래핑), Registry 패턴, LevelManager/StreamRouter 통합, 단위 테스트 통과 |
| **M3** | MetricsCollector + Prometheus 통합 | MetricsCollector 구현, 사전 정의 메트릭 등록, Prometheus Registry 통합, 중복 등록 방지, 단위 테스트 통과 |
| **M4** | Tracer + Span + 샘플링 | Tracer 구현, Span/noopSpan 구현, 샘플링 로직, 제로 오버헤드 검증, 단위 테스트 + 벤치마크 통과 |
| **M5** | Observer 통합 + 통합 테스트 | Observer 구조체, 전체 통합 테스트, `go test -race` 통과, 커버리지 85%+, 벤치마크 기록 |

---

## 6. 리스크 및 대응

| 리스크 | 심각도 | 대응 |
|--------|--------|------|
| slog.Handler 합성의 복잡도 증가 | High | routingHandler를 최소 기능으로 구현, slog.Handler 인터페이스 4개 메서드만 충실히 구현 |
| Prometheus 레이블 카디널리티 폭발 | High | 사전 등록 컴포넌트만 허용, 동적 레이블 생성 차단 |
| 와일드카드 패턴 매칭 성능 | Medium | 컴포넌트 수가 수백 개 이내이므로 선형 탐색으로 충분, 프로파일링 후 필요 시 트라이 구조 도입 |
| LevelManager 동시성 문제 | Medium | slog.LevelVar의 atomic 연산 활용, sync.Map으로 레지스트리 관리 |
| Tracer 메모리 누수 (Span 미종료) | Medium | Span에 최대 지속시간 설정, 주기적 미종료 Span 정리 고루틴 |
| StreamRouter의 Writer 에러 처리 | Low | 에러 발생 Writer 건너뛰기, 내부 에러 카운터 기록 |

---

## 7. 테스트 전략

- **TDD** (RED-GREEN-REFACTOR) 적용 (새 코드)
- **Table-driven tests** 사용
- `go test -race` 필수 (동시성 안전 검증 - LevelManager, MetricsCollector)
- **벤치마크 테스트**:
  - `NewLogger()`, `ComponentLogger.Info()` - 로거 생성 및 로그 출력 성능
  - `SetLevel()`, `SetLevelByPattern()` - 레벨 변경 성능
  - `Counter.Inc()`, `Histogram.Observe()` - 메트릭 기록 성능
  - `StartSpan()` (활성/비활성) - 트레이싱 오버헤드 측정
  - `StreamRouter.Handler().Handle()` - 로그 라우팅 성능
- **커버리지 목표**: 85%+
- **제로 오버헤드 검증**: 비활성 Tracer의 `StartSpan()`이 0 allocation인지 벤치마크로 확인

---

## 8. 의존성 관계

**이 패키지에 의존하는 패키지 (모든 internal/ 패키지):**

- `internal/engine` - Flow Engine 로깅/메트릭/트레이싱
- `internal/node` - Node 처리 로깅/메트릭
- `internal/agent` - Agent 연결/프로토콜 로깅/메트릭
- `internal/script` - Script Engine 실행 로깅/메트릭
- `internal/plugin` - Plugin 로드/실행 로깅/메트릭
- `internal/api` - API 요청/응답 로깅/메트릭
- `internal/auth` - 인증 이벤트 로깅
- `internal/config` - 설정 변경 로깅
- `internal/storage` - 저장소 연산 로깅

**이 패키지의 외부 의존성:**

- `github.com/prometheus/client_golang` v1.18+ (1개만)

**이 패키지가 의존하지 않는 패키지:**

- `pkg/message` (Tier 1 - 독립)
- `pkg/flow` (Tier 1 - 독립)
- 기타 `internal/` 패키지 (순환 의존성 방지)
