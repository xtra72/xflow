# xflow - 기술 스택 명세

## 기술 스택 개요

xflow는 Go 기반 고성능 백엔드와 React 기반 인터랙티브 프론트엔드를 결합한 풀스택 플랫폼이다.

| 영역 | 기술 | 비고 |
|------|------|------|
| Backend | Go 1.23+ | 메인 서버, CLI, 에이전트 |
| HTTP Framework | Fiber v3 또는 Echo v4 | REST API 서버 |
| Frontend | React 19 + TypeScript 5.x | 웹 대시보드 SPA |
| Node Editor | React Flow | 플로우 에디터 UI |
| CSS | Tailwind CSS v4 | 유틸리티 퍼스트 스타일링 |
| State Management | Zustand | 경량 상태 관리 |
| Build Tool | Vite | 프론트엔드 번들러 |
| MQTT | Eclipse Paho Go | MQTT 3.1.1/5.0 클라이언트 |
| WebSocket | gorilla/websocket | 양방향 실시간 통신 |
| gRPC | google.golang.org/grpc | 고성능 서비스 간 통신 |
| Samsung NASA | 자체 프로토콜 구현 | 삼성 시스템 에어컨 제어 (RS-485/TCP) |
| MODBUS TCP/RTU | 표준 라이브러리 (net) + go.bug.st/serial (RTU) | MODBUS 클라이언트(TCP/RTU)/서버(TCP) (FC01-FC06, FC15-FC16) |
| TCP/UDP Socket | 표준 라이브러리 (net) | TCP/UDP Server/Client 에이전트 (4종 프레이밍, 자동 재연결) |
| DB (기본) | SQLite (modernc.org/sqlite) | CGo-free SQLite |
| DB (프로덕션) | PostgreSQL 16+ | 프로덕션 저장소 |
| Cache | Redis 7+ | 캐시, Pub/Sub, 세션 |
| Plugin (Go) | Go plugin 패키지 | 네이티브 플러그인 |
| Plugin (WASM) | Wazero | Go-native WASM 런타임 |
| Script Engine | Lua (GopherLua) | 실시간 스크립트 실행 엔진 |
| CLI | Cobra + Viper | CLI 프레임워크 + 설정 관리 |
| ORM | GORM 또는 sqlc | 데이터베이스 액세스 |
| Migration | golang-migrate | 스키마 마이그레이션 |
| Logging | slog (표준 라이브러리) | 구조화된 로깅 |
| Metrics | Prometheus + expvar | 메트릭 수집 |
| Testing | testing + testify | 단위/통합 테스트 |
| Linting | golangci-lint | Go 코드 품질 |
| Release | GoReleaser | 크로스 플랫폼 빌드/배포 |

---

## 프레임워크 선택 근거

### Backend: Go 1.23+

**선택 이유**:
- 네이티브 동시성(고루틴, 채널)으로 FBP 엔진의 노드 병렬 실행에 최적
- 단일 바이너리 컴파일로 배포 단순화 (Docker 이미지 최소화)
- 낮은 메모리 풋프린트로 에지 환경 에이전트에 적합
- 강력한 표준 라이브러리 (net/http, encoding/json, crypto 등)
- 정적 타입 시스템으로 컴파일 타임 안전성 보장

**대안 비교**:
- Node.js: 단일 스레드 이벤트 루프로 CPU 바운드 처리에 불리, GC 오버헤드
- Rust: 학습 곡선이 높고, 프로토타이핑 속도 느림
- Java: JVM 오버헤드, 컨테이너 환경에서 메모리 사용량 과다

### HTTP Framework: Fiber v3 또는 Echo v4

**선택 이유**:
- Fiber: fasthttp 기반 최고 수준의 HTTP 성능, Express.js 유사 API
- Echo: net/http 호환, 미들웨어 생태계 풍부, 안정성 검증
- 두 프레임워크 모두 라우팅, 미들웨어, 유효성 검증 내장
- OpenAPI 스펙 자동 생성 연동 가능

**최종 선택 기준**: 프로젝트 초기 벤치마크 결과에 따라 결정. Fiber는 순수 성능에서 우위, Echo는 표준 라이브러리 호환성에서 우위.

### Frontend: React 19 + TypeScript

**선택 이유**:
- React Flow 라이브러리와의 네이티브 통합 (노드 에디터 핵심 요구사항)
- TypeScript로 대규모 프론트엔드 코드베이스 타입 안전성 확보
- React 19의 서버 컴포넌트, 자동 배칭 등 최신 성능 최적화
- 풍부한 UI 컴포넌트 생태계 (shadcn/ui, Radix UI 등)

**대안 비교**:
- Vue: React Flow에 대응하는 성숙한 노드 에디터 라이브러리 부재
- Svelte: 생태계 규모가 작아 엔터프라이즈 UI 컴포넌트 선택지 제한
- Angular: 번들 크기가 크고, 노드 에디터 통합 옵션 제한

### React Flow (노드 에디터)

**선택 이유**:
- Node-RED 스타일의 드래그 앤 드롭 노드 에디터를 React에서 구현하는 데 최적
- 커스텀 노드 타입, 커스텀 엣지, 미니맵, 컨트롤 등 풍부한 기능
- 성능 최적화 (가상화, 지연 렌더링)
- 활발한 커뮤니티 및 지속적인 업데이트

### MQTT: Eclipse Paho Go

**선택 이유**:
- MQTT 3.1.1 및 5.0 프로토콜 완전 지원
- Eclipse Foundation 지원의 공식 Go 클라이언트
- QoS 0/1/2, 와일드카드 토픽, 유언(Will) 메시지 지원
- 자동 재연결 및 오프라인 메시지 큐잉

### Script Engine: Lua (GopherLua)

**선택 이유**:
- 경량 스크립트 엔진으로 IoT/임베디드 환경에 최적 (낮은 메모리 풋프린트)
- GopherLua: 순수 Go 구현, CGo 의존성 없음 (크로스 컴파일 호환)
- 빠른 VM 시작 시간, Lua VM 풀링으로 오버헤드 최소화
- 실행 중 스크립트 교체(핫 리로드) 용이
- 샌드박스 격리 실행 지원 (위험 함수 차단, 메모리/CPU 제한)
- 간결한 문법으로 비개발자도 데이터 변환 로직 작성 가능

