# Acceptance Criteria — SPEC-HVACR-SYNC-001

> HVACR 에이전트 미러링/동기화 (게이트웨이 ↔ 서버) over MQTT
> 관련: [spec.md](./spec.md), [plan.md](./plan.md)
> 모든 기준은 기계적으로 검증 가능하다 (Go build/test -race, grep, 라운드트립 테스트).

## Definition of Done

- [ ] REQ-SYNC-001-01-XX ~ 08-XX 요구사항 구현 완료
- [ ] `go build ./...` 성공
- [ ] `go test -race ./internal/agent/samsung/...` 전부 green
- [ ] samsung 패키지 커버리지 ≥ 85%
- [ ] 기존 serial/tcp 경로 회귀 없음 (characterization 테스트 green)
- [ ] 신규 mirror transport 타입 등록 grep 확인
- [ ] 와이어 포맷 라운드트립 테스트 green
- [ ] deadlock 회피 경계 준수 (락 하 `a.Name()` 미호출)
- [ ] `cmd/xflowd/main.go` 배선 확인

---

## Module 1 — Ingress 추출 (behavior-preserving)

### AC-1.1: ingress 진입점 존재
```
Given samsung Hvacr01Agent 소스
When  ingress 메서드 존재를 grep 으로 확인
Then  ingestDecodedMessage (또는 동등 진입점) + (선택) ingestFrameBytes 가 정의되어 있다
```
검증:
```bash
grep -n "func (a \*Hvacr01Agent) ingestDecodedMessage\|func (a \*Hvacr01Agent) ingestFrameBytes" internal/agent/samsung/agent.go
```

### AC-1.2: receiveLoop 가 ingress 를 호출
```
Given 리팩터된 receiveLoop
When  receiveLoop 본문을 확인
Then  인라인 Decode→handleMessage 블록이 ingress 메서드 호출로 치환되어 있다
```
검증: `receiveLoop` 내부에서 `ingestFrameBytes`/`ingestDecodedMessage` 호출이 존재. (수동 코드리뷰 + grep)

### AC-1.3: 기존 serial/tcp 경로 행위 보존
```
Given 기존 samsung 테스트 스위트
When  go test -race 실행
Then  리팩터 전/후 모든 테스트가 동일하게 green (디코드 성공/실패 카운트, unsupported 인덱스 로그 억제, log_decode_errors 동작 불변)
```
검증:
```bash
go test -race ./internal/agent/samsung/...
```

### AC-1.4: deadlock 회피 (락 하 Name 미호출)
```
Given ingress/tap 코드 경로
When  a.mu.Lock 보유 구간을 확인
Then  해당 구간에서 a.Name() / a.ID() 호출이 없다 (필드 직접 읽기)
```
검증(휴리스틱 + 코드리뷰):
```bash
go test -race ./internal/agent/samsung/...   # deadlock 시 타임아웃/행
```

---

## Module 2 — 서버측 Mirror 입력

### AC-2.1: mirror transport 타입 등록
```
Given config transport 검증 switch + NewNasaTransport 팩토리
When  신규 mirror 타입 등록을 grep 으로 확인
Then  config.go switch 와 transport.go 팩토리 양쪽에 신규 타입 분기가 존재한다
```
검증:
```bash
grep -n "mirror" internal/agent/samsung/config.go internal/agent/samsung/transport.go
```
(설계 2b 채택 시: mirror 입력 소스 함수 존재를 대신 확인)

### AC-2.2: 미러 주입 시 서버 상태 수렴
```
Given mock MQTT 로 게이트웨이 업링크(디코드 메시지 wire) 재생
When  서버 mirror 에이전트가 해당 메시지들을 수신·주입
Then  서버측 GetDeviceState 결과가 게이트웨이가 동일 메시지로 도달한 상태와 일치한다
```
검증: 신규 테스트 `TestMirrorConverges` (mock transport/MQTT 로 프레임 재생 → 상태 비교).

### AC-2.3: mirror 필수 설정 누락 거부
```
Given mirror 입력 설정에서 브로커/토픽 누락
When  Init/config 파싱
Then  명시적 에러를 반환한다 (silent default 금지)
```
검증: 테이블 테스트 (누락 케이스별 에러).

---

## Module 3 — 게이트웨이 업링크 Tap

