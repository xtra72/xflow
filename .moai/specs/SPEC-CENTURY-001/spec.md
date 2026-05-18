# SPEC-CENTURY-001: Century HVAC 프로토콜 패시브 에이전트 및 플로우 노드

## 메타데이터

| 항목 | 값 |
|------|-----|
| ID | SPEC-CENTURY-001 |
| 버전 | 0.1.1 |
| 상태 | Draft |
| 생성일 | 2026-05-18 |
| 수정일 | 2026-05-18 |
| 작성자 | xtra |
| 우선순위 | Medium |
| 관련 SPEC | SPEC-SERIAL-001, SPEC-LGCNP-001, SPEC-NASA-001, SPEC-NODE-002, SPEC-AGENT-001, SPEC-AGENT-005, SPEC-ENGINE-001 |
| 프로토콜 문서 | `references/protocols/century_hvac_protocol_spec.md` (v0.3, 캡처 4건 검증) |

---

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 | 작성자 | 상태 |
|------|------|----------|--------|------|
| 2026-05-18 | 0.1.0 | 초안 작성. Century HVAC 마스터-슬레이브 바이너리 프로토콜의 RS-485 회선 **패시브 스니프(passive capture) 전용** 에이전트와 4종 플로우 노드(status/control/combined/raw-frame) 도입. NASA 에이전트 스켈레톤 + LGCNP 패시브 캡처 패턴(다층 검증, ring buffer, DeviceProvider)을 결합한 구조. 미확정 필드는 `confirmation_status` 마커(`confirmed`/`inferred`/`unknown`)로 타입드 노출하여 향후 캡처를 통한 확정 진화를 허용. | xtra | Draft |
| 2026-05-18 | 0.1.1 | 다중 IDU 자동 발견 v0.1.0 범위 포함 (A7/REQ-CENTURY-013 갱신, 리스크 R3 삭제). WRITE 중복 제거 옵션 추가 (REQ-CENTURY-027, dedupe_writes 설정 필드, 시나리오 F1~F4 신설). | xtra | Draft |

---

## 1. 환경 (Environment)

### 1.1 시스템 컨텍스트

xflow 는 IoT/HVAC 데이터 스트림 처리를 위한 FBP 게이트웨이다. 현재 다음 HVAC 프로토콜이 통합되어 있다:

- **Samsung NASA** (`internal/agent/samsung/`): 능동 폴링 + 디코딩 (master/slave 모두 수행)
- **LG LGCP / LGAP** (`internal/agent/lg/lgcp_*.go`, `lgap_*.go`): 능동·패시브 혼합
- **LG LGCNP-01** (`internal/agent/lg/lgcnp_*.go`): RS-485 회선 **패시브 캡처 전용**, 다층 검증 + ring buffer + DeviceProvider 패턴 확립

본 SPEC 은 **Century 시스템 에어컨**용 신규 에이전트를 추가한다. Century 프로토콜은 마스터-슬레이브 바이너리 프로토콜로, xflow 는 기존 마스터(상위 컨트롤러) ↔ 슬레이브(에어컨 본체) RS-485 회선에 **passive tap** 하여 양방향 프레임을 모두 디코딩한다. 송신은 일절 수행하지 않는다.

### 1.2 기술 스택

- **언어**: Go 1.25.6
- **시리얼 통신**: SPEC-SERIAL-001 의 `internal/agent/serial` 패키지 또는 동등한 raw 바이트 스트림 트랜스포트 사용 (frame scanner 는 본 에이전트 내부에 둠)
- **라이프사이클**: `pkg/lifecycle.BaseLifecycle`
- **메시지 시스템**: `pkg/message.Message`, payload timestamp 는 epoch milliseconds (`int64`, `UnixMilli()`) — 프로젝트 컨벤션
- **에이전트 프레임워크**: `internal/agent.Agent` 인터페이스 + 선택 인터페이스(`MessageReceiver`, `StatefulAgent`, `TransportChecker`, `BufferInfoProvider`)
- **디바이스 관리**: `internal/device.DeviceProvider` 인터페이스
- **노드 프레임워크**: `internal/node` (`Node`, `SourceNode`, `MultiSourceNode`)
- **Web UI**: React + TypeScript (`web/src/config/agentSchemas.ts`, `web/src/config/nodeSchemas.ts`)
- **참조 구현 템플릿**:
  - 파일 레이아웃·등록 패턴·인터페이스 합성: Samsung NASA (`internal/agent/samsung/agent.go`, `crc.go`, `frame_scanner.go`, `protocol.go`, `register.go`, `provider.go`)
  - 패시브 캡처 + 다층 검증 + ring buffer + 노드 구성: LG LGCNP-01 (`internal/agent/lg/lgcnp_*.go`, `internal/node/lgcnp.go`)

### 1.3 LGCNP 와의 핵심 차이점

| 항목 | LGCNP-01 | Century |
|------|----------|---------|
| 물리 계층 | RS-485, 1200 bps, 8N1 | RS-485, 보레이트는 캡처 환경 의존 (사용자 설정) |
| 프레임 구분 | STX 패턴(0x58 / 0x81–0x85) + 고정 길이 | 8B 헤더(LE) + 가변 payload + 2B CRC. `payload_length` 로 경계 결정 |
| 무결성 검증 | 6계층(이중 기록 + 체크섬 + 물리 범위 + 변화율 + SEQ 순서) | 4계층(길이 → CRC → 헤더 → 페이로드 prefix → 레지스터별 길이) |
| CRC | 없음(SEQ 01/04/05 만 XOR/SUM 보조 체크섬) | **CRC-16/ARC** (poly 0x8005, init **0x0000**, RefIn/RefOut, LE 저장) — Modbus RTU(init 0xFFFF) 와 구분 |
| 주소 | 가변(IDU 0x81–0x85 / ODU 무주소) | Master=0x0030, Slave=0x0001 (LE u16) |
| 폴링 주기 | 약 4–6초(20B+40B×5 IDU 한 사이클) | 약 511.9 ms (σ=0.3 ms) per cycle |
| Function code | 없음(STX 가 유형 분류) | `0x06`=ACK, `0x0B`=Read, `0x0C`=Write |
| 레지스터 | 패킷 유형별 고정 오프셋 | 페이로드 prefix(`sub_dev_id`, reserved, register) + 레지스터별 data |
| 운전 방향 | 패시브 read-only | **패시브 read-only**(WRITE 프레임도 회선상에서 캡처·디코딩하나 본 에이전트가 송신하지는 않음) |

### 1.4 제약 (Constraints)

- **패시브 전용**: 본 에이전트는 RS-485 회선에 **수신 전용(RX-only)** 으로 부착된다. 어떤 경우에도 회선에 바이트를 송신하지 않는다. 시리얼 트랜스포트의 Write 경로를 호출해서는 안 된다.
- **반이중 단일 tap**: RS-485 반이중 회선에 직접 청취만 하므로 회선 충돌을 야기하지 않는다. 외부 컨트롤러의 폴링 사이클은 본 에이전트의 활성·비활성과 무관하게 계속된다.
- **재해석 가능성**: 미확정 필드(특히 reg 0x04 의 `status_bits`, `op_val_1/2`, WRITE 의 `write_live_0/1/14/15`)는 추가 캡처로 의미가 확정되면 SPEC 후속 버전에서 필드명을 갱신할 수 있다. payload 의 `confirmation_status` 마커가 이러한 진화를 가시화한다.

---

## 2. 가정 (Assumptions)

