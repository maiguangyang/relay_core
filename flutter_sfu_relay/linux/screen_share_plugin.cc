/*
 * Screen Share Plugin for Linux (GTK+)
 * Implements native floating toolbar and corner borders
 *
 * Note: Linux doesn't have a standard API for excluding windows from capture.
 * The overlay will be visible in screen recordings.
 */

#include "screen_share_plugin.h"

#include <cairo/cairo.h>
#include <flutter_linux/flutter_linux.h>
#include <gdk/gdk.h>
#include <gtk/gtk.h>
#include <string.h>

// Channel name
#define CHANNEL_NAME "com.flutter_sfu_relay.screen_share"

// Forward declarations
static void create_toolbar_window(void);
static void create_border_windows(void);
static void destroy_overlay_windows(void);
static gboolean on_toolbar_draw(GtkWidget *widget, cairo_t *cr, gpointer data);
static gboolean on_border_draw(GtkWidget *widget, cairo_t *cr, gpointer data);
static void on_stop_button_clicked(GtkWidget *widget, gpointer data);

// Global state
static FlMethodChannel *g_channel = NULL;
static GtkWidget *g_toolbar_window = NULL;
static GtkWidget *g_border_windows[4] = {NULL, NULL, NULL, NULL};

// Colors
static const double GREEN_R = 0.15;
static const double GREEN_G = 0.85;
static const double GREEN_B = 0.35;
static const double DARK_R = 0.15;
static const double DARK_G = 0.15;
static const double DARK_B = 0.15;

// =============================================================================
// Method Call Handler
// =============================================================================

static void method_call_cb(FlMethodChannel *channel, FlMethodCall *method_call,
                           gpointer user_data) {
  const gchar *method = fl_method_call_get_name(method_call);
  g_autoptr(FlMethodResponse) response = NULL;

  if (strcmp(method, "isSupported") == 0) {
    g_autoptr(FlValue) result = fl_value_new_bool(TRUE);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));

  } else if (strcmp(method, "setExcludeFromCapture") == 0) {
    // Linux doesn't have a standard API for excluding windows from capture
    // Just return success
    g_autoptr(FlValue) result = fl_value_new_bool(TRUE);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));

  } else if (strcmp(method, "showOverlay") == 0) {
    create_toolbar_window();
    create_border_windows();
    g_autoptr(FlValue) result = fl_value_new_bool(TRUE);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));

  } else if (strcmp(method, "hideOverlay") == 0) {
    destroy_overlay_windows();
    g_autoptr(FlValue) result = fl_value_new_bool(TRUE);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));

  } else {
    response = FL_METHOD_RESPONSE(fl_method_not_implemented_response_new());
  }

  fl_method_call_respond(method_call, response, NULL);
}

// =============================================================================
// Plugin Registration
// =============================================================================

void screen_share_plugin_register_with_registrar(FlPluginRegistrar *registrar) {
  g_autoptr(FlStandardMethodCodec) codec = fl_standard_method_codec_new();

  g_channel =
      fl_method_channel_new(fl_plugin_registrar_get_messenger(registrar),
                            CHANNEL_NAME, FL_METHOD_CODEC(codec));

  fl_method_channel_set_method_call_handler(g_channel, method_call_cb, NULL,
                                            NULL);
}

// =============================================================================
// Toolbar Window
// =============================================================================

