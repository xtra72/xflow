// TsdbSourceSection — 에이전트 선택 · bucket 능력 게이팅 · 3단 드릴다운 · 48 상한.
//
// 디스커버리 조회기는 props 로 주입하므로 네트워크가 없다. `useAgents` 와 i18n 만
// 모킹한다(StoreSourceSection.test.tsx 와 같은 패턴).
//
// @spec SPEC-TSDB-002 §2.9 (U9) · §2.13 (S1) · §2.15 (O1) · §2.18 (U11)

import type React from 'react';
import { useState } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

/** 에이전트 목록 — influxdb v2 · influxdb v3 · store 각 1개. */
let mockAgents: Array<{
  id: string;
  name: string;
  type: string;
  config?: Record<string, unknown>;
}> = [];
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: mockAgents } }),
}));

import { TsdbSourceSection, type TsdbDiscoveryFetchers } from './TsdbSourceSection';
import { defaultTsdbSource, type TsdbSourceConfig } from './panels/charts/chartChannelTypes';

function makePanel(config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type: 'line-chart', title: '테스트', config };
}

/** 열거 응답 형상으로 감싼다 — 테스트는 시리즈 목록만 신경 쓰면 된다. */
function enumOf(
  series: Array<{ tags: Record<string, string>; fields: string[] }>,
  truncated = false,
) {
  return {
    series,
    field_exact: true,
    count: series.length,
    truncated,
    window: { start_ms: 0, end_ms: 1 },
  };
}

/** 주입 조회기 기본값 — 각 테스트가 필요한 것만 덮어쓴다. */
function makeFetchers(over: Partial<TsdbDiscoveryFetchers> = {}): TsdbDiscoveryFetchers {
  return {
    fetchBuckets: vi.fn(async () => [{ name: 'metrics' }, { name: 'logs' }]),
    fetchMeasurements: vi.fn(async () => ['cpu', 'mem']),
    fetchFieldKeys: vi.fn(async () => ['usage', 'idle']),
    fetchTagKeys: vi.fn(async () => ['host']),
    fetchTagValues: vi.fn(async () => ['a', 'b']),
    fetchSeriesEnum: vi.fn(async () => enumOf([
      { tags: { host: 'a' }, fields: ['usage'] },
      { tags: { host: 'b' }, fields: ['usage'] },
    ])),
    ...over,
  };
}

function tsdbConfig(over: Partial<TsdbSourceConfig> = {}): Record<string, unknown> {
  return {
    data_source: 'tsdb',
    tsdb_source: { ...defaultTsdbSource(), ...over },
  };
}

/**
 * 제어 컴포넌트 하네스 — 부모가 패치를 반영해야 선택이 누적된다(프로덕션과 같은 흐름).
 * `onPatch` 로 매 패치를 관측한다.
 */
function Harness({
  initial,
  fetchers,
  onPatch,
}: {
  initial: Record<string, unknown>;
  fetchers: TsdbDiscoveryFetchers;
  onPatch?: (patch: Record<string, unknown>) => void;
}): React.ReactElement {
  const [config, setConfig] = useState<Record<string, unknown>>(initial);
  return (
    <TsdbSourceSection
      panel={makePanel(config)}
      fetchers={fetchers}
      onConfigChange={(patch) => {
        onPatch?.(patch);
        setConfig((prev) => ({ ...prev, ...patch }));
      }}
    />
  );
}

/**
 * measurement 셀렉트에 값을 넣는다.
 *
 * 목록이 도착하기 전에 `fireEvent.change` 를 쏘면 제어 셀렉트에 해당 option 이 아직
 * 없어 값이 그대로 버려진다(빈 화면 그대로). 그래서 옵션이 채워질 때까지 기다린다.
 */
async function selectMeasurement(value: string): Promise<void> {
  const ms = screen.getByTestId('chart-tsdb-measurement-select') as HTMLSelectElement;
  await waitFor(() =>
    expect(Array.from(ms.options).map((o) => o.value)).toContain(value),
  );
  fireEvent.change(ms, { target: { value } });
  await waitFor(() => expect(ms.value).toBe(value));
}

