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
    let showExitButton: Bool  // Show the exit/rotate button in toolbar
    let showDismissButton: Bool  // Show dismiss keyboard button in accessory

    @StateObject private var terminalController: TerminalController

    // Codespace shutdown detection
    @State private var showShutdownAlert = false
    @State private var shutdownMessage: String?

    init(workspaceId: String, baseURL: String, codespaceName: String? = nil, authToken: String? = nil, shouldConnect: Bool = true, showExitButton: Bool = true, showDismissButton: Bool = true) {
        self.workspaceId = workspaceId
        self.baseURL = baseURL
        self.codespaceName = codespaceName
        self.authToken = authToken
        self.shouldConnect = shouldConnect
        self.showExitButton = showExitButton
        self.showDismissButton = showDismissButton
        _terminalController = StateObject(wrappedValue: TerminalController(
            workspaceId: workspaceId,
            baseURL: baseURL,
            codespaceName: codespaceName,
            authToken: authToken,
            showDismissButton: showDismissButton
        ))
    }

    var body: some View {
        ZStack {
            // Black background that extends to edges
            Color.black
                .ignoresSafeArea()

            // Terminal view - respects bottom safe area, ignores top for nav bar
            TerminalViewRepresentable(controller: terminalController)
                .ignoresSafeArea(.container, edges: .top)
        }
        .ignoresSafeArea(.container, edges: .top)
        .preferredColorScheme(.dark)
        .toolbar {
            if showExitButton {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        rotateToPortrait()
                    } label: {
                        Image(systemName: "arrow.down.right.and.arrow.up.left")
                            .font(.body)
                    }
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
        .onReceive(NotificationCenter.default.publisher(for: .codespaceShutdownDetected)) { notification in
            // Handle codespace shutdown notification
            if let message = notification.userInfo?["message"] as? String {
                shutdownMessage = message
                showShutdownAlert = true
            }
        }
        .alert("Codespace Unavailable", isPresented: $showShutdownAlert) {
            Button("Reconnect") {
                // Reset shutdown state and reconnect
                HealthCheckService.shared.resetShutdownState()
                terminalController.reconnect()
            }
            Button("Cancel", role: .cancel) {
                // Just dismiss the alert
            }
        } message: {
            Text(shutdownMessage ?? "Your codespace has shut down. Tap 'Reconnect' to restart it.")
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

        // Remove default accessory - we use a floating toolbar instead
        terminalView.inputAccessoryView = nil

        // Wrap in a container with floating glass toolbar
        let wrapper = TerminalViewWrapper()
        wrapper.setup(with: terminalView, controller: controller)

        return wrapper
    }

    func updateUIView(_ uiView: TerminalViewWrapper, context: Context) {
        // Terminal updates are handled via the controller
    }
}

// Wrapper view that properly manages the terminal and floating glass toolbar
class TerminalViewWrapper: UIView {
    private var terminalView: SwiftTerm.TerminalView?
    private weak var controller: TerminalController?
    private var refreshControl: UIRefreshControl?

    // Floating glass toolbar
    private var floatingToolbar: GlassTerminalAccessory?
    private var keyboardHeight: CGFloat = 0
    private var keyboardObservers: [NSObjectProtocol] = []

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

        // Add bottom content inset so terminal content extends behind the glass toolbar
        terminalView.contentInset.bottom = 56

        // Create floating glass toolbar that overlays the terminal
        // We'll position it manually in layoutSubviews to handle the wider terminal view
        let toolbar = GlassTerminalAccessory(
            terminalView: terminalView,
            controller: controller,
            showDismissButton: controller.showDismissButton
        )
        floatingToolbar = toolbar
        controller.accessoryView = toolbar

        // Observe keyboard to position toolbar above it
        setupKeyboardObservers()

        // Add pull-to-refresh for reconnecting WebSocket
        setupRefreshControl(for: terminalView, controller: controller)
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()

        // Add toolbar to window so it's positioned relative to screen, not the scrollable terminal
        if let window = window, let toolbar = floatingToolbar, toolbar.superview == nil {
            window.addSubview(toolbar)
            positionToolbar()
        }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        positionToolbar()
    }

    private func positionToolbar() {
        guard let toolbar = floatingToolbar, let window = window else { return }

        let screenWidth = window.bounds.width
        let toolbarHeight: CGFloat = 56
        let safeAreaBottom = window.safeAreaInsets.bottom

        // Position toolbar at bottom of screen, above keyboard if visible
        let bottomOffset = keyboardHeight > 0 ? keyboardHeight : safeAreaBottom
        let toolbarY = window.bounds.height - bottomOffset - toolbarHeight

        toolbar.frame = CGRect(
            x: 0,
            y: toolbarY,
            width: screenWidth,
            height: toolbarHeight
        )
    }

    override func removeFromSuperview() {
        // Clean up toolbar from window when we're removed
        floatingToolbar?.removeFromSuperview()
        super.removeFromSuperview()
    }

    private func setupKeyboardObservers() {
        let showObserver = NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillShowNotification,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            self?.handleKeyboardWillShow(notification)
        }

        let hideObserver = NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillHideNotification,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            self?.handleKeyboardWillHide(notification)
        }

        let changeObserver = NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillChangeFrameNotification,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            self?.handleKeyboardWillChangeFrame(notification)
        }

        keyboardObservers = [showObserver, hideObserver, changeObserver]
    }

    private func handleKeyboardWillShow(_ notification: Notification) {
        guard let keyboardFrame = notification.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect,
              let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt,
              let window = window else {
            return
        }

        // Convert keyboard frame to window coordinates for proper landscape handling
        let keyboardFrameInWindow = window.convert(keyboardFrame, from: nil)
        keyboardHeight = max(0, window.bounds.height - keyboardFrameInWindow.origin.y)

        UIView.animate(
            withDuration: duration,
            delay: 0,
            options: UIView.AnimationOptions(rawValue: curve << 16),
            animations: {
                self.positionToolbar()
                // Show toolbar when keyboard appears
                self.floatingToolbar?.alpha = 1
            }
        )
    }

    private func handleKeyboardWillHide(_ notification: Notification) {
        guard let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt else {
            return
        }

        keyboardHeight = 0

        // Check if we're in landscape mode
        let isLandscape = UIDevice.current.orientation.isLandscape ||
            (window?.bounds.width ?? 0) > (window?.bounds.height ?? 0)

        UIView.animate(
            withDuration: duration,
            delay: 0,
            options: UIView.AnimationOptions(rawValue: curve << 16),
            animations: {
                self.positionToolbar()
                // Hide toolbar in landscape when keyboard is dismissed
                if isLandscape {
                    self.floatingToolbar?.alpha = 0
                }
            }
        )
    }

    private func handleKeyboardWillChangeFrame(_ notification: Notification) {
        guard let keyboardFrame = notification.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect,
              let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt,
              let window = window else {
            return
        }

        // Convert keyboard frame to window coordinates for proper landscape handling
        let keyboardFrameInWindow = window.convert(keyboardFrame, from: nil)
        keyboardHeight = max(0, window.bounds.height - keyboardFrameInWindow.origin.y)

        UIView.animate(
            withDuration: duration,
            delay: 0,
            options: UIView.AnimationOptions(rawValue: curve << 16),
            animations: {
                self.positionToolbar()
            }
        )
    }

    deinit {
        keyboardObservers.forEach { NotificationCenter.default.removeObserver($0) }
    }

    private func setupRefreshControl(for terminalView: SwiftTerm.TerminalView, controller: TerminalController) {
        let refresh = UIRefreshControl()
        refresh.tintColor = .white
        refresh.attributedTitle = NSAttributedString(
            string: "Reconnecting...",
            attributes: [.foregroundColor: UIColor.white]
        )
        refresh.addTarget(self, action: #selector(handleRefresh), for: .valueChanged)

        // SwiftTerm's TerminalView is a UIScrollView subclass
        terminalView.refreshControl = refresh
        self.refreshControl = refresh
    }

    @objc private func handleRefresh() {
        guard let controller = controller else {
            refreshControl?.endRefreshing()
            return
        }

        NSLog("🔄 Pull-to-refresh triggered - reconnecting WebSocket")

        // Disconnect first
        controller.disconnect()

        // Wait longer to ensure WebSocket fully closes, then reconnect
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self, weak controller] in
            guard let controller = controller else {
                self?.refreshControl?.endRefreshing()
                return
            }

            controller.connect()

            // End refreshing after connection starts
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                self?.refreshControl?.endRefreshing()
            }
        }
    }
}

