# HVACR 미러 동기화 — 브로커 보안 · ACL 운영 가이드

> **관련 SPEC**: [SPEC-HVACR-SYNC-001](../../.moai/specs/SPEC-HVACR-SYNC-001/spec.md) (M9 미러 브로커 보안)
> **최종 업데이트**: 2026-08-06

이 가이드는 HVACR 에이전트 미러링(게이트웨이 ↔ 서버 동기화)이 사용하는 MQTT 브로커의
**보안 설정**과 **토픽 ACL** 구성 방법을 설명합니다. 미러 동기화는 MQTT 브로커를 통해
디코드 메시지(업링크)와 제어(다운링크)를 주고받으므로, **브로커가 실질적인 보안 경계**가
됩니다.

---

## 1. 역할 분담 — 무엇을 누가 책임지는가

| 주체 | 책임 |
|------|------|
| **브로커** (Mosquitto/EMQX 등) | 인증·ACL **강제(enforce)** — 어느 클라이언트가 어느 토픽을 pub/sub 할지 |
| **xflow** | ACL을 **가능하게(enable)** — ① 게이트웨이/서버별 **인증 identity**(username/password/TLS), ② `{prefix}/{gateway_id}/*` **토픽 네임스페이스** |

xflow는 브로커가 ACL을 걸 수 있는 재료(고유 identity + 게이트웨이별 네임스페이스 토픽)를
제공할 뿐, **접근 통제 자체는 브로커에서 설정**해야 합니다. 이 문서의 ACL 예시는 브로커에
그대로 넣는 운영 설정입니다.

---

## 2. 토픽 스킴

기본 prefix 는 `xflow/hvacr` (에이전트 설정 `mirror_topic_prefix` 로 변경 가능, 게이트웨이·서버
동일해야 함). `{gateway_id}` 는 게이트웨이 식별자(`mirror_gateway_id`).

| 토픽 | 방향 | 용도 |
|------|------|------|
| `{prefix}/{gateway_id}/up/nasa` | 게이트웨이 → 서버 | 디코드 NASA 메시지(업링크) |
| `{prefix}/{gateway_id}/up/ack` | 게이트웨이 → 서버 | 제어 ACK(선택) |
| `{prefix}/{gateway_id}/up/snapshot/{addr}` | 게이트웨이 → 서버 | retain 상태 스냅샷(재동기화) |
| `{prefix}/{gateway_id}/down/control` | 서버 → 게이트웨이 | 제어 명령(다운링크) |

**비대칭 원칙**: 게이트웨이는 자기 `up/*` 를 **발행**하고 자기 `down/control` 을 **구독**하며,
서버는 `up/*` 를 **구독**하고 `down/control` 을 **발행**합니다.

---

## 3. 브로커 보안 체크리스트 (배포 전 필수)

1. **익명 접근 차단 + 인증 필수화** — 게이트웨이·서버마다 고유 username/password
   (xflow 에이전트 설정 `mirror_username` / `mirror_password`)
2. **TLS 암호화** — `ssl://` 브로커 + `mirror_tls` 활성, 사설 CA 사용 시 `mirror_ca_cert`
   (PEM 또는 경로)로 서버 검증
3. **per-topic ACL** — 아래 §4 템플릿으로 각 클라이언트를 자기 토픽으로 제한
4. **네트워크 격리** — 브로커를 공개 인터넷에 노출하지 않고 폐쇄망/VPN 내부에만 배치
5. **제어 불필요 시 제어면 제거** — 모니터링 전용이면 게이트웨이 `mirror_control_enabled=false`
   (+ 에이전트 `control_enabled=false`)로 제어 주입면 자체를 없앰

### xflow 에이전트 설정 ↔ 브로커 인증 매핑

| xflow 필드(연결 방식=mirror 또는 미러 업링크 활성 시) | 브로커 |
|------|------|
| `mirror_username` / `mirror_password` | MQTT 인증 자격증명(ACL의 `user` 키) |
| `mirror_tls` + `mirror_ca_cert` | TLS 연결 + 서버 인증서 검증 |
| `mirror_gateway_id` | 토픽 네임스페이스 → ACL 범위 단위 |

---

## 4. ACL 템플릿

### 4.1 Mosquitto (`mosquitto.acl`)

