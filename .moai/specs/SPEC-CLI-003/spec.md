---
id: SPEC-CLI-003
version: "1.0.0"
status: draft
created: "2026-02-19"
updated: "2026-02-19"
author: xtra
priority: medium
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-19 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-CLI-003: 스크립트 실행 및 명령어 히스토리 강화

## 1. Environment (환경)

### 1.1 시스템 개요

xflow CLI 도구의 스크립트 실행 및 명령어 히스토리 강화 기능이다. 여러 명령어를 스크립트 파일에 작성하여 순차적으로 실행할 수 있으며, REPL 모드에서 실질적인 readline 라이브러리(`chzyer/readline`)를 통합하여 화살표 키 히스토리 탐색, Ctrl+R 역방향 검색, 히스토리 파일 영속화를 지원한다.

본 SPEC은 다음을 포함한다:

- **Script Executor** (`script.go`): 스크립트 파일 파싱, 순차 실행, 에러 처리 정책, 실행 결과 요약
- **Readline Integration** (`readline_adapter.go`): `chzyer/readline` 기반 readlineInterface 구현체, 히스토리 영속화, 탭 자동완성 연동
- **History Enhancement** (`interactive.go` 수정): 기존 인메모리 히스토리를 파일 기반 영속 히스토리로 교체, 히스토리 관리 명령어 확장

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/cli/` (기존 CLI 패키지 내 확장)
- **Tier**: internal (비공개 패키지)
- **CLI 프레임워크**: `github.com/spf13/cobra` v1.8+ (SPEC-CLI-001과 동일)
- **Readline 라이브러리**: `github.com/chzyer/readline` v1.5+ (SPEC-CLI-002에서 go.mod에 추가됨)
- **의존 SPEC**:
  - SPEC-CLI-001: 기본 CLI 시스템 (root.go, client.go, output.go, errors.go)
  - SPEC-CLI-002: Interactive Mode (interactive.go, autocomplete.go, wizard.go)
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`

### 1.3 의존성

#### SPEC-CLI-002 의존 모듈

| 모듈 | 파일 | 사용 용도 |
|------|------|----------|
| REPL Core | interactive.go | 스크립트 명령어를 REPL 실행 엔진으로 위임, 히스토리 교체 |
| AutoCompleter | autocomplete.go | readline 탭 자동완성 콜백 연동 |
| readlineInterface | interactive.go | 기존 인터페이스를 readline 라이브러리 구현체로 교체 |

#### 현재 코드베이스 상태

| 구성 요소 | 현재 상태 | SPEC-CLI-003 목표 |
|-----------|----------|------------------|
| readlineInterface | `stdioReadline` (bufio.Scanner 기반, 기본 라인 입력만 가능) | `chzyer/readline` 기반 구현체로 교체 |
| SaveHistory() | no-op (아무 동작 없음) | 파일 기반 영속 저장 |
| history 필드 | 인메모리 슬라이스, 최대 1000항목 | readline 라이브러리 내장 히스토리로 통합 |
| 화살표 키 탐색 | 미지원 | Up/Down 화살표 키로 히스토리 탐색 |
| Ctrl+R 검색 | 미지원 | readline 내장 역방향 검색 |
| 스크립트 실행 | 미구현 | 파일 기반 배치 명령어 실행 |
| AutoCompleter 통합 | 독립 엔진 (readline 입력 루프와 미연동) | readline CompleteFunc 콜백으로 연동 |

### 1.4 제약 조건

- **하위 호환성 유지**: 기존 `readlineInterface` 인터페이스를 변경하지 않으며, 새로운 구현체를 주입
- **테스트 가능성 유지**: `readlineInterface` 목 주입 패턴을 유지하여 기존 테스트에 영향 없음
- **크로스 플랫폼**: Linux, macOS에서 readline 기능 완전 지원 (Windows는 최선 노력)
- **비대화형 환경**: 스크립트 실행은 TTY가 아닌 환경에서도 동작해야 함
- **기존 테스트 보존**: 91.7% 커버리지와 70+ 서브테스트를 유지하거나 향상

### 1.5 스코프 경계

**포함 (In Scope)**:
- 스크립트 파일 파싱 및 순차 실행
- `chzyer/readline` 기반 readlineInterface 구현체
- 히스토리 파일 영속화 (`~/.xflow/history`)
- 화살표 키(Up/Down) 히스토리 탐색
- Ctrl+R 역방향 히스토리 검색
- 기존 AutoCompleter와 readline 자동완성 콜백 연동
- 히스토리 관리 명령어 확장 (`history clear`, `history search`)
- 스크립트 실행 결과 요약 출력
- 환경 변수 치환 (P2, 선택)

