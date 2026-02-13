# SPEC-CFG-001: 구현 계획

> TAG: SPEC-CFG-001
> 상태: Planned
> 개발 방법론: Hybrid (TDD for new code)

---

## 1. 구현 개요

- **SPEC-CFG-001**: Viper 기반 다중 소스 설정 관리 및 런타임 핫 리로드
- **패키지**: `internal/config/`
- **개발 방법론**: Hybrid (TDD for new code)
- **의존성**: `github.com/spf13/viper` v1.18+, `github.com/spf13/cobra` v1.8+, `internal/observe` (선택적)

---

## 2. 구현 파일 및 순서

| 단계 | 파일 | 내용 | 예상 라인 수 |
|------|------|------|-------------|
| 1 | `internal/config/errors.go` | 에러 변수 정의 (`ErrInvalidPort`, `ErrImmutableKey` 등), `ValidationErrors` 타입 | ~80 |
| 2 | `internal/config/types.go` | `ServerConfig`, `EngineConfig`, `StorageConfig`, `AuthConfig`, `ObserveConfig`, `ScriptConfig`, `PluginConfig` 구조체 | ~150 |
| 3 | `internal/config/mutable.go` | Mutable/Immutable 키 레지스트리, `IsMutable(key) bool`, `IsImmutable(key) bool` | ~80 |
| 4 | `internal/config/defaults.go` | `SetDefaults(v *viper.Viper)` 기본값 설정 함수 | ~100 |
| 5 | `internal/config/validate.go` | `Validate(cfg Config) error`, 개별 검증 함수 (포트, 경로, 필수값, Duration 등) | ~200 |
| 6 | `internal/config/hotreload.go` | `OnChange`, `Set`, `WatchConfig`, `StopWatch`, `ChangeEvent`, `ChangeHistory` | ~250 |
| 7 | `internal/config/config.go` | `Config` 인터페이스, `Configurable` 인터페이스, `viperConfig` 구현, `Load()`, `LoadOption` | ~300 |
| 8 | `internal/config/config_test.go` | 전체 단위/통합 테스트 (table-driven) | ~500 |

**총 예상: ~1,660 lines**

---

## 3. 기술 스택

- **Go 1.23+**
- **핵심 의존성**:
  - `github.com/spf13/viper` v1.18+ - 다중 소스 설정 관리, 파일 감시
  - `github.com/spf13/cobra` v1.8+ - CLI 플래그 통합 (`pflag.FlagSet` 바인딩)
  - `github.com/spf13/pflag` - Cobra가 사용하는 플래그 라이브러리
- **표준 라이브러리**: `sync`, `time`, `fmt`, `errors`, `os`, `path/filepath`, `log/slog`
- **테스트**: `testing`, `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`
- **내부 의존성**: `internal/observe` (선택적, 로깅 연동)

---

## 4. 설계 결정사항

### Viper 감싸기 전략

- Viper 인스턴스를 `viperConfig` 내부에 캡슐화
- 외부에 Viper API를 직접 노출하지 않음 (의존성 역전)
- 타입 안전한 카테고리별 접근자(`Server()`, `Engine()` 등)로 설정 제공
- Viper의 `Get(key)` 대신 구조화된 구조체를 반환하여 컴파일 타임 타입 안전성 확보

### Mutable/Immutable 분류

- 키별 Mutable/Immutable을 `map[string]bool`로 레지스트리 관리
- 와일드카드 패턴 지원 (예: `server.tls.*`는 TLS 하위 모든 키를 Immutable로 분류)
- `Set()` 호출 시 레지스트리를 먼저 확인하여 Immutable 키 변경을 차단
- 향후 커스텀 Mutable/Immutable 분류 확장 가능

### 콜백 관리

- `map[string][]ChangeCallback` 구조로 키별 콜백 목록 관리
- `sync.RWMutex`로 콜백 등록/해제/호출의 동시성 안전 보장
- 콜백은 별도 고루틴에서 비동기 실행 (호출자 차단 방지)
- 콜백 내 panic은 `recover()`로 처리, 로그 기록 후 다음 콜백 계속 실행

### 카테고리 구조체 vs Viper 직접 접근

- `Server()`, `Engine()` 등은 호출 시점에 Viper에서 값을 읽어 구조체를 생성
- 캐싱하지 않음 (항상 최신 값 반환, 핫 리로드 호환)
- `sync.RWMutex.RLock()`으로 읽기 보호

### 설정 파일 탐색 순서

1. `--config` CLI 플래그로 지정된 경로 (최우선)
2. `./xflow.yaml` (현재 디렉토리)
3. `$HOME/.xflow/xflow.yaml` (사용자 홈)
4. `/etc/xflow/xflow.yaml` (시스템 전역)

---

## 5. 마일스톤

