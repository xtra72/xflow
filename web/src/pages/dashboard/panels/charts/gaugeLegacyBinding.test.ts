// 게이지 값 소스 판정 + 레거시 → store_source 이관 변환의 순수 단위 테스트.
//
// SPEC-CHART-002 M5.6 / AC-19 · AC-20.
//
// 이 파일은 plan.md M5 의 **판정 진리표 7행을 전수**로 잠근다. 게이지의 값 해석
// 경로는 `pickChartEmitterSource` · `pickStoreSource` · `useStoreLatestValue` ·
// `chartLiveValue` · `liveValue` · `hasBinding` · `parsedBase` 7지점에 분산되어
// 있어(plan.md §4 위험 표) 분기 조건을 컴포넌트 안에 인라인으로 두면 조용한 회귀를
// 만든다. 판정을 순수 함수로 뽑아 여기서 전수 검증한다.
//
// 진리표의 "레거시 `dataSources`" 열은 **결과를 바꾸지 않는다** — 7행 중 어느
// 쌍에서도 있음/없음이 선택 경로를 뒤집지 않는다. 그 사실 자체가 계약이므로 각 행을
// 두 변형으로 모두 돌린다.

import { describe, it, expect } from 'vitest';

import type { StoreSourceConfig, TsdbSourceConfig } from './chartChannelTypes';
import { DEFAULT_STORE_SOURCE_WINDOW } from './chartChannelTypes';
import {
  buildGaugeStoreMigrationPatch,
  findMigratableGaugeStoreBinding,
  gaugeValueSourceFlags,
  isGaugeStoreSourceActive,
  resolveGaugeMigrationState,
  resolveGaugeValueSource,
  type GaugeLegacyDataSource,
} from './gaugeLegacyBinding';
// SPEC-TSDB-002 M2 — M1 이 신설한 소스 판정 정본. 게이지의 활성 항이 이것과 동치임을
// 잠그기 위해서만 사용한다(프로덕션 코드는 아직 이 모듈을 호출하지 않는다).
import { isStoreSourceActive, resolvePanelSourceBinding } from './panelDataSource';

/** 활성 store_source(keys 모드, 시리즈 1개). */
function activeKeysStore(): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'keys',
    series: [{ key: 'k.room1' }],
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
  };
}

/** 활성 store_source(tag 모드, 태그 필터 1개). */
function activeTagStore(): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'tag',
    tag_filters: { room: '1' },
    series: [],
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
  };
}

/** 비활성 store_source — keys 모드인데 시리즈가 0개. */
function inactiveKeysStore(): StoreSourceConfig {
  return { ...activeKeysStore(), series: [] };
}

/** 비활성 store_source — tag 모드인데 태그 필터가 0개. */
function inactiveTagStore(): StoreSourceConfig {
  return { ...activeTagStore(), tag_filters: {} };
}

/** 활성 tsdb_source — 에이전트와 시리즈가 둘 다 있어야 활성이다. */
function activeTsdb(): TsdbSourceConfig {
  return {
    backend: 'influxdb',
    agent_name: 'influx-1',
    bucket: 'metrics',
    series: [{ key: 'room1', field: 'temp' }],
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
  };
}

/** 비활성 tsdb_source — 시리즈가 0개. */
function inactiveTsdb(): TsdbSourceConfig {
  return { ...activeTsdb(), series: [] };
}

/** 비활성 tsdb_source — 시리즈는 있으나 에이전트가 비어 있다. */
function agentlessTsdb(): TsdbSourceConfig {
  return { ...activeTsdb(), agent_name: '' };
}

/** F2 의 store 레거시 바인딩 항목(acceptance.md 공통 픽스처). */
const F2_STORE_BINDING: GaugeLegacyDataSource = {
  sourceType: 'store',
  storeAgentId: 'a1',
  storeAgent: 'store-a',
  storeKey: 'k1',
  storeNamespace: 'default',
};

/** 레거시 바인딩 배열(있음). */
function legacyPresent(): GaugeLegacyDataSource[] {
  return [{ ...F2_STORE_BINDING }];
}

/**
 * 진리표 1행을 두 변형(레거시 있음/없음)으로 모두 확인한다.
 *
 * 진리표의 마지막 열이 결과에 영향을 주지 않는다는 것 자체가 계약이므로, 행마다
 * `dataSources` 를 넣은 config 와 뺀 config 를 모두 통과시킨다.
 */
