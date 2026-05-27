---
id: SPEC-WEB-001
version: "1.33.0"
status: completed
created: "2026-03-07"
updated: "2026-05-23"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-05-23 | 1.33.0 | **HVAC 제어 패널 mode/fan_speed 표시 fix + hvac 통일 ID 정규화**. (1) `AcControlPanel` / `OutdoorControlPanel` / `DeviceDetailPanel` 등에서 v0.14.x 백엔드 필드명 통일 (`current_temp` → `current_temperature`, `*_temp` → `*_temperature`) 후 잔존하던 스탈 키 참조 일괄 갱신 — LGCNP ODU 6개 온도 키 (`outdoor_temp`, `comp_*_temp`, `condenser_temp_*`, `avg_temp`) → `*_temperature`. `deviceLabels.ts` 의 NASA/LGAP/LGCP/LGCNP 라벨 맵과 `PROPERTY_ORDER`, `nodeSchemas` 의 outputDesc 예제, `nodeTypeMeta` deduplicate `compare_fields` 예제 갱신. (2) v0.7.5 부터 백엔드가 mode / fan_speed 를 hvac 통일 ID (int) 로 emit 함에 따라 프론트엔드의 `'cool' === 1` 비교 실패로 mode/풍량 표시 안 됨 문제 해결. `acControlTypes.ts` 에 `normalizeAcMode` / `normalizeFanSpeed` 헬퍼 신설 (int + 레거시 string + `cooling/heating/dehumidify/slow/off` 별칭 호환). `AcControlPanel` / `DeviceDetailPanel` 의 props 읽기 + `deviceLabels.formatPropertyValue` 의 속성 테이블 표시 모두 정규화 적용. |
| 2026-05-23 | 1.32.0 | **mqtt-publisher default_topic JSONPath 보간 + dashboard 패널 키 동기화**. `default_topic` description 갱신 (예: `xflow/{$.metadata.device_type}/{$.metadata.device_id}/status`). |
| 2026-03-07 | 1.0.0 | 초기 SPEC 작성 - 에이전트 통계 버그 수정 + 컴포넌트별 로그 레벨 제어 |
| 2026-03-07 | 1.1.0 | Module 4 추가: 플로우 노드 통계 + 로그 레벨 UI. M1 근본 원인 백엔드로 수정 |
| 2026-03-07 | 1.2.0 | Module 5 추가: 캔버스 런타임 통계. M4 In/Out 분리 표시 + 실시간 갱신. 백엔드 포트 카운터/브릿지 버그 수정 |
| 2026-03-08 | 1.3.0 | Module 7 추가: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링. SPEC-OBS-004 연동 |
| 2026-03-09 | 1.4.0 | Module 8 추가: 동적 포트 시스템. 노드 타입/설정 기반 포트 동적 계산, 브릿지 방향별 포트, 스위치 라우트별 출력 포트 |
| 2026-03-09 | 1.5.0 | Module 9 추가: 에러 포트 타입 지원. 프론트엔드 PortDef/NodeTypeDefinition에 'error' direction 추가, CustomNode 에러 포트 렌더링(빨간색, 하단 배치), NodeHandle 에러 포트 색상 |
| 2026-03-09 | 1.6.0 | Module 10 추가: Handle ID 접두사 제거. 포트 이름을 Handle ID로 직접 사용하여 프론트엔드/백엔드 Handle↔Edge 매핑 간소화. Module 9 실제 구현 반영(에러 포트 Right position + offset 배치). PropertyPanel Port 타입에 'error' 추가 |
| 2026-03-09 | 1.7.0 | Module 11 추가: 리스트 정렬 기능. FlowListPage/AgentListPage 컬럼 정렬 지원, 이름 기본 정렬, 백엔드 ListOptions.Sort 구현, 프론트엔드 정렬 UI 컴포넌트 |
| 2026-03-10 | 1.8.0 | Module 12 추가: 대시보드 패널 재구성. 2x2 위젯 그리드 → 3패널 구조(FlowPanel + AgentPanel + ResourcePanel). 플로우/에이전트 상태 요약 + 리스트 테이블 통합 패널, 시스템 리소스 패널 |
| 2026-03-10 | 1.9.0 | Module 13 추가: Import/Export 기능. 플로우/에이전트를 JSON/YAML 파일로 내보내기(Export) 및 가져오기(Import). CLI 호환 포맷, ImportDialog 공용 모달, 클라이언트 사이드 파일 파싱, 이름 충돌 방지 |
| 2026-03-17 | 1.10.0 | Module 14 추가: 에이전트 연결 노드 표시(LinkedNodesSection), 연결 상태 중복 제거, DynamicForm visibleWhen 조건부 필드, console-logger 출력 설정 스키마, output 노드 스키마 |
| 2026-03-17 | 1.11.0 | Module 15 추가: 화면 테마 시스템. CSS Variable 디자인 토큰, Day/Night 프리셋, Custom 테마 지원, 테마 선택 UI, 커스텀 테마 에디터, dark: 클래스 마이그레이션 |
| 2026-03-17 | 1.12.0 | Module 16 추가: 대시보드 커스터마이징 시스템. 멀티 대시보드 페이지, 패널 추가/삭제, 디바이스/로그 패널 타입, 대시보드 관리 툴바, Zustand persist 마이그레이션 |
| 2026-03-17 | 1.13.0 | Module 17 추가: UI 레이아웃 재구성. 대시보드 관리 UI를 DashboardToolbar에서 Header.tsx로 이전(라우트 조건부 렌더링). 패널 추가/초기화 버튼을 별도 설정 바에서 조작 바 편집 토글 앞으로 인라인 이동. 디바이스 페이지도 동일 패턴 적용. DashboardToolbar.tsx 삭제. console-logger Process 메서드 로그 레벨 버그 수정(Debug→Info) |
| 2026-03-17 | 1.14.0 | Module 18 추가: 플로우 리스트 개선. 업타임 컬럼 추가(running 상태만 표시), 자동 시작 토글 스위치(flow metadata auto_start 기반), 서버 부팅 시 auto_start 플로우 자동 배포/시작. 로그 레벨 UI 위치 이동(StatsTab→ConfigTab). 다크 모드 확장 패널 배경 수정(bg-gray-800/50→bg-(--color-bg-sunken)). LGAP 프로토콜 표시 수정(NASADeviceAdapter Protocol/ExtraProperties 오버라이드) |
| 2026-03-17 | 1.15.0 | Module 19 추가: 플로우 Undeploy API(POST /flows/{id}/undeploy) + FlowActionMenu 상태별 액션 제어(상태 기반 canStart/canStop/canDeploy/canUndeploy), Deploy(Cable)/Undeploy(Unplug) 아이콘, 대시보드 패널 상태 아이콘 표시(FlowPanel/AgentPanel/DevicePanel 텍스트→아이콘) |
| 2026-03-17 | 1.16.0 | Module 20 추가: 에이전트 연결 상태 정확성 개선. TransportChecker 인터페이스 도입으로 에이전트 connected 필드가 실제 트랜스포트 연결 상태 반영. 미연결 시 uptime 숨김. 프론트엔드 상태 아이콘/액션 버튼 connected 필드 기반 통일 |
| 2026-03-17 | 1.17.0 | Module 21 추가: 플로우 리스트 상태 아이콘 + 상태 필터링. FlowStatusBadge를 대시보드 FlowPanel과 동일한 아이콘 기반 렌더링으로 교체. 상태 필터를 드롭다운에서 아이콘 뱃지 토글 버튼으로 변경 |
| 2026-03-18 | 1.18.0 | Module 22-25 추가: 패널 설정 다이얼로그(PanelSettingsDialog 모달, 컬럼 가시성/디바이스 필터/로그 설정), 악센트 컬러 시스템(요소별 악센트 컬러, 4종 미니프리뷰, AccentGroupControls), 단일 디바이스 패널(SingleDevicePanel + AddPanelDialog 개선), 에이전트 리스트 개선(검색/상태 필터/페이지네이션/상태 아이콘) |
| 2026-03-19 | 1.19.0 | Module 26-29 추가: 에어컨 제어 패널(AcControlPanel, 전원/온도/모드/풍량/스윙 제어 UI), HVAC 공조기 제어 패널(HvacControlPanel, 센서 데이터/환기 모드/온습도 설정/스케줄), 게이지 차트 패널 시스템(GaugePanel 7종 SVG 게이지 + 게이지 설정 다이얼로그), 대시보드 헤더 테마 마이그레이션(하드코딩 slate→CSS Variable) |
| 2026-03-27 | 1.20.0 | Module 30 추가: 대시보드 그리드 시스템 확장(동적 그리드 컬럼 설정/그리드 라인 표시/ResizeObserver 반응형), PropertiesGridPanel 신규(디바이스 속성 그리드 레이아웃), LGAP/LGCP 에이전트 스키마 추가(agentSchemas.ts 14+12개 설정 필드), nodeSchemas.ts 확장(Bridge publish_topic/LGAP·LGCP 노드 스키마), 17종 패널 타입 확장(기존 5종→17종, panelDefaultSize 매핑), i18n 80+ 신규 번역 키 |
| 2026-03-27 | 1.21.0 | Module 31 추가: 디바이스 페이지 테이블 전환. 그리드 카드 레이아웃에서 AgentListPage 패턴의 테이블 리스트로 전환. DeviceSearchFilter 신규(검색/상태 뱃지/프로토콜·타입 필터), AddDeviceDialog 추출(NASA/Modbus 디바이스 추가 폼), SortableHeader 정렬, 확장 가능 행(DeviceDetailPanel), 페이지네이션 |
| 2026-03-28 | 1.22.0 | Module 32 추가: DeviceDetailPanel 좌우 분할 레이아웃(상태 좌측/제어+메타데이터 우측), MetadataSection 읽기 모드 키/값 테이블 전환(그리드 카드→table), Vite 프록시 타겟 포트 수정(8080→8081) |
| 2026-03-28 | 1.23.0 | Module 33 추가: NASA Agent 설정 UI 개선. 설정 2열 레이아웃(연결 좌측/운영 우측), 라벨 개선(전송→연결 방식, 폴링→상태 확인 요청 간격), 상태 변경 알람 전송 필드 추가, 체크박스 라벨 통일(활성/비활성→필드명 표시), 로그 레벨 운영 컬럼 통합, Makefile 추가 |
| 2026-03-29 | 1.24.0 | Module 34 추가: 디바이스 속성 한국어 라벨(COMMAND_LABELS/PARAM_LABELS/ENUM_LABELS + humanizeKey 폴백), MQTT 에이전트 토픽 탭(NO_DEVICES_TAB/HAS_TOPICS_TAB 제어, 구독/수신/발행 3분류 트리 구조, SubscriptionTree 접이식 UI + 모두 접기/펼치기, TopicStatsTable 발행 토픽, StatCard 요약, detail=full API), CLI agent topics 명령 연동 |
| 2026-03-30 | 1.25.0 | Module 35 추가: Store 에이전트 키/값 저장소 탭(StoreTab, StoreEntryRow, 키/값/네임스페이스/TTL/갱신시각 컬럼, 접이식 값 표시, 새로고침 버튼, detail=full API), store-write/store-read 노드 스키마(nodeSchemas.ts + nodeTypeMeta.ts 추가), AgentDetailPanel HAS_STORE_TAB 집합 추가, API detail 레벨 개선(summary에서도 StatefulAgent.State() 호출, entries 제외) |
| 2026-03-30 | 1.27.0 | Module 37 추가: 노드 enable/disable UI(PropertyPanel 활성화 토글 스위치), 비활성 노드 흐리게 표시(CustomNode opacity-45), 노드 카테고리 아이콘 수정(백엔드 6종 카테고리 processing/routing/io/error/debug/storage 아이콘 매핑, rf_category fallback으로 NodeRegistry.TypeMeta 조회, Engine.NodeRegistry() 공개 접근자 추가), flowToReactFlowConfig/flowToInfo를 FlowServiceAdapter 메서드로 전환 |
| 2026-03-30 | 1.26.0 | Module 36 추가: 실외기/제어기 모니터링 패널(OutdoorControlPanel, 압축기 주파수/용량 표시, 운전 모드 뱃지, 4종 상태 인디케이터 LED 그리드, 모니터링 전용 읽기 패널). Store 히스토리 버그 수정(API 응답 경로 수정 + 카운터/리스트 수 동기화). store-write/store-read 노드 UI 스키마 확장(nodeSchemas.ts + nodeTypeMeta.ts) |
| 2026-03-30 | 1.28.0 | Module 38 추가: 디바이스 제어 UI 통일. NASA 디바이스 그리드 패널 전환(NasaIndoorRemoteControl 제거→GenericPropertiesGrid), 제어 순서 통일(전원→운전 모드→온도→풍량→고정 설치, PROPERTY_ORDER/COMMAND_ORDER/LGAP 리모컨), 전원 슬라이드 스위치(OFF 상태 커맨드 버퍼링, ON 시 일괄 적용), Bool 컨트롤 슬라이드 스위치 통일, 스피너 thumb 오버레이(레이아웃 시프트 방지) |
| 2026-04-16 | 1.29.0 | **Module 39 추가: 필수 필드 검증 & 노드 경고 뱃지 시스템**. (1) `getRequiredFieldErrors(nodeType, data, agentType?)` 헬퍼 신규 (nodeSchemas.ts) — visibleWhen 을 고려한 필수 필드 검증, 빈 문자열/null/빈 배열/빈 객체를 "값 없음"으로 판정. (2) `PropertyPanel` 에 호박색 경고 배너 + Apply 버튼 비활성화 (`disabled={hasMissingRequired}`) — 필수 항목 누락 시 저장 자체를 차단. (3) `CustomNode` 캔버스 카드에 실시간 경고 뱃지 — 좌측 상단 AlertTriangle 아이콘, 호박색 테두리 강조, tooltip 으로 누락 필드 목록 표시. 잘못 설정된 노드가 배포되기 전에 시각적으로 인지 가능. **Module 40 추가: Trigger 노드 전용 스케줄 에디터**. `trigger` 노드 스키마 신규 등록, `TriggerScheduleEditor` 컴포넌트 (interval/cron/once/times 4종 전용 위젯 + 프리셋 칩 + 실시간 인라인 검증), `ConfigField.type='trigger_schedules'` 신규 타입, `ConfigField.advanced` 플래그 + `DynamicForm` 접을 수 있는 "고급 설정" 섹션 인프라 (source_ch_size 기본 접힘), `payload_mode` UI 전용 가상 필드 (PropertyPanel payload/payload_template 자동 토글 + 저장 시 비활성 키 정리). 관련 SPEC: SPEC-NODE-004 v1.1.0 |
| 2026-04-16 | 1.30.0 | **Module 41 수정: AcControlPanel mode 컨벤션 정렬 (회귀 수정)**. AcControlPanel의 `AcMode` 타입과 `MODE_CONFIG` 키를 `'cooling'/'heating'/'dehumidify'` 에서 백엔드 통일 컨벤션 `'cool'/'heat'/'dry'` 로 정렬. 이전엔 백엔드(LGCNP/LGCP/NASA/LGAP)가 보낸 `mode: "cool"` 을 프론트엔드가 매칭 실패하여 대시보드에서 운전 모드가 **아예 표시되지 않던 버그** 수정. `fan` / `auto` 는 기존과 동일. `deviceLabels.ENUM_LABELS` 는 양쪽 표기 모두 지원 중이라 한국어 표시에는 영향 없음. 관련 SPEC: SPEC-LG-HVACR-001 v1.2 REQ-M3-04a |
| 2026-05-14 | 1.31.0 | **Module 13 수정: 플로우 Import/Export round-trip 무결성 hotfix**. xagent04 실배포 검증에서 발견된 데이터 손실 결함 3종 수정. (1) **export `required_agents` 누락 수정** — `extractAgentNames` 입력이 React Flow 형식인데 XFlow 형식을 기대하던 불일치 해결 + 대소문자 무시 매칭 (커밋 `e5443c4`). (2) **import 노드-agent 자동 매핑** — import 시 `agent_id` 를 새 시스템 기준으로 재해결, export 에서 `agent_id` 제거, UI `agent_ref` 필드 채움 (커밋 `16d91ae`). (3) **CLI/import 이중 wrapping 데이터 손실** — CLI 가 export 파일(`{name, definition}`)을 다시 `definition` 으로 이중 래핑하던 버그 + export config 중첩 + NodeDef 관대 unmarshal + wires 변환 무조건 적용 (커밋 `4a85fa3`, hotfix Z+Y+W). 신규 요구사항 REQ-WEB-001-13-15~18 추가, REQ-WEB-001-13-13(CLI 호환) 보강. 관련 SPEC: SPEC-CLI-001, SPEC-FLOW-001 (`required_agents`/`agent_ref`). |

---

# SPEC-WEB-001: 웹 UI 에이전트 통계 표시 버그 수정 및 디버깅 레벨 설정

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 Web Dashboard에서 에이전트 관련 두 가지 이슈를 해결한다:

1. **에이전트 목록 통계 버그 수정**: 에이전트 목록 페이지에서 uptime, messages_in, messages_out 통계가 항상 빈 값(`-`)으로 표시되는 버그 (근본 원인: 백엔드 ListAgents가 `detail` 파라미터 무시)
2. **컴포넌트별 디버깅 레벨 설정**: 현재 글로벌 로그 레벨 설정만 지원하는데, 에이전트/노드 단위의 개별 로그 레벨 제어 기능 추가
3. **플로우 노드 통계 및 로그 레벨 표시**: 플로우 목록에서 노드별 In/Out 메시지 통계와 개별 로그 레벨 제어 기능 추가
4. **캔버스 런타임 통계 표시**: React Flow 에디터 캔버스에서 노드별 실시간 상태(running/stopped/error)와 In/Out 메시지 수 표시
5. **백엔드 포트 카운터 및 브릿지 버그 수정**: engine.go 포트 카운터 초기화 누락, bridge.go msgCh/Process 반환값 버그 수정
6. **로그 뷰어 컴포넌트/소스 필드 표시 및 필터링**: WebSocket `log.entry` 페이로드의 `component`/`source` 필드를 로그 뷰어에 표시하고, 소스 종류(agent/node/flow 등) 및 컴포넌트 이름으로 필터링 기능 추가
7. **동적 포트 시스템**: 플로우 노드의 입출력 포트가 설정한 갯수/방향과 일치하지 않는 버그 수정. 노드 타입과 설정(브릿지 방향, 스위치 라우트 등)에 따라 포트를 동적으로 계산하고, 설정 변경 시 포트를 재계산하는 시스템 구현
8. **에러 포트 타입 지원**: 프론트엔드가 백엔드의 `error` 포트 direction을 인식하지 못해 에러 포트가 렌더링되지 않는 버그 수정. `PortDef`/`NodeTypeDefinition` 타입에 `'error'` 추가, CustomNode에서 에러 포트를 빨간색으로 오른쪽(출력 포트 아래)에 렌더링, NodeHandle에 에러 포트 색상과 offset 위치 지원 추가
9. **Handle ID 접두사 제거**: Handle ID에서 불필요한 접두사(`in-`, `out-`, `err-`)를 제거하고 포트 이름을 그대로 Handle ID로 사용. 포트 direction이 이미 별도 필드로 구분되므로 접두사 불필요. 프론트엔드(CustomNode, NodeHandle, PropertyPanel)와 백엔드(flow_adapter.go)의 Handle↔Edge 매핑 간소화
10. **리스트 정렬 기능**: 플로우 목록과 에이전트 목록 페이지에서 각 컬럼 헤더를 클릭하여 정렬 가능. 이름(name)을 기본 정렬로 사용하며, 백엔드 `ListOptions.Sort` 파라미터를 실제 구현하여 서버 사이드 정렬 지원
11. **대시보드 패널 재구성**: 대시보드의 2x2 위젯 그리드(SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget, ResourceWidget)를 3패널 구조로 재구성. FlowPanel은 상단에 플로우 상태 요약(running/stopped/error/stored/loaded 건수), 하단에 플로우 리스트 테이블(이름, 상태, 노드 수, 동작 시간, 시작/정지 액션). AgentPanel은 동일한 구조로 에이전트 상태 요약 + 리스트 테이블. ResourcePanel은 CPU/메모리 사용률 게이지 + 미니 차트 유지
12. **Import/Export 기능**: 플로우와 에이전트를 JSON/YAML 파일로 내보내기(Export) 및 가져오기(Import). 내보내기 시 런타임 필드(id, status, stats, timestamps)를 제거하고 정의 데이터만 포함. 가져오기 시 클라이언트 사이드에서 FileReader API로 파일을 파싱하고, JSON/YAML 자동 감지 후 유효성 검사. ImportDialog 공용 모달에서 파일 선택, 드래그 앤 드롭, 미리보기, 이름 편집, 유효성 에러 표시. CLI(`xflowd flow import`/`xflowd agent import`) 호환 포맷 지원
13. **대시보드 커스터마이징 시스템**: 대시보드를 사용자가 자유롭게 구성할 수 있도록 멀티 대시보드 페이지 지원. 각 페이지는 독립적인 패널 구성(플로우/에이전트/리소스/디바이스/로그)을 가지며, 패널 추가/삭제가 가능. 기본 페이지 지정, 대시보드 전환 드롭다운, 대시보드 관리 툴바(추가/삭제/기본지정/이름편집) 제공. 기존 단일 대시보드 상태를 DashboardPageConfig 배열로 마이그레이션하여 Zustand persist 호환 유지
14. **UI 레이아웃 재구성**: 대시보드 관리 컨트롤을 DashboardToolbar에서 Header.tsx 라우트 조건부 렌더링으로 이전. 편집 모드 버튼(패널 추가/초기화, 디바이스 추가/초기화)을 별도 설정 바에서 조작 바의 편집 토글 앞으로 인라인 이동
15. **console-logger 로그 레벨 수정**: Process 메서드의 Debug→Info 레벨 변경으로 기본 설정에서 메시지 출력 보장
16. **플로우 Undeploy API + 대시보드 상태 아이콘**: 플로우 배포 해제 API 엔드포인트 추가, FlowActionMenu에서 상태 기반 액션 활성화/비활성화, 대시보드 패널의 상태 표시를 텍스트 뱃지에서 아이콘으로 변경
17. **패널 설정 다이얼로그**: 대시보드 패널별 상세 설정을 위한 PanelSettingsDialog 모달. 패널 제목 편집, 타입별 설정 섹션(ColumnsSection, FilterSection, LogsSection), 악센트 컬러 시스템 통합
18. **악센트 컬러 시스템**: 대시보드 패널 요소별 악센트 컬러 커스터마이징. 미니프리뷰에서 요소 클릭으로 그룹 선택, AccentGroupControls로 활성화/프리셋/커스텀 컬러 지정. 4종 미니프리뷰(NasaMiniPreview, ListMiniPreview, ResourceMiniPreview, LogMiniPreview), _base 그룹 통합, 패널 컴포넌트에 acColor 헬퍼 패턴 적용
19. **단일 디바이스 패널 + 패널 추가 개선**: 개별 디바이스를 대시보드에 패널로 추가하는 'device' 타입 패널(SingleDevicePanel). PanelType에 'device' 추가, addPanelWithConfig API, AddPanelDialog에서 패널 타입 선택 및 디바이스 선택 콤보박스
20. **에이전트 리스트 개선**: 에이전트 목록 페이지에 검색, 상태 필터 아이콘 뱃지 토글, 페이지네이션(10/20/50개), 상태 아이콘(Activity/CircleStop/AlertTriangle) 추가. FlowSearchFilter와 동일 패턴의 AgentSearchFilter 컴포넌트
21. **에어컨 제어 패널**: 대시보드에 에어컨(AC) 디바이스 전용 제어 패널 추가. 전원 ON/OFF, 현재 온도 대형 표시, 설정 온도 조절(16~30°C), 운전 모드 5종(냉방/난방/자동/제습/팬), 풍량 4단계(자동/약/중/강), 스윙 토글, 필터 상태 표시
22. **HVAC 공조기 제어 패널**: 대시보드에 공조기(HVAC) 디바이스 전용 제어 패널 추가. 4종 센서 데이터(온도/습도/CO2/전력), 환기 모드 선택(자동환기/외기냉방/전열교환/바이패스/배기), 온·습도 설정값 조절, 운전 스케줄 표시
23. **게이지 차트 패널 시스템**: 7종 SVG 게이지 차트(심플 도넛/반원/멀티링/니들/니들 레인보우/세로 바/반원 레인보우) 렌더링 + 게이지 설정 다이얼로그(유형 선택/값 범위/단위/데이터 소스 바인딩/임계값·컬러 설정)
24. **대시보드 헤더 테마 마이그레이션**: 대시보드 타이틀 바의 하드코딩된 Tailwind 색상 클래스(bg-white, text-slate-*, border-slate-*)를 CSS Variable 테마 토큰(--color-bg-surface, --color-text-primary 등)으로 교체
25. **대시보드 그리드 시스템 확장 + LGAP/LGCP 스키마 + PropertiesGrid 패널**: 동적 그리드 컬럼 설정(dashboardGridCols, 기본 10, 범위 4-100) + 그리드 라인 표시 토글(dashboardShowGridLines) + ResizeObserver 기반 반응형 셀 크기 계산 + PropertiesGridPanel 신규(디바이스 속성 그리드 레이아웃, visibleProperties 설정) + LGAP 에이전트 14개 설정 필드/LGCP 에이전트 12개 설정 필드 스키마 추가 + nodeSchemas.ts Bridge publish_topic/LGAP·LGCP 노드 스키마 + 17종 패널 타입 확장(panelDefaultSize 매핑) + i18n 80+ 신규 번역 키
26. **Store 에이전트 키/값 저장소 탭**: Store 타입 에이전트의 상세 패널에 저장소 전용 탭 추가. 키/값/네임스페이스/TTL/갱신시각 5개 컬럼 테이블, 접이식 값 표시(StoreEntryRow 클릭 토글), 새로고침 버튼(spin 애니메이션), detail='full' API 연동으로 전체 엔트리 로딩, detail='summary'에서도 total_keys 등 기본 상태 제공. store-write/store-read 노드 스키마(nodeSchemas.ts/nodeTypeMeta.ts) 추가. HAS_STORE_TAB 집합으로 Store 탭 표시 제어

### 1.2 기술 환경

- **프론트엔드**: TypeScript 5.x, React 19.x, Vite 6.x, Tailwind CSS 4.x
- **백엔드**: Go, xflowd 데몬
- **패키지 경로**: `web/src/` (프론트엔드), `internal/` (백엔드)
- **상태 관리**: Zustand 5.x (클라이언트 상태), @tanstack/react-query 5.x (서버 상태)
- **HTTP 클라이언트**: axios
- **UI 컴포넌트**: shadcn/ui + Radix UI
- **의존 SPEC**:
  - SPEC-API-001: REST API 엔드포인트
  - SPEC-OBS-001: 관찰성 시스템 (LevelManager 인터페이스)
  - SPEC-OBS-004: 로그 WebSocket 페이로드 확장 (component/source 필드)
  - SPEC-AGENT-001: Agent 관리

### 1.3 설계 원칙

