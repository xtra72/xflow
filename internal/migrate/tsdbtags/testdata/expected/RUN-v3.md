# tsdb-tags 마이그레이션 실행 가이드 (Phase C § C2)

본 디렉토리는 `xflowd migrate tsdb-tags` 가 생성한 InfluxDB backfill 스크립트와
운영자 실행 가이드를 포함합니다.

> Generated at: 2026-05-26T10:30:00Z
> Target: v3
> Bucket/Database: xflow
> Mappings: 3 composite -> uid

## 0. 안전 원칙

- **사전 백업 필수**: InfluxDB 백업 (snapshot, dump) 을 먼저 수행하세요.
- **staging 우선**: production 적용 전 staging 환경에서 동일 스크립트로 리허설.
- **모니터링**: 실행 중 Influx 의 부하 + 쿼리 결과 확인.
- **롤백 가능성**: 백업에서 복원 가능 — 운영자가 별도 검증 절차 보유.

## 1. 사전 점검 (Pre-flight)

다음을 확인하세요:

- [ ] InfluxDB 인스턴스 백업 완료 (스냅샷 / dump).
- [ ] staging 환경에서 동일 스크립트 dry-run 또는 실행 완료 후 검증.
- [ ] 클러스터 부하 여유 (peak time 회피).
- [ ] device_ids.json 매핑이 최신 (생성 시점과 production 시점이 일치).
- [ ] 본 디렉토리의 스크립트를 `diff` / 코드 리뷰로 검토.

## (운영자 사전 검토 체크리스트 끝)

## 2. 실행 절차

### v3 (SQL / InfluxQL) 실행

InfluxDB v3 는 tag UPDATE 를 지원하지 않으므로 **line-protocol re-write** 가 필요합니다:

```bash
# 1. SELECT 스크립트 검토
less migration-v3.sql

# 2. 영향 row 확인 (read-only)
influx3 query --database xflow --file migration-v3.sql

# 3. (운영자 자체) 각 row 를 line-protocol 로 재작성 — 다음 단계 필요:
#    a. SELECT 결과를 JSON / CSV 로 dump
#    b. 각 row 에 uid 태그 추가
#    c. influx3 write 로 다시 ingest (line-protocol)
#    예시 (Telegraf, Vector, 자체 스크립트):
#      influx3 write --database xflow --file rewritten.lp
```

> 본 도구는 SELECT 단계만 자동화합니다. line-protocol re-write 는 운영자가
> 환경에 맞는 도구 (Telegraf transform, custom script 등) 로 수행합니다.
> v3 native INSERT INTO ... SELECT 지원 여부는 운영 시점의 InfluxDB v3
> 버전 문서를 확인하세요.

## 3. 검증

실행 후 검증 (SQL):

```sql
-- uid 가 채워진 row 수 확인
SELECT COUNT(*) AS rows_with_uid
FROM <measurement>
WHERE uid IS NOT NULL;
```

## 4. 롤백

마이그레이션 결과가 만족스럽지 않으면:

1. xflowd 데몬 중지 (`systemctl stop xflowd` 등).
2. InfluxDB 사전 백업에서 데이터 복원.
3. xflowd 데몬 재시작.

## 5. 다음 단계 (C3 — Dual-tag 기간 운영)

C2 backfill 이 완료되면 xflowd 데몬은 새 데이터를 두 tag (id + uid) 모두로
기록해야 합니다. 이는 SPEC-DEVICE-IDENTITY-001 Phase C § C3 의 책임으로,
별도 PR 시리즈에서 진행됩니다.

## 부록: 본 디렉토리의 파일 구조

- migration-v3.sql
- RUN.md
