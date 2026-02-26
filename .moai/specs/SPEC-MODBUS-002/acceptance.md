---
id: SPEC-MODBUS-002
type: acceptance
version: "1.0.0"
created: "2026-02-26"
updated: "2026-02-27"
author: xtra
---

# SPEC-MODBUS-002 인수 기준: MODBUS/TCP Server Agent 구현

## 1. Module 1: Server Configuration (서버 설정)

### Scenario 1.1: 유효한 서버 설정 파싱

```gherkin
Given Transport.Options에 listen_port 5020, unit_id 1, max_connections 5,
      register_map에 holding_registers(count:100), coils(count:50)이 설정된 경우
When parseModbusServerConfig(opts)가 호출되면
Then ModbusServerConfig가 정상적으로 반환되어야 한다
And ListenPort가 5020이어야 한다
And UnitID가 1이어야 한다
And MaxConnections가 5이어야 한다
And RegisterMapConfig.HoldingRegisters.Count가 100이어야 한다
And RegisterMapConfig.Coils.Count가 50이어야 한다
```

### Scenario 1.2: 기본값 적용

```gherkin
Given Transport.Options에 register_map만 설정된 경우 (다른 필드 생략)
When parseModbusServerConfig(opts)가 호출되면
Then ListenAddress 기본값 "0.0.0.0"이 적용되어야 한다
And ListenPort 기본값 502가 적용되어야 한다
And UnitID 기본값 1이 적용되어야 한다
And MaxConnections 기본값 10이 적용되어야 한다
And IdleTimeout 기본값 60s가 적용되어야 한다
And MsgChannelSize 기본값 256이 적용되어야 한다
```

### Scenario 1.3: register_map 누락 에러

```gherkin
Given Transport.Options에 register_map이 누락된 경우
When parseModbusServerConfig(opts)가 호출되면
Then ErrInvalidRegisterMap 에러가 반환되어야 한다
```

### Scenario 1.4: 초기값 파싱

```gherkin
Given holding_registers에 count:5, initial_values: [100, 200, 300]이 설정된 경우
When parseModbusServerConfig(opts)가 호출되면
Then HoldingRegisters.Count가 5이어야 한다
And HoldingRegisters.InitialValues가 [100, 200, 300]이어야 한다
And 주소 0~2는 100, 200, 300으로 초기화되고 주소 3~4는 0으로 초기화되어야 한다
```

### Scenario 1.5: 유효하지 않은 포트 거부

```gherkin
Given listen_port가 0인 경우
When parseModbusServerConfig(opts)가 호출되면
Then 에러가 반환되어야 한다

Given listen_port가 70000인 경우
When parseModbusServerConfig(opts)가 호출되면
Then 에러가 반환되어야 한다
```

### Scenario 1.6: 유효하지 않은 unit_id 거부

```gherkin
Given unit_id가 248인 경우
When parseModbusServerConfig(opts)가 호출되면
Then 에러가 반환되어야 한다
```

---

## 2. Module 2: TCP Listener (TCP 리스너)

### Scenario 2.1: 리스닝 시작

```gherkin
Given ModbusServerAgent가 초기화된 경우
When Start(ctx)가 호출되면
Then 설정된 주소:포트에서 TCP 리스닝이 시작되어야 한다
And 로그에 리스닝 주소와 포트가 기록되어야 한다
```

### Scenario 2.2: 클라이언트 연결 수락

```gherkin
Given ModbusServerAgent가 리스닝 중인 경우
When 외부 MODBUS 클라이언트가 TCP 연결을 요청하면
Then 연결이 수락되어야 한다
And 활성 연결 수가 1 증가해야 한다
And client_connected 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 2.3: 최대 연결 수 제한

```gherkin
Given max_connections가 2이고 2개의 클라이언트가 이미 연결된 경우
When 3번째 클라이언트가 TCP 연결을 요청하면
Then 연결이 즉시 거부(close)되어야 한다
And 로그에 경고가 기록되어야 한다
```

### Scenario 2.4: 클라이언트 연결 해제 알림

```gherkin
Given 1개의 클라이언트가 연결된 경우
When 클라이언트가 TCP 연결을 닫으면
Then 활성 연결 수가 1 감소해야 한다
And client_disconnected 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 2.5: Graceful Shutdown

