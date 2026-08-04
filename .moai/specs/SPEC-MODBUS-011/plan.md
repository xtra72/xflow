# SPEC-MODBUS-011 구현 계획 (plan.md)

## 기술 접근

xsfm 장치탭의 검증된 일괄 등록 패턴(공용 `BulkRegisterPanel` + `parseDelimitedRows` + best-effort
`useBulkAddDevices`)을 두 modbus 장치탭에 이식한다. 백엔드는 기존 `add_device` exec를 행별로
반복 호출하므로 Go 코드 변경이 없다. 신규 코드는 (1) 두 순수 파서, (2) 두 best-effort 실행 훅,
(3) 두 섹션의 패널 배선, (4) i18n 확장이다.

## 재사용 지도 (file:line)

| 재사용 대상 | 위치 | 용도 |
| ----------- | ---- | ---- |
| `BulkRegisterPanel` (공용 UI) | `web/src/pages/agents/BulkRegisterPanel.tsx:14` | textarea + 제출 + formatHint + 실패 인라인 목록. props 그대로 사용. |
| `parseDelimitedRows` (행 파서) | `web/src/hooks/useStation.ts:89` | 개행 분리·\r 제거·탭/콤마 자동 감지·셀 trim·1-based line 보존. 두 파서의 1차 분해에 재사용. |
| `BulkFailure` / `BulkResult` / `EMPTY_REQUIRED` / `INVALID_INDEX` | `web/src/hooks/useStation.ts:63,70,77,80` | 실패/결과 타입·sentinel. modbus용 sentinel 추가 시 동일 패턴. |
| `useBulkAddDevices` (best-effort 실행 선례) | `web/src/hooks/useStation.ts:486` | 행별 add_device 순차 호출·실패 수집·1회 무효화 구조 복제. |
| xsfm 패널 배선(토글/상태/제출) | `web/src/pages/agents/XsfmDevicesTab.tsx:430,448-466` | showBulk 토글, bulkText 상태, submit, formatFailure 배선 참조. |
| client 단일 add 흐름 | `web/src/pages/agents/AgentDetailPanel.tsx:1538,1579-1602` | `ModbusClientDevicesSection` + `toEmitDevice(row, agentTransport)` + `add_device` exec. |
| client 방출 형상 · `toEmitDevice`/`EmittedDevice` | `web/src/components/property/ModbusDevicesEditor.tsx:129,327` | 파서 방출 형상 계약. `FC_TO_AREA`(56), `parseBulkGroups`(409)는 그룹 파싱 참조. |
| gateway 단일 add 흐름 | `web/src/pages/agents/AgentDetailPanel.tsx:1272` | `ModbusDevicesSection`(config.devices 편집 경로) — 일괄은 exec add_device 경로 신설. |
| gateway 방출 형상 · `EmittedDevice`/`register_map` | `web/src/components/property/ModbusServerDevicesEditor.tsx:149,389` | `{unit_id, name?, register_map}` 계약. |
| gateway 백엔드 `processAddDevice` | `internal/agent/modbusserver/agent.go:1232` | 필수 params 계약(unit_id 1-247, register_map 필수, name 선택). READ ONLY(미변경). |
| client 백엔드 `processAddDevice` | `internal/agent/modbus/runtime_device.go:35` | `parseDeviceConfig` 수용 형상 확인. READ ONLY(미변경). |
| i18n 네임스페이스 | `web/src/lib/i18n/ko.json`, `en.json` (`agents.detail.devices.*`) | 기존 modbus 키(`modbusAddSuccess` 등) 옆에 `bulk.*` 확장. `airDevices.bulk.*` 구조 참조. |

## 파일 계획

