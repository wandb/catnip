//
//  PTYDataSource.swift
//  catnip
//
//  Protocol abstraction for PTY data sources (real WebSocket or mock replay)
//

import Foundation
import Combine

// Protocol for any source of PTY data
protocol PTYDataSource: AnyObject {
    var isConnected: Published<Bool>.Publisher { get }
    var error: Published<String?>.Publisher { get }
    var onData: ((Data) -> Void)? { get set }
    var onJSONMessage: ((PTYControlMessage) -> Void)? { get set }

    func connect()
    func disconnect()
    func sendInput(_ text: String)
    func sendResize(cols: UInt16, rows: UInt16)
    func sendReady()
}

// Wrapper to make PTYWebSocketManager conform to PTYDataSource
class LivePTYDataSource: PTYDataSource {
    private let webSocketManager: PTYWebSocketManager

    var isConnected: Published<Bool>.Publisher {
        webSocketManager.$isConnected
    }

    var error: Published<String?>.Publisher {
        webSocketManager.$error
    }

    var onData: ((Data) -> Void)? {
        get { webSocketManager.onData }
        set { webSocketManager.onData = newValue }
    }

    var onJSONMessage: ((PTYControlMessage) -> Void)? {
        get { webSocketManager.onJSONMessage }
        set { webSocketManager.onJSONMessage = newValue }
    }

    init(workspaceId: String, agent: String = "claude", baseURL: String, codespaceName: String? = nil, authToken: String? = nil) {
        self.webSocketManager = PTYWebSocketManager(
            workspaceId: workspaceId,
            agent: agent,
            baseURL: baseURL,
            codespaceName: codespaceName,
            authToken: authToken
        )
    }

    func connect() {
        webSocketManager.connect()
    }

    func disconnect() {
        webSocketManager.disconnect()
    }

    func sendInput(_ text: String) {
        webSocketManager.sendInput(text)
    }

    func sendResize(cols: UInt16, rows: UInt16) {
        webSocketManager.sendResize(cols: cols, rows: rows)
    }

    func sendReady() {
        webSocketManager.sendReady()
    }
}
