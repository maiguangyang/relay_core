#!/bin/bash

# ==========================================
# 配置部分 (请根据你的环境修改 NDK 路径)
# ==========================================
PROJECT_NAME="librelay"
OUTPUT_DIR="./build"
GO_ENTRY_POINT="."

# 尝试自动查找 NDK 路径，如果找不到，请手动修改下面的路径
# 常见路径: ~/Library/Android/sdk/ndk/<version>
ANDROID_NDK_HOME=$(ls -d $HOME/Library/Android/sdk/ndk/* | sort -V | tail -n 1)

# 颜色
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== 开始编译 FFI 专用库 (C-Shared/C-Archive) ===${NC}"
echo -e "NDK Path detected: $ANDROID_NDK_HOME"

if [ -z "$ANDROID_NDK_HOME" ]; then
    echo -e "${RED}Error: Could not find Android NDK. Please set ANDROID_NDK_HOME manually in script.${NC}"
    exit 1
fi

rm -rf $OUTPUT_DIR
mkdir -p $OUTPUT_DIR

# ==========================================
# 1. Android (生成 .so) - 手动交叉编译
# ==========================================
echo -e "${YELLOW}[1/5] Compiling for Android (arm64-v8a)...${NC}"
mkdir -p $OUTPUT_DIR/android/jniLibs/arm64-v8a

# 确定 NDK 工具链路径 (macOS)
TOOLCHAIN="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/darwin-x86_64/bin"
# Android API Level (通常选 24+)
API=24
CC_ANDROID="$TOOLCHAIN/aarch64-linux-android$API-clang"

if [ ! -f "$CC_ANDROID" ]; then
     echo -e "${RED}Error: NDK Compiler not found at $CC_ANDROID${NC}"
     exit 1
fi

# 核心命令：CGO_ENABLED=1 + 指定 CC
# -checklinkname=0 修复 Go 1.23+ 对 wlynxg/anet 的 linkname 限制
CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$CC_ANDROID \
go build -ldflags="-s -w -checklinkname=0" -buildmode=c-shared -o $OUTPUT_DIR/android/jniLibs/arm64-v8a/librelay.so $GO_ENTRY_POINT

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✔ Android arm64 build success${NC}"
else
    echo -e "${RED}✘ Android build failed${NC}"
    exit 1
fi

# (如果需要 armeabi-v7a，需修改 GOARCH=arm 并指定对应的 armv7a-linux-androideabi$API-clang)

# ==========================================
# 2. iOS (生成 .xcframework)
# ==========================================
echo -e "${YELLOW}[2/5] Compiling for iOS (Static Lib + XCFramework)...${NC}"
mkdir -p $OUTPUT_DIR/ios

# 2.1 编译 iPhone 真机 (arm64)
echo "   Building for iPhoneOS..."
IPHONEOS_SDK=$(xcrun -sdk iphoneos --show-sdk-path)
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
CC="$(xcrun -sdk iphoneos -find clang) -isysroot $IPHONEOS_SDK -arch arm64" \
CGO_CFLAGS="-isysroot $IPHONEOS_SDK -arch arm64 -miphoneos-version-min=12.0" \
CGO_LDFLAGS="-isysroot $IPHONEOS_SDK -arch arm64 -miphoneos-version-min=12.0" \
go build -ldflags="-s -w -checklinkname=0" -buildmode=c-archive -o $OUTPUT_DIR/ios/${PROJECT_NAME}_arm64.a $GO_ENTRY_POINT

# 2.2 编译 iPhone 模拟器 (arm64 + amd64)
# 注意：现在的模拟器很多也是 arm64 (M1/M2/M3 Mac)
echo "   Building for iOS Simulator (arm64)..."
IPHONESIM_SDK=$(xcrun -sdk iphonesimulator --show-sdk-path)
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
CC="$(xcrun -sdk iphonesimulator -find clang) -isysroot $IPHONESIM_SDK -arch arm64" \
CGO_CFLAGS="-isysroot $IPHONESIM_SDK -arch arm64 -mios-simulator-version-min=12.0" \
CGO_LDFLAGS="-isysroot $IPHONESIM_SDK -arch arm64 -mios-simulator-version-min=12.0" \
go build -ldflags="-s -w -checklinkname=0" -buildmode=c-archive -o $OUTPUT_DIR/ios/${PROJECT_NAME}_sim_arm64.a $GO_ENTRY_POINT

echo "   Building for iOS Simulator (amd64)..."
CGO_ENABLED=1 GOOS=ios GOARCH=amd64 \
CC="$(xcrun -sdk iphonesimulator -find clang) -isysroot $IPHONESIM_SDK -arch x86_64" \
CGO_CFLAGS="-isysroot $IPHONESIM_SDK -arch x86_64 -mios-simulator-version-min=12.0" \
CGO_LDFLAGS="-isysroot $IPHONESIM_SDK -arch x86_64 -mios-simulator-version-min=12.0" \
go build -ldflags="-s -w -checklinkname=0" -buildmode=c-archive -o $OUTPUT_DIR/ios/${PROJECT_NAME}_sim_amd64.a $GO_ENTRY_POINT

# 合并模拟器架构 (Universal Static Lib)
lipo -create -output $OUTPUT_DIR/ios/${PROJECT_NAME}_sim.a \
    $OUTPUT_DIR/ios/${PROJECT_NAME}_sim_arm64.a \
    $OUTPUT_DIR/ios/${PROJECT_NAME}_sim_amd64.a

# 2.3 准备用于 XCFramework 的目录结构
# CocoaPods 要求 xcframework 中每个 slice 的 library 名称必须相同
echo "   Preparing files for XCFramework..."
mkdir -p $OUTPUT_DIR/ios/device
mkdir -p $OUTPUT_DIR/ios/simulator

# 复制并重命名为统一的名称 (librelay.a)
cp $OUTPUT_DIR/ios/${PROJECT_NAME}_arm64.a $OUTPUT_DIR/ios/device/${PROJECT_NAME}.a
cp $OUTPUT_DIR/ios/${PROJECT_NAME}_arm64.h $OUTPUT_DIR/ios/device/Headers
cp $OUTPUT_DIR/ios/${PROJECT_NAME}_sim.a $OUTPUT_DIR/ios/simulator/${PROJECT_NAME}.a
cp $OUTPUT_DIR/ios/${PROJECT_NAME}_sim_arm64.h $OUTPUT_DIR/ios/simulator/Headers

# 2.4 生成 XCFramework (这是 iOS 现代集成的标准方式)
echo "   Creating XCFramework..."
xcodebuild -create-xcframework \
    -library $OUTPUT_DIR/ios/device/${PROJECT_NAME}.a \
    -headers $OUTPUT_DIR/ios/device/Headers \
    -library $OUTPUT_DIR/ios/simulator/${PROJECT_NAME}.a \
    -headers $OUTPUT_DIR/ios/simulator/Headers \
    -output $OUTPUT_DIR/ios/$PROJECT_NAME.xcframework

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✔ iOS XCFramework build success${NC}"
    # 清理中间文件
    rm -rf $OUTPUT_DIR/ios/device $OUTPUT_DIR/ios/simulator
    rm $OUTPUT_DIR/ios/*.a $OUTPUT_DIR/ios/*.h 2>/dev/null || true
else
    echo -e "${RED}✘ iOS build failed${NC}"
    exit 1
fi

# ==========================================
# 3. macOS (生成 Universal .dylib)
# ==========================================
echo -e "${YELLOW}[3/5] Compiling for macOS...${NC}"
mkdir -p $OUTPUT_DIR/macos

CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -buildmode=c-shared -o $OUTPUT_DIR/macos/${PROJECT_NAME}_arm64.dylib $GO_ENTRY_POINT
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -buildmode=c-shared -o $OUTPUT_DIR/macos/${PROJECT_NAME}_amd64.dylib $GO_ENTRY_POINT

lipo -create -output $OUTPUT_DIR/macos/$PROJECT_NAME.dylib \
    $OUTPUT_DIR/macos/${PROJECT_NAME}_arm64.dylib \
    $OUTPUT_DIR/macos/${PROJECT_NAME}_amd64.dylib

# 保留一份头文件，清理其他
mv $OUTPUT_DIR/macos/${PROJECT_NAME}_arm64.h $OUTPUT_DIR/macos/$PROJECT_NAME.h 2>/dev/null || true
rm $OUTPUT_DIR/macos/${PROJECT_NAME}_*.dylib $OUTPUT_DIR/macos/${PROJECT_NAME}_*.h 2>/dev/null || true
echo -e "${GREEN}✔ macOS build success${NC}"

# ==========================================
# 4. Windows (需 MinGW)
# ==========================================
echo -e "${YELLOW}[4/5] Compiling for Windows...${NC}"
mkdir -p $OUTPUT_DIR/windows
if command -v x86_64-w64-mingw32-gcc &> /dev/null; then
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
    go build -ldflags="-s -w" -buildmode=c-shared -o $OUTPUT_DIR/windows/$PROJECT_NAME.dll $GO_ENTRY_POINT
    echo -e "${GREEN}✔ Windows build success${NC}"
else
    echo -e "${RED}Skipping Windows (mingw not found, run: brew install mingw-w64)${NC}"
fi

# ==========================================
# 5. Linux (需要交叉编译器或在 Linux 上直接编译)
# ==========================================
echo -e "${YELLOW}[5/5] Compiling for Linux...${NC}"
mkdir -p $OUTPUT_DIR/linux

# 检查是否有 Linux 交叉编译器 (zig 或 musl-gcc)
if command -v zig &> /dev/null; then
    # 使用 Zig 作为交叉编译器 (推荐方式)
    CC="zig cc -target x86_64-linux-gnu" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -checklinkname=0" -buildmode=c-shared -o $OUTPUT_DIR/linux/$PROJECT_NAME.so $GO_ENTRY_POINT
    echo -e "${GREEN}✔ Linux (x64) build success (via zig)${NC}"
elif command -v x86_64-linux-musl-gcc &> /dev/null; then
    # 使用 musl-cross 工具链
    CC=x86_64-linux-musl-gcc CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -checklinkname=0 -linkmode external -extldflags '-static'" -buildmode=c-shared -o $OUTPUT_DIR/linux/$PROJECT_NAME.so $GO_ENTRY_POINT
    echo -e "${GREEN}✔ Linux (x64) build success (via musl)${NC}"
elif [[ "$(uname -s)" == "Linux" ]]; then
    # 在 Linux 上直接编译
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -checklinkname=0" -buildmode=c-shared -o $OUTPUT_DIR/linux/$PROJECT_NAME.so $GO_ENTRY_POINT
    echo -e "${GREEN}✔ Linux (x64) build success${NC}"
else
    echo -e "${RED}Skipping Linux build (no cross-compiler found)${NC}"
    echo -e "${RED}  Install options: brew install zig OR brew install FiloSottile/musl-cross/musl-cross${NC}"
fi

echo -e "${GREEN}=== 🎉 构建完成！请检查 $OUTPUT_DIR ===${NC}"