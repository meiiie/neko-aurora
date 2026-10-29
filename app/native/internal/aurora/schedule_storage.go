package aurora

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"nekoaurora.local/native/internal/privatefs"
)

var errScheduleStorage = errors.New("schedule_storage_unavailable")

const maxScheduleBytes = 256 << 10

func readSchedules(path string) ([]schedule, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScheduleBytes {
		return nil, errScheduleStorage
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errScheduleStorage
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, errScheduleStorage
	}
	data, err := io.ReadAll(io.LimitReader(file, maxScheduleBytes+1))
	if err != nil || len(data) > maxScheduleBytes || !utf8.Valid(data) {
		return nil, errScheduleStorage
	}
	var list []schedule
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&list) != nil || len(list) > maxSchedules {
		return nil, errScheduleStorage
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errScheduleStorage
	}
	ids := map[string]bool{}
	for _, x := range list {
		if strings.TrimSpace(x.ID) == "" || len(x.ID) > 128 || ids[x.ID] || strings.TrimSpace(x.Bot) == "" || strings.TrimSpace(x.Text) == "" || utf8.RuneCountInString(x.Text) > 1000 {
			return nil, errScheduleStorage
		}
		ids[x.ID] = true
		if x.Repeat == "interval" {
			if _, _, valid := x.intervalState(); !valid {
				return nil, errScheduleStorage
			}
			continue
		}
		if x.IntervalMinutes != 0 || x.IntervalAnchor != "" || x.IntervalGeneration != 0 {
			return nil, errScheduleStorage
		}
		if !hhmm.MatchString(x.At) {
			return nil, errScheduleStorage
		}
		switch x.Repeat {
		case "daily", "weekdays":
		case "once":
			if !validScheduleDay(x.Date) {
				return nil, errScheduleStorage
			}
		default:
			return nil, errScheduleStorage
		}
		for _, day := range []string{x.Last, x.StartDate} {
			if day != "" && !validScheduleDay(day) {
				return nil, errScheduleStorage
			}
		}
	}
	return list, nil
}

func validScheduleDay(day string) bool {
	parsed, err := time.Parse("2006-01-02", day)
	return err == nil && parsed.Format("2006-01-02") == day
}

func openSchedules(dir string) *schedStore {
	st := &schedStore{path: filepath.Join(dir, "schedules.json")}
	st.L, st.loadErr = readSchedules(st.path)
	return st
}

func (st *schedStore) saveLocked() error {
	if st.loadErr != nil {
		return errScheduleStorage
	}
	data, err := json.Marshal(st.L)
	if err != nil || len(data) > maxScheduleBytes {
		return errScheduleStorage
	}
	if privatefs.WriteFile(st.path, data) != nil {
		return errScheduleStorage
	}
	return nil
}

func (st *schedStore) storageError() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.loadErr
}