| 파일 | 변경 | 내용 |
| ---- | ---- | ---- |
| `web/src/hooks/useModbusBulk.ts` (신규) | 신규 | 순수 파서 `parseModbusClientBulk`/`parseModbusGatewayBulk`(한 행=그룹/세그먼트, 신원 컬럼 그룹핑) + 실행 훅 `useModbusClientBulkAdd`/`useModbusGatewayBulkAdd`. modbus sentinel(`INVALID_UNIT_ID`, `INVALID_PORT`, `INVALID_FC`, `INVALID_GROUP`, `INVALID_SEGMENT`, `INVALID_SHARED_ADDRESS`, `NO_CURRENT_DEVICE`, `EMPTY_SEGMENTS`) 정의. |
| `web/src/hooks/useModbusBulk.test.ts` (신규) | 신규 | 두 파서 순수 함수 단위 테스트(정상/헤더 스킵/다중 행 그룹핑·이어붙임/필수 누락/무효 fc·정수/shared 세그먼트/실패 행). |
| `web/src/pages/agents/AgentDetailPanel.tsx` | 편집 | `ModbusClientDevicesSection`·`ModbusDevicesSection`에 showBulk 토글·bulkText 상태·`BulkRegisterPanel` 배선·제출 핸들러 추가. 기존 흐름 불변. |
| `web/src/lib/i18n/ko.json` | 편집 | `agents.detail.devices.bulk.*`(client/gateway 공용 또는 분리) 키 추가. |
| `web/src/lib/i18n/en.json` | 편집 | ko와 정합하는 영문 키 추가. |

## 마일스톤 (우선순위 기반, 시간 예측 없음)

### 1차 목표 (Priority High) — 파서 + 테스트
- `useModbusBulk.ts`에 `parseModbusClientBulk`/`parseModbusGatewayBulk` 순수 함수 구현(TDD: 테스트 먼저).
- `useModbusBulk.test.ts` 작성 후 통과. 방출 형상이 `EmittedDevice`(client)·`{unit_id,name?,register_map}`(gateway) 계약과 일치.

### 2차 목표 (Priority High) — 실행 훅
- `useModbusClientBulkAdd`/`useModbusGatewayBulkAdd` best-effort 구현(행별 add_device 순차, 실패 수집, 1회 무효화).

### 3차 목표 (Priority Medium) — UI 통합
- 두 섹션에 토글·패널·제출·결과 토스트 배선. i18n ko/en 확장. 기존 단일 CRUD 흐름 회귀 없음 확인.

### 최종 목표 (Priority Medium) — 품질 게이트
- `tsc` 클린, `vitest` 클린. 하드코딩 문자열 0. ko/en 키 정합. 백엔드 diff 0.

## 아키텍처 방향

- 파서는 **순수 함수**(부수효과 없음)로 두어 단위 테스트 용이성 확보. UI/네트워크와 분리.
- 실행 훅은 파서 결과의 유효 행만 백엔드로 보내고, 파서 실패 + 백엔드 실패를 단일 `BulkFailure[]`로 합류.
- client/gateway 파서를 한 모듈(`useModbusBulk.ts`)에 공존시키되 방출 형상·필수 규칙만 분기.

## 위험 및 대응

| 위험 | 대응 |
| ---- | ---- |
| gateway `register_map` 백엔드 필수 → 세그먼트 없는 디바이스 등록 시도 | 파서가 유효 세그먼트 0개 디바이스를 `EMPTY_SEGMENTS`로 사전 실패 집계(백엔드 미호출). |
| 다중 행 그룹핑에서 이어붙임 행이 선행 디바이스 없이 나타남 | 신원 컬럼이 모두 빈 첫 데이터 행은 `NO_CURRENT_DEVICE`로 집계(디바이스 시작 행이 먼저 필요). 헤더는 첫 행 신원 셀 비정수로 자동 스킵. |
| 부분 성공 시 목록 다중 갱신으로 인한 깜빡임 | 모든 행 처리 후 **1회만** refetch/무효화(xsfm 선례 준수). |
| per-device 오버라이드·backing 미지원으로 인한 기대 불일치 | Non-Goal로 명시. 붙여넣기 후 개별 편집 다이얼로그로 보완. |

## Non-Goals

- 신규 백엔드 bulk 명령/일괄 원자성.
- 파일 업로드(붙여넣기 텍스트만).
- 범위/패턴 자동 생성(붙여넣기 표만).
- per-device transport/serial_port/share_session 오버라이드, gateway backing 세그먼트(초기 범위 제외). gateway shared 세그먼트는 v0.2.0에서 지원(`shared,<주소>` 두 셀 삽입).
