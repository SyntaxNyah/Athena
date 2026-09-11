/* Athena - A server for Attorney Online 2 written in Go
Copyright (C) 2022 MangosArentLiterature <mango@transmenace.dev>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>. */

package athena

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	hangmanOptInDuration  = 30 * time.Second // window to type /hangman join
	hangmanCooldown       = 3 * time.Minute  // area cooldown between games
	hangmanMaxWrong       = 6                // wrong guesses allowed before game over
	hangmanMinPlayers     = 1                // minimum opt-in players (host counts)
	hangmanPunishDuration = 10 * time.Minute // length of punishment for wrong-guessers
)

// hangmanRules is the welcome text broadcast when a game opens.
const hangmanRules = `🎲 HANGMAN STARTING! 🎲
Type /hangman join within 30 seconds to participate.

📋 HOW TO PLAY:
• A secret word is chosen — dashes show unguessed letters:  _ _ _ _ _
• Type /hangman guess <letter>    to guess a single letter.
• Type /hangman guess <word>      to guess the whole word.
• You have %d wrong guesses before the hangman is complete.

✅ SUCCESS  — word solved:  everyone is safe!
❌ FAILURE  — max wrong guesses reached:  players who made wrong guesses
              receive a random punishment!

• Type /hangman status at any time to see the current board.
• Themes available: animals | courtroom | nature | food | random
• Hosts can also choose a custom word:  /hangman start custom <word>`

// hangmanArt holds the ASCII gallows stages indexed by wrong-guess count (0–6).
var hangmanArt = [7]string{
	" ___\n|   |\n|\n|\n|\n|___",
	" ___\n|   |\n|   O\n|\n|\n|___",
	" ___\n|   |\n|   O\n|   |\n|\n|___",
	" ___\n|   |\n|   O\n|  /|\n|\n|___",
	" ___\n|   |\n|   O\n|  /|\\\n|\n|___",
	" ___\n|   |\n|   O\n|  /|\\\n|  /\n|___",
	" ___\n|   |\n|   O\n|  /|\\\n|  / \\\n|___",
}

// ── Word pools ────────────────────────────────────────────────────────────────

var hangmanWordsAnimals = []string{
	"elephant", "penguin", "dolphin", "panther", "leopard",
	"crocodile", "flamingo", "vulture", "antelope", "gorilla",
	"cheetah", "lobster", "sparrow", "hamster", "porcupine",
	"platypus", "salamander", "chameleon", "scorpion", "falcon",
	"giraffe", "rhinoceros", "chimpanzee", "octopus", "jellyfish",
	"seahorse", "kangaroo", "koala", "wombat", "narwhal",
	"axolotl", "capybara", "armadillo", "wolverine", "mandrill",
	"echidna", "tapir", "pangolin", "okapi", "quetzal",
}

var hangmanWordsCourtroom = []string{
	"attorney", "witness", "verdict", "courtroom", "justice",
	"evidence", "objection", "testimony", "suspect", "alibi",
	"motive", "defense", "prosecution", "argument", "statement",
	"penalty", "innocent", "guilty", "hearing", "ruling",
	"motion", "appeal", "statute", "warrant", "custody",
	"bailiff", "counsel", "exonerate", "felony", "subpoena",
	"plaintiff", "rebuttal", "tribunal", "deponent", "perjury",
	"acquittal", "indictment", "jurisprudence", "magistrate", "summons",
}

var hangmanWordsNature = []string{
	"mountain", "volcano", "glacier", "canyon", "prairie",
	"tornado", "thunder", "lightning", "rainbow", "horizon",
	"waterfall", "cavern", "forest", "desert", "island",
	"ocean", "meadow", "tundra", "swamp", "avalanche",
	"earthquake", "blizzard", "monsoon", "tsunami", "typhoon",
	"stalactite", "peninsula", "archipelago", "fjord", "savanna",
	"crevasse", "estuary", "mangrove", "wetland", "tundra",
}