### AC-3.1: 디코드 성공 프레임만 업링크
```
Given 게이트웨이 에이전트 + 유효 프레임 1개 + CRC 불량 프레임 1개 주입
When  receiveLoop 처리
Then  publish 는 유효(디코드 성공) 프레임 1건만 발생하고, CRC 불량은 발행되지 않는다
```
검증: 신규 테스트 `TestUplinkTapPublishesDecodedOnly` (mock publisher 로 발행 호출 캡처).

### AC-3.2: tap 비활성 시 단독 동작 (행위 보존)
```
Given mirror_uplink_enabled=false
When  게이트웨이 에이전트 동작
Then  업링크 발행이 발생하지 않으며 기존 단독 에이전트와 동일하게 동작한다
```
검증: `TestUplinkTapDisabled`.

### AC-3.3: 발행 실패 격리
```
Given publisher 가 에러 반환하도록 설정
When  업링크 발행 실패
Then  로컬 상태 갱신/수신 루프는 중단되지 않고 계속된다 (에러 카운터 증가 + 로그)
```
검증: `TestUplinkPublishFailureIsolated`.

---

## Module 4 — 제어 역경로

### AC-4.1: 서버 제어 → downlink 발행 (로컬 미실행)
```
Given mirror 모드 서버 에이전트
When  Process({"command":"set_power","device_id":"living-room","params":{"power":true}}) 호출
Then  로컬 트랜스포트 Send 는 호출되지 않고, downlink 토픽으로 제어 wire 가 발행된다
```
검증: `TestServerControlPublishesDownlink` (로컬 Send 미호출 assert + publish 캡처).

### AC-4.2: 게이트웨이 downlink 구독 → Process 실행
```
Given 게이트웨이 에이전트 + downlink 토픽 제어 wire 수신
When  구독 콜백 처리
Then  wire 가 Process 입력 JSON 으로 매핑되어 Process 가 호출되고 RS485 제어가 수행된다
```
검증: `TestGatewayDownlinkInvokesProcess` (mock transport Send 호출 확인).

### AC-4.3: 업링크/다운링크 토픽 분리 (루프백 없음)
```
Given 게이트웨이+서버 왕복
When  제어 downlink 후 결과 상태 업링크
Then  업링크(.../up/nasa)와 다운링크(.../down/control)가 서로 다른 토픽이며 재-downlink 루프가 없다
```
검증: 토픽 상수/구독 대상 grep + 통합 테스트.

---

## Module 5 — 와이어 포맷

### AC-5.1: 디코드 메시지 라운드트립 무손실
```
Given 임의의 NasaMessage (여러 index 니블 크기: 1/2/4 byte + 가변)
When  wire 직렬화 → 역직렬화 → ingestDecodedMessage
Then  결과 디바이스 상태가 원본 메시지 직접 주입 결과와 동일하다 (ts 제외 무손실)
```
검증: property 테스트 `TestWireRoundTrip` — MessageSetValueSize 규칙 준수 확인 포함.

### AC-5.2: 타임스탬프 epoch millis
```
Given wire payload
When  ts 필드를 확인
Then  ts 는 int64 epoch milliseconds (UnixMilli) 이며 RFC3339/time.Time 문자열이 아니다
```
검증:
```bash
grep -n "UnixMilli" internal/agent/samsung/*.go
```

### AC-5.3: payload 에 gateway_id 없음
```
Given 업링크/다운링크 wire 스키마
When  wire 구조체 필드를 확인
Then  gateway_id 필드가 payload 에 존재하지 않는다 (토픽이 식별 전담)
```
검증: wire 구조체 정의 코드리뷰 + grep(`gateway_id` payload 부재).

---

## Module 6 — 토픽 스킴

### AC-6.1: 업링크/다운링크 토픽 분리 정의
```
Given 토픽 스킴 상수/빌더
When  토픽 생성 함수를 확인
Then  {prefix}/{gateway_id}/up/nasa (업링크) 와 {prefix}/{gateway_id}/down/control (다운링크) 가 분리 정의된다
```
검증: 토픽 빌더 단위 테스트 (gateway_id 치환 확인).

