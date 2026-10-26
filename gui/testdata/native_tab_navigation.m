// Standalone policy test: no NSApplication, real window, or input injection.
#import "../native_tab_navigation_darwin.m"
#include <assert.h>

@interface TabViewProbe : NSObject
@property BOOL marked;
@property int downs;
@property int ups;
@end
@implementation TabViewProbe
- (BOOL)hasMarkedText { return self.marked; }
- (void)keyDown:(NSEvent *)event { self.downs++; }
- (void)keyUp:(NSEvent *)event { self.ups++; }
@end
@interface TabWindowProbe : NSObject
@property(assign) id view;
@property(assign) id responder;
@end
@implementation TabWindowProbe
- (id)contentView { return self.view; }
- (id)firstResponder { return self.responder; }
@end
@interface TabEventProbe : NSObject
@property NSEventType type;
@property unsigned short keyCode;
@property NSEventModifierFlags modifierFlags;
@property(assign) id window;
@end
@implementation TabEventProbe
@end

int main(void) {
 @autoreleasepool {
  TabViewProbe *view = [TabViewProbe new];
  TabWindowProbe *window = [TabWindowProbe new];
  window.view = view; window.responder = view;
  TabEventProbe *event = [TabEventProbe new];
  event.window = window; event.keyCode = 48; event.type = NSEventTypeKeyDown;
  for (unsigned mods=0; mods<32; mods++) {
   event.modifierFlags = ((mods&1)?NSEventModifierFlagControl:0) |
    ((mods&2)?NSEventModifierFlagShift:0) | ((mods&4)?NSEventModifierFlagCommand:0) |
    ((mods&8)?NSEventModifierFlagOption:0) | ((mods&16)?NSEventModifierFlagCapsLock:0);
   int before = view.downs;
   BOOL want = (mods&1) && !(mods&12);
   assert(silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window) == want);
   assert(view.downs == before + want);
  }
  event.modifierFlags = NSEventModifierFlagControl;
  event.type = NSEventTypeKeyUp;
  assert(silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  assert(view.ups == 1);
  event.type = NSEventTypeKeyDown;
  view.marked = YES;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  view.marked = NO; window.responder = nil;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  window.responder = view; event.window = nil;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  event.window = window; event.keyCode = 49;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  event.keyCode = 48; event.type = NSEventTypeFlagsChanged;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  event.type = NSEventTypeKeyDown; window.view = nil;
  assert(!silkForwardTabNavigation((NSEvent *)event,(NSWindow *)window));
  assert(view.downs == 4 && view.ups == 1);
  [event release]; [window release]; [view release];
  puts("NATIVE_TAB_POLICY_OK modifiers=32 exclusions=6");
 }
 return 0;
}
