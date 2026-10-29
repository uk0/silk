package gl21

import (
	"fmt"
	"strings"
)

func supportsInstancing(version, extensions string, attributes int32, entries bool) bool {
	if !entries || attributes < 7 {
		return false
	}
	var major, minor int
	if _, err := fmt.Sscanf(version, "%d.%d", &major, &minor); err != nil || major < 2 {
		return false
	}
	has := func(name string) bool {
		for _, token := range strings.Fields(extensions) {
			if token == name {
				return true
			}
		}
		return false
	}
	draw := major > 3 || major == 3 && minor >= 1 || has("GL_ARB_draw_instanced") || has("GL_EXT_draw_instanced")
	arrays := major > 3 || major == 3 && minor >= 3 || has("GL_ARB_instanced_arrays")
	// Fixed per-instance colors/IDs must not acquire perspective-interpolation rounding.
	flat := major >= 3 || has("GL_EXT_gpu_shader4")
	return draw && arrays && flat
}
