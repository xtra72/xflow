# SPEC-REMOTE-001 인수 기준 (acceptance.md)

> 원격 관리 서버/클라이언트 — xflow 인스턴스 fleet 등록·승인·원격 제어·인벤토리 미러링
> Given/When/Then 시나리오 기반. 각 시나리오는 spec.md 의 EARS 요구사항에 매핑된다.

> **상태(2026-06-05)**: `planned`. 아래 인수 시나리오는 구현(`/moai run`) 시 충족 대상이다.
>
> 참고할 확정 동작:
> - 전송은 클라이언트가 서버로 dial 하는 영속 WS(관리 WS 는 모니터링 WS 와 별도 엔드포인트).
> - 제어는 라이브 연결 위 RPC 명령 → 클라이언트가 기존 어댑터(FlowServiceAdapter/AgentServiceAdapter/device 핸들러)로 적용.
> - 인벤토리는 클라이언트 push(스냅샷+델타) → 서버 DB 캐시, 모든 항목 source_instance_id 태깅, 오프라인 last-known 보존.
> - 노출 범위는 클라이언트 config 가 결정(opt-in). 노드 = managed node(xflow 설치본), device = IoT 디바이스(별개).

## 1. 인수 시나리오 (Given-When-Then)

### AC-1: 영속 instance_id 생성·재시작 불변 (REQ-A03)

- Given: instance_id 가 아직 없는 신규 xflow 설치본이다.
- When: 인스턴스를 기동하고 종료 후 재기동한다.
- Then: 최초 기동 시 instance_id(UUID)가 1회 생성·영속되고, 재기동 후에도 동일 값을 유지한다(노드 재식별 가능).

### AC-2: 모드 분기 — disabled 회귀 (REQ-A01, N03)

- Given: `remote_management.mode = disabled`(기본).
- When: 인스턴스를 기동한다.
- Then: 어떤 관리 연결도 생성/수락하지 않으며, 기존 xflow 동작이 회귀 없이 유지된다.

### AC-3: 클라이언트 dial + 인증 핸드셰이크 (REQ-A02, B01, B02)

- Given: `mode = client`, 유효한 `server_url`(wss)이 설정되어 있다.
- When: 클라이언트를 기동한다.
- Then: 클라이언트가 서버 관리 WS 엔드포인트로 영속 연결을 dial 하고(outbound), 핸드셰이크에서 자격(노드 토큰 또는 등록 요청/부트스트랩 시크릿)이 검증된다. server_url 이 비면 설정 오류를 기록하고 접속을 시도하지 않는다.

### AC-4: heartbeat + 자동 재연결(백오프) (REQ-B03, B04)

- Given: 클라이언트가 서버에 연결되어 있다.
- When: 연결이 끊긴다(네트워크 단절).
- Then: heartbeat 로 단절을 감지하고, 클라이언트가 지수 백오프로 재연결을 시도한다. 재연결 성공 시 재인증 후 인벤토리를 재동기화한다.

### AC-5: 등록 요청 → pending 보류 (REQ-C01, C02)

- Given: 미등록 노드가 서버에 처음 접속한다.
- When: 클라이언트가 instance_id + 노드 정보(hostname, version, 노출 자원 요약)로 `register` 를 보낸다.
- Then: 서버가 그 요청을 pending 상태로 보류하고, 즉시 관리 권한을 부여하지 않는다.

### AC-6: 관리자 승인 → 토큰 발급 (REQ-C03, C04)

- Given: pending 상태의 노드가 있다.
- When: 관리자가 해당 노드를 승인한다.
- Then: 서버가 노드 상태를 approved 로 갱신하고, 노드 토큰(JWT)을 발급해 `register_ack` 로 전달한다. 클라이언트는 토큰을 영속한다.

### AC-7: 거부된 노드 차단 (REQ-C03, C06, F03)

- Given: pending 상태의 노드가 있다.
- When: 관리자가 해당 노드를 거부한다.
- Then: 노드 상태가 rejected 가 되고, 서버는 그 노드에 명령을 디스패치하지 않으며 관리 대상으로 노출하지 않는다(등록 응답만 허용).

### AC-8: 재접속 재인증·세션 복원 (REQ-C05)

