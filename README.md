# xflow

IoT Flow Based Programming (FBP) 플랫폼

## 프로젝트 소개

xflow는 IoT 환경을 위한 Flow Based Programming 플랫폼이다. 노드 기반 비주얼 프로그래밍 방식으로 데이터 처리 파이프라인을 구성하고, 센서 데이터 수집부터 가공, 전달까지의 전체 흐름을 관리한다.

## 주요 기능

- **메시지 시스템**: 인터페이스 기반 메시지 처리 (Payload, Metadata, 변경 이력 추적)
- **플로우 엔진**: 노드 간 데이터 전달 및 처리 파이프라인
- **생명주기 관리**: 7개 상태 머신, 공통 Lifecycle/Configurable 인터페이스, 콜백 메커니즘
- **키-값 저장소**: 네임스페이스 격리, 이중 TTL 만료, 휘발성/영속 백엔드, Bridge Node 메시지 프로토콜
- **JSONPath 지원**: dot-notation, 배열 인덱스, 와일드카드를 통한 중첩 데이터 접근
- **선택적 변경 이력**: Decorator 패턴 기반 제로 오버헤드 이력 추적
- **설정 관리**: Viper 기반 다중 소스 설정, 5단계 오버라이드, 런타임 핫 리로드
- **MQTT 토픽 구독**: Bridge 노드를 통한 설정/런타임 토픽 동적 구독 관리, SubscriberAgent 인터페이스
- **MODBUS/TCP 통신**: MBAP 프레임 직접 구현, FC01~FC16 읽기/쓰기, 다중 디바이스 관리, Interval/Event 모드, Write-Through 캐시
- **TCP/UDP 소켓 통신**: TCP Server/Client, UDP Server/Client 4종 에이전트, 4종 프레이밍(raw/newline/length_prefix/fixed_size), 자동 재연결, IP 차단, 다중 연결 관리
- **시리얼 포트 통신**: 범용 시리얼(RS-232/RS-485) 에이전트, 4종 프레이밍(raw/newline/length_prefix/fixed_size), USB 핫플러그 감지, BridgeNode를 통한 Input/Output/InputOutput 방향별 공유 접근
- **통합 디바이스 관리**: 프로토콜 무관 통합 디바이스 인터페이스, 중앙 레지스트리, 사용자 정의 이름 관리, 통합 편집 모드, REST API, 웹 대시보드, WebSocket 실시간 상태 업데이트
- **TSDB/Store 데이터 뷰어**: 시리즈 페이지네이션, 다중 시리즈 매트릭스 쿼리, 절대/상대 시간 모드, 인터벌 프리셋, 집계(min/max/avg), 5,000행 경고, react-window 가상 스크롤, CSV 내보내기 (SPEC-WEB-005)
- **Store 정적 키 및 태그 메타데이터**: `allow_dynamic_keys` 정책, 정적 키 정의(`keys`), 키별 태그 맵, 다중 AND 태그 필터링 API, 태그 chip UI 필터 (SPEC-STORE-003)

## 인증 (Authentication)

> **중요 (v0.2.0+)**: `basic_auth` 는 **필수** 입니다.
>
> SPEC-DASHBOARD-001 v0.2.0 부터 `serverCfg.BasicAuth.Enabled=true` 가 기본값이며,
> `/api/dashboards/*` 를 포함한 보호된 모든 엔드포인트는 유효한 JWT 를 요구합니다.
> 익명 접근은 허용되지 않습니다.

### 인증 설정 표

| 설정 | 기본값 | 설명 |
|------|--------|------|
| `serverCfg.BasicAuth.Enabled` | `true` (v0.2.0+) | basic_auth 활성화 여부. `false` 로 부팅 시 거부되거나, `XFLOW_ALLOW_NO_AUTH=1` 이 설정되어 있으면 경고 로그와 함께 강제 활성화됩니다. |
| `XFLOW_ALLOW_NO_AUTH` (env) | (미설정) | **개발/데모 환경 전용 opt-out**. `1` 로 설정하면 `BasicAuth.Enabled=false` 상태에서도 부팅을 허용하되 경고 로그를 출력하고 basic_auth 를 강제 활성화합니다. **운영 환경에서는 절대 사용하지 마세요.** |

