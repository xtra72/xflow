---
id: SPEC-THINGPLUS-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-THINGPLUS-001
---

# SPEC-THINGPLUS-001 수락 기준

## 핵심 시나리오 (Given/When/Then)

### AC-THINGPLUS-001-01: 미등록 디바이스 업링크 auto-connect + 텔레메트리 (REQ-map-connect, up-telemetry, up-ts)

```gherkin
Given thingplus-gateway 에이전트가 게이트웨이 access token으로 브로커에 연결된 상태이고
And device_name_path가 기본값 "$.device"로 설정되어 있고
And "Device A"가 아직 게이트웨이에 연결되지 않은 상태일 때
When {"device": "Device A", "temperature": 42} 페이로드의 인입 메시지를 처리하면
Then 에이전트는 먼저 v1/gateway/connect에 {"device":"Device A"}를 발행해야 한다
And PUBACK 수신 후 "Device A" 상태가 connected로 전이되어야 한다
And v1/gateway/telemetry에 {"Device A":[{"ts":<epoch_ms>,"values":{"temperature":42}}]} 형식으로 발행해야 한다
And ts 값은 time.Time.UnixMilli() 결과(int64 epoch milliseconds)여야 한다
```

### AC-THINGPLUS-001-02: 다운링크 RPC → thingplus.rpc.request 방출 → 플로우 응답 → RPC 발행 (REQ-dn-rpc-recv, dn-rpc-reply)

```gherkin
Given thingplus-gateway 에이전트가 브로커에 연결되어 v1/gateway/rpc를 구독한 상태이고
And "Device A"가 connected 상태일 때
When 브로커가 v1/gateway/rpc로 {"device":"Device A","data":{"id":1,"method":"setValue","params":{"v":10}}}를 전달하면
Then 에이전트는 Type()이 "thingplus.rpc.request"인 플로우 메시지를 방출해야 한다
And 방출된 메시지는 device_id(또는 fallback NAME "Device A")와 RPC id 1을 포함해야 한다

When 플로우가 해당 RPC에 대한 응답 메시지를 에이전트로 전달하면
Then 에이전트는 v1/gateway/rpc에 {"device":"Device A","id":1,"data":{"success":true}} 형식으로 응답을 발행해야 한다
```

### AC-THINGPLUS-001-03: 재연결 시 알려진 디바이스 재connect (REQ-core-reconnect, map-reconnect)

```gherkin
Given thingplus-gateway 에이전트가 브로커에 연결되어 있고
And "Device A"와 "Device B"가 connected 상태로 알려져 있을 때
When 브로커 연결이 끊겼다가 재연결되면
Then OnConnect 핸들러에서 v1/gateway/rpc와 v1/gateway/attributes를 재구독해야 한다
And v1/gateway/connect에 {"device":"Device A"}와 {"device":"Device B"}를 다시 발행해야 한다
And 두 디바이스 모두 connected 상태로 복구되어야 한다
```

### AC-THINGPLUS-001-04: 공유 속성 push → thingplus.attr.update 방출 (REQ-dn-shared, dn-name-resolve)

```gherkin
Given thingplus-gateway 에이전트가 브로커에 연결되어 v1/gateway/attributes를 구독한 상태일 때
When 브로커가 v1/gateway/attributes로 {"device":"Device A","data":{"fw":"1.0"}}를 전달하면
Then 에이전트는 "Device A"를 device_id로 역해석해야 한다
And Type()이 "thingplus.attr.update"인 플로우 메시지를 방출해야 한다
And 방출된 메시지 페이로드에 {"fw":"1.0"} 속성 데이터가 포함되어야 한다
```

---

## 엣지 케이스

### AC-THINGPLUS-001-05: 연결 끊김 시 업링크 무손실 버퍼링 (REQ-up-nolost)

```gherkin
Given thingplus-gateway 에이전트의 업링크 버퍼(buffer_size)가 설정되어 있고
And 브로커 연결이 끊긴 상태일 때
When 텔레메트리 인입 메시지가 도착하면
Then 메시지를 조용히 폐기하지 않고 bounded 버퍼에 저장해야 한다
And 버퍼가 가득 차면 메트릭 또는 에러로 관찰 가능하게 처리해야 한다 (silent drop 금지)

When 브로커가 재연결되면
Then 버퍼에 저장된 텔레메트리가 발행되어야 한다
```

### AC-THINGPLUS-001-06: access token 로그 미노출 (REQ-core-nocred-log)

