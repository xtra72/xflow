---
id: SPEC-MODBUS-001
type: acceptance
version: "1.0.0"
created: "2026-02-26"
updated: "2026-02-26"
author: xtra
---

# SPEC-MODBUS-001 인수 기준: MODBUS/TCP Client Agent 구현

## 1. Module 1: Connection Management (연결 관리)

### AC-01: Init 시 TCP 연결 수립

```gherkin
Given ModbusConfig에 device_id "plc-01", host "192.168.1.100", port 502, unit_id 1이 설정된 경우
When MODBUSAgent.Init(config)이 호출되면
Then 각 디바이스에 대해 TCP 소켓 연결이 시도되어야 한다
And 연결 성공 시 해당 디바이스의 online 상태가 true로 설정되어야 한다
And deviceCaches 맵에 unit_id별 RegisterCache가 초기 생성되어야 한다
And msgCh 채널이 msg_channel_size(기본값: 256) 버퍼 크기로 생성되어야 한다
```

### AC-02: 연결 끊김 시 자동 재연결

```gherkin
Given MODBUSAgent가 Running 상태이고 device "plc-01"이 연결된 경우
When TCP 연결이 끊어지면
Then reconnect_interval(기본값: 10초) 간격으로 자동 재연결을 시도해야 한다
And 재연결 성공 시 online 상태가 true로 복원되어야 한다
And 재연결 성공 시 Health 상태가 Healthy로 복원되어야 한다
And 재연결 시도마다 slog에 device_id와 함께 로그가 기록되어야 한다
```

### AC-03: 연결 실패 시 Health Degraded

```gherkin
Given MODBUSAgent가 Running 상태인 경우
When 디바이스 "plc-01"에 대한 TCP 연결 시도가 실패하면
Then Health 상태가 Degraded로 보고되어야 한다
And 해당 디바이스의 online 상태가 false로 설정되어야 한다
And 폴링이 해당 디바이스에 대해 일시 중단되어야 한다
```

### AC-04: 최대 재연결 시도 초과 시 중지 (무한 루프 방지)

```gherkin
Given max_reconnect_attempts가 10으로 설정된 경우
When 디바이스 "plc-01"에 대한 재연결 시도가 10회 연속 실패하면
Then 해당 디바이스의 재연결을 중지해야 한다
And ErrMaxReconnectExceeded 에러를 로그에 기록해야 한다
And Health 상태를 Unhealthy로 설정해야 한다
And 다른 디바이스의 폴링에는 영향이 없어야 한다
```

### AC-05: 다중 디바이스 연결 독립성

```gherkin
Given 디바이스 "plc-01"(192.168.1.100:502)과 "sensor-01"(192.168.1.101:502)이 설정된 경우
When "plc-01"의 TCP 연결이 실패하면
Then "sensor-01"의 폴링은 정상 계속되어야 한다
And "plc-01"의 에러가 "sensor-01"의 RegisterCache에 영향을 주지 않아야 한다
And Health 상태는 Degraded로 보고되어야 한다 (전체 Unhealthy가 아님)
```

---

## 2. Module 2: Register Reading (레지스터 읽기)

### AC-06: FC03 Holding Register 읽기

```gherkin
Given 디바이스 "plc-01"에 function_code 3, start_address 0, quantity 10이 설정된 경우
When 폴링 주기가 도래하면
Then FC03(0x03) Read Holding Registers 요청이 전송되어야 한다
And MBAP Header에 올바른 Transaction ID, Protocol ID(0x0000), Length, Unit ID가 포함되어야 한다
And PDU에 시작 주소(0x0000)와 수량(0x000A)이 Big-Endian으로 인코딩되어야 한다
And 응답에서 10개의 16-bit 레지스터 값이 파싱되어야 한다
```

### AC-07: FC01 Coil 읽기

```gherkin
Given 디바이스 "plc-01"에 function_code 1, start_address 0, quantity 16이 설정된 경우
When 폴링 주기가 도래하면
Then FC01(0x01) Read Coils 요청이 전송되어야 한다
And 응답에서 16개의 bit 값이 파싱되어야 한다
And 각 coil 값이 bool 타입으로 RegisterCache.Coils에 저장되어야 한다
```

