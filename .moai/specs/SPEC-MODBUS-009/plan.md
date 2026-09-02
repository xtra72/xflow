# SPEC-MODBUS-009 — 구현 계획 (Implementation Plan)

> 대상: `modbus-client`(type id, 패키지 `internal/agent/modbus/`) 디바이스 관리 UX 재구성.
> 방법론: hybrid — 신규 코드는 TDD(RED-GREEN-REFACTOR), 기존 파일 편집은 DDD(특성화/행위 보존). 커버리지 ≥85%, `code_comments: ko`.
> Tier: **M** (프론트+백엔드, 5~15 파일, 300~1000 LOC 예상) → 산출물 3파일(spec/plan/acceptance).

관련: SPEC-MODBUS-006(transport 선택), SPEC-MODBUS-008(런타임 add/remove·per-device transport·session sharing·frame log — 본 SPEC이 활용·확장).

---

## 1. 기술 접근 (Technical Approach)

### 백엔드 (Go, `internal/agent/modbus/`)

- **명령 표면 확장**: `Process`(agent.go:946) 스위치에 `list_devices`·`update_device` 두 case를 기존 스타일(1 동사 = 1 case + `processXxx` 핸들러)로 추가. add_device(973)/remove_device(975) 바로 아래에 자연 배치.
- **`list_devices`(F1)**: `a.mu` RLock 스냅샷 조회. 부작용 없음. 응답 형태는 프론트가 이미 소비하는 형태(AgentDetailPanel.tsx:3617의 `data[]` — device_id/address/source)와 정렬하되, modbus-client 고유 메타(host/port/unit_id/transport/register_groups/share_session)를 확장 필드로 포함. modbus-gateway `list_devices`(modbusserver/agent.go:325)를 응답 스키마 참조로 사용.
- **`update_device`(F2)**: `set_config`(set_config.go:33)의 디바이스-스코프 재구성 로직을 재사용/공통화. 흐름: params 파싱 → `a.mu.Lock` → `findDeviceLocked`(runtime_device.go:216) 조회(미존재 → `ErrDeviceNotFound`) → `rejectInitOnlyFields`(set_config.go:204)로 금지 필드 거부 → register_groups/unit_id/케이던스 in-place 변경 → copy-on-write clone(runtime_device.go:317~)으로 통계/캐시 정합. **연결/트랜스포트 재생성 금지**.
- **중복 회피**: `set_config`와 `update_device`가 공유하는 재구성 로직은 공통 헬퍼(예: `applyDeviceReconfigLocked`)로 추출하여 단일 소스화. set_config는 특성화 테스트로 행위 보존.

### 프론트엔드 (TypeScript/React, `web/src/`)

- **F3(a) 설정 탭 숨김**: `ConfigTab`(AgentDetailPanel.tsx:769) 또는 `FormField.tsx`(337, `modbus_devices` 렌더)에서 agentType === 'modbus-client' && field.name === 'devices'일 때 렌더 스킵. 최소 침습 지점 선택(다른 타입·필드 무영향). 스키마(agentSchemas.ts:106)는 유지.
- **F3(b) 장치 탭 섹션**: `DevicesTab`(AgentDetailPanel.tsx:3570)에 modbus-client 분기 추가(3663 modbus-gateway / 3669 xsfm 분기와 동일 스타일). 신규 컴포넌트 `ModbusClientDevicesSection`을 `ModbusDevicesSection`(1255) 구조 참조로 작성. list_devices 목록 + add/remove/update UI. 편집 필드는 `ModbusDevicesEditor`(ModbusDevicesEditor.tsx:1248) 재사용. exec는 `useExecAgent`(useAgent.ts:145), 제거는 useDevice.ts:247 패턴 참조.
- **i18n**: `agents.detail.devices.*` 키 확장(ko.json 및 대응 로케일). 하드코딩 금지.

---

## 2. 재사용 맵 (Reuse Map — file:line)

| 목적 | 재사용/참조 대상 (file:line) | 활용 방식 |
|------|------------------------------|-----------|
| 명령 디스패치 | `internal/agent/modbus/agent.go:946` (`Process`), `:953-975` (스위치) | list_devices/update_device case 추가 |
| add/remove 라이프사이클 | `internal/agent/modbus/runtime_device.go:35`(add), `:140`(remove), `:216`(findDeviceLocked), `:232`(buildRuntimeDeviceLocked), `:317~`(clone 헬퍼) | update_device 조회·정합에 재사용 |
| 재구성 로직 | `internal/agent/modbus/set_config.go:33`(processSetConfig), `:153`(그룹 스케줄러), `:204`(rejectInitOnlyFields), `:223`(parseSetConfigRegisterGroups) | update_device 재구성 로직 공통화 |
| 디바이스 설정 | `internal/agent/modbus/config.go:50`(DeviceConfig), `:55`(RegisterGroups), `:58`(Transport), `:65`(ShareSession) | 필드 파싱/검증 |
| 오류 규약 | `internal/agent/modbus/runtime_device.go` (ErrMissingDeviceID/ErrDuplicateDevice/ErrDeviceNotFound) | update_device 거부 |
| 응답 스키마 참조 | `internal/agent/modbusserver/agent.go:325` (gateway list_devices) | list_devices 응답 형태 정렬 |
| 설정 탭 | `web/src/pages/agents/AgentDetailPanel.tsx:769` (ConfigTab), `web/src/components/property/FormField.tsx:337` (modbus_devices 렌더), `web/src/config/agentSchemas.ts:106` (devices 필드) | modbus-client devices 숨김 |
| 장치 탭 | `web/src/pages/agents/AgentDetailPanel.tsx:3570` (DevicesTab), `:1255`/`:3663` (ModbusDevicesSection 참조), `:3669` (xsfm 분기 스타일) | modbus-client 섹션 분기·참조 |
| 편집 폼 | `web/src/components/property/ModbusDevicesEditor.tsx:1248` | 디바이스 추가/수정 필드 재사용 |
| exec 훅 | `web/src/hooks/useAgent.ts:145` (useExecAgent), `web/src/hooks/useDevice.ts:70`(realtime)/`:239`(delete)/`:247`(remove_device) | CRUD 명령 발행 |
| 기존 list_devices 소비 | `web/src/pages/agents/AgentDetailPanel.tsx:3617` | 응답 형태 소비 패턴 참조 |

