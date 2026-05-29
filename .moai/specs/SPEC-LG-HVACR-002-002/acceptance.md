# SPEC-LG-HVACR-002-002: 수락 기준

---
id: SPEC-LG-HVACR-002-002
document: acceptance
version: "1.0.0"
---

> **명명 규약 (v2.0 rename, 2026-05-29 이후)**: 프로토콜 `lg_icp02` (LG ICP-02) / 에이전트 `lg_hvacr02` (LG HVACR-02). 이전 SPEC ID: SPEC-LGCP-002.

## 1. 프레임 빌더 (M1)

### AC-001: 제어 프레임 빌드 기본 구조

```gherkin
Given LG ICP-02 프레임 빌더가 초기화되어 있다
When DA="44550065", SA="44550000", CMD=0x0201, 페이로드=[18 41 18 80 29 C0]으로 프레임을 빌드한다
Then 프레임은 STX=0x56으로 시작한다
And LEN 필드는 프레임 전체 길이 - 1 (STX 제외)이다
And DLEN=0x04, DA=44550065, SLEN=0x04, SA=44550000이다
And CMD=0201이다
And 페이로드 앞에 PLEN=0x06이 있다
And 프레임 끝 2바이트는 CRC-16/XMODEM (Big-Endian)이다
And VerifyIcp02CRC(frame) == true이다
```

### AC-002: CRC 교차 검증

```gherkin
Given 프레임 빌더로 생성된 제어 프레임이 있다
When 기존 VerifyIcp02CRC() 함수로 CRC를 검증한다
Then CRC가 유효하다 (true 반환)
```

### AC-003: 시퀀스 번호 관리

```gherkin
Given Hvacr02SequenceManager가 초기화되어 있다
When CMD=0x0201로 NextSEQ0을 3번 호출한다
Then SEQ0 값은 0x00, 0x01, 0x02 순서로 증가한다

When CMD=0x0604로 NextSEQ0을 호출한다
Then CMD=0x0604의 SEQ0은 0x00이다 (CMD별 독립 카운터)

When NextSEQ1을 5번 호출한다
Then SEQ1 값은 연속으로 증가한다 (전역 카운터)
```

### AC-004: 시퀀스 번호 순환

```gherkin
Given SEQ1 카운터가 0xFF인 상태에서
When NextSEQ1()을 호출하면
Then SEQ1 값은 0x00으로 순환한다
```

### AC-005: Transport Write 메서드

```gherkin
Given LGAPTransport 인터페이스를 구현하는 시리얼 트랜스포트가 있다
When Write(data) 메서드를 호출한다
Then 시리얼 포트로 data 바이트가 전송된다
And 전송된 바이트 수가 반환된다
```

### AC-006: 에코 필터링

```gherkin
Given LG HVACR-02 에이전트가 제어 프레임 [56 12 ...] 을 전송했다
When captureLoop에서 동일한 바이트 [56 12 ...] 을 에코 윈도우 내에 수신한다
Then 해당 프레임은 에코로 판별되어 무시된다
And framesCaptured 카운터에 반영되지 않는다
And 디버그 로그에 "echo frame filtered" 메시지가 기록된다
```

### AC-007: 에코 윈도우 만료

```gherkin
Given LG HVACR-02 에이전트가 제어 프레임을 전송한 지 에코 윈도우가 경과했다
When captureLoop에서 동일 바이트의 프레임을 수신한다
Then 정상 프레임으로 처리된다 (에코 필터 미적용)
```

---

## 2. 제어 명령 (M2)

### AC-010: 전원 ON 명령

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
And 대상 실내기 주소가 "44550065"이다
When Process({"command":"set_power", "address":"44550065", "params":{"power":true}})를 호출한다
Then 시리얼 포트로 다음 구조의 프레임이 전송된다:
  - STX=0x56
  - DA=44550065
  - SA=44550000
  - CMD=0201
  - 페이로드에 [18 41] [18 80] [29 C0] 포함
And 응답 JSON에 "status":"ok"가 포함된다
```

### AC-011: 전원 OFF 명령

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"set_power", "address":"44550065", "params":{"power":false}})를 호출한다
Then 전송된 프레임의 페이로드에 [18 40] [18 80] [29 C0]이 포함된다
```

### AC-012: 전원 ON + 압축기 용량

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"set_power", "address":"44550065", "params":{"power":true, "compressor_capacity":10}})를 호출한다
Then 전송된 프레임의 페이로드에 [18 41] [18 8A] [29 C0]이 포함된다
  (8A = 0x80 | 10)
