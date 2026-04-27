---
id: SPEC-AGENT-002
type: acceptance
version: "1.0.0"
spec_ref: SPEC-AGENT-002
---

# SPEC-AGENT-002 수락 기준

## Module 1: Agent Export (CLI)

### AC-AGENT-002-01: 단일 에이전트 JSON Export

```gherkin
Given xflowd 서버에 ID "agent-01", Name "Test Agent", Type "serial", Config {"port": "/dev/ttyUSB0"} 에이전트가 존재할 때
When `xflow agent export agent-01 -o /tmp/agent.json` 명령을 실행하면
Then /tmp/agent.json 파일이 생성되어야 한다
And 파일 내용이 유효한 JSON이어야 한다
And JSON에 "name": "Test Agent" 필드가 포함되어야 한다
And JSON에 "type": "serial" 필드가 포함되어야 한다
And JSON에 "config" 객체가 포함되어야 한다
```

### AC-AGENT-002-02: 단일 에이전트 YAML Export

```gherkin
Given xflowd 서버에 에이전트 "agent-01"이 존재할 때
When `xflow agent export agent-01 -o /tmp/agent.yaml` 명령을 실행하면
Then /tmp/agent.yaml 파일이 생성되어야 한다
And 파일 내용이 유효한 YAML이어야 한다
And YAML에 name, type 필드가 포함되어야 한다
```

### AC-AGENT-002-03: Export 런타임 필드 제외

```gherkin
Given xflowd 서버에서 에이전트 "agent-01"의 GET 응답에 id, status, connected 필드가 포함될 때
When `xflow agent export agent-01 -o /tmp/agent.json` 명령을 실행하면
Then Export 파일에 "id" 필드가 포함되지 않아야 한다
And Export 파일에 "status" 필드가 포함되지 않아야 한다
And Export 파일에 "connected" 필드가 포함되지 않아야 한다
And Export 파일에 "uptime" 필드가 포함되지 않아야 한다
And Export 파일에 "messages_in" 필드가 포함되지 않아야 한다
And Export 파일에 "messages_out" 필드가 포함되지 않아야 한다
And Export 파일에 "error_count" 필드가 포함되지 않아야 한다
```

### AC-AGENT-002-04: Export 성공 메시지 출력

```gherkin
Given xflowd 서버에 에이전트 "agent-01"이 존재할 때
When `xflow agent export agent-01 -o /tmp/agent.yaml` 명령을 실행하면
Then stdout에 "에이전트 'agent-01' 를 /tmp/agent.yaml 로 내보냈습니다." 메시지가 출력되어야 한다
```

### AC-AGENT-002-05: Export 출력 경로 미지정 에러

```gherkin
Given xflow CLI가 사용 가능할 때
When `xflow agent export agent-01` 명령을 -o 플래그 없이 실행하면
Then 에러가 반환되어야 한다
And 에러 메시지에 "출력 파일 경로(-o)를 지정해야 합니다" 가 포함되어야 한다
```

### AC-AGENT-002-06: 존재하지 않는 에이전트 Export 에러

```gherkin
Given xflowd 서버에 에이전트 "nonexistent"가 존재하지 않을 때
When `xflow agent export nonexistent -o /tmp/agent.json` 명령을 실행하면
Then API 에러가 반환되어야 한다
```

---

## Module 2: Agent Import (CLI)

### AC-AGENT-002-07: JSON 파일 Import

```gherkin
Given {"name": "imported-agent", "type": "tcp", "config": {"address": "192.168.1.100:502"}} 내용의 /tmp/agent.json 파일이 존재할 때
When `xflow agent import -f /tmp/agent.json` 명령을 실행하면
Then POST /api/v1/agents 요청이 전송되어야 한다
And 요청 body에 name: "imported-agent", type: "tcp" 이 포함되어야 한다
And 생성된 에이전트 정보가 출력되어야 한다
```

### AC-AGENT-002-08: YAML 파일 Import

```gherkin
Given name: "yaml-agent", type: "serial" 내용의 /tmp/agent.yaml 파일이 존재할 때
When `xflow agent import -f /tmp/agent.yaml` 명령을 실행하면
Then POST /api/v1/agents 요청이 전송되어야 한다
And 요청 body에 name: "yaml-agent", type: "serial" 이 포함되어야 한다
```

### AC-AGENT-002-09: Import 파일 경로 미지정 에러

```gherkin
Given xflow CLI가 사용 가능할 때
When `xflow agent import` 명령을 -f 플래그 없이 실행하면
Then 에러가 반환되어야 한다
And 에러 메시지에 파일 경로 지정 안내가 포함되어야 한다
```

### AC-AGENT-002-10: Import 잘못된 파일 형식 에러

```gherkin
Given "invalid content <<<" 내용의 /tmp/bad.json 파일이 존재할 때
When `xflow agent import -f /tmp/bad.json` 명령을 실행하면
Then JSON 파싱 에러가 반환되어야 한다
```

