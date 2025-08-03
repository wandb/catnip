#!/bin/bash
# Test script to verify Playwright MCP server installation

echo "🔍 Testing Playwright MCP server installation..."

# Check if @playwright/mcp-server is installed
echo "📦 Checking npm package installation..."
if npm list -g @playwright/mcp-server >/dev/null 2>&1; then
    echo "✅ @playwright/mcp-server is installed globally"
else
    echo "❌ @playwright/mcp-server is NOT installed"
    exit 1
fi

# Check if chromium browser is installed
echo "🌐 Checking Chromium browser installation..."
if npx playwright show-browsers 2>&1 | grep -q "chromium"; then
    echo "✅ Chromium browser is installed"
else
    echo "❌ Chromium browser is NOT installed"
    exit 1
fi

# Check if MCP configuration exists
echo "📋 Checking MCP configuration..."
if [ -f "$HOME/.mcp.json" ]; then
    echo "✅ MCP configuration found at $HOME/.mcp.json"
    echo "📄 Configuration content:"
    cat "$HOME/.mcp.json" | jq . 2>/dev/null || cat "$HOME/.mcp.json"
else
    echo "❌ MCP configuration not found at $HOME/.mcp.json"
    exit 1
fi

# Try to start the MCP server briefly
echo "🚀 Testing MCP server startup..."
timeout 5s npx -y @playwright/mcp-server --headless --browser chromium --no-sandbox >/dev/null 2>&1
EXIT_CODE=$?

# Exit code 124 means timeout (which is expected)
if [ $EXIT_CODE -eq 124 ]; then
    echo "✅ MCP server started successfully (timed out as expected)"
elif [ $EXIT_CODE -eq 0 ]; then
    echo "✅ MCP server started and exited successfully"
else
    echo "❌ MCP server failed to start (exit code: $EXIT_CODE)"
    exit 1
fi

echo "🎉 All tests passed! Playwright MCP server is properly installed."