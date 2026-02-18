---
id: SPEC-CLI-002
type: plan
version: "1.0.0"
status: draft
created: "2026-02-17"
updated: "2026-02-18"
author: xtra
---

# SPEC-CLI-002: Interactive Mode - 구현 계획

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 준수):
- **신규 코드 (TDD)**: `interactive.go`, `autocomplete.go`, `wizard.go` 전체가 신규 작성이므로 RED-GREEN-REFACTOR 사이클 적용
- 테스트 커버리지 목표: 85% 이상
- **기존 코드 (DDD)**: `root.go` 수정은 기존 테스트 보존 후 변경 적용

### 1.2 핵심 설계 결정

1. **readline 기반 REPL**: `github.com/chzyer/readline`은 줄 편집, 히스토리, 시그널 처리, 자동완성 콜백을 네이티브로 지원하므로 별도 구현 최소화
2. **Cobra 명령어 트리 재사용**: REPL 입력을 `os.Args` 형태로 변환하여 기존 Cobra `rootCmd.SetArgs()` / `rootCmd.Execute()`로 위임, 코드 중복 제거
3. **자동완성 엔진 분리**: `autocomplete.go`를 독립 모듈로 분리하여 Cobra 트리 기반 정적 완성 + API 기반 동적 완성을 통합
4. **survey/v2 Wizard**: Wizard 모드는 survey/v2의 프롬프트 체인으로 구현하여 유효성 검사 및 기본값 제안을 선언적으로 정의
5. **인터페이스 기반 설계**: `InteractiveSession`, `Completer`, `WizardRunner` 인터페이스를 정의하여 테스트 시 모킹 가능

### 1.3 기술 스택

| 구성 요소 | 선택 | 비고 |
|-----------|------|------|
| REPL Engine | github.com/chzyer/readline v1.5+ | 줄 편집, 히스토리, 시그널 |
| Wizard Prompts | github.com/AlecAivazis/survey/v2 v2.3+ | 단계별 프롬프트 |
| CLI Framework | github.com/spf13/cobra v1.8+ (기존) | 명령어 트리 재사용 |
| Testing | stretchr/testify v1.9+ (기존) | 테스트 어설션 |

---

## 2. 파일별 구현 상세

### 2.1 P0 핵심 파일 (마일스톤 1)

#### `internal/cli/interactive.go` (~200 라인)

- `InteractiveSession` 구조체:
  - `rl *readline.Instance`: readline 인스턴스
  - `rootCmd *cobra.Command`: Cobra 루트 명령어 참조
  - `client *HTTPClient`: API 클라이언트 (프롬프트용 서버 상태 확인)
  - `formatter OutputFormatter`: 출력 포맷터
  - `completer *AutoCompleter`: 자동완성 엔진 참조
- `NewInteractiveSession(rootCmd, client, formatter) *InteractiveSession`
- `Start() error`: REPL 메인 루프
  - readline.Config 설정: 프롬프트, 히스토리 파일, AutoComplete, Ctrl+C/D 핸들링
  - 환영 메시지 출력
  - 루프: `rl.Readline()` -> 입력 트리밍 -> 특수 명령어 처리 -> Cobra 실행 위임
- `Stop()`: readline 인스턴스 정리, 종료 메시지 출력
- `executeCommand(input string) error`: 입력 문자열을 `shlex` 방식으로 토큰화 -> `rootCmd.SetArgs(args)` -> `rootCmd.ExecuteC()`
  - `xflow` 접두사 자동 제거
  - 에러 발생 시 output.go 에러 포맷터 사용
- `buildPrompt() string`: 서버 연결 상태 기반 동적 프롬프트 생성
- `handleSpecialCommand(input string) (bool, error)`: `help`, `exit`, `quit`, `clear`, `history`, `wizard` 처리
- `suggestCommand(input string) string`: Levenshtein 거리 기반 유사 명령어 제안
- `newInteractiveCmd(rootCmd, client, formatter) *cobra.Command`: Cobra `interactive` 명령어 정의
  - `--wizard` 플래그: Wizard 모드 직접 진입 (P2)
- 의존성: `readline`, `cobra`, `client.go`, `output.go`, `errors.go`

