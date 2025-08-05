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
  },
  {
    id: "node-express",
    name: "Node.js + Express API",
    description: "RESTful API with Express, TypeScript, and basic middleware",
    category: "backend",
    language: "TypeScript",
    framework: "Express",
    icon: "🚀",
  },
  {
    id: "vue-vite",
    name: "Vue 3 + Vite",
    description: "Vue 3 app with Composition API, TypeScript, and Tailwind CSS",
    category: "frontend",
    language: "TypeScript",
    framework: "Vue",
    icon: "💚",
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
