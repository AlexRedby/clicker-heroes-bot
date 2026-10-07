package bot

import (
	"strings"
	"testing"
	"time"
)

func TestRunProgressionNeedsFreshExports(t *testing.T) {
	for _, export := range []*saveExportOptions{nil, {}} {
		_, err := configureRun(pipelineOptions{progression: true, export: export, fishInterval: time.Second})
		if err == nil || !strings.Contains(err.Error(), "-export-dir") {
			t.Fatal("autonomous cycle accepted no export folder", err)
		}
	}
}

func TestRunIndependentMercenaries(t *testing.T) {
	for _, mercenaries := range []bool{false, true} {
		o, err := configureRun(pipelineOptions{mercenaries: mercenaries, fishInterval: time.Second})
		if err != nil || o.mercenaries != mercenaries || o.progression || o.heroes || o.skills || o.ascension || o.autoClickers || o.gilds {
			t.Fatalf("fish/quest run enabled growth or reset: %+v %v", o, err)
		}
	}
}
