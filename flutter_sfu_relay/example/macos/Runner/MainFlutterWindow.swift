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
    
    // 注意：不再默认设置 sharingType = .none
    // 这样可以正常截图。只在屏幕共享期间临时设置
    
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
      result(true)
      
    case "getSelfWindowIDs":
      var windowIDs: [Int] = []
      for window in NSApplication.shared.windows {
        windowIDs.append(window.windowNumber)
      }
      result(windowIDs)
      
    case "showScreenShareUI":
      // 🔑 在屏幕共享开始时，临时设置 sharingType = .none
      // 这样在共享期间窗口不会被捕获，但平时可以正常截图
      self.sharingType = .none
      
      // 显示屏幕共享覆盖层（浮动控制栏 + 绿色边框 + 窗口最小化）
      ScreenShareOverlayController.shared.showScreenShareUI()
      result(true)
      
    case "hideScreenShareUI":
      // 🔑 在屏幕共享结束时，恢复正常的 sharingType
      // 这样可以正常截图
      self.sharingType = .readOnly
      
      // 隐藏屏幕共享覆盖层（恢复窗口）
      ScreenShareOverlayController.shared.hideScreenShareUI()
      result(true)
      
    default:
      result(FlutterMethodNotImplemented)
    }
  }
}
