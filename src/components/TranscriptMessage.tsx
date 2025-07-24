import { useState } from "react";
import type { TranscriptMessage as TranscriptMessageType } from "../lib/transcript-types";
import { getChildMessages, formatTimestamp } from "../lib/transcript-utils";
import { MessageContent } from "./MessageContent";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";
import { ChevronDown, Code2 } from "lucide-react";
import { ToolCall } from "./ToolCall";

interface ExtendedTranscriptMessage extends TranscriptMessageType {
  aggregatedToolMessages?: TranscriptMessageType[];
}

interface TranscriptMessageProps {
  message: ExtendedTranscriptMessage;
  allMessages: TranscriptMessageType[];
  messageTree: Map<string, TranscriptMessageType[]>;
  depth: number;
  isChronological?: boolean;
}

interface ToolStats {
  totalCalls: number;
  totalInput: number;
  totalOutput: number;
  totalCached: number;
}

// Function to extract text content
function getTextContent(message: TranscriptMessageType): string {
  if (!message.message?.content) return "";

  if (typeof message.message.content === "string") {
    return message.message.content;
  }

  if (Array.isArray(message.message.content)) {
    return message.message.content
      .filter((item) => item.type === "text")
      .map((item) => item.text || "")
      .join("\n");
  }

  return "";
}

// Function to extract tool calls
function getToolCalls(message: TranscriptMessageType): any[] {
  if (!message.message?.content || !Array.isArray(message.message.content)) {
    return [];
  }

  return message.message.content.filter((item) => item.type === "tool_use");
}

// Function to find tool results for a given tool call
function findToolResult(
  toolId: string,
  allMessages: TranscriptMessageType[],
): { content: string; isError: boolean } | undefined {
  for (const msg of allMessages) {
    if (
      msg.type === "user" &&
      msg.message?.content &&
      Array.isArray(msg.message.content)
    ) {
      for (const item of msg.message.content) {
        if (item.type === "tool_result" && item.tool_use_id === toolId) {
          return {
            content: item.content || "",
            isError: item.is_error || false,
          };
        }
      }
    }
  }
  return undefined;
}

// Calculate aggregated tool stats
function calculateAggregatedStats(
  toolMessages: TranscriptMessageType[],
): ToolStats {
  let totalCalls = 0;
  let totalInput = 0;
  let totalOutput = 0;
  let totalCached = 0;

  for (const msg of toolMessages) {
    const calls = getToolCalls(msg);
    totalCalls += calls.length;

    if (msg.message?.usage) {
      totalInput += msg.message.usage.input_tokens || 0;
      totalOutput += msg.message.usage.output_tokens || 0;
      totalCached += msg.message.usage.cache_read_input_tokens || 0;
    }
  }

  return { totalCalls, totalInput, totalOutput, totalCached };
}