- Given: 승인되어 노드 토큰을 가진 노드가 있다.
- When: 노드가 재접속한다.
- Then: 노드 토큰으로 재인증되고, 재등록 없이 관리 세션이 복원된다.

### AC-9: 폐기(revocation) (REQ-C07, F07)

- Given: 승인·연결된 노드가 있다.
- When: 관리자가 그 노드를 폐기한다.
- Then: 서버가 노드 토큰을 무효화(blacklist)하고 관리 연결을 종료하며, 이후 그 토큰의 재인증을 거부한다.

### AC-10: 플로우 명령 원격 적용 (REQ-D01, D02, D05, D07)

- Given: 승인·온라인 노드가 있다.
- When: 서버 관리자가 그 노드에 플로우 Deploy(또는 Create/Update/Start/Stop/Pause) 명령을 디스패치한다.
- Then: 클라이언트가 `FlowServiceAdapter` 로 적용하고, 상관 id 가 일치하는 `command_result`(성공 결과)를 반환한다.

### AC-11: 에이전트 명령 원격 적용 (REQ-D03, D05)

- Given: 승인·온라인 노드가 있다.
- When: 서버가 에이전트 Start/Stop/Restart(또는 CRUD) 명령을 디스패치한다.
- Then: 클라이언트가 `AgentServiceAdapter` 로 적용하고 `command_result` 를 반환한다.

### AC-12: 디바이스 메타데이터 명령 적용 (REQ-D04, D05)

- Given: 승인·온라인 노드가 있다.
- When: 서버가 IoT 디바이스 메타데이터 명령을 디스패치한다.
- Then: 클라이언트가 device 핸들러 경로로 적용하고 `command_result` 를 반환한다.

### AC-13: 명령 타임아웃 (REQ-D06)

- Given: 명령이 디스패치되었으나 노드가 제한 시간 내 결과를 반환하지 않는다.
- When: 타임아웃이 경과한다.
- Then: 서버가 그 명령을 타임아웃 처리(미적용 간주)하고 호출자에게 명확한 오류를 반환하며, 서버 캐시를 갱신하지 않는다.

### AC-14: 적용 실패 보고 (REQ-D09)

- Given: 승인·온라인 노드가 있다.
- When: 로컬 어댑터 검증/충돌로 명령 적용이 실패한다.
- Then: 클라이언트가 부분 적용 없이 실패를 `command_result` 오류로 보고하고, 로컬 상태를 일관되게 유지한다.

### AC-15: 명령 권한 강제 (REQ-D08, F04)

- Given: 미승인(pending/rejected) 노드 또는 비-관리자 출처의 명령.
- When: 명령 수신/발행을 시도한다.
- Then: 승인 노드만 명령을 수신·적용하고, 서버 관리자만 명령을 발행할 수 있다. 미승인 출처 명령은 클라이언트가 거부한다.

### AC-16: 오프라인 노드 명령 거절 (REQ-B07)

- Given: 대상 노드가 offline 이다.
- When: 서버가 그 노드에 명령을 디스패치하려 한다.
- Then: 명령이 거절되거나 보류 사유와 함께 명확한 오류가 반환된다(라이브 RPC 만; 오프라인 큐잉 없음).

### AC-17: 접속 시 인벤토리 스냅샷 (REQ-E01, E07)

- Given: 클라이언트의 노출 설정에 일부 플로우/에이전트/디바이스가 포함된다.
- When: 클라이언트가 (재)접속한다.
- Then: 노출 범위의 인벤토리 스냅샷(redacted)이 서버로 push 되고, 노출되지 않은 자원은 포함되지 않는다.

### AC-18: 변경 시 인벤토리 델타 (REQ-E02)

- Given: 노드가 서버에 연결되어 인벤토리를 미러링한 상태다.
- When: 노드의 노출된 자원이 추가/수정/삭제된다.
- Then: 해당 변경에 대한 `inventory_delta` 가 서버로 push 되고 서버 캐시에 반영된다.

### AC-19: 서버 DB 캐시 + 출처 태깅 + 통합 목록 (REQ-E03, E04, E05)

