# The Flutter tooling requires that developers have CMake 3.10 or later
# installed. You should not increase this version, as doing so will cause
# the plugin to fail to compile for some customers of the plugin.
cmake_minimum_required(VERSION 3.10)

# Project-level configuration.
set(PROJECT_NAME "flutter_sfu_relay")
project(${PROJECT_NAME} LANGUAGES C CXX)

# 预编译的 Go 库路径 (已复制到当前目录)
set(PREBUILT_LIB_PATH "${CMAKE_CURRENT_SOURCE_DIR}")

# Find required packages
find_package(PkgConfig REQUIRED)
pkg_check_modules(GTK REQUIRED gtk+-3.0)
pkg_check_modules(GLIB REQUIRED glib-2.0)

# This value is used when generating builds using this plugin, so it must
# not be changed
set(PLUGIN_NAME "flutter_sfu_relay_plugin")

# Define the plugin library target
add_library(${PLUGIN_NAME} SHARED
  screen_share_plugin.cc
)

# Apply a standard set of build settings
apply_standard_settings(${PLUGIN_NAME})

# Set compiler flags
set_target_properties(${PLUGIN_NAME} PROPERTIES
  CXX_VISIBILITY_PRESET hidden
  CXX_STANDARD 17
)
target_compile_definitions(${PLUGIN_NAME} PRIVATE FLUTTER_PLUGIN_IMPL)

# Include directories
target_include_directories(${PLUGIN_NAME} PRIVATE
  "${CMAKE_CURRENT_SOURCE_DIR}"
  ${GTK_INCLUDE_DIRS}
  ${GLIB_INCLUDE_DIRS}
)

# Link libraries
target_link_libraries(${PLUGIN_NAME} PRIVATE
  flutter
  ${GTK_LIBRARIES}
  ${GLIB_LIBRARIES}
)

# 如果预编译库存在，链接它
if(EXISTS "${PREBUILT_LIB_PATH}/librelay.so")
  # 添加预编译库作为链接库
  target_link_libraries(${PLUGIN_NAME} PRIVATE
    "${PREBUILT_LIB_PATH}/librelay.so"
  )
  
  # 设置要打包的库
  set(flutter_sfu_relay_bundled_libraries
    "${PREBUILT_LIB_PATH}/librelay.so"
    PARENT_SCOPE
  )
else()
  message(WARNING "Pre-built library not found at ${PREBUILT_LIB_PATH}/librelay.so")
  message(WARNING "Please run 'build_all.sh' first to generate the Go library")
  
  # 设置为空
  set(flutter_sfu_relay_bundled_libraries "" PARENT_SCOPE)
endif()
