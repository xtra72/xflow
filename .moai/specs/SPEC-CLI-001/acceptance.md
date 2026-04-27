---
id: SPEC-CLI-001
type: acceptance
version: "1.1.0"
status: draft
created: "2026-02-13"
updated: "2026-03-10"
author: xtra
---

# SPEC-CLI-001: CLI Tool (xflow) - 인수 테스트 기준

## 1. Root Command & Global Flags (P0)

### AC-CLI-001-01: 루트 명령어 및 글로벌 플래그
```gherkin
기능: 루트 명령어 및 글로벌 설정

  시나리오: 루트 명령어 실행 시 도움말 출력
    주어진 xflow 바이너리가 설치되어 있을 때
    만약 `xflow` 명령어를 인자 없이 실행하면
    그러면 사용법(Usage)과 사용 가능한 서브커맨드 목록이 출력되어야 한다
    그리고 프로그램 이름, 버전, 설명이 포함되어야 한다

  시나리오: --help 플래그로 도움말 출력
    주어진 xflow 바이너리가 설치되어 있을 때
    만약 `xflow --help` 명령어를 실행하면
    그러면 글로벌 플래그 목록(--config, --server, --format, --token, --verbose, --quiet, --no-color)이 출력되어야 한다

  시나리오: --format 글로벌 플래그 적용
    주어진 xflowd 서버가 정상 동작 중이고
    그리고 플로우가 1개 이상 등록되어 있을 때
    만약 `xflow flow list --format json` 명령어를 실행하면
    그러면 출력이 유효한 JSON 형식이어야 한다

  시나리오: --no-color 플래그 적용
    주어진 터미널이 컬러를 지원하고
    만약 `xflow status --no-color` 명령어를 실행하면
    그러면 출력에 ANSI 이스케이프 시퀀스가 포함되지 않아야 한다

  시나리오: --verbose 플래그 적용
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow flow list --verbose` 명령어를 실행하면
    그러면 HTTP 요청 URL, 메서드, 응답 상태 코드가 stderr에 출력되어야 한다

  시나리오: --quiet 플래그 적용
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow flow list --quiet` 명령어를 실행하면
    그러면 최소한의 데이터만 출력되어야 한다
    그리고 헤더나 장식 문자가 포함되지 않아야 한다
```

### AC-CLI-001-02: 서버 URL 해석 우선순위
```gherkin
기능: 서버 URL 결정 우선순위

  시나리오: --server 플래그 최우선
    주어진 설정 파일에 server.url이 "http://config:8080"이고
    그리고 XFLOW_SERVER 환경 변수가 "http://env:8080"일 때
    만약 `xflow status --server http://flag:8080` 명령어를 실행하면
    그러면 "http://flag:8080"으로 요청이 전송되어야 한다

  시나리오: 환경 변수 2순위
    주어진 설정 파일에 server.url이 "http://config:8080"이고
    그리고 XFLOW_SERVER 환경 변수가 "http://env:8080"이고
    그리고 --server 플래그가 없을 때
    만약 `xflow status` 명령어를 실행하면
    그러면 "http://env:8080"으로 요청이 전송되어야 한다

  시나리오: 설정 파일 3순위
    주어진 설정 파일에 server.url이 "http://config:8080"이고
    그리고 XFLOW_SERVER 환경 변수가 없고
    그리고 --server 플래그가 없을 때
    만약 `xflow status` 명령어를 실행하면
    그러면 "http://config:8080"으로 요청이 전송되어야 한다

  시나리오: 기본값 최후순위
    주어진 설정 파일이 없고
    그리고 XFLOW_SERVER 환경 변수가 없고
    그리고 --server 플래그가 없을 때
    만약 `xflow status` 명령어를 실행하면
    그러면 "http://localhost:8080"으로 요청이 전송되어야 한다
```

### AC-CLI-001-03: 토큰 해석 우선순위
```gherkin
기능: 인증 토큰 결정 우선순위

  시나리오: --token 플래그 최우선
    주어진 설정 파일에 auth.token이 "config-token"이고
    그리고 XFLOW_TOKEN 환경 변수가 "env-token"일 때
    만약 `xflow flow list --token flag-token` 명령어를 실행하면
    그러면 Authorization 헤더에 "Bearer flag-token"이 포함되어야 한다

  시나리오: 토큰 미설정 시 인증 헤더 생략
    주어진 설정 파일에 auth.token이 없고
    그리고 XFLOW_TOKEN 환경 변수가 없고
    그리고 --token 플래그가 없을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 Authorization 헤더가 포함되지 않아야 한다
```

### AC-CLI-001-04: 버전 명령어
```gherkin
기능: 버전 정보 출력

  시나리오: 버전 정보 표시
    주어진 xflow 바이너리가 빌드 정보와 함께 컴파일되었을 때
    만약 `xflow version` 명령어를 실행하면
    그러면 버전 번호가 출력되어야 한다
    그리고 커밋 해시가 출력되어야 한다
    그리고 빌드 시간이 출력되어야 한다
    그리고 Go 버전이 출력되어야 한다