- **A1**: Century HVAC 실외기/실내기와 기존 마스터 컨트롤러는 xflow 가 비파괴적으로 tap 할 수 있는 전용 RS-485 버스를 사용한다.
- **A2**: 마스터의 폴링 주기는 약 511.9 ms (σ=0.3 ms) 이며, 한 사이클은 `READ reg 0x02 → RESP → READ reg 0x03 → RESP → READ reg 0x04 → RESP → WRITE reg 0x04 → ACK → WRITE reg 0x04 → ACK` 의 9 프레임으로 구성된다. WRITE 가 두 번 동일 전송되는 것은 신뢰성 목적의 redundancy 로 추정된다.
- **A3**: CRC-16/ARC 알고리즘은 프로토콜 문서 §3 및 §9 의 Python 참조 구현과 비트 단위 동일한 결과를 산출한다. 모든 캡처(CAP-1 ~ CAP-4)에서 CRC 일치율 99.7% 이상이 검증되었다.
- **A4**: 마스터 주소는 `0x0030`, 슬레이브 주소는 `0x0001`, `sub_dev_id` 는 `0x3B` 가 관측된 단일 indoor unit 환경이다. 다른 환경에서는 이 값들이 다를 수 있으므로 설정으로 노출한다. 다중 indoor unit (`sub_dev_id` 가 여러 개) 환경은 본 SPEC 의 v0.1.0 범위에 **포함** 되며(아래 A7 참조), 데이터 모델은 sub_dev_id 를 키로 한 자동 발견을 지원한다.
- **A5**: 모드 코드 중 `0x00`(꺼짐/대기)과 `0x01`(냉방)은 ground truth 로 확정되었으며, `0x02` 이상(난방/제습/송풍 등)은 미관측이다. 후속 캡처로 추가될 때 SPEC 의 enum 을 **additive** 하게 확장한다 — 기존 코드 호환을 깨지 않는다.
- **A6**: 미확정 바이트의 의미는 후속 SPEC 개정에서 변경될 수 있다. 이 SPEC 은 `confirmation_status` 라는 payload 마커(`confirmed` / `inferred` / `unknown`)로 각 필드의 신뢰도를 명시하여, downstream 컨슈머가 안정성을 알고 사용하도록 한다.
- **A7**: 다중 indoor unit 자동 발견을 v0.1.0에 포함한다. 각 `sub_dev_id`별로 독립된 `CenturyDevice` 인스턴스를 관리하며, 캡처된 모든 `sub_dev_id`에 대해 device tracking을 수행한다. 현재 검증된 캡처(CAP-1~CAP-4)는 단일 유닛(`sub_dev_id=0x3B`)뿐이므로 다중 유닛 코드 경로는 단위 테스트 및 추론 기반 시나리오로 검증한다. 본 에이전트는 패시브 캡처 전용이므로 "에어컨 제어"(reg 0x04 WRITE 프레임 전송)는 일절 수행하지 않는다 — 회선상에서 관측되는 WRITE 프레임은 외부 마스터의 명령이며 본 에이전트는 이를 디코딩만 한다.
- **A8**: 시리얼 회선 노이즈, 케이블 분리, USB 어댑터 hot-unplug 등 물리 계층 이상은 SPEC-SERIAL-001 의 트랜스포트 계층이 이미 처리한다. 본 에이전트는 그 위에서 프레임 경계 탐지·CRC 검증만 수행한다.
- **A9**: payload 의 모든 timestamp 필드는 프로젝트 컨벤션(`epoch_milliseconds`, `int64`, `time.Time.UnixMilli()`)을 따른다. `time.Time`/RFC3339 문자열을 payload 에 직접 노출하지 않는다.

---

## 3. 요구사항 (Requirements)

### M1: 프레임 스캐너와 CRC

#### REQ-CENTURY-001: 에이전트 타입 등록

시스템은 **항상** `agent.DefaultManager` 에 `"century-hvac"` 에이전트 타입을 등록해야 한다.

- 등록 함수: `RegisterCenturyTypes(mgr *agent.DefaultManager) error`
- 팩토리: `func(config agent.AgentConfig) (agent.Agent, error)`
- LG LGCNP 와 동일한 등록 패턴을 따른다(`internal/agent/lg/lgcnp_register.go` 참조)
- `cmd/xflowd/main.go` 의 부트스트랩에 `century.RegisterCenturyTypes(agentMgr)` 호출을 추가한다.

#### REQ-CENTURY-002: 에이전트 설정 파싱

**WHEN** 사용자가 Century 에이전트를 생성할 때, **THEN** 시스템은 `AgentConfig.Transport.Options` 맵에서 다음 설정을 파싱해야 한다:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| transport_type | string | 선택 | "serial" | `serial` 만 v0.1.0 지원. `tcp-client`/`tcp-server` 는 v0.2.0+ 후속 |
| serial_port | string | serial 모드 필수 | - | 시리얼 포트 경로 (예: `/dev/ttyUSB0`) |
| baud_rate | int | 선택 | 9600 | 캡처 환경에 따라 사용자 설정. 프로토콜 문서가 보레이트를 명시하지 않으므로 사용자가 측정 |
| data_bits | int | 선택 | 8 | 5/6/7/8 |
| stop_bits | int | 선택 | 1 | 1/2 |
| parity | string | 선택 | "none" | none/even/odd/mark/space |
| read_timeout | duration | 선택 | "200ms" | 0 이하는 200ms 로 클램프 |
| master_address | int (hex) | 선택 | 0x0030 | 마스터 주소(LE u16). 캡처 환경 의존 |
| slave_address | int (hex) | 선택 | 0x0001 | 슬레이브 주소(LE u16) |
| sub_dev_id | int (hex) | 선택 | 0x3B | payload prefix 의 sub_dev_id (indoor unit ID 추정) |
| ring_buffer_size | int | 선택 | 128 | 캡처 프레임 ring buffer 크기 |
| offline_timeout | duration | 선택 | "5s" | 폴링 주기(약 512ms)의 약 10배. 이 시간 동안 디바이스 프레임 미수신 시 오프라인 전이 |
| auto_discovery | bool | 선택 | true | 회선상 관측된 `sub_dev_id` 를 디바이스로 자동 등록 |
| notify_interval | duration | 선택 | "0s" | 0 이면 상태 변경 시에만 알림, 양수면 주기적 알림 |
| log_decode_errors | bool | 선택 | false | per-error WARN 로그(CRC 불일치, 페이로드 prefix 위반 등) 토글. 통계 카운터는 항상 증가 |
| log_drops | bool | 선택 | false | ring buffer 가득 참으로 인한 프레임 드롭 시 per-drop WARN 로그 |
| log_unconfirmed_fields | bool | 선택 | false | 미확정(`unknown` 또는 `inferred`) 바이트가 알려진 값 외로 관측될 때 debug 로그 |
| dedupe_writes | bool | 선택 | true | 동일 polling cycle 내 중복 WRITE 프레임을 1개로 합침. raw frame 노드는 dedupe 와 무관하게 모든 frame emit. 자세한 cycle 경계 감지 휴리스틱은 REQ-CENTURY-027 참조 |

#### REQ-CENTURY-003: 프레임 스캐너 (frame scanner)

시스템은 **항상** 바이트 스트림에서 Century 프레임 경계를 탐지해야 한다.

- 입력: `io.Reader` (시리얼 포트 또는 동등한 트랜스포트)
- 출력: 완전한 프레임 바이트(`src(2) + dst(2) + payload_length(2) + reserved(1) + fc(1) + payload(N) + crc(2)` = `10 + N` 바이트)
- 절차:
  1. 헤더 8바이트 읽기 (`src`, `dst`, `payload_length` LE u16, `reserved`, `function_code`)
  2. `payload_length` 가 합리 범위(`0 < N ≤ max_payload_length`, 기본 256) 내인지 검증
  3. `payload` N 바이트 + CRC 2바이트 읽기
  4. 완성된 프레임을 다음 단계(CRC 검증, 헤더 검증, 페이로드 prefix 검증)로 넘긴다
