---
id: SPEC-CLI-001
version: "1.1.0"
status: draft
created: "2026-02-13"
updated: "2026-03-10"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-03-10 | 1.1.0 | Module 11-13 추가: CLI 출력 가독성 개선 (Flow Detail Formatter, Status Detail Formatter, TextFormatter Enhancement) |

---

# SPEC-CLI-001: CLI Tool (xflow) - 원격 API 클라이언트, 명령어 체계, 출력 포맷터, 설정 관리

## 1. Environment (환경)

### 1.1 시스템 개요

xflow CLI 도구는 원격 xflowd 데몬 서버에 REST API를 통해 접속하여 플랫폼의 모든 관리 작업을 수행하는 커맨드라인 클라이언트이다. 웹 대시보드와 동일한 API 엔드포인트(SPEC-API-001)를 사용하며, DevOps 자동화와 스크립팅에 최적화된 인터페이스를 제공한다.

본 SPEC은 다음을 포함한다:

- **Root Command & Global Flags** (`root.go`): 루트 명령어, 글로벌 플래그, 설정 파일 로딩, 서버 URL/토큰 해석 우선순위
- **API Client** (`client.go`): HTTP 클라이언트 추상화, 인증 헤더 주입, 에러 응답 파싱, 타임아웃/재시도
- **Flow Commands** (`flow.go`): 플로우 CRUD, 실행 제어(deploy/start/stop/restart), 내보내기/가져오기
- **Agent Commands** (`agent.go`): Agent CRUD, 생명주기 제어(start/stop/restart)
- **Node Commands** (`node.go`): 노드 타입 목록 조회, 노드 상세 정보
- **Plugin Commands** (`plugin.go`): 플러그인 목록/설치/제거/업데이트
- **Config Commands** (`config.go`): CLI 설정 초기화, 서버 URL 설정, 토큰 설정
- **Status & Monitoring** (`status.go`): 서버 상태 조회, 로그 스트리밍, 메트릭 요약
- **Output Formatter** (`output.go`): 테이블/JSON/YAML/텍스트 출력 포맷, 컬러 지원, 진행 표시기
- **Error Types** (`errors.go`): CLI 전용 에러 타입, 사용자 친화적 에러 메시지

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/cli/` (CLI 명령어 정의), `cmd/xflow/` (바이너리 엔트리포인트)
- **Tier**: internal (비공개 패키지)
- **CLI 프레임워크**: `github.com/spf13/cobra` v1.8+
- **설정 관리**: `github.com/spf13/viper` v1.18+
- **HTTP 클라이언트**: Go 표준 `net/http`
- **JSON 처리**: Go 표준 `encoding/json`
- **YAML 처리**: `gopkg.in/yaml.v3`
- **테이블 출력**: `github.com/olekukonko/tablewriter` 또는 자체 구현
- **컬러 출력**: `github.com/fatih/color`
- **의존 SPEC**:
  - SPEC-API-001: CLI가 소비하는 REST API 엔드포인트
  - SPEC-AUTH-001: JWT 토큰 및 API 키를 통한 CLI 인증
  - SPEC-CFG-001: 설정 시스템 (서버 측)
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **순수 API 클라이언트**: CLI는 로컬 데이터 처리 로직을 가지지 않으며, 모든 데이터 조작은 원격 xflowd 서버를 통해 수행
- **스크립팅 친화적**: JSON 출력 모드를 통해 다른 도구와의 파이프라인 조합 지원 (`xflow flow list --format json | jq ...`)
- **실패 시 명확한 안내**: 에러 발생 시 원인과 해결 방법을 포함한 메시지 제공
- **최소 설정 원칙**: 기본값만으로 즉시 사용 가능, 필요 시 설정 파일로 커스터마이징
- **크로스 플랫폼**: Linux, macOS, Windows에서 동일하게 동작

### 1.4 스코프 경계

**포함 (In Scope)**:
- CLI 명령어 정의 및 인자 파싱
- REST API 호출 및 응답 처리
- 출력 포맷팅 (table, JSON, YAML, plain text)
- 설정 파일 관리 (`~/.xflow/config.yaml`)
- 에러 메시지 및 사용자 안내

**제외 (Out of Scope)**:
- REST API 서버 구현 (SPEC-API-001)
- 인증/인가 로직 (SPEC-AUTH-001)
- Flow Engine 런타임 (SPEC-ENGINE-001)
- 웹 대시보드 (별도 SPEC)
- 로컬 플로우 실행 (모든 실행은 원격)

---

## 2. Assumptions (가정 사항)

### 2.1 기술 가정

- **AS-CLI-001**: xflowd 서버가 SPEC-API-001에 정의된 REST API를 제공하고 있다
- **AS-CLI-002**: 서버 응답은 표준 API 응답 엔벨로프(`{success, data, error, meta}`)를 따른다
- **AS-CLI-003**: 인증은 Bearer 토큰(JWT) 또는 API 키 헤더를 통해 수행된다
- **AS-CLI-004**: CLI 설정 파일은 `~/.xflow/config.yaml`에 위치한다
- **AS-CLI-005**: Go 1.23+ 환경에서 `net/http` 표준 클라이언트가 충분한 성능을 제공한다

### 2.2 운영 가정

- **AS-CLI-006**: 사용자는 네트워크를 통해 xflowd 서버에 접근 가능하다
- **AS-CLI-007**: CLI는 단일 사용자가 대화형(interactive) 또는 스크립트 모드로 사용한다
- **AS-CLI-008**: CLI 바이너리는 GoReleaser를 통해 Linux/macOS/Windows 크로스 컴파일된다

---

## 3. Requirements (요구사항)

### 3.1 Module 1: Root Command & Global Flags (P0) - root.go

#### REQ-CLI-001-01-01: 루트 명령어 설정
시스템은 **항상** `xflow` 루트 명령어를 제공해야 하며, 프로그램 이름, 버전, 설명을 포함해야 한다.

#### REQ-CLI-001-01-02: 글로벌 플래그
시스템은 **항상** 다음 글로벌 플래그를 제공해야 한다:
- `--config <path>`: 설정 파일 경로 (기본: `~/.xflow/config.yaml`)
- `--server <url>`: xflowd 서버 URL
- `--format <json|yaml|table|text>`: 출력 형식 (기본: `table`)
- `--token <token>`: 인증 토큰
- `--verbose`: 상세 출력 모드
- `--quiet`: 최소 출력 모드
- `--no-color`: 컬러 비활성화

#### REQ-CLI-001-01-03: 서버 URL 해석 우선순위
**WHEN** CLI가 서버 URL을 결정할 때, **THEN** 다음 우선순위를 따라야 한다:
1. `--server` 플래그 (최우선)
2. `XFLOW_SERVER` 환경 변수
3. 설정 파일의 `server.url` 값
4. 기본값 `http://localhost:8080`

