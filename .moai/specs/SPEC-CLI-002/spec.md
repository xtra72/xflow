---
id: SPEC-CLI-002
version: "1.0.0"
status: completed
created: "2026-02-17"
updated: "2026-02-18"
author: xtra
priority: medium
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-17 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-CLI-002: Interactive Mode - REPL + Wizard 하이브리드 CLI

## 1. Environment (환경)

### 1.1 시스템 개요

xflow CLI 도구의 Interactive Mode는 REPL(Read-Eval-Print Loop)과 Wizard 하이브리드 접근 방식을 제공하는 대화형 CLI 확장 기능이다. 기존 SPEC-CLI-001의 Cobra 기반 명령어 체계 위에 구축되며, 사용자가 `xflow interactive` 명령어를 실행하면 대화형 셸 세션에 진입한다.

본 SPEC은 다음을 포함한다:

- **REPL Core** (`interactive.go`): readline 기반 대화형 루프, 프롬프트 표시, 명령어 파싱/실행, 세션 관리
- **Tab Completion Engine** (`autocomplete.go`): 명령어/서브커맨드/플래그 자동완성, API 리소스 이름 동적 자동완성
- **Wizard Mode** (`wizard.go`): survey/v2 기반 단계별 가이드 프롬프트, 파라미터 수집, 요약/확인, 실행

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/cli/` (기존 CLI 패키지 내 확장)
- **Tier**: internal (비공개 패키지)
- **CLI 프레임워크**: `github.com/spf13/cobra` v1.8+ (SPEC-CLI-001과 동일)
- **REPL 라이브러리**: `github.com/chzyer/readline` - 줄 편집, 히스토리, 시그널 처리
- **Wizard 라이브러리**: `github.com/AlecAivazis/survey/v2` - 다중 단계 프롬프트, 유효성 검사
- **의존 SPEC**:
  - SPEC-CLI-001: 기본 CLI 시스템 (root.go, client.go, output.go, errors.go)
  - SPEC-API-001: CLI가 소비하는 REST API 엔드포인트 (자동완성용 리소스 조회)
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`

### 1.3 의존성

#### SPEC-CLI-001 의존 모듈

| 모듈 | 파일 | 사용 용도 |
|------|------|----------|
| Root Command | root.go | `interactive` 서브커맨드 등록 |
| API Client | client.go | REPL 내 API 호출, 자동완성용 리소스 조회 |
| Output Formatter | output.go | REPL 출력 포맷팅, 프로그레스 표시기 |
| Error Types | errors.go | 에러 메시지 및 사용자 안내 |

#### 외부 라이브러리

| 라이브러리 | 버전 | 용도 |
|-----------|------|------|
| github.com/chzyer/readline | v1.5+ | REPL 줄 편집, 히스토리, Ctrl+R 검색 |
| github.com/AlecAivazis/survey/v2 | v2.3+ | Wizard 단계별 프롬프트 |

### 1.4 제약 조건

- **순수 API 클라이언트 유지**: Interactive Mode 역시 모든 데이터 조작은 원격 xflowd 서버를 통해 수행
- **기존 명령어 호환**: REPL 내에서 실행되는 명령어는 셸에서 실행하는 것과 동일한 결과를 생성
- **크로스 플랫폼**: Linux, macOS에서 REPL/Wizard 동작 보장 (Windows는 readline 제한으로 최선 노력)
- **비대화형 환경 감지**: stdin이 TTY가 아닌 경우 interactive 모드 진입을 거부하고 안내 메시지 출력
- **기존 출력 포맷 재사용**: REPL 내 출력은 output.go의 기존 Formatter를 그대로 사용

### 1.5 스코프 경계

**포함 (In Scope)**:
- REPL 세션 관리 (시작, 종료, 시그널 처리)
- 명령어 파싱 및 Cobra 명령어 트리 실행 위임
- readline 기반 줄 편집 및 히스토리
- 명령어/서브커맨드/플래그 탭 자동완성
- API 리소스 이름(플로우명, Agent ID 등) 동적 자동완성
- Wizard 모드 단계별 프롬프트
- 명령어 히스토리 파일 영속화 (`~/.xflow/history`)

