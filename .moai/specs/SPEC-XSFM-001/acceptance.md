---
id: SPEC-XSFM-001
type: acceptance
version: "0.3.0"
created: "2026-07-28"
updated: "2026-07-28"
author: xtra
---

# SPEC-XSFM-001 인수 기준: 지하철 역사 설비 관리 에이전트 (MQTT)

## 1. Module 1: MQTT 트랜스포트 · 설정

### Scenario 1.1: 설정 파싱 및 초기화

```gherkin
Given transport_mode 미지정(기본 direct)이고 XSFMConfig에 broker "tcp://broker:1883", state_topic_template "xsfm/{device_id}/state", command_topic_template "xsfm/{device_id}/cmd", 유효한 payload_mapping이 설정된 경우
When XSFMAgent.Init(config)이 호출되면
Then 에이전트 상태가 Running으로 전이되어야 한다
And MQTT 클라이언트가 broker/tls/client_id/lwt 설정으로 생성되어야 한다
And 설정 기반 디바이스가 로스터에 Source="config", Online=false로 등록되어야 한다
```

### Scenario 1.2: 토픽 템플릿 placeholder 누락 거부

```gherkin
Given state_topic_template이 "xsfm/state" (device_id placeholder 없음)인 경우
When parseXSFMConfig(opts)가 호출되면
Then ErrInvalidTopicTemplate 에러가 반환되어야 한다
```

### Scenario 1.3: 브로커 주소 누락 거부

```gherkin
Given broker가 빈 문자열인 경우
When parseXSFMConfig(opts)가 호출되면
Then ErrBrokerRequired 에러가 반환되어야 한다
```

### Scenario 1.4: 페이로드 매핑 누락 거부

```gherkin
Given payload_mapping에 power_field / fan_speed_field가 없는 경우
When parseXSFMConfig(opts)가 호출되면
Then ErrInvalidPayloadMapping 에러가 반환되어야 한다
```

### Scenario 1.5: Agent / MessageReceiver 인터페이스 준수

```gherkin
Given XSFMAgent 인스턴스가 생성된 경우
When agent.Agent 및 agent.MessageReceiver 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.Agent = (*XSFMAgent)(nil))
```

### Scenario 1.6: MQTT 재연결 시 구독 복원 (direct 모드)

```gherkin
Given transport_mode가 "direct"이고 XSFMAgent가 Running 상태이며 디바이스 3대의 state 토픽을 구독 중인 경우
When MQTT 연결이 끊겼다가 자동 재연결되면
Then 재연결 후 3대의 state 토픽 구독이 모두 복원되어야 한다
```

### Scenario 1.7: transport_mode 기본값 direct

```gherkin
Given XSFMConfig에 transport_mode가 지정되지 않은 경우
When parseXSFMConfig(opts)가 호출되면
Then transport_mode가 "direct"로 기본 설정되어야 한다 (v0.1.0 동작과 일치)
```

### Scenario 1.8: 유효하지 않은 transport_mode 거부

```gherkin
Given transport_mode가 "bridge" (direct/port 이외의 값)인 경우
When parseXSFMConfig(opts)가 호출되면
Then ErrInvalidTransportMode 에러가 반환되어야 한다
```

### Scenario 1.9: port 모드에서 브로커/토픽 설정 미사용 허용

```gherkin
Given transport_mode가 "port"이고 broker와 토픽 템플릿이 비어 있으나 유효한 payload_mapping이 있는 경우
When XSFMAgent.Init(config)이 호출되면
Then 브로커/토픽 누락에 대한 에러 없이 Running으로 전이되어야 한다
And MQTT 클라이언트가 생성되지 않아야 한다 (client=nil)
```

### Scenario 1.10: direct 모드에서 브로커 누락은 여전히 거부

```gherkin
Given transport_mode가 "direct"이고 broker가 빈 문자열인 경우
When parseXSFMConfig(opts)가 호출되면
Then ErrBrokerRequired 에러가 반환되어야 한다
```

---

## 1B. Module 1B: 듀얼 트랜스포트 모드 (direct | port)

> 트랜스포트 모드는 I/O 경계만 선택하며, 페이로드 매핑·로스터·제어 구성·그룹 fan-out 등 프로토콜/로직 레이어는 두 모드에서 동일하다. 아래 시나리오는 각 모드의 I/O 경계 동작을 검증한다.

### Scenario 1B.1: direct 모드 — 구독 상태 유입이 로스터를 갱신

