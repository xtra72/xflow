---
id: SPEC-REMOTE-001
title: "원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링"
version: "1.6.0"
status: planned
created: "2026-06-05"
updated: "2026-06-08"
author: "xtra"
priority: high
related_specs:
  - SPEC-AUTH-001
  - SPEC-AUTH-003
  - SPEC-WEB-001
  - SPEC-FLOW-001
  - SPEC-AGENT-001
  - SPEC-DEVICE-001
  - SPEC-DEVICE-IDENTITY-001
  - SPEC-CFG-001
  - SPEC-SOCKET-001
  - SPEC-MQTT-003
  - SPEC-DASHBOARD-001
  - SPEC-CHART-001
tags:
  - remote-management
  - fleet
  - websocket
  - registration
  - approval
  - remote-control
  - inventory-mirror
  - instance-identity
  - jwt
  - server-client
  - query-proxy
  - unified-ui
  - full-parity
  - node-grouping
  - node-dashboard
  - information-architecture
  - remote-dashboard
  - dashboard-parity
  - chart-stream-proxy
  - panel-target-awareness
  - manager-view
  - node-resolution
  - top-bar-layout
  - fixed-canvas
  - letterbox-scaling
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-05 | xtra | 초기 SPEC 작성 — 원격 관리 서버/클라이언트(fleet 관리). 전송=클라이언트가 서버로 dial 하는 영속 WebSocket, 제어=라이브 연결 위 RPC 명령(로컬 어댑터 적용), 인벤토리=클라이언트 미러링+서버 DB 캐시, 노출 제어=클라이언트 config opt-in. 신규 xflow 인스턴스 식별자(instance_id) 도입. M1~M5 마일스톤 정의 |
| 1.1.0 | 2026-06-06 | xtra | v1.1 확장 — 수동 enrollment(그룹 H). (A) 사전 등록(instance_id allow-list): 접속 전 approved 노드 사전 생성 + 접속 시 자동 승인. (B) enrollment 토큰: 관리자가 발급한 1회성/횟수·기간 제한 가입 토큰을 register 에 운반해 관리자 수동 승인 없이 자동 승인. 토큰은 SHA-256 해시로만 저장(원본 1회 노출), 즉시 폐기 가능. 모든 신규 REST 엔드포인트 admin-gated, 자동 승인 감사 기록 |
| 1.2.0 | 2026-06-06 | xtra | v1.2 확장 — 원격 자원 편집(그룹 I). 관리자가 서버 웹 UI 에서 승인·온라인 노드의 플로우/에이전트를 FULL CRUD(생성·수정·삭제). 기존 시각 편집기(EditorPage.tsx React Flow) 재사용. 모든 편집은 그룹 D 명령 경유 → 노드 어댑터 로컬 적용 → 결과 수신 후에만 미러 캐시 갱신(서버 단독 영속 금지 — A4/E08). 시크릿 redaction 라운드트립 보존(마스킹 필드 생략·노드 측 병합), 실패 의미(offline=503/timeout=504/apply-fail=502). §1.5 비목표의 "서버 측 직접 영속 편집" 항목 대체. 마일스톤 M7 추가. (2026-06-06 RESOLVED §5.9-6~9: 시크릿=필드 부재+노드 backfill, 동시성=노드 권위+비차단 경고(잠금 없음), 신규 ID=노드 채번, 신규 생성 자원=수동 노출(opt-in 보존)) |
| 1.5.0 | 2026-06-08 | xtra | v1.5 확장 — 원격 노드 대시보드 패리티(그룹 L). 관리자가 서버에서 승인·온라인 원격 노드의 대시보드를 **로컬과 동일한 DashboardPage**(`target=remote:{instanceId}`)로 보고 제어한다(M8 query/스트림 프록시 + target 추상화 재사용). (A) **대시보드 CONFIG 읽기 프록시** — 노드의 대시보드 설정(dashboardPages[]/activeDashboardId/grid/refresh/deviceGridLayout, SPEC-DASHBOARD-001 `GET /dashboards/{shared,mine}`)을 그룹 J query-action(`dashboard` 도메인 `get_shared`/`get_mine`)으로 **READ-ONLY** 취득(원격 config **편집은 v1.5 비목표**, 차기 위임). (B) **패널 데이터 소스 target-aware** — flows/agents/devices/single-device 패널은 기존 target 훅(useResourceTargets/useDetailTargets) 재사용, resource(메트릭)·logs·CHART 를 신규 query-action/스트림으로 원격화: 신규 `monitor/metrics` query-action, 신규 `monitor/logs` 스트림 action, **CHART 는 M8 스트림 프록시에 `chart` stream-action(subscribe/stream_data) 추가**(`/ws/chart/{channel}` 직결 대신 서버 경유 중계, 폴링 폴백·teardown·백프레셔 J08/J08b 재사용). control 패널(ac/hvac/outdoor)은 useDeviceDetailTarget + **그룹 D 명령**으로 디바이스 쓰기. 패널은 dashboard 레벨 TargetProvider/prop 으로 target 수신(target 없으면 로컬 렌더 바이트 동일). (C) **디바이스/자원 ref 네임스페이싱** — 원격 target 하에서 패널 device ref 는 **그 노드 기준** 해석(대시보드 config 는 **노드-로컬**: 노드별 fetch, deviceId 는 그 노드의 것 — 중앙 cross-node config 대신 v1 단순성 채택). (D) **UI** — `노드 대시보드`(NodeDashboard, M9)에 **대시보드 서브탭** 추가 및/또는 DashboardPage `target=remote:{id}` 지원, 게이팅(승인+온라인+노출)·redaction·실패 의미(503/504/502/404)·read-only-by-default·원격 컨텍스트 표시. 마일스톤 M10 추가. (OQ-L1~L6 ✅ RESOLVED 2026-06-08 — 사용자 권고안대로 확정: 노드-로컬 read-only config / CHART=M8 스트림 프록시 chart-action 확장 / 원격 config READ-ONLY(편집 차기) / control 쓰기=그룹 D 재사용 / 대시보드 서브탭(노드 대시보드) / 메트릭=query-action·로그=스트림) |
| 1.6.0 | 2026-06-08 | xtra | v1.6 확장 — 관리자 뷰(Manager View)·노드 화면 충실 재현(그룹 M). xflowd 서버 모드 구조는 **유지**하되(별도 관리자 앱/바이너리/백엔드 분리/별도 SPA 없음 — M7~M10 백엔드·컴포넌트 전부 재사용), 원격 관리 UI 를 노드 화면을 충실히 재현하는 **전용 관리자 뷰**로 재구성한다. (A) **노드 해상도 보고** — 노드가 자신의 장비 모니터(키오스크/터치스크린) 해상도(width×height)를 보고(기본: 노드 config `display.resolution`/`display.width`+`display.height` — xflowd 는 헤드리스 데몬이므로 운영자가 장비 화면 해상도를 선언, M9 register/heartbeat 시스템 정보 페이로드로 운반, 하위 호환 — 미보고 시 관리자 뷰가 합리적 기본값/컨테이너 크기로 폴백), 서버가 `managed_nodes` 에 저장·노드 상세에 노출, 관리자 뷰가 소비. (B) **관리자 뷰 레이아웃** — 노드 선택 시 관리 크롬(노드 선택기=좌측 디렉토리를 대체하는 노드 피커 드롭다운(M9 그룹 구조 유지)·서브탭 네비 개요/플로우/에이전트/디바이스/대시보드·관리 액션·나가기)을 **상단 수평 바**로 옮기고, 노드 화면이 그 아래 **전체 너비/높이**를 점유한다(좌우 폭 축소/왜곡 방지). 전역 좌측 사이드바는 이 뷰에서 숨김/접힘. M9 NodeManagementPage(좌측 디렉토리) → 상단 바 관리자 뷰로 **진화**(디렉토리/그룹 선택은 상단 노드 피커로 이전, 서브탭은 상단 바로 이전). 등록 관리는 그대로. (C) **해상도 충실 대시보드 렌더** — RemoteDashboardView(그룹 L) 가 노드 보고 해상도에 맞춘 **고정 캔버스**에 렌더 후 가용 영역에 종횡비 보존 스케일-투-핏(레터박스, 패널 리플로우 없음) → 노드 장비에서 보이는 그대로의 왜곡 없는 복제. **변경 경로 불변**(그룹 D/I). 마일스톤 M11 추가. (OQ-M1~M5 ✅ 전부 RESOLVED 2026-06-08 — 사용자 권고안대로 확정: 노드 config 선언 해상도(폴백=기본값/컨테이너) / 대시보드 전용 고정 캔버스 / 그룹 묶음 드롭다운 노드 피커 / fit·레터박스 스케일(수동 줌 차기) / NodeManagementPage 제자리 진화) |
| 1.4.0 | 2026-06-07 | xtra | v1.4 확장 — 원격 관리 UI 정보 구조 재편 + 노드 그룹핑 + 노드 대시보드(그룹 K). 사이드바 `원격 관리` 그룹을 **두 개의 최상위 진입점**(`노드 관리`(운영) + `등록 관리`(온보딩))으로 재편하여 기존 `관리 노드`(RemoteNodesPage) + `원격 노드 제어`(RemoteControlPage) 를 **대체**한다. (A) 노드 그룹핑: `managed_nodes` 에 **단일 그룹 라벨**(group_name, 기본 빈값→"전체"(All) 기본 버킷) 추가 — 한 노드는 **최대 하나의 그룹**에만 속하고, 그룹은 자유 입력 단일 레벨 라벨이며, 그룹 배정/해제·distinct 그룹 목록·그룹별 노드 목록(전체 기본 버킷 포함) API + admin-gated 영속. 그룹 이름 변경=노드 재라벨링, 그룹 비움/삭제=노드를 "전체"로 환원. (B) 노드 시스템 정보 보고(BASIC): 노드가 OS+arch+version+started_at(epoch ms, uptime 산출용)을 register/heartbeat 페이로드로 보고(자원 메트릭 CPU/메모리/디스크 **제외**), 서버가 managed_nodes 에 저장·노드 상세/목록 API 로 노출(하위 호환 — 미보고 노드는 필드 빈값). (C) 노드별 운영 요약: 기존 미러(그룹 E)에서 플로우/에이전트/디바이스 카운트+상태 분해를 **파생**(신규 쿼리 우선이 아니라 미러 데이터 우선). (D) UI 재구성: `노드 관리`=디렉토리 뷰(단일 레벨 그룹 트리, "전체" 기본)+노드 선택→**노드 대시보드**(시스템 정보 BASIC + 운영 요약 + Flow/Agent/Device 서브탭이 기존 M8 통합 제어(`target=remote:{instanceId}`) 재사용); `등록 관리`=토큰 관리(EnrollmentTokenSection)+노드 등록 관리(pending 승인 큐·승인/거부/폐기·사전 등록). 마일스톤 M9 추가. (OQ-K1~K7 ✅ RESOLVED 2026-06-07 — 사용자 권고안대로 확정: 서버 전용 그룹 / register+heartbeat 시스템 정보 / 미러 파생 운영 요약 / started_at 서버 파생 uptime / 빈 라벨=가상 "전체" 버킷 / 사전 등록·승인 UI 등록 관리 흡수 / /admin/remote/control 리다이렉트) |
| 1.7.0 | 2026-06-21 | xtra | v1.7 확장 — 원격 프로그램 버전 관리(릴리스 호스팅 + 업데이트 소스 + 아키텍처-aware 그룹 일괄 업데이트)(그룹 O). 관리 서버가 노드용 프로그램 이미지(아키텍처별 `xflowd` 바이너리 + Ed25519 `.sig`)를 호스팅하고, 노드가 무변경 updater 로 소비하는 **익명 GitHub-Releases 호환 피드**(`GET /api/v1/updates/releases/latest`·`/releases`·`/releases/download/{version}/{filename}`, asset `browser_download_url` https 강제)를 제공한다. (A) **릴리스 저장소** — SQLite 메타(`releases`/`release_assets`) + 디스크 `{data}/releases/{version}/`, SHA256 자동 계산·`checksum.txt` 자동 생성. admin 관리 API(`GET/POST /remote/releases`·`DELETE /remote/releases/{version}`·`DELETE …/assets/{os}/{arch}`·multipart 업로드 `POST …/{version}/assets`). 설정 `remote_management.public_base_url`(또는 요청 Host 유도)·`remote_management.releases_dir`. (B) **업데이트 소스 서버 저장** — `update_url`+채널을 SettingsRepository(`remote.update_source`)에 1회 저장(필요시만 변경), `GET/PUT /remote/update-source`(admin), 원격 업데이트 명령(UpdateNode/UpdateGroup)에 서버가 자동 주입. `update_url` 은 빈 값 또는 `https://` 만 허용, **공개키는 절대 전송하지 않음**(노드 로컬 신뢰 앵커). (C) **아키텍처/OS-aware 그룹 일괄 업데이트** — 서버가 각 노드의 보고된 OS/Arch 로 per-node 타깃 버전을 계산해 per-node `system/update` 디스패치, 전략 3종(`latest`=채널 내 각 os/arch 최고 semver / `pin`=단일 버전(기존 호환) / `per_arch`=(os/arch)→버전 맵), 자산 없는 노드는 사유와 함께 건너뜀(부분 성공). (D) **client WS insecure_skip_verify** — `remote_management.insecure_skip_verify`(기본 false), 관리 WS(wss) TLS 인증서 검증 스킵(자체 서명/사설망 전용 옵트인, 명령 무결성은 별도 보장). 마일스톤 M12 추가. (보안 불변식: 서버는 사전 서명 `.sig` 만 저장, https 강제, 공개키 노드 로컬, insecure_skip_verify 는 전송 무결성과 무관 — 자가 업데이트는 Ed25519 로 보장.) |
| 1.3.0 | 2026-06-06 | xtra | v1.3 확장 — 원격 노드 FULL 제어 패리티(그룹 J). 원격 노드의 플로우/에이전트/디바이스를 **별도 원격 페이지가 아니라 로컬과 동일한 웹 UI**(FlowListPage/AgentListPage/DeviceListPage + 상세 패널)로 제어한다. 목록·라이프사이클을 넘어 **상세 패널까지 FULL 패리티**(에이전트 통계/설정/디바이스/토픽/store/세션/시리즈, 디바이스 실시간 상태+명령+메타데이터, 플로우 노드 레벨 런타임/로그). 이를 위해 미러(그룹 E)·명령(그룹 D)과 구분되는 **신규 READ/QUERY 프록시**를 신설: 그룹 D 명령과 **대칭되는 per-domain query-action**(`{domain, query_action, args}`)을 `query`/`query_result` 로 운반하고 노드가 각 action 을 로컬 read 핸들러로 매핑해 라이브 JSON 반환(READ-ONLY — 변경은 그룹 D/M7 유지). 실시간 데이터(디바이스 상태·에이전트 라이브 통계/시리즈)는 **스트리밍 프록시**(subscribe/stream_data/unsubscribe, 서버 경유 중계, teardown·백프레셔)로 제공(폴링은 폴백). 서버는 query-action 응답을 **단기 TTL 캐시**(스트리밍/라이브 action 캐시 우회, 변경 시 무효화). UI 는 `useEditorFlowTarget` 의 target 추상화를 목록/제어 페이지로 확장(useFlowsTarget/useAgentsTarget/useDevicesTarget), `?target=remote:{instanceId}` 쿼리 파라미터 + 노드 셀렉터로 동일 페이지 재사용. 기존 RemoteResourcesPage 는 노드 셀렉터로 재용도화/폐기. 마일스톤 M8 추가. (OQ-J1~J7 RESOLVED: per-domain query-action / FULL 커버리지 / 스트리밍 포함 / 디바이스 쓰기=그룹 D / 최소 감사 / 단기 TTL 캐시 / 서버 admin 게이팅) |

> **상태(Status)** — `planned`. 본 SPEC 은 PLAN 단계 산출물이며 구현 코드를 포함하지 않는다. `/moai run SPEC-REMOTE-001` 로 마일스톤 단위 증분 구현한다.

# SPEC-REMOTE-001: 원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링

## 1. 개요

### 1.1 목적

하나의 xflow 인스턴스가 **원격 관리 서버(management server)** 로 동작하여, xflow 가 설치된 다른 장비들(**관리 대상 노드, managed node**)을 등록·승인·원격 제어하는 fleet 관리 기능을 제공한다. 각 관리 대상 노드는 **원격 관리 클라이언트(management client)** 기능을 가진다.

- 클라이언트는 설정에 따라 관리 서버에 접속하고, 최초에 **등록 요청(registration request)** 을 보낸다.
- 서버는 요청이 들어온 클라이언트의 **등록 여부를 결정**(관리자 승인/거부)한다.
- 등록된(승인된) 노드는 서버에서 원격으로 설정 변경 등이 가능하다. 서버는 그 노드의 **플로우·에이전트·디바이스를 직접 제어·변경**할 수 있다.
- 설정에 따라 노드의 플로우·에이전트·디바이스가 서버의 목록에 나오도록 할 수 있으며, 이때 **어느 노드와 연관된 것인지(source device tagging)** 표시한다.

### 1.2 핵심 모델 (확정 — 반드시 준수)

> **중요**: 본 SPEC 은 아래 4개 확정 아키텍처 결정을 따른다. 이 결정들은 사용자와 합의된 **고정 제약**이며 재논의하지 않는다.

#### 결정 1 — 전송/연결 방향 = 클라이언트가 서버로 dial 하는 영속 WebSocket

클라이언트(관리 대상 노드)가 서버로 **영속 WebSocket** 연결을 건다(NAT/방화벽 친화적 — 클라이언트는 outbound 만 필요). 기존 gorilla/websocket Hub/Client/Message 인프라를 재사용한다. 서버는 이 채널로 명령을 push 하고, 클라이언트는 같은 채널로 텔레메트리/등록/인벤토리를 push 한다. **웹 UI 모니터링 WS 와 관리 WS 는 별도 엔드포인트**이다(혼용 금지).

#### 결정 2 — 원격 제어 = 라이브 연결 위 RPC 명령 → 로컬 어댑터 적용

서버는 라이브 연결 위로 RPC 스타일 **명령(command)** 을 보낸다. 클라이언트는 명령을 받아 **자신의 기존 서비스 어댑터**(`FlowServiceAdapter`, `AgentServiceAdapter`, device 핸들러)를 호출하여 로컬에 적용하고 결과/ack 를 반환한다. 역터널(reverse tunnel)도, 공유 DB 도 없다.

#### 결정 3 — 자원 목록/동기화 = 클라이언트 인벤토리 미러링 + 서버 DB 캐시

클라이언트는 접속 시 + 변경 시 자신의 자원 인벤토리(플로우/에이전트/디바이스)를 서버로 **미러링**한다. 서버는 이를 **DB 에 캐시**하여, 노드가 오프라인일 때도 마지막 알려진 상태(last-known)를 목록에 표시한다. 서버 측 편집은 (온라인) 노드로 디스패치되는 명령이 된다. 미러링된 모든 항목은 **출처 노드(source device)** 로 태깅된다(UI 에 노드 연관 표시).

#### 결정 4 — 노출 제어 = 클라이언트(노드) config 가 결정

어떤 플로우/에이전트/디바이스를 서버에 공유/노출할지는 **클라이언트(노드) 설정이 결정**한다(노드 측 opt-in 프라이버시). 서버는 노드가 노출하기로 한 것만 본다.

### 1.3 배경 (재사용할 기존 인프라)

본 기능은 greenfield(기존 remote/cluster/federation 코드 없음)이나, 다음 기존 인프라를 **재사용**한다(재발명 금지).

- **WebSocket**: `internal/api/ws/{hub.go,client.go,message.go,broadcaster.go,event_publisher.go}` — `Hub`(Register/Unregister/Broadcast/BroadcastMessage), `Client`(ReadPump/WritePump, ping 54s/pong 60s), `Message{Type string, Payload json.RawMessage, Timestamp string}`, `NewMessage(msgType, payload)`. 본 관리 채널의 프레이밍은 이 `Message` 봉투를 확장하여 사용한다.
- **인증**: `internal/auth/{jwt.go,credentials.go}` — `JWTService.GenerateTokens(username, role)`(HS256)·`ValidateToken`, SQLite credentials, `EnsureDefaultAdmin`. 노드 인증 토큰(device credential)은 이 `JWTService` 를 재사용해 발급한다.
- **WS 인증 핸들러**: `internal/api/handler/websocket.go` — Bearer 헤더 또는 `?token=` 쿼리 파라미터. 관리 WS 핸드셰이크 인증도 동일 패턴을 따른다(단, 별도 엔드포인트).
- **서비스 어댑터(원격 명령 대상)**: `internal/api/service/{flow_adapter.go,agent_adapter.go,node_adapter.go}` — 플로우 Create/Get/List/Update/Delete/Deploy/Start/Stop/Pause, 에이전트 List/Get/Create/Update/Delete/Start/Stop/Restart, 디바이스 핸들러 `internal/api/handler/device.go`. `FlowServiceAdapter` 는 `engine *engine.Engine`·`repo storage.FlowRepository` 를 보유한다.
- **Config**: `internal/config/{config.go,types.go}` — viper, `ServerConfig`, `OnChange(key, fn)`/`WatchConfig()` 핫리로드. 신규 `remote_management` 섹션을 추가한다(mode: server|client|disabled, server_url, instance_id, auto_register, heartbeat_interval, exposure 설정).
- **HTTP**: `internal/api/{router.go,server.go,middleware.go}` — net/http 커스텀 `Router`, `RegisterRoutes`/`RegisterRawHandler`. 배선 지점: `cmd/xflowd/main.go`(~700–800).
- **Storage**: `internal/storage/{repository.go,sqlite.go,factory.go}` — `FlowRepository`(Save/Get/List/Delete/Close) 패턴. 서버 측 `managed_nodes`/`mirrored_flows`/`mirrored_agents`/`mirrored_devices` 테이블 + `ManagedNodeRepository` 를 추가한다.
- **dial+재연결+프레이밍 참조**: `internal/agent/socket/`, MQTT 에이전트(paho auto-reconnect/backoff), HVAC TCP 에이전트의 재연결/백오프 패턴.
- **인스턴스 식별자 선례**: 현재 전역 xflow 인스턴스 식별자는 **없다**(IoT 디바이스 단위 UUID 만 `DeviceIDRepository` 로 존재). 본 SPEC 이 영속 `instance_id` 를 신설한다(Section 5.2).

### 1.4 용어 (Glossary) — managed node vs IoT device

용어 혼동을 막기 위해 본 SPEC 은 다음을 엄격히 구분한다.

| 용어 | 영문 | 정의 |
|------|------|------|
| 관리 대상 노드 / 노드 | managed node | xflow **설치본(인스턴스)** 하나. 관리 서버에 등록·관리되는 단위. `instance_id` 로 식별. |
| 관리 서버 | management server | fleet 을 등록·승인·제어하는 xflow 인스턴스(mode=server). |
| 관리 클라이언트 | management client | 서버에 접속·등록되는 노드 측 기능(mode=client). |
| 디바이스 / IoT 디바이스 | IoT device | 플로우/에이전트 내부의 IoT 장치 엔티티(예: Modbus/HVAC 유닛). 노드와 다른 개념. 기존 `DeviceIDRepository` UUID 로 식별. |
| 인스턴스 식별자 | instance_id | 노드(xflow 설치본)를 식별하는 영속 UUID. 본 SPEC 신설. |
| 노드 토큰 / 디바이스 크레덴셜 | node token / device credential | 승인된 노드가 재접속 인증에 쓰는 JWT. `JWTService` 로 발급. |
| 자원 타깃 | resource target | (v1.3) 웹 UI 가 자원을 조회·제어할 대상. `local`(서버 자신) 또는 `remote:{instance_id}`(원격 노드). 로컬 페이지를 동일 코드로 양쪽에 파라미터화하는 추상화(예: `useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget`, `useEditorFlowTarget` 패턴 확장). |
| READ/QUERY 프록시 | query proxy | (v1.3) 서버가 노드의 화이트리스트된 로컬 read 엔드포인트를 WS 세션 위로 프록시해 노드의 라이브 JSON 을 반환하는 READ-ONLY RPC. 미러(그룹 E)·명령(그룹 D)과 별개. |
| 노드 그룹 | node group | (v1.4) 관리 노드를 조직화하는 **단일 레벨 자유 입력 라벨**. 한 노드는 **최대 하나의 그룹**에 속한다(다중 소속 없음). 그룹 미지정 노드는 기본 버킷 **"전체"(All)** 에 표시된다. 그룹은 별도 엔티티가 아니라 `managed_nodes.group_name` 컬럼의 distinct 값 집합이다(그룹 이름 변경=구성원 재라벨링, 그룹 비움=구성원이 "전체"로 환원). |
| 노드 대시보드 | node dashboard | (v1.4) `노드 관리`에서 노드를 선택하면 표시되는 노드 단위 종합 화면. **시스템 정보(BASIC)** + **운영 요약** + **Flow/Agent/Device 서브탭**(M8 통합 제어 `target=remote:{instanceId}` 재사용)으로 구성된다. |
| 시스템 정보(BASIC) | system info (basic) | (v1.4) 노드가 보고하는 기본 시스템 메타: hostname, OS, arch, version, started_at(uptime 산출), online/last_seen, status. **자원 메트릭(CPU/메모리/디스크)은 본 마일스톤 제외.** |
| 운영 요약 | operational summary | (v1.4) 노드의 플로우/에이전트/디바이스 카운트 + 상태 분해(플로우 running/stopped, 에이전트 connected, 디바이스 online)를 **기존 미러(그룹 E)에서 파생**한 요약. |
| 원격 대시보드 | remote dashboard | (v1.5) 관리자가 서버에서 승인·온라인 원격 노드의 대시보드를 **로컬과 동일한 `DashboardPage`** 로 `target=remote:{instanceId}` 파라미터화하여 보고 제어하는 화면. M8 query/스트림 프록시 + target 추상화를 재사용한다. v1.5 는 **READ-ONLY**(원격 대시보드 config 편집은 비목표). |
| 대시보드 config | dashboard config | (v1.5) SPEC-DASHBOARD-001 의 인스턴스별 서버 영속 대시보드 설정(`dashboardPages[]`, `activeDashboardId`, grid cols, refresh interval, `deviceGridLayout`). `GET/PUT/DELETE /dashboards/{shared,mine}`(global+user 스코프). v1.5 원격 패리티는 노드의 대시보드 config 를 **노드-로컬**(노드별 fetch)로 READ-ONLY 취득하며, 그 안의 deviceId 는 해당 노드 기준으로 해석한다. |
| 패널 target 인식 | panel target-awareness | (v1.5) 대시보드 패널이 dashboard 레벨 `target`(local | remote:{instanceId})을 입력으로 받아 각 데이터 소스(디바이스 상태/명령, 에이전트/플로우, 자원 메트릭, 로그, 차트 스트림)를 로컬 vs 원격(프록시)으로 해석하는 추상화. `target` 미지정 시 로컬 렌더는 바이트 동일. |
| 차트 스트림 프록시 | chart stream proxy | (v1.5) 노드의 `/ws/chart/{channel}` 라이브 차트 데이터를 **별도 WS 경로 신설 없이** M8 스트림 프록시에 `chart` stream-action(subscribe/stream_data/unsubscribe)을 추가하여 서버 경유로 브라우저에 중계하는 메커니즘. teardown·백프레셔(J08b)·폴링 폴백을 재사용한다. |
| 관리자 뷰 | manager view | (v1.6) 원격 노드의 화면을 **충실히 재현**하기 위해 재구성된 전용 원격 관리 화면. 노드 선택 시 관리 크롬(노드 피커·서브탭 네비·관리 액션·나가기)은 **상단 수평 바**에, 선택 노드의 화면은 그 아래 **전체 너비/높이**에 배치하며, 전역 좌측 사이드바는 숨김/접힘된다. **별도 관리자 앱/바이너리/백엔드 분리/별도 SPA 가 아니라**, xflowd 서버 모드 안에서 M7~M10 백엔드·컴포넌트를 재사용해 기존 `노드 관리`(NodeManagementPage, M9)를 **상단 바 레이아웃으로 진화**시킨 것이다. |
| 노드 해상도 | node resolution | (v1.6) 노드의 **장비 모니터(키오스크/터치스크린) 디스플레이 해상도**(width×height, px). xflowd 는 헤드리스 데몬이므로 런타임 자동 감지 대상이 없어, 운영자가 노드 config 로 선언한 장비 화면 해상도를 1차 출처로 한다(`display.resolution` 또는 `display.width`+`display.height`). M9 시스템 정보 페이로드로 register/heartbeat 시 운반되며, 서버가 저장·노드 상세로 노출한다. 미보고 시 관리자 뷰가 합리적 기본값/컨테이너 크기로 폴백한다(하위 호환). 노드의 IoT 디바이스 해상도와 무관하다. |
| 노드 피커 | node picker | (v1.6) 관리자 뷰 **상단 바**에서 관리 대상 노드를 선택하는 컨트롤. M9 좌측 디렉토리(그룹 트리)를 대체하되 **그룹 구조(REQ-K01~K05)는 보존**한다(권고: 그룹별로 묶인 드롭다운). 노드 선택 시 화면이 선택 노드로 전환된다. |
| 고정 캔버스 | fixed canvas | (v1.6) 원격 노드의 대시보드를 노드 보고 해상도(width×height)와 동일한 **고정 픽셀 크기 컨테이너**에 렌더하고, 가용 영역에 **종횡비 보존 스케일-투-핏(레터박스)** 으로 맞추는 렌더 방식. 패널이 반응형으로 리플로우되지 않아(고정 그리드) 노드 장비에서 보이는 레이아웃을 왜곡 없이 복제한다. CSS transform scale + 레터박스 여백으로 구현하며, 패널 컴포넌트 코드는 분기하지 않는다(REQ-L09/A16 일관). |

