package nativegala

// Stats is the work one parse did, as the generated parser counts it:
// Passes (1 when the first pass succeeds), Predictions (runtime SLL
// predictions) and FullContext (full-context predictions).
type Stats = galaGalopStats

// ParseWithStats parses src as Parse does (the entry's start state and
// repair plan come from the generated tables) and reports the parse's work.
func ParseWithStats(src string) (SourceFileNode, Stats, error) {
	ts := Tokens(src)
	if len(ts.Errors) > 0 {
		return SourceFileNode{}, Stats{}, &ts.Errors[0]
	}
	p := newgalaGalopParser(ts)
	rule := GalaRULE_sourceFile
	t, root, err := p.run((*galaGalopParser).ruleSourceFile, galaGalopRuleStartState[rule], galaGalopRuleRepairs[rule])
	if err != nil {
		return SourceFileNode{}, p.stats, err
	}
	return SourceFileNode{GalaNodeRef{t, root}}, p.stats, nil
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
