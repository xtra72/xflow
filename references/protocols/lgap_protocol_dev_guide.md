# LGAP 프로토콜 개발 가이드

> LG 시스템 에어컨 PI-485 통신 프로토콜 (LG Airconditioner Protocol)
>
> 참조: [jourdant/esphome-lgap](https://github.com/jourdant/esphome-lgap/blob/main/protocol.md)

---

## 1. 프로토콜 개요

LGAP(LG Airconditioner Protocol)는 LG 시스템 에어컨 실외기(ODU)와 BMS(Building Management System) 게이트웨이 간 통신에 사용되는 LG 독자 프로토콜이다. 공식 게이트웨이로는 Modbus RTU, LonWorks, BACnet 등을 지원하는 제품이 있으며, PI-485 확장 보드를 통해 RS-485 인터페이스로 접근한다.

### 1.1 통신 사양

| 항목 | 사양 |
|------|------|
| 물리 인터페이스 | RS-485 (2선식) |
| 통신 속도 | 4,800 bps |
| 데이터 비트 | 8 |
| 패리티 | None |
| 정지 비트 | 1 |
| 연결 핀 | CENA/CENB (중앙 제어) 또는 PI-485 출력 핀 |
| 온도 단위 | 섭씨(°C) 전용 |

### 1.2 통신 방식

- **마스터/슬레이브**: 컨트롤러(마스터)가 요청, 실외기(슬레이브)가 응답
- **폴링 방식**: 실외기는 자발적으로 상태를 전송하지 않음 → 주기적 폴링 필수
- **요청/응답 크기**: 요청 8바이트, 응답 16바이트 (고정)
- **잘못된 요청 시**: 응답 0바이트 (타임아웃)

---

## 2. 체크섬 계산

모든 요청/응답 패킷의 마지막 바이트는 체크섬이다. LG 벽걸이 컨트롤러 프로토콜과 동일한 방식을 사용한다.

### 2.1 계산 공식

```
checksum = (모든 비체크섬 바이트의 합 % 256) XOR 0x55
```

### 2.2 계산 단계

1. 체크섬을 제외한 모든 바이트를 합산
2. 256으로 나머지 연산 (modulo 256) — `byte` 타입 사용 시 자동 오버플로우로 생략 가능
3. 결과값을 `0x55` (85)와 XOR 연산

### 2.3 계산 예시

**요청 패킷 예시:**

```
데이터:  0x00  0x00  0xA0  0x00  0x00  0x00  0x08
합산:    0 + 0 + 160 + 0 + 0 + 0 + 8 = 168
체크섬:  (168 % 256) XOR 0x55 = 168 XOR 85 = 253 (0xFD)
패킷:    00 00 A0 00 00 00 08 FD
```

**응답 패킷 예시:**

```
데이터:  0x10  0x02  0xA0  0x40  0x00  0x00  0x10  0x48  0x79  0x7F  0x7F  0x28  0x00  0x18  0x33
합산:    16+2+160+64+0+0+16+72+121+127+127+40+0+24+51 = 820
체크섬:  (820 % 256) XOR 0x55 = 52 XOR 85 = 97 (0x61)
패킷:    10 02 A0 40 00 00 10 48 79 7F 7F 28 00 18 33 61
```

### 2.4 C 구현 코드

```c
uint8_t lgap_calc_checksum(const uint8_t *data, uint8_t len) {
    uint8_t sum = 0;
    for (uint8_t i = 0; i < len; i++) {
        sum += data[i];
    }
    return sum ^ 0x55;
}

bool lgap_verify_checksum(const uint8_t *packet, uint8_t len) {
    uint8_t calc = lgap_calc_checksum(packet, len - 1);
    return (calc == packet[len - 1]);
}
```

---

## 3. 요청 패킷 (TX: 컨트롤러 → 실외기)

8바이트 고정 길이. 특정 존(Zone) 또는 실내기(IDU)의 상태를 조회하거나 제어 명령을 전송한다.

### 3.1 패킷 구조 요약

```
+------+------+------+------+------+------+------+------+
| TX0  | TX1  | TX2  | TX3  | TX4  | TX5  | TX6  | TX7  |
|Header| Cmd  | CmdID| Zone |Status| Mode | Temp | Chk  |
| 0x10 | 0x00 | 0xA0 |  N   | Flags|MdFnSw| Tgt  | XOR  |
+------+------+------+------+------+------+------+------+
```

### 3.2 바이트별 상세 정의

#### TX0 — 프레임 헤더 (Frame Header)

| 비트 | 설명 |
|------|------|
| `[7:0]` | 프레임 헤더/길이. 일반적으로 `0x10` (16) 사용 |

#### TX1 — 명령 타입 (Command Type)

| 비트 | 설명 |
|------|------|
| `[7:0]` | 명령 타입. 일반 제어 시 `0x00` |

#### TX2 — 명령 ID (Command ID)

| 비트 | 설명 |
|------|------|
| `[7:0]` | 명령 식별자. 설정/제어 프레임: `0xA0`. 응답 RX2에 동일 값 에코됨 |

#### TX3 — 존 번호 (Zone Number)

| 비트 | 설명 | 값 |
|------|------|-----|
| `[7:4]` | 그룹 번호 (Group) | 0~15 |
| `[3:0]` | 실내기 번호 (Indoor Unit) | 0~15 |

**주의사항:**
- 단일 실내기 시스템: Zone 0 사용
- 멀티 실내기 시스템: 실내기에 프로그래밍된 존 번호 - 1 (0 인덱스)
- 월패드 또는 리모컨에서 존 번호 확인 가능

#### TX4 — 상태/제어 플래그

| 비트 | 필드명 | 설명 | 값 |
|------|--------|------|-----|
| `[0]` | **ON** | 전원 상태 | 0: OFF, 1: ON |
| `[1]` | **EXE** | 쓰기 실행 플래그 | 0: 읽기 전용, 1: 쓰기 모드 |
| `[2]` | **Lock** | 차일드 락 | 0: 해제, 1: 잠금 |
| `[3]` | — | 예약 | 0 |
| `[4]` | **Plasma** | 이온 제어 | 0: OFF, 1: ON |
| `[7:5]` | — | 예약 | 0 |

**플래그 조합 예시:**

| 목적 | TX4 값 | 비트 패턴 |
|------|--------|-----------|
| 읽기 전용 (OFF 상태) | `0x00` | `0000_0000` |
| 읽기 전용 (ON 상태) | `0x01` | `0000_0001` |
| ON + 쓰기 실행 | `0x03` | `0000_0011` |
| OFF + 쓰기 실행 | `0x02` | `0000_0010` |
| ON + 쓰기 + 차일드 락 | `0x07` | `0000_0111` |
| ON + 플라즈마 + 쓰기 | `0x13` | `0001_0011` |

#### TX5 — 운전 모드 / 풍량 / 스윙

| 비트 | 필드명 | 설명 | 값 |
|------|--------|------|-----|
| `[2:0]` | **Mode** | 운전 모드 | 0: 냉방, 1: 제습, 2: 송풍, 3: 자동(상태전용), 4: 난방 |
| `[3]` | **Auto Swing** | 자동 풍향 (덕트형) | 0: 수동, 1: 자동 |
| `[6:4]` | **Fan Speed** | 풍량 단계 | 0: 변경없음, 1: 약, 2: 중, 3: 강, 4: 자동, 5: 미풍, 6: 터보, 7: 미풍+터보 |
| `[7]` | — | 예약 | 0 |

**운전 모드 코드 표:**

| 코드 | 모드 | 설명 | 온도 범위 |
|------|------|------|-----------|
| 0 | Cool (냉방) | 냉방 운전 | 18~30°C |
| 1 | Dry (제습) | 제습 운전 | 18~30°C |
| 2 | Fan (송풍) | 송풍 전용 | 18~30°C |
| 3 | Auto (자동) | 자동 운전 (상태 확인 전용) | 18~30°C |
| 4 | Heat (난방) | 난방 운전 | 16~30°C |

**풍량 코드 표:**

| 코드 | 풍량 | 설명 |
|------|------|------|
| 0 | No change | 변경 없음 (읽기 시) |
| 1 | Low | 약풍 |
| 2 | Medium | 중풍 |
| 3 | High | 강풍 |
| 4 | Auto | 자동 |
| 5 | Slow/Quiet | 미풍/저소음 (선택적) |
| 6 | Power/Turbo | 파워/터보 (선택적) |
| 7 | Slow+Power | 미풍+파워 (선택적) |

**TX5 조합 예시:**

| 목적 | TX5 값 | 비트 패턴 | 설명 |
|------|--------|-----------|------|
| 냉방 + 자동풍 | `0x40` | `0100_0000` | Mode=0(냉방), Fan=4(자동) |
| 난방 + 강풍 | `0x34` | `0011_0100` | Mode=4(난방), Fan=3(강풍) |
| 제습 + 약풍 | `0x11` | `0001_0001` | Mode=1(제습), Fan=1(약풍) |
| 냉방 + 자동풍 + 스윙 | `0x48` | `0100_1000` | Mode=0(냉방), Swing=1, Fan=4(자동) |

#### TX6 — 설정 온도 (Target Temperature)

| 비트 | 설명 |
|------|------|
| `[3:0]` | 설정 온도 값 (인코딩됨) |
| `[7:4]` | 미사용 (0 권장) |

**인코딩 공식:**

```
TX6 = 설정온도(°C) - 15
```

**변환 테이블:**

| 설정 온도 | TX6 값 | Hex |
|-----------|--------|-----|
| 16°C | 1 | 0x01 |
| 17°C | 2 | 0x02 |
| 18°C | 3 | 0x03 |
| 20°C | 5 | 0x05 |
| 22°C | 7 | 0x07 |
| 24°C | 9 | 0x09 |
| 25°C | 10 | 0x0A |
| 26°C | 11 | 0x0B |
| 28°C | 13 | 0x0D |
| 30°C | 15 | 0x0F |

**모드별 온도 범위:**

| 모드 | 최소 | 최대 | TX6 범위 |
|------|------|------|----------|
| 난방 (Heat) | 16°C | 30°C | 1~15 |
| 냉방/제습/송풍/자동 | 18°C | 30°C | 3~15 |

#### TX7 — 체크섬

```
TX7 = (TX0 + TX1 + TX2 + TX3 + TX4 + TX5 + TX6) XOR 0x55
```

---

## 4. 응답 패킷 (RX: 실외기 → 컨트롤러)

16바이트 고정 길이. 요청이 잘못된 경우 0바이트 반환 (타임아웃).

### 4.1 패킷 구조 요약

```
+------+------+------+------+------+------+------+------+------+------+------+------+------+------+------+------+
| RX0  | RX1  | RX2  | RX3  | RX4  | RX5  | RX6  | RX7  | RX8  | RX9  | RX10 | RX11 | RX12 | RX13 | RX14 | RX15 |
|Header|Status| Echo | Unkn | Zone |Error | Mode |TgtTmp|RmTmp |PipeIn|PipeOt|ActLd |PwrFlg|DsnLd |OduLd | Chk  |
+------+------+------+------+------+------+------+------+------+------+------+------+------+------+------+------+
```

### 4.2 바이트별 상세 정의

#### RX0 — 프레임 헤더

| 비트 | 설명 | 값 |
|------|------|-----|
| `[7:0]` | 프레임 헤더/길이 | `0x10` (16) |

#### RX1 — 실내기 상태 플래그

| 비트 | 필드명 | 설명 | 값 |
|------|--------|------|-----|
| `[0]` | **ON** | 전원 상태 | 0: OFF, 1: ON |
| `[1]` | **Connected** | 실내기 연결 상태 | 0: 미연결, 1: 연결됨 |
| `[2]` | **Lock** | 차일드 락 상태 | 0: 해제, 1: 잠금 |
| `[3]` | — | 예약/미확인 | — |
| `[4]` | **Plasma** | 이온 상태 | 0: OFF, 1: ON |
| `[7:5]` | — | 예약/미확인 | — |

#### RX2 — 요청 에코 (Request Echo)

요청 TX2 값이 그대로 반환됨. 요청-응답 대응 검증에 사용.

#### RX3 — 미확인 (Unknown)

기능 미확인. 추가 리버스 엔지니어링 필요.

#### RX4 — 존 번호 (Zone Number)

요청한 존 번호가 반환됨. 요청 TX3과 일치 확인.

#### RX5 — 에러 코드 (Error Code)

| 값 | 설명 |
|-----|------|
| 0 | 정상 (에러 없음) |
| 1~255 | LG 서비스 에러/알람 코드 |

**주요 에러 코드 확인 방법:** LG 서비스 매뉴얼 참조. 에러 발생 시 해당 코드를 BMS 또는 모니터링 시스템에 전달하여 알람 처리.

#### RX6 — 운전 모드 / 풍량 / 스윙 (현재 상태)

| 비트 | 필드명 | 설명 | 값 |
|------|--------|------|-----|
| `[2:0]` | **Mode** | 현재 운전 모드 | TX5와 동일 코드 (0~4) |
| `[3]` | **Swing** | 자동 풍향 상태 | 0: 수동, 1: 자동 |
| `[6:4]` | **Fan Speed** | 현재 풍량 | TX5와 동일 코드 (0~7) |
| `[7]` | — | 예약 | — |

#### RX7 — 설정 온도 (Target Temperature)

**디코딩 공식:**

```
설정온도(°C) = (RX7 & 0x0F) + 15
```

**예시:** `RX7 = 0x17` → `(0x17 & 0x0F) + 15 = 7 + 15 = 22°C`

#### RX8 — 실내 온도 (Room Temperature)

**디코딩 공식:**

```
실내온도(°C) = (192 - RX8) / 3
```

**예시:** `RX8 = 0x66 (102)` → `(192 - 102) / 3 = 30°C`

#### RX9 — 입구 배관 온도 (Pipe In Temperature)

**디코딩 공식:**

```
입구배관온도(°C) = (192 - RX9) / 3
```

#### RX10 — 출구 배관 온도 (Pipe Out Temperature)

**디코딩 공식:**

```
출구배관온도(°C) = (192 - RX10) / 3
```

#### RX11 — 존 활성 부하 (Zone Active Load)

| 항목 | 설명 |
|------|------|
| 범위 | 0~255 |
| 의미 | 존별 실시간 동적 부하 지표 |
| LonWorks 매핑 | `nvoLoadEstimate` / `nvoUnitLoad` |
| 기준값 | 204 = 유휴 상태, 낮을수록 부하 높음 |

#### RX12 — 존 전원 상태 플래그 (Zone Power State Flag)

| 값 | 설명 |
|-----|------|
| 0 | 운전 중 (Running) |
| 1 | 정지/유휴 (Off/Idle) |

LonWorks `nvoOnOff` 매핑. 상태 전환 시 값이 일시적으로 불안정할 수 있음(jitter).

#### RX13 — 존 설계 부하 지표 (Zone Design Load Index)

| 항목 | 설명 |
|------|------|
| 범위 | 0~255 |
| 의미 | 고정된 용량/덕트 크기 기반 설계 부하 |
| LonWorks 매핑 | `nciRatedCapacity` |
| 대표값 예시 | 9, 12, 24, 36 등 |

#### RX14 — 실외기 총 부하 (ODU Total Load)

| 항목 | 설명 |
|------|------|
| 범위 | 0~255 |
| 의미 | 시스템 전체 부하 지표 (활성 존 부하의 합산) |
| LonWorks 매핑 | `nvoThermalLoad` / `nvoOduLoadFactor` |
| 특성 | 동일 실외기에 연결된 모든 존에서 동일한 값 반환 |

#### RX15 — 체크섬

```
RX15 = (RX0 + RX1 + ... + RX14) XOR 0x55
```

---

## 5. 온도 변환 공식 요약

**중요: 설정 온도와 실측 온도의 변환 공식이 다르므로 반드시 구분하여 사용할 것!**

### 5.1 설정 온도 (Target Temperature, RX7)

```
인코딩: TX6 = 설정온도 - 15
디코딩: 설정온도 = (RX7 & 0x0F) + 15
유효범위: 16~30°C
```

### 5.2 실측 온도 (Room/Pipe, RX8~RX10)

```
디코딩: 온도(°C) = (192 - raw_byte) / 3
```

### 5.3 변환 테이블 (실측 온도)

| Raw 값 | Hex | 온도(°C) |
|--------|-----|----------|
| 132 | 0x84 | 20.0 |
| 117 | 0x75 | 25.0 |
| 102 | 0x66 | 30.0 |
| 87 | 0x57 | 35.0 |
| 72 | 0x48 | 40.0 |
| 57 | 0x39 | 45.0 |
| 42 | 0x2A | 50.0 |

### 5.4 C 구현 코드

```c
// 설정 온도 인코딩/디코딩
uint8_t lgap_encode_target_temp(uint8_t celsius) {
    if (celsius < 16) celsius = 16;
    if (celsius > 30) celsius = 30;
    return celsius - 15;
}

uint8_t lgap_decode_target_temp(uint8_t raw) {
    return (raw & 0x0F) + 15;
}

// 실측 온도 디코딩 (실내/배관 온도)
float lgap_decode_room_temp(uint8_t raw) {
    return (192.0f - raw) / 3.0f;
}
```

---

## 6. LonWorks 프로토콜 매핑

응답 바이트 11~14는 LG LonWorks BMS 통합 프로토콜과 정확히 매핑된다.

| LGAP 바이트 | 필드명 | LonWorks 필드 | 용도 |
|-------------|--------|---------------|------|
| RX11 | Zone Active Load | `nvoLoadEstimate` / `nvoUnitLoad` | 존별 실시간 동적 부하 |
| RX12 | Zone Power State | `nvoOnOff` | 존 ON/OFF 상태 |
| RX13 | Zone Design Load | `nciRatedCapacity` | 고정 설계 용량 가중치 |
| RX14 | ODU Total Load | `nvoThermalLoad` / `nvoOduLoadFactor` | 전체 압축기 부하 |

이 데이터를 활용하면 에너지 모니터링, 부하 분산 분석, 압축기 가동률 추적이 가능하다.

---

## 7. 구현 가이드

### 7.1 폴링 전략

실외기는 자발적으로 상태를 전송하지 않으므로 각 존을 주기적으로 폴링해야 한다.

```
1. 읽기 요청 전송 (TX4 bit1 = 0)
2. 16바이트 응답 수신 및 파싱
3. 내부 상태 갱신
4. 적정 간격 대기 (존 간 1~5초 권장)
5. 다음 존으로 반복
```

**권장 폴링 주기:**

| 시나리오 | 주기 | 비고 |
|---------|------|------|
| 단일 존 모니터링 | 3~5초 | 일반적 사용 |
| 멀티 존 순차 폴링 | 존 간 1~2초 | 전체 사이클 = N × 간격 |
| 제어 명령 직후 | 0.5~1초 | 상태 확인용 빠른 폴링 |

### 7.2 쓰기(제어) 절차

```
1. TX4 bit1 = 1 (EXE 플래그 설정)
2. TX4~TX6에 원하는 상태 설정
3. 요청 전송
4. 응답 수신 → 새 상태 확인
5. TX4 bit1 = 0 으로 복귀 (읽기 전용 모드)
```

**제어 흐름도:**

```
[읽기모드] → 상태변경 필요 → [EXE=1 설정] → 요청전송 → 응답확인 → [EXE=0 복귀] → [읽기모드]
```

### 7.3 멀티 존 관리

- 동일한 RS-485 인터페이스로 모든 존 관리
- TX3 (존 번호)를 변경하며 순차적 폴링
- RX14 (실외기 총 부하)는 동일 실외기 내 모든 존에서 동일 값
- **중요**: 모든 존은 동일한 냉/난방 모드를 사용해야 함 (시스템 제약)

### 7.4 에러 처리

```c
// 응답 검증 절차
bool lgap_validate_response(const uint8_t *rx, uint8_t rx_len,
                            const uint8_t *tx) {
    // 1. 응답 길이 확인
    if (rx_len != 16) return false;  // 0바이트 = 잘못된 요청

    // 2. 체크섬 검증
    if (!lgap_verify_checksum(rx, 16)) return false;

    // 3. 요청-응답 대응 확인 (RX2 == TX2)
    if (rx[2] != tx[2]) return false;

    // 4. 에러 코드 확인
    if (rx[5] != 0) {
        // 에러 코드를 모니터링 시스템에 보고
        report_error(rx[4], rx[5]);  // 존 번호, 에러 코드
    }

    return true;
}
```

### 7.5 부하 모니터링

```c
// 존 효율 계산 (0.0 ~ 1.0, 1.0 = 최대 부하)
float zone_efficiency = (204.0f - rx[11]) / 204.0f;

// 존 전력 소비 추정 (kW)
float zone_power_kw = zone_efficiency * zone_design_load_kw;

// 시스템 부하율 (%)
float system_load_pct = ((float)rx[14] / sum_of_all_design_loads) * 100.0f;

// 압축기 가동 상태 판단
bool compressor_active = (rx[14] > 0) && (rx[12] == 0);
```

---

## 8. 구현된 기능 매핑 (PMBUSB00A 기준)

LGAP에서 LG 공식 Modbus 게이트웨이(PMBUSB00A)의 기능과 매핑된 항목 목록.

### 8.1 프로토콜 레벨 구현

| 기능 | 요청 바이트 | 응답 바이트 | 상태 |
|------|-------------|-------------|------|
| 전원 ON/OFF | TX4 bit0 | RX1 bit0 | 구현완료 |
| 운전 모드 (냉방/난방/제습/송풍/자동) | TX5 bit[2:0] | RX6 bit[2:0] | 구현완료 |
| 풍량 (약/중/강/자동/미풍/터보) | TX5 bit[6:4] | RX6 bit[6:4] | 구현완료 |
| 설정 온도 (16~30°C) | TX6 | RX7 | 구현완료 |
| 실내 온도 | — | RX8 | 구현완료 |
| 입구 배관 온도 | — | RX9 | 구현완료 |
| 출구 배관 온도 | — | RX10 | 구현완료 |
| 자동 풍향 (스윙) | TX5 bit3 | RX6 bit3 | 구현완료 |
| 차일드 락 | TX4 bit2 | RX1 bit2 | 구현완료 |
| 플라즈마 이온 | TX4 bit4 | RX1 bit4 | 구현완료 |
| 에러/알람 코드 | — | RX5 | 구현완료 |
| 존 활성 부하 | — | RX11 | 구현완료 |
| 존 전원 플래그 | — | RX12 | 구현완료 |
| 존 설계 부하 | — | RX13 | 구현완료 |
| 실외기 총 부하 | — | RX14 | 구현완료 |

### 8.2 미매핑 항목

| 기능 | 추정 위치 | 비고 |
|------|-----------|------|
| 필터 알람 해제 | 요청 (미확인) | 필터 알람 클리어 쓰기 명령 |
| 필터 알람 상태 | 응답 (미확인) | 필터 정비 필요 플래그 |
| 실내기 주소 잠금 | 요청/응답 (미확인) | 존 번호 변경 방지 |
| 설정 온도 제한 | 응답 (미확인) | 모드별 최소/최대 온도 제약 |

### 8.3 예약/미확인 비트

| 위치 | 비트 | 상태 |
|------|------|------|
| TX4 | bit3, bit[7:5] | 예약 (항상 0) |
| TX5 | bit7 | 예약 (항상 0) |
| RX1 | bit3, bit[7:5] | 미확인/예약 |
| RX2 | 전체 | TX2 에코 (별도 디코딩 불필요) |
| RX3 | 전체 | 기능 미확인 |
| RX6 | bit7 | 예약 |

---

## 9. 패킷 예제 모음

### 9.1 존 0 상태 읽기 (Read Only)

```
TX: 10 00 A0 00 00 00 00 F5
     │  │  │  │  │  │  │  └─ 체크섬: (0x10+0+0xA0+0+0+0+0) XOR 0x55
     │  │  │  │  │  │  └──── TX6: 설정온도 없음 (읽기 전용)
     │  │  │  │  │  └─────── TX5: 모드/풍량 없음
     │  │  │  │  └────────── TX4: 0x00 (EXE=0, 읽기 전용)
     │  │  │  └───────────── TX3: Zone 0
     │  │  └──────────────── TX2: 0xA0 (제어 프레임)
     │  └─────────────────── TX1: 0x00 (일반 명령)
     └────────────────────── TX0: 0x10 (프레임 헤더)
```

### 9.2 존 0 냉방 ON — 24°C, 자동풍

```
TX: 10 00 A0 00 03 40 09 A7
     │           │  │  │  └─ 체크섬
     │           │  │  └──── TX6: 24-15=9 (24°C)
     │           │  └─────── TX5: 0100_0000 → Fan=4(자동), Mode=0(냉방)
     │           └────────── TX4: 0000_0011 → ON=1, EXE=1
```

### 9.3 존 0 난방 ON — 22°C, 강풍, 스윙

```
TX: 10 00 A0 00 03 3C 07 AB
     │           │  │  │  └─ 체크섬
     │           │  │  └──── TX6: 22-15=7 (22°C)
     │           │  └─────── TX5: 0011_1100 → Fan=3(강), Swing=1, Mode=4(난방)
     │           └────────── TX4: 0000_0011 → ON=1, EXE=1
```

### 9.4 존 0 전원 OFF

```
TX: 10 00 A0 00 02 00 00 F7
     │           │  └────── TX5: 모드/풍량 무관
     │           └───────── TX4: 0000_0010 → ON=0, EXE=1
```

### 9.5 응답 패킷 파싱 예시

```
RX: 10 03 A0 40 00 00 40 07 75 7F 7F 28 00 18 33 55

바이트별 해석:
  RX0  = 0x10 → 프레임 헤더 (16)
  RX1  = 0x03 → bit0=1(ON), bit1=1(연결됨)
  RX2  = 0xA0 → TX2 에코 확인
  RX3  = 0x40 → 미확인
  RX4  = 0x00 → Zone 0
  RX5  = 0x00 → 에러 없음
  RX6  = 0x40 → bit[2:0]=0(냉방), bit[6:4]=4(자동풍)
  RX7  = 0x07 → (0x07 & 0x0F)+15 = 22°C (설정온도)
  RX8  = 0x75 → (192-117)/3 = 25.0°C (실내온도)
  RX9  = 0x7F → (192-127)/3 = 21.7°C (입구배관온도)
  RX10 = 0x7F → (192-127)/3 = 21.7°C (출구배관온도)
  RX11 = 0x28 → 존 활성 부하 40 → 효율: (204-40)/204 = 80.4%
  RX12 = 0x00 → 운전 중
  RX13 = 0x18 → 설계 부하 24
  RX14 = 0x33 → 실외기 총 부하 51
  RX15 = 0x55 → 체크섬 OK
```

---

## 10. XIAO-nRF52840 펌웨어 적용 가이드

본 프로젝트에서 LGAP를 XIAO-nRF52840 디바이스(Method A: PI485)에 적용하기 위한 핵심 구현 사항.

### 10.1 하드웨어 연결

```
XIAO-nRF52840 ──(UART)──→ MAX485 ──(RS-485)──→ PI-485 (실외기)
   TX(D6) ────────────→ DI                    CENA ───→ A(+)
   RX(D7) ←────────────  RO                    CENB ───→ B(-)
   DE/RE(D5) ─────────→ DE + RE (결합)
```

### 10.2 시리얼 설정

```c
#define LGAP_BAUD_RATE    4800
#define LGAP_SERIAL_CFG   SERIAL_8N1
#define LGAP_TX_SIZE      8
#define LGAP_RX_SIZE      16
#define LGAP_TIMEOUT_MS   500   // 응답 타임아웃
#define LGAP_POLL_MS      3000  // 폴링 주기
```

### 10.3 핵심 데이터 구조

```c
typedef struct {
    bool     power_on;
    uint8_t  mode;          // 0:냉방, 1:제습, 2:송풍, 3:자동, 4:난방
    uint8_t  fan_speed;     // 0~7
    bool     swing;
    bool     child_lock;
    bool     plasma;
    uint8_t  target_temp;   // 16~30 °C
    float    room_temp;     // 실내 온도
    float    pipe_in_temp;  // 입구 배관 온도
    float    pipe_out_temp; // 출구 배관 온도
    uint8_t  error_code;    // 에러 코드
    uint8_t  zone_load;     // 존 활성 부하
    uint8_t  odu_load;      // 실외기 총 부하
    bool     connected;     // 실내기 연결 상태
} lgap_state_t;

typedef struct {
    uint8_t  zone;          // 존 번호
    bool     write;         // 쓰기 모드
    bool     power_on;
    uint8_t  mode;
    uint8_t  fan_speed;
    bool     swing;
    bool     child_lock;
    bool     plasma;
    uint8_t  target_temp;   // 16~30 °C
} lgap_command_t;
```

### 10.4 패킷 빌드/파싱 함수

```c
// 요청 패킷 생성
void lgap_build_request(const lgap_command_t *cmd, uint8_t *buf) {
    buf[0] = 0x10;  // 프레임 헤더
    buf[1] = 0x00;  // 일반 명령
    buf[2] = 0xA0;  // 제어 프레임 ID
    buf[3] = cmd->zone;

    // TX4: 상태 플래그
    buf[4] = 0;
    if (cmd->power_on)   buf[4] |= 0x01;  // bit0: ON
    if (cmd->write)      buf[4] |= 0x02;  // bit1: EXE
    if (cmd->child_lock) buf[4] |= 0x04;  // bit2: Lock
    if (cmd->plasma)     buf[4] |= 0x10;  // bit4: Plasma

    // TX5: 모드 + 풍량 + 스윙
    buf[5] = (cmd->mode & 0x07);                    // bit[2:0]: Mode
    if (cmd->swing) buf[5] |= 0x08;                 // bit3: Swing
    buf[5] |= ((cmd->fan_speed & 0x07) << 4);       // bit[6:4]: Fan

    // TX6: 설정 온도
    buf[6] = lgap_encode_target_temp(cmd->target_temp);

    // TX7: 체크섬
    buf[7] = lgap_calc_checksum(buf, 7);
}

// 응답 패킷 파싱
bool lgap_parse_response(const uint8_t *rx, lgap_state_t *state) {
    // 체크섬 검증
    if (!lgap_verify_checksum(rx, 16)) return false;

    // RX1: 상태 플래그
    state->power_on   = (rx[1] & 0x01) != 0;
    state->connected  = (rx[1] & 0x02) != 0;
    state->child_lock = (rx[1] & 0x04) != 0;
    state->plasma     = (rx[1] & 0x10) != 0;

    // RX5: 에러 코드
    state->error_code = rx[5];

    // RX6: 운전 모드 + 풍량 + 스윙
    state->mode      = rx[6] & 0x07;
    state->swing     = (rx[6] & 0x08) != 0;
    state->fan_speed = (rx[6] >> 4) & 0x07;

    // RX7: 설정 온도
    state->target_temp = lgap_decode_target_temp(rx[7]);

    // RX8~RX10: 실측 온도
    state->room_temp     = lgap_decode_room_temp(rx[8]);
    state->pipe_in_temp  = lgap_decode_room_temp(rx[9]);
    state->pipe_out_temp = lgap_decode_room_temp(rx[10]);

    // RX11~RX14: 부하 데이터
    state->zone_load = rx[11];
    state->odu_load  = rx[14];

    return true;
}
```

### 10.5 BLE 연동 데이터 변환

LGAP 상태 → BLE Characteristic 매핑 (기존 BLE 프로토콜 설계 기반):

| LGAP 데이터 | BLE Characteristic | 바이트 위치 |
|-------------|-------------------|-------------|
| power_on, mode, fan_speed | Status (0x0002) | Byte 0~2 |
| target_temp | Temperature (0x0003) | Byte 0 |
| room_temp | Temperature (0x0003) | Byte 1~2 (×10) |
| error_code | Status (0x0002) | Byte 3 |
| zone_load, odu_load | Power (0x0005) | Byte 0~1 |

---

## 부록 A. 빠른 참조 카드

### 요청 (TX) 8바이트

| Byte | 이름 | 기본값 | 비고 |
|------|------|--------|------|
| 0 | Header | 0x10 | 고정 |
| 1 | Command | 0x00 | 일반 제어 |
| 2 | CmdID | 0xA0 | 제어 프레임 |
| 3 | Zone | 0~255 | High:그룹, Low:실내기 |
| 4 | Flags | — | ON/EXE/Lock/Plasma |
| 5 | Mode | — | Mode[2:0]/Swing[3]/Fan[6:4] |
| 6 | Temp | — | 설정온도 - 15 |
| 7 | Checksum | — | (sum TX0~6) XOR 0x55 |

### 응답 (RX) 16바이트

| Byte | 이름 | 공식 |
|------|------|------|
| 0 | Header | 0x10 |
| 1 | Status | ON/Connected/Lock/Plasma |
| 2 | Echo | = TX2 |
| 3 | Unknown | — |
| 4 | Zone | 존 번호 |
| 5 | Error | 0=정상, 1~255=에러 |
| 6 | Mode | Mode[2:0]/Swing[3]/Fan[6:4] |
| 7 | Target Temp | (val & 0x0F) + 15 |
| 8 | Room Temp | (192 - val) / 3 |
| 9 | Pipe In | (192 - val) / 3 |
| 10 | Pipe Out | (192 - val) / 3 |
| 11 | Zone Load | 204=유휴, 낮을수록 고부하 |
| 12 | Zone Power | 0=운전중, 1=정지 |
| 13 | Design Load | 설계 용량 지표 |
| 14 | ODU Load | 시스템 전체 부하 |
| 15 | Checksum | (sum RX0~14) XOR 0x55 |
