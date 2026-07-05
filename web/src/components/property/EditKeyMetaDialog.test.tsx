// EditKeyMetaDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음 / true 일 때 렌더링됨
//   - 키 이름이 읽기 전용으로 표시됨
//   - initialMetricType / initialTags 사전 채움
//   - metric_type 정규식 위반 시 에러 + 저장 버튼 비활성
//   - 태그 전체 교체: 기존 태그 삭제 + 신규 추가 후 onConfirm 페이로드 반영
//   - Esc / 취소 버튼으로 닫힘
//   - isSubmitting=true 일 때 저장 버튼 비활성 + 스피너
//
// @spec SPEC-STORE-003 v0.4.0

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { EditKeyMetaDialog } from './EditKeyMetaDialog';

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

describe('EditKeyMetaDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <EditKeyMetaDialog
        isOpen={false}
        onClose={vi.fn()}
        keyName="indoor:1:temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('isOpen=true 면 키 이름과 metric_type 입력이 표시된다', () => {
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="indoor:1:temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('타입 / 태그 편집')).toBeInTheDocument();
    expect(screen.getByText('indoor:1:temp')).toBeInTheDocument();
    expect(screen.getByLabelText(/메트릭 타입/)).toBeInTheDocument();
    expect(screen.getByText('태그가 없습니다')).toBeInTheDocument();
  });

  it('initialMetricType / initialTags 를 사전 채움한다', () => {
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        initialMetricType="gauge"
        initialTags={{ room: '1', floor: '3' }}
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByLabelText(/메트릭 타입/)).toHaveValue('gauge');
    const keyInputs = screen.getAllByLabelText('태그 키');
    expect(keyInputs).toHaveLength(2);
    const values = keyInputs.map((el) => (el as HTMLInputElement).value).sort();
    expect(values).toEqual(['floor', 'room']);
  });

  it('metric_type 정규식 위반 시 저장 버튼이 비활성된다', () => {
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        onConfirm={vi.fn()}
      />,
    );
    const metricInput = screen.getByLabelText(/메트릭 타입/);
    fireEvent.change(metricInput, { target: { value: 'bad type!' } });
    expect(screen.getByTestId('edit-key-meta-confirm')).toBeDisabled();
    expect(
      screen.getByText(/허용되지 않는 문자가 포함/),
    ).toBeInTheDocument();
  });

  it('저장 시 metric_type 과 태그 전체를 onConfirm 으로 전달한다', () => {
    const onConfirm = vi.fn();
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        initialMetricType="gauge"
        initialTags={{ room: '1' }}
        onConfirm={onConfirm}
      />,
    );
    // metric_type 변경
    fireEvent.change(screen.getByLabelText(/메트릭 타입/), {
      target: { value: 'temperature' },
    });
    // 신규 태그 행 추가 후 입력
    fireEvent.click(screen.getByText('태그 추가'));
    const keyInputs = screen.getAllByLabelText('태그 키');
    const valInputs = screen.getAllByLabelText('태그 값');
    // 마지막 행이 신규 행
    fireEvent.change(keyInputs[keyInputs.length - 1]!, {
      target: { value: 'floor' },
    });
    fireEvent.change(valInputs[valInputs.length - 1]!, {
      target: { value: '3' },
    });

    fireEvent.click(screen.getByTestId('edit-key-meta-confirm'));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledWith({
      metric_type: 'temperature',
      tags: { room: '1', floor: '3' },
    });
  });

  it('metric_type 을 비우면 payload 에서 생략된다 (전체 교체 tags 만 전송)', () => {
    const onConfirm = vi.fn();
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        initialMetricType="gauge"
        initialTags={{ room: '1' }}
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/메트릭 타입/), {
      target: { value: '' },
    });
    fireEvent.click(screen.getByTestId('edit-key-meta-confirm'));
    expect(onConfirm).toHaveBeenCalledWith({ tags: { room: '1' } });
  });

  it('기존 태그를 삭제하면 전체 교체 payload 에서 빠진다', () => {
    const onConfirm = vi.fn();
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        initialTags={{ room: '1', floor: '3' }}
        onConfirm={onConfirm}
      />,
    );
    // 첫 번째 태그 삭제
    const removeButtons = screen.getAllByLabelText('태그 삭제');
    fireEvent.click(removeButtons[0]!);
    fireEvent.click(screen.getByTestId('edit-key-meta-confirm'));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    const payload = onConfirm.mock.calls[0]![0] as {
      tags: Record<string, string>;
    };
    expect(Object.keys(payload.tags)).toHaveLength(1);
  });

  it('Esc 키로 모달이 닫힌다', () => {
    const onClose = vi.fn();
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={onClose}
        keyName="k"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('isSubmitting=true 면 저장 버튼이 비활성되고 라벨이 변경된다', () => {
    render(
      <EditKeyMetaDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="k"
        onConfirm={vi.fn()}
        isSubmitting={true}
      />,
    );
    const confirm = screen.getByTestId('edit-key-meta-confirm');
    expect(confirm).toBeDisabled();
    expect(confirm).toHaveTextContent('저장 중...');
  });
});
