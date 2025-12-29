/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-29 13:56:48
 *
 * Screen Share Plugin for Windows
 * Implements native floating toolbar and corner borders (matching macOS)
 */

// Force MSVC to use UTF-8 for source file encoding
#if defined(_MSC_VER)
#pragma execution_character_set("utf-8")
#endif
#include "include/flutter_sfu_relay/screen_share_plugin.h"

#include <flutter/method_channel.h>
#include <flutter/plugin_registrar_windows.h>
#include <flutter/standard_method_codec.h>

#include <dwmapi.h>
#include <string>
#include <windows.h>
#include <windowsx.h>

// WDA_EXCLUDEFROMCAPTURE is available in Windows 10 version 2004+
#ifndef WDA_EXCLUDEFROMCAPTURE
#define WDA_EXCLUDEFROMCAPTURE 0x00000011
#endif

namespace flutter_sfu_relay {

// Static members
ScreenShareOverlay *ScreenShareOverlay::instance_ = nullptr;
const wchar_t *ScreenShareOverlay::kToolbarClassName = L"ScreenShareToolbar";
const wchar_t *ScreenShareOverlay::kBorderClassName = L"ScreenShareBorder";

// Button ID
#define ID_STOP_BUTTON 1001

// =============================================================================
// ScreenSharePlugin Implementation
// =============================================================================

// static
extern "C" __declspec(dllexport) void ScreenSharePluginRegisterWithRegistrar(
    FlutterDesktopPluginRegistrarRef registrar) {
  auto *plugin_registrar = new flutter::PluginRegistrarWindows(registrar);
  flutter_sfu_relay::ScreenSharePlugin::RegisterWithRegistrar(plugin_registrar);
}

// static
void ScreenSharePlugin::RegisterWithRegistrar(
    flutter::PluginRegistrarWindows *registrar) {
  auto plugin = std::make_unique<ScreenSharePlugin>(registrar);
  registrar->AddPlugin(std::move(plugin));
}

ScreenSharePlugin::ScreenSharePlugin(flutter::PluginRegistrarWindows *registrar)
    : registrar_(registrar) {
  channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      registrar->messenger(), "com.flutter_sfu_relay.screen_share",
      &flutter::StandardMethodCodec::GetInstance());

  channel_->SetMethodCallHandler([this](const auto &call, auto result) {
    this->HandleMethodCall(call, std::move(result));
  });

  // Create overlay controller
  overlay_ = std::make_unique<ScreenShareOverlay>(channel_.get());
}

ScreenSharePlugin::~ScreenSharePlugin() { overlay_.reset(); }

void ScreenSharePlugin::HandleMethodCall(
    const flutter::MethodCall<flutter::EncodableValue> &method_call,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result) {

  if (method_call.method_name() == "isSupported") {
    result->Success(flutter::EncodableValue(true));

  } else if (method_call.method_name() == "setExcludeFromCapture") {
    const auto *args =
        std::get_if<flutter::EncodableMap>(method_call.arguments());
    if (args) {
      auto it = args->find(flutter::EncodableValue("exclude"));
      if (it != args->end()) {
        bool exclude = std::get<bool>(it->second);
        SetExcludeFromCapture(exclude);
        result->Success(flutter::EncodableValue(true));
        return;
      }
    }
    result->Error("INVALID_ARGS", "Missing 'exclude' parameter");

  } else if (method_call.method_name() == "showOverlay") {
    overlay_->Show();
    result->Success(flutter::EncodableValue(true));

  } else if (method_call.method_name() == "hideOverlay") {
    overlay_->Hide();
    result->Success(flutter::EncodableValue(true));

  } else if (method_call.method_name() == "minimizeWindow") {
    HWND hwnd = GetMainWindow();
    if (hwnd) {
      // Save the window handle for restore
      main_window_handle_ = hwnd;
      ShowWindow(hwnd, SW_MINIMIZE);
      OutputDebugString(L"[ScreenShare] Window minimized\n");
      result->Success(flutter::EncodableValue(true));
    } else {
      result->Error("NO_WINDOW", "Could not find main window");
    }

  } else if (method_call.method_name() == "restoreWindow") {
    HWND hwnd = main_window_handle_ ? main_window_handle_ : GetMainWindow();
    if (hwnd) {
      ShowWindow(hwnd, SW_RESTORE);
      SetForegroundWindow(hwnd);
      OutputDebugString(L"[ScreenShare] Window restored\n");
      result->Success(flutter::EncodableValue(true));
    } else {
      result->Error("NO_WINDOW", "Could not find main window");
    }

  } else {
    result->NotImplemented();
  }
}