function resolveFromConfig(config: Record<string, unknown>) {
  return resolveGaugeValueSource(gaugeValueSourceFlags(config));
}

describe('isGaugeStoreSourceActive — store_source 활성 판정 (기존 패널과 동일 규칙)', () => {
  it('store_source 자체가 없으면 비활성이다', () => {
    expect(isGaugeStoreSourceActive(undefined)).toBe(false);
  });

  it('keys 모드 + 시리즈 1개 이상이면 활성이다', () => {
    expect(isGaugeStoreSourceActive(activeKeysStore())).toBe(true);
  });

  it('keys 모드 + 시리즈 0개면 비활성이다', () => {
    expect(isGaugeStoreSourceActive(inactiveKeysStore())).toBe(false);
  });

  it('tag 모드 + 태그 필터 1개 이상이면 (시리즈가 0개여도) 활성이다', () => {
    expect(isGaugeStoreSourceActive(activeTagStore())).toBe(true);
  });

  it('tag 모드 + 태그 필터 0개면 비활성이다', () => {
    expect(isGaugeStoreSourceActive(inactiveTagStore())).toBe(false);
  });

  it('selection_mode 미지정 + 시리즈 1개 이상이면 활성이다(구 config 하위호환)', () => {
    const store = { ...activeKeysStore() };
    delete store.selection_mode;
    expect(isGaugeStoreSourceActive(store)).toBe(true);
  });
});

describe('판정 진리표', () => {
  // ---- 1행: data_source 미지정/'channel' + 레거시 있음 → legacy ----
  it('data_source 미지정 + 레거시 있음 → legacy', () => {
    expect(resolveFromConfig({ dataSources: legacyPresent() })).toBe('legacy');
  });

  it("data_source 'channel' + 레거시 있음 → legacy", () => {
    expect(
      resolveFromConfig({ data_source: 'channel', dataSources: legacyPresent() }),
    ).toBe('legacy');
  });

  it('data_source 미지정 + store_source·series_reduce 가 있어도 레거시다', () => {
    // 채널 모드에서 series_reduce 는 읽지 않는다(§2.10 [S2]).
    expect(
      resolveFromConfig({
        store_source: activeKeysStore(),
        series_reduce: 'max',
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  // ---- 2행: data_source 미지정/'channel' + 레거시 없음 → legacy(→ static) ----
  it('data_source 미지정 + 레거시 없음 → legacy', () => {
    expect(resolveFromConfig({})).toBe('legacy');
  });

  it("data_source 'channel' + 레거시 없음 → legacy", () => {
    expect(resolveFromConfig({ data_source: 'channel' })).toBe('legacy');
  });

  // ---- 3행: store + store_source 비활성(series 0) + series_reduce 있음 → legacy ----
  it('store + store_source 비활성(series 0) + 레거시 있음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: inactiveKeysStore(),
        series_reduce: 'avg',
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  it('store + store_source 비활성(tag_filters 0) + 레거시 있음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: inactiveTagStore(),
        series_reduce: 'avg',
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  it('store + store_source 자체가 없음 + 레거시 있음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        series_reduce: 'avg',
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  // ---- 4행: store + 비활성 + 레거시 없음 → legacy(→ static) ----
  it('store + store_source 비활성(series 0) + 레거시 없음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: inactiveKeysStore(),
        series_reduce: 'avg',
      }),
    ).toBe('legacy');
  });

  it('store + store_source 비활성(tag_filters 0) + 레거시 없음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: inactiveTagStore(),
        series_reduce: 'avg',
      }),
    ).toBe('legacy');
  });

  // ---- 5행: store + 활성 + series_reduce 부재 + 레거시 있음 → legacy (§2.9 [S1]) ----
  it('store + store_source 활성 + series_reduce 부재 + 레거시 있음 → legacy', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeKeysStore(),
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  it('store + store_source 활성 + series_reduce 부재 + 레거시 없음 → legacy', () => {
    // 5행의 레거시 없음 변형. 신규 경로가 스위치(series_reduce)를 못 켰으므로
    // 레거시 경로로 가고, 그 경로에 바인딩이 없으므로 static config.value 가 된다.
    expect(
      resolveFromConfig({ data_source: 'store', store_source: activeKeysStore() }),
    ).toBe('legacy');
  });

  // ---- 6행: store + 활성 + series_reduce 있음 + 레거시 있음 → store-source ----
  it('store + store_source 활성 + series_reduce 있음 + 레거시 있음 → store-source', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeKeysStore(),
        series_reduce: 'last',
        dataSources: legacyPresent(),
      }),
    ).toBe('store-source');
  });

  it('store + tag 모드 활성 + series_reduce 있음 + 레거시 있음 → store-source', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeTagStore(),
        series_reduce: 'last',
        dataSources: legacyPresent(),
      }),
    ).toBe('store-source');
  });

  // ---- 7행: store + 활성 + series_reduce 있음 + 레거시 없음 → store-source ----
  it('store + store_source 활성 + series_reduce 있음 + 레거시 없음 → store-source', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeKeysStore(),
        series_reduce: 'last',
      }),
    ).toBe('store-source');
  });

  it('7종 대표값 어느 것이어도 판정은 동일하다(값이 아니라 유무 스위치다)', () => {
    for (const fn of ['max', 'avg', 'min', 'last', 'sum', 'count', 'delta'] as const) {
      expect(
        resolveFromConfig({
          data_source: 'store',
          store_source: activeKeysStore(),
          series_reduce: fn,
        }),
      ).toBe('store-source');
    }
  });
});

