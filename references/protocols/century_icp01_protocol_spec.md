# Century ICP-01 에어컨 통신 프로토콜 스펙 (Reverse-Engineered, v0.5)

**프로토콜 코드**: `century_icp01` (Century ICP-01 wire protocol)
**에이전트 매핑**: 본 프로토콜을 패시브 sniff 하는 xflow 에이전트 타입은 `century_hvacr01` (Century HVACR-01). 자세한 사양: `SPEC-CENTURY-HVACR-001`.
**문서 상태**: 검증 진행 중 (캡처 4건, 핵심 필드 ground-truth/거동으로 확정)
**분석 일자**: 2026-05-18
**캡처 소스**: `xagent05` 호스트의 `xflowd` 데몬 DEBUG 출력
**검증율**: CRC 일치율 99.7% 이상 (4개 캡처 전체)

### 사용된 캡처

| ID | 일시 | 길이 | 에어컨 상태 |
|---|---|---|---|
| CAP-1 | 2026-05-15 22:47~22:50 | 156초 / 3,047 프레임 | 꺼짐(추정) |
| CAP-2 | 2026-05-18 09:46 | 약 2초 | 꺼짐(추정) |
| CAP-3 | 2026-05-18 10:01 | 약 7초 | **냉방 / 설정 25℃ / 바람 17** (냉방 시작 직후) ← ground truth |
| CAP-4 | 2026-05-18 10:45 | 약 4초 | 냉방 / 설정 25℃ / 바람 17 (냉방 약 44분 경과, steady-state) |

> CAP-3·CAP-4는 사용자가 에어컨 설정을 명시적으로 알려준 캡처로, 본 문서의 register 의미 해석은 이 둘 기준으로 검증되었습니다. CAP-3는 냉방 시작 직후, CAP-4는 충분히 운전된 정상상태를 포착해 운전 과도/정상 거동을 비교할 수 있습니다. "✓ 확정"으로 표시된 항목은 ground truth 또는 물리 거동으로 확인된 것이고, "추정"은 추가 캡처로 검증이 필요한 항목입니다.

---

## 1. 개요

마스터-슬레이브 구조의 바이너리 요청/응답 프로토콜. 마스터(상위 컨트롤러)가 약 512ms 주기로 슬레이브(에어컨)를 polling 한다.

| 항목 | 값 |
|---|---|
| 통신 모델 | Master/Slave (request-response) |
| 인코딩 | Binary, 고정 헤더(8B) + 가변 payload + CRC(2B) |
| 멀티바이트 정수 | Little-endian |
| 무결성 검증 | CRC-16/ARC, LE byte order |
| 폴링 주기 | 약 511.9ms (σ=0.3ms) |
| 온도 인코딩 | LE u16, 0.1℃ 분해능 (값 ÷10) |

---

## 2. 프레임 포맷

```
 0       2       4       6   7   8                       N+8     N+10
+-------+-------+-------+---+---+-----------------------+-----------+
| src   | dst   | p_len |rsv| fc|     payload (N B)     |  CRC-16   |
| LE u16| LE u16| LE u16|=00|   |                       |  LE u16   |
+-------+-------+-------+---+---+-----------------------+-----------+
```

| 오프셋 | 길이 | 필드 | 설명 |
|---|---|---|---|
| 0 | 2 | `src_addr` | 송신자 주소 (LE) |
| 2 | 2 | `dst_addr` | 수신자 주소 (LE) |
| 4 | 2 | `payload_length` | payload 바이트 수 `N` (LE) |
| 6 | 1 | `reserved` | 전 캡처 `0x00` 고정 |
| 7 | 1 | `function_code` | §4 참조 |
| 8 | N | `payload` | §5~6 참조 |
| 8+N | 2 | `crc` | CRC-16/ARC, LE |

`total_length = 8 + payload_length + 2`

---

## 3. CRC 알고리즘

| 항목 | 값 |
|---|---|
| 명칭 | CRC-16/ARC (= CRC-16/IBM) |
| Polynomial | `0x8005` (reflected `0xA001`) |
| Init | `0x0000` |
| RefIn / RefOut | true / true |
| XorOut | `0x0000` |
| 저장 순서 | Little-endian |
| 적용 범위 | 헤더 + payload (CRC 제외) |

> Modbus RTU와 polynomial은 같으나 init이 `0x0000`(Modbus는 `0xFFFF`)인 점에 유의.

