# SPEC-CENTURY-001: 인수 기준

> **SPEC ID**: SPEC-CENTURY-001
> **버전**: 0.4.2
> **상태**: Implemented (v0.1.2 41/41 + v0.4.x 그룹 H 15개 모두 통과; v0.2.0 그룹 G 8개 시나리오는 M6 구현 예정)
> **형식**: Given-When-Then (Gherkin)
> **분류**: A=프레임 디코딩 / B=에이전트 런타임 / C=플로우 노드 / D=설정 및 등록 / E=필드 디코딩 정책 / F=WRITE 중복 처리 / G=TCP transport (v0.2.0) / H=Device-centric output (v0.3.0 → v0.4.2, Breaking x2)

## 변경 이력

| 날짜 | 버전 | 변경 |
|------|------|------|
| 2026-05-18 | 0.1.0 | 초안 작성 (A~E 그룹, AC-A1~AC-E5) |
| 2026-05-18 | 0.1.1 | B1 시나리오를 B1a/B1b 로 확장하여 다중 IDU 검증. 그룹 F (WRITE 중복 처리, F1~F4) 신설. |
| 2026-05-18 | 0.1.2 | M1-M5 구현 완료. "Verification Results" 부록 신설 — 41 시나리오의 자동 테스트 매핑 (테스트 함수명 → AC ID). 상태 Implemented 전이. |
| 2026-05-18 | 0.2.0 | 그룹 G (TCP transport, G1~G8) 신설 — tcp-client dial/reconnect/read timeout, tcp-server accept/secondary rejection, transport-aware cycle_idle_timeout default, AC-B9 invariant under TCP. Definition of Done 에 G 그룹 추가. Verification Results 부록의 그룹 G 는 "PENDING (M6)" 상태. 상태 Implemented → Draft. |
| 2026-05-19 | 0.3.0 | **Breaking** — 그룹 H (Device-centric output, H1~H10) 신설. agent msgCh emit 의 default 가 register-decoded 에서 device_state 로 전환 (REQ-CENTURY-033/034/035). H1 register-decoded opt-out, H2 첫 reg02 emit, H3 reg04 current_temp_c, H4 동일값 미emit, H5 mode/power 전이, H6 keepalive fallback, H7 offline 전이 즉시 emit, H8 register-only mode, H9 both-off ErrCenturyNoOutputEnabled, H10 multi-IDU 독립. Definition of Done 에 H 그룹 추가. |
| 2026-05-19 | 0.3.1~0.3.8 | schema 통일 + register-decoded transform + change detection 강화 (8 hotfix). AC-H 시나리오의 필드 이름 마이그레이션: `set_temp_c`→`target_temp`, `current_temp_c`→`current_temp`, `fan`→`fan_speed`, `"cooling"`→`"cool"`. 증발기 (`evap_temp_*_c`) 는 device_state schema 에서 제거 (Reg03Decoded 만). v0.3.5 부터 top-level `sub_dev_id`→`dev_id`. |
| 2026-05-19 | 0.3.9 | AC-H11 신설 (keepalive_mode "absolute" wall-clock 정렬 검증). 옵션 `keepalive_mode: relative` (default) / `absolute` (crontab 패턴). |
| 2026-05-19 | 0.3.10 | AC-H12 신설 (lastKeepaliveTime 분리 — change 가 자주 발생해도 keepalive 가 interval 마다 fire). |
| 2026-05-19 | 0.3.11 | AC-H13 신설 (deviceStateBuf drop-oldest semantics) + AC-H14 신설 (`drain_device_state` Process command — polling 노드에서 device_state 관측). |
| 2026-05-19 | 0.4.0 | **Breaking** — AC-H 의 모든 device_state field access 가 nested `state` 그룹으로 마이그레이션 (`m.mode`→`m.state.mode` 등). register-decoded 메시지에 `type` 필드 5종 추가 검증 추가. |
| 2026-05-19 | 0.4.1 | AC-H2 가드 추가 — Reg02 수신 전 emit 보류. 회귀 `TestAgent_DeviceStateGatedByReg02` (Reg04 only → 0 emit, Reg02 후 정상 통합 상태). |
| 2026-05-19 | 0.4.2 | AC-H2 rename to "FirstEmitAfterReg02AndReg04" — Reg02 AND Reg04 모두 수신 후 첫 emit (1회 통합). AC-H3 의미 갱신 "통합 1회 emit". AC-H15 신설 (Reg02-only no emit, Reg04-only no emit 대칭 검증). AC-H2/H4/H5/H6/H7/H10 + Coverage/Drain/Keepalive 테스트 모두 Reg04 frame 추가 주입으로 마이그레이션. 상태 Draft → Implemented (M7 + 11 hotfix 사이클 완료). |

---

## 그룹 A: 프레임 디코딩

### AC-A1: CAP-3 Read Response reg 0x02 디코딩 (냉방 시작 / 25℃ / 바람 17)

```gherkin
Given 프로토콜 문서 부록 A 의 CAP-3 reg 0x02 응답 23바이트 프레임이 주어졌을 때
  And raw = "01 00 30 00 14 00 00 06 3b 00 02 00 01 11 00 00 00 00 fa 00 00 00 fa 00 1b 39 39 00 c1 4c"
When  CenturyAgent 의 프레임 스캐너와 디코더 디스패치를 통과하면
Then  메시지 type 이 "century_reg02_response" 이어야 한다
And   src_addr=0x0001, dst_addr=0x0030, function_code=0x06, register=0x02 가 디코딩되어야 한다
And   fields.mode.value == "cooling" 이고 confirmation_status == "confirmed"
And   fields.fan.value == 17 이고 confirmation_status == "confirmed"
And   fields.setpoint_c.value == 25.0 이고 confirmation_status == "confirmed"
And   17 개 모든 data 바이트가 fields 에 노출되어야 한다
And   reg02_byte_3, reg02_byte_4 등 unknown 필드는 confirmation_status == "unknown"
And   reg02_live_13, reg02_live_14, reg02_live_15 는 confirmation_status == "inferred"
```

### AC-A2: CAP-4 Read Response reg 0x03 디코딩 (정상 상태 / 증발기 9.0/8.5℃)

```gherkin
Given 프로토콜 문서 부록 C 의 CAP-4 reg 0x03 응답 22바이트 프레임이 주어졌을 때
  And data 영역 16바이트 = "5a 00 55 00 00 00 ... 00"
When  디코더 디스패치를 통과하면
Then  메시지 type 이 "century_reg03_response"
And   fields.temp_evap_a_c.value == 9.0 이고 confirmation_status == "confirmed"
And   fields.temp_evap_b_c.value == 8.5 이고 confirmation_status == "confirmed"
And   reg03_pad_4 ~ reg03_pad_15 의 12 개 필드는 모두 value=0, confirmation_status="unknown"
```

### AC-A3: CAP-4 Read Response reg 0x04 디코딩 (운전 부하 데이터)

```gherkin
Given CAP-4 의 reg 0x04 응답 data 14바이트 = "39 f6 09 00 00 00 00 2c e4 03 fc 00 e0 04"
When  decodeReg04Response 를 호출하면
Then  fields.status_bits.value == 0x39 이고 confirmation_status == "inferred"
And   fields.reg04_const_1.value == 0xF6 이고 confirmation_status == "inferred"
And   fields.reg04_const_2.value == 0x09 이고 confirmation_status == "inferred"
And   fields.reg04_const_7.value == 0x2C 이고 confirmation_status == "inferred"
And   fields.op_val_1.value == 996 이고 confirmation_status == "inferred"
And   fields.temp_A_c.value == 25.2 이고 confirmation_status == "inferred"
And   fields.op_val_2.value == 1248 이고 confirmation_status == "inferred"
And   reg04_byte_3 ~ reg04_byte_6 의 4 개 필드는 value=0, confirmation_status="unknown"
```

### AC-A4: CAP-3 Write reg 0x04 디코딩 (모드 명령 = 냉방)

```gherkin
Given CAP-3 의 reg 0x04 write 요청 data 16바이트 = "00 00 00 00 01 00 ... 00 00 c7 00"
When  decodeReg04Write 를 호출하면
Then  메시지 type 이 "century_reg04_write_request"
And   observation_mode 메타가 "passive" 로 설정되어야 한다
And   fields.mode_cmd.value == "cooling" 이고 confirmation_status == "confirmed"
And   fields.write_live_0.value == 0, fields.write_live_1.value == 0
And   fields.write_byte_14.value == 0xC7 (199)
And   fields.write_live_15.value == 0
And   CenturyAgent.transport.Write() 가 호출되지 않아야 한다 (passive 검증)
```

### AC-A5: ACK 프레임 디코딩 (1B payload)

```gherkin
Given 11바이트 프레임 "01 00 30 00 01 00 00 06 00 03 f3" 이 주어졌을 때
When  프레임 스캐너가 처리하면
Then  function_code == 0x06 으로 식별되어야 한다
And   payload_length == 1, payload == [0x00] 이어야 한다
And   메시지 type 이 "century_ack"
And   페이로드 prefix 검증을 건너뛰어야 한다 (ACK 분기)
And   ackCount 카운터가 증가해야 한다
```

### AC-A6: CRC 불일치 프레임 폐기

```gherkin
Given 헤더와 payload 는 유효하나 CRC 가 의도적으로 1비트 변조된 23바이트 프레임이 주어졌을 때
When  프레임 스캐너가 처리하면
Then  ErrCRCMismatch 가 반환되어야 한다
And   framesInvalid 와 invalidCRCMismatch 카운터가 증가해야 한다
And   메시지가 디코더로 전달되지 않아야 한다
And   에이전트가 Error 상태로 전이하지 않아야 한다
And   log_decode_errors=true 일 때만 WARN 로그가 출력되어야 한다
```