```

### AC-CLI-001-05: 설정 파일 로딩
```gherkin
기능: 설정 파일 로딩 동작

  시나리오: 기본 경로 설정 파일 로딩
    주어진 ~/.xflow/config.yaml 파일이 존재하고 유효할 때
    만약 CLI가 시작되면
    그러면 설정 파일의 값이 Viper에 로딩되어야 한다

  시나리오: --config 플래그로 커스텀 경로 지정
    주어진 /tmp/custom-config.yaml 파일이 존재할 때
    만약 `xflow flow list --config /tmp/custom-config.yaml` 명령어를 실행하면
    그러면 /tmp/custom-config.yaml의 설정이 로딩되어야 한다

  시나리오: 설정 파일 미존재 시 기본값 사용
    주어진 ~/.xflow/config.yaml 파일이 존재하지 않을 때
    만약 CLI가 시작되면
    그러면 에러가 발생하지 않아야 한다
    그리고 기본값(server: localhost:8080, format: table)으로 동작해야 한다
```

---

## 2. API Client (P0)

### AC-CLI-001-06: HTTP 클라이언트 기본 동작
```gherkin
기능: API 클라이언트 HTTP 통신

  시나리오: GET 요청 성공
    주어진 xflowd 서버가 정상 동작 중이고
    그리고 유효한 인증 토큰이 설정되어 있을 때
    만약 GET /api/v1/flows 요청을 전송하면
    그러면 200 OK 응답을 수신해야 한다
    그리고 응답 본문이 JSON으로 파싱되어야 한다

  시나리오: POST 요청 시 Content-Type 설정
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 POST /api/v1/flows 요청을 JSON 본문으로 전송하면
    그러면 Content-Type 헤더가 "application/json"이어야 한다

  시나리오: 인증 헤더 자동 주입
    주어진 토큰이 "my-jwt-token"으로 설정되어 있을 때
    만약 API 요청을 전송하면
    그러면 Authorization 헤더가 "Bearer my-jwt-token"이어야 한다

  시나리오: 타임아웃 적용
    주어진 기본 타임아웃이 30초로 설정되어 있을 때
    만약 서버가 35초 동안 응답하지 않으면
    그러면 타임아웃 에러가 발생해야 한다
    그리고 사용자에게 타임아웃 메시지가 표시되어야 한다
```

### AC-CLI-001-07: 서버 연결 확인
```gherkin
기능: 서버 연결 상태 확인

  시나리오: Ping 성공
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 Ping() 함수를 호출하면
    그러면 에러 없이 성공을 반환해야 한다

  시나리오: Ping 실패 - 서버 미가동
    주어진 xflowd 서버가 중지된 상태일 때
    만약 Ping() 함수를 호출하면
    그러면 연결 에러를 반환해야 한다
```

---

## 3. Flow Commands (P0)

### AC-CLI-001-08: 플로우 목록 및 조회
```gherkin
기능: 플로우 목록 조회 및 상세 조회

  시나리오: 플로우 목록 테이블 출력
    주어진 xflowd 서버에 3개의 플로우가 등록되어 있을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 3개의 플로우가 테이블 형식으로 출력되어야 한다
    그리고 각 행에 ID, 이름, 상태, 노드 수, 생성일이 포함되어야 한다

  시나리오: 플로우 목록 JSON 출력
    주어진 xflowd 서버에 플로우가 등록되어 있을 때
    만약 `xflow flow list --format json` 명령어를 실행하면
    그러면 유효한 JSON 배열이 출력되어야 한다

  시나리오: 플로우 상세 조회
    주어진 ID가 "flow-001"인 플로우가 있을 때
    만약 `xflow flow get flow-001` 명령어를 실행하면
    그러면 플로우의 ID, 이름, 설명, 상태, 노드 목록, 연결 정보, 생성일, 수정일이 출력되어야 한다

  시나리오: 존재하지 않는 플로우 조회
    주어진 ID가 "nonexistent"인 플로우가 없을 때
    만약 `xflow flow get nonexistent` 명령어를 실행하면
    그러면 "리소스를 찾을 수 없습니다" 에러가 출력되어야 한다
    그리고 종료 코드가 1이어야 한다
```

### AC-CLI-001-09: 플로우 생성 및 수정
```gherkin
기능: 플로우 생성 및 수정

  시나리오: JSON 파일로 플로우 생성
    주어진 유효한 플로우 정의 JSON 파일이 /tmp/flow.json에 있을 때
    만약 `xflow flow create -f /tmp/flow.json` 명령어를 실행하면
    그러면 "플로우가 생성되었습니다" 메시지와 생성된 플로우 ID가 출력되어야 한다

  시나리오: YAML 파일로 플로우 생성
    주어진 유효한 플로우 정의 YAML 파일이 /tmp/flow.yaml에 있을 때
    만약 `xflow flow create -f /tmp/flow.yaml` 명령어를 실행하면
    그러면 "플로우가 생성되었습니다" 메시지와 생성된 플로우 ID가 출력되어야 한다

  시나리오: 파일 미존재 시 에러
    주어진 /tmp/nofile.json 파일이 존재하지 않을 때
    만약 `xflow flow create -f /tmp/nofile.json` 명령어를 실행하면
    그러면 "파일을 찾을 수 없습니다: /tmp/nofile.json" 에러가 출력되어야 한다

  시나리오: 플로우 수정
    주어진 ID가 "flow-001"인 플로우가 있고
    그리고 유효한 수정 파일이 /tmp/update.json에 있을 때
    만약 `xflow flow update flow-001 -f /tmp/update.json` 명령어를 실행하면
    그러면 "플로우가 수정되었습니다" 메시지가 출력되어야 한다
