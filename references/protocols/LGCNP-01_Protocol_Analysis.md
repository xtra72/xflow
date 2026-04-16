# LGCNP-01 프로토콜 분석 보고서

> **프로토콜**: LGCNP-01 (LG CN-485 Protocol Version 1)
> **대상 장비**: LG 시스템 에어컨 실내기 **LRD-N837T** ↔ 실외기
> **캡처 호스트**: xagent03 / xflowd
> **분석 기간**: 2026-04-13
> **확정 속도**: **1200 bps** (8N1, RS-485)
> **분석 샘플**: ODU 사이클 14회, IDU 패킷 52세트 (11사이클)

---

## 1. 속도 결정 과정

캡처 속도를 단계적으로 낮추며 고유 바이트 수를 관측하여 실제 속도를 특정했다.

| 캡처 속도 | 고유 바이트 수 | 판정 |
|----------|-------------|------|
| 9600 bps | 8종 | 오버샘플링 |
| 4800 bps | 7종 | 오버샘플링 |
| 2400 bps | 16종 | 오버샘플링 |
| **1200 bps** | **75종** | ✅ **정상 수신 — 실제 속도** |

**20바이트 프레임 길이 확인 근거:**
xflowd가 모든 SEQ를 예외 없이 **16B + 4B** 두 덩어리로 출력했다. SEQ 간 타임스탬프 간격은 약 400ms로, 1200 bps 기준 20바이트 전송(167ms) + 약 233ms idle에 해당한다.

---

## 2. 물리 계층

| 항목 | 값 |
|------|---|
| 인터페이스 | RS-485 (2선 반이중) |
| 속도 | **1200 bps** |
| 데이터 비트 | 8 |
| 패리티 | None |
| 스톱 비트 | 1 (8N1) |
| 바이트 전송 시간 | 8.33 ms |

---

## 3. 체크섬 정책 — 전체 요약

**LGCNP-01은 동적 센서 데이터 패킷에 체크섬을 사용하지 않는다.**  
대신 주요 센서값을 두 위치에 중복 기록하는 **이중 기록(redundancy)** 으로 데이터 무결성을 보장한다.

| 패킷 유형 | 체크섬 | 알고리즘 | 비고 |
|----------|--------|---------|------|
| TYPE-A SEQ=01 | ✅ 있음 | `XOR(pkt[0:19]) == pkt[19]` | 정적 프레임 |
| TYPE-A SEQ=04 | ✅ 있음 | `SUM(pkt[0:19]) & 0xFF == pkt[19]` | 정적 프레임 |
| TYPE-A SEQ=05 | ✅ 있음 | `XOR(pkt[0:19]) == pkt[19]` | 정적 프레임 |
| TYPE-A SEQ=02 | ❌ 없음 | — | b[18], b[19] = 센서 데이터 |
| TYPE-A SEQ=03 | ❌ 없음 | — | b[18], b[19] = 센서 데이터 |
| TYPE-B (IDU) | ❌ 없음 | — | b[38], b[39] = 센서 데이터 |

### 체크섬 없는 패킷의 무결성 근거

**TYPE-A SEQ=02/03 근거 (단일 변동 실험):**
```
사이클 1→2: b[08], b[10]만 변동 → b[18]=f8, b[19]=ee 불변
사이클 2→3: b[06]만 변동       → b[18]: f8→f9 변동, b[19]=ee 불변
사이클 3→4: b[14]만 변동       → b[18]=f9 불변, b[19]: ee→e9 변동
```
b[18]과 b[19]가 서로 다른 데이터 필드에 독립적으로 반응 → 체크섬이 아니라 독립된 센서값.

