//go:build linux

package app

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <gdk/gdkx.h>

enum { OP_MONITOR_AT, OP_PREPARE, OP_RESIZE, OP_MOVE, OP_FOCUS };

// One request to the GTK main thread: what to do, its two integer
// arguments, and room for the answer.
typedef struct {
	int kind;
	int a, b;
	int left, top, right, bottom;
	int ok;
	int done;
	GMutex mu;
	GCond cond;
} NativeOp;

// The title run.go gives the main window, set once from Go.
static const char *mainTitle = NULL;

static void setMainTitle(const char *title) {
	mainTitle = title;
}

// Wails keeps its GtkWindow to itself, so it's found the way
// screen_windows.go finds its HWND: by looking through this process's
// toplevels. Matching on the title skips anything else GTK may have
// open, like the web inspector under `wails dev`.
static GtkWindow *mainWindow(void) {
	static GtkWindow *cached = NULL;
	if (cached != NULL) {
		return cached;
	}
	GList *toplevels = gtk_window_list_toplevels();
	for (GList *l = toplevels; l != NULL; l = l->next) {
		GtkWindow *w = GTK_WINDOW(l->data);
		if (gtk_window_get_window_type(w) == GTK_WINDOW_TOPLEVEL &&
		    gtk_bin_get_child(GTK_BIN(w)) != NULL &&
		    g_strcmp0(gtk_window_get_title(w), mainTitle) == 0) {
			cached = w;
			break;
		}
	}
	g_list_free(toplevels);
	return cached;
}

// Where the window last was, kept current from its configure events so
// the answer is still there after the window itself has gone.
static GMutex posMu;
static int posX, posY, posKnown;

static gboolean onConfigure(GtkWidget *widget, GdkEvent *event, gpointer data) {
	int x, y;
	gtk_window_get_position(GTK_WINDOW(widget), &x, &y);
	g_mutex_lock(&posMu);
	posX = x;
	posY = y;
	posKnown = 1;
	g_mutex_unlock(&posMu);
	return FALSE;
}

static int lastPosition(int *x, int *y) {
	g_mutex_lock(&posMu);
	int known = posKnown;
	*x = posX;
	*y = posY;
	g_mutex_unlock(&posMu);
	return known;
}

// setFixedSize pins the window at exactly width x height.
//
// Both halves are needed. The size request is what GTK sizes a
// non-resizable window from, and the only way to get one under 200px
// tall. The geometry hints replace the ones Wails set from MinWidth /
// MinHeight and the monitor size - left alone, those tell the window
// manager the window may be anything in that range, and it stays at
// whatever height it was first mapped with.
static void setFixedSize(GtkWindow *win, int width, int height) {
	GdkGeometry g = {0};
	g.min_width = g.max_width = width;
	g.min_height = g.max_height = height;
	gtk_window_set_geometry_hints(win, NULL, &g, GDK_HINT_MIN_SIZE | GDK_HINT_MAX_SIZE);
	gtk_widget_set_size_request(GTK_WIDGET(win), width, height);
	gtk_window_resize(win, width, height);
}

static void runOp(NativeOp *op) {
	if (op->kind == OP_MONITOR_AT) {
		GdkDisplay *display = gdk_display_get_default();
		if (display == NULL) {
			return;
		}
		// Resolves to the nearest monitor when the point is on none.
		GdkMonitor *monitor = gdk_display_get_monitor_at_point(display, op->a, op->b);
		if (monitor == NULL) {
			return;
		}
		GdkRectangle r;
		gdk_monitor_get_geometry(monitor, &r);
		op->left = r.x;
		op->top = r.y;
		op->right = r.x + r.width;
		op->bottom = r.y + r.height;
		op->ok = 1;
		return;
	}

	GtkWindow *win = mainWindow();
	if (win == NULL) {
		return;
	}

	switch (op->kind) {
	case OP_PREPARE: {
		// Only while unmapped: the window manager reads the type when it
		// first takes the window on, and ignores a change after that.
		if (!gtk_widget_get_mapped(GTK_WIDGET(win))) {
			gtk_window_set_type_hint(win, GDK_WINDOW_TYPE_HINT_UTILITY);
		}
		// Once, however many times the page reloads under `wails dev`.
		static int tracking = 0;
		if (!tracking) {
			g_signal_connect(win, "configure-event", G_CALLBACK(onConfigure), NULL);
			tracking = 1;
		}
		setFixedSize(win, op->a, op->b);
		break;
	}
	case OP_RESIZE:
		setFixedSize(win, op->a, op->b);
		break;
	case OP_MOVE:
		gtk_window_move(win, op->a, op->b);
		break;
	case OP_FOCUS: {
		// A request for focus carries the time of the user action behind
		// it, and the window manager turns down one that looks older than
		// whatever the user last did elsewhere. The hotkey that led here
		// was read on a separate X connection, so GTK has no event of its
		// own to take a time from - the server's current time stands in.
		guint32 now = GDK_CURRENT_TIME;
		GdkWindow *gdkWin = gtk_widget_get_window(GTK_WIDGET(win));
		if (gdkWin != NULL && GDK_IS_X11_WINDOW(gdkWin)) {
			now = gdk_x11_get_server_time(gdkWin);
		}
		gtk_window_present_with_time(win, now);
		break;
	}
	}
	op->ok = 1;
}

static gboolean runOpIdle(gpointer data) {
	NativeOp *op = data;
	runOp(op);
	g_mutex_lock(&op->mu);
	op->done = 1;
	g_cond_signal(&op->cond);
	g_mutex_unlock(&op->mu);
	return G_SOURCE_REMOVE;
}

