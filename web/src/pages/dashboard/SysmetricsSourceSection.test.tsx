// sysmetrics 데이터소스 섹션 — 시리즈 표 배선 테스트.
//
// 설정 다이얼로그와 같은 방식(부모가 draft 를 들고 갱신된 panel 을 다시 내려준다)으로
// 감싸 실제 클릭을 재현한다.
//
// 잠그는 것:
//   - Store · TSDB 와 **같은 표**(`SeriesSelectTable`)로 시리즈를 고른다
//   - 여럿 골라도 전부 남는다 (보고된 결함: 하나만 남던 문제)
//   - 이미 고른 대상이 보고 목록에서 사라져도 행이 남아 해제할 수 있다
//   - 고른 행에서 이름·색을 그 자리에서 편집한다 (두 번째 목록을 만들지 않는다)

import { describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

const AGENT = { id: 'a1', name: 'host-1', type: 'sysmetrics' };

/** 에이전트 스냅샷이 알려주는 대상 목록. 테스트가 갈아 끼운다. */
const snapshotRef = vi.hoisted(() => ({
  interfaces: ['en0', 'en1', 'en2'] as string[] | null,
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [AGENT] } }),
  useAgent: () => ({
    data:
      snapshotRef.interfaces === null
        ? undefined
        : {
            state: {
              status: 'running',
              collected_at: 1_000,
              interval_seconds: 5,
              network: Object.fromEntries(
                snapshotRef.interfaces.map((n) => [n, { bytes_recv: 1 }]),
              ),
              targets: {
                mountpoints: ['/'],
                devices: ['disk0'],
                interfaces: snapshotRef.interfaces,
              },
            },
          },
  }),
}));

// 아래 "실제 경로" 묶음은 StoreSourceSection 을 거친다 — 그쪽이 쓰는 조회들을 스텁한다.
vi.mock('@/services/api/charts', () => ({
  listChartChannels: () => Promise.resolve([]),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: [], isLoading: false }),
  useStoreTagPairs: () => ({ data: [], isLoading: false }),
}));

import { SERIES_ID_SEPARATOR } from '@/services/api/seriesLabels';
import type { PanelConfig } from '@/stores/uiStore';
import { SysmetricsSourceSection } from './SysmetricsSourceSection';
import { PanelSettingsDataSource } from './PanelSettingsDataSource';
import { sysmetricRowId } from './panels/charts/sysmetricsSource';

/**
 * 설정 다이얼로그와 같은 배선의 컨테이너.
 *
 * `onConfigChange` 로 받은 patch 를 draft 에 얕게 병합하고, 갱신된 config 를 다시
 * 내려준다(`PanelSettingsDialog` 의 `patchConfig` + `panel` useMemo 와 같다).
 */
function Harness({
  initial,
  onDraft,
}: {
  initial: Record<string, unknown>;
  onDraft: (c: Record<string, unknown>) => void;
}) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial);
  const panel = { id: 'p1', type: 'graph-chart', title: 't', config } as PanelConfig;
  return (
    <SysmetricsSourceSection
      panel={panel}
      onConfigChange={(patch) =>
        setConfig((prev) => {
          const next = { ...prev, ...patch };
          onDraft(next);
          return next;
        })
      }
    />
  );
}

function baseConfig(series: unknown[] = []): Record<string, unknown> {
  return {
    data_source: 'sysmetrics',
    sysmetrics_source: { agent_id: 'a1', agent_name: 'host-1', series },
  };
}

function pickedSeries(config: Record<string, unknown>): { key: string; target?: string }[] {
  const src = config.sysmetrics_source as { series?: { key: string; target?: string }[] };
  return src.series ?? [];
}

/** `SeriesSelectTable` 의 testid 규약 — SeriesID 의 NUL 구분자를 '~' 로 바꾼다. */
function safeId(key: string, target?: string): string {
  return sysmetricRowId({ key, target }).split(SERIES_ID_SEPARATOR).join('~');
}

/** 표의 행 체크박스를 집는다. */
function rowCheckbox(key: string, target?: string): HTMLElement {
  return screen.getByTestId(`series-select-${safeId(key, target)}`);
}

/**
 * 표의 키 검색으로 목록을 좁힌다.
 *
 * 표는 페이지로 나뉘므로 후보가 한 페이지를 넘으면 원하는 행이 2페이지에 있을 수
 * 있다. 검색은 사용자가 실제로 쓰는 경로이기도 하다.
 */
function searchKey(text: string): void {
  fireEvent.click(screen.getByTestId('series-filter-key'));
  fireEvent.change(screen.getByTestId('series-filter-key-input'), { target: { value: text } });
}

