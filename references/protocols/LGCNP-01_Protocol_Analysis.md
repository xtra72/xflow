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

### 5.3 SEQ=02 — ODU 실시간 센서 데이터

```
Offset  크기  필드명           값 범위        설명
──────────────────────────────────────────────────────────
[00]    1B   STX              0x58           고정
[01]    1B   SEQ              0x02           고정
[02:06] 4B   SUBHEADER        e0 06 02 00    고정 서브헤더
[06]    1B   FLAG_A           0x80/0x81      1비트 플래그 (컴프레서 상태?)
[07]    1B   CTR_A            0x4b~0x4d      감소 카운터
[08]    1B   CTR_B            0x9b~0x9f      감소 카운터
[09]    1B   RESERVED         0x00           고정
[10]    1B   CTR_C            0x31~0x35      증가 카운터
[11]    1B   FLAG_B           0xcd/0xce      1비트 플래그
[12]    1B   FLAG_C           0xe5/0xe6      1비트 플래그
[13]    1B   RESERVED         0x00           고정
[14]    1B   OUTDOOR_TEMP_A   0x6b~0x6e      외기 온도 A
[15]    1B   OUTDOOR_TEMP_B   0x5c~0x63      외기 온도 B
[16]    1B   FIXED            0x0f           고정
[17]    1B   RESERVED         0x00           고정
[18]    1B   SENSOR_A         0xf8/0xf9/0xfe 센서값 (FLAG_A 연동)
[19]    1B   SENSOR_B         0x90~0xee      센서값 (OUTDOOR_TEMP 연동)
──────────────────────────────────────────────────────────
체크섬: 없음 — b[18], b[19] 모두 독립 센서 데이터
```

