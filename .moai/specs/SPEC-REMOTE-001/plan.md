# SPEC-REMOTE-001 구현 계획 (plan.md)

> 원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링
> 본 문서는 PLAN 단계 산출물이다. 코드는 포함하지 않으며 기술 접근/마일스톤/위험을 정의한다.

> **상태(2026-06-06)**: 4개 확정 아키텍처 결정(영속 WS dial / 라이브 RPC over 연결 / 인벤토리 미러+서버 DB 캐시 / 클라이언트 노출 opt-in)을 고정 제약으로 둔다. 마일스톤 M1~M6·H·M7 백엔드 완료, M7 웹(7.4)·M8(그룹 J, v1.3) 계획 단계. M8 은 원격 노드 FULL 제어 패리티(READ/QUERY 프록시 + 통합 UI)를 증분 구현한다.

## 1. 기술 접근

### 1.1 신규 모듈 — `internal/remote` (server/client/protocol)

- **protocol**(`internal/remote/protocol.go`): 관리 메시지 Type 상수와 페이로드 구조를 정의한다. 전송 봉투는 기존 `internal/api/ws.Message{Type,Payload,Timestamp}` + `NewMessage` 를 재사용한다(REQ-REMOTE-N04). 신규 메시지 버스를 만들지 않는다.
- **server**(`internal/remote/server.go`): 노드 연결 수락(별도 WS 엔드포인트), 등록/승인 상태 머신, 명령 디스패처(상관 id·타임아웃), 인벤토리 수신→DB 캐시, 노드 online/offline 추적.
- **client**(`internal/remote/client.go`): `server_url` dial + 백오프 재연결, 인증 핸드셰이크, 등록 요청, heartbeat, 인벤토리 스냅샷/델타 송신, 명령 수신→로컬 어댑터 적용→결과 반환.
- **instance_id**(`internal/remote/instance_id.go`): 영속 instance_id 생성/로드.

### 1.2 전송 — 기존 gorilla/websocket Hub/Client 재사용

- 관리 채널은 **클라이언트가 서버로 dial 하는 영속 WS**(결정 1). 클라이언트는 outbound 만 필요(NAT/방화벽 친화).
- 기존 `Hub`(Register/Unregister/Broadcast/BroadcastMessage)·`Client`(ReadPump/WritePump, ping 54s/pong 60s)를 서버 측 노드 연결 관리에 재사용한다.
- 관리 WS 와 웹 UI 모니터링 WS 는 **별도 엔드포인트**로 분리한다(REQ-REMOTE-N02). 모니터링 WS 흐름과 섞지 않는다.
- 클라이언트 dial+재연결+프레이밍은 `internal/agent/socket/`, MQTT(paho auto-reconnect/backoff), HVAC TCP 에이전트의 재연결/백오프 패턴을 준용한다.

### 1.3 인증 — 기존 JWTService 재사용

- 승인 시 노드 토큰을 `JWTService.GenerateTokens(username, role)`(HS256)로 발급하고, 재접속은 `ValidateToken` 으로 인증, 폐기는 `Blacklist`/`IsBlacklisted` 로 처리한다(REQ-C04/C05/C07/F07).
- 핸드셰이크 인증은 `internal/api/handler/websocket.go` 의 Bearer/`?token=` 패턴을 준용하되 **별도 관리 엔드포인트**에서 수행한다(REQ-B02).
- (선택) 부트스트랩 enrollment 시크릿으로 등록 1차 신뢰 검증(REQ-C08). mTLS/핀닝은 OPEN QUESTION.

### 1.4 원격 제어 — 라이브 RPC over 연결 → 로컬 어댑터

- 서버는 `command`(command_id, target instance_id, domain, action, args)를 라이브 연결로 디스패치(결정 2, REQ-D01).
- 클라이언트는 출처 검증 후 `domain` 에 따라 기존 어댑터로 라우팅한다:
  - flow → `FlowServiceAdapter`(Create/Get/List/Update/Delete/Deploy/Start/Stop/Pause, REQ-D02).
  - agent → `AgentServiceAdapter`(List/Get/Create/Update/Delete/Start/Stop/Restart, REQ-D03).
  - device → device 핸들러(`internal/api/handler/device.go`, REQ-D04).
- 적용은 로컬 API 와 동일 검증/제약을 받는다(원격 우회 없음 — A5). 결과는 `command_result` 로 상관 id 와 함께 반환(REQ-D05/D07). 타임아웃은 미적용 처리(REQ-D06). 실패는 부분 적용 없이 오류 보고(REQ-D09).
- 역터널·공유 DB 없음.

### 1.5 인벤토리 미러링 — 클라이언트 push + 서버 DB 캐시

- 클라이언트는 접속 시 `inventory_snapshot`, 변경 시 `inventory_delta` 를 노출 범위로 push 한다(결정 3·4, REQ-E01/E02/E07).
- 서버는 이를 `mirrored_flows`/`mirrored_agents`/`mirrored_devices` 에 캐시하고, 모든 행을 `source_instance_id` 로 태깅한다(REQ-E03/E04).
- 오프라인 시 행을 삭제하지 않고 `online=false`·`last_seen` 만 갱신해 last-known 을 보존한다(REQ-E06).
- 서버 측 편집은 노드 명령(그룹 D)으로 전파하고, 명령 결과 수신 후에야 캐시를 갱신한다(서버 단독 영속 금지 — REQ-E08, A4).

