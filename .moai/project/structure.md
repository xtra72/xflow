# xflow - 프로젝트 구조

## 개요

xflow는 Go 표준 프로젝트 레이아웃(golang-standards/project-layout)을 기반으로 구성된다. 백엔드는 Go로, 프론트엔드 웹 대시보드는 React/TypeScript로 개발한다.

---

## 디렉토리 구조

```
xflow/
├── cmd/                          # 실행 바이너리 엔트리포인트
│   ├── xflowd/                   # 데몬 서버 엔트리포인트
│   │   └── main.go              # 데몬 서버 시작
│   ├── xflow/                    # CLI 도구 엔트리포인트
│   │   └── main.go              # CLI 클라이언트 라우팅
│   └── xflow-agent/             # 경량 데이터 수집 에이전트
│       └── main.go              # 에이전트 전용 엔트리포인트
│
├── internal/                     # 비공개 패키지 (외부 임포트 불가)
│   ├── engine/                   # FBP 런타임 엔진
│   │   ├── engine.go            # 엔진 코어 (플로우 실행 루프)
│   │   ├── scheduler.go         # 노드 스케줄링 및 실행 순서 결정
│   │   ├── backpressure.go      # 백프레셔 제어 메커니즘
│   │   ├── state.go             # 플로우 상태 관리 (시작/중지/일시정지)
│   │   ├── wire.go              # Wire 시스템 (노드 간 연결선, 바이패스/버퍼링)
│   │   ├── ttl.go               # 메시지 TTL 관리 (유효시간 만료 처리)
│   │   └── engine_test.go       # 엔진 단위 테스트
│   │
│   ├── node/                     # 내장 노드 타입 정의
│   │   ├── registry.go          # 노드 타입 레지스트리
│   │   ├── base.go              # 노드 기본 인터페이스 및 구현
│   │   ├── filter.go            # 필터 노드 (조건부 데이터 통과)
│   │   ├── transform.go         # 변환 노드 (데이터 매핑/변환)
│   │   ├── switch.go            # 분기 노드 (조건 기반 라우팅)
│   │   ├── aggregate.go         # 집계 노드 (윈도우 기반 집계)
│   │   ├── mapping.go           # 매핑 노드 (키 기반 값 매핑)
│   │   ├── bridge.go            # 브릿지 노드 (Agent-Flow 연결, 단방향/양방향/요청-응답)
│   │   ├── script.go            # 스크립트 노드 (Lua 실행, 핫 리로드)
│   │   ├── debug.go             # 디버그 노드 (로깅/검사)
│   │   ├── catch.go             # 에러 캐치 노드 (에러 메시지 수신)
│   │   ├── status.go            # 상태 수신 노드 (Agent/Node 상태 변경 이벤트)
│   │   ├── deadletter.go        # 폐기 메시지 수신 노드 (드롭/타임아웃/필터 제외)
│   │   └── node_test.go         # 노드 단위 테스트
│   │
│   ├── agent/                    # Agent 시스템 (설정 기반 프로토콜 파싱 서비스)
│   │   ├── agent.go             # Agent 인터페이스 및 기본 구현
│   │   ├── manager.go           # Agent 생명주기 관리 (시작/중지/재시작)
│   │   ├── registry.go          # Agent 등록 및 조회
│   │   ├── health.go            # Agent 헬스 체크 및 상태 모니터링
│   │   ├── shared.go            # 다중 플로우 공유 참조 카운팅
│   │   ├── agent_test.go        # Agent 프레임워크 테스트
│   │   │
│   │   ├── transport/           # Transport Interface (통신 인터페이스)
│   │   │   ├── transport.go     # Transport 인터페이스 정의
│   │   │   ├── serial.go        # Serial 통신 (RS-485/RS-232)
│   │   │   ├── tcp.go           # TCP 클라이언트/서버
│   │   │   ├── udp.go           # UDP 통신
│   │   │   └── transport_test.go # Transport 테스트
│   │   │
│   │   ├── protocol/            # Protocol Definition (프로토콜 정의 엔진)
│   │   │   ├── definition.go    # 프로토콜 정의 구조체 (필드, 포맷, 규칙)
│   │   │   ├── parser.go        # 설정 기반 바이트 파서/직렬화
│   │   │   ├── field.go         # 필드 타입 정의 (int, float, string, bytes 등)
│   │   │   ├── checksum.go      # 체크섬/CRC 검증 (CRC-16, XOR 등)
│   │   │   ├── loader.go        # YAML/JSON 프로토콜 정의 파일 로더
│   │   │   └── protocol_test.go # Protocol 테스트
│   │   │
│   │   ├── mqtt/                # MQTT Client Agent (표준)
│   │   │   ├── agent.go         # MQTT Agent 구현 (Agent 인터페이스)
│   │   │   ├── client.go        # MQTT 클라이언트 래퍼 (Eclipse Paho)
│   │   │   ├── subscriber.go    # MQTT 구독 노드
│   │   │   ├── publisher.go     # MQTT 발행 노드
│   │   │   └── mqtt_test.go     # MQTT Agent 테스트
│   │   │
│   │   ├── http/                # HTTP Client/Server Agent (표준)
│   │   │   ├── agent.go         # HTTP Agent 구현
│   │   │   ├── client.go        # HTTP 클라이언트 (폴링/웹훅)
│   │   │   ├── server.go        # HTTP 수신 엔드포인트
│   │   │   └── http_test.go     # HTTP Agent 테스트
│   │   │
│   │   ├── websocket/           # WebSocket Client/Server Agent (표준)
│   │   │   ├── agent.go         # WebSocket Agent 구현
│   │   │   ├── client.go        # WebSocket 클라이언트
│   │   │   ├── server.go        # WebSocket 서버
│   │   │   └── ws_test.go       # WebSocket Agent 테스트
│   │   │
│   │   ├── grpc/                # gRPC Client/Server Agent (표준)
│   │   │   ├── agent.go         # gRPC Agent 구현
│   │   │   ├── client.go        # gRPC 클라이언트
│   │   │   ├── server.go        # gRPC 서버
│   │   │   └── grpc_test.go     # gRPC Agent 테스트
│   │   │
│   │   ├── modbus/              # MODBUS/TCP Client Agent (SPEC-MODBUS-001)
│   │   │   ├── agent.go         # MODBUSAgent 구현 (Agent 인터페이스)
│   │   │   ├── cache.go         # 레지스터 캐시 (폴링 최적화)
│   │   │   ├── config.go        # MODBUSConfig 설정 파싱
│   │   │   ├── device.go        # MODBUS 디바이스 관리
│   │   │   ├── errors.go        # 센티널 에러 정의
│   │   │   ├── protocol.go      # MODBUS 프로토콜 인코딩/디코딩 (FC01-FC06, FC15-FC16)
│   │   │   ├── register.go      # 에이전트 타입 등록
│   │   │   ├── transport.go     # MODBUS/TCP 전송 계층
│   │   │   ├── write.go         # MODBUS 쓰기 명령 처리
│   │   │   ├── agent_test.go    # MODBUSAgent 테스트
│   │   │   ├── cache_test.go    # 레지스터 캐시 테스트
│   │   │   ├── config_test.go   # MODBUSConfig 테스트
│   │   │   ├── protocol_test.go # MODBUS 프로토콜 테스트
│   │   │   └── write_test.go    # 쓰기 명령 테스트
│   │   │
│   │   ├── modbusserver/        # MODBUS/TCP Server Agent (SPEC-MODBUS-002)
│   │   │   ├── agent.go         # MODBUSServerAgent 구현 (Agent 인터페이스)
│   │   │   ├── config.go        # MODBUSServerConfig 설정 파싱
│   │   │   ├── errors.go        # 센티널 에러 정의
│   │   │   ├── handler.go       # MODBUS 요청 핸들러 (FC01-FC06, FC15-FC16)
│   │   │   ├── listener.go      # TCP 리스너 관리
│   │   │   ├── register.go      # 에이전트 타입 등록
│   │   │   ├── register_map.go  # 레지스터 맵 관리
│   │   │   ├── request.go       # MODBUS 요청/응답 파싱
│   │   │   ├── agent_test.go    # MODBUSServerAgent 테스트
│   │   │   ├── config_test.go   # MODBUSServerConfig 테스트
│   │   │   ├── handler_test.go  # 요청 핸들러 테스트
│   │   │   ├── listener_test.go # TCP 리스너 테스트
│   │   │   ├── register_map_test.go # 레지스터 맵 테스트
│   │   │   └── request_test.go  # 요청/응답 파싱 테스트
│   │   │
│   │   ├── socket/              # TCP/UDP 소켓 통신 에이전트
│   │   │   ├── common.go       # 프레이밍 상수 및 기본값
│   │   │   ├── config.go       # 소켓 설정 파싱 및 검증
│   │   │   ├── framing.go      # Framer 인터페이스 (raw, newline, length_prefix, fixed_size)
│   │   │   ├── connection.go   # TCP 연결 관리자 (추적, IP 차단)
│   │   │   ├── tcp_server.go   # TCP 서버 에이전트
│   │   │   ├── tcp_client.go   # TCP 클라이언트 에이전트 (자동 재연결)
│   │   │   ├── udp_server.go   # UDP 서버 에이전트
│   │   │   ├── udp_client.go   # UDP 클라이언트 에이전트
│   │   │   └── register.go     # 에이전트 타입 팩토리 등록
│   │   │
│   │   ├── serial/              # 범용 시리얼 포트 에이전트 (SPEC-SERIAL-001)
│   │   │   ├── common.go       # 상수 및 기본값 (baud_rate, parity 등)
│   │   │   ├── errors.go       # 센티널 에러 정의 (13개)
│   │   │   ├── config.go       # 시리얼 설정 파싱 및 검증
│   │   │   ├── framing.go      # SerialFramer 인터페이스 (io.Reader/Writer 기반)
│   │   │   ├── agent.go        # SerialAgent 구현 (5개 인터페이스)
│   │   │   └── register.go     # 에이전트 타입 팩토리 등록
│   │   │
│   │   ├── system/              # System Agent (내장 서비스)
│   │   │   ├── store_errors.go     # 센티널 에러 정의 (8개) [SPEC-STORE-001]
│   │   │   ├── store.go            # Store 인터페이스, StoreEntry, StoreRepository, StoreAgent, agentStore [SPEC-STORE-001]
│   │   │   ├── store_options.go    # StoreOption, storeConfig, 5개 옵션 함수 [SPEC-STORE-001]
│   │   │   ├── store_volatile.go   # VolatileStore (sync.Map, lazy expiration, path.Match glob) [SPEC-STORE-001]
│   │   │   ├── store_ttl.go        # ttlManager (백그라운드 만료 스캔, atomic.Bool) [SPEC-STORE-001]
│   │   │   ├── store_namespace.go  # NamespacedStore 데코레이터 ("{ns}:{key}" 접두사) [SPEC-STORE-001]
│   │   │   ├── store_persistent.go # PersistentStore (Write-Through 캐시 + Repository) [SPEC-STORE-001]
│   │   │   ├── store_bridge.go     # BridgeHandler (메시지 기반 Store 접근 디스패처) [SPEC-STORE-001]
│   │   │   ├── timer_errors.go     # 센티널 에러 정의 (10개) [SPEC-TIMER-001]
│   │   │   ├── timer.go            # Timer 인터페이스, TimerAgent, 생명주기 관리 [SPEC-TIMER-001]
│   │   │   ├── timer_options.go    # TimerOption, timerConfig, 3개 옵션 함수 [SPEC-TIMER-001]
│   │   │   ├── timer_interval.go   # Interval 타이머 (time.Ticker, 독립 goroutine) [SPEC-TIMER-001]
│   │   │   ├── timer_cron.go       # Cron 타이머 (robfig/cron/v3, 5/6필드) [SPEC-TIMER-001]
│   │   │   ├── timer_timeout.go    # Timeout 타이머 (time.AfterFunc, 단일 실행) [SPEC-TIMER-001]
│   │   │   ├── timer_bridge.go     # BridgeHandler (메시지 기반 Timer 제어) [SPEC-TIMER-001]
│   │   │   ├── event.go         # 시스템 이벤트 Agent (발행/구독) [미구현]
│   │   │   ├── logger.go        # 로그 관리 Agent (로그 작성/스트림) [미구현]
│   │   │   ├── file.go          # 파일 시스템 Agent (읽기/쓰기/감시) [미구현]
│   │   │   └── system_test.go   # System Agent 테스트 [미구현]
│   │   │
│   │   └── samsung/             # Samsung HVACR-01 에이전트 (Samsung NASA 프로토콜, SPEC-SAMSUNG-HVACR-001)
│   │       ├── address.go       # NASAAddress 3바이트 주소 타입 및 헬퍼
│   │       ├── agent.go         # NASAAgent 구현 (Agent + MessageReceiver 인터페이스)
│   │       ├── config.go        # NASAConfig 설정 파싱
│   │       ├── crc.go           # CRC16-CCITT 체크섬
│   │       ├── device.go        # NASADevice, NASADeviceState 타입
│   │       ├── discovery.go     # 실외기/실내기 자동 디스커버리
│   │       ├── errors.go        # 센티널 에러 23개
│   │       ├── message.go       # NASAMessage, 명령 코드, Message Index 상수
│   │       ├── protocol.go      # NASAProtocol 인코딩/디코딩
│   │       ├── register.go      # 에이전트 타입 등록
│   │       ├── transport.go     # NASATransport 인터페이스 (Serial/TCP)
│   │       ├── address_test.go  # NASAAddress 테스트
│   │       ├── agent_test.go    # NASAAgent 테스트
│   │       ├── config_test.go   # NASAConfig 테스트
│   │       ├── crc_test.go      # CRC16-CCITT 테스트
│   │       ├── device_test.go   # NASADevice 테스트
│   │       ├── discovery_test.go # 디스커버리 테스트
│   │       ├── message_test.go  # NASAMessage 테스트
│   │       ├── protocol_test.go # NASAProtocol 테스트
│   │       ├── register_test.go # 타입 등록 테스트
│   │       └── transport_test.go # NASATransport 테스트
│   │
│   ├── modbus/                    # 공유 MODBUS 데이터 타입 변환 패키지
│   │   ├── types.go              # MODBUS 데이터 타입 변환 유틸리티 (uint16, int16, float32, uint32, int32)
│   │   └── types_test.go         # 타입 변환 테스트 (99.4% 커버리지)
│   │
│   ├── api/                      # REST API 핸들러
│   │   ├── router.go            # API 라우터 설정
│   │   ├── middleware.go        # 미들웨어 (로깅, CORS, 레이트리밋)
│   │   ├── handler/             # 엔드포인트별 핸들러
│   │   │   ├── flow.go          # 플로우 CRUD 핸들러
│   │   │   ├── agent.go         # Agent 관리 핸들러
│   │   │   ├── node.go          # 노드 카탈로그 핸들러
│   │   │   ├── execution.go     # 실행 제어 핸들러
│   │   │   ├── monitor.go       # 모니터링 핸들러
│   │   │   └── auth.go          # 인증 핸들러
│   │   ├── dto/                 # 데이터 전송 객체
│   │   │   ├── request.go       # 요청 DTO
│   │   │   └── response.go      # 응답 DTO
│   │   └── api_test.go          # API 통합 테스트
│   │
│   ├── auth/                     # 인증/인가
│   │   ├── jwt.go               # JWT 토큰 생성 및 검증
│   │   ├── rbac.go              # 역할 기반 접근 제어
│   │   ├── oauth.go             # OAuth2 연동
│   │   ├── apikey.go            # API 키 관리
│   │   └── auth_test.go         # 인증 테스트
│   │
│   ├── config/                   # 설정 관리 시스템
│   │   ├── errors.go            # 센티널 에러 정의 (9개)
│   │   ├── types.go             # 설정 카테고리 구조체
│   │   ├── mutable.go           # Mutable/Immutable 키 레지스트리
│   │   ├── defaults.go          # 기본값 설정
│   │   ├── validate.go          # 유효성 검증
│   │   └── config.go            # Config 인터페이스, Load, HotReload
│   │
│   ├── script/                   # Lua 스크립트 엔진
│   │   ├── engine.go            # Lua VM 관리 (GopherLua 기반)
│   │   ├── sandbox.go           # 샌드박스 환경 (메모리/CPU 제한, 함수 화이트리스트)
│   │   ├── loader.go            # 스크립트 로더 (파일, DB, 인라인)
│   │   ├── hotreload.go         # 핫 리로드 (실행 중 스크립트 교체)
│   │   ├── stdlib.go            # 내장 라이브러리 (JSON, math, string, time)
│   │   ├── bridge.go            # Go-Lua 데이터 브릿지 (메시지 변환)
│   │   └── script_test.go       # 스크립트 엔진 테스트
│   │
│   ├── observe/                  # 관찰성 (컴포넌트별 로깅/추적)
│   │   ├── logger.go            # 컴포넌트별 로거 팩토리 (slog 기반)
│   │   ├── level.go             # 런타임 로그 레벨 관리 (컴포넌트별 개별 설정)
│   │   ├── metrics.go           # 컴포넌트별 메트릭 수집 (Prometheus)
│   │   ├── trace.go             # 메시지 추적 (플로우 경로 트레이싱)
│   │   ├── stream.go            # 로그 스트림 분리 (컴포넌트별 출력 채널)
│   │   └── observe_test.go      # 관찰성 테스트
│   │
│   ├── plugin/                   # 플러그인 시스템
│   │   ├── manager.go           # 플러그인 매니저 (로드/언로드)
│   │   ├── go_plugin.go         # Go 네이티브 플러그인 로더
│   │   ├── wasm_plugin.go       # WASM 플러그인 로더 (Wazero)
│   │   ├── registry.go          # 플러그인 레지스트리
│   │   └── plugin_test.go       # 플러그인 테스트
│   │
│   ├── storage/                  # 데이터 저장소
│   │   ├── repository.go        # 저장소 인터페이스
│   │   ├── sqlite.go            # SQLite 구현 (기본)
│   │   ├── postgres.go          # PostgreSQL 구현 (프로덕션)
│   │   ├── migration.go         # 데이터베이스 마이그레이션
│   │   └── storage_test.go      # 저장소 테스트
│   │
│   └── cli/                      # CLI 명령어 정의
│       ├── root.go              # 루트 명령어 및 공통 플래그
│       ├── flow.go              # 플로우 관련 명령어
│       ├── node.go              # 노드 관련 명령어
│       ├── plugin.go            # 플러그인 관련 명령어
│       ├── server.go            # 서버 시작/중지 명령어
│       ├── config.go            # 설정 관련 명령어
│       └── cli_test.go          # CLI 테스트
│
├── pkg/                          # 공개 패키지 (외부 임포트 가능)
│   ├── flow/                     # 플로우 정의 및 직렬화 (인터페이스 기반 설계)
│   │   ├── errors.go             # 패키지 에러 정의 (12개 sentinel error)
│   │   ├── state.go              # FlowState 상태 모델 (8개 상태, 전이 규칙)
│   │   ├── node.go               # NodeDef, Port, PortDirection, AgentRef, BridgeDirection, Options 패턴
│   │   ├── connection.go         # Wire, WireMode, NewWire(), WireOption
│   │   ├── flow.go               # Flow 인터페이스, defaultFlow, NewFlow(), FlowOption, FlowConfig
│   │   ├── path.go               # NodePath, Dot 표기법 파서 (ParseNodePath, bracket escaping)
│   │   ├── serialize.go          # JSON/YAML 직렬화, FlowFromJSON, FlowFromYAML, 파일 로드/저장
│   │   ├── validate.go           # 11개 규칙 기반 플로우 유효성 검증 (ValidationError, ValidationSeverity)
│   │   ├── errors_test.go        # 에러 테스트
│   │   ├── state_test.go         # 상태 모델 테스트 (52 sub-tests)
│   │   ├── node_test.go          # 노드 정의 테스트
│   │   ├── connection_test.go    # Wire 테스트
│   │   ├── flow_test.go          # Flow 인터페이스 테스트 (33 test functions)
│   │   ├── path_test.go          # Dot 표기법 파서 테스트
│   │   ├── serialize_test.go     # JSON/YAML 직렬화 테스트 (21 tests)
│   │   └── validate_test.go      # 유효성 검증 테스트 (19 tests)
│   │
│   ├── message/                  # 메시지 타입 정의 (인터페이스 기반)
│   │   ├── message.go           # Message 인터페이스, defaultMessage, New(), Options 패턴, Clone()
│   │   ├── payload.go           # Payload 인터페이스, mapPayload, NewPayload(), deep copy 유틸리티
│   │   ├── metadata.go          # Metadata 인터페이스, mapMetadata, NewMetadata(), 시스템 키 상수
│   │   ├── history.go           # ChangeRecord 구조체, historyRecorder, Decorator 패턴 (historyPayload/historyMetadata)
│   │   ├── path.go              # JSONPath 평가 (evaluatePath, parsePath, splitPathParts, resolveTokens)
│   │   ├── json.go              # JSON 직렬화/역직렬화 (MarshalJSON, FromJSON)
│   │   ├── errors.go            # 패키지 에러 정의 (ErrKeyExists, ErrInvalidPath, ErrPathNotFound)
│   │   ├── message_test.go      # Message 테스트 (13 tests)
│   │   ├── payload_test.go      # Payload 테스트 (13 tests)
│   │   ├── metadata_test.go     # Metadata 테스트 (7 tests)
│   │   ├── history_test.go      # History 테스트 (11 tests)
│   │   ├── path_test.go         # JSONPath 테스트 (9 tests)
│   │   └── json_test.go         # 직렬화/역직렬화 테스트 (7 tests)
│   │
│   └── lifecycle/               # 공통 생명주기 관리 (상태 머신, 콜백, 헬스 체크)
│       ├── errors.go            # 센티널 에러 정의 (6개: ErrInvalidState, ErrInvalidStateTransition 등)
│       ├── errors_test.go       # errors.Is() 호환성 테스트
│       ├── state.go             # State 타입 (string 기반), 7개 상태 상수, ValidTransitions 전이 맵, ParseState
│       ├── state_test.go        # 상태 연산 테이블 기반 테스트
│       ├── event.go             # StateChangeEvent 구조체, StateChangeCallback, UnsubscribeFunc
│       ├── lifecycle.go         # Lifecycle 인터페이스 (Init, Start, Pause, Resume, Stop, State)
│       ├── configurable.go      # Configurable 인터페이스 (Configure, GetConfig)
│       ├── options.go           # BaseOption, WithName(), WithOnStateChange()
│       ├── base.go              # BaseLifecycle 구현체 (sync.Mutex, 상태 전이, 콜백 메커니즘, 패닉 복구)
│       ├── base_test.go         # 동시성, 콜백, 인터페이스 구현 테스트
│       ├── health.go            # HealthChecker 인터페이스, HealthStatus, RecoveryPolicy, DefaultRecoveryPolicy()
│       └── health_test.go       # 헬스 모듈 테스트
│
├── web/                          # 웹 대시보드 프론트엔드
│   ├── package.json             # Node.js 의존성
│   ├── tsconfig.json            # TypeScript 설정
│   ├── vite.config.ts           # Vite 빌드 설정
│   ├── tailwind.config.ts       # Tailwind CSS 설정
│   ├── src/
│   │   ├── main.tsx             # React 앱 엔트리포인트
│   │   ├── App.tsx              # 루트 컴포넌트
│   │   ├── components/          # 재사용 컴포넌트
│   │   │   ├── FlowEditor/     # 플로우 에디터 (React Flow 기반)
│   │   │   ├── Dashboard/      # 대시보드 위젯
│   │   │   ├── NodePalette/    # 노드 팔레트 (드래그 소스)
│   │   │   ├── PropertyPanel/  # 노드 속성 편집 패널
│   │   │   └── common/         # 공통 UI 컴포넌트
│   │   │       ├── ImportDialog.tsx    # 공용 Import 모달 컴포넌트 (파일 선택, 드래그 앤 드롭, JSON/YAML 파싱, 미리보기)
│   │   │       └── PanelSettingsDropdown.tsx  # 패널 설정 드롭다운 컴포넌트 (패널 제목/컬럼 구성)
│   │   ├── pages/               # 페이지 컴포넌트
│   │   │   ├── FlowEditorPage/ # 플로우 편집 페이지
│   │   │   ├── DashboardPage/  # 대시보드 페이지
│   │   │   ├── SettingsPage/   # 설정 페이지
│   │   │   └── LoginPage/      # 로그인 페이지
│   │   ├── hooks/               # 커스텀 React 훅
│   │   ├── stores/              # 상태 관리 (Zustand)
│   │   ├── services/            # API 클라이언트
│   │   ├── types/               # TypeScript 타입 정의
│   │   └── utils/               # 유틸리티 함수
│   │       ├── download.ts      # JSON 파일 다운로드 유틸리티 (Blob + URL.createObjectURL)
│   │       └── importParser.ts  # Import 파일 파싱 및 유효성 검증 유틸리티 (JSON/YAML 자동 감지)
│   └── public/                  # 정적 에셋
│
├── api/                          # API 명세
│   ├── openapi.yaml             # OpenAPI 3.0 스펙
│   └── proto/                   # gRPC Protocol Buffers 정의
│       └── xflow.proto          # xflow 서비스 프로토 정의
│
├── configs/                      # 설정 파일 템플릿
│   ├── xflow.yaml               # 서버 설정 템플릿
│   ├── xflow-agent.yaml         # 에이전트 설정 템플릿
│   └── xflow.example.yaml       # 예제 설정 (전체 옵션 포함)
│
├── deployments/                  # 배포 설정
│   ├── docker/
│   │   ├── Dockerfile           # 멀티스테이지 빌드 (xflowd + xflow)
│   │   ├── Dockerfile.agent     # 에이전트 전용 경량 이미지
│   │   └── docker-compose.yml   # 개발 환경 구성
│   └── k8s/
│       ├── deployment.yaml      # Kubernetes 디플로이먼트
│       ├── service.yaml         # Kubernetes 서비스
│       ├── configmap.yaml       # 설정 ConfigMap
│       └── ingress.yaml         # 인그레스 설정
│
├── test/                         # 통합 테스트
│   ├── integration/             # 통합 테스트 스위트
│   │   ├── flow_test.go         # 플로우 실행 통합 테스트
│   │   ├── api_test.go          # API 엔드투엔드 테스트
│   │   └── agent_test.go        # Agent 통합 테스트
│   ├── e2e/                     # E2E 테스트
│   │   └── scenario_test.go     # 시나리오 기반 테스트
│   └── testdata/                # 테스트 데이터
│       ├── flows/               # 테스트용 플로우 정의
│       └── fixtures/            # 테스트 픽스처
│
├── plugins/                      # 예제 플러그인
│   ├── example-transform/       # 예제: 커스텀 변환 노드
│   │   ├── main.go              # 플러그인 엔트리포인트
│   │   └── README.md            # 플러그인 개발 가이드
│   └── example-wasm/            # 예제: WASM 플러그인
│       ├── main.go              # WASM 빌드 소스
│       └── README.md            # WASM 플러그인 가이드
│
├── docs/                         # 프로젝트 문서
│   ├── architecture.md          # 아키텍처 설계 문서
│   ├── api-reference.md         # API 참조 문서
│   └── plugin-dev-guide.md      # 플러그인 개발 가이드
│
├── scripts/                      # 빌드 및 유틸리티 스크립트
│   ├── build.sh                 # 빌드 스크립트
│   ├── generate.sh              # 코드 생성 스크립트
│   └── migrate.sh               # DB 마이그레이션 스크립트
│
├── go.mod                        # Go 모듈 정의
├── go.sum                        # Go 의존성 체크섬
├── Makefile                      # 빌드, 테스트, 린트 태스크
├── .goreleaser.yaml              # GoReleaser 배포 설정
├── .golangci.yml                 # golangci-lint 설정
├── README.md                     # 프로젝트 README
├── LICENSE                       # 라이선스
└── CHANGELOG.md                  # 변경 이력
```

