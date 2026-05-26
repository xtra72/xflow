# 디바이스 ID 체계 마이그레이션 가이드

본 문서는 xflow 의 디바이스 식별 체계가 composite key (`"agent:local_id"`) 에서 Kubernetes 패턴 (`uid` + `name` + `reference`) 으로 단계적으로 진화하는 과정을 운영자·외부 클라이언트 작성자 관점에서 정리한다.

> 본 가이드는 SPEC-DEVICE-IDENTITY-001 의 운영 동반 문서이다. 기술 명세는 [.moai/specs/SPEC-DEVICE-IDENTITY-001/spec.md](../../.moai/specs/SPEC-DEVICE-IDENTITY-001/spec.md) 를 참조하라.

---

## 1. 한눈에 보기

xflow 는 디바이스 식별을 두 가지 별도 개념으로 분리한다 (Kubernetes 의 `metadata.uid` / `metadata.name` 패턴 차용):

| 개념 | 필드 | 변경 가능성 | 사용처 |
|---|---|---|---|
| **Identity** | `uid` (UUID v4) | 불변 | 시계열 tag, REST URL, MQTT topic, 영속 메타데이터 키 |
| **Name** | `name` | 변경 가능 | 사람이 읽는 라벨, UI 표시, yaml 의 가독성 친화 참조 |
| **Reference** | `agent/name` (파생) | 구성요소 변경 시 갱신 | yaml 의 `pinned` 참조, REST URL 의 사람 친화 경로 |
| **Composite (legacy)** | `id` (`agent:local_id`) | 변경 가능 | v0.x 호환 alias (Deprecated, Phase D 에서 제거 예정) |

핵심 메시지:

- **호환 기간 동안 외부 클라이언트는 무수정으로 동작한다** (Soft Deprecation).
- 신규 코드는 `uid` 또는 `agent/name` 사용을 권장한다.
- Phase D (xflowd v1.0 메이저 버전) 에서 composite (`agent:local_id`) 가 제거된다. 충분한 호환 기간 (Phase B/C 완료 후 최소 6개월) 이 확보된다.

---

## 2. 4 Phase 진화 전략

| Phase | 목표 | 호환성 | 본 가이드 작성 시점 상태 |
|---|---|---|---|
| **Phase A** | UUID 1급 격상 (`Device.UID()` + emit/REST 의 `uid` 필드) | 완전 호환 | ✅ 완료 (v0.x) |
| **Phase B** | 내부 사용처 UUID 전환 (registry / callback / log / yaml / inventory) | Soft Deprecation | ✅ B1/B2 완료 |
| **Phase C** | 영속 데이터 + 시계열 DB 마이그레이션 (CLI 도구) | 운영 윈도우 권장 | 🟢 C1 (device-ids) + C2 (tsdb-tags) 완료 / C3 (Dual-tag) 다음 세션 |
| **Phase D** | composite 완전 제거 (Breaking, xflowd v1.0) | Breaking | ⏳ Phase B/C 완료 후 최소 6개월 호환 기간 |

각 Phase 의 상세 요구사항은 SPEC-DEVICE-IDENTITY-001 의 EARS 모듈 M1~M10 참조.

---

## 3. Phase A/B1 적용 상태 (현재)

본 가이드 작성 시점 (2026-05-26) 기준으로 다음이 적용되어 있다.

### 3.1 Phase A 완료 항목

- `Device.UID() string` 인터페이스 메서드.
- 5개 어댑터 (NASA / LGCNP / LGCP / Century / Modbus) 의 `UID()` 구현.
- REST `GET /api/v1/devices` 응답에 `uid` 필드 노출 (omitempty).
- `DeviceIDRepository` 미설정 시 1회 경고 로그.
- `xflowd_device_uid_missing_total` Prometheus counter.

### 3.2 Phase B1 (본 세션) 완료 항목

