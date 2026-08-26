// SeriesSelectTable 단위 테스트 — 저장소 스타일 정렬/컬럼 필터/선택.
//
// @spec SPEC-WEB-005

import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { SeriesSelectTable, type SeriesRow } from './SeriesSelectTable';
import { makeSeriesId, SERIES_ID_SEPARATOR } from '@/services/api/seriesLabels';

// i18n: ko.json 을 점 표기 키로 해석하는 mock. 컴포넌트가 useTranslation 을
// 쓰지만 I18nProvider 로 감싸지 않으므로 키를 한국어로 해석해 단언을 통과시킨다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

/** SeriesID → 행/체크박스 testid 접미사(NUL → '~'). */
function safe(id: string): string {
  return id.split(SERIES_ID_SEPARATOR).join('~');
}

function row(
  key: string,
  field: string,
  dataType: string,
  registration: string,
  tags: Record<string, string>,
): SeriesRow {
  return { id: makeSeriesId(key, field, tags), key, field, dataType, registration, tags };
}

const ROWS: SeriesRow[] = [
  row('alpha', 'temp', 'float', 'manual', { room: '1' }),
  row('bravo', 'humid', 'int', 'auto', { room: '2' }),
  row('charlie', 'temp', 'float', 'manual', { room: '2' }),
];

