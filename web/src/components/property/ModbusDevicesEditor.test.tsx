// ModbusDevicesEditor(modbus-client) 컴포넌트 테스트.
//
// Server 에디터와 동일한 UX(컴팩트 목록 + 팝업 + 영역별 one-line 행 + 일괄등록)로 재구성됨.
// register 설정은 4개 영역으로 조직하고, 영역이 function_code 를 유도한다
// (1=coils 2=discrete_inputs 3=holding_registers 4=input_registers).
//
// 검증 대상:
//   - 빈 value → 안내 문구
//   - legacy JSON 문자열 파싱 → 목록 렌더
//   - transport tcp: 팝업에 host/port 노출·방출 / rtu: 숨김·방출 제외
//   - register_groups round-trip (function_code / start_address / quantity / data_type /
//     poll_interval / name / type_map 보존, fc↔영역)
//   - 디바이스 레벨 일괄등록: fc→영역 분배 + poll_interval/name 방출
//   - parseBulkGroups 단위 테스트
//   - readOnly 모드 버튼 숨김
//
// i18n: ko.json 을 점 표기로 해석하는 mock.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { ModbusDevicesEditor, parseBulkGroups } from './ModbusDevicesEditor';

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

function dialog(): HTMLElement {
  return screen.getByRole('dialog');
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

  it('legacy JSON 문자열 value 를 파싱해 목록을 렌더링한다', () => {
    const json = '[{"unit_id":7,"host":"10.0.0.5","register_groups":[]}]';
    render(<ModbusDevicesEditor value={json} onChange={vi.fn()} transport="tcp" />);
    expect(screen.getByText(/유닛 ID: 7/)).toBeInTheDocument();
    expect(screen.getByText(/10\.0\.0\.5:502/)).toBeInTheDocument();
  });

  it('transport=tcp 는 팝업에 host/port 를 노출한다', () => {
    render(
      <ModbusDevicesEditor
        value={[{ unit_id: 1, host: 'h', register_groups: [] }]}
        onChange={vi.fn()}
        transport="tcp"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    expect(within(dialog()).getByText('호스트')).toBeInTheDocument();
    expect(within(dialog()).getByText('포트')).toBeInTheDocument();
    expect(within(dialog()).getByText('유닛 ID')).toBeInTheDocument();
  });

  it('transport=rtu 는 팝업에서 host/port 를 숨긴다', () => {
    render(
      <ModbusDevicesEditor
        value={[{ unit_id: 1, register_groups: [] }]}
        onChange={vi.fn()}
        transport="rtu"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    expect(within(dialog()).queryByText('호스트')).not.toBeInTheDocument();
    expect(within(dialog()).queryByText('포트')).not.toBeInTheDocument();
    expect(within(dialog()).getByText('유닛 ID')).toBeInTheDocument();
  });

  it('디바이스 삭제 → 빈 배열을 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor
        value={[{ unit_id: 1, host: 'h', register_groups: [] }]}
        onChange={onChange}
        transport="tcp"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '디바이스 삭제' }));
    expect(lastEmit(onChange)).toEqual([]);
  });

  it('디바이스 추가(tcp): 팝업에서 host 입력 + 보유레지스터 세그먼트 → register_groups(fc3) 방출', () => {
    const onChange = vi.fn();
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="tcp" />);

    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));
    // host 입력(필수).
    fireEvent.change(within(dialog()).getByPlaceholderText('192.168.1.10'), {
      target: { value: '10.0.0.9' },
    });
    // holding_registers(3번째) 세그먼트 추가.
    const addBtns = within(dialog()).getAllByRole('button', { name: '세그먼트 추가' });
    fireEvent.click(addBtns[2]!);
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        host: '10.0.0.9',
        port: 502,
        register_groups: [
          { function_code: 3, start_address: 0, quantity: 1, data_type: 'uint16' },
        ],
      },
    ]);
  });

  it('register_groups round-trip: fc↔영역 그룹핑 후 저장하면 function_code/type_map 을 보존한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor
        value={[
          {
            id: 'dev1',
            host: '10.0.0.5',
            port: 502,
            unit_id: 7,
            register_groups: [
              {
                function_code: 3,
                start_address: 0,
                quantity: 10,
                data_type: 'uint16',
                poll_interval: '5s',
                name: 'temp',
                type_map: [{ address: 0, data_type: 'float32', byte_order: 'big_endian' }],
              },
              { function_code: 1, start_address: 0, quantity: 8, data_type: 'uint16' },
            ],
          },
        ]}
        onChange={onChange}
        transport="tcp"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    // 방출은 영역 순서(coils → holding)로 정렬된다. function_code 는 영역에서 유도되어 보존.
    expect(lastEmit(onChange)).toEqual([
      {
        id: 'dev1',
        host: '10.0.0.5',
        port: 502,
        unit_id: 7,
        register_groups: [
          { function_code: 1, start_address: 0, quantity: 8, data_type: 'uint16' },
          {
            function_code: 3,
            start_address: 0,
            quantity: 10,
            data_type: 'uint16',
            poll_interval: '5s',
            name: 'temp',
            type_map: [{ address: 0, data_type: 'float32', byte_order: 'big_endian' }],
          },
        ],
      },
    ]);
  });

  it('rtu 디바이스는 host/port 를 방출하지 않는다', () => {
    const onChange = vi.fn();
    render(
      <ModbusDevicesEditor
        value={[
          { unit_id: 9, register_groups: [{ function_code: 4, start_address: 0, quantity: 3 }] },
        ]}
        onChange={onChange}
        transport="rtu"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 9,
        register_groups: [
          { function_code: 4, start_address: 0, quantity: 3, data_type: 'uint16' },
        ],
      },
    ]);
  });

  it('일괄등록: fc 로 영역 분배 + poll_interval/name 방출 (holding + coils)', () => {
    const onChange = vi.fn();
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="tcp" />);

    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));
    fireEvent.change(within(dialog()).getByPlaceholderText('192.168.1.10'), {
      target: { value: '10.0.0.9' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '일괄등록' }));

    const textarea = within(dialog()).getByRole('textbox', { name: '일괄등록' });
    fireEvent.change(textarea, {
      target: { value: '3,0,10,uint16,5s,temp\n1,0,8,uint16,,door' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '적용' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        host: '10.0.0.9',
        port: 502,
        register_groups: [
          { function_code: 1, start_address: 0, quantity: 8, data_type: 'uint16', name: 'door' },
          {
            function_code: 3,
            start_address: 0,
            quantity: 10,
            data_type: 'uint16',
            poll_interval: '5s',
            name: 'temp',
          },
        ],
      },
    ]);
  });

  it('readOnly 모드에서는 추가/삭제 버튼을 숨긴다', () => {
    render(
      <ModbusDevicesEditor
        value={[{ unit_id: 1, host: 'h', register_groups: [] }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '디바이스 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '디바이스 삭제' })).not.toBeInTheDocument();
  });
});

describe('DeviceEditDialog 검증 UX (필수 표식 + 제출 후 오류 + 저장 말풍선)', () => {
  // 추가 다이얼로그를 연다(tcp → host 필수, 최초엔 비어있어 저장 불가).
  const openAddDialog = (onChange = vi.fn()) => {
    render(<ModbusDevicesEditor value={[]} onChange={onChange} transport="tcp" />);
    fireEvent.click(screen.getByRole('button', { name: '디바이스 추가' }));
    return onChange;
  };

  it('(a) 최초 열림: 빈 필수 필드라도 빨간 테두리/말풍선을 표시하지 않는다', () => {
    openAddDialog();
    // 저장 차단 말풍선(alert) 없음.
    expect(within(dialog()).queryByRole('alert')).toBeNull();
    // host 입력에 빨간 테두리 클래스가 없다(제출 전).
    const host = within(dialog()).getByPlaceholderText('192.168.1.10');
    expect(host.className).not.toContain('border-red-400');
    // 필수 표식(*)은 노출된다(유닛 ID/호스트 라벨).
    expect(within(dialog()).getAllByText('*').length).toBeGreaterThanOrEqual(2);
  });

  it('(b) 필수 누락 상태로 저장 시도: 말풍선 + 빨간 테두리를 표시하고 저장하지 않는다', () => {
    const onChange = openAddDialog();
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    // 말풍선(alert) + 제목 + 호스트 필수 메시지가 노출된다.
    expect(within(dialog()).getByRole('alert')).toBeInTheDocument();
    expect(within(dialog()).getByText('다음 항목을 확인하세요')).toBeInTheDocument();
    expect(
      within(dialog()).getByText('TCP 디바이스는 호스트가 필요합니다.'),
    ).toBeInTheDocument();
    // host 입력에 빨간 테두리가 생긴다(제출 후).
    const host = within(dialog()).getByPlaceholderText('192.168.1.10');
    expect(host.className).toContain('border-red-400');
    // 저장은 발생하지 않는다.
    expect(onChange).not.toHaveBeenCalled();
  });

  it('(c) 필수 채운 뒤 저장: onSave(onChange) 가 호출되고 말풍선이 사라진다', () => {
    const onChange = openAddDialog();
    // 먼저 빈 상태로 저장 시도 → 말풍선 노출.
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));
    expect(within(dialog()).getByRole('alert')).toBeInTheDocument();

    // host 를 채우면 말풍선이 자동으로 사라진다(canSave true).
    fireEvent.change(within(dialog()).getByPlaceholderText('192.168.1.10'), {
      target: { value: '10.0.0.9' },
    });
    expect(screen.queryByRole('alert')).toBeNull();

    // 다시 저장 → 방출된다.
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));
    expect(lastEmit(onChange)).toEqual([
      { unit_id: 1, host: '10.0.0.9', port: 502, register_groups: [] },
    ]);
  });
});

