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
  
  # 使用预编译的 Go XCFramework (包含静态库 .a)
  s.vendored_frameworks = 'librelay.xcframework'
  
  # 静态库需要的系统框架和库
  s.frameworks = 'Foundation', 'Security', 'SystemConfiguration', 'CoreFoundation'
  s.libraries = 'resolv'
  
  # 占位文件 (CocoaPods 需要至少一个源文件)
  s.source_files = 'Classes/**/*'
  
  s.dependency 'Flutter'
  s.platform = :ios, '13.0'
  
  # 静态库链接需要 use_frameworks! 或者下面的配置
  s.static_framework = true

  s.pod_target_xcconfig = { 
    'DEFINES_MODULE' => 'YES', 
    'EXCLUDED_ARCHS[sdk=iphonesimulator*]' => 'i386',
    'LD_RUNPATH_SEARCH_PATHS' => '$(inherited) @executable_path/Frameworks @loader_path/Frameworks',
    # 确保链接器能找到静态库
    'OTHER_LDFLAGS' => '$(inherited) -ObjC',
  }
  
  s.user_target_xcconfig = {
    'OTHER_LDFLAGS' => '$(inherited) -ObjC',
  }
end
