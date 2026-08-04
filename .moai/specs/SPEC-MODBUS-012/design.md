---
id: SPEC-MODBUS-012
title: "MODBUS Gateway 대시보드 패널 스위트 — 기술 설계"
version: "0.1.0"
status: draft
created: 2026-08-04
updated: 2026-08-04
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "web/src/pages/dashboard + internal/agent/modbusserver"
lifecycle: spec-anchored
tier: L
tags: "modbus, gateway, dashboard, design, architecture, backing-metrics, register-grid, concurrency"
---

# SPEC-MODBUS-012 — 기술 설계 (design.md)

## 1. 아키텍처 개요

```
[Dashboard] AddPanelDialog ──(needsAgent: modbus-gateway 필터)──▶ config{agentId[,unitId]}
     │
     ▼ renderDashboardPanel(switch)
 ┌───────────────────────────── 6 Panel Components ─────────────────────────────┐
 │ RealDevices  VirtualDevices  SharedRegisters  DeviceRegisters  BusStats  Summary │
 └───────┬───────────┬───────────────┬────────────────┬─────────────┬──────────┘
         │           │               │                │             │
   useExecAgent(agentId, {command, params})  ◀── dashboardRefreshInterval 폴링
         │           │               │                │             │
         ▼           ▼               ▼                ▼             ▼
   get_device_status  list_devices  get_map(u0)  get_device_status(uN)  get_status/list_clients
         │(backing*)                                                     
         ▼
 [Backend] agent.go Process switch ──▶ deviceManager / backedStore(atomic counters*)
```
`*` = 본 SPEC 신규.

- 프론트 패널은 **읽기 전용**이며 각자 폴링한다(공유 상태 없음, 기존 패널 모델과 동일).
- 레지스터 그리드는 공유 컴포넌트 `RegisterMapGrid` + 판정 유틸 `registerCellState`로 REQ-04/05가 재사용한다.
- 백엔드 변경은 백킹 관측 카운터 + 노출로 **국소화**한다.

## 2. 프론트엔드 설계

### 2.1 신규 PanelType 및 config (uiStore.ts)
```ts
// PanelType 유니온에 추가 (uiStore.ts:177-203)
| 'modbus-real-devices' | 'modbus-virtual-devices'
| 'modbus-shared-registers' | 'modbus-device-registers'
| 'modbus-bus-stats' | 'modbus-summary-stats'

// createDefaultPanel (uiStore.ts:319-446) case 예
case 'modbus-device-registers':
  return { type, title: '가상 디바이스 레지스터', config: { agentId: '', unitId: 0 } };
case 'modbus-shared-registers':
  return { type, title: '공유 레지스터 맵', config: { agentId: '' } };
// 나머지 4종: config { agentId: '' }
```
`panelDefaultSize`: 레지스터 맵 그리드 `{w:6,h:5}`, 목록 `{w:4,h:5}`, 버스 미니차트 `{w:6,h:3}`, 종합 바 `{w:8,h:2}`(목업 레이아웃 부합, 추후 조정 가능).

### 2.2 에이전트 선택 스텝 (AddPanelDialog.tsx)
- `PanelOption`에 `needsAgent?: boolean` 플래그 추가(`:76-96` 인터페이스 확장).
- `handleSelect`(`:217-251`)에 `needsAgent` 분기 → `setStep('agent')`.
- 신규 `AgentStep` 컴포넌트: `useAgents()` → `a.type === 'modbus-gateway'` 필터(`FacilityStep:833-837` 미러). 완료 시 `addPanelWithConfig(type, {agentId}, agentName)`.
- 가상 디바이스 레지스터 맵(`modbus-device-registers`)만 2차 `UnitStep`: 선택된 에이전트에 `execAgent(agentId,{command:'list_devices'})` → unit 셀렉트 → `{agentId, unitId}`.

### 2.3 공유 레지스터 그리드 (신규)
- `web/src/pages/dashboard/panels/modbus/RegisterMapGrid.tsx`: props `{ registerMap, registerCounts }`. 4영역(`REGISTER_AREA_ORDER`, `AgentDetailPanel.tsx:1164-1173` 라벨/순서 재사용) 그리드 렌더 + 영역별 POINTS/ACTIVE/DEGRADED 헤더.
- `web/src/pages/dashboard/panels/modbus/registerCellState.ts` (순수 유틸, TDD):
```ts
export type CellState = 'active' | 'inactive' | 'degraded';
export function cellState(area: string, addr: number,
  snapshot: RegisterSnapshot, counts: RegisterCounts): CellState;
// bool 영역: true→active / false→inactive
// 숫자 영역: !=0→active / 0→inactive
// 정의(count>0) 기대되나 스냅샷 부재/조회 에러 → degraded
```
REQ-04(공유 `get_map` unit 0)와 REQ-05(가상 `get_device_status` unit N)는 동일 `RegisterMapGrid`+`registerCellState`를 사용(AC-13).

### 2.4 exec 응답 언랩·폴링·graceful
- 언랩: `res.result ?? res`(`AgentDetailPanel.tsx:3950-3956` 패턴)로 envelope 흡수.
- 폴링: `useUIStore(s=>s.dashboardRefreshInterval)` 간격 `useExecAgent` 반복(또는 `useQuery` refetchInterval).
- 원격/정지/부재: `TargetContext`(`AgentPanel.tsx:49`) 및 exec 실패 시 안내 문구(회귀 0).

