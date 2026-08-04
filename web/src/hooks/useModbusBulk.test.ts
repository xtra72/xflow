// SPEC-MODBUS-011 — MODBUS Client·Gateway 일괄 등록 파서·실행 훅 단위 테스트.
//
// 포맷 v0.2.0: 한 줄 = 레지스터 그룹/세그먼트. 신원 컬럼이 채워진 행이 디바이스를 시작하고,
// 빈 신원 행은 직전 디바이스에 그룹/세그먼트를 이어 붙인다.
//
// AC-01/02: client 파서(다중 행 그룹핑·이어붙임·헤더 스킵 / 필수·무효 필드 실패 집계).
// AC-03/04: gateway 파서(register_map 방출·shared 세그먼트 / 세그먼트 필수·무효 실패).
// AC-05/06/07: best-effort 실행 훅(전량 성공 / 부분 성공 / 파서+백엔드 실패 합류).

import { createElement, type ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

import {
  EMPTY_SEGMENTS,
  INVALID_FC,
  INVALID_SEGMENT,
  INVALID_SHARED_ADDRESS,
  INVALID_UNIT_ID,
  NO_CURRENT_DEVICE,
  parseModbusClientBulk,
  parseModbusGatewayBulk,
  useModbusClientBulkAdd,
  useModbusGatewayBulkAdd,
} from './useModbusBulk';
import { EMPTY_REQUIRED } from './useStation';

// ---- AC-01/02: Client 파서 ----

describe('parseModbusClientBulk (REQ-01)', () => {
  it('AC-01 — 헤더 스킵 + 다중 행 그룹핑(신원 행 시작, 빈 신원 행 이어붙임)', () => {
    // 2개 그룹, 1개 디바이스: 1행 헤더, 2행 신원+그룹1, 3행 이어붙임 그룹2.
    const text = [
      'host,port,unit_id,fc,address,count,data_type,polling_interval,comment',
      '192.168.0.10,502,1,3,0,10,uint16,5s,온도',
      ',,,1,0,8,uint16,,도어',
    ].join('\n');

    const { devices, failures } = parseModbusClientBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(1);
    expect(devices[0]).toEqual({
      host: '192.168.0.10',
      port: 502,
      unit_id: 1,
      register_groups: [
        { function_code: 3, start_address: 0, quantity: 10, data_type: 'uint16', poll_interval: '5s', name: '온도' },
        { function_code: 1, start_address: 0, quantity: 8, data_type: 'uint16', name: '도어' },
      ],
    });
  });

  it('port 공란(→502 기본 생략), data_type 공란(방출 생략), 다중 디바이스 경계', () => {
    const text = [
      '192.168.0.10,502,1,3,0,10',
      '192.168.0.11,,2,4,100,4',
    ].join('\n');
    const { devices, failures } = parseModbusClientBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(2);
    // data_type 공란 → 방출 생략, port 공란 → 방출 생략.
    expect(devices[0]).toEqual({
      host: '192.168.0.10',
      port: 502,
      unit_id: 1,
      register_groups: [{ function_code: 3, start_address: 0, quantity: 10 }],
    });
    expect(devices[1]).toEqual({
      host: '192.168.0.11',
      unit_id: 2,
      register_groups: [{ function_code: 4, start_address: 100, quantity: 4 }],
    });
    expect('port' in devices[1]!).toBe(false);
    expect('data_type' in devices[0]!.register_groups[0]!).toBe(false);
  });

  it('AC-02 — host 누락 / unit_id 범위 초과 / 무효 fc 를 원본 줄 번호로 실패 집계', () => {
    // 1행 host 공란(선행 콤마)이라 신원 행이지만 host 필수 위반, 2행 unit_id>247, 3행 무효 fc(9).
    const t = [',502,1,3,0,1', '192.168.0.20,502,999,3,0,1', '192.168.0.21,502,3,9,0,1'].join('\n');
    const { devices, failures } = parseModbusClientBulk(t);

    expect(devices).toEqual([]);
    expect(failures).toHaveLength(3);
    expect(failures[0]).toMatchObject({ line: 1, reason: EMPTY_REQUIRED });
    expect(failures[1]).toMatchObject({ line: 2, reason: INVALID_UNIT_ID });
    expect(failures[2]).toMatchObject({ line: 3, reason: INVALID_FC });
  });

  it('선행 이어붙임 행(현재 디바이스 없음)은 NO_CURRENT_DEVICE 실패', () => {
    // 헤더 없이 첫 데이터 행이 빈 신원(이어붙임) → 현재 디바이스 없음.
    const t = [',,,3,0,4', '192.168.0.30,502,5,3,0,4'].join('\n');
    const { devices, failures } = parseModbusClientBulk(t);
    expect(devices).toHaveLength(1);
    expect(failures).toHaveLength(1);
    expect(failures[0]).toMatchObject({ line: 1, reason: NO_CURRENT_DEVICE });
    expect(devices[0]).toEqual({
      host: '192.168.0.30',
      port: 502,
      unit_id: 5,
      register_groups: [{ function_code: 3, start_address: 0, quantity: 4 }],
    });
  });

  it('탭 구분자 자동 감지 + rtu 상속 시 host 미요구/미방출', () => {
    const tab = '192.168.0.30\t502\t5\t3\t0\t4';
    const { devices } = parseModbusClientBulk(tab);
    expect(devices[0]).toEqual({
      host: '192.168.0.30',
      port: 502,
      unit_id: 5,
      register_groups: [{ function_code: 3, start_address: 0, quantity: 4 }],
    });

    // rtu: host 공란 허용, unit_id 로 신원 시작.
    const rtu = parseModbusClientBulk(',,7,3,0,4', 'rtu');
    expect(rtu.failures).toEqual([]);
    expect(rtu.devices[0]).toEqual({
      unit_id: 7,
      register_groups: [{ function_code: 3, start_address: 0, quantity: 4 }],
    });
  });
});

// ---- AC-03/04: Gateway 파서 ----

describe('parseModbusGatewayBulk (REQ-02)', () => {
  it('AC-03 — 다중 세그먼트 그룹핑 + shared 세그먼트(name 생략, data_type 기본 uint16)', () => {
    // 2개 세그먼트, 1개 디바이스; 2번째가 shared. 1행 헤더, 2행 신원+local, 3행 이어붙임 shared.
    const text = [
      'unit_id,name,fc,address,count,data_type,comment',
      '1,meter-A,1,0,8,uint16,도어 센서',
      ',,3,0,10,uint16,shared,200,펌프 상태',
    ].join('\n');

    const { devices, failures } = parseModbusGatewayBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(1);
    expect(devices[0]).toEqual({
      unit_id: 1,
      name: 'meter-A',
      register_map: {
        coils: [{ address: 0, count: 8, data_type: 'uint16', description: '도어 센서' }],
        holding_registers: [{ address: 0, count: 10, shared_address: 200, description: '펌프 상태' }],
      },
    });
    // shared 세그먼트는 data_type 미방출.
    expect('data_type' in devices[0]!.register_map['holding_registers']![0]!).toBe(false);
  });

  it('name 생략(빈 신원 name) + 다중 디바이스 경계 + 숫자 fc → area 매핑', () => {
    const text = ['1,,4,100,4', '2,Meter-B,2,0,16'].join('\n');
    const { devices, failures } = parseModbusGatewayBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(2);
    expect(devices[0]).toEqual({
      unit_id: 1,
      register_map: { input_registers: [{ address: 100, count: 4, data_type: 'uint16' }] },
    });
    expect('name' in devices[0]!).toBe(false);
    expect(devices[1]).toEqual({
      unit_id: 2,
      name: 'Meter-B',
      register_map: { discrete_inputs: [{ address: 0, count: 16, data_type: 'uint16' }] },
    });
  });

  it('AC-04 — 세그먼트 없음(EMPTY_SEGMENTS) / 무효 fc / count<1 / 무효 shared_address 실패 집계', () => {
    // 1행 신원만(세그먼트 없음), 2행 무효 fc(9), 3행 count<1, 4행 shared 마커+비정수 shared_address.
    const text = ['1,NoSeg', '2,BadFc,9,0,1', '3,BadCount,1,0,0', '4,BadShared,3,0,10,uint16,shared,xx'].join('\n');
    const { devices, failures } = parseModbusGatewayBulk(text);

    expect(devices).toEqual([]);
    expect(failures).toHaveLength(4);
    expect(failures[0]).toMatchObject({ line: 1, reason: EMPTY_SEGMENTS });
    expect(failures[1]).toMatchObject({ line: 2, reason: INVALID_FC });
    expect(failures[2]).toMatchObject({ line: 3, reason: INVALID_SEGMENT });
    expect(failures[3]).toMatchObject({ line: 4, reason: INVALID_SHARED_ADDRESS });
  });

  it('unit_id 범위 초과 실패', () => {
    const { devices, failures } = parseModbusGatewayBulk('999,X,1,0,1');
    expect(devices).toEqual([]);
    expect(failures[0]).toMatchObject({ line: 1, reason: INVALID_UNIT_ID });
  });
});

// ---- AC-05/06/07: best-effort 실행 훅 ----

function wrapper() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
}

