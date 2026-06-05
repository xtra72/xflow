---
id: SPEC-REMOTE-001
title: "원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링"
version: "1.0.0"
status: planned
created: "2026-06-05"
updated: "2026-06-05"
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
---

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-06-05 | xtra | 초기 SPEC 작성 — 원격 관리 서버/클라이언트(fleet 관리). 전송=클라이언트가 서버로 dial 하는 영속 WebSocket, 제어=라이브 연결 위 RPC 명령(로컬 어댑터 적용), 인벤토리=클라이언트 미러링+서버 DB 캐시, 노출 제어=클라이언트 config opt-in. 신규 xflow 인스턴스 식별자(instance_id) 도입. M1~M5 마일스톤 정의 |

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

**제외(Non-goals):**
- 역터널/리버스 프록시, 노드 간 직접 P2P, 공유 데이터베이스(명시적 금지 — RPC over 라이브 연결만).
- 서버→노드 일괄 펌웨어/바이너리 업데이트(별도 SPEC-UPDATE 계열 영역, 본 SPEC 은 설정/자원 제어만).
- 멀티 서버 HA/클러스터링, 서버 페일오버.
- 노드 그룹/정책 기반 일괄 배포 오케스트레이션(향후 SPEC).
- 미러링된 자원의 서버 측 직접 영속 편집(서버는 명령 디스패치만; 영속 정의는 노드가 소유).
- 노드↔노드 자원 복제/마이그레이션.

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
| `register` | client→server | instance_id, hostname, version, exposure 요약, (선택)부트스트랩 시크릿 | 등록 요청(REQ-C01) |
| `register_ack` | server→client | status(pending/approved/rejected), (승인 시) node_token | 등록 응답·토큰 발급(REQ-C03/C04) |
| `command` | server→client | command_id, target instance_id, domain(flow/agent/device), action, args | 원격 명령(REQ-D01) |
| `command_result` | client→server | command_id, ok, result \| error | 명령 결과/ack(REQ-D05) |
| `inventory_snapshot` | client→server | instance_id, flows[], agents[], devices[] (노출 범위, redacted) | 접속 시 전체 인벤토리(REQ-E01) |
| `inventory_delta` | client→server | instance_id, op(add/update/remove), kind, item | 변경 델타(REQ-E02) |
| `heartbeat` | both | instance_id, ts | 생존성(REQ-B03) |
| `status` | client→server | instance_id, online/health 메타 | 상태 텔레메트리(REQ-B05 보조) |

- 상관: `command`/`command_result` 는 `command_id` 로 1:1 매칭(REQ-D07).
- 봉투의 `Timestamp` 는 RFC3339(기존 `NewMessage` 규약)이며, 페이로드 내부 디바이스 타임스탬프는 프로젝트 규약(epoch ms, int64)을 따른다.

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

- 핫리로드: 기존 `OnChange`/`WatchConfig` 로 mode/server_url/exposure 변경을 감지·반영(REQ-A06/A07).
- `bootstrap_secret`/발급 토큰은 시크릿이며 redaction·비커밋 대상(REQ-F06).

### 5.4 신규 저장 스키마 (서버 측)

`internal/storage` 에 `ManagedNodeRepository`(인터페이스 + sqlite 구현)와 미러 테이블을 추가한다. `FlowRepository` 패턴(Save/Get/List/Delete/Close)을 준용한다.

| 테이블 | 핵심 컬럼(요지) | 의미 |
|--------|------------------|------|
| `managed_nodes` | instance_id(PK), hostname, version, status(pending/approved/rejected), token_id, last_seen, online | 등록·상태·식별 |
| `mirrored_flows` | id, source_instance_id(FK), name, definition(redacted), updated_at | 노드별 플로우 미러(태그=source_instance_id) |
| `mirrored_agents` | id, source_instance_id(FK), name, kind, config(redacted), updated_at | 노드별 에이전트 미러 |
| `mirrored_devices` | id, source_instance_id(FK), node_assoc, name, meta, updated_at | 노드별 IoT 디바이스 미러 |

- 출처 태깅: 모든 미러 행은 `source_instance_id` 를 보유(REQ-E04/E05).
- last-known: 오프라인 시 행을 삭제하지 않고 `online=false`·`last_seen` 만 갱신(REQ-E06).
- redaction: 정의/설정 저장 시 시크릿 마스킹(REQ-F06).

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

### 5.9 OPEN QUESTIONS (구현 단계에서 결정)

1. **명령 큐잉 정책**: 본 SPEC 은 라이브 RPC 만(오프라인 노드 명령은 거절, REQ-B07). 오프라인 큐잉/지연 적용은 향후 SPEC 으로 분리.
2. **서버↔노드 겸용 인스턴스**: 한 인스턴스가 server 와 client 를 동시에(상위 서버에 등록되면서 하위 노드를 관리, 계층형 fleet) 겸하는 것은 본 SPEC 제외 — 향후 과제.
3. **인벤토리 델타 충돌 해소**: 서버 편집(E08)과 노드 로컬 변경(E02)이 경합할 때의 우선순위/머지 정책 정밀화는 구현 시 결정(기본: 노드가 권위 — A4).
4. **부트스트랩 신뢰 강도**: 부트스트랩 시크릿(C08) 외 mTLS/인증서 핀닝 도입 여부는 보안 검토 후 결정.
5. **exposure 표현**: all/none/목록/태그 기반 중 어떤 표현을 1차 채택할지 구현 시 확정(기본: all/none + 명시 목록).

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
| REQ-REMOTE-N01 ~ N04 | server 연결 관리, 엔드포인트 분리, disabled 회귀, Message 봉투 | 부하/회귀 + ws 호환 |