```gherkin
Given 2개의 클라이언트가 연결된 경우
When Stop(ctx)가 호출되면
Then TCP 리스너가 닫혀야 한다
And 모든 클라이언트 연결이 종료되어야 한다
And 에이전트 상태가 Stopped로 전이되어야 한다
```

### Scenario 2.6: 유휴 타임아웃

```gherkin
Given idle_timeout이 "1s"이고 클라이언트가 연결된 경우
When 클라이언트가 1초 동안 요청을 보내지 않으면
Then 해당 연결이 닫혀야 한다
And client_disconnected 이벤트가 전송되어야 한다
```

---

## 3. Module 3: Connection Handler (연결 핸들러)

### Scenario 3.1: MBAP 프레임 정상 읽기

```gherkin
Given 클라이언트가 유효한 FC03 읽기 요청(MBAP + PDU)을 전송한 경우
When 핸들러가 프레임을 읽으면
Then Transaction ID, Protocol ID, Length, Unit ID가 올바르게 파싱되어야 한다
And Function Code와 데이터가 요청 핸들러로 전달되어야 한다
```

### Scenario 3.2: Transaction ID 에코

```gherkin
Given 클라이언트가 Transaction ID 0x1234로 요청을 전송한 경우
When 서버가 응답을 반환하면
Then 응답의 Transaction ID가 0x1234이어야 한다
```

### Scenario 3.3: Unit ID 불일치 무시

```gherkin
Given 서버의 unit_id가 1인 경우
When 클라이언트가 Unit ID 5로 요청을 전송하면
Then 해당 요청은 무시(응답 없음)되어야 한다
And Debug 레벨 로그에 불일치가 기록되어야 한다
```

### Scenario 3.4: 불완전한 MBAP 헤더

```gherkin
Given 클라이언트가 3 bytes만 전송하고 연결을 닫은 경우
When 핸들러가 프레임을 읽으면
Then 해당 연결이 정리되어야 한다
And 에러 로그가 기록되어야 한다
```

### Scenario 3.5: 비정상적 Length 필드 거부

```gherkin
Given 클라이언트가 Length 필드가 300인 MBAP Header를 전송한 경우
When 핸들러가 프레임을 읽으면
Then 해당 연결이 닫혀야 한다
And 보안 경고가 로그에 기록되어야 한다
```

---

## 4. Module 4: Request Handler (요청 핸들러)

### Scenario 4.1: FC03 Read Holding Registers

```gherkin
Given holding_registers 영역에 주소 0~9에 값 [100, 200, 300, 400, 500, 600, 700, 800, 900, 1000]이 설정된 경우
When FC03 요청 (start_address=2, quantity=3)이 수신되면
Then 응답 PDU에 byte_count=6, 데이터=[300, 400, 500] (Big-Endian)이 포함되어야 한다
```

### Scenario 4.2: FC01 Read Coils

```gherkin
Given coils 영역에 주소 0~7에 값 [true, false, true, true, false, false, true, false]이 설정된 경우
When FC01 요청 (start_address=0, quantity=8)이 수신되면
Then 응답 PDU에 byte_count=1, 데이터=[0b01001101] (0x4D)이 포함되어야 한다
```

### Scenario 4.3: FC06 Write Single Register

```gherkin
Given holding_registers 영역에 주소 10의 현재 값이 0인 경우
When FC06 요청 (address=10, value=1234)이 수신되면
Then holding_registers[10]이 1234로 변경되어야 한다
And 응답이 요청과 동일한 echo-back이어야 한다
And register_changed 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 4.4: FC05 Write Single Coil

```gherkin
Given coils 영역에 주소 5의 현재 값이 false인 경우
When FC05 요청 (address=5, value=0xFF00)이 수신되면
Then coils[5]가 true로 변경되어야 한다
And 응답이 요청과 동일한 echo-back이어야 한다
And register_changed 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 4.5: FC16 Write Multiple Registers