var hangmanWordsFood = []string{
	"spaghetti", "chocolate", "avocado", "broccoli", "cinnamon",
	"pineapple", "blueberry", "strawberry", "raspberry", "cantaloupe",
	"asparagus", "artichoke", "mushroom", "eggplant", "cucumber",
	"jalapeno", "croissant", "quesadilla", "bruschetta", "edamame",
	"guacamole", "mozzarella", "parmesan", "tiramisu", "macaroon",
	"enchilada", "hummus", "kimchi", "lasagna", "risotto",
	"prosciutto", "gnocchi", "tapioca", "sourdough", "churros",
}

// hangmanWordsAll is the combined pool used by the "random" theme.
// Populated in init() to avoid repeating the slices.
var hangmanWordsAll []string

func init() {
	all := make([]string, 0,
		len(hangmanWordsAnimals)+len(hangmanWordsCourtroom)+
			len(hangmanWordsNature)+len(hangmanWordsFood))
	all = append(all, hangmanWordsAnimals...)
	all = append(all, hangmanWordsCourtroom...)
	all = append(all, hangmanWordsNature...)
	all = append(all, hangmanWordsFood...)
	hangmanWordsAll = all
}

// ── State ─────────────────────────────────────────────────────────────────────

// hangmanState is the per-area mutable game state.
type hangmanState struct {
	mu             sync.Mutex
	optInActive    bool
	gameActive     bool
	word           string           // lowercase secret word
	revealed       []bool           // revealed[i] = true when word[i] is known
	wrongLetters   []rune           // wrong single-letter guesses (for display)
	wrongWordCount int              // wrong full-word guesses (each costs 1 strike)
	guessedLetters map[rune]bool    // all tried single letters
	wrongGuessers  map[int]bool     // UID → made at least one wrong guess
	participants   map[int]struct{} // opted-in UIDs
	hostUID        int              // UID of the player who started the game
	lastEnd        time.Time
	area           *area.Area
}

var (
	hangmanAreas   = map[*area.Area]*hangmanState{}
	hangmanAreasMu sync.Mutex
)

