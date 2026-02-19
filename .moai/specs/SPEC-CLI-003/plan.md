---
id: SPEC-CLI-003
version: "1.0.0"
status: draft
created: "2026-02-19"
updated: "2026-02-19"
author: xtra
priority: medium
---

# SPEC-CLI-003: 구현 계획 - 스크립트 실행 및 명령어 히스토리 강화

## 1. 구현 개요

### 1.1 목표

기존 SPEC-CLI-002 대화형 모드 위에 두 가지 핵심 기능을 추가한다:

1. **스크립트 실행**: 여러 명령어를 파일에 작성하여 순차 배치 실행
2. **히스토리 강화**: `chzyer/readline` 라이브러리를 활용한 실질적 터미널 기능 (화살표 키 탐색, Ctrl+R 검색, 파일 영속화, 탭 자동완성 연동)

### 1.2 영향 범위

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `script.go` | 신규 | 스크립트 파일 파싱 및 실행기 |
| `script_test.go` | 신규 | 스크립트 실행기 테스트 |
| `readline_adapter.go` | 신규 | chzyer/readline readlineInterface 구현체 |
| `readline_adapter_test.go` | 신규 | readline 어댑터 테스트 |
| `interactive.go` | 수정 | readline 구현체 교체, source/run/history 명령어 확장 |
| `interactive_test.go` | 수정 | 신규 기능 테스트 추가 |
| `autocomplete.go` | 수정 (최소) | readline CompleteFunc 어댑터 함수 추가 |
| `root.go` | 수정 | script 서브커맨드 등록 |

---

## 2. 마일스톤

### Milestone 1: P0 - Script Execution (핵심 목표)

**범위**: REQ-SCR-001 ~ REQ-SCR-009

#### M1-1: 스크립트 파서 구현

- `script.go` 파일 생성
- `ScriptExecutor` 구조체 정의
- `ParseScriptFile(path string) ([]ScriptLine, error)` 함수 구현
  - 파일 읽기, 주석 제거, 빈 줄 무시, 줄 끝 공백 제거
  - `ScriptLine` 구조체: `{LineNumber int, Command string}`
- 단위 테스트: 파일 파싱 (주석, 빈 줄, 일반 명령어, 인코딩)

#### M1-2: 순차 실행기 구현

- `Execute(lines []ScriptLine) (*ScriptResult, error)` 메서드 구현
- 기존 `executeCommand()` 함수를 활용하여 각 명령어 실행
- 에러 처리 정책: stop-on-error (기본) / continue-on-error (플래그)
- `ScriptResult` 구조체로 실행 결과 수집 (Total, Success, Failed, Skipped)
- 실행 결과 요약 출력 함수 구현
- 단위 테스트: 정상 실행, 에러 중단, 에러 계속, 빈 스크립트

#### M1-3: CLI 및 REPL 통합

- `root.go`에 `script` 서브커맨드 등록
  - `-f, --file` 플래그: 스크립트 파일 경로
  - `--continue-on-error` 플래그
- `interactive.go`에 REPL 특수 명령어 추가
  - `source <file>`: 스크립트 실행
  - `run <file>`: source 별칭
- 종료 코드 처리: 모든 성공 = 0, 실패 있음 = 1
- 통합 테스트: CLI 모드 스크립트 실행, REPL 모드 source 명령어

---

### Milestone 2: P1 - Readline Integration 및 History Enhancement (보조 목표)

**범위**: REQ-SCR-010 ~ REQ-SCR-015

#### M2-1: readline 어댑터 구현

- `readline_adapter.go` 파일 생성
- `chzyerReadline` 구조체 정의 (readlineInterface 인터페이스 구현)
  - `Readline()`: `instance.Readline()` 위임
  - `SetPrompt()`: `instance.SetPrompt()` 위임
  - `Close()`: `instance.Close()` 위임
  - `SaveHistory()`: `instance.SaveHistory()` 위임 (파일 자동 기록)
- `newChzyerReadline(config ChzyerConfig) (*chzyerReadline, error)` 팩토리 함수
  - readline.Config 구성: Prompt, HistoryFile, HistoryLimit, AutoComplete
  - 히스토리 파일 디렉토리 자동 생성 (`~/.xflow/`)
