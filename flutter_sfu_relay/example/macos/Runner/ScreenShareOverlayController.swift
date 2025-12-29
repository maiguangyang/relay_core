import Cocoa
import FlutterMacOS

/// 屏幕共享覆盖层控制器
/// 实现类似腾讯会议的浮动控制栏和绿色边框效果
class ScreenShareOverlayController: NSObject {
    
    static let shared = ScreenShareOverlayController()
    
    // 浮动控制栏窗口
    private var toolbarWindow: NSWindow?
    
    // 绿色边框窗口（四个角）
    private var borderWindows: [NSWindow] = []
    
    // 主窗口引用（用于最小化/恢复）
    private weak var mainWindow: NSWindow?
    
    // 方法通道引用
    private var methodChannel: FlutterMethodChannel?
    
    private override init() {
        super.init()
    }
    
    // MARK: - Setup
    
    func setup(mainWindow: NSWindow, channel: FlutterMethodChannel) {
        self.mainWindow = mainWindow
        self.methodChannel = channel
    }
    
    // MARK: - Public Methods
    
    /// 显示屏幕共享 UI（包括最小化主窗口）
    func showScreenShareUI() {
        DispatchQueue.main.async {
            // 最小化主窗口
            self.mainWindow?.miniaturize(nil)
            
            // 延迟显示覆盖层，等待窗口最小化动画完成
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                self.createToolbarWindow()
                self.createBorderWindows()
            }
        }
    }
    
    /// 隐藏屏幕共享 UI（包括恢复主窗口）
    func hideScreenShareUI() {
        DispatchQueue.main.async {
            // 隐藏覆盖层
            self.toolbarWindow?.orderOut(nil)
            self.toolbarWindow = nil
            
            for window in self.borderWindows {
                window.orderOut(nil)
            }
            self.borderWindows.removeAll()
            
            // 恢复主窗口
            self.mainWindow?.deminiaturize(nil)
            self.mainWindow?.makeKeyAndOrderFront(nil)
        }
    }
    
    // MARK: - Stop Sharing Callback
    
    @objc func stopSharingButtonClicked() {
        print("[ScreenShare] Stop sharing button clicked - sending to Flutter")
        
        // 先隐藏 UI
        hideScreenShareUI()
        
        // 然后通知 Flutter
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
            self.methodChannel?.invokeMethod("onStopSharingRequested", arguments: nil)
        }
    }
    
    // MARK: - Floating Toolbar
    
    private func createToolbarWindow() {
        guard let screen = NSScreen.main else { return }
        
        // 工具栏尺寸
        let toolbarWidth: CGFloat = 200
        let toolbarHeight: CGFloat = 36
        
        // 计算位置（屏幕顶部居中）
        let screenFrame = screen.frame
        let x = screenFrame.midX - toolbarWidth / 2
        let y = screenFrame.maxY - toolbarHeight - 45
        
        let frame = NSRect(x: x, y: y, width: toolbarWidth, height: toolbarHeight)
        
        // 创建窗口
        let window = NSWindow(
            contentRect: frame,
            styleMask: [.borderless],
            backing: .buffered,
            defer: false
        )
        
        // 窗口配置
        window.level = .floating
        window.isOpaque = false
        window.backgroundColor = .clear
        window.hasShadow = true
        window.isMovableByWindowBackground = true
        window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        
        // 🔑 设置不会被屏幕捕获
        window.sharingType = .none
        
        // 创建内容视图
        let contentView = TencentStyleToolbar(
            frame: NSRect(x: 0, y: 0, width: toolbarWidth, height: toolbarHeight),
            controller: self
        )
        window.contentView = contentView
        
        window.orderFront(nil)
        self.toolbarWindow = window
    }
    
    // MARK: - Green Corner Borders
    
    private func createBorderWindows() {
        guard let screen = NSScreen.main else { return }
        
        let screenFrame = screen.frame
        let borderThickness: CGFloat = 4
        let cornerLength: CGFloat = 60
        
        // 定义四个角的位置
        let corners: [(x: CGFloat, y: CGFloat, isTop: Bool, isLeft: Bool)] = [
            (screenFrame.minX, screenFrame.maxY - cornerLength, true, true),
            (screenFrame.maxX - cornerLength, screenFrame.maxY - cornerLength, true, false),
            (screenFrame.minX, screenFrame.minY, false, true),
            (screenFrame.maxX - cornerLength, screenFrame.minY, false, false)
        ]
        
        for (x, y, isTop, isLeft) in corners {
            let frame = NSRect(x: x, y: y, width: cornerLength, height: cornerLength)
            
            let window = NSWindow(
                contentRect: frame,
                styleMask: [.borderless],
                backing: .buffered,
                defer: false
            )
            
            window.level = .screenSaver
            window.isOpaque = false
            window.backgroundColor = .clear
            window.hasShadow = false
            window.ignoresMouseEvents = true
            window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
            window.sharingType = .none
            
            let borderView = GreenCornerView(
                frame: NSRect(x: 0, y: 0, width: cornerLength, height: cornerLength),
                isTop: isTop,
                isLeft: isLeft,
                thickness: borderThickness
            )
            window.contentView = borderView
            
            window.orderFront(nil)
            borderWindows.append(window)
        }
    }
}