describe('resolveGaugeValueSource — TSDB 축 (store 행과 같은 모양)', () => {
  it('tsdb + 활성 + series_reduce 있음 → store-source(공용 시리즈 경로)', () => {
    expect(
      resolveFromConfig({
        data_source: 'tsdb',
        tsdb_source: activeTsdb(),
        series_reduce: 'last',
      }),
    ).toBe('store-source');
  });

  it('tsdb + 활성인데 series_reduce 부재 → legacy', () => {
    // 게이지 고유의 논리곱은 소스 종류와 직교한다 — store 와 똑같이 레거시로 떨어진다.
    expect(
      resolveFromConfig({ data_source: 'tsdb', tsdb_source: activeTsdb() }),
    ).toBe('legacy');
  });

  it('tsdb 비활성(시리즈 0 / 에이전트 없음)은 series_reduce 가 있어도 legacy', () => {
    for (const tsdb_source of [undefined, inactiveTsdb(), agentlessTsdb()]) {
      expect(
        resolveFromConfig({ data_source: 'tsdb', tsdb_source, series_reduce: 'last' }),
      ).toBe('legacy');
    }
  });

  it('종류 축이 활성 항을 고른다 — tsdb 를 골랐는데 store 만 활성이면 legacy', () => {
    // 두 블록은 공존한다(소스를 오가며 설정이 남는다). 활성 판정을 하나로 합치면
    // 남아 있는 store 설정 때문에 TSDB 게이지가 값을 내는 것처럼 잘못 판정된다.
    expect(
      resolveFromConfig({
        data_source: 'tsdb',
        store_source: activeKeysStore(),
        tsdb_source: inactiveTsdb(),
        series_reduce: 'last',
      }),
    ).toBe('legacy');
    // 반대 방향도 같다.
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: inactiveKeysStore(),
        tsdb_source: activeTsdb(),
        series_reduce: 'last',
      }),
    ).toBe('legacy');
  });
});

describe('gaugeValueSourceFlags — config → 판정 입력 파생', () => {
  it('레거시 dataSources 는 판정 입력에 포함되지 않는다', () => {
    const withLegacy = gaugeValueSourceFlags({
      data_source: 'store',
      store_source: activeKeysStore(),
      series_reduce: 'last',
      dataSources: legacyPresent(),
    });
    const withoutLegacy = gaugeValueSourceFlags({
      data_source: 'store',
      store_source: activeKeysStore(),
      series_reduce: 'last',
    });
    expect(withLegacy).toEqual(withoutLegacy);
  });

  it('플래그 4종을 config 에서 그대로 파생한다', () => {
    expect(
      gaugeValueSourceFlags({
        data_source: 'store',
        store_source: activeKeysStore(),
        series_reduce: 'delta',
      }),
    ).toEqual({
      dataSource: 'store',
      storeSourceActive: true,
      tsdbSourceActive: false,
      hasSeriesReduce: true,
    });
  });

  it('data_source 미지정은 undefined 로 유지된다(채널 해석은 판정 함수가 한다)', () => {
    expect(gaugeValueSourceFlags({})).toEqual({
      dataSource: undefined,
      storeSourceActive: false,
      tsdbSourceActive: false,
      hasSeriesReduce: false,
    });
  });
});