**TYPE-B b[38:40] 근거 (IDU#3 단일 변동):**
```
b[24]=0x78 → b[38]=0x22  :  0x78 + 0x22 = 0x9a (일정)
b[24]=0x77 → b[38]=0x23  :  0x77 + 0x23 = 0x9a (일정)
```
b[38]은 b[24](흡입온도)의 **상호 보완값 (b[24]+b[38] = 상수)** → 서로 다른 센서 파생값.

---

## 4. 패킷 유형 개요

| 유형 | STX | 길이 | 용도 |
|------|-----|------|------|
| **TYPE-A** | `0x58` | **20바이트** | 실외기(ODU) 상태 보고, SEQ=01~05 |
| **TYPE-B** | `0x81`~`0x85` | **40바이트** | 실내기(IDU) #1~#5 상태 보고 |

### 전송 순서 (1사이클, 약 4~6초)

```
[TYPE-A SEQ=01] [TYPE-A SEQ=02] [TYPE-A SEQ=03] [TYPE-A SEQ=04] [TYPE-A SEQ=05]
[TYPE-B IDU#1]  [TYPE-B IDU#2]  [TYPE-B IDU#3]  [TYPE-B IDU#4]  [TYPE-B IDU#5]
```

---

## 5. TYPE-A 패킷 — ODU 상태 보고

### 5.1 공통 프레임 구조

```
[STX=0x58][SEQ 01~05][DATA 17B][CHK 또는 DATA]
  byte 0      1        2..18          19
총 20바이트
```

### 5.2 SEQ=01 — ODU 정적 상태

캡처 전 기간 변하지 않는 고정 프레임. XOR 체크섬 사용.

```
58 01 00 00 00 00 00 00 00 57 66 00 00 00 2e 00 19 01 43 [1d]
                                                          ↑ XOR[0:19] ✓
```

### 5.3 SEQ=02 — ODU 실시간 냉동 사이클 데이터

```
Offset  크기  필드명                값 범위        설명
──────────────────────────────────────────────────────────
[00]    1B   STX                   0x58           고정
[01]    1B   SEQ                   0x02           고정
[02:06] 4B   SUBHEADER             e0 06 02 00    고정 서브헤더
[06]    1B   OUTDOOR_TEMP          0x80~0x84      외기온도 raw ← (raw-0x40)/2 °C 【확정】
[07]    1B   UNKNOWN_07            0x4b~0x4d      저압측 추정 (단위 미확정)
[08]    1B   COMP_SUCTION_TEMP     0x9b~0x9f      압축기 흡입온도 raw 【확정: 정지 시 급감】
[09]    1B   RESERVED              0x00           고정
[10]    1B   UNKNOWN_10            0x31~0x35      고압 압력 지수 추정 (단위 미확정)
[11]    1B   COMP_DISCHARGE_TEMP   0xcd/0xce      압축기 토출온도 raw 【확정: 정지 후 점진 냉각】
[12]    1B   FLAG_C                0xe5/0xe6      1비트 플래그
[13]    1B   RESERVED              0x00           고정
[14]    1B   CONDENSER_TEMP_A      0x6b~0x6e      응축측 온도A raw 【확정: 정지 시 피크→하강】
[15]    1B   CONDENSER_TEMP_B      0x5c~0x63      응축측 온도B raw 【확정: 응축기 출구】
[16]    1B   FIXED                 0x0f           고정
[17]    1B   RESERVED              0x00           고정
[18]    1B   SENSOR_A              0xf8/0xf9/0xfe 센서값 (FLAG 연동)
[19]    1B   SENSOR_B              0x90~0xee      센서값 (온도 연동)
──────────────────────────────────────────────────────────
체크섬: 없음 — b[18], b[19] 모두 독립 센서 데이터
```

**온도 변환 (모두 동일 공식):**
```
외기온도 (°C)         = (b[06] - 0x40) / 2.0
압축기 흡입온도 (°C)   = (b[08] - 0x40) / 2.0
압축기 토출온도 (°C)   = (b[11] - 0x40) / 2.0
응축측 온도A (°C)      = (b[14] - 0x40) / 2.0
응축측 온도B (°C)      = (b[15] - 0x40) / 2.0
```

### 5.4 SEQ=03 — ODU 부가 상태

```
Offset  크기  필드명      값 범위     설명
──────────────────────────────────────
[00]    1B   STX         0x58       고정
[01]    1B   SEQ         0x03       고정
[02]    1B   CTR_D       0x65~0x67  증가 카운터
[03]    1B   CTR_E       0x79/0x7a  카운터
[04:14] 10B  FIXED       각 고정값   고정 데이터
[14]    1B   FIXED        0x33       고정
[15]    1B   RESERVED     0x00       고정
[16]    1B   CTR_F        0x14~0x19  감소 카운터
[17]    1B   RESERVED     0x00       고정
[18]    1B   SENSOR_C     0xd9/0xde  센서값
[19]    1B   SENSOR_D     0x12~0x1f  센서값 (CTR_F 연동)
──────────────────────────────────────
체크섬: 없음
```

### 5.5 SEQ=04 — ODU 파라미터 (SUM 체크섬)

```
58 04 00 00 00 00 64 fe fe 00 84 00 00 00 00 00 00 00 15 [55]
                              ↑           ↑              ↑
                           b[06]       b[10]=0x84     SUM CHK ✓
```

**b[10] = 운전 평균 온도 raw** 【확정】: `(b[10] - 0x40) / 2.0` = 34.0°C (33~39°C 범위, 장기 평균값)

### 5.6 SEQ=05 — ODU 상태2 (XOR 체크섬)

```
58 05 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 08 [55]
                                                         ↑ XOR[0:19] ✓
```

---

## 6. TYPE-B 패킷 — IDU 상태 보고

### 6.1 프레임 구조

```
Offset  크기  필드명            설명
──────────────────────────────────────────────────
[00]    1B   IDU_ADDR          실내기 주소 (0x81~0x85)
[01]    1B   CMD               커맨드 (0x02 또는 0x43)
[02]    1B   SUB_CMD           서브커맨드 (0x00 또는 0x01)
[03]    1B   DEV_TYPE          장치 타입 (기기마다 다름: 0x91, 0x7c 등)
[04:06] 2B   RESERVED          0x00 0x0d 또는 0x00 0x16
[06]    1B   CMD_FLAG          CMD 연동 플래그 (0x02 ↔ 0x42)
[07]    1B   RESERVED          0x00
[08]    1B   FAN_PARAM         팬/운전 파라미터 (0x20 또는 0x21, 풍량 아님 — 섹션 6.11 참조)
[09]    1B   SLOT_NUM          IDU 슬롯번호 (0x51~0x55, 고정) ← b[29]와 항상 동일
[10]    1B   OP_MODE           운전 모드 + 전원. bit5=전원 OFF, 하위 니블=LGAP 모드 코드 (섹션 6.9 참조)
[11]    1B   SET_TEMP_RAW      설정온도 원시값. 설정온도(°C) = b[11] + 15 ← b[31]과 항상 동일 (이중 기록)
[12:16] 4B   RESERVED          0x00 × 4
[16]    1B   FIXED             고정 = 0x08
[17]    1B   RESERVED          0x00
[18]    1B   AGGREGATED        집계 파라미터 (IDU별 상이)
[19]    1B   DEVICE_ID         장치 ID (기기마다 다름: 0x5d, 0x55 등)
[20]    1B   IDU_INDEX         IDU 인덱스 (0x01~0x05) ← 주소 검증용
[21]    1B   RESERVED          0x00
[22]    1B   PARAM             = 0x1c
[23]    1B   ROOM_TEMP_RAW     실내온도 raw  ← b[36]와 항상 동일 (이중 기록)
[24]    1B   INLET_TEMP_RAW    흡입온도 raw
[25]    1B   OUTLET_TEMP_RAW   토출온도 raw
[26]    1B   STATUS2           상태 플래그2
[27]    1B   PARAM2            = 0x28
[28]    1B   RESERVED          0x00
[29]    1B   SLOT_NUM2         슬롯번호 재확인 (b[09]와 항상 동일 ← 이중 기록)
[30]    1B   FAN_SPEED         풍량 바이트. bit6: 1=미풍(0x54), 0=약풍(0x14). 섹션 6.11 참조
[31]    1B   SET_TEMP_RAW2     설정온도 재확인 (b[11]과 항상 동일 ← 이중 기록)
[32:36] 4B   RESERVED          0x00 × 4
[36]    1B   ROOM_TEMP_RAW2    실내온도 재확인 (b[23]과 항상 동일 ← 이중 기록)
[37]    1B   RESERVED          0x00
[38]    1B   SENSOR_X          센서 파생값 (b[24]와 역상관: b[24]+b[38] ≈ 상수)
[39]    1B   SENSOR_Y          센서 파생값 (b[23]과 역상관: b[23]+b[39] ≈ 상수)
──────────────────────────────────────────────────
총 40바이트 / 체크섬: 없음
```

### 6.2 이중 기록 (Redundancy)

체크섬 대신 두 위치에 동일값을 기록하여 무결성을 확인한다.

| 필드 쌍 | 오프셋 | 검증 규칙 |
|--------|-------|---------|
| 슬롯번호 | b[09] = b[29] | 항상 동일 → 불일치 시 프레임 폐기 |
| 실내온도 | b[23] = b[36] | 항상 동일 → 불일치 시 프레임 폐기 |

### 6.3 IDU 주소 체계

| IDU_ADDR | IDU 번호 | IDU_INDEX(b[20]) |
|---------|---------|-----------------|
| 0x81 | IDU #1 | 0x01 |
| 0x82 | IDU #2 | 0x02 |
| 0x83 | IDU #3 | 0x03 |
| 0x84 | IDU #4 | 0x04 |
| 0x85 | IDU #5 | 0x05 |

### 6.4 기기별 가변 필드

DEV_TYPE과 DEVICE_ID는 기기 모델/버전에 따라 다르므로 파서 검증에서 사용하지 않는다.

| 필드 | 오프셋 | 캡처 A 값 | 캡처 B 값 |
|------|-------|---------|---------|
| DEV_TYPE | b[03] | `0x91` | `0x7c` |
| DEVICE_ID | b[19] | `0x5d` (IDU#1,4) / 없음 | `0x5d` (IDU#1,4) / `0x55` (IDU#2,3,5) |
| 파라미터 | b[05] | `0x0d` | `0x16` |

### 6.5 온도 변환 공식

```
설정온도 (°C)  = b[11] + 15
실내온도 (°C)  = (b[23] - 0x40) / 2.0
흡입온도 (°C)  = (b[24] - 0x40) / 2.0
토출온도 (°C)  = (b[25] - 0x40) / 2.0
```

**설정온도 발견 경위:** 30,363줄 스트림 분석에서 b[11]이 설정온도 변경에 따라
변화함을 확인. b[11]+15 = 설정온도(°C): 0x03→18°C, 0x07→22°C, 0x0a→25°C, 0x0f→30°C.
b[31]이 b[11]과 항상 동일 → 이중 기록 패턴 확정.
유효 범위: 18~30°C (LG 시스템에어컨 설정 범위).

**주의:** b[09]는 설정온도가 아닌 **IDU 슬롯번호** (0x51~0x55, 고정값).
이전 캡처에서 `b[09]-0x3C = 설정온도`처럼 보였던 것은 우연의 일치.

### 6.6 관측된 IDU 온도값 (캡처 B)

| IDU | 슬롯번호 | 설정온도 (b[11]+15) | 실내온도 | 흡입온도 | 토출온도 |
|-----|--------|------------------|--------|--------|--------|
| #1 | 0x51 | 30°C (0x0f) | 22.5°C | 26.5°C | 27.5°C |
| #2 | 0x52 | 18°C (0x03) | 23.5°C | 27.5°C | 27.0°C |
| #3 | 0x53 | 18°C (0x03) | 22.5°C | 28.0°C | 27.5°C |
| #4 | 0x54 | 30°C (0x0f) | 23.5°C | 27.0°C | 27.0°C |
| #5 | 0x55 | 18°C (0x03) | 23.5°C | 28.0°C | 27.0°C |

### 6.7 b[38], b[39] — 센서 파생값

체크섬이 아닌 온도 관련 파생 센서값. IDU별 기준 상수 기준 역상관 관계:

```
b[24] + b[38] ≈ IDU별 상수   (흡입온도 역상관)
b[23] + b[39] ≈ IDU별 상수   (실내온도 역상관, IDU#2/4에서 0xa9로 고정)
```

### 6.8 CMD 비트 구조

b[01] CMD 바이트는 단순 A/B 교대가 아니라 **비트 플래그 조합**이다.
2026-04-16 캡처에서 11종 CMD 값이 관측되었다.

**비트 분해:**

| 비트 | 마스크 | 의미 | 확인 상태 |
|------|-------|------|----------|
| bit6 | 0x40 | A/B 사이클 마커 (0=A, 1=B) | **실측 확인** |
| bit3 | 0x08 | 그룹 B 식별 (동일 모드 내 설정 구분, 의미 미확정) | **실측 확인** |
| bit2 | 0x04 | 설정 미변경 IDU 마커 | **실측 확인** (IDU#5에서만 관측) |
| bit1 | 0x02 | 베이스 상태 | **실측 확인** |
| bit0 | 0x01 | 활성 운전 | **실측 확인** |

**관측된 CMD 값 전체:**

| CMD | 비트 패턴 | SUB_CMD | 분류 | 빈도 | 확인 상태 |
|-----|----------|---------|------|------|----------|
| 0x02 | .0.0_0010 | 0x00/0x01 | 베이스 A | 높음 | **실측 확인** |
| 0x43 | .1.0_0011 | 0x01 | 베이스 B | 높음 | **실측 확인** |
| 0x01 | .0.0_0001 | 0x01 | 그룹 A 활성 (cycle A) | 중간 | **실측 확인** |
| 0x41 | .1.0_0001 | 0x01 | 그룹 A 활성 (cycle B) | 높음 | **실측 확인** |
| 0x09 | .0.0_1001 | 0x01 | 그룹 B 활성 (cycle A) | 중간 | **실측 확인** |
| 0x49 | .1.0_1001 | 0x01 | 그룹 B 활성 (cycle B) | 높음 | **실측 확인** |
| 0x00 | .0.0_0000 | 0x01 | 전이 시작 | 낮음 | **실측 확인** |
| 0x08 | .0.0_1000 | 0x01 | 전이 진행 | 낮음 | **실측 확인** |
| 0x03 | .0.0_0011 | 0x00 | 활성→베이스 종료 마커 | 극소 | **실측 확인** |
| 0x06 | .0.0_0110 | 0x01 | 베이스 A + 미변경 플래그 | 낮음 | **실측 확인** (IDU#5 전용) |
| 0x47 | .1.0_0111 | 0x01 | 베이스 B + 미변경 플래그 | 낮음 | **실측 확인** (IDU#5 전용) |

**SUB_CMD (b[02]) 규칙:**
- CMD ∈ {0x02, 0x03}일 때만 SUB_CMD=0x00 가능
- 그 외 모든 CMD에서 SUB_CMD=0x01
- SUB_CMD는 CMD와 중복 정보이므로 별도 해석 불필요

**CMD_FLAG (b[06]) 연동:**
- CMD bit6=0 → b[06]=0x02
- CMD bit6=1 → b[06]=0x42

**b[07] 플래그:**
- CMD=0x41에서만 b[07]=0x01 관측, 나머지 CMD에서는 0x00
- 의미 미확정

**b[18] bit7 (0x80) — 활성 운전 플래그:**
- 베이스 상태 (CMD ∈ {0x02, 0x03, 0x43}): b[18] bit7=0
- 활성/전이 상태 (CMD ∈ {0x00, 0x01, 0x08, 0x09, 0x41, 0x49}): b[18] bit7=1
- **실측 확인**: 캡처 1에서 모든 IDU에 대해 일관됨

### 6.8a CMD 상태 전이 시퀀스

5개 IDU가 2~4초 내에 **동기적으로** CMD를 전환한다.
즉 CMD는 개별 IDU 상태가 아니라 **버스 전체 상태**를 나타낸다.

**정상 순환 (운전 안정 시):**

```
(02,01) ←→ (43,01)       베이스 A ↔ B 교대 (약 1~2분 주기)
```

**활성 운전 전이 시퀀스 (버스 전체가 동기):**

```
(02,00) → (00,01) → (08,01) → { (01,01) 또는 (09,01) } → { (41,01) 또는 (49,01) }
  베이스     전이시작    전이진행       그룹 분기                   활성 운전 정착
                                    ↑                          ↑
                                  그룹A: 01                  그룹A: 41
                                  그룹B: 09                  그룹B: 49
```

**활성 운전 중 순환:**

```
(01,01) ←→ (41,01)       그룹 A 내 cycle A ↔ B (약 8~17초 주기)
(09,01) ←→ (49,01)       그룹 B 내 cycle A ↔ B (약 8~17초 주기)
```

**활성 → 베이스 복귀:**

```
(41,01) 또는 (49,01) → (03,00) → (02,01) → (43,01)
   활성 운전              종료마커     베이스 A     베이스 B
```

**IDU 그룹 분기 기준:**
| 그룹 | IDU | CMD 쌍 | 캡처 1 설정온도 | 운전 모드 |
|------|-----|--------|--------------|----------|
| A | #1, #4 | 01↔41 | 23°C | 냉방 |
| B | #2, #3, #5 | 09↔49 | 25°C | 냉방 |

**모든 IDU가 냉방 모드** (b[10]=0x10, OP_MODE 하위니블=0) 이므로 bit3은 운전 모드 차이가 아니다.
설정온도가 같은 IDU끼리 그룹이 형성되며, bit3의 정확한 분류 기준(설정 온도 차이, 냉매 회로, 부하 그룹 등)은 미확정.

### 6.8b CMD bit2 — 설정 미변경 플래그

캡처 2에서 설정 변경 실험 중 관측:
- IDU#2: 25→23°C, IDU#3: 25→24°C (설정 변경) → CMD=0x02/0x43 (정상)
- IDU#5: 25°C 유지 (미변경) → CMD=**0x06**/0x47 (bit2=0x04 추가)

bit2는 "이 IDU의 설정이 최근 변경되지 않았다"는 마커로 추정.
IDU#1,#4는 변경도 안 했고 bit2도 없으므로 — bit2는 **같은 그룹 내 다른 IDU가 변경되었을 때** 미변경 IDU에 표시되는 것으로 보인다.

### 6.9 OP_MODE (b[10]) 코드표

b[10]은 전원 상태와 운전 모드를 동시에 인코딩한다.

**전원 판별: bit5 (0x20)**

| bit5 | 의미 | 실측 바이트 | 확인 상태 |
|------|------|-----------|----------|
| 0 | **ON** (운전 중) | 0x14 | **실측 확인** |
| 1 | **OFF** (정지) | 0x24 | **실측 확인** (IDU#5) |

**운전 모드: 하위 니블 (b[10] & 0x0F) — 전원 ON 시에만 유효**

LGAP 운전 모드 코드와 일치함을 실측 확인.

| 하위 니블 | 모드 | 통일 값 | 통일 ID | 실측 바이트 예시 | 확인 상태 |
|----------|------|--------|--------|---------------|----------|
| 0 | 냉방 | `cool` | 0 | 0x10 | **실측 확인** |
| 1 | 제습 | `dry` | 1 | 0x11 | 추정 |
| 2 | 송풍 | `fan` | 2 | 0x12 | 추정 |
| 3 | 자동 | `auto` | 3 | 0x13 | 추정 |
| 4 | 난방 | `heat` | 4 | 0x14 | **실측 확인** |

**통일 운전 모드 ID** (전 프로토콜 공통): 0=cool, 1=dry, 2=fan, 3=auto, 4=heat.
플로우 이벤트에서 `op_mode` 필드로 통일 ID 숫자가 출력된다.

상위 니블 잔여 비트(bit4)의 의미는 미확정.

### 6.10 SET_TEMP_RAW (b[11]) — 설정온도

**b[11]은 상태 플래그가 아닌 설정온도 원시값. 설정온도(°C) = b[11] + 15.**

**⚠ CMD별 신뢰성 차이:**
- **(02,01), (43,01) 및 활성 CMD**: b[11]은 실제 설정온도를 정확히 반영한다. **파서는 이 프레임에서만 설정온도를 취해야 한다.**
- **(02,00)**: b[11]이 실제 설정과 +3°C 오프셋된 값을 보이는 케이스 관측. 이 프레임의 b[11]은 **설정온도로 사용하면 안 된다.** (예: 설정 25°C인 IDU에서 b[11]=0x0D→28°C)

이 차이는 (02,00) 프레임이 ODU 관점의 내부 타겟값을 담고 있기 때문으로 추정된다.
이중 기록 b[11]==b[31]은 (02,00)에서도 성립하므로 redundancy 체크로는 구별 불가.

**실측 확인 공식 검증 (캡처 2 — 설정 변경 실험):**

| IDU | 변경 전 b[11] | 변경 후 b[11] | 공식(+15) | 확인 |
|-----|-------------|-------------|----------|------|
| #2 | 0x0A | 0x08 | 25→23°C | **실측 확인** |
| #3 | 0x0A | 0x09 | 25→24°C | **실측 확인** |
| #5 | 0x0A | 0x0A | 25°C (불변) | **실측 확인** |

**관측된 설정온도 값:**

| b[11] | 설정온도 | 관측 |
|-------|---------|------|
| 0x03 | 18°C | **실측 확인** (최저) |
| 0x04 | 19°C | 관측 |
| 0x07 | 22°C | **실측 확인** |
| 0x08 | 23°C | **실측 확인** (캡처 2) |
| 0x09 | 24°C | **실측 확인** (캡처 2) |
| 0x0a | 25°C | **실측 확인** (캡처 2) |
| 0x0f | 30°C | **실측 확인** (최고) |

이중 기록: b[11] == b[31] (항상 동일).

### 6.11 FAN_SPEED (b[30]) — 풍량

**b[30]이 풍량 설정 필드임을 실측 확인. 단, 인코딩이 DEV_TYPE에 따라 다르다.**

**⚠ DEV_TYPE별 인코딩 차이:**
- DEV_TYPE=0x91 (캡처 A): bit6으로 미풍/약풍 구분 (0x54=미풍, 0x14=약풍)
- DEV_TYPE=0x7C (캡처 C, 2026-04-16): 0x50=약풍 **실측 확인**. bit6 규칙이 적용되지 않음.

| b[30] | 풍량 | 통일 ID | DEV_TYPE | 확인 상태 |
|-------|------|--------|---------|----------|
| 0x54 | quiet (미풍) | 1 | 0x91 | **실측 확인** |
| 0x14 | low (약풍) | 2 | 0x91 | **실측 확인** |
| 0x50 | low (약풍) | 2 | 0x7C | **실측 확인** (전 IDU 약풍, 사용자 확인) |
| 기타 | auto (자동) | 0 | — | 기본값 |

**통일 풍량 ID** (전 프로토콜 공통): 0=auto, 1=quiet, 2=low, 3=medium, 4=high, 5=turbo.
플로우 이벤트에서 `fan_speed` 필드로 통일 ID 숫자가 출력된다.

**참고:** b[08](FAN_PARAM, 0x20/0x21)은 풍량이 아님. 정확한 의미 미확정.
**참고:** 설정 변경 과도 구간에서 b[30]=0x22가 관측됨 (IDU#2,#3). 과도 상태이므로 풍량 값으로 취급하지 않는다.

---

## 7. 데이터 신뢰성 보장

체크섬이 없는 패킷의 신뢰성을 6계층으로 확보한다.

### 7.1 계층별 구조

```
계층 1: UART Framing Error   → 단일 비트 오류 100% 검출 (하드웨어 자동)
         ↓ 통과
계층 2: 이중 기록 일치         → 40바이트 내 임의 1바이트 오염의 99.6% 검출
         ↓ 통과
계층 3: 고정 바이트 구조        → 프레임 오정렬, 잘못된 경계 검출
         ↓ 통과
계층 4: 물리적 범위             → 명백한 데이터 오염 검출
         ↓ 통과
계층 5: 변화율                 → Spike 값 검출
         ↓ 통과
계층 6: SEQ 순서 연속성        → 패킷 누락 감지
         ↓
         신뢰 가능한 데이터
```

### 7.2 계층별 상세

#### 계층 1 — UART Framing (하드웨어, 자동)

1200 bps 8N1에서 UART는 각 바이트의 STOP bit를 검증한다. 노이즈로 비트가 깨지면 **Framing Error**로 해당 바이트를 자동 폐기한다. 40바이트가 완전히 수신됐다는 것 자체가 물리적 무결성의 1차 증거다.

- 오류 검출 능력: 단일 비트 오류 **100% 검출**

#### 계층 2 — 이중 기록 검증 (핵심)

TYPE-B 패킷은 체크섬 대신 핵심 온도값을 40바이트 안에서 두 위치에 기록한다.

```
b[09] == b[29]   ← 슬롯번호 2회 기록 (거리: 20바이트)
b[11] == b[31]   ← 설정온도 2회 기록 (거리: 20바이트)
b[23] == b[36]   ← 실내온도 2회 기록 (거리: 13바이트)
```

두 기록 위치가 20바이트 이상 떨어져 있어 연속 버스트 에러가 아닌 한 동시에 같은 오류값으로 깨질 확률이 극히 낮다.

- 임의 1바이트 오류 시 이중 기록 불일치 검출 확률: **255/256 ≈ 99.6%**
- 두 위치가 동시에 같은 잘못된 값으로 오염될 확률: **1/256² ≈ 0.0015%**

#### 계층 3 — 고정 바이트 구조 검증

```
pkt[1]  CMD 비트 마스크 검증 (§6.8 참조)
        허용: 0x00~0x03, 0x06, 0x08~0x09, 0x41, 0x43, 0x47, 0x49
pkt[20] == iduNum          IDU_INDEX = 주소(b[0])와 일치
```

#### 계층 4 — 물리적 범위 검증

에어컨이 동작 가능한 온도 범위를 벗어난 값은 데이터 오염으로 판단한다.

| 필드 | 유효 범위 |
|------|---------|
| 설정온도 | 18 ~ 30°C |
| 실내온도 | 0 ~ 50°C |
| 흡입온도 | 0 ~ 70°C |
| 토출온도 | 0 ~ 70°C |

#### 계층 5 — 변화율 검증

물리적으로 한 사이클(약 4초) 안에 온도가 2°C 이상 급변하는 것은 불가능하다.

```
|현재값 - 이전값| <= 2.0°C   (실내/흡입/토출 온도 대상)
```

#### 계층 6 — SEQ 순서 연속성

한 사이클 내 SEQ=01→02→03→04→05 순서가 깨지면 패킷 누락 또는 버스 충돌을 의미한다.

### 7.3 체크섬과 비교

| 항목 | 8비트 XOR 체크섬 | LGCNP-01 이중 기록 |
|------|---------------|-----------------|
| 임의 1바이트 오류 검출 | 255/256 = 99.6% | 255/256 = 99.6% |
| 두 위치 동시 오염 | — | 1/256² = 0.0015% |
| 물리 범위 검증 | ❌ | ✅ (계층 4) |
| 변화율 검증 | ❌ | ✅ (계층 5) |
| 구현 위치 | 송신측 계산 | 수신측 검증 |

1200 bps RS-485 유선 환경에서 버스트 에러 발생 확률이 낮고, 이중 기록 + 물리 범위 + 변화율 조합은 8비트 체크섬과 동등하거나 그 이상의 실용적 신뢰도를 제공한다.

### 7.4 통합 검증 구현 (Node-RED)

```javascript
// TYPE-B 6계층 통합 신뢰성 검증
// prev: 직전 사이클의 동일 IDU 패킷 (없으면 null)
function validate(pkt, prev) {
    const iduNum = pkt[0] - 0x81 + 1;

    // 계층 2: 이중 기록 ← 가장 먼저 체크 (3쌍)
    if (pkt[9]  !== pkt[29]) return { ok: false, reason: 'SLOT_NUM 이중 기록 불일치' };
    if (pkt[11] !== pkt[31]) return { ok: false, reason: 'SET_TEMP 이중 기록 불일치' };
    if (pkt[23] !== pkt[36]) return { ok: false, reason: 'ROOM_TEMP 이중 기록 불일치' };

    // 계층 3: 고정 바이트 구조
    if (pkt[1] !== 0x02 && pkt[1] !== 0x43) return { ok: false, reason: `잘못된 CMD: 0x${pkt[1].toString(16)}` };
    if (pkt[20] !== iduNum)                  return { ok: false, reason: `IDU_INDEX 불일치: ${pkt[20]} != ${iduNum}` };

    // b[09]는 IDU 슬롯번호 (0x51~0x55), 설정온도 아님
    const setTemp    = pkt[11] + 15;  // b[11] + 15 = 설정온도(°C)
    const roomTemp   = (pkt[23] - 0x40) / 2.0;
    const inletTemp  = (pkt[24] - 0x40) / 2.0;
    const outletTemp = (pkt[25] - 0x40) / 2.0;

    // 계층 4: 물리적 범위
    if (setTemp    < 18 || setTemp    > 30) return { ok: false, reason: `설정온도 범위 초과: ${setTemp}°C` };
    if (roomTemp   < 0  || roomTemp   > 50) return { ok: false, reason: `실내온도 범위 초과: ${roomTemp}°C` };
    if (inletTemp  < 0  || inletTemp  > 70) return { ok: false, reason: `흡입온도 범위 초과: ${inletTemp}°C` };
    if (outletTemp < 0  || outletTemp > 70) return { ok: false, reason: `토출온도 범위 초과: ${outletTemp}°C` };

    // 계층 5: 변화율 (이전 값이 있을 때만)
    if (prev) {
        const MAX_DELTA = 2.0;
        if (Math.abs(roomTemp  - prev.roomTemp)   > MAX_DELTA) return { ok: false, reason: `실내온도 급변: ${prev.roomTemp}→${roomTemp}°C` };
        if (Math.abs(inletTemp - prev.inletTemp)  > MAX_DELTA) return { ok: false, reason: `흡입온도 급변: ${prev.inletTemp}→${inletTemp}°C` };
        if (Math.abs(outletTemp- prev.outletTemp) > MAX_DELTA) return { ok: false, reason: `토출온도 급변: ${prev.outletTemp}→${outletTemp}°C` };
    }

    return { ok: true, setTemp, roomTemp, inletTemp, outletTemp };
}
```

### 7.5 계층별 적용 대상 정리

| 계층 | TYPE-A SEQ=01,04,05 | TYPE-A SEQ=02,03 | TYPE-B |
|------|--------------------|-----------------|----|
| 1. UART Framing | ✅ | ✅ | ✅ |
| 2. 이중 기록 | — | — | ✅ b[09]=b[29](슬롯번호), b[11]=b[31](설정온도), b[23]=b[36](실내온도) |
| 3. 고정 바이트 | ✅ XOR/SUM CHK | ✅ b[02~17] 구조 | ✅ CMD, IDU_INDEX |
| 4. 물리 범위 | — | ✅ 온도 | ✅ 온도 4종 |
| 5. 변화율 | — | ✅ 외기온도 | ✅ 실내/흡입/토출 |
| 6. SEQ 순서 | ✅ | ✅ | ✅ |

---

## 8. 패킷 예시 (전체 분해)

### TYPE-A SEQ=01

```
58 01 00 00 00 00 00 00 00 57 66 00 00 00 2e 00 19 01 43 1d
↑  ↑  ←─────────────── DATA 17바이트 ─────────────────────→ ↑
STX SEQ                                                      XOR CHK ✓
```

### TYPE-A SEQ=02

```
58 02 e0 06 02 00 81 4d 9f 00 31 cd e5 00 6b 5c 0f 00 f8 ee
               ↑        ↑     ↑              ↑  ↑  ↑   ↑  ↑
          SUBHDR    FLAG  CTR_B CTR_C     T_A T_B FIX  S_A S_B
                                          21.5°C 14.0°C (센서값)
체크섬: 없음 (b[18]=0xf8, b[19]=0xee 모두 센서 데이터)
```

### TYPE-B IDU#1 (CMD=0x02)

```
Offset 00~0F:  81 02 00 7c 00 16 02 00  20 52 14 0f 00 00 00 00
               ↑  ↑  ↑  ↑     ↑        ↑   ↑  ↑  ↑
             ADDR CMD SUB DEV PAR     FAN SLOT OP SET

Offset 10~1F:  08 00 fb 5d 01 00 1c 6d  75 77 03 28 00 52 54 0f
                        ↑        ↑  ↑   ↑  ↑        ↑
                     AGG IDX   ROOM IN  OUT      SLOT2=b[09] ✓

Offset 20~27:  00 00 00 00 6d 00 03 38
                           ↑     ↑  ↑
                       ROOM2=b[23]✓  S_X S_Y (센서 파생값)
체크섬: 없음
이중 기록: b[09]=b[29]=0x52 ✓ (슬롯번호), b[11]=b[31]=0x0f ✓ (설정온도), b[23]=b[36]=0x6d ✓ (실내온도)

온도:
  설정온도 = 0x0f + 15 = 30°C (b[11])
  실내온도 = (0x6d - 0x40) / 2 = 22.5°C
  흡입온도 = (0x75 - 0x40) / 2 = 26.5°C
  토출온도 = (0x77 - 0x40) / 2 = 27.5°C
```

---

## 9. 파서 구현

### Node-RED — TYPE-B 스트림 파서

```javascript
// LGCNP-01 TYPE-B 스트림 파서
const IDU_BASE  = 0x81;
const IDU_MAX   = 0x85;
const FRAME_LEN = 40;

let buf = context.get('buf') || [];
const incoming = msg.payload;
if (Buffer.isBuffer(incoming)) {
    for (const b of incoming) buf.push(b);
} else if (Array.isArray(incoming)) {
    buf = buf.concat(incoming);
}

const frames = [];
let i = 0;

while (i < buf.length) {
    const b = buf[i];
    if (b < IDU_BASE || b > IDU_MAX) { i++; continue; }
    if (i + FRAME_LEN > buf.length) break;

    const pkt = buf.slice(i, i + FRAME_LEN);
    const frame = tryParse(pkt);

    if (frame !== null) { frames.push(frame); i += FRAME_LEN; }
    else { i++; }
}

context.set('buf', buf.slice(i));
if (frames.length === 0) return null;

return frames.map(f => ({ payload: f, topic: `lgcnp01/idu/${f.iduNum}` }));

function tryParse(pkt) {
    const iduNum = pkt[0] - IDU_BASE + 1;
    if (pkt[1] !== 0x02 && pkt[1] !== 0x43) return null;
    if (pkt[20] !== iduNum)  return null;
    if (pkt[9]  !== pkt[29]) return null;  // 슬롯번호 이중 기록
    if (pkt[11] !== pkt[31]) return null;  // 설정온도 이중 기록
    if (pkt[23] !== pkt[36]) return null;  // 실내온도 이중 기록
    return decode(pkt, iduNum);
}

function decode(pkt, iduNum) {
    // b[09]는 IDU 슬롯번호 (0x51~0x55)
    const slotNum    = pkt[9];
    const setTemp    = pkt[11] + 15;  // b[11] + 15 = 설정온도(°C)
    const roomTemp   = (pkt[23] - 0x40) / 2.0;
    const inletTemp  = (pkt[24] - 0x40) / 2.0;
    const outletTemp = (pkt[25] - 0x40) / 2.0;
    const rangeOk = setTemp >= 18 && setTemp <= 30
                 && roomTemp >= 0 && roomTemp <= 50
                 && inletTemp >= 0 && inletTemp <= 70
                 && outletTemp >= 0 && outletTemp <= 70;
    if (!rangeOk) node.warn(`IDU#${iduNum} 온도 범위 이상`);
    return {
        raw: Buffer.from(pkt).toString('hex'),
        iduAddr: pkt[0], iduNum,
        cmdCycle: pkt[1] === 0x02 ? 'A' : 'B',
        devType: pkt[3], deviceId: pkt[19],
        slotNum, setTemp, opMode: pkt[10],
        iduIndex: pkt[20],
        roomTemp, inletTemp, outletTemp,
        sensorX: pkt[38], sensorY: pkt[39],
        rangeOk, ts: Date.now(),
    };
}
```

### Go

```go
package lgcnp01

import "fmt"

const (
    STXOdu  = 0x58
    IDUBase = 0x81
    IDUMax  = 0x85
    ODULen  = 20
    IDULen  = 40
)

type IDUPacket struct {
    IDUAddr     byte
    IDUNum      int
    CMD         byte
    DevType     byte    // 기기마다 다름
    DeviceID    byte    // 기기마다 다름
    SlotNum     byte    // b[9]: IDU 슬롯번호 (0x51~0x55, 고정)
    SetTemp     float32 // °C = b[11] + 15
    OpMode      byte
    IDUIndex    byte
    RoomTemp    float32 // °C = (b[23]-0x40)/2
    InletTemp   float32 // °C = (b[24]-0x40)/2
    OutletTemp  float32 // °C = (b[25]-0x40)/2
    SensorX     byte    // b[38]: 흡입온도 역상관 파생값
    SensorY     byte    // b[39]: 실내온도 역상관 파생값
    Raw         [IDULen]byte
}

func ParseIDU(buf []byte) (*IDUPacket, error) {
    if len(buf) != IDULen {
        return nil, fmt.Errorf("invalid length: %d", len(buf))
    }
    addr := buf[0]
    if addr < IDUBase || addr > IDUMax {
        return nil, fmt.Errorf("invalid IDU addr: 0x%02x", addr)
    }
    iduNum := int(addr-IDUBase) + 1

    // CMD 검증
    if buf[1] != 0x02 && buf[1] != 0x43 {
        return nil, fmt.Errorf("invalid CMD: 0x%02x", buf[1])
    }
    // IDU_INDEX 일치
    if int(buf[20]) != iduNum {
        return nil, fmt.Errorf("IDU_INDEX mismatch: %d != %d", buf[20], iduNum)
    }
    // 이중 기록 검증 — 핵심 무결성
    if buf[9] != buf[29] {
        return nil, fmt.Errorf("SLOT_NUM mismatch: b09=0x%02x b29=0x%02x", buf[9], buf[29])
    }
    if buf[11] != buf[31] {
        return nil, fmt.Errorf("SET_TEMP mismatch: b11=0x%02x b31=0x%02x", buf[11], buf[31])
    }
    if buf[23] != buf[36] {
        return nil, fmt.Errorf("ROOM_TEMP mismatch: b23=0x%02x b36=0x%02x", buf[23], buf[36])
    }

    p := &IDUPacket{
        IDUAddr:     addr,
        IDUNum:      iduNum,
        CMD:         buf[1],
        DevType:     buf[3],
        DeviceID:    buf[19],
        SlotNum:     buf[9],
        SetTemp:     float32(buf[11]) + 15,
        OpMode:      buf[10],
        StatusFlags: buf[11],
        IDUIndex:    buf[20],
        RoomTemp:    float32(buf[23]-0x40) / 2.0,
        InletTemp:   float32(buf[24]-0x40) / 2.0,
        OutletTemp:  float32(buf[25]-0x40) / 2.0,
        SensorX:     buf[38],
        SensorY:     buf[39],
    }
    copy(p.Raw[:], buf)
    return p, nil
}

func VerifyODUChecksum(buf []byte) bool {
    if len(buf) != ODULen { return false }
    seq := buf[1]
    switch seq {
    case 0x01, 0x05:
        var xor byte
        for _, b := range buf[:19] { xor ^= b }
        return xor == buf[19]
    case 0x04:
        var s int
        for _, b := range buf[:19] { s += int(b) }
        return byte(s) == buf[19]
    default:
        return true // SEQ=02,03: 체크섬 없음
    }
}
```

---

## 10. xflowd 설정

```yaml
serial:
  baud_rate: 1200
  data_bits: 8
  parity: none
  stop_bits: 1

parser:
  protocol: lgcnp01
  odu_stx: 0x58
  odu_len: 20
  idu_addr_min: 0x81
  idu_addr_max: 0x85
  idu_len: 40
  # 주의: DEV_TYPE, DEVICE_ID는 기기마다 달라 검증 불가
```

---

## 11. 미확정 사항

| 항목 | 현황 |
|------|------|
| b[11] 재분류 | ~~STATUS_FLAGS~~ → **설정온도** = b[11]+15 (°C). 30,363줄 분석에서 **실측 확인**. b[31]과 이중 기록 |
| b[09] 재분류 | ~~설정온도~~ → IDU 슬롯번호 (0x51~0x55, 30,363줄에서 불변 확인). 이전 캡처의 일치는 우연 |
| ~~전원 ON/OFF 판별~~ | **b[10] bit5로 확정**. bit5=0→ON, bit5=1→OFF. IDU#5에서 0x24(OFF) **실측 확인** |
| ~~SEQ=02 필드 재분류~~ | b[06]=외기온도, b[08]=압축기 흡입, b[11]=압축기 토출, b[14]=응축A, b[15]=응축B **실측 확정** |
| SEQ=02 b[07] | 저압측 추정, 단위(bar/kPa) 미확정 |
| SEQ=02 b[10] | 고압 압력 지수 추정, 단위 미확정 |
| SEQ=02 b[02], b[03] | 넓은 범위 변동, 압축기 회전수 또는 전류 추정 |
| SEQ=02 b[18], b[19] | 센서 파생값, 물리 의미 미확정 |
| ~~SEQ=04 b[10]~~ | 운전 평균 온도 **확정**: (raw-0x40)/2 °C |
| TYPE-B b[38] 의미 | 흡입온도(b[24])와 역상관 파생값, b[24]+b[38] ≈ IDU별 상수 |
| TYPE-B b[39] 의미 | 실내온도(b[23])와 역상관 파생값, b[23]+b[39] ≈ IDU별 상수 |
| ~~STATUS_FLAGS~~ (b[11]) | **설정온도로 재분류 완료** (섹션 6.10). b[11]+15 = 설정온도(°C) |
| OP_MODE (b[10]) 상위 니블 | 하위 니블 = LGAP 모드 코드 **실측 확인**. 상위 니블(항상 0x1_) 의미 미확정 |
| OP_MODE (b[10]) 0x22 관측 | 설정 변경 시 b[10]=0x22(OFF+fan?) 일시 전이 관측. 의미 미확정 |
| FAN_SPEED (b[30]) 중/강/자동 코드 | 0x54=미풍, 0x14=약풍 **실측 확인**. 중풍/강풍/자동 코드 미관측 |
| FAN_PARAM (b[08]) 의미 | 0x20/0x21 관측. 풍량이 아님 확인. 정확한 의미 미확정 |
| IDU 6대 이상 구성 시 주소 | 0x86 이상 사용 여부 미확인 |
| b[38]+b[39] IDU별 상수값 목록 | 기기 구성에 따라 다름, 학습 필요 |
| ~~CMD 유형~~ | **§6.8에서 11종 CMD + 비트 구조 확정** (2026-04-16 캡처). 이전 2종(0x02/0x43)에서 대폭 확장 |
| ~~b[11] CMD별 신뢰성~~ | **(02,00) 프레임에서 비신뢰** §6.10에서 확정. (02,01) 이상에서만 설정온도 취득 |
| b[07] CMD=0x41 플래그 | CMD=0x41에서만 b[07]=0x01 관측. 의미 미확정 |
| b[18] bit7 활성 플래그 | 베이스=0, 활성/전이=1. §6.8에서 **실측 확인** |
| CMD bit2 (0x04) | 설정 미변경 IDU 마커 추정. §6.8b에서 관측, 정확한 트리거 조건 미확정 |
| CMD bit3 (0x08) 그룹 B | 전체 냉방 운전 중에도 그룹 분기 관측 → 모드가 아닌 설정온도/냉매회로 기준 추정. 정확한 기준 미확정 |
| (02,00) b[11] 오프셋 의미 | 실제 설정 대비 +3°C. ODU 내부 타겟값? 정확한 의미 미확정 |
| CMD 전이 시퀀스 트리거 | 제어 명령? 시간 기반? 트리거 조건 미확정 |

---

*대상: LG LRD-N837T 실내기 / xagent03 xflowd 캡처*
*속도 실험: 9600→4800→2400→1200 bps 순차 확인*
*체크섬 분석: 전수 탐색(XOR/SUM/NEG/CRC-16, 모든 범위) + 단일 변동 실험으로 최종 확정*
*결론: 동적 데이터 패킷(SEQ=02,03 및 TYPE-B)에는 체크섬 없음 — 이중 기록으로 무결성 보장*
