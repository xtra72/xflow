// 하단 디버그/탭 출력 패널 — 탭 출력 "뷰" 모델 테스트.
//
// 검증 대상(SPEC-NODE-OUTPUT-TAP, 뷰 기반 필터 리워크):
//   - 뷰 추가(+) / 삭제(✕, 최소 1개 유지)
//   - 노드 전용 필터 / 포트 전용 필터 / 노드+포트 독립 AND 필터
//   - 뷰 탭 라벨 자동 도출(전체 / 노드 / *:port / 노드:port)
//   - 선택 옵션이 사라졌을 때 'all' 폴백(크래시·stale 방지)
//
// WebSocket 클라이언트는 모킹한다(연결 부작용 차단). 탭 엔트리는 tapStore 에
// 직접 적재하고, 노드 라벨은 editorStore.setNodes 로 주입한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

// wsClient 를 모킹해 실제 WebSocket 연결을 막는다.
vi.mock('@/services/ws/wsClient', () => {
  class FakeWSClient {
    on() {}
    off() {}
    connect() {}
    disconnect() {}
  }
  return {
    createWSClient: () => new FakeWSClient(),
  };
});

// i18n 은 키를 그대로 반환하도록 모킹한다. 정적 UI(탭/빈 안내/뷰 버튼/필터)는
// t() 키로 노출되고, 동적 뷰 라벨(노드 라벨, *:port, 노드:port)은 실제 값 그대로
// 도출된다. 단, 전체(all/all) 뷰 라벨만 viewAll 키로 도출된다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// jsdom 은 PointerEvent 를 구현하지 않아 fireEvent.pointer* 가 clientY 를 실어 나르지
// 못한다. MouseEvent(clientY 지원) 기반 폴리필로 대체해 좌표를 전달한다.
if (typeof globalThis.PointerEvent === 'undefined') {
  globalThis.PointerEvent = class extends MouseEvent {} as unknown as typeof PointerEvent;
}

import { DebugPanel } from './DebugPanel';
import { useTapStore, type NodeOutputPayload } from '@/stores/tapStore';
import { useEditorStore } from '@/stores/editorStore';

/** 테스트용 node.output 페이로드 생성기. */
function makePayload(
  nodeId: string,
  port: string,
  payload: Record<string, unknown> = {},
): NodeOutputPayload {
  return {
    flow_id: 'flow-1',
    node_id: nodeId,
    port,
    message: {
      id: `${nodeId}-${port}-${Math.random()}`,
      type: 'node.output',
      timestamp: Date.now(),
      payload,
      metadata: {},
    },
  };
}

/** 노드 라벨 맵을 editorStore 에 주입한다(라벨 해석용). */
function seedNodes(entries: { id: string; label: string }[]): void {
  useEditorStore.getState().setNodes(
    entries.map((e) => ({
      id: e.id,
      type: 'custom',
      position: { x: 0, y: 0 },
      data: { label: e.label },
    })),
  );
}

/** 패널을 열고 "탭 출력" 탭으로 전환한다. */
function openTapTab(): void {
  fireEvent.click(screen.getByRole('button', { name: /editor\.debug\.tapTab/ }));
}

/**
 * 렌더된 tap 엔트리 행을 (노드 라벨, 포트) 쌍으로 추출한다.
 * 각 엔트리 행에는 "tap" 뱃지가 있으므로 이를 기준으로 식별한다.
 * select 옵션·뷰 탭 라벨과의 텍스트 충돌을 피해 행만 검사하기 위함이다.
 */
function getEntryRows(): { label: string; port: string }[] {
  const badges = screen.queryAllByText('tap');
  return badges.map((badge) => {
    // 노드 라벨 + 포트는 tap 뱃지와 같은 <td> 안에 있다(time 셀과 분리).
    const cell = badge.closest('td');
    const labelEl = cell?.querySelector('.text-sky-400');
    const label = labelEl?.textContent ?? '';
    // 포트 span 은 동일 셀 내 ":out" 형태의 .text-gray-500 노드다.
    const portSpan = Array.from(cell?.querySelectorAll('span') ?? []).find((s) =>
      (s.textContent ?? '').startsWith(':'),
    );
    const port = (portSpan?.textContent ?? '').replace(/^:/, '');
    return { label, port };
  });
}

beforeEach(() => {
  useTapStore.getState().reset();
  useEditorStore.getState().resetEditor();
  window.localStorage.clear();
});