```gherkin
Given transport_mode가 "direct"이고 목 MQTTClient가 주입된 상태에서 "ap-101"이 등록·구독 중인 경우
When 구독한 state 토픽으로 {power:true, fan_speed:2}에 해당하는 페이로드가 수신되면
Then payload_mapping으로 디코딩되어 "ap-101"의 Power=true, FanSpeed=2가 갱신되어야 한다
And device_state_changed 메시지가 msgCh로 emit되어야 한다
```

### Scenario 1B.2: direct 모드 — 제어 명령이 브로커로 발행

```gherkin
Given transport_mode가 "direct"이고 목 MQTTClient가 주입되며 "ap-101"이 등록된 경우
When Process({"command":"set_power","device_id":"ap-101","params":{"power":true}})가 호출되면
Then 목 MQTTClient의 Publish가 renderTopic(command_topic_template,"ap-101") 토픽으로 1회 호출되어야 한다
And 제어 출력 포트로는 아무것도 방출되지 않아야 한다 (direct 모드는 브로커 발행 경로)
```

### Scenario 1B.3: port 모드 — 입력 포트 상태 유입이 로스터/상태를 갱신

```gherkin
Given transport_mode가 "port"이고 브로커 연결 없이 "ap-101"이 등록된 경우
When 상태 노드의 상태 입력 포트로 "ap-101"의 {power:true, fan_speed:2} 상태 메시지가 공급되면
Then direct 모드와 동일한 payload_mapping 디코딩 경로로 "ap-101"의 Power=true, FanSpeed=2, LastSeen이 갱신되어야 한다
And device_state_changed 텔레메트리 메시지가 msgCh로 emit되어야 한다
And 어떠한 브로커 구독도 사용되지 않아야 한다
```

### Scenario 1B.4: port 모드 — 개별 제어 명령이 제어 출력 포트로 방출

```gherkin
Given transport_mode가 "port"이고 "ap-101"이 등록된 경우
When Process({"command":"set_power","device_id":"ap-101","params":{"power":true}})가 호출되면
Then payload_mapping.power_field로 인코딩된 명령 메시지가 제어 출력 포트로 1개 방출되어야 한다 (하류 mqtt-out 노드가 발행)
And 방출된 명령 메시지의 페이로드 포맷이 direct 모드 발행 페이로드와 동일해야 한다 (공유 인코딩 경로)
And 에이전트가 직접 브로커에 발행하지 않아야 한다
```

### Scenario 1B.5: port 모드 — set_fan_speed 명령이 제어 출력 포트로 방출

```gherkin
Given transport_mode가 "port"이고 "ap-101"이 전원 ON 상태인 경우
When Process({"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":2}})가 호출되면
Then fan_speed 2가 payload_mapping.fan_speed_field로 인코딩된 명령 메시지가 제어 출력 포트로 방출되어야 한다
```

### Scenario 1B.6: port 모드 — 그룹 명령이 멤버별 N개 메시지를 제어 출력 포트로 방출

```gherkin
Given transport_mode가 "port"이고 group_id "concourse-b1"에 "ap-101","ap-102","ap-103"이 소속된 경우
When Process({"command":"set_power","group_id":"concourse-b1","params":{"power":true}})가 호출되면
Then 제어 출력 포트로 3개의 멤버별 명령 메시지(ap-101/ap-102/ap-103)가 방출되어야 한다 (per-member fan-out)
And 집계 응답에 3개 멤버의 device_id별 status가 포함되어야 한다
And 그룹 제어 감사 레코드가 group_id + 멤버 목록과 함께 기록되어야 한다
```

### Scenario 1B.7: port 모드 — 상태 입력 포트와 제어 출력 포트는 별개의 포트

```gherkin
Given transport_mode가 "port"인 에이전트의 플로우 노드 구성인 경우
When 상태 노드의 상태 입력 포트와 제어 노드의 제어 출력 포트를 조회하면
Then 두 포트는 서로 다른 별개의 포트여야 한다 (단일 포트로 결합되지 않음)
And 상태 입력 포트는 상류 mqtt-in 노드에, 제어 출력 포트는 하류 mqtt-out 노드에 연결 가능해야 한다
```

### Scenario 1B.8: 두 모드의 인코딩/디코딩 결과 동일성

