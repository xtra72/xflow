// TsdbSourceSection — 에이전트 선택 · bucket 능력 게이팅 · 3단 드릴다운 · 48 상한.
//
// 디스커버리 조회기는 props 로 주입하므로 네트워크가 없다. `useAgents` 와 i18n 만
// 모킹한다(StoreSourceSection.test.tsx 와 같은 패턴).
//
// @spec SPEC-TSDB-002 §2.9 (U9) · §2.13 (S1) · §2.15 (O1) · §2.18 (U11)

import type React from 'react';
import { useState } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';

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

/** 시리즈 목록을 새로 만든다 — 새 모델에서 목록은 리프레시로만 갱신된다. */
async function refreshList(): Promise<void> {
  const btn = await screen.findByTestId('chart-tsdb-refresh');
  await act(async () => {
    fireEvent.click(btn);
  });
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
    // 사유는 타이틀 뒤 "?" 로 접혀 있다. 기본 상태에서는 보이지 않는다.
    expect(screen.queryByTestId('chart-tsdb-fill-reason')).toBeNull();
    const why = screen.getByTestId('chart-tsdb-fill-why');
    expect(why).toHaveAttribute('aria-expanded', 'false');

    // 펼치면 사유가 **실제 텍스트로 DOM 에 들어간다** — hover 툴팁이 아니라
    // 토글인 이유이며, 키보드·스크린리더 사용자에게도 도달한다.
    fireEvent.click(why);
    expect(screen.getByTestId('chart-tsdb-fill-reason')).toHaveTextContent(
      'dashboard.chart.capReasonFillAvg',
    );
    expect(why).toHaveAttribute('aria-expanded', 'true');
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
    // 목록은 **리프레시로만** 채워진다(요구 4) — 누르기 전에는 비어 있다.
    expect(seriesTableCheckboxes()).toHaveLength(0);
    await refreshList();
    // 선택 표 **안**의 체크박스만 센다.
    expect(seriesTableCheckboxes()).toHaveLength(2);
  });

  // [삭제] 태그 값을 고르면 후보 행의 태그가 좁혀지고 선택에 반영된다
  //   드릴다운 제거. 트리의 개별 값 선택이 이를 대체하며 아래 트리 테스트가 덮는다.

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
    await refreshList();
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
    await refreshList();
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
  // [삭제] measurement 를 고르면 태그 키가 그룹 기준 후보로 나온다
  //   트리 노드 테스트로 대체.

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
    await refreshList();
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
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-rack'));

    // 그룹 기준을 걸면 표가 **조합마다 한 행**으로 펼쳐진다. 열거를 기다린다.
    await refreshList();
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));

    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches).length).toBe(1));

    const entry = lastSeries(patches)[0]!;
    // 키 순서는 정렬해 고정한다 — 같은 선택이 다른 config 를 만들면 안 된다.
    expect(entry.group_by).toEqual(['host', 'rack']);
    // 항목은 하나로 유지되고 고른 조합만 group_filter 에 모인다.
    expect(entry.group_filter).toEqual([{ host: 'a', rack: 'r1' }]);
  });

  // [삭제] 값을 고정한 태그는 그룹 기준으로 고를 수 없다 (UB1-3)
  //   태그 값 고정 필터가 트리로 흡수되어 '고정된 키' 개념 자체가 사라졌다. UB1-3 은 서버 검증으로만 남는다.

  // [삭제] 그룹 기준으로 고른 뒤 그 태그 값을 고정하면 그룹 축에서 빠진다
  //   위와 같은 사유 — 고정이라는 별도 조작이 없다.

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
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    expect((screen.getByTestId('chart-tsdb-tree-key-host') as HTMLInputElement).checked).toBe(
      true,
    );

    await selectMeasurement('mem');
    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-tree-key-host') as HTMLInputElement).checked,
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
    await refreshList();
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(1));

    // 조건 1 — 그룹 없이 등록.
    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(1));
    expect(lastSeries(patches)[0]!.group_by).toBeUndefined();

    // 조건 2 — 그룹 축을 켠다. 등록분은 그대로여야 한다.
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    await refreshList();
    // 고정된 등록분 1 + 새 그룹 후보 2 = 3. 등록한 것이 목록에서 사라지지
    // 않는다는 것이 요구 5 의 핵심이다.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
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
    await refreshList();
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(1));
    fireEvent.click(seriesTableCheckboxes()[0]!);
    await waitFor(() => expect(lastSeries(patches)).toHaveLength(1));

    // 조건을 바꾸고(그룹 축 추가) 다시 등록한다.
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    await refreshList();
    // 고정 1 + 그룹 후보 2 = 3. 고정분은 맨 앞이므로 그 뒤를 고른다.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
    fireEvent.click(seriesTableCheckboxes()[1]!);

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
  it('다이얼로그를 다시 열면 measurement 와 그룹 기준이 복원된다', async () => {
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
        (screen.getByTestId('chart-tsdb-tree-key-host') as HTMLInputElement).checked,
      ).toBe(true),
    );

    // 태그 필터 표시는 제거됐다(트리가 대체) — 복원 대상은 measurement 와 트리다.
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
  // [삭제] 그룹 기준을 걸면 실제 태그 값이 목록으로 나온다
  //   미리보기 목록이 트리의 자식 노드로 대체됐다.

  // [삭제] 미리보기는 사전 필터를 서버에 넘긴다
  //   사전 필터가 트리로 흡수되어 별도 축이 아니다.

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

  // [삭제] 열거 실패는 편집을 막지 않는다
  //   리프레시 실패 경고로 대체 — chart-tsdb-refresh-error 가 같은 계약을 고정한다.
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
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    await refreshList();
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
    await refreshList();
    // 트리가 저장된 값(A·C)만 고른 상태로 복원되므로 후보도 그 둘이다.
    // B 를 보려면 트리에서 값 선택을 풀거나 키만 체크하면 된다.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));
    for (const b of seriesTableCheckboxes()) expect(b.checked).toBe(true);
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
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    // 키가 둘이면 조합이 필요하므로 열거 경로를 탄다(태그 값 조회는 조합을 못 준다).
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-rack'));

    await refreshList();
    // 목록은 2개지만 그것이 전부가 아니라는 사실이 화면에 드러나야 한다.
    expect(await screen.findByTestId('chart-tsdb-refresh-truncated')).toBeTruthy();
    // 기본 field 2종 × 조합 2개 = 4행. 개수보다 **절단 경고가 뜬다**는 것이
    // 이 테스트의 요지다 — 짧은 목록을 전부인 것처럼 보여 주지 않아야 한다.
    expect(seriesTableCheckboxes()).toHaveLength(4);
  });

  it('잘리지 않으면 경고를 표시하지 않는다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={makeFetchers({ fetchTagKeys: vi.fn(async () => ['host']) })}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    await refreshList();
    expect(screen.queryByTestId('chart-tsdb-refresh-truncated')).toBeNull();
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
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    // 다중 키라야 열거 경로를 탄다(단일 키는 태그 값 조회를 쓴다).
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-rack'));
    await refreshList();
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

// ===== 그룹 기준 트리 (사용자 요구 3) =====

describe('TsdbSourceSection — 그룹 기준 트리', () => {
  function treeFetchers() {
    return makeFetchers({
      fetchTagKeys: vi.fn(async () => ['host', 'rack']),
      fetchFieldKeys: vi.fn(async () => ['usage']),
      fetchTagValues: vi.fn(async (_a: string, _m: string, key: string) =>
        key === 'host' ? ['A', 'B', 'C'] : ['r1', 'r2'],
      ),
    });
  }

  it('키만 체크하면 그 키의 모든 값이 후보가 된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={treeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-key-host')).toBeTruthy());
    fireEvent.click(screen.getByTestId('chart-tsdb-tree-key-host'));
    await refreshList();
    // host 값 3종 × field 1 = 3.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
  });

  it('펼쳐서 개별 값을 고르면 그 값들만 후보가 된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={treeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    await waitFor(() => expect(screen.getByTestId('chart-tsdb-tree-expand-host')).toBeTruthy());

    fireEvent.click(screen.getByTestId('chart-tsdb-tree-expand-host'));
    // 값 노드는 펼칠 때 지연 조회한다.
    const a = await screen.findByTestId('chart-tsdb-tree-value-host-A');
    fireEvent.click(a);
    fireEvent.click(await screen.findByTestId('chart-tsdb-tree-value-host-C'));

    await refreshList();
    // 고른 2개만 후보다 — 값 선택이 곧 범위 제한이다.
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(2));
  });

  it('값을 고르면 그 키가 자동으로 그룹 축이 된다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={treeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    fireEvent.click(await screen.findByTestId('chart-tsdb-tree-expand-host'));
    fireEvent.click(await screen.findByTestId('chart-tsdb-tree-value-host-A'));

    // 키를 따로 켜라고 요구하면 조작이 두 번이다.
    await waitFor(() =>
      expect(
        (screen.getByTestId('chart-tsdb-tree-key-host') as HTMLInputElement).checked,
      ).toBe(true),
    );
  });

  it('리프레시 전에는 조건을 바꿔도 목록이 그대로다', async () => {
    render(
      <Harness
        initial={tsdbConfig({ agent_id: 'ix2', agent_name: 'influx-v2' })}
        fetchers={treeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    await refreshList();
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(1));

    // 그룹 축을 켜도 리프레시 전에는 목록이 바뀌지 않는다(요구 4).
    fireEvent.click(await screen.findByTestId('chart-tsdb-tree-key-host'));
    expect(seriesTableCheckboxes()).toHaveLength(1);

    await refreshList();
    await waitFor(() => expect(seriesTableCheckboxes()).toHaveLength(3));
  });
});

