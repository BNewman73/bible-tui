package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	// "strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Navigation states
type navState int

const (
	stateTestament navState = iota
	stateBook
	stateChapter
	stateVerse
	stateReading
)

// Bible data structures
type Testament struct {
	Name  string
	Books []Book
}

type Book struct {
	Name     string
	Chapters int
}

type Chapter struct {
	Number int
	Verses int
}

type Verse struct {
	Number int
	Text   string
}

// API Response structure
type BibleResponse struct {
	Reference string `json:"reference"`
	Verses    []struct {
		BookID   string `json:"book_id"` // Changed from int to string
		BookName string `json:"book_name"`
		Chapter  int    `json:"chapter"`
		Verse    int    `json:"verse"`
		Text     string `json:"text"`
	} `json:"verses"`
	Text            string `json:"text"`
	Translation     string `json:"translation_id"`
	TranslationName string `json:"translation_name"`
	TranslationNote string `json:"translation_note"`
}

// List item implementation
type item struct {
	title       string
	description string
	value       interface{}
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.title }

// Messages
type verseMsg BibleResponse
type errMsg error

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF06B7")).
			Bold(true)

	verseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#04B575"))

	referenceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Bold(true).
			Underline(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	breadcrumbStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true)
)

// Model
type model struct {
	list          list.Model
	viewport      viewport.Model
	spinner       spinner.Model
	state         navState
	loading       bool
	err           error
	content       string
	ready         bool
	breadcrumb    []string
	selectedTest  Testament
	selectedBook  Book
	selectedChap  int
	selectedVerse int
	width         int
	height        int
}

func initialModel() model {
	l := createTestamentList()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return model{
		list:    l,
		spinner: s,
		state:   stateTestament,
		loading: false,
		err:     nil,
		content: "",
		width:   80,
		height:  24,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "backspace":
			return m.handleBack()
		case "enter":
			return m.handleSelect()
		case "p":
			if m.state == stateReading {
				return m.navigateVerse(-1)
			}
		case "n":
			if m.state == stateReading {
				return m.navigateVerse(1)
			}
		}

	case verseMsg:
		m.loading = false
		m.content = formatBibleResponse(BibleResponse(msg), m.width-6) // Account for padding and borders
		m.state = stateReading
		if !m.ready {
			m.viewport = viewport.New(m.width-4, m.height-8)
			m.ready = true
		}
		m.viewport.SetContent(m.content)
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		h, v := docStyle.GetFrameSize()
		listWidth := msg.Width - h
		listHeight := msg.Height - v - 6 // Account for title and help text
		if listHeight < 10 {
			listHeight = 10
		}
		m.list.SetSize(listWidth, listHeight)

		if m.ready {
			// Update viewport size
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - 8

			// If we're currently reading, reformat content with new width
			if m.state == stateReading && m.content != "" {
				// Re-parse the original response to reformat with new width
				// For now, just update the viewport size
				m.viewport.SetContent(m.content)
			}
		}
	}

	if m.state == stateReading && m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	if !m.loading {
		m.list, cmd = m.list.Update(msg)
	}

	return m, cmd
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

func (m model) View() string {
	if m.state == stateReading {
		return m.renderReading()
	}

	var s strings.Builder

	// Title
	s.WriteString(titleStyle.Render("📖 Bible CLI Reader"))
	s.WriteString("\n\n")

	// Breadcrumb
	if len(m.breadcrumb) > 0 {
		s.WriteString(breadcrumbStyle.Render("📍 " + strings.Join(m.breadcrumb, " > ")))
		s.WriteString("\n\n")
	}

	// Loading indicator
	if m.loading {
		s.WriteString(m.spinner.View() + " Loading...")
		s.WriteString("\n\n")
	}

	// Error display
	if m.err != nil {
		s.WriteString(errorStyle.Render("Error: " + m.err.Error()))
		s.WriteString("\n\n")
	}

	// List
	s.WriteString(docStyle.Render(m.list.View()))

	/*
		// Help text
		s.WriteString("\n")
		helpText := "Enter: select • Delete: back • q/Ctrl+C: quit"
		s.WriteString(helpStyle.Render(helpText))
	*/

	return s.String()
}

