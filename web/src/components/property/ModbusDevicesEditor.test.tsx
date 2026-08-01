// ModbusDevicesEditor 컴포넌트 테스트.
//
// 검증 대상:
//   - 빈 value → "디바이스가 없습니다" 안내
//   - 디바이스 추가/삭제 + onChange 방출
//   - 레지스터 그룹 추가/삭제
//   - transport=tcp: host/port 노출 / transport=rtu: host/port 숨김(unit_id 만)
//   - 고급 type_map 항목 추가/삭제
//   - 방출 JSON 이 백엔드 형상과 일치 (빈 선택 필드 생략)
//   - legacy JSON 문자열 value 파싱
//
// i18n: StoreKeysEditor.test 와 동일하게 ko.json 을 점 표기로 해석하는 mock 사용.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { ModbusDevicesEditor } from './ModbusDevicesEditor';

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

describe('ModbusDevicesEditor', () => {
  it('빈 value 는 안내 문구를 표시한다', () => {
    render(<ModbusDevicesEditor value={undefined} onChange={vi.fn()} />);
    expect(screen.getByText(/디바이스가 없습니다/)).toBeInTheDocument();
  });

  it('배열/객체/문자열이 아닌 무효 value 는 빈 목록으로 폴백한다', () => {
    render(<ModbusDevicesEditor value={42} onChange={vi.fn()} />);
    expect(screen.getByText(/디바이스가 없습니다/)).toBeInTheDocument();
  });

  it('legacy JSON 문자열 value 를 파싱해 디바이스를 렌더링한다', () => {
    const json = '[{"unit_id":7,"host":"10.0.0.5","register_groups":[]}]';
    render(<ModbusDevicesEditor value={json} onChange={vi.fn()} transport="tcp" />);
    expect(screen.getByDisplayValue('10.0.0.5')).toBeInTheDocument();
    expect(screen.getByDisplayValue('7')).toBeInTheDocument();
  });

  it('깨진 JSON 문자열은 빈 목록으로 폴백한다', () => {
    render(<ModbusDevicesEditor value={'{not json'} onChange={vi.fn()} />);
    expect(screen.getByText(/디바이스가 없습니다/)).toBeInTheDocument();
  });

  it('transport=tcp 는 host/port 를 노출한다', () => {
    render(
      <ModbusDevicesEditor value={[{ unit_id: 1 }]} onChange={vi.fn()} transport="tcp" />,
    );
    expect(screen.getByText('호스트')).toBeInTheDocument();
    expect(screen.getByText('포트')).toBeInTheDocument();
    expect(screen.getByText('유닛 ID')).toBeInTheDocument();
  });

  it('transport=rtu 는 host/port 를 숨기고 unit_id 만 노출한다', () => {
    render(
      <ModbusDevicesEditor value={[{ unit_id: 1 }]} onChange={vi.fn()} transport="rtu" />,
    );
    expect(screen.queryByText('호스트')).not.toBeInTheDocument();
    expect(screen.queryByText('포트')).not.toBeInTheDocument();
    expect(screen.getByText('유닛 ID')).toBeInTheDocument();
  });

  it('디바이스 추가 → onChange 로 기본 디바이스를 방출한다 (tcp)', () => {
    const onChange = vi.fn();
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="tcp" />);

    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));

    expect(lastEmit(onChange)).toEqual([
      { unit_id: 1, port: 502, register_groups: [] },
    ]);
  });

  it('디바이스 추가 → rtu 에서는 host/port 를 방출하지 않는다', () => {
    const onChange = vi.fn();
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="rtu" />);

    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));

    expect(lastEmit(onChange)).toEqual([{ unit_id: 1, register_groups: [] }]);
  });

  it('디바이스 삭제 → 빈 배열을 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor value={[{ unit_id: 1 }]} onChange={onChange} transport="tcp" />,
    );

    fireEvent.click(screen.getByRole('button', { name: '디바이스 삭제' }));

    expect(lastEmit(onChange)).toEqual([]);
  });

  it('그룹 추가/삭제 → register_groups 를 갱신한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor value={[{ unit_id: 2 }]} onChange={onChange} transport="tcp" />,
    );

    fireEvent.click(screen.getByRole('button', { name: '그룹 추가' }));
    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 2,
        port: 502,
        register_groups: [
          { function_code: 3, start_address: 0, quantity: 1, data_type: 'uint16' },
        ],
      },
    ]);

    fireEvent.click(screen.getByRole('button', { name: '그룹 삭제' }));
    expect(lastEmit(onChange)).toEqual([
      { unit_id: 2, port: 502, register_groups: [] },
    ]);
  });

  it('tcp 디바이스 + 그룹 + type_map 엔트리를 백엔드 형상으로 방출한다 (빈 필드 생략)', () => {
    const onChange = vi.fn();
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="tcp" />);

    // 디바이스 추가 → 그룹 추가 → 고급 열기 → type_map 항목 추가
    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));
    fireEvent.click(screen.getByRole('button', { name: '그룹 추가' }));
    fireEvent.click(
      screen.getByRole('button', { name: '고급: type_map (주소별 타입)' }),
    );
    fireEvent.click(screen.getByRole('button', { name: '항목 추가' }));

    // 빈 host/id/name/poll_interval 은 방출되지 않는다. byte_order 는 엔트리 레벨.
    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        port: 502,
        register_groups: [
          {
            function_code: 3,
            start_address: 0,
            quantity: 1,
            data_type: 'uint16',
            type_map: [
              { address: 0, data_type: 'uint16', byte_order: 'big_endian' },
            ],
          },
        ],
      },
    ]);
  });

  it('rtu 디바이스는 unit_id 만 방출한다 (host/port 없음)', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor
        value={[{ unit_id: 9, register_groups: [{ function_code: 4, quantity: 3 }] }]}
        onChange={onChange}
        transport="rtu"
      />,
    );

    // 그룹 삭제로 방출을 트리거해 rtu 형상을 확인한다.
    fireEvent.click(screen.getByRole('button', { name: '그룹 삭제' }));

    expect(lastEmit(onChange)).toEqual([{ unit_id: 9, register_groups: [] }]);
  });

  it('고급 type_map 항목 삭제 → 그룹에서 type_map 키를 제거한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor
        value={[
          {
            unit_id: 1,
            host: 'h',
            register_groups: [
              {
                function_code: 3,
                quantity: 2,
                type_map: [
                  { address: 0, data_type: 'uint16', byte_order: 'big_endian' },
                ],
              },
            ],
          },
        ]}
        onChange={onChange}
        transport="tcp"
      />,
    );

    // type_map 엔트리가 있으면 고급 섹션이 자동으로 열린다.
    fireEvent.click(screen.getByRole('button', { name: '항목 삭제' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        host: 'h',
        port: 502,
        register_groups: [
          { function_code: 3, start_address: 0, quantity: 2, data_type: 'uint16' },
        ],
      },
    ]);
  });

  it('readOnly 모드에서는 추가/삭제 버튼을 숨긴다', () => {
    render(
      <ModbusDevicesEditor value={[{ unit_id: 1 }]} onChange={vi.fn()} readOnly />,
    );
    expect(screen.queryByRole('button', { name: '디바이스 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '디바이스 삭제' })).not.toBeInTheDocument();
  });
});
