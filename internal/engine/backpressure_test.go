package engine

import "testing"

func TestBackpressureStrategy_Constants(t *testing.T) {
	// 백프레셔 전략 상수가 올바른지 검증한다.
	if StrategyBlock != "block" {
		t.Errorf("expected StrategyBlock to be 'block', got %q", StrategyBlock)
	}
	if StrategyDrop != "drop" {
		t.Errorf("expected StrategyDrop to be 'drop', got %q", StrategyDrop)
	}
}

func TestDropPolicy_Constants(t *testing.T) {
	// 드롭 정책 상수가 올바른지 검증한다.
	if DropNewest != "drop_newest" {
		t.Errorf("expected DropNewest to be 'drop_newest', got %q", DropNewest)
	}
	if DropOldest != "drop_oldest" {
		t.Errorf("expected DropOldest to be 'drop_oldest', got %q", DropOldest)
	}
}

func TestDefaultBackpressurePolicy(t *testing.T) {
	// 기본 백프레셔 정책이 올바른 기본값을 반환하는지 검증한다.
	policy := DefaultBackpressurePolicy()

	if policy.Strategy != StrategyBlock {
		t.Errorf("expected default strategy StrategyBlock, got %q", policy.Strategy)
	}
	if policy.BufferHighWaterMark != 0.8 {
		t.Errorf("expected default BufferHighWaterMark 0.8, got %f", policy.BufferHighWaterMark)
	}
	if policy.DropPolicy != DropNewest {
		t.Errorf("expected default DropPolicy DropNewest, got %q", policy.DropPolicy)
	}
}
