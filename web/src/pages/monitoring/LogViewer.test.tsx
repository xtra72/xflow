// LogViewer 컬럼 헤더 필터 + 시간 정렬 UI 테스트.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (key: string) => key, locale: 'ko', setLocale: () => {} }),
}));

import LogViewer, { type LogEntry } from './LogViewer';

/** 로컬 시간 기준 epoch ms */
function at(h: number, m: number): number {
  return new Date(2026, 0, 15, h, m, 0).getTime();
}

const ENTRIES: LogEntry[] = [
  {
    id: '1',
    timestamp: '09:00:00',
    ts: at(9, 0),
    level: 'INFO',
    message: 'morning',
    source: 'flow',
    componentKind: 'http',
    componentName: 'f1',
  },
  {
    id: '2',
    timestamp: '12:00:00',
    ts: at(12, 0),
    level: 'ERROR',
    message: 'noon',
    source: 'agent',
    componentKind: 'modbus',
    componentName: 'a1',
  },
  {
    id: '3',
    timestamp: '20:00:00',
    ts: at(20, 0),
    level: 'WARN',
    message: 'evening',
    source: 'agent',
    componentKind: 'modbus',
    componentName: 'a2',
  },
];

/** 현재 렌더된 메시지 목록 (행 순서대로) */
function messages(): string[] {
  return screen.getAllByTestId('log-row').map((row) => row.textContent ?? '');
}

describe('LogViewer 시간 정렬', () => {
  it('기본은 오름차순이며 토글하면 내림차순이 된다', () => {
    render(<LogViewer entries={ENTRIES} />);

    expect(messages()[0]).toContain('morning');
    expect(messages()[2]).toContain('evening');

    fireEvent.click(screen.getByTestId('log-sort-time'));

    expect(messages()[0]).toContain('evening');
    expect(messages()[2]).toContain('morning');
  });

  it('다시 토글하면 오름차순으로 돌아온다', () => {
    render(<LogViewer entries={ENTRIES} />);
    const button = screen.getByTestId('log-sort-time');

    fireEvent.click(button);
    fireEvent.click(button);

    expect(messages()[0]).toContain('morning');
  });
});

describe('LogViewer 컬럼 헤더 필터', () => {
  it('레벨 필터는 컬럼 헤더에서 다중 선택된다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-level'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-ERROR'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-WARN'));

    const rows = messages();
    expect(rows).toHaveLength(2);
    expect(rows.join()).toContain('noon');
    expect(rows.join()).toContain('evening');
    expect(rows.join()).not.toContain('morning');
    expect(screen.getByTestId('log-filter-count-level')).toHaveTextContent('2');
  });

  it('소스 필터가 동작한다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-source'));
    fireEvent.click(screen.getByTestId('log-filter-option-source-flow'));

    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toContain('morning');
  });

  it('타입·이름 옵션은 현재 로그에서 뽑는다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-kind'));
    const kindMenu = screen.getByTestId('log-filter-menu-kind');
    expect(within(kindMenu).getByText('http')).toBeInTheDocument();
    expect(within(kindMenu).getByText('modbus')).toBeInTheDocument();
    // 데이터에 없는 값은 옵션에 나오지 않는다.
    expect(within(kindMenu).queryByText('mqtt')).not.toBeInTheDocument();
  });

  it('이름 필터가 동작한다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-name'));
    fireEvent.click(screen.getByTestId('log-filter-option-name-a2'));

    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toContain('evening');
  });

  it('전체를 누르면 그 컬럼 선택이 해제된다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-level'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-ERROR'));
    expect(messages()).toHaveLength(1);

    fireEvent.click(screen.getByTestId('log-filter-all-level'));
    expect(messages()).toHaveLength(3);
  });

  it('시간 구간 필터가 컬럼 헤더에서 동작한다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-time'));
    fireEvent.change(screen.getByTestId('log-filter-time-from'), {
      target: { value: '10:00' },
    });
    fireEvent.change(screen.getByTestId('log-filter-time-to'), {
      target: { value: '13:00' },
    });

    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toContain('noon');
  });

  it('여러 컬럼 필터는 AND 로 걸린다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-source'));
    fireEvent.click(screen.getByTestId('log-filter-option-source-agent'));
    fireEvent.click(screen.getByTestId('log-filter-level'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-WARN'));

    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toContain('evening');
  });

  it('필터 해제 버튼이 모든 컬럼 필터를 되돌린다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-level'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-ERROR'));
    expect(messages()).toHaveLength(1);

    fireEvent.click(screen.getByTestId('log-clear-all-filters'));
    expect(messages()).toHaveLength(3);
  });

  it('필터 결과가 없으면 안내 문구가 나온다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-time'));
    fireEvent.change(screen.getByTestId('log-filter-time-from'), {
      target: { value: '01:00' },
    });
    fireEvent.change(screen.getByTestId('log-filter-time-to'), {
      target: { value: '02:00' },
    });

    expect(screen.queryAllByTestId('log-row')).toHaveLength(0);
    // 로그 자체가 없는 경우와 필터로 0건인 경우는 문구가 다르다.
    expect(screen.getByText('monitoring.noFilteredLogs')).toBeInTheDocument();
  });

  it('필터가 걸리면 건수가 "보이는 수 / 전체 수" 로 표시된다', () => {
    render(<LogViewer entries={ENTRIES} />);
    expect(screen.getByText(`3monitoring.countUnit`)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('log-filter-level'));
    fireEvent.click(screen.getByTestId('log-filter-option-level-ERROR'));

    expect(screen.getByText(`1 / 3monitoring.countUnit`)).toBeInTheDocument();
  });
});

