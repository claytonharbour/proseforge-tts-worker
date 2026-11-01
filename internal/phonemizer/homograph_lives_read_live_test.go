//go:build unit

package phonemizer

import "testing"

// #112: "live", "lives" and "read" are often said wrong. The keyword rules in data/homographs.json decide
// them (nearest trigger within the clause wins; an exact tie goes to the default), so this is a data change
// plus the tests below. Two tables:
//
//   - homographFixCases: sentences the rules got wrong before #112, plus guards that must keep their reading.
//   - homographBookCases: every read/live/lives sentence in two real books (The Last Ferry Home, Past
//     Imperfect), with the reading the worker gave BEFORE the change, which was checked by hand and is
//     correct. A rule edit that moves any of them is a regression, not an improvement.
//
// What the keyword engine cannot settle (no trigger beats the pronoun next to the word):
// "I read every morning", "Whenever I read a book I take notes", "We read it daily" with a pronoun in front,
// and "The game is live now" / "was live" (an "is" cue also fires from AFTER the word, so "stories live in is
// copied" would turn into the adjective, and "in" as a verb cue ties with "go" in "go live in the studio").
// Those stay on the default, and are not asserted here.

const (
	readPast  = "ɹˈɛd" // "red"
	readPres  = "ɹˈid" // "reed"
	liveVerb  = "lˈɪv" // "liv"
	liveAdj   = "lˈIv" // "lyve"
	livesVerb = "lˈɪvz"
	livesNoun = "lˈIvz" // the plural of "life"
)

type homographCase struct {
	word, sentence, want string
}

var homographFixCases = []homographCase{
	// "lives" as a verb was always said as the plural of "life".
	{"lives", "She lives in Boston.", livesVerb},
	{"lives", "He lives for his work.", livesVerb},
	{"lives", "My grandmother lives alone.", livesVerb},
	{"lives", "Who lives here?", livesVerb},
	{"lives", "She lives with her sister.", livesVerb},
	// guards: the noun stays the noun.
	{"lives", "Cats have nine lives.", livesNoun},
	{"lives", "Three lives were lost.", livesNoun},
	{"lives", "Their lives changed forever.", livesNoun},
	{"lives", "He saves lives in the emergency room.", livesNoun},

	// "live" as an adjective or adverb was always said as the verb.
	{"live", "The band plays live tonight.", liveAdj},
	{"live", "They keep live animals in the shop.", liveAdj},
	{"live", "The soldiers used live ammunition.", liveAdj},
	{"live", "She performed live for an hour.", liveAdj},
	// guards: the verb stays the verb, and the readings that already worked do not move.
	{"live", "Where do you live?", liveVerb},
	{"live", "They live in Paris.", liveVerb},
	{"live", "I want to live forever.", liveVerb},
	{"live", "We live next door to each other.", liveVerb},
	{"live", "Long may you live.", liveVerb},
	{"live", "We watched it live on television.", liveAdj},
	{"live", "It was a live concert.", liveAdj},
	{"live", "Be careful, that is a live wire.", liveAdj},

	// found by the corpus check: the later word "is" must not turn a verb into the adjective.
	{"live", "The database your stories live in is copied once a day.", liveVerb},
	{"live", "This is where we live.", liveVerb},
	// inflected forms are owned by the dictionary and must not move.
	{"living", "He is living in Paris.", "lˈɪvɪŋ"},
	{"living", "She was living there.", "lˈɪvɪŋ"},
	{"lived", "He had lived there for years.", "lˈɪvd"},

	// "read": unambiguous present-tense cues.
	{"read", "Do you read much?", readPres},
	{"read", "What did you read?", readPres},
	{"read", "I'll read it tomorrow.", readPres},
	{"read", "I did read it, honestly.", readPres},
	// "read": a passive participle is the past reading even before "aloud".
	{"read", "The letter was read aloud at the funeral.", readPast},
	{"read", "The book had been read by millions.", readPast},
	// guards: these already worked and must not move.
	{"read", "Read the instructions carefully.", readPres},
	{"read", "Please read this.", readPres},
	{"read", "She likes to read.", readPres},
	{"read", "You should read it.", readPres},
	{"read", "I have read that book.", readPast},
	{"read", "Have you read it?", readPast},
	{"read", "He read the letter and wept.", readPast},
	{"read", "He read aloud to the children.", readPast},
	{"read", "She read every book in the library.", readPast},
	{"read", "Last night I read three chapters.", readPast},
}

