---
id: SPEC-OBS-002
type: acceptance
version: "1.0.0"
---

# SPEC-OBS-002 인수 기준: File-Based Log Output Configuration

## 1. Config Types Extension (Module 1)

### AC-OBS-002-01-01: ObserveConfig 필드 확인

```gherkin
Scenario: ObserveConfig에 Format, Output 필드가 존재한다
  Given ObserveConfig 구조체가 정의되어 있을 때
  Then Format string 필드가 존재해야 한다
  And Output string 필드가 존재해야 한다
  And 기존 DefaultLevel, MetricsEnabled, TraceEnabled 필드도 유지되어야 한다
```

### AC-OBS-002-01-02: Viper 기본값 등록

```gherkin
Scenario: observe.format과 observe.output 기본값이 등록된다
  Given 새 Viper 인스턴스에 SetDefaults()를 호출하면
  Then v.GetString("observe.format")이 "json"이어야 한다
  And v.GetString("observe.output")이 "stdout"이어야 한다
```

### AC-OBS-002-01-03: Observe() 접근자 확장

```gherkin
Scenario: Observe() 접근자가 Format과 Output을 반환한다
  Given config.yaml에 observe.format: "text", observe.output: "/var/log/xflow.log"가 설정되어 있을 때
  When cfg.Observe()를 호출하면
  Then ObserveConfig.Format이 "text"이어야 한다
  And ObserveConfig.Output이 "/var/log/xflow.log"이어야 한다
```

### AC-OBS-002-01-04: 설정 파일 없이 기본값 동작

```gherkin
Scenario: 설정 파일이 없을 때 기본값이 적용된다
  Given 설정 파일 없이 Config를 로드하면
  When cfg.Observe()를 호출하면
  Then ObserveConfig.Format이 "json"이어야 한다
  And ObserveConfig.Output이 "stdout"이어야 한다
  And 기존 ObserveConfig.DefaultLevel이 "info"이어야 한다
```

### AC-OBS-002-01-05: Immutable 설정 확인

```gherkin
Scenario: observe.format과 observe.output은 immutable이다
  Given IsMutable 함수가 정의되어 있을 때
  Then IsMutable("observe.format")이 false이어야 한다
  And IsMutable("observe.output")이 false이어야 한다
  And IsImmutable("observe.format")이 true이어야 한다
  And IsImmutable("observe.output")이 true이어야 한다
```

---

## 2. Config Validation (Module 2)

### AC-OBS-002-02-01: 유효한 Format 값 검증 통과

```gherkin
Scenario Outline: 유효한 observe.format 값은 검증을 통과한다
  Given observe.format이 <format>으로 설정되어 있을 때
  When Validate()를 실행하면
  Then observe.format 관련 에러가 없어야 한다

  Examples:
    | format |
    | "json" |
    | "text" |
```

### AC-OBS-002-02-02: 무효한 Format 값 검증 실패

```gherkin
Scenario Outline: 무효한 observe.format 값은 검증에 실패한다
  Given observe.format이 <format>으로 설정되어 있을 때
  When Validate()를 실행하면
  Then ErrInvalidLogFormat 에러가 반환되어야 한다

  Examples:
    | format  |
    | "xml"   |
    | "yaml"  |
    | ""      |
    | "JSON"  |
```

### AC-OBS-002-02-03: 유효한 Output 값 검증 통과

```gherkin
Scenario Outline: 유효한 observe.output 값은 검증을 통과한다
  Given observe.output이 <output>으로 설정되어 있을 때
  When Validate()를 실행하면
  Then observe.output 관련 에러가 없어야 한다

  Examples:
    | output                          |
    | "stdout"                        |
    | "/var/log/xflow.log"            |
    | "./logs/xflow.log"              |
    | "stdout+/var/log/xflow.log"     |
    | "stdout+./logs/xflow.log"       |
```

### AC-OBS-002-02-04: 무효한 Output 값 검증 실패

```gherkin
Scenario Outline: 무효한 observe.output 값은 검증에 실패한다
  Given observe.output이 <output>으로 설정되어 있을 때
  When Validate()를 실행하면
  Then ErrInvalidLogOutput 에러가 반환되어야 한다

  Examples:
    | output    |
    | ""        |
    | "+"       |
    | "stdout+" |
    | "+/path"  |
```

---

## 3. Observer Initialization (Module 3)

### AC-OBS-002-03-01: Format 설정 적용

```gherkin
Scenario: observe.format 설정이 Observer에 적용된다
  Given config.yaml에 observe.format: "text"가 설정되어 있을 때
  When 서버가 시작되면
  Then Observer가 "text" 포맷으로 초기화되어야 한다
```

### AC-OBS-002-03-02: stdout 출력 (기본 동작)

```gherkin
Scenario: observe.output이 "stdout"이면 기본 동작을 유지한다
  Given config.yaml에 observe.output: "stdout"이 설정되어 있을 때
  When 서버가 시작되면
  Then Observer의 기본 Writer가 os.Stdout이어야 한다
  And 파일 핸들이 열리지 않아야 한다
```

### AC-OBS-002-03-03: 파일 출력

```gherkin
Scenario: observe.output이 파일 경로이면 파일에 출력한다
  Given config.yaml에 observe.output: "/tmp/xflow-test.log"가 설정되어 있을 때
  When 서버가 시작되고 로그가 기록되면
  Then /tmp/xflow-test.log 파일에 로그가 기록되어야 한다
  And 표준 출력에는 로그가 출력되지 않아야 한다
```

