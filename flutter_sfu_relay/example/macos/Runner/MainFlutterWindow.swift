import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
    
  private var methodChannel: FlutterMethodChannel?
  
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)
    
    // 🔑 核心解决方案：设置窗口的 sharingType 为 none
    // 这会让 macOS 在屏幕捕获时自动排除这个窗口
    // 这是腾讯会议、Zoom、Teams 等专业应用使用的解决方案！
    self.sharingType = .none
    
    // Register custom screen capture method channel
    let channel = FlutterMethodChannel(
      name: "com.example.screencapture",
      binaryMessenger: flutterViewController.engine.binaryMessenger
    )
    self.methodChannel = channel
    
    // 设置屏幕共享覆盖层控制器
    ScreenShareOverlayController.shared.setup(mainWindow: self, channel: channel)
    
    channel.setMethodCallHandler { [weak self] call, result in
      self?.handleScreenCaptureMethod(call: call, result: result)
    }

    super.awakeFromNib()
  }
  
  private func handleScreenCaptureMethod(call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "isSupported":
      // 由于使用 sharingType = .none，所有 macOS 版本都支持自排除
      result(true)
      
    case "getSelfWindowIDs":
      // 返回当前应用的所有窗口 ID（用于调试）
      var windowIDs: [Int] = []
      for window in NSApplication.shared.windows {
        windowIDs.append(window.windowNumber)
      }
      result(windowIDs)
      
    case "showScreenShareUI":
      // 显示屏幕共享覆盖层（浮动控制栏 + 绿色边框 + 窗口最小化）
      ScreenShareOverlayController.shared.showScreenShareUI()
      result(true)
      
    case "hideScreenShareUI":
      // 隐藏屏幕共享覆盖层（恢复窗口）
      ScreenShareOverlayController.shared.hideScreenShareUI()
      result(true)
      
    default:
      result(FlutterMethodNotImplemented)
    }
  }
}