---

## 디렉토리 상세 설명

### cmd/ - 실행 바이너리 엔트리포인트

#### cmd/xflowd/

xflow 데몬 서버의 엔트리포인트이다. API 서버, Flow Engine, 웹 대시보드를 데몬 프로세스로 실행한다.

- `xflowd`: 데몬 프로세스로 백그라운드 실행 (HTTP + WebSocket + gRPC)
- `xflowd --foreground`: 포그라운드 실행 (개발/디버깅)
- `xflowd --config <path>`: 설정 파일 지정

#### cmd/xflow/

xflow CLI 도구의 엔트리포인트이다. REST API를 통해 원격 xflowd 서버에 접속하여 관리 작업을 수행한다.

- `xflow flow list`: 원격 서버의 플로우 목록 조회
- `xflow flow deploy <file>`: 원격 서버에 플로우 배포
- `xflow plugin install <name>`: 원격 서버에 플러그인 설치
- `xflow config server <url>`: 접속할 서버 주소 설정

#### cmd/xflow-agent/

경량 데이터 수집 에이전트 바이너리이다. 에지 환경에서 최소한의 리소스로 데이터를 수집하고, 중앙 xflow 서버로 전송하는 역할을 한다.

- 최소 메모리 풋프린트 (10-50MB)
- 제한된 노드 타입만 포함 (커넥터 + 필터 + 변환)
- 중앙 서버와의 자동 연결 및 설정 동기화