/**
 * 선택 표 안의 체크박스만 돌려준다.
 *
 * `screen.getAllByRole('checkbox')` 는 그룹 기준 체크박스(SPEC-TSDB-004)까지
 * 집계하므로, 표의 행 수를 단언하는 곳에서는 범위를 좁혀야 한다.
 */
function seriesTableCheckboxes(): HTMLInputElement[] {
  const table = screen.queryByTestId('chart-tsdb-series-select');
  if (!table) return [];
  return Array.from(table.querySelectorAll('input[type="checkbox"]'));
}

/** 현재 config 의 tsdb_source 를 마지막 패치에서 읽는다. */
function lastSeries(patches: Array<Record<string, unknown>>): TsdbSourceConfig['series'] {
  const last = patches[patches.length - 1]?.tsdb_source as TsdbSourceConfig | undefined;
  return last?.series ?? [];
}

beforeEach(() => {
  mockAgents = [
    { id: 'ix2', name: 'influx-v2', type: 'influxdb', config: { version: '2' } },
    { id: 'ix3', name: 'influx-v3', type: 'influxdb', config: { version: '3' } },
    { id: 'st1', name: 'store-1', type: 'store' },
  ];
});

describe('TsdbSourceSection — 에이전트 선택 (§2.18)', () => {
  it('influxdb 타입 에이전트만 선택지로 노출한다', () => {
    render(<Harness initial={tsdbConfig()} fetchers={makeFetchers()} />);
    const select = screen.getByTestId('chart-tsdb-agent-select') as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).toEqual(['', 'ix2', 'ix3']);
    // store 에이전트는 TSDB 소스가 지원하는 백엔드가 아니므로 애초에 고를 수 없다.
    expect(values).not.toContain('st1');
  });

  it('에이전트를 고르면 agent_id 와 agent_name 을 함께 기록하고 시리즈를 비운다', () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [{ key: 'cpu', field: 'usage' }],
        })}
        fetchers={makeFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    fireEvent.change(screen.getByTestId('chart-tsdb-agent-select'), {
      target: { value: 'ix3' },
    });
    const next = patches[0]?.tsdb_source as TsdbSourceConfig;
    expect(next.agent_id).toBe('ix3');
    expect(next.agent_name).toBe('influx-v3');
    // 스키마가 다른 에이전트로 옮겼으므로 이전 시리즈는 해석되지 않는다.
    expect(next.series).toEqual([]);
  });

  it('에이전트 미선택이면 시리즈 표 대신 안내를 보여주고 디스커버리를 호출하지 않는다', () => {
    const fetchers = makeFetchers();
    render(<Harness initial={tsdbConfig()} fetchers={fetchers} />);
    expect(screen.getByTestId('chart-tsdb-no-agent')).toBeInTheDocument();
    expect(screen.queryByTestId('chart-tsdb-series-select')).not.toBeInTheDocument();
    expect(fetchers.fetchMeasurements).not.toHaveBeenCalled();
    expect(fetchers.fetchBuckets).not.toHaveBeenCalled();
  });
});