### AC-08: FC02 Discrete Input 읽기

```gherkin
Given 디바이스 "plc-01"에 function_code 2, start_address 0, quantity 8이 설정된 경우
When 폴링 주기가 도래하면
Then FC02(0x02) Read Discrete Inputs 요청이 전송되어야 한다
And 응답에서 8개의 bit 값이 파싱되어야 한다
And 각 값이 bool 타입으로 RegisterCache.DiscreteInputs에 저장되어야 한다
```

### AC-09: FC04 Input Register 읽기

```gherkin
Given 디바이스 "plc-01"에 function_code 4, start_address 0, quantity 5가 설정된 경우
When 폴링 주기가 도래하면
Then FC04(0x04) Read Input Registers 요청이 전송되어야 한다
And 응답에서 5개의 16-bit 레지스터 값이 파싱되어야 한다
And 각 값이 uint16 타입으로 RegisterCache.InputRegisters에 저장되어야 한다
```

### AC-10: MODBUS Exception 응답 처리 (읽기)

```gherkin
Given 디바이스 "plc-01"에 FC03 읽기 요청이 전송된 경우
When MODBUS Exception 응답(Function Code MSB=1, Exception Code=0x02)이 수신되면
Then Exception Code "Illegal Data Address (0x02)"를 로그에 기록해야 한다
And 해당 디바이스/레지스터 그룹의 에러 카운터를 증가시켜야 한다
And 다음 레지스터 그룹의 폴링을 계속해야 한다
And RegisterCache는 갱신하지 않아야 한다
```

---

## 3. Module 3: Read Mode (읽기 모드)

### AC-11: Cached 모드 주기적 폴링 및 캐시 반환

```gherkin
Given read_mode가 "cached"이고 poll_interval이 "5s"인 경우
When 5초 간격으로 폴링 주기가 도래하면
Then 모든 디바이스/레지스터 그룹에 대해 MODBUS 읽기 요청이 전송되어야 한다
And 응답 값이 RegisterCache에 저장되어야 한다

When read_registers 명령이 Process()를 통해 수신되면
Then 디바이스에 직접 쿼리하지 않고 RegisterCache의 캐시된 값을 반환해야 한다
```

### AC-12: Direct 모드 직접 읽기

```gherkin
Given read_mode가 "direct"인 경우
When read_registers 명령이 Process()를 통해 수신되면
Then 디바이스에 직접 MODBUS 읽기 요청을 전송해야 한다
And 응답 결과를 즉시 반환해야 한다
And RegisterCache를 갱신하지 않아야 한다
```

### AC-13: Cached 모드 force=true 강제 읽기

```gherkin
Given read_mode가 "cached"인 경우
When force: true 파라미터가 포함된 read_registers 명령이 수신되면
Then 캐시를 무시하고 디바이스에 직접 쿼리해야 한다
And 쿼리 결과를 RegisterCache에 갱신해야 한다
And 쿼리 결과를 반환해야 한다
```

### AC-14: Direct 모드에서 주기적 폴링 미실행

```gherkin
Given read_mode가 "direct"인 경우
When MODBUSAgent.Start()가 호출되면
Then pollTicker가 시작되지 않아야 한다
And 주기적인 MODBUS 읽기 요청이 발생하지 않아야 한다
And msgCh에 폴링 기반 메시지가 전달되지 않아야 한다
```

### AC-15: 기본 read_mode는 "cached"

```gherkin
Given Transport.Options에 read_mode가 지정되지 않은 경우
When parseModbusConfig(opts)가 호출되면
Then config.ReadMode가 "cached"로 설정되어야 한다
```

---

## 4. Module 5: Interval Mode (인터벌 모드)

### AC-16: Cached + Interval 모드 전체 데이터 전송

```gherkin
Given mode가 "interval"이고 read_mode가 "cached"인 경우
When 폴링 주기가 도래하여 레지스터 데이터가 읽기 완료되면
Then 수집된 전체 레지스터 데이터를 JSON Message로 변환해야 한다
And 메시지 type이 "register_data"이어야 한다
And 메시지에 device_id, unit_id, mode("interval"), registers, timestamp가 포함되어야 한다
And 변환된 메시지를 msgCh에 즉시 전달해야 한다
```

