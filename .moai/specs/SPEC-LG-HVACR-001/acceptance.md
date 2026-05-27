# SPEC-LG-HVACR-001: 인수 기준

> **SPEC ID**: SPEC-LG-HVACR-001
> **형식**: Given-When-Then (Gherkin)

> **명명 규약**: 본 문서는 v1.0 rename (2026-05-27) 기준이다. 프로토콜 코드 `lg_icp01` (LG ICP-01), 에이전트 타입 `lg_hvacr01` (LG HVACR-01), 노드 타입 `lg_hvacr01` / `lg_hvacr01_status` / `lg_hvacr01_control`.

---

## M1: 프레임 파서

### AC-M1-01: TYPE-A 프레임 조립

```gherkin
Given 바이트 스트림에 0x58로 시작하는 20바이트 시퀀스가 있을 때
When  프레임 파서가 바이트를 읽으면
Then  20바이트 TYPE-A 프레임으로 조립되어야 한다
And   프레임의 SEQ 값(byte[1])이 01~05 범위여야 한다
```

### AC-M1-02: TYPE-B 프레임 조립

```gherkin
Given 바이트 스트림에 0x81~0x85로 시작하는 40바이트 시퀀스가 있을 때
When  프레임 파서가 바이트를 읽으면
Then  40바이트 TYPE-B 프레임으로 조립되어야 한다
And   프레임의 IDU 번호가 (STX - 0x81 + 1)로 산출되어야 한다
```

### AC-M1-03: TYPE-A SEQ=01 XOR 체크섬

```gherkin
Given TYPE-A 프레임: 58 01 00 00 00 00 00 00 00 57 66 00 00 00 2e 00 19 01 43 1d
When  체크섬을 검증하면
Then  XOR(pkt[0:19]) == pkt[19] (0x1d)가 성립해야 한다
And   checksum_valid가 true여야 한다
```

### AC-M1-04: TYPE-A SEQ=04 SUM 체크섬

```gherkin
Given TYPE-A 프레임: 58 04 00 00 00 00 64 fe fe 00 84 00 00 00 00 00 00 00 15 55
When  체크섬을 검증하면
Then  SUM(pkt[0:19]) & 0xFF == pkt[19] (0x55)가 성립해야 한다
And   checksum_valid가 true여야 한다
```

### AC-M1-05: TYPE-A SEQ=02/03 체크섬 없음

```gherkin
Given TYPE-A 프레임 SEQ=02: 58 02 e0 06 02 00 81 4d 9f 00 31 cd e5 00 6b 5c 0f 00 f8 ee
When  체크섬을 검증하면
Then  체크섬 검증을 수행하지 않아야 한다 (b[18]=0xf8, b[19]=0xee는 센서 데이터)
And   checksum_valid가 true여야 한다 (패스)
```

### AC-M1-06: TYPE-B 이중 기록 검증 성공

```gherkin
Given TYPE-B 프레임에서 b[09]=0x52, b[29]=0x52 (설정온도 일치)
And   b[23]=0x6d, b[36]=0x6d (실내온도 일치)
When  이중 기록을 검증하면
Then  redundancy_valid가 true여야 한다
```

### AC-M1-07: TYPE-B 이중 기록 검증 실패

```gherkin
Given TYPE-B 프레임에서 b[09]=0x52, b[29]=0x53 (설정온도 불일치)
When  이중 기록을 검증하면
Then  redundancy_valid가 false여야 한다
And   프레임이 폐기되어야 한다
And   framesInvalid 카운터가 증가해야 한다
```

### AC-M1-08: TYPE-B 고정 바이트 구조 검증

```gherkin
Given TYPE-B 프레임에서 IDU_ADDR=0x83 (IDU#3)
When  고정 바이트를 검증하면
Then  pkt[1]이 0x02 또는 0x43이어야 한다
And   pkt[20]이 3이어야 한다 (0x83 - 0x81 + 1)
```

### AC-M1-09: TYPE-B 온도 변환 정확도