**제외 (Out of Scope)**:
- TUI 대시보드 (bubbletea/tview 기반 전체화면 UI)
- 멀티라인 스크립트 편집기
- REPL 내 플러그인 시스템
- 원격 세션 공유 / 멀티플레이어 REPL

---

## 2. Assumptions (가정 사항)

### 2.1 기술 가정

- **AS-INT-001**: SPEC-CLI-001의 root.go, client.go, output.go, errors.go가 구현되어 있다
- **AS-INT-002**: Cobra 명령어 트리가 프로그래밍 방식으로 접근 가능하다 (명령어 순회, 서브커맨드 목록)
- **AS-INT-003**: `github.com/chzyer/readline`이 macOS/Linux에서 안정적으로 동작한다
- **AS-INT-004**: `github.com/AlecAivazis/survey/v2`가 TTY 환경에서 멀티라인 입력을 지원한다
- **AS-INT-005**: API 리소스 목록 조회(자동완성용)가 합리적인 응답 시간(< 500ms) 내에 반환된다

### 2.2 운영 가정

- **AS-INT-006**: 사용자는 터미널(TTY) 환경에서 interactive 모드를 사용한다
- **AS-INT-007**: 히스토리 파일(`~/.xflow/history`)에 대한 읽기/쓰기 권한이 있다
- **AS-INT-008**: REPL 세션 중 서버 연결이 유지된다 (연결 끊김 시 에러 메시지 표시 후 재시도 가능)

---

## 3. Requirements (요구사항)

### 3.1 P0 (핵심) - REPL Core

#### REQ-INT-001: REPL 세션 시작
**WHEN** 사용자가 `xflow interactive`를 실행할 때, **THEN** 시스템은 대화형 REPL 세션을 시작해야 한다.
시작 시 환영 메시지와 도움말 안내(`help` 또는 `?`로 명령어 목록 조회)를 표시해야 한다.

#### REQ-INT-002: 컨텍스트 인식 프롬프트
**WHILE** REPL 모드에 있는 동안, 시스템은 컨텍스트 인식 프롬프트를 표시해야 한다.
- 기본 프롬프트: `xflow> `
- 서버 연결 시: `xflow [서버호스트]> ` (예: `xflow [localhost:8080]> `)
- 서버 미연결 시: `xflow [disconnected]> `

#### REQ-INT-003: 명령어 파싱 및 실행
**WHEN** 사용자가 REPL에서 유효한 CLI 명령어를 입력할 때, **THEN** 시스템은 해당 명령어를 셸에서 실행한 것과 동일하게 파싱하고 실행해야 한다.
`xflow` 접두사는 선택적이다 (예: `flow list`와 `xflow flow list` 모두 동작).

#### REQ-INT-004: 세션 종료
**WHEN** 사용자가 `exit`, `quit`을 입력하거나 Ctrl+D를 누를 때, **THEN** 시스템은 REPL 세션을 정상적으로 종료해야 한다.
종료 시 "세션을 종료합니다." 메시지를 표시해야 한다.

#### REQ-INT-005: 잘못된 명령어 처리
**WHEN** 사용자가 잘못된 명령어를 입력할 때, **THEN** 시스템은 에러 메시지와 함께 가장 유사한 명령어를 제안해야 한다.
Levenshtein 거리 기반 또는 접두사 매칭으로 유사 명령어를 식별해야 한다.

#### REQ-INT-006: 기존 출력 포맷터 사용
**WHILE** REPL 모드에 있는 동안, 시스템은 명령어 출력을 기존 output.go 포맷터를 사용하여 표시해야 한다.
`--format` 플래그는 REPL 세션 내에서도 명령어별로 지정 가능해야 한다.

### 3.2 P1 (중요) - Enhanced REPL

#### REQ-INT-007: 탭 자동완성 - 명령어
**WHILE** REPL 모드에 있는 동안, 시스템은 Tab 키를 통해 명령어, 서브커맨드, 플래그에 대한 자동완성을 제공해야 한다.
Cobra 명령어 트리를 기반으로 현재 입력 컨텍스트에 맞는 후보를 제안해야 한다.