### 1.6 신규 인스턴스 식별자 — instance_id

- 전역 인스턴스 식별자가 없으므로(디바이스 단위 UUID 만 존재) 영속 instance_id(UUID)를 신설한다(REQ-A03).
- 최초 기동 1회 생성 → 로컬 영속(파일 또는 SQLite 메타). 재시작/재접속 간 불변. `remote_management.instance_id` 로 명시 지정 가능.
- IoT 디바이스 UUID(`DeviceIDRepository`)와 별개 네임스페이스. 용어 엄격 구분(managed node vs IoT device, spec §1.4).

### 1.7 설정 — `remote_management` 섹션

- `internal/config/types.go` 에 `RemoteManagementConfig`(mode, server_url, instance_id, auto_register, heartbeat_interval, bootstrap_secret, exposure, tls)를 추가하고 `Config` 에 편입.
- `defaults.go`/`validate.go` 에 기본값/검증 추가. 핫리로드는 기존 `OnChange`/`WatchConfig` 재사용(REQ-A06/A07).
- 시크릿(bootstrap_secret/발급 토큰)은 redaction·비커밋 대상(REQ-F06).

### 1.8 저장 — ManagedNodeRepository + 미러 테이블 (서버)

- `FlowRepository` 패턴(Save/Get/List/Delete/Close)을 준용한 `ManagedNodeRepository`(인터페이스 + sqlite 구현)를 추가한다.
- 테이블: `managed_nodes`(등록·상태·식별), `mirrored_flows`/`mirrored_agents`/`mirrored_devices`(출처 태깅 미러). `factory.go` 에 배선.

### 1.9 배선 — cmd/xflowd/main.go

- `cmd/xflowd/main.go`(~700–800)에서 `remote_management.mode` 분기:
  - `server`: 승인/목록 API 라우트(`RegisterRoutes`) + 관리 WS 핸들러(`RegisterRawHandler`) 등록, `ManagedNodeRepository` 기동.
  - `client`: `internal/remote` 클라이언트 기동(dial/등록/heartbeat/명령 적용).
  - `disabled`: 어떤 관리 연결도 생성/수락하지 않음(기본, 회귀 0 — REQ-N03).

### 1.10 원격 자원 편집 — 명령 디스패치 경유 FULL CRUD (v1.2, 그룹 I)

- **원칙(불변)**: 서버는 미러 자원을 **직접 영속 편집하지 않는다**. 모든 생성/수정/삭제는 그룹 D `command` 로 (온라인) 노드에 전파되어 노드 어댑터(`FlowServiceAdapter`/`AgentServiceAdapter`, 영속 소유자)가 로컬 적용하고, **명령 결과 수신 후에만** 서버가 미러 캐시를 갱신한다(REQ-E08·A4 경로 재사용, REQ-I01~I04).
- **인프라 재사용**: `command` 프로토콜은 임의 domain/action 을 이미 지원하고, 클라이언트 어댑터는 Create/Update/Delete 를 이미 보유한다. 신규 작업은 4가지로 한정된다 — (1) flow/agent 도메인의 `create`/`update`/`delete` action 의미 확정(`protocol.go`), (2) 신규 서버 REST 엔드포인트(`remote.go`), (3) 해당 action 의 클라이언트 어댑터 바인딩(`client.go`), (4) 웹 편집기 통합(`EditorPage.tsx` + 원격 자원 페이지).
- **REST 표면**(admin-gated, 기존 `/remote/*` 인증 재사용 — REQ-F04):
  - flow: `POST /remote/nodes/{id}/flows`(201) · `PATCH /remote/nodes/{id}/flows/{flow_id}` · `DELETE /remote/nodes/{id}/flows/{flow_id}`(204).
  - agent: `POST /remote/nodes/{id}/agents`(201) · `PATCH /remote/nodes/{id}/agents/{agent_id}` · `DELETE /remote/nodes/{id}/agents/{agent_id}`(204).
- **게이팅**(REQ-I05): 승인(approved) ∧ 온라인(online) 노드만 허용. 수정/삭제 대상은 노출 범위(exposure scope) 내여야 함(REQ-D08/E07 일관). 위반 시 명령 미디스패치 + 명확한 오류.
- **실패 의미**(REQ-I11): 오프라인 → 503, 명령 타임아웃(REQ-D06) → 504, 노드 어댑터 적용 실패(REQ-D09) → 502(노드 사유 전달). 어느 실패에서도 미러 캐시 비갱신(성공 결과 시에만 — REQ-E08).
- **시크릿 redaction 라운드트립**(REQ-I07·F06): 미러 정의는 시크릿 마스킹 상태. 편집 저장 시 마스킹/미변경 시크릿 필드를 **갱신 페이로드에서 생략**하고, 노드는 수신 정의를 **기존 시크릿과 병합**(부재 필드는 기존값 유지)해 적용한다. 마스킹 자리표시자의 영속 금지. 1차 권고 메커니즘: 페이로드 필드 부재 처리 + 노드 측 기존 정의 로드 후 병합(어댑터 진입 전).
- **웹 편집기 통합**(REQ-I08~I10): 기존 시각 편집기(`EditorPage.tsx`, React Flow)를 재사용해 원격 플로우를 동일 캔버스로 열고 저장(로컬 vs `instance_id` 노드 저장 대상 구분). 에이전트는 편집·설정 surface 제공. 원격 자원 페이지에 생성/삭제 액션·성공/실패 피드백·상태 게이팅(승인/온라인/노출).
- **감사**(REQ-I12·F05): 원격 생성/수정/삭제(누가/언제/노드/자원/action)를 감사 기록, 시크릿 값 비포함(REQ-F06).