#### REQ-CLI-001-01-04: 토큰 해석 우선순위
**WHEN** CLI가 인증 토큰을 결정할 때, **THEN** 다음 우선순위를 따라야 한다:
1. `--token` 플래그 (최우선)
2. `XFLOW_TOKEN` 환경 변수
3. 설정 파일의 `auth.token` 값

#### REQ-CLI-001-01-05: 버전 명령어
**WHEN** `xflow version`을 실행할 때, **THEN** 빌드 버전, 커밋 해시, 빌드 시간, Go 버전을 출력해야 한다.

#### REQ-CLI-001-01-06: 설정 파일 로딩
**WHEN** CLI가 시작될 때, **THEN** `--config` 플래그 또는 기본 경로의 설정 파일을 Viper로 로딩해야 한다.
**IF** 설정 파일이 존재하지 않는 경우, **THEN** 에러를 발생시키지 않고 기본값으로 동작해야 한다.

---

### 3.2 Module 2: API Client (P0) - client.go

#### REQ-CLI-001-02-01: HTTP 클라이언트 구조체
시스템은 **항상** 다음 필드를 포함하는 `Client` 구조체를 제공해야 한다:
- `baseURL`: xflowd 서버 기본 URL
- `token`: 인증 토큰 (Bearer 또는 API Key)
- `httpClient`: `*http.Client` (타임아웃 설정 포함)
- `format`: 출력 형식

#### REQ-CLI-001-02-02: 범용 요청 메서드
시스템은 **항상** GET, POST, PUT, DELETE HTTP 메서드를 지원하는 범용 요청 함수를 제공해야 한다.

#### REQ-CLI-001-02-03: 인증 헤더 주입
**WHEN** API 요청을 전송할 때, **THEN** 토큰이 설정되어 있으면 `Authorization: Bearer <token>` 헤더를 자동으로 주입해야 한다.

#### REQ-CLI-001-02-04: 에러 응답 파싱
**WHEN** 서버가 4xx 또는 5xx 응답을 반환할 때, **THEN** 표준 API 에러 엔벨로프를 파싱하여 사용자 친화적 에러 메시지를 생성해야 한다.

#### REQ-CLI-001-02-05: 타임아웃 설정
시스템은 **항상** HTTP 요청에 기본 30초 타임아웃을 적용해야 한다. 설정 파일에서 변경 가능해야 한다.

