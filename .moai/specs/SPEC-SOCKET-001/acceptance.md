# SPEC-SOCKET-001: 수락 기준

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SOCKET-001 |
| 형식 | Given-When-Then (Gherkin) |

---

## 1. TCP Server 에이전트 수락 기준

### AC-TCPS-001: 에이전트 등록 및 시작

```gherkin
Given tcp-server 에이전트 타입이 레지스트리에 등록되어 있다
When host=0.0.0.0, port=9100 설정으로 에이전트를 Init하고 Start한다
Then 에이전트 상태가 Running이다
And 9100 포트에서 TCP 리스너가 수신 대기 중이다
```

### AC-TCPS-002: 클라이언트 연결 수신

```gherkin
Given TCP 서버 에이전트가 포트 9100에서 Running 상태이다
When 외부 TCP 클라이언트가 포트 9100에 연결한다
Then 연결이 성공적으로 수락된다
And ConnectionManager에 연결 정보(원격 주소, 연결 시각)가 등록된다
```

### AC-TCPS-003: 최대 접속 수 제한

```gherkin
Given TCP 서버 에이전트가 max_connections=2로 설정되어 Running 상태이다
And 이미 2개의 클라이언트가 연결되어 있다
When 3번째 클라이언트가 연결을 시도한다
Then 연결이 즉시 거절된다
And 기존 2개의 연결은 영향받지 않는다
```

### AC-TCPS-004: 데이터 수신 (newline 프레이밍)

```gherkin
Given TCP 서버 에이전트가 framing=newline으로 설정되어 Running 상태이다
And 클라이언트가 연결되어 있다
When 클라이언트가 "hello\nworld\n" 데이터를 전송한다
Then 에이전트가 "hello"와 "world" 두 개의 메시지를 순서대로 수신한다
```

### AC-TCPS-005: 데이터 수신 (length_prefix 프레이밍)

```gherkin
Given TCP 서버 에이전트가 framing=length_prefix로 설정되어 Running 상태이다
And 클라이언트가 연결되어 있다
When 클라이언트가 4바이트 길이 접두사 + 페이로드를 전송한다
Then 에이전트가 페이로드를 정확히 분리하여 수신한다
```

### AC-TCPS-006: 데이터 수신 (fixed_size 프레이밍)

```gherkin
Given TCP 서버 에이전트가 framing=fixed_size, fixed_size=10으로 설정되어 Running 상태이다
And 클라이언트가 연결되어 있다
When 클라이언트가 30바이트 데이터를 전송한다
Then 에이전트가 10바이트씩 3개의 메시지로 분리하여 수신한다
```

### AC-TCPS-007: 데이터 수신 (raw 프레이밍)

```gherkin
Given TCP 서버 에이전트가 framing=raw, buffer_size=4096으로 설정되어 Running 상태이다
And 클라이언트가 연결되어 있다
When 클라이언트가 1000바이트 데이터를 전송한다
Then 에이전트가 수신된 데이터를 그대로 하나의 메시지로 전달한다
```

### AC-TCPS-008: 연결 차단

```gherkin
Given TCP 서버 에이전트가 Running 상태이다
And 클라이언트 A(192.168.1.100:12345)가 연결되어 있다
When 192.168.1.100 주소에 대한 차단 요청이 수신된다
Then 클라이언트 A의 연결이 즉시 종료된다
And 192.168.1.100이 차단 목록에 추가된다
And 이후 192.168.1.100에서의 새 연결이 거절된다
```

### AC-TCPS-009: 연결 정보 조회

```gherkin
Given TCP 서버 에이전트가 Running 상태이다
And 2개의 클라이언트가 연결되어 있다
When 연결 정보 조회 요청이 수신된다
Then 2개의 연결 정보(원격 주소, 연결 시각, 전송/수신 바이트 수)가 반환된다
```

### AC-TCPS-010: Graceful Shutdown

```gherkin
Given TCP 서버 에이전트가 Running 상태이다
And 3개의 클라이언트가 연결되어 있다
When 에이전트를 Stop한다
Then 모든 클라이언트 연결이 정상 종료된다
And TCP 리스너가 닫힌다
And 에이전트 상태가 Stopped로 전이된다
```

### AC-TCPS-011: Pause/Resume 동작

```gherkin
Given TCP 서버 에이전트가 Running 상태이다
And 클라이언트가 연결되어 있다
When 에이전트를 Pause한다
Then 새 연결은 거절된다
And 기존 연결은 유지된다
And 수신 데이터가 버퍼에 저장된다
When 에이전트를 Resume한다
Then 버퍼에 저장된 데이터가 순서대로 전달된다
And 새 연결이 다시 수락된다
```

---

## 2. TCP Client 에이전트 수락 기준

### AC-TCPC-001: 서버 연결

```gherkin
Given TCP 클라이언트 에이전트가 host=localhost, port=9100으로 설정되어 있다
And 대상 서버가 포트 9100에서 수신 대기 중이다
When 에이전트를 Init하고 Start한다
Then 서버와 TCP 연결이 수립된다
And 에이전트 상태가 Running이다
```

