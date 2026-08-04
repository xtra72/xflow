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

import {
  ModbusServerDevicesEditor,
  modbusServerDevicesValid,
  parseBulkSegments,
} from './ModbusServerDevicesEditor';

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

  it('일괄등록(디바이스 레벨): fc 로 영역 분배 + 설명 방출 (로컬 coils + 공유 holding)', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    // 디바이스 레벨 일괄등록 패널 열기(팝업에 단일 버튼).
    fireEvent.click(within(dialog()).getByRole('button', { name: '일괄등록' }));

    // fc 1 = coils(로컬 5열), fc 3 = holding_registers(공유 7열).
    const textarea = within(dialog()).getByRole('textbox', { name: '일괄등록' });
    fireEvent.change(textarea, {
      target: { value: '1,0,8,uint16,door sensor\n3,0,10,uint16,shared,200,pump status' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '적용' }));

    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        register_map: {
          coils: [{ address: 0, count: 8, data_type: 'uint16', description: 'door sensor' }],
          holding_registers: [
            { address: 0, count: 10, shared_address: 200, description: 'pump status' },
          ],
        },
      },
    ]);
  });

  // SPEC-MODBUS-008: 세그먼트 0개 디바이스 저장 차단(백엔드 register_map ≥1 영역 규칙).
  it('세그먼트 0개 디바이스는 경고 배너 + 디바이스별 오류로 표시된다 (저장 차단)', () => {
    const onChange = vi.fn();
    // register_map 이 비어있는(세그먼트 0개) 서빙 디바이스.
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // 상단 경고 배너(role="alert")가 나타난다.
    const banner = screen.getByRole('alert');
    expect(banner).toHaveTextContent(/저장할 수 없습니다/);

    // 디바이스별 인라인 오류가 나타난다.
    expect(
      screen.getByText(/최소 1개 레지스터 세그먼트가 필요합니다/),
    ).toBeInTheDocument();

    // 부모(ModbusDevicesSection)의 저장 게이팅에 쓰이는 검증 헬퍼가 invalid 로 판정한다.
    expect(modbusServerDevicesValid([{ unit_id: 1, register_map: {} }])).toBe(false);
  });

  it('세그먼트가 있는 디바이스는 경고 배너를 표시하지 않는다', () => {
    render(
      <ModbusServerDevicesEditor
        value={[
          {
            unit_id: 1,
            register_map: { holding_registers: [{ address: 0, count: 4 }] },
          },
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('공유 컨테이너(디바이스 0)도 세그먼트 0개면 invalid 로 판정한다', () => {
    // 컨테이너(unit 0) 세그먼트 0 + 서빙 디바이스 세그먼트 있음 → 전체 invalid.
    expect(
      modbusServerDevicesValid([
        { unit_id: 0, register_map: {} },
        { unit_id: 1, register_map: { coils: [{ address: 0, count: 1 }] } },
      ]),
    ).toBe(false);
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

// ── SPEC-MODBUS-010 REQ-06 (AC-13 프론트엔드): 백킹 설정 노출 + 하위 호환 ──
describe('ModbusServerDevicesEditor 백킹(upstream) 설정 (SPEC-MODBUS-010)', () => {
  const BACKING_ENABLE = '실제 디바이스 백킹 활성화';

  it('백킹 미활성 디바이스는 backing 키를 방출하지 않는다 (순수 slave 보존)', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    // 편집 팝업에 백킹 활성 체크박스는 있으나 꺼진 상태로 저장.
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    expect(
      within(dialog()).getByRole('checkbox', { name: BACKING_ENABLE }),
    ).not.toBeChecked();
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    // 방출 디바이스에 backing 키가 전혀 없어야 한다(하위 호환, 백엔드 nil = 순수 slave).
    const emitted = lastEmit(onChange) as Array<Record<string, unknown>>;
    expect(emitted).toEqual([{ unit_id: 1, register_map: {} }]);
    expect('backing' in emitted[0]!).toBe(false);
  });

  it('direct+tcp 백킹 활성화 → transport/mode/unit_id/host/port/timeout 방출', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: BACKING_ENABLE }));
    fireEvent.change(within(dialog()).getByRole('textbox', { name: '호스트' }), {
      target: { value: '10.0.0.5' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        register_map: {},
        backing: {
          transport: 'tcp',
          mode: 'direct',
          unit_id: 1,
          host: '10.0.0.5',
          port: 502,
          timeout: '2s',
        },
      },
    ]);
  });

  it('indirect 모드 → poll_interval + timeout 을 방출한다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: BACKING_ENABLE }));
    fireEvent.change(within(dialog()).getByRole('textbox', { name: '호스트' }), {
      target: { value: '10.0.0.5' },
    });
    fireEvent.change(within(dialog()).getByRole('combobox', { name: '모드' }), {
      target: { value: 'indirect' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 1,
        register_map: {},
        backing: {
          transport: 'tcp',
          mode: 'indirect',
          unit_id: 1,
          host: '10.0.0.5',
          port: 502,
          poll_interval: '1s',
          timeout: '2s',
        },
      },
    ]);
  });

  it('rtu 트랜스포트 → serial_port + 시리얼 파라미터를 number 로 방출한다 (host/port 없음)', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('checkbox', { name: BACKING_ENABLE }));
    fireEvent.change(within(dialog()).getByRole('combobox', { name: '트랜스포트' }), {
      target: { value: 'rtu' },
    });
    fireEvent.change(within(dialog()).getByRole('textbox', { name: '시리얼 포트' }), {
      target: { value: '/dev/ttyUSB0' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    const emitted = lastEmit(onChange) as Array<{ backing: Record<string, unknown> }>;
    const backing = emitted[0]!.backing;
    expect(backing).toEqual({
      transport: 'rtu',
      mode: 'direct',
      unit_id: 1,
      serial_port: '/dev/ttyUSB0',
      baud_rate: 9600,
      data_bits: 8,
      stop_bits: 1,
      parity: 'none',
      timeout: '2s',
    });
    // 백엔드 toInt 는 문자열을 수용하지 않으므로 숫자 필드는 number 여야 한다.
    expect(typeof backing.baud_rate).toBe('number');
    expect(typeof backing.data_bits).toBe('number');
    expect(typeof backing.stop_bits).toBe('number');
    expect('host' in backing).toBe(false);
    expect('port' in backing).toBe(false);
  });

  it('공유 맵(컨테이너, 디바이스 0) 팝업에는 백킹 활성 체크박스가 없다', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[{ unit_id: 1, register_map: {} }]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: '공유 맵 추가' }));
    expect(
      within(dialog()).queryByRole('checkbox', { name: BACKING_ENABLE }),
    ).not.toBeInTheDocument();
  });

  it('backing 이 있는 value 를 라운드트립 파싱해 재방출한다 (활성 상태 복원)', () => {
    const onChange = vi.fn();
    render(
      <ModbusServerDevicesEditor
        value={[
          {
            unit_id: 9,
            register_map: {},
            backing: {
              transport: 'tcp',
              mode: 'indirect',
              unit_id: 3,
              host: 'plc.local',
              port: 600,
              poll_interval: '2s',
              timeout: '10s',
            },
          },
        ]}
        onChange={onChange}
      />,
    );

    // 편집 팝업에 백킹 체크박스가 켜진 상태로 복원된다.
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    expect(
      within(dialog()).getByRole('checkbox', { name: BACKING_ENABLE }),
    ).toBeChecked();
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange)).toEqual([
      {
        unit_id: 9,
        register_map: {},
        backing: {
          transport: 'tcp',
          mode: 'indirect',
          unit_id: 3,
          host: 'plc.local',
          port: 600,
          poll_interval: '2s',
          timeout: '10s',
        },
      },
    ]);
  });
});

