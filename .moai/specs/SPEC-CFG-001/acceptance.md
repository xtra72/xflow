# SPEC-CFG-001: Config System - 인수 테스트 기준

> TAG: SPEC-CFG-001
> Status: Planned
> Created: 2026-02-12

---

## AC-001: Config 기본 로딩 및 인터페이스 준수

**Given** `internal/config` 패키지가 임포트된 상태에서

**When** `config.Load()` 함수를 아무 옵션 없이 호출하면

**Then** Config 인터페이스를 만족하는 객체가 반환되어야 한다

- `Server()`는 기본 `ServerConfig`를 반환한다 (`Port: 8080`, `Host: "0.0.0.0"`)
- `Engine()`는 기본 `EngineConfig`를 반환한다 (`BackpressureThreshold: 1000`)
- `Storage()`는 기본 `StorageConfig`를 반환한다 (`Type: "sqlite"`)
- `Auth()`는 기본 `AuthConfig`를 반환한다
- `Observe()`는 기본 `ObserveConfig`를 반환한다 (`DefaultLevel: "info"`)
- `Script()`는 기본 `ScriptConfig`를 반환한다 (`VMPoolSize: 10`)
- `Plugin()`는 기본 `PluginConfig`를 반환한다

**검증 방법:**
- 모든 카테고리 접근자가 nil이 아닌 구조체를 반환하는지 확인
- 각 기본값이 `defaults.go`에 정의된 값과 일치하는지 확인

---

## AC-001-1: 바이너리별 기본 설정 경로 로딩

**Given** `$HOME/.xflow/xflowd.yaml` 경로에 다음 내용의 설정 파일이 존재하는 상태에서

```yaml
server:
  port: 9090
```

**When** `config.Load(WithConfigName("xflowd"), WithConfigPaths("./", "$HOME/.xflow/", "/etc/xflow/"))` 호출 시

**Then** 기본 경로의 설정 파일이 기본값을 오버라이드해야 한다

- `Server().Port`는 `9090`이어야 한다 (기본값 8080을 오버라이드)
- YAML에 지정되지 않은 값은 기본값을 유지한다 (`Engine().BackpressureThreshold == 1000`)

**추가 검증:**
- 각 바이너리별 설정 파일명 확인:
  - `WithConfigName("xflowd")` → `xflowd.yaml` 탐색
  - `WithConfigName("xflow")` → `xflow.yaml` 탐색
  - `WithConfigName("xflow-agent")` → `xflow-agent.yaml` 탐색
- 탐색 경로 우선순위: `./` → `$HOME/.xflow/` → `/etc/xflow/` (먼저 발견된 파일 사용)
- 기본 경로에 설정 파일이 없으면 에러 없이 기본값만 사용

**검증 방법:**
- 임시 디렉터리에 설정 파일 생성 후 `WithConfigPaths`로 전달
- 파일 정리는 `t.Cleanup()`으로 수행

---

## AC-002: YAML 설정 파일 로딩 (사용자 지정)

**Given** 다음 내용의 임시 YAML 파일이 생성된 상태에서

```yaml
server:
  port: 9090
  host: "127.0.0.1"
engine:
  backpressure_threshold: 2000
storage:
  type: "postgres"
  postgres:
    dsn: "postgres://user:pass@localhost/xflow"
```

**When** `config.Load(WithConfigName("xflowd"), WithConfigPaths("./", "$HOME/.xflow/"), WithConfigFile("/tmp/test-xflow.yaml"))` 호출 시

**Then** 사용자 지정 설정 파일의 값이 기본값과 기본 경로 설정 파일을 오버라이드해야 한다

- `Server().Port`는 `9090`이어야 한다
- `Server().Host`는 `"127.0.0.1"`이어야 한다
- `Engine().BackpressureThreshold`는 `2000`이어야 한다
- `Storage().Type`는 `"postgres"`이어야 한다
- `Storage().PostgresDSN`은 지정된 DSN이어야 한다
- YAML에 지정되지 않은 값은 기본값을 유지한다 (`Observe().DefaultLevel == "info"`)

**검증 방법:**
- 임시 파일 생성 후 `Load()` 호출, 값 비교
- 파일 정리(`os.Remove`)는 테스트 `t.Cleanup()`으로 수행

---

## AC-003: 환경 변수 오버라이드

**Given** 다음 환경 변수가 설정된 상태에서

```
XFLOW_SERVER_PORT=3000
XFLOW_ENGINE_BACKPRESSURE_THRESHOLD=5000
XFLOW_OBSERVE_DEFAULT_LEVEL=debug
```

**When** `config.Load()` 호출 시