describe('시리즈 표 — 다중 선택', () => {
  it('인터페이스 셋을 고르면 셋 다 시리즈로 남는다', () => {
    // 보고된 결함: 여럿 골라도 설정에 하나만 남았다.
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);

    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en1'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en2'));

    expect(pickedSeries(latest)).toEqual([
      { key: 'network.bytes_recv', target: 'en0' },
      { key: 'network.bytes_recv', target: 'en1' },
      { key: 'network.bytes_recv', target: 'en2' },
    ]);
  });

  it('값마다 다른 대상을 고를 수 있다 (곱 모델로는 불가능하던 조합)', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);

    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));
    fireEvent.click(rowCheckbox('network.bytes_sent', 'en1'));

    expect(pickedSeries(latest)).toEqual([
      { key: 'network.bytes_recv', target: 'en0' },
      { key: 'network.bytes_sent', target: 'en1' },
    ]);
  });

  it('종합 행과 개별 대상 행은 서로 다른 시리즈다', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);

    fireEvent.click(rowCheckbox('network.bytes_recv'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));

    expect(pickedSeries(latest)).toEqual([
      { key: 'network.bytes_recv' },
      { key: 'network.bytes_recv', target: 'en0' },
    ]);
  });

  it('다시 누르면 그 시리즈만 빠진다', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);

    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en1'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));

    expect(pickedSeries(latest)).toEqual([{ key: 'network.bytes_recv', target: 'en1' }]);
  });

  it('체크 상태가 화면에도 유지된다', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    render(<Harness initial={baseConfig()} onDraft={() => {}} />);

    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en1'));

    expect(rowCheckbox('network.bytes_recv', 'en0')).toBeChecked();
    expect(rowCheckbox('network.bytes_recv', 'en1')).toBeChecked();
  });

  it('인스턴스 축이 없는 값은 대상 없는 행 하나다', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);
    searchKey('usage_percent');

    fireEvent.click(rowCheckbox('cpu.usage_percent'));
    expect(pickedSeries(latest)).toEqual([{ key: 'cpu.usage_percent' }]);
  });
});

describe('시리즈 표 — 이미 고른 대상이 스냅샷에 없을 때', () => {
  it('행이 남아 체크 상태가 보이고 해제할 수 있다', () => {
    // 에이전트가 지금은 en0 만 보고한다(NIC 제거·다운·수집 필터).
    // 행을 지우면 고른 시리즈가 화면에서 사라지고 해제할 방법도 없어진다.
    snapshotRef.interfaces = ['en0'];
    const initial = baseConfig([
      { key: 'network.bytes_recv', target: 'en0' },
      { key: 'network.bytes_recv', target: 'gone0' },
    ]);
    let latest: Record<string, unknown> = initial;
    render(<Harness initial={initial} onDraft={(c) => (latest = c)} />);

    expect(rowCheckbox('network.bytes_recv', 'gone0')).toBeChecked();

    fireEvent.click(rowCheckbox('network.bytes_recv', 'gone0'));
    expect(pickedSeries(latest)).toEqual([{ key: 'network.bytes_recv', target: 'en0' }]);
  });

  it('첫 표본 이전이어도 이미 고른 시리즈는 보인다', () => {
    snapshotRef.interfaces = null;
    render(
      <Harness
        initial={baseConfig([{ key: 'network.bytes_recv', target: 'en0' }])}
        onDraft={() => {}}
      />,
    );

    expect(rowCheckbox('network.bytes_recv', 'en0')).toBeChecked();
  });
});

describe('시리즈 표 — 행 상세 편집', () => {
  it('고른 행에서 이름을 붙일 수 있고 placeholder 는 내장 표기다', () => {
    snapshotRef.interfaces = ['en0', 'en1'];
    const initial = baseConfig([{ key: 'network.bytes_recv', target: 'en0' }]);
    let latest: Record<string, unknown> = initial;
    render(<Harness initial={initial} onDraft={(c) => (latest = c)} />);

    const id = sysmetricRowId({ key: 'network.bytes_recv', target: 'en0' });
    const input = screen.getByTestId(`chart-sysmetrics-alias-${id}`);
    expect(input).toHaveAttribute('placeholder', 'bytes_recv · {category=network, interface=en0}');

    fireEvent.change(input, { target: { value: '내부망 수신' } });
    expect(pickedSeries(latest)[0]).toMatchObject({ alias: '내부망 수신' });
  });

  it('고르지 않은 행에는 편집 자리가 없다', () => {
    snapshotRef.interfaces = ['en0', 'en1'];
    render(
      <Harness
        initial={baseConfig([{ key: 'network.bytes_recv', target: 'en0' }])}
        onDraft={() => {}}
      />,
    );

    const other = sysmetricRowId({ key: 'network.bytes_recv', target: 'en1' });
    expect(screen.queryByTestId(`chart-sysmetrics-alias-${other}`)).toBeNull();
  });
});

