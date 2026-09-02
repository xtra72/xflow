---
id: SPEC-SYSMETRICS-PANEL-001
title: "sysmetrics 호스트 지표 대시보드 패널 (시스템 · 네트워크 · 스토리지)"
version: "0.2.0"
status: implemented
created: 2026-08-28
updated: 2026-08-28
author: xtra
priority: P2
phase: "v0.38.0 target"
module: "internal/agent/system, web/src/pages/dashboard/panels/sysmetrics, web/src/pages/dashboard"
lifecycle: spec-anchored
tags: "sysmetrics, dashboard, panel, cpu, memory, disk-io, network, storage, stateful-agent"
tier: M
---

# SPEC-SYSMETRICS-PANEL-001: sysmetrics 호스트 지표 대시보드 패널

## HISTORY

| 버전  | 날짜       | 변경 내용 |
| ----- | ---------- | --------- |
| 0.1.0 | 2026-08-28 | 최초 작성. 사용자 결정 3건 반영: 데이터 소스=에이전트 직접 조회, 기존 monitor 패널 5종과 분리한 신규 패널, 패널 3종 + 대상 선택 구성. |
| 0.2.0 | 2026-08-28 | 구현 완료(M1~M6). 구현 중 확정·정정된 사항: (a) `state` 는 `detail=summary` 응답에도 실린다(AC-07 및 위험표 정정 — 웹 에이전트 목록이 summary 로 호출하므로 스냅샷이 목록 응답에 함께 나가며, 실측 비용을 받아들였다), (b) 폴링 훅의 rate 기준점이 마운트 시 두 effect 경쟁으로 항상 비던 버그를 테스트가 검출해 단일 effect + 소속 에이전트 동반 보관으로 수정, (c) 에이전트 선택 스텝의 타입 필터를 불리언 플래그 대신 파라미터로 일반화, (d) 패널 추가 메뉴 system 카테고리를 모니터링/호스트 지표 두 그룹으로 분리(기존 5종에도 그룹 제목이 처음 생겼다). |

---

## 1. Environment (환경)

### 1.1 현재 상태

호스트 시스템 지표를 보는 경로가 두 갈래로 갈라져 있고, 둘 다 요청 범위를 덮지 못한다.

**A. 라이브 monitor 패널 5종** (`monitor-stats` / `monitor-metrics` / `monitor-network` / `monitor-logs` / `monitor-events`)

`/monitor/*` REST 를 폴링해 그린다. 이 중 CPU·메모리 항목은 **호스트 지표가 아니다**:

- `CPUUsagePercent` 는 `0` 하드코딩이다 (`internal/api/handler/monitor.go:295` — "샘플링이 필요하므로 v1 에서는 미지원"). 즉 화면의 CPU 게이지는 언제나 0 이다.
- `MemoryUsagePct` 는 `mem.Alloc / mem.Sys` — **xflowd 프로세스의 Go 힙** 비율이며 호스트 메모리가 아니다.
- 디스크 I/O · 스토리지 사용량 항목은 아예 없다.
- `monitor-network` 만이 예외로, `gopsutil` 기반 `/monitor/network` 에서 인터페이스별 rx/tx 증가량과 누적을 실제로 그린다.

**B. sysmetrics 에이전트** (`internal/agent/system/sysmetrics_*.go`)

호스트 CPU·메모리·디스크 I/O·네트워크·스토리지를 표본 주기마다 실제로 수집한다. 그러나 수집 결과를 **채널로 방출하기만** 하고(`emitSample` → `recvCh` → `sysmetrics-in` 노드), 조회 가능한 상태로 남기지 않는다. 따라서 이 데이터를 화면에서 보려면 사용자가 직접 플로우를 구성하고 storage-write 로 저장한 뒤 차트 채널을 손으로 매어야 한다.

정리하면 **호스트 CPU·메모리를 실제 값으로 갖고 있는 유일한 소스가 sysmetrics 에이전트이고, 그 값을 볼 수 있는 화면이 없다.**

### 1.2 관련 자산 (재사용 대상)