// ===== 시리즈 이름 형식 (사용자 요구) =====

describe('TsdbSourceSection — 시리즈 이름 형식', () => {
  it('입력과 기본 표기 안내를 제공한다', async () => {
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [{ key: 'cpu', field: 'usage', tags: { host: 'a' } }],
        })}
        fetchers={makeFetchers()}
      />,
    );
    expect(screen.getByTestId('chart-tsdb-series-name-format')).toBeTruthy();
    // 미지정이 정상 상태이며, 그때 무엇이 쓰이는지 화면이 말해야 한다.
    expect(screen.getByTestId('chart-tsdb-series-name-format-hint').textContent).toContain(
      'dashboard.chart.tsdbSeriesNameFormatHint',
    );
    expect(
      (screen.getByTestId('chart-tsdb-series-name-format-input') as HTMLInputElement).value,
    ).toBe('');
  });

  it('형식을 입력하면 config 에 반영되고 비우면 미지정으로 돌아간다', async () => {
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
    const input = screen.getByTestId('chart-tsdb-series-name-format-input');
    fireEvent.change(input, { target: { value: 'CPU {$.tags.host}' } });
    await waitFor(() => {
      const last = patches[patches.length - 1]?.tsdb_source as { series_name_format?: string };
      expect(last?.series_name_format).toBe('CPU {$.tags.host}');
    });

    // 공백만 남기면 undefined 로 되돌아간다 — 미지정이 정상 상태다.
    fireEvent.change(input, { target: { value: '   ' } });
    await waitFor(() => {
      const last = patches[patches.length - 1]?.tsdb_source as { series_name_format?: string };
      expect(last?.series_name_format).toBeUndefined();
    });
  });
});

describe('TsdbSourceSection — 이름 형식 위치', () => {
  it('이름 형식이 시리즈 목록보다 위에 온다', async () => {
    render(
      <Harness
        initial={tsdbConfig({
          agent_id: 'ix2',
          agent_name: 'influx-v2',
          series: [{ key: 'cpu', field: 'usage' }],
        })}
        fetchers={makeFetchers()}
      />,
    );
    await selectMeasurement('cpu');
    const fmt = screen.getByTestId('chart-tsdb-series-name-format');
    const list = await screen.findByTestId('chart-tsdb-series-select');
    // DOM 순서로 위/아래를 단언한다 — 이름을 정한 뒤 목록에서 고르는 흐름이다.
    expect(fmt.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