### 1.11 원격 노드 FULL 제어 패리티 — query-action 프록시 + 스트리밍 + TTL 캐시 + 통합 UI (v1.3, 그룹 J)

> **OQ-J1~J7 전부 RESOLVED(2026-06-06)** — 아래는 확정 설계다. 이전 초안 대비 **변경점**: (1) HTTP-over-WS path 프록시 → **per-domain query-action**(OQ-J1), (2) 최소 allowlist → **FULL 커버리지**(OQ-J2), (3) 폴링만/스트리밍 연기 → **스트리밍 프록시 포함**(OQ-J3), (4) v1 pass-through → **단기 TTL 캐시 처음부터**(OQ-J6).

- **목표**: 원격 노드의 플로우/에이전트/디바이스를 **별도 원격 페이지가 아니라 로컬과 동일한 웹 UI**(로컬 목록 페이지 + 상세 패널)로 제어한다. 목록·라이프사이클(그룹 D/E/I)을 넘어 **상세 패널까지 FULL 패리티**를 확장한다(에이전트 통계/설정/디바이스/토픽/store/세션/시리즈, 디바이스 실시간 상태/명령/메타데이터, 플로우 노드 레벨 런타임/로그).
- **신규 READ/QUERY 프록시(서버↔노드, REQ-J01~J07, J16)**: 미러(그룹 E, 요약 정의만)·명령(그룹 D, 변경 전용)과 **별개**인 READ-ONLY RPC(`query`/`query_result`)를 신설한다. 아키텍처는 **per-domain query-action**(OQ-J1 RESOLVED): 서버가 `query{domain, query_action, args}`(그룹 D command 와 대칭)를 노드로 전파 → 노드가 각 query-action 을 **자신의 로컬 read 핸들러로 매핑·실행** → redaction(REQ-J06) 된 JSON 회신. raw HTTP path 프록시는 사용하지 않는다.
  - **query-action allowlist(FULL 커버리지, REQ-J04, OQ-J2)**: 도메인별 **열거 집합** — flow: get/nodes/status/logs(+list); agent: get/stats/config/devices/topics/store/sessions/series(+list); device: get/state/commands/metadata(+list). 노드 측 매핑 표는 spec §5.10.1. 변경 의미 action 명시 배제. list 는 미러 우선·라이브 보강.
  - **게이팅(REQ-J05)**: 승인 ∧ 온라인 ∧ 노출 범위(REQ-E07/A06 일관).
  - **실패 의미(REQ-J07)**: offline→503, timeout→504, node-error→502(그룹 I `mapRemoteCommandError` 재사용).
  - **단기 TTL 캐시(REQ-J16, OQ-J6 RESOLVED — 처음부터)**: 서버가 query-action 응답(redacted)을 `{instance_id, domain, query_action, args}` 키로 단기 TTL 캐시. 정적/완만 action 만 캐시, **라이브 action(state/stats/series)은 캐시 우회**. 관련 변경(그룹 D/I) 성공 시 무효화. 게이팅은 캐시 적중 시에도 매 요청 평가.
- **스트리밍 프록시(REQ-J08/J08b, OQ-J3 RESOLVED — 포함)**: 노드의 라이브 push(디바이스 실시간 상태, 에이전트 라이브 통계/시리즈)를 서버 경유로 브라우저에 중계. `subscribe{subscription_id, domain, stream_action, args}` → 노드 소스 구독 → `stream_data{subscription_id, payload(redacted)}` push → 서버 fan-out → 브라우저. `unsubscribe`/노드 오프라인 시 **teardown**, 느린 소비자 **백프레셔**(coalesce/drop/속도 제한), 구독 누수 방지. 스트림도 READ-ONLY·redaction·캐시 우회. **폴링은 폴백**(스트림 미지원/실패 시 query-action 폴링).
- **통합 UI(REQ-J09~J14)**: `useEditorFlowTarget`(이미 local vs remote 분기) 패턴을 목록/제어로 확장 → `useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget`(타깃 추상화). `FlowListPage`/`AgentListPage`/`DeviceListPage` + 상세 패널이 `target`(local | remote:{instanceId})을 받아 원격에서도 동일 렌더링(별도 화면 미신설). 데이터 소스는 로컬 API vs (query-action + 스트림 구독), 액션은 로컬 vs 그룹 D 명령 / 그룹 I(M7) CRUD 로 투명 전환(REQ-J12). 노드 셀렉터 + `?target=remote:{instanceId}` 라우팅·네비게이션. 기존 `RemoteResourcesPage` 는 노드 셀렉터로 재용도화/폐기(REQ-J14).
- **변경 경로 불변(OQ-J4 RESOLVED)**: 디바이스 쓰기(명령·메타데이터) 포함 모든 변경은 **그룹 D 명령 / 그룹 I CRUD 로만**(프록시 경유 변경 금지 — REQ-J03/J12, 신규 쓰기 경로 미신설).
- **감사(REQ-J15, OQ-J5 RESOLVED)**: 일반 read 미감사, **노출 위반·오류 접근만** 로깅. 변경은 기존 감사 유지. 시크릿 비포함.
- **인가(OQ-J7 RESOLVED)**: 서버 admin 게이팅(REQ-F04 재사용) + 노드 세션 권위 실행. 사용자 단위 인가 매핑은 v1.3 비도입.

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