// hangmanGetState returns (and lazily creates) the per-area game state.
func hangmanGetState(a *area.Area) *hangmanState {
	hangmanAreasMu.Lock()
	defer hangmanAreasMu.Unlock()
	st, ok := hangmanAreas[a]
	if !ok {
		st = &hangmanState{area: a}
		hangmanAreas[a] = st
	}
	return st
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// hangmanDisplayWord builds the "_ a _ _ m a n" style board string.
func hangmanDisplayWord(word string, revealed []bool) string {
	var b strings.Builder
	ri := 0
	for _, ch := range word {
		if ri > 0 {
			b.WriteByte(' ')
		}
		if revealed[ri] {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
		ri++
	}
	return b.String()
}

// hangmanAllRevealed reports whether every letter has been guessed.
func hangmanAllRevealed(revealed []bool) bool {
	for _, r := range revealed {
		if !r {
			return false
		}
	}
	return true
}

// hangmanWrongStr formats the list of wrong single-letter guesses plus an
// indicator for any wrong full-word attempts.
func hangmanWrongStr(letters []rune, wordCount int) string {
	if len(letters) == 0 && wordCount == 0 {
		return "(none)"
	}
	// Pre-size: each letter is ≈3 bytes ("X, "); each word-strike is ≈6 bytes ("★, ").
	var b strings.Builder
	b.Grow(len(letters)*3 + wordCount*6)
	for _, r := range letters {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	for i := 0; i < wordCount; i++ {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteString("★")
	}
	return b.String()
}

// hangmanTotalWrong returns the current total wrong-guess count.
func hangmanTotalWrong(st *hangmanState) int {
	return len(st.wrongLetters) + st.wrongWordCount
}

// hangmanBoardMessage builds the full status string shown after each guess.
func hangmanBoardMessage(word string, revealed []bool, wrongLetters []rune, wrongWordCount int) string {
	total := len(wrongLetters) + wrongWordCount
	if total > hangmanMaxWrong {
		total = hangmanMaxWrong
	}
	return fmt.Sprintf("%s\nWord: %s\nWrong guesses (%d/%d): %s",
		hangmanArt[total],
		hangmanDisplayWord(word, revealed),
		total, hangmanMaxWrong,
		hangmanWrongStr(wrongLetters, wrongWordCount),
	)
}

// pickHangmanWord selects a random word from the given theme pool.
func pickHangmanWord(theme string) string {
	var pool []string
	switch theme {
	case "animals":
		pool = hangmanWordsAnimals
	case "courtroom":
		pool = hangmanWordsCourtroom
	case "nature":
		pool = hangmanWordsNature
	case "food":
		pool = hangmanWordsFood
	default:
		pool = hangmanWordsAll
	}
	return pool[rand.Intn(len(pool))]
}

// ── Command entry point ───────────────────────────────────────────────────────

// cmdHangman is the entry point for the /hangman command.
func cmdHangman(client *Client, args []string, usage string) {
	if len(args) == 0 {
		client.SendServerMessage(usage)
		return
	}
	switch strings.ToLower(args[0]) {
	case "start":
		hangmanStart(client, args[1:])
	case "join":
		hangmanJoin(client)
	case "guess":
		if len(args) < 2 {
			client.SendServerMessage("Usage: /hangman guess <letter|word>")
			return
		}
		hangmanGuess(client, strings.Join(args[1:], ""))
	case "status":
		hangmanStatus(client)
	case "stop":
		hangmanStop(client)
	default:
		client.SendServerMessage(usage)
	}
}

// ── Start ─────────────────────────────────────────────────────────────────────

// hangmanStart opens the opt-in window for a new Hangman game.
// Supported args:
//
//	/hangman start                        → random theme
//	/hangman start animals|courtroom|…    → specific theme
//	/hangman start custom <word>          → host supplies the word
func hangmanStart(client *Client, args []string) {
	st := hangmanGetState(client.Area())
	st.mu.Lock()

	if st.optInActive || st.gameActive {
		st.mu.Unlock()
		client.SendServerMessage("A Hangman game is already in progress in this area.")
		return
	}
	if !st.lastEnd.IsZero() {
		if rem := hangmanCooldown - time.Since(st.lastEnd); rem > 0 {
			st.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf(
				"Hangman is on cooldown in this area. Please wait %d seconds.",
				int((rem+time.Second-1)/time.Second),
			))
			return
		}
	}

	// Determine word and theme label.
	theme := "random"
	var word string
	if len(args) >= 1 {
		switch strings.ToLower(args[0]) {
		case "custom":
			if len(args) < 2 {
				st.mu.Unlock()
				client.SendServerMessage("Usage: /hangman start custom <word>  (letters only, 3–30 chars)")
				return
			}
			raw := strings.ToLower(strings.Join(args[1:], ""))
			for _, ch := range raw {
				if !unicode.IsLetter(ch) {
					st.mu.Unlock()
					client.SendServerMessage("Custom word must contain letters only (no spaces or numbers).")
					return
				}
			}
			if len([]rune(raw)) < 3 || len([]rune(raw)) > 30 {
				st.mu.Unlock()
				client.SendServerMessage("Custom word must be 3–30 letters long.")
				return
			}
			word = raw
			theme = "custom"
		case "animals", "courtroom", "nature", "food", "random":
			theme = strings.ToLower(args[0])
		default:
			st.mu.Unlock()
			client.SendServerMessage(
				"Unknown theme. Choices: animals | courtroom | nature | food | random | custom <word>",
			)
			return
		}
	}
	if word == "" {
		word = pickHangmanWord(theme)
	}

	wordLen := utf8.RuneCountInString(word)
	st.optInActive = true
	st.gameActive = false
	st.word = word
	st.revealed = make([]bool, wordLen)
	st.wrongLetters = nil
	st.wrongWordCount = 0
	st.guessedLetters = make(map[rune]bool)
	st.wrongGuessers = make(map[int]bool)
	st.participants = make(map[int]struct{})
	st.hostUID = client.Uid()
	st.mu.Unlock()

	themeDisplay := theme
	if theme == "custom" {
		themeDisplay = "custom word (host knows — kept secret!)"
	}

	sendAreaServerMessage(client.Area(), fmt.Sprintf(
		hangmanRules+"\n\nTheme: %s | Word length: %d letters",
		hangmanMaxWrong, themeDisplay, wordLen,
	))
	addToBuffer(client, "CMD",
		fmt.Sprintf("Started Hangman (theme=%s, len=%d)", theme, wordLen), false)

	// Auto-enrol the host.
	hangmanJoin(client)

	go hangmanOptInTimer(st)
}

// ── Join ──────────────────────────────────────────────────────────────────────

// hangmanJoin opts a player into the active join window.
func hangmanJoin(client *Client) {
	uid := client.Uid()
	st := hangmanGetState(client.Area())
	st.mu.Lock()

	if !st.optInActive {
		st.mu.Unlock()
		client.SendServerMessage("There is no Hangman game open to join in this area right now.")
		return
	}
	if _, already := st.participants[uid]; already {
		st.mu.Unlock()
		client.SendServerMessage("You have already joined the Hangman game.")
		return
	}
	st.participants[uid] = struct{}{}
	count := len(st.participants)
	st.mu.Unlock()

	client.SendServerMessage(fmt.Sprintf("🔤 You joined the Hangman game! (%d participant(s) so far)", count))
	sendAreaServerMessage(client.Area(),
		fmt.Sprintf("🔤 %v joined Hangman! (%d participant(s))", client.OOCName(), count))
}

// ── Status ────────────────────────────────────────────────────────────────────

// hangmanStatus shows the current board to the requesting player.
func hangmanStatus(client *Client) {
	st := hangmanGetState(client.Area())
	st.mu.Lock()

	if !st.gameActive && !st.optInActive {
		st.mu.Unlock()
		client.SendServerMessage("No Hangman game is active in this area.")
		return
	}
	if st.optInActive && !st.gameActive {
		count := len(st.participants)
		st.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf(
			"🔤 Hangman: join window open — %d player(s) joined so far. Type /hangman join!", count,
		))
		return
	}

	word := st.word
	revealed := make([]bool, len(st.revealed))
	copy(revealed, st.revealed)
	wrongLetters := make([]rune, len(st.wrongLetters))
	copy(wrongLetters, st.wrongLetters)
	wrongWordCount := st.wrongWordCount
	st.mu.Unlock()

	client.SendServerMessage("🔤 HANGMAN STATUS\n" +
		hangmanBoardMessage(word, revealed, wrongLetters, wrongWordCount))
}

// ── Guess ─────────────────────────────────────────────────────────────────────

// hangmanGuess processes a player's letter or word guess.
func hangmanGuess(client *Client, raw string) {
	uid := client.Uid()
	st := hangmanGetState(client.Area())
	st.mu.Lock()

	if !st.gameActive {
		st.mu.Unlock()
		client.SendServerMessage("No active Hangman game in this area. Use /hangman status to check.")
		return
	}
	if _, ok := st.participants[uid]; !ok {
		st.mu.Unlock()
		client.SendServerMessage(
			"You are not participating in this Hangman game. Watch with /hangman status.",
		)
		return
	}

	guess := strings.ToLower(strings.TrimSpace(raw))
	if guess == "" {
		st.mu.Unlock()
		client.SendServerMessage("Please provide a letter or word to guess.")
		return
	}
	// Validate letters-only input.
	for _, ch := range guess {
		if !unicode.IsLetter(ch) {
			st.mu.Unlock()
			client.SendServerMessage("Guesses must contain letters only.")
			return
		}
	}

	word := st.word

	// ── Full-word guess ───────────────────────────────────────────────────────
	if utf8.RuneCountInString(guess) > 1 {
		if guess == word {
			// Correct — reveal everything and end the game.
			for i := range st.revealed {
				st.revealed[i] = true
			}
			st.gameActive = false
			st.optInActive = false
			st.lastEnd = time.Now()
			st.mu.Unlock()

			sendAreaServerMessage(client.Area(), fmt.Sprintf(
				"🎉 %v guessed the whole word! The word was: %s\n🏆 HANGMAN SOLVED! Everyone is safe!",
				client.OOCName(), strings.ToUpper(word),
			))
			addToBuffer(client, "HANGMAN",
				fmt.Sprintf("Solved hangman by full-word guess: '%s'", word), false)
			return
		}

		// Wrong full-word guess — costs one strike.
		st.wrongWordCount++
		st.wrongGuessers[uid] = true
		total := hangmanTotalWrong(st)
		board := hangmanBoardMessage(word, st.revealed, st.wrongLetters, st.wrongWordCount)

		if total >= hangmanMaxWrong {
			// Collect state for post-lock punishment.
			wrongGuessers := make(map[int]bool, len(st.wrongGuessers))
			for k, v := range st.wrongGuessers {
				wrongGuessers[k] = v
			}
			hangArea := st.area
			st.gameActive = false
			st.optInActive = false
			st.lastEnd = time.Now()
			st.mu.Unlock()

			sendAreaServerMessage(hangArea, fmt.Sprintf(
				"❌ %v guessed '%s' — wrong!\n%s\n💀 GAME OVER! The word was: %s",
				client.OOCName(), strings.ToUpper(guess), board, strings.ToUpper(word),
			))
			hangmanPunishLosers(hangArea, wrongGuessers)
			return
		}
		st.mu.Unlock()

		sendAreaServerMessage(client.Area(), fmt.Sprintf(
			"❌ %v guessed '%s' — not the word!\n%s",
			client.OOCName(), strings.ToUpper(guess), board,
		))
		return
	}

	// ── Single-letter guess ───────────────────────────────────────────────────
	letter, _ := utf8.DecodeRuneInString(guess)
	if st.guessedLetters[letter] {
		st.mu.Unlock()
		client.SendServerMessage(
			fmt.Sprintf("Letter '%s' has already been guessed.", strings.ToUpper(string(letter))),
		)
		return
	}
	st.guessedLetters[letter] = true

	// Check whether the letter appears in the word.
	found := false
	ri := 0
	for _, ch := range word {
		if ch == letter {
			st.revealed[ri] = true
			found = true
		}
		ri++
	}
	if !found {
		st.wrongLetters = append(st.wrongLetters, letter)
		st.wrongGuessers[uid] = true
	}

	total := hangmanTotalWrong(st)
	allDone := hangmanAllRevealed(st.revealed)
	board := hangmanBoardMessage(word, st.revealed, st.wrongLetters, st.wrongWordCount)

	if allDone {
		// Win — last letter fills the word.
		st.gameActive = false
		st.optInActive = false
		st.lastEnd = time.Now()
		st.mu.Unlock()

		sendAreaServerMessage(client.Area(), fmt.Sprintf(
			"✅ %v guessed '%s' — correct!\n%s\n🏆 HANGMAN SOLVED! Everyone is safe!",
			client.OOCName(), strings.ToUpper(string(letter)), board,
		))
		addToBuffer(client, "HANGMAN",
			fmt.Sprintf("Final letter '%s' solved word '%s'", string(letter), word), false)
		return
	}

	if total >= hangmanMaxWrong {
		// Loss — max wrong guesses reached.
		wrongGuessers := make(map[int]bool, len(st.wrongGuessers))
		for k, v := range st.wrongGuessers {
			wrongGuessers[k] = v
		}
		hangArea := st.area
		st.gameActive = false
		st.optInActive = false
		st.lastEnd = time.Now()
		st.mu.Unlock()

		sendAreaServerMessage(hangArea, fmt.Sprintf(
			"❌ %v guessed '%s' — wrong!\n%s\n💀 GAME OVER! The word was: %s",
			client.OOCName(), strings.ToUpper(string(letter)), board, strings.ToUpper(word),
		))
		hangmanPunishLosers(hangArea, wrongGuessers)
		return
	}

	// Game continues.
	st.mu.Unlock()
	if found {
		sendAreaServerMessage(client.Area(), fmt.Sprintf(
			"✅ %v guessed '%s' — it's in the word!\n%s",
			client.OOCName(), strings.ToUpper(string(letter)), board,
		))
	} else {
		sendAreaServerMessage(client.Area(), fmt.Sprintf(
			"❌ %v guessed '%s' — not in the word!\n%s",
			client.OOCName(), strings.ToUpper(string(letter)), board,
		))
	}
}

// ── Stop ──────────────────────────────────────────────────────────────────────

// hangmanStop forcibly ends a game.  Only the host, CMs, and moderators may stop.
func hangmanStop(client *Client) {
	uid := client.Uid()
	st := hangmanGetState(client.Area())
	st.mu.Lock()

	if !st.optInActive && !st.gameActive {
		st.mu.Unlock()
		client.SendServerMessage("No Hangman game is active in this area.")
		return
	}

	isMod := permissions.IsModerator(client.Perms())
	isCM := client.Area().HasCM(uid)
	isHost := st.hostUID == uid
	isGranted := clientHasCommandGrant(client, "hangman")
	if !isMod && !isCM && !isHost && !isGranted {
		st.mu.Unlock()
		client.SendServerMessage("Only the game host, CMs, or moderators can stop Hangman.")
		return
	}

	word := st.word
	st.gameActive = false
	st.optInActive = false
	st.lastEnd = time.Now()
	st.mu.Unlock()

	sendAreaServerMessage(client.Area(), fmt.Sprintf(
		"🛑 Hangman stopped by %v. The word was: %s",
		client.OOCName(), strings.ToUpper(word),
	))
	addToBuffer(client, "CMD", "Stopped Hangman game", false)
}

// ── Opt-in timer ──────────────────────────────────────────────────────────────

// hangmanOptInTimer waits for the opt-in window then starts the game (or cancels
// if too few players joined).
func hangmanOptInTimer(st *hangmanState) {
	time.Sleep(hangmanOptInDuration)

	st.mu.Lock()
	if !st.optInActive {
		st.mu.Unlock()
		return // cancelled externally (e.g. /hangman stop during opt-in)
	}
	st.optInActive = false
	count := len(st.participants)

	if count < hangmanMinPlayers {
		st.lastEnd = time.Now()
		st.mu.Unlock()
		sendAreaServerMessage(st.area, fmt.Sprintf(
			"🔤 Hangman cancelled — not enough participants (%d joined, need at least %d).",
			count, hangmanMinPlayers,
		))
		return
	}

	word := st.word
	wordLen := utf8.RuneCountInString(word)
	st.gameActive = true
	st.mu.Unlock()

	// Build the initial board (all dashes) with a pre-sized Builder.
	var initialBoard strings.Builder
	initialBoard.Grow(2*wordLen - 1)
	for i := 0; i < wordLen; i++ {
		if i > 0 {
			initialBoard.WriteByte(' ')
		}
		initialBoard.WriteByte('_')
	}

	sendAreaServerMessage(st.area, fmt.Sprintf(
		"🔤 HANGMAN BEGINS! %d player(s) joined.\n%s\nWord (%d letters): %s\n"+
			"Type /hangman guess <letter> or /hangman guess <full word>\n"+
			"Type /hangman status to see the board at any time.",
		count, hangmanArt[0], wordLen, initialBoard.String(),
	))
}

// ── Punishment ────────────────────────────────────────────────────────────────

// hangmanPunishLosers applies a random punishment to every player who made at
// least one wrong guess during a failed game.
func hangmanPunishLosers(a *area.Area, wrongGuessers map[int]bool) {
	punished := 0
	for uid, wasWrong := range wrongGuessers {
		if !wasWrong {
			continue
		}
		c, err := getClientByUid(uid)
		if err != nil {
			continue
		}
		pType := hotPotatoPunishmentPool[rand.Intn(len(hotPotatoPunishmentPool))]
		c.AddPunishment(pType, hangmanPunishDuration, "Hangman: too many wrong guesses")
		c.SendServerMessage(fmt.Sprintf(
			"💀 You made wrong guesses and failed to solve the word! Punished with '%v' for %v.",
			pType, hangmanPunishDuration,
		))
		punished++
	}

	if punished > 0 {
		sendAreaServerMessage(a, fmt.Sprintf(
			"💀 %d player(s) who made wrong guesses received a random punishment!",
			punished,
		))
	} else {
		sendAreaServerMessage(a,
			"🎭 Nobody made wrong guesses — somehow everyone stays clean despite the loss!")
	}
}
