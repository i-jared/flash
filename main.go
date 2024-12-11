package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

type Flashcard struct {
	Front    string
	Back     string
	Reviewed string
}

type FlashFile struct {
	Title    string
	Stats    string
	Cards    []Flashcard
	Filename string
}

var (
	styleDefault = tcell.StyleDefault
	styleTitle   = tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true)
	stylePrompt  = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	styleScore   = tcell.StyleDefault.Foreground(tcell.NewRGBColor(0, 255, 255)) // Cyan
	styleCorrect = tcell.StyleDefault.Foreground(tcell.ColorGreen)
	styleWrong   = tcell.StyleDefault.Foreground(tcell.ColorRed)
)

func parseFlashFile(filename string) (*FlashFile, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var ff FlashFile
	ff.Filename = filename

	// Parse title (between ###)
	inTitle := false
	titleLines := []string{}
	for _, line := range lines {
		if line == "###" {
			if !inTitle {
				inTitle = true
				continue
			} else {
				break
			}
		}
		if inTitle {
			titleLines = append(titleLines, line)
		}
	}
	ff.Title = strings.Join(titleLines, "\n")

	// Parse stats (between &&&)
	inStats := false
	statsLines := []string{}
	for _, line := range lines {
		if line == "&&&" {
			if !inStats {
				inStats = true
				continue
			} else {
				break
			}
		}
		if inStats && line != "" {
			statsLines = append(statsLines, line)
		}
	}
	ff.Stats = strings.Join(statsLines, "\n")

	// Parse cards
	var currentCard Flashcard
	inCard := false
	section := ""
	reviewedLines := []string{} // To accumulate review entries

	for _, line := range lines {
		if line == "***" {
			if inCard {
				// Join all reviewed lines before adding the card
				if len(reviewedLines) > 0 {
					currentCard.Reviewed = strings.Join(reviewedLines, "\n")
				}
				ff.Cards = append(ff.Cards, currentCard)
				currentCard = Flashcard{}
				reviewedLines = []string{} // Reset for next card
			}
			inCard = !inCard
			continue
		}

		if inCard {
			switch {
			case line == "!FRONT":
				section = "front"
			case line == "!BACK":
				section = "back"
			case line == "!REVIEWED":
				section = "reviewed"
				reviewedLines = []string{} // Reset at start of reviewed section
			case line != "":
				switch section {
				case "front":
					currentCard.Front += line + "\n"
				case "back":
					currentCard.Back += line + "\n"
				case "reviewed":
					if line != "" {
						reviewedLines = append(reviewedLines, line)
					}
				}
			}
		}
	}

	return &ff, nil
}

func saveFlashFile(ff *FlashFile) error {
	var content strings.Builder

	// Write title
	content.WriteString("###\n")
	content.WriteString(ff.Title)
	content.WriteString("\n###\n")

	// Write stats
	content.WriteString("&&&\n")
	content.WriteString(ff.Stats)
	if !strings.HasSuffix(ff.Stats, "\n") && ff.Stats != "" {
		content.WriteString("\n")
	}
	content.WriteString("&&&\n")

	// Write cards
	content.WriteString("***\n") // Start with ***
	for i, card := range ff.Cards {
		content.WriteString("\n!FRONT\n\n")
		content.WriteString(strings.TrimSpace(card.Front))
		content.WriteString("\n\n!BACK\n\n")
		content.WriteString(strings.TrimSpace(card.Back))
		content.WriteString("\n\n!REVIEWED\n\n")
		content.WriteString(strings.TrimSpace(card.Reviewed))
		content.WriteString("\n\n***\n") // End each card with ***
		if i < len(ff.Cards)-1 {
			content.WriteString("***\n") // Start next card with another ***
		}
	}

	return os.WriteFile(ff.Filename, []byte(content.String()), 0644)
}

func getPreviousScore(ff *FlashFile) string {
	if ff.Stats == "" {
		return "No previous scores"
	}
	scores := strings.Split(ff.Stats, "\n")
	if len(scores) <= 1 {
		return "No previous scores"
	}

	// Sort scores in descending order by date
	// Each score is in format "2024/12/05 22:03    1/2"
	sort.Slice(scores, func(i, j int) bool {
		// Extract datetime from each score
		dateI := strings.Split(scores[i], "    ")[0]
		dateJ := strings.Split(scores[j], "    ")[0]
		// Compare dates in reverse order
		return dateI > dateJ
	})

	// Return all scores
	return strings.Join(scores, "\n")
}

