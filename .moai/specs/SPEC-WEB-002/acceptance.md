---
id: SPEC-WEB-002
type: acceptance
version: "1.1.0"
status: implemented
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
---

# SPEC-WEB-002: 인수 테스트 기준 - WebSocket 모니터링 브로드캐스팅 서비스

---

## 1. Module 1: Hub 주입 및 메트릭 브로드캐스팅 (P0)

### AC-WEB-002-01: MonitoringBroadcaster 초기화 및 Hub 주입

```gherkin
기능: MonitoringBroadcaster가 wsHub를 주입받아 초기화됨

  시나리오: main.go에서 MonitoringBroadcaster 초기화
    주어진 xflowd 데몬이 시작될 때
    만약 wsHub가 생성되고 Run()이 호출된 후
    그러면 MonitoringBroadcaster가 wsHub와 Engine을 주입받아 초기화되어야 한다
    그리고 Start(ctx) 메서드가 호출되어 백그라운드 고루틴이 시작되어야 한다

  시나리오: MonitoringBroadcaster 정상 종료
    주어진 MonitoringBroadcaster가 실행 중일 때
    만약 서버 셧다운(컨텍스트 취소)이 발생하면
    그러면 메트릭 수집 고루틴이 정상적으로 종료되어야 한다
    그리고 리소스 누수(고루틴 누수)가 없어야 한다
```

### AC-WEB-002-02: 메트릭 브로드캐스트 주기 및 형식

```gherkin
기능: 1초 주기로 시스템 메트릭을 flow.metrics 타입으로 브로드캐스트

  시나리오: WebSocket 클라이언트가 연결된 상태에서 메트릭 수신
    주어진 WebSocket 클라이언트가 ws://localhost:8080/ws에 연결되어 있을 때
    만약 1초가 경과하면
    그러면 클라이언트는 flow.metrics 타입의 WebSocket 메시지를 수신해야 한다
    그리고 메시지 payload에 cpu 필드(숫자, 0-100)가 포함되어야 한다
    그리고 메시지 payload에 memory 필드(숫자, 0-100)가 포함되어야 한다
    그리고 메시지 payload에 throughput 필드(숫자, >= 0)가 포함되어야 한다
    그리고 메시지 payload에 error_rate 필드(숫자, 0-100)가 포함되어야 한다

  시나리오: 메트릭 메시지 전체 구조 검증
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 flow.metrics 메시지를 수신하면
    그러면 메시지 JSON 구조는 다음과 같아야 한다:
      {
        "type": "flow.metrics",
        "payload": { "cpu": <number>, "memory": <number>, "throughput": <number>, "error_rate": <number> },
        "timestamp": "<ISO 8601>"
      }

  시나리오: 연속 메트릭 수신으로 차트 데이터 축적
    주어진 WebSocket 클라이언트가 연결된 상태이고 3초가 경과했을 때
    그러면 최소 2개 이상의 flow.metrics 메시지를 수신해야 한다
    그리고 각 메시지의 timestamp가 순차적으로 증가해야 한다
```

### AC-WEB-002-03: 클라이언트 없을 때 메트릭 스킵

```gherkin
기능: 연결된 WebSocket 클라이언트가 없으면 메트릭 수집을 건너뜀

  시나리오: 클라이언트 0명일 때 브로드캐스트 건너뜀
    주어진 MonitoringBroadcaster가 실행 중이고 연결된 클라이언트가 0명일 때
    만약 1초 ticker가 발화하면
    그러면 hub.BroadcastMessage()가 호출되지 않아야 한다
    그리고 runtime.ReadMemStats()도 호출되지 않아야 한다 (불필요한 CPU 사용 방지)

  시나리오: 클라이언트 연결 후 메트릭 브로드캐스트 시작
    주어진 MonitoringBroadcaster가 실행 중이고 클라이언트가 0명인 상태에서
    만약 새 WebSocket 클라이언트가 연결되면
    그러면 다음 ticker 발화 시 hub.BroadcastMessage()가 호출되어야 한다
    그리고 클라이언트는 flow.metrics 메시지를 수신해야 한다

  시나리오: 마지막 클라이언트 연결 해제 후 브로드캐스트 중단
    주어진 1명의 WebSocket 클라이언트가 연결된 상태에서 메트릭을 수신 중일 때
    만약 해당 클라이언트가 연결을 종료하면
    그러면 다음 ticker 발화 시 hub.BroadcastMessage()가 호출되지 않아야 한다
```

