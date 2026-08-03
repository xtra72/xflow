// Modbus 명령셋 에디터 테스트 (modbus-write / modbus-read / modbus-control).
//
// 검증: WriteOp(단일 value / 다중 values), ReadOp, ControlOp(params), unit_id 0 허용,
//        parseValuesInput 순수 함수, readOnly 버튼 숨김.
//
// i18n: ko.json 을 점 표기로 해석하는 mock.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import {
  ModbusRwCommandSetEditor,
  ModbusControlCommandSetEditor,
  parseValuesInput,
} from './ModbusCommandSetEditor';

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

function lastEmit(onChange: ReturnType<typeof vi.fn>): unknown {
  return onChange.mock.calls.at(-1)?.[0];
}

describe('parseValuesInput', () => {
  it('토큰 0개 → 빈 객체(value/values 생략)', () => {
    expect(parseValuesInput('')).toEqual({});
    expect(parseValuesInput('  ')).toEqual({});
  });
  it('토큰 1개 → value(숫자)', () => {
    expect(parseValuesInput('42')).toEqual({ value: 42 });
  });
  it('토큰 2개 이상 → values 배열', () => {
    expect(parseValuesInput('1, 2, 3')).toEqual({ values: [1, 2, 3] });
    expect(parseValuesInput('1 2 3')).toEqual({ values: [1, 2, 3] });
  });
  it('숫자가 아니면 문자열 토큰 유지', () => {
    expect(parseValuesInput('on')).toEqual({ value: 'on' });
  });
});

describe('ModbusRwCommandSetEditor (write)', () => {
  it('단일 값 write op → { area, address, value, data_type, byte_order }', () => {
    const onChange = vi.fn();
    render(<ModbusRwCommandSetEditor value={[]} onChange={onChange} mode="write" />);

    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    fireEvent.change(screen.getByRole('textbox', { name: '값' }), {
      target: { value: '42' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        area: 'holding_registers',
        address: 0,
        value: 42,
        data_type: 'uint16',
        byte_order: 'big_endian',
      },
    ]);
  });

  it('다중 값 write op → values 배열 + unit_id 0 허용', () => {
    const onChange = vi.fn();
    render(<ModbusRwCommandSetEditor value={[]} onChange={onChange} mode="write" />);

    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    fireEvent.change(screen.getByRole('textbox', { name: '값' }), {
      target: { value: '1 2 3' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '유닛 ID' }), {
      target: { value: '0' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        area: 'holding_registers',
        address: 0,
        values: [1, 2, 3],
        data_type: 'uint16',
        byte_order: 'big_endian',
        unit_id: 0,
      },
    ]);
  });

  it('legacy config(values 배열)를 파싱해 라운드트립한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusRwCommandSetEditor
        value={[{ area: 'coils', address: 5, values: [1, 0, 1] }]}
        onChange={onChange}
        mode="write"
      />,
    );
    // 값 입력에 "1, 0, 1" 로 역직렬화되어 표시된다.
    expect(screen.getByDisplayValue('1, 0, 1')).toBeInTheDocument();
  });
});

describe('ModbusRwCommandSetEditor (read)', () => {
  it('read op → { area, address, count, data_type, byte_order }', () => {
    const onChange = vi.fn();
    render(<ModbusRwCommandSetEditor value={[]} onChange={onChange} mode="read" />);

    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: '주소' }), {
      target: { value: '100' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '개수' }), {
      target: { value: '10' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        area: 'holding_registers',
        address: 100,
        count: 10,
        data_type: 'uint16',
        byte_order: 'big_endian',
      },
    ]);
  });
});

describe('ModbusControlCommandSetEditor', () => {
  it('params 없는 control op → { action }', () => {
    const onChange = vi.fn();
    render(<ModbusControlCommandSetEditor value={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    expect(lastEmit(onChange)).toEqual([{ action: 'start' }]);
  });

  it('action + params(JSON) → { action, params }', () => {
    const onChange = vi.fn();
    render(<ModbusControlCommandSetEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    fireEvent.change(screen.getByRole('combobox', { name: '액션' }), {
      target: { value: 'add_device' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: 'params (JSON)' }), {
      target: { value: '{"unit_id":5,"host":"10.0.0.9"}' },
    });

    expect(lastEmit(onChange)).toEqual([
      { action: 'add_device', params: { unit_id: 5, host: '10.0.0.9' } },
    ]);
  });

  it('유효하지 않은 JSON params 는 생략된다', () => {
    const onChange = vi.fn();
    render(<ModbusControlCommandSetEditor value={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '추가' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'params (JSON)' }), {
      target: { value: '{bad json' },
    });
    expect(lastEmit(onChange)).toEqual([{ action: 'start' }]);
  });
});

describe('readOnly', () => {
  it('추가 버튼을 숨긴다 (write/control)', () => {
    const { rerender } = render(
      <ModbusRwCommandSetEditor
        value={[{ area: 'coils', address: 0, value: 1 }]}
        onChange={vi.fn()}
        mode="write"
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '추가' })).not.toBeInTheDocument();

    rerender(
      <ModbusControlCommandSetEditor
        value={[{ action: 'start' }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '추가' })).not.toBeInTheDocument();
  });
});
