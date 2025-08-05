export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  category: "frontend" | "backend" | "fullstack" | "other";
  language: string;
  framework?: string;
  icon?: string;
}
