// StoreKeysEditor 컴포넌트 테스트.
//
// 검증 대상:
//   - 초기 value 를 행으로 렌더링 (data_type / metric_type 컬럼 포함)
//   - "행 추가" 버튼 → 빈 행 추가 + onChange 호출
//   - 행 삭제 버튼 → 행 제거 + onChange 호출
//   - 태그 추가 → key/value 입력 + "태그 추가" 버튼 → onChange 호출
//   - 태그 칩 X 클릭 → 태그 제거
//   - 중복 키 경고 (soft warning, blocking 아님)
//   - readOnly 모드: 입력 비활성, 추가/삭제 버튼 숨김
//   - 잘못된 형식(비-JSON 배열 등) 은 빈 목록으로 폴백
//   - v0.7.0 (M13): data_type 셀렉트 컬럼 (6종 enum, manual 모드 필수)
//   - v0.7.0 (M13): metric_type 텍스트 컬럼 (정규식 검증)
//   - v0.7.0 (M13): registrationType prop + onValidityChange 콜백
//
// @spec SPEC-WEB-005 v0.7.0 (M13)
// @spec SPEC-STORE-003 v0.3.0

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { StoreKeysEditor, type StoreKeyEntry } from './StoreKeysEditor';

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

describe('StoreKeysEditor', () => {
  it('초기 value 가 없으면 "정적 키가 없습니다" 표시', () => {
    render(<StoreKeysEditor value={undefined} onChange={vi.fn()} />);
    expect(screen.getByText('정적 키가 없습니다')).toBeInTheDocument();
  });

  it('배열이 아닌 value 는 빈 목록으로 취급', () => {
    render(<StoreKeysEditor value={{ not: 'array' }} onChange={vi.fn()} />);
    expect(screen.getByText('정적 키가 없습니다')).toBeInTheDocument();
  });

  it('초기 배열 value 의 각 엔트리를 행으로 렌더링', () => {
    const value: StoreKeyEntry[] = [
      {
        key: 'indoor/1/temp',
        data_type: 'float',
        metric_type: 'temperature',
        tags: { room: '1', type: 'temperature' },
      },
      { key: 'indoor/2/temp', data_type: 'float', tags: { room: '2' } },
    ];
    render(<StoreKeysEditor value={value} onChange={vi.fn()} />);
    expect(screen.getByDisplayValue('indoor/1/temp')).toBeInTheDocument();
    expect(screen.getByDisplayValue('indoor/2/temp')).toBeInTheDocument();
    // 태그 칩 텍스트
    expect(screen.getByText('room=1')).toBeInTheDocument();
    expect(screen.getByText('type=temperature')).toBeInTheDocument();
    expect(screen.getByText('room=2')).toBeInTheDocument();
    // metric_type 입력값 — 첫 번째 행만 명시적으로 설정됨
    expect(screen.getByDisplayValue('temperature')).toBeInTheDocument();
  });

  it('"행 추가" 버튼 클릭 시 빈 행을 추가하고 onChange 호출', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: /행 추가/ }));
    // 새 행은 data_type / metric_type 모두 빈 문자열 → entry 에서 생략됨.
    expect(onChange).toHaveBeenCalledWith([
      { key: 'a', tags: {} },
      { key: '', tags: {} },
    ]);
  });

  it('행 삭제 버튼 클릭 시 해당 행을 제거하고 onChange 호출', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [
      { key: 'a', tags: {} },
      { key: 'b', tags: {} },
    ];
    render(<StoreKeysEditor value={value} onChange={onChange} />);
    // 첫 번째 행의 삭제 버튼 클릭
    const deleteButtons = screen.getAllByRole('button', { name: '행 삭제' });
    fireEvent.click(deleteButtons[0]!);
    expect(onChange).toHaveBeenCalledWith([{ key: 'b', tags: {} }]);
  });

  it('키 입력 필드 변경 시 onChange 호출 (key 갱신)', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'old', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);
    const input = screen.getByDisplayValue('old') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'new' } });
    expect(onChange).toHaveBeenCalledWith([{ key: 'new', tags: {} }]);
  });

  it('태그 추가: 키 입력 + "태그 추가" 버튼 클릭 → 새 태그 emit', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);

    const keyInput = screen.getByLabelText('태그 키') as HTMLInputElement;
    const valInput = screen.getByLabelText('태그 값') as HTMLInputElement;
    fireEvent.change(keyInput, { target: { value: 'room' } });
    fireEvent.change(valInput, { target: { value: '1' } });
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));

    expect(onChange).toHaveBeenCalledWith([{ key: 'a', tags: { room: '1' } }]);
  });

  it('태그 키가 비어있으면 "태그 추가" 는 no-op', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);

    const addTagBtn = screen.getByRole('button', { name: /태그 추가/ }) as HTMLButtonElement;
    expect(addTagBtn.disabled).toBe(true);
    fireEvent.click(addTagBtn);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('Enter 키로 태그 추가 가능', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);

    const keyInput = screen.getByLabelText('태그 키') as HTMLInputElement;
    const valInput = screen.getByLabelText('태그 값') as HTMLInputElement;
    fireEvent.change(keyInput, { target: { value: 'type' } });
    fireEvent.change(valInput, { target: { value: 'temp' } });
    fireEvent.keyDown(valInput, { key: 'Enter' });

    expect(onChange).toHaveBeenCalledWith([{ key: 'a', tags: { type: 'temp' } }]);
  });

  it('태그 칩의 X 버튼 클릭 시 해당 태그 제거 + onChange 호출', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: { room: '1', type: 'temp' } }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);

    // "room 태그 삭제" aria-label 을 가진 버튼 클릭
    fireEvent.click(screen.getByRole('button', { name: 'room 태그 삭제' }));
    expect(onChange).toHaveBeenCalledWith([{ key: 'a', tags: { type: 'temp' } }]);
  });

  it('중복 키는 경고 메시지를 표시 (soft warning, 저장 차단 없음)', () => {
    render(
      <StoreKeysEditor
        value={[
          { key: 'dup', tags: {} },
          { key: 'dup', tags: {} },
        ]}
        onChange={vi.fn()}
      />,
    );
    // "중복된 키입니다" 메시지가 두 행 모두에 표시돼야 한다 (양쪽 다 하이라이트)
    const warnings = screen.getAllByText('중복된 키입니다');
    expect(warnings.length).toBe(2);
  });

  it('빈 키는 중복 검사 대상에서 제외', () => {
    // 빈 키가 두 개 있어도 "중복" 경고는 표시되지 않아야 함.
    render(
      <StoreKeysEditor
        value={[
          { key: '', tags: {} },
          { key: '', tags: {} },
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.queryByText('중복된 키입니다')).not.toBeInTheDocument();
  });

  it('readOnly 모드: 입력 비활성 + "행 추가" / 삭제 / "태그 추가" 버튼 숨김', () => {
    render(
      <StoreKeysEditor
        value={[{ key: 'a', tags: { room: '1' } }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const keyInput = screen.getByDisplayValue('a') as HTMLInputElement;
    // readOnly 모드에서는 disabled 가 아닌 readOnly attr 를 사용한다.
    // 이유: disabled 는 브라우저가 텍스트를 흐리게 렌더링하여
    // 다크모드에서 값이 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 참조).
    expect(keyInput.readOnly).toBe(true);
    expect(keyInput.disabled).toBe(false);
    expect(screen.queryByRole('button', { name: /행 추가/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '행 삭제' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /태그 추가/ })).not.toBeInTheDocument();
  });

  it('readOnly=true 인 키 input 은 readOnly attr 만 가지며 disabled 가 아니어야 한다', () => {
    // 회귀 방지: disabled 가 다시 추가되면 다크모드에서 값이 보이지 않는다.
    render(
      <StoreKeysEditor
        value={[{ key: 'k1', tags: {} }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const input = screen.getByDisplayValue('k1') as HTMLInputElement;
    expect(input.readOnly).toBe(true);
    expect(input.disabled).toBe(false);
  });

  it('readOnly=true 일 때 input 의 value 가 정확히 표시된다', () => {
    // FormField 의 가시성 수정 (commit b4ad829) 과 동일한 회귀 가드.
    render(
      <StoreKeysEditor
        value={[{ key: 'indoor/1/temperature', tags: {} }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const input = screen.getByDisplayValue('indoor/1/temperature') as HTMLInputElement;
    expect(input.value).toBe('indoor/1/temperature');
  });

  it('readOnly=true 일 때 행 추가 버튼이 렌더되지 않는다', () => {
    render(
      <StoreKeysEditor
        value={[{ key: 'a', tags: {} }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: /행 추가/ })).not.toBeInTheDocument();
  });

  it('readOnly=true 일 때 태그 추가 폼이 렌더되지 않는다', () => {
    render(
      <StoreKeysEditor
        value={[{ key: 'a', tags: { room: '1' } }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    // 태그 키/값 입력란과 추가 버튼은 모두 숨겨져야 한다.
    expect(screen.queryByLabelText('태그 키')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('태그 값')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /태그 추가/ })).not.toBeInTheDocument();
  });

  // ---- Blur 자동 commit 회귀 가드 ----
  //
  // 버그 시나리오: 사용자가 "태그 키"/"태그 값" 입력 후 "태그 추가" 버튼을
  // 누르지 않고 (Enter 도 누르지 않고) 외부의 "저장" 버튼을 클릭하면,
  // 입력값이 TagChipsEditor 내부 state 에 머물러 있어 부모에 emit 되지 않고
  // 결과적으로 빈 tags 가 저장된다.
  //
  // 수정: 태그 폼 외부로 포커스가 이동(blur with relatedTarget outside form)
  //   하면 keyInput 이 비어있지 않은 한 자동으로 chip 으로 commit 한다.

  it('태그 입력 후 추가 버튼 누르지 않고 폼 외부를 클릭(blur) 하면 자동으로 chip 으로 commit 된다', () => {
    const onChange = vi.fn();
    render(<StoreKeysEditor value={[{ key: 'indoor:1', tags: {} }]} onChange={onChange} />);

    const tagKeyInput = screen.getByLabelText('태그 키') as HTMLInputElement;
    const tagValueInput = screen.getByLabelText('태그 값') as HTMLInputElement;
    fireEvent.change(tagKeyInput, { target: { value: 'room' } });
    fireEvent.change(tagValueInput, { target: { value: '1' } });

    // 폼 외부 요소(키 input — 행 단위 키 input)로 blur. relatedTarget 이
    // 태그 폼 ref 의 자손이 아니므로 commit 이 발생해야 한다.
    const rowKeyInput = screen.getByDisplayValue('indoor:1');
    fireEvent.blur(tagValueInput, { relatedTarget: rowKeyInput });

    expect(onChange).toHaveBeenLastCalledWith([{ key: 'indoor:1', tags: { room: '1' } }]);
  });

  it('태그 키만 있고 값이 비어있어도 폼 외부 blur 시 commit 된다 (추가 버튼과 동일)', () => {
    const onChange = vi.fn();
    render(<StoreKeysEditor value={[{ key: 'indoor:1', tags: {} }]} onChange={onChange} />);

    const tagKeyInput = screen.getByLabelText('태그 키') as HTMLInputElement;
    fireEvent.change(tagKeyInput, { target: { value: 'room' } });

    const rowKeyInput = screen.getByDisplayValue('indoor:1');
    fireEvent.blur(tagKeyInput, { relatedTarget: rowKeyInput });

    expect(onChange).toHaveBeenLastCalledWith([{ key: 'indoor:1', tags: { room: '' } }]);
  });

  it('태그 키가 비어있으면 폼 외부 blur 가 발생해도 commit 되지 않는다', () => {
    const onChange = vi.fn();
    render(<StoreKeysEditor value={[{ key: 'indoor:1', tags: {} }]} onChange={onChange} />);

    const tagValueInput = screen.getByLabelText('태그 값') as HTMLInputElement;
    fireEvent.change(tagValueInput, { target: { value: '1' } });

    const rowKeyInput = screen.getByDisplayValue('indoor:1');
    fireEvent.blur(tagValueInput, { relatedTarget: rowKeyInput });

    // key 가 비어있으므로 onChange 가 호출되지 않아야 한다.
    expect(onChange).not.toHaveBeenCalled();
  });

  it('태그 폼 내부에서의 포커스 이동(키→값)은 commit 을 유발하지 않는다', () => {
    const onChange = vi.fn();
    render(<StoreKeysEditor value={[{ key: 'indoor:1', tags: {} }]} onChange={onChange} />);

    const tagKeyInput = screen.getByLabelText('태그 키') as HTMLInputElement;
    const tagValueInput = screen.getByLabelText('태그 값') as HTMLInputElement;
    fireEvent.change(tagKeyInput, { target: { value: 'room' } });

    // 키 input 에서 값 input 으로 포커스 이동 — 폼 내부 이동이므로 commit 안 됨.
    fireEvent.blur(tagKeyInput, { relatedTarget: tagValueInput });

    expect(onChange).not.toHaveBeenCalled();
  });

  // ─────────────────────────────────────────────────────────────────────
  // v0.7.0 (M13) 신규 테스트 — data_type / metric_type 컬럼
  // ─────────────────────────────────────────────────────────────────────

  describe('data_type 셀렉트 컬럼 (M13)', () => {
    it('헤더에 "데이터 타입" 컬럼이 표시된다', () => {
      render(<StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />);
      expect(screen.getByText('데이터 타입')).toBeInTheDocument();
    });

    it('각 행에 6종 enum 옵션 셀렉트가 렌더된다', () => {
      render(
        <StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />,
      );
      const select = screen.getByLabelText('데이터 타입') as HTMLSelectElement;
      const optionValues = Array.from(select.options).map((o) => o.value);
      // 빈 옵션(unset placeholder) + 6종 enum
      expect(optionValues).toContain('');
      expect(optionValues).toContain('int');
      expect(optionValues).toContain('float');
      expect(optionValues).toContain('string');
      expect(optionValues).toContain('boolean');
      expect(optionValues).toContain('bytes');
      expect(optionValues).toContain('json');
    });

    it('초기 value 의 data_type 값이 셀렉트에 반영된다', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', data_type: 'float', tags: {} }]}
          onChange={vi.fn()}
        />,
      );
      const select = screen.getByLabelText('데이터 타입') as HTMLSelectElement;
      expect(select.value).toBe('float');
    });

    it('data_type 변경 시 onChange 가 호출되고 entry 에 포함된다', () => {
      const onChange = vi.fn();
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={onChange}
        />,
      );
      const select = screen.getByLabelText('데이터 타입') as HTMLSelectElement;
      fireEvent.change(select, { target: { value: 'int' } });
      expect(onChange).toHaveBeenCalledWith([
        { key: 'k1', data_type: 'int', tags: {} },
      ]);
    });

    it('manual 모드에서 data_type 미입력 행은 인라인 에러 표시', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          registrationType="manual"
        />,
      );
      expect(
        screen.getByText('data_type 은 manual 모드에서 필수입니다'),
      ).toBeInTheDocument();
    });

    it('auto 모드에서 data_type 미입력은 에러 없이 통과', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          registrationType="auto"
        />,
      );
      expect(
        screen.queryByText('data_type 은 manual 모드에서 필수입니다'),
      ).not.toBeInTheDocument();
    });

    it('manual 모드에서 data_type 선택 시 인라인 에러가 사라진다', () => {
      const { rerender } = render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          registrationType="manual"
        />,
      );
      expect(
        screen.getByText('data_type 은 manual 모드에서 필수입니다'),
      ).toBeInTheDocument();

      // data_type 선택 후 재렌더
      rerender(
        <StoreKeysEditor
          value={[{ key: 'k1', data_type: 'int', tags: {} }]}
          onChange={vi.fn()}
          registrationType="manual"
        />,
      );
      expect(
        screen.queryByText('data_type 은 manual 모드에서 필수입니다'),
      ).not.toBeInTheDocument();
    });
  });

  describe('metric_type 텍스트 컬럼 (M13)', () => {
    it('헤더에 "메트릭 타입" 컬럼이 표시된다', () => {
      render(<StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />);
      expect(screen.getByText('메트릭 타입')).toBeInTheDocument();
    });

    it('각 행에 metric_type 입력란과 "unknown" placeholder 가 렌더된다', () => {
      render(<StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />);
      const input = screen.getByLabelText('메트릭 타입') as HTMLInputElement;
      expect(input.placeholder).toBe('unknown');
    });

    it('metric_type 입력 시 onChange 가 호출되고 entry 에 포함된다', () => {
      const onChange = vi.fn();
      render(
        <StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={onChange} />,
      );
      const input = screen.getByLabelText('메트릭 타입') as HTMLInputElement;
      fireEvent.change(input, { target: { value: 'temperature' } });
      expect(onChange).toHaveBeenCalledWith([
        { key: 'k1', metric_type: 'temperature', tags: {} },
      ]);
    });

    it('정규식 위반(점 포함) 입력 시 인라인 에러 표시', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', metric_type: 'room.temp', tags: {} }]}
          onChange={vi.fn()}
        />,
      );
      expect(
        screen.getByText(/허용되지 않는 문자가 포함되었습니다/),
      ).toBeInTheDocument();
    });

    it('빈 metric_type 은 통과 (백엔드 default unknown 적용)', () => {
      render(
        <StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />,
      );
      expect(
        screen.queryByText(/허용되지 않는 문자가 포함되었습니다/),
      ).not.toBeInTheDocument();
    });

    it('영문/숫자/하이픈/언더스코어는 통과', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', metric_type: 'room-temp_2', tags: {} }]}
          onChange={vi.fn()}
        />,
      );
      expect(
        screen.queryByText(/허용되지 않는 문자가 포함되었습니다/),
      ).not.toBeInTheDocument();
    });
  });

  describe('onValidityChange 콜백 (M13)', () => {
    it('manual 모드 + data_type 누락 행이 있으면 valid=false 를 호출한다', () => {
      const onValidityChange = vi.fn();
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          registrationType="manual"
          onValidityChange={onValidityChange}
        />,
      );
      // 마지막 호출이 false 여야 한다.
      const lastCall = onValidityChange.mock.calls.at(-1);
      expect(lastCall?.[0]).toBe(false);
    });

    it('manual 모드 + 모든 행 data_type 입력 시 valid=true 를 호출한다', () => {
      const onValidityChange = vi.fn();
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', data_type: 'int', tags: {} }]}
          onChange={vi.fn()}
          registrationType="manual"
          onValidityChange={onValidityChange}
        />,
      );
      const lastCall = onValidityChange.mock.calls.at(-1);
      expect(lastCall?.[0]).toBe(true);
    });

    it('auto 모드 + data_type 누락은 valid=true (선택 사항)', () => {
      const onValidityChange = vi.fn();
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          registrationType="auto"
          onValidityChange={onValidityChange}
        />,
      );
      const lastCall = onValidityChange.mock.calls.at(-1);
      expect(lastCall?.[0]).toBe(true);
    });

    it('metric_type 정규식 위반 시 valid=false', () => {
      const onValidityChange = vi.fn();
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', metric_type: 'room.temp', tags: {} }]}
          onChange={vi.fn()}
          registrationType="auto"
          onValidityChange={onValidityChange}
        />,
      );
      const lastCall = onValidityChange.mock.calls.at(-1);
      expect(lastCall?.[0]).toBe(false);
    });
  });

  describe('컬럼 너비 (M13)', () => {
    it('5컬럼(키:데이터 타입:메트릭 타입:태그:삭제) 헤더가 모두 렌더된다', () => {
      render(<StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />);
      expect(screen.getByText('키')).toBeInTheDocument();
      expect(screen.getByText('데이터 타입')).toBeInTheDocument();
      expect(screen.getByText('메트릭 타입')).toBeInTheDocument();
      expect(screen.getByText('태그')).toBeInTheDocument();
    });
  });

  // ─────────────────────────────────────────────────────────────────────
  // v0.7.0 (Task 14, Phase F) — manual 등록 배지 (MetadataChips)
  // 이 에디터는 yaml 정적 정의 전용이므로 모든 행이 manual 등록이다.
  // TsdbDataViewerModal 의 auto/manual 배지와 동일 컴포넌트로 시각 통일.
  // ─────────────────────────────────────────────────────────────────────

  describe('manual 등록 배지 (Task 14)', () => {
    it('각 행에 manual 배지가 렌더된다', () => {
      render(
        <StoreKeysEditor
          value={[
            { key: 'k1', tags: {} },
            { key: 'k2', tags: {} },
          ]}
          onChange={vi.fn()}
        />,
      );
      const manualBadges = screen.getAllByTestId('metadata-registration-manual');
      expect(manualBadges).toHaveLength(2);
    });

    it('manual 배지는 yaml 정적 정의임을 명시하는 a11y 라벨을 가진다', () => {
      render(
        <StoreKeysEditor value={[{ key: 'k1', tags: {} }]} onChange={vi.fn()} />,
      );
      const manualBadge = screen.getByTestId('metadata-registration-manual');
      expect(manualBadge).toHaveAttribute('aria-label', '수동 등록');
      expect(manualBadge).toHaveTextContent('manual');
    });

    it('readOnly 모드에서도 manual 배지는 계속 노출된다 (시각 일관성)', () => {
      render(
        <StoreKeysEditor
          value={[{ key: 'k1', tags: {} }]}
          onChange={vi.fn()}
          readOnly
        />,
      );
      expect(
        screen.getByTestId('metadata-registration-manual'),
      ).toBeInTheDocument();
    });

    it('빈 목록에는 manual 배지가 표시되지 않는다', () => {
      render(<StoreKeysEditor value={undefined} onChange={vi.fn()} />);
      expect(
        screen.queryByTestId('metadata-registration-manual'),
      ).not.toBeInTheDocument();
    });

    it('auto 배지(런타임 자동 등록)는 어떤 상황에서도 노출되지 않는다', () => {
      // 이 에디터는 yaml 정적 정의 전용 — 'auto' 배지는 정의상 등장할 수 없다.
      render(
        <StoreKeysEditor
          value={[
            { key: 'k1', tags: {} },
            { key: 'k2', tags: {} },
          ]}
          onChange={vi.fn()}
        />,
      );
      expect(
        screen.queryByTestId('metadata-registration-auto'),
      ).not.toBeInTheDocument();
    });
  });
});
