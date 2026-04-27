# xflow - 아키텍처 설계서

## 1. 개요

이 문서는 xflow의 아키텍처 패턴, 컴포넌트 상호작용, 데이터 흐름, 설계 결정을 기술한다. 프로덕트 정의(product.md), 프로젝트 구조(structure.md), 기술 스택(tech.md)과 중복되지 않도록 아키텍처적 관점에서의 심층 분석에 집중한다.

대상 독자: xflow 코어 개발자, 아키텍처 리뷰어, 플러그인/에이전트 개발자

---

## 2. 시스템 아키텍처

### 4계층 아키텍처

```
                   +-----------------------+
                   |    Client Layer       |
                   |  Web / CLI / API      |
                   +-----------+-----------+
                               |
                        REST / WebSocket
                               |
                   +-----------+-----------+
                   |    API Layer          |
                   |  Router + Middleware  |
                   |  Auth + Rate Limit   |
                   +-----------+-----------+
                               |
                     Internal Function Call
                               |
     +------------+------------+------------+
     |            |            |            |
+----+----+ +----+----+ +----+----+ +------+------+
|  Agent  | |  Flow   | |  Node   | |   Script    |
|  Layer  | | Engine  | | System  | |   Engine    |
|         | | (DAG    | | (처리   | | (Lua VM     |
| MQTT    | |  실행,   | |  단위)  | |  Pool)      |
| HTTP    | |  스케줄)  | |         | |             |
| MODBUS  | |         | |         | |             |
| NASA    | |         | |         | |             |
+---------+ +----+----+ +---------+ +-------------+
                 |
     +-----------+-----------+
     |  Infrastructure Layer |
     |  Storage / Cache      |
     |  Plugin Runtime       |
     +---+-------+-------+--+
         |       |       |
      SQLite  Postgres  Redis
```

### 계층 간 의존 방향

의존성은 반드시 상위에서 하위로만 흐른다. 하위 계층은 상위 계층을 알지 못하며, 이 규칙은 인터페이스를 통해 강제된다. 횡단 관심사인 observe(관찰성) 패키지만이 모든 internal 패키지에서 참조 가능한 예외이다.

패키지 의존성 흐름의 상세 다이어그램은 structure.md의 "패키지 의존성 흐름" 절을 참조한다.

---

## 3. 데이터 흐름 아키텍처

### 인바운드 데이터 흐름 (수집)

```
외부 디바이스                    xflow 내부
                                                      +-> Filter -> Transform -> DB 저장
+----------+   TCP/Serial   +---------+   +---------+ |
| PLC      | -------------> | Agent   |-->| Bridge  |-+-> Switch -> Aggregate -> MQTT 발행
| 센서     |                | (파싱)  |   | Node    | |
| 에어컨   |                +---------+   +---------+ +-> Script -> Debug
+----------+                                          |
                                                      +-> [에러] -> Catch Node -> 알림
                                                      +-> [폐기] -> DeadLetter -> 로깅
```

1. 외부 디바이스가 Transport(TCP/Serial/UDP)를 통해 데이터 전송
2. Agent가 Protocol Definition에 따라 바이트 스트림을 파싱하여 구조화된 데이터로 변환
3. Bridge Node가 Agent의 데이터를 Message 객체로 래핑하여 Flow 그래프에 주입
4. Wire(Go 채널)를 통해 노드 간 Message가 전달되며 DAG 순서로 처리
5. 각 노드의 에러는 에러 출력 포트를 통해 Catch/DeadLetter 노드로 분기

### 아웃바운드 데이터 흐름 (제어)

```
Flow 처리 결과                    외부 디바이스

+-----------+   +---------+   +---------+   TCP/Serial   +----------+
| 처리 노드  |-->| Bridge  |-->| Agent   | ------------> | PLC      |
| (명령 생성) |   | Node    |   | (직렬화) |               | 에어컨   |
+-----------+   | (Out)   |   +---------+                +----------+
                +---------+
```

### Request-Reply 흐름

```
+------+   요청    +--------+   명령   +---------+   TCP   +--------+
| Node |--------->| Bridge |-------->| Agent   |------->| 디바이스 |
|      |   (CID)  | (Req)  |  (CID)  | (직렬화) |        |        |
+------+          +--------+         +---------+        +--------+
   ^                  |                   ^                  |
   |    응답 (CID)    |    응답 (CID)     |    응답 바이트    |
   +------------------+-------------------+------------------+
```