| 마일스톤 | 내용 | 완료 기준 |
|---------|------|----------|
| **M1** | 에러 정의 + 카테고리 구조체 + Mutable 레지스트리 | `errors.go`, `types.go`, `mutable.go` 작성, 컴파일 성공, 에러 테스트 통과 |
| **M2** | 기본값 + 유효성 검증 | `defaults.go`, `validate.go` 작성, 기본값 설정 테스트, 유효/무효 입력 검증 테스트 통과 |
| **M3** | Config 인터페이스 + Load 함수 + 다중 소스 로딩 | `config.go` 작성, YAML/환경변수/플래그 통합 로딩 테스트 통과, 우선순위 검증 |
| **M4** | 핫 리로드 + 콜백 + 변경 이력 | `hotreload.go` 작성, `Set()` + `OnChange()` + `WatchConfig()` 테스트 통과, Mutable/Immutable 구분 검증 |
| **M5** | 동시성 테스트 + 통합 테스트 + 벤치마크 | 커버리지 85%+, `go test -race` 통과, 벤치마크 기록, `internal/observe` 연동 테스트 |

---

## 6. 리스크 및 대응

| 리스크 | 심각도 | 대응 |
|--------|--------|------|
| Viper `WatchConfig()`의 fsnotify 이벤트 중복 발사 | High | debounce 로직 추가 (100ms 내 중복 이벤트 무시), 테스트에서 시간 기반 검증 |
| 환경 변수 자동 바인딩 시 키 이름 충돌 | Medium | `XFLOW_` 접두사 강제, `AutomaticEnv()` + `SetEnvKeyReplacer()` 사용 |
| 콜백 내 무한 루프/데드락 | Medium | 콜백 타임아웃(5초) 설정, 별도 고루틴에서 실행, `context.WithTimeout` 적용 |
| 대규모 설정 파일에서 파싱 지연 | Low | 기본 설정 파일 크기 제한, 벤치마크로 성능 검증 |
| `internal/observe` 초기화 전 로깅 불가 | Low | `slog.Default()` 폴백 사용, `WithLogger` 옵션으로 명시적 로거 주입 |

---

## 7. 테스트 전략

- **TDD** (RED-GREEN-REFACTOR) 적용 (새 코드)
- **Table-driven tests** 사용
- `go test -race` 필수 (동시성 검증)
- **테스트 헬퍼**: 임시 YAML 파일 생성/정리를 위한 `testutil` 함수 작성
- **벤치마크 테스트**: `Load()`, `Set()`, `Server()` (읽기), 동시 읽기/쓰기
- **커버리지 목표**: 85%+

### 테스트 케이스 분류

| 분류 | 테스트 대상 | 테스트 유형 |
|------|-----------|------------|
| 기본값 | `SetDefaults()` 호출 후 모든 기본값 확인 | 단위 |
| 유효성 검증 | 유효/무효 설정 입력별 에러 확인 | 단위 (table-driven) |
| 다중 소스 | YAML + 환경변수 + 플래그 우선순위 검증 | 통합 |
| 핫 리로드 | 파일 변경 감지, `Set()` 콜백 호출 | 통합 |
| Mutable/Immutable | Immutable 키 `Set()` 시 에러, Mutable 키 성공 | 단위 |
| 동시성 | 다중 고루틴 읽기/쓰기 경쟁 조건 | 단위 (`-race`) |
| 콜백 안전성 | 콜백 panic 시 복구, 구독 해제 후 미호출 | 단위 |
| 변경 이력 | FIFO 제한, 이력 조회 | 단위 |

---

## 8. 의존성 관계

**이 패키지에 의존하는 패키지:**

- `internal/engine` - 엔진 설정 (백프레셔, 실행 정책)
- `internal/node` - 노드 설정 (필터 조건, 변환 규칙)
- `internal/agent` - 에이전트 설정 (인증 정보, 프로토콜 파라미터)
- `internal/script` - 스크립트 엔진 설정 (타임아웃, VM 풀)
- `internal/plugin` - 플러그인 설정 (디렉토리, 활성화)
- `internal/api` - API 서버 설정 (CORS, 레이트 리밋)
- `internal/auth` - 인증 설정 (JWT, OAuth2)
- `internal/storage` - 저장소 설정 (DB 타입, 연결 문자열)
- `internal/observe` - 관찰성 설정 (로그 레벨, 메트릭)
- `internal/cli` - CLI 플래그 바인딩
- `cmd/xflowd` - 데몬 서버 설정 로딩
- `cmd/xflow` - CLI 도구 설정 로딩
- `cmd/xflow-agent` - 에이전트 설정 로딩

**이 패키지의 외부 의존성:**

- `github.com/spf13/viper` v1.18+
- `github.com/spf13/cobra` v1.8+ (pflag 바인딩)
- `internal/observe` (선택적, 로깅)

---

## 9. 전문가 상담 권장

이 SPEC의 구현 단계(`/moai:2-run`)에서 다음 전문가 상담을 권장한다:

| 전문가 | 상담 영역 | 이유 |
|--------|----------|------|
| expert-backend | Viper 통합 아키텍처, 동시성 패턴, Options Pattern 구현 | Go 백엔드 핵심 인프라 패키지 |
| expert-testing | 동시성 테스트 전략, 임시 파일 테스트 헬퍼 | `-race` 플래그 테스트, 파일 시스템 의존 테스트 |