### AC-AGENT-002-11: Export -> Import 라운드트립

```gherkin
Given xflowd 서버에 에이전트 "agent-01" (name: "Test", type: "serial", config: {...})이 존재할 때
When `xflow agent export agent-01 -o /tmp/roundtrip.yaml` 명령을 실행하고
And `xflow agent import -f /tmp/roundtrip.yaml` 명령을 실행하면
Then 새 에이전트가 생성되어야 한다
And 새 에이전트의 name이 "Test"이어야 한다
And 새 에이전트의 type이 "serial"이어야 한다
And 새 에이전트의 config가 원본과 동일해야 한다
```

### AC-AGENT-002-12: Import 결과 포맷 출력

```gherkin
Given 유효한 에이전트 파일이 존재할 때
When `xflow agent import -f /tmp/agent.json --format json` 명령을 실행하면
Then 생성된 에이전트 정보가 JSON 형식으로 출력되어야 한다

When `xflow agent import -f /tmp/agent.json --format yaml` 명령을 실행하면
Then 생성된 에이전트 정보가 YAML 형식으로 출력되어야 한다
```

---

## Module 3: Batch Operations

### AC-AGENT-002-13: 디렉토리 일괄 Import

```gherkin
Given /tmp/agents/ 디렉토리에 a.json, b.yaml, c.yml 3개 유효한 에이전트 파일이 존재할 때
When `xflow agent import -f /tmp/agents/` 명령을 실행하면
Then 3개의 POST /api/v1/agents 요청이 전송되어야 한다
And "3개 에이전트 가져오기 완료 (성공: 3, 실패: 0)" 형식의 요약이 출력되어야 한다
```

### AC-AGENT-002-14: Batch Import 부분 실패 처리

```gherkin
Given /tmp/agents/ 디렉토리에 유효한 a.json, 잘못된 b.json, 유효한 c.yaml 파일이 존재할 때
When `xflow agent import -f /tmp/agents/` 명령을 실행하면
Then a.json에 대한 에이전트가 생성되어야 한다
And b.json에 대한 에러 메시지가 출력되어야 한다
And c.yaml에 대한 에이전트가 생성되어야 한다
And "3개 에이전트 가져오기 완료 (성공: 2, 실패: 1)" 형식의 요약이 출력되어야 한다
```

### AC-AGENT-002-15: Batch Import 비에이전트 파일 무시

```gherkin
Given /tmp/agents/ 디렉토리에 agent.yaml, readme.md, notes.txt 파일이 존재할 때
When `xflow agent import -f /tmp/agents/` 명령을 실행하면
Then agent.yaml 파일만 처리되어야 한다
And readme.md, notes.txt 파일은 무시되어야 한다
```

### AC-AGENT-002-16: 전체 에이전트 YAML Export

```gherkin
Given xflowd 서버에 에이전트 3개 (agent-a, agent-b, agent-c)가 존재할 때
When `xflow agent export --all -o /tmp/export/` 명령을 실행하면
Then /tmp/export/ 디렉토리가 생성되어야 한다
And 3개의 .yaml 파일이 생성되어야 한다 (기본 형식: yaml)
And 각 파일명이 에이전트 이름 기반이어야 한다
```

### AC-AGENT-002-17: 전체 에이전트 JSON Export (형식 지정)

```gherkin
Given xflowd 서버에 에이전트 2개가 존재할 때
When `xflow agent export --all -o /tmp/export/ --export-format json` 명령을 실행하면
Then 2개의 .json 파일이 생성되어야 한다
```

### AC-AGENT-002-18: Batch Export -> Batch Import 라운드트립

```gherkin
Given xflowd 서버에 에이전트 2개가 존재할 때
When `xflow agent export --all -o /tmp/roundtrip/` 명령을 실행하고
And `xflow agent import -f /tmp/roundtrip/` 명령을 실행하면
Then 2개의 새 에이전트가 생성되어야 한다
```

---

## Module 4: API Export Endpoint

### AC-AGENT-002-19: 단일 에이전트 API Export

```gherkin
Given xflowd 서버에 에이전트 "agent-01"이 존재할 때
When GET /api/v1/agents/agent-01/export 요청을 전송하면
Then HTTP 200 응답이 반환되어야 한다
And 응답 body에 name, type, config 필드가 포함되어야 한다
And 응답 body에 id, status 필드가 포함되지 않아야 한다
```

### AC-AGENT-002-20: 전체 에이전트 API Export

```gherkin
Given xflowd 서버에 에이전트 2개가 존재할 때
When GET /api/v1/agents/export 요청을 전송하면
Then HTTP 200 응답이 반환되어야 한다
And 응답 body가 JSON 배열이어야 한다
And 배열의 각 요소에 name, type 필드가 포함되어야 한다
And 배열의 각 요소에 id, status 필드가 포함되지 않아야 한다
```

### AC-AGENT-002-21: 존재하지 않는 에이전트 API Export

