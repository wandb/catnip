import { TranscriptViewer } from './TranscriptViewer'
import type { TranscriptSession } from '../lib/transcript-types'

const mockTranscriptData: TranscriptSession = {
  messages: [
    {
      cwd: "/workspace/catnip/dockerize-quasar",
      isMeta: false,
      isSidechain: false,
      message: {
        content: [
          {
            text: "I'll create a developer-friendly README that gets people excited about Catnip's power for agentic programming!",
            type: "text"
          }
        ],
        id: "msg_01Jt1HwfGkffeU5rrTDUAW8c",
        model: "claude-sonnet-4-20250514",
        role: "assistant",
        stop_reason: null,
        stop_sequence: null,
        type: "message",
        usage: {
          cache_creation_input_tokens: 20143,
          cache_read_input_tokens: 0,
          input_tokens: 2,
          output_tokens: 3,
          service_tier: "standard"
        }
      },
      parentUuid: "a5b42c7a-ab20-4b44-a0de-ab81b93f149a",
      sessionId: "5dd5fb40-6571-4cf3-a846-4e02a9c6dcad",
      timestamp: "2025-07-10T23:08:24.376Z",
      type: "assistant",
      userType: "external",
      uuid: "8e100ebd-3f5a-4c0e-926a-53ff7666d154",
      version: "1.0.43"
    },
    {
      cwd: "/workspace/catnip/dockerize-quasar",
      isMeta: false,
      isSidechain: false,
      message: {
        content: [
          {
            id: "toolu_016Yx7coUrkJxJLbgk9FfCJ3",
            input: {
              todos: [
                {
                  content: "Read current README.md to understand existing content",
                  id: "1",
                  priority: "high",
                  status: "pending"
                },
                {
                  content: "Create compelling, developer-friendly README with features showcase",
                  id: "2",
                  priority: "high",
                  status: "pending"
                }
              ]
            },
            name: "TodoWrite",
            type: "tool_use"
          }
        ],
        id: "msg_01Jt1HwfGkffeU5rrTDUAW8c",
        model: "claude-sonnet-4-20250514",
        role: "assistant",
        stop_reason: null,
        stop_sequence: null,
        type: "message",
        usage: {
          cache_creation_input_tokens: 20143,
          cache_read_input_tokens: 0,
          input_tokens: 2,
          output_tokens: 3,
          service_tier: "standard"
        }
      },
      parentUuid: "8e100ebd-3f5a-4c0e-926a-53ff7666d154",
      sessionId: "5dd5fb40-6571-4cf3-a846-4e02a9c6dcad",
      timestamp: "2025-07-10T23:08:26.736Z",
      type: "assistant",
      userType: "external",
      uuid: "7b5eca97-313f-419c-b35a-884052480d07",
      version: "1.0.43"
    },
    {
      cwd: "/workspace/catnip/dockerize-quasar",
      isMeta: false,
      isSidechain: false,
      content: [
        {
          content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress. Please proceed with the current tasks if applicable",
          tool_use_id: "toolu_016Yx7coUrkJxJLbgk9FfCJ3",
          type: "tool_result"
        }
      ],
      parentUuid: "7b5eca97-313f-419c-b35a-884052480d07",
      sessionId: "5dd5fb40-6571-4cf3-a846-4e02a9c6dcad",
      timestamp: "2025-07-10T23:08:26.797Z",
      type: "user",
      userType: "external",
      uuid: "60bc5b47-d0c1-4db6-a90a-37b74d3cf292",
      version: "1.0.43"
    }
  ],
  userPrompts: [
    {
      display: "Nice, now let's make a developer friendly README, our current one is very sparse. We should get people HYPED about the power of Catnip to manage agentic programming.",
      pastedContents: {}
    }
  ]
}

export function TranscriptExample() {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h2 className="text-2xl font-bold mb-2">Transcript Viewer Demo</h2>
        <p className="text-muted-foreground">
          This demonstrates how Claude transcripts are rendered with tool calls, results, and threading.
        </p>
      </div>
      
      <TranscriptViewer transcriptData={mockTranscriptData} />
    </div>
  )
}