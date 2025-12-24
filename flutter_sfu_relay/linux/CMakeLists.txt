# The Flutter tooling requires that developers have CMake 3.10 or later
# installed. You should not increase this version, as doing so will cause
# the plugin to fail to compile for some customers of the plugin.
cmake_minimum_required(VERSION 3.10)

# Project-level configuration.
set(PROJECT_NAME "flutter_sfu_relay")
project(${PROJECT_NAME} LANGUAGES C CXX)

# 预编译的 Go 库路径 (已复制到当前目录)
set(PREBUILT_LIB_PATH "${CMAKE_CURRENT_SOURCE_DIR}")

# 如果预编译库存在，直接使用
if(EXISTS "${PREBUILT_LIB_PATH}/librelay.so")
  # 创建导入目标
  add_library(${PROJECT_NAME} SHARED IMPORTED)
  set_target_properties(${PROJECT_NAME} PROPERTIES
    IMPORTED_LOCATION "${PREBUILT_LIB_PATH}/librelay.so"
  )
  
  # 设置要打包的库
  set(flutter_sfu_relay_bundled_libraries
    "${PREBUILT_LIB_PATH}/librelay.so"
    PARENT_SCOPE
  )
else()
  message(WARNING "Pre-built library not found at ${PREBUILT_LIB_PATH}/librelay.so")
  message(WARNING "Please run 'build_all.sh' first to generate the Go library")
  
  # 创建空目标以避免构建失败
  set(flutter_sfu_relay_bundled_libraries "" PARENT_SCOPE)
endif()