HWND ScreenSharePlugin::GetMainWindow() {
  // Try to get the Flutter window via the registrar's view
  if (registrar_ && registrar_->GetView()) {
    HWND hwnd = registrar_->GetView()->GetNativeWindow();
    if (hwnd) {
      // Get the top-level ancestor (in case this is a child view)
      HWND topLevel = GetAncestor(hwnd, GA_ROOT);
      return topLevel ? topLevel : hwnd;
    }
  }
  // Fallback to GetActiveWindow (less reliable)
  return ::GetActiveWindow();
}

void ScreenSharePlugin::SetExcludeFromCapture(bool exclude) {
  HWND hwnd = GetMainWindow();
  if (!hwnd) {
    OutputDebugString(L"[ScreenShare] ERROR: Could not find main window\n");
    return;
  }

  // Get current extended style
  LONG_PTR exStyle = GetWindowLongPtr(hwnd, GWL_EXSTYLE);
  bool hasLayered = (exStyle & WS_EX_LAYERED) != 0;

  if (exclude) {
    // WORKAROUND: WS_EX_LAYERED conflicts with WDA_EXCLUDEFROMCAPTURE
    // Temporarily remove the layered style, apply affinity, then restore
    if (hasLayered) {
      OutputDebugString(L"[ScreenShare] Removing WS_EX_LAYERED temporarily\n");
      original_ex_style_ = exStyle;
      SetWindowLongPtr(hwnd, GWL_EXSTYLE, exStyle & ~WS_EX_LAYERED);
      // Force window to redraw with new style
      SetWindowPos(hwnd, nullptr, 0, 0, 0, 0,
                   SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_FRAMECHANGED);
    }
  }

  DWORD affinity = exclude ? WDA_EXCLUDEFROMCAPTURE : WDA_NONE;
  BOOL success = SetWindowDisplayAffinity(hwnd, affinity);

  if (!success) {
    DWORD error = GetLastError();
    wchar_t buf[256];
    swprintf_s(buf,
               L"[ScreenShare] SetWindowDisplayAffinity FAILED! Error: %lu\n",
               error);
    OutputDebugString(buf);

    // If failed and we removed layered, restore it
    if (exclude && hasLayered && original_ex_style_) {
      SetWindowLongPtr(hwnd, GWL_EXSTYLE, original_ex_style_);
      SetWindowPos(hwnd, nullptr, 0, 0, 0, 0,
                   SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_FRAMECHANGED);
      OutputDebugString(
          L"[ScreenShare] Restored WS_EX_LAYERED after failure\n");
    }
  } else {
    OutputDebugString(
        exclude ? L"[ScreenShare] Window excluded from capture successfully\n"
                : L"[ScreenShare] Window capture restored successfully\n");

    // When disabling exclusion, restore original style if it had WS_EX_LAYERED
    if (!exclude && original_ex_style_ &&
        (original_ex_style_ & WS_EX_LAYERED)) {
      SetWindowLongPtr(hwnd, GWL_EXSTYLE, original_ex_style_);
      SetWindowPos(hwnd, nullptr, 0, 0, 0, 0,
                   SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_FRAMECHANGED);
      OutputDebugString(L"[ScreenShare] Restored WS_EX_LAYERED\n");
      original_ex_style_ = 0;
    }
  }
}

// =============================================================================
// ScreenShareOverlay Implementation
// =============================================================================