```gherkin
Given TYPE-B 프레임에서 b[09]=0x52, b[23]=0x6d, b[24]=0x75, b[25]=0x77
When  온도를 변환하면
Then  설정온도 = 0x52 - 0x3C = 22.0도C
And   실내온도 = (0x6d - 0x40) / 2.0 = 22.5도C
And   흡입온도 = (0x75 - 0x40) / 2.0 = 26.5도C
And   토출온도 = (0x77 - 0x40) / 2.0 = 27.5도C
```

### AC-M1-10: 물리 범위 초과 감지

```gherkin
Given TYPE-B 프레임에서 설정온도 raw = 0x30 (변환: -12도C, 범위 밖)
When  물리 범위를 검증하면
Then  경고 로그가 기록되어야 한다
And   프레임 자체는 폐기되지 않아야 한다 (rangeOk=false 표시)
```

### AC-M1-11: TYPE-A SEQ=02 외기온도 추출

```gherkin
Given TYPE-A SEQ=02 프레임에서 b[14]=0x6b, b[15]=0x5c
When  외기온도를 변환하면
Then  외기온도A = (0x6b - 0x40) / 2.0 = 21.5도C
And   외기온도B = (0x5c - 0x40) / 2.0 = 14.0도C
```

### AC-M1-12: 동기화 복구

```gherkin
Given 바이트 스트림에 잘못된 바이트 (0x00, 0xFF, 0x33) 뒤에 유효한 TYPE-A 프레임이 있을 때
When  프레임 파서가 바이트를 읽으면
Then  잘못된 바이트를 스킵하고 유효한 프레임을 추출해야 한다
```

---

## M2: LG HVACR-01 에이전트

### AC-M2-01: 에이전트 초기화

```gherkin
Given serial_port="/dev/ttyUSB0", baud_rate=1200 설정이 주어졌을 때
When  NewHvacr01Agent를 호출하면
Then  에이전트가 Running 상태로 초기화되어야 한다
And   Type()이 "lg_hvacr01"를 반환해야 한다
```

### AC-M2-02: 기본 보레이트 1200

```gherkin
Given baud_rate 설정이 생략되었을 때
When  설정을 파싱하면
Then  BaudRate가 1200이어야 한다 (9600이 아님)
```

### AC-M2-03: 캡처 루프 TYPE-A 처리

```gherkin
Given 에이전트가 시작되어 캡처 루프가 동작 중일 때
When  유효한 TYPE-A SEQ=02 프레임이 수신되면
Then  device_state 메시지 (msg.Type="device_state.change" 등) 가 생성되어야 한다 (이전 schema: `lgcnp_odu_frame`, v1.6.8 부터 통일)
And   외기온도 파싱 결과가 포함되어야 한다
And   framesCaptured 카운터가 증가해야 한다
And   oduFramesCaptured 카운터가 증가해야 한다
```

### AC-M2-04: 캡처 루프 TYPE-B 처리

```gherkin
Given 에이전트가 시작되어 캡처 루프가 동작 중일 때
When  유효한 TYPE-B IDU#3 프레임이 수신되면
Then  device_state 메시지 (msg.Type="device_state.change" 등) 가 생성되어야 한다 (이전 schema: `lgcnp_idu_frame`, v1.6.8 부터 통일)
And   온도값 4종(설정/실내/흡입/토출)이 포함되어야 한다
And   iduFramesCaptured 카운터가 증가해야 한다
And   IDU#3 디바이스 상태가 갱신되어야 한다
```

### AC-M2-05: get_stats 커맨드

```gherkin
Given 에이전트가 동작 중이고 10개 프레임을 캡처했을 때
When  Process({"command":"get_stats"})를 호출하면
Then  frames_captured >= 10인 JSON이 반환되어야 한다
And   odu_frames_captured, idu_frames_captured 필드가 포함되어야 한다
And   transport_connected 필드가 포함되어야 한다
```

### AC-M2-06: get_recent 커맨드

