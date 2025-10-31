//
//  AuthView.swift
//  catnip
//
//  Authentication screen with GitHub OAuth
//

import SwiftUI

struct AuthView: View {
    @EnvironmentObject var authManager: AuthManager

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
    }
}

#Preview {
    AuthView()
        .environmentObject(MockAuthManager() as AuthManager)
}
