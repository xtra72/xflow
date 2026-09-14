---
id: SPEC-LG-HVACR-003
type: acceptance
version: "1.0.0"
created: "2026-09-14"
updated: "2026-09-14"
---

# SPEC-LG-HVACR-003 수락 기준: LG HVACR-03 (PMBUSB00A) 제어 에이전트

전 기준은 **하드웨어 없이** mock 트랜스포트로 검증 가능해야 한다. 현장 실측이 필요한 항목은 §10 에 별도 분리한다.

---

## 1. 프로토콜 층 — 주소 계산 (M1)

### AC-001: Coil / Discrete 주소 계산

**Given** 실내기 주소 N 과 항목번호 item(0-base)이 주어질 때
**When** `pmbusCoilAddr(N, item)` 또는 `pmbusDiscreteAddr(N, item)` 를 호출하면
**Then** `N*16 + item` 을 반환해야 한다

### AC-002: Holding / Input 주소 계산

**Given** 실내기 주소 N 과 항목번호 item(0-base)이 주어질 때
**When** `pmbusHoldingAddr(N, item)` 또는 `pmbusInputAddr(N, item)` 를 호출하면
**Then** `N*20 + item` 을 반환해야 한다

### AC-003: 문서 §4.6 조견표 전 행 일치

**Given** 문서 §4.6 조견표의 N=0~15 전 16행
**When** 각 행의 Coil 시작 / Discrete 시작 / Holding 시작 / 모드 / 풍량 / 설정온도 / Input 시작 / 에러 / 실내온도 주소를 계산 함수로 산출하면
**Then** 모든 값이 조견표와 일치해야 한다 (예: N=3 설정온도 = 62, N=15 Holding 시작 = 300)

### AC-004: 1-base 번호 표기 미사용

**Given** 프로토콜 층 소스 파일
**When** `40001` / `30001` / `10001` / `00001` 리터럴을 검색하면
**Then** 주석 외 코드에서 발견되지 않아야 한다 (문서 §4.1 — 프레임 주소는 0-base)

---

## 2. 프로토콜 층 — 값 인코딩/디코딩 (M1)

### AC-005: 온도 디코딩 — 양수

**Given** `temp_scale: 10` 설정에서
**When** Input Register 값 `0x00F5`(245)를 디코딩하면
**Then** `24.5` °C 를 반환해야 한다 (문서 §5.1)

### AC-006: 온도 디코딩 — 음수 (signed int16)

**Given** `temp_scale: 10` 설정에서
**When** Input Register 값 `0xFFC4` 를 디코딩하면
**Then** `-6.0` °C 를 반환해야 한다 (문서 §4.5 — signed 해석 필수)

> `uint16` 로 해석하면 6553.2 가 나온다. 이 테스트가 signed 해석을 고정한다.

### AC-007: 온도 인코딩

**Given** `temp_scale: 10` 설정에서
**When** `24.0` °C 를 인코딩하면
**Then** `240`(`0x00F0`)을 반환해야 한다

### AC-008: temp_scale 설정 반영