CID = Correlation ID. 요청 시 UUID를 생성하여 메타데이터에 포함하고, 응답 수신 시 CID로 매칭하여 요청 노드에 응답을 전달한다. sync.Map으로 대기 중인 요청을 관리하며, 타임아웃 시 에러 포트로 전달한다.

---

## 4. 핵심 아키텍처 패턴

### 4.1 FBP (Flow-Based Programming)

xflow의 근간이 되는 패턴이다. 프로그램을 독립적인 프로세스(노드)들의 네트워크로 구성하며, 노드 간 데이터는 사전 정의된 연결(Wire)을 통해 전달된다.

- **노드는 블랙박스**: 각 노드는 입력 포트와 출력 포트만 노출하며, 내부 구현을 숨긴다
- **Wire는 Go 채널**: 노드 간 연결은 Go 채널로 구현하여 동시성과 백프레셔를 자연스럽게 확보
- **DAG 실행**: 스케줄러가 토폴로지 정렬로 실행 순서를 결정하되, 독립적인 노드는 병렬 실행

```
Port 구조:

  입력 포트(들)          Node          출력 포트(들)
       |              +------+             |
  in --+--> [처리] ---+--> out
       |              |      |             |
       |              +--+-+-+             |
                         | |
                    error  config
                    포트    포트
```

### 4.2 Bridge 패턴

Agent와 Flow의 생명주기가 독립적이기 때문에, 두 시스템을 연결하는 Bridge Node가 중재자 역할을 수행한다.

핵심 설계 결정:
- Agent는 여러 Flow에서 공유 가능하므로, Flow가 삭제되어도 Agent는 유지된다
- Bridge Node 생성 시 Agent의 참조 카운트를 증가시키고, 삭제 시 감소시킨다
- 참조 카운트가 0이 되면 Agent를 안전하게 종료한다
- Bridge Node 초기화 시 config 토픽을 자동 구독하여 런타임 제어 메시지를 처리한다

### 4.3 TypeOverlay 패턴

MODBUS 프로토콜에서 사용하는 비침습적 타입 해석 레이어이다. 원시 uint16 레지스터 저장소 위에 사용자가 정의한 데이터 타입(int16, float32, uint32 등)을 오버레이하여 해석한다.

```
물리 계층:    [uint16] [uint16] [uint16] [uint16] ...
                 |        |        |        |
TypeOverlay: [  float32  ] [    uint32    ]
             (2 레지스터)    (2 레지스터)
```

두 가지 적용 지점:
- **Bridge-Level TypeOverlay (Server: RegisterMap)**: MODBUS 서버의 레지스터 맵에서 외부 클라이언트가 읽기/쓰기 시 타입 변환을 적용
- **Cache-Level TypeOverlay (Client: RegisterCache)**: MODBUS 클라이언트의 폴링 캐시에서 레지스터 값을 읽을 때 타입 변환을 적용

공통 변환 유틸리티는 internal/modbus/ 패키지에서 제공하며, 빅 엔디안/리틀 엔디안 바이트 오더를 모두 지원한다.

### 4.4 Repository 패턴

저장소 추상화를 통해 구현체를 교체 가능하게 한다.

```
internal/storage/repository.go    <- 인터페이스 정의
internal/storage/sqlite.go        <- SQLite 구현 (개발용)
internal/storage/postgres.go      <- PostgreSQL 구현 (프로덕션)
```

모든 상위 패키지는 인터페이스에만 의존하므로, 설정에 따라 저장소를 런타임에 선택할 수 있다.

### 4.5 Observer 패턴

생명주기 상태 전이 시 등록된 콜백을 호출하는 패턴이다. pkg/lifecycle/ 패키지에서 구현한다.

- OnStateChange() 메서드로 콜백을 등록하며, UnsubscribeFunc을 반환하여 해제 가능
- 콜백 호출은 뮤텍스 락을 해제한 후 수행하여 데드락을 방지
- safeCallCallback으로 콜백 내 패닉을 recover하여 시스템 안정성을 보장

### 4.6 Decorator 패턴

Message의 변경 이력 추적에 적용한다. History 비활성화 시 제로 오버헤드를 보장한다.

```
비활성화(기본):  Message --> Payload (직접 접근)
활성화:         Message --> historyPayload --> Payload
                              |
                        ChangeRecord 기록
```