- **WHEN** `reserved` 바이트가 `0x00` 이 아니거나 `function_code` 가 알려진 값(`0x06`/`0x0B`/`0x0C`) 외이면, **THEN** 프레임을 폐기하고 다음 가능한 SOF 후보부터 재동기화한다.
- **IF** 헤더와 트레일 바이트 간 inter-frame gap 이 발생하면(읽기 중 일시 정지), 시스템은 최대 ~50 ms 의 in-frame gap 을 허용해야 한다. 그 이상은 incomplete frame 으로 폐기한다.

#### REQ-CENTURY-004: CRC-16/ARC 구현

시스템은 **항상** 다음 매개변수의 CRC-16/ARC 를 구현해야 한다:

- Polynomial: `0x8005` (reflected `0xA001`)
- Init: **`0x0000`** (Modbus RTU 의 `0xFFFF` 가 아님)
- RefIn / RefOut: `true` / `true`
- XorOut: `0x0000`
- 적용 범위: 프레임의 헤더(8B) + payload(N), CRC 2바이트 제외
- 저장 순서: little-endian (`frame[8+N] = crc & 0xFF`, `frame[8+N+1] = (crc >> 8) & 0xFF`)

**IF** 수신 프레임의 CRC 가 일치하지 않으면, **THEN** 프레임을 폐기하고 `framesInvalid` 카운터를 증가시킨다. `log_decode_errors=true` 이면 WARN 로그를 출력한다. 에이전트는 Error 상태로 전이하지 않는다.

**IF** 어떤 구현이 Modbus init `0xFFFF` 를 사용하면, 본 SPEC 의 캡처 프레임에 대해 검증 실패해야 한다 (회귀 방지 테스트 필수).

#### REQ-CENTURY-005: 헤더와 페이로드 prefix 파싱

**WHEN** 프레임 스캐너가 완성된 프레임을 전달하면, **THEN** 시스템은 다음을 추출해야 한다:

- `src_addr` (LE u16, payload offset 0–1)
- `dst_addr` (LE u16, 2–3)
- `payload_length` (LE u16, 4–5)
- `reserved` (1B, 6, 항상 `0x00`)
- `function_code` (1B, 7)
- `payload` (N B, 8 ~ 8+N-1)
- `crc` (LE u16, 8+N ~ 8+N+1)

**IF** `function_code` ≠ `0x06`(ACK) 이고 `payload_length` ≥ 3 이면, **THEN** 페이로드 prefix 를 추가로 파싱한다:

- `sub_dev_id` (1B, payload[0])
- `reserved2` (1B, payload[1], `0x00`)
- `register` (1B, payload[2], `0x02`/`0x03`/`0x04`)
- `data` (payload[3:])

**WHEN** `payload[1]` 이 `0x00` 이 아니거나 `register` 가 `{0x02, 0x03, 0x04}` 외이면, **THEN** 프레임을 미확정(`unknown`)으로 분류하고 raw frame 노드로만 노출한다(디코드된 status 이벤트 생성 X). 통계로 `payloadPrefixInvalid` 를 증가시킨다.

#### REQ-CENTURY-006: Register 0x02 응답 디코딩 (Read response, 17B data)

**WHEN** `function_code=0x06` 이고 `register=0x02` 이며 `data` 길이가 17B 이면, **THEN** 시스템은 다음 17 바이트를 모두 타입드 필드로 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `reg02_byte_0` | u8 | unknown | 4 캡처 모두 `0x00` |
| 1 | `mode` | u8 enum | **confirmed** | 운전 모드 (`0x00`=off/standby, `0x01`=cooling, 기타 미관측) |
| 2 | `fan` | u8 | **confirmed** | 바람 세기 (CAP-3: `0x11`=17, 1~3단 아닌 수치형 step) |
| 3–6 | `reg02_byte_3` ~ `reg02_byte_6` | u8 | unknown | 4 캡처 모두 `0x00` |
| 7–8 | `setpoint_c` | LE u16 ÷ 10.0 | **confirmed** | 설정 온도 (`0x00FA`=25.0℃) |
| 9–10 | `reg02_byte_9` ~ `reg02_byte_10` | u8 | unknown | 4 캡처 모두 `0x00` |
| 11–12 | `reg02_word_11` | LE u16 ÷ 10.0 | inferred | 25.0℃ 관측, setpoint 복제 또는 다른 슬롯 가능성 |
| 13 | `reg02_live_13` | u8 | inferred | CAP-3 `0x1B`=27, 운전 시 채워짐 |
| 14 | `reg02_live_14` | u8 | inferred | CAP-3 `0x39`↔`0x38` 미세 변동, 라이브 센서 추정 |
| 15 | `reg02_live_15` | u8 | inferred | CAP-3 `0x39`=57 |
| 16 | `reg02_byte_16` | u8 | unknown | reserved 추정 |

`mode` enum 디코딩은 **additive**: `0x00`/`0x01` 외 값은 `mode_unknown_<hex>` 문자열로 노출하고 `confirmation_status=unknown` 마커를 부여한다. 후속 SPEC 에서 확정된 값을 enum 에 추가할 때 기존 동작을 깨지 않는다.

**IF** `data` 길이가 17B 가 아니면, **THEN** 프레임을 `lengthMismatch` 로 폐기한다.

#### REQ-CENTURY-007: Register 0x03 응답 디코딩 (Read response, 16B data)

**WHEN** `function_code=0x06` 이고 `register=0x03` 이며 `data` 길이가 16B 이면, **THEN** 시스템은 다음을 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0–1 | `temp_evap_a_c` | LE u16 ÷ 10.0 | **confirmed** | 증발기 냉매 배관 온도 word0 (위치 미지정, "a") |
| 2–3 | `temp_evap_b_c` | LE u16 ÷ 10.0 | **confirmed** | 증발기 냉매 배관 온도 word1 ("b") |
| 4–15 | `reg03_pad_4` ~ `reg03_pad_15` | u8 | unknown | 4 캡처 모두 `0x00` (zero-padding) |

확정 근거: CAP-3(과도, 19.5/19.5℃) → CAP-4(정상, 9.0/8.5℃) 거동으로 증발기 냉매 배관 온도 2개 독립 센서임이 확정됨(프로토콜 문서 §6.2 v0.2→v0.3).

#### REQ-CENTURY-008: Register 0x04 응답 디코딩 (Read response, 14B data)

**WHEN** `function_code=0x06` 이고 `register=0x04` 이며 `data` 길이가 14B 이면, **THEN** 시스템은 다음을 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `status_bits` | u8 bitmap | inferred | 캡처별 변동(`0x63`/`0x4D`/`0x3B`/`0x39`). 전원/모드/동적 비트 혼합 추정 |
| 1 | `reg04_const_1` | u8 | inferred | 4 캡처 모두 `0xF6` |
| 2 | `reg04_const_2` | u8 | inferred | 4 캡처 모두 `0x09` |
| 3–6 | `reg04_byte_3` ~ `reg04_byte_6` | u8 | unknown | 4 캡처 모두 `0x00` |
| 7 | `reg04_const_7` | u8 | inferred | 4 캡처 모두 `0x2C` |
| 8–9 | `op_val_1` | LE u16 | inferred | 정상운전 시 채워짐 (CAP-4: 996). 압축기 주파수/소비전력/적산값 추정 |
| 10–11 | `temp_A_c` | LE u16 ÷ 10.0 | inferred | 운전 중 `25.2℃` 안정. 실내/리턴에어 온도 추정 |
| 12–13 | `op_val_2` | LE u16 | inferred | 운전 부하 변동 (CAP-3: 252, CAP-4: 1248) |

`status_bits` 는 개별 비트 디코딩을 시도하지 않는다(미확정). 전체 바이트를 그대로 노출하여 downstream 분석을 가능하게 한다.

