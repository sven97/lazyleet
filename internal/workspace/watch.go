package workspace

import (
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// Watcher reports when the solution file changes on disk. It watches the parent
// directory (editors typically save by writing a temp file and renaming it, so
// watching the file inode directly misses saves) and filters events down to the
// solution file's name.
type Watcher struct {
	fs     *fsnotify.Watcher
	target string        // absolute path of the solution file
	Events chan struct{} // one signal per relevant change; coalescing is the caller's job
	done   chan struct{}
}

// Watch starts watching the directory containing solutionPath.
func Watch(solutionPath string) (*Watcher, error) {
	abs, err := filepath.Abs(solutionPath)
	if err != nil {
		return nil, err
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(filepath.Dir(abs)); err != nil {
		fw.Close()
		return nil, err
	}

	w := &Watcher{
		fs:     fw,
		target: abs,
		Events: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if filepath.Clean(ev.Name) != w.target {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			// Non-blocking send: if a signal is already queued the caller will
			// pick up this change when it drains the channel.
			select {
			case w.Events <- struct{}{}:
			default:
			}
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

// Close stops watching.
func (w *Watcher) Close() error {
	close(w.done)
	return w.fs.Close()
}
