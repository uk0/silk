//go:build linux && wayland

#include "file_dialog_portal_wayland.h"
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>

/* The exporter subset of xdg-foreign-unstable-v1/v2. Both versions use
 * destroy=0, export=1 (new_id, wl_surface), and exported.handle=0 (string).
 * Reference: wayland-protocols/unstable/xdg-foreign. The application never
 * imports surfaces; GTK/the chooser backend does that separately.
 */
static const struct wl_message destroy_msg = {"destroy", "", NULL};
static const struct wl_message handle_msg = {"handle", "s", NULL};
static const struct wl_interface exported_v1 = {
    "zxdg_exported_v1", 1, 1, &destroy_msg, 1, &handle_msg,
};
static const struct wl_interface exported_v2 = {
    "zxdg_exported_v2", 1, 1, &destroy_msg, 1, &handle_msg,
};
static const struct wl_interface *export_v1_types[] = {&exported_v1, &wl_surface_interface};
static const struct wl_interface *export_v2_types[] = {&exported_v2, &wl_surface_interface};
static const struct wl_message exporter_v1_requests[] = {
    {"destroy", "", NULL}, {"export", "no", export_v1_types},
};
static const struct wl_message exporter_v2_requests[] = {
    {"destroy", "", NULL}, {"export_toplevel", "no", export_v2_types},
};
static const struct wl_interface exporter_v1 = {
    "zxdg_exporter_v1", 1, 2, exporter_v1_requests, 0, NULL,
};
static const struct wl_interface exporter_v2 = {
    "zxdg_exporter_v2", 1, 2, exporter_v2_requests, 0, NULL,
};

struct silk_foreign_registry {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_callback *sync;
    uint32_t v1, v2;
    int ready;
};

static void registry_global(void *data, struct wl_registry *registry,
                            uint32_t name, const char *interface, uint32_t version) {
    struct silk_foreign_registry *reg = data;
    (void)registry;
    if (version < 1) return;
    if (strcmp(interface, exporter_v2.name) == 0) reg->v2 = name;
    if (strcmp(interface, exporter_v1.name) == 0) reg->v1 = name;
}

static void registry_remove(void *data, struct wl_registry *registry, uint32_t name) {
    struct silk_foreign_registry *reg = data;
    (void)registry;
    if (name == reg->v2) reg->v2 = 0;
    if (name == reg->v1) reg->v1 = 0;
}

static const struct wl_registry_listener registry_listener = {
    registry_global, registry_remove,
};

static void synced(void *data, struct wl_callback *callback, uint32_t serial) {
    struct silk_foreign_registry *reg = data;
    (void)serial;
    wl_callback_destroy(callback);
    reg->sync = NULL;
    reg->ready = 1;
}
static const struct wl_callback_listener sync_listener = {synced};

struct silk_foreign_registry *silk_foreign_discover(struct wl_display *display) {
    struct silk_foreign_registry *reg = calloc(1, sizeof(*reg));
    if (!reg) return NULL;
    reg->display = display;
    reg->registry = wl_display_get_registry(display);
    if (!reg->registry || wl_registry_add_listener(reg->registry, &registry_listener, reg) != 0)
        goto failure;
    reg->sync = wl_display_sync(display);
    if (!reg->sync || wl_callback_add_listener(reg->sync, &sync_listener, reg) != 0)
        goto failure;
    /* GLFW pumps the default queue and flushes outgoing requests. No blocking
     * roundtrip or secondary reader may own the display socket here. */
    return reg;
failure:
    silk_foreign_forget(reg);
    return NULL;
}

int silk_foreign_state(const struct silk_foreign_registry *reg) {
    if (wl_display_get_error(reg->display)) return -1;
    if (!reg->ready) return 0;
    return reg->v1 || reg->v2 ? 1 : -1;
}

void silk_foreign_forget(struct silk_foreign_registry *reg) {
    if (reg->sync) wl_callback_destroy(reg->sync);
    if (reg->registry) wl_registry_destroy(reg->registry);
    free(reg);
}

struct silk_foreign_export {
    struct wl_proxy *proxy;
    char *handle;
};

static void remember_handle(void *data, struct wl_proxy *proxy, const char *handle) {
    struct silk_foreign_export *export = data;
    (void)proxy;
    if (!export->handle) export->handle = strdup(handle);
}
static const struct {
    void (*handle)(void *, struct wl_proxy *, const char *);
} export_listener = {remember_handle};

static void destroy_proxy(struct wl_proxy *proxy) {
    wl_proxy_marshal_flags(proxy, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
}

struct silk_foreign_export *silk_foreign_export(
    struct silk_foreign_registry *reg, struct wl_surface *surface) {
    if (silk_foreign_state(reg) != 1) return NULL;
    int v2 = reg->v2 != 0;
    struct silk_foreign_export *export = calloc(1, sizeof(*export));
    if (!export) return NULL;
    struct wl_proxy *factory = wl_registry_bind(
        reg->registry, v2 ? reg->v2 : reg->v1, v2 ? &exporter_v2 : &exporter_v1, 1);
    if (factory) {
        export->proxy = wl_proxy_marshal_flags(factory, 1,
            v2 ? &exported_v2 : &exported_v1, 1, 0, NULL, surface);
        /* Existing exports stay valid after their factory is destroyed. */
        destroy_proxy(factory);
    }
    if (!export->proxy || wl_proxy_add_listener(export->proxy,
            (void (**)(void))&export_listener, export) != 0) {
        silk_foreign_revoke(export);
        return NULL;
    }
    return export;
}

const char *silk_foreign_handle(const struct silk_foreign_export *export) {
    return export->handle;
}

void silk_foreign_revoke(struct silk_foreign_export *export) {
    if (export->proxy) destroy_proxy(export->proxy);
    free(export->handle);
    free(export);
}
