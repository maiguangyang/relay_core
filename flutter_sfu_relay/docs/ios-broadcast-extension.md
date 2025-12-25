# iOS Broadcast Extension 配置指南

iOS 屏幕共享需要配置 **Broadcast Upload Extension**。以下是详细步骤：

## 前提条件

- Xcode 14+
- Apple Developer 账号
- iOS 真机（模拟器不支持屏幕共享）

## 配置步骤

### 1. 在 Xcode 中打开项目

```bash
open ios/Runner.xcworkspace
```

### 2. 创建 Broadcast Upload Extension

1. 菜单: **File → New → Target**
2. 搜索 **Broadcast Upload Extension**
3. 点击 **Next**
4. 配置:
   - **Product Name**: `BroadcastExtension`
   - **Language**: Swift
   - **Include UI Extension**: ❌ 取消勾选
5. 点击 **Finish**
6. 弹出激活 scheme 时点击 **Activate**

### 3. 配置 App Group

#### 主应用 (Runner)
1. 选择 **Runner** target
2. 点击 **Signing & Capabilities** 标签
3. 点击 **+ Capability**
4. 添加 **App Groups**
5. 点击 **+** 添加: `group.com.example.flutterSfuRelayExample`

#### Broadcast Extension
1. 选择 **BroadcastExtension** target
2. 点击 **Signing & Capabilities** 标签
3. 点击 **+ Capability**
4. 添加 **App Groups**
5. 添加相同的 App Group: `group.com.example.flutterSfuRelayExample`

### 4. 修改 SampleHandler.swift

替换 `BroadcastExtension/SampleHandler.swift` 内容:

```swift
import ReplayKit
import LiveKit

class SampleHandler: LKSampleHandler {
    override init() {
        super.init()
        // 确保与主应用使用相同的 App Group
        self.appGroupIdentifier = "group.com.example.flutterSfuRelayExample"
    }
}
```

### 5. 添加 LiveKit 依赖（CocoaPods）

编辑 `ios/Podfile`，添加 Broadcast Extension target:

```ruby
target 'BroadcastExtension' do
  use_frameworks!
  pod 'LiveKit', '~> 2.0'
end
```

然后运行:
```bash
cd ios && pod install
```

### 6. 更新主应用 Info.plist

在 `ios/Runner/Info.plist` 中添加:

```xml
<key>RTCScreenSharingExtension</key>
<string>com.example.flutterSfuRelayExample.BroadcastExtension</string>
<key>RTCAppGroupIdentifier</key>
<string>group.com.example.flutterSfuRelayExample</string>
```

### 7. 设置 Broadcast Extension 部署目标

1. 选择 **BroadcastExtension** target
2. **General** 标签
3. 设置 **Minimum Deployments → iOS** 为 `13.0`

### 8. Flutter 代码更新

屏幕共享代码已在 `home_page.dart` 中处理。确保调用:

```dart
await _localParticipant!.setScreenShareEnabled(
  true,
  screenShareOptions: ScreenShareCaptureOptions(
    useiOSBroadcastExtension: true,  // iOS 使用 Broadcast Extension
  ),
);
```

## 验证

1. 在 iOS 真机上运行应用
2. 加入会议
3. 点击屏幕共享按钮
4. 系统会弹出 Broadcast 选择器
5. 选择你的应用开始共享

## 常见问题

### Extension 不出现在选择器中
- 确保 App Group 配置正确且两个 target 使用相同的 ID
- 确保代码签名正确
- 重新构建并安装到真机

### 屏幕共享没有画面
- 检查 SampleHandler 中的 appGroupIdentifier 是否正确
- 查看 Xcode Console 日志

## 参考资料

- [LiveKit iOS Screen Sharing](https://docs.livekit.io/references/client-sdk-ios/#screen-sharing)
- [Apple ReplayKit](https://developer.apple.com/documentation/replaykit)