describe('findMigratableGaugeStoreBinding — 이관 대상 탐색', () => {
  it('첫 번째 유효한 store 항목을 반환한다', () => {
    const second: GaugeLegacyDataSource = {
      sourceType: 'store',
      storeAgentId: 'a2',
      storeAgent: 'store-b',
      storeKey: 'k2',
    };
    const found = findMigratableGaugeStoreBinding({
      dataSources: [{ sourceType: 'resource', resource: 'cpu' }, F2_STORE_BINDING, second],
    });
    expect(found).toEqual(F2_STORE_BINDING);
  });

  it('storeKey 가 없는 store 항목은 건너뛴다', () => {
    const valid: GaugeLegacyDataSource = {
      sourceType: 'store',
      storeAgent: 'store-b',
      storeKey: 'k2',
    };
    const found = findMigratableGaugeStoreBinding({
      dataSources: [{ sourceType: 'store', storeAgent: 'store-a' }, valid],
    });
    expect(found).toEqual(valid);
  });

  it('에이전트(id/name) 가 모두 없는 store 항목은 건너뛴다', () => {
    expect(
      findMigratableGaugeStoreBinding({
        dataSources: [{ sourceType: 'store', storeKey: 'k1' }],
      }),
    ).toBeUndefined();
  });

  it('storeAgentId 만 있어도(구 config 반대 케이스) 유효하다', () => {
    const onlyId: GaugeLegacyDataSource = {
      sourceType: 'store',
      storeAgentId: 'a9',
      storeKey: 'k9',
    };
    expect(findMigratableGaugeStoreBinding({ dataSources: [onlyId] })).toEqual(onlyId);
  });

  it('resource / flow / chart-emitter 만 있으면 undefined 다(이관 불가)', () => {
    expect(
      findMigratableGaugeStoreBinding({
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' },
          { sourceType: 'flow', flowId: 'f1', dataField: 'x' },
          { sourceType: 'chart-emitter', channelName: 'ch1' },
        ],
      }),
    ).toBeUndefined();
  });

  it('dataSources 가 없거나 배열이 아니면 undefined 다', () => {
    expect(findMigratableGaugeStoreBinding({})).toBeUndefined();
    expect(findMigratableGaugeStoreBinding({ dataSources: 'nope' })).toBeUndefined();
    expect(findMigratableGaugeStoreBinding({ dataSources: [] })).toBeUndefined();
  });

  it('배열 안의 null 항목에서 터지지 않는다', () => {
    expect(
      findMigratableGaugeStoreBinding({ dataSources: [null, undefined, F2_STORE_BINDING] }),
    ).toEqual(F2_STORE_BINDING);
  });
});

