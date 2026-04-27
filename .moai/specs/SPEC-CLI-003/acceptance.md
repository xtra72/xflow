---
id: SPEC-CLI-003
version: "1.0.0"
status: draft
created: "2026-02-19"
updated: "2026-02-19"
author: xtra
priority: medium
---

# SPEC-CLI-003: 인수 기준 - 스크립트 실행 및 명령어 히스토리 강화

## 1. P0 - Script Execution 인수 기준

### AC-SCR-001: 스크립트 파일 파싱

**Scenario 1: 기본 스크립트 파싱**

```gherkin
Given 다음 내용의 스크립트 파일 "test.xflow"가 존재한다:
  """
  # 서버 상태 확인
  status

  flow list --format json

  # 플로우 배포
  flow deploy my-flow
  """
When 시스템이 "test.xflow" 파일을 파싱한다
Then 3개의 실행 가능한 명령어가 추출된다
And 주석 줄과 빈 줄은 제외된다
And 명령어 순서는 "status", "flow list --format json", "flow deploy my-flow"이다
```

**Scenario 2: 빈 파일 처리**

```gherkin
Given 빈 스크립트 파일 "empty.xflow"가 존재한다
When 시스템이 "empty.xflow" 파일을 파싱한다
Then 0개의 명령어가 추출된다
And 에러가 발생하지 않는다
```

**Scenario 3: 주석만 있는 파일**

```gherkin
Given 주석만 포함된 스크립트 파일 "comments.xflow"가 존재한다:
  """
  # 이것은 주석입니다
  # 실행할 명령어가 없습니다
  """
When 시스템이 "comments.xflow" 파일을 파싱한다
Then 0개의 명령어가 추출된다
And 에러가 발생하지 않는다
```

**Scenario 4: 줄 끝 공백 제거**

```gherkin
Given "  flow list  " (앞뒤 공백 포함) 줄이 있는 스크립트 파일이 존재한다
When 시스템이 해당 파일을 파싱한다
Then 추출된 명령어는 "flow list"이다 (앞뒤 공백 제거)
```

---

### AC-SCR-002: CLI 플래그를 통한 스크립트 실행

**Scenario 1: 정상 스크립트 실행**

```gherkin
Given "flow list" 명령어가 포함된 유효한 스크립트 파일 "commands.xflow"가 존재한다
When 사용자가 "xflow script -f commands.xflow"를 실행한다
Then 시스템은 스크립트 내 각 명령어를 순차적으로 실행한다
And 종료 코드 0을 반환한다
```

**Scenario 2: --file 장문 플래그**

```gherkin
Given 유효한 스크립트 파일 "commands.xflow"가 존재한다
When 사용자가 "xflow script --file commands.xflow"를 실행한다
Then "-f" 플래그와 동일하게 동작한다
```

---

### AC-SCR-003: REPL 내 스크립트 실행

**Scenario 1: source 명령어**

```gherkin
Given REPL 세션이 활성화되어 있다
And 유효한 스크립트 파일 "batch.xflow"가 존재한다
When 사용자가 REPL에서 "source batch.xflow"를 입력한다
Then 시스템은 해당 스크립트 파일을 로드하여 순차 실행한다
```

**Scenario 2: run 별칭**

```gherkin
Given REPL 세션이 활성화되어 있다
And 유효한 스크립트 파일 "batch.xflow"가 존재한다
When 사용자가 REPL에서 "run batch.xflow"를 입력한다
Then "source batch.xflow"와 동일하게 동작한다
```

**Scenario 3: 파일 인자 누락**

```gherkin
Given REPL 세션이 활성화되어 있다
When 사용자가 "source"만 입력한다 (파일 경로 없이)
Then 시스템은 사용법 안내 메시지를 출력한다
```

---

### AC-SCR-004: 순차 실행

**Scenario 1: 순서 보장**

```gherkin
Given 3개의 명령어(A, B, C)가 포함된 스크립트 파일이 존재한다
When 시스템이 해당 스크립트를 실행한다
Then 명령어 A가 먼저 실행된다
And 명령어 A 완료 후 명령어 B가 실행된다
And 명령어 B 완료 후 명령어 C가 실행된다
```

---

### AC-SCR-005: 에러 시 중단 (기본 동작)

**Scenario 1: 중간 명령어 실패 시 중단**

