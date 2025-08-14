export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  category: "frontend" | "backend" | "fullstack" | "basic";
  language: string;
  framework?: string;
  icon?: string;
}