### AC-17: Direct 모드에서 Interval 모드 미적용

```gherkin
Given mode가 "interval"이고 read_mode가 "direct"인 경우
When MODBUSAgent.Start()가 호출되면
Then 주기적 폴링이 시작되지 않아야 한다
And msgCh에 interval 기반 메시지가 전달되지 않아야 한다
```

---

## 5. Module 6: Event Mode (이벤트 모드)

### AC-18: Cached + Event 모드 변경 감지 전송

```gherkin
Given mode가 "event"이고 read_mode가 "cached"인 경우
When 폴링에서 Holding Register 주소 0의 값이 1234에서 1235로 변경되면
Then 변경된 레지스터 데이터만 JSON Message로 변환해야 한다
And 메시지 type이 "register_changed"이어야 한다
And changed_registers에 해당 레지스터의 old 값(1234)과 new 값(1235)이 포함되어야 한다
And 변경된 메시지를 msgCh에 전달해야 한다
```

### AC-19: Event 모드 Heartbeat 전체 상태 전송

```gherkin
Given mode가 "event"이고 heartbeat_interval이 "60s"인 경우
When heartbeat 타이머가 만료되면
Then 변경 여부와 무관하게 전체 레지스터 상태를 msgCh에 전달해야 한다
And 메시지 type이 "register_data"이어야 한다
And mode가 "event"로 표시되어야 한다
```

### AC-20: Event 모드 RegisterCache 기반 비교

```gherkin
Given mode가 "event"이고 RegisterCache에 이전 값이 저장된 경우
When 새로운 폴링 결과가 수신되면
Then RegisterCache에 저장된 이전 값과 현재 값을 비교해야 한다
And 변경된 레지스터만 식별해야 한다
And 비교 완료 후 RegisterCache를 새로운 값으로 갱신해야 한다
```

### AC-21: 값 미변경 시 이벤트 미전송

```gherkin
Given mode가 "event"이고 RegisterCache에 이전 값이 저장된 경우
When 폴링 결과가 이전 캐시와 동일하면
Then msgCh에 메시지를 전달하지 않아야 한다
And RegisterCache의 LastUpdateTime만 갱신해야 한다
```

### AC-22: Direct 모드에서 Event 모드 미적용

```gherkin
Given mode가 "event"이고 read_mode가 "direct"인 경우
When MODBUSAgent.Start()가 호출되면
Then 주기적 폴링이 시작되지 않아야 한다
And 변경 감지 로직이 실행되지 않아야 한다
And msgCh에 event 기반 메시지가 전달되지 않아야 한다
```

---

## 6. Module 7: Bridge Integration (브릿지 통합)

### AC-23: BridgeIn 폴링 데이터 수신

```gherkin
Given MODBUSAgent가 MessageReceiver 인터페이스를 구현한 경우
When ReceiveMessage(ctx)가 호출되면
Then msgCh 채널과 ctx.Done() 채널을 select로 대기해야 한다
And msgCh에 메시지가 있으면 JSON 바이트를 반환해야 한다
And ctx가 취소되면 context.Canceled 에러를 반환해야 한다
```

### AC-24: BridgeOut 쓰기 명령 수신

```gherkin
Given Bridge를 통해 Process(data []byte)가 호출된 경우
When data가 {"command":"write_register","device_id":"plc-01","params":{"address":40001,"value":1234}} 형식이면
Then JSON을 파싱하여 command 필드를 식별해야 한다
And 해당 device_id의 디바이스에 FC06 쓰기 요청을 전송해야 한다
And 결과를 JSON 응답으로 반환해야 한다
```

### AC-25: BridgeInOut 양방향 동작

```gherkin
Given Bridge 방향이 BridgeInOut으로 설정된 경우
When MODBUSAgent가 동작 중이면
Then ReceiveMessage()를 통한 데이터 수신(BridgeIn)이 가능해야 한다
And Process()를 통한 명령 전송(BridgeOut)이 동시에 가능해야 한다
And 양방향 동작이 서로 간섭하지 않아야 한다
```

### AC-26: Message Metadata 포함