- **최소 변경 원칙**: 기존 백엔드 LevelManager 인터페이스를 최대한 활용하여 새로운 추상화를 최소화한다
- **프론트엔드+백엔드 동시 수정**: Issue 1은 프론트엔드(`detail=summary` 추가)와 백엔드(`ListOptions.Detail` 파라미터 전달) 양쪽 수정이 필요하다
- **점진적 기능 추가**: 글로벌 로그 레벨 기능을 유지하면서 컴포넌트별 로그 레벨 기능을 추가한다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Module 1: 에이전트 목록 API `detail` 파라미터 전달 버그 수정 (프론트엔드 + 백엔드)
- Module 2: 컴포넌트별 로그 레벨 API 엔드포인트 추가 (백엔드)
- Module 3: 에이전트 컴포넌트별 로그 레벨 UI 추가 (프론트엔드)
- Module 4: 플로우 노드 통계 + 로그 레벨 UI 추가 (프론트엔드) — In/Out 분리 표시, 실시간 갱신
- Module 5: 캔버스 런타임 통계 표시 (프론트엔드) — RuntimeStatsContext, CustomNode In/Out 표시
- Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 + 소스 종류 필터 + 컴포넌트 이름 필터 (프론트엔드)
- Module 8: 동적 포트 시스템 — 노드 타입/설정 기반 포트 동적 계산 + 브릿지 방향별 포트 + 스위치 라우트별 출력 포트 + 설정 변경 시 포트 재계산 (프론트엔드 + 백엔드)
- Module 9: 에러 포트 타입 지원 — PortDef/NodeTypeDefinition에 `'error'` direction 추가 + CustomNode 에러 포트 빨간색 오른쪽 렌더링 + NodeHandle 에러 포트 색상/offset (프론트엔드)
- Module 10: Handle ID 접두사 제거 — 포트 이름을 Handle ID로 직접 사용 + flow_adapter.go TrimPrefix 제거 + PropertyPanel Port 타입 'error' 추가 (프론트엔드 + 백엔드)
- Module 11: 리스트 정렬 기능 — FlowListPage/AgentListPage 정렬 가능 컬럼 헤더 UI + 백엔드 sort 파라미터 처리 구현 (프론트엔드 + 백엔드)
- Module 12: 대시보드 패널 재구성 — 기존 2x2 위젯 그리드(SystemStatusWidget + AgentStatusWidget + RecentFlowsWidget + ResourceWidget)를 3패널 구조로 전환. FlowPanel(상태 요약 + 플로우 리스트 테이블), AgentPanel(상태 요약 + 에이전트 리스트 테이블), ResourcePanel(CPU/메모리 사용률 게이지) (프론트엔드)
- Module 13: Import/Export 기능 — 플로우/에이전트를 JSON/YAML 파일로 내보내기 + 가져오기. 백엔드 플로우 Export API 추가 + 프론트엔드 downloadJSON 유틸 + importParser 유틸 + ImportDialog 공용 모달 + FlowListPage/AgentListPage 툴바 버튼 + flowService/agentService Export 함수 (프론트엔드 + 백엔드)
- 백엔드 버그 수정: engine.go 포트 카운터 초기화, bridge.go msgCh/Process 반환값
- Module 15: 화면 테마 시스템 — CSS Variable 디자인 토큰 시스템 + Day/Night 테마 프리셋 + Custom 테마 지원(사용자 정의 색상) + 테마 선택 UI(System/Day/Night/Custom 4옵션) + 커스텀 테마 에디터(컬러 피커, 라이브 프리뷰) + `dark:` Tailwind 클래스 CSS Variable 마이그레이션 (프론트엔드)
- Module 16: 대시보드 커스터마이징 시스템 — 멀티 대시보드 페이지(DashboardPageConfig[] 데이터 모델) + 대시보드 관리 UI(Header.tsx 라우트 조건부 렌더링: 드롭다운/추가/삭제/기본지정/이름편집) + 패널 추가/삭제 시스템(AddPanelDialog, 5종 패널 타입) + 디바이스·로그 패널 타입(DevicePanel, LogPanel) + Zustand persist 마이그레이션 (프론트엔드)
- Module 17: UI 레이아웃 재구성 — 대시보드 관리 컨트롤을 DashboardToolbar에서 Header.tsx로 이전(라우트별 조건부 렌더링) + DashboardToolbar.tsx 삭제 + 패널 추가/초기화 버튼을 별도 설정 바에서 조작 바 편집 토글 앞으로 인라인 이동 + 디바이스 페이지 동일 패턴 적용(추가/초기화 버튼 편집 모드 전용 인라인 배치) + console-logger Process 로그 레벨 버그 수정(Debug→Info) (프론트엔드 + 백엔드)
- Module 19: 플로우 Undeploy API + 대시보드 상태 아이콘 — 플로우 배포 해제 API(`POST /flows/{id}/undeploy`) + FlowActionMenu 상태별 액션 제어(canStart/canStop/canRestart/canDeploy/canUndeploy/canDelete), Deploy(Cable)/Undeploy(Unplug) 아이콘, 대시보드 FlowPanel/AgentPanel/DevicePanel 상태 아이콘 표시 (프론트엔드 + 백엔드)
- Module 20: 에이전트 연결 상태 정확성 개선 — TransportChecker 인터페이스 도입, 에이전트 connected 필드가 lifecycle 상태 대신 실제 트랜스포트 연결 여부 반영, 미연결 시 uptime 숨김, 프론트엔드 AgentPanel/AgentActionButtons/AgentStatusBadge connected 기반 통일 (프론트엔드 + 백엔드)
- Module 21: 플로우 리스트 상태 아이콘 + 상태 필터링 — FlowListPage 상태 표시를 대시보드 FlowPanel과 동일한 아이콘 기반 렌더링으로 교체(Activity/CircleStop/AlertTriangle/FileText/Rocket), FlowSearchFilter 상태 필터를 select 드롭다운에서 아이콘+라벨 뱃지 토글 버튼으로 변경 (프론트엔드)
- Module 22: 패널 설정 다이얼로그 — PanelSettingsDialog 모달 컴포넌트(1217줄 신규). 패널 제목 편집 + 타입별 설정 섹션(ColumnsSection: flows/agents 패널 컬럼 가시성 토글, FilterSection: devices 패널 에이전트/프로토콜별 필터, LogsSection: logs 패널 최대 줄 수/레벨 필터/소스 필터) + DashboardPage/uiStore 통합 (프론트엔드)
- Module 23: 악센트 컬러 시스템 — 패널 요소별 악센트 컬러 커스터마이징. panelColor(_base 그룹), accentElements(Record<string, string|boolean>), AccentGroupControls(활성화 토글+프리셋+커스텀 피커+리셋), 4종 미니프리뷰(NasaMiniPreview/ListMiniPreview/ResourceMiniPreview/LogMiniPreview), _base 그룹 통합(conic-gradient), 패널 컴포넌트 acColor 헬퍼 적용(FlowPanel/AgentPanel/DevicePanel/LogPanel/ResourceWidget), NASA 리모컨 서브프로퍼티(temperature value/label) (프론트엔드)
- Module 24: 단일 디바이스 패널 + 패널 추가 개선 — SingleDevicePanel(118줄 신규, 단일 디바이스 NASA 리모컨 UI) + PanelType에 'device' 추가(기존 'devices' 리스트와 별도) + addPanelWithConfig API(uiStore에 config/title 지정 패널 추가) + AddPanelDialog 개선(패널 타입 선택 UI, device 타입 시 에이전트/디바이스 선택 콤보박스) (프론트엔드)
- Module 25: 에이전트 리스트 개선 — AgentSearchFilter(73줄 신규, 검색+상태 필터 아이콘 뱃지 토글) + 상태 아이콘(텍스트 뱃지→Activity/CircleStop/AlertTriangle 아이콘) + 페이지네이션(10/20/50개 단위, 이전/다음 네비게이션) + 클라이언트 사이드 검색/필터(이름/타입 검색, 상태별 필터링, useMemo) (프론트엔드)
- Module 26: 에어컨 제어 패널 — AcControlPanel(257줄 신규). PanelType에 'ac-control' 추가 + 전원 ON/OFF 토글(상태 뱃지 연동) + 현재 온도 대형 표시(5xl 폰트) + 설정 온도 조절(16~30°C, ±1°C 버튼) + 운전 모드 5종 버튼 그리드(냉방/난방/자동/제습/팬, 아이콘+라벨) + 풍량 4단계 선택(자동/약/중/강, ring 강조) + 스윙 토글 + 필터 교체 경고. useDeviceRealtime 훅 연동, deviceId config 기반 디바이스 바인딩 (프론트엔드)
- Module 27: HVAC 공조기 제어 패널 — HvacControlPanel(256줄 신규). PanelType에 'hvac-control' 추가 + 4종 센서 카드(온도/습도/CO2/전력, 아이콘+값+단위) + 환기 모드 5종 선택(자동환기/외기냉방/전열교환/바이패스/배기) + 온도 설정값 조절(16~30°C) + 습도 설정값 조절(30~70%) + 운전 스케줄 표시(시작/종료 시간) + 전원 토글 + 설정 버튼. useDeviceRealtime 훅 연동, deviceId config 기반 디바이스 바인딩 (프론트엔드)
- Module 28: 게이지 차트 패널 시스템 — GaugePanel(514줄 신규) + PanelSettingsDialog 게이지 섹션(~550줄 추가). PanelType에 'gauge' 추가. **GaugePanel**: 7종 SVG 게이지 렌더러(SimpleGauge 360° 도넛, HalfGauge 반원, MultiRingGauge 동심원, NeedleGauge 원형 니들, NeedleRainbowGauge 레인보우 니들, VerticalBarGauge 세로 바, HalfRainbowGauge 5단계 등급). 헬퍼(polarToCartesian, describeArc, describeDonutArc), ThresholdEntry 기반 구간 컬러링, multi-ring values[] 지원. **게이지 설정 다이얼로그**: GaugeSection(유형 4열 그리드+SVG 미니아이콘, 값 범위 min/max, 단위 입력, 데이터 소스 바인딩(리소스/플로우, multi-ring 다중 행), 임계값·컬러(개별 지정 threshold rows+컬러피커 / 연속 컬러 3종 테마 프리셋 토글)) + GaugeMiniPreview(7종 SVG 프리뷰) + GaugeTypeIcon(미니 아이콘) (프론트엔드)
- Module 29: 대시보드 헤더 테마 마이그레이션 — DashboardPage.tsx header 섹션(약 290줄)의 하드코딩된 Tailwind 색상 클래스를 CSS Variable 테마 토큰으로 교체. bg-white→bg-(--color-bg-surface), text-slate-900→text-(--color-text-primary), border-slate-200→border-(--color-border-default), bg-slate-100→bg-(--color-bg-elevated) 등. dark: 접두사 제거(CSS 변수가 자동 처리). 드롭다운(대시보드 셀렉터/테마/그리드 설정) + 편집 모드 버튼(테마/그리드/패널 추가/취소/저장) + 일반 모드 버튼(WS 상태/새로고침/편집) 모두 포함 (프론트엔드)
- Module 30: 대시보드 그리드 시스템 확장 + LGAP/LGCP 스키마 + PropertiesGrid 패널 — **대시보드 그리드 확장**: dashboardGridCols(기본 10, 범위 4-100) 동적 그리드 컬럼 설정, dashboardShowGridLines 그리드 라인 표시 토글, ResizeObserver 기반 반응형 셀 크기 계산, 그리드 설정 드롭다운 UI. **PropertiesGridPanel 신규**: 디바이스 속성을 그리드 레이아웃으로 표시, visibleProperties 설정으로 표시 컬럼 제어, 패널별 디바이스 바인딩. **LGAP/LGCP 에이전트 스키마**: agentSchemas.ts에 LGAP 에이전트 14개 설정 필드(시리얼 포트/보드레이트/폴링/디바이스 목록/타임아웃/백오프), LGCP 에이전트 12개 설정 필드(시리얼 설정/CRC 검증/자동 디스커버리/상태 리포팅/백오프), 에이전트 타입 라벨 업데이트('lgap'→LG LGAP, 'lgcp'→LG LGCP Capture). **nodeSchemas.ts 확장**: Bridge 에이전트 기본값에 showPublishTopic 플래그 추가, logger/mqtt 브릿지에 publish_topic 필드 추가, LGAP/LGCP 노드 스키마 정의. **17종 패널 타입 확장**(기존 5종에서): 데이터(device/table/properties-grid), 차트(stat/gauge/line-chart/bar-chart/pie-chart), 콘텐츠(text), 제어(ac-control/hvac-control/custom-control), panelDefaultSize 매핑 추가. **i18n 확장**: 80+ 신규 번역 키(패널 카테고리/패널 타입 라벨/AC·HVAC 제어 라벨/테마 라벨) (프론트엔드)

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- 로그 레벨 영속화 (서버 재시작 시 초기화됨)
- 로그 레벨 변경 이력 감사(audit) 로그
- 실시간 WebSocket 기반 로그 레벨 동기화

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| LevelManager | Go 백엔드의 `observe.LevelManager` 인터페이스. 컴포넌트별 slog.Level을 관리한다 |
| Component | 로그 레벨을 개별 설정할 수 있는 단위. `agent.{name}`, `node.{name}` 형식 |
| Pattern | 와일드카드(`*`)를 포함하는 컴포넌트 매칭 문자열. 예: `agent.*` |
| detail parameter | 에이전트 목록 API의 쿼리 파라미터. `summary`일 때 stats, uptime, health 포함 |
| Global Log Level | 시스템 전체에 적용되는 기본 로그 레벨 |
| Component Log Level | 특정 에이전트 또는 노드에만 적용되는 개별 로그 레벨 |
| Source | `classifySource()`가 분류한 로그 출처 카테고리. agent, node, flow, api, engine, system 중 하나 |
| Source Filter | 로그 뷰어에서 source 카테고리별 토글 필터 (다중 선택 가능) |
| Component Filter | 로그 뷰어에서 component 이름 텍스트 검색 필터 (포함 매칭) |
| PortDef | 프론트엔드 포트 정의 인터페이스. `{ name: string, direction: 'input' \| 'output' }` |
| computePortsForNode | 노드 타입과 설정(config)을 기반으로 포트를 동적 계산하는 함수 |
| BridgeDirection | 브릿지 노드의 데이터 방향. `BridgeIn`(agent->flow), `BridgeOut`(flow->agent), `BridgeInOut`, `BridgeRequestReply` |
| Switch Routes | 스위치 노드의 라우팅 경로 목록. 각 라우트가 별도 출력 포트로 매핑됨 |
| getDefaultPorts | 노드 타입의 정적 기본 포트를 반환하는 기존 함수. Module 8에서 `computePortsForNode`로 대체/보완 |
| Error Port | 백엔드 `PortError` direction을 가진 포트. 노드의 에러 출력 경로를 나타낸다. 프론트엔드에서 빨간색으로 노드 하단에 렌더링됨 |
| PortDirection (확장) | 프론트엔드 포트 방향. Module 9에서 기존 `'input' \| 'output'`에 `'error'`가 추가되어 `'input' \| 'output' \| 'error'`로 확장됨 |
| Handle ID | React Flow Handle 컴포넌트의 `id` prop. Module 10에서 포트 이름(`name`)을 그대로 사용하도록 변경. 이전에는 방향 접두사(`in-`, `out-`, `err-`)를 포함했음 |
| Handle↔Edge 매핑 | React Flow Edge의 `sourceHandle`/`targetHandle`과 Handle `id`의 대응 관계. 접두사 제거로 포트 이름 = Handle ID = Edge Handle 값으로 단순화됨 |
| Sort Parameter | 목록 API의 `sort` 쿼리 파라미터. `field:direction` 형식 (예: `name:asc`, `created_at:desc`) |
| Sortable Header | 클릭하여 정렬 방향을 변경할 수 있는 테이블 컬럼 헤더. 정렬 방향 인디케이터(▲/▼)를 포함 |
| FlowPanel | 대시보드의 플로우 통합 패널. 상단에 상태별 건수 요약, 하단에 플로우 리스트 테이블을 포함 |
| AgentPanel | 대시보드의 에이전트 통합 패널. 상단에 상태별 건수 요약, 하단에 에이전트 리스트 테이블을 포함 |
| ResourcePanel | 대시보드의 시스템 리소스 패널. CPU 사용률, 메모리 사용률을 게이지와 미니 차트로 표시 |
| Status Summary | 패널 상단의 상태별 건수 요약 영역. 배지 또는 카운터 형태로 각 상태(running/stopped/error 등)의 항목 수를 표시 |
| Export | 플로우 또는 에이전트의 정의 데이터를 JSON 파일로 다운로드하는 기능. 런타임 필드(id, status, stats, timestamps)를 제거하고 정의 데이터만 포함 |
| Import | JSON 또는 YAML 파일에서 플로우/에이전트 정의를 읽어 시스템에 새로 생성하는 기능. 클라이언트 사이드 파일 파싱 후 기존 Create API를 호출 |
| ImportDialog | Import 기능의 공용 모달 컴포넌트. 파일 선택(file picker), 드래그 앤 드롭, 미리보기, 이름 편집, 유효성 에러 표시, 로딩 상태를 제공 |
| Export Format (Flow) | 플로우 내보내기 파일 형식. `{ name, description?, definition }` 구조. definition에 nodes와 wires 포함 |
| Export Format (Agent) | 에이전트 내보내기 파일 형식. `{ name, type, config? }` 구조 |
| Runtime Fields | 내보내기 시 제거되는 런타임 전용 필드. id, status, stats, created_at, updated_at, uptime, health 등 |
| CLI 호환 포맷 | CLI의 `xflowd flow import`/`xflowd agent import` 명령과 동일한 파일 형식. 웹에서 내보낸 파일을 CLI에서 가져오기 가능하고, CLI에서 내보낸 파일을 웹에서 가져오기 가능 |
| js-yaml | YAML 파싱을 위한 JavaScript 라이브러리. Import 시 `.yaml`/`.yml` 확장자 파일을 파싱하는 데 사용 |
| Design Token | UI 디자인의 기본 단위로, CSS Custom Property(변수)로 구현되는 색상/간격/타이포그래피 값. 시맨틱 토큰은 용도를 설명하는 이름(예: `--color-bg-primary`)을 사용하여 테마 전환 시 값만 교체됨 |
| CSS Custom Property | `--property-name` 형식의 CSS 변수. `var(--property-name)` 구문으로 참조하며, `:root` 또는 `[data-theme]` 셀렉터에서 테마별 값을 정의함 |
| @theme 블록 | Tailwind CSS v4의 커스텀 CSS 변수 정의 블록. `index.css`의 `@theme { }` 내부에 변수를 선언하면 Tailwind 유틸리티 클래스에서 참조 가능 |
| Day 테마 | 라이트 모드 프리셋. 밝은 배경(gray-50)과 어두운 텍스트(gray-900)를 기본으로 하는 색상 체계 |
| Night 테마 | 다크 모드 프리셋. 어두운 배경(gray-900)과 밝은 텍스트(gray-100)를 기본으로 하는 색상 체계 |
| System 테마 | 운영체제의 `prefers-color-scheme` 미디어 쿼리를 감지하여 Day 또는 Night 테마를 자동 적용하는 모드 |
| Custom 테마 | 사용자가 직접 시맨틱 토큰 값을 지정하여 만든 테마. Zustand 스토어와 localStorage에 저장됨 |
| data-theme 속성 | `<html>` 요소에 부여되는 `data-theme="day\|night\|custom"` HTML 속성. CSS 셀렉터 `[data-theme="night"]`로 테마별 토큰 값을 전환함 |
| resolvedTheme | `useTheme` 훅이 반환하는 실제 적용 테마. System 모드일 때 OS 설정을 해석한 결과(day 또는 night)를 반환함 |
| 시맨틱 컬러 토큰 | UI 용도를 기반으로 이름 붙인 색상 변수. `--color-bg-primary`(주 배경), `--color-text-primary`(주 텍스트), `--color-border-default`(기본 테두리) 등 |
| DashboardPageConfig | 대시보드 페이지 1개를 정의하는 인터페이스. `{ id, name, isDefault, panels, layout }` 구조. 각 페이지는 독립적인 패널 구성과 react-grid-layout 레이아웃을 가짐 |
| PanelConfig | 대시보드 패널 1개를 정의하는 인터페이스. `{ id, type, title, config }` 구조. type은 PanelType 유니언으로 5종 패널을 구분 |
| PanelType | 대시보드 패널 종류를 나타내는 유니언 타입. `'flows' \| 'agents' \| 'resource' \| 'devices' \| 'logs'` |
| DashboardToolbar | 대시보드 상단 툴바 컴포넌트. 대시보드 선택 드롭다운, 추가/삭제/기본지정 버튼, 이름 인라인 편집 기능을 제공 |
| DevicePanel | 디바이스 상태를 표시하는 대시보드 패널. 상단에 상태 요약 카드(전체/온라인/오프라인 수), 하단에 디바이스 리스트 테이블을 포함 |
| LogPanel | 실시간 로그 스트림을 표시하는 대시보드 패널. WebSocket 연결로 로그를 수신하며, 소스 필터와 레벨 필터를 제공 |
| AddPanelDialog | 패널 추가 다이얼로그 컴포넌트. 5종 패널 타입을 아이콘과 설명과 함께 선택 가능 |
| activeDashboardId | uiStore에서 현재 활성화된 대시보드 페이지의 ID를 추적하는 상태 |
| dashboardPages | uiStore에서 모든 대시보드 페이지 설정을 저장하는 `DashboardPageConfig[]` 배열 상태 |
| AcControlPanel | 에어컨 디바이스 전용 대시보드 제어 패널. 전원, 온도, 모드, 풍량, 스윙 제어 UI를 제공하며 useDeviceRealtime 훅으로 실시간 데이터 표시. mode/fan_speed 값은 **백엔드 통일 컨벤션** (mode: cool/heat/dry/fan/auto, fan_speed: auto/quiet/low/medium/high/turbo) 을 그대로 사용 — cooling/heating/dehumidify 같은 별도 표기 금지 (v1.30.0 회귀 수정) |
| HvacControlPanel | HVAC 공조기 디바이스 전용 대시보드 제어 패널. 센서 데이터(온도/습도/CO2/전력), 환기 모드, 온습도 설정, 스케줄 표시 |
| GaugePanel | 7종 SVG 게이지 차트를 렌더링하는 대시보드 패널. GaugeType 유니언으로 렌더러를 선택하며, ThresholdEntry로 구간별 색상을 지정 |
| GaugeType | 게이지 차트 유형 유니언 타입. `'simple' \| 'half' \| 'multi-ring' \| 'needle' \| 'needle-rainbow' \| 'vertical-bar' \| 'half-rainbow'` |
| ThresholdEntry | 게이지 임계값 정의 인터페이스. `{ name: string, color: string, from: number, to: number }`. 값이 from~to 범위에 있으면 해당 color로 표시 |
| DataSourceBinding | 게이지 데이터 소스 바인딩 인터페이스. `{ sourceType: 'resource' \| 'flow', resource?: string, flowId?: string, dataField?: string }`. 리소스(CPU/메모리) 또는 플로우 데이터 필드와 연결 |
| GaugeSection | PanelSettingsDialog 내 게이지 전용 설정 섹션. 유형 선택, 값 범위, 단위, 데이터 소스 바인딩, 임계값·컬러 설정을 포함 |
| GaugeMiniPreview | PanelSettingsDialog 우측 컬럼의 게이지 SVG 프리뷰 컴포넌트. 선택된 게이지 유형에 따라 간략한 SVG 형태를 렌더링 |
| dashboardGridCols | uiStore의 대시보드 그리드 컬럼 수 설정 상태. 기본값 10, 범위 4-100. react-grid-layout의 cols 속성과 연동 |
| dashboardShowGridLines | uiStore의 그리드 라인 표시 여부 토글 상태. 활성화 시 대시보드에 그리드 가이드 라인을 오버레이로 표시 |
| PropertiesGridPanel | 디바이스 속성을 그리드 레이아웃으로 표시하는 대시보드 패널. visibleProperties 설정으로 표시할 컬럼을 제어하며 패널별 디바이스 바인딩을 지원 |
| panelDefaultSize | 패널 타입별 기본 그리드 크기(w, h) 매핑. 패널 추가 시 타입에 맞는 적절한 기본 크기를 자동 할당 |
| LGAP 에이전트 스키마 | agentSchemas.ts에 정의된 LG LGAP 에이전트의 14개 설정 필드. 시리얼 포트, 보드레이트, 폴링 간격, 디바이스 목록, 타임아웃, 백오프 등을 포함 |
| LGCP 에이전트 스키마 | agentSchemas.ts에 정의된 LG LGCP Capture 에이전트의 12개 설정 필드. 시리얼 설정, CRC 검증, 자동 디스커버리, 상태 리포팅, 백오프 등을 포함 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: 백엔드 `GET /agents` API에서 `detail` 쿼리 파라미터를 `ListOptions`에 전달하고, `agentToHandlerInfo`에서 detail 값에 따라 stats, uptime, health 필드를 포함한다
- A-002: `observe.LevelManager` 인터페이스의 `SetLevel`, `GetLevel`, `SetLevelByPattern`, `ListLevels` 메서드가 정상 동작한다
- A-003: `internal/api/handler/monitor.go`의 기존 `PUT /monitor/loglevel` 핸들러가 글로벌 로그 레벨 변경 기능을 올바르게 수행한다
- A-004: 프론트엔드 `AgentInfo` 타입에 이미 `stats`, `uptime`, `health` optional 필드가 정의되어 있다
- A-007: SPEC-OBS-004 구현이 완료되어 백엔드 WebSocket `log.entry` 페이로드에 `component`와 `source` 필드가 포함되어 전송된다
- A-008: `classifySource()` 함수가 반환하는 source 카테고리는 `agent`, `node`, `flow`, `api`, `engine`, `system` 6가지이다

- A-009: `web/src/config/nodeSchemas.ts`의 `NODE_SCHEMAS` 레지스트리에 각 노드 타입의 기본 포트 정의(`defaultPorts`)가 존재하며, `getDefaultPorts(nodeType)` 함수가 이를 반환한다
- A-010: 브릿지 노드의 direction 설정은 노드 config 객체의 `direction` 필드에 저장되며, 값은 `in`, `out`, `inout`, `request_reply` 중 하나이다
- A-011: 스위치 노드의 라우트 설정은 노드 config 객체의 `routes` 배열에 저장되며, 각 라우트에 `name` 필드가 존재한다
- A-012: 백엔드 `pkg/flow/node.go`의 `NewNodeDef()` 팩토리가 항상 기본 `[in, out]` 포트를 생성하며, 노드 타입별 포트 커스터마이징 로직이 없다
- A-013: 백엔드 에이전트 Export API(`GET /agents/{id}/export`, `GET /agents/export`)가 이미 구현되어 있으며, `{ name, type, config? }` 형식으로 응답한다
- A-014: CLI의 `pkg/flow/serialize.go`가 생성하는 내보내기 파일 형식이 `{ name, description?, definition }` 구조이며, 웹 Export API도 동일한 형식을 사용한다
- A-015: 브라우저 환경에서 Blob 및 URL.createObjectURL API가 지원되며, 파일 다운로드에 사용할 수 있다
- A-016: 기존 `POST /flows` 및 `POST /agents` Create API가 내보내기 형식의 데이터를 수신하여 새 리소스를 생성할 수 있다

### 3.2 운영 가정

- A-005: 컴포넌트별 로그 레벨은 서버 메모리에만 저장되며, 서버 재시작 시 기본 레벨로 초기화된다
- A-006: 동시에 활성화되는 컴포넌트별 로그 레벨 오버라이드 수는 최대 100개 이내이다

### 3.3 테마 시스템 가정

- A-017: Tailwind CSS v4를 사용하며 `@tailwindcss/vite` 플러그인이 설정되어 있다. `tailwind.config.ts` 및 `postcss.config.js`는 존재하지 않는다
- A-018: `index.css`의 `@theme` 블록에 CSS Custom Property를 선언하면 Tailwind 유틸리티 클래스에서 `var(--token-name)` 구문으로 참조 가능하다
- A-019: 현재 테마 시스템은 `uiStore.ts`에서 `theme: 'light' | 'dark' | 'system'` 3가지 모드를 지원하며, `useTheme` 훅이 `<html>` 요소에 `dark` 클래스를 토글한다
- A-020: 프로젝트 전체에 약 885개의 `dark:` Tailwind variant 클래스가 존재하며, 이를 CSS Variable 기반 시맨틱 토큰으로 점진적 마이그레이션한다
- A-021: 브라우저는 CSS Custom Property(`var()` 구문), `prefers-color-scheme` 미디어 쿼리, `data-*` HTML 속성을 지원한다
- A-022: lucide-react 아이콘 라이브러리가 설치되어 있으며, 테마 관련 아이콘(Sun, Moon, Monitor, Palette)을 사용할 수 있다
- A-023: 커스텀 테마 데이터는 Zustand `persist` 미들웨어를 통해 localStorage에 저장되며, 서버 동기화는 본 SPEC 범위에 포함되지 않는다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: 에이전트 목록 통계 버그 수정 (P0 - 버그 수정)

#### REQ-WEB-001-01-01 (Event-Driven)
**WHEN** 에이전트 목록 페이지가 로드될 때, **THEN** `GET /agents` API 호출 시 `detail=summary` 쿼리 파라미터를 포함하여 stats, uptime, health 데이터를 함께 요청해야 한다.

#### REQ-WEB-001-01-02 (State-Driven)
**IF** 에이전트 목록 API 응답에 `stats` 필드가 포함된 상태 **THEN** 에이전트 테이블의 "업타임" 컬럼에 `uptime` 값을, "메시지 (IN/OUT)" 컬럼에 `stats.messages_in / stats.messages_out` 값을 표시해야 한다.

#### REQ-WEB-001-01-03 (State-Driven)
**IF** 에이전트의 `stats` 필드가 없거나 null인 상태 **THEN** 해당 통계 컬럼에 `-` 기본값을 표시해야 한다.

### 4.2 Module 2: 백엔드 컴포넌트별 로그 레벨 API (P1 - 신규 기능)

#### REQ-WEB-001-02-01 (Event-Driven)
**WHEN** `PUT /monitor/loglevel/{component}` 요청을 수신하면, **THEN** `LevelManager.SetLevel(component, level)`을 호출하여 해당 컴포넌트의 로그 레벨을 설정하고, 성공 응답을 반환해야 한다.

#### REQ-WEB-001-02-02 (Event-Driven)
**WHEN** `GET /monitor/loglevel` 요청을 수신하면, **THEN** `LevelManager.ListLevels()`를 호출하여 모든 컴포넌트의 로그 레벨 맵(`component -> level`)과 현재 기본 레벨을 반환해야 한다.

#### REQ-WEB-001-02-03 (Event-Driven)
**WHEN** `DELETE /monitor/loglevel/{component}` 요청을 수신하면, **THEN** 해당 컴포넌트의 개별 로그 레벨 오버라이드를 제거하고 기본 레벨로 리셋해야 한다.

#### REQ-WEB-001-02-04 (Unwanted)
시스템은 유효하지 않은 로그 레벨 값(debug, info, warn, error 외)에 대해 설정을 **허용하지 않아야 한다**. 400 Bad Request 응답을 반환해야 한다.

#### REQ-WEB-001-02-05 (Ubiquitous)
시스템은 **항상** 기존 `PUT /monitor/loglevel` 글로벌 로그 레벨 변경 API의 하위 호환성을 유지해야 한다.

### 4.3 Module 3: 프론트엔드 컴포넌트별 로그 레벨 UI (P1 - 신규 기능)

#### REQ-WEB-001-03-01 (Event-Driven)
**WHEN** 에이전트 상세 패널이 열리면, **THEN** 해당 에이전트의 현재 로그 레벨을 표시하는 드롭다운 컨트롤을 통계 탭 또는 별도 영역에 제공해야 한다.

#### REQ-WEB-001-03-02 (Event-Driven)
**WHEN** 에이전트 로그 레벨 드롭다운 값을 변경하면, **THEN** `PUT /monitor/loglevel/agent.{agent-id}` API를 호출하여 해당 에이전트의 로그 레벨을 변경하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-03-03 (Event-Driven)
**WHEN** 에이전트 로그 레벨을 "기본값" 옵션으로 변경하면, **THEN** `DELETE /monitor/loglevel/agent.{agent-id}` API를 호출하여 개별 오버라이드를 제거해야 한다.

#### REQ-WEB-001-03-04 (Event-Driven)
**WHEN** 설정 페이지의 시스템 탭이 로드되면, **THEN** `GET /monitor/loglevel` API를 호출하여 글로벌 기본 레벨과 컴포넌트별 오버라이드 목록을 표시해야 한다.

#### REQ-WEB-001-03-05 (Event-Driven)
**WHEN** 설정 페이지에서 컴포넌트별 오버라이드의 "리셋" 버튼을 클릭하면, **THEN** `DELETE /monitor/loglevel/{component}` API를 호출하여 해당 오버라이드를 제거하고 목록을 갱신해야 한다.

#### REQ-WEB-001-03-06 (State-Driven)
**IF** 사용자 역할이 Viewer인 경우 **THEN** 로그 레벨 변경 드롭다운과 리셋 버튼을 비활성화(disabled)해야 한다.

### 4.4 Module 4: 플로우 노드 통계 및 로그 레벨 UI (P1 - 신규 기능)

#### REQ-WEB-001-04-01 (Event-Driven)
**WHEN** 플로우 목록 테이블의 행을 클릭하면, **THEN** 해당 플로우의 노드 인스턴스 목록을 `GET /flows/{id}/nodes` API로 조회하여 확장 패널에 표시해야 한다.

#### REQ-WEB-001-04-02 (State-Driven)
**IF** 플로우 확장 패널이 열린 상태 **THEN** 각 노드의 이름, 타입, 상태(running/stopped/error 배지), 포트별 In/Out 메시지 합계(입력/출력 아이콘 분리)를 테이블 형태로 표시해야 한다.

#### REQ-WEB-001-04-03 (Event-Driven)
**WHEN** 플로우 확장 패널의 노드별 로그 레벨 드롭다운을 변경하면, **THEN** `PUT /monitor/loglevel/node.{node-name}` API를 호출하여 해당 노드의 로그 레벨을 변경하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-04-04 (Event-Driven)
**WHEN** 노드 로그 레벨을 "기본값" 옵션으로 변경하면, **THEN** `DELETE /monitor/loglevel/node.{node-name}` API를 호출하여 개별 오버라이드를 제거해야 한다.

#### REQ-WEB-001-04-05 (State-Driven)
**IF** 플로우가 배포되지 않아 노드 정보가 없는 상태 **THEN** "노드 정보가 없습니다. 플로우를 배포하면 노드가 표시됩니다." 안내 메시지를 표시해야 한다.

#### REQ-WEB-001-04-06 (State-Driven)
**IF** 플로우가 running 상태 **THEN** 노드 인스턴스 데이터를 3초 간격으로 자동 갱신하여 실시간 통계를 반영해야 한다. 플로우가 running이 아닌 상태에서는 자동 갱신을 비활성화해야 한다.

### 4.5 Module 5: 캔버스 런타임 통계 표시 (P1 - 신규 기능)