- 비TTY 환경 감지: `isatty` 패키지 또는 `readline.IsTerminal()` 사용
- 단위 테스트: 인터페이스 만족 확인, 설정 전달 확인

#### M2-2: REPL 세션에 readline 통합

- `interactive.go`의 `NewInteractiveSession()` 수정
  - TTY 환경: `chzyerReadline` 사용
  - 비TTY 환경: 기존 `stdioReadline` 폴백
- 인메모리 히스토리 관리 코드와 readline 내장 히스토리 통합
  - 기존 `s.history` 슬라이스는 readline이 관리하므로 단순화 가능
  - `printHistory()` 함수는 readline 히스토리 파일을 읽어 표시
- 기존 테스트 호환: mock readlineInterface 주입 패턴 유지
- 단위 테스트: TTY/비TTY 분기, readline 초기화

#### M2-3: 히스토리 관리 명령어 확장

- `history` 명령어 기존 동작 유지 (번호 포함 목록)
- `history clear` 서브명령어 추가
  - 인메모리 히스토리 초기화
  - 히스토리 파일 삭제 또는 비움
- `history search <pattern>` 서브명령어 추가
  - 문자열 부분 일치 검색
  - 대소문자 무시 옵션
  - 일치하는 항목을 번호와 함께 출력
- 단위 테스트: clear, search 정상/비정상 케이스

#### M2-4: AutoCompleter readline 연동

- `autocomplete.go`에 readline CompleteFunc 어댑터 추가
  - `ToReadlineCompleter() readline.AutoCompleter` 메서드
  - 기존 `Complete(line, pos)` 결과를 readline 후보 형식으로 변환
- readline.Config의 AutoComplete 필드에 어댑터 설정
- Tab 키 입력 시 AutoCompleter 후보가 readline에 의해 표시됨
- 단위 테스트: 어댑터 변환 로직, 빈 결과, 다중 후보

---

### Milestone 3: P2 - Advanced Script Features (선택 목표)

**범위**: REQ-SCR-016 ~ REQ-SCR-018

#### M3-1: 환경 변수 치환

- 스크립트 파싱 단계에서 `$VAR_NAME` 및 `${VAR}` 패턴 감지
- `os.ExpandEnv()` 또는 정규식 기반 치환 구현
- 미정의 변수는 빈 문자열로 치환
- 단위 테스트: 다양한 변수 패턴, 미정의 변수, 이스케이프

#### M3-2: Dry-run 및 Verbose 모드

- `--dry-run` 플래그: 명령어 실행 없이 `[줄번호] <명령어>` 목록 출력
- `--verbose` 플래그: 각 명령어 실행 전 진행 정보 출력
- `root.go` script 명령어에 플래그 등록
- 단위 테스트: dry-run 출력 형식, verbose 출력 형식

---

## 3. 기술 접근 방식

### 3.1 스크립트 실행기 설계

**원칙**: 기존 `executeCommand()` 함수를 최대한 재사용하여 스크립트 내 명령어가 REPL에서 직접 입력한 것과 동일하게 동작하도록 보장한다.

**실행 흐름**:

```
스크립트 파일 --> 파서 --> []ScriptLine --> 실행기 --> ScriptResult
                  |                         |
                  |-- 주석/빈줄 제거        |-- executeCommand() 위임
                  |-- 환경 변수 치환(P2)    |-- 에러 정책 적용
                                            |-- 결과 수집
```

**에러 정책 구현**:
- `continueOnError = false` (기본): 첫 실패 시 루프 중단, 남은 명령어는 Skipped로 분류
- `continueOnError = true`: 실패해도 루프 계속, 실패 목록을 Errors에 수집

### 3.2 readline 통합 전략

**원칙**: 기존 `readlineInterface` 추상화를 유지하면서, 실제 구현체만 교체한다. 기존 목 테스트에 영향을 주지 않는다.

**분기 전략**:

```
NewInteractiveSession()
  |
  ├── stdin이 TTY인가?
  │     ├── 예: chzyerReadline 생성 (readline.Config 포함)
  │     └── 아니오: stdioReadline 생성 (기존 동작)
  |
  └── readlineFn 필드에 할당
```

