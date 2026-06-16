package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zackb/yoro/internal/model"
	"github.com/zackb/yoro/internal/store"
)

// contactsPane is the three-column miller view: books | contacts | detail.
type contactsPane struct {
	theme Theme
	keys  KeyMap
	store store.Store

	width, height int

	allBooks     []model.Collection // every address book, all sources
	sources      []store.SourceInfo // sources that have at least one book, in order
	activeSource string             // id of the source whose books are shown
	counts       map[string]int     // contact count per book ID, computed on refresh

	books   []model.Collection // books of the active source
	bookIdx int

	all      []model.Contact // contacts of the selected book
	filtered []int           // indices into all (search result / identity)
	curIdx   int             // index into filtered
	focus    focusCol

	searching bool
	search    textinput.Model
	query     string

	gfx    *graphics
	status string
}

func newContactsPane(theme Theme, keys KeyMap, st store.Store) *contactsPane {
	ti := textinput.New()
	ti.Prompt = IconSearch + " "
	ti.Placeholder = "search contacts"
	return &contactsPane{
		theme:  theme,
		keys:   keys,
		store:  st,
		search: ti,
		focus:  focusMiddle,
		gfx:    newGraphics(),
	}
}

func (p *contactsPane) refresh() {
	p.allBooks = p.store.AddressBooks()
	p.sources = sourcesWithBooks(p.store.Sources(), p.allBooks)
	p.counts = make(map[string]int, len(p.allBooks))
	for _, b := range p.allBooks {
		p.counts[b.ID] = len(p.store.Contacts(b.ID))
	}
	if _, ok := p.sourceByID(p.activeSource); !ok {
		p.activeSource = ""
		if len(p.sources) > 0 {
			p.activeSource = p.sources[0].ID
		}
	}
	p.filterBooks()
}

// filterBooks narrows allBooks to the active source. With a single source the
// filter is a no-op, so behavior is unchanged from before.
func (p *contactsPane) filterBooks() {
	p.books = p.books[:0]
	for _, b := range p.allBooks {
		if p.activeSource == "" || b.Source == p.activeSource {
			p.books = append(p.books, b)
		}
	}
	p.bookIdx = clamp(p.bookIdx, 0, max0(len(p.books)-1))
	p.loadBook()
}

// cycleSource switches the active contacts source (contacts are browsed one
// source at a time to avoid duplicate people across mirrored sources).
func (p *contactsPane) cycleSource() {
	if len(p.sources) < 2 {
		return
	}
	cur := 0
	for i, s := range p.sources {
		if s.ID == p.activeSource {
			cur = i
			break
		}
	}
	next := p.sources[(cur+1)%len(p.sources)]
	p.activeSource = next.ID
	p.bookIdx = 0
	p.curIdx = 0
	p.filterBooks()
	p.status = "source: " + next.Name
}

func (p *contactsPane) activeSourceName() string {
	if s, ok := p.sourceByID(p.activeSource); ok {
		return s.Name
	}
	return ""
}

func (p *contactsPane) sourceByID(id string) (store.SourceInfo, bool) {
	for _, s := range p.sources {
		if s.ID == id {
			return s, true
		}
	}
	return store.SourceInfo{}, false
}

func (p *contactsPane) loadBook() {
	p.all = nil
	if len(p.books) > 0 {
		p.all = p.store.Contacts(p.books[p.bookIdx].ID)
	}
	p.applyFilter()
}

func (p *contactsPane) applyFilter() {
	p.filtered = p.filtered[:0]
	q := strings.ToLower(strings.TrimSpace(p.query))
	for i, c := range p.all {
		if q == "" || contactContains(c, q) {
			p.filtered = append(p.filtered, i)
		}
	}
	p.curIdx = clamp(p.curIdx, 0, max0(len(p.filtered)-1))
}

func (p *contactsPane) setSize(w, h int) { p.width, p.height = w, h }

func (p *contactsPane) Update(msg tea.Msg) (tea.Cmd, bool) {
	if p.searching {
		return p.updateSearch(msg)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch {
	case key.Matches(km, p.keys.Left):
		if p.focus > focusLeft {
			p.focus--
		}
	case key.Matches(km, p.keys.Right):
		if p.focus < focusRight {
			p.focus++
		}
	case key.Matches(km, p.keys.Down):
		p.move(1)
	case key.Matches(km, p.keys.Up):
		p.move(-1)
	case key.Matches(km, p.keys.HalfDown):
		p.move(p.listHeight() / 2)
	case key.Matches(km, p.keys.HalfUp):
		p.move(-p.listHeight() / 2)
	case key.Matches(km, p.keys.PageDown):
		p.move(p.listHeight())
	case key.Matches(km, p.keys.PageUp):
		p.move(-p.listHeight())
	case key.Matches(km, p.keys.Bottom):
		p.moveTo(len(p.filtered) - 1)
	case key.Matches(km, p.keys.Top):
		if km.String() == "g" {
			p.moveTo(0)
		}
	case key.Matches(km, p.keys.Search):
		p.startSearch()
	case key.Matches(km, p.keys.Yank):
		return p.yank(), true
	case key.Matches(km, p.keys.SwitchSource):
		p.cycleSource()
	case key.Matches(km, p.keys.Escape):
		if p.query != "" {
			p.query = ""
			p.applyFilter()
		}
	default:
		return nil, false
	}
	return nil, true
}

func (p *contactsPane) updateSearch(msg tea.Msg) (tea.Cmd, bool) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			p.query = p.search.Value()
			p.searching = false
			p.search.Blur()
			p.applyFilter()
			return nil, true
		case "esc":
			p.searching = false
			p.search.Blur()
			return nil, true
		}
	}
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	p.query = p.search.Value()
	p.applyFilter()
	return cmd, true
}

