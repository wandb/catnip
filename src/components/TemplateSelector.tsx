import { useState } from "react";
import { projectTemplates } from "@/lib/templates";
import type { ProjectTemplate } from "@/types/template";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ArrowRight, Code2, Server, Layers, Package } from "lucide-react";

interface TemplateSelectorProps {
  onSelectTemplate: (template: ProjectTemplate) => void;
}

const categoryIcons = {
  frontend: <Code2 className="w-4 h-4" />,
  backend: <Server className="w-4 h-4" />,
  fullstack: <Layers className="w-4 h-4" />,
  basic: <Package className="w-4 h-4" />,
};

export function TemplateSelector({ onSelectTemplate }: TemplateSelectorProps) {
  const [selectedCategory, setSelectedCategory] =
    useState<ProjectTemplate["category"]>("basic");

  const categories: ProjectTemplate["category"][] = [
    "basic",
    "frontend",
    "backend",
    "fullstack",
  ];
  const filteredTemplates = projectTemplates.filter(
    (t) => t.category === selectedCategory,
  );

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-semibold mb-2">Choose a Template</h3>
        <p className="text-sm text-muted-foreground">
          Start your new project with a pre-configured template
        </p>
      </div>

      <Tabs
        value={selectedCategory}
        onValueChange={(v) =>
          setSelectedCategory(v as ProjectTemplate["category"])
        }
      >
        <TabsList className="grid w-full grid-cols-4">
          {categories.map((category) => (
            <TabsTrigger key={category} value={category} className="capitalize">
              <span className="flex items-center gap-2">
                {categoryIcons[category]}
                {category}
              </span>
            </TabsTrigger>
          ))}
        </TabsList>

        {categories.map((category) => (
          <TabsContent key={category} value={category} className="mt-4">
            <div className="grid gap-4 md:grid-cols-2">
              {filteredTemplates.map((template) => (
                <Card
                  key={template.id}
                  className="cursor-pointer hover:border-primary transition-colors"
                  onClick={() => onSelectTemplate(template)}
                >
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-2">
                        <span className="text-2xl">{template.icon}</span>
                        <CardTitle className="text-base">
                          {template.name}
                        </CardTitle>
                      </div>
                      <ArrowRight className="w-4 h-4 text-muted-foreground" />
                    </div>
                  </CardHeader>
                  <CardContent>
                    <CardDescription className="mb-3">
                      {template.description}
                    </CardDescription>
                    <div className="flex gap-2">
                      <Badge variant="secondary">{template.language}</Badge>
                      {template.framework && (
                        <Badge variant="outline">{template.framework}</Badge>
                      )}
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}