#### REQ-WEB-001-05-01 (State-Driven)
**IF** 에디터 페이지에서 플로우가 running 상태 **THEN** React Flow 캔버스의 각 CustomNode에 실시간 상태 표시등(running=초록, stopped=회색, error=빨강)과 In/Out 메시지 수를 표시해야 한다.

#### REQ-WEB-001-05-02 (State-Driven)
**IF** 플로우가 running 상태 **THEN** 에디터 페이지는 3초 간격으로 `GET /flows/{id}/nodes` API를 폴링하여 런타임 통계를 갱신해야 한다.

#### REQ-WEB-001-05-03 (Unwanted)
런타임 통계 데이터는 에디터 스토어(editorStore)의 노드 데이터에 직접 저장**하지 않아야 한다**. isDirty 플래그 및 undo/redo 이력에 영향을 주지 않도록 별도의 React Context(RuntimeStatsContext)를 통해 전달해야 한다.

#### REQ-WEB-001-05-04 (State-Driven)
**IF** 플로우가 running이 아닌 상태 **THEN** CustomNode의 런타임 통계 표시를 숨기고 폴링을 비활성화해야 한다.

### 4.7 Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링 (P1 - 신규 기능)

#### REQ-WEB-001-07-01 (Ubiquitous)
시스템은 **항상** WebSocket `log.entry` 페이로드의 `component`와 `source` 필드를 LogEntry 데이터에 포함하여 저장해야 한다. `component`와 `source` 필드가 없는 로그 항목은 각각 빈 문자열과 `system`을 기본값으로 사용한다.

#### REQ-WEB-001-07-02 (Event-Driven)
**WHEN** WebSocket `log.entry` 메시지를 수신하면, **THEN** `component`와 `source` 필드를 페이로드에서 추출하여 LogEntry 객체에 포함해야 한다.

#### REQ-WEB-001-07-03 (State-Driven)
**IF** 로그 항목에 `component`와 `source` 필드가 있는 상태 **THEN** 로그 뷰어의 각 행에서 레벨 배지 옆에 source 카테고리 배지(색상 구분)와 component 이름을 표시해야 한다.

#### REQ-WEB-001-07-04 (Event-Driven)
**WHEN** 로그 뷰어 툴바의 source 필터 버튼(agent/node/flow/api/engine/system)을 토글하면, **THEN** 해당 source 카테고리의 로그만 표시하거나, 모든 필터가 비활성화되면 전체 로그를 표시해야 한다. 다중 선택이 가능해야 한다.

#### REQ-WEB-001-07-05 (Event-Driven)
**WHEN** 로그 뷰어 툴바의 컴포넌트 검색 입력에 텍스트를 입력하면, **THEN** component 이름에 해당 텍스트가 포함된(대소문자 구분 없음) 로그 항목만 표시해야 한다. 입력은 300ms 디바운스 처리한다.

#### REQ-WEB-001-07-06 (State-Driven)
**IF** 레벨 필터, source 필터, 컴포넌트 검색이 동시에 활성화된 상태 **THEN** 세 가지 필터 조건을 모두 AND 결합하여 적용해야 한다. 필터 결과 건수를 툴바에 실시간으로 표시해야 한다.

#### REQ-WEB-001-07-07 (Unwanted)
source 필터와 컴포넌트 검색 기능은 기존 가상화 렌더링 성능에 영향을 주지 **않아야 한다**. MAX_ENTRIES(10,000건) 상태에서도 필터링이 16ms(60fps) 이내에 완료되어야 한다.

### 4.8 Module 8: 동적 포트 시스템 (P0 - 버그 수정)

#### REQ-WEB-001-08-01 (Event-Driven)
**WHEN** 노드를 생성하거나 노드 설정이 변경될 때, **THEN** `computePortsForNode(nodeType, config)` 함수를 호출하여 노드 타입과 현재 설정(config)을 기반으로 포트 목록을 동적으로 계산해야 한다. 기존 `getDefaultPorts(nodeType)`의 정적 포트 대신 설정 기반 포트를 반환해야 한다.

#### REQ-WEB-001-08-02 (State-Driven)
**IF** 브릿지 노드의 direction이 `in`(agent->flow)인 상태 **THEN** 출력 포트(output)만 표시해야 한다. **IF** direction이 `out`(flow->agent)인 상태 **THEN** 입력 포트(input)만 표시해야 한다. **IF** direction이 `inout` 또는 `request_reply`인 상태 **THEN** 입력 포트와 출력 포트를 모두 표시해야 한다. direction이 미설정인 경우 기존 기본값(input+output)을 유지해야 한다.

#### REQ-WEB-001-08-03 (State-Driven)
**IF** 스위치 노드에 라우트가 설정된 상태 **THEN** 입력 포트 1개와 각 라우트별 출력 포트(라우트 이름으로 명명) + 기본(default) 출력 포트를 표시해야 한다. **IF** 라우트가 미설정인 상태 **THEN** 기본 입출력 포트(`in`, `out`)를 표시해야 한다.

#### REQ-WEB-001-08-04 (Event-Driven)
**WHEN** 에디터에서 노드의 설정(config)이 변경되면(예: 브릿지 direction 변경, 스위치 라우트 추가/삭제), **THEN** `computePortsForNode(nodeType, updatedConfig)`를 호출하여 포트를 재계산하고, 에디터 스토어의 해당 노드 `data.ports`를 업데이트해야 한다. 기존 엣지(연결선) 중 삭제된 포트에 연결된 엣지는 자동으로 제거해야 한다.

#### REQ-WEB-001-08-05 (Event-Driven)
**WHEN** 백엔드에서 브릿지 타입의 `NewNodeDef()`를 생성할 때, **THEN** direction 설정에 따라 적절한 Inputs/Outputs 포트를 생성해야 한다. `BridgeIn`은 Outputs만, `BridgeOut`은 Inputs만, `BridgeInOut`/`BridgeRequestReply`는 Inputs와 Outputs 모두를 기본 포트로 설정해야 한다.

### 4.9 Module 9: 에러 포트 타입 지원 (P0 - 버그 수정)

#### REQ-WEB-001-09-01 (State-Driven)
**IF** 프론트엔드가 백엔드로부터 포트 데이터를 수신한 상태 **THEN** `'input'`과 `'output'` 외에 `'error'`를 유효한 포트 direction으로 지원해야 한다. `PortDef` 타입과 `NodeTypeDefinition.ports[].direction` 타입에 `'error'`를 추가해야 한다.

#### REQ-WEB-001-09-02 (State-Driven)
**IF** 노드에 에러 포트가 존재하는 상태 **THEN** CustomNode 컴포넌트는 에러 포트를 출력 포트와 시각적으로 구분하여 빨간색으로 노드 오른쪽(출력 포트 아래)에 offset 배치하여 렌더링해야 한다. 출력 포트와 에러 포트가 모두 있는 경우 `((i+1)/(total+1))*100%` 공식으로 균등 분배한다.

#### REQ-WEB-001-09-03 (State-Driven)
**IF** NodeHandle이 에러 포트를 렌더링하는 상태 **THEN** 핸들은 빨간색(`bg-red-500`)으로 표시되어 출력 포트(초록색)와 입력 포트(파란색)와 구분되어야 한다.

#### REQ-WEB-001-09-04 (Event-Driven)
**WHEN** 에러 포트를 지원하는 노드 타입에 대해 `computePortsForNode()`가 호출될 때, **THEN** 반환되는 포트 배열에 에러 포트 정의(`direction: 'error'`)가 포함되어야 한다.

#### REQ-WEB-001-09-05 (State-Driven)
**IF** 에러 포트가 없는 기존 노드를 로드하는 상태 **THEN** 에러 포트 핸들 없이 기존과 동일하게 정상 렌더링되어야 한다.

### 4.10 Module 10: Handle ID 접두사 제거 (P0 - 리팩토링)

#### REQ-WEB-001-10-01 (Ubiquitous)
시스템은 **항상** 포트 이름(`name`)을 React Flow Handle ID로 직접 사용해야 한다. 포트 방향 접두사(`in-`, `out-`, `err-`)를 Handle ID에 추가**하지 않아야 한다**. 포트 direction은 `PortDef.direction` 필드로 이미 구분되므로 접두사는 불필요하다.

#### REQ-WEB-001-10-02 (State-Driven)
**IF** 백엔드가 React Flow Edge를 생성하는 상태 **THEN** `sourceHandle`에 Wire의 `SourcePort`를 그대로 설정하고, `targetHandle`에 Wire의 `TargetPort`를 그대로 설정해야 한다. 접두사를 추가하거나 제거하는 변환을 적용**하지 않아야 한다**.

#### REQ-WEB-001-10-03 (State-Driven)
**IF** 프론트엔드가 React Flow Edge를 백엔드 Wire로 변환하는 상태 **THEN** Edge의 `sourceHandle`을 Wire의 `source_port`로 직접 매핑하고, `targetHandle`을 `target_port`로 직접 매핑해야 한다. `TrimPrefix` 등의 변환을 적용**하지 않아야 한다**.

#### REQ-WEB-001-10-04 (State-Driven)
**IF** PropertyPanel에서 포트를 관리하는 상태 **THEN** Port 타입이 `'input' | 'output' | 'error'` 세 가지 direction을 모두 지원해야 한다. 삭제된 포트에 연결된 엣지 정리 시 포트 이름(`name`)으로 직접 비교해야 한다.

### 4.11 Module 11: 리스트 정렬 기능 (P1 - 신규 기능)

#### REQ-WEB-001-11-01 (Ubiquitous)
시스템은 **항상** 리스트 페이지(FlowListPage, AgentListPage)의 데이터를 정렬된 상태로 표시해야 한다. 명시적 정렬 요청이 없는 경우 기본 정렬은 `name:asc` (이름 오름차순)이다.

#### REQ-WEB-001-11-02 (Event-Driven)
**WHEN** 사용자가 정렬 가능한 컬럼 헤더를 클릭하면 **THEN** 해당 필드 기준으로 정렬 방향이 토글되어야 한다 (asc → desc → asc). 현재 정렬 기준과 방향을 시각적으로 표시해야 한다 (▲ 오름차순, ▼ 내림차순).

#### REQ-WEB-001-11-03 (State-Driven)
**IF** FlowListPage가 표시되는 상태 **THEN** 이름(name), 상태(status), 생성일(created_at), 수정일(updated_at) 컬럼이 정렬 가능해야 한다. 노드(node_count) 컬럼과 액션 컬럼은 정렬 불가이다.

#### REQ-WEB-001-11-04 (State-Driven)
**IF** AgentListPage가 표시되는 상태 **THEN** 이름(name), 타입(type), 상태(status) 컬럼이 정렬 가능해야 한다. 업타임, 메시지, 액션 컬럼은 정렬 불가이다.

#### REQ-WEB-001-11-05 (Event-Driven)
**WHEN** 사용자가 정렬 기준을 변경하면 **THEN** 프론트엔드는 `sort` 쿼리 파라미터를 `field:direction` 형식으로 백엔드 API에 전달해야 한다 (예: `?sort=name:asc`).

#### REQ-WEB-001-11-06 (State-Driven)
**IF** 백엔드가 `sort` 쿼리 파라미터를 수신한 상태 **THEN** `FlowServiceAdapter.ListFlows`와 `AgentServiceAdapter.ListAgents`는 결과를 페이지네이션 적용 전에 해당 필드와 방향으로 정렬해야 한다.

### 4.12 Module 12: 대시보드 패널 재구성 (P1 - 리팩토링)

#### REQ-WEB-001-12-01 (Event-Driven)
**WHEN** 대시보드 페이지(`/dashboard`)가 로드될 때, **THEN** 기존 2x2 위젯 그리드 대신 3패널 구조(FlowPanel, AgentPanel, ResourcePanel)를 렌더링해야 한다.

#### REQ-WEB-001-12-02 (State-Driven)
**IF** FlowPanel이 렌더링된 상태 **THEN** 상단 영역에 플로우 상태별 건수 요약(running, stopped, error, stored, loaded)을 배지 형태로 표시하고, 하단 영역에 플로우 리스트 테이블을 표시해야 한다.

#### REQ-WEB-001-12-03 (State-Driven)
**IF** FlowPanel 리스트 테이블이 표시되는 상태 **THEN** 각 행에 이름(클릭 시 에디터 이동), 상태(FlowStatusBadge), 노드 수(node_count), 동작 시간(updated_at 기반 상대 시간), 액션(시작/정지 토글 버튼)을 표시해야 한다. 기본 정렬은 이름(name) 오름차순이다.

#### REQ-WEB-001-12-04 (Event-Driven)
**WHEN** FlowPanel 리스트의 컬럼 헤더(이름)를 클릭하면, **THEN** 해당 필드 기준으로 정렬 방향이 토글되어야 한다 (asc -> desc -> asc). 현재 정렬 기준과 방향을 SortableHeader 컴포넌트로 시각적으로 표시해야 한다.

#### REQ-WEB-001-12-05 (Event-Driven)
**WHEN** FlowPanel 리스트의 액션 버튼(시작/정지)을 클릭하면, **THEN** 플로우의 현재 상태에 따라 적절한 API(`POST /flows/{id}/start` 또는 `POST /flows/{id}/stop`)를 호출하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-12-06 (State-Driven)
**IF** 플로우가 10개를 초과하는 상태 **THEN** FlowPanel 리스트는 최대 10개 행을 표시하고, 하단에 "더 보기" 링크를 표시하여 클릭 시 FlowListPage(`/flows`)로 이동해야 한다.

#### REQ-WEB-001-12-07 (State-Driven)
**IF** AgentPanel이 렌더링된 상태 **THEN** 상단 영역에 에이전트 상태별 건수 요약(total, active/running, inactive/stopped)을 배지 형태로 표시하고, 하단 영역에 에이전트 리스트 테이블을 표시해야 한다.

#### REQ-WEB-001-12-08 (State-Driven)
**IF** AgentPanel 리스트 테이블이 표시되는 상태 **THEN** 각 행에 이름, 타입, 상태(AgentStatusBadge), 업타임(uptime), 메시지 IN/OUT(stats.messages_in / stats.messages_out), 액션(시작/정지 토글 버튼)을 표시해야 한다. 기본 정렬은 이름(name) 오름차순이다.

#### REQ-WEB-001-12-09 (Event-Driven)
**WHEN** AgentPanel 리스트의 컬럼 헤더(이름)를 클릭하면, **THEN** 해당 필드 기준으로 정렬 방향이 토글되어야 한다.

#### REQ-WEB-001-12-10 (Event-Driven)
**WHEN** AgentPanel 리스트의 액션 버튼(시작/정지)을 클릭하면, **THEN** 에이전트의 현재 상태에 따라 적절한 API를 호출하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-12-11 (State-Driven)
**IF** 에이전트가 10개를 초과하는 상태 **THEN** AgentPanel 리스트는 최대 10개 행을 표시하고, 하단에 "더 보기" 링크를 표시하여 클릭 시 AgentListPage(`/agents`)로 이동해야 한다.

#### REQ-WEB-001-12-12 (State-Driven)
**IF** ResourcePanel이 렌더링된 상태 **THEN** CPU 사용률(%)과 메모리 사용률(%)을 게이지 또는 프로그레스 바로 표시하고, 기존 미니 에어리어 차트를 유지해야 한다.

#### REQ-WEB-001-12-13 (Ubiquitous)
시스템은 **항상** 대시보드 데이터를 기존 `useFlows()` 훅과 `useQuery(['monitor', 'metrics'])` 쿼리를 통해 조회해야 한다. 에이전트 데이터는 `useAgents()` 훅(또는 동등한 API 호출)을 사용해야 한다.

#### REQ-WEB-001-12-14 (State-Driven)
**IF** 반응형 레이아웃이 적용되는 상태 **THEN** 데스크톱에서는 FlowPanel과 AgentPanel을 나란히(2열) 배치하고 ResourcePanel을 전체 너비로 아래에 배치해야 한다. 모바일에서는 3패널을 단일 열(세로 스택)으로 배치해야 한다.

#### REQ-WEB-001-12-15 (Unwanted)
기존 SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget의 기능이 새 패널 구조에서 **누락되어서는 안 된다**. 기존 위젯이 제공하던 모든 정보가 새 패널에 포함되어야 한다.

### 4.13 Module 13: Import/Export 기능 (P2 - 신규 기능)

#### REQ-WEB-001-13-01 (Event-Driven)
**WHEN** 사용자가 플로우의 내보내기(Export) 버튼을 클릭하면, **THEN** 시스템은 `{ name, description?, definition }` 구조의 JSON 파일을 다운로드해야 한다. 런타임 필드(id, status, stats, timestamps)는 제거되어야 한다.

#### REQ-WEB-001-13-02 (Event-Driven)
**WHEN** 사용자가 FlowListPage에서 "전체 내보내기" 버튼을 클릭하면, **THEN** 시스템은 모든 플로우 Export 객체의 배열을 포함하는 JSON 파일을 다운로드해야 한다.

#### REQ-WEB-001-13-03 (Event-Driven)
**WHEN** 사용자가 에이전트의 내보내기(Export) 버튼을 클릭하면, **THEN** 시스템은 `{ name, type, config? }` 구조의 JSON 파일을 다운로드해야 한다.

#### REQ-WEB-001-13-04 (Event-Driven)
**WHEN** 사용자가 AgentListPage에서 "전체 내보내기" 버튼을 클릭하면, **THEN** 시스템은 모든 에이전트 Export 객체의 배열을 포함하는 JSON 파일을 다운로드해야 한다.

#### REQ-WEB-001-13-05 (Ubiquitous)
시스템은 **항상** Export 데이터에서 런타임 전용 필드(id, status, stats, created_at, updated_at, uptime, health)를 제거해야 한다. Export 파일에는 정의 데이터만 포함되어야 한다.

#### REQ-WEB-001-13-06 (Event-Driven)
**WHEN** 사용자가 FlowListPage 또는 AgentListPage에서 "가져오기" 버튼을 클릭하면, **THEN** ImportDialog가 열리고 `.json` 및 `.yaml`/`.yml` 확장자를 지원하는 파일 선택기(file picker)를 표시해야 한다.

#### REQ-WEB-001-13-07 (Event-Driven)
**WHEN** 사용자가 ImportDialog에서 파일을 선택하거나 드롭하면, **THEN** 시스템은 파일을 파싱하고(확장자로 JSON/YAML 자동 감지), 구조를 유효성 검사하고, 이름, 타입/설명, 편집 가능한 이름 필드가 포함된 미리보기를 표시해야 한다.

#### REQ-WEB-001-13-08 (State-Driven)
**IF** 파싱된 파일이 항목의 배열을 포함하는 상태 **THEN** ImportDialog는 각 항목에 대한 개별 이름 편집 필드가 포함된 리스트 미리보기를 표시해야 한다. 단일 항목 파일은 단일 미리보기를 표시해야 한다.

#### REQ-WEB-001-13-09 (Event-Driven)
**WHEN** 사용자가 ImportDialog에서 가져오기를 확인하면, **THEN** 시스템은 각 항목에 대해 적절한 Create API(`POST /flows` 또는 `POST /agents`)를 (편집된 이름과 함께) 호출해야 한다.

#### REQ-WEB-001-13-10 (Unwanted)
시스템은 동일한 이름의 기존 플로우/에이전트를 자동으로 덮어쓰기**하지 않아야 한다**. 사용자는 가져오기 전에 ImportDialog 미리보기에서 이름을 편집할 수 있어야 한다.

#### REQ-WEB-001-13-11 (State-Driven)
**IF** 가져온 플로우 파일에 필수 `name` 및 `definition` 필드가 없는 상태 **THEN** ImportDialog는 유효성 에러를 표시하고 확인 버튼을 비활성화해야 한다.

#### REQ-WEB-001-13-12 (State-Driven)
**IF** 가져온 에이전트 파일에 필수 `name` 및 `type` 필드가 없는 상태 **THEN** ImportDialog는 유효성 에러를 표시하고 확인 버튼을 비활성화해야 한다.

#### REQ-WEB-001-13-13 (Ubiquitous)
시스템은 **항상** CLI `xflowd flow import` / `xflowd agent import` 명령과 호환되는 Export 파일을 생성해야 한다. CLI에서 내보낸 파일은 웹 UI에서 가져올 수 있어야 한다.

> **v1.31.0 보강**: CLI 호환은 **이중 래핑(double-wrapping) 금지**를 포함한다.
> Export 파일이 이미 `{ name, definition }` 구조이므로, CLI 의 `flow import`/`flow
> create` 는 이를 다시 `definition` 으로 감싸서는 안 된다. CLI 측 동작은 SPEC-CLI-001
> 에서 정의한다.

#### REQ-WEB-001-13-14 (Event-Driven)
**WHEN** Import API 호출이 실패하면(예: 중복 이름, 유효성 에러), **THEN** ImportDialog는 API 응답의 에러 메시지를 표시하고, 대화상자를 열어 둔 채 사용자가 수정 후 재시도할 수 있어야 한다.

#### REQ-WEB-001-13-15 (Event-Driven) [v1.31.0 신규] Export 시 required_agents 추출

**WHEN** 사용자가 플로우를 내보내면, **THEN** Export 데이터는 플로우가 참조하는 모든 에이전트의 목록을 `required_agents` 배열로 포함해야 한다.

- `required_agents` 추출 로직(`extractAgentNames`)의 입력은 **React Flow 형식**의 노드 데이터이다 (이전 버그: XFlow 형식을 기대하여 항상 빈 배열 반환).
- 각 `required_agents` 항목은 에이전트 `name` 과 `type` 을 포함한다 (SPEC-FLOW-002 의 대체 에이전트 선택이 `type` 에 의존).
- 에이전트 이름 매칭은 **대소문자를 무시**해야 한다.

#### REQ-WEB-001-13-16 (Event-Driven) [v1.31.0 신규] Import 시 노드-에이전트 자동 매핑

**WHEN** 플로우를 가져올 때, **THEN** 시스템은 각 노드의 에이전트 참조를 현재 시스템 기준으로 재해결(re-resolve)해야 한다:

1. Export 파일의 노드별 `agent_id` 는 원본 시스템의 ID 이므로 신뢰하지 않는다 — import 시 제거한다
2. 노드의 에이전트 참조는 `agent_ref`(이름 기반) 필드로 채워, 가져오는 시스템의 에이전트 레지스트리에서 재해결되도록 한다
3. Export 시에는 노드의 `agent_id` 를 출력에서 제거하여 ID 가 시스템 간 누출되지 않도록 한다

#### REQ-WEB-001-13-17 (Unwanted) [v1.31.0 신규] 이중 래핑 금지

시스템은 이미 `{ name, definition }` 구조인 Export 파일을 가져오거나 생성할 때 `definition` 을 **다시 한 번 `definition` 으로 감싸서는 안 된다**. Export → Import round-trip 시 플로우 정의 구조가 보존되어야 한다.

#### REQ-WEB-001-13-18 (Ubiquitous) [v1.31.0 신규] Export/Import round-trip 무결성

시스템은 **항상** 플로우 Export → Import → Export 의 round-trip 후에도 노드, 와이어, 노드 설정(config), 에이전트 참조가 손실 없이 보존되도록 보장해야 한다. 특히 `wires` 변환 로직은 export 데이터에 무조건 적용되어야 하며, NodeDef unmarshal 은 필드 누락에 관대(lenient)해야 한다.

### 4.6 백엔드 버그 수정 (P0 - 버그 수정)

#### REQ-WEB-001-06-01 (Unwanted)
엔진의 포트 카운터 초기화 시 모든 포트(입력/출력/에러)에 대해 `atomic.Int64` 카운터를 0으로 초기화**해야 한다**. 일부 포트만 초기화되어 나머지 포트의 메시지 카운트가 0으로 보고되는 버그를 수정한다.

#### REQ-WEB-001-06-02 (Unwanted)
Bridge 노드의 `Process()` 메서드는 정상 처리 시 nil을 반환**해야 한다**. 불필요한 에러 반환으로 메시지 처리가 중단되는 버그를 수정한다.

### 4.15 Module 15: 화면 테마 시스템 (P1 - 신규 기능)

#### M15-1: CSS Variable 디자인 토큰 시스템

#### REQ-WEB-001-15-01 (Ubiquitous)
시스템은 **항상** `index.css`의 `@theme` 블록 및 `:root` 셀렉터에 시맨틱 컬러 토큰을 CSS Custom Property로 정의해야 한다. 토큰은 배경(`--color-bg-*`), 텍스트(`--color-text-*`), 테두리(`--color-border-*`), 상태(`--color-status-*`), 인터랙션(`--color-interactive-*`) 카테고리를 포함해야 한다.

#### REQ-WEB-001-15-02 (Ubiquitous)
시스템은 **항상** 시맨틱 토큰이 Tailwind 유틸리티 클래스에서 `var(--token-name)` 구문으로 참조 가능하도록 `@theme` 블록에 등록해야 한다.

#### REQ-WEB-001-15-03 (Event-Driven)
**WHEN** 테마가 변경되면, **THEN** `<html>` 요소의 `data-theme` 속성 값이 해당 테마 식별자(`day`, `night`, `custom`)로 업데이트되어야 하고, CSS 셀렉터 `[data-theme="..."]`에 정의된 토큰 값이 즉시 적용되어야 한다.

#### M15-2: Day/Night 테마 프리셋

#### REQ-WEB-001-15-04 (Ubiquitous)
시스템은 **항상** Day(라이트) 프리셋과 Night(다크) 프리셋의 시맨틱 토큰 매핑을 제공해야 한다. Day 프리셋은 현재 라이트 모드 색상(예: `bg-gray-50`, `text-gray-900`)을, Night 프리셋은 현재 다크 모드 색상(예: `bg-gray-900`, `text-gray-100`)을 CSS Variable 값으로 정의해야 한다.

#### REQ-WEB-001-15-05 (Event-Driven)
**WHEN** 사용자가 Day 또는 Night 테마를 선택하면, **THEN** 해당 프리셋의 모든 시맨틱 토큰 값이 `[data-theme]` 셀렉터를 통해 적용되어야 하고, 기존 `dark:` 클래스 기반 스타일과 시각적으로 동일한 결과를 보여야 한다.

#### M15-3: Custom 테마 지원

#### REQ-WEB-001-15-06 (Event-Driven)
**WHEN** 사용자가 Custom 테마 모드를 선택하면, **THEN** 사용자가 이전에 저장한 커스텀 테마 색상이 시맨틱 토큰에 적용되어야 한다. 저장된 커스텀 테마가 없으면 Day 프리셋을 기본값으로 사용해야 한다.

#### REQ-WEB-001-15-07 (Ubiquitous)
시스템은 **항상** 커스텀 테마 데이터(시맨틱 토큰 값 맵)를 Zustand 스토어에 저장하고, `persist` 미들웨어를 통해 localStorage에 영속화해야 한다.

#### REQ-WEB-001-15-08 (Event-Driven)
**WHEN** 사용자가 커스텀 테마 색상을 저장(Save)하면, **THEN** 변경된 토큰 값이 즉시 Zustand 스토어에 반영되고 localStorage에 영속화되어야 한다.

#### M15-4: 테마 선택 UI

#### REQ-WEB-001-15-09 (Event-Driven)
**WHEN** 사용자가 Header의 테마 버튼을 클릭하면, **THEN** System, Day, Night, Custom 4가지 옵션이 포함된 드롭다운/팝오버가 표시되어야 한다. 현재 활성 테마에 체크 또는 하이라이트 표시가 있어야 한다.

#### REQ-WEB-001-15-10 (Event-Driven)
**WHEN** 사용자가 드롭다운에서 테마 옵션을 선택하면, **THEN** 선택된 테마가 즉시 적용되어야 하고, Header의 테마 아이콘이 선택된 테마를 반영하여 변경되어야 한다(Day=Sun, Night=Moon, System=Monitor, Custom=Palette).

#### REQ-WEB-001-15-11 (Ubiquitous)
시스템은 **항상** 기존 3-cycle 토글(light->dark->system)을 4옵션 드롭다운/팝오버로 대체해야 한다. `uiStore`의 `theme` 타입은 `'system' | 'day' | 'night' | 'custom'`으로 변경되어야 한다.

#### M15-5: 커스텀 테마 에디터

#### REQ-WEB-001-15-12 (Event-Driven)
**WHEN** 사용자가 Custom 테마 드롭다운 옆의 편집 버튼(또는 Custom 옵션 선택 후 에디터 진입)을 클릭하면, **THEN** 시맨틱 토큰별 컬러 피커가 포함된 테마 에디터 UI(모달 또는 사이드 패널)가 표시되어야 한다.

#### REQ-WEB-001-15-13 (Event-Driven)
**WHEN** 사용자가 테마 에디터에서 색상 값을 변경하면, **THEN** 변경된 색상이 실시간으로 화면에 라이브 프리뷰되어야 한다. 저장 전 변경 사항은 임시 상태로 관리되며, 취소(Cancel) 시 이전 테마로 복원되어야 한다.

#### REQ-WEB-001-15-14 (Event-Driven)
**WHEN** 사용자가 테마 에디터에서 "저장" 버튼을 클릭하면, **THEN** 현재 편집 중인 토큰 값이 커스텀 테마로 저장되어야 한다. **WHEN** "취소" 버튼을 클릭하면, **THEN** 모든 편집 내용이 폐기되고 이전 테마 상태로 복원되어야 한다.

#### REQ-WEB-001-15-15 (Event-Driven)
**WHEN** 사용자가 테마 에디터에서 "초기화" 버튼을 클릭하면, **THEN** 커스텀 테마가 Day 프리셋 기본값으로 리셋되어야 한다.

#### M15-6: CSS 마이그레이션

#### REQ-WEB-001-15-16 (Ubiquitous)
시스템은 **항상** 기존 하드코딩된 `dark:` Tailwind variant 클래스를 CSS Variable 기반 시맨틱 토큰으로 점진적으로 마이그레이션해야 한다. 마이그레이션 후에도 Day/Night 테마에서 기존과 시각적으로 동일한 결과를 보여야 한다.

#### REQ-WEB-001-15-17 (State-Driven)
**IF** `dark:` 클래스와 CSS Variable 토큰이 동일 요소에 공존하는 과도기 상태라면, **THEN** CSS Variable 토큰이 우선 적용되어야 하고, `dark:` 클래스는 fallback으로만 동작해야 한다.

#### REQ-WEB-001-15-18 (Unwanted)
테마 전환 시 FOUC(Flash of Unstyled Content)가 발생**하지 않아야 한다**. `<html>` 요소의 `data-theme` 속성 변경은 페인트 전에 동기적으로 적용되어야 한다.

#### REQ-WEB-001-15-19 (Event-Driven)
**WHEN** 브라우저를 새로고침하면, **THEN** localStorage에 저장된 테마 설정이 즉시 로드되어 이전 세션과 동일한 테마가 적용되어야 한다.

#### REQ-WEB-001-15-20 (Event-Driven)
**WHEN** System 테마 모드에서 운영체제의 `prefers-color-scheme` 설정이 변경되면, **THEN** 자동으로 Day 또는 Night 프리셋으로 전환되어야 한다.

