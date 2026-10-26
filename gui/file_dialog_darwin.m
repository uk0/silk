//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

// Two references: the synchronous Go caller and the AppKit completion block.
// Releasing/cancelling while unwinding or closing the owner is safe even when
// AppKit invokes completion later. No Go pointer is stored in native memory.
typedef struct {
    NSSavePanel *panel;
    NSWindow *owner;
    char *path;
    int done;
    int cancelled;
    int refs;
} SilkFileDialog;

static void silkFileDialogUnref(SilkFileDialog *session) {
    if (--session->refs == 0) {
        [session->panel release];
        [session->owner release];
        free(session->path);
        free(session);
    }
}

void *silkBeginFileDialog(void *ownerHandle, int kind) {
    if (!ownerHandle || ![NSThread isMainThread] || kind < 0 || kind > 2) {
        return NULL;
    }
    NSWindow *owner = (NSWindow *)ownerHandle;
    if (![owner isVisible] || [owner attachedSheet]) {
        return NULL;
    }
    NSSavePanel *panel;
    if (kind == 2) {
        panel = [NSSavePanel savePanel];
        [panel setTitle:@"Save File"];
        [panel setNameFieldStringValue:@"design.silkui"];
        [panel setCanCreateDirectories:YES];
    } else {
        NSOpenPanel *open = [NSOpenPanel openPanel];
        [open setTitle:kind == 1 ? @"Open Folder" : @"Open File"];
        [open setCanChooseFiles:kind == 0];
        [open setCanChooseDirectories:kind == 1];
        [open setAllowsMultipleSelection:NO];
        [open setResolvesAliases:YES];
        panel = open;
    }
    SilkFileDialog *session = calloc(1, sizeof(*session));
    if (!session) {
        return NULL;
    }
    session->panel = [panel retain];
    session->owner = [owner retain];
    session->refs = 2;
    [NSApp activateIgnoringOtherApps:YES];
    [owner makeKeyAndOrderFront:nil];
    [panel beginSheetModalForWindow:owner completionHandler:^(NSModalResponse response) {
        if (response == NSModalResponseOK && !session->cancelled) {
            const char *path = [[[session->panel URL] path] UTF8String];
            if (path) {
                session->path = strdup(path);
            }
        }
        // AppKit may invoke completion while the panel is still onscreen.
        [session->panel orderOut:nil];
        session->done = 1;
        silkFileDialogUnref(session);
    }];
    return session;
}

int silkFileDialogDone(void *handle) {
    SilkFileDialog *session = handle;
    return !session || session->done;
}

const char *silkFileDialogPath(void *handle) {
    SilkFileDialog *session = handle;
    return session ? session->path : NULL;
}

void silkCancelFileDialog(void *handle) {
    SilkFileDialog *session = handle;
    if (session && !session->done && !session->cancelled) {
        session->cancelled = 1;
        [session->panel cancel:nil];
    }
}

// A sheet can leave AppKit with no key window after orderOut. In that state
// GLFW receives no key events until the user clicks its content view. Restore
// only this live owner, without activating the app or overriding another sheet
// or window that acquired focus while the dialog was open.
void silkRestoreFileDialogFocus(void *handle) {
    SilkFileDialog *session = handle;
    if (!session || !session->done || session->cancelled || ![NSThread isMainThread]) {
        return;
    }
    NSWindow *owner = session->owner;
    NSWindow *key = [NSApp keyWindow];
    NSWindow *sheet = [owner attachedSheet];
    if ([NSApp isActive] && [owner isVisible] && ![owner isMiniaturized] &&
        (!key || key == owner || key == session->panel) &&
        (!sheet || sheet == session->panel)) {
        [owner makeKeyWindow];
    }
}

void silkReleaseFileDialog(void *handle) {
    SilkFileDialog *session = handle;
    if (session) {
        silkCancelFileDialog(session);
        silkFileDialogUnref(session);
    }
}
