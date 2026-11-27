# Linux/X11 native file chooser

SilkIDE prefers the desktop FileChooser portal when a visible X11 owner and the
session service are available. A minimal desktop still uses Zenity if the
portal request cannot start. Canceling the portal does not open another picker.

![Actual SilkIDE PDF save panel under Openbox/X11, with document.pdf and the PDF filter](images/designer-gtk-pdf-picker.png)

This is an original capture of a source-built workbench in an isolated Ubuntu
24.04 ARM64 desktop. Tests used xdg-desktop-portal 1.18.4 and the GTK backend
1.15.1. For both PDF and SVG the GTK window's X11 transient owner matched
the exact Designer window, and both panels selected the intended format
and offered the correct default filename.

- Canceling export wrote no file. Saving the untouched defaults produced
  `document.pdf` and `document.svg`; independent readers confirmed a
  280 × 115 mm page and the authored positions of both button rasters.
- The original design's hash did not change. The IDE closed normally.
- A separate native SDK probe checked acceptance, cancellation and removal
  of the owner while its picker was showing. Protocol tests separately
  checked that cancellation requested from the application closes the
  owning D-Bus request.

The original tested code archive is
`49bb813dffdc22a35d353d4d3ef7dcfe071706be10422f6a5a58bf5dcebda8f6`.
Adding this record and its image made no runtime changes. Native Wayland
foreign-surface export is not covered by this verification.