- Given: 여러 노드가 각자 인벤토리를 미러링했다.
- When: 서버에서 관리 노드 목록과 자원 목록을 조회한다.
- Then: 각 노드와 그 플로우/에이전트/디바이스가 출처 노드(source_instance_id) 태그와 함께 표시되어, 어느 노드와 연관된 자원인지 식별된다.

### AC-20: 오프라인 last-known 표시 (REQ-B06, E06)

- Given: 인벤토리를 미러링한 노드가 오프라인이 된다.
- When: 서버에서 그 노드의 자원을 조회한다.
- Then: 마지막 미러 상태(last-known)가 offline 표식과 함께 표시되며, 미러 행은 삭제되지 않는다.

### AC-21: 노출 변경 재미러링 (REQ-A04, A07)

- Given: 노드가 일부 자원을 노출 중이다.
- When: 노출 설정을 변경(확대/축소)한다.
- Then: 갱신된 범위로 스냅샷/델타가 다시 미러링되고, 노출 해제된 자원은 서버 캐시에서 제거된다.

### AC-22: 서버 편집 → 명령 전파 후 캐시 갱신 (REQ-E08, A4)

- Given: 서버에 미러링된 자원이 있다.
- When: 서버 측에서 그 자원을 편집한다.
- Then: 편집이 (온라인) 노드로의 명령으로 전파되고, 명령 결과 수신 후에야 서버 캐시가 갱신된다(서버 단독 영속 없음).

### AC-23: 설정 핫리로드 (REQ-A06)

- Given: 인스턴스가 기동 중이다.
- When: `remote_management` 설정(mode/server_url/exposure)을 런타임에 변경한다.
- Then: 기존 OnChange/WatchConfig 로 변경이 감지되어 (재접속 포함) 안전하게 반영된다.

### AC-24: 관리 WS ↔ 모니터링 WS 분리 (REQ-N02, N04)

- Given: 서버 모드 인스턴스에 웹 UI 모니터링 WS 와 관리 WS 가 모두 존재한다.
- When: 두 채널을 동시에 사용한다.
- Then: 두 채널이 별도 엔드포인트로 동작하여 상호 간섭이 없고, 관리 메시지는 기존 `Message{Type,Payload,Timestamp}` 봉투로 표현된다.

### AC-25: 보안 — TLS·시크릿·감사 (REQ-F01, F05, F06)

- Given: TLS(wss)가 구성되고, 시크릿 필드를 가진 자원이 미러링된다.
- When: 노드가 연결되어 원격 명령으로 변경이 발생한다.
- Then: 전송이 TLS 로 보호되고, 시크릿 필드는 redaction 정책으로 마스킹/제외되며, 원격 변경(누가/언제/어느 노드/무엇)이 감사 로그에 기록된다.

### AC-26: 부트스트랩 신뢰(선택) (REQ-C08)

- Given: 부트스트랩 enrollment 시크릿이 구성되어 있다.
- When: 노드가 등록 요청 시 사전 공유 시크릿을 제시한다.
- Then: 1차 신뢰가 검증된다(미구성 시에는 순수 관리자 승인 흐름으로 동작).

### AC-27: 서버 웹 UI (REQ-G01~G04) — 추후 구현

- Given: 서버 모드 인스턴스에 관리 노드와 pending 요청이 있다.
- When: 관리자가 웹 UI 를 연다.
- Then: 관리 노드 목록(상태/online·offline), pending 승인 뷰(승인/거부), 노드별 자원 목록(디바이스 태그), 명령/상태 피드백이 표시된다.

### AC-28: 원격 플로우 생성 (REQ-I01, I06)

- Given: 승인·온라인 노드가 있고, 관리자가 새 플로우 정의를 준비했다.
- When: 관리자가 `POST /api/v1/remote/nodes/{instance_id}/flows`(정의 JSON)를 호출한다.
- Then: 서버가 `command{domain:flow, action:create}` 를 노드로 전파하고, 노드 `FlowServiceAdapter.Create` 결과를 받은 후에만 미러 캐시에 생성 행을 반영하며 201 로 생성 자원(또는 식별자)을 반환한다. 서버 단독 미러 영속은 일어나지 않는다.
- And(§5.9-8 RESOLVED, 노드 채번): 새 flow ID 는 노드 어댑터 `Create` 가 부여·반환하고 서버는 그 식별자를 미러에 기록한다(서버는 ID 를 생성하지 않는다).
- And(§5.9-9 RESOLVED, 수동 노출): 새로 생성된 플로우는 **자동 노출되지 않는다** — 운영자가 노드 노출 설정(REQ-A04/A07)을 갱신하기 전까지 서버 목록/미러에 노출되지 않은 채 유지된다(opt-in 보존).

