import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  ChevronDown,
  ChevronRight,
  Code,
  ExternalLink,
  Copy,
} from "lucide-react";
import { TextContent } from "@/components/TextContent";
import "prismjs/themes/prism-tomorrow.css";

interface OpenAPISpec {
  openapi?: string;
  swagger?: string;
  info: {
    title: string;
    version: string;
    description?: string;
  };
  host?: string;
  basePath?: string;
  schemes?: string[];
  paths: Record<string, Record<string, any>>;
  definitions?: Record<string, any>;
}

interface APIEndpoint {
  path: string;
  method: string;
  summary?: string;
  description?: string;
  tags?: string[];
  parameters?: any[];
  responses?: Record<string, any>;
}

interface APIModel {
  name: string;
  description?: string;
  properties: Record<
    string,
    {
      type: string;
      description?: string;
      example?: any;
      format?: string;
      items?: any;
      $ref?: string;
    }
  >;
}

// Syntax highlighted code block component
function CodeBlock({
  code,
  language = "json",
}: {
  code: string;
  language?: string;
}) {
  const [copied, setCopied] = useState(false);
  const codeRef = useRef<HTMLElement>(null);

  useEffect(() => {
    const loadPrism = async () => {
      if (codeRef.current) {
        const Prism = (await import("prismjs")).default;
        // @ts-expect-error - prismjs components don't have type declarations
        await import("prismjs/components/prism-json");
        Prism.highlightElement(codeRef.current);
      }
    };

    void loadPrism();
  }, [code]);

  const copyToClipboard = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy code:", err);
    }
  };

  return (
    <div className="relative">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-muted-foreground">
          Example Response
        </span>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 px-2"
          onClick={copyToClipboard}
        >
          <Copy className="w-3 h-3 mr-1" />
          {copied ? "Copied!" : "Copy"}
        </Button>
      </div>
      <pre className="bg-gray-900 text-gray-100 p-4 rounded-lg overflow-x-auto text-sm border">
        <code ref={codeRef} className={`language-${language}`}>
          {code}
        </code>
      </pre>
    </div>
  );
}

// Generate example response from schema
function generateExampleFromSchema(
  schema: any,
  definitions: Record<string, any> = {},
  visited = new Set(),
): any {
  if (!schema) return null;

  // Handle $ref references
  if (schema.$ref) {
    const refKey = schema.$ref.replace("#/definitions/", "");
    if (visited.has(refKey)) {
      return `[Circular reference to ${refKey}]`;
    }
    visited.add(refKey);
    const refSchema = definitions[refKey];
    if (refSchema) {
      const result = generateExampleFromSchema(refSchema, definitions, visited);
      visited.delete(refKey);
      return result;
    }
    return null;
  }

  // Handle arrays
  if (schema.type === "array") {
    if (schema.items) {
      const itemExample = generateExampleFromSchema(
        schema.items,
        definitions,
        visited,
      );
      return [itemExample];
    }
    return [];
  }

  // Handle objects
  if (schema.type === "object" || schema.properties) {
    const obj: any = {};

    // Handle additionalProperties (like maps)
    if (schema.additionalProperties && !schema.properties) {
      if (schema.additionalProperties.$ref) {
        const example = generateExampleFromSchema(
          schema.additionalProperties,
          definitions,
          visited,
        );
        return {
          "example-key": example,
        };
      }
      return {};
    }

    // Handle regular properties
    if (schema.properties) {
      Object.entries(schema.properties).forEach(
        ([key, propSchema]: [string, any]) => {
          obj[key] = generateExampleFromSchema(
            propSchema,
            definitions,
            visited,
          );
        },
      );
    }

    return obj;
  }

  // Handle primitives with examples
  if (schema.example !== undefined) {
    return schema.example;
  }

  // Handle primitives by type
  switch (schema.type) {
    case "string":
      if (schema.format === "date-time") return "2024-01-15T14:30:00Z";
      if (schema.format === "date") return "2024-01-15";
      if (schema.format === "email") return "user@example.com";
      if (schema.format === "uri") return "https://example.com";
      return "string";
    case "integer":
    case "number":
      return 42;
    case "boolean":
      return true;
    default:
      return null;
  }
}