**제외 (Out of Scope)**:
- 스크립트 언어 문법 (조건문, 반복문, 함수 정의 등)
- 파이프라인 연결 (`|`, `>`, `>>` 등 쉘 연산자)
- 스크립트 간 의존성 관리
- 병렬 명령어 실행
- 원격 스크립트 실행 (URL에서 스크립트 로딩)

---

## 2. Assumptions (가정 사항)

### 2.1 기술 가정

- **AS-SCR-001**: SPEC-CLI-002의 interactive.go, autocomplete.go가 구현되어 있다
- **AS-SCR-002**: `github.com/chzyer/readline` v1.5+가 go.mod에 등록되어 있다
- **AS-SCR-003**: 기존 `readlineInterface` 인터페이스(Readline, SetPrompt, Close, SaveHistory 메서드)가 유지된다
- **AS-SCR-004**: `executeCommand()` 함수가 단일 명령어 실행을 안정적으로 처리한다
- **AS-SCR-005**: `tokenizeInput()` 함수가 명령어 문자열을 올바르게 토큰화한다
- **AS-SCR-006**: 플래그 리셋 로직이 명령어 간 상태 오염을 방지한다

### 2.2 운영 가정

- **AS-SCR-007**: 사용자는 `~/.xflow/` 디렉토리에 대한 읽기/쓰기 권한이 있다
- **AS-SCR-008**: 스크립트 파일은 UTF-8 인코딩이며 합리적인 크기(10,000 라인 이하)이다
- **AS-SCR-009**: 스크립트 파일에 포함된 명령어는 기존 CLI 명령어 체계를 따른다

---

## 3. Requirements (요구사항)

### 3.1 P0 (핵심) - Script Execution

#### REQ-SCR-001: 스크립트 파일 포맷
시스템은 **항상** 다음 스크립트 파일 포맷을 지원해야 한다:
- 한 줄에 하나의 명령어
- `#`으로 시작하는 줄은 주석으로 처리
- 빈 줄은 무시
- 줄 끝 공백은 제거

#### REQ-SCR-002: CLI 플래그를 통한 스크립트 실행
**WHEN** 사용자가 `xflow script -f <file>` 또는 `xflow script --file <file>`을 실행할 때, **THEN** 시스템은 지정된 파일을 읽어 각 명령어를 순차적으로 실행해야 한다.

#### REQ-SCR-003: REPL 내 스크립트 실행
**WHEN** REPL 모드에서 사용자가 `source <file>` 또는 `run <file>`을 입력할 때, **THEN** 시스템은 지정된 파일을 읽어 각 명령어를 순차적으로 실행해야 한다.

#### REQ-SCR-004: 순차 실행
**WHILE** 스크립트가 실행되는 동안, 시스템은 명령어를 파일에 기재된 순서대로 하나씩 실행해야 한다. 이전 명령어가 완료된 후에 다음 명령어를 실행해야 한다.

#### REQ-SCR-005: 에러 시 중단 (기본 동작)
**IF** 스크립트 실행 중 명령어가 실패하는 경우, **THEN** 시스템은 기본적으로 나머지 명령어 실행을 중단하고, 실패한 명령어 정보와 에러 내용을 출력해야 한다.

#### REQ-SCR-006: 에러 시 계속 실행 (옵션)
**WHEN** `--continue-on-error` 플래그가 지정된 경우, **THEN** 시스템은 명령어 실패 시에도 다음 명령어를 계속 실행하고, 최종적으로 실패한 명령어 목록을 요약 출력해야 한다.

#### REQ-SCR-007: 종료 코드
**WHEN** 스크립트 실행이 완료되었을 때, **THEN** 시스템은 모든 명령어가 성공하면 종료 코드 0을, 하나라도 실패하면 0이 아닌 종료 코드를 반환해야 한다.

#### REQ-SCR-008: 파일 미발견 에러
**IF** 지정된 스크립트 파일이 존재하지 않는 경우, **THEN** 시스템은 파일 경로를 포함한 에러 메시지를 출력하고 종료 코드 1을 반환해야 한다.

#### REQ-SCR-009: 실행 결과 요약
**WHEN** 스크립트 실행이 완료되었을 때, **THEN** 시스템은 다음을 포함하는 실행 결과 요약을 출력해야 한다:
- 전체 명령어 수
- 성공한 명령어 수
- 실패한 명령어 수
- 건너뛴 명령어 수 (에러 중단 시)

---

### 3.2 P1 (중요) - Readline Integration 및 History Enhancement

