# Thingplus Gateway 노드 입출력 형식 참고

`thingplus-gateway` 에이전트(ThingsBoard Gateway MQTT `v1/gateway/*`)를 플로우에 연결하는
전용 노드 두 종의 입출력 메시지 형식을 정리한다.

- **thingplus-uplink** — 업링크(디바이스 → 플랫폼). 인입 메시지를 받아 텔레메트리/속성을
  게이트웨이로 발행하고, RPC 응답도 발행한다. 포트: `in` → `out` (processing).
- **thingplus-downlink** — 다운링크(플랫폼 → 플로우). 게이트웨이가 수신한 RPC/공유 속성을
  플로우 메시지로 방출한다. 포트: `out` (source).

관련 SPEC: SPEC-THINGPLUS-001. 에이전트 예제: `examples/agents/thingplus-gateway.yaml`,
플로우 예제: `examples/flows/thingplus-gateway.yaml`.

---

## 1. thingplus-uplink (업링크)

노드는 인입 메시지를 **전체 `message.Message`로 마샬**하여 에이전트 `Process()`에 전달한다.
에이전트는 메시지 `type` 또는 payload 형태로 **텔레메트리**인지 **RPC 응답**인지 판별한다.

### 1.1 입력 — 텔레메트리 / 클라이언트 속성

인입 payload는 디바이스 식별 필드(`device_name_path`, 기본 `$.device`)와 나머지 값들로 구성된다.
디바이스 키를 제외한 나머지가 텔레메트리 값(및 클라이언트 속성)으로 분리된다.

```json
{ "device": "Device A", "temperature": 42, "humidity": 60 }
```

- `device` (또는 설정된 `device_name_path` 경로): 디바이스 NAME (필수)
- 그 외 키: 텔레메트리 값

미등록 디바이스는 발행 전 `v1/gateway/connect`로 자동 connect된다.

**브로커 발행 결과** — `v1/gateway/telemetry`:

```json
{ "Device A": [ { "ts": 1751932800000, "values": { "temperature": 42, "humidity": 60 } } ] }
```

- `ts`: epoch milliseconds (`time.Time.UnixMilli()`, int64). 타임스탬프가 없으면 `ts`를
  생략하여 서버 시각을 사용한다.

클라이언트 속성이 포함된 경우 `v1/gateway/attributes`로 `{"Device A":{...}}` 형식으로 발행된다.

### 1.2 입력 — RPC 응답

메시지 `type`이 `thingplus.rpc.response`이거나, payload에 rpc `id`(또는 `rpc_id`)와 `data`가
함께 있으면 RPC 응답으로 처리된다.

```json
{ "device": "Device A", "id": 1, "data": { "success": true } }
```

- `device`: 디바이스 NAME (필수)
- `id` 또는 `rpc_id`: 원 RPC 요청 id (필수)
- `data`: 응답 본문. `data`가 없으면 `id`/`rpc_id`/`device`/`device_id`를 제외한 나머지 키가
  `data`로 사용된다.

**브로커 발행 결과** — `v1/gateway/rpc`:

```json
{ "device": "Device A", "id": 1, "data": { "success": true } }
```

> 판별 주의: `id`만 있고 `data`가 없으면 RPC 응답으로 인식되지 않고 텔레메트리로 처리된다.
> RPC 응답은 반드시 `device` + `id`(또는 `rpc_id`) + `data`를 포함해야 한다.

### 1.3 출력 (`out` 포트)

입력 메시지의 패스스루 클론에 발행 메타데이터를 부가하여 방출한다.

- `type`: `response`
- metadata: `node_id`, `agent:{type,id}` 그룹(기본 ON, `emit_agent: false`로 해제)

### 1.4 설정 필드

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `agent_ref` | agent_select | (필수) | `thingplus-gateway` 에이전트 참조 |
| `emit_agent` | boolean | `true` | metadata에 `agent` 그룹 포함 여부 |

발행 토픽은 에이전트가 내부적으로 결정하므로 노드에 토픽 설정이 없다.

---

## 2. thingplus-downlink (다운링크)

소스 노드로, 에이전트 다운링크 채널을 드레인하여 메시지를 방출한다. 에이전트는 `onConnect`에서
`v1/gateway/rpc`, `v1/gateway/attributes`를 자동 구독하므로 노드의 `topics`는 선택 사항이다.

방출 메시지는 에이전트가 조립한 `message.Message`를 `message.FromJSON`으로 복원하므로
**`type`이 보존**된다(범용 bridge 경로처럼 `event`로 덮어쓰지 않음).

### 2.1 출력 — RPC 요청