// Add this helper function
func findSingleFlashFile() (string, error) {
	files, err := filepath.Glob("*.flsh")
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no .flsh files found in current directory")
	}
	if len(files) > 1 {
		return "", fmt.Errorf("multiple .flsh files found, please specify which one to use")
	}
	return files[0], nil
}

func main() {
	if len(os.Args) < 2 {
		// Show file selection menu
		files, err := filepath.Glob("*.flsh")
		if err != nil || len(files) == 0 {
			fmt.Println("Usage:")
			fmt.Println("  Review all cards: flash file.flsh")
			fmt.Println("  Review wrong cards: flash review file.flsh")
			fmt.Println("  Add card: flash add file.flsh")
			fmt.Println("  Create new file: flash new <name>")
			os.Exit(1)
		}

		// Initialize screen for file selection
		screen, err := tcell.NewScreen()
		if err != nil {
			log.Fatal(err)
		}
		if err := screen.Init(); err != nil {
			log.Fatal(err)
		}
		defer screen.Fini()

		// Load all flash files
		var flashFiles []FlashFile
		for _, f := range files {
			ff, err := parseFlashFile(f)
			if err != nil {
				log.Printf("Error reading %s: %v\n", f, err)
				continue
			}
			flashFiles = append(flashFiles, *ff)
		}

		if len(flashFiles) == 0 {
			fmt.Println("No valid .flsh files found")
			os.Exit(1)
		}

		selected := showFileSelection(screen, flashFiles)
		if selected == nil {
			return
		}
		// Instead of modifying os.Args, handle the selected file directly
		handleRegularReview(selected)
		return
	}

	// Check command type first
	switch os.Args[1] {
	case "new":
		if len(os.Args) != 3 {
			fmt.Println("Usage: flash new <name>")
			fmt.Println("Creates a new flashcard file (will add .flsh extension if not present)")
			os.Exit(1)
		}
		err := createNewFlashFile(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		return
	case "add":
		filename := ""
		if len(os.Args) > 2 {
			filename = os.Args[2]
		} else {
			var err error
			filename, err = findSingleFlashFile()
			if err != nil {
				fmt.Println("Usage: flash add file.flsh")
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
		}
		err := addFlashcard(filename)
		if err != nil {
			log.Fatal(err)
		}
		return
	case "review":
		filename := ""
		if len(os.Args) > 2 {
			filename = os.Args[2]
		} else {
			var err error
			filename, err = findSingleFlashFile()
			if err != nil {
				fmt.Println("Usage: flash review file.flsh")
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
		}
		err := reviewWrongCards(filename)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	// Handle regular review (no command)
	var filename string
	if filepath.Ext(os.Args[1]) == ".flsh" {
		filename = os.Args[1]
	} else {
		var err error
		filename, err = findSingleFlashFile()
		if err != nil {
			fmt.Println("Usage:")
			fmt.Println("  Review all cards: flash file.flsh")
			fmt.Println("  Review wrong cards: flash review file.flsh")
			fmt.Println("  Add card: flash add file.flsh")
			fmt.Println("  Create new file: flash new <name>")
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Handle direct file path or show file selection
	var selectedFile *FlashFile
	ff, err := parseFlashFile(filename)
	if err != nil {
		log.Printf("Error reading %s: %v\n", filename, err)
		os.Exit(1)
	}
	selectedFile = ff

	// Initialize screen for flashcard review
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		log.Fatal(err)
	}
	screen.Clear()
	defer screen.Fini()

	// Show title page
	if !showTitlePage(screen, selectedFile) {
		return // User quit
	}

	// Run through flashcards
	correct := 0
	total := 0

	for i := range selectedFile.Cards {
		if showCard(screen, &selectedFile.Cards[i]) {
			// User quit early
			break
		}
		total++
		if strings.HasSuffix(selectedFile.Cards[i].Reviewed, "Y") {
			correct++
		}
	}

	if total > 0 {
		// Update stats with timestamp
		currentTime := time.Now().Format("2006/01/02 15:04")
		newScore := fmt.Sprintf("%s    %d/%d", currentTime, correct, total)
		if selectedFile.Stats != "" {
			selectedFile.Stats += "\n"
		}
		selectedFile.Stats += newScore

		// Display score comparison in UI
		screen.Clear()
		drawText(screen, 0, 0, "Current score:", styleTitle)
		drawText(screen, 0, 1, newScore, styleScore)
		drawText(screen, 0, 3, "Previous scores:", styleTitle)

		// Get previous scores and count lines
		prevScores := getPreviousScore(selectedFile)
		scoreLines := strings.Split(prevScores, "\n")
		numPrevScoreLines := len(scoreLines)

		// Draw scores and graph side by side
		drawText(screen, 0, 4, prevScores, styleScore)
		drawScoreGraph(screen, 40, 4, scoreLines, 30, 10)

		drawText(screen, 0, 6+numPrevScoreLines, "Press any key to exit", stylePrompt)
		screen.Show()

		// Wait for keypress and save
		for {
			ev := screen.PollEvent()
			switch ev.(type) {
			case *tcell.EventKey:
				err = saveFlashFile(selectedFile)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("%d/%d\n", correct, total)
				return
			}
		}
	}
}

func showTitlePage(screen tcell.Screen, ff *FlashFile) bool {
	screen.Clear()

	// Draw title
	titleLines := strings.Split(ff.Title, "\n")
	for i, line := range titleLines {
		drawText(screen, 0, i, line, styleTitle)
	}

	drawText(screen, 0, len(titleLines)+2, "Press ENTER to continue, q to quit", stylePrompt)
	screen.Show()

	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 'q' {
				return false
			}
			if ev.Key() == tcell.KeyEnter {
				return true
			}
		}
	}
}

func showFileSelection(screen tcell.Screen, files []FlashFile) *FlashFile {
	screen.Clear()

	// Calculate the width of the number prefix (e.g., "1. ")
	prefixWidth := 3 // Width of "X. " where X is the number
	currentY := 0

	for i, file := range files {
		// Draw the file number
		drawText(screen, 0, currentY, fmt.Sprintf("%d.", i+1), styleTitle)

		// Split title into lines and draw each line with proper indentation
		titleLines := strings.Split(file.Title, "\n")
		for j, line := range titleLines {
			if j == 0 {
				// First line comes after the number
				drawText(screen, prefixWidth, currentY, line, styleTitle)
			} else {
				// Subsequent lines are indented to match
				drawText(screen, prefixWidth, currentY+j, line, styleTitle)
			}
		}
		currentY += len(titleLines) + 1 // Add space between files
	}

	drawText(screen, prefixWidth, currentY, "Select a file (1-9) or press 'q' to quit:", stylePrompt)
	screen.Show()

	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Rune() == 'q' {
				return nil
			}
			if ev.Rune() >= '1' && ev.Rune() <= '9' {
				idx := int(ev.Rune() - '1')
				if idx < len(files) {
					return &files[idx]
				}
			}
		}
	}
}

func showCard(screen tcell.Screen, card *Flashcard) bool {
	screen.Clear()

	// Show front
	drawText(screen, 0, 0, "Front:", styleTitle)
	drawText(screen, 0, 2, card.Front, styleDefault)
	drawText(screen, 0, 15, "Press SPACE to see back, q to quit", stylePrompt)
	screen.Show()

	// Wait for space
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 'q' {
				return true
			}
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' || ev.Key() == tcell.KeyEnter {
				goto showBack
			}
		}
	}