#### REQ-SCR-010: chzyer/readline 기반 구현체
**WHEN** REPL 세션이 TTY 환경에서 시작될 때, **THEN** 시스템은 `chzyer/readline` 라이브러리 기반의 readlineInterface 구현체를 사용해야 한다.
비TTY 환경에서는 기존 `stdioReadline` 구현체를 폴백으로 사용해야 한다.

#### REQ-SCR-011: 히스토리 파일 영속화
**WHILE** REPL 모드에 있는 동안, 시스템은 입력된 명령어를 `~/.xflow/history` 파일에 저장해야 한다.
- 파일 형식: 한 줄에 하나의 명령어 (readline 표준 형식)
- 크기 제한: 기본 1000 항목, 초과 시 오래된 항목부터 제거
- 세션 간 히스토리가 유지되어야 한다

#### REQ-SCR-012: 화살표 키 히스토리 탐색
**WHEN** REPL 모드에서 사용자가 Up 화살표 키를 누를 때, **THEN** 시스템은 이전 명령어를 입력 줄에 표시해야 한다.
**WHEN** REPL 모드에서 사용자가 Down 화살표 키를 누를 때, **THEN** 시스템은 다음(더 최근) 명령어를 입력 줄에 표시해야 한다.

#### REQ-SCR-013: Ctrl+R 역방향 히스토리 검색
**WHEN** REPL 모드에서 사용자가 Ctrl+R을 누를 때, **THEN** 시스템은 역방향 히스토리 검색 모드에 진입해야 한다.
검색어 입력 시 일치하는 가장 최근 명령어를 실시간으로 표시해야 한다.

#### REQ-SCR-014: 히스토리 관리 명령어 확장
**WHEN** REPL 모드에서 사용자가 `history` 명령어를 입력할 때, **THEN** 시스템은 최근 명령어 히스토리를 번호와 함께 목록으로 표시해야 한다.
**WHEN** `history clear`를 입력할 때, **THEN** 시스템은 인메모리 및 파일 히스토리를 모두 삭제해야 한다.
**WHEN** `history search <pattern>`을 입력할 때, **THEN** 시스템은 패턴과 일치하는 히스토리 항목을 검색하여 표시해야 한다.

#### REQ-SCR-015: AutoCompleter와 readline 연동
**WHILE** REPL 모드에 있는 동안, 시스템은 기존 AutoCompleter 엔진을 readline 라이브러리의 자동완성 콜백으로 연동해야 한다.
Tab 키를 누르면 AutoCompleter가 제안하는 후보 목록을 readline이 표시해야 한다.

---

### 3.3 P2 (선택) - Advanced Script Features

#### REQ-SCR-016: 환경 변수 치환
**WHEN** 스크립트 파일에 `$VAR_NAME` 또는 `${VAR}` 형식의 환경 변수 참조가 포함된 경우, **THEN** 시스템은 해당 참조를 실행 환경의 환경 변수 값으로 치환해야 한다.
**IF** 참조된 환경 변수가 정의되지 않은 경우, **THEN** 빈 문자열로 치환해야 한다.

#### REQ-SCR-017: Dry-run 모드
**WHEN** `--dry-run` 플래그가 지정된 경우, **THEN** 시스템은 각 명령어를 실행하지 않고 실행할 명령어 목록만 출력해야 한다.
출력에는 줄 번호와 명령어 내용이 포함되어야 한다.

#### REQ-SCR-018: Verbose 모드
**WHEN** `--verbose` 플래그가 지정된 경우, **THEN** 시스템은 각 명령어 실행 전에 `[줄번호] 실행: <명령어>` 형식의 진행 정보를 출력해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/cli/
  script.go                  # 스크립트 파일 파싱 및 실행기 (신규)
  script_test.go             # 스크립트 테스트 (신규)
  readline_adapter.go        # chzyer/readline 기반 readlineInterface 구현체 (신규)
  readline_adapter_test.go   # readline 어댑터 테스트 (신규)
  interactive.go             # (수정) readline 구현체 교체, 히스토리 명령어 확장, source/run 명령어 추가
  interactive_test.go        # (수정) 신규 기능 테스트 추가
  autocomplete.go            # (수정 최소) readline CompleteFunc 어댑터 추가
  root.go                    # (수정) script 서브커맨드 등록