```gherkin
Given 폴링 또는 이벤트로 메시지가 생성된 경우
When 메시지가 msgCh에 전달되면
Then Metadata에 device_id(디바이스 식별자)가 포함되어야 한다
And Metadata에 unit_id(MODBUS Unit ID)가 포함되어야 한다
And Metadata에 timestamp(ISO 8601 형식)가 포함되어야 한다
And Metadata에 mode("interval" 또는 "event")가 포함되어야 한다
```

### AC-27: Process() 명령 라우팅

```gherkin
Given MODBUSAgent가 Running 상태인 경우
When Process({"command":"write_register",...})가 호출되면
Then 쓰기 명령 핸들러로 라우팅되어야 한다

When Process({"command":"read_registers",...})가 호출되면
Then 읽기 명령 핸들러로 라우팅되어야 한다

When Process({"command":"get_cache",...})가 호출되면
Then 캐시 조회 핸들러로 라우팅되어야 한다

When Process({"command":"unknown_cmd",...})가 호출되면
Then ErrInvalidCommand 에러가 반환되어야 한다
```

---

## 7. Module 8: Lifecycle (라이프사이클)

### AC-28: Pause 시 폴링 중지, TCP 연결 유지

```gherkin
Given MODBUSAgent가 Running 상태이고 폴링이 진행 중인 경우
When Pause(ctx)가 호출되면
Then 폴링 고루틴이 일시 중지되어야 한다
And TCP 연결은 유지되어야 한다 (연결을 닫지 않음)
And 에이전트 상태가 Paused로 전이되어야 한다
And msgCh에 새로운 폴링 메시지가 전달되지 않아야 한다
```

### AC-29: Resume 시 폴링 재개

```gherkin
Given MODBUSAgent가 Paused 상태인 경우
When Resume(ctx)가 호출되면
Then 폴링 고루틴이 재개되어야 한다
And 에이전트 상태가 Running으로 전이되어야 한다
And 다음 poll_interval에 폴링이 실행되어야 한다
```

### AC-30: StatefulAgent.State() 캐시 상태 반환

```gherkin
Given MODBUSAgent가 Running 상태이고 RegisterCache에 데이터가 있는 경우
When State()가 호출되면
Then map[string]any 형태로 다음 정보가 반환되어야 한다:
  - device_count: 관리 중인 디바이스 수
  - read_mode: 현재 읽기 모드 ("cached" 또는 "direct")
  - mode: 현재 동작 모드 ("interval" 또는 "event")
  - devices: 디바이스별 상태 (unit_id, host, port, online, register_groups)
And register_groups에 function_code, start_address, quantity, last_update, stale 정보가 포함되어야 한다
```

---

## 8. Module 9: Register Write (레지스터 쓰기)

### AC-31: FC05 Write Single Coil

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When Process({"command":"write_coil","device_id":"plc-01","params":{"address":1,"value":true}})가 호출되면
Then FC05(0x05) Write Single Coil 요청이 전송되어야 한다
And 값 true는 0xFF00으로, false는 0x0000으로 인코딩되어야 한다
And 성공 시 {"status":"ok","command":"write_coil",...} JSON 응답이 반환되어야 한다
```

### AC-32: FC06 Write Single Register

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When Process({"command":"write_register","device_id":"plc-01","params":{"address":40001,"value":1234}})가 호출되면
Then FC06(0x06) Write Single Register 요청이 전송되어야 한다
And 값 1234가 Big-Endian 2 bytes로 인코딩되어야 한다
And 성공 시 {"status":"ok","command":"write_register",...} JSON 응답이 반환되어야 한다
```

### AC-33: FC15 Write Multiple Coils

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When Process({"command":"write_coils","device_id":"plc-01","params":{"address":1,"values":[true,false,true,true]}})가 호출되면
Then FC15(0x0F) Write Multiple Coils 요청이 전송되어야 한다
And 4개의 coil 값이 비트 배열로 인코딩되어야 한다
And 성공 시 {"status":"ok","command":"write_coils","quantity":4,...} JSON 응답이 반환되어야 한다
```

### AC-34: FC16 Write Multiple Registers

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When Process({"command":"write_registers","device_id":"plc-01","params":{"address":40001,"values":[1234,5678,9012]}})가 호출되면
Then FC16(0x10) Write Multiple Registers 요청이 전송되어야 한다
And 3개의 값이 각각 Big-Endian 2 bytes로 인코딩되어야 한다
And 성공 시 {"status":"ok","command":"write_registers","quantity":3,...} JSON 응답이 반환되어야 한다
```

