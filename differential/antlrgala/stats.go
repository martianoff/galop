package antlrgala

import "github.com/antlr4-go/antlr/v4"

// Stats is the work one parse of galop's antlr flavor did, as the generated
// parser counts it (see nativegala.Stats).
type Stats = galaGalopStats

// ParseGalopWithStats parses stream with galop's antlr flavor, as
// NewgalaGalopParser(stream).SourceFile() does, and reports the parse's work.
func ParseGalopWithStats(stream antlr.TokenStream) (antlr.ParserRuleContext, Stats, error) {
	p := NewgalaGalopParser(stream)
	ctx, err := p.parse(p.ruleSourceFile, galaGalopRuleStartState[galaParserRULE_sourceFile], true)
	return ctx, p.stats, err
}

// CacheStates is how many states the process's SLL DFA cache and
// full-context cache hold.
func CacheStates() (sll, fullContext int) {
	galaGalopDFA.mu.Lock()
	sll = len(galaGalopDFA.states)
	galaGalopDFA.mu.Unlock()
	galaGalopFull.mu.RLock()
	fullContext = len(galaGalopFull.states)
	galaGalopFull.mu.RUnlock()
	return sll, fullContext
}
