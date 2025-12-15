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
    @Environment(\.dismiss) private var dismiss

    // CatnipInstaller for status refresh
    @StateObject private var installer = CatnipInstaller.shared

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
                Task {
                    // CRITICAL: Refresh user status BEFORE navigation
                    // This triggers worker verification with ?refresh=true
                    // Rate-limited to prevent abuse (10s server, 10s client)
                    do {
                        try await installer.fetchUserStatus(forceRefresh: true)
                        NSLog("✅ Refreshed user status before reconnect")
                    } catch {
                        NSLog("⚠️ Failed to refresh status: \(error)")
                        // Continue anyway - user will see current state
                        // Graceful degradation if network fails
                    }

                    // Reset health check state
                    await MainActor.run {
                        HealthCheckService.shared.resetShutdownState()
                        // Dismiss this view to go back to CodespaceView with fresh data
                        // CodespaceView will auto-reconnect via SSE
                        dismiss()
                    }
                }
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
    private var navigationPad: NavigationPadView?
    private var keyboardHeight: CGFloat = 0
    private var keyboardObservers: [NSObjectProtocol] = []
    private var resizeObservers: [NSObjectProtocol] = []
    private var lastBounds: CGRect = .zero

    func setup(with terminalView: SwiftTerm.TerminalView, controller: TerminalController) {
        self.terminalView = terminalView
        self.controller = controller

        backgroundColor = .black

        // Add terminal view
        addSubview(terminalView)
        terminalView.translatesAutoresizingMaskIntoConstraints = false

        // Configure scrollbars to not consume width
        terminalView.showsVerticalScrollIndicator = true
        terminalView.showsHorizontalScrollIndicator = false
        // Ensure scrollbars are overlay style (don't reduce content area)
        terminalView.scrollIndicatorInsets = .zero

        // Pin terminal to all edges with minimal padding
        // Reduced from 12pt to 8pt to compensate for any SwiftTerm internal spacing
        // This ensures cols/rows match visual width more accurately
        NSLayoutConstraint.activate([
            terminalView.topAnchor.constraint(equalTo: topAnchor),
            terminalView.bottomAnchor.constraint(equalTo: bottomAnchor),
            terminalView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),  // Left padding
            terminalView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8)  // Right padding to balance
        ])

        // Add content insets: bottom for glass toolbar
        // Left padding is handled by constraints, not content inset (to ensure cols/rows match visual width)
        terminalView.contentInset.bottom = 56

        // Set scroll indicator insets so they don't appear behind the toolbar
        terminalView.scrollIndicatorInsets.bottom = 56

        // Create floating glass toolbar that overlays the terminal
        // We'll position it manually in layoutSubviews to handle the wider terminal view
        let toolbar = GlassTerminalAccessory(
            terminalView: terminalView,
            controller: controller,
            showDismissButton: controller.showDismissButton
        )
        floatingToolbar = toolbar
        controller.accessoryView = toolbar

        // Create navigation pad (floating D-pad)
        let navPad = NavigationPadView(
            terminalView: terminalView,
            controller: controller
        )
        navigationPad = navPad
        toolbar.navigationPad = navPad

        // Observe keyboard to position toolbar above it
        setupKeyboardObservers()

        // Observe orientation and bounds changes for terminal resize
        setupResizeObservers()

        // Add pull-to-refresh for reconnecting WebSocket
        setupRefreshControl(for: terminalView, controller: controller)
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()

        // Add toolbar to this view so it's positioned relative to terminal view bounds, not screen
        if let toolbar = floatingToolbar, toolbar.superview == nil {
            addSubview(toolbar)
            toolbar.alpha = 1  // Always visible at bottom of terminal view
            positionToolbar()
        }

        // Add navigation pad to this view (initially hidden)
        if let navPad = navigationPad, navPad.superview == nil {
            navPad.alpha = 0  // Hidden until toggled
            addSubview(navPad)
            positionNavigationPad()
        }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        positionToolbar()
        positionNavigationPad()

        // layoutSubviews is called frequently, use handleBoundsChange to deduplicate
        handleBoundsChange()
    }

    private func positionToolbar() {
        guard let toolbar = floatingToolbar else { return }

        // Use wrapper's visible width (viewport width), not terminal's scrollable width
        let toolbarWidth = bounds.width
        let toolbarHeight: CGFloat = 56
        let safeAreaBottom = safeAreaInsets.bottom

        // Position toolbar at bottom of visible viewport, above keyboard if visible
        let bottomOffset = keyboardHeight > 0 ? keyboardHeight : safeAreaBottom
        let toolbarY = bounds.height - bottomOffset - toolbarHeight

        toolbar.frame = CGRect(
            x: 0,
            y: toolbarY,
            width: toolbarWidth,
            height: toolbarHeight
        )
    }

    private func positionNavigationPad() {
        guard let navPad = navigationPad else { return }

        let size = NavigationPadView.size
        let margin: CGFloat = 16
        let gapAboveToolbar: CGFloat = 2  // Very close to toolbar for tight visual grouping

        // Position in lower-right corner, above toolbar
        let x = bounds.width - size - margin
        let y = (floatingToolbar?.frame.minY ?? bounds.height) - size - gapAboveToolbar

        navPad.frame = CGRect(x: x, y: y, width: size, height: size)
    }

    override func removeFromSuperview() {
        // Clean up toolbar and navigation pad from window when we're removed
        floatingToolbar?.removeFromSuperview()
        navigationPad?.removeFromSuperview()
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

    private func setupResizeObservers() {
        // Observe device orientation changes
        let orientationObserver = NotificationCenter.default.addObserver(
            forName: UIDevice.orientationDidChangeNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.handleOrientationChange()
        }

        resizeObservers = [orientationObserver]
    }

    private func handleOrientationChange() {
        // Force layout and trigger resize after orientation change
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [weak self] in
            self?.forceTerminalResize()
        }
    }

    private func handleBoundsChange() {
        // Bounds changed, check if width changed significantly
        if abs(bounds.width - lastBounds.width) > 1.0 {
            forceTerminalResize()
        }
    }

    private func forceTerminalResize() {
        lastBounds = bounds

        // Force terminal to relayout with new bounds
        terminalView?.setNeedsLayout()
        terminalView?.layoutIfNeeded()

        // Small delay to ensure SwiftTerm has recalculated cols based on new width
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
            self?.controller?.handleResize()
        }
    }

    private func handleKeyboardWillShow(_ notification: Notification) {
        guard let keyboardFrame = notification.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect,
              let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt else {
            return
        }

        // Convert keyboard frame to this view's coordinates
        let keyboardFrameInView = convert(keyboardFrame, from: nil)
        keyboardHeight = max(0, bounds.height - keyboardFrameInView.origin.y)

        UIView.animate(
            withDuration: duration,
            delay: 0,
            options: UIView.AnimationOptions(rawValue: curve << 16),
            animations: {
                self.positionToolbar()
            }
        )
    }

    private func handleKeyboardWillHide(_ notification: Notification) {
        guard let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt else {
            return
        }

        keyboardHeight = 0

        UIView.animate(
            withDuration: duration,
            delay: 0,
            options: UIView.AnimationOptions(rawValue: curve << 16),
            animations: {
                self.positionToolbar()
            }
        )
    }

    private func handleKeyboardWillChangeFrame(_ notification: Notification) {
        guard let keyboardFrame = notification.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect,
              let duration = notification.userInfo?[UIResponder.keyboardAnimationDurationUserInfoKey] as? Double,
              let curve = notification.userInfo?[UIResponder.keyboardAnimationCurveUserInfoKey] as? UInt else {
            return
        }

        // Convert keyboard frame to this view's coordinates
        let keyboardFrameInView = convert(keyboardFrame, from: nil)
        keyboardHeight = max(0, bounds.height - keyboardFrameInView.origin.y)

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
        resizeObservers.forEach { NotificationCenter.default.removeObserver($0) }
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

    // Navigation pad state
    weak var navigationPad: NavigationPadView?
    private var isNavigationPadVisible = false
    private var navPadToggleButton: UIButton?

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

        // Narrower button for "!"
        let bashBtn = createGlassButton(title: "!", minWidth: 24, action: #selector(bangPressed), accessibilityHint: "Toggle bash mode")
        bashButton = bashBtn
        stackView.addArrangedSubview(bashBtn)

        // Narrower button for "\n"
        stackView.addArrangedSubview(createGlassButton(title: "\\n", minWidth: 24, action: #selector(newlinePressed), accessibilityLabel: "Newline", accessibilityHint: "Send newline character"))
        stackView.addArrangedSubview(createGlassButton(title: "tab", action: #selector(tabPressed), accessibilityHint: "Send tab key for autocomplete"))

        // Navigation pad toggle (replaces 4 individual arrow buttons)
        let navPadButton = createGlassButton(
            systemImage: "dpad",
            action: #selector(navigationPadPressed),
            accessibilityLabel: "Navigation pad",
            accessibilityHint: "Show directional controls"
        )
        navPadToggleButton = navPadButton
        stackView.addArrangedSubview(navPadButton)

        // Help toggle (smaller icon and narrower width)
        let helpBtn = createGlassButton(systemImage: "questionmark", iconSize: 12, minWidth: 24, action: #selector(helpPressed), accessibilityLabel: "Help", accessibilityHint: "Show or hide help menu")
        helpButton = helpBtn
        stackView.addArrangedSubview(helpBtn)

        // Dismiss button (only if enabled)
        if showDismissButton {
            let dismissButton = createGlassButton(systemImage: "keyboard.chevron.compact.down", action: #selector(dismissPressed), accessibilityLabel: "Dismiss keyboard", accessibilityHint: "Hide the keyboard")
            stackView.addArrangedSubview(dismissButton)
        }
    }

    private func createGlassButton(title: String? = nil, systemImage: String? = nil, iconSize: CGFloat = 16, minWidth: CGFloat = 40, action: Selector, accessibilityLabel: String? = nil, accessibilityHint: String? = nil) -> UIButton {
        let button = UIButton(type: .system)

        if let imageName = systemImage {
            let config = UIImage.SymbolConfiguration(pointSize: iconSize, weight: .medium)
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
            button.widthAnchor.constraint(greaterThanOrEqualToConstant: minWidth),
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
            updateButtonTextHighlight(modeButton, active: true, color: .systemCyan)
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

    // MARK: - State Synchronization (for reconnection)

    func syncPlanMode(enabled: Bool) {
        // Sync plan mode state without sending terminal commands
        // Called after reconnection to match TUI state
        isPlanMode = enabled
        if enabled {
            modeButton?.setTitle("plan", for: .normal)
            updateButtonTextHighlight(modeButton, active: true, color: .systemCyan)
        } else {
            modeButton?.setTitle("code", for: .normal)
            updateButtonTextHighlight(modeButton, active: false, color: .label)
        }
        NSLog("🔄 Synced plan mode state: %@", enabled ? "ON" : "OFF")
    }

    func syncBashMode(enabled: Bool) {
        // Sync bash mode state without sending terminal commands
        isBashMode = enabled
        if enabled {
            updateButtonTextHighlight(bashButton, active: true, color: .systemPink)
        } else {
            updateButtonTextHighlight(bashButton, active: false, color: .label)
        }
        NSLog("🔄 Synced bash mode state: %@", enabled ? "ON" : "OFF")
    }

    func syncHelpMode(enabled: Bool) {
        // Sync help mode state without sending terminal commands
        isHelpActive = enabled
        if enabled {
            updateButtonTextHighlight(helpButton, active: true, color: .systemCyan)
        } else {
            updateButtonTextHighlight(helpButton, active: false, color: .label)
        }
        NSLog("🔄 Synced help mode state: %@", enabled ? "ON" : "OFF")
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

    // MARK: - Navigation Pad Management

    @objc private func navigationPadPressed() {
        if isNavigationPadVisible {
            hideNavigationPad()
        } else {
            showNavigationPad()
        }
    }

    private func showNavigationPad() {
        guard let navigationPad = navigationPad, !isNavigationPadVisible else { return }

        isNavigationPadVisible = true

        // Update toggle button state
        updateButtonTextHighlight(navPadToggleButton, active: true, color: .systemBlue)

        // Get button position in parent view coordinates for morph animation
        var originPoint: CGPoint?
        if let button = navPadToggleButton, let parent = superview {
            let buttonCenter = button.convert(CGPoint(x: button.bounds.midX, y: button.bounds.midY), to: parent)
            originPoint = buttonCenter
        }

        // Animate navigation pad in from button position
        navigationPad.show(fromPoint: originPoint)
    }

    private func hideNavigationPad() {
        guard isNavigationPadVisible else { return }

        isNavigationPadVisible = false

        // Update toggle button state
        updateButtonTextHighlight(navPadToggleButton, active: false, color: .label)

        // Get button position in parent view coordinates for morph animation
        var targetPoint: CGPoint?
        if let button = navPadToggleButton, let parent = superview {
            let buttonCenter = button.convert(CGPoint(x: button.bounds.midX, y: button.bounds.midY), to: parent)
            targetPoint = buttonCenter
        }

        // Animate navigation pad out to button position
        navigationPad?.hide(toPoint: targetPoint)
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

// MARK: - Navigation Pad View (Floating D-pad)

class NavigationPadView: UIView {
    static var size: CGFloat = 80  // Configurable for experimentation

    private weak var terminalView: SwiftTerm.TerminalView?
    private weak var controller: TerminalController?

    private var glassContainer: UIVisualEffectView?
    private var upButton: UIButton!
    private var downButton: UIButton!
    private var leftButton: UIButton!
    private var rightButton: UIButton!

    private var repeatingTimer: Timer?
    private var currentDirection: ArrowDirection?
    private var intendedCenter: CGPoint = .zero  // Track proper position for animations

    enum ArrowDirection {
        case up, down, left, right
    }

    init(terminalView: SwiftTerm.TerminalView, controller: TerminalController) {
        self.terminalView = terminalView
        self.controller = controller

        super.init(frame: CGRect(x: 0, y: 0, width: Self.size, height: Self.size))

        setupUI()
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private func setupUI() {
        // Transparent background
        backgroundColor = .clear
        isOpaque = false

        // Create glass container
        let glassView = createGlassContainer()
        glassContainer = glassView
        addSubview(glassView)

        glassView.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            glassView.leadingAnchor.constraint(equalTo: leadingAnchor),
            glassView.trailingAnchor.constraint(equalTo: trailingAnchor),
            glassView.topAnchor.constraint(equalTo: topAnchor),
            glassView.bottomAnchor.constraint(equalTo: bottomAnchor)
        ])

        // Add etched X pattern for visual depth
        addEtchedXPattern(to: glassView)

        // Create directional buttons in diamond layout
        createButtons(in: glassView.contentView)
    }

    private func createGlassContainer() -> UIVisualEffectView {
        let effectView = UIVisualEffectView()
        effectView.clipsToBounds = true

        // Use compile-time check for iOS 26+ SDK availability
        #if compiler(>=6.0)
        if #available(iOS 26.0, *) {
            let glassEffect = UIGlassEffect(style: .regular)
            glassEffect.isInteractive = true
            effectView.cornerConfiguration = .capsule()

            UIView.animate(withDuration: 0.4) {
                effectView.effect = glassEffect
            }
        } else {
            effectView.effect = UIBlurEffect(style: .systemUltraThinMaterial)
            effectView.layer.cornerRadius = 22
        }
        #else
        effectView.effect = UIBlurEffect(style: .systemUltraThinMaterial)
        effectView.layer.cornerRadius = 22
        #endif

        return effectView
    }

    private func addEtchedXPattern(to glassView: UIVisualEffectView) {
        let size = Self.size
        let inset: CGFloat = 12  // Distance from edges

        // Create path for X pattern - two diagonal lines
        let xPath = UIBezierPath()

        // Top-left to bottom-right diagonal
        xPath.move(to: CGPoint(x: inset, y: inset))
        xPath.addLine(to: CGPoint(x: size - inset, y: size - inset))

        // Top-right to bottom-left diagonal
        xPath.move(to: CGPoint(x: size - inset, y: inset))
        xPath.addLine(to: CGPoint(x: inset, y: size - inset))

        // Create shadow layer (darker, slightly offset down-right for depth)
        let shadowLayer = CAShapeLayer()
        shadowLayer.path = xPath.cgPath
        shadowLayer.strokeColor = UIColor.black.withAlphaComponent(0.25).cgColor
        shadowLayer.lineWidth = 0.75
        shadowLayer.fillColor = nil
        shadowLayer.lineCap = .round
        shadowLayer.frame = CGRect(x: 0.6, y: 0.6, width: size, height: size)

        // Create highlight layer (lighter, etched glass look)
        let highlightLayer = CAShapeLayer()
        highlightLayer.path = xPath.cgPath
        highlightLayer.strokeColor = UIColor.white.withAlphaComponent(0.22).cgColor
        highlightLayer.lineWidth = 0.75
        highlightLayer.fillColor = nil
        highlightLayer.lineCap = .round
        highlightLayer.frame = CGRect(x: 0, y: 0, width: size, height: size)

        // Add to glass view's layer (below content)
        glassView.layer.insertSublayer(shadowLayer, at: 0)
        glassView.layer.insertSublayer(highlightLayer, at: 1)
    }

    private func createButtons(in container: UIView) {
        let buttonSize: CGFloat = 36
        let centerOffset: CGFloat = 20  // Distance from center to button center
        let center = Self.size / 2

        // Up button (top)
        upButton = createDirectionButton(
            systemImage: "arrow.up",
            direction: .up,
            x: center - buttonSize/2,
            y: center - centerOffset - buttonSize/2
        )
        container.addSubview(upButton)

        // Down button (bottom)
        downButton = createDirectionButton(
            systemImage: "arrow.down",
            direction: .down,
            x: center - buttonSize/2,
            y: center + centerOffset - buttonSize/2
        )
        container.addSubview(downButton)

        // Left button
        leftButton = createDirectionButton(
            systemImage: "arrow.left",
            direction: .left,
            x: center - centerOffset - buttonSize/2,
            y: center - buttonSize/2
        )
        container.addSubview(leftButton)

        // Right button
        rightButton = createDirectionButton(
            systemImage: "arrow.right",
            direction: .right,
            x: center + centerOffset - buttonSize/2,
            y: center - buttonSize/2
        )
        container.addSubview(rightButton)
    }

    private func createDirectionButton(systemImage: String, direction: ArrowDirection, x: CGFloat, y: CGFloat) -> UIButton {
        let button = UIButton(type: .system)

        // Use lighter weight for more subtle appearance
        let config = UIImage.SymbolConfiguration(pointSize: 16, weight: .light)
        let image = UIImage(systemName: systemImage, withConfiguration: config)
        button.setImage(image, for: .normal)

        // Reduce opacity for subtler look that complements the etched glass
        button.tintColor = UIColor.label.withAlphaComponent(0.65)
        button.backgroundColor = .clear

        button.frame = CGRect(x: x, y: y, width: 36, height: 36)

        // Add subtle etched glass effect to button itself
        addEtchedEffect(to: button)

        // Add tap gesture for instant response
        let tapGesture = UITapGestureRecognizer(target: self, action: #selector(handleTap(_:)))
        button.addGestureRecognizer(tapGesture)

        // Add long press for repeat mode
        let longPress = UILongPressGestureRecognizer(target: self, action: #selector(handleLongPress(_:)))
        longPress.minimumPressDuration = 0.5
        button.addGestureRecognizer(longPress)

        // Tag to identify direction
        button.tag = direction.rawValue

        // Accessibility
        button.accessibilityLabel = "\(systemImage.replacingOccurrences(of: "arrow.", with: "")) arrow"
        button.isAccessibilityElement = true

        return button
    }

    private func addEtchedEffect(to button: UIButton) {
        // Add etched glass effect directly to the arrow icon
        // This creates a subtle shadow + highlight on the symbol itself

        // Wait for imageView to be created, then add shadow/highlight
        DispatchQueue.main.async {
            guard let imageView = button.imageView else { return }

            // Add very subtle drop shadow to the icon for depth (reduced for lighter icons)
            imageView.layer.shadowColor = UIColor.black.cgColor
            imageView.layer.shadowOffset = CGSize(width: 0.4, height: 0.4)
            imageView.layer.shadowOpacity = 0.3  // Reduced from 0.4 to complement lighter icons
            imageView.layer.shadowRadius = 0.4
            imageView.layer.masksToBounds = false

            // Create a duplicate image view for the highlight (etched glass effect)
            if let image = button.image(for: .normal) {
                let highlightImageView = UIImageView(image: image)
                highlightImageView.tintColor = UIColor.white.withAlphaComponent(0.2)  // Slightly reduced
                highlightImageView.frame = imageView.frame
                highlightImageView.contentMode = imageView.contentMode

                // Offset slightly up-left for etched highlight
                highlightImageView.center = CGPoint(
                    x: imageView.center.x - 0.4,
                    y: imageView.center.y - 0.4
                )

                // Insert behind the main image
                button.insertSubview(highlightImageView, belowSubview: imageView)

                // Make highlight move with the main imageView
                highlightImageView.translatesAutoresizingMaskIntoConstraints = false
                NSLayoutConstraint.activate([
                    highlightImageView.centerXAnchor.constraint(equalTo: imageView.centerXAnchor, constant: -0.4),
                    highlightImageView.centerYAnchor.constraint(equalTo: imageView.centerYAnchor, constant: -0.4),
                    highlightImageView.widthAnchor.constraint(equalTo: imageView.widthAnchor),
                    highlightImageView.heightAnchor.constraint(equalTo: imageView.heightAnchor)
                ])
            }
        }
    }

    @objc private func handleTap(_ gesture: UITapGestureRecognizer) {
        guard let button = gesture.view as? UIButton,
              let direction = ArrowDirection(rawValue: button.tag) else { return }

        // Visual feedback
        animateButtonPress(button)

        // Send arrow key
        sendArrowKey(direction)
    }

    @objc private func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
        guard let button = gesture.view as? UIButton,
              let direction = ArrowDirection(rawValue: button.tag) else { return }

        switch gesture.state {
        case .began:
            // Enter repeat mode
            currentDirection = direction

            // Highlight button
            UIView.animate(withDuration: 0.2) {
                button.backgroundColor = UIColor.systemBlue.withAlphaComponent(0.3)
            }

            // Send first arrow key
            sendArrowKey(direction)

            // Start repeating timer
            repeatingTimer = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
                self?.sendArrowKey(direction)
            }

        case .ended, .cancelled:
            // Exit repeat mode
            currentDirection = nil

            // Clear highlight
            UIView.animate(withDuration: 0.2) {
                button.backgroundColor = .clear
            }

            // Stop timer
            repeatingTimer?.invalidate()
            repeatingTimer = nil

        default:
            break
        }
    }

    private func animateButtonPress(_ button: UIButton) {
        UIView.animate(withDuration: 0.1, animations: {
            button.transform = CGAffineTransform(scaleX: 0.95, y: 0.95)
        }) { _ in
            UIView.animate(withDuration: 0.1) {
                button.transform = .identity
            }
        }
    }

    private func sendArrowKey(_ direction: ArrowDirection) {
        switch direction {
        case .up:
            terminalView?.send(txt: "\u{1B}[A")
        case .down:
            terminalView?.send(txt: "\u{1B}[B")
        case .left:
            terminalView?.send(txt: "\u{1B}[D")
        case .right:
            terminalView?.send(txt: "\u{1B}[C")
        }
    }

    func cleanup() {
        repeatingTimer?.invalidate()
        repeatingTimer = nil
        currentDirection = nil
    }

    // MARK: - Show/Hide Animations (Liquid Glass Style)

    func show(fromPoint: CGPoint? = nil, completion: (() -> Void)? = nil) {
        // Store the intended final position BEFORE any modifications
        // This is where positionNavigationPad() placed us
        if intendedCenter == .zero {
            intendedCenter = center
        }
        let finalCenter = intendedCenter

        if let origin = fromPoint {
            // Morph from button position
            center = origin
            transform = CGAffineTransform(scaleX: 0.15, y: 0.15)  // Start very small like a button
            alpha = 0
        } else {
            // Fallback: standard entrance from below
            transform = CGAffineTransform(scaleX: 0.3, y: 0.3)
                .rotated(by: .pi / 12)
                .translatedBy(x: 0, y: 20)
            alpha = 0
        }

        // Animate glass effect materialization (iOS 26+ feature)
        #if compiler(>=6.0)
        if #available(iOS 26.0, *), let glassContainer = glassContainer {
            glassContainer.effect = nil
        }
        #endif

        // Fluid spring animation for liquid glass feel with morph effect
        UIView.animate(
            withDuration: 0.55,
            delay: 0,
            usingSpringWithDamping: 0.68,  // Slightly higher for smoother morph
            initialSpringVelocity: 0.6,
            options: [.curveEaseOut, .allowUserInteraction],
            animations: {
                self.alpha = 1.0
                self.center = finalCenter
                self.transform = .identity

                // Materialize glass effect for liquid glass shimmer
                #if compiler(>=6.0)
                if #available(iOS 26.0, *), let glassContainer = self.glassContainer {
                    let glassEffect = UIGlassEffect(style: .regular)
                    glassEffect.isInteractive = true
                    glassContainer.effect = glassEffect
                }
                #endif
            },
            completion: { _ in
                completion?()
            }
        )
    }

    func hide(toPoint: CGPoint? = nil, completion: (() -> Void)? = nil) {
        // Animate out: morph back to button position or standard exit
        UIView.animate(
            withDuration: 0.4,
            delay: 0,
            usingSpringWithDamping: 0.85,  // Higher damping for controlled collapse
            initialSpringVelocity: 0.3,
            options: [.curveEaseIn, .allowUserInteraction],
            animations: {
                self.alpha = 0

                if let target = toPoint {
                    // Morph back to button position
                    self.center = target
                    self.transform = CGAffineTransform(scaleX: 0.15, y: 0.15)  // Collapse to button size
                } else {
                    // Fallback: standard exit
                    self.transform = CGAffineTransform(scaleX: 0.3, y: 0.3)
                        .rotated(by: -.pi / 12)
                        .translatedBy(x: 0, y: 20)
                }

                // Dematerialize glass effect
                #if compiler(>=6.0)
                if #available(iOS 26.0, *), let glassContainer = self.glassContainer {
                    glassContainer.effect = nil
                }
                #endif
            },
            completion: { [weak self] _ in
                guard let self = self else { return }

                // Reset transform for next show
                self.transform = .identity

                // Reset to intended position for next show animation
                self.center = self.intendedCenter

                // Clean up any active repeat timers
                self.cleanup()
                completion?()
            }
        )
    }
}

extension NavigationPadView.ArrowDirection: RawRepresentable {
    init?(rawValue: Int) {
        switch rawValue {
        case 0: self = .up
        case 1: self = .down
        case 2: self = .left
        case 3: self = .right
        default: return nil
        }
    }

    var rawValue: Int {
        switch self {
        case .up: return 0
        case .down: return 1
        case .left: return 2
        case .right: return 3
        }
    }
}

// MARK: - Legacy Custom Terminal Accessory View (Fallback)

typealias CustomTerminalAccessory = GlassTerminalAccessory

// Controller managing SwiftTerm terminal and PTY data source (real or mock)
class TerminalController: NSObject, ObservableObject {
    @Published var isConnected = false
    @Published var hasReceivedData = false
    @Published var bufferReplayComplete = false
    @Published var error: String?

    let terminalView: SwiftTerm.TerminalView
    private let dataSource: PTYDataSource
    let showDismissButton: Bool

    private var hasSentReady = false

    // Ctrl modifier state
    private var ctrlModifierActive = false
    weak var accessoryView: GlassTerminalAccessory?

    // Buffer batching for performance during large buffer replays
    // Keep buffer on main thread to eliminate queue hopping
    private var pendingDataBuffer: [UInt8] = []
    private var feedTimer: Timer?

    // Connection generation tracking to invalidate stale async callbacks
    private var connectionGeneration: Int = 0

    // Connection state tracking for event-based initialization
    private var hasInitializedTerminal = false

    // Debouncing for status updates to prevent rapid UI changes
    private var statusUpdateTimer: Timer?

    // Recent PTY output buffer for backspace fix (last 5KB)
    // Used to detect prompt text after reconnection
    private var recentOutputBuffer: Data = Data()
    private let maxBufferSize = 5 * 1024  // 5KB

    // Flag to suppress sending during textInputStorage population
    private var suppressSendDuringPopulation = false

    // Convenience init for live WebSocket connection
    convenience init(workspaceId: String, baseURL: String, codespaceName: String? = nil, authToken: String? = nil, showDismissButton: Bool = true) {
        let liveDataSource = LivePTYDataSource(
            workspaceId: workspaceId,
            agent: "claude",
            baseURL: baseURL,
            codespaceName: codespaceName,
            authToken: authToken
        )
        self.init(dataSource: liveDataSource, showDismissButton: showDismissButton)
    }

    // Primary init accepting any PTYDataSource (for testing/preview)
    init(dataSource: PTYDataSource, showDismissButton: Bool = true) {
        // Create terminal view
        self.terminalView = SwiftTerm.TerminalView(frame: .zero)
        self.showDismissButton = showDismissButton
        self.dataSource = dataSource

        super.init()

        // Setup terminal
        setupTerminal()

        // Setup data source callbacks
        setupDataSourceCallbacks()
    }

    private func setupTerminal() {
        terminalView.terminalDelegate = self

        // Configure terminal options
        terminalView.optionAsMetaKey = true

        // CRITICAL: Set nativeBackgroundColor to opaque black (not .clear)
        // SwiftTerm defaults to .clear on iOS, which breaks inverse video for TUI cursors
        // Inverse video needs opaque colors to properly swap fg/bg
        terminalView.nativeBackgroundColor = UIColor.black

        // Set caret to clear to prevent system caret from rendering
        // The TUI controls its own cursor rendering via escape sequences
        terminalView.caretColor = UIColor.clear

        // TODO: Fix backspace issue after reconnection
        // SwiftTerm blocks backspace when textInputStorage is empty (line 1127 in iOSTerminalView.swift)
        // Need to either: 1) Fork SwiftTerm and expose textInputStorage, or
        // 2) Implement custom text input handling to bypass SwiftTerm's check
    }

    private func setupDataSourceCallbacks() {
        // Handle binary PTY output
        dataSource.onData = { [weak self] data in
            guard let self = self else { return }

            // Always process on main thread to eliminate queue hopping
            DispatchQueue.main.async {
                // Mark that we've received data (for loading indicator)
                if !self.hasReceivedData {
                    self.hasReceivedData = true
                    self.updateAccessoryStatus()
                }

                // Append to recent output buffer (keep last 5KB for backspace fix)
                self.recentOutputBuffer.append(data)
                if self.recentOutputBuffer.count > self.maxBufferSize {
                    self.recentOutputBuffer = self.recentOutputBuffer.suffix(self.maxBufferSize)
                }

                // During buffer replay, batch data for better performance
                // After buffer replay, feed immediately for responsive live interaction
                if self.bufferReplayComplete {
                    // Live mode - feed immediately (already on main thread)
                    let bytes = ArraySlice([UInt8](data))
                    self.terminalView.feed(byteArray: bytes)
                } else {
                    // Buffer replay mode - batch for performance (on main thread)
                    self.batchData(data)
                }
            }
        }

        // Handle JSON control messages
        dataSource.onJSONMessage = { [weak self] message in
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

                    // Initialize terminal after buffer replay is complete
                    // Only do this once per connection
                    if !self.hasInitializedTerminal {
                        self.hasInitializedTerminal = true

                        // Detect TUI state from buffer contents
                        self.detectAndSyncTUIState()

                        // Send resize to ensure TUI is properly sized
                        self.handleResize()

                        // Now that buffer is complete, show the cursor
                        // This allows the TUI to control cursor rendering via escape sequences
                        self.terminalView.showCursor(source: self.terminalView.getTerminal())
                    }

                    // Note: We don't query cursor position for Claude sessions because:
                    // 1. Claude's TUI may not be fully initialized yet, leading to wrong position
                    // 2. SwiftTerm should track cursor correctly from escape sequences
                    // 3. The forced resize will trigger a redraw with correct cursor positioning
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

        // Monitor connection status and send ready signal when connected
        dataSource.isConnected
            .receive(on: DispatchQueue.main)
            .sink { [weak self] isConnected in
                guard let self = self else { return }

                // Send ready signal as soon as connected (event-based, not time-based)
                if isConnected && !self.hasSentReady {
                    self.hasSentReady = true
                    self.sendReadySignal()
                }

                self.updateAccessoryStatus()
            }
            .store(in: &cancellables)

        dataSource.isConnected
            .receive(on: DispatchQueue.main)
            .assign(to: &$isConnected)

        dataSource.error
            .receive(on: DispatchQueue.main)
            .sink { [weak self] _ in
                self?.updateAccessoryStatus()
            }
            .store(in: &cancellables)

        dataSource.error
            .receive(on: DispatchQueue.main)
            .assign(to: &$error)
    }

    private var cancellables = Set<AnyCancellable>()

    // MARK: - Batching for Performance

    private func batchData(_ data: Data) {
        // Already on main thread - no queue hopping needed
        // Append to pending buffer
        pendingDataBuffer.append(contentsOf: data)

        // Cancel existing timer and schedule new one
        feedTimer?.invalidate()

        // Flush immediately if buffer gets large (prevents unbounded memory growth)
        if pendingDataBuffer.count > 32768 { // 32KB threshold
            flushPendingData()
        } else {
            // Schedule flush after a short delay (allows batching multiple packets)
            // Reduced delay for faster initial render
            let delay: TimeInterval = 0.008 // ~120fps, half the previous delay
            feedTimer = Timer.scheduledTimer(withTimeInterval: delay, repeats: false) { [weak self] _ in
                self?.flushPendingData()
            }
        }
    }

    private func flushPendingData() {
        // Already on main thread (SwiftTerm requires it)
        guard !pendingDataBuffer.isEmpty else { return }

        let bytes = ArraySlice(pendingDataBuffer)
        pendingDataBuffer.removeAll(keepingCapacity: true)

        terminalView.feed(byteArray: bytes)
    }

    // MARK: - TUI State Detection and Backspace Fix

    private func detectAndSyncTUIState() {
        // Parse our recentOutputBuffer to detect TUI state and fix backspace issue
        guard let bufferText = String(data: recentOutputBuffer, encoding: .utf8) else {
            NSLog("⚠️ Unable to decode recent buffer as UTF-8")
            return
        }

        NSLog("🔍 Analyzing recent buffer (%d bytes) for TUI state", recentOutputBuffer.count)

        // Strip ANSI color codes for text matching
        let cleanedBuffer = stripANSIColorCodes(bufferText)

        // Detect plan mode: "plan mode on (shift+tab to cycle)"
        if cleanedBuffer.contains("plan mode on") {
            NSLog("✅ Detected: Plan mode is ON")
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.syncPlanMode(enabled: true)
            }
        } else {
            NSLog("ℹ️ Detected: Plan mode is OFF (code mode)")
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.syncPlanMode(enabled: false)
            }
        }

        // Detect bash mode: "! for bash mode"
        // When "! for bash mode" is visible → we ARE in bash mode (it's the mode indicator)
        // When it's not visible → we are NOT in bash mode
        if cleanedBuffer.contains("! for bash mode") {
            NSLog("✅ Detected: IN bash mode (indicator visible)")
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.syncBashMode(enabled: true)
            }
        } else {
            NSLog("ℹ️ NOT in bash mode (indicator not visible)")
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.syncBashMode(enabled: false)
            }
        }

        // Detect help mode: "For more help: https://docs.claude.com"
        if cleanedBuffer.contains("For more help:") && cleanedBuffer.contains("docs.claude.com") {
            NSLog("✅ Detected: Help mode is ACTIVE")
            DispatchQueue.main.async { [weak self] in
                self?.accessoryView?.syncHelpMode(enabled: true)
            }
        }

        // Fix backspace issue: Extract prompt text after ">" and populate SwiftTerm's textInputStorage
        populateTextInputStorage(from: cleanedBuffer)
    }

    private func populateTextInputStorage(from bufferText: String) {
        // Extract text after the last ">" prompt marker
        // This text is what the user should be able to backspace after reconnection

        let lines = bufferText.components(separatedBy: "\n")

        // Find the last line with a ">" prompt
        for line in lines.reversed() {
            if let promptRange = line.range(of: ">") {
                // Extract text after ">" (still has ANSI codes)
                let afterPromptWithCodes = String(line[promptRange.upperBound...])

                // Check if this is a placeholder vs actual user input
                // Method 1: Check for "Try " prefix (most reliable)
                // Method 2: Check for dim/gray escape codes: ESC[2m or ESC[90m
                let cleanedText = stripANSIColorCodes(afterPromptWithCodes)
                if cleanedText.trimmingCharacters(in: .whitespaces).hasPrefix("Try \"") {
                    NSLog("ℹ️ Skipping placeholder prompt (starts with 'Try \"')")
                    continue  // Skip placeholders
                }
                if afterPromptWithCodes.contains("\u{1B}[2m") || afterPromptWithCodes.contains("\u{1B}[90m") {
                    NSLog("ℹ️ Skipping placeholder prompt (dim/gray ANSI codes)")
                    continue  // Skip placeholders
                }

                // Strip ANSI codes to get clean text
                let afterPrompt = stripANSIColorCodes(afterPromptWithCodes).trimmingCharacters(in: .whitespaces)

                if !afterPrompt.isEmpty {
                    NSLog("🔍 Found user prompt text to populate: \"%@\"", afterPrompt)

                    // Populate textInputStorage using insertText with send suppression
                    populateTextInputStorageViaInsertText(afterPrompt)
                    return
                }
            }
        }

        NSLog("ℹ️ No user prompt text found to populate")
    }

    private func populateTextInputStorageViaInsertText(_ text: String) {
        // Use SwiftTerm's insertText() method to populate textInputStorage
        // But suppress the send to server by setting a flag

        NSLog("📝 Populating textInputStorage with: \"%@\"", text)

        // Set flag to suppress sending
        suppressSendDuringPopulation = true

        // Call insertText - this will populate textInputStorage and update selectedTextRange
        // Our send() delegate method will see the flag and skip sending to server
        terminalView.insertText(text)

        // Clear flag
        suppressSendDuringPopulation = false

        NSLog("✅ Successfully populated textInputStorage without sending to server")
    }

    private func stripANSIColorCodes(_ text: String) -> String {
        // Strip ANSI escape sequences (color codes, cursor movement, etc.)
        // Pattern matches:
        // - ESC[...m (colors, styles)
        // - ESC[...H (cursor position)
        // - ESC[...J (clear screen)
        // - ESC[...K (clear line)
        // - Other CSI sequences
        let pattern = "\\u{1B}\\[[0-9;?]*[a-zA-Z]"
        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
            return text
        }
        let range = NSRange(text.startIndex..., in: text)
        return regex.stringByReplacingMatches(in: text, options: [], range: range, withTemplate: "")
    }

    func connect() {
        // Increment generation to invalidate any pending callbacks from previous connection
        connectionGeneration += 1
        let currentGeneration = connectionGeneration
        hasInitializedTerminal = false

        NSLog("🔌 TerminalController.connect() - generation %d", currentGeneration)

        dataSource.connect()

        // Auto-focus terminal immediately to show keyboard with custom accessory
        // No need to wait - this is UI only and doesn't depend on connection
        _ = terminalView.becomeFirstResponder()

        // Send ready signal as soon as connected (event-based, not time-based)
        // We'll monitor the isConnected publisher in setupWebSocketCallbacks
    }

    func focusTerminal() {
        _ = terminalView.becomeFirstResponder()
    }

    func disconnect() {
        // Increment generation to invalidate any pending callbacks
        connectionGeneration += 1

        NSLog("🔌 TerminalController.disconnect() - generation now %d", connectionGeneration)

        dataSource.disconnect()

        // Clean up batching resources (on main thread)
        feedTimer?.invalidate()
        feedTimer = nil
        pendingDataBuffer.removeAll()

        // Clean up status update timer
        statusUpdateTimer?.invalidate()
        statusUpdateTimer = nil

        // Clear recent output buffer on disconnect
        recentOutputBuffer = Data()

        // Reset state for next connection
        hasReceivedData = false
        bufferReplayComplete = false
        hasSentReady = false
        hasInitializedTerminal = false
    }

    func reconnect() {
        NSLog("🔌 TerminalController.reconnect()")
        disconnect()
        // Small delay to ensure clean disconnection
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            self?.connect()
        }
    }

    // Target terminal dimensions for optimal TUI rendering
    // 74 columns: Claude Code's TUI works best with ~70+ cols for diff views, progress bars,
    // and wrapped text. This target is achieved on iPad and iPhone landscape, but iPhone
    // portrait (~50-55 cols at 12pt font) will have narrower output with more line wrapping.
    // Note: This is a target, not enforced - actual cols depend on available screen width.
    static let minCols: UInt16 = 74
    private static let minRows: UInt16 = 15

    // Calculate minimum width for minCols (DRY helper)
    static func calculateMinWidth(font: UIFont) -> CGFloat {
        let charWidth = ("M" as NSString).size(withAttributes: [.font: font]).width
        // Add extra buffer to ensure we actually get minCols displayed
        // Account for padding, margins, scrollbar, and rounding errors
        return charWidth * CGFloat(minCols + 4)
    }

    private func sendReadySignal() {
        // CRITICAL: Force layout pass to ensure we have correct dimensions
        // Without this, portrait mode gets landscape dimensions on initial load
        terminalView.setNeedsLayout()
        terminalView.layoutIfNeeded()

        // 150ms delay: SwiftTerm needs time to recalculate terminal dimensions after
        // layoutIfNeeded(). This accounts for the layout pass completing, SwiftTerm's
        // internal column/row recalculation, and any pending view updates. Shorter delays
        // (50-100ms) resulted in stale dimensions being sent on device rotation.
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) { [weak self] in
            guard let self = self else { return }

            // Get current terminal dimensions - use actual size, no minimum enforcement
            let terminal = self.terminalView.getTerminal()
            let cols = UInt16(terminal.cols)
            let rows = UInt16(terminal.rows)

            // DEBUG: Log all dimension info
            print("📐 [sendReadySignal] Terminal buffer: \(terminal.cols)x\(terminal.rows)")
            print("📐 [sendReadySignal] TerminalView bounds: \(self.terminalView.bounds)")
            print("📐 [sendReadySignal] TerminalView frame: \(self.terminalView.frame)")
            print("📐 [sendReadySignal] Sending to backend: \(cols)x\(rows)")

            // Note: Don't call showCursor() - let Claude's TUI control cursor visibility
            // via escape sequences. Early showCursor() causes cursor to be visible at wrong position.

            // Send resize to ensure backend knows our dimensions
            self.dataSource.sendResize(cols: cols, rows: rows)

            // Send ready signal to trigger buffer replay
            self.dataSource.sendReady()
        }
    }

    func handleResize() {
        // Get current terminal dimensions - use actual size, no minimum enforcement
        let terminal = terminalView.getTerminal()
        let cols = UInt16(terminal.cols)
        let rows = UInt16(terminal.rows)

        // DEBUG: Log all dimension info
        print("📐 [handleResize] Terminal buffer: \(terminal.cols)x\(terminal.rows)")
        print("📐 [handleResize] TerminalView bounds: \(terminalView.bounds)")
        print("📐 [handleResize] TerminalView frame: \(terminalView.frame)")
        print("📐 [handleResize] Sending to backend: \(cols)x\(rows)")

        dataSource.sendResize(cols: cols, rows: rows)
    }

    @objc func dismissKeyboard() {
        _ = terminalView.resignFirstResponder()
    }

    func setCtrlModifier(active: Bool) {
        ctrlModifierActive = active
    }

    // MARK: - Accessory Status Updates

    func updateAccessoryStatus() {
        // Debounce status updates to prevent rapid UI changes
        // Only debounce during connecting/loading states, not for final states
        let shouldDebounce = !isConnected || !hasReceivedData || !bufferReplayComplete

        if shouldDebounce {
            statusUpdateTimer?.invalidate()
            statusUpdateTimer = Timer.scheduledTimer(withTimeInterval: 0.05, repeats: false) { [weak self] _ in
                self?.performStatusUpdate()
            }
        } else {
            // For final states (connected and ready), update immediately
            performStatusUpdate()
        }
    }

    private func performStatusUpdate() {
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
        // Check if we're suppressing sends during textInputStorage population
        if suppressSendDuringPopulation {
            // Don't send to server - we're just populating the input buffer
            return
        }

        // User typed input - send to backend
        var string = String(bytes: data, encoding: .utf8) ?? ""

        // DIAGNOSTIC: Log what's being sent, especially backspace/delete
        let byteArray = Array(data)
        if byteArray.contains(0x7F) || byteArray.contains(0x08) {
            NSLog("⌫ Backspace/Delete pressed - sending bytes: %@", byteArray.map { String(format: "0x%02X", $0) }.joined(separator: " "))
        }

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

        dataSource.sendInput(string)
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
        // Portrait terminal preview with realistic Claude TUI content
        TerminalViewWithMockData(
            showExitButton: false,
            showDismissButton: true
        )
        .ignoresSafeArea()
        .previewDisplayName("Portrait Terminal")

        // Landscape terminal preview with realistic Claude TUI content
        TerminalViewWithMockData(
            showExitButton: true,
            showDismissButton: true
        )
        .ignoresSafeArea()
        .previewInterfaceOrientation(.landscapeLeft)
        .previewDisplayName("Landscape Terminal")
    }
}

// Helper view that creates a TerminalView with mock data source
private struct TerminalViewWithMockData: View {
    let showExitButton: Bool
    let showDismissButton: Bool

    @StateObject private var terminalController: TerminalController

    init(showExitButton: Bool, showDismissButton: Bool) {
        self.showExitButton = showExitButton
        self.showDismissButton = showDismissButton

        // Create mock data source with realistic Claude content
        let mockDataSource = MockPTYDataSource.createPreviewDataSource(playbackSpeed: 1.0)
        _terminalController = StateObject(wrappedValue: TerminalController(
            dataSource: mockDataSource,
            showDismissButton: showDismissButton
        ))
    }

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            TerminalViewRepresentable(controller: terminalController)
                .ignoresSafeArea(.container, edges: .top)
        }
        .ignoresSafeArea(.container, edges: .top)
        .preferredColorScheme(.dark)
        .toolbar {
            if showExitButton {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        // Preview only - no action
                    } label: {
                        Image(systemName: "arrow.down.right.and.arrow.up.left")
                            .font(.body)
                    }
                }
            }
        }
        .onAppear {
            // Connect and start replay
            terminalController.connect()
        }
        .onDisappear {
            terminalController.disconnect()
        }
    }
}
#endif
