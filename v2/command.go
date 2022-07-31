package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/xyproto/vt100"
)

const commandTimeout = 10 * time.Second

// CommandToFunction takes an editor command as a string (with optional arguments) and returns a function that
// takes no arguments and performs the suggested action, like "save". Some functions may take an undo snapshot first.
func (e *Editor) CommandToFunction(c *vt100.Canvas, tty *vt100.TTY, status *StatusBar, bookmark *Position, undo *Undo, args ...string) (func(), error) {
	if len(args) == 0 {
		return nil, errors.New("no command given")
	}

	trimmedCommand := strings.TrimPrefix(strings.TrimSpace(args[0]), ":")

	if strings.HasPrefix(trimmedCommand, "!") {
		return func() {

			cmd := exec.Command(trimmedCommand[1:])
			if len(args) > 1 {
				cmd.Args = args[1:]
			}

			// Now run the cmd with the current block of lines as input
			stdin, err := cmd.StdinPipe()
			if err != nil {
				status.Clear(c)
				status.SetError(err)
				status.Show(c, e)
				return
			}
			go func() {
				defer stdin.Close()
				io.WriteString(stdin, e.Block(e.LineIndex()))
			}()

			// Gather the output in the same way as CombinedOutput and Run
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			err = cmd.Start()
			if err != nil {
				status.Clear(c)
				status.SetError(err)
				status.Show(c, e)
				return
			}

			outputString := ""

			// Create a completion channel, thanks
			// https://medium.com/@vCabbage/go-timeout-commands-with-os-exec-commandcontext-ba0c861ed738
			done := make(chan error)
			go func() { done <- cmd.Wait() }()

			// Start a timer
			timeout := time.After(commandTimeout)

			// Check if the timeout channel or done channel receives something first
			select {
			case <-timeout:
				cmd.Process.Kill()
				status.Clear(c)
				status.SetErrorMessage("command timed out")
				status.Show(c, e)
				return
			case err := <-done:
				outputString = buf.String()
				if err != nil {
					status.Clear(c)
					status.SetErrorMessage(cmd.String() + ": " + err.Error())
					status.Show(c, e)
					return
				}
			}

			if outputString == "" {
				status.Clear(c)
				status.SetErrorMessage("no output")
				status.Show(c, e)
				return
			}

			undo.Snapshot(e)
			e.ReplaceBlock(c, status, bookmark, outputString)
		}, nil
	}

	// Argument checks, remember to use all available aliases
	switch trimmedCommand {
	case "if", "i", "insertfile", "insert", "insertf":
		if len(args) != 2 {
			return nil, fmt.Errorf("%s requires a filename as the second argument", trimmedCommand)
		}
	default:
		if len(args) != 1 {
			return nil, fmt.Errorf("%s takes no arguments", args[0])
		}
	}

	const (
		nothing = iota
		copyall
		help
		insertdate
		insertfile
		quit
		save
		savequit
		savequitclear
		sortblock
		sortstrings
		version
	)

	// Define args and corresponding functions
	var commandLookup = map[int]func(){
		copyall: func() { // copy all contents to the clipboard
			if err := clipboard.WriteAll(e.String()); err != nil {
				status.Clear(c)
				status.SetError(err)
				status.Show(c, e)
			} else {
				status.SetMessageAfterRedraw("Copied everything")
			}
		},
		help: func() { // display an informative status message
			// TODO: Draw the same type of box that is used in debug mode, listing all possible commands
			status.SetMessageAfterRedraw(":wq, s, save, sq, savequit, q, quit, h, help, sort, v, version")
		},
		insertdate: func() { // insert te current date
			undo.Snapshot(e)
			// If a space is added after the string here, it will be stripped when the command menu disappears.
			// This is why e.addSpace is used. There is probably a better way than using the addSpace variable.
			// TODO: Find a way to not use e.addSpace
			dateString := time.Now().Format(time.RFC3339)[:10]
			e.InsertString(c, dateString)
			e.addSpace = true
		},
		insertfile: func() { // insert a file
			undo.Snapshot(e)
			editedFileDir := filepath.Dir(e.filename)
			if err := e.InsertFile(c, filepath.Join(editedFileDir, strings.TrimSpace(args[1]))); err != nil {
				status.Clear(c)
				status.SetError(err)
				status.Show(c, e)
			}
		},
		save: func() { // save the current file
			e.UserSave(c, tty, status)
		},
		savequit: func() { // save and quit
			e.UserSave(c, tty, status)
			e.quit = true
		},
		savequitclear: func() { // save and quit, then clear the screen
			e.UserSave(c, tty, status)
			e.quit = true
			e.clearOnQuit = true
		},
		sortblock: func() { // sort the current block of lines, until the next blank line or EOF
			undo.Snapshot(e)
			e.SortBlock(c, status, bookmark)
		},
		sortstrings: func() { // sort the words on the current line
			undo.Snapshot(e)
			e.SortStrings(c, status)
			e.redraw = true
			e.redrawCursor = true
		},
		quit: func() { // quit
			e.quit = true
		},
		version: func() { // display the program name and version as a status message
			status.SetMessageAfterRedraw(versionString)
		},
	}

	// TODO: Also handle the command arguments, command[1:], if given.
	//       For instance, the save commands could take a filename.

	// Helpful command aliases that can also handle some typos and abbreviations
	var functionID int
	switch trimmedCommand {
	case "copyall", "copya":
		functionID = copyall
	case "qs", "byes", "cus", "exitsave", "quitandsave", "quitsave", "qw", "saq", "saveandquit", "saveexit", "saveq", "savequit", "savq", "sq", "wq":
		functionID = savequit
	case "s", "sa", "sav", "save", "w", "ww":
		functionID = save
	case "bye", "cu", "ee", "exit", "q", "qq", "qu", "qui", "quit":
		functionID = quit
	case "h", "he", "hh", "hel", "help":
		functionID = help
	case "if", "i", "insertfile", "insert", "insertf":
		functionID = insertfile
	case "insertdate", "insertd", "id", "date":
		functionID = insertdate
	case "v", "ver", "vv", "version":
		functionID = version
	case "sb", "so", "sor", "sort":
		functionID = sortblock
	case "sortstrings", "sortw", "sortwords", "sow", "ss", "sw", "sortfields", "sf":
		functionID = sortstrings
	default:
		return nil, fmt.Errorf("unknown command: %s", args[0])
	}

	// Return the selected function
	f, ok := commandLookup[functionID]
	if !ok {
		return nil, fmt.Errorf("implementation missing for command: %s", args[0])
	}
	return f, nil
}

// RunCommand takes a command string and performs and action (like "save" or "quit")
func (e *Editor) RunCommand(c *vt100.Canvas, tty *vt100.TTY, status *StatusBar, bookmark *Position, undo *Undo, args ...string) error {
	f, err := e.CommandToFunction(c, tty, status, bookmark, undo, args...)
	if err != nil {
		return err
	}
	f()
	return nil
}
