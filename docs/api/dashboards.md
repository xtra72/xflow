# 대시보드 API (`/api/dashboards/*`)

> **관련 SPEC**: [SPEC-DASHBOARD-001](../../.moai/specs/SPEC-DASHBOARD-001/spec.md) v0.2.0
> **최종 업데이트**: 2026-05-12

xflow 대시보드 페이지/그리드/레이아웃 구성을 SQLite 영속 저장소에 저장하고, 어떤 브라우저/기기/시크릿창에서 접속해도 (공유) + (본인 개인) 두 snapshot 이 일관되게 보이도록 한다.

## 1. 개요

두 가지 사용 컨텍스트를 병행 지원한다:

- **공유 대시보드 (`scope=global`, `owner=NULL`)**: 모든 인증된 사용자가 동일하게 보는 운영용 대시보드. **admin 만 편집 가능**, editor/viewer 는 읽기 전용.
- **개인 대시보드 (`scope=user`, `owner=<username>`)**: 각 사용자가 자유롭게 구성하는 개인용 대시보드. 본인만 GET/PUT/DELETE 가능.

핵심 설계 원칙:

1. **URL 이 source-of-truth**: `/shared` 는 공유, `/mine` 은 개인. 클라이언트는 `(scope, owner)` 를 직접 지정할 수 없다.
2. **JWT 가 owner 결정자**: `/mine` 의 owner 는 항상 JWT `Claims.Username` 으로 결정된다. owner spoofing 차단.
3. **서버가 메타데이터 부여**: `version`, `updatedAt` 은 서버가 부여하며 클라이언트 페이로드의 해당 값은 무시된다.
4. **last-write-wins + If-Match**: 충돌은 `If-Match: <version>` 헤더와 `409 Conflict` 로 해결한다.

## 2. 엔드포인트 요약

| Method | Path | 인증 | 권한 |
|--------|------|------|------|
| GET    | `/api/dashboards/shared` | 필수 (JWT) | 인증된 사용자 (admin/editor/viewer) |
| PUT    | `/api/dashboards/shared` | 필수 (JWT) | **admin only** |
| DELETE | `/api/dashboards/shared` | 필수 (JWT) | **admin only** |
| GET    | `/api/dashboards/mine`   | 필수 (JWT) | 인증된 사용자 (본인) |
| PUT    | `/api/dashboards/mine`   | 필수 (JWT) | 인증된 사용자 (본인) |
| DELETE | `/api/dashboards/mine`   | 필수 (JWT) | 인증된 사용자 (본인) |