describe('실제 설정 다이얼로그 경로 — PanelSettingsDataSource 를 거친 다중 선택', () => {
  function FullHarness({ onDraft }: { onDraft: (c: Record<string, unknown>) => void }) {
    const [config, setConfig] = useState<Record<string, unknown>>(baseConfig());
    const panel = { id: 'p1', type: 'graph-chart', title: 't', config } as PanelConfig;
    return (
      <PanelSettingsDataSource
        panel={panel}
        onConfigChange={(patch) =>
          setConfig((prev) => {
            const next = { ...prev, ...patch };
            onDraft(next);
            return next;
          })
        }
      />
    );
  }

  it('인터페이스 셋을 고르면 셋 다 남는다', () => {
    snapshotRef.interfaces = ['en0', 'en1', 'en2'];
    let latest: Record<string, unknown> = baseConfig();
    render(<FullHarness onDraft={(c) => (latest = c)} />);

    fireEvent.click(rowCheckbox('network.bytes_recv', 'en0'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en1'));
    fireEvent.click(rowCheckbox('network.bytes_recv', 'en2'));

    expect(pickedSeries(latest).map((s) => s.target)).toEqual(['en0', 'en1', 'en2']);
  });
});

describe('SysmetricsSourceSection — 인터벌 집계', () => {
  it('저장된 집계를 선택한 상태로 렌더한다', () => {
    render(
      <Harness
        initial={{
          data_source: 'sysmetrics',
          sysmetrics_source: {
            agent_id: 'a1',
            agent_name: 'host-1',
            series: [],
            aggregation: 'max',
          },
        }}
        onDraft={() => {}}
      />,
    );
    expect((screen.getByTestId('chart-sysmetrics-aggregation') as HTMLSelectElement).value).toBe(
      'max',
    );
  });

  it('집계를 바꾸면 소스 설정에 저장된다', () => {
    // 종전에는 config 에만 있고 조작 통로가 없어 저장된 값이 무엇이든 바꿀 수 없었다.
    let latest: Record<string, unknown> = baseConfig();
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);

    fireEvent.change(screen.getByTestId('chart-sysmetrics-aggregation'), {
      target: { value: 'first' },
    });
    const src = latest.sysmetrics_source as { aggregation?: string };
    expect(src.aggregation).toBe('first');
  });

  it('설정이 없으면 기본 집계를 보여준다 — 빈 값으로 두지 않는다', () => {
    render(<Harness initial={baseConfig()} onDraft={() => {}} />);
    const sel = screen.getByTestId('chart-sysmetrics-aggregation') as HTMLSelectElement;
    expect(sel.value).not.toBe('');
  });
});

describe('조회 방식 — 이력 / 실시간', () => {
  it('미지정이면 이력이 선택돼 있다(이 축이 생기기 전 저장된 패널의 동작)', () => {
    render(<Harness initial={baseConfig()} onDraft={() => {}} />);
    expect(
      screen.getByTestId('chart-sysmetrics-query-mode-history').getAttribute('aria-checked'),
    ).toBe('true');
    expect(
      screen.getByTestId('chart-sysmetrics-query-mode-live').getAttribute('aria-checked'),
    ).toBe('false');
  });

  it('실시간을 고르면 query_mode 가 저장된다', () => {
    let latest: Record<string, unknown> = {};
    render(<Harness initial={baseConfig()} onDraft={(c) => (latest = c)} />);
    fireEvent.click(screen.getByTestId('chart-sysmetrics-query-mode-live'));
    expect(
      (latest.sysmetrics_source as { query_mode?: string }).query_mode,
    ).toBe('live');
  });

  it('실시간에서는 창·인터벌·집계·빈버킷 칸을 내린다(뜻이 없는 칸은 고쳐도 아무 일이 없다)', () => {
    render(
      <Harness
        initial={baseConfig()}
        onDraft={() => {}}
      />,
    );
    // 이력에서는 보인다.
    expect(screen.getByTestId('chart-sysmetrics-aggregation')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('chart-sysmetrics-query-mode-live'));
    expect(screen.queryByTestId('chart-sysmetrics-aggregation')).toBeNull();
    expect(screen.queryByTestId('chart-sysmetrics-fill')).toBeNull();
  });

  it('모드마다 감수하는 것을 다른 문구로 고지한다', () => {
    render(<Harness initial={baseConfig()} onDraft={() => {}} />);
    const notice = () => screen.getByTestId('chart-sysmetrics-mode-notice').textContent;
    expect(notice()).toBe('dashboard.chart.sysmetricsHistoryNotice');
    fireEvent.click(screen.getByTestId('chart-sysmetrics-query-mode-live'));
    expect(notice()).toBe('dashboard.chart.sysmetricsLiveNotice');
  });
});