```python
def crc16_arc(data: bytes) -> int:
    crc = 0x0000
    for b in data:
        crc ^= b
        for _ in range(8):
            crc = (crc >> 1) ^ 0xA001 if (crc & 1) else (crc >> 1)
    return crc
```

---

## 4. 어드레싱 및 Function Code

| 주소 | 역할 |
|---|---|
| `0x0030` | Master (상위 컨트롤러) |
| `0x0001` | Slave (에어컨 본체) |

| FC | 명칭 | 방향 |
|---|---|---|
| `0x06` | Response / ACK | Slave → Master |
| `0x0B` | Read Request | Master → Slave |
| `0x0C` | Write Request | Master → Slave |

---

## 5. Payload 공통 구조

ACK(1바이트 payload)를 제외한 모든 payload는 다음 3바이트 prefix로 시작한다.

| payload 오프셋 | 필드 | 설명 |
|---|---|---|
| 0 | `sub_dev_id` | 전 캡처 `0x3B`. indoor unit ID로 추정 |
| 1 | `reserved` | `0x00` 고정 |
| 2 | `register` | 0x02 / 0x03 / 0x04 |
| 3~ | `data` | register별 의미 (§6) |

본 문서에서 `data[n]`은 payload 오프셋 `3+n`을 가리킨다. ACK payload는 1바이트 `0x00`.

---

## 6. Register Map

### 6.1 Register 0x02 — 현재 설정 Readback ✓

응답 data 길이 17B. **이 register는 에어컨의 현재 설정 상태를 담는다.**

| data 오프셋 | 필드 | 인코딩 | CAP-3 관측값 | 상태 |
|---|---|---|---|---|
| 1 | `mode` (운전 모드) | u8 | `0x01` (냉방) | **✓ 확정** |
| 2 | `fan` (바람 세기) | u8 | `0x11` = 17 | **✓ 확정** |
| 7–8 | **`current_temp` (현재 실내온도)** | LE u16, ÷10 | `0x00FA` = 250 → 25.0℃ | **✓ 확정 (2026-05-30 실측 검증)** |
| 11–12 | **`setpoint` (설정 온도)** | LE u16, ÷10 | `0x00FA` = 250 → 25.0℃ | **✓ 확정 (2026-05-29 실측 검증)** |
| 13 | (미상, live) | u8 | `0x1B` = 27 | 추정 |
| 14 | (미상, live 변동) | u8 | `0x39`↔`0x38` (57↔56) | 추정 |
| 15 | (미상) | u8 | `0x39` = 57 | 추정 |
| 0,3,4,5,6,9,10,16 | reserved/미사용 | — | `0x00` | — |

**모드 코드 (`data[1]`)** — CAP-3로 확정된 값만 표기:

| 값 | 의미 | 근거 |
|---|---|---|
| `0x00` | 꺼짐 / 대기 | CAP-1, CAP-2 (에어컨 꺼진 상태) |
| `0x01` | **냉방 (cooling)** | ✓ CAP-3 ground truth |
| `0x02`~ | 난방/제습/송풍 등 (미관측) | 추가 캡처 필요 |

**팬 세기 (`data[2]`)**: CAP-3에서 `0x11`(17)이 사용자 설정 "바람 17"과 일치. 1~3단 구조가 아니라 0~N 범위의 수치형 step 값으로 보임 (정확한 범위 미확정).

> **2026-05-29 setpoint 위치 정정**: 6-point 실측 실험 (18/20/22/24/26/28°C) 으로 **setpoint 는 `data[11..12]` LE u16 ÷10** 임이 확정되었다 (180/200/220/240/260/280 → 18~28°C 완벽 linear). 이전 spec 은 `data[7..8]` 을 setpoint 로 가정했으나, CAP-3/4 fixture 가 우연히 두 위치 모두 250 (= 25°C) 이라 미검증 상태였음.

> **2026-05-30 current_temp 확정**: 동일 ambient 조건 (실내 26°C) 에서 OFF↔ON 전환 캡처로 **`data[7..8]` LE u16 ÷10 은 현재 실내온도**임이 확정되었다. OFF/ON 양쪽 모두 `0x0104` (=260 → 26.0°C) 로 ambient 와 일치. 이전 가설(cooling capacity ceiling / max compressor speed) 은 폐기 — 6-point 실험 당시의 240~250 값들은 cooling ceiling 이 아니라 그 시점의 ambient 온도였음. 이로써 Reg 0x02 는 mode/fan/current_temp/setpoint 4종을 모두 담는 "현재 설정+상태" register 로 확정. Reg 0x04 data[10..11] 의 indoor temp 추정은 폐기되고 `reg04_word_10` 으로 격하 (§6.3). data[13–15]는 운전 중에만 0이 아니며 data[14]가 미세 변동하므로 라이브 센서값으로 보이나 미확정.

