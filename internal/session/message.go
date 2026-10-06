package session

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrMessageNotFound is returned when a message ID is not in the session.
var ErrMessageNotFound = errors.New("message not found")

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type PartType string

const (
	PartText       PartType = "text"
	PartToolCall   PartType = "tool-call"
	PartToolResult PartType = "tool-result"
	PartReasoning  PartType = "reasoning"
	PartFile       PartType = "file"
	PartImage      PartType = "image"
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
	ImageData  string   `json:"imageData,omitempty"`
	MediaType  string   `json:"mediaType,omitempty"`
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
	Input     int        `json:"input"`
	Output    int        `json:"output"`
	Reasoning int        `json:"reasoning"`
	Cache     CacheUsage `json:"cache"`
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

func ImagePart(data, mediaType string) Part {
	return Part{Type: PartImage, ImageData: data, MediaType: mediaType}
}

// StoredPart represents a message part as persisted in the part table.
type StoredPart struct {
	ID        string   `json:"id"`
	MessageID string   `json:"messageID"`
	SessionID string   `json:"sessionID"`
	Type      string   `json:"type"`
	Text      string   `json:"text,omitempty"`
	Time      PartTime `json:"time"`
}

// PartTime tracks when a part started and ended streaming.
type PartTime struct {
	Start int64 `json:"start"`
	End   int64 `json:"end,omitempty"`
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

// ReplaceAll atomically replaces all messages for a session with msgs.
// Used after compaction so the durable history matches the in-memory compacted list.
func (ms *MessageStore) ReplaceAll(sessionID string, msgs []Message) error {
	tx, err := ms.store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM message WHERE session_id = ?", sessionID); err != nil {
		return err
	}

	for i := range msgs {
		msg := msgs[i]
		msg.SessionID = sessionID
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
		if nowMs == 0 {
			nowMs = time.Now().UnixMilli()
		}
		if _, err := tx.Exec(
			`INSERT INTO message (id, session_id, time_created, time_updated, data)
			 VALUES (?, ?, ?, ?, ?)`,
			msg.ID, sessionID, nowMs, nowMs, string(dataJSON),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
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

func (ms *MessageStore) DeleteByID(messageID string) error {
	_, err := ms.store.db.Exec("DELETE FROM message WHERE id = ?", messageID)
	return err
}

// DeleteAfterTime deletes all messages in a session created after the given
// timestamp (in milliseconds). Returns the number of deleted messages.
func (ms *MessageStore) DeleteAfterTime(sessionID string, afterMs int64) (int64, error) {
	result, err := ms.store.db.Exec(
		"DELETE FROM message WHERE session_id = ? AND time_created > ?",
		sessionID, afterMs,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteAfter deletes all messages in a session that appear after messageID
// in creation order. The target message itself is kept. Returns the number
// of deleted messages. Deletes run in a single transaction.
func (ms *MessageStore) DeleteAfter(sessionID, messageID string) (int64, error) {
	messages, err := ms.List(sessionID)
	if err != nil {
		return 0, err
	}

	idx := -1
	for i, m := range messages {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, ErrMessageNotFound
	}

	toDelete := messages[idx+1:]
	if len(toDelete) == 0 {
		return 0, nil
	}

	tx, err := ms.store.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning delete transaction: %w", err)
	}
	defer tx.Rollback()

	var deleted int64
	for _, m := range toDelete {
		if _, err := tx.Exec("DELETE FROM message WHERE id = ?", m.ID); err != nil {
			return 0, fmt.Errorf("deleting message %s: %w", m.ID, err)
		}
		deleted++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing delete transaction: %w", err)
	}
	return deleted, nil
}

func (ms *MessageStore) Count(sessionID string) (int, error) {
	var count int
	err := ms.store.db.QueryRow(
		"SELECT COUNT(*) FROM message WHERE session_id = ?", sessionID,
	).Scan(&count)
	return count, err
}

// PartStore handles part persistence in the part table.
type PartStore struct {
	db *sql.DB
}

func NewPartStore(db *sql.DB) *PartStore {
	return &PartStore{db: db}
}

func (ps *PartStore) Save(part StoredPart) error {
	data, err := json.Marshal(part)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = ps.db.Exec(
		`INSERT OR REPLACE INTO part (id, message_id, session_id, time_created, time_updated, data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		part.ID, part.MessageID, part.SessionID, now, now, string(data),
	)
	return err
}

func (ps *PartStore) ListByMessage(messageID string) ([]StoredPart, error) {
	rows, err := ps.db.Query(
		`SELECT data FROM part WHERE message_id = ? ORDER BY time_created ASC`,
		messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parts []StoredPart
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			return nil, err
		}
		var p StoredPart
		if err := json.Unmarshal([]byte(dataJSON), &p); err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

func (ps *PartStore) DeleteByMessage(messageID string) error {
	_, err := ps.db.Exec("DELETE FROM part WHERE message_id = ?", messageID)
	return err
}