| 자산 | 위치 | 재사용 내용 |
|---|---|---|
| `StatefulAgent` | `internal/agent/agent.go:39` | `State() map[string]any` → `GET /agents/{id}?detail=full` 의 `state` 필드로 노출 |
| `SysResourcesResponse` | `internal/api/handler/monitor_sysresources.go` | 호스트의 마운트·디스크 장치·인터페이스 이름 목록 |
| `SysResourceSelector` | `web/src/components/property/SysResourceSelector.tsx` | 대상 선택 체크박스 위젯 (에이전트 설정에서 이미 사용 중) |
| `networkSeries.ts` | `web/src/pages/monitoring/` | 누적 카운터 → 단위시간당 증가량 변환, 인터페이스 색상, `TOTAL_INTERFACE` 합산 |
| `monitorPanelConfig.ts` | `web/src/pages/dashboard/panels/monitor/` | `items` / `maxCols` / `refreshMs` / `windowSec` 설정 읽기 규약 |
| `useAutoColumns`, `useThrottledValue` | 같은 위치 | 폭 기반 열 수 축소, 갱신 스로틀 |
| `MultiSeriesChart`, `NetworkChart`, `StatItem` | `web/src/pages/monitoring/` | 다중 시리즈 차트 · 통계 타일 |

### 1.3 아키텍처 제약

- **에이전트는 사용자 에이전트다.** sysmetrics 는 시스템 내장 5종이 아니라 사용자가 생성하는 에이전트이므로, 패널은 항상 특정 에이전트 인스턴스에 바인딩된다. 여러 sysmetrics 에이전트가 공존할 수 있다.
- **표본 수집은 소비자와 무관하게 돈다.** `sampleLoop` 는 플로우 연결 여부와 무관하게 매 틱 `collectSample` 을 호출하며, 채널이 가득 차면 그 표본을 버린다(`emitSample` 의 `default` 분기). 이 성질 덕분에 **플로우 없이도** 최신 표본을 확보할 수 있다.
- **에이전트 이름은 변경될 수 있다.** 패널 config 는 `agent_id`(불변)를 정본으로 저장하고 이름은 표시용 스냅샷으로 둔다 (SPEC-WEB-006 규약).
- 지표 하나가 실패해도 나머지 표본은 유효하다(`emitSample` 의 부분 실패 허용). 패널도 같은 태도를 따라야 한다.

---

## 2. Assumptions (가정)

- A1: sysmetrics 에이전트의 수집 구현(`sysmetrics_collect.go`)과 방출 경로(`recvCh` → `sysmetrics-in`)는 변경하지 않는다. 상태 스냅샷은 **추가**이지 대체가 아니다.
- A2: 기존 monitor 패널 5종은 그대로 둔다. 타입·렌더 경로·설정 어휘를 건드리지 않는다.
- A3: rate 계열(네트워크·디스크 I/O)은 누적 카운터를 브라우저에서 차분해 계산한다. 기존 `monitor-network` 와 같은 방식이며, 서버가 rate 를 계산하지 않는다.
- A4: 히스토리는 브라우저 메모리 창(`windowSec`)에만 남는다. 영구 히스토리는 sysmetrics-in → storage-write 저장 경로의 몫이며 본 SPEC 범위가 아니다.
- A5: 패널은 로컬 xflowd 호스트를 관측한다. 원격 호스트 수집은 저장 경로의 몫이다(A4 와 같은 경계).
- A6: 모든 Go 코드는 `go test -race` 를 통과한다.

---

## 3. Requirements (요구사항)

EARS 형식. `WHERE`(상태 전제) / `WHEN`(사건) / `IF`(조건) / `WHILE`(지속 조건) 을 사용한다.

### Module 1: 에이전트 상태 스냅샷 (백엔드)

**REQ-SYSMETRICS-PANEL-001-01 — 최신 표본 보관**
WHILE sysmetrics 에이전트가 Running 상태인 동안, 시스템은 매 표본 주기마다 수집한 표본을 최신 스냅샷으로 보관해야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-02 — 소비자 독립성**
IF 표본 채널이 가득 차 그 표본이 방출되지 못하면, 시스템은 그럼에도 최신 스냅샷을 갱신해야 한다(SHALL). 스냅샷 갱신은 채널 전송보다 **먼저** 일어난다.