> "device" 라는 단어는 **IoT 디바이스** 에만 사용한다. xflow 설치본은 항상 "managed node / 노드" 로 부른다. 명령군에서 "디바이스 메타데이터 제어"는 IoT 디바이스를 의미한다.

### 1.5 범위

**포함:**
- 모드/설정: server|client|disabled 모드, server_url, 영속 instance_id 생성, 노출(exposure) 설정, heartbeat 간격.
- 연결 라이프사이클: 클라이언트 dial, 인증 핸드셰이크, heartbeat, 백오프 자동 재연결, 서버의 노드별 online/offline 추적, 정상 disconnect.
- 등록/승인: 등록 요청, 서버 pending 큐, 관리자 승인/거부(API + 최소 UI), 토큰 발급, 재접속 시 재인증, 폐기(revocation).
- 원격 제어: 서버가 명령 디스패치(플로우 CRUD+deploy/start/stop/pause, 에이전트 CRUD+start/stop/restart, 디바이스 메타데이터), 클라이언트가 로컬 어댑터로 적용, ack/result/error 반환, 명령 타임아웃·실패 처리, 권한(서버 관리자만 명령, 승인 노드만 수신).
- 인벤토리 미러링/목록: 접속 시 스냅샷 + 변경 시 델타 push, 서버 DB 캐시, 노드 및 그 플로우/에이전트/디바이스를 출처 노드 태그와 함께 목록화, 오프라인 last-known 표시, 클라이언트 노출 설정 준수.
- 보안: TLS, 토큰 인증, 승인 게이팅, 명령 권한, 원격 변경 감사(audit), 시크릿 처리.
- 서버 웹 UI: 관리 노드 목록, pending 승인 뷰, 노드별 자원 목록(디바이스 태그), 명령/상태 피드백(구현은 추후 expert-frontend).
- 원격 자원 편집(v1.2, 그룹 I): 관리자가 서버 웹 UI 에서 승인·온라인 노드의 플로우/에이전트를 생성·수정·삭제. 기존 시각 편집기 재사용, 명령 디스패치 경유 적용, 시크릿 redaction 라운드트립 보존, 편집 게이팅·실패 의미.
- 원격 노드 FULL 제어 패리티(v1.3, 그룹 J): 원격 노드의 플로우/에이전트/디바이스를 **로컬과 동일한 웹 UI**(로컬 목록 페이지 + 상세 패널 재사용)로 제어. (a) 신규 READ/QUERY 프록시 — 그룹 D command 와 대칭되는 **per-domain query-action**(열거 allowlist, FULL 커버리지)으로 노드의 라이브 데이터(상세/통계/노드 레벨 런타임·로그 등)를 온디맨드 취득(READ-ONLY), 서버 **단기 TTL 캐시**. (b) **스트리밍 프록시** — 디바이스 실시간 상태·에이전트 라이브 통계/시리즈를 서버 경유로 브라우저에 중계(subscribe/stream_data/unsubscribe, teardown·백프레셔; 폴링은 폴백). (c) 통합 UI — `target`(local | remote:{instanceId}) 추상화로 로컬 페이지를 원격에 파라미터화, 노드 셀렉터 + `?target=` 라우팅.
- 원격 노드 대시보드 패리티(v1.5, 그룹 L): 관리자가 서버에서 승인·온라인 원격 노드의 대시보드를 **로컬과 동일한 `DashboardPage`**(`target=remote:{instanceId}`)로 보고 제어. (a) **대시보드 config 읽기 프록시** — 노드의 대시보드 설정(`dashboardPages[]`/`activeDashboardId`/grid/refresh/`deviceGridLayout`, SPEC-DASHBOARD-001)을 그룹 J query-action(`dashboard` 도메인 `get_shared`/`get_mine`)으로 **READ-ONLY** 취득(원격 config 편집은 v1.5 비목표). (b) **패널 데이터 소스 target-aware** — flows/agents/devices/single-device 패널은 기존 target 훅 재사용, 자원 메트릭(신규 `monitor/metrics` query-action)·로그(신규 `monitor/logs` 스트림 action)·CHART(M8 스트림 프록시에 `chart` stream-action 추가, `/ws/chart/{channel}` 직결 대신 서버 경유 중계)·control 패널(useDeviceDetailTarget + 그룹 D 명령으로 디바이스 쓰기)을 원격화. 패널은 dashboard 레벨 TargetProvider/prop 으로 target 수신(target 없으면 로컬 렌더 바이트 동일). (c) **디바이스/자원 ref 네임스페이싱** — 원격 target 하에서 패널 device ref 는 그 노드 기준 해석(대시보드 config 는 **노드-로컬** — 노드별 fetch, deviceId 는 그 노드의 것; 중앙 cross-node config 비채택). (d) **UI** — 노드 대시보드(M9)에 **대시보드 서브탭** 추가 및/또는 `DashboardPage target=remote:{id}` 지원, 게이팅(승인+온라인+노출)·redaction·실패 의미·read-only-by-default·원격 컨텍스트 표시.
- 원격 관리 UI 정보 구조 재편 + 노드 그룹핑 + 노드 대시보드(v1.4, 그룹 K): (a) **노드 그룹핑** — `managed_nodes` 에 단일 그룹 라벨(`group_name`, 기본 빈값→"전체") 추가, 그룹 배정/해제·distinct 그룹 목록·그룹별 노드 목록 API + admin-gated 영속(단일 레벨, 노드당 최대 1 그룹). (b) **노드 시스템 정보 보고(BASIC)** — 노드가 OS+arch+version+started_at(epoch ms)을 register/heartbeat 페이로드로 보고(자원 메트릭 제외), 서버가 저장·상세/목록 API 로 노출, uptime 은 started_at 파생, 하위 호환(미보고 노드 빈값). (c) **노드별 운영 요약** — 기존 미러(그룹 E)에서 플로우/에이전트/디바이스 카운트+상태 분해를 파생. (d) **UI 재구성** — 사이드바 `원격 관리` 그룹을 `노드 관리`(디렉토리 뷰 + 노드 대시보드: 시스템 정보 + 운영 요약 + M8 통합 제어 재사용 Flow/Agent/Device 서브탭) + `등록 관리`(토큰 관리 + 노드 등록 관리)로 재편하여 기존 `관리 노드`/`원격 노드 제어` 진입점을 대체. M8 통합 제어 페이지·편집기 라우트는 새 `노드 관리`에서 도달 가능.
- 관리자 뷰 + 노드 화면 충실 재현(v1.6, 그룹 M): xflowd 서버 모드 구조를 **유지**한 채(별도 관리자 앱/바이너리/백엔드 분리/별도 SPA 없음 — M7~M10 백엔드·컴포넌트 전부 재사용) 원격 관리 UI 를 노드 화면을 충실히 재현하는 **전용 관리자 뷰**로 재구성. (a) **노드 해상도 보고** — 노드가 장비 모니터 해상도(width×height)를 M9 시스템 정보 페이로드로 보고(1차 출처=노드 config `display.resolution`/`display.width`+`display.height`, 헤드리스 데몬 특성상 운영자 선언), 서버가 `managed_nodes` 저장·노드 상세 노출, 하위 호환(미보고 시 폴백). (b) **관리자 뷰 레이아웃** — 노드 선택 시 관리 크롬(노드 피커=좌측 디렉토리 대체·M9 그룹 보존, 서브탭 네비 개요/플로우/에이전트/디바이스/대시보드, 관리 액션, 나가기)을 **상단 수평 바**로 옮기고 노드 화면이 **전체 너비/높이**를 점유, 전역 좌측 사이드바 숨김/접힘(좌우 폭 축소·왜곡 방지). M9 NodeManagementPage(좌측 디렉토리) → 상단 바 관리자 뷰로 **진화**(디렉토리/그룹 선택→상단 노드 피커, 서브탭→상단 바). 등록 관리는 그대로. (c) **해상도 충실 대시보드 렌더** — RemoteDashboardView(그룹 L)가 노드 보고 해상도 **고정 캔버스**에 렌더 후 종횡비 보존 스케일-투-핏(레터박스, 패널 리플로우 없음) → 노드 장비 화면의 왜곡 없는 복제, 패널(M10) 은 이 캔버스 안에 렌더. **변경 경로 불변**(그룹 D/I — 프록시 경유 변경 금지, REQ-J03/J12 일관).

**제외(Non-goals):**
- 역터널/리버스 프록시, 노드 간 직접 P2P, 공유 데이터베이스(명시적 금지 — RPC over 라이브 연결만).
- ~~서버→노드 일괄 펌웨어/바이너리 업데이트~~ → **v1.7(그룹 O)에서 부분 대체**: 관리 서버가 노드용 프로그램 이미지를 호스팅하고(릴리스 피드), 업데이트 소스를 저장해 명령에 주입하며, 아키텍처/OS-aware 그룹 일괄 업데이트(`system/update`)를 디스패치한다. 단, **바이너리 적용·검증은 노드의 기존 자가 업데이트(SPEC-UPDATE-001)** 가 수행하며 서버는 사전 서명 `.sig` 만 배포한다(서명·적용 로직 미신설 — A22). 펌웨어(비 xflowd) 업데이트는 여전히 제외.
- 멀티 서버 HA/클러스터링, 서버 페일오버.
- 노드 그룹/정책 기반 일괄 배포 오케스트레이션(향후 SPEC).
- ~~미러링된 자원의 서버 측 직접 영속 편집~~ → **v1.2(그룹 I)에서 대체**: 원격 플로우/에이전트 편집(생성·수정·삭제)은 이제 지원되나, **명령 디스패치 경유로만** 허용된다(서버는 (온라인) 노드로 명령을 전파하고 결과 수신 후에야 미러 캐시를 갱신; **서버 단독 직접 영속은 여전히 금지** — A4/REQ-E08). 영속 정의의 권위 소유자는 변함없이 노드이다.
- 노드↔노드 자원 복제/마이그레이션.
- **임의 노드 read 표면 프록시**: 원격 노드의 read 접근은 **열거된 per-domain query-action allowlist**(REQ-J04) 와 열거된 스트림 action(REQ-J08)만 허용한다. allowlist 밖 임의 action·raw HTTP path 프록시·서버 관리 자체 자원(설정/시스템/감사 등)의 노드 프록시 노출은 제외한다.
- **프록시 경유 변경**: read 프록시(query/스트림)는 **READ-ONLY** 이며, 모든 변경(디바이스 명령·메타데이터·라이프사이클·CRUD)은 그룹 D 명령 / 그룹 I(M7) CRUD 경로로만 수행한다(프록시 경유 변경 금지 — REQ-J03).
> 참고: v1.3 은 실시간 데이터를 **스트리밍 프록시**(REQ-J08, OQ-J3 RESOLVED)로 제공한다(폴링은 폴백). 이전 초안의 "스트리밍 연기" 비목표는 **철회**되었다.
- **노드 자원 메트릭(v1.4 그룹 K 제외)**: 노드 시스템 정보는 **BASIC(hostname/OS/arch/version/uptime/online/last_seen/status)** 만 보고·표시한다. **CPU/메모리/디스크 등 자원 사용률 메트릭은 본 마일스톤 제외**(향후 SPEC). 시스템 정보는 register/heartbeat 페이로드 경유 보고이며, 별도 메트릭 폴링/시계열은 신설하지 않는다.
- **다중 그룹 소속(v1.4 그룹 K 제외)**: 노드 그룹은 **단일 레벨·노드당 최대 1 그룹**이다. 계층형(중첩) 그룹, 한 노드의 다중 그룹 소속, 태그 기반 다중 분류, 그룹 단위 일괄 정책/배포 오케스트레이션은 제외한다(향후 SPEC).
- **원격 대시보드 config 편집(v1.5 그룹 L 제외)**: v1.5 원격 대시보드 패리티는 **READ-ONLY** 이다. 서버에서 원격 노드의 대시보드 config(`dashboardPages[]`/패널 레이아웃/`deviceGridLayout` 등, SPEC-DASHBOARD-001 `PUT/DELETE /dashboards/{shared,mine}`)를 **편집·생성·삭제·저장하는 기능은 제외**한다(차기 단계 위임 — OQ-L3 RESOLVED). 단, 대시보드 **패널이 표시하는 자원에 대한 제어 액션**(플로우 라이프사이클, 에이전트 start/stop, 디바이스 명령·메타데이터)은 v1.5 에 포함되며 **기존 그룹 D 명령 / 그룹 I CRUD 경로**로만 수행된다(신규 변경 경로 미신설, 프록시 경유 변경 금지 — REQ-J03/J12 일관).
- **중앙 cross-node 대시보드 config(v1.5 그룹 L 제외)**: 대시보드 config 는 **노드-로컬**(노드별 fetch, deviceId 는 그 노드 기준)로 둔다(OQ-L1 RESOLVED). 여러 노드의 패널을 한 대시보드 config 에 `{nodeId, deviceId}` 로 혼합 참조하는 **중앙 cross-node 대시보드**는 제외한다(향후 SPEC).
- **차트용 별도 WS 프록시 경로 신설(v1.5 그룹 L 제외)**: 원격 차트 스트리밍은 M8 스트림 프록시에 `chart` stream-action 을 **추가**해 재사용하며(OQ-L2 RESOLVED), `/ws/chart/{channel}` 를 그대로 프록시하는 별도 WS 경로(제2 WS 핸들러)는 신설하지 않는다.
- **노드 자원 메트릭 시계열/대시보드 풀(v1.5 그룹 L 부분 제외)**: resource 위젯은 노드의 `/monitor/metrics` 스냅샷을 신규 `monitor/metrics` query-action(단기 TTL 캐시 J16)으로 온디맨드 취득한다. 노드 메트릭의 **장기 시계열 수집/서버측 영속/그룹 K BASIC 외 자원 메트릭(CPU/메모리/디스크) 상시 폴링**은 제외한다(K 그룹 비목표 일관 — 대시보드 패널 표시 목적의 온디맨드 read 만 허용).
- **별도 관리자 앱/바이너리/백엔드 분리/별도 SPA(v1.6 그룹 M 제외)**: 관리자 뷰는 **xflowd 서버 모드 안의 UI 재구성**이다. 별도 관리자 전용 바이너리·프로세스, 백엔드 모듈 분리(server/client 외 신규 역할), 별도 SPA/프론트엔드 번들, 관리 기능의 별도 서비스 이전("기능 이전"=앱 분리)은 제외한다. M7~M10 의 백엔드(프록시·명령·미러·그룹·해상도 보고)·React 컴포넌트(통합 제어·노드 대시보드·DashboardPage·패널)를 전부 **재사용**한다(재발명/재작성 금지).
- **노드 화면 픽셀 스트리밍/원격 데스크톱(v1.6 그룹 M 제외)**: 충실 재현은 노드의 **대시보드 config + 데이터를 재구성**(그룹 L 데이터 경로 + 고정 캔버스 스케일)하는 것이며, 노드 화면을 비트맵/비디오로 캡처·인코딩·스트리밍하는 VNC/RDP 류 원격 데스크톱, 프레임버퍼 미러링은 제외한다(향후 SPEC).
- **런타임 해상도 자동 감지(v1.6 그룹 M 제외)**: xflowd 는 헤드리스 데몬이므로 OS/디스플레이 서버로부터 모니터 해상도를 런타임 자동 감지하지 않는다. 노드 해상도는 **운영자 config 선언**(`display.resolution` 등)을 1차 출처로 하며, 자동 감지(연결 디스플레이 enumerate)는 제외한다(OQ-M1).
- **대시보드 외 노드 화면 고정 캔버스 충실 재현(v1.6 그룹 M 부분 제외)**: 고정 캔버스(레터박스) 충실 재현은 **대시보드 서브탭**에 적용한다(OQ-M2). 플로우/에이전트/디바이스 통합 제어 서브탭은 상단 바 아래 **전체 너비 반응형**으로 두며(관리 도구 화면 — 노드 장비 화면 복제 대상 아님) 고정 캔버스 스케일을 적용하지 않는다.
- **수동 줌/팬 등 대화형 스케일 컨트롤(v1.6 그룹 M 제외)**: 1차 스케일 모드는 종횡비 보존 fit/레터박스(자동)이다(OQ-M4). 사용자 수동 줌 슬라이더·팬·1:1 픽셀 토글 등 대화형 스케일 컨트롤은 차기 단계로 분리한다.

## 2. 환경

| 항목 | 상세 |
|------|------|
| 백엔드 런타임 | Go 1.23+ |
| 백엔드 대상 모듈 | `internal/remote`(신규 — server/client/protocol), `internal/api/ws`(Message 봉투 재사용), `internal/auth`(JWT 노드 토큰), `internal/api/service`(어댑터 명령 적용), `internal/api/handler`(device 핸들러, 승인 API), `internal/config`(remote_management 섹션), `internal/storage`(ManagedNodeRepository + 미러 테이블), `cmd/xflowd/main.go`(배선) |
| 전송 | gorilla/websocket(기존 Hub/Client), 영속 연결, TLS(wss) |
| 인증 | JWT(HS256, `JWTService`), 등록 부트스트랩 시크릿(선택) |
| 프론트 런타임 | TypeScript 5.9+, React 19 |
| 프론트 대상 모듈 | `web/src/` 관리 노드 목록/승인/노드별 자원 뷰(신규, 추후 구현) |
| 테스트 | Go `testing`+`testify`, `go test -race ./...`, 프론트 Vitest + Testing Library |
| 개발 방법론 | Hybrid (`.moai/config/sections/quality.yaml` development_mode=hybrid): 신규 = TDD, 기존 변경 = 동작 보존 DDD, 커버리지 85%+ |

## 3. 가정

- **A1**: 클라이언트는 outbound WebSocket 만 가능하면 동작한다(서버가 클라이언트로 inbound 접속할 필요 없음 — NAT/방화벽 뒤 노드 지원).
- **A2**: 서버와 클라이언트는 동일 xflow 바이너리이며 `remote_management.mode` 로 역할이 갈린다. 한 인스턴스는 동시에 server 와 client 를 겸하지 않는다(본 SPEC 범위 — 겸용은 향후 과제).
- **A3**: 노드의 `instance_id` 는 한 번 생성되면 로컬에 영속되어 재시작/재접속 간 동일하다. 이것으로 서버는 동일 노드를 재식별한다.
- **A4**: 서버는 미러링된 자원 정의의 **캐시 소유자**일 뿐이며, **권위 있는 정의(authoritative definition)** 는 노드가 소유한다. 서버 측 편집은 노드 명령으로 전파되어야 반영된다.
- **A5**: 클라이언트는 기존 어댑터(`FlowServiceAdapter`/`AgentServiceAdapter`/device 핸들러)를 그대로 호출해 명령을 적용한다. 명령 적용은 로컬 API 호출과 동일한 검증/제약을 받는다(원격이라고 우회 없음).
- **A6**: 노출(exposure) 설정은 노드 측에서 평가된다. 서버는 노출되지 않은 자원의 존재를 알 수 없다(미러에 포함되지 않음).
- **A7**: 등록 승인 전(pending) 노드는 명령을 받을 수도, 자신을 관리당할 수도 없다. 미승인/거부 노드는 연결은 가능하나 등록 응답만 받는다.
- **A8**: 한 서버는 다수 노드를 관리하며, 각 노드는 단일 서버에 등록된다(멀티 서버 등록은 본 SPEC 제외).
- **A9**: 시크릿 필드(에이전트/디바이스 자격증명 등)는 기존 `secret_fields.go` redaction 정책과 일관되게 미러링 시 마스킹/제외된다.
- **A10**(v1.3): 노드는 자신의 로컬 read 핸들러(플로우/에이전트/디바이스 상세·통계·상태·시리즈·노드 레벨 런타임)를 이미 보유한다. READ/QUERY 프록시는 이 **기존 로컬 핸들러를 노드 내부에서 재실행**해 결과 JSON 을 반환하는 것이며, 서버는 그 결과를 가공 없이(또는 최소 가공으로) 전달한다(권위는 노드 — A4 일관).
- **A11**(v1.3): READ/QUERY 프록시는 미러(그룹 E)·명령(그룹 D)과 **독립된 별개 RPC** 이다. 미러는 요약 정의만(id/name/status/online/definition/updated_at), 프록시는 라이브 리치 런타임 데이터를 운반한다. 프록시는 **READ-ONLY**, 명령/CRUD 는 변경 전용으로 역할이 분리된다.
- **A12**(v1.3): 로컬 상세 패널은 폴링(`useDeviceRealtime`/`refetchInterval`)·온디맨드 fetch 로 동작한다. 원격 패리티에서 정적/온디맨드 데이터는 per-domain query-action(REQ-J04, 서버 단기 TTL 캐시 REQ-J16) 으로, 실시간 데이터는 **스트리밍 프록시**(REQ-J08, 서버 경유 중계)로 취득한다(폴링은 폴백). 노드는 query-action·스트림 action 을 자신의 기존 로컬 read/실시간 소스에 매핑한다(권위는 노드 — A4/A10).
- **A13**(v1.4): 노드 그룹은 **서버 측 속성**이다. 그룹 배정은 관리자가 서버에서 수행하며 `managed_nodes.group_name` 으로 영속된다. 그룹은 노드의 권위 정의(A4)와 무관한 **서버 운영 메타데이터**이므로, 그룹 변경은 노드로 명령을 전파하지 않는다(그룹 D 비경유 — 미러/자원 정의에 영향 없음). 그룹은 단일 레벨 자유 입력 라벨이며, 별도 그룹 엔티티 테이블을 신설하지 않고 `group_name` distinct 값으로 표현한다.
- **A14**(v1.4): 노드 시스템 정보(OS/arch/version/started_at)는 노드가 **register(필수) 및/또는 heartbeat(갱신)** 페이로드로 보고하는 BASIC 메타이다. 서버는 이를 `managed_nodes` 에 저장하고 상세/목록 API 로 노출한다. uptime 은 `started_at` 과 서버 현재 시각의 차로 산출(서버 시계 신뢰). **하위 호환**: 이전 버전 노드가 이 필드를 보내지 않아도 등록/관리는 정상 동작하며, 미보고 필드는 빈값/미표시로 처리된다(REQ-N03 회귀 0 유지).
- **A15**(v1.4): 노드별 운영 요약(플로우/에이전트/디바이스 카운트+상태 분해)은 **기존 미러(그룹 E) 데이터에서 파생**한다. 서버는 이미 노드별 미러 행(`mirrored_flows`/`mirrored_agents`/`mirrored_devices`, online/status 포함)을 보유하므로, 요약은 신규 노드 왕복 질의 없이 미러 집계로 계산된다(노드 오프라인 시에도 last-known 미러로 요약 제공 — REQ-E06 일관). 라이브 정밀 요약이 필요하면 그룹 J query-action 으로 보강할 수 있으나 기본 출처는 미러이다.

- **A16**(v1.5): 원격 대시보드는 **로컬 `DashboardPage` 를 재사용**하며(별도 원격 대시보드 화면 미신설), 그룹 L 은 그룹 J 의 target 추상화(`useResourceTargets`/`useDetailTargets`/`useTargetParam`/`useTargetGating`)·query 프록시·스트림 프록시·`useRemoteStream` 을 **재사용**한다. 패널이 `target` 을 받지 않거나 `target=local` 이면 렌더는 **로컬과 바이트 동일**(회귀 0)해야 한다. 대시보드 패널의 데이터 소스만 target-aware 화하며, 패널 UI/레이아웃 코드는 분기하지 않는다.

- **A17**(v1.5): 원격 대시보드 config 는 **노드-로컬**이다. 노드가 자신의 대시보드 config(SPEC-DASHBOARD-001 `dashboardPages[]` 등)의 **권위 소유자**이며(A4 일관), 서버는 그룹 J `dashboard` 도메인 query-action(`get_shared`/`get_mine`)으로 **READ-ONLY** 취득한다. config 내부의 `deviceId`/패널 자원 참조는 **그 노드 기준**으로 해석되고(서버 자신의 디바이스와 혼동 금지), 노드의 redaction 정책이 config 응답에도 적용된다. v1.5 는 원격 config 를 편집/저장하지 않는다(비목표).

- **A18**(v1.5): 차트 패널의 라이브 스트리밍은 노드의 `/ws/chart/{channel}` 소스를 **노드 내부에서 구독**하여 그룹 J 스트림 프록시(`chart` stream-action)로 서버 경유 중계한다(A10 일관 — 노드가 기존 로컬 실시간 소스 재실행). `useChartChannel` 은 이미 `wsBaseUrl`/`createClient` 주입점을 보유하므로, 원격 target 에서는 직접 WS 직결 대신 스트림 프록시(SSE) 어댑터를 주입하여 동일 패널 코드가 동작한다. 스트림 미지원/실패 시 차트는 query-action 폴링(스냅샷)으로 폴백된다(J08 폴백 일관).

- **A19**(v1.6): 관리자 뷰는 **xflowd 서버 모드 내부의 UI 재구성**이며 별도 앱/바이너리/백엔드 분리/별도 SPA 가 아니다. 그룹 M 은 M7~M10 의 백엔드(query/스트림 프록시·그룹 D 명령·미러·노드 그룹·시스템 정보 보고)와 React 컴포넌트(통합 제어 페이지·`NodeDashboard`·`DashboardPage`·패널·`RemoteDashboardView`)를 **재사용**하며, 신규는 (1) 노드 해상도 보고 필드·저장·노출, (2) 상단 바 관리자 뷰 셸 + 노드 피커 + 전역 사이드바 숨김, (3) 대시보드 고정 캔버스 스케일 래퍼뿐이다. **변경 경로는 불변**(그룹 D/I — 프록시 경유 변경 금지, REQ-J03/J12 일관).

- **A20**(v1.6): 노드 해상도는 노드의 **장비 모니터(키오스크/터치스크린) 디스플레이 해상도**(width×height, px)를 의미한다. xflowd 는 헤드리스 데몬이므로 OS 디스플레이로부터 런타임 자동 감지하지 않고, 운영자가 노드 config(`display.resolution` 또는 `display.width`+`display.height`)로 **선언**한 값을 1차 출처로 한다(OQ-M1 ✅ RESOLVED — config 선언). 노드는 이 값을 M9 시스템 정보 페이로드(register 필수·heartbeat 갱신)로 운반하고 서버는 `managed_nodes` 에 저장한다. **하위 호환**: 미보고/미설정 노드는 빈값으로 처리되며(REQ-N03/K09 일관, 회귀 0), 관리자 뷰는 합리적 기본값(예: 1920×1080) 또는 가용 컨테이너 크기로 폴백한다. 이 해상도는 그룹 L 의 IoT 디바이스/대시보드 deviceId 네임스페이싱과 무관한 **노드(설치본) 표시 메타**이다.

- **A21**(v1.6): 대시보드 충실 재현은 노드 장비 화면을 **비트맵/비디오로 스트리밍하지 않고**, 그룹 L 의 노드-로컬 대시보드 config(REQ-L01) + 패널 데이터(REQ-L04~L08)를 **재구성**한 뒤 노드 보고 해상도와 동일한 고정 캔버스에 렌더하여 종횡비 보존 스케일-투-핏(레터박스)한다. 패널 컴포넌트는 **target 미지정 시 로컬 바이트 동일**(A16) 원칙을 유지하며 고정 캔버스는 그 바깥의 래퍼(스케일 컨테이너)로 적용된다(패널 UI/레이아웃 코드 비분기). 노드 측에 해상도에 따른 렌더 변형은 없다(노드 권위 config 그대로 — A4/A17).

- **A22**(v1.7): 관리 서버가 호스팅하는 노드용 릴리스 피드는 **노드의 기존 자가 업데이트 메커니즘(SPEC-UPDATE-001)을 변경하지 않는다**. 피드는 노드 updater 가 소비하는 **익명 GitHub-Releases 호환** 형태(`releases/latest`·`releases`·`releases/download/{version}/{filename}`, asset 객체에 `browser_download_url`·`size`·`name`)이며, asset URL 은 항상 **https 강제**(`public_base_url` 설정 또는 요청 Host/`X-Forwarded-*` 에서 유도)이다. 무결성은 노드 측 Ed25519 서명 검증(SPEC-UPDATE-001 M4)으로 보장되며 서버는 **사전 서명된 `.sig` 만 저장·배포**한다(서버가 서명을 생성하지 않음). 공개키는 노드 로컬(`update.public_key_path`)에만 존재하며 서버·피드·업데이트 소스 어디에도 전송되지 않는다.

- **A23**(v1.7): 원격 업데이트 소스(`update_url`+채널)는 **서버 운영 메타데이터**로 SettingsRepository(`remote.update_source`)에 1회 저장되고 필요시에만 변경된다(그룹 D 비경유 — 노드 권위 자원 정의에 영향 없음, A13 일관). 서버는 원격 업데이트 명령(UpdateNode/UpdateGroup)을 디스패치할 때 저장된 소스를 명령 인자에 **자동 주입**한다(요청이 명시하면 요청 우선). `update_url` 은 빈 값(=해제, 노드 로컬 설정 사용) 또는 `https://` 스킴만 허용한다(http 거부). 업데이트 소스에는 공개키를 포함하지 않는다(A22 — 노드 로컬 신뢰 앵커).