```gherkin
Given 3개의 명령어(A, B-실패, C)가 포함된 스크립트 파일이 존재한다
And --continue-on-error 플래그가 지정되지 않았다
When 시스템이 해당 스크립트를 실행한다
Then 명령어 A가 성공적으로 실행된다
And 명령어 B가 실행되고 실패한다
And 명령어 C는 실행되지 않는다 (건너뜀)
And 종료 코드가 0이 아니다
And 실패한 명령어 정보와 에러 내용이 출력된다
```

---

### AC-SCR-006: 에러 시 계속 실행 (옵션)

**Scenario 1: --continue-on-error 플래그**

```gherkin
Given 3개의 명령어(A, B-실패, C)가 포함된 스크립트 파일이 존재한다
And --continue-on-error 플래그가 지정되었다
When 시스템이 해당 스크립트를 실행한다
Then 명령어 A가 성공적으로 실행된다
And 명령어 B가 실행되고 실패한다
And 명령어 C가 실행된다 (건너뛰지 않음)
And 실행 결과 요약에 실패한 명령어 목록이 포함된다
```

---

### AC-SCR-007: 종료 코드

**Scenario 1: 전체 성공 시**

```gherkin
Given 모든 명령어가 성공하는 스크립트 파일이 존재한다
When 시스템이 해당 스크립트를 실행한다
Then 종료 코드 0을 반환한다
```

**Scenario 2: 실패 포함 시**

```gherkin
Given 하나 이상의 명령어가 실패하는 스크립트 파일이 존재한다
When 시스템이 해당 스크립트를 실행한다
Then 종료 코드가 0이 아닌 값이다
```

---

### AC-SCR-008: 파일 미발견 에러

**Scenario 1: 존재하지 않는 파일**

```gherkin
Given "nonexistent.xflow" 파일이 존재하지 않는다
When 사용자가 "xflow script -f nonexistent.xflow"를 실행한다
Then 시스템은 "파일을 찾을 수 없습니다: nonexistent.xflow" 에러 메시지를 출력한다
And 종료 코드 1을 반환한다
```

---

### AC-SCR-009: 실행 결과 요약

**Scenario 1: 전체 성공 요약**

```gherkin
Given 5개의 명령어가 포함된 스크립트 파일이 존재한다
And 모든 명령어가 성공한다
When 시스템이 해당 스크립트 실행을 완료한다
Then 실행 결과 요약이 출력된다
And 요약에 "전체: 5, 성공: 5, 실패: 0, 건너뜀: 0" 정보가 포함된다
```

**Scenario 2: 부분 실패 요약 (continue-on-error)**

```gherkin
Given 5개의 명령어가 포함된 스크립트 파일이 존재한다
And 2번째와 4번째 명령어가 실패한다
And --continue-on-error 플래그가 지정되었다
When 시스템이 해당 스크립트 실행을 완료한다
Then 실행 결과 요약이 출력된다
And 요약에 "전체: 5, 성공: 3, 실패: 2, 건너뜀: 0" 정보가 포함된다
And 실패한 명령어의 줄 번호와 에러 내용이 상세 출력된다
```

**Scenario 3: 에러 중단 요약**

```gherkin
Given 5개의 명령어가 포함된 스크립트 파일이 존재한다
And 2번째 명령어가 실패한다
And --continue-on-error 플래그가 지정되지 않았다
When 시스템이 해당 스크립트 실행을 완료한다
Then 실행 결과 요약이 출력된다
And 요약에 "전체: 5, 성공: 1, 실패: 1, 건너뜀: 3" 정보가 포함된다
```

---

## 2. P1 - Readline Integration 및 History Enhancement 인수 기준

### AC-SCR-010: chzyer/readline 기반 구현체

**Scenario 1: TTY 환경에서 readline 사용**

```gherkin
Given stdin이 TTY(터미널)에 연결되어 있다
When 사용자가 "xflow interactive"를 실행한다
Then 시스템은 chzyer/readline 기반의 입력 처리기를 사용한다
And 줄 편집 기능 (좌우 이동, 삭제, 삽입)이 동작한다
```

**Scenario 2: 비TTY 환경에서 폴백**

```gherkin
Given stdin이 파이프에 연결되어 있다 (비TTY)
When 시스템이 REPL 세션을 초기화한다
Then 시스템은 기존 stdioReadline 구현체를 사용한다
And 기존 동작과 동일하게 작동한다
```