- **B-T1**: `device.DeviceRegistry` 에 `GetByUID(uid)` / `GetByAgentName(agent, name)` / `ResolveDevice(ref)` 메서드 추가. composite `Get(id)` 는 시맨틱 유지 (호환).
- **B-T2**: 5개 HVAC 에이전트의 callback 시그니처 V2 진입점 추가. v1 시그니처는 Deprecated 마킹하되 동작 유지 (wrapper 패턴).
- **B-T6**: `internal/logger/device_format.go` 헬퍼 — `FormatDevice(d)`, `DeviceUIDAttr(d)`, `DeviceAttrs(d)`.
- **B-T7**: `internal/config/yaml_resolver.go` — `ParseDeviceRef(ref, file, line)` 가 UUID / agent/name / composite 세 형식을 모두 지원.
- **B-T8**: inventory 노드 payload 의 `device_uuid` → `uid` 정규화 (`device_uuid` alias 유지). SPEC-INVENTORY-001 v0.3.0.
- **B-T9**: `xflowd_device_composite_use_total{source}` Prometheus counter — composite alias 사용 빈도 추적.

### 3.3 Phase B2 (다음 세션) 잔여 항목

- **B-T3**: WebSocket event payload 의 `uid` 1급 필드.
- **B-T4**: REST URL resolver — UUID / agent/name / composite alias dispatch.
- **B-T5**: `GET /api/v1/devices:resolve?agent=X&name=Y` 엔드포인트.

---

## 4. 클라이언트 마이그레이션 가이드

### 4.1 REST 호출자

#### 4.1.1 Phase A/B1 단계 (현재)

기존 v0.x 패턴이 그대로 작동한다:

```http
GET /api/v1/devices/lgcnp:81       # composite alias — 호환 유지
GET /api/v1/devices                  # 전체 목록
```

응답에 `uid` 필드가 1급으로 포함된다 (Phase A 부터):

```json
{
  "id": "lgcnp:81",
  "uid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "name": "indoor-1",
  "agent_name": "lgcnp",
  "online": true
}
```

#### 4.1.2 Phase B2 진행 시 추가 (다음 세션)

다음 URL 패턴이 추가될 예정이다 (B-T3/T4/T5):

```http
GET /api/v1/devices/{uuid}                            # UUID resolver (권장)
GET /api/v1/devices/lgcnp/indoor-1                    # agent/name resolver (권장)
GET /api/v1/devices:resolve?agent=lgcnp&name=indoor-1 # 명시적 name 기반 resolver
GET /api/v1/devices/lgcnp:81                          # composite alias (Deprecation 헤더 포함)
```

composite alias 사용 시 `Deprecation: true` 와 `Sunset: <date>` 헤더가 응답에 포함된다 (B-T4 완료 후).

#### 4.1.3 마이그레이션 권장 사항

- **권장 1순위**: UUID 사용. 에이전트 rename / 디바이스 재배치에 안정적이다.
- **권장 2순위**: `agent/name` 사용. 사람이 읽기 좋고 yaml 친화적.
- **비권장**: composite (`agent:local_id`) — Phase D 에서 제거된다.

### 4.2 yaml 작성자

#### 4.2.1 Phase B 부터 (현재)

세 가지 형식을 모두 허용한다 (`internal/config.ParseDeviceRef`):

```yaml
flow:
  pinned:
    - "lgcnp/indoor-1"                          # 권장 — 사람 친화
    - "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"   # 권장 — 자동 생성 yaml
    - "lgcnp:81"                                 # Deprecated — 경고 메트릭 증가
```

#### 4.2.2 메트릭 관찰

composite 형식 사용 시 다음 Prometheus counter 가 증가한다:

```
xflowd_device_composite_use_total{source="yaml"} += 1  # 매 파싱 시
```

운영자는 `/metrics` 엔드포인트에서 이 값이 0 인지 확인하여 마이그레이션 완료 여부를 판단할 수 있다.

#### 4.2.3 매칭 실패 처리

알 수 없는 형식은 부팅 실패가 아니라 `ErrInvalidDeviceReference` 를 반환한다 (호출자 결정):

```yaml
flow:
  pinned:
    - "invalid-format-xyz"   # 부팅 실패 또는 경고 — 호출자 정책에 따름
```

### 4.3 로그 파싱

#### 4.3.1 Phase A/B1 단계 (현재)