> 큰 기능이므로 인스턴스 식별·설정·연결 → 등록/승인·인증 → 원격 명령 → 인벤토리 미러 → 서버 UI 순으로 분할한다. 각 마일스톤은 독립 검증 가능 단위로 둔다.

### 마일스톤 1 — 우선순위 High: 인스턴스 식별 + 설정 + 연결 라이프사이클
- 영속 instance_id 생성/로드(`internal/remote/instance_id.go`).
- `remote_management` 설정 섹션(types/defaults/validate) + 핫리로드 훅.
- 클라이언트 dial + 백오프 자동 재연결 + heartbeat, 서버 online/offline 추적(기존 Hub/Client 재사용).
- 검증: instance_id_test, config_test, client/server 연결·재연결 테스트.
- 의존: 없음. 이후 모든 마일스톤의 토대.
- 매핑: REQ-A01~A07(설정/식별), B01~B07(연결), N02~N04.

### 마일스톤 2 — 우선순위 High: 등록 & 승인 + 인증
- `register`/`register_ack` 프로토콜, 서버 등록/승인 상태 머신(pending/approved/rejected/revoked).
- 관리자 승인/거부/폐기 API(`internal/api/handler/remote.go`) + 노드 토큰 발급/검증/폐기(JWTService 재사용).
- 재접속 재인증·세션 복원, (선택) 부트스트랩 시크릿.
- `managed_nodes` 테이블 + `ManagedNodeRepository`.
- 검증: server_test(register/approve/reject/revoke), handler_test, jwt 재사용 테스트.
- 의존: 마일스톤 1(연결).
- 매핑: REQ-C01~C08, F02/F03/F07.

### 마일스톤 3 — 우선순위 High: 원격 명령 디스패치/적용
- `command`/`command_result` 프로토콜, 서버 디스패처(상관 id·타임아웃), 클라이언트 적용(어댑터 라우팅).
- flow/agent/device 도메인 명령을 기존 어댑터로 적용. 권한(서버 발행/승인 노드 수신) 강제. 실패/타임아웃 처리.
- 검증: dispatcher_test(상관/타임아웃), apply_test(각 도메인), 권한/실패 케이스.
- 의존: 마일스톤 2(승인 노드만 명령).
- 매핑: REQ-D01~D09, F04.

### 마일스톤 4 — 우선순위 Medium: 인벤토리 미러링 + 서버 목록
- `inventory_snapshot`/`inventory_delta`(노출 범위, redacted) 클라이언트 송신.
- 서버 수신→`mirrored_flows`/`mirrored_agents`/`mirrored_devices` 캐시, source_instance_id 태깅, last-known/오프라인 표시.
- 서버 편집→명령 전파(E08), 노출 변경 재미러링(A07).
- 검증: inventory_test(스냅샷/델타/노출 준수), repository_test(태깅/last-known), 편집→명령 전파.
- 의존: 마일스톤 2·3.
- 매핑: REQ-E01~E08, A04/A07, F06.

### 마일스톤 5 — 우선순위 Medium: 서버 웹 UI
- 관리 노드 목록, pending 승인 뷰, 노드별 자원 목록(디바이스 태그), 명령/상태 피드백.
- 구현은 expert-frontend 가 담당(본 SPEC 은 계약 정의). 관리 WS/REST API 소비.
- 검증: Vitest + Testing Library(목록/승인/태깅 표시/상태 피드백).
- 의존: 마일스톤 2·3·4(API/데이터 계약).
- 매핑: REQ-G01~G04.

### 마일스톤 6 — 우선순위 Low(최종 목표): 보안·감사·통합·회귀 ✅ 완료(2026-06-05)
- TLS(wss) 강제 옵션(remote_management.require_secure + 검증), 원격 변경 감사 로그
  (remote_audit 테이블 + GET /api/remote/audit), 시크릿 redaction 검증.
- 토큰 하드닝: JWT jti 클레임 + jti 블랙리스트 → managed_nodes.token_id 에 원본 토큰
  대신 jti 저장(DB-안전 폐기). 웹 어드민 토큰 하위 호환(jti 없는 레거시 토큰 유효).
- disabled 회귀(기존 동작 불변), 다중 노드 확장성(N 동시 연결 -race + SQLite busy_timeout),
  end-to-end(등록→승인→명령→적용→결과→미러, 폐기→재인증 거부).
- 검증: jwt_jti_test, jwt_issuer_jti_test, remote_audit_sqlite_test, audit_test,
  remote_admin_audit_test, remote_secure_test, integration_m6_test, regression_m6_test.
- 의존: 전 마일스톤.
- 매핑: REQ-F01/F05/F06/F07, N01/N03.