```gherkin
Given 동일한 payload_mapping으로 direct 모드와 port 모드 에이전트가 각각 구성된 경우
When 동일한 set_power 명령을 각 모드에서 처리하면
Then 두 모드가 산출한 제어 명령 페이로드가 바이트 단위로 동일해야 한다 (I/O 경계만 다르고 로직 레이어는 동일)
```

---

## 2. Module 2: 디바이스 로스터 · 영속화

### Scenario 2.1: 런타임 디바이스 등록

```gherkin
Given XSFMAgent가 Running 상태인 경우
When Process({"command":"add_device","device_id":"ap-101","name":"대합실-A","group_id":"concourse-b1"})가 호출되면
Then device_id "ap-101"이 로스터에 Source="bridge"로 등록되어야 한다
And "ap-101"의 state 토픽이 구독되어야 한다
And device_registered 이벤트가 msgCh로 전달되어야 한다
```

### Scenario 2.2: 설정 디바이스 제거 거부

```gherkin
Given device_id "ap-config"가 Source="config"로 등록된 경우
When Process({"command":"remove_device","device_id":"ap-config"})가 호출되면
Then ErrConfigDeviceProtected 에러가 반환되어야 한다
```

### Scenario 2.3: group_id 속성 수정

```gherkin
Given device_id "ap-101"이 group_id ""인 경우
When Process({"command":"set_device","device_id":"ap-101","group_id":"platform-1"})가 호출되면
Then "ap-101"의 group_id가 "platform-1"로 갱신되어야 한다
And GroupMembers("platform-1")에 "ap-101"이 포함되어야 한다
```

### Scenario 2.4: 로스터 영속화 라운드트립

```gherkin
Given registry_path가 설정되고 device_id "ap-101"(Source="bridge", group_id "platform-1")이 등록된 경우
When 에이전트가 Stop 후 동일 설정으로 재시작되면
Then "ap-101"이 group_id "platform-1"과 함께 로스터에 복원되어야 한다
And Source="config" 디바이스는 영속화 파일에 저장되지 않아야 한다
```

### Scenario 2.5: 위치 계층 속성(station/place/index) 등록 및 라운드트립 (v0.3.0)

```gherkin
Given registry_path가 설정된 경우
When Process({"command":"add_device","device_id":"ap-101","station":"ST-101","place":"승강장","index":3})가 호출되면
Then "ap-101"의 Station="ST-101", Place="승강장", Index=3이 로스터에 반영되어야 한다
And 에이전트가 Stop 후 동일 설정으로 재시작되면 "ap-101"의 station/place/index가 그대로 복원되어야 한다
```

### Scenario 2.6: 위치 속성 미지정 하위호환 (v0.3.0)

```gherkin
Given station/place/index가 없던 v0.2.0 포맷의 영속화 파일을 로드하는 경우
When 에이전트가 Init으로 로스터를 복원하면
Then station=""/place=""/index=0으로 하위호환 복원되고 오류 없이 Running으로 전이되어야 한다
```

---

## 2B. Module 2B: 역사 레지스트리 (station → line 호선 매핑, v0.3.0)

### Scenario 2B.1: 역사 레지스트리 설정 시드 + station→line 조회

```gherkin
Given station_registry 시드에 {"ST-101": {"line":"line-2","display_name":"강남","order":5}}가 설정된 경우
When ResolveLine("ST-101")이 호출되면
Then "line-2"가 반환되어야 한다
And GetStation("ST-101")의 display_name이 "강남", order가 5여야 한다
```

### Scenario 2B.2: 미등록 station 조회 거부

```gherkin
Given 역사 레지스트리에 "ST-999"가 없는 경우
When ResolveLine("ST-999")이 호출되면
Then ErrStationNotFound 에러가 반환되어야 한다
```

### Scenario 2B.3: 역사 레지스트리 CRUD 라운드트립 (device_metadata 패턴)

```gherkin
Given station_registry_path가 설정된 경우
When Process({"command":"add_station","station":"ST-102","line":"line-2","display_name":"역삼","order":6})가 호출되고 에이전트가 재시작되면
Then "ST-102"가 line "line-2"와 함께 레지스트리에 복원되어야 한다 (단일 JSON 파일 + atomic write)
And list_stations가 ST-101/ST-102를 order 순으로 반환해야 한다
```

### Scenario 2B.4: 호선별 역사 목록 (StationsByLine)

```gherkin
Given "line-2"에 ST-101(order 5), ST-102(order 6)이 등록된 경우
When StationsByLine("line-2")가 호출되면
Then ST-101, ST-102가 order 오름차순으로 반환되어야 한다
```