### AC-A7: Modbus init 0xFFFF CRC 거부 (회귀 방지)

```gherkin
Given CAP-3 의 CRC 가 정확히 CRC-16/ARC (init 0x0000) 로 계산된 23바이트 프레임이 주어졌을 때
When  의도적으로 init 0xFFFF (Modbus RTU 변형) 로 동일 프레임을 검증하면
Then  검증이 실패해야 한다 (init 차이로 인한 CRC 불일치)
And   본 회귀 테스트는 CI 에서 항상 실행되어야 한다
```

### AC-A8: payload_length 비합리적 값 — 재동기화

```gherkin
Given 스트림에 헤더 reserved 가 0xFF 이거나 payload_length=0xFFFF 인 노이즈 8바이트가 선행한 뒤,
  And 유효한 CAP-3 reg 0x02 응답이 이어지는 경우
When  프레임 스캐너가 처리하면
Then  invalidHeaderInvalid 카운터가 증가하면서 1바이트씩 shift 하여 재동기화해야 한다
And   다음에 등장한 유효 프레임을 정상 추출해야 한다
```

### AC-A9: 페이로드 prefix reserved2 위반

```gherkin
Given function_code=0x06, register byte 위치(payload[2])가 0x05 인 payload prefix 를 가진 프레임
When  프레임 스캐너 + 페이로드 prefix 검증이 수행되면
Then  ErrPayloadPrefixInvalid 가 반환되어야 한다
And   invalidPayloadPrefix 카운터가 증가해야 한다
And   century-raw-frame 노드에는 validation_stage="payload_prefix" 로 노출되어야 한다 (디코드 status 노드에는 노출되지 않음)
```

### AC-A10: 레지스터별 길이 불일치

```gherkin
Given function_code=0x06, register=0x02, data 길이 16B (정상 17B 아님)
When  decodeReg02Response 가 호출되면
Then  ErrRegisterLengthInvalid 가 반환되어야 한다
And   invalidRegisterLength 카운터가 증가해야 한다
```

### AC-A11: in-frame gap 허용 (50ms 이내)

```gherkin
Given 트랜스포트가 헤더 8B 를 송신한 후 40ms 의 idle gap 이 발생하고,
  And 이어서 payload + CRC 가 송신되는 경우
When  프레임 스캐너가 처리하면
Then  완전한 프레임으로 조립되어야 한다
And   in-frame gap 이 50ms 를 초과하면 incomplete frame 으로 폐기되어야 한다
```

---

## 그룹 B: 에이전트 런타임

### AC-B1: 디바이스 자동 발견 — 다중 sub_dev_id 지원

#### AC-B1a: 첫 sub_dev_id 자동 발견

```gherkin
Given auto_discovery=true 인 CenturyAgent 가 시작되었고 devices 맵이 비어있을 때
When  CAP-3 의 reg 0x02 응답 (sub_dev_id=0x3B) 이 회선에서 최초 수신되면
Then  devices 맵이 1개 엔트리를 가져야 한다 (키 0x3B)
And   주소 "3b" 의 CenturyDevice 가 자동 등록되어야 한다
And   Source 가 "auto"
And   Label 이 기본 "indoor-3b"
And   Online 이 true
And   DeviceProvider.ListDevices() 가 해당 디바이스를 포함해야 한다
And   INFO 레벨 로그가 1회 출력되어야 한다 ("디바이스 자동 발견 sub_dev_id=0x3B")
```

#### AC-B1b: 다중 sub_dev_id 격리 등록 (합성 frame)

```gherkin
Given AC-B1a 통과 후 devices 맵에 0x3B device 가 1개 등록된 상태일 때
  And CAP-3 reg 0x02 응답을 base 로 sub_dev_id 만 0x3C 로 변경하고 CRC 를 재계산한 합성 frame 이 회선에 주입되었을 때
When  CenturyAgent 가 해당 frame 을 처리하면
Then  devices 맵이 2개 엔트리를 가져야 한다 (키 0x3B 와 0x3C)
And   각 device 가 독립된 상태를 보유해야 한다 (한 device 의 상태 갱신이 다른 device 에 영향 없음)
And   두 번째 device 의 Label 이 기본 "indoor-3c"
And   두 번째 device 의 Source 가 "auto"
And   INFO 레벨 로그가 추가로 1회 출력되어야 한다 ("디바이스 자동 발견 sub_dev_id=0x3C")
And   DeviceProvider.ListDevices() 가 2개의 device 를 반환해야 한다
And   첫 polling cycle 이내에 새 device 가 생성되어야 한다 (지연 없음)

When  sub_dev_id=0x3B 의 후속 frame 이 들어오면
Then  0x3B device 만 갱신되고 0x3C device 의 lastSeen 은 변경되지 않아야 한다
```

### AC-B2: 오프라인 감지 (offline_timeout = 5s)

```gherkin
Given CenturyDevice 가 online 상태이고 offline_timeout=200ms (테스트용) 인 경우
When  300ms 동안 해당 sub_dev_id 의 프레임이 수신되지 않으면
Then  디바이스의 Online 이 false 로 전환되어야 한다
And   onDeviceStateChange 콜백이 호출되어야 한다
And   INFO 레벨 로그 "디바이스 오프라인 sub_dev_id=0x3B" 가 출력되어야 한다
And   디바이스의 마지막 알려진 status 는 보존되어야 한다 (stale 표시)
```

### AC-B3: ring buffer 오버플로 + drop 카운팅

```gherkin
Given ring_buffer_size=4, log_drops=false 의 CenturyAgent 가 동작 중이고
  And 5 개의 유효 프레임이 빠르게 연속 수신되는 경우
When  drain 으로 ring buffer 를 비우지 않은 상태에서 5번째 프레임이 push 되면
Then  가장 오래된 1 번째 프레임이 evict 되어야 한다
And   framesDropped 카운터가 1 증가해야 한다
And   log_drops=false 이므로 per-drop WARN 로그는 출력되지 않아야 한다
```

### AC-B4: log_drops=true 일 때 per-drop WARN 로그

```gherkin
Given ring_buffer_size=2, log_drops=true 의 CenturyAgent 가 동작 중일 때
When  4 개 프레임이 연속 수신되어 2 개가 drop 되면
Then  framesDropped 가 2 만큼 증가해야 한다
And   "century 에이전트 메시지 버퍼 가득 참, 드롭" WARN 로그가 2 회 출력되어야 한다
And   드롭은 log_drops 값과 무관하게 항상 통계로 계수되어야 한다
```

### AC-B5: 에이전트 라이프사이클 전이

```gherkin
Given 유효한 설정의 CenturyAgent 가 Init→Start 로 Running 상태일 때

When  Pause 를 호출하면
Then  상태가 Paused 로 전이되어야 한다
And   captureLoop 이 일시 정지되어야 한다
And   ring buffer 의 기존 프레임은 보존되어야 한다

When  Resume 을 호출하면
Then  상태가 Running 으로 복귀해야 한다
And   captureLoop 이 재개되어야 한다

When  Stop 을 호출하면
Then  상태가 Stopped 로 전이되어야 한다
And   트랜스포트가 닫혀야 한다
And   stopCh 가 다음 Start() 를 위해 재설정되어야 한다 (SPEC-SERIAL-001 v2.2.0 패턴)
```

### AC-B6: get_stats 커맨드

```gherkin
Given CenturyAgent 가 10 개 reg 0x02 응답, 5 개 reg 0x04 write 를 캡처한 상태일 때
When  Process({"command":"get_stats"}) 를 호출하면
Then  JSON 응답에 다음 필드가 포함되어야 한다:
      - frames_captured, frames_valid, frames_invalid, frames_dropped, bytes_received
      - invalid_length_mismatch, invalid_crc_mismatch, invalid_header_invalid,
        invalid_payload_prefix, invalid_register_length
      - reg02_response_count, reg03_response_count, reg04_response_count,
        reg04_write_count, ack_count
      - unconfirmed_field_observations
      - transport_connected (bool)
And   reg02_response_count >= 10, reg04_write_count >= 5
```

### AC-B7: get_recent 커맨드

```gherkin
Given ring buffer 에 5 개 프레임 (seq=1..5) 이 있고 마지막 seq 가 5 일 때
When  Process({"command":"get_recent","count":3,"last_seq":2}) 를 호출하면
Then  seq=3,4,5 의 3 개 프레임이 반환되어야 한다
And   각 프레임은 디코딩된 typed payload 와 raw_hex 를 포함해야 한다
And   ring buffer 의 프레임이 소비되지 않아야 한다 (get_recent 는 비파괴)
```

### AC-B8: drain 커맨드

```gherkin
Given ring buffer 에 5 개 프레임이 있을 때
When  Process({"command":"drain"}) 를 호출하면
Then  5 개 프레임이 반환되어야 한다
And   ring buffer 가 비워져야 한다
And   이후 get_recent 호출 시 빈 결과가 반환되어야 한다
```

### AC-B9: 패시브 송신 금지 (transport.Write 미호출)

```gherkin
Given CenturyAgent 가 동작 중일 때
When  다음 시나리오가 발생하면:
      - century-status 노드가 polling 함
      - century-control 노드가 제어 메시지를 수신함
      - century 통합 노드가 제어 키 포함 메시지를 수신함
      - get_stats / get_recent / drain 커맨드가 호출됨
Then  어떤 경로로도 트랜스포트의 Write() 가 호출되지 않아야 한다
And   회선 송신 바이트 수가 0 이어야 한다 (mock 트랜스포트 검증)
```

### AC-B10: BufferInfo 와 FrameNotifyCh