### AC-35: 읽기 전용 레지스터 쓰기 거부

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When FC02(Discrete Input) 또는 FC04(Input Register) 영역에 대한 쓰기 명령이 수신되면
Then ErrReadOnlyRegister 에러가 반환되어야 한다
And MODBUS 요청이 디바이스에 전송되지 않아야 한다
And errors.Is(err, ErrReadOnlyRegister)가 true를 반환해야 한다
```

### AC-36: 오프라인 디바이스 쓰기 거부

```gherkin
Given 디바이스 "plc-01"이 오프라인(online=false) 상태인 경우
When Process({"command":"write_register","device_id":"plc-01","params":{"address":40001,"value":1234}})가 호출되면
Then ErrDeviceOffline 에러가 반환되어야 한다
And MODBUS 요청이 디바이스에 전송되지 않아야 한다
```

### AC-37: 쓰기 MODBUS Exception 응답 처리

```gherkin
Given 디바이스 "plc-01"에 FC06 쓰기 요청이 전송된 경우
When MODBUS Exception 응답(Exception Code=0x02)이 수신되면
Then {"status":"error","exception_code":2,"error":"modbus exception: illegal data address (0x02)",...} 응답이 반환되어야 한다
And ErrModbusException 에러를 로그에 기록해야 한다
And RegisterCache는 갱신하지 않아야 한다
```

### AC-38: 수량 제한 초과 거부

```gherkin
Given 디바이스 "plc-01"이 온라인 상태인 경우
When FC15 write_coils 명령의 values 길이가 1968을 초과하면
Then ErrQuantityExceeded 에러가 반환되어야 한다
And MODBUS 요청이 디바이스에 전송되지 않아야 한다

When FC16 write_registers 명령의 values 길이가 123을 초과하면
Then ErrQuantityExceeded 에러가 반환되어야 한다
And MODBUS 요청이 디바이스에 전송되지 않아야 한다
```

### AC-39: Write-Through 캐시 갱신 (성공 후에만)

```gherkin
Given 디바이스 "plc-01"이 온라인 상태이고 RegisterCache에 기존 값이 있는 경우
When FC06 쓰기 요청이 성공 응답을 수신하면
Then RegisterCache의 해당 레지스터를 새로운 값으로 갱신해야 한다
And LastUpdateTime을 현재 시각으로 갱신해야 한다
```

### AC-40: 쓰기 실패 시 캐시 미변경

```gherkin
Given 디바이스 "plc-01"이 온라인 상태이고 RegisterCache에 holding_register[0]=1234가 있는 경우
When FC06 쓰기 요청이 실패(타임아웃 또는 Exception)하면
Then RegisterCache의 holding_register[0]는 1234로 유지되어야 한다
And 롤백 로직이 불필요해야 한다 (캐시가 변경되지 않았으므로)
```

---

## 9. Module 10: Register Synchronization (레지스터 동기화)

### AC-41: Init 시 캐시 초기화 (비어있음, 모든 그룹 stale)

```gherkin
Given ModbusConfig에 디바이스 "plc-01"이 설정된 경우
When MODBUSAgent.Init(config)이 호출되면
Then deviceCaches에 해당 unit_id의 RegisterCache가 생성되어야 한다
And RegisterCache의 Coils, DiscreteInputs, HoldingRegisters, InputRegisters 맵이 비어있어야 한다
And 모든 레지스터 그룹의 stale 상태가 true이어야 한다 (LastUpdateTime이 zero value)
```

### AC-42: 폴링 성공 시 캐시 갱신

```gherkin
Given MODBUSAgent가 Running 상태이고 폴링이 진행 중인 경우
When FC03 읽기 응답이 성공적으로 수신되면
Then RegisterCache.HoldingRegisters가 응답 값으로 갱신되어야 한다
And RegisterCache.LastUpdateTime["FC03_0"]이 현재 시각으로 갱신되어야 한다
```

### AC-43: StatefulAgent.State() 캐시 상태 노출

```gherkin
Given RegisterCache에 데이터가 저장된 경우
When State()가 호출되면
Then 반환값에 devices 키가 포함되어야 한다
And 각 디바이스의 register_groups에 다음 정보가 포함되어야 한다:
  - function_code: MODBUS Function Code
  - start_address: 시작 주소
  - quantity: 수량
  - last_update: ISO 8601 형식 마지막 갱신 시각
  - stale: boolean 상태