**Then** 환경 변수 값이 기본값과 설정 파일 값을 오버라이드해야 한다

- `Server().Port`는 `3000`이어야 한다
- `Engine().BackpressureThreshold`는 `5000`이어야 한다
- `Observe().DefaultLevel`은 `"debug"`이어야 한다

**검증 방법:**
- `t.Setenv()`로 환경 변수 설정 (테스트 종료 시 자동 복원)
- 환경 변수 + YAML 파일 조합 시 환경 변수가 우선하는지 확인

---

## AC-004: 5단계 오버라이드 체인 전체 검증

**Given** 다음 5개 소스가 모두 `server.port`를 설정한 상태에서

| 우선순위 | 소스 | 설정값 |
|---------|------|-------|
| 5 (최하위) | 기본값 | `8080` |
| 4 | 기본 경로 설정 파일 (`$HOME/.xflow/xflowd.yaml`) | `9090` |
| 3 | 사용자 지정 설정 파일 (`--config custom.yaml`) | `7070` |
| 2 | 환경 변수 (`XFLOW_SERVER_PORT`) | `3000` |
| 1 (최우선) | CLI 플래그 (`--port`) | `4000` |

**When** `config.Load(WithConfigName("xflowd"), WithConfigPaths(defaultPaths...), WithConfigFile("custom.yaml"), WithFlags(flagSet))` 호출 시

**Then** CLI 플래그 값이 최우선으로 적용되어야 한다

- `Server().Port`는 `4000`이어야 한다

**단계별 검증:**
1. 아무 소스 없이 `Load()` → `Port == 8080` (기본값)
2. 기본 경로 파일만 → `Port == 9090` (기본값 < 기본 경로 파일)
3. 기본 경로 파일 + 사용자 지정 파일 → `Port == 7070` (기본 경로 파일 < 사용자 지정 파일)
4. 기본 경로 파일 + 사용자 지정 파일 + 환경 변수 → `Port == 3000` (사용자 지정 파일 < ENV)
5. 모든 소스 → `Port == 4000` (ENV < CLI)

**검증 방법:**
- `pflag.NewFlagSet()`으로 테스트용 플래그셋 생성
- `t.Setenv()`로 환경 변수 설정
- 임시 디렉터리에 기본 경로 파일과 사용자 지정 파일 각각 생성
- 5단계 각각을 독립된 서브테스트로 검증

---

## AC-005: 유효성 검증 - 유효한 설정

**Given** 모든 기본값이 유효한 상태에서

**When** `Validate(cfg)` 호출 시

**Then** `nil` 에러를 반환해야 한다 (기본값은 항상 유효)

**검증 방법:**
- `Load()` 직후 `Validate()` 호출하여 에러 없음 확인
- 프로덕션 모드 + JWT 시크릿 설정된 경우도 에러 없음 확인

---

## AC-006: 유효성 검증 - 포트 범위 에러

**Given** `server.port`가 `0` 또는 `70000`으로 설정된 상태에서

**When** `Validate(cfg)` 호출 시

**Then** `ErrInvalidPort`를 포함한 `ValidationErrors`를 반환해야 한다

**검증 방법:**
- `errors.Is(err, ErrInvalidPort)`가 `true`인지 확인
- `ValidationErrors`로 타입 단언하여 에러 목록에 포함 확인
- 유효한 포트 (`1`, `8080`, `65535`) 통과 확인

---

## AC-007: 유효성 검증 - 복수 에러 집계

**Given** `server.port=0`, `storage.type="mongodb"`, `observe.default_level="verbose"` 로 설정된 상태에서

**When** `Validate(cfg)` 호출 시

**Then** 3개의 에러를 집계한 `ValidationErrors`를 반환해야 한다

- `Errors()` 슬라이스 길이는 `3`이어야 한다
- `HasErrors()`는 `true`이어야 한다
- `Error()` 문자열에 3개 에러 메시지가 모두 포함되어야 한다
- 첫 번째 에러에서 중단하지 않고 전체 검증을 완료해야 한다

---

## AC-008: 유효성 검증 - TLS 인증서 파일 존재

**Given** `server.tls.enabled=true`이고, `server.tls.cert_file`이 존재하지 않는 경로인 상태에서

**When** `Validate(cfg)` 호출 시

**Then** `ErrFileNotFound`를 포함한 에러를 반환해야 한다

**추가 검증:**
- TLS 비활성화 시 파일 경로 검증을 건너뛰는지 확인
- `cert_file`과 `key_file` 모두 존재하지 않으면 2개 에러 반환

---

## AC-009: 유효성 검증 - 프로덕션 모드 JWT 필수

**Given** `server.mode=production`이고 `auth.jwt.secret=""`인 상태에서