### Scenario 2B.5: line은 디바이스 속성이 아님 (SSOT는 레지스트리)

```gherkin
Given "ap-101"이 Station="ST-101"로 등록되고 ST-101→line-2 매핑이 있는 경우
When "ap-101"의 line을 조회하면
Then ResolveLine("ap-101".Station)=="line-2"로 도출되어야 한다 (디바이스 로스터에 line이 중복 저장되지 않음)
```

---

## 3. Module 3: 개별 제어 (2-축)

> 아래 시나리오는 `transport_mode="direct"`(기본값) 기준의 제어 의미론(값 검증·2-축·감사)을 검증한다. "command 토픽으로 발행"은 direct 모드의 명령 출력 경계이며, `port` 모드의 등가 방출(제어 출력 포트 emit)은 §1B(Scenario 1B.4/1B.5)에서 검증한다. 값 검증·게이트 정책·감사 로직은 두 모드에서 동일하다.

### Scenario 3.1: set_power on

```gherkin
Given device_id "ap-101"이 로스터에 등록된 경우
When Process({"command":"set_power","device_id":"ap-101","params":{"power":true}})가 호출되면
Then payload_mapping.power_field로 인코딩된 명령이 renderTopic(command_topic_template,"ap-101") 토픽으로 발행되어야 한다
And 제어 감사 레코드가 기록되어야 한다
And 응답 status가 "ok"여야 한다
```

### Scenario 3.2: set_power off

```gherkin
Given device_id "ap-101"이 전원 ON 상태인 경우
When Process({"command":"set_power","device_id":"ap-101","params":{"power":false}})가 호출되면
Then off 값이 인코딩된 명령이 command 토픽으로 발행되어야 한다
```

### Scenario 3.3: set_fan_speed 1/2/3 (전원 ON)

```gherkin
Given device_id "ap-101"이 전원 ON 상태인 경우
When Process({"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":2}})가 호출되면
Then fan_speed 2가 payload_mapping.fan_speed_field로 인코딩되어 발행되어야 한다
And 제어 감사 레코드가 기록되어야 한다
```

### Scenario 3.4: 유효하지 않은 풍량 값 거부

```gherkin
Given device_id "ap-101"이 등록된 경우
When Process({"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":4}})가 호출되면
Then ErrInvalidFanSpeed 에러가 반환되어야 한다
And 어떠한 MQTT 발행도 수행되지 않아야 한다
```

### Scenario 3.5: 전원 OFF 상태에서 풍량 명령 (기본 거부 정책)

```gherkin
Given device_id "ap-101"이 전원 OFF 상태이고 기본 정책(거부)이 적용된 경우
When Process({"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":3}})가 호출되면
Then ErrPowerOff 에러가 반환되어야 한다
```

### Scenario 3.6: 미등록 디바이스 제어 거부

```gherkin
Given device_id "ap-999"가 로스터에 없는 경우
When Process({"command":"set_power","device_id":"ap-999","params":{"power":true}})가 호출되면
Then ErrDeviceNotFound 에러가 반환되어야 한다
```

### Scenario 3.7: 복합 제어 (set_multiple)

```gherkin
Given device_id "ap-101"이 전원 OFF 상태인 경우
When Process({"command":"set_multiple","device_id":"ap-101","params":{"power":true,"fan_speed":3}})가 호출되면
Then 전원 ON이 먼저 발행된 뒤 fan_speed 3이 발행되어야 한다
```

---

## 3B. Module 3 확장: 제어 응답 대기 (state echo 대기, v0.3.0)

> 응답 = 디바이스가 새 상태를 상태 유입으로 다시 보고하는 상태 에코(별도 ack 아님). 상관은 device_id(+명령). 아래는 direct 기준이며 port 모드 에코 상관은 Scenario 3B.3에서 검증한다.

### Scenario 3B.1: 응답 대기 성공 (타임아웃 내 상태 에코가 명령 반영)

```gherkin
Given control_response_timeout이 "5s"이고 "ap-101"이 등록된 경우
When Process({"command":"set_power","device_id":"ap-101","params":{"power":true}})가 호출되고
And 타임아웃 이전에 "ap-101"의 상태 유입으로 power=true 에코가 도착하면
Then 해당 제어가 성공(status "ok")으로 resolve되어야 한다
And 응답이 반환되기 전에 pending-command가 device_id "ap-101"로 등록되었어야 한다
```

