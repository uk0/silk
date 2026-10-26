//go:build darwin && cgo

#import <Cocoa/Cocoa.h>

int silkInputIsMainThread(void) { return [NSThread isMainThread]; }

int silkHasMarkedText(void *handle) {
    if (!handle || ![NSThread isMainThread]) return 0;
    id responder = [(NSWindow *)handle firstResponder];
    return [responder respondsToSelector:@selector(hasMarkedText)] &&
           [(id<NSTextInputClient>)responder hasMarkedText];
}

int silkMouseButtonEventPosition(void *handle, int button, int action, double *x, double *y) {
    if (!handle || !x || !y || ![NSThread isMainThread]) {
        return 0;
    }
    NSWindow *window = (NSWindow *)handle;
    NSEvent *event = [NSApp currentEvent];
    if (!event || [event window] != window) {
        return 0;
    }
    // Never borrow a preceding move/key event, or the coordinates of another
    // native popup. GLFW button numbers and press/release are 0/1 here.
    BOOL pressed;
    switch ([event type]) {
        case NSEventTypeLeftMouseDown:
        case NSEventTypeRightMouseDown:
        case NSEventTypeOtherMouseDown:
            pressed = YES;
            break;
        case NSEventTypeLeftMouseUp:
        case NSEventTypeRightMouseUp:
        case NSEventTypeOtherMouseUp:
            pressed = NO;
            break;
        default:
            return 0;
    }
    if ([event buttonNumber] != button || action != (pressed ? 1 : 0)) {
        return 0;
    }
    NSView *content = [window contentView];
    if (!content) {
        return 0;
    }
    // Match GLFW cocoa_window.m mouseMoved: logical client coordinates,
    // top-left origin. Do not poll mouseLocationOutsideOfEventStream: AX and
    // remote input can deliver a click without moving that physical pointer.
    NSPoint point = [event locationInWindow];
    *x = point.x;
    *y = [content frame].size.height - point.y;
    return 1;
}

int silkScrollEventPosition(void *handle, double *x, double *y) {
    if (!handle || !x || !y || ![NSThread isMainThread]) return 0;
    NSWindow *window = (NSWindow *)handle;
    NSEvent *event = [NSApp currentEvent];
    if (!event || [event window] != window || [event type] != NSEventTypeScrollWheel) return 0;
    NSView *content = [window contentView];
    if (!content) return 0;
    NSPoint point = [event locationInWindow];
    *x = point.x;
    *y = [content frame].size.height - point.y;
    return 1;
}
