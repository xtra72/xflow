---
id: SPEC-MODBUS-012
title: "MODBUS Gateway 대시보드 패널 스위트 — 인수 기준"
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
tags: "modbus, gateway, dashboard, panel, acceptance, gherkin, given-when-then"
---

# SPEC-MODBUS-012 — 인수 기준 (acceptance.md)

> 형식: Given-When-Then. 프론트 AC는 vitest + React Testing Library, 백엔드 AC는 `go test -race` 로 검증한다.

## REQ-01 — 패널 프레임워크 통합

### AC-01 (6종 PanelType 등록)
- **Given** 대시보드 편집 모드에서 패널 추가 다이얼로그를 연다
- **When** data 카테고리를 본다
- **Then** 6종 옵션(`modbus-real-devices`, `modbus-virtual-devices`, `modbus-shared-registers`, `modbus-device-registers`, `modbus-bus-stats`, `modbus-summary-stats`)이 ko/en 라벨·설명과 함께 표시된다

### AC-02 (에이전트 선택 스텝 + config 저장)
- **Given** 실행 중 `modbus-gateway` 에이전트 2개, `modbus-client` 1개가 존재한다
- **When** 6종 중 하나를 선택한다
- **Then** 에이전트 선택 스텝에 `modbus-gateway` 에이전트 2개만 나열되고, 선택 완료 시 `config.agentId`가 저장된 패널이 활성 대시보드에 추가된다 (가상 디바이스 레지스터 맵은 unit 2차 선택 후 `config.unitId`도 저장)

### AC-03 (미설정/부재 graceful)
- **Given** `config.agentId`가 비었거나 대상 에이전트가 정지 상태다
- **When** 패널이 렌더된다
- **Then** "에이전트 미설정" 또는 빈 상태 안내가 표시되고 콘솔 에러/크래시가 없다

## REQ-02 — 실제 연결 디바이스 목록 패널

### AC-04 (백킹 디바이스 필터)
- **Given** 게이트웨이에 백킹 디바이스 2개(direct 1, indirect 1)와 순수 slave 1개가 있다
- **When** 실제 디바이스 목록 패널이 폴링한다
- **Then** `list_devices`의 `backed=true` 항목 2개만 표시되고 순수 slave는 제외된다

### AC-05 (upstream 메트릭 표시)
- **Given** 백킹 디바이스가 upstream 요청을 처리했다
- **When** 패널이 `get_device_status`를 조회한다
- **Then** 각 디바이스 행에 mode(direct/indirect), slave id(upstream unit), request_count, error_count, avg_latency_ms가 표시된다

### AC-06 (연결/stale degraded 구분)
- **Given** indirect 백킹 디바이스의 `last_ok`가 timeout을 초과했다
- **When** 패널이 렌더된다
- **Then** 해당 디바이스가 degraded(stale) 스타일로 구분 표시된다

## REQ-03 — 가상 디바이스 목록 패널

### AC-07 (U01~ 목록 + 영역 배지)
- **Given** 게이트웨이에 가상 디바이스 U01, U02가 있다
- **When** 가상 디바이스 목록 패널이 폴링한다
- **Then** 각 디바이스가 unit_id, name, `register_counts` 기반 영역 배지(CO/DI/IR/HR)와 함께 나열된다

### AC-08 (active/stale 판정)
- **Given** U01은 최근 read/write 접근이 있었고 U02는 무접근이다
- **When** 폴링 델타를 계산한다
- **Then** U01은 active, U02는 stale 상태로 표시된다

## REQ-04 — 공유 레지스터 맵 패널

### AC-09 (unit 0 그리드 렌더)
- **Given** 게이트웨이에 unit 0 공유 컨테이너가 구성되어 있다
- **When** 공유 레지스터 맵 패널이 `get_map`(`unit_id=0`)을 조회한다
- **Then** 4영역(Coils/Discrete Inputs/Input Registers/Holding Registers) 그리드가 스냅샷 값으로 렌더된다

### AC-10 (값 기반 셀 상태)
- **Given** holding_registers 스냅샷에 값 0(inactive), 123(active), count>0이나 스냅샷 부재 주소(degraded)가 있다
- **When** 그리드가 셀 상태를 판정한다
- **Then** 각 셀이 값 기반 규칙(≠0 active / 0 inactive / 범위 밖·에러 degraded)에 따라 색상 구분된다