### Scenario 3B.2: 응답 대기 실패 (에코 미도착 → ErrControlTimeout)

```gherkin
Given control_response_timeout이 "5s"이고 "ap-101"이 등록된 경우
When set_power 명령이 발행되었으나 5초 내 매칭 상태 에코가 도착하지 않으면
Then 해당 제어가 ErrControlTimeout으로 실패 처리되고 status가 "timeout"이어야 한다
And pending-command 항목이 제거되어야 한다
```

### Scenario 3B.3: port 모드 에코 상관 (입력 포트 경유)

```gherkin
Given transport_mode가 "port"이고 control_response_timeout이 "5s"이며 "ap-101"이 등록된 경우
When set_power 명령이 제어 출력 포트로 방출되고
And 이후 상태 입력 포트로 "ap-101"의 power=true 에코 메시지가 공급되면
Then direct 모드와 동일한 디코딩·상관 경로로 pending이 성공 resolve되어야 한다 (device_id 상관)
```

### Scenario 3B.4: 타임아웃 후 도착한 에코 무시

```gherkin
Given "ap-101"의 set_power pending이 타임아웃으로 이미 제거된 경우
When 타임아웃 이후 "ap-101"의 power=true 에코가 뒤늦게 도착하면
Then 로스터 상태는 정상 갱신되되 이미 만료된 pending을 소급 성공 처리하지 않아야 한다
```

### Scenario 3B.5: 동일 디바이스 동시 명령 독립 판정

```gherkin
Given "ap-101"에 set_power와 set_fan_speed가 거의 동시에 발행되어 각각 pending인 경우
When set_power 에코만 먼저 도착하면
Then set_power pending만 성공 resolve되고 set_fan_speed pending은 영향받지 않아야 한다
And set_fan_speed 에코가 타임아웃되면 set_fan_speed만 ErrControlTimeout으로 판정되어야 한다
```

### Scenario 3B.6: control_response_timeout=0 fire-and-forget

```gherkin
Given control_response_timeout이 "0"인 경우
When set_power 명령이 처리되면
Then 발행 즉시 성공 반환되고 상태 에코를 대기하지 않아야 한다 (pending 미등록)
```

---

## 4. Module 4: 그룹 제어 fan-out (NET-NEW)

> 아래 시나리오는 `transport_mode="direct"`(기본값) 기준으로 "개별 MQTT 명령이 발행"을 검증한다. `port` 모드의 등가 동작(멤버별 N개 메시지를 제어 출력 포트로 방출)은 §1B Scenario 1B.6에서 검증한다. fan-out 도출·부분 실패·집계·우선순위 로직은 두 모드에서 동일하다.

### Scenario 4.1: 그룹 명령이 모든 멤버로 fan-out

```gherkin
Given group_id "concourse-b1"에 device_id "ap-101","ap-102","ap-103"이 소속된 경우
When Process({"command":"set_power","group_id":"concourse-b1","params":{"power":true}})가 호출되면
Then "ap-101","ap-102","ap-103" 각각의 command 토픽으로 개별 MQTT 명령이 발행되어야 한다 (per-device fan-out)
And 집계 응답에 3개 멤버의 device_id별 status가 포함되어야 한다
And 그룹 제어 감사 레코드가 group_id + 멤버 목록과 함께 기록되어야 한다
```

### Scenario 4.2: 부분 실패 best-effort

```gherkin
Given group_id "concourse-b1"에 3대가 소속되고 "ap-102" 발행이 실패하는 경우
When 그룹 set_power 명령이 fan-out되면
Then "ap-101","ap-103" 발행은 계속 시도되어야 한다 (중단하지 않음)
And 집계 응답에서 "ap-102"는 status "error", 나머지는 "ok"로 표기되어야 한다
And 응답 최상위 status가 "partial"(또는 "error")로 표기되어야 한다
```

### Scenario 4.3: 빈 그룹 no-op

```gherkin
Given group_id "empty-grp"에 소속된 디바이스가 없는 경우
When Process({"command":"set_power","group_id":"empty-grp","params":{"power":true}})가 호출되면
Then ErrEmptyGroup 에러(또는 빈 멤버 집계)가 반환되어야 한다
And 어떠한 MQTT 발행도 수행되지 않아야 한다
```

### Scenario 4.4: device_id + group_id 동시 지정 시 device_id 우선

