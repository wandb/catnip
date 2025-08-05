import type { ProjectTemplate } from "@/types/template";

export const projectTemplates: ProjectTemplate[] = [
  {
    id: "react-vite",
    name: "React + Vite",
    description: "Modern React app with Vite, TypeScript, and Tailwind CSS",
    category: "frontend",
    language: "TypeScript",
    framework: "React",
    icon: "⚛️",
    files: [
      {
        path: "package.json",
        content: JSON.stringify(
          {
            name: "my-react-app",
            private: true,
            version: "0.0.0",
            type: "module",
            scripts: {
              dev: "vite",
              build: "tsc && vite build",
              preview: "vite preview",
            },
            dependencies: {
              react: "^18.2.0",
              "react-dom": "^18.2.0",
            },
            devDependencies: {
              "@types/react": "^18.2.0",
              "@types/react-dom": "^18.2.0",
              "@vitejs/plugin-react": "^4.2.0",
              autoprefixer: "^10.4.16",
              postcss: "^8.4.32",
              tailwindcss: "^3.4.0",
              typescript: "^5.3.0",
              vite: "^5.0.0",
            },
          },
          null,
          2,
        ),
      },
      {
        path: "tsconfig.json",
        content: JSON.stringify(
          {
            compilerOptions: {
              target: "ES2020",
              useDefineForClassFields: true,
              lib: ["ES2020", "DOM", "DOM.Iterable"],
              module: "ESNext",
              skipLibCheck: true,
              moduleResolution: "bundler",
              allowImportingTsExtensions: true,
              resolveJsonModule: true,
              isolatedModules: true,
              noEmit: true,
              jsx: "react-jsx",
              strict: true,
              noUnusedLocals: true,
              noUnusedParameters: true,
              noFallthroughCasesInSwitch: true,
            },
            include: ["src"],
            references: [{ path: "./tsconfig.node.json" }],
          },
          null,
          2,
        ),
      },
      {
        path: "vite.config.ts",
        content: `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
})`,
      },
      {
        path: "tailwind.config.js",
        content: `/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}`,
      },
      {
        path: "postcss.config.js",
        content: `export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}`,
      },
      {
        path: "index.html",
        content: `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>React + Vite App</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>`,
      },
      {
        path: "src/main.tsx",
        content: `import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)`,
      },
      {
        path: "src/App.tsx",
        content: `import { useState } from 'react'
import './App.css'

function App() {
  const [count, setCount] = useState(0)

  return (
    <div className="min-h-screen bg-gray-100 dark:bg-gray-900 flex items-center justify-center">
      <div className="text-center">
        <h1 className="text-4xl font-bold text-gray-900 dark:text-white mb-8">
          Vite + React
        </h1>
        <div className="space-y-4">
          <button
            onClick={() => setCount((count) => count + 1)}
            className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
          >
            count is {count}
          </button>
          <p className="text-gray-600 dark:text-gray-400">
            Edit <code className="bg-gray-200 dark:bg-gray-800 px-1 rounded">src/App.tsx</code> and save to test HMR
          </p>
        </div>
      </div>
    </div>
  )
}

export default App`,
      },
      {
        path: "src/index.css",
        content: `@tailwind base;
@tailwind components;
@tailwind utilities;`,
      },
      {
        path: "src/App.css",
        content: ``,
      },
      {
        path: ".gitignore",
        content: `# Logs
logs
*.log
npm-debug.log*
yarn-debug.log*
yarn-error.log*
pnpm-debug.log*
lerna-debug.log*

node_modules
dist
dist-ssr
*.local

# Editor directories and files
.vscode/*
!.vscode/extensions.json
.idea
.DS_Store
*.suo
*.ntvs*
*.njsproj
*.sln
*.sw?`,
      },
      {
        path: "README.md",
        content: `# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

## Getting Started

\`\`\`bash
npm install
npm run dev
\`\`\`

## Available Scripts

- \`npm run dev\` - Start the development server
- \`npm run build\` - Build for production
- \`npm run preview\` - Preview the production build locally`,
      },
    ],
  },
  {
    id: "node-express",
    name: "Node.js + Express API",
    description: "RESTful API with Express, TypeScript, and basic middleware",
    category: "backend",
    language: "TypeScript",
    framework: "Express",
    icon: "🚀",
    files: [
      {
        path: "package.json",
        content: JSON.stringify(
          {
            name: "node-express-api",
            version: "1.0.0",
            description: "Express API with TypeScript",
            main: "dist/index.js",
            scripts: {
              dev: "nodemon",
              build: "tsc",
              start: "node dist/index.js",
            },
            dependencies: {
              express: "^4.18.2",
              cors: "^2.8.5",
              dotenv: "^16.3.1",
            },
            devDependencies: {
              "@types/express": "^4.17.21",
              "@types/node": "^20.10.0",
              "@types/cors": "^2.8.17",
              nodemon: "^3.0.2",
              "ts-node": "^10.9.2",
              typescript: "^5.3.0",
            },
          },
          null,
          2,
        ),
      },
      {
        path: "tsconfig.json",
        content: JSON.stringify(
          {
            compilerOptions: {
              target: "ES2020",
              module: "commonjs",
              lib: ["ES2020"],
              outDir: "./dist",
              rootDir: "./src",
              strict: true,
              esModuleInterop: true,
              skipLibCheck: true,
              forceConsistentCasingInFileNames: true,
              resolveJsonModule: true,
              moduleResolution: "node",
            },
            include: ["src/**/*"],
            exclude: ["node_modules", "dist"],
          },
          null,
          2,
        ),
      },
      {
        path: "nodemon.json",
        content: JSON.stringify(
          {
            watch: ["src"],
            ext: "ts",
            exec: "ts-node src/index.ts",
          },
          null,
          2,
        ),
      },
      {
        path: "src/index.ts",
        content: `import express, { Request, Response } from 'express';
import cors from 'cors';
import dotenv from 'dotenv';

// Load environment variables
dotenv.config();

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// Routes
app.get('/', (req: Request, res: Response) => {
  res.json({
    message: 'Welcome to Express TypeScript API',
    timestamp: new Date().toISOString()
  });
});

app.get('/api/health', (req: Request, res: Response) => {
  res.json({
    status: 'ok',
    uptime: process.uptime(),
    timestamp: new Date().toISOString()
  });
});

// Start server
app.listen(PORT, () => {
  console.log(\`Server is running on port \${PORT}\`);
});`,
      },
      {
        path: ".env.example",
        content: `PORT=3000
NODE_ENV=development`,
      },
      {
        path: ".gitignore",
        content: `node_modules/
dist/
.env
.DS_Store
*.log
coverage/
.vscode/
.idea/`,
      },
      {
        path: "README.md",
        content: `# Express TypeScript API

A simple REST API built with Express and TypeScript.

## Getting Started

\`\`\`bash
# Install dependencies
npm install

# Copy environment variables
cp .env.example .env

# Run in development mode
npm run dev

# Build for production
npm run build

# Run production build
npm start
\`\`\`

## API Endpoints

- \`GET /\` - Welcome message
- \`GET /api/health\` - Health check endpoint`,
      },
    ],
  },
  {
    id: "vue-vite",
    name: "Vue 3 + Vite",
    description: "Vue 3 app with Composition API, TypeScript, and Tailwind CSS",
    category: "frontend",
    language: "TypeScript",
    framework: "Vue",
    icon: "💚",
    files: [
      {
        path: "package.json",
        content: JSON.stringify(
          {
            name: "vue-vite-app",
            private: true,
            version: "0.0.0",
            type: "module",
            scripts: {
              dev: "vite",
              build: "vue-tsc && vite build",
              preview: "vite preview",
            },
            dependencies: {
              vue: "^3.3.11",
            },
            devDependencies: {
              "@vitejs/plugin-vue": "^4.5.2",
              autoprefixer: "^10.4.16",
              postcss: "^8.4.32",
              tailwindcss: "^3.4.0",
              typescript: "^5.3.0",
              vite: "^5.0.8",
              "vue-tsc": "^1.8.25",
            },
          },
          null,
          2,
        ),
      },
      {
        path: "tsconfig.json",
        content: JSON.stringify(
          {
            compilerOptions: {
              target: "ES2020",
              useDefineForClassFields: true,
              module: "ESNext",
              lib: ["ES2020", "DOM", "DOM.Iterable"],
              skipLibCheck: true,
              moduleResolution: "node",
              allowImportingTsExtensions: true,
              resolveJsonModule: true,
              isolatedModules: true,
              noEmit: true,
              jsx: "preserve",
              strict: true,
              noUnusedLocals: true,
              noUnusedParameters: true,
              noFallthroughCasesInSwitch: true,
            },
            include: [
              "src/**/*.ts",
              "src/**/*.d.ts",
              "src/**/*.tsx",
              "src/**/*.vue",
            ],
            references: [{ path: "./tsconfig.node.json" }],
          },
          null,
          2,
        ),
      },
      {
        path: "vite.config.ts",
        content: `import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
})`,
      },
      {
        path: "tailwind.config.js",
        content: `/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}`,
      },
      {
        path: "index.html",
        content: `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Vue + Vite App</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>`,
      },
      {
        path: "src/main.ts",
        content: `import { createApp } from 'vue'
import './style.css'
import App from './App.vue'

createApp(App).mount('#app')`,
      },
      {
        path: "src/App.vue",
        content: `<script setup lang="ts">
import { ref } from 'vue'

const count = ref(0)
</script>

<template>
  <div class="min-h-screen bg-gray-100 dark:bg-gray-900 flex items-center justify-center">
    <div class="text-center">
      <h1 class="text-4xl font-bold text-gray-900 dark:text-white mb-8">
        Vite + Vue
      </h1>
      <div class="space-y-4">
        <button
          @click="count++"
          class="bg-green-500 hover:bg-green-700 text-white font-bold py-2 px-4 rounded"
        >
          count is {{ count }}
        </button>
        <p class="text-gray-600 dark:text-gray-400">
          Edit <code class="bg-gray-200 dark:bg-gray-800 px-1 rounded">src/App.vue</code> and save to test HMR
        </p>
      </div>
    </div>
  </div>
</template>`,
      },
      {
        path: "src/style.css",
        content: `@tailwind base;
@tailwind components;
@tailwind utilities;`,
      },
      {
        path: ".gitignore",
        content: `# Logs
logs
*.log
npm-debug.log*
yarn-debug.log*
yarn-error.log*
pnpm-debug.log*
lerna-debug.log*

node_modules
dist
dist-ssr
*.local

# Editor directories and files
.vscode/*
!.vscode/extensions.json
.idea
.DS_Store
*.suo
*.ntvs*
*.njsproj
*.sln
*.sw?`,
      },
    ],
  },
  {
    id: "python-fastapi",
    name: "Python FastAPI",
    description:
      "Modern Python API with FastAPI, async support, and auto-documentation",
    category: "backend",
    language: "Python",
    framework: "FastAPI",
    icon: "🐍",
    files: [
      {
        path: "requirements.txt",
        content: `fastapi==0.104.1
uvicorn[standard]==0.24.0
pydantic==2.5.0
python-dotenv==1.0.0`,
      },
      {
        path: "main.py",
        content: `from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import Optional
from datetime import datetime
import os
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

app = FastAPI(
    title="FastAPI Template",
    description="A simple FastAPI template with basic endpoints",
    version="1.0.0"
)

# Configure CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Models
class HealthResponse(BaseModel):
    status: str
    timestamp: datetime
    version: str

class MessageResponse(BaseModel):
    message: str
    timestamp: datetime

# Routes
@app.get("/")
async def root():
    return MessageResponse(
        message="Welcome to FastAPI!",
        timestamp=datetime.now()
    )

@app.get("/health", response_model=HealthResponse)
async def health_check():
    return HealthResponse(
        status="healthy",
        timestamp=datetime.now(),
        version="1.0.0"
    )

@app.get("/api/items/{item_id}")
async def read_item(item_id: int, q: Optional[str] = None):
    return {"item_id": item_id, "q": q}

if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("PORT", 8000))
    uvicorn.run("main:app", host="0.0.0.0", port=port, reload=True)`,
      },
      {
        path: ".env.example",
        content: `PORT=8000`,
      },
      {
        path: ".gitignore",
        content: `# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python
env/
venv/
ENV/
.venv
*.egg-info/
dist/
build/

# Environment
.env

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db`,
      },
      {
        path: "README.md",
        content: `# FastAPI Template

A modern Python API built with FastAPI.

## Getting Started

\`\`\`bash
# Create virtual environment
python -m venv venv

# Activate virtual environment
# On Windows: venv\\Scripts\\activate
# On macOS/Linux: source venv/bin/activate

# Install dependencies
pip install -r requirements.txt

# Copy environment variables
cp .env.example .env

# Run the application
python main.py
\`\`\`

## API Documentation

Once running, visit:
- Interactive API docs: http://localhost:8000/docs
- Alternative API docs: http://localhost:8000/redoc

## Endpoints

- \`GET /\` - Welcome message
- \`GET /health\` - Health check
- \`GET /api/items/{item_id}\` - Get item by ID`,
      },
    ],
  },
  {
    id: "nextjs-app",
    name: "Next.js App",
    description:
      "Full-stack React framework with App Router, TypeScript, and Tailwind CSS",
    category: "fullstack",
    language: "TypeScript",
    framework: "Next.js",
    icon: "▲",
    files: [
      {
        path: "package.json",
        content: JSON.stringify(
          {
            name: "nextjs-app",
            version: "0.1.0",
            private: true,
            scripts: {
              dev: "next dev",
              build: "next build",
              start: "next start",
              lint: "next lint",
            },
            dependencies: {
              next: "14.0.4",
              react: "^18",
              "react-dom": "^18",
            },
            devDependencies: {
              "@types/node": "^20",
              "@types/react": "^18",
              "@types/react-dom": "^18",
              autoprefixer: "^10.0.1",
              eslint: "^8",
              "eslint-config-next": "14.0.4",
              postcss: "^8",
              tailwindcss: "^3.3.0",
              typescript: "^5",
            },
          },
          null,
          2,
        ),
      },
      {
        path: "tsconfig.json",
        content: JSON.stringify(
          {
            compilerOptions: {
              target: "es5",
              lib: ["dom", "dom.iterable", "esnext"],
              allowJs: true,
              skipLibCheck: true,
              strict: true,
              noEmit: true,
              esModuleInterop: true,
              module: "esnext",
              moduleResolution: "bundler",
              resolveJsonModule: true,
              isolatedModules: true,
              jsx: "preserve",
              incremental: true,
              plugins: [
                {
                  name: "next",
                },
              ],
              paths: {
                "@/*": ["./src/*"],
              },
            },
            include: [
              "next-env.d.ts",
              "**/*.ts",
              "**/*.tsx",
              ".next/types/**/*.ts",
            ],
            exclude: ["node_modules"],
          },
          null,
          2,
        ),
      },
      {
        path: "next.config.js",
        content: `/** @type {import('next').NextConfig} */
const nextConfig = {}

module.exports = nextConfig`,
      },
      {
        path: "tailwind.config.ts",
        content: `import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './src/pages/**/*.{js,ts,jsx,tsx,mdx}',
    './src/components/**/*.{js,ts,jsx,tsx,mdx}',
    './src/app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
export default config`,
      },
      {
        path: "src/app/layout.tsx",
        content: `import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'

const inter = Inter({ subsets: ['latin'] })

export const metadata: Metadata = {
  title: 'Next.js App',
  description: 'Created with Next.js App Router',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body className={inter.className}>{children}</body>
    </html>
  )
}`,
      },
      {
        path: "src/app/page.tsx",
        content: `export default function Home() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-24">
      <div className="z-10 max-w-5xl w-full items-center justify-between font-mono text-sm">
        <h1 className="text-4xl font-bold text-center mb-8">
          Welcome to Next.js!
        </h1>
        <p className="text-center text-gray-600 dark:text-gray-400">
          Get started by editing{' '}
          <code className="font-mono font-bold">src/app/page.tsx</code>
        </p>
      </div>
    </main>
  )
}`,
      },
      {
        path: "src/app/globals.css",
        content: `@tailwind base;
@tailwind components;
@tailwind utilities;`,
      },
      {
        path: ".gitignore",
        content: `# See https://help.github.com/articles/ignoring-files/ for more about ignoring files.

# dependencies
/node_modules
/.pnp
.pnp.js
.yarn/install-state.gz

# testing
/coverage

# next.js
/.next/
/out/

# production
/build

# misc
.DS_Store
*.pem

# debug
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# local env files
.env*.local

# vercel
.vercel

# typescript
*.tsbuildinfo
next-env.d.ts`,
      },
    ],
  },
];

export function getTemplateById(id: string): ProjectTemplate | undefined {
  return projectTemplates.find((template) => template.id === id);
}

export function getTemplatesByCategory(
  category: ProjectTemplate["category"],
): ProjectTemplate[] {
  return projectTemplates.filter((template) => template.category === category);
}