#### `internal/cli/interactive_test.go` (~150 라인)

- 명령어 파싱 테스트: 입력 문자열 -> 토큰 배열 변환 검증
- 특수 명령어 처리 테스트: `help`, `exit`, `quit`, `clear`, `history`
- 프롬프트 생성 테스트: 서버 연결/비연결 상태별 프롬프트 형식
- 유사 명령어 제안 테스트: 잘못된 입력 -> 가장 가까운 명령어 제안
- `xflow` 접두사 제거 테스트: `xflow flow list` -> `flow list`
- 비TTY 환경 감지 테스트: stdin이 파이프일 때 에러 반환

#### `internal/cli/root.go` (수정, ~5 라인 변경)

- `NewRootCmd()` 함수 내 `interactive` 서브커맨드 등록 추가:
  - `rootCmd.AddCommand(newInteractiveCmd(rootCmd, client, formatter))`
- 의존성: `interactive.go`

### 2.2 P1 확장 파일 (마일스톤 2)

#### `internal/cli/autocomplete.go` (~150 라인)

- `AutoCompleter` 구조체:
  - `rootCmd *cobra.Command`: Cobra 명령어 트리
  - `client *HTTPClient`: API 리소스 조회용
  - `cache map[string]cacheEntry`: 리소스 캐시
  - `cacheTTL time.Duration`: 캐시 유효 기간 (기본 30초)
- `NewAutoCompleter(rootCmd, client, cacheTTL) *AutoCompleter`
- `Do(line []rune, pos int) (newLine [][]rune, length int)`: readline.AutoCompleter 인터페이스 구현
  - 입력 파싱 -> 현재 위치의 컨텍스트 판별 (명령어/서브커맨드/플래그/인자)
  - 명령어/서브커맨드: Cobra 트리 순회로 후보 목록 생성
  - 플래그: 현재 명령어의 등록된 플래그 목록에서 매칭
  - 인자(리소스 ID): API 호출로 동적 리소스 이름 조회
- `completeCommands(tokens []string) []string`: 명령어 트리 기반 정적 완성
- `completeFlags(cmd *cobra.Command, prefix string) []string`: 플래그 완성
- `completeResource(resourceType, prefix string) []string`: API 기반 동적 완성
  - `flow` -> GET /api/v1/flows -> 플로우 이름/ID 목록
  - `agent` -> GET /api/v1/agents -> Agent 이름/ID 목록
  - 결과를 캐시에 저장, TTL 초과 시 갱신
- `resolveResourceType(tokens []string) string`: 토큰 컨텍스트에서 리소스 타입 추론
- 의존성: `cobra`, `client.go`

#### `internal/cli/autocomplete_test.go` (~100 라인)

- 명령어 자동완성 테스트: `fl` + Tab -> `flow`
- 서브커맨드 자동완성 테스트: `flow l` + Tab -> `flow list`
- 플래그 자동완성 테스트: `flow list --f` + Tab -> `flow list --format`
- 리소스 자동완성 테스트: `flow get my-` + Tab -> `flow get my-flow-1` (Mock API)
- 캐시 동작 테스트: 첫 호출 API 조회, 두 번째 호출 캐시 히트
- 캐시 만료 테스트: TTL 초과 후 API 재조회

### 2.3 P2 확장 파일 (마일스톤 3)

#### `internal/cli/wizard.go` (~200 라인)

- `WizardRunner` 구조체:
  - `rootCmd *cobra.Command`: 명령어 트리 참조
  - `client *HTTPClient`: API 클라이언트
  - `formatter OutputFormatter`: 출력 포맷터
- `NewWizardRunner(rootCmd, client, formatter) *WizardRunner`
- `Run(commandPath string) error`: Wizard 메인 진입점
  - 명령어 경로 파싱 (예: `flow deploy`)
  - Cobra 명령어 탐색 -> 필수 인자 및 플래그 추출
  - 단계별 프롬프트 생성 -> survey.Ask() 실행
  - 요약 표시 -> 확인 -> 명령어 실행
