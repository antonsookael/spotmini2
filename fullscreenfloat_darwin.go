//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// setFloatOverFullscreenNative makes the window visible above another
// app's fullscreen Space, which plain always-on-top doesn't achieve on
// macOS: Wails' WindowSetAlwaysOnTop only raises the window's level
// (see SetAlwaysOnTop in WailsContext.m), and level alone has no effect
// across Spaces. A fullscreen app gets its own Space, so a window
// belonging to a different one simply isn't drawn there no matter how
// high its level. Joining all Spaces plus FullScreenAuxiliary is what
// actually lets it follow you into a fullscreen app.
//
// Runs on the main queue because mutating NSWindow off the main thread
// is undefined - the bound Go method that calls this is invoked from
// the frontend, so it isn't already on the main thread. Deferring also
// gives the right ordering: WindowSetAlwaysOnTop runs synchronously
// before this, so the level set below wins over the one it sets.
// findWailsWindow returns the app's real content window. Matching on
// Wails' own NSWindow subclass rather than reaching for
// -[NSApp mainWindow] matters: mainWindow is nil whenever the app
// isn't focused (the common case here, since this is a background
// utility), and the obvious fallback - the first visible window in
// -[NSApp windows] - can pick one of the several other NSWindows the
// process owns, silently configuring the wrong one.
static NSWindow *findWailsWindow(void) {
	Class wailsWindowClass = NSClassFromString(@"WailsWindow");
	if (wailsWindowClass != nil) {
		for (NSWindow *w in [NSApp windows]) {
			if ([w isKindOfClass:wailsWindowClass]) {
				return w;
			}
		}
	}

	// Only reached if Wails ever renames that class.
	NSWindow *win = [NSApp mainWindow];
	if (win != nil) {
		return win;
	}
	for (NSWindow *w in [NSApp windows]) {
		if ([w isVisible]) {
			return w;
		}
	}
	return nil;
}

static void setFloatOverFullscreenNative(int enabled) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = findWailsWindow();
		if (win == nil) {
			return;
		}

		NSWindowCollectionBehavior behaviour = [win collectionBehavior];
		if (enabled) {
			behaviour |= NSWindowCollectionBehaviorCanJoinAllSpaces;
			behaviour |= NSWindowCollectionBehaviorFullScreenAuxiliary;
			[win setCollectionBehavior:behaviour];
			// Above NSFloatingWindowLevel, which isn't reliably drawn
			// over a fullscreen app's own chrome.
			[win setLevel:NSStatusWindowLevel];
		} else {
			behaviour &= ~NSWindowCollectionBehaviorCanJoinAllSpaces;
			behaviour &= ~NSWindowCollectionBehaviorFullScreenAuxiliary;
			[win setCollectionBehavior:behaviour];
			// Level deliberately left alone - WindowSetAlwaysOnTop has
			// already set the correct normal/floating one.
		}
	});
}
*/
import "C"

// setFloatOverFullscreen controls whether the window is shown above
// other apps' fullscreen Spaces.
func setFloatOverFullscreen(enabled bool) {
	v := C.int(0)
	if enabled {
		v = C.int(1)
	}
	C.setFloatOverFullscreenNative(v)
}