```gherkin
Given holding_registers 영역에 주소 0~99가 정의된 경우
When FC16 요청 (address=10, quantity=3, values=[100, 200, 300])이 수신되면
Then holding_registers[10]=100, [11]=200, [12]=300으로 변경되어야 한다
And 응답에 시작 주소 10, 수량 3이 포함되어야 한다
And register_changed 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 4.6: FC15 Write Multiple Coils

```gherkin
Given coils 영역에 주소 0~49가 정의된 경우
When FC15 요청 (address=0, quantity=4, values=[true, false, true, true])이 수신되면
Then coils[0]=true, [1]=false, [2]=true, [3]=true로 변경되어야 한다
And 응답에 시작 주소 0, 수량 4가 포함되어야 한다
```

### Scenario 4.7: 지원하지 않는 Function Code

```gherkin
Given 서버가 FC01~FC06, FC15, FC16만 지원하는 경우
When FC08 (Diagnostics) 요청이 수신되면
Then Exception 응답(FC=0x88, Exception Code=0x01 Illegal Function)이 반환되어야 한다
```

### Scenario 4.8: 주소 범위 초과

```gherkin
Given holding_registers 영역에 start_address=0, count=100이 설정된 경우
When FC03 요청 (start_address=95, quantity=10)이 수신되면
Then Exception 응답(Exception Code=0x02 Illegal Data Address)이 반환되어야 한다
```

### Scenario 4.9: FC05 잘못된 코일 값

```gherkin
Given coils 영역이 정의된 경우
When FC05 요청 (address=0, value=0x1234)이 수신되면
Then Exception 응답(Exception Code=0x03 Illegal Data Value)이 반환되어야 한다
```

### Scenario 4.10: 읽기 전용 영역 쓰기 거부

```gherkin
Given input_registers 영역이 정의된 경우
When FC06 요청으로 input_registers에 쓰기를 시도하면
Then Exception 응답(Exception Code=0x01 Illegal Function)이 반환되어야 한다
```

### Scenario 4.11: 수량 0 거부

```gherkin
Given holding_registers 영역이 정의된 경우
When FC03 요청 (start_address=0, quantity=0)이 수신되면
Then Exception 응답(Exception Code=0x03 Illegal Data Value)이 반환되어야 한다
```

### Scenario 4.12: 수량 최대 초과 거부

```gherkin
Given holding_registers 영역이 정의된 경우 (count >= 200)
When FC03 요청 (start_address=0, quantity=126)이 수신되면
Then Exception 응답(Exception Code=0x03 Illegal Data Value)이 반환되어야 한다
```

---

## 5. Module 5: Register Map (레지스터 맵)

### Scenario 5.1: 설정 기반 초기화

```gherkin
Given RegisterMapConfig에 holding_registers(start:0, count:5, initial_values:[10,20,30])이 설정된 경우
When NewRegisterMap(config)가 호출되면
Then holdingRegisters[0]=10, [1]=20, [2]=30, [3]=0, [4]=0이 설정되어야 한다
```

### Scenario 5.2: 범위 내 읽기

```gherkin
Given holding_registers에 start:0, count:100이 정의된 경우
When ReadHoldingRegisters(start=10, quantity=5)가 호출되면
Then 주소 10~14의 값이 uint16 슬라이스로 반환되어야 한다
```

### Scenario 5.3: 범위 외 읽기 에러

```gherkin
Given holding_registers에 start:0, count:100이 정의된 경우
When ReadHoldingRegisters(start=98, quantity=5)가 호출되면
Then ErrAddressNotMapped 에러가 반환되어야 한다
```

### Scenario 5.4: 동시성 안전

```gherkin
Given RegisterMap 인스턴스가 생성된 경우
When 10개 goroutine이 동시에 ReadHoldingRegisters와 WriteHoldingRegisters를 호출하면
Then 데이터 레이스 없이 모든 작업이 완료되어야 한다
And go test -race에서 에러가 발생하지 않아야 한다
```

### Scenario 5.5: 변경 추적

```gherkin
Given holdingRegisters[10]의 현재 값이 100인 경우
When WriteHoldingRegisters(start=10, values=[200])가 호출되면
Then 반환된 ChangeSet에 address=10, old_value=100, new_value=200이 포함되어야 한다
```

### Scenario 5.6: 스냅샷 조회

```gherkin
Given RegisterMap에 coils, holding_registers가 각각 설정된 경우
When GetSnapshot()가 호출되면
Then 모든 영역의 현재 값이 map[string]any로 반환되어야 한다
And 원본 맵과 독립적인 복제본이어야 한다
```

---

## 6. Module 6: Bridge Integration (Bridge 연동)

### Scenario 6.1: set_register 명령 처리

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Process({"command":"set_register","params":{"address":10,"value":1234}})가 호출되면
Then holdingRegisters[10]이 1234로 변경되어야 한다
And 응답 JSON에 "status":"ok"가 포함되어야 한다
And register_updated 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 6.2: set_coil 명령 처리

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Process({"command":"set_coil","params":{"address":5,"value":true}})가 호출되면
Then coils[5]가 true로 변경되어야 한다
And register_updated 이벤트가 msgCh에 전달되어야 한다
```