historyPayload/historyMetadata가 원본 Payload/Metadata를 감싸서, 모든 변경 연산을 ChangeRecord로 기록한 후 원본에 위임한다.

### 4.7 Factory 패턴

Agent와 Node의 생성을 레지스트리 기반 팩토리로 관리한다.

- internal/node/registry.go: 노드 타입 이름으로 인스턴스를 생성하는 팩토리
- internal/agent/registry.go: 에이전트 타입 이름으로 인스턴스를 생성하는 팩토리
- 플러그인이 새로운 타입을 런타임에 등록할 수 있다

### 4.8 Options 패턴

Go의 관용적 함수형 옵션 패턴을 전 시스템에 걸쳐 적용한다.

적용 지점:
- `message.New(WithHistory(true), WithMaxHistory(50))`
- `flow.NewFlow(WithName("my-flow"), WithDescription("..."))`
- `flow.NewWire(WithBufferSize(10))`
- `lifecycle.NewBase(WithName("agent-1"), WithOnStateChange(cb))`

이 패턴은 기본값을 유지하면서도 선택적으로 설정을 오버라이드할 수 있게 한다.

---

## 5. 동시성 모델

### Goroutine-per-Node 실행

Flow Engine은 각 노드를 독립 goroutine에서 실행한다. 노드 간 통신은 Go 채널(Wire)을 통해 수행되므로, 공유 메모리 없이 메시지 전달 방식으로 동시성을 달성한다.

```
Flow 실행 시:

goroutine-1: [Filter Node]  --chan--> goroutine-2: [Transform Node]
                                            |
                                           chan
                                            |
                                      goroutine-3: [Switch Node]
                                       /          \
                                     chan          chan
                                      |              |
                              goroutine-4:    goroutine-5:
                              [Aggregate]     [Bridge Node]
```

### 동기화 전략

| 리소스 | 동기화 도구 | 사용 위치 |
|--------|------------|-----------|
| 노드 간 메시지 전달 | Go 채널 (unbuffered/buffered) | Wire |
| 생명주기 상태 전이 | sync.Mutex | BaseLifecycle |
| Agent 레지스트리 | sync.Map | internal/agent/registry.go |
| Store 데이터 | sync.Map | system/store_volatile.go |
| TTL 매니저 플래그 | atomic.Bool | system/store_ttl.go |
| Timer 제어 | sync.RWMutex + atomic | system/timer.go |
| 메시지 분기(Switch) | Clone() 후 별도 채널 | 데이터 레이스 방지 |
| 설정 변경 콜백 | 스냅샷 복사 후 호출 | config.go, base.go |

### 백프레셔 메커니즘

Go 채널의 특성을 활용한 자연스러운 백프레셔 구현:

```
바이패스 모드 (unbuffered):
  송신 노드 --[chan]-- 수신 노드
  수신 노드가 읽지 않으면 송신 노드가 자동 대기

버퍼 모드 (buffered):
  송신 노드 --[chan buf=N]-- 수신 노드
  버퍼가 가득 차면 송신 노드가 대기
  버퍼 내 TTL 만료 메시지는 주기적 스캔으로 제거
```

---

## 6. 컴포넌트 상호작용

### Agent - Bridge - Flow 상호작용

```
+---------------------+         +-------------------+
|       Agent         |         |       Flow        |
|  +-----------+      |         |    +-----------+  |
|  | Transport |      |  참조   |    | Bridge    |  |
|  | (TCP/     |<-----|---------|----| Node      |  |
|  |  Serial)  |      |  카운팅  |    | (In/Out/  |  |
|  +-----------+      |         |    |  InOut)   |  |
|  | Protocol  |      |         |    +-----------+  |
|  | (파싱)    |      |         |         |         |
|  +-----------+      |         |    Wire(chan)      |
|  | Lifecycle |      |         |         |         |
|  | (독립)    |      |         |    +-----------+  |
|  +-----------+      |         |    | Node(s)   |  |
+---------------------+         |    +-----------+  |
                                +-------------------+
```

Agent와 Flow는 독립된 생명주기를 갖는다. Bridge Node가 Agent에 대한 참조를 유지하며, 양쪽의 상태 변화에 따라 메시지 전달 방식을 조정한다.

### Node - Wire - Node 상호작용