> **CAP-1 (꺼짐 상태) 차이**: `data[11..12]` (setpoint) 는 0 (active cooling target 없음), `data[7..8]` (current_temp) 은 250 → 25.0°C (꺼짐 시에도 실내온도 센서는 계속 보고). 이전 spec 의 "꺼짐 상태에서도 setpoint 유지" 관찰은 실제로 `data[7..8]` (current_temp) 이 유지된 것이며 setpoint 그 자체가 아니었다.

### 6.2 Register 0x03 — 증발기 냉매 배관 온도 ✓

응답 data 길이 16B. word0/word1 모두 LE u16, ÷10. **실내기 증발기(열교환기)의 냉매 배관 2점 온도.**

| data 오프셋 | 필드 | CAP-1(꺼짐) | CAP-2(꺼짐) | CAP-3(냉방 시작) | CAP-4(냉방 정상상태) |
|---|---|---|---|---|---|
| 0–1 | `evaporator_temperature_a` (액관 추정) | 21.5℃ | 26.0℃ | 19.5℃ | **9.0℃** |
| 2–3 | `evaporator_temperature_b` (가스관 추정) | 22.0℃ | 26.0℃ | 19.5℃ | **8.5℃** |

> **v0.2 → v0.3 확정**: v0.2에서는 "코일 온도(미확정)"로만 표기했으나, CAP-4에서 결론이 났다.
> - **냉방 과도→정상 거동**: 냉방 시작 직후(CAP-3)엔 19.5℃였다가 압축기가 충분히 운전된 뒤(CAP-4, 약 44분 경과)엔 9.0/8.5℃까지 내려갔다. 이는 가동 중 증발기의 전형적 증발온도대(5~12℃)로, 이 값들이 **증발기 냉매 배관 온도**임을 확정한다. (토출 공기 온도 후보는 폐기 — 공기 온도라면 이렇게까지 내려가지 않는다.)
> - **2개 독립 센서 확정**: 꺼짐·과도 상태에서는 word0=word1로 같았으나(평형), 정상상태(CAP-4)에서 9.0/8.5℃로 분리됐다. 한 값의 복제가 아니라 **액관·가스관 등 서로 다른 2개 지점**의 측정값이다.
> - 액관/가스관 어느 쪽이 word0인지는 미확정(난방 전환 캡처로 구분 가능). 라벨 `evaporator_temperature_a/b`는 위치 미지정 의미.

### 6.3 Register 0x04 — 운전 상태 + 운전 데이터

응답 data 길이 14B.

| data 오프셋 | 필드 | CAP-1 | CAP-2 | CAP-3(냉방 시작) | CAP-4(정상상태) | 비고 |
|---|---|---|---|---|---|---|
| 0 | `status` (bitmap) | `0x63` | `0x4D` | `0x3B` | `0x39` | 캡처마다 변동, 동적 비트 포함 |
| 1 | (미상) | `0xF6` | `0xF6` | `0xF6` | `0xF6` | 4캡처 동일 |
| 2 | (미상) | `0x09` | `0x09` | `0x09` | `0x09` | 4캡처 동일 |
| 7 | (미상) | `0x2C` | `0x2C` | `0x2C` | `0x2C` | 4캡처 동일 |
| 8–9 | `op_val_1` (운전 부하 추정) | `0` | `0` | `0` | `996` | 정상운전 시 채워짐 |
| 10–11 | `reg04_word_10` (의미 미확정) | `0` | `0` | `0x00FC`=252 | `0x00FC`=252 | 운전 중에만 채워짐, 실내온도 가설 폐기 (2026-05-30) |
| 12–13 | `op_val_2` (운전 부하 추정) | `0` | `0` | `0x00FC`=252 | `1248` | 운전 부하에 따라 변동 |

> **v0.3 추가**: CAP-4에서 data[8–9]가 `996`, data[12–13]이 `1248`로 새로 채워졌다(CAP-3에선 0 또는 252). 압축기가 정상 부하 운전에 들어가면서 나타나는 값이라 **압축기 주파수 / 소비전력 / 운전 적산값** 등의 운전 부하 지표로 추정되나, 단위·의미는 미확정. data[0] `status`는 4캡처 모두 다른 값(`0x63`/`0x4D`/`0x3B`/`0x39`)으로, 모드 외 동적 비트가 섞인 bitmap이다.

