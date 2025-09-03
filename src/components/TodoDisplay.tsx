import { Badge } from "./ui/badge";
import { CheckCircle, Circle, Clock } from "lucide-react";
import { cn } from "../lib/utils";

interface Todo {
  id: string;
  content: string;
  status: "pending" | "in_progress" | "completed";
  priority?: "high" | "medium" | "low";
}

interface TodoDisplayProps {
  todos: Todo[];
}

const STATUS_ICONS = {
  completed: CheckCircle,
  in_progress: Clock,
  pending: Circle,
};

const STATUS_COLORS = {
  completed: "text-green-600 dark:text-green-400",
  in_progress: "text-blue-600 dark:text-blue-400",
  pending: "text-gray-400 dark:text-gray-500",
};

const PRIORITY_COLORS = {
  high: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300",
  medium:
    "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300",
  low: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
};

export function TodoDisplay({ todos }: TodoDisplayProps) {
  if (!todos || todos.length === 0) {
    return (
      <div className="text-xs text-muted-foreground">No todos available</div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="text-xs text-muted-foreground mb-2">
        {todos.length} todo{todos.length !== 1 ? "s" : ""}
      </div>

      {todos.map((todo, index) => {
        const StatusIcon = STATUS_ICONS[todo.status];
        const statusColor = STATUS_COLORS[todo.status];
        const priorityColor = todo.priority
          ? PRIORITY_COLORS[todo.priority]
          : PRIORITY_COLORS.low;

        // Use a combination of id, index, and content to ensure unique keys
        const uniqueKey =
          todo.id || `todo-${index}-${todo.content.slice(0, 20)}`;

        return (
          <div
            key={uniqueKey}
            className={cn(
              "flex items-start gap-2 p-2 rounded border text-xs",
              todo.status === "completed" && "opacity-75",
            )}
          >
            <StatusIcon
              className={cn("h-3 w-3 mt-0.5 flex-shrink-0", statusColor)}
            />

            <div className="flex-1 min-w-0">
              <div
                className={cn(
                  "break-words",
                  todo.status === "completed" &&
                    "line-through text-muted-foreground",
                )}
              >
                {todo.content}
              </div>

              <div className="flex items-center gap-2 mt-1">
                <Badge
                  variant="secondary"
                  className={cn("text-xs px-1 py-0", priorityColor)}
                >
                  {todo.priority || "low"}
                </Badge>

                <Badge variant="outline" className="text-xs px-1 py-0">
                  {todo.status.replace("_", " ")}
                </Badge>

                {todo.id && (
                  <span className="text-xs text-muted-foreground font-mono">
                    #{todo.id}
                  </span>
                )}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
