package ui

// Contact photos are rendered with the kitty graphics protocol using Unicode
// placeholders (a.k.a. virtual placements). The pixel data is transmitted to
// the terminal once per image; the on-screen avatar is then a grid of U+10EEEE
// placeholder cells whose foreground color encodes the image id and whose
// combining diacritics encode the row/column of the image each cell shows.
//
// Why placeholders instead of direct placement: Bubble Tea renders a text
// cell-grid and diffs frames, and lipgloss reflows column bodies. A real image
// escape dumped into that pipeline gets mismeasured and clobbered. Placeholder
// cells are ordinary width-1 glyphs, so layout and redraws behave, and the
// image re-displays from the terminal's store on every repaint without
// retransmitting.
//
// Under tmux the graphics escapes must be wrapped in passthrough sequences and
// the pane must have allow-passthrough on, which we enable at startup.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"  // register decoders
	_ "image/jpeg" // register decoders
	"image/png"
	"os"
	"os/exec"
	"strings"
)

// placeholder is kitty's Unicode placeholder codepoint.
const placeholder = "\U0010EEEE"

// diacritics encode row/column indices for placeholder cells. Kitty defines 297
// of them; the first 64 cover any avatar we draw. Derived from kitty's
// gen/rowcolumn-diacritics.txt.
var diacritics = []rune{
	0x305, 0x30d, 0x30e, 0x310, 0x312, 0x33d, 0x33e, 0x33f,
	0x346, 0x34a, 0x34b, 0x34c, 0x350, 0x351, 0x352, 0x357,
	0x35b, 0x363, 0x364, 0x365, 0x366, 0x367, 0x368, 0x369,
	0x36a, 0x36b, 0x36c, 0x36d, 0x36e, 0x36f, 0x483, 0x484,
	0x485, 0x486, 0x487, 0x592, 0x593, 0x594, 0x595, 0x597,
	0x598, 0x599, 0x59c, 0x59d, 0x59e, 0x59f, 0x5a0, 0x5a1,
	0x5a8, 0x5a9, 0x5ab, 0x5ac, 0x5af, 0x5c4, 0x610, 0x611,
	0x612, 0x613, 0x614, 0x615, 0x616, 0x617, 0x657, 0x658,
}

// graphics detects terminal support once and remembers which images have been
// transmitted, so a photo's pixels are sent at most once.
type graphics struct {
	enabled bool
	tmux    bool
	out     *os.File
	sent    map[uint32]bool
}

func newGraphics() *graphics {
	g := &graphics{
		enabled: graphicsCapable(),
		tmux:    os.Getenv("TMUX") != "",
		out:     os.Stdout,
		sent:    map[uint32]bool{},
	}
	if g.enabled && g.tmux {
		// Per-pane so we don't touch the user's global tmux config. Best effort.
		_ = exec.Command("tmux", "set-option", "-p", "allow-passthrough", "on").Run()
	}
	return g
}

// graphicsCapable reports whether the terminal speaks the kitty graphics
// protocol. Detection is env-based because querying the terminal directly
// fights Bubble Tea for stdin.
func graphicsCapable() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" || os.Getenv("GHOSTTY_BIN_DIR") != "" {
		return true
	}
	if term := os.Getenv("TERM"); strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") {
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty", "WezTerm":
		return true
	}
	return false
}

// avatar returns the placeholder grid for an image, transmitting its pixels to
// the terminal the first time it is seen. cols bounds the avatar width; rows is
// derived from the image's aspect ratio (terminal cells are ~twice as tall as
// wide). ok is false when graphics are unavailable or the image can't decode,
// in which case the caller should fall back to text.
func (g *graphics) avatar(data []byte, cols int) (block string, rows int, ok bool) {
	if !g.enabled || len(data) == 0 || cols < 2 {
		return "", 0, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", 0, false
	}
	cols = clamp(cols, 2, len(diacritics))

	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw == 0 || ih == 0 {
		return "", 0, false
	}
	rows = clamp(cols*ih/(iw*2), 1, min(12, len(diacritics)))

	id := imageID(data)
	if !g.sent[id] {
		if err := g.transmit(id, downscale(img, 320), cols, rows); err != nil {
			return "", 0, false
		}
		g.sent[id] = true
	}
	return placeholderBlock(id, cols, rows), rows, true
}

// transmit sends the image's pixels to the terminal as a PNG, creating a
// virtual placement (U=1) for Unicode placeholders. Written straight to stdout
// — not through lipgloss/Bubble Tea — so the large payload isn't mismeasured;
// because this runs during View(), the bytes precede the frame that uses them.
func (g *graphics) transmit(id uint32, img image.Image, cols, rows int) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())

	const chunkSize = 4096
	var out strings.Builder
	first := true
	for len(payload) > 0 {
		n := min(chunkSize, len(payload))
		part := payload[:n]
		payload = payload[n:]
		more := 0
		if len(payload) > 0 {
			more = 1
		}
		var esc string
		if first {
			esc = fmt.Sprintf("\x1b_Ga=T,U=1,i=%d,f=100,c=%d,r=%d,q=2,m=%d;%s\x1b\\",
				id, cols, rows, more, part)
			first = false
		} else {
			esc = fmt.Sprintf("\x1b_Gm=%d;%s\x1b\\", more, part)
		}
		if g.tmux {
			esc = tmuxWrap(esc)
		}
		out.WriteString(esc)
	}
	_, err := g.out.WriteString(out.String())
	return err
}

// placeholderBlock builds the rows×cols grid of placeholder cells. The image id
// is carried in each line's foreground color; row/column come from diacritics.
func placeholderBlock(id uint32, cols, rows int) string {
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", (id>>16)&0xff, (id>>8)&0xff, id&0xff)
	var b strings.Builder
	for r := 0; r < rows; r++ {
		b.WriteString(fg)
		for c := 0; c < cols; c++ {
			b.WriteString(placeholder)
			b.WriteRune(diacritics[r])
			b.WriteRune(diacritics[c])
		}
		b.WriteString("\x1b[39m")
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// tmuxWrap encloses a terminal escape in a tmux passthrough sequence, doubling
// inner ESCs as tmux requires.
func tmuxWrap(s string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(s, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// imageID hashes the photo bytes to a stable 24-bit, nonzero kitty image id
// (24 bits so it fits a truecolor foreground without a high-byte diacritic).
func imageID(data []byte) uint32 {
	const offset, prime = 2166136261, 16777619
	h := uint32(offset)
	for _, c := range data {
		h = (h ^ uint32(c)) * prime
	}
	id := h & 0xffffff
	if id == 0 {
		id = 1
	}
	return id
}

// downscale shrinks img so its longest edge is at most max, using
// nearest-neighbor sampling (the terminal rescales to the cell box anyway; this
// only bounds the transmitted payload). Images already within bounds pass through.
func downscale(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	nw, nh := w, h
	if w >= h {
		nw, nh = max, h*max/w
	} else {
		nw, nh = w*max/h, max
	}
	if nh < 1 {
		nh = 1
	}
	if nw < 1 {
		nw = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*w/nw
			dst.Set(x, y, img.At(sx, sy))
		}
	}
	return dst
}

// avatarWidth picks the avatar's cell width for a detail column of inner width w.
func avatarWidth(w int) int {
	return clamp(w/2, 6, 18)
}