#### REQ-CENTURY-009: Register 0x04 Write 요청 디코딩 (Write request, 16B data)

**WHEN** `function_code=0x0C` 이고 `register=0x04` 이며 `data` 길이가 16B 이면, **THEN** 시스템은 다음을 노출해야 한다(본 에이전트가 송신하는 것이 아니라 회선상 관측된 마스터의 명령):

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `write_live_0` | u8 | inferred | 운전 중 set, CAP-4: `0x02` |
| 1 | `write_live_1` | u8 | inferred | 운전 중 set, CAP-4: `0x04` |
| 2–3 | `write_byte_2` ~ `write_byte_3` | u8 | unknown | 4 캡처 모두 `0x00` |
| 4 | `mode_cmd` | u8 enum | **confirmed** | 마스터의 모드 명령 (`0x00`=off, `0x01`=cooling, 기타 미관측). REQ-CENTURY-006 의 `mode` enum 과 동일 디코딩 규칙 |
| 5–13 | `write_byte_5` ~ `write_byte_13` | u8 | unknown | 4 캡처 모두 `0x00` |
| 14 | `write_byte_14` | u8 | inferred | 캡처별 `0xC4`/`0xC7`/`0xC0` (19.2~19.9 범위). 마스터 측 설정/펌웨어 추정, 미확정 |
| 15 | `write_live_15` | u8 | inferred | CAP-4 내 `0x0F`~`0x11` 불규칙 변동. 마스터 측 라이브 센서값 추정 |

이 디코딩은 **passive observation** 임을 명확히 한다. 본 에이전트는 어떠한 경우에도 WRITE 프레임을 송신하지 않는다.

#### REQ-CENTURY-010: ACK 프레임 처리 (1-byte payload)

**WHEN** `function_code=0x06` 이고 `payload_length=1` 이고 `payload=[0x00]` 이면, **THEN** 시스템은 이를 `ack` 이벤트로 노출해야 한다.

- `src_addr` 가 슬레이브(`0x0001`)인 ACK 는 직전 WRITE 에 대한 슬레이브의 확인이다
- ACK 프레임은 페이로드 prefix(sub_dev_id/register) 가 없으므로 별도 디코더로 처리한다

#### REQ-CENTURY-011: 다층 검증 순서

시스템은 **항상** 수신 프레임에 대해 다음 검증을 순서대로 수행해야 한다:

1. **길이 검증**: `len(frame) == 10 + payload_length`
2. **CRC 검증**: REQ-CENTURY-004 의 CRC-16/ARC 계산값 일치
3. **헤더 검증**: `reserved == 0x00`, `function_code ∈ {0x06, 0x0B, 0x0C}`
4. **페이로드 prefix 검증** (ACK 제외): `payload[1] == 0x00`, `register ∈ {0x02, 0x03, 0x04}`
5. **레지스터별 길이 검증**: 각 (function_code, register, role) 조합에 대해 정확한 `data` 길이

각 단계에서 실패한 프레임은 해당 단계의 통계 카운터를 증가시키고 후속 단계를 건너뛴다.

#### REQ-CENTURY-012: 캡처 ring buffer 와 드롭 처리

시스템은 **항상** 검증된 프레임을 ring buffer 에 저장해야 한다.

- 크기: `ring_buffer_size` (기본 128, 폴링 주기 약 512ms 기준 약 65초 분량)
- 동시성: atomic / lock-free 또는 short-lived mutex
- **WHEN** ring buffer 가 가득 차고 새 프레임이 들어오면, **THEN** 가장 오래된 프레임을 evict 하고 `framesDropped` 카운터를 증가시킨다.
- **IF** `log_drops=true` 이면, drop 시 per-drop WARN 로그를 출력한다(기본 `false`).
- 모든 드롭은 `log_drops` 값과 무관하게 `framesDropped` 통계로 계수되어야 한다 (samsung-nasa `log_decode_errors` 패턴과 일관).

### M2: 디바이스 관리

#### REQ-CENTURY-013: 디바이스 자동 발견 (다중 IDU 포함)

**WHEN** `auto_discovery=true` 이고 회선상 관측된 프레임의 `sub_dev_id` 가 처음 등장하면, **THEN** 시스템은 `CenturyDevice` 를 자동 등록해야 한다.

- 디바이스 키: `sub_dev_id` (예: `"3b"`)
- `Source`: `"auto"` (자동 발견) / `"config"` (사용자 설정)
- `Label`: 기본 `"indoor-<sub_dev_id_hex>"`, 사용자 설정으로 override 가능

**WHEN** 새로운 `sub_dev_id` 가 frame 에서 관측되면, **THEN** 시스템은 자동으로 새 `CenturyDevice` 인스턴스를 생성하고 `Source="auto"` 로 표시해야 한다. 기존 device 는 `sub_dev_id` 를 키로 lookup 되며 무관한 `sub_dev_id` frame 은 별도 device 로 분리되어 추적되어야 한다.

- v0.1.0 은 다중 indoor unit 자동 발견을 **포함** 한다 (A7 참조). 현재 검증된 캡처는 단일 유닛(`0x3B`)뿐이지만, `devices map[uint8]*CenturyDevice` 데이터 모델이 임의 개수의 sub_dev_id 를 키로 관리하며, 각 device 의 상태는 독립적으로 유지된다.
- 다중 IDU 코드 경로는 단위 테스트(합성 frame 의 다중 sub_dev_id 매핑) 와 추론 기반 시나리오(B1a/B1b) 로 검증한다 — 실제 다중 유닛 캡처가 확보되면 후속 SPEC 에서 명시적 ground truth 추가.

#### REQ-CENTURY-014: 온라인/오프라인 감지

**WHEN** 디바이스에서 `offline_timeout` 기간 동안 어떤 프레임도 수신되지 않으면, **THEN** 시스템은 해당 디바이스를 오프라인으로 전이해야 한다.

- 기본 `offline_timeout` = 5초 (폴링 주기 약 512ms 의 약 10배)
- 오프라인 전이 시 `onDeviceStateChange` 콜백 호출
- 디바이스의 마지막 알려진 status 는 보존(다음 프레임 수신까지 stale 표시)

#### REQ-CENTURY-015: DeviceProvider 인터페이스 구현

시스템은 **항상** `device.DeviceProvider` 인터페이스를 구현하여 디바이스 목록을 노출해야 한다.

- `ListDevices() []device.Device`
- 각 디바이스는 `ID`, `Name`, `Properties` (현재 status 의 평탄화된 맵) 노출
- 통일 속성명(LGCNP/NASA 와 정렬): `power`, `mode`, `fan_speed`, `target_temp`, `current_temp`
- Century 전용 속성: `temp_evap_a`, `temp_evap_b`, `op_val_1`, `op_val_2`, `status_bits`

### M3: 플로우 노드

#### REQ-CENTURY-016: CenturyStatusNode (SourceNode)

시스템은 **항상** `century-status` 노드 타입을 제공해야 한다.

- 인터페이스: `SourceNode` (폴링 기반)
- 설정 필드: `agent_ref` (필수), `poll_interval` (기본 `100ms`), `timeout` (기본 `5s`), `poll_command` (기본 `drain`), `recent_count` (기본 10), `batch_size` (기본 32)
- 동작: 에이전트의 ring buffer 에서 디코딩된 reg 0x02/0x03/0x04 응답을 폴링하여, 각 프레임을 개별 status 이벤트 메시지로 `out` 포트에 송출
- `FrameNotifyCh` 지원: 에이전트 알림 즉시 반응
- 메타데이터: `century_source="poll_bulk"`, `century_node_id=<node id>`

#### REQ-CENTURY-017: CenturyControlNode (ProcessNode, not_supported)

