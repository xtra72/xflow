// PromoteToStaticDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음 / true 일 때 렌더링됨
//   - 키 이름이 읽기 전용으로 표시됨
//   - Esc / 배경 / 취소 버튼으로 모달 닫힘
//   - data_type 미선택 시 변환 버튼 비활성 + tooltip 표시 (M14)
//   - data_type 선택 시 변환 버튼 활성, onConfirm 페이로드에 포함됨 (M14)
//   - metric_type 선택 입력 — 빈 값 통과, 정규식 위반 시 에러 + 비활성 (M14)
//   - defaultDataType prop 전달 시 셀렉트 사전 채움 (M14)
//   - 태그 추가/삭제 → onConfirm 페이로드의 tags 필드에 반영
//   - 잘못된 형식의 태그 키 입력 시 경고 표시 + 변환 버튼 비활성화
//   - isSubmitting=true 일 때 변환 버튼 비활성 + 스피너 + 취소/닫기 비활성
//
// @spec SPEC-STORE-003 v0.3.0
// @spec SPEC-WEB-005 v0.7.0 (M14)

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { PromoteToStaticDialog } from './PromoteToStaticDialog';

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

describe('PromoteToStaticDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <PromoteToStaticDialog
        isOpen={false}
        onClose={vi.fn()}
        keyName="indoor:1:room_temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('isOpen=true 면 모달과 키 이름, data_type/metric_type 입력이 표시된다', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="indoor:1:room_temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('정적 키로 변환')).toBeInTheDocument();
    expect(screen.getByText('indoor:1:room_temp')).toBeInTheDocument();
    // M14: data_type / metric_type 입력 필드 존재
    expect(screen.getByLabelText(/데이터 타입/)).toBeInTheDocument();
    expect(screen.getByLabelText(/메트릭 타입/)).toBeInTheDocument();
    // 초기 상태: 태그 행 없음
    expect(screen.getByText('태그가 없습니다')).toBeInTheDocument();
  });

  it('Esc 키로 모달이 닫힌다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('배경 클릭 시 모달이 닫힌다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    // 다이얼로그 요소(배경)를 직접 클릭
    const dialog = screen.getByRole('dialog');
    fireEvent.click(dialog);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('취소 버튼 클릭 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '취소' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // ─────────────────────────────────────────────────────────────────
  // M14: data_type 필수 검증
  // ─────────────────────────────────────────────────────────────────

  it('data_type 미선택 시 변환 버튼이 비활성화된다 (M14)', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    // 인라인 에러 메시지 노출
    expect(
      screen.getByText('data_type 은 manual 모드에서 필수입니다'),
    ).toBeInTheDocument();
  });

  it('data_type 미선택 상태에서 변환 클릭은 무시된다 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('data_type 선택 후 변환 → onConfirm 페이로드에 data_type 포함 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    // data_type = float 선택
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'float' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).toHaveBeenCalledWith({
      data_type: 'float',
      tags: {},
    });
  });

  it('defaultDataType prop 전달 시 셀렉트가 사전 채워진다 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
        defaultDataType="boolean"
      />,
    );
    const select = screen.getByLabelText(/데이터 타입/) as HTMLSelectElement;
    expect(select.value).toBe('boolean');
    // 변환 버튼이 즉시 활성화되어야 한다
    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(false);

    fireEvent.click(confirmBtn);
    expect(onConfirm).toHaveBeenCalledWith({
      data_type: 'boolean',
      tags: {},
    });
  });

  // ─────────────────────────────────────────────────────────────────
  // M14: metric_type 검증
  // ─────────────────────────────────────────────────────────────────

  it('metric_type 빈 값은 페이로드에서 생략된다 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'int' },
    });
    // metric_type 입력 없음
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).toHaveBeenCalledWith({
      data_type: 'int',
      tags: {},
    });
    // metric_type 키 자체가 없어야 함 (백엔드 default 적용)
    const arg = onConfirm.mock.calls[0]?.[0] as Record<string, unknown>;
    expect(arg).not.toHaveProperty('metric_type');
  });

  it('metric_type 정상 값은 페이로드에 포함된다 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'float' },
    });
    fireEvent.change(screen.getByLabelText(/메트릭 타입/), {
      target: { value: 'gauge' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).toHaveBeenCalledWith({
      data_type: 'float',
      metric_type: 'gauge',
      tags: {},
    });
  });

  it('metric_type 정규식 위반 시 에러 표시 + 변환 비활성 (M14)', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'int' },
    });
    // metric_type 에 점(.) 포함 — 정규식 위반
    fireEvent.change(screen.getByLabelText(/메트릭 타입/), {
      target: { value: 'metric.bad' },
    });
    expect(
      screen.getByText('허용되지 않는 문자가 포함되었습니다 (영숫자, _, - 만 허용)'),
    ).toBeInTheDocument();

    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    fireEvent.click(confirmBtn);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  // ─────────────────────────────────────────────────────────────────
  // 태그 편집 (회귀 방지)
  // ─────────────────────────────────────────────────────────────────

  it('태그 추가 → 변환 → onConfirm 이 태그 맵을 포함한다', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    // data_type 필수이므로 먼저 선택
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'string' },
    });

    // 태그 행 추가
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));

    // 키와 값 입력
    const keyInput = screen.getByLabelText('태그 키');
    const valInput = screen.getByLabelText('태그 값');
    fireEvent.change(keyInput, { target: { value: 'room' } });
    fireEvent.change(valInput, { target: { value: 'kitchen' } });

    // 변환 클릭
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).toHaveBeenCalledWith({
      data_type: 'string',
      tags: { room: 'kitchen' },
    });
  });

  it('잘못된 태그 키 형식(예: room.1)은 경고 표시 + 변환 비활성', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'int' },
    });
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    const keyInput = screen.getByLabelText('태그 키');
    const valInput = screen.getByLabelText('태그 값');
    fireEvent.change(keyInput, { target: { value: 'room.1' } });
    fireEvent.change(valInput, { target: { value: 'kitchen' } });

    expect(
      screen.getByText('태그 키는 영문/숫자/언더스코어/하이픈만 허용됩니다'),
    ).toBeInTheDocument();

    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    fireEvent.click(confirmBtn);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('태그 키만 입력하고 값을 비워두면 변환 버튼이 비활성화된다', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.change(screen.getByLabelText(/데이터 타입/), {
      target: { value: 'int' },
    });
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    const keyInput = screen.getByLabelText('태그 키');
    fireEvent.change(keyInput, { target: { value: 'room' } });
    // 값은 비워둠

    expect(screen.getByText('태그 값을 입력하세요')).toBeInTheDocument();

    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
  });

  it('태그 행 삭제 버튼 클릭 시 행이 제거된다', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    expect(screen.getByLabelText('태그 키')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '태그 삭제' }));
    expect(screen.queryByLabelText('태그 키')).not.toBeInTheDocument();
    // 삭제 후 빈 상태 메시지 다시 표시
    expect(screen.getByText('태그가 없습니다')).toBeInTheDocument();
  });

  // ─────────────────────────────────────────────────────────────────
  // isSubmitting 상태
  // ─────────────────────────────────────────────────────────────────

  it('isSubmitting=true 일 때 변환 버튼 비활성 + 스피너 + 텍스트 변경', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={vi.fn()}
        isSubmitting={true}
        // 검증 통과 상태로 만들어 isSubmitting 만의 효과를 확인
        defaultDataType="int"
      />,
    );
    const confirmBtn = screen.getByRole('button', { name: /변환 중/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    expect(screen.getByText('변환 중...')).toBeInTheDocument();
  });

  it('isSubmitting=true 일 때 Esc 와 배경 클릭이 무시된다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
        isSubmitting={true}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('dialog'));
    expect(onClose).not.toHaveBeenCalled();
  });
});