```gherkin
Given CenturyAgent 가 동작 중일 때
When  BufferInfo() 를 호출하면
Then  ring_buffer_size, ring_buffer_used, drop_count 가 포함된 BufferInfo 가 반환되어야 한다

When  새 유효 프레임이 ring buffer 에 push 되면
Then  FrameNotifyCh() 가 반환한 채널에 non-blocking 신호가 전달되어야 한다
```

---

## 그룹 C: 플로우 노드

### AC-C1: CenturyStatusNode 폴링 — typed status 이벤트 송출

```gherkin
Given century-status 노드가 agent_ref="my-century" 로 설정되고 poll_interval=100ms 일 때
When  Init 후 폴링 루프가 시작되고 CenturyAgent 의 ring buffer 에 3 개의 reg 0x02 응답이 있으면
Then  out 포트에 3 개의 개별 century_reg02_response 메시지가 송출되어야 한다
And   각 메시지의 메타데이터에 century_source="poll_bulk" 가 설정되어야 한다
And   메타데이터에 century_node_id 가 포함되어야 한다
And   payload 의 timestamp 가 int64 epoch milliseconds 이어야 한다
```

### AC-C2: CenturyStatusNode agent_ref 누락 검증

```gherkin
Given agent_ref 가 빈 문자열인 century-status 노드 설정
When  Configure 를 호출하면
Then  ErrCenturyMissingAgentRef 에러가 반환되어야 한다
```

### AC-C3: CenturyControlNode — 항상 not_supported 응답

```gherkin
Given century-control 노드가 초기화되었을 때
When  제어 메시지 {"power": true, "mode": "cooling", "temperature": 24.0} 를 Process 에 전달하면
Then  out 포트에 다음 구조의 응답이 반환되어야 한다:
      {
        "status": "not_supported",
        "reason": "century_passive_only",
        "message": "Century HVAC agent operates in passive sniff mode; control commands are never transmitted",
        "request": <원본 요약>
      }
And   CenturyAgent.Process() 가 호출되지 않아야 한다
And   트랜스포트 Write() 가 호출되지 않아야 한다
```

### AC-C4: CenturyNode 통합 — 제어 키 감지 시 not_supported

```gherkin
Given century 통합 노드가 초기화되었을 때

When  제어 키가 없는 빈 메시지 {} 가 Process 에 전달되면
Then  상태 조회 결과(get_stats 또는 get_recent 응답)가 반환되어야 한다

When  제어 키 ("power" / "mode" / "temperature" / "setpoint" / "fan_speed") 중 하나라도 포함된 메시지가 전달되면
Then  AC-C3 와 동일한 not_supported 응답이 반환되어야 한다
```

### AC-C5: CenturyRawFrameNode — 모든 프레임 노출

```gherkin
Given century-raw-frame 노드가 agent_ref="my-century" 로 설정되었을 때
When  CenturyAgent 가 1 개의 유효 reg 0x02 응답과 1 개의 CRC 불일치 프레임을 수신하면
Then  out 포트에 2 개의 century_raw_frame 메시지가 송출되어야 한다
And   유효 프레임 메시지의 validation_stage == "ok", crc_ok == true
And   CRC 불일치 프레임 메시지의 validation_stage == "crc", crc_ok == false
And   각 메시지의 raw 필드에 헤더 + payload + CRC 전체 바이트가 포함되어야 한다
And   confirmation_status 가 "raw" 로 설정되어야 한다
And   payload 의 timestamp 가 int64 epoch milliseconds 이어야 한다
```

### AC-C6: 에이전트 타입 불일치 검증

```gherkin
Given agent_ref 가 LGCNP 에이전트(lgcnp 타입)를 가리키는 경우
When  century-status 노드의 initAgent 가 호출되면
Then  ErrCenturyAgentNotCentury 에러가 반환되어야 한다
```

### AC-C7: Init-tolerance — deferred connection

```gherkin
Given century-status 노드가 agent_ref="future-century" 로 설정되었으나 해당 에이전트가 아직 매니저에 등록되지 않은 경우
When  노드의 Init 이 호출되면
Then  hard-fail 하지 않고 경고(WARNING) 로그를 출력해야 한다
And   노드 상태가 Running 으로 전이되어야 한다 (deferred connection)

When  추후 "future-century" 에이전트가 활성화되고 ReinitNodesForAgent 가 호출되면
Then  노드가 자동으로 재초기화되어 에이전트와 연결되어야 한다

When  AgentResolver 자체가 설정되지 않은 구성 오류인 경우
Then  Init 이 hard-fail (에러 반환) 해야 한다 (SPEC-SERIAL-001 REQ-SERIAL-016 와 동일 정책)
```

### AC-C8: CenturyStatusNode FrameNotifyCh 즉시 반응

```gherkin
Given century-status 노드의 poll_interval=10s (긴 폴링) 로 설정되고 FrameNotifyCh 가 연결된 상태일 때
When  CenturyAgent 의 ring buffer 에 새 프레임이 push 되고 FrameNotifyCh 에 신호가 발생하면
Then  10s 폴링 타이머를 기다리지 않고 즉시 폴링을 수행하여 out 포트로 메시지를 송출해야 한다
```

---

## 그룹 D: 설정 및 등록

### AC-D1: 에이전트 타입 등록

```gherkin
Given agent.DefaultManager 가 초기화되고
When  century.RegisterCenturyTypes(mgr) 가 호출되면
Then  "century-hvac" 타입이 매니저에 등록되어야 한다
And   해당 타입으로 에이전트 인스턴스 생성이 가능해야 한다
And   잘못된 타입명("century")은 등록되지 않아야 한다
```

### AC-D2: 필수 필드 누락 — clear error

```gherkin
Given transport.options 에 serial_port 가 누락된 YAML 설정
When  CenturyAgent.Init 이 호출되면
Then  ErrSerialPortRequired 에러가 반환되어야 한다
And   에러 메시지에 "serial_port is required" 가 포함되어야 한다
And   에이전트가 Init 상태에 머물러야 한다 (Running 으로 전이 X)
```

### AC-D3: hex 입력 허용

```gherkin
Given transport.options 에 다음이 설정된 경우:
      master_address: "0x0030"
      slave_address: "0x0001"
      sub_dev_id: "0x3B"
When  parseCenturyConfig 가 호출되면
Then  파싱이 성공해야 한다
And   master_address=48, slave_address=1, sub_dev_id=59 로 정규화되어야 한다

When  사용자가 정수 형태로도 입력하면 (master_address: 48):
Then  동일하게 파싱되어야 한다
```

### AC-D4: read_timeout 클램프

```gherkin
Given transport.options.read_timeout="0s" 설정
When  parseCenturyConfig 가 호출되면
Then  read_timeout 이 200ms 로 클램프되어야 한다 (SPEC-SERIAL-001 v2.3.0 패턴)
```

### AC-D5: Web UI 에이전트 스키마

```gherkin
Given agentSchemas.ts 의 AGENT_TYPES 배열을 검사할 때
Then  {value:"century-hvac", label:"Century HVAC (passive)"} 가 포함되어야 한다

When  사용자가 UI 에서 century-hvac 타입을 선택하면
Then  CENTURY_HVAC_FIELDS 의 모든 필드가 폼에 노출되어야 한다
And   transport_type 기본값이 "serial" 이어야 한다
And   transport_type="serial" 일 때 serial_port/baud_rate/data_bits/stop_bits/parity 필드가 visible
And   master_address 기본값이 "0x0030", sub_dev_id 기본값이 "0x3B" 이어야 한다
And   offline_timeout 기본값이 "5s" 이어야 한다
And   ring_buffer_size 기본값이 128 이어야 한다
```

### AC-D6: Web UI 노드 스키마

```gherkin
Given nodeSchemas.ts 의 노드 타입 목록을 검사할 때
Then  "century-status", "century-control", "century", "century-raw-frame" 4 개 노드가 등록되어야 한다
And   각 노드의 카테고리가 "io" (또는 raw-frame 은 "io"/"debug")
And   각 노드의 agent_select 옵션에 "century-hvac" 가 포함되어야 한다

When  century-control 노드를 폼에서 선택하면
Then  설명에 "패시브 전용" 또는 "미지원" 문구가 표시되어야 한다
```

### AC-D7: cmd/xflowd 부트스트랩 통합

```gherkin
Given cmd/xflowd/main.go 의 에이전트 매니저 초기화 코드를 검사할 때
Then  century.RegisterCenturyTypes(agentMgr) 호출이 포함되어야 한다
And   호출 위치는 samsung.RegisterSamsungNASATypes / lg.RegisterLGCNPTypes 인근이어야 한다

When  xflowd 가 century-hvac 에이전트를 포함한 설정 파일로 부팅되면
Then  부팅이 성공해야 하고 에이전트 매니저에 등록되어야 한다
```

---

## 그룹 E: 필드 디코딩 정책

### AC-E1: confirmation_status 마커 노출

```gherkin
Given 임의의 디코딩된 century_reg02_response / reg03_response / reg04_response / reg04_write_request 메시지
When  payload 를 검사할 때
Then  fields 객체 내의 모든 typed 필드는 다음 셋을 노출해야 한다:
      - value: 의미적 값 (숫자 또는 문자열)
      - raw: 원시 바이트 (u8 또는 u16)
      - confirmation_status: "confirmed" / "inferred" / "unknown" 중 하나

And   confirmation_status 값은 다음 분포를 보여야 한다:
      - reg 0x02: mode/fan/setpoint_c → confirmed; live 13/14/15 → inferred; 나머지 → unknown
      - reg 0x03: temp_evap_a/b → confirmed; pad_4..15 → unknown
      - reg 0x04 response: status_bits/const_1/2/7/op_val_1/2/temp_A → inferred; byte_3..6 → unknown
      - reg 0x04 write: mode_cmd → confirmed; write_live_0/1/15, write_byte_14 → inferred; 나머지 → unknown
```