```gherkin
Given "ap-101"(group "concourse-b1")이 등록된 경우
When Process({"command":"set_power","device_id":"ap-101","group_id":"concourse-b1","params":{"power":true}})가 호출되면
Then 단일 디바이스 "ap-101"에만 발행되어야 한다 (그룹 fan-out 미수행)
```

### Scenario 4.5: station 셀렉터 일괄 제어 (v0.3.0)

```gherkin
Given "ap-101","ap-102"가 Station="ST-101"이고 "ap-201"이 Station="ST-201"인 경우
When Process({"command":"set_power","station":"ST-101","params":{"power":true}})가 호출되면
Then "ST-101"에 속한 "ap-101","ap-102"에만 per-device fan-out되어야 한다 ("ap-201" 제외)
And 각 멤버가 자체 응답 대기를 수행하고 집계 응답에 멤버별 status가 포함되어야 한다
```

### Scenario 4.6: line(호선) 셀렉터 일괄 제어 (v0.3.0)

```gherkin
Given ST-101→line-2, ST-102→line-2 매핑이 있고 "ap-101"(ST-101), "ap-102"(ST-102), "ap-301"(ST-301→line-3)이 등록된 경우
When Process({"command":"set_fan_speed","line":"line-2","params":{"fan_speed":1}})가 호출되면
Then StationsByLine("line-2")=[ST-101,ST-102]로 도출되어 "ap-101","ap-102"에만 fan-out되어야 한다 ("ap-301" 제외)
And line→stations→devices 역방향 조회가 역사 레지스트리를 SSOT로 사용해야 한다
```

### Scenario 4.7: 셀렉터 fan-out 멤버별 응답 대기 집계 (ok/error/timeout)

```gherkin
Given line "line-2"에 "ap-101","ap-102","ap-103"이 대상이고 "ap-102"가 응답 타임아웃, "ap-103" 발행이 실패하는 경우
When line 셀렉터 set_power 명령이 fan-out되면
Then "ap-101"은 status "ok", "ap-102"는 "timeout"(ErrControlTimeout), "ap-103"은 "error"로 집계되어야 한다
And 나머지 멤버 처리가 중단되지 않아야 한다 (best-effort)
And 최상위 status가 "partial"로 표기되어야 한다
```

### Scenario 4.8: 셀렉터 우선순위 (device_id > station > line > group_id)

```gherkin
Given "ap-101"이 station "ST-101", group "concourse-b1"에 속한 경우
When Process({"command":"set_power","device_id":"ap-101","station":"ST-101","line":"line-2","group_id":"concourse-b1","params":{"power":true}})가 호출되면
Then device_id "ap-101" 단일 대상으로만 해석되어야 한다 (station/line/group fan-out 미수행)
```

### Scenario 4.9: line 대상 중 미등록 station 디바이스 제외

```gherkin
Given "ap-101"(ST-101→line-2), "ap-102"(ST-999, 레지스트리 미등록)가 등록된 경우
When Process({"command":"set_power","line":"line-2","params":{"power":true}})가 호출되면
Then "ap-101"에만 fan-out되고 미등록 station "ST-999"의 "ap-102"는 대상에서 제외되어야 한다
And 제외 사실이 집계 응답/로그에 표기되어야 한다
```

---

## 5. Module 5: 상태 모니터링

> 아래 시나리오는 `transport_mode="direct"`(기본값) 기준으로 "state 토픽 구독" 유입을 검증한다. `port` 모드의 등가 동작(상태 입력 포트 유입 → 동일 디코딩 경로)은 §1B Scenario 1B.3에서 검증한다. 디코딩·관측 기반 emit·타임아웃 오프라인 판정 로직은 두 모드에서 동일하며, LWT 경로는 direct 모드 전용이다.

### Scenario 5.1: state 구독 → 로스터 갱신 + 상태 emit

```gherkin
Given device_id "ap-101"이 등록되고 state 토픽을 구독 중인 경우
When state 토픽에 {power:true, fan_speed:2}에 해당하는 페이로드가 수신되면
Then payload_mapping으로 디코딩되어 "ap-101"의 Power=true, FanSpeed=2, LastSeen이 갱신되어야 한다
And device_state_changed 메시지가 changed_fields와 함께 msgCh로 emit되어야 한다
And 메시지 timestamp가 epoch milliseconds(int64)여야 한다
```

### Scenario 5.2: 관측 기반 emit (미관측 축 생략)