### AC-11 (영역 집계)
- **Given** 각 영역의 `register_counts`와 스냅샷이 있다
- **When** 그리드 헤더가 렌더된다
- **Then** 영역별 POINTS(정의 개수)/ACTIVE/DEGRADED 집계가 표시된다

### AC-11b (공유 컨테이너 부재)
- **Given** 공유 컨테이너가 미구성이다(`get_map` unit 0가 ErrNoSharedContainer)
- **When** 패널이 렌더된다
- **Then** 빈/안내 상태가 표시되고 크래시가 없다

## REQ-05 — 가상 디바이스 레지스터 맵 패널

### AC-12 (unit N 그리드 렌더)
- **Given** `config.unitId=2`인 가상 디바이스가 있다
- **When** 패널이 `get_device_status`(`unit_id=2`)를 조회한다
- **Then** 해당 디바이스의 레지스터 맵이 4영역 그리드로 렌더된다

### AC-13 (판정 로직 공유)
- **Given** REQ-04와 동일한 값 조합이다
- **When** 셀 상태를 판정한다
- **Then** REQ-04와 동일한 공유 유틸(`registerCellState`)로 동일 결과가 나온다(중복 구현 없음)

## REQ-06 — 버스/종합 통계 + 백엔드 메트릭

### AC-14 (백엔드 backing 카운터)
- **Given** direct 백킹 디바이스가 upstream 조회 5회(성공 4, 실패 1)를 수행했다
- **When** `get_device_status`(`unit_id=N`)를 호출한다
- **Then** `backing` 서브객체가 `{mode:"direct", request_count:5, error_count:1, avg_latency_ms>0, last_ok, connected}` 를 반환하고, 순수 slave는 `backing:null`을 반환한다

### AC-15 (list_devices backed/mode)
- **Given** 백킹 1개, 순수 slave 1개가 있다
- **When** `list_devices`를 호출한다
- **Then** 각 항목에 `backed`(bool)·`mode`가 포함되고 값이 올바르다

### AC-16 (카운터 동시성)
- **Given** indirect 폴러가 주기 폴링 중이고 동시에 마스터 서빙 요청이 들어온다
- **When** `go test -race`로 백킹 카운터를 검증한다
- **Then** 데이터 레이스가 검출되지 않고(atomic) 카운트가 유실 없이 누적된다

### AC-17 (종합 통계 바)
- **Given** 게이트웨이에 active_connections=3, clients=2, 디바이스 4개가 있다
- **When** 종합 통계 패널이 `get_status`+`list_devices`+`list_clients`를 조회한다
- **Then** active_connections, clients, uptime, 총 디바이스, 총 reads/writes/errors가 요약 바에 표시된다

### AC-18 (버스 미니차트 델타 누적)
- **Given** 연속 두 폴링 사이 총 read_count가 60 증가했다
- **When** 버스 통계 패널이 델타를 누적한다
- **Then** reads/min 계열이 미니차트에 추가되고 기존 차트 프리미티브로 렌더된다(신규 차트 라이브러리 없음)

## REQ-07 — 하위 호환·품질

### AC-19 (기존 패널 회귀 0)
- **Given** 기존 대시보드 패널(flows/agents/device/facility 등)이 있다
- **When** 본 SPEC 변경 후 기존 패널 테스트를 실행한다
- **Then** 모든 기존 테스트가 통과하고 렌더 결과가 회귀하지 않는다

### AC-20 (type id 불변 + 스냅샷 경로 불변)
- **Given** 게이트웨이 exec 명령 세트가 있다
- **When** 기존 특성화 테스트를 실행한다
- **Then** type id `modbus-gateway`/`modbus-client`가 불변이고 `get_map`/`get_*` 응답 형상이 변경되지 않는다

### AC-21 (신규 의존성 0)
- **Given** 변경된 `package.json`과 `go.mod`가 있다
- **When** 의존성 diff를 확인한다
- **Then** 신규 npm(modbus/차트) 및 go.mod modbus 모듈이 추가되지 않았다

### AC-22 (품질 게이트 + i18n 정합)
- **Given** 전체 변경이 완료되었다
- **When** tsc/vitest/`go test -race`/커버리지를 실행한다
- **Then** 타입/테스트 클린, 커버리지 ≥85%, ko/en i18n 키가 1:1 정합하며 레지스터 쓰기 경로가 없다(관측 전용)
