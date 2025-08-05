package templates

import (
	"fmt"
	"os"
	"path/filepath"
)

// TemplateDefinition represents a template with its file structure
type TemplateDefinition struct {
	ID    string
	Files map[string]string // path -> content
	Dirs  []string          // directories to create
}

// GetSupportedTemplates returns the list of templates that need manual file setup
func GetSupportedTemplates() []string {
	return []string{"node-express", "python-fastapi"}
}

// GetTemplateDefinition returns the template definition for a given template ID
func GetTemplateDefinition(templateID string) (*TemplateDefinition, error) {
	switch templateID {
	case "node-express":
		return getNodeExpressTemplate(), nil
	case "python-fastapi":
		return getPythonFastAPITemplate(), nil
	default:
		return nil, fmt.Errorf("template %s does not require manual file setup", templateID)
	}
}

// SetupTemplateFiles creates the template files in the given project path
func SetupTemplateFiles(templateID, projectPath string) error {
	template, err := GetTemplateDefinition(templateID)
	if err != nil {
		return err
	}

	// Create directories
	for _, dir := range template.Dirs {
		dirPath := filepath.Join(projectPath, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create files
	for path, content := range template.Files {
		filePath := filepath.Join(projectPath, path)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create file %s: %w", path, err)
		}
	}

	return nil
}

func getNodeExpressTemplate() *TemplateDefinition {
	return &TemplateDefinition{
		ID:   "node-express",
		Dirs: []string{"src"},
		Files: map[string]string{
			"package.json": `{
  "name": "express-api",
  "version": "1.0.0",
  "description": "Express API with TypeScript",
  "main": "dist/index.js",
  "scripts": {
    "dev": "nodemon",
    "build": "tsc",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "express": "^4.18.2",
    "cors": "^2.8.5",
    "dotenv": "^16.3.1"
  },
  "devDependencies": {
    "@types/express": "^4.17.21",
    "@types/node": "^20.10.0",
    "@types/cors": "^2.8.17",
    "nodemon": "^3.0.2",
    "ts-node": "^10.9.2",
    "typescript": "^5.3.0"
  }
}`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "lib": ["ES2020"],
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "moduleResolution": "node"
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}`,
			"src/index.ts": `import express, { Request, Response } from 'express';
import cors from 'cors';
import dotenv from 'dotenv';

dotenv.config();

const app = express();
const PORT = process.env.PORT || 3000;

app.use(cors());
app.use(express.json());

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

app.listen(PORT, () => {
  console.log(` + "`Server is running on port ${PORT}`" + `);
});`,
			".gitignore": `node_modules/
dist/
.env
.DS_Store
*.log`,
		},
	}
}

func getPythonFastAPITemplate() *TemplateDefinition {
	return &TemplateDefinition{
		ID:   "python-fastapi",
		Dirs: []string{},
		Files: map[string]string{
			"requirements.txt": `fastapi==0.104.1
uvicorn[standard]==0.24.0
pydantic==2.5.0
python-dotenv==1.0.0`,
			"main.py": `from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from datetime import datetime
import os
from dotenv import load_dotenv

load_dotenv()

app = FastAPI(
    title="FastAPI Template",
    description="A simple FastAPI template with basic endpoints",
    version="1.0.0"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

class HealthResponse(BaseModel):
    status: str
    timestamp: datetime
    version: str

class MessageResponse(BaseModel):
    message: str
    timestamp: datetime

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

if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("PORT", 8000))
    uvicorn.run("main:app", host="0.0.0.0", port=port, reload=True)`,
			".gitignore": `__pycache__/
*.py[cod]
.env
venv/
.venv/
.DS_Store`,
		},
	}
}