### AC-6.2: 서버 구독 대상 정확성
```
Given gateway_id="gw01" 서버 설정
When  서버 에이전트 구독
Then  .../gw01/up/nasa (및 활성 시 .../gw01/up/ack) 를 구독한다
```
검증: `TestServerSubscribesUplinkTopics`.

---

## Module 7 — 동기화 의미론

### AC-7.1: online/offline·discovery replay
```
Given 게이트웨이가 수신한 device online→offline 전이 + discovery notification 메시지 시퀀스
When  동일 시퀀스를 서버에 replay
Then  서버측에서 동일한 online/offline 전이와 device 등록(discovery)이 재현된다
```
검증: `TestReplayOnlineOfflineDiscovery`.

### AC-7.2: 서버 재시작 재동기화
```
Given 채택된 재동기화 메커니즘 (retain 스냅샷 7a 또는 resync 7b)
When  서버 에이전트 재시작/재구독
Then  게이트웨이의 현재 디바이스 상태를 재확보한다 (구독 즉시 스냅샷 수신 또는 resync 후 재emit 수신)
```
검증: `TestResyncOnRestart`.

### AC-7.3: QoS/retain 정책
```
Given 업링크/다운링크/스냅샷 발행
When  발행 옵션을 확인
Then  업링크 디코드 메시지 QoS≥1 retain=false, 다운링크 제어 QoS1 retain=false, (7a 채택 시)스냅샷 retain=true
```
검증: publish 호출 인자 캡처 테스트 (`retained` 플래그 assert).

### AC-7.4: MQTT 재연결 후 재동기화
```
Given MQTT 연결 끊김 후 복구
When  paho AutoReconnect 로 재연결
Then  mirror Available() 가 연결 상태를 반영하고, 재연결 후 재동기화(AC-7.2)가 수행된다
```
검증: `TestReconnectResync` (연결 상태 토글 시뮬레이션).

---

## Module 8 — 경계/스코프 가드

### AC-8.1: 1차 samsung 한정
```
Given 본 SPEC 변경 범위
When  변경 파일 경로를 확인
Then  구현은 internal/agent/samsung/ (+ 등록 배선) 에 한정되며 lg/century 에이전트 로직은 변경되지 않는다
```
검증:
```bash
git diff --name-only main | grep -E "internal/agent/(lg|century)/" && echo "OUT-OF-SCOPE CHANGE" || echo "OK"
```

### AC-8.2: 기존 입력 경로 무변경
```
Given serial/tcp-client/tcp-server 경로
When  전체 samsung 테스트 실행
Then  기존 경로 테스트가 전부 green (행위 보존)
```
검증:
```bash
go test -race ./internal/agent/samsung/...
```

### AC-8.3: 등록 배선
```
Given 신규 mirror transport 타입 (설계 2a)
When  cmd/xflowd/main.go 및 팩토리 확인
Then  신규 타입이 config switch + NewNasaTransport + (필요 시)main.go 에 배선되어 런타임 활성화된다
```
검증:
```bash
grep -n "mirror" internal/agent/samsung/config.go internal/agent/samsung/transport.go
grep -n "RegisterSamsungHvacr01Types" cmd/xflowd/main.go
```

### AC-8.4: thingplus 독립
```
Given 미러링 데이터 경로
When  import/의존 확인
Then  thingplus/ThingsBoard 전용 패키지에 의존하지 않고 범용 MQTT(MessagePublisher/SubscriberAgent)만 사용한다
```
검증: import 그래프 코드리뷰 + grep.

---

## 통합 시나리오 (End-to-End)

### AC-E2E-1: 게이트웨이 → 서버 상태 미러링
```
Given 게이트웨이(로컬 mock transport) + 서버(mirror 입력), 공통 mock MQTT
When  게이트웨이에 실내기 상태 프레임(전원/모드/온도) 시퀀스 주입
Then  서버측 get_all 결과 디바이스 상태가 게이트웨이와 동일하게 수렴한다
```

### AC-E2E-2: 서버 제어 → 게이트웨이 실행 대칭
```
Given AC-E2E-1 구성
When  서버 Process 로 set_multiple(power/mode/target_temp) 발행
Then  다운링크를 통해 게이트웨이 Process 가 호출되고, 게이트웨이 로컬 transport 로 C013 제어 프레임이 Send 된다
```
검증: `TestE2EMirrorAndControl` (양방향 mock 배선, `-race`).