### 4.16 Module 16: 대시보드 커스터마이징 시스템 (P1 - 신규 기능)

#### M16-1: 멀티 대시보드 데이터 모델

#### REQ-WEB-001-16-01 (Ubiquitous)
시스템은 **항상** `uiStore`에 `dashboardPages: DashboardPageConfig[]` 상태를 유지해야 한다. 각 `DashboardPageConfig`는 `{ id: string, name: string, isDefault: boolean, panels: PanelConfig[], layout: DashboardLayoutItem[] }` 구조를 가져야 한다.

#### REQ-WEB-001-16-02 (Event-Driven)
**WHEN** 기존 단일 대시보드 상태(`dashboardLayout`, `dashboardVisibleMetrics`, `flowPanelTitle`, `agentPanelTitle` 등)가 localStorage에 존재할 때 앱이 로드되면, **THEN** Zustand persist `migrate` 함수가 해당 상태를 `DashboardPageConfig` 구조의 기본 페이지(`id='default'`)로 자동 변환해야 한다.

#### REQ-WEB-001-16-03 (Ubiquitous)
시스템은 **항상** `activeDashboardId` 상태를 유지하여 현재 표시 중인 대시보드 페이지를 추적해야 한다. 초기값은 `isDefault=true`인 페이지의 `id`여야 한다.

#### REQ-WEB-001-16-04 (Ubiquitous)
시스템은 **항상** 대시보드 CRUD 액션(`addDashboardPage`, `removeDashboardPage`, `updateDashboardPage`, `setDefaultDashboardPage`, `setActiveDashboard`)을 uiStore에 제공해야 한다.

#### REQ-WEB-001-16-05 (Ubiquitous)
시스템은 **항상** 패널 CRUD 액션(`addPanel`, `removePanel`, `updatePanelConfig`)을 uiStore에 제공해야 한다. 각 액션은 `activeDashboardId`가 가리키는 페이지의 `panels`와 `layout` 배열을 수정해야 한다.

#### REQ-WEB-001-16-06 (Unwanted)
시스템은 `isDefault=true`인 페이지가 2개 이상 존재하는 상태를 **허용하지 않아야 한다**. `setDefaultDashboardPage` 호출 시 기존 기본 페이지의 `isDefault`를 `false`로 변경한 후 대상 페이지를 `true`로 설정해야 한다.

#### REQ-WEB-001-16-07 (Unwanted)
시스템은 마지막 남은 대시보드 페이지의 삭제를 **허용하지 않아야 한다**. `dashboardPages` 배열의 길이가 1일 때 `removeDashboardPage` 호출을 무시하거나 경고를 표시해야 한다.

#### REQ-WEB-001-16-08 (Ubiquitous)
시스템은 **항상** `dashboardPages`와 `activeDashboardId`를 Zustand `persist` 미들웨어의 `partialize`에 포함하여 localStorage에 영속화해야 한다.

#### M16-2: 대시보드 관리 UI

#### REQ-WEB-001-16-09 (Ubiquitous)
시스템은 **항상** `DashboardPage.tsx` 상단에 `DashboardToolbar` 컴포넌트를 렌더링해야 한다. 툴바는 대시보드 선택 드롭다운, 추가 버튼, 삭제 버튼, 기본 지정 버튼을 포함해야 한다.

#### REQ-WEB-001-16-10 (State-Driven)
**IF** 대시보드 선택 드롭다운이 열린 상태 **THEN** 모든 대시보드 페이지 목록을 표시해야 하며, `isDefault=true`인 페이지 이름 옆에 별표 표시를 해야 한다.

#### REQ-WEB-001-16-11 (Event-Driven)
**WHEN** 사용자가 추가 버튼을 클릭하면, **THEN** 대시보드 이름 입력 다이얼로그를 표시하고, 이름 입력 후 확인 시 빈 패널 구성의 새 대시보드 페이지를 생성하여 자동으로 활성화해야 한다.

#### REQ-WEB-001-16-12 (Event-Driven)
**WHEN** 사용자가 삭제 버튼을 클릭하면, **THEN** 현재 활성 대시보드의 삭제 확인 다이얼로그를 표시해야 한다. 확인 시 해당 페이지를 삭제하고, 삭제된 페이지가 기본 페이지였다면 남은 첫 번째 페이지를 기본으로 자동 지정해야 한다.

#### REQ-WEB-001-16-13 (Event-Driven)
**WHEN** 사용자가 기본 지정 버튼을 클릭하면, **THEN** 현재 활성 대시보드를 기본 페이지로 설정해야 한다. 기존 기본 페이지의 `isDefault`는 `false`로 변경되어야 한다.

#### REQ-WEB-001-16-14 (Event-Driven)
**WHEN** 사용자가 대시보드 이름 영역을 더블클릭하거나 연필 아이콘을 클릭하면, **THEN** 이름이 인라인 편집 모드로 전환되어야 한다. Enter 키 또는 포커스 해제 시 변경된 이름이 저장되어야 한다.

#### REQ-WEB-001-16-15 (Event-Driven)
**WHEN** 앱이 로드되면, **THEN** `isDefault=true`인 대시보드 페이지가 자동으로 활성화(`activeDashboardId` 설정)되어야 한다. 기본 페이지가 없으면 첫 번째 페이지를 활성화해야 한다.

#### M16-3: 패널 추가/삭제 시스템

#### REQ-WEB-001-16-16 (State-Driven)
**IF** 대시보드가 편집 모드(`dashboardEditMode=true`)인 상태 **THEN** 툴바 또는 그리드 영역에 "패널 추가" 버튼을 표시해야 한다.

#### REQ-WEB-001-16-17 (Event-Driven)
**WHEN** 사용자가 "패널 추가" 버튼을 클릭하면, **THEN** `AddPanelDialog`가 열리고, 5종 패널 타입(`flows`, `agents`, `resource`, `devices`, `logs`)을 아이콘과 설명과 함께 선택 가능하게 표시해야 한다.

#### REQ-WEB-001-16-18 (Event-Driven)
**WHEN** 사용자가 `AddPanelDialog`에서 패널 타입을 선택하면, **THEN** 해당 타입의 패널이 기본 크기로 그리드의 빈 공간에 자동 배치되어야 한다. 빈 공간이 없으면 기존 패널 아래(하단)에 추가되어야 한다.

#### REQ-WEB-001-16-19 (State-Driven)
**IF** 대시보드가 편집 모드인 상태 **THEN** 각 패널의 우상단에 삭제 버튼(X)을 표시해야 한다.

#### REQ-WEB-001-16-20 (Event-Driven)
**WHEN** 사용자가 편집 모드에서 패널의 삭제 버튼(X)을 클릭하면, **THEN** 해당 패널이 확인 없이 즉시 삭제되어야 한다. 패널의 `PanelConfig`와 대응하는 `layout` 항목이 모두 제거되어야 한다.

#### REQ-WEB-001-16-21 (Ubiquitous)
시스템은 **항상** 동일 타입의 패널을 복수 개 추가하는 것을 허용해야 한다. 예를 들어 플로우 패널(`flows`)을 2개 이상 배치할 수 있어야 한다.

#### M16-4: 디바이스/로그 패널 타입

#### REQ-WEB-001-16-22 (Ubiquitous)
시스템은 **항상** `DevicePanel` 컴포넌트를 제공해야 한다. 상단에 디바이스 상태 요약 카드(전체/온라인/오프라인 수)를, 하단에 디바이스 리스트 테이블(이름, 타입, 상태, 마지막 통신 시각)을 표시해야 한다.

#### REQ-WEB-001-16-23 (Ubiquitous)
시스템은 **항상** `LogPanel` 컴포넌트를 제공해야 한다. WebSocket을 통해 실시간 로그 스트림을 수신하여 표시하고, 소스 필터(agent/node/flow/system)와 레벨 필터(debug/info/warn/error)를 제공해야 한다.

#### REQ-WEB-001-16-24 (Event-Driven)
**WHEN** DevicePanel에서 PanelSettingsDropdown을 열면, **THEN** 패널 제목 편집과 표시 컬럼 선택(이름, 타입, 상태, 마지막 통신) 옵션을 제공해야 한다.

#### REQ-WEB-001-16-25 (Event-Driven)
**WHEN** LogPanel에서 PanelSettingsDropdown을 열면, **THEN** 패널 제목 편집과 최대 표시 줄 수 설정(50/100/200/500) 옵션을 제공해야 한다.

#### REQ-WEB-001-16-26 (Ubiquitous)
시스템은 **항상** 기존 `FlowPanel`, `AgentPanel`, `ResourceWidget` 컴포넌트를 패널 타입(`flows`, `agents`, `resource`)으로 재사용해야 한다. 기존 컴포넌트의 변경은 최소화해야 한다.

### 4.17 Module 17: UI 레이아웃 재구성 (P1 - 개선)

#### M17-1: 대시보드 관리 UI Header 이전

#### REQ-WEB-001-17-01 (Ubiquitous)
시스템은 **항상** `Header.tsx`에서 현재 라우트(`location.pathname`)에 따라 좌측 영역을 조건부 렌더링해야 한다. `'/'` 라우트에서는 대시보드 선택/관리 컨트롤을, 기타 라우트에서는 정적 페이지 타이틀(`<h1>`)을 표시해야 한다.

#### REQ-WEB-001-17-02 (Ubiquitous)
시스템은 **항상** 대시보드 라우트(`'/'`)의 Header 좌측에 다음 컨트롤을 표시해야 한다: 대시보드 선택 드롭다운(`<select>`), 이름 편집 버튼(Pencil), 대시보드 추가 버튼(Plus), 삭제 버튼(Trash2), 기본 지정 버튼(Star). 이름 편집 활성화 시 드롭다운이 인라인 입력 필드로 전환되어야 한다.

#### REQ-WEB-001-17-03 (Unwanted)
시스템은 `DashboardToolbar.tsx` 컴포넌트를 **포함하지 않아야 한다**. 대시보드 관리 기능은 `Header.tsx`의 라우트 조건부 렌더링으로 완전히 대체되어야 한다.

#### M17-2: 편집 모드 버튼 인라인 배치

#### REQ-WEB-001-17-04 (State-Driven)
**IF** 대시보드가 편집 모드(`dashboardEditMode=true`)인 상태 **THEN** 조작 바에서 편집 토글 버튼 앞에 "추가"(PanelTop 아이콘) 버튼과 "초기화"(RotateCcw 아이콘) 버튼을 인라인으로 표시해야 한다. 별도의 편집 모드 설정 바는 사용하지 않아야 한다.

#### REQ-WEB-001-17-05 (State-Driven)
**IF** 디바이스 페이지가 편집 모드(`deviceGridEditMode=true`)인 상태 **THEN** 조작 바에서 편집 토글 버튼 앞에 "추가"(Plus 아이콘) 버튼과 "초기화"(RotateCcw 아이콘) 버튼을 인라인으로 표시해야 한다. "추가" 클릭 시 AddDeviceDialog가 열려야 한다.

#### M17-3: 디바이스 페이지 레이아웃 통일

#### REQ-WEB-001-17-06 (Ubiquitous)
시스템은 **항상** `DeviceListPage.tsx` 상단에 설명 텍스트와 편집 모드 토글을 포함하는 조작 바를 표시해야 한다. 별도의 `<h2>` 페이지 타이틀과 "추가" 버튼을 포함하는 헤더 영역을 사용하지 않아야 한다. 페이지 타이틀은 `Header.tsx`에서 라우트 기반으로 표시되어야 한다.

#### M17-4: console-logger 로그 레벨 수정

#### REQ-WEB-001-17-07 (Unwanted)
시스템은 `console_logger.go`의 `Process` 메서드에서 `slog.Debug` 레벨로 메시지를 기록**하지 않아야 한다**. 기본 로그 핸들러 레벨(`slog.LevelInfo`)에 의해 메시지가 필터링되는 것을 방지하기 위해 `slog.Info` 레벨을 사용해야 한다.

### 4.18 Module 18: 플로우 리스트 개선 + UI 버그 수정 (P1 - 신규 기능/개선)

#### M18-1: LGAP 프로토콜 표시 수정

#### REQ-WEB-001-18-01 (Unwanted)
시스템은 LGAP 에이전트의 디바이스를 "NASA indoor" 으로 표시**하지 않아야 한다**. `NASADeviceAdapter`의 `Protocol()` 메서드는 `NASADeviceInfo.Protocol` 필드가 설정된 경우 해당 값을 반환하고, 비어있으면 "nasa"를 기본값으로 사용해야 한다.

#### REQ-WEB-001-18-02 (State-Driven)
**IF** `NASADeviceInfo.Protocol`이 비어있지 않은 경우 **THEN** `Name()` 메서드는 해당 프로토콜 이름을 대문자로 사용해야 한다 (예: "LGAP indoor 11"). `State()` 메서드는 `ExtraProperties` 맵의 키-값을 `Properties`에 병합해야 한다.

#### M18-2: 로그 레벨 설정 탭 이전

#### REQ-WEB-001-18-03 (Ubiquitous)
시스템은 **항상** 에이전트 상세 패널의 **설정 탭(`ConfigTab`)**에 로그 레벨 드롭다운을 표시해야 한다. 통계 탭(`StatsTab`)에는 로그 레벨 UI를 포함하지 않아야 한다.

#### M18-3: 다크 모드 확장 패널 배경 수정

#### REQ-WEB-001-18-04 (Ubiquitous)
시스템은 **항상** 에이전트 목록 및 플로우 목록의 확장된 상세 패널 배경에 `bg-(--color-bg-sunken)` CSS Variable 토큰을 사용해야 한다. 반투명 배경(`dark:bg-gray-800/50`)을 사용하지 않아야 한다.

#### M18-4: 플로우 리스트 업타임 컬럼

#### REQ-WEB-001-18-05 (State-Driven)
**IF** 플로우의 상태가 `running`이고 `uptime` 필드가 존재하는 경우 **THEN** 플로우 목록 테이블의 "업타임" 컬럼에 해당 값을 표시해야 한다. 그 외 상태에서는 `-`를 표시해야 한다.

#### M18-5: 플로우 자동 시작 토글

#### REQ-WEB-001-18-06 (State-Driven)
**IF** 사용자가 플로우 목록의 "자동시작" 토글을 클릭하는 경우 **THEN** 시스템은 `PATCH /flows/{id}` API로 `auto_start` 값을 토글하고, 플로우 목록을 갱신해야 한다. 자동 시작 설정은 플로우 메타데이터(`auto_start` 키)에 저장되어야 한다.

#### REQ-WEB-001-18-07 (Event-Driven)
**WHEN** 서버가 시작되는 경우 **THEN** 저장소에서 `auto_start` 메타데이터가 `"true"`인 플로우를 자동으로 배포하고 시작해야 한다. 각 플로우의 성공/실패를 로그에 기록해야 한다.

### 4.19 Module 19: 플로우 Undeploy API + 대시보드 상태 아이콘 (P1 - 신규 기능/개선)

#### M19-1: 플로우 Undeploy API

#### REQ-WEB-001-19-01 (Event-Driven)
**WHEN** 사용자가 `POST /flows/{id}/undeploy` API를 호출하는 경우 **THEN** 시스템은 해당 플로우를 엔진 메모리에서 제거(배포 해제)하고, `flow_undeployed` WebSocket 이벤트를 발행해야 한다.

#### REQ-WEB-001-19-02 (Unwanted)
시스템은 `running` 또는 `paused` 상태의 플로우를 배포 해제**하지 않아야 한다**. 해당 상태에서 undeploy 요청 시 에러를 반환해야 한다.

#### M19-2: FlowActionMenu 상태별 액션 제어

#### REQ-WEB-001-19-03 (State-Driven)
**IF** 플로우 상태가 `stored`, `loaded`, 또는 `stopped`인 경우 **THEN** "시작" 버튼이 활성화되어야 한다. 그 외 상태에서는 비활성화되어야 한다.

#### REQ-WEB-001-19-04 (State-Driven)
**IF** 플로우 상태가 `stored`인 경우 **THEN** "배포" 버튼(Cable 아이콘)이 활성화되어야 한다. **IF** 플로우 상태가 `loaded`, `stopped`, 또는 `error`인 경우 **THEN** "배포 해제" 버튼(Unplug 아이콘)이 활성화되어야 한다.

#### M19-3: 대시보드 패널 상태 아이콘

#### REQ-WEB-001-19-05 (Ubiquitous)
시스템은 **항상** 대시보드 FlowPanel의 상태 컬럼에 텍스트 뱃지 대신 `STATUS_CONFIG`에 정의된 아이콘을 표시해야 한다. 각 아이콘에는 `title` 속성으로 상태 레이블 툴팁을 제공해야 한다.

#### REQ-WEB-001-19-06 (Ubiquitous)
시스템은 **항상** 대시보드 AgentPanel의 상태 컬럼에 아이콘을 표시해야 한다. 연결됨(Activity), 연결 해제(CircleStop), 오류(AlertTriangle) 아이콘을 사용하고, 각각 적절한 색상(green/gray/red)을 적용해야 한다.

#### REQ-WEB-001-19-07 (Ubiquitous)
시스템은 **항상** 대시보드 DevicePanel의 상태 컬럼에 Wifi(온라인, green)/WifiOff(오프라인, gray) 아이콘을 표시해야 한다. 텍스트 뱃지 컴포넌트(`DeviceStatusBadge`)를 사용하지 않아야 한다.

### 4.20 Module 20: 에이전트 연결 상태 정확성 개선 (P1 - 버그 수정)

**REQ-WEB-001-20-01** (State-Driven)
While 에이전트가 StateRunning 상태이지만 트랜스포트가 실제 연결되지 않은 상태에서, the system shall API 응답의 connected 필드를 false로 반환한다.

**REQ-WEB-001-20-02** (Event-Driven)
When agent.TransportChecker 인터페이스를 구현한 에이전트의 connected 필드 결정 시, the system shall TransportConnected() 메서드의 반환값을 사용하여 실제 트랜스포트 연결 상태를 반영한다.

**REQ-WEB-001-20-03** (State-Driven)
While 에이전트의 connected 필드가 false인 상태에서, the system shall API 응답에서 uptime 필드를 빈 값으로 반환하여 업타임이 표시되지 않도록 한다.

**REQ-WEB-001-20-04** (Unwanted Behavior)
The system shall NOT use agent.status === 'running' 라이프사이클 상태만으로 에이전트 연결 상태를 판단한다. connected 필드를 우선 사용한다.

**REQ-WEB-001-20-05** (State-Driven)
While 에이전트 패널에서 connected가 false인 에이전트의 액션 버튼 표시 시, the system shall 시작(Play) 버튼을 활성화하고 중지(Pause) 버튼을 비활성화한다.

### 4.21 Module 21: 플로우 리스트 상태 아이콘 + 상태 필터링 (P1 - 개선)

**REQ-WEB-FLOW-ICON-01** (EARS: Event-driven)
WHEN 플로우 리스트가 렌더링될 때, THE SYSTEM SHALL 각 플로우의 상태를 대시보드 FlowPanel과 동일한 아이콘으로 표시한다. (running→Activity, stopped→CircleStop, error→AlertTriangle, stored→FileText, loaded→Rocket)

**REQ-WEB-FLOW-FILTER-01** (EARS: Event-driven)
WHEN 사용자가 상태 필터 뱃지 버튼을 클릭할 때, THE SYSTEM SHALL 해당 상태의 플로우만 필터링하여 목록에 표시한다.

**REQ-WEB-FLOW-FILTER-02** (EARS: Event-driven)
WHEN 사용자가 이미 활성화된 상태 필터 뱃지를 재클릭할 때, THE SYSTEM SHALL 필터를 해제하여 전체 플로우를 표시한다.

### 4.22 Module 22: 패널 설정 다이얼로그 (P2 - 신규 기능)

#### M22-1: PanelSettingsDialog 모달

#### REQ-WEB-001-22-01 (Event-Driven)
**WHEN** 사용자가 대시보드 패널의 설정 버튼을 클릭하는 경우 **THEN** 시스템은 PanelSettingsDialog 모달을 열고, 해당 패널의 제목과 타입별 설정 섹션을 표시해야 한다.

#### REQ-WEB-001-22-02 (Event-Driven)
**WHEN** 사용자가 PanelSettingsDialog에서 패널 제목을 편집하고 저장하는 경우 **THEN** 시스템은 uiStore의 `updatePanelTitle` 메서드를 호출하여 패널 제목을 갱신해야 한다.

#### REQ-WEB-001-22-03 (Event-Driven)
**WHEN** 사용자가 설정을 변경하고 저장 버튼을 클릭하는 경우 **THEN** 시스템은 uiStore의 `updatePanelConfig` 메서드를 호출하여 패널 설정을 저장하고 다이얼로그를 닫아야 한다.

#### M22-2: ColumnsSection (컬럼 가시성 토글)

#### REQ-WEB-001-22-04 (State-Driven)
**IF** 패널 타입이 `flows` 또는 `agents`인 경우 **THEN** PanelSettingsDialog는 ColumnsSection을 표시하여 테이블에 표시할 컬럼을 체크박스로 선택할 수 있어야 한다.

#### M22-3: FilterSection (디바이스 필터)

#### REQ-WEB-001-22-05 (State-Driven)
**IF** 패널 타입이 `devices`인 경우 **THEN** PanelSettingsDialog는 FilterSection을 표시하여 에이전트별, 프로토콜별 필터 조건을 설정할 수 있어야 한다.

#### M22-4: LogsSection (로그 설정)

#### REQ-WEB-001-22-06 (State-Driven)
**IF** 패널 타입이 `logs`인 경우 **THEN** PanelSettingsDialog는 LogsSection을 표시하여 최대 줄 수, 로그 레벨 필터, 소스 필터를 설정할 수 있어야 한다.

### 4.23 Module 23: 악센트 컬러 시스템 (P2 - 신규 기능)

#### M23-1: 패널 색상 (panelColor)

#### REQ-WEB-001-23-01 (Event-Driven)
**WHEN** 사용자가 미니프리뷰의 _base 그룹(전체 색상) 영역을 클릭하는 경우 **THEN** 시스템은 AccentGroupControls에서 패널 기본 색상(panelColor)을 설정할 수 있어야 한다. _base 그룹은 체크박스 없이 항상 활성 상태여야 한다.

#### M23-2: 요소별 악센트 컬러 (accentElements)

#### REQ-WEB-001-23-02 (State-Driven)
**IF** 패널에 accentElements 설정이 존재하는 경우 **THEN** 시스템은 `Record<string, string|boolean>` 형태로 그룹별 활성화 여부와 색상 값을 저장하고, 각 패널 컴포넌트에서 `acColor(group)` 헬퍼 패턴으로 해당 그룹의 컬러를 적용해야 한다.

#### M23-3: AccentGroupControls

#### REQ-WEB-001-23-03 (Event-Driven)
**WHEN** 사용자가 미니프리뷰에서 특정 그룹 영역을 클릭하는 경우 **THEN** AccentGroupControls는 해당 그룹의 활성화 토글, 프리셋 컬러 선택, 커스텀 컬러 피커, 리셋 기능을 제공해야 한다.

#### M23-4: 4종 미니프리뷰 컴포넌트

#### REQ-WEB-001-23-04 (State-Driven)
**IF** 패널 타입이 `device`(NASA 리모컨)인 경우 **THEN** NasaMiniPreview를 표시하고, indicators/temperature/modes/fan/borders/vane 그룹을 클릭 가능한 영역으로 제공해야 한다.

#### REQ-WEB-001-23-05 (State-Driven)
**IF** 패널 타입이 `flows`, `agents`, 또는 `devices`(리스트)인 경우 **THEN** ListMiniPreview를 표시하고, header/badges/table 그룹을 클릭 가능한 영역으로 제공해야 한다.

#### REQ-WEB-001-23-06 (State-Driven)
**IF** 패널 타입이 `resource`인 경우 **THEN** ResourceMiniPreview를 표시하고, header/cpu/memory/throughput/errorRate 그룹을 클릭 가능한 영역으로 제공해야 한다.

#### REQ-WEB-001-23-07 (State-Driven)
**IF** 패널 타입이 `logs`인 경우 **THEN** LogMiniPreview를 표시하고, header/levels/timestamp/source 그룹을 클릭 가능한 영역으로 제공해야 한다.

#### M23-5: _base 그룹 통합

#### REQ-WEB-001-23-08 (Ubiquitous)
시스템은 **항상** 모든 미니프리뷰 최상단에 '전체 색상' 클릭 존을 표시해야 한다. 이 영역은 conic-gradient 무지개로 시각적으로 표시되며, 클릭 시 _base 그룹이 선택되어야 한다.

#### M23-6: 패널 컴포넌트 적용

#### REQ-WEB-001-23-09 (State-Driven)
**IF** 패널 설정에 accentElements가 정의된 경우 **THEN** FlowPanel, AgentPanel, DevicePanel(리스트), LogPanel, ResourceWidget은 `acColor(group)` 헬퍼 패턴으로 해당 그룹별 컬러를 인라인 스타일로 적용해야 한다.

#### M23-7: NASA 리모컨 서브프로퍼티

#### REQ-WEB-001-23-10 (State-Driven)
**IF** NasaMiniPreview의 temperature 그룹이 선택된 경우 **THEN** AccentGroupControls는 value/label 하위 속성을 SubPropertyRow 컴포넌트로 개별 색상 지정할 수 있어야 한다.

### 4.24 Module 24: 단일 디바이스 패널 + 패널 추가 개선 (P2 - 신규 기능)

#### M24-1: SingleDevicePanel

#### REQ-WEB-001-24-01 (State-Driven)
**IF** 대시보드 패널 타입이 `device`인 경우 **THEN** 시스템은 SingleDevicePanel을 렌더링하여 단일 디바이스의 NASA 리모컨 UI를 대시보드 패널로 표시해야 한다.

#### M24-2: PanelType 확장

#### REQ-WEB-001-24-02 (Ubiquitous)
시스템은 **항상** PanelType 유니언에 `'device'` 타입을 포함해야 한다. `'device'` 타입은 기존 `'devices'`(리스트)와 별도로 단일 디바이스 패널을 나타낸다.

#### M24-3: addPanelWithConfig API

#### REQ-WEB-001-24-03 (Event-Driven)
**WHEN** 사용자가 AddPanelDialog에서 device 타입 패널을 추가하는 경우 **THEN** 시스템은 uiStore의 `addPanelWithConfig` 메서드를 호출하여 config(에이전트ID, 디바이스ID)와 title을 지정하여 패널을 추가해야 한다.

#### M24-4: AddPanelDialog 개선

#### REQ-WEB-001-24-04 (Event-Driven)
**WHEN** AddPanelDialog에서 패널 타입을 선택하는 경우 **THEN** 시스템은 패널 타입 선택 UI를 제공하고, `device` 타입 선택 시 에이전트 선택 콤보박스와 디바이스 선택 콤보박스를 추가로 표시해야 한다.

### 4.25 Module 25: 에이전트 리스트 개선 (P2 - 개선)

#### M25-1: AgentSearchFilter

#### REQ-WEB-001-25-01 (Event-Driven)
**WHEN** 에이전트 목록 페이지가 로드될 때 **THEN** 시스템은 AgentSearchFilter 컴포넌트를 표시하여 검색 입력과 상태 필터 아이콘 뱃지 토글을 제공해야 한다. FlowSearchFilter와 동일한 UI 패턴을 사용해야 한다.

#### M25-2: 상태 아이콘

#### REQ-WEB-001-25-02 (Ubiquitous)
시스템은 **항상** 에이전트 목록의 상태 컬럼에 텍스트 뱃지 대신 아이콘을 표시해야 한다. 연결됨(Activity, green), 중지(CircleStop, gray), 오류(AlertTriangle, red) 아이콘을 사용해야 한다.

#### M25-3: 페이지네이션

#### REQ-WEB-001-25-03 (Event-Driven)
**WHEN** 에이전트 목록의 항목 수가 페이지 크기를 초과하는 경우 **THEN** 시스템은 10/20/50개 단위 페이지 크기 선택과 이전/다음 페이지 네비게이션을 제공해야 한다.

#### M25-4: 클라이언트 사이드 검색/필터

#### REQ-WEB-001-25-04 (Event-Driven)
**WHEN** 사용자가 검색 입력에 텍스트를 입력하는 경우 **THEN** 시스템은 에이전트 이름과 타입을 기준으로 클라이언트 사이드 필터링을 수행하고, `useMemo`로 최적화된 결과를 표시해야 한다.

#### REQ-WEB-001-25-05 (Event-Driven)
**WHEN** 사용자가 상태 필터 뱃지를 클릭하는 경우 **THEN** 시스템은 해당 상태의 에이전트만 필터링하여 목록에 표시하고, 재클릭 시 필터를 해제해야 한다.

### 4.26 Module 30: 대시보드 그리드 시스템 확장 + LGAP/LGCP 스키마 + PropertiesGrid 패널 (P1 - 신규 기능/개선)

#### M30-1: 동적 그리드 컬럼 설정

#### REQ-WEB-001-30-01 (Event-Driven)
**WHEN** 사용자가 대시보드 편집 모드에서 그리드 설정 드롭다운을 통해 그리드 컬럼 수를 변경하는 경우 **THEN** 시스템은 `dashboardGridCols` 상태를 업데이트하고 react-grid-layout의 cols 속성에 반영하여 대시보드 레이아웃을 즉시 재계산해야 한다. 기본값 10, 허용 범위 4-100.

#### M30-2: 그리드 라인 표시 토글

#### REQ-WEB-001-30-02 (Event-Driven)
**WHEN** 사용자가 그리드 라인 표시 토글을 활성화하는 경우 **THEN** 시스템은 `dashboardShowGridLines` 상태를 업데이트하고 대시보드에 그리드 가이드 라인 오버레이를 표시해야 한다.

#### M30-3: 반응형 셀 크기 계산

#### REQ-WEB-001-30-03 (Ubiquitous)
시스템은 **항상** ResizeObserver를 사용하여 대시보드 컨테이너 크기 변경을 감지하고, 현재 그리드 컬럼 수에 기반하여 셀 크기를 자동으로 재계산해야 한다.

#### M30-4: PropertiesGridPanel

#### REQ-WEB-001-30-04 (Event-Driven)
**WHEN** 대시보드에 'properties-grid' 타입 패널이 추가된 경우 **THEN** 시스템은 PropertiesGridPanel 컴포넌트를 렌더링하여 바인딩된 디바이스의 속성을 그리드 레이아웃으로 표시해야 한다. `visibleProperties` 설정으로 표시할 컬럼을 제어할 수 있어야 한다.

#### M30-5: LGAP/LGCP 에이전트 스키마

#### REQ-WEB-001-30-05 (Ubiquitous)
시스템은 **항상** agentSchemas.ts에 LGAP 에이전트 14개 설정 필드(시리얼 포트, 보드레이트, 폴링 간격, 디바이스 목록, 타임아웃, 백오프 등)와 LGCP 에이전트 12개 설정 필드(시리얼 설정, CRC 검증, 자동 디스커버리, 상태 리포팅, 백오프 등)를 정의하고, 에이전트 타입 라벨을 'lgap'(LG LGAP), 'lgcp'(LG LGCP Capture)로 표시해야 한다.

