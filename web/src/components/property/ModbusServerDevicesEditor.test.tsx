// ModbusServerDevicesEditor 컴포넌트 테스트 (컴팩트 목록 + 팝업 편집, address 키 모델).
//
// 세그먼트 에디터는 컬럼형 단일 행 레이아웃 + 선택 기반 일괄 삭제이다:
//   [선택] | 주소 | 개수 | 데이터 타입 | 공유 | 공유 주소
//
// 검증 대상:
//   - 빈 value → 시작점 서빙 디바이스 1개 시드 (unit_id 1)
//   - 컴팩트 목록: unit_id + name 표시, 팝업으로 편집
//   - 방출 devices JSON 이 백엔드 형상과 일치 (변경 없음):
//       * 세그먼트 키는 address
//       * local 세그먼트: address/count/data_type
//       * shared 세그먼트: address/count/shared_address (data_type 없음)
//       * 디바이스 0 컨테이너: unit_id 0, 전부 local, 공유 토글 없음
//   - 공유 토글 flip: data_type ↔ shared_address
//   - 선택 기반 일괄 삭제
//   - legacy JSON 문자열 value 파싱
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

/** 팝업 내 특정 영역(0=coils,1=discrete,2=holding,3=input)에 세그먼트 1개 추가. */
function addSegment(areaIndex: number): void {
  const btns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
  fireEvent.click(btns[areaIndex]!);
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

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    addSegment(2); // holding_registers
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

  it('공유 토글 ON → address/count/shared_address 로 방출하고 data_type 을 생략한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 2, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    addSegment(2); // holding_registers
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: '공유' }));
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

  it('공유 토글 flip: local(data_type) → 저장 → shared(shared_address) 로 전환된다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 4, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // 1) 로컬 세그먼트 추가 후 저장 → data_type 포함.
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    addSegment(2); // holding_registers
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));
    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 4,
        register_map: {
          holding_registers: [{ address: 0, count: 1, data_type: 'uint16' }],
        },
      },
    ]);

    // 2) 다시 편집 → 공유 토글 ON → 저장 → shared_address 로 전환(data_type 제거).
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: '공유' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));
    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 4,
        register_map: {
          holding_registers: [{ address: 0, count: 1, shared_address: 0 }],
        },
      },
    ]);
  });

  it('선택 기반 일괄 삭제: 두 행 선택 후 "선택 삭제" → 해당 영역이 비워진다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    addSegment(2); // holding_registers 세그먼트 1
    addSegment(2); // holding_registers 세그먼트 2

    // 두 행 선택.
    const rowChecks = within(dialog()).getAllByRole('checkbox', { name: '행 선택' });
    expect(rowChecks).toHaveLength(2);
    fireEvent.click(rowChecks[0]!);
    fireEvent.click(rowChecks[1]!);

    // 선택 삭제.
    fireEvent.click(within(dialog()).getByRole('button', { name: /선택 삭제/ }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    // 영역이 비워져 register_map 에서 생략된다.
    expect(lastEmit(onChange)).toEqual([{ unit_id: 1, register_map: {} }]);
  });

  it('공유 맵(디바이스 0) 추가 → 팝업에 공유 토글이 없고 unit_id 0 으로 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '공유 맵 추가' }));
    addSegment(2); // holding_registers
    // 컨테이너 팝업에는 공유 토글(checkbox name '공유')이 없다.
    expect(
      within(dialog()).queryByRole('checkbox', { name: '공유' }),
    ).not.toBeInTheDocument();
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
    addSegment(0); // coils
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: '공유' }));
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
