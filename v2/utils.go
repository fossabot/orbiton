package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/xyproto/env/v2"
)

// hasAnyPrefixWord checks if the given line is prefixed with any one of the given words
func hasAnyPrefixWord(line string, wordList []string) bool {
	for _, word := range wordList {
		if strings.HasPrefix(line, word+" ") {
			return true
		}
	}
	return false
}

// hasAnyPrefix checks if the given line is prefixed with any one of the given strings
func hasAnyPrefix(line string, stringList []string) bool {
	for _, s := range stringList {
		if strings.HasPrefix(line, s) {
			return true
		}
	}
	return false
}

// hasS checks if the given string slice contains the given string
func hasS(sl []string, s string) bool {
	for _, e := range sl {
		if e == s {
			return true
		}
	}
	return false
}

// firstWordContainsOneOf checks if the first word of the given string contains
// any one of the given strings
func firstWordContainsOneOf(s string, sl []string) bool {
	if s == "" {
		return false
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	firstWord := fields[0]
	for _, e := range sl {
		if strings.Contains(firstWord, e) {
			return true
		}
	}
	return false
}

// hasKey checks if the given string map contains the given key
func hasKey(m map[string]string, key string) bool {
	_, found := m[key]
	return found
}

// filterS returns all strings that makes the function f return true
func filterS(sl []string, f func(string) bool) []string {
	var results []string
	for _, e := range sl {
		if f(e) {
			results = append(results, e)
		}
	}
	return results
}

// equalStringSlices checks if two given string slices are equal or not
// returns true if they are equal
func equalStringSlices(a, b []string) bool {
	lena := len(a)
	if lena != len(b) {
		return false
	}
	for i := 0; i < lena; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Check if the given string only consists of the given rune,
// ignoring the other given runes.
func consistsOf(s string, e rune, ignore []rune) bool {
OUTER_LOOP:
	for _, r := range s {
		for _, x := range ignore {
			if r == x {
				continue OUTER_LOOP
			}
		}
		if r != e {
			return false
		}
	}
	return true
}

// hexDigit checks if the given rune is 0-9, a-f, A-F or x
func hexDigit(r rune) bool {
	switch r {
	case 'x', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'A', 'a', 'B', 'b', 'C', 'c', 'D', 'd', 'E', 'e', 'F', 'f':
		return true
	}
	return false
}

// hasWords checks if a range of more than one letter is found
func hasWords(s string) bool {
	letterCount := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letterCount++
		} else {
			letterCount = 0
		}
		if letterCount > 1 {
			return true
		}
	}
	return false
}

// allUpper checks if all letters in a string are uppercase
func allUpper(s string) bool {
	for _, r := range s {
		if !unicode.IsUpper(r) && unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// allLower checks if all letters in a string are lowercase
func allLower(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsLower(r) {
			return false
		}
	}
	return true
}

// runeCount counts the instances of r in the given string
func runeCount(s string, r rune) int {
	counter := 0
	for _, e := range s {
		if e == r {
			counter++
		}
	}
	return counter
}

// abs returns the absolute value of the given int
func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// distance returns the distance between two points
func distance(x1, x2, y1, y2 int) float64 {
	x1f := float64(x1)
	x2f := float64(x2)
	y1f := float64(y1)
	y2f := float64(y2)
	return math.Sqrt((x1f*x1f - x2f*x2f) + (y1f*y1f - y2f*y2f))
}

// runeFromUBytes returns a rune from a byte slice on the form "U+0000"
func runeFromUBytes(bs []byte) (rune, error) {
	if !bytes.HasPrefix(bs, []byte("U+")) && !bytes.HasPrefix(bs, []byte("u+")) {
		return rune(0), errors.New("not a rune on the form U+0000 or u+0000")
	}
	numberString := string(bs[2:])
	unicodeNumber, err := strconv.ParseUint(numberString, 16, 64)
	if err != nil {
		return rune(0), err
	}
	return rune(unicodeNumber), nil
}

// logf, for quick "printf-style" debugging
// Will call log.Fatalln if there are problems!
func logf(format string, args ...interface{}) {
	logFilename := filepath.Join(tempDir, "o.log")
	if isDarwin() {
		logFilename = "/tmp/o.log"
	}
	err := flogf(logFilename, format, args...)
	if err != nil {
		log.Fatalln(err)
	}
}

// Silence the "logf is unused" message by staticcheck
var _ = logf

// flogf, for logging to a file with a fprintf-style function
func flogf(logfile, format string, args ...interface{}) error {
	f, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		f, err = os.Create(logfile)
		if err != nil {
			return err
		}
	}
	_, err = f.WriteString(fmt.Sprintf(format, args...))
	if err != nil {
		return err
	}
	err = f.Sync()
	if err != nil {
		return err
	}
	return f.Close()
}

