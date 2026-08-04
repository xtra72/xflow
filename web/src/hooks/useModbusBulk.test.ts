// SPEC-MODBUS-011 — MODBUS Client·Gateway 일괄 등록 파서·실행 훅 단위 테스트.
//
// AC-01/02: client 파서(정상 방출 형상 / 필수·무효 필드 실패 집계).
// AC-03/04: gateway 파서(register_map 방출 / 세그먼트 필수·무효 실패).
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
  INVALID_GROUP,
  INVALID_SEGMENT,
  INVALID_UNIT_ID,
  parseModbusClientBulk,
  parseModbusGatewayBulk,
  useModbusClientBulkAdd,
  useModbusGatewayBulkAdd,
} from './useModbusBulk';
import { EMPTY_REQUIRED } from './useStation';

// ---- AC-01/02: Client 파서 ----

describe('parseModbusClientBulk (REQ-01)', () => {
  it('AC-01 — 헤더 스킵 + 정상 방출 형상(id 생략, port 공란 생략, groups)', () => {
    const text = [
      'host,port,unit_id,id,groups',
      '192.168.0.10,502,1,,3:0:10:uint16;4:100:4',
      '192.168.0.11,,2',
    ].join('\n');

    const { devices, failures } = parseModbusClientBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(2);
    expect(devices[0]).toEqual({
      host: '192.168.0.10',
      port: 502,
      unit_id: 1,
      register_groups: [
        { function_code: 3, start_address: 0, quantity: 10, data_type: 'uint16' },
        { function_code: 4, start_address: 100, quantity: 4 },
      ],
    });
    // id 키 없음, port 공란 → 방출 생략, groups 빈 배열.
    expect(devices[1]).toEqual({ host: '192.168.0.11', unit_id: 2, register_groups: [] });
    expect('id' in devices[0]!).toBe(false);
    expect('data_type' in devices[0]!.register_groups[1]!).toBe(false);
  });

  it('AC-02 — host 누락 / unit_id 범위 초과 / 무효 fc 를 원본 줄 번호로 실패 집계, 유효 device 0개', () => {
    // 1행 host 공란(선행 콤마), 2행 unit_id>247, 3행 무효 fc(9). 헤더 없음(1행 unit_id 정수라 데이터 취급).
    const t = [',502,1', '192.168.0.20,502,999', '192.168.0.21,502,3,,9:0:1'].join('\n');
    const { devices, failures } = parseModbusClientBulk(t);

    expect(devices).toEqual([]);
    expect(failures).toHaveLength(3);
    expect(failures[0]).toMatchObject({ line: 1, reason: EMPTY_REQUIRED });
    expect(failures[1]).toMatchObject({ line: 2, reason: INVALID_UNIT_ID });
    expect(failures[2]).toMatchObject({ line: 3, reason: INVALID_GROUP });
  });

  it('탭 구분자 자동 감지 + rtu 상속 시 host 미요구/미방출', () => {
    const tab = '192.168.0.30\t502\t5\tdev-a\t3:0:4';
    const { devices } = parseModbusClientBulk(tab);
    expect(devices[0]).toEqual({
      host: '192.168.0.30',
      port: 502,
      unit_id: 5,
      id: 'dev-a',
      register_groups: [{ function_code: 3, start_address: 0, quantity: 4 }],
    });

    const rtu = parseModbusClientBulk(',,7', 'rtu');
    expect(rtu.failures).toEqual([]);
    expect(rtu.devices[0]).toEqual({ unit_id: 7, register_groups: [] });
  });
});

// ---- AC-03/04: Gateway 파서 ----