describe('TsdbSourceSection — 백엔드 버전별 능력 (§2.13 · OQ10)', () => {
  it('v2 는 bucket 을 목록에서 고른다', async () => {
    const fetchers = makeFetchers();
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={fetchers}
      />,
    );
    expect(screen.getByTestId('chart-tsdb-backend')).toHaveTextContent(
      'dashboard.chart.tsdbBackendInfluxV2',
    );
    const select = screen.getByTestId('chart-tsdb-bucket-select') as HTMLSelectElement;
    await waitFor(() =>
      expect(Array.from(select.options).map((o) => o.value)).toEqual([
        '',
        'metrics',
        'logs',
      ]),
    );
    expect(fetchers.fetchBuckets).toHaveBeenCalledWith('influx-v2');
    // 관리 조작 안내는 v2 에서 뜨지 않는다 — v2 는 관리가 가능하다.
    expect(screen.queryByTestId('chart-tsdb-management-notice')).not.toBeInTheDocument();
  });

  it('v3 는 bucket 자유 입력 + 질의 미반영 사유 + 관리 미지원 안내를 표시한다', () => {
    const fetchers = makeFetchers();
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix3', agent_name: 'influx-v3' })}
        fetchers={fetchers}
      />,
    );
    expect(screen.getByTestId('chart-tsdb-backend')).toHaveTextContent(
      'dashboard.chart.tsdbBackendInfluxV3',
    );
    // v3 는 목록 API 가 없으므로 자유 입력이다(OQ10).
    expect(screen.getByTestId('chart-tsdb-bucket-input')).toBeInTheDocument();
    expect(screen.queryByTestId('chart-tsdb-bucket-select')).not.toBeInTheDocument();
    expect(fetchers.fetchBuckets).not.toHaveBeenCalled();
    // §HISTORY-0.4.0 (2): v3 에서 bucket 은 질의에 도달하지 않는다 — 그 사실을 드러낸다.
    expect(screen.getByText('dashboard.chart.capReasonBucketV3')).toBeInTheDocument();
    // 관리 조작은 501 이지만 디스커버리는 활성이다(§2.10 · §2.13).
    const notice = screen.getByTestId('chart-tsdb-management-notice');
    expect(notice).toHaveAttribute('aria-disabled', 'true');
    expect(notice).toHaveTextContent('dashboard.chart.capReasonManagementV3');
    expect(fetchers.fetchMeasurements).toHaveBeenCalled();
  });

  it('미지원 설정은 숨기지 않고 aria-disabled 로 표시한다', () => {
    // AC-38 — §2.13 [S1] 은 미지원 선택지를 **숨기지 말고** 비활성 + 사유로 두라고
    // 규정한다(SPEC-AUTH-006 §4.2 원칙 승계). 숨기면 사용자는 "이 소스에 없는 기능"과
    // "내가 못 찾는 것"을 구분할 수 없다.
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers()}
      />,
    );
    // fill: 'avg' 는 목록에 남아 있으며 비활성이다.
    const avg = screen.getByTestId('chart-tsdb-fill-avg');
    expect(avg).toBeInTheDocument();
    expect(avg).toHaveAttribute('aria-disabled', 'true');
    expect(avg).toBeDisabled();
    // 사유가 툴팁이 아니라 읽히는 문구로 존재한다(스크린리더 도달).
    expect(screen.getByTestId('chart-tsdb-fill-reason')).toHaveTextContent(
      'dashboard.chart.capReasonFillAvg',
    );
    // 나머지 fill 전략은 TSDB 에서 지원되므로 셀렉트 자체는 활성이다.
    expect(screen.getByTestId('chart-tsdb-fill')).not.toHaveAttribute(
      'aria-disabled',
      'true',
    );
  });
});

