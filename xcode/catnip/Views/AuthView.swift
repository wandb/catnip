//
//  AuthView.swift
//  catnip
//
//  Authentication screen with GitHub OAuth
//

import SwiftUI

// MARK: - Shake Gesture Detection

extension UIDevice {
    static let deviceDidShakeNotification = Notification.Name(rawValue: "deviceDidShakeNotification")
}

extension UIWindow {
    open override func motionEnded(_ motion: UIEvent.EventSubtype, with event: UIEvent?) {
        if motion == .motionShake {
            NotificationCenter.default.post(name: UIDevice.deviceDidShakeNotification, object: nil)
        }
    }
}

struct DeviceShakeViewModifier: ViewModifier {
    let action: () -> Void

    func body(content: Content) -> some View {
        content
            .onAppear()
            .onReceive(NotificationCenter.default.publisher(for: UIDevice.deviceDidShakeNotification)) { _ in
                action()
            }
    }
}

extension View {
    func onShake(perform action: @escaping () -> Void) -> some View {
        self.modifier(DeviceShakeViewModifier(action: action))
    }
}

// MARK: - Auth View

struct AuthView: View {
    @EnvironmentObject var authManager: AuthManager
    @State private var showMoreOptions = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 20) {
                    Spacer(minLength: 80)

                    // Logo
                    Image("logo")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 80, height: 80)
                        .clipShape(RoundedRectangle(cornerRadius: 16))
                        .shadow(color: Color.black.opacity(0.1), radius: 8, x: 0, y: 2)

                    Text("Catnip")
                        .font(.largeTitle.weight(.bold))
                        .foregroundStyle(.primary)

                    Text("Access your GitHub Codespaces")
                        .font(.body)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.bottom, 8)

                    Button {
                        Task {
                            await authManager.login()
                        }
                    } label: {
                        Text("Sign in with GitHub")
                    }
                    .buttonStyle(ProminentButtonStyle())
                    .padding(.horizontal, 20)

                    Spacer()
                }
                .padding(.horizontal, 20)
            }
            .scrollBounceBehavior(.basedOnSize)
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if showMoreOptions {
                    ToolbarItem(placement: .navigationBarTrailing) {
                        Menu {
                            Button {
                                authManager.enterPreviewMode()
                            } label: {
                                Label("Preview Mode", systemImage: "eye")
                            }
                        } label: {
                            Image(systemName: "ellipsis")
                                .imageScale(.large)
                                .fontWeight(.bold)
                        }
                    }
                }
            }
            .onShake {
                showMoreOptions = true
            }
        }
    }
}

#Preview {
    AuthView()
        .environmentObject(MockAuthManager() as AuthManager)
}