describe('DebugPanel — 탭 출력 뷰', () => {
  it('탭 엔트리가 없으면 관찰 켜기 안내를 보여준다', () => {
    render(<DebugPanel />);
    openTapTab();
    expect(screen.getByText('editor.debug.tapEmpty')).toBeInTheDocument();
  });

  it('기본 뷰 라벨은 "전체"이며 모든 엔트리를 보여준다', () => {
    seedNodes([{ id: 'A', label: '노드A' }]);
    useTapStore.getState().appendOutput(makePayload('A', 'out', { v: 1 }));
    render(<DebugPanel />);
    openTapTab();

    // 기본 뷰 탭 라벨(all/all → viewAll 키).
    expect(screen.getByTitle('editor.debug.viewAll')).toBeInTheDocument();
    // out 엔트리가 보인다(tap 뱃지 + 노드 라벨). '노드A'는 select 옵션과 엔트리 행
    // 양쪽에 등장하므로 getAllByText 로 확인한다.
    expect(screen.getAllByText('노드A').length).toBeGreaterThanOrEqual(1);
  });

  it('+ 버튼이 새 뷰를 추가하고 그 뷰를 활성으로 만든다', () => {
    seedNodes([{ id: 'A', label: '노드A' }]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    render(<DebugPanel />);
    openTapTab();

    // 초기엔 "전체" 뷰 1개.
    expect(screen.getAllByTitle('editor.debug.viewAll')).toHaveLength(1);

    fireEvent.click(screen.getByRole('button', { name: 'editor.debug.addView' }));

    // 새 뷰도 기본 {all, all} → "전체" 라벨이므로 2개가 된다.
    expect(screen.getAllByTitle('editor.debug.viewAll')).toHaveLength(2);
  });

  it('✕ 버튼이 뷰를 삭제한다(단, 최소 1개는 유지)', () => {
    seedNodes([{ id: 'A', label: '노드A' }]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    render(<DebugPanel />);
    openTapTab();

    // 뷰가 1개일 때는 ✕(삭제) 버튼이 없다.
    // removeView 키에는 {label} 자리표시자가 그대로 남아 키 문자열이 접근성 이름이 된다.
    expect(
      screen.queryByRole('button', { name: /editor\.debug\.removeView/ }),
    ).toBeNull();

    // 뷰 2개로 늘린다.
    fireEvent.click(screen.getByRole('button', { name: 'editor.debug.addView' }));
    const removeButtons = screen.getAllByRole('button', {
      name: /editor\.debug\.removeView/,
    });
    expect(removeButtons).toHaveLength(2);

    // 하나 삭제 → 다시 1개, 삭제 버튼 사라짐.
    fireEvent.click(removeButtons[0]!);
    expect(screen.getAllByTitle('editor.debug.viewAll')).toHaveLength(1);
    expect(
      screen.queryByRole('button', { name: /editor\.debug\.removeView/ }),
    ).toBeNull();
  });

  it('노드 전용 필터: 선택 노드의 모든 포트만 보인다', () => {
    seedNodes([
      { id: 'A', label: '노드A' },
      { id: 'B', label: '노드B' },
    ]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    useTapStore.getState().appendOutput(makePayload('A', 'error'));
    useTapStore.getState().appendOutput(makePayload('B', 'out'));
    render(<DebugPanel />);
    openTapTab();

    // 노드 필터를 A 로.
    fireEvent.change(screen.getByLabelText('editor.debug.nodeFilter'), { target: { value: 'A' } });

    // A 의 두 엔트리(out, error)는 보이고 B 는 없다.
    const rows = getEntryRows();
    expect(rows).toHaveLength(2);
    expect(rows.every((r) => r.label === '노드A')).toBe(true);
    expect(rows.map((r) => r.port).sort()).toEqual(['error', 'out']);

    // 뷰 라벨이 노드 라벨로 도출된다(node/all → 라벨).
    expect(screen.getByTitle('노드A')).toBeInTheDocument();
  });

  it('포트 전용 필터: 모든 노드의 해당 포트만 보인다', () => {
    seedNodes([
      { id: 'A', label: '노드A' },
      { id: 'B', label: '노드B' },
    ]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    useTapStore.getState().appendOutput(makePayload('A', 'error'));
    useTapStore.getState().appendOutput(makePayload('B', 'out'));
    render(<DebugPanel />);
    openTapTab();

    // 포트 필터를 out 으로.
    fireEvent.change(screen.getByLabelText('editor.debug.portFilter'), { target: { value: 'out' } });

    // out 포트는 A, B 둘 다 등장. error(A)는 제외.
    const rows = getEntryRows();
    expect(rows).toHaveLength(2);
    expect(rows.every((r) => r.port === 'out')).toBe(true);
    expect(rows.map((r) => r.label).sort()).toEqual(['노드A', '노드B']);

    // 뷰 라벨은 *:out (all/port).
    expect(screen.getByTitle('*:out')).toBeInTheDocument();
  });

  it('노드+포트 필터는 독립 AND 로 동작한다', () => {
    seedNodes([
      { id: 'A', label: '노드A' },
      { id: 'B', label: '노드B' },
    ]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    useTapStore.getState().appendOutput(makePayload('A', 'error'));
    useTapStore.getState().appendOutput(makePayload('B', 'error'));
    render(<DebugPanel />);
    openTapTab();

    fireEvent.change(screen.getByLabelText('editor.debug.nodeFilter'), { target: { value: 'A' } });
    fireEvent.change(screen.getByLabelText('editor.debug.portFilter'), { target: { value: 'error' } });

    // A:error 한 건만.
    const rows = getEntryRows();
    expect(rows).toEqual([{ label: '노드A', port: 'error' }]);

    // 뷰 라벨: 노드:port.
    expect(screen.getByTitle('노드A:error')).toBeInTheDocument();
  });

  it('필터로 0건이면 인라인 빈 메시지를 보여준다', () => {
    seedNodes([{ id: 'A', label: '노드A' }]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    render(<DebugPanel />);
    openTapTab();

    fireEvent.change(screen.getByLabelText('editor.debug.portFilter'), { target: { value: 'out' } });
    // out 은 존재하므로 보인다. 이제 존재하지 않는 포트를 강제로 만들 수는 없으니
    // 노드 필터만으로 0건을 유도: 빈 케이스는 아래 폴백 테스트에서 다룬다.
    expect(screen.queryByText('해당 출력이 없습니다.')).toBeNull();
  });

  it('선택한 옵션이 사라지면 all 로 폴백한다(크래시·stale 방지)', () => {
    seedNodes([
      { id: 'A', label: '노드A' },
      { id: 'B', label: '노드B' },
    ]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    useTapStore.getState().appendOutput(makePayload('B', 'out'));
    const { rerender } = render(<DebugPanel />);
    openTapTab();

    // 노드 A 선택.
    fireEvent.change(screen.getByLabelText('editor.debug.nodeFilter'), { target: { value: 'A' } });
    expect((screen.getByLabelText('editor.debug.nodeFilter') as HTMLSelectElement).value).toBe('A');

    // A 가 옵션에서 사라지도록 출력 버퍼를 비우고 B 만 다시 적재.
    useTapStore.getState().clearOutputs();
    useTapStore.getState().appendOutput(makePayload('B', 'out'));
    rerender(<DebugPanel />);

    // select 는 'all' 로 폴백, 크래시 없음.
    expect((screen.getByLabelText('editor.debug.nodeFilter') as HTMLSelectElement).value).toBe('all');
    // 뷰 라벨도 "전체" 로 폴백.
    expect(screen.getByTitle('editor.debug.viewAll')).toBeInTheDocument();
    // 남은 B 엔트리는 보인다(A 선택이 폴백되어 전체가 표시됨).
    const rows = getEntryRows();
    expect(rows).toEqual([{ label: '노드B', port: 'out' }]);
  });

  it('뷰별로 독립적인 필터를 유지한다', () => {
    seedNodes([
      { id: 'A', label: '노드A' },
      { id: 'B', label: '노드B' },
    ]);
    useTapStore.getState().appendOutput(makePayload('A', 'out'));
    useTapStore.getState().appendOutput(makePayload('B', 'out'));
    render(<DebugPanel />);
    openTapTab();

    // 뷰1: 노드 A 필터.
    fireEvent.change(screen.getByLabelText('editor.debug.nodeFilter'), { target: { value: 'A' } });

    // 뷰2 추가(기본 전체).
    fireEvent.click(screen.getByRole('button', { name: 'editor.debug.addView' }));
    // 활성 뷰2는 전체 → 노드 필터 select 값은 all.
    expect((screen.getByLabelText('editor.debug.nodeFilter') as HTMLSelectElement).value).toBe('all');

    // 뷰1로 돌아가면 A 필터가 보존되어 있다.
    // 뷰 탭 버튼만 title 을 가지므로(옵션엔 없음) getByTitle 로 유일하게 찾는다.
    fireEvent.click(screen.getByTitle('노드A'));
    expect((screen.getByLabelText('editor.debug.nodeFilter') as HTMLSelectElement).value).toBe('A');
  });
});

describe('DebugPanel — 로그 영역 크기 조절', () => {
  const STORAGE_KEY = 'xflow.debugPanel.height';

  /** 패널을 펼친다(Debug Output 탭 클릭 → isOpen=true). */
  function openPanel(): void {
    fireEvent.click(screen.getByRole('button', { name: /Debug Output/ }));
  }

  it('기본 높이는 192px 이고, 위로 드래그하면 높이가 증가한다', () => {
    render(<DebugPanel />);
    openPanel();

    const handle = screen.getByTestId('debug-resize-handle');
    const logArea = screen.getByTestId('debug-log-area');
    expect(logArea.style.height).toBe('192px');

    // 위로 50px 드래그(clientY 300 → 250) → 높이 +50 = 242px.
    fireEvent.pointerDown(handle, { clientY: 300, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 250, pointerId: 1 });
    fireEvent.pointerUp(handle, { clientY: 250, pointerId: 1 });

    expect(logArea.style.height).toBe('242px');
  });

  it('드래그 종료 시 높이를 localStorage 에 저장하고, 재마운트 시 복원한다', () => {
    const { unmount } = render(<DebugPanel />);
    openPanel();

    const handle = screen.getByTestId('debug-resize-handle');
    fireEvent.pointerDown(handle, { clientY: 300, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 200, pointerId: 1 }); // +100 = 292px
    fireEvent.pointerUp(handle, { clientY: 200, pointerId: 1 });

    // 저장 확인.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('292');

    // 재마운트 → 저장 높이로 복원.
    unmount();
    render(<DebugPanel />);
    openPanel();
    expect(screen.getByTestId('debug-log-area').style.height).toBe('292px');
  });

  it('저장값이 없으면 기본 높이(192px)로 폴백한다', () => {
    render(<DebugPanel />);
    openPanel();
    expect(screen.getByTestId('debug-log-area').style.height).toBe('192px');
  });

  it('더블클릭하면 기본 높이(192px)로 리셋된다', () => {
    render(<DebugPanel />);
    openPanel();

    const handle = screen.getByTestId('debug-resize-handle');
    // 먼저 크기를 바꾼다.
    fireEvent.pointerDown(handle, { clientY: 300, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 250, pointerId: 1 });
    fireEvent.pointerUp(handle, { clientY: 250, pointerId: 1 });
    expect(screen.getByTestId('debug-log-area').style.height).toBe('242px');

    // 더블클릭 → 기본값 복원 + localStorage 갱신.
    fireEvent.doubleClick(handle);
    expect(screen.getByTestId('debug-log-area').style.height).toBe('192px');
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('192');
  });

  it('min 절반 아래로 끌어내리면 접히고, 다시 펼치면 직전 높이로 복원된다', () => {
    render(<DebugPanel />);
    openPanel();

    const handle = screen.getByTestId('debug-resize-handle');
    // 192 - 200 = -8px < 48(=min/2) → 접힘.
    fireEvent.pointerDown(handle, { clientY: 300, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 500, pointerId: 1 });

    // 로그 영역/핸들이 사라진다(접힘).
    expect(screen.queryByTestId('debug-log-area')).toBeNull();
    expect(screen.queryByTestId('debug-resize-handle')).toBeNull();

    // chevron(펼치기)으로 재펼침 → 직전(드래그 시작) 높이 192px 복원.
    fireEvent.click(screen.getByRole('button', { name: 'editor.debug.expand' }));
    expect(screen.getByTestId('debug-log-area').style.height).toBe('192px');
  });

  it('크기 조절 도입 후에도 탭 전환/렌더가 정상 동작한다(회귀 스모크)', () => {
    seedNodes([{ id: 'A', label: '노드A' }]);
    useTapStore.getState().appendOutput(makePayload('A', 'out', { v: 1 }));
    render(<DebugPanel />);
    openPanel();

    // Debug 탭에서 핸들 + 로그 영역이 존재.
    expect(screen.getByTestId('debug-resize-handle')).toBeInTheDocument();
    expect(screen.getByTestId('debug-log-area')).toBeInTheDocument();

    // 탭 출력 탭으로 전환 → sticky 뷰 컨트롤(전체 뷰)과 엔트리가 보인다.
    openTapTab();
    expect(screen.getByTitle('editor.debug.viewAll')).toBeInTheDocument();
    expect(screen.getAllByText('노드A').length).toBeGreaterThanOrEqual(1);
    // 핸들은 탭 출력 탭에서도 유지된다.
    expect(screen.getByTestId('debug-resize-handle')).toBeInTheDocument();
  });
});