func (m model) renderReading() string {
	var s strings.Builder

	// Title
	s.WriteString(titleStyle.Render("📖 Bible Reader"))
	s.WriteString("\n\n")

	// Breadcrumb
	if len(m.breadcrumb) > 0 {
		breadcrumbText := "📍 " + strings.Join(m.breadcrumb, " > ")
		// Wrap breadcrumb if too long
		if len(breadcrumbText) > m.width-4 {
			breadcrumbText = breadcrumbText[:m.width-7] + "..."
		}
		s.WriteString(breadcrumbStyle.Render(breadcrumbText))
		s.WriteString("\n\n")
	}

	// Content
	if m.content != "" {
		s.WriteString(m.viewport.View())
		s.WriteString("\n")
	}

	// Help text
	s.WriteString(helpStyle.Render("j/k: scroll • p/n: prev/next verse • delete: back • q/Ctrl+C: quit"))

	return s.String()
}

func (m model) handleBack() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	if len(m.breadcrumb) > 0 {
		m.breadcrumb = m.breadcrumb[:len(m.breadcrumb)-1]
	}

	switch m.state {
	case stateTestament:
		return m, tea.Quit
	case stateBook:
		m.state = stateTestament
		m.breadcrumb = []string{}
		m.list = createTestamentList()
		m.err = nil
	case stateChapter:
		m.state = stateBook
		m.list = createBookList(m.selectedTest)
		m.err = nil
	case stateVerse:
		m.state = stateChapter
		m.list = createChapterList(m.selectedBook)
		m.err = nil
	case stateReading:
		m.state = stateVerse
		m.list = createVerseList(m.selectedBook, m.selectedChap)
		m.err = nil
	}

	return m, nil
}

func (m model) navigateVerse(direction int) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	newVerse := m.selectedVerse + direction
	maxVerses := verseCount(m.selectedBook.Name, m.selectedChap)

	// Check bounds for current chapter
	if newVerse < 1 {
		// Try to go to previous chapter
		if m.selectedChap > 1 {
			m.selectedChap--
			maxVerses = verseCount(m.selectedBook.Name, m.selectedChap)
			m.selectedVerse = maxVerses
		} else {
			return m, nil
			/*
				// Try to go to previous book
				books := getBookList(m.selectedTest)
				currentBookIndex := findBookIndex(books, m.selectedBook.Name)
				if currentBookIndex > 0 {
					m.selectedBook = books[currentBookIndex-1]
					m.selectedChap = m.selectedBook.Chapters
					m.selectedVerse = verseCount(m.selectedBook.Name, m.selectedChap)
				} else {
					// At the beginning, can't go further back
					return m, nil
				}
			*/
		}
	} else if newVerse > maxVerses {
		// Try to go to next chapter
		if m.selectedChap < m.selectedBook.Chapters {
			m.selectedChap++
			m.selectedVerse = 1
		} else {
			return m, nil
			/*
				// Try to go to next book
				books := getBookList(m.selectedTest)
				currentBookIndex := findBookIndex(books, m.selectedBook.Name)
				if currentBookIndex < len(books)-1 {
					m.selectedBook = books[currentBookIndex+1]
					m.selectedChap = 1
					m.selectedVerse = 1
				} else {
					// At the end, can't go further forward
					return m, nil
				}
			*/
		}
	} else {
		m.selectedVerse = newVerse
	}

	// Update breadcrumb
	m.breadcrumb = []string{
		m.selectedTest.Name,
		m.selectedBook.Name,
		fmt.Sprintf("Chapter %d", m.selectedChap),
		fmt.Sprintf("Verse %d", m.selectedVerse),
	}

	// Fetch the new verse
	m.loading = true
	reference := fmt.Sprintf("%s %d:%d", m.selectedBook.Name, m.selectedChap, m.selectedVerse)
	return m, tea.Batch(
		m.spinner.Tick,
		fetchVerse(reference),
	)
}

/*
func getBookList(testament Testament) []Book {
	return testament.Books
}

func findBookIndex(books []Book, bookName string) int {
	for i, book := range books {
		if book.Name == bookName {
			return i
		}
	}
	return -1
}
*/