- `buildPrompts(cmd *cobra.Command) []*survey.Question`: 명령어의 Args/Flags를 survey 질문으로 변환
  - 필수 인자: survey.Input 프롬프트
  - Bool 플래그: survey.Confirm 프롬프트
  - Enum 플래그 (format 등): survey.Select 프롬프트
  - 파일 입력 (-f 플래그): survey.Input + 파일 존재 검증
  - 복합 데이터: survey.Editor 프롬프트 (JSON/YAML 멀티라인)
- `showSummary(cmd *cobra.Command, answers map[string]any) string`: 수집된 파라미터 요약 테이블 생성
- `confirmAndExecute(cmd *cobra.Command, answers map[string]any) error`: 확인 프롬프트 후 실행
- `resolveSelectOptions(flagName string) []string`: 플래그별 선택지 조회
  - `--format`: `["table", "json", "yaml", "text"]`
  - 리소스 선택: API 조회로 동적 목록 생성
- 의존성: `survey/v2`, `cobra`, `client.go`, `output.go`

#### `internal/cli/wizard_test.go` (~150 라인)

- 프롬프트 생성 테스트: 명령어별 올바른 질문 목록 생성 검증
- 요약 출력 테스트: 수집된 답변 -> 포맷된 요약 테이블
- 명령어 경로 파싱 테스트: `flow deploy` -> Cobra 명령어 탐색
- 유효성 검사 테스트: 빈 필수 값, 잘못된 파일 경로 등
- 확인/취소 플로우 테스트: 확인 시 실행, 취소 시 미실행
- survey.AskOne 모킹 기반 통합 테스트

---

## 3. 마일스톤

### 마일스톤 1: REPL Core (Primary Goal)

**목표**: 대화형 REPL 세션의 기본 기능 구현

| 순서 | 파일 | 예상 라인 | 설명 |
|------|------|----------|------|
| 1 | interactive.go | ~200 | REPL 메인 루프, readline 통합, 세션 관리 |
| 2 | interactive_test.go | ~150 | REPL 단위 테스트 |
| 3 | root.go (수정) | ~5 변경 | interactive 서브커맨드 등록 |

**예상 총 라인**: ~355 라인

**완료 기준**:
- `xflow interactive` 실행 시 REPL 세션 진입
- 컨텍스트 인식 프롬프트 표시 (서버 연결 상태 반영)
- REPL 내에서 기존 CLI 명령어 실행 가능 (`flow list`, `agent list` 등)
- `exit`, `quit`, Ctrl+D로 세션 종료
- 잘못된 명령어 입력 시 유사 명령어 제안
- 비TTY 환경에서 진입 거부 및 안내 메시지
- 테스트 커버리지 85% 이상

### 마일스톤 2: Enhanced REPL (Secondary Goal)

**목표**: 탭 자동완성 및 히스토리 영속화

| 순서 | 파일 | 예상 라인 | 설명 |
|------|------|----------|------|
| 1 | autocomplete.go | ~150 | 자동완성 엔진 (명령어 + API 리소스) |
| 2 | autocomplete_test.go | ~100 | 자동완성 테스트 |

**예상 총 라인**: ~250 라인

**완료 기준**:
- Tab 키로 명령어/서브커맨드/플래그 자동완성
- Tab 키로 API 리소스 이름 동적 자동완성 (플로우명, Agent ID)
- 자동완성 결과 캐싱 (TTL 30초)
- 명령어 히스토리 `~/.xflow/history` 영속화
- Up/Down 화살표로 히스토리 탐색
- Ctrl+R로 역방향 히스토리 검색
- 테스트 커버리지 85% 이상

### 마일스톤 3: Wizard Mode (Final Goal)

**목표**: 단계별 가이드 Wizard 프롬프트

| 순서 | 파일 | 예상 라인 | 설명 |
|------|------|----------|------|
| 1 | wizard.go | ~200 | Wizard 단계별 프롬프트 |
| 2 | wizard_test.go | ~150 | Wizard 테스트 |

**예상 총 라인**: ~350 라인

**완료 기준**:
- `wizard flow deploy` 또는 `xflow interactive --wizard flow deploy`로 Wizard 진입
- 필수 파라미터 단계별 프롬프트 (기본값, 유효성 검사)
- JSON/YAML 복합 데이터 멀티라인 입력 지원
- 파라미터 요약 표시 및 실행 확인
- 확인 시 명령어 실행, 취소 시 Wizard 종료
- 테스트 커버리지 85% 이상