브로커 `v1/gateway/rpc` 수신 `{"device":"Device A","data":{"id":1,"method":"setValue","params":{"v":10}}}`을
파싱하여 방출한다.

- `type`: `thingplus.rpc.request`
- payload:

```json
{
  "device": "Device A",
  "device_id": "3f2a…",
  "id": 1,
  "method": "setValue",
  "params": { "v": 10 }
}
```

- metadata: `device`, `device_id`, `node_id`, `agent:{type,id}` 그룹

`device_id`는 NAME↔device_id 매핑(`ResolveDeviceID`)으로 역해석된다. 매핑이 없으면 NAME이
그대로 device_id로 사용된다(fallback).

### 2.2 출력 — 공유 속성 변경

브로커 `v1/gateway/attributes` 수신 `{"device":"Device A","data":{"fw":"1.0"}}`을 파싱하여 방출한다.

- `type`: `thingplus.attr.update`
- payload:

```json
{
  "device": "Device A",
  "device_id": "3f2a…",
  "data": { "fw": "1.0" }
}
```

- metadata: `device`, `device_id`, `node_id`, `agent:{type,id}` 그룹

### 2.3 출력 — raw fallback

`message.FromJSON` 복원에 실패한 원시 페이로드는 다음 형식으로 방출된다.

- `type`: `thingplus.raw`
- payload: `{ "_raw": <원본 바이트> }`

### 2.4 설정 필드

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `agent_ref` | agent_select | (필수) | `thingplus-gateway` 에이전트 참조 |
| `topics` | string_list | (선택) | 추가 구독 토픽. 비우면 에이전트 자동 구독에 의존 |
| `buffer_size` | number | `64` | 수신 버퍼 크기 |
| `emit_agent` | boolean | `true` | metadata에 `agent` 그룹 포함 여부 |

---

## 3. RPC 왕복 흐름

```
플랫폼 ──(v1/gateway/rpc)──▶ 게이트웨이 ──▶ thingplus-downlink
                                              │ type: thingplus.rpc.request
                                              │ payload: {device, device_id, id, method, params}
                                              ▼
                                        플로우 처리 (id 유지)
                                              │
                                              ▼  {device, id, data}
                          thingplus-uplink ──▶ 게이트웨이 ──(v1/gateway/rpc)──▶ 플랫폼
                          (type: thingplus.rpc.response 또는 id+data)
```

- 다운링크 방출 메시지의 `device`와 `id`를 응답 메시지에 그대로 실어 보내면, 업링크 노드가
  이를 RPC 응답으로 인식하여 `v1/gateway/rpc`로 발행한다.
- 응답 발행은 디바이스가 `connected` 상태일 때 수행된다(connect PUBACK 게이팅, A7).

---

## 4. 타입별 라우팅 예시

다운링크 방출 메시지는 `type`으로 라우팅한다(`examples/flows/thingplus-gateway.yaml` 참고).

```yaml
- name: "rpc-filter"
  type: "filter"
  config:
    condition: "$.type == 'thingplus.rpc.request'"
- name: "attr-filter"
  type: "filter"
  config:
    condition: "$.type == 'thingplus.attr.update'"
```

---

## 5. 참고

- **device_name_path**: 인입 메시지에서 디바이스 NAME을 추출하는 경로(에이전트 설정, 기본 `$.device`).
  다음 네 가지 형태를 지원한다:
  | 형태 | 예 | 해석 대상 |
  |------|-----|-----------|
  | 기본(payload) | `$.device` | payload의 해당 키 (하위호환 기본값) |
  | 명시적 payload | `$.payload.device` | payload의 해당 키 |
  | metadata 그룹 | `$.metadata.device.name` | metadata 그룹 `device`의 `name` 필드 |
  | flat metadata | `$.metadata.device_id` | metadata의 flat 키 |

  > metadata 경로(`$.metadata.*`)는 인입 메시지가 `message.Message`로 전달될 때만 해석된다
  > (thingplus-uplink 노드는 전체 메시지를 마샬하여 전달하므로 metadata가 보존된다). raw JSON
  > payload만 전달되는 경우 metadata 경로는 해석되지 않으므로 payload 경로를 사용한다.
- **타임스탬프 규약**: 텔레메트리 `ts`는 epoch milliseconds(int64). 부재 시 생략.
- **agent 메타 그룹**: 두 노드 모두 기본적으로 `agent:{type,id}` 그룹을 metadata에 부착한다
  (`emit_agent: false`로 해제).
- **무손실 업링크**: 브로커 연결이 끊긴 동안의 텔레메트리는 bounded 버퍼에 저장되며 재연결 시
  flush된다(silent drop 금지).