```

### AC-44: Staleness 감지

```gherkin
Given stale_threshold가 poll_interval * 3 (기본값: 15초)으로 설정된 경우
When 특정 레지스터 그룹의 LastUpdateTime + stale_threshold < 현재 시각이면
Then 해당 레지스터 그룹의 stale 상태가 true로 표시되어야 한다
And register_group_stale 타입의 경고 메시지가 msgCh에 전달되어야 한다
```

### AC-45: Stale 복구

```gherkin
Given 레지스터 그룹이 stale=true 상태인 경우
When 해당 레지스터 그룹의 읽기가 성공하면
Then stale 상태가 false로 변경되어야 한다
And LastUpdateTime이 현재 시각으로 갱신되어야 한다
```

### AC-46: Event 모드 캐시 기반 변경 감지

```gherkin
Given mode가 "event"이고 RegisterCache에 이전 값 {0: 1234, 1: 5678}이 있는 경우
When 폴링 결과가 {0: 1234, 1: 9999}이면
Then 주소 1의 변경만 감지해야 한다 (old: 5678, new: 9999)
And 주소 0은 변경되지 않았으므로 무시해야 한다
```

### AC-47: 캐시 값 미변경 시 이벤트 미전송

```gherkin
Given mode가 "event"이고 RegisterCache에 이전 값 {0: 1234, 1: 5678}이 있는 경우
When 폴링 결과가 {0: 1234, 1: 5678}이면 (동일)
Then msgCh에 메시지를 전달하지 않아야 한다
```

### AC-48: 온디맨드 캐시 조회 (get_cache)

```gherkin
Given RegisterCache에 데이터가 저장된 경우
When Process({"command":"get_cache","device_id":"plc-01"})가 호출되면
Then {"status":"ok","device_id":"plc-01","cache":{...},"last_update_times":{...}} JSON이 반환되어야 한다
And cache 객체에 coils, discrete_inputs, holding_registers, input_registers가 포함되어야 한다
```

### AC-49: 동시성 안전 (go test -race)

```gherkin
Given RegisterCache가 여러 고루틴에서 동시에 접근되는 경우
When 폴링 고루틴이 캐시를 쓰고, Process() 고루틴이 캐시를 읽을 때
Then 데이터 레이스가 발생하지 않아야 한다
And go test -race ./internal/agent/modbus/... 명령이 에러 없이 통과해야 한다
And sync.RWMutex를 통한 동시성 보호가 적용되어야 한다
```

### AC-50: 기본 StaleThreshold = PollInterval * 3

```gherkin
Given stale_threshold가 설정되지 않고 poll_interval이 "5s"인 경우
When parseModbusConfig(opts)가 호출되면
Then config.StaleThreshold가 15초(5s * 3)로 설정되어야 한다

Given stale_threshold가 "30s"로 명시적으로 설정된 경우
When parseModbusConfig(opts)가 호출되면
Then config.StaleThreshold가 30초로 설정되어야 한다 (명시적 설정 우선)
```

---

## 10. Module 11 & 12: Configuration & Error Handling (설정 및 오류 처리)

### AC-51: 필수 필드 누락 시 명확한 에러

```gherkin
Given Transport.Options에 devices 필드가 누락된 경우
When parseModbusConfig(opts)가 호출되면
Then "devices is required" 메시지를 포함한 에러가 반환되어야 한다

Given devices 배열이 비어있는 경우
When parseModbusConfig(opts)가 호출되면
Then "at least one device is required" 메시지를 포함한 에러가 반환되어야 한다