신규 구조화 필드가 추가된다:

```
INFO  device offline device=lgcnp/indoor-1 device_uid=a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d device_agent=lgcnp device_name=indoor-1
```

기존 `device_id=lgcnp:81` 형식 라인도 점진적으로 위 형식으로 마이그레이션된다.

#### 4.3.2 권장 파싱 키

- **권장 1순위**: `device_uid` — UUID, 시계열 추적용 안정 키.
- **권장 2순위**: `device` — 사람 친화 표시 (`agent/name` 또는 fallback).
- **비권장**: `device_id` — composite, Phase D 에서 사라질 수 있음.

#### 4.3.3 메트릭 관찰

로그 사이트가 composite fallback 으로 표시한 횟수:

```
xflowd_device_composite_use_total{source="log"} += 1  # FormatDevice 의 composite fallback path
```

이 값이 높으면 `Device.Name()` 이 비어 있어 fallback 이 자주 발생하는 것 — 디바이스 메타데이터 보완을 검토.

### 4.4 inventory 노드 소비자

#### 4.4.1 Phase B1 부터 (현재)

`devices` source 의 항목 payload 에 `uid` 가 1급 키로 추가된다. `device_uuid` 는 alias 로 유지 (v0.2.0 호환):

```json
{
  "id": "lgcnp:81",
  "uid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "device_uuid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "name": "indoor-1",
  "agent_name": "lgcnp",
  ...
}
```

두 키 (`uid`, `device_uuid`) 는 항상 동일한 값이다. 둘 다 포함되거나 둘 다 생략된다 (graceful degradation 유지).

#### 4.4.2 마이그레이션 권장 사항

- **신규 소비자**: `uid` 사용.
- **기존 v0.2.0 소비자**: 무수정으로 동작 — `device_uuid` 알리어스가 계속 존재.
- `device_uuid` 는 v0.4.0 또는 xflowd v1.0 에서 제거 예정.

### 4.5 시계열 DB / MQTT 통합

#### 4.5.1 권장 tag/topic 키

- **권장**: `uid` (UUID) — 에이전트 rename 안전.
- **비권장**: `id` (composite) — Phase D 에서 제거 가능.

#### 4.5.2 Phase C 진입 시 (예정)

호환 기간 동안 두 tag 가 병기 기록된다:

```
measurement,id=lgcnp:81,uid=a58ba668-5741-... field=value timestamp
```

Phase D 진입 후 `xflowd migrate tsdb-drop-composite` 도구로 `id` tag 를 정리할 수 있다.

### 4.6 에이전트 callback 등록자 (외부 플러그인)

#### 4.6.1 v0.x 패턴 (Deprecated)

```go
agent.SetDeviceStateChangeCallback(func(agentName, deviceID string) {
    // deviceID 는 composite key ("lgcnp:81")
    log.Println("device state changed:", deviceID)
})
```

#### 4.6.2 Phase B1 부터 권장 패턴

```go
agent.SetDeviceStateChangeCallbackV2(func(agentName, deviceUID, deviceCompositeID string) {
    // deviceUID 는 UUID v4 (1급). 빈 문자열이면 graceful — UUID 없음.
    // deviceCompositeID 는 v0.x 호환 alias.
    if deviceUID != "" {
        log.Println("device state changed:", agentName, "uid=", deviceUID)
    } else {
        log.Println("device state changed (no UID):", deviceCompositeID)
    }
})
```

#### 4.6.3 v1 / V2 동시 등록 안전

V2 와 v1 콜백은 동시에 등록 가능하다. 에이전트는 V2 와 v1 콜백을 모두 호출하므로 외부 플러그인의 부분 마이그레이션이 안전하다.

---

## 5. Phase C1 마이그레이션 도구 운영 가이드

### 5.0 `xflowd migrate device-ids` 도구

SPEC-DEVICE-IDENTITY-001 Phase C § C1 으로 도입된 마이그레이션 CLI 도구이다. `device_metadata.json` 파일의 map key 를 composite (`agent:unit_id`) 에서 UUID 로 변환한다.

#### 5.0.1 매핑 권위 (Source of Truth)