### 자격증명 저장소

- v0.2.0 부터 자격증명은 SQLite `users` 테이블에 저장됩니다 (`~/.xflow/xflow.db`).
- 기존 `~/.xflow/users.yaml` 사용자는 부팅 시 1회 자동 이관되며, yaml 은 `users.yaml.migrated` 로 rename 됩니다. 이후 어떤 인증 흐름에서도 yaml 은 참조되지 않습니다.
- yaml 도 SQLite `users` 테이블도 비어 있는 경우 `admin/admin` (role=admin) 기본 계정이 자동 생성됩니다. **운영 환경에서는 첫 로그인 직후 반드시 비밀번호를 변경하세요.**

### 역할 (Roles)

| Role | 권한 |
|------|------|
| `admin` | 모든 API 접근 + 공유 대시보드 편집 (`PUT/DELETE /api/dashboards/shared`) |
| `editor` | 데이터 조작 가능 + 공유 대시보드는 **GET 만** 가능 |
| `viewer` | 읽기 전용 |

상세 API 명세는 [`docs/api/dashboards.md`](docs/api/dashboards.md) 참조.

## 프로젝트 구조

```
xflow/
├── pkg/
│   ├── flow/             # 플로우 패키지 (인터페이스 기반 설계)
│   │   ├── errors.go     # 패키지 에러 정의 (12개 sentinel error)
│   │   ├── state.go      # FlowState 상수, 상태 전이 맵, 유효성 검증
│   │   ├── node.go       # NodeDef, Port, PortDirection, AgentRef, BridgeDirection
│   │   ├── connection.go # Wire, WireMode, NewWire(), WireOption
│   │   ├── flow.go       # Flow 인터페이스, defaultFlow, NewFlow(), FlowOption
│   │   ├── path.go       # NodePath, Dot 표기법 파서
│   │   ├── serialize.go  # JSON/YAML 직렬화, 파일 로드/저장
│   │   └── validate.go   # 11개 규칙 기반 플로우 유효성 검증
│   │
│   ├── message/          # 메시지 패키지 (인터페이스 기반 설계)
│   │   ├── message.go    # Message 인터페이스 및 생성자
│   │   ├── payload.go    # Payload 인터페이스 (데이터 조작)
│   │   ├── metadata.go   # Metadata 인터페이스 (시스템 메타정보)
│   │   ├── history.go    # 변경 이력 추적 (Decorator 패턴)
│   │   ├── path.go       # JSONPath 평가 로직
│   │   ├── json.go       # JSON 직렬화/역직렬화
│   │   └── errors.go     # 패키지 에러 정의
│   │
│   └── lifecycle/        # 생명주기 관리 패키지 (공통 상태 머신)
│       ├── errors.go     # 센티널 에러 정의 (6개)
│       ├── state.go      # State 타입, 7개 상태 상수, 전이 규칙
│       ├── event.go      # StateChangeEvent, 콜백 타입 정의
│       ├── lifecycle.go  # Lifecycle 인터페이스 (Init/Start/Pause/Resume/Stop)
│       ├── configurable.go # Configurable 인터페이스 (Configure/GetConfig)
│       ├── options.go    # BaseOption, WithName, WithOnStateChange
│       ├── base.go       # BaseLifecycle 구현체 (상태 머신, 콜백, 패닉 복구)
│       └── health.go     # HealthChecker, HealthStatus, RecoveryPolicy
├── internal/
│   ├── agent/
│   │   ├── serialize.go    # AgentConfig JSON/YAML 직렬화 (time.Duration 문자열 변환)
│   │   ├── serialize_test.go
│   │   ├── modbus/        # MODBUS/TCP Client 에이전트 (SPEC-MODBUS-001)
│   │   │   ├── errors.go          # 센티널 에러 정의 (12개)
│   │   │   ├── protocol.go        # MBAP 프레임 빌더/파서, FC01~FC16 인코딩/디코딩
│   │   │   ├── config.go          # ModbusConfig, DeviceConfig 파싱 및 검증
│   │   │   ├── transport.go       # ModbusTCPTransport (TCP 연결, MBAP 송수신)
│   │   │   ├── device.go          # ModbusDevice (디바이스별 상태, 연결 관리)
│   │   │   ├── cache.go           # RegisterCache (staleness 감지, CompareAndUpdate)
│   │   │   ├── agent.go           # ModbusAgent 전체 구현 (Interval/Event/Direct 모드)
│   │   │   ├── write.go           # 쓰기 명령 핸들러 (FC05/06/15/16, Write-Through)
│   │   │   └── register.go        # 에이전트 타입 팩토리 등록
│   │   ├── socket/          # TCP/UDP 소켓 에이전트 (SPEC-SOCKET-001)
│   │   ├── serial/          # 시리얼 포트 에이전트 (SPEC-SERIAL-001)
│   │   │   ├── common.go            # 프레이밍 상수 및 기본값
│   │   │   ├── errors.go            # 센티널 에러 정의 (10개)
│   │   │   ├── config.go            # 소켓 설정 파싱 (TCP/UDP Server/Client)
│   │   │   ├── framing.go           # Framer 인터페이스 및 4종 구현체, ConnReader
│   │   │   ├── connection.go        # ConnectionManager (TCP 연결 추적/차단)
│   │   │   ├── tcp_server.go        # TCP 서버 에이전트 (다중 연결, IP 차단)
│   │   │   ├── tcp_client.go        # TCP 클라이언트 에이전트 (자동 재연결)
│   │   │   ├── udp_server.go        # UDP 서버 에이전트 (피어 추적)
│   │   │   ├── udp_client.go        # UDP 클라이언트 에이전트
│   │   │   └── register.go          # 에이전트 타입 팩토리 등록
│   │   └── system/        # Store 시스템 에이전트 (SPEC-STORE-001)
│   │       ├── store_errors.go     # 센티널 에러 정의 (8개)
│   │       ├── store.go            # Store 인터페이스, StoreEntry, StoreAgent, ForNamespace
│   │       ├── store_options.go    # StoreOption, storeConfig, 5개 옵션 함수
│   │       ├── store_volatile.go   # VolatileStore (sync.Map, lazy expiration)
│   │       ├── store_ttl.go        # ttlManager (백그라운드 만료 스캔)
│   │       ├── store_namespace.go  # NamespacedStore 데코레이터
│   │       ├── store_persistent.go # PersistentStore (Write-Through 캐시)
│   │       └── store_bridge.go     # BridgeHandler (메시지 디스패처)
│   │
│   ├── device/            # 통합 디바이스 모델링 및 관리 (SPEC-DEVICE-001)
│   │   ├── device.go      # Device/ControllableDevice 인터페이스, DeviceState, CommandSpec
│   │   ├── registry.go    # DeviceRegistry 중앙 레지스트리 (필터링, 메타데이터, 명령 실행)
│   │   └── adapter/
│   │       └── nasa.go    # NASADevice → Device 어댑터 (ControllableDevice 지원)
│   │
│   ├── config/            # 설정 관리 시스템 (Tier 2 - 횡단 관심사)
│   │   ├── errors.go      # 센티널 에러 정의 (9개)
│   │   ├── types.go       # 설정 카테고리 구조체 (7개) + StorageConfig
│   │   ├── mutable.go     # Mutable/Immutable 키 레지스트리
│   │   ├── defaults.go    # 기본값 설정 (SetDefaults)
│   │   ├── validate.go    # 유효성 검증 (8개 검증기)
│   │   └── config.go      # Config 인터페이스, Load(), HotReload
│   │
│   ├── storage/           # 영속 저장소 (Flow + Agent Repository)
│   │   ├── repository.go       # FlowRepository 인터페이스 (Save/Get/List/Delete/Close)
│   │   ├── file.go             # File 백엔드 (YAML, 원자적 쓰기)
│   │   ├── sqlite.go           # SQLite 백엔드 (JSON BLOB, WAL 모드)
│   │   ├── postgres.go         # PostgreSQL 백엔드 (JSONB, 커넥션 풀링)
│   │   ├── agent_repository.go # AgentRepository 인터페이스 (Save/Get/List/Delete/Close)
│   │   ├── agent_file.go       # Agent File 백엔드 (YAML, {dir}/agents/)
│   │   ├── agent_sqlite.go     # Agent SQLite 백엔드 (agents 테이블)
│   │   ├── agent_postgres.go   # Agent PostgreSQL 백엔드 (agents 테이블, JSONB)
│   │   └── factory.go          # NewRepository/NewAgentRepository 팩토리 (Type 기반 디스패치)
│   │
│   └── observe/          # 관찰성 시스템 (Tier 2 - 횡단 관심사)
│       ├── options.go    # Option 패턴 (Factory, Metrics, Tracer, Stream, Observer)
│       ├── level.go      # LevelManager - 런타임 로그 레벨 관리
│       ├── stream.go     # StreamRouter - 컴포넌트별 로그 스트림 라우팅
│       ├── logger.go     # ComponentLogger, LoggerFactory - slog 기반 로깅
│       ├── metrics.go    # MetricsCollector - Prometheus 메트릭 수집
│       ├── trace.go      # Tracer, Span - 메시지 트레이싱 (noopSpan)
│       └── observe.go    # Observer 통합 구조체
│   │
│   └── node/             # 노드 구현 (데이터 처리 노드)
│       ├── base.go        # BaseNode 공통 구현체
│       ├── bridge.go      # BridgeNode (에이전트 연동)
│       ├── filter.go      # FilterNode (조건 필터링)
│       ├── transform.go   # TransformNode (데이터 변환)
│       ├── aggregate.go   # AggregateNode (집계)
│       ├── condition.go   # 조건식 파서 (렉서, 파서, 평가기)
│       ├── expression.go  # 변환식 파서
│       ├── path.go        # JSONPath 평가
│       └── errors.go      # 센티널 에러 정의
└── go.mod
```

