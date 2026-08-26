// 직전값 사용 기간 편집기.
//
// 미설정이 종전 동작(계속 이어 씀)이라는 점, 초과 처리는 기간을 정했을 때만
// 뜻이 있다는 점이 핵심이다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { FillPreviousLimitField, type FillPreviousLimitValue } from './FillPreviousLimitField';

function renderField(value: FillPreviousLimitValue = {}) {
  const onChange = vi.fn();
  render(<FillPreviousLimitField value={value} onChange={onChange} testIdPrefix="fp" />);
  return { onChange };
}

describe('직전값 사용 기간', () => {
  it('기본은 제한 없음 — 종전 동작이다', () => {
    renderField();
    expect((screen.getByTestId('fp-max') as HTMLSelectElement).value).toBe('');
  });

  it('기간을 정하기 전에는 초과 처리를 묻지 않는다', () => {
    renderField();
    expect(screen.queryByTestId('fp-overflow')).toBeNull();
  });

  it('프리셋을 고르면 ms 로 저장된다', () => {
    const { onChange } = renderField();
    fireEvent.change(screen.getByTestId('fp-max'), { target: { value: '300000' } });
    expect(onChange).toHaveBeenCalledWith({ maxMs: 300_000 });
  });

  it('제한 없음으로 되돌리면 기간을 지운다', () => {
    const { onChange } = renderField({ maxMs: 300_000 });
    fireEvent.change(screen.getByTestId('fp-max'), { target: { value: '' } });
    expect(onChange).toHaveBeenCalledWith({ maxMs: undefined });
  });

  it('기간을 정하면 초과 처리가 나타나고 기본은 비움이다', () => {
    renderField({ maxMs: 300_000 });
    expect((screen.getByTestId('fp-overflow') as HTMLSelectElement).value).toBe('');
    expect(screen.queryByTestId('fp-overflow-value')).toBeNull();
  });

  it('지정 값 채우기를 고르면 값 입력이 나타난다', () => {
    const { onChange } = renderField({ maxMs: 300_000 });
    fireEvent.change(screen.getByTestId('fp-overflow'), { target: { value: 'value' } });
    expect(onChange).toHaveBeenCalledWith({
      maxMs: 300_000,
      overflow: 'value',
      overflowValue: 0,
    });
  });

  it('비움으로 되돌리면 값도 함께 지운다', () => {
    const { onChange } = renderField({ maxMs: 300_000, overflow: 'value', overflowValue: -1 });
    fireEvent.change(screen.getByTestId('fp-overflow'), { target: { value: '' } });
    expect(onChange).toHaveBeenCalledWith({
      maxMs: 300_000,
      overflow: undefined,
      overflowValue: undefined,
    });
  });

  it('지정 값 0 도 저장된다 — 비움과 다르다', () => {
    const { onChange } = renderField({ maxMs: 300_000, overflow: 'value', overflowValue: -1 });
    fireEvent.change(screen.getByTestId('fp-overflow-value'), { target: { value: '0' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ overflowValue: 0 }));
  });

  it('프리셋에 없는 기간은 직접 입력 칸으로 보여 준다', () => {
    renderField({ maxMs: 90_000 });
    expect((screen.getByTestId('fp-max') as HTMLSelectElement).value).toBe('custom');
    expect((screen.getByTestId('fp-max-custom') as HTMLInputElement).value).toBe('90');
  });
});
