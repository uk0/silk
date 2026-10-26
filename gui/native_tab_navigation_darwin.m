//go:build darwin && cgo

#import <Cocoa/Cocoa.h>

// Return NO for everything AppKit, a native dialog or an IME owns. The
// modifier mask deliberately ignores CapsLock and device-dependent bits.
static BOOL silkForwardTabNavigation(NSEvent *event, NSWindow *window) {
    NSEventType type = [event type];
    if (type != NSEventTypeKeyDown && type != NSEventTypeKeyUp) return NO;
    if ([event window] != window || [event keyCode] != 48) return NO;
    NSEventModifierFlags flags = [event modifierFlags];
    if (!(flags & NSEventModifierFlagControl) ||
        (flags & (NSEventModifierFlagCommand | NSEventModifierFlagOption))) return NO;
    NSView *view = [window contentView];
    if (!view || [window firstResponder] != view) return NO;
    if ([view respondsToSelector:@selector(hasMarkedText)] &&
        [(id<NSTextInputClient>)view hasMarkedText]) return NO;
    // Exactly once: returning nil from the monitor prevents AppKit from
    // swallowing or redispatching the same chord. GLFW handles repeat/mods.
    if (type == NSEventTypeKeyDown) [view keyDown:event];
    else [view keyUp:event];
    return YES;
}

void *silkInstallTabNavigation(void *handle) {
    if (!handle || ![NSThread isMainThread]) return NULL;
    // Capture only the raw handle, not a retaining NSWindow reference. The
    // owner removes this monitor before detaching/queuing native destruction.
    id token = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown | NSEventMaskKeyUp
        handler:^NSEvent *(NSEvent *event) {
            return silkForwardTabNavigation(event, (NSWindow *)handle) ? nil : event;
        }];
    return [token retain];
}

void silkRemoveTabNavigation(void *handle) {
    if (!handle || ![NSThread isMainThread]) return;
    id token = (id)handle;
    [NSEvent removeMonitor:token];
    [token release];
}