**대안 비교**:
- JavaScript (Goja): 문법 친숙하나 메모리 사용량이 Lua 대비 높음
- Starlark: 결정적 실행에 적합하나 범용 스크립팅에 기능 제한
- Tengo: Go 네이티브지만 생태계/커뮤니티가 Lua 대비 작음

**활용 범위**:
- Script 노드: 플로우 내 커스텀 데이터 변환/필터링/분기 로직
- Agent 후처리: 원시 바이트 데이터의 커스텀 디코딩
- 조건부 알림: 복합 비즈니스 룰 기반 임계값 판단

### Agent: Transport Interface + Protocol Definition

**설계 원칙**:
- 사용자가 프로토콜 구조(메시지 포맷, 필드 정의, 체크섬 등)를 YAML/JSON으로 설정
- 통신 인터페이스(Serial, TCP, UDP)를 선택하여 Agent에 바인딩
- 설정 기반 바이트 파싱 엔진이 프로토콜 정의에 따라 데이터를 자동 파싱/직렬화

**Transport Interface 구현**:
- Serial(RS-485/RS-232): go.bug.st/serial 패키지 활용, 보레이트/패리티/스톱비트 설정 가능
- Serial Agent (SPEC-SERIAL-001): 범용 시리얼 포트 에이전트, io.ReadWriteCloser 기반 SerialFramer, 4종 프레이밍, USB 핫플러그 감지
- TCP: Go 표준 라이브러리 net 패키지, 연결 풀링 및 타임아웃 관리
- UDP: Go 표준 라이브러리 net 패키지, 멀티캐스트 지원

**Protocol Definition Engine**:
- 필드 타입: uint8, uint16_be/le, int32, float32, string, bytes, bitmask
- 체크섬 알고리즘: CRC-16, CRC-32, XOR, Modbus CRC
- 메시지 구조: 헤더, 페이로드, 트레일러 분리 정의
- 조건부 파싱: 헤더 값에 따라 페이로드 구조 분기

**Samsung NASA (커스텀 Agent 예시)**:
- 삼성 시스템 에어컨(NASA: Next-generation of Air-conditioning System Architecture) 프로토콜
- Transport: Serial(RS-485) 또는 TCP 인터페이스 선택
- Protocol: NASA 프로토콜 정의(nasa.yaml)로 바이트 구조 설정
- 디바이스 자동 탐색, 실내기/실외기 제어 및 모니터링

**MODBUS (표준 Agent — TCP/RTU)**:
- 산업 자동화 표준 프로토콜 MODBUS 클라이언트 및 서버 구현
- 외부 MODBUS 라이브러리 미사용 — TCP 는 Go 표준 라이브러리 net 패키지, RTU 는 go.bug.st/serial(century 재사용)만 사용
- 지원 기능 코드: FC01(Read Coils), FC02(Read Discrete Inputs), FC03(Read Holding Registers), FC04(Read Input Registers), FC05(Write Single Coil), FC06(Write Single Register), FC15(Write Multiple Coils), FC16(Write Multiple Registers)
- 클라이언트: PLC/센서 등 슬레이브 디바이스 레지스터 폴링, 캐시 기반 최적화, 디바이스 관리. 트랜스포트 선택(`transport: tcp | rtu`, 미지정 시 tcp — 하위 호환), RTU 반이중 시리얼 마스터(in-house CRC-16 poly 0xA001), 레지스터 그룹별 독립 폴링 주기, 플로우 노드를 통한 런타임 재구성(`set_config`) 지원 (SPEC-MODBUS-006). type id `modbus-tcp` 보존
- 서버: xflow를 MODBUS/TCP 서버로 동작, 외부 SCADA/HMI 시스템 연동, 레지스터 맵 관리
- 공유 데이터 타입 변환: internal/modbus/ 패키지에서 uint16/int16/float32/uint32/int32 + raw 패스스루 + 4순열 바이트순서(ABCD/BADC/CDAB/DCBA) 레지스터 변환 유틸리티 제공

### Storage: SQLite + PostgreSQL 이중 전략

**선택 이유**:
- SQLite: 설정 없이 즉시 사용, 개발 환경 및 단일 인스턴스 운영에 적합
- PostgreSQL: 다중 인스턴스, 고가용성, 복잡한 쿼리 지원
- 리포지토리 패턴으로 저장소 구현을 추상화하여 교체 용이
- modernc.org/sqlite 사용으로 CGo 의존성 제거 (크로스 컴파일 용이)

### Plugin: Go Plugin + WASM (Wazero)

**선택 이유**:
- Go Plugin: 네이티브 성능, Go 개발자에게 친숙한 개발 경험
- WASM (Wazero): 언어 무관 플러그인 (Rust, C, AssemblyScript 등), 샌드박스 보안
- Wazero는 CGo 없이 순수 Go로 구현된 WASM 런타임 (크로스 컴파일 호환)
- 두 방식을 동시 지원하여 사용자 선택권 극대화

### CLI: Cobra + Viper

**선택 이유**:
- Cobra: Go CLI 사실상 표준 (kubectl, docker, gh 등이 사용)
- Viper: 설정 파일, 환경 변수, CLI 플래그 통합 관리
- 자동 도움말 생성, 자동 완성(bash, zsh, fish, PowerShell) 지원
- 서브커맨드 구조로 직관적인 명령어 체계 구성
- CLI는 원격 API 클라이언트로 동작하여 데몬 서버에 접속

---

## 개발 환경 요구사항

### 필수 요구사항

| 도구 | 최소 버전 | 용도 |
|------|-----------|------|
| Go | 1.23+ | 백엔드 빌드 및 실행 |
| Node.js | 22+ (LTS) | 프론트엔드 빌드 |
| npm/pnpm | 10+ / 9+ | Node.js 패키지 관리 |
| Docker | 24+ | 컨테이너 빌드 및 실행 |
| Docker Compose | v2+ | 개발 환경 구성 |
| Git | 2.40+ | 버전 관리 |

