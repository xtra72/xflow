---
id: SPEC-FACILITY-DASHBOARD-001
title: "지하철 시설물 관리 대시보드 패널 (라인·역사·기기)"
version: "0.1.0"
status: in-progress
created: 2026-07-28
updated: 2026-07-29
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "web/src/pages/dashboard"
lifecycle: spec-anchored
tier: M
tags: "dashboard, facility, subway, airpurifier, line-panel, station-panel, device-panel, bulk-control, aggregation, station-registry"
depends_on:
  - SPEC-AIRPURIFIER-001
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-07-28 | 0.1.0 | 초기 SPEC 작성 — 지하철 역사 시설물 관리 비전의 대시보드 3종 패널(라인/역사/기기) 정의. SPEC-AIRPURIFIER-001(v0.3.0)이 소유하는 디바이스 위치 계층(station/place/index) + 역사 레지스트리(station→line) + line/station fan-out 표면을 **소비**하여 시각화·통계·일괄제어 UI를 구성한다. 저장 영속화(패널 스키마)는 SPEC-DASHBOARD-001을 소비한다 |

---

# SPEC-FACILITY-DASHBOARD-001: 지하철 시설물 관리 대시보드 패널 (라인·역사·기기)

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP 플랫폼이며, React 기반 웹 UI(`web/`)에 **대시보드 패널 시스템**을 이미 보유하고 있다. 사용자는 대시보드 페이지에 여러 종류의 패널(패널 타입)을 자유롭게 추가·배치·설정하며, 그 구성은 서버(SQLite)에 영속 저장된다(SPEC-DASHBOARD-001).

본 SPEC은 **지하철 역사 시설물 관리(facility management)** 비전에서 **첫 디바이스 타입인 공기청정기(air purifier)** 를 대상으로, 운영자가 라인(호선) → 역사(station) → 기기(device) 계층으로 시설물 상태를 조망·제어할 수 있는 **3종의 신규 대시보드 패널 타입**을 정의한다.

1. **라인 패널(Line panel)** — 한 호선 전체 기기의 라인도(line diagram) + 역사별 상태 요약 + 라인 통계 + 라인 일괄 제어.
2. **역사 패널(Station panel)** — 한 역사의 통계 + 기기별 상태 목록 + 역사 일괄 제어.
3. **기기 패널(Device panel)** — 단일 기기의 상태 + 제어(응답 대기 피드백 포함).

이 세 패널은 기존 대시보드 패널 시스템의 **새 패널 타입**으로 편입되며, SPEC-AIRPURIFIER-001이 소유·노출하는 데이터/제어 표면만 소비한다. 본 SPEC은 **디바이스 에이전트/프로토콜/제어 내부 동작을 재정의하지 않는다**.

### 1.2 기술 환경