describe('parseBulkSegments (fc 기반)', () => {
  it('fc 1-4 → 올바른 영역으로 매핑한다', () => {
    const r = parseBulkSegments(
      '1,0,8,uint16,a\n2,0,4,uint16,b\n3,0,10,uint16,c\n4,0,2,uint16,d',
      true,
    );
    expect(r.errors).toEqual([]);
    expect(r.segments.map((s) => s.area)).toEqual([
      'coils',
      'discrete_inputs',
      'holding_registers',
      'input_registers',
    ]);
  });

  it('5열 로컬 라인 → data_type + description 포함, shared=false', () => {
    const r = parseBulkSegments('1,0,8,int16,door', true);
    expect(r.errors).toEqual([]);
    expect(r.segments).toEqual([
      {
        area: 'coils',
        address: 0,
        count: 8,
        shared: false,
        dataType: 'int16',
        sharedAddress: 0,
        description: 'door',
      },
    ]);
  });

  it('탭 구분 5열 로컬 라인도 파싱한다', () => {
    const r = parseBulkSegments('3\t16\t10\tuint16\ttemp', true);
    expect(r.errors).toEqual([]);
    expect(r.segments[0]).toMatchObject({
      area: 'holding_registers',
      address: 16,
      count: 10,
      shared: false,
      dataType: 'uint16',
      description: 'temp',
    });
  });

  it('data_type 셀이 비면 기본값 uint16', () => {
    const r = parseBulkSegments('1,5,8,,note', true);
    expect(r.errors).toEqual([]);
    expect(r.segments[0]).toMatchObject({ dataType: 'uint16', description: 'note' });
  });

  it('7열 공유 라인 → shared=true, shared_address + description 포함 (data_type 은 방출에서 생략)', () => {
    const r = parseBulkSegments('3,0,10,uint16,shared,200,pump', true);
    expect(r.errors).toEqual([]);
    expect(r.segments).toEqual([
      {
        area: 'holding_registers',
        address: 0,
        count: 10,
        shared: true,
        dataType: 'uint16',
        sharedAddress: 200,
        description: 'pump',
      },
    ]);
  });

  it('헤더 줄(첫 셀 비숫자) 자동 무시', () => {
    const r = parseBulkSegments('fc,주소,개수,타입,설명\n1,0,4,uint16,x', true);
    expect(r.errors).toEqual([]);
    expect(r.segments).toHaveLength(1);
    expect(r.segments[0]).toMatchObject({ area: 'coils', address: 0, count: 4 });
  });

  it('잘못된 fc(5) → invalidFc 오류', () => {
    const r = parseBulkSegments('5,0,8,uint16,x', true);
    expect(r.segments).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'invalidFc' }]);
  });

  it('열 개수 오류(6열) → wrongColumnCount', () => {
    const r = parseBulkSegments('1,0,8,uint16,shared,200', true);
    expect(r.segments).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'wrongColumnCount' }]);
  });

  it('잘못된 data_type → invalidDataType', () => {
    const r = parseBulkSegments('1,0,8,badtype,x', true);
    expect(r.segments).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'invalidDataType' }]);
  });

  it('잘못된 개수(0) → invalidCount', () => {
    const r = parseBulkSegments('1,0,0,uint16,x', true);
    expect(r.errors).toEqual([{ line: 1, code: 'invalidCount' }]);
  });

  it('컨테이너(allowShared=false)에서 7열 공유 라인 → containerNoShared', () => {
    const r = parseBulkSegments('3,0,10,uint16,shared,200,pump', false);
    expect(r.segments).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'containerNoShared' }]);
  });

  it('혼합 붙여넣기: 5열 로컬 + 7열 공유가 함께 파싱된다 (설명 양쪽 포함)', () => {
    const r = parseBulkSegments(
      '1,0,8,uint16,coil desc\n3,0,10,uint16,shared,200,shared desc',
      true,
    );
    expect(r.errors).toEqual([]);
    expect(r.segments).toEqual([
      {
        area: 'coils',
        address: 0,
        count: 8,
        shared: false,
        dataType: 'uint16',
        sharedAddress: 0,
        description: 'coil desc',
      },
      {
        area: 'holding_registers',
        address: 0,
        count: 10,
        shared: true,
        dataType: 'uint16',
        sharedAddress: 200,
        description: 'shared desc',
      },
    ]);
  });

  it('빈 줄은 무시하고, 유효/무효가 섞이면 유효 세그먼트와 오류를 함께 반환한다', () => {
    const r = parseBulkSegments('1,0,4,uint16,a\n\nbad\n2,8,2,uint16,b', true);
    expect(r.segments.map((s) => s.area)).toEqual(['coils', 'discrete_inputs']);
    // 'bad' 는 3번째 줄(원본), 열 개수 오류.
    expect(r.errors).toEqual([{ line: 3, code: 'wrongColumnCount' }]);
  });
});

