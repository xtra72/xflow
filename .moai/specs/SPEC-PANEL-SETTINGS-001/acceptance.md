---
id: SPEC-PANEL-SETTINGS-001
version: "0.5.0"
status: draft
created: 2026-08-08
updated: 2026-08-11
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
- **Then** 미리보기 · 옵션 · 데이터소스 3영역이 모두 존재하고, 기본 배치(**좌측 컬럼 상단=미리보기 / 좌측
  컬럼 하단=데이터소스 / 우측=옵션**)로 표시되며, 기존 옵션/데이터소스 편집 기능이 손실 없이 각 영역에 배치된다

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

## M6 — 데이터 소스 시리즈 선택 UI 개선

### AC-17 (REQ-15) 컬럼 필터 OR/AND 결합
- **Given** 시리즈 선택 테이블에 다음 행이 있다 (metric 컬럼 / tag `floor` 컬럼):
  - R1: metric=`temp`, floor=`1F`
  - R2: metric=`humidity`, floor=`1F`
  - R3: metric=`co2`, floor=`1F`
  - R4: metric=`temp`, floor=`2F`
- **When** 사용자가 metric 필터에 `temp` **와** `humidity` 를 선택(같은 컬럼 다중값)하고, 동시에 floor 필터에
  `1F` 를 선택(다른 컬럼)한다
- **Then** 결과는 `(metric ∈ {temp, humidity}) AND (floor = 1F)` → **R1, R2** 만 표시된다
  - **And** R3(metric=co2)은 metric OR 집합 불충족으로 제외, R4(floor=2F)는 floor AND 조건 불충족으로 제외
- **And (Edge)** metric 필터만 `temp` 로 설정하고 floor 필터를 비우면 floor 는 AND 결합에서 제외되어
  `metric = temp` 인 **R1, R4** 가 표시된다
- **And (하위호환)** 필터를 하나도 설정하지 않으면 기존 동작과 동일하게 전체 행이 표시된다
- **And (표시 필터 분리)** 태그(floor) 필터는 이 통일된 **표시(display) 필터**의 일부로만 작동하며,
  단독으로 패널을 동적 바인딩 모드(`selection_mode:'tag'`)로 전환하지 않는다. 바인딩(선택)은 명시적 체크박스로
  유지되고, 동적 바인딩 활성화는 REQ-22 토글로만 제어된다(AC-24 참조)

### AC-17b (REQ-15) 태그 키(종류)별 OR/AND 결합
- **Given** 시리즈 선택 테이블에 다음 행이 있다 (태그 키 `device_id`, `type`):
  - S1: device_id=`A`, type=`report`
  - S2: device_id=`B`, type=`report`
  - S3: device_id=`C`, type=`report`
  - S4: device_id=`A`, type=`command`
- **When** 사용자가 `device_id` 필터에 `A` **와** `B` 를 선택(같은 태그 키 다중값)하고, `type` 필터에 `report`
  를 선택(다른 태그 키)한다
- **Then** 결과는 `(device_id ∈ {A, B}) AND (type = report)` → **S1, S2** 만 표시된다
  - **And** S3(device_id=C)은 device_id OR 집합 불충족, S4(type=command)는 type AND 조건 불충족으로 제외
- **And** "태그" 전체를 한 컬럼으로 뭉쳐 모든 `key=value` 를 OR 하지 않는다 — 각 태그 키가 자기 AND 차원이다
- **And (하위호환)** `tag_filters` 스키마는 `Record<string,string>` 로 유지된다(다중값-per-키는 표시 필터 상태;
  동적 바인딩 파생 시 태그 키당 best-effort last-value)

### AC-18 (REQ-16) "이름(name)" 표기
- **Given** 시리즈 선택/세부 정보 UI
- **When** 컬럼 헤더·세부 편집 라벨이 렌더된다
- **Then** "별칭"/"alias" 대신 "이름"/"Name" 이 표시된다
- **And** 저장된 config 의 `StoreSeriesRef.alias` 필드명은 변경되지 않고, 기존 config 파싱이 그대로 동작한다(표기만 변경)