```

### AC-CLI-001-10: 플로우 삭제
```gherkin
기능: 플로우 삭제 (확인 프롬프트 포함)

  시나리오: 확인 프롬프트 후 삭제
    주어진 Stopped 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow delete flow-001` 명령어를 실행하고
    그리고 확인 프롬프트에 "y"를 입력하면
    그러면 "플로우가 삭제되었습니다" 메시지가 출력되어야 한다

  시나리오: 확인 프롬프트에서 취소
    주어진 Stopped 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow delete flow-001` 명령어를 실행하고
    그리고 확인 프롬프트에 "n"을 입력하면
    그러면 "작업이 취소되었습니다" 메시지가 출력되어야 한다
    그리고 플로우가 삭제되지 않아야 한다

  시나리오: --yes 플래그로 확인 생략
    주어진 Stopped 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow delete flow-001 --yes` 명령어를 실행하면
    그러면 확인 프롬프트 없이 플로우가 삭제되어야 한다

  시나리오: 실행 중인 플로우 삭제 시도
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow delete flow-001 --yes` 명령어를 실행하면
    그러면 서버가 409 Conflict를 반환해야 한다
    그리고 "실행 중인 플로우는 삭제할 수 없습니다" 에러 메시지가 출력되어야 한다
```

### AC-CLI-001-11: 플로우 실행 제어
```gherkin
기능: 플로우 배포, 시작, 중지, 재시작

  시나리오: 플로우 배포
    주어진 Created 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow deploy flow-001` 명령어를 실행하면
    그러면 "플로우가 배포되었습니다" 메시지가 출력되어야 한다

  시나리오: 플로우 시작
    주어진 Deployed 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow start flow-001` 명령어를 실행하면
    그러면 "플로우가 시작되었습니다" 메시지가 출력되어야 한다

  시나리오: 플로우 중지
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow stop flow-001` 명령어를 실행하면
    그러면 "플로우가 중지되었습니다" 메시지가 출력되어야 한다

  시나리오: 플로우 재시작
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow restart flow-001` 명령어를 실행하면
    그러면 "플로우가 재시작되었습니다" 메시지가 출력되어야 한다

  시나리오: 플로우 런타임 상태 조회
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 `xflow flow status flow-001` 명령어를 실행하면
    그러면 실행 상태, 처리 메시지 수, 에러 수, 가동 시간이 출력되어야 한다
```

### AC-CLI-001-12: 플로우 내보내기/가져오기
```gherkin
기능: 플로우 내보내기 및 가져오기

  시나리오: JSON 형식으로 내보내기
    주어진 ID가 "flow-001"인 플로우가 있을 때
    만약 `xflow flow export flow-001 -o /tmp/flow.json` 명령어를 실행하면
    그러면 /tmp/flow.json 파일이 생성되어야 한다
    그리고 파일 내용이 유효한 JSON이어야 한다

  시나리오: YAML 형식으로 내보내기
    주어진 ID가 "flow-001"인 플로우가 있을 때
    만약 `xflow flow export flow-001 -o /tmp/flow.yaml` 명령어를 실행하면
    그러면 /tmp/flow.yaml 파일이 생성되어야 한다
    그리고 파일 내용이 유효한 YAML이어야 한다

  시나리오: 파일에서 플로우 가져오기
    주어진 유효한 플로우 정의 파일이 /tmp/imported.json에 있을 때
    만약 `xflow flow import -f /tmp/imported.json` 명령어를 실행하면
    그러면 새 플로우가 생성되어야 한다
    그리고 생성된 플로우 ID가 출력되어야 한다
```

---

## 4. Agent Commands (P0)

### AC-CLI-001-13: Agent CRUD
```gherkin
기능: Agent 생성, 조회, 삭제

  시나리오: Agent 목록 조회
    주어진 3개의 Agent가 등록되어 있을 때
    만약 `xflow agent list` 명령어를 실행하면
    그러면 3개의 Agent가 테이블 형식으로 출력되어야 한다
    그리고 각 행에 ID, 이름, 타입, 상태, 연결 상태가 포함되어야 한다

  시나리오: Agent 상세 조회
    주어진 ID가 "agent-001"인 Agent가 있을 때
    만약 `xflow agent get agent-001` 명령어를 실행하면
    그러면 Agent의 설정, 통계, 연결 정보가 출력되어야 한다

  시나리오: Agent 생성
    주어진 유효한 Agent 설정 파일이 /tmp/agent.yaml에 있을 때
    만약 `xflow agent create -f /tmp/agent.yaml` 명령어를 실행하면
    그러면 "Agent가 생성되었습니다" 메시지와 Agent ID가 출력되어야 한다

  시나리오: 참조 중인 Agent 삭제 시도
    주어진 Agent "agent-001"이 2개의 플로우에서 참조 중일 때
    만약 `xflow agent delete agent-001 --yes` 명령어를 실행하면
    그러면 "플로우에서 참조 중인 Agent는 삭제할 수 없습니다" 에러가 출력되어야 한다
