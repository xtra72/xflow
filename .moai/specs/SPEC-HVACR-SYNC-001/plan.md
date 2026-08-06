# Implementation Plan — SPEC-HVACR-SYNC-001

> HVACR 에이전트 미러링/동기화 (게이트웨이 ↔ 서버) over MQTT
> 관련: [spec.md](./spec.md), [acceptance.md](./acceptance.md)
> 개발 방법론: hybrid (신규 코드 TDD, 기존 경로 리팩터 DDD). 목표 커버리지 85%.

## 1. 기술 접근 (Technical Approach)

### 1.1 핵심 리팩터 — 디코드-메시지 ingress 추출 (behavior-preserving)

현 `receiveLoop`(`agent.go:2144`)는 로컬 `frameScanner`를 두고 인라인 블록(`agent.go:2200-2246`)에서 `scanner.Write → Next → Decode → handleMessage`를 직접 수행한다. `handleMessage(msg *NasaMessage)`(`agent.go:2251`)는 이미 "디코드 메시지 → 상태 반영"의 순수 진입점에 가깝다.

접근:
1. `handleMessage`를 재사용 진입점으로 승격/명명(`ingestDecodedMessage`). 필요 시 얇은 wrapper만 추가하고 시그니처는 보존.
2. 바이트 급전이 필요한 경로(설계 2a)를 위해 `ingestFrameBytes(frame []byte)`를 추가: `Decode` → 실패 시 기존 로그/카운팅 규약 재현 → 성공 시 `ingestDecodedMessage`.
3. `receiveLoop`의 인라인 블록을 `ingestFrameBytes` 호출로 치환. 관측 동작(디코드 성공/실패 카운트, unsupported 인덱스 로그 억제, `log_decode_errors` 옵션, `LogMessages` RX 로그)을 그대로 이관.

DDD 원칙: 리팩터 전 characterization 테스트로 기존 serial/tcp 경로 동작을 고정(PRESERVE)한 뒤 추출(IMPROVE).

### 1.2 서버측 mirror 입력 — 설계 결정 지점

두 후보(spec.md §REQ-SYNC-001-02-01):
- **(2a) 채널 급전형 `NasaTransport` (`transport_type: "mirror"`)** — 와이어 포맷을 `raw` 프레임 hex로 정의하고, MQTT 콜백이 프레임 바이트를 내부 채널에 push, `Receive`가 반환. `receiveLoop`/재연결/통계 무변경 재사용. `Available()`은 MQTT 연결 상태.
- **(2b) 직접 ingress 급전** — 와이어 포맷을 구조화 JSON(`sets` 배열)으로 정의하고, MQTT 콜백이 `*NasaMessage`로 역직렬화 후 `ingestDecodedMessage` 직접 호출. Decode 재수행 생략.

**권장 1차안**: (2a) + raw-hex 와이어 포맷. 이유 — `receiveLoop`/재연결/`Available` 계약/통계를 무변경 재사용하고, `mockTransport`(`agent_test.go:156`) 선례가 채널 급전형 transport의 테스트 가능성을 입증한다. 구조화 JSON(2b)은 사람이 읽기 쉽고 프로토콜 중립적이라 후속 lg 확장 시 이점이 있으므로, 와이어 포맷은 `raw` 필드와 `sets` 필드를 모두 수용하는 형태(둘 중 하나 존재)로 정의해 향후 전환 여지를 남긴다.

이 결정은 M2 착수 전 확정하고 acceptance.md의 라운드트립 테스트가 채택안을 검증한다.

### 1.3 업링크 tap

`ingestDecodedMessage` 진입 시(로컬 상태 반영과 병행) 디코드 메시지를 와이어 포맷으로 직렬화해 `MessagePublisher.PublishMessage`로 발행. 발행은 짧은 버퍼 채널 + 별도 goroutine으로 비동기화하여 수신 루프를 차단하지 않음. 발행 실패는 카운터+로그, 재연결은 paho에 위임.

락 주의: tap 직렬화는 `handleMessage`의 `a.mu.Lock()` 밖에서 수행하거나, 락 하에서는 `a.Name()` 호출 금지 규약을 준수(필드 직접 읽기).

### 1.4 제어 역경로

- 서버: `Process`의 제어 case에서 로컬 실행 대신 downlink 발행. mirror 모드 플래그로 분기.
- 게이트웨이: downlink 토픽 구독 콜백 → 와이어 포맷을 `Process` 입력 JSON으로 매핑 → `Process` 호출.
- ack(선택): 게이트웨이가 실행 결과를 `.../up/ack`로 발행, 서버가 `req_id`로 상관.

### 1.5 재사용 우선 원칙 (simplicity ladder)

- MQTT 발행/구독/재연결: `MQTTAgent`/`internal/node/mqtt.go` 재사용 (신규 MQTT 스택 금지).
- 제어 dispatch: `agent.go:463` `Process` 재사용.
- resync: `request_state`/`get_all` dispatch(`agent.go:502-524`) 재사용.
- 트랜스포트 계약: `NasaTransport` 인터페이스 재사용.

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