describe('LogViewer 팝오버 닫기', () => {
  it('Esc 로 필터 메뉴가 닫힌다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-level'));
    expect(screen.getByTestId('log-filter-menu-level')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByTestId('log-filter-menu-level')).not.toBeInTheDocument();
  });

  it('바깥을 누르면 필터 메뉴가 닫힌다', () => {
    render(<LogViewer entries={ENTRIES} />);

    fireEvent.click(screen.getByTestId('log-filter-level'));
    expect(screen.getByTestId('log-filter-menu-level')).toBeInTheDocument();

    fireEvent.mouseDown(document.body);
    expect(screen.queryByTestId('log-filter-menu-level')).not.toBeInTheDocument();
  });
});

describe('LogViewer 이름 링크', () => {
  const LINKABLE: LogEntry[] = [
    { id: '1', timestamp: '09:00:00', level: 'INFO', message: 'a', source: 'agent', componentName: 'modbus-1' },
    { id: '2', timestamp: '09:00:01', level: 'INFO', message: 'b', source: 'system', componentName: 'core' },
    { id: '3', timestamp: '09:00:02', level: 'INFO', message: 'c', source: 'flow', componentName: 'main' },
  ];

  it('에이전트·플로우 이름만 링크로 그린다', () => {
    render(<LogViewer entries={LINKABLE} onSelectComponent={() => {}} />);

    const links = screen.getAllByTestId('log-component-link');
    expect(links.map((l) => l.textContent)).toEqual(['modbus-1', 'main']);
    // system 소스는 갈 곳이 없어 평범한 텍스트로 남는다.
    expect(screen.getByText('core').tagName).not.toBe('BUTTON');
  });

  it('클릭하면 소스와 이름을 넘긴다', () => {
    const onSelect = vi.fn();
    render(<LogViewer entries={LINKABLE} onSelectComponent={onSelect} />);

    fireEvent.click(screen.getAllByTestId('log-component-link')[0]!);

    expect(onSelect).toHaveBeenCalledWith('agent', 'modbus-1');
  });

  it('핸들러가 없으면 링크를 그리지 않는다 (기존 동작 보존)', () => {
    render(<LogViewer entries={LINKABLE} />);

    expect(screen.queryAllByTestId('log-component-link')).toHaveLength(0);
    expect(screen.getByText('modbus-1')).toBeInTheDocument();
  });
});
