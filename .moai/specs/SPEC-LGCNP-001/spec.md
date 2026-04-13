# SPEC-LGCNP-001: LGCNP-01 프로토콜 에이전트 및 플로우 노드 구현

> **SPEC ID**: SPEC-LGCNP-001
> **제목**: LGCNP-01 (LG CN-485 Protocol) 에이전트 및 플로우 노드
> **생성일**: 2026-04-12
> **상태**: Planned
> **우선순위**: High
> **추적성**: LGCNP-01 프로토콜 분석 보고서 (`references/protocols/LGCNP-01_Protocol_Analysis.md`)

---

## 1. Environment (환경)

### 1.1 프로젝트 컨텍스트

- **프로젝트**: xflow (Go 모듈: `github.com/xtra/xflow`)
- **대상 장비**: LG 시스템 에어컨 실내기 **LRD-N837T** + 실외기
- **프로토콜**: LGCNP-01 (LG CN-485 Protocol Version 1)
- **물리 계층**: RS-485, **1200 bps**, 8N1 (반이중)
- **기존 유사 구현**: LGCP 에이전트 (`internal/agent/lg/lgcp_*.go`), LGCP 노드 (`internal/node/lgcp.go`)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **시리얼 통신**: 기존 `LGAPTransport` 인터페이스 재사용
- **라이프사이클**: `pkg/lifecycle.BaseLifecycle`
- **메시지 시스템**: `pkg/message.Message`
- **에이전트 프레임워크**: `internal/agent.Agent` 인터페이스
- **노드 프레임워크**: `internal/node.Node`, `internal/node.SourceNode` 인터페이스
- **디바이스 관리**: `internal/device.DeviceProvider` 인터페이스
- **Web UI**: React + TypeScript (`web/src/config/`)

### 1.3 LGCP와의 핵심 차이점

| 항목 | LGCP | LGCNP-01 |
|------|------|----------|
| 보레이트 | 9600 bps | **1200 bps** |
| 프레임 유형 | 단일 (STX=0x56) | **이중**: TYPE-A (0x58, 20B ODU) + TYPE-B (0x81~0x85, 40B IDU) |
| CRC | CRC-16/XMODEM | **없음** -- 이중 기록 기반 무결성 |
| 무결성 보장 | CRC 체크섬 | **6계층 신뢰성 모델** |
| 주소 체계 | 가변 길이 (DLEN/SLEN) | **고정**: TYPE-A 주소 없음, TYPE-B는 IDU_ADDR (0x81~0x85) |
| 레지스터 | MSB 기반 가변 길이 | **고정 오프셋** (패킷 유형별) |
| 온도 변환 | 원시 레지스터 값 | **전용 공식**: 설정=(b[9]-0x3C), 실내/흡입/토출=((b[n]-0x40)/2.0) |
| IDU 수 | 자동 탐색 | **고정 0x81~0x85** (최대 5대) |
| 프레임 감지 | STX+LEN+ETX | **STX 패턴 기반**: 0x58 (ODU) 또는 0x81~0x85 (IDU), 고정 길이 |

---

## 2. Assumptions (가정)

### 2.1 프로토콜 가정

