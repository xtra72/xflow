// 시리즈 이름 형식 입력의 레이아웃·토큰 도움말 테스트.
//
// 토큰 목록은 입력 아래 상시 펼침 줄이었다가, 시리즈 세부 설정과 같은 규약으로
// 입력 뒤 물음표 도움말이 됐다. 미리보기는 그 줄에서 빠져 입력 바로 뒤로 왔다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { SeriesNameFormatField } from './SeriesNameFormatField';

const sample = { key: 'temp', field: 'value', tags: { location: '실습실', spot: 'A1' } };

function renderField(value?: string) {
  const onChange = vi.fn();
  render(
    <SeriesNameFormatField
      value={value}
      onChange={onChange}
      sample={sample}
      testIdPrefix="fmt"
    />,
  );
  return { onChange };
}

describe('이름 형식 — 토큰 도움말', () => {
  it('기본 상태에서는 토큰 목록이 보이지 않는다', () => {
    renderField();

    expect(screen.getByTestId('fmt-token-help')).toBeTruthy();
    expect(screen.queryByTestId('fmt-tokens')).toBeNull();
    expect(screen.queryByTestId('fmt-token-measurement')).toBeNull();
  });

  it('물음표를 누르면 토큰이 열린다', () => {
    renderField();

    fireEvent.click(screen.getByTestId('fmt-token-help'));

    expect(screen.getByTestId('fmt-token-measurement').textContent).toBe('{$.measurement}');
    expect(screen.getByTestId('fmt-token-field')).toBeTruthy();
    expect(screen.getByTestId('fmt-token-location')).toBeTruthy();
  });

  it('토큰을 누르면 형식에 삽입된다', () => {
    const { onChange } = renderField();

    fireEvent.click(screen.getByTestId('fmt-token-help'));
    fireEvent.click(screen.getByTestId('fmt-token-location'));

    expect(onChange).toHaveBeenCalledWith('{$.location}');
  });

  it('표본이 없으면 물음표를 노출하지 않는다', () => {
    render(
      <SeriesNameFormatField
        value={undefined}
        onChange={vi.fn()}
        sample={undefined}
        testIdPrefix="fmt"
      />,
    );

    expect(screen.queryByTestId('fmt-token-help')).toBeNull();
  });
});

describe('이름 형식 — 한 줄 배치', () => {
  it('입력·물음표·미리보기가 같은 wrap 행의 형제다', () => {
    renderField('{$.location}-{$.spot}');

    const input = screen.getByTestId('fmt-input');
    const preview = screen.getByTestId('fmt-preview');
    const help = screen.getByTestId('fmt-token-help');

    const row = input.parentElement!;
    expect(preview.parentElement).toBe(row);
    // 물음표는 팝오버 컨테이너(span) 안에 있으므로 그 컨테이너가 형제다.
    expect(help.parentElement?.parentElement).toBe(row);
    expect(row.className).toContain('flex-wrap');
  });

  it('미리보기가 입력 뒤에 온다', () => {
    renderField('{$.location}');

    const input = screen.getByTestId('fmt-input');
    const preview = screen.getByTestId('fmt-preview');

    expect(
      input.compareDocumentPosition(preview) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(preview.textContent).toContain('실습실');
  });

  it('입력 폭은 고정이다 — w-full 이면 미리보기가 아랫줄로 밀린다', () => {
    renderField();

    const input = screen.getByTestId('fmt-input');
    // classList 로 본다 — className 문자열은 max-w-full 때문에 'w-full' 을 포함한다.
    expect(input.classList.contains('w-full')).toBe(false);
    expect(input.classList.contains('w-56')).toBe(true);
  });

  it('형식이 비면 미리보기를 내지 않는다', () => {
    renderField(undefined);

    expect(screen.queryByTestId('fmt-preview')).toBeNull();
  });
});