describe('TsdbSourceSection — measurement → field → tag 드릴다운 (§2.15)', () => {
  it('measurement 를 고르면 field 목록으로 선택 표를 채운다', async () => {
    const fetchers = makeFetchers();
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={fetchers}
      />,
    );
    expect(screen.getByTestId('chart-tsdb-no-measurement')).toBeInTheDocument();

    const ms = screen.getByTestId('chart-tsdb-measurement-select') as HTMLSelectElement;
    await waitFor(() => expect(ms.options.length).toBe(3));
    fireEvent.change(ms, { target: { value: 'cpu' } });

    await waitFor(() =>
      expect(screen.getByTestId('chart-tsdb-series-select')).toBeInTheDocument(),
    );
    expect(fetchers.fetchFieldKeys).toHaveBeenCalledWith('influx-v2', 'cpu', undefined);
    // 선택 표 **안**의 체크박스만 센다. 전역 조회는 그룹 기준 체크박스까지
    // 집계해 "표에 field 행이 몇 개인가" 라는 이 단언의 의도를 흐린다.
    expect(seriesTableCheckboxes()).toHaveLength(2);
  });

  it('태그 값을 고르면 후보 행의 태그가 좁혀지고 선택에 반영된다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    const fetchers = makeFetchers();
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={fetchers}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() =>
      expect(screen.getByTestId('chart-tsdb-tag-key-select')).toBeEnabled(),
    );
    fireEvent.change(screen.getByTestId('chart-tsdb-tag-key-select'), {
      target: { value: 'host' },
    });
    await waitFor(() =>
      expect(fetchers.fetchTagValues).toHaveBeenCalledWith(
        'influx-v2',
        'cpu',
        'host',
        undefined,
      ),
    );
    fireEvent.change(screen.getByTestId('chart-tsdb-tag-value-select'), {
      target: { value: 'a' },
    });
    expect(screen.getByTestId('chart-tsdb-tag-filters')).toHaveTextContent('host=a');

    // 행 식별자는 Store 와 같은 규칙(`key field tags`)이므로 태그가 좁혀지면 id 도 바뀐다.
    fireEvent.click(screen.getByTestId('series-select-cpu usage host=a'));
    expect(lastSeries(patches)).toEqual([
      { key: 'cpu', field: 'usage', tags: { host: 'a' } },
    ]);
  });

  it('체크를 해제하면 시리즈가 제거된다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [{ key: 'cpu', field: 'usage' }],
        })}
        fetchers={makeFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    // 태그 없는 시리즈의 식별자는 태그부가 빈 문자열이다(`"cpu usage "`).
    // testing-library 의 기본 정규화가 후행 공백을 지우므로 질의는 trim 된 형태로 쓴다.
    const box = await screen.findByTestId('series-select-cpu usage');
    expect(box).toBeChecked();
    fireEvent.click(box);
    expect(lastSeries(patches)).toEqual([]);
  });
});

describe('TsdbSourceSection — 시리즈 48 상한 (§2.9 [U9])', () => {
  it('일괄 선택이 48 개를 넘으면 초과분을 반영하지 않고 경고한다', async () => {
    // 상한은 안내가 아니라 **강제**다 — 통과시키면 폴링당 요청 수가 상한 없이 늘어난다.
    const fields = Array.from({ length: 60 }, (_, i) => `f${i}`);
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchFieldKeys: vi.fn(async () => fields) })}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(60));

    fireEvent.click(screen.getByTestId('tsdb-select-all'));

    expect(lastSeries(patches)).toHaveLength(48);
    expect(screen.getByTestId('chart-tsdb-over-limit')).toBeInTheDocument();
  });
});

describe('TsdbSourceSection — 디스커버리 실패는 편집을 막지 않는다', () => {
  it('bucket 목록 조회가 실패해도 안내만 띄우고 나머지 편집은 살아 있다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchBuckets: vi.fn(async () => {
            throw new Error('501');
          }),
        })}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId('chart-tsdb-bucket-error')).toBeInTheDocument(),
    );
    // 목록이 없어도 measurement 드릴다운은 계속 쓸 수 있다.
    expect(screen.getByTestId('chart-tsdb-measurement-select')).toBeEnabled();
  });
});

// ===== group by 축 (SPEC-TSDB-004 §2.1 · M6) =====