export function TranscriptMessage({
  message,
  allMessages,
  messageTree,
  depth,
  isChronological = false,
}: TranscriptMessageProps) {
  const [showToolCalls, setShowToolCalls] = useState(false);
  const childMessages = isChronological
    ? []
    : getChildMessages(message.uuid, messageTree);
  const isAssistant = message.type === "assistant";

  const textContent = getTextContent(message);
  const directToolCalls = getToolCalls(message);

  // Check if this message has aggregated tool messages
  const hasAggregatedTools =
    message.aggregatedToolMessages && message.aggregatedToolMessages.length > 0;
  const aggregatedStats = hasAggregatedTools
    ? calculateAggregatedStats(message.aggregatedToolMessages!)
    : null;

  // For messages that only contain tools (no aggregation), render them directly
  const hasOnlyDirectTools =
    isAssistant &&
    !textContent.trim() &&
    directToolCalls.length > 0 &&
    !hasAggregatedTools;

  return (
    <div
      className={cn("flex flex-col", depth > 0 && !isChronological && "ml-8")}
    >
      <div
        className={cn("flex", isAssistant ? "justify-start" : "justify-end")}
      >
        <div
          className={cn(
            "max-w-[80%] space-y-2",
            isAssistant ? "items-start" : "items-end",
          )}
        >
          {/* Message Header */}
          <div
            className={cn(
              "flex items-center gap-2 text-xs text-muted-foreground",
              isAssistant ? "justify-start" : "justify-end",
            )}
          >
            <span>{isAssistant ? "Claude" : "You"}</span>
            <span className="font-mono">
              {formatTimestamp(message.timestamp)}
            </span>
          </div>

          {/* Message Bubble */}
          {textContent && (
            <div
              className={cn(
                "px-4 py-3 rounded-2xl inline-block break-words shadow-sm",
                isAssistant
                  ? "bg-muted text-foreground rounded-tl-sm"
                  : "bg-[var(--imessage-blue)] text-white rounded-tr-sm",
              )}
            >
              <MessageContent
                message={message}
                allMessages={allMessages}
                showOnlyText={true}
                inheritTextColor={!isAssistant}
              />
            </div>
          )}

          {/* Usage Stats (when no tool calls) */}
          {!hasAggregatedTools &&
            !hasOnlyDirectTools &&
            message.message?.usage && (
              <div className="flex gap-2 text-xs text-muted-foreground">
                <span>In: {message.message.usage.input_tokens}</span>
                <span>Out: {message.message.usage.output_tokens}</span>
                {message.message.usage.cache_read_input_tokens && (
                  <span>
                    Cache: {message.message.usage.cache_read_input_tokens}
                  </span>
                )}
              </div>
            )}
        </div>
      </div>

      {/* Aggregated Tool Calls (appear under summary text with full width) */}
      {hasAggregatedTools && aggregatedStats && (
        <div className={cn("mt-2", isAssistant ? "pl-0" : "pl-[20%]")}>
          <div className="space-y-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowToolCalls(!showToolCalls)}
              className={cn(
                "h-auto py-1.5 px-3 text-xs text-muted-foreground hover:text-foreground",
                "flex items-center gap-2 bg-muted/50 hover:bg-muted rounded-lg w-auto",
              )}
            >
              <div className="flex items-center gap-2">
                <span className="flex items-center gap-1">
                  <Code2 className="h-3 w-3" />
                  {aggregatedStats.totalCalls} tool{" "}
                  {aggregatedStats.totalCalls === 1 ? "call" : "calls"}
                </span>
                <span className="text-muted-foreground/50">•</span>
                <span>{aggregatedStats.totalInput.toLocaleString()} input</span>
                <span className="text-muted-foreground/50">•</span>
                <span>
                  {aggregatedStats.totalOutput.toLocaleString()} output
                </span>
                {aggregatedStats.totalCached > 0 && (
                  <>
                    <span className="text-muted-foreground/50">•</span>
                    <span>
                      {aggregatedStats.totalCached.toLocaleString()} cached
                      tokens
                    </span>
                  </>
                )}
              </div>
              {showToolCalls ? (
                <ChevronDown className="h-3 w-3 ml-1 rotate-180" />
              ) : (
                <ChevronDown className="h-3 w-3 ml-1" />
              )}
            </Button>

            {/* Expandable Aggregated Tool Calls */}
            {showToolCalls && (
              <div className="space-y-2 mt-2">
                {message.aggregatedToolMessages!.map((toolMsg) => {
                  const msgToolCalls = getToolCalls(toolMsg);
                  return msgToolCalls.map((toolCall) => {
                    const result = findToolResult(
                      toolCall.id || "",
                      allMessages,
                    );
                    return (
                      <ToolCall
                        key={toolCall.id}
                        toolId={toolCall.id || ""}
                        toolName={toolCall.name || ""}
                        toolInput={toolCall.input || {}}
                        result={result}
                        defaultExpanded={true}
                        fullWidth={true}
                      />
                    );
                  });
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Direct tool calls (for messages with only tools and no aggregation) */}
      {hasOnlyDirectTools && (
        <div
          className={cn("flex", isAssistant ? "justify-start" : "justify-end")}
        >
          <div className="max-w-[80%] space-y-2">
            {directToolCalls.map((toolCall) => {
              const result = findToolResult(toolCall.id || "", allMessages);
              return (
                <ToolCall
                  key={toolCall.id}
                  toolId={toolCall.id || ""}
                  toolName={toolCall.name || ""}
                  toolInput={toolCall.input || {}}
                  result={result}
                  fullWidth={false}
                />
              );
            })}
          </div>
        </div>
      )}

      {/* Child Messages */}
      {childMessages.map((childMessage) => (
        <TranscriptMessage
          key={childMessage.uuid}
          message={childMessage}
          allMessages={allMessages}
          messageTree={messageTree}
          depth={depth + 1}
        />
      ))}
    </div>
  );
}