> **v0.5 정정 (2026-05-30)**: data[10–11] 의 "실내/리턴에어 온도" 가설은 폐기되었다. 동일 ambient 26°C 에서 OFF↔ON 캡처 시 data[10–11] 값이 ambient 와 일치하지 않은 반면 Reg 0x02 data[7..8] 이 정확히 ambient (260=26°C) 와 일치했기 때문에, **현재 실내온도의 실제 source 는 Reg 0x02 data[7..8] (`current_temp`)** 으로 확정 (§6.1). data[10–11] 은 `reg04_word_10` 으로 격하 (운전 중에만 채워지는 미상 필드).

### 6.4 Register 0x04 Write — 운전 제어 명령

요청 data 길이 16B. 마스터가 매 cycle 슬레이브로 전송, 슬레이브는 1바이트 ACK 회신.

| data 오프셋 | 필드 | CAP-1 | CAP-2 | CAP-3 | CAP-4 | 상태 |
|---|---|---|---|---|---|---|
| 0 | (미상) | `0x00` | `0x00` | `0x00` | `0x02` | 운전 중 set, 미확정 |
| 1 | (미상) | `0x00` | `0x00` | `0x00` | `0x04` | 운전 중 set, 미확정 |
| 4 | `mode_cmd` (모드 명령) | `0x00` | `0x00` | `0x01` | `0x01` | **✓ 확정** |
| 14 | (미상 상수) | `0xC4` | `0xC7` | `0xC7` | `0xC0` | 미확정 (캡처별 19.2~19.9 범위) |
| 15 | (미상, live) | `0x00` | `0x00` | `0x00` | `0x0F`~`0x11` | 캡처 내 15~17 변동, 미확정 |
| 그 외 | reserved | `0x00` | `0x00` | `0x00` | `0x00` | — |

> **v0.3 추가**: CAP-4에서 WRITE에 새 필드가 등장했다. data[0]=`0x02`, data[1]=`0x04`가 운전 중 set 되고, data[15]가 캡처 내에서 `0x0F`~`0x11`(15~17)로 단조 감소가 아닌 **불규칙 변동**을 보인다 — 카운터가 아니라 라이브 센서값(마스터측 온도 등)으로 추정. 마스터가 슬레이브로 써넣는 값이라 마스터(컨트롤러) 측 상태/센서로 보이나 의미는 미확정. data[14]는 캡처별 `0xC4`/`0xC7`/`0xC0`로 19.2~19.9 범위를 오가며, v0.1의 "설정 온도" 추정은 v0.2에서 이미 철회됨.

---

## 7. 통신 시퀀스 및 타이밍

### 7.1 한 polling cycle (약 512ms)

```
MASTER ──READ  reg=0x02──▶ SLAVE        설정 readback 요청
MASTER ◀─RESP  reg=0x02── SLAVE         (mode/fan/setpoint)
MASTER ──READ  reg=0x03──▶ SLAVE        증발기 냉매온도 요청
MASTER ◀─RESP  reg=0x03── SLAVE
MASTER ──READ  reg=0x04──▶ SLAVE        운전상태 요청
MASTER ◀─RESP  reg=0x04── SLAVE
MASTER ──WRITE reg=0x04──▶ SLAVE        운전 제어 명령
MASTER ◀─ACK ───────────  SLAVE         (1B = 0x00)
MASTER ──WRITE reg=0x04──▶ SLAVE        동일 명령 재전송
MASTER ◀─ACK ───────────  SLAVE
─── 다음 cycle ───
```

### 7.2 타이밍 특성

| 항목 | 값 |
|---|---|
| Cycle 주기 | 약 511.9ms (σ=0.3ms) |
| Request→Response 응답시간 | 약 27~45ms |
| Cycle당 read | 3 (reg 0x02, 0x03, 0x04) |
| Cycle당 write | 2 (둘 다 reg 0x04, 동일 payload) |

WRITE는 매 cycle 두 번 동일 전송된다 (신뢰성용 redundancy 추정).

---

## 8. 정리: 확정 / 미확정

### 8.1 확정 (✓)