- **A24**(v1.7): 그룹 일괄 업데이트의 노드별 타깃 버전은 **서버가 각 노드의 보고된 OS/Arch(그룹 K BASIC 시스템 정보, REQ-K07/K08)로 해석**한다(노드는 변경 없이 기존 `system/update` 명령 + `TargetVersion` 인자 재사용). 미매핑/자산 없는 노드는 디스패치하지 않고 **사유와 함께 결과에 건너뜀**으로 보고한다(부분 성공 — 다른 아키텍처는 계속 진행). 전략(`latest`/`pin`/`per_arch`)은 서버 측 해석 정책일 뿐 노드 동작·프로토콜 봉투를 바꾸지 않는다(REQ-N04 유지).

## 4. 요구사항 (EARS)

### 4.1 그룹 A — 모드 & 설정 (Mode & Config)

**REQ-REMOTE-A01**: 모드 선택
시스템은 **항상** `remote_management.mode` 설정값(`server` | `client` | `disabled`)에 따라 원격 관리 역할을 결정해야 한다. `disabled` 이면 어떤 관리 연결도 생성/수락하지 않아야 한다.

**REQ-REMOTE-A02**: 클라이언트 서버 주소
**WHEN** mode 가 `client` 이면, **THEN** 시스템은 `remote_management.server_url`(wss URL)로 접속을 시도해야 한다. `server_url` 이 비어 있으면 명확한 설정 오류를 기록하고 접속을 시도하지 않아야 한다.

**REQ-REMOTE-A03**: 영속 instance_id 생성
시스템은 **항상** 기동 시 영속 `instance_id`(UUID)를 보유해야 한다. 존재하지 않으면 한 번 생성하여 로컬에 영속하고, 이후 재시작/재접속 간 동일 값을 유지해야 한다(노드 재식별 기반).

**REQ-REMOTE-A04**: 노출(exposure) 설정
시스템은 **항상** 클라이언트 설정으로 어떤 플로우/에이전트/디바이스를 서버에 노출할지 결정해야 한다(opt-in). 노출되지 않은 자원은 인벤토리 미러에 포함되지 않아야 한다.

**REQ-REMOTE-A05**: heartbeat 간격 설정
시스템은 **항상** `remote_management.heartbeat_interval` 에 따라 heartbeat 주기를 결정해야 한다. 미설정 시 안전 기본값을 사용해야 한다.

**REQ-REMOTE-A06**: 설정 핫리로드
**WHEN** `remote_management` 설정이 런타임에 변경되면, **THEN** 시스템은 기존 `OnChange`/`WatchConfig` 메커니즘으로 이를 감지하고, 모드/주소/노출 변경을 (재접속을 포함해) 안전하게 반영해야 한다.

**REQ-REMOTE-A07**: 노출 변경 재미러링
**WHEN** 노출 설정이 변경되면, **THEN** 클라이언트는 갱신된 노출 범위로 인벤토리 스냅샷/델타를 다시 미러링해야 한다(노출 해제된 자원은 서버 캐시에서 제거되도록 신호).

### 4.2 그룹 B — 연결 라이프사이클 (Connection Lifecycle)

**REQ-REMOTE-B01**: 클라이언트 dial
**WHEN** mode=client 이고 server_url 이 유효하면, **THEN** 클라이언트는 서버 관리 WS 엔드포인트로 영속 연결을 dial 해야 한다(클라이언트→서버 방향, outbound).

**REQ-REMOTE-B02**: 인증 핸드셰이크
**WHEN** 클라이언트가 연결을 수립하면, **THEN** 시스템은 핸드셰이크에서 자격(노드 토큰 또는 부트스트랩 시크릿/등록 요청)을 검증해야 한다(`internal/api/handler/websocket.go` 의 Bearer/`?token=` 패턴 재사용, 단 별도 관리 엔드포인트).

**REQ-REMOTE-B03**: heartbeat
시스템은 **항상** 영속 연결에서 주기적 heartbeat 를 주고받아 연결 생존성을 확인해야 한다(기존 ping 54s/pong 60s 기반 위 관리 레벨 heartbeat 포함).

**REQ-REMOTE-B04**: 자동 재연결(백오프)
**IF** 관리 연결이 끊기면, **THEN** 클라이언트는 지수 백오프로 재연결을 시도해야 한다(MQTT/소켓 에이전트의 재연결 패턴 준용). 재연결 성공 시 재인증 후 인벤토리를 재동기화해야 한다.

**REQ-REMOTE-B05**: 서버 측 online/offline 추적
시스템은 **항상** 서버에서 각 노드의 online/offline 상태를 연결/heartbeat/disconnect 로 추적하고 갱신해야 한다.

**REQ-REMOTE-B06**: 정상 disconnect
**WHEN** 노드가 정상 종료하면, **THEN** 클라이언트는 가능하면 정상 disconnect 를 통지하고, 서버는 해당 노드를 offline 으로 표시하되 last-known 인벤토리를 보존해야 한다.

**REQ-REMOTE-B07**: 오프라인 명령 거절
**IF** 대상 노드가 offline 인데 서버가 명령을 디스패치하려 하면, **THEN** 시스템은 명령을 거절하거나 보류 사유와 함께 명확한 오류를 반환해야 한다(라이브 연결 위 RPC 만 — 큐잉/오프라인 적용은 본 SPEC 제외).

### 4.3 그룹 C — 등록 & 승인 (Registration & Approval)

**REQ-REMOTE-C01**: 등록 요청
**WHEN** 클라이언트가 서버에 처음(또는 미등록 상태로) 접속하면, **THEN** 클라이언트는 `instance_id` + 노드 정보(hostname, version, 노출 자원 요약)를 담은 등록 요청(`register`)을 보내야 한다.

**REQ-REMOTE-C02**: pending 큐 보류
**WHEN** 서버가 미등록 노드의 등록 요청을 받으면, **THEN** 서버는 그 요청을 **pending** 상태로 보류하고, 즉시 관리 권한을 부여하지 않아야 한다.

**REQ-REMOTE-C03**: 관리자 승인/거부
**WHEN** 관리자가 pending 노드를 승인 또는 거부하면, **THEN** 서버는 해당 노드의 등록 상태를 갱신해야 한다(승인=approved, 거부=rejected). 승인/거부 결정은 서버가 한다.

**REQ-REMOTE-C04**: 승인 시 토큰 발급
**WHEN** 노드가 승인되면, **THEN** 서버는 그 노드용 노드 토큰(JWT, `JWTService` 재사용)을 발급하고 `register_ack` 로 전달해야 한다. 클라이언트는 이 토큰을 영속하여 이후 재접속 인증에 사용해야 한다.

**REQ-REMOTE-C05**: 재접속 재인증
**WHEN** 승인된 노드가 재접속하면, **THEN** 시스템은 노드 토큰으로 재인증하고, 유효하면 재등록 없이 관리 세션을 복원해야 한다.

**REQ-REMOTE-C06**: 미승인 노드 차단
**IF** 노드가 pending 또는 rejected 상태이면, **THEN** 서버는 그 노드에 명령을 디스패치하지 않고, 그 노드를 관리 대상으로 노출하지 않아야 한다(등록 응답만 허용).

**REQ-REMOTE-C07**: 폐기(revocation)
**WHEN** 관리자가 등록된 노드를 폐기하면, **THEN** 서버는 노드 토큰을 무효화(blacklist)하고 관리 연결을 종료하며, 이후 그 토큰의 재인증을 거부해야 한다.

**REQ-REMOTE-C08**: 부트스트랩 신뢰(선택)
**WHERE** 부트스트랩 enrollment 시크릿이 구성되어 있으면, 시스템은 등록 요청 시 사전 공유 시크릿으로 1차 신뢰를 검증하는 옵션을 제공할 수 있다(미구성 시 순수 관리자 승인 흐름).

### 4.4 그룹 D — 원격 제어 (Remote Control)

**REQ-REMOTE-D01**: 명령 디스패치
**WHEN** 서버 관리자가 승인된 온라인 노드에 대해 자원 작업을 지시하면, **THEN** 서버는 라이브 연결로 `command` 메시지(대상 instance_id + 작업 + 페이로드)를 디스패치해야 한다.

**REQ-REMOTE-D02**: 플로우 명령 적용
시스템은 **항상** 플로우 명령(Create/Get/List/Update/Delete/Deploy/Start/Stop/Pause)을 클라이언트의 `FlowServiceAdapter` 호출로 적용해야 한다.

**REQ-REMOTE-D03**: 에이전트 명령 적용
시스템은 **항상** 에이전트 명령(List/Get/Create/Update/Delete/Start/Stop/Restart)을 클라이언트의 `AgentServiceAdapter` 호출로 적용해야 한다.

**REQ-REMOTE-D04**: 디바이스 메타데이터 명령 적용
시스템은 **항상** IoT 디바이스 메타데이터 명령을 클라이언트의 device 핸들러(`internal/api/handler/device.go`) 경로로 적용해야 한다.

**REQ-REMOTE-D05**: 결과/ack 반환
**WHEN** 클라이언트가 명령을 적용하면, **THEN** 클라이언트는 같은 연결로 `command_result`(성공 결과 또는 오류)를 명령 상관 id 와 함께 반환해야 한다.

**REQ-REMOTE-D06**: 명령 타임아웃
**IF** 디스패치된 명령에 대해 제한 시간 내 결과가 오지 않으면, **THEN** 서버는 그 명령을 타임아웃 처리하고 호출자에게 명확한 오류를 반환해야 한다(미적용으로 간주).

**REQ-REMOTE-D07**: 명령 상관(correlation)
시스템은 **항상** 각 명령에 상관 id 를 부여하여 요청-결과를 1:1 매칭하고, 동시 다수 명령을 구분해야 한다.

**REQ-REMOTE-D08**: 명령 권한
시스템은 **항상** 서버 관리자만 명령을 발행할 수 있고, 승인된 노드만 명령을 수신·적용하도록 권한을 강제해야 한다. 미승인 출처의 명령은 클라이언트가 거부해야 한다.

**REQ-REMOTE-D09**: 적용 실패 처리
**IF** 명령 적용이 로컬 어댑터에서 실패하면(검증 오류·충돌 등), **THEN** 클라이언트는 부분 적용 없이(가능한 한) 실패를 `command_result` 오류로 보고하고 로컬 상태를 일관되게 유지해야 한다.

### 4.5 그룹 E — 인벤토리 미러링 & 목록 (Inventory Mirroring & Listing)

**REQ-REMOTE-E01**: 접속 시 스냅샷
**WHEN** 클라이언트가 (재)접속하면, **THEN** 클라이언트는 노출 범위의 플로우/에이전트/디바이스 인벤토리 스냅샷(`inventory_snapshot`)을 서버로 push 해야 한다.

**REQ-REMOTE-E02**: 변경 시 델타
**WHEN** 노드의 노출된 자원이 변경되면(추가/수정/삭제), **THEN** 클라이언트는 인벤토리 델타(`inventory_delta`)를 서버로 push 해야 한다.

**REQ-REMOTE-E03**: 서버 DB 캐시
시스템은 **항상** 미러링된 인벤토리를 서버 DB(`mirrored_flows`/`mirrored_agents`/`mirrored_devices`)에 캐시하여, 노드가 오프라인일 때도 last-known 상태를 제공해야 한다.

**REQ-REMOTE-E04**: 출처 노드 태깅
시스템은 **항상** 미러링된 모든 항목을 출처 노드(`instance_id`)로 태깅하여, 어떤 노드와 연관된 자원인지 식별 가능하게 해야 한다(디바이스 연관 표시).

**REQ-REMOTE-E05**: 통합 목록
시스템은 **항상** 서버에서 관리 노드 목록과, 각 노드의 플로우/에이전트/디바이스를 출처 노드 태그와 함께 목록화할 수 있어야 한다.

**REQ-REMOTE-E06**: 오프라인 last-known 표시
**WHILE** 노드가 offline 인 동안, 시스템은 그 노드의 마지막 미러 상태(last-known)를 offline 표식과 함께 표시해야 한다.

**REQ-REMOTE-E07**: 노출 설정 준수
시스템은 **항상** 클라이언트 노출 설정에 포함되지 않은 자원을 미러·목록에서 제외해야 한다(서버는 노출된 것만 본다).

**REQ-REMOTE-E08**: 서버 편집→명령 전파
**WHEN** 서버 측에서 미러링된 자원에 대한 편집이 발생하면, **THEN** 시스템은 그것을 (온라인) 노드로의 명령(그룹 D)으로 전파해야 하며, 명령 결과 수신 후에야 서버 캐시를 갱신해야 한다(서버 단독 영속 금지 — A4).

### 4.6 그룹 F — 보안 (Security)

**REQ-REMOTE-F01**: TLS 전송
시스템은 **항상** 관리 연결을 TLS(wss)로 보호할 수 있어야 한다(평문 전송 금지 권고).

**REQ-REMOTE-F02**: 토큰 인증
시스템은 **항상** 승인된 노드의 재접속을 노드 토큰(JWT)으로 인증해야 한다.

**REQ-REMOTE-F03**: 승인 게이팅
시스템은 **항상** 등록 승인 전 노드에 어떤 관리 권한도 부여하지 않아야 한다(C06 재확인).

**REQ-REMOTE-F04**: 명령 권한 강제
시스템은 **항상** 명령 발행/수신 권한을 강제해야 한다(서버 관리자 발행, 승인 노드 수신 — D08 재확인).

**REQ-REMOTE-F05**: 원격 변경 감사(audit)
시스템은 **항상** 원격 명령에 의한 변경(누가/언제/어느 노드/무엇)을 감사 로그로 기록해야 한다.

**REQ-REMOTE-F06**: 시크릿 처리
시스템은 **항상** 미러링/전송 시 시크릿 필드를 기존 redaction 정책(`secret_fields.go`)과 일관되게 마스킹/제외해야 한다. 노드 토큰/부트스트랩 시크릿은 버전관리에 커밋하지 않아야 한다.

**REQ-REMOTE-F07**: 폐기 즉시성
**WHEN** 노드 토큰이 폐기되면, **THEN** 시스템은 해당 토큰의 후속 인증을 즉시 거부해야 한다(JWTService blacklist 재사용).

> 보안 요구는 OWASP API Security(인증·인가·과도 노출·감사) 원칙을 참조한다.

### 4.7 그룹 G — 웹 UI (서버 측)

**REQ-REMOTE-G01**: 관리 노드 목록
시스템은 **항상** 서버 웹 UI 에서 관리 노드 목록(instance_id, hostname, version, online/offline, 등록 상태)을 제공해야 한다.

**REQ-REMOTE-G02**: pending 승인 뷰
시스템은 **항상** pending 등록 요청 목록과 승인/거부 액션을 제공해야 한다.

**REQ-REMOTE-G03**: 노드별 자원 목록(디바이스 태그)
시스템은 **항상** 노드별 플로우/에이전트/디바이스 목록을 출처 노드 태그와 함께 표시해야 한다.

**REQ-REMOTE-G04**: 명령/상태 피드백
시스템은 **항상** 원격 명령의 진행/성공/실패와 노드 online/offline 상태 변화를 UI 에 피드백해야 한다.

> 그룹 G 구현은 추후 expert-frontend 가 담당한다(본 SPEC 은 계약·요구만 정의).

### 4.7b 그룹 H — 수동 enrollment (Manual Enrollment) — v1.1 확장

> **범위(v1.1)** — 본 그룹은 v1.1 확장이다. 기존 등록/승인 상태 머신(그룹 C)을 보존한 채, 관리자가 노드를 수동으로(개별 register 승인 없이) 가입시키는 두 경로를 추가한다: (A) instance_id allow-list 기반 사전 등록, (B) enrollment 토큰. disabled/client 회귀는 없다(REQ-N03 유지). 본 그룹의 모든 REST 엔드포인트는 기존 `/remote/*` admin 인증을 재사용한다(REQ-F04).

**REQ-REMOTE-H01**: 사전 등록 생성·삭제 (pre-registration)
시스템은 **항상** 관리자가 노드 접속 전에 `instance_id`(+선택 `name`)로 노드를 `approved` 상태(online=false, 토큰 미발급)로 사전 생성하고(`POST /api/v1/remote/nodes`, 201), 삭제할 수 있도록(`DELETE /api/v1/remote/nodes/{instance_id}`, 204) 해야 한다. 동일 `instance_id` 중복 생성은 409 로 거부하고, 삭제 시 라이브 연결이 있으면 종료하며 발급된 노드 토큰이 있으면 폐기해야 한다(REQ-F07).

**REQ-REMOTE-H02**: 사전 등록 노드 접속 시 자동 승인
**WHEN** `approved` 이지만 노드 토큰이 미발급인 노드(사전 등록)가 처음 `register` 하면, **THEN** 시스템은 관리자 추가 개입 없이 즉시 노드 토큰을 발급하고 `register_ack{approved, node_token}` 를 응답해야 한다. 이미 토큰을 보유한 `approved` 노드의 재접속은 기존 동작(정상 재접속)을 유지해야 한다.

**REQ-REMOTE-H03**: enrollment 토큰 발급
시스템은 **항상** 관리자가 enrollment 토큰을 발급할 수 있도록(`POST /api/v1/remote/enrollment-tokens`, 본문 `{label?, expires_in?(duration), max_uses?(int)}`) 해야 하며, 발급 응답에서 원본 토큰을 **1회만** 노출하고(`{id, token, label, expires_at, max_uses}`) 저장소에는 원본 토큰을 보관하지 않아야 한다(REQ-H06).

**REQ-REMOTE-H04**: enrollment 토큰 목록·폐기
시스템은 **항상** enrollment 토큰의 메타데이터 목록을 토큰/해시 노출 없이 제공하고(`GET /api/v1/remote/enrollment-tokens`), 토큰을 즉시 폐기할 수 있도록(`DELETE /api/v1/remote/enrollment-tokens/{id}`, 204) 해야 한다. 폐기된 토큰의 후속 사용은 즉시 거부되어야 한다.

**REQ-REMOTE-H05**: enrollment 토큰 기반 자동 승인
**WHEN** `register` 가 유효한(해시 일치·미만료·미폐기·미소진) enrollment 토큰을 운반하면, **THEN** 시스템은 다이얼한 `instance_id`(+hostname/version)로 노드를 `approved` 로 생성하고, 노드 토큰을 발급하며, enrollment 토큰의 `uses` 를 증가시키고 `register_ack{approved, node_token}` 를 응답해야 한다. **WHEN** 토큰이 무효/만료/폐기/소진이면, **THEN** 시스템은 기존 `pending` 흐름으로 폴백해야 한다(보수적 — 관리자 결정 위임). enrollment 토큰 게이트는 기존 선택적 `bootstrap_secret` 게이트(REQ-C08)와 조합되며, `bootstrap_secret` 검증을 먼저 통과한 뒤에만 평가된다.

**REQ-REMOTE-H06**: 토큰 해싱·1회 노출
시스템은 **항상** enrollment 토큰을 ≥256비트 암호학적 랜덤으로 생성하고, 저장소에는 SHA-256 해시만 영속하며(원본 비저장), 원본 토큰은 발급 응답에서만 1회 노출하고 이후 어디에도(목록/로그/감사) 노출하지 않아야 한다(REQ-F06 일관).

**REQ-REMOTE-H07**: 사용 횟수 원자성
시스템은 **항상** `max_uses` 가 설정된 enrollment 토큰의 `uses` 증가를 원자적(조건부 `uses < max_uses`)으로 수행하여, 동시 다수 노드 register 경합에서도 `max_uses` 를 초과 발급하지 않아야 한다.

**REQ-REMOTE-H08**: 수동 enrollment 감사
시스템은 **항상** 수동 enrollment 에 의한 변경을 감사 기록해야 한다(REQ-F05 일관): 관리자 사전 등록/토큰 발급/폐기는 actor=관리자로, enrollment 토큰 기반 자동 승인은 enrollment 토큰 **id** 로(원본 토큰 금지 — REQ-F06) 기록한다.

### 4.7c 그룹 I — 원격 자원 편집 (Remote Resource Editing) — v1.2 확장

> **범위(v1.2)** — 본 그룹은 관리자가 서버 웹 UI 에서 **승인·온라인** 노드의 플로우/에이전트를 **생성·수정·삭제(FULL CRUD)** 하는 기능을 추가한다. 편집은 기존 시각 편집기(`web/src/pages/editor/EditorPage.tsx`, React Flow 캔버스)를 **재사용**한다(원격 플로우/에이전트를 동일 편집기로 열어 노드로 저장). 본 그룹은 **서버 단독 영속을 절대 하지 않는다**: 모든 편집은 그룹 D 명령으로 (온라인) 노드에 전파되어 노드가 자신의 어댑터(`FlowServiceAdapter`/`AgentServiceAdapter`, 영속 소유자)로 로컬 적용하고, **명령 결과 수신 후에야** 서버가 미러 캐시를 갱신한다(REQ-E08·A4 경로 재사용). 기존 `command` 프로토콜은 임의 domain/action 을 이미 지원하고, 클라이언트 어댑터는 Create/Update/Delete 를 이미 보유하므로, 신규 작업은 (1) create/update/delete action 의미 확정, (2) 신규 서버 REST 엔드포인트, (3) 해당 action 의 클라이언트 어댑터 바인딩, (4) 웹 편집기 통합이다. 본 그룹은 **§1.5 비목표의 "미러링된 자원의 서버 측 직접 영속 편집" 항목을 대체**한다(편집은 이제 명령 디스패치 경유로 허용 — 서버 단독 영속은 여전히 금지).

**REQ-REMOTE-I01**: 원격 플로우 생성 엔드포인트
시스템은 **항상** 관리자가 승인·온라인 노드에 새 플로우를 생성하도록(`POST /api/v1/remote/nodes/{instance_id}/flows`, 본문=플로우 정의 JSON) 제공해야 한다. 서버는 이를 `command{domain:flow, action:create}`(그룹 D)로 전파하고, 노드의 `FlowServiceAdapter.Create` 적용 결과를 받은 **후에만** 미러 캐시에 생성 행을 반영하며, 성공 시 생성된 자원(또는 그 식별자)을 201 로 반환해야 한다. 서버는 명령 전파 없이 자체적으로 미러에 영속 생성하지 않아야 한다(A4).
> **결정(§5.9-8 RESOLVED, 노드 채번)**: 새 flow ID 는 노드의 어댑터 `Create` 가 부여·반환하고 서버는 반환된 식별자를 미러에 기록한다(서버가 ID 를 생성하지 않음). **결정(§5.9-9 RESOLVED, 수동 노출)**: 새로 생성된 자원은 **자동 노출되지 않는다** — 운영자가 노드 노출 설정(REQ-A04/A07)을 갱신하기 전까지 미노출 상태를 유지한다.

**REQ-REMOTE-I02**: 원격 플로우 수정 엔드포인트
시스템은 **항상** 관리자가 승인·온라인 노드의 기존 플로우를 수정하도록(`PATCH /api/v1/remote/nodes/{instance_id}/flows/{flow_id}`, 본문=갱신 정의 JSON) 제공해야 한다. 서버는 이를 `command{domain:flow, action:update}` 로 전파하고, 노드의 `FlowServiceAdapter.Update` 결과 수신 후에만 미러 캐시를 갱신해야 한다(REQ-E08 경로).
> **결정(§5.9-7 RESOLVED, 노드 권위 + 경고)**: 편집 동시성은 낙관적 잠금/버전 체크/409 없이 노드 권위 last-write-wins(A4)로 처리한다. 동시 노드 로컬 변경이 감지되면 UI 에 비차단 경고를 표시하되 편집을 거부하지 않는다.

**REQ-REMOTE-I03**: 원격 플로우 삭제 엔드포인트
시스템은 **항상** 관리자가 승인·온라인 노드의 플로우를 삭제하도록(`DELETE /api/v1/remote/nodes/{instance_id}/flows/{flow_id}`, 204) 제공해야 한다. 서버는 이를 `command{domain:flow, action:delete}` 로 전파하고, 노드의 `FlowServiceAdapter.Delete` 결과 수신 후에만 미러 캐시에서 해당 행을 제거해야 한다.

**REQ-REMOTE-I04**: 원격 에이전트 생성·수정·삭제 엔드포인트
시스템은 **항상** 관리자가 승인·온라인 노드의 에이전트를 생성(`POST /api/v1/remote/nodes/{instance_id}/agents`, 201)·수정(`PATCH /api/v1/remote/nodes/{instance_id}/agents/{agent_id}`)·삭제(`DELETE /api/v1/remote/nodes/{instance_id}/agents/{agent_id}`, 204)하도록 제공해야 한다. 서버는 각각을 `command{domain:agent, action:create|update|delete}` 로 전파하고, 노드의 `AgentServiceAdapter` 대응 메서드 결과 수신 후에만 미러 캐시를 갱신해야 한다(REQ-E08 경로).
> **결정(§5.9 RESOLVED)**: 에이전트도 플로우와 동일하게 — 새 agent ID 는 노드 어댑터 `Create` 가 채번(§5.9-8), 새로 생성된 에이전트는 자동 노출되지 않음(§5.9-9, 수동 노출), 수정 동시성은 노드 권위 + 비차단 경고(§5.9-7)를 따른다.

**REQ-REMOTE-I05**: 편집 게이팅 (승인·온라인·노출 범위)
시스템은 **항상** 원격 생성/수정/삭제 명령을 **승인(approved)** 이고 **온라인(online)** 인 노드에 대해서만 허용해야 한다. 추가로, 수정/삭제 대상 자원은 해당 노드의 **노출 범위(exposure scope)** 내에 있어야 하며(REQ-D08/E07 일관), 범위 밖 자원에 대한 편집은 거부해야 한다. 미승인/오프라인/범위 밖 요청은 명령을 디스패치하지 않고 명확한 오류를 반환해야 한다.

**REQ-REMOTE-I06**: create/update/delete action 의미 및 어댑터 바인딩
시스템은 **항상** `command` 프로토콜의 flow/agent 도메인에 대해 `create`·`update`·`delete` action 을 명확한 의미로 정의하고, 클라이언트에서 각 action 을 해당 로컬 어댑터 메서드(`FlowServiceAdapter.Create/Update/Delete`, `AgentServiceAdapter.Create/Update/Delete`)에 바인딩해야 한다. 적용은 로컬 API 와 동일한 검증/제약을 받아야 한다(원격 우회 없음 — A5).

**REQ-REMOTE-I07**: 시크릿 redaction 라운드트립 보존
시스템은 **항상** 미러링된 정의 JSON 의 마스킹된 시크릿 필드(REQ-F06)가 편집 저장 시 노드의 실제 시크릿 값을 덮어쓰지 않도록 보장해야 한다. 구체적으로, 마스킹되었거나 변경되지 않은 시크릿 필드는 **갱신 페이로드에서 생략**되어야 하며, 노드는 수신한 정의를 자신의 **기존 시크릿과 병합**(생략된 시크릿 필드는 기존 값 유지)하여 적용해야 한다. 마스킹된 자리표시자 값이 그대로 영속되어서는 안 된다.
> **결정(§5.9-6 RESOLVED, 필드 부재 + 노드 backfill)**: 메커니즘이 확정되었다 — 갱신 페이로드는 미변경/마스킹 시크릿 필드를 **완전히 생략(필드 부재)** 하며 sentinel/자리표시자 값을 와이어로 전송하지 않는다. 노드는 어댑터 `Create`/`Update` 호출 **전에** 자신의 기존 자원 정의를 로드하여 부재한 시크릿 필드를 **기존값으로 backfill** 한 뒤 어댑터에 전달한다.

**REQ-REMOTE-I08**: 시각 편집기 재사용 (원격 플로우)
시스템은 **항상** 관리자가 원격(미러링된) 플로우를 기존 시각 편집기(`EditorPage.tsx`, React Flow 캔버스)로 열어 편집하고, 동일 편집기에서 대상 노드로 저장(REQ-I02 의 PATCH, 신규 플로우는 REQ-I01 의 POST)할 수 있도록 제공해야 한다. 편집기는 로컬 편집과 원격 편집을 구분하여 저장 대상(로컬 저장소 vs `instance_id` 노드)을 명확히 해야 한다.

**REQ-REMOTE-I09**: 에이전트 편집/설정 surface
시스템은 **항상** 관리자가 원격 에이전트를 편집/구성할 수 있는 편집·설정 surface 를 제공해야 한다(에이전트 종류별 설정 폼 또는 정의 편집). 저장 시 REQ-I04 의 create/update 경로로 대상 노드에 전파해야 한다.

**REQ-REMOTE-I10**: 원격 자원 페이지의 생성/삭제 액션 및 피드백
시스템은 **항상** 원격 자원 페이지(노드별 플로우/에이전트 목록)에서 생성·삭제 액션을 제공하고, 작업의 성공/실패를 명확한 피드백(성공 알림, 오류 사유)으로 표시해야 한다. UI 는 노드의 승인/온라인/노출 상태에 따라 편집·생성·삭제 액션을 게이팅(비활성/숨김)해야 한다(REQ-I05 일관).

**REQ-REMOTE-I11**: 편집 실패 의미 (offline/timeout/apply-fail)
시스템은 **항상** 원격 편집 엔드포인트의 실패를 다음과 같이 구분된 의미로 반환해야 한다: 대상 노드 오프라인 → **503**(Service Unavailable), 명령 타임아웃(REQ-D06) → **504**(Gateway Timeout), 노드 어댑터 적용 실패(REQ-D09, 검증/충돌) → **502**(Bad Gateway, 노드 오류 사유 전달). 어느 실패에서도 서버 미러 캐시는 **갱신되지 않아야** 한다(성공 결과 수신 시에만 갱신 — REQ-E08).

**REQ-REMOTE-I12**: 원격 편집 감사
시스템은 **항상** 원격 자원 생성/수정/삭제(누가/언제/어느 노드/어느 자원/어떤 action)를 감사 로그로 기록해야 한다(REQ-F05 일관). 감사 기록에는 시크릿 값(마스킹/실제 모두)을 포함하지 않아야 한다(REQ-F06).

