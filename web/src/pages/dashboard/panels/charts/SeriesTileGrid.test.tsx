// SeriesTileGrid 단위 테스트 (SPEC-CHART-002 M3.1, TDD).
//
// 다중 출력 그리드의 세 계약을 잠근다.
//  1) 열 수 자동 결정 — ceil(sqrt(N)) 을 상한으로 하고 최소 타일 폭이 실제 열 수를 줄인다
//  2) multi_output_limit 초과분 잘림 + `+K` 표기(접근 가능한 텍스트로 개수 전달)
//  3) 단일 항목(N=1)은 1열 1행 — 다중 출력 도입이 단일 시리즈 외형을 바꾸지 않는다
//
// @spec SPEC-CHART-002 §2.4 [U4] / AC-14

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

// i18n 스텁 — 키를 그대로 반환하되, 잘림 안내는 실제 템플릿을 돌려준다. 키만 반환하면
// `{count}` 치환이 일어났는지 확인할 수 없어 "접근 가능한 텍스트로 개수를 전달한다"
// (AC-14)를 검증할 수 없다.
vi.mock('@/lib/i18n', () => {
  const templates: Record<string, string> = {
    'dashboard.chart.multiOutputTruncated': '표시 상한을 넘어 {count}개 시리즈가 생략되었습니다.',
  };
  return { useTranslation: () => ({ t: (k: string) => templates[k] ?? k }) };
});

import { SeriesTileGrid } from './SeriesTileGrid';
import {
  DEFAULT_MULTI_OUTPUT_LIMIT,
  MIN_TILE_WIDTH_PX,
  applyMultiOutputLimit,
  DEFAULT_TILE_ROWS,
  MAX_TILE_ROWS,
  normalizeTileRows,
  tileColumnCount,
} from './multiOutputLimit';

/** 이름만 가진 최소 항목 — 그리드는 항목의 형상을 알지 못한다(제네릭). */
function items(n: number): Array<{ name: string }> {
  return Array.from({ length: n }, (_, i) => ({ name: `s${i}` }));
}

function renderGrid(n: number, limit?: number, fillRows?: boolean, rows?: number) {
  return render(
    <SeriesTileGrid
      items={items(n)}
      limit={limit}
      fillRows={fillRows}
      rows={rows}
      renderItem={(it) => <span data-testid="tile-label">{it.name}</span>}
    />,
  );
}

describe('normalizeTileRows', () => {
  it('미지정·비정상 값은 기본값(1)으로 되돌린다', () => {
    // 0 을 "행 없음" 으로 해석하면 사용자가 실수로 패널을 비울 수 있다(표시 상한과 같은 규칙).
    expect(normalizeTileRows(undefined)).toBe(DEFAULT_TILE_ROWS);
    expect(normalizeTileRows(0)).toBe(DEFAULT_TILE_ROWS);
    expect(normalizeTileRows(-3)).toBe(DEFAULT_TILE_ROWS);
    expect(normalizeTileRows(Number.NaN)).toBe(DEFAULT_TILE_ROWS);
  });

  it('소수는 버리고 상한을 넘으면 잘라낸다', () => {
    expect(normalizeTileRows(2.9)).toBe(2);
    expect(normalizeTileRows(MAX_TILE_ROWS)).toBe(MAX_TILE_ROWS);
    expect(normalizeTileRows(MAX_TILE_ROWS + 5)).toBe(MAX_TILE_ROWS);
  });
});

describe('tileColumnCount', () => {
  it('기본(1행)이면 항목 수만큼 열이 된다 — 한 줄로 늘어놓는다', () => {
    expect(tileColumnCount(0)).toBe(1);
    expect(tileColumnCount(1)).toBe(1);
    expect(tileColumnCount(4)).toBe(4);
    expect(tileColumnCount(12)).toBe(12);
  });

  it('행 수를 지정하면 ceil(N / rows) 열이 된다', () => {
    expect(tileColumnCount(4, 2)).toBe(2);
    expect(tileColumnCount(9, 3)).toBe(3);
    // 나누어떨어지지 않으면 마지막 행이 덜 찬다(열은 올림).
    expect(tileColumnCount(5, 2)).toBe(3);
    expect(tileColumnCount(7, 3)).toBe(3);
  });

  it('행 수가 항목 수보다 많아도 열은 최소 1 이다', () => {
    expect(tileColumnCount(2, 8)).toBe(1);
  });

  it('비정상 행 수는 기본값(1행)으로 취급한다', () => {
    expect(tileColumnCount(4, 0)).toBe(4);
    expect(tileColumnCount(4, Number.NaN)).toBe(4);
  });
});

