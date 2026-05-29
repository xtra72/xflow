# HVACR-01 에이전트 config 통일 마이그레이션 (3종 — LG / Samsung / Century)

3종 HVACR-01 에이전트 (`lg_hvacr01`, `samsung_hvacr01`, `century_hvacr01`) 의 에이전트-level config 필드, 기본값, 로그 옵션을 LG 명세 기준으로 통일하면서 발생한 schema 변경에 대한 운영자 마이그레이션 가이드.

본 변경은 backend 가 deprecated alias 를 **silent accept 하지 않고 명시적 parse error 로 거부**한다. 따라서 구버전 yaml 을 그대로 부팅하면 즉시 실패하므로, 마이그레이션을 회피할 수 없다 (fail loud).

대상 노드 / 노드 config 는 변경되지 않는다 — 본 가이드는 에이전트 (`type: <vendor>_hvacr01`) 의 `transport.options` (또는 `config`) 에 한정된다. status / control / 통합 노드의 schema 변경은 `status-node-unification.md` 를 참조.

## 핵심 변화 요약

| 항목 | 이전 | 이후 |
|------|------|------|
| Samsung `report_interval` 기본값 | `"0s"` (변경 감지만) | `"60s"` (keepalive 활성, 0=비활성) |
| Samsung `auto_discovery` 기본값 | `false` | `true` (LG / Century 와 정렬) |
| Century `offline_timeout` 기본값 | `"5s"` | `"30s"` (LG / Samsung 와 정렬) |
| Century `reconnect_initial` | 인식 | **rename → `reconnect_interval`** (alias 미수용) |
| Samsung `include_raw_message_sets` | 인식 | **rename → `include_raw_hex`** (alias 미수용) |
| 3개 에이전트 `notify_interval` | alias 로 silent accept | **parse error 로 거부** |
| Century `keepalive_interval` | 인식 | **`report_interval` 로 통합** (alias 미수용, parse error) |
| Century `keepalive_mode` | 인식 | **`report_mode` 로 통합** (alias 미수용, parse error) |
| LG 로그 옵션 | `log_decode_errors` 없음 | `log_decode_errors`, `log_drops`, `log_state_updates` 추가 |
| Samsung 로그 옵션 | `log_decode_errors` 만 (v1.9.0+) | `log_drops`, `log_state_updates` 추가 |
| Century 로그 옵션 | 3개 모두 보유 | 변경 없음 |

## 1. 필드 rename (alias 미수용)

### 1.1 sed-style 일괄 치환

운영자 yaml repo 에 다음 치환을 적용한다 (대소문자 정확히 일치):

```sh
# 3개 에이전트 공통 (LG / Samsung / Century)
sed -i '' 's/^\(\s*\)notify_interval:/\1report_interval:/' examples/agents/*.yaml

# Samsung 전용
sed -i '' 's/^\(\s*\)include_raw_message_sets:/\1include_raw_hex:/' examples/agents/samsung_hvacr01-*.yaml

# Century 전용
sed -i '' 's/^\(\s*\)reconnect_initial:/\1reconnect_interval:/' examples/agents/century_hvacr01-*.yaml
sed -i '' 's/^\(\s*\)keepalive_interval:/\1report_interval:/' examples/agents/century_hvacr01-*.yaml
sed -i '' 's/^\(\s*\)keepalive_mode:/\1report_mode:/' examples/agents/century_hvacr01-*.yaml
```

(GNU sed 는 `-i` 인자 형식이 다르다 — `sed -i 's/.../.../' file` 사용.)

flow yaml 에는 본 필드들이 등장하지 않는다 — 위 5개 필드는 모두 에이전트 config 전용이다. 일부 flow yaml 이 에이전트를 inline 정의하던 경우에만 동일 치환을 적용한다.

### 1.2 의미 보존 확인

- `report_interval` (3종): 변경 감지 없이도 주기적 emit. `0` 이면 keepalive 비활성 (change-only).
- `report_mode` (Century, v0.3.9 도입): `"relative"` (default — 마지막 emit 으로부터 interval) / `"absolute"` (wall-clock 정렬, linux crontab 패턴). `keepalive_mode` 이름이 바뀌었을 뿐 의미·구현 동일.
- `reconnect_interval` (Century): tcp-client 재연결 backoff 초기값. 의미·구현 동일.
- `include_raw_hex` (3종 통일): register-decoded / state 메시지에 원시 바이트 hex 포함 여부. 의미·구현 동일.