---

## 3. 마일스톤 (Milestones — 우선순위 기반, 시간 예측 없음)

의존 순서: 백엔드 명령(M1→M2)이 선행되어야 프론트 섹션(M4)이 실제 데이터로 동작. 설정 탭 숨김(M3)은 독립적.

### M1 — 백엔드 `list_devices` (우선순위: High / 1차 목표)
- `Process` 스위치에 case 추가 + `processListDevices` 핸들러(RLock 스냅샷).
- 응답 스키마 확정(gateway 형태 정렬 + modbus-client 확장 필드).
- 단위 테스트(TDD): 다중 디바이스, 0-device, RLock 무변형.

### M2 — 백엔드 `update_device` (우선순위: High / 1차 목표)
- 공통 재구성 헬퍼 추출(set_config와 공유).
- `processUpdateDevice` 핸들러: 조회→금지필드 거부→in-place 변경→copy-on-write 정합.
- 단위 테스트(TDD): 정상 수정(연결 유지 확인), 미존재 ID 거부(원자성), init 전용 필드 거부, register_groups 변경, `-race`.
- set_config 특성화 테스트로 행위 보존 확인(DDD).

### M3 — 프론트 설정 탭 숨김 (우선순위: Medium / 2차 목표)
- ConfigTab/FormField 렌더 게이팅(modbus-client devices 스킵).
- vitest: modbus-client에서 devices 미렌더, modbus-gateway 등 무영향.

### M4 — 프론트 장치 탭 modbus-client 섹션 (우선순위: High / 2차 목표)
- `ModbusClientDevicesSection` 신규(참조: ModbusDevicesSection).
- list_devices 목록 + add/remove/update UI, ModbusDevicesEditor 필드 재사용.
- DevicesTab 분기 추가(기존 분기 불변).
- i18n 키 확장.
- vitest: 목록 렌더, add/remove/update exec 호출 검증, i18n 키 존재.

### M5 — 통합·품질·회귀 (우선순위: High / 최종 목표)
- 백엔드 `go test -race ./internal/agent/modbus/...` 클린 + 커버리지 ≥85%.
- 프론트 `tsc` + `vitest` 통과.
- 하위 호환 회귀: 기존 명령·폴링·transport·session sharing 동작 보존(특성화).
- i18n 정합(누락 키 0), go.mod 신규 의존 0.

---

## 4. 아키텍처 설계 방향 (Architecture)

- **명령 표면**: 단일 `Process` 문자열 스위치 유지. 신규 병렬 메커니즘 금지. list_devices=조회(RLock), update_device=변경(Lock, copy-on-write).
- **재구성 단일 소스**: set_config ↔ update_device의 register_groups/케이던스 재구성 로직을 공통 헬퍼로 추출 → 이중 유지 방지.
- **프론트 관심사 분리**: 설정 탭=스키마 주도(숨김만), 장치 탭=전용 섹션(라이프사이클 CRUD). modbus-client의 디바이스 관리 단일 진입점을 장치 탭으로 통합.
- **참조 패턴 준수**: modbus-gateway `ModbusDevicesSection`을 구조 참조하여 UI 일관성 확보.

---

## 5. 위험 및 대응 (Risks & Mitigations)

| 위험 | 영향 | 대응 |
|------|------|------|
| update_device와 set_config 로직 중복/드리프트 | 유지보수 부담·행위 불일치 | 공통 헬퍼 추출 + set_config 특성화 테스트 |
| update_device의 그룹/통계 갱신이 폴링 goroutine과 경합 | -race 실패·데이터 경합 | SPEC-008 copy-on-write 패턴 준수, a.mu Lock 하 통째 교체 |
| 설정 탭 숨김이 다른 agentType/필드에 부작용 | 회귀 | agentType+field.name 정확 게이팅, vitest 회귀 |
| list_devices 응답 형태와 프론트 소비 형태 불일치 | 목록 미표시 | AgentDetailPanel.tsx:3617 소비 형태·gateway 스키마와 사전 정렬 |
| i18n 키 누락 | 하드코딩/미번역 | 기존 `agents.detail.devices.*` 확장, 키 존재 테스트 |
| 실제 하드웨어 RTU 타이밍(잔여) | 실환경 편차 | 본 SPEC 범위 밖(Non-Goal), 실장비 검증은 별도 |

---

## 6. 완료 정의 연결 (Definition of Done → acceptance.md)

M1~M5 완료 = acceptance.md의 AC-01~AC-09 전량 충족 + 품질 게이트(§5 REQ-05) 통과. 상세 Given/When/Then은 acceptance.md 참조.