### 선택 요구사항

| 도구 | 용도 |
|------|------|
| golangci-lint | Go 코드 정적 분석 |
| air | Go 핫 리로딩 (개발 시) |
| protoc | Protocol Buffers 컴파일 |
| buf | Protobuf 린팅 및 관리 |
| GoReleaser | 릴리스 빌드 자동화 |
| k9s | Kubernetes 클러스터 관리 TUI |
| mosquitto | 로컬 MQTT 브로커 (테스트용) |

### 권장 IDE/에디터 설정

- VS Code + Go 확장 (gopls 통합)
- VS Code + ESLint + Prettier (프론트엔드)
- VS Code + Tailwind CSS IntelliSense
- GoLand (JetBrains) - Go 전용 IDE

---

## 빌드 및 배포 설정

### Makefile 태스크

```makefile
# 주요 빌드 태스크
make build          # xflowd(서버) + xflow(CLI) 바이너리 빌드
make build-agent    # xflow-agent 바이너리 빌드
make build-web      # 프론트엔드 빌드
make build-all      # 전체 빌드 (서버 + 에이전트 + 웹)

# 개발
make dev            # xflowd 개발 서버 시작 (핫 리로딩)
make dev-web        # 프론트엔드 개발 서버
make dev-all        # 백엔드 + 프론트엔드 동시 실행

# 테스트
make test           # 단위 테스트 실행
make test-race      # 레이스 디텍터 포함 테스트
make test-cover     # 커버리지 리포트 생성
make test-integration # 통합 테스트 실행
make test-e2e       # E2E 테스트 실행

# 코드 품질
make lint           # golangci-lint 실행
make fmt            # 코드 포맷팅 (gofmt + goimports)
make vet            # go vet 실행
make check          # lint + vet + test 전체 검증

# 코드 생성
make generate       # go generate 실행
make proto          # protobuf 컴파일

# Docker
make docker-build   # Docker 이미지 빌드
make docker-up      # Docker Compose 환경 시작
make docker-down    # Docker Compose 환경 중지

# 데이터베이스
make migrate-up     # 마이그레이션 적용
make migrate-down   # 마이그레이션 롤백
make migrate-create # 새 마이그레이션 생성

# 릴리스
make release        # GoReleaser로 릴리스 빌드
make snapshot       # GoReleaser 스냅샷 빌드 (테스트용)
```

### Docker Compose (개발 환경)

개발 환경은 Docker Compose로 다음 서비스를 구성한다:

- **xflowd**: 데몬 서버 (API + Flow Engine + 웹 대시보드)
- **postgres**: PostgreSQL 데이터베이스
- **redis**: Redis 캐시 및 Pub/Sub
- **mosquitto**: Eclipse Mosquitto MQTT 브로커 (테스트용)
- **prometheus**: 메트릭 수집 (선택)
- **grafana**: 메트릭 시각화 (선택)

### CI/CD 파이프라인 (GitHub Actions)

```
Push/PR → Lint → Test → Build → (main) → Docker Build → Deploy
```

- **Lint 단계**: golangci-lint, ESLint, Prettier
- **Test 단계**: 단위 테스트 (race detector 포함), 커버리지 85% 이상
- **Build 단계**: Go 바이너리 빌드, 프론트엔드 빌드
- **Docker 단계**: 멀티스테이지 빌드, 이미지 태깅
- **Deploy 단계**: Kubernetes 배포 (main 브랜치)

---

## 아키텍처 개요

### 시스템 레이어

xflow의 데이터 처리 파이프라인은 4개의 레이어로 구성된다.

```
┌─────────────────────────────────────────────────────┐
│                   Client Layer                       │
│         (Web Dashboard / CLI / External API)         │
├─────────────────────────────────────────────────────┤
│                   API Layer                          │
│              (REST API / WebSocket)                  │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌──────────┐    ┌──────────────┐    ┌──────────┐  │
│  │  Agent   │───▶│  Flow Engine  │───▶│  Output  │  │
│  │  Layer   │    │   (FBP Core)  │    │  Layer   │  │
│  │          │    │               │    │          │  │
│  │  MQTT    │    │  ┌─────────┐  │    │  DB      │  │
│  │  HTTP    │    │  │ Node    │  │    │  MQTT    │  │
│  │  WebSocket│   │  │ Graph   │  │    │  HTTP    │  │
│  │  gRPC    │    │  │ Runtime │  │    │  WebSocket│ │
│  │  NASA    │    │  └─────────┘  │    │  gRPC    │  │
│  │  MODBUS  │    │               │    │  MODBUS  │  │
│  └──────────┘    └──────────────┘    └──────────┘  │
│                                                      │
├─────────────────────────────────────────────────────┤
│                 Infrastructure Layer                  │
│     (SQLite/PostgreSQL / Redis / Plugin Runtime)     │
└─────────────────────────────────────────────────────┘
```

#### 1. Agent Layer (설정 기반 프로토콜 파싱 레이어)

Agent는 Transport Interface(통신 인터페이스)와 Protocol Definition(프로토콜 정의)을 결합하여 동작한다. 사용자가 프로토콜 구조를 설정하면 이에 맞게 데이터를 파싱하며, 통신 인터페이스를 선택할 수 있다.

- **Transport Interface**: Serial(RS-485/RS-232), TCP, UDP 등 통신 인터페이스 추상화
- **Protocol Definition**: 사용자 설정 기반 바이트 파싱 엔진 (메시지 포맷, 필드, 체크섬 정의)
- **표준 Agent**: MQTT, HTTP, WebSocket, gRPC, MODBUS/TCP, ThingsBoard Gateway MQTT(`thingplus-gateway`, `v1/gateway/*` 다중화 게이트웨이 — 기존 Eclipse Paho 재사용, 신규 의존성 없음) (사전 정의된 프로토콜)
- **커스텀 Agent**: Samsung NASA 등 사용자 정의 프로토콜 (YAML 설정 기반)
- Agent 프레임워크: 독립 생명주기, 다중 플로우 공유, 참조 카운팅 기반 관리
- 커넥션 풀링: Agent가 연결을 유지하여 플로우 재배포 시에도 연결 단절 없음

