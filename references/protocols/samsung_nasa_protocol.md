# Samsung NASA Protocol Analysis — 삼성 시스템 에어컨 통신 프로토콜 분석 정리

## **개요**

- 삼성 시스템 에어컨 원격 제어를 위한 프로토콜 자료
- **인터넷 검색 및 테스트를 통해 확인된 내용으로 실제 프로토콜과 다를 수 있음**

## **주소 체계**

![주소체계.png](%EC%82%BC%EC%84%B1%20%EC%8B%9C%EC%8A%A4%ED%85%9C%20%EC%97%90%EC%96%B4%EC%BB%A8%20%ED%86%B5%EC%8B%A0%20%ED%94%84%EB%A1%9C%ED%86%A0%EC%BD%9C%20%EB%B6%84%EC%84%9D%20%EC%A0%95%EB%A6%AC/%E1%84%8C%E1%85%AE%E1%84%89%E1%85%A9%E1%84%8E%E1%85%A6%E1%84%80%E1%85%A8.png)

- 외부제어기: 전체 구성에서 1대만 연결 가능
- 실외기: 최대 16대
- 실내기: 실외기 1대 당 최대 64대

## **통신**

- 인터페이스 : RS485
    - 설정 : 9600bps, 8bit, 1 stop bit, even parity
    - 연결: F1, F2, V1, V2에 EW11 연결
- 프로토콜 : NASA Protocol

## **프레임**

### **구조**

```
[STX][LEN][SA][DA][CMD][SEQ#][CNT][MSG0][MSG1]...[CRC][ETX]
```

- Byte Order: Big-Endian

### **예제**

```
32 0015 620000 200000 C013 A8  02  400001 42010118 CD4D 34

```

|  | 위치(bytes) | 데이터(Hex) | 설명 |
| --- | --- | --- | --- |
| STX | 0 | 32 | Start of Packet |
| LEN | 1~2 | 0015 | Packet Length (STX, ETX 제외) |
| SA | 3~5 | 620000 | Source Address |
| DA | 6~8 | 200000 | Destination Address |
| CMD | 9~10 | C013 | Command |
| SEQ# | 11 | A8 | Packet Number |
| CNT | 12 | 02 | Number of Message Sets |
| MSG0 | 13~15 | 4000 01 | Message Set 1 |
| MSG1 | 16~19 | 4201 0118 | Message Set 2 |
| CRC | 20~21 | CD4D | Checksum (CRC16-CCITT) |
| ETX | 22 | 34 | End of Packet |

### **주소 체계**

- 10 xx 00: 에어컨 실외기
- 20 xx yy: 에어컨 실내기
- 6A EE FF: 외부제어기
- B0 FF FF: Broadcasting
- B0 xx FF: 실외기 Broadcasting
- B3 xx yy: 실내기 Broadcasting
- 10 FF FF: 실외기 주소 미확정
- 10 FF 00: 실외기 Random주소 확정
- 참고
    - xx : 실외기 physical 주소 (0x00 ~ 0x0F)
    - yy: 실내기 physical 주소 (0x00 ~ 0x3F)

### **Command(명령 코드)**

| Command | Status | Source | Destination | Comment |
| --- | --- | --- | --- | --- |
| **C001** | Standby | 외부제어기 | 실외기 | Request |
| **C005** | Standby | 실외기 | 외부제어기 | Response |
| **C011** | Normal | 외부제어기 | 실외기 | Request |
| **C012** | Normal | 외부제어기 | 실외기 | Setting |
| **C013** | Normal | 외부제어기 | 실외기 | Control |
| **C014** | Normal | 실내외기 |  | Notification(Status) |
| **C016** | Normal | 실외기 | 외부제어기 | Request/Setting에 대한 Response |

### **Message Set**

- Index와 Value로 구성

### **Index**

- 크기: 2 바이트
    
    
    | Index | 설명 |
    | --- | --- |
    | 4000 | 전원 |
    | 4001 | 모드 (자동/냉방/제습/송풍) |
    | 4006 | 풍량 (자동/미풍/약풍/강풍) |
    | 4007 | 롱바람 |
    | 4011 | 풍향 (상하) |
    | 4043 | 청정 |
    | 4060 | 무풍 |
    | 407E | 풍향 (좌우) |
    | 4111 | 자동건조 설정 |
    | 4201 | 설정온도 |
    | 4203 | 실내온도 |