describe('buildGaugeStoreMigrationPatch — 레거시 → store_source 변환 (§2.8 [E2])', () => {
  it('AC-20 이 명시한 patch 형상을 만든다', () => {
    expect(buildGaugeStoreMigrationPatch(F2_STORE_BINDING)).toEqual({
      data_source: 'store',
      series_reduce: 'last',
      store_source: {
        agent_id: 'a1',
        agent_name: 'store-a',
        namespace: 'default',
        selection_mode: 'keys',
        series: [{ key: 'k1' }],
        time_window_ms: 60 * 60 * 1000,
        interval_ms: 60 * 1000,
        aggregation: 'average',
        refresh_interval_ms: 5000,
      },
    });
  });

  it("namespace 미지정은 'default' 로 채운다", () => {
    const patch = buildGaugeStoreMigrationPatch({
      sourceType: 'store',
      storeAgentId: 'a1',
      storeAgent: 'store-a',
      storeKey: 'k1',
    });
    expect(patch.store_source.namespace).toBe('default');
  });

  it('storeAgentId 가 없는 구 config 는 agent_id 를 만들지 않는다', () => {
    const patch = buildGaugeStoreMigrationPatch({
      sourceType: 'store',
      storeAgent: 'store-a',
      storeKey: 'k1',
    });
    expect(patch.store_source.agent_id).toBeUndefined();
    expect(patch.store_source.agent_name).toBe('store-a');
  });

  it('storeAgent 이름이 없으면 agent_name 은 빈 문자열이다(id 로 해석된다)', () => {
    const patch = buildGaugeStoreMigrationPatch({
      sourceType: 'store',
      storeAgentId: 'a1',
      storeKey: 'k1',
    });
    expect(patch.store_source.agent_name).toBe('');
    expect(patch.store_source.agent_id).toBe('a1');
  });

  it("series_reduce 는 항상 'last' 다 — 레거시 mode:'latest' 폴링과 의미가 가장 가깝다", () => {
    expect(buildGaugeStoreMigrationPatch(F2_STORE_BINDING).series_reduce).toBe('last');
  });

  it('patch 는 dataSources 키를 포함하지 않는다(비파괴 보장의 구조적 근거)', () => {
    const patch = buildGaugeStoreMigrationPatch(F2_STORE_BINDING);
    expect(Object.keys(patch).sort()).toEqual([
      'data_source',
      'series_reduce',
      'store_source',
    ]);
  });

  it('이관 결과 patch 를 적용한 config 는 진리표상 store-source 로 판정된다', () => {
    // 이관 → 즉시 신규 경로가 이긴다(§4.5 "신규 경로가 값을 낼 수 있으면 신규가 이긴다").
    const before: Record<string, unknown> = { dataSources: legacyPresent(), value: 42 };
    const binding = findMigratableGaugeStoreBinding(before)!;
    const after = { ...before, ...buildGaugeStoreMigrationPatch(binding) };
    expect(resolveFromConfig(before)).toBe('legacy');
    expect(resolveFromConfig(after)).toBe('store-source');
  });

  it("이관 후 data_source 를 'channel' 로 되돌리면 다시 legacy 로 판정된다(롤백)", () => {
    const before: Record<string, unknown> = { dataSources: legacyPresent(), value: 42 };
    const binding = findMigratableGaugeStoreBinding(before)!;
    const migrated: Record<string, unknown> = {
      ...before,
      ...buildGaugeStoreMigrationPatch(binding),
    };
    const reverted: Record<string, unknown> = { ...migrated, data_source: 'channel' };
    expect(resolveFromConfig(reverted)).toBe('legacy');
    // 되돌린 config 에도 레거시 바인딩이 그대로 남아 있어야 복구가 성립한다.
    expect(reverted.dataSources).toEqual(legacyPresent());
  });
});

// ---------------------------------------------------------------------------
// M6 추가 (A) — 이관 액션 재실행 가드.
//
// 이관은 `config.dataSources` 를 보존하므로(§2.8 [E2] 4항) 이관 후에도 레거시 항목이
// 계속 존재한다. "레거시가 있는가" 만으로 활성 여부를 정하면 버튼이 영구히 활성이고,
// 이관 → store_source 손질 → 재클릭 시 사용자의 손질이 기본값에 덮어써진다. SPEC 의
// 이관 설계는 전부 비파괴이므로(UB1-3 · §4.5) 그 재실행을 막는 것이 계약이다.
// ---------------------------------------------------------------------------