- 프레임 포맷, CRC 알고리즘(CRC-16/ARC, LE)
- Function code 3종, Master/Slave 주소
- Polling 주기/구조
- **Reg 0x02 = 현재 설정+상태 readback**: mode(`data[1]`), fan(`data[2]`), current_temp(`data[7–8]` LE u16 ÷10, 2026-05-30 확정), setpoint(`data[11–12]` LE u16 ÷10, 2026-05-29 위치 정정)
- 모드 코드 `0x01`=냉방, `0x00`=꺼짐
- **Reg 0x03 = 증발기 냉매 배관 온도** (word0/word1, LE u16 ÷10) — 2개 독립 센서
- **WRITE = 운전 제어 명령**: mode_cmd(`data[4]`)
- 온도 인코딩 = LE u16, 0.1℃ 분해능

### 8.2 미확정 (추가 캡처 필요)

| 항목 | 확정 방법 |
|---|---|
| 모드 코드 `0x02`+ (난방/제습/송풍) | 각 모드로 바꿔가며 캡처 |
| 팬 세기 값 범위 | 바람 단계를 최소~최대로 바꿔가며 캡처 |
| Reg 0x02 data[13–16] (live 필드) | 운전 시간/모터 RPM/디퓨저 위치 등 추정, 단위 미확정 |
| Reg 0x03 word0/word1 액관·가스관 구분 | 냉방/난방 전환 시 두 값의 대소 역전 관찰 |
| Reg 0x04 data[0] status 비트맵 | 전원·모드·팬 개별 변경 후 비트 매핑 |
| Reg 0x04 data[8–9]·data[12–13] 운전 부하값 | 압축기 부하 변동 구간 장시간 캡처, 단위 추정 |
| Reg 0x04 data[10–13] (`reg04_word_10`/`op_val_2`) 의미 | 다양한 운전 조건 변동 캡처 — 실내온도 가설은 §6.1 current_temp 확정으로 폐기됨 |
| WRITE data[0]·data[1] (운전 중 set) | 모드별·운전조건별 캡처 |
| WRITE data[14]·data[15] | 마스터 설정/펌웨어 정보 확인 |
| 다른 register(0x00/0x01/0x05+) 존재 | 장시간 캡처 |
| 다중 indoor unit 시 `sub_dev_id` | 유닛 추가 환경 캡처 |

### 8.3 권장 다음 캡처 시나리오

1. 설정 온도만 1℃씩 단계적으로 변경 (모드·팬 고정) → setpoint 외 어느 바이트가 연동되는지 확인
2. 팬 세기를 최저~최고로 변경 → `data[2]` 값 범위 확정
3. 냉방→난방→제습→송풍 순차 전환 → 모드 코드표 완성
4. 전원 ON/OFF → reg 0x04 `data[0]` 비트 변화 관찰
5. 냉방 가동 후 실내가 식는 동안 장시간 캡처 → reg 0x02 `current_temp` (data[7..8]) 의 실시간 추이 검증 및 reg 0x04 data[10–13] 의미 추가 단서 수집

---

## 9. 참조 구현 (Python 디섹터)

```python
from dataclasses import dataclass
from typing import Optional

FC_RESPONSE, FC_READ, FC_WRITE = 0x06, 0x0B, 0x0C

MODE_NAMES = {0x00: "꺼짐/대기", 0x01: "냉방"}  # 0x02+ 미확정


def crc16_arc(data: bytes) -> int:
    crc = 0x0000
    for b in data:
        crc ^= b
        for _ in range(8):
            crc = (crc >> 1) ^ 0xA001 if (crc & 1) else (crc >> 1)
    return crc


@dataclass
class Frame:
    src: int
    dst: int
    fc: int
    payload: bytes
    crc_ok: bool

    @property
    def register(self) -> Optional[int]:
        return self.payload[2] if len(self.payload) >= 3 else None

    @property
    def data(self) -> bytes:
        return self.payload[3:] if len(self.payload) >= 3 else b""


def parse(frame: bytes) -> Optional[Frame]:
    if len(frame) < 10:
        return None
    src  = frame[0] | (frame[1] << 8)
    dst  = frame[2] | (frame[3] << 8)
    plen = frame[4] | (frame[5] << 8)
    fc   = frame[7]
    if len(frame) != 8 + plen + 2:
        return None
    payload  = frame[8:8 + plen]
    recv_crc = frame[-2] | (frame[-1] << 8)
    return Frame(src, dst, fc, payload, crc16_arc(frame[:-2]) == recv_crc)


def decode_reg02(d: bytes) -> dict:
    """Register 0x02 응답 해석 (현재 설정 + 상태 readback)."""
    if len(d) < 13:
        return {}
    return {
        "mode":            MODE_NAMES.get(d[1], f"0x{d[1]:02x}?"),
        "fan":             d[2],
        "current_temp_c":  (d[7]  | (d[8]  << 8)) / 10.0,  # 2026-05-30 확정: data[7..8]
        "setpoint_c":      (d[11] | (d[12] << 8)) / 10.0,  # 2026-05-29 정정: data[11..12]
    }


def decode_reg03(d: bytes) -> dict:
    """Register 0x03 응답 해석 (증발기 냉매 배관 온도)."""
    if len(d) < 4:
        return {}
    return {
        "evaporator_temperature_a": (d[0] | (d[1] << 8)) / 10.0,
        "evaporator_temperature_b": (d[2] | (d[3] << 8)) / 10.0,
    }


def build_read(register: int, sub_dev_id: int = 0x3B) -> bytes:
    payload = bytes([sub_dev_id, 0x00, register])
    body = bytes([0x30, 0x00, 0x01, 0x00, len(payload), 0x00, 0x00, FC_READ]) + payload
    crc = crc16_arc(body)
    return body + bytes([crc & 0xFF, (crc >> 8) & 0xFF])
```