### **Value**

- 크기: index의 2번째 니블 값에 따라 결정
    
    
    | 2번째 니블 | 값 크기(bytes) |
    | --- | --- |
    | 0 | 1 |
    | 1 | 1 |
    | 2 | 2 |
    | 4 | 4 |
    | 6 | Command에 따라 다름 |
- 예제 - 온도 표현
    - 섭씨 온도 × 10을 16진수로 표현
    - 예: 18°C = 0x00B4, 19°C = 0x00BE, 30°C = 0x012C

## **실외기 제어**

- 외부 제어를 위해서는 두단계 필요
    - 실외기 주소 등록 필요 확인
    - 실외기 주소 확정

### **실외기 주소 확인 요청**

- 외부제어기에서 실외기의 주소 확인 요청
- 명령 코드: C0 01
- SA: 외부제어기(6A EE FF)
- DA: 모든 실외기(B0 FF 10)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 04 08 | FF FF FF FF | 주소 확인 |

### **예제**

```
32 00 14 6A EE FF B0 FF 10 C0 11 00 01 04 08 FF FF FF FF C3 F8 34
```

- Command: C0 14
- SA: 6A EE FF
- DA: B0 FF 10
- Message Set: 04 08 FF FF FF FF

### **실외기 주소 확인 응답**

- 실외기들은 설정된 주소를 외부제어기에 전송
- 명령 코드: C0 05
- SA: 임의의 실외기(10 xx 00)
- DA: 외부제어기(6A EE FF)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 04 08 | 10 xx 00 | 실외기 주소 |

### **예제**

```
32 00 14 10 00 00 6A EE FF C0 05 01 01 04 08 00 10 00 00 3C 4D 34
32 00 14 10 01 00 6A EE FF C0 05 01 01 04 08 00 10 01 00 A1 80 34
32 00 14 10 02 00 6A EE FF C0 05 01 01 04 08 00 10 02 00 17 F6 34
32 00 14 10 03 00 6A EE FF C0 05 01 01 04 08 00 10 02 00 B9 0A 34
```

- 실외기(10 00 00, 10 01 00)은 정상 주소 확인 완료
- 실외기(10 02 00)는 두대가 중복 설정 응답(새롭게 설정 필요)

### **실외기 주소 등록 필요 확인 요청**

- 외부제어기에서 실외기의 주소 등록이 필요한지 확인 요청
- 명령 코드: C0 14
- SA: 외부제어기(6A EE FF)
- DA: 모든 실외기(B0 FF FF)
- Sequence #: 0
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 04 | 00 | 주소 등록 필요 확인 |

### **예제**

```
32 00 11 6A EE FF B0 FF FF C0 14 00 01 20 04 00 41 A3 34
```

- Command: C0 14
- SA: 6A EE FF
- DA: B0 FF FF
- Message Set: 20 04 00

### **실외기 주소 등록 필요 확인 응답**

- 주소 등록이 필요한 실외기에서 외부제어기에 확인 응답
- 명령 코드: C0 14
- SA: 임의의 실외기(10 FF FF)
- DA: 외부제어기(6A EE FF)
- Sequence #: 임의의 값
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 04 | 01 | 주소 등록 필요 응답 |
    | 04 18 | XX XX XX XX | Random Address |
    | 02 17 | XX XX | Network Address |
    | 04 17 | XX XX XX XX | Origin Address |
    | 04 19 | XX XX XX XX | Setting Address |

### **예제**

```
32 00 27 10 FF FF B0 FF FF C0 14 0C 05 20 04 01 04 18 00 10 9F D9 02 17 1D D1
04 17 00 10 00 00 04 19 00 10 00 00 FC 53 34
```

### **실외기 주소 확정 요청**

- 외부제어기에서 특정 실외기에 주소 확정 요청
- 명령 코드: C0 12
- SA: 외부제어기(6A EE FF)
- DA: 실외기(Step 2에서 받은 Random Address 이용)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 04 | 03 | 주소 등록 요청 |
    | 04 18 | 주소 등록 필요 확인에서 받은 주소 | Random Address |
    | 02 17 | 주소 등록 필요 확인에서 받은 주소 | Network Address |
    | 04 17 | 주소 등록 필요 확인에서 받은 Setting Address | Origin Address |
    | 04 19 | 주소 등록 필요 확인에서 받은 주소 | Setting Address |

