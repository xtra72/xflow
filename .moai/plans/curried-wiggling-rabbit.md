# Plan: NASA Agent Device Management Overhaul

## Context

NASA 에이전트의 디바이스 관리 방식을 정적(YAML 설정) → 동적(런타임 추가/제거)으로 전환.
현재 `device_addresses`는 필수 설정이며, 디바이스는 에이전트 설정 파일에 고정되어 있음.
동적 추가/제거 API(`add_device`/`remove_device`)는 이미 백엔드에 구현되어 있지만 프론트엔드 UI가 없음.

**요구사항:**
1. `device_addresses` 필수 → 선택으로 변경
2. Web UI 에이전트 설정에서 `device_addresses` 필드 제거
3. 에이전트 페이지 DevicesTab에 디바이스 추가/제거 UI 추가
4. 디바이스 변경 시 디바이스 페이지 실시간 반영
5. 디바이스 페이지에서도 디바이스 추가 가능 (에이전트 선택 → 디바이스 필드)

## Milestone 1: Backend - Config Relaxation

### 1.1 `device_addresses` 선택으로 변경

**File**: [config.go](internal/agent/samsung/config.go)

- Line 130: 주석 `(필수)` → `(선택)` 변경
- Lines 141-143: 삭제 (`device_addresses` 필수 검증 3줄)

### 1.2 add_device/remove_device params 폴백

**File**: [agent.go](internal/agent/samsung/agent.go)

`processAddDevice` (L591), `processRemoveDevice` (L652)에서 `req.Address`/`req.DeviceID`/`req.DeviceType`이 비어있으면 `req.Params`에서 폴백 읽기.

현재 API exec DTO는 `{ command, params }` 구조이므로 `address`, `device_id`, `device_type`이 params 내에 전달됨. 기존 직접 Process() 호출과의 하위 호환성 유지.

### 1.3 테스트 업데이트

**File**: [config_test.go](internal/agent/samsung/config_test.go)

- `TestParseNASAConfig_MissingDeviceAddresses` (L177): 에러 기대 → 성공 + 빈 배열 검증
- `TestParseNASAConfig_EmptyDeviceAddresses` (L189): 에러 기대 → 성공 + 빈 배열 검증

## Milestone 2: Frontend - 에이전트 설정 스키마 변경

**File**: [agentSchemas.ts](web/src/config/agentSchemas.ts)

- Line 98: `device_addresses` 항목 삭제 (SAMSUNG_NASA_FIELDS에서 제거)

## Milestone 3: Frontend - DevicesTab 디바이스 관리 UI

### 3.1 useExecAgent 쿼리 무효화 추가

**File**: [useAgent.ts](web/src/hooks/useAgent.ts)

`useExecAgent` (L115-120)에 `onSuccess` 콜백 추가: `queryClient.invalidateQueries({ queryKey: ['devices'] })`

### 3.2 DevicesTab 리라이트

**File**: [AgentDetailPanel.tsx](web/src/pages/agents/AgentDetailPanel.tsx)

기존 DevicesTab (L323-375, 읽기 전용) → 디바이스 관리 UI로 확장:

**구조:**
- 상단: "디바이스 추가" 버튼 (samsung-nasa 타입일 때만)
- 인라인 추가 폼: 주소(필수), 디바이스 ID(선택), 디바이스 타입(자동/실내기/실외기)
- 디바이스 카드: 소스 배지(설정/동적) + 삭제 버튼(동적 디바이스만)
- 하단 안내: "동적 디바이스는 에이전트 재시작 시 초기화됩니다"

**소스 정보 획득**: 마운트 시 `execAgent({ command: "list_devices" })` 호출하여 소스 맵 구성.
기존 DeviceInfo 타입에 source 필드가 없으므로, list_devices 응답으로 보완.

**시그니처 변경**: `DevicesTab({ agentId })` → `DevicesTab({ agentId, agentType })` (부모에서 전달)

**삭제 보호**: `source === "config"` 디바이스는 삭제 버튼 숨김 + 잠금 아이콘

## Milestone 4: Frontend - 디바이스 페이지에서 디바이스 추가

**File**: [DeviceListPage.tsx](web/src/pages/devices/DeviceListPage.tsx)

헤더에 "디바이스 추가" 버튼 추가 → 다이얼로그:

1. **Step 1**: 에이전트 선택 (samsung-nasa 타입 에이전트만 필터링)
2. **Step 2**: 디바이스 필드 입력 (주소, 디바이스 ID, 타입)
3. **Submit**: `useExecAgent` 호출 → `onSuccess`에서 devices 쿼리 무효화

## Files

| File | Action | Description |
|------|--------|-------------|
| [config.go](internal/agent/samsung/config.go) | Edit | device_addresses 선택으로 변경 |
| [agent.go](internal/agent/samsung/agent.go) | Edit | processAddDevice/RemoveDevice params 폴백 |
| [config_test.go](internal/agent/samsung/config_test.go) | Edit | 필수 검증 테스트 → 선택 검증으로 변경 |
| [agentSchemas.ts](web/src/config/agentSchemas.ts) | Edit | device_addresses 필드 삭제 |
| [useAgent.ts](web/src/hooks/useAgent.ts) | Edit | useExecAgent에 devices 쿼리 무효화 추가 |
| [AgentDetailPanel.tsx](web/src/pages/agents/AgentDetailPanel.tsx) | Edit | DevicesTab 디바이스 CRUD UI 추가 |
| [DeviceListPage.tsx](web/src/pages/devices/DeviceListPage.tsx) | Edit | 디바이스 추가 다이얼로그 추가 |

## Reusable Patterns

- `useExecAgent` (useAgent.ts L115): 에이전트 exec API 뮤테이션 — 디바이스 추가/제거에 재사용
- `processAddDevice` (agent.go L591): address, device_id, device_type 파라미터 구조
- `processRemoveDevice` (agent.go L652): address 또는 device_id로 디바이스 식별
- `DevicesTab` (AgentDetailPanel.tsx L323): 기존 디바이스 목록 UI 구조 — 확장

## Verification

1. `go test ./internal/agent/samsung/...` — 백엔드 테스트 통과
2. `go build ./cmd/xflowd` — 백엔드 빌드 확인
3. `npx tsc --noEmit` — 프론트엔드 타입 체크
4. device_addresses 없는 에이전트 YAML로 시작 가능 확인
5. 기존 device_addresses 있는 YAML도 정상 동작 확인 (하위 호환)
6. 에이전트 설정 페이지에서 device_addresses 필드 사라진 것 확인
7. DevicesTab에서 디바이스 추가/삭제 동작 확인
8. 디바이스 페이지에서 디바이스 추가 다이얼로그 동작 확인
9. 디바이스 추가/삭제 후 디바이스 목록 실시간 갱신 확인