## 예제

`examples/` 디렉토리에 플로우, 에이전트, 설정 파일 예제가 포함되어 있다.

### 플로우 예제 (`examples/flows/`)

| 파일 | 설명 | 노드 구성 |
|------|------|-----------|
| [simple-pipeline.yaml](examples/flows/simple-pipeline.yaml) | 기본 3단계 파이프라인. HTTP 입력 → JSON 변환 → 콘솔 출력 | bridge → transform → bridge |
| [mqtt-metrics.yaml](examples/flows/mqtt-metrics.yaml) | MQTT 센서 데이터를 수신하여 다중 필드(temperature, humidity) 집계 메트릭을 생성 | bridge → filter → transform → aggregate → bridge |
| [influxdb-metrics.yaml](examples/flows/influxdb-metrics.yaml) | MQTT 센서 데이터를 InfluxDB WriteData 형식으로 변환하여 저장 | bridge → transform → bridge |
| [etl-pipeline.yaml](examples/flows/etl-pipeline.yaml) | CSV ETL 파이프라인. 추출 → 유효성 검사 → 정규화 → 중복 제거 → DB 적재 | bridge → filter → transform → filter → bridge |
| [iot-sensor.json](examples/flows/iot-sensor.json) | IoT 온도 센서 파이프라인. MQTT 수신 → JSON 파싱 → 임계값 필터(35도 초과) → Webhook 알림 + 시계열 DB 저장 | bridge → transform → filter → bridge(x2) |
| [modbus-monitoring.yaml](examples/flows/modbus-monitoring.yaml) | MODBUS/TCP PLC 레지스터 모니터링. PLC 수신 → 센서 값 추출 → 온도 임계값 필터(80도 초과) → 알람 출력 | bridge → transform → filter → bridge(x2) |
| [serial-to-ethernet-server.yaml](examples/flows/serial-to-ethernet-server.yaml) | 범용 시리얼 ↔ TCP 서버 양방향 게이트웨이. 시리얼 수신 → 프레임 변환 → TCP 서버 송신 (역방향 포함) | bridge(x2) → transform(x2) → bridge(x2) |
| [serial-to-ethernet-client.yaml](examples/flows/serial-to-ethernet-client.yaml) | 범용 시리얼 ↔ TCP 클라이언트 양방향 게이트웨이. 시리얼 수신 → 프레임 변환 → TCP 클라이언트 송신 (역방향 포함) | bridge(x2) → transform(x2) → bridge(x2) |