## 2. Samsung 기본값 변경

### 2.1 `report_interval` 기본값 0 → 60s

이전: Samsung 에이전트는 `report_interval` 을 명시하지 않으면 변경 감지 시에만 emit 했다 (LG / Century 와 비대칭).

이후: 명시하지 않으면 60s 마다 keepalive emit 이 시작된다.

**영향**:
- downstream consumer 가 변경 빈도 외 별도 keepalive 가 도착하기 시작한다. trigger 값 `"report"` (LG / Century) 또는 `"keepalive"` (Century 의 v0.3.x emit 정책 — `report_interval` 의 fallback emit) 으로 구분 가능.
- 변경 감지만 원하는 환경에서는 명시적으로 `report_interval: "0s"` 설정.

```yaml
# Before — 변경 감지만 (의도적이었던 경우, 명시화 필요)
agents:
  - id: "samsung-hvacr01-01"
    type: "samsung_hvacr01"
    transport:
      options:
        # report_interval 미설정 → 기본 "0s" (변경 감지만)
        ...

# After — 동일 동작을 유지하려면 명시
agents:
  - id: "samsung-hvacr01-01"
    type: "samsung_hvacr01"
    transport:
      options:
        report_interval: "0s"   # 명시적으로 keepalive 비활성
        ...
```

### 2.2 `auto_discovery` 기본값 false → true

이전: Samsung 에이전트는 `auto_discovery: true` 를 명시해야 회선상 관측된 미등록 디바이스를 자동 등록했다.

이후: 명시하지 않으면 자동 탐색이 활성화된다 (LG / Century 와 동일).

**영향**:
- 기존 yaml 이 `auto_discovery: true` 를 명시했다면 그대로 두어도 동작 변화 없음 (단순히 생략 가능).
- 자동 탐색을 의도적으로 비활성화하던 환경 (수동 디바이스 등록만 허용) 에서는 명시적으로 `auto_discovery: false` 설정.

```yaml
# Before — 자동 탐색 명시 활성
auto_discovery: true

# After — 동일 동작, 생략 가능 (혹은 명시 유지)
# auto_discovery: true  # 이제 default — 생략 가능

# 자동 탐색 비활성을 원하는 경우 — 명시 필요
auto_discovery: false
```

## 3. Century `offline_timeout` 기본값 변경

이전: `offline_timeout` 기본값 `"5s"` (Century polling cycle 약 512ms 의 약 10배).

이후: `"30s"` (LG / Samsung 과 동일).

**영향**:
- 디바이스 offline 전이 시점이 5s → 30s 로 느려진다. dashboard 의 offline 인디케이터 반응성에 차이가 발생할 수 있다.
- 5s 의 빠른 반응을 원하는 환경 (회선이 안정적이고 빠른 fault 감지가 중요) 에서는 명시적으로 `offline_timeout: "5s"` 설정.

```yaml
# Before — 5s 빠른 반응 (의도적이었던 경우, 명시화 필요)
agents:
  - type: "century_hvacr01"
    transport:
      options:
        # offline_timeout 미설정 → 기본 "5s"
        ...

# After — 동일 동작을 유지하려면 명시
agents:
  - type: "century_hvacr01"
    transport:
      options:
        offline_timeout: "5s"
        ...
```

## 4. 로그 옵션 상향 통일

### 4.1 신규 노출 키 (3개 에이전트 모두 동일)

| 키 | LG | Samsung | Century |
|----|-----|----------|----------|
| `log_decode_errors` | **신규 (v0.18.27)** | 보유 (v1.9.0+) | 보유 |
| `log_drops` | **신규 (v0.18.27)** | **신규 (v1.18.0+)** | 보유 |
| `log_state_updates` | **신규 (v0.18.27)** | **신규 (v1.18.0+)** | 보유 |

모든 키는 boolean, default `false`. 의미는 3개 에이전트에서 동일하다:

- `log_decode_errors`: per-error WARN 로그 (CRC 불일치, frame 검증 실패, payload prefix 위반 등). 통계 카운터 (`framesInvalid` 등) 는 옵션 값과 무관하게 항상 증가.
- `log_drops`: msgCh / ring buffer 가득 참으로 인한 frame drop 시 per-drop WARN 로그. 통계 카운터 (`framesDropped` 등) 도 옵션 값과 무관하게 항상 증가.
- `log_state_updates`: device state 변경 / report / keepalive emit 시 DEBUG 로그 (운영자가 emit 흐름을 추적하고 싶을 때 활성).