### 마일스톤 H — v1.1 확장: 수동 enrollment ✅ 완료(2026-06-06)
- 사전 등록(allow-list): `internal/remote/enrollment.go`(PreRegister/RemoveNode) + 사전 등록
  노드 접속 시 자동 승인 분기(`registration.go` handleRegister).
- enrollment 토큰: `internal/storage/enrollment_token_repository.go` +
  `enrollment_token_sqlite.go`(SHA-256 해시 저장, max_uses 조건부 원자 증가) +
  팩토리 배선. `internal/api/handler/remote_enrollment.go`(발급 1회 노출/목록 은닉/폐기,
  admin-gated) + 토큰 검증·자동 승인(`enrollment.go` tryEnrollmentAutoApprove).
- 프로토콜/설정: `protocol.go` RegisterPayload.EnrollmentToken,
  `client.go` ClientConfig.EnrollmentToken(register 운반), `config/types.go`+`config.go`+
  `defaults.go` remote_management.enrollment_token.
- 신규 파일: internal/storage/enrollment_token_repository.go,
  internal/storage/enrollment_token_sqlite.go,
  internal/storage/enrollment_token_sqlite_test.go,
  internal/remote/enrollment.go, internal/remote/enrollment_test.go,
  internal/remote/enrollment_coverage_test.go,
  internal/api/handler/remote_enrollment.go,
  internal/api/handler/remote_enrollment_test.go.
- 수정 파일: internal/remote/protocol.go, internal/remote/server.go,
  internal/remote/registration.go, internal/remote/client.go,
  internal/config/types.go, internal/config/config.go, internal/config/defaults.go,
  cmd/xflowd/main.go.
- 검증: enrollment_test, enrollment_coverage_test, enrollment_token_sqlite_test,
  remote_enrollment_test(신규 코드 함수-평균 커버리지 ≥91%, -race, go vet + golangci-lint clean).
- 의존: M2(등록/승인 상태 머신), M6(감사).
- 매핑: REQ-H01~H08, REQ-C08/F04/F06/F07 일관.

### 마일스톤 7 — v1.2 확장: 원격 자원 편집 (FULL CRUD via 명령 디스패치) — 백엔드(7.1~7.3) ✅ 완료 / 웹(7.4) 🔲 계획
> 그룹 I. 관리자가 서버 웹 UI 에서 승인·온라인 노드의 플로우/에이전트를 생성·수정·삭제한다. **서버 단독 영속 절대 금지** — 모든 편집은 그룹 D 명령 → 노드 어댑터 로컬 적용 → 결과 후에만 미러 캐시 갱신(REQ-E08·A4). 기존 시각 편집기(EditorPage.tsx) 재사용. 4단계(프로토콜→서버→클라이언트→웹)로 분할한다.

- **단계 7.1 — 프로토콜(action 의미)** ✅: `internal/remote/protocol.go` 에 flow/agent 도메인 `ActionCreate`/`ActionUpdate`/`ActionDelete` 상수·의미·시크릿 생략 규약("필드 부재 + 노드 backfill") 확정. 신규 메시지 타입 없이 기존 `command`/`command_result` 재사용(REQ-I06, N04 유지).
  - 검증: 상수 값이 기존 어댑터 리터럴과 일치(와이어 호환), 페이로드 인코딩.
- **단계 7.2 — 서버 REST + 전파/게이팅/실패 의미** ✅: `internal/api/handler/remote_editing.go`(신규) 에 flow/agent CRUD 엔드포인트(POST/PATCH/DELETE) 추가, `internal/remote/editing.go`(신규) 가 노출 범위 게이팅(REQ-I05 — 미러 존재) + 결과 후에만 미러 캐시 갱신(REQ-E08 — UpsertMirror/DeleteMirror) 제공. Dispatch 경유 502/503/504 매핑(REQ-I11, mapRemoteCommandError 재사용) + 명령 감사(REQ-I12, dispatch recordCommandAudit, actor 전파). `internal/api/router.go` PATCH 메서드 추가. server 모드 배선(cmd/xflowd/main.go).
  - 검증: remote_editing_test(6 엔드포인트 happy path, 미승인/오프라인→503·범위 밖→404·504·502, create 미러 비강제, 실패 시 캐시 불변, actor 전파), editing_test(미러 mutator + 노출 범위).
- **단계 7.3 — 클라이언트 어댑터 바인딩 + 시크릿 병합** ✅: `cmd/xflowd/remote_commands.go` 의 flow/agent commander 가 create/update/delete action → `FlowServiceAdapter`/`AgentServiceAdapter` Create/Update/Delete 라우팅(REQ-I06, 기존 바인딩 재사용), update 시 `mergeSecrets`(신규) 가 GetFlow/GetAgent 로 기존 정의를 로드해 생략 시크릿을 backfill 후 어댑터 적용(REQ-I07). 적용은 로컬 API 와 동일 검증(A5).
  - 검증: remote_secret_merge_test(마스킹 생략→기존값 backfill, 제공 시크릿 유지, 중첩 backfill, agent backfill, Definition 미포함 시 backfill 생략).
- **단계 7.4 — 웹 편집기 통합(추후 expert-frontend)**: `web/src/pages/editor/EditorPage.tsx` 원격 플로우 열기/저장(로컬 vs 노드 구분, REQ-I08), 에이전트 편집·설정 surface(REQ-I09), 원격 자원 페이지 생성/삭제·피드백·게이팅(REQ-I10).
  - 검증: EditorPage 원격 통합 Vitest, 원격 자원 페이지 생성/삭제·피드백·게이팅 Vitest.