---

## 4. 의존성 그래프

```
internal/cli/root.go (수정)
    └── internal/cli/interactive.go     (REPL 세션)
            ├── github.com/chzyer/readline
            ├── internal/cli/client.go     (SPEC-CLI-001)
            ├── internal/cli/output.go     (SPEC-CLI-001)
            ├── internal/cli/errors.go     (SPEC-CLI-001)
            ├── internal/cli/autocomplete.go  (자동완성)
            │     ├── github.com/spf13/cobra
            │     └── internal/cli/client.go
            └── internal/cli/wizard.go     (Wizard)
                  ├── github.com/AlecAivazis/survey/v2
                  ├── github.com/spf13/cobra
                  ├── internal/cli/client.go
                  └── internal/cli/output.go

외부 의존 SPEC:
    SPEC-CLI-001 ────── 기본 CLI 시스템 (root.go, client.go, output.go, errors.go)
    SPEC-API-001 ────── REST API 엔드포인트 (자동완성용 리소스 조회)
```

구현 순서 제약:
- SPEC-CLI-001의 P0 모듈이 먼저 구현되어야 함 (root.go, client.go, output.go, errors.go)
- `interactive.go` -> `autocomplete.go` -> `wizard.go` 순서 (후속 모듈이 이전 모듈에 의존)
- `autocomplete.go`와 `wizard.go`는 `interactive.go` 이후에 독립적으로 병렬 구현 가능

---

## 5. 리스크 분석

### R1: readline 크로스 플랫폼 호환성 (확률: 중 / 영향: 중)
- **설명**: `chzyer/readline`이 Windows에서 일부 기능(Ctrl+R, 시그널 처리)이 제한될 수 있다
- **대응**: Linux/macOS를 1차 지원 플랫폼으로 설정. Windows는 기본 REPL 기능만 보장. 빌드 태그로 플랫폼별 분기

### R2: Cobra 명령어 재실행 부작용 (확률: 중 / 영향: 고)
- **설명**: Cobra 명령어를 REPL 루프 내에서 반복 실행 시 전역 상태(Viper 바인딩, 플래그 기본값)가 누적될 수 있다
- **대응**: 매 실행마다 `rootCmd.SetArgs()`로 인자를 초기화하고, 실행 후 플래그 상태를 리셋. `rootCmd.ResetFlags()`는 동작하지 않을 수 있으므로 별도 `resetCmdState()` 함수 구현

### R3: API 자동완성 지연 (확률: 중 / 영향: 저)
- **설명**: 서버 응답이 느린 경우 Tab 자동완성 시 UI가 멈추는 느낌을 줄 수 있다
- **대응**: 자동완성 요청에 500ms 타임아웃 적용. 타임아웃 시 정적 완성 결과만 반환. 백그라운드 캐시 갱신

### R4: survey/v2 TTY 의존성 (확률: 저 / 영향: 중)
- **설명**: survey/v2는 TTY를 요구하며, CI/CD 환경이나 파이프 연결 시 패닉이 발생할 수 있다
- **대응**: Wizard 진입 전 TTY 확인. 비TTY 환경에서 Wizard 시도 시 명확한 에러 메시지

### R5: 히스토리 파일 동시 접근 (확률: 저 / 영향: 저)
- **설명**: 여러 REPL 세션이 동시에 실행될 경우 히스토리 파일 쓰기 충돌 가능
- **대응**: readline의 기본 히스토리 관리에 위임 (append 방식). 파일 잠금은 과도한 복잡성이므로 미적용

---

## 6. 파일 의존성 매트릭스

| 파일 | errors.go | output.go | client.go | root.go | cobra | readline | survey/v2 |
|------|:---------:|:---------:|:---------:|:-------:|:-----:|:--------:|:---------:|
| interactive.go | O | O | O | O | O | O | |
| autocomplete.go | | | O | | O | O | |
| wizard.go | | O | O | | O | | O |
| root.go (수정) | | | | - | O | | |

O = 의존

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-18*
*작성: MoAI manager-spec*