> 근거: 플로우를 구성하지 않은 사용자(패널만 쓰는 경우)에게 채널은 항상 가득 찬 상태다. 전송 성공에 스냅샷을 매달면 패널이 영구히 빈 화면이 된다.

**REQ-SYSMETRICS-PANEL-001-03 — State() 노출**
WHERE sysmetrics 에이전트가 `agent.StatefulAgent` 를 구현할 때, `State()` 는 최신 스냅샷과 수집 설정을 map 으로 반환해야 한다(SHALL). 반환 키는 다음을 포함한다:

| 키 | 내용 |
|---|---|
| `status` | `running` / `stopped` / `no_sample` |
| `collected_at` | 표본 시각 (epoch ms) |
| `interval_seconds` | 표본 주기 |
| `cpu` / `memory` | 스칼라 지표 오브젝트 (수집 비활성 시 부재) |
| `disk_io` / `network` / `storage` | 장치·인터페이스·마운트별 오브젝트 맵 |
| `targets` | `{mountpoints, devices, interfaces}` — 이 에이전트가 실제로 수집 중인 대상 이름 목록 |

**REQ-SYSMETRICS-PANEL-001-04 — 표본 이전 조회**
IF 아직 표본이 하나도 수집되지 않았다면, `State()` 는 `status: "no_sample"` 을 반환해야 하며 오류를 반환해서는 안 된다(SHALL NOT).

**REQ-SYSMETRICS-PANEL-001-05 — 동시성 안전**
WHEN `State()` 가 표본 루프와 동시에 호출되면, 시스템은 데이터 경합 없이 일관된 스냅샷 하나를 반환해야 한다(SHALL).

### Module 2: 시스템 패널 (`sysmetrics-system`)

**REQ-SYSMETRICS-PANEL-001-06 — 표시 항목**
WHERE 시스템 패널이 배치될 때, 패널은 다음 항목을 표시할 수 있어야 한다(SHALL): CPU 사용률, 메모리 사용률/사용량/전체, 디스크 I/O 종합(읽기·쓰기 bytes/s, 횟수/s), 네트워크 종합(rx/tx bytes/s, 패킷/s).

**REQ-SYSMETRICS-PANEL-001-07 — 종합의 정의**
WHERE 디스크 I/O 종합과 네트워크 종합을 계산할 때, 시스템은 에이전트가 수집한 **모든 장치·인터페이스의 합**으로 계산해야 한다(SHALL). 합산 대상은 에이전트 설정이 정하며 패널이 다시 고르지 않는다.

**REQ-SYSMETRICS-PANEL-001-08 — 항목 선택**
WHERE 패널 설정에 `items` 가 있을 때, 패널은 선택된 항목만 표시해야 한다(SHALL). 빈 배열은 "모두 껐다"는 정상 상태로 존중한다(monitor 패널과 동일 규약).

### Module 3: 네트워크 패널 (`sysmetrics-network`)

**REQ-SYSMETRICS-PANEL-001-09 — 인터페이스별 시리즈**
WHERE 네트워크 패널에 인터페이스가 선택되어 있을 때, 패널은 선택된 인터페이스마다 시리즈를 하나씩 겹쳐 그려야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-10 — 종합 시리즈**
IF 선택된 인터페이스가 없으면, 패널은 전체 합산 시리즈 하나만 그려야 한다(SHALL). 이것이 "네트워크 종합"이다.

**REQ-SYSMETRICS-PANEL-001-11 — rate 계산**
WHEN 연속한 두 표본을 받으면, 패널은 누적 카운터의 차분을 경과 시간으로 나눠 단위시간당 증가량을 계산해야 한다(SHALL). 단위시간은 초/분/시 중 설정으로 고른다.

**REQ-SYSMETRICS-PANEL-001-12 — 카운터 되감김 방어**
IF 새 표본의 누적값이 직전 값보다 작으면(인터페이스 재설정·재부팅), 패널은 그 구간의 rate 를 0 으로 처리해야 하며 음수 rate 를 그려서는 안 된다(SHALL NOT).

### Module 4: 스토리지 패널 (`sysmetrics-storage`)