func (m model) handleSelect() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	selectedItem := m.list.SelectedItem()
	if selectedItem == nil {
		return m, nil
	}

	itemData, ok := selectedItem.(item)
	if !ok {
		m.err = fmt.Errorf("invalid item type")
		return m, nil
	}

	switch m.state {
	case stateTestament:
		testament, ok := itemData.value.(Testament)
		if !ok {
			m.err = fmt.Errorf("invalid testament data")
			return m, nil
		}
		m.selectedTest = testament
		m.state = stateBook
		m.breadcrumb = []string{testament.Name}
		m.list = createBookList(testament)

	case stateBook:
		book, ok := itemData.value.(Book)
		if !ok {
			m.err = fmt.Errorf("invalid book data")
			return m, nil
		}
		m.selectedBook = book
		m.state = stateChapter
		m.breadcrumb = append(m.breadcrumb, book.Name)
		m.list = createChapterList(book)

	case stateChapter:
		chapter, ok := itemData.value.(Chapter)
		if !ok {
			m.err = fmt.Errorf("invalid chapter data")
			return m, nil
		}
		m.selectedChap = chapter.Number
		m.state = stateVerse
		m.breadcrumb = append(m.breadcrumb, fmt.Sprintf("Chapter %d", chapter.Number))
		m.list = createVerseList(m.selectedBook, chapter.Number)

	case stateVerse:
		verse, ok := itemData.value.(Verse)
		if !ok {
			m.err = fmt.Errorf("invalid verse data")
			return m, nil
		}
		m.selectedVerse = verse.Number
		m.loading = true
		m.breadcrumb = append(m.breadcrumb, fmt.Sprintf("Verse %d", verse.Number))

		reference := fmt.Sprintf("%s %d:%d", m.selectedBook.Name, m.selectedChap, verse.Number)
		return m, tea.Batch(
			m.spinner.Tick,
			fetchVerse(reference),
		)
	}

	m.err = nil
	return m, nil
}

