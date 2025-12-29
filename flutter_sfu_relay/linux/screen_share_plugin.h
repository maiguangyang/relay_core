#ifndef SCREEN_SHARE_PLUGIN_H_
#define SCREEN_SHARE_PLUGIN_H_

#include <flutter_linux/flutter_linux.h>
#include <gtk/gtk.h>

G_BEGIN_DECLS

// Plugin registration function
void screen_share_plugin_register_with_registrar(FlPluginRegistrar *registrar);

G_END_DECLS

#endif // SCREEN_SHARE_PLUGIN_H_
