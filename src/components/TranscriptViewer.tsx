import { useState, useEffect } from "react";
import type {
  ParsedTranscript,
  TranscriptSession,
  TranscriptMessage as TranscriptMessageType,
} from "../lib/transcript-types";
import { parseTranscript } from "../lib/transcript-utils";
import { TranscriptMessage } from "./TranscriptMessage";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";
import { ErrorDisplay } from "./ErrorDisplay";

interface TranscriptViewerProps {
  sessionId?: string;
  transcriptData?: TranscriptSession;
}

interface ProcessedMessage extends TranscriptMessageType {
  aggregatedToolMessages?: TranscriptMessageType[];
}

// Function to extract text content from a message
function getTextContent(message: TranscriptMessageType): string {
  if (!message.message?.content) return "";
  
  if (typeof message.message.content === "string") {
    return message.message.content;
  }
  
  if (Array.isArray(message.message.content)) {
    return message.message.content
      .filter(item => item.type === "text")
      .map(item => item.text || "")
      .join("\n");
  }
  
  return "";
}

// Function to check if message contains only tool calls
function hasOnlyToolCalls(message: TranscriptMessageType): boolean {
  if (message.type !== "assistant" || !message.message?.content || !Array.isArray(message.message.content)) {
    return false;
  }
  
  const hasText = message.message.content.some(item => item.type === "text" && item.text?.trim());
  const hasTools = message.message.content.some(item => item.type === "tool_use");
  
  return !hasText && hasTools;
}

// Preprocess messages to group tool-only messages with their summaries
function preprocessMessages(messages: TranscriptMessageType[]): ProcessedMessage[] {
  const processed: ProcessedMessage[] = [];
  const toolOnlyMessages: TranscriptMessageType[] = [];
  
  for (let i = 0; i < messages.length; i++) {
    const message = messages[i];
    
    // Skip user messages that only contain tool results
    if (
      message.type === "user" &&
      message.message?.content &&
      Array.isArray(message.message.content) &&
      message.message.content.every(item => item.type === "tool_result")
    ) {
      continue;
    }
    
    if (message.type === "assistant") {
      if (hasOnlyToolCalls(message)) {
        // This is a tool-only message, collect it
        toolOnlyMessages.push(message);
      } else {
        // This message has text content
        const textContent = getTextContent(message);
        if (textContent.trim() && toolOnlyMessages.length > 0) {
          // This is a summary message following tool calls
          processed.push({
            ...message,
            aggregatedToolMessages: [...toolOnlyMessages]
          });
          toolOnlyMessages.length = 0; // Clear the array
        } else {
          // Regular message with text
          processed.push(message);
        }
      }
    } else {
      // Non-assistant message
      // If we have pending tool messages, add them as standalone
      if (toolOnlyMessages.length > 0) {
        processed.push(...toolOnlyMessages);
        toolOnlyMessages.length = 0;
      }
      processed.push(message);
    }
  }
  
  // Add any remaining tool-only messages
  if (toolOnlyMessages.length > 0) {
    processed.push(...toolOnlyMessages);
  }
  
  return processed;
}

export function TranscriptViewer({
  sessionId,
  transcriptData,
}: TranscriptViewerProps) {
  const [data, setData] = useState<TranscriptSession | null>(
    transcriptData || null
  );
  const [parsedTranscript, setParsedTranscript] =
    useState<ParsedTranscript | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (sessionId && !transcriptData) {
      fetchTranscriptData(sessionId);
    } else if (transcriptData) {
      setData(transcriptData);
    }
  }, [sessionId, transcriptData]);

  useEffect(() => {
    if (data) {
      const parsed = parseTranscript(data);
      setParsedTranscript(parsed);
    }
  }, [data]);

  const fetchTranscriptData = async (id: string) => {
    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`/v1/claude/session/${id}`);
      if (!response.ok) {
        throw new Error(`Failed to fetch transcript: ${response.statusText}`);
      }

      const transcriptData = await response.json();
      
      // Convert the transcript data to the expected format
      const sessionData: TranscriptSession = {
        sessionId: transcriptData.sessionId,
        messages: transcriptData.messages || [],
        startTime: transcriptData.startTime,
        endTime: transcriptData.endTime,
      };
      
      setData(sessionData);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to fetch transcript"
      );
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <Card className="w-full max-w-4xl mx-auto">
        <CardContent className="p-6">
          <div className="flex items-center justify-center">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-2">Loading transcript...</span>
          </div>
        </CardContent>
      </Card>
    );
  }

  const handleRetry = () => {
    if (sessionId) {
      fetchTranscriptData(sessionId);
    }
  };

  if (error) {
    return (
      <ErrorDisplay
        title="Failed to Load Transcript"
        message={error}
        onRetry={sessionId ? handleRetry : undefined}
        retryLabel="Try Again"
      />
    );
  }

  if (!parsedTranscript) {
    return (
      <Card className="w-full max-w-4xl mx-auto">
        <CardContent className="p-6">
          <div className="text-muted-foreground">
            No transcript data available
          </div>
        </CardContent>
      </Card>
    );
  }

  const { messages, messageTree } = parsedTranscript;
  const processedMessages = preprocessMessages(messages);
  
  // Find the model from the first assistant message
  const model = messages.find(msg => msg.type === 'assistant' && msg.message?.model)?.message?.model;

  return (
    <div className="w-full max-w-4xl mx-auto space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            Claude Session Transcript
            {sessionId && (
              <Badge variant="secondary" className="font-mono text-xs">
                {sessionId}
              </Badge>
            )}
            {model && (
              <Badge variant="outline" className="text-xs">
                {model}
              </Badge>
            )}
          </CardTitle>
          <div className="text-sm text-muted-foreground">
            {messages.length} messages • Session started{" "}
            {new Date(messages[0]?.timestamp).toLocaleString()}
          </div>
        </CardHeader>
        <CardContent className="px-6 py-4">
          <div className="space-y-4">
            {processedMessages.map((message) => (
              <TranscriptMessage
                key={message.uuid}
                message={message}
                allMessages={messages}
                messageTree={messageTree}
                depth={0}
                isChronological={true}
              />
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}