시스템은 **항상** `century-control` 노드 타입을 제공해야 한다 (API 대칭성용 placeholder).

- 인터페이스: `Node.Process(ctx, msg)`
- **WHEN** 어떤 메시지가 `Process` 로 전달되면, **THEN** 노드는 **항상** 다음 응답을 반환해야 한다:

  ```json
  {
    "status": "not_supported",
    "reason": "century_passive_only",
    "message": "Century HVAC agent operates in passive sniff mode; control commands are never transmitted",
    "request": <원본 메시지의 payload 요약>
  }
  ```

- 노드는 절대로 에이전트의 트랜스포트 Write 경로를 호출하지 않는다.
- `error` 포트가 아닌 `out` 포트로 응답을 반환한다(downstream 로직이 status 필드로 분기 가능).

#### REQ-CENTURY-018: CenturyNode (combined SourceNode + ProcessNode)

시스템은 **항상** `century` 통합 노드 타입을 제공해야 한다.

- SourceNode 동작: REQ-CENTURY-016 과 동일하게 status 이벤트 송출
- ProcessNode 동작: 입력 메시지에 제어 키(`power`, `mode`, `temperature`, `setpoint`, `fan_speed`)가 포함되면 REQ-CENTURY-017 의 not_supported 응답을 반환. 제어 키가 없으면 상태 조회(`get_stats`/`get_recent`) 결과를 반환.
- LGCNP 의 `LGCNPNode` 통합 패턴과 일관 (`internal/node/lgcnp.go`)

#### REQ-CENTURY-019: CenturyRawFrameNode (SourceNode, 디버깅·역공학용)

시스템은 **항상** `century-raw-frame` 노드 타입을 제공해야 한다.

- 인터페이스: `SourceNode`
- 동작: 에이전트가 캡처한 **모든** 프레임(유효/무효 모두)을 디코딩 적용 **이전**의 원시 바이트로 송출
- payload 구조:
  - `raw` (bytes): 헤더 + payload + CRC 전체 (`10 + N` 바이트)
  - `src_addr`, `dst_addr` (u16)
  - `function_code` (u8)
  - `payload_length` (u16)
  - `register` (u8 or null, ACK 는 null)
  - `crc_ok` (bool)
  - `validation_stage` (string): `"length"` / `"crc"` / `"header"` / `"payload_prefix"` / `"register_length"` / `"ok"` — 어느 단계까지 통과했는지
  - `confirmation_status`: `"raw"` (마커 자체가 raw 임을 명시)
  - `timestamp` (`int64`, epoch milliseconds)
- 용도: 디버깅, 프레임 capture, 후속 캡처를 통한 미확정 필드 의미 확정 작업 지원

### M4: 메시지 페이로드 스키마

#### REQ-CENTURY-020: 타입드 페이로드 스키마와 confirmation_status 마커

시스템은 **항상** 디코딩된 프레임의 모든 바이트를 페이로드의 타입드 필드로 노출해야 한다.

- 각 필드 그룹별 분리된 객체 구조(예시는 §4.4 참조)
- 모든 필드는 다음 메타와 함께 노출:
  - `value`: 실제 값(숫자/문자열)
  - `raw`: 원시 바이트 값(hex 또는 u8/u16)
  - `confirmation_status`: `"confirmed"` / `"inferred"` / `"unknown"` 중 하나
- 모든 timestamp 는 `epoch_milliseconds` (`int64`, `UnixMilli()`) — 프로젝트 컨벤션 (`project_timestamp_convention.md`)

#### REQ-CENTURY-021: confirmation_status 분류 기준

시스템은 **항상** 다음 분류 기준을 적용해야 한다:

- **confirmed**: 프로토콜 문서 §8.1 "확정" 섹션에 해당. CAP-3/CAP-4 의 ground truth 또는 명확한 물리적 거동(예: 증발기 냉매 배관 온도)으로 검증됨
- **inferred**: 4개 캡처에서 일관된 패턴 또는 합리적 추론. 의미는 확실치 않으나 라이브 변동, 일관 상수 등 동작 특성이 확인됨
- **unknown**: 4개 캡처 모두 `0x00` 인 reserved/zero-padding 또는 의미 추정 불가
- 향후 캡처로 의미 확정 시: SPEC 후속 버전(0.2.0+)에서 `unknown` → `inferred` → `confirmed` 진화. 이때 필드명은 **유지** 하고 `confirmation_status` 만 갱신한다(downstream 호환). 의미가 달라질 때만 새 필드명을 추가하고 구필드명은 `deprecated` 마킹.

### M5: Web UI 스키마

#### REQ-CENTURY-022: Web UI 에이전트 스키마

시스템은 **항상** `web/src/config/agentSchemas.ts` 에 다음을 추가해야 한다:

- `AGENT_TYPES` 배열에 `{ value: 'century-hvac', label: 'Century HVAC (passive)' }`
- `CENTURY_HVAC_FIELDS` 상수: REQ-CENTURY-002 의 모든 설정 필드를 `ConfigField` 로 노출
- `transport_type` 의 visibleWhen 으로 `serial_port`, `baud_rate`, `data_bits`, `stop_bits`, `parity` 표시 제어
- `master_address`/`slave_address`/`sub_dev_id` 는 hex 입력 허용 (예: `"0x3B"` 또는 `"3B"`)

#### REQ-CENTURY-023: Web UI 노드 스키마

시스템은 **항상** `web/src/config/nodeSchemas.ts` 에 다음을 추가해야 한다:

- `century-status` (카테고리: io, 출력 포트: `out`/`error`)
- `century-control` (카테고리: io, 출력 포트: `out`/`error`, 설명에 "제어 미지원 (패시브 전용)" 명시)
- `century` 통합 (카테고리: io)
- `century-raw-frame` (카테고리: io 또는 debug, 출력 포트: `out`)
- 모든 노드의 `agent_ref` 옵션에 `century-hvac` 포함

### M6: 품질 게이트와 운영

#### REQ-CENTURY-024: 테스트 커버리지와 골든 픽스처

시스템은 **항상** 다음 테스트를 포함해야 한다:

- `go test -race ./internal/agent/century/...` 통과
- `go test -race ./internal/node/...` 통과 (century 노드 포함)
- 패키지 커버리지 ≥85%
- 골든 픽스처: 프로토콜 문서 부록 A(CAP-3 냉방 시작), 부록 B(CAP-1 꺼짐), 부록 C(CAP-4 냉방 정상) 의 원시 프레임 hex 를 testdata 로 보존하고, 디코딩 결과의 회귀를 검증한다
- CRC 회귀 테스트: Modbus init `0xFFFF` 사용 구현이 본 SPEC 의 캡처 프레임에 실패함을 명시적으로 검증 (REQ-CENTURY-004)

#### REQ-CENTURY-025: 구조화 로그와 통계

시스템은 **항상** 다음 통계를 atomic 카운터로 추적해야 한다:

- `framesCaptured` (전체 캡처 시도)
- `framesValid` (다층 검증 통과)
- `framesInvalid` (검증 실패: 단계별 세분화 — `lengthMismatch`, `crcMismatch`, `headerInvalid`, `payloadPrefixInvalid`, `registerLengthInvalid`)
- `framesDropped` (ring buffer 가득 참)
- `bytesReceived`
- 디코딩별 카운터: `reg02ResponseCount`, `reg03ResponseCount`, `reg04ResponseCount`, `reg04WriteCount`, `ackCount`, `unconfirmedFieldObservations`

구조화 로그(`slog`)는 다음을 출력한다:

- CRC 불일치 (level: WARN, `log_decode_errors=true` 일 때만 per-error)
- ring buffer 드롭 (level: WARN, `log_drops=true` 일 때만 per-drop)
- 미확정 필드의 새 관측값 (level: DEBUG, `log_unconfirmed_fields=true` 일 때만; 예: `mode` 가 `0x02` 이상 처음 관측)
- 디바이스 자동 발견 (level: INFO, 1회)
- 디바이스 오프라인/온라인 전이 (level: INFO)

#### REQ-CENTURY-026: 확장성 — additive enum 과 reserved 필드명

시스템은 **항상** 다음 확장 정책을 따라야 한다:

- `mode`/`mode_cmd` enum 은 **additive only**: 새 코드(`0x02` 이상)가 확정되면 enum 에 **추가** 한다. 기존 코드(`0x00`/`0x01`) 의 의미를 변경하지 않는다.
- 미확정(`unknown`/`inferred`) 필드의 이름은 캡처 위치 기반으로 명명(`reg02_byte_3`, `write_live_15` 등) 하여, 의미가 확정되면 새 alias 필드명을 추가하고 기존 이름을 `deprecated` 마킹한다. 기존 이름의 값은 새 이름과 동일하게 유지된다(transitional duplication).
- payload 스키마의 새 필드 추가는 SemVer minor 버전 증가, 의미 변경은 major 버전 증가에 해당한다.

#### REQ-CENTURY-027: WRITE 중복 제거

Century 마스터는 신뢰성 목적으로 각 polling cycle 마다 동일 WRITE 프레임을 두 번 전송한다(A2 참조). 이 redundancy 를 다운스트림에 그대로 노출하지 않기 위한 옵션을 제공한다.

**WHEN** `dedupe_writes=true`(기본값)이고 같은 polling cycle 내에서 동일한 raw payload 를 가진 두 번째 WRITE 프레임이 관측되면, **THEN** 시스템은 두 번째 프레임에 대한 decoded event 를 emit 하지 않아야 한다(첫 번째만 emit). raw frame 노드는 dedupe 와 무관하게 모든 frame 을 emit 한다.

**IF** `dedupe_writes=false` 이면 **THEN** 모든 WRITE 프레임이 그대로 emit 되어야 한다.

**WHEN** cycle 경계 감지가 필요할 때, **THEN** 시스템은 마지막 READ response(reg `0x04` 응답) 또는 ACK 직후를 cycle 시작점으로 간주한다. 또는 inter-frame idle 이 `100ms` 를 초과하면 새 cycle 로 간주한다.

- 중복 판정 기준: `(function_code, sub_dev_id, register, payload data 의 raw bytes)` 가 동일
- dedup 된 frame 은 `framesValid` 통계에는 계수되지만 별도 카운터(`writesDeduped`)로도 추적
- `log_drops=true` 인 경우 dedup 된 frame 도 DEBUG 레벨로 로깅 가능 (운영자가 cycle 경계 휴리스틱을 검증할 수 있게)

---

## 4. 명세 (Specifications)

### 4.1 파일 구조 (신규)

```
internal/agent/century/
  agent.go           # CenturyAgent: lifecycle + capture loop + interface 구현
  config.go          # CenturyConfig + parseCenturyConfig
  device.go          # CenturyDevice, CenturyDeviceState, DeviceProvider 구현
  errors.go          # 센티널 에러 정의
  crc.go             # CRC-16/ARC 구현 (init 0x0000)
  frame_scanner.go   # 헤더 → payload_length → CRC 경계 탐지
  frame.go           # CenturyFrame 구조체, src/dst/fc/register 접근자
  register.go        # 디코더 디스패치 (function_code × register × role → decoder fn)
  decoder_reg02.go   # Register 0x02 응답 디코더 (17B)
  decoder_reg03.go   # Register 0x03 응답 디코더 (16B)
  decoder_reg04.go   # Register 0x04 응답 + Write 디코더
  decoder_ack.go     # ACK 프레임 디코더
  message.go         # MessagePayload 구성 (typed fields + confirmation_status)
  provider.go        # DeviceProvider 어댑터
  ring_buffer.go     # 캡처 프레임 ring buffer
  serial_opener.go   # 시리얼 트랜스포트 wrapper (SPEC-SERIAL-001 재사용)
  registration.go    # RegisterCenturyTypes (관례상 register.go 와 다른 파일에)
  agent_test.go
  crc_test.go
  frame_scanner_test.go
  decoder_reg02_test.go
  decoder_reg03_test.go
  decoder_reg04_test.go
  decoder_ack_test.go
  testdata/
    cap1_off.hex        # CAP-1 한 사이클 (꺼짐)
    cap3_cool_start.hex # CAP-3 한 사이클 (냉방 시작)
    cap4_cool_steady.hex# CAP-4 한 사이클 (냉방 정상)

internal/node/
  century.go            # CenturyStatusNode, CenturyControlNode, CenturyNode, CenturyRawFrameNode
  century_test.go

web/src/config/
  agentSchemas.ts       # CENTURY_HVAC_FIELDS 추가
  nodeSchemas.ts        # century-status, century-control, century, century-raw-frame 추가

cmd/xflowd/
  main.go               # century.RegisterCenturyTypes(agentMgr) 호출 추가 (보통 lg.RegisterLGCNPTypes 인근)

examples/config/
  century-hvac-passive.yaml  # 패시브 캡처 샘플 설정
```

### 4.2 인터페이스 합성

`CenturyAgent` 는 다음 인터페이스를 구현한다 (NASA 패턴 참조). 다중 IDU 지원을 위해 내부 상태는 `devices map[uint8]*CenturyDevice` (sub_dev_id 를 키로) 로 보관하며, 모든 디코딩 이벤트는 해당 `sub_dev_id` 의 device 인스턴스로 라우팅된다:

- `agent.Agent` (필수: Init/Start/Stop/Pause/Resume/Configure/Process/ID/Name/Type/Info/Stats/Health)
- `agent.MessageReceiver` (`ReceiveMessage() <-chan []byte`)
- `agent.StatefulAgent` (`State() agent.AgentState`)
- `agent.TransportChecker` (`TransportConnected() bool`)
- `agent.BufferInfoProvider` (`BufferInfo() agent.BufferInfo`, `FrameNotifyCh() <-chan struct{}`)
- `device.DeviceProvider` (`ListDevices() []device.Device`)

`Process()` 커맨드는 다음을 지원한다:

- `get_stats`: 캡처 통계 JSON 반환
- `get_recent`: 최근 프레임 조회 (`count`, `last_seq` 인자)
- `drain`: ring buffer 의 모든 프레임 소비
- 그 외 알려지지 않은 커맨드: `not_supported` 오류 반환

### 4.3 디코더 디스패치 (register.go)

```go
type decoderKey struct {
    fc       byte // 0x06 / 0x0B / 0x0C
    register byte // 0x02 / 0x03 / 0x04
    role     role // master / slave (src_addr 기반 판정)
}

type decoderFn func(frame *CenturyFrame) (CenturyMessage, error)

var decoders = map[decoderKey]decoderFn{
    {0x06, 0x02, slaveResp}: decodeReg02Response,
    {0x06, 0x03, slaveResp}: decodeReg03Response,
    {0x06, 0x04, slaveResp}: decodeReg04Response,
    {0x0C, 0x04, masterReq}: decodeReg04Write,
    // ACK 는 payload_length=1 로 별도 분기
}
```

### 4.4 메시지 페이로드 예시 (REQ-CENTURY-020)

#### 예시 1: Register 0x02 응답 (CAP-3, 냉방 25℃, 바람 17)