**온도 변환:**
```
OUTDOOR_TEMP_A (°C) = (b[14] - 0x40) / 2.0
OUTDOOR_TEMP_B (°C) = (b[15] - 0x40) / 2.0
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
                                                         ↑ SUM[0:19] & 0xFF ✓
```

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
[08]    1B   FAN_PARAM         풍량 (하위 3비트 = LGAP 풍량 코드, 섹션 6.11 참조)
[09]    1B   SLOT_NUM          IDU 슬롯번호 (0x51~0x55, 고정) ← b[29]와 항상 동일
[10]    1B   OP_MODE           운전 모드 (하위 니블 = LGAP 모드 코드, 아래 표 참조)
[11]    1B   STATUS_FLAGS      상태 플래그 (0이면 OFF, 0 이외이면 ON 추정)
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
[30]    1B   PARAM3            파라미터
[31]    1B   STATUS_FLAGS2     b[11]과 동일
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
실내온도 (°C)  = (b[23] - 0x40) / 2.0
흡입온도 (°C)  = (b[24] - 0x40) / 2.0
토출온도 (°C)  = (b[25] - 0x40) / 2.0
```

**주의:** b[09]는 설정온도가 아닌 **IDU 슬롯번호** (0x51~0x55, 고정값).
이전 캡처에서 `b[09]-0x3C = 설정온도`처럼 보였던 것은 기술자가
슬롯1→21°C, 슬롯2→22°C 방식으로 설정한 우연의 일치였음.
30,363줄 스트림 분석에서 b[09]가 단 한 번도 변하지 않음을 확인.

**설정온도:** STATUS 패킷(CMD=0x02/0x43)에는 없음.
COMMAND 패킷(CMD=0x01/0x03/0x41/0x49) 분석 필요.

### 6.6 관측된 IDU 온도값 (캡처 B)

| IDU | 슬롯번호 | 실내온도 | 흡입온도 | 토출온도 |
|-----|--------|--------|--------|--------|
| #1 | 0x51 | 22.5°C | 26.5°C | 27.5°C |
| #2 | 0x52 | 23.5°C | 27.5°C | 27.0°C |
| #3 | 0x53 | 22.5°C | 28.0°C | 27.5°C |
| #4 | 0x54 | 23.5°C | 27.0°C | 27.0°C |
| #5 | 0x55 | 23.5°C | 28.0°C | 27.0°C |

### 6.7 b[38], b[39] — 센서 파생값

체크섬이 아닌 온도 관련 파생 센서값. IDU별 기준 상수 기준 역상관 관계:

```
b[24] + b[38] ≈ IDU별 상수   (흡입온도 역상관)
b[23] + b[39] ≈ IDU별 상수   (실내온도 역상관, IDU#2/4에서 0xa9로 고정)
```

### 6.8 CMD 사이클 패턴

| 필드 | 사이클 A | 사이클 B |
|------|--------|--------|
| b[01] CMD | `0x02` | `0x43` |
| b[02] SUB_CMD | `0x00` | `0x01` |
| b[06] CMD_FLAG | `0x02` | `0x42` |

약 5~6초 주기로 교대.

### 6.9 OP_MODE (b[10]) 코드표

**하위 니블(b[10] & 0x0F)이 LGAP 운전 모드 코드와 일치함을 실측 확인.**

| 하위 니블 | 모드 | 실측 바이트 예시 | 확인 상태 |
|----------|------|---------------|----------|
| 0 | cooling (냉방) | 0x10 | 추정 |
| 1 | dehumidify (제습) | 0x11 | 추정 |
| 2 | fan (송풍) | 0x12 | 추정 |
| 3 | auto (자동) | 0x13 | 추정 |
| 4 | heating (난방) | 0x14 | **실측 확인** |

상위 니블(0x10)의 의미는 미확정. 캡처 데이터에서 항상 0x1_로 관측됨.

### 6.10 STATUS_FLAGS (b[11]) 해석

| 값 | 관측 | 추정 의미 |
|----|------|----------|
| 0x00 | 미관측 | OFF (전원 꺼짐) |
| 0x03 | 관측 | ON (일반 운전) |
| 0x04 | 관측 | ON (운전 상태 변이) |
| 0x07 | 관측 | ON (운전 상태 변이) |
| 0x0F | 관측 | ON (전체 플래그 활성) |

**전원 판별**: STATUS_FLAGS != 0이면 ON으로 추정. 비트별 정확한 의미는 미확정.

### 6.11 FAN_PARAM (b[08]) 코드표

**하위 3비트(b[08] & 0x07)가 LGAP 풍량 코드와 일치하는 것으로 추정.**

| 하위 3비트 | 풍량 | 관측 바이트 예시 | 확인 상태 |
|-----------|------|---------------|----------|
| 0 | low (약) 또는 기본값 | 0x20 | 관측 |
| 1 | low (약) | 0x21 | 관측 |
| 2 | medium (중) | 0x22 | 추정 |
| 3 | high (강) | 0x23 | 추정 |
| 4 | auto (자동) | 0x24 | 추정 |
| 5 | quiet (미풍) | 0x25 | 추정 |

상위 비트(0x20)의 의미는 미확정. 캡처 데이터에서 항상 0x2_로 관측됨.

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
b[23] == b[36]   ← 실내온도 2회 기록 (거리: 13바이트)
```

두 기록 위치가 20바이트 이상 떨어져 있어 연속 버스트 에러가 아닌 한 동시에 같은 오류값으로 깨질 확률이 극히 낮다.

- 임의 1바이트 오류 시 이중 기록 불일치 검출 확률: **255/256 ≈ 99.6%**
- 두 위치가 동시에 같은 잘못된 값으로 오염될 확률: **1/256² ≈ 0.0015%**

#### 계층 3 — 고정 바이트 구조 검증

```
pkt[1]  in {0x02, 0x43}   CMD 유효 범위
pkt[20] == iduNum          IDU_INDEX = 주소(b[0])와 일치
```

#### 계층 4 — 물리적 범위 검증

에어컨이 동작 가능한 온도 범위를 벗어난 값은 데이터 오염으로 판단한다.

| 필드 | 유효 범위 |
|------|---------|
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

    // 계층 2: 이중 기록 ← 가장 먼저 체크
    if (pkt[9]  !== pkt[29]) return { ok: false, reason: 'SLOT_NUM 이중 기록 불일치' };
    if (pkt[23] !== pkt[36]) return { ok: false, reason: 'ROOM_TEMP 이중 기록 불일치' };

    // 계층 3: 고정 바이트 구조
    if (pkt[1] !== 0x02 && pkt[1] !== 0x43) return { ok: false, reason: `잘못된 CMD: 0x${pkt[1].toString(16)}` };
    if (pkt[20] !== iduNum)                  return { ok: false, reason: `IDU_INDEX 불일치: ${pkt[20]} != ${iduNum}` };

    // b[09]는 IDU 슬롯번호 (0x51~0x55), 설정온도 아님
    const roomTemp   = (pkt[23] - 0x40) / 2.0;
    const inletTemp  = (pkt[24] - 0x40) / 2.0;
    const outletTemp = (pkt[25] - 0x40) / 2.0;

    // 계층 4: 물리적 범위
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

    return { ok: true, roomTemp, inletTemp, outletTemp };
}
```

### 7.5 계층별 적용 대상 정리

| 계층 | TYPE-A SEQ=01,04,05 | TYPE-A SEQ=02,03 | TYPE-B |
|------|--------------------|-----------------|----|
| 1. UART Framing | ✅ | ✅ | ✅ |
| 2. 이중 기록 | — | — | ✅ b[09]=b[29](슬롯번호), b[23]=b[36](실내온도) |
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
             ADDR CMD SUB DEV PAR     FAN SLOT OP STS

Offset 10~1F:  08 00 fb 5d 01 00 1c 6d  75 77 03 28 00 52 54 0f
                        ↑        ↑  ↑   ↑  ↑        ↑
                     AGG IDX   ROOM IN  OUT      SLOT2=b[09] ✓

Offset 20~27:  00 00 00 00 6d 00 03 38
                           ↑     ↑  ↑
                       ROOM2=b[23]✓  S_X S_Y (센서 파생값)
체크섬: 없음
이중 기록: b[09]=b[29]=0x52 ✓ (슬롯번호), b[23]=b[36]=0x6d ✓ (실내온도)

온도:
  실내온도 = (0x6d - 0x40) / 2 = 22.5°C
  흡입온도 = (0x75 - 0x40) / 2 = 26.5°C
  토출온도 = (0x77 - 0x40) / 2 = 27.5°C
  b[09]=0x52: IDU 슬롯번호 (설정온도 아님)
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
    if (pkt[23] !== pkt[36]) return null;  // 실내온도 이중 기록
    return decode(pkt, iduNum);
}

function decode(pkt, iduNum) {
    // b[09]는 IDU 슬롯번호 (0x51~0x55), 설정온도 아님
    const slotNum    = pkt[9];
    const roomTemp   = (pkt[23] - 0x40) / 2.0;
    const inletTemp  = (pkt[24] - 0x40) / 2.0;
    const outletTemp = (pkt[25] - 0x40) / 2.0;
    const rangeOk = roomTemp >= 0 && roomTemp <= 50
                 && inletTemp >= 0 && inletTemp <= 70
                 && outletTemp >= 0 && outletTemp <= 70;
    if (!rangeOk) node.warn(`IDU#${iduNum} 온도 범위 이상`);
    return {
        raw: Buffer.from(pkt).toString('hex'),
        iduAddr: pkt[0], iduNum,
        cmdCycle: pkt[1] === 0x02 ? 'A' : 'B',
        devType: pkt[3], deviceId: pkt[19],
        slotNum, opMode: pkt[10], statusFlags: pkt[11],
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
    OpMode      byte
    StatusFlags byte
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
| **설정온도 필드 위치** | **STATUS 패킷(CMD=0x02/0x43)에 설정온도 없음. COMMAND 패킷(CMD=0x01/0x03/0x41/0x49) 분석 필요. 리모컨 온도 변경 시점 캡처 필요** |
| b[09] 재분류 | ~~설정온도~~ → IDU 슬롯번호 (0x51~0x55, 30,363줄에서 불변 확인). 이전 캡처의 일치는 우연 |
| TYPE-A SEQ=02/03 b[18], b[19] 정확한 의미 | 각각 특정 플래그/온도 필드와 역상관, 물리 의미 미확정 |
| TYPE-B b[38] 의미 | 흡입온도(b[24])와 역상관 파생값, b[24]+b[38] ≈ IDU별 상수 |
| TYPE-B b[39] 의미 | 실내온도(b[23])와 역상관 파생값, b[23]+b[39] ≈ IDU별 상수 |
| STATUS_FLAGS (b[11]) 비트별 의미 | 0x03/0x07/0x0f/0x04 관측. != 0이면 ON 추정 확인. 개별 비트 의미 미확정 |
| OP_MODE (b[10]) 상위 니블 | 하위 니블 = LGAP 모드 코드 **실측 확인**. 상위 니블(항상 0x1_) 의미 미확정 |
| FAN_PARAM (b[08]) 상위 비트 | 하위 3비트 = LGAP 풍량 코드 추정. 상위 비트(항상 0x2_) 의미 미확정. 실측 추가 확인 필요 |
| IDU 6대 이상 구성 시 주소 | 0x86 이상 사용 여부 미확인 |
| b[38]+b[39] IDU별 상수값 목록 | 기기 구성에 따라 다름, 학습 필요 |

---

*대상: LG LRD-N837T 실내기 / xagent03 xflowd 캡처*
*속도 실험: 9600→4800→2400→1200 bps 순차 확인*
*체크섬 분석: 전수 탐색(XOR/SUM/NEG/CRC-16, 모든 범위) + 단일 변동 실험으로 최종 확정*
*결론: 동적 데이터 패킷(SEQ=02,03 및 TYPE-B)에는 체크섬 없음 — 이중 기록으로 무결성 보장*