Given device에 host 필드가 누락된 경우
When parseModbusConfig(opts)가 호출되면
Then "host is required for device" 메시지를 포함한 에러가 반환되어야 한다
```

### AC-52: 기본값 적용

```gherkin
Given Transport.Options에 최소 설정만 제공된 경우
When parseModbusConfig(opts)가 호출되면
Then poll_interval 기본값이 "5s"로 설정되어야 한다
And mode 기본값이 "interval"로 설정되어야 한다
And read_mode 기본값이 "cached"로 설정되어야 한다
And port 기본값이 502로 설정되어야 한다
And unit_id 기본값이 1로 설정되어야 한다
And connect_timeout 기본값이 "5s"로 설정되어야 한다
And read_timeout 기본값이 "3s"로 설정되어야 한다
And reconnect_interval 기본값이 "10s"로 설정되어야 한다
And max_reconnect_attempts 기본값이 10으로 설정되어야 한다
And msg_channel_size 기본값이 256으로 설정되어야 한다
```

### AC-53: 연속 에러 시 Health Degraded

```gherkin
Given 연속 에러 임계값이 3인 경우
When 디바이스 "plc-01"에서 3회 연속 통신 에러가 발생하면
Then Health 상태가 Degraded로 보고되어야 한다

When 해당 디바이스의 다음 통신이 성공하면
Then Health 상태가 Healthy로 복원되어야 한다
And 에러 카운터가 0으로 초기화되어야 한다
```

### AC-54: 타임아웃 시 로그 기록 및 다음 디바이스 계속

```gherkin
Given 디바이스 "plc-01"과 "sensor-01"이 설정된 경우
When "plc-01"의 읽기 요청이 read_timeout(기본값: 3초) 내에 응답하지 않으면
Then 타임아웃 에러를 slog에 기록해야 한다
And "plc-01"을 건너뛰고 "sensor-01"의 폴링을 계속해야 한다
And "sensor-01"의 RegisterCache는 정상 갱신되어야 한다
```

### AC-55: 구조화 slog 로깅

```gherkin
Given MODBUSAgent가 동작 중인 경우
When 로그가 기록되면
Then slog 패키지를 사용해야 한다
And 로그 메시지에 device_id 필드가 포함되어야 한다
And 오류 로그에 function_code, address 필드가 포함되어야 한다
And 에러 발생 시 error 필드가 포함되어야 한다
And 로그 레벨이 적절히 구분되어야 한다 (Info, Warn, Error)
```

---

## 11. 에지 케이스 테스트

### EC-01: 빈 레지스터 응답 처리

```gherkin
Given FC03 읽기 요청이 전송된 경우
When 응답의 데이터 길이가 0이면
Then 에러를 로그에 기록해야 한다
And RegisterCache를 갱신하지 않아야 한다
And 다음 레지스터 그룹의 폴링을 계속해야 한다
```

### EC-02: 유효하지 않은 JSON 명령

```gherkin
Given MODBUSAgent가 Running 상태인 경우
When Process([]byte("invalid json"))가 호출되면
Then ErrInvalidCommand 에러가 반환되어야 한다
```

### EC-03: 미등록 디바이스 ID 명령

```gherkin
Given devices에 "plc-01"만 등록된 경우
When Process({"command":"write_register","device_id":"unknown-device",...})가 호출되면
Then ErrDeviceNotFound 에러가 반환되어야 한다
```

### EC-04: Transaction ID 순환

```gherkin
Given Transaction ID가 0xFFFF에 도달한 경우
When 다음 MODBUS 요청이 전송되면
Then Transaction ID가 0x0000으로 순환되어야 한다
And 요청-응답 매칭이 정상 동작해야 한다
```

### EC-05: msgCh 채널 버퍼 포화

```gherkin
Given msg_channel_size가 256이고 msgCh에 256개의 메시지가 쌓인 경우
When 새로운 폴링 데이터가 수집되면
Then 폴링 고루틴이 블로킹되지 않아야 한다 (비블로킹 전송 또는 드롭)
And 채널 오버플로우 시 slog에 경고가 기록되어야 한다
```

### EC-06: 동시 Process 호출

```gherkin
Given MODBUSAgent가 Running 상태인 경우
When 여러 고루틴에서 동시에 Process()가 호출되면
Then 데이터 레이스 없이 처리되어야 한다
And go test -race에서 에러가 발생하지 않아야 한다
```

### EC-07: Stop 후 리소스 정리

```gherkin
Given MODBUSAgent가 Running 상태인 경우
When Stop(ctx)이 호출되면
Then 폴링 고루틴이 종료되어야 한다 (stopCh 채널 닫기)
And 모든 TCP 연결이 닫혀야 한다
And msgCh 채널이 정리되어야 한다
And 상태가 Stopped로 전이되어야 한다
And 고루틴 누수가 없어야 한다
```

### EC-08: 유효하지 않은 Function Code 거부

```gherkin
Given 레지스터 그룹 설정에 function_code가 99인 경우
When parseModbusConfig(opts)가 호출되면
Then ErrInvalidFunctionCode 에러가 반환되어야 한다
```

### EC-09: 주소 범위 초과 거부

```gherkin
Given 쓰기 명령의 address가 70000인 경우
When Process()를 통해 쓰기 명령이 수신되면
Then ErrAddressOutOfRange 에러가 반환되어야 한다
And MODBUS 요청이 전송되지 않아야 한다
```

### EC-10: 센티널 에러 errors.Is() 호환

```gherkin
Given fmt.Errorf("%w: detail", ErrDeviceOffline) 형태로 래핑된 에러가 있는 경우
When errors.Is(err, ErrDeviceOffline)로 검사하면
Then true가 반환되어야 한다