### internal/ - 비공개 패키지

Go의 `internal` 디렉토리 규칙에 따라 외부 프로젝트에서 임포트할 수 없는 비공개 패키지를 포함한다.

#### internal/engine/

FBP 런타임 엔진의 핵심 구현이다. 노드 그래프를 실행하고, 노드 간 데이터 스트림을 관리한다.

- **engine.go**: 플로우 실행 루프, 노드 초기화 및 종료
- **scheduler.go**: 토폴로지 정렬 기반 실행 순서 결정, 병렬 실행 계획
- **backpressure.go**: Go 채널 버퍼 기반 백프레셔, 속도 제한 및 드롭 정책
- **state.go**: 플로우 생명주기 상태 머신 (Created -> Initializing -> Running ⇄ Paused -> Stopping -> Stopped), 일시정지/재개/런타임 설정 변경 지원
- **wire.go**: Wire(연결선) 시스템. 바이패스 모드(즉시 전달, 버퍼 없음)와 버퍼 모드(설정된 수량만큼 메시지 저장) 지원. 수신 노드 상태 확인 후 전송 여부 결정
- **ttl.go**: 메시지 TTL(유효시간) 관리. 만료 메시지 자동 감지 및 폐기, Dead Letter 노드 연결 시 만료 메시지 전달