```

### AC-CLI-001-14: Agent 생명주기 제어
```gherkin
기능: Agent 시작, 중지, 재시작

  시나리오: Agent 시작
    주어진 Stopped 상태의 Agent "agent-001"이 있을 때
    만약 `xflow agent start agent-001` 명령어를 실행하면
    그러면 "Agent가 시작되었습니다" 메시지가 출력되어야 한다

  시나리오: Agent 중지
    주어진 Running 상태의 Agent "agent-001"이 있을 때
    만약 `xflow agent stop agent-001` 명령어를 실행하면
    그러면 "Agent가 중지되었습니다" 메시지가 출력되어야 한다

  시나리오: Agent 재시작
    주어진 Running 상태의 Agent "agent-001"이 있을 때
    만약 `xflow agent restart agent-001` 명령어를 실행하면
    그러면 "Agent가 재시작되었습니다" 메시지가 출력되어야 한다
```

---

## 5. Node Commands (P1)

### AC-CLI-001-15: 노드 타입 조회
```gherkin
기능: 노드 타입 목록 및 상세 조회

  시나리오: 노드 타입 목록 조회
    주어진 10개의 내장 노드 타입과 2개의 플러그인 노드 타입이 있을 때
    만약 `xflow node list` 명령어를 실행하면
    그러면 12개의 노드 타입이 테이블 형식으로 출력되어야 한다
    그리고 각 행에 타입명, 카테고리, 설명, 소스(내장/플러그인)가 포함되어야 한다

  시나리오: 노드 타입 상세 정보 조회
    주어진 "filter" 노드 타입이 있을 때
    만약 `xflow node info filter` 명령어를 실행하면
    그러면 입력 포트, 출력 포트, 설정 스키마, 설명이 출력되어야 한다

  시나리오: 존재하지 않는 노드 타입 조회
    주어진 "nonexistent" 노드 타입이 없을 때
    만약 `xflow node info nonexistent` 명령어를 실행하면
    그러면 "리소스를 찾을 수 없습니다" 에러가 출력되어야 한다
```

---

## 6. Plugin Commands (P1)

### AC-CLI-001-16: 플러그인 관리
```gherkin
기능: 플러그인 설치, 조회, 제거, 업데이트

  시나리오: 플러그인 목록 조회
    주어진 2개의 플러그인이 설치되어 있을 때
    만약 `xflow plugin list` 명령어를 실행하면
    그러면 2개의 플러그인이 테이블 형식으로 출력되어야 한다
    그리고 각 행에 이름, 버전, 타입(Go/WASM), 상태, 제공 노드 수가 포함되어야 한다

  시나리오: 플러그인 설치 (진행 표시기)
    주어진 유효한 플러그인 이름 "custom-nodes"가 있을 때
    만약 `xflow plugin install custom-nodes` 명령어를 실행하면
    그러면 설치 진행 상태 표시기가 출력되어야 한다
    그리고 "플러그인이 설치되었습니다" 메시지가 출력되어야 한다

  시나리오: 사용 중인 플러그인 제거 시도
    주어진 플러그인 "custom-nodes"의 노드 타입이 플로우에서 사용 중일 때
    만약 `xflow plugin remove custom-nodes --yes` 명령어를 실행하면
    그러면 "사용 중인 노드 타입이 있습니다" 경고가 출력되어야 한다

  시나리오: 플러그인 업데이트
    주어진 업데이트 가능한 플러그인 "custom-nodes"가 있을 때
    만약 `xflow plugin update custom-nodes` 명령어를 실행하면
    그러면 "플러그인이 업데이트되었습니다" 메시지와 새 버전이 출력되어야 한다
