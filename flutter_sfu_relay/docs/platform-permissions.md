# 平台权限配置

使用 `flutter_sfu_relay` 进行音视频会议需要在各平台配置相应的权限。

## Android

编辑 `android/app/src/main/AndroidManifest.xml`，在 `<manifest>` 标签内添加：

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.CAMERA"/>
<uses-permission android:name="android.permission.RECORD_AUDIO"/>
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE"/>
<uses-permission android:name="android.permission.CHANGE_NETWORK_STATE"/>
<uses-permission android:name="android.permission.MODIFY_AUDIO_SETTINGS"/>
<uses-permission android:name="android.permission.BLUETOOTH" android:maxSdkVersion="30"/>
<uses-permission android:name="android.permission.BLUETOOTH_CONNECT"/>
```

## iOS

编辑 `ios/Runner/Info.plist`，在 `<dict>` 内添加：

```xml
<key>NSCameraUsageDescription</key>
<string>此应用需要访问摄像头以进行视频通话</string>
<key>NSMicrophoneUsageDescription</key>
<string>此应用需要访问麦克风以进行语音通话</string>
```

如果需要后台音频，还需添加：

```xml
<key>UIBackgroundModes</key>
<array>
    <string>audio</string>
    <string>voip</string>
</array>
```

## macOS

### Info.plist

编辑 `macos/Runner/Info.plist`，在 `<dict>` 内添加：

```xml
<key>NSMicrophoneUsageDescription</key>
<string>此应用需要访问麦克风以进行语音通话</string>
<key>NSCameraUsageDescription</key>
<string>此应用需要访问摄像头以进行视频通话</string>
```

### Entitlements

编辑 `macos/Runner/DebugProfile.entitlements` 和 `macos/Runner/Release.entitlements`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.app-sandbox</key>
    <true/>
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    <key>com.apple.security.network.server</key>
    <true/>
    <key>com.apple.security.network.client</key>
    <true/>
    <key>com.apple.security.device.camera</key>
    <true/>
    <key>com.apple.security.device.audio-input</key>
    <true/>
</dict>
</plist>
```

## Windows

Windows 不需要在项目中配置静态权限。系统会在运行时自动弹出权限请求对话框。

确保用户在弹出时点击"允许"即可。

## Linux

Linux 权限由系统音频服务（PulseAudio/PipeWire）处理，不需要额外配置。

确保系统已安装必要的依赖：

```bash
# Ubuntu/Debian
sudo apt install libpulse-dev

# Fedora
sudo dnf install pulseaudio-libs-devel
```

## Web

Web 平台权限由浏览器自动管理。当应用请求摄像头或麦克风时，浏览器会弹出权限询问。

如果项目没有 `web` 目录，可以通过以下命令添加：

```bash
flutter create --platforms=web .
```

> **注意**：Web 需要 HTTPS 才能访问媒体设备（localhost 除外）。

## 权限检查表

| 平台 | 麦克风 | 摄像头 | 网络 | 屏幕共享 |
|------|--------|--------|------|----------|
| Android | `RECORD_AUDIO` | `CAMERA` | `INTERNET` | MediaProjection API |
| iOS | `NSMicrophoneUsageDescription` | `NSCameraUsageDescription` | 默认允许 | ReplayKit |
| macOS | `device.audio-input` | `device.camera` | `network.client` | 系统弹窗 |
| Windows | 运行时弹窗 | 运行时弹窗 | 默认允许 | 运行时弹窗 |
| Linux | PulseAudio | V4L2 | 默认允许 | PipeWire |
| Web | 浏览器弹窗 | 浏览器弹窗 | 默认允许 | 浏览器弹窗 |

## 常见问题

### 1. macOS 没有声音

确保：
- `Info.plist` 包含 `NSMicrophoneUsageDescription`
- `Entitlements` 包含 `device.audio-input`
- 运行时弹出权限请求时点击了"允许"
- 检查 系统偏好设置 → 隐私与安全 → 麦克风

### 2. Android 权限被拒绝

使用 `permission_handler` 包在运行时请求权限：

```dart
import 'package:permission_handler/permission_handler.dart';

await [Permission.camera, Permission.microphone].request();
```

### 3. iOS 权限被拒绝

引导用户到 设置 → 隐私 → 麦克风/摄像头 中手动开启。