### AC-29: 원격 플로우 수정 (REQ-I02, E08)

- Given: 승인·온라인 노드에 미러링된 플로우가 있다.
- When: 관리자가 `PATCH /api/v1/remote/nodes/{instance_id}/flows/{flow_id}`(갱신 정의)를 호출한다.
- Then: 서버가 `command{domain:flow, action:update}` 를 전파하고, `FlowServiceAdapter.Update` 결과 수신 후에만 미러 캐시를 갱신한다.
- And(§5.9-7 RESOLVED, 노드 권위 + 경고): 동시 노드 로컬 변경이 있어도 낙관적 잠금/409 없이 노드 권위 last-write-wins 로 적용되며, 동시 변경 감지 시 UI 에 비차단 경고만 표시되고 편집은 거부되지 않는다.

### AC-30: 원격 플로우 삭제 (REQ-I03)

- Given: 승인·온라인 노드에 미러링된 플로우가 있다.
- When: 관리자가 `DELETE /api/v1/remote/nodes/{instance_id}/flows/{flow_id}` 를 호출한다.
- Then: 서버가 `command{domain:flow, action:delete}` 를 전파하고, `FlowServiceAdapter.Delete` 결과 수신 후에만 미러 캐시에서 해당 행을 제거하며 204 를 반환한다.

### AC-31: 원격 에이전트 CRUD (REQ-I04)

- Given: 승인·온라인 노드가 있다.
- When: 관리자가 에이전트 생성(POST, 201)/수정(PATCH)/삭제(DELETE, 204) 엔드포인트를 호출한다.
- Then: 서버가 각각 `command{domain:agent, action:create|update|delete}` 를 전파하고, 노드 `AgentServiceAdapter` 대응 메서드 결과 수신 후에만 미러 캐시를 갱신한다.
- And(§5.9-8/9 RESOLVED): 새 agent ID 는 노드 어댑터 `Create` 가 채번하고, 새로 생성된 에이전트는 자동 노출되지 않으며(수동 노출, 운영자가 노출 설정 갱신 시까지 미노출), 수정 동시성은 노드 권위 + 비차단 경고를 따른다.

### AC-32: 편집 게이팅 — 승인·온라인·노출 범위 (REQ-I05, D08, E07)

- Given: (a) 미승인 노드, (b) 오프라인 노드, (c) 노출 범위 밖 자원 중 하나의 조건이다.
- When: 관리자가 원격 생성/수정/삭제를 시도한다.
- Then: 명령이 디스패치되지 않고, 조건에 맞는 명확한 오류가 반환된다. 승인·온라인이고 대상이 노출 범위 내일 때만 편집이 허용된다.

### AC-33: 시크릿 redaction 라운드트립 보존 (REQ-I07, F06)

- Given: 미러링된 정의에 마스킹된 시크릿 필드가 있고, 관리자가 시크릿 외 필드만 편집해 저장한다.
- When: 갱신 명령이 노드에 적용된다.
- Then(§5.9-6 RESOLVED, 필드 부재 + 노드 backfill): 마스킹/미변경 시크릿 필드는 갱신 페이로드에서 **완전히 생략(필드 부재)** 되며 sentinel/자리표시자 값이 와이어로 전송되지 않는다. 노드는 어댑터 호출 **전에** 기존 자원 정의를 로드해 부재 시크릿을 기존값으로 backfill 한 뒤 적용한다. 마스킹 자리표시자 값이 실제 시크릿을 덮어쓰지 않는다.

### AC-34: 시각 편집기 재사용 — 원격 플로우 (REQ-I08)

