# LG 시스템 에어컨 통신 프로토콜 분석 정리

## 1. 프레임 구조

모든 통신 프레임은 다음과 같은 고정 구조를 가집니다.

```
[STX][LEN][DLEN][DA][SLAN][SA][CMD][SEQ0#][PLEN][PAYLOAD][SEQ1#][CRC]

```

| Field | size(Bytes) | 설명 |
| --- | --- | --- |
| STX | 1 | 프레임 시작(`0x56`) |
| LEN | 1 | 프레임 전체 길이 |
| DLEN | 1 | 목적지 주소 길이 |
| DA | 4 | 목적지 주소 |
| SLEN | 1 | 출발지  주소 길이 |
| SA | 4 | 출발지 주소 |
| CMD | 2 | 명령 코드 |
| SEQ0# | 1 | 명령 순서 번호(명령어 종류별로 1씩 증가) |
| PLEN | 1 | 데이터 길이 |
| PAYLOAD | N | 데이터 |
| SEQ1# | 1 | 프레임 시퀀스 번호 (프레임마다 1씩 증가) |
| CRC | 2 | Checksum (CRC16-CCITT ^ 0x5C56) |

### 예제

```
56
2D
04 44 55 00 66
04 44 55 00 00
02 04 F8
1A 11 00 10 C0 18 00 1A C0 13 00 13 40 13 C0 16 00 18 40 18 80 29 C0 1D C0 91 9D
98
DF 35

```

| Field | size | Value |
| --- | --- | --- |
| STX | 1 | 56 |
| LEN | 1 | 2D |
| DLEN | 1 | 04 |
| DA | 4 | 44 55 00 66 |
| SLEN | 1 | 04 |
| SA | 4 | 44 55 00 00 |
| CMD | 2 | 02 04 |
| SEQ0# | 1 | F8 |
| PLEN | 1 | 26 |
| PAYLOAD | N | 11 00 10 C0 18 ... C0 91 9D |
| SEQ1# | 1 | 98 |
| CRC | 2 | DF 35 |

### Command

| Command | Description | Example |
| --- | --- | --- |
| **02 01** | 설정 | `02 01 3F 04 15 80 68 D8` |
| **02 04** | 상태 | `02 04` |
| **06 04** | 디바이스들에게 상태 요청(순번 항상 0) | `06 04` |
- 필드 코드에 대한 의미는 정확하지 않고, 패턴으로 분석한 결과임

### CRC 계산 방식

- 알고리즘 - CRC-16/CCITT-FALSE
    - 다항식: `0x1021`
    - 초기값: `0xFFFF`
    - RefIn: False
    - RefOut: False
    - XorOut: 0x0000
- 입력
    
    ```
    data = frame[2 : -2]
    
    ```
    
    - Start(0x56), Length 제외
    - Tail의 CRC16 제외
    - **SEQ 바이트 포함**
- 출력
    
    ```
    output = crc_calc ^ 0x5C56
    
    ```
    
    - CRC 계산 후 **상수 XOR 마스크 0x5C56 적용**
- 저장 형식
    - **Big-endian** (상위 바이트, 하위 바이트 순)

### 구현 예시 (Python)

```python
def crc16_ccitt_false(data: bytes, init=0xFFFF):
    reg = init
    for b in data:
        reg ^= (b << 8) & 0xFFFF
        for _ in range(8):
            if reg & 0x8000:
                reg = ((reg << 1) ^ 0x1021) & 0xFFFF
            else:
                reg = (reg << 1) & 0xFFFF
    return reg

def verify_frame(frame: bytes) -> bool:
    data = frame[2:-2]  # exclude start/len, include seq
    crc_calc = crc16_ccitt_false(data)
    stored_crc = (frame[-2] << 8) | frame[-1]
    return (crc_calc ^ 0x5C56) == stored_crc

```

---

> **상세 분석**: 레지스터 맵, 온도 인코딩, 제어 명령 등 상세 분석은 [LGCP_Protocol_Analysis.md](LGCP_Protocol_Analysis.md) 참조.
>
> **주요 확정 사항 (2026-03-26)**:
> - 실내 온도: reg `0x61` attr `0x9_` ext=V → `(157V - V² - 796) / 162` °C (NTC 2차 다항식, 0.5°C 반올림, 제조사 앱 검증 완료)
> - reg `0x74`는 고정값 (0x19=25)으로 실내 온도 아님
> - 3바이트 확장 인코딩: 상위 니블 `0x1_`, `0x5_`, `0x9_`, `0xD_`
> - 제어 명령(0x0201)과 상태 응답(0x0204) 페이로드 분리 처리 필요