```

---

## 7. Config Commands (P0)

### AC-CLI-001-17: 설정 초기화 및 관리
```gherkin
기능: CLI 설정 파일 관리

  시나리오: 설정 초기화
    주어진 ~/.xflow/config.yaml 파일이 존재하지 않을 때
    만약 `xflow config init` 명령어를 실행하면
    그러면 ~/.xflow/config.yaml 파일이 기본값으로 생성되어야 한다
    그리고 "설정 파일이 생성되었습니다" 메시지가 출력되어야 한다

  시나리오: 기존 설정 파일 존재 시 초기화
    주어진 ~/.xflow/config.yaml 파일이 이미 존재할 때
    만약 `xflow config init` 명령어를 실행하면
    그러면 "설정 파일이 이미 존재합니다. 덮어쓰시겠습니까?" 확인 프롬프트가 표시되어야 한다

  시나리오: 설정 값 조회
    주어진 설정 파일에 server.url이 "http://example.com:8080"일 때
    만약 `xflow config get server.url` 명령어를 실행하면
    그러면 "http://example.com:8080"이 출력되어야 한다

  시나리오: 설정 값 변경
    주어진 설정 파일이 존재할 때
    만약 `xflow config set server.url http://new-server:8080` 명령어를 실행하면
    그러면 설정 파일의 server.url이 "http://new-server:8080"으로 변경되어야 한다

  시나리오: 서버 URL 단축 명령어
    주어진 설정 파일이 존재할 때
    만약 `xflow config server http://my-server:9090` 명령어를 실행하면
    그러면 설정 파일의 server.url이 "http://my-server:9090"으로 변경되어야 한다

  시나리오: 토큰 단축 명령어
    주어진 설정 파일이 존재할 때
    만약 `xflow config token my-new-token` 명령어를 실행하면
    그러면 설정 파일의 auth.token이 "my-new-token"으로 변경되어야 한다

  시나리오: 전체 설정 조회 (토큰 마스킹)
    주어진 설정 파일에 auth.token이 "my-secret-token"일 때
    만약 `xflow config list` 명령어를 실행하면
    그러면 전체 설정이 출력되어야 한다
    그리고 토큰 값이 "****...oken"과 같이 마스킹되어야 한다
```

---

## 8. Status & Monitoring (P1)

### AC-CLI-001-18: 서버 상태 및 모니터링
```gherkin
기능: 서버 상태 조회 및 모니터링

  시나리오: 서버 상태 조회
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow status` 명령어를 실행하면
    그러면 서버 버전이 출력되어야 한다
    그리고 가동 시간이 출력되어야 한다
    그리고 실행 중인 플로우 수가 출력되어야 한다
    그리고 활성 Agent 수가 출력되어야 한다
    그리고 시스템 리소스 사용량이 출력되어야 한다

  시나리오: 서버 미가동 시 상태 조회
    주어진 xflowd 서버가 중지된 상태일 때
    만약 `xflow status` 명령어를 실행하면
    그러면 "서버에 연결할 수 없습니다" 에러가 출력되어야 한다
    그리고 "xflow config server <url> 명령어로 서버 주소를 확인하세요" 안내가 출력되어야 한다

  시나리오: 로그 스트리밍 (P1 선택적)
    주어진 xflowd 서버가 정상 동작 중이고
    그리고 SSE 스트리밍이 지원될 때
    만약 `xflow logs engine` 명령어를 실행하면
    그러면 engine 컴포넌트의 로그가 실시간으로 출력되어야 한다
    그리고 Ctrl+C로 스트리밍을 중지할 수 있어야 한다

  시나리오: 메트릭 요약 조회 (P1 선택적)
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow metrics` 명령어를 실행하면
    그러면 시스템 메트릭 요약이 출력되어야 한다
```

---

## 9. Output Formatter (P0)

### AC-CLI-001-19: 출력 포맷 지원
```gherkin
기능: 다양한 출력 형식 지원

  시나리오: 테이블 포맷 (기본)
    주어진 3개의 플로우 데이터가 있을 때
    만약 포맷 지정 없이 출력하면
    그러면 정렬된 컬럼 헤더와 행으로 출력되어야 한다
    그리고 컬럼 너비가 데이터에 맞게 자동 조정되어야 한다

  시나리오: JSON 포맷
    주어진 플로우 데이터가 있을 때
    만약 --format json으로 출력하면
    그러면 유효한 JSON이 출력되어야 한다
    그리고 들여쓰기가 적용되어야 한다

  시나리오: YAML 포맷
    주어진 플로우 데이터가 있을 때
    만약 --format yaml으로 출력하면
    그러면 유효한 YAML이 출력되어야 한다

  시나리오: 텍스트 포맷
    주어진 플로우 데이터가 있을 때
    만약 --format text로 출력하면
    그러면 최소한의 스크립팅 친화적 출력이 나와야 한다
    그리고 파이프라인에서 사용할 수 있는 형식이어야 한다

  시나리오: 파이프 연결 시 컬러 자동 비활성화
    주어진 stdout이 파이프에 연결된 상태일 때
    만약 `xflow flow list | cat` 명령어를 실행하면
    그러면 ANSI 이스케이프 시퀀스가 포함되지 않아야 한다

  시나리오: 진행 표시기 표시
    주어진 장시간 작업(플러그인 설치 등)이 진행 중일 때
    만약 작업이 실행되면
    그러면 스피너 또는 프로그레스 바가 표시되어야 한다
    그리고 작업 완료 시 표시기가 사라져야 한다