```

### AC-013: 온도 설정

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"target_temperature", "address":"44550065", "params":{"temperature":25.0}})를 호출한다
Then 전송된 프레임의 페이로드에 [64 8A]가 포함된다
  (8A = 0x80 | (25 - 15) = 0x80 | 0x0A)
```

### AC-014: 온도 범위 초과

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"target_temperature", "address":"44550065", "params":{"temperature":31.0}})를 호출한다
Then 에러가 반환된다: "temperature out of range: 31, must be 15-30"
And 시리얼 포트로 프레임이 전송되지 않는다
```

### AC-015: 온도 최소값

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"target_temperature", "address":"44550065", "params":{"temperature":15.0}})를 호출한다
Then 전송된 프레임의 페이로드에 [64 80]가 포함된다
  (80 = 0x80 | 0 = 0x80 | (15-15))
```

### AC-016: 온도 최대값

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"target_temperature", "address":"44550065", "params":{"temperature":30.0}})를 호출한다
Then 전송된 프레임의 페이로드에 [64 8F]가 포함된다
  (8F = 0x80 | 15 = 0x80 | (30-15))
```

### AC-017: 풍량 설정

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
And 대상 실내기의 현재 모드가 "cooling" (코드 0)이다
When Process({"command":"set_fan_speed", "address":"44550065", "params":{"fan_speed":"high"}})를 호출한다
Then 전송된 프레임의 페이로드에 [64 50 30]이 포함된다
  (30 = high(3) << 4 | cooling(0))
```

### AC-018: 모드 설정

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
And 대상 실내기의 현재 풍량이 "auto" (코드 5)이다
When Process({"command":"set_mode", "address":"44550065", "params":{"mode":"heating"}})를 호출한다
Then 전송된 프레임의 페이로드에 [64 50 54]가 포함된다
  (54 = auto(5) << 4 | heating(4))
```

### AC-019: 현재 상태 미확인 시 기본값

```gherkin
Given 대상 실내기의 현재 상태가 없다 (아직 캡처된 적 없음)
When set_fan_speed 명령을 호출한다
Then 모드 기본값 0 (냉방)을 사용한다

When set_mode 명령을 호출한다
Then 풍량 기본값 5 (자동)을 사용한다
```

### AC-020: 복합 제어 (set_multiple)

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"set_multiple", "address":"44550065", "params":{"power":true, "temperature":24, "fan_speed":"medium", "mode":"cooling"}})를 호출한다
Then 단일 0x0201 프레임이 전송된다
And 페이로드에 전원 ON + 온도 24도 + 풍량 medium/모드 cooling 레지스터가 모두 포함된다
```

### AC-021: 잘못된 풍량 값

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"set_fan_speed", "address":"44550065", "params":{"fan_speed":"supersonic"}})를 호출한다
Then 에러가 반환된다: "invalid fan_speed: supersonic"
And 시리얼 포트로 프레임이 전송되지 않는다
```

### AC-022: 잘못된 모드 값

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"set_mode", "address":"44550065", "params":{"mode":"turbo"}})를 호출한다
Then 에러가 반환된다: "invalid mode: turbo"
```

---

## 3. 디바이스 제어 통합 (M3)

### AC-030: ControllableDevice 반환 (indoor)

```gherkin
Given LG HVACR-02 에이전트에 indoor 타입 디바이스 "44550065"가 등록되어 있다
And control_enabled=true이다
When Hvacr02DeviceProvider.Devices()를 호출한다
Then "44550065" 디바이스는 ControllableDevice 인터페이스를 구현한다
And Commands()는 5개 명령(set_power, target_temperature, set_fan_speed, set_mode, set_multiple)을 반환한다
```

### AC-031: 읽기 전용 디바이스 (controller)

```gherkin
Given LG HVACR-02 에이전트에 controller 타입 디바이스 "44550000"가 등록되어 있다
When Hvacr02DeviceProvider.Devices()를 호출한다
Then "44550000" 디바이스는 Device 인터페이스를 구현한다 (ControllableDevice가 아님)
```

### AC-032: CommandExecutor 체인

```gherkin
Given indoor 디바이스 "44550065"의 ControllableDevice가 있다
When Execute(ctx, "set_power", {"power":true})를 호출한다
Then Hvacr02Agent.Process()가 올바른 JSON으로 호출된다
And 시리얼 포트로 전원 ON 프레임이 전송된다
```

### AC-033: CommandSpec 파라미터 검증

```gherkin
Given target_temperature 명령의 CommandSpec이 있다
Then ParamSpec "temperature"는 Type="float", Required=true, Min=15, Max=30이다

Given set_fan_speed 명령의 CommandSpec이 있다
Then ParamSpec "fan_speed"는 Type="string", Required=true, Options=["low","medium","high","turbo","auto"]이다
```