#### M30-6: nodeSchemas.ts 확장

#### REQ-WEB-001-30-06 (Ubiquitous)
시스템은 **항상** nodeSchemas.ts에서 Bridge 에이전트 기본값에 `showPublishTopic` 플래그를 포함하고, logger/mqtt 브릿지에 `publish_topic` 필드를 제공하며, LGAP/LGCP 노드 스키마를 정의해야 한다.

#### M30-7: 17종 패널 타입 확장

#### REQ-WEB-001-30-07 (Ubiquitous)
시스템은 **항상** PanelType 유니언을 17종으로 확장하여 데이터(device, table, properties-grid), 차트(stat, gauge, line-chart, bar-chart, pie-chart), 콘텐츠(text), 제어(ac-control, hvac-control, custom-control) 타입을 지원하고, 각 타입별 `panelDefaultSize` 매핑을 제공해야 한다.

#### M30-8: i18n 확장

#### REQ-WEB-001-30-08 (Ubiquitous)
시스템은 **항상** 80개 이상의 신규 번역 키(패널 카테고리, 패널 타입 라벨, AC/HVAC 제어 라벨, 테마 라벨)를 한국어(ko.json)와 영어(en.json) i18n 파일에 포함해야 한다.

### 4.27 Module 31: 디바이스 페이지 테이블 전환 (P1 - UI 개선)

#### M31-1: 테이블 리스트 레이아웃

#### REQ-WEB-001-31-01 (Event-Driven)
사용자가 디바이스 페이지를 열면, 시스템은 **그리드 카드 레이아웃 대신** AgentListPage와 동일한 패턴의 테이블 리스트 레이아웃을 표시해야 한다. 테이블 컬럼: 확장 아이콘, 이름, 타입, 프로토콜, 상태, 에이전트, 마지막 확인 시각.

#### M31-2: DeviceSearchFilter 컴포넌트

#### REQ-WEB-001-31-02 (Ubiquitous)
시스템은 **항상** DeviceSearchFilter 컴포넌트를 제공하여 이름 검색(텍스트 입력), 상태 필터(온라인/오프라인 뱃지 토글), 프로토콜 필터(NASA/LGAP/Modbus 셀렉트), 타입 필터(실내기/실외기/센서/제어기 셀렉트)를 지원해야 한다.

#### M31-3: AddDeviceDialog 컴포넌트 추출

#### REQ-WEB-001-31-03 (Ubiquitous)
시스템은 **항상** 디바이스 추가 폼을 AddDeviceDialog로 분리하여 NASA 디바이스(주소/디바이스 ID/타입)와 Modbus 디바이스(유닛 ID/이름/레지스터 맵 멀티 블록)의 추가를 지원해야 한다.

#### M31-4: 정렬 및 페이지네이션

#### REQ-WEB-001-31-04 (Event-Driven)
사용자가 컬럼 헤더를 클릭하면, 시스템은 SortableHeader를 통해 해당 컬럼 기준으로 오름차순/내림차순 정렬을 수행해야 한다. 페이지네이션은 10/20/50개 단위를 지원한다.

#### M31-5: 확장 가능 행

#### REQ-WEB-001-31-05 (Event-Driven)
사용자가 디바이스 행의 확장 아이콘을 클릭하면, 시스템은 해당 행 아래에 DeviceDetailPanel을 인라인으로 표시해야 한다.

### 4.28 Module 32: DeviceDetailPanel 좌우 분할 레이아웃 (P1 - UI 개선)

#### M32-1: 2컬럼 레이아웃

#### REQ-WEB-001-32-01 (Ubiquitous)
시스템은 **항상** DeviceDetailPanel을 좌우 2컬럼 그리드(`lg:grid-cols-2`)로 렌더링해야 한다. 좌측에 StatePropertiesSection(상태 속성), 우측에 CommandsSection(제어) + MetadataSection(메타데이터)을 배치한다. `lg` 브레이크포인트 미만에서는 단일 컬럼으로 폴백한다.

#### M32-2: 메타데이터 키/값 테이블

#### REQ-WEB-001-32-02 (Event-Driven)
시스템은 MetadataSection 읽기 모드에서 그리드 카드 레이아웃 대신 `<table>` 기반 키/값 테이블을 표시해야 한다. 행 항목: 위치(MapPin 아이콘), 그룹, 태그(Tag 아이콘), 커스텀 레이블(동적 행). 편집 모드는 기존 폼 레이아웃을 유지한다.

### 4.29 Module 33: NASA Agent 설정 UI 개선 (P1 - UI 개선)

#### M33-1: 설정 2열 레이아웃

#### REQ-WEB-001-33-01 (Ubiquitous)
시스템은 Samsung NASA 에이전트 설정 탭을 **항상** 2컬럼 그리드로 렌더링해야 한다. 좌측 "연결" 컬럼에 transport_type, serial/TCP 관련 필드, 우측 "운영" 컬럼에 상태 확인 요청 간격, 체크박스 필드, 로그 레벨을 배치한다.

#### M33-2: 설정 라벨 개선

#### REQ-WEB-001-33-02 (Ubiquitous)
시스템은 NASA 에이전트 설정 필드 라벨을 다음과 같이 표시해야 한다: "전송 방식"→"연결 방식", "폴링 간격"→"상태 확인 요청 간격". LGAP 에이전트의 transport_type 라벨도 "연결 방식"으로 통일한다.

#### M33-3: 상태 변경 알람 필드 추가

#### REQ-WEB-001-33-03 (Event-Driven)
사용자가 "상태 변경 알람 전송" 체크박스를 활성화하면, 시스템은 notify_on_change 설정을 저장해야 한다.

#### M33-4: 체크박스 라벨 통일

#### REQ-WEB-001-33-04 (Ubiquitous)
시스템은 boolean 타입 필드를 "활성/비활성" 텍스트 대신 체크박스 옆에 필드 라벨을 직접 표시해야 한다. 상단 라벨은 boolean 필드에서 중복 표시하지 않는다.

#### M33-5: 로그 레벨 운영 컬럼 통합

#### REQ-WEB-001-33-05 (Ubiquitous)
시스템은 NASA 에이전트의 로그 레벨 설정을 운영 컬럼 하단에 통합하여 표시해야 한다. 다른 에이전트 타입은 기존 하단 독립 블록을 유지한다.

#### M33-6: Makefile 추가

#### REQ-WEB-001-33-06 (Ubiquitous)
프로젝트 루트에 Makefile을 제공하여 `make`로 프론트엔드+백엔드 전체 빌드, `make web`/`make server` 개별 빌드, `make run`/`make dev`/`make test`/`make lint`/`make clean` 타겟을 지원해야 한다.

### 4.30 Module 38: 디바이스 제어 UI 통일

#### M38-1: NASA 디바이스 GenericPropertiesGrid 전환

#### REQ-WEB-001-38-01 (Ubiquitous)
시스템은 NASA indoor 디바이스의 상태 속성을 `GenericPropertiesGrid` 컴포넌트로 표시해야 한다. 기존 `NasaIndoorRemoteControl` 전용 컴포넌트를 제거하고 LGCP와 동일한 그리드 패턴을 사용한다.

#### M38-2: 속성 표시 순서 통일

#### REQ-WEB-001-38-02 (Ubiquitous)
시스템은 모든 프로토콜(NASA, LGCP, LGAP)의 상태 속성을 다음 순서로 표시해야 한다: 전원(power) → 운전 모드(mode) → 설정 온도(target_temp) → 현재 온도(current_temp) → 풍량(fan_speed) → 고정 설치(swing_vertical, swing_auto, locked, plasma, filter_alarm) → 센서/배관 → 컨트롤러/실외기 → 에러 코드. `PROPERTY_ORDER` 배열에 정의된 순서를 따르며, 목록에 없는 키는 맨 뒤에 원래 순서대로 표시한다.

#### M38-3: 커맨드 표시 순서 통일

#### REQ-WEB-001-38-03 (Ubiquitous)
시스템은 디바이스 커맨드 섹션을 다음 순서로 표시해야 한다: set_power → set_mode → target_temperature → set_fan_speed → set_swing → set_lock → set_plasma. `COMMAND_ORDER` 배열과 `sortCommands()` 함수로 정렬하며, 목록에 없는 커맨드는 맨 뒤에 표시한다.

#### M38-4: LGAP 리모컨 제어 순서 통일

#### REQ-WEB-001-38-04 (Ubiquitous)
시스템은 LGAP 디바이스의 `LgapRemoteControl` 컴포넌트 내부 섹션 순서를 전원 → 운전 모드 → 온도 → 풍량 → 스윙/플라즈마 순으로 배치해야 한다.

#### M38-5: 전원 슬라이드 스위치

#### REQ-WEB-001-38-05 (Ubiquitous)
시스템은 전원(set_power) 커맨드를 슬라이드 스위치(`role="switch"`, `aria-checked`)로 렌더링해야 한다. ON 상태는 녹색(bg-green-500), OFF 상태는 회색(bg-gray-300)으로 표시한다.

#### M38-6: OFF 상태 커맨드 버퍼링

#### REQ-WEB-001-38-06 (Event-Driven)
사용자가 전원 OFF 상태에서 다른 제어(온도, 모드, 풍량 등)를 변경하면, 시스템은 해당 명령을 즉시 전송하지 않고 `pendingChanges` 버퍼에 저장해야 한다. OFF 상태의 제어 영역은 `opacity-60`으로 시각적으로 구분하고, 버퍼된 변경 건수를 배지로 표시한다.

#### M38-7: 전원 ON 시 일괄 적용

#### REQ-WEB-001-38-07 (Event-Driven)
사용자가 전원을 ON으로 전환하면, 시스템은 `set_power` 명령과 함께 `pendingChanges` 버퍼에 저장된 모든 명령을 순차적으로 전송하고 버퍼를 초기화해야 한다.

#### M38-8: Bool 컨트롤 슬라이드 스위치 통일

#### REQ-WEB-001-38-08 (Ubiquitous)
시스템은 boolean 타입 커맨드 파라미터(예: set_lock, set_plasma)를 ON/OFF 버튼 대신 슬라이드 스위치(파란색, bg-blue-500)로 렌더링해야 한다.

#### M38-9: 스피너 레이아웃 시프트 방지

#### REQ-WEB-001-38-09 (Unwanted)
전원 토글 시 로딩 스피너가 스위치 옆에 추가 요소로 렌더링되어 레이아웃이 밀리는 현상이 발생하지 않아야 한다. 스피너는 스위치 thumb 내부에 오버레이로 표시하여 스위치 크기를 유지한다.

---

## 5. Specifications (기술 사양)

### 5.1 Module 1: 에이전트 목록 통계 버그 수정

**근본 원인**: 백엔드 `ListAgents`에서 `detail` 쿼리 파라미터를 `ListOptions`에 전달하지 않고, `agentToHandlerInfo(ag, "")`로 빈 문자열을 하드코딩하여 stats/uptime/health가 항상 생략됨.

**수정 파일 (프론트엔드)**:
- `web/src/services/api/agentService.ts`: `getAgents()` 호출에 `detail: 'summary'` 파라미터 추가

**수정 파일 (백엔드)**:
- `internal/api/dto/request.go`: `ListOptions` 구조체에 `Detail` 필드 추가
- `internal/api/handler/flow.go`: `parseListOptions()`에 `ctx.Query("detail")` 파싱 추가
- `internal/api/service/agent_adapter.go`: `agentToHandlerInfo(ag, "")` → `agentToHandlerInfo(ag, opts.Detail)` 변경

### 5.2 Module 2: 백엔드 API 엔드포인트

| Method | Endpoint | Request Body | Response | 설명 |
|--------|----------|-------------|----------|------|
| GET | `/monitor/loglevel` | - | `{ default_level: string, components: Record<string, string> }` | 전체 로그 레벨 조회 |
| PUT | `/monitor/loglevel` | `{ level: string }` | `{ level: string }` | 글로벌 기본 레벨 변경 (기존) |
| PUT | `/monitor/loglevel/{component}` | `{ level: string }` | `{ component: string, level: string }` | 컴포넌트별 레벨 설정 |
| DELETE | `/monitor/loglevel/{component}` | - | `{ component: string }` | 컴포넌트 레벨 리셋 |

**유효한 level 값**: `debug`, `info`, `warn`, `error`

**component 형식 예시**:
- `agent.modbus-001` - 특정 에이전트
- `node.transform-1` - 특정 노드
- `engine.scheduler` - 엔진 컴포넌트

### 5.3 Module 3: 프론트엔드 API 서비스 확장

**파일**: `web/src/services/api/monitorService.ts` - 확장

```typescript
// 기존 함수 유지
export async function setLogLevel(level: string): Promise<void>;

// 새로 추가
export async function getLogLevels(): Promise<LogLevelInfo>;
export async function setComponentLogLevel(component: string, level: string): Promise<void>;
export async function resetComponentLogLevel(component: string): Promise<void>;
```

**새 타입 정의**:
```typescript
interface LogLevelInfo {
  default_level: string;
  components: Record<string, string>;
}
```

### 5.4 Module 4: 플로우 노드 통계 및 로그 레벨 UI

**신규 파일**: `web/src/pages/flows/FlowDetailPanel.tsx`
- 플로우 확장 패널로 노드 인스턴스 테이블 표시
- 각 노드: 이름, 타입, 상태 배지(NodeStateBadge), **In/Out 메시지 분리 표시(PortStats)**, 로그 레벨 드롭다운(NodeLogLevelSelect)
- In 표시: `ArrowDownToLine` 아이콘 + input 방향 포트 메시지 합계
- Out 표시: `ArrowUpFromLine` 아이콘 + output 방향 포트 메시지 합계
- `useFlowNodes(flowId, refetchInterval)` 훅으로 `GET /flows/{id}/nodes` 호출
- `useFlowStatus(flowId)`로 running 상태 감지 → running 시 3초 자동 갱신
- `node.{name}` 키로 monitor API를 통한 로그 레벨 제어

**수정 파일**: `web/src/pages/flows/FlowListPage.tsx`
- 테이블에 확장 토글 컬럼(ChevronDown/ChevronRight) 추가
- FlowRow 컴포넌트로 확장 가능 행 분리
- 플로우명 클릭 시 에디터 이동, 행 클릭 시 패널 토글

**수정 파일**: `web/src/hooks/useFlow.ts`
- `useFlowNodes(flowId, refetchInterval?)`: 선택적 `refetchInterval` 파라미터 추가로 실시간 갱신 지원

**기존 API 활용**:
- `GET /flows/{id}/nodes`: 노드 인스턴스 목록 (FlowNodeInfo[])
- `GET /flows/{id}/status`: 플로우 상태 조회 (실시간 갱신 조건 판단)
- `GET /monitor/loglevel`: 현재 로그 레벨 조회 (`node.{name}` 키)
- `PUT /monitor/loglevel/node.{name}`: 노드 로그 레벨 설정
- `DELETE /monitor/loglevel/node.{name}`: 노드 로그 레벨 리셋

### 5.5 Module 5: 캔버스 런타임 통계

**신규 파일**: `web/src/contexts/RuntimeStatsContext.ts`
- `NodeRuntimeStats` 인터페이스: `{ inMessages: number, outMessages: number, state: string }`
- `RuntimeStatsContext`: `Record<string, NodeRuntimeStats>`를 제공하는 React Context
- `useNodeRuntimeStats(nodeId)`: 개별 노드의 런타임 통계를 구독하는 커스텀 훅
- editorStore와 완전 분리하여 isDirty/undo/redo에 영향 없음

**수정 파일**: `web/src/pages/editor/EditorPage.tsx`
- `useFlowStatus(flowId)`: 5초 간격 플로우 상태 폴링
- 플로우 running 시 `useFlowNodes(flowId, 3000)`: 3초 간격 노드 데이터 폴링
- `runtimeStatsMap` 구성: 노드별 포트 direction 필터링으로 inMessages/outMessages 계산
- `<RuntimeStatsContext.Provider>` 래핑으로 하위 CustomNode에 통계 전달

**수정 파일**: `web/src/components/flow/CustomNode.tsx`
- `useNodeRuntimeStats(id)` 훅으로 런타임 통계 구독
- 상태 표시등: runtime state에 따라 초록(running)/빨강(error)/회색(기본) 점
- In/Out 표시: `ArrowDownToLine`/`ArrowUpFromLine` 아이콘 + 메시지 수 (노드 라벨 아래)

### 5.6 백엔드 버그 수정

**수정 파일**: `internal/engine/engine.go`
- 포트 카운터 초기화 시 모든 포트(입력/출력/에러)에 대해 `atomic.Int64` 카운터를 명시적으로 0으로 초기화
- 이전 코드: 일부 포트의 카운터가 초기화되지 않아 메시지 카운트가 항상 0으로 보고됨

**수정 파일**: `internal/node/bridge.go`
- `msgCh` 채널 초기화 누락 수정
- `Process()` 메서드의 불필요한 에러 반환 수정 → 정상 처리 시 nil 반환
- BridgeOut 노드의 Process 반환값 수정

**수정 파일**: `internal/node/bridge_test.go`
- 브릿지 노드 수정 사항에 맞는 테스트 케이스 업데이트

### 5.7 Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링

**배경**: SPEC-OBS-004에서 백엔드 `wsLogWriter.Write()`가 `log.entry` 페이로드에 `component`와 `source` 필드를 추가 전송하도록 구현 완료. 프론트엔드 LogViewer는 아직 이 필드를 사용하지 않음.

**수정 파일 1**: `web/src/pages/monitoring/LogViewer.tsx`

- `LogEntry` 인터페이스 확장:
  - `component?: string` 필드 추가 (예: `agent.modbus-001`, `node.transform-1`)
  - `source?: string` 필드 추가 (예: `agent`, `node`, `flow`, `api`, `engine`, `system`)

- `SourceType` 타입 및 `SOURCE_STYLES` 맵 추가:
  - `agent`: 보라색 배지 (`bg-purple-100 text-purple-700`)
  - `node`: 청록색 배지 (`bg-teal-100 text-teal-700`)
  - `flow`: 녹색 배지 (`bg-green-100 text-green-700`)
  - `api`: 주황색 배지 (`bg-orange-100 text-orange-700`)
  - `engine`: 남색 배지 (`bg-indigo-100 text-indigo-700`)
  - `system`: 회색 배지 (`bg-gray-200 text-gray-600`)

- 필터 상태 추가:
  - `sourceFilter: Set<SourceType>` — 활성화된 source 카테고리 (빈 Set = 전체 표시)
  - `componentSearch: string` — 컴포넌트 이름 검색어 (빈 문자열 = 전체 표시)
  - `debouncedSearch: string` — 300ms 디바운스된 검색어

- 필터 로직 (기존 level 필터와 AND 결합):
  1. `filter !== null` → level 필터 적용
  2. `sourceFilter.size > 0` → source 필터 적용
  3. `debouncedSearch !== ''` → component 이름 포함 매칭 (대소문자 무시)

- 툴바 UI 확장:
  - 기존 레벨 필터 버튼 행 유지
  - 레벨 필터 오른쪽에 구분선(|) + source 필터 토글 버튼 6개 추가
  - source 버튼은 다중 선택 가능 (클릭 시 Set에 add/delete 토글)
  - 레벨 필터 행 아래에 컴포넌트 검색 입력 (Search 아이콘 + input)

- 로그 행 표시 확장:
  - 기존: `[timestamp] [LEVEL] message`
  - 변경: `[timestamp] [LEVEL] [source배지] [component명] message`
  - source 배지: SOURCE_STYLES 색상, 레벨 배지보다 작은 폰트
  - component 이름: `text-gray-500` 색상, 고정 너비 (`w-[140px]` truncate)

**수정 파일 2**: `web/src/pages/monitoring/MonitoringPage.tsx`

- `handleLog` 콜백 수정:
  - 기존 타입 캐스팅에 `component?: string`, `source?: string` 추가
  - LogEntry 생성 시 `component: d.component ?? ''`, `source: d.source ?? 'system'` 포함

### 5.8 Module 8: 동적 포트 시스템

**근본 원인**: 세 가지 문제가 복합적으로 발생하여 노드 포트가 설정과 불일치함.

1. **브릿지 노드 방향 무시**: `BRIDGE_DEFAULT_PORTS`가 direction과 무관하게 항상 `[{name:'in', direction:'input'}, {name:'out', direction:'output'}]`를 반환. BridgeIn은 output만, BridgeOut은 input만 표시해야 함.
2. **스위치 노드 정적 포트**: 스위치 노드가 `[in, out]` 고정 포트를 사용하지만, 실제로는 라우트 수에 따라 다수의 출력 포트가 필요함.
3. **설정 변경 시 포트 미갱신**: `getDefaultPorts(nodeType)`가 노드 생성 시점에만 호출되고, 이후 설정(direction, routes) 변경 시 포트가 재계산되지 않음.

**신규 함수**: `web/src/config/nodeSchemas.ts` — `computePortsForNode(nodeType, config)`

```typescript
export function computePortsForNode(nodeType: string, config?: Record<string, unknown>): PortDef[] {
  if (nodeType === 'bridge') {
    const direction = config?.direction as string | undefined;
    switch (direction) {
      case 'in':     return [{ name: 'out', direction: 'output' }];
      case 'out':    return [{ name: 'in', direction: 'input' }];
      case 'inout':
      case 'request_reply':
        return [{ name: 'in', direction: 'input' }, { name: 'out', direction: 'output' }];
      default:
        return [{ name: 'in', direction: 'input' }, { name: 'out', direction: 'output' }];
    }
  }
  if (nodeType === 'switch') {
    const routes = config?.routes as Array<{ name: string }> | undefined;
    const ports: PortDef[] = [{ name: 'in', direction: 'input' }];
    if (routes && routes.length > 0) {
      routes.forEach(r => ports.push({ name: r.name, direction: 'output' }));
      ports.push({ name: 'default', direction: 'output' });
    } else {
      ports.push({ name: 'out', direction: 'output' });
    }
    return ports;
  }
  // 기타 노드: 기존 getDefaultPorts 로직 유지
  return getDefaultPorts(nodeType);
}
```

**수정 파일 1**: `web/src/config/nodeSchemas.ts`
- `computePortsForNode(nodeType, config)` 함수 추가
- 기존 `getDefaultPorts(nodeType)` 함수는 하위 호환성을 위해 유지 (config가 없는 경우 호출됨)
- `BRIDGE_DEFAULT_PORTS` 상수는 deprecated 처리 (computePortsForNode 내부로 로직 이동)

**수정 파일 2**: `web/src/pages/editor/EditorPage.tsx`
- 새 노드 생성 시: `getDefaultPorts(nodeType.type)` → `computePortsForNode(nodeType.type, initialConfig)` 변경
- 노드 설정 변경 콜백: config 변경 시 `computePortsForNode(nodeType, updatedConfig)` 호출하여 `data.ports` 업데이트
- 삭제된 포트에 연결된 엣지 자동 정리 로직 추가

**수정 파일 3**: `pkg/flow/node.go`
- `NewNodeDef()` 팩토리 또는 별도 함수에서 브릿지 direction 파라미터를 확인하여 Inputs/Outputs 포트를 적절히 생성
- BridgeIn: `Outputs = [Port{Name:"out"}]`, `Inputs = []`
- BridgeOut: `Inputs = [Port{Name:"in"}]`, `Outputs = []`
- BridgeInOut/BridgeRequestReply: `Inputs = [Port{Name:"in"}]`, `Outputs = [Port{Name:"out"}]`

**수정 파일 4** (선택): `web/src/components/flow/CustomNode.tsx`
- 포트 렌더링이 `data.ports`의 direction 필터로 동작하므로 기본적으로 변경 불필요
- error 포트 표시가 필요한 경우에만 수정

### 5.9 Module 9: 에러 포트 타입 지원

**근본 원인**: 프론트엔드 포트 타입 시스템이 `'input' | 'output'`만 허용하여 백엔드의 `PortError` direction(`"error"`)을 가진 포트가 무시됨.

1. **PortDef 타입 제한**: `web/src/config/nodeSchemas.ts:7`에서 `direction: 'input' | 'output'`만 정의
2. **NodeTypeDefinition 타입 제한**: `web/src/types/node.ts:47`에서 동일하게 `direction: 'input' | 'output'`만 정의
3. **CustomNode 포트 필터링**: `web/src/components/flow/CustomNode.tsx:61-62`에서 `direction === 'input'`과 `direction === 'output'`만 필터링하여 `direction === 'error'` 포트가 렌더링에서 누락됨
4. **NodeHandle 색상 미지원**: `web/src/components/flow/NodeHandle.tsx`에서 파란색(input)과 초록색(output) 2가지 색상만 지원

**백엔드 참고** (수정 불필요):
- `pkg/flow/node.go:16`: `PortError PortDirection = "error"` — 이미 에러 포트 타입 정의됨
- `pkg/flow/node.go:112`: `WithErrorPort()` — 에러 포트 생성 옵션 존재
- `internal/api/service/flow_adapter.go:737-741`: 에러 포트를 `direction: "error"`로 직렬화하여 프론트엔드로 전달

**수정 파일 1**: `web/src/config/nodeSchemas.ts`
- `PortDef` 인터페이스의 `direction` 타입을 `'input' | 'output'`에서 `'input' | 'output' | 'error'`로 확장
- `computePortsForNode()` 함수에서 에러 포트를 지원하는 노드 타입(예: `WithErrorPort` 옵션이 적용된 노드)의 포트 배열에 에러 포트 정의 포함

**수정 파일 2**: `web/src/types/node.ts`
- `NodeTypeDefinition.ports[].direction` 타입을 `'input' | 'output'`에서 `'input' | 'output' | 'error'`로 확장

**수정 파일 3**: `web/src/components/flow/CustomNode.tsx`
- 기존 포트 필터링에 에러 포트 그룹 추가: `const errorPorts = ports.filter(p => p.direction === 'error')`
- 출력 포트와 에러 포트를 합쳐 `rightPorts` 배열 구성: `const rightPorts = [...outputPorts, ...errorPorts]`
- 에러 포트를 노드 오른쪽(Right position)에 출력 포트 아래로 배치하여 빨간색 핸들로 렌더링
- 다중 핸들 offset 배치: `((i+1)/(total+1))*100%` 공식으로 균등 분배
- 에러 포트가 없는 노드는 기존과 동일하게 렌더링 (하위 호환)

**수정 파일 4**: `web/src/components/flow/NodeHandle.tsx`
- `isError` prop 추가: 에러 포트 여부를 외부에서 전달
- `offset` prop 추가: 다중 핸들 배치 시 `style={{ top: offset }}` 적용
- 에러 포트용 빨간색(`bg-red-500`) 핸들 색상 추가
- 포트 타입별 색상: input=파란색(`bg-blue-500`), output=초록색(`bg-green-500`), error=빨간색(`bg-red-500`)

### 5.10 Module 10: Handle ID 접두사 제거

**배경**: Module 8/9에서 포트 시스템을 개선하면서 Handle ID에 방향 접두사(`in-`, `out-`, `err-`)를 사용하고 있었으나, 포트 direction이 별도 필드로 이미 구분되므로 접두사가 불필요함. 접두사가 프론트엔드/백엔드 간 불일치를 유발하여 와이어 연결이 실패하는 원인이 됨.

**설계 원칙**: 포트 이름(`name`) = Handle ID = Wire Port. 중간 변환 없이 직접 매핑.

**수정 파일 1**: `web/src/components/flow/CustomNode.tsx`
- Input Handle: `id={port.name}` (이전: `id={\`in-${port.name}\`}`)
- Output Handle: `id={port.name}` (이전: `id={\`out-${port.name}\`}`)
- Error Handle: `id={port.name}` (이전: `id={\`err-${port.name}\`}`)

**수정 파일 2**: `internal/api/service/flow_adapter.go`
- `flowToReactFlowConfig()`: Edge 생성 시 `sourceHandle: w.SourcePort`, `targetHandle: w.TargetPort` (이전: `"out-" + w.SourcePort`, `"in-" + w.TargetPort`)
- `normalizeReactFlowDefinition()`: Edge 역변환 시 `source_port: sourceHandle`, `target_port: targetHandle` (이전: `strings.TrimPrefix(sh, "out-")`, `strings.TrimPrefix(th, "in-")`)
- `strings` 패키지 import 제거 (TrimPrefix 미사용)

**수정 파일 3**: `web/src/components/property/PropertyPanel.tsx`
- `Port` 타입: `direction: 'input' | 'output'` → `direction: 'input' | 'output' | 'error'`
- 엣지 정리 로직: `.map((p) => p.name)` (이전: 접두사 포함 Handle ID 비교)

### 5.11 Module 11: 리스트 정렬 기능

**배경**: FlowListPage와 AgentListPage에 정렬 기능이 없어 사용자가 원하는 항목을 찾기 어려움. 백엔드 `ListOptions.Sort` 필드와 프론트엔드 `ListOptions.sort` 타입이 이미 정의되어 있으나 실제 구현되지 않은 상태.

**정렬 파라미터 형식**: `field:direction`
- `field`: 정렬할 필드명 (예: `name`, `status`, `created_at`)
- `direction`: `asc` (오름차순) 또는 `desc` (내림차순)
- 기본값: `name:asc`

**수정 파일 1**: `internal/api/service/flow_adapter.go`
- `ListFlows()`: 상태 필터링 후, 페이지네이션 적용 전에 `opts.Sort` 파싱하여 `sort.Slice` 적용
- 정렬 가능 필드: `name`, `status`, `created_at`, `updated_at`
- `parseSortParam(sort string) (field string, ascending bool)` 유틸리티 함수 추가

**수정 파일 2**: `internal/api/service/agent_adapter.go`
- `ListAgents()`: 상태 필터링 후, 페이지네이션 적용 전에 `opts.Sort` 파싱하여 `sort.Slice` 적용
- 정렬 가능 필드: `name`, `type`, `status`

**수정 파일 3**: `web/src/pages/flows/FlowListPage.tsx`
- 정렬 상태: `useState<{field: string, direction: 'asc'|'desc'}>({field: 'name', direction: 'asc'})`
- 정렬 가능 헤더에 `SortableHeader` 컴포넌트 적용 (이름, 상태, 생성일, 수정일)
- API 호출 시 `sort` 파라미터 포함
- 클라이언트 사이드 정렬이 아닌 서버 사이드 정렬 사용

**수정 파일 4**: `web/src/pages/agents/AgentListPage.tsx`
- 정렬 상태: `useState<{field: string, direction: 'asc'|'desc'}>({field: 'name', direction: 'asc'})`
- 정렬 가능 헤더에 `SortableHeader` 컴포넌트 적용 (이름, 타입, 상태)
- API 호출 시 `sort` 파라미터 포함

**신규 파일**: `web/src/components/common/SortableHeader.tsx`
- Props: `label: string, field: string, currentSort: {field, direction}, onSort: (field) => void`
- 현재 정렬 필드이면 방향 인디케이터 표시 (▲/▼)
- 클릭 시 onSort 콜백 호출
- Tailwind CSS 스타일: 호버 시 커서 변경, 정렬 활성 시 색상 강조

**신규 파일(선택)**: `internal/api/service/sort_util.go`
- `parseSortParam(sort string) (field string, ascending bool)` 공통 정렬 파라미터 파싱 함수
- 유효하지 않은 정렬 파라미터는 기본값(`name:asc`)으로 폴백