```
Node-A                Wire                 Node-B
  |                    |                     |
  |-- Process() -->    |                     |
  |   결과 생성         |                     |
  |-- 출력 포트 -->    [Go 채널에 전송]        |
  |                    |-- 수신 대기 해제 -->  |
  |                    |                     |-- Process() -->
  |                    |                     |   결과 생성
  |                    |                     |
```

각 노드의 Process() 메서드는 입력 메시지를 받아 처리 후 출력 포트로 전달한다. Wire는 Go 채널이므로, 수신 노드가 준비되지 않으면 송신 노드가 자연스럽게 대기한다.

### System Agent 접근 경로

```
접근 방식 1: 직접 참조 (Bridge 불필요)
+------------+    Go API / Lua 바인딩    +---------------+
| Node/Script|------------------------->| System Agent  |
| Plugin     |                          | (Store/Timer) |
+------------+                          +---------------+

접근 방식 2: Bridge 경유 (플로우 시각화 가능)
+------------+    Wire    +--------+    Agent API    +---------------+
| Node       |----------->| Bridge |---------------->| System Agent  |
+------------+            | Node   |                 | (Event/Timer) |
                          +--------+                 +---------------+
```

---

## 7. 생명주기 아키텍처

### 상태 머신

모든 컴포넌트가 공유하는 7-상태 머신이다. pkg/lifecycle/ 패키지에서 구현한다.

```
Created --> Initializing --> Running <--> Paused
                               |
                               v
                             Error --> (자동 복구) --> Running
                               |                       |
                               v                       v
                           Stopping --> Stopped
```

유효 전이 규칙:
- Created -> Initializing (Init 호출)
- Initializing -> Running (초기화 성공)
- Initializing -> Error (초기화 실패)
- Running -> Paused (Pause 호출)
- Paused -> Running (Resume 호출)
- Running -> Stopping (Stop 호출)
- Paused -> Stopping (Stop 호출)
- Running -> Error (런타임 오류)
- Error -> Stopping (Stop 호출 또는 복구 실패)
- Error -> Initializing (자동 복구)
- Stopping -> Stopped (종료 완료)

### Hot Configuration

```
설정 변경 흐름:

  API 요청                     Viper                 컴포넌트
  PUT /config -----> Config.Set() -----> OnChange 콜백 -----> 런타임 반영
                         |
                    Mutable 키 검증
                    (Immutable이면 거부)
```

Mutable/Immutable 키 레지스트리가 런타임 변경 가능 여부를 제어한다. 변경 가능 키 10개가 등록되어 있으며, 와일드카드 패턴(server.tls.*)으로 Immutable 범위를 지정한다.

파일 기반 Hot Reload: fsnotify로 설정 파일 변경을 감시하며, 100ms 디바운스를 적용하여 빈번한 이벤트를 병합한다.

### 자동 복구 (RecoveryPolicy)

```
Error 발생 --> RecoveryPolicy 확인
                   |
              +----+----+
              |         |
         재시도 가능  최대 횟수 초과
              |         |
         지수 백오프     |
         대기 후 Init   Stopping
              |
         성공 -> Running
         실패 -> 재시도 또는 Stopping
```

DefaultRecoveryPolicy: 지수 백오프(1초 시작, 최대 30초), 최대 5회 재시도, 초과 시 Stopped 전이.

### 참조 카운팅 (Agent 공유)

```
Flow-A 생성: Bridge(Agent-X) --> refCount++ (1)
Flow-B 생성: Bridge(Agent-X) --> refCount++ (2)
Flow-A 삭제: Bridge 해제     --> refCount-- (1)  Agent 유지
Flow-B 삭제: Bridge 해제     --> refCount-- (0)  Agent 종료
```

---

## 8. 메시지 아키텍처

### 인터페이스 기반 설계

```
공개 API (인터페이스)              비공개 구현체 (unexported)
+-----------------+              +-------------------+
| Message         |  <---------- | defaultMessage    |
| Payload         |  <---------- | mapPayload        |
| Metadata        |  <---------- | mapMetadata       |
+-----------------+              +-------------------+
                                        |
                                 Decorator 래퍼
                                 +-------------------+
                                 | historyPayload    |
                                 | historyMetadata   |
                                 +-------------------+
```

외부 플러그인은 인터페이스에만 의존하므로, 내부 구현을 변경해도 호환성이 유지된다.

### Deep Copy 전략