describe('TsdbSourceSection — 그룹 기준 (SPEC-TSDB-004)', () => {
  it('measurement 를 고르면 태그 키가 그룹 기준 후보로 나온다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host', 'rack']) })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by')).toBeTruthy());
    expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy();
    expect(screen.getByTestId('chart-tsdb-group-by-rack')).toBeTruthy();
  });

  it('고르지 않으면 선택된 시리즈에 group_by 가 실리지 않는다 (정확 일치 모드)', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-series-select')).toBeTruthy());
    const boxes = seriesTableCheckboxes();
    fireEvent.click(boxes[boxes.length - 1]!);

    await waitFor(() => expect(lastSeries(patches).length).toBeGreaterThan(0));
    expect(lastSeries(patches)[0]!.group_by).toBeUndefined();
  });

  it('그룹 행을 고르면 group_by 와 고른 조합(group_filter)이 함께 실린다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['rack', 'host']),
          fetchFieldKeys: vi.fn(async () => ['usage']),
          fetchSeriesEnum: vi.fn(async () => enumOf([
            { tags: { host: 'a', rack: 'r1' }, fields: ['usage'] },
            { tags: { host: 'b', rack: 'r2' }, fields: ['usage'] },
          ])),
        })}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-rack'));

    // 그룹 기준을 걸면 표가 **조합마다 한 행**으로 펼쳐진다. 열거를 기다린다.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));

    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches).length).toBe(1));

    const entry = lastSeries(patches)[0]!;
    // 키 순서는 정렬해 고정한다 — 같은 선택이 다른 config 를 만들면 안 된다.
    expect(entry.group_by).toEqual(['host', 'rack']);
    // 항목은 하나로 유지되고 고른 조합만 group_filter 에 모인다.
    expect(entry.group_filter).toEqual([{ host: 'a', rack: 'r1' }]);
  });

  it('값을 고정한 태그는 그룹 기준으로 고를 수 없다 (UB1-3)', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host']) })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());

    // host 를 태그 필터로 고정한다.
    fireEvent.change(screen.getByTestId('chart-tsdb-tag-key-select'), {
      target: { value: 'host' },
    });
    const tv = screen.getByTestId('chart-tsdb-tag-value-select') as HTMLSelectElement;
    await waitFor(() => expect(Array.from(tv.options).map((o) => o.value)).toContain('a'));
    fireEvent.change(tv, { target: { value: 'a' } });

    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).disabled,
      ).toBe(true),
    );
    // 사유가 화면에 읽히는 문구로 있어야 한다(툴팁만으로는 스크린리더에 닿지 않는다).
    expect(screen.getByTestId('chart-tsdb-group-by-pinned')).toBeTruthy();
  });

  it('그룹 기준으로 고른 뒤 그 태그 값을 고정하면 그룹 축에서 빠진다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host']) })}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    expect((screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked).toBe(
      true,
    );

    fireEvent.change(screen.getByTestId('chart-tsdb-tag-key-select'), {
      target: { value: 'host' },
    });
    const tv = screen.getByTestId('chart-tsdb-tag-value-select') as HTMLSelectElement;
    await waitFor(() => expect(Array.from(tv.options).map((o) => o.value)).toContain('a'));
    fireEvent.change(tv, { target: { value: 'a' } });

    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked,
      ).toBe(false),
    );
  });

  it('group by 항목이 있으면 설정 화면에 그 사실이 드러난다 (§2.11 S1)', async () => {
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [
            { key: 'cpu', field: 'usage', group_by: ['host'] },
            { key: 'mem', field: 'used' },
          ],
        })}
        fetchers={makeFetchers()}
      />,
    );
    // 등록 목록은 조건과 무관하게 등록분 **전체**를 보여 준다.
    const items = await screen.findAllByTestId('chart-tsdb-registered-item');
    expect(items).toHaveLength(2);
    const text = items.map((i) => i.textContent).join(' ');
    expect(text).toContain('cpu.usage');
    expect(text).toContain('mem.used');
  });

  it('measurement 를 바꾸면 그룹 축이 초기화된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host']) })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    expect((screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked).toBe(
      true,
    );

    await selectMeasurement('mem');
    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked,
      ).toBe(false),
    );
  });
});