- Given: 서버 웹 UI 에 원격(미러링된) 플로우가 있다.
- When: 관리자가 그 플로우를 기존 시각 편집기(React Flow 캔버스)로 연다.
- Then: 로컬 플로우와 동일한 편집기에서 편집되며, 저장 시 대상 `instance_id` 노드로(신규=POST/기존=PATCH) 전파된다. 편집기는 저장 대상(로컬 vs 원격 노드)을 명확히 구분한다.

### AC-35: 에이전트 편집 surface + 원격 페이지 생성/삭제·피드백 (REQ-I09, I10)

- Given: 노드별 원격 자원 페이지에 플로우/에이전트 목록이 있다.
- When: 관리자가 에이전트를 편집/구성하거나 자원을 생성/삭제한다.
- Then: 에이전트 편집·설정 surface 가 제공되고, 생성/삭제 액션이 동작하며, 성공/실패 피드백이 표시된다. UI 는 노드의 승인/온라인/노출 상태에 따라 액션을 게이팅한다.

### AC-36: 원격 편집 실패 의미 — 503/504/502 (REQ-I11, D06, D09, E08)

- Given: 원격 편집 요청이 진행된다.
- When: (a) 대상 노드 오프라인, (b) 명령 타임아웃, (c) 노드 어댑터 적용 실패 중 하나가 발생한다.
- Then: 각각 (a) 503, (b) 504, (c) 502(노드 오류 사유 전달)가 반환되고, 어느 경우에도 서버 미러 캐시는 갱신되지 않는다.

### AC-37: 원격 편집 감사 (REQ-I12, F05, F06)

- Given: 관리자가 원격 자원을 생성/수정/삭제한다.
- When: 편집 명령이 처리된다.
- Then: 누가/언제/어느 노드/어느 자원/어떤 action 이 감사 로그에 기록되며, 시크릿 값(마스킹/실제 모두)은 감사에 포함되지 않는다.

### AC-38: per-domain query-action 프록시 — 라이브 상세 취득 (REQ-J01, J02, J11)

- Given: 승인·온라인 노드가 있고, 관리자가 그 노드의 에이전트 상세를 보려 한다.
- When: 서버가 read 프록시로 `query{domain:agent, query_action:get, args}`(+`query_action:stats`)를 노드에 전달한다(raw HTTP path 아님 — 그룹 D command 와 대칭되는 per-domain query-action).
- Then: 노드가 각 query-action 을 자신의 로컬 read 핸들러로 매핑·실행한 라이브 JSON(상세+통계)을 `query_result{query_id, status, body(redacted)}` 로 반환하고, UI 는 **로컬과 동일한** 상세 패널 데이터를 표시한다. 상관 id(query_id)로 요청-응답이 1:1 매칭된다.

### AC-39: 프록시 READ-ONLY 강제 (REQ-J03, J12)

- Given: read 프록시(query + 스트림)가 동작 중이다.
- When: 변경 의미 query-action(create/update/delete/execute/metadata-write/start/stop/config) 또는 미열거 action 으로 프록시를 시도한다.
- Then: 프록시가 거부되고, 모든 변경은 그룹 D 명령 / 그룹 I(M7) CRUD 경로로만 수행된다(디바이스 명령·메타데이터도 그룹 D 경로).

### AC-40: query-action allowlist 강제 — FULL 커버리지 (REQ-J04)

- Given: 도메인별 지원 query-action allowlist(flow: get/nodes/status/logs·list; agent: get/stats/config/devices/topics/store/sessions/series·list; device: get/state/commands/metadata·list)가 구성되어 있다.
- When: 관리자가 (a) 열거된 query-action 과 (b) 미열거 임의 action 으로 각각 질의한다.
- Then: (a) 는 정상 프록시되어 모든 로컬 상세 패널을 원격에서 동작시키기에 충분한 데이터를 반환하고(FULL 커버리지), (b) 는 즉시 거부된다(임의 action 차단).

### AC-41: 프록시 게이팅 — 승인·온라인·노출 범위 (REQ-J05, E07, A06)

- Given: (a) 미승인 노드, (b) 오프라인 노드, (c) 노출 범위 밖 자원 중 하나의 조건이다.
- When: 관리자가 READ/QUERY 프록시 질의를 시도한다.
- Then: 질의가 프록시되지 않고 조건에 맞는 명확한 오류가 반환된다. 승인·온라인이고 대상이 노출 범위 내일 때만 질의가 허용된다.