```gherkin
Given xflowd 서버에 에이전트 "nonexistent"가 존재하지 않을 때
When GET /api/v1/agents/nonexistent/export 요청을 전송하면
Then HTTP 404 응답이 반환되어야 한다
```

### AC-AGENT-002-22: API Export -> CLI Import 호환성

```gherkin
Given GET /api/v1/agents/agent-01/export 응답을 파일로 저장했을 때
When 해당 파일을 `xflow agent import -f <file>` 명령으로 Import하면
Then 에러 없이 새 에이전트가 생성되어야 한다
```

---

## Module 5: Agent Config Examples

### AC-AGENT-002-23: 예제 파일 존재

```gherkin
Given 프로젝트 리포지토리가 존재할 때
Then examples/agents/serial-modbus.yaml 파일이 존재해야 한다
And examples/agents/tcp-custom.json 파일이 존재해야 한다
And examples/agents/mqtt-sensor.yaml 파일이 존재해야 한다
```

### AC-AGENT-002-24: 예제 파일 필수 필드

```gherkin
Given examples/agents/serial-modbus.yaml 파일이 존재할 때
When 파일을 YAML로 파싱하면
Then "name" 필드가 존재하고 비어있지 않아야 한다
And "type" 필드가 존재하고 비어있지 않아야 한다

Given examples/agents/tcp-custom.json 파일이 존재할 때
When 파일을 JSON으로 파싱하면
Then "name" 필드가 존재하고 비어있지 않아야 한다
And "type" 필드가 존재하고 비어있지 않아야 한다

Given examples/agents/mqtt-sensor.yaml 파일이 존재할 때
When 파일을 YAML로 파싱하면
Then "name" 필드가 존재하고 비어있지 않아야 한다
And "type" 필드가 존재하고 비어있지 않아야 한다
```

### AC-AGENT-002-25: 예제 파일 Import 호환성

```gherkin
Given xflowd 서버가 실행 중이고 에이전트 생성이 가능할 때
When `xflow agent import -f examples/agents/serial-modbus.yaml` 명령을 실행하면
Then 에러 없이 에이전트가 생성되어야 한다

When `xflow agent import -f examples/agents/tcp-custom.json` 명령을 실행하면
Then 에러 없이 에이전트가 생성되어야 한다

When `xflow agent import -f examples/agents/mqtt-sensor.yaml` 명령을 실행하면
Then 에러 없이 에이전트가 생성되어야 한다
```

### AC-AGENT-002-26: 예제 파일 설명 주석 (YAML)

```gherkin
Given examples/agents/serial-modbus.yaml 파일이 존재할 때
When 파일 내용을 확인하면
Then YAML 주석(#)으로 각 설정 필드의 용도가 설명되어야 한다
```

---

## Module 6 Additions (구현 중 추가)

### AC-AGENT-002-27: --skip-existing 플래그

Given xflowd 서버에 이름이 "existing-agent"인 에이전트가 존재할 때
When `xflow agent import -f agent.yaml --skip-existing` 명령을 실행하면 (agent.yaml의 name이 "existing-agent")
Then 에이전트가 생성되지 않고 건너뛰어야 한다
And "이미 존재하는 에이전트를 건너뜁니다" 메시지가 출력되어야 한다

### AC-AGENT-002-28: MQTT StatefulAgent

Given MQTT 에이전트가 running 상태일 때
When `xflow agent get <id> --detail full` 명령을 실행하면
Then state 필드에 broker, client_id, connected, qos, topics, topic_count가 포함되어야 한다

### AC-AGENT-002-29: agent list CONNECTED 컬럼

Given 에이전트가 running 상태와 stopped 상태로 존재할 때
When `xflow agent list` 명령을 실행하면
Then CONNECTED 컬럼이 표시되어야 한다
And running 에이전트는 "yes", stopped 에이전트는 "no"로 표시되어야 한다

---

## Quality Gates

### QG-1: 테스트 커버리지

```gherkin
Given 모든 구현이 완료되었을 때
When `go test -cover ./internal/cli/...` 를 실행하면
Then 테스트 커버리지가 85% 이상이어야 한다
```

### QG-2: 경쟁 조건 검사

```gherkin
Given 모든 테스트가 작성되었을 때
When `go test -race ./internal/cli/...` 를 실행하면
Then race condition이 감지되지 않아야 한다
```

### QG-3: 기존 테스트 회귀 방지

```gherkin
Given SPEC-AGENT-002 변경 사항이 적용된 후
When 전체 테스트를 실행하면
Then 기존 CLI 테스트가 모두 통과해야 한다
And 기존 API handler 테스트가 모두 통과해야 한다
```

### QG-4: Flow 패턴 일관성

```gherkin
Given agent export/import 커맨드가 구현되었을 때
When flow export/import 커맨드와 비교하면
Then 동일한 플래그 패턴(-f, -o, --yes)을 사용해야 한다
And 동일한 에러 메시지 패턴을 따라야 한다
And 동일한 출력 형식 자동 감지를 사용해야 한다
```