### Scenario 6.3: set_input 명령 처리 (내부 전용)

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Process({"command":"set_input","params":{"area":"input_registers","address":0,"value":5000}})가 호출되면
Then inputRegisters[0]이 5000으로 변경되어야 한다
And 이후 FC04 읽기 요청에서 주소 0의 값이 5000이어야 한다
```

### Scenario 6.4: get_map 명령 처리

```gherkin
Given RegisterMap에 다양한 값이 설정된 경우
When Process({"command":"get_map"})가 호출되면
Then 응답 JSON에 coils, discrete_inputs, holding_registers, input_registers의 전체 스냅샷이 포함되어야 한다
```

### Scenario 6.5: get_status 명령 처리

```gherkin
Given ModbusServerAgent가 2개의 클라이언트가 연결된 상태인 경우
When Process({"command":"get_status"})가 호출되면
Then 응답 JSON에 listen_address, listen_port, active_connections=2, unit_id가 포함되어야 한다
```

### Scenario 6.6: ReceiveMessage 채널 기반 수신

```gherkin
Given ModbusServerAgent가 Running 상태이고 msgCh에 메시지가 있는 경우
When ReceiveMessage(ctx)가 호출되면
Then msgCh에서 JSON 바이트 메시지가 반환되어야 한다
And 메시지는 유효한 JSON 형식이어야 한다
```

### Scenario 6.7: ReceiveMessage 컨텍스트 취소

```gherkin
Given msgCh가 비어있는 경우
When 취소된 context로 ReceiveMessage(ctx)가 호출되면
Then context.Canceled 에러가 즉시 반환되어야 한다
```

### Scenario 6.8: 외부 쓰기 알림 (register_changed)

```gherkin
Given 외부 MODBUS 클라이언트가 연결되어 FC06 요청을 전송한 경우
When holdingRegisters[10]의 값이 변경되면
Then register_changed 이벤트가 msgCh에 전달되어야 한다
And 이벤트의 source가 "external"이어야 한다
And remote_addr이 클라이언트 주소를 포함해야 한다
```

### Scenario 6.9: 내부 설정 알림 (register_updated)

```gherkin
Given Process()를 통해 set_register 명령이 처리된 경우
When holdingRegisters[10]의 값이 변경되면
Then register_updated 이벤트가 msgCh에 전달되어야 한다
And 이벤트의 source가 "internal"이어야 한다
```

### Scenario 6.10: 유효하지 않은 명령 거부

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Process({"command":"unknown_command"})가 호출되면
Then ErrInvalidCommand 에러가 반환되어야 한다
```

### Scenario 6.11: 유효하지 않은 JSON 거부

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Process([]byte("invalid json"))가 호출되면
Then JSON 파싱 에러가 반환되어야 한다
```

---

## 7. Module 7: Agent Lifecycle (에이전트 생명주기)

### Scenario 7.1: 에이전트 생성 및 초기화

```gherkin
Given 유효한 서버 설정이 제공된 경우
When NewModbusServerAgent(agentConfig)가 호출되면
Then 에이전트가 성공적으로 생성되어야 한다
And 상태가 Running이어야 한다
And RegisterMap이 설정에 따라 초기화되어야 한다
```

### Scenario 7.2: Agent 인터페이스 준수

```gherkin
Given ModbusServerAgent 인스턴스가 생성된 경우
When agent.Agent 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.Agent = (*ModbusServerAgent)(nil))
```

### Scenario 7.3: MessageReceiver 인터페이스 준수

```gherkin
Given ModbusServerAgent 인스턴스가 생성된 경우
When agent.MessageReceiver 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.MessageReceiver = (*ModbusServerAgent)(nil))
```

### Scenario 7.4: StatefulAgent 인터페이스 준수

```gherkin
Given ModbusServerAgent 인스턴스가 생성된 경우
When agent.StatefulAgent 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.StatefulAgent = (*ModbusServerAgent)(nil))
```

### Scenario 7.5: Type() 반환값

```gherkin
Given ModbusServerAgent 인스턴스가 생성된 경우
When Type()가 호출되면
Then "modbus-tcp-server"가 반환되어야 한다
```

### Scenario 7.6: Health 상태 보고 (Running)

```gherkin
Given ModbusServerAgent가 Running 상태이고 리스너가 활성인 경우
When Health()가 호출되면
Then Status가 HealthHealthy이어야 한다
```

### Scenario 7.7: Health 상태 보고 (Paused)

```gherkin
Given ModbusServerAgent가 Paused 상태인 경우
When Health()가 호출되면
Then Status가 HealthDegraded이어야 한다
```

### Scenario 7.8: Pause / Resume

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When Pause(ctx)가 호출되면
Then 새로운 요청 처리가 일시 중단되어야 한다
And 기존 연결은 유지되어야 한다

When Resume(ctx)가 호출되면
Then 요청 처리가 재개되어야 한다
```