### AC-42: 프록시 응답 redaction (REQ-J06, F06)

- Given: 시크릿 필드를 가진 자원(에이전트 자격증명 등)이 노출 범위에 있다.
- When: 관리자가 그 자원의 상세를 프록시 질의한다.
- Then: 노드가 **전송 전에** 시크릿 필드를 redaction 정책(`secret_fields.go`)으로 마스킹/제외하여, 시크릿이 와이어로 평문 전송되지 않는다(미러와 동일 정책).

### AC-43: 프록시 실패 의미 — 503/504/502 (REQ-J07)

- Given: 프록시 질의가 진행된다.
- When: (a) 대상 노드 오프라인, (b) 질의 타임아웃, (c) 노드 측 질의 오류 중 하나가 발생한다.
- Then: 각각 (a) 503, (b) 504, (c) 502(노드 오류 사유 전달)가 반환된다(그룹 I I11 과 동일 매핑).

### AC-44: 스트리밍 프록시 — 서버 경유 라이브 push 중계 (REQ-J08)

- Given: 원격 노드의 디바이스 실시간 상태/에이전트 라이브 통계·시리즈를 보는 상세 패널이 열려 있다.
- When: 패널이 해당 실시간 소스를 구독한다(`subscribe{subscription_id, domain, stream_action, args}`).
- Then: 노드가 소스 갱신마다 `stream_data{subscription_id, payload(redacted)}` 를 push 하고, 서버가 이를 해당 브라우저 세션으로 fan-out 하여 UI 가 라이브로 갱신된다(로컬 실시간 동형). 스트림은 READ-ONLY 이며, 스트림 미지원/실패 시 query-action 폴링으로 폴백된다.

### AC-45: 자원 타깃 추상화 — 로컬/원격 투명 전환 (REQ-J09, J12)

- Given: `useFlowsTarget`/`useAgentsTarget`/`useDevicesTarget` 추상화가 제공된다.
- When: 동일 페이지가 `target=local` 과 `target=remote:{instanceId}` 로 각각 동작한다.
- Then: 데이터 소스(로컬 API vs 프록시)와 액션 엔드포인트(로컬 vs 그룹 D 명령 / 그룹 I CRUD)가 투명하게 전환되며, 페이지/패널 본문은 동일 코드로 동작한다.

### AC-46: 로컬 페이지·상세 패널 재사용 + FULL 패리티 (REQ-J10, J11)

- Given: 서버 웹 UI 에서 관리자가 원격 노드를 선택했다.
- When: `FlowListPage`/`AgentListPage`/`DeviceListPage` 와 상세 패널을 연다.
- Then: 별도 원격 전용 화면 없이 **로컬과 동일한** 목록·상세 패널이 렌더링되며, 에이전트 통계/설정/디바이스/토픽/store/세션/시리즈·디바이스 실시간 상태/명령 스펙/메타데이터·플로우 상태/노드 레벨 런타임·로그가 로컬과 동일하게 표시된다.

### AC-47: 노드 셀렉터 + target 쿼리 라우팅 + 네비게이션 (REQ-J13, J14)

- Given: 관리자가 로컬/원격 자원을 제어하려 한다.
- When: 노드 셀렉터에서 원격 노드를 선택한다.
- Then: `?target=remote:{instanceId}` 쿼리 파라미터로 동일 목록/제어 페이지가 라우팅되고(미지정/`local` 은 로컬), 사이드바/네비게이션이 현재 타깃을 표시한다. 기존 `RemoteResourcesPage` 는 노드 셀렉터로 재용도화/폐기되어 로컬/원격 UX 가 분기되지 않는다.

### AC-48: 통합 액션 레이어 — 변경은 그룹 D/I 라우팅 (REQ-J12, J03)

- Given: 원격 타깃 페이지에서 라이프사이클/CRUD/디바이스 명령을 수행한다.
- When: 관리자가 플로우 deploy/start/stop, 에이전트 start/stop/restart, 디바이스 명령·메타데이터, 플로우/에이전트 생성·수정·삭제를 실행한다.
- Then: 라이프사이클·디바이스 쓰기는 그룹 D 명령으로, 생성·수정·삭제는 그룹 I(M7) CRUD 로 라우팅된다. 어떤 변경도 READ/QUERY 프록시(query/스트림)를 경유하지 않는다.