```

---

## 10. Error Types (P0)

### AC-CLI-001-20: 에러 메시지 및 안내
```gherkin
기능: 사용자 친화적 에러 메시지

  시나리오: 서버 연결 불가
    주어진 xflowd 서버가 중지된 상태이고
    그리고 서버 URL이 "http://localhost:8080"일 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 "서버에 연결할 수 없습니다: http://localhost:8080" 에러가 출력되어야 한다
    그리고 "xflow config server <url> 명령어로 서버 주소를 확인하세요" 안내가 출력되어야 한다

  시나리오: 인증 실패
    주어진 잘못된 토큰이 설정되어 있을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 "인증에 실패했습니다. 토큰을 확인해주세요" 에러가 출력되어야 한다
    그리고 "xflow config token <token> 명령어로 토큰을 설정하세요" 안내가 출력되어야 한다

  시나리오: 권한 부족
    주어진 읽기 전용 권한의 토큰이 설정되어 있을 때
    만약 `xflow flow delete flow-001 --yes` 명령어를 실행하면
    그러면 "이 작업을 수행할 권한이 없습니다" 에러가 출력되어야 한다

  시나리오: 리소스 미발견
    주어진 ID가 "missing-id"인 플로우가 없을 때
    만약 `xflow flow get missing-id` 명령어를 실행하면
    그러면 "리소스를 찾을 수 없습니다: flow missing-id" 에러가 출력되어야 한다

  시나리오: 잘못된 입력 (인자 누락)
    주어진 인자 없이 명령어를 실행할 때
    만약 `xflow flow get` 명령어를 실행하면 (ID 인자 누락)
    그러면 "인자가 필요합니다: <id>" 에러가 출력되어야 한다
    그리고 올바른 사용법 예시가 출력되어야 한다

  시나리오: 파일 미발견
    주어진 /tmp/nonexistent.json 파일이 존재하지 않을 때
    만약 `xflow flow create -f /tmp/nonexistent.json` 명령어를 실행하면
    그러면 "파일을 찾을 수 없습니다: /tmp/nonexistent.json" 에러가 출력되어야 한다

  시나리오: 설정 미초기화
    주어진 설정 파일이 초기화되지 않은 상태이고
    그리고 설정 파일 경로가 --config으로 지정되었지만 파일이 없을 때
    만약 CLI가 설정을 로드하려고 하면
    그러면 기본값으로 동작해야 한다
```

---

## 11. 엣지 케이스

### AC-CLI-001-21: 경계 조건 및 특수 상황
```gherkin
기능: 경계 조건 처리

  시나리오: 빈 목록 응답
    주어진 등록된 플로우가 없을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 "등록된 플로우가 없습니다" 메시지 또는 빈 테이블이 출력되어야 한다
    그리고 종료 코드가 0이어야 한다

  시나리오: 매우 긴 플로우 이름
    주어진 이름이 200자인 플로우가 있을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 테이블이 깨지지 않고 적절히 출력되어야 한다

  시나리오: 특수 문자가 포함된 플로우 이름
    주어진 이름에 한국어, 특수 문자가 포함된 플로우가 있을 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 플로우 이름이 올바르게 표시되어야 한다

  시나리오: 서버 응답 지연
    주어진 xflowd 서버 응답이 25초 걸릴 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 30초 타임아웃 이내이므로 정상 응답을 수신해야 한다

  시나리오: 잘못된 형식의 서버 응답
    주어진 서버가 비표준 응답 형식을 반환할 때
    만약 `xflow flow list` 명령어를 실행하면
    그러면 "서버 응답을 파싱할 수 없습니다" 에러가 출력되어야 한다

  시나리오: 동시에 여러 CLI 인스턴스 실행
    주어진 설정 파일이 존재할 때
    만약 2개의 CLI 프로세스가 동시에 `xflow config set`을 실행하면
    그러면 설정 파일이 손상되지 않아야 한다

  시나리오: 네트워크 중단 후 복구
    주어진 `xflow flow list` 실행 중 네트워크가 중단되었을 때
    만약 연결이 끊어지면
    그러면 타임아웃 후 적절한 에러 메시지가 출력되어야 한다
    그리고 CLI가 행(hang)되지 않아야 한다

  시나리오: 잘못된 --format 값
    주어진 유효하지 않은 형식 값이 지정되었을 때
    만약 `xflow flow list --format xml` 명령어를 실행하면
    그러면 "지원하지 않는 출력 형식입니다: xml. 사용 가능: json, yaml, table, text" 에러가 출력되어야 한다
```

---

## 12. 크로스 플랫폼 호환성

### AC-CLI-001-22: 플랫폼별 동작 검증
```gherkin
기능: 크로스 플랫폼 지원

  시나리오: Linux에서 설정 파일 경로
    주어진 Linux 환경에서 HOME이 "/home/user"일 때
    만약 `xflow config init` 명령어를 실행하면
    그러면 /home/user/.xflow/config.yaml 파일이 생성되어야 한다

  시나리오: macOS에서 설정 파일 경로
    주어진 macOS 환경에서 HOME이 "/Users/user"일 때
    만약 `xflow config init` 명령어를 실행하면
    그러면 /Users/user/.xflow/config.yaml 파일이 생성되어야 한다

  시나리오: Windows에서 설정 파일 경로
    주어진 Windows 환경에서 USERPROFILE이 "C:\Users\user"일 때
    만약 `xflow config init` 명령어를 실행하면
    그러면 C:\Users\user\.xflow\config.yaml 파일이 생성되어야 한다

  시나리오: 바이너리 크로스 컴파일
    주어진 GoReleaser 설정이 완료되었을 때
    만약 크로스 컴파일을 실행하면
    그러면 linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 바이너리가 생성되어야 한다