describe('parseModbusGatewayBulk (REQ-02)', () => {
  it('AC-03 — 정상 파싱 및 register_map 방출(name 생략, data_type 기본 uint16)', () => {
    const text = [
      'unit_id,name,segments',
      '1,Meter-A,holding_registers:0:10:uint16;input_registers:100:4',
      '2,,coils:0:8',
    ].join('\n');

    const { devices, failures } = parseModbusGatewayBulk(text);

    expect(failures).toEqual([]);
    expect(devices).toHaveLength(2);
    expect(devices[0]).toEqual({
      unit_id: 1,
      name: 'Meter-A',
      register_map: {
        holding_registers: [{ address: 0, count: 10, data_type: 'uint16' }],
        input_registers: [{ address: 100, count: 4, data_type: 'uint16' }],
      },
    });
    expect(devices[1]).toEqual({
      unit_id: 2,
      register_map: { coils: [{ address: 0, count: 8, data_type: 'uint16' }] },
    });
    expect('name' in devices[1]!).toBe(false);
  });

  it('AC-04 — 세그먼트 없음 / 무효 area / count<1 실패 집계, 유효 params 0개', () => {
    const text = ['1,NoSeg,', '2,BadArea,foo:0:1', '3,BadCount,coils:0:0'].join('\n');
    const { devices, failures } = parseModbusGatewayBulk(text);

    expect(devices).toEqual([]);
    expect(failures).toHaveLength(3);
    expect(failures[0]).toMatchObject({ line: 1, reason: EMPTY_SEGMENTS });
    expect(failures[1]).toMatchObject({ line: 2, reason: INVALID_SEGMENT });
    expect(failures[2]).toMatchObject({ line: 3, reason: INVALID_SEGMENT });
  });

  it('숫자 area(1-4)와 다중 세그먼트 병합', () => {
    const { devices, failures } = parseModbusGatewayBulk('10,Multi,3:0:2;3:10:4;1:0:8');
    expect(failures).toEqual([]);
    expect(devices[0]).toEqual({
      unit_id: 10,
      name: 'Multi',
      register_map: {
        holding_registers: [
          { address: 0, count: 2, data_type: 'uint16' },
          { address: 10, count: 4, data_type: 'uint16' },
        ],
        coils: [{ address: 0, count: 8, data_type: 'uint16' }],
      },
    });
  });

  it('unit_id 범위 초과 실패', () => {
    const { devices, failures } = parseModbusGatewayBulk('999,X,coils:0:1');
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

  it('AC-05 — 전량 성공: add_device 3회 순차 호출, BulkResult{total:3,ok:3,failed:[]}', async () => {
    execAgentMock.mockResolvedValue({});
    const text = [
      '192.168.0.1,502,1,,3:0:4',
      '192.168.0.2,502,2,,3:0:4',
      '192.168.0.3,502,3,,3:0:4',
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

  it('AC-06 — 부분 성공: 2번째 행 백엔드 실패, 1·3 등록 계속, failed 는 백엔드 메시지 원문', async () => {
    execAgentMock
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('duplicate device id'))
      .mockResolvedValueOnce({});
    const text = [
      '192.168.0.1,502,1,,3:0:4',
      '192.168.0.2,502,2,,3:0:4',
      '192.168.0.3,502,3,,3:0:4',
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
      '192.168.0.1,502,999,,3:0:4',
      '192.168.0.2,502,2,,3:0:4',
      '192.168.0.3,502,3,,3:0:4',
    ].join('\n');

    const { result } = renderHook(() => useModbusClientBulkAdd('agent-x'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync({ text, transport: 'tcp' });

    expect(res.total).toBe(3);
    expect(res.ok).toBe(1);
    expect(res.failed).toHaveLength(2);
    expect(res.failed.map((f) => f.line).sort()).toEqual([1, 3]);
    expect(execAgentMock).toHaveBeenCalledTimes(2); // 유효 행 2개만 백엔드 호출.
  });
});

describe('useModbusGatewayBulkAdd (REQ-03)', () => {
  beforeEach(() => execAgentMock.mockReset());

  it('전량 성공: add_device params 는 {unit_id,name?,register_map}', async () => {
    execAgentMock.mockResolvedValue({});
    const text = ['1,Meter-A,holding_registers:0:10', '2,,coils:0:8'].join('\n');

    const { result } = renderHook(() => useModbusGatewayBulkAdd('gw-1'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync(text);

    expect(res).toEqual({ total: 2, ok: 2, failed: [] });
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'gw-1', {
      command: 'add_device',
      params: { unit_id: 1, name: 'Meter-A', register_map: { holding_registers: [{ address: 0, count: 10, data_type: 'uint16' }] } },
    });
  });

  it('세그먼트 없는 행은 백엔드 미호출(EMPTY_SEGMENTS 사전 차단)', async () => {
    execAgentMock.mockResolvedValue({});
    const text = ['1,NoSeg,', '2,Ok,coils:0:8'].join('\n');

    const { result } = renderHook(() => useModbusGatewayBulkAdd('gw-1'), { wrapper: wrapper() });
    const res = await result.current.mutateAsync(text);

    expect(res.total).toBe(2);
    expect(res.ok).toBe(1);
    expect(res.failed).toHaveLength(1);
    expect(res.failed[0]).toMatchObject({ line: 1, reason: EMPTY_SEGMENTS });
    expect(execAgentMock).toHaveBeenCalledTimes(1); // 유효 행 1개만.
  });
});
