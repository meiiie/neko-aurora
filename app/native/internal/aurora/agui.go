// Portions of this file are derived from:
//
//  1. AG-UI protocol, sdks/typescript/packages/core/src/generated/types.ts
//     (EventType enum), https://github.com/ag-ui-protocol/ag-ui
//     commit 8aa86867fb2918001dca1c64db3976cd37f50703
//  2. OpenMuse, packages/domain/src/agent.ts (TaskStatus / TaskStep / RunEvent),
//     https://github.com/CopilotKit/openmuse
//     commit 1ac68f3909f2478ab6280883f1ab5ea65eb5719d
//
// Both are MIT licensed; see THIRD_PARTY_NOTICES.md at the repository root.
//
// MIT License
//
// Copyright (c) 2025 (AG-UI contributors)
// Copyright (c) 2026 OpenMuse contributors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package aurora

// EventType is the AG-UI event discriminator (translated from the TS enum).
type EventType string

const (
	TEXT_MESSAGE_START        EventType = "TEXT_MESSAGE_START"
	TEXT_MESSAGE_CONTENT      EventType = "TEXT_MESSAGE_CONTENT"
	TEXT_MESSAGE_END          EventType = "TEXT_MESSAGE_END"
	TEXT_MESSAGE_CHUNK        EventType = "TEXT_MESSAGE_CHUNK"
	TOOL_CALL_START           EventType = "TOOL_CALL_START"
	TOOL_CALL_ARGS            EventType = "TOOL_CALL_ARGS"
	TOOL_CALL_END             EventType = "TOOL_CALL_END"
	TOOL_CALL_CHUNK           EventType = "TOOL_CALL_CHUNK"
	TOOL_CALL_RESULT          EventType = "TOOL_CALL_RESULT"
	STATE_SNAPSHOT            EventType = "STATE_SNAPSHOT"
	STATE_DELTA               EventType = "STATE_DELTA"
	MESSAGES_SNAPSHOT         EventType = "MESSAGES_SNAPSHOT"
	ACTIVITY_SNAPSHOT         EventType = "ACTIVITY_SNAPSHOT"
	ACTIVITY_DELTA            EventType = "ACTIVITY_DELTA"
	RAW                       EventType = "RAW"
	CUSTOM                    EventType = "CUSTOM"
	RUN_STARTED               EventType = "RUN_STARTED"
	RUN_FINISHED              EventType = "RUN_FINISHED"
	RUN_ERROR                 EventType = "RUN_ERROR"
	STEP_STARTED              EventType = "STEP_STARTED"
	STEP_FINISHED             EventType = "STEP_FINISHED"
	REASONING_START           EventType = "REASONING_START"
	REASONING_MESSAGE_START   EventType = "REASONING_MESSAGE_START"
	REASONING_MESSAGE_CONTENT EventType = "REASONING_MESSAGE_CONTENT"
	REASONING_MESSAGE_END     EventType = "REASONING_MESSAGE_END"
	REASONING_MESSAGE_CHUNK   EventType = "REASONING_MESSAGE_CHUNK"
	REASONING_END             EventType = "REASONING_END"
	REASONING_ENCRYPTED_VALUE EventType = "REASONING_ENCRYPTED_VALUE"
	SUBAGENT_STARTED          EventType = "SUBAGENT_STARTED"
	SUBAGENT_FINISHED         EventType = "SUBAGENT_FINISHED"
	SUBAGENT_ERROR            EventType = "SUBAGENT_ERROR"
)

// TaskStatus mirrors OpenMuse's task lifecycle; mapped from ledger Work status.
type TaskStatus string

const (
	TaskQueued          TaskStatus = "queued"
	TaskRunning         TaskStatus = "running"
	TaskWaitingApproval TaskStatus = "waiting_approval"
	TaskWaitingInput    TaskStatus = "waiting_input"
	TaskScheduled       TaskStatus = "scheduled"
	TaskPaused          TaskStatus = "paused"
	TaskSucceeded       TaskStatus = "succeeded"
	TaskFailed          TaskStatus = "failed"
	TaskCancelled       TaskStatus = "cancelled"
)

// RunEvent follows OpenMuse's RunEvent shape (kind/title/detail per task).
type RunEvent struct {
	ID     string `json:"id"`
	TaskID string `json:"taskId"`
	Date   string `json:"date"`
	Kind   string `json:"kind"` // plan|step|observation|approval|result|error|status
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// AGUIEvent is the wire envelope for AG-UI events on the Aurora SSE stream.
type AGUIEvent struct {
	Type      EventType `json:"type"`
	ThreadID  string    `json:"threadId,omitempty"`
	RunID     string    `json:"runId,omitempty"`
	MessageID string    `json:"messageId,omitempty"`
	StepName  string    `json:"stepName,omitempty"`
	Delta     string    `json:"delta,omitempty"`
	Message   string    `json:"message,omitempty"`
}
