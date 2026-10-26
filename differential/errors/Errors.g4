// A grammar whose decisions galop settles without runtime prediction, with
// the constructs ANTLR's error strategy treats differently: rule-level token
// sets, set matches, optional blocks, `*` and `+` loops with one or several
// alternatives or a block first, non-greedy loops, nullable rules, labels,
// wildcards and comments on a hidden channel. errors_test.go checks galop's
// syntax errors against ANTLR's on mutations of a few programs.
grammar Errors;

prog : decl* EOF ;
decl : 'let' name ('=' expr)? ';'           # Let
     | 'fn' name '(' params? ')' block       # Fn
     | 'type' name ('{' field+ '}' | '=' type) # TypeDecl
     ;
params : param (',' param)* ;
param : name ':' type ;
field : name type ';' ;
type : name ('[' type ']')? | '*' type ;
block : '{' stmt* '}' ;
stmt : 'return' expr? ';'
     | 'if' expr block ('else' (block | stmt))?
     | 'loop' (stmt | block)+ 'end'
     | expr ('=' expr)? ';'
     | 'pair' ((name | INT) ('+' | '-'))+ ';'
     | 'each' (name | '(' expr ')')+? 'do' block
     | 'skip' (name ',')*? 'until' name ';'
     | ';'
     ;
expr : unary (op unary)* ;
unary : ('-' | '!')* primary ;
primary : name args? | literal | '(' expr ')' | '[' ~']'* ']' | '#' . ;
args : '(' (expr (',' expr)*)? ')' ;
op : '+' | '-' | '*' | '/' | '==' | '<' ;
literal : INT | STRING | 'true' | 'false' ;
name : ID | 'type' ;

ID : [a-zA-Z_] [a-zA-Z_0-9]* ;
INT : [0-9]+ ;
STRING : '"' ~["\n]* '"' ;
COMMENT : '//' ~[\n]* -> channel(HIDDEN) ;
WS : [ \t\r\n]+ -> skip ;
