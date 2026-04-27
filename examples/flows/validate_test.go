package flows_test

import (
	"testing"

	"github.com/xtra/xflow/pkg/flow"
)

func TestLoadExampleFlows(t *testing.T) {
	examples := []struct {
		path      string
		wantName  string
		wantNodes int
		wantWires int
	}{
		{
			path:      "simple-pipeline.yaml",
			wantName:  "simple-pipeline",
			wantNodes: 3,
			wantWires: 2,
		},
		{
			path:      "iot-sensor.json",
			wantName:  "iot-sensor-pipeline",
			wantNodes: 5,
			wantWires: 4,
		},
		{
			path:      "etl-pipeline.yaml",
			wantName:  "csv-etl-pipeline",
			wantNodes: 6,
			wantWires: 6,
		},
		{
			path:      "mqtt-metrics.yaml",
			wantName:  "mqtt-metrics",
			wantNodes: 9,
			wantWires: 12,
		},
	}

	for _, tt := range examples {
		t.Run(tt.path, func(t *testing.T) {
			f, err := flow.LoadFlowFromFile(tt.path)
			if err != nil {
				t.Fatalf("LoadFlowFromFile(%s) 실패: %v", tt.path, err)
			}

			if f.Name() != tt.wantName {
				t.Errorf("Name = %q, 기대값 %q", f.Name(), tt.wantName)
			}
			if len(f.Nodes()) != tt.wantNodes {
				t.Errorf("Nodes = %d, 기대값 %d", len(f.Nodes()), tt.wantNodes)
			}
			if len(f.Wires()) != tt.wantWires {
				t.Errorf("Wires = %d, 기대값 %d", len(f.Wires()), tt.wantWires)
			}
		})
	}
}
