// A lexer grammar for galop's Python target tests: lexer commands, a channel
// of its own, a recursive fragment past the DFA's rule-call depth (the tests
// generate it with -DlexerDepth=2, so nested blocks are matched on the ATN),
// non-greedy loops, code points above ASCII, literals with escapes, and a
// token named like a Python word ANTLR's Python target escapes.
lexer grammar Commands;

channels { COMMENTS }

BLOCK : '{' BODY '}' ;
fragment BODY : ( '{' BODY '}' | ~[{}] )* ;

LINE_COMMENT : '//' .*? '\n' -> channel(COMMENTS) ;
DOC : '/**' .*? '*/' -> channel(HIDDEN) ;
COMMENT : '/*' .*? '*/' -> skip ;

AT : '@' -> more ;
DOLLAR : '$' -> more ;
NUM : [0-9]+ -> type(ID) ;
ID : [a-zA-Z_\u00C0-\u024F\u0370-\u03FF]+ ;
None : 'none' ;
STR : '"' ( '\\' . | ~["\\\n] )* '"' ;
QUOTE : '\'' ;
NL : '\r'? '\n' ;
TAB : '\t' ;
ARROW : '\u2192' ;
EMOJI : [\u{1F600}-\u{1F64F}] ;
WS : ' '+ -> skip ;