### 4.7d 그룹 J — 원격 노드 FULL 제어 패리티 (Remote Node Full Control Parity) — v1.3 확장

> **범위(v1.3)** — 본 그룹은 원격 노드의 플로우/에이전트/디바이스를 **별도 원격 페이지가 아니라 로컬과 동일한 웹 UI**(로컬 목록 페이지 + 상세 패널)로 제어하는 FULL 패리티를 추가한다. 목록·라이프사이클(이미 그룹 D/E/I 로 가능)을 넘어 **상세 패널까지** 패리티를 확장한다: 에이전트 통계/설정/디바이스/토픽/store/세션/시리즈, 디바이스 실시간 상태+명령+메타데이터, 플로우 노드 레벨 런타임/로그. 이를 위해 미러(그룹 E, 요약 정의만)·명령(그룹 D, 변경 전용)과 **구분되는 신규 READ/QUERY 프록시**(서버↔노드)를 신설한다 — 그룹 D 명령(command)과 **대칭**되는 **per-domain query-action** 집합(예: flow=get/nodes/status/logs; agent=get/stats/config/devices/topics/store/sessions/series; device=get/state/commands/metadata)을 `query`/`query_result` RPC 로 운반하고, 노드가 각 query-action 을 자신의 로컬 read 핸들러에 매핑하여 라이브 JSON 을 반환한다(**READ-ONLY**). raw HTTP path 프록시가 아니라 **열거된 query-action** 만 지원하며, allowlist 는 도메인별 지원 query-action 집합이다(§5.10 OQ-J1 RESOLVED = per-domain query-action). 추가로, 노드의 실시간 데이터(디바이스 실시간 상태, 에이전트 라이브 통계/시리즈)는 **스트리밍 프록시**(subscribe/stream_data/unsubscribe)로 서버를 경유해 브라우저로 중계한다(§5.10 OQ-J3 RESOLVED = 스트리밍 포함; 폴링은 폴백). 서버는 query-action 응답을 **단기 TTL 캐시**하여 반복 폴 부하를 줄인다(§5.10 OQ-J6 RESOLVED = 처음부터 캐시; 스트리밍/라이브 액션은 캐시 우회). UI 는 `useEditorFlowTarget` 의 **target 추상화**(local | remote:{instanceId})를 목록/제어 페이지로 확장(`useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget`)하여 로컬 페이지를 원격에 동일 코드로 재사용한다. 변경(라이프사이클/CRUD/디바이스 명령·메타데이터)은 **여전히 그룹 D 명령 / 그룹 I(M7) CRUD 경로로만** 수행하며, **프록시(query/스트림) 경유 변경은 금지**한다.

#### J-a. READ/QUERY 프록시 (서버↔노드)

**REQ-REMOTE-J01**: per-domain query-action READ 프록시 RPC
시스템은 **항상** 서버가 승인·온라인 노드에 read 질의를 전달하고 노드의 응답을 돌려받는 요청/응답 RPC(`query`/`query_result`)를 라이브 WS 세션 위로 제공해야 한다. 질의는 **그룹 D 명령(command)과 대칭되는 per-domain query-action** 으로 표현되며(`{domain, query_action, args}`), 노드는 각 query-action 을 **자신의 기존 로컬 read 핸들러에 매핑**하여 결과 JSON 을 반환해야 한다(권위는 노드 — A4/A10 일관). raw HTTP path 프록시는 사용하지 않는다(OQ-J1 RESOLVED). 본 RPC 는 미러(그룹 E)·명령(그룹 D)과 **별개**이다(A11).

**REQ-REMOTE-J02**: 질의 페이로드 형태·상관·타임아웃 ({domain, query-action, args})
시스템은 **항상** 질의 페이로드를 `{domain(flow/agent/device), query_action, args}` 형태로 표현하고(raw GET path 아님), 각 질의에 **상관 id**(query_id)를 부여하여 요청-응답을 1:1 매칭하며(REQ-D07 패턴 준용), **제한 시간** 내 응답이 없으면 타임아웃 처리해야 한다(REQ-D06 패턴 준용). `query_result` 는 `query_id`, 상태(코드), 본문(JSON, redacted) 또는 오류를 운반해야 한다.

**REQ-REMOTE-J03**: READ-ONLY 강제 (변경은 그룹 D/I 경로만)
시스템은 **항상** READ/QUERY 프록시(query + 스트림)를 **읽기 전용**으로 제한해야 한다. 어떤 상태 변경(생성/수정/삭제/명령 실행/메타데이터 변경/라이프사이클)도 프록시를 통해 수행되어서는 안 되며, 모든 변경은 그룹 D 명령 / 그룹 I(M7) CRUD 경로로만 전파되어야 한다. 변경 의미를 갖는 query-action(또는 미열거 action)은 프록시에서 거부되어야 한다.

**REQ-REMOTE-J04**: 지원 query-action allowlist (도메인별 열거 집합 — FULL 커버리지)
시스템은 **항상** 도메인별로 **열거된 read query-action 집합**만 허용해야 하며(임의 action 차단 — allowlist = 지원 query-action 의 enumerated set), 모든 로컬 상세 패널을 원격에서 동작시키기에 충분한 **FULL 커버리지**를 제공해야 한다(OQ-J2 RESOLVED). 지원 query-action 은 다음을 포함한다:
> - **flow**: `list`, `get`(상세 정의), `status`, `nodes`(노드 목록), `node`(단일 노드 런타임), `logs`(노드/플로우 로그).
> - **agent**: `list`, `get`(상세, detail=full), `stats`(통계), `config`(설정), `devices`(연결 디바이스), `topics`(토픽), `store`(store query/keys/tags/key), `sessions`(세션), `series`(시리즈/TSDB).
> - **device**: `list`, `get`, `state`(실시간 상태), `commands`(명령 스펙), `metadata`(읽기).
> 각 query-action 은 노드 측에서 대응 로컬 read 핸들러(`/flows/{id}`·`/flows/{id}/status`·`/flows/{id}/nodes[/{nodeID}]`·`/agents/{id}[?detail=full]`·`/agents/{id}/stats`·`/store/{agent}/*`·`/tsdb/*`·`/devices/{ref}` 등)로 매핑된다(매핑은 5.10.1 참조). 목록(`list`)은 **그룹 E 미러를 우선**하되 라이브 목록이 필요한 경우 query-action 으로 보강할 수 있다. **변경 의미 action**(create/update/delete/execute/metadata-write/start/stop/config 등)은 query allowlist 에서 **명시 배제**된다(REQ-J03 — 변경은 그룹 D/I).

**REQ-REMOTE-J05**: 프록시 게이팅 (승인·온라인·노출 범위)
시스템은 **항상** READ/QUERY 프록시를 **승인(approved)** 이고 **온라인(online)** 인 노드에 대해서만 허용해야 한다. 추가로, 질의 대상 자원은 해당 노드의 **노출 범위(exposure scope)** 내에 있어야 하며(REQ-E07/A06 일관), 노출되지 않은 자원에 대한 질의는 거부되어야 한다(서버는 노출된 것만 질의 가능). 미승인/오프라인/범위 밖 질의는 프록시하지 않고 명확한 오류를 반환해야 한다.

**REQ-REMOTE-J06**: 프록시 응답 redaction (전송 전 노드 마스킹)
시스템은 **항상** 프록시된 응답의 시크릿 필드를 **노드가 전송 전에** 기존 redaction 정책(`secret_fields.go`, REQ-F06 일관)으로 마스킹/제외해야 한다. 미러(그룹 E)와 동일한 redaction 정책이 라이브 프록시 응답에도 적용되어, 시크릿이 와이어로 평문 전송되지 않아야 한다.

**REQ-REMOTE-J07**: 프록시 실패 의미 (offline/timeout/node-error)
시스템은 **항상** READ/QUERY 프록시 실패를 기존 의미와 일관되게 반환해야 한다: 대상 노드 오프라인 → **503**(Service Unavailable), 질의 타임아웃(REQ-J02) → **504**(Gateway Timeout), 노드 측 질의 오류 → **502**(Bad Gateway, 노드 오류 사유 전달). (그룹 I REQ-I11 의 503/504/502 매핑과 동일.)

**REQ-REMOTE-J08**: 스트리밍 프록시 (서버 경유 라이브 push 중계)
시스템은 **항상** 노드의 실시간 데이터(디바이스 실시간 상태, 에이전트 라이브 통계/시리즈)를 **스트리밍 프록시**로 서버를 경유해 브라우저로 중계해야 한다(OQ-J3 RESOLVED — 스트리밍 포함). 스트리밍은 WS 세션 위 `subscribe`/`stream_data`/`unsubscribe` RPC 로 구성되며, 각 구독에 **상관 id**(subscription_id)를 부여해 다중 구독을 구분해야 한다. 노드는 구독된 실시간 소스의 갱신을 `stream_data` 로 push 하고, 서버는 이를 해당 브라우저 세션으로 fan-out 한다. 스트리밍도 **READ-ONLY** 이며(REQ-J03), 응답 데이터는 전송 전 redaction 된다(REQ-J06). **폴링은 폴백**으로 유지된다(스트림 미지원/실패 시 query-action 폴링으로 대체).

**REQ-REMOTE-J08b**: 스트리밍 라이프사이클·teardown·백프레셔
시스템은 **항상** 스트리밍 구독의 라이프사이클을 안전하게 관리해야 한다: (a) 노드 연결 종료/노드 오프라인 시 해당 노드의 모든 구독을 **teardown**(자동 unsubscribe·정리)해야 하며, (b) 브라우저 세션 종료/`unsubscribe` 시 노드로 unsubscribe 를 전파해 노드 측 소스 구독을 해제해야 하고, (c) 소비자가 느린 경우 **백프레셔**(최신값 우선 drop/coalesce 또는 전송 속도 제한)로 서버/노드 메모리 폭증을 방지해야 한다. 구독 누수(leak)가 없어야 한다.

#### J-b. 통합 UI (로컬 페이지 재사용)

**REQ-REMOTE-J09**: 자원 타깃 추상화 (useFlowsTarget/useAgentsTarget/useDevicesTarget)
시스템은 **항상** `useEditorFlowTarget` 패턴을 확장한 **자원 타깃 추상화**(`useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget`)를 제공하여, `target`(local | remote:{instanceId})에 따라 (1) 데이터 소스(로컬 API vs READ/QUERY 프록시)와 (2) 액션 엔드포인트(로컬 라이프사이클/CRUD vs 그룹 D 명령 / 그룹 I(M7) CRUD)를 **투명하게 전환**해야 한다. 페이지/패널 본문은 타깃에 무관하게 동일 코드로 동작해야 한다.

**REQ-REMOTE-J10**: 로컬 목록 페이지 + 상세 패널 재사용
시스템은 **항상** 로컬 목록 페이지(`FlowListPage`/`AgentListPage`/`DeviceListPage`)와 그 상세 패널(`FlowDetailPanel`/`AgentDetailPanel`/`DeviceDetailPanel`)이 `target` 을 입력으로 받아 **원격에서도 동일하게 렌더링**하도록 해야 한다. 별도 원격 전용 목록/상세 화면을 신설하지 않는다(로컬 코드 재사용).

**REQ-REMOTE-J11**: 상세 패널 FULL 패리티 (상세/통계/상태/시리즈/노드 레벨)
시스템은 **항상** 원격 타깃의 상세 패널이 **로컬과 동일한** 리치 데이터를 표시하도록 해야 한다: 에이전트 통계/설정/연결 디바이스/토픽/store/세션/시리즈, 디바이스 실시간 상태·명령 스펙·메타데이터, 플로우 상태·노드 레벨 런타임/로그. 이들 데이터는 REQ-J04 의 도메인별 query-action(정적/온디맨드) 및 REQ-J08 의 스트리밍(실시간)으로 노드에서 라이브 취득해야 한다.

**REQ-REMOTE-J12**: 통합 액션 레이어 — 변경은 그룹 D/I 라우팅
시스템은 **항상** 원격 타깃의 모든 변경 액션을 그룹 D 명령(라이프사이클: 플로우 deploy/start/stop/pause, 에이전트 start/stop/restart, 디바이스 명령 `execute`·메타데이터 `metadata`) 또는 그룹 I(M7) CRUD(플로우/에이전트 생성·수정·삭제) 경로로 라우팅해야 한다. **디바이스 쓰기(명령/메타데이터)는 그룹 D 명령 경로로** 처리하며(신규 프록시 경유 금지 — REQ-J03), 통합 액션 레이어가 타깃에 따라 로컬 vs 그룹 D/I 호출을 선택해야 한다.

**REQ-REMOTE-J13**: 노드 셀렉터 + target 쿼리 라우팅 + 네비게이션
시스템은 **항상** 사용자가 원격 노드를 선택할 수 있는 **노드 셀렉터**를 제공하고, 선택 시 `?target=remote:{instanceId}`(미지정/`local` 은 로컬) 쿼리 파라미터로 동일 목록/제어 페이지를 라우팅해야 한다. 사이드바/네비게이션은 현재 타깃(로컬/원격 노드 이름)을 명확히 표시하고 타깃 간 전환을 제공해야 한다.

**REQ-REMOTE-J14**: 기존 원격 자원 페이지 재용도화/폐기
시스템은 **항상** 기존 별도 원격 자원 페이지(`RemoteResourcesPage`)를 **노드 셀렉터로 재용도화하거나 폐기**해야 한다. 원격 노드의 자원 제어는 재사용된 로컬 페이지(`?target=remote:{instanceId}`)로 일원화되어, 로컬/원격 제어 UX 가 분기되지 않아야 한다.

**REQ-REMOTE-J15**: 프록시 접근 최소 감사 (일반 read 미감사)
시스템은 **항상** 일반 read 질의/스트림 접근은 **감사하지 않아야** 하며(정상 읽기는 미기록), **노출 위반(범위 밖 질의 시도)·오류 접근(거부/실패)만** 로깅해야 한다(OQ-J5 RESOLVED — 최소 감사). 변경(그룹 D/I)은 기존 감사를 유지한다(REQ-F05/I12). 어떤 로그/감사에도 시크릿 값(마스킹/실제 모두)을 포함하지 않아야 한다(REQ-F06 일관).

**REQ-REMOTE-J16**: 단기 TTL 프록시 응답 캐시 (처음부터)
시스템은 **항상** 서버에서 query-action 응답을 **단기 TTL 로 캐시**하여 반복 폴로 인한 노드 부하를 줄여야 한다(OQ-J6 RESOLVED — v1 부터 캐시, pass-through 아님). 명세:
> - **기본 TTL**: 소(小) 기본값(예: 1~2초 수준의 짧은 TTL — 정확한 값은 구현 시 확정하되 "단기" 원칙 고정). 캐시 키 = `{instance_id, domain, query_action, args}`.
> - **per-action 캐시 가능성**: 정적/완만 변동 데이터(get/config/nodes/commands/metadata 등)는 캐시 대상, **스트리밍/라이브 action(state/stats/series 등 실시간)은 캐시를 우회**(항상 노드/스트림 직격)한다.
> - **무효화**: 동일 노드/자원에 대한 관련 변경(그룹 D 명령 / 그룹 I CRUD)이 성공하면 해당 캐시 항목을 **무효화**하여 stale 데이터를 방지한다.
> - redaction 된 본문만 캐시하며(REQ-J06 일관), 게이팅(REQ-J05)은 캐시 적중 시에도 매 요청 평가된다.

### 4.7e 그룹 K — 원격 관리 UI 재편 + 노드 그룹핑 + 노드 대시보드 (UI IA + Node Grouping + Node Dashboard) — v1.4 확장

> **범위(v1.4)** — 본 그룹은 (A) 노드 그룹핑(백엔드+API+UI), (B) 노드 시스템 정보 보고(BASIC), (C) 노드별 운영 요약, (D) 사이드바 정보 구조(IA) 재편을 추가한다. 사이드바 `원격 관리` 그룹을 **두 최상위 진입점**으로 재편한다: **`노드 관리`(운영)** = 디렉토리 뷰(단일 레벨 그룹 트리, "전체" 기본) + 노드 선택 시 **노드 대시보드**(시스템 정보 BASIC + 운영 요약 + Flow/Agent/Device 서브탭이 M8 통합 제어 `target=remote:{instanceId}` 재사용); **`등록 관리`(온보딩)** = 토큰 관리(EnrollmentTokenSection) + 노드 등록 관리(pending 승인 큐·승인/거부/폐기·사전 등록). 이 재편은 기존 `관리 노드`(RemoteNodesPage) + `원격 노드 제어`(RemoteControlPage) 진입점을 **대체**한다. 그룹핑은 **서버 측 운영 메타데이터**(A13)이며 노드 권위 정의(A4)·미러(그룹 E)·명령(그룹 D)과 무관하다. 시스템 정보는 **BASIC** 만(자원 메트릭 제외), 운영 요약은 **미러 파생**(A15)이다.

#### K-a. 노드 그룹핑 (백엔드 + API)

**REQ-REMOTE-K01**: 단일 그룹 라벨 영속
시스템은 **항상** 각 관리 노드에 대해 **단일 그룹 라벨**(`managed_nodes.group_name`, 자유 입력 문자열, 기본 빈값)을 영속해야 한다. 한 노드는 **최대 하나의 그룹**에만 속하며(다중 소속 없음), 그룹 라벨이 빈값인 노드는 기본 버킷 **"전체"(All)** 에 속하는 것으로 간주된다. 그룹은 단일 레벨(계층 없음)이다.

**REQ-REMOTE-K02**: 그룹 배정/해제
시스템은 **항상** 관리자가 노드의 그룹을 배정·변경·해제할 수 있도록(`PUT /api/v1/remote/nodes/{instance_id}/group`, 본문 `{group_name}`; 빈 문자열 또는 `DELETE` 는 그룹 해제="전체"로 환원) 제공해야 한다. 배정은 `managed_nodes.group_name` 갱신으로 즉시 반영되어야 하며, 노드로 명령을 전파하지 않아야 한다(서버 운영 메타데이터 — A13, 그룹 D 비경유).

**REQ-REMOTE-K03**: distinct 그룹 목록
시스템은 **항상** 현재 사용 중인 distinct 그룹 라벨 목록(`GET /api/v1/remote/groups`)을 제공해야 하며, 응답은 항상 기본 버킷 **"전체"** 를 포함하고 각 그룹의 노드 수를 함께 제공할 수 있어야 한다(별도 그룹 엔티티 없이 `group_name` distinct 집계).

**REQ-REMOTE-K04**: 그룹별 노드 목록 (전체 기본 버킷)
시스템은 **항상** 노드를 그룹별로 묶은 목록(`GET /api/v1/remote/nodes?group_by=group` 또는 그룹화 응답)을 제공해야 한다. 그룹 미지정(빈 라벨) 노드는 **"전체"** 버킷에 모이고, 각 노드 항목은 online/status/시스템 정보 요약을 포함해야 한다.

**REQ-REMOTE-K05**: 그룹 이름 변경·삭제 의미 (재라벨링·환원)
시스템은 **항상** 그룹 이름 변경을 해당 그룹에 속한 노드들의 `group_name` **재라벨링**으로 처리해야 한다(별도 그룹 엔티티가 없으므로 그룹 자체를 수정하지 않음). 그룹 비우기/삭제는 해당 그룹 노드들의 `group_name` 을 빈값으로 되돌려 **"전체"로 환원**해야 한다. 빈 그룹(구성원 0)은 자동으로 distinct 목록에서 사라진다.

**REQ-REMOTE-K06**: 그룹 작업 admin 게이팅
시스템은 **항상** 모든 그룹 배정/해제/목록 엔드포인트를 기존 `/remote/*` admin 인증으로 게이팅해야 한다(REQ-F04 일관). 비-관리자 접근은 거부되어야 한다.

#### K-b. 노드 시스템 정보 보고 (BASIC)

**REQ-REMOTE-K07**: 시스템 정보 보고 페이로드 확장 (BASIC)
시스템은 **항상** 노드가 자신의 BASIC 시스템 정보(`os`, `arch`, `version`, `started_at`(epoch ms))를 서버에 보고하도록, `register`(RegisterPayload/HelloPayload) **및/또는** `heartbeat`(HeartbeatPayload) 페이로드를 확장해야 한다. 자원 메트릭(CPU/메모리/디스크)은 보고하지 않아야 한다(본 마일스톤 제외).

**REQ-REMOTE-K08**: 시스템 정보 저장·노출
시스템은 **항상** 보고된 시스템 정보를 `managed_nodes`(`os`, `arch`, `started_at` 컬럼; `hostname`/`version` 은 기존 컬럼 재사용)에 저장하고, 노드 상세/목록 API 로 노출해야 한다. **uptime** 은 `started_at` 과 서버 현재 시각의 차로 산출하여 제공해야 한다(별도 uptime 필드 저장 불필요). online/last_seen/status 는 기존 추적값(REQ-B05/E06)을 재사용한다.

**REQ-REMOTE-K09**: 시스템 정보 하위 호환
시스템은 **항상** 시스템 정보 필드를 보내지 않는(구버전) 노드도 등록·관리가 정상 동작하도록 보장해야 한다. 미보고 필드는 빈값/미표시로 처리되며, 기존 동작에 회귀를 일으키지 않아야 한다(REQ-N03 일관 — 필드 선택적).

#### K-c. 노드별 운영 요약 (미러 파생)

**REQ-REMOTE-K10**: 노드 운영 요약 파생·노출
시스템은 **항상** 노드별 운영 요약(플로우 카운트 + running/stopped 분해, 에이전트 카운트 + connected 분해, 디바이스 카운트 + online 분해)을 **기존 미러(그룹 E) 데이터에서 파생**하여 제공해야 한다(`GET /api/v1/remote/nodes/{instance_id}` 상세 응답에 포함 또는 `.../summary`). 신규 노드 왕복 질의보다 미러 집계를 우선해야 하며(A15), 노드 오프라인 시에도 last-known 미러로 요약을 제공해야 한다(REQ-E06 일관).

#### K-d. UI 재구성 (프론트엔드)

**REQ-REMOTE-K11**: 사이드바 IA 재편 (노드 관리 + 등록 관리)
시스템은 **항상** 사이드바 `원격 관리` 그룹을 **두 최상위 진입점** — **`노드 관리`(운영)** 와 **`등록 관리`(온보딩)** — 로 구성하여, 기존 `관리 노드`(RemoteNodesPage) + `원격 노드 제어`(RemoteControlPage) 진입점을 **대체**해야 한다. 두 진입점은 기존 admin 권한 + server 모드 게이팅(현행 사이드바 게이팅)을 유지해야 한다.

**REQ-REMOTE-K12**: 노드 관리 — 디렉토리 뷰 (단일 레벨 그룹 트리)
시스템은 **항상** `노드 관리` 페이지에서 **단일 레벨 디렉토리 뷰**(그룹 → 노드)를 제공해야 한다. 그룹은 단일 레벨로 표시되고, 그룹 미지정 노드는 기본 **"전체"** 그룹 아래 표시되며, 각 노드 항목은 online/last-seen·status 표식을 보여야 한다. 디렉토리 뷰에서 노드의 그룹 배정/해제(단일 그룹) affordance 를 제공해야 한다(REQ-K02 소비).

**REQ-REMOTE-K13**: 노드 관리 — 노드 대시보드 (시스템 + 운영 + 서브탭)
시스템은 **항상** 디렉토리 뷰에서 노드를 선택하면 **노드 대시보드**를 표시해야 한다. 대시보드는 (a) **시스템 정보(BASIC)** — hostname, OS, arch, version, uptime(started_at 파생), online/last-seen, status, (b) **운영 요약** — 플로우 요약(카운트+running/stopped), 에이전트 요약(카운트+connected), 디바이스 요약(카운트+online)(REQ-K10 소비), (c) **Flow/Agent/Device 서브탭** — 기존 **M8 통합 제어 페이지**(`target=remote:{instanceId}`, 그룹 J FULL 패리티)를 재사용 — 을 포함해야 한다.

**REQ-REMOTE-K14**: 노드 대시보드 서브탭 = M8 통합 제어 재사용
시스템은 **항상** 노드 대시보드의 Flow/Agent/Device 서브탭이 별도 원격 전용 화면을 신설하지 않고 **기존 M8 통합 제어**(로컬 `FlowListPage`/`AgentListPage`/`DeviceListPage` + 상세 패널을 `target=remote:{instanceId}` 로 재사용, REQ-J09~J14)로 라우팅·렌더링되도록 해야 한다. 변경은 여전히 그룹 D 명령 / 그룹 I CRUD 경로로만 수행된다(REQ-J03/J12 일관).

**REQ-REMOTE-K15**: 등록 관리 — 토큰 관리 + 노드 등록 관리
시스템은 **항상** `등록 관리` 페이지에서 (a) **토큰 관리** — enrollment 토큰 발급/목록/폐기(기존 `EnrollmentTokenSection`, REQ-H03/H04 소비) 와 (b) **노드 등록 관리** — pending 승인 큐 + 승인/거부/폐기 + 사전 등록(기존 RemoteNodesPage 의 승인 UI·`PreRegisterNodeDialog`, REQ-C02/C03/C07/H01 소비) 을 제공해야 한다.

**REQ-REMOTE-K16**: IA 마이그레이션 (기존 진입점 대체, 딥링크 보존)
시스템은 **항상** 기존 `관리 노드`(`/admin/remote`) + `원격 노드 제어`(`/admin/remote/control`) 진입점을 새 `노드 관리`/`등록 관리`로 **마이그레이션(대체)** 하되, 기존 M8 통합 제어 페이지(`/flows|agents|devices?target=remote:{id}`)와 원격 편집기 라우트(`/admin/remote/nodes/{id}/flows/...`)는 새 `노드 관리`(노드 대시보드 서브탭)에서 **도달 가능**하게 유지해야 한다. 로컬/원격 제어 UX 가 분기되지 않아야 한다(REQ-J14 일관).

### 4.7f 그룹 L — 원격 노드 대시보드 패리티 (Remote Node Dashboard Parity) — v1.5 확장

> **범위(v1.5)** — 본 그룹은 관리자가 서버에서 승인·온라인 원격 노드의 **대시보드를 로컬과 동일한 `DashboardPage`**(`web/src/pages/dashboard/DashboardPage.tsx`)로 `target=remote:{instanceId}` 파라미터화하여 보고 제어하는 패리티를 추가한다. 이는 가장 무거운 원격 영역으로, **그룹 J(M8) query/스트림 프록시 + target 추상화**(`useResourceTargets`/`useDetailTargets`/`useTargetParam`/`useTargetGating`/`useRemoteStream`)를 **재사용**한다. 구성: (L-a) **대시보드 config 읽기 프록시** — 노드의 대시보드 config(SPEC-DASHBOARD-001 `dashboardPages[]`/`activeDashboardId`/grid/refresh/`deviceGridLayout`)를 그룹 J `dashboard` 도메인 query-action(`get_shared`/`get_mine`)으로 **READ-ONLY** 취득. (L-b) **패널 데이터 소스 target-aware** — 약 10종 패널의 데이터 바인딩을 로컬 vs 원격으로 해석: flows/agents/devices/single-device(기존 target 훅 재사용), 자원 메트릭(신규 `monitor/metrics` query-action), 로그(신규 `monitor/logs` 스트림 action), **차트(M8 스트림 프록시 `chart` stream-action 추가)**, control 패널(useDeviceDetailTarget + 그룹 D 명령). (L-c) **디바이스/자원 ref 네임스페이싱** — 원격 target 하 패널 device ref 는 그 노드 기준 해석(config 노드-로컬). (L-d) **UI** — 노드 대시보드(M9)에 대시보드 서브탭 및/또는 `DashboardPage target=remote:{id}`. **변경 경로 불변**: 모든 변경(플로우 라이프사이클·에이전트 start/stop·디바이스 명령/메타데이터)은 그룹 D 명령 / 그룹 I CRUD 로만(프록시 경유 변경 금지 — REQ-J03/J12 일관). 원격 대시보드 config 편집은 **v1.5 비목표**(OQ-L3). **OQ-L1~L6 ✅ 전부 RESOLVED(2026-06-08, 사용자 권고안대로 확정) — 구현 시 고정 제약이다.**

#### L-a. 대시보드 config 읽기 프록시 (서버↔노드)

**REQ-REMOTE-L01**: 대시보드 config query-action 추가 (READ-ONLY)
시스템은 **항상** 그룹 J query-action allowlist(REQ-J04)에 **`dashboard` 도메인**과 read query-action(`get_shared`, `get_mine`)을 추가하여, 노드의 대시보드 config(SPEC-DASHBOARD-001 `GET /dashboards/shared` · `GET /dashboards/mine`)를 **READ-ONLY** 로 취득할 수 있어야 한다. 노드는 각 query-action 을 자신의 로컬 대시보드 read 핸들러로 매핑하여 config JSON(`dashboardPages[]`, `activeDashboardId`, grid cols, refresh interval, `deviceGridLayout`)을 반환해야 한다. 변경 의미 action(`put`/`delete`)은 allowlist 에서 **명시 배제**된다(REQ-J03 — 원격 config 편집 비목표).

**REQ-REMOTE-L02**: 대시보드 config 게이팅·redaction·실패 의미
시스템은 **항상** 대시보드 config query-action 에 그룹 J 의 게이팅(승인 ∧ 온라인 ∧ 노출 범위, REQ-J05)·redaction(노드가 전송 전 마스킹, REQ-J06)·실패 의미(offline→503, timeout→504, node-error→502, REQ-J07)·단기 TTL 캐시(REQ-J16, config 는 완만 변동이므로 캐시 대상)를 동일하게 적용해야 한다. 대시보드 config 가 노드에 존재하지 않으면(미설정) **404** 또는 빈 config 로 명확히 응답해야 한다.