### AC-49: 단기 TTL 프록시 캐시 + 무효화 (REQ-J16)

- Given: 서버가 query-action 응답을 단기 TTL 로 캐시하도록 구성되어 있다.
- When: (a) 동일 `{instance_id, domain, query_action, args}` 질의가 TTL 내 반복되고, (b) 그 자원에 대한 변경(그룹 D 명령 / 그룹 I CRUD)이 성공하며, (c) 라이브 action(state/stats/series)을 질의한다.
- Then: (a) 캐시 적중으로 노드 왕복 없이 응답되고(반복 폴 부하 감소, 게이팅은 매 요청 평가), (b) 관련 캐시 항목이 무효화되어 stale 데이터가 제거되며, (c) 라이브 action 은 캐시를 우회해 항상 노드/스트림에서 직격된다.

### AC-50: 스트리밍 라이프사이클 — teardown·백프레셔·누수 방지 (REQ-J08b)

- Given: 브라우저 세션이 원격 노드의 실시간 소스를 구독 중이다.
- When: (a) 노드가 오프라인이 되거나 연결이 종료되고, (b) 브라우저가 세션 종료 또는 `unsubscribe` 하며, (c) 소비자가 느려 갱신이 쌓인다.
- Then: (a) 해당 노드의 모든 구독이 자동 teardown 되고, (b) unsubscribe 가 노드로 전파되어 노드 측 소스 구독이 해제되며, (c) 백프레셔(최신값 우선 coalesce/drop 또는 속도 제한)로 메모리 폭증이 방지된다. 구독 누수가 없다.

## 2. 품질 게이트 (Definition of Done)

- [ ] 모든 EARS 요구사항(REQ-REMOTE-A01~J16, N01~N04)에 대응 인수 시나리오 통과.
- [ ] 백엔드: 신규 코드(`internal/remote/*`, `managed_node_*`, remote 핸들러, instance_id) TDD, 커버리지 85%+.
- [ ] 백엔드: 기존 변경(config types/defaults/validate, 어댑터 명령 진입, main.go 배선) 동작 보존 — 기존 회귀 스위트 100% 통과.
- [ ] ws/auth/adapter 인프라 재사용 — 기존 ws/auth/adapter 테스트 전부 통과(회귀 0).
- [ ] `go test -race ./...` 통과, golangci-lint zero, gofmt/goimports 클린.
- [ ] 등록/승인: pending→approved→재인증·복원, pending/rejected 차단, revoke 토큰 무효화 검증.
- [ ] 원격 명령: flow/agent/device 적용, 상관 id 매칭, 타임아웃·실패·권한 케이스 검증.
- [ ] 인벤토리: 스냅샷/델타·출처 태깅·last-known·노출 준수·서버 편집→명령 전파 검증.
- [ ] 보안: TLS(wss), 토큰 인증/폐기, 승인 게이팅, redaction, 감사 로그 검증.
- [ ] disabled 회귀(기존 동작 불변), 관리 WS↔모니터링 WS 분리 검증.
- [ ] 프론트(추후): 관리 노드 목록/승인/자원 태깅/상태 피드백 Vitest 통과.
- [ ] 원격 편집(그룹 I): flow/agent FULL CRUD REST → command 전파 → 결과 후에만 미러 캐시 갱신 검증.
- [ ] 원격 편집: 게이팅(승인·온라인·노출 범위), 실패 의미(503/504/502), 시크릿 redaction 라운드트립(마스킹 필드 생략·노드 병합), 원격 편집 감사 검증.
- [ ] 원격 편집(프론트, 추후): 시각 편집기 원격 재사용·에이전트 설정 surface·생성/삭제·피드백·게이팅 Vitest 통과.
- [ ] 원격 제어 패리티(그룹 J, 백엔드): per-domain query-action allowlist(FULL 커버리지)·게이팅(승인·온라인·노출)·redaction·상관/타임아웃·실패 의미(503/504/502)·READ-ONLY 거부, 스트리밍 프록시(subscribe/stream_data/unsubscribe·teardown·백프레셔), 단기 TTL 캐시(무효화·라이브 우회) 검증.
- [ ] 원격 제어 패리티(그룹 J, 프론트, 추후): 타깃 추상화(useFlowsTarget/useAgentsTarget/useDevicesTarget)·로컬 페이지/상세 패널 원격 재사용·FULL 패리티(상세/통계/상태/시리즈/노드 레벨)·스트림 구독 소비·노드 셀렉터·target 라우팅·폴링 폴백 Vitest 통과.
- [ ] LSP 품질 게이트(run): error/type-error/lint-error 0.