showBack:
	screen.Clear()
	drawText(screen, 0, 0, "Front:", styleTitle)
	drawText(screen, 0, 2, card.Front, styleDefault)
	drawText(screen, 0, 8, "Back:", styleTitle)
	drawText(screen, 0, 10, card.Back, styleDefault)
	drawText(screen, 0, 16, "Did you get it right? (y/n) (q to quit)", stylePrompt)
	screen.Show()

	// Wait for y/n
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 'q' {
				return true
			}
			if ev.Rune() == 'y' || ev.Rune() == 'Y' {
				if card.Reviewed != "" {
					card.Reviewed += "\n"
				}
				card.Reviewed += time.Now().Format("2006/01/02") + " Y"
				return false
			}
			if ev.Rune() == 'n' || ev.Rune() == 'N' {
				if card.Reviewed != "" {
					card.Reviewed += "\n"
				}
				card.Reviewed += time.Now().Format("2006/01/02") + " N"
				return false
			}
		}
	}
}

func drawText(screen tcell.Screen, x, y int, text string, style tcell.Style) {
	width, _ := screen.Size()
	maxWidth := width - x

	lines := strings.Split(text, "\n")
	currentY := y

	for _, line := range lines {
		if line == "" {
			currentY++
			continue
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			currentY++
			continue
		}

		currentLine := words[0]

		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) < maxWidth {
				currentLine += " " + word
			} else {
				for i, r := range currentLine {
					screen.SetContent(x+i, currentY, r, nil, style)
				}
				currentY++
				currentLine = word
			}
		}

		for i, r := range currentLine {
			screen.SetContent(x+i, currentY, r, nil, style)
		}
		currentY++
	}
}

