import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:flutter_sfu_relay/screen_share_helper.dart';
import '../theme/app_theme.dart';

/// Backwards-compatible wrapper that delegates to plugin's ScreenShareHelper
/// 向后兼容的包装类，代理到插件的 ScreenShareHelper
class ScreenCaptureChannel {
  static VoidCallback? get onStopSharingRequested =>
      ScreenShareHelper.onStopSharingRequested;
  static set onStopSharingRequested(VoidCallback? callback) {
    ScreenShareHelper.onStopSharingRequested = callback;
  }

  static void initialize() => ScreenShareHelper.initialize();

  static Future<bool> isSupported() async => ScreenShareHelper.isSupported;

  static Future<List<int>> getSelfWindowIDs() async => [];

  static Future<bool> showScreenShareUI() async {
    // Both Windows and macOS: Use setExcludeFromCapture to hide window from capture
    await ScreenShareHelper.setExcludeFromCapture(true);
    await ScreenShareHelper.showOverlay();
    return true;
  }

  static Future<bool> hideScreenShareUI() async {
    await ScreenShareHelper.setExcludeFromCapture(false);
    await ScreenShareHelper.hideOverlay();
    return true;
  }
}

/// 缩略图组件 - 监听缩略图变化
class _ThumbnailWidget extends StatefulWidget {
  const _ThumbnailWidget({
    required this.source,
    required this.selected,
    required this.onTap,
  });

  final rtc.DesktopCapturerSource source;
  final bool selected;
  final Function(rtc.DesktopCapturerSource) onTap;

  @override
  State<_ThumbnailWidget> createState() => _ThumbnailWidgetState();
}

class _ThumbnailWidgetState extends State<_ThumbnailWidget> {
  final List<StreamSubscription> _subscriptions = [];
  Uint8List? _thumbnail;
  String _name = '';

  @override
  void initState() {
    super.initState();
    _name = widget.source.name;
    _thumbnail = widget.source.thumbnail?.isNotEmpty == true
        ? widget.source.thumbnail
        : null;

    // 监听缩略图变化
    _subscriptions.add(
      widget.source.onThumbnailChanged.stream.listen((thumbnail) {
        if (mounted) {
          setState(() {
            _thumbnail = thumbnail;
          });
        }
      }),
    );

    // 监听名称变化
    _subscriptions.add(
      widget.source.onNameChanged.stream.listen((name) {
        if (mounted) {
          setState(() {
            _name = name;
          });
        }
      }),
    );
  }

