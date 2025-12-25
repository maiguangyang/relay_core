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
    'OTHER_LDFLAGS' => '$(inherited) -ObjC',
  }
  
  # 读取导出符号列表，使用 -exported_symbol（单数）添加每个符号
  # 这不会替换默认导出列表，所以 _main 等符号仍然存在
  exports_file = File.expand_path('librelay_exports.txt', __dir__)
  if File.exist?(exports_file)
    symbols = File.readlines(exports_file).map(&:strip).reject(&:empty?)
    exported_flags = symbols.map { |sym| "-Wl,-exported_symbol,#{sym}" }.join(' ')
    
    s.user_target_xcconfig = {
      # 必须手动导出 _main，否则 "No entry point found"
      # 同时导出 Go 符号
      'OTHER_LDFLAGS' => "$(inherited) -ObjC -Wl,-exported_symbol,_main #{exported_flags}",
    }
  else
    s.user_target_xcconfig = {
      'OTHER_LDFLAGS' => '$(inherited) -ObjC',
    }
  end
end
