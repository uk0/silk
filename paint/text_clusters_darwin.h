#include <stddef.h>
#include <cairo.h>
#if defined(CAIRO_HAS_QUARTZ_FONT) && CAIRO_HAS_QUARTZ_FONT
enum { SILK_CLUSTER_AVAILABLE = 1 };
#else
enum { SILK_CLUSTER_AVAILABLE = 0 };
#endif
typedef struct { int begin, end; } silk_cluster_span;
typedef struct {
    cairo_font_face_t *face;
    char *name, *key;
    double size;
    cairo_glyph_t *glyphs;
    int count;
} silk_cluster_run;
typedef struct { silk_cluster_run *runs; int count; double advance; } silk_cluster_shape;
silk_cluster_span *silk_cluster_spans(const char *, size_t, int *);
void silk_cluster_free(void *);
silk_cluster_shape *silk_cluster_shape_create(const char *, double, int, int, const char *, size_t);
void silk_cluster_shape_free(silk_cluster_shape *);
