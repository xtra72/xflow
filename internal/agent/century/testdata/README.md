# Century HVAC raw frame fixtures

원시 프레임 바이너리 파일들. 모두 SPEC-CENTURY-001 의 골든 픽스처이다.

## 출처

- **CAP-3** (2026-05-18 10:01, 냉방 시작 직후, ground truth: 냉방 / 25.0℃ / 바람 17)
- 원본: `references/protocols/century_hvac_protocol_spec.md` v0.3 부록 A
- CRC 알고리즘: CRC-16/ARC, init **0x0000**, polynomial 0x8005 (reflected 0xA001), LE 저장

## 파일

| 파일 | 길이 | 내용 |
|---|---|---|
| `cap3_read_req_reg02.bin` | 13 B | 마스터 → 슬레이브 reg 0x02 read request |
| `cap3_reg02_response.bin` | 30 B | 슬레이브 → 마스터 reg 0x02 read response (payload 20 B) |
| `cap3_read_req_reg03.bin` | 13 B | 마스터 → 슬레이브 reg 0x03 read request |
| `cap3_reg03_response.bin` | 29 B | 슬레이브 → 마스터 reg 0x03 read response (payload 19 B) |
| `cap3_read_req_reg04.bin` | 13 B | 마스터 → 슬레이브 reg 0x04 read request |
| `cap3_reg04_response.bin` | 27 B | 슬레이브 → 마스터 reg 0x04 read response (payload 17 B) |
| `cap3_write_reg04.bin` | 29 B | 마스터 → 슬레이브 reg 0x04 write request (payload 19 B) |
| `cap3_ack.bin` | 11 B | 슬레이브 → 마스터 ACK (payload 1 B = 0x00) |
| `cap3_full_cycle.bin` | 165 B | 위 8 개 frame 의 회선상 등장 순서 그대로 concat |

## 재생성 방법

이 파일들은 원시 bytes 이므로 hex editor 가 없으면 직접 편집하기 어렵다.
원본 hex 표기(공백 포함)는 SPEC 부록 A 에 그대로 남아 있으며, 필요시
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
