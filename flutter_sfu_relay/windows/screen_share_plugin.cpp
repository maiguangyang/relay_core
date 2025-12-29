/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-29 13:56:48
 *
 * Screen Share Plugin for Windows
 * Implements native floating toolbar and corner borders (matching macOS)
 */
#include "screen_share_plugin.h"

#include <flutter/method_channel.h>
#include <flutter/plugin_registrar_windows.h>
#include <flutter/standard_method_codec.h>

#include <dwmapi.h>
#include <string>
#include <windows.h>
#include <windowsx.h>

#pragma comment(lib, "dwmapi.lib")

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

  } else {
    result->NotImplemented();
  }
}

HWND ScreenSharePlugin::GetMainWindow() { return ::GetActiveWindow(); }

void ScreenSharePlugin::SetExcludeFromCapture(bool exclude) {
  HWND hwnd = GetMainWindow();
  if (hwnd) {
    DWORD affinity = exclude ? WDA_EXCLUDEFROMCAPTURE : WDA_NONE;
    SetWindowDisplayAffinity(hwnd, affinity);

    OutputDebugString(exclude ? L"[ScreenShare] Window excluded from capture\n"
                              : L"[ScreenShare] Window capture restored\n");
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

  // Create brushes
  toolbar_brush_ = CreateSolidBrush(RGB(38, 38, 38)); // Dark gray
  green_brush_ = CreateSolidBrush(RGB(38, 217, 89));  // Green

  // Create fonts
  label_font_ =
      CreateFontW(14, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                  OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                  DEFAULT_PITCH | FF_DONTCARE, L"Microsoft YaHei");
  button_font_ =
      CreateFontW(12, 0, 0, 0, FW_MEDIUM, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                  OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                  DEFAULT_PITCH | FF_DONTCARE, L"Microsoft YaHei");
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

  int toolbarWidth = 200;
  int toolbarHeight = 36;
  int x = (screenWidth - toolbarWidth) / 2;
  int y = 45; // Distance from top

  toolbar_window_ = CreateWindowExW(
      WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_LAYERED, kToolbarClassName,
      L"Screen Share Toolbar", WS_POPUP, x, y, toolbarWidth, toolbarHeight,
      nullptr, nullptr, GetModuleHandle(nullptr), nullptr);

  if (toolbar_window_) {
    // Set layered window for rounded corners effect
    SetLayeredWindowAttributes(toolbar_window_, 0, 245, LWA_ALPHA);

    // Exclude from screen capture
    SetWindowDisplayAffinity(toolbar_window_, WDA_EXCLUDEFROMCAPTURE);

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

      // Exclude from screen capture
      SetWindowDisplayAffinity(hwnd, WDA_EXCLUDEFROMCAPTURE);

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
    // Create stop button
    HWND button = CreateWindowExW(
        0, L"BUTTON", L"结束共享",
        WS_CHILD | WS_VISIBLE | BS_PUSHBUTTON | BS_FLAT, 118, 6, 72, 24, hwnd,
        (HMENU)ID_STOP_BUTTON, GetModuleHandle(nullptr), nullptr);

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

    // Draw green dot
    if (instance_ && instance_->green_brush_) {
      HBRUSH oldBrush = (HBRUSH)SelectObject(hdc, instance_->green_brush_);
      Ellipse(hdc, 12, 12, 22, 22);
      SelectObject(hdc, oldBrush);
    }

    // Draw status text
    SetBkMode(hdc, TRANSPARENT);
    SetTextColor(hdc, RGB(255, 255, 255));
    if (instance_ && instance_->label_font_) {
      SelectObject(hdc, instance_->label_font_);
    }
    RECT textRect = {28, 9, 115, 27};
    DrawTextW(hdc, L"正在共享屏幕", -1, &textRect,
              DT_LEFT | DT_VCENTER | DT_SINGLELINE);

    EndPaint(hwnd, &ps);
    return 0;
  }

  case WM_CTLCOLORBTN: {
    // Style the stop button
    HDC hdcBtn = (HDC)wParam;
    SetBkColor(hdcBtn, RGB(230, 64, 77)); // Red background
    SetTextColor(hdcBtn, RGB(255, 255, 255));
    return (LRESULT)CreateSolidBrush(RGB(230, 64, 77));
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