// runOnMain carries out op on the thread running the GTK main loop,
// the only one GTK may be touched from, and waits for it to finish.
//
// Queued with g_idle_add rather than g_main_context_invoke: before the
// loop has started nobody owns the context yet, and invoke would take
// that as leave to run the callback right here, on a Go thread, while
// the real main thread is still building the window.
static void runOnMain(NativeOp *op) {
	if (g_main_context_is_owner(g_main_context_default())) {
		runOp(op);
		return;
	}

	g_mutex_init(&op->mu);
	g_cond_init(&op->cond);
	g_idle_add(runOpIdle, op);

	g_mutex_lock(&op->mu);
	while (!op->done) {
		g_cond_wait(&op->cond, &op->mu);
	}
	g_mutex_unlock(&op->mu);

	g_mutex_clear(&op->mu);
	g_cond_clear(&op->cond);
}
*/
import "C"

import (
	"os"
	"sync"
)

// GTK picks its backend from the environment when it initialises, which
// is inside wails.Run - an init here is early enough to steer it.
//
// Forced to X11 (so XWayland, on a Wayland session) because a native
// Wayland window is deliberately denied the things this app is made of:
// it can't read or set its own position, so dragging, edge snapping and
// the restored position all stop working, and it can't ask to stay
// above other windows. Under XWayland all of those behave as they do on
// Windows and macOS. An explicit GDK_BACKEND is left alone.
func init() {
	if os.Getenv("GDK_BACKEND") == "" {
		os.Setenv("GDK_BACKEND", "x11")
	}
	// Never freed: it has to outlive every lookup, which is the life of
	// the process.
	C.setMainTitle(C.CString(windowTitle))
}

// The window starts hidden on Linux so prepareNativeWindow can run
// before the window manager ever sees it; domReady shows it.
const nativeStartHidden = true

// requestedSize is the size last asked for through resizeWindowNative.
// Kept because a GTK resize only lands once the main loop gets round to
// it, so asking the window for its size straight after setting it
// answers with the old one - and every resize here is computed from the
// current size.
var requestedSize struct {
	sync.Mutex
	width, height int
	ok            bool
}

func rememberSize(width, height int) {
	requestedSize.Lock()
	requestedSize.width, requestedSize.height, requestedSize.ok = width, height, true
	requestedSize.Unlock()
}

// prepareNativeWindow does what has to happen before the window is
// first shown: sizes it, and marks it as a utility window.
//
// The type is what keeps the desktop's hands off it. A small, frameless,
// always-on-top strip is exactly what a utility window is, and window
// managers treat one accordingly - tiling ones leave it floating instead
// of stretching it into a tile, and compositor effects that restyle
// ordinary app windows (rounded corners, outlines) pass it over, so the
// bar keeps the square, borderless edge it's drawn with.
func prepareNativeWindow(width, height int) {
	op := C.NativeOp{kind: C.OP_PREPARE, a: C.int(width), b: C.int(height)}
	C.runOnMain(&op)
	if op.ok != 0 {
		rememberSize(width, height)
	}
}

// resizeWindowNative sets the window's size through its GTK size
// request rather than Wails' WindowSetSize.
//
// The window is non-resizable (see DisableResize in run.go), and GTK
// sizes one of those from its content's request, not from what it's
// asked to be: WindowSetSize can grow it but never take it back under
// 200px tall, which left the 50px bar four times its height.
func resizeWindowNative(width, height int) bool {
	op := C.NativeOp{kind: C.OP_RESIZE, a: C.int(width), b: C.int(height)}
	C.runOnMain(&op)
	if op.ok == 0 {
		return false
	}
	rememberSize(width, height)
	return true
}

func windowSizeNative() (width, height int, ok bool) {
	requestedSize.Lock()
	defer requestedSize.Unlock()
	return requestedSize.width, requestedSize.height, requestedSize.ok
}

// Wails destroys the window before OnShutdown runs on Linux; see
// positionAtShutdown in windowpos.go.
const windowGoneAtShutdown = true

// lastWindowPositionNative returns where the window was the last time it
// moved, and false if it never got as far as being shown.
func lastWindowPositionNative() (x, y int, ok bool) {
	var cx, cy C.int
	known := C.lastPosition(&cx, &cy)
	return int(cx), int(cy), known != 0
}

// focusWindowNative raises the window and gives it keyboard focus.
// Wails' WindowShow only makes sure the window is visible on Linux,
// which an always-on-top bar already is.
func focusWindowNative() bool {
	op := C.NativeOp{kind: C.OP_FOCUS}
	C.runOnMain(&op)
	return op.ok != 0
}

// monitorBoundsAt returns the full bounds of whichever monitor contains
// (x, y), in the same GDK coordinate space Wails' WindowGetPosition and
// WindowSetPosition use on Linux.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	op := C.NativeOp{kind: C.OP_MONITOR_AT, a: C.int(x), b: C.int(y)}
	C.runOnMain(&op)
	if op.ok == 0 {
		return 0, 0, 0, 0, false
	}
	return int(op.left), int(op.top), int(op.right), int(op.bottom), true
}

// workAreaOriginAt returns the origin of whichever monitor contains
// (x, y). As on Windows, Wails' WindowSetPosition takes its x/y as an
// offset from the window's current monitor while WindowGetPosition
// reports an absolute one - so an absolute position means subtracting
// this first.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	left, top, _, _, ok := monitorBoundsAt(x, y)
	return left, top, ok
}

// moveWindowNative puts the window's top-left corner at an absolute
// desktop coordinate, sidestepping that offset the same way
// screen_windows.go does.
func moveWindowNative(x, y int) bool {
	op := C.NativeOp{kind: C.OP_MOVE, a: C.int(x), b: C.int(y)}
	C.runOnMain(&op)
	return op.ok != 0
}