#### internal/node/

내장 노드 타입의 정의와 구현이다. 모든 노드는 공통 인터페이스를 구현한다.

- **registry.go**: 노드 타입 등록/조회, 팩토리 패턴
- **base.go**: Node 인터페이스 (Init, Process, Pause, Resume, Shutdown, Configure), BaseNode 기본 구현
- **filter.go**: 조건식 기반 데이터 필터링
- **transform.go**: JSONPath, 템플릿 기반 데이터 변환
- **switch.go**: 조건부 라우팅 (다중 출력 포트)
- **aggregate.go**: 시간/개수 기반 윈도우 집계
- **mapping.go**: 키 기반 값 매핑 (소스 필드 값으로 매핑 테이블 조회, 기본값 지원)
- **bridge.go**: Agent-Flow 브릿지 노드. Agent와 플로우를 연결하는 전용 노드. 단방향 수신(In), 단방향 송신(Out), 양방향(InOut), 요청/응답(Request-Reply) 모드 지원. 각 Agent는 하나 이상의 Bridge 노드와 연결 가능. 요청/응답 시 Correlation ID 기반 응답 라우팅. Bridge 초기화 시 config 토픽 자동 구독, 런타임 제어 메시지 처리, 셧다운 시 토픽 자동 정리
- **script.go**: Lua 스크립트 실행 노드, internal/script/ 엔진 연동, 핫 리로드 지원
- **catch.go**: 에러 캐치 노드. 플로우 내 노드에서 발생한 에러 메시지를 수신. 원본 메시지, 에러 원인, 발생 노드 정보 포함
- **status.go**: 상태 수신 노드. Agent 및 Node의 상태 전이 이벤트(시작/중지/에러 등)를 수신
- **deadletter.go**: 폐기 메시지 수신 노드. 백프레셔 드롭, 필터 제외, 타임아웃으로 폐기된 메시지를 수신. 미연결 시 자동 폐기

