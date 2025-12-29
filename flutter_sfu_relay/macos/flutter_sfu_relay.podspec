#
# To learn more about a Podspec see http://guides.cocoapods.org/syntax/podspec.html.
# Run `pod lib lint flutter_sfu_relay.podspec` to validate before publishing.
#
Pod::Spec.new do |s|
  s.name             = 'flutter_sfu_relay'
  s.version          = '0.0.1'
  s.summary          = 'Flutter SFU Relay - 局域网代理转发 SDK'
  s.description      = <<-DESC
基于 Pion WebRTC 的嵌入式微型 SFU 核心，支持 RTP 纯透传转发。
                       DESC
  s.homepage         = 'https://github.com/example/flutter_sfu_relay'
  s.license          = { :file => '../LICENSE' }
  s.author           = { 'Your Company' => 'email@example.com' }

  s.source           = { :path => '.' }
  
  # 使用预编译的 Go 动态库
  s.vendored_libraries = 'librelay.dylib'
  
  # 头文件和 Swift 源码
  s.public_header_files = 'librelay.h'
  s.source_files = 'librelay.h', 'Classes/**/*.swift'

  s.dependency 'FlutterMacOS'

  s.platform = :osx, '10.14'
  s.pod_target_xcconfig = { 
    'DEFINES_MODULE' => 'YES',
    # 确保运行时可以找到动态库
    'LD_RUNPATH_SEARCH_PATHS' => '$(inherited) @executable_path/../Frameworks @loader_path/../Frameworks'
  }
  s.swift_version = '5.0'
end
