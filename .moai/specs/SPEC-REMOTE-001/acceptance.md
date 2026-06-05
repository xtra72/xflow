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

## 2. 품질 게이트 (Definition of Done)

- [ ] 모든 EARS 요구사항(REQ-REMOTE-A01~G04, N01~N04)에 대응 인수 시나리오 통과.
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
- [ ] LSP 품질 게이트(run): error/type-error/lint-error 0.

## 3. 검증 방법·도구

| 영역 | 도구 | 대상 |
|------|------|------|
| Go 단위 | `go test` + testify | instance_id, config(remote_management), 등록/승인 상태 머신, 명령 디스패처(상관/타임아웃), 어댑터 적용, ManagedNodeRepository(태깅/last-known) |
| Go 동시성 | `go test -race ./...` | 연결·디스패처·미러 수신 경합 |
| Go 정적 | golangci-lint, gofmt, goimports | 전 변경 |
| 보안 | TLS(wss) 검증, jwt 인증/blacklist, redaction(secret_fields), 감사 로그 | F 그룹 |
| 프론트 단위(추후) | Vitest + Testing Library | 관리 노드 목록/승인/자원 태깅/상태 피드백 |
| 통합/회귀 | 기존 ws/auth/adapter 회귀 + end-to-end | 등록→승인→명령→미러, 재연결, disabled 회귀 |
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
| (전반) | A05, B05, F02, N01 |