---

## 10. 변경 이력

| 버전 | 일자 | 변경 내용 |
|---|---|---|
| v0.1 | 2026-05-15 | 초안. CAP-1 기반. 프레임 포맷/CRC/3개 register 식별. |
| v0.2 | 2026-05-18 | CAP-2·CAP-3 추가. CAP-3 ground truth(냉방/25℃/바람17)로 검증. **확정**: reg 0x02 mode·fan·setpoint, WRITE mode_cmd. **정정**: reg 0x02 setpoint는 단일바이트가 아닌 LE u16 / reg 0x03은 실내온도가 아닌 코일온도 / WRITE setpoint 추정 철회(실제 모드는 data[4]). |
| v0.3 | 2026-05-18 | CAP-4(냉방 정상상태) 추가. **확정**: reg 0x03 = 증발기 냉매 배관 온도(2개 독립 센서) — 냉방 과도(19.5℃)→정상(9.0/8.5℃) 거동으로 검증, 토출 공기 후보 폐기. **추가**: reg 0x04 data[8–9]·data[12–13] 운전 부하값 / WRITE data[0]·data[1]·data[15] 운전 중 신규 필드. |
| v0.4 | 2026-05-29 | **정정**: reg 0x02 setpoint 위치 `data[7..8]` → **`data[11..12]`** (6-point 사용자 실측 실험으로 확정 — 18/20/22/24/26/28°C 가 data[11..12]÷10 과 완벽 linear 일치). 이전 가정은 CAP-3/4 가 우연히 두 byte 쌍 모두 250 이라 검증되지 않았음. **재분류**: `data[7..8]` 은 setpoint 아닌 별개 운전 파라미터 (≤25°C 시 250 고정, 26°C → 245, 28°C → 240 — cooling capacity ceiling / max compressor speed 추정) — `reg02_word_7` 로 노출. CAP-1 (꺼짐) 해석 정정: `data[11..12]=0` (active target 없음), `data[7..8]=250` (이전 ceiling 유지). |
| v0.5 | 2026-05-30 | **확정**: reg 0x02 `data[7..8]` 의 실제 의미는 **현재 실내온도 (`current_temp`)** — 동일 ambient 26°C 에서 OFF↔ON 캡처로 `0x0104=260 → 26.0°C` 가 양쪽 모두에서 ambient 와 일치함을 확인. v0.4 의 "cooling capacity ceiling / max compressor speed" 가설은 폐기됨 — 6-point 실험 (18~28°C) 의 240~250 값들은 ceiling 이 아니라 그 시점의 실제 ambient 온도였음. 이로써 Reg 0x02 는 mode/fan/current_temp/setpoint 4종을 모두 담는 "현재 설정+상태" register 로 확정. **격하**: reg 0x04 `data[10..11]` 의 "실내/리턴에어 온도 (`temp_A`)" 가설은 폐기되고 `reg04_word_10` (의미 미확정) 으로 표기. **컴파일러 구현**: 디코더에서 `Reg02Word7` → `CurrentTempC` (Confirmed), `Reg04.TempAC` → `Reg04Word10` (Inferred) 로 리네이밍. snapshot/payload 의 `current_temp` 소스를 Reg04 → Reg02 로 변경. |

---

## 부록 A. CAP-3 원시 프레임 예시 (냉방 / 설정 25℃ / 바람 17)