- 의존: M2(승인 상태 머신), M3(명령 디스패치/상관/타임아웃), M4(미러 캐시·E08 전파 경로), M6(감사·시크릿 redaction).
- 매핑: REQ-I01~I12, REQ-E08/D06/D08/D09/F04/F05/F06 일관.

### 마일스톤 8 — v1.3 확장: 원격 노드 FULL 제어 패리티 (query-action 프록시 + 스트리밍 + TTL 캐시 + 통합 UI) 🔲 계획
> 그룹 J(OQ-J1~J7 RESOLVED). 원격 노드의 플로우/에이전트/디바이스를 **로컬과 동일한 웹 UI**(로컬 목록 페이지 + 상세 패널 재사용)로 FULL 제어한다. 핵심은 (1) 미러(그룹 E)·명령(그룹 D)과 **별개인 READ-ONLY per-domain query-action 프록시**(온디맨드 상세/통계/노드 레벨, FULL 커버리지) + **단기 TTL 캐시**, (2) **스트리밍 프록시**(라이브 디바이스 상태·에이전트 시리즈를 서버 경유 중계), (3) `useEditorFlowTarget` 의 target 추상화를 목록/제어 페이지로 확장이다. **변경은 여전히 그룹 D/I 경로**(프록시 경유 변경 금지). 4단계(프로토콜→서버 프록시+캐시+스트림 중계→클라이언트 query+stream 핸들러→웹 통합)로 분할한다.

- **단계 8.1 — 프록시 프로토콜(query-action + 스트림)** 🔲: `internal/remote/protocol.go` 에 (a) READ-ONLY query 메시지(`query`/`query_result`, 페이로드 `{domain, query_action, args, query_id}` — 그룹 D command 와 대칭, raw path 아님)와 (b) 스트림 메시지(`subscribe`/`stream_data`/`unsubscribe`, `subscription_id`) 정의. 기존 `command`/`command_result`(변경)와 역할 분리, 봉투/상관/타임아웃 패턴 공유(REQ-J01/J02/J08, N04 유지). 도메인별 지원 query-action·stream-action 열거(allowlist).
  - 검증: query/stream 인코딩·상관 id(query_id/subscription_id) 라운드트립, 미열거/변경 의미 action 거부 규약(READ-ONLY, REQ-J03).
- **단계 8.2 — 서버 query 프록시 + TTL 캐시 + 스트림 중계** 🔲: `internal/remote/query_proxy.go`(신규) 가 query-action allowlist(REQ-J04) 강제 + 게이팅(승인 ∧ 온라인 ∧ 노출 범위, REQ-J05) + 상관/타임아웃(REQ-J02) + 502/503/504(REQ-J07, `mapRemoteCommandError` 재사용) + **단기 TTL 캐시(REQ-J16: `{instance_id,domain,action,args}` 키, 라이브 action 우회, 변경 시 무효화)** 제공. `internal/remote/stream_proxy.go`(신규) 가 subscribe/stream_data/unsubscribe **fan-out**·subscription 레지스트리·**disconnect teardown**·**백프레셔**(REQ-J08/J08b). `internal/api/handler/remote.go`(수정) 에 query REST(도메인별 read) + 브라우저↔서버 스트림 엔드포인트(WS/SSE) 추가, admin-gated(REQ-F04). server 모드 배선(cmd/xflowd/main.go). 최소 감사(REQ-J15: 위반/오류만).
  - 검증: query_action_test(allowlist 일치/불일치, 미승인·오프라인→503·범위 밖 거부·504·502, READ-ONLY 거부), ttl_cache_test(적중·무효화·라이브 우회·게이팅 매 요청), stream_proxy_test(fan-out·teardown·백프레셔·누수 방지).
- **단계 8.3 — 클라이언트 query+stream 핸들러 + redaction** 🔲: `internal/remote/client.go`(수정) 가 (a) `query` 수신 → query-action 을 노드 로컬 read 핸들러로 매핑(REQ-J04 매핑 표, A10) → 전송 전 redaction(`secret_fields.go`, REQ-J06) → `query_result` 반환, (b) `subscribe` 수신 → 노드 실시간 소스 구독 → 갱신마다 redaction 후 `stream_data` push → `unsubscribe`/연결 종료 시 소스 구독 해제(REQ-J08/J08b). allowlist 노드 측 2차 강제(심층 방어).
  - 검증: query_action_map_test(도메인별 매핑·결과 회신), query_redaction_test(시크릿 마스킹), stream_source_test(구독→push→teardown), allowlist 2차 강제.