```gherkin
Given device_id "ap-101"이 재시작 직후 fan_speed를 아직 관측하지 않은 경우
When power만 포함된 state 페이로드가 수신되면
Then emit되는 상태 payload에 fan_speed가 포함되지 않아야 한다 (기본값 방출 금지)
And power=false인 경우 신뢰할 수 없는 fan_speed는 payload에서 생략되어야 한다
```

### Scenario 5.3: 오프라인 감지 (타임아웃)

```gherkin
Given offline_timeout이 "60s"이고 "ap-101"이 온라인인 경우
When 60초 동안 "ap-101"의 state 메시지가 수신되지 않으면
Then "ap-101"의 Online이 false로 변경되어야 한다
And device_offline 이벤트가 msgCh로 전달되어야 한다
```

### Scenario 5.4: 오프라인 감지 (LWT, direct 모드 전용)

```gherkin
Given transport_mode가 "direct"이고 lwt_enabled이 true이고 "ap-101"이 온라인인 경우
When 브로커가 "ap-101" 세션 단절에 대한 LWT를 통지하면
Then "ap-101"이 오프라인으로 판정되고 device_offline 이벤트가 전달되어야 한다
```

### Scenario 5.5: 온라인 복구

```gherkin
Given "ap-101"이 오프라인 상태인 경우
When "ap-101"이 다시 state 메시지를 보고하면
Then Online이 true로 복원되고 device_online 이벤트가 전달되어야 한다
```

### Scenario 5.6: request_state 캐시 응답

```gherkin
Given "ap-101"의 상태가 캐시된 경우
When Process({"command":"request_state","device_id":"ap-101"})가 호출되면
Then MQTT 통신 없이 캐시된 현재 상태가 즉시 JSON으로 반환되어야 한다
```

---

## 6. Module 6: 로깅

### Scenario 6.1: 상태 시계열이 influxdb_write 경로로 흐름

```gherkin
Given xsfm_status 노드가 influxdb_write 노드에 연결된 플로우인 경우
When 에이전트가 power/fan_speed 변경을 msgCh로 emit하면
Then status 노드가 이를 플로우로 전달하고 influxdb_write 노드가 시계열로 기록해야 한다
And 에이전트는 InfluxDB에 직접 기록하지 않아야 한다
```

### Scenario 6.2: 개별 제어 감사 기록

```gherkin
Given "ap-101"에 대한 set_power 명령이 처리된 경우
When 명령이 완료되면
Then who/when/what/device_id "ap-101"을 포함한 제어 감사 레코드가 remote_audit_repository에 기록되어야 한다
```

### Scenario 6.3: 그룹 제어 감사 기록

```gherkin
Given group_id "concourse-b1"(멤버 3대)에 대한 그룹 set_power가 처리된 경우
When 명령이 완료되면
Then 감사 레코드에 대상 group_id와 fan-out된 멤버 목록이 포함되어야 한다
```

---

## 7. Module 7~9: 등록 · 배선 · 어댑터 · 프론트엔드

### Scenario 7.1: 타입 등록 및 팩토리 생성

```gherkin
Given RegisterXSFMTypes(registry)가 호출된 경우
When registry.CreateAgent("xsfm", config)가 호출되면
Then XSFMAgent 인스턴스가 생성되어 반환되어야 한다
```

### Scenario 7.2: main.go 배선

```gherkin
Given cmd/xflowd/main.go의 에이전트 타입 등록 지점(:376-400 부근)인 경우
When 애플리케이션이 부트스트랩되면
Then RegisterXSFMTypes가 호출되어 "xsfm" 타입이 사용 가능해야 한다
```

### Scenario 8.1: 디바이스 어댑터 CommandSpec

```gherkin
Given internal/device/adapter/xsfm.go의 CommandSpec인 경우
When CommandSpec을 조회하면
Then set_power(bool)과 set_fan_speed(enum ["1","2","3"])가 선언되어 있어야 한다
And CommandExecutor가 명령을 에이전트 Process() JSON 명령으로 변환해야 한다
```

### Scenario 9.1: 프론트엔드 에이전트 스키마

```gherkin
Given web/src/config/agentSchemas.ts인 경우
When 에이전트 타입 목록과 필드 스키마를 조회하면
Then { value: 'xsfm', label: 'Subway Facilities Manager' } 엔트리와 XSFM_FIELDS(broker/tls/토픽템플릿/페이로드매핑/qos/offline_timeout)가 존재해야 한다
And en/ko i18n 문자열이 추가되어야 한다
```

---

## 8. Definition of Done (완료 정의)

