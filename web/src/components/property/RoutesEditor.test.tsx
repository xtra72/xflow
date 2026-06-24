// RoutesEditor 의 구조화 라우트 편집 동작 테스트 (SPEC-SWITCH-001).
// 결과는 순서 보존된 Array<{ name, condition }> 이어야 한다(백엔드 first-match 의존).

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { RoutesEditor } from './RoutesEditor';

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

describe('RoutesEditor 렌더링 (AC-SWITCH-040)', () => {
  it('초기값 배열을 (조건식, 출력 포트명) 행으로 렌더링한다', () => {
    render(
      <RoutesEditor
        value={[
          { name: 'hot', condition: '$.payload.temp >= 30' },
          { name: 'cold', condition: '$.payload.temp < 10' },
        ]}
        onChange={vi.fn()}
      />,
    );

    // 한국어 헤더 라벨
    expect(screen.getByText('조건식')).toBeInTheDocument();
    expect(screen.getByText('출력 포트명')).toBeInTheDocument();

    // 값이 입력에 채워진다
    expect(screen.getByDisplayValue('$.payload.temp >= 30')).toBeInTheDocument();
    expect(screen.getByDisplayValue('hot')).toBeInTheDocument();
    expect(screen.getByDisplayValue('cold')).toBeInTheDocument();
  });

  it('빈 값이면 안내 문구를 표시한다', () => {
    render(<RoutesEditor value={[]} onChange={vi.fn()} />);
    expect(screen.getByText(/라우트 없음/)).toBeInTheDocument();
  });
});

describe('RoutesEditor 행 추가/삭제 (AC-SWITCH-041)', () => {
  it('"라우트 추가" 클릭 시 행이 추가된다', () => {
    const onChange = vi.fn();
    render(<RoutesEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '라우트 추가' }));

    // 빈 행이 1개 추가됨 — 입력 위젯이 나타난다
    expect(screen.getByLabelText('조건식')).toBeInTheDocument();
    expect(screen.getByLabelText('출력 포트명')).toBeInTheDocument();
  });

  it('조건식/포트명 입력 시 {name, condition} 객체 배열을 순서대로 emit 한다', () => {
    const onChange = vi.fn();
    render(
      <RoutesEditor value={[{ name: '', condition: '' }]} onChange={onChange} />,
    );

    fireEvent.change(screen.getByLabelText('조건식'), {
      target: { value: '$.payload.temp >= 30' },
    });
    fireEvent.change(screen.getByLabelText('출력 포트명'), {
      target: { value: 'hot' },
    });

    // 마지막 emit 은 name/condition 키를 가진 배열
    const last = onChange.mock.calls.at(-1)?.[0];
    expect(last).toEqual([{ name: 'hot', condition: '$.payload.temp >= 30' }]);
  });

  it('삭제 클릭 시 해당 행이 제거된다', () => {
    const onChange = vi.fn();
    render(
      <RoutesEditor
        value={[
          { name: 'hot', condition: 'a' },
          { name: 'cold', condition: 'b' },
        ]}
        onChange={onChange}
      />,
    );

    // 첫 행 삭제
    const [firstDeleteButton] = screen.getAllByRole('button', { name: '삭제' });
    expect(firstDeleteButton).toBeDefined();
    fireEvent.click(firstDeleteButton as HTMLElement);

    const last = onChange.mock.calls.at(-1)?.[0];
    expect(last).toEqual([{ name: 'cold', condition: 'b' }]);
  });
});

describe('RoutesEditor 순서 보존', () => {
  it('여러 행의 순서를 입력 순서대로 유지한다', () => {
    const onChange = vi.fn();
    const value = [
      { name: 'warm', condition: '$.payload.temp >= 20' },
      { name: 'hot', condition: '$.payload.temp >= 30' },
      { name: 'cold', condition: '$.payload.temp < 10' },
    ];
    render(<RoutesEditor value={value} onChange={onChange} />);

    // 마지막 포트명을 수정하면 전체 배열이 같은 순서로 emit 된다
    const nameInputs = screen.getAllByLabelText('출력 포트명');
    const thirdNameInput = nameInputs[2];
    expect(thirdNameInput).toBeDefined();
    fireEvent.change(thirdNameInput as HTMLElement, { target: { value: 'freezing' } });

    const last = onChange.mock.calls.at(-1)?.[0] as Array<{ name: string }>;
    expect(last.map((r) => r.name)).toEqual(['warm', 'hot', 'freezing']);
  });
});

describe('RoutesEditor 읽기 전용', () => {
  it('readOnly 면 추가/삭제 버튼을 숨긴다', () => {
    render(
      <RoutesEditor
        value={[{ name: 'hot', condition: 'a' }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '라우트 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '삭제' })).not.toBeInTheDocument();
  });
});
