package antlrgala

import "github.com/antlr4-go/antlr/v4"

// galop's generated parser in its antlr flavor joins the harness as "galop".
func init() {
	SameTypes["galop"] = func(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
		tree, err := NewgalaGalopParser(stream).SourceFile()
		if err != nil {
			return nil, err
		}
		return tree, nil
	}
}

// ParseGalopTokens parses stream with galop's antlr flavor generated with
// -Xno-brackets: prediction reads tokens only.
func ParseGalopTokens(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
	return NewgalaGalopTokParser(stream).SourceFile()
}

// ParseGalopByRerun parses stream with galop's antlr flavor, rerunning the
// whole parse in LL mode after a syntax error of the SLL pass, as ANTLR does,
// instead of repairing the failed parse.
func ParseGalopByRerun(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
	p := NewgalaGalopParser(stream)
	return p.parse(p.ruleSourceFile, false)
}
