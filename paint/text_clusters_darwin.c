//go:build darwin && cgo && !silk_pure_go

#include "text_clusters_darwin.h"
#include <CoreText/CoreText.h>
#if defined(CAIRO_HAS_QUARTZ_FONT) && CAIRO_HAS_QUARTZ_FONT
#include <cairo-quartz.h>
#endif
#include <stdlib.h>
#include <string.h>
#include <math.h>

void silk_cluster_free(void *p) { free(p); }

// Return only multi-code-unit composed sequences; Go validates rune counts and
// excludes surrogate pairs, controls and private-use font identities.
silk_cluster_span *silk_cluster_spans(const char *bytes, size_t size, int *count) {
    *count = 0;
    CFStringRef text = CFStringCreateWithBytes(NULL, (const UInt8 *)bytes, size, kCFStringEncodingUTF8, false);
    if (!text) return NULL;
    CFIndex length = CFStringGetLength(text);
    silk_cluster_span *out = NULL;
    size_t capacity = 0;
    for (CFIndex i = 0; i < length;) {
        CFRange range = CFStringGetRangeOfComposedCharactersAtIndex(text, i);
        if (range.length <= 0) break;
        if (range.length > 1) {
            if ((size_t)*count == capacity) {
                capacity = capacity ? capacity * 2 : 16;
                void *next = realloc(out, capacity * sizeof(*out));
                if (!next) { free(out); out = NULL; *count = 0; break; }
                out = next;
            }
            out[(*count)++] = (silk_cluster_span){(int)range.location, (int)(range.location + range.length)};
        }
        i = range.location + range.length;
    }
    CFRelease(text);
    return out;
}

static char *utf8(CFStringRef s) {
    if (!s) return NULL;
    CFIndex n = CFStringGetMaximumSizeForEncoding(CFStringGetLength(s), kCFStringEncodingUTF8) + 1;
    char *out = malloc(n);
    if (out && !CFStringGetCString(s, out, n, kCFStringEncodingUTF8)) { free(out); out = NULL; }
    return out;
}

void silk_cluster_shape_free(silk_cluster_shape *shape) {
    if (!shape) return;
    for (int i = 0; i < shape->count; i++) {
        silk_cluster_run *r = &shape->runs[i];
        if (r->face) cairo_font_face_destroy(r->face);
        free(r->name); free(r->key); free(r->glyphs);
    }
    free(shape->runs); free(shape);
}

silk_cluster_shape *silk_cluster_shape_create(const char *family, double size, int bold, int italic, const char *bytes, size_t length) {
    silk_cluster_shape *out = NULL;
    CFStringRef name = CFStringCreateWithCString(NULL, family, kCFStringEncodingUTF8);
    CFStringRef text = CFStringCreateWithBytes(NULL, (const UInt8 *)bytes, length, kCFStringEncodingUTF8, false);
    if (!name || !text || !isfinite(size) || size <= 0) goto done;
    CTFontRef font = CTFontCreateWithName(name, size, NULL);
    if (!font) goto done;
    CTFontSymbolicTraits traits = (bold ? kCTFontBoldTrait : 0) | (italic ? kCTFontItalicTrait : 0);
    if (traits) {
        CTFontRef styled = CTFontCreateCopyWithSymbolicTraits(font, size, NULL, traits, traits);
        if (styled) { CFRelease(font); font = styled; }
    }
    CFDictionaryRef attributes = CFDictionaryCreate(NULL, (const void *[]){kCTFontAttributeName}, (const void *[]){font}, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFAttributedStringRef attributed = attributes ? CFAttributedStringCreate(NULL, text, attributes) : NULL;
    CTLineRef line = attributed ? CTLineCreateWithAttributedString(attributed) : NULL;
    if (line) {
        CFArrayRef runs = CTLineGetGlyphRuns(line);
        int count = (int)CFArrayGetCount(runs);
        out = calloc(1, sizeof(*out));
        if (out) out->runs = calloc(count, sizeof(*out->runs));
        if (out && out->runs) {
            out->count = count;
            out->advance = CTLineGetTypographicBounds(line, NULL, NULL, NULL);
            for (int i = 0; i < count; i++) {
                CTRunRef run = CFArrayGetValueAtIndex(runs, i);
                silk_cluster_run *r = &out->runs[i];
                CTFontRef actual = CFDictionaryGetValue(CTRunGetAttributes(run), kCTFontAttributeName);
                CGFontRef cg = actual ? CTFontCopyGraphicsFont(actual, NULL) : NULL;
                CFStringRef ps = cg ? CGFontCopyPostScriptName(cg) : NULL;
                r->name = utf8(ps);
                // Variations form part of face identity; glyph IDs must never
                // be reused with a differently instantiated variable font.
                CFDictionaryRef variations = cg ? CGFontCopyVariations(cg) : NULL;
                CFStringRef variationKey = variations ? CFCopyDescription(variations) : NULL;
                CFStringRef key = ps ? CFStringCreateWithFormat(NULL, NULL, CFSTR("coretext:%@:%@"), ps, variationKey ? variationKey : CFSTR("")) : NULL;
                r->key = utf8(key);
                if (cg) {
#if defined(CAIRO_HAS_QUARTZ_FONT) && CAIRO_HAS_QUARTZ_FONT
                    r->face = cairo_quartz_font_face_create_for_cgfont(cg);
#endif
                    CGFontRelease(cg);
                }
                if (ps) CFRelease(ps);
                if (key) CFRelease(key);
                if (variations) CFRelease(variations);
                if (variationKey) CFRelease(variationKey);
                r->size = actual ? CTFontGetSize(actual) : size;
                r->count = (int)CTRunGetGlyphCount(run);
                r->glyphs = calloc(r->count, sizeof(*r->glyphs));
                CGGlyph *glyphs = calloc(r->count, sizeof(*glyphs));
                CGPoint *positions = calloc(r->count, sizeof(*positions));
                bool valid = r->face && cairo_font_face_status(r->face) == CAIRO_STATUS_SUCCESS && r->name && r->key && r->glyphs && glyphs && positions && !strstr(r->name, "LastResort") && CGAffineTransformIsIdentity(CTRunGetTextMatrix(run));
                if (valid) {
                    CTRunGetGlyphs(run, CFRangeMake(0, 0), glyphs);
                    CTRunGetPositions(run, CFRangeMake(0, 0), positions);
                    for (int j = 0; j < r->count; j++) {
                        if (glyphs[j] == 0) valid = false;
                        r->glyphs[j] = (cairo_glyph_t){glyphs[j], positions[j].x, -positions[j].y};
                    }
                }
                free(glyphs); free(positions);
                if (!valid) { silk_cluster_shape_free(out); out = NULL; break; }
            }
        } else { silk_cluster_shape_free(out); out = NULL; }
        CFRelease(line);
    }
    if (attributed) CFRelease(attributed);
    if (attributes) CFRelease(attributes);
    CFRelease(font);
done:
    if (name) CFRelease(name);
    if (text) CFRelease(text);
    return out;
}