// Add this new function to draw the graph
func drawScoreGraph(screen tcell.Screen, x, y int, scores []string, width, height int) {
	if len(scores) < 2 {
		return
	}

	// Parse scores into numbers (in reverse order to show oldest to newest)
	var numbers []float64
	for i := len(scores) - 1; i >= 0; i-- { // Changed this line to reverse the order
		score := scores[i]
		parts := strings.Split(score, "    ")
		if len(parts) != 2 {
			continue
		}
		scoreParts := strings.Split(parts[1], "/")
		if len(scoreParts) != 2 {
			continue
		}
		num, err := strconv.ParseFloat(scoreParts[0], 64)
		if err != nil {
			continue
		}
		den, err := strconv.ParseFloat(scoreParts[1], 64)
		if err != nil {
			continue
		}
		numbers = append(numbers, num/den*100) // Convert to percentage
	}

	// Find min and max
	min, max := numbers[0], numbers[0]
	for _, n := range numbers {
		if n < min {
			min = n
		}
		if n > max {
			max = n
		}
	}
	if min == max {
		max = min + 1 // Avoid division by zero
	}

	// Draw axes
	for i := 0; i < height; i++ {
		screen.SetContent(x, y+i, '│', nil, styleScore)
	}
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y+height-1, '─', nil, styleScore)
	}
	screen.SetContent(x, y+height-1, '└', nil, styleScore)

	// Draw points and lines
	lastX, lastY := -1, -1
	for i, score := range numbers {
		// Calculate position
		px := x + 1 + (i * (width - 2) / (len(numbers) - 1))
		py := y + height - 2 - int((score-min)/(max-min)*float64(height-3))

		// Draw point
		screen.SetContent(px, py, '●', nil, styleScore)

		// Draw line from last point
		if lastX != -1 {
			drawLine(screen, lastX, lastY, px, py, styleScore)
		}
		lastX, lastY = px, py
	}

	// Draw scale
	maxStr := fmt.Sprintf("%.0f%%", max)
	minStr := fmt.Sprintf("%.0f%%", min)
	drawText(screen, x-len(maxStr)-1, y, maxStr, styleScore)
	drawText(screen, x-len(minStr)-1, y+height-2, minStr, styleScore)
}

// Add this helper function to draw lines
func drawLine(screen tcell.Screen, x1, y1, x2, y2 int, style tcell.Style) {
	// Simple line drawing algorithm
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	steep := dy > dx

	if steep {
		x1, y1 = y1, x1
		x2, y2 = y2, x2
	}
	if x1 > x2 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}

	dx = x2 - x1
	dy = abs(y2 - y1)
	err := dx / 2
	ystep := 1
	if y1 >= y2 {
		ystep = -1
	}

	for ; x1 <= x2; x1++ {
		var x, y int
		if steep {
			x, y = y1, x1
		} else {
			x, y = x1, y1
		}
		screen.SetContent(x, y, '·', nil, style)
		err -= dy
		if err < 0 {
			y1 += ystep
			err += dx
		}
	}
}

// Helper function for absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func getMultilineInput(screen tcell.Screen, startY int, prompt string, bottomPrompt string) string {
	var lines []string
	currentLine := ""
	y := startY
	_, height := screen.Size()

	// Initial draw
	screen.Clear()
	drawText(screen, 0, 0, prompt, stylePrompt)
	drawText(screen, 0, height-1, bottomPrompt, stylePrompt)

	for {
		// Clear input area and redraw all lines
		for i := startY; i < height-1; i++ {
			for j := 0; j < 80; j++ {
				screen.SetContent(j, i, ' ', nil, styleDefault)
			}
		}

		// Draw previous lines
		for i, line := range lines {
			for j, r := range line {
				screen.SetContent(j, startY+i, r, nil, styleDefault)
			}
		}

		// Draw current line
		for i, r := range currentLine {
			screen.SetContent(i, y, r, nil, styleDefault)
		}

		screen.Show()

		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape:
				return ""
			case tcell.KeyEnter:
				if len(lines) > 0 || len(currentLine) > 0 {
					if len(currentLine) > 0 {
						lines = append(lines, currentLine)
					}
					return strings.Join(lines, "\n")
				}
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(currentLine) > 0 {
					currentLine = currentLine[:len(currentLine)-1]
				}
			default:
				if ev.Rune() != 0 {
					currentLine += string(ev.Rune())
				}
			}
		}
	}
}