#### 2. Flow Engine (플로우 엔진 레이어)

FBP 런타임의 핵심이다. 노드 그래프를 실행하고 데이터 스트림을 관리한다.

- 스케줄러: DAG 토폴로지에 따라 노드 실행 순서 결정
- 런타임: 고루틴 기반 노드 병렬 실행
- 라우터: 노드 간 메시지 전달 및 분기 처리
- 백프레셔: Go 채널 버퍼 기반 흐름 제어

#### 3. Processing Nodes (처리 노드)

플로우 엔진 내에서 데이터를 변환, 필터링, 집계하는 개별 처리 단위이다.

- 내장 노드: 필터, 변환, 분기, 집계, 디버그
- 플러그인 노드: Go 또는 WASM 기반 커스텀 노드
- 노드 레지스트리: 사용 가능한 모든 노드 타입 관리

#### 4. Output Layer (출력 레이어)

처리된 데이터의 최종 목적지를 담당한다.

- 데이터베이스 저장 (SQLite/PostgreSQL)
- 외부 시스템 전송 (MQTT, HTTP, WebSocket, gRPC)
- 알림 발송 (이메일, Slack, 웹훅)
- 대시보드 실시간 스트림 (SSE/WebSocket)

### 구성 요소 간 통신

| 통신 경로 | 프로토콜 | 용도 |
|-----------|----------|------|
| CLI -> API | HTTP/gRPC (원격) | 원격 서버에 명령어 실행 |
| Web Dashboard -> API | HTTP/WebSocket (원격) | 원격 서버 UI 상호작용 + 실시간 업데이트 |
| API -> Engine | 내부 함수 호출 | 플로우 제어 |
| Engine -> Nodes | Go 채널 | 노드 간 메시지 전달 |
| Engine -> Bridge Node -> Agent | 내부 인터페이스 | Bridge Node를 통한 Agent 참조 및 메시지 교환 |
| Agent -> Transport | Serial/TCP/UDP | Agent가 선택한 Transport Interface로 통신 |
| Transport -> External | MQTT/HTTP/WS/gRPC/NASA/MODBUS/Custom | 프로토콜 정의에 따른 외부 시스템 통신 |
| Engine -> Storage | 내부 인터페이스 | 상태 영속화 |
| Engine -> Plugin | Go Plugin API/WASM ABI | 플러그인 실행 |

---

## 주요 의존성 목록

### Go 백엔드 의존성

| 패키지 | 버전 | 용도 |
|--------|------|------|
| github.com/gofiber/fiber/v3 또는 github.com/labstack/echo/v4 | 최신 | HTTP 프레임워크 |
| github.com/spf13/cobra | v1.8+ | CLI 프레임워크 |
| github.com/spf13/viper | v1.18+ | 설정 관리 |
| github.com/fsnotify/fsnotify | v1.9+ | 파일 변경 감시 (설정 핫 리로드) |
| github.com/eclipse/paho.mqtt.golang | v1.4+ | MQTT 클라이언트 |
| github.com/gorilla/websocket | v1.5+ | WebSocket 통신 |
| google.golang.org/grpc | v1.60+ | gRPC 프레임워크 |
| google.golang.org/protobuf | v1.32+ | Protocol Buffers |
| github.com/tetratelabs/wazero | v1.6+ | WASM 런타임 |
| modernc.org/sqlite | 최신 | CGo-free SQLite 드라이버 |
| github.com/jackc/pgx/v5 | v5.5+ | PostgreSQL 드라이버 |
| github.com/redis/go-redis/v9 | v9.4+ | Redis 클라이언트 |
| github.com/golang-migrate/migrate/v4 | v4.17+ | DB 마이그레이션 |
| github.com/golang-jwt/jwt/v5 | v5.2+ | JWT 인증 |
| github.com/yuin/gopher-lua | v1.1+ | Lua 스크립트 엔진 (순수 Go) |
| go.bug.st/serial | v1.6+ | 시리얼 포트 통신 (Samsung NASA RS-485, Serial Agent) |
| gopkg.in/yaml.v3 | v3.0+ | YAML 직렬화/역직렬화 (Flow 정의 파일) |
| github.com/stretchr/testify | v1.9+ | 테스트 어설션 |
| github.com/prometheus/client_golang | v1.18+ | Prometheus 메트릭 |
| go.uber.org/zap 또는 log/slog | 최신 | 구조화된 로깅 |

### 프론트엔드 의존성

| 패키지 | 버전 | 용도 |
|--------|------|------|
| react | 19.x | UI 프레임워크 |
| react-dom | 19.x | DOM 렌더링 |
| typescript | 5.x | 타입 시스템 |
| @xyflow/react (React Flow) | 12.x+ | 노드 에디터 |
| tailwindcss | 4.x | CSS 프레임워크 |
| zustand | 5.x | 상태 관리 |
| @tanstack/react-query | 5.x | 서버 상태 관리 |
| axios 또는 ky | 최신 | HTTP 클라이언트 |
| react-router | 7.x | 라우팅 |
| lucide-react | 최신 | 아이콘 |
| recharts 또는 @tremor/react | 최신 | 차트/시각화 |
| react-window | 2.2.7 | 대용량 매트릭스 가상 스크롤 (TSDB/Store 데이터 뷰어 500행+) |
| @monaco-editor/react | 최신 | 웹 기반 Lua 코드 에디터 (구문 강조, 자동 완성) |
| js-yaml | 4.x | YAML 파싱 (Import/Export 기능) |
| vite | 6.x | 빌드 도구 |
| vitest | 3.x | 테스트 프레임워크 |
| @testing-library/react | 최신 | 컴포넌트 테스트 |
| eslint | 9.x | 코드 린팅 |
| prettier | 3.x | 코드 포맷팅 |

### 개발 도구 의존성

