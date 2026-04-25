// StoreKeysEditor 컴포넌트 테스트.
//
// 검증 대상:
//   - 초기 value 를 행으로 렌더링
//   - "행 추가" 버튼 → 빈 행 추가 + onChange 호출
//   - 행 삭제 버튼 → 행 제거 + onChange 호출
//   - 태그 추가 → key/value 입력 + "태그 추가" 버튼 → onChange 호출
//   - 태그 칩 X 클릭 → 태그 제거
//   - 중복 키 경고 (soft warning, blocking 아님)
//   - readOnly 모드: 입력 비활성, 추가/삭제 버튼 숨김
//   - 잘못된 형식(비-JSON 배열 등) 은 빈 목록으로 폴백
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { StoreKeysEditor, type StoreKeyEntry } from './StoreKeysEditor';

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
      { key: 'indoor/1/temp', tags: { room: '1', type: 'temperature' } },
      { key: 'indoor/2/temp', tags: { room: '2' } },
    ];
    render(<StoreKeysEditor value={value} onChange={vi.fn()} />);
    expect(screen.getByDisplayValue('indoor/1/temp')).toBeInTheDocument();
    expect(screen.getByDisplayValue('indoor/2/temp')).toBeInTheDocument();
    // 태그 칩 텍스트
    expect(screen.getByText('room=1')).toBeInTheDocument();
    expect(screen.getByText('type=temperature')).toBeInTheDocument();
    expect(screen.getByText('room=2')).toBeInTheDocument();
  });

  it('"행 추가" 버튼 클릭 시 빈 행을 추가하고 onChange 호출', () => {
    const onChange = vi.fn();
    const value: StoreKeyEntry[] = [{ key: 'a', tags: {} }];
    render(<StoreKeysEditor value={value} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: /행 추가/ }));
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
});