// MARK: - Tencent Style Toolbar (仿腾讯会议风格)

class TencentStyleToolbar: NSView {
    
    private weak var controller: ScreenShareOverlayController?
    
    init(frame frameRect: NSRect, controller: ScreenShareOverlayController) {
        self.controller = controller
        super.init(frame: frameRect)
        wantsLayer = true
        setupUI()
    }
    
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }
    
    override func draw(_ dirtyRect: NSRect) {
        // 深灰色圆角背景 (类似腾讯会议)
        let path = NSBezierPath(roundedRect: bounds.insetBy(dx: 1, dy: 1), xRadius: 8, yRadius: 8)
        NSColor(red: 0.15, green: 0.15, blue: 0.15, alpha: 0.95).setFill()
        path.fill()
    }
    
    private func setupUI() {
        // 1. 绿色圆点指示器
        let dot = NSView(frame: NSRect(x: 12, y: 12, width: 10, height: 10))
        dot.wantsLayer = true
        dot.layer?.backgroundColor = NSColor(red: 0.2, green: 0.85, blue: 0.4, alpha: 1.0).cgColor
        dot.layer?.cornerRadius = 5
        addSubview(dot)
        
        // 2. 状态文本
        let label = NSTextField(frame: NSRect(x: 28, y: 9, width: 85, height: 18))
        label.stringValue = "正在共享屏幕"
        label.isBezeled = false
        label.isEditable = false
        label.isSelectable = false
        label.backgroundColor = .clear
        label.textColor = NSColor.white
        label.font = NSFont.systemFont(ofSize: 12, weight: .regular)
        addSubview(label)
        
        // 3. 结束共享按钮 (使用 NSButton)
        let button = NSButton(frame: NSRect(x: 118, y: 6, width: 72, height: 24))
        button.title = "结束共享"
        button.bezelStyle = .rounded
        button.wantsLayer = true
        button.isBordered = false
        button.layer?.backgroundColor = NSColor(red: 0.9, green: 0.25, blue: 0.3, alpha: 1.0).cgColor
        button.layer?.cornerRadius = 4
        button.font = NSFont.systemFont(ofSize: 12, weight: .medium)
        button.contentTintColor = .white
        button.target = controller
        button.action = #selector(ScreenShareOverlayController.stopSharingButtonClicked)
        addSubview(button)
    }
}

// MARK: - Green Corner View

class GreenCornerView: NSView {
    
    private let isTop: Bool
    private let isLeft: Bool
    private let thickness: CGFloat
    
    init(frame frameRect: NSRect, isTop: Bool, isLeft: Bool, thickness: CGFloat) {
        self.isTop = isTop
        self.isLeft = isLeft
        self.thickness = thickness
        super.init(frame: frameRect)
    }
    
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }
    
    override func draw(_ dirtyRect: NSRect) {
        let green = NSColor(red: 0.15, green: 0.85, blue: 0.35, alpha: 1.0)
        green.setStroke()
        
        let path = NSBezierPath()
        path.lineWidth = thickness
        path.lineCapStyle = .square
        
        let w = bounds.width
        let h = bounds.height
        let t = thickness / 2
        
        if isTop && isLeft {
            path.move(to: NSPoint(x: t, y: 0))
            path.line(to: NSPoint(x: t, y: h - t))
            path.line(to: NSPoint(x: w, y: h - t))
        } else if isTop && !isLeft {
            path.move(to: NSPoint(x: 0, y: h - t))
            path.line(to: NSPoint(x: w - t, y: h - t))
            path.line(to: NSPoint(x: w - t, y: 0))
        } else if !isTop && isLeft {
            path.move(to: NSPoint(x: t, y: h))
            path.line(to: NSPoint(x: t, y: t))
            path.line(to: NSPoint(x: w, y: t))
        } else {
            path.move(to: NSPoint(x: 0, y: t))
            path.line(to: NSPoint(x: w - t, y: t))
            path.line(to: NSPoint(x: w - t, y: h))
        }
        
        path.stroke()
    }
}