describe('applyMultiOutputLimit', () => {
  it('기본 상한은 12 이며 초과분 개수를 함께 돌려준다', () => {
    expect(DEFAULT_MULTI_OUTPUT_LIMIT).toBe(12);
    const r = applyMultiOutputLimit(items(20));
    expect(r.visible).toHaveLength(12);
    expect(r.truncated).toBe(8);
    // 잘림은 "순서상 뒤에서부터" — 앞 12개가 남는다.
    expect(r.visible[0]!.name).toBe('s0');
    expect(r.visible[11]!.name).toBe('s11');
  });

  it('상한 이하이면 원본 그대로이고 truncated 는 0 이다', () => {
    const r = applyMultiOutputLimit(items(3));
    expect(r.visible).toHaveLength(3);
    expect(r.truncated).toBe(0);
  });

  it('limit 을 재정의할 수 있고 비정상 값은 기본 상한으로 되돌린다', () => {
    expect(applyMultiOutputLimit(items(20), 5).visible).toHaveLength(5);
    expect(applyMultiOutputLimit(items(20), 0).visible).toHaveLength(12);
    expect(applyMultiOutputLimit(items(20), -3).visible).toHaveLength(12);
    expect(applyMultiOutputLimit(items(20), Number.NaN).visible).toHaveLength(12);
    expect(applyMultiOutputLimit(items(20), 2.7).visible).toHaveLength(2);
  });
});