메시지 분기(Switch 노드 등)에서 데이터 레이스를 방지하기 위해 Clone()을 사용한다. deep copy는 수동 재귀(deepCopyMap/deepCopyValue)로 구현하여 JSON 라운드트립 대비 성능을 최적화했다.

```
Switch Node 분기 시:
  원본 Message --Clone()--> 복제 Message-A --> 경로 A
                --Clone()--> 복제 Message-B --> 경로 B
```

### JSONPath 지원

Payload의 중첩 데이터에 접근하기 위해 JSONPath 평가 엔진을 내장한다.

- dot-notation: `sensors.temperature`
- 배열 인덱스: `sensors[0].value`
- 와일드카드: `sensors[*].status`
- 파서: parsePath -> splitPathParts -> resolveTokens -> 값 반환

---

## 9. 설정 아키텍처

### 5단계 오버라이드 체인

우선순위가 높은 순서:

```
CLI 플래그 (--port 8080)
       |
환경변수 (XFLOW_SERVER_PORT=8080)
       |
사용자 지정 파일 (--config custom.yaml)
       |
기본 경로 파일 (~/.xflow/config.yaml, ./xflow.yaml)
       |
코드 기본값 (defaults.go)
```

상위 소스의 값이 하위 소스의 값을 덮어쓴다.

### 7개 설정 카테고리

```
+--Config 인터페이스----------------------------+
|                                               |
|  ServerConfig    EngineConfig   StorageConfig  |
|  AuthConfig      ObserveConfig  ScriptConfig   |
|  PluginConfig                                  |
|                                               |
|  + 6개 하위 구조체                              |
+-----------------------------------------------+
```

### Mutable/Immutable 구분

런타임 변경 가능한 키와 불가능한 키를 레지스트리로 관리한다. Set() 호출 시 Mutable 여부를 검증하고, Immutable 키에 대한 변경은 에러를 반환한다.

- Mutable 예시: observe.default_level, engine.backpressure_threshold
- Immutable 예시: server.port, server.tls.*, storage.type

### 유효성 검증

8개 검증기가 설정 로드 시 유효성을 검사한다: 포트 범위, 스토리지 타입, 로그 레벨, 양수값, 기간 문자열, TLS 파일 존재, 프로덕션 JWT 비밀키, PostgreSQL DSN 형식.

---

## 10. 관찰성 아키텍처

### 컴포넌트별 관찰성 모델

```
+------------------+     slog.With("component", ID)     +--------+
|  Flow Engine     |------------------------------------>|        |
|  agent.mqtt.c1   |------------------------------------>| Logger |
|  node.filter.n3  |------------------------------------>| Factory|
|  script.vm-pool  |------------------------------------>|        |
+------------------+                                    +--------+
         |                                                   |
    Prometheus                                          slog.Handler
    메트릭 등록                                              |
         |                                          +--------+--------+
         v                                          |        |        |
  /metrics 엔드포인트                               File   stdout  WebSocket
```

각 컴포넌트는 고유한 로거 인스턴스를 가지며, 로그 레벨을 독립적으로 제어한다. 런타임에 API를 통해 특정 컴포넌트의 로그 레벨만 변경할 수 있다.

### 메시지 추적

메시지가 플로우를 통과하는 전체 경로를 기록한다. 디버그 모드에서 각 노드에서의 처리 시간, 입력/출력 데이터 스냅샷을 수집하여 병목 지점을 식별한다.

관찰성 시스템의 기술 스택 상세(slog, Prometheus, expvar)는 tech.md를 참조한다.

---

## 11. 보안 아키텍처

### 인증 계층

```
+--클라이언트--+    +--API 미들웨어-----+    +--인증 모듈--+
|             |    |                   |    |            |
| JWT 토큰    |--->| 토큰 검증         |--->| jwt.go     |
| API 키      |--->| 키 검증           |--->| apikey.go  |
| OAuth2 코드 |--->| OAuth2 콜백      |--->| oauth.go   |
|             |    |                   |    |            |
+-------------+    +-------------------+    +------------+
```

### 인가 모델 (RBAC)

```
사용자 --> 역할(Admin/Editor/Viewer) --> 권한 매핑
              |                             |
              +-- 리소스 수준 권한 --+        |
                                   |        |
                              플로우별     API 엔드포인트별
                              권한 지정    권한 검증
```

### 샌드박스 보안