```
─── Read Request reg 0x02 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 02 33 b8

─── Read Response reg 0x02 (설정 readback) ───────────
01 00 30 00 14 00 00 06 3b 00 02 [00 01 11 00 00 00 00 fa 00 00 00 fa 00 1b 39 39 00] c1 4c
  data[1]=01 → 냉방   data[2]=11 → 바람17
  data[7..8]=fa 00 → 현재 실내온도 25.0℃ (current_temp, 2026-05-30 확정)
  data[11..12]=fa 00 → 설정 25.0℃ (setpoint, 2026-05-29 실측 검증)
  ※ CAP-3 에서는 ambient 25°C 와 설정 25°C 가 우연히 동일 — 다른 ambient/setpoint 조합에서 두 값이 분리됨

─── Read Request reg 0x03 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 03 f2 78

─── Read Response reg 0x03 (증발기 냉매온도) ────────────────
01 00 30 00 13 00 00 06 3b 00 03 [c3 00 c3 00 00 00 00 00 00 00 00 00 00 00 00 00] da 41
  word0=word1=0x00C3=195 → 19.5℃ (냉방 시작 직후, 아직 과도상태)

─── Read Request reg 0x04 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 04 b3 ba

─── Read Response reg 0x04 (운전상태) ────────────────
01 00 30 00 11 00 00 06 3b 00 04 [3b f6 09 00 00 00 00 2c 00 00 fc 00 fc 00] 75 59
  data[0]=3b → status   data[10..13]=fc 00 fc 00 → reg04_word_10 / op_val_2 (의미 미확정)

─── Write reg 0x04 (운전 제어 명령) ──────────────────
30 00 01 00 13 00 00 0c 3b 00 04 [00 00 00 00 01 00 00 00 00 00 00 00 00 00 c7 00] d8 7e
  data[4]=01 → 냉방 명령

─── ACK (Write 응답) ─────────────────────────────────
01 00 30 00 01 00 00 06 00 03 f3
  payload=0x00
```

## 부록 B. CAP-1 원시 프레임 예시 (꺼짐 상태)

CAP-1(2026-05-15)은 에어컨이 꺼진 상태로 추정되는 캡처다. 운전 모드/팬이 `0x00`이고, reg 0x04의 운전 중 온도 필드(data[10–13])가 0으로 비어 있는 점이 CAP-3와 대조된다. CRC는 8개 프레임 모두 검증 통과.

```
─── Read Request reg 0x02 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 02 33 b8

─── Read Response reg 0x02 (설정 readback) ───────────
01 00 30 00 14 00 00 06 3b 00 02 [00 00 00 00 00 00 00 fa 00 00 00 00 00 00 00 00 00] 01 33
  data[1]=00 → 꺼짐/대기   data[2]=00 → 팬 정지
  data[7..8]=fa 00 → current_temp=25.0℃ (꺼짐 상태에서도 실내온도 센서는 계속 보고, 2026-05-30 확정)
  data[11..12]=00 00 → setpoint=0 (active cooling target 없음, 2026-05-29 정정)
  ※ 이전 spec 의 "꺼짐 상태에서도 setpoint 유지" 관찰은 실제로 data[7..8]
     (current_temp) 이 유지된 것으로 재해석됨 — setpoint 그 자체가 아님
  data[13~16]=00 → 운전 중에만 채워지는 live 필드, 여기선 비어 있음

─── Read Request reg 0x03 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 03 f2 78

─── Read Response reg 0x03 (증발기 냉매온도) ────────────────
01 00 30 00 13 00 00 06 3b 00 03 [d7 00 dc 00 00 00 00 00 00 00 00 00 00 00 00 00] cb 91
  word0=0x00D7=215 → 21.5℃   word1=0x00DC=220 → 22.0℃
  (꺼짐 상태 → 코일이 실내온도와 평형, 두 값이 비슷)

─── Read Request reg 0x04 ────────────────────────────
30 00 01 00 03 00 00 0b 3b 00 04 b3 ba

─── Read Response reg 0x04 (운전상태) ────────────────
01 00 30 00 11 00 00 06 3b 00 04 [63 f6 09 00 00 00 00 2c 00 00 00 00 00 00] 5d 91
  data[0]=63 → status (CAP-2=0x4D, CAP-3=0x3B 와 모두 다름)
  data[10..13]=00 00 00 00 → 운전 중 온도 필드 비어 있음 (꺼짐)

─── Write reg 0x04 (운전 제어 명령) ──────────────────
30 00 01 00 13 00 00 0c 3b 00 04 [00 00 00 00 00 00 00 00 00 00 00 00 00 00 c4 00] 25 4d
  data[4]=00 → 모드 명령 = 꺼짐
  data[14]=c4 (CAP-2/CAP-3 의 0xC7 과 다름, 의미 미확정)

─── ACK (Write 응답) ─────────────────────────────────
01 00 30 00 01 00 00 06 00 03 f3
  payload=0x00
```