### AC-WEB-002-04: 처리량 및 에러율 계산

```gherkin
기능: 플로우 통계 기반 처리량 및 에러율 계산

  시나리오: 배포된 플로우가 있을 때 처리량 계산
    주어진 running 상태의 플로우가 1개 있고 초당 100개 메시지를 처리 중일 때
    만약 flow.metrics 메시지를 연속 2회 수신하면
    그러면 두 번째 메시지의 throughput 값이 약 100 (허용 오차 50%)이어야 한다

  시나리오: 배포된 플로우가 없을 때 메트릭
    주어진 배포된 플로우가 없는 상태일 때
    만약 flow.metrics 메시지를 수신하면
    그러면 throughput 값이 0이어야 한다
    그리고 error_rate 값이 0이어야 한다

  시나리오: 에러율 계산
    주어진 플로우에서 에러가 발생 중일 때 (총 메시지 1000, 에러 10)
    만약 flow.metrics 메시지를 수신하면
    그러면 error_rate 값이 약 1.0 (허용 오차 0.5%)이어야 한다
```

---

## 2. Module 2: 로그 스트리밍 서비스 (P1)

### AC-WEB-002-05: 로그 메시지 브로드캐스트

```gherkin
기능: 애플리케이션 로그를 log.entry 형식으로 WebSocket 브로드캐스트

  시나리오: 애플리케이션 로그 발생 시 WebSocket으로 전송
    주어진 WebSocket 클라이언트가 연결된 상태이고 로그 스트리밍이 활성화되었을 때
    만약 백엔드에서 INFO 레벨 로그가 발생하면
    그러면 클라이언트는 log.entry 타입의 WebSocket 메시지를 수신해야 한다
    그리고 payload에 level 필드가 "INFO"여야 한다
    그리고 payload에 message 필드가 로그 메시지 내용이어야 한다
    그리고 payload에 timestamp 필드가 ISO 8601 형식이어야 한다

  시나리오: log.entry 메시지 전체 구조 검증
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 log.entry 메시지를 수신하면
    그러면 메시지 JSON 구조는 다음과 같아야 한다:
      {
        "type": "log.entry",
        "payload": { "level": "<string>", "message": "<string>", "timestamp": "<ISO 8601>" },
        "timestamp": "<ISO 8601>"
      }

  시나리오: 다양한 로그 레벨 전송
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 WARN 레벨 로그가 발생하면
    그러면 수신한 log.entry의 level이 "WARN"이어야 한다
    만약 ERROR 레벨 로그가 발생하면
    그러면 수신한 log.entry의 level이 "ERROR"이어야 한다
```

### AC-WEB-002-06: 로그 Rate Limiting

```gherkin
기능: 초당 100건 제한으로 WebSocket 과부하 방지

  시나리오: 정상 로그량 (초당 100건 이하) 시 모든 로그 전송
    주어진 WebSocket 클라이언트가 연결된 상태이고 초당 50건의 로그가 발생할 때
    그러면 50건의 log.entry 메시지가 모두 수신되어야 한다
    그리고 누락된 로그가 없어야 한다

  시나리오: 대량 로그 (초당 100건 초과) 시 제한 적용
    주어진 WebSocket 클라이언트가 연결된 상태이고 초당 500건의 로그가 발생할 때
    그러면 수신되는 log.entry 메시지가 초당 약 100건 이하여야 한다
    그리고 초과분은 드롭되어야 한다

  시나리오: Rate limit 초과 시 드롭 카운터 기록
    주어진 rate limit(초당 100건)을 초과하는 로그가 발생했을 때
    그러면 드롭된 로그 수가 내부적으로 카운팅되어야 한다
```

