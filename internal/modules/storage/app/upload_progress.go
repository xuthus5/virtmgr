package app

import (
	"context"
	"fmt"
	"io"
)

type uploadProgressTracker struct {
	ctx          context.Context
	reader       io.Reader
	total        int64
	read         int64
	lastProgress int
	recorder     TaskRecorder
	taskID       string
	name         string
}

func (t *uploadProgressTracker) Read(buffer []byte) (int, error) {
	n, err := t.reader.Read(buffer)
	t.read += int64(n)
	progress := t.Progress()
	if progress >= t.lastProgress+10 || (err != nil && progress > t.lastProgress) {
		t.lastProgress = progress
		if updateErr := t.recorder.Progress(t.ctx, t.taskID, progress, fmt.Sprintf("正在上传 %s", t.name)); updateErr != nil {
			return n, updateErr
		}
	}
	return n, err
}

func (t *uploadProgressTracker) Progress() int {
	if t.total <= 0 || t.read <= 0 {
		return 0
	}
	progress := int(t.read * 100 / t.total)
	if progress > 99 {
		return 99
	}
	return progress
}