### **예제**

```
32 00 27 6A EE FF 10 9F D9 C0 12 3E 05 20 04 03 04 18 00 10 9F D9 02 17 1D D1 04 17 00 10 00 00 04 19 00 10 00 00 FC 9A 34
```

### **실외기 주소 확정 응답**

- 주소 확정 요청을 받은 실외기가 외부제어기에 응답
- 명령 코드: C0 15
- SA: 실외기(10 xx 00)
- DA: 외부제어기(6A EE FF)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 04 | 04 | 주소 등록 완료 |
    | 04 18 | **주소 확정 요청**에서 받은 주소 | Random Address |
    | 02 17 | **주소 확정 요청**에서 받은 주소 | Network Address |
    | 04 17 | **주소 확정 요청**에서 받은 주소 | Origin Address |
    | 04 19 | **주소 확정 요청**에서 받은 주소 | Setting Address |

### **예제**

```
32 00 27 10 00 00 6A EE FF C0 15 3E 05 20 04 04 04 18 00 10 9F D9 02 17 1D D1 04 17 00 10 00 00 04 19 00 10 00 00 64 65 34
```

## **실외기 통신 준비 상태 확인 요청**

- 외부제어기에서 실외기에 요청
- 명령 코드: C0 11
- SA: 외부제어기(6A EE FF)
- DA: 모든 실외기(B0 FF 10)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 10 | FF | 고정 |

### **예제**

```
32 00 11 6A EE FF B0 FF 10 C0 11 01 01 20 10 FF D0 4F 34
```

## **실외기 통신 준비 상태 확인 응답**

- 통신 준비 상태 확인 요청을 받은 실외기가 외부제어기에 응답
- 명령 코드: C0 15
- SA: 실외기(10 xx 00)
- DA: 외부제어기(6A EE FF)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 20 10 | Ax: 준비 상태
    이외: 준비 안됨 | 통신 준비 상태 |

### **예제**

```
32 00 11 10 00 00 6A EE FF C0 15 01 01 20 10 A8 2B EF 34: 실외기 0번 Ready 상태
32 00 11 10 01 00 6A EE FF C0 15 01 01 20 10 A8 28 9A 34: 실외기 1번 Ready 상태
32 00 11 10 02 00 6A EE FF C0 15 01 01 20 10 58 C2 1A 34: 실외기 2번 Ready 상태 아님
32 00 11 10 03 00 6A EE FF C0 15 01 01 20 10 A8 2E 70 34: 실외기 3번 Ready 상태
```

## **실내기 주소 확인 요청**

- 외부제어기에서 실내기에 요청
- 명령 코드: C0 11
- SA: 외부제어기(6A EE FF)
- DA: 모든 실내기(B2 FF 20)
- Sequence #: 1
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 04 08 | FF FF FF FF | 고정 |

### **예제**

```
32 00 14 6A EE FF B2 FF 20 C0 11 01 01 04 08 FF FF FF FF EA 55 34
```

## **실내기 주소 확인 응답**

- 주소 확인 요청을 수신한 모든 실내기
- 명령 코드: C0 15
- SA: 모든 실내기(20 xx yy)
- DA: 외부제어기(6A EE FF)
- Sequence #: 1
- Message Set
    
    
    | Index |  | Description |
    | --- | --- | --- |
    | 04 08 | 00 02 xx yy | SA |

### **예제**

```
32 00 14 20 00 00 6A EE FF C0 15 01 01 04 08 00 20 00 00 2B EF 34
32 00 14 20 00 01 6A EE FF C0 15 01 01 04 08 00 20 00 01 28 9A 34
32 00 14 20 00 02 6A EE FF C0 15 01 01 04 08 00 20 00 02 2D 05 34
32 00 14 20 00 03 6A EE FF C0 15 01 01 04 08 00 20 00 03 2E 70 34
```

## **실내기 상태 확인**

- 실내기 상태를 실외기에 전송
- 외부제어기에서는 실내기와 실외기간 통신 데이터 확인
- 명령 코드: C0 14
- SA: 실내기(20 xx yy)
    - xx: 실외기 주소
    - yy: 실내기 주소
