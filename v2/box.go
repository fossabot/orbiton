package main

import (
	"github.com/xyproto/vt100"
)

// Box is a position, width and height
type Box struct {
	X int
	Y int
	W int
	H int
}

// BoxTheme contains the runes used to draw boxes
type BoxTheme struct {
	TL, TR, BL, BR, VL, VR, HT, HB rune
}

// NewBox creates a new box/container
func NewBox() *Box {
	return &Box{0, 0, 0, 0}
}

// NewBoxTheme creates a new theme/style for a box/container
func NewBoxTheme() *BoxTheme {
	// TODO: Respect the color settings from in theme.go
	return &BoxTheme{
		TL: '╭', // top left
		TR: '╮', // top right
		BL: '╰', // bottom left
		BR: '╯', // bottom right
		VL: '│', // vertical line, left side
		VR: '│', // vertical line, right side
		HT: '─', // horizontal line
		HB: '─', // horizontal bottom line
	}
}

// NewCanvasBox creates a new box/container for the entire canvas/screen
func NewCanvasBox(c *vt100.Canvas) *Box {
	w := int(c.W())
	h := int(c.H())
	return &Box{0, 0, w, h}
}

// Center will place a Box at the center of the given container.
func (b *Box) Center(container *Box) {
	widthleftover := container.W - b.W
	heightleftover := container.H - b.H
	b.X = container.X + widthleftover/2
	b.Y = container.Y + heightleftover/2
}

// Fill will place a Box so that it fills the entire given container.
func (b *Box) Fill(container *Box) {
	b.X = container.X
	b.Y = container.Y
	b.W = container.W
	b.H = container.H
}

// FillWithMargins will place a Box inside a given container, with the given margins.
// Margins are given in number of characters.
func (b *Box) FillWithMargins(container *Box, margins int) {
	b.Fill(container)
	b.X += margins
	b.Y += margins
	b.W -= margins * 2
	b.H -= margins * 2
}

// UpperRightPlacement will place a box in the upper right corner of a container, like a little window
func (b *Box) UpperRightPlacement(container *Box) {
	w := float64(container.W)
	h := float64(container.H)
	b.X = int(w * 0.6)
	b.Y = int(h * 0.1)
	b.W = int(w * 0.3)
	b.H = int(h * 0.2)
}

// LowerRightPlacement will place a box in the lower right corner of a container, like a little window
func (b *Box) LowerRightPlacement(container *Box) {
	w := float64(container.W)
	h := float64(container.H)
	b.X = int(w * 0.6)
	b.Y = int(h * 0.4)
	b.W = int(w * 0.3)
	b.H = int(h * 0.5)
}

// LowerPlacement will place a box in the lower right corner of a container, like a little window
func (b *Box) LowerPlacement(container *Box) {
	w := float64(container.W)
	h := float64(container.H)
	b.X = int(w * 0.1)
	b.Y = int(h * 0.3)
	b.W = int(w * 0.8)
	b.H = int(h * 0.7)
}

// Say will output text at the given coordinates, with the configured theme
func (e *Editor) Say(c *vt100.Canvas, x, y int, text string) {
	c.Write(uint(x), uint(y), e.BoxTextColor, e.BoxBackground, text)
}

// DrawBox can draw a box using "text graphics".
// The given Box struct defines the size and placement.
// If extrude is True, the box looks a bit more like it's sticking out.
func (e *Editor) DrawBox(t *BoxTheme, c *vt100.Canvas, r *Box, extrude bool) *Box {
	x := uint(r.X)
	y := uint(r.Y)
	width := uint(r.W)
	height := uint(r.H)
	FG1 := e.StatusForeground
	FG2 := e.BoxTextColor
	if !extrude {
		FG1 = e.BoxTextColor
		FG2 = e.StatusForeground
	}
	c.WriteRune(x, y, FG1, e.BoxBackground, t.TL)
	//c.Write(x+1, y, FG1, e.BoxBackground, RepeatRune(t.HT, width-2))
	for i := x + 1; i < x+(width-1); i++ {
		c.WriteRune(i, y, FG1, e.BoxBackground, t.HT)
	}
	c.WriteRune(x+width-1, y, FG1, e.BoxBackground, t.TR)
	for i := y + 1; i < y+height; i++ {
		c.WriteRune(x, i, FG1, e.BoxBackground, t.VL)
		c.Write(x+1, i, FG1, e.BoxBackground, repeatRune(' ', width-2))
		c.WriteRune(x+width-1, i, FG2, e.BoxBackground, t.VR)
	}
	c.WriteRune(x, y+height-1, FG1, e.BoxBackground, t.BL)
	for i := x + 1; i < x+(width-1); i++ {
		c.WriteRune(i, y+height-1, FG2, e.BoxBackground, t.HB)
	}
	//c.Write(x+1, y+height-1, FG2, e.BoxBackground, RepeatRune(t.HB, width-2))
	c.WriteRune(x+width-1, y+height-1, FG2, e.BoxBackground, t.BR)
	return &Box{int(x), int(y), int(width), int(height)}
}

// DrawList will draw a list widget. Takes a Box struct for the size and position.
// Takes a list of strings to be listed and an int that represents
// which item is currently selected. Does not scroll or wrap.
// Set selected to -1 to skip highlighting one of the items.
func (e *Editor) DrawList(c *vt100.Canvas, r *Box, items []string, selected int) {
	for i, s := range items {
		color := e.BoxTextColor
		if i == selected {
			color = e.BoxHighlight
		}
		c.Write(uint(r.X), uint(r.Y+i), color, e.BoxBackground, s)
	}
}

// DrawTitle draws a title at the top of a box, not exactly centered
func (e *Editor) DrawTitle(c *vt100.Canvas, r *Box, title string) {
	titleWithSpaces := " " + title + " "
	e.Say(c, r.X+(r.W-len(titleWithSpaces))/2, r.Y, titleWithSpaces)
}

// DrawRaw can output a multiline string at the given coordinates.
// Uses the default background color.
// Returns the final y coordinate after drawing.
func (e *Editor) DrawRaw(c *vt100.Canvas, x, y int, text string) int {
	var i int
	for i, line := range splitTrim(text) {
		c.Write(uint(x), uint(y+i), e.Foreground, e.BoxBackground, line)
	}
	return y + i
}