| 도구 | 버전 | 용도 |
|------|------|------|
| golangci-lint | v1.56+ | Go 정적 분석 |
| air | v1.50+ | Go 핫 리로딩 |
| buf | v1.28+ | Protobuf 관리 |
| goreleaser | v1.24+ | 릴리스 자동화 |
| docker | 24+ | 컨테이너 빌드 |
| docker-compose | v2+ | 개발 환경 |

---

## Wire 아키텍처 (연결선)

### 개요

Wire는 노드 간 메시지를 전달하는 연결선이다. Go 채널을 기반으로 구현되며, 바이패스와 버퍼링 두 가지 모드를 지원한다.

### 바이패스 모드 (기본)

- Go 채널 버퍼 크기 0 (unbuffered channel)
- 메시지가 즉시 수신 노드로 전달
- 수신 노드가 처리 중이면 송신 노드가 자연스럽게 대기 (Go 채널 특성에 의한 백프레셔)
- 가장 낮은 메모리 사용량, 가장 낮은 지연시간

### 버퍼 모드

- Go 채널 버퍼 크기를 N으로 설정 (buffered channel)
- 수신 노드가 Paused 상태이거나 처리 중일 때 버퍼에 메시지 축적
- 버퍼가 가득 차면 백프레셔 발동 (송신 노드 대기)
- 버퍼 크기는 Wire 설정에서 지정 (기본: 0 = 바이패스)

### 메시지 TTL (유효시간)

- 메시지 생성 시 TTL을 메타데이터에 설정 (선택적)
- Wire에서 메시지를 전달하기 전 TTL 만료 여부 확인
- 만료된 메시지 처리:
  1. Dead Letter 노드가 연결되어 있으면 만료 사유와 함께 전달
  2. 연결되어 있지 않으면 자동 폐기
  3. 관찰성 시스템에 만료 카운터 증가 및 로그 기록
- TTL 검사는 버퍼 모드에서만 의미 있음 (바이패스는 즉시 전달)

### 수신 노드 상태 확인

송신 노드는 Wire를 통해 수신 노드의 상태를 간접적으로 확인한다:

| 수신 노드 상태 | Wire 동작 (바이패스) | Wire 동작 (버퍼) |
|--------------|-------------------|----------------|
| Running | 즉시 전달 | 즉시 전달 또는 버퍼 |
| Paused | 송신 차단 (대기) | 버퍼에 저장, 가득 차면 대기 |
| Stopped | 에러 포트로 전달 | 에러 포트로 전달 |

### 성능 고려사항

- 바이패스 모드: Go 채널 네이티브 성능, 추가 오버헤드 없음
- 버퍼 모드: 버퍼 크기에 비례하는 메모리 사용, TTL 검사 오버헤드 최소화 (배치 처리)
- TTL 만료 검사: 별도 고루틴이 주기적으로 버퍼 내 만료 메시지 스캔 (설정 가능한 주기)

---

## System Agent 아키텍처 (내장 서비스)

### 개요

시스템 에이전트는 별도의 연결 설정 없이 노드나 다른 에이전트에서 바로 사용할 수 있는 내장 서비스이다. Transport/Protocol 설정이 불필요하며, 시스템 시작 시 자동으로 활성화된다.

### 접근 방식

시스템 에이전트는 두 가지 방식으로 접근 가능하다:

**직접 참조 (Bridge Node 불필요)**:
- 노드 코드에서 시스템 에이전트의 Go 인터페이스를 직접 호출
- Script 노드에서 Lua 바인딩을 통해 `xflow.event.emit("name", data)` 형태로 호출
- Plugin에서 Go 인터페이스 또는 WASM ABI를 통해 접근

**Bridge Node 경유 (선택적)**:
- 플로우 에디터에서 시스템 에이전트를 Bridge Node로 연결하여 시각적 구성
- 이벤트 구독, 타이머 트리거 등을 플로우 그래프에서 명시적으로 표현

### 제공 시스템 에이전트

| Agent | 구현 | 동시성 | 특이사항 |
|-------|------|--------|---------|
| Event | Go 채널 기반 Pub/Sub | 다중 구독자 동시 전달 | 버퍼링 설정 가능 |
| Logger | slog + observe 패키지 연동 | 잠금 없는 로깅 | 관찰성 시스템과 통합 |
| File | os + fsnotify | 파일 감시 고루틴 | 샌드박스 경로 제한 |
| Timer | time.Ticker + cron/v3 + time.AfterFunc | goroutine 기반 트리거 (atomic, RWMutex) | SPEC-TIMER-001 구현 완료. Timer 인터페이스(5개 메서드), TimerAgent(BaseLifecycle 임베딩), IntervalTimer(독립 goroutine), CronTimer(5/6필드), TimeoutTimer(단일 실행+자동 제거), BridgeHandler(메시지 디스패처). 186개 테스트, 86.5% 커버리지 |
| Store | sync.Map + StoreRepository (Write-Through) | 동시성 안전 (sync.Map, atomic.Bool) | SPEC-STORE-001 구현 완료. 3계층 합성(NamespacedStore->agentStore->VolatileStore), 이중 TTL(lazy+스캔), PersistentStore(JSON, 롤백), BridgeHandler(메시지 디스패처). 106개 테스트, 90.9% 커버리지 |

### 보안

- File Agent: 설정된 허용 디렉토리 내에서만 파일 접근 가능 (샌드박스)
- Store Agent: NamespacedStore 데코레이터 기반 "{namespace}:{key}" 격리, ForNamespace() 팩토리로 플로우별 독립 키 공간 생성, Paused 상태 시 쓰기 차단/읽기 허용, Stopped 상태 시 전체 차단
- Timer Agent: 최소 간격(100ms) 제한, 최대 타이머 수(1000) 제한, Paused 시 등록 거부/Cancel 허용, 핸들러 패닉 recover 보호, graceful shutdown(WaitGroup)

---

## Bridge Node 아키텍처 (Agent-Flow 연결)

### 개요

Bridge Node는 Agent와 Flow를 연결하는 전용 노드이다. Agent는 플로우와 독립적으로 실행되므로, Bridge Node가 두 시스템 간 메시지 교환을 중재한다.

### 연결 모드