### AC-OBS-002-03-04: stdout + 파일 동시 출력

```gherkin
Scenario: observe.output이 "stdout+파일경로"이면 동시 출력한다
  Given config.yaml에 observe.output: "stdout+/tmp/xflow-test.log"가 설정되어 있을 때
  When 서버가 시작되고 로그가 기록되면
  Then 표준 출력에 로그가 출력되어야 한다
  And /tmp/xflow-test.log 파일에도 동일한 로그가 기록되어야 한다
```

### AC-OBS-002-03-05: 디렉토리 자동 생성

```gherkin
Scenario: 로그 파일 디렉토리가 없으면 자동 생성한다
  Given config.yaml에 observe.output: "/tmp/xflow-new-dir/test.log"가 설정되어 있을 때
  And /tmp/xflow-new-dir/ 디렉토리가 존재하지 않을 때
  When 서버가 시작되면
  Then /tmp/xflow-new-dir/ 디렉토리가 생성되어야 한다
  And /tmp/xflow-new-dir/test.log 파일이 생성되어야 한다
```

### AC-OBS-002-03-06: 파일 열기 실패 시 서버 시작 중단

```gherkin
Scenario: 파일을 열 수 없으면 서버 시작이 실패한다
  Given config.yaml에 observe.output: "/root/no-permission.log"가 설정되어 있을 때
  And 해당 경로에 쓰기 권한이 없을 때
  When 서버 시작을 시도하면
  Then 에러가 반환되어야 한다
  And 에러 메시지에 "로그 출력" 관련 내용이 포함되어야 한다
```

### AC-OBS-002-03-07: 파일 Append 모드 확인

```gherkin
Scenario: 기존 파일이 있으면 append 모드로 열린다
  Given /tmp/xflow-existing.log 파일에 기존 내용이 있을 때
  And config.yaml에 observe.output: "/tmp/xflow-existing.log"가 설정되어 있을 때
  When 서버가 시작되고 로그가 기록되면
  Then 기존 파일 내용이 보존되어야 한다
  And 새 로그가 파일 끝에 추가되어야 한다
```

---

## 4. CLI Flag Integration (Module 4)

### AC-OBS-002-04-01: --log-output 플래그 존재

```gherkin
Scenario: xflowd 커맨드에 --log-output 플래그가 존재한다
  When xflowd --help를 실행하면
  Then --log-output 플래그가 도움말에 표시되어야 한다
```

### AC-OBS-002-04-02: CLI 플래그가 설정 파일보다 우선

```gherkin
Scenario: --log-output 플래그가 설정 파일 값을 오버라이드한다
  Given config.yaml에 observe.output: "stdout"이 설정되어 있을 때
  When xflowd --log-output /tmp/cli-test.log를 실행하면
  Then 로그가 /tmp/cli-test.log에 기록되어야 한다
  And 표준 출력에는 로그가 출력되지 않아야 한다
```

---

## 5. File Handle Lifecycle (Module 5)

### AC-OBS-002-05-01: 서버 종료 시 파일 핸들 닫기

```gherkin
Scenario: 서버 종료 시 로그 파일 핸들이 닫힌다
  Given config.yaml에 observe.output: "/tmp/xflow-close-test.log"가 설정되어 있을 때
  And 서버가 실행 중일 때
  When SIGTERM 시그널을 보내면
  Then 서버가 정상 종료되어야 한다
  And 로그 파일 핸들이 닫혀야 한다
```

### AC-OBS-002-05-02: stdout 모드에서는 파일 핸들 관리 불필요

```gherkin
Scenario: stdout 모드에서는 Close 호출이 없다
  Given config.yaml에 observe.output: "stdout"이 설정되어 있을 때
  When 서버가 시작되고 종료되면
  Then 파일 관련 Close 호출이 없어야 한다
  And 서버가 정상 종료되어야 한다
```

---

## 6. Quality Gate

### Definition of Done

- [ ] 모든 인수 기준 시나리오에 대한 테스트 통과
- [ ] 기존 config 패키지 테스트 통과 (regression 없음)
- [ ] `go test -race ./internal/config/...` 통과
- [ ] `go test -race ./cmd/xflowd/...` 통과
- [ ] `go vet ./...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (수정된 파일 기준)
- [ ] 설정 파일 없이 시작 시 기존 동작 완전 호환

### 검증 방법

- **단위 테스트**: validateLogFormat, validateLogOutput, parseLogOutput 함수
- **통합 테스트**: Viper 기반 Config 로드 후 ObserveConfig 필드 확인
- **E2E 테스트**: 실제 파일 경로로 Observer 초기화 및 로그 기록 확인

### 추적성

| 인수 기준 ID | 요구사항 ID | 모듈 |
|-------------|-------------|------|
| AC-OBS-002-01-01 ~ 01-05 | REQ-OBS-002-01-01 ~ 01-04 | Config Types Extension |
| AC-OBS-002-02-01 ~ 02-04 | REQ-OBS-002-02-01 ~ 02-03 | Config Validation |
| AC-OBS-002-03-01 ~ 03-07 | REQ-OBS-002-03-01 ~ 03-05 | Observer Initialization |
| AC-OBS-002-04-01 ~ 04-02 | REQ-OBS-002-04-01 ~ 04-02 | CLI Flag Integration |
| AC-OBS-002-05-01 ~ 05-02 | REQ-OBS-002-05-01 ~ 05-02 | File Handle Lifecycle |
