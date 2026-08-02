// ModbusServerDevicesEditor 컴포넌트 테스트 (컴팩트 목록 + 팝업 편집, address 키 모델).
//
// 검증 대상:
//   - 빈 value → 시작점 서빙 디바이스 1개 시드 (unit_id 1)
//   - 컴팩트 목록: unit_id + name 표시, 팝업으로 편집
//   - 방출 devices JSON 이 백엔드 형상과 일치:
//       * 세그먼트 키는 address (start_address 아님)
//       * local 세그먼트: address/count/data_type
//       * shared 세그먼트: address/count/shared_address (data_type 없음)
//       * 디바이스 0 컨테이너: unit_id 0, 전부 local
//   - legacy JSON 문자열 value 파싱 (address / shared_address / start_address 폴백)
//   - readOnly 모드에서 추가/삭제 버튼 숨김
//
// i18n: ModbusDevicesEditor.test 와 동일하게 ko.json 을 점 표기로 해석하는 mock 사용.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { ModbusServerDevicesEditor } from './ModbusServerDevicesEditor';

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

/** 마지막 onChange 인자 (방출된 devices 배열). */
function lastEmit(onChange: ReturnType<typeof vi.fn>): unknown {
  return onChange.mock.calls.at(-1)?.[0];
}

/** 열린 팝업 모달 컨테이너. */
function dialog(): HTMLElement {
  return screen.getByRole('dialog');
}

describe('ModbusServerDevicesEditor', () => {
  it('빈 value 는 시작점 서빙 디바이스 1개(unit_id 1)를 시드한다', () => {
    render(<ModbusServerDevicesEditor value={undefined} onChange={vi.fn()} />);
    expect(screen.getByText(/디바이스 1/)).toBeInTheDocument();
    expect(screen.getByText(/유닛 ID: 1/)).toBeInTheDocument();
  });

  it('legacy JSON 문자열 value 를 파싱해 컨테이너 + 서빙 디바이스를 렌더링한다', () => {
    const json = JSON.stringify([
      { unit_id: 0, name: 'shared', register_map: { holding_registers: [{ address: 0, count: 100 }] } },
      { unit_id: 7, name: 'boiler', register_map: { coils: [{ start_address: 0, count: 8 }] } },
    ]);
    render(<ModbusServerDevicesEditor value={json} onChange={vi.fn()} />);
    // 컨테이너(디바이스 0) 섹션 + 서빙 디바이스 유닛 ID 7.
    expect(screen.getByText(/유닛 ID: 7/)).toBeInTheDocument();
    expect(screen.getByText('boiler')).toBeInTheDocument();
  });

  it('디바이스 삭제 → 빈 배열을 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 5, register_map: {} }]}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '디바이스 삭제' }));
    expect(lastEmit(onChange)).toEqual([]);
  });

  it('팝업에서 로컬 세그먼트 추가 → address/count/data_type 로 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // 편집 팝업 열기.
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    // 보유 레지스터 영역의 "세그먼트 추가" (4개 영역 중 3번째). 첫 번째 매칭이 coils 이므로
    // 영역별 버튼을 모두 찾아 holding_registers(3번째)를 클릭한다.
    const addSegBtns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
    fireEvent.click(addSegBtns[2]!); // holding_registers
    // 저장.
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        register_map: {
          holding_registers: [{ address: 0, count: 1, data_type: 'uint16' }],
        },
      },
    ]);
  });

  it('팝업에서 공유 세그먼트 → address/count/shared_address 로 방출하고 data_type 을 생략한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 2, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    const addSegBtns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
    fireEvent.click(addSegBtns[2]!); // holding_registers
    // 공유 체크박스 ON.
    fireEvent.click(within(dialog()).getByRole('checkbox'));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 2,
        register_map: {
          holding_registers: [{ address: 0, count: 1, shared_address: 0 }],
        },
      },
    ]);
  });

  it('공유 맵(디바이스 0) 추가 → 팝업 세그먼트는 전부 local 로, unit_id 0 으로 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // 공유 맵 추가 → 팝업 열림 (컨테이너 모드).
    fireEvent.click(screen.getByRole('button', { name: '공유 맵 추가' }));
    // 컨테이너 팝업에는 공유 토글(checkbox)이 없다.
    expect(within(dialog()).queryByRole('checkbox')).not.toBeInTheDocument();
    const addSegBtns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
    fireEvent.click(addSegBtns[2]!); // holding_registers
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    // 컨테이너가 먼저, 그 다음 서빙 디바이스.
    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 0,
        register_map: {
          holding_registers: [{ address: 0, count: 1, data_type: 'uint16' }],
        },
      },
      { unit_id: 1, register_map: {} },
    ]);
  });

  it('방출된 세그먼트의 숫자 필드는 number 타입이다 (address/count/shared_address)', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    const addSegBtns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
    fireEvent.click(addSegBtns[0]!); // coils
    fireEvent.click(within(dialog()).getByRole('checkbox')); // shared ON
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    const emitted = lastEmit(onChange) as Array<{
      unit_id: number;
      register_map: { coils: Array<{ address: unknown; count: unknown; shared_address: unknown }> };
    }>;
    const seg = emitted[0]!.register_map.coils[0]!;
    expect(typeof emitted[0]!.unit_id).toBe('number');
    expect(typeof seg.address).toBe('number');
    expect(typeof seg.count).toBe('number');
    expect(typeof seg.shared_address).toBe('number');
  });

  it('name 입력 시 방출 디바이스에 name 을 포함한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 3, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    // 팝업 내 name 입력(빈 값) 찾기.
    const nameInput = within(dialog()).getByPlaceholderText('device-1');
    fireEvent.change(nameInput, { target: { value: 'sensor-a' } });
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      { unit_id: 3, name: 'sensor-a', register_map: {} },
    ]);
  });

  it('readOnly 모드에서는 추가/삭제 버튼을 숨긴다', () => {
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(
      screen.queryByRole('button', { name: '디바이스 추가' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: '디바이스 삭제' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: '공유 맵 추가' }),
    ).not.toBeInTheDocument();
  });
});
