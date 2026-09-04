package session

import (
	"encoding/json"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type PartType string

const (
	PartText      PartType = "text"
	PartToolCall  PartType = "tool-call"
	PartToolResult PartType = "tool-result"
	PartReasoning PartType = "reasoning"
	PartFile      PartType = "file"
)

type Part struct {
	Type       PartType `json:"type"`
	Text       string   `json:"text,omitempty"`
	ToolCallID string   `json:"toolCallID,omitempty"`
	ToolName   string   `json:"toolName,omitempty"`
	ToolArgs   string   `json:"toolArgs,omitempty"`
	ToolResult string   `json:"toolResult,omitempty"`
	ToolError  bool     `json:"toolError,omitempty"`
	FilePath   string   `json:"filePath,omitempty"`
	FileData   string   `json:"fileData,omitempty"`
}

type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionID"`
	Role      Role      `json:"role"`
	Parts     []Part    `json:"parts"`
	Model     string    `json:"model,omitempty"`
	Tokens    *MsgUsage `json:"tokens,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type MsgUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	Reasoning  int `json:"reasoning"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

func TextPart(text string) Part {
	return Part{Type: PartText, Text: text}
}

func ToolCallPart(id, name, args string) Part {
	return Part{
		Type:       PartToolCall,
		ToolCallID: id,
		ToolName:   name,
		ToolArgs:   args,
	}
}

func ToolResultPart(id, name, result string, isError bool) Part {
	return Part{
		Type:       PartToolResult,
		ToolCallID: id,
		ToolName:   name,
		ToolResult: result,
		ToolError:  isError,
	}
}

func ReasoningPart(text string) Part {
	return Part{Type: PartReasoning, Text: text}
}

type MessageStore struct {
	store *Store
}

func NewMessageStore(store *Store) *MessageStore {
	return &MessageStore{store: store}
}

type messageData struct {
	Role   Role      `json:"role"`
	Parts  []Part    `json:"parts"`
	Model  string    `json:"model,omitempty"`
	Tokens *MsgUsage `json:"tokens,omitempty"`
}

func (ms *MessageStore) Append(msg *Message) error {
	data := messageData{
		Role:   msg.Role,
		Parts:  msg.Parts,
		Model:  msg.Model,
		Tokens: msg.Tokens,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}

	nowMs := msg.CreatedAt.UnixMilli()
	_, err = ms.store.db.Exec(
		`INSERT INTO message (id, session_id, time_created, time_updated, data)
		 VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, nowMs, nowMs, string(dataJSON),
	)
	return err
}

func (ms *MessageStore) List(sessionID string) ([]Message, error) {
	rows, err := ms.store.db.Query(
		`SELECT id, session_id, time_created, data
		 FROM message WHERE session_id = ? ORDER BY time_created ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var dataJSON string
		var createdMs int64

		if err := rows.Scan(&m.ID, &m.SessionID, &createdMs, &dataJSON); err != nil {
			return nil, err
		}

		m.CreatedAt = time.UnixMilli(createdMs)

		var data messageData
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			return nil, err
		}
		m.Role = data.Role
		m.Parts = data.Parts
		m.Model = data.Model
		m.Tokens = data.Tokens

		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (ms *MessageStore) Delete(sessionID string) error {
	_, err := ms.store.db.Exec("DELETE FROM message WHERE session_id = ?", sessionID)
	return err
}

func (ms *MessageStore) Count(sessionID string) (int, error) {
	var count int
	err := ms.store.db.QueryRow(
		"SELECT COUNT(*) FROM message WHERE session_id = ?", sessionID,
	).Scan(&count)
	return count, err
}
