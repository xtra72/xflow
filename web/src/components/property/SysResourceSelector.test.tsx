// 시스템 관측 대상 선택기 테스트.
//
// 하중 지지점은 "이미 고른 값이 지금 호스트에 없어도 화면에서 사라지지 않는다"이다.
// 목록에만 의존해 그리면 마운트 해제·NIC 제거·타 장비 설정 복사 시 그 항목이 조용히
// 설정에서 빠진다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (key: string) => key, locale: 'ko', setLocale: () => {} }),
}));

const resourcesRef = vi.hoisted(() => ({
  data: undefined as unknown,
  isLoading: false,
  isError: false,
}));

vi.mock('@/services/api/monitorService', () => ({
  useSysResources: () => resourcesRef,
}));

import { SysResourceSelector } from './SysResourceSelector';

/** 목록 조회 결과를 세팅한다. */
function seed(data: unknown, over: Partial<typeof resourcesRef> = {}) {
  resourcesRef.data = data;
  resourcesRef.isLoading = over.isLoading ?? false;
  resourcesRef.isError = over.isError ?? false;
}

const HOST = {
  mountpoints: ['/', '/data'],
  devices: ['disk0', 'disk1'],
  interfaces: ['en0', 'lo0'],
};

describe('SysResourceSelector', () => {
  beforeEach(() => {
    seed(HOST);
  });

  it('호스트 목록을 체크박스로 보여준다', () => {
    render(<SysResourceSelector kind="interfaces" value={[]} onChange={() => {}} />);

    expect(screen.getByTestId('sysresource-interfaces-option-en0')).toBeInTheDocument();
    expect(screen.getByTestId('sysresource-interfaces-option-lo0')).toBeInTheDocument();
  });

  it('축마다 다른 목록을 그린다', () => {
    render(<SysResourceSelector kind="mountpoints" value={[]} onChange={() => {}} />);

    expect(screen.getByTestId('sysresource-mountpoints-option-/')).toBeInTheDocument();
    // 인터페이스 목록이 섞여 나오면 안 된다.
    expect(screen.queryByTestId('sysresource-mountpoints-option-en0')).not.toBeInTheDocument();
  });

  it('선택한 항목이 체크되어 있다', () => {
    render(<SysResourceSelector kind="devices" value={['disk1']} onChange={() => {}} />);

    expect(screen.getByTestId('sysresource-devices-option-disk1')).toBeChecked();
    expect(screen.getByTestId('sysresource-devices-option-disk0')).not.toBeChecked();
  });

  it('체크하면 정렬된 배열로 넘긴다', () => {
    const onChange = vi.fn();
    render(<SysResourceSelector kind="interfaces" value={['lo0']} onChange={onChange} />);

    fireEvent.click(screen.getByTestId('sysresource-interfaces-option-en0'));

    // 클릭 순서가 아니라 정렬 순서 — 저장값이 클릭 순서에 따라 흔들리면 안 된다.
    expect(onChange).toHaveBeenCalledWith(['en0', 'lo0']);
  });

  it('체크를 풀면 목록에서 빠진다', () => {
    const onChange = vi.fn();
    render(<SysResourceSelector kind="interfaces" value={['en0', 'lo0']} onChange={onChange} />);

    fireEvent.click(screen.getByTestId('sysresource-interfaces-option-en0'));

    expect(onChange).toHaveBeenCalledWith(['lo0']);
  });

  describe('호스트에 없는 선택값', () => {
    it('화면에서 사라지지 않고 표시된다', () => {
      // 이것이 이 컴포넌트의 핵심 계약이다.
      render(<SysResourceSelector kind="interfaces" value={['eth9']} onChange={() => {}} />);

      expect(screen.getByTestId('sysresource-interfaces-option-eth9')).toBeChecked();
      expect(screen.getByTestId('sysresource-interfaces-missing-eth9')).toBeInTheDocument();
    });

    it('호스트에 있는 항목에는 표식이 붙지 않는다', () => {
      render(<SysResourceSelector kind="interfaces" value={['en0']} onChange={() => {}} />);

      expect(screen.queryByTestId('sysresource-interfaces-missing-en0')).not.toBeInTheDocument();
    });
  });

  describe('직접 입력', () => {
    it('목록에 없는 대상을 추가할 수 있다', () => {
      const onChange = vi.fn();
      render(<SysResourceSelector kind="mountpoints" value={[]} onChange={onChange} />);

      fireEvent.change(screen.getByTestId('sysresource-mountpoints-draft'), {
        target: { value: '/mnt/later' },
      });
      fireEvent.click(screen.getByTestId('sysresource-mountpoints-add'));

      expect(onChange).toHaveBeenCalledWith(['/mnt/later']);
    });

    it('이미 고른 값은 중복 추가되지 않는다', () => {
      const onChange = vi.fn();
      render(<SysResourceSelector kind="interfaces" value={['en0']} onChange={onChange} />);

      fireEvent.change(screen.getByTestId('sysresource-interfaces-draft'), {
        target: { value: 'en0' },
      });
      fireEvent.click(screen.getByTestId('sysresource-interfaces-add'));

      expect(onChange).not.toHaveBeenCalled();
    });

    it('공백만 입력하면 추가하지 않는다', () => {
      const onChange = vi.fn();
      render(<SysResourceSelector kind="interfaces" value={[]} onChange={onChange} />);

      fireEvent.change(screen.getByTestId('sysresource-interfaces-draft'), {
        target: { value: '   ' },
      });
      fireEvent.click(screen.getByTestId('sysresource-interfaces-add'));

      expect(onChange).not.toHaveBeenCalled();
    });
  });

  it('전체로 버튼이 선택을 비운다 (= 전체 관측)', () => {
    const onChange = vi.fn();
    render(<SysResourceSelector kind="devices" value={['disk0']} onChange={onChange} />);

    fireEvent.click(screen.getByTestId('sysresource-devices-clear'));

    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('선택이 없으면 비우기 버튼도 없다', () => {
    render(<SysResourceSelector kind="devices" value={[]} onChange={() => {}} />);

    expect(screen.queryByTestId('sysresource-devices-clear')).not.toBeInTheDocument();
  });

  describe('조회 실패·빈 목록', () => {
    it('조회에 실패해도 직접 입력은 열려 있다', () => {
      // 목록을 못 받았다고 설정 편집이 막히면 안 된다.
      seed(undefined, { isError: true });
      render(<SysResourceSelector kind="devices" value={[]} onChange={() => {}} />);

      expect(screen.getByTestId('sysresource-devices-error')).toBeInTheDocument();
      expect(screen.getByTestId('sysresource-devices-draft')).toBeInTheDocument();
    });

    it('조회에 실패해도 기존 선택값은 보인다', () => {
      seed(undefined, { isError: true });
      render(<SysResourceSelector kind="devices" value={['disk9']} onChange={() => {}} />);

      expect(screen.getByTestId('sysresource-devices-option-disk9')).toBeChecked();
    });

    it('읽기 전용이면 직접 입력을 감춘다', () => {
      render(<SysResourceSelector kind="devices" value={['disk0']} onChange={() => {}} readOnly />);

      expect(screen.queryByTestId('sysresource-devices-draft')).not.toBeInTheDocument();
      expect(screen.getByTestId('sysresource-devices-option-disk0')).toBeDisabled();
    });
  });

  it('저장값이 배열이 아니어도 터지지 않는다', () => {
    // 구버전 config 는 쉼표 문자열을 담고 있을 수 있다.
    render(<SysResourceSelector kind="interfaces" value={'en0,lo0'} onChange={() => {}} />);

    expect(screen.getByTestId('sysresource-interfaces-option-en0')).not.toBeChecked();
  });
});
