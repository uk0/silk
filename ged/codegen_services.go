package ged

import (
	"fmt"
	"strings"
)

// fieldInfo is the per-widget record the code generator collects while walking
// the scene tree. It lives at package scope (rather than inside GenerateCode) so
// the SCADA-services emission helpers below can operate on the same slice the
// constructor emission builds.
type fieldInfo struct {
	name          string
	goType        string
	constructor   string
	factoryName   string
	x, y, w, h    float64
	defaultText   string
	tagName       string      // design-time SCADA/组态 tag bound to this widget's value
	widget        interface{} // live designed widget, for reflecting design-time property values
	eventHandlers map[string]string
	code          string // user-written event handler code
	parentField   string // owning container field; "" = top-level (ui.Form)
	parentAdd     bool   // parent is a simple-AddWidget container
}

// scadaServiceFactories are the factory names that, by themselves, make a design
// a SCADA screen: the field-device component, the alarm panel and the five
// operator panels. Their runtime wiring (alarm feed, recipe/report/eventlog/
// trend/stats plumbing, device polling) lives in scada.BindScreen, so a design
// containing any of them needs a scada.Services to bind against.
var scadaServiceFactories = map[string]bool{
	"gui.DeviceComponent": true,
	"gui.AlarmPanel":      true,
	"gui.RecipePanel":     true,
	"gui.ReportView":      true,
	"gui.EventLogPanel":   true,
	"gui.TrendPanel":      true,
	"gui.StatsPanel":      true,
}

// sceneNeedsServices reports whether the generated app must own a shared
// scada.Services. It expands the old "hasTags" test: a design needs services when
// any widget carries a design-time tag (the industrial-widget TagName() seam) OR
// is one of the backend-bound SCADA widgets (device / alarm / operator panels).
// A plain UI with none of these keeps the byte-for-byte legacy output.
func sceneNeedsServices(fields []fieldInfo) bool {
	for i := range fields {
		if fields[i].tagName != "" {
			return true
		}
		if scadaServiceFactories[fields[i].factoryName] {
			return true
		}
	}
	return false
}

// emitBindServices writes the generated BindServices method: it records the
// shared services handle, exposes the tag registry through ui.Tags for
// compatibility, hands every tag-bound industrial widget its design-time tag name
// (so scada.BindScreen can resolve it against the shared registry), then binds the
// whole screen tree — industrial widgets, alarm panel and the five operator panels
// — through scada.BindScreen. The bind's stop func is dropped: a generated
// top-level app lives for the process, and services.Stop tears the container down.
func emitBindServices(buf *strings.Builder, imports map[string]bool, typeName string, fields []fieldInfo) {
	imports["github.com/uk0/silk/scada"] = true

	buf.WriteString("\n// BindServices attaches the shared SCADA services to this screen: it records the\n")
	buf.WriteString("// handle, exposes the tag registry via ui.Tags, gives each tag-bound widget its\n")
	buf.WriteString("// design-time tag, then wires the whole screen through scada.BindScreen. Call once\n")
	buf.WriteString("// after construction and before services.Start.\n")
	fmt.Fprintf(buf, "func (ui *%s) BindServices(s *scada.Services) error {\n", typeName)
	buf.WriteString("\tui.Services = s\n")
	buf.WriteString("\tui.Tags = s.Tags\n")
	for i := range fields {
		if fields[i].tagName != "" {
			fmt.Fprintf(buf, "\tui.%s.SetTagName(%q)\n", fields[i].name, fields[i].tagName)
		}
	}
	fmt.Fprintf(buf, "\t_, err := scada.BindScreen(s, ui.Form, %s)\n", buildScreenOptions(fields))
	buf.WriteString("\treturn err\n")
	buf.WriteString("}\n")
}

// buildScreenOptions renders the scada.ScreenOptions literal for BindScreen from
// what the scene knows: the distinct tag names feed the tag-consuming operator
// panels that are actually present (Trend/Stats/Recipe/Report), and a placed
// DeviceComponent opts the drivers in. Only string / []string / bool fields are
// emitted, so the generated file needs no time or report import. An empty literal
// is returned when the scene supplies nothing to configure.
func buildScreenOptions(fields []fieldInfo) string {
	var tags []string
	seen := map[string]bool{}
	var hasTrend, hasStats, hasRecipe, hasReport, hasDevice bool
	for i := range fields {
		if tn := fields[i].tagName; tn != "" && !seen[tn] {
			seen[tn] = true
			tags = append(tags, tn)
		}
		switch fields[i].factoryName {
		case "gui.TrendPanel":
			hasTrend = true
		case "gui.StatsPanel":
			hasStats = true
		case "gui.RecipePanel":
			hasRecipe = true
		case "gui.ReportView":
			hasReport = true
		case "gui.DeviceComponent":
			hasDevice = true
		}
	}

	var lines []string
	if hasTrend && len(tags) > 0 {
		lines = append(lines, fmt.Sprintf("\t\tTrendTag: %q,", tags[0]))
	}
	if hasStats && len(tags) > 0 {
		lines = append(lines, fmt.Sprintf("\t\tStatsTags: %s,", goStringSlice(tags)))
	}
	if hasRecipe && len(tags) > 0 {
		lines = append(lines, fmt.Sprintf("\t\tRecipeTags: %s,", goStringSlice(tags)))
	}
	if hasReport && len(tags) > 0 {
		lines = append(lines, fmt.Sprintf("\t\tReportTags: %s,", goStringSlice(tags)))
	}
	if hasDevice {
		lines = append(lines, "\t\tEnableDrivers: true,")
	}
	if len(lines) == 0 {
		return "scada.ScreenOptions{}"
	}
	return "scada.ScreenOptions{\n" + strings.Join(lines, "\n") + "\n\t}"
}

// goStringSlice renders a []string{...} Go literal with each element quoted.
func goStringSlice(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return "[]string{" + strings.Join(parts, ", ") + "}"
}