Mosquitto ACL에서 `topic write` = 발행(publish), `topic read` = 구독(subscribe).

```
# ── 게이트웨이: 자기 up/* 발행 + 자기 down/control 구독만 ──
user gw-station-01
topic write xflow/hvacr/gw-station-01/up/#
topic read  xflow/hvacr/gw-station-01/down/control

# 게이트웨이가 여럿이면 각 게이트웨이마다 위 블록을 gateway_id 만 바꿔 반복
user gw-station-02
topic write xflow/hvacr/gw-station-02/up/#
topic read  xflow/hvacr/gw-station-02/down/control

# ── 서버(중앙): 모든 게이트웨이 up/* 구독 + down/control 발행 ──
user hvacr-server
topic read  xflow/hvacr/+/up/#
topic write xflow/hvacr/+/down/control
```

`mosquitto.conf` 에서 익명 금지 + ACL/비밀번호 파일 연결:

```
allow_anonymous false
password_file /etc/mosquitto/passwd
acl_file       /etc/mosquitto/mosquitto.acl
```

### 4.2 EMQX (파일 기반 ACL, `acl.conf`)

```
%% 게이트웨이: 자기 up/* 발행 + 자기 down/control 구독만
{allow, {user, "gw-station-01"}, publish,   ["xflow/hvacr/gw-station-01/up/#"]}.
{allow, {user, "gw-station-01"}, subscribe, ["xflow/hvacr/gw-station-01/down/control"]}.

%% 서버(중앙): 모든 게이트웨이 up/* 구독 + down/control 발행
{allow, {user, "hvacr-server"}, subscribe, ["xflow/hvacr/+/up/#"]}.
{allow, {user, "hvacr-server"}, publish,   ["xflow/hvacr/+/down/control"]}.

%% 그 외 전부 거부(기본 거부)
{deny, all}.
```

---

## 5. 멀티테넌트 / 현장 분리 (서버 범위 제한)

여러 운영자·현장이 서로의 게이트웨이를 제어하면 안 되는 경우, 서버 username 을
게이트웨이별로 범위 제한합니다(와일드카드 `+` 대신 특정 `gateway_id`).

```
# 현장 A 담당 서버만 gw-station-01 제어
user server-siteA
topic read  xflow/hvacr/gw-station-01/up/#
topic write xflow/hvacr/gw-station-01/down/control
```

서버가 N개 게이트웨이를 미러하면 미러 에이전트를 게이트웨이별로 두므로, 각 에이전트에
범위 제한된 `mirror_username` 을 부여하면 됩니다.

---

## 6. 각 ACL이 막는 위협

| ACL 규칙 | 차단하는 위협 |
|----------|---------------|
| 게이트웨이를 자기 `up/#` 만 발행 허용 | 탈취된 게이트웨이가 **다른 게이트웨이 텔레메트리 위조** 불가 |
| 게이트웨이의 `down/control` 을 **구독만** 허용(발행 금지) | 게이트웨이가 **가짜 서버 노릇**(다른 현장 제어) 불가 |
| 서버의 `up/*` **쓰기 금지**(구독만) | 탈취된 서버가 **가짜 텔레메트리 주입** 불가 |
| 서버 username 범위를 특정 `gateway_id` 로 제한 | 한 서버/운영자가 **담당 밖 게이트웨이 제어** 불가 |

> **주의**: `down/control` 발행 권한은 실장비를 명령하는 시스템 최고 권한입니다. ACL은
> *어느 게이트웨이를* 제어할지 범위만 제한할 수 있고 능력 자체는 줄일 수 없으므로,
> **서버 자격증명을 가장 강하게 보호**해야 합니다.

---

## 7. 전체 흐름 요약

```
게이트웨이(gw-station-01)  ── write {p}/gw-station-01/up/#        ─┐
                          ── read  {p}/gw-station-01/down/control ←┘  브로커 인증 + ACL
서버(hvacr-server)        ── read  {p}/+/up/#                     ←┘   (강제)
                          ── write {p}/+/down/control
```

xflow가 제공한 identity(§3)와 토픽 네임스페이스(§2) 위에 브로커 인증·ACL(§4)을 얹으면,
"게이트웨이는 발행만, 서버는 명령만" 이 강제되어 접근 통제가 완성됩니다.