### AC-E2: additive mode/mode_cmd enum — 미관측 코드 처리

```gherkin
Given CAP-3 reg 0x02 응답을 가공하여 data[1] (mode byte) 를 0x02 로 수정한 합성 프레임
When  decodeReg02Response 가 호출되면
Then  fields.mode.value == "mode_unknown_02" 이어야 한다
And   fields.mode.raw == 2
And   fields.mode.confirmation_status == "unknown"
And   디코더가 에러 없이 정상 처리되어야 한다 (전체 페이로드가 송출됨)
And   unconfirmedFieldObservations 카운터가 증가해야 한다
And   log_unconfirmed_fields=true 일 때만 DEBUG 로그가 출력되어야 한다

When  data[1] 을 0x05 등 다른 미관측 값으로 변경해도 동일하게 동작해야 한다 (additive)
```

### AC-E3: 향후 SPEC 갱신 시 호환성 — 필드명 안정성

```gherkin
Given downstream 컨슈머가 fields.write_live_0 필드를 참조하는 플로우를 운영 중일 때
When  SPEC-CENTURY-001 v0.2.0 이 발표되어 write_live_0 의 의미가 "compressor_freq_index" 로 확정되면
Then  새 버전의 payload 는 fields.compressor_freq_index (신규) 와 fields.write_live_0 (deprecated alias) 를 모두 노출해야 한다
And   두 필드의 value 가 동일해야 한다
And   downstream 플로우가 수정 없이 계속 동작해야 한다 (REQ-CENTURY-026)
```

### AC-E4: timestamp 컨벤션 (epoch milliseconds)

```gherkin
Given 임의의 century_* 메시지가 송출될 때
When  payload.timestamp 필드를 검사하면
Then  타입이 int64 또는 JSON number 이어야 한다
And   값이 time.Now().UnixMilli() 와 유사한 범위(1.7e12 수준) 이어야 한다
And   time.Time 또는 RFC3339 문자열 형식이 아니어야 한다
(프로젝트 컨벤션: project_timestamp_convention.md)
```

### AC-E5: raw_hex 노출

```gherkin
Given 디코딩된 status 메시지가 송출될 때
When  payload.raw_hex 필드를 검사하면
Then  헤더 + payload + CRC 전체 바이트의 hex 인코딩 문자열이어야 한다
And   문자열 길이가 2 * (10 + payload_length) 이어야 한다
And   downstream 파이프라인에서 hex.DecodeString 으로 round-trip 가능해야 한다
```

---

## 그룹 F: WRITE 중복 처리 (REQ-CENTURY-027)

### AC-F1: dedupe_writes=true — 동일 cycle 내 중복 1개로 합침

```gherkin
Given dedupe_writes=true (기본값) 의 CenturyAgent 가 동작 중이고
  And century-status 노드와 century-raw-frame 노드가 각각 연결된 상태일 때
When  동일 polling cycle 내에서 동일한 raw payload 를 가진 두 개의 WRITE reg 0x04 프레임이 회선에서 수신되면
Then  decoded event 카운트는 1 이어야 한다 (century_reg04_write_request 메시지 1개만 century-status 노드 out 으로 emit)
And   raw frame 노드 out 의 century_raw_frame 메시지 카운트는 2 이어야 한다 (dedup 과 무관하게 모든 frame emit)
And   writesDeduped 카운터가 1 증가해야 한다
And   reg04WriteCount 카운터는 1 만 증가해야 한다 (dedup 된 frame 은 디코딩 카운트에 포함되지 않음)
```

### AC-F2: dedupe_writes=false — 모든 WRITE emit

```gherkin
Given dedupe_writes=false 의 CenturyAgent 가 동작 중일 때
When  동일 polling cycle 내에서 동일한 raw payload 를 가진 두 개의 WRITE reg 0x04 프레임이 수신되면
Then  decoded event 카운트가 2 이어야 한다 (century_reg04_write_request 메시지 2개 emit)
And   raw frame 노드 out 의 century_raw_frame 메시지 카운트도 2
And   writesDeduped 카운터가 증가하지 않아야 한다 (0 유지)
And   reg04WriteCount 카운터가 2 증가해야 한다
```

### AC-F3: cycle 경계 넘어가면 dedup 무효 (reg 0x04 응답 마커)

```gherkin
Given dedupe_writes=true 의 CenturyAgent 가 동작 중일 때
When  다음 순서로 frame 이 수신되면:
      1. WRITE reg 0x04 (payload X)
      2. ACK
      3. READ reg 0x02 response  (← 새 cycle 시작 신호 후보)
      4. READ reg 0x03 response
      5. READ reg 0x04 response  (← 1차 cycle 경계 마커 / 마지막 READ response)
      6. WRITE reg 0x04 (payload X — 1번과 동일 raw payload)
Then  decoded event 카운트가 2 이어야 한다 (cycle 이 바뀌었으므로 dedup 되지 않음)
And   writesDeduped 카운터가 증가하지 않아야 한다
And   cycleTracker 가 5번 frame 이후 새 cycle 로 전이했음을 내부 상태로 보유해야 한다
```

### AC-F4: inter-frame idle > 100ms — 새 cycle 로 간주 (2차 fallback)

```gherkin
Given dedupe_writes=true 의 CenturyAgent 가 동작 중이고
  And reg 0x04 응답 마커가 도착하지 않는 비정형 회선 상황을 시뮬레이션할 때
When  다음 시퀀스가 발생하면:
      1. WRITE reg 0x04 (payload X)
      2. 150ms idle (frame 없음)
      3. WRITE reg 0x04 (payload X — 1번과 동일 raw payload)
Then  두 번째 frame 이 새 cycle 로 간주되어 dedup 되지 않아야 한다
And   decoded event 카운트가 2 이어야 한다
And   writesDeduped 카운터가 증가하지 않아야 한다

When  idle 이 50ms 인 동일 시퀀스가 발생하면 (100ms 임계값 미만)
Then  두 번째 frame 이 dedup 되어 decoded event 카운트가 1 이어야 한다
And   writesDeduped 카운터가 1 증가해야 한다
```

---

## 그룹 G: TCP transport (v0.2.0, REQ-CENTURY-028 ~ REQ-CENTURY-032)

### AC-G1: TCP-client connect — 정상 수신 + AC-B9 불변식

```gherkin
Given transport_type="tcp-client", tcp_host="192.168.1.100", tcp_port=4196 의 CenturyAgent 설정과
  And mock TCP 서버(test net.Listener) 가 CAP-3 의 raw 바이트 (한 cycle 9 프레임) 를 stream 으로 송신할 준비된 상태일 때
When  agent.Start 가 호출되고 mock 서버가 연결을 accept 하여 raw 바이트를 push 하면
Then  capture loop 가 frame scanner 를 통해 모든 9 프레임을 디코딩해야 한다
And   sub_dev_id=0x3B device 가 자동 발견되어야 한다 (AC-B1a 와 동일 동작)
And   디코딩 결과가 serial 모드와 비트 단위로 동일해야 한다 (CAP-3 골든 픽스처 회귀)
And   mock 트랜스포트의 WriteCount == 0 이어야 한다 (AC-B9 invariant under TCP)
And   transportConnected() == true 이어야 한다
```

### AC-G2: TCP-client dial failure → exponential backoff

```gherkin
Given transport_type="tcp-client" 이고 tcp_host 가 unreachable (예: 127.0.0.1:1 또는 missing local listener) 인 설정과
  And reconnect_initial=100ms, max_reconnect_backoff=1s (테스트용 짧은 값), tcp_connect_timeout=50ms 일 때
When  agent.Start 가 호출되면
Then  첫 dial 시도가 실패해야 한다 (ConnectionRefused 또는 timeout)
And   ErrCenturyTCPDialFailed wrapping 된 에러가 로그에 기록되어야 한다
And   capture loop 가 100ms backoff 후 재시도해야 한다
And   두 번째 실패 후 backoff 가 200ms 로 증가
And   세 번째 실패 후 400ms, 네 번째 800ms, 다섯 번째 1s (cap)
And   다섯 번째 이후 모든 backoff 는 1s 로 유지되어야 한다 (max cap)

When  agent.Stop 이 호출되면
Then  backoff sleep 도중이라도 즉시 종료되어야 한다 (context cancel)
```

### AC-G3: TCP-client reconnect after disconnect

```gherkin
Given transport_type="tcp-client" 의 CenturyAgent 가 mock 서버에 연결되어 frame 을 수신 중일 때
  And reconnect_initial=100ms 의 짧은 backoff 설정
When  mock 서버가 연결을 의도적으로 close 하면 (io.EOF)
Then  capture loop 가 io.EOF 를 감지해야 한다
And   ring buffer 의 기존 frame 은 보존되어야 한다 (디바이스 상태 stale 표시)
And   100ms backoff 후 재연결을 시도해야 한다

When  mock 서버가 listener 를 재시작하고 두 번째 accept 가 성공하면
Then  capture loop 가 새 연결로 정상 복귀해야 한다
And   재연결 직후 수신되는 첫 frame 이 정상 디코딩되어야 한다
And   backoff timer 가 reconnect_initial (100ms) 로 리셋되어야 한다
And   device 가 offline 상태였다면 다음 frame 수신 시 online 으로 복귀해야 한다
```

### AC-G4: TCP-client read timeout