```

### 4.2 명령어 트리 확장

```
xflow
  ├── script
  │   └── -f, --file <file>         # 스크립트 파일 실행
  │       ├── --continue-on-error   # 에러 시 계속 실행
  │       ├── --dry-run             # 실행하지 않고 명령어 목록만 출력 (P2)
  │       └── --verbose             # 각 명령어 실행 전 진행 정보 출력 (P2)
  ├── interactive
  │   └── (REPL 내 추가 명령어)
  │       ├── source <file>          # 스크립트 파일 실행
  │       ├── run <file>             # 스크립트 파일 실행 (source 별칭)
  │       ├── history                # 히스토리 목록 (기존)
  │       ├── history clear          # 히스토리 삭제
  │       └── history search <pat>   # 히스토리 검색
  ├── (기존 명령어...)
  └── ...
```

### 4.3 스크립트 파일 예시

```bash
# xflow 자동화 스크립트 예시
# 서버 상태 확인 후 플로우 배포

status
flow list --format json

# 특정 플로우 배포 및 시작
flow deploy my-sensor-flow
flow start my-sensor-flow

# Agent 상태 확인
agent list
```

### 4.4 핵심 인터페이스

```go
// ScriptExecutor - 스크립트 실행기
type ScriptExecutor struct {
    session         *InteractiveSession
    continueOnError bool
    dryRun          bool
    verbose         bool
    writer          io.Writer
}

// ScriptResult - 스크립트 실행 결과
type ScriptResult struct {
    Total    int
    Success  int
    Failed   int
    Skipped  int
    Errors   []ScriptError
}

// ScriptError - 개별 명령어 에러 정보
type ScriptError struct {
    Line    int
    Command string
    Error   error
}
```

```go
// chzyerReadline - chzyer/readline 기반 readlineInterface 구현체
type chzyerReadline struct {
    instance *readline.Instance
}

// readlineInterface 를 만족 (기존 인터페이스 유지)
func (r *chzyerReadline) Readline() (string, error)
func (r *chzyerReadline) SetPrompt(prompt string)
func (r *chzyerReadline) Close() error
func (r *chzyerReadline) SaveHistory(cmd string) error
```

### 4.5 readline 설정

```go
// readline.Config 설정
config := &readline.Config{
    Prompt:              "xflow> ",
    HistoryFile:         filepath.Join(homeDir, ".xflow", "history"),
    HistoryLimit:        1000,
    AutoComplete:        completerAdapter, // AutoCompleter 래핑
    InterruptPrompt:     "^C",
    EOFPrompt:           "exit",
    HistorySearchFold:   true, // 대소문자 무시 검색
}
```

### 4.6 설정 파일 확장 (`~/.xflow/config.yaml`)

```yaml
# 기존 interactive 설정에 추가
interactive:
  history_size: 1000
  history_file: "~/.xflow/history"
  autocomplete_cache_ttl: 30s

script:
  continue_on_error: false       # 기본 에러 시 중단
  verbose: false                 # 기본 verbose 비활성화
  env_substitution: true         # 환경 변수 치환 활성화 (P2)
```

---

## 5. 구현 우선순위

| 우선순위 | 모듈 | 근거 |
|---------|------|------|
| P0 (핵심) | Script Executor (script.go) | 배치 자동화의 핵심 기능 |
| P0 (핵심) | root.go 수정 | script 서브커맨드 등록 |
| P0 (핵심) | interactive.go 수정 | source/run REPL 명령어 추가 |
| P1 (중요) | Readline Adapter (readline_adapter.go) | 실질적인 터미널 조작 기능 |
| P1 (중요) | interactive.go 수정 | readline 구현체 교체, 히스토리 명령어 확장 |
| P1 (중요) | autocomplete.go 수정 | readline CompleteFunc 어댑터 |
| P2 (선택) | 환경 변수 치환 | 스크립트 유연성 향상 |
| P2 (선택) | Dry-run / Verbose 모드 | 디버깅 및 검증 도구 |

---

## 6. 추적성 (Traceability)

| 요구사항 ID | 관련 SPEC | 설명 |
|------------|----------|------|
| REQ-SCR-001~009 | SPEC-CLI-001 | executeCommand() 재사용, 에러 처리 패턴 활용 |
| REQ-SCR-003 | SPEC-CLI-002 | REPL 세션 내 source/run 명령어 통합 |
| REQ-SCR-010~015 | SPEC-CLI-002 | 기존 readlineInterface 구현체 교체, AutoCompleter 연동 |
| REQ-SCR-011 | SPEC-CLI-002 (REQ-INT-008) | 히스토리 영속화 요구사항 실질 구현 |
| REQ-SCR-012~013 | SPEC-CLI-002 (REQ-INT-009~010) | 화살표 키/Ctrl+R 요구사항 실질 구현 |
| REQ-SCR-015 | SPEC-CLI-002 (REQ-INT-007) | AutoCompleter readline 통합 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-19*
*작성: MoAI SPEC Builder*