- [ ] 9개 모듈의 모든 EARS 요구사항 구현
- [ ] **듀얼 트랜스포트 모드** (`transport_mode`: direct | port, 기본 direct) — I/O 경계 추상화(`CommandSink` + state ingress) + 모드별 설정 검증 분기
- [ ] **direct 모드**: MQTT 트랜스포트(Paho) 연결/구독/발행/LWT/재연결 동작
- [ ] **port 모드**: 브로커 연결 없이 상태 입력 포트 유입 + 제어 출력 포트 방출, 상태 입력 포트 ≠ 제어 출력 포트 (분리)
- [ ] 두 모드의 공유 로직 레이어 동작 동일성 (인코딩/디코딩 바이트 단위 동일)
- [ ] 설정 주도 토픽 템플릿(direct) + 페이로드 매핑(양 모드 공통) 시임 완성 (하드코딩 없음)
- [ ] 2-축 상태 모델 + 개별 제어 (set_power, set_fan_speed 1/2/3, 풍량은 ON에서만 유효)
- [ ] **디바이스 위치 계층 (station/place/index) — 선택/하위호환 + 영속화 라운드트립 (v0.3.0)**
- [ ] **역사 레지스트리 (station→line SSOT) — device_metadata 패턴 저장소 + CRUD/lookup(ResolveLine/StationsByLine) (v0.3.0)**
- [ ] **제어 응답 대기 (control_response_timeout + pending-command 레지스트리, 에코 상관 by device_id, ErrControlTimeout, 늦은 에코 무시, 동시 명령 독립) — 개별·셀렉터 공통 (v0.3.0)**
- [ ] **line/station 셀렉터 일괄 제어 (station registry 경유, 멤버별 응답 대기 + ok/error/timeout 집계) (v0.3.0)**
- [ ] 그룹 fan-out (부분 실패 best-effort + 집계 응답) — NET-NEW
- [ ] 상태 구독 → 로스터 갱신 + 관측 기반 emit + 오프라인 감지(LWT + 타임아웃)
- [ ] 상태 시계열(status → influxdb_write, 직접 기록 없음) + 제어 감사(개별/그룹)
- [ ] 로스터 add/remove/set + 영속화 라운드트립 (device_id 키잉)
- [ ] 타입 등록 + main.go 배선 + API 어댑터 + 프론트엔드 스키마 + i18n
- [ ] 디바이스 어댑터 CommandSpec + Executor
- [ ] 단위 테스트 (목 브로커/목 매핑 기반) 통과, 85% 커버리지 목표
- [ ] TRUST 5 품질 게이트 통과

## 9. 품질 게이트 및 검증 방법

- 목(mock) MQTTClient 주입으로 direct 모드 발행/구독을 검증한다 (실 브로커 불필요).
- 목 `CommandSink` + 목 상태 입력 포트 주입으로 port 모드 방출/유입을 검증한다 (실 브로커·실 노드 불필요).
- direct/port 두 모드가 동일 payload_mapping으로 바이트 단위 동일한 명령 페이로드를 산출함을 assert한다 (공유 로직 레이어 동일성).
- port 모드에서 상태 입력 포트와 제어 출력 포트가 별개의 포트임을 assert한다.
- 목 payload_mapping으로 시임 경로를 매뉴얼 확보 전에도 선검증한다.
- 그룹/셀렉터 fan-out은 멤버별 방출 호출 횟수(direct=발행, port=출력 포트 emit)와 집계 응답(ok/error/timeout)을 assert한다.
- 오프라인 감지·응답 대기 타임아웃은 가상 시계(fake clock) 또는 짧은 offline_timeout/control_response_timeout으로 검증한다.
- 관측 기반 emit은 payload에 미관측 축 부재를 assert한다.
- 응답 대기는 목 상태 유입(direct 구독 콜백 / port 입력 포트)으로 에코를 주입하여 성공 resolve와 타임아웃(ErrControlTimeout)을 assert하고, 동시 명령의 독립 판정·늦은 에코 무시를 검증한다.
- 역사 레지스트리는 device_metadata 저장소 패턴을 미러하여 station→line 라운드트립(파일 재로드)과 StationsByLine 정렬을 assert한다.
- 위치 속성(station/place/index)은 라운드트립 + v0.2.0 포맷 하위호환 로드를 assert한다.
- line/station 셀렉터는 대상 도출(레지스트리 경유)과 미등록 station 제외, 우선순위(device_id>station>line>group_id)를 assert한다.