```gherkin
Given transport_type="tcp-client", tcp_read_timeout=500ms 의 CenturyAgent 가 mock 서버에 연결된 상태일 때
When  mock 서버가 연결만 유지하고 500ms 동안 어떤 byte 도 송신하지 않으면
Then  capture loop 의 read 가 net.Error.Timeout() 으로 timeout 해야 한다
And   해당 연결이 close 되어야 한다
And   AC-G3 와 동일한 재연결 흐름이 트리거되어야 한다 (reconnect_initial 100ms 부터)

When  mock 서버가 timeout 이내에 frame 의 첫 byte 라도 송신하면
Then  read 가 timeout 되지 않아야 한다 (SetReadDeadline 이 매 read 직전 갱신됨)
```

### AC-G5: TCP-server listen + accept

```gherkin
Given transport_type="tcp-server", tcp_host="127.0.0.1", tcp_port=0 (OS 할당) 의 CenturyAgent 설정 (테스트용)
When  agent.Start 가 호출되면
Then  net.Listen 이 성공해야 한다
And   transportConnected() 가 listener 활성 상태를 반영해야 한다 (또는 활성 연결 없음 상태 노출)

When  외부 클라이언트(test net.Dialer) 가 해당 endpoint 로 연결하고 CAP-3 cycle 의 raw 바이트를 push 하면
Then  agent 가 accept 한 연결에서 모든 9 프레임을 디코딩해야 한다
And   sub_dev_id=0x3B device 가 자동 발견되어야 한다
And   디코딩 결과가 serial / tcp-client 모드와 동일해야 한다
And   mock connection 의 WriteCount == 0 이어야 한다 (AC-B9 invariant under TCP-server)
```

### AC-G6: TCP-server second connection rejected

```gherkin
Given transport_type="tcp-server" 의 CenturyAgent 가 동작 중이고
  And 첫 클라이언트가 이미 연결되어 frame 을 push 중일 때
When  두 번째 클라이언트가 같은 endpoint 로 연결을 시도하면
Then  agent 가 두 번째 연결을 즉시 close 해야 한다
And   INFO 레벨 로그 "tcp-server: rejected secondary connection from <peer>" 가 출력되어야 한다
And   첫 클라이언트의 frame 수신은 영향받지 않고 계속되어야 한다
And   activeConnections 카운터(stats) 가 1 을 유지해야 한다

When  첫 클라이언트가 연결을 close 하고 두 번째 클라이언트가 다시 연결하면
Then  두 번째 클라이언트의 연결이 정상 accept 되어야 한다 (listener 는 유지)
And   디바이스 상태가 새 연결에서 정상 갱신되어야 한다
```

### AC-G7: Transport-aware cycle_idle_timeout default

```gherkin
Given transport_type="serial" 의 YAML 설정에 cycle_idle_timeout 이 명시되지 않은 경우
When  parseCenturyConfig 가 호출되면
Then  config.CycleIdleTimeout == 100ms 이어야 한다 (serial 기본값)
And   cycle tracker 가 100ms idle gap 을 cycle 경계로 사용해야 한다 (REQ-CENTURY-027 회귀)

Given transport_type="tcp-client" 의 YAML 설정에 cycle_idle_timeout 이 명시되지 않은 경우
When  parseCenturyConfig 가 호출되면
Then  config.CycleIdleTimeout == 200ms 이어야 한다 (tcp 기본값)
And   cycle tracker 가 200ms idle gap 을 cycle 경계로 사용해야 한다

Given transport_type="tcp-server" 의 YAML 설정에 cycle_idle_timeout 이 명시되지 않은 경우
When  parseCenturyConfig 가 호출되면
Then  config.CycleIdleTimeout == 200ms 이어야 한다 (tcp 기본값)

Given 임의의 transport_type 의 YAML 설정에 cycle_idle_timeout="150ms" 가 명시된 경우
When  parseCenturyConfig 가 호출되면
Then  config.CycleIdleTimeout == 150ms 이어야 한다 (사용자 명시 우선)
And   transport 와 무관하게 동일 값이 적용되어야 한다
```

### AC-G8: AC-B9 invariant under TCP (모든 G 시나리오 통합 검증)

```gherkin
Given AC-G1, AC-G3, AC-G4, AC-G5, AC-G6 의 모든 시나리오를 실행 중일 때
  And 각 시나리오의 transport wrapper 는 mock 으로 Write 호출을 카운트하도록 instrumented 되어 있을 때
When  각 시나리오가 완료된 시점에서 mock wrapper 의 WriteCount 를 검사하면
Then  모든 시나리오에서 WriteCount == 0 이어야 한다
And   tcp-client wrapper 의 Write 메서드가 호출되면 ErrTransportPassiveOnly 를 반환해야 한다 (방어적 구현)
And   tcp-server 의 accept 된 net.Conn 에도 Write 호출이 0회여야 한다
And   본 시나리오는 AC-B9 의 TCP 확장이며 v0.1.2 의 AC-B9 가 serial 에서 보장하던 불변식과 동일 정책

When  본 검증을 CI 에서 항상 실행하면
Then  TCP transport 도입으로 인한 회귀 방지가 보장되어야 한다 (passive sniff 정책 유지)
```

---

## 그룹 H: Device-centric output (v0.3.0 → v0.4.2, REQ-CENTURY-033 ~ REQ-CENTURY-037, **Breaking x2**)

> **v0.4.0 schema migration**: device_state 의 5 핵심 + online 은 nested `state` 그룹. AC 의 모든 access 는 `payload.state.<field>` 로 마이그레이션.
> **v0.4.2 strict gate**: 첫 emit 은 Reg02 AND Reg04Read 모두 수신 후에만 발생 (이전 single-register fallback 정책 제거).

### AC-H1: emit_device_state=true + emit_register_decoded=false (v0.3.0 default) — register-decoded 차단

```gherkin
Given emit_device_state=true (default), emit_register_decoded=false (v0.3.0 default) 의 CenturyAgent 가 동작 중일 때
When  CAP-3 한 cycle (9 프레임 — reg 0x02/0x03/0x04 read + 0x04 write + ACK) 이 회선에서 수신되면
Then  msgCh 에서 관찰된 모든 메시지의 payload 의 type 필드가 "device_state" 이어야 한다
And   "century_reg02_response" / "century_reg03_response" / "century_reg04_response" / "century_reg04_write_request" / "century_ack" 타입의 메시지는 0 회 관찰되어야 한다
And   dev_id="0x3B" device 의 device_state 메시지가 적어도 1회 emit 되어야 한다 (Reg02 AND Reg04 도착 후 trigger="change")
```

### AC-H2: FirstEmitAfterReg02AndReg04 — Reg02 + Reg04Read 모두 수신 후 첫 emit (v0.4.2 strict gate)

```gherkin
Given emit_device_state=true 인 CenturyAgent 가 동작 중이고 devices 맵이 비어있을 때
When  CAP-3 의 reg 0x02 응답 단 1개 frame 만 수신되면 (Reg04Read 미수신)
Then  msgCh 에 device_state 메시지가 0회 emit 되어야 한다 (Reg02 AND Reg04 gate 미통과)

When  이어서 CAP-3 의 reg 0x04 read response (current_temp=25.2℃) 가 수신되면
Then  msgCh 에 device_state 메시지가 정확히 1회 emit 되어야 한다 (gate 통과, 통합 첫 emit)
And   payload.type == "device_state"
And   payload.dev_id == "0x3B"
And   payload.label == "indoor-3b"
And   payload.state.online == true
And   payload.state.power == true (mode != 0x00)
And   payload.state.mode == "cool"
And   payload.state.fan_speed == 17
And   payload.state.target_temp == 25.0
And   payload.state.current_temp == 25.2
And   payload.trigger == "change" (첫 emit)
And   payload.timestamp_ms 와 payload.last_seen_ms 는 epoch ms (int64) 이어야 한다
And   payload 에 evap_temp_a_c / evap_temp_b_c 필드가 노출되지 않아야 한다 (v0.3.1 cleanup)
```

### AC-H3: register 0x02 + 0x04 read 통합 1회 emit (v0.4.2 — 이전 "후속 reg04 update" 의미 갱신)

```gherkin
Given emit_device_state=true 인 CenturyAgent 가 동작 중일 때
When  Reg02 와 Reg04Read frame 이 동일 cycle 내 수신되면
Then  msgCh 에 device_state 메시지가 정확히 1회 emit 되어야 한다 (Reg02 단독 emit 후 Reg04 추가 emit 의 2회 패턴이 아닌 통합 1회)
And   payload.state.mode / state.fan_speed / state.target_temp 는 Reg02 데이터로 채워져야 한다
And   payload.state.current_temp 는 Reg04Read 데이터 (25.2) 로 채워져야 한다
And   payload.trigger == "change"

When  후속 동일 cycle 에서 current_temp 가 25.5 로 변경된 Reg04Read frame 이 수신되면
Then  device_state 메시지가 1회 추가 emit 되어야 한다 (change detector 트리거)
And   payload.state.current_temp == 25.5
```

### AC-H4: 같은 값의 frame 연속 수신 — emit 1회

```gherkin
Given emit_device_state=true 인 CenturyAgent 가 동작 중이고
  And AC-H2 의 Reg02 + Reg04 통합 emit 이 발생한 상태일 때
When  동일한 raw bytes 의 Reg02 + Reg04 frame 이 두 번 더 연속 수신되면 (5 핵심 필드 모두 동일)
Then  추가 device_state emit 이 발생하지 않아야 한다 (5 핵심 필드 모두 unchanged)
And   msgCh 의 device_state 메시지 누적 카운트가 AC-H2 의 시점에서 변하지 않아야 한다
```

### AC-H5: mode 0x00 → 0x01 전이 — power false → true change emit