// // pplogf, for pretty print logging to a file, using
// // import "github.com/k0kubun/pp/v3"
// func pplogf(format string, args ...interface{}) error {
// 	logFilename := filepath.Join(tempDir, "o.log")
// 	f, err := os.OpenFile(logFilename, os.O_APPEND|os.O_WRONLY, 0644)
// 	if err != nil {
// 		f, err = os.Create(logFilename)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	prettyPrinter := pp.New()
// 	prettyPrinter.SetOutput(f)
// 	prettyPrinter.Printf(format, args...)
// 	err = f.Sync()
// 	if err != nil {
// 		return err
// 	}
// 	return f.Close()
// }

// repeatRune can repeat a rune, n number of times.
// Returns an empty string if memory can not be allocated within append.
func repeatRune(r rune, n uint) string {
	var sb strings.Builder
	for i := uint(0); i < n; i++ {
		_, err := sb.WriteRune(r)
		if err != nil {
			// In the unlikely event that append inside WriteRune won't work
			return ""
		}
	}
	return sb.String()
}

// capitalizeWords can change "john bob" to "John Bob"
func capitalizeWords(s string) string {
	words := strings.Fields(s)
	var newWords []string
	for _, word := range words {
		if len(word) > 1 {
			capitalizedWord := strings.ToUpper(string(word[0])) + word[1:]
			newWords = append(newWords, capitalizedWord)
		} else {
			newWords = append(newWords, word)
		}
	}
	return strings.Join(newWords, " ")
}

// getFullName tries to find the full name of the current user
func getFullName() (fullName string) {
	// Start out with whatever is in $LOGNAME, then capitalize the words
	fullName = capitalizeWords(env.Str("LOGNAME", "name"))
	// Then look for ~/.gitconfig
	gitConfigFilename := env.ExpandUser("~/.gitconfig")
	if exists(gitConfigFilename) {
		data, err := os.ReadFile(gitConfigFilename)
		if err != nil {
			return fullName
		}
		// Look for a line starting with "name =", in the "[user]" section
		inUserSection := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "[user]" {
				inUserSection = true
				continue
			} else if strings.HasPrefix(trimmedLine, "[") {
				inUserSection = false
				continue
			}
			if inUserSection && strings.HasPrefix(trimmedLine, "name =") {
				foundName := strings.TrimSpace(strings.SplitN(trimmedLine, "name =", 2)[1])
				if len(foundName) > len(fullName) {
					fullName = foundName
				}
			}
		}
	}
	return fullName
}

// onlyAZaz checks if the given string only contains letters a-z and A-Z
func onlyAZaz(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// lastEntryIsNot checks that the last entry of xs is not the given x
func lastEntryIsNot(xs []string, x string) bool {
	l := len(xs)
	if l == 0 {
		return true
	}
	return xs[l-1] != x
}

// manIsParent checks if the parent process is an executable named "man"
func manIsParent() bool {
	parentPID := os.Getppid()
	parentPath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", parentPID))
	if err != nil {
		return false
	}
	baseName := filepath.Base(parentPath)
	return baseName == "man"
}

// parentCommand returns either the command of the parent process or an empty string
func parentCommand() string {
	commandString, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", os.Getppid()))
	if err != nil {
		return ""
	}
	return string(commandString)
}