운영 권장값 (3개 에이전트 공통):

```yaml
# 운영 환경 (조용한 로그)
log_decode_errors: false
log_drops: false
log_state_updates: false

# 디버깅 / RE 환경 (noise 추적)
log_decode_errors: true
log_drops: true
log_state_updates: true
```

## 5. backend 의 fail-loud 거부 동작

다음 5개 키 중 어느 하나라도 config 에 존재하면 에이전트 init 시점에 parse error 로 즉시 거부된다:

- `notify_interval` (3종 모두)
- `include_raw_message_sets` (Samsung)
- `reconnect_initial` (Century)
- `keepalive_interval` (Century)
- `keepalive_mode` (Century)

에러 메시지 예 (Century):

```
century_hvacr01 agent config: unknown key "keepalive_interval"
  hint: use "report_interval" instead (renamed for cross-vendor parity).
        See docs/migration/hvacr-config-unification.md
```

이전 (v1.6.0 / v0.6.0) 의 silent accept 동작과 달리, 본 변경은 운영자가 deprecation 사실을 인지하지 못한 채 alias 를 누적하던 문제 (구버전 yaml 이 작동하는 것처럼 보이지만 default 값이 적용됨) 를 해소한다.

## 6. 전체 마이그레이션 절차

1. **yaml 일괄 치환** (§1.1) — sed 또는 IDE find/replace 로 5개 필드를 신규 이름으로 변경.
2. **Samsung 의도 검토** (§2) — `report_interval`, `auto_discovery` 의 기본값 변경이 의도에 부합하는지 확인. 부합하지 않으면 명시.
3. **Century `offline_timeout` 검토** (§3) — 5s 의 빠른 반응이 의도였다면 명시.
4. **로그 옵션 점검** (§4) — 3개 에이전트 모두 동일한 keys 가 노출되므로, 운영 / 디버깅 환경에 맞게 통일.
5. **부팅 검증** — 부팅 즉시 parse error 가 발생하지 않는지 확인. 발생 시 에러 메시지의 `hint` 항목에 따라 추가 수정.
6. **emit 동작 검증** — Samsung `report_interval` 기본 60s 활성화로 인한 downstream message rate 증가가 감내 가능한지 확인.

## 7. 관련 SPEC 항목

- `SPEC-LG-HVACR-001` §변경 이력 v1.18.27
- `SPEC-SAMSUNG-HVACR-001` §변경 이력 v1.18.0 (이름 잠정)
- `SPEC-CENTURY-HVACR-001` §변경 이력 v0.5.0 (이름 잠정)
- `CHANGELOG.md` [Unreleased] § 3종 HVACR-01 에이전트 config 통일

## 8. FAQ

**Q: 본 변경이 frontend Web UI 에도 영향을 주는가?**

A: 예. Web UI 의 agentSchemas.ts 에서 deprecated 필드는 더 이상 노출되지 않으며, 신규 필드 / 신규 로그 옵션이 노출된다. 이미 frontend commit (`1917628`) 에 반영되어 있다.

**Q: 이전 yaml backup 을 보존하고 싶다.**

A: 운영자 책임. 본 가이드는 yaml repository 의 in-place 변경을 가정한다. 변경 전 git tag 또는 별도 백업 파일 생성을 권장.

**Q: 본 변경이 device_metadata.json / TSDB tag / influxdb measurement 에 영향을 주는가?**

A: 아니오. 본 변경은 에이전트 init 시점의 config schema 만 다루며, runtime 의 device id / state emit payload schema 는 변경되지 않는다. status / control / 통합 노드의 schema 도 변경되지 않는다 (그 부분은 `status-node-unification.md` 참조).

**Q: `report_interval` 의 의미가 3개 에이전트에서 정확히 동일한가?**

A: 의미는 동일 — "변경 감지가 없어도 이 시간마다 emit". 단, Century 는 v0.3.x 부터 trigger 값을 `"keepalive"` 로, LG / Samsung 은 v1.6.0 / v1.9.0a 부터 `"report"` 로 emit 한다. trigger 값 통일은 본 SPEC 범위 밖 (downstream consumer 가 필요 시 추가 정렬 가능).