#### REQ-CLI-001-02-06: 연결 상태 확인
**WHEN** `ping` 또는 `health check` 요청을 할 때, **THEN** xflowd 서버의 `/health` 엔드포인트에 GET 요청을 보내고 응답 상태를 반환해야 한다.

#### REQ-CLI-001-02-07: JSON 직렬화/역직렬화
시스템은 **항상** 요청 본문을 JSON으로 직렬화하고, 응답 본문을 JSON에서 역직렬화해야 한다. Content-Type 헤더를 `application/json`으로 설정해야 한다.

---

### 3.3 Module 3: Flow Commands (P0) - flow.go

#### REQ-CLI-001-03-01: 플로우 목록 조회
**WHEN** `xflow flow list`를 실행할 때, **THEN** 서버의 모든 플로우 목록을 조회하여 출력해야 한다.
테이블 출력에는 ID, 이름, 상태, 노드 수, 생성일이 포함되어야 한다.

#### REQ-CLI-001-03-02: 플로우 상세 조회
**WHEN** `xflow flow get <id>`를 실행할 때, **THEN** 지정된 플로우의 상세 정보를 출력해야 한다.
상세 정보에는 ID, 이름, 설명, 상태, 노드 목록, 연결 정보, 생성일, 수정일이 포함되어야 한다.

#### REQ-CLI-001-03-03: 플로우 생성
**WHEN** `xflow flow create -f <file>`를 실행할 때, **THEN** 지정된 JSON/YAML 파일을 읽어 서버에 플로우를 생성해야 한다.
**IF** 파일이 존재하지 않는 경우, **THEN** `파일을 찾을 수 없습니다: <path>` 에러를 출력해야 한다.

#### REQ-CLI-001-03-04: 플로우 수정
**WHEN** `xflow flow update <id> -f <file>`를 실행할 때, **THEN** 지정된 파일로 기존 플로우 정의를 업데이트해야 한다.

#### REQ-CLI-001-03-05: 플로우 삭제
**WHEN** `xflow flow delete <id>`를 실행할 때, **THEN** 확인 프롬프트를 표시한 후 플로우를 삭제해야 한다.
`--yes` 또는 `-y` 플래그가 있으면 확인 프롬프트를 건너뛰어야 한다.
시스템은 **실행 중인 플로우를 확인 없이 삭제하지 않아야** 한다.

#### REQ-CLI-001-03-06: 플로우 배포
**WHEN** `xflow flow deploy <id>`를 실행할 때, **THEN** 지정된 플로우를 배포 상태로 전환해야 한다.

#### REQ-CLI-001-03-07: 플로우 시작
**WHEN** `xflow flow start <id>`를 실행할 때, **THEN** 지정된 플로우의 실행을 시작해야 한다.

#### REQ-CLI-001-03-08: 플로우 중지
**WHEN** `xflow flow stop <id>`를 실행할 때, **THEN** 지정된 플로우의 실행을 중지해야 한다.

#### REQ-CLI-001-03-09: 플로우 재시작
**WHEN** `xflow flow restart <id>`를 실행할 때, **THEN** 지정된 플로우를 중지한 후 다시 시작해야 한다.

#### REQ-CLI-001-03-10: 플로우 내보내기
**WHEN** `xflow flow export <id> -o <file>`를 실행할 때, **THEN** 플로우 정의를 지정된 파일로 내보내야 한다.
파일 확장자에 따라 JSON(`.json`) 또는 YAML(`.yaml`, `.yml`) 형식을 자동 선택해야 한다.

#### REQ-CLI-001-03-11: 플로우 가져오기
**WHEN** `xflow flow import -f <file>`를 실행할 때, **THEN** 파일에서 플로우 정의를 읽어 서버에 새 플로우로 생성해야 한다.

#### REQ-CLI-001-03-12: 플로우 상태 조회
**WHEN** `xflow flow status <id>`를 실행할 때, **THEN** 플로우의 런타임 상태 (실행 상태, 처리 메시지 수, 에러 수, 가동 시간)를 출력해야 한다.

---

### 3.4 Module 4: Agent Commands (P0) - agent.go

#### REQ-CLI-001-04-01: Agent 목록 조회
**WHEN** `xflow agent list`를 실행할 때, **THEN** 서버에 등록된 모든 Agent 목록을 출력해야 한다.
테이블 출력에는 ID, 이름, 타입, 상태, 연결 상태가 포함되어야 한다.

