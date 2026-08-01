// ModbusServerDevicesEditor 컴포넌트 테스트.
//
// 검증 대상:
//   - 빈 value → 시작점 디바이스 1개 시드 (unit_id 1)
//   - 디바이스 추가/삭제 + onChange 방출
//   - 방출 devices JSON 이 백엔드 형상과 일치 (unit_id + register_map, 빈 name 생략)
//   - RegisterMapEditor 재사용: 영역 추가 시 register_map 이 segment 배열로 방출
//   - name 입력 시 방출에 포함
//   - legacy JSON 문자열 value 파싱
//   - readOnly 모드에서 추가/삭제 버튼 숨김
//
// i18n: ModbusDevicesEditor.test 와 동일하게 ko.json 을 점 표기로 해석하는 mock 사용.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

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

describe('ModbusServerDevicesEditor', () => {
  it('빈 value 는 시작점 디바이스 1개(unit_id 1)를 시드한다', () => {
    render(<ModbusServerDevicesEditor value={undefined} onChange={vi.fn()} />);
    expect(screen.getByDisplayValue('1')).toBeInTheDocument();
    expect(screen.getByText(/디바이스 1/)).toBeInTheDocument();
  });

  it('legacy JSON 문자열 value 를 파싱해 디바이스를 렌더링한다', () => {
    const json =
      '[{"unit_id":7,"name":"boiler","register_map":{"holding_registers":[{"start_address":0,"count":10}]}}]';
    render(<ModbusServerDevicesEditor value={json} onChange={vi.fn()} />);
    expect(screen.getByDisplayValue('7')).toBeInTheDocument();
    expect(screen.getByDisplayValue('boiler')).toBeInTheDocument();
  });

  it('디바이스 추가 → onChange 로 unit_id + 빈 register_map 을 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));

    // 두 디바이스: 기존 unit_id 1 + 신규 unit_id 1, 둘 다 빈 register_map.
    expect(lastEmit(onChange)).toEqual([
      { unit_id: 1, register_map: {} },
      { unit_id: 1, register_map: {} },
    ]);
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

  it('name 입력 시 방출 디바이스에 name 을 포함한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 3, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByDisplayValue('') /* name 입력(빈 값) */, {
      target: { value: 'sensor-a' },
    });

    expect(lastEmit(onChange)).toEqual([
      { unit_id: 3, name: 'sensor-a', register_map: {} },
    ]);
  });

  it('RegisterMapEditor 재사용: 영역 추가 시 register_map 을 segment 배열로 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // RegisterMapEditor 의 "영역 추가" 버튼 → 기본 holding_registers 행(count 10) 추가.
    fireEvent.click(screen.getByRole('button', { name: '영역 추가' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        register_map: {
          holding_registers: [{ start_address: 0, count: 10 }],
        },
      },
    ]);
  });

  it('방출된 register_map 의 숫자 필드는 number 타입이다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '영역 추가' }));

    const emitted = lastEmit(onChange) as Array<{
      unit_id: number;
      register_map: { holding_registers: Array<{ start_address: unknown; count: unknown }> };
    }>;
    const device = emitted[0]!;
    const seg = device.register_map.holding_registers[0]!;
    expect(typeof device.unit_id).toBe('number');
    expect(typeof seg.start_address).toBe('number');
    expect(typeof seg.count).toBe('number');
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
  });
});