function APIExplorer() {
  const [spec, setSpec] = useState<OpenAPISpec | null>(null);
  const [endpoints, setEndpoints] = useState<APIEndpoint[]>([]);
  const [models, setModels] = useState<APIModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedEndpoints, setExpandedEndpoints] = useState<Set<string>>(
    new Set(),
  );
  const [expandedModels, setExpandedModels] = useState<Set<string>>(new Set());
  const [selectedTag, setSelectedTag] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<"endpoints" | "models">(
    "endpoints",
  );

  useEffect(() => {
    void fetchOpenAPISpec();
  }, []);

  useEffect(() => {
    // Handle initial URL hash on page load
    const hash = window.location.hash;
    if (hash.startsWith("#model-")) {
      const modelName = hash.replace("#model-", "");
      if (models.length > 0) {
        navigateToModelInternal(modelName, false); // Don't update URL since we're reading from it
      }
    }
  }, [models]);

  useEffect(() => {
    // Handle browser back/forward navigation
    const handlePopState = () => {
      const hash = window.location.hash;
      if (hash.startsWith("#model-")) {
        const modelName = hash.replace("#model-", "");
        navigateToModelInternal(modelName, false);
      } else {
        // If no hash, go back to endpoints
        setActiveTab("endpoints");
        setSelectedTag(null);
      }
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const fetchOpenAPISpec = async () => {
    try {
      const response = await fetch("/swagger/doc.json");
      if (!response.ok) {
        throw new Error(`Failed to fetch OpenAPI spec: ${response.statusText}`);
      }

      const data: OpenAPISpec = await response.json();
      setSpec(data);

      // Parse endpoints
      const endpointsList: APIEndpoint[] = [];
      Object.entries(data.paths).forEach(([path, methods]) => {
        Object.entries(methods).forEach(([method, details]) => {
          endpointsList.push({
            path,
            method: method.toLowerCase(),
            ...details,
          });
        });
      });

      setEndpoints(endpointsList);

      // Parse models from definitions
      const modelsList: APIModel[] = [];
      if (data.definitions) {
        Object.entries(data.definitions).forEach(([key, definition]) => {
          // Clean up the ugly model names
          const cleanName = key
            .replace(/^github_com_vanpelt_catnip_internal_models\./, "")
            .replace(/^internal_handlers\./, "");

          modelsList.push({
            name: cleanName,
            description: definition.description,
            properties: definition.properties || {},
          });
        });
      }

      setModels(modelsList);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  };

  const toggleEndpoint = (key: string) => {
    const newExpanded = new Set(expandedEndpoints);
    if (newExpanded.has(key)) {
      newExpanded.delete(key);
    } else {
      newExpanded.add(key);
    }
    setExpandedEndpoints(newExpanded);
  };

  const toggleModel = (key: string) => {
    const newExpanded = new Set(expandedModels);
    if (newExpanded.has(key)) {
      newExpanded.delete(key);
    } else {
      newExpanded.add(key);
    }
    setExpandedModels(newExpanded);
  };

  const getMethodColor = (method: string) => {
    switch (method.toLowerCase()) {
      case "get":
        return "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200";
      case "post":
        return "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200";
      case "put":
        return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200";
      case "delete":
        return "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200";
      case "patch":
        return "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200";
      default:
        return "bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200";
    }
  };

  const getAllTags = () => {
    const tags = new Set<string>();
    endpoints.forEach((endpoint) => {
      endpoint.tags?.forEach((tag) => tags.add(tag));
    });
    return Array.from(tags).sort();
  };

  const filteredEndpoints = selectedTag
    ? endpoints.filter((endpoint) => endpoint.tags?.includes(selectedTag))
    : endpoints;

  const navigateToModelInternal = (
    modelName: string,
    updateUrl: boolean = true,
  ) => {
    setActiveTab("models");
    setSelectedTag(null);
    // Expand the model
    const newExpanded = new Set(expandedModels);
    newExpanded.add(modelName);
    setExpandedModels(newExpanded);

    // Update URL if requested
    if (updateUrl) {
      const newUrl = `${window.location.pathname}${window.location.search}#model-${modelName}`;
      window.history.pushState({ modelName }, "", newUrl);
    }

    // Scroll to the model after a short delay to ensure it's rendered
    setTimeout(() => {
      const element = document.getElementById(`model-${modelName}`);
      if (element) {
        // Get the scrollable container (the main content area)
        const scrollContainer =
          element.closest(".overflow-auto") || document.documentElement;

        // Get element position relative to the scroll container
        const containerRect = scrollContainer.getBoundingClientRect();
        const elementRect = element.getBoundingClientRect();

        // Calculate the target scroll position with buffer
        const bufferSpace = 12; // 12px buffer for nice spacing
        const targetPosition =
          elementRect.top -
          containerRect.top +
          scrollContainer.scrollTop -
          bufferSpace;

        // Scroll with smooth animation
        scrollContainer.scrollTo({
          top: Math.max(0, targetPosition),
          behavior: "smooth",
        });
      }
    }, 150); // Slightly longer delay to ensure DOM is fully updated
  };

  const navigateToModel = (modelName: string) => {
    navigateToModelInternal(modelName, true);
  };

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-muted-foreground">
          Loading API documentation...
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex items-center justify-center">
        <Card className="w-96">
          <CardHeader>
            <CardTitle className="text-red-600">Error</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground">{error}</p>
            <Button onClick={fetchOpenAPISpec} className="mt-4">
              Retry
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="h-full overflow-hidden flex flex-col">
      {/* Header */}
      <div className="border-b bg-card" data-docs-header>
        <div className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold">
                {spec?.info.title || "API Explorer"}
              </h1>
              <div className="text-muted-foreground mt-1">
                <TextContent
                  content={
                    spec?.info.description ||
                    "Explore and test the API endpoints"
                  }
                />
              </div>
              {spec && (
                <div className="flex items-center gap-2 mt-2">
                  <Badge variant="outline">v{spec.info.version}</Badge>
                  {spec.host && <Badge variant="outline">{spec.host}</Badge>}
                </div>
              )}
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" asChild>
                <a href="/swagger/" target="_blank" rel="noopener noreferrer">
                  <ExternalLink className="w-4 h-4 mr-2" />
                  Swagger UI
                </a>
              </Button>
            </div>
          </div>

          {/* Tabs */}
          <div className="flex gap-4 mt-4 border-b">
            <button
              className={`pb-2 px-1 border-b-2 transition-colors ${
                activeTab === "endpoints"
                  ? "border-primary text-primary font-medium"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
              onClick={() => {
                setActiveTab("endpoints");
                setSelectedTag(null);
                // Clear URL hash when returning to endpoints
                const newUrl = `${window.location.pathname}${window.location.search}`;
                window.history.pushState({}, "", newUrl);
              }}
            >
              API Endpoints ({endpoints.length})
            </button>
            <button
              className={`pb-2 px-1 border-b-2 transition-colors ${
                activeTab === "models"
                  ? "border-primary text-primary font-medium"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
              onClick={() => {
                setActiveTab("models");
                // Clear URL hash when manually going to models tab
                const newUrl = `${window.location.pathname}${window.location.search}`;
                window.history.pushState({}, "", newUrl);
              }}
            >
              Data Models ({models.length})
            </button>
          </div>

          {/* Tag Filter - Only show for endpoints */}
          {activeTab === "endpoints" && getAllTags().length > 0 && (
            <div className="flex flex-wrap gap-2 mt-4">
              <Button
                variant={selectedTag === null ? "default" : "outline"}
                size="sm"
                onClick={() => setSelectedTag(null)}
              >
                All
              </Button>
              {getAllTags().map((tag) => (
                <Button
                  key={tag}
                  variant={selectedTag === tag ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSelectedTag(tag)}
                >
                  {tag}
                </Button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6">
        {activeTab === "endpoints" ? (
          <div className="space-y-3">
            {filteredEndpoints.map((endpoint, index) => {
              const key = `${endpoint.method}-${endpoint.path}-${index}`;
              const isExpanded = expandedEndpoints.has(key);

              return (
                <Card key={key} className="overflow-hidden">
                  <CardHeader
                    className="pb-3 cursor-pointer hover:bg-muted/50 transition-colors"
                    onClick={() => toggleEndpoint(key)}
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <Badge className={getMethodColor(endpoint.method)}>
                          {endpoint.method.toUpperCase()}
                        </Badge>
                        <code className="text-sm font-mono">
                          {endpoint.path}
                        </code>
                        {isExpanded ? (
                          <ChevronDown className="w-4 h-4" />
                        ) : (
                          <ChevronRight className="w-4 h-4" />
                        )}
                      </div>
                      {endpoint.tags && (
                        <div className="flex gap-1">
                          {endpoint.tags.map((tag) => (
                            <Badge
                              key={tag}
                              variant="secondary"
                              className="text-xs"
                            >
                              {tag}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>
                    {endpoint.summary && (
                      <p className="text-sm text-muted-foreground mt-2">
                        {endpoint.summary}
                      </p>
                    )}
                  </CardHeader>

                  {isExpanded && (
                    <CardContent className="pt-0">
                      {endpoint.description && (
                        <div className="mb-4">
                          <h4 className="text-sm font-medium mb-2">
                            Description
                          </h4>
                          <div className="text-sm text-muted-foreground">
                            <TextContent content={endpoint.description} />
                          </div>
                        </div>
                      )}

                      {endpoint.parameters &&
                        endpoint.parameters.length > 0 && (
                          <div className="mb-4">
                            <h4 className="text-sm font-medium mb-2">
                              Parameters
                            </h4>
                            <div className="space-y-2">
                              {endpoint.parameters.map((param, idx) => (
                                <div
                                  key={idx}
                                  className="p-2 bg-muted rounded-md"
                                >
                                  <div className="flex items-center gap-2">
                                    <code className="text-xs font-mono">
                                      {param.name}
                                    </code>
                                    <Badge
                                      variant="outline"
                                      className="text-xs"
                                    >
                                      {param.in}
                                    </Badge>
                                    {param.required && (
                                      <Badge
                                        variant="destructive"
                                        className="text-xs"
                                      >
                                        required
                                      </Badge>
                                    )}
                                  </div>
                                  {param.description && (
                                    <div className="text-xs text-muted-foreground mt-1">
                                      <TextContent
                                        content={param.description}
                                      />
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                      {endpoint.responses && (
                        <div>
                          <h4 className="text-sm font-medium mb-2">
                            Responses
                          </h4>
                          <div className="space-y-2">
                            {Object.entries(endpoint.responses).map(
                              ([code, response]) => (
                                <div
                                  key={code}
                                  className="p-2 bg-muted rounded-md"
                                >
                                  <div className="flex items-center gap-2 mb-2">
                                    <Badge
                                      variant={
                                        code.startsWith("2")
                                          ? "default"
                                          : "destructive"
                                      }
                                      className="text-xs"
                                    >
                                      {code}
                                    </Badge>
                                    <div className="text-sm">
                                      <TextContent
                                        content={response.description}
                                      />
                                    </div>
                                  </div>
                                  {response.schema && (
                                    <div className="mt-2 pl-4 border-l-2 border-muted-foreground/20">
                                      <div className="text-xs font-medium text-muted-foreground mb-1">
                                        Response Schema:
                                      </div>
                                      {response.schema.$ref ? (
                                        <div className="flex items-center gap-2 mb-3">
                                          <Badge
                                            variant="outline"
                                            className="text-xs bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300 cursor-pointer hover:bg-blue-100 dark:hover:bg-blue-900 transition-colors"
                                            onClick={() =>
                                              navigateToModel(
                                                response.schema.$ref
                                                  .replace("#/definitions/", "")
                                                  .replace(
                                                    /^github_com_vanpelt_catnip_internal_models\./,
                                                    "",
                                                  )
                                                  .replace(
                                                    /^internal_handlers\./,
                                                    "",
                                                  ),
                                              )
                                            }
                                          >
                                            <Code className="w-3 h-3 mr-1" />
                                            {response.schema.$ref
                                              .replace("#/definitions/", "")
                                              .replace(
                                                /^github_com_vanpelt_catnip_internal_models\./,
                                                "",
                                              )
                                              .replace(
                                                /^internal_handlers\./,
                                                "",
                                              )}
                                          </Badge>
                                        </div>
                                      ) : response.schema.type === "array" &&
                                        response.schema.items?.$ref ? (
                                        <div className="flex items-center gap-2 mb-3">
                                          <span className="text-xs text-muted-foreground">
                                            Array of:
                                          </span>
                                          <Badge
                                            variant="outline"
                                            className="text-xs bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300 cursor-pointer hover:bg-blue-100 dark:hover:bg-blue-900 transition-colors"
                                            onClick={() =>
                                              navigateToModel(
                                                response.schema.items.$ref
                                                  .replace("#/definitions/", "")
                                                  .replace(
                                                    /^github_com_vanpelt_catnip_internal_models\./,
                                                    "",
                                                  )
                                                  .replace(
                                                    /^internal_handlers\./,
                                                    "",
                                                  ),
                                              )
                                            }
                                          >
                                            <Code className="w-3 h-3 mr-1" />
                                            {response.schema.items.$ref
                                              .replace("#/definitions/", "")
                                              .replace(
                                                /^github_com_vanpelt_catnip_internal_models\./,
                                                "",
                                              )
                                              .replace(
                                                /^internal_handlers\./,
                                                "",
                                              )}
                                          </Badge>
                                        </div>
                                      ) : response.schema.type === "object" &&
                                        response.schema.additionalProperties
                                          ?.$ref ? (
                                        <div className="flex items-center gap-2 mb-3">
                                          <span className="text-xs text-muted-foreground">
                                            Map of:
                                          </span>
                                          <Badge
                                            variant="outline"
                                            className="text-xs bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300 cursor-pointer hover:bg-blue-100 dark:hover:bg-blue-900 transition-colors"
                                            onClick={() =>
                                              navigateToModel(
                                                response.schema.additionalProperties.$ref
                                                  .replace("#/definitions/", "")
                                                  .replace(
                                                    /^github_com_vanpelt_catnip_internal_models\./,
                                                    "",
                                                  )
                                                  .replace(
                                                    /^internal_handlers\./,
                                                    "",
                                                  ),
                                              )
                                            }
                                          >
                                            <Code className="w-3 h-3 mr-1" />
                                            {response.schema.additionalProperties.$ref
                                              .replace("#/definitions/", "")
                                              .replace(
                                                /^github_com_vanpelt_catnip_internal_models\./,
                                                "",
                                              )
                                              .replace(
                                                /^internal_handlers\./,
                                                "",
                                              )}
                                          </Badge>
                                        </div>
                                      ) : (
                                        <div className="mb-3">
                                          <Badge
                                            variant="outline"
                                            className="text-xs"
                                          >
                                            {response.schema.type || "object"}
                                          </Badge>
                                        </div>
                                      )}

                                      {/* Generate and display example response */}
                                      {code.startsWith("2") &&
                                        spec?.definitions &&
                                        (() => {
                                          const exampleData =
                                            generateExampleFromSchema(
                                              response.schema,
                                              spec.definitions,
                                            );
                                          if (exampleData) {
                                            return (
                                              <div className="mt-3">
                                                <CodeBlock
                                                  code={JSON.stringify(
                                                    exampleData,
                                                    null,
                                                    2,
                                                  )}
                                                />
                                              </div>
                                            );
                                          }
                                          return null;
                                        })()}
                                    </div>
                                  )}
                                </div>
                              ),
                            )}
                          </div>
                        </div>
                      )}
                    </CardContent>
                  )}
                </Card>
              );
            })}
          </div>
        ) : (
          // Models Tab
          <div className="space-y-4">
            {models.map((model) => {
              const isExpanded = expandedModels.has(model.name);

              return (
                <Card
                  key={model.name}
                  id={`model-${model.name}`}
                  className="overflow-hidden"
                >
                  <CardHeader
                    className="pb-3 cursor-pointer hover:bg-muted/50 transition-colors"
                    onClick={() => toggleModel(model.name)}
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <Badge
                          variant="outline"
                          className="bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
                        >
                          MODEL
                        </Badge>
                        <code className="text-sm font-mono font-semibold">
                          {model.name}
                        </code>
                        {isExpanded ? (
                          <ChevronDown className="w-4 h-4" />
                        ) : (
                          <ChevronRight className="w-4 h-4" />
                        )}
                      </div>
                    </div>
                    {model.description && (
                      <div className="text-sm text-muted-foreground mt-2">
                        <TextContent content={model.description} />
                      </div>
                    )}
                  </CardHeader>

                  {isExpanded && (
                    <CardContent className="pt-0">
                      <h4 className="text-sm font-medium mb-3">Properties</h4>
                      <div className="space-y-3">
                        {Object.entries(model.properties).map(
                          ([propName, prop]) => (
                            <div
                              key={propName}
                              className="p-3 bg-muted rounded-md"
                            >
                              <div className="flex items-center gap-2 mb-1">
                                <code className="text-sm font-mono font-medium">
                                  {propName}
                                </code>
                                <Badge variant="secondary" className="text-xs">
                                  {prop.type}
                                  {prop.type === "array" &&
                                    prop.items?.type &&
                                    ` of ${prop.items.type}`}
                                  {prop.type === "array" &&
                                    prop.items?.$ref &&
                                    ` of ${prop.items.$ref
                                      .replace("#/definitions/", "")
                                      .replace(
                                        /^github_com_vanpelt_catnip_internal_models\./,
                                        "",
                                      )
                                      .replace(/^internal_handlers\./, "")}`}
                                </Badge>
                                {prop.example && (
                                  <Badge variant="outline" className="text-xs">
                                    example: {JSON.stringify(prop.example)}
                                  </Badge>
                                )}
                              </div>
                              {prop.description && (
                                <div className="text-xs text-muted-foreground mt-1">
                                  <TextContent content={prop.description} />
                                </div>
                              )}
                              {prop.$ref && (
                                <div className="mt-1 flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground">
                                    References:
                                  </span>
                                  <Badge
                                    variant="outline"
                                    className="text-xs bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300 cursor-pointer hover:bg-blue-100 dark:hover:bg-blue-900 transition-colors"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      if (prop.$ref) {
                                        navigateToModel(
                                          prop.$ref
                                            .replace("#/definitions/", "")
                                            .replace(
                                              /^github_com_vanpelt_catnip_internal_models\./,
                                              "",
                                            )
                                            .replace(
                                              /^internal_handlers\./,
                                              "",
                                            ),
                                        );
                                      }
                                    }}
                                  >
                                    <Code className="w-3 h-3 mr-1" />
                                    {prop.$ref
                                      ?.replace("#/definitions/", "")
                                      ?.replace(
                                        /^github_com_vanpelt_catnip_internal_models\./,
                                        "",
                                      )
                                      ?.replace(/^internal_handlers\./, "")}
                                  </Badge>
                                </div>
                              )}
                            </div>
                          ),
                        )}
                      </div>
                    </CardContent>
                  )}
                </Card>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

export const Route = createFileRoute("/docs")({
  component: APIExplorer,
});