// List creation functions
func createTestamentList() list.Model {
	items := []list.Item{
		item{title: "Old Testament", description: "39 books from Genesis to Malachi", value: getOldTestament()},
		item{title: "New Testament", description: "27 books from Matthew to Revelation", value: getNewTestament()},
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Select Testament"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	return l
}

func createBookList(testament Testament) list.Model {
	items := make([]list.Item, len(testament.Books))
	for i, book := range testament.Books {
		desc := fmt.Sprintf("%d chapters", book.Chapters)
		items[i] = item{title: book.Name, description: desc, value: book}
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = fmt.Sprintf("Select Book from %s", testament.Name)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return l
}

func createChapterList(book Book) list.Model {
	items := make([]list.Item, book.Chapters)
	for i := 1; i <= book.Chapters; i++ {
		// Rough verse count estimation (this could be made more accurate with real data)
		verseCount := verseCount(book.Name, i)
		desc := fmt.Sprintf("~%d verses", verseCount)
		items[i-1] = item{
			title:       fmt.Sprintf("Chapter %d", i),
			description: desc,
			value:       Chapter{Number: i, Verses: verseCount},
		}
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = fmt.Sprintf("Select Chapter from %s", book.Name)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	return l
}

func createVerseList(book Book, chapter int) list.Model {
	verseCount := verseCount(book.Name, chapter)
	items := make([]list.Item, verseCount)
	for i := 1; i <= verseCount; i++ {
		items[i-1] = item{
			title:       fmt.Sprintf("Verse %d", i),
			description: fmt.Sprintf("%s %d:%d", book.Name, chapter, i),
			value:       Verse{Number: i},
		}
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = fmt.Sprintf("Select Verse from %s Chapter %d", book.Name, chapter)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	return l
}

// Bible data
func getOldTestament() Testament {
	return Testament{
		Name: "Old Testament",
		Books: []Book{
			{"Genesis", 50}, {"Exodus", 40}, {"Leviticus", 27}, {"Numbers", 36}, {"Deuteronomy", 34},
			{"Joshua", 24}, {"Judges", 21}, {"Ruth", 4}, {"1 Samuel", 31}, {"2 Samuel", 24},
			{"1 Kings", 22}, {"2 Kings", 25}, {"1 Chronicles", 29}, {"2 Chronicles", 36}, {"Ezra", 10},
			{"Nehemiah", 13}, {"Esther", 10}, {"Job", 42}, {"Psalms", 150}, {"Proverbs", 31},
			{"Ecclesiastes", 12}, {"Song of Solomon", 8}, {"Isaiah", 66}, {"Jeremiah", 52}, {"Lamentations", 5},
			{"Ezekiel", 48}, {"Daniel", 12}, {"Hosea", 14}, {"Joel", 3}, {"Amos", 9},
			{"Obadiah", 1}, {"Jonah", 4}, {"Micah", 7}, {"Nahum", 3}, {"Habakkuk", 3},
			{"Zephaniah", 3}, {"Haggai", 2}, {"Zechariah", 14}, {"Malachi", 4},
		},
	}
}

func getNewTestament() Testament {
	return Testament{
		Name: "New Testament",
		Books: []Book{
			{"Matthew", 28}, {"Mark", 16}, {"Luke", 24}, {"John", 21}, {"Acts", 28},
			{"Romans", 16}, {"1 Corinthians", 16}, {"2 Corinthians", 13}, {"Galatians", 6}, {"Ephesians", 6},
			{"Philippians", 4}, {"Colossians", 4}, {"1 Thessalonians", 5}, {"2 Thessalonians", 3}, {"1 Timothy", 6},
			{"2 Timothy", 4}, {"Titus", 3}, {"Philemon", 1}, {"Hebrews", 13}, {"James", 5},
			{"1 Peter", 5}, {"2 Peter", 3}, {"1 John", 5}, {"2 John", 1}, {"3 John", 1},
			{"Jude", 1}, {"Revelation", 22},
		},
	}
}

// Word wrapping function
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		// If adding this word would exceed the width, start a new line
		if currentLine.Len() > 0 && currentLine.Len()+1+len(word) > width {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		// Add word to current line
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	// Add the last line if it has content
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}

// API Functions
func fetchVerse(reference string) tea.Cmd {
	return func() tea.Msg {
		cleanRef := strings.TrimSpace(reference)
		encodedRef := url.QueryEscape(cleanRef)

		apiURL := fmt.Sprintf("https://bible-api.com/%s?translation=kjv", encodedRef)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(apiURL)
		if err != nil {
			return errMsg(fmt.Errorf("failed to fetch verse: %w", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errMsg(fmt.Errorf("API returned status %d", resp.StatusCode))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg(fmt.Errorf("failed to read response: %w", err))
		}

		var bibleResp BibleResponse
		if err := json.Unmarshal(body, &bibleResp); err != nil {
			return errMsg(fmt.Errorf("failed to parse response: %w", err))
		}

		if bibleResp.Reference == "" && bibleResp.Text == "" {
			return errMsg(fmt.Errorf("verse not found: %s", reference))
		}

		return verseMsg(bibleResp)
	}
}

func formatBibleResponse(resp BibleResponse, maxWidth int) string {
	var content strings.Builder

	// Calculate usable width for text (accounting for borders and padding)
	textWidth := maxWidth - 6 // 2 for border + 4 for padding
	if textWidth < 20 {
		textWidth = 40 // minimum readable width
	}

	if resp.Reference != "" {
		content.WriteString(referenceStyle.Render(resp.Reference))
		content.WriteString("\n\n")
	}

	if resp.TranslationName != "" {
		translationText := fmt.Sprintf("Translation: %s", resp.TranslationName)
		content.WriteString(wrapText(translationText, textWidth))
		content.WriteString("\n\n")
	}

	if len(resp.Verses) > 0 {
		for _, verse := range resp.Verses {
			verseText := fmt.Sprintf("%d %s", verse.Verse, verse.Text)
			wrappedText := wrapText(verseText, textWidth)

			// Create verse style with proper width
			styledVerse := verseStyle.Width(textWidth + 4).Render(wrappedText)
			content.WriteString(styledVerse)
			content.WriteString("\n\n")
		}
	} else if resp.Text != "" {
		wrappedText := wrapText(resp.Text, textWidth)
		styledVerse := verseStyle.Width(textWidth + 4).Render(wrappedText)
		content.WriteString(styledVerse)
		content.WriteString("\n\n")
	}

	if resp.TranslationNote != "" {
		noteText := "Note: " + resp.TranslationNote
		wrappedNote := wrapText(noteText, textWidth)
		content.WriteString(helpStyle.Render(wrappedNote))
		content.WriteString("\n")
	}

	return content.String()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