#### internal/modbus/

MODBUS 데이터 타입 변환 공유 패키지이다. MODBUS 프로토콜에서 사용하는 16비트 레지스터 값과 Go 네이티브 타입 간의 변환 유틸리티를 제공한다. `internal/agent/modbus/`와 `internal/agent/modbusserver/` 패키지에서 공통으로 사용한다.

- **types.go**: MODBUS 데이터 타입 변환 유틸리티. uint16, int16, float32, uint32, int32 등 MODBUS 레지스터 값과 Go 타입 간 양방향 변환 함수 제공. 빅 엔디안/리틀 엔디안 바이트 오더 지원.
- **types_test.go**: 타입 변환 테스트. 경계값, 엔디안 변환, 부호 있는/없는 정수, 부동소수점 변환 등 포괄적 테스트 (99.4% 커버리지).

#### internal/agent/

Agent 시스템의 핵심 구현이다. Agent는 Transport Interface(통신 인터페이스)와 Protocol Definition(프로토콜 정의)을 결합하여 동작하며, 플로우와 독립적으로 실행되고 여러 플로우에서 공유할 수 있다.

**프레임워크 (루트 파일):**
- **agent.go**: Agent 인터페이스 정의 (Init, Start, Stop, Pause, Resume, Health, Process, Configure), BaseAgent 기본 구현, SubscriberAgent 인터페이스 (Subscribe/Unsubscribe)
- **manager.go**: Agent 생명주기 관리 (생성, 시작, 중지, 재시작, 삭제)
- **registry.go**: 실행 중인 Agent 목록 관리, 이름 및 타입 기반 조회
- **health.go**: 주기적 헬스 체크, 연결 상태 모니터링, 장애 감지 및 자동 재시작
- **shared.go**: 다중 플로우 참조 카운팅, 플로우 삭제 시에도 다른 플로우가 사용 중이면 Agent 유지

**transport/ - Transport Interface:**
사용자가 선택 가능한 통신 인터페이스 추상화 레이어이다.
- **transport.go**: Transport 인터페이스 (Open, Close, Read, Write), 팩토리 패턴
- **serial.go**: Serial(RS-485/RS-232) 통신 구현, 보레이트/패리티/스톱비트 설정
- **tcp.go**: TCP 클라이언트/서버 구현, 연결 풀링, 타임아웃 관리
- **udp.go**: UDP 통신 구현, 멀티캐스트 지원

**protocol/ - Protocol Definition Engine:**
사용자가 프로토콜 구조를 설정하면 이에 맞게 바이트 데이터를 파싱/직렬화하는 엔진이다.
- **definition.go**: 프로토콜 정의 구조체 (메시지 포맷, 헤더, 페이로드 레이아웃)
- **parser.go**: 설정 기반 바이트스트림 파서 및 직렬화기
- **field.go**: 필드 타입 정의 (uint8, uint16, int32, float32, string, bytes, bitmask 등)
- **checksum.go**: 체크섬/CRC 검증 알고리즘 (CRC-16, CRC-32, XOR, Modbus CRC 등)
- **loader.go**: YAML/JSON 프로토콜 정의 파일 로더 및 검증

