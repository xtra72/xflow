# Century HVACR-01 raw frame fixtures

원시 프레임 바이너리 파일들. 모두 SPEC-CENTURY-HVACR-001 의 골든 픽스처이다.

## 출처

- **CAP-1** (2026-05-15 22:47, 꺼짐 상태) — `references/protocols/century_icp01_protocol_spec.md` v0.3 부록 B
- **CAP-3** (2026-05-18 10:01, 냉방 시작 직후, ground truth: 냉방 / 25.0℃ / 바람 17) — 부록 A
- **CAP-4** (2026-05-18 10:45, 냉방 정상상태, 약 44분 경과) — 부록 C
- CRC 알고리즘: CRC-16/ARC, init **0x0000**, polynomial 0x8005 (reflected 0xA001), LE 저장

## 파일

### CAP-3 (냉방 시작 — M1 baseline)

| 파일 | 길이 | 내용 | 핵심 값 |
|---|---|---|---|
| `cap3_read_req_reg02.bin` | 13 B | 마스터 → 슬레이브 reg 0x02 read request | — |
| `cap3_reg02_response.bin` | 30 B | 슬레이브 → 마스터 reg 0x02 read response (payload 20 B) | mode=cooling, fan=17, setpoint=25.0℃, current_temp=25.0℃ (2026-05-29 정정), live14=0x39 |
| `cap3_read_req_reg03.bin` | 13 B | 마스터 → 슬레이브 reg 0x03 read request | — |
| `cap3_reg03_response.bin` | 29 B | 슬레이브 → 마스터 reg 0x03 read response (payload 19 B) | temp_evap_a=19.5℃, temp_evap_b=19.5℃ (과도) |
| `cap3_read_req_reg04.bin` | 13 B | 마스터 → 슬레이브 reg 0x04 read request | — |
| `cap3_reg04_response.bin` | 27 B | 슬레이브 → 마스터 reg 0x04 read response (payload 17 B) | status=0x3B, temp_A=25.2℃, op_val_2=252 |
| `cap3_write_reg04.bin` | 29 B | 마스터 → 슬레이브 reg 0x04 write request (payload 19 B) | mode_cmd=cooling, byte_14=0xC7 |
| `cap3_ack.bin` | 11 B | 슬레이브 → 마스터 ACK (payload 1 B = 0x00) | — |
| `cap3_full_cycle.bin` | 165 B | 위 8 개 frame 의 회선상 등장 순서 그대로 concat | — |

### CAP-1 (꺼짐 상태 — M2 신규)

| 파일 | 길이 | 내용 | 핵심 값 | 출처 라인 |
|---|---|---|---|---|
| `cap1_reg02_response.bin` | 30 B | reg 0x02 read response | mode=off, fan=0, setpoint=0 (꺼짐, 2026-05-29 정정), reg02_word_7=25.0℃ (이전 cooling ceiling 유지) | 부록 B "Read Response reg 0x02" |
| `cap1_reg03_response.bin` | 29 B | reg 0x03 read response | temp_evap_a=21.5℃, temp_evap_b=22.0℃ (실내 평형) | 부록 B "Read Response reg 0x03" |
| `cap1_reg04_response.bin` | 27 B | reg 0x04 read response | status=0x63, op fields 모두 0 | 부록 B "Read Response reg 0x04" |
| `cap1_write_reg04.bin` | 29 B | reg 0x04 write request | mode_cmd=off, data[14]=0xC4 | 부록 B "Write reg 0x04" |

### CAP-4 (냉방 정상상태 — M2 신규)

| 파일 | 길이 | 내용 | 핵심 값 | 출처 라인 |
|---|---|---|---|---|
| `cap4_reg02_response.bin` | 30 B | reg 0x02 read response | mode=cooling, fan=17, setpoint=25.0℃, live14=0x38 (↔0x39 변동) | 부록 C "Read Response reg 0x02" |
| `cap4_reg03_response.bin` | 29 B | reg 0x03 read response | **temp_evap_a=9.0℃, temp_evap_b=8.5℃** (압축기 정상 운전, 두 센서 분리) | 부록 C "Read Response reg 0x03" |
| `cap4_reg04_response.bin` | 27 B | reg 0x04 read response | status=0x39, **op_val_1=996, temp_A=25.2℃, op_val_2=1248** | 부록 C "Read Response reg 0x04" |
| `cap4_write_reg04.bin` | 29 B | reg 0x04 write request | **data[0]=0x02, data[1]=0x04**, mode_cmd=cooling, byte_14=0xC0, live15=0x11 | 부록 C "Write reg 0x04" |

## 재생성 방법

이 파일들은 원시 bytes 이므로 hex editor 가 없으면 직접 편집하기 어렵다.
원본 hex 표기(공백 포함)는 SPEC 부록 A/B/C 에 그대로 남아 있으며, 필요시
다음과 같은 간단한 Go 프로그램으로 재생성한다:

```go
import "encoding/hex"
data, _ := hex.DecodeString(strings.ReplaceAll(
    "01 00 30 00 14 00 00 06 3b 00 02 00 01 11 00 00 00 00 fa 00 00 00 fa 00 1b 39 39 00 c1 4c",
    " ", "",
))
os.WriteFile("cap3_reg02_response.bin", data, 0o644)
```

## CRC 검증

CRC-16/ARC (init 0x0000) 으로 계산한 값과 LE u16 트레일러가 일치해야 한다.
Modbus RTU init (0xFFFF) 로는 일치하지 않아야 한다 (REQ-CENTURY-004 회귀 보호).

모든 13 개 fixture 의 CRC 는 생성 시 검증되었다 (12 개 frame + cap3_full_cycle.bin 의 임베디드 frame 들).

## 부록 B-1 4-capture 핵심 필드 비교 표 (스펙 reference)

| 필드 | CAP-1 (꺼짐) | CAP-3 (냉방 시작) | CAP-4 (냉방 정상) |
|---|---|---|---|
| reg0x02 `data[1]` mode | `0x00` 꺼짐 | `0x01` 냉방 | `0x01` 냉방 |
| reg0x02 `data[2]` fan | `0x00` | `0x11` (17) | `0x11` (17) |
| reg0x02 setpoint | 25.0℃ | 25.0℃ | 25.0℃ |
| reg0x03 냉매온도 a/b | 21.5 / 22.0℃ | 19.5 / 19.5℃ | **9.0 / 8.5℃** |
| reg0x04 `data[0]` status | `0x63` | `0x3B` | `0x39` |
| reg0x04 `data[8..9]` op_val_1 | 0 | 0 | 996 |
| reg0x04 `data[10..11]` temp_A | 0 | 25.2℃ | 25.2℃ |
| reg0x04 `data[12..13]` op_val_2 | 0 | 252 | 1248 |
| WRITE `data[0]` / `data[1]` | 0 / 0 | 0 / 0 | 2 / 4 |
| WRITE `data[4]` mode_cmd | `0x00` | `0x01` | `0x01` |
| WRITE `data[14]` (미상) | `0xC4` | `0xC7` | `0xC0` |
| WRITE `data[15]` (미상) | `0x00` | `0x00` | `0x0F`~`0x11` |
