package hmiapp

import (
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

func TestRuntimeDiagnosticStatusPrioritizesFaultsInNarrowPanes(t *testing.T) {
	before := i18n.Locale()
	defer i18n.SetLocale(before)
	i18n.SetLocale("en")
	base := "long process source " + strings.Repeat("wide context ", 16)
	fault := hmi.RuntimeDiagnostics{
		Sources:        map[string]string{"feed": "unavailable", "storage": "bad quality"},
		RetentionError: "not allowed to prune",
	}
	cases := []struct {
		d        hmi.RuntimeDiagnostics
		snapshot bool
		want     string
	}{
		{fault, false, "2 source errors · storage maintenance error · " + base},
		{hmi.RuntimeDiagnostics{RetentionError: fault.RetentionError}, true, "storage maintenance error · " + base + " · 0 source errors"},
		{hmi.RuntimeDiagnostics{}, false, base},
		{hmi.RuntimeDiagnostics{}, true, base + " · 0 source errors"},
	}
	for _, c := range cases {
		if got := runtimeDiagnosticStatusText(base, c.d, c.snapshot); got != c.want {
			t.Fatalf("unexpected diagnostic caption %q; want %q", got, c.want)
		}
	}
	i18n.SetLocale("zh")
	if got := runtimeDiagnosticStatusText(base, fault, false); !strings.HasPrefix(got, "2 个数据源错误 · 存储维护错误 · ") {
		t.Fatalf("Chinese warnings clipped behind path: %q", got)
	}
}
