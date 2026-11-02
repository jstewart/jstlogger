package tui

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/adifexport"
	"jstlogger/internal/logbook"
)

// exportTimeout bounds an export of the marked QSOs, which reads every one
// of them with its fields and writes a file, so it gets longer than a
// logbook query.
const exportTimeout = time.Minute

// confirmYes answers yes to a confirmation; every other key answers no.
var confirmYes = key.NewBinding(key.WithKeys("y", "Y"))

// pendingConfirm is a y/N question waiting for its answer, and the action
// a yes runs.
type pendingConfirm struct {
	question string
	yes      func(m Model) (Model, tea.Cmd)
}

// indexBulkMsg reports the outcome of a bulk operation on the marked QSOs:
// a status message, the IDs of the QSOs it deleted, whose marks are
// dropped, and whether the index must reload.
type indexBulkMsg struct {
	status  string
	deleted []int64
	reload  bool
	err     error
}

// bulkFuncs returns the bulk operations on the marked QSOs, which the
// mark-prefix runs.
func (m Model) bulkFuncs() []markedFunc {
	k := m.indexKeys
	return []markedFunc{
		{keys: k.BulkDelete, run: Model.askDelete},
		{keys: k.BulkSave, run: func(m Model, _ []int64) (Model, tea.Cmd) {
			return m.openPrompt(promptSave)
		}},
		{keys: k.BulkTag, run: func(m Model, _ []int64) (Model, tea.Cmd) {
			return m.openPrompt(promptTag)
		}},
		{keys: k.BulkUntag, run: func(m Model, _ []int64) (Model, tea.Cmd) {
			return m.openPrompt(promptUntag)
		}},
	}
}

// markedIDs returns the IDs of the marked QSOs in ascending order.
func (x index) markedIDs() []int64 {
	return slices.Sorted(maps.Keys(x.marks))
}

// askConfirm opens a y/N question in the prompt area; a yes runs yes.
func (m Model) askConfirm(question string, yes func(m Model) (Model, tea.Cmd)) Model {
	m = m.closePrompt()
	m.prompt = promptConfirm
	m.confirm = &pendingConfirm{question: question, yes: yes}
	return m
}

// answerConfirm closes the open question and runs its action if msg
// answers yes. Any other key answers no, silently.
func (m Model) answerConfirm(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	c := m.confirm
	m = m.closePrompt()
	if c == nil || !key.Matches(msg, confirmYes) {
		return m, nil
	}
	return c.yes(m)
}

// withMarks calls fn with the IDs of the marked QSOs, looked up again as
// they may have changed since the operation started, or reports on the
// status line that none are marked.
func (m Model) withMarks(fn func(ids []int64) tea.Cmd) (Model, tea.Cmd) {
	ids := m.idx.markedIDs()
	if len(ids) == 0 {
		m.idx.msg = msgNoMarks
		return m, nil
	}
	return m, fn(ids)
}

// askDelete asks whether to delete the marked QSOs.
func (m Model) askDelete(ids []int64) (Model, tea.Cmd) {
	q := fmt.Sprintf("Delete %s? [y/N]", countQSOs(len(ids)))
	return m.askConfirm(q, func(m Model) (Model, tea.Cmd) {
		return m.withMarks(func(ids []int64) tea.Cmd {
			return deleteQSOsCmd(m.logbook, ids)
		})
	}), nil
}

// submitTag tags, when add is true, or untags the marked QSOs with the tag
// typed in the prompt. An empty tag does nothing.
func (m Model) submitTag(s string, add bool) (Model, tea.Cmd) {
	tag := strings.TrimSpace(s)
	if tag == "" {
		return m, nil
	}
	return m.withMarks(func(ids []int64) tea.Cmd {
		return tagQSOsCmd(m.logbook, ids, tag, add)
	})
}

// submitSave closes the prompt and saves the marked QSOs to the ADIF file
// whose path is typed in it, asking first whether to overwrite an existing
// file. An empty path does nothing. A leading ~/ stands for the home
// directory; an error finding it leaves the prompt open.
func (m Model) submitSave(s string) (Model, tea.Cmd, error) {
	path, err := expandHome(strings.TrimSpace(s))
	switch {
	case err != nil:
		return m, nil, err
	case path == "":
		return m.closePrompt(), nil, nil
	}
	save := func(m Model) (Model, tea.Cmd) {
		return m.withMarks(func(ids []int64) tea.Cmd {
			return saveQSOsCmd(m.logbook, ids, path, m.version)
		})
	}
	if _, err := os.Stat(path); err == nil {
		return m.askConfirm(fmt.Sprintf("Overwrite %s? [y/N]", path), save), nil, nil
	}
	m = m.closePrompt()
	m, cmd := save(m)
	return m, cmd, nil
}

// expandHome replaces a leading ~/ in path with the home directory.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %s: %w", path, err)
	}
	return filepath.Join(home, rest), nil
}

// deleteQSOsCmd returns a command that deletes the QSOs with the given IDs,
// all of them or none.
func deleteQSOsCmd(s logbook.Store, ids []int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		if err := s.DeleteMany(ctx, ids); err != nil {
			return indexBulkMsg{err: fmt.Errorf("delete QSOs: %w", err)}
		}
		return indexBulkMsg{
			status:  "Deleted " + countQSOs(len(ids)) + ".",
			deleted: ids,
			reload:  true,
		}
	}
}

// tagQSOsCmd returns a command that adds tag to, when add is true, or
// removes it from the QSOs with the given IDs.
func tagQSOsCmd(s logbook.Store, ids []int64, tag string, add bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		verb, done, op := "untag", "Untagged", s.Untag
		if add {
			verb, done, op = "tag", "Tagged", s.Tag
		}
		if err := op(ctx, ids, tag); err != nil {
			return indexBulkMsg{err: fmt.Errorf("%s QSOs: %w", verb, err)}
		}
		// A tag can change which QSOs a limit such as ~y matches.
		return indexBulkMsg{
			status: done + " " + countQSOs(len(ids)) + ".",
			reload: true,
		}
	}
}

// saveQSOsCmd returns a command that exports the QSOs with the given IDs
// to the ADIF file at path, replacing any file there.
func saveQSOsCmd(s logbook.Store, ids []int64, path, version string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
		defer cancel()
		n, err := adifexport.WriteFile(ctx, s, path, adifexport.Options{
			Filter:  logbook.Filter{IDs: ids},
			Version: version,
		})
		if err != nil {
			return indexBulkMsg{err: fmt.Errorf("save QSOs to %s: %w", path, err)}
		}
		return indexBulkMsg{status: fmt.Sprintf("Saved %s to %s.", countQSOs(n), path)}
	}
}

// applyBulk reports the outcome of a bulk operation on the status line,
// drops the marks of the QSOs it deleted and, if it changed the QSOs,
// returns a command that reloads the index, keeping the cursor on its QSO
// when it is still there and on the same row otherwise.
func (m Model) applyBulk(msg indexBulkMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.idx.msg = msg.err.Error()
		return m, nil
	}
	for _, id := range msg.deleted {
		delete(m.idx.marks, id)
	}
	m.idx.msg = msg.status
	if !msg.reload {
		return m, nil
	}
	id := m.idx.restart()
	return m, loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, id, m.idx.cursor)
}
