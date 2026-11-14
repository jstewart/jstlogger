// Package editor runs the user's text editor ($VISUAL, then $EDITOR, then
// vi) on a temporary file so a long text value can be edited outside the
// app. It builds the command and handles the file; the caller runs the
// command. It knows nothing about the terminal UI.
package editor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// fallback is the editor used when neither $VISUAL nor $EDITOR is set.
const fallback = "vi"

// Command returns the editor command and its arguments, taken from
// $VISUAL, then $EDITOR, then vi. A variable holding only whitespace counts
// as unset. The value is split into words with Split, so "code --wait"
// gives ["code", "--wait"].
func Command(getenv func(string) string) ([]string, error) {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		value := getenv(key)
		if strings.TrimSpace(value) == "" {
			continue
		}
		argv, err := Split(value)
		if err != nil {
			return nil, fmt.Errorf("parse $%s: %w", key, err)
		}
		return argv, nil
	}
	return []string{fallback}, nil
}

// Split splits s into words the way a POSIX shell would, without expanding
// anything. Whitespace separates words. Single quotes keep everything
// between them literally. Double quotes keep everything literally except a
// backslash before ", \, $ or a backtick, which escapes it. Outside quotes a
// backslash escapes the next character. It returns an error for an
// unterminated quote, a trailing backslash, or a string with no words.
func Split(s string) ([]string, error) {
	var (
		words []string
		word  strings.Builder
		// inWord is true once the current word has started, so that an
		// empty quoted string such as '' still makes a word.
		inWord bool
	)
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case ' ', '\t', '\n', '\r':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case '\\':
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("split %q: trailing backslash", s)
			}
			i++
			word.WriteRune(runes[i])
			inWord = true
		case '\'':
			end := indexRune(runes, i+1, '\'')
			if end < 0 {
				return nil, fmt.Errorf("split %q: unterminated single quote", s)
			}
			word.WriteString(string(runes[i+1 : end]))
			i = end
			inWord = true
		case '"':
			i++
			for ; i < len(runes) && runes[i] != '"'; i++ {
				if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]) {
					i++
				}
				word.WriteRune(runes[i])
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("split %q: unterminated double quote", s)
			}
			inWord = true
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("split %q: no command", s)
	}
	return words, nil
}

// indexRune returns the index of the first r in runes at or after from, or
// -1 when there is none.
func indexRune(runes []rune, from int, r rune) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

// SingleLine flattens edited text into one line: newlines and runs of
// whitespace become single spaces, and leading and trailing whitespace is
// removed. It suits single-line fields such as the ADIF COMMENT.
func SingleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// MultiLine tidies edited text that may span lines: CRLF and lone CR line
// endings become LF, and the trailing newlines editors append are removed.
// Everything else is kept as written. It suits fields such as the ADIF
// NOTES and QSLMSG.
func MultiLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, "\n")
}

// Prepare writes value to a new temporary file and returns the command
// that opens it in the editor argv, along with the file's path. The path
// is passed as the last argument, never through a shell. The caller runs
// the command and then calls ReadBack with the path. On error no file is
// left behind.
func Prepare(argv []string, value string) (*exec.Cmd, string, error) {
	if len(argv) == 0 {
		return nil, "", errors.New("prepare editor: no editor command")
	}
	f, err := os.CreateTemp("", "jstlogger-*.txt")
	if err != nil {
		return nil, "", fmt.Errorf("create editor file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(value); err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("write editor file %s: %w", path, err),
			f.Close(),
			os.Remove(path),
		)
	}
	if err := f.Close(); err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("close editor file %s: %w", path, err),
			os.Remove(path),
		)
	}
	args := append(append([]string{}, argv[1:]...), path)
	return exec.Command(argv[0], args...), path, nil
}

// ReadBack returns the contents of the file at path and removes the file,
// whether or not the read succeeded. The text is returned as the editor
// left it; tidy it with SingleLine or MultiLine.
func ReadBack(path string) (string, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		readErr = fmt.Errorf("read editor file: %w", readErr)
	}
	var removeErr error
	// A file that could not be read because it is gone needs no removing.
	if err := os.Remove(path); err != nil && (readErr == nil || !errors.Is(err, fs.ErrNotExist)) {
		removeErr = fmt.Errorf("remove editor file: %w", err)
	}
	if err := errors.Join(readErr, removeErr); err != nil {
		return "", err
	}
	return string(data), nil
}
