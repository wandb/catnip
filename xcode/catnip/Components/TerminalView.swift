//
//  TerminalView.swift
//  catnip
//
//  SwiftTerm-based terminal view with WebSocket PTY connection
//

import SwiftUI
import SwiftTerm
import Combine

// Terminal view wrapping SwiftTerm with PTY WebSocket connection
struct TerminalView: View {
    let workspaceId: String
    let baseURL: String
    let codespaceName: String?
    let authToken: String?
    let shouldConnect: Bool  // Only connect when explicitly told to

    @StateObject private var terminalController: TerminalController

    init(workspaceId: String, baseURL: String, codespaceName: String? = nil, authToken: String? = nil, shouldConnect: Bool = true) {
        self.workspaceId = workspaceId
        self.baseURL = baseURL
        self.codespaceName = codespaceName
        self.authToken = authToken
        self.shouldConnect = shouldConnect
        _terminalController = StateObject(wrappedValue: TerminalController(
            workspaceId: workspaceId,
            baseURL: baseURL,
            codespaceName: codespaceName,
            authToken: authToken
        ))
    }

    var body: some View {
        ZStack {
            // Black background that extends to edges
            Color.black
                .ignoresSafeArea()

            VStack(spacing: 0) {
                // Connection status bar (only when not connected)
                if !terminalController.isConnected {
                    HStack {
                        ProgressView()
                            .scaleEffect(0.7)
                        Text(terminalController.error ?? "Connecting to terminal...")
                            .font(.caption)
                            .foregroundColor(.white)
                    }
                    .padding(8)
                    .frame(maxWidth: .infinity)
                    .background(Color.orange.opacity(0.8))
                }

                // Terminal view - respects keyboard to push content up
                // SwiftTerm has built-in accessory view with esc, tab, arrows, etc.
                TerminalViewRepresentable(controller: terminalController)
            }
        }
        .ignoresSafeArea(.container, edges: .top)
        .preferredColorScheme(.dark)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    rotateToPortrait()
                } label: {
                    Image(systemName: "arrow.down.right.and.arrow.up.left")
                        .font(.body)
                }
            }
        }
        .onAppear {
            // Only connect if we're supposed to (prevents premature connection)
            if shouldConnect {
                NSLog("🐱 TerminalView connecting (landscape mode)")
                terminalController.connect()
            } else {
                NSLog("🐱 TerminalView NOT connecting (portrait mode)")
            }
        }
        .onDisappear {
            terminalController.disconnect()
        }
    }

    private func rotateToPortrait() {
        // Request portrait orientation
        if let windowScene = UIApplication.shared.connectedScenes.first as? UIWindowScene {
            windowScene.requestGeometryUpdate(.iOS(interfaceOrientations: .portrait)) { error in
                NSLog("🐱 Failed to rotate to portrait: \(error.localizedDescription)")
            }
        }
    }
}

// UIViewRepresentable wrapper for TerminalView
// SwiftTerm's TerminalView has a built-in inputAccessoryView with special keys
struct TerminalViewRepresentable: UIViewRepresentable {
    let controller: TerminalController

    func makeUIView(context: Context) -> TerminalViewWrapper {
        let terminalView = controller.terminalView
        terminalView.backgroundColor = UIColor.black
        terminalView.font = UIFont.monospacedSystemFont(ofSize: 12, weight: .regular)

        // Disable autocorrection and suggestions that can interfere with terminal
        terminalView.autocorrectionType = .no
        terminalView.autocapitalizationType = .none
        terminalView.spellCheckingType = .no
        terminalView.smartQuotesType = .no
        terminalView.smartDashesType = .no
        terminalView.smartInsertDeleteType = .no

        // Replace the default accessory view with our custom one
        let customAccessory = CustomTerminalAccessory(terminalView: terminalView, controller: controller)
        terminalView.inputAccessoryView = customAccessory

        // Wrap in a container
        let wrapper = TerminalViewWrapper()
        wrapper.setup(with: terminalView, controller: controller)

        return wrapper
    }