### AC-19 (REQ-17) 컬럼 순서 재배치
- **Given** 시리즈 선택 테이블이 렌더된 상태
- **When** 컬럼 헤더를 좌→우로 읽는다
- **Then** **키(key) · 이름(name) · 메트릭(metric) · 태그(tag)** 순서로 배치된다
- **And (Edge)** 태그가 없는 store 에서는 태그 컬럼이 생략되어도 나머지 순서(key · name · metric)가 유지된다

### AC-20 (REQ-18) 선택 행 인라인 펼침 세부 정보 + 레이아웃
- **Given** 시리즈 선택 테이블에서 일부 행이 체크(선택)된 상태
- **When** 선택된 어떤 행의 펼침(expand) 액션을 실행한다
- **Then** 그 행 안에 인라인으로 이름(편집)·색상·선 스타일·(heatmap 시)좌표 **편집 필드만** 노출된다
- **And** 이름(name) 필드는 **편집 가능한 텍스트 입력**이며, 편집 시 해당 시리즈 `StoreSeriesRef.alias` 에 반영되고
  미리보기·차트 범례/표시에 반영된다
- **And (레이아웃)** 이름 필드 **옆의 색상 입력은 없으나**, 시리즈별 **색상 편집은 여전히 가능**하다(색상 컨트롤이
  선 스타일/세부 영역의 다른 위치에 존재; `color` config 값은 편집 가능)
- **And (레이아웃)** (heatmap)좌표(x/y)는 하단 별도 배치가 아니라 **이름 뒤 2열(이름 | 좌표)**로 배치된다
- **And (레이아웃)** 세부 편집 텍스트/라벨 폰트는 **최소 본문(sm) 수준 이상**으로 가독성 있게 표시된다
- **And** 접으면 그 행의 세부 편집 필드가 숨겨진다(기본 접힘)
- **And** **미선택(미체크) 행은 펼침 상세가 제공되지 않는다**
- **And** 편집 항목을 담는 **별도 그룹/섹션이 존재하지 않는다**(선택 행 인라인 펼침이 유일 편집 지점)

### AC-21 (REQ-19 폐지) 키 설명 서브라인 제거
- **Given** 선택된 시리즈 행을 펼친 상태
- **When** 펼침 상세 내용을 본다
- **Then** 이름 위에 키·종류(type)·태그를 나열하는 **설명 서브라인이 존재하지 않는다**(REQ-19 폐지)
- **And** 펼침 상세에는 시리즈 편집 필드(이름/색상/선 스타일/(heatmap)좌표)만 있다
- **And** 키는 **키(key) 컬럼 + 이름(alias) 컬럼의 mono 키 에코**로만 노출되며, 펼침에서 중복 표시되지 않는다
- **And** 별도의 "키 상세만" 펼침(`keyRowExpansion`)도 존재하지 않는다

### AC-22 (REQ-20) 별도 SelectedSeriesList 섹션 부재
- **Given** 시리즈를 체크하여 선택한 상태
- **When** 세부 편집(이름/색상/선 스타일)을 찾는다
- **Then** 편집은 오직 선택 행의 인라인 펼침에서만 이루어진다
- **And** 별도 `SelectedSeriesList` 섹션(그룹)이 **존재하지 않는다**(단일 편집 지점=행 펼침, 중복 노출 없음)

### AC-23 (REQ-21) 센서 좌표 → 선택 행 펼침 통합 (heatmap 전용)
- **Given** 편집 대상이 heatmap 패널이고 시리즈가 선택된 상태
- **When** 선택 행을 펼쳐 센서 좌표(x/y) 편집을 찾는다
- **Then** 좌표 편집이 별도 heatmap 영역이 아니라 그 선택 행의 인라인 펼침 안에서 해당 시리즈와 함께 편집된다
- **And (Edge)** 차트 5종(비 heatmap) 패널에서는 센서 좌표 필드가 노출되지 않는다

