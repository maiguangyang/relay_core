import Cocoa
import FlutterMacOS

/// Screen Share Plugin for Flutter SFU Relay
/// Provides screen capture self-exclusion functionality for macOS
public class ScreenSharePlugin: NSObject, FlutterPlugin {
    
    private static var instance: ScreenSharePlugin?
    private var methodChannel: FlutterMethodChannel?
    private var overlayController: ScreenShareOverlay?
    
    public static func register(with registrar: FlutterPluginRegistrar) {
        let channel = FlutterMethodChannel(
            name: "com.flutter_sfu_relay.screen_share",
            binaryMessenger: registrar.messenger
        )
        
        let instance = ScreenSharePlugin()
        instance.methodChannel = channel
        instance.overlayController = ScreenShareOverlay(channel: channel)
        
        registrar.addMethodCallDelegate(instance, channel: channel)
        ScreenSharePlugin.instance = instance
    }
    
    public func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        switch call.method {
        case "isSupported":
            result(true)
            
        case "setExcludeFromCapture":
            guard let args = call.arguments as? [String: Any],
                  let exclude = args["exclude"] as? Bool else {
                result(FlutterError(code: "INVALID_ARGS", message: "Missing 'exclude' parameter", details: nil))
                return
            }
            setExcludeFromCapture(exclude)
            result(true)
            
        case "showOverlay":
            overlayController?.show()
            result(true)
            
        case "hideOverlay":
            overlayController?.hide()
            result(true)
            
        default:
            result(FlutterMethodNotImplemented)
        }
    }
    
    private func setExcludeFromCapture(_ exclude: Bool) {
        DispatchQueue.main.async {
            for window in NSApplication.shared.windows {
                window.sharingType = exclude ? .none : .readOnly
            }
        }
    }
}

// MARK: - Screen Share Overlay Controller

class ScreenShareOverlay {
    
    private weak var methodChannel: FlutterMethodChannel?
    private var toolbarWindow: NSWindow?
    private var borderWindows: [NSWindow] = []
    
    init(channel: FlutterMethodChannel) {
        self.methodChannel = channel
    }
    
    func show() {
        DispatchQueue.main.async {
            self.createToolbarWindow()
            self.createBorderWindows()
        }
    }
    
    func hide() {
        DispatchQueue.main.async {
            self.toolbarWindow?.orderOut(nil)
            self.toolbarWindow = nil
            
            for window in self.borderWindows {
                window.orderOut(nil)
            }
            self.borderWindows.removeAll()
        }
    }
    
    @objc func stopSharingButtonClicked() {
        hide()
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
            self.methodChannel?.invokeMethod("onStopSharingRequested", arguments: nil)
        }
    }
    
    private func createToolbarWindow() {
        guard let screen = NSScreen.main else { return }
        
        let toolbarWidth: CGFloat = 200
        let toolbarHeight: CGFloat = 36
        let screenFrame = screen.frame
        let x = screenFrame.midX - toolbarWidth / 2
        let y = screenFrame.maxY - toolbarHeight - 45
        
        let frame = NSRect(x: x, y: y, width: toolbarWidth, height: toolbarHeight)
        
        let window = NSWindow(
            contentRect: frame,
            styleMask: [.borderless],
            backing: .buffered,
            defer: false
        )
        
        window.level = .floating
        window.isOpaque = false
        window.backgroundColor = .clear
        window.hasShadow = true
        window.isMovableByWindowBackground = true
        window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        window.sharingType = .none
        
        let contentView = ToolbarContentView(
            frame: NSRect(x: 0, y: 0, width: toolbarWidth, height: toolbarHeight),
            target: self,
            action: #selector(stopSharingButtonClicked)
        )
        window.contentView = contentView
        
        window.orderFront(nil)
        self.toolbarWindow = window
    }
    
    private func createBorderWindows() {
        guard let screen = NSScreen.main else { return }
        
        let screenFrame = screen.frame
        let borderThickness: CGFloat = 4
        let cornerLength: CGFloat = 60
        
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
            
            let borderView = CornerBorderView(
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

// MARK: - Toolbar Content View

class ToolbarContentView: NSView {
    
    init(frame frameRect: NSRect, target: AnyObject, action: Selector) {
        super.init(frame: frameRect)
        setupUI(target: target, action: action)
    }
    
    required init?(coder: NSCoder) { fatalError() }
    
    override func draw(_ dirtyRect: NSRect) {
        let path = NSBezierPath(roundedRect: bounds.insetBy(dx: 1, dy: 1), xRadius: 8, yRadius: 8)
        NSColor(red: 0.15, green: 0.15, blue: 0.15, alpha: 0.95).setFill()
        path.fill()
    }
    
    private func setupUI(target: AnyObject, action: Selector) {
        // Green dot
        let dot = NSView(frame: NSRect(x: 12, y: 12, width: 10, height: 10))
        dot.wantsLayer = true
        dot.layer?.backgroundColor = NSColor(red: 0.2, green: 0.85, blue: 0.4, alpha: 1.0).cgColor
        dot.layer?.cornerRadius = 5
        addSubview(dot)
        
        // Status text
        let label = NSTextField(frame: NSRect(x: 28, y: 9, width: 85, height: 18))
        label.stringValue = "正在共享屏幕"
        label.isBezeled = false
        label.isEditable = false
        label.isSelectable = false
        label.backgroundColor = .clear
        label.textColor = .white
        label.font = NSFont.systemFont(ofSize: 12)
        addSubview(label)
        
        // Stop button
        let button = NSButton(frame: NSRect(x: 118, y: 6, width: 72, height: 24))
        button.title = "结束共享"
        button.bezelStyle = .rounded
        button.wantsLayer = true
        button.isBordered = false
        button.layer?.backgroundColor = NSColor(red: 0.9, green: 0.25, blue: 0.3, alpha: 1.0).cgColor
        button.layer?.cornerRadius = 4
        button.font = NSFont.systemFont(ofSize: 12, weight: .medium)
        button.contentTintColor = .white
        button.target = target
        button.action = action
        addSubview(button)
    }
}

// MARK: - Corner Border View

class CornerBorderView: NSView {
    
    private let isTop: Bool
    private let isLeft: Bool
    private let thickness: CGFloat
    
    init(frame frameRect: NSRect, isTop: Bool, isLeft: Bool, thickness: CGFloat) {
        self.isTop = isTop
        self.isLeft = isLeft
        self.thickness = thickness
        super.init(frame: frameRect)
    }
    
    required init?(coder: NSCoder) { fatalError() }
    
    override func draw(_ dirtyRect: NSRect) {
        let green = NSColor(red: 0.15, green: 0.85, blue: 0.35, alpha: 1.0)
        green.setStroke()
        
        let path = NSBezierPath()
        path.lineWidth = thickness
        path.lineCapStyle = .square
        
        let w = bounds.width, h = bounds.height, t = thickness / 2
        
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