- Script Engine: Lua VM에서 위험 함수(os, io) 차단, 메모리/CPU 제한
- WASM Plugin: Wazero 샌드박스 환경에서 격리 실행
- File Agent: 설정된 허용 디렉토리 내에서만 파일 접근 가능
- Store Agent: 네임스페이스 기반 키 공간 격리

보안 프로토콜의 상세(JWT 만료 정책, TLS 설정, 감사 로깅)는 tech.md의 보안 아키텍처 절을 참조한다.

---

## 12. 배포 아키텍처

### 바이너리 구성

```
+---------------------+
| cmd/xflowd/         |  데몬 서버 (풀 기능)
| - API Server        |  - 모든 Agent/Node/Plugin 포함
| - Flow Engine       |  - 웹 대시보드 내장
| - Web Dashboard     |  - 메모리: ~256MB
+---------------------+

+---------------------+
| cmd/xflow/          |  CLI 클라이언트 (원격 제어)
| - REST API 호출     |  - xflowd에 원격 접속
| - 설정 관리         |  - 인증 토큰 관리
+---------------------+

+---------------------+
| cmd/xflow-agent/    |  경량 에이전트 (에지용)
| - 제한된 노드 세트   |  - 필터 + 변환 + 커넥터만 포함
| - 중앙 서버 연결     |  - 메모리: ~10-50MB
+---------------------+
```

### 컨테이너 배포

```
Docker 멀티스테이지 빌드:

Stage 1: Go 빌드         --> xflowd, xflow 바이너리
Stage 2: Node.js 빌드    --> 웹 대시보드 정적 파일
Stage 3: Alpine 최종      --> 바이너리 + 정적 파일 조합
                              결과: ~100MB 이미지

에이전트 이미지:
Alpine + xflow-agent 바이너리 --> ~20MB 이미지
```

### Kubernetes 배포

```
+-- Namespace: xflow --------------------------------+
|                                                     |
|  Deployment: xflowd (replicas: N)                  |
|    - ConfigMap: xflow-config                       |
|    - Secret: xflow-secrets                         |
|  Service: xflowd-svc (ClusterIP)                   |
|  Ingress: xflowd-ingress (TLS)                     |
|                                                     |
|  DaemonSet: xflow-agent (에지 노드에 배포)           |
|    - ConfigMap: agent-config                        |
|                                                     |
|  StatefulSet: PostgreSQL (프로덕션 스토리지)          |
|  Deployment: Redis (캐시/세션)                      |
|                                                     |
+-----------------------------------------------------+
```

---

## 13. 확장성 아키텍처

### 플러그인 시스템

```
+----플러그인 매니저----+
|                      |
|  발견 -> 로드 -> 초기화 -> 레지스트리 등록
|                      |
|  +-- Go Plugin --+   |   +-- WASM Plugin --+
|  | .so 파일 로드  |   |   | .wasm 파일 로드  |
|  | 네이티브 성능  |   |   | Wazero 런타임   |
|  | Go 언어만     |   |   | 다국어 지원     |
|  +--------------+   |   | (Rust/C/AS)     |
|                      |   +-----------------+
+----------------------+
         |
         v
  노드 레지스트리에 새 타입 등록
  -> 플로우에서 사용 가능
```

### 커스텀 노드 확장

Node 인터페이스를 구현하면 커스텀 노드를 만들 수 있다. 필수 메서드: Init, Process, Pause, Resume, Shutdown, Configure. 플러그인 매니저가 노드 레지스트리에 등록하면 플로우 에디터에서 사용 가능해진다.

### 커스텀 Agent 확장

Agent 인터페이스를 구현하고 Transport/Protocol을 설정하면 커스텀 Agent를 만들 수 있다. YAML로 프로토콜 구조를 정의하는 방식(Custom Protocol Agent)과 Go 코드로 직접 구현하는 방식(Samsung NASA Agent 참조) 두 가지를 지원한다.

---

## 14. 패키지 의존성

패키지 의존성 흐름 다이어그램은 structure.md의 "패키지 의존성 흐름" 절을 참조한다.

핵심 원칙 요약:
- cmd/ -> internal/ -> pkg/ 단방향 의존
- pkg/ 패키지는 외부 의존성 없는 순수 인터페이스/구조체
- internal/observe/는 횡단 관심사로 모든 internal/ 패키지에서 참조 가능
- 순환 의존성은 인터페이스를 통해 방지

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-28*
*작성: MoAI Documentation Manager*
