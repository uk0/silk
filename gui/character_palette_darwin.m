//go:build darwin && cgo

#import <Cocoa/Cocoa.h>

int silkShowCharacterPalette(void *handle) {
    if (!handle || ![NSThread isMainThread]) return 0;
    NSWindow *window = (NSWindow *)handle;
    NSView *view = [window contentView];
    // A stale SDK request must not target a native file dialog, another app,
    // a hidden owner or a text field in some other NSWindow.
    if (![NSApp isActive] || ![window isKeyWindow] || ![window isVisible] ||
        [window isMiniaturized] || !view || [window firstResponder] != view ||
        ![view conformsToProtocol:@protocol(NSTextInputClient)]) return 0;
    if ([(id<NSTextInputClient>)view hasMarkedText]) return 0;
    [NSApp orderFrontCharacterPalette:nil];
    return 1;
}
