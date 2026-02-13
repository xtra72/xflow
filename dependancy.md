# xflow SPEC 의존성 분석 및 우선순위

## 의존성 그래프

```
                    cmd/xflowd  cmd/xflow  cmd/xflow-agent
                         │          │           │
                         ▼          ▼           │
                    internal/api  internal/cli  │
                         │          │           │
                    internal/auth   │           │
                         │          │           │
                    ┌────┴──────────┘           │
                    ▼                           ▼
              internal/engine/ ◄──── internal/config/
                    │
         ┌──────────┼──────────┐
         ▼          ▼          ▼
   internal/    internal/   internal/
    node/       agent/      plugin/
     │            │
     │     ┌──────┼──────┐
     │     ▼      ▼      ▼
     │  transport/ protocol/ standard agents
     │            │
     ▼            ▼
   internal/    internal/
    script/     storage/
         │          │
         ▼          ▼
    pkg/message/  pkg/flow/

  ┌─────────────────────────────────────┐
  │  internal/observe/ (횡단 관심사)     │
  │  모든 internal/ 에서 임포트          │
  └─────────────────────────────────────┘
```

---

## SPEC 우선순위 (의존성 순)

### Tier 1 - 데이터 기반 (의존성 없음)

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **1** | SPEC-MSG-001 | `pkg/message/` (Message, Payload, Metadata, History) | 모든 노드, 엔진, Agent가 사용하는 핵심 데이터 구조. 변경 이력 추적 포함 |
| **2** | SPEC-FLOW-001 | `pkg/flow/` (Flow, Node, Connection/Wire 정의) | 플로우 직렬화/역직렬화, Wire 모드/버퍼/TTL 설정 구조체 |

### Tier 2 - 횡단 인프라

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **3** | SPEC-OBS-001 | `internal/observe/` (logger, level, metrics, trace, stream) | 모든 internal/ 패키지에서 임포트하는 횡단 관심사. 이후 모든 구현에서 활용 |
| **4** | SPEC-CFG-001 | `internal/config/` (config, validate, defaults, hotreload) | 설정 로딩 + 런타임 핫 리로드. 엔진/Agent/노드 모두 설정에 의존 |

### Tier 3 - 코어 런타임

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **5** | SPEC-LIFE-001 | Lifecycle + Configurable 인터페이스 | 공통 상태 머신 (Created→Running⇄Paused→Stopped). Engine/Node/Agent 모두 구현 |
| **6** | SPEC-STORE-001 | `internal/storage/` (repository, sqlite, postgres, migration) | 플로우 영속화. 엔진이 플로우 로드 시 필요 |
| **7** | SPEC-ENGINE-001 | `internal/engine/` (engine, scheduler, backpressure, state, wire, ttl) | FBP 엔진 코어. Wire 바이패스/버퍼, TTL, 백프레셔 포함 |

### Tier 4 - 노드 시스템

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **8** | SPEC-NODE-001 | `internal/node/` base + registry + 기본 노드 (filter, transform, switch, aggregate, debug) | 노드 인터페이스 + MVP 내장 노드 5종 |
| **9** | SPEC-ERR-001 | `internal/node/` catch, status, deadletter + 에러 포트 시스템 | 에러 출력 포트, Catch/Status/Dead Letter 노드, 자동 폐기 정책 |

### Tier 5 - Agent 시스템

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **10** | SPEC-AGENT-001 | `internal/agent/` 프레임워크 (agent, manager, registry, health, shared, transport/, protocol/) | Agent 코어. Transport Interface + Protocol Definition 엔진 |
| **11** | SPEC-BRIDGE-001 | `internal/node/bridge.go` | Bridge Node (In/Out/InOut/Request-Reply). Agent 프레임워크에 의존 |
| **12** | SPEC-AGENT-002 | `internal/agent/mqtt/` + `internal/agent/http/` | MVP 표준 Agent 2종. Agent 프레임워크 위에 구현 |

### Tier 6 - 스크립트 엔진

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **13** | SPEC-SCRIPT-001 | `internal/script/` + `internal/node/script.go` | Lua 엔진 (VM 풀, 샌드박스, 핫 리로드, stdlib) + Script 노드 |

### Tier 7 - API/인증/CLI

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **14** | SPEC-AUTH-001 | `internal/auth/` (jwt, rbac, apikey) | JWT 인증 (MVP). OAuth2는 확장 범위 |
| **15** | SPEC-API-001 | `internal/api/` (router, middleware, handlers, dto) | REST API. 엔진/Agent/인증에 의존 |
| **16** | SPEC-CLI-001 | `internal/cli/` + `cmd/xflowd/` + `cmd/xflow/` | CLI 도구 + 데몬 서버 엔트리포인트 |

### Tier 8 - 프론트엔드

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **17** | SPEC-WEB-001 | `web/` (React Flow 에디터, 대시보드, 모니터링) | API에 의존. 백엔드 완성 후 구현 |

### Tier 9 - 확장 (MVP 이후)

| 순위 | SPEC ID | 범위 | 근거 |
|------|---------|------|------|
| **18** | SPEC-SYS-001 | `internal/agent/system/` (Event, Logger, File, Timer, Store) | 시스템 내장 Agent 5종 |
| **19** | SPEC-PLUGIN-001 | `internal/plugin/` (Go plugin + WASM/Wazero) | 플러그인 시스템 |
| **20** | SPEC-AGENT-003 | WebSocket + gRPC Agent | 추가 표준 Agent |
| **21** | SPEC-NASA-001 | `internal/agent/samsung/` | Samsung NASA 커스텀 Agent |
| **22** | SPEC-EDGE-001 | `cmd/xflow-agent/` | 경량 에지 에이전트 |
| **23** | SPEC-DEPLOY-001 | `deployments/` (Docker, K8s, CI/CD) | 배포 자동화 |

---

## 의존성 요약

```
SPEC-MSG-001 ─┬─▶ SPEC-OBS-001 ─┬─▶ SPEC-ENGINE-001 ─▶ SPEC-NODE-001 ─▶ SPEC-ERR-001
SPEC-FLOW-001 ┘   SPEC-CFG-001 ─┘        │
                  SPEC-LIFE-001 ──────────┘   SPEC-AGENT-001 ─▶ SPEC-BRIDGE-001
                  SPEC-STORE-001 ─────────────────┘   │
                                                      ▼
                                              SPEC-AGENT-002 (MQTT, HTTP)
                                                      │
                  SPEC-SCRIPT-001 ◄───────────────────┘
                        │
                        ▼
                  SPEC-AUTH-001 ─▶ SPEC-API-001 ─▶ SPEC-CLI-001 ─▶ SPEC-WEB-001
```

---

**MVP 완성까지**: SPEC 1~17 (총 17개 SPEC)
**우선 실행 가능 단위**: SPEC 1~2를 먼저 구현하면 이후 Tier 2~4를 병렬 진행 가능
