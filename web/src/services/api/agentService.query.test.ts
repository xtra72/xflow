// agentService 의 exec/query 엔드포인트 분리 검증(SPEC-DASHBOARD-004 후속).
//
// 배경: 읽기 전용 역할(`*.read` 만 보유)이 `/exec`(`agent.execute`)를 호출하면 403 이
// 나고 에이전트 기반 패널이 빈 화면이 된다. 읽기 전용 명령을 `/query`(`agent.read`)로
// 보내 이 결함을 닫는다.
//
// 여기서 검증하는 것:
//   1. 12개 읽기 전용 명령이 실제로 `/agents/{id}/query` URL 로 나간다(엔드포인트 고정).
//   2. 쓰기 명령은 그대로 `/agents/{id}/exec` 로 나간다.
//   3. AGENT_QUERY_COMMANDS 가 서버 화이트리스트와 같은 12개 이름으로 고정된다(드리프트 감지).

import { beforeEach, describe, expect, it, vi } from 'vitest';

const postMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: vi.fn(),
  getList: vi.fn(),
  post: postMock,
  put: vi.fn(),
  del: vi.fn(),
}));

import { AGENT_QUERY_COMMANDS, execAgent, queryAgent } from './agentService';

/**
 * 서버 화이트리스트(internal/api/handler/agent.go 의 `queryReadOnlyCommands`)의 사본.
 *
 * 이 배열은 프론트엔드 상수를 그대로 참조하지 않고 테스트 안에 직접 적어 둔다. 상수를
 * 재사용하면 상수를 고치는 순간 테스트도 같이 따라가 버려 드리프트를 못 잡는다. 서버에
 * 명령이 추가/삭제될 때만 이 목록을 손으로 고쳐야 하며, 그때 서버 코드도 함께 고쳤는지
 * 확인해야 한다.
 */
const SERVER_ALLOWLIST = [
  'list_devices',
  'list_clients',
  'list_stations',
  'list_lines',
  'list_groups',
  'list_gateways',
  'list_connections',
  'list_models',
  'get_status',
  'get_map',
  'get_device_status',
  'get_history',
] as const;

describe('AGENT_QUERY_COMMANDS (드리프트 가드)', () => {
  it('서버 화이트리스트와 정확히 같은 12개 이름을 가진다', () => {
    expect([...AGENT_QUERY_COMMANDS].sort()).toEqual([...SERVER_ALLOWLIST].sort());
  });

  it('12개이며 중복이 없다', () => {
    expect(AGENT_QUERY_COMMANDS).toHaveLength(12);
    expect(new Set(AGENT_QUERY_COMMANDS).size).toBe(12);
  });
});

describe('queryAgent', () => {
  beforeEach(() => {
    postMock.mockReset();
    postMock.mockResolvedValue({ result: {} });
  });

  it.each(SERVER_ALLOWLIST)('%s 는 /agents/{id}/query 로 보낸다', async (command) => {
    await queryAgent('agent-1', { command });

    expect(postMock).toHaveBeenCalledTimes(1);
    expect(postMock).toHaveBeenCalledWith('/agents/agent-1/query', { command });
    // /exec 로 새는 호출이 없어야 한다.
    const urls = postMock.mock.calls.map((c) => c[0] as string);
    expect(urls.some((u) => u.endsWith('/exec'))).toBe(false);
  });

  it('params 를 그대로 전달한다', async () => {
    await queryAgent('agent-1', { command: 'get_device_status', params: { unit_id: 3 } });

    expect(postMock).toHaveBeenCalledWith('/agents/agent-1/query', {
      command: 'get_device_status',
      params: { unit_id: 3 },
    });
  });

  it('거부되면 그대로 실패한다 — /exec 로 재시도하지 않는다', async () => {
    postMock.mockRejectedValueOnce(new Error('400 command not allowed'));

    await expect(queryAgent('agent-1', { command: 'list_devices' })).rejects.toThrow(
      '400 command not allowed',
    );
    expect(postMock).toHaveBeenCalledTimes(1);
    expect(postMock).toHaveBeenCalledWith('/agents/agent-1/query', { command: 'list_devices' });
  });
});

describe('execAgent', () => {
  beforeEach(() => {
    postMock.mockReset();
    postMock.mockResolvedValue({ result: {} });
  });

  it('쓰기 명령은 /agents/{id}/exec 로 보낸다', async () => {
    await execAgent('agent-1', { command: 'set_power', params: { power: true } });

    expect(postMock).toHaveBeenCalledWith('/agents/agent-1/exec', {
      command: 'set_power',
      params: { power: true },
    });
  });

  it('add_device 도 /exec 로 보낸다(agent.execute 필요)', async () => {
    await execAgent('agent-1', { command: 'add_device', params: { address: '20.00.01' } });

    expect(postMock).toHaveBeenCalledWith('/agents/agent-1/exec', {
      command: 'add_device',
      params: { address: '20.00.01' },
    });
  });
});