> **인증 필수**: 모든 엔드포인트는 `Authorization: Bearer <jwt>` 헤더를 요구한다. `basic_auth` 가 비활성화된 상태로는 부팅을 거부한다. 자세한 내용은 [README 인증 섹션](../../README.md#인증-authentication) 참조.

> 이 표 밖에 상한 조회 엔드포인트 `GET /api/v1/dashboard-limits`(권한 `dashboard.read`)가 있다 — §4 [페이로드 크기 한도](#페이로드-크기-한도) 참조.

## 3. 응답 코드

| 코드 | 의미 |
|------|------|
| `200 OK` | 성공 (GET / PUT) |
| `204 No Content` | DELETE 성공 |
| `400 Bad Request` | payload schema 오류, URL vs body `scope` 불일치 |
| `401 Unauthorized` | JWT 없음 / 만료 |
| `403 Forbidden` | 권한 부족 (예: editor 가 `PUT /shared` 시도) |
| `404 Not Found` | GET 시 snapshot 미존재 (초기 상태) |
| `409 Conflict` | PUT 시 `If-Match` version 불일치 (body 에 서버측 최신 snapshot 포함) |
| `413 Payload Too Large` | payload 크기가 서버 예산 초과 (§4 [페이로드 크기 한도](#페이로드-크기-한도)) |
| `500 Internal Server Error` | 저장소 I/O 실패 |

## 4. 데이터 모델

### `DashboardSnapshot` (응답 본문)

```json
{
  "scope": "global | user",
  "owner": "string | null",
  "version": 1,
  "updatedAt": 1776339916504,
  "payload": {
    "dashboardPages": [],
    "activeDashboardId": "string",
    "dashboardGridCols": 12,
    "dashboardShowGridLines": true,
    "dashboardRefreshInterval": 5000,
    "deviceGridLayout": {}
  }
}
```

| 필드 | 타입 | 설명 |
|------|------|------|
| `scope` | `"global"` \| `"user"` | URL 로부터 결정 (shared=global, mine=user). 서버가 부여. |
| `owner` | `string \| null` | scope=global 시 `null`, scope=user 시 JWT 의 username. 서버가 부여. |
| `version` | `int64` | 단조 증가 정수. 서버가 부여 (`old.version + 1`). |
| `updatedAt` | `int64` | epoch ms. 서버가 부여 (`time.Now().UnixMilli()`). |
| `payload` | `object` | 대시보드 구성 본체. 클라이언트 source-of-truth. |

### `DashboardPutRequest.payload` (요청 본문)

PUT 요청 body 는 다음 구조의 JSON 이다:

```json
{
  "payload": {
    "dashboardPages": [
      {
        "id": "string",
        "name": "string",
        "panels": []
      }
    ],
    "activeDashboardId": "string",
    "dashboardGridCols": 12,
    "dashboardShowGridLines": true,
    "dashboardRefreshInterval": 5000,
    "deviceGridLayout": {
      "<pageId>": [
        { "i": "panel-1", "x": 0, "y": 0, "w": 4, "h": 3 }
      ]
    }
  }
}
```

> **클라이언트는 `scope` / `owner` / `version` / `updatedAt` 을 body 에 포함하지 않는다.** 포함하더라도 서버가 무시한다 (owner spoofing 차단, UB-003). 단, body 의 `scope` 가 URL 의 scope 와 불일치하면 silent normalize 없이 `400 Bad Request` 로 거부된다 (UB-006).

### 페이로드 크기 한도

> **고정 256 KB 가 아니다** (SPEC-CANVAS-007 §결정 14). 예산은 서버 설정 `dashboard.max_canvas_elements` 에서 **유도된다** — 운영자가 캔버스에 몇 개짜리 도면을 들일지 정하고, 서버가 그 수에 필요한 자리를 확보한다.

- 예산 = `max_canvas_elements × 1 KiB`, `[256 KB, 8 MiB]` 로 죈다. 기본값 1024 개에서 **1 MiB**.
  - 바닥 256 KB: 요소 수를 줄인 설정이 이미 저장되던 대시보드를 깨뜨리지 못하게 한다.
  - 천장 8 MiB: 페이로드 상한은 DoS 방어물이므로 설정 오타가 그 성질을 없애지 못하게 한다. 이 값이 곧 요청 하나가 메모리에 올릴 수 있는 바이트의 상한이다.
- 초과 시 `413 Payload Too Large`. 응답 메시지는 **그 요청에 실제로 적용된 상한**을 말한다 (자산 업로드는 자기 상한 12 MiB 를 말한다).
- 평균 가정: 페이지당 패널 20개 이하, 사용자당 페이지 10개 이하 (ASM-004).
- 적용 중인 값은 아래 조회 엔드포인트로 읽는다.

#### `GET /api/v1/dashboard-limits` — 적용 중인 상한 조회

| 항목 | 값 |
|------|-----|
| 인증 | 필수 (JWT) |
| 권한 | `dashboard.read` |
| 부수 효과 | 없음 (읽기 전용) |

`200 OK` 응답 `data`:

```json
{
  "max_canvas_elements": 1024,
  "payload_budget_bytes": 1048576
}
```

- `max_canvas_elements` — 캔버스 패널의 SVG 가져오기 한 번이 만들 수 있는 요소 수의 상한. 편집기가 이 값을 가져오기 거절 상한으로 쓴다.
- `payload_budget_bytes` — 위 수에서 유도된 대시보드 PUT 본문 상한. 두 수는 **같은 출처에서 나온 한 쌍**이라 서로 어긋날 수 없다.

> **admin 게이트를 두지 않는다.** 이 값은 비밀이 아니라 편집기가 동작하려면 알아야 하는 수다. admin 으로 죄면 admin 이 아닌 편집자만 컴파일 기본값으로 떨어져, 운영자가 상한을 올려도 그들에게만 가져오기가 거절된다.
>
> **클라이언트는 이 창구가 없어도 동작해야 한다.** 오프라인 · 구형 서버(`404`) · 권한 없음(`403`) · 응답 형상 변경은 전부 클라이언트 컴파일 기본값으로 떨어지고 가져오기는 계속된다. 상한이 낡는 것보다 가져오기를 아예 못 하게 되는 것이 나쁘다.

### `payload.schemaVersion` (선택)

- 기본값 `1`.
- 알 수 없는 값이면 `400 Bad Request`.

## 5. 엔드포인트별 상세

### 5.1 `GET /api/dashboards/shared` — 공유 snapshot 조회

모든 인증된 사용자가 호출할 수 있다 (admin/editor/viewer).

**요청 헤더**:

```text
Authorization: Bearer <jwt>
```

**응답 (200 OK)**:

```json
{
  "scope": "global",
  "owner": null,
  "version": 3,
  "updatedAt": 1776339916504,
  "payload": { ... }
}
```

**응답 (404 Not Found)**: 공유 snapshot 이 아직 없는 초기 상태. 클라이언트는 빌트인 `DEFAULT_DASHBOARD_PAGE` 로 메모리 상태를 초기화한다.

**curl 예시**:

```bash
curl -X GET https://xflow.example.com/api/dashboards/shared \
  -H "Authorization: Bearer ${JWT}"
```

---

### 5.2 `PUT /api/dashboards/shared` — 공유 snapshot 저장 (admin only)

**권한**: `admin` 역할만 허용. editor/viewer 가 호출하면 `403 Forbidden`.

**요청 헤더**:

```text
Authorization: Bearer <jwt>
Content-Type: application/json
If-Match: 3
```

`If-Match` 헤더는 권장 사항이다. 없으면 unconditional update 로 처리되지만 동시 편집 시 충돌 손실 위험이 있다.

**요청 본문**:

```json
{
  "payload": { ... }
}
```

**응답 (200 OK)**:

```json
{
  "scope": "global",
  "owner": null,
  "version": 4,
  "updatedAt": 1776340000000,
  "payload": { ... }
}
```

**응답 (409 Conflict)**: `If-Match` 헤더의 version 이 서버의 현재 version 과 다를 때. body 에 서버측 최신 snapshot 이 포함된다.

```json
{
  "scope": "global",
  "owner": null,
  "version": 5,
  "updatedAt": 1776340500000,
  "payload": { ... }
}
```

클라이언트는 응답의 server snapshot 을 store 에 적용한 뒤, 로컬 변경을 `If-Match: 5` 로 1회만 재PUT 한다 (last-write-wins, 무한루프 방지).

**curl 예시**:

```bash
curl -X PUT https://xflow.example.com/api/dashboards/shared \
  -H "Authorization: Bearer ${JWT}" \
  -H "Content-Type: application/json" \
  -H "If-Match: 3" \
  -d '{
    "payload": {
      "dashboardPages": [{"id": "home", "name": "Home", "panels": []}],
      "activeDashboardId": "home",
      "dashboardGridCols": 12,
      "dashboardShowGridLines": true,
      "dashboardRefreshInterval": 5000,
      "deviceGridLayout": {}
    }
  }'
```

---

### 5.3 `DELETE /api/dashboards/shared` — 공유 snapshot 삭제 (admin only)

**권한**: `admin` 역할만 허용. editor/viewer 가 호출하면 `403 Forbidden`.

**요청 헤더**:

```text
Authorization: Bearer <jwt>
```

**응답 (204 No Content)**: 성공. 이후 `GET /api/dashboards/shared` 는 `404 Not Found` 를 반환한다.

**curl 예시**:

```bash
curl -X DELETE https://xflow.example.com/api/dashboards/shared \
  -H "Authorization: Bearer ${JWT}"
```

---

### 5.4 `GET /api/dashboards/mine` — 본인 개인 snapshot 조회

JWT `Claims.Username` 으로 owner 가 결정된다. 다른 사용자의 개인 대시보드는 절대 조회할 수 없다 (UB-005).

**요청 헤더**:

```text
Authorization: Bearer <jwt>
```

**응답 (200 OK)**:

```json
{
  "scope": "user",
  "owner": "alice",
  "version": 7,
  "updatedAt": 1776339916504,
  "payload": { ... }
}
```

**응답 (404 Not Found)**: 해당 사용자의 snapshot 이 아직 없는 초기 상태. 클라이언트는 빌트인 `DEFAULT_DASHBOARD_PAGE` 로 메모리 상태를 초기화하고, 첫 변경 시 자동으로 PUT 하여 snapshot 을 최초 생성한다.

**curl 예시**:

```bash
curl -X GET https://xflow.example.com/api/dashboards/mine \
  -H "Authorization: Bearer ${JWT}"
```

---

### 5.5 `PUT /api/dashboards/mine` — 본인 개인 snapshot 저장

모든 인증된 사용자가 자신의 개인 snapshot 을 저장할 수 있다. owner 는 JWT 의 username 으로 자동 결정되며, body 의 `owner` 는 무시된다 (owner spoofing 차단, UB-003).

**요청 헤더**:

```text
Authorization: Bearer <jwt>
Content-Type: application/json
If-Match: 6
```

**요청 본문**:

```json
{
  "payload": { ... }
}
```

**응답 (200 OK)**:

```json
{
  "scope": "user",
  "owner": "alice",
  "version": 7,
  "updatedAt": 1776340000000,
  "payload": { ... }
}
```

**응답 (409 Conflict)**: 공유 PUT 과 동일한 동작. 응답 body 에 서버측 최신 snapshot 이 포함되며 클라이언트는 1회 재PUT 으로 last-write-wins.

**curl 예시**:

```bash
curl -X PUT https://xflow.example.com/api/dashboards/mine \
  -H "Authorization: Bearer ${JWT}" \
  -H "Content-Type: application/json" \
  -H "If-Match: 6" \
  -d '{
    "payload": {
      "dashboardPages": [{"id": "my-home", "name": "내 홈", "panels": []}],
      "activeDashboardId": "my-home",
      "dashboardGridCols": 6,
      "dashboardShowGridLines": false,
      "dashboardRefreshInterval": 10000,
      "deviceGridLayout": {}
    }
  }'
```

---

### 5.6 `DELETE /api/dashboards/mine` — 본인 개인 snapshot 삭제

**요청 헤더**:

```text
Authorization: Bearer <jwt>
```

**응답 (204 No Content)**: 성공. 이후 `GET /api/dashboards/mine` 는 `404 Not Found` 를 반환한다. 다른 사용자의 row 와 공유 row 는 영향받지 않는다.

**curl 예시**:

```bash
curl -X DELETE https://xflow.example.com/api/dashboards/mine \
  -H "Authorization: Bearer ${JWT}"
```

## 6. `If-Match` 헤더 사용 패턴

- **권장**: PUT 요청에 마지막으로 알고 있는 `version` 을 `If-Match` 헤더로 보낸다.
- **첫 PUT** (snapshot 최초 생성): `If-Match` 헤더 생략 (unconditional). 서버는 새 row 를 생성하고 `version=1` 을 부여한다.
- **충돌 처리**: 응답이 `409 Conflict` 이면 body 의 server snapshot 을 store 에 적용한 뒤, 로컬 변경을 `If-Match: <server-version>` 으로 **1회만** 재PUT 한다.
- **무한루프 방지**: 두 번째 `409` 발생 시 클라이언트는 토스트로 사용자에게 알리고 server snapshot 으로 강제 동기화한다.
- **클라이언트 직렬화**: 같은 클라이언트가 in-flight PUT 중이면 추가 변경을 큐에 쌓아 직전 PUT 응답 수신 후에만 다음 PUT 을 발행한다 (동시 PUT 금지).

## 7. 권한 매트릭스 요약

| Role | GET /shared | PUT /shared | DELETE /shared | GET /mine | PUT /mine | DELETE /mine |
|------|-------------|-------------|----------------|-----------|-----------|--------------|
| admin | ✅ | ✅ | ✅ | ✅ (본인) | ✅ (본인) | ✅ (본인) |
| editor | ✅ | ❌ 403 | ❌ 403 | ✅ (본인) | ✅ (본인) | ✅ (본인) |
| viewer | ✅ | ❌ 403 | ❌ 403 | ✅ (본인) | ✅ (본인) | ✅ (본인) |
| 미인증 | ❌ 401 | ❌ 401 | ❌ 401 | ❌ 401 | ❌ 401 | ❌ 401 |

> **개인 대시보드 cross-user 격리** (UB-005): `/mine` 의 owner 는 항상 JWT `Claims.Username` 으로 결정된다. 사용자 A 가 사용자 B 의 개인 대시보드를 조회/수정/삭제하는 경로는 존재하지 않는다.

## 8. 관련 문서

- [SPEC-DASHBOARD-001 spec.md](../../.moai/specs/SPEC-DASHBOARD-001/spec.md) — 본 API 의 정식 SPEC
- [SPEC-DASHBOARD-001 acceptance.md](../../.moai/specs/SPEC-DASHBOARD-001/acceptance.md) — Acceptance Criteria (17개 시나리오)
- [README 인증 섹션](../../README.md#인증-authentication) — `basic_auth` 필수화 안내 및 `XFLOW_ALLOW_NO_AUTH` 설정
- [CHANGELOG v0.2.0 항목](../../CHANGELOG.md) — 변경 사항 전체 정리