## 3. 검증 방법·도구

| 영역 | 도구 | 대상 |
|------|------|------|
| Go 단위 | `go test` + testify | instance_id, config(remote_management), 등록/승인 상태 머신, 명령 디스패처(상관/타임아웃), 어댑터 적용, ManagedNodeRepository(태깅/last-known) |
| Go 동시성 | `go test -race ./...` | 연결·디스패처·미러 수신 경합 |
| Go 정적 | golangci-lint, gofmt, goimports | 전 변경 |
| 보안 | TLS(wss) 검증, jwt 인증/blacklist, redaction(secret_fields), 감사 로그 | F 그룹 |
| 프론트 단위(추후) | Vitest + Testing Library | 관리 노드 목록/승인/자원 태깅/상태 피드백, 타깃 추상화·로컬 페이지 원격 재사용·노드 셀렉터·스트림 구독 소비·폴링 폴백(그룹 J) |
| 프록시(그룹 J) | `go test` + testify | per-domain query-action allowlist(FULL 커버리지)·게이팅(승인·온라인·노출)·redaction·상관/타임아웃·실패 의미(503/504/502)·READ-ONLY 거부, 스트리밍(fan-out·teardown·백프레셔), 단기 TTL 캐시(무효화·라이브 우회) |
| 통합/회귀 | 기존 ws/auth/adapter 회귀 + end-to-end | 등록→승인→명령→미러, 원격 상세 query-action 패리티(상세/통계/노드 레벨)·실시간 스트림(상태/시리즈), 재연결, disabled 회귀 |
| 품질 | TRUST 5, LSP 게이트 | run 단계 zero-error |

## 4. 추적성 매핑

| 인수 시나리오 | EARS 요구사항 |
|---------------|---------------|
| AC-1 | A03 |
| AC-2 | A01, N03 |
| AC-3 | A02, B01, B02 |
| AC-4 | B03, B04 |
| AC-5 | C01, C02 |
| AC-6 | C03, C04 |
| AC-7 | C03, C06, F03 |
| AC-8 | C05 |
| AC-9 | C07, F07 |
| AC-10 | D01, D02, D05, D07 |
| AC-11 | D03, D05 |
| AC-12 | D04, D05 |
| AC-13 | D06 |
| AC-14 | D09 |
| AC-15 | D08, F04 |
| AC-16 | B07 |
| AC-17 | E01, E07 |
| AC-18 | E02 |
| AC-19 | E03, E04, E05 |
| AC-20 | B06, E06 |
| AC-21 | A04, A07 |
| AC-22 | E08 |
| AC-23 | A06 |
| AC-24 | N02, N04 |
| AC-25 | F01, F05, F06 |
| AC-26 | C08 |
| AC-27 | G01, G02, G03, G04 |
| AC-28 | I01, I06 |
| AC-29 | I02, E08 |
| AC-30 | I03 |
| AC-31 | I04 |
| AC-32 | I05, D08, E07 |
| AC-33 | I07, F06 |
| AC-34 | I08 |
| AC-35 | I09, I10 |
| AC-36 | I11, D06, D09, E08 |
| AC-37 | I12, F05, F06 |
| AC-38 | J01, J02, J11 |
| AC-39 | J03, J12 |
| AC-40 | J04 |
| AC-41 | J05, E07, A06 |
| AC-42 | J06, F06 |
| AC-43 | J07 |
| AC-44 | J08 |
| AC-45 | J09, J12 |
| AC-46 | J10, J11 |
| AC-47 | J13, J14 |
| AC-48 | J12, J03 |
| AC-49 | J16 |
| AC-50 | J08b |
| (전반) | A05, B05, F02, N01, J15 |
