# M1 실측 프로브 — 관측 기록

**목적**: OQ3 확정 — v3 의 `GROUP BY` **부분 지정**(전체 `*` 가 아닌)에서도 태그가 행 컬럼으로 드러나는가.

## 0. 환경

| 항목 | 값 |
|------|-----|
| 이미지 | `influxdb:3-core` (INFLUXDB_VERSION=3.11.2) |
| 컨테이너 | `moai-influx3`, 포트 8181 |
| 인증 | `--without-auth` — **재설치 시 변경**. 이전 컨테이너는 인증이 켜져 있었고 admin 토큰이 소실돼 접근 불가였다 |
| database | `probe` |
| 실행 | `influxdb3 serve --node-id node0 --object-store file --data-dir /var/lib/influxdb3 --without-auth` |
| 관측 시각 | 2026-08-23 |

> **재설치 사유.** 직전 컨테이너(2026-08-23T00:23:24 생성)는 SPEC-TSDB-003 M1 세션이 만들었고 admin 토큰을 발급했으나(00:23:42) 그 값을 어디에도 남기지 않았다. 카탈로그는 해시만 보관하므로 복구 불가였다. 같은 실수를 반복하지 않도록 로컬 프로브 컨테이너는 `--without-auth` 로 운용한다.

## 1. 주입 데이터 (M1.1)

태그 2종(`host` · `rack`) + **태그 결손 시리즈 1종**(`host=d` 는 `rack` 없음). 4시리즈 × 3버킷 = 12점.

```
cpu,host=a,rack=r1 usage=10.0  (+11.0, +12.0)
cpu,host=b,rack=r1 usage=20.0  (+21.0, +22.0)
cpu,host=c,rack=r2 usage=30.0  (+31.0, +32.0)
cpu,host=d        usage=40.0  (+41.0, +42.0)
```

버킷 시각: `14:11:00` · `14:12:00` · `14:13:00` (UTC). 질의 창 `2026-08-23T14:06:00Z` ~ `2026-08-23T14:21:00Z`.

## 2. P0 — 베이스라인 (GROUP BY 태그 없음, 현행 형태)

```sql
SELECT mean("usage") FROM "cpu" WHERE time >= '...' AND time < '...' GROUP BY time(60000ms) FILL(none)
```

```json
[{"iox::measurement":"cpu","time":"2026-08-23T14:11:00","mean":25.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:12:00","mean":26.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:13:00","mean":27.0}]
```

4시리즈가 한 줄로 접혀 평균 25.0 = (10+20+30+40)/4. 현행 동작 확인.

## 3. P1 — **OQ3 핵심**: 부분 지정 GROUP BY

```sql
... GROUP BY time(60000ms), "host" FILL(none)
```

```json
[{"iox::measurement":"cpu","time":"2026-08-23T14:11:00","host":"a","mean":10.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:12:00","host":"a","mean":11.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:13:00","host":"a","mean":12.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:11:00","host":"b","mean":20.0},
 ...]
```

**관측**: `host` 가 **각 행의 평탄한 컬럼**으로 드러난다. 컬럼 이름은 **태그 키 그대로**다.

## 4. P2 — 다중 키 + 태그 결손 (M1.4)

```sql
... GROUP BY time(60000ms), "host", "rack" FILL(none)
```

첫 버킷(`14:11:00`)의 전 행:

```
{'time': '2026-08-23T14:11:00', 'host': 'a', 'rack': 'r1', 'mean': 10.0}
{'time': '2026-08-23T14:11:00', 'host': 'b', 'rack': 'r1', 'mean': 20.0}
{'time': '2026-08-23T14:11:00', 'host': 'c', 'rack': 'r2', 'mean': 30.0}
{'time': '2026-08-23T14:11:00', 'host': 'd',                'mean': 40.0}
```

총 12행, 고유 시각 3개.

**관측 3건**:

1. **다중 키 정상** — `host`·`rack` 둘 다 평탄한 컬럼으로 나온다.
2. **태그 결손 시리즈는 해당 키가 행에서 아예 빠진다** — `null` 이나 빈 문자열이 아니라 **키 자체가 없다**. 구현의 `seriesRowGroupTags` 는 `row[k]` → `nil` → `seriesTagValueString(nil)` → `""` 로 처리하므로 **`{"host":"d","rack":""}` 인 별도 그룹**이 된다. spec.md 엣지 케이스("결손 시리즈는 그 키가 빈 값인 별도 그룹")와 일치한다.
3. **결손 시리즈가 유실되지 않는다** — 4시리즈 × 3버킷 = 12행 전량.

## 5. P3 — FILL(null) 과의 상호작용

```sql
... GROUP BY time(60000ms), "host" FILL(null)
```

```json
[{"iox::measurement":"cpu","time":"2026-08-23T14:06:00","host":"a"},
 {"iox::measurement":"cpu","time":"2026-08-23T14:07:00","host":"a"}, ...]
```

**관측**: 빈 버킷 행에는 `host` 는 있고 **`mean` 키가 아예 없다**(`"mean": null` 이 아니다). 구현의 `seriesRowValue` 는 컬럼 부재 시 `nil` 을 반환하므로 빈 버킷으로 정상 처리된다 — 기존 fill 경로와 같은 처분이다.

## 6. P4 — 존재하지 않는 태그 키 (UB1-4)

```sql
... GROUP BY time(60000ms), "nosuchtag" FILL(none)
```

```json
[{"iox::measurement":"cpu","time":"2026-08-23T14:11:00","mean":25.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:12:00","mean":26.0},
 {"iox::measurement":"cpu","time":"2026-08-23T14:13:00","mean":27.0}]
```

**관측**: **오류가 아니다.** 200 이 반환되고 그룹이 나뉘지 않은 결과가 온다(해당 컬럼 없음). 구현에서는 전 행이 `{"nosuchtag":""}` 인 그룹 1개로 접힌다 — spec.md 엣지 케이스("그룹 0개 또는 빈 태그 그룹 1개. 500 아님")와 일치하며 UB1-4 를 만족한다.

## 7. 버킷 경계 불변 (AC-08 런타임 확인)

| 질의 | 산출 시각 |
|------|-----------|
| P0 (group by 없음) | `14:11:00` · `14:12:00` · `14:13:00` |
| P1 (`, "host"`) | 동일 |
| P2 (`, "host", "rack"`) | 동일 |

**태그 키 추가가 버킷 경계를 바꾸지 않는다.** `GROUP BY` 첫 자리의 `time(d)` 가 경계를 결정하고 뒤따르는 태그 키는 분할 축일 뿐이라는 §2.4 의 전제가 실서버에서 확인되었다.

## 8. 결론 — OQ3 확정

**드러난다.** v3 의 부분 지정 `GROUP BY` 는 `GROUP BY *` 와 같이 태그를 행 컬럼으로 노출하며, 컬럼 이름은 태그 키 그대로다.

따라서 spec.md §6 가정 2 의 잠정이 확정되고, **대체 경로(`GROUP BY *` 로 질의 후 서버에서 접기)는 불필요하다.** M3 의 `normalizeSeriesBuckets` 가 방언 중립으로 작성된 것이 그대로 유효하다 — v2/v3 분기가 필요 없다.