### 에이전트 예제 (`examples/agents/`)

| 파일 | 타입 | 설명 |
|------|------|------|
| [mqtt-sensor.yaml](examples/agents/mqtt-sensor.yaml) | `mqtt-client` | MQTT 브로커에 연결하여 센서 데이터를 구독. QoS, 자동 재연결, 동적 토픽 관리 지원 |
| [console-logger.yaml](examples/agents/console-logger.yaml) | `logger` | 수신 데이터를 stdout에 JSON 형식으로 출력 |
| [error-logger.yaml](examples/agents/error-logger.yaml) | `logger` | 에러 데이터를 stderr에 `[ERROR]` 접두사로 출력 |
| [influxdb-writer.yaml](examples/agents/influxdb-writer.yaml) | `influxdb` | InfluxDB 2.x/3.x에 데이터를 저장. 단일/배치 쓰기, 쿼리 실행 지원 |
| [http-receiver.yaml](examples/agents/http-receiver.yaml) | `http` | HTTP POST 엔드포인트에서 JSON 데이터를 수신 |
| [serial-modbus.yaml](examples/agents/serial-modbus.yaml) | `serial` | Modbus RTU 프로토콜 기반 시리얼 통신. 레지스터 폴링 지원 |
| [tcp-custom.json](examples/agents/tcp-custom.json) | `tcp` | TCP 소켓 기반 커스텀 프로토콜 통신 |
| [modbus-plc.yaml](examples/agents/modbus-plc.yaml) | `modbus-tcp` | MODBUS/TCP PLC 디바이스 통신. 다중 디바이스, 레지스터 폴링, Interval/Event 모드 지원 |
| [serial-gateway.yaml](examples/agents/serial-gateway.yaml) | `serial` | 범용 시리얼 게이트웨이 에이전트. serial-to-ethernet 예제용 (9600bps, newline 프레이밍) |
| [tcp-server-gateway.yaml](examples/agents/tcp-server-gateway.yaml) | `tcp-server` | TCP 서버 게이트웨이 에이전트. 외부 클라이언트 접속 대기 (0.0.0.0:8899) |
| [tcp-client-gateway.yaml](examples/agents/tcp-client-gateway.yaml) | `tcp-client` | TCP 클라이언트 게이트웨이 에이전트. 원격 서버 접속, 자동 재연결 |