/** 선택 상태를 보유하는 controlled 래퍼 — 토글이 체크 상태에 반영되도록. */
function Harness({
  rows = ROWS,
  showRegistration = true,
  onToggleSpy,
}: {
  rows?: SeriesRow[];
  showRegistration?: boolean;
  onToggleSpy?: (id: string) => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  return (
    <SeriesSelectTable
      rows={rows}
      selectedIds={selected}
      onToggle={(id) => {
        onToggleSpy?.(id);
        setSelected((prev) =>
          prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
        );
      }}
      onSelectMany={(ids) =>
        setSelected((prev) => Array.from(new Set([...prev, ...ids])))
      }
      onClearMany={(ids) =>
        setSelected((prev) => prev.filter((id) => !ids.includes(id)))
      }
      showRegistration={showRegistration}
    />
  );
}

/** tbody 행 체크박스만 추출(헤더/팝오버 체크박스 제외). */
function rowCheckboxes(): HTMLInputElement[] {
  return screen
    .getAllByRole('checkbox')
    .filter((el) =>
      (el.getAttribute('data-testid') ?? '').startsWith('series-select-'),
    ) as HTMLInputElement[];
}

describe('SeriesSelectTable', () => {
  it('모든 시리즈를 행으로 렌더하고 컬럼 헤더를 노출한다', () => {
    render(<Harness />);
    expect(rowCheckboxes().length).toBe(3);
    expect(screen.getByTestId('series-col-key')).toBeInTheDocument();
    expect(screen.getByTestId('series-col-field')).toBeInTheDocument();
    expect(screen.getByTestId('series-col-dataType')).toBeInTheDocument();
    expect(screen.getByTestId('series-col-tags')).toBeInTheDocument();
    expect(screen.getByTestId('series-col-registration')).toBeInTheDocument();
  });

  it('showRegistration=false 면 등록 컬럼이 숨겨진다', () => {
    render(<Harness showRegistration={false} />);
    expect(screen.queryByTestId('series-col-registration')).toBeNull();
  });

  it('행 체크박스 클릭 시 onToggle 이 SeriesID 로 호출된다', () => {
    const spy = vi.fn();
    render(<Harness onToggleSpy={spy} />);
    const id = makeSeriesId('alpha', 'temp', { room: '1' });
    fireEvent.click(screen.getByTestId(`series-select-${safe(id)}`));
    expect(spy).toHaveBeenCalledWith(id);
  });

  it('전체 선택/해제는 현재 필터된 행을 대상으로 한다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('tsdb-select-all'));
    expect(rowCheckboxes().every((c) => c.checked)).toBe(true);
    fireEvent.click(screen.getByTestId('tsdb-clear-all'));
    expect(rowCheckboxes().some((c) => c.checked)).toBe(false);
  });

  it('필드 컬럼 필터에서 값을 선택하면 해당 행만 남는다', () => {
    render(<Harness />);
    // metric 필터 팝오버 열기 → 'temp' 선택.
    fireEvent.click(screen.getByTestId('series-filter-field'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-temp'));
    // temp 인 alpha/charlie 만 남고 humid 인 bravo 는 제외.
    expect(
      screen.getByTestId(`series-tr-${safe(makeSeriesId('alpha', 'temp', { room: '1' }))}`),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`series-tr-${safe(makeSeriesId('charlie', 'temp', { room: '2' }))}`),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId(`series-tr-${safe(makeSeriesId('bravo', 'humid', { room: '2' }))}`),
    ).toBeNull();
    expect(rowCheckboxes().length).toBe(2);
  });

  it('태그 컬럼 필터(room=2)는 해당 태그를 가진 행만 남긴다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-tags'));
    fireEvent.click(screen.getByTestId('series-filter-opt-tags-room=2'));
    // room=2 인 bravo/charlie 만.
    expect(rowCheckboxes().length).toBe(2);
    expect(
      screen.queryByTestId(`series-tr-${safe(makeSeriesId('alpha', 'temp', { room: '1' }))}`),
    ).toBeNull();
  });

  it('데이터 타입 필터(int)는 int 행만 남긴다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-dataType'));
    fireEvent.click(screen.getByTestId('series-filter-opt-dataType-int'));
    expect(rowCheckboxes().length).toBe(1);
    expect(
      screen.getByTestId(`series-tr-${safe(makeSeriesId('bravo', 'humid', { room: '2' }))}`),
    ).toBeInTheDocument();
  });

  it('필터 전체(clear) 버튼은 해당 컬럼 필터를 해제한다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-temp'));
    expect(rowCheckboxes().length).toBe(2);
    fireEvent.click(screen.getByTestId('series-filter-clear-field'));
    expect(rowCheckboxes().length).toBe(3);
  });

  it('키 컬럼 정렬은 asc ↔ desc 로 순서를 토글한다', () => {
    render(<Harness />);
    const keyOrder = () =>
      rowCheckboxes().map((c) => (c.getAttribute('aria-label') ?? '').replace(' 선택', ''));
    // 기본 정렬: key asc → alpha, bravo, charlie.
    expect(keyOrder()).toEqual(['alpha', 'bravo', 'charlie']);
    // key 정렬 버튼 클릭 → desc.
    fireEvent.click(screen.getByTestId('series-sort-key'));
    expect(keyOrder()).toEqual(['charlie', 'bravo', 'alpha']);
  });

  it('모든 행이 필터에서 제외되면 빈 상태 메시지를 보여준다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-temp'));
    fireEvent.click(screen.getByTestId('series-filter-clear-field'));
    // dataType=int + tag room=1 → 교집합 없음.
    fireEvent.click(screen.getByTestId('series-filter-dataType'));
    fireEvent.click(screen.getByTestId('series-filter-opt-dataType-int'));
    fireEvent.click(screen.getByTestId('series-filter-tags'));
    fireEvent.click(screen.getByTestId('series-filter-opt-tags-room=1'));
    expect(screen.getByText('일치하는 시리즈가 없습니다.')).toBeInTheDocument();
    expect(rowCheckboxes().length).toBe(0);
  });

  it('선택 후 필드 필터로 행이 숨겨져도 선택 상태는 유지된다', () => {
    render(<Harness />);
    const bravoId = makeSeriesId('bravo', 'humid', { room: '2' });
    fireEvent.click(screen.getByTestId(`series-select-${safe(bravoId)}`));
    // temp 만 남기면 bravo(humid) 는 숨겨진다.
    fireEvent.click(screen.getByTestId('series-filter-field'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-temp'));
    expect(screen.queryByTestId(`series-select-${safe(bravoId)}`)).toBeNull();
    // 필터 해제 시 bravo 가 다시 보이고 여전히 체크되어 있다.
    fireEvent.click(screen.getByTestId('series-filter-clear-field'));
    const bravoCb = screen.getByTestId(
      `series-select-${safe(bravoId)}`,
    ) as HTMLInputElement;
    expect(bravoCb.checked).toBe(true);
  });

  it('등록 컬럼 필터(auto)는 auto 행만 남긴다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-registration'));
    fireEvent.click(screen.getByTestId('series-filter-opt-registration-auto'));
    const visible = rowCheckboxes();
    expect(visible.length).toBe(1);
    const tr = screen.getByTestId(
      `series-tr-${safe(makeSeriesId('bravo', 'humid', { room: '2' }))}`,
    );
    expect(within(tr).getByText('auto')).toBeInTheDocument();
  });
});

