package antlrgala

import "github.com/antlr4-go/antlr/v4"

// Stats is the work one parse of galop's antlr flavor did, as the generated
// parser counts it (see nativegala.Stats).
type Stats = galaGalopStats

// ParseGalopWithStats parses stream with galop's antlr flavor, as
// NewgalaGalopParser(stream).SourceFile() does, and reports the parse's work.
func ParseGalopWithStats(stream antlr.TokenStream) (antlr.ParserRuleContext, Stats, error) {
	p := NewgalaGalopParser(stream)
	rule := galaParserRULE_sourceFile
	ctx, err := p.parse(p.ruleSourceFile, galaGalopRuleStartState[rule], galaGalopRuleRepairs[rule])
	return ctx, p.stats, err
}

// CacheStates is how many states the process's SLL DFA cache and
// full-context cache hold.
func CacheStates() (sll, fullContext int) {
	sll, _, _ = galaGalopDFAStats()
	galaGalopFull.mu.RLock()
	fullContext = len(galaGalopFull.states)
	galaGalopFull.mu.RUnlock()
	return sll, fullContext
}

// SLLCache reports the SLL DFA cache's bound at work: the most states it
// held, and how many times it started over.
func SLLCache() (peak, resets int) {
	_, peak, resets = galaGalopDFAStats()
	return peak, resets
}