```gherkin
Given emit_device_state=true 인 CenturyAgent 가 동작 중일 때
When  CAP-1 의 reg 0x02 응답 (mode=0x00 / off) + reg 0x04 read response 가 수신되어 첫 통합 emit 발생 (v0.4.2 gate)
Then  payload.state.power == false, payload.state.mode == "off"
And   payload.trigger == "change"

When  이어서 CAP-3 의 reg 0x02 응답 (mode=0x01 / cool) 이 수신되면
Then  추가 device_state 메시지가 1회 emit 되어야 한다
And   payload.state.power == true (false → true 전이)
And   payload.state.mode == "cool" ("off" → "cool" 전이)
And   payload.trigger == "change"
```

### AC-H6: keepalive_interval=2s — 변경 없이 3s 경과 후 keepalive emit

```gherkin
Given emit_device_state=true, keepalive_interval=2s, keepalive_mode="relative" (default) 인 CenturyAgent 가 동작 중이고
  And AC-H2 의 첫 통합 emit 으로 lastKeepaliveTime[0x3B] 가 t0 anchor 로 마킹된 상태일 때
When  t0 이후 변경 없이 3s 가 경과하면 (frame 미수신 또는 동일 값 frame 만 수신)
Then  msgCh 에 device_state 메시지가 1회 추가 emit 되어야 한다 (keepalive ticker 가 1s 주기로 깨어남)
And   payload.trigger == "keepalive"
And   payload.state 의 5 핵심 필드는 마지막 change emit 시점의 값과 동일
And   lastKeepaliveTime[0x3B] 가 keepalive emit 시점으로 갱신되어야 한다 (lastEmitTime 와 별개, v0.3.10)
```

### AC-H7: device online → offline 전이 — 즉시 trigger="change" emit (v0.4.2 gate 통과 device 만)

```gherkin
Given emit_device_state=true, offline_timeout=200ms (테스트용) 인 CenturyAgent 가 동작 중이고
  And dev_id="0x3B" device 가 Reg02 AND Reg04Read 모두 수신하여 첫 device_state emit 이 발생한 상태일 때
When  200ms 이상 dev_id="0x3B" 의 frame 이 수신되지 않으면
Then  offlineWatchLoop 이 device.Online 을 false 로 전이시켜야 한다
And   msgCh 에 device_state 메시지가 즉시 1회 추가 emit 되어야 한다 (gate 통과 device 이므로 emit)
And   payload.state.online == false
And   payload.trigger == "change"
And   payload.state 의 5 핵심 필드는 마지막 알려진 값을 유지 (stale snapshot)
```

### AC-H8: emit_device_state=false + emit_register_decoded=true — register-only mode (v0.2.x 호환 + v0.4.0 type 필드)

```gherkin
Given emit_device_state=false, emit_register_decoded=true 의 CenturyAgent 가 동작 중일 때
When  CAP-3 한 cycle 의 9 frame 이 수신되면
Then  msgCh 에 v0.2.x 와 동일한 register-decoded 메시지가 emit 되어야 한다
And   각 메시지의 top-level "type" 필드가 다음 중 하나여야 한다 (v0.4.0, REQ-CENTURY-037):
      "century_reg02_response" / "century_reg03_response" / "century_reg04_response" / "century_reg04_write_request" / "century_ack"
And   device_state 메시지는 0 회 emit 되어야 한다
And   lastEmitState / lastEmitTime / lastKeepaliveTime / keepaliveLoop / deviceStateBuf 가 활성화되지 않아야 한다
```

### AC-H9: emit_device_state=false + emit_register_decoded=false — ErrCenturyNoOutputEnabled

```gherkin
Given YAML 설정에 emit_device_state=false, emit_register_decoded=false 가 모두 명시된 경우
When  parseCenturyConfig 가 호출되면
Then  ErrCenturyNoOutputEnabled 에러가 반환되어야 한다
And   에러 메시지에 "최소 하나의 output stream 활성화 필요" 또는 동등한 안내가 포함되어야 한다
And   에이전트가 Init 단계에서 실패하여 Running 으로 전이하지 않아야 한다
```

### AC-H10: 다중 IDU (dev_id="0x3B" + "0x3C") — 독립 device_state stream (v0.4.2 gate 적용)

```gherkin
Given emit_device_state=true, auto_discovery=true 인 CenturyAgent 가 동작 중일 때
When  dev_id="0x3B" 의 Reg02 + Reg04Read frame (mode=cool) 이 모두 수신되면
Then  device_state 메시지가 1회 emit 되어야 한다 (dev_id="0x3B", trigger="change")

When  이어서 dev_id="0x3C" 의 Reg02 + Reg04Read frame (mode=cool, 합성 frame) 이 모두 수신되면
Then  device_state 메시지가 1회 추가 emit 되어야 한다 (dev_id="0x3C", trigger="change")
And   0x3B 의 lastEmitState 와 0x3C 의 lastEmitState 는 별개 key 로 보유되어야 한다

When  dev_id="0x3B" 의 후속 frame 이 동일 값으로 들어오면
Then  0x3B 의 emit 은 발생하지 않아야 한다 (unchanged)
And   0x3C 의 lastKeepaliveTime 은 영향받지 않아야 한다

When  dev_id="0x3B" 만 offline 으로 전이하면 (offline_timeout 경과)
Then  0x3B 의 device_state 메시지가 trigger="change", state.online=false 로 emit 되어야 한다
And   0x3C 는 state.online=true 상태를 유지하고 emit 트리거되지 않아야 한다
```

### AC-H11: keepalive_mode="absolute" — wall-clock 정렬 (v0.3.9, crontab 패턴)

```gherkin
Given emit_device_state=true, keepalive_interval=1m, keepalive_mode="absolute" 인 CenturyAgent 가 동작 중이고
  And t0 = "2026-05-19T10:00:23Z" 시각에 첫 통합 emit 이 발생하여 lastKeepaliveTime[0x3B] = t0 으로 anchored 된 상태일 때

When  t1 = "2026-05-19T10:00:59Z" (interval 미경과, 다음 정렬 시점 t="10:01:00" 미도달) 에 keepalive ticker 가 깨어나면
Then  shouldKeepaliveFire(t1, t0, 1m, "absolute") == false 이어야 한다 (now.Truncate(1m)="10:00:00", t0 이후이지만 anchor 시점이 t1 의 Truncate 보다 늦지 않음)
And   추가 emit 이 발생하지 않아야 한다

When  t2 = "2026-05-19T10:01:00Z" (다음 정렬 시점 도달) 에 keepalive ticker 가 깨어나면
Then  shouldKeepaliveFire(t2, t0, 1m, "absolute") == true 이어야 한다
And   trigger="keepalive" device_state 메시지가 1회 emit 되어야 한다
And   lastKeepaliveTime[0x3B] 가 t2 로 갱신되어야 한다

Given 또한 helper unit test 가 12 cases (invalid interval, relative 경계, default/unknown mode fallback, absolute 1m/5m/1h 정렬) 를 통과해야 한다
And   parseCenturyConfig 가 `keepalive_mode: "absolute"` / `"relative"` / 빈 string (default) 을 수용하고, 기타 값 (e.g. "crontab") 은 `fmt.Errorf("invalid keepalive_mode")` 반환
```

### AC-H12: lastKeepaliveTime 분리 — change 빈도와 무관한 keepalive (v0.3.10 root cause fix)

```gherkin
Given emit_device_state=true, keepalive_interval=300ms 인 CenturyAgent 가 동작 중일 때
When  매 80ms 마다 mode alternating frame (cool ↔ off, Reg02 + Reg04 페어) 이 1500ms 동안 주입되면 (지속적 change)
Then  change emit 이 매번 발생하더라도 keepalive 가 최소 1회 fire 되어야 한다 (lastKeepaliveTime 이 change emit 으로 갱신되지 않으므로)
And   payload.trigger=="keepalive" 메시지가 적어도 1회 관찰되어야 한다

Given 또한 no-change 시나리오에서:
When  AC-H6 의 환경 (변경 없이 keepalive_interval 경과) 이 그대로 재현되면
Then  AC-H6 과 동일하게 keepalive 가 fire 되어야 한다 (no-change 환경에서도 정상 동작 — non-breaking)
```

### AC-H13: deviceStateBuf — drop-oldest semantics (v0.3.11)

```gherkin
Given emit_device_state=true, ring_buffer_size=4 (테스트용, deviceStateBuf cap=2) 인 CenturyAgent 가 동작 중일 때
When  서로 다른 device_state event 3개가 연속 emit 되면 (e.g. mode/fan/setpoint 가 매번 변하는 Reg02+Reg04 페어)
Then  deviceStateBuf 는 가장 최신 2개 event 만 보유해야 한다 (가장 오래된 1개는 drop)
And   drain_device_state Process command 호출 시 응답이 `{"count":2, "events":[<event2>, <event3>]}` 이어야 한다 (FIFO 순서)
And   두 번째 drain 호출 시 `{"count":0, "events":[]}` 이어야 한다 (비파괴 drain 후 buffer 비어있음)
And   drop 자체는 별도 카운터에 누적되지 않아야 한다 (msgCh 의 best-effort drop 정책과 일관, A18)
```

### AC-H14: drain_device_state Process command — polling 노드 forward (v0.3.11)