#### REQ-CLI-001-04-02: Agent 상세 조회
**WHEN** `xflow agent get <id>`를 실행할 때, **THEN** Agent의 상세 정보(설정, 통계, 연결 정보)를 출력해야 한다.

#### REQ-CLI-001-04-03: Agent 생성
**WHEN** `xflow agent create -f <file>`를 실행할 때, **THEN** 설정 파일로부터 새 Agent를 생성해야 한다.

#### REQ-CLI-001-04-04: Agent 시작
**WHEN** `xflow agent start <id>`를 실행할 때, **THEN** 지정된 Agent를 시작해야 한다.

#### REQ-CLI-001-04-05: Agent 중지
**WHEN** `xflow agent stop <id>`를 실행할 때, **THEN** 지정된 Agent를 중지해야 한다.

#### REQ-CLI-001-04-06: Agent 재시작
**WHEN** `xflow agent restart <id>`를 실행할 때, **THEN** 지정된 Agent를 재시작해야 한다.

#### REQ-CLI-001-04-07: Agent 삭제
**WHEN** `xflow agent delete <id>`를 실행할 때, **THEN** 확인 프롬프트 후 Agent를 삭제해야 한다.
시스템은 **플로우에서 참조 중인 Agent를 경고 없이 삭제하지 않아야** 한다.

---

### 3.5 Module 5: Node Commands (P1) - node.go

#### REQ-CLI-001-05-01: 노드 타입 목록 조회
**WHEN** `xflow node list`를 실행할 때, **THEN** 서버에 등록된 모든 노드 타입 목록을 출력해야 한다.
테이블 출력에는 타입명, 카테고리, 설명, 소스(내장/플러그인)가 포함되어야 한다.

#### REQ-CLI-001-05-02: 노드 타입 상세 정보
**WHEN** `xflow node info <type>`를 실행할 때, **THEN** 노드 타입의 상세 정보(입/출력 포트, 설정 스키마, 설명)를 출력해야 한다.

---

### 3.6 Module 6: Plugin Commands (P1) - plugin.go

#### REQ-CLI-001-06-01: 플러그인 목록 조회
**WHEN** `xflow plugin list`를 실행할 때, **THEN** 서버에 설치된 플러그인 목록을 출력해야 한다.
테이블 출력에는 이름, 버전, 타입(Go/WASM), 상태, 제공 노드 수가 포함되어야 한다.

#### REQ-CLI-001-06-02: 플러그인 설치
**WHEN** `xflow plugin install <name|path>`를 실행할 때, **THEN** 지정된 플러그인을 서버에 설치해야 한다.
설치 진행 상태를 표시해야 한다.

#### REQ-CLI-001-06-03: 플러그인 제거
**WHEN** `xflow plugin remove <name>`를 실행할 때, **THEN** 확인 프롬프트 후 플러그인을 제거해야 한다.
시스템은 **사용 중인 플러그인 노드가 있는 경우 경고를 표시해야** 한다.

#### REQ-CLI-001-06-04: 플러그인 업데이트
**WHEN** `xflow plugin update <name>`를 실행할 때, **THEN** 지정된 플러그인을 최신 버전으로 업데이트해야 한다.

---

### 3.7 Module 7: Config Commands (P0) - config.go

#### REQ-CLI-001-07-01: 설정 초기화
**WHEN** `xflow config init`를 실행할 때, **THEN** `~/.xflow/config.yaml` 파일을 기본값으로 생성해야 한다.
**IF** 파일이 이미 존재하는 경우, **THEN** 덮어쓸지 확인 프롬프트를 표시해야 한다.

#### REQ-CLI-001-07-02: 설정 값 조회
**WHEN** `xflow config get <key>`를 실행할 때, **THEN** 지정된 키의 현재 설정 값을 출력해야 한다.

#### REQ-CLI-001-07-03: 설정 값 설정
**WHEN** `xflow config set <key> <value>`를 실행할 때, **THEN** 설정 파일에 값을 저장해야 한다.

#### REQ-CLI-001-07-04: 서버 URL 단축 명령어
**WHEN** `xflow config server <url>`를 실행할 때, **THEN** `server.url` 설정을 업데이트해야 한다.

#### REQ-CLI-001-07-05: 토큰 단축 명령어
**WHEN** `xflow config token <token>`를 실행할 때, **THEN** `auth.token` 설정을 업데이트해야 한다.

#### REQ-CLI-001-07-06: 설정 전체 조회
**WHEN** `xflow config list`를 실행할 때, **THEN** 현재 활성 설정을 모두 출력해야 한다.
토큰 값은 마스킹(`****...xxxx`)하여 출력해야 한다.

