---
id: SPEC-PANEL-SETTINGS-001
version: "0.1.0"
status: draft
created: 2026-08-08
updated: 2026-08-08
author: xtra
priority: P2
lifecycle_level: spec-anchored
title: "Panel Settings 재설계 — 인수 기준 (acceptance)"
phase: plan
module: web/dashboard
tier: L
tags: [dashboard, panel, settings, acceptance, gherkin, trust5, frontend]
---

# SPEC-PANEL-SETTINGS-001 — 인수 기준 (acceptance.md)

> Given/When/Then. 각 EARS 모듈(M1~M5) 최소 커버 + 엣지 케이스 + TRUST5/hybrid 품질 게이트.

## M1 — 3분할 레이아웃 셸

### AC-01 (REQ-01) 3분할 셸 구성
- **Given** Store 바인딩 패널(예: line-chart)의 설정을 연다
- **When** 설정 셸이 렌더된다
- **Then** 미리보기 · 옵션 · 데이터소스 3영역이 모두 존재하고, 기본 배치(1/4 미리보기 · 2/4·3/4 옵션 · 4/4
  데이터소스)로 표시되며, 기존 옵션/데이터소스 편집 기능이 손실 없이 각 영역에 배치된다

### AC-02 (REQ-02) 리사이즈 + 영속
- **Given** 3분할 셸이 표시된 상태
- **When** 사용자가 좌우(또는 상하) 경계를 드래그하여 놓는다
- **Then** 인접 영역 크기가 실시간 조정되고, 드래그 종료 시 갱신 비율이 **해당 패널의** localStorage 키에 저장된다

### AC-03 (REQ-03) 비율 복원
- **Given** 특정 패널에 저장된 비율이 localStorage 에 존재
- **When** 그 패널의 설정을 다시 연다
- **Then** 저장된 비율로 3분할 셸이 복원된다
- **Edge** 저장 값이 없거나 손상(파싱 실패)이면 기본 비율로 폴백하고 예외를 던지지 않는다

## M2 — 데이터소스 선택

### AC-04 (REQ-04) Store 선택 → config 반영
- **Given** 데이터소스 영역에서 Store 가 선택된 상태
- **When** 사용자가 store 리스트에서 대상을 선택한다
- **Then** 선택 결과가 패널의 `StoreSourceConfig` 로 반영되고 미리보기가 갱신된다

### AC-05 (REQ-05) TSDB 비활성 안내 (Edge)
- **Given** 데이터소스 토글에 Store/TSDB 가 노출된 상태
- **When** 사용자가 TSDB 를 선택한다
- **Then** 실동작 대신 비활성 안내 placeholder(후속 SPEC 안내)가 표시되고, 기존 Store 설정은 파괴되지 않으며
  잘못된 빈 데이터가 렌더되지 않는다

## M3 — 공용 Store 리스트

### AC-06 (REQ-06) 동일 컬럼·렌더
- **Given** 패널 설정의 데이터소스 영역
- **When** Store 리스트가 렌더된다
- **Then** 에이전트 상세와 동일한 `STORE_COLUMNS` 정의·정렬 비교자(`storeEntrySort`)를 사용하는 공용
  `StoreEntryTable` 로 표시된다

### AC-07 (REQ-07) 행 선택 체크박스
- **Given** 공용 리스트가 표시된 상태
- **When** 사용자가 행 선행 체크박스를 토글한다
- **Then** 해당 엔트리가 선택 집합에 추가/제거되고 선택 집합이 `StoreSourceConfig`(선택 계열)로 반영된다

### AC-08 (REQ-08) 필터/정렬/표시숨김 영속
- **Given** 공용 리스트에서 컬럼 필터·정렬·표시숨김을 설정
- **When** 설정을 닫았다가 재진입한다
- **Then** 저장된 필터/정렬/표시숨김 상태가 복원된다