**When** `Validate(cfg)` 호출 시

**Then** `ErrRequiredField`를 반환해야 한다

**추가 검증:**
- `server.mode=development`일 때는 JWT 시크릿이 비어 있어도 에러 없음
- JWT 시크릿이 설정된 프로덕션 모드에서는 에러 없음

---

## AC-010: Mutable 키 런타임 변경

**Given** Config가 로딩된 상태에서

**When** `cfg.Set("engine.backpressure_threshold", 5000)` 호출 시

**Then**
- 에러 없이 성공해야 한다
- `cfg.Engine().BackpressureThreshold`가 `5000`으로 변경되어야 한다
- 변경 이력에 해당 변경이 기록되어야 한다

**검증 방법:**
- `Set()` 반환값이 `nil`인지 확인
- 변경 후 `Engine()` 값 확인
- `ChangeHistory()` 마지막 항목의 Key, OldValue, NewValue 확인

---

## AC-011: Immutable 키 변경 차단

**Given** Config가 로딩된 상태에서

**When** `cfg.Set("server.port", 9090)` 호출 시

**Then** `ErrImmutableKey` 에러를 반환하고, 설정값은 변경되지 않아야 한다

**검증 방법:**
- `errors.Is(err, ErrImmutableKey)`가 `true`인지 확인
- `Server().Port`가 원래 값을 유지하는지 확인
- 다른 Immutable 키 (`server.tls.enabled`, `storage.type`, `script.vm_pool_size`) 에서도 동일 동작 확인

---

## AC-012: 변경 콜백 호출

**Given** Config가 로딩되고, `engine.backpressure_threshold` 키에 콜백이 등록된 상태에서

**When** `cfg.Set("engine.backpressure_threshold", 3000)` 호출 시

**Then** 등록된 콜백이 호출되어야 한다

- `ChangeEvent.Key`는 `"engine.backpressure_threshold"`이어야 한다
- `ChangeEvent.OldValue`는 이전 값이어야 한다
- `ChangeEvent.NewValue`는 `3000`이어야 한다
- `ChangeEvent.Timestamp`는 현재 시각 근처여야 한다
- `ChangeEvent.Source`는 `"api"`이어야 한다

**추가 검증:**
- 동일 키에 복수 콜백 등록 시 모두 호출되는지 확인
- 다른 키 변경 시 해당 키의 콜백은 호출되지 않는지 확인

---

## AC-013: 콜백 구독 해제

**Given** 콜백이 등록되고 `UnsubscribeFunc`이 반환된 상태에서

**When** `unsubscribe()` 호출 후 해당 키의 설정을 변경하면

**Then** 구독 해제된 콜백은 호출되지 않아야 한다

**검증 방법:**
- 콜백 호출 횟수를 `atomic.Int32`로 카운팅
- 구독 해제 전: `Set()` -> 카운터 증가 확인
- 구독 해제 후: `Set()` -> 카운터 미증가 확인

---

## AC-014: 콜백 panic 복구

**Given** panic을 발생시키는 콜백이 등록된 상태에서

**When** 해당 키의 설정을 변경하면

**Then**
- panic이 외부로 전파되지 않아야 한다
- `Set()` 메서드는 에러 없이 완료되어야 한다
- 동일 키에 등록된 다른 콜백은 정상 호출되어야 한다

**검증 방법:**
- panic 콜백 + 정상 콜백 2개를 동일 키에 등록
- `Set()` 호출 후 정상 콜백의 호출 여부 확인
- `Set()` 메서드가 panic하지 않음 확인

---

## AC-015: 변경 이력 FIFO 관리

**Given** Config가 로딩된 상태에서

**When** Mutable 키에 110회의 `Set()` 연산을 수행하면

**Then** `ChangeHistory()`는 최신 100건만 반환해야 한다 (기본 maxChangeHistory=100)

**검증 방법:**
- `len(ChangeHistory())`가 `100`인지 확인
- 첫 번째 항목이 11번째 변경에 해당하는지 확인 (1~10번째는 제거)
- 마지막 항목이 110번째 변경에 해당하는지 확인

---

## AC-016: 런타임 변경 시 유효성 검증

**Given** Config가 로딩된 상태에서

**When** `cfg.Set("observe.default_level", "invalid_level")` 호출 시

**Then** `ErrInvalidLogLevel` 에러를 반환하고, 설정값은 변경되지 않아야 한다

**추가 검증:**
- `cfg.Set("observe.default_level", "debug")`는 성공해야 한다
- 유효하지 않은 포트 범위 값으로 `Set()` 호출 시에도 차단 확인
- 검증 실패 시 콜백이 호출되지 않는지 확인
- 변경 이력에 실패한 변경은 기록되지 않는지 확인

