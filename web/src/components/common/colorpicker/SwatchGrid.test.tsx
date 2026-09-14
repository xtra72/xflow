// 색 칸 격자 — **탭 정지 하나**와 그 안의 방향키 이동.
//
// 행마다 길이가 다른 것이 이 격자의 어려운 점이다(무채색 8칸, 색상환 10칸). 짧은 행에서
// 아래로 내려갈 때와 긴 행에서 짧은 행으로 올라올 때를 각각 본다.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import SwatchGrid from './SwatchGrid';

/** 8칸 · 3칸 · 2칸 — 길이가 서로 다른 세 행. */
const ROWS = [
  ['#000000', '#111111', '#222222', '#333333', '#444444', '#555555', '#666666', '#777777'],
  ['#ff0000', '#00ff00', '#0000ff'],
  ['#aa0000', '#00aa00'],
];

function grid(): HTMLElement {
  return screen.getByTestId('grid');
}

function renderGrid(props: Partial<React.ComponentProps<typeof SwatchGrid>> = {}) {
  const onPick = vi.fn();
  const utils = render(
    <SwatchGrid
      rows={ROWS}
      value={undefined}
      onPick={onPick}
      ariaLabel="프리셋"
      testId="grid"
      {...props}
    />,
  );
  return { onPick, ...utils };
}

describe('격자 — 접근성 형상', () => {
  it('목록상자와 항목의 역할을 갖는다', () => {
    renderGrid({ value: '#0000ff' });

    expect(grid()).toHaveAttribute('role', 'listbox');
    expect(grid()).toHaveAttribute('aria-label', '프리셋');
    const option = screen.getByLabelText('#0000ff');
    expect(option).toHaveAttribute('role', 'option');
    expect(option).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByLabelText('#ff0000')).toHaveAttribute('aria-selected', 'false');
  });

  it('격자 전체가 탭 정지 하나다', () => {
    renderGrid({ value: '#0000ff' });

    const stops = Array.from(grid().querySelectorAll<HTMLElement>('*')).filter(
      (el) => el.tabIndex >= 0,
    );
    // 13칸이 전부 탭 정지면 팝오버 하나를 지나는 데 Tab 을 열세 번 눌러야 한다.
    expect(stops).toHaveLength(1);
    // 그 하나는 **현재 값의 칸**이다 — 열자마자 지금 색이 어디 있는지 보인다.
    expect(stops[0]).toBe(screen.getByLabelText('#0000ff'));
  });

  it('값이 격자에 없으면 첫 칸이 탭 정지다', () => {
    renderGrid({ value: '#123456' });

    expect(screen.getByLabelText('#000000').tabIndex).toBe(0);
    expect(screen.queryAllByRole('option', { selected: true })).toHaveLength(0);
  });
});

describe('격자 — 방향키', () => {
  it('→ 로 오른쪽 칸으로 가고 포커스도 따라간다', () => {
    renderGrid({ value: '#000000' });

    fireEvent.keyDown(grid(), { key: 'ArrowRight' });

    expect(screen.getByLabelText('#111111').tabIndex).toBe(0);
    expect(document.activeElement).toBe(screen.getByLabelText('#111111'));
  });

  it('행 끝에서 → 는 멈춘다 — 다음 행으로 감기지 않는다', () => {
    renderGrid({ value: '#0000ff' }); // 2행 마지막 칸

    fireEvent.keyDown(grid(), { key: 'ArrowRight' });

    expect(document.activeElement).toBe(screen.getByLabelText('#0000ff'));
  });

  it('행 처음에서 ← 는 멈춘다', () => {
    renderGrid({ value: '#ff0000' });

    fireEvent.keyDown(grid(), { key: 'ArrowLeft' });

    expect(document.activeElement).toBe(screen.getByLabelText('#ff0000'));
  });

  it('↓ 로 아래 행으로 가고, 그 행이 짧으면 마지막 칸으로 붙는다', () => {
    renderGrid({ value: '#777777' }); // 1행 8번째 칸(col 7)

    fireEvent.keyDown(grid(), { key: 'ArrowDown' });

    // 2행은 3칸뿐이다. col 7 이 그대로 가면 없는 칸을 가리킨다.
    expect(document.activeElement).toBe(screen.getByLabelText('#0000ff'));
  });

  it('↑ 로 위 행으로 돌아간다 — 칸 번호는 유지된다', () => {
    renderGrid({ value: '#00ff00' }); // 2행 col 1

    fireEvent.keyDown(grid(), { key: 'ArrowUp' });

    expect(document.activeElement).toBe(screen.getByLabelText('#111111'));
  });

  it('마지막 행에서 ↓ 는 멈추고 첫 행에서 ↑ 도 멈춘다', () => {
    renderGrid({ value: '#aa0000' }); // 3행

    fireEvent.keyDown(grid(), { key: 'ArrowDown' });
    expect(document.activeElement).toBe(screen.getByLabelText('#aa0000'));

    fireEvent.keyDown(grid(), { key: 'ArrowUp' });
    fireEvent.keyDown(grid(), { key: 'ArrowUp' });
    fireEvent.keyDown(grid(), { key: 'ArrowUp' });
    expect(document.activeElement).toBe(screen.getByLabelText('#000000'));
  });

  it('방향키가 아닌 키는 커서를 움직이지 않는다', () => {
    renderGrid({ value: '#000000' });

    fireEvent.keyDown(grid(), { key: 'Tab' });
    fireEvent.keyDown(grid(), { key: 'a' });

    expect(screen.getByLabelText('#000000').tabIndex).toBe(0);
  });
});

describe('격자 — 고르기', () => {
  it('Enter 로 커서 자리의 색을 낸다', () => {
    const { onPick } = renderGrid({ value: '#000000' });

    fireEvent.keyDown(grid(), { key: 'ArrowDown' });
    fireEvent.keyDown(grid(), { key: 'Enter' });

    expect(onPick).toHaveBeenCalledExactlyOnceWith('#ff0000');
  });

  it('Space 로도 고른다', () => {
    const { onPick } = renderGrid({ value: '#111111' });

    fireEvent.keyDown(grid(), { key: ' ' });

    expect(onPick).toHaveBeenCalledExactlyOnceWith('#111111');
  });

  it('눌러서 고른다', () => {
    const { onPick } = renderGrid();

    fireEvent.click(screen.getByLabelText('#00aa00'));

    expect(onPick).toHaveBeenCalledExactlyOnceWith('#00aa00');
  });
});

describe('격자 — 칸이 없는 경우', () => {
  it('빈 격자에서 방향키·Enter 가 아무 일도 하지 않는다', () => {
    const { onPick } = renderGrid({ rows: [] });

    fireEvent.keyDown(grid(), { key: 'ArrowRight' });
    fireEvent.keyDown(grid(), { key: 'Enter' });

    expect(onPick).not.toHaveBeenCalled();
  });

  it('빈 행 하나짜리 격자도 마찬가지다', () => {
    const { onPick } = renderGrid({ rows: [[]] });

    fireEvent.keyDown(grid(), { key: 'ArrowDown' });
    fireEvent.keyDown(grid(), { key: ' ' });

    expect(onPick).not.toHaveBeenCalled();
  });
});
