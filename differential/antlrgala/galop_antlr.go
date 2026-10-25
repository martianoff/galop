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