### AC-TCPC-002: 자동 재연결

```gherkin
Given TCP 클라이언트 에이전트가 reconnect_interval=1s로 설정되어 Running 상태이다
And 서버와 연결되어 있다
When 서버가 연결을 끊는다
Then 에이전트가 1초 후 자동으로 재연결을 시도한다
And 서버가 다시 수신 대기 중이면 연결이 복구된다
And 에이전트 상태가 Running을 유지한다
```

### AC-TCPC-003: 최대 재시도 초과

```gherkin
Given TCP 클라이언트 에이전트가 max_retries=3, reconnect_interval=100ms로 설정되어 있다
And 대상 서버가 응답하지 않는다
When 에이전트를 Start한다
Then 3번의 재연결 시도 후 재연결을 중단한다
And 에이전트 상태가 Error로 전이된다
```

### AC-TCPC-004: 무한 재시도

```gherkin
Given TCP 클라이언트 에이전트가 max_retries=0 (무한)으로 설정되어 있다
And 대상 서버가 응답하지 않는다
When 에이전트를 Start한다
Then 에이전트가 계속 재연결을 시도한다
When 서버가 수신 대기를 시작한다
Then 연결이 성공적으로 수립된다
```

### AC-TCPC-005: 연결 타임아웃

```gherkin
Given TCP 클라이언트 에이전트가 connect_timeout=500ms로 설정되어 있다
And 대상 호스트가 연결을 수락하지 않는다 (블랙홀)
When 에이전트가 연결을 시도한다
Then 500ms 후 연결 시도가 타임아웃된다
And 재연결 로직이 트리거된다
```

### AC-TCPC-006: 데이터 송수신

```gherkin
Given TCP 클라이언트 에이전트가 framing=newline으로 서버에 연결되어 있다
When 에이전트를 통해 "test message" 데이터를 전송한다
Then 서버가 "test message\n" 데이터를 수신한다
When 서버가 "response\n" 데이터를 전송한다
Then 에이전트가 "response" 메시지를 수신한다
```

### AC-TCPC-007: Graceful Shutdown

```gherkin
Given TCP 클라이언트 에이전트가 Running 상태이고 재연결 루프가 활성화되어 있다
When 에이전트를 Stop한다
Then 재연결 루프가 중단된다
And TCP 연결이 정상 종료된다
And 에이전트 상태가 Stopped로 전이된다
```

---

## 3. UDP Server 에이전트 수락 기준

### AC-UDPS-001: 데이터그램 수신

```gherkin
Given UDP 서버 에이전트가 port=9200에서 Running 상태이다
When 외부 UDP 클라이언트가 포트 9200으로 데이터그램을 전송한다
Then 에이전트가 데이터그램을 수신한다
And 발신 주소 정보가 메시지 메타데이터에 포함된다
```

### AC-UDPS-002: IP 차단

```gherkin
Given UDP 서버 에이전트가 Running 상태이다
When 192.168.1.100 주소에 대한 차단 요청이 수신된다
Then 192.168.1.100이 차단 목록에 추가된다
When 192.168.1.100에서 데이터그램이 수신된다
Then 해당 데이터그램은 폐기된다
And 처리되지 않는다
```

### AC-UDPS-003: 피어 정보 조회

```gherkin
Given UDP 서버 에이전트가 Running 상태이다
And 3개의 다른 주소에서 데이터그램을 수신했다
When 연결 정보 조회 요청이 수신된다
Then 3개의 피어 정보(주소, 최종 수신 시각, 수신 바이트 수)가 반환된다
```

---

## 4. UDP Client 에이전트 수락 기준

### AC-UDPC-001: 데이터그램 송신

```gherkin
Given UDP 클라이언트 에이전트가 host=localhost, port=9200으로 설정되어 Running 상태이다
When 에이전트를 통해 "test data" 데이터를 전송한다
Then 대상 주소로 UDP 데이터그램이 전송된다
```

### AC-UDPC-002: 응답 수신

```gherkin
Given UDP 클라이언트 에이전트가 Running 상태이다
And 대상 서버가 응답을 보낸다
When 응답 데이터그램이 수신된다
Then 에이전트가 내부 수신 채널로 데이터를 전달한다
```

---

## 5. 소켓 브릿지 노드 수락 기준

### AC-NODE-001: socket-input 노드 (BridgeIn)

```gherkin
Given tcp-server 에이전트가 Running 상태이다
And socket-input 노드가 해당 에이전트에 연결되어 플로우에 배치되어 있다
When 외부 TCP 클라이언트가 데이터를 전송한다
Then socket-input 노드가 데이터를 수신하여 플로우의 다음 노드로 전달한다
And 메시지의 payload에 수신 데이터가 포함된다
And 메시지의 metadata에 소스 정보(에이전트 ID, 원격 주소)가 포함된다
```

### AC-NODE-002: socket-output 노드 (BridgeOut)