---

## 4. 상태 확인 (M4)

### AC-040: 제어 후 상태 변경 확인 성공

```gherkin
Given LG HVACR-02 에이전트가 실행 중이고 captureLoop가 프레임을 수신 중이다
And 디바이스 "44550065"의 현재 전원 상태가 OFF이다
When set_power(power=true) 명령을 전송한다
And 3초 내에 captureLoop가 "44550065"의 전원 ON 상태 프레임을 수신한다
Then Process() 응답에 "verified":true가 포함된다
```

### AC-041: 제어 후 상태 확인 타임아웃

```gherkin
Given LG HVACR-02 에이전트가 실행 중이다
And control_verify_timeout이 1초로 설정되어 있다
When target_temperature(temperature=25) 명령을 전송한다
And 1초 내에 상태 변경 프레임이 수신되지 않는다
Then Process() 응답에 "status":"ok", "verified":false, "message":"command sent, verification timeout"가 포함된다
And 에러는 반환되지 않는다 (경고 수준)
```

---

## 5. 하위 호환성 (M6)

### AC-050: control_enabled=false 시 제어 명령 거부

```gherkin
Given control_enabled=false인 LG HVACR-02 에이전트가 실행 중이다 (기본값)
When Process({"command":"set_power", "address":"44550065", "params":{"power":true}})를 호출한다
Then 에러가 반환된다: "control not enabled for this agent"
And 시리얼 포트로 프레임이 전송되지 않는다
```

### AC-051: control_enabled=false 시 기존 명령 정상 동작

```gherkin
Given control_enabled=false인 LG HVACR-02 에이전트가 실행 중이다
When Process({"command":"get_stats"})를 호출한다
Then 통계 정보가 정상적으로 반환된다

When Process({"command":"get_recent"})를 호출한다
Then 최근 프레임 목록이 정상적으로 반환된다
```

### AC-052: capabilities 목록 업데이트

```gherkin
Given control_enabled=false인 LG HVACR-02 에이전트가 있다
When capabilities를 조회한다
Then ["passive-monitor"]가 반환된다

Given control_enabled=true인 LG HVACR-02 에이전트가 있다
When capabilities를 조회한다
Then ["passive-monitor", "active-control"]이 반환된다
```

### AC-053: 패시브 캡처 영향 없음

```gherkin
Given control_enabled=true인 LG HVACR-02 에이전트가 실행 중이다
And captureLoop가 프레임을 캡처 중이다
When 제어 명령을 전송한다
Then captureLoop는 중단 없이 계속 프레임을 캡처한다
And 에코 프레임만 필터링될 뿐 다른 프레임은 정상 처리된다
And framesCaptured 통계는 에코를 제외한 정상 프레임만 카운트한다
```

---

## 6. 에러 처리

### AC-060: 존재하지 않는 주소

```gherkin
Given LG HVACR-02 에이전트에 "44550099" 디바이스가 등록되어 있지 않다
When set_power 명령에 address="44550099"를 지정한다
Then 프레임은 전송된다 (주소 검증은 프로토콜 레벨에서 수행)
And 상태 확인에서 타임아웃이 발생한다
```

### AC-061: 시리얼 전송 실패

```gherkin
Given 시리얼 포트가 연결 해제된 상태이다
When 제어 명령을 전송한다
Then 에러가 반환된다: "serial write failed: ..."
And 재연결 로직이 트리거된다
```

### AC-062: 잘못된 주소 형식

```gherkin
Given Process({"command":"set_power", "address":"ZZZZ", "params":{"power":true}})를 호출한다
Then 에러가 반환된다: "invalid address format: ZZZZ"
```

### AC-063: 필수 파라미터 누락

```gherkin
Given Process({"command":"target_temperature", "address":"44550065", "params":{}})를 호출한다
Then 에러가 반환된다: "missing required parameter: temperature"
```

---

## 7. 품질 게이트

### Definition of Done

- [ ] 모든 AC (수락 기준) 시나리오에 대한 테스트가 통과한다
- [ ] 신규 파일 테스트 커버리지 85% 이상
- [ ] 기존 LG HVACR-02 테스트가 모두 통과한다 (회귀 없음)
- [ ] `go vet ./internal/agent/lg/...` 경고 없음
- [ ] `go test -race ./internal/agent/lg/...` 통과
- [ ] 프레임 빌더 golden test로 실제 캡처 프레임과 CRC 교차 검증 완료
- [ ] control_enabled=false 시 기존 동작 100% 호환 확인