### Scenario 7.9: State() 정보 조회

```gherkin
Given ModbusServerAgent가 Running 상태이고 1개의 클라이언트가 연결된 경우
When State()가 호출되면
Then listen_address, listen_port, unit_id, active_connections=1, max_connections, register_map이 포함되어야 한다
```

---

## 8. Module 8: Registration & Examples (타입 등록)

### Scenario 8.1: 에이전트 타입 등록

```gherkin
Given 빈 DefaultManager가 있는 경우
When RegisterModbusServerTypes(mgr)가 호출되면
Then "modbus-tcp-server" 타입이 등록되어야 한다
```

### Scenario 8.2: 팩토리를 통한 에이전트 생성

```gherkin
Given "modbus-tcp-server" 타입이 등록된 DefaultManager가 있는 경우
When 유효한 AgentConfig로 에이전트를 생성하면
Then ModbusServerAgent 인스턴스가 반환되어야 한다
And 에이전트 Type()이 "modbus-tcp-server"를 반환해야 한다
```

---

## 9. Module 9: Error Handling (에러 처리)

### Scenario 9.1: 서버 전용 센티널 에러 정의 확인

```gherkin
Given modbusserver 패키지가 로드된 경우
When 6개의 서버 전용 센티널 에러 변수를 확인하면
Then ErrServerAlreadyRunning, ErrListenFailed, ErrMaxConnectionsReached,
     ErrInvalidRegisterMap, ErrAddressNotMapped, ErrReadOnlyArea가 정의되어 있어야 한다
And 각 에러는 errors.Is()로 비교 가능해야 한다
```

### Scenario 9.2: errors.Is() 호환성

```gherkin
Given fmt.Errorf("%w: detail", ErrAddressNotMapped) 형태로 래핑된 에러가 있는 경우
When errors.Is(err, ErrAddressNotMapped)로 검사하면
Then true가 반환되어야 한다
```

### Scenario 9.3: 구조화 로깅

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When 외부 클라이언트로부터 유효하지 않은 요청이 수신되면
Then 로그에 remote_addr, function_code, unit_id, error 필드가 포함되어야 한다
```

---

## 10. 통합 시나리오

### Scenario 10.1: End-to-End 읽기/쓰기 사이클

```gherkin
Given ModbusServerAgent가 포트 15020에서 리스닝 중이고
      holding_registers[0~9]이 모두 0으로 초기화된 경우
When 외부 MODBUS 클라이언트가 TCP 연결을 수립하고
     FC06 요청 (address=5, value=42)을 전송하면
Then 서버가 echo-back 응답을 반환해야 한다
And holding_registers[5]가 42로 변경되어야 한다
And register_changed 이벤트가 msgCh에 전달되어야 한다

When 동일 클라이언트가 FC03 요청 (start_address=0, quantity=10)을 전송하면
Then 응답 데이터에서 주소 5의 값이 42이어야 한다
```

### Scenario 10.2: Bridge Process와 외부 읽기 연동

```gherkin
Given ModbusServerAgent가 리스닝 중이고 input_registers 영역이 정의된 경우
When Bridge Process()를 통해 set_input(area=input_registers, address=0, value=9999)가 호출되면
Then inputRegisters[0]이 9999로 변경되어야 한다