- [A-01] LGCNP-01 프로토콜은 **읽기 전용(패시브 캡처)**이다. 현재 알려진 쓰기 명령은 없다.
- [A-02] IDU 주소 범위는 0x81~0x85 (5대)로 고정되며, 실제 연결된 IDU만 패킷을 발생시킨다.
- [A-03] ODU(TYPE-A)는 항상 STX=0x58로 시작하며, SEQ=01~05의 5개 서브패킷으로 한 사이클을 구성한다.
- [A-04] TYPE-B(IDU) 패킷의 b[38], b[39]는 센서 파생값이며 체크섬이 아니다 (분석 보고서 확정).
- [A-05] **1200 bps**에서 20바이트 전송은 약 167ms, 40바이트 전송은 약 333ms 소요된다.
- [A-06] 한 사이클(SEQ=01~05 + IDU#1~#5)은 약 4~6초이다.

### 2.2 구현 가정

- [A-07] 기존 `LGAPTransport` 인터페이스(시리얼/TCP)를 보레이트 1200으로 재사용할 수 있다.
- [A-08] LGCP 에이전트의 아키텍처 패턴(캡처 루프, 링 버퍼, 디바이스 관리)을 따른다.
- [A-09] 디바이스 상태 속성명은 NASA/LGCP와 통일한다 (`power`, `current_temp`, `target_temp`, `mode`).
- [A-10] 제어 노드는 플레이스홀더로 구현하며, `control_enabled` 플래그로 비활성화 상태를 유지한다.

---

## 3. Requirements (요구사항) -- EARS 형식

### M1: LGCNP 프레임 파서

**[REQ-M1-01]** 시스템은 **항상** RS-485 버스에서 수신된 바이트 스트림을 TYPE-A(20바이트, STX=0x58)와 TYPE-B(40바이트, STX=0x81~0x85) 두 유형의 프레임으로 분류해야 한다.

**[REQ-M1-02]** **WHEN** 바이트 0x58이 수신되면 **THEN** 이후 19바이트를 추가 수신하여 20바이트 TYPE-A 프레임으로 조립해야 한다.

**[REQ-M1-03]** **WHEN** 바이트 0x81~0x85가 수신되면 **THEN** 이후 39바이트를 추가 수신하여 40바이트 TYPE-B 프레임으로 조립해야 한다.

**[REQ-M1-04]** **WHEN** TYPE-A 프레임에서 SEQ=01 또는 SEQ=05이면 **THEN** `XOR(pkt[0:19]) == pkt[19]` 체크섬을 검증해야 한다.

**[REQ-M1-05]** **WHEN** TYPE-A 프레임에서 SEQ=04이면 **THEN** `SUM(pkt[0:19]) & 0xFF == pkt[19]` 체크섬을 검증해야 한다.

**[REQ-M1-06]** **WHEN** TYPE-A 프레임에서 SEQ=02 또는 SEQ=03이면 **THEN** 체크섬 검증을 수행하지 않아야 한다 (b[18], b[19]는 센서 데이터).

**[REQ-M1-07]** **WHEN** TYPE-B 프레임이 수신되면 **THEN** 이중 기록 검증(`b[9]==b[29]`, `b[23]==b[36]`)을 수행해야 한다.

**[REQ-M1-08]** **WHEN** TYPE-B 프레임에서 이중 기록이 불일치하면 **THEN** 해당 프레임을 폐기하고 무효 카운터를 증가시켜야 한다.

**[REQ-M1-09]** **WHEN** TYPE-B 프레임이 수신되면 **THEN** 고정 바이트 구조를 검증해야 한다:
- `pkt[1]`이 0x02 또는 0x43이어야 한다 (CMD 유효 범위)
- `pkt[20]`이 IDU 번호(`pkt[0] - 0x81 + 1`)와 일치해야 한다 (IDU_INDEX)

**[REQ-M1-10]** **WHEN** TYPE-B 프레임에서 온도값이 추출되면 **THEN** 물리적 범위를 검증해야 한다:
- 설정온도: 16~30도C
- 실내온도: 0~50도C
- 흡입온도: 0~70도C
- 토출온도: 0~70도C

**[REQ-M1-11]** **IF** 온도값이 물리적 범위를 벗어나면 **THEN** 경고를 로그에 기록하되 프레임을 완전히 폐기하지는 않아야 한다.

**[REQ-M1-12]** **가능하면** 변화율 검증을 제공한다 -- 이전 사이클 대비 온도 변화가 2.0도C 이상이면 경고를 발생시킨다 (설정온도 제외).

### M2: LGCNP 에이전트

**[REQ-M2-01]** 시스템은 **항상** `agent.Agent` 인터페이스를 구현하는 `LGCNPAgent`를 제공해야 한다.

**[REQ-M2-02]** `LGCNPAgent`는 **항상** LGCP 에이전트와 동일한 라이프사이클(Init/Start/Stop/Pause/Resume)을 따라야 한다.

**[REQ-M2-03]** **WHEN** 에이전트가 시작되면 **THEN** `LGAPTransport`를 1200 bps 8N1로 열고 캡처 루프를 시작해야 한다.

**[REQ-M2-04]** **WHEN** 캡처 루프에서 유효한 TYPE-A 프레임이 수신되면 **THEN** ODU 상태 이벤트를 생성하여 링 버퍼에 저장해야 한다.

**[REQ-M2-05]** **WHEN** 캡처 루프에서 유효한 TYPE-B 프레임이 수신되면 **THEN** IDU 상태 이벤트를 생성하고 온도값을 변환하여 링 버퍼에 저장해야 한다.

**[REQ-M2-06]** 시스템은 **항상** 다음 온도 변환 공식을 적용해야 한다:
- 설정온도(도C) = `b[9] - 0x3C`
- 실내온도(도C) = `(b[23] - 0x40) / 2.0`
- 흡입온도(도C) = `(b[24] - 0x40) / 2.0`
- 토출온도(도C) = `(b[25] - 0x40) / 2.0`

**[REQ-M2-07]** **WHEN** TYPE-A SEQ=02 프레임이 수신되면 **THEN** 외기 온도를 추출해야 한다:
- 외기온도A(도C) = `(b[14] - 0x40) / 2.0`
- 외기온도B(도C) = `(b[15] - 0x40) / 2.0`

**[REQ-M2-08]** 시스템은 **항상** `agent.MessageReceiver`, `agent.StatefulAgent`, `agent.BufferInfoProvider`, `agent.TransportChecker` 인터페이스를 구현해야 한다.

**[REQ-M2-09]** 시스템은 **항상** Process 메서드에서 `get_stats`, `get_recent`, `drain` 커맨드를 지원해야 한다.

**[REQ-M2-10]** **WHEN** 트랜스포트 연결이 끊어지면 **THEN** 지수 백오프로 재연결을 시도해야 한다.

**[REQ-M2-11]** 시스템은 **항상** 캡처 통계를 원자적(atomic)으로 추적해야 한다: `framesCaptured`, `framesValid`, `framesInvalid`, `framesDropped`, `bytesReceived`.

### M3: 디바이스 관리

**[REQ-M3-01]** **WHEN** TYPE-B 패킷의 IDU_ADDR(0x81~0x85)에서 새로운 주소가 발견되면 **THEN** 해당 IDU를 자동으로 디바이스로 등록해야 한다.

**[REQ-M3-02]** **WHEN** TYPE-A 패킷이 수신되면 **THEN** ODU를 단일 디바이스로 등록해야 한다.

**[REQ-M3-03]** 시스템은 **항상** `device.DeviceProvider` 인터페이스를 구현하여 디바이스 목록을 노출해야 한다.

**[REQ-M3-04]** **WHEN** 유효한 TYPE-B 프레임이 수신되면 **THEN** 해당 IDU 디바이스의 상태를 갱신해야 한다:
- `target_temp`: 설정온도
- `current_temp`: 실내온도
- `inlet_temp`: 흡입온도
- `outlet_temp`: 토출온도
- `op_mode`: 운전 모드 (b[10])
- `status_flags`: 상태 플래그 (b[11])

**[REQ-M3-05]** **WHEN** 디바이스에서 `OfflineTimeout` 기간 동안 패킷이 수신되지 않으면 **THEN** 해당 디바이스를 오프라인으로 전환해야 한다.

**[REQ-M3-06]** **WHEN** 디바이스 상태가 변경되면 **THEN** 등록된 콜백(onDeviceStateChange)을 호출하여 UI에 알려야 한다.

### M4: 플로우 노드

**[REQ-M4-01]** 시스템은 **항상** `lgcnp-status` 노드 타입을 제공해야 한다 (SourceNode 인터페이스, 폴링 기반).

**[REQ-M4-02]** `lgcnp-status` 노드는 **항상** `agent_ref` 설정으로 LGCNP 에이전트를 참조해야 한다.

**[REQ-M4-03]** **WHEN** `lgcnp-status` 노드가 폴링하면 **THEN** 에이전트의 `get_recent` 또는 `drain` 커맨드로 새 프레임을 수신하여 개별 메시지로 출력해야 한다.

**[REQ-M4-04]** 시스템은 **항상** `lgcnp-control` 노드 타입을 제공해야 한다 (플레이스홀더).

**[REQ-M4-05]** `lgcnp-control` 노드는 시스템은 **항상** `control_enabled: false` 기본값으로 비활성화 상태를 유지해야 한다.

**[REQ-M4-06]** 시스템은 **항상** `lgcnp` 통합 노드 타입을 제공해야 한다 (상태 + 제어 통합).

**[REQ-M4-07]** **WHEN** `lgcnp` 통합 노드에 제어 키(power, mode, temperature, fan_speed)가 포함된 메시지가 입력되면 **THEN** "제어 미지원" 응답을 반환해야 한다.

### M5: Web UI 스키마

**[REQ-M5-01]** 시스템은 **항상** `agentSchemas.ts`에 `lgcnp` 에이전트 타입을 등록해야 한다 (보레이트 기본값 1200).

**[REQ-M5-02]** 시스템은 **항상** `nodeSchemas.ts`에 `lgcnp-status`, `lgcnp-control`, `lgcnp` 노드 스키마를 등록해야 한다.

**[REQ-M5-03]** 시스템은 **항상** 에이전트 스키마에서 `agent_select` 옵션에 `lgcnp`를 포함해야 한다.

### M6: 타입 등록

**[REQ-M6-01]** 시스템은 **항상** `agent.DefaultManager`에 `"lgcnp"` 에이전트 타입을 등록해야 한다.

**[REQ-M6-02]** 시스템은 **항상** 노드 레지스트리에 `"lgcnp-status"`, `"lgcnp-control"`, `"lgcnp"` 노드 타입을 등록해야 한다.

---

## 4. Specifications (명세)

### 4.1 패킷 구조

#### TYPE-A (ODU, 20바이트)

```
[STX=0x58][SEQ 01~05][DATA 17B][CHK 또는 DATA]
  byte 0      1        2..18          19
```

- SEQ=01: ODU 정적 상태, XOR 체크섬
- SEQ=02: ODU 실시간 센서 (외기온도A/B), 체크섬 없음
- SEQ=03: ODU 부가 상태, 체크섬 없음
- SEQ=04: ODU 파라미터, SUM 체크섬
- SEQ=05: ODU 상태2, XOR 체크섬

#### TYPE-B (IDU, 40바이트)

```
[IDU_ADDR][CMD][SUB_CMD][DEV_TYPE][...][SET_TEMP]...[ROOM_TEMP][INLET][OUTLET]...
  0x81~85   1     2        3       4~8     9         23        24     25
```

- 이중 기록: b[9]==b[29] (설정온도), b[23]==b[36] (실내온도)
- CMD 사이클: 0x02/0x43 교대 (5~6초 주기)
- b[38], b[39]: 센서 파생값 (체크섬 아님)

### 4.2 6계층 신뢰성 모델

| 계층 | 검증 내용 | 적용 대상 |
|------|----------|----------|
| 1. UART Framing | 하드웨어 자동 (Stop bit) | 모든 패킷 |
| 2. 이중 기록 | b[9]=b[29], b[23]=b[36] | TYPE-B |
| 3. 고정 바이트 | CMD 범위, IDU_INDEX 일치 | TYPE-B; XOR/SUM CHK: TYPE-A SEQ=01,04,05 |
| 4. 물리 범위 | 온도 유효 범위 | TYPE-A SEQ=02, TYPE-B |
| 5. 변화율 | 사이클 간 2도C 이내 | TYPE-A SEQ=02, TYPE-B |
| 6. SEQ 순서 | 01->02->03->04->05 연속 | TYPE-A |

### 4.3 파일 구조 (신규)

```
internal/agent/lg/
  lgcnp_agent.go       -- LGCNPAgent 구조체, 라이프사이클, 캡처 루프
  lgcnp_frame.go       -- LGCNP 프레임 파서 (TYPE-A/TYPE-B)
  lgcnp_config.go      -- LGCNPConfig 파싱
  lgcnp_device.go      -- LGCNPDevice 모델, 상태 관리
  lgcnp_register.go    -- 에이전트 타입 등록

internal/node/
  lgcnp.go             -- lgcnp-status, lgcnp-control, lgcnp 노드

web/src/config/
  agentSchemas.ts      -- lgcnp 에이전트 UI 스키마 추가
  nodeSchemas.ts       -- lgcnp 노드 UI 스키마 추가
```

### 4.4 이벤트 JSON 구조

#### TYPE-A 이벤트

```json
{
  "type": "lgcnp_odu_frame",
  "timestamp": "2026-04-12T10:00:00.000Z",
  "seq": 123,
  "raw_hex": "580200...",
  "odu_seq": 2,
  "checksum_valid": true,
  "parsed": {
    "outdoor_temp_a": 21.5,
    "outdoor_temp_b": 14.0,
    "flag_a": 129,
    "ctr_a": 77
  }
}
```

#### TYPE-B 이벤트

```json
{
  "type": "lgcnp_idu_frame",
  "timestamp": "2026-04-12T10:00:00.000Z",
  "seq": 124,
  "raw_hex": "810200...",
  "idu_addr": 129,
  "idu_num": 1,
  "cmd_cycle": "A",
  "redundancy_valid": true,
  "parsed": {
    "set_temp": 22.0,
    "room_temp": 22.5,
    "inlet_temp": 26.5,
    "outlet_temp": 27.5,
    "op_mode": 20,
    "status_flags": 15,
    "dev_type": 124,
    "device_id": 93
  }
}
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 프로토콜 분석 섹션 | 구현 파일 |
|------------|-----------------|----------|
| REQ-M1-01~03 | 섹션 4 (패킷 유형 개요) | lgcnp_frame.go |
| REQ-M1-04~06 | 섹션 3 (체크섬 정책) | lgcnp_frame.go |
| REQ-M1-07~08 | 섹션 6.2 (이중 기록) | lgcnp_frame.go |
| REQ-M1-09 | 섹션 7.2 계층 3 | lgcnp_frame.go |
| REQ-M1-10~11 | 섹션 7.2 계층 4 | lgcnp_frame.go |
| REQ-M1-12 | 섹션 7.2 계층 5 | lgcnp_agent.go |
| REQ-M2-01~11 | 전체 | lgcnp_agent.go |
| REQ-M2-06 | 섹션 6.5 (온도 변환) | lgcnp_agent.go |
| REQ-M2-07 | 섹션 5.3 (SEQ=02) | lgcnp_agent.go |
| REQ-M3-01~06 | 섹션 6.3 (IDU 주소) | lgcnp_device.go |
| REQ-M4-01~07 | -- | lgcnp.go (node) |
| REQ-M5-01~03 | -- | agentSchemas.ts, nodeSchemas.ts |
| REQ-M6-01~02 | -- | lgcnp_register.go, registry.go |