```gherkin
Given tcp-client 에이전트가 서버에 연결되어 Running 상태이다
And socket-output 노드가 해당 에이전트에 연결되어 플로우에 배치되어 있다
When 플로우의 이전 노드가 메시지를 socket-output 노드로 전달한다
Then socket-output 노드가 메시지의 payload를 에이전트를 통해 서버로 전송한다
```

### AC-NODE-003: socket-inout 노드 (BridgeInOut)

```gherkin
Given tcp-client 에이전트가 서버에 연결되어 Running 상태이다
And socket-inout 노드가 해당 에이전트에 연결되어 플로우에 배치되어 있다
When 서버가 데이터를 전송한다
Then socket-inout 노드가 데이터를 수신하여 플로우로 전달한다
When 플로우에서 socket-inout 노드로 메시지를 전달한다
Then socket-inout 노드가 메시지를 에이전트를 통해 서버로 전송한다
```

### AC-NODE-004: 노드 레지스트리 등록

```gherkin
Given 시스템이 초기화된다
When 노드 레지스트리를 조회한다
Then socket-input, socket-output, socket-inout 3종의 노드 타입이 등록되어 있다
And 각 노드의 카테고리가 "network"이다
And 각 노드의 설명, 기본 설정, 포트 정보가 올바르게 정의되어 있다
```

---

## 6. 프레이밍 수락 기준

### AC-FRAME-001: 불완전 메시지 처리

```gherkin
Given TCP 에이전트가 framing=newline으로 설정되어 있다
When 클라이언트가 "hel" (불완전 메시지)를 전송한다
Then 에이전트가 추가 데이터를 기다린다
When 클라이언트가 "lo\n" (나머지)를 전송한다
Then 에이전트가 "hello" 메시지를 완성하여 전달한다
```

### AC-FRAME-002: 다중 메시지 일괄 수신

```gherkin
Given TCP 에이전트가 framing=newline으로 설정되어 있다
When 클라이언트가 "msg1\nmsg2\nmsg3\n"을 한 번에 전송한다
Then 에이전트가 "msg1", "msg2", "msg3" 3개의 메시지를 순서대로 전달한다
```

### AC-FRAME-003: 최대 메시지 크기 초과

```gherkin
Given TCP 에이전트가 max_message_size=1024로 설정되어 있다
When 클라이언트가 2048바이트의 단일 메시지를 전송한다
Then 에이전트가 프레이밍 에러를 발생시킨다
And 에러 포트로 에러 메시지가 전달된다
```

---

## 7. 설정 검증 수락 기준

### AC-CFG-001: 필수 설정 누락

```gherkin
Given 소켓 에이전트 설정에 port가 누락되어 있다
When 에이전트를 Init한다
Then 유효성 검증 에러가 반환된다
And 에러 메시지에 "port is required"가 포함된다
```

### AC-CFG-002: 잘못된 프레이밍 설정

```gherkin
Given TCP 에이전트 설정에 framing=invalid_value가 설정되어 있다
When 에이전트를 Init한다
Then 유효성 검증 에러가 반환된다
And 에러 메시지에 지원되는 프레이밍 방식 목록이 포함된다
```

### AC-CFG-003: fixed_size 프레이밍에 크기 누락

```gherkin
Given TCP 에이전트 설정에 framing=fixed_size가 설정되어 있다
And fixed_size 값이 누락되어 있다
When 에이전트를 Init한다
Then 유효성 검증 에러가 반환된다
And 에러 메시지에 "fixed_size is required when framing is fixed_size"가 포함된다
```

---

## 8. 품질 게이트

### Definition of Done

- [ ] 4종 에이전트 (tcp-server, tcp-client, udp-server, udp-client) 구현 완료
- [ ] 3종 소켓 브릿지 노드 (socket-input, socket-output, socket-inout) 구현 완료
- [ ] 모든 단위 테스트 통과 (`go test -race ./internal/agent/socket/...`)
- [ ] 테스트 커버리지 85% 이상
- [ ] 에이전트 레지스트리 등록 확인
- [ ] 노드 레지스트리 등록 확인
- [ ] slog 기반 구조화된 로깅 적용
- [ ] golangci-lint 경고 없음
- [ ] 프레이밍 4종 (raw, newline, length_prefix, fixed_size) 동작 검증
- [ ] TCP 클라이언트 재연결 기능 검증
- [ ] TCP 서버 접속 관리 (조회, 차단) 기능 검증
- [ ] Pause/Resume 시 데이터 무손실 검증

### 검증 방법 및 도구

| 검증 항목 | 도구 | 명령어 |
|-----------|------|--------|
| 단위 테스트 | Go testing | `go test -race -cover ./internal/agent/socket/...` |
| 커버리지 | Go cover | `go test -coverprofile=coverage.out ./internal/agent/socket/...` |
| 린팅 | golangci-lint | `golangci-lint run ./internal/agent/socket/...` |
| 레이스 디텍션 | Go race detector | `go test -race ./internal/agent/socket/...` |
| 통합 테스트 | Go testing | `go test -race ./internal/node/... -run Socket` |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-01*
*작성: MoAI SPEC Builder (manager-spec)*
