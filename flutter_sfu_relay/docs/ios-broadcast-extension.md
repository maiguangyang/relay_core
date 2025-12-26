# iOS 屏幕共享配置指南 (Broadcast Extension)

要在 iOS 上实现屏幕共享（系统级录屏），必须创建一个 **Broadcast Upload Extension**。这是因为 iOS 的沙盒机制限制了应用后台访问屏幕内容的能力。

本指南详述了如何为 Flutter 应用配置 iOS 屏幕共享功能。

## 📋 前置要求

- **真机调试**：iOS 屏幕共享无法在模拟器上使用。
- **iOS 版本**：建议设置为 iOS 14.0 或更高版本 (支持 `os_log` 等API)。
- **开发者账号**：需要配置 App Groups 功能。

---

## 🛠 第一步：创建 Broadcast Upload Extension

1. 打开项目的 Xcode 工作区：
   ```bash
   open ios/Runner.xcworkspace
   ```
2. 在 Xcode 中，选择菜单栏 **File -> New -> Target...**
3. 选择 **Broadcast Upload Extension**（注意不是 UI Extension），点击 Next。
4. **Product Name** 输入 `BroadcastExtension`。
5. **Language** 选择 **Swift**。
6. 取消勾选 "Include UI Extension"。
7. 点击 **Finish**。
8. 当提示 "Activate scheme?" 时，选择 **Cancel** (我们不需要专门调试 Extension 的 scheme)。

---

## 🔗 第二步：配置 App Groups

App Groups 用于主 App 和 Extension 之间共享数据（如 Socket 连接路径）。

1. **主 App (Runner) 配置**：
   - 选择项目根节点 -> **Runner** target -> **Signing & Capabilities**。
   - 点击 **+ Capability**，搜索并添加 **App Groups**。
   - 点击 **+** 号添加一个新的 Group，格式建议为：`group.<your_bundle_id>`
     - 例如：`group.com.example.flutterSfuRelayExample`
   - 确保该 Group 被勾选。

2. **Extension (BroadcastExtension) 配置**：
   - 选择 **BroadcastExtension** target -> **Signing & Capabilities**。
   - 点击 **+ Capability**，同样添加 **App Groups**。
   - **重要**：必须勾选与主 App **完全相同**的 Group ID。

---

## 📄 第三步：添加 Swift 代码文件

你需要将以下 5 个 Swift 文件添加到 `ios/BroadcastExtension/` 目录中，并确保它们被包含在 `BroadcastExtension` target 中。

1. **获取文件**：这些文件通常来自 LiveKit 的示例代码。
   - `SampleHandler.swift` (主逻辑)
   - `SampleUploader.swift`
   - `SocketConnection.swift`
   - `DarwinNotificationCenter.swift`
   - `Atomic.swift`

2. **修改 `SampleHandler.swift`**：
   - 确保 `appGroupIdentifier` 与你在第二步中创建的 Group ID 一致。
   
   ```swift
   import LiveKit
   // ...
   private enum Constants {
       static let appGroupIdentifier = "group.com.example.flutterSfuRelayExample" // 修改这里
   }
   ```

3. **修复 `broadcastLogger` 作用域问题**：
   - 由于 Swift 文件间作用域隔离，`SampleHandler.swift` 中定义的全局 `broadcastLogger` 可能无法被其他文件访问。
   - 建议在 `SocketConnection.swift` 和 `SampleUploader.swift` 两个文件的头部（import 语句下）**也添加**同样的 Logger 定义：

   ```swift
   // 在 SocketConnection.swift 和 SampleUploader.swift 中添加：
   import OSLog
   
   private let broadcastLogger = OSLog(subsystem: "com.example.flutterSfuRelayExample", category: "Broadcast")
   ```

---

## ⚙️ 第四步：配置 Info.plist 和 Build Settings

### 1. 主 App Info.plist (`ios/Runner/Info.plist`)
添加 `RTCAppGroupIdentifier` 键，值必须是你的 App Group ID。这告诉 Flutter SDK 使用哪个 Group 进行通信。

```xml
<key>RTCAppGroupIdentifier</key>
<string>group.com.example.flutterSfuRelayExample</string>
<key>NSCameraUsageDescription</key>
<string>我们需要摄像头权限进行视频会议</string>
<key>NSMicrophoneUsageDescription</key>
<string>我们需要麦克风权限进行音频会议</string>
```

### 2. Extension Info.plist (`ios/BroadcastExtension/Info.plist`)
确保 `RPBroadcastProcessMode` 设置为 `RPBroadcastProcessModeSampleBuffer`。

### 3. Deployment Target
为了支持 `os_log` 等现代 API，请确保 `BroadcastExtension` target 的 **iOS Deployment Target** 设置为 **14.0** 或更高。

---

## 🐛 第五步：解决构建问题 (Troubleshooting)

### 1. Build Cycle Error (循环依赖)
如果遇到 `Cycle inside Runner; building could produce unreliable results` 错误，通常是因为 "Embed Foundation Extensions" 构建阶段的位置问题。

**解决方法**：
1. 在 Xcode 中选择 **Runner** target -> **Build Phases**。
2. 找到 **Embed Foundation Extensions** 阶段。
3. 将其**向上拖动**，放置在 `Thin Binary` 和 `[CP] Embed Pods Frameworks` 之前。
   - 推荐顺序：`Embed Frameworks` -> `Embed Foundation Extensions` -> `Thin Binary` -> Pods 脚本。

### 2. Signing / Provisioning
确保两个 Target (`Runner` 和 `BroadcastExtension`) 都选择了正确的开发团队，并且 Provisioning Profile 状态正常（Xcode 通常会自动管理）。

---

## 📱 第六步：Flutter 端调用

在 Flutter 代码中，iOS 屏幕共享需要特殊处理：

1. **检测设备**：模拟器不支持 ReplayKit，使用前请检查。
2. **启用共享**：调用 `setScreenShareEnabled(true)`。

```dart
import 'dart:io';
import 'package:device_info_plus/device_info_plus.dart';

// ...

if (!kIsWeb && Platform.isIOS) {
  // 1. 检查模拟器
  final deviceInfo = DeviceInfoPlugin();
  final iosInfo = await deviceInfo.iosInfo;
  
  if (!iosInfo.isPhysicalDevice) {
    print("iOS 模拟器不支持屏幕共享");
    return;
  }

  // 2. 启用屏幕共享
  // 这会触发系统的屏幕录制弹窗
  await _localParticipant!.setScreenShareEnabled(true);
}
```

---

## ✅ 验证

1. 连上 iPhone 真机。
2. 运行 `flutter run --release` (或 debug)。
3. 点击屏幕共享按钮。
4. 系统应弹出 "屏幕直播" 提示框，选择你的应用并点击 "开始直播"。
5. 状态栏时间变红，表示正在录屏，远端应能看到你的屏幕画面。