```gherkin
Given emit_device_state=true 인 CenturyAgent 와 century-status 노드가 연결된 상태일 때
When  Reg02 + Reg04Read frame 이 수신되어 device_state event 가 emit 되면
Then  msgCh 로 1회 (Bridge 컨슈머용) + deviceStateBuf 로 1회 (polling 노드용) 이중 publish 되어야 한다

When  century-status 노드의 pollLoop 가 매 ticker tick 마다 `drain_device_state` Process command 를 호출하면
Then  centuryNodeBase.drainDeviceStateEvents helper 가 buffer 의 모든 event 를 sourceCh 로 forward 해야 한다
And   각 forward 된 메시지의 metadata 에 `century_source="device_state"` 가 포함되어야 한다
And   msgCh Bridge 컨슈머와 polling 노드 두 경로가 비경쟁으로 동일 event 를 관측해야 한다

Given 회귀 테스트 `TestProcessDrainDeviceState_BasicFlow`:
When  change emit + keepalive emit 두 종류가 모두 buffer 에 있는 상태에서 drain 하면
Then  응답에 두 종류 모두 포함되어야 하며, 두 번째 drain 시 빈 응답이어야 한다
```

### AC-H15: Reg02-only no emit + Reg04-only no emit — Reg02 AND Reg04 strict gate (v0.4.2)

```gherkin
Given emit_device_state=true 인 CenturyAgent 가 동작 중이고 devices 맵이 비어있을 때

When  CAP-3 의 reg 0x02 응답 (Reg02 만, Reg04Read 없음) 이 수신되면
Then  msgCh 에 device_state 메시지가 0회 emit 되어야 한다 (gate 미통과)
And   회귀 테스트 TestAgent_DeviceStateGatedByReg04 가 이를 검증

When  대칭적으로 CAP-3 의 reg 0x04 read response (Reg04Read 만, Reg02 없음) 가 수신되면
Then  msgCh 에 device_state 메시지가 0회 emit 되어야 한다 (gate 미통과)
And   회귀 테스트 TestAgent_DeviceStateGatedByReg02 가 이를 검증 (v0.4.1 부터 존재)

When  Reg02 또는 Reg04Read 중 누락된 한 쪽이 추가로 수신되어 두 register 가 모두 관측되면
Then  device_state 메시지가 정확히 1회 emit 되어야 한다 (gate 통과, 정상 통합 첫 emit — AC-H2 와 일관)
And   payload.state.mode / state.fan_speed / state.target_temp 는 Reg02 데이터 (정상 power/mode 노출)
And   payload.state.current_temp 는 Reg04Read 데이터 (잘못된 0.0 fallback 없음)
And   payload.trigger == "change"
```

---

## 품질 게이트 (Definition of Done)

### 필수 통과 조건

- [ ] 그룹 A (프레임 디코딩) 의 모든 acceptance 통과 (AC-A1 ~ AC-A11)
- [ ] 그룹 B (에이전트 런타임) 의 모든 acceptance 통과 (AC-B1a, AC-B1b, AC-B2 ~ AC-B10)
- [ ] 그룹 C (플로우 노드) 의 모든 acceptance 통과 (AC-C1 ~ AC-C8)
- [ ] 그룹 D (설정 및 등록) 의 모든 acceptance 통과 (AC-D1 ~ AC-D7)
- [ ] 그룹 E (필드 디코딩 정책) 의 모든 acceptance 통과 (AC-E1 ~ AC-E5)
- [ ] 그룹 F (WRITE 중복 처리) 의 모든 acceptance 통과 (AC-F1 ~ AC-F4)
- [ ] 그룹 G (TCP transport, v0.2.0) 의 모든 acceptance 통과 (AC-G1 ~ AC-G8) — M6 구현 후
- [x] 그룹 H (Device-centric output, v0.3.0 → v0.4.2) 의 모든 acceptance 통과 (AC-H1 ~ AC-H15) — M7 + 11 hotfix 사이클 완료
- [ ] v0.2.0 추가 항목: REQ-CENTURY-028 ~ REQ-CENTURY-032 모두 Implemented (M6 PENDING)
- [x] v0.3.0 ~ v0.4.2 추가 항목: REQ-CENTURY-033 ~ REQ-CENTURY-037 모두 Implemented (**Breaking x2** — Migration: v0.3 register-decoded → v0.4 state group + type 필드, gate Reg02 AND Reg04)
- [ ] AC-B9 transport.Write 0회 불변식이 TCP 모드(tcp-client + tcp-server) 에서도 유지됨 (AC-G8 — M6 PENDING)
- [x] AC-B9 불변식이 v0.3.0~v0.4.2 device_state emit + deviceStateBuf + drain_device_state 경로에서도 유지됨 (송신 경로 미사용)
- [ ] `go test -race ./internal/agent/century/...` 통과
- [ ] `go test -race ./internal/node/...` 통과
- [ ] `cd web && npm test` 통과
- [ ] 테스트 커버리지: `internal/agent/century` 85% 이상, `internal/node` century 관련 함수 85% 이상
- [ ] `go vet ./...` 경고 없음
- [ ] golangci-lint 경고 없음
- [ ] 프로토콜 문서 부록 A (CAP-3), 부록 B (CAP-1), 부록 C (CAP-4) 의 모든 프레임을 골든 픽스처로 회귀 검증 통과
- [ ] CRC-16/ARC init 0x0000 vs Modbus init 0xFFFF 의 명시적 회귀 거부 테스트 통과
- [ ] xflowd 데몬 부팅 + century-hvac 에이전트 등록 + 첫 사이클 캡처 smoke 테스트 통과
- [ ] CenturyAgent 가 어떤 경로로도 transport.Write() 를 호출하지 않음을 mock 트랜스포트로 검증 (AC-B9)

### 선택 통과 조건

- [ ] 실제 Century HVAC 장비 회선에 RX-only tap 으로 24 시간 캡처, 디코딩 결과의 일관성 검증
- [ ] `century-raw-frame` 노드로 새 캡처를 수집하여 미확정 필드(`status_bits` 비트 매핑, `op_val_1`/`op_val_2` 단위) 의미 발굴 시도
- [ ] 실제 다중 indoor unit (다중 `sub_dev_id`) 환경에서의 ground truth 캡처 확보 → 후속 SPEC 으로 acceptance 보강 (v0.1.0 은 합성 frame 으로 검증, AC-B1b)
- [ ] 모드 코드 `0x02` 이상의 추가 캡처 확보 시 additive enum 으로 SPEC 갱신

---

## 부록: Verification Results (v0.1.2)

각 AC 시나리오가 어떤 자동 테스트 함수에서 검증되는지 매핑한다. M1-M5 commit chain
(14ee853 → bfdfaf0 → d33da37 → bad2e06 → 3f1b970 → [M5]) 에서 누적 추가된 테스트들이다.

### 그룹 A: 프레임 디코딩 (11/11 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-A1 | TestDecodeReg02Response_CAP3_Cooling | internal/agent/century/decoder_reg02_test.go |
| AC-A2 | TestDecodeReg03Response_CAP4_Steady | internal/agent/century/decoder_reg03_test.go |
| AC-A3 | TestDecodeReg04Response_CAP4_Steady | internal/agent/century/decoder_reg04_test.go |
| AC-A4 | TestDecodeReg04Write_CAP3_PassiveObservation | internal/agent/century/decoder_reg04_test.go |
| AC-A5 | TestDecodeAck_SinglePayloadByte | internal/agent/century/decoder_test.go |
| AC-A6 | TestFrameScanner_RejectsCRCMismatch | internal/agent/century/frame_scanner_test.go |
| AC-A7 | TestCRC16ARC_ModbusInitRejectsCenturyFrames | internal/agent/century/crc_test.go |
| AC-A8 | TestFrameScanner_ResyncOnInvalidHeader | internal/agent/century/frame_scanner_test.go |
| AC-A9 | TestFrameParser_PayloadPrefixInvalid | internal/agent/century/frame_parser_test.go |
| AC-A10 | TestDecodeReg02Response_RejectsWrongLength | internal/agent/century/decoder_reg02_test.go |
| AC-A11 | TestFrameScanner_InFrameGapTolerance | internal/agent/century/frame_scanner_test.go |

### 그룹 B: 에이전트 런타임 (10/10 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-B1a | TestCenturyAgent_AutoDiscovery_FirstSubDevID | internal/agent/century/agent_test.go |
| AC-B1b | TestCenturyAgent_AutoDiscovery_MultiSubDevID | internal/agent/century/agent_test.go |
| AC-B2 | TestCenturyAgent_OfflineDetection_PerDevice | internal/agent/century/agent_test.go |
| AC-B3 | TestRingBuffer_OverflowEvictsOldest | internal/agent/century/ring_buffer_test.go |
| AC-B4 | TestCenturyAgent_LogDropsToggles | internal/agent/century/agent_test.go |
| AC-B5 | TestCenturyAgent_LifecycleTransitions | internal/agent/century/agent_test.go |
| AC-B6 | TestCenturyAgent_GetStatsCommand | internal/agent/century/agent_test.go |
| AC-B7 | TestCenturyAgent_GetRecentCommand | internal/agent/century/agent_test.go |
| AC-B8 | TestCenturyAgent_DrainCommand | internal/agent/century/agent_test.go |
| AC-B9 | TestCenturyAgent_NeverWritesToTransport | internal/agent/century/agent_test.go |
| AC-B10 | TestCenturyAgent_BufferInfoAndFrameNotifyCh | internal/agent/century/agent_test.go |

### 그룹 C: 플로우 노드 (8/8 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-C1 | TestCenturyStatusNode_PollEmitsTypedStatus | internal/node/century_test.go |
| AC-C2 | TestCenturyStatusNode_MissingAgentRef | internal/node/century_test.go |
| AC-C3 | TestCenturyControlNode_AlwaysNotSupported | internal/node/century_test.go |
| AC-C4 | TestCenturyNode_CombinedRouting | internal/node/century_test.go |
| AC-C5 | TestCenturyRawFrameNode_EmitsAllFrames | internal/node/century_test.go |
| AC-C6 | TestCenturyStatusNode_RejectsNonCenturyAgent | internal/node/century_test.go |
| AC-C7 | TestCenturyStatusNode_DeferredInitTolerance | internal/node/century_test.go |
| AC-C8 | TestCenturyStatusNode_FrameNotifyChImmediateReact | internal/node/century_test.go |