---

### 3.8 Module 8: Status & Monitoring (P1) - status.go

#### REQ-CLI-001-08-01: 서버 상태 조회
**WHEN** `xflow status`를 실행할 때, **THEN** xflowd 서버의 상태를 출력해야 한다.
출력에는 서버 버전, 가동 시간, 실행 중인 플로우 수, 활성 Agent 수, 시스템 리소스 사용량이 포함되어야 한다.

#### REQ-CLI-001-08-02: 로그 스트리밍
**가능하면** `xflow logs <component>` 명령어로 특정 컴포넌트의 로그를 실시간 스트리밍할 수 있어야 한다.
SSE 또는 WebSocket을 통해 서버로부터 로그를 수신해야 한다.

#### REQ-CLI-001-08-03: 메트릭 요약
**가능하면** `xflow metrics` 명령어로 현재 시스템 메트릭 요약을 조회할 수 있어야 한다.

---

### 3.9 Module 9: Output Formatter (P0) - output.go

#### REQ-CLI-001-09-01: 포맷터 인터페이스
시스템은 **항상** `Formatter` 인터페이스를 통해 출력 형식을 추상화해야 한다.
인터페이스는 `Format(data any, writer io.Writer) error` 메서드를 포함해야 한다.

#### REQ-CLI-001-09-02: 테이블 포맷
시스템은 **항상** 기본 테이블 포맷(사람이 읽기 쉬운 정렬된 컬럼)을 제공해야 한다.
컬럼 헤더와 행 데이터를 정렬하여 표시해야 한다.

#### REQ-CLI-001-09-03: JSON 포맷
시스템은 **항상** JSON 포맷(기계 판독 가능, 들여쓰기 적용)을 제공해야 한다.

#### REQ-CLI-001-09-04: YAML 포맷
시스템은 **항상** YAML 포맷(사람이 읽기 쉬운 구조화 데이터)을 제공해야 한다.

#### REQ-CLI-001-09-05: 텍스트 포맷
시스템은 **항상** 일반 텍스트 포맷(최소한의 스크립팅 친화적 출력)을 제공해야 한다.

#### REQ-CLI-001-09-06: --format 플래그 통합
**WHEN** `--format` 글로벌 플래그가 지정될 때, **THEN** 모든 명령어의 출력이 지정된 형식을 사용해야 한다.

#### REQ-CLI-001-09-07: 컬러 출력
시스템은 **항상** 터미널 지원 여부를 자동 감지하여 컬러 출력을 제공해야 한다.
`--no-color` 플래그로 비활성화 가능해야 한다.
**WHILE** stdout이 파이프에 연결된 동안, 시스템은 컬러 출력을 자동 비활성화해야 한다.

#### REQ-CLI-001-09-08: 진행 표시기
**WHILE** 장시간 작업(플러그인 설치, 플로우 배포 등)이 진행되는 동안, 시스템은 진행 상태 표시기(스피너 또는 프로그레스 바)를 표시해야 한다.

---

### 3.10 Module 10: Error Types (P0) - errors.go

#### REQ-CLI-001-10-01: 서버 연결 불가 에러
**IF** xflowd 서버에 연결할 수 없는 경우, **THEN** 서버 URL과 함께 `서버에 연결할 수 없습니다` 에러를 출력하고, `xflow config server <url>` 명령어 안내를 표시해야 한다.

#### REQ-CLI-001-10-02: 인증 실패 에러
**IF** 서버가 401 Unauthorized를 반환하는 경우, **THEN** `인증에 실패했습니다. 토큰을 확인해주세요` 에러를 출력하고, `xflow config token <token>` 명령어 안내를 표시해야 한다.

#### REQ-CLI-001-10-03: 권한 부족 에러
**IF** 서버가 403 Forbidden을 반환하는 경우, **THEN** `이 작업을 수행할 권한이 없습니다` 에러를 출력해야 한다.

#### REQ-CLI-001-10-04: 리소스 미발견 에러
**IF** 서버가 404 Not Found를 반환하는 경우, **THEN** 리소스 타입과 ID를 포함한 `리소스를 찾을 수 없습니다` 에러를 출력해야 한다.

#### REQ-CLI-001-10-05: 잘못된 입력 에러
**IF** 명령어 인자가 잘못되었거나 파일 형식이 올바르지 않은 경우, **THEN** 구체적인 원인과 올바른 사용법 예시를 포함한 에러를 출력해야 한다.

#### REQ-CLI-001-10-06: 파일 미발견 에러
**IF** 지정된 파일이 존재하지 않는 경우, **THEN** 파일 경로를 포함한 `파일을 찾을 수 없습니다` 에러를 출력해야 한다.

