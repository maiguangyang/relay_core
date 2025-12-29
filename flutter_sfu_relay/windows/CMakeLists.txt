# The Flutter tooling requires that developers have a version of Visual Studio
# installed that includes CMake 3.14 or later. You should not increase this
# version, as doing so will cause the plugin to fail to compile for some
# customers of the plugin.
cmake_minimum_required(VERSION 3.14)

# Project-level configuration.
# Flutter expects the plugin target to be named ${plugin_name}_plugin
set(PROJECT_NAME "flutter_sfu_relay_plugin")
project(${PROJECT_NAME} LANGUAGES C CXX)

# 预编译的 Go 库路径 (已复制到当前目录)
set(PREBUILT_LIB_PATH "${CMAKE_CURRENT_SOURCE_DIR}")

# Screen Share Plugin 源文件
set(PLUGIN_SOURCES
  "screen_share_plugin.cpp"
  "include/flutter_sfu_relay/screen_share_plugin.h"
)

# 创建插件库
add_library(${PROJECT_NAME} SHARED ${PLUGIN_SOURCES})

# 设置插件属性
target_compile_definitions(${PROJECT_NAME} PRIVATE FLUTTER_PLUGIN_IMPL)

# Apply standard settings from the Flutter plugin interface
apply_standard_settings(${PROJECT_NAME})

# Include directories: PRIVATE for internal compilation, INTERFACE for consumers
target_include_directories(${PROJECT_NAME} PRIVATE
  "${CMAKE_CURRENT_SOURCE_DIR}"
  "${FLUTTER_EPHEMERAL_DIR}/include"
  "${FLUTTER_EPHEMERAL_DIR}/include/flutter"
)
target_include_directories(${PROJECT_NAME} INTERFACE
  "${CMAKE_CURRENT_SOURCE_DIR}/include")

target_link_libraries(${PROJECT_NAME} PRIVATE flutter flutter_wrapper_plugin user32.lib gdi32.lib dwmapi.lib)

# 如果预编译库存在，直接使用
if(EXISTS "${PREBUILT_LIB_PATH}/librelay.dll")
  # 设置要打包的库
  set(flutter_sfu_relay_bundled_libraries
    "${PREBUILT_LIB_PATH}/librelay.dll"
    PARENT_SCOPE
  )
else()
  message(WARNING "Pre-built library not found at ${PREBUILT_LIB_PATH}/librelay.dll")
  message(WARNING "Please run 'build_all.sh' first to generate the Go library")
  
  # 创建空目标以避免构建失败
  set(flutter_sfu_relay_bundled_libraries "" PARENT_SCOPE)
endif()