describe('parseBulkGroups (fc 기반)', () => {
  it('fc 1-4 → 올바른 영역으로 매핑한다', () => {
    const r = parseBulkGroups(
      '1,0,8,uint16,,a\n2,0,4,uint16,,b\n3,0,10,uint16,,c\n4,0,2,uint16,,d',
    );
    expect(r.errors).toEqual([]);
    expect(r.groups.map((g) => g.area)).toEqual([
      'coils',
      'discrete_inputs',
      'holding_registers',
      'input_registers',
    ]);
  });

  it('6열: poll_interval + 설명(name) 포함', () => {
    const r = parseBulkGroups('3,16,10,int16,5s,temp');
    expect(r.errors).toEqual([]);
    expect(r.groups).toEqual([
      {
        area: 'holding_registers',
        address: 16,
        quantity: 10,
        dataType: 'int16',
        pollInterval: '5s',
        name: 'temp',
      },
    ]);
  });

  it('5열(설명 생략) → name 은 빈 문자열', () => {
    const r = parseBulkGroups('1,0,8,uint16,1s');
    expect(r.errors).toEqual([]);
    expect(r.groups[0]).toMatchObject({ area: 'coils', pollInterval: '1s', name: '' });
  });

  it('poll_interval 빈 셀 허용 + data_type 빈 셀은 uint16', () => {
    const r = parseBulkGroups('3,0,10,,,note');
    expect(r.errors).toEqual([]);
    expect(r.groups[0]).toMatchObject({ dataType: 'uint16', pollInterval: '', name: 'note' });
  });

  it('탭 구분도 파싱한다', () => {
    const r = parseBulkGroups('4\t0\t2\tuint16\t\tin');
    expect(r.errors).toEqual([]);
    expect(r.groups[0]).toMatchObject({ area: 'input_registers', address: 0, quantity: 2 });
  });

  it('헤더 줄(첫 셀 비숫자) 자동 무시', () => {
    const r = parseBulkGroups('fc,주소,개수,타입,폴링,설명\n3,0,4,uint16,,x');
    expect(r.errors).toEqual([]);
    expect(r.groups).toHaveLength(1);
  });

  it('잘못된 fc(5) → invalidFc', () => {
    const r = parseBulkGroups('5,0,8,uint16,,x');
    expect(r.groups).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'invalidFc' }]);
  });

  it('열 개수 오류(4열/7열) → wrongColumnCount', () => {
    expect(parseBulkGroups('3,0,8,uint16').errors).toEqual([
      { line: 1, code: 'wrongColumnCount' },
    ]);
    expect(parseBulkGroups('3,0,8,uint16,5s,x,extra').errors).toEqual([
      { line: 1, code: 'wrongColumnCount' },
    ]);
  });

  it('잘못된 data_type → invalidDataType', () => {
    const r = parseBulkGroups('3,0,8,badtype,,x');
    expect(r.errors).toEqual([{ line: 1, code: 'invalidDataType' }]);
  });

  it('잘못된 개수(0) → invalidCount', () => {
    const r = parseBulkGroups('3,0,0,uint16,,x');
    expect(r.errors).toEqual([{ line: 1, code: 'invalidCount' }]);
  });

  it('빈 줄 무시 + 유효/무효 혼합 시 유효 그룹과 오류를 함께 반환', () => {
    const r = parseBulkGroups('1,0,4,uint16,,a\n\nbad\n3,8,2,uint16,,b');
    expect(r.groups.map((g) => g.area)).toEqual(['coils', 'holding_registers']);
    expect(r.errors).toEqual([{ line: 3, code: 'wrongColumnCount' }]);
  });
});
