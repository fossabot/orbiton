package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xyproto/syntax"
	"github.com/xyproto/vt100"
)

const versionString = "o 2.6.6"

func main() {
	var (
		// Color scheme for the "text edit" mode
		defaultEditorForeground       = vt100.LightGreen // for when syntax highlighting is not in use
		defaultEditorBackground       = vt100.BackgroundDefault
		defaultEditorStatusForeground = vt100.White
		defaultEditorStatusBackground = vt100.BackgroundBlack
		defaultEditorSearchHighlight  = vt100.LightMagenta
		defaultEditorHighlightTheme   = syntax.TextConfig{
			String:        "lightyellow",
			Keyword:       "lightred",
			Comment:       "gray",
			Type:          "lightblue",
			Literal:       "lightgreen",
			Punctuation:   "lightblue",
			Plaintext:     "lightgreen",
			Tag:           "lightgreen",
			TextTag:       "lightgreen",
			TextAttrName:  "lightgreen",
			TextAttrValue: "lightgreen",
			Decimal:       "white",
			Whitespace:    "",
		}

		version = flag.Bool("version", false, "show version information")
		help    = flag.Bool("help", false, "show simple help")

		statusDuration = 2700 * time.Millisecond

		copyLine   string   // for the cut/copy/paste functionality
		bookmark   Position // for the bookmark/jump functionality
		statusMode bool     // if information should be shown at the bottom

		firstLetterSinceStart string
	)

	flag.Parse()

	if *version {
		fmt.Println(versionString)
		return
	}

	if *help {
		fmt.Println(versionString + " - simple and limited text editor")
		fmt.Print(`
Hotkeys

ctrl-q to quit
ctrl-s to save
ctrl-w to format the current file with "go fmt"
ctrl-a go to start of line, then start of text
ctrl-e go to end of line
ctrl-p to scroll up 10 lines
ctrl-n to scroll down 10 lines
ctrl-k to delete characters to the end of the line, then delete the line
ctrl-g to toggle filename/line/column/unicode/word count status display
ctrl-d to delete a single character
ctrl-t to toggle syntax highlighting
ctrl-r to toggle text or draw mode (for ASCII graphics)
ctrl-x to cut the current line
ctrl-c to copy the current line
ctrl-v to paste the current line
ctrl-b to bookmark the current position
ctrl-j to jump to the bookmark
ctrl-h to show a minimal help text
ctrl-u to undo
ctrl-l to jump to a specific line
ctrl-f to find a string. Press ctrl-f and return to repeat the search.
esc to redraw the screen and clear the last search.
`)
		return
	}

	filename, lineNumber := FilenameAndLineNumber(flag.Arg(0), flag.Arg(1))
	if filename == "" {
		fmt.Fprintln(os.Stderr, "Need a filename.")
		os.Exit(1)
	}

	baseFilename := filepath.Base(filename)
	gitMode := baseFilename == "COMMIT_EDITMSG" || (strings.HasPrefix(baseFilename, "git-") && !strings.Contains(baseFilename, ".") && strings.Count(baseFilename, "-") >= 2)
	defaultHighlight := gitMode || baseFilename == "PKGBUILD" || strings.Contains(baseFilename, ".")

	tty, err := vt100.NewTTY()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}

	vt100.Init()

	c := vt100.NewCanvas()
	c.ShowCursor()

	// 4 spaces per tab, scroll 10 lines at a time
	e := NewEditor(4, defaultEditorForeground, defaultEditorBackground, defaultHighlight, true, 10, defaultEditorSearchHighlight, defaultEditorHighlightTheme)
	e.gitMode = gitMode

	status := NewStatusBar(defaultEditorStatusForeground, defaultEditorStatusBackground, e, statusDuration)

	// Try to load the filename, ignore errors since giving a new filename is also okay
	loaded := e.Load(c, tty, filename) == nil

	// If we're editing a git commit message, add a newline
	if e.gitMode {
		e.gitColor = vt100.LightGreen
		status.fg = vt100.LightBlue
		status.bg = vt100.BackgroundDefault
		e.InsertLineBelow()
	}

	// Draw editor lines from line 0 to h onto the canvas at 0,0
	e.DrawLines(c, false, false)

	// Friendly status message
	statusMessage := "New " + filename
	if loaded {
		if !e.Empty() {
			statusMessage = "Loaded " + filename
		} else {
			statusMessage = "Loaded empty file: " + filename
		}
		fileInfo, err := os.Stat(filename)
		if err != nil {
			quitError(tty, err)
		}
		if fileInfo.IsDir() {
			quitError(tty, errors.New(filename+" is a directory"))
		}
		testFile, err := os.OpenFile(filename, os.O_WRONLY, 0664)
		if err != nil {
			// Can not open the file for writing
			statusMessage += " (read only)"
			// Set the color to red when in read-only mode
			e.fg = vt100.Red
			// Disable syntax highlighting, to make it clear that the text is red
			e.highlight = false
			// Draw the editor lines again
			e.DrawLines(c, false, false)
		}
		testFile.Close()
	} else if err := e.Save(filename, true); err != nil {
		// Check if the new file can be saved before the user starts working on the file.
		quitError(tty, err)
	} else {
		// Creating a new empty file worked out fine, don't save it until the user saves it
		if os.Remove(filename) != nil {
			// This should never happen
			quitError(tty, errors.New("could not remove an empty file that was just created: "+filename))
		}
	}
	status.SetMessage(statusMessage)
	status.Show(c, e)
	c.Draw()

	// Undo buffer with room for 8192 actions
	undo := NewUndo(8192)

	// Resize handler
	SetUpResizeHandler(c, e, tty)

	tty.SetTimeout(2 * time.Millisecond)

	previousX := -1
	previousY := -1

	if lineNumber > 0 {
		e.redraw = e.GoToLineNumber(lineNumber, c, status)
		e.redrawCursor = true
	}

	quit := false
	for !quit {
		key := tty.KeyBlock()
		switch key {
		case "c:17": // ctrl-q, quit
			quit = true
		case "c:23": // ctrl-w, format
			undo.Snapshot(e)
			// Map from formatting command to a list of file extensions
			format := map[*exec.Cmd][]string{
				exec.Command("/usr/bin/goimports", "-w", "--"):                                             []string{".go"},
				exec.Command("/usr/bin/clang-format", "-fallback-style=WebKit", "-style=file", "-i", "--"): []string{".cpp", ".cxx", ".h", ".hpp", ".c++", ".h++"},
			}
			formatted := false
		OUT:
			for cmd, extensions := range format {
				for _, ext := range extensions {
					if strings.HasSuffix(filename, ext) {
						// Use a globally unique temp file
						if f, err := ioutil.TempFile("/tmp", "__o*"+ext); err == nil {
							// no error, everything is fine
							tempFilename := f.Name()
							err := e.Save(tempFilename, true)
							if err == nil {
								// Format the temporary file
								cmd.Args = append(cmd.Args, tempFilename)
								output, err := cmd.CombinedOutput()
								if err != nil {
									// Only grab the first error message
									errorMessage := strings.TrimSpace(string(output))
									if strings.Count(errorMessage, "\n") > 0 {
										errorMessage = strings.TrimSpace(strings.SplitN(errorMessage, "\n", 2)[0])
									}
									status.SetMessage("Failed to format code: " + errorMessage)
									if strings.Count(errorMessage, ":") >= 3 {
										fields := strings.Split(errorMessage, ":")
										// Go To Y:X, if available
										foundY := -1
										if y, err := strconv.Atoi(fields[1]); err == nil { // no error
											foundY = y - 1
											e.redraw = e.GoTo(foundY, c, status)
											foundX := -1
											if x, err := strconv.Atoi(fields[2]); err == nil { // no error
												foundX = x - 1
											}
											if foundX != -1 {
												tabs := strings.Count(e.Line(foundY), "\t")
												e.pos.sx = foundX + (tabs * (e.spacesPerTab - 1))
											}
										}
										e.redrawCursor = true
									}
									status.Show(c, e)
									break OUT
								} else {
									e.Load(c, tty, tempFilename)
									// Mark the data as changed, despite just having loaded a file
									e.changed = true
									formatted = true
								}
								// Try to remove the temporary file regardless if "goimports -w" worked out or not
								_ = os.Remove(tempFilename)
							}
							// Try to close the file. f.Close() checks if f is nil before closing.
							_ = f.Close()
							e.redraw = true
						}
						break OUT
					}
				}
			}
			if !formatted {
				status.SetMessage("Can only format Go or C++ code.")
				status.Show(c, e)
			}
		case "c:6": // ctrl-f, find string
			s := e.SearchTerm()
			//e.SetSearchTerm(s, c, status)
			status.ClearAll(c)
			if s == "" {
				status.SetMessage("Search:")
			} else {
				status.SetMessage("Search: " + s)
			}
			status.ShowNoTimeout(c, e)
			doneCollectingLetters := false
			for !doneCollectingLetters {
				key2 := tty.KeyBlock()
				switch key2 {
				case "c:127": // backspace
					if len(s) > 0 {
						s = s[:len(s)-1]
						e.SetSearchTerm(s, c, status)
						status.SetMessage("Search: " + s)
						status.ShowNoTimeout(c, e)
					}
				case "c:27", "c:17": // esc or ctrl-q
					s = ""
					e.SetSearchTerm(s, c, status)
					fallthrough
				case "c:13": // return
					doneCollectingLetters = true
				default:
					if key2 != "" {
						s += key2 // string(rune(key2))
						e.SetSearchTerm(s, c, status)
						status.SetMessage("Search: " + s)
						status.ShowNoTimeout(c, e)
					}
				}
			}
			status.ClearAll(c)
			if s != "" {
				// Go to the next line with "s"
				foundY := -1
				foundX := -1
				for y := e.DataY(); y < e.Len(); y++ {
					lineContents := e.Line(y)
					if y == e.DataY() {
						x, err := e.DataX()
						if err != nil {
							continue
						}
						// Search from the next position on this line
						x++
						if x >= len(lineContents) {
							continue
						}
						if strings.Contains(lineContents[x:], s) {
							foundX = x + strings.Index(lineContents[x:], s)
							foundY = y
							break
						}
					} else {
						if strings.Contains(lineContents, s) {
							foundX = strings.Index(lineContents, s)
							foundY = y
							break
						}
					}
				}
				if foundY != -1 {
					e.redraw = e.GoTo(foundY, c, status)
					if foundX != -1 {
						tabs := strings.Count(e.Line(foundY), "\t")
						e.pos.sx = foundX + (tabs * (e.spacesPerTab - 1))
					}
					e.redraw = true
					e.redrawCursor = e.redraw
				} else {
					e.GoTo(e.lineBeforeSearch, c, status)
					status.SetMessage("Not found (no wraparound)")
					status.Show(c, e)
				}
			}
		case "c:18": // ctrl-r, toggle draw mode
			e.ToggleDrawMode()
			statusMessage := "Text mode"
			if e.DrawMode() {
				statusMessage = "Draw mode"
			}
			status.SetMessage(statusMessage)
			status.Show(c, e)
		case "c:7": // ctrl-g, status mode
			statusMode = !statusMode
			if statusMode {
				status.ShowLineColWordCount(c, e, filename)
			} else {
				status.ClearAll(c)
			}
		case "←": // left arrow
			if !e.DrawMode() {
				e.Prev(c)
				if e.AfterLineScreenContents() {
					e.End()
				}
				e.SaveX(true)
			} else {
				// Draw mode
				e.pos.Left()
			}
			e.redrawCursor = true
		case "→": // right arrow
			if !e.DrawMode() {
				if e.DataY() < e.Len() {
					e.Next(c)
				}
				if e.AfterLineScreenContents() {
					e.End()
				}
				e.SaveX(true)
			} else {
				// Draw mode
				e.pos.Right(c)
			}
			e.redrawCursor = true
		case "↑": // up arrow
			// Move the screen cursor
			if !e.DrawMode() {
				if e.DataY() > 0 {
					// Move the position up in the current screen
					if e.UpEnd(c) != nil {
						// If below the top, scroll the contents up
						if e.DataY() > 0 {
							e.redraw = e.ScrollUp(c, status, 1)
							e.redrawCursor = true
							e.pos.Down(c)
							e.UpEnd(c)
						}
					}
					// If the cursor is after the length of the current line, move it to the end of the current line
					if e.AfterLineScreenContents() {
						e.End()
					}
				}
				// If the cursor is after the length of the current line, move it to the end of the current line
				if e.AfterLineScreenContents() {
					e.End()
				}
			} else {
				e.pos.Up()
			}
			e.redrawCursor = true
		case "↓": // down arrow
			if !e.DrawMode() {
				if e.DataY() < e.Len() {
					// Move the position down in the current screen
					if e.DownEnd(c) != nil {
						// If at the bottom, don't move down, but scroll the contents
						// Output a helpful message
						if !e.AfterEndOfDocument() {
							e.redraw = e.ScrollDown(c, status, 1)
							e.redrawCursor = true
							e.pos.Up()
							e.DownEnd(c)
						}
					}
					// If the cursor is after the length of the current line, move it to the end of the current line
					if e.AfterLineScreenContents() {
						e.End()
					}
				}
				// If the cursor is after the length of the current line, move it to the end of the current line
				if e.AfterLineScreenContents() {
					e.End()
				}
			} else {
				e.pos.Down(c)
			}
			e.redrawCursor = true
		case "c:14": // ctrl-n, scroll down
			e.redraw = e.ScrollDown(c, status, e.pos.scrollSpeed)
			e.redrawCursor = true
			if !e.DrawMode() && e.AfterLineScreenContents() {
				e.End()
			}
		case "c:16": // ctrl-p, scroll up
			e.redraw = e.ScrollUp(c, status, e.pos.scrollSpeed)
			e.redrawCursor = true
			if !e.DrawMode() && e.AfterLineScreenContents() {
				e.End()
			}
		case "c:8": // ctrl-h, help
			status.SetMessage("[" + versionString + "] ctrl-s to save, ctrl-q to quit")
			status.Show(c, e)
		case "c:20": // ctrl-t, toggle syntax highlighting
			e.ToggleHighlight()
			if e.highlight {
				e.bg = defaultEditorBackground
			} else {
				e.bg = vt100.BackgroundDefault
			}
			// Now do a full reset/redraw
			fallthrough
		case "c:27": // esc, clear search term, reset, clean and redraw
			status.ClearAll(c)
			e.SetSearchTerm("", c, status)
			vt100.Close()
			vt100.Reset()
			vt100.Clear()
			vt100.Init()
			c = vt100.NewCanvas()
			c.ShowCursor()
			e.redrawCursor = true
			e.redraw = true
		case " ": // space
			undo.Snapshot(e)
			// Place a space
			if !e.DrawMode() {
				e.InsertRune(' ')
				e.redraw = true
			} else {
				e.SetRune(' ')
			}
			e.WriteRune(c)
			if e.DrawMode() {
				e.redraw = true
			} else {
				// Move to the next position
				e.Next(c)
			}
		case "c:13": // return
			undo.Snapshot(e)
			// if the current line is empty, insert a blank line
			if !e.DrawMode() {
				e.TrimRight(e.DataY())
				lineContents := e.CurrentLine()
				e.FirstScreenPosition(e.DataY())
				if e.pos.AtStartOfLine() {
					// Insert a new line a the current y position, then shift the rest down.
					e.InsertLineAbove()
					// Also move the cursor to the start, since it's now on a new blank line.
					e.pos.Down(c)
					e.Home()
				} else if e.AtOrBeforeStartOfTextLine() {
					x := e.pos.ScreenX()
					// Insert a new line a the current y position, then shift the rest down.
					e.InsertLineAbove()
					// Also move the cursor to the start, since it's now on a new blank line.
					e.pos.Down(c)
					e.pos.SetX(x)
				} else if e.AtOrAfterEndOfLine() && e.AtLastLineOfDocument() {
					leadingWhitespace := e.LeadingWhitespace()
					if len(lineContents) > 0 && (strings.HasSuffix(lineContents, "(") || strings.HasSuffix(lineContents, "{") || strings.HasSuffix(lineContents, "[")) {
						// "smart indentation"
						leadingWhitespace += "\t"
					}
					e.InsertLineBelow()
					h := int(c.Height())
					if e.pos.sy >= (h - 1) {
						e.ScrollDown(c, status, 1)
						e.redrawCursor = true
					}
					e.pos.Down(c)
					e.Home()
					// Insert the same leading whitespace for the new line, while moving to the right
					for _, r := range leadingWhitespace {
						e.InsertRune(r)
						e.Next(c)
					}
				} else if e.AfterEndOfLine() {
					leadingWhitespace := e.LeadingWhitespace()
					if len(lineContents) > 0 && (strings.HasSuffix(lineContents, "(") || strings.HasSuffix(lineContents, "{") || strings.HasSuffix(lineContents, "[")) {
						// "smart indentation"
						leadingWhitespace += "\t"
					}
					e.InsertLineBelow()
					e.Down(c, status)
					e.Home()
					// Insert the same leading whitespace for the new line, while moving to the right
					for _, r := range leadingWhitespace {
						e.InsertRune(r)
						e.Next(c)
					}
				} else {
					// Split the current line in two
					if !e.SplitLine() {
						// Grab the leading whitespace from the current line
						leadingWhitespace := e.LeadingWhitespace()
						// Insert a line below, then move down and to the start of it
						e.InsertLineBelow()
						e.Down(c, status)
						e.Home()
						// Insert the same leading whitespace for the new line, while moving to the right
						for _, r := range leadingWhitespace {
							e.InsertRune(r)
							e.Next(c)
						}
					} else {
						e.Down(c, status)
						e.Home()
					}
				}
			} else {
				if e.AtLastLineOfDocument() {
					e.CreateLineIfMissing(e.DataY() + 1)
				}
				e.pos.Down(c)
			}
			e.redraw = true
		case "c:127": // backspace
			undo.Snapshot(e)
			if !e.DrawMode() && e.EmptyLine() {
				e.DeleteLine(e.DataY())
				e.pos.Up()
				e.TrimRight(e.DataY())
				e.End()
			} else if !e.DrawMode() && e.pos.AtStartOfLine() {
				if e.DataY() > 0 {
					e.pos.Up()
					e.End()
					e.TrimRight(e.DataY())
					e.Delete()
				}
			} else {
				// Move back
				e.Prev(c)
				// Type a blank
				e.SetRune(' ')
				e.WriteRune(c)
				if !e.DrawMode() && !e.AtOrAfterEndOfLine() {
					// Delete the blank
					e.Delete()
				}
			}
			e.redrawCursor = true
			e.redraw = true
		case "c:9": // tab
			undo.Snapshot(e)
			if !e.DrawMode() {
				// Place a tab
				if !e.DrawMode() {
					e.InsertRune('\t')
				} else {
					e.SetRune('\t')
				}
				// Write the spaces that represent the tab
				e.WriteTab(c)
				// Move to the next position
				if !e.DrawMode() {
					e.Next(c)
				}
			}
			e.redrawCursor = true
			e.redraw = true
		case "c:1": // ctrl-a, home
			// toggle between start of line and start of non-whitespace
			if e.AtStartOfTextLine() {
				e.Home()
			} else {
				e.pos.SetX(e.FirstScreenPosition(e.DataY()))
			}
			e.SaveX(true)
		case "c:5": // ctrl-e, end
			if e.AfterEndOfLine() { // && !e.EmptyLine() {
				// go to the end of the next line if already at the end of the line
				e.Down(c, status)
				e.End()
			} else {
				e.End()
			}
			e.SaveX(true)
		case "c:4": // ctrl-d, delete
			undo.Snapshot(e)
			if e.Empty() {
				status.SetMessage("Empty")
				status.Show(c, e)
			} else {
				e.Delete()
				e.redraw = true
			}
			e.redrawCursor = true
		case "c:19": // ctrl-s, save
			if err := e.Save(filename, !e.DrawMode()); err != nil {
				status.SetMessage(err.Error())
				status.Show(c, e)
			} else {
				// TODO: Go to the end of the document at this point, if needed
				// Lines may be trimmed for whitespace, so move to the end, if needed
				if !e.DrawMode() && e.AfterLineScreenContents() {
					e.End()
				}
				// Status message
				status.SetMessage("Saved " + filename)
				status.Show(c, e)
				c.Draw()
			}
		case "c:21", "c:26": // ctrl-u or ctrl-z, undo (ctrl-z may background the application)
			if err := undo.Restore(e); err == nil {
				//c.Draw()
				x := e.pos.ScreenX()
				y := e.pos.ScreenY()
				vt100.SetXY(uint(x), uint(y))
				e.redrawCursor = true
				e.redraw = true
			} else {
				status.SetMessage("Nothing more to undo")
				status.Show(c, e)
			}
		case "c:12": // ctrl-l, go to line number
			status.ClearAll(c)
			status.SetMessage("Go to line number:")
			status.ShowNoTimeout(c, e)
			lns := ""
			doneCollectingDigits := false
			for !doneCollectingDigits {
				numkey := tty.KeyBlock()
				switch numkey {
				case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9": // 0 .. 9
					lns += numkey // string('0' + (numkey - 48))
					status.SetMessage("Go to line number: " + lns)
					status.ShowNoTimeout(c, e)
				case "c:127": // backspace
					if len(lns) > 0 {
						lns = lns[:len(lns)-1]
						status.SetMessage("Go to line number: " + lns)
						status.ShowNoTimeout(c, e)
					}
				case "c:27", "c:17": // esc or ctrl-q
					lns = ""
					fallthrough
				case "c:13": // return
					doneCollectingDigits = true
				}
			}
			status.ClearAll(c)
			if lns != "" {
				if ln, err := strconv.Atoi(lns); err == nil { // no error
					e.redraw = e.GoToLineNumber(ln, c, status)
				}
			}
			e.redrawCursor = true
		case "c:11": // ctrl-k, delete to end of line
			undo.Snapshot(e)
			if e.Empty() {
				status.SetMessage("Empty")
				status.Show(c, e)
			} else {
				e.DeleteRestOfLine()
				if !e.DrawMode() && e.EmptyRightTrimmedLine() {
					// Deleting the rest of the line cleared this line,
					// so just remove it.
					e.DeleteLine(e.DataY())
				}
				vt100.Do("Erase End of Line")
				e.redraw = true
			}
			e.redrawCursor = true
		case "c:24": // ctrl-x, cut line
			undo.Snapshot(e)
			y := e.DataY()
			copyLine = e.Line(y)
			e.DeleteLine(y)
			e.redrawCursor = true
			e.redraw = true
		case "c:3": // ctrl-c, copy line
			copyLine = e.Line(e.DataY())
			e.redraw = true
		case "c:22": // ctrl-v, paste line
			undo.Snapshot(e)
			e.SetLine(e.DataY(), copyLine)
			e.End()
			e.redrawCursor = true
			e.redraw = true
		case "c:2": // ctrl-b, bookmark
			bookmark = e.pos
		case "c:10": // ctrl-j, jump to bookmark
			// TODO: Add a check for if a bookmark exists?
			e.pos = bookmark
			e.redraw = true
		default:
			if unicode.IsLetter([]rune(key)[0]) { // letter
				undo.Snapshot(e)
				dropO := false
				if firstLetterSinceStart == "" {
					firstLetterSinceStart = key
				} else if firstLetterSinceStart == "O" && ([]rune(key)[0] >= 'A' && []rune(key)[0] <= 'Z') {
					// If the first typed letter since starting this editor was 'O', and this is also uppercase,
					// then disregard the initial 'O'. This is to help vim-users.
					dropO = true
					// Set the first letter since start to something that will not trigger this branch any more.
					firstLetterSinceStart = "x"
				}
				if dropO {
					// Replace the previous letter.
					e.Prev(c)
					e.SetRune([]rune(key)[0])
					e.WriteRune(c)
					e.Next(c)
				} else if !e.DrawMode() {
					// Insert a letter. This is what normally happens.
					e.InsertRune([]rune(key)[0])
					e.WriteRune(c)
					e.Next(c)
				} else {
					// Replace this letter.
					e.SetRune([]rune(key)[0])
					e.WriteRune(c)
				}
				e.redraw = true
			} else if key != "" { // any other key
				undo.Snapshot(e)
				// Place *something*
				r := []rune(key)[0]

				// "smart dedent"
				if r == '}' || r == ']' || r == ')' {
					lineContents := strings.TrimSpace(e.CurrentLine())
					whitespaceInFront := e.LeadingWhitespace()
					if len(lineContents) == 0 && len(whitespaceInFront) > 0 {
						// move one step left
						e.Prev(c)
						// trim trailing whitespace
						e.TrimRight(e.DataY())
					}
				}

				if !e.DrawMode() {
					e.InsertRune([]rune(key)[0])
				} else {
					e.SetRune([]rune(key)[0])
				}
				e.WriteRune(c)
				if len(string(r)) > 0 {
					if !e.DrawMode() {
						// Move to the next position
						e.Next(c)
					}
				}
				e.redrawCursor = true
				e.redraw = true
			}
		}
		if statusMode {
			status.ShowLineColWordCount(c, e, filename)
		}
		if e.redraw {
			// Draw the editor lines on the canvas, respecting the offset
			e.DrawLines(c, true, false)
			e.redraw = false
		} else if e.Changed() {
			c.Draw()
		}
		x := e.pos.ScreenX()
		y := e.pos.ScreenY()
		if e.redrawCursor || x != previousX || y != previousY {
			vt100.SetXY(uint(x), uint(y))
			e.redrawCursor = false
		}
		previousX = x
		previousY = y
	}
	tty.Close()
	vt100.Clear()
	vt100.Close()
	//fmt.Println(filename)
}