**단방향 수신 (In)**:
- Agent → Bridge → Flow Nodes
- Agent가 외부에서 수신한 데이터를 플로우로 전달
- Bridge의 출력 포트에 연결된 노드가 데이터를 처리

**단방향 송신 (Out)**:
- Flow Nodes → Bridge → Agent
- 플로우에서 처리된 데이터를 Agent를 통해 외부로 전송
- Bridge의 입력 포트로 수신한 메시지를 Agent에게 전달

**양방향 (InOut)**:
- Agent ⇄ Bridge ⇄ Flow Nodes
- 수신과 송신을 하나의 Bridge Node에서 모두 처리
- 입력 포트와 출력 포트가 동시에 활성화

**요청/응답 (Request-Reply)**:
- Flow → Bridge → Agent → (외부 처리) → Bridge → 응답 노드
- Correlation ID를 메시지 메타데이터에 포함하여 요청-응답 매칭
- 응답 전달 대상: 요청을 보낸 노드(기본) 또는 설정에서 지정한 노드
- 타임아웃 설정으로 무한 대기 방지

### 구현 세부사항

**Agent 참조**:
- Bridge Node는 설정에서 Agent 이름/ID를 지정하여 바인딩
- 하나의 Agent에 여러 Bridge Node가 연결 가능 (멀티캐스트)
- Agent의 참조 카운팅에 포함되어 생명주기 관리

**메시지 변환**:
- Agent의 프로토콜 데이터 ↔ 플로우의 Message 구조체 변환
- Bridge Node에서 자동 매핑 또는 Lua 스크립트 기반 커스텀 변환 설정 가능

**요청/응답 Correlation**:
- 요청 시 고유 Correlation ID 생성 (UUID)
- Agent 응답에 Correlation ID 포함하여 매칭
- 대기 중인 요청은 sync.Map으로 관리, 타임아웃 시 에러 포트로 전달

---

## 에러 및 폐기 메시지 처리 (Error & Dead Letter)

### 에러 출력 포트

모든 노드는 기본 출력 포트와 별도로 에러 출력 포트를 가진다.

```
┌─────────────┐
│    Node     │──── output (정상 메시지)
│             │──── error  (에러 메시지)
└─────────────┘
```

- 에러 포트는 Go 채널로 구현, 기본 출력 채널과 동일한 인터페이스
- 에러 포트에 Catch 노드가 연결되지 않으면 채널을 생성하지 않음 (zero allocation)
- 에러 메시지에는 원본 Message, error 값, 발생 노드 ID가 래핑된 ErrorMessage 구조체 사용

### 전용 수신 노드

**Catch 노드**:
- 플로우 내 특정 노드 또는 전체 노드의 에러 출력을 수신
- 범위 설정: 특정 노드 지정 또는 와일드카드로 플로우 전체 에러 수신
- 수신한 에러 메시지를 다른 노드로 전달하여 에러 처리 로직 구성 가능

**Status 노드**:
- Agent 및 Node의 상태 전이 이벤트를 수신 (Running → Paused, Error 발생 등)
- 상태 변경에 반응하는 자동화 플로우 구성에 활용 (예: Agent 연결 끊김 시 알림 발송)

**Dead Letter 노드**:
- 백프레셔 드롭, 필터 제외, 처리 타임아웃 등으로 폐기된 메시지를 수신
- 폐기 원인(reason)과 폐기 시점 메타데이터 포함
- DB 저장, 알림, 재처리 큐 등 후속 처리 연결 가능

### 자동 폐기 정책

수신 노드가 연결되지 않은 경우:
- 에러 메시지: 관찰성 시스템에 에러 카운터 증가 + 에러 로그 기록 후 폐기
- 폐기 메시지: 관찰성 시스템에 드롭 카운터 증가 후 폐기
- 상태 이벤트: 관찰성 시스템에 상태 전이 로그 기록 후 폐기
- 모든 경우 Prometheus 메트릭에 반영되어 대시보드에서 추적 가능

---

## 메시지 아키텍처 (Message Architecture)

### 설계 원칙

메시지 시스템은 인터페이스 기반 설계를 채택한다. 모든 공개 API(Message, Payload, Metadata)는 Go 인터페이스로 정의되며, 구현체(defaultMessage, mapPayload, mapMetadata)는 unexported struct로 캡슐화한다. 이를 통해 외부 확장성을 보장하면서 내부 구현을 보호한다.

### 메시지 인터페이스

Agent와 Node 간 전달되는 메시지는 `Message` 인터페이스로 정의된다. 인터페이스는 ID(), Timestamp(), Payload(), Metadata(), History(), HistoryEnabled(), Clone() 메서드를 제공한다.

기본 구현체 `defaultMessage`는 unexported struct이며, `New(opts ...Option) Message` 팩토리 함수를 통해서만 생성할 수 있다. Options 패턴으로 WithHistory, WithMaxHistory, WithMetadata, WithPayload 옵션을 지원한다.

### Payload 인터페이스

Payload는 `Payload` 인터페이스로 정의된다. 기본 구현체 `mapPayload`는 `map[string]any` 기반의 가변 데이터 컨테이너이다. 모든 노드가 데이터를 자유롭게 추가, 변경, 삭제할 수 있다.

- **Add(key, value) error**: 새 키-값 쌍 추가 (키 존재 시 ErrKeyExists 반환)
- **Set(key, value)**: 기존 키의 값 교체 (없으면 추가, upsert)
- **Delete(key)**: 키 제거
- **Get(key) (any, bool)**: 값 조회
- **GetPath(jsonpath) (any, error)**: JSONPath 기반 중첩 데이터 접근 (예: `$.sensors[0].temperature`)
- **Keys() []string**: 모든 최상위 키 목록 반환
- **ToMap() map[string]any**: deep copy된 map 반환
- **ToJSON() ([]byte, error)**: JSON 직렬화
- **Clone() Payload**: deep copy 반환

deep copy는 수동 재귀 방식(deepCopyMap/deepCopyValue)으로 구현하여, JSON 라운드트립 대비 성능을 최적화했다.

