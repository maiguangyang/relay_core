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
  
  # 使用预编译的 Go XCFramework
  s.vendored_frameworks = 'librelay.xcframework'
  
  s.dependency 'Flutter'
  s.platform = :ios, '13.0'

  # Flutter.framework does not contain a i386 slice.
  s.pod_target_xcconfig = { 
    'DEFINES_MODULE' => 'YES', 
    'EXCLUDED_ARCHS[sdk=iphonesimulator*]' => 'i386',
    'LD_RUNPATH_SEARCH_PATHS' => '$(inherited) @executable_path/Frameworks @loader_path/Frameworks'
  }
end
