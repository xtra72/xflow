# xflow

IoT Flow Based Programming (FBP) 플랫폼

## 프로젝트 소개

xflow는 IoT 환경을 위한 Flow Based Programming 플랫폼이다. 노드 기반 비주얼 프로그래밍 방식으로 데이터 처리 파이프라인을 구성하고, 센서 데이터 수집부터 가공, 전달까지의 전체 흐름을 관리한다.

## 주요 기능

- **메시지 시스템**: 인터페이스 기반 메시지 처리 (Payload, Metadata, 변경 이력 추적)
- **플로우 엔진**: 노드 간 데이터 전달 및 처리 파이프라인
- **JSONPath 지원**: dot-notation, 배열 인덱스, 와일드카드를 통한 중첩 데이터 접근
- **선택적 변경 이력**: Decorator 패턴 기반 제로 오버헤드 이력 추적

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
│   └── message/          # 메시지 패키지 (인터페이스 기반 설계)
│       ├── message.go    # Message 인터페이스 및 생성자
│       ├── payload.go    # Payload 인터페이스 (데이터 조작)
│       ├── metadata.go   # Metadata 인터페이스 (시스템 메타정보)
│       ├── history.go    # 변경 이력 추적 (Decorator 패턴)
│       ├── path.go       # JSONPath 평가 로직
│       ├── json.go       # JSON 직렬화/역직렬화
│       └── errors.go     # 패키지 에러 정의
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
- **외부 의존성**: github.com/google/uuid, gopkg.in/yaml.v3

## 라이선스

이 프로젝트의 라이선스는 별도로 정의된다.