describe('TsdbSourceSection — 검색 커서와 등록의 분리 (조건을 바꿔 누적 등록)', () => {
  // **UB1-13 을 대체한다.** 종전에는 "그룹 기준을 체크하면 이미 고른 시리즈에
  // 반영된다" 를 요구했으나, 검색/등록을 분리한 모델에서는 그 동작이 오히려
  // 해롭다 — 조건 A 로 등록한 뒤 조건 B 를 검색하려고 축을 바꾸면 A 가 덮여
  // 사라진다. 커서 변경은 등록분을 건드리지 않는 것이 맞다.
  it('그룹 기준을 바꿔도 이미 등록된 시리즈는 그대로다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host']),
          fetchFieldKeys: vi.fn(async () => ['usage']),
          fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) =>
            key === 'host' ? ['A', 'B'] : ['a', 'b'],
          ),
        })}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(1));

    // 조건 1 — 그룹 없이 등록.
    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(1));
    expect(lastSeries(patches)[0]!.group_by).toBeUndefined();

    // 조건 2 — 그룹 축을 켠다. 등록분은 그대로여야 한다.
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));
    expect(lastSeries(patches)).toHaveLength(1);
    expect(lastSeries(patches)[0]!.group_by).toBeUndefined();
  });

  it('조건을 바꿔 검색한 시리즈를 추가로 등록한다 (누적)', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host']),
          fetchFieldKeys: vi.fn(async () => ['usage']),
          fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) =>
            key === 'host' ? ['A', 'B'] : ['a', 'b'],
          ),
        })}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(1));
    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(1));

    // 조건을 바꾸고(그룹 축 추가) 다시 등록한다.
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));
    fireEvent.click(seriesTableCheckboxes()[0]!);

    // 두 조건의 등록이 **함께** 남는다.
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(2));
    const entries = lastSeries(patches);
    expect(entries.filter((e) => e.group_by === undefined)).toHaveLength(1);
    expect(entries.filter((e) => (e.group_by?.length ?? 0) > 0)).toHaveLength(1);
  });

  it('등록 목록에서 개별 해제할 수 있다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [
            { key: 'cpu', field: 'usage' },
            { key: 'mem', field: 'used' },
          ],
        })}
        fetchers={makeFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    const items = await screen.findAllByTestId('chart-tsdb-registered-item');
    expect(items).toHaveLength(2);

    fireEvent.click(screen.getByTestId('chart-tsdb-registered-remove-0'));
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(1));
    expect(lastSeries(patches)[0]!.key).toBe('mem');
  });

  it('등록이 없으면 안내를 표시한다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers()}
      />,
    );
    expect(screen.getByTestId('chart-tsdb-registered-empty')).toBeTruthy();
  });

  it('전체 해제는 등록분을 비운다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [{ key: 'cpu', field: 'usage' }],
        })}
        fetchers={makeFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    fireEvent.click(await screen.findByTestId('chart-tsdb-registered-clear'));
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(0));
  });
});

describe('TsdbSourceSection — 저장된 선택에서 커서 복원 (버그 재현)', () => {
  it('다이얼로그를 다시 열면 measurement · 그룹 기준 · 태그 필터가 복원된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [
            { key: 'cpu', field: 'usage', tags: { region: 'kr' }, group_by: ['host'] },
          ],
        })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host', 'region']) })}
      />,
    );

    // measurement 셀렉트가 저장된 값을 가리킨다.
    const ms = screen.getByTestId('chart-tsdb-measurement-select') as HTMLSelectElement;
    await waitFor(() => expect(ms.value).toBe('cpu'));

    // 그룹 기준 체크가 복원된다.
    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked,
      ).toBe(true),
    );

    // 태그 필터도 복원된다.
    expect(screen.getByTestId('chart-tsdb-tag-filters').textContent).toContain('region=kr');
  });

  it('선택이 없으면 커서는 비어 있다 (신규 패널)', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers()}
      />,
    );
    const ms = screen.getByTestId('chart-tsdb-measurement-select') as HTMLSelectElement;
    expect(ms.value).toBe('');
  });
});