#### REQ-INT-008: 히스토리 영속화
**WHILE** REPL 모드에 있는 동안, 시스템은 입력된 명령어 히스토리를 `~/.xflow/history` 파일에 저장해야 한다.
히스토리는 세션 간에 유지되어야 한다.

#### REQ-INT-009: 히스토리 탐색
**WHEN** 사용자가 Up/Down 화살표 키를 누를 때, **THEN** 시스템은 명령어 히스토리를 순방향/역방향으로 탐색해야 한다.

#### REQ-INT-010: 역방향 히스토리 검색
**WHILE** REPL 모드에 있는 동안, 시스템은 Ctrl+R을 통한 역방향 히스토리 검색을 지원해야 한다.
검색어 입력 시 일치하는 가장 최근 명령어를 실시간으로 표시해야 한다.

#### REQ-INT-011: 탭 자동완성 - API 리소스
**WHEN** 사용자가 부분 명령어를 입력하고 Tab을 누를 때, **THEN** 시스템은 API를 쿼리하여 리소스 이름(플로우 이름, Agent ID 등)을 자동완성해야 한다.
자동완성 결과는 캐싱하여 반복 호출을 최소화해야 한다 (캐시 TTL: 30초).

### 3.3 P2 (선택) - Wizard Mode

#### REQ-INT-012: Wizard 모드 진입
**WHEN** 사용자가 `xflow interactive --wizard flow deploy` 또는 REPL 내에서 `wizard flow deploy`를 입력할 때, **THEN** 시스템은 단계별 Wizard 모드를 시작해야 한다.

#### REQ-INT-013: 단계별 파라미터 수집
**WHILE** Wizard 모드에 있는 동안, 시스템은 필수 파라미터 각각에 대해 프롬프트를 표시하고, 기본값 제안과 입력 유효성 검사를 수행해야 한다.
파라미터 순서는 명령어의 필수 인자 -> 선택 플래그 순으로 진행해야 한다.

#### REQ-INT-014: 복합 데이터 입력
**WHILE** Wizard 모드에 있는 동안, 시스템은 JSON/YAML 형식의 복합 파라미터에 대해 멀티라인 입력을 지원해야 한다.
에디터(survey/v2의 Editor 프롬프트)를 통해 구조화된 데이터 입력이 가능해야 한다.

#### REQ-INT-015: 실행 전 요약 및 확인
**WHEN** Wizard가 모든 파라미터 수집을 완료했을 때, **THEN** 시스템은 수집된 파라미터 요약을 표시하고 실행 확인을 요청해야 한다.
확인 시 해당 명령어를 실행하고, 취소 시 Wizard를 종료해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/cli/
  interactive.go             # REPL 메인 루프, cobra 명령어, readline 통합
  interactive_test.go        # REPL 테스트
  autocomplete.go            # 탭 자동완성 엔진 (명령어 + API 리소스)
  autocomplete_test.go       # 자동완성 테스트
  wizard.go                  # Wizard 단계별 프롬프트 (survey/v2)
  wizard_test.go             # Wizard 테스트
  root.go                    # (수정) interactive 서브커맨드 등록
```

### 4.2 명령어 트리 확장

```
xflow
  ├── interactive                    # REPL 대화형 모드 진입
  │   └── --wizard <command>         # Wizard 모드로 직접 진입 (P2)
  ├── (기존 명령어...)
  └── ...
```

### 4.3 REPL 세션 내 특수 명령어

| 명령어 | 설명 |
|--------|------|
| `help` 또는 `?` | 사용 가능한 명령어 목록 표시 |
| `exit` 또는 `quit` | REPL 세션 종료 |
| `clear` | 화면 지우기 |
| `wizard <command>` | 지정된 명령어의 Wizard 모드 진입 (P2) |
| `history` | 최근 명령어 히스토리 표시 |

### 4.4 핵심 인터페이스

```go
// InteractiveSession - REPL 세션 인터페이스
type InteractiveSession interface {
    Start() error
    Stop()
}

// Completer - 자동완성 인터페이스
type Completer interface {
    Complete(line string, pos int) ([]string, int)
}