**REQ-SYSMETRICS-PANEL-001-13 — 파티션별 표시**
WHERE 스토리지 패널에 마운트가 선택되어 있을 때, 패널은 선택된 마운트마다 사용률·사용량·여유·전체를 표시해야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-14 — 종합 표시**
IF 선택된 마운트가 없으면, 패널은 수집 중인 전체 마운트의 합계(사용량·여유·전체)와 그로부터 계산한 사용률을 표시해야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-15 — 중복 볼륨 표기**
WHERE 서로 다른 마운트가 같은 용량 수치를 갖는 경우(macOS 의 `/` 와 `/System/Volumes/Data` 처럼 한 볼륨의 여러 마운트), 패널은 각 마운트를 그대로 개별 표시해야 한다(SHALL). 합산에서의 중복 제거는 하지 않으며, 이 성질을 설명 문구로 알린다.

> 근거: 어떤 마운트가 같은 볼륨인지는 OS 마다 다르고 에이전트도 알지 못한다. 임의로 합치면 사용자가 고른 대상이 화면에서 사라진다. 종합을 정확히 보려면 에이전트 설정에서 마운트를 골라 두는 것이 옳은 해법이다.

### Module 5: 공통 패널 배선

**REQ-SYSMETRICS-PANEL-001-16 — 에이전트 바인딩**
WHERE 세 패널 중 하나를 추가할 때, 추가 다이얼로그는 sysmetrics 타입 에이전트 선택 스텝을 거쳐야 하며(SHALL), 선택 결과를 `agent_id` 정본 + `agent_name` 스냅샷으로 저장해야 한다.

**REQ-SYSMETRICS-PANEL-001-17 — 대상 선택 UI**
WHERE 패널 설정에서 대상(인터페이스·마운트·장치)을 고를 때, 시스템은 `SysResourceSelector` 를 재사용해야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-18 — 공통 표시 옵션**
세 패널은 monitor 패널과 동일한 어휘의 표시 옵션(`items`, `maxCols`, `refreshMs`, `windowSec`)을 지원해야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-19 — 카탈로그 등록**
WHERE 패널 추가 다이얼로그가 열릴 때, 세 패널은 `system` 카테고리 안에서 기존 monitor 5종과 **구분된 하위 그룹**으로 노출되어야 한다(SHALL).

**REQ-SYSMETRICS-PANEL-001-20 — 에이전트 부재 폴백**
IF 바인딩된 에이전트가 삭제되었거나 중지 상태이면, 패널은 빈 차트 대신 상태 안내를 표시해야 한다(SHALL). 오류로 대시보드 전체가 깨져서는 안 된다(SHALL NOT).

**REQ-SYSMETRICS-PANEL-001-21 — 비활성 지표 처리**
IF 에이전트 설정에서 해당 지표 수집이 꺼져 있으면(예: `CollectStorage: false`), 패널은 그 항목을 "수집 꺼짐" 으로 표시해야 하며 0 값으로 그려서는 안 된다(SHALL NOT).

> 근거: 0 으로 그리면 "디스크가 비어 있다" 와 "관측하지 않는다" 가 화면에서 구분되지 않는다.

---

## 4. Solution (해결 방안)

### 4.1 데이터 경로

```
sysmetrics agent (sampleLoop, 5s 기본)
        │
        ├─ 최신 스냅샷 저장 ──→ State() ──→ GET /agents/{id}?detail=full
        │                                          │
        │                                    패널 폴링(refreshMs)
        │                                          │
        │                              브라우저 창 누적(windowSec) + rate 차분
        │                                          │
        │                        sysmetrics-system / -network / -storage
        │
        └─ recvCh ──→ sysmetrics-in 노드 ──→ (기존 플로우 경로, 본 SPEC 범위 밖)
```

핵심은 갈래가 **스냅샷 저장 시점에서 갈린다**는 것이다. 채널 전송 성공 여부와 무관하게 스냅샷이 갱신되므로, 플로우를 만들지 않은 사용자도 패널만으로 지표를 본다.

### 4.2 패널 3종과 요청 7종의 대응