**REQ-REMOTE-L03**: 대시보드 config 노드-로컬 권위·deviceId 네임스페이싱
시스템은 **항상** 원격 대시보드 config 와 그 내부 자원 참조(`deviceId`, 패널 config)를 **해당 노드 기준**으로 해석해야 한다(A17). 서버는 config 의 권위 소유자가 아니며(노드 권위 — A4), config 를 서버 측에 영속 저장하지 않고(미러 비대상) 프록시 취득·단기 캐시만 한다. config 내부 deviceId 는 서버 자신의 디바이스가 아니라 **그 노드의 IoT 디바이스**를 가리킨다(혼동 금지 — §1.4).

#### L-b. 패널 데이터 소스 target-aware

**REQ-REMOTE-L04**: 자원/플로우/에이전트/디바이스 패널 target-aware
시스템은 **항상** flows/agents/devices/single-device 패널(`FlowPanel`/`AgentPanel`/`DevicePanel`/`SingleDevicePanel`)이 dashboard 레벨 `target` 을 받아 **기존 그룹 J target 추상화**(`useResourceTargets`/`useDetailTargets`)로 로컬 vs 원격(query-action/스트림)을 해석하도록 해야 한다. 신규 데이터 경로를 만들지 않고 M8 자산을 재사용해야 한다.

**REQ-REMOTE-L05**: 자원 메트릭(resource 위젯) 원격 query-action
시스템은 **항상** resource 위젯(`ResourceWidget`, 로컬 `GET /monitor/metrics`)의 원격 데이터를 그룹 J query-action 으로 취득하도록, query-action allowlist 에 **`monitor` 도메인 `metrics`**(read)를 추가해야 한다. 노드는 이를 자신의 `/monitor/metrics` read 핸들러로 매핑해 메트릭 스냅샷 JSON 을 반환하고, 서버는 단기 TTL 캐시(REQ-J16)·게이팅(REQ-J05)·실패 의미(REQ-J07)를 적용해야 한다. 자원 메트릭의 **장기 시계열/상시 폴링은 신설하지 않는다**(대시보드 패널 표시 목적의 온디맨드 read 만 — 그룹 K BASIC 비목표 일관).

**REQ-REMOTE-L06**: 로그 패널 원격 스트림 action
시스템은 **항상** logs 패널(`LogPanel`, 로컬 로그 WS 스트림)의 원격 데이터를 그룹 J **스트림 프록시**로 중계하도록, 스트림 action allowlist 에 **`monitor` 도메인 `logs`**(stream)를 추가해야 한다. 노드는 자신의 로그 스트림 소스를 구독하여 `stream_data` 로 push 하고, 서버가 브라우저 세션으로 fan-out(teardown·백프레셔 — REQ-J08b)해야 한다. 로그는 라이브 스트림이므로 캐시를 우회한다(REQ-J16). 스트림 미지원/실패 시 query-action 폴백을 따른다(REQ-J08).

**REQ-REMOTE-L07**: 차트 패널 원격 스트림 — M8 스트림 프록시 chart-action 확장
시스템은 **항상** 차트 패널(line/stat/bar/pie/table, `useChartChannel`/`useChartChannels`, 로컬 `/ws/chart/{channelName}`)의 라이브 데이터를 **별도 WS 프록시 경로 신설 없이** 그룹 J 스트림 프록시에 **`chart` stream-action**(subscribe/stream_data/unsubscribe, args=`{channelName}`)을 추가하여 서버 경유로 중계해야 한다(OQ-L2 RESOLVED). 노드는 자신의 `/ws/chart/{channel}` 소스를 구독해 backfill/append 이벤트를 `stream_data` 로 push 하고, 서버가 fan-out 해야 한다. teardown·백프레셔(REQ-J08b)·redaction(REQ-J06)·캐시 우회(REQ-J16)를 동일 적용하며, 스트림 미지원/실패 시 폴링 폴백(스냅샷 query-action)을 따른다(REQ-J08). `useChartChannel` 의 `wsBaseUrl`/`createClient` 주입점을 이용해 원격 target 에서 스트림 프록시(SSE) 어댑터를 주입하여 동일 차트 패널 코드가 동작해야 한다.

**REQ-REMOTE-L08**: control 패널 원격 명령 라우팅 (그룹 D)
시스템은 **항상** control 패널(`AcControlPanel`/`HvacControlPanel`/`OutdoorControlPanel`, 로컬 `useDeviceRealtime` + `useExecuteCommand`)의 원격 동작을 (a) 실시간 상태 read 는 그룹 J `device.state`(useDeviceDetailTarget 재사용), (b) 명령 쓰기는 **그룹 D 명령**(REQ-D04 디바이스 명령 경로)으로 라우팅해야 한다. 디바이스 쓰기를 프록시(query/스트림) 경유로 수행해서는 안 된다(READ-ONLY 강제 — REQ-J03/J12). control 패널 config 의 bare `deviceId` 는 원격 target 하에서 **그 노드의 디바이스**로 해석되어 그 노드로 명령이 디스패치되어야 한다(REQ-L03 네임스페이싱).

**REQ-REMOTE-L09**: 패널 target 전파 (TargetProvider/prop)
시스템은 **항상** `DashboardPage`/`renderPanel` 이 dashboard 레벨 `target`(local | remote:{instanceId})을 **TargetProvider(context) 또는 패널 prop** 으로 각 패널에 전파하도록 해야 한다. `renderPanel` 은 현재 패널에 target 을 넘기지 않으므로 이를 확장하되, **target 미지정/`local` 시 패널 렌더는 로컬과 바이트 동일**(회귀 0 — A16)해야 한다. control 패널 config 의 bare `deviceId` 등 자원 참조는 target 컨텍스트로 노드 네임스페이스를 해석한다(REQ-L03).

#### L-c. UI 통합 & 게이팅

**REQ-REMOTE-L10**: 노드 대시보드 — 대시보드 서브탭 / DashboardPage 원격 target
시스템은 **항상** 노드 대시보드(M9 `NodeDashboard`)에 **대시보드 서브탭**을 추가하거나(OQ-L5 RESOLVED — 서브탭 1차 진입) `DashboardPage` 를 `target=remote:{instanceId}` 로 라우팅하여, 관리자가 원격 노드의 대시보드를 로컬과 동일한 `DashboardPage` 로 열 수 있도록 해야 한다. 서브탭은 별도 원격 대시보드 화면을 신설하지 않고 **로컬 `DashboardPage` 를 재사용**해야 한다(A16). 대시보드 config 는 REQ-L01 의 `dashboard` query-action 으로 노드-로컬 취득한다.

**REQ-REMOTE-L11**: 원격 대시보드 게이팅·실패 의미·원격 컨텍스트 표시
시스템은 **항상** 원격 대시보드를 **승인 ∧ 온라인 ∧ 노출 범위** 노드에 대해서만 표시·동작시켜야 하며(REQ-J05 일관, `useTargetGating` 재사용), 실패를 구분된 의미로 처리해야 한다: 노드 오프라인→503, 프록시/스트림 타임아웃→504, 노드 측 오류→502, 대시보드 config 미설정→404(또는 빈 대시보드). UI 는 **현재 원격 노드 컨텍스트**(노드 이름·원격 표식)를 다른 원격 뷰(M8/M9)와 일관되게 표시해야 한다(최근 IA — 노드 디렉토리 + 대시보드 서브탭).

**REQ-REMOTE-L12**: read-only-by-default + 변경 경로 불변
시스템은 **항상** 원격 대시보드를 **기본 READ-ONLY** 로 동작시켜야 한다: 원격 노드의 **대시보드 config 편집/저장/생성/삭제는 비활성/숨김**(v1.5 비목표 — OQ-L3). 다만 패널이 표시하는 **자원에 대한 제어 액션**(플로우 라이프사이클, 에이전트 start/stop/restart, 디바이스 명령·메타데이터)은 **기존 그룹 D 명령 / 그룹 I CRUD 경로로만** 수행되어야 하며(신규 변경 경로·프록시 경유 변경 금지 — REQ-J03/J12), control 패널 명령은 REQ-L08 의 그룹 D 라우팅을 따른다.

**REQ-REMOTE-L13**: 원격 대시보드 접근 최소 감사
시스템은 **항상** 원격 대시보드의 일반 read(config 취득·패널 query/스트림)는 **감사하지 않아야** 하며(정상 읽기 미기록 — REQ-J15 일관), 노출 위반·오류 접근만 로깅해야 한다. control 패널 등에서 발생하는 변경(그룹 D/I)은 기존 감사를 유지하고(REQ-F05/J15), 어떤 로그/감사에도 시크릿 값을 포함하지 않아야 한다(REQ-F06).

### 4.7g 그룹 M — 관리자 뷰 & 노드 화면 충실 재현 (Manager View & Faithful Node Screen Reproduction) — v1.6 확장

> **범위(v1.6)** — 본 그룹은 원격 관리 UI 를 노드의 화면을 **충실히 재현**하는 전용 **관리자 뷰**로 재구성한다. xflowd 서버 모드 구조는 **유지**하며(별도 관리자 앱/바이너리/백엔드 분리/별도 SPA 없음 — A19), M7~M10 의 백엔드·React 컴포넌트를 전부 **재사용**한다. 구성: (M-a) **노드 해상도 보고** — 노드가 장비 모니터 해상도(width×height)를 M9 시스템 정보 페이로드로 보고하고 서버가 저장·노출한다. (M-b) **관리자 뷰 레이아웃** — 노드 선택 시 관리 크롬을 **상단 수평 바**(노드 피커·서브탭 네비·관리 액션·나가기)로 옮기고 노드 화면이 **전체 너비/높이**를 점유하며, 전역 좌측 사이드바를 숨김/접힘하여 좌우 폭 축소·왜곡을 방지한다. 이는 M9 `NodeManagementPage`(좌측 디렉토리) 를 상단 바 관리자 뷰로 **진화**시킨다. (M-c) **해상도 충실 대시보드 렌더** — `RemoteDashboardView`(그룹 L) 를 노드 보고 해상도 **고정 캔버스**에 렌더 후 종횡비 보존 스케일-투-핏(레터박스, 패널 리플로우 없음)한다. **변경 경로는 불변**(그룹 D/I — 프록시 경유 변경 금지, REQ-J03/J12 일관). **본 그룹은 M9 의 좌측 디렉토리 `NodeManagementPage` 레이아웃을 대체(supersede)한다**(그룹 K 의 디렉토리/노드 대시보드 기능은 보존하되 좌측 master/detail 배치 → 상단 바 풀폭 배치로 진화). **OQ-M1~M5 ✅ 전부 RESOLVED(2026-06-08, 사용자 권고안대로 확정) — 구현 시 고정 제약이다.**

#### M-a. 노드 해상도 보고 (백엔드)

**REQ-REMOTE-M01**: 노드 해상도 보고 (시스템 정보 확장)
시스템은 **항상** 노드가 자신의 **장비 모니터(키오스크/터치스크린) 디스플레이 해상도**(`display_width`, `display_height` — px 정수)를 서버에 보고하도록, M9 시스템 정보 페이로드(`register`(RegisterPayload/HelloPayload) 및/또는 `heartbeat`(HeartbeatPayload), REQ-K07)를 확장해야 한다. 해상도의 **1차 출처는 노드 config**(`display.resolution` 형식 `"WIDTHxHEIGHT"` 또는 `display.width`+`display.height`)이며(xflowd 는 헤드리스 데몬 — 런타임 자동 감지 비대상, A20/OQ-M1), 노드는 config 값을 페이로드로 운반한다. 자원 메트릭(CPU/메모리/디스크)은 본 그룹에서도 보고하지 않는다(그룹 K 비목표 일관).

**REQ-REMOTE-M02**: 해상도 저장·노출
시스템은 **항상** 보고된 노드 해상도를 `managed_nodes`(`display_width`/`display_height` 컬럼)에 저장하고, 노드 상세 API(`GET /api/v1/remote/nodes/{instance_id}`, NodeDetail)로 노출해야 한다(REQ-K08 시스템 정보 노출과 일관). 관리자 뷰는 이 값을 소비해 고정 캔버스 크기를 결정한다(REQ-M07).

**REQ-REMOTE-M03**: 해상도 하위 호환·폴백
시스템은 **항상** 해상도 필드를 보내지 않는(구버전/미설정) 노드도 등록·관리·관리자 뷰 표시가 정상 동작하도록 보장해야 한다. 미보고 시 해상도 필드는 빈값/0 으로 처리되고, 관리자 뷰는 **합리적 기본 해상도**(예: 1920×1080) 또는 **가용 컨테이너 크기**로 폴백해야 한다(REQ-N03/K09 일관 — 회귀 0). 잘못된 형식(음수/0/비정수)도 폴백으로 안전 처리해야 한다.

#### M-b. 관리자 뷰 레이아웃 (프론트엔드)

**REQ-REMOTE-M04**: 상단 바 관리 크롬 + 전체 너비 노드 화면
시스템은 **항상** 관리자 뷰에서 노드가 선택되면 관리 크롬(노드 피커·서브탭 네비·관리 액션·나가기)을 **상단 수평 바**에 배치하고, 그 아래 노드 화면이 **남은 전체 너비/높이**를 점유하도록 해야 한다. 좌측 디렉토리/사이드바를 노드 화면 영역에서 제거하여 노드 화면의 **좌우 폭 축소(왜곡 유발)를 방지**해야 한다. 이는 M9 `NodeManagementPage` 의 좌측(디렉토리 20rem)+우측(대시보드) master/detail 배치를 상단 바 + 풀폭 배치로 진화시킨다(REQ-M10 대체).

**REQ-REMOTE-M05**: 노드 피커 (M9 그룹 구조 보존)
시스템은 **항상** 상단 바에 좌측 디렉토리를 대체하는 **노드 피커**(드롭다운/수평 선택기)를 제공해야 하며, M9 노드 그룹핑(REQ-K01~K05, 단일 레벨 그룹·"전체" 기본 버킷)을 **보존**하여 노드를 그룹별로 묶어 선택 가능하게 해야 한다(그룹별 묶음 드롭다운 — OQ-M3 ✅ RESOLVED). 피커에서 노드를 전환하면 상단 바 컨텍스트(노드 이름·online/status)와 하단 노드 화면이 즉시 전환되어야 한다. 노드 그룹 배정/해제 affordance(REQ-K02)는 보존하되 관리자 뷰 컨텍스트에 맞는 위치(피커 메뉴 또는 관리 액션)로 재배치할 수 있다.

**REQ-REMOTE-M06**: 상단 서브탭 네비 + 전역 사이드바 숨김
시스템은 **항상** M9 노드 대시보드의 서브탭(개요/플로우/에이전트/디바이스/대시보드, REQ-K13)을 **상단 바**에 배치해야 하며, 관리자 뷰에서 앱의 **전역 좌측 사이드바(`Sidebar`)를 숨김/접힘**하여 노드 화면이 가로막히지 않게 해야 한다. 관리자 뷰를 나가면(나가기 액션) 전역 사이드바가 이전 상태로 복원되어야 한다(다른 화면 회귀 0). 서브탭 전환·딥링크(`?node=`/`?tab=`, M9 동작)는 보존되어야 한다.

#### M-c. 해상도 충실 대시보드 렌더 (프론트엔드)

**REQ-REMOTE-M07**: 고정 캔버스 (노드 해상도)
시스템은 **항상** 원격 대시보드(`RemoteDashboardView`/대시보드 서브탭, 그룹 L)를 노드 보고 해상도(REQ-M02 `display_width`×`display_height`)와 **동일한 고정 픽셀 크기 캔버스**에 렌더해야 한다. 캔버스 내부의 대시보드 그리드/패널은 **반응형 리플로우 없이**(고정 컬럼·고정 레이아웃) 노드 장비에서의 배치를 그대로 유지해야 한다(패널 re-flow 금지). 해상도 미보고 시 REQ-M03 폴백 크기를 캔버스 크기로 사용한다.

**REQ-REMOTE-M08**: 레터박스 스케일-투-핏 (종횡비 보존)
시스템은 **항상** 고정 캔버스(REQ-M07)를 관리자 뷰의 가용 콘텐츠 영역에 **종횡비 보존 스케일-투-핏(레터박스)** 으로 맞춰야 한다. 가용 영역과 캔버스 종횡비가 다르면 남는 축에 레터박스 여백(letterbox/pillarbox)을 두고, 패널을 늘이거나(stretch) 잘라내지(crop) 않아야 한다. 스케일은 CSS transform(또는 동등) 으로 적용하여 패널 내부 폰트/요소가 비례 축소·확대되도록 하고, 패널 컴포넌트 코드는 분기하지 않아야 한다(A21/REQ-L09·A16 일관 — target 미지정 로컬 렌더 바이트 동일). 1차 스케일 모드는 fit/레터박스(자동)이다(OQ-M4 ✅ RESOLVED — 수동 줌은 차기).

**REQ-REMOTE-M09**: 패널 데이터·게이팅·READ-ONLY 불변
시스템은 **항상** 고정 캔버스 안에 렌더되는 대시보드 패널(약 10종, M10/REQ-L04~L08)이 그룹 L 의 데이터 경로(대시보드 config `dashboard.get_shared`/`get_mine`·메트릭 query-action·로그/차트 스트림·control 그룹 D)·게이팅(승인∧온라인∧노출, REQ-J05/L11)·실패 의미(503/504/502/404)·redaction(REQ-J06)·**read-only-by-default**(원격 config 편집 비활성, REQ-L12)를 **변경 없이 그대로** 따르도록 해야 한다. 충실 재현은 **렌더 레이아웃(고정 캔버스+스케일)** 에만 관여하며, 데이터·변경·감사 경로는 그룹 L/D/I 를 재사용한다(신규 데이터/변경 경로 미신설 — 프록시 경유 변경 금지 REQ-J03/J12).

#### M-d. 마이그레이션 (M9 레이아웃 대체)

**REQ-REMOTE-M10**: M9 좌측 디렉토리 레이아웃 대체 (기능 보존·딥링크 보존)
시스템은 **항상** M9 `NodeManagementPage` 의 좌측 디렉토리(20rem) + 우측 노드 대시보드 master/detail 레이아웃을 **상단 바 관리자 뷰**로 대체(supersede)하되, M9 의 모든 **기능**(노드 그룹핑·그룹 배정/해제·노드 대시보드 개요(시스템 정보+운영 요약)·Flow/Agent/Device 서브탭·대시보드 서브탭·딥링크 `?node=`/`?tab=`)을 보존해야 한다. 디렉토리/그룹 선택은 상단 노드 피커(REQ-M05)로, 서브탭은 상단 바(REQ-M06)로 이전된다. `등록 관리`(EnrollmentManagementPage)는 **변경 없이 그대로** 유지된다. 기존 라우트(`/admin/remote`, M8/편집기 딥링크, `/admin/remote/control` 리다이렉트 — REQ-K16)는 관리자 뷰에서 도달 가능하게 유지되어야 한다(워크플로 단절 금지).

### 4.7f 그룹 O — 원격 프로그램 버전 관리 (Remote Program Version Management) — v1.7 확장

> **범위(v1.7)** — 관리 서버가 노드용 프로그램 이미지를 호스팅하고(O-a), 원격 업데이트 소스를 서버에 저장해 명령에 자동 주입하며(O-b), 노드의 보고된 아키텍처/OS 별로 그룹 일괄 업데이트를 디스패치하고(O-c), 관리 WS 의 TLS 검증 스킵 옵트인을 제공한다(O-d). 노드 자가 업데이트 메커니즘(SPEC-UPDATE-001)은 **무변경 재사용**하며 서버는 사전 서명된 `.sig` 만 저장·배포한다(A22~A24).

#### O-a. 릴리스 호스팅 (Release Hosting)

**REQ-REMOTE-O01**: 릴리스 이미지 저장
시스템은 **항상** 서버 모드에서 아키텍처별 `xflowd` 바이너리 + Ed25519 `.sig` 를 메타(SQLite `releases`/`release_assets`) + 디스크(`{data}/releases/{version}/`)로 저장해야 하며, 업로드 시 각 바이너리의 SHA256 을 자동 계산하고 버전별 `checksum.txt` 를 자동 생성해야 한다.

**REQ-REMOTE-O02**: 노드용 익명 릴리스 피드 (GitHub-Releases 호환)
시스템은 **항상** 노드 updater 가 무변경으로 소비할 수 있는 **인증 없는** GitHub-Releases 호환 피드를 제공해야 한다: `GET /api/v1/updates/releases/latest`(최신 stable 단일 객체), `GET /api/v1/updates/releases`(전체 릴리스 배열, semver 내림차순), `GET /api/v1/updates/releases/download/{version}/{filename}`(바이너리/`.sig`/`checksum.txt` 스트림). asset 객체는 `name`·`size`·`browser_download_url` 을 포함한다.

**REQ-REMOTE-O03**: asset URL https 강제
시스템은 **항상** 피드 asset 의 `browser_download_url` 을 https 로 강제해야 하며, base 는 설정 `remote_management.public_base_url`(우선) 또는 요청 Host(+`X-Forwarded-*`)에서 유도해야 한다.

**REQ-REMOTE-O04**: 릴리스 관리 API (admin)
시스템은 **항상** admin 인증 하에서 릴리스 관리 API 를 제공해야 한다: `GET /remote/releases`(목록), `POST /remote/releases`(릴리스 메타 생성), `DELETE /remote/releases/{version}`(릴리스 삭제), `DELETE /remote/releases/{version}/assets/{os}/{arch}`(자산 삭제), multipart 업로드 `POST /remote/releases/{version}/assets`(필드: os/arch/binary/signature).

**REQ-REMOTE-O05**: 릴리스 저장 설정
시스템은 **항상** `remote_management.public_base_url`(피드 asset URL base, 빈 값이면 요청 Host 유도)·`remote_management.releases_dir`(릴리스 디스크 루트, 빈 값이면 `{data}/releases`) 설정을 지원해야 한다.

#### O-b. 업데이트 소스 서버 저장 (Update Source)

**REQ-REMOTE-O06**: 업데이트 소스 저장·조회 (admin)
시스템은 **항상** 원격 업데이트 소스(`update_url`+채널)를 SettingsRepository 키 `remote.update_source` 에 1회 저장하고 필요시에만 변경하도록, admin API `GET /remote/update-source`(조회)·`PUT /remote/update-source`(설정, `{update_url, channel}`)를 제공해야 한다.

**REQ-REMOTE-O07**: 업데이트 소스 자동 주입
시스템은 **항상** 원격 업데이트 명령(UpdateNode/UpdateGroup → `system/update`)을 디스패치할 때 저장된 업데이트 소스(`update_url`/채널)를 명령 인자에 자동 주입해야 한다(요청이 명시하면 요청 값 우선).

**REQ-REMOTE-O08**: 업데이트 소스 https 강제
시스템은 **WHEN** `update_url` 이 빈 값도 `https://` 스킴도 아니면, **THEN** 설정을 거부해야 한다(빈 값=해제 → 노드 로컬 설정 사용 허용).

**REQ-REMOTE-O09**: 공개키 비전송 불변식
시스템은 **절대** 업데이트 소스/릴리스 피드/명령 어디에도 노드 검증 공개키를 전송하지 않아야 한다(공개키는 노드 로컬 신뢰 앵커 — `update.public_key_path`, A22).

#### O-c. 아키텍처/OS-aware 그룹 일괄 업데이트 (Group Update)

**REQ-REMOTE-O10**: per-node 타깃 버전 해석
시스템은 **WHEN** 관리자가 그룹 일괄 업데이트를 요청하면, **THEN** 그룹 내 승인 노드 각각의 보고된 OS/Arch(`os/arch`, REQ-K07/K08)로 타깃 버전을 해석해 per-node `system/update` 를 디스패치해야 한다.

**REQ-REMOTE-O11**: 업데이트 전략 3종
시스템은 **항상** 그룹 일괄 업데이트 전략으로 (1) `latest`(채널 내 각 os/arch 의 최고 semver), (2) `pin`(단일 버전 — 기존 호환), (3) `per_arch`((os/arch)→버전 맵)을 지원해야 한다.

**REQ-REMOTE-O12**: 자산 없는 노드 건너뜀 (부분 성공)
시스템은 **WHEN** 노드의 OS/Arch 에 대한 타깃 버전/자산이 없으면, **THEN** 해당 노드를 디스패치하지 않고 사유와 함께 결과에 건너뜀으로 보고해야 하며, 다른 아키텍처 노드는 계속 진행해야 한다.

#### O-d. 관리 WS TLS 옵트인 (Client WS Insecure Skip Verify)

**REQ-REMOTE-O13**: 관리 WS insecure_skip_verify
시스템은 **항상** `remote_management.insecure_skip_verify`(기본 false)를 지원하여, client 모드가 관리 WS(wss) 핸드셰이크의 TLS 인증서 검증을 건너뛸 수 있도록 해야 한다(자체 서명 인증서/사설망 전용 옵트인). 이 옵션은 전송 무결성 보장과 무관하며, 명령/자가 업데이트 무결성은 별도(토큰·Ed25519)로 보장된다.

### 4.8 비기능 요구사항

**REQ-REMOTE-N01**: 다중 노드 확장성
시스템은 **항상** 다수 노드의 영속 연결과 인벤토리 미러를 메모리/연결 한계 내에서 처리해야 하며, 연결 수에 따른 자원 사용을 예측 가능하게 유지해야 한다.

**REQ-REMOTE-N02**: 모니터링 채널 분리
시스템은 **항상** 관리 WS 와 웹 UI 모니터링 WS 를 분리된 엔드포인트로 유지하여 상호 간섭을 방지해야 한다.

**REQ-REMOTE-N03**: 기존 동작 불변
시스템은 **항상** `remote_management.mode=disabled`(기본)일 때 기존 xflow 동작을 회귀 없이 유지해야 한다.

**REQ-REMOTE-N04**: 메시지 봉투 호환
시스템은 **항상** 관리 메시지를 기존 `Message{Type,Payload,Timestamp}` 봉투로 표현하여 ws 인프라와 호환을 유지해야 한다.

## 5. 명세

### 5.1 메시지 프로토콜 스케치 (관리 WS, `Message` 봉투 확장)

모든 관리 메시지는 기존 `Message{Type string, Payload json.RawMessage, Timestamp string}` 봉투(`NewMessage(type, payload)`)를 사용한다. 관리 채널은 별도 엔드포인트(예: `/api/remote/ws`)이며, 웹 UI 모니터링 WS 와 분리된다(REQ-REMOTE-N02).

| Type | 방향 | 페이로드(요지) | 의미 |
|------|------|----------------|------|
| `register` | client→server | instance_id, hostname, version, exposure 요약, (선택)부트스트랩 시크릿, (v1.4 선택) os, arch, started_at(epoch ms), **(v1.6 선택) display_width, display_height(px)** | 등록 요청(REQ-C01) + BASIC 시스템 정보 보고(REQ-K07) + 노드 해상도 보고(REQ-M01) |
| `register_ack` | server→client | status(pending/approved/rejected), (승인 시) node_token | 등록 응답·토큰 발급(REQ-C03/C04) |
| `command` | server→client | command_id, target instance_id, domain(flow/agent/device, **(v1.7) system**), action(**(v1.7) `update`**), args(**(v1.7) `system/update`: target_version, channel, update_url, restart**) | 원격 명령(REQ-D01) + (v1.7) 원격 프로그램 업데이트(REQ-O07/O10) |
| `command_result` | client→server | command_id, ok, result \| error | 명령 결과/ack(REQ-D05) |
| `query` | server→client | query_id, domain(flow/agent/device, **(v1.5) dashboard/monitor**), query_action(열거 allowlist, REQ-J04/L01/L05), args | READ-ONLY per-domain 질의 프록시(REQ-J01/J02) |
| `query_result` | client→server | query_id, status, body(JSON, redacted) \| error | 질의 응답/ack(REQ-J01/J06/J07) |
| `subscribe` | server→client | subscription_id, domain, stream_action(state/stats/series, **(v1.5) chart/logs**), args | 실시간 스트림 구독 시작(REQ-J08/L06/L07) |
| `stream_data` | client→server | subscription_id, payload(JSON, redacted) | 구독 소스 갱신 push(REQ-J08) |
| `unsubscribe` | both | subscription_id | 스트림 구독 해제·teardown(REQ-J08b) |
| `inventory_snapshot` | client→server | instance_id, flows[], agents[], devices[] (노출 범위, redacted) | 접속 시 전체 인벤토리(REQ-E01) |
| `inventory_delta` | client→server | instance_id, op(add/update/remove), kind, item | 변경 델타(REQ-E02) |
| `heartbeat` | both | instance_id, ts, (v1.4 선택) os, arch, version, started_at(epoch ms), **(v1.6 선택) display_width, display_height(px)** | 생존성(REQ-B03) + BASIC 시스템 정보 갱신(REQ-K07) + 노드 해상도 갱신(REQ-M01) |
| `status` | client→server | instance_id, online/health 메타 | 상태 텔레메트리(REQ-B05 보조) |

