# SPEC-MODBUS-011 수용 기준 (acceptance.md)

Given/When/Then 형식. 모든 AC는 프론트엔드 단위 테스트(`vitest`) 또는 수동 UI 검증으로 확인한다.

---

## REQ-MODBUS-011-01 — Client 붙여넣기 파서

### AC-01 — 다중 행 그룹핑 및 방출 형상
- **Given** client 일괄 등록 텍스트(2개 그룹, 1개 디바이스)
  ```
  host,port,unit_id,fc,address,count,data_type,polling_interval,comment
  192.168.0.10,502,1,3,0,10,uint16,5s,온도
  ,,,1,0,8,uint16,,도어
  ```
- **When** `parseModbusClientBulk(text)`를 호출하면
- **Then** 첫 줄은 헤더로 스킵되고, 2행이 디바이스를 시작하며 3행(빈 신원)이 그룹을 이어 붙여 1개의 유효
  `EmittedDevice`가 산출된다.
  - `{ host:'192.168.0.10', port:502, unit_id:1, register_groups:[{function_code:3,start_address:0,quantity:10,data_type:'uint16',poll_interval:'5s',name:'온도'},{function_code:1,start_address:0,quantity:8,data_type:'uint16',name:'도어'}] }` (id 키 없음).
- **And** `failures`(BulkFailure)는 비어 있다.

### AC-02 — 필수/무효 필드 행 실패 집계
- **Given** 텍스트
  ```
  ,502,1,3,0,1
  192.168.0.20,502,999,3,0,1
  192.168.0.21,502,3,9,0,1
  ```
- **When** 파싱하면
- **Then** 1행은 host 누락(tcp 상속, `EMPTY_REQUIRED`)으로, 2행은 unit_id 범위 초과(>247, `INVALID_UNIT_ID`)로,
  3행은 무효 fc(9, `INVALID_FC`)로 각각 원본 줄 번호를 가진 `BulkFailure`로 집계되고, 유효 device는 0개다
  (유효 그룹 0개 디바이스는 방출 안 함, 백엔드 미호출).

---

## REQ-MODBUS-011-02 — Gateway 붙여넣기 파서

### AC-03 — 다중 세그먼트 그룹핑 + shared 세그먼트
- **Given** gateway 텍스트(2개 세그먼트, 1개 디바이스; 2번째가 shared)
  ```
  unit_id,name,fc,address,count,data_type,comment
  1,meter-A,1,0,8,uint16,도어 센서
  ,,3,0,10,uint16,shared,200,펌프 상태
  ```
- **When** `parseModbusGatewayBulk(text)`를 호출하면
- **Then** 2행이 디바이스를 시작하고 3행(빈 신원)이 shared 세그먼트를 이어 붙여 1개의 유효 params가 산출된다.
  - `{ unit_id:1, name:'meter-A', register_map:{ coils:[{address:0,count:8,data_type:'uint16',description:'도어 센서'}], holding_registers:[{address:0,count:10,shared_address:200,description:'펌프 상태'}] } }` (shared 세그먼트는 data_type 미방출).

### AC-04 — 세그먼트 필수/무효 행 실패
- **Given** 텍스트
  ```
  1,NoSeg
  2,BadFc,9,0,1
  3,BadCount,1,0,0
  4,BadShared,3,0,10,uint16,shared,xx
  ```
- **When** 파싱하면
- **Then** 1행은 세그먼트 없음(`EMPTY_SEGMENTS`), 2행은 무효 fc(9, `INVALID_FC`), 3행은 count<1(`INVALID_SEGMENT`),
  4행은 비정수 shared_address(`INVALID_SHARED_ADDRESS`)로 각각 `BulkFailure`로 집계되고 유효 params는 0개다
  (백엔드 register_map 필수 위반 사전 차단).

---

## REQ-MODBUS-011-03 — 일괄 등록 실행 (best-effort)