`device_ids.json` 파일이 composite → UUID 매핑의 권위이다. 이 파일은 운영 중 HVAC 에이전트가 자동으로 채워 넣는다 (`agent.ResolveDeviceID`). 마이그레이션 도구는 이 매핑을 읽기만 한다 (수정 안 함).

#### 5.0.2 CLI 시그니처

```bash
xflowd migrate device-ids \
    --metadata-dir <path> \
    --id-repo <path> \
    [--backup-dir <path>] \
    [--dry-run] \
    [--strict] \
    [--yes]
```

| 플래그 | 필수 | 의미 |
|---|---|---|
| `--metadata-dir` | 예 | `device_metadata.json` 위치 (기본 `~/.xflow/storage/device_metadata`) |
| `--id-repo` | 예 | `device_ids.json` 위치 (기본 `~/.xflow/storage/device_ids`) |
| `--backup-dir` | 아니오 | 백업 저장 위치 (기본: `<metadata-dir>/.backup-<UTC-timestamp>`) |
| `--dry-run` | 아니오 | 실제 변경 없이 계획만 출력 |
| `--strict` | 아니오 | ambiguous mapping 발견 시 abort (기본은 skip + warn) |
| `--yes` | 아니오 | 대화형 확인 건너뛰기 (CI/배치 용도) |

#### 5.0.3 매핑 카테고리

도구는 각 metadata key 를 4 카테고리로 분류한다:

| 카테고리 | 의미 | 동작 |
|---|---|---|
| **Convert** | composite → UUID 매핑이 존재하고 충돌 없음 | UUID key 로 변환 |
| **AlreadyUUID** | 이미 UUID 명명 (idempotent) | 건드리지 않음 |
| **Orphan** | composite key 인데 `device_ids.json` 에 매핑 없음 | skip + warn (`--strict` 시 abort) |
| **Ambiguous** | 다대일 매핑 또는 UUID 키 collision | skip + warn (`--strict` 시 abort) |

#### 5.0.4 권장 운영 절차

1. **staging 리허설** — 프로덕션 직전 staging 환경에서 동일한 metadata + id-repo 복사본으로 dry-run:
   ```bash
   xflowd migrate device-ids \
       --metadata-dir /tmp/staging-metadata \
       --id-repo /tmp/staging-ids \
       --dry-run
   ```
   `Plan summary: convert=N ambiguous=0 orphan=0 ...` 라인 확인. ambiguous / orphan 이 0 이 아니면 운영자 검토.

2. **외부 백업** — 도구는 자동 백업하지만, 운영자도 별도 스냅샷 권장:
   ```bash
   cp -a /var/lib/xflow/device_metadata /var/lib/xflow/device_metadata.preflight-$(date +%Y%m%d)
   ```

3. **dry-run 후 실제 실행** — 매핑 100% 깨끗하면 실제 실행 (대화형 확인):
   ```bash
   xflowd migrate device-ids \
       --metadata-dir /var/lib/xflow/device_metadata \
       --id-repo /var/lib/xflow/device_ids
   # → 마이그레이션을 실행하시겠습니까? [y/N]: y
   ```

4. **검증** — `Result: converted=N skipped=0 verified=true backup=<path>` 출력 확인. `verified=false` 면 수동 복원 필요 (절차 § 5.0.6).

5. **재실행 (idempotency 확인)** — 동일 명령을 재실행하여 `Plan summary: convert=0 ...` 가 나오는지 확인. 0 이면 마이그레이션 완료.

#### 5.0.5 ambiguous mapping 처리

`device_ids.json` 에 동일 UUID 가 여러 composite 와 매핑되어 있으면 (다대일) 도구는 안전을 위해 skip 한다. 운영자가 다음을 결정해야 한다:

- 의도된 다대일 매핑인지 (한 디바이스가 두 이름으로 등록됨).
- 의도되지 않은 데이터 손상인지.

운영자는 `device_ids.json` 을 수동으로 검토·수정한 후 마이그레이션을 재실행한다.

#### 5.0.6 복구 절차 (검증 실패 또는 운영자 결정)