**표준 Agent (사전 정의된 프로토콜):**
- **mqtt/**: MQTT Client Agent - Eclipse Paho Go 기반, QoS 0/1/2, 커넥션 풀링
- **http/**: HTTP Client/Server Agent - 폴링/웹훅 수신, 요청/응답 관리
- **websocket/**: WebSocket Client/Server Agent - gorilla/websocket 기반, 자동 재연결
- **grpc/**: gRPC Client/Server Agent - protobuf 기반, 스트리밍
- **modbus/ (SPEC-MODBUS-001 구현 완료)**: MODBUS/TCP Client Agent - Go 표준 라이브러리(net) 기반, FC01-FC06/FC15-FC16 기능 코드 지원. 14개 파일(소스 9 + 테스트 5)로 구성. MODBUSAgent(Agent 인터페이스), 디바이스 관리, 레지스터 캐시(폴링 최적화), 프로토콜 인코딩/디코딩, 쓰기 명령 처리. internal/modbus/ 공유 패키지를 활용한 데이터 타입 변환 지원.
- **socket/ (SPEC-SOCKET-001 구현 완료)**: TCP/UDP 소켓 통신 에이전트. Go 표준 라이브러리(net) 기반. TCP Server/Client, UDP Server/Client 4종 에이전트, net.Conn 기반 Framer로 4종 프레이밍(raw/newline/length_prefix/fixed_size) 지원. ConnectionManager를 통한 TCP 연결 추적/IP 차단. 자동 재연결.
- **serial/ (SPEC-SERIAL-001 구현 완료)**: 범용 시리얼 포트(RS-232/RS-485) 에이전트. go.bug.st/serial 라이브러리 기반. io.ReadWriteCloser 기반 SerialFramer로 4종 프레이밍 지원. USB 디바이스 분리 감지(ENXIO/EIO). BridgeNode를 통한 Input/Output/InputOutput 방향별 공유 접근. 14개 파일(소스 8 + 테스트 6), 4,120줄, 94.3% 커버리지.
- **modbusserver/ (SPEC-MODBUS-002 구현 완료)**: MODBUS/TCP Server Agent - Go 표준 라이브러리(net) 기반, FC01-FC06/FC15-FC16 기능 코드 지원. 14개 파일(소스 8 + 테스트 6)로 구성. MODBUSServerAgent(Agent 인터페이스), TCP 리스너 관리, 레지스터 맵 관리, 요청 핸들러, 요청/응답 파싱. 클라이언트 에이전트와 쌍으로 동작하여 MODBUS/TCP 양방향 통신 지원.

**시스템 Agent (내장 서비스):**
- **system/ (Store Agent, SPEC-STORE-001 구현 완료)**: 키-값 저장소 시스템 에이전트. 8개 소스 + 7개 테스트 파일로 구성. Store 인터페이스(7개 메서드), StoreAgent(BaseLifecycle 임베딩), VolatileStore(sync.Map 인메모리), PersistentStore(Write-Through 캐시 + StoreRepository), NamespacedStore("{namespace}:{key}" 데코레이터), ttlManager(lazy + 백그라운드 이중 만료), BridgeHandler(메시지 프로토콜 디스패처). 106개 테스트, 90.9% 커버리지.
- **system/event.go** (미구현): 시스템 이벤트 Agent. 플로우 상태 변경, Agent 연결/해제, 에러 발생 등 내부 이벤트를 발행/구독. 별도 설정 없이 자동 활성화
- **system/logger.go** (미구현): 로그 관리 Agent. 컴포넌트별 로그 작성, 로그 레벨 동적 제어, 로그 스트림 실시간 구독
- **system/file.go** (미구현): 파일 시스템 Agent. 로컬 파일 읽기/쓰기, 디렉토리 감시(fsnotify), 파일 변경 이벤트 발생
- **system/ (MQTT Subscriber Agent, SPEC-MQTT-001 구현 완료)**: MQTTSubscriberAgent에 SubscriberAgent 인터페이스 구현. 동적 토픽 구독/해제, 재연결 시 토픽 복원, Bridge 연동 지원
- **system/ (Timer Agent, SPEC-TIMER-001 구현 완료)**: 타이머/스케줄러 시스템 에이전트. 7개 소스 + 6개 테스트 파일로 구성. Timer 인터페이스(5개 메서드: SetInterval, SetCron, SetTimeout, Cancel, List), TimerAgent(BaseLifecycle 임베딩, Configurable, HealthChecker), IntervalTimer(time.Ticker, goroutine per timer, TickCount 추적), CronTimer(robfig/cron/v3, 5/6필드 표현식), TimeoutTimer(time.AfterFunc, 단일 실행, 자동 제거), TimerBridgeHandler(메시지 기반 Timer 제어 디스패처). 186개 테스트, 86.5% 커버리지
- **system/ (Thingplus Gateway Agent, SPEC-THINGPLUS-001 구현 완료)**: ThingsBoard Gateway MQTT API(`v1/gateway/*`) 양방향 게이트웨이 에이전트(`thingplus-gateway`). `mqtt_agent.go` 패턴 재사용(`*lifecycle.BaseLifecycle` 임베딩), Eclipse Paho 기반. 4개 소스(`thingplus_agent.go`, `thingplus_codec.go`, `thingplus_mapping.go`, `thingplus_register.go`) + 3개 테스트로 구성. 단일 MQTT 연결로 다수 하위 디바이스 프록시 — 업링크(텔레메트리 `ts=UnixMilli`/클라이언트 속성 발행 + 무손실 bounded 버퍼) 및 다운링크(`v1/gateway/rpc`·`attributes` 구독 → `thingplus.rpc.request`/`thingplus.attr.update` 방출 + RPC 응답 발행). NAME↔device_id 양방향 매핑(JSONPath 추출, repo-nil fallback), 디바이스 상태 머신(disconnected→connecting→connected), access token(MQTT username) 인증 + TLS. 짝을 이루는 stateless Bridge 어댑터 `internal/node/adapter/thingplus.go`가 플로우 경계에서 메시지 `Type()`을 보존한다. 커버리지 codec 92.6% / mapping 96.6% / adapter 95.0% / 에이전트 코어 80.4%. A7(v5 PUBACK)·A8(attributes/response 인코딩)은 라이브 브로커 스모크 테스트 이연.

**커스텀 Agent (설정 기반 프로토콜):**
- **samsung/ (SPEC-SAMSUNG-HVACR-001 구현 완료)**: Samsung HVACR-01 에이전트 (Samsung NASA 프로토콜). RS-485 시리얼 또는 TCP를 통해 삼성 시스템 에어컨을 제어하고 모니터링한다. 21개 파일(소스 11 + 테스트 10), 7,352줄, 87.4% 커버리지.
  - `address.go`: NASAAddress 3바이트 주소 타입 및 헬퍼
  - `agent.go`: NASAAgent 구현 (Agent + MessageReceiver 인터페이스)
  - `config.go`: NASAConfig 설정 파싱
  - `crc.go`: CRC16-CCITT 체크섬
  - `device.go`: NASADevice, NASADeviceState 타입
  - `discovery.go`: 실외기/실내기 자동 디스커버리
  - `errors.go`: 센티널 에러 23개
  - `message.go`: NASAMessage, 명령 코드, Message Index 상수
  - `protocol.go`: NASAProtocol 인코딩/디코딩
  - `register.go`: 에이전트 타입 등록
  - `transport.go`: NASATransport 인터페이스 (Serial/TCP)

Agent 활용 예시:
- MQTT Client Agent: 브로커 연결을 유지하며 여러 플로우에서 토픽별 구독 공유
- MODBUS/TCP Client Agent: PLC/센서 등 MODBUS 슬레이브 디바이스에서 레지스터 값을 주기적으로 폴링하여 데이터 수집, 캐시 기반 최적화로 불필요한 통신 최소화
- MODBUS/TCP Server Agent: xflow를 MODBUS/TCP 서버로 동작시켜 외부 SCADA/HMI 시스템이 xflow의 데이터를 MODBUS 레지스터로 읽기/쓰기 가능
- Samsung NASA Agent: RS-485로 에어컨 시스템 연결, 프로토콜 정의에 따라 바이트 데이터를 파싱하여 온도/상태 데이터 공유
- Custom Protocol Agent: 사용자가 YAML로 정의한 산업 프로토콜(BACnet 등)을 Serial/TCP 인터페이스로 통신

#### internal/api/

RESTful API 서버 구현이다. CLI와 웹 대시보드 모두 원격에서 이 API를 통해 xflow 데몬 서버와 상호작용한다.

- **router.go**: 엔드포인트 라우팅, API 버전 관리 (/api/v1/)
- **middleware.go**: 인증, CORS, 요청 로깅, 레이트 리밋, 에러 핸들링
- **handler/**: 도메인별 핸들러 (플로우, 노드, 실행, 모니터링, 인증)
- **dto/**: 요청/응답 데이터 전송 객체, 입력 유효성 검증

#### internal/auth/

인증 및 인가 시스템이다. 다중 인증 방식을 지원한다.

- **jwt.go**: JWT 액세스/리프레시 토큰, 토큰 갱신 로직
- **rbac.go**: 역할(Admin, Editor, Viewer) 기반 권한 관리
- **oauth.go**: OAuth2 프로바이더 (Google, GitHub) 연동
- **apikey.go**: API 키 발급, 검증, 만료 관리

#### internal/config/

Viper 기반 다중 소스 설정 관리 시스템이다. 5단계 오버라이드 체인(코드 기본값 -> 기본 경로 파일 -> 사용자 지정 파일 -> 환경변수 -> CLI 플래그)을 따라 설정을 로드한다.

- **errors.go**: 9개 센티널 에러 + ValidationErrors 집계 타입
- **types.go**: 7개 카테고리 구조체 (ServerConfig, EngineConfig, StorageConfig, AuthConfig, ObserveConfig, ScriptConfig, PluginConfig) + 6개 하위 구조체
- **mutable.go**: Mutable/Immutable 키 레지스트리. 런타임 변경 가능 키 10개 등록, 와일드카드 패턴(`server.tls.*`) 기반 Immutable 처리
- **defaults.go**: SetDefaults 함수. 7개 카테고리 전체 기본값 정의
- **validate.go**: Validate 함수. 8개 검증기(포트 범위, 스토리지 타입, 로그 레벨, 양수값, 기간 문자열, TLS 파일, 프로덕션 JWT, PostgreSQL DSN) 기반 유효성 검증
- **config.go**: Config 인터페이스, viperConfig 구현체, Load() 팩토리, 7개 카테고리 접근자, Set()(Mutable 키만 허용, 패닉 복구 콜백), OnChange() 콜백 등록/해제, WatchConfig()(fsnotify, 100ms 디바운스), FIFO 변경 이력(최대 100건)

#### internal/script/

Lua 스크립트 엔진이다. GopherLua 기반으로 플로우 실행 중 실시간 스크립트 실행 및 핫 리로드를 지원한다.

- **engine.go**: Lua VM 풀 관리, VM 생성/재사용/해제, 스크립트 컴파일 캐싱
- **sandbox.go**: 스크립트 격리 실행 환경, 메모리/CPU 사용량 제한, 위험 함수(os, io) 차단
- **loader.go**: 스크립트 소스 로딩 (파일 시스템, DB 저장, 인라인 코드)
- **hotreload.go**: 실행 중인 스크립트를 무중단 교체, 버전 관리 및 롤백
- **stdlib.go**: xflow 전용 내장 라이브러리 (JSON 인코딩/디코딩, 수학 함수, 문자열 처리, 시간 함수, 로깅)
- **bridge.go**: Go 구조체 ↔ Lua 테이블 양방향 데이터 변환, 메시지 타입 매핑

Script 노드 활용 예시:
- 센서 데이터 변환: 원시 ADC 값을 물리량(온도, 습도)으로 변환하는 Lua 함수
- 비즈니스 룰: 복합 조건 분기 로직을 Lua 스크립트로 정의하여 실시간 수정
- Agent 데이터 후처리: Agent가 수신한 바이트 데이터를 Lua로 커스텀 디코딩

#### internal/observe/

관찰성(Observability) 시스템이다. 시스템의 모든 구성 요소(Flow Engine, Agent, Node, Script Engine, Plugin, API Server)가 개별적으로 디버깅 가능하도록 로그 분리, 로그 레벨 관리, 메트릭 수집, 메시지 추적 기능을 제공한다.

- **logger.go**: slog 기반 컴포넌트별 로거 팩토리. 각 구성 요소(예: `engine.scheduler`, `agent.mqtt.client1`, `node.filter.node-3`)에 고유 로거를 할당하여 로그를 분리한다.
- **level.go**: 컴포넌트별 로그 레벨 개별 설정 및 런타임 변경. API를 통해 실행 중에 특정 컴포넌트의 로그 레벨만 DEBUG로 변경 가능 (예: `PUT /api/v1/observe/level?component=agent.mqtt&level=debug`).
- **metrics.go**: Prometheus 기반 컴포넌트별 메트릭 수집. 처리량, 오류율, 지연시간 등을 구성 요소 단위로 측정한다.
- **trace.go**: 메시지 추적. 메시지가 플로우를 통과하는 경로를 기록하여 디버깅 시 데이터 흐름을 시각화한다.
- **stream.go**: 로그 스트림 분리. 컴포넌트별로 독립적인 로그 출력 채널을 제공하여 파일, stdout, WebSocket 등 다양한 대상으로 로그를 라우팅한다.

활용 예시:
- 특정 Agent만 DEBUG 모드로 전환하여 프로토콜 파싱 과정 추적
- 특정 Node의 입/출력 메시지를 실시간으로 모니터링
- Flow Engine 스케줄러의 실행 순서 결정 과정을 상세 로깅
- Web Dashboard에서 컴포넌트별 로그 스트림을 실시간 조회

#### internal/plugin/

플러그인 시스템 구현이다. Go 네이티브 플러그인과 WASM 플러그인을 모두 지원한다.

- **manager.go**: 플러그인 생명주기 관리 (발견, 로드, 초기화, 언로드)
- **go_plugin.go**: Go plugin 패키지를 사용한 .so 파일 로딩
- **wasm_plugin.go**: Wazero 런타임을 통한 .wasm 파일 실행
- **registry.go**: 플러그인이 제공하는 노드 타입을 노드 레지스트리에 등록

#### internal/storage/

데이터 저장소 추상화 레이어이다. 리포지토리 패턴으로 저장소 구현을 교체할 수 있다.

- **repository.go**: 저장소 인터페이스 (FlowRepository, UserRepository 등)
- **sqlite.go**: SQLite 구현 (개발/단일 인스턴스)
- **postgres.go**: PostgreSQL 구현 (프로덕션/다중 인스턴스)
- **migration.go**: 스키마 마이그레이션 (golang-migrate 기반)

#### internal/cli/

CLI 명령어 정의이다. Cobra 라이브러리를 사용하여 계층적 명령어 구조를 구성한다.

- **root.go**: 루트 명령어, 글로벌 플래그 (--config, --server <url>, --format, --token)
- **flow.go**: `xflow flow [list|get|create|update|delete|deploy|start|stop]`
- **agent.go**: `xflow agent [list|get|create|start|stop|restart|delete]`
- **node.go**: `xflow node [list|info]`
- **plugin.go**: `xflow plugin [list|install|remove|update]`
- **status.go**: `xflow status` (원격 xflowd 서버 상태 조회)
- **config.go**: `xflow config [get|set|init]`

### pkg/ - 공개 패키지

외부 프로젝트에서 임포트하여 사용할 수 있는 공개 API 패키지이다.

#### pkg/flow/

인터페이스 기반 플로우 정의, 노드/와이어 구성, 상태 모델, 경로 지정 패키지이다. Flow 인터페이스(defaultFlow unexported 구현체)를 중심으로 NodeDef, Wire, Port, AgentRef 등 핵심 데이터 구조를 정의한다. FlowState 8개 상태와 전이 규칙을 제공하고, Dot 표기법(ParseNodePath)으로 플로우 내 노드를 주소 지정한다. JSON/YAML 양방향 직렬화와 파일 로드/저장을 지원하며, 11개 규칙 기반 유효성 검증(Validate)으로 Wire 참조 무결성, 노드 중복, Bridge Node AgentRef 검증 등을 수행한다. 외부 도구에서 xflow 플로우 파일을 읽고 쓸 수 있다.

#### pkg/message/

노드 간 전달되는 메시지 타입을 정의한다. 모든 공개 API는 인터페이스(Message, Payload, Metadata)로 정의되며, 구현체(defaultMessage, mapPayload, mapMetadata)는 unexported로 캡슐화한다. 플러그인 개발 시 이 패키지의 인터페이스를 사용하여 메시지를 처리한다.

- **message.go**: Message 인터페이스(ID, Timestamp, Payload, Metadata, History, HistoryEnabled, Clone)와 unexported 구현체 defaultMessage. New() 팩토리 함수와 Options 패턴(WithHistory, WithMaxHistory, WithMetadata, WithPayload) 제공
- **payload.go**: Payload 인터페이스와 unexported 구현체 mapPayload(map[string]any 기반). Add/Set/Delete/Get/GetPath/Keys/ToMap/ToJSON/Clone 연산 지원. deep copy는 수동 재귀(deepCopyMap/deepCopyValue)로 구현하여 성능 최적화
- **metadata.go**: Metadata 인터페이스와 unexported 구현체 mapMetadata(map[string]string 기반). Get/Set/Has/Remove/All/Clone 연산 지원. 시스템 메타 키 상수 정의 (MetaKeySource, MetaKeyFlowID, MetaKeyNodeID, MetaKeyTTL, MetaKeyCorrelationID)
- **history.go**: ChangeRecord 구조체(Target/Operation/Key/OldValue/NewValue/NodeID/Timestamp). Decorator 패턴으로 historyPayload와 historyMetadata가 원본을 감싸서 변경 이력을 추적. FIFO 방식으로 최대 기록 수 제한
- **path.go**: JSONPath 평가 로직 (evaluatePath, parsePath, splitPathParts, resolveTokens). dot-notation, 배열 인덱스, 와일드카드 접근 지원
- **json.go**: Message JSON 직렬화(MarshalJSON)와 역직렬화(FromJSON) 기능
- **errors.go**: 패키지 에러 변수 정의 (ErrKeyExists, ErrInvalidPath, ErrPathNotFound)

#### pkg/lifecycle/

xflow 컴포넌트(Flow, Node, Agent, Plugin 등)의 공통 생명주기를 관리하는 패키지이다. 7개 상태(Created/Initializing/Running/Paused/Stopping/Stopped/Error)와 유효 전이 규칙을 정의하고, 임베딩 가능한 BaseLifecycle 기본 구현체를 제공한다. 표준 라이브러리만 사용하며 외부 의존성이 없다.

- **errors.go**: 6개 센티널 에러 (ErrInvalidState, ErrInvalidStateTransition, ErrInvalidStateForConfigure, ErrAlreadyInitialized, ErrNotRunning, ErrNotPaused). errors.Is() 호환
- **state.go**: State 타입(string 기반), 7개 상태 상수(StateCreated/StateInitializing/StateRunning/StatePaused/StateStopping/StateStopped/StateError), String()/IsValid() 메서드, ValidTransitions 전이 맵, IsValidTransition() 유효성 검증, ParseState() 문자열 파싱
- **event.go**: StateChangeEvent 구조체(Component/From/To/Timestamp/Error), StateChangeCallback 콜백 타입, UnsubscribeFunc 구독 해제 타입
- **lifecycle.go**: Lifecycle 인터페이스 (Init/Start/Pause/Resume/Stop/State). 모든 메서드는 context.Context를 수신하여 취소/타임아웃 전파 지원
- **configurable.go**: Configurable 인터페이스 (Configure/GetConfig). Created 또는 Stopped 상태에서만 설정 변경 가능. GetConfig()는 방어적 복사본 반환
- **options.go**: BaseOption 함수 타입, WithName(이름 설정), WithOnStateChange(콜백 등록) 옵션 제공
- **base.go**: BaseLifecycle 구현체. sync.Mutex 기반 동시성 안전 상태 전이, 콜백 스냅샷 복사 후 락 해제 상태에서 호출(데드락 방지), safeCallCallback으로 패닉 복구, OnStateChange() 콜백 등록/구독 해제
- **health.go**: HealthChecker 인터페이스(HealthCheck), HealthStatus 구조체(Healthy/Message/LastChecked/Details), RecoveryPolicy 자동 복구 전략(지수 백오프, 최대 재시도, 초과 시 동작 설정), DefaultRecoveryPolicy() 합리적 기본값 제공

### web/ - 웹 대시보드 프론트엔드

React 19 + TypeScript 기반 SPA(Single Page Application)이다.

- **React Flow**: 노드 에디터 라이브러리 (드래그 앤 드롭, 연결선, 줌/팬)
- **Tailwind CSS**: 유틸리티 퍼스트 CSS 프레임워크
- **Zustand**: 경량 상태 관리 라이브러리
- **Vite**: 빠른 개발 서버 및 번들러
- **react-window 2.2.7**: 대용량 매트릭스 가상 스크롤 (TSDB/Store 데이터 뷰어, SPEC-WEB-005)

**주요 재사용 컴포넌트 및 추상화** (SPEC-WEB-005, SPEC-STORE-003):

- `web/src/services/api/seriesDataSource.ts`: `SeriesDataSource` 통합 어댑터. tsdb 와 store 두 데이터 소스를 동일 인터페이스로 추상화하여 UI 컴포넌트가 `kind` 무관하게 동작
- `web/src/services/api/tsdbCsvExport.ts`: 재사용 가능한 CSV 변환/다운로드 유틸 (UTF-8 BOM, 로컬 ISO-8601 timezone offset, zero-dep)
- `web/src/components/property/StoreKeysEditor.tsx`: Store 정적 키 + 태그 행 편집기 (운영/데이터 섹션 분리 UI). 키:태그 컬럼 비율 1:3 (`table-fixed` + `colgroup`).
- `web/src/components/property/TagFilterChips.tsx`: 재사용 가능한 태그 chip 필터 컴포넌트 + `matchesTagFilter` 유틸 (저장소 + 데이터 뷰어 공용)
- `web/src/components/common/ConfirmDialog.tsx` (SPEC-STORE-003 v0.2.0): 재사용 가능한 확인 다이얼로그 컴포넌트 (default/danger variant). 키 초기화 등 destructive 액션 보호.
- `web/src/pages/agents/PromoteToStaticDialog.tsx` (SPEC-STORE-003 v0.2.0): 동적 키 → 정적 키 변환 다이얼로그 (태그 입력 + Configure API 재사용, 별도 백엔드 변경 없음).
- `web/src/services/api/keyTagExtractor.ts` (SPEC-WEB-005 v0.5.0): 키 문자열에서 태그 자동 추출 (InfluxDB 라인 프로토콜 + colon/slash 위치 기반 segments). 정적 태그 미존재 시 fallback.

**신규 HTTP 엔드포인트** (SPEC-STORE-003):

- `GET /api/v1/store/{name}/keys?tag=k:v`: 다중 AND 태그 필터 키 목록 (v0.1.0)
- `GET /api/v1/store/{name}/tags`: 유니크한 태그 (key, values[]) 페어 목록 (v0.1.0)
- `GET /api/v1/store/{name}/keys`: 응답에 optional `tags` 맵 포함 (omitempty 하위호환, v0.1.0)
- `DELETE /api/v1/store/{name}/keys/{key}` (v0.2.0): 단일 키 초기화. 정적 키는 history만, 동적 키는 entry 완전 삭제.
- `DELETE /api/v1/store/{name}/keys` (v0.2.0): bulk 키 초기화. 응답에 `cleared_count` / `deleted_count`.

**신규 백엔드 메서드** (SPEC-STORE-003 v0.2.0):

- `Store.ClearHistory(ctx, key) error`: `VolatileStore`/`NamespacedStore`/`PersistentStore` 모두 구현. history 만 비우고 entry 메타데이터(태그) 보존.
- `UserStoreAgent.IsStaticKey(key) bool` / `DeleteEntry(ctx, key) error` / `ClearHistory(ctx, key) error`: 정적/동적 키 분기 헬퍼.
- `agentStore.SetAllowDynamicKeys(bool)` / `SetStaticKeys([]StaticKey)` (v0.1.0 post-fix): runtime Configure 적용용 setter.

**버킷 타임스탬프 정렬 정책 변경** (SPEC-STORE-003 v0.2.0): Store 집계 쿼리의 버킷 시작점을 사용자 시작 시각이 아닌 epoch 0 기준 벽시계 경계로 정렬 (`floor(tsMs / intervalMs) * intervalMs`). 1m → 초=0, 5m → 분 0/5/10/..., 1h → 분=초=0. TSDB 엔진은 이미 동일 정렬 사용 중이라 변경 없음.

### api/ - API 명세

OpenAPI 3.0 스펙과 gRPC Protocol Buffers 정의를 포함한다.

### configs/ - 설정 파일 템플릿

배포 환경별 설정 파일 템플릿이다. 실제 설정 파일은 이 템플릿을 복사하여 사용한다.

### deployments/ - 배포 설정

Docker 및 Kubernetes 배포 설정을 포함한다.

- **docker/Dockerfile**: 멀티스테이지 빌드 (빌드 → 프론트엔드 빌드 → 최종 이미지)
- **docker/Dockerfile.agent**: Alpine 기반 최소 이미지 (~20MB)
- **docker/docker-compose.yml**: 개발 환경 (xflow + PostgreSQL + Redis + MQTT 브로커)
- **k8s/**: Kubernetes 매니페스트 (Deployment, Service, ConfigMap, Ingress)

### test/ - 통합 테스트

단위 테스트와 별도로, 여러 패키지를 조합한 통합 테스트와 E2E 테스트를 포함한다.

### plugins/ - 예제 플러그인

플러그인 개발을 위한 예제 코드와 가이드를 포함한다.

---

## 패키지 의존성 흐름

```
cmd/xflow/ ──────────┐
cmd/xflow-agent/ ────┤
                     ▼
              internal/cli/
                     │
                     ▼
              internal/api/ ◄── internal/auth/
                     │
                     ▼
           internal/engine/ ◄── internal/config/
                     │
              ┌──────┼──────┐
              ▼      ▼      ▼
     internal/   internal/  internal/
      node/     agent/      plugin/
              │
              ▼
        internal/storage/
              │
              ▼
    pkg/flow/  pkg/message/  pkg/lifecycle/

  ┌─────────────────────────────────────┐
  │  internal/observe/  (횡단 관심사)    │
  │  모든 internal/ 패키지에서 임포트    │
  └─────────────────────────────────────┘
```

- `cmd/` 패키지는 `internal/` 패키지만 임포트한다
- `internal/` 패키지 간에는 의존성 방향이 상위에서 하위로 흐른다
- `pkg/` 패키지는 외부 의존성이 없는 순수 데이터 구조체 및 인터페이스이다
- `internal/observe/`는 횡단 관심사(cross-cutting concern)로 모든 `internal/` 패키지에서 임포트한다
- 순환 의존성은 인터페이스를 통해 방지한다

---

*문서 버전: 1.5.0*
*최종 수정: 2026-03-11*
*작성: MoAI Documentation Manager*
