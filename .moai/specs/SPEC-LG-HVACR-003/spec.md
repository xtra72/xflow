---
id: SPEC-LG-HVACR-003
version: "1.0.0"
status: completed
created: "2026-09-14"
updated: "2026-09-14"
author: xtra
priority: high
tags: lg_hvacr03, pmbusb00a, modbus-rtu, modbus-tcp, gateway, control, lg, indoor-unit, hvac
prerequisite: SPEC-LG-HVACR-002-001, SPEC-MODBUS-006, SPEC-DEVICE-IDENTITY-001
---

# SPEC-LG-HVACR-003: LG HVACR-03 (PMBUSB00A Modbus 게이트웨이) 제어 에이전트

> **명명 규약**: 게이트웨이 하드웨어 `PMBUSB00A` / 에이전트 타입 ID `lg_hvacr03` (LG HVACR-03). 기존 `lg_hvacr01`(LG ICP-01) · `lg_hvacr02`(LG ICP-02, 역공학 패시브 캡처) 계열의 세 번째 항목이며, **LG 공식 Modbus 경로**라는 점에서 앞의 둘과 성격이 다르다.

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-09-14 | xtra | 최초 작성 |
| 1.0.2 | 2026-09-14 | xtra | 플로우 노드를 범위에 포함. 최초의 "노드 재사용 불가" 판단을 정정 — 타입 체크 한 곳만 넓히면 기존 노드 3종을 공유할 수 있음이 확인되어, 1,351줄 복제 대신 구현 공유 + 타입 이름 분리로 구현했다. 부수적으로 `unit_id` 어드레싱 필터의 hex 전용 파싱을 10진/16진 공용으로 보정했다 (기존 `lg_hvacr02` 의 8자리 주소 필터도 이 보정으로 비로소 동작한다). § 4.9 추가. |
| 1.0.1 | 2026-09-14 | xtra | M1~M10 구현 완료. 문서 §5 완성 프레임 13종 골든 테스트 통과, 프로토콜 층 커버리지 100%, 신규 파일 전체 88.5%. `set_multiple` / `set_temp_limit` 의 FC16 단일 트랜잭션, 잠금 진단, 기기 종류 게이트 검증 완료. ERV·하이드로킷은 장비 미확보로 미실측. 플로우 노드는 범위 제외(후속 SPEC). |

---

## 1. 개요

LG PMBUSB00A는 LG 시스템 에어컨의 실외기 중앙제어 버스를 표준 Modbus RTU로 변환하는 공식 게이트웨이 모듈이다. 본 SPEC은 이 게이트웨이를 Modbus **마스터**로서 폴링·제어하는 xflow 에이전트 `lg_hvacr03` 을 정의한다.

기준 문서: [references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md](references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md)

### 1.1 목적

- LG 시스템 에어컨(MULTI V 5 / MULTI V S / ERV / THERMA V)의 **상태 수집과 제어**를 공식 Modbus 경로로 제공한다.
- 디바이스 관리 · 메시지 출력 형식 · 명령 인터페이스를 **`lg_hvacr02` 와 동일하게** 맞춰, 기존 플로우·대시보드·제어 패널·디바이스 목록이 추가 학습 없이 동작하게 한다.
- 역공학 계열(`lg_hvacr02`)이 제공하지 못하는 **정식 쓰기 제어**(FC05/06/16)와 **에러 코드·알람·필터·잠금 상태**를 노출한다.

### 1.2 범위

**포함**

- `lg_hvacr03` 에이전트: Modbus 마스터 폴링 엔진, 내부 디바이스 관리, 상태 투영, 제어 명령
- 트랜스포트 2종: RTU(시리얼) / TCP-client (시리얼-이더넷 컨버터 뒤의 게이트웨이)
- 레지스터 맵 4영역 전체: Coil ①~⑩, Discrete ①~⑤, Holding ①~⑥, Input ①~⑥
- 제어 명령: 문서에 정의된 **모든 쓰기 가능 레지스터**
- `device.DeviceProvider` + `adapter.CommandExecutor` 배선, 백엔드·프론트엔드 타입 등록

**제외 (후속 SPEC)**