// SPEC-MODBUS-008: 부모 저장 게이팅 헬퍼(백엔드 register_map ≥1 영역 규칙, config.go:667).
describe('modbusServerDevicesValid', () => {
  it('빈 devices 배열은 유효하다 (디바이스 없음)', () => {
    expect(modbusServerDevicesValid([])).toBe(true);
    expect(modbusServerDevicesValid(undefined)).toBe(true);
    expect(modbusServerDevicesValid('')).toBe(true);
  });

  it('register_map 이 비어있는 디바이스는 invalid', () => {
    expect(modbusServerDevicesValid([{ unit_id: 1, register_map: {} }])).toBe(false);
  });

  it('영역이 1개 이상인 디바이스는 valid', () => {
    expect(
      modbusServerDevicesValid([
        { unit_id: 1, register_map: { coils: [{ address: 0, count: 1 }] } },
      ]),
    ).toBe(true);
  });

  it('여러 디바이스 중 하나라도 세그먼트 0개면 invalid', () => {
    expect(
      modbusServerDevicesValid([
        { unit_id: 1, register_map: { coils: [{ address: 0, count: 1 }] } },
        { unit_id: 2, register_map: {} },
      ]),
    ).toBe(false);
  });

  it('JSON 문자열 value 도 파싱해 검증한다', () => {
    expect(
      modbusServerDevicesValid(
        JSON.stringify([
          { unit_id: 7, register_map: { holding_registers: [{ address: 0, count: 2 }] } },
        ]),
      ),
    ).toBe(true);
    expect(
      modbusServerDevicesValid(JSON.stringify([{ unit_id: 7, register_map: {} }])),
    ).toBe(false);
  });
});
