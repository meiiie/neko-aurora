package historyexport

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxMessages          = 500
	MaxOutputBytes       = 1 << 20
	MaxFilesPerMessage   = 64
	MaxIDBytes           = 128
	MaxTitleBytes        = 256
	MaxNameBytes         = 256
	MaxTextBytes         = 64 << 10
	MaxDownloadPathBytes = 512
	MaxFileBytes         = 1 << 30
)

type History struct {
	Version        int       `json:"version"`
	ConversationID string    `json:"conversationId"`
	Title          string    `json:"title"`
	BotID          string    `json:"botId"`
	Archived       bool      `json:"archived"`
	AtSequence     uint64    `json:"atSequence"`
	Messages       []Message `json:"messages"`
}

type Message struct {
	RequestID string `json:"requestId"`
	BotID     string `json:"botId"`
	CreatedAt string `json:"createdAt"`
	Text      string `json:"text"`
	Reply     string `json:"reply"`
	Status    string `json:"status"`
	Files     []File `json:"files"`
}

type File struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SHA256       string `json:"sha256"`
	Bytes        int    `json:"bytes"`
	DownloadPath string `json:"downloadPath"`
}

type ErrorKind string

const (
	InvalidHistory    ErrorKind = "invalid_history"
	UnsupportedFormat ErrorKind = "unsupported_format"
	LimitExceeded     ErrorKind = "limit_exceeded"
)

type Error struct {
	Kind  ErrorKind
	Field string
}

func (e *Error) Error() string {
	return "historyexport: " + string(e.Kind) + " (" + e.Field + ")"
}

func Encode(history History, format string) ([]byte, error) {
	if format != "json" && format != "text" {
		return nil, rejected(UnsupportedFormat, "format")
	}
	if err := validate(history); err != nil {
		return nil, err
	}
	writer := &boundedWriter{}
	if format == "json" {
		copyHistory := history
		copyHistory.Messages = make([]Message, len(history.Messages))
		for i, message := range history.Messages {
			copyHistory.Messages[i] = message
			copyHistory.Messages[i].Files = make([]File, len(message.Files))
			copy(copyHistory.Messages[i].Files, message.Files)
		}
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(copyHistory); err != nil {
			return nil, err
		}
	} else {
		encodeText(writer, history)
	}
	if writer.err != nil {
		return nil, writer.err
	}
	return writer.data, nil
}

func rejected(kind ErrorKind, field string) error {
	return &Error{Kind: kind, Field: field}
}

func validID(value string) bool {
	if len(value) == 0 || len(value) > MaxIDBytes || !asciiAlphanumeric(value[0]) {
		return false
	}
	for i := range value {
		if !asciiAlphanumeric(value[i]) && value[i] != '-' && value[i] != '_' && value[i] != '.' {
			return false
		}
	}
	return true
}

func asciiAlphanumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func validTimestamp(value string) bool {
	if len(value) < 20 || len(value) > 64 || value[13] != ':' || value[16] != ':' || value[19] == ',' {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Year() < 1 {
		return false
	}
	if value[len(value)-1] != 'Z' {
		if len(value) < 25 {
			return false
		}
		offset := value[len(value)-6:]
		if offset[1:3] > "23" || offset[4:6] > "59" {
			return false
		}
	}
	return true
}

func validate(history History) error {
	if history.Version != 1 {
		return rejected(InvalidHistory, "history.version")
	}
	if len(history.Messages) > MaxMessages {
		return rejected(LimitExceeded, "history.messages")
	}
	if !validID(history.ConversationID) || !validID(history.BotID) {
		return rejected(InvalidHistory, "history.identity")
	}
	inputBytes := len(history.ConversationID) + len(history.BotID)
	check := func(value, field string, maximum int, multiline bool) error {
		if len(value) > maximum {
			return rejected(LimitExceeded, field)
		}
		if !utf8.ValidString(value) {
			return rejected(InvalidHistory, field)
		}
		for _, r := range value {
			if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
				return rejected(InvalidHistory, field)
			}
		}
		if len(value) > MaxOutputBytes-inputBytes {
			return rejected(LimitExceeded, "history.input_bytes")
		}
		inputBytes += len(value)
		return nil
	}
	if err := check(history.Title, "history.title", MaxTitleBytes, false); err != nil {
		return err
	}
	requests, files := map[string]bool{}, map[string]bool{}
	for _, message := range history.Messages {
		if len(message.Files) > MaxFilesPerMessage {
			return rejected(LimitExceeded, "message.files")
		}
		if !validID(message.RequestID) || !validID(message.BotID) || requests[message.RequestID] {
			return rejected(InvalidHistory, "message.identity")
		}
		requests[message.RequestID] = true
		if !validTimestamp(message.CreatedAt) {
			return rejected(InvalidHistory, "message.created_at")
		}
		if len(message.Status) == 0 || len(message.Status) > 64 || message.Status[0] < 'a' || message.Status[0] > 'z' {
			return rejected(InvalidHistory, "message.status")
		}
		for _, r := range message.Status {
			if !(r >= 'a' && r <= 'z') && r != '_' {
				return rejected(InvalidHistory, "message.status")
			}
		}
		for _, item := range []struct {
			value, field string
			maximum      int
			multiline    bool
		}{
			{message.RequestID, "message.request_id", MaxIDBytes, false},
			{message.BotID, "message.bot_id", MaxIDBytes, false},
			{message.CreatedAt, "message.created_at", 64, false},
			{message.Status, "message.status", 64, false},
			{message.Text, "message.text", MaxTextBytes, true},
			{message.Reply, "message.reply", MaxTextBytes, true},
		} {
			if err := check(item.value, item.field, item.maximum, item.multiline); err != nil {
				return err
			}
		}
		for _, file := range message.Files {
			if !validID(file.ID) || files[file.ID] {
				return rejected(InvalidHistory, "file.identity")
			}
			files[file.ID] = true
			if file.Bytes < 0 || file.Name == "" || len(file.SHA256) != 64 {
				return rejected(InvalidHistory, "file.metadata")
			}
			if file.Bytes > MaxFileBytes {
				return rejected(LimitExceeded, "file.bytes")
			}
			for _, r := range file.SHA256 {
				if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
					return rejected(InvalidHistory, "file.sha256")
				}
			}
			for _, item := range []struct {
				value, field string
				maximum      int
			}{
				{file.ID, "file.id", MaxIDBytes},
				{file.Name, "file.name", MaxNameBytes},
				{file.SHA256, "file.sha256", 64},
				{file.DownloadPath, "file.download_path", MaxDownloadPathBytes},
			} {
				if err := check(item.value, item.field, item.maximum, false); err != nil {
					return err
				}
			}
			deliveryPath := strings.HasPrefix(message.RequestID, "report-delivery-") && file.DownloadPath == "/v1/report-deliveries/"+message.RequestID+"/artifacts/"+file.ID
			if file.DownloadPath != "" && file.DownloadPath != "/v1/channels/"+history.ConversationID+"/artifacts/"+file.ID && !deliveryPath {
				return rejected(InvalidHistory, "file.download_path")
			}
		}
	}
	return nil
}

type boundedWriter struct {
	data []byte
	err  error
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	if w.err == nil && len(data) > MaxOutputBytes-len(w.data) {
		w.err = rejected(LimitExceeded, "history.output_bytes")
	}
	if w.err != nil {
		return 0, w.err
	}
	w.data = append(w.data, data...)
	return len(data), nil
}

func encodeText(writer *boundedWriter, history History) {
	fmt.Fprintf(writer, "Neko Bot conversation history\nVersion: %d\nConversation: %s\nTitle: %s\nBot: %s\nArchived: %t\nAt sequence: %d\nMessages: %d\n", history.Version, history.ConversationID, history.Title, history.BotID, history.Archived, history.AtSequence, len(history.Messages))
	fmt.Fprint(writer, "Files contain metadata and local pointers only; obtain bytes through a separate authenticated download.\n")
	for i, message := range history.Messages {
		fmt.Fprintf(writer, "\nMessage %d\nRequest: %s\nBot: %s\nCreated at: %s\nStatus: %s\nUser text:\n%s\nReply:\n%s\nFiles: %d\n", i+1, message.RequestID, message.BotID, message.CreatedAt, message.Status, message.Text, message.Reply, len(message.Files))
		for j, file := range message.Files {
			fmt.Fprintf(writer, "File %d\nID: %s\nName: %s\nSHA-256: %s\nBytes: %d\nDownload path: %s\n", j+1, file.ID, file.Name, file.SHA256, file.Bytes, file.DownloadPath)
		}
	}
}
