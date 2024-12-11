package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	reviewedLines := []string{}  // To accumulate review entries

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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one .flsh file as argument")
			os.Exit(1)
	}

	var files []FlashFile
	for _, arg := range os.Args[1:] {
		if filepath.Ext(arg) != ".flsh" {
			continue
		}
			ff, err := parseFlashFile(arg)
			if err != nil {
				log.Printf("Error reading %s: %v\n", arg, err)
				continue
			}
			files = append(files, *ff)
	}

	if len(files) == 0 {
		fmt.Println("No valid .flsh files provided")
		os.Exit(1)
	}

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		log.Fatal(err)
	}
	defer screen.Fini()

	// Show file selection menu
	selectedFile := showFileSelection(screen, files)
	if selectedFile == nil {
		return
	}

	// Run through flashcards
	correct := 0
	total := len(selectedFile.Cards)
	
	for i := range selectedFile.Cards {
		showCard(screen, &selectedFile.Cards[i])
		if strings.HasSuffix(selectedFile.Cards[i].Reviewed, "Y") {
			correct++
		}
	}

	// Update stats with timestamp
	currentTime := time.Now().Format("2006/01/02 15:04")
	newScore := fmt.Sprintf("%s    %d/%d", currentTime, correct, total)
	if selectedFile.Stats != "" {
		selectedFile.Stats += "\n"
	}
	selectedFile.Stats += newScore
	
	// Display score comparison
	screen.Clear()
	drawText(screen, 0, 0, "Current score:")
	drawText(screen, 0, 1, newScore)
	drawText(screen, 0, 3, "Previous scores:")
	
	// Get previous scores and count lines
	prevScores := getPreviousScore(selectedFile)
	numPrevScoreLines := len(strings.Split(prevScores, "\n"))
	
	drawText(screen, 0, 4, prevScores)
	drawText(screen, 0, 6 + numPrevScoreLines, "Press any key to exit")
	screen.Show()
	
	// Wait for keypress before exiting
	for {
		ev := screen.PollEvent()
		switch ev.(type) {
		case *tcell.EventKey:
			err = saveFlashFile(selectedFile)
			if err != nil {
				log.Fatal(err)
			}
			return
		}
	}
}

func showFileSelection(screen tcell.Screen, files []FlashFile) *FlashFile {
	screen.Clear()
	
	for i, file := range files {
		drawText(screen, 0, i*2, fmt.Sprintf("%d. %s", i+1, file.Title))
	}
	drawText(screen, 0, len(files)*2+1, "Select a file (1-9) or press 'q' to quit:")
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

func showCard(screen tcell.Screen, card *Flashcard) {
	screen.Clear()
	
	// Show front
	drawText(screen, 0, 0, "Front:")
	drawText(screen, 0, 2, card.Front)
	drawText(screen, 0, 15, "Press SPACE to see back")
	screen.Show()

	// Wait for space
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' || ev.Key() == tcell.KeyEnter {
				goto showBack
			}
		}
	}

showBack:
	screen.Clear()
	drawText(screen, 0, 0, "Front:")
	drawText(screen, 0, 2, card.Front)
	drawText(screen, 0, 8, "Back:")
	drawText(screen, 0, 10, card.Back)
	drawText(screen, 0, 16, "Did you get it right? (y/n)")
	screen.Show()

	// Wait for y/n
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Rune() == 'y' || ev.Rune() == 'Y' {
				if card.Reviewed != "" {
					card.Reviewed += "\n"
				}
				card.Reviewed += time.Now().Format("2006/01/02") + " Y"
				return
			}
			if ev.Rune() == 'n' || ev.Rune() == 'N' {
				if card.Reviewed != "" {
					card.Reviewed += "\n"
				}
				card.Reviewed += time.Now().Format("2006/01/02") + " N"
				return
			}
		}
	}
}

func drawText(screen tcell.Screen, x, y int, text string) {
	width, _ := screen.Size()
	maxWidth := width - x  // Available width from x position to screen edge
	
	// Split text into lines first
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
			// Check if adding the next word (plus a space) would exceed the width
			if len(currentLine)+1+len(word) < maxWidth {
				currentLine += " " + word
			} else {
				// Draw current line and move to next line
				for i, r := range currentLine {
					screen.SetContent(x+i, currentY, r, nil, tcell.StyleDefault)
				}
				currentY++
				currentLine = word
			}
		}

		// Draw the last line
		for i, r := range currentLine {
			screen.SetContent(x+i, currentY, r, nil, tcell.StyleDefault)
		}
		currentY++
	}
} 