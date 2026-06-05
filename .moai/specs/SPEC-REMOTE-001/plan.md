# SPEC-REMOTE-001 구현 계획 (plan.md)

> 원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링
> 본 문서는 PLAN 단계 산출물이다. 코드는 포함하지 않으며 기술 접근/마일스톤/위험을 정의한다.

> **상태(2026-06-05)**: `planned`. 4개 확정 아키텍처 결정(영속 WS dial / 라이브 RPC over 연결 / 인벤토리 미러+서버 DB 캐시 / 클라이언트 노출 opt-in)을 고정 제약으로 둔다. 마일스톤 M1~M5 로 증분 구현한다.

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

## 4. 개발 방법론

- **Hybrid** (`.moai/config/sections/quality.yaml` development_mode=hybrid):
  - 신규 코드(`internal/remote/*`, `managed_node_*`, remote 핸들러, instance_id) = **TDD**(RED-GREEN-REFACTOR, 신규 커버리지 85%+).
  - 기존 변경(config types/defaults/validate, 어댑터 명령 진입, main.go 배선) = **동작 보존 DDD**(ANALYZE-PRESERVE-IMPROVE, 회귀 0).
- 기존 ws/auth/adapter 인프라는 재사용(재발명 금지) → 기존 회귀 스위트로 보호.

## 5. 검증 전략 요약

- 백엔드 단위: instance_id, config(remote_management), 등록/승인 상태 머신, 명령 디스패처(상관/타임아웃), 어댑터 적용, ManagedNodeRepository(태깅/last-known), redaction/audit.
- 동시성: `go test -race ./...`(연결·디스패처·미러 수신 경합).
- 프론트 단위(추후): 관리 노드 목록/승인/자원 태깅/상태 피드백 Vitest.
- 통합/회귀: 등록→승인→명령→미러 end-to-end, 재연결, disabled 회귀, 기존 ws/auth/adapter 불변.
- 보안: TLS(wss), 토큰 인증/폐기, 승인 게이팅, 명령 권한, OWASP API Security 참조.
- 품질 게이트: TRUST 5, LSP zero-error(run), golangci-lint zero, gofmt/goimports 클린.