### 설정 예제 (`examples/config/`)

| 파일 | 대상 | 설명 |
|------|------|------|
| [config.yaml](examples/config/config.yaml) | xflow CLI | CLI 클라이언트 설정. 서버 접속 URL, 인증 토큰, 출력 형식 |
| [xflow.yaml](examples/config/xflow.yaml) | xflowd 서버 | 개발 환경 서버 설정. SQLite 스토리지, CORS 허용, 디버그 로깅 |
| [xflow-production.yaml](examples/config/xflow-production.yaml) | xflowd 서버 | 프로덕션 환경 서버 설정. TLS, PostgreSQL, OAuth2, 트레이싱 활성화 |
| [agent.yaml](examples/config/agent.yaml) | xflow-agent | 경량 에지 에이전트 설정. 리소스 제한, 플러그인 비활성화 |

### 실행 방법

```bash
# 에이전트 등록
xflow agent import -f examples/agents/mqtt-sensor.yaml
xflow agent import -f examples/agents/console-logger.yaml

# 플로우 등록 및 실행
xflow flow import -f examples/flows/mqtt-metrics.yaml
xflow flow deploy mqtt-metrics
xflow flow start mqtt-metrics
```

## 구현 현황

### pkg/flow (SPEC-FLOW-001)

인터페이스 기반 플로우 정의, 노드/와이어 구성, 상태 모델, 경로 지정 패키지이다. Flow 인터페이스와 NodeDef, Wire 등 핵심 데이터 구조를 정의하고, JSON/YAML 직렬화, 11개 규칙 기반 유효성 검증, Dot 표기법 경로 파서를 제공한다.

