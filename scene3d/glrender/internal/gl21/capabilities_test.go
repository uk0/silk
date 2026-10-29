package gl21

import "testing"

func TestInstancingRequiresCapabilityEntriesAndAttributes(t *testing.T) {
	const arb = "GL_ARB_draw_instanced GL_ARB_instanced_arrays GL_EXT_gpu_shader4"
	for _, c := range []struct {
		name, version, ext string
		attributes         int32
		entries, want      bool
	}{
		{"legacy arb", "2.1 Metal - 91.7", arb, 16, true, true},
		{"legacy ext draw", "2.1", "GL_EXT_draw_instanced GL_ARB_instanced_arrays GL_EXT_gpu_shader4", 7, true, true},
		{"core", "3.3", "", 7, true, true},
		{"core4", "4.6 (Mesa)", "", 16, true, true},
		{"core draw arb arrays", "3.1", "GL_ARB_instanced_arrays", 7, true, true},
		{"no flat shading", "2.1", "GL_ARB_draw_instanced GL_ARB_instanced_arrays", 16, true, false},
		{"glx stub only", "2.1", "", 16, true, false},
		{"missing entries", "3.3", arb, 16, false, false},
		{"missing draw", "2.1", "GL_ARB_instanced_arrays", 16, true, false},
		{"missing divisor", "3.1", "GL_ARB_draw_instanced", 16, true, false},
		{"extension suffix", "2.1", arb + "_not_real", 16, true, false},
		{"attribute limit", "4.1", arb, 6, true, false},
		{"es not desktop", "OpenGL ES 3.3", arb, 16, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := supportsInstancing(c.version, c.ext, c.attributes, c.entries); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestFramebufferBlitRequiresCapabilityAndEntry(t *testing.T) {
	for _, c := range []struct {
		version, ext string
		entry, want  bool
	}{
		{"2.1 Metal", "GL_EXT_framebuffer_blit", true, true},
		{"2.1", "GL_ARB_framebuffer_object", true, true},
		{"3.0 Mesa", "", true, true},
		{"4.6", "", false, false},
		{"2.1", "", true, false},
		{"2.1", "GL_EXT_framebuffer_blit_not_real", true, false},
		{"1.5", "GL_EXT_framebuffer_blit", true, false},
		{"OpenGL ES 3.0", "GL_EXT_framebuffer_blit", true, false},
		{"bad", "GL_EXT_framebuffer_blit", true, false},
	} {
		if got := supportsFramebufferBlit(c.version, c.ext, c.entry); got != c.want {
			t.Fatalf("%+v: %v", c, got)
		}
	}
}
