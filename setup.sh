#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Setting up Catnip development environment...${NC}"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to install missing tools
install_instructions() {
    echo -e "${RED}❌ $1 is not installed${NC}"
    echo -e "${YELLOW}📋 Installation instructions:${NC}"
    case $1 in
        "pnpm")
            echo "  • Install via npm: npm install -g pnpm"
            echo "  • Install via curl: curl -fsSL https://get.pnpm.io/install.sh | sh -"
            echo "  • Install via Homebrew: brew install pnpm"
            echo "  • More options: https://pnpm.io/installation"
            ;;
        "just")
            echo "  • Install via cargo: cargo install just"
            echo "  • Install via Homebrew: brew install just"
            echo "  • Install via package manager: https://github.com/casey/just#installation"
            ;;
        "go")
            echo "  • Download from: https://golang.org/dl/"
            echo "  • Install via Homebrew: brew install go"
            ;;
        "node")
            echo "  • Download from: https://nodejs.org/"
            echo "  • Install via Homebrew: brew install node"
            echo "  • Install via nvm: https://github.com/nvm-sh/nvm"
            ;;
    esac
    echo ""
}

# Check required dependencies
missing_deps=()

echo -e "${YELLOW}🔍 Checking dependencies...${NC}"

if ! command_exists node; then
    install_instructions "node"
    missing_deps+=("node")
fi

if ! command_exists pnpm; then
    install_instructions "pnpm"
    missing_deps+=("pnpm")
fi

if ! command_exists go; then
    install_instructions "go"
    missing_deps+=("go")
fi

if ! command_exists just; then
    install_instructions "just"
    missing_deps+=("just")
fi

# Exit if dependencies are missing
if [ ${#missing_deps[@]} -ne 0 ]; then
    echo -e "${RED}❌ Missing dependencies: ${missing_deps[*]}${NC}"
    echo -e "${YELLOW}Please install the missing dependencies and run this script again.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ All dependencies are installed${NC}"

# Install pnpm packages
echo -e "${YELLOW}📦 Installing pnpm packages...${NC}"
if pnpm install; then
    echo -e "${GREEN}✅ pnpm packages installed${NC}"
else
    echo -e "${RED}❌ Failed to install pnpm packages${NC}"
    exit 1
fi

# Install Go dependencies
echo -e "${YELLOW}📦 Installing Go dependencies...${NC}"
cd container
if just deps; then
    echo -e "${GREEN}✅ Go dependencies installed${NC}"
else
    echo -e "${RED}❌ Failed to install Go dependencies${NC}"
    exit 1
fi
cd ..

# Install pre-commit hook
echo -e "${YELLOW}🪝 Installing pre-commit hook...${NC}"
if [ -f ".git/hooks/pre-commit" ]; then
    echo -e "${YELLOW}⚠️  Pre-commit hook already exists. Backing up...${NC}"
    mv .git/hooks/pre-commit .git/hooks/pre-commit.backup
fi

# Create the pre-commit hook
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🎨 Running pre-commit formatters...${NC}"

# Track if any files were formatted
formatted_files=false

# Format TypeScript/JavaScript files
echo "Formatting TypeScript/JavaScript files..."
if pnpm format:changed 2>/dev/null; then
    # Check if any files were actually formatted
    if [ -n "$(git diff --name-only)" ]; then
        formatted_files=true
        echo -e "${GREEN}✅ TypeScript/JavaScript files formatted${NC}"
    else
        echo -e "${GREEN}✅ No TypeScript/JavaScript files needed formatting${NC}"
    fi
else
    echo -e "${RED}❌ Failed to format TypeScript/JavaScript files${NC}"
    exit 1
fi

# Format Go files
echo "Formatting Go files..."
cd container
if just format-go-changed 2>/dev/null; then
    # Check if any files were actually formatted
    if [ -n "$(git diff --name-only)" ]; then
        formatted_files=true
        echo -e "${GREEN}✅ Go files formatted${NC}"
    else
        echo -e "${GREEN}✅ No Go files needed formatting${NC}"
    fi
else
    echo -e "${RED}❌ Failed to format Go files${NC}"
    exit 1
fi
cd ..

# If files were formatted, add them to staging and inform user
if [ "$formatted_files" = true ]; then
    echo -e "${YELLOW}📝 Files were formatted. Adding to staging area...${NC}"
    git add -u
    echo -e "${GREEN}✅ Pre-commit formatting complete${NC}"
else
    echo -e "${GREEN}✅ All files already formatted${NC}"
fi

echo -e "${GREEN}🎉 Pre-commit hook completed successfully${NC}"
EOF

# Make the hook executable
chmod +x .git/hooks/pre-commit
echo -e "${GREEN}✅ Pre-commit hook installed${NC}"

# Build initial setup
echo -e "${YELLOW}🏗️  Building initial setup...${NC}"
cd container
if just build; then
    echo -e "${GREEN}✅ Go server built successfully${NC}"
else
    echo -e "${RED}❌ Failed to build Go server${NC}"
    exit 1
fi
cd ..

echo -e "${GREEN}🎉 Catnip development environment is ready!${NC}"
echo -e "${BLUE}📚 Quick start:${NC}"
echo "  • Run frontend dev server: pnpm dev"
echo "  • Run with Cloudflare: pnpm dev:cf"
echo "  • Build Go server: cd container && just build"
echo "  • Run tests: cd container && just test"
echo ""
echo -e "${YELLOW}💡 The pre-commit hook will automatically format changed files on commit${NC}"