### 2.5 버스 통계 델타 누적
- 세션-로컬 링버퍼(`useRef<number[]>`)에 각 폴링의 `Σread_count`/`Σwrite_count` 스냅을 저장, 인접 델타 → per-min 환산. 기존 `charts/LineChartPanel` 계열 프리미티브로 렌더(신규 라이브러리 금지).

## 3. 백엔드 설계 — 백킹 관측 메트릭

### 3.1 backedStore 카운터 (backed_store.go)
```go
type backedStore struct {
    // 기존 필드 (inner, transport, upUnitID, mode, timeout, lastOK) ...
    reqCount  atomic.Int64 // upstream 요청 총수 (direct 서빙 + indirect 폴)
    errCount  atomic.Int64 // upstream 실패 총수 (ErrGatewayTargetFailed 등)
    latencyNs atomic.Int64 // 누적 레이턴시(ns); avg = latencyNs/reqCount
}
```
- 계측점: `sendUpstream`(`:223-227`)에서 `start := time.Now()` → 반환 시 `reqCount++`, 성공/실패에 따라 `errCount`/`latencyNs += elapsed`. 모든 upstream 헬퍼(`upstreamReadRegisters`/`upstreamReadBits`/`upstreamWrite*`)가 `sendUpstream`을 경유하므로 단일 계측점으로 충분.
- `recordPollSuccess`(`:209-211`)는 기존대로 `lastOK` 갱신(성공 판정 소스).

### 3.2 deviceBacking store 참조 (backing_lifecycle.go)
```go
type deviceBacking struct {
    transport modbus.ModbusTransport
    poller    *devicePoller
    store     *backedStore // 신규: 메트릭 조회용 참조 (:54 newBackedStore 결과 보관)
}
```
`setupBacking`(`:37-63`)에서 생성한 `bs`를 `backing.store`에 보관한다. 순수 slave는 `deviceBacking` 자체가 없으므로 조회 시 `backing:null`.

### 3.3 노출 (agent.go)
- `processGetDeviceStatus`(`:1377-1407`): `a.mu.RLock`으로 `a.backings[byte(uid)]` 조회 → 존재 시 `resp["backing"] = {mode, connected, request_count, error_count, avg_latency_ms, last_ok}`, 부재 시 `resp["backing"] = nil`.
  - `connected`: `transport` 연결 상태(간이) 또는 indirect는 `!isStale()`.
  - `mode`: `"direct"|"indirect"`.
- `processListDevices`(`:1184-1207`): 각 항목에 `backed` = (`a.backings[uid] != nil`), `mode` 추가.

### 3.4 동시성 (confusion 관리)
- 백킹 카운터는 폴러 goroutine(`pollOnce`)과 서빙 goroutine(리스너→backedStore) + 조회 goroutine(`processGetDeviceStatus`)이 접근한다. 전부 `atomic.Int64`로 lock-free. `a.backings` 맵 접근만 기존 `a.mu` 규약을 따른다(`:1357-1360` 패턴).
- **DeviceStats(device_manager.go:27)와 backing 카운터는 별개**다. DeviceStats는 마스터→게이트웨이 서빙 통계(RequestHandler 기록), backing 카운터는 게이트웨이→upstream 통신 통계다. 실제 디바이스 패널의 latency/err rate는 **backing 카운터**를 쓴다(혼동 금지).

## 4. 목업 대비 설계 결정 (confusion 명시)

| 목업 요소 | 데이터 소스 | 결정 |
|---|---|---|
| 상단 종합 통계 바 | `get_status`+`list_devices`+`list_clients` | 그대로 |
| 좌측 실제 디바이스(latency/err) | `get_device_status.backing` | 백엔드 신규 |
| 중앙 레지스터 그리드 색상/집계 | `get_map`/`get_device_status` snapshot + 프론트 판정 | 값 기반, 백엔드 미변경 |
| 우측 가상 디바이스 U01~ active/stale | `list_devices` | 그대로 |
| 하단 버스 미니차트 | `list_devices` 델타 누적 | 프론트 누적 |
| **"CRC errors"** | (백엔드 카운터 없음) | **Non-Goal** — error_count 델타 "errors/min"로 대체(백엔드 변경을 백킹 메트릭으로 한정하는 제약 준수) |

CRC/frame 오류 전용 카운터는 현재 백엔드에 없고, 이를 추가하면 "백엔드 변경 = 백킹 메트릭 노출로 한정"(REQ-07-04) 제약을 위반하므로 본 SPEC 범위에서 제외한다. 목업의 CRC 칸은 총 오류율(errors/min)로 대체 표기한다.

## 5. TRUST 5 매핑

- **Tested**: `registerCellState`/그리드/패널 vitest, backing 카운터 `go test -race`, 커버리지 ≥85%.
- **Readable**: 코드 주석 한국어(`language.yaml code_comments: ko`), 기존 파일 스타일 준수.
- **Unified**: 기존 패널/exec/atomic 패턴 재사용, prettier/gofmt.
- **Secured**: 관측 전용(쓰기 없음), 입력(unit_id) 검증 재사용.
- **Trackable**: REQ↔AC↔파일:라인 추적, 커밋 `feat(SPEC-MODBUS-012)`.