func (p *contactsPane) startSearch() {
	p.searching = true
	p.focus = focusMiddle
	p.search.SetValue(p.query)
	p.search.CursorEnd()
	p.search.Focus()
}

func (p *contactsPane) move(d int) { p.moveTo(p.curIdx + d) }
func (p *contactsPane) moveTo(i int) {
	if p.focus == focusLeft {
		if len(p.books) > 0 {
			p.bookIdx = clamp(i, 0, len(p.books)-1)
			p.curIdx = 0
			p.loadBook()
		}
		return
	}
	p.curIdx = clamp(i, 0, max0(len(p.filtered)-1))
}

// selectedBook returns the active address book, the target for a new contact.
func (p *contactsPane) selectedBook() (model.Collection, bool) {
	if p.bookIdx < 0 || p.bookIdx >= len(p.books) {
		return model.Collection{}, false
	}
	return p.books[p.bookIdx], true
}

func (p *contactsPane) selected() (model.Contact, bool) {
	if p.curIdx < 0 || p.curIdx >= len(p.filtered) {
		return model.Contact{}, false
	}
	return p.all[p.filtered[p.curIdx]], true
}

func (p *contactsPane) yank() tea.Cmd {
	c, ok := p.selected()
	if !ok {
		return nil
	}
	var val string
	switch {
	case len(c.Emails) > 0:
		val = c.Emails[0].Value
	case len(c.Phones) > 0:
		val = c.Phones[0].Value
	default:
		p.status = "nothing to yank"
		return nil
	}
	p.status = "yanked " + val
	return copyToClipboard(val)
}

func (p *contactsPane) listHeight() int { return max0(p.height - 2) }

// View renders the three columns side by side within w x h.
func (p *contactsPane) View() string {
	w, h := p.width, p.height
	bookW, listW, detailW := threeColumns(w, 22, 16, 38, 28, 52)

	booksTitle := "ADDRESS BOOKS"
	if len(p.sources) > 1 {
		booksTitle = "BOOKS · " + Truncate(p.activeSourceName(), bookW-10)
	}
	books := p.theme.Column(booksTitle, p.booksBody(bookW-2), bookW, h, p.focus == focusLeft)
	listTitle := fmt.Sprintf("CONTACTS (%d)", len(p.filtered))
	list := p.theme.Column(listTitle, p.listBody(listW-2, h-3), listW, h, p.focus == focusMiddle)
	detail := p.theme.Column("DETAIL", p.detailBody(detailW-2), detailW, h, p.focus == focusRight)
	return lipgloss.JoinHorizontal(lipgloss.Top, books, list, detail)
}

func (p *contactsPane) booksBody(w int) string {
	var b strings.Builder
	for i, col := range p.books {
		sel := i == p.bookIdx
		label := fmt.Sprintf("%s %s", IconContacts, col.Name)
		count := fmt.Sprintf(" %d", p.counts[col.ID])
		line := PadRight(Truncate(label, w-lipgloss.Width(count)), w-lipgloss.Width(count)) + count
		b.WriteString(p.theme.SelectStyle(sel, p.focus == focusLeft).Render(PadRight(line, w)))
		b.WriteByte('\n')
	}
	return b.String()
}

// rowPrefixW is the fixed leading width of every list row: a tiny photo
// thumbnail (or the source glyph) plus a separating space. Keeping it constant
// aligns the names regardless of which contacts have photos.
const rowPrefixW = 3

func (p *contactsPane) listBody(w, h int) string {
	if p.searching {
		// Show the search input on the first line, list below.
		p.search.Width = w - 2
	}
	names := make([]string, len(p.filtered))
	for i, idx := range p.filtered {
		names[i] = p.all[idx].DisplayName()
	}
	visible, top := scrollWindow(names, p.curIdx, max0(h))
	glyph := p.provenanceGlyph()
	nameW := max0(w - rowPrefixW)
	var b strings.Builder
	if p.searching {
		b.WriteString(p.search.View())
		b.WriteByte('\n')
	}
	// Build prefixes only for visible rows so off-screen photos aren't
	// transmitted to the terminal until scrolled into view.
	for i, name := range visible {
		idx := top + i
		prefix := p.rowPrefix(p.all[p.filtered[idx]], glyph)
		styled := p.theme.SelectStyle(idx == p.curIdx, p.focus == focusMiddle && !p.searching).
			Render(PadRight(Truncate(name, nameW), nameW))
		b.WriteString(prefix + styled)
		b.WriteByte('\n')
	}
	if len(p.filtered) == 0 {
		b.WriteString(p.theme.ItemDim.Render("no contacts"))
	}
	return b.String()
}

