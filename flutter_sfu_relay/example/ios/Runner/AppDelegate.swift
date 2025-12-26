import Flutter
import UIKit
import ReplayKit

@main
@objc class AppDelegate: FlutterAppDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    GeneratedPluginRegistrant.register(with: self)
    
    let controller : FlutterViewController = window?.rootViewController as! FlutterViewController
    let channel = FlutterMethodChannel(name: "com.example.flutterSfuRelay/broadcast_picker",
                                      binaryMessenger: controller.binaryMessenger)
    channel.setMethodCallHandler({
        [weak self] (call: FlutterMethodCall, result: @escaping FlutterResult) -> Void in
        if call.method == "launchBroadcastPicker" {
            self?.launchBroadcastPicker()
            result(nil)
        } else {
            result(FlutterMethodNotImplemented)
        }
    })
    
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func launchBroadcastPicker() {
      guard let rootController = window?.rootViewController else { return }
      
      // 创建 Picker，位置不重要，因为我们马上模拟点击
      // 但为了保险，放在屏幕中间
      let picker = RPSystemBroadcastPickerView(frame: CGRect(x: 0, y: 0, width: 44, height: 44))
      picker.showsMicrophoneButton = false
      picker.preferredExtension = "com.example.flutterSfuRelayExample.BroadcastExtension"
      picker.isHidden = true // 隐藏该控件，只利用它的点击行为
      
      rootController.view.addSubview(picker)
      
      // 遍历查找按钮并触发点击
      for view in picker.subviews {
          if let button = view as? UIButton {
              button.sendActions(for: .allTouchEvents)
              button.sendActions(for: .touchUpInside)
              break
          }
      }
  }
}