도구가 자동 생성한 백업 디렉토리 (`<backup-dir>/device_metadata.json`) 가 원본을 byte-perfect 보존한다. `<backup-dir>/manifest.json` 은 sha256 검증용.

수동 복원:

```bash
# 백업 디렉토리 확인 (예: /var/lib/xflow/device_metadata/.backup-20260526T103000Z)
ls -la /var/lib/xflow/device_metadata/.backup-*

# 원본 복원 (xflowd 데몬 중지 후 권장)
systemctl stop xflowd
cp /var/lib/xflow/device_metadata/.backup-20260526T103000Z/device_metadata.json \
   /var/lib/xflow/device_metadata/device_metadata.json
systemctl start xflowd
```

sha256 검증 (선택, 백업 무결성 확인):

```bash
cd /var/lib/xflow/device_metadata/.backup-20260526T103000Z
jq -r '.source_sha256' manifest.json
sha256sum device_metadata.json
# 두 값이 일치해야 한다
```

#### 5.0.7 위험 신호와 대응

| 증상 | 의미 | 대응 |
|---|---|---|
| `Result: verified=false` | 변환 후 sha256 합산 불일치 (이론적으로 발생하지 않아야 함) | 즉시 백업으로 복원, 도구 issue 보고 |
| `Plan summary: orphan>0` | composite 가 `device_ids.json` 에 등록되지 않음 | 디바이스가 데몬에 한 번도 등록되지 않았거나, `device_ids.json` 손상. 운영자 검토 |
| `Plan summary: ambiguous>0` | 다대일 매핑 또는 UUID 키 collision | § 5.0.5 절차 |
| `초기화 실패: metadata-dir 가 비어 있습니다` | 필수 플래그 누락 | `--metadata-dir`, `--id-repo` 모두 지정 |
| `backup-dir 이미 존재합니다` | 동일 backup-dir 재사용 시도 | 기본값 사용 또는 새 경로 지정 |

#### 5.0.8 다음 단계 (C2/C3 진행 시)

본 도구는 **영속 메타데이터 (`device_metadata.json`) 만** 다룬다. 시계열 DB (Influx) 의 tag 마이그레이션은 별도 도구 `xflowd migrate tsdb-tags` (Phase C2, § 5.1 참조) 의 책임이다. C1 완료 후에도 시계열 DB 는 composite tag 를 계속 보유하며, C2 에서 backfill 된다.

---

### 5.1 `xflowd migrate tsdb-tags` 도구 (Phase C § C2)

SPEC-DEVICE-IDENTITY-001 Phase C § C2 로 도입된 시계열 DB (InfluxDB v2/v3) backfill 스크립트 생성 도구이다.

#### 5.1.1 핵심 동작 모델

본 도구는 **read-only** 이다. InfluxDB 서버에 어떤 write 도 수행하지 않는다. 대신 다음 단계를 수행한다:

1. **연결 + 버전 감지**: `--target auto` 의 경우 `X-Influxdb-Version` 헤더로 v2/v3 식별.
2. **Schema 스캔**: 모든 measurement → 모든 tag key → 모든 tag value 를 수집 (read-only).
3. **분류**: composite tag value 후보를 4 카테고리로 분류 (Mapped / Orphan / Ambiguous / UUIDAlready).
4. **스크립트 생성**: Mapped entries 로부터 `migration-v2.flux` 또는 `migration-v3.sql` + `RUN.md` 생성.

**중요**: 생성된 스크립트는 운영자가 직접 staging → production 순서로 실행한다. 본 도구는 절대 자동 실행하지 않는다.

#### 5.1.2 CLI 시그니처

```bash
xflowd migrate tsdb-tags \
    --influx-url <url> \
    --influx-token <token> \
    --bucket <name> \
    [--org <org>] \
    [--id-repo <path>] \
    [--output-dir <path>] \
    [--target {v2|v3|auto}] \
    [--measurements <list>] \
    [--dry-run]
```