describe('TsdbSourceSection — 그룹 미리보기 (사용자 보고)', () => {
  it('그룹 기준을 걸면 실제 태그 값이 목록으로 나온다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host']),
          fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) =>
            key === 'host' ? ['A', 'B', 'C'] : ['a', 'b'],
          ),
        })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));

    const items = await screen.findAllByTestId('chart-tsdb-group-preview-item');
    expect(items).toHaveLength(3);
    expect(items.map((i) => i.textContent)).toEqual(['host=A', 'host=B', 'host=C']);
    // 이 파일의 i18n 모의는 키를 그대로 돌려주므로 개수 문구는 키로 단언한다.
    // 개수 자체는 위 목록 길이가 고정한다.
    expect(screen.getByTestId('chart-tsdb-group-preview-count').textContent).toContain(
      'dashboard.chart.tsdbGroupPreviewCount',
    );
  });

  it('미리보기는 사전 필터를 서버에 넘긴다', async () => {
    const fetchTagValues = vi.fn(async (_a: string, _m: string, key: string) =>
      key === 'host' ? ['A'] : ['a', 'b'],
    );
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host', 'region']), fetchTagValues })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());

    // region 을 사전 필터로 고정한다.
    fireEvent.change(screen.getByTestId('chart-tsdb-tag-key-select'), {
      target: { value: 'region' },
    });
    const tv = screen.getByTestId('chart-tsdb-tag-value-select') as HTMLSelectElement;
    await waitFor(() => expect(Array.from(tv.options).map((o) => o.value)).toContain('a'));
    fireEvent.change(tv, { target: { value: 'a' } });

    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    // 태그 값 조회는 (agent, measurement, tagKey, bucket, filters) 로 부른다.
    await waitFor(() =>
      expect(
        fetchTagValues.mock.calls.some((c) => (c as unknown as unknown[])[4] !== undefined),
      ).toBe(true),
    );
    const withFilter = fetchTagValues.mock.calls.find(
      (c) => (c as unknown as unknown[])[4] !== undefined,
    ) as unknown as [string, string, string, string | undefined, Record<string, string>];
    expect(withFilter[4]).toEqual({ region: 'a' });
  });

  it('그룹 기준이 없으면 미리보기를 그리지 않는다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-series-select')).toBeTruthy());
    expect(screen.queryByTestId('chart-tsdb-group-preview')).toBeNull();
  });

  it('열거 실패는 편집을 막지 않는다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host']),
          fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) => {
            if (key === 'host') throw new Error('403');
            return ['a', 'b'];
          }),
        })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));

    expect(await screen.findByTestId('chart-tsdb-group-preview-error')).toBeTruthy();
    // 체크는 그대로 유지된다 — 미리보기 실패가 설정을 되돌리지 않는다.
    expect((screen.getByTestId('chart-tsdb-group-by-host') as HTMLInputElement).checked).toBe(
      true,
    );
  });
});

describe('TsdbSourceSection — 그룹 행 선택 (사용자 요청: N개로 나눠 고르기)', () => {
  function groupedFetchers() {
    // 그룹 키가 하나이므로 미리보기는 태그 값 조회(D3)를 탄다.
    return makeFetchers({
      fetchTagKeys: vi.fn(async () => ['host']),
      fetchFieldKeys: vi.fn(async () => ['usage']),
      // 드롭다운과 그룹 미리보기가 같은 조회기를 쓰므로 **키로 구분**한다.
      fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) =>
        key === 'host' ? ['A', 'B', 'C'] : ['a', 'b'],
      ),
    });
  }

  async function setupGrouped(patches: Array<Record<string, unknown>>) {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={groupedFetchers()}
        onPatch={(p) => patches.push(p)}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
  }

  it('그룹 기준을 걸면 표가 그룹 수만큼 행으로 나뉜다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    await setupGrouped(patches);
    // field 1개 × 그룹 3개 = 행 3개.
    expect(seriesTableCheckboxes()).toHaveLength(3);
  });

  it('여러 그룹을 고르면 항목 1개에 조합이 모인다 (요청이 늘지 않는다)', async () => {
    const patches: Array<Record<string, unknown>> = [];
    await setupGrouped(patches);

    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches).length).toBe(1));
    fireEvent.click(seriesTableCheckboxes()[2]!);
    await waitFor(() =>
      expect(lastSeries(patches)[0]!.group_filter).toHaveLength(2),
    );

    const entry = lastSeries(patches)[0]!;
    expect(lastSeries(patches)).toHaveLength(1);
    expect(entry.group_filter).toEqual([{ host: 'A' }, { host: 'C' }]);
  });

  it('마지막 조합을 해제하면 항목이 사라진다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    await setupGrouped(patches);

    fireEvent.click(seriesTableCheckboxes()[1]!);
    await waitFor(() => expect(lastSeries(patches).length).toBe(1));
    fireEvent.click(seriesTableCheckboxes()[1]!);
    // 빈 group_filter 는 백엔드 규약상 "전 그룹" 이라 남기면 해제가 오히려
    // 전부 켜는 결과가 된다.
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(0));
  });

  it('전체 선택은 모든 조합을 한 항목에 모은다', async () => {
    const patches: Array<Record<string, unknown>> = [];
    await setupGrouped(patches);

    fireEvent.click(screen.getByTestId('tsdb-select-all'));
    await waitFor(() => expect(lastSeries(patches).length).toBe(1));
    expect(lastSeries(patches)[0]!.group_filter).toHaveLength(3);
  });

  it('저장된 선택이 표의 체크 상태로 복원된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [
            {
              key: 'cpu',
              field: 'usage',
              group_by: ['host'],
              group_filter: [{ host: 'A' }, { host: 'C' }],
            },
          ],
        })}
        fetchers={groupedFetchers()}
      />,
    );
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
    const boxes = seriesTableCheckboxes();
    expect(boxes[0]!.checked).toBe(true);
    expect(boxes[1]!.checked).toBe(false);
    expect(boxes[2]!.checked).toBe(true);
  });
});

