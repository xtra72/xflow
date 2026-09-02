// CreateAgentModal — 2열 레이아웃 분기 + 모달 폭 회귀 테스트.
//
// 범위:
//   - TWO_COL_CONFIG 타입(modbus-client)은 넓은 모달(max-w-4xl) + 2열 그리드로
//     렌더하며, 디바이스 편집기(modbus_devices)는 우측 컬럼에 위치한다.
//   - 비-TWO_COL 타입(tcp-client)은 기존 좁은 단일 컬럼(max-w-lg)을 유지한다(회귀 방지).
//
// 데이터 뮤테이션 훅과 i18n 은 스텁으로 격리한다.

import { render, screen, fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import CreateAgentModal from './CreateAgentModal';
import { TWO_COL_CONFIG } from './twoColumnConfigMap';

// 생성 뮤테이션 스텁(reset 은 open effect 에서 호출됨).
vi.mock('@/hooks/useAgent', () => ({
  useCreateAgent: () => ({
    isPending: false,
    isError: false,
    mutateAsync: vi.fn(),
    reset: vi.fn(),
  }),
}));

// i18n 은 키를 그대로 반환한다(라벨은 스키마의 field.label 을 직접 사용하므로 무관).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

/** 모달 컨테이너(role=dialog 의 직속 자식 div)를 반환한다. */
function getModalContainer(): HTMLElement {
  const dialog = screen.getByRole('dialog');
  const container = dialog.querySelector(':scope > div');
  if (!(container instanceof HTMLElement)) throw new Error('modal container not found');
  return container;
}

/** 타입 셀렉트 값을 변경한다. */
function selectType(value: string): void {
  const select = document.getElementById('agent-type');
  if (!(select instanceof HTMLSelectElement)) throw new Error('type select not found');
  fireEvent.change(select, { target: { value } });
}

describe('CreateAgentModal 2열 레이아웃 분기', () => {
  it('modbus-client 는 넓은 모달 + 2열 그리드로 렌더하고 디바이스 편집기를 우측 컬럼에 둔다', () => {
    render(<CreateAgentModal open onClose={vi.fn()} />);
    selectType('modbus-client');

    // (a) 모달이 넓어진다.
    const container = getModalContainer();
    expect(container.className).toContain('max-w-4xl');
    expect(container.className).not.toContain('max-w-lg');

    // (b) 2열 그리드가 존재하고 디바이스 필드(디바이스 설정)가 우측(두 번째) 컬럼에 있다.
    const grid = container.querySelector('.grid.grid-cols-2');
    expect(grid).not.toBeNull();
    const columns = grid!.querySelectorAll(':scope > div');
    expect(columns.length).toBe(2);
    const rightColumn = columns[1] as HTMLElement;
    expect(rightColumn.textContent).toContain('디바이스 설정');
    // 좌측 컬럼에는 디바이스 필드가 없어야 한다(스칼라 필드만).
    expect((columns[0] as HTMLElement).textContent).not.toContain('디바이스 설정');
  });

  it('비-TWO_COL 타입(tcp-client)은 좁은 단일 컬럼 모달을 유지한다(회귀 방지)', () => {
    render(<CreateAgentModal open onClose={vi.fn()} />);
    selectType('tcp-client');

    const container = getModalContainer();
    expect(container.className).toContain('max-w-lg');
    expect(container.className).not.toContain('max-w-4xl');
    // 2열 그리드가 없어야 한다(단일 컬럼 DynamicForm 경로).
    expect(container.querySelector('.grid.grid-cols-2')).toBeNull();
  });

  it('samsung_hvacr01 은 넓은 모달 + 연결|운영 2열 그리드로 렌더한다', () => {
    render(<CreateAgentModal open onClose={vi.fn()} />);
    selectType('samsung_hvacr01');

    const container = getModalContainer();
    expect(container.className).toContain('max-w-4xl');

    const grid = container.querySelector('.grid.grid-cols-2');
    expect(grid).not.toBeNull();
    const columns = grid!.querySelectorAll(':scope > div');
    expect(columns.length).toBe(2);

    // 좌(연결): 연결 방식(transport_type)이 좌측 컬럼에 위치한다.
    expect((columns[0] as HTMLElement).textContent).toContain('연결 방식');
    // 우(운영): 상태 확인 요청(운영 필드)이 우측 컬럼에, 좌측엔 없어야 한다.
    expect((columns[1] as HTMLElement).textContent).toContain('상태 확인 요청 활성');
    expect((columns[0] as HTMLElement).textContent).not.toContain('상태 확인 요청 활성');
  });

  it('chirpstack 은 넓은 모달 + 연결|운영 2열 그리드로 렌더한다', () => {
    render(<CreateAgentModal open onClose={vi.fn()} />);
    selectType('chirpstack-client');

    const container = getModalContainer();
    expect(container.className).toContain('max-w-4xl');

    const grid = container.querySelector('.grid.grid-cols-2');
    expect(grid).not.toBeNull();
    const columns = grid!.querySelectorAll(':scope > div');
    expect(columns.length).toBe(2);

    // 좌(연결): 브로커 주소. 우(운영): 구독 토픽.
    expect((columns[0] as HTMLElement).textContent).toContain('브로커 주소');
    expect((columns[1] as HTMLElement).textContent).toContain('구독 토픽');
    expect((columns[0] as HTMLElement).textContent).not.toContain('구독 토픽');
  });

  it('chirpstack 좌측(연결) 컬럼은 연결 노브만, 운영 노브는 우측에 둔다', () => {
    const entry = TWO_COL_CONFIG['chirpstack-client'];
    expect(entry).toBeDefined();
    const left = entry!.left;
    for (const k of ['broker', 'client_id', 'username', 'password', 'keep_alive_sec', 'connect_timeout_sec', 'auto_reconnect', 'clean_session']) {
      expect(left.has(k), `연결 컬럼 누락: ${k}`).toBe(true);
    }
    // 운영 필드는 "left 에 없는 전부" 규칙으로 우측에 배치된다.
    for (const k of ['topics', 'qos', 'buffer_size', 'emit_comm_state', 'comm_report_interval', 'offline_threshold', 'measurement_emit_mode', 'timestamp_source']) {
      expect(left.has(k), `운영 필드가 연결 컬럼에 잘못 포함: ${k}`).toBe(false);
    }
  });

  it('samsung_hvacr01 좌측(연결) 컬럼 집합에 MQTT 보안 필드가 포함된다', () => {
    const entry = TWO_COL_CONFIG['samsung_hvacr01'];
    expect(entry).toBeDefined();
    const left = entry!.left;
    for (const k of ['mirror_broker', 'mirror_username', 'mirror_password', 'mirror_tls', 'mirror_ca_cert']) {
      expect(left.has(k), `연결 컬럼 누락: ${k}`).toBe(true);
    }
  });
});