### AC-09 (REQ-09) actions→Alias 치환
- **Given** 패널 설정 컨텍스트의 공용 리스트
- **When** 컬럼이 렌더된다
- **Then** 에이전트 상세의 `actions` 컬럼 대신 **Alias Name** 컬럼이 표시된다
- **And** 에이전트 상세 화면 컨텍스트에서는 여전히 `actions` 컬럼이 표시된다(컨텍스트별 컬럼 세트)

### AC-10 (엣지) 빈 store
- **Given** 바인딩 가능한 store 엔트리가 없음
- **When** 데이터소스 영역이 렌더된다
- **Then** 빈 리스트를 graceful 하게 안내하고 예외를 던지지 않는다

## M4 — 색상 프리셋 (heatmap 전용)

### AC-11 (REQ-10/11) 프리셋 제공·적용
- **Given** 편집 대상이 heatmap 패널
- **When** 옵션 영역을 렌더하고 사용자가 gradient 프리셋(예: Viridis)을 선택한다
- **Then** 4~5종 이름있는 프리셋 + 커스텀 ColorStop 편집이 제공되고, 선택한 프리셋의 ColorStop 배열이
  heatmap draft 색상표로 반영되어 미리보기가 갱신된다

### AC-12 (REQ-12) heatmap 외 미노출 (Edge)
- **Given** 편집 대상이 차트 5종(예: bar-chart)
- **When** 옵션 영역을 렌더한다
- **Then** 색상 프리셋 UI 가 노출되지 않는다

## M5 — 라이브 미리보기

### AC-13 (REQ-13) 즉시 갱신 (실 데이터·디바운스)
- **Given** 설정 셸이 열린 상태
- **When** 사용자가 옵션·색상·데이터소스를 변경한다
- **Then** 변경이 디바운스되어 미리보기의 실 패널 렌더가 draft config + 실 store 데이터로 갱신된다

### AC-14 (REQ-14) 미확정·저장·롤백
- **Given** 편집으로 draft 상태가 committed 와 다름
- **When** 사용자가 취소한다
- **Then** draft 가 폐기되고 committed 상태로 롤백된다
- **And** 저장 시 draft 가 committed 로 승격된다

### AC-15 (엣지) 선택 상한
- **Given** 사용자가 많은 계열을 선택하려 함
- **When** 선택이 합리적 상한(예: 수십 개)을 초과한다
- **Then** 상한 초과가 안내되고 미리보기 성능이 보호된다

## 회귀 (Regression)

### AC-16 기존 설정 회귀 0
- **Given** `StoreEntryTable` 추출 + 셸 골격 교체 이후
- **When** 에이전트 상세 Store 리스트, 기존 차트 5종/heatmap 설정 편집을 사용한다
- **Then** 기존 특성화 테스트가 모두 통과하고 관측 가능한 동작 회귀가 없다(회귀 0)

## 품질 게이트 (TRUST5 / hybrid)

- **Tested**: 신규 코드 커버리지 ≥ 85%. 에이전트 상세 Store 리스트 특성화 테스트(추출 전 스냅샷) 통과. 신규
  순수/독립 코드(프리셋 상수·비율 훅·draft 상태)는 TDD.
- **Readable**: 명확한 네이밍, 컨텍스트 주입 경계 문서화(design.md 공유 계약).
- **Unified**: 프로젝트 포매터/린트 규칙 준수, 기존 파일 스타일 일치.
- **Secured**: 입력(localStorage 파싱) 방어, TSDB placeholder 로 미검증 데이터 렌더 차단.
- **Trackable**: SPEC-PANEL-SETTINGS-001 참조 커밋, REQ↔파일 추적성(spec.md §추적성).
- **LSP**: run 단계 error/type/lint 0 (quality.yaml lsp_quality_gates.run.max_* = 0).

## Definition of Done

- REQ-01~14 전량 구현 + AC-01~16 통과.
- 신규 npm 의존성 0, 백엔드 무변경(불투명 JSON config 유지).
- 색상 프리셋 heatmap 전용, TSDB 실동작 미구현(placeholder + 후속 안내).
- tsc `--noEmit`/eslint 0, 신규 커버리지 ≥ 85%, 에이전트 상세·기존 설정 회귀 0.