```json
{
  "type": "century_reg02_response",
  "timestamp": 1737216000123,
  "seq": 1024,
  "src_addr": 1,
  "dst_addr": 48,
  "sub_dev_id": 59,
  "register": 2,
  "raw_hex": "010030001400000663b00020001110000000000fa0000000fa001b3939",
  "fields": {
    "mode":        { "value": "cooling",  "raw": 1,    "confirmation_status": "confirmed" },
    "fan":         { "value": 17,         "raw": 17,   "confirmation_status": "confirmed" },
    "setpoint_c":  { "value": 25.0,       "raw": 250,  "confirmation_status": "confirmed" },
    "reg02_byte_0":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_3":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_4":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_5":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_6":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_9":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_10":  { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_word_11":  { "value": 25.0, "raw": 250,  "confirmation_status": "inferred" },
    "reg02_live_13":  { "value": 27,   "raw": 27,   "confirmation_status": "inferred" },
    "reg02_live_14":  { "value": 57,   "raw": 57,   "confirmation_status": "inferred" },
    "reg02_live_15":  { "value": 57,   "raw": 57,   "confirmation_status": "inferred" },
    "reg02_byte_16":  { "value": 0,    "raw": 0,    "confirmation_status": "unknown" }
  }
}
```

#### 예시 2: Register 0x04 응답 (CAP-4 정상상태)

```json
{
  "type": "century_reg04_response",
  "timestamp": 1737218640456,
  "seq": 5832,
  "sub_dev_id": 59,
  "register": 4,
  "fields": {
    "status_bits":   { "value": 57,   "raw": "0x39", "confirmation_status": "inferred" },
    "reg04_const_1": { "value": 246,  "raw": "0xf6", "confirmation_status": "inferred" },
    "reg04_const_2": { "value": 9,    "raw": "0x09", "confirmation_status": "inferred" },
    "reg04_byte_3":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_4":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_5":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_6":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_const_7": { "value": 44,   "raw": "0x2c", "confirmation_status": "inferred" },
    "op_val_1":      { "value": 996,  "raw": 996,    "confirmation_status": "inferred" },
    "temp_A_c":      { "value": 25.2, "raw": 252,    "confirmation_status": "inferred" },
    "op_val_2":      { "value": 1248, "raw": 1248,   "confirmation_status": "inferred" }
  }
}
```

#### 예시 3: Register 0x04 Write 요청 (CAP-4)

```json
{
  "type": "century_reg04_write_request",
  "observation_mode": "passive",
  "timestamp": 1737218640712,
  "sub_dev_id": 59,
  "register": 4,
  "fields": {
    "write_live_0":  { "value": 2,    "raw": "0x02", "confirmation_status": "inferred" },
    "write_live_1":  { "value": 4,    "raw": "0x04", "confirmation_status": "inferred" },
    "mode_cmd":      { "value": "cooling", "raw": 1, "confirmation_status": "confirmed" },
    "write_byte_14": { "value": 192,  "raw": "0xc0", "confirmation_status": "inferred" },
    "write_live_15": { "value": 17,   "raw": "0x11", "confirmation_status": "inferred" }
  }
}
```

#### 예시 4: Raw frame (REQ-CENTURY-019)

```json
{
  "type": "century_raw_frame",
  "timestamp": 1737216000123,
  "raw": "0100300014000006...",
  "src_addr": 1,
  "dst_addr": 48,
  "function_code": 6,
  "payload_length": 20,
  "register": 2,
  "crc_ok": true,
  "validation_stage": "ok",
  "confirmation_status": "raw"
}
```

### 4.5 예시 YAML 에이전트 설정 (`examples/config/century-hvac-passive.yaml`)

```yaml
agents:
  - id: century-living-room
    type: century-hvac
    transport:
      type: serial
      options:
        serial_port: /dev/ttyUSB0
        baud_rate: 9600
        data_bits: 8
        stop_bits: 1
        parity: none
        read_timeout: 200ms
        master_address: 0x0030
        slave_address: 0x0001
        sub_dev_id: 0x3B
        ring_buffer_size: 128
        offline_timeout: 5s
        auto_discovery: true
        log_decode_errors: false
        log_drops: false
        log_unconfirmed_fields: false
        dedupe_writes: true

flows:
  - id: century-status-flow
    nodes:
      - id: src
        type: century-status
        agent_ref: century-living-room
        poll_interval: 100ms
      - id: log
        type: debug
    wires:
      - { from: src.out, to: log.in }

  - id: century-raw-capture-flow
    nodes:
      - id: src
        type: century-raw-frame
        agent_ref: century-living-room
      - id: store
        type: file-write
        options:
          path: /var/log/xflow/century-frames.jsonl
    wires:
      - { from: src.out, to: store.in }
```

### 4.6 추적성 태그