| 플래그 | 필수 | 의미 |
|---|---|---|
| `--influx-url` | 예 | InfluxDB 서버 주소 (예: `http://localhost:8086`) |
| `--influx-token` | 예 | 인증 토큰 (**read-only 권한 권장**) |
| `--bucket` | 예 | v2 bucket 명 또는 v3 database 명 |
| `--org` | v2 시 필수 | v2 organization 명 (v3 에서는 선택) |
| `--id-repo` | 아니오 | `device_ids.json` 경로 (기본: `~/.xflow/storage/device_ids/device_ids.json`) |
| `--output-dir` | 아니오 | 스크립트 출력 디렉토리 (기본: `./tsdb-migrations-<UTC-timestamp>`) |
| `--target` | 아니오 | `v2` / `v3` / `auto` (기본: `auto`) |
| `--measurements` | 아니오 | 특정 measurement 만 처리 (콤마 구분, 미지정 시 전체) |
| `--dry-run` | 아니오 | 스크립트 생성 없이 영향 분석만 수행 |

#### 5.1.3 분류 카테고리

도구는 schema 에서 발견된 각 tag value 를 다음 4 카테고리로 분류한다:

| 카테고리 | 의미 | 동작 |
|---|---|---|
| **Mapped** | composite → UUID 매핑이 존재하고 충돌 없음 | 스크립트에 포함 (변환 대상) |
| **UUIDAlready** | tag value 자체가 UUID v4 형식 | 건드리지 않음 (idempotent) |
| **Orphan** | composite shape 인데 `device_ids.json` 에 매핑 없음 | skip + stderr 경고 (운영자 검토 대상) |
| **Ambiguous** | 동일 UUID 가 여러 composite 와 매핑됨 (다대일) | skip + stderr 경고 (운영자가 매핑 정리 후 재실행) |

#### 5.1.4 v2 vs v3 의 차이

| 측면 | v2 (Flux) | v3 (SQL / InfluxQL) |
|---|---|---|
| Schema 조회 | `schema.measurements()`, `schema.measurementTagValues()` | `SHOW MEASUREMENTS`, `SHOW TAG VALUES` |
| 생성 스크립트 | `migration-v2.flux` (실행 가능한 Flux 쿼리) | `migration-v3.sql` (SELECT 만 — 운영자 line-protocol re-write 필요) |
| 적용 방법 | `influx query --file migration-v2.flux` | SELECT → JSON dump → line-protocol re-write (운영자 자체 도구) |
| 이유 | Flux 의 `to()` 는 동일 series 에 tag 추가 가능 | InfluxDB v3 는 tag UPDATE 미지원 — 새 row 작성 필요 |

#### 5.1.5 권장 운영 절차

1. **사전 백업** — InfluxDB snapshot / dump 생성 (운영 환경에 맞게 조정).

2. **staging dry-run** — production 직전 staging 의 동일 schema 에서 dry-run:
   ```bash
   xflowd migrate tsdb-tags \
       --influx-url http://staging-influx:8086 \
       --influx-token <STAGING_READONLY_TOKEN> \
       --bucket xflow \
       --org acme \
       --target v2 \
       --dry-run
   ```
   `Plan summary: mapped=N ambiguous=0 orphan=0 ...` 라인 확인.

3. **스크립트 생성** — dry-run 결과 만족스러우면 실제 생성:
   ```bash
   xflowd migrate tsdb-tags \
       --influx-url http://staging-influx:8086 \
       --influx-token <STAGING_READONLY_TOKEN> \
       --bucket xflow \
       --org acme \
       --target v2 \
       --output-dir ./tsdb-migrations-staging
   ```
   출력: `tsdb-migrations-staging/migration-v2.flux` + `RUN.md`.

4. **staging 실행** — 생성된 `RUN.md` 의 절차 따라 staging Influx 에 적용 후 검증.

5. **production 적용** — staging 검증 완료 후 production 에서 새로 `xflowd migrate tsdb-tags` 실행 + 동일 절차.

6. **검증** — `RUN.md` 의 § 3 "검증" 쿼리 실행 후 `uid` tag 가 채워졌는지 확인.

#### 5.1.6 ambiguous / orphan mapping 처리

