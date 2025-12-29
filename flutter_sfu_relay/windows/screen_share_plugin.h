#ifndef SCREEN_SHARE_PLUGIN_H_
#define SCREEN_SHARE_PLUGIN_H_

#include <flutter/method_channel.h>
#include <flutter/plugin_registrar_windows.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <vector>
#include <windows.h>

namespace flutter_sfu_relay {

// Forward declaration
class ScreenShareOverlay;

// Screen Share Plugin for Windows
// Provides screen capture self-exclusion and native overlay UI
// Screen Share Plugin for Windows
// Provides screen capture self-exclusion and native overlay UI
class ScreenSharePlugin : public flutter::Plugin {
public:
  static void RegisterWithRegistrar(flutter::PluginRegistrarWindows *registrar);

  ScreenSharePlugin(flutter::PluginRegistrarWindows *registrar);
  virtual ~ScreenSharePlugin();

private:
  void HandleMethodCall(
      const flutter::MethodCall<flutter::EncodableValue> &method_call,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result);

  void SetExcludeFromCapture(bool exclude);
  HWND GetMainWindow();

  flutter::PluginRegistrarWindows *registrar_;
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>> channel_;
  std::unique_ptr<ScreenShareOverlay> overlay_;
};

// Screen Share Overlay - Creates floating toolbar and corner borders
class ScreenShareOverlay {
public:
  ScreenShareOverlay(flutter::MethodChannel<flutter::EncodableValue> *channel);
  ~ScreenShareOverlay();

  void Show();
  void Hide();

private:
  static LRESULT CALLBACK ToolbarWndProc(HWND hwnd, UINT msg, WPARAM wParam,
                                         LPARAM lParam);
  static LRESULT CALLBACK BorderWndProc(HWND hwnd, UINT msg, WPARAM wParam,
                                        LPARAM lParam);

  void CreateToolbarWindow();
  void CreateBorderWindows();
  void RegisterWindowClasses();
  void UnregisterWindowClasses();

  flutter::MethodChannel<flutter::EncodableValue> *channel_;
  HWND toolbar_window_;
  std::vector<HWND> border_windows_;
  HBRUSH toolbar_brush_;
  HBRUSH green_brush_;
  HFONT label_font_;
  HFONT button_font_;
  bool classes_registered_;

  static ScreenShareOverlay *instance_;
  static const wchar_t *kToolbarClassName;
  static const wchar_t *kBorderClassName;
};

} // namespace flutter_sfu_relay

#endif // SCREEN_SHARE_PLUGIN_H_