### M1 — Ingress 추출 (Primary Goal, Priority High)
- characterization 테스트로 기존 serial/tcp 경로 고정.
- `ingestDecodedMessage`/`ingestFrameBytes` 추출, `receiveLoop` 치환.
- 완료 조건: 기존 테스트 전부 green + `-race` 통과, 관측 동작 불변.

### M2 — 와이어 포맷 + 라운드트립 (Primary Goal, Priority High)
- 와이어 포맷 (역)직렬화 구현 (§1.2 채택안).
- 라운드트립 테스트: `NasaMessage` → wire → `NasaMessage` → `ingestDecodedMessage` 동일 상태.
- 완료 조건: 라운드트립 property 테스트 green.

### M3 — 서버 mirror 입력 (Secondary Goal, Priority High)
- mirror transport(2a) 또는 ingress 급전(2b) 구현.
- `config.go` transport switch + `NewNasaTransport` 분기 + mirror 설정 파싱.
- `cmd/xflowd/main.go` 배선.
- 완료 조건: mock MQTT로 업링크 주입 시 서버 상태 수렴 테스트 green.

### M4 — 게이트웨이 업링크 tap (Secondary Goal, Priority Medium)
- tap 발행 경로 + 설정(`mirror_uplink_enabled`/브로커/토픽/QoS).
- CRC/디코드 실패 미전송, 발행 실패 격리.
- 완료 조건: 디코드 성공 프레임만 발행됨을 확인하는 테스트 green.

### M5 — 제어 역경로 (Secondary Goal, Priority Medium)
- 서버 downlink 발행 + 게이트웨이 downlink 구독→Process.
- (선택) ack 상관.
- 완료 조건: 서버 제어 → downlink → 게이트웨이 Process 호출 검증 테스트 green.

### M6 — 동기화 의미론 + 재연결 (Final Goal, Priority Medium)
- retain 스냅샷(7a) 또는 resync(7b) 채택·구현.
- QoS/retain 정책 적용, MQTT 재연결 후 재동기화.
- online/offline·discovery replay 검증.
- 완료 조건: 서버 재시작 시나리오 재동기화 테스트 green.

### Mx (Optional Goal) — lg 일반화 note
- 본 SPEC 범위 외. ingress/tap/와이어 포맷의 프로토콜 중립성 회고 후 후속 SPEC 발의.

## 3. 아키텍처 설계 방향

- **역할 분기는 설정 기반**: 동일 `Hvacr01Agent` 코드가 게이트웨이/서버/단독 3역할을 설정으로 분기. mirror 모드 플래그 + tap 활성 플래그.
- **입력 소스 격리**: mirror 입력과 로컬 트랜스포트는 상호 배타(한 인스턴스는 하나의 입력 소스). frameScanner는 소스별 분리.
- **토픽이 곧 식별자**: gateway_id는 토픽에만, payload는 중립.
- **방향 격리**: `.../up/*` vs `.../down/control` 분리로 루프백 원천 차단.

## 4. 위험 및 대응

| 위험 | 영향 | 대응 |
| --- | --- | --- |
| RWMutex 재귀 deadlock (락 하 `a.Name()`) | 미러 경로 교착 | REQ-SYNC-001-01-03 규약, `-race` 테스트, 코드리뷰 체크 |
| ingress 추출이 기존 동작 변경 | serial/tcp 회귀 | characterization 테스트 선행(DDD PRESERVE) |
| online/offline 전이 순서 손실 | 서버 상태 불일치 | QoS 1, 순서 보존(단일 게이트웨이 단일 토픽), resync fallback |
| 업링크 발행이 수신 루프 차단 | 실시간성 저하 | 비동기 발행(버퍼 채널+goroutine), 발행 실패 격리 |
| 와이어 포맷 lossy | replay 불일치 | 라운드트립 property 테스트(M2 필수 게이트) |
| 재시작 직후 상태 공백 | 서버 일시 미동기 | retain 스냅샷(7a) 또는 resync(7b) |
| 제어 retain 재적용 부작용 | 의도치 않은 제어 | 다운링크 retain=false 강제 |

## 5. 품질 게이트 (TRUST 5)

- **Tested**: 85% 커버리지, ingress 라운드트립·mirror 수렴·downlink 실행 테스트, `-race`.
- **Readable**: Korean 주석(프로젝트 `code_comments: ko`), 기존 samsung 패키지 네이밍 준수.
- **Unified**: gofmt/기존 스타일, 기존 config/transport 파싱 패턴 재사용.
- **Secured**: MQTT 입력 검증(역직렬화 실패 안전 처리), 제어 명령 파라미터 검증(기존 `Process` 검증 재사용).
- **Trackable**: Conventional commits(`feat(SPEC-HVACR-SYNC-001): ...`), REQ ID 참조.

## 6. 라이브러리 버전

신규 외부 의존성 없음. 기존 `github.com/eclipse/paho.mqtt.golang`(MQTTAgent), `go.bug.st/serial`(기존 트랜스포트) 재사용. 상세 버전 확정은 `/moai:2-run` 단계에서 수행.