- **프론트엔드**: TypeScript + React (`web/`), Zustand(`web/src/stores/uiStore.ts`), 대시보드 패널 시스템(`web/src/pages/dashboard/`).
- **기존 패널 시스템** (EXTEND 대상, 확인됨 — 재탐색 불필요):
  - `web/src/stores/uiStore.ts` — `PanelType` 유니온, `PanelConfig`, `panelDefaultSize()`, `createDefaultPanel()`, `addPanel/addPanelWithConfig/updatePanelConfig`.
  - `web/src/pages/dashboard/AddPanelDialog.tsx` — 패널 타입 선택 목록(`type/icon/labelKey/descriptionKey/needsDevice`), device/chart 다단계 스텝.
  - `web/src/pages/dashboard/renderDashboardPanel.tsx` — `switch (panel.type)` 렌더 디스패치.
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx` — 패널별 설정 편집.
  - `web/src/pages/dashboard/panels/` — 기존 패널: `DevicePanel.tsx`, `SingleDevicePanel.tsx`, `AcControlPanel.tsx`, `HvacControlPanel.tsx`, `LogPanel.tsx`, `GaugePanel.tsx` 등 (신규 패널이 미러링할 패턴).
- **데이터/제어 API 표면** (SPEC-AIRPURIFIER-001 + 기존 API):
  - 단일 기기 조회/제어: `deviceService.getDevice(id)`, `deviceService.executeCommand(id, {command, params})` → `POST /devices/{id}/execute` → 어댑터 → 에이전트 `Process`.
  - 에이전트 Process 명령(로스터/레지스트리/셀렉터 일괄): `agentService.execAgent(id, {command, params, ...selector})` → `POST /agents/{id}/exec` → `AgentServiceAdapter.ExecAgent` → 에이전트 `Process`.
  - 공기청정기 에이전트 Process 명령(REQ-AIRPUR-001-03-06): `list_devices`, `list_stations`, `set_power`, `set_fan_speed`, `set_multiple`(셀렉터 `station`/`line`/`group_id` 지원).
- **패널 영속화**: SPEC-DASHBOARD-001의 `DashboardSnapshot.payload.dashboardPages[].panels[]` 스키마. 신규 패널은 그 스키마의 `type` + `config`로 표현되어 그대로 저장/복원된다.
- **테스트 프레임워크**: Vitest + React Testing Library (`*.test.tsx`).

### 1.3 설계 원칙

- **기존 패턴 준수**: 신규 패널은 `panels/` 하위 컴포넌트로 작성하고, 타입 등록(`PanelType` 유니온) → AddPanelDialog 항목 → renderDashboardPanel case → PanelSettingsDialog 설정의 4-지점 와이어링을 기존 패널(`ac-control`/`hvac-control`)과 동일하게 따른다.
- **소비 전용(consume-only)**: 디바이스 위치 계층, station→line 매핑, line/station fan-out은 **SPEC-AIRPURIFIER-001의 SSOT**를 그대로 소비한다. 본 SPEC은 자체 매핑/제어 로직을 만들지 않는다.
- **집계는 UI/집계 레이어에서**: 라인/역사 통계와 역사별 요약은 에이전트가 노출하는 로스터(`list_devices`) + 역사 레지스트리(`list_stations`)로부터 파생한다. 백엔드 전용 집계 엔드포인트는 MVP 범위 외로 두되(대규모 fleet 성능 시 후속), 계산 방식은 §3 Module 5에서 결정한다.
- **응답 대기 UX 노출**: 개별·일괄 제어 모두 SPEC-AIRPURIFIER-001의 응답 대기(state echo) 결과(`ok`/`error`/`timeout`)를 사용자에게 그대로 반영한다.

### 1.4 범위 경계

**본 SPEC이 소유(신규 정의)하는 것**

- 신규 패널 타입 3종(`facility-line` / `facility-station` / `facility-device`)의 UI/렌더/설정 정의.
- 라인도(line diagram) 렌더링 — 역사 레지스트리의 정렬 정보로 역사를 라인 위에 배치하고 역사별 상태 요약 배지를 표시.
- 라인/역사 통계 집계 규칙(상태별 카운트: online/offline, power on/off, fan_speed 분포)과 그 **데이터 소스 선택**(client-side 집계 vs 백엔드 엔드포인트).
- 일괄 제어 UI(라인/역사 셀렉터로 fan-out 호출 + 멤버별 응답 대기 집계 결과 렌더링).
- 기존 대시보드 패널 시스템으로의 등록/와이어링(4-지점) + 영속 스키마(SPEC-DASHBOARD-001) 정합.

**본 SPEC이 소비만 하는 것(재정의하지 않음)**

- 디바이스 에이전트/프로토콜/제어 내부 동작, 위치 계층(station/place/index), 역사 레지스트리(station→line), 개별·일괄 제어(set_power/set_fan_speed), 응답 대기(state echo, `control_response_timeout`, `ErrControlTimeout`), line/station/group 셀렉터 fan-out 및 멤버별 집계 응답 → **모두 SPEC-AIRPURIFIER-001의 범위**.
- 대시보드 페이지/패널 서버 영속화 메커니즘(SQLite, 공유/개인 스코프, 권한, `If-Match` 충돌) → **SPEC-DASHBOARD-001의 범위**. 본 SPEC은 신규 패널이 그 스키마에 맞게 저장·복원됨만 보장.

**범위 외(Non-Goals)**

- 공기청정기 외 다른 시설물 디바이스 타입(공조/조명/센서 등)의 패널 — 후속 SPEC.
- PM2.5/CO2 등 센서 텔레메트리의 시각화 — 본 SPEC은 power/fan_speed/online 상태 중심(SPEC-AIRPURIFIER-001 상태 축과 동일).
- 역사 레지스트리 자체의 편집(CRUD) UI — 레지스트리 CRUD는 SPEC-AIRPURIFIER-001의 `add_station`/`remove_station`/`list_stations`가 소유. 본 SPEC은 `list_stations` **조회**만 소비한다(레지스트리 편집 화면은 후속).
- 실시간 WebSocket 푸시 — 본 SPEC은 대시보드 refresh 주기(폴링)로 갱신(기존 패널 관행 준수).
- 지리적 좌표 기반 실측 노선도(GIS) — 라인도는 역사 레지스트리의 **순서(order)** 기반 논리적 배치이며 실좌표 지도가 아니다.

---

## 2. Assumptions (가정)

- **A-1. AIRPURIFIER-001 표면 가용성**: 본 SPEC은 SPEC-AIRPURIFIER-001(v0.3.0 이상)이 정의한 (a) 디바이스 위치 계층(`station`/`place`/`index`), (b) 역사 레지스트리(station→{line, display_name, order}), (c) `list_devices`/`list_stations` 조회 명령, (d) `station`/`line` 셀렉터 일괄 제어 + 멤버별 `ok`/`error`/`timeout` 집계 응답이 에이전트 `Process`/exec 경로로 이용 가능하다고 가정한다. 이들이 미구현이면 본 SPEC의 패널은 빈/에러 상태로 안전 degrade한다(A-7).
- **A-2. 데이터 소스는 에이전트 exec**: 라인/역사/기기 화면에 필요한 로스터·레지스트리 데이터는 `POST /agents/{id}/exec`의 `list_devices`/`list_stations` 응답으로 취득한다. 별도의 시설물 전용 REST 리소스는 MVP에서 도입하지 않는다(Module 5 결정).
- **A-3. 통계 집합 정의**: "통계 정보"는 상태별 **카운트**로 한정한다 — {online/offline 수, power on/off 수, fan_speed(1/2/3) 분포, 총 기기 수}. 시계열/추세는 기존 차트 패널(SPEC-CHART-001)의 몫이며 본 패널의 통계는 스냅샷 카운트다.
- **A-4. 라인도 배치 소스는 역사 레지스트리의 order/display_name**: 라인 위 역사 배치 순서와 표시명은 역사 레지스트리(`list_stations`)의 `order`·`display_name`에서 온다. 별도 좌표 데이터는 사용하지 않는다.
- **A-5. 셀렉터 대상 지정**: 라인 일괄 제어는 셀렉터 `line`, 역사 일괄 제어는 셀렉터 `station`으로 에이전트 exec를 호출한다. 우선순위·해석(REQ-AIRPUR-001-04-04)은 에이전트가 소유하며, 패널은 단일 셀렉터만 지정한다.
- **A-6. 응답 대기 결과는 exec 응답으로 반환**: 개별 제어(`executeCommand`)와 일괄 제어(`execAgent`)의 응답에는 SPEC-AIRPURIFIER-001의 응답 대기 판정(개별: `ok`/`timeout`, 일괄: 멤버별 `ok`/`error`/`timeout` 집계)이 포함되어 반환된다고 가정한다. 패널은 이 응답을 렌더링할 뿐 자체 타임아웃 로직을 두지 않는다(단, 네트워크 레벨 요청 타임아웃은 별개).
- **A-7. 미등록/빈 상태 안전 degrade**: 미등록 station(레지스트리에 없음)·빈 라인·빈 역사·미구현 표면은 패널이 크래시 없이 "데이터 없음"/"미등록" 안내로 표시한다.
- **A-8. 에이전트 식별**: 시설물 패널은 대상 공기청정기 **에이전트 ID**를 패널 `config`로 보유한다(패널 추가 시 선택). 하나의 에이전트가 다수 역사·라인의 기기를 로스터로 관리하는 것을 기본 가정한다(단일 에이전트 스코프). 다중 에이전트 통합 집계는 후속(OI-3).
- **A-9. 영속 정합**: 신규 패널의 `config`(에이전트 ID, 선택된 line/station, 표시 옵션)는 SPEC-DASHBOARD-001의 `payload` snapshot에 포함되어 그대로 저장/복원되며, 알 수 없는 필드를 추가하지 않는다(schema 확장은 `config` 내부 키로 한정).

---

## 3. Requirements (요구사항 - EARS Format)

> REQ ID 체계: `REQ-FACDASH-001-{모듈}-{순번}`. 모듈 1=라인 패널, 2=역사 패널, 3=기기 패널, 4=패널 등록/와이어링, 5=데이터/집계 소스, 6=일괄 제어 통합.

### Module 1: 라인 패널 (Line panel)

#### REQ-FACDASH-001-01-01 (Ubiquitous) 라인 패널 구성 요소

`facility-line` 패널은 **항상** 다음 네 영역을 렌더링해야 한다: (1) 라인도(line diagram), (2) 역사별 기기 상태 간략 정보, (3) 라인 통계 정보, (4) 라인 일괄 제어. 패널 `config`는 대상 `agentId`와 대상 `line`(호선 식별자), 표시 옵션을 보유한다.

#### REQ-FACDASH-001-01-02 (Ubiquitous) 라인도 렌더링

라인도는 **항상** 대상 호선에 속한 역사들을 역사 레지스트리(`list_stations`)의 `order` 순으로 라인 위에 배치하고, 각 역사 노드에 상태 요약 배지(온라인/오프라인 수, power on/off 수, 결함 지시자)를 표시해야 한다. 역사 표시명은 레지스트리의 `display_name`을 사용한다.

#### REQ-FACDASH-001-01-03 (Ubiquitous) 역사별 간략 상태 정보

라인 패널은 **항상** 대상 호선의 각 역사에 대해 간략 상태(역사명 + 총 기기 수 + online/offline + power on/off 요약)를 목록/그리드로 제공해야 한다.

#### REQ-FACDASH-001-01-04 (Ubiquitous) 라인 통계 정보

라인 패널은 **항상** 호선 레벨 통계(총 기기 수, online/offline 카운트, power on/off 카운트, fan_speed 1/2/3 분포)를 A-3의 집계 규칙(Module 5)에 따라 표시해야 한다.

#### REQ-FACDASH-001-01-05 (Event-Driven) 라인 일괄 제어

**WHEN** 사용자가 라인 패널에서 일괄 제어(set_power on/off 또는 set_fan_speed 1/2/3)를 실행하면 **THEN** 패널은 셀렉터 `line`으로 에이전트 fan-out(Module 6, REQ-FACDASH-001-06-02)을 호출하고, 반환된 멤버별 응답 대기 집계 결과(`ok`/`error`/`timeout`)를 렌더링해야 한다.

#### REQ-FACDASH-001-01-06 (State-Driven) 빈/미등록 호선 처리

**IF** 대상 호선에 역사·기기가 하나도 없거나 호선이 레지스트리에 없으면 **THEN** 라인 패널은 크래시 없이 "표시할 역사/기기가 없습니다" 안내를 표시하고 일괄 제어를 비활성화해야 한다.

### Module 2: 역사 패널 (Station panel)

#### REQ-FACDASH-001-02-01 (Ubiquitous) 역사 패널 구성 요소

`facility-station` 패널은 **항상** 다음 세 영역을 렌더링해야 한다: (1) 역사 통계 정보, (2) 기기별 상태 정보 목록, (3) 역사 일괄 제어. 패널 `config`는 대상 `agentId`와 대상 `station`을 보유한다.

#### REQ-FACDASH-001-02-02 (Ubiquitous) 역사 통계 정보

역사 패널은 **항상** 역사 레벨 통계(총 기기 수, online/offline, power on/off, fan_speed 분포)를 Module 5 집계 규칙으로 표시해야 한다.

#### REQ-FACDASH-001-02-03 (Ubiquitous) 기기별 상태 정보

역사 패널은 **항상** 대상 역사에 소속된(디바이스 `Station` 일치) 각 기기의 상태(기기명/ID, online, power, fan_speed, place/index)를 목록으로 제공해야 한다.

#### REQ-FACDASH-001-02-04 (Event-Driven) 역사 일괄 제어

**WHEN** 사용자가 역사 패널에서 일괄 제어를 실행하면 **THEN** 패널은 셀렉터 `station`으로 에이전트 fan-out(REQ-FACDASH-001-06-02)을 호출하고, 멤버별 응답 대기 집계 결과를 렌더링해야 한다.

#### REQ-FACDASH-001-02-05 (State-Driven) 빈/미등록 역사 처리

**IF** 대상 역사에 기기가 없거나 역사가 레지스트리에 미등록이면 **THEN** 역사 패널은 크래시 없이 "미등록 역사" 또는 "기기 없음" 안내를 표시하고 일괄 제어를 비활성화해야 한다.

### Module 3: 기기 패널 (Device panel)

#### REQ-FACDASH-001-03-01 (Ubiquitous) 기기 상태 정보

`facility-device` 패널은 **항상** 단일 기기의 상태(power, fan_speed, online, station/place/index)를 표시해야 한다. 패널 `config`는 대상 `agentId`와 `deviceId`를 보유한다.

#### REQ-FACDASH-001-03-02 (Event-Driven) 기기 제어와 응답 대기 피드백

**WHEN** 사용자가 기기 패널에서 `set_power`(on/off) 또는 `set_fan_speed`(1/2/3)를 실행하면 **THEN** 패널은 `deviceService.executeCommand`(또는 에이전트 exec의 device_id 대상)로 제어를 호출하고, SPEC-AIRPURIFIER-001의 응답 대기 결과를 **성공(`ok`, 상태 에코 반영)** 또는 **타임아웃(`timeout`/`ErrControlTimeout`)** 으로 사용자에게 명시적으로 표시해야 한다.

#### REQ-FACDASH-001-03-03 (State-Driven) 풍량-전원 의존 UX

**IF** 대상 기기의 전원이 OFF 상태이면 **THEN** 기기 패널은 풍량 제어의 의미론(전원 ON일 때만 유효, REQ-AIRPUR-001-03-03)을 UX에 반영해야 한다 — 풍량 컨트롤 비활성화 또는 거부(`ErrPowerOff`) 결과의 명시적 안내 중 하나.

#### REQ-FACDASH-001-03-04 (Optional) 기존 SingleDevicePanel 확장 재사용

가능하면 기기 패널은 기존 `SingleDevicePanel.tsx`/`DevicePanel.tsx` 패턴을 확장·재사용하여 상태 표시 로직 중복을 피할 수 있다. 재사용 여부는 plan.md의 기술 결정에 따른다.

### Module 4: 패널 등록/와이어링 (Panel registration & wiring)

#### REQ-FACDASH-001-04-01 (Ubiquitous) 신규 패널 타입 등록

시스템은 **항상** 3종 신규 패널 타입(`facility-line`, `facility-station`, `facility-device`)을 `web/src/stores/uiStore.ts`의 `PanelType` 유니온에 추가하고, `panelDefaultSize()`·`createDefaultPanel()`에 기본 크기/기본 config를 정의해야 한다.

#### REQ-FACDASH-001-04-02 (Ubiquitous) AddPanelDialog 등록

시스템은 **항상** 3종 패널을 `AddPanelDialog.tsx`의 패널 타입 목록에 항목(type/icon/labelKey/descriptionKey)으로 추가해야 한다. 기기 패널은 `needsDevice` 스텝을, 라인/역사 패널은 대상 선택(line/station) 스텝을 제공한다.

#### REQ-FACDASH-001-04-03 (Ubiquitous) renderDashboardPanel 디스패치

시스템은 **항상** `renderDashboardPanel.tsx`의 `switch (panel.type)`에 3종 case를 추가하여 해당 패널 컴포넌트로 렌더 디스패치해야 한다.

#### REQ-FACDASH-001-04-04 (Ubiquitous) PanelSettingsDialog 설정

시스템은 **항상** 3종 패널의 설정(대상 agentId, line/station/deviceId, 표시 옵션)을 `PanelSettingsDialog.tsx`에서 편집 가능하게 제공해야 한다.

#### REQ-FACDASH-001-04-05 (Ubiquitous) 영속 스키마 정합 (SPEC-DASHBOARD-001 소비)

신규 패널의 `type` + `config`는 **항상** SPEC-DASHBOARD-001의 `DashboardSnapshot.payload.dashboardPages[].panels[]` 스키마로 표현되어 그대로 저장·복원되어야 하며, 스냅샷 스키마에 신규 최상위 필드를 추가하지 않아야 한다(확장은 `config` 내부 키로 한정).

#### REQ-FACDASH-001-04-06 (i18n) 라벨/설명 다국어

시스템은 **항상** 신규 패널의 라벨/설명/컨트롤 텍스트를 `web/src/lib/i18n/ko.json`·`en.json`에 키로 추가하고 하드코딩하지 않아야 한다(기존 `dashboard.panelTypes.*`/`dashboard.addPanel.descriptions.*` 관행 준수).

### Module 5: 데이터/집계 소스 (Data & aggregation)

#### REQ-FACDASH-001-05-01 (Ubiquitous) 데이터 소스 = 에이전트 exec (client-side 집계 채택)

시스템은 **항상** 라인/역사/기기 화면 데이터를 에이전트 exec의 `list_devices`(로스터: device_id, station, place, index, online, power, fan_speed) + `list_stations`(역사 레지스트리: station→{line, display_name, order})로부터 취득하고, 통계·역사별 요약·라인도 배치를 **client-side에서 집계**해야 한다. **결정**: MVP는 신규 백엔드 집계 엔드포인트를 도입하지 않고 기존 AIRPURIFIER exec 표면을 재사용한다(A-2).

#### REQ-FACDASH-001-05-02 (Ubiquitous) 통계 집계 규칙

집계 로직은 **항상** 로스터를 station/line으로 그룹핑하여 {총 기기 수, online/offline, power on/off, fan_speed(1/2/3) 분포}를 카운트로 산출해야 한다. line은 디바이스 속성이 아니므로(REQ-AIRPUR-001-02-07/A-10) 각 디바이스의 `station`을 레지스트리에서 조회하여 line을 해석한 뒤 집계한다.

#### REQ-FACDASH-001-05-03 (State-Driven) 미등록 station 집계 처리

**IF** 로스터의 어떤 디바이스가 레지스트리에 없는 station을 참조하면 **THEN** 그 디바이스는 line 집계에서 제외되고, 그 사실이 UI(예: "미분류 N대")로 표기되어야 한다(REQ-AIRPUR-001-02-13 정합).

#### REQ-FACDASH-001-05-04 (Event-Driven) 갱신 주기

**WHEN** 대시보드 refresh 주기가 도래하거나 사용자가 수동 새로고침하면 **THEN** 패널은 `list_devices`/`list_stations`를 재조회하여 상태/통계를 갱신해야 한다(기존 패널 폴링 관행 준수).

#### REQ-FACDASH-001-05-05 (Optional, deferred) 백엔드 집계 엔드포인트

가능하면(대규모 fleet 성능 이슈 시) 시스템은 라인/역사 통계를 서버측에서 집계하는 전용 엔드포인트를 제공할 수 있다. **v0.1.0 미구현** — client-side 집계를 채택하고, 성능 한계 관찰 시 후속 SPEC으로 승격한다(OI-1).

### Module 6: 일괄 제어 통합 (Bulk-control integration)

#### REQ-FACDASH-001-06-01 (Ubiquitous) 일괄 제어 호출 경로

라인/역사 일괄 제어는 **항상** `agentService.execAgent(agentId, {command, <selector>, params})`(→ `POST /agents/{id}/exec` → 에이전트 `Process`)로 호출되어야 하며, 셀렉터는 라인=`line`, 역사=`station`으로 지정한다. 패널은 SPEC-AIRPURIFIER-001의 셀렉터 fan-out을 호출만 하고 fan-out 로직을 재구현하지 않는다.

#### REQ-FACDASH-001-06-02 (Event-Driven) 멤버별 집계 결과 렌더링

**WHEN** 일괄 제어 응답이 수신되면 **THEN** 패널은 멤버별 결과 배열(`{device_id, status: ok|error|timeout, error?}`)을 사용자에게 표시해야 한다 — 성공/실패/타임아웃 수 요약 + 실패·타임아웃 멤버 상세.

#### REQ-FACDASH-001-06-03 (State-Driven) 빈 대상 일괄 제어 방지

**IF** 대상 라인/역사에 멤버 기기가 없으면 **THEN** 패널은 일괄 제어를 비활성화하거나 실행 시 `ErrEmptyGroup` 의미론(REQ-AIRPUR-001-04-02)의 no-op 응답을 "대상 없음"으로 안내해야 한다.

#### REQ-FACDASH-001-06-04 (Event-Driven) 제어 진행/결과 피드백

**WHEN** 일괄/개별 제어가 진행 중이면 **THEN** 패널은 진행 표시(로딩)를 노출하고, 완료 시 결과(성공 수/타임아웃 수)를 명시적으로 사용자에게 알려야 한다 — 소리 없는 실패(silent drop) 금지.

### Unwanted Behavior Requirements (금지 동작)

- **UB-001**: 패널은 station→line 매핑·fan-out·응답 대기 판정을 **자체적으로 재구현하지 않아야** 한다 — 모두 SPEC-AIRPURIFIER-001 표면을 호출한다.
- **UB-002**: 패널은 제어 결과를 **소리 없이 누락**하지 않아야 한다 — 성공/실패/타임아웃을 항상 사용자에게 표시한다.
- **UB-003**: 패널은 대시보드 스냅샷 스키마에 신규 최상위 필드를 추가하지 않아야 한다 — 확장은 패널 `config` 내부로 한정(SPEC-DASHBOARD-001 백워드 호환).
- **UB-004**: 패널은 미등록 station의 디바이스를 line 집계에 **묵시적으로 포함**하지 않아야 한다 — 제외 + 별도 표기(REQ-FACDASH-001-05-03).
- **UB-005**: 패널은 전원 OFF 상태에서 풍량 제어를 **성공으로 위장**하지 않아야 한다 — 비활성화 또는 `ErrPowerOff` 안내(REQ-FACDASH-001-03-03).

---

## 4. Specifications (명세)

### 신규 패널 타입

| 패널 타입           | 컴포넌트(예정)                              | config 주요 필드                                   |
| --------------- | ------------------------------------- | ---------------------------------------------- |
| `facility-line`    | `panels/FacilityLinePanel.tsx`      | `agentId`, `line`, 표시 옵션                        |
| `facility-station` | `panels/FacilityStationPanel.tsx`   | `agentId`, `station`, 표시 옵션                     |
| `facility-device`  | `panels/FacilityDevicePanel.tsx`    | `agentId`, `deviceId`, 표시 옵션                    |

### 소비하는 AIRPURIFIER-001 표면 (재정의 금지)

| 소비 대상                     | AIRPURIFIER-001 REQ            | 호출 경로                                                   |
| ------------------------ | ----------------------------- | ------------------------------------------------------- |
| 로스터 조회(위치/상태)            | REQ-AIRPUR-001-02-06 `list_devices` | `execAgent(agentId, {command:"list_devices"})`          |
| 역사 레지스트리 조회             | REQ-AIRPUR-001-02-11 `list_stations` | `execAgent(agentId, {command:"list_stations"})`         |
| 개별 제어 + 응답 대기           | REQ-AIRPUR-001-03-02/03/07/08 | `executeCommand(deviceId, {command, params})`           |
| line 셀렉터 일괄 제어          | REQ-AIRPUR-001-04-06          | `execAgent(agentId, {command, line, params})`           |
| station 셀렉터 일괄 제어       | REQ-AIRPUR-001-04-05          | `execAgent(agentId, {command, station, params})`        |
| 멤버별 응답 집계(ok/error/timeout) | REQ-AIRPUR-001-04-07      | 위 exec 응답 body의 `results[]`                             |
| 미등록 station 제외 표기       | REQ-AIRPUR-001-02-13          | 집계 응답/로스터 대비 레지스트리 조회 결과                             |

### 소비하는 DASHBOARD-001 표면 (재정의 금지)

- `DashboardSnapshot.payload.dashboardPages[].panels[]` — 신규 패널은 `{ id, type, title, config }`로 표현되어 기존 저장/복원(공유/개인 스코프, `If-Match` 충돌 정책 포함) 흐름을 그대로 탄다.

### Traceability

| Requirement                     | Implementation Files (예정)                                                                 | Tests (예정)                              |
| ------------------------------- | --------------------------------------------------------------------------------------- | -------------------------------------- |
| REQ-FACDASH-001-01-*(라인 패널)     | `panels/FacilityLinePanel.tsx`, 라인도 서브컴포넌트, `lib/facilityAggregation.ts`                  | `FacilityLinePanel.test.tsx`           |
| REQ-FACDASH-001-02-*(역사 패널)     | `panels/FacilityStationPanel.tsx`                                                        | `FacilityStationPanel.test.tsx`        |
| REQ-FACDASH-001-03-*(기기 패널)     | `panels/FacilityDevicePanel.tsx` (SingleDevicePanel 재사용 가능)                              | `FacilityDevicePanel.test.tsx`         |
| REQ-FACDASH-001-04-*(등록/와이어링)   | `stores/uiStore.ts`, `AddPanelDialog.tsx`, `renderDashboardPanel.tsx`, `PanelSettingsDialog.tsx`, `lib/i18n/{ko,en}.json` | `AddPanelDialog.test.tsx` (신규 타입 case) |
| REQ-FACDASH-001-05-*(데이터/집계)    | `lib/facilityAggregation.ts`, `services/api/agentService.ts`(exec 재사용)                    | `facilityAggregation.test.ts`          |
| REQ-FACDASH-001-06-*(일괄 제어)     | `panels/FacilityLinePanel.tsx`, `FacilityStationPanel.tsx`, 결과 렌더 서브컴포넌트                  | 각 패널 test + 일괄 제어 시나리오                   |

---

## 5. Open Issues

- **OI-1 (백엔드 집계 엔드포인트)** — client-side 집계가 대규모 fleet(수백~수천 기기)에서 성능/전송량 문제를 일으키면 서버측 집계 엔드포인트를 후속 SPEC으로 도입한다(REQ-FACDASH-001-05-05, deferred).
- **OI-2 (역사 레지스트리 편집 UI)** — 본 SPEC은 `list_stations` 조회만 소비한다. 레지스트리 CRUD(add/remove station, line/order 편집) 화면은 후속 SPEC.
- **OI-3 (다중 에이전트 통합 집계)** — 하나의 라인/역사가 여러 에이전트에 걸친 경우의 통합 집계는 본 SPEC 범위 외(단일 agentId 스코프, A-8). 요구 명확화 시 후속.
- **OI-4 (라인도 시각 정밀도)** — 초기 라인도는 order 기반 논리 배치다. 실측 노선도/분기/환승역 표현은 후속 검토.

---

## 6. 비고 (Notes)

- 본 SPEC은 **소비 SPEC**이다. 데이터/제어/매핑의 SSOT는 SPEC-AIRPURIFIER-001, 영속화의 SSOT는 SPEC-DASHBOARD-001이며, 본 SPEC은 그 위에 시각화·통계·제어 UI만 얹는다.
- 선례 참조: SPEC-DASHBOARD-001(패널 영속화), SPEC-CHART-001(차트/stat 패널), SPEC-DEVICE-001(기기 패널/상태), 기존 `AcControlPanel`/`HvacControlPanel`(제어 패널 UX 패턴).
- 라인/역사 통계는 **스냅샷 카운트**이며 시계열이 아니다(A-3). 추세 시각화가 필요하면 기존 차트 패널을 병행 사용한다.
