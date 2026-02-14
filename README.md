# xflow

IoT Flow Based Programming (FBP) 플랫폼

## 프로젝트 소개

xflow는 IoT 환경을 위한 Flow Based Programming 플랫폼이다. 노드 기반 비주얼 프로그래밍 방식으로 데이터 처리 파이프라인을 구성하고, 센서 데이터 수집부터 가공, 전달까지의 전체 흐름을 관리한다.

## 주요 기능

- **메시지 시스템**: 인터페이스 기반 메시지 처리 (Payload, Metadata, 변경 이력 추적)
- **플로우 엔진**: 노드 간 데이터 전달 및 처리 파이프라인
- **생명주기 관리**: 7개 상태 머신, 공통 Lifecycle/Configurable 인터페이스, 콜백 메커니즘
- **JSONPath 지원**: dot-notation, 배열 인덱스, 와일드카드를 통한 중첩 데이터 접근
- **선택적 변경 이력**: Decorator 패턴 기반 제로 오버헤드 이력 추적
- **설정 관리**: Viper 기반 다중 소스 설정, 5단계 오버라이드, 런타임 핫 리로드

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
│   ├── config/            # 설정 관리 시스템 (Tier 2 - 횡단 관심사)
│   │   ├── errors.go      # 센티널 에러 정의 (9개)
│   │   ├── types.go       # 설정 카테고리 구조체 (7개)
│   │   ├── mutable.go     # Mutable/Immutable 키 레지스트리
│   │   ├── defaults.go    # 기본값 설정 (SetDefaults)
│   │   ├── validate.go    # 유효성 검증 (8개 검증기)
│   │   └── config.go      # Config 인터페이스, Load(), HotReload
│   │
│   └── observe/          # 관찰성 시스템 (Tier 2 - 횡단 관심사)
│       ├── options.go    # Option 패턴 (Factory, Metrics, Tracer, Stream, Observer)
│       ├── level.go      # LevelManager - 런타임 로그 레벨 관리
│       ├── stream.go     # StreamRouter - 컴포넌트별 로그 스트림 라우팅
│       ├── logger.go     # ComponentLogger, LoggerFactory - slog 기반 로깅
│       ├── metrics.go    # MetricsCollector - Prometheus 메트릭 수집
│       ├── trace.go      # Tracer, Span - 메시지 트레이싱 (noopSpan)
│       └── observe.go    # Observer 통합 구조체
└── go.mod
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

### internal/config (SPEC-CFG-001)

Viper 기반 다중 소스 설정 관리 시스템이다. 5단계 오버라이드 체인(코드 기본값 -> 기본 경로 파일 -> 사용자 지정 파일 -> 환경변수 -> CLI 플래그), 7개 카테고리별 타입 안전 접근자, Mutable/Immutable 키 레지스트리, fsnotify 기반 핫 리로드(100ms 디바운스), OnChange 콜백(패닉 복구), FIFO 변경 이력(최대 100건)을 제공한다.

- 테스트: 87개 전체 통과
- 커버리지: 96.1%
- Race Detector: 이상 없음
- 벤치마크: Load ~19K ops/s, Set ~3.3M ops/s

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
