# SPEC-MODBUS-007 인수 기준 (acceptance.md)

> 형식: Given / When / Then. 요구사항 M1~M5 대응. 입력 계약은 `modbus-read` 출력(`{success, values[], agent_type}`, 엔트리 `{index, area, address, count, values[]|raw, data_type?}`, unit_id 없음).

## AC-01 — Happy path: 엔트리 전체 remap (주소 + area + device_id 재작성)

- **Given** `modbus-read`가 엔트리 `{index:0, area:"holding_registers", address:100, count:5, values:[10,20,30,40,50]}` 를 방출하고,
  remap 규칙이 `{source_area:"holding_registers", source_address:100, count:5, target_unit_id:3, target_area:"input_registers", target_address:200}` 로 설정됨.
- **When** `modbus-remap` `Process` 실행.
- **Then** 출력 엔트리는 `{area:"input_registers", address:200, count:5, values:[10,20,30,40,50], unit_id:3}` 이고, 값 배열은 위치 보존, `success == true`.
  (M2/M3, `targetAddr = 200 + (100 − 100) = 200`, count 보존, area·device_id 재작성.)

## AC-02 — 템플릿 적용 → 인스턴스화된 규칙이 정상 remap

- **Given** 노드 config에 템플릿 `{area:"holding_registers", offset:1000}` 저장, 적용 파라미터 `(device_id:7, start:100, count:3)`.
  그리고 `modbus-read`가 엔트리 `{area:"holding_registers", address:100, count:3, values:[1,2,3]}` 방출.
- **When** 템플릿을 인스턴스화하여 규칙 `{source_area:"holding_registers", source_address:100, count:3, target_unit_id:7, target_area:"holding_registers", target_address:1100}` 생성 후 `Process` 실행.
- **Then** 출력 엔트리는 `{area:"holding_registers", address:1100, count:3, values:[1,2,3], unit_id:7}`, `success == true`.
  (M4, `target_address = start + offset = 100 + 1000 = 1100`, target_area = source_area(별도 지정 없음), unit_id = device_id.)

## AC-03 — 미매칭/경계 → 거부 (success=false + error port)

- **Given** `modbus-read`가 엔트리 `{area:"holding_registers", address:100, count:5, ...}` 하나만 방출하고,
  규칙 소스가 `{source_area:"holding_registers", source_address:100, count:10}` (count 불일치, 정확 매칭 실패).
- **When** `Process` 실행.
- **Then** 부분 매핑 없이 해당 규칙을 거부하고, 규칙별 error를 표기하며 전체 `success == false`, 결과를 error port로 라우팅.
  (M3, 엔트리 전체 정확 매칭 실패 → `ErrAddressNotMapped` 의미 준거.)

## AC-04 — 값 보존 불변식

- **Given** 임의의 매칭 규칙과 엔트리(values 또는 raw 포함).
- **When** `Process`로 remap 수행.
- **Then** 출력 엔트리의 값 배열(values) 또는 raw는 입력과 바이트/순서 동일(주소·영역·unit_id만 재작성, 값 자체 변형 없음).
  (M2 Unwanted.)

## AC-05 — 비-modbus-read 페이로드 거부

- **Given** `values[]` 스키마가 아닌 임의 페이로드 입력.
- **When** `Process` 실행.
- **Then** 추측성 매핑 없이 error(`return (nil, err)`)로 error port 라우팅.
  (M1 Unwanted.)

## AC-06 — payload 오버라이드가 config 기본을 이김

- **Given** 노드 config에 기본 `rules` A 설정, payload에 `rules` B 존재.
- **When** `Process` 실행.
- **Then** config A 대신 payload B 규칙으로 remap 수행.
  (M4 State-driven, `command_set` 오버라이드 패턴.)

## AC-07 — nodeSchemas 엔트리 + 노드 등록

- **Given** 빌드된 백엔드/프론트.
- **When** 노드 목록(`/nodes`, registry) 및 노드 config 폼 조회.
- **Then** `modbus-remap`이 `modbus` 카테고리로 등록·노출되고, config 폼이 `RegisterRemapEditor` 필드를 렌더.
  (M5, F1~F2.)

## AC-08 — From→To 2-테이블 편집기

- **Given** 노드 property 패널.
- **When** `RegisterRemapEditor` 표시.
- **Then** 좌(From: area/address/count)·우(To: device_id/area/address) 2-테이블이 나란히 표시되고, 규칙 추가/편집/삭제 및 템플릿 정의(area/offset)·적용(device_id/start/count)이 동작하며, `toRows`/`toEmit` 라운드트립이 값 손실 없이 유지.
  (M5 Optional, F3.)

---

## 엣지 케이스

- **다중 엔트리 × 다중 규칙**: 여러 read-op 엔트리와 여러 규칙이 동시에 존재할 때 각 규칙이 정확히 하나의 엔트리에 매칭(1:1), 매칭된 것만 remap.
- **area 변경**: `holding_registers`→`input_registers` 등 area 재지정이 출력 엔트리에 반영.
- **payload 오버라이드**: AC-06 확장 — `templates` 오버라이드도 동일하게 config 기본을 대체.
- **빈 규칙 셋**: config·payload 모두 규칙 없음 → 명확한 error(빈 규칙 셋).
- **boolean area(coils/discrete_inputs)**: boolean 값 표현이 remap 후에도 보존(data_type 무시 영역).

---

## 품질 게이트 기준 (Definition of Done)

- [ ] `go build ./...` 통과.
- [ ] `go test ./internal/node/...` 통과 (AC-01~06 커버).
- [ ] `modbus-remap` 노드가 registry(`/nodes`)에 `modbus` 카테고리로 등록.
- [ ] 프론트 web build / `tsc --noEmit` 클린.
- [ ] 노드 config 폼에서 From→To 2-테이블 편집 동작(AC-07~08).
- [ ] TRUST 5 품질 게이트(테스트/가독성/일관성/보안/추적성) 충족.