- ~~플로우 노드~~ — **범위에 포함됨 (v1.0.2 정정)**. 최초 판단은 "기존 `internal/node/lg_hvacr02.go` 가 `*lg.Hvacr02Agent` 로 타입 체크하므로 재사용 불가"였으나, 실제로는 타입 체크가 단 한 곳([lg_hvacr02.go:228](internal/node/lg_hvacr02.go#L228))이고 나머지 1,351줄은 `agent.Agent` 인터페이스로 `Process()` 만 호출한다. 주소도 파싱하지 않고 문자열 그대로 전달한다. 명령 집합과 `device_state` 형식을 `lg_hvacr02` 와 동일하게 맞춘 설계 덕분에 **노드 구현을 그대로 공유**할 수 있다. § 4.9 참조.
- 전력량·소비전력 계측 (문서 §9 — PMBUSB00A 미제공)
- 압축기 주파수·제상 상태 등 저수준 진단 (문서 §9 — LGCP 스니핑 경로 유지)

### 1.3 `lg_hvacr02` 와의 관계

| 축 | lg_hvacr02 (LG ICP-02) | lg_hvacr03 (PMBUSB00A) | 본 SPEC 방침 |
|----|----------------------|----------------------|------------|
| 수집 방식 | 패시브 캡처 (송신 금지) | **능동 폴링 마스터** | 신규 구현 |
| 디바이스 발견 | 버스 프레임 관측 (`ensureDevice`) | **FC02 전체 스캔** (연결 비트맵) | 신규 구현 |
| 디바이스 관리 | `devices` map + offline watch + report gate | 동일 | **동일 복제** |
| 메시지 출력 | `device_state` payload | 동일 | **동일 복제** |
| 상태 투영 | `toProperties()`, hvac 통일 ID | 동일 + 에러/알람/필터/잠금 확장 | **확장 복제** |
| 제어 방식 | 서모스탯 사칭 (SEQ 추적·에코 판별·버스 정숙 대기) | **정식 레지스터 쓰기** | 신규 구현 (대폭 단순) |
| 명령 인터페이스 | `Process()` 14종 | 동일 14종 + 확장 | **동일 복제 + 확장** |
| CRC | CRC-16/XMODEM | **CRC-16/MODBUS** | 패키지 분리로 격리 |

> **CRC 혼용 방지**: 문서 §2 가 경고하는 XMODEM/MODBUS 혼용 위험은 코드 경계로 이미 해소되어 있다. LGCP 계열은 `internal/agent/lg/checksum.go` · `lg_icp02_crc.go`, Modbus 계열은 `internal/modbus/rtu.go` 의 `CRC16` 을 쓴다. 본 에이전트는 **후자만** 참조하며, 전자를 import 하지 않는다.

---

## 2. 환경 (Environment)

### 기술 스택

- 언어: Go 1.23+
- 에이전트 프레임워크: `internal/agent/lg/` (동일 패키지에 신규 파일 추가)
- 생명주기: `pkg/lifecycle/` (BaseLifecycle)
- 시리얼: `go.bug.st/serial` v1.6.4 (기존 의존성, 추가 의존성 없음)

### 재사용 자산

| 자산 | 위치 | 용도 |
|------|------|------|
| `NewModbusRTUTransport` | [internal/agent/modbus/transport_rtu.go:69](internal/agent/modbus/transport_rtu.go#L69) | 반이중 동기 시리얼 마스터 (turnaround mutex, T3.5 silence, `io.ReadWriteCloser` seam) |
| `NewModbusTCPTransport` | [internal/agent/modbus/transport.go:59](internal/agent/modbus/transport.go#L59) | TCP 트랜스포트 |
| `CRC16` / `BuildRTUADU` / `ParseRTUADU` | [internal/modbus/rtu.go](internal/modbus/rtu.go) | CRC-16/MODBUS (0xA001, init 0xFFFF), ADU 프레이밍 |
| `hvac.ModeFromName` / `FanSpeedFromName` | [internal/agent/hvac/codes.go](internal/agent/hvac/codes.go) | mode / fan_speed 통일 ID 변환 |
| `agent.ResolveDeviceID` | `internal/agent/` | 디바이스 UUID 해석 |
| `agent.DeviceEntry` | `internal/agent/` | 설정 기반 디바이스 등록 |

### 신규 의존 방향

```
internal/agent/lg  ──(신규)──▶  internal/agent/modbus   (트랜스포트)
                   ──(신규)──▶  internal/modbus         (CRC/ADU)
                   ──(기존)──▶  internal/agent/hvac     (통일 ID)
```

> 역방향 의존은 생기지 않으므로 import cycle 위험이 없다.

---

## 3. 가정 및 제약 (Assumptions)

### 3.1 실측 전 미검증 가정 — 설정으로 노출

문서 §8 이 스스로 실측 필요를 명시한 항목 중 **상위 로직의 전제가 되는 세 가지**는 설정값으로 열어 두고, 문서값을 기본값으로 삼는다. 현장 실측이 문서와 다르면 재빌드 없이 YAML 한 줄로 교정한다.

| 설정 키 | 기본값 | 근거 | 실측 항목 |
|--------|-------|------|----------|
| `temp_scale` | `10` | 문서 §4.4 ③ — 16.0~30.0 °C 를 ×10 (160~300) | §8 #1 (★★★) |
| `address_base` | `0` | 문서 §3.2 — N = 0~15 | §8 #2 (★★★) |
| `fan_auto_code` | `4` | 문서 §4.4 ② — 1=약, 2=중, 3=강, 4=자동 | §8 #3 (★★) |

> **`fan_auto_code` 가 필요한 이유**: 문서 §7.1-2 가 지적하듯 LGCP 는 5단계(4=초강, 5=자동)이고 Modbus 는 4값(4=자동)이다. 값 4가 현장에서 "초강"으로 동작하면 `fan_auto_code: 5` 로 교정하고, 4를 `turbo` 로 매핑한다.

> **기본값의 근거 — 문서 내부 정합성 확인 (2026-09-14)**: SPEC 작성 시점에 문서 §5 의 완성 프레임 13종을 CRC-16/MODBUS 로 독립 재계산하여 13/13 일치를 확인했다. 그 과정에서 `IDU 3 설정온도 26.0` 프레임(`01 06 00 3E 01 04 E8 55`)의 주소 `0x003E`(62)가 `3×20+2` 로 **0-base** 와, 값 `0x0104`(260)이 `26.0×10` 으로 **×10 스케일**과 각각 일치함이 확인되었다. 문서가 스스로와 일관되다는 것이 하드웨어와 일치한다는 뜻은 아니므로 §8 #1·#2 의 실측 필요는 그대로 남지만, `temp_scale: 10` / `address_base: 0` 을 기본값으로 삼는 근거는 된다.

### 3.2 프로토콜 제약 — 구현이 반드시 지켜야 할 것

| # | 제약 | 출처 | 구현 대응 |
|:-:|------|------|----------|
| 1 | 레지스터 "번호"(40001 등)는 1-base, **프레임 주소는 0-base** | §4.1 | 주소 계산은 0-base 만 사용, 상수에 번호 표기 병기 금지 |
| 2 | 온도는 **signed int16** (0xFFC4 = −6.0 °C) | §4.5 | `int16` 캐스팅 필수, `uint16` 해석 금지 |
| 3 | 쓰기 후 즉시 반영되지 않음 (수 초 소요) | §6.2-1 | read-back 을 `control_verify_delay`(기본 3s) 이후 수행 |
| 4 | 잠금 코일(④~⑧) 설정 시 해당 항목 쓰기가 **조용히 무시됨** | §6.2-2 | 잠금 상태를 상태에 노출하고, 제어 실패 시 진단 힌트로 사용 |
| 5 | 미설치 N 에 대한 쓰기는 **예외 없이 무시될 수 있음** | §6.2-4 | 쓰기 전 Discrete ①(연결 상태) 확인 |
| 6 | 모드 전환 시 온도·풍량이 함께 바뀜 (모드별 마지막 값 기억) | §6.2-5 | `set_multiple` 은 **FC16 단일 트랜잭션** 필수 |
| 7 | 설정 온도 상·하한(40004/40005) 초과 시 클램핑 또는 거부 | §6.2-3 | 쓰기 전 범위 검증, 초과 시 에러 반환 |
| 8 | 5초보다 빠른 폴링은 게이트웨이가 응답을 거르기 시작 | §6.1 | `poll_interval` 최소 5s 강제 (하한 검증) |

---

## 4. 요구사항 (EARS)

### 4.1 트랜스포트 및 연결

**REQ-LG-HVACR-003-TRANSPORT-001** (Ubiquitous)
시스템은 `transport_type` 설정으로 `rtu`(시리얼) 또는 `tcp-client` 트랜스포트를 선택할 수 있어야 한다. 기본값은 `rtu` 이다.

**REQ-LG-HVACR-003-TRANSPORT-002** (Ubiquitous)
시스템은 RTU 트랜스포트에서 9600-8N1 을 기본 시리얼 파라미터로 사용해야 한다 (문서 §2).

**REQ-LG-HVACR-003-TRANSPORT-003** (Ubiquitous)
시스템은 Modbus ADU 프레이밍과 CRC 계산에 `internal/modbus` 패키지의 `BuildRTUADU` / `ParseRTUADU` / `CRC16` 만을 사용해야 하며, LGCP 계열 체크섬 함수를 참조해서는 안 된다.

**REQ-LG-HVACR-003-TRANSPORT-004** (Event-driven)
**When** 트랜스포트 연결이 끊어지면, **the** 시스템 **shall** `reconnect_interval` 간격으로 재연결을 시도하고 `max_reconnect_backoff` 까지 지수 백오프해야 한다.

**REQ-LG-HVACR-003-TRANSPORT-005** (Event-driven)
**When** 트랜스포트가 끊어진 상태가 지속되면, **the** 시스템 **shall** 모든 디바이스를 오프라인으로 표시해야 한다.

**REQ-LG-HVACR-003-TRANSPORT-006** (Ubiquitous)
시스템은 `slave_id` 설정(1~16, 기본 1)으로 게이트웨이의 Modbus 슬레이브 주소를 지정할 수 있어야 한다 (문서 §3.1).

### 4.2 디바이스 발견 및 관리

**REQ-LG-HVACR-003-DEVICE-001** (Event-driven)
**When** 에이전트가 시작되면, **the** 시스템 **shall** FC02 로 Discrete 0~255 를 1 트랜잭션 읽어 설치된 실내기 N 을 판별해야 한다 (문서 §4.3, §5).

**REQ-LG-HVACR-003-DEVICE-002** (Ubiquitous)
시스템은 `scan_interval`(기본 30s) 마다 FC02 전체 스캔을 반복하여 디바이스 연결 상태를 갱신해야 한다 (문서 §6.1).

**REQ-LG-HVACR-003-DEVICE-003** (State-driven)
**While** `auto_discovery` 가 활성인 동안, **the** 시스템 **shall** 스캔에서 새로 발견된 N 을 `source: "auto"` 디바이스로 등록해야 한다.

**REQ-LG-HVACR-003-DEVICE-004** (Ubiquitous)
시스템은 설정의 `devices` 항목을 `source: "config"` 디바이스로 등록해야 하며, 이들은 스캔에서 미발견되어도 목록에서 제거되지 않아야 한다.

**REQ-LG-HVACR-003-DEVICE-005** (Ubiquitous)
시스템은 디바이스별 `report_enabled` 플래그를 유지해야 하며, `false` 인 디바이스의 `device_state` 방출을 억제해야 한다.

**REQ-LG-HVACR-003-DEVICE-006** (Event-driven)
**When** 디바이스가 `offline_timeout`(기본 30s) 동안 연결 비트에서 0 으로 관측되면, **the** 시스템 **shall** 해당 디바이스를 오프라인으로 표시해야 한다.

**REQ-LG-HVACR-003-DEVICE-007** (Ubiquitous)
시스템은 디바이스 식별에 `agent.ResolveDeviceID` 로 해석한 UUID(`device_id`)와 프로토콜 주소(`unit_id`)를 분리하여 유지해야 한다.

> **주소 표기**: `lg_hvacr02` 의 `unit_id` 는 8자리 hex 문자열이었으나, PMBUSB00A 의 N 은 0~15 정수이다. `unit_id` 는 **10진 문자열**(`"0"`~`"15"`)로 표기한다. 문서 §3.2 가 명시하듯 LGCP 물리 주소와 Modbus N 은 **별개 값**이므로 hex 표기를 흉내 내면 오히려 혼동을 부른다.

### 4.3 폴링 엔진

**REQ-LG-HVACR-003-POLL-001** (Ubiquitous)
시스템은 4개 레지스터 영역을 각각 독립 주기로 폴링해야 한다.

| 영역 | FC | 주소 | 개수 | 설정 키 | 기본 주기 |
|------|:--:|------|:----:|--------|:---------:|
| 연결/알람 비트맵 | 02 | 0~255 | 256 bit | `scan_interval` | 30s |
| 코일 상태 | 01 | N×16 | 10 bit | `poll_interval` | 10s |
| Holding (모드/풍량/온도) | 03 | N×20 | 6 word | `poll_interval` | 10s |
| Input (에러/센서) | 04 | N×20 | 6 word | `poll_interval` | 10s |

**REQ-LG-HVACR-003-POLL-002** (Ubiquitous)
시스템은 `poll_interval` 이 5초 미만으로 설정되면 설정 오류를 반환해야 한다 (문서 §6.1 — 그보다 빠르면 게이트웨이가 응답을 거른다).

**REQ-LG-HVACR-003-POLL-003** (Ubiquitous)
시스템은 Modbus 트랜잭션을 직렬화하여, 한 시점에 하나의 요청/응답만 버스에 흐르게 해야 한다.

**REQ-LG-HVACR-003-POLL-004** (Event-driven)
**When** 특정 N 에 대한 폴링이 `request_timeout`(기본 1s) 내에 응답하지 않으면, **the** 시스템 **shall** 해당 N 을 건너뛰고 다음 N 으로 진행해야 하며, 전체 폴링 사이클을 중단해서는 안 된다.

**REQ-LG-HVACR-003-POLL-005** (State-driven)
**While** 디바이스가 오프라인인 동안, **the** 시스템 **shall** 해당 N 의 코일/Holding/Input 폴링을 건너뛰어 버스 대역을 절약해야 한다.

### 4.4 상태 디코딩 및 투영

**REQ-LG-HVACR-003-STATE-001** (Ubiquitous)
시스템은 Input Register 의 온도 값을 **signed int16** 으로 해석하고 `temp_scale` 로 나누어 °C 로 변환해야 한다 (문서 §4.5).

**REQ-LG-HVACR-003-STATE-002** (Ubiquitous)
시스템은 Holding ① 운전 모드 코드(0=냉방, 1=제습, 2=송풍, 3=자동, 4=난방)를 `hvac.ModeFromName` 통일 ID 로 변환하여 출력해야 한다.

**REQ-LG-HVACR-003-STATE-003** (Ubiquitous)
시스템은 Holding ② 풍량 코드를 `fan_auto_code` 설정을 반영하여 `hvac.FanSpeedFromName` 통일 ID 로 변환해야 한다.

**REQ-LG-HVACR-003-STATE-004** (Ubiquitous)
시스템은 상태 투영에서 `lg_hvacr02` 와 **동일한 키 이름**을 사용해야 한다: `power`, `current_temperature`, `target_temperature`, `fan_speed`, `mode`.

**REQ-LG-HVACR-003-STATE-005** (State-driven)
**While** 디바이스 전원이 OFF 인 동안, **the** 시스템 **shall** 운전 관련 속성(`target_temperature` 등)을 투영에서 생략하고 `fan_speed` / `mode` 는 통일 ID 0 으로 출력해야 한다 (`lg_hvacr02` 의 `indoorProperties()` 와 동일 규약).

**REQ-LG-HVACR-003-STATE-006** (Ubiquitous)
시스템은 PMBUSB00A 고유 정보를 추가 속성으로 노출해야 한다.

| 속성 키 | 출처 | 타입 | 비고 |
|--------|------|------|------|
| `error_code` | Input ① | int | 0 = 정상 |
| `alarm` | Discrete ② | bool | |
| `filter_alarm` | Discrete ③ | bool | |
| `pipe_in_temperature_c` | Input ③ | float | `temp_scale` 적용 |
| `pipe_out_temperature_c` | Input ④ | float | `temp_scale` 적용 |
| `swing` | Coil ② | bool | 에어컨 |
| `lock_remote` / `lock_mode` / `lock_fan` / `lock_temp` / `lock_address` | Coil ④~⑧ | bool | 제어 무시 진단용 |
| `temp_limit_high_c` / `temp_limit_low_c` | Holding ④/⑤ | float | `temp_scale` 적용 |
| `water_tank_temperature_c` | Input ⑤ | float | 하이드로킷 |
| `solar_temperature_c` | Input ⑥ | float | AWHP |
| `erv_mode` | Holding ⑥ | int | ERV — 0=열교환, 1=자동, 2=보통 |
| `erv_rapid` / `erv_eco` | Coil ⑨/⑩ | bool | ERV |

> 값이 유효하지 않거나 해당 기기 종류가 아닌 속성은 투영에 **포함하지 않는다** (JSON 직렬화에서 자연 제외). 문서 §5.1 의 "미사용(에어컨)" 워드가 0 으로 오는 경우처럼, 0 을 유효값으로 오해하지 않도록 기기 종류별 게이트를 둔다.

### 4.5 메시지 출력 형식

**REQ-LG-HVACR-003-EMIT-001** (Ubiquitous)
시스템은 `lg_hvacr02` 와 **바이트 구조가 동일한** `device_state` 페이로드를 방출해야 한다.

```json
{
  "unit_id": "3",
  "device_id": "<UUID v4>",
  "trigger": "report | change | response",
  "state": { "power": true, "current_temperature": 24.5, "target_temperature": 26.0,
             "fan_speed": 5, "mode": 1, "error_code": 0, "alarm": false },
  "metadata": { "name": "indoor-3", "address": "3", "device_type": "HVACR.IDU" },
  "last_seen_ms": 1757808000000
}
```

**REQ-LG-HVACR-003-EMIT-002** (Ubiquitous)
시스템은 방출 페이로드를 최근 프레임 링 버퍼(64개)와 `msgCh`(bridge 활성 시) **양쪽**에 push 해야 한다.

**REQ-LG-HVACR-003-EMIT-003** (Event-driven)
**When** `trigger` 가 `"report"` 가 아니면, **the** 시스템 **shall** 직전 방출 투영과 동일한 경우 방출을 생략해야 한다 (dedup).

**REQ-LG-HVACR-003-EMIT-004** (Ubiquitous)
시스템은 `report_interval`(기본 60s) 주기로 모든 디바이스 상태를 `trigger: "report"` 로 방출해야 하며, 이 경우 dedup 을 적용해서는 안 된다.

**REQ-LG-HVACR-003-EMIT-005** (Ubiquitous)
시스템은 `last_seen_ms` 를 epoch milliseconds (int64) 로 출력해야 한다. `trigger: "report"` 는 발행 시점, 그 외는 관측 시점을 사용한다.

**REQ-LG-HVACR-003-EMIT-006** (Event-driven)
**When** 실내 온도만 변경되고 변화폭이 `event_temp_threshold`(기본 1.0 °C) 미만이면, **the** 시스템 **shall** `change` 방출을 억제해야 한다.

### 4.6 명령 인터페이스

**REQ-LG-HVACR-003-CMD-001** (Ubiquitous)
시스템은 `Process()` 에서 `lg_hvacr02` 와 **동일한 조회·관리 명령 9종**을 지원해야 한다: `get_stats`, `get_recent`, `drain`, `get_all`, `request_state`, `get_state`, `list_devices`, `remove_device`, `set_device`.

**REQ-LG-HVACR-003-CMD-002** (Ubiquitous)
시스템은 `get_recent` 에서 `count > 0` 은 비파괴 조회, `count == 0` 은 drain(파괴적)으로 해석해야 한다 (HVAC 노드 공통 규약).

**REQ-LG-HVACR-003-CMD-003** (Ubiquitous)
시스템은 요청 DTO 로 `lg_hvacr02` 와 동일한 필드를 받아야 한다: `command`, `count`, `address`, `device_id`, `params`, `node_id`, `flow_id`, `last_seq`.

### 4.7 제어

**REQ-LG-HVACR-003-CTRL-001** (State-driven)
**While** `control_enabled` 가 `false` 인 동안, **the** 시스템 **shall** 모든 제어 명령에 대해 `ErrHvacr03ControlNotEnabled` 를 반환해야 한다.

**REQ-LG-HVACR-003-CTRL-002** (Ubiquitous)
시스템은 `lg_hvacr02` 와 동일한 기본 제어 명령 5종을 지원해야 한다.

| 명령 | Modbus 매핑 | 파라미터 |
|------|------------|---------|
| `set_power` | FC05 Coil `N×16+0` (ON=0xFF00 / OFF=0x0000) | `power: bool` |
| `set_mode` | FC06 Holding `N×20+0` | `mode: string` |
| `set_fan_speed` | FC06 Holding `N×20+1` | `fan_speed: string` |
| `target_temperature` | FC06 Holding `N×20+2` | `temperature: float` |
| `set_multiple` | **FC16 Holding `N×20+0..2` 단일 트랜잭션** | 위 3종 조합 |

**REQ-LG-HVACR-003-CTRL-003** (Ubiquitous)
시스템은 문서에 정의된 나머지 쓰기 가능 레지스터를 제어 명령으로 노출해야 한다.

| 명령 | Modbus 매핑 | 적용 기기 |
|------|------------|----------|
| `set_swing` | FC05 Coil `N×16+1` | 에어컨 |
| `clear_filter_alarm` | FC05 Coil `N×16+2` | 공통 |
| `set_lock` | FC05 Coil `N×16+3..7` (`target: remote\|mode\|fan\|temp\|address`) | 공통 |
| `set_temp_limit` | FC16 Holding `N×20+3..4` (상·하한 동시) | 공통 |
| `set_erv_mode` | FC06 Holding `N×20+5` | ERV |
| `set_erv_rapid` | FC05 Coil `N×16+8` | ERV |
| `set_erv_eco` | FC05 Coil `N×16+9` | ERV |

**REQ-LG-HVACR-003-CTRL-004** (Ubiquitous)
시스템은 `set_multiple` 을 **반드시 FC16 단일 트랜잭션**으로 전송해야 하며, 개별 FC06 쓰기로 분할해서는 안 된다 (문서 §6.2-5 — 모드 전환 시 온도·풍량이 함께 바뀌므로 분할 쓰기는 중간 상태를 남긴다).

**REQ-LG-HVACR-003-CTRL-005** (Event-driven)
**When** 제어 명령이 수신되면, **the** 시스템 **shall** 대상 N 의 연결 상태(Discrete ①)를 먼저 확인하고, 미연결이면 `ErrHvacr03DeviceNotConnected` 를 반환해야 한다 (문서 §6.2-4 — 미설치 N 쓰기는 조용히 무시된다).

**REQ-LG-HVACR-003-CTRL-006** (Ubiquitous)
시스템은 설정 온도 쓰기 시 `[16.0, 30.0]` 범위 및 현재 상·하한 제한(Holding ④/⑤)을 검증하고, 벗어나면 `ErrHvacr03TemperatureOutOfRange` 를 반환해야 한다 (문서 §6.2-3).

**REQ-LG-HVACR-003-CTRL-007** (Event-driven)
**When** 제어 쓰기가 성공하면, **the** 시스템 **shall** `control_verify_delay`(기본 3s) 이후 read-back 하여 반영 여부를 확인하고 결과를 응답에 포함해야 한다 (문서 §6.2-1 — 쓰기 직후 read-back 은 이전 값을 돌려줄 수 있다).

**REQ-LG-HVACR-003-CTRL-008** (Event-driven)
**When** read-back 결과가 기대값과 다르고 해당 항목의 잠금 코일(Coil ④~⑧)이 설정되어 있으면, **the** 시스템 **shall** 응답에 잠금이 원인임을 나타내는 진단 정보를 포함해야 한다 (문서 §6.2-2).

**REQ-LG-HVACR-003-CTRL-009** (Ubiquitous)
시스템은 제어 쓰기와 폴링 읽기가 동일 버스를 공유하므로, 쓰기를 폴링과 동일한 직렬화 경로로 처리해야 한다.

### 4.8 통합 및 등록

**REQ-LG-HVACR-003-REG-001** (Ubiquitous)
시스템은 `lg.RegisterLGHvacr03Types(mgr)` 로 타입 ID `lg_hvacr03` 을 등록해야 하며, 이 호출은 `cmd/xflowd/main.go` 의 등록 블록에 배선되어야 한다.

**REQ-LG-HVACR-003-REG-002** (Ubiquitous)
시스템은 `device.DeviceProvider` 를 구현하여 관리 디바이스를 통합 디바이스 인터페이스로 노출해야 한다.

**REQ-LG-HVACR-003-REG-003** (State-driven)
**While** `control_enabled` 가 활성인 동안, **the** 시스템 **shall** 실내기 디바이스를 `device.ControllableDevice` 로 노출해야 한다.

**REQ-LG-HVACR-003-REG-004** (Ubiquitous)
시스템은 프론트엔드의 에이전트 타입 등록 지점 전체에 `lg_hvacr03` 을 추가해야 한다: `agentSchemas.ts`(3곳), `agentTypeMeta.ts`, `twoColumnConfigMap.ts`, `AgentTypesPage.tsx`, `DeviceListPage.tsx`(제거 가능·리포트 토글 2곳).

### 4.9 플로우 노드 (v1.0.2 추가)

**REQ-LG-HVACR-003-NODE-001** (Ubiquitous)
시스템은 기존 `lg_hvacr02` 노드 구현(`internal/node/lg_hvacr02.go`)을 `lg_hvacr03` 에이전트에도 적용해야 하며, 노드 구현을 복제해서는 안 된다.

> **근거**: 노드는 `agent.Agent` 인터페이스로 `Process()` 만 호출하고 주소를 파싱하지 않는다. 본 SPEC 이 명령 집합(조회 9종)과 `device_state` 페이로드 형식을 `lg_hvacr02` 와 동일하게 요구하므로(REQ-CMD-001, REQ-EMIT-001), 노드 입장에서 두 에이전트는 구별되지 않는다. 1,351줄을 복제하면 유지보수가 이중화된다.

**REQ-LG-HVACR-003-NODE-002** (Ubiquitous)
시스템은 노드 타입 `lg-hvacr03-status` / `lg-hvacr03-control` / `lg-hvacr03` 을 등록해야 하며, `_` 별칭도 함께 제공해야 한다.

> 구현은 공유하되 타입 이름은 분리한다. 플로우에서 `lg_hvacr02_status` 노드가 `lg_hvacr03` 에이전트를 참조하면 계열이 어긋나 보여 읽기 어렵다.

**REQ-LG-HVACR-003-NODE-003** (Ubiquitous)
시스템은 status 노드의 `unit_id` 어드레싱 필터에서 10진 주소를 올바르게 매칭해야 한다.

> 기존 `hexByteEqual` 은 `parseHexByte`(1~2자리, 8-bit) 기반이라 `"10"` 을 `0x10`(16) 으로 읽어 N=10~15 가 조용히 빗나간다. 정규화 문자열 일치를 먼저 시도하고 실패 시 기존 hex 동치 비교로 폴백하여, 10진(`lg_hvacr03`)과 8자리 hex(`lg_hvacr02`)를 모두 지원한다.

**REQ-LG-HVACR-003-NODE-004** (Ubiquitous)
시스템은 `lg_hvacr02` 의 기존 `unit_id` 필터 동작(`"0x58"` 과 `"58"` 의 hex byte 동치)을 보존해야 한다.

> 정규화 일치를 **추가**하는 방식이므로 기존에 `true` 이던 비교가 `false` 로 바뀌지 않는다. 부수적으로 기존에 파싱 실패로 모든 프레임을 걸러내던 8자리 LGCP 주소 필터가 비로소 동작하게 된다.

---

## 5. 설정 스키마

```yaml
agents:
  - name: lg-pmbus-gateway
    type: lg_hvacr03
    enabled: true
    transport:
      options:
        # --- 트랜스포트 ---
        transport_type: rtu          # rtu | tcp-client
        serial_port: /dev/ttyUSB0    # rtu 필수
        baud_rate: 9600
        data_bits: 8
        stop_bits: 1
        parity: none
        tcp_host: 192.168.0.50       # tcp-client 필수
        tcp_port: 502
        slave_id: 1                  # 1~16 (DIP SW_02M)

        # --- 폴링 ---
        poll_interval: 10s           # 최소 5s
        scan_interval: 30s
        request_timeout: 1s
        offline_timeout: 30s
        reconnect_interval: 5s
        max_reconnect_backoff: 5m

        # --- 미검증 가정 (문서 §8) ---
        temp_scale: 10               # §8 #1 실측 필요
        address_base: 0              # §8 #2 실측 필요
        fan_auto_code: 4             # §8 #3 실측 필요

        # --- 디바이스 관리 ---
        auto_discovery: true
        report_interval: 60s
        event_temp_threshold: 1.0
        devices:
          - address: "0"
            name: 거실
          - address: "3"
            name: 침실

        # --- 제어 ---
        control_enabled: false
        control_verify_delay: 3s

        # --- 로그 ---
        log_messages: false          # TX/RX 프레임 hex
        log_state_updates: false
        log_decode_errors: false
        log_drops: false
```

---

## 6. 수락 기준 요약

전체 수락 기준은 [acceptance.md](acceptance.md) 참조. 핵심 게이트:

1. 문서 §5 의 완성 프레임 12종이 구현 코드의 인코더 출력과 **바이트 단위로 일치**한다.
2. `device_state` 페이로드 구조가 `lg_hvacr02` 와 키·타입 단위로 동일하다.
3. 하드웨어 없이 mock 트랜스포트로 폴링·제어·재연결 전 경로가 검증된다.
4. 단위 테스트 커버리지 85% 이상 (TRUST 5 Tested).

---

## 7. 위험 및 완화

| 위험 | 영향 | 완화 |
|------|------|------|
| 문서 §8 의 미검증 가정이 현장과 불일치 | 온도·주소 전 계산이 어긋남 | §3.1 설정 노출 — YAML 교정으로 재빌드 불필요 |
| ERV / 하이드로킷 레지스터를 실측할 장비가 없음 | 해당 명령의 실동작 미검증 | 구현·단위시험은 수행하되 SPEC 에 **미실측** 명시, 기기 종류별 투영 게이트로 오탐 차단 |
| Modbus 쓰기와 폴링의 버스 경합 | 프레임 desync | REQ-CTRL-009 — 단일 직렬화 경로 |
| 시리얼-이더넷 컨버터의 다중 클라이언트 프레임 분할 | 스트림 desync | `tcp-client` 사용 시 컨버터를 broadcast 모드로 둘 것을 문서화 (기존 RPI 함정 기록) |
| `lg` 패키지 비대화 (현재 21,898줄) | 탐색성 저하 | 신규 파일을 `lg_hvacr03_*` / `lg_pmbus_*` 접두로 일관 분리 |

---

## 8. 참고

- [references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md](references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md) — 기준 프로토콜 분석
- [SPEC-LG-HVACR-002-001](../SPEC-LG-HVACR-002-001/spec.md) — lg_hvacr02 에이전트 (복제 원본)
- [SPEC-LG-HVACR-002-003](../SPEC-LG-HVACR-002-003/spec.md) — lg_hvacr02 플로우 노드 (후속 SPEC 참조 구현)
- [SPEC-MODBUS-006](../SPEC-MODBUS-006/spec.md) — Modbus RTU 트랜스포트
- [SPEC-DEVICE-IDENTITY-001](../SPEC-DEVICE-IDENTITY-001/spec.md) — 디바이스 UUID 체계