    func updateUIView(_ uiView: TerminalViewWrapper, context: Context) {
        // Terminal updates are handled via the controller
    }
}

// Wrapper view that properly manages the terminal and adds Done button to accessory
class TerminalViewWrapper: UIView {
    private var terminalView: SwiftTerm.TerminalView?
    private weak var controller: TerminalController?
    private weak var dismissButton: UIButton?
    private static let dismissButtonTag = 99999

    func setup(with terminalView: SwiftTerm.TerminalView, controller: TerminalController) {
        self.terminalView = terminalView
        self.controller = controller

        backgroundColor = .black

        // Add terminal view
        addSubview(terminalView)
        terminalView.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            terminalView.topAnchor.constraint(equalTo: topAnchor),
            terminalView.bottomAnchor.constraint(equalTo: bottomAnchor),
            terminalView.leadingAnchor.constraint(equalTo: leadingAnchor),
            terminalView.trailingAnchor.constraint(equalTo: trailingAnchor)
        ])
    }
}

// MARK: - Custom Terminal Accessory View

class CustomTerminalAccessory: UIInputView {
    private weak var terminalView: SwiftTerm.TerminalView?
    private weak var controller: TerminalController?

    init(terminalView: SwiftTerm.TerminalView, controller: TerminalController) {
        self.terminalView = terminalView
        self.controller = controller

        // Standard accessory height for iOS
        // Width will be set by auto-layout to match keyboard width
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: 44), inputViewStyle: .keyboard)

        setupUI()
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private func setupUI() {
        backgroundColor = UIColor.systemGray5

        // Create horizontal stack for buttons
        let stackView = UIStackView()
        stackView.axis = .horizontal
        stackView.spacing = 8
        stackView.alignment = .center
        stackView.distribution = .fillProportionally
        stackView.translatesAutoresizingMaskIntoConstraints = false

        addSubview(stackView)

        NSLayoutConstraint.activate([
            stackView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),
            stackView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
            stackView.topAnchor.constraint(equalTo: topAnchor, constant: 4),
            stackView.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -4)
        ])

        // Essential buttons for LLM prompt input
        // Left side: Navigation and special keys
        let leftStack = createButtonStack()
        stackView.addArrangedSubview(leftStack)

        // Essential keys
        leftStack.addArrangedSubview(createButton(title: "esc", action: #selector(escPressed)))
        leftStack.addArrangedSubview(createButton(title: "tab", action: #selector(tabPressed)))
        leftStack.addArrangedSubview(createButton(title: "/↵", action: #selector(newlinePressed)))
        leftStack.addArrangedSubview(createButton(title: "/", action: #selector(slashPressed)))

        // Arrow keys
        let arrowStack = UIStackView()
        arrowStack.axis = .horizontal
        arrowStack.spacing = 4

        arrowStack.addArrangedSubview(createButton(title: "↑", action: #selector(upPressed)))
        arrowStack.addArrangedSubview(createButton(title: "↓", action: #selector(downPressed)))
        arrowStack.addArrangedSubview(createButton(title: "←", action: #selector(leftPressed)))
        arrowStack.addArrangedSubview(createButton(title: "→", action: #selector(rightPressed)))

        leftStack.addArrangedSubview(arrowStack)

        // Spacer to push dismiss button to the right
        let spacer = UIView()
        spacer.setContentHuggingPriority(.defaultLow, for: .horizontal)
        stackView.addArrangedSubview(spacer)

        // Right side: Dismiss button
        let dismissButton = createDismissButton()
        stackView.addArrangedSubview(dismissButton)
    }

    private func createButtonStack() -> UIStackView {
        let stack = UIStackView()
        stack.axis = .horizontal
        stack.spacing = 6
        stack.alignment = .center
        return stack
    }

    private func createButton(title: String, action: Selector) -> UIButton {
        let button = UIButton(type: .system)
        button.setTitle(title, for: .normal)
        button.titleLabel?.font = UIFont.systemFont(ofSize: 14, weight: .medium)
        button.setTitleColor(.label, for: .normal)
        button.backgroundColor = UIColor.systemBackground
        button.layer.cornerRadius = 6
        button.layer.borderWidth = 0.5
        button.layer.borderColor = UIColor.separator.cgColor
        button.addTarget(self, action: action, for: .touchUpInside)

        button.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: 44),
            button.heightAnchor.constraint(equalToConstant: 36)
        ])

        return button
    }

    private func createDismissButton() -> UIButton {
        let button = UIButton(type: .system)
        let config = UIImage.SymbolConfiguration(pointSize: 20, weight: .semibold)
        let icon = UIImage(systemName: "keyboard.chevron.compact.down", withConfiguration: config)
        button.setImage(icon, for: .normal)
        button.tintColor = .label
        button.backgroundColor = UIColor.systemBackground
        button.layer.cornerRadius = 6
        button.layer.borderWidth = 0.5
        button.layer.borderColor = UIColor.separator.cgColor
        button.addTarget(controller, action: #selector(TerminalController.dismissKeyboard), for: .touchUpInside)

        button.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            button.widthAnchor.constraint(equalToConstant: 44),
            button.heightAnchor.constraint(equalToConstant: 36)
        ])

        return button
    }

    // MARK: - Button Actions

    @objc private func escPressed() {
        terminalView?.send(txt: "\u{1B}") // ESC
    }

    @objc private func tabPressed() {
        terminalView?.send(txt: "\t") // TAB
    }

    @objc private func newlinePressed() {
        terminalView?.send(txt: "/\n") // Slash + newline for multi-line prompts
    }

    @objc private func slashPressed() {
        terminalView?.send(txt: "/") // Slash for help/search commands
    }

    @objc private func upPressed() {
        terminalView?.send(txt: "\u{1B}[A") // Up arrow
    }

    @objc private func downPressed() {
        terminalView?.send(txt: "\u{1B}[B") // Down arrow
    }

    @objc private func leftPressed() {
        terminalView?.send(txt: "\u{1B}[D") // Left arrow
    }

    @objc private func rightPressed() {
        terminalView?.send(txt: "\u{1B}[C") // Right arrow
    }
}

// Controller managing SwiftTerm terminal and WebSocket connection
class TerminalController: NSObject, ObservableObject {
    @Published var isConnected = false
    @Published var error: String?

    let terminalView: SwiftTerm.TerminalView
    private let webSocketManager: PTYWebSocketManager

    private var hasSentReady = false

    init(workspaceId: String, baseURL: String, codespaceName: String? = nil, authToken: String? = nil) {
        // Create terminal view
        self.terminalView = SwiftTerm.TerminalView(frame: .zero)

        // Create WebSocket manager with mobile authentication
        self.webSocketManager = PTYWebSocketManager(
            workspaceId: workspaceId,
            agent: "claude",
            baseURL: baseURL,
            codespaceName: codespaceName,
            authToken: authToken
        )

        super.init()

        // Setup terminal
        setupTerminal()

        // Setup WebSocket callbacks
        setupWebSocketCallbacks()
    }

    private func setupTerminal() {
        terminalView.terminalDelegate = self

        // Configure terminal options
        terminalView.optionAsMetaKey = true

        // Hide the iOS system cursor by making caret invisible
        terminalView.caretColor = UIColor.clear
    }

    private func setupWebSocketCallbacks() {
        // Handle binary PTY output
        webSocketManager.onData = { [weak self] data in
            guard let self = self else { return }

            // Feed data to terminal
            let bytes = ArraySlice([UInt8](data))
            self.terminalView.feed(byteArray: bytes)
        }

        // Handle JSON control messages
        webSocketManager.onJSONMessage = { [weak self] message in
            guard self != nil else { return }

            switch message.type {
            case "read-only":
                // Handle read-only status (could show indicator)
                NSLog("🔒 Terminal read-only status: %@", message.data ?? "unknown")

            case "buffer-complete":
                // Buffer replay complete
                NSLog("📋 Buffer replay complete")

            case "buffer-size":
                // Backend telling us what size the buffer was captured at
                if let cols = message.cols, let rows = message.rows {
                    NSLog("📐 Buffer size: %dx%d", cols, rows)
                }

            default:
                NSLog("📨 Received control message: %@", message.type)
            }
        }

        // Monitor connection status
        webSocketManager.$isConnected
            .receive(on: DispatchQueue.main)
            .assign(to: &$isConnected)

        webSocketManager.$error
            .receive(on: DispatchQueue.main)
            .assign(to: &$error)
    }

    func connect() {
        webSocketManager.connect()

        // Wait a bit for connection to establish, then send ready signal
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            guard let self = self, !self.hasSentReady else { return }
            self.hasSentReady = true
            self.sendReadySignal()
        }
    }

    func disconnect() {
        webSocketManager.disconnect()
    }

    private func sendReadySignal() {
        // Get current terminal dimensions
        let cols = UInt16(terminalView.getTerminal().cols)
        let rows = UInt16(terminalView.getTerminal().rows)

        // Send resize to ensure backend knows our dimensions
        webSocketManager.sendResize(cols: cols, rows: rows)

        // Send ready signal to trigger buffer replay
        webSocketManager.sendReady()
    }

    func handleResize() {
        let cols = UInt16(terminalView.getTerminal().cols)
        let rows = UInt16(terminalView.getTerminal().rows)
        webSocketManager.sendResize(cols: cols, rows: rows)
    }

    @objc func dismissKeyboard() {
        _ = terminalView.resignFirstResponder()
    }
}

// MARK: - TerminalViewDelegate

extension TerminalController: TerminalViewDelegate {
    func send(source: SwiftTerm.TerminalView, data: ArraySlice<UInt8>) {
        // User typed input - send to backend
        let string = String(bytes: data, encoding: .utf8) ?? ""
        webSocketManager.sendInput(string)
    }

    func scrolled(source: SwiftTerm.TerminalView, position: Double) {
        // Handle scrolling if needed
    }

    func setTerminalTitle(source: SwiftTerm.TerminalView, title: String) {
        // Terminal title changed (Claude might set this)
        NSLog("📝 Terminal title: %@", title)
    }

    func sizeChanged(source: SwiftTerm.TerminalView, newCols: Int, newRows: Int) {
        // Terminal size changed - notify backend
        handleResize()
    }

    func setTerminalIconTitle(source: SwiftTerm.TerminalView, title: String) {
        // Icon title changed
    }

    func hostCurrentDirectoryUpdate(source: SwiftTerm.TerminalView, directory: String?) {
        // Directory changed
        if let dir = directory {
            NSLog("📁 Directory changed: %@", dir)
        }
    }

    func requestOpenLink(source: SwiftTerm.TerminalView, link: String, params: [String : String]) {
        // Handle link requests (URLs in terminal output)
        NSLog("🔗 Link requested: %@", link)
    }

    func clipboardCopy(source: SwiftTerm.TerminalView, content: Data) {
        // Handle clipboard copy requests
        if let string = String(data: content, encoding: .utf8) {
            UIPasteboard.general.string = string
        }
    }

    func rangeChanged(source: SwiftTerm.TerminalView, startY: Int, endY: Int) {
        // Handle visible range changes
    }
}

#if DEBUG
struct TerminalView_Previews: PreviewProvider {
    static var previews: some View {
        TerminalView(
            workspaceId: "test-workspace",
            baseURL: "ws://localhost:8080"
        )
    }
}
#endif