**히스토리 통합**:
- `chzyer/readline`은 자체적으로 히스토리 파일 관리를 지원
- `HistoryFile` 설정으로 파일 경로 지정, `HistoryLimit`으로 크기 제한
- `SaveHistory()` 호출 시 자동으로 파일에 기록
- 기존 `s.history` 인메모리 슬라이스는 readline 비사용 환경(테스트, 비TTY)에서만 활용

### 3.3 AutoCompleter 연동

**접근 방식**: 기존 `AutoCompleter.Complete()` 결과를 readline의 `AutoCompleter` 인터페이스로 래핑하는 어댑터 패턴을 사용한다.

```go
// readline.AutoCompleter 인터페이스 구현
type readlineCompleterAdapter struct {
    completer *AutoCompleter
}

func (a *readlineCompleterAdapter) Do(line []rune, pos int) ([][]rune, int) {
    // AutoCompleter.Complete() 호출
    // 결과를 [][]rune 형식으로 변환
}
```

---

## 4. 리스크 및 대응 방안

| 리스크 | 영향 | 대응 방안 |
|--------|------|----------|
| chzyer/readline이 Windows에서 제한적 | 크로스 플랫폼 지원 저하 | 비TTY 환경 폴백으로 대응, Windows 사용자에게 안내 |
| readline 통합 시 기존 테스트 깨짐 | 테스트 커버리지 하락 | mock readlineInterface 패턴 유지, 통합 전 기존 테스트 전체 통과 확인 |
| 스크립트 실행 시 플래그 상태 오염 | 연속 명령어 간 부작용 | 기존 플래그 리셋 로직 검증, 각 명령어 실행 전 Cobra 상태 초기화 |
| 히스토리 파일 권한 문제 | 파일 읽기/쓰기 실패 | 디렉토리/파일 자동 생성, 권한 에러 시 경고 후 인메모리 모드 폴백 |
| 대용량 스크립트 파일 (수만 줄) | 메모리 사용량 증가 | 파일 크기 제한 경고 (10,000 줄), 스트리밍 파서 고려 |

---

## 5. 테스트 전략

### 5.1 단위 테스트

**script.go 테스트 (table-driven)**:
- 스크립트 파싱: 주석, 빈 줄, 일반 명령어, 인라인 주석, 공백 처리
- 순차 실행: 전체 성공, 중간 실패(중단), 중간 실패(계속), 빈 스크립트
- 에러 처리: 파일 미발견, 빈 파일, 권한 오류
- 환경 변수 치환 (P2): 정의된 변수, 미정의 변수, 복합 패턴

**readline_adapter.go 테스트**:
- readlineInterface 인터페이스 만족 확인
- readline.Config 전달 확인
- 비TTY 환경 폴백 동작

**interactive.go 추가 테스트**:
- `source`/`run` REPL 명령어 동작
- `history clear` 명령어
- `history search` 명령어
- readline 구현체 분기 (TTY/비TTY)

### 5.2 통합 테스트

- CLI `xflow script -f <file>` 실행 및 종료 코드 확인
- REPL `source <file>` 실행
- `--continue-on-error` 플래그 동작 확인

### 5.3 커버리지 목표

| 파일 | 목표 커버리지 |
|------|-------------|
| script.go | 90% 이상 |
| readline_adapter.go | 85% 이상 |
| interactive.go (수정분) | 기존 91.7% 유지 또는 향상 |
| autocomplete.go (수정분) | 기존 커버리지 유지 |

### 5.4 회귀 테스트

- 기존 `interactive_test.go` 전체 통과 확인 (70+ 서브테스트)
- `go test -race ./internal/cli/...` 통과 확인
- 기존 REPL 기능 (명령어 실행, 종료, 도움말, 히스토리 표시) 동작 확인

---

## 6. 전문가 자문 권장

### 6.1 expert-backend 자문 추천

이 SPEC은 Go 백엔드 구현을 포함하므로 다음 영역에서 expert-backend 자문을 권장한다:

- `chzyer/readline` 라이브러리 통합 패턴 및 최선의 활용 방법
- readline AutoCompleter 인터페이스 어댑터 설계
- 스크립트 파서의 에러 처리 전략 및 에지 케이스 검토
- 기존 코드베이스와의 통합 시 호환성 보장 전략

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-19*
*작성: MoAI SPEC Builder*
