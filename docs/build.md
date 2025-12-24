# 构建配置

## 环境要求

| 工具 | 版本 | 用途 |
|-----|------|-----|
| Go | 1.21+ | 编译 |
| Xcode | 14+ | iOS/macOS 构建 |
| Android NDK | r25+ | Android 交叉编译 |
| musl-cross (可选) | - | Linux 交叉编译 |
| zig (可选) | - | 替代交叉编译器 |

## 快速构建

```bash
# 赋予执行权限
chmod +x build_all.sh

# 构建全平台
./build_all.sh
```

## 构建产物

```
build/
├── android/
│   ├── jniLibs/arm64-v8a/librelay.so
│   └── librelay.h
├── ios/
│   └── librelay.xcframework/
├── linux/
│   ├── librelay.so
│   └── librelay.h
├── macos/
│   ├── librelay.dylib
│   └── librelay.h
└── windows/
    ├── librelay.dll
    └── librelay.h
```

## 单平台构建

### Android

```bash
export ANDROID_NDK_HOME=$HOME/Library/Android/sdk/ndk/25.0.0
export CC=$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/darwin-x86_64/bin/aarch64-linux-android21-clang

CGO_ENABLED=1 \
GOOS=android \
GOARCH=arm64 \
CC=$CC \
go build -buildmode=c-shared \
  -ldflags="-s -w -checklinkname=0" \
  -o build/android/librelay.so
```

### iOS

```bash
# iPhoneOS (arm64)
SDK=$(xcrun --sdk iphoneos --show-sdk-path)
CGO_ENABLED=1 \
GOOS=ios \
GOARCH=arm64 \
CGO_CFLAGS="-isysroot $SDK -arch arm64" \
CGO_LDFLAGS="-isysroot $SDK -arch arm64" \
go build -buildmode=c-archive -o build/ios/librelay_arm64.a

# iOS Simulator (arm64)
SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
CGO_ENABLED=1 \
GOOS=ios \
GOARCH=arm64 \
CGO_CFLAGS="-isysroot $SDK -arch arm64" \
CGO_LDFLAGS="-isysroot $SDK -arch arm64" \
go build -buildmode=c-archive -tags ios -o build/ios/librelay_sim_arm64.a

# 创建 XCFramework
xcodebuild -create-xcframework \
  -library build/ios/librelay_arm64.a -headers build/ios/ \
  -library build/ios/librelay_sim_arm64.a -headers build/ios/ \
  -output build/ios/librelay.xcframework
```

### macOS

```bash
# Universal Binary (x86_64 + arm64)
CGO_ENABLED=1 \
GOOS=darwin \
GOARCH=amd64 \
go build -buildmode=c-shared -o build/macos/librelay_amd64.dylib

CGO_ENABLED=1 \
GOOS=darwin \
GOARCH=arm64 \
go build -buildmode=c-shared -o build/macos/librelay_arm64.dylib

# 合并
lipo -create -output build/macos/librelay.dylib \
  build/macos/librelay_amd64.dylib \
  build/macos/librelay_arm64.dylib
```

### Windows

```bash
CGO_ENABLED=1 \
GOOS=windows \
GOARCH=amd64 \
CC=x86_64-w64-mingw32-gcc \
go build -buildmode=c-shared -o build/windows/librelay.dll
```

### Linux

```bash
# 使用 musl (推荐)
CGO_ENABLED=1 \
GOOS=linux \
GOARCH=amd64 \
CC=x86_64-linux-musl-gcc \
go build -buildmode=c-shared -o build/linux/librelay.so

# 或使用 zig
CGO_ENABLED=1 \
GOOS=linux \
GOARCH=amd64 \
CC="zig cc -target x86_64-linux-musl" \
go build -buildmode=c-shared -o build/linux/librelay.so
```

## 常见问题

### Android: `wlynxg/anet: invalid reference to net.zoneCache`

**原因**: Go 1.23+ 与 `wlynxg/anet` 库的链接兼容性问题

**解决方案**: 添加链接器标志

```bash
-ldflags="-s -w -checklinkname=0"
```

### iOS: `stdlib.h file not found`

**原因**: CGO 未正确配置 SDK 路径

**解决方案**: 设置 `CGO_CFLAGS` 和 `CGO_LDFLAGS`

```bash
SDK=$(xcrun --sdk iphoneos --show-sdk-path)
CGO_CFLAGS="-isysroot $SDK -arch arm64"
CGO_LDFLAGS="-isysroot $SDK -arch arm64"
```

### Windows: 找不到 mingw-w64

**解决方案**: 安装 mingw-w64

```bash
# macOS
brew install mingw-w64

# Ubuntu
apt install mingw-w64
```

### Linux 交叉编译失败

**解决方案**: 安装 musl-cross 或 zig

```bash
# macOS - 安装 musl-cross
brew install FiloSottile/musl-cross/musl-cross

# 或安装 zig
brew install zig
```

## 库大小优化

```bash
# 使用 -s -w 去除调试信息
-ldflags="-s -w"

# 使用 UPX 进一步压缩（可选）
upx --best librelay.so
```

## 验证构建

```bash
# 检查导出符号
nm -gU build/macos/librelay.dylib | grep -E "^[0-9a-f]+ T _"

# 检查大小
ls -lh build/*/librelay.*

# 测试编译
go build -buildmode=c-shared -o /tmp/test.dylib .
```