```

---

## 13. Flow Detail Formatter (P0) - v1.1.0

### AC-CLI-001-23: 플로우 상세 출력 가독성
```gherkin
기능: 플로우 상세 조회 출력 가독성 개선

  시나리오: flow get 기본 출력 형식
    주어진 이름이 "mqtt-to-modbus-v2"인 플로우가 있고
    그리고 해당 플로우에 8개의 노드와 5개의 엣지가 있을 때
    만약 `xflow flow get mqtt-to-modbus-v2` 명령어를 실행하면
    그러면 ID, Name, Status, Nodes, Created At 필드가 정렬된 라벨과 함께 출력되어야 한다
    그리고 출력에 "map[" 문자열이 포함되지 않아야 한다
    그리고 Nodes 섹션에 미니 테이블 형식의 노드 목록이 표시되어야 한다
    그리고 Edges 섹션에 "source_label -> target_label" 형식의 연결 리스트가 표시되어야 한다

  시나리오: 노드 미니 테이블 출력
    주어진 3개의 노드가 있는 플로우가 있고
    그리고 첫 번째 노드의 label이 "mqtt-receiver", type이 "bridge", direction이 "in", agent_name이 "mqtt-agent"이고
    그리고 두 번째 노드의 label이 "transform-node", type이 "transform", direction이 없고, agent_name이 없을 때
    만약 `xflow flow get <flow-id>` 명령어를 실행하면
    그러면 Nodes 섹션에 "mqtt-receiver"가 표시되어야 한다
    그리고 "bridge", "in", "mqtt-agent"가 같은 행에 표시되어야 한다
    그리고 direction이 없는 노드는 "-"로 표시되어야 한다
    그리고 agent_name이 없는 노드는 "-"로 표시되어야 한다

  시나리오: 엣지 연결 리스트 - 노드 ID를 라벨로 해석
    주어진 nodes 배열에 id가 "abc-123"이고 label이 "mqtt-receiver"인 노드와
    그리고 id가 "def-456"이고 label이 "address-resolver"인 노드가 있고
    그리고 edges 배열에 source가 "abc-123", target이 "def-456", sourceHandle이 "out", targetHandle이 "in"인 엣지가 있을 때
    만약 `xflow flow get <flow-id>` 명령어를 실행하면
    그러면 Edges 섹션에 "mqtt-receiver -> address-resolver (out -> in)"이 표시되어야 한다
    그리고 UUID 형식의 원본 노드 ID가 출력에 나타나지 않아야 한다

  시나리오: 매핑되지 않는 노드 ID 처리
    주어진 edges에 존재하지 않는 노드 ID "unknown-id-12345678-abcd"가 참조되어 있을 때
    만약 `xflow flow get <flow-id>` 명령어를 실행하면
    그러면 해당 엣지에서 노드 ID의 앞 8자 "unknown-"이 표시되어야 한다

  시나리오: flow get JSON 출력은 변경 없음
    주어진 플로우가 있을 때
    만약 `xflow flow get <flow-id> --format json` 명령어를 실행하면
    그러면 API 응답 원본 그대로의 JSON이 출력되어야 한다
    그리고 전처리된 node_count나 변환된 데이터가 포함되지 않아야 한다

  시나리오: flow get YAML 출력은 변경 없음
    주어진 플로우가 있을 때
    만약 `xflow flow get <flow-id> --format yaml` 명령어를 실행하면
    그러면 API 응답 원본 그대로의 YAML이 출력되어야 한다
```

### AC-CLI-001-24: 플로우 상세 --detail 플래그 (선택적)
```gherkin
기능: 플로우 상세 표시 수준 제어

  시나리오: --detail summary (기본값)
    주어진 노드에 expression, address_table 등 세부 설정이 있는 플로우가 있을 때
    만약 `xflow flow get <flow-id>` 명령어를 실행하면
    그러면 노드 미니 테이블과 엣지 연결 리스트만 표시되어야 한다
    그리고 expression, address_table 등의 상세 데이터는 표시되지 않아야 한다

  시나리오: --detail full
    주어진 노드에 expression, address_table 등 세부 설정이 있는 플로우가 있을 때
    만약 `xflow flow get <flow-id> --detail full` 명령어를 실행하면
    그러면 노드 미니 테이블 외에 각 노드의 전체 data 맵이 섹션별로 전개되어 출력되어야 한다
    그리고 출력에 "map[" 문자열이 포함되지 않아야 한다
```

---

## 14. Status Detail Formatter (P1) - v1.1.0

### AC-CLI-001-25: 서버 상태 출력 가독성
```gherkin
기능: 서버 상태 출력 가독성 개선

  시나리오: status 명령어 구조화 출력
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow status` 명령어를 실행하면
    그러면 필드가 정렬된 "라벨: 값" 형식으로 출력되어야 한다
    그리고 출력에 "map[" 문자열이 포함되지 않아야 한다
    그리고 중첩 데이터가 있으면 별도 섹션으로 표시되어야 한다

  시나리오: status metrics 구조화 출력
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow status metrics` 명령어를 실행하면
    그러면 메트릭 카테고리(CPU, Memory 등)가 별도 섹션으로 구분되어 출력되어야 한다
    그리고 출력에 "map[" 문자열이 포함되지 않아야 한다

  시나리오: status JSON 출력은 변경 없음
    주어진 xflowd 서버가 정상 동작 중일 때
    만약 `xflow status --format json` 명령어를 실행하면
    그러면 API 응답 원본 그대로의 JSON이 출력되어야 한다
