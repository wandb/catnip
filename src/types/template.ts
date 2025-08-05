export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  category: "frontend" | "backend" | "fullstack" | "other";
  language: string;
  framework?: string;
  icon?: string;
  files: TemplateFile[];
}

export interface TemplateFile {
  path: string;
  content: string;
  binary?: boolean;
}