// MARK: - Glass Terminal Accessory View (iOS 26+ Liquid Glass Design)

class GlassTerminalAccessory: UIInputView {
    private weak var terminalView: SwiftTerm.TerminalView?
    private weak var controller: TerminalController?
    private let showDismissButton: Bool

    // Mode toggle state (regular vs plan)
    private var isPlanMode = false
    private var modeButton: UIButton?

    // Ctrl toggle state
    private var isCtrlActive = false
    private var ctrlButton: UIButton?

    // Help toggle state
    private var isHelpActive = false
    private var helpButton: UIButton?

    // Bash mode toggle state
    private var isBashMode = false
    private var bashButton: UIButton?

    // Glass effect views
    private var glassContainer: UIVisualEffectView?
    private var buttonStackView: UIStackView?
    private var statusLabel: UILabel?
    private var statusSpinner: UIActivityIndicatorView?
    private var buttonsContainer: UIView?
    private var buttonScrollView: UIScrollView?

    // Status message state
    private var isShowingStatus = false

    init(terminalView: SwiftTerm.TerminalView, controller: TerminalController, showDismissButton: Bool = true) {
        self.terminalView = terminalView
        self.controller = controller
        self.showDismissButton = showDismissButton

        // Height for floating toolbar - 56pt for comfortable touch targets
        // Use .default style for transparent background that shows the glass effect
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: 56), inputViewStyle: .default)

        // Allow the view to extend across the full width
        allowsSelfSizing = true
        setupUI()
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private func setupUI() {
        // Fully transparent background to show content behind the glass
        backgroundColor = .clear
        isOpaque = false

        // Create the floating glass container
        let glassView = createGlassContainer()
        glassContainer = glassView
        addSubview(glassView)

        glassView.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            glassView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 16),
            glassView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -16),
            glassView.topAnchor.constraint(equalTo: topAnchor, constant: 8),
            glassView.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -8)
        ])

        // Create buttons container (for hiding during status)
        let buttonsView = UIView()
        buttonsView.translatesAutoresizingMaskIntoConstraints = false
        buttonsContainer = buttonsView
        glassView.contentView.addSubview(buttonsView)

        NSLayoutConstraint.activate([
            buttonsView.leadingAnchor.constraint(equalTo: glassView.contentView.leadingAnchor),
            buttonsView.trailingAnchor.constraint(equalTo: glassView.contentView.trailingAnchor),
            buttonsView.topAnchor.constraint(equalTo: glassView.contentView.topAnchor),
            buttonsView.bottomAnchor.constraint(equalTo: glassView.contentView.bottomAnchor)
        ])

        // Create scroll view for buttons
        let scrollView = UIScrollView()
        scrollView.showsHorizontalScrollIndicator = false
        scrollView.showsVerticalScrollIndicator = false
        scrollView.translatesAutoresizingMaskIntoConstraints = false
        buttonScrollView = scrollView
        buttonsView.addSubview(scrollView)

        NSLayoutConstraint.activate([
            scrollView.leadingAnchor.constraint(equalTo: buttonsView.leadingAnchor, constant: 8),
            scrollView.trailingAnchor.constraint(equalTo: buttonsView.trailingAnchor, constant: -8),
            scrollView.topAnchor.constraint(equalTo: buttonsView.topAnchor),
            scrollView.bottomAnchor.constraint(equalTo: buttonsView.bottomAnchor)
        ])

        // Create horizontal stack for buttons
        let stackView = UIStackView()
        stackView.axis = .horizontal
        stackView.spacing = 8
        stackView.alignment = .center
        stackView.translatesAutoresizingMaskIntoConstraints = false
        buttonStackView = stackView

        scrollView.addSubview(stackView)

        NSLayoutConstraint.activate([
            stackView.leadingAnchor.constraint(equalTo: scrollView.contentLayoutGuide.leadingAnchor),
            stackView.trailingAnchor.constraint(equalTo: scrollView.contentLayoutGuide.trailingAnchor),
            stackView.topAnchor.constraint(equalTo: scrollView.contentLayoutGuide.topAnchor),
            stackView.bottomAnchor.constraint(equalTo: scrollView.contentLayoutGuide.bottomAnchor),
            stackView.heightAnchor.constraint(equalTo: scrollView.frameLayoutGuide.heightAnchor)
        ])

        // Add buttons
        addToolbarButtons(to: stackView)

        // Create status view (hidden by default)
        setupStatusView(in: glassView.contentView)
    }

    private func createGlassContainer() -> UIVisualEffectView {
        let effectView = UIVisualEffectView()
        effectView.clipsToBounds = true

        // Use compile-time check for iOS 26+ SDK availability
        // This allows compilation with older Xcode versions in CI
        #if compiler(>=6.0)
        // Apply glass effect with animation for iOS 26+
        if #available(iOS 26.0, *) {
            // Create glass effect with regular style for the liquid glass appearance
            let glassEffect = UIGlassEffect(style: .regular)
            glassEffect.isInteractive = true

            // Set capsule corner configuration for pill shape
            effectView.cornerConfiguration = .capsule()

            // Apply effect with animation to trigger materialize
            UIView.animate(withDuration: 0.4) {
                effectView.effect = glassEffect
            }
        } else {
            // Fallback for older iOS - use blur effect with rounded corners
            effectView.effect = UIBlurEffect(style: .systemUltraThinMaterial)
            effectView.layer.cornerRadius = 22
        }
        #else
        // Fallback for older SDK - use blur effect with rounded corners
        effectView.effect = UIBlurEffect(style: .systemUltraThinMaterial)
        effectView.layer.cornerRadius = 22
        #endif

        return effectView
    }

    private func setupStatusView(in container: UIView) {
        // Status container (centered, hidden by default)
        let statusContainer = UIStackView()
        statusContainer.axis = .horizontal
        statusContainer.spacing = 8
        statusContainer.alignment = .center
        statusContainer.translatesAutoresizingMaskIntoConstraints = false
        statusContainer.isHidden = true
        statusContainer.tag = 1001 // Tag for finding later

        // Spinner
        let spinner = UIActivityIndicatorView(style: .medium)
        spinner.color = .label
        spinner.startAnimating()
        statusSpinner = spinner

        // Label
        let label = UILabel()
        label.font = .systemFont(ofSize: 14, weight: .medium)
        label.textColor = .label
        label.text = "Connecting..."
        statusLabel = label

        // Accessibility
        label.accessibilityTraits = .updatesFrequently

        statusContainer.addArrangedSubview(spinner)
        statusContainer.addArrangedSubview(label)

        container.addSubview(statusContainer)

        NSLayoutConstraint.activate([
            statusContainer.centerXAnchor.constraint(equalTo: container.centerXAnchor),
            statusContainer.centerYAnchor.constraint(equalTo: container.centerYAnchor)
        ])
    }

    private func updateButtonAlignment() {
        guard let scrollView = buttonScrollView else { return }

        // Center buttons by adding equal padding on both sides
        // Use contentSize which is set after layout completes
        let contentWidth = scrollView.contentSize.width
        let scrollViewWidth = scrollView.bounds.width

        if contentWidth > 0 && contentWidth < scrollViewWidth {
            let padding = (scrollViewWidth - contentWidth) / 2
            scrollView.contentInset = UIEdgeInsets(top: 0, left: padding, bottom: 0, right: padding)
        } else {
            // Content fills or exceeds scroll view, no centering needed
            scrollView.contentInset = .zero
        }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        // Defer alignment update to after layout pass completes
        // This ensures contentSize and bounds are accurate
        DispatchQueue.main.async {
            self.updateButtonAlignment()
        }
    }

    private func addToolbarButtons(to stackView: UIStackView) {
        // Mode toggle (code/plan)
        let modeBtn = createGlassButton(title: "code", action: #selector(modePressed), accessibilityHint: "Toggle between code and plan mode")
        modeButton = modeBtn
        stackView.addArrangedSubview(modeBtn)

        // Ctrl toggle
        let ctrlBtn = createGlassButton(title: "ctrl", action: #selector(ctrlPressed), accessibilityHint: "Toggle control key modifier")
        ctrlButton = ctrlBtn
        stackView.addArrangedSubview(ctrlBtn)

        // Essential keys
        stackView.addArrangedSubview(createGlassButton(title: "esc", action: #selector(escPressed), accessibilityHint: "Send escape key"))
        stackView.addArrangedSubview(createGlassButton(title: "/", action: #selector(slashPressed), accessibilityHint: "Send slash for commands"))

        let bashBtn = createGlassButton(title: "!", action: #selector(bangPressed), accessibilityHint: "Toggle bash mode")
        bashButton = bashBtn
        stackView.addArrangedSubview(bashBtn)

        stackView.addArrangedSubview(createGlassButton(title: "\\n", action: #selector(newlinePressed), accessibilityLabel: "Newline", accessibilityHint: "Send newline character"))
        stackView.addArrangedSubview(createGlassButton(title: "tab", action: #selector(tabPressed), accessibilityHint: "Send tab key for autocomplete"))

        // Arrow keys with SF Symbols
        stackView.addArrangedSubview(createGlassButton(systemImage: "arrow.up", action: #selector(upPressed), accessibilityLabel: "Up arrow", accessibilityHint: "Navigate up or previous command"))
        stackView.addArrangedSubview(createGlassButton(systemImage: "arrow.down", action: #selector(downPressed), accessibilityLabel: "Down arrow", accessibilityHint: "Navigate down or next command"))
        stackView.addArrangedSubview(createGlassButton(systemImage: "arrow.left", action: #selector(leftPressed), accessibilityLabel: "Left arrow", accessibilityHint: "Move cursor left"))
        stackView.addArrangedSubview(createGlassButton(systemImage: "arrow.right", action: #selector(rightPressed), accessibilityLabel: "Right arrow", accessibilityHint: "Move cursor right"))

        // Help toggle
        let helpBtn = createGlassButton(systemImage: "questionmark", action: #selector(helpPressed), accessibilityLabel: "Help", accessibilityHint: "Show or hide help menu")
        helpButton = helpBtn
        stackView.addArrangedSubview(helpBtn)

        // Dismiss button (only if enabled)
        if showDismissButton {
            let dismissButton = createGlassButton(systemImage: "keyboard.chevron.compact.down", action: #selector(dismissPressed), accessibilityLabel: "Dismiss keyboard", accessibilityHint: "Hide the keyboard")
            stackView.addArrangedSubview(dismissButton)
        }
    }

    private func createGlassButton(title: String? = nil, systemImage: String? = nil, action: Selector, accessibilityLabel: String? = nil, accessibilityHint: String? = nil) -> UIButton {
        let button = UIButton(type: .system)

        if let imageName = systemImage {
            let config = UIImage.SymbolConfiguration(pointSize: 16, weight: .medium)
            let image = UIImage(systemName: imageName, withConfiguration: config)
            button.setImage(image, for: .normal)
        }

        if let title = title {
            button.setTitle(title, for: .normal)
            button.titleLabel?.font = .systemFont(ofSize: 14, weight: .medium)
        }

        button.tintColor = .label
        button.setTitleColor(.label, for: .normal)

        // No background - buttons are inside the glass container
        button.backgroundColor = .clear

        button.addTarget(self, action: action, for: .touchUpInside)

        // Accessibility
        button.accessibilityLabel = accessibilityLabel ?? title
        button.accessibilityHint = accessibilityHint
        button.isAccessibilityElement = true

        button.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: 40),
            button.heightAnchor.constraint(equalToConstant: 36)
        ])

        return button
    }

    // MARK: - Status Message Display

    func showStatus(message: String) {
        guard !isShowingStatus else {
            statusLabel?.text = message
            return
        }

        isShowingStatus = true
        statusLabel?.text = message

        // Find status container
        guard let statusContainer = glassContainer?.contentView.viewWithTag(1001) else { return }

        UIView.animate(withDuration: 0.25) {
            self.buttonsContainer?.alpha = 0
            statusContainer.isHidden = false
            statusContainer.alpha = 1
        }

        // Announce status change for accessibility
        UIAccessibility.post(notification: .announcement, argument: message)
    }

    func hideStatus() {
        guard isShowingStatus else { return }

        isShowingStatus = false

        // Find status container
        guard let statusContainer = glassContainer?.contentView.viewWithTag(1001) else { return }

        UIView.animate(withDuration: 0.25) {
            self.buttonsContainer?.alpha = 1
            statusContainer.alpha = 0
        } completion: { _ in
            statusContainer.isHidden = true
        }
    }

    // MARK: - Button Actions

    @objc private func dismissPressed() {
        controller?.dismissKeyboard()
    }

    @objc private func modePressed() {
        // Shift+Tab = ESC [ Z
        let shiftTab = "\u{1B}[Z"

        if isPlanMode {
            // Switch back to code mode - send shift+tab once
            terminalView?.send(txt: shiftTab)
            isPlanMode = false
            modeButton?.setTitle("code", for: .normal)
            updateButtonTextHighlight(modeButton, active: false, color: .label)
        } else {
            // Switch to plan mode - send shift+tab 3 times with delays
            terminalView?.send(txt: shiftTab)
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
                self?.terminalView?.send(txt: shiftTab)
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
                    self?.terminalView?.send(txt: shiftTab)
                }
            }
            isPlanMode = true
            modeButton?.setTitle("plan", for: .normal)
            updateButtonTextHighlight(modeButton, active: true, color: .systemBlue)
        }
    }

    @objc private func ctrlPressed() {
        isCtrlActive.toggle()

        if isCtrlActive {
            updateButtonTextHighlight(ctrlButton, active: true, color: .systemOrange)
            controller?.setCtrlModifier(active: true)
        } else {
            updateButtonTextHighlight(ctrlButton, active: false, color: .label)
            controller?.setCtrlModifier(active: false)
        }
    }

    // Called by controller after ctrl+key is sent
    func clearCtrlState() {
        isCtrlActive = false
        updateButtonTextHighlight(ctrlButton, active: false, color: .label)
    }

    // Called by controller when user presses enter to exit bash mode
    func clearBashState() {
        if isBashMode {
            isBashMode = false
            updateButtonTextHighlight(bashButton, active: false, color: .systemPink)
        }
    }

    @objc private func escPressed() {
        terminalView?.send(txt: "\u{1B}") // ESC
        // Clear help state when ESC is pressed
        if isHelpActive {
            isHelpActive = false
            updateButtonTextHighlight(helpButton, active: false, color: .label)
        }
    }

    @objc private func tabPressed() {
        terminalView?.send(txt: "\t") // TAB
    }

    @objc private func bangPressed() {
        isBashMode.toggle()

        if isBashMode {
            terminalView?.send(txt: "!") // Enter bash mode
            updateButtonTextHighlight(bashButton, active: true, color: .systemPink)
        } else {
            terminalView?.send(txt: "\u{1B}") // Exit bash mode with ESC
            updateButtonTextHighlight(bashButton, active: false, color: .label)
        }
    }

    @objc private func newlinePressed() {
        terminalView?.send(txt: "\n") // Line feed
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

    @objc private func helpPressed() {
        if isHelpActive {
            // Already in help mode - send ESC to exit
            terminalView?.send(txt: "\u{1B}")
            isHelpActive = false
            updateButtonTextHighlight(helpButton, active: false, color: .label)
        } else {
            // Enter help mode - send /help then carriage return with delay
            terminalView?.send(txt: "/help")
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                self?.terminalView?.send(txt: "\r")
            }
            isHelpActive = true
            updateButtonTextHighlight(helpButton, active: true, color: .systemCyan)
        }
    }

    // Background color highlight for buttons like bash mode
    private func updateButtonHighlight(_ button: UIButton?, active: Bool, color: UIColor = .clear) {
        guard let button = button else { return }

        UIView.animate(withDuration: 0.2) {
            if active {
                button.backgroundColor = color.withAlphaComponent(0.3)
            } else {
                button.backgroundColor = .clear
            }
        }
    }

    // Text color + bold highlight for buttons like mode, ctrl, help
    private func updateButtonTextHighlight(_ button: UIButton?, active: Bool, color: UIColor) {
        guard let button = button else { return }

        UIView.animate(withDuration: 0.2) {
            if active {
                button.setTitleColor(color, for: .normal)
                button.titleLabel?.font = .systemFont(ofSize: 14, weight: .bold)
                button.tintColor = color
            } else {
                button.setTitleColor(.label, for: .normal)
                button.titleLabel?.font = .systemFont(ofSize: 14, weight: .medium)
                button.tintColor = .label
            }
        }
    }
}

// MARK: - Legacy Custom Terminal Accessory View (Fallback)

typealias CustomTerminalAccessory = GlassTerminalAccessory

// Controller managing SwiftTerm terminal and WebSocket connection
class TerminalController: NSObject, ObservableObject {
    @Published var isConnected = false
    @Published var hasReceivedData = false
    @Published var bufferReplayComplete = false
    @Published var error: String?

    let terminalView: SwiftTerm.TerminalView
    private let webSocketManager: PTYWebSocketManager
    let showDismissButton: Bool

    private var hasSentReady = false

    // Ctrl modifier state
    private var ctrlModifierActive = false
    weak var accessoryView: GlassTerminalAccessory?

    // Buffer batching for performance during large buffer replays
    private var pendingDataBuffer: [UInt8] = []
    private var feedTimer: Timer?
    private let feedQueue = DispatchQueue(label: "com.catnip.terminal.feed", qos: .userInteractive)

    // Connection generation tracking to invalidate stale async callbacks
    private var connectionGeneration: Int = 0

    init(workspaceId: String, baseURL: String, codespaceName: String? = nil, authToken: String? = nil, showDismissButton: Bool = true) {
        // Create terminal view
        self.terminalView = SwiftTerm.TerminalView(frame: .zero)
        self.showDismissButton = showDismissButton

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

            // Mark that we've received data (for loading indicator)
            if !self.hasReceivedData {
                DispatchQueue.main.async {
                    self.hasReceivedData = true
                    self.updateAccessoryStatus()
                }
            }

            // During buffer replay, batch data for better performance
            // After buffer replay, feed immediately for responsive live interaction
            if self.bufferReplayComplete {
                // Live mode - feed immediately on main thread
                DispatchQueue.main.async {
                    let bytes = ArraySlice([UInt8](data))
                    self.terminalView.feed(byteArray: bytes)
                }
            } else {
                // Buffer replay mode - batch for performance
                self.batchData(data)
            }
        }

        // Handle JSON control messages
        webSocketManager.onJSONMessage = { [weak self] message in
            guard let self = self else { return }

            switch message.type {
            case "read-only":
                // Handle read-only status (could show indicator)
                NSLog("🔒 Terminal read-only status: %@", message.data ?? "unknown")

            case "buffer-complete":
                // Buffer replay complete - flush any pending data and mark complete
                NSLog("📋 Buffer replay complete")
                self.flushPendingData()
                DispatchQueue.main.async {
                    self.bufferReplayComplete = true
                    self.updateAccessoryStatus()
                }

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
            .sink { [weak self] _ in
                self?.updateAccessoryStatus()
            }
            .store(in: &cancellables)

        webSocketManager.$isConnected
            .receive(on: DispatchQueue.main)
            .assign(to: &$isConnected)

        webSocketManager.$error
            .receive(on: DispatchQueue.main)
            .sink { [weak self] _ in
                self?.updateAccessoryStatus()
            }
            .store(in: &cancellables)

        webSocketManager.$error
            .receive(on: DispatchQueue.main)
            .assign(to: &$error)
    }

    private var cancellables = Set<AnyCancellable>()

    // MARK: - Batching for Performance

    private func batchData(_ data: Data) {
        feedQueue.async { [weak self] in
            guard let self = self else { return }

            // Append to pending buffer
            self.pendingDataBuffer.append(contentsOf: data)

            // Cancel existing timer on main thread
            DispatchQueue.main.async {
                self.feedTimer?.invalidate()

                // Schedule flush after a short delay (allows batching multiple packets)
                // Use shorter delay during buffer replay for better perceived performance
                let delay: TimeInterval = 0.016 // ~60fps
                self.feedTimer = Timer.scheduledTimer(withTimeInterval: delay, repeats: false) { [weak self] _ in
                    self?.flushPendingData()
                }
            }

            // Also flush if buffer gets large (prevents unbounded memory growth)
            if self.pendingDataBuffer.count > 32768 { // 32KB threshold
                DispatchQueue.main.async {
                    self.feedTimer?.invalidate()
                }
                self.flushPendingData()
            }
        }
    }

    private func flushPendingData() {
        feedQueue.async { [weak self] in
            guard let self = self else { return }
            guard !self.pendingDataBuffer.isEmpty else { return }

            let dataToFeed = self.pendingDataBuffer
            self.pendingDataBuffer.removeAll(keepingCapacity: true)

            // Feed on main thread (SwiftTerm requires it)
            DispatchQueue.main.async {
                let bytes = ArraySlice(dataToFeed)
                self.terminalView.feed(byteArray: bytes)
            }
        }
    }

    func connect() {
        // Increment generation to invalidate any pending callbacks from previous connection
        connectionGeneration += 1
        let currentGeneration = connectionGeneration

        NSLog("🔌 TerminalController.connect() - generation %d", currentGeneration)

        webSocketManager.connect()

        // Wait a bit for connection to establish, then send ready signal
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            guard let self = self,
                  self.connectionGeneration == currentGeneration,
                  !self.hasSentReady else { return }
            self.hasSentReady = true
            self.sendReadySignal()
        }

        // Auto-focus terminal to show keyboard with custom accessory
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { [weak self] in
            guard let self = self, self.connectionGeneration == currentGeneration else { return }
            _ = self.terminalView.becomeFirstResponder()
        }

        // Send another resize after layout settles (helps with orientation changes)
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in
            guard let self = self, self.connectionGeneration == currentGeneration else { return }
            self.handleResize()
        }
    }

    func focusTerminal() {
        _ = terminalView.becomeFirstResponder()
    }

    func disconnect() {
        // Increment generation to invalidate any pending callbacks
        connectionGeneration += 1

        NSLog("🔌 TerminalController.disconnect() - generation now %d", connectionGeneration)

        webSocketManager.disconnect()

        // Clean up batching resources
        feedTimer?.invalidate()
        feedTimer = nil
        feedQueue.async { [weak self] in
            self?.pendingDataBuffer.removeAll()
        }

        // Reset state for next connection
        hasReceivedData = false
        bufferReplayComplete = false
        hasSentReady = false
    }

    func reconnect() {
        NSLog("🔌 TerminalController.reconnect()")
        disconnect()
        // Small delay to ensure clean disconnection
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            self?.connect()
        }
    }

    // Minimum terminal dimensions for TUI rendering
    private static let minCols: UInt16 = 40
    private static let minRows: UInt16 = 15

    private func sendReadySignal() {
        // Get current terminal dimensions with minimums for TUI compatibility
        let cols = max(UInt16(terminalView.getTerminal().cols), Self.minCols)
        let rows = max(UInt16(terminalView.getTerminal().rows), Self.minRows)

        NSLog("📐 Sending ready signal with dimensions: %dx%d (min: %dx%d)", cols, rows, Self.minCols, Self.minRows)

        // Send resize to ensure backend knows our dimensions
        webSocketManager.sendResize(cols: cols, rows: rows)

        // Send ready signal to trigger buffer replay
        webSocketManager.sendReady()
    }

    func handleResize() {
        // Get current terminal dimensions with minimums for TUI compatibility
        let cols = max(UInt16(terminalView.getTerminal().cols), Self.minCols)
        let rows = max(UInt16(terminalView.getTerminal().rows), Self.minRows)

        NSLog("📐 Resize event: %dx%d (actual: %dx%d)", cols, rows, terminalView.getTerminal().cols, terminalView.getTerminal().rows)

        webSocketManager.sendResize(cols: cols, rows: rows)
    }

    @objc func dismissKeyboard() {
        _ = terminalView.resignFirstResponder()
    }

    func setCtrlModifier(active: Bool) {
        ctrlModifierActive = active
    }

    // MARK: - Accessory Status Updates

    func updateAccessoryStatus() {
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }

            if let error = self.error {
                self.accessoryView?.showStatus(message: error)
            } else if !self.isConnected {
                self.accessoryView?.showStatus(message: "Connecting to Claude...")
            } else if !self.hasReceivedData {
                self.accessoryView?.showStatus(message: "Connecting to Claude...")
            } else if !self.bufferReplayComplete {
                self.accessoryView?.showStatus(message: "Rendering...")
            } else {
                self.accessoryView?.hideStatus()
            }
        }
    }
}

// MARK: - TerminalViewDelegate

extension TerminalController: TerminalViewDelegate {
    func send(source: SwiftTerm.TerminalView, data: ArraySlice<UInt8>) {
        // User typed input - send to backend
        var string = String(bytes: data, encoding: .utf8) ?? ""

        // Apply ctrl modifier if active
        if ctrlModifierActive && !string.isEmpty {
            // Convert to ctrl character (ctrl+a = 0x01, ctrl+b = 0x02, etc.)
            // For a-z, ctrl code is char - 'a' + 1
            // For A-Z, ctrl code is char - 'A' + 1
            var ctrlString = ""
            for char in string {
                let scalar = char.unicodeScalars.first?.value ?? 0
                if scalar >= 97 && scalar <= 122 { // a-z
                    let ctrlCode = scalar - 97 + 1
                    ctrlString.append(Character(UnicodeScalar(ctrlCode)!))
                } else if scalar >= 65 && scalar <= 90 { // A-Z
                    let ctrlCode = scalar - 65 + 1
                    ctrlString.append(Character(UnicodeScalar(ctrlCode)!))
                } else {
                    ctrlString.append(char)
                }
            }
            string = ctrlString

            // Clear ctrl state after use
            ctrlModifierActive = false
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.clearCtrlState()
            }
        }

        // Check for enter key to exit bash mode
        if string.contains("\r") || string.contains("\n") {
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.clearBashState()
            }
        }

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

        // Open the link in the default system browser
        guard let url = URL(string: link) else {
            NSLog("⚠️ Invalid URL: %@", link)
            return
        }

        DispatchQueue.main.async {
            UIApplication.shared.open(url, options: [:]) { success in
                if success {
                    NSLog("✅ Opened URL in browser: %@", link)
                } else {
                    NSLog("❌ Failed to open URL: %@", link)
                }
            }
        }
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
        // Full screen terminal preview for testing glass accessory
        TerminalView(
            workspaceId: "test-workspace",
            baseURL: "ws://localhost:8080",
            shouldConnect: false,
            showExitButton: false,
            showDismissButton: true
        )
        .ignoresSafeArea()
        .previewDisplayName("Portrait Terminal")

        // Landscape terminal preview
        TerminalView(
            workspaceId: "test-workspace",
            baseURL: "ws://localhost:8080",
            shouldConnect: false,
            showExitButton: true,
            showDismissButton: true
        )
        .ignoresSafeArea()
        .previewInterfaceOrientation(.landscapeLeft)
        .previewDisplayName("Landscape Terminal")
    }
}
#endif