#### REQ-CLI-001-10-07: 설정 미초기화 에러
**IF** 설정 파일이 필요하지만 초기화되지 않은 경우, **THEN** `xflow config init` 명령어 안내를 포함한 에러를 출력해야 한다.

---

### 3.11 Module 11: Flow Detail Formatter (P0) - flow.go, output.go

#### REQ-CLI-001-11-01: flow get에 DetailFormatter 적용
**WHEN** `xflow flow get <id>` 명령어를 실행할 때, **THEN** 단일 플로우 객체 출력에 `TextFormatter` 폴백 대신 `DetailFormatter`를 사용해야 한다.
현재 `format == "table"` 일 때 `format = "text"`로 변경하여 `TextFormatter`에 위임하는 방식은 중첩 데이터(nodes, edges)에 대해 Go의 raw `map[...]` 형식을 출력하므로, `DetailFormatter`를 사용하여 구조화된 가독성 높은 출력을 제공해야 한다.

#### REQ-CLI-001-11-02: 플로우 상세 필드 순서
시스템은 **항상** 플로우 상세 출력에서 다음 필드 순서를 따라야 한다:
1. `id` (ID)
2. `name` (Name)
3. `description` (Description)
4. `status` (Status)
5. `node_count` (Nodes) - config.nodes 배열의 길이로 계산
6. `created_at` (Created At)
7. `updated_at` (Updated At)

#### REQ-CLI-001-11-03: 노드 미니 테이블 렌더링
**WHEN** 플로우 상세 출력에서 config의 `nodes` 데이터를 표시할 때, **THEN** 각 노드를 미니 테이블 형식으로 렌더링해야 한다.
미니 테이블 컬럼에는 `label`(라벨), `type`(노드 타입), `direction`(방향), `agent_name`(Agent)이 포함되어야 한다.
노드의 `data` 맵에서 해당 필드를 추출하여 표시해야 한다.
값이 없는 필드는 `-`로 표시해야 한다.

#### REQ-CLI-001-11-04: 엣지 연결 리스트 렌더링
**WHEN** 플로우 상세 출력에서 config의 `edges` 데이터를 표시할 때, **THEN** 각 엣지를 `source_label -> target_label (sourceHandle -> targetHandle)` 형식의 가독성 높은 연결 리스트로 렌더링해야 한다.
엣지의 `source`, `target` 필드에 저장된 노드 ID를 사람이 읽을 수 있는 노드 라벨로 치환해야 한다.

#### REQ-CLI-001-11-05: 상세 표시 플래그
**가능하면** `--detail` 플래그를 제공하여 노드의 세부 데이터(expression, address_table, config 등)를 포함한 확장 출력을 지원해야 한다.
- `--detail summary` (기본값): 노드 미니 테이블과 엣지 연결 리스트만 표시
- `--detail full`: 각 노드의 전체 data 맵을 섹션별로 전개하여 표시

#### REQ-CLI-001-11-06: 노드 ID -> 라벨 해석
시스템은 **항상** 엣지 출력에서 노드 ID를 사람이 읽을 수 있는 라벨로 해석해야 한다.
nodes 배열에서 `id` -> `data.label` 매핑 테이블을 구축하고, edges의 `source`/`target` 필드 값을 해당 라벨로 치환해야 한다.
**IF** 매핑에 해당하는 노드를 찾을 수 없는 경우, **THEN** 원본 ID의 앞 8자를 표시해야 한다 (예: `0740d7e0`).

---

### 3.12 Module 12: Status Detail Formatter (P1) - status.go

#### REQ-CLI-001-12-01: status 명령어에 DetailFormatter 적용
**WHEN** `xflow status` 명령어를 실행할 때, **THEN** `TextFormatter` 폴백 대신 `DetailFormatter`를 사용하여 서버 상태를 구조화된 형식으로 출력해야 한다.
필드 순서, 라벨 매핑, 섹션 분리를 적용하여 가독성을 향상시켜야 한다.

#### REQ-CLI-001-12-02: status metrics에 DetailFormatter 적용
**WHEN** `xflow status metrics` 명령어를 실행할 때, **THEN** `DetailFormatter`를 사용하여 메트릭을 그룹별 섹션으로 구분하여 출력해야 한다.
중첩된 메트릭 데이터(CPU, 메모리, 디스크 등)는 `sectionKeys`를 통해 별도 섹션으로 렌더링해야 한다.

---

### 3.13 Module 13: TextFormatter Enhancement (P1) - output.go