### 5.12 Module 12: 대시보드 패널 재구성

**배경**: 대시보드가 4개의 독립 위젯(SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget, ResourceWidget)을 2x2 그리드로 배치하고 있으나, 플로우 관련 정보(상태 요약 + 최근 플로우)가 분리되어 있어 사용성이 떨어짐. 플로우와 에이전트를 각각 통합 패널로 재구성하고, 시스템 리소스는 별도 패널로 유지.

**설계 원칙**:
- FlowPanel과 AgentPanel은 간결한(compact) 리스트를 표시. FlowListPage/AgentListPage의 전체 테이블이 아닌 간소화된 행 사용 (expand/collapse 없음)
- 기존 공용 컴포넌트(FlowStatusBadge, AgentStatusBadge, SortableHeader)를 재사용
- 액션 버튼은 간단한 시작/정지 토글 (FlowActionMenu 드롭다운이 아닌 단일 버튼)
- 리스트는 최대 10행 표시, 초과 시 "더 보기" 링크로 전체 목록 페이지 이동
- 기존 `useFlows()`, `useAgents()` 훅 재사용

**삭제 파일**:
- `web/src/pages/dashboard/widgets/SystemStatusWidget.tsx` — FlowPanel 상단 영역으로 기능 흡수
- `web/src/pages/dashboard/widgets/RecentFlowsWidget.tsx` — FlowPanel 하단 영역으로 기능 흡수
- `web/src/pages/dashboard/widgets/AgentStatusWidget.tsx` — AgentPanel 상단 영역으로 기능 흡수

**유지 파일**:
- `web/src/pages/dashboard/widgets/ResourceWidget.tsx` — ResourcePanel로 유지 또는 소폭 개선 (CPU/메모리 게이지 표시)

**신규 파일 1**: `web/src/pages/dashboard/panels/FlowPanel.tsx`

- **상단: 플로우 상태 요약**
  - `useFlows()` 훅 데이터에서 상태별 카운트 계산: running, stopped, error, stored, loaded
  - 각 상태를 FlowStatusBadge + 건수 형태로 가로 배치 (예: `🟢 Running 3  ⚪ Stopped 2  🔴 Error 1`)
  - 기존 SystemStatusWidget의 상태 카운트 기능을 완전 대체

- **하단: 플로우 리스트 테이블**
  - 컬럼: 이름 | 상태 | 노드 수 | 동작 시간 | 액션
  - `이름`: 클릭 시 `/editor/{flowId}`로 이동 (Link 컴포넌트)
  - `상태`: FlowStatusBadge 컴포넌트
  - `노드 수`: `FlowInfo.node_count` 표시
  - `동작 시간`: `FlowInfo.updated_at` 기반 상대 시간 표시 (`formatDistanceToNow` 또는 유사 유틸). running 플로우는 "활성 시간"으로, stopped 플로우는 "마지막 활동"으로 표시
  - `액션`: running 상태 → 정지(Stop) 버튼, stopped/stored 상태 → 시작(Start) 버튼, error 상태 → 재시작(Restart) 버튼. 단일 아이콘 버튼으로 구현 (Play/Pause 아이콘)
  - 정렬: SortableHeader로 이름 기본 오름차순 정렬. 클라이언트 사이드 정렬 (`useMemo`)
  - 최대 10행 표시. 초과 시 하단에 "더 보기 →" 링크 (`/flows` 이동)
  - 기존 RecentFlowsWidget의 최근 10건 표시를 대체하되, `updated_at` 정렬 대신 이름 정렬을 기본으로 사용

- **데이터 조회**: `useFlows()` 훅으로 전체 플로우 목록 조회 (기존 대시보드와 동일)
- **액션 API**: `flowService.startFlow(id)`, `flowService.stopFlow(id)` 기존 함수 재사용

**신규 파일 2**: `web/src/pages/dashboard/panels/AgentPanel.tsx`

- **상단: 에이전트 상태 요약**
  - 에이전트 데이터에서 상태별 카운트 계산: total, active(running), inactive(stopped/error)
  - 각 상태를 AgentStatusBadge + 건수 형태로 가로 배치
  - 기존 AgentStatusWidget의 total/active/inactive 카운트 기능을 완전 대체

- **하단: 에이전트 리스트 테이블**
  - 컬럼: 이름 | 타입 | 상태 | 업타임 | 메시지 IN/OUT | 액션
  - `이름`: 에이전트 이름 표시
  - `타입`: 에이전트 타입 표시
  - `상태`: AgentStatusBadge 컴포넌트
  - `업타임`: `AgentInfo.uptime` 표시 (없으면 `-`)
  - `메시지 IN/OUT`: `AgentInfo.stats.messages_in / stats.messages_out` 표시 (없으면 `-`)
  - `액션`: running → 정지 버튼, stopped → 시작 버튼. 단일 아이콘 버튼
  - 정렬: SortableHeader로 이름 기본 오름차순 정렬. 클라이언트 사이드 정렬
  - 최대 10행 표시. 초과 시 하단에 "더 보기 →" 링크 (`/agents` 이동)

- **데이터 조회**: `useAgents()` 훅 또는 `agentService.getAgents({ detail: 'summary' })` 호출. `detail=summary`로 stats/uptime 포함
- **액션 API**: 에이전트 시작/정지 기존 API 재사용

**수정 파일**: `web/src/pages/dashboard/DashboardPage.tsx`

- 기존 2x2 그리드 레이아웃 제거
- 3패널 레이아웃으로 변경:
  - 데스크톱 (md breakpoint 이상): `grid grid-cols-2 gap-4` 상단 영역에 FlowPanel + AgentPanel, 하단 전체 너비에 ResourcePanel
  - 모바일 (md 미만): `flex flex-col gap-4` 단일 열 스택
- 기존 위젯 import 제거: SystemStatusWidget, AgentStatusWidget, RecentFlowsWidget
- 신규 패널 import 추가: FlowPanel, AgentPanel
- ResourceWidget은 기존 그대로 유지하거나 ResourcePanel로 래핑

**재사용 컴포넌트**:
- `web/src/components/common/FlowStatusBadge.tsx` — 플로우 상태 배지
- `web/src/components/common/AgentStatusBadge.tsx` — 에이전트 상태 배지 (없으면 신규 생성)
- `web/src/components/common/SortableHeader.tsx` — 정렬 가능 헤더 (Module 11에서 생성)
- `web/src/hooks/useFlows.ts` — 플로우 목록 훅
- `web/src/hooks/useAgents.ts` — 에이전트 목록 훅 (없으면 기존 패턴으로 신규 생성)

**FlowPanel 플로우 액션 상세**:
- `running` → Stop 버튼 (`Pause` 아이콘, `flowService.stopFlow(id)`)
- `stopped` / `stored` / `loaded` → Start 버튼 (`Play` 아이콘, `flowService.startFlow(id)`)
- `error` → Restart 버튼 (`RotateCcw` 아이콘, `flowService.restartFlow(id)` 또는 stop → start)
- 버튼 클릭 시 로딩 스피너 표시, 성공/실패 토스트 알림

**동작 시간(Uptime) 표시 로직**:
- `FlowInfo`에 `uptime` 필드가 없으므로 `updated_at` 기반 상대 시간을 사용
- running 플로우: `updated_at`으로부터의 경과 시간 (예: "2시간 전" → "활성: 2시간")
- stopped 플로우: `updated_at`을 "마지막 활동" 시간으로 표시 (예: "마지막 활동: 1일 전")
- `updated_at`이 없으면 `-` 표시

### 5.13 Module 13: Import/Export 기능

**배경**: 플로우와 에이전트를 JSON/YAML 파일로 내보내기(Export) 및 가져오기(Import) 기능 추가. CLI(`xflowd flow import`/`xflowd agent import`)와 동일한 포맷을 사용하여 웹↔CLI 간 상호 호환성을 보장.

**Export 포맷**:
- 플로우: `{ name: string, description?: string, definition: { nodes: [], wires: [] } }` — 런타임 필드(id, status, stats, created_at, updated_at) 제거
- 에이전트: `{ name: string, type: string, config?: object }` — 런타임 필드(id, status, stats, uptime, health) 제거

**Import 접근법**:
- 클라이언트 사이드 파일 파싱: FileReader API로 파일 읽기, 확장자 기반 JSON/YAML 자동 감지
- 파싱된 데이터를 기존 Create API(`POST /flows`, `POST /agents`)로 전송
- 전용 Upload 엔드포인트 불필요

**스크립트 처리**: 스크립트는 플로우 노드 config에 포함되어 있으므로 플로우 정의의 일부로 자동 Import/Export됨. 독립적인 스크립트 Import/Export는 없음.

**신규 백엔드 엔드포인트**:

| Method | Endpoint | Response | 설명 |
|--------|----------|----------|------|
| GET | `/flows/{id}/export` | `{ name, description?, definition }` | 단일 플로우 Export (런타임 필드 제거) |
| GET | `/flows/export` | `[{ name, description?, definition }, ...]` | 전체 플로우 Export (배열) |

**기존 백엔드 엔드포인트 재사용**:

| Method | Endpoint | 설명 |
|--------|----------|------|
| GET | `/agents/{id}/export` | 단일 에이전트 Export (기존 구현) |
| GET | `/agents/export` | 전체 에이전트 Export (기존 구현) |
| POST | `/flows` | 플로우 생성 (Import 시 사용) |
| POST | `/agents` | 에이전트 생성 (Import 시 사용) |

**수정 파일 (백엔드)**: `internal/api/handler/flow.go`
- `Export()` 메서드 추가: 단일 플로우 Export — 플로우 정보 조회 후 런타임 필드 제거하여 `{ name, description?, definition }` 반환
- `ExportAll()` 메서드 추가: 전체 플로우 Export — 모든 플로우를 Export 형식 배열로 반환
- 라우트 등록: `GET /flows/:id/export`, `GET /flows/export`

**신규 파일 1**: `web/src/lib/utils/download.ts`
- `downloadJSON(data: unknown, filename: string): void` — Blob + URL.createObjectURL로 JSON 파일 다운로드
- Content-Type: `application/json`
- UTF-8 BOM 미포함

**신규 파일 2**: `web/src/lib/utils/importParser.ts`
- `parseImportFile(file: File): Promise<unknown>` — FileReader API + 확장자 기반 JSON.parse / yaml.load 자동 감지
- `validateFlowImport(data: unknown): ValidationResult` — 플로우 필수 필드(`name`, `definition`) 검증
- `validateAgentImport(data: unknown): ValidationResult` — 에이전트 필수 필드(`name`, `type`) 검증
- `ValidationResult`: `{ valid: boolean, errors: string[], items: ImportItem[] }`
- 배열 입력 시 각 항목 개별 검증

**신규 파일 3**: `web/src/components/common/ImportDialog.tsx`
- Props: `open: boolean, onClose: () => void, type: 'flow' | 'agent', onImportSuccess: () => void`
- 파일 선택기(file picker): `.json`, `.yaml`, `.yml` 확장자 필터
- 드래그 앤 드롭 영역: `onDragOver`, `onDrop` 이벤트 핸들러
- 미리보기 영역: 파싱 성공 시 항목별 이름, 타입/설명 표시 + 편집 가능한 이름 필드
- 배열 파일: 리스트 형태로 각 항목 미리보기 + 개별 이름 편집
- 유효성 에러: 빨간색 에러 메시지 표시 + 확인 버튼 비활성화
- 로딩 상태: Import 진행 중 스피너 + 버튼 비활성화
- API 에러: 에러 메시지 표시 + 대화상자 유지 (재시도 가능)
- 성공 시: 토스트 알림 + 대화상자 닫기 + onImportSuccess 콜백 호출

**수정 파일 1**: `web/src/services/api/flowService.ts`
- `exportFlow(id: string): Promise<FlowExport>` — `GET /flows/{id}/export` 호출
- `exportAllFlows(): Promise<FlowExport[]>` — `GET /flows/export` 호출
- `FlowExport` 타입: `{ name: string, description?: string, definition: FlowDefinition }`

**수정 파일 2**: `web/src/services/api/agentService.ts`
- `exportAgent(id: string): Promise<AgentExport>` — `GET /agents/{id}/export` 호출
- `exportAllAgents(): Promise<AgentExport[]>` — `GET /agents/export` 호출
- `AgentExport` 타입: `{ name: string, type: string, config?: Record<string, unknown> }`

**수정 파일 3**: `web/src/pages/flows/FlowActionMenu.tsx`
- "내보내기" (Export) 메뉴 항목 추가: `Download` 아이콘
- 클릭 시 `flowService.exportFlow(id)` 호출 후 `downloadJSON(data, \`${name}.json\`)` 실행

**수정 파일 4**: `web/src/pages/flows/FlowListPage.tsx`
- 툴바에 "가져오기" 버튼 추가: `Upload` 아이콘 + "가져오기" 텍스트
- 툴바에 "전체 내보내기" 버튼 추가: `Download` 아이콘 + "전체 내보내기" 텍스트
- "전체 내보내기" 클릭 시 `flowService.exportAllFlows()` 호출 후 `downloadJSON(data, 'flows.json')` 실행
- "가져오기" 클릭 시 `<ImportDialog type="flow" />` 모달 열기
- Import 성공 시 플로우 목록 쿼리 무효화(invalidate)

**수정 파일 5**: `web/src/pages/agents/AgentListPage.tsx`
- 툴바에 "가져오기" 버튼 추가: `Upload` 아이콘 + "가져오기" 텍스트
- 툴바에 "전체 내보내기" 버튼 추가: `Download` 아이콘 + "전체 내보내기" 텍스트
- "전체 내보내기" 클릭 시 `agentService.exportAllAgents()` 호출 후 `downloadJSON(data, 'agents.json')` 실행
- "가져오기" 클릭 시 `<ImportDialog type="agent" />` 모달 열기
- Import 성공 시 에이전트 목록 쿼리 무효화(invalidate)

**수정 파일 6**: `web/package.json`
- `js-yaml` ^4.1.0 의존성 추가 (YAML 파싱)
- `@types/js-yaml` ^4.0.9 devDependency 추가

**이름 충돌 처리**:
- 자동 덮어쓰기 없음
- ImportDialog 미리보기에서 사용자가 이름을 편집한 후 확인
- API가 중복 이름 에러를 반환하면 ImportDialog에서 에러 표시 + 재시도 가능

### 5.14 UI 변경 요약

| 위치 | 변경 내용 |
|------|----------|
| `AgentDetailPanel.tsx` 통계 탭 | 로그 레벨 드롭다운 추가 (debug/info/warn/error/기본값) |
| `SettingsPage.tsx` 시스템 탭 | 컴포넌트별 오버라이드 목록 테이블 추가 (컴포넌트명, 레벨, 리셋 버튼) |
| `monitorService.ts` | `getLogLevels`, `setComponentLogLevel`, `resetComponentLogLevel` 함수 추가 |
| `agentService.ts` | `getAgents` 호출에 `detail=summary` 파라미터 추가 |
| `FlowDetailPanel.tsx` (신규) | 노드 인스턴스 테이블 + In/Out 분리 표시 + 로그 레벨 드롭다운 + 실시간 갱신 |
| `FlowListPage.tsx` | 확장 가능 행 추가 (FlowDetailPanel 토글) |
| `RuntimeStatsContext.ts` (신규) | 캔버스 런타임 통계 Context + useNodeRuntimeStats 훅 |
| `EditorPage.tsx` | RuntimeStatsContext.Provider 래핑 + 런타임 통계 폴링 |
| `CustomNode.tsx` | 런타임 상태 표시등 + In/Out 메시지 수 표시 |
| `useFlow.ts` | useFlowNodes에 refetchInterval 파라미터 추가 |
| `LogViewer.tsx` | LogEntry에 component/source 필드 추가, source 필터 버튼, 컴포넌트 검색 입력, 행별 source/component 배지 표시 |
| `MonitoringPage.tsx` | handleLog에서 component/source 필드 추출 |
| `nodeSchemas.ts` | `computePortsForNode(nodeType, config)` 함수 추가, 브릿지/스위치 동적 포트 로직 |
| `EditorPage.tsx` | 노드 생성 시 `computePortsForNode` 사용, 설정 변경 시 포트 재계산 + 엣지 정리 |
| `node.go` | `NewNodeDef()` 브릿지 direction 기반 포트 생성 |
| `nodeSchemas.ts` | `PortDef.direction` 타입에 `'error'` 추가 |
| `node.ts` | `NodeTypeDefinition.ports[].direction` 타입에 `'error'` 추가 |
| `CustomNode.tsx` | 에러 포트 필터링 + rightPorts(출력+에러) 합산 + offset 배치 |
| `NodeHandle.tsx` | `isError`/`offset` prop 추가 + 에러 포트 핸들 색상 `bg-red-500` |
| `flow_adapter.go` | Handle↔Edge 매핑에서 접두사(`in-`/`out-`/`err-`) 추가/제거 로직 삭제 |
| `PropertyPanel.tsx` | Port 타입에 `'error'` direction 추가, 엣지 정리 시 포트 이름 직접 비교 |
| `FlowListPage.tsx` | 정렬 가능 컬럼 헤더 추가 (이름, 상태, 생성일, 수정일), sort 파라미터 API 전달 |
| `AgentListPage.tsx` | 정렬 가능 컬럼 헤더 추가 (이름, 타입, 상태), sort 파라미터 API 전달 |
| `SortableHeader.tsx` (신규) | 정렬 방향 인디케이터(▲/▼) + 클릭 토글 공용 컴포넌트 |
| `flow_adapter.go` | ListFlows sort 파라미터 처리 구현, 정렬 후 페이지네이션 |
| `agent_adapter.go` | ListAgents sort 파라미터 처리 구현, 정렬 후 페이지네이션 |
| `DashboardPage.tsx` | 2x2 위젯 그리드 → 3패널 레이아웃(FlowPanel + AgentPanel + ResourceWidget) 재구성, 반응형 그리드 |
| `FlowPanel.tsx` (신규) | 플로우 상태 요약(상태별 건수 배지) + 플로우 리스트 테이블(이름/상태/노드 수/동작 시간/액션) + 정렬 + 더 보기 링크 |
| `AgentPanel.tsx` (신규) | 에이전트 상태 요약(total/active/inactive 배지) + 에이전트 리스트 테이블(이름/타입/상태/업타임/메시지/액션) + 정렬 + 더 보기 링크 |
| `SystemStatusWidget.tsx` | 삭제 (FlowPanel로 기능 흡수) |
| `RecentFlowsWidget.tsx` | 삭제 (FlowPanel로 기능 흡수) |
| `AgentStatusWidget.tsx` | 삭제 (AgentPanel로 기능 흡수) |
| `ResourceWidget.tsx` | 유지 (CPU/메모리 사용률 게이지 + 미니 차트) |
| `download.ts` (신규) | `downloadJSON(data, filename)` 유틸리티 함수. Blob + URL.createObjectURL로 JSON 파일 다운로드 |
| `importParser.ts` (신규) | `parseImportFile(file)`, `validateFlowImport(data)`, `validateAgentImport(data)` 유틸리티 함수 |
| `ImportDialog.tsx` (신규) | Import 공용 모달. 파일 선택 + 드래그 앤 드롭 + 미리보기 + 이름 편집 + 유효성 에러 + 로딩 상태 |
| `flowService.ts` | `exportFlow(id)`, `exportAllFlows()` Export API 호출 함수 추가 |
| `agentService.ts` | `exportAgent(id)`, `exportAllAgents()` Export API 호출 함수 추가 |
| `FlowActionMenu.tsx` | "내보내기" (Export) 메뉴 항목 추가 (Download 아이콘) |
| `FlowListPage.tsx` | "가져오기"/"전체 내보내기" 툴바 버튼 추가 + ImportDialog 연동 |
| `AgentListPage.tsx` | "가져오기"/"전체 내보내기" 툴바 버튼 추가 + ImportDialog 연동 |
| `flow.go` (백엔드) | `Export()`, `ExportAll()` 핸들러 추가 + 라우트 등록 |
| `package.json` | `js-yaml` ^4.1.0 + `@types/js-yaml` ^4.0.9 의존성 추가 |

### 5.15 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC | 의존 내용 |
|-------------|-----------|----------|
| Module 1 | SPEC-API-001 | `GET /agents?detail=summary` 쿼리 파라미터 지원 |
| Module 2 | SPEC-OBS-001 | `observe.LevelManager` 인터페이스 (SetLevel, GetLevel, ListLevels) |
| Module 3 | SPEC-API-001 | 신규 `/monitor/loglevel/{component}` 엔드포인트 |
| Module 4 | SPEC-API-001 | `GET /flows/{id}/nodes`, `/monitor/loglevel/node.{name}` 엔드포인트 |
| Module 7 | SPEC-OBS-004 | WebSocket `log.entry` 페이로드의 `component`/`source` 필드 (백엔드 구현 완료) |
| Module 8 | - | 독립 모듈. `nodeSchemas.ts`, `EditorPage.tsx`, `node.go` 자체 수정으로 완결 |
| Module 9 | - | 독립 모듈. `nodeSchemas.ts`, `node.ts`, `CustomNode.tsx`, `NodeHandle.tsx` 자체 수정으로 완결. 백엔드 수정 불필요 |
| Module 10 | Module 8, 9 | Handle ID 매핑 간소화. Module 8/9에서 도입한 포트 시스템의 Handle ID 규칙 통합 |
| Module 11 | SPEC-API-001 | `GET /flows?sort=field:dir`, `GET /agents?sort=field:dir` 쿼리 파라미터 |
| Module 12 | Module 1, 11 | Module 1의 에이전트 `detail=summary` 수정 + Module 11의 SortableHeader 컴포넌트 재사용. FlowStatusBadge, useFlows() 훅 등 기존 컴포넌트/훅 활용 |
| Module 13 | SPEC-API-001 | `POST /flows`, `POST /agents` Create API (Import 시 사용). 기존 `GET /agents/{id}/export`, `GET /agents/export` 엔드포인트 재사용 |
| Module 13 | - | 신규 플로우 Export API: `GET /flows/{id}/export`, `GET /flows/export`. 백엔드 `flow.go`에 핸들러 추가 |

### 5.17 Module 15: 화면 테마 시스템

#### 5.17.1 CSS Variable 디자인 토큰 체계

**토큰 카테고리 및 네이밍 규칙**:

| 카테고리 | 접두사 | 예시 | 용도 |
|----------|--------|------|------|
| 배경 | `--color-bg-` | `--color-bg-primary`, `--color-bg-secondary`, `--color-bg-surface`, `--color-bg-elevated` | 페이지 배경, 카드, 패널 배경 |
| 텍스트 | `--color-text-` | `--color-text-primary`, `--color-text-secondary`, `--color-text-muted`, `--color-text-inverse` | 제목, 본문, 보조 텍스트 |
| 테두리 | `--color-border-` | `--color-border-default`, `--color-border-subtle`, `--color-border-strong` | 구분선, 카드 테두리 |
| 상태 | `--color-status-` | `--color-status-running`, `--color-status-stopped`, `--color-status-error`, `--color-status-warning` | 상태 배지, 인디케이터 |
| 인터랙션 | `--color-interactive-` | `--color-interactive-primary`, `--color-interactive-hover`, `--color-interactive-active`, `--color-interactive-focus` | 버튼, 링크, 선택 영역 |
| 시맨틱 | `--color-` | `--color-success`, `--color-warning`, `--color-error`, `--color-info` | 피드백 색상 |

**토큰 정의 파일**: `web/src/index.css`

```
@theme {
  /* 레이아웃 (기존) */
  --header-height: 56px;
  --sidebar-width: 240px;
  --sidebar-collapsed-width: 64px;

  /* 시맨틱 컬러 토큰 */
  --color-bg-primary: var(--token-bg-primary);
  --color-bg-secondary: var(--token-bg-secondary);
  --color-bg-surface: var(--token-bg-surface);
  --color-bg-elevated: var(--token-bg-elevated);
  --color-text-primary: var(--token-text-primary);
  --color-text-secondary: var(--token-text-secondary);
  --color-text-muted: var(--token-text-muted);
  --color-border-default: var(--token-border-default);
  --color-border-subtle: var(--token-border-subtle);
  --color-interactive-primary: var(--token-interactive-primary);
  --color-interactive-hover: var(--token-interactive-hover);
  --color-status-running: var(--token-status-running);
  --color-status-stopped: var(--token-status-stopped);
  --color-status-error: var(--token-status-error);
  --color-success: var(--token-success);
  --color-warning: var(--token-warning);
  --color-error: var(--token-error);
  --color-info: var(--token-info);
}
```

**Day 프리셋 토큰 값** (`:root` 또는 `[data-theme="day"]`):

| 토큰 | Day 값 | 출처 (현재 Tailwind 클래스) |
|------|--------|---------------------------|
| `--token-bg-primary` | `#f9fafb` (gray-50) | `bg-gray-50` |
| `--token-bg-secondary` | `#f3f4f6` (gray-100) | `bg-gray-100` |
| `--token-bg-surface` | `#ffffff` (white) | `bg-white` |
| `--token-bg-elevated` | `#ffffff` (white) | `bg-white shadow` |
| `--token-text-primary` | `#111827` (gray-900) | `text-gray-900` |
| `--token-text-secondary` | `#4b5563` (gray-600) | `text-gray-600` |
| `--token-text-muted` | `#9ca3af` (gray-400) | `text-gray-400` |
| `--token-border-default` | `#e5e7eb` (gray-200) | `border-gray-200` |
| `--token-border-subtle` | `#f3f4f6` (gray-100) | `border-gray-100` |
| `--token-interactive-primary` | `#3b82f6` (blue-500) | `bg-blue-500` |
| `--token-interactive-hover` | `#2563eb` (blue-600) | `hover:bg-blue-600` |
| `--token-status-running` | `#10b981` (emerald-500) | `bg-emerald-500` |
| `--token-status-stopped` | `#6b7280` (gray-500) | `bg-gray-500` |
| `--token-status-error` | `#ef4444` (red-500) | `bg-red-500` |

**Night 프리셋 토큰 값** (`[data-theme="night"]`):

| 토큰 | Night 값 | 출처 (현재 `dark:` 클래스) |
|------|----------|--------------------------|
| `--token-bg-primary` | `#111827` (gray-900) | `dark:bg-gray-900` |
| `--token-bg-secondary` | `#1f2937` (gray-800) | `dark:bg-gray-800` |
| `--token-bg-surface` | `#1f2937` (gray-800) | `dark:bg-gray-800` |
| `--token-bg-elevated` | `#374151` (gray-700) | `dark:bg-gray-700` |
| `--token-text-primary` | `#f3f4f6` (gray-100) | `dark:text-gray-100` |
| `--token-text-secondary` | `#d1d5db` (gray-300) | `dark:text-gray-300` |
| `--token-text-muted` | `#6b7280` (gray-500) | `dark:text-gray-500` |
| `--token-border-default` | `#374151` (gray-700) | `dark:border-gray-700` |
| `--token-border-subtle` | `#1f2937` (gray-800) | `dark:border-gray-800` |
| `--token-interactive-primary` | `#3b82f6` (blue-500) | `dark:bg-blue-500` |
| `--token-interactive-hover` | `#60a5fa` (blue-400) | `dark:hover:bg-blue-400` |
| `--token-status-running` | `#10b981` (emerald-500) | 동일 |
| `--token-status-stopped` | `#9ca3af` (gray-400) | `dark:bg-gray-400` |
| `--token-status-error` | `#f87171` (red-400) | `dark:bg-red-400` |

#### 5.17.2 테마 적용 메커니즘

**`data-theme` 속성 기반 전환**:
- `<html data-theme="day">`: Day 프리셋 토큰 적용
- `<html data-theme="night">`: Night 프리셋 토큰 적용
- `<html data-theme="custom">`: 사용자 정의 토큰 적용 (인라인 CSS Variable)
- `<html data-theme="day">` 또는 `<html data-theme="night">` (System 모드): OS 설정에 따라 자동 결정

**기존 `dark` 클래스와의 호환성**: 마이그레이션 과도기에 `data-theme="night"` 설정 시 `<html>` 요소에 `dark` 클래스도 함께 추가하여 아직 마이그레이션되지 않은 `dark:` 클래스가 정상 동작하도록 보장한다. 마이그레이션 완료 후 `dark` 클래스 의존성을 제거한다.

#### 5.17.3 Zustand 스토어 변경

**uiStore.ts 변경 사항**:

```typescript
// 기존 theme 타입 변경
theme: 'system' | 'day' | 'night' | 'custom';  // 기존: 'light' | 'dark' | 'system'

// 커스텀 테마 토큰 저장소 추가
customThemeTokens: Record<string, string>;  // { '--token-bg-primary': '#f9fafb', ... }

// 신규 액션
setCustomThemeTokens: (tokens: Record<string, string>) => void;
resetCustomThemeTokens: () => void;
```

**localStorage 영속화**: `partialize` 함수에 `customThemeTokens` 필드를 추가하여 브라우저 새로고침 시에도 커스텀 테마가 유지되도록 한다.

**마이그레이션**: 기존 localStorage에 저장된 `theme: 'light'`은 `'day'`로, `theme: 'dark'`는 `'night'`로 자동 변환하는 `migrate` 함수를 Zustand `persist` 옵션에 추가한다.

#### 5.17.4 useTheme 훅 변경

**변경 전**: `theme: 'light' | 'dark' | 'system'`, `resolvedTheme: 'light' | 'dark'`
**변경 후**: `theme: 'system' | 'day' | 'night' | 'custom'`, `resolvedTheme: 'day' | 'night' | 'custom'`

**주요 변경**:
- `resolvedTheme` 반환 타입을 `'day' | 'night' | 'custom'`으로 변경. System 모드일 때 OS 설정에 따라 `'day'` 또는 `'night'` 반환
- `<html>` 요소에 `data-theme` 속성을 설정 (기존 `dark` 클래스 토글은 마이그레이션 호환을 위해 유지)
- Custom 테마일 때 `customThemeTokens`를 `<html>` 요소의 인라인 CSS Variable로 적용
- `toggleTheme` 함수 제거 (드롭다운 UI로 대체)

#### 5.17.5 테마 선택 UI

**위치**: Header 컴포넌트의 기존 테마 토글 버튼 영역
**형태**: 클릭 시 드롭다운/팝오버 메뉴 (Popover 또는 커스텀 드롭다운)
**옵션**:

| 옵션 | 아이콘 | 설명 |
|------|--------|------|
| System | `Monitor` (lucide-react) | OS 설정에 따라 자동 전환 |
| Day | `Sun` (lucide-react) | 밝은 테마 |
| Night | `Moon` (lucide-react) | 어두운 테마 |
| Custom | `Palette` (lucide-react) | 사용자 정의 테마 (편집 버튼 포함) |

**동작**: 옵션 선택 시 `useTheme().setTheme(mode)` 호출로 즉시 테마 전환. Custom 옵션의 편집 버튼 클릭 시 테마 에디터 모달 열기.

#### 5.17.6 커스텀 테마 에디터

