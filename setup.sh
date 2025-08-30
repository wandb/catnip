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
            echo "  • Install via corepack: corepack enable && corepack prepare pnpm@latest --activate"
            echo "  • Install via curl: curl -fsSL https://get.pnpm.io/install.sh | sh -"
            echo "  • Install via Homebrew: brew install pnpm"
            echo "  • More options: https://pnpm.io/installation"
            ;;
        "just")
            echo "  • Install via npm: npm install -g just-install"
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

# Function to attempt automatic installation
try_auto_install() {
    local tool=$1
    echo -e "${YELLOW}🔧 Attempting to auto-install $tool...${NC}"
    
    case $tool in
        "pnpm")
            # Try corepack first (available in Node.js 16.13+)
            if command_exists corepack; then
                echo "Enabling pnpm via corepack..."
                if corepack enable && corepack prepare pnpm@latest --activate; then
                    echo -e "${GREEN}✅ pnpm installed via corepack${NC}"
                    return 0
                fi
            fi
            # Fall back to npm
            if command_exists npm; then
                echo "Installing pnpm via npm..."
                if npm install -g pnpm; then
                    echo -e "${GREEN}✅ pnpm installed via npm${NC}"
                    return 0
                fi
            fi
            ;;
        "just")
            # Try npm installation
            if command_exists npm; then
                echo "Installing just via npm..."
                if npm install -g just-install && command_exists just; then
                    echo -e "${GREEN}✅ just installed via npm${NC}"
                    return 0
                fi
            fi
            ;;
    esac
    
    return 1
}

# Check required dependencies
missing_deps=()

echo -e "${YELLOW}🔍 Checking dependencies...${NC}"

if ! command_exists node; then
    install_instructions "node"
    missing_deps+=("node")
fi

if ! command_exists pnpm; then
    if ! try_auto_install "pnpm"; then
        install_instructions "pnpm"
        missing_deps+=("pnpm")
    fi
fi

if ! command_exists go; then
    install_instructions "go"
    missing_deps+=("go")
fi

if ! command_exists just; then
    if ! try_auto_install "just"; then
        install_instructions "just"
        missing_deps+=("just")
    fi
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
if pnpm install --frozen-lockfile; then
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

# Track files that were formatted
formatted_files_list=""

# Get list of staged TypeScript/JavaScript files before formatting
staged_ts_files=$(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(ts|tsx|js|jsx)$' || true)

# Format TypeScript/JavaScript files
echo "Formatting TypeScript/JavaScript files..."
if pnpm format:changed 2>/dev/null; then
    # Check if any of the staged files have unstaged changes after formatting
    if [ -n "$staged_ts_files" ]; then
        formatted_ts_files=""
        for file in $staged_ts_files; do
            if git diff --name-only | grep -q "^$file$"; then
                formatted_ts_files="$formatted_ts_files $file"
            fi
        done
        if [ -n "$formatted_ts_files" ]; then
            formatted_files_list="$formatted_files_list$formatted_ts_files"
            echo -e "${GREEN}✅ TypeScript/JavaScript files formatted${NC}"
        else
            echo -e "${GREEN}✅ No TypeScript/JavaScript files needed formatting${NC}"
        fi
    else
        echo -e "${GREEN}✅ No TypeScript/JavaScript files to format${NC}"
    fi
else
    echo -e "${RED}❌ Failed to format TypeScript/JavaScript files${NC}"
    echo -e "${RED}💥 Commit blocked - fix formatting issues and try again${NC}"
    exit 1
fi

# Check if there are any changes in the src directory
src_changes=$(git diff --cached --name-only --diff-filter=ACM | grep '^src/' || true)

if [ -n "$src_changes" ]; then
    # Run TypeScript lint checks
    echo "Running TypeScript lint checks..."
    if pnpm lint >/dev/null 2>&1; then
        echo -e "${GREEN}✅ TypeScript lint checks passed${NC}"
    else
        echo -e "${RED}❌ TypeScript lint checks failed${NC}"
        echo -e "${YELLOW}Run 'pnpm lint' to see details${NC}"
        echo -e "${RED}💥 Commit blocked - fix lint issues and try again${NC}"
        exit 1
    fi

    # Run TypeScript type checks
    echo "Running TypeScript type checks..."
    if pnpm typecheck >/dev/null 2>&1; then
        echo -e "${GREEN}✅ TypeScript type checks passed${NC}"
    else
        echo -e "${RED}❌ TypeScript type checks failed${NC}"
        echo -e "${YELLOW}Run 'pnpm typecheck' to see details${NC}"
        echo -e "${RED}💥 Commit blocked - fix type errors and try again${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✅ No src files changed, skipping TypeScript lint and type checks${NC}"
fi

# Check if there are any changes in the container directory
container_changes=$(git diff --cached --name-only --diff-filter=ACM | grep '^container/' || true)

if [ -n "$container_changes" ]; then
    # Get list of staged Go files before formatting
    staged_go_files=$(echo "$container_changes" | grep '\.go$' || true)
    
    # Format Go files
    echo "Formatting Go files..."
    cd container
    if just format-go-changed 2>/dev/null; then
        # Check if any of the staged Go files have unstaged changes after formatting
        if [ -n "$staged_go_files" ]; then
            formatted_go_files=""
            for file in $staged_go_files; do
                # Remove container/ prefix for checking
                file_without_prefix=${file#container/}
                if git diff --name-only | grep -q "^$file_without_prefix$"; then
                    formatted_go_files="$formatted_go_files $file"
                fi
            done
            if [ -n "$formatted_go_files" ]; then
                formatted_files_list="$formatted_files_list$formatted_go_files"
                echo -e "${GREEN}✅ Go files formatted${NC}"
            else
                echo -e "${GREEN}✅ No Go files needed formatting${NC}"
            fi
        else
            echo -e "${GREEN}✅ No Go files to format${NC}"
        fi
    else
        echo -e "${RED}❌ Failed to format Go files${NC}"
        echo -e "${RED}💥 Commit blocked - fix Go formatting issues and try again${NC}"
        exit 1
    fi

    # Run Go lint checks
    echo "Running Go lint checks..."
    if just lint >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Go lint checks passed${NC}"
    else
        echo -e "${RED}❌ Go lint checks failed${NC}"
        echo -e "${YELLOW}Run 'cd container && just lint' to see details${NC}"
        echo -e "${RED}💥 Commit blocked - fix Go lint issues and try again${NC}"
        exit 1
    fi
    cd ..
else
    echo -e "${GREEN}✅ No Go files changed, skipping Go formatting and lint checks${NC}"
fi

# If files were formatted, add only those specific files to staging
if [ -n "$formatted_files_list" ]; then
    echo -e "${YELLOW}📝 Files were formatted. ${formatted_files_list}${NC}"
    exit 1
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