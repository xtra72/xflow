# lg_hvacr03 — LG PMBUSB00A Modbus 게이트웨이 에이전트

SPEC-LG-HVACR-003 구현. LG 시스템 에어컨을 LG **공식 Modbus 경로**로 폴링·제어한다.

기준 문서: [PMBUSB00A_Modbus_Protocol_Analysis.md](../../../references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md)

---

## LG 계열 3종 비교

| | `lg_hvacr01` | `lg_hvacr02` | `lg_hvacr03` |
|---|---|---|---|
| 프로토콜 | LG ICP-01 (역공학) | LG ICP-02 (역공학) | **Modbus RTU/TCP (공식)** |
| 수집 방식 | 패시브 캡처 | 패시브 캡처 | **능동 폴링 마스터** |
| 디바이스 발견 | 버스 프레임 관측 | 버스 프레임 관측 | **FC02 전체 스캔** |
| 제어 | 없음 | 서모스탯 사칭 | **정식 레지스터 쓰기** |
| CRC | 자체 체크섬 | CRC-16/XMODEM | **CRC-16/MODBUS** |
| 주소 체계 | 물리 주소 | 8자리 hex (예: `44550067`) | **중앙 주소 N, 0~15** |
| 에러/알람 | 미확인 | 미확인 | **노출** |
| 필터·잠금 | 미확인 | 미확인 | **노출** |
| 압축기 Hz·제상 | — | **노출** | 노출 안 됨 |
| 최대 대수 | 버스 물리 한계 | 버스 물리 한계 | 모듈당 16대 (4모듈 64대) |

**어느 쪽을 쓰나**: 제어·BMS 연동은 `lg_hvacr03`(공식 경로), 압축기 주파수·제상 사이클 같은
저수준 진단은 `lg_hvacr02`(LGCP 스니핑). 두 경로를 병행하는 것이 가장 실용적이며, 실제로
같은 버스에 둘 다 물릴 수 있다 — 한쪽은 실외기 중앙제어 버스, 다른 쪽은 실내기↔실외기 버스다.

> **CRC 혼용 주의**: `lg_hvacr02` 는 CRC-16/XMODEM(0x1021, init 0, Big-Endian),
> `lg_hvacr03` 은 CRC-16/MODBUS(0xA001, init 0xFFFF, Little-Endian)를 쓴다. 두 계열은
> 코드 경로가 분리되어 있다 — `lg_hvacr03_*` / `lg_pmbus_*` 파일은
> `internal/modbus` 의 `CRC16` 만 참조하고 LGCP 체크섬을 import 하지 않는다.

---

## 파일 구성

**프로토콜 층** (트랜스포트 비의존 순수 함수, 커버리지 100%)

| 파일 | 역할 |
|---|---|
| `lg_pmbus_register.go` | 레지스터 주소 계산, 항목 상수 |
| `lg_pmbus_codec.go` | 온도·모드·풍량 인코딩/디코딩 |
| `lg_pmbus_pdu.go` | Modbus PDU 빌더, 응답 파서 |
| `lg_pmbus_device.go` | 디바이스 모델, 기기 종류별 상태 투영 |

**에이전트 층**

| 파일 | 역할 |
|---|---|
| `lg_hvacr03_agent.go` | 구조체, 생명주기, Modbus 트랜잭션, 재연결 |
| `lg_hvacr03_config.go` | 설정 파싱 |
| `lg_hvacr03_device_mgmt.go` | 디바이스 등록·오프라인 감시 |
| `lg_hvacr03_poll.go` | 스캔·폴링 엔진 |
| `lg_hvacr03_emit.go` | `device_state` 방출, dedup |
| `lg_hvacr03_process.go` | 명령 디스패처, 조회·관리 명령 |
| `lg_hvacr03_control.go` | 제어 명령, read-back 검증, 잠금 진단 |
| `lg_hvacr03_provider.go` | `device.DeviceProvider` |
| `lg_hvacr03_executor.go` | `adapter.CommandExecutor` 브릿지 |
| `lg_hvacr03_errors.go` | 센티널 에러, Modbus 예외 |
| `lg_hvacr03_register.go` | 타입 등록 |

---

## 설치 직후 확인할 것

LG 매뉴얼의 레지스터 맵을 근거로 구현했지만, 다음 세 값은 **펌웨어·기종에 따라 다를 수
있어** 설정으로 열어 두었다. 실측이 문서와 다르면 YAML 한 줄만 고치면 된다 (재빌드 불필요).

### 1. 온도 스케일 (`temp_scale`, 기본 10) — 최우선

리모컨으로 **24 ℃** 를 설정한 뒤 해당 실내기의 설정온도를 읽는다.

