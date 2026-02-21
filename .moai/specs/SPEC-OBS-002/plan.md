---
id: SPEC-OBS-002
type: plan
version: "1.0.0"
---

# SPEC-OBS-002 구현 계획: File-Based Log Output Configuration

## 1. 구현 전략

### 1.1 접근 방식

기존 `internal/observe` 패키지가 이미 `WithObserverWriter(io.Writer)`와 `WithObserverFormat(format string)` 옵션을 제공하므로, observe 레이어를 변경할 필요 없이 **config 레이어에서 설정을 파싱하여 기존 옵션에 전달**하는 방식으로 구현한다.

### 1.2 개발 방법론

Hybrid 모드 적용:
- 기존 파일 수정 (types.go, defaults.go, config.go, validate.go, main.go): DDD 방식 (ANALYZE-PRESERVE-IMPROVE)
- 새 함수 추가 (parseLogOutput, validateLogFormat, validateLogOutput): TDD 방식 (RED-GREEN-REFACTOR)

---

## 2. 마일스톤

### Primary Goal: Config 타입 및 기본값 확장

**대상 파일**: `internal/config/types.go`, `internal/config/defaults.go`, `internal/config/config.go`

**작업 내역**:
- `ObserveConfig` 구조체에 `Format string`, `Output string` 필드 추가
- `SetDefaults()`에 `observe.format = "json"`, `observe.output = "stdout"` 기본값 등록
- `Observe()` 접근자에서 `v.GetString("observe.format")`, `v.GetString("observe.output")` 읽기 추가
- 기존 테스트가 깨지지 않는지 확인 (characterization test)

**완료 기준**: 기존 테스트 통과 + 새 필드 접근 가능

### Secondary Goal: 검증 함수 추가

**대상 파일**: `internal/config/validate.go`, `internal/config/errors.go`

**작업 내역**:
- `ErrInvalidLogFormat`, `ErrInvalidLogOutput` 에러 변수 추가
- `validateLogFormat()` 함수 구현 ("json" | "text" 검증)
- `validateLogOutput()` 함수 구현 (빈 문자열, "stdout", 파일 경로, "stdout+경로" 패턴 검증)
- `Validate()` 함수에 새 검증 함수 호출 추가
- 단위 테스트 작성 (테이블 드리븐)

**완료 기준**: 유효/무효 입력에 대한 검증 테스트 통과

### Tertiary Goal: Observer 초기화 및 CLI 플래그 통합

**대상 파일**: `cmd/xflowd/main.go`

**작업 내역**:
- `parseLogOutput(output string) (io.Writer, io.Closer, error)` 함수 구현
- `--log-output` CLI 플래그 추가
- `runServer()` 함수에서 Format/Output 설정 기반 Observer 초기화 로직 추가
- 파일 핸들 생명주기 관리 (`defer closer.Close()`)
- 통합 테스트 작성

**완료 기준**: stdout/파일/동시 출력 시나리오 모두 동작

### Optional Goal: 로그 출력 파싱 유틸리티 분리

**대상 파일**: `internal/config/output.go` (신규, 선택적)

**작업 내역**:
- `parseLogOutput`을 `cmd/xflowd/main.go`에서 `internal/config/` 패키지로 분리 고려
- 재사용성 평가 후 결정

**완료 기준**: 코드 리뷰 시 결정

---

## 3. 기술 설계 방향

### 3.1 설정 오버라이드 체인

```
기본값 (defaults.go) → 설정 파일 (config.yaml) → 환경변수 → CLI 플래그
```

- `observe.format`: 기본값 "json" → 설정 파일 → (CLI 플래그 없음)
- `observe.output`: 기본값 "stdout" → 설정 파일 → `--log-output` CLI 플래그

### 3.2 출력 대상 파싱 전략

| 입력 | 파싱 결과 | io.Writer | io.Closer |
|------|----------|-----------|-----------|
| `"stdout"` | 표준 출력 | `os.Stdout` | `nil` |
| `"/var/log/xflow.log"` | 파일 출력 | `*os.File` | `*os.File` |
| `"stdout+/var/log/xflow.log"` | 동시 출력 | `io.MultiWriter(os.Stdout, file)` | `file` |

### 3.3 파일 열기 전략

- 플래그: `os.O_APPEND | os.O_CREATE | os.O_WRONLY`
- 퍼미션: `0644`
- 디렉토리 자동 생성: `os.MkdirAll(filepath.Dir(path), 0755)`

### 3.4 파일 핸들 닫기 전략

`cmd/xflowd/main.go`의 `runServer()` 함수 내에서:
- `parseLogOutput()` 호출 후 반환된 `io.Closer`를 `defer closer.Close()`로 등록
- 서버 종료(ctx 취소) 후 Agent 매니저 종료 후, 함수 리턴 시 자동 Close
- `defer` 순서: 파일 Close는 가장 마지막에 실행 (로그 기록 완료 후)

---

## 4. 리스크 및 대응

### R1: 기존 테스트 영향

- **리스크**: `ObserveConfig` 필드 추가로 기존 테스트 실패 가능
- **대응**: 기존 테스트에서 `ObserveConfig` 리터럴을 사용하는 경우 새 필드 기본값 추가. characterization test로 기존 동작 보존 확인

### R2: 파일 경로 권한 문제

- **리스크**: 지정된 파일 경로에 쓰기 권한이 없을 수 있음
- **대응**: `validateLogOutput()`에서는 형식만 검증, 실제 파일 열기는 서버 시작 시 수행하며 실패 시 명확한 에러 메시지 반환

### R3: 파일 핸들 누수

- **리스크**: 비정상 종료 시 파일 핸들이 닫히지 않을 수 있음
- **대응**: `defer` 패턴으로 보장. OS가 프로세스 종료 시 파일 디스크립터를 정리하므로 치명적 문제는 아님

### R4: 로그 로테이션 미지원

- **리스크**: 장기 실행 시 로그 파일이 무한 증가
- **대응**: 본 SPEC 범위 외. 외부 도구(logrotate)나 향후 SPEC에서 lumberjack 통합 고려

---

## 5. 추적성

| 마일스톤 | 관련 요구사항 | 우선순위 |
|---------|-------------|---------|
| Primary Goal | REQ-OBS-002-01-01 ~ 01-04 | P0 |
| Secondary Goal | REQ-OBS-002-02-01 ~ 02-03 | P0 |
| Tertiary Goal | REQ-OBS-002-03-01 ~ 03-05, 04-01 ~ 04-02, 05-01 ~ 05-02 | P0 + P1 |
| Optional Goal | (구조 개선) | P1 |
