import { createFileRoute } from "@tanstack/react-router";
import { Layout } from "lucide-react";
import { SessionPlayground } from "@/components/SessionPlayground";

// Main playground page component
function PlaygroundPage() {
  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-2 mb-6">
        <Layout size={24} />
        <h1 className="text-3xl font-bold">Playground</h1>
      </div>

      {/* Session cards grid */}
      <SessionPlayground />
    </div>
  );
}

export const Route = createFileRoute("/playground")({
  component: PlaygroundPage,
});