describe('resolveGaugeMigrationState — 이관 액션 3상태 (§2.8 [E2])', () => {
  it('레거시 store 바인딩만 있으면 available 이다', () => {
    expect(resolveGaugeMigrationState({ dataSources: legacyPresent() })).toBe('available');
  });

  it('이관할 수 있는 store 항목이 없으면 unavailable 이다', () => {
    expect(
      resolveGaugeMigrationState({
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' },
          { sourceType: 'flow', flowId: 'f1', dataField: 'x' },
          { sourceType: 'chart-emitter', channelName: 'ch1' },
        ] satisfies GaugeLegacyDataSource[],
      }),
    ).toBe('unavailable');
    expect(resolveGaugeMigrationState({})).toBe('unavailable');
  });

  it('이관 직후에는 already-migrated 로 뒤집힌다(레거시가 보존되어 있어도)', () => {
    const before: Record<string, unknown> = { dataSources: legacyPresent(), value: 42 };
    expect(resolveGaugeMigrationState(before)).toBe('available');

    const binding = findMigratableGaugeStoreBinding(before)!;
    const after = { ...before, ...buildGaugeStoreMigrationPatch(binding) };

    // 레거시는 그대로 남아 있다 — 그런데도 상태는 available 이 아니어야 한다.
    expect(findMigratableGaugeStoreBinding(after)).toBeDefined();
    expect(resolveGaugeMigrationState(after)).toBe('already-migrated');
  });

  it('사용자가 store_source 를 손질한 뒤에도 already-migrated 를 유지한다(덮어쓰기 차단)', () => {
    const customized: Record<string, unknown> = {
      dataSources: legacyPresent(),
      data_source: 'store',
      series_reduce: 'max',
      store_source: {
        ...activeKeysStore(),
        series: [{ key: 'k.room1' }, { key: 'k.room2' }, { key: 'k.room3' }],
        time_window_ms: 24 * 60 * 60 * 1000,
      } satisfies StoreSourceConfig,
    };
    expect(resolveGaugeMigrationState(customized)).toBe('already-migrated');
  });

  it('series_reduce 를 지워 레거시로 폴백 중이어도 already-migrated 다', () => {
    // 이 상태는 값 소스로는 legacy 지만(§2.9 게이지 추가 조건), 보호해야 할 사용자
    // store_source 설정은 그대로 존재한다. 두 판정은 목적이 다르므로 갈린다.
    const config: Record<string, unknown> = {
      dataSources: legacyPresent(),
      data_source: 'store',
      store_source: activeKeysStore(),
    };
    expect(resolveFromConfig(config)).toBe('legacy');
    expect(resolveGaugeMigrationState(config)).toBe('already-migrated');
  });

  it('store 모드라도 store_source 가 비활성이면 available 로 남는다', () => {
    // 아직 시리즈를 하나도 고르지 않은 상태 — 덮어쓸 사용자 설정이 없으므로 이관을 막을
    // 이유가 없다.
    const config: Record<string, unknown> = {
      dataSources: legacyPresent(),
      data_source: 'store',
      store_source: inactiveKeysStore(),
    };
    expect(resolveGaugeMigrationState(config)).toBe('available');
  });
});

// ---------------------------------------------------------------------------
// M6 추가 (B) — `defaultStoreSource()` 드리프트 면적 폐쇄.
//
// 이관 기본값을 이 모듈이 복제하고 있었다. 값은 이제 `chartChannelTypes.ts` 의
// `DEFAULT_STORE_SOURCE_WINDOW` 한 곳에만 있으며, `ChartPanelSections.tsx` 의
// `defaultStoreSource()` 도 같은 상수를 쓴다(같은 항목은 그 파일의 테스트가 잠근다).
// ---------------------------------------------------------------------------

