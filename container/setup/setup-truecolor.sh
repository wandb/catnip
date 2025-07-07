#!/bin/bash
# Setup script to ensure 24-bit true color support

echo "🎨 Setting up 24-bit true color support..."

# Create custom terminfo for 24-bit color if xterm-direct doesn't exist
if ! infocmp xterm-direct >/dev/null 2>&1; then
    echo "📝 Creating custom xterm-direct terminfo..."
    
    # Create temporary directory for terminfo
    mkdir -p /tmp/terminfo
    
    # Create xterm-direct terminfo source
    cat > /tmp/terminfo/xterm-direct.src << 'EOF'
# xterm-direct terminfo for 24-bit color support
xterm-direct|xterm with direct color indexing,
	use=xterm-256color,
	RGB,
	colors#0x1000000,
	pairs#0x10000,
	setb24=\E[48:2:%p1%{65536}%/%d:%p1%{256}%/%{255}%&%d:%p1%{255}%&%dm,
	setf24=\E[38:2:%p1%{65536}%/%d:%p1%{256}%/%{255}%&%d:%p1%{255}%&%dm,
EOF

    # Compile and install the terminfo
    tic -x /tmp/terminfo/xterm-direct.src
    
    if [ $? -eq 0 ]; then
        echo "✅ Successfully installed xterm-direct terminfo"
    else
        echo "❌ Failed to install xterm-direct terminfo, falling back to xterm-256color"
    fi
    
    # Cleanup
    rm -rf /tmp/terminfo
else
    echo "✅ xterm-direct terminfo already available"
fi

# Test if true color is working
echo "🧪 Testing true color support..."
echo -e "\033[38;2;255;0;0mRed\033[0m \033[38;2;0;255;0mGreen\033[0m \033[38;2;0;0;255mBlue\033[0m"

echo "🎨 True color setup complete!"