ScreenShareOverlay::ScreenShareOverlay(
    flutter::MethodChannel<flutter::EncodableValue> *channel)
    : channel_(channel), toolbar_window_(nullptr), toolbar_brush_(nullptr),
      green_brush_(nullptr), label_font_(nullptr), button_font_(nullptr),
      classes_registered_(false) {
  instance_ = this;
  RegisterWindowClasses();

  // Create brushes - match macOS colors
  toolbar_brush_ = CreateSolidBrush(
      RGB(38, 38, 38)); // Dark gray: rgba(0.15, 0.15, 0.15, 0.95)
  green_brush_ =
      CreateSolidBrush(RGB(51, 217, 102)); // Green: rgba(0.2, 0.85, 0.4, 1.0)

  // Get DPI scale for high-DPI displays
  HDC hdc = GetDC(nullptr);
  int dpi = GetDeviceCaps(hdc, LOGPIXELSY);
  ReleaseDC(nullptr, hdc);
  int fontScale = MulDiv(dpi, 100, 96); // Scale factor in percent

  // Create fonts with proper scaling (increased sizes for better readability)
  int labelHeight = MulDiv(16, fontScale, 100);  // 16pt for label
  int buttonHeight = MulDiv(14, fontScale, 100); // 14pt for button

  label_font_ = CreateFontW(-labelHeight, 0, 0, 0, FW_NORMAL, FALSE, FALSE,
                            FALSE, DEFAULT_CHARSET, OUT_TT_PRECIS,
                            CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                            DEFAULT_PITCH | FF_DONTCARE, L"Microsoft YaHei UI");
  button_font_ = CreateFontW(
      -buttonHeight, 0, 0, 0, FW_SEMIBOLD, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
      OUT_TT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
      DEFAULT_PITCH | FF_DONTCARE, L"Microsoft YaHei UI");
}

ScreenShareOverlay::~ScreenShareOverlay() {
  Hide();

  if (toolbar_brush_)
    DeleteObject(toolbar_brush_);
  if (green_brush_)
    DeleteObject(green_brush_);
  if (label_font_)
    DeleteObject(label_font_);
  if (button_font_)
    DeleteObject(button_font_);

  UnregisterWindowClasses();
  instance_ = nullptr;
}

void ScreenShareOverlay::RegisterWindowClasses() {
  if (classes_registered_)
    return;

  WNDCLASSEXW wcToolbar = {};
  wcToolbar.cbSize = sizeof(WNDCLASSEXW);
  wcToolbar.lpfnWndProc = ToolbarWndProc;
  wcToolbar.hInstance = GetModuleHandle(nullptr);
  wcToolbar.lpszClassName = kToolbarClassName;
  wcToolbar.hbrBackground = (HBRUSH)GetStockObject(NULL_BRUSH);
  wcToolbar.hCursor = LoadCursor(nullptr, IDC_ARROW);
  RegisterClassExW(&wcToolbar);

  WNDCLASSEXW wcBorder = {};
  wcBorder.cbSize = sizeof(WNDCLASSEXW);
  wcBorder.lpfnWndProc = BorderWndProc;
  wcBorder.hInstance = GetModuleHandle(nullptr);
  wcBorder.lpszClassName = kBorderClassName;
  wcBorder.hbrBackground = (HBRUSH)GetStockObject(NULL_BRUSH);
  RegisterClassExW(&wcBorder);

  classes_registered_ = true;
}

void ScreenShareOverlay::UnregisterWindowClasses() {
  if (!classes_registered_)
    return;
  UnregisterClassW(kToolbarClassName, GetModuleHandle(nullptr));
  UnregisterClassW(kBorderClassName, GetModuleHandle(nullptr));
  classes_registered_ = false;
}

void ScreenShareOverlay::Show() {
  CreateToolbarWindow();
  CreateBorderWindows();
}

void ScreenShareOverlay::Hide() {
  if (toolbar_window_) {
    DestroyWindow(toolbar_window_);
    toolbar_window_ = nullptr;
  }

  for (HWND hwnd : border_windows_) {
    if (hwnd)
      DestroyWindow(hwnd);
  }
  border_windows_.clear();
}

void ScreenShareOverlay::CreateToolbarWindow() {
  if (toolbar_window_)
    return;

  // Get screen dimensions
  int screenWidth = GetSystemMetrics(SM_CXSCREEN);

  // Larger toolbar for better readability
  int toolbarWidth = 240;
  int toolbarHeight = 40;
  int x = (screenWidth - toolbarWidth) / 2;
  int y = 40; // Distance from top

  toolbar_window_ = CreateWindowExW(
      WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_LAYERED, kToolbarClassName,
      L"Screen Share Toolbar", WS_POPUP, x, y, toolbarWidth, toolbarHeight,
      nullptr, nullptr, GetModuleHandle(nullptr), nullptr);

  if (toolbar_window_) {
    // Set layered window for transparency
    SetLayeredWindowAttributes(toolbar_window_, 0, 245, LWA_ALPHA);

    // Create rounded corners (matching macOS 8px radius)
    HRGN rgn =
        CreateRoundRectRgn(0, 0, toolbarWidth + 1, toolbarHeight + 1, 16, 16);
    SetWindowRgn(toolbar_window_, rgn, TRUE);

    ShowWindow(toolbar_window_, SW_SHOWNOACTIVATE);
    UpdateWindow(toolbar_window_);
  }
}