describe('이관 기본 조회 창은 공용 상수 단일 정본이다 (§2.8 [E2] 1항)', () => {
  it('patch 의 시간창/버킷/집계/폴링이 DEFAULT_STORE_SOURCE_WINDOW 와 같다', () => {
    const { store_source } = buildGaugeStoreMigrationPatch(F2_STORE_BINDING);
    expect({
      time_window_ms: store_source.time_window_ms,
      interval_ms: store_source.interval_ms,
      aggregation: store_source.aggregation,
      refresh_interval_ms: store_source.refresh_interval_ms,
    }).toEqual(DEFAULT_STORE_SOURCE_WINDOW);
  });

  it('공용 상수의 값은 spec 이 적은 기본값 그대로다', () => {
    // 상수를 통해 간접 비교하면 상수 자체가 바뀌었을 때 두 테스트가 함께 통과해 버린다.
    // 그래서 값 자체를 여기서 한 번 못박는다.
    expect(DEFAULT_STORE_SOURCE_WINDOW).toEqual({
      time_window_ms: 60 * 60 * 1000,
      interval_ms: 60 * 1000,
      aggregation: 'average',
      refresh_interval_ms: 5000,
    });
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `gaugeLegacyBinding.ts:122` 는 다른 지점과 달리 소스 활성 판정에
// **`hasSeriesReduce` 논리곱이 결합**되어 있다. 그 항은 소스 활성이 아니라 게이지
// 고유의 레거시 우선순위 규칙이며 SPEC-CHART-002 §2.9 가 소유한다.
//
// M3 은 `flags.storeSourceActive` 항만 `resolvePanelSourceBinding(config).active`
// 로 치환하고 `hasSeriesReduce` 논리곱은 **이 파일에 그대로 남긴다**. 아래 두 축이
// 그 계약을 잠근다.
//   (1) 진리표 4행이 무변경일 것 (CT-09 ~ CT-12)
//   (2) `storeSourceActive` 항이 `resolvePanelSourceBinding(config).active` 와
//       **동치**일 것 — 이것이 성립해야 치환이 동작 보존이다
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.3 CT-09 ~ CT-12 / AC-11
// ---------------------------------------------------------------------------
describe('게이지 판정 진리표 특성화 (SPEC-TSDB-002 M2, CT-09~CT-12)', () => {
  it('CT-09: data_source 부재/channel → legacy (소스·대표값과 무관)', () => {
    expect(resolveFromConfig({})).toBe('legacy');
    expect(resolveFromConfig({ dataSources: legacyPresent() })).toBe('legacy');
    // 활성 store_source 와 series_reduce 가 모두 있어도 data_source 가 없으면 legacy.
    expect(
      resolveFromConfig({ store_source: activeKeysStore(), series_reduce: 'last' }),
    ).toBe('legacy');
    expect(
      resolveFromConfig({
        data_source: 'channel',
        store_source: activeKeysStore(),
        series_reduce: 'last',
      }),
    ).toBe('legacy');
  });

  it('CT-10: store + 소스 비활성 → legacy (series_reduce 유무와 무관)', () => {
    for (const store of [undefined, inactiveKeysStore(), inactiveTagStore()]) {
      expect(
        resolveFromConfig({ data_source: 'store', store_source: store, series_reduce: 'last' }),
      ).toBe('legacy');
      expect(resolveFromConfig({ data_source: 'store', store_source: store })).toBe('legacy');
    }
  });

  it('CT-11: store + 소스 활성 + series_reduce **부재** → legacy (하중 지지 행)', () => {
    // 이 행이 `hasSeriesReduce` 논리곱의 유일한 증거다. 그 항을 소스 활성 판정으로
    // 함께 옮기면 이 행이 'store-source' 로 뒤집힌다.
    expect(resolveFromConfig({ data_source: 'store', store_source: activeKeysStore() })).toBe(
      'legacy',
    );
    expect(resolveFromConfig({ data_source: 'store', store_source: activeTagStore() })).toBe(
      'legacy',
    );
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeKeysStore(),
        dataSources: legacyPresent(),
      }),
    ).toBe('legacy');
  });

  it('CT-12: store + 소스 활성 + series_reduce 있음 → store-source', () => {
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeKeysStore(),
        series_reduce: 'last',
      }),
    ).toBe('store-source');
    expect(
      resolveFromConfig({
        data_source: 'store',
        store_source: activeTagStore(),
        series_reduce: 'last',
      }),
    ).toBe('store-source');
  });

  it('CT-09~CT-12: storeSourceActive 항은 resolvePanelSourceBinding(config).active 와 동치다', () => {
    // M3 의 치환이 동작 보존인 **구조적 근거**. 두 판정이 갈리는 config 가 하나라도
    // 있으면 게이지의 활성 조건이 바뀐다.
    const stores = [
      undefined,
      activeKeysStore(),
      activeTagStore(),
      inactiveKeysStore(),
      inactiveTagStore(),
    ];
    const kinds = [undefined, 'channel', 'store'];
    for (const store_source of stores) {
      for (const data_source of kinds) {
        const config: Record<string, unknown> = { store_source };
        if (data_source !== undefined) config.data_source = data_source;
        expect(gaugeValueSourceFlags(config).storeSourceActive).toBe(
          isStoreSourceActive(store_source),
        );
        // `data_source:'store'` 인 config 에 한해 panelDataSource 의 active 와도 동치다.
        if (data_source === 'store') {
          expect(resolvePanelSourceBinding(config).active).toBe(
            gaugeValueSourceFlags(config).storeSourceActive,
          );
        }
      }
    }
  });
});