// rowPrefix returns the width-3 leading cell for a contact: a 2×1 photo
// thumbnail when one is available, else the source glyph. The thumbnail is raw
// (kept outside SelectStyle) so the placeholder's id-encoding color isn't
// overwritten by the row's foreground style.
func (p *contactsPane) rowPrefix(c model.Contact, glyph string) string {
	if block, ok := p.gfx.thumbnail(c.Photo, 2, 1); ok {
		return block + " "
	}
	return p.theme.ItemDim.Render(glyph) + "  "
}

// provenanceGlyph returns the cloud (DAV) or disk (local) glyph for the active
// source. Contacts are browsed one source at a time, so every visible row
// shares it — yazi-style provenance at a glance. Falls back to a person icon
// when the source type is unknown.
func (p *contactsPane) provenanceGlyph() string {
	if s, ok := p.sourceByID(p.activeSource); ok {
		return sourceGlyph(s.Type)
	}
	return IconPerson
}

func (p *contactsPane) detailBody(w int) string {
	c, ok := p.selected()
	if !ok {
		return p.theme.ItemDim.Render("no selection")
	}
	var b strings.Builder
	b.WriteString(p.theme.Title.Render(Truncate(c.DisplayName(), w)) + "\n")
	if sub := joinNonEmpty(" · ", c.Title, c.Role, c.Org); sub != "" {
		b.WriteString(p.theme.Label.Render(Truncate(fmt.Sprintf("%s %s", IconOrg, sub), w)) + "\n")
	}
	if c.Nickname != "" {
		b.WriteString(p.theme.Label.Render(Truncate(fmt.Sprintf("%s “%s”", IconPerson, c.Nickname), w)) + "\n")
	}
	b.WriteString("\n")

	if block, _, ok := p.gfx.avatar(c.Photo, avatarWidth(w)); ok {
		b.WriteString(block + "\n\n")
	} else {
		avatar := IconPerson
		if len(c.Photo) > 0 {
			avatar = IconPerson + " [photo]"
		}
		b.WriteString(p.theme.ItemDim.Render(avatar) + "\n\n")
	}

	for _, e := range c.Emails {
		b.WriteString(p.field(IconEmail, e.Value, typeLabel(e.Types), w))
	}
	for _, t := range c.Phones {
		b.WriteString(p.field(IconPhone, t.Value, typeLabel(t.Types), w))
	}
	if c.URL != "" {
		b.WriteString(p.field(IconLink, c.URL, "", w))
	}
	for _, a := range c.Addresses {
		if s := formatAddress(a); s != "" {
			b.WriteString(p.field(IconLocation, s, typeLabel(a.Types), w))
		}
	}
	if c.Birthday != nil {
		b.WriteString(p.field(IconCake, c.Birthday.Format("Jan 2, 2006"), "", w))
	}
	if c.Anniversary != nil {
		b.WriteString(p.field(IconHeart, c.Anniversary.Format("Jan 2, 2006"), "anniversary", w))
	}
	if c.Note != "" {
		b.WriteString("\n" + p.theme.Label.Render(IconNote+" note") + "\n")
		b.WriteString(p.theme.Value.Render(Truncate(c.Note, w)) + "\n")
	}
	return b.String()
}

func (p *contactsPane) field(icon, value, label string, w int) string {
	line := fmt.Sprintf("%s %s", icon, value)
	if label != "" {
		line += " " + p.theme.Label.Render("("+label+")")
	}
	return p.theme.Value.Render(Truncate(line, w)) + "\n"
}

func typeLabel(types []string) string { return strings.Join(types, "/") }

// joinNonEmpty joins the non-empty, trimmed parts with sep.
func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// formatAddress renders an address as a single comma-separated line.
func formatAddress(a model.Address) string {
	return joinNonEmpty(", ", a.Street, a.Locality, a.Region, a.PostalCode, a.Country)
}

func contactContains(c model.Contact, q string) bool {
	if strings.Contains(strings.ToLower(c.FN), q) {
		return true
	}
	for _, e := range c.Emails {
		if strings.Contains(strings.ToLower(e.Value), q) {
			return true
		}
	}
	for _, t := range c.Phones {
		if strings.Contains(strings.ToLower(t.Value), q) {
			return true
		}
	}
	return false
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// sourcesWithBooks returns the sources, in their configured order, that own at
// least one address book.
func sourcesWithBooks(sources []store.SourceInfo, books []model.Collection) []store.SourceInfo {
	present := map[string]bool{}
	for _, b := range books {
		present[b.Source] = true
	}
	var out []store.SourceInfo
	for _, s := range sources {
		if present[s.ID] {
			out = append(out, s)
		}
	}
	return out
}