---

### AC-SCR-011: 히스토리 파일 영속화

**Scenario 1: 히스토리 저장**

```gherkin
Given REPL 세션이 활성화되어 있다
When 사용자가 "flow list"를 입력하고 실행한다
Then 해당 명령어가 ~/.xflow/history 파일에 저장된다
```

**Scenario 2: 세션 간 히스토리 유지**

```gherkin
Given 이전 REPL 세션에서 "flow list"와 "agent list"를 실행했다
And ~/.xflow/history 파일에 해당 명령어가 저장되어 있다
When 새로운 REPL 세션을 시작한다
Then 이전 세션의 히스토리가 로드된다
And Up 화살표 키로 이전 명령어에 접근할 수 있다
```

**Scenario 3: 히스토리 크기 제한**

```gherkin
Given 히스토리 최대 크기가 1000으로 설정되어 있다
And 현재 히스토리에 1000개의 항목이 있다
When 새로운 명령어를 실행한다
Then 가장 오래된 항목이 제거된다
And 새 명령어가 히스토리 끝에 추가된다
And 총 히스토리 항목 수는 1000개를 유지한다
```

**Scenario 4: 히스토리 디렉토리 자동 생성**

```gherkin
Given ~/.xflow/ 디렉토리가 존재하지 않는다
When REPL 세션이 시작되고 명령어를 실행한다
Then 시스템은 ~/.xflow/ 디렉토리를 자동 생성한다
And 히스토리 파일이 정상적으로 생성된다
```

---

### AC-SCR-012: 화살표 키 히스토리 탐색

**Scenario 1: Up 화살표 키**

```gherkin
Given REPL 세션이 활성화되어 있다
And 히스토리에 "flow list", "agent list", "status" 명령어가 있다
When 사용자가 Up 화살표 키를 한 번 누른다
Then 입력 줄에 "status" (가장 최근 명령어)가 표시된다
When 사용자가 Up 화살표 키를 한 번 더 누른다
Then 입력 줄에 "agent list"가 표시된다
```

**Scenario 2: Down 화살표 키**

```gherkin
Given REPL 세션이 활성화되어 있다
And 사용자가 Up 화살표 키를 3번 눌러 히스토리를 탐색했다
When 사용자가 Down 화살표 키를 누른다
Then 더 최근 명령어가 입력 줄에 표시된다
```

---

### AC-SCR-013: Ctrl+R 역방향 히스토리 검색

**Scenario 1: 검색어 입력**

```gherkin
Given REPL 세션이 활성화되어 있다
And 히스토리에 "flow list", "flow deploy my-flow", "agent list" 명령어가 있다
When 사용자가 Ctrl+R을 누르고 "deploy"를 입력한다
Then "flow deploy my-flow" 명령어가 검색 결과로 표시된다
```

---

### AC-SCR-014: 히스토리 관리 명령어 확장

**Scenario 1: history 명령어 (기존 동작 유지)**

```gherkin
Given REPL 세션이 활성화되어 있다
And 히스토리에 3개의 명령어가 있다
When 사용자가 "history"를 입력한다
Then 번호와 함께 히스토리 목록이 출력된다:
  """
    1  flow list
    2  agent list
    3  status
  """
```

**Scenario 2: history clear**

```gherkin
Given REPL 세션이 활성화되어 있다
And 히스토리에 여러 명령어가 있다
When 사용자가 "history clear"를 입력한다
Then 인메모리 히스토리가 초기화된다
And 히스토리 파일 내용이 삭제된다
And "히스토리가 삭제되었습니다." 메시지가 출력된다
```

**Scenario 3: history search**

```gherkin
Given REPL 세션이 활성화되어 있다
And 히스토리에 "flow list", "flow deploy my-flow", "agent list" 명령어가 있다
When 사용자가 "history search flow"를 입력한다
Then "flow"를 포함하는 히스토리 항목만 번호와 함께 출력된다:
  """
    1  flow list
    2  flow deploy my-flow
  """
```

**Scenario 4: history search 결과 없음**

```gherkin
Given REPL 세션이 활성화되어 있다
When 사용자가 "history search nonexistent"를 입력한다
Then "일치하는 히스토리가 없습니다." 메시지가 출력된다
```