describe('TsdbSourceSection — 열거 절단 · 시간창 (사용자 보고: 시리즈 2개만 보임)', () => {
  it('열거가 잘리면 경고를 표시한다 (조용히 짧은 목록을 주지 않는다)', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host', 'rack']),
          fetchSeriesEnum: vi.fn(async () =>
            enumOf(
              [
                { tags: { host: 'A', rack: 'r1' }, fields: ['usage'] },
                { tags: { host: 'B', rack: 'r2' }, fields: ['usage'] },
              ],
              true, // 상한에 걸림
            ),
          ),
        })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    // 키가 둘이면 조합이 필요하므로 열거 경로를 탄다(태그 값 조회는 조합을 못 준다).
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-rack'));

    // 목록은 2개지만 그것이 전부가 아니라는 사실이 화면에 드러나야 한다.
    expect(await screen.findByTestId('chart-tsdb-group-preview-truncated')).toBeTruthy();
    expect(await screen.findAllByTestId('chart-tsdb-group-preview-item')).toHaveLength(2);
  });

  it('잘리지 않으면 경고를 표시하지 않는다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host']) })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    await screen.findAllByTestId('chart-tsdb-group-preview-item');
    expect(screen.queryByTestId('chart-tsdb-group-preview-truncated')).toBeNull();
  });

  it('열거에 패널의 시간창을 넘긴다 (서버 기본 30일을 쓰지 않는다)', async () => {
    const fetchSeriesEnum = vi.fn(async () =>
      enumOf([{ tags: { host: 'A' }, fields: ['usage'] }]),
    );
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          time_window_ms: 3_600_000,
        })}
        fetchers={makeFetchers({
          fetchTagKeys: vi.fn(async () => ['host', 'rack']),
          fetchSeriesEnum,
        })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-group-by-host')).toBeTruthy());
    // 다중 키라야 열거 경로를 탄다.
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-group-by-rack'));
    await waitFor(() => expect(fetchSeriesEnum).toHaveBeenCalled());

    const call = fetchSeriesEnum.mock.calls[fetchSeriesEnum.mock.calls.length - 1] as unknown as [
      string,
      string,
      Record<string, string>,
      string | undefined,
      { startMs: number; endMs: number } | undefined,
    ];
    expect(call[4]).toBeDefined();
    // 창 길이가 패널 설정과 같아야 한다 — 30일 기본값이 아니다.
    expect(call[4]!.endMs - call[4]!.startMs).toBe(3_600_000);
  });
});