### AC-05 — 전량 성공
- **Given** 유효 client 행 3개와 성공하는 `add_device` exec
- **When** `useModbusClientBulkAdd`로 제출하면
- **Then** `add_device`가 3회 순차 호출되고 `BulkResult { total:3, ok:3, failed:[] }`가 반환된다.
- **And** 목록 갱신(list_devices refetch)은 처리 후 **1회만** 발생한다.

### AC-06 — 부분 성공(중복 ID·검증 실패 행 스킵)
- **Given** 유효 행 3개 중 2번째 `add_device` exec가 중복 ID 오류를 던짐
- **When** 제출하면
- **Then** 1·3번은 등록되고 2번은 실패로 수집되어 `BulkResult { total:3, ok:2, failed:[{line, reason}] }`가
  반환된다(중단 없이 계속 진행). 실패 사유는 백엔드 오류 메시지 원문이다.

### AC-07 — 파서 실패 + 백엔드 실패 합류
- **Given** 파서가 1개 행을 무효로 집계하고 나머지 2개 유효 행 중 1개가 백엔드 실패
- **When** 제출하면
- **Then** 최종 `failed`에는 파서 실패 1건 + 백엔드 실패 1건이 모두 포함되고 `ok:1`이다.

---

## REQ-MODBUS-011-04 — UI 통합

### AC-08 — 두 장치탭 진입점
- **Given** modbus-client 장치탭(`ModbusClientDevicesSection`)과 modbus-gateway 장치탭
  (`ModbusDevicesSection`)
- **When** 각 탭을 렌더링하면
- **Then** 각 탭에 일괄 등록 토글 버튼이 있고, 클릭 시 `BulkRegisterPanel`(textarea + formatHint +
  제출 + 실패 목록)이 표시된다.

### AC-09 — 기존 흐름 불변 + i18n
- **Given** 일괄 등록 기능 추가 후
- **When** 기존 단일 add/update/remove를 수행하면
- **Then** 기존 동작이 회귀 없이 유지된다.
- **And** 일괄 등록 패널의 모든 문자열(토글/제목/placeholder/formatHint/제출/토스트/실패 사유)은
  `agents.detail.devices.*` i18n 키로 표기되며 하드코딩 문자열이 없다.

---

## REQ-MODBUS-011-05 — 품질·하위 호환

### AC-10 — 백엔드 미변경
- **Given** 본 SPEC 구현 완료 후
- **When** `git diff`로 백엔드 Go 변경을 확인하면
- **Then** `internal/agent/modbus/**`·`internal/agent/modbusserver/**`에 변경이 0건이다
  (기존 `add_device` 재사용만). 에이전트 type id `modbus-client`/`modbus-gateway` 불변.

### AC-11 — 프론트 품질 게이트
- **Given** 신규/변경 프론트 코드
- **When** `tsc`(타입 체크)와 `vitest`(단위 테스트)를 실행하면
- **Then** 둘 다 에러 0으로 통과한다.
- **And** 두 파서는 순수 함수로 단위 테스트가 존재한다.

### AC-12 — 의존성·i18n 정합
- **Given** 구현 완료 후
- **When** `package.json` 및 i18n 파일을 검사하면
- **Then** 신규 npm 의존성이 0건이고, `ko.json`과 `en.json`에 추가된 `bulk.*` 키 집합이 서로 정합한다
  (한쪽에만 있는 키 없음).

---

## Definition of Done

- [ ] AC-01 ~ AC-12 전부 통과.
- [ ] `useModbusBulk.ts`(파서 2 + 실행 훅 2) 및 `useModbusBulk.test.ts` 추가, vitest 통과.
- [ ] `AgentDetailPanel.tsx` 두 섹션에 패널 배선, 기존 CRUD 회귀 없음.
- [ ] `ko.json`/`en.json` `agents.detail.devices.bulk.*` 키 정합 추가.
- [ ] `tsc` 클린, 하드코딩 문자열 0, 백엔드 diff 0.
- [ ] 코드 주석 한국어.