### AC-24 (REQ-22) 명시적 동적 바인딩 토글 + 표시/바인딩 분리
- **(a) 신규 패널 기본 OFF = keys/명시 선택**
  - **Given** 새로 생성한 Store 바인딩 패널
  - **When** 데이터소스 영역이 렌더된다
  - **Then** 동적 바인딩 토글이 **OFF** 이고 `selection_mode` 는 `'keys'` 이며, 체크박스로 선택한 `series[]` 만
    바인딩된다
  - **And** 이 상태에서 태그 컬럼 필터를 설정해도 표시 행만 좁혀질 뿐 `selection_mode` 는 `'keys'` 로 유지된다
    (표시/바인딩 분리)
- **(b) 토글 ON = 동적 바인딩으로 매칭 키 해석**
  - **Given** 동적 바인딩 토글이 표시된 상태
  - **When** 사용자가 토글을 **ON** 으로 켜고 태그-컬럼 필터로 태그 기준을 지정한다
  - **Then** `selection_mode` 가 `'tag'` 로 전환되고, 태그 기준이 `tag_filters` 로 기록되어 poll 시점에 매칭 키가
    동적으로 해석된다(`useStoreChartData.ts:299/372` 경로)
- **(c) 하위호환: 기존 tag 모드 패널 로드**
  - **Given** `selection_mode:'tag'` + `tag_filters` 로 저장된 기존 패널
  - **When** 그 패널 설정을 연다
  - **Then** 동적 바인딩 토글이 **기본 ON** 으로 로드되고 `tag_filters` 가 보존되어, 기존 태그 모드 동작이
    바뀌지 않는다
  - **And** `selection_mode`/`tag_filters` 필드는 제거되지 않는다(additive only)
- **(d) 세부 정보 ↔ 바인딩 모드 독립**
  - **Given** 선택/바인딩된 시리즈가 있는 상태
  - **When** 동적 바인딩 토글을 OFF(keys) 로 두거나 ON(tag) 으로 켠다 — 어느 쪽이든
  - **Then** 두 경우 모두 해당 시리즈의 세부 정보(행 인라인 펼침 편집: 이름/색상/선 스타일/(heatmap)위치)에
    접근·편집할 수 있다
  - **And** 세부 정보 편집이 바인딩 모드에 의해 숨겨지거나 게이팅되지 않는다(현행 keys-모드 게이트 제거)
  - **And (tag 모드 best-effort)** 동적 바인딩 ON 에서 세부 스타일/이름 편집은 현재 해석된 key 기준으로 반영된다
    (design.md §8 영속 고려사항 참조)

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

- REQ-01~22 전량 구현(REQ-19 폐지) + AC-01~24(AC-17b 포함) 통과.
- 신규 npm 의존성 0, 백엔드 무변경(불투명 JSON config 유지).
- 색상 프리셋 heatmap 전용, TSDB 실동작 미구현(placeholder + 후속 안내).
- M6: 통일된 표시 필터 — 같은 컬럼/태그 키 OR·컬럼/태그 키 간 AND, **태그는 태그 키(종류)별 차원**(태그 전체를
  한 컬럼으로 뭉치지 않음), 미설정 시 기존 동작. "이름" 표기(필드 `alias` 불변), 컬럼 순서 key·name·metric·tag.
- M6 세부 정보 배치·정제(v0.5.0): 시리즈별 세부 정보 = **선택된 행의 인라인 펼침 상세**, 내용은 **편집 필드만**
  (이름 편집/색상/선 스타일/(heatmap)좌표) — 이름 위 키·종류·태그 설명 라인 제거(REQ-19 폐지). 레이아웃: 이름 옆
  색상 입력 제거(색상 편집은 유지·이동), 좌표 이름 뒤 2열, 폰트 최소 sm. 별도 `SelectedSeriesList`/`StoreSourceEditor`
  섹션 제거, 미선택 행 펼침 없음. 세부 정보는 바인딩 모드(keys/tag)와 독립(keys-모드 게이트 제거).
- M6 표시/바인딩 분리: 태그 필터는 표시 필터 전용(모드 자동 전환 없음), 동적 바인딩은 명시적 토글로만
  제어(신규 OFF/keys, ON/tag), 기존 `selection_mode:'tag'` 패널은 토글 ON+`tag_filters` 보존 로드(additive only).
- tsc `--noEmit`/eslint 0, 신규 커버리지 ≥ 85%, 에이전트 상세·기존 설정 회귀 0.