- 상관: `command`/`command_result` 는 `command_id` 로 1:1 매칭(REQ-D07).
- 봉투의 `Timestamp` 는 RFC3339(기존 `NewMessage` 규약)이며, 페이로드 내부 디바이스 타임스탬프는 프로젝트 규약(epoch ms, int64)을 따른다.
- v1.2(그룹 I): `command` 의 `action` 은 flow/agent 도메인에서 `create`/`update`/`delete` 를 포함하며(임의 domain/action 지원 인프라 재사용), 클라이언트는 이를 각 어댑터의 `Create`/`Update`/`Delete` 로 라우팅한다(REQ-I06). `update`/`create` 의 `args` 정의 JSON 은 마스킹/미변경 시크릿 필드를 **완전히 생략(필드 부재, sentinel 미전송)** 하며, 노드가 어댑터 호출 전 기존 정의를 로드해 부재 시크릿을 기존값으로 backfill 한 뒤 적용한다(REQ-I07, §5.9-6 RESOLVED).
- v1.4(그룹 K): `register`/`heartbeat` 페이로드는 BASIC 시스템 정보(`os`, `arch`, `started_at`(epoch ms))를 **선택 필드**로 운반한다(REQ-K07). 신규 메시지 타입은 신설하지 않으며 기존 봉투/타입을 재사용한다(REQ-N04 유지). 필드 미존재(구버전 노드)는 빈값으로 처리(REQ-K09, 하위 호환). 노드 그룹(`group_name`)은 **서버 측 속성**이므로 와이어 프로토콜로 운반되지 않는다(A13 — 그룹 배정은 서버 REST 전용, 노드 비경유).
- v1.6(그룹 M): `register`/`heartbeat` 페이로드는 노드 해상도(`display_width`, `display_height`, px 정수)를 **선택 필드**로 운반한다(REQ-M01). M9 시스템 정보 필드(os/arch/started_at)와 동일하게 기존 봉투/타입을 재사용하며 신규 메시지 타입을 신설하지 않는다(REQ-N04 유지). 필드 미존재(구버전/미설정 노드)는 빈값/0 으로 처리(REQ-M03, 하위 호환). 해상도는 노드 config 선언값(A20)이며 와이어로는 시스템 정보의 일부로 운반된다. 관리자 뷰 레이아웃·고정 캔버스 스케일은 **순수 프론트엔드** 변경으로 신규 와이어 메시지가 없다(데이터는 그룹 L 재사용).
- v1.5(그룹 L): 그룹 J 프록시 인프라를 **재사용**하여 대시보드 패리티를 제공한다. query-action allowlist 에 **`dashboard` 도메인**(`get_shared`/`get_mine` — READ-ONLY, REQ-L01)과 **`monitor` 도메인**(`metrics` query-action, REQ-L05)을 추가하고, 스트림 action allowlist 에 **`chart`**(REQ-L07, M8 스트림 프록시 chart-action — `/ws/chart/{channel}` 별도 경로 미신설)와 **`monitor` 도메인 `logs`**(REQ-L06)를 추가한다. 변경 의미 action(`dashboard.put`/`delete` 등)은 명시 배제(READ-ONLY — REQ-J03, 원격 config 편집 비목표). 신규 메시지 타입은 신설하지 않으며 기존 `query`/`subscribe` 봉투를 재사용한다(REQ-N04). 대시보드 config·메트릭은 완만 변동이므로 단기 TTL 캐시 대상, 차트/로그는 라이브이므로 캐시 우회(REQ-J16).
- v1.7(그룹 O): 원격 프로그램 업데이트는 기존 `command` 봉투의 **`system` 도메인 `update` action**(`SystemUpdateArgs{target_version, channel, update_url, restart}`)으로 운반된다(신규 메시지 타입 미신설 — REQ-N04 유지). 서버는 디스패치 시 저장된 업데이트 소스(`remote.update_source`)의 `update_url`/채널을 args 에 자동 주입하고(요청 명시 시 우선 — REQ-O07), 그룹 일괄 업데이트는 노드별 OS/Arch 로 `target_version` 을 해석한다(REQ-O10/O11). 노드는 이 args 로 자신의 기존 자가 업데이트 파이프라인(check→download→verify(Ed25519)→apply, SPEC-UPDATE-001)을 실행하므로 **노드 프로토콜·updater 무변경**이다(A22). 릴리스 피드(`/api/v1/updates/releases/*`)·릴리스 관리 API·업데이트 소스 API 는 관리 WS 가 아닌 **REST** 경로이며, 피드는 무인증·관리 API 는 admin-gated 이다(REQ-O02/O04/O06).
- v1.3(그룹 J): `query`/`query_result`(온디맨드 읽기)와 `subscribe`/`stream_data`/`unsubscribe`(실시간 스트림)는 모두 **READ-ONLY** 프록시 전용이다(REQ-J01/J03/J08). `query` 페이로드는 **그룹 D 명령과 대칭되는 per-domain query-action**(`{domain, query_action, args}`, raw GET path 아님 — OQ-J1 RESOLVED)으로 노드의 로컬 read 핸들러를 지정하며, 노드가 실행 후 **redaction(REQ-J06)** 된 본문을 `query_result{query_id, status, body|error}` 로 반환한다. 스트림은 `subscribe{subscription_id, domain, stream_action, args}` 로 시작해 `stream_data{subscription_id, payload}` 로 갱신을 push 하고, `unsubscribe` 또는 노드 오프라인 시 teardown 된다(REQ-J08b). 서버는 query-action 응답을 단기 TTL 로 캐시하되 스트리밍/라이브 action 은 캐시 우회한다(REQ-J16). `command`/`command_result`(변경)와 read 프록시(읽기)는 역할이 분리되며, 봉투/상관/타임아웃 패턴은 공유한다(REQ-D06/D07 준용 — REQ-J02). 변경 의미 action 은 프록시에서 거부된다(REQ-J03).

### 5.2 신규 — xflow 인스턴스 식별자(instance_id)

- 현재 전역 인스턴스 식별자가 없으므로(디바이스 단위 UUID 만 존재), 노드(설치본)를 식별하는 **영속 `instance_id`(UUID)** 를 신설한다.
- 생성: 최초 기동 시 1회 생성 → 로컬 영속(예: `instance_id` 전용 파일 또는 SQLite 메타 테이블). 이후 재시작/재접속 간 불변(REQ-A03).
- 노출: `remote_management.instance_id` 설정으로 명시 지정 가능(미지정 시 자동 생성·영속). 서버는 이 값으로 동일 노드를 재식별하고 등록 상태/토큰/미러를 연결한다.
- IoT 디바이스 UUID(`DeviceIDRepository`)와는 **별개 네임스페이스**이며 혼동 금지(Section 1.4).

### 5.3 신규 설정 스키마 (`remote_management` 섹션)

`internal/config/types.go` 에 `RemoteManagementConfig` 를 추가하고 `Config` 에 편입한다. viper 키 예:

| 키 | 타입 | 기본값 | 의미 |
|----|------|--------|------|
| `remote_management.mode` | string | `"disabled"` | `server` \| `client` \| `disabled` (REQ-A01) |
| `remote_management.server_url` | string | `""` | client 모드 접속 wss URL (REQ-A02) |
| `remote_management.instance_id` | string | `""`(자동생성) | 노드 식별 UUID (REQ-A03) |
| `remote_management.auto_register` | bool | `true` | 미등록 시 자동 등록 요청 여부 (REQ-C01) |
| `remote_management.heartbeat_interval` | duration | 안전 기본값 | heartbeat 주기 (REQ-A05) |
| `remote_management.bootstrap_secret` | string | `""` | (선택) enrollment 사전 공유 시크릿 (REQ-C08) |
| `remote_management.exposure.flows` | string/list | 정책값 | 노출할 플로우 범위(all/none/목록) (REQ-A04) |
| `remote_management.exposure.agents` | string/list | 정책값 | 노출할 에이전트 범위 |
| `remote_management.exposure.devices` | string/list | 정책값 | 노출할 디바이스 범위 |
| `remote_management.tls.*` | — | — | wss 용 TLS(기존 TLSConfig 준용, REQ-F01) |
| `remote_management.insecure_skip_verify` | bool | `false` | (v1.7) client 모드 관리 WS(wss) TLS 인증서 검증 스킵(자체 서명/사설망 전용 옵트인, REQ-O13) |
| `remote_management.public_base_url` | string | `""`(요청 Host 유도) | (v1.7) 노드 릴리스 피드 asset `browser_download_url` base(https 강제, REQ-O03/O05) |
| `remote_management.releases_dir` | string | `""`(=`{data}/releases`) | (v1.7) 릴리스 이미지 디스크 루트(REQ-O01/O05) |

- 핫리로드: 기존 `OnChange`/`WatchConfig` 로 mode/server_url/exposure 변경을 감지·반영(REQ-A06/A07).
- `bootstrap_secret`/발급 토큰은 시크릿이며 redaction·비커밋 대상(REQ-F06).

### 5.4 신규 저장 스키마 (서버 측)

`internal/storage` 에 `ManagedNodeRepository`(인터페이스 + sqlite 구현)와 미러 테이블을 추가한다. `FlowRepository` 패턴(Save/Get/List/Delete/Close)을 준용한다.

| 테이블 | 핵심 컬럼(요지) | 의미 |
|--------|------------------|------|
| `managed_nodes` | instance_id(PK), hostname, version, status(pending/approved/rejected), token_id, last_seen, online, (v1.4) group_name(기본 빈값→"전체"), os, arch, started_at(epoch ms), **(v1.6) display_width, display_height(px, 0=미보고)** | 등록·상태·식별 + (v1.4) 단일 그룹 라벨·BASIC 시스템 정보(REQ-K01/K08) + (v1.6) 노드 해상도(REQ-M01/M02) |
| `mirrored_flows` | id, source_instance_id(FK), name, definition(redacted), updated_at | 노드별 플로우 미러(태그=source_instance_id) |
| `mirrored_agents` | id, source_instance_id(FK), name, kind, config(redacted), updated_at | 노드별 에이전트 미러 |
| `mirrored_devices` | id, source_instance_id(FK), node_assoc, name, meta, updated_at | 노드별 IoT 디바이스 미러 |
| `releases` | version(PK), channel, notes, published_at | (v1.7) 노드용 릴리스 메타(REQ-O01) |
| `release_assets` | version(FK), os, arch, filename, size, sha256, kind(binary/signature/checksum) | (v1.7) 릴리스 자산(아키텍처별 바이너리 + Ed25519 `.sig`, SHA256, REQ-O01) |

- 출처 태깅: 모든 미러 행은 `source_instance_id` 를 보유(REQ-E04/E05).
- last-known: 오프라인 시 행을 삭제하지 않고 `online=false`·`last_seen` 만 갱신(REQ-E06).
- redaction: 정의/설정 저장 시 시크릿 마스킹(REQ-F06).
- (v1.6) 노드 해상도: `managed_nodes` 에 `display_width`/`display_height`(px 정수, 0=미보고) 컬럼을 추가한다(REQ-M01/M02). 미보고/잘못된 값은 0 으로 저장되며 관리자 뷰가 폴백 처리한다(REQ-M03). 노드 상세(NodeDetail)로 노출된다. 관리자 뷰 레이아웃/고정 캔버스는 서버 저장 불필요(순수 프론트 — A19/A21).
- (v1.4) 그룹·시스템 정보: `managed_nodes` 에 `group_name`(단일 그룹 라벨, 기본 빈값→"전체"; REQ-K01), `os`/`arch`/`started_at`(BASIC 시스템 정보; REQ-K08) 컬럼을 추가한다. 그룹은 별도 엔티티 테이블 없이 `group_name` distinct 값으로 표현하며(REQ-K03/K05), 운영 요약은 미러 테이블 집계로 파생한다(저장 컬럼 불필요 — REQ-K10/A15). uptime 은 `started_at` 파생값으로 저장하지 않는다(REQ-K08).
- (v1.7) 릴리스: `releases`/`release_assets` 테이블에 노드용 프로그램 이미지 메타·자산(아키텍처별 바이너리 + Ed25519 `.sig`)을 저장하고, 바이너리 본문은 디스크(`{releases_dir}/{version}/`)에 둔다. 업로드 시 SHA256 자동 계산·`checksum.txt` 자동 생성(REQ-O01). 서버는 사전 서명된 `.sig` 만 저장하며 서명을 생성하지 않는다(A22). 업데이트 소스(`update_url`+채널)는 별도 테이블 없이 `SettingsRepository` 키 `remote.update_source`(JSON)로 저장한다(REQ-O06). 공개키는 어디에도 저장·전송하지 않는다(REQ-O09).

### 5.5 서버/클라이언트 컴포넌트 (신규 `internal/remote`)

| 컴포넌트 | 역할 |
|----------|------|
| protocol | 관리 메시지 Type 상수·페이로드 구조·인코딩(기존 Message 봉투 위) |
| server | 노드 연결 수락(별도 WS 엔드포인트), 등록/승인 상태 머신, 명령 디스패처(상관 id·타임아웃), 인벤토리 수신→`ManagedNodeRepository` 캐시, online/offline 추적 |
| client | server_url dial + 백오프 재연결, 인증 핸드셰이크, 등록 요청, heartbeat, 인벤토리 스냅샷/델타 송신, 명령 수신→로컬 어댑터 적용→결과 반환 |
| 어댑터 브리지 | client 가 보유한 `FlowServiceAdapter`/`AgentServiceAdapter`/device 핸들러로 명령 라우팅(검증/제약 일관 — A5) |

- 배선: `cmd/xflowd/main.go`(~700–800)에서 mode 에 따라 server 또는 client 를 기동하고, server 모드는 승인 API 라우트(`RegisterRoutes`)와 관리 WS 핸들러(`RegisterRawHandler`)를 등록한다.

### 5.6 등록/승인 상태 머신 (서버)

```
(연결) → register 수신 → [pending]
  [pending] --관리자 승인--> [approved] (node_token 발급 → register_ack)
  [pending] --관리자 거부--> [rejected]
  [approved] --재접속(토큰 유효)--> [approved] (세션 복원)
  [approved] --폐기(revoke)--> [revoked] (토큰 blacklist + 연결 종료)
  [rejected]/[pending] --명령 디스패치 차단--
```

- 승인 결정 주체 = 서버(관리자). pending/rejected 는 명령·관리 노출에서 제외(REQ-C06/F03).
- 토큰 발급/검증/폐기는 `JWTService.GenerateTokens`/`ValidateToken`/`Blacklist` 재사용(REQ-C04/C05/C07/F07).

### 5.7 원격 명령 적용 경로 (클라이언트)

1. `command` 수신 → 출처 검증(서버 권한, 승인 노드 여부, D08).
2. `domain`(flow/agent/device) + `action` → 해당 로컬 어댑터 메서드로 라우팅(D02/D03/D04).
3. 어댑터 적용(로컬 API 와 동일 검증). 성공/실패 판정.
4. `command_result`(command_id, ok, result|error) 반환(D05/D09).
5. 서버는 result 수신 후에만 미러 캐시 갱신(E08). 타임아웃 시 미적용 처리(D06).

### 5.8 파일 구조 (예정 — 신규/수정)

| 파일 | 역할 | 변경 타입 |
|------|------|-----------|
| `internal/remote/protocol.go` | 관리 메시지 Type 상수·페이로드 구조 | 신규 |
| `internal/remote/server.go` | 등록/승인 상태 머신, 명령 디스패처, 인벤토리 수신, online/offline | 신규 |
| `internal/remote/client.go` | dial+백오프 재연결, 등록, heartbeat, 인벤토리 송신, 명령 적용 | 신규 |
| `internal/remote/instance_id.go` | 영속 instance_id 생성/로드 | 신규 |
| `internal/storage/managed_node_repository.go` | `ManagedNodeRepository` 인터페이스 | 신규 |
| `internal/storage/managed_node_sqlite.go` | sqlite 구현(managed_nodes + 미러 테이블) | 신규 |
| `internal/config/types.go` | `RemoteManagementConfig` 추가 | 수정 |
| `internal/config/defaults.go`/`validate.go` | remote_management 기본값/검증 | 수정 |
| `internal/api/handler/remote.go` | 승인/거부/폐기/목록 API + 관리 WS 핸들러 | 신규 |
| `internal/api/service/{flow_adapter,agent_adapter,node_adapter}.go` | 명령 적용 진입(기존 메서드 재사용; 필요 시 노출 인터페이스 추출) | 재사용/수정 |
| `cmd/xflowd/main.go` | mode 기반 server/client 기동·라우트 배선(~700–800) | 수정 |
| `web/src/` (관리 노드/승인/자원 뷰) | 서버 웹 UI(그룹 G) | 신규(추후) |
| `internal/api/handler/remote.go` (원격 flow/agent CRUD) | `POST/PATCH/DELETE /remote/nodes/{id}/flows[/{id}]`·`/agents[/{id}]` → command 전파·502/503/504 매핑 (그룹 I) | 수정 |
| `internal/remote/protocol.go` (create/update/delete action) | flow/agent 도메인 create/update/delete action 의미 상수·페이로드 (그룹 I) | 수정 |
| `internal/remote/client.go` (어댑터 바인딩+시크릿 병합) | create/update/delete action → 어댑터 메서드 라우팅, 마스킹 시크릿 생략 정의의 기존값 병합 (그룹 I) | 수정 |
| `web/src/pages/editor/EditorPage.tsx` (원격 편집 재사용) | 원격 플로우를 동일 React Flow 캔버스로 열기/저장(로컬 vs `instance_id` 노드 구분) (그룹 I) | 수정 |
| `web/src/` (원격 자원 페이지 생성/삭제·에이전트 설정 surface) | 원격 자원 생성/삭제 액션·피드백·게이팅, 에이전트 편집 surface (그룹 I) | 신규(추후) |
| `internal/remote/protocol.go` (query/stream) | per-domain query-action 메시지·페이로드(`{domain, query_action, args}`·query_id), 스트림 메시지(subscribe/stream_data/unsubscribe·subscription_id) (그룹 J) | 수정 |
| `internal/remote/query_proxy.go` (서버 query 프록시) | query-action allowlist(REQ-J04)·게이팅(J05)·상관/타임아웃(J02)·502/503/504(J07)·단기 TTL 캐시(J16) (그룹 J) | 신규 |
| `internal/remote/stream_proxy.go` (서버 스트림 중계) | subscribe/stream_data/unsubscribe fan-out, subscription 레지스트리, disconnect teardown·백프레셔(REQ-J08/J08b) (그룹 J) | 신규 |
| `internal/remote/client.go` (query+stream 핸들러) | query-action → 로컬 read 핸들러 매핑·redaction(J06)→query_result; subscribe→실시간 소스 구독→stream_data push·unsubscribe teardown(J08/J08b) (그룹 J) | 수정 |
| `internal/api/handler/remote.go` (프록시 REST/스트림 엔드포인트) | 서버 측 query-action REST 진입(도메인별 read) + 브라우저↔서버 스트림 엔드포인트(WS/SSE) → query/스트림 전파·502/503/504 (그룹 J) | 수정 |
| `web/src/hooks/useFlowsTarget.ts`·`useAgentsTarget.ts`·`useDevicesTarget.ts` | 자원 타깃 추상화(local vs remote:{instanceId} 데이터 소스·액션 전환, query-action·스트림 구독, `useEditorFlowTarget` 패턴 확장, REQ-J09/J12) (그룹 J) | 신규(추후) |
| `web/src/pages/{flows,agents,devices}/*ListPage.tsx`·`*DetailPanel.tsx` | target 입력 수용·원격 동일 렌더링·query-action(상세/통계/노드 레벨)·스트림 구독(실시간 상태/시리즈)(REQ-J10/J11) (그룹 J) | 수정(추후) |
| `web/src/services/api/remoteService.ts` (프록시/스트림 클라이언트) | query-action 호출(TTL 캐시 소비)·스트림 구독/해제(REQ-J08)·폴링 폴백 (그룹 J) | 수정(추후) |
| `web/src/pages/remote/RemoteResourcesPage.tsx` (노드 셀렉터 재용도화) | 노드 셀렉터로 재용도화/폐기, `?target=remote:{id}` 라우팅·네비게이션(REQ-J13/J14) (그룹 J) | 수정(추후) |
| `internal/storage/managed_node_sqlite.go` (group/system-info) | `managed_nodes` 에 group_name/os/arch/started_at 컬럼·마이그레이션, 그룹 배정/distinct 그룹/그룹별 노드 쿼리(REQ-K01~K05/K08) (그룹 K) | 수정 |
| `internal/storage/managed_node_repository.go` (group/summary 인터페이스) | SetNodeGroup/ListGroups/ListNodesByGroup·시스템 정보 저장·운영 요약 파생 메서드(REQ-K02/K03/K04/K10) (그룹 K) | 수정 |
| `internal/remote/protocol.go` (system-info 필드) | RegisterPayload/HelloPayload/HeartbeatPayload 에 os/arch/started_at 선택 필드(REQ-K07, 하위 호환) (그룹 K) | 수정 |
| `internal/remote/client.go` (system-info 보고) | register/heartbeat 송신 시 os/arch/version/started_at 채움(런타임 수집)(REQ-K07) (그룹 K) | 수정 |
| `internal/remote/server.go`/`registration.go` (system-info 수신·저장) | register/heartbeat 수신 시 시스템 정보 저장, uptime 파생 노출(REQ-K08/K09) (그룹 K) | 수정 |
| `internal/api/handler/remote.go` (group/summary REST) | `PUT/DELETE /remote/nodes/{id}/group`·`GET /remote/groups`·그룹별 노드 목록·노드 상세(시스템 정보+운영 요약) admin-gated(REQ-K02~K06/K10) (그룹 K) | 수정 |
| `web/src/components/layout/Sidebar.tsx` (IA 재편) | `원격 관리` 그룹 = `노드 관리`+`등록 관리`(기존 `관리 노드`+`원격 노드 제어` 대체), admin+server 게이팅 유지(REQ-K11/K16) (그룹 K) | 수정(추후) |
| `web/src/pages/remote/NodeManagementPage.tsx` (노드 관리) | 디렉토리 뷰(단일 레벨 그룹 트리, "전체" 기본)+그룹 배정/해제+노드 선택→노드 대시보드(REQ-K12/K13) (그룹 K) | 신규(추후) |
| `web/src/components/remote/NodeDashboard.tsx` (노드 대시보드) | 시스템 정보(BASIC)+운영 요약+Flow/Agent/Device 서브탭(M8 통합 제어 `target=remote:{id}` 재사용)(REQ-K13/K14) (그룹 K) | 신규(추후) |
| `web/src/pages/remote/EnrollmentManagementPage.tsx` (등록 관리) | 토큰 관리(EnrollmentTokenSection)+노드 등록 관리(pending 승인·승인/거부/폐기·사전 등록, RemoteNodesPage 승인부 이관)(REQ-K15) (그룹 K) | 신규(추후) |
| `web/src/services/api/remoteService.ts`·`web/src/hooks/useRemote.ts` (group/summary 클라이언트) | 그룹 배정/해제·distinct 그룹·그룹별 노드·노드 시스템 정보+운영 요약 API 소비(REQ-K02~K05/K10) (그룹 K) | 수정(추후) |
| `web/src/router.tsx` (라우트 재편) | `/admin/remote`→`노드 관리`(디렉토리/대시보드)·`등록 관리` 라우트, M8/편집기 딥링크 보존(REQ-K16) (그룹 K) | 수정(추후) |
| `internal/remote/protocol.go` (dashboard/monitor query·chart/logs stream allowlist) | allowedQueryActions 에 `dashboard`(get_shared/get_mine)·`monitor`(metrics), streamableActions 에 `chart`·`monitor`(logs) 추가(REQ-L01/L05/L06/L07, READ-ONLY) (그룹 L) | 수정 |
| `internal/remote/query.go`/`client_query.go` (dashboard/metrics query 매핑) | QuerySource 가 `dashboard.get_shared/get_mine`→`/dashboards/{shared,mine}`, `monitor.metrics`→`/monitor/metrics` 로컬 read 핸들러 매핑·redaction(REQ-L01/L05) (그룹 L) | 수정 |
| `internal/remote/client_stream.go` (chart/logs stream 소스 바인딩) | StreamSource 가 `chart`(args=channelName)→`/ws/chart/{channel}` 소스, `monitor.logs`→로그 스트림 소스 구독·`stream_data` push·teardown(REQ-L06/L07/J08b) (그룹 L) | 수정 |
| `cmd/xflowd/*` (노드 측 dashboard/monitor/chart 소스 어댑터 바인딩) | 노드의 대시보드 read·`/monitor/metrics`·로그 스트림·`/ws/chart/{channel}` 소스를 QuerySource/StreamSource 에 바인딩(secret_fields redactor 주입)(REQ-L01/L05/L06/L07) (그룹 L) | 수정 |
| `internal/api/handler/remote.go` (dashboard/monitor query REST + chart/logs 스트림 엔드포인트) | 서버 측 dashboard/monitor query-action REST 진입 + 브라우저↔서버 chart/logs 스트림 엔드포인트(SSE) → query/스트림 전파·게이팅·502/503/504/404(REQ-L02/L11) (그룹 L) | 수정 |
| `web/src/pages/dashboard/DashboardPage.tsx` (target 전파) | `renderPanel` 이 dashboard 레벨 target(local | remote:{id})을 TargetProvider/패널 prop 으로 전파, target 미지정 시 로컬 바이트 동일(REQ-L09, A16) (그룹 L) | 수정(추후) |
| `web/src/pages/dashboard/panels/*` (패널 target-aware) | flows/agents/devices/single-device(기존 target 훅 재사용), resource(monitor.metrics), logs(monitor.logs 스트림), control(useDeviceDetailTarget+그룹 D)(REQ-L04/L05/L06/L08) (그룹 L) | 수정(추후) |
| `web/src/pages/dashboard/panels/charts/useChartChannel.ts`·`useChartChannels.ts` (원격 스트림 어댑터 주입) | 원격 target 에서 `wsBaseUrl`/`createClient` 주입점으로 스트림 프록시(SSE) 어댑터 주입(WS 직결 대신), 폴링 폴백(REQ-L07, A18) (그룹 L) | 수정(추후) |
| `web/src/hooks/*`·`web/src/services/api/remoteService.ts` (dashboard config·metrics·logs·chart 프록시 클라이언트) | dashboard config query-action·metrics query-action·logs/chart 스트림 구독 클라이언트·폴링 폴백(REQ-L01/L05/L06/L07) (그룹 L) | 수정(추후) |
| `web/src/components/remote/NodeDashboard.tsx` (대시보드 서브탭) | 노드 대시보드에 대시보드 서브탭 추가 → 로컬 DashboardPage `target=remote:{id}` 재사용·게이팅·원격 컨텍스트·read-only(REQ-L10/L11/L12) (그룹 L) | 수정(추후) |
| `internal/api/handler/release_feed.go` (노드용 릴리스 피드) | 무인증 GitHub-Releases 호환 피드(latest/list/download)·asset https 강제(REQ-O02/O03) (그룹 O) | 신규 |
| `internal/api/handler/release_admin.go` (릴리스 관리 API) | admin 릴리스/자산 CRUD·multipart 업로드(os/arch/binary/signature)(REQ-O04) (그룹 O) | 신규 |
| `internal/storage` (releases/release_assets + 디스크) | `releases`/`release_assets` 메타 + `{releases_dir}/{version}/` 디스크·SHA256·checksum.txt 자동 생성(REQ-O01) (그룹 O) | 신규 |
| `internal/api/handler/remote_version.go` (업데이트 소스 + 명령 주입) | `GET/PUT /remote/update-source`(`remote.update_source` 저장/조회)·`system/update` 명령에 소스 자동 주입·https 검증(REQ-O06/O07/O08) (그룹 O) | 신규/수정 |
| `internal/remote/grouping.go` (그룹 일괄 업데이트) | `GroupUpdatePlan`·`DispatchGroupUpdate`·per-node OS/Arch 타깃 해석·전략 latest/pin/per_arch·부분 성공(REQ-O10/O11/O12) (그룹 O) | 신규/수정 |
| `internal/remote/protocol.go` (system/update args) | `SystemUpdateArgs{target_version, channel, update_url, restart}` + `system` 도메인 `update` action(REQ-O07/O10) (그룹 O) | 수정 |
| `internal/remote/client.go` (관리 WS TLS 옵트인) | `newGorillaDialer(insecureSkipVerify)` → `TLSClientConfig.InsecureSkipVerify`(REQ-O13) (그룹 O) | 수정 |
| `internal/config/types.go`/`config.go`/`defaults.go` (v1.7 설정) | `remote_management.insecure_skip_verify`/`public_base_url`/`releases_dir`(REQ-O05/O13) (그룹 O) | 수정 |
| `cmd/xflowd/main.go`/`remote_system_update.go` (배선·자가 업데이트 실행) | 피드/관리/소스 라우트 배선, `system/update` 수신 시 노드 updater 파이프라인(check→download→verify(Ed25519)→apply) 실행·`update.insecure_skip_verify` 연결(REQ-O02/O04/O07) (그룹 O) | 수정 |
| `web/src/` (릴리스/업데이트 관리 UI) | 릴리스 업로드/목록·업데이트 소스 설정·그룹 일괄 업데이트(전략 선택)·노드별 업데이트 액션(REQ-O04/O06/O10) (그룹 O) | 신규(추후) |

### 5.9 OPEN QUESTIONS (구현 단계에서 결정)

> **그룹 I(v1.2) 결정 사항 (2026-06-06 RESOLVED)** — 아래 6~9는 사용자에 의해 확정되었으며 구현 시 고정 제약이다.

