// KeyValueMapEditor 의 opt-in 강화(pathHelper 칩, 커스텀 라벨/placeholder) 테스트.
//
// storage-write 노드의 tag/field 값에 $. JSONPath 참조를 쉽게 입력할 수 있도록
// 추가된 기능을 검증한다. 모든 신규 동작은 opt-in 이며, prop 미지정 시
// 기존 "키"/"값" 기본 동작이 그대로 유지되어야 한다(다른 노드 무회귀).

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { KeyValueMapEditor } from './KeyValueMapEditor';

// i18n: 실제 ko 번역을 반환하는 mock — 컴포넌트가 useTranslation 을 쓰지만
// 이 테스트는 I18nProvider 로 감싸지 않으므로, ko.json 을 점 표기 키로 해석해
// 기존 한국어 단언을 그대로 통과시킨다.
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

describe('KeyValueMapEditor 기본(opt-in 미사용) 동작', () => {
  it('prop 미지정 시 "키"/"값" 헤더와 placeholder 를 사용한다', () => {
    render(<KeyValueMapEditor value={{ a: '1' }} onChange={vi.fn()} />);

    // 헤더
    expect(screen.getByRole('columnheader', { name: '키' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: '값' })).toBeInTheDocument();

    // placeholder
    expect(screen.getByPlaceholderText('키')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('값')).toBeInTheDocument();
  });

  it('pathHelper 미지정 시 빠른 삽입 칩을 렌더링하지 않는다', () => {
    render(<KeyValueMapEditor value={{ a: '1' }} onChange={vi.fn()} />);
    expect(screen.queryByTestId('path-helper-chips')).not.toBeInTheDocument();
    expect(screen.queryByText('빠른 삽입:')).not.toBeInTheDocument();
  });
});

describe('KeyValueMapEditor 커스텀 라벨/placeholder', () => {
  it('keyLabel/valueLabel 이 컬럼 헤더에 적용된다', () => {
    render(
      <KeyValueMapEditor
        value={{ region: '$.payload.region' }}
        onChange={vi.fn()}
        keyLabel="태그 이름"
        valueLabel="값 ($. JSONPath / metadata 키)"
      />,
    );
    expect(screen.getByRole('columnheader', { name: '태그 이름' })).toBeInTheDocument();
    expect(
      screen.getByRole('columnheader', { name: '값 ($. JSONPath / metadata 키)' }),
    ).toBeInTheDocument();
  });

  it('keyPlaceholder/valuePlaceholder 가 입력 placeholder 에 적용된다', () => {
    render(
      <KeyValueMapEditor
        value={{ '': '' }}
        onChange={vi.fn()}
        keyPlaceholder="태그 키"
        valuePlaceholder="$.payload.temperature"
      />,
    );
    expect(screen.getByPlaceholderText('태그 키')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('$.payload.temperature')).toBeInTheDocument();
  });
});

describe('KeyValueMapEditor pathHelper 칩 렌더링', () => {
  it('pathHelper=true 이면 4개의 $. 칩과 "빠른 삽입:" 힌트를 렌더링한다', () => {
    render(<KeyValueMapEditor value={{ a: '' }} onChange={vi.fn()} pathHelper />);

    const chipContainer = screen.getByTestId('path-helper-chips');
    expect(within(chipContainer).getByText('빠른 삽입:')).toBeInTheDocument();
    expect(within(chipContainer).getByRole('button', { name: '$.payload.' })).toBeInTheDocument();
    expect(within(chipContainer).getByRole('button', { name: '$.metadata.' })).toBeInTheDocument();
    expect(within(chipContainer).getByRole('button', { name: '$.type' })).toBeInTheDocument();
    expect(within(chipContainer).getByRole('button', { name: '$.timestamp' })).toBeInTheDocument();
  });

  it('pathHelper=true 라도 readOnly 면 칩을 렌더링하지 않는다', () => {
    render(<KeyValueMapEditor value={{ a: '' }} onChange={vi.fn()} pathHelper readOnly />);
    expect(screen.queryByTestId('path-helper-chips')).not.toBeInTheDocument();
  });
});

describe('KeyValueMapEditor pathHelper 칩 삽입', () => {
  it('포커스된 값 셀에 칩 텍스트를 삽입하고 onChange 를 호출한다', () => {
    const onChange = vi.fn();
    render(<KeyValueMapEditor value={{ region: '' }} onChange={onChange} pathHelper />);

    // 값 input 포커스
    const valueInput = screen.getByPlaceholderText('값') as HTMLInputElement;
    fireEvent.focus(valueInput);

    // $.payload. 칩 클릭
    fireEvent.click(screen.getByRole('button', { name: '$.payload.' }));

    // 마지막 onChange 인자에 삽입된 값이 반영되어야 한다.
    const lastCall = onChange.mock.calls.at(-1)?.[0] as Record<string, string>;
    expect(lastCall).toEqual({ region: '$.payload.' });
  });

  it('caret 위치(selectionStart)에 삽입한다', () => {
    const onChange = vi.fn();
    render(<KeyValueMapEditor value={{ region: 'abc' }} onChange={onChange} pathHelper />);

    const valueInput = screen.getByDisplayValue('abc') as HTMLInputElement;
    fireEvent.focus(valueInput);
    // caret 을 1번 위치(a 다음)로 이동.
    valueInput.setSelectionRange(1, 1);

    fireEvent.click(screen.getByRole('button', { name: '$.type' }));

    const lastCall = onChange.mock.calls.at(-1)?.[0] as Record<string, string>;
    expect(lastCall).toEqual({ region: 'a$.typebc' });
  });

  it('포커스된 셀이 없으면 마지막 행의 값에 append 한다', () => {
    const onChange = vi.fn();
    render(
      <KeyValueMapEditor value={{ a: 'x', b: 'y' }} onChange={onChange} pathHelper />,
    );

    // 아무 input 도 포커스하지 않고 바로 칩 클릭 → 마지막 행(b) 값에 append.
    fireEvent.click(screen.getByRole('button', { name: '$.metadata.' }));

    const lastCall = onChange.mock.calls.at(-1)?.[0] as Record<string, string>;
    expect(lastCall).toEqual({ a: 'x', b: 'y$.metadata.' });
  });

  it('행이 없으면 칩 텍스트를 값으로 갖는 새 행을 추가한다', () => {
    const onChange = vi.fn();
    render(<KeyValueMapEditor value={{}} onChange={onChange} pathHelper />);

    fireEvent.click(screen.getByRole('button', { name: '$.timestamp' }));

    const lastCall = onChange.mock.calls.at(-1)?.[0] as Record<string, string>;
    // 키가 빈 문자열인 새 행, 값은 칩 텍스트.
    expect(lastCall).toEqual({ '': '$.timestamp' });
  });
});