#### REQ-CLI-001-13-01: TextFormatter 중첩 데이터 렌더링 금지
`TextFormatter.formatMap()` 메서드는 중첩된 map 또는 slice 값에 대해 raw `%v` 포맷을 **사용하지 않아야** 한다.
현재 `fmt.Fprintf(writer, "%s: %v\n", key, val.Interface())` 패턴이 중첩 데이터에 Go 내부 표현(`map[...]`)을 노출시키므로, 이를 개선해야 한다.

#### REQ-CLI-001-13-02: 중첩 map 들여쓰기 렌더링
**WHEN** TextFormatter가 값이 map 타입인 필드를 출력할 때, **THEN** 해당 map의 각 키-값 쌍을 2칸 들여쓰기로 재귀적으로 렌더링해야 한다.

#### REQ-CLI-001-13-03: 중첩 slice 리스트 렌더링
**WHEN** TextFormatter가 값이 slice 타입인 필드를 출력할 때, **THEN** 각 요소를 들여쓰기된 리스트로 렌더링해야 한다.
요소가 map인 경우 각 키-값 쌍을 들여쓰기로 표시하고, 요소 간 빈 줄로 구분해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
cmd/xflow/
  main.go                    # 바이너리 엔트리포인트

internal/cli/
  root.go                    # 루트 명령어, 글로벌 플래그, 설정 로딩
  client.go                  # HTTP API 클라이언트
  flow.go                    # 플로우 명령어 (list|get|create|update|delete|deploy|start|stop|restart|export|import|status)
  agent.go                   # Agent 명령어 (list|get|create|start|stop|restart|delete)
  node.go                    # 노드 명령어 (list|info)
  plugin.go                  # 플러그인 명령어 (list|install|remove|update)
  config.go                  # 설정 명령어 (init|get|set|server|token|list)
  status.go                  # 상태/모니터링 명령어 (status|logs|metrics)
  output.go                  # 출력 포맷터 (table|json|yaml|text)
  errors.go                  # CLI 에러 타입
  cli_test.go                # CLI 테스트
```

### 4.2 명령어 트리

```
xflow
  ├── version                              # 버전 정보
  ├── flow
  │   ├── list                             # 플로우 목록
  │   ├── get <id>                         # 플로우 상세
  │   ├── create -f <file>                 # 플로우 생성
  │   ├── update <id> -f <file>            # 플로우 수정
  │   ├── delete <id> [--yes]              # 플로우 삭제
  │   ├── deploy <id>                      # 플로우 배포
  │   ├── start <id>                       # 플로우 시작
  │   ├── stop <id>                        # 플로우 중지
  │   ├── restart <id>                     # 플로우 재시작
  │   ├── export <id> -o <file>            # 플로우 내보내기
  │   ├── import -f <file>                 # 플로우 가져오기
  │   └── status <id>                      # 플로우 런타임 상태
  ├── agent
  │   ├── list                             # Agent 목록
  │   ├── get <id>                         # Agent 상세
  │   ├── create -f <file>                 # Agent 생성
  │   ├── start <id>                       # Agent 시작
  │   ├── stop <id>                        # Agent 중지
  │   ├── restart <id>                     # Agent 재시작
  │   └── delete <id> [--yes]              # Agent 삭제
  ├── node
  │   ├── list                             # 노드 타입 목록
  │   └── info <type>                      # 노드 타입 상세
  ├── plugin
  │   ├── list                             # 플러그인 목록
  │   ├── install <name|path>              # 플러그인 설치
  │   ├── remove <name> [--yes]            # 플러그인 제거
  │   └── update <name>                    # 플러그인 업데이트
  ├── config
  │   ├── init                             # 설정 초기화
  │   ├── get <key>                        # 설정 조회
  │   ├── set <key> <value>                # 설정 변경
  │   ├── server <url>                     # 서버 URL 설정
  │   ├── token <token>                    # 토큰 설정
  │   └── list                             # 전체 설정 조회
  ├── status                               # 서버 상태
  ├── logs <component>                     # 로그 스트리밍 (P1)
  └── metrics                              # 메트릭 요약 (P1)
```

### 4.3 핵심 인터페이스

```go
// Formatter - 출력 포맷 인터페이스
type Formatter interface {
    Format(data any, writer io.Writer) error
}

// Client - API 클라이언트 인터페이스
type Client interface {
    Get(path string, result any) error
    Post(path string, body any, result any) error
    Put(path string, body any, result any) error
    Delete(path string, result any) error
    Ping() error
}
```

### 4.4 설정 파일 구조 (`~/.xflow/config.yaml`)

```yaml
server:
  url: "http://localhost:8080"
  timeout: 30s