1. **명령 큐잉 정책**: 본 SPEC 은 라이브 RPC 만(오프라인 노드 명령은 거절, REQ-B07). 오프라인 큐잉/지연 적용은 향후 SPEC 으로 분리.
2. **서버↔노드 겸용 인스턴스**: 한 인스턴스가 server 와 client 를 동시에(상위 서버에 등록되면서 하위 노드를 관리, 계층형 fleet) 겸하는 것은 본 SPEC 제외 — 향후 과제.
3. **인벤토리 델타 충돌 해소**: 서버 편집(E08)과 노드 로컬 변경(E02)이 경합할 때의 우선순위/머지 정책 정밀화는 구현 시 결정(기본: 노드가 권위 — A4). (그룹 I 편집 경합은 6.7 RESOLVED 로 구체화됨.)
4. **부트스트랩 신뢰 강도**: 부트스트랩 시크릿(C08) 외 mTLS/인증서 핀닝 도입 여부는 보안 검토 후 결정.
5. **exposure 표현**: all/none/목록/태그 기반 중 어떤 표현을 1차 채택할지 구현 시 확정(기본: all/none + 명시 목록).
6. **✅ RESOLVED — 시크릿 라운드트립 메커니즘(그룹 I, REQ-I07)** → **"필드 부재 + 노드 backfill"**: 갱신 페이로드는 변경되지 않은/마스킹된 시크릿 필드를 **완전히 생략(필드 부재)** 한다. sentinel/자리표시자 값은 와이어로 전송하지 않는다. 노드는 어댑터 `Create`/`Update` 호출 **전에** 자신의 기존 자원 정의를 로드하여 부재한 시크릿 필드를 **기존값으로 backfill** 한 뒤 어댑터에 전달한다.
7. **✅ RESOLVED — 편집 동시성(그룹 I, REQ-I02/I04)** → **"노드 권위 + 경고"**: 낙관적 잠금/버전 체크/409 를 도입하지 **않는다**. 노드를 권위로 하는 last-write-wins(원칙 A4)를 적용한다. 다만 동시 노드 로컬 변경이 감지되면 UI 에 **비차단(non-blocking) 경고**를 표시하되 편집을 거부하지 않는다.
8. **✅ RESOLVED — 신규 원격 자원 식별자 채번(REQ-I01)** → **"노드 채번(node-assigned ID)"**: 노드의 어댑터 `Create` 가 flow/agent ID 를 부여·반환하고, 서버는 반환된 식별자를 미러에 기록한다. 서버는 ID 를 자체 생성하지 **않는다**.
9. **✅ RESOLVED — 신규 생성 자원의 노출 범위(그룹 I, REQ-I01/I04)** → **"수동 노출(opt-in 보존)"**: 원격에서 새로 생성한 자원은 **자동 노출되지 않는다**. 운영자가 노드의 노출 설정(exposure config, REQ-A04/A07)을 갱신하기 전까지 미노출 상태로 유지된다(클라이언트 opt-in 프라이버시 보존).

> **그룹 J(v1.3) OPEN QUESTIONS — ✅ 전부 RESOLVED (2026-06-06, 사용자 확정)** — 아래 7개는 사용자에 의해 확정되었으며 구현 시 고정 제약이다. 일부는 이전 초안 권고를 **변경**한다.

- **OQ-J1 ✅ RESOLVED → "per-domain query-action"**: read 프록시는 raw HTTP-over-WS path 프록시가 아니라 **그룹 D command 와 대칭되는 per-domain query-action**(`{domain, query_action, args}`)을 사용한다. 예: flow=get/nodes/status/logs; agent=get/stats/config/devices/topics/store/sessions/series; device=get/state/commands/metadata. 노드가 각 query-action 을 로컬 read 핸들러로 매핑한다. allowlist = 도메인별 **열거된 query-action 집합**(GET path allowlist 폐기).
- **OQ-J2 ✅ RESOLVED → "FULL 커버리지"**: 최소 집합이 아니라, 모든 로컬 상세 패널을 원격에서 동작시키는 데 필요한 **전체 read query-action**(에이전트 stats/config/devices/topics/store/sessions/series, 디바이스 state/commands/metadata, 플로우 nodes/status/logs + 각 도메인 list/get)을 열거·지원한다. 목록(list)은 미러 우선이되 상세/런타임은 query-action 으로 취득.
- **OQ-J3 ✅ RESOLVED → "스트리밍 프록시 포함"**(이전 초안 "폴링만/연기"를 **반전**): 노드의 라이브 push(디바이스 실시간 상태, 에이전트 라이브 통계/시리즈)를 서버 경유로 브라우저에 중계하는 **스트리밍 프록시**(subscribe/stream_data/unsubscribe, 상관 id, disconnect teardown, 백프레셔, READ-ONLY)를 v1.3 범위에 **포함**한다(REQ-J08/J08b). **폴링은 폴백**으로 유지.
- **OQ-J4 ✅ RESOLVED → "그룹 D 재사용"**: 디바이스 쓰기(`execute`·`metadata`)는 신규 경로 없이 **기존 그룹 D 명령 경로**로 처리(REQ-J12). 변경은 프록시 경유 금지(REQ-J03).
- **OQ-J5 ✅ RESOLVED → "최소 감사"**: 일반 read 는 **미감사**, **노출 위반·오류 접근만** 로깅(REQ-J15). 변경(그룹 D/I)은 기존 감사 유지.
- **OQ-J6 ✅ RESOLVED → "단기 TTL 캐시(처음부터)"**(이전 초안 "v1 pass-through"를 **변경**): 서버는 query-action 응답을 **처음부터 단기 TTL 로 캐시**해 반복 폴 부하를 줄인다. per-action 캐시 가능성(스트리밍/라이브 action 캐시 우회), 관련 변경 시 무효화(REQ-J16).
- **OQ-J7 ✅ RESOLVED → "서버 admin 게이팅 + 노드 세션 권위"**: 프록시 질의는 서버 admin 게이팅(REQ-F04)되고 노드는 인증된 WS 세션 권위로 로컬 핸들러를 실행한다. **사용자 단위 인가 매핑은 v1.3 비도입.**

> **그룹 K(v1.4) OPEN QUESTIONS — ✅ 전부 RESOLVED (2026-06-07, 사용자 확정 — 권고안대로)** — 아래 7개는 사용자에 의해 권고안대로 확정되었으며 구현 시 고정 제약이다.

- **OQ-K1 ✅ RESOLVED → "서버 전용(server-assigned only)"**: 그룹은 서버 관리자만 배정하며 **서버 측 운영 메타데이터**(A13)로만 둔다. 노드 config 기반 그룹 제안은 v1.4 에 **도입하지 않는다**(단순성·권위 분리). 향후 노드 제안 그룹이 필요하면 별도 확장(노드가 register 에 `suggested_group` 운반, 서버 승인)으로 추가.

- **OQ-K2 ✅ RESOLVED → "register(최초) + heartbeat(갱신), query-action 비사용"**: BASIC 시스템 정보는 register 로 최초 보고(필수 필드 채움)하고, 변경 가능 필드(version/started_at 재기동 반영)는 heartbeat 로 갱신한다. 정적 BASIC 메타이므로 그룹 J query-action(라이브 read 프록시)을 신설하지 **않는다**. (started_at 은 재기동 시 변하므로 재접속 register 가 자연 갱신.)

- **OQ-K3 ✅ RESOLVED → "미러 파생 우선(mirror-derived)"**: 운영 요약의 1차 출처는 미러(그룹 E) 집계로 **고정**한다(A15 — 신규 왕복 없음, 오프라인 last-known 제공). 대시보드가 더 정밀한 라이브 수치를 원하면 그룹 J query-action(flow.status/agent.stats 등)으로 **보강(opt-in)** 하되, 라이브 query 는 선택적 보강일 뿐 기본 출처는 미러다.

- **OQ-K4 ✅ RESOLVED → "서버 파생(started_at 기반)"**: 노드는 `started_at`(epoch ms, 프로젝트 타임스탬프 규약 일관)만 보고하고, 서버가 `now - started_at` 으로 uptime 을 산출한다(노드는 uptime 을 직접 보고하지 **않는다**). 노드별 단조 시계 차이를 피하고 표시 시점 기준 일관성을 확보한다.

- **OQ-K5 ✅ RESOLVED → "빈 라벨 = 가상 '전체' 버킷(예약 라벨 비영속)"**: `group_name=""` 을 미지정으로 두고 UI/API 응답에서 가상 "전체" 버킷으로 묶는다. "전체"를 실제 라벨로 영속하지 **않아**(예약어 충돌·재라벨링 복잡성 회피) REQ-K05 환원이 단순해진다.

- **OQ-K6 ✅ RESOLVED → "흡수(이관)"**: `PreRegisterNodeDialog`·승인/거부/폐기 UI 를 `등록 관리`의 "노드 등록 관리" 섹션으로 이관하고, 토큰 관리(`EnrollmentTokenSection`)와 한 페이지에 둔다. 기존 컴포넌트는 재사용(재작성 없음). RemoteNodesPage 는 폐기한다.

- **OQ-K7 ✅ RESOLVED → "노드 관리로 통합, /admin/remote/control 은 리다이렉트"**: 노드 셀렉터·제어 진입을 `노드 관리` 디렉토리/대시보드로 일원화하고, 기존 `/admin/remote/control` 경로는 `노드 관리`로 **리다이렉트**(북마크 보존). M8 통합 제어 페이지(`/flows|agents|devices?target=remote:{id}`)·편집기 라우트는 대시보드 서브탭에서 도달 가능하게 유지한다(REQ-K16).

> **그룹 L(v1.5) OPEN QUESTIONS — ✅ 전부 RESOLVED (2026-06-08, 사용자 권고안대로 확정)** — 아래 6개는 사용자에 의해 권고안대로 확정되었으며 구현 시 고정 제약이다.

- **OQ-L1 ✅ RESOLVED → "노드-로컬 read-only config"**: 원격 대시보드 config 공유 모델은 **노드-로컬**(노드별 fetch, config 의 deviceId 는 그 노드 기준)로 확정한다(A17/REQ-L03). 서버가 `{nodeId, deviceId}` 참조로 여러 노드 패널을 혼합하는 중앙 cross-node config 는 **도입하지 않는다** — v1 단순성, 노드 권위(A4) 일관, deviceId 혼동 회피. 중앙 cross-node 대시보드는 향후 SPEC.

- **OQ-L2 ✅ RESOLVED → "M8 스트림 프록시 chart-action 확장"**: 차트 스트리밍은 **M8 스트림 프록시에 `chart` stream-action 추가**(기존 subscribe/stream_data/teardown/백프레셔/폴백 재사용)로 확정한다(REQ-L07). `/ws/chart/{channel}` 를 그대로 프록시하는 **별도 WS 프록시 경로(제2 WS 핸들러)는 신설하지 않는다** — 인프라 단일화, teardown·백프레셔·redaction·캐시 정책 재사용, WS 경로 중복 회피.

- **OQ-L3 ✅ RESOLVED → "v1.5 READ-ONLY, 편집은 차기"**: 원격 대시보드 config 는 **v1.5 READ-ONLY**(보기·자원 제어만)로 확정한다(REQ-L12, §1.5 비목표). v1.5 에서 원격 config 편집(`PUT/DELETE /dashboards/{shared,mine}` 프록시 변경 경로)은 **도입하지 않는다** — config 편집은 변경 의미라 READ-ONLY 원칙(J03)·노드 권위·redaction 라운드트립(I07 유사) 추가 설계가 필요하므로 차기 단계로 분리.

- **OQ-L4 ✅ RESOLVED → "control 쓰기 = 그룹 D 재사용"**: control 패널(ac/hvac/outdoor) 명령 쓰기는 **기존 그룹 D 명령**(REQ-D04 디바이스 명령)으로 확정한다(REQ-L08). 대시보드 전용 신규 쓰기 경로는 **신설하지 않는다** — 디바이스 쓰기는 이미 그룹 D 로 일원화(OQ-J4 일관), 프록시 경유 변경 금지(J03).

- **OQ-L5 ✅ RESOLVED → "노드 대시보드 서브탭"**: 원격 대시보드 진입은 **노드 대시보드(M9 `NodeDashboard`)의 대시보드 서브탭**으로 확정한다(REQ-L10). M9 IA(노드 디렉토리 + 대시보드 서브탭) 일관, Flow/Agent/Device 서브탭과 나란히 배치, 노드 컨텍스트 표시 일원화. 독립 `DashboardPage target=remote:{id}` 라우트는 보조로 둘 수 있으나 **1차 진입은 서브탭**이다.

- **OQ-L6 ✅ RESOLVED → "메트릭=query-action, 로그=스트림"**: 프록시 방식은 **메트릭=query-action**(REQ-L05, `monitor.metrics`, `/monitor/metrics` 스냅샷·완만 변동·단기 TTL 캐시 적합) + **로그=스트림**(REQ-L06, `monitor.logs`, 라이브 tail·캐시 우회·teardown/백프레셔 적합)으로 확정한다. 각각 폴링 폴백을 보유한다.

> **그룹 M(v1.6) OPEN QUESTIONS — ✅ 전부 RESOLVED (2026-06-08, 사용자 권고안대로 확정)** — 아래 5개는 사용자에 의해 권고안대로 확정되었으며 구현 시 고정 제약이다.

- **OQ-M1 ✅ RESOLVED → "노드 config 선언(config-declared), 폴백=관리자 뷰 기본값/컨테이너"**: 노드 장비 모니터 해상도는 **노드 config 선언**(`display.resolution` 또는 `display.width`+`display.height` — 운영자가 장비 화면 해상도 명시)을 1차 출처로 확정한다. 런타임 자동 감지(연결 디스플레이 enumerate)는 **도입하지 않는다**(xflowd 는 헤드리스 데몬이라 디스플레이 서버 감지 대상이 없을 수 있고, "장비 모니터"는 키오스크/터치스크린이라 배포 시 운영자가 가장 정확히 안다). 미보고/미설정 시 관리자 뷰가 **기본값(1920×1080)/컨테이너 크기**로 폴백한다. 서버 측 노드별 수동 설정(override)은 보조로 추후 추가 가능. → A20/REQ-M01/REQ-M03 에 반영(고정 제약).

- **OQ-M2 ✅ RESOLVED → "대시보드 전용 고정 캔버스; 플로우/에이전트/디바이스는 전체 너비 반응형"**: 고정 캔버스(레터박스) 충실 재현은 **대시보드 서브탭에만** 적용하기로 확정한다. 플로우/에이전트/디바이스 통합 제어 서브탭은 상단 바 아래 **전체 너비 반응형**으로 유지한다(고정 캔버스 스케일 미적용). 근거: 대시보드는 노드 장비에 실제 표시되는 운영 화면(키오스크)이라 충실 재현 가치가 크고, 통합 제어 페이지는 관리 도구 화면(노드 장비 화면이 아님)이라 관리자 화면 크기 반응형이 더 유용. → §1.5 비목표·REQ-M07·REQ-M09 에 반영(고정 제약).

- **OQ-M3 ✅ RESOLVED → "그룹 묶음 드롭다운(grouped dropdown)"**: 상단 바 노드 선택 컨트롤은 **그룹별로 묶인 드롭다운**(상단 단일 드롭다운, 그룹 헤더 아래 노드)으로 확정한다. M9 노드 그룹핑(REQ-K01~K05) 구조를 보존하며 공간 절약·다수 노드 확장성 이점을 가진다. 노드 수가 많아지면 검색 보강(콤보박스)을 추후 추가 가능. → REQ-M05 에 반영(고정 제약).

- **OQ-M4 ✅ RESOLVED → "fit/레터박스(종횡비 보존); 수동 줌은 차기"**: 고정 캔버스 스케일 모드는 **fit/레터박스**(종횡비 보존, 남는 축 여백)로 확정한다. fill(영역 채움 — 종횡비 깨짐/crop)은 **채택하지 않는다**(충실 재현의 핵심은 왜곡 없는 비례 유지). 수동 줌(슬라이더·팬·1:1 토글)은 가치 있으나 1차 범위 밖으로 **차기 대화형 컨트롤로 분리**한다. → §1.5 비목표·REQ-M08 에 반영(고정 제약).

- **OQ-M5 ✅ RESOLVED → "NodeManagementPage 제자리 진화(evolve in place)"**: 상단 바 관리자 뷰는 기존 **`NodeManagementPage`(`/admin/remote`)를 제자리에서 진화**(좌측 디렉토리 → 상단 바 풀폭)시키는 것으로 확정한다. 신규 라우트/모드 병존(`/admin/remote/manager` 등, 기존 디렉토리 뷰 병존)은 **도입하지 않는다**(두 레이아웃 유지보수 부담·UX 이원화 회피). 라우트·딥링크·게이팅·`등록 관리` 분리는 보존하고 M9 기능을 전부 보존한다(REQ-M10). 사용자 결정(LOCKED: "기능 이전"=UI 재구성, 별도 앱/바이너리/백엔드 분리/별도 SPA 없음)과 일관. → REQ-M10/§5.13.4 마이그레이션에 반영(고정 제약).

### 5.10 그룹 J(v1.3) 명세 — READ/QUERY 프록시(query-action + 스트리밍) & 통합 UI

#### 5.10.1 아키텍처 — per-domain query-action 프록시 (OQ-J1/J2 RESOLVED)

- **프록시 형태**: 그룹 D command 와 **대칭**되는 **per-domain query-action**(`query{domain, query_action, args}`)을 사용한다. 노드는 각 query-action 을 자신의 로컬 read 핸들러로 매핑한다(아래 매핑 표). raw HTTP path 프록시는 사용하지 않는다.
- **query-action allowlist(FULL 커버리지, REQ-J04)** — 도메인별 열거 집합 + 노드 측 로컬 핸들러 매핑:
> | domain | query_action | 노드 로컬 read 핸들러 매핑 |
> |--------|--------------|---------------------------|
> | flow | `list` | `GET /flows`(미러 우선, 라이브 보강) |
> | flow | `get` | `GET /flows/{id}` |
> | flow | `status` | `GET /flows/{id}/status` |
> | flow | `nodes` | `GET /flows/{id}/nodes` |
> | flow | `node` | `GET /flows/{id}/nodes/{nodeID}` |
> | flow | `logs` | 플로우/노드 로그 read 소스 |
> | agent | `list` | `GET /agents`(미러 우선) |
> | agent | `get` | `GET /agents/{id}?detail=full` |
> | agent | `stats` | `GET /agents/{id}/stats` |
> | agent | `config` | `GET /agents/{id}`(config 섹션) |
> | agent | `devices` | 에이전트 연결 디바이스 read |
> | agent | `topics` | 에이전트 토픽 read |
> | agent | `store` | `GET /store/{agent}/query|keys|tags|keys/{key}` |
> | agent | `sessions` | 에이전트 세션 read |
> | agent | `series` | `GET /tsdb/query|series|series/{key}/latest|stats` |
> | device | `list` | `GET /devices`(미러 우선) |
> | device | `get` | `GET /devices/{ref}` |
> | device | `state` | 디바이스 실시간 상태 read(라이브 — 스트림 가능) |
> | device | `commands` | 디바이스 명령 스펙 read |
> | device | `metadata` | 디바이스 메타데이터 read |
> 미열거 action·변경 의미 action(create/update/delete/execute/metadata-write/start/stop/config)은 거부(REQ-J03).
- **READ-ONLY 불변(REQ-J03)**: query·스트림은 읽기 전용. 변경은 그룹 D 명령(디바이스 쓰기 포함, OQ-J4) / 그룹 I(M7) CRUD 로만(REQ-J12).
- **게이팅(REQ-J05)**: 승인 ∧ 온라인 ∧ 노출 범위. 노출되지 않은 자원 질의 불가(REQ-E07/A06).
- **redaction(REQ-J06)**: 노드가 전송 전 `secret_fields.go` 로 마스킹(미러와 동일 정책). 서버는 가공 없이 전달(A10).
- **실패(REQ-J07)**: offline→503, timeout→504, node-error→502(그룹 I I11 동일 매핑 — `mapRemoteCommandError` 재사용).
- **단기 TTL 캐시(REQ-J16, OQ-J6)**: 서버가 query-action 응답(redacted)을 `{instance_id, domain, query_action, args}` 키로 **단기 TTL 캐시**. 정적/완만 action 만 캐시, **스트리밍/라이브 action(state/stats/series)은 캐시 우회**. 관련 변경(그룹 D/I) 성공 시 무효화. 게이팅은 캐시 적중 시에도 매 요청 평가.

#### 5.10.2 스트리밍 프록시 (REQ-J08/J08b, OQ-J3 RESOLVED — 포함)

- **메커니즘**: WS 세션 위 `subscribe{subscription_id, domain, stream_action, args}` → 노드가 해당 실시간 소스를 구독 → 갱신마다 `stream_data{subscription_id, payload(redacted)}` push → 서버가 해당 브라우저 세션으로 fan-out. `unsubscribe{subscription_id}` 로 해제.
- **대상**: 디바이스 실시간 상태(`device.state`), 에이전트 라이브 통계/시리즈(`agent.stats`/`agent.series`). 로컬 패널의 실시간 갱신을 원격에서 동형 제공.
- **라이프사이클(REQ-J08b)**: 노드 오프라인/연결 종료 → 해당 노드 전 구독 teardown. 브라우저 세션 종료/unsubscribe → 노드로 전파해 소스 구독 해제. 누수 없음.
- **백프레셔(REQ-J08b)**: 느린 소비자에 대해 최신값 우선 coalesce/drop 또는 전송 속도 제한으로 메모리 폭증 방지.
- **READ-ONLY(REQ-J03)** + **redaction(REQ-J06)** 동일 적용. 스트림은 캐시 우회(REQ-J16).
- **폴백**: 스트림 미지원/실패 시 query-action 폴링(서버 단기 TTL 캐시 경유)으로 대체.

#### 5.10.3 통합 UI 경로 (REQ-J09~J14)

1. **타깃 추상화**: `useEditorFlowTarget`(이미 local vs remote 분기) 패턴을 목록/제어로 확장 → `useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget`. 데이터 소스(로컬 API vs 프록시)·액션(로컬 vs 그룹 D/I)을 투명 전환(REQ-J09/J12).
2. **페이지 재사용**: `FlowListPage`/`AgentListPage`/`DeviceListPage` + 상세 패널이 `target` 을 받아 원격에서도 동일 렌더링(REQ-J10/J11). 별도 원격 화면 미신설.
3. **상세 패리티**: 에이전트 통계/설정/디바이스/토픽/store/세션/시리즈, 디바이스 실시간 상태/명령 스펙/메타데이터, 플로우 상태/노드 레벨 런타임·로그 → 정적/온디맨드는 per-domain query-action(REQ-J04, 서버 TTL 캐시 REQ-J16), 실시간(디바이스 상태·에이전트 라이브 통계/시리즈)은 스트리밍 구독(REQ-J08) → 동일 상세 패널 렌더(REQ-J11).
4. **네비게이션**: 노드 셀렉터 + `?target=remote:{instanceId}` 라우팅(미지정=local), 사이드바 현재 타깃 표시(REQ-J13). `RemoteResourcesPage` 는 노드 셀렉터로 재용도화/폐기(REQ-J14).

### 5.11 그룹 K(v1.4) 명세 — 노드 그룹핑 + 시스템 정보 + 운영 요약 + UI 재편

#### 5.11.1 노드 그룹핑 (REQ-K01~K06)

- **모델**: `managed_nodes.group_name`(자유 입력 문자열, 기본 빈값). 노드당 최대 1 그룹, 단일 레벨(계층 없음). 별도 그룹 엔티티 테이블 없음 — distinct `group_name` 집합이 곧 그룹 목록.
- **"전체" 기본 버킷(OQ-K5 RESOLVED)**: `group_name=""` = 미지정 = 가상 "전체" 버킷(예약 라벨 비영속). UI/API 가 빈 라벨 노드를 "전체"로 묶어 표시.
- **REST(admin-gated, REQ-K06)**:
  - 배정/해제: `PUT /api/v1/remote/nodes/{id}/group {group_name}` · `DELETE /api/v1/remote/nodes/{id}/group`(또는 빈 문자열 PUT) → "전체"로 환원.
  - 목록: `GET /api/v1/remote/groups`(distinct + 노드 수, 항상 "전체" 포함) · `GET /api/v1/remote/nodes?group_by=group`(그룹별 노드).
- **이름 변경·삭제 의미(REQ-K05)**: 그룹 이름 변경 = 구성원 `group_name` 재라벨링. 그룹 비우기 = 구성원 빈값 환원. 구성원 0 그룹은 distinct 목록에서 자동 소멸.
- **권위(A13)**: 그룹은 **서버 운영 메타데이터** — 노드로 명령 전파 없음(그룹 D 비경유), 미러/노드 정의 무관.

#### 5.11.2 노드 시스템 정보(BASIC) + 운영 요약 (REQ-K07~K10)

- **시스템 정보 보고(OQ-K2 RESOLVED)**: 노드가 `register`(최초) + `heartbeat`(갱신)로 `os`/`arch`/`version`/`started_at`(epoch ms) 보고. 자원 메트릭(CPU/메모리/디스크) **제외**. 신규 메시지 타입 없이 기존 페이로드 확장(REQ-N04). 하위 호환 — 미보고 필드 빈값(REQ-K09).
- **저장·노출(REQ-K08)**: `managed_nodes`(os/arch/started_at + 기존 hostname/version/online/last_seen/status). **uptime = now − started_at** 서버 파생(OQ-K4 RESOLVED, 저장 안 함).
- **운영 요약(REQ-K10, A15)**: 미러(그룹 E) 집계 파생 — 플로우(카운트+running/stopped), 에이전트(카운트+connected), 디바이스(카운트+online). 오프라인 시 last-known 미러로 제공. 라이브 정밀 수치는 그룹 J query-action 보강 가능(OQ-K3 RESOLVED = 미러 우선).

#### 5.11.3 UI 정보 구조 재편 (REQ-K11~K16)

- **사이드바 `원격 관리`(server 모드+admin 게이팅 유지)**:
  - **`노드 관리`(운영)**: 디렉토리 뷰(단일 레벨 그룹 트리, "전체" 기본; 노드 online/status·그룹 배정/해제) → 노드 선택 → **노드 대시보드**(시스템 정보 BASIC + 운영 요약 + Flow/Agent/Device 서브탭=M8 통합 제어 `target=remote:{instanceId}` 재사용).
  - **`등록 관리`(온보딩)**: 토큰 관리(`EnrollmentTokenSection`) + 노드 등록 관리(pending 승인 큐·승인/거부/폐기·사전 등록 — 기존 RemoteNodesPage 승인부·`PreRegisterNodeDialog` 이관).
- **대체(REQ-K11/K16)**: 기존 `관리 노드`(RemoteNodesPage, `/admin/remote`) + `원격 노드 제어`(RemoteControlPage, `/admin/remote/control`)를 위 둘로 대체. `RemoteControlPage` 셀렉터 개념은 `노드 관리` 디렉토리 안으로 흡수, RemoteNodesPage 승인/등록부는 `등록 관리`로 이관.
- **딥링크 보존(OQ-K7 RESOLVED)**: `/admin/remote/control` → `노드 관리` 리다이렉트. M8 통합 제어(`/flows|agents|devices?target=remote:{id}`)·원격 편집기 라우트는 대시보드 서브탭에서 도달 가능.
- **변경 경로 불변**: 대시보드 서브탭의 모든 변경은 그룹 D 명령 / 그룹 I CRUD 로만(REQ-J03/J12/K14 일관 — 신규 변경 경로 미신설).

### 5.12 그룹 L(v1.5) 명세 — 원격 노드 대시보드 패리티

> 그룹 J(M8) query/스트림 프록시 + target 추상화를 **재사용**한다(재발명 금지). 신규는 (1) query/stream allowlist 확장(`dashboard`/`monitor` 도메인 + `chart`/`logs` stream-action), (2) 노드 측 read/스트림 소스 바인딩, (3) 차트용 스트림 어댑터 주입, (4) 패널 target 전파(TargetProvider/prop), (5) 노드 대시보드 대시보드 서브탭뿐이다.

#### 5.12.1 대시보드 config 읽기 프록시 (REQ-L01~L03)

- **query-action 추가(REQ-L01)**: 그룹 J allowlist 에 `dashboard` 도메인 + `get_shared`/`get_mine`(READ-ONLY) 추가. 노드 매핑:
> | domain | query_action | 노드 로컬 read 핸들러 매핑 |
> |--------|--------------|---------------------------|
> | dashboard | `get_shared` | `GET /dashboards/shared` (global 스코프 config) |
> | dashboard | `get_mine` | `GET /dashboards/mine` (인증 사용자 스코프 config) |
> 변경 의미 action(`put`/`delete`)은 **명시 배제**(REQ-J03 — 원격 config 편집 비목표).
- **게이팅·redaction·실패·캐시(REQ-L02)**: 승인 ∧ 온라인 ∧ 노출(REQ-J05), 노드 전송 전 redaction(REQ-J06), offline→503/timeout→504/node-error→502(REQ-J07), config 미설정→404(또는 빈 config). config 는 완만 변동이므로 단기 TTL 캐시 대상(REQ-J16).
- **노드-로컬 권위·네임스페이싱(REQ-L03, A17)**: config 는 노드 권위(A4) — 서버 미러 비대상·미영속, 프록시 취득·단기 캐시만. config 내부 `deviceId`/패널 자원 참조는 **그 노드 기준**(서버 디바이스와 혼동 금지 — §1.4).

#### 5.12.2 패널 데이터 소스 target-aware (REQ-L04~L09)

