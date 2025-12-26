//
//  SampleHandler.swift
//  BroadcastExtension
//
//  Created by LiveKit on 2021/12/26.
//

import ReplayKit
import OSLog

private let broadcastLogger = OSLog(subsystem: "com.example.flutterSfuRelayExample", category: "Broadcast")

private enum Constants {
    // 必须与我们在 Info.plist 和 Capabilities 中配置的 App Group 一致
    static let appGroupIdentifier = "group.com.example.flutterSfuRelayExample"
}

class SampleHandler: RPBroadcastSampleHandler {

    private var clientConnection: SocketConnection?
    private var uploader: SampleUploader?
    private var isConnected = false
    
    override func broadcastStarted(withSetupInfo setupInfo: [String : NSObject]?) {
        
        let socketFilePath = self.socketFilePath()
        os_log(.debug, log: broadcastLogger, "broadcastStarted, socketPath: %@", socketFilePath)
        
        guard FileManager.default.fileExists(atPath: socketFilePath) else {
            os_log(.error, log: broadcastLogger, "socket file not found at: %@", socketFilePath)
            // 尝试等待一会？或者直接报错
            let error = NSError(domain: RPRecordingErrorDomain, code: RPRecordingErrorCode.failedToStart.rawValue, userInfo: [NSLocalizedDescriptionKey: "Socket file not found"])
            finishBroadcastWithError(error)
            return
        }

        if let connection = SocketConnection(filePath: socketFilePath) {
            clientConnection = connection
            uploader = SampleUploader(connection: connection)
            
            connection.didClose = { [weak self] error in
                os_log(.debug, log: broadcastLogger, "socket closed: %@", error?.localizedDescription ?? "nil")
                
                if let error = error {
                    self?.finishBroadcastWithError(error)
                } else {
                    //socket closed normally
                    let code = RPRecordingErrorCode.unknown.rawValue
                    self?.finishBroadcastWithError(NSError(domain: RPRecordingErrorDomain, code: code, userInfo: nil))
                }
            }
            
            connection.didOpen = { [weak self] in
                 os_log(.debug, log: broadcastLogger, "socket connected")
                 self?.isConnected = true
            }
            
            if !connection.open() {
                os_log(.error, log: broadcastLogger, "failed to open socket connection")
                // connection.didClose should be called
            }
        } else {
             os_log(.error, log: broadcastLogger, "failed to init SocketConnection")
        }
    }
    
    override func broadcastPaused() {
        // User has requested to pause the broadcast. Samples will stop being delivered.
    }
    
    override func broadcastResumed() {
        // User has requested to resume the broadcast. Samples delivery will resume.
    }
    
    override func broadcastFinished() {
        // User has requested to finish the broadcast.
        os_log(.debug, log: broadcastLogger, "broadcastFinished")
        clientConnection?.close()
        clientConnection = nil
        uploader = nil
        isConnected = false
    }
    
    override func processSampleBuffer(_ sampleBuffer: CMSampleBuffer, with sampleBufferType: RPSampleBufferType) {
        
        guard isConnected else { return }
        
        switch sampleBufferType {
        case RPSampleBufferType.video:
            // Handle video sample buffer
            if let uploader = uploader {
                uploader.send(sample: sampleBuffer)
            }
        case RPSampleBufferType.audioApp:
            // Handle audio sample buffer for app audio
            break
        case RPSampleBufferType.audioMic:
            // Handle audio sample buffer for mic audio
            break
        @unknown default:
            // Handle other sample buffer types
            fatalError("Unknown type of sample buffer")
        }
    }
    
    private func socketFilePath() -> String {
        guard let sharedContainer = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: Constants.appGroupIdentifier) else {
            os_log(.error, log: broadcastLogger, "App Group Container not found for identifier: %@", Constants.appGroupIdentifier)
            return ""
        }
        
        // flutter_webrtc / LiveKit 约定的 socket 文件名
        return sharedContainer.appendingPathComponent("rtc_AppGroupIdentifier").path
    }
}