- 테스트: 207개 전체 통과
- 커버리지: 94.2%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### pkg/message (SPEC-MSG-001)

첫 번째로 구현된 핵심 패키지이다. 노드 간 데이터 전달의 기본 단위인 Message 시스템을 제공한다.

- 테스트: 57개 전체 통과
- 커버리지: 93.6%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### pkg/lifecycle (SPEC-LIFE-001)

xflow 컴포넌트의 공통 생명주기 관리 시스템이다. 7개 상태(Created/Initializing/Running/Paused/Stopping/Stopped/Error)와 유효 전이 규칙을 정의하고, 임베딩 가능한 BaseLifecycle 기본 구현체를 제공한다. Lifecycle/Configurable 인터페이스, 상태 변경 콜백(Observer 패턴, 패닉 복구), HealthChecker/RecoveryPolicy(지수 백오프)를 포함한다. 표준 라이브러리만 사용하며 외부 의존성이 없다.

- 테스트: 132개 전체 통과
- 커버리지: 100.0%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/observe (SPEC-OBS-001)

XFlow 엔진의 횡단 관심사(Observability) 시스템이다. 컴포넌트별 구조화된 로깅, 런타임 로그 레벨 관리(와일드카드 패턴), Prometheus 메트릭 수집, 메시지 트레이싱(제로 오버헤드 noopSpan), 로그 스트림 라우팅(fan-out)을 통합 제공한다.

- 테스트: 86개 전체 통과
- 커버리지: 95.7%
- Race Detector: 이상 없음
- 벤치마크: 비활성 Tracer 0 allocs/op (2.1ns/op)

### internal/agent/system (SPEC-STORE-001)

xflow 엔진의 키-값 저장소 시스템 에이전트이다. sync.Map 기반 인메모리 저장소(VolatileStore), Write-Through 캐시 영속 저장소(PersistentStore), 네임스페이스 격리(NamespacedStore), 이중 TTL 만료(lazy expiration + 백그라운드 스캔), Bridge Node 메시지 프로토콜(BridgeHandler)을 제공한다. BaseLifecycle 임베딩으로 7상태 생명주기를 관리하고, ForNamespace() 팩토리로 플로우별 독립 키 공간을 생성한다.

- 테스트: 106개 전체 통과
- 커버리지: 90.9%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/config (SPEC-CFG-001)

Viper 기반 다중 소스 설정 관리 시스템이다. 5단계 오버라이드 체인(코드 기본값 -> 기본 경로 파일 -> 사용자 지정 파일 -> 환경변수 -> CLI 플래그), 7개 카테고리별 타입 안전 접근자, Mutable/Immutable 키 레지스트리, fsnotify 기반 핫 리로드(100ms 디바운스), OnChange 콜백(패닉 복구), FIFO 변경 이력(최대 100건)을 제공한다.

- 테스트: 87개 전체 통과
- 커버리지: 96.1%
- Race Detector: 이상 없음
- 벤치마크: Load ~19K ops/s, Set ~3.3M ops/s

### internal/agent + internal/node (SPEC-MQTT-001)

Bridge 노드를 통한 MQTT 토픽 동적 구독 관리 시스템이다. SubscriberAgent 인터페이스로 에이전트의 동적 토픽 구독/해제를 추상화하고, MQTTSubscriberAgent가 이를 구현한다. Bridge 설정 토픽 자동 구독, 런타임 제어 메시지(subscribe/unsubscribe) 처리, 셧다운 시 자동 정리, MQTT 재연결 시 토픽 복원을 지원한다.

- 테스트: 18개 신규 테스트 추가, 전체 통과
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/node - Filter 조건식 파서 (SPEC-FILTER-001)

