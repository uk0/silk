package glui

// Built-in GLSL programs for the glui renderer. Source uses GLSL 120 to
// remain compatible with the OpenGL 2.1 baseline established by the rest
// of the project (window_glfw.go, gl_renderer.go).
//
// Vertex layout for ALL programs is: vec2 a_pos, vec2 a_uv, vec4 a_color.
// Stride 32 bytes. Programs simply ignore inputs they don't use.
//
// Coordinates are passed in already-projected clip space [-1,1]. The CPU
// projects logical/point coordinates to clip space because GL 2.1 lacks
// per-program uniform-buffer-objects and we want to keep all four programs
// pipeline-compatible without driver-specific quirks.

const rectVertSrc = `
#version 120
attribute vec2 a_pos;
attribute vec2 a_uv;
attribute vec4 a_color;
varying vec2 v_uv;
varying vec4 v_color;
void main() {
    v_uv = a_uv;
    v_color = a_color;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
`

const rectFragSrc = `
#version 120
varying vec2 v_uv;
varying vec4 v_color;
uniform vec4 u_corner;  // xy = halfSize in points, z = radius, w = AA width (1 px)
void main() {
    // Signed distance to a rounded box. v_uv is centered at (0,0), in points.
    vec2 d = abs(v_uv) - u_corner.xy + vec2(u_corner.z);
    float sd = length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - u_corner.z;
    float a = 1.0 - smoothstep(-u_corner.w, u_corner.w, sd);
    gl_FragColor = vec4(v_color.rgb, v_color.a * a);
}
`

const pathVertSrc = `
#version 120
attribute vec2 a_pos;
attribute vec2 a_uv;
attribute vec4 a_color;
varying vec4 v_color;
void main() {
    v_color = a_color;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
`

const pathFragSrc = `
#version 120
varying vec4 v_color;
void main() { gl_FragColor = v_color; }
`

const imageVertSrc = `
#version 120
attribute vec2 a_pos;
attribute vec2 a_uv;
attribute vec4 a_color;
varying vec2 v_uv;
varying vec4 v_color;
void main() {
    v_uv = a_uv;
    v_color = a_color;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
`

const imageFragSrc = `
#version 120
varying vec2 v_uv;
varying vec4 v_color;
uniform sampler2D u_tex;
void main() {
    vec4 c = texture2D(u_tex, v_uv);
    gl_FragColor = c * v_color;
}
`

// SDF (signed distance field) glyph rendering. The atlas stores a single
// channel where 0 = far outside the glyph, 0.5 = on the contour, 1 = far
// inside. Smoothstep around 0.5 gives clean anti-aliased edges at any zoom.
const glyphVertSrc = `
#version 120
attribute vec2 a_pos;
attribute vec2 a_uv;
attribute vec4 a_color;
varying vec2 v_uv;
varying vec4 v_color;
void main() {
    v_uv = a_uv;
    v_color = a_color;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
`

const glyphFragSrc = `
#version 120
varying vec2 v_uv;
varying vec4 v_color;
uniform sampler2D u_atlas;
uniform float u_smoothness; // ~0.04..0.08 depending on font size
void main() {
    float d = texture2D(u_atlas, v_uv).r;
    float a = smoothstep(0.5 - u_smoothness, 0.5 + u_smoothness, d);
    gl_FragColor = vec4(v_color.rgb, v_color.a * a);
}
`