| 요구사항 | 파일 | 함수/구조체 |
|----------|------|------------|
| REQ-CENTURY-001 | registration.go | RegisterCenturyTypes |
| REQ-CENTURY-002 | config.go | CenturyConfig, parseCenturyConfig |
| REQ-CENTURY-003 | frame_scanner.go | FrameScanner.Read, scanHeader |
| REQ-CENTURY-004 | crc.go | crc16ARC |
| REQ-CENTURY-005 | frame.go | parseHeader, parsePayloadPrefix |
| REQ-CENTURY-006 | decoder_reg02.go | decodeReg02Response |
| REQ-CENTURY-007 | decoder_reg03.go | decodeReg03Response |
| REQ-CENTURY-008 | decoder_reg04.go | decodeReg04Response |
| REQ-CENTURY-009 | decoder_reg04.go | decodeReg04Write |
| REQ-CENTURY-010 | decoder_ack.go | decodeAck |
| REQ-CENTURY-011 | register.go, frame_scanner.go | validate (단계별 검증 체이닝) |
| REQ-CENTURY-012 | ring_buffer.go | RingBuffer.Push (드롭 카운팅), agent.go (log_drops 분기) |
| REQ-CENTURY-013 | device.go | CenturyAgent.discoverDevice |
| REQ-CENTURY-014 | device.go | CenturyAgent.checkOfflineTimeout |
| REQ-CENTURY-015 | provider.go | CenturyDeviceProvider.ListDevices |
| REQ-CENTURY-016 | internal/node/century.go | CenturyStatusNode |
| REQ-CENTURY-017 | internal/node/century.go | CenturyControlNode (not_supported) |
| REQ-CENTURY-018 | internal/node/century.go | CenturyNode |
| REQ-CENTURY-019 | internal/node/century.go | CenturyRawFrameNode |
| REQ-CENTURY-020 | message.go | MessagePayload, FieldMeta |
| REQ-CENTURY-021 | message.go | ConfirmationStatus enum |
| REQ-CENTURY-022 | web/src/config/agentSchemas.ts | CENTURY_HVAC_FIELDS |
| REQ-CENTURY-023 | web/src/config/nodeSchemas.ts | century-* node entries |
| REQ-CENTURY-024 | testdata/*.hex, *_test.go | golden fixture 회귀 테스트 |
| REQ-CENTURY-025 | agent.go | atomic counters, slog 구조화 로그 |
| REQ-CENTURY-026 | decoder_reg02.go, decoder_reg04.go | additive mode/mode_cmd enum |
| REQ-CENTURY-027 | agent.go, register.go | cycle tracker + writeDeduplicator (writesDeduped 카운터) |

---

## 5. 구현 노트 (Implementation Notes)

### 5.1 프레임 경계 탐지 전략

- Century 프레임은 STX 가 없으므로 LGCNP/NASA 와 달리 **헤더 8B 사전 읽기 → `payload_length` 사용 → 정확한 길이 확보** 가 가능하다.
- 재동기화는 헤더 검증 실패 시(reserved≠0x00, fc 미지원, payload_length 비합리) 1바이트씩 shift 하며 다음 후보를 찾는 sliding window 방식.
- inter-frame in-flight gap 은 최대 ~50 ms 허용. 폴링 주기 약 512 ms 에 비해 충분히 짧음.

### 5.2 반이중 회선 sniff 의 송신 금지

- `SerialAgent` 의 `Process()` 가 Write 경로를 트리거하는 통상의 동작과 달리, Century 에이전트는 어떤 경로로도 Write 를 호출해서는 안 된다.
- `CenturyControlNode` 의 `not_supported` 응답은 agent.Process 를 거치지 않고 노드 내부에서 즉시 응답한다 (REQ-CENTURY-017).
- v0.2.0+ 에서 능동 폴링 모드를 추가할 경우, 본 SPEC 과 별도의 SPEC 으로 분리한다(예: SPEC-CENTURY-002 "Active polling and control" — 본 SPEC 의 호환을 깨지 않음).

### 5.3 골든 픽스처 (testdata)

프로토콜 문서 §부록 A/B/C 의 hex 덤프를 testdata 파일로 보존하고, 각 디코더 테스트에서 다음을 검증:

- **CAP-1** (꺼짐): mode=off, fan=0, setpoint=25.0℃, reg04 운전 필드 모두 0
- **CAP-3** (냉방 시작): mode=cooling, fan=17, setpoint=25.0℃, reg03=19.5/19.5℃ (과도), reg04 temp_A=25.2℃ op_val_2=252
- **CAP-4** (냉방 정상): reg03=9.0/8.5℃ (정상 증발기 온도), reg04 op_val_1=996 op_val_2=1248
- **CRC 회귀**: 각 캡처 프레임이 CRC-16/ARC (init `0x0000`) 로 검증 성공, init `0xFFFF` 로는 모두 실패함을 확인

### 5.4 LGCNP 와의 코드 재사용

LGCNP 패시브 캡처 구조에서 다음 패턴을 차용하되 Century 도메인으로 재작성한다:

- ring buffer (`recentFrames` slice + atomic seq counter)
- DeviceProvider 어댑터 패턴 (`internal/agent/lg/lgcnp_device.go` 참조)
- `BufferInfoProvider`/`FrameNotifyCh` 구현
- 노드 base 패턴 (`internal/node/lgcnp.go` 의 `lgcnpNodeBase` → `centuryNodeBase`)
- Init-tolerance / deferred connection (`REQ-SERIAL-016`, LGCNP v1.3 패턴): Century 노드 4종은 Init 시점에 에이전트 resolve 실패 시 hard-fail 하지 않고 deferred connection 으로 Running 전이

NASA 패턴에서 차용하는 것:

- 파일 분할 단위 (frame_scanner / crc / protocol / executor / serial_opener / provider 의 명확한 분리)
- `RegisterXXXTypes(mgr)` 호출 위치 (cmd/xflowd/main.go:309 인근 패턴)
- 에이전트 인터페이스 합성 방식 (`agent.Agent` + `MessageReceiver` + `StatefulAgent` + `TransportChecker` + `BufferInfoProvider`)

### 5.5 MaxPayloadLength = 256 의 근거

프레임 스캐너의 `payload_length` 합리성 검증은 `MaxPayloadLength=256` 을 상한으로 사용한다 (REQ-CENTURY-003). CAP-1~CAP-4 에서 관측된 최대 payload 길이는 reg 0x02 응답의 20B 이지만, 256 으로 보수적으로 잡은 이유:

- 펌웨어 버전 차이로 레지스터별 데이터 길이가 확장될 가능성 (예: 향후 reg 0x05 도입 시)
- 16B/32B 정렬 등 합리적 메모리 예산 안에 머무름
- ring buffer 한 칸당 비용은 frame 헤더 10B + payload(최대 256B) + CRC 2B = ~270B 이므로 기본 128 칸 기준 약 34 KB 로 무시 가능

이 값은 단순히 비합리적 헤더(예: payload_length=0xFFFF) 를 거부하기 위한 sanity gate 이며, 실제 디코더는 레지스터별 정확한 길이를 다시 검증한다 (REQ-CENTURY-011 의 단계 5).

### 5.6 WRITE 중복 제거의 cycle 경계 휴리스틱 (REQ-CENTURY-027)

Century 의 한 polling cycle 은 9 프레임으로 구성되며(A2 참조), 두 번 반복되는 WRITE 가 cycle 내부에 위치한다. 새 cycle 의 시작을 판정하는 신호 우선순위:

1. **1차 신호**: 마지막 READ response(reg `0x04` 응답) 또는 ACK 직후를 cycle 시작점으로 간주. 9 프레임 시퀀스의 구조적 마커이므로 가장 신뢰도 높음.
2. **2차 신호 (fallback)**: inter-frame idle 이 `100ms` 를 초과하면 새 cycle 로 간주. 마스터의 cycle 간 idle 은 약 100ms~수백ms 로 관측됨 (전체 cycle ~511.9ms 중 9 프레임이 차지하는 시간 + idle).

휴리스틱 오작동 시 운영자 진단을 위해 `log_drops=true` 일 때 dedup 된 WRITE frame 도 DEBUG 레벨로 로깅한다(`writesDeduped` 카운터와 함께). 실제 신규 명령이 누락되는 정황이 확인되면 `dedupe_writes=false` 로 fallback 가능.

### 5.7 다중 IDU 테스트 전략 (REQ-CENTURY-013, A7)

현재 검증된 캡처(CAP-1~CAP-4) 는 단일 IDU(`sub_dev_id=0x3B`) 뿐이지만, v0.1.0 의 데이터 모델은 다중 IDU 를 정식 지원한다. 검증은 다음 방식으로 수행:

- **합성 frame 단위 테스트**: CAP-3 의 reg 0x02 응답을 base 로 `sub_dev_id` 만 `0x3C`, `0x3D` 등으로 변경한 합성 frame 을 주입하여, `devices` 맵에 독립 device 가 등록되고 각 device 의 상태가 격리됨을 검증 (B1a/B1b 시나리오).
- **CRC 재계산**: 합성 frame 의 sub_dev_id 변경은 payload 의 일부이므로 CRC 도 재계산해야 함 (테스트 헬퍼 제공).
- **다중 IDU 환경 권장 사항**: 실제 다중 유닛 회선에 배포할 때는 점진적 roll-out — 먼저 `log_decode_errors=true`, `log_unconfirmed_fields=true` 로 운영하여 sub_dev_id 별 frame 패턴을 관측한 뒤, 의심 정황이 없으면 운영 모드로 전환.
- **후속 SPEC**: 실제 다중 IDU 캡처가 확보되면 SPEC-CENTURY-002(또는 본 SPEC v0.2.0) 에서 ground truth 기반 acceptance 추가.

### 5.8 단계별 검증 카운터 (REQ-CENTURY-011, REQ-CENTURY-025)

다층 검증의 각 단계에서 별도 카운터를 증가시켜, 운영 중 회선 품질·프로토콜 deviation 을 진단할 수 있게 한다. 예시:

- `lengthMismatch` 증가 → 프레임 스캐너가 끊긴 바이트 스트림을 보고 있음. 트랜스포트 layer 점검 필요
- `crcMismatch` 비율 ≥ 0.3% → 회선 노이즈 또는 다른 프로토콜이 같은 회선에 섞여 있을 가능성
- `payloadPrefixInvalid` 증가 → `sub_dev_id` 가 설정값과 다름. 다중 unit 환경 가능성
- `registerLengthInvalid` 증가 → 펌웨어 버전 차이로 데이터 길이가 다른 변종 존재 가능

---

*SPEC 버전: 0.1.1*
*초안 작성일: 2026-05-18 (v0.1.0), 갱신: 2026-05-18 (v0.1.1 — 다중 IDU 포함, dedupe_writes 추가)*
*작성자: xtra*
*프로토콜 ground truth: references/protocols/century_hvac_protocol_spec.md v0.3 (CAP-1 ~ CAP-4)*
