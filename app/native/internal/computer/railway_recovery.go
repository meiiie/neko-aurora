package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var ErrRecoveryPending = errors.New("computer_recovery_pending")
var ErrRecoveryConflict = errors.New("computer_recovery_local_files_changed")

type RecoveryStatus struct {
	State      string `json:"state"`
	CanRecover bool   `json:"canRecover"`
	Message    string `json:"message"`
}

type railwayPending struct {
	Version     int    `json:"version"`
	SandboxID   string `json:"sandboxId"`
	Phase       string `json:"phase"`
	HomeSHA256  string `json:"homeSHA256"`
	CommandHash string `json:"commandSHA256"`
	Started     string `json:"started"`
}

func pendingFile(home string) string { return filepath.Clean(home) + ".sandbox-pending" }

func pendingExists(home string) bool {
	_, err := os.Lstat(pendingFile(home))
	return !errors.Is(err, os.ErrNotExist)
}

func readPending(home string) (*railwayPending, error) {
	info, err := os.Lstat(pendingFile(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 4096 {
		return nil, ErrRecoveryPending
	}
	f, err := os.Open(pendingFile(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 4097))
	decoder.DisallowUnknownFields()
	var p railwayPending
	if err = decoder.Decode(&p); err != nil {
		return nil, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return nil, ErrRecoveryPending
	}
	if p.Version != 1 || p.SandboxID == "" || len(p.SandboxID) > 200 || len(p.HomeSHA256) != 64 || len(p.CommandHash) != 64 {
		return nil, ErrRecoveryPending
	}
	if p.Phase != "preparing" && p.Phase != "command" && p.Phase != "pull" {
		return nil, ErrRecoveryPending
	}
	return &p, nil
}

func writePending(home string, p railwayPending, create bool) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var f *os.File
	if create {
		f, err = os.OpenFile(pendingFile(home), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	} else {
		f, err = os.CreateTemp(filepath.Dir(home), ".sandbox-pending-*")
	}
	if err != nil {
		return err
	}
	if !create {
		defer os.Remove(f.Name())
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if !create {
		return os.Rename(f.Name(), pendingFile(home))
	}
	return nil
}

func clearPending(home string) error {
	err := os.Remove(pendingFile(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (r *Railway) RecoveryStatus(home string) *RecoveryStatus {
	p, err := readPending(home)
	if err != nil {
		return &RecoveryStatus{State: "blocked", Message: "Chưa đọc được trạng thái máy ảo. Lệnh mới đang bị chặn để giữ dữ liệu."}
	}
	if p == nil {
		return nil
	}
	if p.Phase == "pull" {
		return &RecoveryStatus{State: "files", CanRecover: true, Message: "Lệnh đã kết thúc nhưng file chưa được xác nhận đồng bộ. Có thể tải lại file mà không chạy lại lệnh; bản local phải còn nguyên."}
	}
	return &RecoveryStatus{State: "unknown", Message: "Chưa xác nhận kết quả máy ảo. Lệnh mới và đồng bộ lên đang bị chặn. Không tự chạy lại; giữ bản local để đối chiếu trước khi làm mới máy."}
}

func (r *Railway) Recover(ctx context.Context, s Spec) error {
	gate := railwayHomeGate(s.Home)
	gate.Lock()
	defer gate.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := readPending(s.Home)
	if err != nil {
		return ErrRecoveryPending
	}
	if p == nil {
		return nil
	}
	if p.Phase != "pull" {
		return ErrRecoveryPending
	}
	if err := r.Available(); err != nil {
		return err
	}
	local, err := packHome(s.Home)
	if err != nil {
		return err
	}
	if homeSum(local) != p.HomeSHA256 {
		return ErrRecoveryConflict
	}
	if err = r.pull(ctx, p.SandboxID, s); err != nil {
		return fmt.Errorf("%w: file transfer unconfirmed", ErrRecoveryPending)
	}
	return clearPending(s.Home)
}

func (m *Manager) Recover(ctx context.Context, bot string) error {
	backend, ok := m.sb.(interface {
		Recover(context.Context, Spec) error
	})
	if !ok {
		return ErrUnavailable
	}
	unlock := m.lock(bot)
	defer unlock()
	home, err := m.home(bot)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return backend.Recover(ctx, Spec{Home: home, Limits: m.Limits})
}
