package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestQuotedReplyReceiptIsolationAndTamperRejection(t *testing.T) {
	for _, kind := range []string{"snapshot", "missing", "source"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			w, _, err := s.Accept(Input{BotID: "bot-meiiie", Text: "Source", ClientNonce: "quote-source-ledger"})
			if err != nil {
				t.Fatal(err)
			}
			foreign, _, err := s.Accept(Input{BotID: "bot-davin", Text: "Other", ClientNonce: "quote-foreign-ledger"})
			if err != nil {
				t.Fatal(err)
			}
			q := &MessageQuote{ID: w.RequestID + ":u", TaskID: w.RequestID, Role: "user", Author: "Owner", Text: "Source"}
			in := OwnerMessageInput{ThreadID: "bot-meiiie", Text: "Reply", ClientNonce: "quote-receipt-ledger", ReplyTo: q.ID}
			accepted, _, err := s.AcceptOwnerMessage(in, OwnerMessagePlan{BotIDs: []string{"bot-meiiie"}, Quote: q})
			if err != nil {
				t.Fatal(err)
			}
			q.Text = "mutated by caller"
			r, ok := s.OwnerMessageForWork(accepted[0].RequestID)
			if !ok || r.Quote.Text != "Source" {
				t.Fatal("plan mutated receipt")
			}
			r.Quote.Text = "mutated read"
			r.WorkIDs[0] = "changed"
			r, _ = s.OwnerMessageForWork(accepted[0].RequestID)
			if r.Quote.Text != "Source" || r.WorkIDs[0] != accepted[0].RequestID {
				t.Fatal("read mutated receipt")
			}
			path := s.file.Name()
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			r, ok = s.OwnerMessageForWork(accepted[0].RequestID)
			if !ok || r.Quote.Text != "Source" {
				t.Fatal("restart lost receipt index")
			}
			s.Close()
			data, _ := os.ReadFile(path)
			start := bytes.LastIndexByte(data[:len(data)-1], '\n') + 1
			var frame record
			if err = json.Unmarshal(data[start:], &frame); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "snapshot":
				frame.Message.Quote.Text = "tampered"
			case "missing":
				frame.Message.Quote = nil
			case "source":
				frame.Message.Quote.TaskID = foreign.RequestID
			}
			frame.Digest, _ = digestRecord(frame)
			line, _ := json.Marshal(frame)
			if err = os.WriteFile(path, append(append(data[:start], line...), '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if opened, err := Open(dir); !errors.Is(err, ErrCorrupt) {
				if opened != nil {
					opened.Close()
				}
				t.Fatal("tampered quote accepted", err)
			}
		})
	}
}