| 요청 항목 | 담당 패널 | 방식 |
|---|---|---|
| 시스템 · CPU | `sysmetrics-system` | `items` 에 포함 |
| 시스템 · 메모리 | `sysmetrics-system` | `items` 에 포함 |
| 시스템 · IO 종합 | `sysmetrics-system` | 전체 장치 합산 |
| 시스템 · 네트워크 종합 | `sysmetrics-system` | 전체 인터페이스 합산 |
| 네트워크 · 종합 | `sysmetrics-network` | 대상 미선택 = 합산 |
| 네트워크 · 인터페이스별 | `sysmetrics-network` | 대상 선택 |
| 스토리지 · 종합 | `sysmetrics-storage` | 대상 미선택 = 합계 |
| 스토리지 · 파티션별 | `sysmetrics-storage` | 대상 선택 |

"종합"과 "개별"을 별도 패널 유형으로 나누지 않고 **대상 선택의 결과**로 둔다. 같은 패널을 두 번 배치해 하나는 비우고 하나는 고르면 종합·개별이 나란히 놓인다.

### 4.3 파일 배치

```
internal/agent/system/
  sysmetrics_state.go          # 신규 — 스냅샷 보관 + State()
  sysmetrics_state_test.go     # 신규
  sysmetrics_agent.go          # 수정 — emitSample 에서 스냅샷 갱신

web/src/pages/dashboard/panels/sysmetrics/     # 신규 디렉터리
  SysMetricsSystemPanel.tsx
  SysMetricsNetworkPanel.tsx
  SysMetricsStoragePanel.tsx
  sysMetricsPanelConfig.ts     # 설정 읽기 (monitorPanelConfig 규약 준용)
  sysMetricsSeries.ts          # 스냅샷 → 시리즈 변환 + rate 차분 + 합산
  useSysMetricsSnapshot.ts     # 에이전트 폴링 훅
  *.test.tsx / *.test.ts

web/src/pages/dashboard/
  AddPanelDialog.tsx           # 수정 — system 카테고리에 하위 그룹 추가
  renderDashboardPanel.tsx     # 수정 — 3 타입 렌더 분기
  PanelSettingsDialog.tsx      # 수정 — 대상 선택 섹션
web/src/types/dashboard.ts     # 수정 — PanelType 3종 추가
web/src/lib/i18n/{ko,en}.json  # 수정 — 라벨·설명·항목 이름
```

---

## 5. Out of Scope (범위 밖)

- 영구 히스토리 조회 (Store/TSDB 경로) — 저장 규약이 별도 결정 사항이다.
- 원격 호스트 지표 수집 — 저장 경로에서 다룬다.
- 기존 monitor 패널 5종의 CPU·메모리 소스 교체 — 사용자 결정에 따라 분리 유지한다. 다만 이 SPEC 이 완료되면 monitor 패널의 CPU 가 0 인 문제는 **여전히 남으며**, 후속 SPEC 후보로 기록한다.
- 임계값 알림 / 경보.
- 프로세스별 자원 사용량.

---

## 6. Risks (위험)

| 위험 | 영향 | 완화 |
|---|---|---|
| 표본 주기(기본 5s)와 패널 폴링 주기가 어긋나 같은 표본을 중복 수신 | rate 가 0 으로 계단화 | `collected_at` 이 직전과 같으면 새 점을 추가하지 않는다 |
| 여러 패널이 같은 에이전트를 각자 폴링 | 중복 요청 | 에이전트별 쿼리 키를 공유해 캐시 하나를 나눠 쓴다 |
| 마운트가 많은 호스트(macOS 10+개) | 스토리지 패널이 읽히지 않음 | 대상 선택으로 좁히도록 유도하고, 기본은 종합 표시 |
| `State()` 가 큰 맵을 반환해 에이전트 목록 응답이 무거워짐 | 목록 화면 지연 | **실측 정정**: 웹 에이전트 목록은 `detail=summary` 로 호출하고(`agentService.ts:20`), `state` 는 summary 에도 실린다(`agent_adapter.go:620`). 즉 sysmetrics 에이전트마다 스냅샷이 목록 응답에 함께 나간다. 마운트 10개 기준 약 2KB/에이전트이고 `State()` 자체는 저장된 스냅샷을 읽을 뿐이라 수집 비용은 없다. 어댑터를 고쳐 요약에서 떼어내는 대신 이 비용을 받아들인다 — 에이전트 목록은 크지 않고, 떼어내면 패널이 `full` 을 써야 해 응답이 오히려 커진다 |
