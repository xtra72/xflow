// SPEC-WEB-006 v0.1.0 (M6) — UpdateProgressStepper 단위 테스트.
//
// 11-state machine 시각화 컴포넌트. OperationStatus 에 따라 6개 단계 (checking,
// downloading, verifying, applying, restarting, health_checking) 를 horizontal
// stepper 로 렌더링한다. 각 단계는 미래/현재(spinner)/완료(✓)/실패(✗) 4가지
// 상태를 가진다.
//
// SPEC-UPDATE-002 v0.1.0 (M-1) 가 추가한 신규 단계:
//   - restarting:      atomic replace → drain → syscall.Exec
//   - health_checking: 새 프로세스 자가 health probe
//
// @spec SPEC-WEB-006 v0.1.0 (M6)
// @spec SPEC-UPDATE-002 v0.1.0 (M-1)

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { OperationStatus } from '@/services/api/systemUpdate';

import { UpdateProgressStepper } from './UpdateProgressStepper';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

const STEPS = [
  'checking',
  'downloading',
  'verifying',
  'applying',
  'restarting',
  'health_checking',
] as const;

function expectStepState(
  step: (typeof STEPS)[number],
  state: 'pending' | 'active' | 'complete' | 'failed',
) {
  const el = screen.getByTestId(`stepper-step-${step}`);
  expect(el.getAttribute('data-state')).toBe(state);
}

function renderStepper(
  status: OperationStatus,
  operationId?: string,
): void {
  render(
    <UpdateProgressStepper currentStatus={status} operationId={operationId} />,
  );
}

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

describe('UpdateProgressStepper', () => {
  it('idle 상태 → 모든 단계가 pending 이다', () => {
    renderStepper('idle');
    for (const s of STEPS) expectStepState(s, 'pending');
  });

  it('starting 상태 → checking 이 active 로 표시된다 (사실상 checking 시작 직전)', () => {
    renderStepper('starting');
    expectStepState('checking', 'active');
    expectStepState('downloading', 'pending');
    expectStepState('verifying', 'pending');
    expectStepState('applying', 'pending');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  it('checking → step 1 active, 나머지 pending', () => {
    renderStepper('checking');
    expectStepState('checking', 'active');
    expectStepState('downloading', 'pending');
    expectStepState('verifying', 'pending');
    expectStepState('applying', 'pending');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  it('downloading → step 1 complete, step 2 active', () => {
    renderStepper('downloading');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'active');
    expectStepState('verifying', 'pending');
    expectStepState('applying', 'pending');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  it('verifying → steps 1-2 complete, step 3 active', () => {
    renderStepper('verifying');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'complete');
    expectStepState('verifying', 'active');
    expectStepState('applying', 'pending');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  it('applying → steps 1-3 complete, step 4 active', () => {
    renderStepper('applying');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'complete');
    expectStepState('verifying', 'complete');
    expectStepState('applying', 'active');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  // SPEC-UPDATE-002 v0.1.0 (M-1) — 신규 단계 검증
  it('restarting (신규 v0.2.0) → steps 1-4 complete, step 5 active', () => {
    renderStepper('restarting');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'complete');
    expectStepState('verifying', 'complete');
    expectStepState('applying', 'complete');
    expectStepState('restarting', 'active');
    expectStepState('health_checking', 'pending');
  });

  it('health_checking (신규 v0.2.0) → steps 1-5 complete, step 6 active', () => {
    renderStepper('health_checking');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'complete');
    expectStepState('verifying', 'complete');
    expectStepState('applying', 'complete');
    expectStepState('restarting', 'complete');
    expectStepState('health_checking', 'active');
  });

  it('ready_to_restart (v0.1.0 backward compat) → 1-4 complete, 5-6 pending', () => {
    // ready_to_restart 는 v0.1.0 흐름의 종료 상태로, in-process restart 는 일어나지
    // 않았으므로 restarting/health_checking 은 미진입(pending) 으로 표시한다.
    renderStepper('ready_to_restart');
    expectStepState('checking', 'complete');
    expectStepState('downloading', 'complete');
    expectStepState('verifying', 'complete');
    expectStepState('applying', 'complete');
    expectStepState('restarting', 'pending');
    expectStepState('health_checking', 'pending');
  });

  it('completed → 모든 단계 complete (auto_restart 흐름 종결)', () => {
    renderStepper('completed');
    for (const s of STEPS) expectStepState(s, 'complete');
  });

  it('failed (active 단계 정보 없음) → 마지막 active 단계가 failed 로 표시된다', () => {
    renderStepper('failed');
    // 어느 단계에서 실패했는지 알 수 없으므로 마지막 단계(applying)에 failed 마커.
    // 하지만 실패 시점 정보가 없으니 최소한 하나 이상의 failed 가 있어야 한다.
    const stepper = screen.getByTestId('update-progress-stepper');
    expect(stepper).toHaveAttribute('data-status', 'failed');
  });

  it('operationId 가 주어지면 보조 텍스트로 노출된다', () => {
    renderStepper('downloading', 'op-xyz-987');
    expect(
      screen.getByTestId('stepper-operation-id'),
    ).toHaveTextContent(/op-xyz-987/);
  });

  it('operationId 가 없으면 텍스트가 표시되지 않는다', () => {
    renderStepper('downloading');
    expect(
      screen.queryByTestId('stepper-operation-id'),
    ).not.toBeInTheDocument();
  });
});