**형태**: 모달 다이얼로그 (Dialog) 또는 사이드 패널
**구성 요소**:
- 카테고리별 시맨틱 토큰 그룹 (배경, 텍스트, 테두리, 상태, 인터랙션)
- 각 토큰에 대한 컬러 피커 (네이티브 `<input type="color">` + hex 입력 필드)
- 라이브 프리뷰: 편집 중 토큰 값이 `<html>` 요소의 인라인 스타일로 실시간 적용
- 저장(Save) 버튼: `setCustomThemeTokens(tokens)` 호출
- 취소(Cancel) 버튼: 임시 변경 폐기, 이전 테마 복원
- 초기화(Reset) 버튼: Day 프리셋 기본값으로 리셋

**수정 파일**:
- `web/src/components/theme/ThemeEditorModal.tsx` (신규)
- `web/src/components/theme/ColorTokenInput.tsx` (신규)

#### 5.17.7 CSS 마이그레이션 전략

**점진적 마이그레이션 우선순위**:

| 순위 | 대상 | 파일 수 | 접근 방식 |
|------|------|---------|----------|
| 1 | 전역 레이아웃 (body, sidebar, header) | 3-5 | `index.css`, `Sidebar.tsx`, `Header.tsx`의 `bg-*`/`dark:bg-*` → `bg-[var(--color-bg-*)]` |
| 2 | 공통 컴포넌트 (카드, 배지, 버튼) | 5-10 | `StatusBadge`, `FlowStatusBadge` 등의 색상 → 시맨틱 토큰 |
| 3 | 대시보드/패널 | 5-8 | `FlowPanel`, `AgentPanel`, `ResourceWidget` 배경/텍스트 → 토큰 |
| 4 | 리스트/테이블 페이지 | 5-8 | `FlowListPage`, `AgentListPage` 등 테이블 스타일 → 토큰 |
| 5 | 에디터/모니터링 | 5-10 | `EditorPage`, `CustomNode`, `LogViewer` 등 → 토큰 |
| 6 | 폼/모달/다이얼로그 | 5-8 | `DynamicForm`, `ImportDialog` 등 → 토큰 |

**마이그레이션 패턴**:
- Before: `className="bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"`
- After: `className="bg-[var(--color-bg-surface)] text-[var(--color-text-primary)]"`

**호환 전략**: `dark:` 클래스와 CSS Variable을 공존시키되, CSS Variable이 우선 적용되도록 CSS 특이성(specificity)을 관리한다. `[data-theme]` 셀렉터가 `.dark` 셀렉터보다 높은 특이성을 갖도록 정의한다.

#### 5.17.8 변경 파일 요약

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/index.css` | 수정 | @theme 블록에 시맨틱 컬러 토큰 추가, `:root`/`[data-theme]` 셀렉터에 Day/Night 프리셋 정의 |
| `web/src/stores/uiStore.ts` | 수정 | theme 타입 변경, customThemeTokens 상태/액션 추가, persist migrate 함수 추가 |
| `web/src/hooks/useTheme.ts` | 수정 | resolvedTheme 타입 변경, data-theme 속성 설정, 커스텀 토큰 인라인 적용, toggleTheme 제거 |
| `web/src/lib/theme/ThemeProvider.tsx` | 수정 | 필요 시 data-theme 초기화 로직 추가 |
| `web/src/components/layout/Header.tsx` | 수정 | 3-cycle 토글 → 4옵션 드롭다운 UI 교체, 테마 에디터 모달 트리거 |
| `web/src/components/theme/ThemeSelector.tsx` | 신규 | 테마 선택 드롭다운/팝오버 컴포넌트 |
| `web/src/components/theme/ThemeEditorModal.tsx` | 신규 | 커스텀 테마 에디터 모달 (카테고리별 컬러 피커, 라이브 프리뷰, 저장/취소/초기화) |
| `web/src/components/theme/ColorTokenInput.tsx` | 신규 | 개별 토큰 컬러 피커 + hex 입력 컴포넌트 |
| `web/src/lib/theme/tokens.ts` | 신규 | 토큰 정의 상수 (카테고리, 이름, 기본값, Day/Night 프리셋 매핑) |
| 전체 컴포넌트 (M15-6 마이그레이션) | 수정 | `dark:` 클래스 → CSS Variable 시맨틱 토큰 교체 (약 30-40 파일) |

### 5.20 Module 16: 대시보드 커스터마이징 시스템

#### 5.20.1 M16-1: 멀티 대시보드 데이터 모델

**데이터 타입 정의** (`web/src/types/dashboard.ts` 신규 또는 `web/src/types/index.ts` 확장):

```typescript
interface DashboardPageConfig {
  id: string              // UUID (crypto.randomUUID())
  name: string            // 사용자 지정 이름
  isDefault: boolean      // 기본 페이지 여부 (전체에서 정확히 1개만 true)
  panels: PanelConfig[]   // 이 페이지의 패널 목록
  layout: DashboardLayoutItem[]  // react-grid-layout 위치/크기
}

interface PanelConfig {
  id: string              // UUID, react-grid-layout의 key로 사용
  type: PanelType         // 패널 종류
  title: string           // 사용자 편집 가능한 패널 제목
  config: Record<string, any>  // 패널별 개별 설정 (컬럼 가시성, 메트릭 등)
}

type PanelType = 'flows' | 'agents' | 'resource' | 'devices' | 'logs'
```

**uiStore 상태 확장** (`web/src/stores/uiStore.ts` 수정):

- `dashboardPages: DashboardPageConfig[]` — 모든 대시보드 페이지 설정 배열
- `activeDashboardId: string` — 현재 활성 대시보드 ID
- 기존 단일 대시보드 상태(`dashboardLayout`, `dashboardVisibleMetrics`, `flowPanelTitle`, `agentPanelTitle`, `dashboardRefreshInterval`, `dashboardEditMode`)는 `DashboardPageConfig` 내부로 이동
- persist `partialize`에 `dashboardPages`, `activeDashboardId` 포함

**Zustand persist 마이그레이션**:

```typescript
// persist migrate 함수 (version 2 → 3)
migrate: (persistedState: any, version: number) => {
  if (version < 3) {
    // 기존 단일 대시보드 → DashboardPageConfig 변환
    const defaultPage: DashboardPageConfig = {
      id: 'default',
      name: '기본 대시보드',
      isDefault: true,
      panels: [
        { id: 'flow-panel', type: 'flows', title: persistedState.flowPanelTitle || '플로우', config: {} },
        { id: 'agent-panel', type: 'agents', title: persistedState.agentPanelTitle || '에이전트', config: {} },
        { id: 'resource-panel', type: 'resource', title: '시스템 리소스', config: {} },
      ],
      layout: persistedState.dashboardLayout || [
        { i: 'flow-panel', x: 0, y: 0, w: 4, h: 4 },
        { i: 'agent-panel', x: 4, y: 0, w: 4, h: 4 },
        { i: 'resource-panel', x: 8, y: 0, w: 4, h: 4 },
      ],
    }
    return {
      ...persistedState,
      dashboardPages: [defaultPage],
      activeDashboardId: 'default',
    }
  }
  return persistedState
},
version: 3,
```

**CRUD 액션 시그니처**:

| 액션 | 시그니처 | 동작 |
|------|----------|------|
| `addDashboardPage` | `(name: string) => string` | 새 빈 페이지 생성, 생성된 ID 반환, 자동 활성화 |
| `removeDashboardPage` | `(id: string) => void` | 페이지 삭제 (최소 1페이지 유지), 삭제 대상이 활성이면 다른 페이지 활성화 |
| `updateDashboardPage` | `(id: string, updates: Partial<DashboardPageConfig>) => void` | 페이지 부분 업데이트 |
| `setDefaultDashboardPage` | `(id: string) => void` | 기본 페이지 변경 (기존 기본 해제 → 대상 설정) |
| `setActiveDashboard` | `(id: string) => void` | 활성 대시보드 전환 |
| `addPanel` | `(type: PanelType, title?: string) => void` | 활성 페이지에 패널 추가 + 레이아웃 자동 배치 |
| `removePanel` | `(panelId: string) => void` | 활성 페이지에서 패널 및 레이아웃 제거 |
| `updatePanelConfig` | `(panelId: string, config: Partial<PanelConfig>) => void` | 패널 설정 업데이트 |

#### 5.20.2 M16-2: 대시보드 관리 UI

**DashboardToolbar 컴포넌트** (`web/src/components/dashboard/DashboardToolbar.tsx` 신규):

- 위치: `DashboardPage.tsx` 상단, react-grid-layout 위
- 레이아웃: 가로 배치 (좌: 드롭다운+이름, 우: 액션 버튼들)

**구성 요소**:

| 요소 | 컴포넌트 | 동작 |
|------|----------|------|
| 대시보드 선택 드롭다운 | `<Select>` (shadcn/ui) | 전체 페이지 목록 표시, 기본 페이지에 별표 표시, 선택 시 `setActiveDashboard` 호출 |
| 대시보드 이름 | 인라인 편집 텍스트 | 더블클릭 시 `<input>`으로 전환, Enter/blur 시 `updateDashboardPage({ name })` 호출 |
| 추가 버튼 | `<Button>` + `<Dialog>` | 이름 입력 다이얼로그 → `addDashboardPage(name)` 호출 |
| 삭제 버튼 | `<Button>` + `<AlertDialog>` | 삭제 확인 → `removeDashboardPage(id)` 호출. 마지막 페이지일 때 비활성화 |
| 기본 지정 버튼 | `<Button>` | `setDefaultDashboardPage(id)` 호출. 이미 기본이면 비활성화 |
| 편집 모드 토글 | 기존 `dashboardEditMode` 토글 | 기존 편집 모드 유지 |

**DashboardPage.tsx 수정사항**:

- 기존 고정 3패널 렌더링 → `activeDashboardId`에 해당하는 `DashboardPageConfig.panels` 기반 동적 렌더링
- `<ResponsiveGridLayout>` 레이아웃 소스: `activePage.layout`
- 패널 타입에 따른 컴포넌트 매핑:

```typescript
const PANEL_COMPONENTS: Record<PanelType, React.ComponentType<PanelProps>> = {
  flows: FlowPanel,
  agents: AgentPanel,
  resource: ResourceWidget,
  devices: DevicePanel,
  logs: LogPanel,
}
```

**앱 로드 시 기본 페이지 활성화**:

- `DashboardPage` 마운트 시 `activeDashboardId`가 유효하지 않으면 `isDefault=true` 페이지를 찾아 설정
- `isDefault=true` 페이지가 없으면 `dashboardPages[0]`을 활성화

#### 5.20.3 M16-3: 패널 추가/삭제 시스템

**AddPanelDialog 컴포넌트** (`web/src/components/dashboard/AddPanelDialog.tsx` 신규):

- shadcn/ui `<Dialog>` 기반
- 5종 패널 타입을 카드 형태로 표시:

| 패널 타입 | 아이콘 | 이름 | 설명 |
|-----------|--------|------|------|
| `flows` | Activity | 플로우 현황 | 플로우 상태 요약 및 리스트 |
| `agents` | Bot | 에이전트 현황 | 에이전트 상태 요약 및 리스트 |
| `resource` | Cpu | 프로세스 리소스 | CPU/메모리 사용률 |
| `devices` | HardDrive | 디바이스 | 디바이스 상태 및 리스트 |
| `logs` | ScrollText | 로그 | 실시간 로그 스트림 |

**패널 자동 배치 알고리즘**:

```typescript
function findNextPosition(layout: DashboardLayoutItem[], panelWidth: number, panelHeight: number): { x: number, y: number } {
  // 1. 기존 레이아웃에서 최대 y+h 계산
  // 2. 12컬럼 그리드에서 빈 공간 탐색 (좌상단부터)
  // 3. 빈 공간 없으면 최하단에 배치 (x=0, y=maxBottom)
}
```

**패널 기본 크기**:

| 패널 타입 | 기본 너비(w) | 기본 높이(h) | 최소 너비 | 최소 높이 |
|-----------|-------------|-------------|-----------|-----------|
| `flows` | 4 | 4 | 3 | 3 |
| `agents` | 4 | 4 | 3 | 3 |
| `resource` | 4 | 4 | 2 | 3 |
| `devices` | 4 | 3 | 3 | 2 |
| `logs` | 6 | 3 | 3 | 2 |

**편집 모드 패널 삭제 UI**:

- 기존 `dashboardEditMode` 토글 재사용
- 편집 모드 시 각 패널 우상단에 `<button className="absolute top-1 right-1">X</button>` 표시
- 클릭 시 확인 없이 `removePanel(panelId)` 즉시 호출

#### 5.20.4 M16-4: 디바이스/로그 패널 타입

**DevicePanel 컴포넌트** (`web/src/components/dashboard/DevicePanel.tsx` 신규):

- 데이터 소스: `GET /devices` API (React Query)
- 상단: 상태 요약 카드 3개 (전체 수, 온라인 수, 오프라인 수)
- 하단: 디바이스 리스트 테이블

| 컬럼 | 필드 | 설명 |
|------|------|------|
| 이름 | `name` | 디바이스 이름 |
| 타입 | `type` | 디바이스 타입 (mqtt, modbus 등) |
| 상태 | `status` | 온라인/오프라인 배지 |
| 마지막 통신 | `last_seen` | 상대 시간 표시 (예: "3분 전") |

- PanelSettingsDropdown: 제목 편집, 표시 컬럼 선택 체크박스

**LogPanel 컴포넌트** (`web/src/components/dashboard/LogPanel.tsx` 신규):

- 데이터 소스: 기존 WebSocket `log.entry` 이벤트 구독
- 로그 표시: 최신 로그가 상단에 표시되는 역순 스트림
- 필터 바: 소스 필터(agent/node/flow/system 토글) + 레벨 필터(debug/info/warn/error 토글)
- 최대 줄 수: PanelConfig의 `config.maxLines` 설정에 따라 표시 (기본값: 100)
- 자동 스크롤: 새 로그 수신 시 자동 스크롤 (수동 스크롤 시 일시 중지)

| 컬럼 | 필드 | 설명 |
|------|------|------|
| 시각 | `timestamp` | HH:mm:ss 형식 |
| 레벨 | `level` | 색상 배지 (debug=gray, info=blue, warn=yellow, error=red) |
| 소스 | `source` | 소스 카테고리 (기존 classifySource 재사용) |
| 메시지 | `message` | 로그 메시지 본문 (줄바꿈 시 말줄임 + 확장) |

- PanelSettingsDropdown: 제목 편집, 최대 표시 줄 수 설정(50/100/200/500)

**기존 패널 재사용**:

- `FlowPanel` (`flows` 타입): 기존 컴포넌트 그대로 사용. PanelConfig의 `title`을 props로 전달하여 제목 오버라이드
- `AgentPanel` (`agents` 타입): 기존 컴포넌트 그대로 사용. PanelConfig의 `title`을 props로 전달
- `ResourceWidget` (`resource` 타입): 기존 컴포넌트 그대로 사용. PanelConfig의 `title`을 props로 전달

#### 5.20.5 Module 16 파일 변경 요약

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/types/dashboard.ts` (또는 `types/index.ts`) | 신규/수정 | `DashboardPageConfig`, `PanelConfig`, `PanelType` 타입 정의 |
| `web/src/stores/uiStore.ts` | 수정 | `dashboardPages`, `activeDashboardId` 상태 추가, CRUD 액션 추가, persist migrate v3 |
| `web/src/pages/DashboardPage.tsx` | 수정 | 고정 3패널 → 동적 패널 렌더링, DashboardToolbar 추가, 패널 타입별 컴포넌트 매핑 |
| `web/src/components/dashboard/DashboardToolbar.tsx` | 신규 | 대시보드 선택 드롭다운, 추가/삭제/기본지정 버튼, 이름 인라인 편집 |
| `web/src/components/dashboard/AddPanelDialog.tsx` | 신규 | 패널 타입 선택 다이얼로그 (5종 패널 카드) |
| `web/src/components/dashboard/DevicePanel.tsx` | 신규 | 디바이스 상태 요약 카드 + 디바이스 리스트 테이블 |
| `web/src/components/dashboard/LogPanel.tsx` | 신규 | 실시간 로그 스트림 + 소스/레벨 필터 |

### 5.22 Module 17: UI 레이아웃 재구성

#### 5.22.1 M17-1: 대시보드 관리 UI Header 이전

**구현 완료:**
- `Header.tsx`: `isDashboardRoute = location.pathname === '/'` 조건부 렌더링 추가
- 대시보드 라우트: 좌측에 `<select>` 드롭다운(대시보드 목록) + 관리 버튼(Pencil/Plus/Trash2/Star) 표시
- 인라인 이름 편집: `<input>` 필드로 전환, Enter/Blur 확정, Escape 취소
- `CreateDashboardDialog` 렌더링을 Header 내부로 이동
- `DashboardToolbar.tsx` 삭제

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/components/layout/Header.tsx` | 수정 | 대시보드 관리 UI(선택/추가/삭제/이름편집/기본지정) 추가, useUIStore 연동 |
| `web/src/pages/dashboard/DashboardToolbar.tsx` | 삭제 | 기능이 Header.tsx로 완전 이전 |

#### 5.22.2 M17-2: 편집 모드 버튼 인라인 배치

**구현 완료:**
- `DashboardPage.tsx`: 별도 편집 모드 설정 바 제거, "추가"/"초기화" 버튼을 조작 바의 편집 토글 앞에 `{editMode && (<>...</>)}` 패턴으로 인라인 배치
- `DeviceListPage.tsx`: 동일 패턴 적용. "추가"(Plus)/"초기화"(RotateCcw) 버튼을 편집 토글 앞에 인라인 배치

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | 편집 모드 설정 바 제거, 패널 추가/초기화 버튼 조작 바 인라인 이동 |
| `web/src/pages/devices/DeviceListPage.tsx` | 수정 | h2 헤더 제거, 추가/초기화 버튼 편집 모드 전용 인라인 배치, 설명 텍스트 추가 |

#### 5.22.3 M17-4: console-logger 로그 레벨 수정

**구현 완료:**
- `console_logger.go`: `Process` 메서드에서 `a.logger.Debug(...)` → `a.logger.Info(...)` 변경
- 근본 원인: `parseConsoleLoggerConfig`가 기본 레벨을 `slog.LevelInfo`로 설정하므로, `Debug` 레벨 메시지는 핸들러에 의해 필터링되어 출력 파일에 기록되지 않음
- `TestConsoleLoggerAgent_ConfigureNewOutput` 테스트 통과 확인

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/agent/system/console_logger.go` | 수정 | Process 메서드 Debug→Info 로그 레벨 변경 |

#### 5.22.4 Module 17 파일 변경 요약

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/components/layout/Header.tsx` | 수정 | 대시보드 관리 UI(선택/추가/삭제/이름편집/기본지정) 라우트 조건부 렌더링 |
| `web/src/pages/dashboard/DashboardToolbar.tsx` | 삭제 | Header.tsx로 기능 이전 완료 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | DashboardToolbar 제거, 패널 추가/초기화 인라인 배치, AddPanelDialog 추가 |
| `web/src/pages/devices/DeviceListPage.tsx` | 수정 | h2 헤더 제거, 추가/초기화 버튼 편집 모드 인라인, 설명 텍스트 추가 |
| `web/src/stores/uiStore.ts` | 수정 | (M16에서 추가한 상태 유지, deviceAddDialogOpen 제거) |
| `internal/agent/system/console_logger.go` | 수정 | Process Debug→Info 로그 레벨 수정 |

### 5.23 Module 18: 플로우 리스트 개선 + UI 버그 수정

#### 5.23.1 M18-1: LGAP 프로토콜 표시 수정

**구현 완료:**
- `NASADeviceInfo` 구조체에 `Protocol string`과 `ExtraProperties map[string]any` 필드 추가
- `Protocol()`: `info.Protocol` 설정 시 해당 값 반환, 기본값 "nasa"
- `Name()`: 프로토콜 이름을 대문자로 표시 (`strings.ToUpper(a.info.Protocol)`)
- `State()`: `ExtraProperties` 맵 항목을 Properties에 병합
- LGAP provider: `Protocol: "lgap"`, `ExtraProperties`에 7개 LGAP 전용 필드 설정

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/device/adapter/nasa.go` | 수정 | NASADeviceInfo에 Protocol, ExtraProperties 추가. Protocol/Name/State 메서드 수정 |
| `internal/agent/lg/provider.go` | 수정 | Protocol="lgap" 설정, SwingVertical→ExtraProperties 이전 |
| `internal/device/adapter/nasa_test.go` | 수정 | 프로토콜 오버라이드, ExtraProperties 병합 테스트 추가 |

#### 5.23.2 M18-2: 로그 레벨 설정 탭 이전

**구현 완료:**
- `AgentDetailPanel.tsx`의 `StatsTab`에서 로그 레벨 관련 코드 전체 이동 (state, useEffect, handler, JSX)
- `ConfigTab`의 `DynamicForm` 아래에 로그 레벨 `<select>` 드롭다운 배치

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/agents/AgentDetailPanel.tsx` | 수정 | 로그 레벨 UI를 StatsTab→ConfigTab 이동 |

#### 5.23.3 M18-3: 다크 모드 확장 패널 배경 수정

**구현 완료:**
- `bg-gray-50 dark:bg-gray-800/50` → `bg-(--color-bg-sunken)` 교체
- `--color-bg-sunken`: light=#f3f4f6, dark=#030712 (불투명 배경)

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/agents/AgentListPage.tsx` | 수정 | 확장 패널 배경 CSS Variable 적용 |
| `web/src/pages/flows/FlowListPage.tsx` | 수정 | 확장 패널 배경 CSS Variable 적용 |

#### 5.23.4 M18-4/5: 플로우 리스트 업타임 + 자동 시작

**구현 완료:**

**백엔드:**
- `handler.FlowInfo`에 `Uptime string`, `AutoStart bool` 필드 추가
- `dto.FlowUpdateRequest`에 `AutoStart *bool` 필드 추가
- `flowStatusToInfo()`: 엔진 `FlowStatus.Uptime` → `info.Uptime` 변환
- `flowToInfo()`: 메타데이터 `auto_start` → `info.AutoStart` 변환
- `ListFlows()`: 저장소를 먼저 로드하여 배포된 플로우의 `auto_start` 메타데이터 병합
- `GetFlow()`: 저장소에서 `auto_start` 메타데이터 로드
- `UpdateFlow()`: `auto_start` 메타데이터 설정/해제, Definition 재생성 시 메타데이터 이전
- `cmd/xflowd/main.go`: 서버 부팅 시 `auto_start=true` 플로우 자동 배포+시작

**프론트엔드:**
- `FlowInfo` 타입에 `uptime`, `auto_start` 필드 추가
- `FlowUpdateRequest`에 `auto_start` 필드 추가
- `FlowListPage.tsx`: 업타임 컬럼(running 시 표시), 자동시작 토글 스위치 추가
- `colSpan` 7→9 업데이트

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/api/handler/flow.go` | 수정 | FlowInfo에 Uptime, AutoStart 필드 추가 |
| `internal/api/dto/request.go` | 수정 | FlowUpdateRequest에 AutoStart 추가 |
| `internal/api/service/flow_adapter.go` | 수정 | flowToInfo, flowStatusToInfo, ListFlows, GetFlow, UpdateFlow 수정 |
| `cmd/xflowd/main.go` | 수정 | 서버 부팅 시 auto_start 플로우 자동 시작 |
| `web/src/types/flow.ts` | 수정 | FlowInfo, FlowUpdateRequest 타입 확장 |
| `web/src/pages/flows/FlowListPage.tsx` | 수정 | 업타임 컬럼 + 자동시작 토글 추가, colSpan 업데이트 |

### 5.24 Module 19: 플로우 Undeploy API + 대시보드 상태 아이콘

#### 5.24.1 M19-1: 플로우 Undeploy API

**구현 완료:**

**백엔드:**
- `FlowManager` interface에 `UndeployFlow(ctx context.Context, id string) error` 추가
- `POST /flows/{id}/undeploy` route + `Undeploy` handler 추가
- `FlowServiceAdapter.UndeployFlow()`: 엔진에 없으면 nil, running/paused면 에러, 그 외 engine.UndeployFlow 호출
- `EventFlowUndeployed = "flow_undeployed"` 이벤트 상수 + 메시지 추가

**프론트엔드:**
- `flowService.ts`: `undeployFlow(id)` 함수 추가
- `useFlow.ts`: `useUndeployFlow()` hook 추가 (flows, flows/id, flows/id/status invalidation)
- `hooks/index.ts`: barrel export 추가

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/api/handler/flow.go` | 수정 | FlowManager interface에 UndeployFlow 추가, POST /flows/{id}/undeploy route + handler |
| `internal/api/service/flow_adapter.go` | 수정 | UndeployFlow 메서드 구현 |
| `internal/api/ws/event_publisher.go` | 수정 | EventFlowUndeployed 이벤트 추가 |
| `internal/api/handler/flow_test.go` | 수정 | mock 메서드 추가, route count 15→16 |
| `web/src/services/api/flowService.ts` | 수정 | undeployFlow 함수 추가 |
| `web/src/hooks/useFlow.ts` | 수정 | useUndeployFlow hook 추가 |
| `web/src/hooks/index.ts` | 수정 | useUndeployFlow barrel export |

#### 5.24.2 M19-2: FlowActionMenu 상태별 액션 제어

**구현 완료:**
- 기존 `isRunning` boolean → 상태별 세밀한 비활성화 플래그:
  - `canStart`: stored, loaded, stopped
  - `canStop`: running, paused
  - `canRestart`: running, paused
  - `canDeploy`: stored
  - `canUndeploy`: loaded, stopped, error
  - `canDelete`: stored, stopped, error
  - `hasConfig`: flow.config != null (Export 조건)
- Deploy: Cable 아이콘 (purple hover), Undeploy: Unplug 아이콘 (orange hover)
- Export: Download 아이콘, hasConfig 조건 활성화
- Delete: Trash2 아이콘, confirm 다이얼로그

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/flows/FlowActionMenu.tsx` | 수정 | 상태별 canXxx 플래그, Cable/Unplug 아이콘, Undeploy 버튼 추가 |

#### 5.24.3 M19-3: 대시보드 패널 상태 아이콘

**구현 완료:**
- FlowPanel: `FlowStatusBadge` → `STATUS_CONFIG` 아이콘 인라인 렌더링 (Activity/CircleStop/AlertTriangle/FileText/Rocket)
- AgentPanel: `AgentStatusBadge` → Activity(연결됨, green)/CircleStop(해제, gray)/AlertTriangle(오류, red) 아이콘
- DevicePanel: `DeviceStatusBadge` → Wifi(온라인, green)/WifiOff(오프라인, gray) 아이콘
- 모든 아이콘에 title 툴팁 속성 추가

**파일 변경:**
| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/dashboard/panels/FlowPanel.tsx` | 수정 | FlowStatusBadge → STATUS_CONFIG 아이콘 |
| `web/src/pages/dashboard/panels/AgentPanel.tsx` | 수정 | AgentStatusBadge → Activity/CircleStop/AlertTriangle 아이콘 |
| `web/src/pages/dashboard/panels/DevicePanel.tsx` | 수정 | DeviceStatusBadge → Wifi/WifiOff 아이콘 |

### 5.21 크로스-SPEC 의존성 (Module 16)

| 모듈 | 의존성 | 설명 |
|------|--------|------|
| Module 16 M16-1 | Module 12 | uiStore.ts의 기존 대시보드 상태(dashboardLayout, flowPanelTitle, agentPanelTitle 등)를 DashboardPageConfig로 마이그레이션. Module 12가 추가한 FlowPanel/AgentPanel/ResourceWidget을 패널 타입으로 재사용 |
| Module 16 M16-4 | Module 7 | LogPanel이 기존 WebSocket log.entry 이벤트와 classifySource 함수를 재사용. Module 7의 소스 필터링 로직 공유 |
| Module 16 M16-4 | SPEC-DEVICE-001 | DevicePanel이 디바이스 API(`GET /devices`)에 의존. SPEC-DEVICE-001의 API가 선행 구현되어야 함 |
| Module 16 M16-2 | Module 15, 17 | 대시보드 관리 UI가 Module 17에서 Header.tsx로 이전됨. 테마 시스템의 CSS Variable 토큰을 사용하여 스타일링 |

### 5.18 크로스-SPEC 의존성 (Module 15)

| 모듈 | 의존성 | 설명 |
|------|--------|------|
| Module 15 | - | 독립 모듈. 프론트엔드 전용으로 백엔드 수정 불필요. 기존 모듈과 파일 공유 범위: Header.tsx(M12 대시보드와 Header 공유), index.css(전역 스타일), uiStore.ts(M12 대시보드 레이아웃 스토어 공유) |
| Module 15 M15-6 | Module 1-13 | CSS 마이그레이션 시 기존 모듈이 수정한 컴포넌트의 `dark:` 클래스도 대상에 포함. 기존 모듈 완료 후 M15-6 진행 권장 |

### 5.25 Module 20: 에이전트 연결 상태 정확성 개선

**배경**: LGAP 에이전트가 설정 미완료/트랜스포트 미연결 상태에서도 대시보드에서 "연결됨"으로 표시되고, 액션 버튼이 연결 상태와 동일하게 표시되며, 업타임이 계속 증가하는 버그 수정.

**근본 원인**: `connected` 필드가 `info.State == lifecycle.StateRunning`만으로 결정됨. LGAP 에이전트는 Init() 단계에서 즉시 StateRunning으로 전환되고, Start()에서 transport.Open() 실패해도 에러를 반환하지 않고 재연결 루프만 생성.

#### 백엔드 변경

| 파일 | 변경 내용 |
|------|-----------|
| `internal/agent/agent.go` | `TransportChecker` 인터페이스 추가 — `TransportConnected() bool` 메서드 정의 |
| `internal/agent/lg/agent.go` | LGAP 에이전트 `TransportConnected()` 구현 — `transport.Available()` 위임 |
| `internal/agent/samsung/agent.go` | NASA 에이전트 `TransportConnected()` 구현 — `transport.Available()` 위임 |
| `internal/api/service/agent_adapter.go` | `connected` 결정 로직 변경: StateRunning 이후 `TransportChecker` 구현 시 `TransportConnected()` 결과로 오버라이드. `connected=false`이면 uptime 빈 값 |

#### 프론트엔드 변경

| 파일 | 변경 내용 |
|------|-----------|
| `web/src/pages/dashboard/panels/AgentPanel.tsx` | 상태 아이콘·요약·액션 버튼: `agent.connected === true \|\| agent.status === 'running'` → `agent.connected === true` |
| `web/src/pages/agents/AgentActionButtons.tsx` | 버튼 활성화 로직: `agent.status === 'running'` → `agent.connected === true` |
| `web/src/pages/agents/AgentStatusBadge.tsx` | `connected ?? status === 'running'` → `connected === true`, 미사용 `status` prop 제거 |
| `web/src/pages/agents/AgentListPage.tsx` | `AgentStatusBadge` 호출에서 미사용 `status` prop 제거 |

### 5.26 Module 21: 플로우 리스트 상태 아이콘 + 상태 필터링

#### 5.26.1 FlowListPage 상태 아이콘