func addFlashcard(filename string) error {
	// Read existing file or create new one
	var ff *FlashFile
	var err error

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// Create new file if it doesn't exist
		ff = &FlashFile{
			Filename: filename,
			Title:    filepath.Base(filename),
		}
	} else {
		ff, err = parseFlashFile(filename)
		if err != nil {
			return fmt.Errorf("error reading file: %v", err)
		}
	}

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	// Get front of card
	front := getMultilineInput(screen, 2,
		"please write card front:",
		"press Enter to continue")
	if front == "" {
		return nil // User cancelled
	}

	// Get back of card
	back := getMultilineInput(screen, 2,
		"please write card back:",
		"press Enter to save")
	if back == "" {
		return nil // User cancelled
	}

	// Add the new card
	ff.Cards = append(ff.Cards, Flashcard{
		Front: front,
		Back:  back,
	})

	// Save the file
	return saveFlashFile(ff)
}

func reviewWrongCards(filename string) error {
	// Read the file
	ff, err := parseFlashFile(filename)
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}

	// Find cards that were wrong in their last review
	var wrongCards []int // Store indices of wrong cards
	for i, card := range ff.Cards {
		reviews := strings.Split(card.Reviewed, "\n")
		if len(reviews) > 0 {
			lastReview := reviews[len(reviews)-1]
			if strings.HasSuffix(lastReview, "N") {
				wrongCards = append(wrongCards, i)
			}
		}
	}

	if len(wrongCards) == 0 {
		fmt.Println("No cards to review - all cards were correct in last review!")
		return nil
	}

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	// Track score for wrong cards review
	reviewed := 0
	correct := 0

	// Show and review wrong cards
	for _, idx := range wrongCards {
		if showCard(screen, &ff.Cards[idx]) {
			// User quit early
			break
		}
		reviewed++
		if strings.HasSuffix(ff.Cards[idx].Reviewed, "Y") {
			correct++
		}
	}

	// Save file (only card review history is updated, not the stats)
	err = saveFlashFile(ff)
	if err != nil {
		return err
	}

	screen.Fini() // Properly close the screen
	// Print score to terminal before exiting
	if reviewed > 0 {
		fmt.Printf("%d/%d\n", correct, reviewed)
	}
	return nil
}

// Add this new function
func createNewFlashFile(name string) error {
	// Add .flsh extension if not present
	if !strings.HasSuffix(name, ".flsh") {
		name += ".flsh"
	}

	// Check if file already exists
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("file %s already exists", name)
	}

	// Create new FlashFile
	ff := &FlashFile{
		Filename: name,
		Title:    strings.TrimSuffix(filepath.Base(name), ".flsh"), // Use filename without extension as title
	}

	// Save the empty file
	return saveFlashFile(ff)
}

// Add this new function to handle regular review
func handleRegularReview(selectedFile *FlashFile) {
	// Initialize screen for flashcard review
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		log.Fatal(err)
	}
	screen.Clear()
	defer screen.Fini()

	// Show title page
	if !showTitlePage(screen, selectedFile) {
		return // User quit
	}

	// Run through flashcards
	correct := 0
	total := 0

	for i := range selectedFile.Cards {
		if showCard(screen, &selectedFile.Cards[i]) {
			// User quit early
			break
		}
		total++
		if strings.HasSuffix(selectedFile.Cards[i].Reviewed, "Y") {
			correct++
		}
	}

	if total > 0 {
		// Update stats with timestamp
		currentTime := time.Now().Format("2006/01/02 15:04")
		newScore := fmt.Sprintf("%s    %d/%d", currentTime, correct, total)
		if selectedFile.Stats != "" {
			selectedFile.Stats += "\n"
		}
		selectedFile.Stats += newScore

		// Display score comparison in UI
		screen.Clear()
		drawText(screen, 0, 0, "Current score:", styleTitle)
		drawText(screen, 0, 1, newScore, styleScore)
		drawText(screen, 0, 3, "Previous scores:", styleTitle)

		// Get previous scores and count lines
		prevScores := getPreviousScore(selectedFile)
		scoreLines := strings.Split(prevScores, "\n")
		numPrevScoreLines := len(scoreLines)

		// Draw scores and graph side by side
		drawText(screen, 0, 4, prevScores, styleScore)
		drawScoreGraph(screen, 40, 4, scoreLines, 30, 10)

		drawText(screen, 0, 6+numPrevScoreLines, "Press any key to exit", stylePrompt)
		screen.Show()

		// Wait for keypress and save
		for {
			ev := screen.PollEvent()
			switch ev.(type) {
			case *tcell.EventKey:
				err = saveFlashFile(selectedFile)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("%d/%d\n", correct, total)
				return
			}
		}
	}
}