void ScreenShareOverlay::CreateBorderWindows() {
  if (!border_windows_.empty())
    return;

  int screenWidth = GetSystemMetrics(SM_CXSCREEN);
  int screenHeight = GetSystemMetrics(SM_CYSCREEN);
  int cornerSize = 60;

  // Corner positions: top-left, top-right, bottom-left, bottom-right
  struct {
    int x, y;
  } corners[] = {{0, 0},
                 {screenWidth - cornerSize, 0},
                 {0, screenHeight - cornerSize},
                 {screenWidth - cornerSize, screenHeight - cornerSize}};

  for (int i = 0; i < 4; i++) {
    HWND hwnd = CreateWindowExW(
        WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_TRANSPARENT | WS_EX_LAYERED,
        kBorderClassName, nullptr, WS_POPUP, corners[i].x, corners[i].y,
        cornerSize, cornerSize, nullptr, nullptr, GetModuleHandle(nullptr),
        (LPVOID)(intptr_t)i);

    if (hwnd) {
      // Make window transparent except for the drawn content
      SetLayeredWindowAttributes(hwnd, RGB(0, 0, 0), 0, LWA_COLORKEY);

      // Note: NOT excluding from capture - WS_EX_LAYERED conflicts with
      // WDA_EXCLUDEFROMCAPTURE

      ShowWindow(hwnd, SW_SHOWNOACTIVATE);
      UpdateWindow(hwnd);
      border_windows_.push_back(hwnd);
    }
  }
}

// Toolbar Window Procedure
LRESULT CALLBACK ScreenShareOverlay::ToolbarWndProc(HWND hwnd, UINT msg,
                                                    WPARAM wParam,
                                                    LPARAM lParam) {
  switch (msg) {
  case WM_CREATE: {
    // Create stop button as OWNER DRAW for custom styling (matching macOS)
    HWND button = CreateWindowExW(
        0, L"BUTTON", L"结束共享", WS_CHILD | WS_VISIBLE | BS_OWNERDRAW, 152, 8,
        76, 24, hwnd, (HMENU)ID_STOP_BUTTON, GetModuleHandle(nullptr), nullptr);

    if (button && instance_ && instance_->button_font_) {
      SendMessage(button, WM_SETFONT, (WPARAM)instance_->button_font_, TRUE);
    }
    return 0;
  }

  case WM_PAINT: {
    PAINTSTRUCT ps;
    HDC hdc = BeginPaint(hwnd, &ps);

    RECT rect;
    GetClientRect(hwnd, &rect);

    // Fill with dark background
    if (instance_ && instance_->toolbar_brush_) {
      FillRect(hdc, &rect, instance_->toolbar_brush_);
    }

    // Draw larger green dot (12px diameter)
    if (instance_ && instance_->green_brush_) {
      HBRUSH oldBrush = (HBRUSH)SelectObject(hdc, instance_->green_brush_);
      HPEN nullPen = (HPEN)GetStockObject(NULL_PEN);
      HPEN oldPen = (HPEN)SelectObject(hdc, nullPen);
      Ellipse(hdc, 14, 14, 26, 26);
      SelectObject(hdc, oldPen);
      SelectObject(hdc, oldBrush);
    }

    // Draw status text
    SetBkMode(hdc, TRANSPARENT);
    SetTextColor(hdc, RGB(255, 255, 255));
    if (instance_ && instance_->label_font_) {
      SelectObject(hdc, instance_->label_font_);
    }
    RECT textRect = {32, 8, 148, 32};
    DrawTextW(hdc, L"正在共享屏幕", -1, &textRect,
              DT_LEFT | DT_VCENTER | DT_SINGLELINE);

    EndPaint(hwnd, &ps);
    return 0;
  }

  case WM_DRAWITEM: {
    // Owner-draw the stop button (red with rounded corners, matching macOS)
    LPDRAWITEMSTRUCT lpDIS = (LPDRAWITEMSTRUCT)lParam;
    if (lpDIS->CtlID == ID_STOP_BUTTON) {
      HDC hdc = lpDIS->hDC;
      RECT rc = lpDIS->rcItem;

      // Red background: RGB(230, 64, 77) = macOS rgba(0.9, 0.25, 0.3, 1.0)
      HBRUSH redBrush = CreateSolidBrush(RGB(230, 64, 77));

      // Create rounded rect region (4px radius like macOS)
      HRGN rgn = CreateRoundRectRgn(rc.left, rc.top, rc.right, rc.bottom, 8, 8);
      FillRgn(hdc, rgn, redBrush);
      DeleteObject(rgn);
      DeleteObject(redBrush);

      // Draw button text
      SetBkMode(hdc, TRANSPARENT);
      SetTextColor(hdc, RGB(255, 255, 255));
      if (instance_ && instance_->button_font_) {
        SelectObject(hdc, instance_->button_font_);
      }
      DrawTextW(hdc, L"结束共享", -1, &rc,
                DT_CENTER | DT_VCENTER | DT_SINGLELINE);

      return TRUE;
    }
    return DefWindowProc(hwnd, msg, wParam, lParam);
  }

  case WM_COMMAND: {
    if (LOWORD(wParam) == ID_STOP_BUTTON) {
      if (instance_) {
        instance_->Hide();
        // Notify Flutter
        if (instance_->channel_) {
          instance_->channel_->InvokeMethod("onStopSharingRequested", nullptr);
        }
      }
    }
    return 0;
  }

  case WM_NCHITTEST: {
    // Allow dragging the window
    return HTCAPTION;
  }

  case WM_DESTROY:
    return 0;

  default:
    return DefWindowProc(hwnd, msg, wParam, lParam);
  }
}