```gherkin
Given 링 버퍼에 5개 프레임(seq=1~5)이 있을 때
When  Process({"command":"get_recent","count":3,"last_seq":2})를 호출하면
Then  seq=3,4,5인 프레임 3개가 반환되어야 한다
And   최신순(5,4,3)이 아니라 시간순으로 정렬 가능해야 한다
```

### AC-M2-07: 라이프사이클 전이

```gherkin
Given 에이전트가 Running 상태일 때
When  Pause를 호출하면
Then  상태가 Paused로 전이되어야 한다
And   캡처 루프가 일시 정지되어야 한다

When  Resume을 호출하면
Then  상태가 Running으로 복귀해야 한다
And   캡처 루프가 재개되어야 한다

When  Stop을 호출하면
Then  상태가 Stopped로 전이되어야 한다
And   트랜스포트가 닫혀야 한다
```

### AC-M2-08: 재연결 루프

```gherkin
Given 에이전트가 동작 중이고 트랜스포트 연결이 끊어졌을 때
When  재연결 루프가 시작되면
Then  지수 백오프(5s, 10s, 20s, ...)로 재연결을 시도해야 한다
And   모든 디바이스가 오프라인으로 전환되어야 한다
And   재연결 성공 시 캡처 루프가 재시작되어야 한다
```

---

## M3: 디바이스 관리

### AC-M3-01: IDU 자동 발견

```gherkin
Given auto_discovery=true 설정일 때
When  TYPE-B IDU_ADDR=0x83 프레임이 최초 수신되면
Then  주소 "83"의 IDU 디바이스가 등록되어야 한다
And   Label이 "indoor-3"이어야 한다
And   Type이 "indoor"이어야 한다
And   Online이 true여야 한다
```

### AC-M3-02: ODU 디바이스 등록

```gherkin
Given auto_discovery=true 설정일 때
When  TYPE-A 프레임이 최초 수신되면
Then  주소 "odu"의 ODU 디바이스가 등록되어야 한다
And   Type이 "outdoor"이어야 한다
```

### AC-M3-03: 디바이스 상태 갱신

```gherkin
Given IDU#1이 등록되어 있을 때
When  TYPE-B IDU#1 프레임에서 설정온도=22도C, 실내온도=22.5도C가 수신되면
Then  디바이스 상태의 target_temp이 22.0이어야 한다
And   current_temp이 22.5이어야 한다
```

### AC-M3-04: 오프라인 감지

```gherkin
Given IDU#2가 온라인이고 OfflineTimeout=30s일 때
When  30초 동안 IDU#2 패킷이 수신되지 않으면
Then  IDU#2의 Online이 false로 전환되어야 한다
And   onDeviceStateChange 콜백이 호출되어야 한다
```

### AC-M3-05: DeviceProvider 인터페이스

```gherkin
Given 에이전트에 3개 IDU와 1개 ODU가 등록되어 있을 때
When  DeviceProvider().ListDevices()를 호출하면
Then  4개 디바이스가 반환되어야 한다
And   각 디바이스에 ID, Name, Properties가 포함되어야 한다
```

### AC-M3-06: 통일 속성명

```gherkin
Given IDU 디바이스 상태에 온도값이 있을 때
When  toProperties()를 호출하면
Then  속성명이 current_temp, target_temp을 포함해야 한다 (NASA/LGCP와 통일)
And   inlet_temp, outlet_temp이 추가로 포함되어야 한다 (LG ICP-01 전용)
```

---

## M4: 플로우 노드

### AC-M4-01: lg_hvacr01_status 노드 폴링

```gherkin
Given lg_hvacr01_status 노드가 agent_ref="my-hvacr01"로 설정되었을 때
When  Init을 호출하고 폴링 루프가 시작되면
Then  에이전트에서 get_recent로 프레임을 수신해야 한다
And   각 프레임이 개별 메시지로 SourceCh에 출력되어야 한다
And   메타데이터에 node_source="poll_bulk"이 설정되어야 한다 (v1.11.0 부터 prefix-less)
```

### AC-M4-02: lg_hvacr01_status 노드 agent_ref 검증