- DA: 실외기(B3 xx FF)
- Sequence #: 00 ~ FF (1씩 증가)
- Message Set
    
    
    | Index | Description | Description |
    | --- | --- | --- |
    | 4000 | 00: off, 01: on, 그 외 : 무시 | 에어컨 운전 |
    | 4001 | 00 : 자동, 01 : 냉방, 02 : 제습, 03 : 송풍, 04 : 난방, 그 외 : 무시 | 에어컨 운전모드 상태 |
    | 4006 | 00 : 자동, 01 : 미풍, 02 : 약풍, 03 : 강풍, 그 외 : 무시 | 에어컨 바람세기 상태 |
    | 4011 | 00 : Off, 01 : On, 그 외:무시 | 에어컨 Swing On/Off 상태 |
    | 4027 | 00 : Off, 01 : On, 그 외:무시 | 에어컨 필터 청소 알림 On/Off 상태 |
    | 4201 | 온도 * 10 | 에어컨 설정 온도 |
    | 4203 | 온도*10
    0x0000~0x7FFF:영상 온도
    0x8000~0xFFFF:영하 온도 | 에어컨 실내 온도 |
    | 0202 |  | 에어컨 에러 코드 |
    | 0409 | 00 00 00 00 : 제한없음
    00 00 6A 6A : 사용제한 | 유무선리모컨 사용 제한 |

### **예제**

```
32 00 3C 20 02 0A B3 02 FF C0 14 00 0D 04 48 00 00 00 0C 40 07 FE 40 11 00 40 12 00 40 2F 00 40 4F FF 40 AE 0A 40 AF 0A 40 BD 01 42 01 00 F0 42 02 00 F0 42 2A 00 96 42 2B 01 7C 64 06 34

32 00 3D 20 02 0A B3 02 FF C0 14 AE 0C 22 F7 00 0C 22 F9 00 00 22 FA 00 00 22 FB 00 33 22 FC 03 FC 22 FD 00 01 22 FE 00 0A 22 FF 00 02 24 FB 00 00 01 0F 40 00 00 40 01 00 40 02 FF 69 DD 34
```

## **실내기 제어 요청**

- 명령 코드: C0 13
- SA: 외부제어기(6A EE FF)
- DA:
    - 단일 장비 (20 xx yy : 실외기 xx번의 실내기 yy번)
    - 실외기별(B2 xx 20 : 실외기 xx번의 전체 실내기)
    - 전체(전체 실외기의 전체 실내기)
- Sequence #: 0
- Message Set
    
    
    | Index | Value | Description |
    | --- | --- | --- |
    | 40 00 | 00 : Off, 01 : On | 운전 On/Off 설정 |
    | 40 01 | 00 : 자동, 01 : 냉방, 02 : 제습, 03 : 송풍, 04 : 난방 | 운전 모드 설정 |
    | 40 06 | 00 : 자동, 01 : 미풍, 02 : 약풍, 03 : 강풍 | 바람세기 설정 |
    | 40 11 | 00 : Off, 01 : On | Swing On/Off 설정 |
    | 40 25 | 00 : Off, 01 : On | 필터 청소 알림 리셋 설정 |
    | 42 01 | 온도 * 10 | 온도 설정 |
    | 04 09 | 00 00 00 00 : 제한, 00 00 6A 6A : 해제 | 유선 리모컨 사용 제한 설정 |
    | 40 50 | 00 : On, 01 : Off | 부저 On/Off 설정 |

### **예제**

```
32 00 11 6A EE FF 20 01 00 C0 13 00 01 40 00 01 4D 9F 34

32 00 2A 6A EE FF 20 01 00 C0 13 00 08 40 00 01 40 01 01 40 06 03 40 11 01 40 27 00 42 01 00 B4 04 09 00 00 00 00 40 50 00 58 20 34
```

## **실내기 제어 응답**

- 명령 코드: C0 16
- DA: 실내기(20 xx yy)
- SA: 외부제어기(6A EE FF)
- Sequence #: 0
- Message Set: none

### **예제**

```
32 00 0E 20 01 00 6A EE FF C0 16 00 00 81 29 34
```

# **주의사항**

1. **패킷 번호 관리**: 이전 패킷 보다 큰 번호 사용 필요
2. **데이터 수신 처리**: 수신 데이터가 끊길 경우 다음 데이터와 연결
3. **시작 바이트 식별**: 32 앞에 임의 바이트 포함 가능 (FFFDF1 등)

# **제한사항**

- 패킷 충돌 시 에어컨 동작 불가 가능성