package antlrgala

import "github.com/antlr4-go/antlr/v4"

// galop's generated parser joins the harness as the "galop" companion.
func init() {
	Companions["galop"] = func(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
		tree, err := NewgalaGalopParser(stream).SourceFile()
		if err != nil {
			return nil, err
		}
		return tree, nil
	}
}