도구는 안전을 위해 ambiguous (다대일) / orphan (매핑 없음) entry 를 스크립트에 포함하지 않는다. 발견 시 stderr 로 경고:

```
WARN: ambiguous mapping 2 건 — 운영자 수동 검토 필요 (생성된 스크립트에 포함되지 않음)
WARN: orphan tag value 3 건 — composite 형태이나 device_ids.json 에 매핑 없음 (skip)
```

- **Ambiguous**: `device_ids.json` 을 수동 검토하여 의도된 다대일인지 데이터 손상인지 판단. 필요 시 매핑 정리 후 재실행.
- **Orphan**: 디바이스가 데몬에 등록되지 않은 상태이거나 `device_ids.json` 손상 — Phase A/B 의 디바이스 등록 메트릭 (`xflowd_device_uid_missing_total`) 확인.

#### 5.1.7 위험 신호와 대응

| 증상 | 의미 | 대응 |
|---|---|---|
| `auto 버전 감지 실패` | Influx 서버가 헤더를 노출하지 않거나 인증 필요 | `--target v2` 또는 `--target v3` 명시 |
| `Influx 연결 실패` | 네트워크 / token 권한 부재 | `--influx-token` 권한 확인 (read-only 충분) |
| `Plan summary: ambiguous>0` | 다대일 매핑 | § 5.1.6 절차 |
| `Plan summary: orphan>0` | 등록되지 않은 composite | § 5.1.6 절차 |
| `output-dir 이미 존재합니다` | 동일 경로 재사용 시도 | 기본값 (timestamp suffix) 사용 또는 새 경로 지정 |

#### 5.1.8 안전 가드 요약

본 도구가 **절대로 하지 않는 것**:

- InfluxDB 서버에 write API 호출 (`Write`, `WritePoints`, `to(...)` 의 직접 실행).
- `device_ids.json` 또는 메타데이터 파일 수정.
- 스크립트 자동 실행 (생성만 함).
- 운영자 확인 없는 production 적용.

본 도구가 **하는 것**:

- Schema 메타데이터 조회 (read-only).
- `device_ids.json` 의 매핑 로드 (read-only).
- 스크립트 + 운영자 가이드 (`RUN.md`) 의 디스크 출력.
- 분류 결과 + 경고 stdout/stderr 출력.

#### 5.1.9 v3 line-protocol re-write 절차 (참고)

InfluxDB v3 의 backfill 은 도구가 SELECT 만 생성하므로 운영자가 후속 작업을 수행해야 한다. `RUN.md` 의 § 2 "실행 절차" 가 다음을 안내한다:

1. `migration-v3.sql` 의 SELECT 실행 → JSON / CSV dump.
2. 각 row 에 `uid` 태그 추가 (운영자 자체 변환 스크립트 또는 Telegraf / Vector transform).
3. `influx3 write` 또는 SDK 로 line-protocol 형식의 새 row 작성.

운영 환경의 도구 선택에 따라 절차가 달라지므로 본 가이드는 시그니처만 제시한다.

#### 5.1.10 C3 (Dual-tag 기간 운영) 와의 관계

C2 가 시계열 DB 의 기존 데이터를 backfill 한다면, C3 는 새로 기록되는 데이터에 두 tag (`id` + `uid`) 를 모두 자동 포함하도록 xflowd 데몬 자체를 개선한다 (별도 PR 시리즈).

C2 완료 시점부터 C3 시작 전까지는 새 데이터에 `uid` 가 없을 수 있다 — C3 가 적용된 후 데이터는 자동으로 두 tag 를 보유.

---

## 6. 운영자 가이드 (일반)

### 6.0 마이그레이션 진척도 모니터링

다음 메트릭을 주기적으로 확인하라:

```
# Phase A 의 graceful degradation 추적 (UUID 미발급 디바이스 수)
xflowd_device_uid_missing_total{agent_name="..."}

# Phase B 의 composite alias 사용 빈도 (source 별 누적)
xflowd_device_composite_use_total{source="yaml|log|rest_url|ws_event|callback"}
```

목표:

- `xflowd_device_uid_missing_total` 가 0 으로 안정화 → DeviceIDRepository 가 모든 디바이스에 매핑됨.
- `xflowd_device_composite_use_total` 가 0 또는 무시 가능 수준 → 외부 클라이언트 마이그레이션 완료.

### 6.1 Phase B → C 전환 시점 판단

- 위 두 메트릭이 안정된 후 Phase C 의 영속 메타데이터 + 시계열 DB 마이그레이션을 staging 에서 리허설.
- C1 (영속 메타데이터) 도구는 § 5.0 절차 참조. C2 (시계열 DB) 도구는 § 5.1 절차 참조. C3 (Dual-tag 운영) 은 별도 세션에서 도입 예정.

### 6.2 Phase D 진입 결정 (xflowd v1.0)

다음 조건을 모두 만족해야 한다:

1. Phase B 완료 후 최소 6개월 호환 기간 경과.
2. `xflowd_device_composite_use_total` 가 source 별로 0 또는 무시 가능 수준.
3. 모든 운영 인스턴스의 영속 메타데이터가 UUID key 로 변환 완료 (Phase C1 도구 사용).
4. 시계열 DB 의 UUID tag backfill 완료 (Phase C2 도구 사용).
5. 외부 클라이언트 (REST 호출자, MQTT 구독자, yaml 작성자) 마이그레이션 안내 공식 통보.

### 6.3 긴급 롤백 가이드

- **Phase A/B (코드 변경 only)**: `git revert` 로 즉시 롤백 가능.
- **Phase C1 (영속 메타데이터 변경)**: 도구 자동 생성 백업 (`<backup-dir>/device_metadata.json` + `manifest.json`) 에서 복원. § 5.0.6 절차 참조.
- **Phase C2 (시계열 DB 변경)**: 사전 InfluxDB snapshot 에서 복원. C2 도구 자체는 read-only 이므로 도구 실행만으로는 데이터 변경 없음. 스크립트 실행 시 운영자가 사전 백업으로부터 복원.
- **Phase D (Breaking)**: 별도 메이저 버전이므로 운영자가 v0.x 로 다운그레이드 가능.

---

## 7. 일정 안내 (잠정)

| 시점 | 이벤트 |
|---|---|
| 2026-05-26 | Phase A + B1/B2 + C1 (device-ids) + C2 (tsdb-tags) 도구 완료 |
| C2 직후 | C3 (Dual-tag 기간 운영) 진행 예정 (xflowd 데몬의 새 데이터 기록 단계에서 두 tag 자동 부착) |
| Phase C 완료 직후 | Deprecation 메트릭 모니터링 시작 (운영자 통보) |
| Phase C 완료 + 3개월 | 프로덕션 마이그레이션 (운영자 staging 리허설 → C1+C2 도구 실행 → 검증) |
| Phase C 완료 + 6개월 | Phase D 진입 검토 (preflight + 클라이언트 통보) |
| Phase C 완료 + ≥6개월 | xflowd v1.0 메이저 릴리스 (Phase D Breaking) |

> 본 일정은 잠정이며 외부 클라이언트 마이그레이션 진척도에 따라 조정된다.

---

## 8. 추가 참고

- 기술 명세: [.moai/specs/SPEC-DEVICE-IDENTITY-001/spec.md](../../.moai/specs/SPEC-DEVICE-IDENTITY-001/spec.md)
- 구현 계획: [.moai/specs/SPEC-DEVICE-IDENTITY-001/plan.md](../../.moai/specs/SPEC-DEVICE-IDENTITY-001/plan.md)
- 인수 기준: [.moai/specs/SPEC-DEVICE-IDENTITY-001/acceptance.md](../../.moai/specs/SPEC-DEVICE-IDENTITY-001/acceptance.md)
- 관련 SPEC:
  - SPEC-DEVICE-001 (Device 인터페이스 기본 정의)
  - SPEC-AGENT-001 (`agent.ResolveDeviceID` / `DeviceIDRepository`)
  - SPEC-INVENTORY-001 v0.3.0 (inventory 노드의 `uid` 키 정규화)

문의 / 마이그레이션 지원이 필요하면 운영 팀에 문의하라.