---

## AC-017: 동시성 안전 - 읽기/쓰기 경쟁

**Given** Config가 로딩된 상태에서

**When** 10개 고루틴이 동시에 `Server()`를 읽고, 1개 고루틴이 `Set("server.rate_limit.requests_per_second", ...)` 를 반복 호출하면

**Then** `go test -race`에서 데이터 경쟁이 감지되지 않아야 한다

**검증 방법:**
- `sync.WaitGroup`으로 10개 읽기 + 1개 쓰기 고루틴 동시 실행
- 1000회 반복 수행
- `-race` 플래그로 경쟁 조건 검출

---

## AC-018: 파일 변경 감지 (WatchConfig)

**Given** YAML 설정 파일로 Config가 로딩되고 `WatchConfig()`가 호출된 상태에서

**When** YAML 파일의 `engine.backpressure_threshold` 값을 외부에서 수정하면

**Then** 변경이 감지되고, 등록된 콜백이 호출되어야 한다

**검증 방법:**
- 임시 YAML 파일 생성, `Load()` + `WatchConfig()` 호출
- 콜백 등록 후 파일 내용 수정 (`os.WriteFile`)
- 콜백 호출 대기 (최대 2초 타임아웃)
- `Engine().BackpressureThreshold`가 새 값으로 업데이트 확인

**주의사항:**
- fsnotify 이벤트 전달 지연을 감안하여 충분한 대기 시간 설정
- debounce 로직 확인 (100ms 이내 중복 이벤트 무시)

---

## AC-019: Configurable 인터페이스 호환

**Given** `Configurable` 인터페이스를 구현한 모의 구성 요소가 있는 상태에서

**When** `Configure(map[string]any{"threshold": 5000})` 호출 시

**Then** 구성 요소가 새 설정을 적용하고, `GetConfig()`에서 변경된 값을 반환해야 한다

**검증 방법:**
- 테스트용 `mockConfigurable` 구현
- `Configure()` -> `GetConfig()` 왕복(round-trip) 검증
- 잘못된 설정 키 전달 시 에러 반환 확인

---

## AC-020: 환경 변수 접두사 커스터마이징

**Given** `CUSTOM_SERVER_PORT=5000` 환경 변수가 설정된 상태에서

**When** `config.Load(WithEnvPrefix("CUSTOM"))` 호출 시

**Then** `Server().Port`가 `5000`이어야 한다

**검증 방법:**
- 기본 접두사(`XFLOW_`)와 커스텀 접두사 모두 테스트
- 접두사가 없는 환경 변수는 무시되는지 확인

---

## 품질 게이트

| 항목 | 기준 | 검증 명령 |
|------|------|----------|
| 테스트 커버리지 | 85% 이상 | `go test -coverprofile=cover.out ./internal/config/...` |
| 경쟁 조건 | go test -race 통과 | `go test -race ./internal/config/...` |
| 벤치마크 | `Load()`, `Set()`, `Server()` 벤치마크 기록 | `go test -bench=. -benchmem ./internal/config/...` |
| 린트 | go vet + golangci-lint 통과 | `go vet ./internal/config/... && golangci-lint run ./internal/config/...` |
| 에러 타입 | 모든 에러가 `errors.Is()`로 비교 가능 | AC-006, AC-011 테스트 케이스 |
| 다중 소스 우선순위 | CLI > ENV > 사용자 지정 파일 > 기본 경로 파일 > Default 5단계 검증 | AC-001-1, AC-004 테스트 케이스 |

---

## Definition of Done

- [ ] AC-001 ~ AC-020 모든 인수 테스트 통과
- [ ] 테스트 커버리지 85% 이상 달성
- [ ] `go test -race` 경쟁 조건 없음
- [ ] `go vet` 및 `golangci-lint` 경고 없음
- [ ] 벤치마크 결과 기록 완료
- [ ] 모든 에러 변수가 `errors.Is()`로 비교 가능
- [ ] 5단계 오버라이드 체인 (기본값 → 기본 경로 파일 → 사용자 지정 파일 → ENV → CLI) 검증 완료
- [ ] 바이너리별 기본 설정 경로 로딩 (xflowd, xflow, xflow-agent) 검증 완료
- [ ] Mutable/Immutable 키 분류 검증 완료
- [ ] 콜백 panic 복구 및 동시성 안전 검증 완료
- [ ] WatchConfig 파일 변경 감지 검증 완료
- [ ] ValidationErrors 복수 에러 집계 검증 완료
- [ ] 변경 이력 FIFO 관리 검증 완료