FilterNode에 문자열 기반 조건식 파서를 추가하여, YAML 플로우 정의에서 `condition: "$.payload.temperature >= 30"` 형태로 직접 조건식을 작성할 수 있다. 렉서(Tokenizer), 재귀 하강 파서(Recursive Descent Parser), AST 평가기(Evaluator)를 포함하며, 기존 Go 함수 타입 조건식과 완전 호환된다.

- 지원 연산자: `>=`, `<=`, `==`, `!=`, `>`, `<`, `&&`, `||`, `!`, `exists()`
- 지원 타입: 숫자, 문자열, 불리언, null, JSONPath
- 조건식 예시: `$.payload.temperature >= -40 && $.payload.temperature <= 150`
- 테스트: 83+ 부테스트 전체 통과
- 커버리지: 92.5%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/node - Aggregate Group-By + Sliding Window (SPEC-AGG-002)

AggregateNode에 `group_by` 설정을 추가하여 메시지를 그룹별로 분류하고 독립 집계를 수행한다. 단일 키(`"location"`) 및 복합 키(`["location", "device_id"]`)를 지원하며, `max_groups`로 메모리 보호가 가능하다. Sliding Window(`window_type: "sliding"`)를 통해 슬라이딩 윈도우 집계도 지원한다.

- 주요 기능: Group-By 파티셔닝, 복합 키, max_groups, Sliding Window, 그룹 + Sliding Window 조합
- 지원 설정: `group_by` (string/[]string), `max_groups` (int), `window_type: "sliding"`, `slide_interval` (duration)
- 테스트: 전체 통과
- 커버리지: 92.9%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/storage - 영속 저장소 (SPEC-STORAGE-001)

플로우와 에이전트 설정을 영구 저장하는 저장소 시스템이다. FlowRepository와 AgentRepository 두 가지 인터페이스를 제공하며, 각각 Save, Get, List, Delete, Close 메서드를 정의한다. 팩토리 패턴(`NewRepository`, `NewAgentRepository`)으로 `StorageConfig.Type`에 따라 적절한 백엔드를 생성한다.

- **3가지 백엔드**:
  - **File**: YAML 형식 파일 저장. 원자적 쓰기(임시 파일 + rename)로 데이터 무결성 보장
  - **SQLite**: JSON BLOB 저장. WAL 모드로 동시 읽기 성능 최적화
  - **PostgreSQL**: JSONB 컬럼 저장. 커넥션 풀링으로 고부하 환경 지원
- **설정**: `internal/config/types.go`의 `StorageConfig` (Type, FileDirectory, SQLitePath, PostgresDSN, PoolSize)
- **서버 연동**: `cmd/xflowd/main.go`에서 기동 시 양쪽 저장소를 초기화하고 저장된 데이터를 복원
- **파일**: `internal/storage/` 디렉토리 (repository.go, file.go, sqlite.go, postgres.go, agent_repository.go, agent_file.go, agent_sqlite.go, agent_postgres.go, factory.go)

### internal/agent - Agent 영속화 (SPEC-AGENT-STORE-001)

에이전트 설정(AgentConfig)의 직렬화와 영속 저장을 담당한다. `internal/agent/serialize.go`에서 JSON/YAML 직렬화를 구현하며, `time.Duration` 필드를 사람이 읽을 수 있는 문자열(`"5s"`, `"1m30s"`)로 변환한다. AgentRepository를 통해 에이전트 설정을 3가지 백엔드(File, SQLite, PostgreSQL)에 저장하고, 서버 기동 시 저장된 에이전트를 자동 복원한다.

- **직렬화**: `internal/agent/serialize.go` (JSON/YAML, time.Duration 문자열 변환)
- **저장소**: AgentRepository (File: `{dir}/agents/` YAML, SQLite: agents 테이블, PostgreSQL: agents 테이블 JSONB)
- **복원**: 서버 시작 시 저장소에서 에이전트 설정을 로드하여 매니저에 등록
- **롤백**: 에이전트 생성 실패 시 `repo.Save` 실패 -> `manager.Delete`로 정리
- **API 연동**: `internal/api/service/agent_adapter.go`에서 에이전트 CRUD 시 저장소 동기화
- **파일**: `internal/agent/serialize.go`, `internal/agent/serialize_test.go`, `internal/storage/agent_*.go`