```gherkin
Given agent_ref가 비어있을 때
When  Configure를 호출하면
Then  ErrHvacr01MissingAgentRef 에러가 반환되어야 한다
```

### AC-M4-03: lg_hvacr01_control 미지원 응답

```gherkin
Given lg_hvacr01_control 노드가 초기화되었을 때
When  제어 메시지 (power=true)를 Process에 전달하면
Then  {"status":"not_supported","message":"LG ICP-01 control commands not yet discovered"} 응답이 반환되어야 한다
```

### AC-M4-04: lg_hvacr01 통합 노드 상태 조회

```gherkin
Given lg_hvacr01 통합 노드가 초기화되었을 때
When  제어 키가 없는 메시지를 Process에 전달하면
Then  상태 조회(get_stats 또는 get_recent)가 수행되어야 한다
```

### AC-M4-05: lg_hvacr01 통합 노드 제어 시도

```gherkin
Given lg_hvacr01 통합 노드가 초기화되었을 때
When  제어 키(power, temperature 등)가 포함된 메시지를 Process에 전달하면
Then  "제어 미지원" 응답이 반환되어야 한다
```

### AC-M4-06: 에이전트 타입 검증

```gherkin
Given agent_ref가 LGCP 에이전트(lgcp 타입)를 가리킬 때
When  lg_hvacr01_status 노드가 initAgent를 호출하면
Then  에이전트 타입 불일치 에러가 반환되어야 한다
```

---

## M5: Web UI 스키마

### AC-M5-01: 에이전트 타입 등록

```gherkin
Given agentSchemas.ts의 AGENT_TYPES 배열
When  UI에서 에이전트 생성 폼을 열면
Then  "LG HVACR-01" 옵션이 표시되어야 한다
```

### AC-M5-02: 에이전트 설정 필드

```gherkin
Given lg_hvacr01 에이전트 타입을 선택했을 때
When  설정 폼이 렌더링되면
Then  serial_port (필수), baud_rate (기본: 1200) 필드가 표시되어야 한다
And   baud_rate 기본값이 1200이어야 한다 (9600이 아님)
```

### AC-M5-03: 노드 스키마 등록

```gherkin
Given nodeSchemas.ts의 노드 타입 목록
When  UI에서 노드 생성 폼을 열면
Then  lg_hvacr01_status, lg_hvacr01_control, lg_hvacr01 노드 타입이 표시되어야 한다
And   agent_select 필드의 옵션에 'lg_hvacr01'가 포함되어야 한다
```

---

## M6: 타입 등록

### AC-M6-01: 에이전트 타입 등록

```gherkin
Given 에이전트 매니저가 초기화될 때
When  RegisterHvacr01Types가 호출되면
Then  "lg_hvacr01" 타입이 매니저에 등록되어야 한다
And   해당 타입으로 에이전트 생성이 가능해야 한다
```

### AC-M6-02: 노드 타입 등록

```gherkin
Given 노드 레지스트리가 초기화될 때
When  등록 테이블이 로드되면
Then  "lg_hvacr01_status", "lg_hvacr01_control", "lg_hvacr01" 노드가 등록되어야 한다
And   각 노드의 카테고리가 "io"이어야 한다
```

---

## 품질 게이트 (Definition of Done)

### 필수 통과 조건

- [ ] 모든 인수 기준(AC-*) 테스트 통과
- [ ] `go test -race ./internal/agent/lg/...` 통과
- [ ] `go test -race ./internal/node/...` 통과
- [ ] 테스트 커버리지 85% 이상 (신규 파일 기준)
- [ ] `go vet ./...` 경고 없음
- [ ] 프로토콜 분석 보고서의 캡처 데이터로 통합 테스트 통과
- [ ] Web UI에서 lg_hvacr01 에이전트 생성/삭제 정상 동작

### 선택 통과 조건

- [ ] 변화율 검증(계층 5) 구현 및 테스트
- [ ] SEQ 순서 연속성(계층 6) 검증 구현
- [ ] 실제 LRD-N837T 장비 연동 테스트
