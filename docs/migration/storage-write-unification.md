# storage-write 통합 + Store measurement/field 모델 마이그레이션

`store-write` / `influxdb-write` 두 노드를 `storage-write` 하나로 통합하고, Store 의 식별
어휘를 `(key, metric_type, tags)` 에서 `(measurement, field, tags)` 로 바꾼 변경에 대한
운영자 마이그레이션 가이드.

두 변경은 **breaking** 이다. 클린 컷으로 진행했으므로 옛 노드 타입과 옛 config 키는
호환 별칭 없이 제거되었다.

## 왜 바꿨나

`store-write` 와 `influxdb-write` 는 같은 일(메시지를 스토리지에 기록)을 하면서 설정
어휘가 완전히 달랐다. 저장 대상을 바꾸려면 노드를 갈아끼우고 config 를 처음부터 다시
써야 했다. 통합 후에는 **설정을 그대로 둔 채 에이전트만 바꾸면** 저장 대상이 바뀐다.

어휘를 InfluxDB 모델(measurement + field + tags)로 맞춘 것도 같은 이유다. Store 의
`key`/`metric_type` 과 InfluxDB 의 `measurement`/`field` 는 같은 축을 다른 이름으로
부르고 있었다.

## 1. 노드 통합

### 1-1. 노드 타입

| 이전 | 이후 |
|------|------|
| `store-write` | `storage-write` |
| `influxdb-write` | `storage-write` |

백엔드는 `agent_ref` 가 가리키는 **에이전트 타입**으로 결정된다 — `store` 면 키-값
저장소, `influxdb` 면 시계열 DB 다. `tsdb-write` 는 이번 통합 범위가 아니며 그대로다.

### 1-2. config 어휘

기록 규약이 메시지 구조를 그대로 따르도록 바뀌었다. 경로를 일일이 매핑하지 않는다.

| 소스 | 용도 |
|------|------|
| `msg.payload` 키/값 | 측정값 (키 = 측정 종류, 값 = 값). 1개 이상 |
| `msg.metadata` | 태그 (그룹은 `device.id` 형태 평면 태그로 펼침) |
| `msg.timestamp` | 기록 시각 |

설정에서 고르는 것은 payload 키를 어떻게 배치할지(`payload_mode`)뿐이다.

```yaml
# 이전 (store-write)
type: "store-write"
config:
  key_template: "{location}:{device_id}"
  namespace: "sensors"
  metrics:
    - metric_type: "temperature"
      value_key: "$.payload.temperature"
      data_type: "float"
      min_interval: "30s"
      min_change: 0.5

# 이후 (storage-write)
type: "storage-write"
config:
  measurement: "{location}:{device_id}"
  namespace: "sensors"
  exclude_keys: ["location", "device_id"]
```

```yaml
# 이전 (influxdb-write)
type: "influxdb-write"
config:
  measurement: "temperature"
  tag_mappings: { location: "room" }
  field_mappings: { value: "temp_celsius" }

# 이후 (storage-write) — 태그는 metadata 에서 자동, 필드는 payload 에서 자동
type: "storage-write"
config:
  measurement: "temperature"
```

### 1-3. payload 키 처리 모드

| 모드 | 동작 |
|------|------|
| `fields`(기본) | payload 의 모든 키/값을 `measurement` 하나 아래 여러 필드로 기록 |
| `split` | payload 키마다 별도 measurement 로 분리 |

`split` 모드에서 값의 모양에 따라 필드가 정해진다:

```
{temperature: 23.5}                    → measurement=temperature, fields={value: 23.5}
{radio: {count: 2, rssi: -109}}        → measurement=radio, fields={count: 2, rssi: -109}
```

오브젝트를 통째로 `value` 에 넣으면 JSON 문자열로 뭉개져 집계·차트가 쓸 수 없으므로
오브젝트의 키를 필드로 펼친다.

### 1-4. 제거된 설정

다음은 통합 과정에서 제거되었다. 대체 수단을 함께 적는다.

