#ifndef SILK_FILE_DIALOG_PORTAL_WAYLAND_H_
#define SILK_FILE_DIALOG_PORTAL_WAYLAND_H_

struct wl_display;
struct wl_surface;
struct silk_foreign_registry;
struct silk_foreign_export;

struct silk_foreign_registry *silk_foreign_discover(struct wl_display *display);
/* 0: discovery pending, 1: available, -1: unsupported or disconnected. */
int silk_foreign_state(const struct silk_foreign_registry *reg);
void silk_foreign_forget(struct silk_foreign_registry *reg);

struct silk_foreign_export *silk_foreign_export(
    struct silk_foreign_registry *reg, struct wl_surface *surface);
const char *silk_foreign_handle(const struct silk_foreign_export *export);
void silk_foreign_revoke(struct silk_foreign_export *export);

#endif