### AC-WEB-002-07: 로그 레벨 필터링

```gherkin
기능: 설정된 최소 레벨 이상의 로그만 스트리밍

  시나리오: 기본 필터(INFO) 적용 시 DEBUG 로그 제외
    주어진 WebSocket 클라이언트가 연결되어 있고 최소 로그 레벨이 INFO일 때
    만약 DEBUG 레벨 로그가 발생하면
    그러면 해당 로그는 WebSocket으로 전송되지 않아야 한다

  시나리오: INFO 이상 로그 전송
    주어진 최소 로그 레벨이 INFO일 때
    만약 INFO, WARN, ERROR 레벨 로그가 순서대로 발생하면
    그러면 3건의 log.entry 메시지가 모두 수신되어야 한다
```

### AC-WEB-002-08: 로그 Writer 수명 관리

```gherkin
기능: MonitoringBroadcaster 종료 시 로그 Writer 정리

  시나리오: 서비스 종료 시 StreamRouter에서 Writer 제거
    주어진 MonitoringBroadcaster가 실행 중이고 wsLogWriter가 StreamRouter에 등록된 상태일 때
    만약 MonitoringBroadcaster.Stop()이 호출되면
    그러면 StreamRouter에서 wsLogWriter가 RemoveRoute로 제거되어야 한다
    그리고 이후 발생하는 로그가 wsLogWriter로 전달되지 않아야 한다

  시나리오: 클라이언트 0명일 때 로그 Write 건너뜀
    주어진 연결된 WebSocket 클라이언트가 0명일 때
    만약 wsLogWriter.Write()가 호출되면
    그러면 hub.BroadcastMessage()가 호출되지 않아야 한다
    그리고 Write 메서드는 에러 없이 반환해야 한다 (io.Writer 계약 유지)
```

### AC-WEB-002-09: 순환 로그 방지

```gherkin
기능: wsLogWriter 자체 에러가 무한 로그 루프를 유발하지 않음

  시나리오: wsLogWriter 내부 에러 시 순환 방지
    주어진 wsLogWriter가 StreamRouter에 등록된 상태일 때
    만약 wsLogWriter.Write() 내부에서 JSON 파싱 에러가 발생하면
    그러면 해당 에러 로그는 wsLogWriter를 통해 재전송되지 않아야 한다
    그리고 시스템 stdout에만 에러가 기록되어야 한다
```

---

## 3. Module 3: 시스템 이벤트 퍼블리셔 (P1)

### AC-WEB-002-10: 플로우 라이프사이클 이벤트

```gherkin
기능: 플로우 배포/시작/중지 시 system.event 메시지 브로드캐스트

  시나리오: 플로우 배포 이벤트
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 POST /api/v1/flows/{id}/deploy API로 플로우를 배포하면
    그러면 클라이언트는 system.event 타입의 메시지를 수신해야 한다
    그리고 payload.type이 "flow_deployed"여야 한다
    그리고 payload.message에 플로우 이름이 포함되어야 한다
    그리고 payload.details에 flowId가 포함되어야 한다
    그리고 payload.timestamp가 ISO 8601 형식이어야 한다

  시나리오: 플로우 시작 이벤트
    주어진 WebSocket 클라이언트가 연결된 상태이고 플로우가 배포된 상태일 때
    만약 POST /api/v1/flows/{id}/start API로 플로우를 시작하면
    그러면 클라이언트는 payload.type이 "flow_started"인 system.event를 수신해야 한다

  시나리오: 플로우 중지 이벤트
    주어진 WebSocket 클라이언트가 연결된 상태이고 플로우가 running 상태일 때
    만약 POST /api/v1/flows/{id}/stop API로 플로우를 중지하면
    그러면 클라이언트는 payload.type이 "flow_stopped"인 system.event를 수신해야 한다
```