// Border Window Procedure
LRESULT CALLBACK ScreenShareOverlay::BorderWndProc(HWND hwnd, UINT msg,
                                                   WPARAM wParam,
                                                   LPARAM lParam) {
  switch (msg) {
  case WM_CREATE: {
    // Store corner index
    CREATESTRUCT *cs = (CREATESTRUCT *)lParam;
    SetWindowLongPtr(hwnd, GWLP_USERDATA, (LONG_PTR)cs->lpCreateParams);
    return 0;
  }

  case WM_PAINT: {
    PAINTSTRUCT ps;
    HDC hdc = BeginPaint(hwnd, &ps);

    RECT rect;
    GetClientRect(hwnd, &rect);
    int w = rect.right;
    int h = rect.bottom;

    // Get corner index
    int cornerIndex = (int)GetWindowLongPtr(hwnd, GWLP_USERDATA);

    // Create green pen
    HPEN greenPen = CreatePen(PS_SOLID, 4, RGB(38, 217, 89));
    HPEN oldPen = (HPEN)SelectObject(hdc, greenPen);

    // Draw L-shaped corner border
    switch (cornerIndex) {
    case 0: // Top-left
      MoveToEx(hdc, 2, h, nullptr);
      LineTo(hdc, 2, 2);
      LineTo(hdc, w, 2);
      break;
    case 1: // Top-right
      MoveToEx(hdc, 0, 2, nullptr);
      LineTo(hdc, w - 2, 2);
      LineTo(hdc, w - 2, h);
      break;
    case 2: // Bottom-left
      MoveToEx(hdc, 2, 0, nullptr);
      LineTo(hdc, 2, h - 2);
      LineTo(hdc, w, h - 2);
      break;
    case 3: // Bottom-right
      MoveToEx(hdc, 0, h - 2, nullptr);
      LineTo(hdc, w - 2, h - 2);
      LineTo(hdc, w - 2, 0);
      break;
    }

    SelectObject(hdc, oldPen);
    DeleteObject(greenPen);

    EndPaint(hwnd, &ps);
    return 0;
  }

  default:
    return DefWindowProc(hwnd, msg, wParam, lParam);
  }
}

} // namespace flutter_sfu_relay