var homographBookCases = []homographCase{
	{"reading", "She closed the document when she realised she was reading the same sentence for the fourth time.", "ɹˈidɪŋ"},
	{"read", "She had read it once before putting it away.", "ɹˈɛd"},
	{"Read", "Read half of several books.”", "ɹˈid"},
	{"live", "Where you'd live.", "lˈɪv"},
	{"read", "Mara crossed out the note that read final phase beside the threshold.", "ɹˈɛd"},
	{"read", "Jo read the figure twice.", "ɹˈɛd"},
	{"read", "Eli leaned close enough to read the equipment list.", "ɹˈid"},
	{"read", "She spat, read it again, and went downstairs with toothpaste still sharp on her tongue.", "ɹˈɛd"},
	{"read", "She read out the number.", "ɹˈɛd"},
	{"read", "Jo read the red line Mara drew across its doorway.", "ɹˈɛd"},
	{"read", "Mara read it twice before putting her phone away.", "ɹˈɛd"},
	{"live", "If I’m busy enough, I don’t have to ask whether I could choose to live here.", "lˈɪv"},
	{"read", "He read both messages.", "ɹˈɛd"},
	{"live", "“But I can’t promise to live here forever as evidence that I mean what I say.", "lˈɪv"},
	{"live", "We’ll live without the fancy plaster in the bedrooms.”", "lˈɪv"},
	{"live", "And yes, I’d like to see where you live.”", "lˈɪv"},
	{"live", "Whether one of them would eventually want a home the other could not live in.", "lˈɪv"},
	{"read", "She read the first line, then the second, then the third, because the first two would not hold still.", "ɹˈɛd"},
	{"lived", "The figures now lived in a folder behind the reception desk: grant expenditure, the insurer’s contribution toward the storm damage, and the modest repair loan Jo had agreed to after three separate conversations with her accountant.", "lˈɪvd"},
	{"Read", "Read in any order.", "ɹˈid"},
	{"lives", "A footman led them through corridors lined with paintings of people who looked like they had never, in their entire lives, been uncertain about anything.", "lˈIvz"},
	{"read", "Jonathan read the letter twice.", "ɹˈɛd"},
	{"read", "Cornelius read it.", "ɹˈɛd"},
	{"read", "Jonathan read the letter, then set it on his workbench beside a half-finished waistcoat, a spool of thread, and a fossilized oyster.", "ɹˈɛd"},
	{"live", "Pip continued, “birds and humans live in harmony!", "lˈɪv"},
	{"read", "Pip could not read a room.", "ɹˈid"},
	{"live", "Tito knew the look: they were deciding whether strangers would live long enough to learn the outcome.", "lˈɪv"},
	{"read", "Rome received visitors from territories Tito had never read about, and Leonardo had arrived wrapped in enough costly-looking cloth to make an introduction plausible.", "ɹˈɛd"},
	{"Read", "Read the manual.”", "ɹˈid"},
	{"lives", "“You can argue about the cannonball for the rest of your lives.", "lˈIvz"},
	{"read", "He had read about this garden long before he had stood in it, and he knew how little of it would survive outside the accounts of people who claimed to have seen it.", "ɹˈɛd"},
	{"read", "Leonardo read.", "ɹˈɛd"},
	{"read", "Tito’s script had tightened toward the bottom until Percival had to read the final entry aloud under the workshop lamp: NUMBER 14: IN THE EVENT OF TEMPORAL PARADOX, REMAIN CALM AND CONSULT THE LOG.", "ɹˈid"},
	{"read", "Leonardo read them.", "ɹˈɛd"},
	{"read", "Streetlamp light barely reached Tito’s cramped handwriting, but Percival read it easily.", "ɹˈɛd"},
	{"read", "He read the rules about weapons, Pip’s scouting briefings, and painting Tito’s shell.", "ɹˈɛd"},
	{"lives", "It begins, as so many sensible-shaped lives do, with two and a half glasses of wine and a very poorly assembled kettle.", "lˈIvz"},
	{"read", "He had read so many descriptions of the Roman sky that he could have supplied one for any occasion, but none of them accounted for this blue.", "ɹˈɛd"},
	{"read", "Beyond it lay a courtyard in late-autumn light, thicker and warmer than the light Henry had imagined when he read about this century.", "ɹˈɛd"},
	{"live", "“You look as if you live here.”", "lˈɪv"},
	{"read", "Soot and gum and whatever else its maker had dissolved them in—wine, if the accounts Henry had read were to be believed—ran together in a dark, widening fan.", "ɹˈɛd"},
	{"read", "Henry had spent his life being read by older women.", "ɹˈɛd"},
	{"lives", "The chipped cups, who had heard a great deal in their lives and were known to be discerning, approved of the laugh.", "lˈIvz"},
	{"read", "He had read about the room they were in.", "ɹˈɛd"},
	{"read", "He could not read her expression; she looked as though she were considering something she had already spent a long time thinking about.", "ɹˈid"},
	{"read", "He had known this plan since graduate school, from a drawing he had copied into a footnote that perhaps eleven people had read.", "ɹˈɛd"},
	{"read", "“I would have read it carefully.”", "ɹˈɛd"},
	{"read", "Henry had read every account he could find, including the memoirs and the letters that contradicted them.", "ɹˈɛd"},
	{"lives", "In three different centuries, the Sheriff’s officers looked up from pursuits they no longer had reason to continue; the purpose left their faces, and they went quietly on with their lives.", "lˈIvz"},
}

func TestHomographLivesReadLiveFixes(t *testing.T) {
	p := newTestPhonemizer(t)
	for _, c := range homographFixCases {
		if got := phonemesForWord(t, p, c.sentence, c.word); got != c.want {
			t.Errorf("%q in %q: got %s, want %s", c.word, c.sentence, got, c.want)
		}
	}
}

func TestHomographBookSentencesDoNotMove(t *testing.T) {
	p := newTestPhonemizer(t)
	for _, c := range homographBookCases {
		if got := phonemesForWord(t, p, c.sentence, c.word); got != c.want {
			t.Errorf("regression: %q in %q: got %s, was %s", c.word, c.sentence, got, c.want)
		}
	}
}