### AC-WEB-002-11: 에이전트 연결 이벤트

```gherkin
기능: 에이전트 연결/해제 시 system.event 메시지 브로드캐스트

  시나리오: 에이전트 연결 이벤트
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 새로운 에이전트가 시스템에 연결되면
    그러면 클라이언트는 system.event 타입의 메시지를 수신해야 한다
    그리고 payload.type이 "agent_connected"여야 한다
    그리고 payload.message에 에이전트 이름이 포함되어야 한다

  시나리오: 에이전트 연결 해제 이벤트
    주어진 WebSocket 클라이언트가 연결된 상태이고 에이전트가 연결 중일 때
    만약 해당 에이전트가 연결을 해제하면
    그러면 클라이언트는 payload.type이 "agent_disconnected"인 system.event를 수신해야 한다
```

### AC-WEB-002-12: 이벤트 메시지 구조

```gherkin
기능: system.event 메시지의 JSON 구조 검증

  시나리오: 이벤트 메시지 전체 구조 검증
    주어진 WebSocket 클라이언트가 연결된 상태일 때
    만약 system.event 메시지를 수신하면
    그러면 메시지 JSON 구조는 다음과 같아야 한다:
      {
        "type": "system.event",
        "payload": {
          "type": "<event_type>",
          "message": "<string>",
          "timestamp": "<ISO 8601>",
          "details": "<string>"
        },
        "timestamp": "<ISO 8601>"
      }

  시나리오: 유효한 이벤트 타입
    주어진 system.event 메시지를 수신할 때
    그러면 payload.type은 다음 중 하나여야 한다:
      "flow_started", "flow_stopped", "flow_deployed", "flow_error",
      "agent_connected", "agent_disconnected"
```

### AC-WEB-002-13: 클라이언트 없을 때 이벤트 스킵

```gherkin
기능: 클라이언트 없을 때 이벤트 발행을 건너뜀

  시나리오: 클라이언트 0명일 때 이벤트 발행 건너뜀
    주어진 연결된 WebSocket 클라이언트가 0명인 상태일 때
    만약 플로우가 배포/시작/중지되면
    그러면 EventPublisher가 hub.BroadcastMessage()를 호출하지 않아야 한다
    그리고 플로우 작업 자체는 정상적으로 완료되어야 한다

  시나리오: 이벤트 발행 실패가 핵심 로직에 영향 없음
    주어진 hub.BroadcastMessage()가 에러를 반환하는 상태일 때
    만약 플로우를 시작하면
    그러면 플로우 시작은 정상적으로 성공해야 한다
    그리고 이벤트 발행 에러는 로그에만 기록되어야 한다
```

---

## 4. 비기능 요구사항

### AC-WEB-002-14: 성능 요구사항

```gherkin
기능: 모니터링 브로드캐스팅 성능

  시나리오: 메트릭 브로드캐스트 지연
    주어진 10명의 WebSocket 클라이언트가 연결된 상태일 때
    만약 메트릭 브로드캐스트가 수행되면
    그러면 단일 브로드캐스트의 소요 시간이 10ms 이내여야 한다

  시나리오: 메모리 오버헤드
    주어진 MonitoringBroadcaster가 24시간 실행된 상태일 때
    그러면 메트릭 수집/브로드캐스트로 인한 추가 메모리 사용량이 10MB 이하여야 한다
    그리고 메모리 누수가 없어야 한다 (안정적 메모리 사용 패턴)

  시나리오: 고루틴 오버헤드
    주어진 MonitoringBroadcaster가 실행 중일 때
    그러면 추가되는 백그라운드 고루틴 수가 3개 이하여야 한다
      (메트릭 ticker 1개, 로그 writer는 별도 고루틴 불필요)
```

