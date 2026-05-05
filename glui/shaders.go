package glui

// Built-in GLSL programs for the glui renderer. Source uses GLSL 120 to
// remain compatible with the OpenGL 2.1 baseline established by the rest
// of the project (window_glfw.go, gl_renderer.go).
//
// Vertex layout for ALL programs is 48 bytes:
//   vec2 a_pos, vec2 a_uv, vec4 a_color, vec4 a_corner
// The rect shader is the only one that consumes a_corner; the other three
// simply do not declare it, and the renderer leaves that attribute disabled
// for them. The shared stride lets every batch live in the same VBO.
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
attribute vec4 a_corner;
varying vec2 v_uv;
varying vec4 v_color;
varying vec4 v_corner;
void main() {
    v_uv = a_uv;
    v_color = a_color;
    v_corner = a_corner;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
`

// rectFragSrc evaluates a signed-distance function for a rounded rectangle
// per fragment, with corner geometry supplied via per-vertex a_corner data
// (interpolated unchanged because the same vec4 is written to all four
// vertices of a single rect quad).
//
//   v_corner.xy = half-size in points
//   v_corner.z  = corner radius in points
//   v_corner.w  = anti-aliasing half-width in points (typically 1)
//
// v_uv is centered at the rect's midpoint, in points, so abs(v_uv) maps
// directly into the SDF.
const rectFragSrc = `
#version 120
varying vec2 v_uv;
varying vec4 v_color;
varying vec4 v_corner;
void main() {
    vec2 d = abs(v_uv) - v_corner.xy + vec2(v_corner.z);
    float sd = length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - v_corner.z;
    float a = 1.0 - smoothstep(-v_corner.w, v_corner.w, sd);
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

// Glyph fragment: samples a single-channel coverage atlas and modulates the
// vertex color. We use the red channel rather than alpha for forward
// compatibility with both LUMINANCE (GL 2.1 raster fonts) and R8/SDF atlases.
const glyphFragSrc = `
#version 120
varying vec2 v_uv;
varying vec4 v_color;
uniform sampler2D u_tex;
void main() {
    float a = texture2D(u_tex, v_uv).r;
    gl_FragColor = vec4(v_color.rgb, v_color.a * a);
}
`