```bash
# 에이전트가 떠 있으면 상태 조회로 확인
xflow agent exec <agent-name> --command get_state --address 0
```

`target_temperature` 가 24.0 으로 보이면 그대로 두고, 2.4 로 보이면 `temp_scale: 1` 로
고친다. 이 값이 틀리면 모든 온도 표시와 제어가 10배 어긋난다.

### 2. 실내기 주소 기준 (`address_base`, 기본 0) — 최우선

실내기를 **한 대만** 전원 투입하고 디바이스 목록을 본다.

```bash
xflow agent exec <agent-name> --command list_devices
```

그 실내기가 실외기에서 1번으로 설정되어 있는데 목록에 `"0"` 으로 잡히면 기본값이 맞다.
`"1"` 로 잡혀야 한다면 `address_base: 1` 로 고친다.

### 3. 풍량 자동 코드 (`fan_auto_code`, 기본 4)

풍량을 `auto` 로 지정한 뒤 리모컨 표시를 본다.

```bash
xflow agent exec <agent-name> --command set_fan_speed --address 0 --params '{"fan_speed":"auto"}'
```

리모컨이 "자동"을 표시하면 그대로, "초강"을 표시하면 `fan_auto_code: 5` 로 고친다.
LGCP 는 5단계(4=초강, 5=자동) 체계를 쓰므로 기종에 따라 이쪽일 수 있다.

### 그 밖의 실측 항목

| 항목 | 확인 방법 | 대응 |
|---|---|---|
| 쓰기 반영 지연 | 제어 후 `verified` 가 계속 false | `control_verify_delay` 를 늘린다 |
| 최소 안전 폴링 주기 | `poll_interval` 을 5s 부터 올리며 타임아웃 발생률 측정 | 타임아웃이 잦으면 늘린다 |
| 스윙 지원 기종 | 벽걸이/천장형에서 `set_swing` 동작 확인 | 미지원 기종은 명령을 쓰지 않는다 |

---

## 미실측 범위

**ERV(환기)·THERMA V(하이드로킷)** 전용 레지스터는 해당 장비를 확보하지 못해 실동작을
검증하지 못했다. 구현과 단위 시험은 문서 기준으로 수행했으며, 기기 종류 게이트를 두어
에어컨 디바이스에는 노출되지 않는다.

- ERV 는 프로토콜에 구분 신호가 없어 설정에서 `device_type: "HVACR.ERV"` 로 지정해야 한다.
- 하이드로킷은 목표 온도 기준이 "물"로 관측되면 자동 판별되지만, 설정 지정이 우선한다.

또한 PMBUSB00A 는 **전력량·소비전력을 제공하지 않는다**. 필요하면 별도 전력 미터나
LG ACP / AC Smart 계열 상위 컨트롤러가 필요하다.

---

## 플로우 노드

노드 타입 3종을 쓸 수 있다.

| 노드 타입 | 용도 |
|---|---|
| `lg_hvacr03_status` | 상태 수신 (push 모델) |
| `lg_hvacr03_control` | 제어 명령 전송 |
| `lg_hvacr03` | 상태 + 제어 통합 |

**구현은 `lg_hvacr02` 노드와 공유한다.** 노드는 `agent.Agent` 인터페이스로 `Process()` 만
호출하고 주소를 파싱하지 않으므로, 명령 집합과 `device_state` 형식이 같은 두 에이전트는
노드 입장에서 구별되지 않는다. 타입 이름만 분리해 플로우에서 계열이 어긋나 보이지 않게 했다.

```yaml
nodes:
  - name: 거실-상태
    type: lg_hvacr03_status
    config:
      agent_ref: lg-pmbus-gateway
      unit_id: "3"          # 10진 실내기 번호. 생략하면 전체 수신

  - name: 거실-제어
    type: lg_hvacr03_control
    config:
      agent_ref: lg-pmbus-gateway
      default_address: "3"
```

제어 노드로 보내는 메시지:

```json
{"address": "3", "command": "set_multiple",
 "params": {"mode": "cool", "fan_speed": "high", "temperature": 24.0}}
```

응답에는 read-back 결과가 실린다 — `verified` / `expected` / `actual`, 그리고 잠금으로
무시된 경우 `locked_by`.

> **`unit_id` 필터 주의**: 이 필터는 10진 주소("0"~"15")를 쓴다. 원래 `hexByteEqual` 이
> hex 전용이라 "10"을 0x10(16)으로 읽어 N=10~15 가 조용히 빗나갔는데, 정규화 문자열
> 일치를 먼저 시도하도록 보정했다. 같은 보정으로 `lg_hvacr02` 의 8자리 주소 필터도
> 비로소 동작한다 (이전에는 설정하면 모든 프레임이 걸러졌다).
