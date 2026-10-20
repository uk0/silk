# Silk satin sprite atlas v1

`silk-satin-v1.png` is an image-model-generated transparent 4x4 PNG atlas,
created with the built-in image generation tool on 2026-10-20 for this project.
The original 1254×1254 PNG is embedded unchanged. The image model did not produce
perfectly equal cell spacing: `silk-satin-v1.regions.json` records the calibrated
object rectangles (left/top/right/bottom, exclusive upper bounds). Runtime crops
use a square around each object with transparent padding so neighboring-row
fragments cannot leak into an icon. Cells are sliced and resampled at runtime
in premultiplied RGBA using Catmull-Rom, with a bounded device-size cache.
No external image files or model/network calls are required at runtime.

Rows: new/open/save/undo; redo/play/build/debug; stop/preview/settings/print;
gantt/gauge/valve/tank. Use `paint.SatinIcon("valve")` or
`paint.LoadIcon("satin:valve")`. The IDE defaults common actions to this set;
`SILK_ICON_STYLE=vector` retains the prior vector theme. Structural glyphs
(expanders, resize handles, checks) remain vectors. Generated colors are artwork,
not exact brand/semantic RGB guarantees. Disabled icons use the exact theme mask.

Source prompt: see `silk-satin-v1.prompt.txt`.
Original PNG SHA-256:
`025a0df9922588c3b2d1d322966ed1ec3f3d5f27f2903bc57c4c9308579b72c2`.

Each icon caches at most eight device sizes. Warm rendering reuses the native
surface/pattern instead of decoding or resampling again. Atlas creation copies
caller-owned pixels; render caches, like other Silk icons, belong to the UI
thread. `paint.NewSpriteAtlas` also supports custom square-cell atlases, using
rational cell boundaries when the source dimensions are not divisible by the
grid. Native image size bounds prevent unbounded upsampling/cache growth.