  @override
  void dispose() {
    for (var sub in _subscriptions) {
      sub.cancel();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: () => widget.onTap(widget.source),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        decoration: BoxDecoration(
          color: AppTheme.cardDark,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: widget.selected
                ? AppTheme.secondaryColor
                : Colors.transparent,
            width: 3,
          ),
          boxShadow: widget.selected
              ? [
                  BoxShadow(
                    color: AppTheme.secondaryColor.withOpacity(0.3),
                    blurRadius: 12,
                    spreadRadius: 2,
                  ),
                ]
              : null,
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(9),
          child: Column(
            children: [
              // 缩略图
              Expanded(
                child: Container(
                  width: double.infinity,
                  color: Colors.black,
                  child: _thumbnail != null
                      ? Image.memory(
                          _thumbnail!,
                          fit: BoxFit.contain,
                          gaplessPlayback: true,
                        )
                      : const Center(
                          child: SizedBox(
                            width: 24,
                            height: 24,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: AppTheme.textSecondary,
                            ),
                          ),
                        ),
                ),
              ),
              // 标题
              Container(
                width: double.infinity,
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: widget.selected
                      ? AppTheme.secondaryColor.withOpacity(0.2)
                      : AppTheme.cardDark,
                ),
                child: Row(
                  children: [
                    Icon(
                      widget.source.type == rtc.SourceType.Screen
                          ? Icons.desktop_windows_rounded
                          : Icons.web_asset_rounded,
                      size: 14,
                      color: widget.selected
                          ? AppTheme.secondaryColor
                          : AppTheme.textSecondary,
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        _name,
                        style: TextStyle(
                          color: widget.selected
                              ? AppTheme.textPrimary
                              : AppTheme.textSecondary,
                          fontSize: 12,
                          fontWeight: widget.selected
                              ? FontWeight.w600
                              : FontWeight.normal,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    if (widget.selected)
                      const Icon(
                        Icons.check_circle,
                        color: AppTheme.secondaryColor,
                        size: 16,
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// 屏幕共享选择结果
/// 包含选中的源和需要排除的窗口 ID 列表
class ScreenShareResult {
  final rtc.DesktopCapturerSource source;
  final List<int> excludeWindowIDs;
  final bool isScreen; // true = 屏幕, false = 窗口

  ScreenShareResult({
    required this.source,
    this.excludeWindowIDs = const [],
    this.isScreen = false,
  });
}

/// 自定义屏幕共享选择对话框
class ScreenShareDialog extends StatefulWidget {
  const ScreenShareDialog({super.key});

  @override
  State<ScreenShareDialog> createState() => _ScreenShareDialogState();
}

class _ScreenShareDialogState extends State<ScreenShareDialog>
    with SingleTickerProviderStateMixin {
  final Map<String, rtc.DesktopCapturerSource> _sources = {};
  rtc.SourceType _sourceType = rtc.SourceType.Screen;
  rtc.DesktopCapturerSource? _selectedSource;
  final List<StreamSubscription<rtc.DesktopCapturerSource>> _subscriptions = [];
  Timer? _timer;
  bool _isLoading = true;

  // 原生自排除支持
  bool _nativeExclusionSupported = false;
  List<int> _selfWindowIDs = [];

  late TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _tabController.addListener(_onTabChanged);

    // 检查原生自排除支持并获取自窗口 ID
    _checkNativeSupport();

    // 订阅源添加事件
    _subscriptions.add(
      rtc.desktopCapturer.onAdded.stream.listen((source) {
        _sources[source.id] = source;
        if (mounted) setState(() {});
      }),
    );

    // 订阅源移除事件
    _subscriptions.add(
      rtc.desktopCapturer.onRemoved.stream.listen((source) {
        _sources.remove(source.id);
        if (mounted) setState(() {});
      }),
    );

    // 订阅缩略图更新事件
    _subscriptions.add(
      rtc.desktopCapturer.onThumbnailChanged.stream.listen((source) {
        if (mounted) setState(() {});
      }),
    );

    // 延迟加载源
    Timer(const Duration(milliseconds: 100), _getSources);
  }

  Future<void> _checkNativeSupport() async {
    final supported = await ScreenCaptureChannel.isSupported();
    if (supported) {
      final ids = await ScreenCaptureChannel.getSelfWindowIDs();
      if (mounted) {
        setState(() {
          _nativeExclusionSupported = true;
          _selfWindowIDs = ids;
        });
      }
      debugPrint(
        '[ScreenShare] Native exclusion supported, self window IDs: $ids',
      );
    }
  }

  void _onTabChanged() {
    if (_tabController.indexIsChanging) return;
    _sourceType = _tabController.index == 0
        ? rtc.SourceType.Screen
        : rtc.SourceType.Window;
    // 切换标签时清空选中
    _selectedSource = null;
    _getSources();
  }

  @override
  void dispose() {
    _timer?.cancel();
    _tabController.dispose();
    for (var sub in _subscriptions) {
      sub.cancel();
    }
    super.dispose();
  }

  Future<void> _getSources() async {
    try {
      final sources = await rtc.desktopCapturer.getSources(
        types: [_sourceType],
      );

      // 取消旧定时器
      _timer?.cancel();

      // 定时更新源（刷新缩略图）
      _timer = Timer.periodic(const Duration(seconds: 3), (timer) {
        rtc.desktopCapturer.updateSources(types: [_sourceType]);
      });

      _sources.clear();
      for (var source in sources) {
        _sources[source.id] = source;
      }

      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    } catch (e) {
      debugPrint('[ScreenShare] Error: $e');
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  void _onSelect() {
    _timer?.cancel();
    for (var sub in _subscriptions) {
      sub.cancel();
    }

    if (_selectedSource != null) {
      final isScreen = _selectedSource!.type == rtc.SourceType.Screen;
      final result = ScreenShareResult(
        source: _selectedSource!,
        // 如果是屏幕捕获且支持原生自排除，则传递自窗口 ID 列表
        excludeWindowIDs: isScreen && _nativeExclusionSupported
            ? _selfWindowIDs
            : [],
        isScreen: isScreen,
      );
      Navigator.pop<ScreenShareResult>(context, result);
    } else {
      Navigator.pop<ScreenShareResult>(context, null);
    }
  }

  void _onCancel() {
    _timer?.cancel();
    for (var sub in _subscriptions) {
      sub.cancel();
    }
    Navigator.pop<ScreenShareResult>(context, null);
  }

  List<rtc.DesktopCapturerSource> get _screens =>
      _sources.values.where((s) => s.type == rtc.SourceType.Screen).toList();

  List<rtc.DesktopCapturerSource> get _windows =>
      _sources.values.where((s) => s.type == rtc.SourceType.Window).toList();

  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: Colors.transparent,
      child: Container(
        width: 700,
        height: 550,
        decoration: BoxDecoration(
          color: AppTheme.backgroundDark,
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: AppTheme.cardDark, width: 1),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withOpacity(0.5),
              blurRadius: 30,
              spreadRadius: 5,
            ),
          ],
        ),
        child: Column(
          children: [
            _buildHeader(),
            _buildTabBar(),
            Expanded(child: _buildContent()),
            _buildFooter(),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader() {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        border: Border(bottom: BorderSide(color: AppTheme.cardDark, width: 1)),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: AppTheme.secondaryColor.withOpacity(0.2),
              borderRadius: BorderRadius.circular(12),
            ),
            child: const Icon(
              Icons.screen_share_rounded,
              color: AppTheme.secondaryColor,
              size: 24,
            ),
          ),
          const SizedBox(width: 16),
          const Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '选择共享内容',
                style: TextStyle(
                  color: AppTheme.textPrimary,
                  fontSize: 20,
                  fontWeight: FontWeight.bold,
                ),
              ),
              SizedBox(height: 4),
              Text(
                '选择要共享的屏幕或窗口',
                style: TextStyle(color: AppTheme.textSecondary, fontSize: 14),
              ),
            ],
          ),
          const Spacer(),
          IconButton(
            onPressed: _onCancel,
            icon: const Icon(Icons.close, color: AppTheme.textSecondary),
            style: IconButton.styleFrom(
              backgroundColor: AppTheme.cardDark,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildTabBar() {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
      decoration: BoxDecoration(
        color: AppTheme.cardDark,
        borderRadius: BorderRadius.circular(12),
      ),
      child: TabBar(
        controller: _tabController,
        indicator: BoxDecoration(
          color: AppTheme.secondaryColor,
          borderRadius: BorderRadius.circular(10),
        ),
        indicatorSize: TabBarIndicatorSize.tab,
        labelColor: Colors.white,
        unselectedLabelColor: AppTheme.textSecondary,
        dividerColor: Colors.transparent,
        labelStyle: const TextStyle(fontWeight: FontWeight.w600),
        tabs: [
          Tab(
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Icon(Icons.desktop_windows_rounded, size: 18),
                const SizedBox(width: 8),
                Text('整个屏幕'),
              ],
            ),
          ),
          Tab(
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Icon(Icons.web_asset_rounded, size: 18),
                const SizedBox(width: 8),
                Text('应用窗口'),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildContent() {
    if (_isLoading) {
      return const Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            CircularProgressIndicator(color: AppTheme.secondaryColor),
            SizedBox(height: 16),
            Text(
              '正在获取可用的屏幕和窗口...',
              style: TextStyle(color: AppTheme.textSecondary),
            ),
          ],
        ),
      );
    }

    return TabBarView(
      controller: _tabController,
      children: [_buildGrid(_screens), _buildGrid(_windows)],
    );
  }

  Widget _buildGrid(List<rtc.DesktopCapturerSource> sources) {
    if (sources.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              _tabController.index == 0
                  ? Icons.desktop_access_disabled
                  : Icons.web_asset_off,
              size: 48,
              color: AppTheme.textSecondary,
            ),
            const SizedBox(height: 16),
            Text(
              _tabController.index == 0 ? '没有可用的屏幕' : '没有可用的窗口',
              style: const TextStyle(color: AppTheme.textSecondary),
            ),
          ],
        ),
      );
    }

    return Padding(
      padding: const EdgeInsets.only(left: 20, right: 20, bottom: 10),
      child: GridView.builder(
        padding: const EdgeInsets.only(bottom: 10),
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 3,
          crossAxisSpacing: 16,
          mainAxisSpacing: 16,
          childAspectRatio: 16 / 12,
        ),
        itemCount: sources.length,
        itemBuilder: (context, index) {
          final source = sources[index];
          return _ThumbnailWidget(
            source: source,
            selected: _selectedSource?.id == source.id,
            onTap: (s) => setState(() => _selectedSource = s),
          );
        },
      ),
    );
  }

  Widget _buildFooter() {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: AppTheme.cardDark, width: 1)),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          TextButton(
            onPressed: _onCancel,
            style: TextButton.styleFrom(
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
              ),
            ),
            child: const Text(
              '取消',
              style: TextStyle(color: AppTheme.textSecondary, fontSize: 15),
            ),
          ),
          const SizedBox(width: 12),
          ElevatedButton(
            onPressed: _selectedSource != null ? _onSelect : null,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppTheme.secondaryColor,
              foregroundColor: Colors.white,
              disabledBackgroundColor: AppTheme.cardDark,
              disabledForegroundColor: AppTheme.textSecondary,
              padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 12),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
              ),
              elevation: 0,
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.screen_share_rounded, size: 18),
                SizedBox(width: 8),
                Text(
                  '开始共享',
                  style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