**Given** `temp_scale: 1` 로 설정되었을 때
**When** Holding Register 값 `24` 를 디코딩하면
**Then** `24.0` °C 를 반환해야 한다 (문서 §8 #1 실측 불일치 시 교정 경로)

### AC-009: 운전 모드 코드 → 통일 ID

**Given** Holding ① 운전 모드 코드가 주어질 때
**When** 통일 ID 로 변환하면
**Then** 다음 매핑을 따라야 한다:
- 0(냉방) → `hvac.ModeCool` (1)
- 1(제습) → `hvac.ModeDry` (3)
- 2(송풍) → `hvac.ModeFan` (4)
- 3(자동) → `hvac.ModeOffOrAuto` (0)
- 4(난방) → `hvac.ModeHeat` (2)

### AC-010: 풍량 코드 → 통일 ID (기본 fan_auto_code)

**Given** `fan_auto_code: 4`(기본값)일 때
**When** Holding ② 풍량 코드를 통일 ID 로 변환하면
**Then** 다음 매핑을 따라야 한다:
- 1(약) → `hvac.FanLow` (3)
- 2(중) → `hvac.FanMedium` (4)
- 3(강) → `hvac.FanHigh` (5)
- 4 → `hvac.FanAuto` (1)

### AC-011: 풍량 코드 → 통일 ID (fan_auto_code 교정)

**Given** `fan_auto_code: 5` 로 설정되었을 때
**When** 풍량 코드 4 와 5 를 각각 변환하면
**Then** 4 → `hvac.FanTurbo`(6), 5 → `hvac.FanAuto`(1) 로 변환되어야 한다 (문서 §7.1-2 — LGCP 5단계 대응)

### AC-012: address_base 설정 반영

**Given** `address_base: 1` 로 설정되었을 때
**When** 사용자 지정 주소 `"1"` 에 대한 Coil 시작 주소를 계산하면
**Then** `0` 을 반환해야 한다 (문서 §8 #2 실측 불일치 시 교정 경로)

---

## 3. PDU 및 프레임 — 문서 §5 골든 테스트 (M2)

> **기준값 사전 검증 (2026-09-14)**: 본 절의 기대 프레임 13종은 SPEC 작성 시점에 CRC-16/MODBUS(poly 0xA001, init 0xFFFF, Little-Endian 전송)로 독립 재계산하여 **13/13 전부 일치**함을 확인했다. 골든 테스트의 기준값 자체가 정확하므로, 구현이 이 테스트를 통과하면 CRC 다항식·초기값·엔디안이 함께 고정된다.
>
> 부수적으로 문서의 **내부 정합성**도 확인되었다. `IDU 3 설정온도 26.0` 프레임의 주소 `0x003E`(62)는 `3×20+2` 로 **0-base 주소 체계**와 일치하고, 값 `0x0104`(260)은 `26.0×10` 으로 **×10 스케일**과 일치한다. 이는 문서 §8 #1·#2 의 실측을 대체하지는 않지만(문서가 스스로와 일관되다는 것이 하드웨어와 일치한다는 뜻은 아니다), 기본값 선택의 근거를 뒷받침한다.

### AC-013: 제어 프레임 7종 바이트 일치

**Given** 슬레이브 주소 1 로 설정된 에이전트에서
**When** 각 제어 명령의 ADU 를 생성하면
**Then** 문서 §5 의 hex 와 **바이트 단위로 일치**해야 한다

| 동작 | 기대 프레임 |
|------|------------|
| IDU 0 전원 ON | `01 05 00 00 FF 00 8C 3A` |
| IDU 0 전원 OFF | `01 05 00 00 00 00 CD CA` |
| IDU 3 전원 ON | `01 05 00 30 FF 00 8C 35` |
| IDU 0 난방 모드 | `01 06 00 00 00 04 88 09` |
| IDU 0 풍량 강 | `01 06 00 01 00 03 98 0B` |
| IDU 0 설정온도 24.0 | `01 06 00 02 00 F0 28 4E` |
| IDU 3 설정온도 26.0 | `01 06 00 3E 01 04 E8 55` |

### AC-014: FC16 일괄 쓰기 프레임 일치

**Given** 슬레이브 주소 1 에서
**When** IDU 0 에 대해 모드=난방(4), 풍량=강(3), 온도=24.0(240)을 `set_multiple` 로 인코딩하면
**Then** `01 10 00 00 00 03 06 00 04 00 03 00 F0 E7 04` 와 일치해야 한다

### AC-015: 읽기 프레임 5종 바이트 일치

**Given** 슬레이브 주소 1 에서
**When** 각 읽기 요청의 ADU 를 생성하면
**Then** 문서 §5 의 hex 와 일치해야 한다

| 동작 | 기대 프레임 |
|------|------------|
| IDU 0 상태 읽기 (FC03, 6워드) | `01 03 00 00 00 06 C5 C8` |
| IDU 0 센서 읽기 (FC04, 6워드) | `01 04 00 00 00 06 70 08` |
| IDU 0 상태비트 (FC02, 5bit) | `01 02 00 00 00 05 B8 09` |
| IDU 0 코일 상태 (FC01, 10bit) | `01 01 00 00 00 0A BC 0D` |
| 전 실내기 스캔 (FC02, 256bit) | `01 02 00 00 01 00 79 9A` |

### AC-016: CRC 다항식 검증

**Given** 프로토콜 층이 생성한 임의의 ADU
**When** 마지막 2바이트를 검사하면
**Then** CRC-16/MODBUS (poly 0xA001, init 0xFFFF, **Little-Endian** 전송) 이어야 하며, CRC-16/XMODEM 이어서는 안 된다

### AC-017: 응답 파싱 — Input Register

**Given** 응답 PDU `04 0C 0000 00F5 00D2 00E8 0000 0000` 가 주어질 때
**When** 6워드 Input Register 응답으로 파싱하면
**Then** 문서 §5.1 의 해석과 일치해야 한다:
- 에러 코드 = 0
- 실내 온도 = 24.5 °C
- 배관 입구 = 21.0 °C
- 배관 출구 = 23.2 °C

### AC-018: Modbus 예외 응답 처리

**Given** 응답 PDU 의 FC 최상위 비트가 설정된 예외 응답(예: `83 02`)이 수신될 때
**When** 파싱하면
**Then** 예외 코드를 포함한 에러를 반환해야 하며, 정상 응답으로 오해해서는 안 된다

### AC-019: 슬레이브 주소 불일치 응답 거부

**Given** 슬레이브 주소 1 로 요청했을 때
**When** 응답 ADU 의 슬레이브 주소가 2 이면
**Then** 파싱 에러를 반환해야 한다

---

## 4. 설정 파싱 (M3)

### AC-020: 필수 필드 검증 — RTU

**Given** `transport_type: rtu` 설정에서
**When** `serial_port` 가 누락되면
**Then** `ErrHvacr03SerialPortRequired` 를 반환해야 한다

### AC-021: 필수 필드 검증 — TCP

**Given** `transport_type: tcp-client` 설정에서
**When** `tcp_host` 가 누락되면
**Then** 설정 오류를 반환해야 한다

### AC-022: 알 수 없는 트랜스포트 타입

**Given** `transport_type: tcp-server` 가 설정될 때
**When** 설정을 파싱하면
**Then** `ErrHvacr03UnknownTransportType` 를 반환해야 한다

> `lg_hvacr02` 는 `tcp-server` 를 지원하지만 `lg_hvacr03` 은 능동 마스터이므로 서버 모드가 성립하지 않는다.

### AC-023: poll_interval 하한 강제

**Given** `poll_interval: 3s` 가 설정될 때
**When** 설정을 파싱하면
**Then** `ErrHvacr03PollIntervalTooShort` 를 반환해야 한다 (문서 §6.1 — 5초가 현실적 하한)

### AC-024: slave_id 범위 검증

**Given** `slave_id: 17` 또는 `slave_id: 0` 이 설정될 때
**When** 설정을 파싱하면
**Then** 설정 오류를 반환해야 한다 (문서 §3.1 — DIP 스위치 범위 1~16)

### AC-025: 기본값 적용

**Given** 선택 필드를 모두 생략한 설정에서
**When** 설정을 파싱하면
**Then** SPEC §5 의 기본값이 적용되어야 한다:
`transport_type: rtu`, `baud_rate: 9600`, `data_bits: 8`, `stop_bits: 1`, `parity: none`, `slave_id: 1`, `poll_interval: 10s`, `scan_interval: 30s`, `request_timeout: 1s`, `offline_timeout: 30s`, `temp_scale: 10`, `address_base: 0`, `fan_auto_code: 4`, `auto_discovery: true`, `report_interval: 60s`, `event_temp_threshold: 1.0`, `control_enabled: false`, `control_verify_delay: 3s`

### AC-026: 잘못된 타입 입력에 panic 하지 않음

**Given** `slave_id: "abc"` 처럼 타입이 어긋난 YAML 이 주어질 때
**When** 설정을 파싱하면
**Then** panic 없이 설정 오류를 반환해야 한다

### AC-027: 디바이스 주소 검증

**Given** `devices` 항목에 `address: "16"` 또는 `address: "44550065"` 가 주어질 때
**When** 설정을 파싱하면
**Then** 설정 오류를 반환해야 한다 (N 은 0~15 정수, LGCP hex 주소가 아님 — 문서 §3.2)

---

## 5. 트랜스포트 및 생명주기 (M4)

### AC-028: 트랜스포트 선택

**Given** `transport_type` 이 `rtu` 또는 `tcp-client` 일 때
**When** 에이전트가 시작되면
**Then** 각각 `ModbusRTUTransport` / `ModbusTCPTransport` 가 생성되어야 한다

### AC-029: Start 멱등성

**Given** 이미 실행 중인 에이전트에서
**When** `Start` 가 다시 호출되면
**Then** 백그라운드 고루틴(scanLoop / pollLoop / notifyLoop / offlineWatchLoop)이 중복 기동되지 않아야 한다

> 중복 기동 시 같은 디바이스 report 가 한 틱에 N건 중복 발행된다 (`lg_hvacr02` 의 `bgStarted` 가드 사유).

### AC-030: 재연결 백오프

**Given** 트랜스포트 연결이 실패할 때
**When** 재연결을 반복 시도하면
**Then** `reconnect_interval` 에서 시작해 `max_reconnect_backoff` 까지 지수 증가해야 한다

### AC-031: 연결 끊김 시 전 디바이스 오프라인

**Given** 온라인 디바이스가 존재하는 상태에서
**When** 트랜스포트 연결이 끊어지면
**Then** 모든 디바이스가 오프라인으로 표시되어야 한다

### AC-032: Stop 후 고루틴 정리

**Given** 실행 중인 에이전트에서
**When** `Stop` 이 호출되면
**Then** 모든 백그라운드 고루틴이 종료되고 트랜스포트가 닫혀야 한다

### AC-033: 락 재귀 금지

**Given** `a.mu` 를 보유한 코드 경로에서
**When** 소스를 검사하면
**Then** `a.Name()` / `a.ID()` / `a.Type()` 호출이 없어야 한다 (Go RWMutex 재귀 락 금지 — `lg_hvacr02` v0.18.6 트랩)

---

## 6. 디바이스 발견 및 관리 (M5)

### AC-034: FC02 전체 스캔으로 발견

**Given** mock 이 N=0, 3, 7 의 연결 비트만 1 로 응답할 때
**When** 에이전트가 시작되면
**Then** 정확히 3개 디바이스(주소 "0", "3", "7")가 등록되어야 한다

### AC-035: 단일 트랜잭션 스캔

**Given** 전체 스캔이 수행될 때
**When** mock 이 수신한 요청을 검사하면
**Then** FC02 / 시작주소 0 / 개수 256 인 **단일 요청**이어야 한다 (16대 × 16bit 를 1 트랜잭션으로 — 문서 §6.1)

### AC-036: 알람 및 필터 비트 추출

**Given** mock 이 N=3 의 연결=1, 알람=1, 필터=0 으로 응답할 때
**When** 스캔이 완료되면
**Then** 디바이스 "3" 의 상태에 `alarm: true`, `filter_alarm: false` 가 반영되어야 한다

### AC-037: auto_discovery 비활성

**Given** `auto_discovery: false` 이고 설정 디바이스가 "0" 하나일 때
**When** 스캔에서 N=0,3,7 이 발견되면
**Then** 디바이스 목록은 "0" 하나만 유지해야 한다

### AC-038: 설정 디바이스는 미발견 시에도 유지

**Given** `devices` 에 "5" 가 등록되어 있을 때
**When** 스캔에서 N=5 의 연결 비트가 0 이면
**Then** 디바이스 "5" 는 목록에 남되 오프라인으로 표시되어야 한다

### AC-039: 오프라인 전이

**Given** 온라인 디바이스가 있을 때
**When** `offline_timeout` 동안 연결 비트가 0 으로 관측되면
**Then** 해당 디바이스가 오프라인으로 전이되어야 한다

### AC-040: report_enabled 게이트

**Given** 디바이스의 `report_enabled` 가 `false` 일 때
**When** 상태 변경 또는 주기 보고가 발생하면
**Then** `device_state` 가 방출되지 않아야 하며, `lastEmitted` 도 갱신되지 않아야 한다

### AC-041: device_id / unit_id 분리

**Given** 디바이스 "3" 이 등록되어 있을 때
**When** `device_state` 가 방출되면
**Then** `unit_id` 는 `"3"`, `device_id` 는 `agent.ResolveDeviceID` 가 해석한 UUID v4 여야 한다

---

## 7. 폴링 및 상태 투영 (M6)

### AC-042: 영역별 3 트랜잭션

**Given** 온라인 디바이스 "0" 하나가 있을 때
**When** 폴링 1 사이클이 수행되면
**Then** mock 이 FC01(코일 10bit) / FC03(Holding 6워드) / FC04(Input 6워드) 세 요청을 수신해야 한다

### AC-043: 타임아웃 시 사이클 계속

**Given** 온라인 디바이스 "0", "1", "2" 가 있고 "1" 이 응답하지 않을 때
**When** 폴링 사이클이 수행되면
**Then** "0" 과 "2" 는 정상 폴링되어야 하며, 사이클이 중단되어서는 안 된다

### AC-044: 오프라인 디바이스 스킵

**Given** 디바이스 "3" 이 오프라인일 때
**When** 폴링 사이클이 수행되면
**Then** "3" 에 대한 FC01/FC03/FC04 요청이 전송되지 않아야 한다

### AC-045: 트랜잭션 직렬화

**Given** 폴링과 제어가 동시에 요청될 때
**When** mock 트랜스포트의 요청 수신 순서를 검사하면
**Then** 한 요청의 응답이 완료되기 전에 다음 요청이 전송되지 않아야 한다

### AC-046: 상태 키 이름 — lg_hvacr02 동형

**Given** 전원 ON 실내기의 상태가 투영될 때
**When** 투영 결과의 키를 검사하면
**Then** `power`, `current_temperature`, `target_temperature`, `fan_speed`, `mode` 를 포함해야 한다

### AC-047: 전원 OFF 시 운전 속성 생략

**Given** 실내기 전원이 OFF 일 때
**When** 상태가 투영되면
**Then** `target_temperature` 가 포함되지 않아야 하고, `fan_speed` 와 `mode` 는 통일 ID `0` 이어야 한다

### AC-048: PMBUSB00A 확장 속성

**Given** 실내기 상태가 투영될 때
**When** 투영 결과를 검사하면
**Then** `error_code`, `alarm`, `filter_alarm`, `pipe_in_temperature_c`, `pipe_out_temperature_c`, `swing`, `lock_*`, `temp_limit_high_c`, `temp_limit_low_c` 가 유효한 경우 포함되어야 한다

### AC-049: 기기 종류별 투영 게이트

**Given** 디바이스 종류가 에어컨(`HVACR.IDU`)일 때
**When** Input ⑤(급탕 탱크) / ⑥(태양열) 워드가 0 으로 수신되면
**Then** `water_tank_temperature_c` 와 `solar_temperature_c` 가 투영에 포함되지 않아야 한다

> 문서 §5.1 이 해당 워드를 "미사용(에어컨)" 으로 표기한다. 0 을 유효 온도로 오해하면 대시보드에 0 °C 가 표시된다.

---

## 8. 메시지 방출 (M6)

### AC-050: 페이로드 구조 — lg_hvacr02 동형

**Given** `device_state` 가 방출될 때
**When** JSON 구조를 검사하면
**Then** 최상위 키가 정확히 `unit_id`, `device_id`, `trigger`, `state`, `metadata`, `last_seen_ms` 여야 하고, `metadata` 는 `name`, `address`, `device_type` 을 가져야 한다

### AC-051: 링버퍼 + msgCh 이중 push

**Given** bridge 가 활성인 상태에서
**When** `device_state` 가 방출되면
**Then** 최근 프레임 링 버퍼와 `msgCh` **양쪽**에 push 되어야 한다

### AC-052: dedup — change 트리거

**Given** 직전과 동일한 상태에서
**When** `trigger: "change"` 로 방출을 시도하면
**Then** 방출되지 않아야 한다

### AC-053: dedup 예외 — report 트리거

**Given** 직전과 동일한 상태에서
**When** `trigger: "report"` 로 방출을 시도하면
**Then** 변경 여부와 무관하게 방출되어야 한다

### AC-054: last_seen_ms 형식 및 기준 시각

**Given** `device_state` 가 방출될 때
**When** `last_seen_ms` 를 검사하면
**Then** epoch milliseconds (int64) 여야 하며, `trigger: "report"` 는 발행 시점, 그 외는 관측 시점이어야 한다

### AC-055: event_temp_threshold 게이트

**Given** `event_temp_threshold: 1.0` 설정에서
**When** 실내 온도만 24.5 → 24.8 로 변경되면
**Then** `change` 방출이 억제되어야 한다

### AC-056: event_temp_threshold 통과

**Given** `event_temp_threshold: 1.0` 설정에서
**When** 실내 온도가 24.5 → 26.0 으로 변경되면
**Then** `change` 가 방출되어야 한다

---

## 9. 명령 인터페이스 및 제어 (M7)

### AC-057: 조회 명령 9종 지원

**Given** 에이전트가 실행 중일 때
**When** `get_stats`, `get_recent`, `drain`, `get_all`, `request_state`, `get_state`, `list_devices`, `remove_device`, `set_device` 를 각각 호출하면
**Then** 모두 에러 없이 처리되어야 한다

### AC-058: get_recent count 규약

**Given** 링 버퍼에 프레임이 쌓여 있을 때
**When** `get_recent` 를 `count > 0` 으로 호출하면 비파괴 조회, `count == 0` 으로 호출하면 drain(파괴적)이어야 한다

### AC-059: 알 수 없는 명령

**Given** 에이전트가 실행 중일 때
**When** 지원하지 않는 명령을 호출하면
**Then** 에러를 반환하고 `messages_errored` 통계가 증가해야 한다

### AC-060: control_enabled 게이트

**Given** `control_enabled: false` 일 때
**When** `set_power` 를 호출하면
**Then** `ErrHvacr03ControlNotEnabled` 를 반환하고 **버스에 어떤 쓰기도 전송되지 않아야** 한다

### AC-061: 미연결 디바이스 쓰기 차단

**Given** `control_enabled: true` 이고 N=5 의 연결 비트가 0 일 때
**When** 주소 "5" 에 `set_power` 를 호출하면
**Then** `ErrHvacr03DeviceNotConnected` 를 반환하고 쓰기를 전송하지 않아야 한다 (문서 §6.2-4)

### AC-062: set_multiple 은 FC16 단일 트랜잭션

**Given** `control_enabled: true` 일 때
**When** `set_multiple` 로 모드·풍량·온도를 함께 지정하면
**Then** mock 이 수신한 쓰기 요청은 FC16 **1건**이어야 하며, FC06 3건으로 분할되어서는 안 된다 (문서 §6.2-5)

### AC-063: 온도 범위 검증 — 절대 범위

**Given** `control_enabled: true` 일 때
**When** `target_temperature` 로 `31.0` 또는 `15.0` 을 지정하면
**Then** `ErrHvacr03TemperatureOutOfRange` 를 반환해야 한다

### AC-064: 온도 범위 검증 — 상·하한 제한

**Given** 디바이스의 Holding ④(상한)=26.0, ⑤(하한)=20.0 일 때
**When** `target_temperature` 로 `28.0` 을 지정하면
**Then** `ErrHvacr03TemperatureOutOfRange` 를 반환해야 한다 (문서 §6.2-3)

### AC-065: read-back 검증 지연

**Given** `control_verify_delay: 3s` 설정에서
**When** 제어 쓰기가 성공하면
**Then** 지연 후 read-back 을 수행해야 하며, 쓰기 직후 즉시 읽어서는 안 된다 (문서 §6.2-1)

### AC-066: read-back 결과 응답 포함

**Given** 제어 쓰기 후 read-back 이 수행될 때
**When** 응답을 검사하면
**Then** `status`, `verified`, `expected`, `actual` 필드를 포함해야 한다

### AC-067: 잠금 진단

**Given** 온도 잠금 코일(Coil ⑦)이 설정된 디바이스에서
**When** `target_temperature` 쓰기 후 read-back 이 기대값과 다르면
**Then** 응답에 `locked_by: "temp"` 진단이 포함되어야 한다 (문서 §6.2-2)

### AC-068: 확장 제어 명령 7종

**Given** `control_enabled: true` 일 때
**When** `set_swing`, `clear_filter_alarm`, `set_lock`, `set_temp_limit`, `set_erv_mode`, `set_erv_rapid`, `set_erv_eco` 를 각각 호출하면
**Then** 문서 §4.2 / §4.4 의 레지스터로 매핑된 쓰기가 전송되어야 한다

### AC-069: set_lock 대상 매핑

**Given** `set_lock` 이 호출될 때
**When** `target` 파라미터가 `remote` / `mode` / `fan` / `temp` / `address` 이면
**Then** 각각 Coil `N×16+3` / `+4` / `+5` / `+6` / `+7` 로 매핑되어야 한다

### AC-070: set_temp_limit 은 FC16 동시 쓰기

**Given** `set_temp_limit` 으로 상한과 하한이 함께 지정될 때
**When** mock 이 수신한 요청을 검사하면
**Then** Holding `N×20+3..4` 에 대한 FC16 단일 요청이어야 한다

### AC-071: 기기 종류 불일치 제어 거부

**Given** 디바이스 종류가 에어컨(`HVACR.IDU`)일 때
**When** `set_erv_mode` 를 호출하면
**Then** `ErrHvacr03UnsupportedForDeviceType` 를 반환해야 한다

---

## 10. 통합 및 등록 (M8, M9)

### AC-072: 타입 등록

**Given** `RegisterLGHvacr03Types(mgr)` 가 호출될 때
**When** Manager 에서 타입 `lg_hvacr03` 을 조회하면
**Then** 팩토리가 등록되어 있어야 한다

### AC-073: main.go 배선

**Given** `cmd/xflowd/main.go` 소스에서
**When** `lg.RegisterLGHvacr03Types` 호출을 검색하면
**Then** 등록 블록 내에 존재해야 한다

> 함수를 정의만 하고 배선하지 않으면 에이전트가 조용히 사라진다.

### AC-074: DeviceProvider 노출

**Given** 디바이스가 등록된 에이전트에서
**When** `DeviceProvider().Devices()` 를 호출하면
**Then** 등록된 모든 디바이스가 `device.Device` 로 반환되어야 한다

### AC-075: ControllableDevice 조건부 노출

**Given** `control_enabled: true` 이고 디바이스 종류가 실내기일 때
**When** `DeviceProvider().Devices()` 를 호출하면
**Then** 해당 디바이스가 `device.ControllableDevice` 를 만족해야 한다

### AC-076: ControllableDevice 미노출

**Given** `control_enabled: false` 일 때
**When** `DeviceProvider().Devices()` 를 호출하면
**Then** 어떤 디바이스도 `device.ControllableDevice` 를 만족하지 않아야 한다

### AC-077: CommandExecutor 브리지

**Given** 제어 가능 디바이스에서
**When** `ControllableDevice` 의 명령을 실행하면
**Then** 에이전트의 `Process()` 로 전달되고 결과가 `map[string]any` 로 반환되어야 한다

### AC-078: 명령 스펙 노출

**Given** 제어 가능 실내기 디바이스에서
**When** 명령 스펙을 조회하면
**Then** 기본 5종 + 확장 7종이 모두 포함되어야 한다

### AC-079: 프론트엔드 등록 지점 대칭

**Given** 프론트엔드 소스에서
**When** `lg_hvacr02` 가 등장하는 등록 지점 목록과 `lg_hvacr03` 목록을 비교하면
**Then** `AgentDetailPanel.tsx` 의 `isLgIcp` 분기를 제외한 모든 지점에 `lg_hvacr03` 이 존재해야 한다

| 파일 | 지점 수 |
|------|:------:|
| `web/src/config/agentSchemas.ts` | 3 |
| `web/src/pages/agents/agentTypeMeta.ts` | 1 |
| `web/src/pages/agents/twoColumnConfigMap.ts` | 1 |
| `web/src/pages/agents/AgentTypesPage.tsx` | 1 |
| `web/src/pages/devices/DeviceListPage.tsx` | 2 |

---

## 11. 품질 게이트

### AC-080: CRC 격리

**Given** `lg_hvacr03_*` 및 `lg_pmbus_*` 소스 파일에서
**When** LGCP 체크섬 심볼(`lgIcp02CRC`, `checksum.go` 의 함수 등) 참조를 검색하면
**Then** 발견되지 않아야 한다 (문서 §2 — CRC 혼용 금지)

### AC-081: 커버리지

**Given** `go test -coverprofile` 을 실행할 때
**When** 신규 파일의 커버리지를 측정하면
**Then** 전체 85% 이상, 프로토콜 층(`lg_pmbus_register.go`, `lg_pmbus_codec.go`, `lg_pmbus_pdu.go`)은 100% 여야 한다

### AC-082: 경합 검출

**Given** `go test -race ./internal/agent/lg/...` 를 실행할 때
**When** 폴링·제어·방출이 동시 수행되는 테스트를 포함하면
**Then** 데이터 경합이 검출되지 않아야 한다

### AC-083: 하드웨어 비의존

**Given** AC-001 ~ AC-082 의 모든 테스트에서
**When** 실행 환경에 시리얼 포트나 게이트웨이가 없어도
**Then** 모든 테스트가 통과해야 한다

### AC-084: 린트 및 빌드

**Given** 변경 후 저장소에서
**When** `go build ./...` 와 프로젝트 린터를 실행하면
**Then** 오류가 없어야 한다

---

## 12. 현장 실측 항목 (수락 기준 아님 — 설치 후 확인)

아래는 하드웨어 없이 검증할 수 없으므로 수락 기준에서 제외한다. 게이트웨이 설치 직후 확인하고, 문서와 다르면 설정으로 교정한다.

| # | 확인 항목 | 방법 | 교정 경로 | 우선순위 |
|:-:|----------|------|----------|:-------:|
| 1 | 설정 온도 ×10 스케일 | 리모컨 24 °C 설정 후 Holding ② 읽기 → 240 인지 24 인지 | `temp_scale` | ★★★ |
| 2 | N 의 0-base 여부 | 실내기 1대만 전원 투입 후 FC02 스캔, 어느 비트가 1인지 | `address_base` | ★★★ |
| 3 | 풍량 값 4 의 실제 동작 | 4 기록 후 리모컨 표시 확인 (자동인지 초강인지) | `fan_auto_code` | ★★ |
| 4 | 쓰기 반영 지연 시간 | FC05 ON 직후 간격 read-back, 값 변경 시점 측정 | `control_verify_delay` | ★★ |
| 5 | 최소 안전 폴링 주기 | 주기를 5s 부터 올리며 타임아웃 발생률 측정 | `poll_interval` | ★★ |
| 6 | 실내 온도 음수 표현 | 동결 시험으로 signed 여부 확인 | (AC-006 으로 고정됨) | ★ |
| 7 | 미설치 N 쓰기 시 응답 | 존재하지 않는 N 에 FC06 → 예외코드 반환 여부 | (AC-061 로 차단됨) | ★ |
| 8 | Coil ②(스윙) 지원 기종 | 벽걸이/천장형별 동작 확인 | — | ★ |
| 9 | ERV / 하이드로킷 레지스터 | 해당 기기 확보 후 전 명령 검증 | — | ★ |