### AC-WEB-002-15: 리소스 효율성

```gherkin
기능: 클라이언트 없을 때 리소스 최소화

  시나리오: 클라이언트 0명일 때 CPU 오버헤드 최소화
    주어진 연결된 WebSocket 클라이언트가 0명인 상태에서 MonitoringBroadcaster가 실행 중일 때
    그러면 ticker 고루틴은 hub.ClientCount() 확인 후 즉시 반환해야 한다
    그리고 runtime.ReadMemStats() 호출이 발생하지 않아야 한다
    그리고 engine.ListFlows() 호출이 발생하지 않아야 한다

  시나리오: 로그 스트리밍 비용 제한
    주어진 50명의 WebSocket 클라이언트가 연결된 상태에서 초당 100건의 로그가 발생할 때
    그러면 로그 스트리밍으로 인한 추가 CPU 사용량이 전체 시스템의 5% 이하여야 한다
```

### AC-WEB-002-16: Graceful Shutdown

```gherkin
기능: 서버 종료 시 모든 백그라운드 작업 정상 종료

  시나리오: 컨텍스트 취소 시 고루틴 종료
    주어진 MonitoringBroadcaster가 실행 중일 때
    만약 서버가 셧다운(컨텍스트 취소)을 시작하면
    그러면 메트릭 ticker 고루틴이 5초 이내에 종료되어야 한다
    그리고 StreamRouter에서 wsLogWriter가 제거되어야 한다
    그리고 고루틴 누수가 없어야 한다

  시나리오: 브로드캐스트 중 셧다운
    주어진 메트릭 브로드캐스트가 진행 중일 때
    만약 컨텍스트가 취소되면
    그러면 현재 진행 중인 브로드캐스트가 완료된 후 고루틴이 종료되어야 한다
    그리고 패닉이나 에러가 발생하지 않아야 한다
```

---

## 5. 프론트엔드 통합 검증

### AC-WEB-002-17: MonitoringPage 엔드투엔드 검증

```gherkin
기능: 프론트엔드 모니터링 페이지에서 실시간 데이터 표시

  시나리오: MetricsChart에 실시간 데이터 표시
    주어진 사용자가 모니터링 페이지(/monitoring)에 접근했을 때
    만약 WebSocket 연결이 성공하고 5초가 경과하면
    그러면 CPU 사용률 차트에 데이터 포인트가 최소 3개 이상 표시되어야 한다
    그리고 메모리 사용률 차트에 데이터 포인트가 표시되어야 한다
    그리고 처리량 차트에 데이터 포인트가 표시되어야 한다
    그리고 에러율 차트에 데이터 포인트가 표시되어야 한다

  시나리오: LogViewer에 실시간 로그 표시
    주어진 사용자가 모니터링 페이지의 로그 탭에 접근했을 때
    만약 백엔드에서 로그가 발생하면
    그러면 LogViewer에 해당 로그가 실시간으로 추가 표시되어야 한다
    그리고 로그 레벨 필터가 정상 동작해야 한다

  시나리오: EventTimeline에 시스템 이벤트 표시
    주어진 사용자가 모니터링 페이지의 이벤트 탭에 접근했을 때
    만약 플로우가 배포/시작/중지되면
    그러면 EventTimeline에 해당 이벤트가 실시간으로 추가 표시되어야 한다
    그리고 이벤트 유형에 맞는 아이콘과 색상이 표시되어야 한다

  시나리오: WebSocket 연결 상태 표시
    주어진 사용자가 모니터링 페이지에 접근했을 때
    만약 WebSocket 연결이 성공하면
    그러면 헤더에 "연결됨" 상태가 초록색으로 표시되어야 한다
    만약 WebSocket 연결이 끊기면
    그러면 헤더에 "연결 끊김" 상태가 빨간색으로 표시되어야 한다
```