- **단계 8.4 — 웹 통합(타깃 추상화 + 페이지 재사용 + 스트림 소비 + 노드 셀렉터·네비)(추후 expert-frontend)** 🔲: `web/src/hooks/useFlowsTarget.ts`·`useAgentsTarget.ts`·`useDevicesTarget.ts`(신규, `useEditorFlowTarget` 패턴 확장, REQ-J09/J12), `web/src/pages/{flows,agents,devices}/*ListPage.tsx`·`*DetailPanel.tsx`(수정 — `target` 수용·원격 동일 렌더링·query-action(상세/통계/노드 레벨) + 스트림 구독(실시간 상태/시리즈) 취득, REQ-J10/J11), `web/src/services/api/remoteService.ts`(수정 — query-action 호출·스트림 구독/해제·폴링 폴백, REQ-J08), `web/src/pages/remote/RemoteResourcesPage.tsx`(수정 — 노드 셀렉터 재용도화·`?target=remote:{id}` 라우팅·네비게이션, REQ-J13/J14).
  - 검증: useFlowsTarget/useAgentsTarget/useDevicesTarget Vitest, ListPage/DetailPanel 원격 타깃 통합 Vitest(FULL 패리티·스트림 소비), 노드 셀렉터·target 라우팅·폴링 폴백 Vitest.
- 의존: M3(명령 디스패치/상관/타임아웃 패턴), M4(노출 범위·미러·변경 시 캐시 무효화 훅), M6(감사·시크릿 redaction), M7(그룹 D/I 변경 경로 — 통합 액션 레이어가 라우팅). query/스트림 프록시는 M3 의 상관/타임아웃 인프라를 재사용하나 명령과 독립된 READ-ONLY 경로다.
- 매핑: REQ-J01~J16, REQ-E07/A06/D06/D07/F04/F06 일관.

## 3. 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| 미승인 노드의 무단 제어 | 보안 침해 | 승인 게이팅(C06/F03), 명령 권한 강제(D08/F04), 토큰 인증(F02) |
| 노드 토큰 유출/탈취 | 무단 관리 세션 | TLS(F01), 폐기 즉시성(F07, blacklist), 부트스트랩 신뢰(C08), 시크릿 비커밋(F06) |
| 관리 WS ↔ 모니터링 WS 혼용 | 채널 간섭·권한 혼선 | 엔드포인트 완전 분리(N02), 핸드셰이크 인증 분리(B02) |
| 명령 유실/타임아웃 | 원격 변경 미반영을 반영으로 오인 | 상관 id(D07) + 타임아웃 미적용 처리(D06) + 결과 수신 후에만 캐시 갱신(E08) |
| 서버 캐시 vs 노드 권위 정의 불일치 | 잘못된 목록/편집 | 노드가 권위(A4), 서버 편집은 명령 전파 후 갱신(E08), 충돌 정책 OPEN Q3 |
| 오프라인 노드 명령 시도 | 모호한 실패 | 라이브 RPC 만, 오프라인 명령 거절(B07), 큐잉은 향후 SPEC |
| instance_id 와 IoT device UUID 혼동 | 잘못된 연관 표시 | 용어 엄격 구분(§1.4), 별개 네임스페이스, source 태깅(E04) |
| 노출 설정 누락/오설정 | 의도치 않은 자원 노출 | 클라이언트 opt-in 기본 보수값(A04), 노출 변경 재미러링(A07), redaction(F06) |
| 다중 노드 연결 폭증 | 메모리/연결 한계 | 연결 자원 예측(N01), heartbeat 기반 정리, 부하 테스트 |
| disabled 모드 회귀 | 기존 사용자 영향 | disabled 기본(N03), 회귀 스위트로 보호 |
| 재연결 폭주(backoff 없음) | 서버 부하 | 지수 백오프(B04), MQTT/소켓 패턴 준용 |
| enrollment 토큰 무차별 추측(brute-force) | 무단 자동 승인 | ≥256비트 랜덤(H06), 해시 비교만(원본 비저장), max_uses/expires 제한 + 즉시 폐기(H04/H07). 추가: 전송 TLS(F01), register 경로 레이트리밋은 후속 과제 |
| enrollment 토큰 uses 증가 경합 | max_uses 초과 발급 | 조건부 원자 UPDATE(uses<max_uses), 패자 소진 폴백(H07), SQLite busy_timeout |
| 사전 등록 id 와 실제 접속 충돌 | 의도치 않은 자동 승인 | 사전 등록은 approved+토큰 미발급만, 접속 시 토큰 미보유 1회에 한해 자동 승인(H02), 중복 사전 등록 409(H01) |
| 서버 단독 미러 영속 편집(원칙 위반) | 노드 권위 정의와 불일치 | 모든 편집 명령 디스패치 경유(I01~I04), 결과 수신 후에만 캐시 갱신(E08), 실패 시 캐시 비갱신(I11) — 서버 직접 영속 코드 경로 금지 |
| 마스킹 시크릿이 실제 시크릿 덮어쓰기 | 노드 시크릿 손실/오설정 | 마스킹·미변경 필드 페이로드 생략 + 노드 측 기존 시크릿 병합(I07), 마스킹 자리표시자 영속 금지, secret_roundtrip_test |
| 오프라인/타임아웃/적용실패 의미 혼동 | 잘못된 성공 오인 | 503/504/502 구분 매핑(I11), 어느 실패에서도 캐시 비갱신(E08), 명확한 오류 사유 전달 |
| 범위 밖/미승인 자원 편집 | 권한·노출 우회 | 승인∧온라인∧노출 범위 게이팅(I05/D08/E07), UI 게이팅(I10), 위반 시 미디스패치 |
| 관리자 편집 ↔ 노드 로컬 변경 경합 | 분실 갱신(lost update) | 노드 권위(A4) 유지, 낙관적 잠금/버전 체크 도입은 OPEN Q7, last-write 경고 |
| 프록시로 임의 노드 read 도달 | 권한·노출 우회·정보 노출 | query-action allowlist(열거 집합) 강제(J04, 서버·노드 2중), 변경 의미 action 명시 배제(J03), 게이팅(승인∧온라인∧노출, J05) |
| 프록시 응답에 시크릿 평문 노출 | 자격증명 유출 | 노드가 전송 전 redaction(J06, 미러와 동일 `secret_fields.go`), 서버 무가공 전달(A10), 로그/감사 시크릿 비포함(J15/F06) |
| 프록시 경유 변경 시도 | READ-ONLY 원칙 위반 | 미열거/변경 의미 action 거부(J03), 변경은 그룹 D/I 로만(J12), 디바이스 쓰기=그룹 D 명령(OQ-J4 RESOLVED) — 신규 쓰기 경로 미신설 |
| 폴링/스트림 노드 왕복 부하 | 노드/서버 부하·지연 | 처음부터 단기 TTL 캐시(J16, OQ-J6 RESOLVED — pass-through 아님), 라이브 action 캐시 우회·변경 시 무효화, 스트리밍으로 폴링 빈도 저감 |
| 스트림 구독 누수/teardown 누락 | 노드/서버 메모리 폭증 | disconnect 시 전 구독 자동 teardown·unsubscribe 전파(J08b), subscription 레지스트리, stream_proxy_test(누수 검증) |
| 느린 소비자 backlog | 메모리 폭증·지연 | 백프레셔(J08b — 최신값 우선 coalesce/drop 또는 속도 제한), 큐 상한 |
| 프록시 실패 의미 혼동(offline/timeout/error) | 잘못된 성공 오인 | 503/504/502 구분 매핑(J07, 그룹 I I11 동일), 명확한 노드 오류 사유 전달 |
| 캐시 stale 데이터 노출 | 변경 후 옛 데이터 표시 | 단기 TTL + 관련 변경(그룹 D/I) 성공 시 캐시 무효화(J16), 라이브 action 캐시 우회 |
| 별도 원격 페이지 분기 재발 | 로컬/원격 UX 이원화·유지보수 부담 | 로컬 페이지/상세 패널 재사용(J10/J11), 타깃 추상화로 투명 전환(J09/J12), RemoteResourcesPage 노드 셀렉터 재용도화(J14) |
| 미러 요약 vs 프록시 라이브 데이터 혼동 | 잘못된 데이터 출처 | 미러=요약 정의·오프라인 last-known(E), 프록시=라이브 리치 데이터·온라인 전용(J), 역할 명확 분리(A11) |