static void create_toolbar_window(void) {
  if (g_toolbar_window != NULL)
    return;

  // Get screen dimensions
  GdkDisplay *display = gdk_display_get_default();
  GdkMonitor *monitor = gdk_display_get_primary_monitor(display);
  GdkRectangle geometry;
  gdk_monitor_get_geometry(monitor, &geometry);

  int toolbar_width = 200;
  int toolbar_height = 36;
  int x = geometry.x + (geometry.width - toolbar_width) / 2;
  int y = geometry.y + 45;

  // Create window
  g_toolbar_window = gtk_window_new(GTK_WINDOW_POPUP);
  gtk_window_set_decorated(GTK_WINDOW(g_toolbar_window), FALSE);
  gtk_window_set_skip_taskbar_hint(GTK_WINDOW(g_toolbar_window), TRUE);
  gtk_window_set_skip_pager_hint(GTK_WINDOW(g_toolbar_window), TRUE);
  gtk_window_set_keep_above(GTK_WINDOW(g_toolbar_window), TRUE);
  gtk_window_set_type_hint(GTK_WINDOW(g_toolbar_window),
                           GDK_WINDOW_TYPE_HINT_UTILITY);
  gtk_widget_set_size_request(g_toolbar_window, toolbar_width, toolbar_height);
  gtk_window_move(GTK_WINDOW(g_toolbar_window), x, y);

  // Enable transparency
  GdkScreen *screen = gtk_widget_get_screen(g_toolbar_window);
  GdkVisual *visual = gdk_screen_get_rgba_visual(screen);
  if (visual != NULL) {
    gtk_widget_set_visual(g_toolbar_window, visual);
  }
  gtk_widget_set_app_paintable(g_toolbar_window, TRUE);

  // Create drawing area for custom rendering
  GtkWidget *drawing_area = gtk_drawing_area_new();
  gtk_widget_set_size_request(drawing_area, toolbar_width, toolbar_height);
  g_signal_connect(G_OBJECT(drawing_area), "draw", G_CALLBACK(on_toolbar_draw),
                   NULL);

  // Create overlay for button
  GtkWidget *overlay = gtk_overlay_new();
  gtk_container_add(GTK_CONTAINER(overlay), drawing_area);

  // Create stop button
  GtkWidget *button = gtk_button_new_with_label("结束共享");
  gtk_widget_set_size_request(button, 72, 24);
  gtk_widget_set_halign(button, GTK_ALIGN_END);
  gtk_widget_set_valign(button, GTK_ALIGN_CENTER);
  gtk_widget_set_margin_end(button, 10);

  // Style the button with CSS
  GtkCssProvider *css_provider = gtk_css_provider_new();
  gtk_css_provider_load_from_data(css_provider,
                                  "button { "
                                  "  background: #E53935; "
                                  "  color: white; "
                                  "  border: none; "
                                  "  border-radius: 4px; "
                                  "  font-size: 12px; "
                                  "  font-weight: 500; "
                                  "} "
                                  "button:hover { background: #F44336; } "
                                  "button:active { background: #B71C1C; }",
                                  -1, NULL);
  GtkStyleContext *style_context = gtk_widget_get_style_context(button);
  gtk_style_context_add_provider(style_context,
                                 GTK_STYLE_PROVIDER(css_provider),
                                 GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
  g_object_unref(css_provider);

  g_signal_connect(G_OBJECT(button), "clicked",
                   G_CALLBACK(on_stop_button_clicked), NULL);
  gtk_overlay_add_overlay(GTK_OVERLAY(overlay), button);

  gtk_container_add(GTK_CONTAINER(g_toolbar_window), overlay);
  gtk_widget_show_all(g_toolbar_window);
}

static gboolean on_toolbar_draw(GtkWidget *widget, cairo_t *cr, gpointer data) {
  int width = gtk_widget_get_allocated_width(widget);
  int height = gtk_widget_get_allocated_height(widget);

  // Clear background
  cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
  cairo_set_source_rgba(cr, 0, 0, 0, 0);
  cairo_paint(cr);

  // Draw rounded rectangle background
  cairo_set_operator(cr, CAIRO_OPERATOR_OVER);
  double radius = 8;
  double x = 1, y = 1, w = width - 2, h = height - 2;

  cairo_new_sub_path(cr);
  cairo_arc(cr, x + w - radius, y + radius, radius, -G_PI / 2, 0);
  cairo_arc(cr, x + w - radius, y + h - radius, radius, 0, G_PI / 2);
  cairo_arc(cr, x + radius, y + h - radius, radius, G_PI / 2, G_PI);
  cairo_arc(cr, x + radius, y + radius, radius, G_PI, 3 * G_PI / 2);
  cairo_close_path(cr);

  cairo_set_source_rgba(cr, DARK_R, DARK_G, DARK_B, 0.95);
  cairo_fill(cr);

  // Draw green dot
  cairo_arc(cr, 17, height / 2, 5, 0, 2 * G_PI);
  cairo_set_source_rgb(cr, GREEN_R, GREEN_G, GREEN_B);
  cairo_fill(cr);

  // Draw status text
  cairo_set_source_rgb(cr, 1, 1, 1);
  cairo_select_font_face(cr, "Sans", CAIRO_FONT_SLANT_NORMAL,
                         CAIRO_FONT_WEIGHT_NORMAL);
  cairo_set_font_size(cr, 12);
  cairo_move_to(cr, 28, height / 2 + 4);
  cairo_show_text(cr, "正在共享屏幕");

  return FALSE;
}

static void on_stop_button_clicked(GtkWidget *widget, gpointer data) {
  destroy_overlay_windows();

  // Notify Flutter
  if (g_channel != NULL) {
    fl_method_channel_invoke_method(g_channel, "onStopSharingRequested", NULL,
                                    NULL, NULL, NULL);
  }
}

// =============================================================================
// Border Windows
// =============================================================================

static void create_border_windows(void) {
  // Get screen dimensions
  GdkDisplay *display = gdk_display_get_default();
  GdkMonitor *monitor = gdk_display_get_primary_monitor(display);
  GdkRectangle geometry;
  gdk_monitor_get_geometry(monitor, &geometry);

  int corner_size = 60;

  // Corner positions: top-left, top-right, bottom-left, bottom-right
  int positions[4][2] = {
      {geometry.x, geometry.y},
      {geometry.x + geometry.width - corner_size, geometry.y},
      {geometry.x, geometry.y + geometry.height - corner_size},
      {geometry.x + geometry.width - corner_size,
       geometry.y + geometry.height - corner_size}};

  for (int i = 0; i < 4; i++) {
    if (g_border_windows[i] != NULL)
      continue;

    GtkWidget *window = gtk_window_new(GTK_WINDOW_POPUP);
    gtk_window_set_decorated(GTK_WINDOW(window), FALSE);
    gtk_window_set_skip_taskbar_hint(GTK_WINDOW(window), TRUE);
    gtk_window_set_skip_pager_hint(GTK_WINDOW(window), TRUE);
    gtk_window_set_keep_above(GTK_WINDOW(window), TRUE);
    gtk_window_set_type_hint(GTK_WINDOW(window), GDK_WINDOW_TYPE_HINT_UTILITY);
    gtk_widget_set_size_request(window, corner_size, corner_size);
    gtk_window_move(GTK_WINDOW(window), positions[i][0], positions[i][1]);

    // Enable transparency
    GdkScreen *screen = gtk_widget_get_screen(window);
    GdkVisual *visual = gdk_screen_get_rgba_visual(screen);
    if (visual != NULL) {
      gtk_widget_set_visual(window, visual);
    }
    gtk_widget_set_app_paintable(window, TRUE);

    // Make window click-through
    GdkWindow *gdk_window = gtk_widget_get_window(window);
    if (gdk_window != NULL) {
      cairo_region_t *region = cairo_region_create();
      gdk_window_input_shape_combine_region(gdk_window, region, 0, 0);
      cairo_region_destroy(region);
    }

    // Create drawing area
    GtkWidget *drawing_area = gtk_drawing_area_new();
    g_object_set_data(G_OBJECT(drawing_area), "corner_index",
                      GINT_TO_POINTER(i));
    g_signal_connect(G_OBJECT(drawing_area), "draw", G_CALLBACK(on_border_draw),
                     NULL);
    gtk_container_add(GTK_CONTAINER(window), drawing_area);

    gtk_widget_show_all(window);

    // Set input shape after window is realized
    gtk_widget_realize(window);
    GdkWindow *win = gtk_widget_get_window(window);
    if (win != NULL) {
      cairo_region_t *region = cairo_region_create();
      gdk_window_input_shape_combine_region(win, region, 0, 0);
      cairo_region_destroy(region);
    }

    g_border_windows[i] = window;
  }
}

static gboolean on_border_draw(GtkWidget *widget, cairo_t *cr, gpointer data) {
  int width = gtk_widget_get_allocated_width(widget);
  int height = gtk_widget_get_allocated_height(widget);
  int corner_index =
      GPOINTER_TO_INT(g_object_get_data(G_OBJECT(widget), "corner_index"));

  // Clear background
  cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
  cairo_set_source_rgba(cr, 0, 0, 0, 0);
  cairo_paint(cr);

  // Draw L-shaped border
  cairo_set_operator(cr, CAIRO_OPERATOR_OVER);
  cairo_set_source_rgb(cr, GREEN_R, GREEN_G, GREEN_B);
  cairo_set_line_width(cr, 4);
  cairo_set_line_cap(cr, CAIRO_LINE_CAP_SQUARE);

  double t = 2; // Half thickness

  switch (corner_index) {
  case 0: // Top-left
    cairo_move_to(cr, t, height);
    cairo_line_to(cr, t, t);
    cairo_line_to(cr, width, t);
    break;
  case 1: // Top-right
    cairo_move_to(cr, 0, t);
    cairo_line_to(cr, width - t, t);
    cairo_line_to(cr, width - t, height);
    break;
  case 2: // Bottom-left
    cairo_move_to(cr, t, 0);
    cairo_line_to(cr, t, height - t);
    cairo_line_to(cr, width, height - t);
    break;
  case 3: // Bottom-right
    cairo_move_to(cr, 0, height - t);
    cairo_line_to(cr, width - t, height - t);
    cairo_line_to(cr, width - t, 0);
    break;
  }

  cairo_stroke(cr);

  return FALSE;
}

// =============================================================================
// Cleanup
// =============================================================================

static void destroy_overlay_windows(void) {
  if (g_toolbar_window != NULL) {
    gtk_widget_destroy(g_toolbar_window);
    g_toolbar_window = NULL;
  }

  for (int i = 0; i < 4; i++) {
    if (g_border_windows[i] != NULL) {
      gtk_widget_destroy(g_border_windows[i]);
      g_border_windows[i] = NULL;
    }
  }
}