### internal/agent/modbus - MODBUS/TCP Client Agent (SPEC-MODBUS-001)

MODBUS/TCP 프로토콜 기반 산업용 디바이스 통신 에이전트이다. MBAP Header + PDU 프레임을 직접 구현하며, FC01~FC04 읽기와 FC05/FC06/FC15/FC16 쓰기를 지원한다. 단일 에이전트에서 여러 MODBUS 디바이스를 독립 관리하고, Interval Mode(매 주기 전체 전송)와 Event Mode(변경 감지 + heartbeat) 두 가지 동작 모드를 제공한다. Cached/Direct 읽기 모드, Write-Through 캐시, 자동 재연결, Stale 경고, BridgeInOut 양방향 통합을 포함한다.

- 테스트: 74개 전체 통과
- 커버리지: 84.2%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

### internal/device + internal/api + web - 통합 디바이스 관리 (SPEC-DEVICE-001)

프로토콜별 에이전트에 분산된 디바이스 모델(NASADevice, ModbusDevice)을 통합 인터페이스로 추상화하고, 중앙 레지스트리, REST API, 웹 관리 UI를 제공한다.

**백엔드:**
- **통합 디바이스 모델** (`internal/device/`): Device/ControllableDevice 인터페이스, DeviceState, CommandSpec/ParamSpec, DeviceProvider, DeviceFilter, DeviceMetadata
- **중앙 레지스트리** (`internal/device/registry.go`): RegisterProvider/UnregisterProvider, 필터링, 동시성 안전
- **NASA 디바이스 어댑터** (`internal/device/adapter/nasa.go`): NASADevice를 Device 인터페이스로 래핑, ControllableDevice 지원
- **디바이스 REST API** (`internal/api/handler/device.go`): 6개 엔드포인트
- **에이전트 라이프사이클 훅** (`internal/agent/manager.go`): 에이전트 시작/중지 시 DeviceProvider 자동 등록/해제
- **WebSocket 이벤트** (`internal/api/ws/event_publisher.go`): 실시간 디바이스 상태 변경 알림

**REST API:**

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/devices` | 디바이스 목록 (필터 쿼리 지원) |
| GET | `/api/devices/{id}` | 디바이스 상세 정보 |
| POST | `/api/devices/{id}/execute` | 명령 실행 |
| PUT | `/api/devices/{id}/metadata` | 메타데이터 설정 |
| GET | `/api/devices/{id}/commands` | 지원 명령 목록 |
| GET | `/api/devices/{id}/state` | 현재 상태 조회 |

**프론트엔드:**
- **디바이스 목록 페이지** (`web/src/pages/devices/DeviceListPage.tsx`): react-grid-layout 대시보드, 디바이스 카드, 필터, 검색, 디바이스 추가 다이얼로그
- **디바이스 상세 패널** (`web/src/pages/devices/DeviceDetailPanel.tsx`): 상태/속성 표시, CommandSpec 기반 동적 제어 UI, 리모컨
- **에이전트 디바이스 탭** (`web/src/pages/agents/AgentDetailPanel.tsx`): 디바이스 CRUD(추가/제거), 소스 배지(설정/동적)
- **WebSocket 연동** (`web/src/hooks/useWebSocket.ts`): 실시간 디바이스 상태 업데이트

## 빌드 및 테스트

```bash
# 전체 테스트 실행
go test ./...

# Race Detector 포함 테스트
go test -race ./...

# 커버리지 확인
go test -cover ./...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

## 기술 스택

- **언어**: Go 1.25+
- **외부 의존성**: github.com/google/uuid, gopkg.in/yaml.v3, github.com/prometheus/client_golang, github.com/spf13/viper, github.com/spf13/cobra, github.com/fsnotify/fsnotify

## 라이선스

이 프로젝트의 라이선스는 별도로 정의된다.