describe('useModbusClientBulkAdd (REQ-03)', () => {
  beforeEach(() => execAgentMock.mockReset());

  it('AC-05 — 전량 성공: 디바이스별 add_device 3회 순차 호출, BulkResult{total:3,ok:3,failed:[]}', async () => {
    execAgentMock.mockResolvedValue({});
    const text = [
      '192.168.0.1,502,1,3,0,4',
      '192.168.0.2,502,2,3,0,4',
      '192.168.0.3,502,3,3,0,4',
    ].join('\n');

    const { result } = renderHook(() => useModbusClientBulkAdd('agent-x'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync({ text, transport: 'tcp' });

    expect(res).toEqual({ total: 3, ok: 3, failed: [] });
    expect(execAgentMock).toHaveBeenCalledTimes(3);
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'agent-x', {
      command: 'add_device',
      params: { unit_id: 1, register_groups: [{ function_code: 3, start_address: 0, quantity: 4 }], host: '192.168.0.1', port: 502 },
    });
  });

  it('AC-06 — 부분 성공: 2번째 디바이스 백엔드 실패, 1·3 등록 계속, failed 는 백엔드 메시지 원문', async () => {
    execAgentMock
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('duplicate device id'))
      .mockResolvedValueOnce({});
    const text = [
      '192.168.0.1,502,1,3,0,4',
      '192.168.0.2,502,2,3,0,4',
      '192.168.0.3,502,3,3,0,4',
    ].join('\n');

    const { result } = renderHook(() => useModbusClientBulkAdd('agent-x'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync({ text, transport: 'tcp' });

    expect(res.total).toBe(3);
    expect(res.ok).toBe(2);
    expect(res.failed).toHaveLength(1);
    expect(res.failed[0]).toMatchObject({ line: 2, reason: 'duplicate device id' });
    expect(execAgentMock).toHaveBeenCalledTimes(3);
  });

  it('AC-07 — 파서 실패 + 백엔드 실패 합류: ok:1, failed 2건(파서 1 + 백엔드 1)', async () => {
    // 1행 무효(unit_id 999) → 파서 실패. 2·3행 유효, 3행 백엔드 실패.
    execAgentMock.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('backend rejected'));
    const text = [
      '192.168.0.1,502,999,3,0,4',
      '192.168.0.2,502,2,3,0,4',
      '192.168.0.3,502,3,3,0,4',
    ].join('\n');

    const { result } = renderHook(() => useModbusClientBulkAdd('agent-x'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync({ text, transport: 'tcp' });

    expect(res.total).toBe(3);
    expect(res.ok).toBe(1);
    expect(res.failed).toHaveLength(2);
    expect(res.failed.map((f) => f.line).sort()).toEqual([1, 3]);
    expect(execAgentMock).toHaveBeenCalledTimes(2); // 유효 디바이스 2개만 백엔드 호출.
  });
});

describe('useModbusGatewayBulkAdd (REQ-03)', () => {
  beforeEach(() => execAgentMock.mockReset());

  it('전량 성공: add_device params 는 {unit_id,name?,register_map}', async () => {
    execAgentMock.mockResolvedValue({});
    const text = ['1,Meter-A,3,0,10', '2,,1,0,8'].join('\n');

    const { result } = renderHook(() => useModbusGatewayBulkAdd('gw-1'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync(text);

    expect(res).toEqual({ total: 2, ok: 2, failed: [] });
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'gw-1', {
      command: 'add_device',
      params: { unit_id: 1, name: 'Meter-A', register_map: { holding_registers: [{ address: 0, count: 10, data_type: 'uint16' }] } },
    });
  });

  it('세그먼트 없는 디바이스는 백엔드 미호출(EMPTY_SEGMENTS 사전 차단)', async () => {
    execAgentMock.mockResolvedValue({});
    const text = ['1,NoSeg', '2,Ok,1,0,8'].join('\n');

    const { result } = renderHook(() => useModbusGatewayBulkAdd('gw-1'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync(text);

    expect(res.total).toBe(2);
    expect(res.ok).toBe(1);
    expect(res.failed).toHaveLength(1);
    expect(res.failed[0]).toMatchObject({ line: 1, reason: EMPTY_SEGMENTS });
    expect(execAgentMock).toHaveBeenCalledTimes(1); // 유효 디바이스 1개만.
  });
});