---

### AC-SCR-015: AutoCompleter와 readline 연동

**Scenario 1: Tab 자동완성 동작**

```gherkin
Given REPL 세션이 readline 기반으로 실행 중이다
When 사용자가 "flo"를 입력하고 Tab 키를 누른다
Then readline이 AutoCompleter를 호출한다
And "flow" 명령어가 자동완성된다
```

**Scenario 2: 서브커맨드 자동완성**

```gherkin
Given REPL 세션이 readline 기반으로 실행 중이다
When 사용자가 "flow "를 입력하고 Tab 키를 누른다
Then flow의 서브커맨드 목록 (list, get, create, ...) 이 후보로 표시된다
```

---

## 3. P2 - Advanced Script Features 인수 기준

### AC-SCR-016: 환경 변수 치환

**Scenario 1: $VAR_NAME 치환**

```gherkin
Given 환경 변수 FLOW_NAME="my-sensor-flow"가 설정되어 있다
And 스크립트 파일에 "flow deploy $FLOW_NAME" 명령어가 있다
When 시스템이 해당 스크립트를 실행한다
Then "flow deploy my-sensor-flow"가 실행된다
```

**Scenario 2: ${VAR} 치환**

```gherkin
Given 환경 변수 TARGET="production"이 설정되어 있다
And 스크립트 파일에 "flow deploy ${TARGET}-flow" 명령어가 있다
When 시스템이 해당 스크립트를 실행한다
Then "flow deploy production-flow"가 실행된다
```

**Scenario 3: 미정의 변수**

```gherkin
Given 환경 변수 UNDEFINED_VAR가 설정되지 않았다
And 스크립트 파일에 "flow deploy $UNDEFINED_VAR" 명령어가 있다
When 시스템이 해당 스크립트를 실행한다
Then "flow deploy "가 실행된다 (빈 문자열로 치환)
```

---

### AC-SCR-017: Dry-run 모드

**Scenario 1: 실행 없이 목록 출력**

```gherkin
Given 3개의 명령어가 포함된 스크립트 파일이 존재한다
When 사용자가 "xflow script -f commands.xflow --dry-run"을 실행한다
Then 각 명령어가 줄 번호와 함께 출력된다:
  """
  [1] status
  [3] flow list --format json
  [6] flow deploy my-flow
  """
And 실제 명령어는 실행되지 않는다
```

---

### AC-SCR-018: Verbose 모드

**Scenario 1: 실행 전 진행 정보 출력**

```gherkin
Given 2개의 명령어가 포함된 스크립트 파일이 존재한다
When 사용자가 "xflow script -f commands.xflow --verbose"를 실행한다
Then 각 명령어 실행 전에 진행 정보가 출력된다:
  """
  [1] 실행: status
  (status 실행 결과)
  [3] 실행: flow list
  (flow list 실행 결과)
  """
```

---

## 4. 품질 게이트

### 4.1 테스트 기준

| 항목 | 기준 |
|------|------|
| 단위 테스트 커버리지 (전체) | 85% 이상 |
| script.go 커버리지 | 90% 이상 |
| readline_adapter.go 커버리지 | 85% 이상 |
| interactive.go 커버리지 | 기존 91.7% 유지 또는 향상 |
| Race detector | `go test -race` 통과 |
| 기존 테스트 회귀 | 70+ 서브테스트 전체 통과 |

### 4.2 코드 품질 기준

| 항목 | 기준 |
|------|------|
| golangci-lint | 경고 0건 |
| go vet | 경고 0건 |
| 하위 호환성 | 기존 readlineInterface 인터페이스 변경 없음 |
| 크로스 플랫폼 | Linux, macOS 테스트 통과 |

### 4.3 Definition of Done

- [ ] 모든 P0 요구사항 (REQ-SCR-001 ~ REQ-SCR-009) 구현 완료
- [ ] 모든 P1 요구사항 (REQ-SCR-010 ~ REQ-SCR-015) 구현 완료
- [ ] 단위 테스트 커버리지 85% 이상 달성
- [ ] `go test -race ./internal/cli/...` 통과
- [ ] 기존 70+ 서브테스트 전체 통과 (회귀 없음)
- [ ] golangci-lint 경고 0건
- [ ] 코드 리뷰 완료

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-19*
*작성: MoAI SPEC Builder*
