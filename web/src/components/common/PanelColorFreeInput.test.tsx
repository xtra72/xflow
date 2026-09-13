// 패널 색상 자유 입력 — 부품 자체와, 이 부품이 놓인 **두 화면** 모두를 본다.
//
// 두 화면에 각각 자유 입력을 손으로 심으면 한쪽만 고쳐지는 일이 생긴다. 그래서
// 부품은 한 벌이고, 아래 마지막 두 묶음은 "그 한 벌이 실제로 두 화면에 붙어
// 있는가" 만 확인한다.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import PanelColorFreeInput from './PanelColorFreeInput';
import PanelSettingsDropdown from './PanelSettingsDropdown';

const hex = () => screen.getByTestId('panel-color-hex') as HTMLInputElement;
const native = () => screen.getByTestId('panel-color-native') as HTMLInputElement;

describe('자유 입력 — 값이 반영되는 경로', () => {
  it('완성된 헥스를 치면 그 색이 적용된다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(hex(), { target: { value: '#123abc' } });
    expect(onChange).toHaveBeenCalledWith('#123abc');
  });

  it('3자리 약식은 6자리로 펴서 적용된다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(hex(), { target: { value: '#abc' } });
    expect(onChange).toHaveBeenCalledWith('#aabbcc');
  });

  it('대문자로 쳐도 소문자로 저장된다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(hex(), { target: { value: '#8B5CF6' } });
    expect(onChange).toHaveBeenCalledWith('#8b5cf6');
  });

  it('# 없이 쳐도 적용된다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(hex(), { target: { value: '3b82f6' } });
    expect(onChange).toHaveBeenCalledWith('#3b82f6');
  });

  it('네이티브 색상 피커가 고른 값이 적용된다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(native(), { target: { value: '#10b981' } });
    expect(onChange).toHaveBeenCalledWith('#10b981');
  });
});

describe('자유 입력 — 잘못된 값은 설정에 닿지 않는다', () => {
  it('형식이 맞지 않으면 아무 것도 저장하지 않는다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    for (const bad of ['#ab', 'red', '#gggggg', '#fff; background: url(x)', '#abcdef80']) {
      fireEvent.change(hex(), { target: { value: bad } });
    }
    expect(onChange).not.toHaveBeenCalled();
  });

  it('치는 도중에는 막지 않는다 — 입력칸은 글자를 그대로 들고 있는다', () => {
    const onChange = vi.fn();
    render(<PanelColorFreeInput value={undefined} onChange={onChange} />);

    fireEvent.change(hex(), { target: { value: '#12' } });
    expect(hex().value).toBe('#12');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('형식이 안 맞는 채로 떠나면 실제 저장값으로 되돌린다', () => {
    render(<PanelColorFreeInput value="#ef4444" onChange={vi.fn()} />);

    fireEvent.change(hex(), { target: { value: 'zzz' } });
    fireEvent.blur(hex());
    expect(hex().value).toBe('#ef4444');
  });
});

describe('자유 입력 — 바깥에서 값이 바뀌면 따라간다', () => {
  it('프리셋 선택·초기화로 값이 바뀌면 입력칸도 갱신된다', () => {
    const { rerender } = render(<PanelColorFreeInput value="#ef4444" onChange={vi.fn()} />);
    expect(hex().value).toBe('#ef4444');

    rerender(<PanelColorFreeInput value="#10b981" onChange={vi.fn()} />);
    expect(hex().value).toBe('#10b981');

    rerender(<PanelColorFreeInput value={undefined} onChange={vi.fn()} />);
    expect(hex().value).toBe('');
  });
});

describe('자유 입력 — 접근성', () => {
  it('두 컨트롤 모두 이름을 가진다', () => {
    render(<PanelColorFreeInput value={undefined} onChange={vi.fn()} />);
    // 아이콘만 있는 라벨은 이름이 없다. 이름 없이는 키보드·스크린리더가
    // 무엇을 고르는 칸인지 알 수 없다.
    expect(screen.getByLabelText('panel.color.custom')).toBe(native());
    expect(screen.getByLabelText('panel.color.hex')).toBe(hex());
  });

  it('키보드로 닿는다 — 어느 쪽도 탭 순서에서 빠지지 않는다', () => {
    render(<PanelColorFreeInput value={undefined} onChange={vi.fn()} />);
    for (const el of [native(), hex()]) {
      expect(el.tabIndex).toBeGreaterThanOrEqual(0);
      expect(el).not.toBeDisabled();
    }
  });
});

describe('화면 2/2 — 패널 헤더 드롭다운에도 자유 입력이 있다', () => {
  it('패널 컬러 구역에 자유 입력이 함께 놓인다', () => {
    const onPanelColorChange = vi.fn();
    render(
      <PanelSettingsDropdown
        title="t"
        onTitleChange={vi.fn()}
        panelColor={undefined}
        onPanelColorChange={onPanelColorChange}
      />,
    );
    fireEvent.click(screen.getByLabelText('panel.settings.aria'));

    // 프리셋 스와치는 그대로 있고,
    expect(screen.getByLabelText('#3b82f6')).toBeInTheDocument();
    // 그 옆에 자유 입력이 새로 있다.
    expect(screen.getByTestId('panel-color-free')).toBeInTheDocument();

    fireEvent.change(hex(), { target: { value: '#abc' } });
    expect(onPanelColorChange).toHaveBeenCalledWith('#aabbcc');
  });
});