```gherkin
Given thingplus-gateway 에이전트가 access token으로 설정된 상태일 때
When 연결/재연결/에러 로그가 출력되면
Then access token 값이 평문으로 로그에 나타나지 않아야 한다
And State() 스냅샷에도 토큰이 평문으로 노출되지 않아야 한다
```

### AC-THINGPLUS-001-07: 중복 NAME 및 repo-nil fallback 매핑 (REQ-map-resolve, map-fallback)

```gherkin
Given device_id_repo가 nil이거나 ResolveDeviceID가 매핑을 찾지 못하는 상태일 때
When "Device A" NAME이 인입 메시지에서 추출되면
Then 에이전트는 NAME "Device A" 자체를 device_id 키로 사용해야 한다
And 동작이 중단되지 않아야 한다

Given 동일한 "Device A" NAME이 여러 메시지에서 반복 추출될 때
Then NAME↔device_id 매핑이 일관성을 유지해야 한다 (중복 등록으로 인한 불일치 없음)
```

### AC-THINGPLUS-001-08: connect PUBACK 이전 RPC 게이팅 (REQ-map-puback)

```gherkin
Given "Device A"가 connecting 상태(connect 발행 후 PUBACK 미수신)일 때
When 해당 디바이스에 대한 RPC 응답 발행이 요청되면
Then 에이전트는 PUBACK 수신(connected 전이)까지 발행을 게이팅해야 한다
And connected 전이 후 발행이 진행되어야 한다
```

### AC-THINGPLUS-001-09: 타입 등록 및 설정 파싱 (REQ-core-register, core-config)

```gherkin
Given RegisterThingplusTypes(mgr)가 호출된 상태일 때
Then "thingplus-gateway" 타입이 type_registry에 등록되어 있어야 한다
And 팩토리로 ThingplusGatewayAgent가 생성 가능해야 한다

Given 설정에 device_name_path가 없을 때
When Init(config)를 호출하면
Then device_name_path 기본값이 "$.device"여야 한다
And port 기본값이 1883이어야 한다
```

---

## 통합 테스트 (channel-mocked broker)

### AC-THINGPLUS-001-10: 전체 왕복 흐름 통합 테스트

```gherkin
Given channel-mocked broker에 연결된 thingplus-gateway 에이전트가 주어졌을 때

When {"device":"Device A","temperature":42} 인입 메시지를 처리하면
Then v1/gateway/connect 발행 → PUBACK → v1/gateway/telemetry 발행이 순서대로 수행되어야 한다

When 모킹 브로커가 v1/gateway/rpc로 RPC 요청을 주입하면
Then thingplus.rpc.request 메시지가 방출되어야 한다

When 플로우 응답을 주입하면
Then v1/gateway/rpc에 RPC 응답이 발행되어야 한다

When 연결 끊김→재연결이 발생하면
Then "Device A"가 다시 connect되어야 한다
```

---

## 품질 게이트

### Definition of Done

- [x] 모든 수락 기준(AC-THINGPLUS-001-01 ~ 10) 테스트 통과 (채널 모킹 브로커 기반 단위/통합 테스트)
- [x] `go test ./internal/agent/...` 전체 통과
- [x] `go test -race ./internal/agent/...` 경쟁 상태 없음
- [x] `go vet ./internal/agent/...` 경고 없음
- [x] `golangci-lint` 클린 (0 issues)
- [x] 테스트 커버리지 85% 이상 (`go test -cover`) — codec 92.6% / mapping 96.6% / adapter 95.0% / 에이전트 코어 80.4%(브로커 전용 경로 제외 시 >90%)
- [x] GoDoc 주석 작성 완료 (ThingplusGatewayAgent, ThingplusConfig, 코덱/매핑 공개 심볼)
- [ ] 라이브 브로커 스모크 테스트로 attributes/response 인코딩(A8) 및 v5 PUBACK 타이밍(A7) 확인 (M4 확정 전) — **이연**: 라이브 브로커 접근이 필요한 항목으로 DoD에 따라 스모크 테스트 대기(A7/A8). 빌더/파서는 구현되었으나 tolerant/deferred 상태.
- [x] 예제 agent/flow YAML 동작 확인
- [x] 기존 MQTT 에이전트 테스트 회귀 없음 (전체 회귀 11개 패키지 0 FAIL)

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| 라이브 MQTT 브로커 | attributes/response 인코딩 및 v5 PUBACK 타이밍 스모크 검증 |
