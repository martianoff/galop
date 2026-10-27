package antlr

import "github.com/antlr4-go/antlr/v4"

// GalopEntries are Repair.g4's entry rules in galop's antlr flavor, by name,
// as in native.Entries: Parse repairs a failed SLL parse where the rule allows
// it, Rerun reruns the whole parse in LL mode. Both leave the token stream
// where the parse stopped.
var GalopEntries = map[string]struct {
	Parse, Rerun func(*RepairGalopParser) (antlr.Tree, error)
}{
	"s1": {galop((*RepairGalopParser).S1), rerun((*RepairGalopParser).ruleS1, RepairParserRULE_s1)},
	"s2": {galop((*RepairGalopParser).S2), rerun((*RepairGalopParser).ruleS2, RepairParserRULE_s2)},
	"s3": {galop((*RepairGalopParser).S3), rerun((*RepairGalopParser).ruleS3, RepairParserRULE_s3)},
	"s4": {galop((*RepairGalopParser).S4), rerun((*RepairGalopParser).ruleS4, RepairParserRULE_s4)},
	"s5": {galop((*RepairGalopParser).S5), rerun((*RepairGalopParser).ruleS5, RepairParserRULE_s5)},
	"s6": {galop((*RepairGalopParser).S6), rerun((*RepairGalopParser).ruleS6, RepairParserRULE_s6)},
	"s7": {galop((*RepairGalopParser).S7), rerun((*RepairGalopParser).ruleS7, RepairParserRULE_s7)},
	"s8": {galop((*RepairGalopParser).S8), rerun((*RepairGalopParser).ruleS8, RepairParserRULE_s8)},
	"s9": {galop((*RepairGalopParser).S9), rerun((*RepairGalopParser).ruleS9, RepairParserRULE_s9)},
}

// Entries are the same rules in ANTLR's own parser.
var Entries = map[string]func(*RepairParser) antlr.Tree{
	"s1": func(p *RepairParser) antlr.Tree { return p.S1() },
	"s2": func(p *RepairParser) antlr.Tree { return p.S2() },
	"s3": func(p *RepairParser) antlr.Tree { return p.S3() },
	"s4": func(p *RepairParser) antlr.Tree { return p.S4() },
	"s5": func(p *RepairParser) antlr.Tree { return p.S5() },
	"s6": func(p *RepairParser) antlr.Tree { return p.S6() },
	"s7": func(p *RepairParser) antlr.Tree { return p.S7() },
	"s8": func(p *RepairParser) antlr.Tree { return p.S8() },
	"s9": func(p *RepairParser) antlr.Tree { return p.S9() },
}

func galop[C antlr.Tree](parse func(*RepairGalopParser) (C, error)) func(*RepairGalopParser) (antlr.Tree, error) {
	return func(p *RepairGalopParser) (antlr.Tree, error) {
		c, err := parse(p)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func rerun(rule func(*RepairGalopParser, antlr.ParserRuleContext, int) antlr.ParserRuleContext, index int) func(*RepairGalopParser) (antlr.Tree, error) {
	return func(p *RepairGalopParser) (antlr.Tree, error) {
		return p.parse(func(parent antlr.ParserRuleContext, state int) antlr.ParserRuleContext {
			return rule(p, parent, state)
		}, RepairGalopRuleStartState[index], false)
	}
}