When 외부 MODBUS 클라이언트가 FC04 요청 (start_address=0, quantity=1)을 전송하면
Then 응답 데이터에서 값이 9999이어야 한다
```

### Scenario 10.3: 다중 클라이언트 동시 접근

```gherkin
Given ModbusServerAgent가 리스닝 중인 경우
When 3개의 클라이언트가 동시에 연결하여 각각 FC03 읽기 요청을 전송하면
Then 모든 클라이언트가 올바른 응답을 수신해야 한다
And 데이터 레이스 없이 처리되어야 한다
```

---

## 11. 성능 기준

### Scenario 11.1: 요청 처리 지연

```gherkin
Given ModbusServerAgent가 Running 상태인 경우
When 단일 FC03 읽기 요청이 수신되면
Then 응답이 10ms 이내에 반환되어야 한다
```

### Scenario 11.2: 동시 클라이언트 처리

```gherkin
Given max_connections=10으로 설정된 경우
When 10개의 클라이언트가 동시에 읽기 요청을 전송하면
Then 모든 요청이 100ms 이내에 처리되어야 한다
```

### Scenario 11.3: msgCh 오버플로우 처리

```gherkin
Given msg_channel_size가 10으로 설정된 경우
When 20개의 연속 쓰기 요청으로 이벤트가 발생하면
Then 채널이 가득 차더라도 요청 처리 goroutine이 블로킹되지 않아야 한다
And 채널 오버플로우 시 로그에 경고가 기록되어야 한다
```

---

## 12. Quality Gate 기준

### 12.1 테스트 커버리지

| 파일 | 최소 커버리지 |
|------|-------------|
| `errors.go` | 100% |
| `config.go` | 90%+ |
| `register_map.go` | 90%+ |
| `listener.go` | 80%+ |
| `handler.go` | 85%+ |
| `request.go` | 90%+ |
| `agent.go` | 85%+ |
| `register.go` | 100% |
| **전체 패키지** | **85%+** |

### 12.2 코드 품질

- `go vet ./internal/agent/modbusserver/...` 경고 0건
- `go test -race ./internal/agent/modbusserver/...` 데이터 레이스 0건
- 모든 exported 타입 및 함수에 GoDoc 주석 포함
- 센티널 에러는 `errors.Is()` 호환

### 12.3 Definition of Done

- [x] 모든 요구사항(REQ-MODBUS-002-*)에 대한 테스트 시나리오 존재 -- PASSED
- [x] `go test -race -cover ./internal/agent/modbusserver/...` 통과 (89.5% 커버리지) -- PASSED
- [x] `go vet ./internal/agent/modbusserver/...` 경고 0건 -- PASSED
- [x] TypeRegistry에 "modbus-tcp-server" 타입 등록 완료 -- PASSED
- [x] cmd/xflowd/main.go에서 RegisterModbusServerTypes 호출 추가 -- PASSED
- [x] protocol.go 상수/함수 정상 import 및 재사용 확인 -- PASSED
- [x] 외부 MODBUS 클라이언트 -> 서버 End-to-End 테스트 통과 -- PASSED
- [x] Bridge Process() -> 레지스터 변경 -> 외부 읽기 연동 테스트 통과 -- PASSED
- [x] SPEC-MODBUS-002 문서와 구현 코드 간 추적성(traceability) 확인 -- PASSED

---

---

## 13. 인수 검증 결과

### 13.1 모듈별 검증 현황

| 모듈 | 시나리오 수 | 결과 |
|------|-------------|------|
| Module 1: Server Configuration | 6 | ALL PASSED |
| Module 2: TCP Listener | 6 | ALL PASSED |
| Module 3: Connection Handler | 5 | ALL PASSED |
| Module 4: Request Handler | 12 | ALL PASSED |
| Module 5: Register Map | 6 | ALL PASSED |
| Module 6: Bridge Integration | 11 | ALL PASSED |
| Module 7: Agent Lifecycle | 9 | ALL PASSED |
| Module 8: Registration & Examples | 2 | ALL PASSED |
| Module 9: Error Handling | 3 | ALL PASSED |
| 통합 시나리오 | 3 | ALL PASSED |
| 성능 기준 | 3 | ALL PASSED |

### 13.2 품질 게이트 결과

| 항목 | 기준 | 결과 | 판정 |
|------|------|------|------|
| 테스트 커버리지 | 85%+ | 89.5% | PASSED |
| Race Detection | 0건 | 0건 | PASSED |
| go vet | 0건 경고 | 0건 | PASSED |
| go build | Success | Success | PASSED |

### 13.3 최종 판정

**SPEC-MODBUS-002: ALL ACCEPTANCE CRITERIA PASSED**

- 검증일: 2026-02-27
- 검증자: xtra

---

*SPEC-MODBUS-002 Acceptance v1.0.0*
*작성자: xtra*
*날짜: 2026-02-27*