```

---

## 15. TextFormatter Enhancement (P1) - v1.1.0

### AC-CLI-001-26: TextFormatter 중첩 데이터 렌더링
```gherkin
기능: TextFormatter의 중첩 데이터 가독성 개선

  시나리오: 중첩 map 들여쓰기 렌더링
    주어진 data가 {"name": "test", "config": {"host": "localhost", "port": 8080}} 형태일 때
    만약 TextFormatter로 출력하면
    그러면 "config:" 다음 줄에 들여쓰기된 "host: localhost"가 표시되어야 한다
    그리고 "port: 8080"이 들여쓰기된 형태로 표시되어야 한다
    그리고 출력에 "map[host:localhost port:8080]" 같은 raw 형식이 포함되지 않아야 한다

  시나리오: 중첩 slice 리스트 렌더링
    주어진 data가 {"name": "test", "items": ["a", "b", "c"]} 형태일 때
    만약 TextFormatter로 출력하면
    그러면 "items:" 다음 줄에 들여쓰기된 항목 리스트가 표시되어야 한다
    그리고 출력에 "[a b c]" 같은 raw 형식이 포함되지 않아야 한다

  시나리오: map의 slice 렌더링
    주어진 data가 {"nodes": [{"id": "1", "name": "a"}, {"id": "2", "name": "b"}]} 형태일 때
    만약 TextFormatter로 출력하면
    그러면 각 map 요소가 들여쓰기된 키-값 쌍으로 표시되어야 한다
    그리고 요소 간 구분이 있어야 한다
    그리고 출력에 "map[id:1 name:a]" 같은 raw 형식이 포함되지 않아야 한다

  시나리오: 깊은 중첩 데이터 (3단계)
    주어진 data가 {"level1": {"level2": {"level3": "value"}}} 형태일 때
    만약 TextFormatter로 출력하면
    그러면 각 단계가 2칸씩 들여쓰기 되어 표시되어야 한다
    그리고 최종 값 "value"가 올바르게 표시되어야 한다

  시나리오: 단순 키-값 데이터 하위 호환성
    주어진 data가 {"name": "test", "status": "running"} 형태일 때
    만약 TextFormatter로 출력하면
    그러면 기존과 동일하게 "name: test\nstatus: running\n" 형식으로 출력되어야 한다
```

---

## 16. Definition of Done (v1.1.0 추가 항목)

- [ ] `xflow flow get <name>` 출력에 "map[" 문자열이 포함되지 않는다
- [ ] `xflow flow get <name>` 출력에 노드 미니 테이블이 표시된다
- [ ] `xflow flow get <name>` 출력에 엣지 연결 리스트가 라벨 기반으로 표시된다
- [ ] `xflow status` 출력에 "map[" 문자열이 포함되지 않는다
- [ ] `xflow status metrics` 출력에 메트릭 카테고리가 섹션으로 구분된다
- [ ] TextFormatter가 중첩 map/slice를 들여쓰기로 렌더링한다
- [ ] `--format json`, `--format yaml` 출력이 기존과 동일하다
- [ ] 기존 DetailFormatter 사용 명령어(agent get, flow status)에 영향 없다
- [ ] 테스트 커버리지 85% 이상 유지된다
- [ ] golangci-lint 경고가 0건이다

---

## 17. Definition of Done (v1.0.0 원본)

- [ ] 모든 P0 명령어가 구현되어 동작한다 (flow, agent, config, root, version)
- [ ] 모든 P0 요구사항(REQ-CLI-001-XX-XX)에 대한 테스트가 통과한다
- [ ] 테스트 커버리지가 85% 이상이다
- [ ] 4가지 출력 형식(table, JSON, YAML, text)이 모든 명령어에서 동작한다
- [ ] 서버 URL/토큰 해석 우선순위가 올바르게 적용된다
- [ ] 모든 에러 메시지에 원인과 해결 안내가 포함된다
- [ ] `--yes` 플래그로 확인 프롬프트를 건너뛸 수 있다
- [ ] `--no-color` 플래그와 파이프 감지에 의한 컬러 비활성화가 동작한다
- [ ] `--verbose` 플래그로 HTTP 요청/응답 디버깅이 가능하다
- [ ] 설정 파일 미존재 시 기본값으로 동작한다
- [ ] golangci-lint 경고가 0건이다
- [ ] go test -race 에서 데이터 레이스가 0건이다
- [ ] Linux, macOS, Windows에서 크로스 컴파일이 성공한다

---

*SPEC ID: SPEC-CLI-001*
*버전: 1.1.0*
*상태: draft*
*최종 수정: 2026-03-10*