// 필터 팝오버가 표 밖(body)에 그려지는지 — 두 결함의 뿌리였다.
//
// 팝오버를 표 안에 그리면 (1) `overflow-auto` 컨테이너에 잘려 아래 항목에 닿을 수
// 없고, (2) `sticky` + `z-index` 인 thead 가 쌓임 맥락을 만들어 바깥 백드롭이
// 팝오버를 덮는다 — 항목을 클릭하면 백드롭이 맞아 그대로 닫혔다.
describe('필터 팝오버 배치 (회귀)', () => {
  it('팝오버는 표 컨테이너 밖에 그려진다 — 잘리지 않게', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));

    const popover = screen.getByTestId('series-filter-popover-field');
    const table = screen.getByRole('table');
    // 표 안에 있으면 스크롤 컨테이너에 잘린다.
    expect(table.contains(popover)).toBe(false);
    expect(document.body.contains(popover)).toBe(true);
  });

  it('팝오버 자신이 스크롤된다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));

    const popover = screen.getByTestId('series-filter-popover-field');
    expect(popover.className).toContain('overflow-y-auto');
    expect(popover.className).toContain('max-h-64');
  });

  it('항목을 클릭해도 닫히지 않는다 — 연속 선택이 된다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));

    // 첫 항목 선택 후에도 팝오버가 살아 있어야 두 번째를 고를 수 있다.
    fireEvent.mouseDown(screen.getByTestId('series-filter-opt-field-temp'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-temp'));
    expect(screen.getByTestId('series-filter-popover-field')).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByTestId('series-filter-opt-field-humid'));
    fireEvent.click(screen.getByTestId('series-filter-opt-field-humid'));
    expect(screen.getByTestId('series-filter-popover-field')).toBeInTheDocument();
  });

  it('바깥을 누르면 닫힌다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    expect(screen.getByTestId('series-filter-popover-field')).toBeInTheDocument();

    fireEvent.mouseDown(document.body);
    expect(screen.queryByTestId('series-filter-popover-field')).toBeNull();
  });

  it('필터 버튼을 다시 누르면 닫힌다', () => {
    render(<Harness />);
    const btn = screen.getByTestId('series-filter-field');
    fireEvent.click(btn);
    expect(screen.getByTestId('series-filter-popover-field')).toBeInTheDocument();

    fireEvent.mouseDown(btn);
    fireEvent.click(btn);
    expect(screen.queryByTestId('series-filter-popover-field')).toBeNull();
  });

  it('표가 스크롤되면 닫는다 — 앵커가 어긋난 채 떠 있지 않게', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    expect(screen.getByTestId('series-filter-popover-field')).toBeInTheDocument();

    fireEvent.scroll(window, {});
    expect(screen.queryByTestId('series-filter-popover-field')).toBeNull();
  });

  it('다른 컬럼 필터를 열면 이전 것은 닫힌다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    fireEvent.mouseDown(screen.getByTestId('series-filter-dataType'));
    fireEvent.click(screen.getByTestId('series-filter-dataType'));

    expect(screen.queryByTestId('series-filter-popover-field')).toBeNull();
    expect(screen.getByTestId('series-filter-popover-dataType')).toBeInTheDocument();
  });

  it('백드롭은 더 이상 쓰지 않는다 — 쌓임 맥락 다툼의 원인이었다', () => {
    render(<Harness />);
    fireEvent.click(screen.getByTestId('series-filter-field'));
    expect(screen.queryByTestId('series-filter-backdrop')).toBeNull();
  });
});