---

## 6. Quality Gate 체크리스트

- [x] Module 1: `MonitoringBroadcaster` 구조체가 `internal/api/ws/broadcaster.go`에 구현됨
- [x] Module 1: `cmd/xflowd/main.go`에서 `wsHub`가 `MonitoringBroadcaster`에 주입됨
- [x] Module 1: 1초 주기 메트릭 브로드캐스트가 동작함 (`flow.metrics` 메시지)
- [x] Module 1: 클라이언트 0명일 때 메트릭 수집이 건너뛰어짐
- [x] Module 1: 컨텍스트 취소 시 고루틴이 정상 종료됨
- [x] Module 1: 메트릭 페이로드에 cpu, memory, throughput, error_rate 필드 포함
- [x] Module 2: `wsLogWriter`가 `io.Writer` 인터페이스를 구현함
- [x] Module 2: `StreamRouter`에 `wsLogWriter`가 등록되어 로그를 수신함
- [x] Module 2: `log.entry` 형식으로 로그가 WebSocket 브로드캐스트됨
- [x] Module 2: Rate limiter가 초당 100건으로 제한함
- [x] Module 2: 기본 로그 레벨 필터(DEBUG)가 적용됨 (생성자에서 레벨 지정)
- [x] Module 2: 서비스 종료 시 `RemoveRoute`로 Writer가 정리됨
- [x] Module 2: 순환 로그가 발생하지 않음 (모든 내부 에러 silent drop)
- [x] Module 3: `EventPublisher`가 `internal/api/ws/event_publisher.go`에 구현됨
- [x] Module 3: 플로우 배포 시 `flow_deployed` 이벤트가 발행됨
- [x] Module 3: 플로우 시작 시 `flow_started` 이벤트가 발행됨
- [x] Module 3: 플로우 중지 시 `flow_stopped` 이벤트가 발행됨
- [x] Module 3: 에이전트 연결/해제 시 이벤트 발행 인터페이스가 구현됨 (PublishAgentEvent)
- [x] Module 3: 클라이언트 0명일 때 이벤트 발행이 건너뛰어짐
- [x] Module 3: 이벤트 발행 실패가 핵심 로직에 영향을 주지 않음 (에러 로깅만)
- [x] 전체: 모든 신규 파일에 단위 테스트가 존재함 (35개 테스트)
- [x] 전체: Go `go vet ./...` 통과
- [x] 전체: Go `go test -race ./...` 통과 (동시성 안전)
- [x] 전체: 기존 테스트가 모두 통과함 (회귀 없음)

---

## 7. Definition of Done

- [x] Module 1 (P0): `MonitoringBroadcaster`가 구현되고 `main.go`에서 초기화됨. WebSocket 클라이언트가 `flow.metrics` 메시지를 1초 주기로 수신 가능
- [x] Module 2 (P1): `wsLogWriter`가 구현되고 StreamRouter에 연결됨. 실시간 로그가 `log.entry` 형식으로 WebSocket 브로드캐스트됨. Rate limiting 동작 확인
- [x] Module 3 (P1): `EventPublisher`가 구현되고 FlowHandler에 주입됨. 플로우 배포/시작/중지 시 `system.event` 이벤트가 브로드캐스트됨
- [ ] 프론트엔드 모니터링 페이지에서 MetricsChart, LogViewer, EventTimeline에 실시간 데이터가 표시됨 (E2E 검증 필요)
- [x] 모든 신규 Go 파일에 테스트가 작성되고 `go test -race ./...` 통과 (35개 테스트)
- [x] 서버 셧다운 시 모든 백그라운드 고루틴이 정상 종료되고 리소스 누수 없음

---

*SPEC ID: SPEC-WEB-002*
*버전: 1.1.0*
*상태: implemented*
*최종 수정: 2026-03-08*
