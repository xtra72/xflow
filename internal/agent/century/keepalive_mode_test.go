package century

import (
	"testing"
	"time"
)

// TestShouldKeepaliveFire 는 v0.3.9 의 shouldReportFire helper 가
// "relative" / "absolute" / 비정상 입력에 대해 올바른 fire 결정을 내리는지 검증한다.
//
// 핵심 invariants:
//   - interval <= 0 → 항상 false (caller 가 가드해야 하지만 helper 도 방어적으로).
//   - "relative" (default 포함): now - lastTime >= interval 이면 fire.
//   - "absolute": now.Truncate(interval) > lastTime 이면 fire (wall-clock 정렬).
//   - 알 수 없는 mode 문자열 → "relative" 로 동작 (graceful fallback).
func TestShouldKeepaliveFire(t *testing.T) {
	const minute = 60 * time.Second
	baseAbs := time.Date(2026, 5, 19, 12, 35, 0, 0, time.UTC) // boundary at :35:00

	cases := []struct {
		name     string
		now      time.Time
		last     time.Time
		interval time.Duration
		mode     string
		want     bool
	}{
		// ── invalid interval ──
		{
			name:     "interval=0 returns false (relative)",
			now:      time.Unix(1_000_000, 0),
			last:     time.Unix(0, 0),
			interval: 0,
			mode:     "relative",
			want:     false,
		},
		{
			name:     "interval<0 returns false (absolute)",
			now:      time.Unix(1_000_000, 0),
			last:     time.Unix(0, 0),
			interval: -1 * time.Second,
			mode:     "absolute",
			want:     false,
		},

		// ── relative mode ──
		{
			name:     "relative: elapsed equals interval → fire",
			now:      time.Unix(60, 0),
			last:     time.Unix(0, 0),
			interval: minute,
			mode:     "relative",
			want:     true,
		},
		{
			name:     "relative: elapsed below interval → no fire",
			now:      time.Unix(59, 0),
			last:     time.Unix(0, 0),
			interval: minute,
			mode:     "relative",
			want:     false,
		},
		{
			name:     "relative: elapsed much greater → fire",
			now:      time.Unix(3600, 0),
			last:     time.Unix(0, 0),
			interval: minute,
			mode:     "relative",
			want:     true,
		},
		{
			name:     "default mode (empty string) acts as relative",
			now:      time.Unix(60, 0),
			last:     time.Unix(0, 0),
			interval: minute,
			mode:     "",
			want:     true,
		},
		{
			name:     "unknown mode falls back to relative",
			now:      time.Unix(30, 0),
			last:     time.Unix(0, 0),
			interval: minute,
			mode:     "crontab", // not supported value
			want:     false,     // relative: 30s < 60s
		},

		// ── absolute mode ──
		{
			name:     "absolute: now exactly on boundary, last just before → fire",
			now:      baseAbs,                        // 12:35:00
			last:     baseAbs.Add(-30 * time.Second), // 12:34:30
			interval: minute,
			mode:     "absolute",
			want:     true, // truncate(12:35:00)=12:35:00 > 12:34:30
		},
		{
			name:     "absolute: now within interval after last boundary → no fire",
			now:      baseAbs.Add(30 * time.Second), // 12:35:30
			last:     baseAbs.Add(5 * time.Second),  // 12:35:05 (after boundary)
			interval: minute,
			mode:     "absolute",
			want:     false, // truncate(12:35:30)=12:35:00 NOT > 12:35:05
		},
		{
			name:     "absolute: last before boundary, now after boundary → fire",
			now:      baseAbs.Add(5 * time.Second),   // 12:35:05
			last:     baseAbs.Add(-10 * time.Second), // 12:34:50
			interval: minute,
			mode:     "absolute",
			want:     true, // truncate(12:35:05)=12:35:00 > 12:34:50
		},
		{
			name:     "absolute 5min granularity: skip non-aligned firing",
			now:      time.Date(2026, 5, 19, 12, 32, 0, 0, time.UTC),
			last:     time.Date(2026, 5, 19, 12, 30, 5, 0, time.UTC),
			interval: 5 * time.Minute,
			mode:     "absolute",
			// truncate(12:32) -> 12:30, last=12:30:05 → 12:30 < 12:30:05 → false
			want: false,
		},
		{
			name:     "absolute 5min granularity: next boundary fires",
			now:      time.Date(2026, 5, 19, 12, 35, 10, 0, time.UTC),
			last:     time.Date(2026, 5, 19, 12, 30, 5, 0, time.UTC),
			interval: 5 * time.Minute,
			mode:     "absolute",
			// truncate(12:35:10) -> 12:35:00, last=12:30:05 → 12:35 > 12:30:05 → true
			want: true,
		},
		{
			name:     "absolute hourly: fires once per hour",
			now:      time.Date(2026, 5, 19, 13, 0, 1, 0, time.UTC),
			last:     time.Date(2026, 5, 19, 12, 59, 59, 0, time.UTC),
			interval: time.Hour,
			mode:     "absolute",
			want:     true, // truncate(13:00:01)=13:00 > 12:59:59
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldReportFire(tc.now, tc.last, tc.interval, tc.mode)
			if got != tc.want {
				t.Errorf("shouldReportFire(now=%v, last=%v, interval=%v, mode=%q) = %v, want %v",
					tc.now.Format(time.RFC3339), tc.last.Format(time.RFC3339),
					tc.interval, tc.mode, got, tc.want)
			}
		})
	}
}

// TestParseHvacr01Config_ReportMode 는 v0.3.9 keepalive_mode 옵션 파싱이
// 올바른 default 부여, 허용값 통과, 거부값 에러 반환을 수행하는지 검증한다.
func TestParseHvacr01Config_ReportMode(t *testing.T) {
	baseOpts := func() map[string]any {
		return map[string]any{
			"serial_port": "/dev/ttyUSB-test",
		}
	}

	t.Run("default is relative", func(t *testing.T) {
		cfg, err := parseHvacr01Config(baseOpts())
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if cfg.ReportMode != "relative" {
			t.Errorf("expected default ReportMode=%q, got %q", "relative", cfg.ReportMode)
		}
	})

	t.Run("explicit relative accepted", func(t *testing.T) {
		opts := baseOpts()
		opts["keepalive_mode"] = "relative"
		cfg, err := parseHvacr01Config(opts)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if cfg.ReportMode != "relative" {
			t.Errorf("expected ReportMode=relative, got %q", cfg.ReportMode)
		}
	})

	t.Run("explicit absolute accepted", func(t *testing.T) {
		opts := baseOpts()
		opts["keepalive_mode"] = "absolute"
		cfg, err := parseHvacr01Config(opts)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if cfg.ReportMode != "absolute" {
			t.Errorf("expected ReportMode=absolute, got %q", cfg.ReportMode)
		}
	})

	t.Run("empty string keeps default", func(t *testing.T) {
		opts := baseOpts()
		opts["keepalive_mode"] = ""
		cfg, err := parseHvacr01Config(opts)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if cfg.ReportMode != "relative" {
			t.Errorf("expected ReportMode=relative (default), got %q", cfg.ReportMode)
		}
	})

	t.Run("invalid mode rejected", func(t *testing.T) {
		opts := baseOpts()
		opts["keepalive_mode"] = "crontab"
		_, err := parseHvacr01Config(opts)
		if err == nil {
			t.Fatal("expected error for invalid keepalive_mode, got nil")
		}
	})
}