### 부록 B-1. 4개 캡처 핵심 필드 비교

| 필드 | CAP-1 (꺼짐) | CAP-2 (꺼짐) | CAP-3 (냉방 시작) | CAP-4 (냉방 정상) |
|---|---|---|---|---|
| reg0x02 `data[1]` mode | `0x00` 꺼짐 | `0x00` 꺼짐 | `0x01` 냉방 | `0x01` 냉방 |
| reg0x02 `data[2]` fan | `0x00` | `0x00` | `0x11` (17) | `0x11` (17) |
| reg0x02 `data[7..8]` current_temp | 25.0℃ | 27.0℃ | 25.0℃ | 25.0℃ |
| reg0x02 `data[11..12]` setpoint | 0 (off) | 0 (off) | 25.0℃ | 25.0℃ |
| reg0x03 냉매온도 a/b | 21.5 / 22.0℃ | 26.0 / 26.0℃ | 19.5 / 19.5℃ | **9.0 / 8.5℃** |
| reg0x04 `data[0]` status | `0x63` | `0x4D` | `0x3B` | `0x39` |
| reg0x04 `data[8..9]` op_val_1 | 0 | 0 | 0 | 996 |
| reg0x04 `data[10..11]` reg04_word_10 (미확정) | 0 | 0 | 252 | 252 |
| reg0x04 `data[12..13]` op_val_2 | 0 | 0 | 252 | 1248 |
| WRITE `data[0]` / `data[1]` | 0 / 0 | 0 / 0 | 0 / 0 | 2 / 4 |
| WRITE `data[4]` mode_cmd | `0x00` | `0x00` | `0x01` | `0x01` |
| WRITE `data[14]` (미상) | `0xC4` | `0xC7` | `0xC7` | `0xC0` |
| WRITE `data[15]` (미상) | `0x00` | `0x00` | `0x00` | `0x0F`~`0x11` |

## 부록 C. CAP-4 원시 프레임 예시 (냉방 정상상태, 약 44분 경과)

CAP-4는 동일 설정(냉방/25℃/바람17)에서 압축기가 충분히 운전된 정상상태 캡처다. 설정값(reg 0x02)은 CAP-3와 동일하나 물리량(reg 0x03 냉매온도, reg 0x04 운전값)이 크게 변했다. CRC 검증 통과.

```
─── Read Response reg 0x02 (설정 readback) ───────────
01 00 30 00 14 00 00 06 3b 00 02 [00 01 11 00 00 00 00 fa 00 00 00 fa 00 1b 38 39 00] 90 8c
  data[1]=01·data[2]=11·current_temp(data[7..8])=25.0℃·setpoint(data[11..12])=25.0℃ → CAP-3와 동일 (설정·실내온도 모두 불변)

─── Read Response reg 0x03 (증발기 냉매온도) ─────────
01 00 30 00 13 00 00 06 3b 00 03 [5a 00 55 00 00 00 00 00 00 00 00 00 00 00 00 00] e6 ed
  word0=0x005A=90 → 9.0℃   word1=0x0055=85 → 8.5℃
  (냉방 정상상태 → 증발기 냉매온도가 9℃대로 하강, 두 값 분리)

─── Read Response reg 0x04 (운전상태) ────────────────
01 00 30 00 11 00 00 06 3b 00 04 [39 f6 09 00 00 00 00 2c e4 03 fc 00 e0 04] 2c 7c
  data[0]=39 status   data[8..9]=e4 03 → 996   data[10..11]=fc 00 → reg04_word_10=252 (의미 미확정)
  data[12..13]=e0 04 → 1248  (운전 부하 데이터, CAP-3에선 0/252)

─── Write reg 0x04 (운전 제어 명령) ──────────────────
30 00 01 00 13 00 00 0c 3b 00 04 [02 04 00 00 01 00 00 00 00 00 00 00 00 00 c0 11] 9f 20
  data[4]=01 → 냉방 명령
  data[0]=02·data[1]=04 신규 set   data[15]=0f~11 (15~17 변동, 라이브값 추정)
```
