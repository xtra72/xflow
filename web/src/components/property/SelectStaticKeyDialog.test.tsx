// SelectStaticKeyDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음
//   - 자동 검색 후보 키 목록 + data_type 뱃지 표시
//   - 후보 클릭 시 onSelect(key) 호출
//   - 검색어로 후보 필터링
//   - 목록에 없는 키를 입력하면 "직접 등록"(수동) 행 노출 + 선택 시 onSelect
//   - 후보가 없고 검색어도 없으면 빈 상태 메시지
//
// @spec SPEC-STORE-003 v0.3.0
// @spec SPEC-WEB-005

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import SelectStaticKeyDialog from './SelectStaticKeyDialog';
import type { StoreKeyObject } from '@/services/api/store';

// i18n: ko.json 을 점 표기 키로 해석하는 mock (I18nProvider 없이 렌더).
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

const candidates: StoreKeyObject[] = [
  {
    key: 'indoor:1:room_temp',
    registration: 'auto',
    data_type: 'float',
    metric_type: 'gauge',
    tags: { room: '1' },
  },
  {
    key: 'indoor:1:power',
    registration: 'auto',
    data_type: 'boolean',
    metric_type: 'unknown',
    tags: {},
  },
];

describe('SelectStaticKeyDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <SelectStaticKeyDialog
        isOpen={false}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('후보 키 목록과 data_type 뱃지를 표시한다', () => {
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByTestId('select-static-key-candidate-indoor:1:room_temp')).toBeInTheDocument();
    expect(screen.getByTestId('select-static-key-candidate-indoor:1:power')).toBeInTheDocument();
    expect(screen.getByText('float')).toBeInTheDocument();
    expect(screen.getByText('boolean')).toBeInTheDocument();
  });

  it('후보 클릭 시 onSelect(key) 를 호출한다', () => {
    const onSelect = vi.fn();
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={onSelect}
      />,
    );
    fireEvent.click(screen.getByTestId('select-static-key-candidate-indoor:1:power'));
    expect(onSelect).toHaveBeenCalledWith('indoor:1:power');
  });

  it('검색어로 후보를 필터링한다', () => {
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByTestId('select-static-key-input'), {
      target: { value: 'power' },
    });
    expect(screen.getByTestId('select-static-key-candidate-indoor:1:power')).toBeInTheDocument();
    expect(
      screen.queryByTestId('select-static-key-candidate-indoor:1:room_temp'),
    ).not.toBeInTheDocument();
  });

  it('목록에 없는 키 입력 시 수동 등록 행을 노출하고 선택하면 onSelect 한다', () => {
    const onSelect = vi.fn();
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={onSelect}
      />,
    );
    fireEvent.change(screen.getByTestId('select-static-key-input'), {
      target: { value: 'brand:new:key' },
    });
    const manual = screen.getByTestId('select-static-key-manual');
    expect(manual).toBeInTheDocument();
    fireEvent.click(manual);
    expect(onSelect).toHaveBeenCalledWith('brand:new:key');
  });

  it('Enter 로 입력한 키를 선택한다', () => {
    const onSelect = vi.fn();
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={onSelect}
      />,
    );
    const input = screen.getByTestId('select-static-key-input');
    fireEvent.change(input, { target: { value: 'typed:key' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onSelect).toHaveBeenCalledWith('typed:key');
  });

  it('후보가 없고 검색어도 없으면 빈 상태 메시지를 표시한다', () => {
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={[]}
        onSelect={vi.fn()}
      />,
    );
    expect(
      screen.getByText('등록 가능한 자동 키가 없습니다. 키 이름을 입력해 직접 등록하세요.'),
    ).toBeInTheDocument();
  });

  it('정확히 일치하는 후보가 있으면 수동 행을 노출하지 않는다', () => {
    render(
      <SelectStaticKeyDialog
        isOpen={true}
        onClose={vi.fn()}
        candidates={candidates}
        onSelect={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByTestId('select-static-key-input'), {
      target: { value: 'indoor:1:power' },
    });
    expect(screen.queryByTestId('select-static-key-manual')).not.toBeInTheDocument();
  });
});