describe('SeriesTileGrid', () => {
  it('상한 초과 시 앞에서부터 limit 개만 렌더하고 +K 표기를 노출한다', () => {
    renderGrid(20);
    expect(screen.getAllByTestId('series-tile')).toHaveLength(12);
    const notice = screen.getByTestId('series-tile-truncation');
    // 개수는 접근 가능한 텍스트로 전달된다(aria-hidden 이 아니다).
    expect(notice.textContent).toContain('+8');
    expect(notice.textContent).toContain('8개 시리즈가 생략되었습니다');
  });

  it('multi_output_limit 로 상한을 재정의할 수 있다', () => {
    renderGrid(20, 3);
    expect(screen.getAllByTestId('series-tile')).toHaveLength(3);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+17');
  });

  it('상한 이하이면 잘림 표기를 렌더하지 않는다', () => {
    renderGrid(4);
    expect(screen.getAllByTestId('series-tile')).toHaveLength(4);
    expect(screen.queryByTestId('series-tile-truncation')).toBeNull();
  });

  it('단일 항목이면 그리드가 1열 1행으로 렌더된다', () => {
    renderGrid(1);
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-columns')).toBe('1');
    expect(screen.getAllByTestId('series-tile')).toHaveLength(1);
    expect(screen.queryByTestId('series-tile-truncation')).toBeNull();
  });

  it('기본은 1행 — 열 수가 항목 수와 같고 최소 타일 폭이 스타일에 반영된다', () => {
    renderGrid(9);
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-columns')).toBe('9');
    expect(grid.getAttribute('data-rows')).toBe('1');
    // 최소 타일 폭이 확보되지 않으면 열 수가 줄도록 auto-fit + minmax 로 표현한다.
    // 즉 행 수는 상한이 아니라 목표다 — 좁으면 열이 줄고 행이 늘어난다.
    expect(grid.style.gridTemplateColumns).toContain('auto-fit');
    expect(grid.style.gridTemplateColumns).toContain(`${MIN_TILE_WIDTH_PX}px`);
  });

  it('rows 를 지정하면 열 수가 ceil(N / rows) 로 줄어든다', () => {
    renderGrid(9, undefined, undefined, 3);
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-columns')).toBe('3');
    expect(grid.getAttribute('data-rows')).toBe('3');
  });

  it('rows 가 비정상이면 기본값(1행)으로 되돌린다', () => {
    renderGrid(4, undefined, undefined, 0);
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-columns')).toBe('4');
    expect(grid.getAttribute('data-rows')).toBe('1');
  });

  it('기본은 행 높이를 내용에 맡긴다 — gridAutoRows 를 싣지 않는다', () => {
    // stat 타일의 종전 배치. 키가 실리면 행 높이 결정 방식이 바뀐다.
    renderGrid(4);
    expect(screen.getByTestId('series-tile-grid').style.gridAutoRows).toBe('');
  });

  it('fillRows 면 행 높이를 그리드 높이에서 균등 분배한다', () => {
    // 게이지처럼 내용이 컨테이너 높이를 받아야 하는 타일용. 행 높이를 먼저 확정해
    // "행 ↔ 내용" 순환을 끊어야 낮은 패널에서 잘리지 않고 축소된다.
    renderGrid(4, undefined, true);
    expect(screen.getByTestId('series-tile-grid').style.gridAutoRows).toBe('minmax(0, 1fr)');
  });

  it('renderItem 이 항목 순서 그대로 호출된다(값 크기로 재정렬하지 않는다)', () => {
    renderGrid(5);
    const labels = screen.getAllByTestId('tile-label').map((el) => el.textContent);
    expect(labels).toEqual(['s0', 's1', 's2', 's3', 's4']);
  });

  it('항목이 0개면 그리드도 잘림 표기도 렌더하지 않는다', () => {
    renderGrid(0);
    expect(screen.queryByTestId('series-tile-grid')).toBeNull();
    expect(screen.queryByTestId('series-tile-truncation')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M6.5 — 잘림 표기 `+K` 의 접근성 텍스트.
//
// `+8` 만으로는 무엇이 8개인지 알 수 없다. 시각 표기는 aria-hidden 으로 감추고,
// 스크린리더에는 개수를 보간한 완결 문장 하나만 준다(중복 낭독 방지).
// ---------------------------------------------------------------------------

describe('잘림 표기 접근성 (AC-14 "접근 가능한 텍스트로 개수를 전달한다")', () => {
  /** 잘림 표기의 시각 span 과 스크린리더 전용 span 을 갈라 준다. */
  function truncationParts(): { visual: HTMLElement; sr: HTMLElement } {
    const notice = screen.getByTestId('series-tile-truncation');
    const spans = Array.from(notice.querySelectorAll('span'));
    const visual = spans.find((el) => el.getAttribute('aria-hidden') === 'true');
    const sr = spans.find((el) => el.classList.contains('sr-only'));
    expect(visual).toBeDefined();
    expect(sr).toBeDefined();
    return { visual: visual as HTMLElement, sr: sr as HTMLElement };
  }

  it('시각 표기 +K 는 aria-hidden 이라 스크린리더가 두 번 읽지 않는다', () => {
    renderGrid(20);
    const { visual } = truncationParts();
    expect(visual.textContent).toBe('+8');
    expect(visual).toHaveAttribute('aria-hidden', 'true');
  });

  it('스크린리더 전용 문장이 개수를 보간해 완결 문장으로 전달한다', () => {
    renderGrid(20);
    const { sr } = truncationParts();
    expect(sr.textContent).toBe('표시 상한을 넘어 8개 시리즈가 생략되었습니다.');
    // 보간 슬롯이 남아 있으면 개수가 전달되지 않은 것이다.
    expect(sr.textContent).not.toContain('{count}');
    // sr-only 는 시각적으로만 숨기며 접근성 트리에서는 살아 있어야 한다.
    expect(sr).not.toHaveAttribute('aria-hidden');
  });

  it('상한을 재정의해도 접근성 문장의 개수가 실제 잘린 수와 일치한다', () => {
    renderGrid(20, 3);
    const { visual, sr } = truncationParts();
    expect(visual.textContent).toBe('+17');
    expect(sr.textContent).toContain('17개');
  });
});