// WizardRunner - Wizard 실행 인터페이스
type WizardRunner interface {
    Run(commandPath string) error
}
```

### 4.5 설정 파일 확장 (`~/.xflow/config.yaml`)

```yaml
# 기존 설정에 추가
interactive:
  prompt_style: "default"        # default | minimal | full
  history_size: 1000             # 히스토리 최대 항목 수
  autocomplete_cache_ttl: 30s    # API 리소스 자동완성 캐시 TTL
```

### 4.6 히스토리 파일

- **경로**: `~/.xflow/history`
- **형식**: 한 줄에 하나의 명령어 (readline 표준 형식)
- **크기 제한**: 기본 1000 항목, 초과 시 오래된 항목부터 제거

---

## 5. 구현 우선순위

| 우선순위 | 모듈 | 근거 |
|---------|------|------|
| P0 (핵심) | REPL Core (interactive.go) | 대화형 모드의 기본 기능 |
| P0 (핵심) | root.go 수정 | interactive 서브커맨드 등록 |
| P1 (중요) | Tab Completion (autocomplete.go) | 사용성 핵심 향상 |
| P1 (중요) | History Persistence | 세션 간 연속성 |
| P2 (선택) | Wizard Mode (wizard.go) | 초보 사용자 경험 향상 |

---

## 6. 추적성 (Traceability)

| 요구사항 ID | 관련 SPEC | 설명 |
|------------|----------|------|
| REQ-INT-001~006 | SPEC-CLI-001 | root.go, client.go, output.go, errors.go 재사용 |
| REQ-INT-003 | SPEC-CLI-001 | Cobra 명령어 트리를 REPL에서 실행 |
| REQ-INT-006 | SPEC-CLI-001 (output.go) | 기존 OutputFormatter 인터페이스 활용 |
| REQ-INT-011 | SPEC-API-001 | API 엔드포인트를 통한 리소스 이름 조회 (자동완성) |
| REQ-INT-012~015 | SPEC-CLI-001 | 기존 명령어의 Wizard 래핑 |

---

---

## 7. 구현 결과 (Implementation Notes)

- **구현 완료일**: 2026-02-18
- **구현 범위**: 전체 15개 요구사항 (REQ-INT-001~015) 구현 완료
  - P0 (핵심) REPL Core: REQ-INT-001~006 -- 구현 완료
  - P1 (중요) Enhanced REPL: REQ-INT-007~011 -- 구현 완료
  - P2 (선택) Wizard Mode: REQ-INT-012~015 -- 구현 완료
- **테스트 커버리지**: 91.7% (interactive: 25개 테스트 함수/70+ 서브테스트, autocomplete: 16개, wizard: 18개)
- **Race Detector**: 이상 없음 (go test -race)
- **커밋**: 7d78c89

### 구현 파일

| 파일 | 라인 수 | 설명 |
|------|---------|------|
| `interactive.go` | 468 | REPL 코어: readline 세션, Cobra 실행 위임, 시그널 처리, 유사 명령어 제안 |
| `interactive_test.go` | 1014 | REPL 테스트: 25개 테스트 함수, 70+ 서브테스트 |
| `autocomplete.go` | 340 | 탭 자동완성: 명령어/서브커맨드/플래그/API 리소스, TTL 캐시 |
| `autocomplete_test.go` | 782 | 자동완성 테스트: 16개 테스트 함수 |
| `wizard.go` | 258 | Wizard 모드: survey/v2 기반 단계별 프롬프트 |
| `wizard_test.go` | 686 | Wizard 테스트: 18개 테스트 함수 |
| `root.go` | (수정) | `newInteractiveCmd(rootCmd, &client)` 등록 추가 |

### 추가된 외부 의존성

| 라이브러리 | 버전 | 용도 |
|-----------|------|------|
| `github.com/chzyer/readline` | v1.5.1 | REPL 줄 편집, 히스토리, Ctrl+R 검색 |
| `github.com/AlecAivazis/survey/v2` | v2.3.7 | Wizard 단계별 프롬프트 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-18*
*작성: MoAI SPEC Builder*