| 제거된 설정 | 대체 |
|------|------|
| `values[]` / `metrics[]` 명시 목록 | payload 자동 순회 |
| `key_template` | `measurement` |
| `value_key` / `field_mappings` | payload 키가 곧 필드 |
| `tag_mappings` / `tags` | metadata 자동 |
| `key_mappings` | `payload_mode: split` |
| `timestamp_key` | `msg.timestamp` 자동 |
| `data_type` | 항상 `auto`(값 타입 추론) |
| `min_interval` / `min_change` / `min_change_percent` | 앞단 `deduplicate` / `aggregate` 노드 |
| `snapshot_value_path` / `snapshot_values` | 위 억제 기능 제거로 무의미 |

**미세변화 억제(dead-band)가 사라진 것이 실질적인 기능 손실이다.** 샘플링이 필요하면
`storage-write` 앞에 `deduplicate` 또는 `aggregate` 노드를 두어야 한다.

### 1-5. 백엔드 전용 설정

해당 없는 백엔드는 무시하므로, 두 백엔드용 값을 함께 두어도 오류가 나지 않는다.

| 설정 | 적용 백엔드 |
|------|------|
| `namespace`, `ttl` | store |
| `bool_to_int` | influxdb |

## 2. Store 어휘 변경

### 2-1. 이름 대응

| 이전 | 이후 |
|------|------|
| `SeriesID.Key` | `SeriesID.Measurement` |
| `SeriesID.MetricType` | `SeriesID.Field` |
| `metric_type` (JSON / API / 쿼리 파라미터) | `field` |
| `__metric__` (시리즈 라벨) | `__field__` |
| `ErrInvalidMetricType` | `ErrInvalidField` |

UI 라벨도 함께 바뀌었다 — 키 컬럼은 **Measurement**, 메트릭 컬럼은 **필드** 다.

### 2-2. 저장된 데이터는 그대로 읽힌다

시리즈 키 인코딩 순서를 바꾸지 않았다.

```
이전: metric_type | tagsEncoded | key
이후: field       | tagsEncoded | measurement
```

이름만 바뀌고 바이트 배치는 동일하므로 **기존에 저장된 시리즈가 그대로 읽힌다.**
예전 `metric=temperature, key=dev-1` 데이터는 새 모델에서 `field=temperature,
measurement=dev-1` 로 정확히 대응된다. 별도 데이터 이관 작업이 없다.

단사성 논증도 유지된다 — 제약된 문자집합 필드(`field`, `tags`)를 앞에, 임의 문자열
(`measurement`)을 뒤에 두는 배치가 그대로다.

## 3. 마이그레이션 절차

1. **플로우 정의 수정** — `store-write` / `influxdb-write` 노드를 `storage-write` 로
   바꾸고 config 를 위 대응표대로 다시 쓴다. 옛 노드 타입은 등록되어 있지 않으므로
   플로우 검증에서 거부된다.
2. **dead-band 대체 확인** — 억제 설정을 쓰던 플로우는 앞단 노드로 대체하거나, 기록량
   증가를 감수할지 판단한다.
3. **API 소비자 수정** — `metric_type` 을 읽거나 쓰던 외부 호출(쿼리 파라미터,
   `setmeta` 페이로드, 시리즈 라벨)을 `field` 로 바꾼다.
4. **재빌드·재기동** — 서버 바이너리와 웹 번들을 함께 올린다. 백엔드만 올리면 UI 가
   옛 스키마를 못 찾아 JSON 입력 폼으로 떨어지고, 웹만 올리면 팔레트에 옛 노드가
   남는다.

## 4. 영향 받지 않는 것

- **저장된 Store 데이터** — 인코딩 동일(§2-2).
- **`tsdb-write` / `tsdb-query`** — 이번 통합 범위가 아니다.
- **`store-read`** — 노드 타입과 조회 경로 모두 그대로다.
- **`influxdb-read` / `influxdb-query`** — 쓰기 노드만 통합했다.