## 4. 개발 방법론

- **Hybrid** (`.moai/config/sections/quality.yaml` development_mode=hybrid):
  - 신규 코드(`internal/remote/*`, `managed_node_*`, remote 핸들러, instance_id, `query_proxy.go`, `stream_proxy.go`) = **TDD**(RED-GREEN-REFACTOR, 신규 커버리지 85%+).
  - 기존 변경(config types/defaults/validate, 어댑터 명령 진입, main.go 배선) = **동작 보존 DDD**(ANALYZE-PRESERVE-IMPROVE, 회귀 0).
- 기존 ws/auth/adapter 인프라는 재사용(재발명 금지) → 기존 회귀 스위트로 보호.

## 5. 검증 전략 요약

- 백엔드 단위: instance_id, config(remote_management), 등록/승인 상태 머신, 명령 디스패처(상관/타임아웃), 어댑터 적용, ManagedNodeRepository(태깅/last-known), redaction/audit.
- 동시성: `go test -race ./...`(연결·디스패처·미러 수신 경합).
- 프론트 단위(추후): 관리 노드 목록/승인/자원 태깅/상태 피드백 Vitest.
- 원격 편집(그룹 I): flow/agent CRUD REST→command 전파, 게이팅(승인·온라인·노출), 실패 의미(503/504/502), 시크릿 redaction 라운드트립(생략·병합), 결과 후 캐시 갱신, 원격 편집 감사.
- 원격 제어 패리티(그룹 J): per-domain query-action allowlist(FULL 커버리지)·게이팅(승인·온라인·노출)·redaction·상관/타임아웃·실패 의미(503/504/502)·READ-ONLY 거부; 스트리밍 프록시(subscribe/stream_data/unsubscribe·teardown·백프레셔); 단기 TTL 캐시(무효화·라이브 우회); 통합 UI 타깃 추상화·로컬 페이지/상세 패널 원격 재사용·FULL 패리티·스트림 소비·노드 셀렉터·target 라우팅·폴링 폴백(Vitest, 추후).
- 통합/회귀: 등록→승인→명령→미러 end-to-end, 원격 편집(생성·수정·삭제→전파→적용→결과→캐시), 원격 상세 query-action 패리티(상세/통계/노드 레벨) + 실시간 스트림(상태/시리즈 라이브), 재연결, disabled 회귀, 기존 ws/auth/adapter 불변.
- 보안: TLS(wss), 토큰 인증/폐기, 승인 게이팅, 명령 권한, OWASP API Security 참조.
- 품질 게이트: TRUST 5, LSP zero-error(run), golangci-lint zero, gofmt/goimports 클린.