### 그룹 D: 설정 및 등록 (7/7 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-D1 | TestRegisterCenturyTypes | internal/agent/century/registration_test.go |
| AC-D2 | TestParseCenturyConfig_RequiresSerialPort | internal/agent/century/config_test.go |
| AC-D3 | TestParseCenturyConfig_HexAddressInput | internal/agent/century/config_test.go |
| AC-D4 | TestParseCenturyConfig_ReadTimeoutClamp | internal/agent/century/config_test.go |
| AC-D5 | centurySchema.test.ts (CENTURY_HVAC_FIELDS 시각화) | web/src/config/__tests__/centurySchema.test.ts |
| AC-D6 | centurySchema.test.ts (4 노드 등록 + agent_select) | web/src/config/__tests__/centurySchema.test.ts |
| AC-D7 | cmd/xflowd/main.go:322 `century.RegisterCenturyTypes(agentMgr)` 호출 + binary 빌드 확인 | cmd/xflowd/main.go |

### 그룹 E: 필드 디코딩 정책 (5/5 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-E1 | TestMessagePayload_ConfirmationStatusMarkersAllFields | internal/agent/century/message_test.go |
| AC-E2 | TestDecodeReg02_AdditiveModeEnum_UnknownByte | internal/agent/century/decoder_reg02_test.go |
| AC-E3 | (설계 보증 — v0.2.0 alias 도입 시 적용. SPEC §5.9 closure notes) | spec.md §5.9 |
| AC-E4 | TestMessagePayload_TimestampIsEpochMilliseconds | internal/agent/century/message_test.go |
| AC-E5 | TestMessagePayload_RawHexRoundTrip | internal/agent/century/message_test.go |

### 그룹 F: WRITE 중복 처리 (4/4 통과)

| AC | 자동 테스트 함수 (대표) | 위치 |
|----|------------------------|------|
| AC-F1 | TestWriteDeduplicator_SameCycleDedupes | internal/agent/century/write_deduplicator_test.go |
| AC-F2 | TestWriteDeduplicator_DisabledEmitsAll | internal/agent/century/write_deduplicator_test.go |
| AC-F3 | TestCycleTracker_NewCycleAfterReg04Response | internal/agent/century/cycle_tracker_test.go |
| AC-F4 | TestCycleTracker_IdleGapTriggersNewCycle | internal/agent/century/cycle_tracker_test.go |

### 그룹 G: TCP transport (v0.2.0) — PENDING (M6 구현 예정)

| AC | 자동 테스트 함수 (예정) | 위치 (예정) | 상태 |
|----|------------------------|-------------|------|
| AC-G1 | TestTCPClient_DialAndDecodeCAP3Cycle | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G2 | TestTCPClient_DialFailureExponentialBackoff | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G3 | TestTCPClient_ReconnectAfterEOF | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G4 | TestTCPClient_ReadTimeoutTriggersReconnect | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G5 | TestTCPServer_ListenAcceptAndDecode | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G6 | TestTCPServer_RejectsSecondaryConnection | internal/agent/century/transport_tcp_test.go | PENDING (M6) |
| AC-G7 | TestParseCenturyConfig_CycleIdleTimeoutDefault_TransportAware | internal/agent/century/config_test.go | PENDING (M6) |
| AC-G8 | TestTCPTransport_NeverWritesUnderAllScenarios | internal/agent/century/transport_tcp_test.go | PENDING (M6) |

### 그룹 H: Device-centric output (v0.3.0 → v0.4.2, **Breaking x2**) — Implemented (M7 + 11 hotfix 완료)

| AC | 자동 테스트 함수 | 위치 | 상태 |
|----|------------------|------|------|
| AC-H1 | TestAgent_DeviceState_RegisterDecodedOptOut | internal/agent/century/agent_test.go | ✓ |
| AC-H2 | TestAgent_DeviceState_FirstEmitAfterReg02AndReg04 (v0.4.2 rename) | internal/agent/century/agent_test.go | ✓ |
| AC-H3 | TestAgent_DeviceState_Reg02AndReg04UnifiedSingleEmit (v0.4.2 의미 갱신) | internal/agent/century/agent_test.go | ✓ |
| AC-H4 | TestAgent_DeviceState_SameValueNoReEmit | internal/agent/century/agent_test.go | ✓ |
| AC-H5 | TestAgent_DeviceState_ModeTransitionEmitsPowerChange | internal/agent/century/agent_test.go | ✓ |
| AC-H6 | TestAgent_DeviceState_KeepaliveAfterInterval | internal/agent/century/agent_test.go | ✓ |
| AC-H7 | TestAgent_DeviceState_OfflineTransitionEmits | internal/agent/century/agent_test.go | ✓ |
| AC-H8 | TestAgent_DeviceState_RegisterOnlyMode (v0.4.0: type 필드 검증 추가) | internal/agent/century/agent_test.go | ✓ |
| AC-H9 | TestParseCenturyConfig_BothOutputsOffReturnsError | internal/agent/century/config_test.go | ✓ |
| AC-H10 | TestAgent_DeviceState_MultiSubDevIDIndependent | internal/agent/century/agent_test.go | ✓ |
| AC-H11 | TestShouldKeepaliveFire (12 cases) + TestParseCenturyConfig_KeepaliveMode (5 cases) | internal/agent/century/keepalive_mode_test.go | ✓ (v0.3.9) |
| AC-H12 | TestAgent_KeepaliveFiresDespiteFrequentChanges | internal/agent/century/agent_test.go | ✓ (v0.3.10) |
| AC-H13 | TestPushDeviceStateBuf_DropOldest | internal/agent/century/agent_test.go | ✓ (v0.3.11) |
| AC-H14 | TestProcessDrainDeviceState_BasicFlow | internal/agent/century/agent_test.go | ✓ (v0.3.11) |
| AC-H15 | TestAgent_DeviceStateGatedByReg02 (v0.4.1) + TestAgent_DeviceStateGatedByReg04 (v0.4.2) | internal/agent/century/agent_test.go | ✓ |

### 종합

- **v0.1.2 자동화 비율**: 41/41 (100%)
- **v0.2.0 자동화 비율 (계획)**: 49/49 = 41 (v0.1.2 회귀 보존) + 8 (그룹 G 신규, M6 구현 후)
- **v0.4.2 자동화 비율**: 64/64 = 41 (v0.1.2 회귀 보존, register-decoded 테스트는 emit_register_decoded=true 강제 setup) + 8 (v0.2.0 그룹 G, M6 PENDING) + 15 (그룹 H 완료)
- **수동 검증 필요**: 선택 통과 조건 4건 (실제 장비 캡처, 다중 IDU ground truth 등 — v0.2.0+ 후속), 실제 TCP 컨버터(Moxa NPort 등) 와의 round-trip smoke (M6 완료 후)
- **회귀 테스트 실행**: `go test -race -count=3 ./internal/agent/century/... ./internal/node/...` 3회 반복 통과 (flake 없음)
- **CRC-16/ARC vs Modbus 회귀**: TestCRC16ARC_ModbusInitRejectsCenturyFrames 가 CI 에서 항상 실행됨 (AC-A7)
- **Smoke 검증 (v0.1.2)**: `examples/agents/century-hvac.yaml` 이 project's own `agent.AgentConfigFromYAML` + `century.NewCenturyAgent` 로 round-trip 성공; `examples/flows/century-status-flow.yaml` 이 `flow.LoadFlowFromFile` 로 8 nodes / 8 wires 파싱 성공
- **Smoke 검증 (v0.2.0 계획)**: `examples/agents/century-hvac-tcp-client.yaml` + `century-hvac-tcp-server.yaml` 모두 round-trip 검증 (M6 완료 후)
- **Smoke 검증 (v0.4.2 완료)**: `examples/agents/century-hvac.yaml` 의 모든 신규 옵션 (emit_device_state, emit_register_decoded, keepalive_interval, keepalive_mode, include_register_info, include_raw_hex, include_unknown_fields, include_inferred_fields) round-trip 성공; downstream `century-status` 노드 + Bridge 컨슈머 양쪽에서 v0.4.0 state-grouped `type="device_state"` 메시지 관찰 + Reg02+Reg04 gate 적용 후 첫 emit (~512ms 지연) 확인

---

*Acceptance 버전: 0.4.2 (Implemented — M7 + 11 hotfix 사이클 완료)*
*작성일: 2026-05-18 (v0.1.0), 갱신: 2026-05-18 (v0.1.1 — B1 → B1a/B1b 확장, F 그룹 신설), 2026-05-18 (v0.1.2 — Verification Results 부록 추가), 2026-05-18 (v0.2.0 — 그룹 G TCP transport 신설), 2026-05-19 (v0.3.0 — 그룹 H Device-centric output 신설, Breaking), 2026-05-19 (v0.3.1~v0.3.8 — schema 통일 + register-decoded transform), 2026-05-19 (v0.3.9 — AC-H11 keepalive_mode), 2026-05-19 (v0.3.10 — AC-H12 lastKeepaliveTime 분리), 2026-05-19 (v0.3.11 — AC-H13 deviceStateBuf + AC-H14 drain_device_state), 2026-05-19 (v0.4.0 — state 그룹 + type 필드 마이그레이션), 2026-05-19 (v0.4.1 — Reg02 gate), 2026-05-19 (v0.4.2 — AC-H2 rename FirstEmitAfterReg02AndReg04 + AC-H15 신설, Implemented)*
*작성자: xtra*