- **FlowStatusBadge 제거**: import 및 사용 제거
- **STATUS_CONFIG 추가**: FlowPanel과 동일한 상태별 아이콘+색상 매핑
  - running: Activity (text-green-600)
  - stopped: CircleStop (text-gray-600)
  - error: AlertTriangle (text-red-600)
  - stored: FileText (text-blue-600)
  - loaded: Rocket (text-yellow-600)
- **렌더링**: `<span>` + 아이콘 + title 속성으로 접근성 확보

#### 5.26.2 FlowSearchFilter 상태 필터 뱃지

- **select 드롭다운 제거**: 기존 `<select>` 요소 제거
- **뱃지 버튼 추가**: 각 상태별 아이콘+라벨 뱃지 버튼 5개
- **토글 동작**: 클릭 시 해당 상태로 필터링, 재클릭 시 필터 해제 (빈 문자열)
- **활성화 스타일**: 활성화 시 배경색 강조 (예: bg-green-100 text-green-700)
- **비활성화 스타일**: 텍스트 색상만 (hover 시 bg-(--color-bg-elevated))

#### 5.26.3 변경 파일

| 파일 | 변경 | 설명 |
|------|------|------|
| web/src/pages/flows/FlowListPage.tsx | EDIT | FlowStatusBadge→아이콘 렌더링, STATUS_CONFIG 추가 |
| web/src/pages/flows/FlowSearchFilter.tsx | EDIT | select→뱃지 토글 버튼 |

### 5.27 Module 22: 패널 설정 다이얼로그

**구현 완료:**

PanelSettingsDialog 모달 컴포넌트를 신규 생성하여 대시보드 패널별 상세 설정 기능을 제공한다.

#### 5.27.1 PanelSettingsDialog 모달

- **패널 제목 편집**: 인라인 텍스트 입력으로 패널 제목 변경
- **타입별 설정 섹션**: 패널 타입에 따라 해당하는 설정 섹션만 표시
- **저장/닫기**: 설정 변경 사항을 uiStore에 반영하고 다이얼로그 닫기

#### 5.27.2 ColumnsSection (flows/agents 패널)

- **컬럼 가시성 토글**: 체크박스로 표시할 컬럼 선택/해제
- **패널 타입별 컬럼 목록**: flows/agents 패널의 테이블 컬럼 정의에 따라 토글 항목 생성

#### 5.27.3 FilterSection (devices 패널)

- **에이전트 필터**: 에이전트별 디바이스 필터링 조건 설정
- **프로토콜 필터**: 프로토콜별 디바이스 필터링 조건 설정

#### 5.27.4 LogsSection (logs 패널)

- **최대 줄 수**: 로그 패널에 표시할 최대 로그 라인 수 설정
- **레벨 필터**: 표시할 로그 레벨(debug/info/warn/error) 선택
- **소스 필터**: 표시할 로그 소스(agent/node/flow/api/engine/system) 선택

#### 5.27.5 uiStore 통합

- `updatePanelConfig(dashboardId, panelId, config)`: 패널 설정 업데이트 메서드 추가
- `updatePanelTitle(dashboardId, panelId, title)`: 패널 제목 업데이트 메서드 추가

#### 5.27.6 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/dashboard/PanelSettingsDialog.tsx` | 신규 (1217줄) | PanelSettingsDialog 모달, ColumnsSection, FilterSection, LogsSection |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | PanelSettingsDialog 통합, 설정 버튼 추가 |
| `web/src/stores/uiStore.ts` | 수정 | updatePanelConfig, updatePanelTitle 메서드 추가 |

### 5.28 Module 23: 악센트 컬러 시스템

**구현 완료:**

패널 요소별 악센트 컬러 커스터마이징 시스템을 구현하여 대시보드 시각적 개인화를 지원한다.

#### 5.28.1 panelColor + _base 그룹

- **_base 그룹**: 모든 미니프리뷰 최상단에 '전체 색상' 클릭 존 배치 (conic-gradient 무지개 표시)
- **panelColor**: _base 그룹을 통해 패널 전체 기본 색상 설정
- **기존 ColorSection 프리셋 제거**: 미니프리뷰 기반 UX로 대체

#### 5.28.2 accentElements 데이터 구조

- **타입**: `Record<string, string | boolean>` — 그룹 이름을 키, 색상 문자열 또는 활성화 boolean을 값으로 사용
- **저장**: PanelConfig 내부에 accentElements 필드로 저장, Zustand persist를 통해 localStorage에 영속화

#### 5.28.3 AccentGroupControls

- **활성화 토글**: 그룹별 체크박스로 악센트 컬러 활성화/비활성화 (_base 그룹은 체크박스 없이 항상 활성)
- **프리셋 컬러**: 미리 정의된 색상 팔레트에서 선택
- **커스텀 컬러 피커**: 사용자 정의 색상 입력
- **리셋**: 그룹 색상을 기본값으로 초기화

#### 5.28.4 4종 미니프리뷰 컴포넌트

- **NasaMiniPreview**: NASA 리모컨 미니어처 (device 패널). 그룹: indicators, temperature, modes, fan, borders, vane
- **ListMiniPreview**: 리스트 미니어처 (flows/agents/devices 패널). 그룹: header, badges, table
- **ResourceMiniPreview**: 리소스 미니어처 (resource 패널). 그룹: header, cpu, memory, throughput, errorRate
- **LogMiniPreview**: 로그 미니어처 (logs 패널). 그룹: header, levels, timestamp, source

#### 5.28.5 패널 컴포넌트 적용

- **acColor(group) 헬퍼 패턴**: 패널 컴포넌트에서 그룹 이름으로 악센트 컬러를 조회하여 인라인 스타일로 적용
- **FlowPanel**: header/badges/table 그룹 컬러 적용
- **AgentPanel**: header/badges/table 그룹 컬러 적용
- **DevicePanel (리스트)**: header/badges/table 그룹 컬러 적용
- **LogPanel**: header/levels/timestamp/source 그룹 컬러 적용
- **ResourceWidget**: cpu/memory/throughput/errorRate 독립 컬러 (per-metric accentColor)

#### 5.28.6 NASA 리모컨 서브프로퍼티

- **temperature 그룹**: value/label 하위 속성으로 세분화
- **SubPropertyRow 컴포넌트**: 서브프로퍼티별 개별 색상 지정 UI

#### 5.28.7 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/dashboard/PanelSettingsDialog.tsx` | 수정 | GenericAccentSection, DeviceAccentSection, AccentGroupControls, 4종 MiniPreview 추가 |
| `web/src/pages/dashboard/panels/FlowPanel.tsx` | 수정 | accentElements/acColor 적용 |
| `web/src/pages/dashboard/panels/AgentPanel.tsx` | 수정 | accentElements/acColor 적용 |
| `web/src/pages/dashboard/panels/DevicePanel.tsx` | 수정 | accentElements/acColor 적용 |
| `web/src/pages/dashboard/panels/LogPanel.tsx` | 수정 | accentElements/acColor 적용 |
| `web/src/pages/dashboard/widgets/ResourceWidget.tsx` | 수정 | per-metric accentColor (cpu/memory/throughput/errorRate 독립 컬러) |

### 5.29 Module 24: 단일 디바이스 패널 + 패널 추가 개선

**구현 완료:**

개별 디바이스를 대시보드에 단일 패널로 추가하는 기능과 AddPanelDialog 개선을 구현한다.

#### 5.29.1 SingleDevicePanel

- **신규 컴포넌트**: 단일 디바이스의 NASA 리모컨 UI를 대시보드 패널로 표시
- **패널 config**: `{ agentId: string, deviceId: string }` 형태로 표시할 디바이스 지정
- **데이터 조회**: 에이전트 API를 통해 해당 디바이스 정보 조회 및 표시

#### 5.29.2 PanelType 확장

- **'device' 타입 추가**: PanelType 유니언에 `'device'` 추가 → `'flows' | 'agents' | 'resource' | 'devices' | 'logs' | 'device'`
- **기존 'devices'와 구분**: `'devices'`는 디바이스 리스트 패널, `'device'`는 단일 디바이스 패널

#### 5.29.3 addPanelWithConfig API

- **uiStore 메서드 추가**: `addPanelWithConfig(dashboardId, type, config, title)` — config와 title을 지정하여 패널 추가
- **기존 addPanel과 공존**: 기존 addPanel은 기본 설정으로 패널 추가, addPanelWithConfig는 상세 설정 가능

#### 5.29.4 AddPanelDialog 개선

- **패널 타입 선택 UI**: 기존 패널 타입 목록에 'device' (단일 디바이스) 타입 추가
- **디바이스 선택**: device 타입 선택 시 에이전트 선택 콤보박스 → 디바이스 선택 콤보박스 순차 표시
- **패널 제목 자동 생성**: 선택한 디바이스 이름을 기본 패널 제목으로 사용

#### 5.29.5 DashboardPage 통합

- **SingleDevicePanel 렌더링**: 패널 타입이 'device'인 경우 SingleDevicePanel 컴포넌트 렌더링

#### 5.29.6 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/dashboard/panels/SingleDevicePanel.tsx` | 신규 (118줄) | 단일 디바이스 NASA 리모컨 패널 |
| `web/src/pages/dashboard/AddPanelDialog.tsx` | 수정 | 패널 타입 선택 UI, device 타입 에이전트/디바이스 콤보박스 |
| `web/src/stores/uiStore.ts` | 수정 | PanelType에 'device' 추가, addPanelWithConfig 메서드 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | SingleDevicePanel 렌더링 분기 추가 |

### 5.30 Module 25: 에이전트 리스트 개선

**구현 완료:**

에이전트 목록 페이지에 검색, 상태 필터, 페이지네이션, 상태 아이콘을 추가하여 사용성을 개선한다.

#### 5.30.1 AgentSearchFilter

- **신규 컴포넌트**: FlowSearchFilter와 동일한 패턴으로 구현
- **검색 입력**: 에이전트 이름/타입으로 텍스트 검색
- **상태 필터 뱃지**: 연결됨(Activity)/중지(CircleStop)/오류(AlertTriangle) 아이콘+라벨 뱃지 토글 버튼

#### 5.30.2 상태 아이콘

- **AgentStatusBadge 수정**: 텍스트 뱃지에서 아이콘 기반 렌더링으로 변경
- **아이콘 매핑**: connected=true → Activity(green), stopped → CircleStop(gray), error → AlertTriangle(red)
- **접근성**: 각 아이콘에 title 속성으로 상태 레이블 툴팁 제공

#### 5.30.3 페이지네이션

- **페이지 크기 선택**: 10/20/50개 단위 드롭다운
- **페이지 네비게이션**: 이전/다음 페이지 버튼, 현재 페이지/전체 페이지 표시
- **상태 유지**: 검색/필터 변경 시 첫 페이지로 리셋

#### 5.30.4 클라이언트 사이드 검색/필터

- **useMemo 최적화**: 검색어와 상태 필터를 기반으로 필터링된 에이전트 목록을 메모이제이션
- **검색 대상**: 에이전트 이름(name), 타입(type) 필드
- **필터링 로직**: 검색어 포함 매칭 + 상태 필터 AND 조합

#### 5.30.5 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/agents/AgentSearchFilter.tsx` | 신규 (73줄) | 검색 입력 + 상태 필터 뱃지 토글 |
| `web/src/pages/agents/AgentListPage.tsx` | 수정 | AgentSearchFilter 통합, 검색/필터/페이지네이션 로직 |
| `web/src/pages/agents/AgentStatusBadge.tsx` | 수정 | 텍스트 뱃지 → 아이콘 기반 렌더링 |

### 5.31 Module 30: 대시보드 그리드 시스템 확장 + LGAP/LGCP 스키마 + PropertiesGrid 패널

**구현 완료:**

대시보드 그리드 레이아웃의 동적 컬럼 설정, 디바이스 속성 표시를 위한 PropertiesGridPanel, LGAP/LGCP 에이전트 스키마, nodeSchemas 확장, 17종 패널 타입 확장, i18n 신규 번역 키를 구현한다.

#### 5.31.1 대시보드 그리드 시스템 확장

- **dashboardGridCols**: uiStore에 그리드 컬럼 수 상태 추가. 기본값 10, 범위 4-100. react-grid-layout의 `cols` 속성과 연동
- **dashboardShowGridLines**: uiStore에 그리드 라인 표시 토글 상태 추가. 활성화 시 CSS 오버레이로 그리드 가이드 라인 표시
- **ResizeObserver 반응형**: 대시보드 컨테이너에 ResizeObserver를 연결하여 크기 변경 시 셀 크기(rowHeight, margin)를 자동 재계산
- **그리드 설정 드롭다운**: 대시보드 편집 모드 UI에 그리드 컬럼 수 입력과 그리드 라인 표시 토글을 포함하는 설정 드롭다운 제공

#### 5.31.2 PropertiesGridPanel

- **신규 컴포넌트**: 디바이스 속성(properties)을 그리드 레이아웃으로 표시하는 대시보드 패널
- **visibleProperties 설정**: 패널 config에서 표시할 속성 컬럼 목록을 지정하여 선택적으로 표시
- **디바이스 바인딩**: 패널 config의 agentId/deviceId로 표시할 디바이스를 지정
- **PanelType에 'properties-grid' 추가**: 기존 패널 타입 유니언에 새로운 타입 추가

#### 5.31.3 LGAP/LGCP 에이전트 스키마

- **LGAP 에이전트 스키마**: agentSchemas.ts에 14개 설정 필드 정의
  - 시리얼 포트, 보드레이트, 폴링 간격, 디바이스 목록, 타임아웃, 백오프 등
- **LGCP 에이전트 스키마**: agentSchemas.ts에 12개 설정 필드 정의
  - 시리얼 설정, CRC 검증, 자동 디스커버리, 상태 리포팅, 백오프 등
- **에이전트 타입 라벨 업데이트**: deviceLabels.ts 또는 관련 매핑에서 `'lgap'` → "LG LGAP", `'lgcp'` → "LG LGCP Capture"

#### 5.31.4 nodeSchemas.ts 확장

- **Bridge showPublishTopic 플래그**: Bridge 에이전트 기본값에 `showPublishTopic` 불리언 플래그 추가
- **publish_topic 필드**: logger, mqtt 브릿지 노드 스키마에 `publish_topic` 문자열 필드 추가
- **LGAP/LGCP 노드 스키마**: LGAP 및 LGCP 노드 타입의 설정 스키마 정의 추가

#### 5.31.5 17종 패널 타입 확장

- **PanelType 유니언 확장** (기존 5종 → 17종):
  - 데이터: `device`, `table`, `properties-grid`
  - 차트: `stat`, `gauge`, `line-chart`, `bar-chart`, `pie-chart`
  - 콘텐츠: `text`
  - 제어: `ac-control`, `hvac-control`, `custom-control`
- **panelDefaultSize 매핑**: 각 패널 타입별 기본 그리드 크기(w, h)를 정의하는 매핑 객체 추가
- **AddPanelDialog 카테고리화**: 패널 추가 다이얼로그에서 카테고리별(데이터/차트/콘텐츠/제어)로 패널 타입을 분류하여 표시

#### 5.31.6 i18n 확장

- **80+ 신규 번역 키** (ko.json, en.json):
  - 패널 카테고리 라벨 (데이터, 차트, 콘텐츠, 제어)
  - 17종 패널 타입 라벨
  - AC 제어 라벨 (전원, 온도, 모드, 풍량, 스윙 등)
  - HVAC 제어 라벨 (센서, 환기 모드, 온습도 설정, 스케줄 등)
  - 테마 관련 라벨

#### 5.31.7 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/stores/uiStore.ts` | 수정 | dashboardGridCols, dashboardShowGridLines 상태 추가, PanelType 17종 확장, panelDefaultSize 매핑 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 수정 | ResizeObserver 반응형 셀 크기, 그리드 설정 드롭다운, PropertiesGridPanel 렌더링 분기 |
| `web/src/pages/dashboard/panels/PropertiesGridPanel.tsx` | 신규 | 디바이스 속성 그리드 레이아웃 패널 |
| `web/src/config/agentSchemas.ts` | 수정 | LGAP 14개 필드, LGCP 12개 필드 스키마 추가 |
| `web/src/config/nodeSchemas.ts` | 수정 | Bridge showPublishTopic, publish_topic 필드, LGAP/LGCP 노드 스키마 |
| `web/src/lib/utils/deviceLabels.ts` | 수정 | lgap, lgcp 에이전트 타입 라벨 추가 |
| `web/src/pages/dashboard/AddPanelDialog.tsx` | 수정 | 17종 패널 타입 카테고리화, panelDefaultSize 적용 |
| `web/src/lib/i18n/ko.json` | 수정 | 80+ 신규 번역 키 추가 |
| `web/src/lib/i18n/en.json` | 수정 | 80+ 신규 번역 키 추가 |

### 5.32 Module 31: 디바이스 페이지 테이블 전환

**구현 완료:**

디바이스 페이지를 react-grid-layout 기반 그리드 카드 레이아웃에서 AgentListPage 패턴의 테이블 리스트 레이아웃으로 전환한다.

#### 5.32.1 DeviceListPage 테이블 전환

- **레이아웃 변경**: react-grid-layout 카드 그리드 → HTML 테이블 리스트
- **테이블 컬럼**: 확장 아이콘(ChevronRight/Down), 이름, 타입, 프로토콜, 상태(온라인/오프라인 뱃지), 에이전트, 마지막 확인 시각
- **SortableHeader**: AgentListPage와 동일한 정렬 UI 패턴 적용
- **페이지네이션**: 10/20/50개 단위 선택 가능
- **확장 가능 행**: 행 클릭 시 DeviceDetailPanel을 인라인 확장으로 표시

#### 5.32.2 DeviceSearchFilter 컴포넌트

- **신규 컴포넌트**: `web/src/pages/devices/DeviceSearchFilter.tsx`
- **검색 입력**: 텍스트 기반 디바이스 이름 검색
- **상태 뱃지 필터**: 온라인/오프라인 토글 뱃지 (Wifi/WifiOff 아이콘)
- **프로토콜 셀렉트**: NASA/LGAP/Modbus 드롭다운
- **타입 셀렉트**: 실내기/실외기/센서/제어기 드롭다운

#### 5.32.3 AddDeviceDialog 컴포넌트

- **신규 컴포넌트**: `web/src/pages/devices/AddDeviceDialog.tsx`
- **기존 DeviceListPage에서 추출**: NASA 디바이스 추가 폼(주소/디바이스 ID/타입)과 Modbus 디바이스 추가 폼(유닛 ID/이름/레지스터 맵 멀티 블록) 분리
- **에이전트 필터**: useAgents 훅으로 samsung-nasa, modbus-tcp-server 타입 에이전트만 표시

#### 5.32.4 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/devices/DeviceListPage.tsx` | 수정 | 그리드 카드→테이블 리스트 전환, SortableHeader/페이지네이션/확장 행 |
| `web/src/pages/devices/DeviceSearchFilter.tsx` | 신규 | 검색/상태 뱃지/프로토콜·타입 필터 컴포넌트 |
| `web/src/pages/devices/AddDeviceDialog.tsx` | 신규 | NASA/Modbus 디바이스 추가 다이얼로그 (기존 DeviceListPage에서 추출) |

### 5.33 Module 32: DeviceDetailPanel 좌우 분할 레이아웃

**구현 완료:**

DeviceDetailPanel의 레이아웃을 수직 스택에서 좌우 2컬럼으로 전환하고, MetadataSection 읽기 모드를 키/값 테이블로 변경한다.

#### 5.33.1 2컬럼 그리드 레이아웃

- **레이아웃 변경**: `space-y-6` 수직 스택 → `grid grid-cols-1 gap-6 lg:grid-cols-2` 2컬럼 그리드
- **좌측 컬럼**: StatePropertiesSection (상태 속성 - 프로토콜별 표시)
- **우측 컬럼**: CommandsSection (제어) + MetadataSection (메타데이터)
- **반응형**: `lg` 브레이크포인트 미만에서 단일 컬럼 폴백

#### 5.33.2 MetadataSection 키/값 테이블

- **읽기 모드 변경**: 그리드 카드(`grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4`) → `<table>` 기반 키/값 테이블
- **테이블 행**: 위치(MapPin 아이콘), 그룹, 태그(Tag 아이콘), 커스텀 레이블(동적)
- **스타일**: `divide-y divide-(--color-border-default)` 행 구분선, `whitespace-nowrap` 키 컬럼
- **편집 모드**: 기존 폼 레이아웃 유지 (변경 없음)

#### 5.33.3 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/devices/DeviceDetailPanel.tsx` | 수정 | 2컬럼 그리드 레이아웃 + MetadataSection 테이블 전환 |
| `web/vite.config.ts` | 수정 | Vite 프록시 타겟 포트 수정 (8080 → 8081) |

### 5.34 Module 33: NASA Agent 설정 UI 개선

NASA 에이전트 설정 탭을 2열 레이아웃으로 재구성하고, 설정 필드 라벨과 체크박스 렌더링을 개선한다.

#### 5.34.1 NasaConfigLayout 컴포넌트

AgentDetailPanel 내부에 NasaConfigLayout 컴포넌트를 추가하여 NASA 에이전트 전용 2컬럼 레이아웃을 렌더링한다. `NASA_CONNECTION_FIELDS` Set으로 연결/운영 필드를 분류한다. 운영 컬럼 하단에 로그 레벨 select를 통합하여 별도 블록 없이 일관된 레이아웃을 제공한다.

#### 5.34.2 FormField 체크박스 개선

boolean 타입 필드의 렌더링을 변경: 상단 라벨 숨김(중복 방지), 체크박스 옆에 `field.label` 텍스트 표시, "활성/비활성" 텍스트 제거. 모든 에이전트 타입에 공통 적용된다.

#### 5.34.3 agentSchemas.ts 라벨 변경

- `transport_type`: "전송 방식" → "연결 방식" (NASA + LGAP 통일)
- `poll_interval`: "폴링 간격" → "상태 확인 요청 간격"
- `buzzer_on_control`: description 제거 (체크박스 라벨로 충분)
- `notify_on_change`: 신규 boolean 필드 추가 ("상태 변경 알람 전송")

#### 5.34.4 Makefile

프로젝트 루트에 Makefile 추가. 타겟: all(web+server), web(npm build), server(go build), run, dev, test(test-go+test-web), lint(go vet+tsc), clean, help.

#### 5.34.5 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/config/agentSchemas.ts` | 수정 | NASA/LGAP 라벨 변경, notify_on_change 필드 추가 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | 수정 | NasaConfigLayout 2열 컴포넌트, 로그 레벨 통합, 안내 문구 갱신 |
| `web/src/components/property/FormField.tsx` | 수정 | boolean 필드 라벨 렌더링 변경 |
| `Makefile` | 신규 | 프론트엔드+백엔드 통합 빌드 시스템 |

### 5.35 Module 38: 디바이스 제어 UI 통일

#### 5.35.1 NASA 디바이스 GenericPropertiesGrid 전환

`DeviceDetailPanel.tsx`의 `StatePropertiesSection`에서 NASA indoor 분기를 제거. 기존에 `protocol === 'nasa' && type === 'indoor'`일 때 `NasaIndoorRemoteControl`로 라우팅하던 코드를 삭제하여 모든 NASA 디바이스가 `GenericPropertiesGrid`를 사용하도록 통일. `NasaIndoorRemoteControl` 컴포넌트(~220줄)와 관련 상수(`FAN_LABELS`) 완전 삭제.

#### 5.35.2 속성/커맨드 표시 순서 통일

`deviceLabels.ts`의 `PROPERTY_ORDER` 배열 확장: swing_vertical, swing_auto, locked, plasma, filter_alarm, fan_motor_hz, valve_open, pipe_temp1_c, pipe_temp2_c, pipe_in_temp, pipe_out_temp, zone_load, zone_power, compressor_cap, compressor_hz, compressor_run, outdoor_active, heat_demand, refrigerant_on, op_mode, error_code 추가. 신규 `COMMAND_ORDER` 배열과 `sortCommands()` 함수 추가로 커맨드 섹션도 동일 순서 적용.

#### 5.35.3 LGAP 리모컨 섹션 순서

`LgapRemoteControl` 내부 JSX 섹션 순서를 전원 → 운전 모드 → 온도 → 풍량 → 스윙/플라즈마로 재배치. 기존에 온도가 운전 모드 앞에 있던 순서를 수정.

#### 5.35.4 CommandsSection 전원 인식 아키텍처

`CommandsSection`에 `powerState` prop 추가. 내부 로직:
- `set_power` 커맨드를 분리하여 슬라이드 스위치로 렌더링 (녹색 토글)
- `pendingChanges` 상태(useState)로 OFF 시 변경사항 버퍼링
- `handlePowerToggle`: OFF→ON 시 power 명령 + 버퍼 일괄 전송, ON→OFF 시 power 명령만 전송 + 버퍼 초기화
- `handleExecute`: ON 시 즉시 전송, OFF 시 버퍼에 저장
- OFF 상태 제어 영역에 `opacity-60` 적용, 버퍼 건수 배지 표시

#### 5.35.5 슬라이드 스위치 패턴

전원 스위치: `role="switch"` + `aria-checked` + `translate-x-5`/`translate-x-0` CSS transition. Bool 커맨드(BoolCommandControl): ON/OFF 버튼 → 파란색 슬라이드 스위치(bg-blue-500)로 변경. 스피너 레이아웃 시프트 방지: `executeMutation.isPending` 시 thumb 대신 `Loader2` 아이콘을 스위치 내부에 오버레이 렌더링.

#### 5.35.6 변경 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `web/src/pages/devices/DeviceDetailPanel.tsx` | 수정 | NasaIndoorRemoteControl 제거, CommandsSection 전원 인식 재작성, BoolCommandControl 슬라이드 스위치, 스피너 thumb 오버레이 |
| `web/src/lib/utils/deviceLabels.ts` | 수정 | PROPERTY_ORDER 확장, COMMAND_ORDER/sortCommands() 신규 추가 |

### 5.19 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: 에이전트 목록 통계 버그 수정 | 사용자에게 보이는 기존 기능 장애 |
| P1 (중요) | Module 2: 백엔드 로그 레벨 API | Module 3, 4의 전제 조건 |
| P1 (중요) | Module 3: 에이전트 로그 레벨 UI | 디버깅 효율성 향상 |
| P1 (중요) | Module 4: 노드 통계 + 로그 레벨 UI | 노드 단위 디버깅 지원 |
| P1 (중요) | Module 5: 캔버스 런타임 통계 | 에디터 캔버스 실시간 모니터링 |
| P0 (즉시) | 백엔드 버그 수정: 포트 카운터/브릿지 | 메시지 카운트 정확도 보장 |
| P1 (중요) | Module 7: 로그 뷰어 컴포넌트/소스 필터링 | 로그 디버깅 효율성 향상, SPEC-OBS-004 프론트엔드 연동 |
| P0 (즉시) | Module 8: 동적 포트 시스템 | 노드 포트가 설정과 불일치하는 사용자 가시 버그 수정 |
| P0 (즉시) | Module 9: 에러 포트 타입 지원 | 백엔드 에러 포트가 프론트엔드에서 무시되는 사용자 가시 버그 수정 |
| P0 (즉시) | Module 10: Handle ID 접두사 제거 | Handle ID 불일치로 와이어 연결 실패 방지, 코드 간소화 |
| P1 (중요) | Module 11: 리스트 정렬 기능 | 사용자 편의성 향상, 대량 리스트 탐색 효율화 |
| P1 (중요) | Module 12: 대시보드 패널 재구성 | 대시보드 정보 구조 개선, 플로우/에이전트 운영 효율화, 직접 액션 지원 |
| P2 (개선) | Module 13: Import/Export 기능 | 플로우/에이전트 포터빌리티 향상, CLI↔웹 상호 운용성 확보, 백업/복원 편의 |
| P1 (중요) | Module 15: 화면 테마 시스템 | UI 일관성 및 사용자 개인화 향상, CSS 유지보수성 개선, 885개 dark: 클래스 체계적 관리 |
| P1 (중요) | Module 16: 대시보드 커스터마이징 시스템 | 대시보드 사용자 개인화, 멀티 페이지 구성으로 운영 유연성 향상, 디바이스/로그 패널로 모니터링 범위 확대 |
| P1 (중요) | Module 17: UI 레이아웃 재구성 | 페이지 관리 기능의 레이아웃 계층 구조 정리, 대시보드/디바이스 편집 UI 패턴 통일 |
| P1 (중요) | Module 19: 플로우 Undeploy API + 대시보드 상태 아이콘 | 플로우 라이프사이클 완전성(배포 해제 기능), 상태별 액션 제어로 UX 개선, 대시보드 정보 밀도 향상 |
| P1 (중요) | Module 20: 에이전트 연결 상태 정확성 개선 | 에이전트 연결 상태 정확성 확보, 트랜스포트 미연결 시 올바른 상태·액션·업타임 표시 |
| P1 (중요) | Module 21: 플로우 리스트 상태 아이콘 + 상태 필터링 | 대시보드와 플로우 리스트 상태 표시 일관성 확보, 상태 필터 UX 개선 |
| P2 (개선) | Module 22: 패널 설정 다이얼로그 | 패널별 상세 설정(컬럼 가시성, 디바이스 필터, 로그 설정)으로 대시보드 커스터마이징 심화 |
| P2 (개선) | Module 23: 악센트 컬러 시스템 | 패널 요소별 시각적 개인화, 미니프리뷰 기반 직관적 컬러 설정 UX |
| P2 (개선) | Module 24: 단일 디바이스 패널 + 패널 추가 개선 | 개별 디바이스 모니터링 패널, 대시보드 구성 유연성 향상 |
| P2 (개선) | Module 25: 에이전트 리스트 개선 | 에이전트 목록 검색/필터/페이지네이션으로 대량 에이전트 관리 효율화 |
| P1 (중요) | Module 30: 대시보드 그리드 시스템 확장 + LGAP/LGCP 스키마 + PropertiesGrid 패널 | 대시보드 레이아웃 유연성 확대(동적 그리드 컬럼), LGAP/LGCP 프로토콜 지원 확장, 디바이스 속성 모니터링 패널 추가, 17종 패널 타입으로 대시보드 표현력 강화 |
| P1 (중요) | Module 31: 디바이스 페이지 테이블 전환 | 그리드 카드→테이블 리스트 전환으로 AgentListPage 패턴 통일, 검색/필터/정렬/페이지네이션으로 디바이스 관리 효율화 |
| P1 (중요) | Module 32: DeviceDetailPanel 좌우 분할 | 상태/제어 영역 분리로 정보 가독성 향상, 메타데이터 키/값 테이블로 공간 효율화 |
| P1 (중요) | Module 33: NASA Agent 설정 UI 개선 | 설정 2열 레이아웃으로 가독성 향상, 라벨 명확화, 체크박스 UX 통일, Makefile 빌드 시스템 |
| P1 (중요) | Module 38: 디바이스 제어 UI 통일 | NASA/LGCP/LGAP 제어 순서 통일, 전원 슬라이드 스위치 + OFF 상태 버퍼링, Bool 스위치 통일, 스피너 레이아웃 안정화 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.29.0*
*상태: completed*
*최종 수정: 2026-04-16*
