//go:build cgo && !silk_pure_go

#include "bindings.h"
#include <stdio.h>
#include <string.h>
#if defined(_WIN32)
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#else
#include <dlfcn.h>
#include <pthread.h>
#endif

// Keep the cgo preamble/header independent of the huge platform/GL headers.
// These exact-width types match the desktop GL ABI, including Win32 __stdcall.
_Static_assert(sizeof(uint32_t) == 4 && sizeof(int32_t) == 4, "GL integer ABI");
_Static_assert(sizeof(float) == 4 && sizeof(double) == 8, "GL floating ABI");

uintptr_t silk_gl_thread(void) {
#if defined(_WIN32)
    return (uintptr_t)GetCurrentThreadId();
#else
    return (uintptr_t)pthread_self();
#endif
}
static void *symbol(silk_gl_api *api, const char *name) {
#if defined(_WIN32)
    return (void *)GetProcAddress((HMODULE)api->library, name);
#else
    return dlsym(api->library, name);
#endif
}
static void *lookup(silk_gl_api *api, const char *name) {
    void *p = api->getProc ? api->getProc(name) : NULL;
#if defined(_WIN32)
    // Some WGL implementations use these invalid sentinels, not just NULL.
    if ((uintptr_t)p <= 3 || (uintptr_t)p == UINTPTR_MAX) p = NULL;
#endif
    return p ? p : symbol(api, name);
}
void silk_gl_close(silk_gl_api *api) {
    if (!api) return;
    if (api->library) {
#if defined(_WIN32)
        FreeLibrary((HMODULE)api->library);
#else
        dlclose(api->library);
#endif
    }
    free(api);
}
int silk_gl_current(silk_gl_api *api) {
    if (!api || !api->getCurrent || api->thread != silk_gl_thread()) return 0;
    return api->context == api->getCurrent();
}
silk_gl_api *silk_gl_open(char *error, size_t capacity) {
    silk_gl_api *api = calloc(1, sizeof(*api));
    if (!api) { snprintf(error, capacity, "cannot allocate GL entry table"); return NULL; }
#if defined(_WIN32)
    // Only load the system DLL, never a same-named file in the model/workspace.
    api->library = (void *)LoadLibraryExW(L"opengl32.dll", NULL, LOAD_LIBRARY_SEARCH_SYSTEM32);
    const char *current = "wglGetCurrentContext";
    const char *getProc = "wglGetProcAddress";
#elif defined(__APPLE__)
    api->library = dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", RTLD_LAZY | RTLD_LOCAL);
    const char *current = "CGLGetCurrentContext";
    const char *getProc = NULL;
#else
    api->library = dlopen("libGL.so.1", RTLD_LAZY | RTLD_LOCAL);
    const char *current = "glXGetCurrentContext";
    const char *getProc = "glXGetProcAddressARB";
#endif
    if (!api->library) { snprintf(error, capacity, "cannot load system OpenGL library"); goto fail; }
    api->getCurrent = (silk_current_fn)symbol(api, current);
    if (getProc) api->getProc = (silk_lookup_fn)symbol(api, getProc);
#if !defined(_WIN32) && !defined(__APPLE__)
    if (!api->getProc) api->getProc = (silk_lookup_fn)symbol(api, "glXGetProcAddress");
#endif
    api->thread = silk_gl_thread();
    api->context = api->getCurrent ? api->getCurrent() : NULL;
    if (!api->context) { snprintf(error, capacity, "no current OpenGL context"); goto fail; }
    api->GetString = (const uint8_t *(SILK_GL_CALL *)(uint32_t))lookup(api, "glGetString");
    if (!api->GetString) { snprintf(error, capacity, "missing glGetString"); goto fail; }
    const char *version = (const char *)api->GetString(0x1F02);
    int major = 0, minor = 0;
    if (!version || sscanf(version, "%d.%d", &major, &minor) != 2 || major < 2 || (major == 2 && minor < 1)) {
        snprintf(error, capacity, "desktop OpenGL 2.1 required; driver reports %s", version ? version : "no version");
        goto fail;
    }
    // Assign via a pointer-sized memcpy to avoid aliasing through void** and
    // preserve typed function declarations at every call site.
#define SILK_LOAD(field, name) do { \
        void *p = lookup(api, name); \
        if (!p) { snprintf(error, capacity, "missing %s", name); goto fail; } \
        _Static_assert(sizeof(api->field) == sizeof(p), "GL function pointer ABI"); \
        memcpy(&api->field, &p, sizeof(p)); \
    } while (0)
#define SILK_OPTIONAL(field, name) do { \
        if (!api->field) { \
            void *p = lookup(api, name); \
            _Static_assert(sizeof(api->field) == sizeof(p), "GL optional function pointer ABI"); \
            memcpy(&api->field, &p, sizeof(p)); \
        } \
    } while (0)
#include "load.inc"
#undef SILK_OPTIONAL
#undef SILK_LOAD
    return api;
fail:
    silk_gl_close(api);
    return NULL;
}
int silk_gl_instancing_entries(silk_gl_api *api) {
    return api && api->VertexAttribDivisor && api->DrawElementsInstancedWithOffset;
}
void silk_gl_shader_source(silk_gl_api *api, uint32_t shader, const char *source, int32_t length) {
    api->ShaderSource(shader, 1, &source, &length);
}
