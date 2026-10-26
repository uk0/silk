// Compile the production eligibility policy with responder doubles, not input
// injection. The actual system chooser/commit is covered by separate UI QA.
#import <Cocoa/Cocoa.h>
#include <assert.h>
#include <pthread.h>

@interface PaletteAppProbe : NSObject
@property BOOL active;
@property int requests;
@end
@implementation PaletteAppProbe
- (BOOL)isActive {return self.active;}
- (void)orderFrontCharacterPalette:(id)sender {self.requests++;}
@end
@interface PaletteViewProbe : NSObject
@property BOOL marked;
@property BOOL inputClient;
@end
@implementation PaletteViewProbe
- (BOOL)hasMarkedText {return self.marked;}
- (BOOL)conformsToProtocol:(Protocol *)protocol {return self.inputClient;}
@end
@interface PaletteWindowProbe : NSObject
@property BOOL key;
@property BOOL visible;
@property BOOL miniaturized;
@property(assign) id view;
@property(assign) id responder;
@end
@implementation PaletteWindowProbe
- (BOOL)isKeyWindow {return self.key;}
- (BOOL)isVisible {return self.visible;}
- (BOOL)isMiniaturized {return self.miniaturized;}
- (id)contentView {return self.view;}
- (id)firstResponder {return self.responder;}
@end
static PaletteAppProbe *paletteApp;
#define NSApp paletteApp
#import "../character_palette_darwin.m"
#undef NSApp

static void *requestFromWorker(void *window) {
 @autoreleasepool {
  assert(![NSThread isMainThread]);
  assert(!silkShowCharacterPalette(window));
 }
 return NULL;
}

int main(void) {
 @autoreleasepool {
  paletteApp=[PaletteAppProbe new];paletteApp.active=YES;
  PaletteViewProbe *view=[PaletteViewProbe new];view.inputClient=YES;
  PaletteWindowProbe *window=[PaletteWindowProbe new];window.key=YES;window.visible=YES;window.view=view;window.responder=view;
  assert(silkShowCharacterPalette(window));assert(paletteApp.requests==1);
  assert(!silkShowCharacterPalette(NULL));
  paletteApp.active=NO;assert(!silkShowCharacterPalette(window));paletteApp.active=YES;
  window.key=NO;assert(!silkShowCharacterPalette(window));window.key=YES;
  window.visible=NO;assert(!silkShowCharacterPalette(window));window.visible=YES;
  window.miniaturized=YES;assert(!silkShowCharacterPalette(window));window.miniaturized=NO;
  window.responder=nil;assert(!silkShowCharacterPalette(window));window.responder=view;
  window.view=nil;assert(!silkShowCharacterPalette(window));window.view=view;
  view.inputClient=NO;assert(!silkShowCharacterPalette(window));view.inputClient=YES;
  view.marked=YES;assert(!silkShowCharacterPalette(window));view.marked=NO;
  pthread_t worker;
  assert(pthread_create(&worker,NULL,requestFromWorker,window)==0);
  assert(pthread_join(worker,NULL)==0);
  assert(paletteApp.requests==1);
  [window release];[view release];[paletteApp release];
  puts("NATIVE_CHARACTER_PALETTE_POLICY_OK exclusions=10");
 }
 return 0;
}