auth:
  token: ""

output:
  format: "table"
  color: true
```

### 4.5 API 엔드포인트 매핑

| CLI 명령어 | HTTP 메서드 | API 엔드포인트 |
|-----------|------------|---------------|
| flow list | GET | /api/v1/flows |
| flow get `<id>` | GET | /api/v1/flows/:id |
| flow create | POST | /api/v1/flows |
| flow update `<id>` | PUT | /api/v1/flows/:id |
| flow delete `<id>` | DELETE | /api/v1/flows/:id |
| flow deploy `<id>` | POST | /api/v1/flows/:id/deploy |
| flow start `<id>` | POST | /api/v1/flows/:id/start |
| flow stop `<id>` | POST | /api/v1/flows/:id/stop |
| flow restart `<id>` | POST | /api/v1/flows/:id/restart |
| flow status `<id>` | GET | /api/v1/flows/:id/status |
| agent list | GET | /api/v1/agents |
| agent get `<id>` | GET | /api/v1/agents/:id |
| agent create | POST | /api/v1/agents |
| agent start `<id>` | POST | /api/v1/agents/:id/start |
| agent stop `<id>` | POST | /api/v1/agents/:id/stop |
| agent restart `<id>` | POST | /api/v1/agents/:id/restart |
| agent delete `<id>` | DELETE | /api/v1/agents/:id |
| node list | GET | /api/v1/nodes |
| node info `<type>` | GET | /api/v1/nodes/:type |
| plugin list | GET | /api/v1/plugins |
| plugin install | POST | /api/v1/plugins |
| plugin remove `<name>` | DELETE | /api/v1/plugins/:name |
| plugin update `<name>` | PUT | /api/v1/plugins/:name |
| status | GET | /api/v1/status |

---

## 5. 구현 우선순위

| 우선순위 | 모듈 | 근거 |
|---------|------|------|
| P0 (핵심) | Root Command & Global Flags | 모든 명령어의 기반 |
| P0 (핵심) | API Client | 모든 원격 통신의 기반 |
| P0 (핵심) | Output Formatter | 모든 출력의 기반 |
| P0 (핵심) | Error Types | 모든 에러 처리의 기반 |
| P0 (핵심) | Flow Commands | 핵심 사용자 기능 |
| P0 (핵심) | Agent Commands | 핵심 사용자 기능 |
| P0 (핵심) | Config Commands | 초기 설정에 필수 |
| P0 (핵심) | **Flow Detail Formatter** | **flow get 출력이 읽을 수 없는 raw map 형식 - 사용자 경험 심각 저하** |
| P1 (중요) | Node Commands | 노드 정보 조회 |
| P1 (중요) | Plugin Commands | 플러그인 관리 |
| P1 (중요) | Status & Monitoring | 운영 모니터링 |
| P1 (중요) | **Status Detail Formatter** | **status 명령어도 동일한 raw 출력 문제** |
| P1 (중요) | **TextFormatter Enhancement** | **TextFormatter 자체의 중첩 데이터 렌더링 근본 개선** |

---

## 6. 추적성 (Traceability)

| 요구사항 ID | 관련 SPEC | 설명 |
|------------|----------|------|
| REQ-CLI-001-02-* | SPEC-API-001 | API 클라이언트가 소비하는 REST 엔드포인트 |
| REQ-CLI-001-02-03 | SPEC-AUTH-001 | Bearer 토큰 인증 헤더 주입 |
| REQ-CLI-001-07-* | SPEC-CFG-001 | CLI 설정과 서버 설정 간의 관계 |
| REQ-CLI-001-03-* | SPEC-ENGINE-001, SPEC-FLOW-001 | 플로우 실행 제어 및 정의 |
| REQ-CLI-001-04-* | SPEC-AGENT-001 | Agent 생명주기 관리 |
| REQ-CLI-001-08-* | SPEC-OBS-001 | 관찰성 데이터 조회 |
| REQ-CLI-001-11-* | SPEC-CLI-001 (내부) | flow get 출력 가독성 개선 - DetailFormatter 적용 |
| REQ-CLI-001-12-* | SPEC-CLI-001 (내부) | status 출력 가독성 개선 - DetailFormatter 적용 |
| REQ-CLI-001-13-* | SPEC-CLI-001 (내부) | TextFormatter 중첩 데이터 렌더링 근본 개선 |

---

*문서 버전: 1.1.0*
*최종 수정: 2026-03-10*
*작성: MoAI SPEC Builder*