동시성 안전: 메시지는 단일 고루틴에서만 처리되는 것이 기본이므로 뮤텍스 없이 동작한다. 분기(switch) 노드에서 다중 출력 시 메시지 복제(Clone)를 수행하여 데이터 레이스를 방지한다.

### Metadata 인터페이스

Metadata는 `Metadata` 인터페이스로 정의된다. 기본 구현체 `mapMetadata`는 `map[string]string` 기반이며, 값은 string 타입만 허용한다. Get/Set/Has/Remove/All/Clone 연산을 지원한다. 시스템 메타 키 상수(MetaKeySource, MetaKeyFlowID, MetaKeyNodeID, MetaKeyTTL, MetaKeyCorrelationID)가 정의되어 있다.

### 변경 이력 추적 (Decorator 패턴)

변경 이력은 선택적 기능으로, `WithHistory(true)` 옵션으로 Message 생성 시 활성화한다.

ChangeRecord 구조체는 다음 필드를 가진다:
- **Target**: 변경 대상 ("payload" 또는 "metadata")
- **Operation**: 연산 종류 ("add", "set", "delete")
- **Key**: 변경된 키
- **OldValue**: 이전 값 (add 시 nil)
- **NewValue**: 새 값 (delete 시 nil)
- **NodeID**: 변경을 수행한 노드 ID
- **Timestamp**: 변경 시각

Decorator 패턴으로 구현되어 있다. History 비활성화(기본) 시 Message는 Payload/Metadata를 직접 사용한다(제로 오버헤드). History 활성화 시 historyPayload가 Payload를 감싸고, historyMetadata가 Metadata를 감싸서 모든 변경 연산을 ChangeRecord로 기록한 후 원본에 위임한다.

**성능 고려사항**:
- 비활성화 시: Decorator 래퍼 없이 직접 동작 (zero overhead)
- 활성화 시: 변경 연산 호출 시 ChangeRecord 생성 후 원본에 위임
- 이력 크기 제한: FIFO 방식으로 최대 이력 수 제한 (기본: 100건, WithMaxHistory로 설정 가능)

**활용**:
- 디버깅: 메시지가 어느 노드에서 어떻게 변경되었는지 추적
- 감사: 데이터 변환 파이프라인의 처리 과정 검증
- Web Dashboard: 메시지 이력을 시각적으로 표시하여 데이터 흐름 디버깅

### JSON 직렬화

Message는 커스텀 MarshalJSON 메서드를 통해 JSON 직렬화를 지원한다. FromJSON 함수를 통해 JSON 데이터로부터 Message를 복원할 수 있다. 직렬화 시 id, timestamp, payload, metadata, history_enabled, history 필드가 포함된다.

---

## 런타임 생명주기 관리 (Runtime Lifecycle)

### 공통 상태 머신

모든 구성 요소(Flow, Node, Agent, Script Engine, Plugin)는 동일한 생명주기 상태 인터페이스를 구현한다.

```
Created → Initializing → Running ⇄ Paused → Stopping → Stopped
                            │                              │
                            └── Error ─── (자동 복구) ──────┘
```

**Go 인터페이스 설계** (`pkg/lifecycle/` 패키지로 구현 완료, SPEC-LIFE-001):
- `Lifecycle` 인터페이스: Init, Start, Pause, Resume, Stop, State 메서드 정의
- `Configurable` 인터페이스: Configure, GetConfig 메서드 정의 (런타임 설정 변경)
- `BaseLifecycle` 임베딩 구현체: sync.Mutex 기반 상태 머신, 콜백 메커니즘(Observer 패턴, 패닉 복구)
- `HealthChecker` 인터페이스 + `RecoveryPolicy`: 헬스 체크 및 지수 백오프 자동 복구 전략
- 각 구성 요소가 두 인터페이스를 구현하여 통합 관리 가능
- 상태 전이는 sync.Mutex 기반 동시성 안전 보장, 콜백은 락 해제 후 호출하여 데드락 방지

### 일시정지/재개 메커니즘

**Flow 일시정지**:
- 모든 하위 Node에 Pause 전파, Go 채널은 유지하여 데이터 유실 방지
- 채널 버퍼에 남은 메시지는 재개 시 순서 보장 처리

**Node 일시정지**:
- 입력 채널에서 읽기 중단, 내부 상태(집계 윈도우 등) 보존
- 재개 시 큐잉된 메시지부터 순차 처리

**Agent 일시정지**:
- Transport 연결은 유지, 수신 데이터를 내부 버퍼에 축적
- 재개 시 버퍼 데이터를 우선 처리하여 데이터 연속성 보장

### 런타임 설정 변경 (Hot Configuration)

**설계 원칙**:
- 모든 구성 요소의 설정을 재시작 없이 API를 통해 변경 가능
- 설정 변경은 Viper의 WatchConfig + 컴포넌트별 콜백 패턴으로 전파
- 변경 가능 설정과 불변 설정을 명확히 구분 (예: 포트 번호는 불변, 필터 조건은 가변)

**변경 가능한 설정 범위**:

| 구성 요소 | 런타임 변경 가능 | 재시작 필요 |
|-----------|----------------|------------|
| Flow | 백프레셔 임계값, 실행 정책 | 노드 구성 변경(추가/삭제) |
| Node | 필터 조건, 변환 규칙, 집계 윈도우 | 노드 타입 변경 |
| Agent | 인증 정보, 폴링 주기, 프로토콜 파라미터 | Transport 유형 변경 |
| Script | 스크립트 코드(핫 리로드), 타임아웃 | VM 풀 크기 |
| Plugin | 설정 파라미터 | 플러그인 바이너리 교체 |
| API Server | CORS 설정, 레이트 리밋 | 포트 번호, TLS 인증서 |

**설정 변경 API**:
- `PUT /api/v1/flows/{id}/config`: 플로우 설정 변경
- `PUT /api/v1/agents/{id}/config`: Agent 설정 변경
- `PUT /api/v1/nodes/{flowId}/{nodeId}/config`: 노드 설정 변경
- 설정 변경 이력은 감사 로그에 자동 기록

### 자동 복구 (Auto Recovery)

