# Makefile for flutter_sfu_relay
# Copies built artifacts from ../build to the plugin platform directories.

BUILD_DIR := ../build

.PHONY: copy clean copy-ios copy-macos copy-android copy-windows copy-linux

copy: copy-ios copy-macos copy-android copy-windows copy-linux

copy-ios:
	@echo "👉 Copying iOS artifacts..."
	@rm -rf ios/librelay.xcframework
	@rm -f ios/Classes/librelay.h
	@# Clean potential leftovers
	@rm -f ios/Classes/ios_*.h
	@cp -R $(BUILD_DIR)/ios/librelay.xcframework ios/
	@cp $(BUILD_DIR)/macos/librelay.h ios/Classes/librelay.h
	@echo "✔ iOS done"

copy-macos:
	@echo "👉 Copying macOS artifacts..."
	@rm -f macos/librelay.dylib
	@rm -f macos/librelay.h
	@# Clean potential leftovers (intermediate builds)
	@rm -f macos/librelay_*.dylib macos/librelay_*.h
	@cp $(BUILD_DIR)/macos/librelay.dylib macos/
	@cp $(BUILD_DIR)/macos/librelay.h macos/
	@# Fix install name (LC_ID_DYLIB)
	@wd=`pwd`; cd macos && ./fix_install_name.sh && cd $$wd
	@echo "✔ macOS done"

copy-android:
	@echo "👉 Copying Android artifacts..."
	@rm -rf android/jniLibs/arm64-v8a
	@# Clean potential leftovers
	@rm -rf android/jniLibs/armeabi-v7a android/jniLibs/x86_64
	@mkdir -p android/jniLibs/arm64-v8a
	@cp $(BUILD_DIR)/android/jniLibs/arm64-v8a/librelay.so android/jniLibs/arm64-v8a/
	@echo "✔ Android done"

copy-windows:
	@echo "👉 Copying Windows artifacts..."
	@rm -f windows/librelay.dll
	@rm -f windows/librelay.h
	@# Clean potential leftovers
	@rm -f windows/*.dll windows/*.h
	@cp $(BUILD_DIR)/windows/librelay.dll windows/
	@# Check if header exists for Windows, if not skip
	@if [ -f $(BUILD_DIR)/windows/librelay.h ]; then cp $(BUILD_DIR)/windows/librelay.h windows/; fi
	@echo "✔ Windows done"

copy-linux:
	@echo "👉 Copying Linux artifacts..."
	@rm -f linux/librelay.so
	@rm -f linux/librelay.h
	@# Clean potential leftovers
	@rm -f linux/*.so linux/*.h
	@cp $(BUILD_DIR)/linux/librelay.so linux/
	@cp $(BUILD_DIR)/macos/librelay.h linux/librelay.h
	@echo "✔ Linux done"

clean:
	@echo "Cleaning plugin artifacts (Be careful, this removes libs needed to run)..."
	@rm -rf ios/librelay.xcframework
	@rm -f ios/Classes/librelay.h
	@rm -f macos/librelay.dylib macos/librelay.h macos/librelay_*.dylib macos/librelay_*.h
	@rm -rf android/jniLibs/arm64-v8a android/jniLibs/armeabi-v7a android/jniLibs/x86_64
	@rm -f windows/librelay.dll windows/librelay.h
	@rm -f linux/librelay.so linux/librelay.h
