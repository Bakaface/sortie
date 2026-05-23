package daemon

import (
	"testing"

	"github.com/Bakaface/sortie/internal/config"
)

func TestSummarizeWorkflows_PropagatesHiddenAndSource(t *testing.T) {
	in := []config.WorkflowConfig{
		{
			Name:        "active",
			Description: "Active workflow",
			Source:      "inline",
		},
		{
			Name:   "hidden-one",
			Hidden: true,
			Source: "/some/path/.sortie/workflows/tasks/hidden-one.yml",
		},
	}

	out := summarizeWorkflows(in)
	if len(out) != 2 {
		t.Fatalf("want 2 summaries, got %d", len(out))
	}
	if out[0].Hidden {
		t.Errorf("active workflow should not be hidden in summary")
	}
	if out[0].Source != "inline" {
		t.Errorf("want source=inline, got %q", out[0].Source)
	}
	if !out[1].Hidden {
		t.Errorf("hidden workflow should have Hidden=true in summary")
	}
	if out[1].Source == "inline" || out[1].Source == "" {
		t.Errorf("want file path source, got %q", out[1].Source)
	}
}