- 에러 상태 진입 시 설정된 재시도 정책에 따라 자동 복구 시도
- Agent 연결 끊김: 지수 백오프(exponential backoff) 기반 재연결
- Node 처리 오류: 설정된 재시도 횟수 초과 시 Error 상태로 전이
- 복구 이벤트는 관찰성 시스템에 자동 기록

---

## 관찰성 아키텍처 (Observability)

### 설계 원칙

시스템의 모든 구성 요소는 개별적으로 디버깅 가능해야 한다. 각 컴포넌트(Flow Engine, Agent, Node, Script Engine, Plugin, API Server)는 독립적인 로그 스트림, 로그 레벨, 메트릭을 갖는다.

### 컴포넌트별 로깅

**slog 기반 구조화된 로깅**:
- Go 표준 라이브러리 `log/slog`를 사용하여 구조화된 JSON/Text 로그 출력
- 각 구성 요소에 고유 로거 할당 (예: `slog.With("component", "agent.mqtt.client1")`)
- 로그 레벨: DEBUG, INFO, WARN, ERROR (slog 표준)

**컴포넌트별 로그 레벨 개별 설정**:
- 각 구성 요소의 로그 레벨을 독립적으로 설정 가능
- 기본 로그 레벨은 설정 파일에서 지정 (예: `observe.default_level: info`)
- 특정 컴포넌트만 DEBUG로 전환하여 해당 부분만 상세 추적

**런타임 로그 레벨 변경**:
- REST API를 통해 실행 중에 로그 레벨 변경: `PUT /api/v1/observe/level`
- 서버 재시작 없이 특정 컴포넌트의 로그 레벨을 즉시 변경
- Web Dashboard에서 GUI로 컴포넌트별 로그 레벨 조절 가능

**로그 스트림 분리**:
- 컴포넌트별 독립적인 로그 출력 채널
- 파일(컴포넌트별 분리 파일), stdout, WebSocket(실시간 스트리밍) 지원
- Web Dashboard에서 특정 컴포넌트의 로그만 필터링하여 실시간 조회

### 메트릭 수집

**Prometheus 기반 컴포넌트별 메트릭**:
- 각 구성 요소가 자체 메트릭을 `/metrics` 엔드포인트에 노출
- 컴포넌트 레이블로 구분: `xflow_component_messages_total{component="agent.mqtt"}`
- 표준 메트릭: 처리량(msg/s), 오류율, 지연시간(p50/p95/p99)

**Go expvar 통합**:
- 런타임 상태 정보 (고루틴 수, 메모리 사용량, GC 통계)
- 컴포넌트별 내부 상태 노출 (큐 길이, 연결 상태 등)

### 메시지 추적 (Tracing)

- 메시지가 플로우를 통과하는 전체 경로를 기록
- 각 노드에서의 처리 시간, 입력/출력 데이터 스냅샷
- 디버그 모드에서 특정 메시지의 전체 이동 경로를 시각화
- 성능 병목 지점 식별을 위한 노드별 지연시간 분석

### 구성 요소별 관찰 범위

| 구성 요소 | 로그 항목 | 주요 메트릭 |
|-----------|----------|------------|
| Flow Engine | 스케줄링 결정, 상태 전이, 에러 | 실행 플로우 수, 노드 처리 시간 |
| Agent | 연결/해제, 프로토콜 파싱, 에러 | 수신/송신 메시지 수, 연결 상태 |
| Node | 입/출력 메시지, 처리 결과 | 처리량, 오류율, 지연시간 |
| Script Engine | 스크립트 로드/실행, 에러, 핫 리로드 | VM 풀 사용률, 실행 시간 |
| Plugin | 로드/언로드, 실행 결과 | 플러그인 호출 횟수, 실행 시간 |
| API Server | 요청/응답, 인증, 에러 | 요청률, 응답 시간, 에러율 |

---

## 보안 아키텍처

### 인증 (Authentication)

- JWT 액세스 토큰 (15분 만료) + 리프레시 토큰 (7일 만료)
- API 키 인증 (서비스 간 통신, CLI 연동)
- OAuth2 소셜 로그인 (Google, GitHub)
- 토큰 블랙리스트 (Redis 기반)

### 인가 (Authorization)

- 역할 기반 접근 제어 (RBAC): Admin, Editor, Viewer
- 리소스 수준 권한 (플로우별, 프로젝트별)
- API 엔드포인트별 권한 검증 미들웨어

### 데이터 보안

- TLS/HTTPS 필수 (프로덕션 환경)
- 민감 설정값 암호화 저장 (AES-256-GCM)
- MQTT 통신 TLS 지원
- SQL 인젝션 방지 (파라미터 바인딩)
- XSS 방지 (입력 새니타이징)

### 감사 로깅

- 모든 인증 이벤트 로깅 (로그인, 로그아웃, 토큰 갱신)
- 플로우 변경 이력 추적 (생성, 수정, 삭제, 배포)
- API 요청 로깅 (사용자, 엔드포인트, 응답 코드)

---

## 성능 목표

| 지표 | 목표값 | 비고 |
|------|--------|------|
| API 응답 시간 (p95) | < 50ms | 일반 CRUD 엔드포인트 |
| 메시지 처리 지연 (p95) | < 10ms | 단일 노드 통과 시간 |
| 동시 플로우 실행 | 100+ | 단일 인스턴스 기준 |
| 동시 Agent 실행 | 50+ | 단일 인스턴스 기준 |
| 메시지 처리량 | 10,000+ msg/s | 단순 파이프라인 기준 |
| 서버 메모리 사용량 | < 256MB | 유휴 상태 기준 |
| 에이전트 메모리 사용량 | < 50MB | 경량 에이전트 기준 |
| Docker 이미지 크기 (xflowd) | < 100MB | 멀티스테이지 빌드 |
| Docker 이미지 크기 (에이전트) | < 30MB | Alpine 기반 |
| 서버 시작 시간 | < 1초 | 콜드 스타트 기준 |

---

*문서 버전: 1.5.0*
*최종 수정: 2026-03-11*
*작성: MoAI Documentation Manager*