- **패널별 데이터 소스 매핑**(약 10종, `renderPanel` 에서 target 전파 — REQ-L09):
> | 패널 | 로컬 소스 | 원격(v1.5) 경로 |
> |------|-----------|------------------|
> | flows | `useFlows` | 그룹 J target 훅(`useResourceTargets`) — query-action `flow.list/status` (재사용) |
> | agents | `useAgents` | 그룹 J target 훅 — `agent.list/get/stats` (재사용) |
> | devices | `useDevicesRealtime` | 그룹 J target 훅(`useDevicesTarget`) — `device.list/state` (재사용) |
> | device(single) | `useDeviceRealtime` | `useDeviceDetailTarget` — `device.get/state` (재사용) |
> | resource(metrics) | `getMetrics`(`/monitor/metrics`) | **신규 query-action `monitor.metrics`**(REQ-L05, 온디맨드+TTL 캐시) |
> | logs | 로그 WS 스트림 | **신규 스트림 action `monitor.logs`**(REQ-L06, 라이브·캐시 우회) |
> | charts(line/stat/bar/pie/table) | `useChartChannel`(`/ws/chart/{channel}`) | **M8 스트림 프록시 `chart` stream-action**(REQ-L07, args=channelName) |
> | ac/hvac/outdoor-control | `useDeviceRealtime`+`useExecuteCommand` | read=`useDeviceDetailTarget`(`device.state`), 쓰기=**그룹 D 명령**(REQ-L08, 프록시 비경유) |
> | gauge/properties-grid | 디바이스/속성 read | 해당 디바이스 target 훅 재사용 |
> | text/custom-control | 정적 | target 무관(로컬 동일) |
- **메트릭(REQ-L05)**: `monitor.metrics` query-action — 노드 `/monitor/metrics` 매핑, 스냅샷 read, 단기 TTL 캐시, 장기 시계열/상시 폴링 미신설(K BASIC 비목표 일관).
- **로그(REQ-L06)**: `monitor.logs` stream-action — 노드 로그 스트림 구독→`stream_data` push→서버 fan-out, teardown·백프레셔(J08b), 캐시 우회, 폴링 폴백.
- **차트(REQ-L07, A18)**: `chart` stream-action — 노드 `/ws/chart/{channel}` 소스를 노드 내부 구독→backfill/append 이벤트를 `stream_data` 로 중계→서버 fan-out. **별도 WS 경로 미신설**. `useChartChannel` 의 `wsBaseUrl`/`createClient` 주입점으로 원격 target 에서 스트림 프록시(SSE) 어댑터 주입 → 동일 차트 패널 코드 동작. 스트림 미지원/실패 시 스냅샷 query-action 폴백.
- **control 쓰기(REQ-L08, OQ-L4)**: 디바이스 명령은 그룹 D(REQ-D04)로만. config 의 bare `deviceId` 는 원격 target 하 그 노드 디바이스로 해석(REQ-L03).
- **target 전파(REQ-L09)**: `DashboardPage`/`renderPanel` 이 dashboard 레벨 target 을 TargetProvider(context) 또는 패널 prop 으로 전파. **target 미지정/`local` 시 로컬 렌더 바이트 동일**(회귀 0 — A16). 현 `renderPanel` 은 target 미전달이므로 이를 확장.

#### 5.12.3 UI 통합 & 게이팅 (REQ-L10~L13)

- **진입(REQ-L10, OQ-L5)**: 노드 대시보드(M9 `NodeDashboard`)에 **대시보드 서브탭** 추가(Flow/Agent/Device 서브탭과 나란히), 로컬 `DashboardPage` 를 `target=remote:{instanceId}` 로 재사용. config 는 `dashboard.get_shared`/`get_mine` query-action 으로 노드-로컬 취득.
- **게이팅·실패·컨텍스트(REQ-L11)**: 승인 ∧ 온라인 ∧ 노출(`useTargetGating` 재사용), 503/504/502/404 구분, 원격 노드 컨텍스트(노드 이름·원격 표식) 일관 표시(M8/M9 IA 일관).
- **read-only-by-default(REQ-L12, OQ-L3)**: 원격 대시보드 config 편집/저장/생성/삭제 비활성·숨김(비목표). 자원 제어 액션(플로우 라이프사이클·에이전트 start/stop·디바이스 명령/메타데이터)은 그룹 D/I 로만(신규 변경 경로·프록시 변경 금지 — J03/J12).
- **최소 감사(REQ-L13)**: 일반 read(config·query·스트림) 미감사, 노출 위반·오류만 로깅(J15 일관). 변경(그룹 D/I)은 기존 감사 유지, 시크릿 비포함(F06).

### 5.13 그룹 M(v1.6) 명세 — 관리자 뷰 & 노드 화면 충실 재현

> M7~M10 의 백엔드·React 컴포넌트를 **재사용**한다(재발명/재작성 금지 — A19). 신규는 (1) 노드 해상도 보고 필드·저장·노출, (2) 상단 바 관리자 뷰 셸 + 노드 피커 + 전역 사이드바 숨김, (3) 대시보드 고정 캔버스 스케일 래퍼뿐이다. 데이터·변경·감사 경로는 그룹 L/D/I 불변.

#### 5.13.1 노드 해상도 보고 (REQ-M01~M03)

- **출처(A20/OQ-M1 ✅ RESOLVED)**: 노드 config `display.resolution`(`"1920x1080"`) 또는 `display.width`+`display.height`. xflowd 헤드리스 → 런타임 자동 감지 비대상. 노드는 config 값을 시스템 정보 페이로드(register 필수·heartbeat 갱신)로 운반(REQ-M01). 신규 메시지 타입 없이 M9 페이로드 확장(REQ-N04 유지).
- **저장·노출(REQ-M02)**: `managed_nodes.display_width`/`display_height`(px 정수, 0=미보고). 노드 상세 NodeDetail 로 노출. 서버 파생 불필요(원시 width/height 그대로).
- **하위 호환·폴백(REQ-M03)**: 미보고/0/잘못된 형식 → 관리자 뷰가 기본 해상도(예: 1920×1080) 또는 가용 컨테이너 크기로 폴백. 등록/관리 회귀 0(REQ-N03/K09 일관).
- **신규 config 키(예)**:
> | 키 | 타입 | 기본값 | 의미 |
> |----|------|--------|------|
> | `remote_management.display.resolution` | string | `""` | `"WIDTHxHEIGHT"`(예: `"1920x1080"`). 설정 시 width/height 로 파싱 |
> | `remote_management.display.width` | int | `0` | 장비 화면 가로 px(resolution 미설정 시) |
> | `remote_management.display.height` | int | `0` | 장비 화면 세로 px(resolution 미설정 시) |

#### 5.13.2 관리자 뷰 레이아웃 (REQ-M04~M06)

- **셸 구조**: 노드 선택 시 — 상단 **수평 바**(노드 피커 + 서브탭 네비 개요/플로우/에이전트/디바이스/대시보드 + 관리 액션 + 나가기) / 그 아래 **풀폭 노드 화면 영역**. M9 의 `lg:grid-cols-[20rem_1fr]`(좌 디렉토리 + 우 대시보드) → 상단 바 + 풀폭으로 진화(REQ-M04/M10).
- **노드 피커(REQ-M05, OQ-M3 ✅ RESOLVED)**: 그룹별 묶음 드롭다운. M9 그룹핑(REQ-K01~K05) 보존 — `GET /remote/groups`(전체 포함) + `GET /remote/nodes` 클라이언트 묶기 재사용(NodeManagementPage 의 `nodesByGroup`/`sortedGroups` 로직 재사용). 노드 전환 시 상단 컨텍스트 + 하단 화면 즉시 전환. 그룹 배정/해제(REQ-K02, `NodeGroupMenu`)는 피커 메뉴/관리 액션으로 재배치.
- **상단 서브탭 + 사이드바 숨김(REQ-M06)**: M9 `NodeDashboard` 서브탭 네비를 상단 바로 호이스팅(개요=시스템 정보+운영 요약, 플로우/에이전트/디바이스=M8 통합 제어, 대시보드=M10 RemoteDashboardView). 진입 시 전역 `Sidebar` 숨김/접힘(`uiStore.setSidebarCollapsed(true)` 또는 라우트 레벨 사이드바 미렌더), 나가기 시 복원. 딥링크 `?node=`/`?tab=` 보존.

#### 5.13.3 해상도 충실 대시보드 렌더 (REQ-M07~M09)

- **고정 캔버스(REQ-M07)**: 대시보드 서브탭(RemoteDashboardView)을 `width=display_width`, `height=display_height` 의 고정 픽셀 컨테이너에 렌더. 내부 그리드/패널은 고정 컬럼·고정 레이아웃(리플로우 없음). 해상도 미보고 → REQ-M03 폴백 크기.
- **레터박스 스케일(REQ-M08, OQ-M4 ✅ RESOLVED)**: `scale = min(availW/canvasW, availH/canvasH)` → `transform: scale(scale)` + 중앙 정렬 + 남는 축 레터박스 여백. stretch/crop 금지(종횡비 보존). 패널 폰트/요소 비례 축소. 패널 컴포넌트 코드 비분기(스케일은 바깥 래퍼 — A21/A16).
- **데이터·게이팅·READ-ONLY 불변(REQ-M09)**: 캔버스 내부 패널은 그룹 L 데이터 경로(config `dashboard.get_shared`/`get_mine`·`monitor.metrics` query·`monitor.logs`/`chart` 스트림·control 그룹 D)·게이팅(REQ-J05/L11)·실패(503/504/502/404)·redaction(J06)·read-only-by-default(L12)를 그대로 따름. 충실 재현은 렌더 레이아웃에만 관여(신규 데이터/변경 경로 미신설).
- **재사용/신규**:
> | 항목 | 처리 |
> |------|------|
> | 노드 해상도 보고/저장/노출 | M9 시스템 정보 경로 확장(REQ-M01~M03) — 신규 필드 |
> | 상단 바 셸·노드 피커·사이드바 숨김 | 신규 프론트(NodeManagementPage 진화 + uiStore 사이드바 토글) |
> | 고정 캔버스 + 레터박스 스케일 | 신규 프론트 래퍼(RemoteDashboardView 감싸기) |
> | 그룹핑·노드 대시보드·통합 제어·패널·DashboardPage·프록시·명령·미러 | **전부 재사용(M7~M10)** |

#### 5.13.4 마이그레이션 (REQ-M10)

- M9 `NodeManagementPage` 좌측 디렉토리 master/detail → 상단 바 관리자 뷰 **제자리 진화**(OQ-M5 ✅ RESOLVED — 신규 라우트 병존 아님). 기능 전부 보존: 그룹핑·그룹 배정/해제·개요(시스템+운영)·Flow/Agent/Device 서브탭·대시보드 서브탭·딥링크.
- `등록 관리`(EnrollmentManagementPage) **불변**. 라우트(`/admin/remote`)·게이팅(admin+server)·M8/편집기 딥링크·`/admin/remote/control` 리다이렉트(REQ-K16) 보존.
- **본 그룹은 §5.11.3 의 좌측 디렉토리 master/detail 배치를 대체한다**(그룹 K 기능 보존, 레이아웃만 진화).

### 5.14 그룹 O(v1.7) 명세 — 원격 프로그램 버전 관리

> 노드 자가 업데이트(SPEC-UPDATE-001)·그룹 D 명령·그룹 K 시스템 정보 보고를 **재사용**한다(재발명 금지). 신규는 (1) 릴리스 저장소(메타+디스크) + 무인증 GitHub-Releases 호환 피드, (2) 업데이트 소스 서버 저장(`remote.update_source`)·자동 주입, (3) 아키텍처-aware 그룹 일괄 업데이트(per-node 타깃 해석), (4) 관리 WS insecure_skip_verify 옵트인뿐이다. **변경/무결성 경로 불변**: 서버는 사전 서명 `.sig` 만 저장·배포하고 서명을 생성하지 않으며, 무결성은 노드 Ed25519 검증으로 보장된다(A22).

#### 5.14.1 릴리스 호스팅 (REQ-O01~O05)

- **저장(REQ-O01)**: `releases`(version PK·channel·notes·published_at) + `release_assets`(version·os·arch·filename·size·sha256·kind) SQLite 메타 + 디스크 `{releases_dir}/{version}/`. 업로드 시 SHA256 자동 계산, 버전별 `checksum.txt` 자동 생성.
- **노드 피드(REQ-O02, 무인증·updater 무변경 소비)**: `GET /api/v1/updates/releases/latest`(최신 stable 단일 객체) / `GET /api/v1/updates/releases`(전체, semver 내림차순) / `GET /api/v1/updates/releases/download/{version}/{filename}`(바이너리/`.sig`/`checksum.txt` 스트림). asset 객체 = `{name, size, browser_download_url}`(SPEC-UPDATE-001 checker 호환).
- **https 강제(REQ-O03)**: asset `browser_download_url` base = `remote_management.public_base_url`(우선) 또는 요청 Host(+`X-Forwarded-*`) 유도, 항상 https.
- **관리 API(REQ-O04, admin-gated)**: `GET/POST /remote/releases`, `DELETE /remote/releases/{version}`, `DELETE /remote/releases/{version}/assets/{os}/{arch}`, multipart 업로드 `POST /remote/releases/{version}/assets`(필드 os/arch/binary/signature).
- **설정(REQ-O05)**: `remote_management.public_base_url`·`remote_management.releases_dir`.

#### 5.14.2 업데이트 소스 (REQ-O06~O09)

- **저장(REQ-O06)**: `SettingsRepository` 키 `remote.update_source`(JSON `{update_url, channel}`), 별도 테이블 미신설. admin `GET/PUT /remote/update-source`.
- **자동 주입(REQ-O07)**: UpdateNode/UpdateGroup 디스패치 시 `system/update` args 에 저장된 `update_url`/채널 주입(요청 명시 시 우선).
- **https 강제(REQ-O08)**: `update_url` 은 빈 값(=해제, 노드 로컬 설정 사용) 또는 `https://` 만 허용(http 거부).
- **공개키 비전송(REQ-O09)**: 업데이트 소스/피드/명령에 공개키 미포함(노드 로컬 `update.public_key_path` 만 신뢰 앵커 — A22).

#### 5.14.3 아키텍처-aware 그룹 일괄 업데이트 (REQ-O10~O12)

- **per-node 해석(REQ-O10)**: 그룹 내 승인 노드 각각의 `os/arch`(그룹 K 보고)로 `GroupUpdatePlan` 조회 → per-node `system/update` args(`target_version`) 디스패치.
- **전략(REQ-O11)**: `latest`(채널 내 각 os/arch 최고 semver) / `pin`(단일 버전 — `DefaultVersion`, 기존 호환) / `per_arch`((os/arch)→버전 맵 `VersionByArch`, 미매핑은 `RequireMapping` 시 건너뜀).
- **부분 성공(REQ-O12)**: 타깃/자산 없는 노드는 디스패치하지 않고 사유와 함께 결과(`GroupDispatchResult.Error`)에 건너뜀 보고, 나머지는 계속 진행.

#### 5.14.4 관리 WS TLS 옵트인 (REQ-O13)

- **insecure_skip_verify(REQ-O13)**: `remote_management.insecure_skip_verify`(기본 false) → client Dialer 의 `TLSClientConfig.InsecureSkipVerify`. 자체 서명/사설망 전용 옵트인이며 전송 무결성 보장과 무관(명령=토큰, 자가 업데이트=Ed25519 로 별도 보장). 동일 원칙이 노드 자가 업데이트 다운로드 경로에도 적용된다(SPEC-UPDATE-001 M13 — `update.insecure_skip_verify`, Ed25519 검증 유지).

#### 5.14.5 보안 불변식 (요약)

- 공개키는 **노드 로컬**(`update.public_key_path`)에만 존재 — 서버/피드/소스/명령 비전송(REQ-O09).
- 서버는 **사전 서명된 `.sig` 만 저장·배포**(서명 미생성), 무결성은 노드 Ed25519 검증으로 보장(A22).
- 다운로드 URL 은 **https 강제**(피드 asset·update_url — REQ-O03/O08).
- `insecure_skip_verify`(관리 WS·자가 업데이트 다운로드)는 **TLS 인증서 검증만** 스킵하는 사설망/자체 서명 옵트인이며, 메시지/바이너리 무결성은 토큰·Ed25519 로 별도 보장(REQ-O13).

## 6. 추적성

| 요구사항 ID | 구현 위치(예정) | 검증 |
|-------------|----------------|------|
| REQ-REMOTE-A01 ~ A07 | `internal/config/types.go`/`defaults.go`/`validate.go`, `internal/remote/instance_id.go`, OnChange 훅 | config_test, instance_id_test |
| REQ-REMOTE-B01 ~ B07 | `internal/remote/client.go`(dial/백오프), `internal/remote/server.go`(online/offline), ws Hub/Client | client_test, server_test, 재연결 통합 |
| REQ-REMOTE-C01 ~ C08 | `internal/remote/server.go`(상태머신), `internal/auth/jwt.go`(토큰), `internal/api/handler/remote.go`(승인 API) | server_test(register/approve/reject/revoke), handler_test |
| REQ-REMOTE-D01 ~ D09 | `internal/remote/server.go`(디스패처/상관/타임아웃), `internal/remote/client.go`(적용), 어댑터 | dispatcher_test, apply_test, timeout_test |
| REQ-REMOTE-E01 ~ E08 | `internal/remote/client.go`(스냅샷/델타), `internal/storage/managed_node_*`, server(수신/캐시/태깅) | inventory_test, repository_test |
| REQ-REMOTE-F01 ~ F07 | TLS(wss), `jwt.go`(blacklist), `secret_fields.go`(redaction), audit 로그 | security_test, redaction_test, audit_test |
| REQ-REMOTE-G01 ~ G04 | `web/src/` 관리 UI(추후) | Vitest(추후) |
| REQ-REMOTE-H01 ~ H08 (v1.1) | `internal/remote/enrollment.go`(PreRegister/RemoveNode/enrollment 자동 승인), `internal/remote/registration.go`(handleRegister 자동 승인 분기), `internal/storage/enrollment_token_*`(해시 저장/원자 증가), `internal/api/handler/remote_enrollment.go`(REST), `internal/remote/protocol.go`/`client.go`/`internal/config/*`(enrollment_token 운반) | enrollment_test, enrollment_coverage_test, enrollment_token_sqlite_test, remote_enrollment_test |
| REQ-REMOTE-I01 ~ I12 (v1.2) | `internal/api/handler/remote.go`(원격 flow/agent CRUD REST → command 전파), `internal/remote/protocol.go`(create/update/delete action 의미), `internal/remote/client.go`(어댑터 create/update/delete 바인딩 + 시크릿 병합), `internal/remote/server.go`(편집 게이팅·실패 의미 502/503/504·결과 후 캐시 갱신), `web/src/pages/editor/EditorPage.tsx`(원격 편집 재사용)·원격 자원 페이지(생성/삭제·피드백·게이팅), audit | remote_edit_handler_test, command_action_test, adapter_bind_test, secret_roundtrip_test, edit_gating_test, EditorPage 원격 통합 Vitest |
| REQ-REMOTE-J01 ~ J08b, J16 (v1.3, 프록시/스트림/캐시) | `internal/remote/protocol.go`(query-action·스트림 메시지), `internal/remote/query_proxy.go`(query-action allowlist·게이팅·상관/타임아웃·502/503/504·TTL 캐시), `internal/remote/stream_proxy.go`(subscribe/stream_data/unsubscribe·teardown·백프레셔), `internal/remote/client.go`(query-action 매핑·redaction·스트림 소스 구독), `internal/api/handler/remote.go`(query REST + 브라우저 스트림 엔드포인트) | query_action_test(allowlist·게이팅·실패 의미), query_redaction_test, query_correlation_test, read_only_reject_test, ttl_cache_test(캐시·무효화·라이브 우회), stream_proxy_test(fan-out·teardown·백프레셔) |
| REQ-REMOTE-J09 ~ J15 (v1.3, 통합 UI) | `web/src/hooks/useFlowsTarget.ts`·`useAgentsTarget.ts`·`useDevicesTarget.ts`(타깃 추상화), `web/src/pages/{flows,agents,devices}/*ListPage.tsx`·`*DetailPanel.tsx`(target 재사용·상세 패리티·스트림 소비), `web/src/services/api/remoteService.ts`(query-action 호출·스트림 구독·폴링 폴백), `web/src/pages/remote/RemoteResourcesPage.tsx`(노드 셀렉터·라우팅) | useFlowsTarget/useAgentsTarget/useDevicesTarget Vitest, ListPage/DetailPanel 원격 타깃·스트림 소비 Vitest, 노드 셀렉터·target 라우팅 Vitest |
| REQ-REMOTE-K01 ~ K06 (v1.4, 그룹핑) | `internal/storage/managed_node_sqlite.go`/`managed_node_repository.go`(group_name 컬럼·마이그레이션·SetNodeGroup/ListGroups/ListNodesByGroup), `internal/api/handler/remote.go`(PUT/DELETE group·GET groups·그룹별 노드 REST, admin-gated) | managed_node_group_test(배정/해제/distinct/그룹별/재라벨링/환원), remote_group_handler_test(admin 게이팅) |
| REQ-REMOTE-K07 ~ K10 (v1.4, 시스템 정보+운영 요약) | `internal/remote/protocol.go`(register/heartbeat os/arch/started_at 선택 필드), `internal/remote/client.go`(보고), `internal/remote/server.go`/`registration.go`(수신·저장·uptime 파생), `internal/storage/managed_node_*`(os/arch/started_at 저장·운영 요약 미러 파생), `internal/api/handler/remote.go`(노드 상세=시스템 정보+운영 요약) | system_info_report_test(register/heartbeat 보고·하위 호환 빈값), node_summary_test(미러 파생 카운트/상태 분해·오프라인 last-known), uptime_derive_test |
| REQ-REMOTE-K11 ~ K16 (v1.4, UI 재편) | `web/src/components/layout/Sidebar.tsx`(노드 관리+등록 관리 재편), `web/src/pages/remote/NodeManagementPage.tsx`(디렉토리/대시보드)·`EnrollmentManagementPage.tsx`(토큰+등록), `web/src/components/remote/NodeDashboard.tsx`(시스템+운영+M8 서브탭), `web/src/services/api/remoteService.ts`/`useRemote.ts`(group/summary), `web/src/router.tsx`(라우트 재편·딥링크 보존) | Sidebar IA Vitest(노드 관리/등록 관리·게이팅), NodeManagementPage Vitest(디렉토리/그룹 배정/대시보드), NodeDashboard Vitest(시스템/운영/서브탭 M8 재사용), EnrollmentManagementPage Vitest, router 리다이렉트/딥링크 Vitest |
| REQ-REMOTE-L01 ~ L03 (v1.5, config 프록시) | `internal/remote/protocol.go`(dashboard query allowlist get_shared/get_mine·변경 action 배제), `internal/remote/query.go`/`client_query.go`(dashboard read 매핑·redaction), `cmd/xflowd/*`(노드 대시보드 read 소스 바인딩), `internal/api/handler/remote.go`(dashboard query REST·게이팅·503/504/502/404·TTL 캐시) | dashboard_query_test(allowlist·게이팅·redaction·실패 의미·404·노드-로컬 권위·변경 action 거부), dashboard_cache_test |
| REQ-REMOTE-L04 ~ L09 (v1.5, 패널 target-aware) | `internal/remote/protocol.go`(monitor.metrics query·chart/monitor.logs stream allowlist), `internal/remote/query.go`/`client_query.go`(monitor.metrics 매핑), `internal/remote/client_stream.go`(chart/logs 스트림 소스·teardown·백프레셔), `cmd/xflowd/*`(소스 어댑터 바인딩), `web/src/pages/dashboard/DashboardPage.tsx`(target 전파)·`panels/*`(target-aware)·`charts/useChartChannel.ts`(스트림 어댑터 주입) | metrics_query_test, chart_stream_test(backfill/append 중계·teardown·폴백), logs_stream_test, panel_target_test(로컬 바이트 동일·원격 분기 Vitest), useChartChannel 원격 어댑터 Vitest, control 패널 그룹 D 라우팅 Vitest |
| REQ-REMOTE-L10 ~ L13 (v1.5, UI 통합·게이팅) | `web/src/components/remote/NodeDashboard.tsx`(대시보드 서브탭·로컬 DashboardPage 재사용), `web/src/hooks/useTargetGating.ts`(재사용), `web/src/services/api/remoteService.ts`(config/metrics/logs/chart 클라이언트), audit(최소) | NodeDashboard 대시보드 서브탭 Vitest(게이팅·원격 컨텍스트·read-only·서브탭 진입), 503/504/502/404 실패 표시 Vitest, 최소 감사(노출 위반/오류만) 검증 |
| REQ-REMOTE-M01 ~ M03 (v1.6, 해상도 보고) | `internal/remote/protocol.go`(register/heartbeat display_width/height 선택 필드), `internal/config/types.go`/`defaults.go`(`display.resolution`/`width`/`height` 키), `internal/remote/client.go`(config→페이로드 보고), `internal/remote/server.go`/`registration.go`(수신·저장), `internal/storage/managed_node_*`(display_width/height 컬럼·마이그레이션), `internal/api/handler/remote.go`(NodeDetail 노출), `web/src/types/remote.ts`(NodeDetail 필드) | resolution_report_test(register/heartbeat 보고·저장·하위 호환 0/미보고·잘못된 형식 폴백), NodeDetail 노출 |
| REQ-REMOTE-M04 ~ M06 (v1.6, 관리자 뷰 레이아웃, 프론트) | `web/src/pages/remote/NodeManagementPage.tsx`(좌측 디렉토리 → 상단 바 셸 진화), `web/src/components/remote/NodePicker.tsx`(신규 — 그룹 묶음 드롭다운, M9 그룹핑 재사용), `web/src/components/remote/ManagerViewTopBar.tsx`(신규 — 노드 피커+서브탭 네비+관리 액션+나가기), `web/src/components/remote/NodeDashboard.tsx`(서브탭 네비 상단 바로 호이스팅), `web/src/components/layout/Sidebar.tsx`/`AppLayout.tsx`·`web/src/stores/uiStore.ts`(관리자 뷰 진입 시 전역 사이드바 숨김/복원) | ManagerView 레이아웃 Vitest(상단 바·풀폭 노드 화면·좌측 크롬 제거), NodePicker Vitest(그룹 묶음·전환), 전역 사이드바 숨김/복원 Vitest, 딥링크 보존 Vitest |
| REQ-REMOTE-M07 ~ M09 (v1.6, 고정 캔버스 충실 재현, 프론트) | `web/src/components/remote/FixedCanvasScaler.tsx`(신규 — 노드 해상도 고정 캔버스 + 레터박스 transform scale), `web/src/components/remote/RemoteDashboardView.tsx`(또는 NodeDashboard 대시보드 서브탭, FixedCanvasScaler 로 감싸기), `web/src/hooks/useRemote.ts`(NodeDetail display_width/height 소비) | FixedCanvasScaler Vitest(고정 캔버스 크기=노드 해상도·레터박스 종횡비 보존·stretch/crop 금지·미보고 폴백), 패널 데이터/게이팅/READ-ONLY 불변 Vitest(그룹 L 경로 재사용) |
| REQ-REMOTE-M10 (v1.6, 마이그레이션) | `web/src/pages/remote/NodeManagementPage.tsx`(제자리 진화·M9 기능 보존), `web/src/router.tsx`(라우트·딥링크·리다이렉트 보존), EnrollmentManagementPage 불변 | 마이그레이션 Vitest(M9 기능 보존: 그룹핑/그룹 배정/개요/서브탭/딥링크), 등록 관리 회귀 0, 라우트/리다이렉트 보존 |
| REQ-REMOTE-O01 ~ O05 (v1.7, 릴리스 호스팅) | `internal/api/handler/release_feed.go`(무인증 GitHub-Releases 호환 피드·https 강제), `internal/api/handler/release_admin.go`(admin 릴리스/자산 관리·multipart 업로드), `internal/storage`(`releases`/`release_assets` + 디스크 `{releases_dir}/{version}/`·SHA256·checksum.txt), `internal/config/types.go`/`config.go`/`defaults.go`(`public_base_url`/`releases_dir`), `cmd/xflowd/main.go`(피드/관리 라우트 배선) | release_feed_test(latest/list/download·https 강제·asset 형태), release_admin_test(목록/생성/삭제/자산 삭제/multipart 업로드·admin 게이팅), release_store_test(SHA256·checksum.txt 자동 생성) |
| REQ-REMOTE-O06 ~ O09 (v1.7, 업데이트 소스) | `internal/api/handler/remote_version.go`(`GET/PUT /remote/update-source`·`remote.update_source` 저장/조회·자동 주입·https 검증), `internal/storage`(SettingsRepository) | update_source_test(저장/조회·자동 주입·http 거부·빈 값 해제·공개키 비전송) |
| REQ-REMOTE-O10 ~ O12 (v1.7, 그룹 일괄 업데이트) | `internal/remote/grouping.go`(`GroupUpdatePlan`·`DispatchGroupUpdate`·per-node OS/Arch 타깃 해석·부분 성공), `internal/remote/protocol.go`(`SystemUpdateArgs{target_version,channel,update_url,restart}`) | grouping_test(latest/pin/per_arch·per-arch 타깃·미매핑 건너뜀·부분 성공) |
| REQ-REMOTE-O13 (v1.7, 관리 WS TLS 옵트인) | `internal/remote/client.go`(`newGorillaDialer(insecureSkipVerify)`), `internal/config/types.go`/`config.go`/`defaults.go`(`remote_management.insecure_skip_verify`) | client_dialer_test(insecure_skip_verify 옵트인·기본 false) |
| REQ-REMOTE-N01 ~ N04 | server 연결 관리, 엔드포인트 분리, disabled 회귀, Message 봉투 | 부하/회귀 + ws 호환 |