Given 모든 12개 센티널 에러에 대해
When errors.Is() 호환성을 검사하면
Then 모든 에러가 호환되어야 한다
```

---

## 12. 타입 등록

### AC-REG-01: 에이전트 타입 등록

```gherkin
Given 빈 TypeRegistry가 있는 경우
When RegisterModbusTCPTypes(registry)가 호출되면
Then registry.HasType("modbus-tcp")가 true를 반환해야 한다
And registry.ListTypes()에 "modbus-tcp"가 포함되어야 한다
```

### AC-REG-02: 팩토리를 통한 에이전트 생성

```gherkin
Given "modbus-tcp" 타입이 등록된 TypeRegistry가 있는 경우
When registry.CreateAgent("modbus-tcp", validConfig)가 호출되면
Then MODBUSAgent 인스턴스가 반환되어야 한다
And 에이전트 Type()이 "modbus-tcp"를 반환해야 한다
```

---

## 13. Quality Gate 기준

### 13.1 테스트 커버리지

| 파일 | 최소 커버리지 |
|------|-------------|
| `errors.go` | 100% |
| `config.go` | 90%+ |
| `cache.go` | 85%+ |
| `client.go` | 85%+ |
| `protocol.go` | 85%+ |
| `agent.go` | 85%+ |
| `register.go` | 100% |
| **전체 패키지** | **85%+** |

### 13.2 코드 품질

```bash
go test -race -cover ./internal/agent/modbus/...   # >= 85% 커버리지, 데이터 레이스 0건
go vet ./...                                        # 경고 0건
go build ./cmd/xflowd ./cmd/xflow                   # 빌드 성공
```

### 13.3 Definition of Done

- [ ] 모든 요구사항(REQ-MODBUS-001-*)에 대한 테스트 시나리오 존재
- [ ] `go test -race -cover ./internal/agent/modbus/...` 통과 (85%+ 커버리지)
- [ ] `go vet ./internal/agent/modbus/...` 경고 0건
- [ ] `go build ./cmd/xflowd ./cmd/xflow` 빌드 성공
- [ ] TypeRegistry에 "modbus-tcp" 타입 등록 완료
- [ ] cmd/xflowd/main.go에서 RegisterModbusTCPTypes 호출 추가
- [ ] 모든 12개 센티널 에러 정의 및 errors.Is() 호환 확인
- [ ] RegisterCache 동시성 안전 (sync.RWMutex, go test -race 통과)
- [ ] 예제 설정 YAML 파일 작성
- [ ] SPEC-MODBUS-001 문서와 구현 코드 간 추적성(traceability) 확인
- [ ] Write-Through 캐시 전략 검증 (쓰기 성공 후에만 캐시 갱신)
- [ ] Cached/Direct 읽기 모드 전환 테스트 완료
- [ ] Interval/Event 모드 동작 테스트 완료
- [ ] BridgeInOut 양방향 통합 테스트 완료

---

*SPEC-MODBUS-001 Acceptance v1.0.0*
*작성자: xtra*
*날짜: 2026-02-26*
